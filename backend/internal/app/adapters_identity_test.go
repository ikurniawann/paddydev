package app

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"nuhabit/backend/internal/modules/identity"
	"nuhabit/backend/internal/platform/stall"
	"nuhabit/backend/internal/platform/testutil"
)

// The stall switcher routes run through identityStalls, so they are tested
// here with committed fixtures (the routes read through the pool).
func TestIdentityStallRoutes(t *testing.T) {
	deps := testutil.Deps(t, nil)
	mux := testutil.Mux(identity.New(deps, identityStalls{db: deps.DB}))
	ctx := context.Background()
	sfx := testutil.RandomHex(4)
	id := func(sql string, args ...any) string {
		t.Helper()
		var v string
		if err := deps.DB.QueryRow(ctx, sql, args...).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	holding := id(`INSERT INTO configuration.holdings (name, code) VALUES ('H '||$1, 'H'||$1) RETURNING id::text`, sfx)
	company := id(`INSERT INTO configuration.companies (holding_id, name, code) VALUES ($1, 'C '||$2, 'C'||$2) RETURNING id::text`, holding, sfx)
	branch := id(`INSERT INTO configuration.branches (company_id, name, code) VALUES ($1, 'B '||$2, 'B'||$2) RETURNING id::text`, company, sfx)
	other := id(`INSERT INTO configuration.branches (company_id, name, code) VALUES ($1, 'O '||$2, 'O'||$2) RETURNING id::text`, company, sfx)
	stall2 := id(`INSERT INTO configuration.warehouses (branch_id, name, code) VALUES ($1, 'Stall 2', 'STALL-02') RETURNING id::text`, branch)
	stall1 := id(`INSERT INTO configuration.warehouses (branch_id, name, code) VALUES ($1, 'Stall 1', 'STALL-01') RETURNING id::text`, branch)
	outside := id(`INSERT INTO configuration.warehouses (branch_id, name, code) VALUES ($1, 'Luar', 'STALL-03') RETURNING id::text`, other)
	closed := id(`INSERT INTO configuration.warehouses (branch_id, name, code, is_active) VALUES ($1, 'Tutup', 'STALL-09', false) RETURNING id::text`, branch)
	t.Cleanup(func() {
		for _, sql := range []string{
			`DELETE FROM configuration.user_warehouses WHERE warehouse_id IN (SELECT id FROM configuration.warehouses WHERE branch_id = ANY($1))`,
			`DELETE FROM configuration.warehouses WHERE branch_id = ANY($1)`,
			`DELETE FROM configuration.branches WHERE id = ANY($1)`,
		} {
			_, _ = deps.DB.Exec(ctx, sql, []string{branch, other})
		}
		_, _ = deps.DB.Exec(ctx, `DELETE FROM configuration.companies WHERE id = $1`, company)
		_, _ = deps.DB.Exec(ctx, `DELETE FROM configuration.holdings WHERE id = $1`, holding)
	})
	place := func(s testutil.Staff, warehouses ...string) {
		for _, w := range warehouses {
			if _, err := deps.DB.Exec(ctx, `INSERT INTO configuration.user_warehouses (user_id, warehouse_id) VALUES ($1, $2)`, s.UserID, w); err != nil {
				t.Fatal(err)
			}
		}
	}

	superAdmin := testutil.CreateStaff(t, testutil.StaffOptions{Role: "super_admin", BranchID: &branch})
	cashier := testutil.CreateStaff(t, testutil.StaffOptions{Role: "pos", BranchID: &branch})
	place(cashier, stall1)
	roamer := testutil.CreateStaff(t, testutil.StaffOptions{Role: "pos", BranchID: &branch})
	place(roamer, stall1, stall2)

	post := func(s testutil.Staff, warehouseID any) (int, string, []string) {
		t.Helper()
		rec, _ := testutil.Do(t, mux, testutil.AsStaff(testutil.Request("POST", "/api/auth/active-stall", map[string]any{"warehouse_id": warehouseID}), s))
		return rec.Code, rec.Body.String(), rec.Result().Header.Values("Set-Cookie")
	}
	for _, c := range []struct {
		name   string
		as     testutil.Staff
		id     any
		status int
		want   string
	}{
		{"single placement", cashier, stall2, 403, `{"success":false,"error":"Akun ini tidak perlu mengganti outlet"}`},
		{"roamer to all", roamer, nil, 403, `{"success":false,"error":"Anda hanya dapat memilih outlet penempatan Anda"}`},
		{"roamer outside placements", roamer, outside, 403, `{"success":false,"error":"Outlet di luar penempatan Anda"}`},
		{"other branch", superAdmin, outside, 403, `{"success":false,"error":"Outlet berada di luar branch Anda"}`},
		{"inactive", superAdmin, closed, 404, `{"success":false,"error":"Outlet not found or inactive"}`},
		{"not a uuid", superAdmin, "x", 400, `{"success":false,"error":"Validation failed","details":[{"code":"invalid_format","path":["warehouse_id"],"message":"Invalid UUID"}]}`},
	} {
		status, body, cookies := post(c.as, c.id)
		if status != c.status || body != c.want || len(cookies) != 0 {
			t.Errorf("%s: %d %s %v", c.name, status, body, cookies)
		}
	}

	status, body, cookies := post(roamer, stall2)
	want := `{"success":true,"data":{"stall":{"id":"` + stall2 + `","name":"Stall 2","code":"STALL-02"}}}`
	if status != 200 || body != want || len(cookies) != 2 {
		t.Fatalf("roamer switch: %d %s %v", status, body, cookies)
	}
	if !strings.HasPrefix(cookies[0], "nuhabit-active-stall="+stall2+"; Path=/; Expires=") ||
		!strings.HasSuffix(cookies[0], "; Max-Age=2592000; Secure; HttpOnly; SameSite=lax") ||
		cookies[1] != "arkiv-active-stall=; Path=/; Max-Age=0; Secure; HttpOnly; SameSite=lax" {
		t.Fatalf("cookies:\n%s", strings.Join(cookies, "\n"))
	}
	if status, body, cookies := post(superAdmin, nil); status != 200 || body != `{"success":true,"data":{"stall":null}}` ||
		!strings.HasPrefix(cookies[0], "nuhabit-active-stall=all;") {
		t.Fatalf("all stalls: %d %s %v", status, body, cookies)
	}

	options := func(s testutil.Staff, cookies ...*http.Cookie) string {
		t.Helper()
		r := testutil.AsStaff(testutil.Request("GET", "/api/auth/stall-options", nil), s)
		for _, c := range cookies {
			r.AddCookie(c)
		}
		rec, _ := testutil.Do(t, mux, r)
		if rec.Code != 200 {
			t.Fatalf("stall-options: %d %s", rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}
	view := func(id, name, code string) string {
		return `{"id":"` + id + `","name":"` + name + `","code":"` + code + `","branch_id":"` + branch + `","branch_name":"B ` + sfx + `","label":"` + name + `"}`
	}
	s1, s2 := view(stall1, "Stall 1", "STALL-01"), view(stall2, "Stall 2", "STALL-02")
	// A branch gets its Main Storage from a trigger; it sorts first.
	mainStorage := id(`SELECT id::text FROM configuration.warehouses WHERE branch_id = $1 AND code = 'MAIN'`, branch)
	branchStalls := view(mainStorage, "Main Storage", "MAIN") + "," + s1 + "," + s2
	for _, c := range []struct {
		name string
		got  string
		want string
	}{
		// A cashier reads their stall (receipts print it) but gets no list.
		{"cashier", options(cashier), `{"success":true,"data":{"all_access":false,"can_switch":false,"active":` + s1 + `,"stalls":[]}}`},
		{"roamer unset", options(roamer), `{"success":true,"data":{"all_access":false,"can_switch":true,"active":` + s1 + `,"stalls":[` + s1 + `,` + s2 + `]}}`},
		{"super admin unset", options(superAdmin), `{"success":true,"data":{"all_access":true,"can_switch":true,"active":null,"stalls":[` + branchStalls + `]}}`},
		{"legacy cookie", options(superAdmin, &http.Cookie{Name: stall.LegacyCookieName, Value: stall2}), `{"success":true,"data":{"all_access":true,"can_switch":true,"active":` + s2 + `,"stalls":[` + branchStalls + `]}}`},
		{"explicit all", options(roamer, &http.Cookie{Name: stall.CookieName, Value: "all"}), `{"success":true,"data":{"all_access":false,"can_switch":true,"active":null,"stalls":[` + s1 + `,` + s2 + `]}}`},
	} {
		if c.got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, c.got, c.want)
		}
	}
}

// An all-access user without a branch sees every outlet of every branch; each
// branch has its own "Main Storage", so rows must say which branch they are.
func TestIdentityStallOptionsLabelsAcrossBranches(t *testing.T) {
	deps := testutil.Deps(t, nil)
	mux := testutil.Mux(identity.New(deps, identityStalls{db: deps.DB}))
	ctx := context.Background()
	sfx := testutil.RandomHex(4)
	id := func(sql string, args ...any) string {
		t.Helper()
		var v string
		if err := deps.DB.QueryRow(ctx, sql, args...).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	holding := id(`INSERT INTO configuration.holdings (name, code) VALUES ('H '||$1, 'H'||$1) RETURNING id::text`, sfx)
	company := id(`INSERT INTO configuration.companies (holding_id, name, code) VALUES ($1, 'C '||$2, 'C'||$2) RETURNING id::text`, holding, sfx)
	// Inserted in reverse alphabetical order so the sort cannot be insertion order.
	branchB := id(`INSERT INTO configuration.branches (company_id, name, code) VALUES ($1, 'Zeta '||$2, 'Z'||$2) RETURNING id::text`, company, sfx)
	branchA := id(`INSERT INTO configuration.branches (company_id, name, code) VALUES ($1, 'Alpha '||$2, 'A'||$2) RETURNING id::text`, company, sfx)
	stallB := id(`INSERT INTO configuration.warehouses (branch_id, name, code) VALUES ($1, 'Stall 1', 'STALL-01') RETURNING id::text`, branchB)
	t.Cleanup(func() {
		for _, sql := range []string{
			`DELETE FROM configuration.user_warehouses WHERE warehouse_id IN (SELECT id FROM configuration.warehouses WHERE branch_id = ANY($1))`,
			`DELETE FROM configuration.warehouses WHERE branch_id = ANY($1)`,
			`DELETE FROM configuration.branches WHERE id = ANY($1)`,
		} {
			_, _ = deps.DB.Exec(ctx, sql, []string{branchA, branchB})
		}
		_, _ = deps.DB.Exec(ctx, `DELETE FROM configuration.companies WHERE id = $1`, company)
		_, _ = deps.DB.Exec(ctx, `DELETE FROM configuration.holdings WHERE id = $1`, holding)
	})
	mainA := id(`SELECT id::text FROM configuration.warehouses WHERE branch_id = $1 AND code = 'MAIN'`, branchA)
	mainB := id(`SELECT id::text FROM configuration.warehouses WHERE branch_id = $1 AND code = 'MAIN'`, branchB)

	type stallJSON struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Code       string `json:"code"`
		BranchID   string `json:"branch_id"`
		BranchName string `json:"branch_name"`
		Label      string `json:"label"`
	}
	type optionsJSON struct {
		Data struct {
			Active *stallJSON  `json:"active"`
			Stalls []stallJSON `json:"stalls"`
		} `json:"data"`
	}
	options := func(s testutil.Staff, cookies ...*http.Cookie) optionsJSON {
		t.Helper()
		r := testutil.AsStaff(testutil.Request("GET", "/api/auth/stall-options", nil), s)
		for _, c := range cookies {
			r.AddCookie(c)
		}
		rec, _ := testutil.Do(t, mux, r)
		if rec.Code != 200 {
			t.Fatalf("stall-options: %d %s", rec.Code, rec.Body.String())
		}
		var out optionsJSON
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	superAdmin := testutil.CreateStaff(t, testutil.StaffOptions{Role: "super_admin"})
	got := options(superAdmin)
	var mine []stallJSON
	for _, st := range got.Data.Stalls {
		if st.BranchID == branchA || st.BranchID == branchB {
			mine = append(mine, st)
		}
	}
	nameA, nameB := "Alpha "+sfx, "Zeta "+sfx
	want := []stallJSON{
		{mainA, "Main Storage", "MAIN", branchA, nameA, "Main Storage — " + nameA},
		{mainB, "Main Storage", "MAIN", branchB, nameB, "Main Storage — " + nameB},
		{stallB, "Stall 1", "STALL-01", branchB, nameB, "Stall 1 — " + nameB},
	}
	if len(mine) != len(want) {
		t.Fatalf("stalls of the two branches: %+v", mine)
	}
	for i := range want {
		if mine[i] != want[i] {
			t.Errorf("stall %d:\n got %+v\nwant %+v", i, mine[i], want[i])
		}
	}

	// The active outlet uses the same labelling as the list it sits in.
	active := options(superAdmin, &http.Cookie{Name: stall.CookieName, Value: mainB}).Data.Active
	if active == nil || *active != want[1] {
		t.Errorf("active: %+v, want %+v", active, want[1])
	}
	if all := options(superAdmin, &http.Cookie{Name: stall.CookieName, Value: "all"}).Data.Active; all != nil {
		t.Errorf("explicit all: active %+v", all)
	}

	// A user confined to one branch keeps the plain names.
	inBranch := testutil.CreateStaff(t, testutil.StaffOptions{Role: "super_admin", BranchID: &branchB})
	one := options(inBranch)
	if len(one.Data.Stalls) != 2 || one.Data.Stalls[0].Label != "Main Storage" || one.Data.Stalls[1].Label != "Stall 1" ||
		one.Data.Stalls[0].BranchName != nameB {
		t.Errorf("single branch: %+v", one.Data.Stalls)
	}
}

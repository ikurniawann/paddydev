package desktop_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nuhabit/backend/internal/modules/desktop"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/module"
	"nuhabit/backend/internal/platform/testutil"
)

func setup(t *testing.T) (module.Deps, http.Handler) {
	deps := testutil.Deps(t, nil)
	return deps, testutil.Mux(desktop.New(deps))
}

func get(t *testing.T, h http.Handler, s testutil.Staff, target string) (int, map[string]any, string) {
	t.Helper()
	rec, body := testutil.Do(t, h, testutil.AsStaff(testutil.Request("GET", target, nil), s))
	return rec.Code, body, rec.Body.String()
}

func TestDesktopAuth(t *testing.T) {
	deps, mux := setup(t)
	for _, path := range []string{"/api/desktop/inbox", "/api/desktop/overview", "/api/desktop/search?q=ab", "/api/desktop/status", "/api/desktop/preferences", "/api/desktop/stream"} {
		rec, _ := testutil.Do(t, mux, testutil.Request("GET", path, nil))
		if rec.Code != 401 || rec.Body.String() != `{"success":false,"error":"Authentication required"}` {
			t.Errorf("%s: %d %s", path, rec.Code, rec.Body.String())
		}
	}
	// The wallpaper list is public, also behind the gate.
	rec, body := testutil.Do(t, deps.Auth.Gate(mux), testutil.Request("GET", "/api/desktop/wallpapers", nil))
	if _, isList := body["data"].([]any); rec.Code != 200 || body["success"] != true || !isList {
		t.Fatalf("wallpapers: %d %s", rec.Code, rec.Body.String())
	}

	// The board needs a dashboard menu.
	cashier := testutil.CreateStaff(t, testutil.StaffOptions{Role: "pos", Menus: map[string][]string{"pos.operations": nil}})
	for _, path := range []string{"/api/desktop/overview", "/api/desktop/stream"} {
		if status, _, raw := get(t, mux, cashier, path); status != 403 || raw != `{"success":false,"error":"Insufficient permissions"}` {
			t.Errorf("%s: %d %s", path, status, raw)
		}
	}
}

func TestDesktopOverview(t *testing.T) {
	_, mux := setup(t)
	owner := testutil.CreateStaff(t, testutil.StaffOptions{Role: "admin", Menus: map[string][]string{"dashboard": nil}})

	status, body, raw := get(t, mux, owner, "/api/desktop/overview?periode=mtd")
	if status != 200 || body["cached"] != false {
		t.Fatalf("%d %s", status, raw)
	}
	data := body["data"].(map[string]any)
	if gagal := data["gagal"].([]any); len(gagal) != 0 {
		t.Fatalf("failed sections %v", gagal)
	}
	// Key order follows DesktopOverview.
	wantKeys := []string{`{"data":{"dibuatPada":`, `"periode":{"periode":{"kind":"mtd"`, `"omzetPeriode":{"omzet":`, `"dampakPromo":{"diskon":`,
		`"tamuDiMeja":{"tamu":`, `"pulsaBisnis":{"hariIni":{"omzet":`, `"timHariIni":{"aktif":`, `"perluKeputusan":{"cuti":`,
		`"stokMenipis":{"jumlah":`, `"member":{"memberBaru7Hari":`, `"gagal":[]},"cached":false}`}
	last := -1
	for _, k := range wantKeys {
		i := strings.Index(raw, k)
		if i <= last {
			t.Fatalf("%s out of order in %s", k, raw)
		}
		last = i
	}
	pulse := data["pulsaBisnis"].(map[string]any)
	if days := pulse["tujuhHari"].([]any); len(days) != 7 {
		t.Fatalf("sparkline %v", days)
	}
	team := data["timHariIni"].(map[string]any)
	if team["belum"].(float64) < 0 {
		t.Fatal("belum negative")
	}

	// The second read within a minute comes from the per-period cache.
	if _, body, _ := get(t, mux, owner, "/api/desktop/overview?periode=mtd"); body["cached"] != true {
		t.Fatal("not cached")
	}
	if _, body, _ := get(t, mux, owner, "/api/desktop/overview?periode=bogus"); body["cached"] != false ||
		body["data"].(map[string]any)["periode"].(map[string]any)["periode"].(map[string]any)["kind"] != "today" {
		t.Fatal("unknown periode must fall back to today, cached apart from mtd")
	}
}

func TestDesktopInbox(t *testing.T) {
	deps, mux := setup(t)
	ctx := context.Background()
	var emp, leave string
	if err := deps.DB.QueryRow(ctx, `INSERT INTO hris.employees (full_name, nip, email, phone, join_date)
		VALUES ('Inbox Test', 'T-'||md5(random()::text), 'inbox@desktop.test', '', '2026-01-01') RETURNING id::text`).Scan(&emp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = deps.DB.Exec(ctx, `DELETE FROM hris.employees WHERE id = $1`, emp) })
	// The oldest start date sorts first, so the fixture is always listed.
	if err := deps.DB.QueryRow(ctx, `INSERT INTO hris.leaves (employee_id, leave_type, start_date, end_date, total_days, reason)
		VALUES ($1, 'annual', '1999-08-18', '1999-09-20', 3.5, 'Acara keluarga di luar kota, berangkat pagi dan kembali malam hari berikutnya') RETURNING id::text`, emp).Scan(&leave); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = deps.DB.Exec(ctx, `DELETE FROM hris.leaves WHERE id = $1`, leave) })

	hr := testutil.CreateStaff(t, testutil.StaffOptions{Role: "pos", Menus: map[string][]string{"hris.workforce.leaves": nil}})
	status, body, raw := get(t, mux, hr, "/api/desktop/inbox")
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	sections := body["data"].(map[string]any)["sections"].([]any)
	if len(sections) != 1 {
		t.Fatalf("an HR menu sees only leave requests: %s", raw)
	}
	want := `{"key":"cuti","label":"Pengajuan cuti","total":`
	if !strings.Contains(raw, want) {
		t.Fatalf("section head missing: %s", raw)
	}
	// A trigger recounts total_days as working days.
	var days string
	_ = deps.DB.QueryRow(ctx, `SELECT total_days::float8::text FROM hris.leaves WHERE id = $1`, leave).Scan(&days)
	item := sections[0].(map[string]any)["items"].([]any)[0].(map[string]any)
	// EPIC-072 R6: the panel no longer decides; "Buka" leads to the HR Cuti
	// page with the detail. The reason (often medical) is not shown here:
	// the inbox reaches every hris.* menu, wider than Cuti readers (R7).
	if item["id"] != leave || item["title"] != "Inbox Test" ||
		item["subtitle"] != "Cuti Tahunan · "+days+" hari · 18 Agu–20 Sep" ||
		item["href"] != "/dashboard/hris/leaves" || item["actionable"] != nil {
		t.Fatalf("item %v", item)
	}

	none := testutil.CreateStaff(t, testutil.StaffOptions{Role: "pos", Menus: map[string][]string{"pos.operations": nil}})
	if _, _, raw := get(t, mux, none, "/api/desktop/inbox"); raw != `{"success":true,"data":{"sections":[]}}` {
		t.Fatalf("no sections: %s", raw)
	}
}

func TestDesktopSearch(t *testing.T) {
	deps, mux := setup(t)
	ctx := context.Background()
	tag := "zq" + testutil.RandomHex(4)
	var id string
	if err := deps.DB.QueryRow(ctx, `INSERT INTO pos.pos_customers (phone, name, membership_tier)
		VALUES ('+6299'||$2, $1||' Budi', 'gold') RETURNING id::text`, tag, testutil.RandomHex(4)).Scan(&id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = deps.DB.Exec(ctx, `DELETE FROM pos.pos_customers WHERE id = $1`, id) })
	var phone string
	_ = deps.DB.QueryRow(ctx, `SELECT phone FROM pos.pos_customers WHERE id = $1`, id).Scan(&phone)

	crm := testutil.CreateStaff(t, testutil.StaffOptions{Role: "pos", Menus: map[string][]string{"crm.members": nil}})
	_, _, raw := get(t, mux, crm, "/api/desktop/search?q=+"+strings.ToUpper(tag)+"++")
	want := `{"success":true,"data":{"groups":[{"source":"member","label":"Member","items":[{"source":"member","id":"` + id +
		`","title":"` + tag + ` Budi","subtitle":"` + phone + ` · gold","href":"/dashboard/crm/members?q=%2B` + strings.TrimPrefix(phone, "+") +
		`"}]}],"query":"` + strings.ToUpper(tag) + `"}}`
	if raw != want {
		t.Fatalf("\n got %s\nwant %s", raw, want)
	}
	// Wildcards from the user are literal.
	if _, _, raw := get(t, mux, crm, "/api/desktop/search?q="+tag[:3]+"%25"); strings.Contains(raw, id) {
		t.Fatalf("%% matched as a wildcard: %s", raw)
	}
	if _, _, raw := get(t, mux, crm, "/api/desktop/search?q=k"); raw != `{"success":true,"data":{"groups":[],"query":"k"}}` {
		t.Fatalf("short query: %s", raw)
	}
}

func TestDesktopPreferences(t *testing.T) {
	_, mux := setup(t)
	staff := testutil.CreateStaff(t, testutil.StaffOptions{})
	if _, _, raw := get(t, mux, staff, "/api/desktop/preferences"); raw != `{"success":true,"data":null}` {
		t.Fatalf("unsaved: %s", raw)
	}
	put := func(body any) string {
		rec, _ := testutil.Do(t, mux, testutil.AsStaff(testutil.Request("PUT", "/api/desktop/preferences", body), staff))
		return rec.Body.String()
	}
	got := put(map[string]any{"wallpaper": "wp-1", "widgetVisibility": map[string]any{"omzet": true, "hantu": true},
		"widgetOrder": []any{"stok", "stok", "omzet"}, "soundEnabled": true, "period": "mtd", "jahat": "x"})
	want := `{"success":true,"data":{"wallpaper":"wp-1","widgetVisibility":{"omzet":true},"widgetOrder":["stok","omzet"],"soundEnabled":true,"period":"mtd"}}`
	if got != want {
		t.Fatalf("put: %s", got)
	}
	if _, _, raw := get(t, mux, staff, "/api/desktop/preferences"); raw != want {
		t.Fatalf("get after put: %s", raw)
	}
	if got := put("not json"); got != `{"success":true,"data":{"wallpaper":null,"widgetVisibility":{},"widgetOrder":[],"soundEnabled":false,"period":null}}` {
		t.Fatalf("garbage body saves defaults: %s", got)
	}
}

// The stream answers with SSE headers, a ready event, then the board.
func TestDesktopStream(t *testing.T) {
	deps := testutil.Deps(t, nil)
	srv := httptest.NewServer(testutil.Mux(desktop.New(deps)))
	defer srv.Close()
	owner := testutil.CreateStaff(t, testutil.StaffOptions{Role: "admin", Menus: map[string][]string{"dashboard": nil}})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/api/desktop/stream", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: owner.Token})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	for k, v := range map[string]string{"Content-Type": "text/event-stream; charset=utf-8", "Cache-Control": "no-cache, no-transform",
		"X-Accel-Buffering": "no"} {
		if res.Header.Get(k) != v {
			t.Errorf("%s = %q", k, res.Header.Get(k))
		}
	}
	lines := bufio.NewReader(res.Body)
	event := func() (string, map[string]any) {
		t.Helper()
		var name, data string
		for {
			line, err := lines.ReadString('\n')
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			switch line = strings.TrimSuffix(line, "\n"); {
			case strings.HasPrefix(line, "event: "):
				name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				data = strings.TrimPrefix(line, "data: ")
			case line == "" && name != "":
				var v map[string]any
				if err := json.Unmarshal([]byte(data), &v); err != nil {
					t.Fatalf("%s data %q: %v", name, data, err)
				}
				return name, v
			}
		}
	}
	if name, data := event(); name != "ready" || data["at"] == nil {
		t.Fatalf("first event %s %v", name, data)
	}
	name, data := event()
	if name != "overview" || len(data["hash"].(string)) != 40 {
		t.Fatalf("second event %s %v", name, data)
	}
	if ov := data["overview"].(map[string]any); ov["dibuatPada"] == nil || ov["periode"].(map[string]any)["periode"].(map[string]any)["kind"] != "today" {
		t.Fatalf("overview payload %v", ov)
	}
}

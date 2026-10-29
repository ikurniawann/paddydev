package hris

import (
	"slices"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

const delegatesURL = "/api/hris/schedule-delegates"

// delegateHR is an HR account that manages placements (all actions).
func delegateHR(h *harness) testutil.Staff {
	s, _ := h.staff("hrd", []string{"hris.master.placements"}, true, emp{name: "HR Pengatur"})
	return s
}

func (h *harness) auditCount(action, employeeID string) string {
	return h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE entity = 'hris.employees'
		AND entity_id = $1 AND action = $2`, employeeID, action)
}

// placeExtra adds a running non-primary placement.
func (h *harness) placeExtra(employeeID, branchID string) {
	h.t.Helper()
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date, end_date)
		VALUES ($1, $2, false, '2026-01-01', NULL)`, employeeID, branchID)
}

func branchIDsOf(row map[string]any) []string {
	out := []string{}
	for _, b := range row["branches"].([]any) {
		out = append(out, b.(map[string]any)["id"].(string))
	}
	return out
}

func TestDelegatesAdminSetListRevoke(t *testing.T) {
	h := newHarness(t)
	hr := delegateHR(h)
	p, pid := h.staff("pos", nil, true, emp{name: "Del Adm Pengatur"})
	pos := h.scalar(`INSERT INTO hris.positions (title, department) VALUES ('Shopkeeper Coordinator', 'Ops') RETURNING id::text`)
	h.exec(`UPDATE hris.employees SET job_title_id = $2 WHERE id = $1`, pid, pos)
	h.employee(emp{name: "Del Adm Anak 1", boss: &pid})
	h.employee(emp{name: "Del Adm Anak 2", boss: &pid})
	h.employee(emp{name: "Del Adm Anak Nonaktif", boss: &pid, inactive: true})
	a, b := h.branch("Del Adm A"), h.branch("Del Adm B")
	url := delegatesURL + "/" + pid

	// Not a delegate yet.
	expect(t, h.do(&p, "GET", scheduleAccessURL, nil), 200, "")
	c := h.do(&hr, "GET", delegatesURL, nil)
	expect(t, c, 200, "")
	for _, r := range c.list() {
		if r.(map[string]any)["employee_id"] == pid {
			t.Fatalf("list must not hold %s yet: %s", pid, c.raw)
		}
	}

	set := h.do(&hr, "PUT", url, map[string]any{"branch_ids": []string{a, b}, "can_place": true})
	expect(t, set, 200, "")
	row := set.data()
	if row["employee_id"] != pid || row["full_name"] != "Del Adm Pengatur" || row["position_title"] != "Shopkeeper Coordinator" ||
		row["has_login"] != true || row["can_place"] != true || row["direct_reports"] != float64(2) ||
		row["is_active"] != true || row["updated_by_name"] == nil || row["updated_at"] == nil || row["nip"] == nil {
		t.Fatalf("set = %s", set.raw)
	}
	if got := branchIDsOf(row); len(got) != 2 || !slices.Contains(got, a) || !slices.Contains(got, b) {
		t.Fatalf("branches = %v", got)
	}
	if h.do(&p, "GET", scheduleAccessURL, nil).data()["is_delegate"] != true {
		t.Fatal("the appointed account must be a delegate")
	}

	list := h.do(&hr, "GET", delegatesURL, nil)
	expect(t, list, 200, "")
	var found map[string]any
	for _, r := range list.list() {
		if m := r.(map[string]any); m["employee_id"] == pid {
			found = m
		}
	}
	if found == nil || len(found["branches"].([]any)) != 2 {
		t.Fatalf("list = %s", list.raw)
	}
	if h.scalar(`SELECT created_by::text FROM hris.schedule_delegates WHERE employee_id = $1`, pid) != hr.UserID {
		t.Fatal("created_by must be the HR account")
	}

	// Replace the branch list.
	again := h.do(&hr, "PUT", url, map[string]any{"branch_ids": []string{a}, "can_place": false})
	expect(t, again, 200, "")
	if got := branchIDsOf(again.data()); !slices.Equal(got, []string{a}) || again.data()["can_place"] != false {
		t.Fatalf("again = %s", again.raw)
	}
	if h.auditCount("schedule_delegate.set", pid) != "2" {
		t.Fatal("two set audit rows expected")
	}
	// First audit row has a null before; second carries the first state.
	if h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE entity_id = $1 AND action = 'schedule_delegate.set'
		AND before IS NULL`, pid) != "1" {
		t.Fatal("only the first set has a null before")
	}
	// (The fixture transaction gives every row the same created_at, so the
	// latest row is the one that carries a before.)
	if h.scalar(`SELECT after->>'can_place' FROM audit.audit_log WHERE entity_id = $1 AND action = 'schedule_delegate.set'
		AND before IS NOT NULL`, pid) != "false" {
		t.Fatal("latest audit after must hold can_place=false")
	}
	if h.scalar(`SELECT jsonb_array_length(before->'branch_ids')::text FROM audit.audit_log WHERE entity_id = $1
		AND action = 'schedule_delegate.set' AND before IS NOT NULL`, pid) != "2" {
		t.Fatal("second audit before must list both stores")
	}
	if h.scalar(`SELECT count(*)::text FROM hris.schedule_delegate_branches WHERE employee_id = $1`, pid) != "1" {
		t.Fatal("branch rows must be replaced")
	}

	// Revoke.
	del := h.do(&hr, "DELETE", url, nil)
	expect(t, del, 200, "")
	if del.data()["revoked"] != true {
		t.Fatalf("revoke = %s", del.raw)
	}
	for _, r := range h.do(&hr, "GET", delegatesURL, nil).list() {
		if r.(map[string]any)["employee_id"] == pid {
			t.Fatal("revoked delegate still listed")
		}
	}
	if h.auditCount("schedule_delegate.revoke", pid) != "1" {
		t.Fatal("revoke audit expected")
	}
	if h.do(&p, "GET", scheduleAccessURL, nil).data()["is_delegate"] != false {
		t.Fatal("revoked account must not be a delegate")
	}
	expect(t, h.do(&hr, "DELETE", url, nil), 404, "")
}

func TestDelegatesListOrderAndInactive(t *testing.T) {
	h := newHarness(t)
	hr := delegateHR(h)
	_, z := h.staff("pos", nil, true, emp{name: "Zzz Del Urut"})
	_, a := h.staff("pos", nil, true, emp{name: "Aaa Del Urut"})
	br := h.branch("Del Urut Toko")
	h.delegate(z, false, br)
	h.delegate(a, true, br)
	h.exec(`UPDATE hris.employees SET is_active = false WHERE id = $1`, a)
	c := h.do(&hr, "GET", delegatesURL, nil)
	expect(t, c, 200, "")
	var names []string
	var inactive any
	for _, r := range c.list() {
		m := r.(map[string]any)
		if m["employee_id"] == a || m["employee_id"] == z {
			names = append(names, m["full_name"].(string))
		}
		if m["employee_id"] == a {
			inactive = m["is_active"]
			if m["updated_by_name"] != nil {
				t.Fatalf("unknown updater must be null: %v", m)
			}
		}
	}
	if !slices.Equal(names, []string{"Aaa Del Urut", "Zzz Del Urut"}) || inactive != false {
		t.Fatalf("names %v inactive %v (%s)", names, inactive, c.raw)
	}
}

func TestDelegatesAdminValidation(t *testing.T) {
	h := newHarness(t)
	hr := delegateHR(h)
	_, ok := h.staff("pos", nil, true, emp{name: "Del Val OK"})
	_, off := h.staff("pos", nil, true, emp{name: "Del Val Off", inactive: true})
	noLogin := h.employee(emp{name: "Del Val Tanpa Login"})
	br := h.branch("Del Val Toko")
	dead := h.branch("Del Val Mati")
	h.exec(`UPDATE configuration.branches SET is_active = false WHERE id = $1`, dead)
	body := func(ids ...string) map[string]any { return map[string]any{"branch_ids": ids, "can_place": false} }
	put := func(id string, b map[string]any) call { return h.do(&hr, "PUT", delegatesURL+"/"+id, b) }

	expect(t, put("bukan-uuid", body(br)), 400, "ID karyawan tidak valid")
	expect(t, h.do(&hr, "DELETE", delegatesURL+"/bukan-uuid", nil), 400, "ID karyawan tidak valid")
	expect(t, put("00000000-0000-4000-8000-000000000001", body(br)), 404, "Karyawan tidak ditemukan")
	expect(t, put(off, body(br)), 400, "Karyawan tidak aktif")
	expect(t, put(noLogin, body(br)), 400, "Karyawan belum punya akun login")
	expect(t, put(ok, body()), 400, "Pilih minimal satu toko")
	expect(t, put(ok, map[string]any{"can_place": true}), 400, "Pilih minimal satu toko")
	expect(t, put(ok, body(dead)), 400, "Toko tidak valid")
	expect(t, put(ok, body(br, "00000000-0000-4000-8000-000000000002")), 400, "Toko tidak valid")
	if c := put(ok, body("bukan-uuid")); c.status != 400 {
		t.Fatalf("bad branch uuid: %s", c.raw)
	}
	tooMany := make([]string, 101)
	for i := range tooMany {
		tooMany[i] = br
	}
	if c := put(ok, body(tooMany...)); c.status != 400 {
		t.Fatalf("over 100: %s", c.raw)
	}
	// Nothing was stored and nothing audited by the failures.
	if h.scalar(`SELECT count(*)::text FROM hris.schedule_delegates WHERE employee_id = $1`, ok) != "0" {
		t.Fatal("failed PUTs must not store")
	}
	if h.auditCount("schedule_delegate.set", ok) != "0" {
		t.Fatal("failed PUTs must not audit")
	}
	// Duplicate branch ids collapse.
	c := put(ok, body(br, br))
	expect(t, c, 200, "")
	if len(c.data()["branches"].([]any)) != 1 {
		t.Fatalf("dup = %s", c.raw)
	}
	// An already-stored delegate whose employee is later inactive can still be revoked.
	h.exec(`UPDATE hris.employees SET is_active = false WHERE id = $1`, ok)
	expect(t, h.do(&hr, "DELETE", delegatesURL+"/"+ok, nil), 200, "")
}

func TestDelegatesAdminForbidden(t *testing.T) {
	h := newHarness(t)
	s, id := h.staff("pos", nil, true, emp{name: "Del Terlarang"})
	br := h.branch("Del Terlarang Toko")
	expect(t, h.do(&s, "GET", delegatesURL, nil), 403, "")
	expect(t, h.do(&s, "GET", delegatesURL+"/suggest?employee_id="+id, nil), 403, "")
	expect(t, h.do(&s, "PUT", delegatesURL+"/"+id, map[string]any{"branch_ids": []string{br}, "can_place": true}), 403, "")
	expect(t, h.do(&s, "DELETE", delegatesURL+"/"+id, nil), 403, "")
	expect(t, h.do(nil, "GET", delegatesURL, nil), 401, "")

	// Read-only HR may list and suggest but not save or revoke.
	ro := h.staffActions("hrd", map[string][]string{"hris.master.placements": {"read"}})
	expect(t, h.do(&ro, "GET", delegatesURL, nil), 200, "")
	expect(t, h.do(&ro, "PUT", delegatesURL+"/"+id, map[string]any{"branch_ids": []string{br}, "can_place": true}), 403, "")
	expect(t, h.do(&ro, "DELETE", delegatesURL+"/"+id, nil), 403, "")
}

func TestDelegatesSuggest(t *testing.T) {
	h := newHarness(t)
	hr := delegateHR(h)
	_, p := h.staff("pos", nil, true, emp{name: "Del Sug P"})
	tokoA, tokoC, tokoD, tokoOff := h.branch("Del Sug A"), h.branch("Del Sug C"), h.branch("Del Sug D"), h.branch("Del Sug Off")
	h.exec(`UPDATE configuration.branches SET is_active = false WHERE id = $1`, tokoOff)
	h.place(p, tokoA)
	h.placeExtra(p, tokoOff)
	z := h.employee(emp{name: "Del Sug Z", boss: &p})
	y := h.employee(emp{name: "Del Sug Y", boss: &z})
	h.place(y, tokoC)
	h.placeExtra(y, tokoA)
	gone := h.employee(emp{name: "Del Sug Gone", boss: &p, inactive: true})
	h.place(gone, tokoD)
	w := h.employee(emp{name: "Del Sug W"})
	h.place(w, tokoD)
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date, end_date)
		VALUES ($1, $2, false, '2026-01-01', '2026-09-30')`, z, tokoD)

	c := h.do(&hr, "GET", delegatesURL+"/suggest?employee_id="+p, nil)
	expect(t, c, 200, "")
	got := []string{}
	for _, id := range c.data()["branch_ids"].([]any) {
		got = append(got, id.(string))
	}
	slices.Sort(got)
	want := []string{tokoA, tokoC}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("suggest = %v, want %v", got, want)
	}

	// An employee with nothing yields an empty list, never null.
	lone := h.employee(emp{name: "Del Sug Lone"})
	c = h.do(&hr, "GET", delegatesURL+"/suggest?employee_id="+lone, nil)
	expect(t, c, 200, "")
	if l, ok := c.data()["branch_ids"].([]any); !ok || len(l) != 0 {
		t.Fatalf("lone = %s", c.raw)
	}
	expect(t, h.do(&hr, "GET", delegatesURL+"/suggest", nil), 400, "ID karyawan tidak valid")
	expect(t, h.do(&hr, "GET", delegatesURL+"/suggest?employee_id=xx", nil), 400, "ID karyawan tidak valid")
	expect(t, h.do(&hr, "GET", delegatesURL+"/suggest?employee_id=00000000-0000-4000-8000-000000000001", nil), 404, "Karyawan tidak ditemukan")
}

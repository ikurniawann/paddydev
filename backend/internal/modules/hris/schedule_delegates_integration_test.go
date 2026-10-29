package hris

import (
	"context"
	"slices"
	"testing"
)

const scheduleAccessURL = "/api/hris/me/schedule-access"

// delegate appoints employeeID as store schedule delegate over branches
// (EPIC-071 R1/R2).
func (h *harness) delegate(employeeID string, canPlace bool, branches ...string) {
	h.t.Helper()
	h.exec(`INSERT INTO hris.schedule_delegates (employee_id, can_place) VALUES ($1, $2)`, employeeID, canPlace)
	for _, b := range branches {
		h.exec(`INSERT INTO hris.schedule_delegate_branches (employee_id, branch_id) VALUES ($1, $2)`, employeeID, b)
	}
}

func (h *harness) place(employeeID, branchID string) {
	h.t.Helper()
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date, end_date)
		VALUES ($1, $2, true, '2026-01-01', NULL)`, employeeID, branchID)
}

func TestScheduleAccessNonDelegate(t *testing.T) {
	h := newHarness(t)
	s, _ := h.staff("pos", nil, true)
	c := h.do(&s, "GET", scheduleAccessURL, nil)
	expect(t, c, 200, "")
	d := c.data()
	if d["is_delegate"] != false || d["can_place"] != false {
		t.Fatalf("data = %s", c.raw)
	}
	if b, ok := d["branches"].([]any); !ok || len(b) != 0 {
		t.Fatalf("branches = %s", c.raw)
	}
}

func TestScheduleAccessUnlinkedAccount(t *testing.T) {
	h := newHarness(t)
	s, _ := h.staff("pos", nil, false)
	c := h.do(&s, "GET", scheduleAccessURL, nil)
	expect(t, c, 200, "")
	if c.data()["is_delegate"] != false {
		t.Fatalf("data = %s", c.raw)
	}
}

func TestScheduleAccessDelegate(t *testing.T) {
	h := newHarness(t)
	s, id := h.staff("pos", nil, true)
	tokoB := h.branch("Del Toko B")
	tokoA := h.branch("Del Toko A")
	h.exec(`UPDATE configuration.branches SET is_active = false WHERE id = $1`, tokoB)
	h.delegate(id, true, tokoA, tokoB)

	c := h.do(&s, "GET", scheduleAccessURL, nil)
	expect(t, c, 200, "")
	d := c.data()
	if d["is_delegate"] != true || d["can_place"] != true {
		t.Fatalf("data = %s", c.raw)
	}
	br := d["branches"].([]any)
	if len(br) != 1 {
		t.Fatalf("branches = %s", c.raw)
	}
	b := br[0].(map[string]any)
	if b["id"] != tokoA || b["name"] != "Del Toko A" || b["code"] == nil || b["code"] == "" {
		t.Fatalf("branch = %v", b)
	}
}

func TestScheduleAccessBranchesSortedByCode(t *testing.T) {
	h := newHarness(t)
	s, id := h.staff("pos", nil, true)
	x, y := h.branch("Del Urut X"), h.branch("Del Urut Y")
	h.exec(`UPDATE configuration.branches SET code = 'ZZ-9' WHERE id = $1`, x)
	h.exec(`UPDATE configuration.branches SET code = 'AA-1' WHERE id = $1`, y)
	h.delegate(id, false, x, y)
	c := h.do(&s, "GET", scheduleAccessURL, nil)
	expect(t, c, 200, "")
	br := c.data()["branches"].([]any)
	if len(br) != 2 || br[0].(map[string]any)["id"] != y || br[1].(map[string]any)["id"] != x || c.data()["can_place"] != false {
		t.Fatalf("data = %s", c.raw)
	}
}

func TestScheduleAccessNoActiveBranch(t *testing.T) {
	h := newHarness(t)
	s, id := h.staff("pos", nil, true)
	tokoA := h.branch("Del Mati")
	h.exec(`UPDATE configuration.branches SET is_active = false WHERE id = $1`, tokoA)
	h.delegate(id, true, tokoA)
	c := h.do(&s, "GET", scheduleAccessURL, nil)
	expect(t, c, 200, "")
	if c.data()["is_delegate"] != false {
		t.Fatalf("data = %s", c.raw)
	}
}

func TestScheduleAccessInactiveEmployee(t *testing.T) {
	h := newHarness(t)
	s, id := h.staff("pos", nil, true)
	h.delegate(id, true, h.branch("Del Toko Nonaktif"))
	h.exec(`UPDATE hris.employees SET is_active = false WHERE id = $1`, id)
	c := h.do(&s, "GET", scheduleAccessURL, nil)
	expect(t, c, 200, "")
	if c.data()["is_delegate"] != false {
		t.Fatalf("data = %s", c.raw)
	}
}

func TestScheduleAccessUnauthenticated(t *testing.T) {
	h := newHarness(t)
	expect(t, h.do(nil, "GET", scheduleAccessURL, nil), 401, "")
}

func TestStoreScopeAllowsBranch(t *testing.T) {
	hr := storeScope{}
	if !hr.AllowsBranch("any") || !hr.AllowsBranch("") {
		t.Fatal("HR scope must allow every branch")
	}
	d := storeScope{Delegate: &delegateAccess{BranchIDs: []string{"a", "b"}}}
	if !d.AllowsBranch("a") || !d.AllowsBranch("b") {
		t.Fatal("delegate must allow own branches")
	}
	if d.AllowsBranch("c") || d.AllowsBranch("") {
		t.Fatal("delegate must reject other and empty branches")
	}
}

func TestLoadDelegateAccessNil(t *testing.T) {
	h := newHarness(t)
	s, _ := h.staff("pos", nil, true)
	got, err := loadDelegateAccess(context.Background(), h.tx, s.UserID)
	if err != nil || got != nil {
		t.Fatalf("got %v, %v; want nil, nil", got, err)
	}
}

func TestDelegateOutside(t *testing.T) {
	h := newHarness(t)
	_, p := h.staff("pos", nil, true, emp{name: "Del P"})
	tokoA, tokoC := h.branch("Del Out A"), h.branch("Del Out C")
	h.delegate(p, true, tokoA)

	x := h.employee(emp{name: "Del X"})
	h.place(x, tokoA)
	z := h.employee(emp{name: "Del Z", boss: &p})
	y := h.employee(emp{name: "Del Y", boss: &z})
	h.place(y, tokoC)
	w := h.employee(emp{name: "Del W"})
	h.place(w, tokoC)
	u := h.employee(emp{name: "Del U", boss: &p})

	access := &delegateAccess{EmployeeID: p, BranchIDs: []string{tokoA}}
	out, err := delegateOutside(context.Background(), h.tx, access, "2026-10-05", []string{x, y, w, u})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(out, []string{w}) {
		t.Fatalf("outside = %v, want [%s]", out, w)
	}

	// Input order is kept; unknown and inactive ids count as outside; an
	// ended placement and a not-yet-started one do not count as in scope.
	gone := h.employee(emp{name: "Del Gone", inactive: true})
	h.place(gone, tokoA)
	ended := h.employee(emp{name: "Del Ended"})
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date, end_date)
		VALUES ($1, $2, true, '2026-01-01', '2026-09-30')`, ended, tokoA)
	future := h.employee(emp{name: "Del Future"})
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date, end_date)
		VALUES ($1, $2, true, '2026-11-01', NULL)`, future, tokoA)
	unknown := "00000000-0000-4000-8000-000000000001"
	out, err = delegateOutside(context.Background(), h.tx, access, "2026-10-05", []string{w, unknown, x, gone, ended, future, y})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{w, unknown, gone, ended, future}; !slices.Equal(out, want) {
		t.Fatalf("outside = %v, want %v", out, want)
	}

	// Empty input: nothing outside.
	out, err = delegateOutside(context.Background(), h.tx, access, "2026-10-05", nil)
	if err != nil || len(out) != 0 {
		t.Fatalf("empty = %v, %v", out, err)
	}
}

func TestDelegateOutsideInactiveSubordinate(t *testing.T) {
	h := newHarness(t)
	_, p := h.staff("pos", nil, true, emp{name: "Del P2"})
	tokoA := h.branch("Del In A")
	h.delegate(p, true, tokoA)
	q := h.employee(emp{name: "Del Q Aktif", boss: &p})
	r := h.employee(emp{name: "Del R Nonaktif", boss: &p, inactive: true})
	access := &delegateAccess{EmployeeID: p, BranchIDs: []string{tokoA}}
	out, err := delegateOutside(context.Background(), h.tx, access, "2026-10-05", []string{q, r})
	if err != nil || !slices.Equal(out, []string{r}) {
		t.Fatalf("outside = %v, %v; want [%s]", out, err, r)
	}
}

func TestDelegateSubtreeCycle(t *testing.T) {
	h := newHarness(t)
	_, p := h.staff("pos", nil, true, emp{name: "Del Siklus P"})
	q := h.employee(emp{name: "Del Siklus Q", boss: &p})
	h.exec(`UPDATE hris.employees SET reporting_to = $2 WHERE id = $1`, p, q)
	h.delegate(p, true, h.branch("Del Siklus A"))
	access := &delegateAccess{EmployeeID: p}
	out, err := delegateOutside(context.Background(), h.tx, access, "2026-10-05", []string{q})
	if err != nil {
		t.Fatal(err)
	}
	// Q is in the subtree; the cycle back to P neither loops nor errors.
	if len(out) != 0 {
		t.Fatalf("outside = %v, want none", out)
	}
}

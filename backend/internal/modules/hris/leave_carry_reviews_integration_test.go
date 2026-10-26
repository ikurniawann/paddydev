package hris

import (
	"strings"
	"testing"
)

// EPIC-068 Task 3: GET /api/hris/leave-balances/reviews and
// POST /api/hris/leave-balances/reviews/{id}/resolve (R5).

// reviewFixture is a carry fixture with 2027 pushed below used: one open review.
func (h *harness) reviewFixture() (leaveFixture, string) {
	h.t.Helper()
	f := h.carryFixture(nil)
	editor, _ := h.saldoStaff("pos", false, "read", "update")
	h.belowUsedRow(f, &editor) // 2027: carry 2, used 5, excess 3
	id := h.scalar(`SELECT id::text FROM hris.leave_carry_over_reviews WHERE employee_id = $1 AND year = 2027 AND resolved_at IS NULL`, f.id)
	if id == "" {
		h.t.Fatal("no open review in the fixture")
	}
	return f, id
}

func TestLeaveCarryReviewsList(t *testing.T) {
	h := newHarness(t)
	f, id := h.reviewFixture()
	pvj := h.branch("Toko Tinjauan")
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date) VALUES ($1, $2, true, '2026-01-01')`, f.id, pvj)
	reader, _ := h.saldoStaff("pos", false, "read")
	none, _ := h.saldoStaff("pos", false)
	path := "/api/hris/leave-balances/reviews?year=2027"

	expect(t, h.do(nil, "GET", path, nil), 401, "")
	expect(t, h.do(&none, "GET", path, nil), 403, "Insufficient permissions")

	c := h.do(&reader, "GET", path, nil)
	expect(t, c, 200, "")
	var row map[string]any
	for _, it := range c.list() {
		if m, _ := it.(map[string]any); m["id"] == id {
			row = m
		}
	}
	if row == nil {
		t.Fatalf("review %s missing from %s", id, c.raw)
	}
	if row["employee_id"] != f.id || row["year"] != float64(2027) || row["carry_over_days"] != float64(2) ||
		row["carry_over_used"] != float64(5) || row["excess"] != float64(3) || row["branch_name"] != "Toko Tinjauan" ||
		row["nip"] == nil || row["full_name"] == nil || row["opened_at"] == nil || row["resolved_at"] != nil || row["resolution"] != nil {
		t.Fatalf("row = %v", row)
	}
	cause, _ := row["cause"].(map[string]any)
	if cause["type"] != "hr_update" {
		t.Fatalf("cause = %v", row["cause"])
	}

	// Year filter: another year has none of this employee's review.
	for _, it := range h.do(&reader, "GET", "/api/hris/leave-balances/reviews?year=2026", nil).list() {
		if m, _ := it.(map[string]any); m["id"] == id {
			t.Fatal("2026 list holds the 2027 review")
		}
	}
	expect(t, h.do(&reader, "GET", "/api/hris/leave-balances/reviews?year=abc", nil), 400, "")
	expect(t, h.do(&reader, "GET", path+"&status=bogus", nil), 400, "")

	// A resolved review leaves the default list and shows with status=all.
	h.exec(`UPDATE hris.leave_carry_over_reviews SET resolved_at = now(), resolved_by_name = 'HRD', resolution = 'Sudah dibahas' WHERE id = $1`, id)
	for _, it := range h.do(&reader, "GET", path, nil).list() {
		if m, _ := it.(map[string]any); m["id"] == id {
			t.Fatal("resolved review in the open list")
		}
	}
	var found map[string]any
	for _, it := range h.do(&reader, "GET", path+"&status=all", nil).list() {
		if m, _ := it.(map[string]any); m["id"] == id {
			found = m
		}
	}
	if found == nil || found["resolution"] != "Sudah dibahas" || found["resolved_by_name"] != "HRD" || found["resolved_at"] == nil {
		t.Fatalf("status=all row = %v", found)
	}
}

// Without the review table (setting still locked) the list is empty, not a 500.
func TestLeaveCarryReviewsListWithoutTable(t *testing.T) {
	h := newHarness(t)
	h.exec(`DROP TABLE IF EXISTS hris.leave_carry_over_reviews`)
	reader, _ := h.saldoStaff("pos", false, "read")
	c := h.do(&reader, "GET", "/api/hris/leave-balances/reviews?year=2027", nil)
	expect(t, c, 200, "")
	if len(c.list()) != 0 {
		t.Fatalf("list = %s", c.raw)
	}
}

func TestLeaveCarryReviewResolve(t *testing.T) {
	h := newHarness(t)
	f, id := h.reviewFixture()
	reader, _ := h.saldoStaff("pos", false, "read")
	editor, _ := h.saldoStaff("pos", false, "read", "update")
	path := "/api/hris/leave-balances/reviews/" + id + "/resolve"
	body := map[string]any{"note": "  Dibahas dengan atasan, kelebihan dipotong gaji  "}

	expect(t, h.do(nil, "POST", path, body), 401, "")
	expect(t, h.do(&reader, "POST", path, body), 403, "Insufficient permissions")

	for _, bad := range []map[string]any{{}, {"note": ""}, {"note": "   "}, {"note": strings.Repeat("x", 501)}} {
		if c := h.do(&editor, "POST", path, bad); c.status != 400 {
			t.Fatalf("body %v: status %d, want 400 (%s)", bad, c.status, c.raw)
		}
	}
	expect(t, h.do(&editor, "POST", path, map[string]any{}), 400, "Catatan wajib diisi")
	// "otomatis" marks a review the system closed: HR may not write it (final review).
	for _, word := range []string{"otomatis", "  Otomatis ", "OTOMATIS"} {
		expect(t, h.do(&editor, "POST", path, map[string]any{"note": word}), 400, `Catatan tidak boleh "otomatis" — tulis alasan penyelesaian`)
	}
	expect(t, h.do(&editor, "POST", "/api/hris/leave-balances/reviews/bukan-uuid/resolve", body), 400, "")
	expect(t, h.do(&editor, "POST", "/api/hris/leave-balances/reviews/00000000-0000-0000-0000-000000000000/resolve", body), 404, "")
	if got := h.reviews(f.id, 2027); got != "1|0|3.00|-" {
		t.Fatalf("a refused resolve changed the review: %q", got)
	}
	if h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'leave_balance.review_resolve'`) != "0" {
		t.Fatal("a refused resolve wrote an audit")
	}

	c := h.do(&editor, "POST", path, body)
	expect(t, c, 200, "")
	if c.data()["id"] != id || c.data()["resolution"] != "Dibahas dengan atasan, kelebihan dipotong gaji" || c.data()["resolved_at"] == nil {
		t.Fatalf("response = %s", c.raw)
	}
	if got := h.scalar(`SELECT concat_ws('|', (resolved_at IS NOT NULL)::text, (resolved_by = $2)::text, resolved_by_name, resolution)
		FROM hris.leave_carry_over_reviews WHERE id = $1`, id, editor.UserID); !strings.HasPrefix(got, "true|true|") ||
		!strings.HasSuffix(got, "|Dibahas dengan atasan, kelebihan dipotong gaji") {
		t.Fatalf("stored = %q", got)
	}
	// The balance itself is untouched.
	if got := h.carryOf(f.id, 2027); got != "2.00|5.00|2027-03-31" {
		t.Fatalf("carry = %q", got)
	}

	// Audit: entity employee, before/after, the note as reason.
	a := h.scalar(`SELECT concat_ws('|', entity, entity_id, reason, (actor_id = $1)::text, before->>'resolved', after->>'resolved',
			after->>'excess', after->>'year')
		FROM audit.audit_log WHERE action = 'leave_balance.review_resolve'`, editor.UserID)
	if want := "hris.employees|" + f.id + "|Dibahas dengan atasan, kelebihan dipotong gaji|true|false|true|3|2027"; a != want {
		t.Fatalf("audit = %q, want %q", a, want)
	}

	// A second resolve is refused and writes nothing.
	expect(t, h.do(&editor, "POST", path, body), 409, "Tinjauan sudah diselesaikan")
	if h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'leave_balance.review_resolve'`) != "1" {
		t.Fatal("second resolve wrote an audit")
	}

	// The same shortfall does not reopen after HR resolved it (Task 1 rule).
	if got := h.reviews(f.id, 2027); got != "0|1|-|Dibahas dengan atasan, kelebihan dipotong gaji" {
		t.Fatalf("reviews = %q", got)
	}
}

// carry_over_source is exposed on every balance view (source column).
func TestLeaveBalanceViewCarrySource(t *testing.T) {
	h := newHarness(t)
	f := h.carryFixture(nil) // 2027 row is auto
	reader, _ := h.saldoStaff("pos", false, "read")
	find := func(c call, id string) map[string]any {
		for _, it := range c.list() {
			if m, _ := it.(map[string]any); m["employee_id"] == id {
				return m
			}
		}
		return nil
	}
	if r := find(h.do(&reader, "GET", "/api/hris/leave-balances?year=2027", nil), f.id); r == nil || r["carry_over_source"] != "auto" {
		t.Fatalf("auto row = %v", r)
	}
	h.exec(`UPDATE hris.leave_balances SET carry_over_source = 'manual' WHERE employee_id = $1 AND year = 2027`, f.id)
	if r := find(h.do(&reader, "GET", "/api/hris/leave-balances?year=2027", nil), f.id); r == nil || r["carry_over_source"] != "manual" {
		t.Fatalf("manual row = %v", r)
	}
	one := h.do(&reader, "GET", "/api/hris/leave-balances/"+f.id+"?year=2027", nil)
	if one.data()["carry_over_source"] != "manual" {
		t.Fatalf("single view = %s", one.raw)
	}
	// A row that does not exist yet is automatic.
	g := h.leaveFixture("2024-01-10")
	if r := find(h.do(&reader, "GET", "/api/hris/leave-balances?year=2031", nil), g.id); r == nil || r["carry_over_source"] != "auto" {
		t.Fatalf("missing row = %v", r)
	}
}

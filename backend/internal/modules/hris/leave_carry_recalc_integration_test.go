package hris

import (
	"context"
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

// EPIC-068 tindak lanjut Task 1: sisa bawaan dihitung ulang otomatis.
// fixedNow is 2026-10-05 WIB, so 2026 is the year being carried into 2027.

// carryFixture is an employee with 2026: total 12, used 4 (remaining 8) and
// the 1 January 2027 job already run with the setting on: the auto row of
// 2027 holds the whole remainder (8, expiring 2027-03-31) when max is nil.
func (h *harness) carryFixture(max any) leaveFixture {
	h.t.Helper()
	h.carrySetup(true, max, 3, 31)
	f := h.leaveFixture("2024-01-10")
	h.setBalance(f.id, 2026, 12, 0, 4, 0, 0, nil)
	if err := h.svc.RunLeaveCarryOver(context.Background(), jan1(2027)); err != nil {
		h.t.Fatal(err)
	}
	return f
}

// carryAudit reads the carry-over audit rows of employee whose cause has the
// type: cause|leave id|reason|has actor id|actor|before|after.
func (h *harness) carryAudit(employeeID, causeType string) string {
	h.t.Helper()
	return h.scalar(`SELECT concat_ws('|', after->'cause'->>'type', coalesce(after->'cause'->>'leave_id', '-'), reason,
			(actor_id IS NOT NULL)::text, actor_name, before->>'carry_over_days', after->>'carry_over_days', after->>'actor')
		FROM audit.audit_log WHERE action = 'leave_balance.carry_over' AND entity_id = $1
		  AND after->'cause'->>'type' = $2`, employeeID, causeType)
}

func (h *harness) carrySource(employeeID string, year int) string {
	h.t.Helper()
	return h.scalar(`SELECT carry_over_source FROM hris.leave_balances WHERE employee_id = $1 AND year = $2`, employeeID, year)
}

func (h *harness) reviews(employeeID string, year int) string {
	h.t.Helper()
	return h.scalar(`SELECT concat_ws('|', count(*) FILTER (WHERE resolved_at IS NULL),
			count(*) FILTER (WHERE resolved_at IS NOT NULL),
			coalesce(max(excess) FILTER (WHERE resolved_at IS NULL)::text, '-'),
			coalesce(max(resolution) FILTER (WHERE resolved_at IS NOT NULL), '-'))
		FROM hris.leave_carry_over_reviews WHERE employee_id = $1 AND year = $2`, employeeID, year)
}

func (h *harness) putSaldo(editor *testutil.Staff, employeeID string, body map[string]any) call {
	h.t.Helper()
	return h.do(editor, "PUT", "/api/hris/leave-balances/"+employeeID, body)
}

// Wajib dari user: a December leave approved after 1 January lowers the carry;
// cancelling raises it again; rejecting changes nothing (no balance moved).
func TestCarryRecalcApproveCancelReject(t *testing.T) {
	h := newHarness(t)
	f := h.carryFixture(nil)
	if got := h.carryOf(f.id, 2027); got != "8.00|0.00|2027-03-31" {
		t.Fatalf("carry after the job = %q", got)
	}
	if h.carrySource(f.id, 2027) != "auto" {
		t.Fatal("the job's row must be auto")
	}

	leave := h.request(f, "2026-12-07", "2026-12-08")
	expect(t, leave, 200, "")
	id := leave.data()["id"].(string)
	expect(t, h.approve(f, id), 200, "")
	if got := h.carryOf(f.id, 2027); got != "6.00|0.00|2027-03-31" {
		t.Fatalf("carry after approval = %q, want 6", got)
	}
	// Audit: cause, leave id, readable reason and the signed-in approver.
	got := h.carryAudit(f.id, "leave_approve")
	if !strings.HasPrefix(got, "leave_approve|"+id+"|Cuti "+id+" disetujui|true|") || !strings.Contains(got, "|8|6|") {
		t.Fatalf("approve audit = %q", got)
	}
	if strings.Contains(got, "Sistem") {
		t.Fatalf("approve audit names the system, not the approver: %q", got)
	}

	expect(t, h.do(&f.hr, "PUT", "/api/hris/leaves/"+id, map[string]any{"status": "cancelled"}), 200, "")
	if got := h.carryOf(f.id, 2027); got != "8.00|0.00|2027-03-31" {
		t.Fatalf("carry after cancel = %q, want 8", got)
	}
	got = h.carryAudit(f.id, "leave_cancel")
	if !strings.HasPrefix(got, "leave_cancel|"+id+"|Cuti "+id+" dibatalkan|true|") || !strings.Contains(got, "|6|8|") {
		t.Fatalf("cancel audit = %q", got)
	}

	// A rejection moves no balance: carry and audit stay as they are.
	before := h.carryAudits(f.id)
	other := h.request(f, "2026-12-14", "2026-12-15")
	expect(t, other, 200, "")
	expect(t, h.do(&f.boss, "POST", "/api/hris/leaves/approve", map[string]any{
		"leave_id": other.data()["id"], "action": "reject", "rejection_reason": "Padat"}), 200, "")
	if got := h.carryOf(f.id, 2027); got != "8.00|0.00|2027-03-31" {
		t.Fatalf("carry after reject = %q", got)
	}
	if h.carryAudits(f.id) != before {
		t.Fatal("a rejection wrote a carry-over audit")
	}
}

// Wajib dari user: HR correcting last year's allocation by hand moves the
// carry; so does an import. A figure of 2027 itself never recalculates 2027.
func TestCarryRecalcHRUpdate(t *testing.T) {
	h := newHarness(t)
	f := h.carryFixture(nil)
	editor, _ := h.saldoStaff("pos", false, "read", "update")

	expect(t, h.putSaldo(&editor, f.id, map[string]any{"year": 2026, "annual_leave_adjustment": 2, "reason": "Koreksi jatah"}), 200, "")
	if got := h.carryOf(f.id, 2027); got != "10.00|0.00|2027-03-31" {
		t.Fatalf("carry after HR correction = %q, want 10", got)
	}
	got := h.carryAudit(f.id, "hr_update")
	if !strings.HasPrefix(got, "hr_update|-|Koreksi HR|true|") || !strings.Contains(got, "|8|10|") {
		t.Fatalf("hr audit = %q", got)
	}

	// Changing 2027's own total leaves its carry alone.
	expect(t, h.putSaldo(&editor, f.id, map[string]any{"year": 2027, "annual_leave_total": 13, "reason": "Jatah 2027"}), 200, "")
	if got := h.carryOf(f.id, 2027); got != "10.00|0.00|2027-03-31" {
		t.Fatalf("carry after editing 2027 = %q", got)
	}
	if h.carrySource(f.id, 2027) != "auto" {
		t.Fatal("editing the total must not turn the carry manual")
	}
}

func TestCarryRecalcImport(t *testing.T) {
	h := newHarness(t)
	f := h.saldoImportFixture()
	h.carrySetup(true, nil, 3, 31)
	// Ana: 2026 remaining 7; an auto row for 2027 that the formula fills.
	h.setBalance(f.ana, 2027, 12, 0, 0, 0, 0, nil)
	changed, err := recomputeCarryOver(context.Background(), h.tx, f.ana, 2027, carryCause{Type: "settings"}, nil)
	if err != nil || !changed {
		t.Fatalf("prime: changed %v err %v", changed, err)
	}
	if got := h.carryOf(f.ana, 2027); got != "7.00|0.00|2027-03-31" {
		t.Fatalf("primed carry = %q", got)
	}

	_, x := h.saldoTemplate(&f.hr, "?year=2026&branch_id="+f.branch)
	ar, br := rowOf(t, x, f.anaNIP), rowOf(t, x, f.budiNIP)
	setCell(t, x, colTotal, ar, 14)
	setCell(t, x, colReason, ar, "Koreksi jatah")
	setCell(t, x, colTotal, br, "10,5")
	setCell(t, x, colReason, br, "Jatah khusus")
	file := fileBytes(t, x)
	pre := h.importStep(t, f, "preview", nil, file)
	expect(t, pre, 200, "")
	expect(t, h.importStep(t, f, "commit", map[string]string{"fingerprint": pre.data()["fingerprint"].(string)}, file), 200, "")

	if got := h.carryOf(f.ana, 2027); got != "9.00|0.00|2027-03-31" {
		t.Fatalf("carry after import = %q, want 9", got)
	}
	got := h.carryAudit(f.ana, "import")
	if !strings.HasPrefix(got, "import|-|Import massal|true|") || !strings.Contains(got, "|7|9|") {
		t.Fatalf("import audit = %q", got)
	}
	// Budi had no 2027 row: the import does not create one.
	if h.scalar(`SELECT count(*)::text FROM hris.leave_balances WHERE employee_id = $1 AND year = 2027`, f.budi) != "0" {
		t.Fatal("the import created a 2027 row")
	}
}

// Wajib dari user: a correction that pushes the carry below what is used is
// stored as is, never negative on screen, opens one review, and the review
// closes by itself once a correction undoes the excess.
func TestCarryRecalcBelowUsedOpensAndClosesReview(t *testing.T) {
	h := newHarness(t)
	f := h.carryFixture(nil)
	editor, _ := h.saldoStaff("pos", false, "read", "update")
	h.exec(`UPDATE hris.leave_balances SET carry_over_used = 5 WHERE employee_id = $1 AND year = 2027`, f.id)

	expect(t, h.putSaldo(&editor, f.id, map[string]any{"year": 2026, "annual_leave_adjustment": -6, "reason": "Koreksi turun"}), 200, "")
	if got := h.carryOf(f.id, 2027); got != "2.00|5.00|2027-03-31" {
		t.Fatalf("carry below used = %q, want stored as is", got)
	}
	if got := h.reviews(f.id, 2027); got != "1|0|3.00|-" {
		t.Fatalf("reviews = %q, want one open with excess 3", got)
	}
	view := h.do(&editor, "GET", "/api/hris/leave-balances/"+f.id+"?year=2027", nil)
	expect(t, view, 200, "")
	if d := view.data(); d["carry_over_remaining"] != 0.0 || d["available"].(float64) < 0 {
		t.Fatalf("view = %v: negative carry on screen", d)
	}

	// Lower again: still one review, updated excess.
	expect(t, h.putSaldo(&editor, f.id, map[string]any{"year": 2026, "annual_leave_adjustment": -7, "reason": "Turun lagi"}), 200, "")
	if got := h.carryOf(f.id, 2027); got != "1.00|5.00|2027-03-31" {
		t.Fatalf("carry = %q", got)
	}
	if got := h.reviews(f.id, 2027); got != "1|0|4.00|-" {
		t.Fatalf("reviews after second drop = %q, want still one open, excess 4", got)
	}

	// Undo the correction: carry 8 >= used 5, the review closes by itself.
	expect(t, h.putSaldo(&editor, f.id, map[string]any{"year": 2026, "annual_leave_adjustment": 0, "reason": "Koreksi balik"}), 200, "")
	if got := h.carryOf(f.id, 2027); got != "8.00|5.00|2027-03-31" {
		t.Fatalf("carry = %q", got)
	}
	if got := h.reviews(f.id, 2027); got != "0|1|-|otomatis" {
		t.Fatalf("reviews after undo = %q, want closed 'otomatis'", got)
	}
}

// A leave consuming a carry already below used must not take negative days
// from it (the clamp in consumeAnnualLeave), and the view stays at zero.
func TestCarryRecalcBelowUsedConsumesNothingFromCarry(t *testing.T) {
	h := newHarness(t)
	f := h.carryFixture(nil)
	h.setBalance(f.id, 2027, 12, 0, 0, 0, 0, nil) // reset the row, then below-used state:
	h.exec(`UPDATE hris.leave_balances SET carry_over_days = 1, carry_over_used = 3, carry_over_expires_on = '2027-03-31',
		carry_over_source = 'auto' WHERE employee_id = $1 AND year = 2027`, f.id)
	leave := h.request(f, "2027-02-01", "2027-02-02")
	expect(t, leave, 200, "")
	expect(t, h.approve(f, leave.data()["id"].(string)), 200, "")
	if used, carry := h.balanceOf(f.id, 2027); used != "2.00" || carry != "3.00" {
		t.Fatalf("used %s carry used %s: the leave must come from the annual quota only", used, carry)
	}
}

// Wajib dari user: running the job twice (claim deleted in between) gives the
// same result and no second audit.
func TestCarryRecalcJobTwice(t *testing.T) {
	h := newHarness(t)
	f := h.carryFixture(5)
	if got := h.carryOf(f.id, 2027); got != "5.00|0.00|2027-03-31" {
		t.Fatalf("carry = %q", got)
	}
	audits := h.carryAudits(f.id)
	if got := h.carryAudit(f.id, "job"); !strings.HasPrefix(got, "job|-|Job 1 Januari|false|Sistem|0|5|") {
		t.Fatalf("job audit = %q", got)
	}
	h.exec(`DELETE FROM hris.leave_carry_over_runs WHERE year = 2027`)
	if err := h.svc.RunLeaveCarryOver(context.Background(), jan1(2027)); err != nil {
		t.Fatal(err)
	}
	if got := h.carryOf(f.id, 2027); got != "5.00|0.00|2027-03-31" {
		t.Fatalf("carry after the second run = %q", got)
	}
	if h.carryAudits(f.id) != audits {
		t.Fatalf("audits %s -> %s: the second run audited an unchanged value", audits, h.carryAudits(f.id))
	}
}

// Review 1: with the setting off a December approval changes no carry and
// writes no carry-over audit; the job carries nothing.
func TestCarryRecalcSettingOff(t *testing.T) {
	h := newHarness(t)
	h.carrySetup(false, nil, 3, 31)
	f := h.leaveFixture("2024-01-10")
	h.setBalance(f.id, 2026, 12, 0, 4, 0, 0, nil)
	if err := h.svc.RunLeaveCarryOver(context.Background(), jan1(2027)); err != nil {
		t.Fatal(err)
	}
	if got := h.carryOf(f.id, 2027); got != "0.00|0.00|-" {
		t.Fatalf("carry with the setting off = %q", got)
	}
	leave := h.request(f, "2026-12-07", "2026-12-08")
	expect(t, leave, 200, "")
	expect(t, h.approve(f, leave.data()["id"].(string)), 200, "")
	if got := h.carryOf(f.id, 2027); got != "0.00|0.00|-" {
		t.Fatalf("carry after approval with the setting off = %q", got)
	}
	if h.carryAudits(f.id) != "0" {
		t.Fatal("carry-over audit written while the setting is off")
	}
	changed, err := recomputeCarryOver(context.Background(), h.tx, f.id, 2027, carryCause{Type: "settings"}, nil)
	if err != nil || changed {
		t.Fatalf("recompute with the setting off: changed %v err %v", changed, err)
	}
}

// Only active employees are recalculated.
func TestCarryRecalcInactiveEmployee(t *testing.T) {
	h := newHarness(t)
	h.carrySetup(true, nil, 3, 31)
	off := h.employee(emp{name: "Bawaan Resign", join: "2024-01-10", inactive: true})
	h.setBalance(off, 2026, 12, 0, 4, 0, 0, nil)
	h.setBalance(off, 2027, 12, 0, 0, 0, 0, nil)
	changed, err := recomputeCarryOver(context.Background(), h.tx, off, 2027, carryCause{Type: "settings"}, nil)
	if err != nil || changed {
		t.Fatalf("inactive: changed %v err %v", changed, err)
	}
	if got := h.carryOf(off, 2027); got != "0.00|0.00|-" {
		t.Fatalf("inactive carry = %q", got)
	}
	// The job creates no new-year row for an inactive employee either.
	if err := h.svc.RunLeaveCarryOver(context.Background(), jan1(2028)); err != nil {
		t.Fatal(err)
	}
	if h.scalar(`SELECT count(*)::text FROM hris.leave_balances WHERE employee_id = $1 AND year = 2028`, off) != "0" {
		t.Fatal("an inactive employee got a 2028 row")
	}
}

// Review 3: a carry HR filled by hand is never overwritten by a recalculation;
// clearing it hands the row back to the formula.
func TestCarryRecalcManualCarryNotOverwritten(t *testing.T) {
	h := newHarness(t)
	f := h.carryFixture(nil)
	editor, _ := h.saldoStaff("pos", false, "read", "update")

	expect(t, h.putSaldo(&editor, f.id, map[string]any{"year": 2027, "carry_over_days": 3,
		"carry_over_expires_on": "2027-09-30", "reason": "Kesepakatan"}), 200, "")
	if h.carrySource(f.id, 2027) != "manual" {
		t.Fatalf("source = %q, want manual", h.carrySource(f.id, 2027))
	}
	audits := h.carryAudits(f.id)

	leave := h.request(f, "2026-12-07", "2026-12-08")
	expect(t, leave, 200, "")
	expect(t, h.approve(f, leave.data()["id"].(string)), 200, "")
	expect(t, h.putSaldo(&editor, f.id, map[string]any{"year": 2026, "annual_leave_adjustment": 1, "reason": "Koreksi"}), 200, "")
	if err := h.svc.RunLeaveCarryOver(context.Background(), jan1(2027)); err != nil { // claimed already: no-op
		t.Fatal(err)
	}
	h.exec(`DELETE FROM hris.leave_carry_over_runs WHERE year = 2027`)
	if err := h.svc.RunLeaveCarryOver(context.Background(), jan1(2027)); err != nil {
		t.Fatal(err)
	}
	if got := h.carryOf(f.id, 2027); got != "3.00|0.00|2027-09-30" {
		t.Fatalf("manual carry = %q, want untouched", got)
	}
	if h.carryAudits(f.id) != audits {
		t.Fatal("a manual row got a carry-over audit")
	}

	// Clearing the carry (0 and no expiry) returns the row to the formula:
	// 2026 remaining is 12 + 1 - 6 = 7.
	expect(t, h.putSaldo(&editor, f.id, map[string]any{"year": 2027, "carry_over_days": 0,
		"carry_over_expires_on": nil, "reason": "Kembali otomatis"}), 200, "")
	if h.carrySource(f.id, 2027) != "auto" {
		t.Fatalf("source after clearing = %q, want auto", h.carrySource(f.id, 2027))
	}
	if got := h.carryOf(f.id, 2027); got != "7.00|0.00|2027-03-31" {
		t.Fatalf("carry after clearing = %q, want the formula's 7", got)
	}
}

// A max of 0 means no carry; a negative remainder counts as 0; the amount is
// floored to 0,5.
func TestCarryRecalcMaxAndFloor(t *testing.T) {
	h := newHarness(t)
	h.carrySetup(true, 0, 3, 31)
	a := h.employee(emp{name: "Bawaan Nol", join: "2024-01-10"})
	h.setBalance(a, 2026, 12, 0, 4, 0, 0, nil)
	h.setBalance(a, 2027, 12, 0, 0, 0, 0, nil)
	changed, err := recomputeCarryOver(context.Background(), h.tx, a, 2027, carryCause{Type: "settings"}, nil)
	if err != nil || changed {
		t.Fatalf("max 0: changed %v err %v", changed, err)
	}
	if got := h.carryOf(a, 2027); got != "0.00|0.00|-" {
		t.Fatalf("carry with max 0 = %q", got)
	}
	h.exec(`UPDATE hris.leave_settings SET carry_over_max_days = 3.5`)
	if changed, err := recomputeCarryOver(context.Background(), h.tx, a, 2027, carryCause{Type: "settings", Label: "Batas sisa bawaan diubah"}, nil); err != nil || !changed {
		t.Fatalf("max 3.5: changed %v err %v", changed, err)
	}
	if got := h.carryOf(a, 2027); got != "3.50|0.00|2027-03-31" {
		t.Fatalf("carry with max 3.5 = %q", got)
	}
	got := h.carryAudit(a, "settings")
	if !strings.Contains(got, "|Batas sisa bawaan diubah|false|Sistem|0|3.5|") {
		t.Fatalf("settings audit = %q (nil actor is the system, label is the reason)", got)
	}
}

// Review 2: the order of locks is year N-1, N, N+1 for every path, so a leave
// decision on last year and an edit of this year's row never wait on each
// other in a cycle. Checked here through the only observable: both orders of
// the two operations in one transaction succeed and agree.
func TestCarryRecalcApproveThenEditSameEmployee(t *testing.T) {
	h := newHarness(t)
	f := h.carryFixture(nil)
	editor, _ := h.saldoStaff("pos", false, "read", "update")
	leave := h.request(f, "2026-12-07", "2026-12-08")
	expect(t, leave, 200, "")
	expect(t, h.approve(f, leave.data()["id"].(string)), 200, "")
	expect(t, h.putSaldo(&editor, f.id, map[string]any{"year": 2027, "annual_leave_adjustment": 1, "reason": "Tahun ini"}), 200, "")
	expect(t, h.putSaldo(&editor, f.id, map[string]any{"year": 2026, "annual_leave_adjustment": 1, "reason": "Tahun lalu"}), 200, "")
	if got := h.carryOf(f.id, 2027); got != "7.00|0.00|2027-03-31" {
		t.Fatalf("carry = %q, want 12 + 1 - 6 = 7", got)
	}
}

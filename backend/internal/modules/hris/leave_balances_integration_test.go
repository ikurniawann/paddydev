package hris

import (
	"context"
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/testutil"
)

// EPIC-068 Task 2: the leave-balance core. fixedNow is 2026-10-05 WIB, so the
// editable years are 2026 and 2027.

// saldoStaff is a signed-in HR account holding actions on Saldo Cuti.
func (h *harness) saldoStaff(role string, linked bool, actions ...string) (testutil.Staff, string) {
	h.t.Helper()
	s, id := h.staff(role, nil, linked)
	if len(actions) > 0 {
		testutil.GrantMenu(h.t, s.RoleID, iam.LeafSaldoCuti, actions)
	}
	return s, id
}

// setBalance writes a balance row of employee for year.
func (h *harness) setBalance(employeeID string, year int, total, adjustment, used, carry, carryUsed float64, expires any) {
	h.t.Helper()
	h.exec(`INSERT INTO hris.leave_balances (employee_id, year, annual_leave_total, annual_leave_adjustment, annual_leave_used,
			carry_over_days, carry_over_used, carry_over_expires_on)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::date)
		ON CONFLICT (employee_id, year) DO UPDATE SET annual_leave_total = EXCLUDED.annual_leave_total,
			annual_leave_adjustment = EXCLUDED.annual_leave_adjustment, annual_leave_used = EXCLUDED.annual_leave_used,
			carry_over_days = EXCLUDED.carry_over_days, carry_over_used = EXCLUDED.carry_over_used,
			carry_over_expires_on = EXCLUDED.carry_over_expires_on`,
		employeeID, year, total, adjustment, used, carry, carryUsed, expires)
}

func (h *harness) balanceOf(employeeID string, year int) (used, carryUsed string) {
	h.t.Helper()
	return h.scalar(`SELECT annual_leave_used::text FROM hris.leave_balances WHERE employee_id = $1 AND year = $2`, employeeID, year),
		h.scalar(`SELECT carry_over_used::text FROM hris.leave_balances WHERE employee_id = $1 AND year = $2`, employeeID, year)
}

func putBody(year int, total, adjustment, carry float64, expires any, reason string) map[string]any {
	return map[string]any{"year": year, "annual_leave_total": total, "annual_leave_adjustment": adjustment,
		"carry_over_days": carry, "carry_over_expires_on": expires, "reason": reason}
}

func TestPutLeaveBalance(t *testing.T) {
	h := newHarness(t)
	viewer, _ := h.saldoStaff("pos", true, "read")
	editor, _ := h.saldoStaff("pos", true, "read", "update")
	// The old role check is gone: an hrd account without the action is refused.
	oldHRD, _ := h.staff("hrd", []string{menuWorkforce}, true)
	_, id := h.staff("pos", nil, true, emp{name: "Pemilik Saldo", join: "2024-01-10"})
	path := "/api/hris/leave-balances/" + id
	good := putBody(2026, 14, 1, 2, "2027-03-31", "Penyesuaian awal tahun")

	expect(t, h.do(&viewer, "PUT", path, good), 403, "Insufficient permissions")
	expect(t, h.do(&oldHRD, "PUT", path, good), 403, "Insufficient permissions")
	expect(t, h.do(nil, "PUT", path, good), 401, "")

	h.setBalance(id, 2026, 12, 0, 5, 3, 1, nil)
	cases := map[string]struct {
		body map[string]any
		want string
	}{
		"tanpa alasan":          {putBody(2026, 12, 0, 0, nil, "  "), "Alasan perubahan wajib diisi"},
		"jatah 0,25":            {putBody(2026, 12.25, 0, 0, nil, "Uji"), "Jatah harus kelipatan 0,5"},
		"penyesuaian 0,25":      {putBody(2026, 12, 0.25, 0, nil, "Uji"), "Penyesuaian harus kelipatan 0,5"},
		"bawaan 0,25":           {putBody(2026, 12, 0, 1.25, nil, "Uji"), "Sisa bawaan harus kelipatan 0,5"},
		"jatah negatif":         {putBody(2026, -1, 0, 0, nil, "Uji"), "Jatah tidak boleh negatif"},
		"tahun lalu":            {putBody(2025, 12, 0, 0, nil, "Uji"), "Tahun saldo hanya boleh 2026 (tahun berjalan) atau 2027 (tahun depan)"},
		"dua tahun lagi":        {putBody(2028, 12, 0, 0, nil, "Uji"), "Tahun saldo hanya boleh 2026 (tahun berjalan) atau 2027 (tahun depan)"},
		"kedaluwarsa rusak":     {putBody(2026, 12, 0, 1, "31-03-2027", "Uji"), "Tanggal kedaluwarsa sisa bawaan tidak valid (format YYYY-MM-DD)"},
		"jatah di bawah pakai":  {putBody(2026, 4, 0, 3, nil, "Uji"), "Sisa jatah tidak boleh kurang dari 0 (jatah 4 + penyesuaian 0 − terpakai 5 = -1)"},
		"penyesuaian menekan":   {putBody(2026, 12, -8, 3, nil, "Uji"), "Sisa jatah tidak boleh kurang dari 0 (jatah 12 + penyesuaian -8 − terpakai 5 = -1)"},
		"bawaan di bawah pakai": {putBody(2026, 12, 0, 0.5, nil, "Uji"), "Sisa bawaan (0,5) tidak boleh kurang dari bawaan yang sudah terpakai (1)"},
	}
	for name, c := range cases {
		got := h.do(&editor, "PUT", path, c.body)
		if got.status != 400 || got.body["error"] != c.want {
			t.Errorf("%s: status %d error %v, want 400 %q", name, got.status, got.body["error"], c.want)
		}
	}
	// "Terpakai" is not an input.
	for _, key := range []string{"annual_leave_used", "sick_leave_used"} {
		body := putBody(2026, 12, 0, 0, nil, "Uji")
		body[key] = 1
		expect(t, h.do(&editor, "PUT", path, body), 400, "")
	}
	if used, _ := h.balanceOf(id, 2026); used != "5.00" {
		t.Fatalf("a refused PUT changed the balance: used %s", used)
	}

	// Success: the figures change, "terpakai" stays, the audit row has before/after/reason.
	ok := h.do(&editor, "PUT", path, good)
	expect(t, ok, 200, "")
	d := ok.data()
	if d["annual_leave_total"] != 14.0 || d["annual_leave_adjustment"] != 1.0 || d["annual_leave_used"] != 5.0 ||
		d["annual_leave_remaining"] != 10.0 || d["carry_over_days"] != 2.0 || d["carry_over_used"] != 1.0 ||
		d["carry_over_remaining"] != 1.0 || d["carry_over_expires_on"] != "2027-03-31" || d["carry_over_active"] != true ||
		d["available"] != 11.0 || d["year"] != 2026.0 || d["employee_id"] != id || d["full_name"] != "Pemilik Saldo" {
		t.Fatalf("PUT data = %s", ok.raw)
	}
	if ok.body["message"] == nil {
		t.Fatalf("no message: %s", ok.raw)
	}
	var reason, action, entity, label string
	row := h.tx.QueryRow(t.Context(), `SELECT reason, action, entity, entity_label FROM audit.audit_log
		WHERE entity_id = $1 AND action = 'leave_balance.update'`, id)
	if err := row.Scan(&reason, &action, &entity, &label); err != nil {
		t.Fatalf("audit row: %v", err)
	}
	if reason != "Penyesuaian awal tahun" || entity != "hris.employees" || !strings.HasPrefix(label, "Pemilik Saldo") {
		t.Fatalf("audit = %q %q %q", reason, entity, label)
	}
	if before := h.scalar(`SELECT before->>'annual_leave_total' FROM audit.audit_log WHERE entity_id = $1 AND action = 'leave_balance.update'`, id); before != "12" {
		t.Fatalf("audit before total = %q", before)
	}
	if after := h.scalar(`SELECT after->>'annual_leave_total' || '/' || (after->>'annual_leave_adjustment') || '/' || (after->>'carry_over_days')
		FROM audit.audit_log WHERE entity_id = $1 AND action = 'leave_balance.update'`, id); after != "14/1/2" {
		t.Fatalf("audit after = %q", after)
	}

	// Next year is editable and its missing row is created from the default rule first.
	next := h.do(&editor, "PUT", path, putBody(2027, 13, 0, 0, nil, "Rencana tahun depan"))
	expect(t, next, 200, "")
	if next.data()["annual_leave_total"] != 13.0 {
		t.Fatalf("2027 = %s", next.raw)
	}
	// Omitted figures keep their stored value; null clears the expiry.
	keep := h.do(&editor, "PUT", path, map[string]any{"year": 2026, "reason": "Hanya tanggal", "carry_over_expires_on": nil})
	expect(t, keep, 200, "")
	if k := keep.data(); k["annual_leave_total"] != 14.0 || k["carry_over_days"] != 2.0 || k["carry_over_expires_on"] != nil || k["carry_over_active"] != false {
		t.Fatalf("keep = %s", keep.raw)
	}

	// An inactive employee, and one that does not exist.
	_, gone := h.staff("pos", nil, true, emp{name: "Sudah Keluar", inactive: true})
	expect(t, h.do(&editor, "PUT", "/api/hris/leave-balances/"+gone, good), 400, "Karyawan nonaktif — saldo cuti tidak bisa diubah")
	expect(t, h.do(&editor, "PUT", "/api/hris/leave-balances/00000000-0000-4000-8000-0000000000ab", good), 404, "")
	expect(t, h.do(&editor, "PUT", "/api/hris/leave-balances/bukan-uuid", good), 400, "")
}

func TestGetLeaveBalance(t *testing.T) {
	h := newHarness(t)
	owner, id := h.staff("pos", nil, true, emp{name: "Pemilik Baca", join: "2024-01-10"})
	reader, _ := h.saldoStaff("pos", false, "read")
	karyawan, _ := h.staff("pos", []string{"hris.kepegawaian.users"}, false)
	stranger, _ := h.staff("pos", nil, true)
	h.setBalance(id, 2026, 12, 1, 4, 3, 1, "2026-12-31")
	path := "/api/hris/leave-balances/" + id

	for name, who := range map[string]*testutil.Staff{"pemilik": &owner, "HR saldo": &reader, "menu Karyawan": &karyawan} {
		got := h.do(who, "GET", path+"?year=2026", nil)
		expect(t, got, 200, "")
		d := got.data()
		if d["annual_leave_total"] != 12.0 || d["annual_leave_adjustment"] != 1.0 || d["annual_leave_used"] != 4.0 ||
			d["annual_leave_remaining"] != 9.0 || d["carry_over_remaining"] != 2.0 || d["carry_over_active"] != true ||
			d["available"] != 11.0 || d["nip"] == nil {
			t.Fatalf("%s: %s", name, got.raw)
		}
	}
	expect(t, h.do(&stranger, "GET", path, nil), 403, "Forbidden: Can only view own leave balance")
	expect(t, h.do(nil, "GET", path, nil), 401, "")
	expect(t, h.do(&owner, "GET", path+"?year=1999", nil), 400, "Tahun tidak valid")
	expect(t, h.do(&reader, "GET", "/api/hris/leave-balances/00000000-0000-4000-8000-0000000000ab", nil), 404, "")

	// A year without a row is created from the default rule on first read (R1):
	// joined 2024-01-10, so 12 days in 2027.
	h.exec(`DELETE FROM hris.leave_balances WHERE employee_id = $1 AND year = 2027`, id)
	first := h.do(&owner, "GET", path+"?year=2027", nil)
	expect(t, first, 200, "")
	if first.data()["annual_leave_total"] != 12.0 || first.data()["exists"] != true {
		t.Fatalf("2027 = %s", first.raw)
	}
	if n := h.scalar(`SELECT count(*)::text FROM hris.leave_balances WHERE employee_id = $1 AND year = 2027`, id); n != "1" {
		t.Fatalf("rows 2027 = %s", n)
	}
}

func TestListLeaveBalances(t *testing.T) {
	h := newHarness(t)
	reader, _ := h.saldoStaff("pos", false, "read")
	noRead, _ := h.saldoStaff("pos", false, "update")
	_, withRow := h.staff("pos", nil, true, emp{name: "Zeta Baris Ada", join: "2024-01-10"})
	_, noRow := h.staff("pos", nil, true, emp{name: "Zeta Baris Belum", join: "2025-03-10"})
	_, inactive := h.staff("pos", nil, true, emp{name: "Zeta Nonaktif", inactive: true})
	h.setBalance(withRow, 2026, 12, 2, 3, 0, 0, nil)
	h.exec(`DELETE FROM hris.leave_balances WHERE employee_id = $1 AND year = 2026`, noRow)
	pvj := h.branch("Toko Saldo PVJ")
	other := h.branch("Toko Saldo Lain")
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date) VALUES ($1, $2, true, '2026-01-01')`, withRow, pvj)
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date) VALUES ($1, $2, true, '2026-01-01')`, noRow, other)

	expect(t, h.do(&noRead, "GET", "/api/hris/leave-balances?year=2026", nil), 403, "Insufficient permissions")
	list := h.do(&reader, "GET", "/api/hris/leave-balances?year=2026&q=Zeta", nil)
	expect(t, list, 200, "")
	byID := map[string]map[string]any{}
	for _, it := range list.list() {
		m := it.(map[string]any)
		byID[m["employee_id"].(string)] = m
	}
	if len(byID) != 2 || byID[inactive] != nil {
		t.Fatalf("list = %s", list.raw)
	}
	if r := byID[withRow]; r["annual_leave_total"] != 12.0 || r["annual_leave_adjustment"] != 2.0 || r["annual_leave_used"] != 3.0 ||
		r["annual_leave_remaining"] != 11.0 || r["exists"] != true || r["branch_name"] != "Toko Saldo PVJ" {
		t.Fatalf("existing row = %v", r)
	}
	// join 2025-03-10 -> 9,5 days in 2026 (R1), shown but not written.
	if r := byID[noRow]; r["annual_leave_total"] != 9.5 || r["annual_leave_remaining"] != 9.5 || r["available"] != 9.5 ||
		r["exists"] != false || r["updated_at"] != nil || r["branch_name"] != "Toko Saldo Lain" {
		t.Fatalf("missing row = %v", r)
	}
	if n := h.scalar(`SELECT count(*)::text FROM hris.leave_balances WHERE employee_id = $1 AND year = 2026`, noRow); n != "0" {
		t.Fatalf("the list wrote %s rows", n)
	}
	// Branch filter and search.
	only := h.do(&reader, "GET", "/api/hris/leave-balances?year=2026&branch_id="+pvj+"&q=Zeta", nil)
	expect(t, only, 200, "")
	if l := only.list(); len(l) != 1 || l[0].(map[string]any)["employee_id"] != withRow {
		t.Fatalf("branch filter = %s", only.raw)
	}
	expect(t, h.do(&reader, "GET", "/api/hris/leave-balances?branch_id=bukan-uuid", nil), 400, "")
	if none := h.do(&reader, "GET", "/api/hris/leave-balances?year=2026&q=Zeta%20Tidak%20Ada", nil); len(none.list()) != 0 {
		t.Fatalf("search = %s", none.raw)
	}
}

func TestLeaveBalanceAccess(t *testing.T) {
	h := newHarness(t)
	editor, _ := h.saldoStaff("pos", true, "read", "update")
	exporter, _ := h.saldoStaff("pos", true, "export")
	none, _ := h.staff("pos", nil, true)
	check := func(who *testutil.Staff, want string) {
		t.Helper()
		got := h.do(who, "GET", "/api/hris/me/leave-balance-access", nil)
		expect(t, got, 200, "")
		if got.raw != want {
			t.Fatalf("access = %s, want %s", got.raw, want)
		}
	}
	check(&editor, `{"read":true,"update":true,"import":false,"export":false}`)
	check(&exporter, `{"read":false,"update":false,"import":false,"export":true}`)
	check(&none, `{"read":false,"update":false,"import":false,"export":false}`)
	expect(t, h.do(nil, "GET", "/api/hris/me/leave-balance-access", nil), 401, "")
}

// leaveFixture is an employee with a direct manager, ready to request leave in
// November 2026 (Mon 9 - Tue 10, Mon 16 - Tue 17: two working days each).
type leaveFixture struct {
	staff, boss, hr testutil.Staff
	id              string
}

func (h *harness) leaveFixture(join string) leaveFixture {
	h.t.Helper()
	hr, _ := h.staff("hrd", []string{menuWorkforce}, true)
	boss, bossID := h.staff("pos", nil, true, emp{name: "Atasan Saldo"})
	staff, id := h.staff("pos", nil, true, emp{name: "Pemohon Saldo", join: join, boss: &bossID})
	return leaveFixture{staff: staff, boss: boss, hr: hr, id: id}
}

func (h *harness) request(f leaveFixture, start, end string) call {
	return h.do(&f.staff, "POST", "/api/hris/leaves", map[string]any{
		"leave_type": "annual", "start_date": start, "end_date": end, "reason": "Keperluan keluarga"})
}

func (h *harness) approve(f leaveFixture, leaveID string) call {
	return h.do(&f.boss, "POST", "/api/hris/leaves/approve", map[string]any{"leave_id": leaveID, "action": "approve"})
}

// Review 1 + R5: the carry-over is used first while it has not expired by the
// leave's start date; an expired carry-over is not touched.
func TestApproveLeaveUsesCarryOverFirst(t *testing.T) {
	h := newHarness(t)
	f := h.leaveFixture("2024-01-10")
	h.setBalance(f.id, 2026, 12, 0, 0, 3, 0, "2026-12-31")

	first := h.request(f, "2026-11-09", "2026-11-10")
	expect(t, first, 200, "")
	firstID := first.data()["id"].(string)
	expect(t, h.approve(f, firstID), 200, "")
	if used, carry := h.balanceOf(f.id, 2026); used != "0.00" || carry != "2.00" {
		t.Fatalf("after first: used %s carry %s", used, carry)
	}
	if got := h.scalar(`SELECT carry_over_days_used::text FROM hris.leaves WHERE id = $1`, firstID); got != "2.00" {
		t.Fatalf("leave carry part = %s", got)
	}

	// 1 day of carry-over left: the next two days split 1 + 1.
	second := h.request(f, "2026-11-16", "2026-11-17")
	expect(t, second, 200, "")
	secondID := second.data()["id"].(string)
	expect(t, h.approve(f, secondID), 200, "")
	if used, carry := h.balanceOf(f.id, 2026); used != "1.00" || carry != "3.00" {
		t.Fatalf("after second: used %s carry %s", used, carry)
	}
	if got := h.scalar(`SELECT carry_over_days_used::text FROM hris.leaves WHERE id = $1`, secondID); got != "1.00" {
		t.Fatalf("leave carry part = %s", got)
	}

	// Cancelling the split leave returns each part to its source.
	cancel := h.do(&f.hr, "PUT", "/api/hris/leaves/"+secondID, map[string]any{"status": "cancelled"})
	expect(t, cancel, 200, "")
	if used, carry := h.balanceOf(f.id, 2026); used != "0.00" || carry != "2.00" {
		t.Fatalf("after cancel: used %s carry %s", used, carry)
	}
}

func TestApproveLeaveIgnoresExpiredCarryOver(t *testing.T) {
	h := newHarness(t)
	f := h.leaveFixture("2024-01-10")
	// The carry-over expired on 1 Nov, before the leave starts on 9 Nov.
	h.setBalance(f.id, 2026, 12, 0, 0, 3, 0, "2026-11-01")

	leave := h.request(f, "2026-11-09", "2026-11-10")
	expect(t, leave, 200, "")
	id := leave.data()["id"].(string)
	expect(t, h.approve(f, id), 200, "")
	if used, carry := h.balanceOf(f.id, 2026); used != "2.00" || carry != "0.00" {
		t.Fatalf("used %s carry %s: expired carry-over was touched", used, carry)
	}
	if got := h.scalar(`SELECT carry_over_days_used::text FROM hris.leaves WHERE id = $1`, id); got != "0.00" {
		t.Fatalf("leave carry part = %s", got)
	}
}

// Review 2: a leave approved before EPIC-068 has carry_over_days_used = 0, so
// cancelling it returns everything to the annual quota.
func TestCancelOldLeaveRefundsAnnualQuota(t *testing.T) {
	h := newHarness(t)
	f := h.leaveFixture("2024-01-10")
	h.setBalance(f.id, 2026, 12, 0, 2, 3, 1, "2026-12-31")
	old := h.scalar(`INSERT INTO hris.leaves (employee_id, leave_type, start_date, end_date, total_days, reason, status, decided_as)
		VALUES ($1, 'annual', '2026-11-09', '2026-11-10', 2, 'Cuti lama', 'approved', 'hr') RETURNING id::text`, f.id)
	if got := h.scalar(`SELECT carry_over_days_used::text FROM hris.leaves WHERE id = $1`, old); got != "0.00" {
		t.Fatalf("legacy leave carry part = %s", got)
	}
	expect(t, h.do(&f.hr, "PUT", "/api/hris/leaves/"+old, map[string]any{"status": "cancelled"}), 200, "")
	if used, carry := h.balanceOf(f.id, 2026); used != "0.00" || carry != "1.00" {
		t.Fatalf("used %s carry %s: want everything back to the annual quota", used, carry)
	}
}

// R5: an approval that no longer fits is a 400, never a database error.
func TestApproveLeaveRefusesWhenQuotaGone(t *testing.T) {
	h := newHarness(t)
	f := h.leaveFixture("2024-01-10")
	h.setBalance(f.id, 2026, 12, 0, 0, 0, 0, nil)
	leave := h.request(f, "2026-11-09", "2026-11-10")
	expect(t, leave, 200, "")
	// HR lowers the quota after the request was made.
	h.setBalance(f.id, 2026, 1, 0, 0, 0, 0, nil)
	expect(t, h.approve(f, leave.data()["id"].(string)), 400, "Sisa cuti tahunan tidak cukup")
	if st := h.scalar(`SELECT status::text FROM hris.leaves WHERE id = $1`, leave.data()["id"].(string)); st != "pending" {
		t.Fatalf("status = %s", st)
	}
}

// R6: the request creates the missing row (R1) and is checked against the
// quota available on its start date.
func TestCreateLeaveChecksAvailable(t *testing.T) {
	h := newHarness(t)
	f := h.leaveFixture("2024-01-10")

	// No row yet: it is created with the default (12 for this join date) and the request passes.
	h.exec(`DELETE FROM hris.leave_balances WHERE employee_id = $1 AND year = 2026`, f.id)
	expect(t, h.request(f, "2026-11-09", "2026-11-10"), 200, "")
	if total := h.scalar(`SELECT annual_leave_total::text FROM hris.leave_balances WHERE employee_id = $1 AND year = 2026`, f.id); total != "12.00" {
		t.Fatalf("created total = %s", total)
	}

	// 1 day left: two days are refused with remaining/requested.
	h.setBalance(f.id, 2026, 12, 0, 11, 0, 0, nil)
	refused := h.request(f, "2026-11-16", "2026-11-17")
	expect(t, refused, 400, "Sisa cuti tahunan tidak cukup")
	if d, _ := refused.body["details"].(map[string]any); d["remaining"] != 1.0 || d["requested"] != 2.0 {
		t.Fatalf("details = %s", refused.raw)
	}
	// An unexpired carry-over adds to what is available; an expired one does not.
	h.setBalance(f.id, 2026, 12, 0, 11, 1, 0, "2026-12-31")
	expect(t, h.request(f, "2026-11-16", "2026-11-17"), 200, "")
	h.exec(`DELETE FROM hris.leaves WHERE employee_id = $1`, f.id)
	h.setBalance(f.id, 2026, 12, 0, 11, 1, 0, "2026-11-12")
	expect(t, h.request(f, "2026-11-16", "2026-11-17"), 400, "Sisa cuti tahunan tidak cukup")
	h.setBalance(f.id, 2026, 12, 0, 11, 1, 0, nil)
	expect(t, h.request(f, "2026-11-16", "2026-11-17"), 400, "Sisa cuti tahunan tidak cukup")

	// A short-tenure employee gets no quota (R1): joined 2026-02-01 -> 0 days.
	youngStaff, _ := h.staff("pos", nil, true, emp{name: "Karyawan Baru", join: "2026-02-01"})
	h.exec(`DELETE FROM hris.leave_balances WHERE employee_id IN (SELECT id FROM hris.employees WHERE user_id = $1)`, youngStaff.UserID)
	expect(t, h.do(&youngStaff, "POST", "/api/hris/leaves", map[string]any{
		"leave_type": "annual", "start_date": "2026-11-09", "end_date": "2026-11-09", "reason": "Keperluan keluarga"}), 400, "Sisa cuti tahunan tidak cukup")
}

// R13: ESS reads the shared view (and creates this year's row when missing).
func TestMeLeaveBalanceIsTheSharedView(t *testing.T) {
	h := newHarness(t)
	staff, id := h.staff("pos", nil, true, emp{name: "Pegawai ESS", join: "2025-03-10"})
	h.exec(`DELETE FROM hris.leave_balances WHERE employee_id = $1 AND year = 2026`, id)

	me := h.do(&staff, "GET", "/api/hris/me", nil)
	expect(t, me, 200, "")
	bal, _ := me.data()["leave_balance"].(map[string]any)
	if bal["year"] != 2026.0 || bal["annual_leave_total"] != 9.5 || bal["available"] != 9.5 || bal["exists"] != true ||
		bal["carry_over_active"] != false || bal["employee_id"] != id {
		t.Fatalf("me leave_balance = %s", me.raw)
	}
	if n := h.scalar(`SELECT count(*)::text FROM hris.leave_balances WHERE employee_id = $1 AND year = 2026`, id); n != "1" {
		t.Fatalf("rows = %s", n)
	}

	h.setBalance(id, 2026, 9.5, 1, 2, 2, 0, "2026-12-31")
	home := h.do(&staff, "GET", "/api/hris/me/beranda", nil)
	expect(t, home, 200, "")
	b, _ := home.data()["leave_balance"].(map[string]any)
	if b["available"] != 10.5 || b["carry_over_remaining"] != 2.0 || b["annual_leave_adjustment"] != 1.0 {
		t.Fatalf("beranda leave_balance = %s", home.raw)
	}
}

// racingRepo lets a test run a competing write right after the service has
// read the leave, which is the window a concurrent request would hit.
type racingRepo struct {
	Repository
	afterRead func()
}

func (r racingRepo) PlainLeave(ctx context.Context, id string) (*Row, error) {
	row, err := r.Repository.PlainLeave(ctx, id)
	if r.afterRead != nil {
		r.afterRead()
	}
	return row, err
}

// approvedSplitLeave is an approved annual leave whose 2 days came 1 from the
// carry-over and 1 from the annual quota (used 1 + carry 1 at the start).
func (h *harness) approvedSplitLeave(f leaveFixture) string {
	h.t.Helper()
	h.setBalance(f.id, 2026, 12, 0, 1, 3, 1, "2026-12-31")
	return h.scalar(`INSERT INTO hris.leaves (employee_id, leave_type, start_date, end_date, total_days, reason, status,
			decided_as, carry_over_days_used)
		VALUES ($1, 'annual', '2026-11-09', '2026-11-10', 2, 'Cuti', 'approved', 'hr', 1) RETURNING id::text`, f.id)
}

// Final review 1: two cancels that both read "approved" refund once; the
// loser gets a 409 and the balance keeps the single refund.
func TestCancelLeaveRefundsOnce(t *testing.T) {
	h := newHarness(t)
	f := h.leaveFixture("2024-01-10")
	id := h.approvedSplitLeave(f)

	rival := true
	h.svc.repo = racingRepo{Repository: h.svc.repo, afterRead: func() {
		if !rival {
			return
		}
		rival = false
		// A competing cancel commits between our read and our write.
		h.exec(`UPDATE hris.leaves SET status = 'cancelled' WHERE id = $1`, id)
		h.exec(`UPDATE hris.leave_balances SET annual_leave_used = 0, carry_over_used = 0
			WHERE employee_id = $1 AND year = 2026`, f.id)
	}}
	expect(t, h.do(&f.hr, "PUT", "/api/hris/leaves/"+id, map[string]any{"status": "cancelled"}), 409,
		"Pengajuan sudah berubah, muat ulang halaman")
	if used, carry := h.balanceOf(f.id, 2026); used != "0.00" || carry != "0.00" {
		t.Fatalf("used %s carry %s: the loser refunded again", used, carry)
	}
	// And sequentially a second cancel is refused without touching the balance.
	h.setBalance(f.id, 2026, 12, 0, 3, 3, 2, "2026-12-31")
	expect(t, h.do(&f.hr, "PUT", "/api/hris/leaves/"+id, map[string]any{"status": "cancelled"}), 403, "")
	if used, carry := h.balanceOf(f.id, 2026); used != "3.00" || carry != "2.00" {
		t.Fatalf("used %s carry %s", used, carry)
	}
}

// Final review 1: the owner cancels a request they read as pending while an
// approval lands; the days the approval consumed must not be lost.
func TestCancelRacingApprovalKeepsDays(t *testing.T) {
	h := newHarness(t)
	f := h.leaveFixture("2024-01-10")
	h.setBalance(f.id, 2026, 12, 0, 0, 0, 0, nil)
	req := h.request(f, "2026-11-09", "2026-11-10")
	expect(t, req, 200, "")
	id := req.data()["id"].(string)

	raced := false
	h.svc.repo = racingRepo{Repository: h.svc.repo, afterRead: func() {
		if raced {
			return
		}
		raced = true
		h.exec(`UPDATE hris.leaves SET status = 'approved', decided_as = 'manager' WHERE id = $1`, id)
		h.exec(`UPDATE hris.leave_balances SET annual_leave_used = 2 WHERE employee_id = $1 AND year = 2026`, f.id)
	}}
	expect(t, h.do(&f.staff, "PUT", "/api/hris/leaves/"+id, map[string]any{"status": "cancelled"}), 409,
		"Pengajuan sudah berubah, muat ulang halaman")
	if st := h.scalar(`SELECT status::text FROM hris.leaves WHERE id = $1`, id); st != "approved" {
		t.Fatalf("status = %s: the approval was overwritten", st)
	}
	if used, _ := h.balanceOf(f.id, 2026); used != "2.00" {
		t.Fatalf("used %s: days lost", used)
	}
}

// Final review 10: a read only writes the row of an active employee for this
// or next year; otherwise it shows the default allocation without writing.
func TestGetLeaveBalanceWritesOnlyWhenItShould(t *testing.T) {
	h := newHarness(t)
	reader, _ := h.saldoStaff("pos", false, "read")
	_, active := h.staff("pos", nil, true, emp{name: "Aktif Baca", join: "2024-01-10"})
	_, gone := h.staff("pos", nil, true, emp{name: "Keluar Baca", join: "2024-01-10", inactive: true})
	rows := func(id string, year int) string {
		return h.scalar(`SELECT count(*)::text FROM hris.leave_balances WHERE employee_id = $1 AND year = $2`, id, year)
	}

	h.exec(`DELETE FROM hris.leave_balances WHERE employee_id = $1 AND year IN (2026, 2028)`, gone)
	got := h.do(&reader, "GET", "/api/hris/leave-balances/"+gone+"?year=2026", nil)
	expect(t, got, 200, "")
	if d := got.data(); d["exists"] != false || d["annual_leave_total"] != 12.0 || d["full_name"] != "Keluar Baca" {
		t.Fatalf("inactive = %s", got.raw)
	}
	if n := rows(gone, 2026); n != "0" {
		t.Fatalf("an inactive employee got a row (%s)", n)
	}

	h.exec(`DELETE FROM hris.leave_balances WHERE employee_id = $1 AND year = 2028`, active)
	got = h.do(&reader, "GET", "/api/hris/leave-balances/"+active+"?year=2028", nil)
	expect(t, got, 200, "")
	if got.data()["exists"] != false || rows(active, 2028) != "0" {
		t.Fatalf("2028 = %s rows %s", got.raw, rows(active, 2028))
	}
	// Next year (2027) still creates the row.
	h.exec(`DELETE FROM hris.leave_balances WHERE employee_id = $1 AND year = 2027`, active)
	expect(t, h.do(&reader, "GET", "/api/hris/leave-balances/"+active+"?year=2027", nil), 200, "")
	if n := rows(active, 2027); n != "1" {
		t.Fatalf("2027 rows = %s", n)
	}
}

// beforeTxRepo runs a competing write just before the service opens its
// transaction, i.e. after anything the handler read earlier.
type beforeTxRepo struct {
	Repository
	hook func()
}

func (r beforeTxRepo) InTx(ctx context.Context, fn func(Repository) error) error {
	if r.hook != nil {
		r.hook()
	}
	return r.Repository.InTx(ctx, fn)
}

// Final review 9: a figure left out of the PUT keeps the value stored when
// the row is locked, not the one read before a concurrent edit.
func TestPutLeaveBalanceKeepsOmittedFigureUnderLock(t *testing.T) {
	h := newHarness(t)
	editor, _ := h.saldoStaff("pos", true, "read", "update")
	_, id := h.staff("pos", nil, true, emp{name: "Parsial Saldo", join: "2024-01-10"})
	h.setBalance(id, 2026, 12, 0, 0, 0, 0, nil)
	h.svc.repo = beforeTxRepo{Repository: h.svc.repo, hook: func() {
		h.exec(`UPDATE hris.leave_balances SET annual_leave_total = 20 WHERE employee_id = $1 AND year = 2026`, id)
	}}
	expect(t, h.do(&editor, "PUT", "/api/hris/leave-balances/"+id,
		map[string]any{"year": 2026, "carry_over_days": 1, "reason": "Hanya bawaan"}), 200, "")
	if got := h.scalar(`SELECT concat_ws('|', annual_leave_total, carry_over_days) FROM hris.leave_balances WHERE employee_id = $1 AND year = 2026`, id); got != "20.00|1.00" {
		t.Fatalf("balance = %s: the concurrent edit of the total was overwritten", got)
	}
}

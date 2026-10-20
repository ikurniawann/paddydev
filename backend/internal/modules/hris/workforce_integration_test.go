package hris

import (
	"context"
	"strings"
	"testing"
)

func TestAttendance(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", nil, true)
	staff, own := h.staff("pos", nil, true, emp{name: "Absen Test"})
	h.grantDelete(hr, "hris.workforce.attendance")
	shift := h.scalar(`INSERT INTO hris.shifts (name, start_time, end_time, late_tolerance_minutes)
		VALUES ('Pagi Roster', '08:00', '16:00', 10) RETURNING id::text`)
	h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from) VALUES ($1, 1, $2, '2026-01-01')`, own, shift)
	att := h.scalar(`INSERT INTO hris.attendance (employee_id, date, clock_in, status, is_late, late_minutes, shift_id)
		VALUES ($1, '2026-10-05', '2026-10-05T01:20:00Z', 'present', true, 20, $2) RETURNING id::text`, own, shift)

	mine := h.do(&staff, "GET", "/api/hris/attendance?employee_id=someone-else", nil)
	expect(t, mine, 200, "")
	if len(mine.list()) != 1 || mine.body["pagination"].(map[string]any)["totalPages"] != 1.0 {
		t.Fatalf("own list = %s", mine.raw)
	}
	row := mine.list()[0].(map[string]any)
	if row["employee"].(map[string]any)["full_name"] != "Absen Test" || row["shift"].(map[string]any)["name"] != "Pagi Roster" {
		t.Fatalf("embeds = %v", row)
	}
	unlinked, _ := h.staff("pos", nil, false)
	expect(t, h.do(&unlinked, "GET", "/api/hris/attendance", nil), 403, "Akun ini tidak terhubung ke data karyawan")
	empty := h.do(&unlinked, "GET", "/api/hris/attendance?employee_id=me", nil)
	if empty.raw != `{"data":[],"pagination":{"page":1,"limit":0,"total":0,"totalPages":0}}` {
		t.Fatalf("me without employee = %s", empty.raw)
	}

	expect(t, h.do(&staff, "GET", "/api/hris/attendance/"+att, nil), 200, "")
	expect(t, h.do(&staff, "PUT", "/api/hris/attendance/"+att, map[string]any{"notes": "x"}), 403,
		"Forbidden: Only HRD or managers can update attendance")
	upd := h.do(&hr, "PUT", "/api/hris/attendance/"+att, map[string]any{"validation_notes": "ok", "validated": true})
	expect(t, upd, 200, "")
	if upd.body["message"] != "Attendance updated successfully" || upd.data()["validated_by"] == nil {
		t.Fatalf("validate = %s", upd.raw)
	}

	roster := h.do(&hr, "GET", "/api/hris/attendance/daily-roster?date=2026-10-05", nil)
	expect(t, roster, 200, "")
	d := roster.data()
	if d["is_today"] != true || d["day_of_week"] != 1.0 || d["summary"].(map[string]any)["terlambat"].(float64) < 1 {
		t.Fatalf("roster = %s", roster.raw)
	}
	if got := keys(t, roster.raw, "data", "summary"); got[0] != "scheduled" || got[8] != "tanpa_jadwal" {
		t.Fatalf("summary order %v", got)
	}
	expect(t, h.do(&staff, "GET", "/api/hris/attendance/daily-roster", nil), 403, "Forbidden")
	expect(t, h.do(&hr, "GET", "/api/hris/attendance/daily-roster?date=05-10-2026", nil), 400, "Format tanggal tidak valid")

	sched := h.do(&staff, "GET", "/api/hris/attendance/schedule", nil)
	expect(t, sched, 200, "")
	if r := sched.list()[0].(map[string]any); r["effective_from"] != "2026-01-01" || r["start_time"] != "08:00:00" {
		t.Fatalf("schedule = %s", sched.raw)
	}
	stats := h.do(&staff, "GET", "/api/hris/attendance/stats", nil)
	expect(t, stats, 200, "")
	if s := stats.data(); s["month"] != 10.0 || s["month_late"] != 1.0 || s["active_employees"] != nil {
		t.Fatalf("stats = %s", stats.raw)
	}
	expect(t, h.do(&staff, "DELETE", "/api/hris/attendance/"+att, nil), 403, "Insufficient permissions")
	// HR deletion needs a reason (attendance adjustment log).
	expect(t, h.do(&hr, "DELETE", "/api/hris/attendance/"+att, nil), 400, "Alasan penyesuaian wajib diisi (minimal 5 karakter)")
	expect(t, h.do(&hr, "DELETE", "/api/hris/attendance/"+att, map[string]any{"reason": "Data uji dihapus"}), 200, "")
}

func TestLeaves(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuWorkforce}, true)
	boss, bossID := h.staff("pos", nil, true, emp{name: "Atasan Cuti", phone: "0812 3456 7890"})
	staff, own := h.staff("pos", nil, true, emp{name: "Pemohon Cuti"})
	h.grantDelete(hr, "hris.workforce.leaves")
	h.exec(`UPDATE hris.employees SET reporting_to = $2 WHERE id = $1`, own, bossID)
	h.exec(`INSERT INTO hris.public_holidays (holiday_date, name, type) VALUES ('2031-12-25', 'Natal Test', 'nasional')`)

	expect(t, h.do(&staff, "POST", "/api/hris/leaves", map[string]any{"leave_type": "annual"}), 400, "Validation failed")
	weekend := h.do(&staff, "POST", "/api/hris/leaves", map[string]any{
		"leave_type": "annual", "start_date": "2031-12-25", "end_date": "2031-12-25", "reason": "Liburan akhir tahun"})
	expect(t, weekend, 400, "Rentang tanggal ini sudah hari libur (Natal Test) — tidak perlu mengajukan cuti")

	created := h.do(&staff, "POST", "/api/hris/leaves", map[string]any{
		"leave_type": "annual", "start_date": "2031-12-24", "end_date": "2031-12-29", "reason": "Liburan akhir tahun"})
	expect(t, created, 200, "")
	meta := created.body["meta"].(map[string]any)
	if meta["total_days"] != 3.0 || len(meta["excluded_holidays"].([]any)) != 1 {
		t.Fatalf("leave meta = %s", created.raw)
	}
	leaveID := created.data()["id"].(string)

	// The manager's WhatsApp note goes through the outbox.
	if _, err := h.deps.Events.Dispatch(context.Background(), h.tx); err != nil {
		t.Fatal(err)
	}
	if len(h.wa.sent) != 1 || !strings.HasPrefix(h.wa.sent[0], "6281234567890|Paddy OS — Pengajuan Cuti Tahunan") ||
		!strings.Contains(h.wa.sent[0], "Kepada: Atasan Cuti") {
		t.Fatalf("whatsapp = %v", h.wa.sent)
	}

	list := h.do(&staff, "GET", "/api/hris/leaves?limit=abc", nil)
	expect(t, list, 400, "Invalid input: expected number, received NaN")
	list = h.do(&staff, "GET", "/api/hris/leaves", nil)
	expect(t, list, 200, "")
	if len(list.list()) != 1 || list.body["pagination"].(map[string]any)["limit"] != 20.0 {
		t.Fatalf("leaves = %s", list.raw)
	}
	stranger, _ := h.staff("pos", nil, true)
	expect(t, h.do(&stranger, "GET", "/api/hris/leaves/"+leaveID, nil), 403, "Insufficient permissions")
	expect(t, h.do(&boss, "GET", "/api/hris/leaves/"+leaveID, nil), 200, "")

	approved := h.do(&boss, "POST", "/api/hris/leaves/approve", map[string]any{"leave_id": leaveID, "action": "approve"})
	expect(t, approved, 200, "")
	if approved.body["message"] != "Leave request approved successfully" || approved.body["wa_link"] != nil {
		t.Fatalf("approve = %s", approved.raw)
	}
	if used := h.scalar(`SELECT annual_leave_used::text FROM hris.leave_balances WHERE employee_id = $1 AND year = 2031`, own); used == "" || used == "0" {
		t.Fatalf("quota not consumed: %q", used)
	}
	again := h.do(&boss, "POST", "/api/hris/leaves/approve", map[string]any{"leave_id": leaveID, "action": "approve"})
	expect(t, again, 400, "Leave request already approved")

	expect(t, h.do(&staff, "PUT", "/api/hris/leaves/"+leaveID, map[string]any{"status": "cancelled"}), 403,
		"Cuti yang sudah disetujui hanya bisa dibatalkan oleh HRD/admin")
	cancel := h.do(&hr, "PUT", "/api/hris/leaves/"+leaveID, map[string]any{"status": "cancelled"})
	expect(t, cancel, 200, "")
	if !strings.HasPrefix(cancel.body["message"].(string), "Cuti dibatalkan — kuota") {
		t.Fatalf("cancel = %s", cancel.raw)
	}

	csv := h.do(&hr, "GET", "/api/hris/leaves/export", nil)
	expect(t, csv, 200, "")
	if !strings.HasPrefix(csv.raw, "ID Cuti,NIP,Nama Karyawan") || !strings.Contains(csv.raw, `"2031-12-24"`) {
		t.Fatalf("csv = %s", csv.raw)
	}
	expect(t, h.do(&hr, "GET", "/api/hris/leaves/export?leave_type=sick", nil), 404, "No leave data found")
	expect(t, h.do(&staff, "DELETE", "/api/hris/leaves/"+leaveID, nil), 403, "Insufficient permissions")
	expect(t, h.do(&hr, "DELETE", "/api/hris/leaves/"+leaveID+"?reason=Pengajuan%20ganda", nil), 200, "")

	// Balances: the owner reads (row created on first read), HRD writes.
	bal := h.do(&staff, "GET", "/api/hris/leave-balances/"+own+"?year=2027", nil)
	expect(t, bal, 200, "")
	if bal.data()["annual_leave_total"] != "12.00" && bal.data()["annual_leave_total"] != "12" {
		t.Fatalf("balance = %s", bal.raw)
	}
	expect(t, h.do(&staff, "GET", "/api/hris/leave-balances/"+own+"?year=1999", nil), 400, "Tahun tidak valid")
	expect(t, h.do(&staff, "PUT", "/api/hris/leave-balances/"+own, map[string]any{"annual_leave_total": 14}), 403,
		"Forbidden: Only HRD can update leave balances")
	set := h.do(&hr, "PUT", "/api/hris/leave-balances/"+own+"?year=2027", map[string]any{"annual_leave_total": 14})
	expect(t, set, 200, "")
	if set.body["message"] != "Leave balance updated successfully" {
		t.Fatalf("balance update = %s", set.raw)
	}
}

func TestOvertimeAndShifts(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuKepegawaian}, true)
	boss, bossID := h.staff("pos", nil, true)
	staff, own := h.staff("pos", nil, true)
	h.grantDelete(hr, "hris.kepegawaian.shifts")
	h.exec(`UPDATE hris.employees SET reporting_to = $2 WHERE id = $1`, own, bossID)

	created := h.do(&staff, "POST", "/api/hris/overtime", map[string]any{
		"date": "2026-10-06", "start_time": "17:00", "end_time": "19:30", "reason": "Stock opname"})
	expect(t, created, 200, "")
	if created.data()["hours"] != "2.50" || created.data()["source"] != "employee" {
		t.Fatalf("overtime = %s", created.raw)
	}
	expect(t, h.do(&staff, "POST", "/api/hris/overtime", map[string]any{
		"date": "2026-10-06", "start_time": "17:00", "end_time": "19:30", "reason": "Lagi"}), 400, "Validation failed")
	dup := h.do(&staff, "POST", "/api/hris/overtime", map[string]any{
		"date": "2026-10-06", "start_time": "18:00", "end_time": "19:00", "reason": "Stock opname lagi"})
	expect(t, dup, 400, "Sudah ada pengajuan lembur pending/approved di tanggal tersebut")

	approvals := h.do(&boss, "GET", "/api/hris/overtime?scope=approvals", nil)
	expect(t, approvals, 200, "")
	if len(approvals.list()) != 1 {
		t.Fatalf("approvals = %s", approvals.raw)
	}
	id := created.data()["id"].(string)
	expect(t, h.do(&staff, "POST", "/api/hris/overtime/decide", map[string]any{"overtime_id": id, "action": "approve"}), 403,
		"Tidak bisa memutuskan pengajuan lembur sendiri")
	expect(t, h.do(&boss, "POST", "/api/hris/overtime/decide", map[string]any{"overtime_id": id, "action": "reject"}), 400,
		"Alasan penolakan wajib diisi")
	ok := h.do(&boss, "POST", "/api/hris/overtime/decide", map[string]any{"overtime_id": id, "action": "approve"})
	expect(t, ok, 200, "")
	if ok.body["message"] != "Lembur disetujui" {
		t.Fatalf("decide = %s", ok.raw)
	}

	// Shift master.
	expect(t, h.do(&staff, "GET", "/api/hris/shifts", nil), 200, "")
	expect(t, h.do(&hr, "POST", "/api/hris/shifts", map[string]any{"start_time": "08:00", "end_time": "16:00"}), 400, "Nama shift wajib diisi")
	expect(t, h.do(&hr, "POST", "/api/hris/shifts", map[string]any{"name": "Malam", "start_time": "22:00", "end_time": "06:00"}), 400,
		"Jam selesai harus setelah jam mulai — atau tandai sebagai shift malam (lewat tengah malam)")
	shift := h.do(&hr, "POST", "/api/hris/shifts", map[string]any{"code": "malam1", "name": " Malam ", "start_time": "22:00", "end_time": "06:00", "is_overnight": true})
	expect(t, shift, 201, "")
	if shift.body["message"] != `Shift "Malam" dibuat` || shift.data()["late_tolerance_minutes"] != 10.0 {
		t.Fatalf("shift = %s", shift.raw)
	}
	sid := shift.data()["id"].(string)
	expect(t, h.do(&hr, "PATCH", "/api/hris/shifts/"+sid, map[string]any{"start_time": "25:00"}), 400, "Jam mulai tidak valid")
	expect(t, h.do(&hr, "PATCH", "/api/hris/shifts/"+sid, map[string]any{"sort_order": 3}), 200, "")
	expect(t, h.do(&hr, "PATCH", "/api/hris/shifts/x", nil), 400, "ID shift tidak valid")
	del := h.do(&hr, "DELETE", "/api/hris/shifts/"+sid+"?reason=Shift%20uji%20dihapus", nil)
	expect(t, del, 200, "")
	if del.body["message"] != "Shift dihapus" {
		t.Fatalf("delete = %s", del.raw)
	}
	if got := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'hris.shifts.delete' AND entity_id = $1 AND reason = 'Shift uji dihapus'`, sid); got != "1" {
		t.Fatalf("shift delete audit rows = %s", got)
	}
	expect(t, h.do(&hr, "DELETE", "/api/hris/shifts/"+sid+"?reason=Shift%20uji%20dihapus", nil), 404, "Shift tidak ditemukan")
}

func TestHolidays(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuKepegawaian}, false)
	staff, _ := h.staff("pos", nil, false)
	h.grantDelete(hr, "hris.kepegawaian.holidays")

	created := h.do(&hr, "POST", "/api/hris/holidays", map[string]any{"holiday_date": "2031-08-17", "name": " HUT RI ", "status": "draft"})
	expect(t, created, 201, "")
	if created.body["message"] != `Libur "HUT RI" ditambahkan` || created.data()["holiday_date"] != "2031-08-17" {
		t.Fatalf("holiday = %s", created.raw)
	}
	expect(t, h.do(&hr, "POST", "/api/hris/holidays", map[string]any{"name": "X"}), 400, "Tanggal wajib diisi (YYYY-MM-DD)")
	expect(t, h.do(&hr, "POST", "/api/hris/holidays", map[string]any{"holiday_date": "2031-08-17", "name": "HUT RI"}), 409,
		"Libur dengan tanggal dan nama yang sama sudah ada")

	// Drafts are for HR only.
	staffList := h.do(&staff, "GET", "/api/hris/holidays?year=2031&include_draft=1", nil)
	expect(t, staffList, 200, "")
	if len(staffList.list()) != 0 {
		t.Fatalf("draft leaked: %s", staffList.raw)
	}
	hrList := h.do(&hr, "GET", "/api/hris/holidays?year=2031&include_draft=1", nil)
	if len(hrList.list()) != 1 || hrList.body["meta"].(map[string]any)["end_date"] != "2031-12-31" {
		t.Fatalf("hr list = %s", hrList.raw)
	}
	expect(t, h.do(&hr, "GET", "/api/hris/holidays?start_date=2031-01-01", nil), 400, "start_date dan end_date wajib berformat YYYY-MM-DD")

	id := created.data()["id"].(string)
	expect(t, h.do(&hr, "PATCH", "/api/hris/holidays/"+id, map[string]any{"status": "aktif", "note": ""}), 200, "")
	expect(t, h.do(&hr, "PATCH", "/api/hris/holidays/"+id, map[string]any{"type": "regional"}), 400, "Tipe libur tidak valid")
	expect(t, h.do(&hr, "DELETE", "/api/hris/holidays/"+id+"?reason=Libur%20salah%20input", nil), 200, "")
	expect(t, h.do(&hr, "DELETE", "/api/hris/holidays/"+id+"?reason=Libur%20salah%20input", nil), 404, "Hari libur tidak ditemukan")

	preview := h.do(&hr, "GET", "/api/hris/holidays/import?year=2026", nil)
	expect(t, preview, 200, "")
	if meta := preview.body["meta"].(map[string]any); meta["total"] != 1.0 || meta["year"] != 2026.0 {
		t.Fatalf("preview = %s", preview.raw)
	}
	expect(t, h.do(&hr, "POST", "/api/hris/holidays/import", map[string]any{"items": []any{}}), 400, "Tidak ada hari libur yang dicentang")
	imported := h.do(&hr, "POST", "/api/hris/holidays/import", map[string]any{"items": []any{
		map[string]any{"holiday_date": "2031-12-25", "name": "Natal Impor", "source_ref": "natal-2031@test"}}})
	expect(t, imported, 200, "")
	if imported.body["message"] != "1 hari libur ditambahkan" {
		t.Fatalf("import = %s", imported.raw)
	}
	again := h.do(&hr, "POST", "/api/hris/holidays/import", map[string]any{"items": []any{
		map[string]any{"holiday_date": "2031-12-25", "name": "Natal Impor", "source_ref": "natal-2031@test"}}})
	if again.body["message"] != "0 hari libur ditambahkan, 1 diperbarui" {
		t.Fatalf("re-import = %s", again.raw)
	}
}

// Schema messages ported from shifts-repo.test, employees-shifts.test and
// workforce-route.test.
func TestSchemaMessages(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuKepegawaian}, false)
	e := h.employee(emp{})

	expect(t, h.do(&hr, "POST", "/api/hris/shifts", map[string]any{"name": "Pagi", "start_time": "8", "end_time": "16:00"}), 400,
		"Jam mulai tidak valid (HH:MM)")
	expect(t, h.do(&hr, "POST", "/api/hris/shifts", map[string]any{"name": "Pagi", "start_time": "08:00", "end_time": "16:00",
		"late_tolerance_minutes": -1}), 400, "Toleransi terlambat harus angka ≥ 0")
	expect(t, h.do(&hr, "POST", "/api/hris/shifts", "bukan-json"), 400, "Body JSON tidak valid")

	week := func(shift any) []map[string]any {
		out := []map[string]any{}
		for d := 1; d <= 7; d++ {
			out = append(out, map[string]any{"day_of_week": d, "shift_id": shift})
		}
		return out
	}
	path := "/api/hris/employees/" + e + "/shifts"
	expect(t, h.do(&hr, "PUT", path, map[string]any{"days": week(nil)}), 400, "Tanggal mulai berlaku wajib diisi (YYYY-MM-DD)")
	dup := append(week(nil)[1:], map[string]any{"day_of_week": 2, "shift_id": nil})
	expect(t, h.do(&hr, "PUT", path, map[string]any{"effective_from": "2026-10-05", "days": dup}), 400,
		"Pola jadwal harus lengkap 7 hari (Senin–Minggu)")
	expect(t, h.do(&hr, "PUT", path, map[string]any{"effective_from": "2026-10-05", "days": week("pagi")}), 400, "ID shift tidak valid")
}

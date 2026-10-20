package hris

import (
	"context"
	"strings"
	"testing"
)

// fixedNow is 2026-10-05 (WIB): September (calendar, closing 3 Oct) is past
// its closing date, October is open.

func periodByName(t *testing.T, c call, year, month float64) map[string]any {
	t.Helper()
	for _, p := range c.data()["periods"].([]any) {
		row := p.(map[string]any)
		if row["period_year"] == year && row["period_month"] == month {
			return row
		}
	}
	t.Fatalf("period %v-%v missing in %s", year, month, c.raw)
	return nil
}

func TestAttendancePeriodClosing(t *testing.T) {
	h := newHarness(t)
	fakePeriodRuns = nil
	t.Cleanup(func() { fakePeriodRuns = nil })
	// Catch overlapping periods at once (the constraint is deferred).
	h.exec(`SET CONSTRAINTS hris.attendance_periods_no_overlap IMMEDIATE`)
	// The test database may hold periods from other runs only inside their
	// own rolled-back transactions; start from the default settings.
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 0, closing_offset_days = 3`)

	hr, _ := h.staff("hrd", nil, true)
	manager, _ := h.staff("hrd", []string{"hris.workforce.attendance"}, true) // approve on Absensi = HR manager
	admin, _ := h.staff("super_admin", nil, true)
	recruiter, _ := h.staff("hiring_manager", nil, true)
	crew, crewID := h.staff("pos", nil, true, emp{name: "Crew Periode"})
	h.grantDelete(hr, "hris.workforce.attendance")
	shift := h.scalar(`INSERT INTO hris.shifts (name, start_time, end_time, late_tolerance_minutes)
		VALUES ('Pagi Periode', '08:00', '17:00', 10) RETURNING id::text`)
	for dow := 1; dow <= 7; dow++ {
		h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from) VALUES ($1, $2, $3, '2026-01-01')`,
			crewID, dow, shift)
	}
	// A September row recorded before anything was closed.
	sepRow := h.scalar(`INSERT INTO hris.attendance (employee_id, date, clock_in, clock_out, status)
		VALUES ($1, '2026-09-15', '2026-09-15 08:00+07', '2026-09-15 17:00+07', 'present') RETURNING id::text`, crewID)

	settings := h.do(&hr, "GET", "/api/hris/attendance-settings", nil)
	expect(t, settings, 200, "")
	cur := settings.data()["current_period"].(map[string]any)
	if cur["label"] != "1–31 Okt 2026" || cur["closing_date"] != "2026-11-03" || cur["status"] != "open" ||
		cur["days_until_closing"] != 29.0 {
		t.Fatalf("settings = %s", settings.raw)
	}
	expect(t, h.do(&crew, "GET", "/api/hris/attendance-periods", nil), 403, "")

	// Listing materializes the year; September is past its closing date and
	// is closed by "Sistem".
	list := h.do(&hr, "GET", "/api/hris/attendance-periods?year=2026", nil)
	expect(t, list, 200, "")
	sep := periodByName(t, list, 2026, 9)
	if sep["status"] != "closed" || sep["close_mode"] != "auto" || sep["closed_by_name"] != "Sistem" ||
		len(sep["events"].([]any)) != 1 || sep["close_summary"].(map[string]any)["rows"] != 1.0 {
		t.Fatalf("september = %v", sep)
	}
	oct := periodByName(t, list, 2026, 10)
	if oct["status"] != "open" || list.data()["permissions"].(map[string]any)["can_reopen"] != false {
		t.Fatalf("october = %v / %s", oct, list.raw)
	}
	if n := len(list.data()["periods"].([]any)); n != 10 {
		t.Fatalf("periods listed = %d", n)
	}
	if h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'attendance_period.auto_close' AND entity_id = $1`, sep["id"]) != "1" {
		t.Fatal("auto close not audited")
	}
	sepID, octID := sep["id"].(string), oct["id"].(string)

	// Locked: every write path refuses September with the period in the message.
	locked := "Periode absensi 1–30 Sep 2026 sudah di-closing"
	for _, c := range []call{
		h.do(&hr, "POST", "/api/hris/attendance/manual", map[string]any{"employee_id": crewID, "date": "2026-09-16",
			"clock_in": "08:00", "clock_out": "17:00", "reason": "Lupa absen, ada bukti CCTV"}),
		h.do(&hr, "PUT", "/api/hris/attendance/"+sepRow, map[string]any{"clock_in": "08:30", "reason": "Koreksi jam masuk"}),
		h.do(&hr, "DELETE", "/api/hris/attendance/"+sepRow+"?reason=Data%20ganda%20sistem", nil),
		h.do(&hr, "POST", "/api/hris/attendance", map[string]any{"action": "clock-in", "employee_id": crewID, "date": "2026-09-17",
			"photo": "data:image/png;base64,iVBORw0KGgo="}),
	} {
		expect(t, c, 409, "")
		if !strings.HasPrefix(c.body["error"].(string), locked) {
			t.Fatalf("locked error = %s", c.raw)
		}
	}
	// The database guard backs the application check.
	h.exec("SAVEPOINT guard")
	if _, err := h.tx.Exec(context.Background(), `UPDATE hris.attendance SET notes = 'x' WHERE id = $1`, sepRow); err == nil ||
		!strings.Contains(err.Error(), "sudah di-closing") {
		t.Fatalf("db guard = %v", err)
	}
	h.exec("ROLLBACK TO SAVEPOINT guard")

	// October is open: HR adds a missing row with a reason; lateness follows
	// the shift (08:00 + 10 min tolerance) and work hours the trigger.
	body := map[string]any{"employee_id": crewID, "date": "2026-10-02", "clock_in": "08:30", "clock_out": "17:00",
		"overtime_hours": 1.5, "notes": "Mesin absen error"}
	expect(t, h.do(&hr, "POST", "/api/hris/attendance/manual", body), 400,
		"Alasan penyesuaian wajib diisi (minimal 5 karakter)")
	body["reason"] = "Mesin absensi toko error pagi itu"
	expect(t, h.do(&recruiter, "POST", "/api/hris/attendance/manual", body), 403, "")
	created := h.do(&hr, "POST", "/api/hris/attendance/manual", body)
	expect(t, created, 201, "")
	row := created.data()
	if row["is_manual"] != true || row["is_late"] != true || row["late_minutes"] != 30.0 || row["work_hours"] != "7.50" ||
		row["overtime_hours"] != "1.50" || row["is_overtime"] != true || row["shift_id"] != shift || row["adjustment_count"] != 1.0 {
		t.Fatalf("manual create = %s", created.raw)
	}
	attID := row["id"].(string)
	expect(t, h.do(&hr, "POST", "/api/hris/attendance/manual", body), 409, "")
	expect(t, h.do(&hr, "POST", "/api/hris/attendance/manual", map[string]any{"employee_id": crewID, "date": "2026-10-03",
		"reason": "Tidak ada jam"}), 400, "Jam masuk wajib diisi (kecuali status Tidak hadir)")

	// Edit: clock-in inside the tolerance clears the lateness; a manual
	// value overrides it.
	expect(t, h.do(&hr, "PUT", "/api/hris/attendance/"+attID, map[string]any{"clock_in": "08:05"}), 400,
		"Alasan penyesuaian wajib diisi (minimal 5 karakter)")
	expect(t, h.do(&recruiter, "PUT", "/api/hris/attendance/"+attID, map[string]any{"clock_in": "08:05", "reason": "Koreksi"}), 403, "")
	edited := h.do(&hr, "PUT", "/api/hris/attendance/"+attID, map[string]any{"clock_in": "08:05", "reason": "Jam masuk sesuai CCTV"})
	expect(t, edited, 200, "")
	if d := edited.data(); d["is_late"] != false || d["late_minutes"] != 0.0 || d["work_hours"] != "7.92" || d["adjustment_count"] != 2.0 ||
		d["adjusted_by_name"] == nil {
		t.Fatalf("edit = %s", edited.raw)
	}
	manual := h.do(&hr, "PUT", "/api/hris/attendance/"+attID, map[string]any{"late_minutes": 12, "clock_out": nil,
		"reason": "Keputusan HR: dihitung telat"})
	expect(t, manual, 200, "")
	if d := manual.data(); d["is_late"] != true || d["late_minutes"] != 12.0 || d["clock_out"] != nil || d["work_hours"] != nil {
		t.Fatalf("manual late = %s", manual.raw)
	}
	// Validation alone needs no reason (and HR roles keep it).
	expect(t, h.do(&recruiter, "PUT", "/api/hris/attendance/"+attID, map[string]any{"validated": true}), 200, "")

	history := h.do(&hr, "GET", "/api/hris/attendance-adjustments?attendance_id="+attID, nil)
	expect(t, history, 200, "")
	if l := history.list(); len(l) != 3 || l[0].(map[string]any)["action"] != "update" || l[2].(map[string]any)["action"] != "create" ||
		!strings.Contains(history.raw, `"changed_fields":["clock_in","is_late","late_minutes","work_hours"]`) {
		t.Fatalf("history = %s", history.raw)
	}

	expect(t, h.do(&hr, "DELETE", "/api/hris/attendance/"+attID, nil), 400, "Alasan penyesuaian wajib diisi (minimal 5 karakter)")
	expect(t, h.do(&hr, "DELETE", "/api/hris/attendance/"+attID+"?reason=Input%20salah%20karyawan", nil), 200, "")
	if h.scalar(`SELECT count(*)::text FROM hris.attendance_adjustments WHERE attendance_id = $1 AND action = 'delete'`, attID) != "1" {
		t.Fatal("delete not logged")
	}

	// Closing: October has not ended yet.
	expect(t, h.do(&hr, "POST", "/api/hris/attendance-periods/"+octID+"/close", nil), 400,
		"Periode absensi 1–31 Okt 2026 belum berakhir — closing baru bisa dilakukan setelah 31 Okt 2026")
	// Closing date can move, not into the past.
	expect(t, h.do(&hr, "PATCH", "/api/hris/attendance-periods/"+octID, map[string]any{"closing_date": "2026-10-01"}), 400, "")
	moved := h.do(&hr, "PATCH", "/api/hris/attendance-periods/"+octID, map[string]any{"closing_date": "2026-11-05"})
	expect(t, moved, 200, "")
	if moved.data()["closing_date"] != "2026-11-05" || moved.data()["closing_date_custom"] != true {
		t.Fatalf("closing date = %s", moved.raw)
	}

	// Reopen: only super admin / HR manager, with a reason, not after payroll left draft.
	reopenBody := map[string]any{"reason": "Koreksi absensi karyawan PVJ"}
	expect(t, h.do(&hr, "POST", "/api/hris/attendance-periods/"+sepID+"/reopen", reopenBody), 403, "")
	expect(t, h.do(&manager, "POST", "/api/hris/attendance-periods/"+sepID+"/reopen", map[string]any{"reason": "x"}), 400,
		"Alasan buka ulang wajib diisi (minimal 5 karakter)")
	fakePeriodRuns = []PeriodRun{{ID: "run-sep", Name: "Payroll September 2026", Status: "completed", Year: 2026, Month: 9}}
	blocked := h.do(&admin, "POST", "/api/hris/attendance-periods/"+sepID+"/reopen", reopenBody)
	expect(t, blocked, 409, "")
	if !strings.Contains(blocked.body["error"].(string), "Payroll September 2026\" sudah disetujui") {
		t.Fatalf("blocked reopen = %s", blocked.raw)
	}
	fakePeriodRuns = []PeriodRun{{ID: "run-sep", Name: "Payroll September 2026", Status: "draft", Year: 2026, Month: 9}}
	reopened := h.do(&manager, "POST", "/api/hris/attendance-periods/"+sepID+"/reopen", reopenBody)
	expect(t, reopened, 200, "")
	if d := reopened.data(); d["status"] != "open" || d["closing_date"] != "2026-10-08" || d["reopen_count"] != 1.0 ||
		d["reopen_reason"] != "Koreksi absensi karyawan PVJ" {
		t.Fatalf("reopen = %s", reopened.raw)
	}
	// September accepts corrections again, then HR closes it by hand.
	expect(t, h.do(&hr, "PUT", "/api/hris/attendance/"+sepRow, map[string]any{"clock_in": "08:20", "reason": "Koreksi jam masuk"}), 200, "")
	closed := h.do(&hr, "POST", "/api/hris/attendance-periods/"+sepID+"/close", nil)
	expect(t, closed, 200, "")
	if d := closed.data(); d["status"] != "closed" || d["close_mode"] != "manual" || d["close_summary"].(map[string]any)["adjusted"] != 1.0 ||
		len(d["events"].([]any)) != 3 {
		t.Fatalf("close = %s", closed.raw)
	}
	expect(t, h.do(&hr, "POST", "/api/hris/attendance-periods/"+sepID+"/close", nil), 409, "")

	// Cut-off 20: the open October now runs 1–20 Oct (September stays frozen
	// as 1–30 Sep) and closes 3 days later.
	expect(t, h.do(&hr, "PUT", "/api/hris/attendance-settings", map[string]any{"cutoff_day": 35, "closing_offset_days": 3}), 400, "")
	saved := h.do(&hr, "PUT", "/api/hris/attendance-settings", map[string]any{"cutoff_day": 20, "closing_offset_days": 3})
	expect(t, saved, 200, "")
	if cur := saved.data()["current_period"].(map[string]any); cur["label"] != "1–20 Okt 2026" || cur["closing_date"] != "2026-11-05" {
		t.Fatalf("after cut-off = %s", saved.raw)
	}
	if got := h.scalar(`SELECT start_date::text || '/' || end_date::text FROM hris.attendance_periods WHERE id = $1`, sepID); got != "2026-09-01/2026-09-30" {
		t.Fatalf("frozen september = %s", got)
	}
	// The preview resolves proposed settings the same way, without saving.
	preview := h.do(&hr, "GET", "/api/hris/attendance-settings?preview_cutoff=10&preview_offset=2", nil)
	expect(t, preview, 200, "")
	pv := preview.data()["preview"].(map[string]any)
	if c := pv["current"].(map[string]any); c["label"] != "1–10 Okt 2026" || c["closing_date"] != "2026-10-12" {
		t.Fatalf("preview = %s", preview.raw)
	}
	if n := pv["next"].(map[string]any); n["label"] != "11 Okt – 10 Nov 2026" {
		t.Fatalf("preview next = %s", preview.raw)
	}
	// 21 Oct now belongs to November (21 Oct – 20 Nov).
	nov, err := periodContaining(context.Background(), h.svc.repo.querier(), "2026-10-21", "2026-10-05")
	if err != nil || nov.Year != 2026 || nov.Month != 11 || nov.Start != "2026-10-21" || nov.End != "2026-11-20" {
		t.Fatalf("november = %+v %v", nov, err)
	}
	// Back to calendar months: the October period keeps following the
	// settings. Rows move one by one, so the constraint is checked once all
	// moved (as at commit in production).
	h.exec(`SET CONSTRAINTS hris.attendance_periods_no_overlap DEFERRED`)
	expect(t, h.do(&hr, "PUT", "/api/hris/attendance-settings", map[string]any{"cutoff_day": 31, "closing_offset_days": 5}), 200, "")
	h.exec(`SET CONSTRAINTS hris.attendance_periods_no_overlap IMMEDIATE`)
	if got := h.scalar(`SELECT start_date::text || '/' || end_date::text FROM hris.attendance_periods WHERE id = $1`, octID); got != "2026-10-01/2026-10-31" {
		t.Fatalf("october back to calendar = %s", got)
	}
}

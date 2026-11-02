package hris

import (
	"testing"
)

// wibClock is a timestamptz column as "HH:MM" WIB for the attendance row.
func (h *harness) wibClock(col, attendanceID string) string {
	h.t.Helper()
	return h.scalar(`SELECT to_char(`+col+` AT TIME ZONE 'Asia/Jakarta', 'HH24:MI') FROM hris.attendance WHERE id = $1`, attendanceID)
}

// EPIC-080 R8: a two-shift day is one attendance row: lateness from shift 1,
// scheduled_end from shift 2, break = both breaks + the gap between them.
func TestClockInTwoShiftDay(t *testing.T) {
	h, _ := newFileHarness(t)
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 0, closing_offset_days = 3`)
	photo := dataURL("image/png", pngBytes(t))

	shiftA := h.scalar(`INSERT INTO hris.shifts (code, name, start_time, end_time, break_minutes, late_tolerance_minutes)
		VALUES ('PAGI4X', 'PAGI4X', '08:00', '12:00', 0, 3) RETURNING id::text`)
	shiftB := h.scalar(`INSERT INTO hris.shifts (code, name, start_time, end_time, break_minutes, late_tolerance_minutes)
		VALUES ('SORE4X', 'SORE4X', '17:00', '21:00', 0, 120) RETURNING id::text`)

	staff, emp1 := h.staff("pos", nil, true, emp{name: "Crew Dua Shift"})
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, shift2_id, batch_id)
		VALUES ($1, '2026-10-06', $2, $3, gen_random_uuid())`, emp1, shiftA, shiftB)

	h.setNow("2026-10-06 08:05")
	in := h.do(&staff, "POST", "/api/hris/attendance", clockInBody(photo, nil, ""))
	expect(t, in, 200, "")
	d := in.data()
	id := d["id"].(string)
	if d["shift_id"] != shiftA {
		t.Fatalf("shift_id = %v, want the first shift %s (%s)", d["shift_id"], shiftA, in.raw)
	}
	if got := h.wibClock("scheduled_start", id); got != "08:00" {
		t.Errorf("scheduled_start = %s, want 08:00", got)
	}
	if got := h.wibClock("scheduled_end", id); got != "21:00" {
		t.Errorf("scheduled_end = %s, want 21:00 (end of shift 2)", got)
	}
	if d["break_minutes"] != float64(300) {
		t.Errorf("break_minutes = %v, want 300 (0 + 0 + 5h gap) (%s)", d["break_minutes"], in.raw)
	}
	// Late by shift 1's tolerance (3 min), not shift 2's (120 min).
	if d["is_late"] != true || d["late_minutes"] != float64(5) || d["late_tolerance_minutes"] != float64(3) {
		t.Errorf("lateness = is_late %v, late_minutes %v, tolerance %v, want true/5/3 (%s)",
			d["is_late"], d["late_minutes"], d["late_tolerance_minutes"], in.raw)
	}

	// 08:05 → 21:05 is 13h; minus the 5h gap = 8h net.
	h.setNow("2026-10-06 21:05")
	out := h.do(&staff, "POST", "/api/hris/attendance", map[string]any{"action": "clock-out", "attendance_id": id, "photo": photo})
	expect(t, out, 200, "")
	if wh := h.scalar(`SELECT work_hours::text FROM hris.attendance WHERE id = $1`, id); wh != "8.00" {
		t.Errorf("work_hours = %s, want 8.00 (%s)", wh, out.raw)
	}
}

// A one-shift day takes its break from the shift master (OPSSAT = 0, OPS = 60),
// not the attendance table default.
func TestClockInOneShiftBreakFromMaster(t *testing.T) {
	h, _ := newFileHarness(t)
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 0, closing_offset_days = 3`)
	photo := dataURL("image/png", pngBytes(t))

	for _, tc := range []struct {
		code, date, now string
		want            float64
	}{
		{"OPSSAT", "2026-10-10", "2026-10-10 08:00", 0},
		{"OPS", "2026-10-09", "2026-10-09 08:00", 60},
	} {
		staff, e := h.staff("pos", nil, true, emp{name: "Crew " + tc.code})
		h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id)
			SELECT $1, $2::date, id, gen_random_uuid() FROM hris.shifts WHERE code = $3`, e, tc.date, tc.code)
		h.setNow(tc.now)
		in := h.do(&staff, "POST", "/api/hris/attendance", clockInBody(photo, nil, ""))
		expect(t, in, 200, "")
		if got := in.data()["break_minutes"]; got != tc.want {
			t.Errorf("%s break_minutes = %v, want %v (%s)", tc.code, got, tc.want, in.raw)
		}
	}
}

// EPIC-080 R6: the ESS home and the HR daily roster name both shifts and show
// both clock ranges; a one-shift day reads as before.
func TestTwoShiftDayDisplay(t *testing.T) {
	h := newHarness(t)
	h.setNow("2026-10-06 07:00")
	hr, _ := h.staff("hrd", scheduleTestMenus, false)
	shiftA := h.scalar(`INSERT INTO hris.shifts (code, name, start_time, end_time, break_minutes)
		VALUES ('PAGI4X', 'Pagi Empat', '08:00', '12:00', 0) RETURNING id::text`)
	shiftB := h.scalar(`INSERT INTO hris.shifts (code, name, start_time, end_time, break_minutes)
		VALUES ('SORE4X', 'Sore Empat', '17:00', '21:00', 0) RETURNING id::text`)

	staff, two := h.staff("pos", nil, true, emp{name: "Crew Tampil Dua"})
	one := h.employee(emp{name: "Crew Tampil Satu"})
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, shift2_id, batch_id)
		VALUES ($1, '2026-10-06', $2, $3, gen_random_uuid())`, two, shiftA, shiftB)
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id)
		VALUES ($1, '2026-10-06', $2, gen_random_uuid())`, one, shiftA)

	me := h.do(&staff, "GET", "/api/hris/me", nil)
	expect(t, me, 200, "")
	ts, _ := me.data()["today_shift"].(map[string]any)
	if ts["name"] != "PAGI4X + SORE4X" || ts["start_time"] != "08:00:00" || ts["end_time"] != "21:00:00" ||
		ts["time_label"] != "08.00–12.00, 17.00–21.00" {
		t.Fatalf("ESS today_shift = %s", me.raw)
	}

	roster := h.do(&hr, "GET", "/api/hris/attendance/daily-roster?date=2026-10-06", nil)
	expect(t, roster, 200, "")
	shifts := map[string]map[string]any{}
	for _, e := range roster.data()["employees"].([]any) {
		m := e.(map[string]any)
		if sh, ok := m["shift"].(map[string]any); ok {
			shifts[m["employee_id"].(string)] = sh
		}
	}
	if sh := shifts[two]; sh["id"] != shiftA || sh["name"] != "PAGI4X + SORE4X" || sh["end_time"] != "21:00:00" ||
		sh["time_label"] != "08.00–12.00, 17.00–21.00" {
		t.Fatalf("roster two-shift = %v", sh)
	}
	if sh := shifts[one]; sh["name"] != "Pagi Empat" || sh["end_time"] != "12:00:00" || sh["time_label"] != "08.00–12.00" {
		t.Fatalf("roster one-shift = %v", sh)
	}
}

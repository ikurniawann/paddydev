package hris

import "testing"

func TestGoLiveSchema(t *testing.T) {
	h := newHarness(t)
	if got := h.scalar(`SELECT code FROM hris.shifts WHERE is_default`); got != "DEF" {
		t.Fatalf("default shift = %q, want DEF", got)
	}
	if got := h.scalar(`SELECT start_time::text || '-' || end_time::text || '/' || break_minutes || '/' || late_tolerance_minutes
		FROM hris.shifts WHERE code = 'NS'`); got != "09:00:00-18:00:00/60/15" {
		t.Fatalf("NS = %q", got)
	}
	if got := h.scalar(`SELECT count(*)::text FROM hris.shifts WHERE code IS NULL`); got != "0" {
		t.Fatalf("shifts without code: %s", got)
	}
	e := h.employee(emp{})
	if got := h.scalar(`SELECT schedule_type FROM hris.employees WHERE id = $1`, e); got != "shift" {
		t.Fatalf("schedule_type default = %q", got)
	}
	// Jadwal per tanggal menang atas pola mingguan yang mulai di hari yang sama.
	ns := h.scalar(`SELECT id::text FROM hris.shifts WHERE code = 'NS'`)
	def := h.scalar(`SELECT id::text FROM hris.shifts WHERE code = 'DEF'`)
	h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from) VALUES ($1, 2, $2, '2026-11-03')`, e, ns)
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id) VALUES ($1, '2026-11-03', $2, gen_random_uuid())`, e, def)
	got := h.scalar(`SELECT shift_id::text FROM hris.v_employee_schedule WHERE employee_id = $1 AND day_of_week = 2
		ORDER BY effective_from DESC, priority LIMIT 1`, e)
	if got != def {
		t.Fatalf("view winner = %s, want per-date %s", got, def)
	}
	if got := h.scalar(`SELECT count(*)::text FROM iam.menus WHERE code = 'hris.kepegawaian.schedules' AND route_path = '/dashboard/hris/schedules'`); got != "1" {
		t.Fatalf("menu rows = %s", got)
	}
}

func TestShiftCodeDefaultsForLegacyInserts(t *testing.T) {
	h := newHarness(t)
	code := h.scalar(`INSERT INTO hris.shifts (name, start_time, end_time) VALUES ('Lama', '08:00', '16:00') RETURNING code`)
	if len(code) < 2 || code[0] != 'S' {
		t.Fatalf("auto code = %q, want S<n>", code)
	}
}

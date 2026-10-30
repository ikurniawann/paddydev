package hris

import (
	"context"
	"testing"
)

func TestPerDateScheduleWinsInReaders(t *testing.T) {
	h := newHarness(t)
	e := h.employee(emp{})
	ns := h.scalar(`SELECT id::text FROM hris.shifts WHERE code = 'NS'`)
	def := h.scalar(`SELECT id::text FROM hris.shifts WHERE code = 'DEF'`)
	// Pola mingguan Selasa = NS; jadwal per tanggal Selasa 2026-10-06 = DEF, 2026-10-13 = libur.
	h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from) VALUES ($1, 2, $2, '2026-01-01')`, e, ns)
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id) VALUES
		($1, '2026-10-06', $2, gen_random_uuid()), ($1, '2026-10-13', NULL, gen_random_uuid())`, e, def)
	ctx := context.Background()

	snap, err := h.svc.shiftSnapshot(ctx, e, "2026-10-06")
	if err != nil || snap == nil || snap.shiftID != def || snap.times.StartTime != "08:00:00" {
		t.Fatalf("snapshot 10-06 = %+v %v, want DEF", snap, err)
	}
	if snap, _ := h.svc.shiftSnapshot(ctx, e, "2026-10-13"); snap != nil {
		t.Fatalf("snapshot 10-13 = %+v, want day off", snap)
	}
	if snap, _ := h.svc.shiftSnapshot(ctx, e, "2026-10-20"); snap == nil || snap.shiftID != ns {
		t.Fatalf("snapshot 10-20 = %+v, want weekly NS", snap)
	}
	rows, err := h.svc.repo.RosterRows(ctx, "2026-10-06", 2, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Str("employee_id") == e && r.Str("scheduled_shift_id") != def {
			t.Fatalf("roster shift = %s, want DEF", r.Str("scheduled_shift_id"))
		}
	}
	ess, err := h.svc.repo.ESSSchedule(ctx, e)
	if err != nil || len(ess) != 3 {
		t.Fatalf("ESS rows = %d %v, want 3 (1 weekly + 2 per-date)", len(ess), err)
	}
}

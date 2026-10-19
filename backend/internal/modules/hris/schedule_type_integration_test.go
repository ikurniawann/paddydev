package hris

import (
	"net/http"
	"testing"
)

func TestScheduleTypeToNonShiftClearsFutureDays(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", scheduleTestMenus, false)
	e := h.employee(emp{})
	def := h.scalar(`SELECT id::text FROM hris.shifts WHERE code = 'DEF'`)
	// fixedNow = Senin 2026-10-05; satu jadwal per tanggal lampau, dua ke depan.
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id) VALUES
		($1, '2026-10-01', $2, gen_random_uuid()), ($1, '2026-10-07', $2, gen_random_uuid()), ($1, '2026-10-08', $2, gen_random_uuid())`, e, def)

	dry := h.do(&hr, http.MethodPost, "/api/hris/employees/schedule-type/bulk", map[string]any{
		"employee_ids": []string{e}, "schedule_type": "non_shift", "effective_from": "2026-10-06", "dry_run": true})
	expect(t, dry, http.StatusOK, "")
	if dry.data()["cleared_days"] != float64(2) {
		t.Fatalf("dry run cleared_days = %v", dry.data()["cleared_days"])
	}
	if h.scalar(`SELECT schedule_type FROM hris.employees WHERE id = $1`, e) != "shift" {
		t.Fatal("dry run must not change anything")
	}

	c := h.do(&hr, http.MethodPut, "/api/hris/employees/"+e+"/schedule-type", map[string]any{
		"schedule_type": "non_shift", "effective_from": "2026-10-06"})
	expect(t, c, http.StatusOK, "")
	if h.scalar(`SELECT schedule_type FROM hris.employees WHERE id = $1`, e) != "non_shift" {
		t.Fatal("type not changed")
	}
	if h.scalar(`SELECT count(*)::text FROM hris.employee_shift_days WHERE employee_id = $1`, e) != "1" {
		t.Fatal("future per-date rows must be deleted, past kept")
	}
	got := h.scalar(`SELECT string_agg(es.day_of_week || ':' || coalesce(s.code, 'L'), ',' ORDER BY es.day_of_week)
		FROM hris.employee_shifts es LEFT JOIN hris.shifts s ON s.id = es.shift_id
		WHERE es.employee_id = $1 AND es.effective_from = '2026-10-06'`, e)
	if got != "1:NS,2:NS,3:NS,4:NS,5:NS,6:L,7:L" {
		t.Fatalf("fixed pattern = %s", got)
	}
	if h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE entity = 'hris.employees' AND entity_id = $1 AND action = 'employee.schedule_type'`, e) != "1" {
		t.Fatal("audit missing")
	}

	back := h.do(&hr, http.MethodPut, "/api/hris/employees/"+e+"/schedule-type", map[string]any{
		"schedule_type": "shift", "effective_from": "2026-10-12"})
	expect(t, back, http.StatusOK, "")
	warnings, _ := back.data()["warnings"].([]any)
	if len(warnings) != 1 || warnings[0] != "Karyawan belum punya jadwal shift mulai 12/10/2026; tugaskan di Jadwal Kerja" {
		t.Fatalf("warnings = %v", warnings)
	}
	if h.scalar(`SELECT count(*)::text FROM hris.employee_shifts WHERE employee_id = $1 AND (effective_to IS NULL OR effective_to >= '2026-10-12')`, e) != "0" {
		t.Fatal("fixed pattern must stop on 2026-10-12")
	}
}

func TestApplyFixedScheduleRejectsShiftEmployees(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", scheduleTestMenus, false)
	shiftEmp := h.employee(emp{name: "Kasir Satu"})
	c := h.do(&hr, http.MethodPost, "/api/hris/schedules/fixed", map[string]any{
		"employee_ids": []string{shiftEmp}, "effective_from": "2026-10-06"})
	expect(t, c, http.StatusBadRequest, "Kasir Satu bertipe Shift — jadwal tetap hanya untuk karyawan Non-Shift")
	past := h.do(&hr, http.MethodPost, "/api/hris/schedules/fixed", map[string]any{
		"employee_ids": []string{shiftEmp}, "effective_from": "2026-10-01"})
	expect(t, past, http.StatusBadRequest, "Tanggal mulai tidak boleh sebelum hari ini")
}

func TestWeeklyPatternRejectedForNonShift(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", scheduleTestMenus, false)
	e := h.employee(emp{name: "Kantor Pusat"})
	h.exec(`UPDATE hris.employees SET schedule_type = 'non_shift' WHERE id = $1`, e)
	days := []map[string]any{}
	for d := 1; d <= 7; d++ {
		days = append(days, map[string]any{"day_of_week": d, "shift_id": nil})
	}
	c := h.do(&hr, http.MethodPut, "/api/hris/employees/"+e+"/shifts", map[string]any{"effective_from": "2026-10-06", "days": days})
	expect(t, c, http.StatusBadRequest, "Kantor Pusat Non-Shift memakai jadwal tetap — ubah tipe jadwal ke Shift dulu")
}

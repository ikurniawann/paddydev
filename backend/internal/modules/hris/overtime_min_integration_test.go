package hris

import "testing"

// Tabel payslip Paddy: lembur minimal 1 jam (EPIC-059 R9).
func TestOvertimeMinimumOneHour(t *testing.T) {
	h := newHarness(t)
	staff, _ := h.staff("pos", nil, true)
	expect(t, h.do(&staff, "POST", "/api/hris/overtime", map[string]any{
		"date": "2031-03-04", "start_time": "17:00", "end_time": "17:45", "reason": "Rapat"}), 400,
		"Durasi lembur minimal 1 jam")
	expect(t, h.do(&staff, "POST", "/api/hris/overtime", map[string]any{
		"date": "2031-03-04", "start_time": "17:00", "end_time": "18:00", "reason": "Rapat"}), 200, "")
}

package hris

import "testing"

// Tabel payslip Paddy: lembur minimal 1 jam (EPIC-059 R9).
func TestOvertimeMinimumOneHour(t *testing.T) {
	h, _ := newFileHarness(t)
	staff, _ := h.staff("pos", nil, true)
	expect(t, h.do(&staff, "POST", "/api/hris/overtime", h.otSelfBody(&staff, "2026-10-01", "17:00", "17:45")), 400,
		"Durasi lembur minimal 1 jam")
	expect(t, h.do(&staff, "POST", "/api/hris/overtime", h.otSelfBody(&staff, "2026-10-01", "17:00", "18:00")), 200, "")
}

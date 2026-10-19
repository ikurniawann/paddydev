package hris

import (
	"net/http"
	"testing"

	"nuhabit/backend/internal/platform/iam"
)

func TestShiftCodes(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", iam.HrisKepegawaian, false)
	c := h.do(&hr, http.MethodPost, "/api/hris/shifts", map[string]any{
		"name": "SPG Pagi", "code": " spg1x ", "start_time": "08:00", "end_time": "16:00"})
	expect(t, c, http.StatusCreated, "")
	if c.data()["code"] != "SPG1X" {
		t.Fatalf("code = %v", c.data()["code"])
	}
	dup := h.do(&hr, http.MethodPost, "/api/hris/shifts", map[string]any{
		"name": "Lain", "code": "spg1x", "start_time": "09:00", "end_time": "17:00"})
	expect(t, dup, http.StatusConflict, "Kode shift SPG1X sudah dipakai")
	bad := h.do(&hr, http.MethodPost, "/api/hris/shifts", map[string]any{
		"name": "Lain", "code": "SPG 1", "start_time": "09:00", "end_time": "17:00"})
	expect(t, bad, http.StatusBadRequest, "Kode shift 1–12 huruf/angka tanpa spasi")
	def := h.scalar(`SELECT id::text FROM hris.shifts WHERE code = 'DEF'`)
	del := h.do(&hr, http.MethodDelete, "/api/hris/shifts/"+def, nil)
	expect(t, del, http.StatusBadRequest, "Shift bawaan DEF/NS tidak bisa dihapus atau dinonaktifkan")
	off := h.do(&hr, http.MethodPatch, "/api/hris/shifts/"+def, map[string]any{"is_active": false})
	expect(t, off, http.StatusBadRequest, "Shift bawaan DEF/NS tidak bisa dihapus atau dinonaktifkan")
	// Shift dipakai jadwal per tanggal ke depan tidak bisa dinonaktifkan.
	id := c.data()["id"].(string)
	e := h.employee(emp{})
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id) VALUES ($1, '2026-12-01', $2, gen_random_uuid())`, e, id)
	used := h.do(&hr, http.MethodPatch, "/api/hris/shifts/"+id, map[string]any{"is_active": false})
	expect(t, used, http.StatusBadRequest, "Shift masih dipakai 1 jadwal mulai hari ini — ganti dulu jadwalnya")
}

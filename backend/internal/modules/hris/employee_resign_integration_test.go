package hris

import (
	"net/http"
	"testing"

	"nuhabit/backend/internal/platform/iam"
)

func TestResignEmployee(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", iam.HrisKepegawaian, false)
	login, e := h.staff("employee", nil, true, emp{name: "Rina Resign", join: "2025-01-10"})
	branch := h.scalar(`SELECT id::text FROM configuration.branches LIMIT 1`)
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date) VALUES ($1, $2, true, '2025-01-10')`, e, branch)
	def := h.scalar(`SELECT id::text FROM hris.shifts WHERE code = 'DEF'`)
	h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from) VALUES ($1, 1, $2, '2025-01-10')`, e, def)
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id) VALUES ($1, '2026-10-20', $2, gen_random_uuid())`, e, def)

	bad := h.do(&hr, http.MethodPost, "/api/hris/employees/"+e+"/resign", map[string]any{"effective_date": "2024-12-31", "resign_type": "resign"})
	expect(t, bad, http.StatusBadRequest, "Tanggal efektif tidak boleh sebelum tanggal masuk (10/01/2025)")

	c := h.do(&hr, http.MethodPost, "/api/hris/employees/"+e+"/resign", map[string]any{
		"effective_date": "2026-10-15", "resign_type": "resign", "reason": "Pindah kota"})
	expect(t, c, http.StatusOK, "")
	row := h.scalar(`SELECT employment_status || '|' || is_active || '|' || end_date::text FROM hris.employees WHERE id = $1`, e)
	if row != "resigned|false|2026-10-15" {
		t.Fatalf("employee = %s", row)
	}
	if h.scalar(`SELECT end_date::text FROM hris.employee_placements WHERE employee_id = $1`, e) != "2026-10-15" {
		t.Fatal("placement not closed")
	}
	if h.scalar(`SELECT effective_to::text FROM hris.employee_shifts WHERE employee_id = $1`, e) != "2026-10-15" {
		t.Fatal("weekly pattern not closed")
	}
	if h.scalar(`SELECT count(*)::text FROM hris.employee_shift_days WHERE employee_id = $1`, e) != "0" {
		t.Fatal("per-date rows after resign date must be removed")
	}
	if h.scalar(`SELECT (banned_until IS NOT NULL)::text FROM auth.users WHERE id = $1`, login.UserID) != "true" {
		t.Fatal("login account must be blocked")
	}
	if h.scalar(`SELECT change_type || '|' || coalesce(reason, '') FROM hris.employment_history WHERE employee_id = $1 AND change_type = 'resign'`, e) != "resign|Pindah kota" {
		t.Fatal("history missing")
	}
	if h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE entity_id = $1 AND action = 'employee.resign'`, e) != "1" {
		t.Fatal("audit missing")
	}
	again := h.do(&hr, http.MethodPost, "/api/hris/employees/"+e+"/resign", map[string]any{"effective_date": "2026-10-15", "resign_type": "resign"})
	expect(t, again, http.StatusBadRequest, "Karyawan sudah nonaktif")
}

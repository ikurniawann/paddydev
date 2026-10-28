package hris

import (
	"net/http"
	"testing"
)

// usage returns employees_using per shift id from GET /api/hris/shifts.
func shiftUsage(t *testing.T, c call) map[string]any {
	t.Helper()
	out := map[string]any{}
	for _, it := range c.list() {
		m := it.(map[string]any)
		out[m["id"].(string)] = m["employees_using"]
	}
	return out
}

func TestShiftListEmployeesUsing(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", scheduleTestMenus, false)
	used := h.scalar(`INSERT INTO hris.shifts (name, start_time, end_time) VALUES ('Pakai Uji', '08:00', '17:00') RETURNING id::text`)
	idle := h.scalar(`INSERT INTO hris.shifts (name, start_time, end_time) VALUES ('Nganggur Uji', '09:00', '18:00') RETURNING id::text`)

	byDay := h.employee(emp{name: "Pakai Per Tanggal"})
	byPattern := h.employee(emp{name: "Pakai Pola"})
	both := h.employee(emp{name: "Pakai Dua Sumber"})
	inactive := h.employee(emp{name: "Nonaktif", inactive: true})
	pastOnly := h.employee(emp{name: "Hanya Lampau"})

	// fixedNow = Senin 2026-10-05 WIB.
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id) VALUES
		($1, '2026-10-05', $4, gen_random_uuid()),
		($2, '2026-10-06', $4, gen_random_uuid()),
		($3, '2026-10-01', $4, gen_random_uuid())`, byDay, both, pastOnly, used)
	h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from, effective_to) VALUES
		($1, 2, $5, '2026-01-01', NULL),
		($2, 2, $5, '2026-01-01', '2026-12-31'),
		($3, 3, $5, '2026-01-01', NULL),
		($4, 1, $5, '2026-01-01', '2026-10-04')`, byPattern, both, inactive, pastOnly, used)
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id) VALUES ($1, '2026-10-07', $2, gen_random_uuid())`, inactive, used)

	// Without the param the old shape is kept.
	plain := h.do(&hr, http.MethodGet, "/api/hris/shifts", nil)
	expect(t, plain, http.StatusOK, "")
	for _, it := range plain.list() {
		if _, has := it.(map[string]any)["employees_using"]; has {
			t.Fatalf("employees_using present without with_usage: %s", plain.raw)
		}
	}

	c := h.do(&hr, http.MethodGet, "/api/hris/shifts?with_usage=true", nil)
	expect(t, c, http.StatusOK, "")
	got := shiftUsage(t, c)
	// byDay + byPattern + both (counted once across both sources); not inactive, not pastOnly.
	if got[used] != float64(3) {
		t.Fatalf("used shift employees_using = %v, want 3 (%s)", got[used], c.raw)
	}
	if got[idle] != float64(0) {
		t.Fatalf("idle shift employees_using = %v, want 0", got[idle])
	}
	for id, v := range got {
		if _, isNum := v.(float64); !isNum {
			t.Fatalf("shift %s employees_using = %v, want a number", id, v)
		}
	}
}

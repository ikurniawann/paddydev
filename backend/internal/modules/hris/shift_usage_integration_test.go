package hris

import (
	"net/http"
	"testing"

	"nuhabit/backend/internal/platform/iam"
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
	inactiveOnly := h.scalar(`INSERT INTO hris.shifts (name, start_time, end_time) VALUES ('Pakai Nonaktif Uji', '10:00', '19:00') RETURNING id::text`)

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
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id) VALUES ($1, '2026-10-08', $2, gen_random_uuid())`, inactive, inactiveOnly)

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
	// Same predicate as guardShiftDeactivate: byDay + byPattern + both
	// (counted once across both sources) + inactive (a future row still
	// blocks deactivation); not pastOnly.
	if got[used] != float64(4) {
		t.Fatalf("used shift employees_using = %v, want 4 (%s)", got[used], c.raw)
	}
	// A shift held only by an inactive employee's future row is "in use":
	// the list says so and the server refuses to turn it off.
	if got[inactiveOnly] != float64(1) {
		t.Fatalf("inactive-only shift employees_using = %v, want 1 (%s)", got[inactiveOnly], c.raw)
	}
	off := h.do(&hr, http.MethodPatch, "/api/hris/shifts/"+inactiveOnly, map[string]any{"is_active": false})
	expect(t, off, http.StatusBadRequest, "Shift masih dipakai 1 jadwal mulai hari ini — ganti dulu jadwalnya")
	if got[idle] != float64(0) {
		t.Fatalf("idle shift employees_using = %v, want 0", got[idle])
	}
	for id, v := range got {
		if _, isNum := v.(float64); !isNum {
			t.Fatalf("shift %s employees_using = %v, want a number", id, v)
		}
	}
}

// A shift held only as the SECOND shift of a day (per-date or weekly) still
// counts as in use: the list reports it, deactivation is refused and DELETE
// falls back to deactivate-only instead of tripping the shift2_id FK.
func TestShiftUsageCountsSecondShift(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", scheduleTestMenus, false)
	first := h.scalar(`INSERT INTO hris.shifts (name, start_time, end_time) VALUES ('Pertama Uji', '08:00', '12:00') RETURNING id::text`)
	secDay := h.scalar(`INSERT INTO hris.shifts (name, start_time, end_time) VALUES ('Kedua Tanggal Uji', '17:00', '21:00') RETURNING id::text`)
	secWeek := h.scalar(`INSERT INTO hris.shifts (name, start_time, end_time) VALUES ('Kedua Pola Uji', '18:00', '22:00') RETURNING id::text`)
	secPast := h.scalar(`INSERT INTO hris.shifts (name, start_time, end_time) VALUES ('Kedua Lampau Uji', '19:00', '23:00') RETURNING id::text`)

	byDay := h.employee(emp{name: "Kedua Per Tanggal"})
	byPattern := h.employee(emp{name: "Kedua Pola"})
	pastOnly := h.employee(emp{name: "Kedua Lampau"})

	// fixedNow = Senin 2026-10-05 WIB.
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, shift2_id, batch_id) VALUES
		($1, '2026-10-06', $3, $4, gen_random_uuid()),
		($2, '2026-10-01', $3, $5, gen_random_uuid())`, byDay, pastOnly, first, secDay, secPast)
	h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, shift2_id, effective_from, effective_to)
		VALUES ($1, 2, $2, $3, '2026-01-01', NULL)`, byPattern, first, secWeek)

	c := h.do(&hr, http.MethodGet, "/api/hris/shifts?with_usage=true", nil)
	expect(t, c, http.StatusOK, "")
	got := shiftUsage(t, c)
	if got[secDay] != float64(1) {
		t.Fatalf("second shift (per-date) employees_using = %v, want 1 (%s)", got[secDay], c.raw)
	}
	if got[secWeek] != float64(1) {
		t.Fatalf("second shift (weekly) employees_using = %v, want 1 (%s)", got[secWeek], c.raw)
	}
	if got[secPast] != float64(0) {
		t.Fatalf("second shift on a past date employees_using = %v, want 0 (%s)", got[secPast], c.raw)
	}

	// Same refusal as a first-shift use.
	off := h.do(&hr, http.MethodPatch, "/api/hris/shifts/"+secDay, map[string]any{"is_active": false})
	expect(t, off, http.StatusBadRequest, "Shift masih dipakai 1 jadwal mulai hari ini — ganti dulu jadwalnya")
	offWeek := h.do(&hr, http.MethodPatch, "/api/hris/shifts/"+secWeek, map[string]any{"is_active": false})
	expect(t, offWeek, http.StatusBadRequest, "Shift masih dipakai 1 jadwal mulai hari ini — ganti dulu jadwalnya")

	// DELETE hits the same guard while the second shift is scheduled from
	// today on; once only past rows remain it deactivates instead of tripping
	// the shift2_id FK with a raw 500.
	h.grantDelete(hr, iam.LeafShift)
	for _, id := range []string{secDay, secWeek} {
		del := h.do(&hr, http.MethodDelete, "/api/hris/shifts/"+id+"?reason=Uji%20shift%20kedua", nil)
		expect(t, del, http.StatusBadRequest, "Shift masih dipakai 1 jadwal mulai hari ini — ganti dulu jadwalnya")
	}
	del := h.do(&hr, http.MethodDelete, "/api/hris/shifts/"+secPast+"?reason=Uji%20shift%20kedua", nil)
	expect(t, del, http.StatusOK, "")
	if active := h.scalar(`SELECT is_active::text FROM hris.shifts WHERE id = $1`, secPast); active != "false" {
		t.Fatalf("past-only second shift is_active = %s after DELETE, want false (kept, deactivated)", active)
	}
}

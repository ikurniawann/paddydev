package hris

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"nuhabit/backend/internal/modules/hris/domain"
)

// weekPattern is "1:NS,…,7:L" of the employee's weekly rows from `from`.
func (h *harness) weekPattern(employee, from string) string {
	h.t.Helper()
	return h.scalar(`SELECT string_agg(es.day_of_week || ':' || coalesce(s.code, 'L'), ',' ORDER BY es.day_of_week)
		FROM hris.employee_shifts es LEFT JOIN hris.shifts s ON s.id = es.shift_id
		WHERE es.employee_id = $1 AND es.effective_from = $2::date`, employee, from)
}

const (
	hqWeek  = "1:NS,2:NS,3:NS,4:NS,5:NS,6:L,7:L"
	opsWeek = "1:OPS,2:OPS,3:OPS,4:OPS,5:OPS,6:OPSSAT,7:L"
)

// EPIC-078 R1–R2: the shift master carries the TnC hours — HQ late after
// 09.00 sharp, Operations after 08.10 (Mon–Fri 08–16, Sat 08–14).
func TestWorkHoursShifts(t *testing.T) {
	h := newHarness(t)
	got := h.scalar(`SELECT string_agg(code || ' ' || to_char(start_time, 'HH24:MI') || '-' || to_char(end_time, 'HH24:MI')
		|| ' tol' || late_tolerance_minutes || ' brk' || break_minutes || ' ' || is_active, '; ' ORDER BY code)
		FROM hris.shifts WHERE code IN ('NS', 'OPS', 'OPSSAT')`)
	if got != "NS 09:00-18:00 tol0 brk60 true; OPS 08:00-16:00 tol10 brk60 true; OPSSAT 08:00-14:00 tol10 brk0 true" {
		t.Fatalf("shifts = %s", got)
	}
	times := func(code string) domain.ShiftTimes {
		tol, _ := strconv.Atoi(h.scalar(`SELECT late_tolerance_minutes::text FROM hris.shifts WHERE code = $1`, code))
		return domain.ShiftTimes{StartTime: h.scalar(`SELECT start_time::text FROM hris.shifts WHERE code = $1`, code),
			EndTime: h.scalar(`SELECT end_time::text FROM hris.shifts WHERE code = $1`, code), LateToleranceMinutes: tol}
	}
	wib := time.FixedZone("WIB", 7*3600)
	at := func(day, hm string) time.Time {
		v, _ := time.ParseInLocation("2006-01-02 15:04:05", day+" "+hm, wib)
		return v
	}
	for _, c := range []struct {
		code, day, clock string
		late             bool
		minutes          int
	}{
		{"NS", "2026-11-02", "08:00:00", false, 0},
		{"NS", "2026-11-02", "09:00:00", false, 0},
		{"NS", "2026-11-02", "09:00:30", true, 1},
		{"NS", "2026-11-02", "09:25:00", true, 25},
		{"OPS", "2026-11-02", "08:10:00", false, 0},
		{"OPS", "2026-11-02", "08:11:00", true, 11},
		{"OPSSAT", "2026-11-07", "08:10:00", false, 0},
		{"OPSSAT", "2026-11-07", "08:30:00", true, 30},
	} {
		late, minutes := domain.Lateness(at(c.day, c.clock), c.day, times(c.code))
		if late != c.late || minutes != c.minutes {
			t.Errorf("%s %s = %v/%d, want %v/%d", c.code, c.clock, late, minutes, c.late, c.minutes)
		}
	}
}

// R3: the fixed schedule follows the department — Warehouse gets the
// Operations week, every other department the HQ week.
func TestFixedScheduleByDepartment(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", scheduleTestMenus, false)
	wh := h.department(" warehouse ")
	fin := h.department("Finance Uji")
	gudang := h.employee(emp{name: "Staf Gudang", dept: &wh})
	kantor := h.employee(emp{name: "Staf Kantor", dept: &fin})
	for _, e := range []string{gudang, kantor} {
		c := h.do(&hr, http.MethodPut, "/api/hris/employees/"+e+"/schedule-type", map[string]any{
			"schedule_type": "non_shift", "effective_from": "2026-10-06"})
		expect(t, c, http.StatusOK, "")
	}
	if got := h.weekPattern(gudang, "2026-10-06"); got != opsWeek {
		t.Errorf("warehouse = %s", got)
	}
	if got := h.weekPattern(kantor, "2026-10-06"); got != hqWeek {
		t.Errorf("HQ = %s", got)
	}

	// Re-applying for the branch's/ids' Non-Shift staff audits the label.
	c := h.do(&hr, http.MethodPost, "/api/hris/schedules/fixed", map[string]any{
		"employee_ids": []string{gudang}, "effective_from": "2026-10-12"})
	expect(t, c, http.StatusOK, "")
	if got := h.weekPattern(gudang, "2026-10-12"); got != opsWeek {
		t.Errorf("re-applied warehouse = %s", got)
	}
	if got := h.scalar(`SELECT after->>'pattern' FROM audit.audit_log WHERE entity_id = $1 AND action = 'schedule.fixed_apply'`, gudang); got != patternOperations {
		t.Errorf("audit pattern = %q", got)
	}
}

// R4–R5: the one-off rule application at deploy — Warehouse Non-Shift and
// unscheduled HQ/Warehouse Shift staff get their fixed week; store crews,
// staff with a schedule, staff without a department and inactive staff are
// left alone; nothing before p_from changes.
func TestApplyWorkHoursRule(t *testing.T) {
	h := newHarness(t)
	wh := h.department("Warehouse Uji")
	h.exec(`UPDATE hris.departments SET name = 'WAREHOUSE' WHERE id = $1`, wh)
	fin := h.department("Finance Uji")
	retail := h.department("Retail Uji")
	h.exec(`UPDATE hris.departments SET name = 'Retail Operational ' WHERE id = $1`, retail)
	ns := h.scalar(`SELECT id::text FROM hris.shifts WHERE code = 'NS'`)
	pagi := h.scalar(`SELECT id::text FROM hris.shifts WHERE code = 'DEF'`)

	gudangNS := h.employee(emp{name: "Gudang NS", dept: &wh})
	h.exec(`UPDATE hris.employees SET schedule_type = 'non_shift' WHERE id = $1`, gudangNS)
	h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from)
		SELECT $1, d, CASE WHEN d <= 5 THEN $2::uuid END, '2026-10-28' FROM generate_series(1, 7) d`, gudangNS, ns)
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id) VALUES
		($1, '2026-11-01', $2, gen_random_uuid()), ($1, '2026-11-04', $2, gen_random_uuid())`, gudangNS, pagi)
	gudangShift := h.employee(emp{name: "Gudang Tanpa Jadwal", dept: &wh})
	hqShift := h.employee(emp{name: "Finance Tanpa Jadwal", dept: &fin})
	hqScheduled := h.employee(emp{name: "Finance Berjadwal", dept: &fin})
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id) VALUES ($1, '2026-11-05', $2, gen_random_uuid())`, hqScheduled, pagi)
	hqNS := h.employee(emp{name: "Finance NS", dept: &fin})
	h.exec(`UPDATE hris.employees SET schedule_type = 'non_shift' WHERE id = $1`, hqNS)
	store := h.employee(emp{name: "Kru Toko", dept: &retail})
	noDept := h.employee(emp{name: "Tanpa Departemen"})
	gone := h.employee(emp{name: "Finance Nonaktif", dept: &fin, inactive: true})

	got := h.scalar(`SELECT string_agg(full_name || '=' || type_changed, ', ' ORDER BY full_name)
		FROM hris.apply_work_hours_rule('2026-11-02', 'Sistem uji')`)
	if got != "Finance Tanpa Jadwal=true, Gudang NS=false, Gudang Tanpa Jadwal=true" {
		t.Fatalf("applied = %s", got)
	}
	for e, want := range map[string]string{gudangNS: opsWeek, gudangShift: opsWeek, hqShift: hqWeek} {
		if got := h.weekPattern(e, "2026-11-02"); got != want {
			t.Errorf("%s pattern = %s, want %s", e, got, want)
		}
		if h.scalar(`SELECT schedule_type FROM hris.employees WHERE id = $1`, e) != "non_shift" {
			t.Errorf("%s not Non-Shift", e)
		}
	}
	// The old NS week ends the day before; per-date rows before p_from stay.
	if got := h.scalar(`SELECT effective_to::text FROM hris.employee_shifts WHERE employee_id = $1 AND effective_from = '2026-10-28' LIMIT 1`, gudangNS); got != "2026-11-01" {
		t.Errorf("old week ends %s", got)
	}
	if got := h.scalar(`SELECT string_agg(work_date::text, ',') FROM hris.employee_shift_days WHERE employee_id = $1`, gudangNS); got != "2026-11-01" {
		t.Errorf("per-date rows = %s", got)
	}
	for _, e := range []string{hqScheduled, hqNS, store, noDept, gone} {
		if n := h.scalar(`SELECT count(*)::text FROM hris.employee_shifts WHERE employee_id = $1`, e); n != "0" {
			t.Errorf("%s got a pattern", e)
		}
	}
	if h.scalar(`SELECT schedule_type FROM hris.employees WHERE id = $1`, store) != "shift" {
		t.Error("store crew changed type")
	}
	if got := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE actor_name = 'Sistem uji'
		AND entity_id = ANY(ARRAY[$1, $2, $3])`, gudangNS, gudangShift, hqShift); got != "5" {
		t.Errorf("audit rows = %s (want 3 fixed_apply + 2 schedule_type)", got)
	}
}

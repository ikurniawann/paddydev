package hris

import (
	"strings"
	"testing"
)

// EPIC-079 R8: Host Live, Production and CRM titles move to the shift system
// from p_from — fixed week stopped the day before, audited, and never handed
// a fixed week again by the targets preview or a department move.
func TestShiftSystemTitlesBecomeShift(t *testing.T) {
	h := newHarness(t)
	mkt := h.department("Marketing Uji")
	wh := h.department("Warehouse Uji")
	h.exec(`UPDATE hris.departments SET name = 'WAREHOUSE' WHERE id = $1`, wh)
	fin := h.department("Finance Uji")
	ns := h.scalar(`SELECT id::text FROM hris.shifts WHERE code = 'NS'`)
	ops := h.scalar(`SELECT id::text FROM hris.shifts WHERE code = 'OPS'`)

	title := func(name string) string {
		return h.scalar(`INSERT INTO hris.positions (title) VALUES ($1) RETURNING id::text`, name)
	}
	nonShift := func(name string, dept *string, titleID, shiftID string) string {
		e := h.employee(emp{name: name, dept: dept})
		h.exec(`UPDATE hris.employees SET schedule_type = 'non_shift', job_title_id = $2 WHERE id = $1`, e, titleID)
		// A running week since 2026-10-28 and one starting after p_from.
		h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from)
			SELECT $1, d, CASE WHEN d <= 5 THEN $2::uuid END, '2026-10-28' FROM generate_series(1, 7) d`, e, shiftID)
		h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from)
			SELECT $1, d, CASE WHEN d <= 5 THEN $2::uuid END, '2026-11-09' FROM generate_series(1, 7) d`, e, shiftID)
		return e
	}
	host := nonShift("Host Live Staff Uji", &mkt, title("Host Live Staff Uji"), ns)
	prod := nonShift("Production Staff Uji", &wh, title("Production Staff Uji"), ops)
	crm := nonShift("CRM Supervisor Uji", &mkt, title("CRM Supervisor Uji"), ns)
	finance := nonShift("Finance Staff Uji", &fin, title("Finance Staff Uji"), ns)
	gone := nonShift("Host Live Nonaktif", &mkt, title("Host Live Nonaktif"), ns)
	h.exec(`UPDATE hris.employees SET is_active = false WHERE id = $1`, gone)

	got := h.scalar(`SELECT hris.apply_shift_system_titles('2026-11-02', 'Sistem uji')::text`)
	audits := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE actor_name = 'Sistem uji'`)
	if got != audits {
		t.Fatalf("returned %s, audit rows %s", got, audits)
	}
	for _, e := range []string{host, prod, crm} {
		if h.scalar(`SELECT schedule_type FROM hris.employees WHERE id = $1`, e) != "shift" {
			t.Errorf("%s not Shift", e)
		}
		if n := h.scalar(`SELECT count(*)::text FROM hris.employee_shifts WHERE employee_id = $1 AND effective_from >= '2026-11-02'`, e); n != "0" {
			t.Errorf("%s keeps %s weekly rows from p_from", e, n)
		}
		if n := h.scalar(`SELECT count(*)::text FROM hris.employee_shifts WHERE employee_id = $1
			AND (effective_to IS NULL OR effective_to >= '2026-11-02')`, e); n != "0" {
			t.Errorf("%s still has %s open weekly rows", e, n)
		}
		if got := h.scalar(`SELECT DISTINCT effective_to::text FROM hris.employee_shifts WHERE employee_id = $1`, e); got != "2026-11-01" {
			t.Errorf("%s old week ends %s", e, got)
		}
		a := h.scalar(`SELECT entity_label || '|' || before::text || '|' || after::text || '|' || reason FROM audit.audit_log
			WHERE actor_name = 'Sistem uji' AND action = 'employee.schedule_type' AND entity = 'hris.employees' AND entity_id = $1`, e)
		if !strings.Contains(a, `"non_shift"`) || !strings.Contains(a, `"shift"`) || !strings.Contains(a, `2026-11-02`) ||
			!strings.HasSuffix(a, "|Sistem shift: Host Live/Produksi/CRM (EPIC-079)") {
			t.Errorf("%s audit = %s", e, a)
		}
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE actor_name = 'Sistem uji'
		AND entity_id = ANY(ARRAY[$1, $2, $3])`, host, prod, crm); n != "3" {
		t.Errorf("audit rows for the three = %s", n)
	}
	// Finance and inactive staff are untouched.
	if h.scalar(`SELECT schedule_type FROM hris.employees WHERE id = $1`, finance) != "non_shift" ||
		h.scalar(`SELECT count(*)::text FROM hris.employee_shifts WHERE employee_id = $1 AND effective_to IS NULL`, finance) != "14" {
		t.Errorf("Finance staff changed")
	}
	if h.scalar(`SELECT schedule_type FROM hris.employees WHERE id = $1`, gone) != "non_shift" {
		t.Errorf("inactive staff converted")
	}

	// The EPIC-078 preview keeps them out even though they are Shift with no schedule.
	targets := h.scalar(`SELECT string_agg(employee_id::text, ',') FROM hris.work_hours_rule_targets('2026-11-02')`)
	for _, e := range []string{host, prod, crm} {
		if strings.Contains(targets, e) {
			t.Errorf("%s is a work-hours target", e)
		}
	}

	// Moving into/out of Warehouse hands a Non-Shift employee of these titles no fixed week,
	// and a Non-Shift Warehouse employee of these titles is no target either.
	h.exec(`UPDATE hris.employees SET schedule_type = 'non_shift' WHERE id = $1`, prod)
	h.exec(`UPDATE hris.employee_shifts SET effective_to = NULL WHERE employee_id = $1`, prod)
	if strings.Contains(h.scalar(`SELECT string_agg(employee_id::text, ',') FROM hris.work_hours_rule_targets('2026-11-02')`), prod) {
		t.Errorf("Non-Shift Warehouse Production staff is a work-hours target")
	}
	h.exec(`UPDATE hris.employees SET department_id = $2 WHERE id = $1`, prod, mkt)
	if n := h.scalar(`SELECT count(*)::text FROM hris.employee_shifts WHERE employee_id = $1
		AND effective_from = (now() AT TIME ZONE 'Asia/Jakarta')::date`, prod); n != "0" {
		t.Errorf("department move gave %s fixed-week rows", n)
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE entity_id = $1 AND reason = 'Pindah departemen (EPIC-078)'`, prod); n != "0" {
		t.Errorf("department move audits = %s", n)
	}
}

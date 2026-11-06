package hris

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// EPIC-095 R4/R7: the job applies a scheduled resign the day after the last
// working day, once; an employee already inactive gets the schedule cancelled.

func wib(t *testing.T, s string) time.Time {
	t.Helper()
	at, err := time.ParseInLocation("2006-01-02 15:04", s, wibZone)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

func TestRunScheduledResigns(t *testing.T) {
	h := newHarness(t)
	f := newResignFixture(h)
	def := h.scalar(`SELECT id::text FROM hris.shifts WHERE code = 'DEF'`)
	for _, d := range []string{"2026-11-19", "2026-11-20", "2026-11-21"} {
		h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id) VALUES ($1, $2::date, $3, gen_random_uuid())`, f.e, d, def)
	}
	expect(t, h.do(&f.hr, http.MethodPost, resignURL(f.e), map[string]any{"effective_date": "2026-11-20", "resign_type": "resign"}), http.StatusOK, "")
	ctx := context.Background()

	applied, cancelled, err := h.svc.RunScheduledResigns(ctx, wib(t, "2026-11-20 23:00"))
	if err != nil || applied != 0 || cancelled != 0 {
		t.Fatalf("on the last working day: applied %d cancelled %d err %v", applied, cancelled, err)
	}
	if got := h.scalar(`SELECT is_active::text FROM hris.employees WHERE id = $1`, f.e); got != "true" {
		t.Fatalf("is_active on the last day = %s", got)
	}

	applied, _, err = h.svc.RunScheduledResigns(ctx, wib(t, "2026-11-21 00:05"))
	if err != nil || applied != 1 {
		t.Fatalf("next day: applied %d err %v", applied, err)
	}
	if got := h.scalar(`SELECT is_active::text || '|' || end_date::text FROM hris.employees WHERE id = $1`, f.e); got != "false|2026-11-20" {
		t.Fatalf("employee = %s", got)
	}
	if got := h.scalar(`SELECT string_agg(work_date::text, ',' ORDER BY work_date) FROM hris.employee_shift_days WHERE employee_id = $1`, f.e); got != "2026-11-19,2026-11-20" {
		t.Fatalf("shift days = %s, want 19 and 20 Nov", got)
	}
	if got := h.scalar(`SELECT end_date::text FROM hris.employee_placements WHERE employee_id = $1`, f.e); got != "2026-11-20" {
		t.Fatalf("placement end = %s", got)
	}
	if got := h.scalar(`SELECT (banned_until IS NOT NULL)::text FROM auth.users WHERE id = $1`, f.login.UserID); got != "true" {
		t.Fatal("login must be blocked")
	}
	hrEmp := h.scalar(`SELECT id::text FROM hris.employees WHERE user_id = $1`, f.hr.UserID)
	if got := h.scalar(`SELECT coalesce(recorded_by::text, '') FROM hris.employment_history WHERE employee_id = $1 AND change_type = 'resign'`, f.e); got != hrEmp {
		t.Fatalf("recorded_by = %q, want the HR who scheduled (%s)", got, hrEmp)
	}
	if got := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE entity_id = $1 AND action = 'employee.resign' AND actor_name = 'Sistem (resign terjadwal)'`, f.e); got != "1" {
		t.Fatalf("resign audit = %s, want 1", got)
	}
	if got := h.scalar(`SELECT (applied_at IS NOT NULL)::text FROM hris.scheduled_resignations WHERE employee_id = $1`, f.e); got != "true" {
		t.Fatal("applied_at must be set")
	}
	applied, _, err = h.svc.RunScheduledResigns(ctx, wib(t, "2026-11-21 00:10"))
	if err != nil || applied != 0 {
		t.Fatalf("second run: applied %d err %v, want 0", applied, err)
	}
}

func TestRunScheduledResignsInactive(t *testing.T) {
	h := newHarness(t)
	f := newResignFixture(h)
	expect(t, h.do(&f.hr, http.MethodPost, resignURL(f.e), map[string]any{"effective_date": "2026-11-20", "resign_type": "resign"}), http.StatusOK, "")
	// The deactivation trigger cancels the schedule at once (review C1); the
	// job's own R7 check stays as a guard for rows written around it.
	h.exec(`UPDATE hris.employees SET is_active = false WHERE id = $1`, f.e)
	applied, cancelled, err := h.svc.RunScheduledResigns(context.Background(), wib(t, "2026-11-21 00:05"))
	if err != nil || applied != 0 || cancelled != 0 {
		t.Fatalf("applied %d cancelled %d err %v, want 0/0 (already cancelled)", applied, cancelled, err)
	}
	if got := h.scalar(`SELECT coalesce(cancel_reason, '') FROM hris.scheduled_resignations WHERE employee_id = $1`, f.e); got != "Karyawan dinonaktifkan" {
		t.Fatalf("cancel_reason = %q", got)
	}
	if got := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE entity_id = $1 AND action = 'employee.resign'`, f.e); got != "0" {
		t.Fatalf("resign audit = %s, want 0", got)
	}
}

// Review C1: a schedule must not survive the employee being deactivated by
// another path; after a rehire the job must leave the employee alone.
func TestScheduledResignCancelledOnDeactivateThenRehire(t *testing.T) {
	h := newHarness(t)
	f := newResignFixture(h)
	expect(t, h.do(&f.hr, http.MethodPost, resignURL(f.e), map[string]any{"effective_date": "2026-11-20", "resign_type": "resign"}), http.StatusOK, "")
	h.exec(`UPDATE hris.employees SET is_active = false WHERE id = $1`, f.e)
	if got := h.scalar(`SELECT coalesce(cancel_reason, '') FROM hris.scheduled_resignations WHERE employee_id = $1`, f.e); got != "Karyawan dinonaktifkan" {
		t.Fatalf("cancel_reason after deactivation = %q", got)
	}
	h.exec(`UPDATE hris.employees SET is_active = true WHERE id = $1`, f.e) // rehire
	applied, cancelled, err := h.svc.RunScheduledResigns(context.Background(), wib(t, "2026-11-21 00:05"))
	if err != nil || applied != 0 || cancelled != 0 {
		t.Fatalf("applied %d cancelled %d err %v, want 0/0", applied, cancelled, err)
	}
	if got := h.scalar(`SELECT is_active::text FROM hris.employees WHERE id = $1`, f.e); got != "true" {
		t.Fatalf("rehired employee is_active = %s, want true", got)
	}
}

// Review I2: a due schedule that can no longer be applied (validation) is
// cancelled with the reason instead of retrying every 5 minutes.
func TestScheduledResignInvalidIsCancelled(t *testing.T) {
	h := newHarness(t)
	f := newResignFixture(h)
	expect(t, h.do(&f.hr, http.MethodPost, resignURL(f.e), map[string]any{"effective_date": "2026-11-20", "resign_type": "resign"}), http.StatusOK, "")
	h.exec(`UPDATE hris.employees SET join_date = '2026-11-25' WHERE id = $1`, f.e)
	applied, cancelled, err := h.svc.RunScheduledResigns(context.Background(), wib(t, "2026-11-21 00:05"))
	if err != nil || applied != 0 || cancelled != 1 {
		t.Fatalf("applied %d cancelled %d err %v, want 0/1", applied, cancelled, err)
	}
	if got := h.scalar(`SELECT coalesce(cancel_reason, '') FROM hris.scheduled_resignations WHERE employee_id = $1`, f.e); got != "Tanggal efektif tidak boleh sebelum tanggal masuk (25/11/2026)" {
		t.Fatalf("cancel_reason = %q", got)
	}
	if got := h.scalar(`SELECT is_active::text FROM hris.employees WHERE id = $1`, f.e); got != "true" {
		t.Fatalf("is_active = %s, want true", got)
	}
}

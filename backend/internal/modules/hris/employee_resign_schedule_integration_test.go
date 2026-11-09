package hris

import (
	"net/http"
	"testing"

	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/testutil"
)

// EPIC-095: tanggal efektif (hari terakhir kerja) di masa depan → resign terjadwal.

type resignFixture struct {
	hr, login testutil.Staff
	e         string
}

func newResignFixture(h *harness) resignFixture {
	h.t.Helper()
	h.setNow("2026-11-06 10:00")
	var f resignFixture
	f.hr, _ = h.staff("hrd", iam.HrisKepegawaian, true, emp{name: "HR Resign"})
	f.login, f.e = h.staff("employee", nil, true, emp{name: "Rina Terjadwal", join: "2025-01-10"})
	branch := h.scalar(`SELECT id::text FROM configuration.branches LIMIT 1`)
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date) VALUES ($1, $2, true, '2025-01-10')`, f.e, branch)
	return f
}

func resignURL(id string) string { return "/api/hris/employees/" + id + "/resign" }

func TestResignScheduledFuture(t *testing.T) {
	h := newHarness(t)
	f := newResignFixture(h)
	c := h.do(&f.hr, http.MethodPost, resignURL(f.e), map[string]any{"effective_date": "2026-11-20", "resign_type": "resign", "reason": "Pindah kerja"})
	expect(t, c, http.StatusOK, "")
	if c.body["message"] != "Resign dijadwalkan efektif 20-11-2026" || c.data()["scheduled"] != true {
		t.Fatalf("schedule = %s", c.raw)
	}
	if got := h.scalar(`SELECT is_active::text FROM hris.employees WHERE id = $1`, f.e); got != "true" {
		t.Fatalf("is_active = %s, want still active", got)
	}
	if got := h.scalar(`SELECT (banned_until IS NULL)::text FROM auth.users WHERE id = $1`, f.login.UserID); got != "true" {
		t.Fatal("login must stay open until the schedule runs")
	}
	if got := h.scalar(`SELECT (end_date IS NULL)::text FROM hris.employee_placements WHERE employee_id = $1`, f.e); got != "true" {
		t.Fatal("placement must stay open")
	}
	if got := h.scalar(`SELECT count(*)::text FROM hris.scheduled_resignations WHERE employee_id = $1 AND applied_at IS NULL AND cancelled_at IS NULL`, f.e); got != "1" {
		t.Fatalf("pending schedules = %s, want 1", got)
	}
	if got := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE entity_id = $1 AND action = 'employee.resign_schedule'`, f.e); got != "1" {
		t.Fatalf("schedule audit = %s, want 1", got)
	}
	g := h.do(&f.hr, http.MethodGet, resignURL(f.e), nil)
	expect(t, g, http.StatusOK, "")
	if d := g.data(); d == nil || d["effective_date"] != "2026-11-20" || d["resign_type"] != "resign" || d["reason"] != "Pindah kerja" {
		t.Fatalf("GET = %s", g.raw)
	}
}

func TestResignScheduledLimits(t *testing.T) {
	h := newHarness(t)
	f := newResignFixture(h)
	expect(t, h.do(&f.hr, http.MethodPost, resignURL(f.e), map[string]any{"effective_date": "2027-05-06", "resign_type": "resign"}),
		http.StatusBadRequest, "Tanggal efektif paling jauh 180 hari dari hari ini")
	expect(t, h.do(&f.hr, http.MethodPost, resignURL(f.e), map[string]any{"effective_date": "2024-12-31", "resign_type": "resign"}),
		http.StatusBadRequest, "Tanggal efektif tidak boleh sebelum tanggal masuk (10/01/2025)")
	today := h.do(&f.hr, http.MethodPost, resignURL(f.e), map[string]any{"effective_date": "2026-11-06", "resign_type": "resign"})
	expect(t, today, http.StatusOK, "")
	if today.body["message"] != "Karyawan diresign" {
		t.Fatalf("today = %s", today.raw)
	}
	if got := h.scalar(`SELECT is_active::text FROM hris.employees WHERE id = $1`, f.e); got != "false" {
		t.Fatalf("today = immediate resign, is_active = %s", got)
	}
	if got := h.scalar(`SELECT count(*)::text FROM hris.scheduled_resignations WHERE employee_id = $1`, f.e); got != "0" {
		t.Fatalf("schedules = %s, want 0", got)
	}
}

func TestResignScheduledOnlyOne(t *testing.T) {
	h := newHarness(t)
	f := newResignFixture(h)
	expect(t, h.do(&f.hr, http.MethodPost, resignURL(f.e), map[string]any{"effective_date": "2026-11-20", "resign_type": "resign"}), http.StatusOK, "")
	const msg = "Sudah ada resign terjadwal efektif 20-11-2026 — batalkan jadwal itu dulu"
	expect(t, h.do(&f.hr, http.MethodPost, resignURL(f.e), map[string]any{"effective_date": "2026-11-25", "resign_type": "resign"}), http.StatusBadRequest, msg)
	expect(t, h.do(&f.hr, http.MethodPost, resignURL(f.e), map[string]any{"effective_date": "2026-11-06", "resign_type": "terminated"}), http.StatusBadRequest, msg)
}

func TestResignScheduleCancel(t *testing.T) {
	h := newHarness(t)
	f := newResignFixture(h)
	staff, _ := h.staff("pos", nil, true, emp{name: "Bukan HR"})
	expect(t, h.do(&f.hr, http.MethodPost, resignURL(f.e), map[string]any{"effective_date": "2026-11-20", "resign_type": "resign"}), http.StatusOK, "")
	expect(t, h.do(&staff, http.MethodDelete, resignURL(f.e), nil), http.StatusForbidden, "")
	cancel := h.do(&f.hr, http.MethodDelete, resignURL(f.e), map[string]any{"reason": "Batal resign"})
	expect(t, cancel, http.StatusOK, "")
	if cancel.body["message"] != "Jadwal resign dibatalkan" {
		t.Fatalf("cancel = %s", cancel.raw)
	}
	if got := h.scalar(`SELECT (cancelled_at IS NOT NULL)::text || '|' || coalesce(cancel_reason, '') FROM hris.scheduled_resignations WHERE employee_id = $1`, f.e); got != "true|Batal resign" {
		t.Fatalf("schedule = %s", got)
	}
	if got := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE entity_id = $1 AND action = 'employee.resign_schedule_cancel'`, f.e); got != "1" {
		t.Fatalf("cancel audit = %s, want 1", got)
	}
	g := h.do(&f.hr, http.MethodGet, resignURL(f.e), nil)
	expect(t, g, http.StatusOK, "")
	if g.body["data"] != nil {
		t.Fatalf("GET after cancel = %s, want null", g.raw)
	}
	expect(t, h.do(&f.hr, http.MethodDelete, resignURL(f.e), nil), http.StatusNotFound, "Tidak ada resign terjadwal")
}

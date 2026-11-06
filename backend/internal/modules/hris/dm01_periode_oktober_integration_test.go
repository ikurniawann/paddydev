package hris

import (
	"context"
	"strings"
	"testing"
)

// DM-01 lanjutan 06-11-2026: ringkasan closing periode Oktober dinolkan dan
// riwayat closing-nya dihapus, hanya setelah absensi 1–20 Okt bersih.
func TestDM01PeriodeOktober(t *testing.T) {
	h := newHarness(t)
	script := dm01Script(t, "2026-11-06_dm01_periode_oktober.sql")
	script = strings.ReplaceAll(script, `\set ON_ERROR_STOP on`, "")
	h.exec(`DELETE FROM hris.attendance_periods WHERE period_year = 2026 AND period_month = 10`)
	pid := h.scalar(`INSERT INTO hris.attendance_periods (period_year, period_month, start_date, end_date, closing_date, status, closed_at, close_summary)
		VALUES (2026, 10, '2026-10-01', '2026-10-20', '2026-10-24', 'closed', now(),
		        '{"late": 631, "rows": 3229, "pending_requests": 13}') RETURNING id::text`)
	h.exec(`INSERT INTO hris.attendance_period_events (period_id, action, details) VALUES ($1, 'auto_close', '{"summary": {"rows": 3229}}')`, pid)
	e := h.employee(emp{name: "DM01 Okt"})
	h.exec(`SET LOCAL hris.allow_closed_period_write = 'on'`)
	h.exec(`INSERT INTO hris.attendance (employee_id, date, status) VALUES ($1, '2026-10-05', 'present')`, e)
	h.exec(`SET LOCAL hris.allow_closed_period_write = 'off'`)

	h.exec("SAVEPOINT okt")
	_, err := h.tx.Exec(context.Background(), script)
	if err == nil || !strings.Contains(err.Error(), "jalankan cleanup DM-01 dulu") {
		t.Fatalf("with attendance left: err = %v, want refusal", err)
	}
	h.exec("ROLLBACK TO SAVEPOINT okt")

	h.exec(`SET LOCAL hris.allow_closed_period_write = 'on'`)
	h.exec(`DELETE FROM hris.attendance WHERE employee_id = $1`, e)
	h.exec(`SET LOCAL hris.allow_closed_period_write = 'off'`)
	if _, err := h.tx.Exec(context.Background(), script); err != nil {
		t.Fatalf("script: %v", err)
	}
	if got := h.scalar(`SELECT close_summary::text FROM hris.attendance_periods WHERE id = $1`, pid); got != `{"late": 0, "rows": 0, "pending_requests": 0}` {
		t.Errorf("close_summary = %s", got)
	}
	if got := h.scalar(`SELECT count(*)::text FROM hris.attendance_period_events WHERE period_id = $1`, pid); got != "0" {
		t.Errorf("events = %s, want 0", got)
	}
	if got := h.scalar(`SELECT before->'periods'->0->'close_summary'->>'rows' FROM audit.audit_log
		WHERE action = 'data_cleanup.dm01_periods' AND created_at = now()`); got != "3229" {
		t.Errorf("audit before rows = %q, want 3229", got)
	}
	if got := h.scalar(`SELECT status FROM hris.attendance_periods WHERE id = $1`, pid); got != "closed" {
		t.Errorf("status = %s, want closed", got)
	}
}

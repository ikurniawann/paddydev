package hris

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// EPIC-080 R1/R3/R4/R6: skema aturan checkout otomatis, penanda clock_out_auto,
// shift kedua, dan notifikasi "Jam Kerja Kurang" yang dilewati checkout otomatis.
func TestAutoCheckoutSchema(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	// pgErrCode menjalankan SQL di savepoint supaya transaksi fixture tetap hidup.
	pgErrCode := func(sql string, args ...any) string {
		t.Helper()
		h.exec("SAVEPOINT schema_probe")
		_, err := h.tx.Exec(ctx, sql, args...)
		h.exec("ROLLBACK TO SAVEPOINT schema_probe")
		h.exec("RELEASE SAVEPOINT schema_probe")
		if err == nil {
			return ""
		}
		if pe, ok := err.(*pgconn.PgError); ok {
			return pe.Code
		}
		t.Fatalf("unexpected error type: %v", err)
		return ""
	}

	// 1. Aturan: unik per (jabatan, cabang) dan cabang NULL dihitung sebagai satu nilai.
	pos := h.scalar(`INSERT INTO hris.positions (title) VALUES ('Posisi Uji Checkout') RETURNING id::text`)
	branch := rehireBranch(h, "Toko Uji Checkout")
	h.exec(`INSERT INTO hris.auto_checkout_rules (position_id) VALUES ($1)`, pos)
	if code := pgErrCode(`INSERT INTO hris.auto_checkout_rules (position_id) VALUES ($1)`, pos); code != "23505" {
		t.Fatalf("duplicate all-locations rule: code %q, want 23505", code)
	}
	h.exec(`INSERT INTO hris.auto_checkout_rules (position_id, branch_id) VALUES ($1, $2)`, pos, branch)
	if code := pgErrCode(`INSERT INTO hris.auto_checkout_rules (position_id, branch_id) VALUES ($1, $2)`, pos, branch); code != "23505" {
		t.Fatalf("duplicate branch rule: code %q, want 23505", code)
	}
	if got := h.scalar(`SELECT is_active::text FROM hris.auto_checkout_rules WHERE position_id = $1 AND branch_id IS NULL`, pos); got != "true" {
		t.Fatalf("is_active default = %q, want true", got)
	}

	// 2. Shift kedua tanpa shift pertama ditolak (per tanggal dan mingguan).
	s1 := h.scalar(`INSERT INTO hris.shifts (name, start_time, end_time) VALUES ('Uji Pagi', '08:00', '12:00') RETURNING id::text`)
	s2 := h.scalar(`INSERT INTO hris.shifts (name, start_time, end_time) VALUES ('Uji Sore', '17:00', '21:00') RETURNING id::text`)
	e := h.employee(emp{name: "Karyawan Dua Shift"})
	if code := pgErrCode(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, shift2_id, batch_id) VALUES ($1, '2026-10-06', NULL, $2, gen_random_uuid())`, e, s2); code != "23514" {
		t.Fatalf("shift2 without shift (day): code %q, want 23514", code)
	}
	if code := pgErrCode(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, shift2_id, effective_from) VALUES ($1, 2, NULL, $2, '2026-10-01')`, e, s2); code != "23514" {
		t.Fatalf("shift2 without shift (weekly): code %q, want 23514", code)
	}

	// 3. View membawa shift2_id untuk baris per tanggal dan mingguan.
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, shift2_id, batch_id) VALUES ($1, '2026-10-06', $2, $3, gen_random_uuid())`, e, s1, s2)
	h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, shift2_id, effective_from) VALUES ($1, 3, $2, $3, '2026-10-01')`, e, s1, s2)
	h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from) VALUES ($1, 4, $2, '2026-10-01')`, e, s1)
	rows := h.scalar(`SELECT string_agg(day_of_week || ':' || priority || ':' || coalesce(shift2_id::text, '-'), ',' ORDER BY day_of_week)
		FROM hris.v_employee_schedule WHERE employee_id = $1`, e)
	if want := "2:0:" + s2 + ",3:1:" + s2 + ",4:1:-"; rows != want {
		t.Fatalf("view rows = %q, want %q", rows, want)
	}
	// Kolom lama tidak bergeser: shift2_id paling akhir.
	cols := h.scalar(`SELECT string_agg(attname, ',' ORDER BY attnum) FROM pg_attribute
		WHERE attrelid = 'hris.v_employee_schedule'::regclass AND attnum > 0 AND NOT attisdropped`)
	if want := "employee_id,day_of_week,shift_id,effective_from,effective_to,priority,shift2_id"; cols != want {
		t.Fatalf("view columns = %q, want %q", cols, want)
	}

	// 4. Notifikasi "Jam Kerja Kurang" hanya untuk clock-out manual.
	hr, _ := h.staff("hrd", nil, false)
	short := func() string {
		return h.scalar(`SELECT count(*)::text FROM public.notifications WHERE user_id = $1 AND title LIKE '%Jam Kerja Kurang%'`, hr.UserID)
	}
	manual := h.employee(emp{name: "Pulang Manual"})
	auto := h.employee(emp{name: "Pulang Otomatis"})
	for _, id := range []string{manual, auto} {
		h.exec(`INSERT INTO hris.attendance (employee_id, date, clock_in) VALUES ($1, '2026-10-05', '2026-10-05 01:00:00+00')`, id)
	}
	if got := h.scalar(`SELECT clock_out_auto::text FROM hris.attendance WHERE employee_id = $1`, auto); got != "false" {
		t.Fatalf("clock_out_auto default = %q, want false", got)
	}
	h.exec(`UPDATE hris.attendance SET clock_out = '2026-10-05 05:00:00+00', clock_out_auto = true WHERE employee_id = $1`, auto)
	if got := h.scalar(`SELECT work_hours::text FROM hris.attendance WHERE employee_id = $1`, auto); got != "3.00" {
		t.Fatalf("auto work_hours = %q, want 3.00 (precondition < 7)", got)
	}
	if short() != "0" {
		t.Fatalf("auto checkout notified HR: %s notifications", short())
	}
	h.exec(`UPDATE hris.attendance SET clock_out = '2026-10-05 05:00:00+00', clock_out_auto = false WHERE employee_id = $1`, manual)
	if short() != "1" {
		t.Fatalf("manual clock-out notifications = %s, want 1", short())
	}
	if !strings.Contains(h.scalar(`SELECT title FROM public.notifications WHERE user_id = $1 AND title LIKE '%Jam Kerja Kurang%'`, hr.UserID), "Pulang Manual") {
		t.Fatal("notification is not about the manual clock-out")
	}
}

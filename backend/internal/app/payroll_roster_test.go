package app

import (
	"context"
	"maps"
	"slices"
	"testing"

	"nuhabit/backend/internal/modules/payroll/domain"
	"nuhabit/backend/internal/platform/testutil"
)

// EPIC-079 R7: payroll membaca jadwal dari hris.v_employee_schedule (roster
// per tanggal + pola mingguan), membawa schedule_type karyawan, dan peta
// libur R2 (Non-Shift).
func TestPayrollScheduleReadsRoster(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	var store, hq, ns string
	must(tx.QueryRow(ctx, `INSERT INTO hris.employees (full_name, nip, email, phone, join_date, schedule_type)
		VALUES ('Staf Toko Roster', 'RST-'||md5(random()::text), md5(random()::text)||'@rst.test', '08', '2024-01-01', 'shift')
		RETURNING id::text`).Scan(&store))
	must(tx.QueryRow(ctx, `INSERT INTO hris.employees (full_name, nip, email, phone, join_date, schedule_type)
		VALUES ('Staf HQ Roster', 'RST-'||md5(random()::text), md5(random()::text)||'@rst.test', '08', '2024-01-01', 'non_shift')
		RETURNING id::text`).Scan(&hq))
	must(tx.QueryRow(ctx, `SELECT id::text FROM hris.shifts WHERE code = 'NS'`).Scan(&ns))
	// November 2097: 26 hari ber-shift, libur roster pada 4 tanggal.
	_, err := tx.Exec(ctx, `INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id)
		SELECT $1, d::date, CASE WHEN d::date IN ('2097-11-05', '2097-11-12', '2097-11-19', '2097-11-26') THEN NULL ELSE $2::uuid END,
		       gen_random_uuid()
		FROM generate_series('2097-11-01'::date, '2097-11-30'::date, interval '1 day') d`, store, ns)
	must(err)

	ports := payrollPorts()
	rec, err := ports.Workforce.PeriodRecords(ctx, tx, store, "2097-11-01", "2097-11-30")
	must(err)
	if len(rec.Schedule) != 30 || !rec.Schedule[0].PerDate() {
		t.Fatalf("schedule rows = %d (%+v), want 30 per-date rows", len(rec.Schedule), rec.Schedule)
	}
	if days, has := domain.ScheduledDays(rec.Schedule, "2097-11-01", "2097-11-30"); !has || days != 26 {
		t.Fatalf("ScheduledDays = %d (has=%v), want 26 rostered days", days, has)
	}
	// Pola mingguan ikut terbaca (prioritas 1); roster per tanggal menang di tanggalnya.
	_, err = tx.Exec(ctx, `INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from)
		SELECT $1, dow, $2::uuid, '2097-11-01' FROM generate_series(1, 7) dow`, store, ns)
	must(err)
	rec, err = ports.Workforce.PeriodRecords(ctx, tx, store, "2097-11-01", "2097-11-30")
	must(err)
	weekly := slices.IndexFunc(rec.Schedule, func(r domain.ShiftRow) bool { return r.Priority == 1 })
	if len(rec.Schedule) != 37 || weekly < 0 {
		t.Fatalf("schedule rows = %d, weekly at %d, want 30 per-date + 7 weekly", len(rec.Schedule), weekly)
	}
	if days, _ := domain.ScheduledDays(rec.Schedule, "2097-11-01", "2097-11-30"); days != 26 {
		t.Fatalf("ScheduledDays with weekly pattern = %d, want 26 (roster wins)", days)
	}
	kpi, err := payrollKPISQL{}.ShiftPatterns(ctx, tx, "2097-11-01", "2097-11-30")
	must(err)
	if days, _ := domain.ScheduledDays(kpi[store], "2097-11-01", "2097-11-30"); len(kpi[store]) != 37 || days != 26 {
		t.Fatalf("KPI ShiftPatterns rows = %d, scheduled = %d, want 37 rows and 26 days", len(kpi[store]), days)
	}

	list, err := ports.Employees.PayrollEmployees(ctx, tx, "2097-11-01", "2097-11-30")
	must(err)
	types := map[string]string{}
	for _, e := range list {
		types[e.ID] = e.ScheduleType
	}
	if types[store] != "shift" || types[hq] != "non_shift" {
		t.Fatalf("schedule types store=%q hq=%q", types[store], types[hq])
	}

	// Peta libur Non-Shift (R2): nasional, perusahaan, cuti bersama yang
	// ditetapkan libur; bukan cuti bersama biasa, nonaktif, atau terhapus.
	_, err = tx.Exec(ctx, `INSERT INTO hris.public_holidays (holiday_date, name, type, deducts_leave, status, is_day_off, deleted_at) VALUES
		('2097-12-24', 'Cuti Bersama Kerja', 'cuti_bersama', true, 'aktif', false, NULL),
		('2097-12-25', 'Natal', 'nasional', false, 'aktif', false, NULL),
		('2097-12-26', 'Cuti Bersama Libur', 'cuti_bersama', true, 'aktif', true, NULL),
		('2097-12-27', 'Libur Perusahaan', 'perusahaan', false, 'aktif', false, NULL),
		('2097-12-28', 'Libur Terhapus', 'nasional', false, 'aktif', false, now())`)
	must(err)
	holidays, err := ports.Workforce.Holidays(ctx, tx, "2097-12-01", "2097-12-31")
	must(err)
	if got := slices.Sorted(maps.Keys(holidays)); !slices.Equal(got, []string{"2097-12-25", "2097-12-26", "2097-12-27"}) {
		t.Fatalf("holidays = %v, want 25, 26, 27 Dec", got)
	}
}

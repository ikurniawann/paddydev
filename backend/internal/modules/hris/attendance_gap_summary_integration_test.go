package hris

import (
	"context"
	"testing"

	"nuhabit/backend/internal/platform/database"
)

// superAdminDirectory is fakeDirectory with super admin accounts.
type superAdminDirectory struct {
	fakeDirectory
	ids []string
}

func (d superAdminDirectory) SuperAdminUserIDs(context.Context, database.Querier) ([]string, error) {
	return d.ids, nil
}

// EPIC-067 R4, R5, R9: ringkasan absen terlewat satu tanggal untuk HR,
// dihitung dengan aturan yang sama dengan absen terlewat ESS.
func TestAttendanceGapSummary(t *testing.T) {
	h := newHarness(t)
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 0, closing_offset_days = 3`)
	// D = Kamis 1 Okt 2026: periode Oktober masih terbuka pada fixedNow (Sen 5 Okt).
	const d = "2026-10-01"
	const dow = 4
	shift := h.scalar(`INSERT INTO hris.shifts (name, start_time, end_time, late_tolerance_minutes)
		VALUES ('Pagi Ringkasan', '08:00', '17:00', 10) RETURNING id::text`)
	dept := h.department("Dept Ringkasan")
	scheduled := func(e emp) string {
		id := h.employee(e)
		h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from) VALUES ($1, $2, $3, '2026-01-01')`,
			id, dow, shift)
		return id
	}
	a := scheduled(emp{name: "Ringkasan A Terlewat", dept: &dept})
	b := scheduled(emp{name: "Ringkasan B Belum Pulang"})
	h.exec(`INSERT INTO hris.attendance (employee_id, date, clock_in, status) VALUES ($1, $2, '2026-10-01 08:05+07', 'present')`, b, d)
	c := scheduled(emp{name: "Ringkasan C Lengkap"})
	h.exec(`INSERT INTO hris.attendance (employee_id, date, clock_in, clock_out, status)
		VALUES ($1, $2, '2026-10-01 08:00+07', '2026-10-01 17:00+07', 'present')`, c, d)
	e := scheduled(emp{name: "Ringkasan E Cuti"})
	h.exec(`INSERT INTO hris.leaves (employee_id, leave_type, start_date, end_date, total_days, reason, status)
		VALUES ($1, 'annual', '2026-09-30', '2026-10-02', 3, 'Cuti uji ringkasan', 'approved')`, e)
	f := scheduled(emp{name: "Ringkasan F Dinas"})
	h.exec(`INSERT INTO hris.business_trips (employee_id, start_date, end_date, destination, purpose, status)
		VALUES ($1, '2026-10-01', '2026-10-01', 'Surabaya', 'Audit toko', 'approved')`, f)
	g := scheduled(emp{name: "Ringkasan G Menunggu"})
	h.exec(`INSERT INTO hris.attendance_requests (employee_id, date, kind, clock_in, reason)
		VALUES ($1, $2, 'manual', '08:00', 'Lupa absen masuk')`, g, d)
	scheduled(emp{name: "Ringkasan H Belum Gabung", join: "2026-10-02"})
	i := scheduled(emp{name: "Ringkasan I Sudah Keluar"})
	h.exec(`UPDATE hris.employees SET end_date = '2026-09-30' WHERE id = $1`, i)
	j := h.employee(emp{name: "Ringkasan J Libur Jadwal"})
	h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from) VALUES ($1, $2, NULL, '2026-01-01')`, j, dow)
	// R4: di luar masa kerja juga tidak dihitung tanpa jadwal.
	h.employee(emp{name: "Ringkasan H2 Belum Gabung Tanpa Jadwal", join: "2026-10-02"})
	i2 := h.employee(emp{name: "Ringkasan I2 Sudah Keluar Tanpa Jadwal"})
	h.exec(`UPDATE hris.employees SET end_date = '2026-09-30' WHERE id = $1`, i2)
	h.employee(emp{name: "Ringkasan K Tanpa Jadwal"})
	scheduled(emp{name: "Ringkasan Nonaktif", inactive: true})
	// Akun super admin bukan staf (sama dengan RosterRows).
	_, boss := h.staff("super_admin", nil, true, emp{name: "Ringkasan Super Admin"})
	h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from) VALUES ($1, $2, $3, '2026-01-01')`,
		boss, dow, shift)
	bossUser := h.scalar(`SELECT user_id::text FROM hris.employees WHERE id = $1`, boss)
	h.svc.directory = superAdminDirectory{ids: []string{bossUser}}

	hr, _ := h.staff("hrd", []string{"hris.workforce.attendance"}, false)
	plain, _ := h.staff("pos", nil, false)
	const url = "/api/hris/attendance/gap-summary"
	summary := func(date string) map[string]any {
		t.Helper()
		got := h.do(&hr, "GET", url+"?date="+date, nil)
		expect(t, got, 200, "")
		return got.data()
	}
	counts := func(label string, s map[string]any, missing, noClockOut, unscheduled float64, closed bool) {
		t.Helper()
		if s["missing"] != missing || s["no_clock_out"] != noClockOut || s["unscheduled"] != unscheduled || s["period_closed"] != closed {
			t.Fatalf("%s = %v, want missing %v no_clock_out %v unscheduled %v period_closed %v",
				label, s, missing, noClockOut, unscheduled, closed)
		}
	}

	got := h.do(&hr, "GET", url+"?date="+d, nil)
	expect(t, got, 200, "")
	if k := keys(t, got.raw, "data"); len(k) != 6 || k[0] != "date" || k[1] != "missing" || k[2] != "no_clock_out" ||
		k[3] != "unscheduled" || k[4] != "period_closed" || k[5] != "employees" {
		t.Fatalf("keys = %v", k)
	}
	s := got.data()
	if s["date"] != d {
		t.Fatalf("date = %v", s["date"])
	}
	counts("D", s, 1, 1, 1, false)
	list, _ := s["employees"].([]any)
	if len(list) != 2 {
		t.Fatalf("employees = %s", got.raw)
	}
	first, second := list[0].(map[string]any), list[1].(map[string]any)
	if first["employee_id"] != a || first["name"] != "Ringkasan A Terlewat" || first["kind"] != "missing" ||
		first["clock_in"] != nil || first["department"] != "Dept Ringkasan" || first["nip"] == nil {
		t.Fatalf("first = %v", first)
	}
	if second["employee_id"] != b || second["kind"] != "no_clock_out" || second["clock_in"] != "08:05" || second["department"] != nil {
		t.Fatalf("second = %v", second)
	}
	if k := keys(t, got.raw, "data", "employees"); len(k) != 6 || k[0] != "employee_id" || k[1] != "name" || k[2] != "nip" ||
		k[3] != "department" || k[4] != "kind" || k[5] != "clock_in" {
		t.Fatalf("employee keys = %v", k)
	}

	// Task 3 memanggil ringkasan di dalam transaksinya sendiri.
	ctx := context.Background()
	var inTx *GapSummary
	if err := h.svc.repo.InTx(ctx, func(r Repository) error {
		var err error
		inTx, err = h.svc.AttendanceGapSummary(ctx, r, d)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if inTx.Date != d || inTx.Missing != 1 || inTx.NoClockOut != 1 || inTx.Unscheduled != 1 || len(inTx.Employees) != 2 {
		t.Fatalf("in tx = %+v", inTx)
	}

	// R5: tanpa jadwal tetap terdaftar bila absen masuk tanpa absen pulang.
	l := h.employee(emp{name: "Ringkasan L Tanpa Jadwal Belum Pulang"})
	h.exec(`INSERT INTO hris.attendance (employee_id, date, clock_in, status) VALUES ($1, $2, '2026-10-01 09:00+07', 'present')`, l, d)
	s = summary(d)
	counts("unscheduled no_clock_out", s, 1, 2, 2, false)
	if list = s["employees"].([]any); len(list) != 3 || list[0].(map[string]any)["kind"] != "missing" ||
		list[1].(map[string]any)["name"] != "Ringkasan B Belum Pulang" || list[2].(map[string]any)["employee_id"] != l {
		t.Fatalf("sorted employees = %v", list)
	}

	// Libur nasional: tidak ada yang terlewat, absen pulang tetap.
	h.exec(`INSERT INTO hris.public_holidays (holiday_date, name) VALUES ($1, 'Libur Uji Ringkasan')`, d)
	counts("holiday", summary(d), 0, 2, 2, false)

	// Periode D ditutup: hanya informasi.
	h.exec(`UPDATE hris.attendance_periods SET status = 'closed', closed_at = now(), close_mode = 'manual', closed_by_name = 'Uji'
		WHERE $1::date BETWEEN start_date AND end_date`, d)
	counts("closed", summary(d), 0, 2, 2, true)

	// Tanpa tanggal: kemarin (WIB).
	yesterday := h.do(&hr, "GET", url, nil)
	expect(t, yesterday, 200, "")
	if yesterday.data()["date"] != "2026-10-04" {
		t.Fatalf("default date = %s", yesterday.raw)
	}
	expect(t, h.do(&hr, "GET", url+"?date=2026-13-01", nil), 400, "Tanggal tidak valid")
	expect(t, h.do(&hr, "GET", url+"?date=1-10-2026", nil), 400, "Tanggal tidak valid")
	expect(t, h.do(&hr, "GET", url+"?date=2026-10-06", nil), 400, "Tanggal tidak boleh di masa depan")
	expect(t, h.do(&hr, "GET", url+"?date=2026-10-05", nil), 200, "")
	expect(t, h.do(&plain, "GET", url+"?date="+d, nil), 403, "")
	expect(t, h.do(nil, "GET", url+"?date="+d, nil), 401, "")
}

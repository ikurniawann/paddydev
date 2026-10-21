package hris

import "testing"

// EPIC-065 R1 (MOM AB-06, K-9): karyawan melihat absen terlewat miliknya di
// periode absensi yang masih terbuka, sampai kemarin.
func TestAttendanceGaps(t *testing.T) {
	h := newHarness(t)
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 0, closing_offset_days = 3`)
	crew, crewID := h.staff("pos", nil, true, emp{name: "Crew Terlewat"})
	loose, looseID := h.staff("pos", nil, true, emp{name: "Crew Tanpa Jadwal"})
	unlinked, _ := h.staff("pos", nil, false)
	shift := h.scalar(`INSERT INTO hris.shifts (name, start_time, end_time, late_tolerance_minutes)
		VALUES ('Pagi Terlewat', '08:00', '17:00', 10) RETURNING id::text`)
	for dow := 1; dow <= 6; dow++ {
		h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from) VALUES ($1, $2, $3, '2026-01-01')`,
			crewID, dow, shift)
	}
	h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from) VALUES ($1, 7, NULL, '2026-01-01')`, crewID)
	h.exec(`INSERT INTO hris.public_holidays (holiday_date, name) VALUES ('2026-10-02', 'Libur Uji Terlewat')`)
	h.exec(`INSERT INTO hris.attendance (employee_id, date, clock_in, status) VALUES ($1, '2026-10-03', '2026-10-03 08:05+07', 'present')`, crewID)
	h.exec(`INSERT INTO hris.attendance (employee_id, date, clock_in, status) VALUES ($1, '2026-10-01', '2026-10-01 08:00+07', 'present')`, looseID)
	const url = "/api/hris/me/attendance-gaps"
	days := func(c call) []map[string]any {
		t.Helper()
		expect(t, c, 200, "")
		var out []map[string]any
		for _, d := range c.data()["days"].([]any) {
			out = append(out, d.(map[string]any))
		}
		return out
	}

	// Kam 1 Okt tanpa absen, Jum 2 Okt libur, Sab 3 Okt belum pulang, Min 4 Okt libur jadwal;
	// hari ini (Sen 5 Okt) tidak dihitung; September sudah di-closing.
	gaps := h.do(&crew, "GET", url, nil)
	if d := gaps.data(); d["start"] != "2026-10-01" || d["end"] != "2026-10-04" {
		t.Fatalf("window = %s", gaps.raw)
	}
	got := days(gaps)
	if len(got) != 2 || got[0]["date"] != "2026-10-01" || got[0]["kind"] != "missing" || got[0]["clock_in"] != nil ||
		got[1]["date"] != "2026-10-03" || got[1]["kind"] != "no_clock_out" || got[1]["clock_in"] != "08:05" {
		t.Fatalf("gaps = %s", gaps.raw)
	}
	// Cuti yang disetujui menghapus hari itu.
	h.exec(`INSERT INTO hris.leaves (employee_id, leave_type, start_date, end_date, total_days, reason, status)
		VALUES ($1, 'annual', '2026-10-01', '2026-10-01', 1, 'Cuti uji terlewat', 'approved')`, crewID)
	if got = days(h.do(&crew, "GET", url, nil)); len(got) != 1 || got[0]["date"] != "2026-10-03" {
		t.Fatalf("after leave = %v", got)
	}
	// Pengajuan yang menunggu menghapus hari itu.
	expect(t, h.do(&crew, "POST", "/api/hris/attendance-requests", attReqBody("2026-10-03", "", "17:00", "Lupa absen pulang")), 201, "")
	if got = days(h.do(&crew, "GET", url, nil)); len(got) != 0 {
		t.Fatalf("after request = %v", got)
	}
	// Tanpa jadwal: hanya absen pulang yang hilang.
	if got = days(h.do(&loose, "GET", url, nil)); len(got) != 1 || got[0]["date"] != "2026-10-01" || got[0]["kind"] != "no_clock_out" {
		t.Fatalf("no schedule = %v", got)
	}
	// Akun tanpa data karyawan: kosong.
	empty := h.do(&unlinked, "GET", url, nil)
	if got = days(empty); len(got) != 0 || empty.data()["start"] != nil {
		t.Fatalf("unlinked = %s", empty.raw)
	}
	expect(t, h.do(nil, "GET", url, nil), 401, "")
}

// EPIC-065 P4: perjalanan dinas yang disetujui menghapus hari itu seperti
// cuti; hari sebelum tanggal bergabung dan sesudah tanggal berakhir bukan
// absen terlewat.
func TestAttendanceGapsEligibility(t *testing.T) {
	h := newHarness(t)
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 0, closing_offset_days = 3`)
	shift := h.scalar(`INSERT INTO hris.shifts (name, start_time, end_time, late_tolerance_minutes)
		VALUES ('Pagi Kelayakan', '08:00', '17:00', 10) RETURNING id::text`)
	const url = "/api/hris/me/attendance-gaps"
	// worker is a crew member scheduled Monday–Saturday.
	worker := func(name, join string) (id string, do func() []string) {
		s, empID := h.staff("pos", nil, true, emp{name: name, join: join})
		for dow := 1; dow <= 6; dow++ {
			h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from) VALUES ($1, $2, $3, '2026-01-01')`,
				empID, dow, shift)
		}
		return empID, func() []string {
			t.Helper()
			c := h.do(&s, "GET", url, nil)
			expect(t, c, 200, "")
			var out []string
			for _, d := range c.data()["days"].([]any) {
				m := d.(map[string]any)
				out = append(out, m["date"].(string)+":"+m["kind"].(string))
			}
			return out
		}
	}
	same := func(label string, got []string, want ...string) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s = %v, want %v", label, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s = %v, want %v", label, got, want)
			}
		}
	}

	// Kam 1, Jum 2, Sab 3 Okt terjadwal; Min 4 Okt tanpa shift.
	tripID, trip := worker("Crew Dinas", "")
	same("no trip", trip(), "2026-10-01:missing", "2026-10-02:missing", "2026-10-03:missing")
	h.exec(`INSERT INTO hris.business_trips (employee_id, start_date, end_date, destination, purpose, status)
		VALUES ($1, '2026-10-01', '2026-10-02', 'Surabaya', 'Audit toko', 'approved')`, tripID)
	same("approved trip", trip(), "2026-10-03:missing")

	// Perjalanan dinas yang belum disetujui / ditolak tidak menghapus hari.
	pendID, pend := worker("Crew Dinas Menunggu", "")
	h.exec(`INSERT INTO hris.business_trips (employee_id, start_date, end_date, destination, purpose, status)
		VALUES ($1, '2026-10-01', '2026-10-01', 'Bandung', 'Rapat', 'pending')`, pendID)
	h.exec(`INSERT INTO hris.business_trips (employee_id, start_date, end_date, destination, purpose, status)
		VALUES ($1, '2026-10-03', '2026-10-03', 'Bandung', 'Rapat', 'rejected')`, pendID)
	same("pending/rejected trip", pend(), "2026-10-01:missing", "2026-10-02:missing", "2026-10-03:missing")

	// Bergabung 2 Okt: 1 Okt belum karyawan.
	_, joiner := worker("Crew Baru", "2026-10-02")
	same("joined 2 Oct", joiner(), "2026-10-02:missing", "2026-10-03:missing")

	// Berakhir 2 Okt: 3 Okt sudah bukan karyawan.
	leaverID, leaver := worker("Crew Keluar", "")
	h.exec(`UPDATE hris.employees SET end_date = '2026-10-02' WHERE id = $1`, leaverID)
	same("ended 2 Oct", leaver(), "2026-10-01:missing", "2026-10-02:missing")

	// Absen masuk tanpa absen pulang tetap terdaftar di hari mana pun; baris
	// "Tidak hadir" dan baris lengkap bukan celah.
	rowsID, rows := worker("Crew Baris", "")
	h.exec(`INSERT INTO hris.attendance (employee_id, date, status) VALUES ($1, '2026-10-01', 'absent')`, rowsID)
	h.exec(`INSERT INTO hris.attendance (employee_id, date, clock_in, clock_out, status)
		VALUES ($1, '2026-10-02', '2026-10-02 08:00+07', '2026-10-02 17:00+07', 'present')`, rowsID)
	h.exec(`INSERT INTO hris.attendance (employee_id, date, clock_in, status, is_manual) VALUES ($1, '2026-10-03', '2026-10-03 08:00+07', 'late', true)`, rowsID)
	same("row types", rows(), "2026-10-03:no_clock_out")
}

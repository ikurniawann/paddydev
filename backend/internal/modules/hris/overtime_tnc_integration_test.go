package hris

import (
	"strings"
	"testing"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/testutil"
)

// EPIC-085: Ketentuan Lembur TnC 03-11 (R1–R4) di jalur HTTP.

// nsWeek gives an employee the HQ pattern NS Mon–Fri (09:00–18:00) from date.
func (h *harness) nsWeek(employeeID, from string) {
	h.t.Helper()
	for dow := 1; dow <= 5; dow++ {
		h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from)
			VALUES ($1, $2, (SELECT id FROM hris.shifts WHERE code = 'NS'), $3::date)`, employeeID, dow, from)
	}
}

// otRow inserts a pending/approved overtime row directly (company source).
func (h *harness) otRow(employeeID, date string, hours float64, status string) {
	h.t.Helper()
	h.exec(`INSERT INTO hris.overtime_requests (employee_id, date, start_time, end_time, hours, source, status, day_type)
		VALUES ($1, $2::date, '18:00', '23:00', $3, 'company', $4, 'regular')`, employeeID, date, hours, status)
}

func expectContains(t *testing.T, c call, status int, part string) {
	t.Helper()
	if c.status != status || !strings.Contains(c.raw, part) {
		t.Fatalf("want %d containing %q, got %d %s", status, part, c.status, c.raw)
	}
}

func TestOvertimeTnC(t *testing.T) {
	h, _ := newFileHarness(t)
	post := func(s *testutil.Staff, date, start, end string) call {
		t.Helper()
		next, _ := domain.ParseDate(date)
		h.setNow(next.AddDate(0, 0, 1).Format(domain.DateLayout) + " 09:00") // H+1 (EPIC-084)
		return h.do(s, "POST", "/api/hris/overtime", h.otSelfBody(s, date, start, end))
	}
	hrd, _ := h.staff("hrd", []string{"hris.kepegawaian.overtime"}, true, emp{name: "Admin Lembur"})

	// (a) R1: Supervisor ke atas ditolak, baik pengajuan sendiri maupun penugasan HR.
	spv, spvID := h.staff("pos", nil, true, emp{name: "Supervisor Toko"})
	h.leaderPosition(spvID, "Supervisor")
	expect(t, post(&spv, "2026-11-02", "18:00", "20:00"), 400, domain.OvertimeLeaderRefused)
	assign := otBody("2026-11-02", "18:00", "20:00")
	assign["employee_id"] = spvID
	expect(t, h.do(&hrd, "POST", "/api/hris/overtime", assign), 400, domain.OvertimeLeaderRefused)

	// (b)–(e) R2/R3 untuk staf HQ (NS 09–18).
	staff, own := h.staff("pos", nil, true, emp{name: "Staf HQ"})
	h.nsWeek(own, "2026-01-01")
	expect(t, post(&staff, "2026-11-02", "17:00", "19:00"), 400, "Lembur hari kerja dimulai setelah jam kerja selesai (18:00)")
	expect(t, post(&staff, "2026-11-02", "18:00", "20:00"), 200, "")
	expect(t, post(&staff, "2026-11-07", "10:00", "12:00"), 200, "") // Sabtu: hari istirahat
	// EPIC-089 R2: lebih dari 4 jam tanpa Keterangan ditolak (bukan lagi batas harian).
	expect(t, post(&staff, "2026-11-03", "18:00", "22:30"), 400, domain.OvertimeLongReasonMsg)
	for _, d := range []string{"2026-11-03", "2026-11-04", "2026-11-05"} {
		expect(t, post(&staff, d, "18:00", "22:00"), 200, "")
	}
	// Minggu ini: 2 + 2 + 4 × 3 = 16 jam.
	expect(t, post(&staff, "2026-11-06", "18:00", "21:00"), 400,
		"Lembur minggu 02-11 s.d. 08-11 sudah 16 jam; maksimal 18 jam per minggu (sisa 2 jam)")

	// (f) R3 batas periode: 70 jam di minggu-minggu lain bulan ini.
	busy, busyID := h.staff("pos", nil, true, emp{name: "Staf Sibuk"})
	for _, d := range []string{"2026-11-09", "2026-11-10", "2026-11-11", "2026-11-12", "2026-11-13",
		"2026-11-16", "2026-11-17", "2026-11-18", "2026-11-19", "2026-11-20",
		"2026-11-23", "2026-11-24", "2026-11-25", "2026-11-26"} {
		h.otRow(busyID, d, 5, "pending")
	}
	expectContains(t, post(&busy, "2026-11-02", "18:00", "21:00"), 400,
		"sudah 70 jam; maksimal 72 jam per periode (sisa 2 jam)")

	// (g) Koreksi jam oleh atasan: baris itu sendiri tidak dihitung dua kali.
	bossStaff, bossID := h.staff("pos", nil, true, emp{name: "Atasan HQ"})
	boss := &bossStaff
	crew, crewID := h.staff("pos", nil, true, emp{name: "Staf Dikoreksi", boss: &bossID})
	h.nsWeek(crewID, "2026-01-01")
	expect(t, post(&crew, "2026-11-02", "18:00", "21:00"), 200, "")
	for _, d := range []string{"2026-11-03", "2026-11-04", "2026-11-05"} {
		expect(t, post(&crew, d, "18:00", "22:00"), 200, "")
	}
	fri := post(&crew, "2026-11-06", "18:00", "20:00")
	expect(t, fri, 200, "")
	friID := fri.data()["id"].(string)
	decide := func(end string) call {
		return h.do(boss, "POST", "/api/hris/overtime/decide", map[string]any{"overtime_id": friID, "action": "approve", "end_time": end})
	}
	expect(t, decide("22:00"), 400, "Lembur minggu 02-11 s.d. 08-11 sudah 15 jam; maksimal 18 jam per minggu (sisa 3 jam)")
	// ID huruf besar tetap mengecualikan barisnya sendiri.
	expect(t, h.do(boss, "POST", "/api/hris/overtime/decide",
		map[string]any{"overtime_id": strings.ToUpper(friID), "action": "approve", "end_time": "21:00"}), 200, "")

	// (h) Koreksi HR (karyawan tanpa atasan) ke lebih dari 4 jam sehari: boleh (EPIC-089 R1/R2);
	// minggu itu 16 + 1 = 17 jam, masih dalam 18.
	tue := h.scalar(`SELECT id::text FROM hris.overtime_requests WHERE employee_id = $1 AND date = '2026-11-03'`, own)
	expect(t, h.do(&hrd, "PATCH", "/api/hris/overtime/"+tue, map[string]any{"end_time": "23:00", "reason": "Jam pulang sebenarnya"}),
		200, "")

	// (i) R4: dinas di tanggal merah — staf shift disarankan Weekend/Libur, tanpa cek jam mulai.
	trip, tripID := h.staff("pos", nil, true, emp{name: "Staf Dinas"})
	h.exec(`UPDATE hris.employees SET schedule_type = 'shift' WHERE id = $1`, tripID)
	h.nsWeek(tripID, "2026-01-01")
	h.exec(`INSERT INTO hris.public_holidays (holiday_date, name, type, deducts_leave, status, is_day_off)
		VALUES ('2026-11-10', 'Libur Uji Dinas', 'nasional', false, 'aktif', true)`)
	h.exec(`INSERT INTO hris.business_trips (employee_id, start_date, end_date, destination, purpose, status)
		VALUES ($1, '2026-11-10', '2026-11-11', 'Bandung', 'Pameran', 'approved')`, tripID)
	hint := h.do(&trip, "GET", "/api/hris/overtime/day-type?date=2026-11-10", nil)
	expect(t, hint, 200, "")
	if hint.data()["day_type"] != "rest_day" {
		t.Fatalf("day-type dinas = %s", hint.raw)
	}
	expect(t, post(&trip, "2026-11-10", "08:00", "11:00"), 200, "")

	// (k) EPIC-089: lebih dari 4 jam dengan Keterangan, lintas tengah malam, tanpa batas 12 jam kotor;
	// batas 18 jam/minggu tetap.
	long, longID := h.staff("pos", nil, true, emp{name: "Staf Panjang"})
	h.nsWeek(longID, "2026-01-01")
	why := func(c map[string]any) map[string]any { c["reason"] = "Closing stok akhir bulan"; return c }
	postWhy := func(date, start, end string) call {
		t.Helper()
		next, _ := domain.ParseDate(date)
		h.setNow(next.AddDate(0, 0, 1).Format(domain.DateLayout) + " 09:00")
		return h.do(&long, "POST", "/api/hris/overtime", why(h.otSelfBody(&long, date, start, end)))
	}
	expect(t, postWhy("2026-11-09", "18:00", "22:30"), 200, "") // 4,5 jam
	expect(t, postWhy("2026-11-10", "20:00", "05:00"), 200, "") // 9 jam lintas tengah malam
	expect(t, postWhy("2026-11-11", "18:00", "23:00"), 400,
		"Lembur minggu 09-11 s.d. 15-11 sudah 13,5 jam; maksimal 18 jam per minggu (sisa 4,5 jam)")
	night, nightID := h.staff("pos", nil, true, emp{name: "Staf Malam"})
	h.nsWeek(nightID, "2026-01-01")
	h.setNow("2026-11-10 09:00")
	expect(t, h.do(&night, "POST", "/api/hris/overtime", why(h.otSelfBody(&night, "2026-11-09", "18:00", "07:30"))), 200, "") // 13,5 jam kotor
	// Penugasan HR 6 jam tidak butuh Keterangan panjang (R2 hanya pengajuan sendiri).
	_, hrTargetID := h.staff("pos", nil, true, emp{name: "Staf Ditugaskan"})
	assign6 := otBody("2026-11-16", "18:00", "00:00")
	assign6["employee_id"], assign6["reason"] = hrTargetID, "Stok!"
	expect(t, h.do(&hrd, "POST", "/api/hris/overtime", assign6), 200, "")

	// (j) Naik level setelah mengajukan: pengajuan lama tetap bisa diputuskan.
	promo, promoID := h.staff("pos", nil, true, emp{name: "Staf Promosi", boss: &bossID})
	old := post(&promo, "2026-11-07", "10:00", "12:00")
	expect(t, old, 200, "")
	h.leaderPosition(promoID, "Supervisor")
	expect(t, h.do(boss, "POST", "/api/hris/overtime/decide", map[string]any{"overtime_id": old.data()["id"], "action": "approve"}), 200, "")
}

// Review Focus 4: minggu lintas periode (cutoff 20) dijumlah utuh untuk batas
// mingguan; batas periode hanya periode tanggal itu.
func TestOvertimeTnCWeekAcrossPeriods(t *testing.T) {
	h, _ := newFileHarness(t)
	h.exec(`INSERT INTO hris.attendance_settings (id, cutoff_day, closing_offset_days) VALUES (true, 20, 3)
		ON CONFLICT (id) DO UPDATE SET cutoff_day = 20, closing_offset_days = 3`)
	staff, own := h.staff("pos", nil, true, emp{name: "Staf Lintas Periode"})
	h.nsWeek(own, "2026-01-01")
	for _, d := range []string{"2026-11-16", "2026-11-17", "2026-11-18", "2026-11-19"} {
		h.otRow(own, d, 4, "approved")
	}
	h.setNow("2026-11-22 09:00")
	expect(t, h.do(&staff, "POST", "/api/hris/overtime", h.otSelfBody(&staff, "2026-11-21", "10:00", "13:00")), 400,
		"Lembur minggu 16-11 s.d. 22-11 sudah 16 jam; maksimal 18 jam per minggu (sisa 2 jam)")
	expect(t, h.do(&staff, "POST", "/api/hris/overtime", h.otSelfBody(&staff, "2026-11-21", "10:00", "12:00")), 200, "")
}

// Review akhir: baris lama (sebelum EPIC-085) yang sudah melewati batas tetap
// bisa dikoreksi selama jamnya tidak bertambah.
func TestOvertimeTnCLegacyCorrection(t *testing.T) {
	h, _ := newFileHarness(t)
	hrd, _ := h.staff("hrd", []string{"hris.kepegawaian.overtime"}, true, emp{name: "Admin Lembur"})
	_, own := h.staff("pos", nil, true, emp{name: "Staf Lama"})
	id := h.scalar(`INSERT INTO hris.overtime_requests (employee_id, date, start_time, end_time, hours, source, status, day_type)
		VALUES ($1, '2026-10-05', '17:00', '23:00', 6, 'employee', 'approved', 'regular') RETURNING id::text`, own)
	path := "/api/hris/overtime/" + id
	expect(t, h.do(&hrd, "PATCH", path, map[string]any{"day_type": "rest_day", "reason": "Ternyata hari libur"}), 200, "")
	expect(t, h.do(&hrd, "PATCH", path, map[string]any{"end_time": "22:00", "reason": "Jam pulang sebenarnya"}), 200, "")
	// EPIC-089: tanpa batas harian, menambah jam juga boleh selama batas minggu/periode aman.
	expect(t, h.do(&hrd, "PATCH", path, map[string]any{"end_time": "23:30", "reason": "Menambah jam"}), 200, "")
}

// Review akhir EPIC-089 (Important 1): lembur lintas hari tidak menabrak jam
// masuk besok atau lembur hari sebelumnya; berlaku juga untuk koreksi HR.
func TestOvertimeCrossDayOverlap(t *testing.T) {
	h, _ := newFileHarness(t)
	hrd, _ := h.staff("hrd", []string{"hris.kepegawaian.overtime"}, true, emp{name: "Admin Lembur"})
	staff, own := h.staff("pos", nil, true, emp{name: "Staf Lintas Hari"})
	h.nsWeek(own, "2026-01-01")
	post := func(date, start, end, now string) call {
		t.Helper()
		h.setNow(now)
		body := h.otSelfBody(&staff, date, start, end)
		body["reason"] = "Closing stok akhir bulan"
		return h.do(&staff, "POST", "/api/hris/overtime", body)
	}
	// Senin 18:00 → Selasa 12:00 melewati jam masuk NS 09:00.
	expect(t, post("2026-11-09", "18:00", "12:00", "2026-11-10 13:00"), 400, "Lembur melewati jam masuk kerja 10 Nov 2026 (09:00)")
	// Sabtu 20:00 → Minggu 08:00 (Minggu libur) boleh; Minggu 06:00–10:00 menabraknya.
	expect(t, post("2026-11-14", "20:00", "08:00", "2026-11-15 09:00"), 200, "")
	expect(t, post("2026-11-15", "06:00", "10:00", "2026-11-15 11:00"), 400, "Lembur bertabrakan dengan lembur 14 Nov 2026 (20:00–08:00)")
	// Koreksi HR yang menggeser selesai melewati jam masuk besok.
	mon := post("2026-11-16", "18:00", "22:00", "2026-11-17 09:00")
	expect(t, mon, 200, "")
	expect(t, h.do(&hrd, "PATCH", "/api/hris/overtime/"+mon.data()["id"].(string),
		map[string]any{"end_time": "10:00", "reason": "Jam pulang sebenarnya"}), 400, "Lembur melewati jam masuk kerja 17 Nov 2026 (09:00)")
}

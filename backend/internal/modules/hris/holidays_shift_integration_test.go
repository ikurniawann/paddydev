package hris

import (
	"net/http"
	"testing"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/testutil"
)

// EPIC-079 R2/R10: HR menetapkan cuti bersama sebagai libur; tipe lain ditolak.
func TestCutiBersamaDayOffToggle(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuKepegawaian}, false)
	const msg = "Hanya cuti bersama yang bisa ditetapkan libur"

	cb := h.do(&hr, http.MethodPost, "/api/hris/holidays", map[string]any{
		"holiday_date": "2031-12-24", "name": "Cuti Bersama Natal", "type": "cuti_bersama"})
	expect(t, cb, http.StatusCreated, "")
	if cb.data()["is_day_off"] != false {
		t.Fatalf("default is_day_off = %v", cb.data()["is_day_off"])
	}
	cbID := cb.data()["id"].(string)

	expect(t, h.do(&hr, http.MethodPatch, "/api/hris/holidays/"+cbID, map[string]any{"is_day_off": true}), http.StatusOK, "")
	if h.scalar(`SELECT is_day_off::text FROM hris.public_holidays WHERE id = $1`, cbID) != "true" {
		t.Fatal("is_day_off not stored")
	}
	if h.scalar(`SELECT deducts_leave::text FROM hris.public_holidays WHERE id = $1`, cbID) != "true" {
		t.Fatal("deducts_leave must keep its cuti bersama default")
	}
	expect(t, h.do(&hr, http.MethodPatch, "/api/hris/holidays/"+cbID, map[string]any{"is_day_off": false}), http.StatusOK, "")
	if h.scalar(`SELECT is_day_off::text FROM hris.public_holidays WHERE id = $1`, cbID) != "false" {
		t.Fatal("is_day_off not cleared")
	}

	nas := h.do(&hr, http.MethodPost, "/api/hris/holidays", map[string]any{
		"holiday_date": "2031-12-25", "name": "Natal", "type": "nasional"})
	expect(t, nas, http.StatusCreated, "")
	nasID := nas.data()["id"].(string)
	expect(t, h.do(&hr, http.MethodPatch, "/api/hris/holidays/"+nasID, map[string]any{"is_day_off": true}), http.StatusBadRequest, msg)
	expect(t, h.do(&hr, http.MethodPost, "/api/hris/holidays", map[string]any{
		"holiday_date": "2031-12-26", "name": "Libur X", "type": "perusahaan", "is_day_off": true}), http.StatusBadRequest, msg)

	// Pindah tipe dari cuti bersama membersihkan penetapan libur.
	expect(t, h.do(&hr, http.MethodPatch, "/api/hris/holidays/"+cbID, map[string]any{"is_day_off": true}), http.StatusOK, "")
	expect(t, h.do(&hr, http.MethodPatch, "/api/hris/holidays/"+cbID, map[string]any{"type": "perusahaan"}), http.StatusOK, "")
	if h.scalar(`SELECT is_day_off::text FROM hris.public_holidays WHERE id = $1`, cbID) != "false" {
		t.Fatal("is_day_off must reset when the type leaves cuti_bersama")
	}

	// Impor memakai aturan yang sama dan tidak menghapus penetapan HR saat diimpor ulang.
	imp := func(extra map[string]any) call {
		item := map[string]any{"holiday_date": "2031-12-27", "name": "Cuti Bersama Impor", "type": "cuti_bersama", "source_ref": "uid-1"}
		for k, v := range extra {
			item[k] = v
		}
		return h.do(&hr, http.MethodPost, "/api/hris/holidays/import", map[string]any{"items": []any{item}})
	}
	expect(t, imp(map[string]any{"is_day_off": true}), http.StatusOK, "")
	if h.scalar(`SELECT is_day_off::text FROM hris.public_holidays WHERE source_ref = 'uid-1'`) != "true" {
		t.Fatal("import must store is_day_off")
	}
	expect(t, imp(nil), http.StatusOK, "")
	if h.scalar(`SELECT is_day_off::text FROM hris.public_holidays WHERE source_ref = 'uid-1'`) != "true" {
		t.Fatal("re-import without is_day_off must keep HR's choice")
	}
	expect(t, h.do(&hr, http.MethodPost, "/api/hris/holidays/import", map[string]any{"items": []any{
		map[string]any{"holiday_date": "2031-12-28", "name": "Natal Impor", "type": "nasional", "is_day_off": true}}}),
		http.StatusBadRequest, msg+": Natal Impor")
}

// EPIC-079 R1-R3: tanggal merah tidak meliburkan staf shift (roster yang
// menentukan); HQ dan Operations (Non-Shift) libur di nasional/perusahaan, dan
// di cuti bersama hanya bila ditetapkan libur.
func TestHolidayFollowsScheduleType(t *testing.T) {
	h := newHarness(t)
	h.setNow("2026-12-28 10:00") // Senin; periode Desember masih terbuka
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 0, closing_offset_days = 3`)
	hr, _ := h.staff("hrd", []string{"hris.workforce.attendance"}, false)
	store, storeID := h.staff("pos", nil, true, emp{name: "Staf Toko Merah"})
	hq, hqID := h.staff("pos", nil, true, emp{name: "Staf HQ Merah"})
	bare, bareID := h.staff("pos", nil, true, emp{name: "Staf HQ Tanpa Jadwal"})
	h.exec(`UPDATE hris.employees SET schedule_type = 'non_shift' WHERE id = ANY($1::uuid[])`, []string{hqID, bareID})

	pagi := h.scalar(`INSERT INTO hris.shifts (name, start_time, end_time, late_tolerance_minutes)
		VALUES ('Shift 1 Merah', '08:00', '16:00', 10) RETURNING id::text`)
	ns := h.scalar(`SELECT id::text FROM hris.shifts WHERE code = 'NS'`)
	// Toko: shift per tanggal pada 25-12 (tanggal merah) dan 24-12, libur roster 26-12.
	for _, d := range []string{"2026-12-24", "2026-12-25"} {
		h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id) VALUES ($1, $2, $3, gen_random_uuid())`,
			storeID, d, pagi)
	}
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id) VALUES ($1, '2026-12-26', NULL, gen_random_uuid())`, storeID)
	// HQ: pola mingguan NS Senin-Jumat, Sabtu-Minggu libur.
	for dow := 1; dow <= 7; dow++ {
		var shift any = ns
		if dow > 5 {
			shift = nil
		}
		h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from) VALUES ($1, $2, $3, '2026-01-01')`,
			hqID, dow, shift)
	}
	h.exec(`INSERT INTO hris.public_holidays (holiday_date, name, type, deducts_leave, status, is_day_off)
		VALUES ('2026-12-25', 'Natal Uji', 'nasional', false, 'aktif', false)`)
	h.exec(`INSERT INTO hris.public_holidays (holiday_date, name, type, deducts_leave, status, is_day_off)
		VALUES ('2026-12-24', 'Cuti Bersama Uji', 'cuti_bersama', true, 'aktif', false)`)

	// Kalender absen.
	status := func(date, employeeID string) string {
		t.Helper()
		c := h.do(&hr, "GET", "/api/hris/attendance/daily-roster?date="+date, nil)
		expect(t, c, 200, "")
		for _, e := range c.data()["employees"].([]any) {
			if m := e.(map[string]any); m["employee_id"] == employeeID {
				return m["status"].(string)
			}
		}
		t.Fatalf("employee %s not in roster %s", employeeID, c.raw)
		return ""
	}
	if got := status("2026-12-25", storeID); got != domain.RosterAbsen {
		t.Fatalf("roster toko 25-12 = %q, want %q", got, domain.RosterAbsen)
	}
	if got := status("2026-12-25", hqID); got != domain.RosterLiburNasional {
		t.Fatalf("roster HQ 25-12 = %q", got)
	}
	if got := status("2026-12-25", bareID); got != domain.RosterLiburNasional {
		t.Fatalf("roster HQ tanpa jadwal 25-12 = %q", got)
	}
	if got := status("2026-12-24", hqID); got != domain.RosterAbsen {
		t.Fatalf("roster HQ cuti bersama biasa = %q, want %q", got, domain.RosterAbsen)
	}

	// Absen terlewat (ESS) untuk satu karyawan pada tanggal tertentu.
	gapOn := func(s *testutil.Staff, date string) bool {
		t.Helper()
		c := h.do(s, "GET", "/api/hris/me/attendance-gaps", nil)
		expect(t, c, 200, "")
		for _, d := range c.data()["days"].([]any) {
			if d.(map[string]any)["date"] == date {
				return true
			}
		}
		return false
	}
	if !gapOn(&store, "2026-12-25") {
		t.Fatal("toko harus punya absen terlewat 25-12")
	}
	if gapOn(&hq, "2026-12-25") {
		t.Fatal("HQ tidak boleh punya absen terlewat di libur nasional")
	}
	if !gapOn(&hq, "2026-12-24") {
		t.Fatal("HQ harus punya absen terlewat di cuti bersama biasa")
	}
	h.exec(`UPDATE hris.public_holidays SET is_day_off = true WHERE holiday_date = '2026-12-24'`)
	if gapOn(&hq, "2026-12-24") {
		t.Fatal("HQ tidak boleh punya absen terlewat di cuti bersama yang ditetapkan libur")
	}
	if got := status("2026-12-24", hqID); got != domain.RosterLiburNasional {
		t.Fatalf("roster HQ cuti bersama libur = %q", got)
	}
	h.exec(`UPDATE hris.public_holidays SET is_day_off = false WHERE holiday_date = '2026-12-24'`)

	// Ringkasan HR (per tanggal) memakai aturan yang sama per karyawan.
	summaryNames := func(date string) map[string]bool {
		t.Helper()
		c := h.do(&hr, "GET", "/api/hris/attendance/gap-summary?date="+date, nil)
		expect(t, c, 200, "")
		out := map[string]bool{}
		for _, e := range c.data()["employees"].([]any) {
			out[e.(map[string]any)["employee_id"].(string)] = true
		}
		return out
	}
	if got := summaryNames("2026-12-25"); !got[storeID] || got[hqID] {
		t.Fatalf("ringkasan 25-12 = %v", got)
	}
	if got := summaryNames("2026-12-24"); !got[storeID] || !got[hqID] {
		t.Fatalf("ringkasan 24-12 = %v", got)
	}

	// Jenis hari lembur.
	dayType := func(s *testutil.Staff, date string) any {
		t.Helper()
		c := h.do(s, "GET", "/api/hris/overtime/day-type?date="+date, nil)
		expect(t, c, 200, "")
		return c.data()["day_type"]
	}
	if got := dayType(&store, "2026-12-25"); got != "regular" {
		t.Fatalf("lembur toko di tanggal merah ber-shift = %v", got)
	}
	if got := dayType(&store, "2026-12-26"); got != "rest_day" {
		t.Fatalf("lembur toko di libur roster = %v", got)
	}
	if got := dayType(&hq, "2026-12-25"); got != "rest_day" {
		t.Fatalf("lembur HQ di libur nasional = %v", got)
	}
	if got := dayType(&hq, "2026-12-24"); got != "regular" {
		t.Fatalf("lembur HQ di cuti bersama biasa = %v", got)
	}
	h.exec(`UPDATE hris.public_holidays SET is_day_off = true WHERE holiday_date = '2026-12-24'`)
	if got := dayType(&hq, "2026-12-24"); got != "rest_day" {
		t.Fatalf("lembur HQ di cuti bersama libur = %v", got)
	}
	if got := dayType(&bare, "2026-12-25"); got != "rest_day" {
		t.Fatalf("lembur HQ tanpa jadwal di libur nasional = %v", got)
	}
}

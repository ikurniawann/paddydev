package hris

import (
	"context"
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

// MOM AP-04 (EPIC-063 R6): AdminHRIS = aksi update di menu Lembur.
func TestOvertimeCorrectionByAdminHRIS(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.staff("hrd", []string{"hris.kepegawaian.overtime"}, true, emp{name: "Admin HRIS"})
	viewer := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd",
		Menus: map[string][]string{"hris.kepegawaian.overtime": {"read", "approve"}}})
	t.Cleanup(func() { _ = h.tx.Rollback(context.Background()) })
	staff, _ := h.staff("pos", nil, true, emp{name: "Crew Koreksi"})

	created := h.do(&staff, "POST", "/api/hris/overtime", otBody("2026-10-06", "17:00", "19:00"))
	expect(t, created, 200, "")
	id := created.data()["id"].(string)
	path := "/api/hris/overtime/" + id

	expect(t, h.do(&viewer, "PATCH", path, map[string]any{"end_time": "20:00", "reason": "Jam pulang sebenarnya"}), 403,
		"Insufficient permissions")
	expect(t, h.do(&admin, "PATCH", path, map[string]any{"end_time": "20:00", "reason": " abc "}), 400,
		"Alasan koreksi minimal 5 karakter")
	expect(t, h.do(&admin, "PATCH", path, map[string]any{"reason": "Tidak ada yang diubah"}), 400, "Tidak ada perubahan")
	expect(t, h.do(&admin, "PATCH", path, map[string]any{"end_time": "17:30", "reason": "Lembur cuma setengah jam"}), 400,
		"Durasi lembur minimal 1 jam")
	// P8: tanggal yang tidak ada di kalender → 400, bukan 500.
	expect(t, h.do(&admin, "PATCH", path, map[string]any{"date": "2026-02-30", "reason": "Salah ketik tanggal"}), 400,
		"Tanggal tidak valid")

	fixed := h.do(&admin, "PATCH", path, map[string]any{"date": "2026-10-07", "end_time": "20:30",
		"day_type": "rest_day", "reason": "Sesuai absensi"})
	expect(t, fixed, 200, "")
	if d := fixed.data(); d["hours"] != "3.50" || d["day_type"] != "rest_day" || d["status"] != "pending" ||
		!strings.HasPrefix(d["date"].(string), "2026-10-07") || d["corrected_by_name"] == nil || d["corrected_at"] == nil ||
		fixed.body["message"] != "Lembur dikoreksi" {
		t.Fatalf("corrected = %s", fixed.raw)
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'overtime.correct' AND entity_id = $1
		AND reason = 'Sesuai absensi' AND before->>'end_time' = '19:00' AND before->>'date' = '2026-10-06'
		AND after->>'end_time' = '20:30' AND after->>'date' = '2026-10-07' AND after->>'day_type' = 'rest_day'`, id); n != "1" {
		t.Fatalf("audit koreksi = %s", n)
	}

	// Tanggal bentrok dengan lembur aktif lain → 400 duplikat, bukan 500.
	expect(t, h.do(&staff, "POST", "/api/hris/overtime", otBody("2026-10-09", "17:00", "19:00")), 200, "")
	expect(t, h.do(&admin, "PATCH", path, map[string]any{"date": "2026-10-09", "reason": "Pindah ke Jumat"}), 400,
		"Sudah ada pengajuan lembur pending/approved di tanggal tersebut")

	// Lembur sendiri tidak bisa dikoreksi sendiri.
	mine := h.do(&admin, "POST", "/api/hris/overtime", otBody("2026-10-08", "17:00", "19:00"))
	expect(t, mine, 200, "")
	expect(t, h.do(&admin, "PATCH", "/api/hris/overtime/"+mine.data()["id"].(string),
		map[string]any{"end_time": "21:00", "reason": "Tambah jam sendiri"}), 403, "Tidak bisa mengoreksi lembur sendiri")

	// Yang ditolak tidak bisa dikoreksi.
	other := h.do(&staff, "POST", "/api/hris/overtime", otBody("2026-10-13", "17:00", "19:00"))
	expect(t, other, 200, "")
	otherID := other.data()["id"].(string)
	expect(t, h.do(&admin, "POST", "/api/hris/overtime/decide", map[string]any{"overtime_id": otherID, "action": "reject",
		"rejection_reason": "Tidak perlu lembur"}), 200, "")
	expect(t, h.do(&admin, "PATCH", "/api/hris/overtime/"+otherID, map[string]any{"end_time": "20:00", "reason": "Koreksi jam"}),
		400, "Hanya lembur yang menunggu atau disetujui yang bisa dikoreksi")

	// Periode absensi yang sudah closing.
	h.exec(`DELETE FROM hris.attendance_periods WHERE start_date <= '2026-11-30' AND end_date >= '2026-09-01'`)
	h.exec(`INSERT INTO hris.attendance_periods (period_year, period_month, start_date, end_date, closing_date,
		status, closed_at, close_mode) VALUES (2026, 10, '2026-09-21', '2026-10-20', '2026-10-21', 'closed', now(), 'manual')`)
	expect(t, h.do(&admin, "PATCH", path, map[string]any{"end_time": "21:00", "reason": "Koreksi sesudah closing"}), 409,
		"Periode absensi 21-09-2026 s.d. 20-10-2026 sudah di-closing — lembur tidak bisa dikoreksi")
}

// Ruling C2: administrasi lembur HR memakai himpunan HRD — hiring_manager
// tidak bisa menugaskan lembur ke orang lain, hrd tetap bisa.
func TestOvertimeAssignmentNeedsHRD(t *testing.T) {
	h := newHarness(t)
	hm, _ := h.staff("hiring_manager", []string{menuWorkforce}, true, emp{name: "Manajer Rekrutmen"})
	hrd, _ := h.staff("hrd", []string{menuKepegawaian}, true, emp{name: "HRD Penugas"})
	_, crew := h.staff("pos", nil, true, emp{name: "Crew Ditugaskan"})

	assign := otBody("2026-10-08", "17:00", "19:00")
	assign["employee_id"] = crew
	expect(t, h.do(&hm, "POST", "/api/hris/overtime", assign), 403,
		"Hanya HRD yang bisa membuat penugasan lembur untuk karyawan lain")
	past := otBody("2026-10-02", "17:00", "19:00")
	past["employee_id"] = crew
	expect(t, h.do(&hm, "POST", "/api/hris/overtime", past), 403,
		"Hanya HRD yang bisa membuat penugasan lembur untuk karyawan lain")
	if n := h.scalar(`SELECT count(*)::text FROM hris.overtime_requests WHERE employee_id = $1`, crew); n != "0" {
		t.Fatalf("rows after refused assignment = %s", n)
	}

	ok := h.do(&hrd, "POST", "/api/hris/overtime", assign)
	expect(t, ok, 200, "")
	if ok.data()["source"] != "company" {
		t.Fatalf("assignment = %s", ok.raw)
	}
	expect(t, h.do(&hrd, "POST", "/api/hris/overtime", past), 200, "")
}

// Koreksi tunduk pada penguncian periode yang sama dengan absensi: periode
// yang tanggal closing-nya lewat terkunci walau belum ada yang menekan Closing,
// dan tanggal baru di periode closed ditolak juga.
func TestOvertimeCorrectionHonoursLazyAndNewDateClosing(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.staff("hrd", []string{"hris.kepegawaian.overtime"}, true, emp{name: "Admin HRIS"})
	staff, _ := h.staff("pos", nil, true, emp{name: "Crew Closing"})
	created := h.do(&staff, "POST", "/api/hris/overtime", otBody("2026-10-06", "17:00", "19:00"))
	expect(t, created, 200, "")
	path := "/api/hris/overtime/" + created.data()["id"].(string)

	// Baru dibuat di periode terbuka: tanggal baru di periode closed → 409.
	h.exec(`DELETE FROM hris.attendance_periods WHERE start_date <= '2026-11-30' AND end_date >= '2026-09-01'`)
	h.exec(`INSERT INTO hris.attendance_periods (period_year, period_month, start_date, end_date, closing_date,
		status, closed_at, close_mode) VALUES (2026, 11, '2026-10-21', '2026-11-20', '2026-11-21', 'closed', now(), 'manual')`)
	expect(t, h.do(&admin, "PATCH", path, map[string]any{"date": "2026-11-10", "reason": "Pindah ke bulan depan"}), 409,
		"Periode absensi 21-10-2026 s.d. 20-11-2026 sudah di-closing — lembur tidak bisa dikoreksi")

	// Periode masih 'open' di tabel tetapi closing-nya sudah lewat (hari ini 05-10-2026) → terkunci.
	// Penugasan HR boleh bertanggal lampau (R3), jadi dipakai untuk tanggal di periode lama.
	_, crew := h.staff("pos", nil, true, emp{name: "Crew Periode Lama"})
	old := otBody("2026-09-10", "17:00", "19:00")
	old["employee_id"] = crew
	oldRow := h.do(&admin, "POST", "/api/hris/overtime", old)
	expect(t, oldRow, 200, "")
	oldID := oldRow.data()["id"].(string)
	h.exec(`DELETE FROM hris.attendance_periods WHERE start_date <= '2026-11-30' AND end_date >= '2026-08-01'`)
	h.exec(`INSERT INTO hris.attendance_periods (period_year, period_month, start_date, end_date, closing_date, status)
		VALUES (2026, 9, '2026-08-21', '2026-09-20', '2026-09-21', 'open')`)
	expect(t, h.do(&admin, "PATCH", "/api/hris/overtime/"+oldID, map[string]any{"end_time": "20:00", "reason": "Koreksi periode lewat closing"}),
		409, "Periode absensi 21-08-2026 s.d. 20-09-2026 sudah di-closing — lembur tidak bisa dikoreksi")
	if got := h.scalar(`SELECT end_time::text FROM hris.overtime_requests WHERE id = $1`, oldID); got != "19:00:00" {
		t.Fatalf("end_time = %s, want unchanged", got)
	}
}

// Lembur yang sudah disetujui bisa dikoreksi tanpa mengubah statusnya; alasan wajib.
func TestOvertimeCorrectionOfApprovedRow(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.staff("hrd", []string{"hris.kepegawaian.overtime"}, true, emp{name: "Admin HRIS"})
	staff, _ := h.staff("pos", nil, true, emp{name: "Crew Disetujui"})
	created := h.do(&staff, "POST", "/api/hris/overtime", otBody("2026-10-06", "17:00", "19:00"))
	expect(t, created, 200, "")
	id := created.data()["id"].(string)
	expect(t, h.do(&admin, "POST", "/api/hris/overtime/decide", map[string]any{"overtime_id": id, "action": "approve"}), 200, "")

	expect(t, h.do(&admin, "PATCH", "/api/hris/overtime/"+id, map[string]any{"end_time": "21:00"}), 400, "")
	fixed := h.do(&admin, "PATCH", "/api/hris/overtime/"+id, map[string]any{"end_time": "21:00", "reason": "Jam pulang sebenarnya"})
	expect(t, fixed, 200, "")
	if d := fixed.data(); d["status"] != "approved" || d["hours"] != "4.00" || d["decided_by"] == nil {
		t.Fatalf("corrected approved = %s", fixed.raw)
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'overtime.correct' AND entity_id = $1
		AND before->>'hours' = '2' AND after->>'hours' = '4'`, id); n != "1" {
		t.Fatalf("audit = %s", n)
	}
}

package hris

import (
	"context"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

// otSelfBody is a self-submitted overtime body with one uploaded PDF (EPIC-066).
// fixedNow = Senin 05-10-2026 10:00 WIB: pakai tanggal/jam yang sudah lewat.
func (h *harness) otSelfBody(s *testutil.Staff, date, start, end string) map[string]any {
	h.t.Helper()
	up := h.otUpload(s, "bukti.pdf", otPDF)
	if up.status != 200 {
		h.t.Fatalf("upload = %s", up.raw)
	}
	return map[string]any{"date": date, "start_time": start, "end_time": end,
		"attachments": []string{up.data()["path"].(string)}}
}

func TestOvertimeInputAfterFinished(t *testing.T) {
	h, _ := newFileHarness(t)
	staff, own := h.staff("pos", nil, true, emp{name: "Crew Input"})
	other, _ := h.staff("pos", nil, true, emp{name: "Crew Lain"})
	post := func(body map[string]any) call { return h.do(&staff, "POST", "/api/hris/overtime", body) }

	// Belum selesai: hari ini 09:00–11:00 (sekarang 10:00 WIB).
	expect(t, post(h.otSelfBody(&staff, "2026-10-05", "09:00", "11:00")), 400,
		"Lembur baru bisa diinput setelah selesai (05-10-2026 11:00 WIB)")
	// Lewat tengah malam 04→05 Okt, selesai 02:00: sudah boleh.
	night := h.otSelfBody(&staff, "2026-10-04", "22:00", "02:00")
	night["break_minutes"] = 60
	ok := post(night)
	expect(t, ok, 200, "")
	if d := ok.data(); d["hours"] != "3.00" || d["break_minutes"] != 60.0 || d["status"] != "pending" || d["reason"] != "" {
		t.Fatalf("lembur malam = %s", ok.raw)
	}

	noFile := map[string]any{"date": "2026-10-02", "start_time": "17:00", "end_time": "19:00"}
	expect(t, post(noFile), 400, "Lampirkan minimal 1 dokumentasi lembur")
	tooShort := h.otSelfBody(&staff, "2026-10-02", "17:00", "18:00")
	tooShort["break_minutes"] = 30
	expect(t, post(tooShort), 400, "Durasi lembur minimal 1 jam setelah dikurangi istirahat")
	badBreak := h.otSelfBody(&staff, "2026-10-02", "17:00", "19:00")
	badBreak["break_minutes"] = 721
	expect(t, post(badBreak), 400, "Validation failed")

	foreign := h.otSelfBody(&other, "2026-10-02", "17:00", "19:00") // path milik karyawan lain
	expect(t, post(foreign), 400, "Dokumentasi lembur tidak valid — unggah ulang filenya")
	forged := map[string]any{"date": "2026-10-02", "start_time": "17:00", "end_time": "19:00",
		"attachments": []string{"overtime-attachments/" + own + "/../../x.pdf"}}
	expect(t, post(forged), 400, "Dokumentasi lembur tidak valid — unggah ulang filenya")
	twice := h.otSelfBody(&staff, "2026-10-02", "17:00", "19:00")
	same := twice["attachments"].([]string)[0]
	twice["attachments"] = []string{same, same}
	expect(t, post(twice), 400, "Dokumentasi lembur tidak boleh sama")
	four := h.otSelfBody(&staff, "2026-10-02", "17:00", "19:00")
	paths := four["attachments"].([]string)
	four["attachments"] = []string{paths[0], paths[0], paths[0], paths[0]}
	expect(t, post(four), 400, "Maksimal 3 dokumentasi lembur")

	// Periode absensi yang sudah di-closing → 409. Materialisasi dulu periodenya
	// (periode dibuat saat pertama dibutuhkan), lalu pastikan tertutup.
	_ = h.svc.ensureAttendanceOpen(context.Background(), "2026-09-15")
	h.exec(`UPDATE hris.attendance_periods SET status = 'closed', range_frozen = true
		WHERE '2026-09-15' BETWEEN start_date AND end_date`)
	closed := post(h.otSelfBody(&staff, "2026-09-15", "17:00", "19:00"))
	if closed.status != 409 {
		t.Fatalf("periode tutup = %s", closed.raw)
	}
}

// Penugasan HR tetap memakai aturan lama (EPIC-063 R3): tanpa lampiran dan
// boleh tanggal lampau; HR yang menginput miliknya sendiri ikut aturan baru.
// fixedNow = Senin 05-10-2026 10:00 WIB.
func TestOvertimeHRAssignmentStillBeforeStart(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuKepegawaian}, true)
	_, own := h.staff("pos", nil, true)

	// HR juga tidak boleh mengajukan lembur untuk dirinya sendiri sebelum selesai…
	expect(t, h.do(&hr, "POST", "/api/hris/overtime", otBody("2026-10-06", "18:00", "20:00")), 400,
		"Lembur baru bisa diinput setelah selesai (06-10-2026 20:00 WIB)")
	// …tetapi boleh mencatat penugasan lampau untuk karyawan lain (R3).
	assign := otBody("2026-10-02", "18:00", "20:00")
	assign["employee_id"] = own
	assigned := h.do(&hr, "POST", "/api/hris/overtime", assign)
	expect(t, assigned, 200, "")
	if assigned.data()["source"] != "company" {
		t.Fatalf("assignment = %s", assigned.raw)
	}
}

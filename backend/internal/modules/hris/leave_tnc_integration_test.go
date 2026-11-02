package hris

import (
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/iam"
)

// EPIC-084: aturan pengajuan TnC 03-11 (R2–R6) di jalur HTTP.

func tncLeave(kind, start, end string, attachment ...string) map[string]any {
	body := map[string]any{"leave_type": kind, "start_date": start, "end_date": end, "reason": "Keperluan uji EPIC-084"}
	if len(attachment) > 0 {
		body["attachment_url"] = attachment[0]
	}
	return body
}

func expectCutoff(t *testing.T, c call) {
	t.Helper()
	if c.status != 409 || !strings.Contains(c.raw, "sudah lewat cut off; pengajuan untuk tanggal ini ditutup") {
		t.Fatalf("want 409 cut off, got %d %s", c.status, c.raw)
	}
}

func TestLeaveTnCSubmission(t *testing.T) {
	h, _ := newFileHarness(t)
	h.setNow("2026-11-03 10:00")
	self, own := h.staff("pos", nil, true, emp{name: "Pemohon TnC"})
	const att = "hris/leaves/uji/ktp.jpg"

	// (a) H-3, (b) sakit hari H, (c) periode Oktober sudah lewat cut off.
	expect(t, h.do(&self, "POST", "/api/hris/leaves", tncLeave("annual", "2026-11-05", "2026-11-05")), 400,
		"Cuti Tahunan diajukan paling lambat H-3 (mulai 06-11-2026 atau sesudahnya)")
	expect(t, h.do(&self, "POST", "/api/hris/leaves", tncLeave("sick", "2026-11-02", "2026-11-02")), 400,
		"Sakit diajukan pada hari sakit (tanggal mulai = hari ini)")
	expectCutoff(t, h.do(&self, "POST", "/api/hris/leaves", tncLeave("bereavement", "2026-10-30", "2026-10-30")))
	expect(t, h.do(&self, "POST", "/api/hris/leaves", tncLeave("sick", "2026-11-03", "2026-11-03")), 200, "")

	// (i) tanggal lahir kosong, lalu (e)–(h) Birthday Leave.
	expect(t, h.do(&self, "POST", "/api/hris/leaves", tncLeave("birthday", "2026-11-10", "2026-11-10", att)), 400,
		"Tanggal lahir belum diisi di data karyawan — hubungi HR")
	h.exec(`UPDATE hris.employees SET birth_date = '1995-11-20' WHERE id = $1`, own)
	h.exec(`INSERT INTO hris.employment_contracts (employee_id, contract_number, contract_type, status, start_date, end_date)
		VALUES ($1, 'PKWT-UJI-1', 'pkwt', 'active', '2025-01-10', '2026-12-31')`, own)
	expect(t, h.do(&self, "POST", "/api/hris/leaves", tncLeave("birthday", "2026-11-10", "2026-11-10")), 400,
		"Birthday Leave (Cuti Ulang Tahun) wajib melampirkan dokumen")
	expect(t, h.do(&self, "POST", "/api/hris/leaves", tncLeave("birthday", "2026-11-10", "2026-11-10", att)), 400,
		"Birthday Leave berlaku setelah masa kontrak awal selesai")
	h.exec(`UPDATE hris.employment_contracts SET end_date = '2026-06-30' WHERE employee_id = $1`, own)
	expect(t, h.do(&self, "POST", "/api/hris/leaves", tncLeave("birthday", "2026-12-07", "2026-12-07", att)), 400,
		"Birthday Leave hanya dalam bulan ulang tahun Anda")
	expect(t, h.do(&self, "POST", "/api/hris/leaves", tncLeave("birthday", "2026-11-10", "2026-11-10", att)), 200, "")
	expect(t, h.do(&self, "POST", "/api/hris/leaves", tncLeave("birthday", "2026-11-11", "2026-11-11", att)), 400,
		"Birthday Leave sudah dipakai tahun 2026")

	// (k) batas hari kerja cuti khusus.
	expect(t, h.do(&self, "POST", "/api/hris/leaves", tncLeave("marriage", "2026-11-16", "2026-11-19")), 400,
		"Cuti Menikah maksimal 3 hari kerja")

	// (j) Izin Pribadi hanya bila cuti tahunan habis.
	h.setBalance(own, 2026, 5, 0, 0, 0, 0, nil)
	expect(t, h.do(&self, "POST", "/api/hris/leaves", tncLeave("personal", "2026-11-04", "2026-11-04")), 400,
		"Izin Pribadi hanya untuk yang belum punya hak cuti atau cutinya habis — sisa Cuti Tahunan Anda 5 hari")
	h.setBalance(own, 2026, 0, 0, 0, 0, 0, nil)
	expect(t, h.do(&self, "POST", "/api/hris/leaves", tncLeave("personal", "2026-11-04", "2026-11-04")), 200, "")

	// (l) HR untuk karyawan lain tidak terkena H-n dan syarat Izin Pribadi.
	hr, _ := h.staff("hrd", []string{menuWorkforce, iam.LeafCuti}, true)
	_, other := h.staff("pos", nil, true, emp{name: "Dicatat HR"})
	h.setBalance(other, 2026, 5, 0, 0, 0, 0, nil)
	body := tncLeave("annual", "2026-11-04", "2026-11-04")
	body["employee_id"] = other
	expect(t, h.do(&hr, "POST", "/api/hris/leaves", body), 200, "")
	body = tncLeave("personal", "2026-11-05", "2026-11-05")
	body["employee_id"] = other
	expect(t, h.do(&hr, "POST", "/api/hris/leaves", body), 200, "")

	// (m) lembur H+1, (n) koreksi absen periode lewat cut off.
	expect(t, h.do(&self, "POST", "/api/hris/overtime", h.otSelfBody(&self, "2026-11-01", "17:00", "19:00")), 400,
		"Lembur diajukan paling lambat H+1 (02-11-2026)")
	expect(t, h.do(&self, "POST", "/api/hris/overtime", h.otSelfBody(&self, "2026-11-02", "17:00", "19:00")), 200, "")
	expectCutoff(t, h.do(&self, "POST", "/api/hris/attendance-requests", attReqBody("2026-10-30", "08:00", "17:00", "Lupa absen masuk dan pulang")))
}

// Review Focus 1: cuti yang mulai sebelum cut off 20 dan selesai sesudahnya
// ditolak setelah tanggal 21.
func TestLeaveTnCCutoff20(t *testing.T) {
	h := newHarness(t)
	h.exec(`INSERT INTO hris.attendance_settings (id, cutoff_day, closing_offset_days) VALUES (true, 20, 3)
		ON CONFLICT (id) DO UPDATE SET cutoff_day = 20, closing_offset_days = 3`)
	h.setNow("2026-11-22 09:00")
	self, _ := h.staff("pos", nil, true, emp{name: "Pemohon Cutoff"})
	expectCutoff(t, h.do(&self, "POST", "/api/hris/leaves", tncLeave("bereavement", "2026-11-18", "2026-11-23")))
	expect(t, h.do(&self, "POST", "/api/hris/leaves", tncLeave("bereavement", "2026-11-21", "2026-11-23")), 200, "")
}

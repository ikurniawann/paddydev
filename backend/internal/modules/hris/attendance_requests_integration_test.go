package hris

import (
	"net/http/httptest"
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/testutil"
)

// attReqBody is a request body; "" leaves a clock out.
func attReqBody(date, in, out, reason string) map[string]any {
	body := map[string]any{"date": date, "reason": reason}
	if in != "" {
		body["clock_in"] = in
	}
	if out != "" {
		body["clock_out"] = out
	}
	return body
}

// EPIC-065 R4/R5/R6 (MOM AB-04, K-9): karyawan mengajukan absen manual atau
// koreksi dengan keterangan; HR menerima notifikasi in-app.
func TestAttendanceRequestCreate(t *testing.T) {
	h, _ := newFileHarness(t)
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 0, closing_offset_days = 3`)
	hr, _ := h.staff("hrd", []string{"hris.workforce.attendance"}, true, emp{name: "HR Absensi"})
	crew, crewID := h.staff("pos", nil, true, emp{name: "Crew Pengajuan"})
	other, _ := h.staff("pos", nil, true, emp{name: "Crew Lain"})
	unlinked, _ := h.staff("pos", nil, false)
	// EPIC-067 R11: the hiring manager reads Absensi without "approve".
	manager, _ := h.staffGrants("hiring_manager", readOnly(iam.LeafAbsensi), true, emp{name: "Hiring Manager"})
	const url = "/api/hris/attendance-requests"
	const why = "Internet toko mati, tidak bisa absen"

	// Validasi (R4/R5).
	expect(t, h.do(&unlinked, "POST", url, attReqBody("2026-10-02", "08:00", "17:00", why)), 400,
		"Akun ini tidak tertaut ke data karyawan")
	expect(t, h.do(&crew, "POST", url, map[string]any{"reason": why, "clock_in": "08:00"}), 400, "Tanggal wajib diisi")
	expect(t, h.do(&crew, "POST", url, attReqBody("2026-10-06", "08:00", "", why)), 400,
		"Tanggal pengajuan tidak boleh di masa depan")
	expect(t, h.do(&crew, "POST", url, attReqBody("2026-10-02", "08:00", "", "Lupa")), 400,
		"Keterangan wajib diisi (minimal 5 karakter)")
	expect(t, h.do(&crew, "POST", url, attReqBody("2026-10-02", "8:00", "", why)), 400, "Format jam harus HH:MM")
	expect(t, h.do(&crew, "POST", url, attReqBody("2026-10-02", "", "", why)), 400,
		"Isi jam masuk dan/atau jam pulang yang diajukan")
	expect(t, h.do(&crew, "POST", url, attReqBody("2026-10-02", "", "17:00", why)), 400, "Jam masuk wajib diisi")
	expect(t, h.do(&crew, "POST", url, attReqBody("2026-10-05", "11:00", "", why)), 400, "Jam yang diajukan belum terjadi")
	closed := h.do(&crew, "POST", url, attReqBody("2026-09-30", "08:00", "17:00", why))
	expect(t, closed, 409, "")
	if !strings.HasPrefix(closed.body["error"].(string), "Periode absensi 1–30 Sep 2026 sudah di-closing") {
		t.Fatalf("closed period = %s", closed.raw)
	}
	expect(t, h.do(&crew, "POST", url, map[string]any{"date": "2026-10-02", "clock_in": "08:00", "reason": why,
		"photo": "data:image/gif;base64,R0lG"}), 400, "Lampiran harus berupa gambar JPG/PNG/WebP")

	// Absen manual dengan foto bukti.
	created := h.do(&crew, "POST", url, map[string]any{"date": "2026-10-02", "clock_in": "08:00", "clock_out": "17:00",
		"reason": "  " + why + "  ", "photo": dataURL("image/png", pngBytes(t))})
	expect(t, created, 201, "")
	d := created.data()
	if d["kind"] != "manual" || d["status"] != "pending" || d["reason"] != why || d["clock_in"] != "08:00" ||
		d["clock_out"] != "17:00" || d["recorded"] != nil || d["employee"].(map[string]any)["full_name"] != "Crew Pengajuan" ||
		created.body["message"] != "Pengajuan absensi terkirim — menunggu persetujuan HR" {
		t.Fatalf("created = %s", created.raw)
	}
	path, _ := d["attachment_path"].(string)
	if !strings.HasPrefix(path, "attendance-requests/"+crewID+"/") {
		t.Fatalf("attachment path = %q", path)
	}
	for _, c := range []struct {
		who  *testutil.Staff
		want int
	}{{&crew, 200}, {&hr, 200}, {&other, 403}, {&manager, 403}, {nil, 401}} {
		if rec := h.send(c.who, httptest.NewRequest("GET", "/api/hris/attendance-requests/attachment/"+path, nil)); rec.Code != c.want {
			t.Fatalf("attachment read = %d, want %d", rec.Code, c.want)
		}
	}
	// Satu pengajuan menunggu per tanggal.
	expect(t, h.do(&crew, "POST", url, attReqBody("2026-10-02", "08:05", "", why)), 400,
		"Masih ada pengajuan absensi yang menunggu untuk tanggal ini")
	// HR menerima notifikasi in-app (kanal notify_hrd yang sudah ada, R6).
	if n := h.scalar(`SELECT count(*)::text FROM public.notifications WHERE user_id = $1 AND type = 'approval'
		AND title = 'Pengajuan Absensi: Crew Pengajuan'
		AND message = 'Crew Pengajuan mengajukan absen manual 2 Okt 2026 (masuk 08:00, pulang 17:00). Tinjau di HRIS → Absensi → Pengajuan Absensi.'`,
		hr.UserID); n != "1" {
		t.Fatalf("hr notifications = %s", n)
	}

	// Koreksi: baris absensi sudah ada (belum absen pulang).
	h.exec(`INSERT INTO hris.attendance (employee_id, date, clock_in, status) VALUES ($1, '2026-10-03', '2026-10-03 08:02+07', 'present')`, crewID)
	corr := h.do(&crew, "POST", url, attReqBody("2026-10-03", "", "17:00", "Lupa absen pulang, HP mati"))
	expect(t, corr, 201, "")
	if corr.data()["kind"] != "correction" || corr.data()["recorded"] == nil {
		t.Fatalf("correction = %s", corr.raw)
	}
	corrID := corr.data()["id"].(string)
	h.exec(`INSERT INTO hris.attendance (employee_id, date, clock_in, clock_out, status)
		VALUES ($1, '2026-10-04', '2026-10-04 08:00:41+07', '2026-10-04 17:00+07', 'present')`, crewID)
	expect(t, h.do(&crew, "POST", url, attReqBody("2026-10-04", "08:00", "", "Jam masuk saya benar")), 400,
		"Jam yang diajukan sama dengan absensi tercatat")

	// Daftar milik sendiri.
	mine := h.do(&crew, "GET", "/api/hris/me/attendance-requests", nil)
	expect(t, mine, 200, "")
	if l := mine.list(); len(l) != 2 {
		t.Fatalf("mine = %s", mine.raw)
	}
	if l := h.do(&other, "GET", "/api/hris/me/attendance-requests", nil).list(); len(l) != 0 {
		t.Fatalf("other sees %v", l)
	}

	// Batal: hanya pemohon, hanya saat menunggu; id huruf besar dikanonikkan.
	expect(t, h.do(&crew, "POST", url+"/bukan-uuid/cancel", nil), 400, "ID pengajuan tidak valid")
	expect(t, h.do(&crew, "POST", url+"/ffffffff-ffff-ffff-ffff-ffffffffffff/cancel", nil), 404, "Pengajuan absensi tidak ditemukan")
	expect(t, h.do(&other, "POST", url+"/"+corrID+"/cancel", nil), 403, "Hanya pemohon yang bisa membatalkan pengajuan ini")
	cancelled := h.do(&crew, "POST", url+"/"+strings.ToUpper(corrID)+"/cancel", nil)
	expect(t, cancelled, 200, "")
	if cancelled.data()["status"] != "cancelled" || cancelled.body["message"] != "Pengajuan absensi dibatalkan" {
		t.Fatalf("cancel = %s", cancelled.raw)
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'attendance_request.cancel'
		AND entity_id = $1 AND before->>'status' = 'pending' AND after->>'status' = 'cancelled'`, corrID); n != "1" {
		t.Fatalf("cancel audit = %s", n)
	}
	expect(t, h.do(&crew, "POST", url+"/"+corrID+"/cancel", nil), 400, "Pengajuan sudah dibatalkan")
	// Setelah dibatalkan, tanggal yang sama boleh diajukan lagi.
	expect(t, h.do(&crew, "POST", url, attReqBody("2026-10-03", "", "17:05", "Lupa absen pulang, HP mati")), 201, "")
	// Tanggal kalender yang tidak ada: 400, bukan 500 dari cast ::date.
	expect(t, h.do(&crew, "POST", url, attReqBody("2026-02-30", "08:00", "", why)), 400, "Format tanggal tidak valid")
}

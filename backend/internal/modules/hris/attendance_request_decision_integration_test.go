package hris

import (
	"fmt"
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

// EPIC-065 R2/R3 (matriks approval: Absen Manual / Koreksi Absen | HR): HRD
// memutus; persetujuan menulis absensi lewat penyesuaian HR yang sama.
func TestAttendanceRequestDecision(t *testing.T) {
	h := newHarness(t)
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 0, closing_offset_days = 3`)
	hr, hrID := h.staff("hrd", []string{"hris.workforce.attendance"}, true, emp{name: "HR Pemutus"})
	recruiter, _ := h.staff("hiring_manager", []string{"hris.workforce.attendance"}, true)
	noMenu, _ := h.staff("hrd", nil, true)
	boss, bossID := h.staff("pos", nil, true, emp{name: "Atasan Crew"})
	crew, crewID := h.staff("pos", nil, true, emp{name: "Crew Diputus", boss: &bossID})
	shift := h.scalar(`INSERT INTO hris.shifts (name, start_time, end_time, late_tolerance_minutes)
		VALUES ('Pagi Pengajuan', '08:00', '17:00', 10) RETURNING id::text`)
	for dow := 1; dow <= 7; dow++ {
		h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from) VALUES ($1, $2, $3, '2026-01-01')`,
			crewID, dow, shift)
	}
	const url = "/api/hris/attendance-requests"
	file := func(date, in, out, reason string) string {
		t.Helper()
		c := h.do(&crew, "POST", url, attReqBody(date, in, out, reason))
		expect(t, c, 201, "")
		return c.data()["id"].(string)
	}
	decide := func(s *testutil.Staff, id, action, reason string) call {
		body := map[string]any{"action": action}
		if reason != "" {
			body["rejection_reason"] = reason
		}
		return h.do(s, "POST", url+"/"+id+"/decide", body)
	}

	// Antrean HR: menu Absensi + HRD; filter employee_id dikanonikkan.
	manual := file("2026-10-02", "08:30", "17:00", "Internet toko mati pagi itu")
	queue := h.do(&hr, "GET", url+"?status=pending&employee_id="+strings.ToUpper(crewID), nil)
	expect(t, queue, 200, "")
	if l := queue.list(); len(l) != 1 || l[0].(map[string]any)["id"] != manual || l[0].(map[string]any)["recorded"] != nil {
		t.Fatalf("queue = %s", queue.raw)
	}
	expect(t, h.do(&hr, "GET", url+"?status=menunggu", nil), 400, "Status tidak valid")
	expect(t, h.do(&crew, "GET", url, nil), 403, "")
	expect(t, h.do(&recruiter, "GET", url, nil), 403, "Hanya HRD yang bisa memproses pengajuan absensi")
	expect(t, decide(&noMenu, manual, "approve", ""), 403, "")
	expect(t, decide(&recruiter, manual, "approve", ""), 403, "Hanya HRD yang bisa memproses pengajuan absensi")
	expect(t, decide(&boss, manual, "approve", ""), 403, "") // atasan langsung bukan pemutus
	badges := h.do(&hr, "GET", "/api/hris/nav-badges", nil)
	want := h.scalar(`SELECT count(*)::text FROM hris.attendance_requests WHERE status = 'pending'`)
	if got := badges.body["badges"].(map[string]any)["/dashboard/hris/attendance"]; fmt.Sprint(got) != want {
		t.Fatalf("badge = %v, want %s (%s)", got, want, badges.raw)
	}
	if _, has := h.do(&crew, "GET", "/api/hris/nav-badges", nil).body["badges"].(map[string]any)["/dashboard/hris/attendance"]; has {
		t.Fatal("crew got the HR attendance badge")
	}

	// Setujui absen manual: jalur penyesuaian HR (keterlambatan dari shift, riwayat, audit).
	approved := decide(&hr, strings.ToUpper(manual), "approve", "")
	expect(t, approved, 200, "")
	d := approved.data()
	if d["status"] != "approved" || d["decided_by"] != hrID || d["decided_by_name"] != hr.FullName ||
		approved.body["message"] != "Pengajuan absensi disetujui — absensi diperbarui" {
		t.Fatalf("approve = %s", approved.raw)
	}
	attID, _ := d["attendance_id"].(string)
	if got := h.scalar(`SELECT is_manual::text || '|' || is_late::text || '|' || late_minutes::text || '|' || shift_id::text || '|' ||
		(clock_in AT TIME ZONE 'Asia/Jakarta')::time::text || '|' || notes FROM hris.attendance WHERE id = $1`, attID); got !=
		"true|true|30|"+shift+"|08:30:00|Absen manual dari pengajuan karyawan" {
		t.Fatalf("attendance = %q", got)
	}
	if got := h.scalar(`SELECT action || '|' || reason FROM hris.attendance_adjustments WHERE attendance_id = $1`, attID); got !=
		"create|Pengajuan karyawan: Internet toko mati pagi itu" {
		t.Fatalf("adjustment = %q", got)
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'attendance_request.approve'
		AND entity_id = $1 AND after->>'attendance_id' = $2 AND before->>'status' = 'pending'`, manual, attID); n != "1" {
		t.Fatalf("approve audit = %s", n)
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'attendance.create' AND entity_id = $1`, attID); n != "1" {
		t.Fatalf("attendance audit = %s", n)
	}
	if n := h.scalar(`SELECT count(*)::text FROM public.notifications n JOIN hris.employees e ON e.user_id = n.user_id
		WHERE e.id = $1 AND n.type = 'status_change' AND n.title = 'Pengajuan absensi disetujui'`, crewID); n != "1" {
		t.Fatalf("employee notification = %s", n)
	}
	// Antrean basi: keputusan kedua ditolak tanpa menulis ulang absensi.
	expect(t, decide(&hr, manual, "approve", ""), 400, "Pengajuan sudah disetujui")
	if n := h.scalar(`SELECT count(*)::text FROM hris.attendance_adjustments WHERE attendance_id = $1`, attID); n != "1" {
		t.Fatalf("adjustments after stale approve = %s", n)
	}

	// Koreksi baris "Tidak hadir": jam diisi, status menjadi Hadir.
	h.exec(`INSERT INTO hris.attendance (employee_id, date, status) VALUES ($1, '2026-10-03', 'absent')`, crewID)
	absent := file("2026-10-03", "08:00", "17:00", "Saya hadir, aplikasi error")
	expect(t, decide(&hr, absent, "approve", ""), 200, "")
	if got := h.scalar(`SELECT status::text || '|' || adjustment_count::text FROM hris.attendance
		WHERE employee_id = $1 AND date = '2026-10-03'`, crewID); got != "present|1" {
		t.Fatalf("absent corrected = %q", got)
	}

	// Absen setelah mengajukan absen manual → disetujui sebagai koreksi baris itu.
	late := file("2026-10-04", "08:00", "17:00", "Absen masuk gagal terkirim")
	h.exec(`INSERT INTO hris.attendance (employee_id, date, clock_in, status) VALUES ($1, '2026-10-04', '2026-10-04 09:15+07', 'present')`, crewID)
	expect(t, decide(&hr, late, "approve", ""), 200, "")
	if got := h.scalar(`SELECT count(*)::text || '|' || max((clock_in AT TIME ZONE 'Asia/Jakarta')::time)::text
		FROM hris.attendance WHERE employee_id = $1 AND date = '2026-10-04'`, crewID); got != "1|08:00:00" {
		t.Fatalf("clocked after request = %q", got)
	}
	if got := h.scalar(`SELECT string_agg(action, ',') FROM hris.attendance_adjustments
		WHERE employee_id = $1 AND date = '2026-10-04'`, crewID); got != "update" {
		t.Fatalf("adjustments = %q", got)
	}

	// Tolak: alasan wajib; tidak ada absensi yang ditulis; karyawan diberi tahu alasannya.
	rejected := file("2026-10-01", "08:00", "17:00", "Lupa absen karena sibuk")
	expect(t, decide(&hr, rejected, "reject", ""), 400, "Alasan penolakan wajib diisi")
	rej := decide(&hr, rejected, "reject", "CCTV menunjukkan tidak hadir")
	expect(t, rej, 200, "")
	if rej.data()["status"] != "rejected" || rej.data()["rejection_reason"] != "CCTV menunjukkan tidak hadir" ||
		rej.body["message"] != "Pengajuan absensi ditolak" {
		t.Fatalf("reject = %s", rej.raw)
	}
	if n := h.scalar(`SELECT count(*)::text FROM hris.attendance WHERE employee_id = $1 AND date = '2026-10-01'`, crewID); n != "0" {
		t.Fatalf("rejected request wrote attendance: %s", n)
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'attendance_request.reject' AND entity_id = $1`, rejected); n != "1" {
		t.Fatalf("reject audit = %s", n)
	}
	if n := h.scalar(`SELECT count(*)::text FROM public.notifications n JOIN hris.employees e ON e.user_id = n.user_id
		WHERE e.id = $1 AND n.title = 'Pengajuan absensi ditolak'
		AND n.message = 'Pengajuan absensi 1 Okt 2026 ditolak HR. Alasan: CCTV menunjukkan tidak hadir'`, crewID); n != "1" {
		t.Fatalf("reject notification = %s", n)
	}

	// HR tidak memutus pengajuannya sendiri.
	own := h.do(&hr, "POST", url, attReqBody("2026-10-02", "08:00", "17:00", "Internet kantor mati"))
	expect(t, own, 201, "")
	expect(t, decide(&hr, own.data()["id"].(string), "approve", ""), 403, "Tidak bisa memutuskan pengajuan absensi sendiri")

	// Periode di-closing setelah pengajuan: setujui 409, tetap menunggu, tolak tetap bisa.
	stale := h.scalar(`INSERT INTO hris.attendance_requests (employee_id, date, kind, clock_in, reason)
		VALUES ($1, '2026-09-29', 'manual', '08:00', 'Diajukan sebelum closing') RETURNING id::text`, crewID)
	blocked := decide(&hr, stale, "approve", "")
	expect(t, blocked, 409, "")
	if !strings.HasPrefix(blocked.body["error"].(string), "Periode absensi 1–30 Sep 2026 sudah di-closing") {
		t.Fatalf("closed period approve = %s", blocked.raw)
	}
	if s := h.scalar(`SELECT status FROM hris.attendance_requests WHERE id = $1`, stale); s != "pending" {
		t.Fatalf("stale status = %s", s)
	}
	expect(t, decide(&hr, stale, "reject", "Periode sudah ditutup"), 200, "")
	expect(t, decide(&hr, "bukan-uuid", "approve", ""), 400, "ID pengajuan tidak valid")
	expect(t, decide(&hr, "ffffffff-ffff-ffff-ffff-ffffffffffff", "approve", ""), 404, "Pengajuan absensi tidak ditemukan")
	expect(t, decide(&hr, rejected, "batal", ""), 400, "")
}

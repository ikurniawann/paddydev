package hris

import (
	"strings"
	"testing"
)

// EPIC-065 R7–R8 (MOM AB-05): jam kerja minimum per cabang; label di baris
// absensi. Ruling C1: notifikasi HR lama "Jam Kerja Kurang" (work_hours < 7,
// semua cabang) tidak berubah dan label tidak menambah notifikasi.
func TestShortHoursLabel(t *testing.T) {
	h := newHarness(t)
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 0, closing_offset_days = 3`)
	hr, _ := h.staff("hrd", []string{"hris.master.placements"}, true)
	staff, _ := h.staff("pos", nil, true)
	_, crewID := h.staff("pos", nil, true, emp{name: "Crew Kantor Pusat"})
	hq, store := h.branch("Kantor Pusat Jam"), h.branch("Toko Tanpa Minimum")
	path := "/api/hris/branches/" + hq + "/min-work-hours"
	att := func(date, in string, out *string, branch string) string {
		t.Helper()
		return h.scalar(`INSERT INTO hris.attendance (employee_id, date, clock_in, clock_out, status, branch_id)
			VALUES ($1, $2::text::date, $3::text::timestamptz, $4::text::timestamptz, 'present', $5) RETURNING id::text`,
			crewID, date, in, out, branch)
	}
	label := func(id string) string {
		t.Helper()
		return h.scalar(`SELECT is_short_hours::text || '|' || COALESCE(min_work_hours::text, '') FROM hris.attendance WHERE id = $1`, id)
	}
	const noticeTitle = `⚠️ Jam Kerja Kurang: Crew Kantor Pusat`
	notices := func() string {
		t.Helper()
		return h.scalar(`SELECT count(*)::text FROM public.notifications WHERE user_id = $1 AND title = $2`, hr.UserID, noticeTitle)
	}

	// Baris sebelum aturan ada: September (sudah di-closing) dan Oktober (terbuka).
	h.exec(`SET LOCAL hris.allow_closed_period_write = 'on'`)
	sep := att("2026-09-29", "2026-09-29 09:00+07", ptr("2026-09-29 17:00+07"), hq)
	h.exec(`SET LOCAL hris.allow_closed_period_write = 'off'`)
	early := att("2026-10-01", "2026-10-01 09:00+07", ptr("2026-10-01 17:30+07"), hq)
	if got := label(early); got != "false|" {
		t.Fatalf("before rule = %q", got)
	}

	// Guard & validasi.
	expect(t, h.do(&staff, "PUT", path, map[string]any{"min_work_hours": 9}), 403, "")
	expect(t, h.do(&hr, "PUT", path, map[string]any{"min_work_hours": 0.5}), 400, "Jam kerja minimum 1–24 jam (kelipatan 0,25)")
	expect(t, h.do(&hr, "PUT", path, map[string]any{"min_work_hours": 9.1}), 400, "Jam kerja minimum 1–24 jam (kelipatan 0,25)")
	expect(t, h.do(&hr, "PUT", "/api/hris/branches/bukan-uuid/min-work-hours", map[string]any{"min_work_hours": 9}), 400,
		"ID toko tidak valid")
	expect(t, h.do(&hr, "PUT", "/api/hris/branches/ffffffff-ffff-ffff-ffff-ffffffffffff/min-work-hours",
		map[string]any{"min_work_hours": 9}), 404, "Toko tidak ditemukan")

	// HQ = 9 (id huruf besar dikanonikkan): baris periode terbuka dilabel ulang, September tidak.
	saved := h.do(&hr, "PUT", "/api/hris/branches/"+strings.ToUpper(hq)+"/min-work-hours", map[string]any{"min_work_hours": 9})
	expect(t, saved, 200, "")
	if d := saved.data(); d["min_work_hours"] != "9.00" || d["relabelled"] != 1.0 || d["id"] != hq ||
		saved.body["message"] != "Jam kerja minimum disimpan" {
		t.Fatalf("saved = %s", saved.raw)
	}
	if got := label(early); got != "true|9.00" {
		t.Fatalf("relabelled = %q", got)
	}
	if got := label(sep); got != "false|" {
		t.Fatalf("closed period relabelled: %q", got)
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'branch_min_hours.update'
		AND entity_id = $1 AND after->>'min_work_hours' = '9.00' AND before->>'min_work_hours' IS NULL`, hq); n != "1" {
		t.Fatalf("audit = %s", n)
	}
	// Pelabelan ulang tidak mengirim notifikasi apa pun.
	if n := notices(); n != "0" {
		t.Fatalf("notifications after relabel = %s", n)
	}

	// Batas saat clock-out (trigger): tepat 9 jam tidak berlabel, 8 jam 59 menit berlabel.
	exact := att("2026-10-02", "2026-10-02 09:00+07", nil, hq)
	short := att("2026-10-03", "2026-10-03 09:00+07", nil, hq)
	h.exec(`UPDATE hris.attendance SET clock_out = '2026-10-02 18:00+07' WHERE id = $1`, exact)
	h.exec(`UPDATE hris.attendance SET clock_out = '2026-10-03 17:59+07' WHERE id = $1`, short)
	if got := label(exact); got != "false|9.00" {
		t.Fatalf("exactly 9 h = %q", got)
	}
	if got := label(short); got != "true|9.00" {
		t.Fatalf("8 h 59 m = %q", got)
	}
	// Notifikasi lama (work_hours < 7) tidak berubah: 8 jam 59 menit berlabel
	// kurang jam tetapi tidak memicu notifikasi.
	if n := notices(); n != "0" {
		t.Fatalf("8h59 label sent a notification: %s", n)
	}
	// Cabang tanpa jam minimum: tidak berlabel; 09.00–15.00 tetap memicu notifikasi lama.
	shop := att("2026-10-04", "2026-10-04 09:00+07", nil, store)
	h.exec(`UPDATE hris.attendance SET clock_out = '2026-10-04 15:00+07' WHERE id = $1`, shop)
	if got := label(shop); got != "false|" {
		t.Fatalf("store without minimum = %q", got)
	}
	if n := notices(); n != "1" {
		t.Fatalf("09:00–15:00 at a store: notifications = %s", n)
	}
	if msg := h.scalar(`SELECT message FROM public.notifications WHERE user_id = $1 AND title = $2`, hr.UserID, noticeTitle); msg !=
		"Crew Kantor Pusat bekerja hanya 5.0 jam hari ini." {
		t.Fatalf("notification message = %q", msg)
	}
	// HQ 09.00–15.00: berlabel dan (aturan lama) satu notifikasi.
	hqEarly := att("2026-10-05", "2026-10-05 09:00+07", nil, hq)
	h.exec(`UPDATE hris.attendance SET clock_out = '2026-10-05 15:00+07' WHERE id = $1`, hqEarly)
	if got := label(hqEarly); got != "true|9.00" {
		t.Fatalf("hq early = %q", got)
	}
	if n := notices(); n != "2" {
		t.Fatalf("notifications after hq early = %s", n)
	}

	// Pindah cabang menghitung ulang label; kembali ke HQ melabel lagi.
	h.exec(`UPDATE hris.attendance SET branch_id = $2 WHERE id = $1`, short, store)
	if got := label(short); got != "false|" {
		t.Fatalf("moved to store = %q", got)
	}
	h.exec(`UPDATE hris.attendance SET branch_id = $2 WHERE id = $1`, short, hq)
	if got := label(short); got != "true|9.00" {
		t.Fatalf("moved back = %q", got)
	}
	// Baris tanpa cabang tidak berlabel.
	noBranch := h.scalar(`INSERT INTO hris.attendance (employee_id, date, clock_in, clock_out, status)
		VALUES ($1, '2026-10-06', '2026-10-06 09:00+07', '2026-10-06 10:00+07', 'present') RETURNING id::text`, crewID)
	if got := label(noBranch); got != "false|" {
		t.Fatalf("no branch = %q", got)
	}

	// API: Rekap, Monitoring, dan daftar toko memuat label/jam minimum.
	list := h.do(&hr, "GET", "/api/hris/attendance?employee_id="+crewID+"&start_date=2026-10-01&end_date=2026-10-06", nil)
	expect(t, list, 200, "")
	found := false
	for _, r := range list.list() {
		if row := r.(map[string]any); row["id"] == short {
			found = row["is_short_hours"] == true && row["min_work_hours"] == "9.00"
		}
	}
	if !found {
		t.Fatalf("list = %s", list.raw)
	}
	roster := h.do(&hr, "GET", "/api/hris/attendance/daily-roster?date=2026-10-03", nil)
	expect(t, roster, 200, "")
	var rosterAtt map[string]any
	for _, e := range roster.data()["employees"].([]any) {
		if row := e.(map[string]any); row["employee_id"] == crewID {
			rosterAtt, _ = row["attendance"].(map[string]any)
		}
	}
	if rosterAtt == nil || rosterAtt["is_short_hours"] != true || rosterAtt["min_work_hours"] != 9.0 {
		t.Fatalf("roster attendance = %v", rosterAtt)
	}
	branchMin := ""
	for _, b := range h.do(&staff, "GET", "/api/hris/branches", nil).list() {
		if row := b.(map[string]any); row["id"] == hq {
			branchMin, _ = row["min_work_hours"].(string)
		}
	}
	if branchMin != "9.00" {
		t.Fatalf("branch list min = %q", branchMin)
	}

	// Dikosongkan: baris periode terbuka kembali tanpa label (September tetap).
	cleared := h.do(&hr, "PUT", path, map[string]any{"min_work_hours": nil})
	expect(t, cleared, 200, "")
	if d := cleared.data(); d["min_work_hours"] != nil || d["relabelled"] != 4.0 {
		t.Fatalf("cleared = %s", cleared.raw)
	}
	if got := label(short); got != "false|" {
		t.Fatalf("after clearing = %q", got)
	}
}

package hris

import "testing"

// EPIC-090 R1: riwayat cuti & lembur bawahan langsung, semua status, periode
// sebelumnya s.d. periode berjalan.
func TestTeamHistory(t *testing.T) {
	h, _ := newFileHarness(t)
	h.setNow("2026-11-05 10:00") // periode bulan kalender: Okt (lalu) + Nov (berjalan)
	boss, bossID := h.staff("pos", nil, true, emp{name: "Atasan Riwayat"})
	_, aID := h.staff("pos", nil, true, emp{name: "Anggota A", boss: &bossID})
	_, bID := h.staff("pos", nil, true, emp{name: "Anggota B", boss: &bossID})
	_, outID := h.staff("pos", nil, true, emp{name: "Bukan Tim"})
	unlinked, _ := h.staff("pos", nil, false)
	leave := func(emp, start, status, review, decidedBy, rejection string) {
		h.exec(`INSERT INTO hris.leaves (employee_id, leave_type, start_date, end_date, total_days, reason, status,
			attachment_url, attachment_review, decided_by_name, rejection_reason)
			VALUES ($1::uuid, 'marriage', $2::date, $2::date, 1, 'Acara keluarga', $3::text::public.leave_status,
			'leave-attachments/' || $1::text || '/s.png', nullif($4, ''), nullif($5, ''), nullif($6, ''))`,
			emp, start, status, review, decidedBy, rejection)
	}
	leave(aID, "2026-11-20", "pending", "awaiting", "", "")
	leave(aID, "2026-11-10", "approved", "verified", "Atasan Riwayat", "")
	leave(aID, "2026-10-12", "rejected", "", "Atasan Riwayat", "Stok opname")
	leave(aID, "2026-09-15", "approved", "", "Atasan Riwayat", "") // di luar rentang
	leave(outID, "2026-11-10", "approved", "", "", "")             // bukan tim
	ot := func(emp, date, status, source string) {
		h.exec(`INSERT INTO hris.overtime_requests (employee_id, date, start_time, end_time, hours, source, status, day_type, reason)
			VALUES ($1, $2::date, '18:00', '20:00', 2, $4, $3, 'regular', 'Closing stok')`, emp, date, status, source)
	}
	ot(bID, "2026-10-20", "approved", "employee")
	ot(bID, "2026-11-03", "pending", "company")
	ot(bID, "2026-09-10", "approved", "employee") // di luar rentang

	c := h.do(&boss, "GET", "/api/hris/me/team-history", nil)
	expect(t, c, 200, "")
	d := c.data()
	if p, _ := d["period"].(map[string]any); p["from"] != "2026-10-01" || p["to"] != "2026-11-30" || p["label"] == "" {
		t.Fatalf("period = %s", c.raw)
	}
	leaves, _ := d["leaves"].([]any)
	if len(leaves) != 3 {
		t.Fatalf("leaves = %s", c.raw)
	}
	first := leaves[0].(map[string]any)
	if first["start_date"] != "2026-11-20" || first["attachment_review"] != "awaiting" ||
		first["employee"].(map[string]any)["full_name"] != "Anggota A" || first["attachment_url"] == nil {
		t.Fatalf("leave[0] = %v", first)
	}
	if last := leaves[2].(map[string]any); last["status"] != "rejected" || last["rejection_reason"] != "Stok opname" ||
		last["decided_by_name"] != "Atasan Riwayat" {
		t.Fatalf("leave[2] = %v", last)
	}
	overtime, _ := d["overtime"].([]any)
	if len(overtime) != 2 || overtime[0].(map[string]any)["date"] != "2026-11-03" || overtime[0].(map[string]any)["source"] != "company" {
		t.Fatalf("overtime = %s", c.raw)
	}
	// Akun tanpa karyawan: kosong, bukan error.
	empty := h.do(&unlinked, "GET", "/api/hris/me/team-history", nil)
	expect(t, empty, 200, "")
	if l, _ := empty.data()["leaves"].([]any); len(l) != 0 {
		t.Fatalf("unlinked = %s", empty.raw)
	}
}

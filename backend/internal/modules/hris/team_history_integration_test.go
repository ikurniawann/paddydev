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
		h.historyLeave(emp, start, start, status, review, decidedBy, rejection)
	}
	leave(aID, "2026-11-20", "pending", "awaiting", "", "")
	leave(aID, "2026-11-10", "approved", "verified", "Atasan Riwayat", "")
	leave(aID, "2026-10-12", "rejected", "", "Atasan Riwayat", "Stok opname")
	leave(aID, "2026-09-15", "approved", "", "Atasan Riwayat", "") // di luar rentang
	leave(outID, "2026-11-10", "approved", "", "", "")             // bukan tim
	// Review akhir: cuti setelah periode berjalan dan cuti panjang yang mulai
	// sebelum rentang tetap tampil (tidak hilang setelah diputus).
	leave(aID, "2026-12-05", "approved", "", "Atasan Riwayat", "")
	h.historyLeave(aID, "2026-09-25", "2026-10-03", "approved", "", "Atasan Riwayat", "")
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
	if len(leaves) != 5 {
		t.Fatalf("leaves = %s", c.raw)
	}
	if leaves[0].(map[string]any)["start_date"] != "2026-12-05" || leaves[4].(map[string]any)["start_date"] != "2026-09-25" {
		t.Fatalf("urutan/rentang = %s", c.raw)
	}
	first := leaves[1].(map[string]any)
	if first["start_date"] != "2026-11-20" || first["attachment_review"] != "awaiting" ||
		first["employee"].(map[string]any)["full_name"] != "Anggota A" || first["attachment_url"] == nil {
		t.Fatalf("leave[0] = %v", first)
	}
	if last := leaves[3].(map[string]any); last["status"] != "rejected" || last["rejection_reason"] != "Stok opname" ||
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

func (h *harness) historyLeave(emp, start, end, status, review, decidedBy, rejection string) {
	h.t.Helper()
	h.exec(`INSERT INTO hris.leaves (employee_id, leave_type, start_date, end_date, total_days, reason, status,
		attachment_url, attachment_review, decided_by_name, rejection_reason)
		VALUES ($1::uuid, 'marriage', $2::date, $3::date, 1, 'Acara keluarga', $4::text::public.leave_status,
		'leave-attachments/' || $1::text || '/s.png', nullif($5, ''), nullif($6, ''), nullif($7, ''))`,
		emp, start, end, status, review, decidedBy, rejection)
}

// Review akhir (Important 2): cutoff 20 dan atasan yang reporting_to-nya
// dirinya sendiri.
func TestTeamHistoryCutoff20(t *testing.T) {
	h, _ := newFileHarness(t)
	h.exec(`INSERT INTO hris.attendance_settings (id, cutoff_day, closing_offset_days) VALUES (true, 20, 3)
		ON CONFLICT (id) DO UPDATE SET cutoff_day = 20, closing_offset_days = 3`)
	h.setNow("2026-11-05 10:00")
	boss, bossID := h.staff("pos", nil, true, emp{name: "Atasan Cutoff"})
	h.exec(`UPDATE hris.employees SET reporting_to = id WHERE id = $1`, bossID)
	_, aID := h.staff("pos", nil, true, emp{name: "Anggota Cutoff", boss: &bossID})
	h.historyLeave(aID, "2026-09-21", "2026-09-21", "approved", "", "", "")
	h.historyLeave(aID, "2026-09-20", "2026-09-20", "approved", "", "", "") // sebelum periode lalu
	h.historyLeave(aID, "2026-11-25", "2026-12-01", "pending", "awaiting", "", "")
	h.historyLeave(bossID, "2026-11-10", "2026-11-10", "approved", "", "", "") // cuti atasan sendiri
	c := h.do(&boss, "GET", "/api/hris/me/team-history", nil)
	expect(t, c, 200, "")
	if p, _ := c.data()["period"].(map[string]any); p["from"] != "2026-09-21" || p["to"] != "2026-11-20" {
		t.Fatalf("period = %s", c.raw)
	}
	leaves, _ := c.data()["leaves"].([]any)
	if len(leaves) != 2 || leaves[0].(map[string]any)["start_date"] != "2026-11-25" ||
		leaves[1].(map[string]any)["start_date"] != "2026-09-21" {
		t.Fatalf("leaves = %s", c.raw)
	}
}

// EPIC-091: Riwayat juga memuat kasbon/pinjaman bawahan langsung (lewat port payroll).
func TestTeamHistoryLoans(t *testing.T) {
	h, _ := newFileHarness(t)
	h.setNow("2026-11-05 10:00")
	boss, _ := h.staff("pos", nil, true, emp{name: "SPV Pinjaman"})
	unlinked, _ := h.staff("pos", nil, false)
	approver := "Manager Area"
	purpose := "Biaya sekolah anak"
	fakeTeamLoans = []TeamLoan{{ID: "11111111-1111-4111-8111-111111111111", EmployeeID: "e1", EmployeeName: "Anggota A",
		LoanType: "kasbon", Principal: "1000000.00", Monthly: "500000.00", Status: "pending",
		CreatedAt: "2026-11-04T03:00:00Z", Tenor: 2, Purpose: &purpose, ApproverName: &approver,
		Attachments: []string{"loan-attachments/e1/1-aaaaaaaaaaaaaaaa.pdf"}, Collateral: []string{}}}
	t.Cleanup(func() { fakeTeamLoans = nil })
	c := h.do(&boss, "GET", "/api/hris/me/team-history", nil)
	expect(t, c, 200, "")
	loans, _ := c.data()["loans"].([]any)
	if len(loans) != 1 {
		t.Fatalf("loans = %s", c.raw)
	}
	l := loans[0].(map[string]any)
	if l["loan_type"] != "kasbon" || l["principal_amount"] != "1000000.00" || l["tenor_months"] != 2.0 ||
		l["manager_approver_name"] != "Manager Area" || l["purpose"] != purpose ||
		l["employee"].(map[string]any)["full_name"] != "Anggota A" || len(l["attachments"].([]any)) != 1 {
		t.Fatalf("loan = %v", l)
	}
	empty := h.do(&unlinked, "GET", "/api/hris/me/team-history", nil)
	if l, _ := empty.data()["loans"].([]any); l == nil || len(l) != 0 {
		t.Fatalf("unlinked loans = %s", empty.raw)
	}
}

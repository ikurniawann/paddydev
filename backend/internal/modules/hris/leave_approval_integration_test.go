package hris

import (
	"context"
	"testing"
)

func decideLeaveBody(id any, action, reason string) map[string]any {
	body := map[string]any{"leave_id": id, "action": action}
	if reason != "" {
		body["rejection_reason"] = reason
	}
	return body
}

// EPIC-063 R1/R2 (Q-APV-3): atasan langsung memutus lebih dulu, HR cadangan &
// override tercatat; tidak ada yang memutus cutinya sendiri.
func TestLeaveDecisionManagerFirstHRBackup(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuWorkforce}, true, emp{name: "HR Cadangan"})
	boss, bossID := h.staff("pos", nil, true, emp{name: "Atasan Langsung"})
	staff, _ := h.staff("pos", nil, true, emp{name: "Crew Cuti", boss: &bossID})
	stranger, _ := h.staff("pos", nil, true, emp{name: "Rekan Lain"})

	first := h.do(&staff, "POST", "/api/hris/leaves", leaveBody("annual", "2031-03-04", "2031-03-04"))
	expect(t, first, 200, "")
	firstID := first.data()["id"].(string)
	expect(t, h.do(&stranger, "POST", "/api/hris/leaves/approve", decideLeaveBody(firstID, "approve", "")), 403,
		"Hanya atasan langsung atau HRD yang bisa memproses pengajuan ini")
	approved := h.do(&boss, "POST", "/api/hris/leaves/approve", decideLeaveBody(firstID, "approve", ""))
	expect(t, approved, 200, "")
	if d := approved.data(); d["decided_as"] != "manager" || d["decided_by_name"] == nil || d["approved_by"] != bossID {
		t.Fatalf("manager approve = %s", approved.raw)
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'leave.approve'
		AND entity = 'hris.leaves' AND entity_id = $1 AND after->>'decided_as' = 'manager'`, firstID); n != "1" {
		t.Fatalf("audit manager approve = %s", n)
	}

	// HR override walau atasan ada; atasan yang telat menekan Setujui ditolak.
	second := h.do(&staff, "POST", "/api/hris/leaves", leaveBody("annual", "2031-03-11", "2031-03-11"))
	expect(t, second, 200, "")
	secondID := second.data()["id"].(string)
	override := h.do(&hr, "POST", "/api/hris/leaves/approve", decideLeaveBody(secondID, "reject", "Stok opname akhir bulan"))
	expect(t, override, 200, "")
	if d := override.data(); d["decided_as"] != "hr" || d["status"] != "rejected" {
		t.Fatalf("HR override = %s", override.raw)
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'leave.reject'
		AND entity_id = $1 AND after->>'decided_as' = 'hr' AND actor_id::text = $2`, secondID, hr.UserID); n != "1" {
		t.Fatalf("audit HR reject = %s", n)
	}
	expect(t, h.do(&boss, "POST", "/api/hris/leaves/approve", decideLeaveBody(secondID, "approve", "")), 400,
		"Leave request already rejected")

	// HR tidak memutus cutinya sendiri.
	mine := h.do(&hr, "POST", "/api/hris/leaves", leaveBody("annual", "2031-03-18", "2031-03-18"))
	expect(t, mine, 200, "")
	expect(t, h.do(&hr, "POST", "/api/hris/leaves/approve", decideLeaveBody(mine.data()["id"], "approve", "")), 403,
		"Tidak bisa memutuskan pengajuan cuti sendiri")
}

// Ruling C1/P4: cadangan HR = himpunan IsHRD; hiring_manager (peran rekrutmen)
// yang bukan atasan tidak boleh memutus cuti, juga tidak lewat menu Kepegawaian.
func TestLeaveHiringManagerIsNotHRBackup(t *testing.T) {
	h := newHarness(t)
	hm, _ := h.staff("hiring_manager", []string{menuWorkforce}, true, emp{name: "Manajer Rekrutmen"})
	_, bossID := h.staff("pos", nil, true, emp{name: "Atasan Langsung"})
	staff, _ := h.staff("pos", nil, true, emp{name: "Crew Cuti", boss: &bossID})

	leave := h.do(&staff, "POST", "/api/hris/leaves", leaveBody("annual", "2031-03-04", "2031-03-04"))
	expect(t, leave, 200, "")
	id := leave.data()["id"].(string)
	expect(t, h.do(&hm, "POST", "/api/hris/leaves/approve", decideLeaveBody(id, "approve", "")), 403,
		"Hanya atasan langsung atau HRD yang bisa memproses pengajuan ini")
	if st := h.scalar(`SELECT status::text FROM hris.leaves WHERE id = $1`, id); st != "pending" {
		t.Fatalf("status = %s", st)
	}
}

// Data live: satu karyawan reporting_to-nya dirinya sendiri (EPIC-063 R2).
func TestLeaveSelfManagerCannotApproveOwn(t *testing.T) {
	h, _ := newFileHarness(t)
	loop, loopID := h.staff("pos", nil, true, emp{name: "Retail Ops Loop", phone: "0812 1111 2222"})
	h.exec(`UPDATE hris.employees SET reporting_to = id WHERE id = $1`, loopID)

	leave := h.do(&loop, "POST", "/api/hris/leaves", leaveBody("annual", "2031-03-04", "2031-03-04"))
	expect(t, leave, 200, "")
	if _, err := h.deps.Events.Dispatch(context.Background(), h.tx); err != nil {
		t.Fatal(err)
	}
	if len(h.wa.sent) != 0 {
		t.Fatalf("pemohon tidak boleh menerima WA sebagai atasannya sendiri: %v", h.wa.sent)
	}
	expect(t, h.do(&loop, "POST", "/api/hris/leaves/approve", decideLeaveBody(leave.data()["id"], "approve", "")), 403,
		"Tidak bisa memutuskan pengajuan cuti sendiri")

	expect(t, h.do(&loop, "POST", "/api/hris/overtime", h.otSelfBody(&loop, "2026-10-01", "17:00", "19:00")), 200, "")
	team := h.do(&loop, "GET", "/api/hris/overtime?scope=approvals", nil)
	expect(t, team, 200, "")
	if len(team.list()) != 0 {
		t.Fatalf("lembur sendiri tidak boleh masuk antrean atasan: %s", team.raw)
	}
}

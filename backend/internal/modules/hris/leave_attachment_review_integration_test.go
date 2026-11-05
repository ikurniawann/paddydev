package hris

import (
	"testing"

	"nuhabit/backend/internal/platform/iam"
)

// EPIC-088 (ES-11): cuti berlampiran disetujui atasan langsung, lalu HR
// memverifikasi lampiran; HR bisa mengembalikan agar karyawan unggah ulang.

func attachedLeave(kind, date, employeeID string) map[string]any {
	body := leaveBody(kind, date, date)
	body["attachment_url"] = "leave-attachments/" + employeeID + "/surat.png"
	return body
}

func reviewBody(id any, action, reason string) map[string]any {
	body := map[string]any{"leave_id": id, "action": action}
	if reason != "" {
		body["reason"] = reason
	}
	return body
}

func TestLeaveAttachmentReviewFlow(t *testing.T) {
	h := newHarness(t)
	hr, hrEmp := h.staff("hrd", []string{menuWorkforce, iam.LeafCuti}, true, emp{name: "HR Verifikasi"})
	hr2, _ := h.staff("hrd", []string{menuWorkforce, iam.LeafCuti}, true, emp{name: "HR Kedua"})
	boss, bossID := h.staff("pos", nil, true, emp{name: "Atasan Langsung"})
	staff, staffID := h.staff("pos", nil, true, emp{name: "Crew Lampiran", boss: &bossID})
	h.setBalance(staffID, 2031, 12, 0, 0, 0, 0, nil)
	const review = "/api/hris/leaves/attachment-review"
	file := func(body map[string]any) string {
		t.Helper()
		c := h.do(&staff, "POST", "/api/hris/leaves", body)
		expect(t, c, 200, "")
		return c.data()["id"].(string)
	}
	col := func(id, column string) string {
		t.Helper()
		return h.scalar(`SELECT coalesce(`+column+`::text, '') FROM hris.leaves WHERE id = $1`, id)
	}
	teamLeaves := func() int {
		t.Helper()
		c := h.do(&boss, "GET", "/api/hris/me/team-approvals", nil)
		expect(t, c, 200, "")
		l, _ := c.data()["leaves"].([]any)
		return len(l)
	}

	// Atasan menyetujui Cuti Tahunan berlampiran → menunggu HR (R3).
	annual := file(attachedLeave("annual", "2031-03-04", staffID))
	if n := teamLeaves(); n != 1 {
		t.Fatalf("antrean atasan sebelum setuju = %d", n)
	}
	approved := h.do(&boss, "POST", "/api/hris/leaves/approve", decideLeaveBody(annual, "approve", ""))
	expect(t, approved, 200, "")
	if approved.body["message"] != "Disetujui atasan — menunggu verifikasi lampiran oleh HR" {
		t.Fatalf("message = %v", approved.body["message"])
	}
	if col(annual, "status") != "pending" || col(annual, "attachment_review") != "awaiting" ||
		col(annual, "manager_approved_by") != bossID || col(annual, "manager_approved_name") == "" {
		t.Fatalf("setelah atasan setuju: %s %s %s", col(annual, "status"), col(annual, "attachment_review"), col(annual, "manager_approved_by"))
	}
	if used, _ := h.balanceOf(staffID, 2031); used != "0" && used != "0.0" && used != "0.00" {
		t.Fatalf("saldo terpotong sebelum verifikasi: %s", used)
	}
	if n := h.scalar(`SELECT count(*)::text FROM public.notifications WHERE user_id = $1 AND type = 'approval'
		AND title = 'Lampiran cuti menunggu verifikasi' AND link = '/dashboard/hris/leaves?review=awaiting'`, hr.UserID); n != "1" {
		t.Fatalf("notifikasi HR = %s", n)
	}
	if n := teamLeaves(); n != 0 {
		t.Fatalf("antrean atasan setelah setuju = %d", n)
	}

	// R4: atasan tidak memutus lagi; HR tidak menyetujui lewat /approve.
	const awaitingMsg = "Pengajuan sudah disetujui atasan dan menunggu verifikasi lampiran oleh HR"
	expect(t, h.do(&boss, "POST", "/api/hris/leaves/approve", decideLeaveBody(annual, "approve", "")), 400, awaitingMsg)
	expect(t, h.do(&hr, "POST", "/api/hris/leaves/approve", decideLeaveBody(annual, "approve", "")), 400, awaitingMsg)

	// R5 izin.
	expect(t, h.do(&boss, "POST", review, reviewBody(annual, "verify", "")), 403, "Hanya HR yang bisa memverifikasi lampiran")
	expect(t, h.do(&staff, "POST", review, reviewBody(annual, "verify", "")), 403, "Hanya HR yang bisa memverifikasi lampiran")

	// Kembalikan.
	expect(t, h.do(&hr, "POST", review, reviewBody(annual, "return", "")), 400, "Alasan pengembalian wajib diisi")
	returned := h.do(&hr, "POST", review, reviewBody(annual, "return", "Surat tidak terbaca"))
	expect(t, returned, 200, "")
	if returned.body["message"] != "Lampiran dikembalikan ke karyawan" || col(annual, "attachment_review") != "returned" ||
		col(annual, "attachment_return_reason") != "Surat tidak terbaca" || col(annual, "attachment_reviewed_by") != hr.UserID {
		t.Fatalf("return = %s", returned.raw)
	}
	if n := h.scalar(`SELECT count(*)::text FROM public.notifications WHERE user_id = $1 AND type = 'status_change'
		AND title = 'Lampiran cuti dikembalikan' AND message LIKE '%Surat tidak terbaca%'`, staff.UserID); n != "1" {
		t.Fatalf("notifikasi karyawan kembali = %s", n)
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'leave.attachment_return' AND entity_id = $1`, annual); n != "1" {
		t.Fatalf("audit return = %s", n)
	}
	expect(t, h.do(&hr, "POST", review, reviewBody(annual, "verify", "")), 400, "Pengajuan ini tidak menunggu verifikasi lampiran")

	// R6 unggah ulang.
	expect(t, h.do(&staff, "PUT", "/api/hris/leaves/"+annual,
		map[string]any{"attachment_url": "leave-attachments/" + bossID + "/x.png"}), 400, "Lampiran tidak valid")
	resub := h.do(&staff, "PUT", "/api/hris/leaves/"+annual,
		map[string]any{"attachment_url": "leave-attachments/" + staffID + "/surat-baru.png"})
	expect(t, resub, 200, "")
	if resub.body["message"] != "Lampiran baru terkirim — menunggu verifikasi HR" || col(annual, "attachment_review") != "awaiting" ||
		col(annual, "attachment_return_reason") != "" || col(annual, "attachment_url") != "leave-attachments/"+staffID+"/surat-baru.png" {
		t.Fatalf("unggah ulang = %s", resub.raw)
	}
	if n := h.scalar(`SELECT count(*)::text FROM public.notifications WHERE user_id = $1 AND title = 'Lampiran cuti menunggu verifikasi'`, hr.UserID); n != "2" {
		t.Fatalf("notifikasi HR setelah unggah ulang = %s", n)
	}
	// Unggah ulang hanya saat dikembalikan.
	expect(t, h.do(&staff, "PUT", "/api/hris/leaves/"+annual,
		map[string]any{"attachment_url": "leave-attachments/" + staffID + "/lagi.png"}), 403, "")

	// Verifikasi → disetujui, saldo terpotong sekali (Review Focus 2, 5).
	verified := h.do(&hr, "POST", review, reviewBody(annual, "verify", ""))
	expect(t, verified, 200, "")
	if verified.body["message"] != "Lampiran terverifikasi — cuti disetujui" {
		t.Fatalf("verify = %s", verified.raw)
	}
	if col(annual, "status") != "approved" || col(annual, "approved_by") != bossID || col(annual, "decided_as") != "manager" ||
		col(annual, "attachment_review") != "verified" || col(annual, "attachment_reviewed_name") == "" {
		t.Fatalf("setelah verify: %s %s %s %s", col(annual, "status"), col(annual, "approved_by"), col(annual, "decided_as"), col(annual, "attachment_review"))
	}
	if used, _ := h.balanceOf(staffID, 2031); used != "1" && used != "1.0" && used != "1.00" {
		t.Fatalf("saldo setelah verify = %s", used)
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'leave.attachment_verify' AND entity_id = $1`, annual); n != "1" {
		t.Fatalf("audit verify = %s", n)
	}
	if n := h.scalar(`SELECT count(*)::text FROM public.notifications WHERE user_id = $1 AND type = 'status_change' AND title = 'Cuti disetujui'`, staff.UserID); n != "1" {
		t.Fatalf("notifikasi karyawan disetujui = %s", n)
	}
	expect(t, h.do(&hr2, "POST", review, reviewBody(annual, "verify", "")), 400, "Pengajuan ini tidak menunggu verifikasi lampiran")
	if used, _ := h.balanceOf(staffID, 2031); used != "1" && used != "1.0" && used != "1.00" {
		t.Fatalf("saldo setelah verify kedua = %s", used)
	}

	// HR menolak pengajuan yang menunggu verifikasi lewat /approve.
	rej := file(attachedLeave("annual", "2031-03-11", staffID))
	expect(t, h.do(&boss, "POST", "/api/hris/leaves/approve", decideLeaveBody(rej, "approve", "")), 200, "")
	expect(t, h.do(&hr, "POST", "/api/hris/leaves/approve", decideLeaveBody(rej, "reject", "Dokumen palsu")), 200, "")
	if col(rej, "status") != "rejected" {
		t.Fatalf("HR reject = %s", col(rej, "status"))
	}

	// Pembatalan saat menunggu HR (Review Focus 4).
	cancel := file(attachedLeave("marriage", "2031-03-18", staffID))
	expect(t, h.do(&boss, "POST", "/api/hris/leaves/approve", decideLeaveBody(cancel, "approve", "")), 200, "")
	expect(t, h.do(&staff, "PUT", "/api/hris/leaves/"+cancel, map[string]any{"status": "cancelled"}), 200, "")
	expect(t, h.do(&hr, "POST", review, reviewBody(cancel, "verify", "")), 400, "Pengajuan ini tidak menunggu verifikasi lampiran")

	// Filter review=awaiting (R8).
	waiting := file(attachedLeave("marriage", "2031-04-01", staffID))
	expect(t, h.do(&boss, "POST", "/api/hris/leaves/approve", decideLeaveBody(waiting, "approve", "")), 200, "")
	list := h.do(&hr, "GET", "/api/hris/leaves?review=awaiting", nil)
	expect(t, list, 200, "")
	if rows := list.list(); len(rows) != 1 || rows[0].(map[string]any)["id"] != waiting {
		t.Fatalf("review=awaiting = %s", list.raw)
	}

	// Tanpa lampiran: atasan langsung menyetujui (R2).
	plain := file(leaveBody("marriage", "2031-04-08", "2031-04-08"))
	expect(t, h.do(&boss, "POST", "/api/hris/leaves/approve", decideLeaveBody(plain, "approve", "")), 200, "")
	if col(plain, "status") != "approved" || col(plain, "attachment_review") != "" {
		t.Fatalf("tanpa lampiran = %s %s", col(plain, "status"), col(plain, "attachment_review"))
	}

	// Verifikator memeriksa pengajuannya sendiri → 403.
	mine := h.do(&hr, "POST", "/api/hris/leaves", attachedLeave("marriage", "2031-04-15", hrEmp))
	expect(t, mine, 200, "")
	h.exec(`UPDATE hris.leaves SET attachment_review = 'awaiting' WHERE id = $1`, mine.data()["id"])
	expect(t, h.do(&hr, "POST", review, reviewBody(mine.data()["id"], "verify", "")), 403, "Tidak bisa memverifikasi pengajuan sendiri")
}

// Review Focus 1 + R2: HR yang juga atasan langsung (atau HR cadangan)
// menyetujui pengajuan berlampiran → langsung disetujui dan terverifikasi.
func TestLeaveAttachmentApprovedByVerifier(t *testing.T) {
	h := newHarness(t)
	hrBoss, hrBossID := h.staff("hrd", []string{menuWorkforce, iam.LeafCuti}, true, emp{name: "HR Atasan"})
	staff, staffID := h.staff("pos", nil, true, emp{name: "Bawahan HR", boss: &hrBossID})
	c := h.do(&staff, "POST", "/api/hris/leaves", attachedLeave("marriage", "2031-03-04", staffID))
	expect(t, c, 200, "")
	id := c.data()["id"].(string)
	done := h.do(&hrBoss, "POST", "/api/hris/leaves/approve", decideLeaveBody(id, "approve", ""))
	expect(t, done, 200, "")
	if d := done.data(); d["status"] != "approved" || d["attachment_review"] != "verified" || d["attachment_reviewed_by"] != hrBoss.UserID {
		t.Fatalf("approve oleh verifikator = %s", done.raw)
	}
}

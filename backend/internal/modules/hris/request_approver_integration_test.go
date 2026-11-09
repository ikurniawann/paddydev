package hris

import (
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

// EPIC-091 (ralat user 05-11): penyetuju semua pengajuan = atasan terdekat
// minimal Supervisor; atasan langsung berlevel Staff (mis. Store Leader) dilewati.
func TestRequestApproverMinimumSupervisor(t *testing.T) {
	h, _ := newFileHarness(t)
	h.setNow("2026-10-02 10:00") // lembur 01-10 masih dalam batas H+1
	mgr, mgrID := h.staff("pos", nil, true, emp{name: "Manager Area"})
	h.leaderPosition(mgrID, "Manager")
	spv, spvID := h.staff("pos", nil, true, emp{name: "SPV Toko", boss: &mgrID})
	h.leaderPosition(spvID, "Supervisor")
	leader, leaderID := h.staff("pos", nil, true, emp{name: "Store Leader", boss: &spvID})
	h.leaderPosition(leaderID, "Staff")
	crew, crewID := h.staff("pos", nil, true, emp{name: "Crew Toko", boss: &leaderID})

	queue := func(s *testutil.Staff) (int, int) {
		t.Helper()
		c := h.do(s, "GET", "/api/hris/me/team-approvals", nil)
		expect(t, c, 200, "")
		l, _ := c.data()["leaves"].([]any)
		o, _ := c.data()["overtime"].([]any)
		return len(l), len(o)
	}
	body := attachedLeave("marriage", "2031-03-04", crewID)
	delete(body, "attachment_url")
	leave := h.do(&crew, "POST", "/api/hris/leaves", body)
	expect(t, leave, 200, "")
	expect(t, h.do(&crew, "POST", "/api/hris/overtime", h.otSelfBody(&crew, "2026-10-01", "17:00", "19:00")), 200, "")

	if l, o := queue(&leader); l != 0 || o != 0 {
		t.Fatalf("antrean Store Leader (Staff) = %d cuti, %d lembur; want 0", l, o)
	}
	if l, o := queue(&spv); l != 1 || o != 1 {
		t.Fatalf("antrean SPV = %d cuti, %d lembur; want 1, 1", l, o)
	}
	// EPIC-092: cuti berjenjang (Manager setelah SPV); lembur tetap salah satu (R0c).
	if l, o := queue(&mgr); l != 0 || o != 1 {
		t.Fatalf("antrean Manager = %d cuti, %d lembur; want 0, 1", l, o)
	}
	// Lonceng pengajuan (EPIC-088 R11) ke SPV, bukan Store Leader.
	if n := h.scalar(`SELECT count(*)::text FROM public.notifications WHERE user_id = $1 AND title = 'Pengajuan cuti: Crew Toko'`, spv.UserID); n != "1" {
		t.Fatalf("lonceng SPV = %s", n)
	}
	if n := h.scalar(`SELECT count(*)::text FROM public.notifications WHERE user_id = $1 AND title LIKE 'Pengajuan %'`, leader.UserID); n != "0" {
		t.Fatalf("lonceng Store Leader = %s", n)
	}
	id := leave.data()["id"]
	expect(t, h.do(&leader, "POST", "/api/hris/leaves/approve", decideLeaveBody(id, "approve", "")), 403,
		"Hanya atasan langsung atau HRD yang bisa memproses pengajuan ini")
	ok := h.do(&spv, "POST", "/api/hris/leaves/approve", decideLeaveBody(id, "approve", ""))
	expect(t, ok, 200, "")
	// EPIC-092: ada Manager di atas SPV → tahap SPV dulu, masih menunggu Manager.
	if d := ok.data(); ok.body["message"] != "Disetujui SPV — menunggu persetujuan Manager" || d["spv_approved_by"] != spvID {
		t.Fatalf("keputusan SPV = %s", ok.raw)
	}
	// Riwayat tim mengikuti penyetuju yang sama (SPV dan Manager).
	for _, s := range []*testutil.Staff{&spv, &mgr} {
		hist := h.do(s, "GET", "/api/hris/me/team-history", nil)
		if l, _ := hist.data()["leaves"].([]any); len(l) != 1 {
			t.Fatalf("riwayat = %s", hist.raw)
		}
	}

	// EPIC-092: Manager tidak bisa memutus cuti sebelum SPV.
	second := h.do(&crew, "POST", "/api/hris/leaves", leaveBody("marriage", "2031-03-11", "2031-03-11"))
	expect(t, second, 200, "")
	expect(t, h.do(&mgr, "POST", "/api/hris/leaves/approve", decideLeaveBody(second.data()["id"], "approve", "")), 403,
		"Bukan giliran Anda memutus pengajuan ini")
	// Lembur tidak berjenjang: Manager bisa memutus; perjalanan dinas berjenjang.
	ot := h.scalar(`SELECT id::text FROM hris.overtime_requests WHERE employee_id = $1`, crewID)
	expect(t, h.do(&mgr, "POST", "/api/hris/overtime/decide", map[string]any{"overtime_id": ot, "action": "approve"}), 200, "")
	trip := h.do(&crew, "POST", "/api/hris/business-trips", map[string]any{"start_date": "2026-10-10",
		"end_date": "2026-10-11", "destination": "Gudang Cikarang", "purpose": "Stock opname gudang"})
	expect(t, trip, 200, "")
	expect(t, h.do(&leader, "POST", "/api/hris/business-trips/"+trip.data()["id"].(string)+"/decide",
		map[string]any{"action": "approve"}), 403, "")
	expect(t, h.do(&mgr, "POST", "/api/hris/business-trips/"+trip.data()["id"].(string)+"/decide",
		map[string]any{"action": "approve"}), 403, "Bukan giliran Anda memutus pengajuan ini")
}

package hris

import (
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/testutil"
)

// tieredChain: crew → Koordinator (Staff) → SPV → Manager, plus an HR approver.
type tieredChain struct {
	hr, mgr, spv, koor, crew           testutil.Staff
	hrID, mgrID, spvID, koorID, crewID string
}

func (h *harness) tieredChain() tieredChain {
	h.t.Helper()
	var c tieredChain
	c.hr, c.hrID = h.staff("hrd", []string{menuWorkforce, iam.LeafCuti, iam.LeafAbsensi}, true, emp{name: "HR Mewakili"})
	c.mgr, c.mgrID = h.staff("pos", nil, true, emp{name: "Manager Area"})
	h.leaderPosition(c.mgrID, "Manager")
	c.spv, c.spvID = h.staff("pos", nil, true, emp{name: "SPV Toko", boss: &c.mgrID})
	h.leaderPosition(c.spvID, "Supervisor")
	c.koor, c.koorID = h.staff("pos", nil, true, emp{name: "Koordinator Toko", boss: &c.spvID})
	h.leaderPosition(c.koorID, "Staff")
	c.crew, c.crewID = h.staff("pos", nil, true, emp{name: "Crew Toko", boss: &c.koorID})
	return c
}

func (h *harness) leaveQueue(s *testutil.Staff) int {
	h.t.Helper()
	c := h.do(s, "GET", "/api/hris/me/team-approvals", nil)
	expect(h.t, c, 200, "")
	l, _ := c.data()["leaves"].([]any)
	return len(l)
}

// EPIC-092 R1/R2/R4: cuti berjenjang SPV → Manager; HR mewakili kapan saja.
func TestLeaveTieredApproval(t *testing.T) {
	h := newHarness(t)
	c := h.tieredChain()
	file := func(date string, attached bool) string {
		t.Helper()
		body := leaveBody("marriage", date, date)
		if attached {
			body = attachedLeave("marriage", date, c.crewID)
		}
		r := h.do(&c.crew, "POST", "/api/hris/leaves", body)
		expect(t, r, 200, "")
		return r.data()["id"].(string)
	}
	decide := func(s *testutil.Staff, id, action string) call {
		return h.do(s, "POST", "/api/hris/leaves/approve", decideLeaveBody(id, action, map[string]string{"reject": "Stok opname"}[action]))
	}
	col := func(id, column string) string {
		return h.scalar(`SELECT coalesce(`+column+`::text, '') FROM hris.leaves WHERE id = $1`, id)
	}

	a := file("2031-03-04", false)
	// Koordinator (Staff) dilewati; Manager belum gilirannya.
	if h.leaveQueue(&c.koor) != 0 || h.leaveQueue(&c.mgr) != 0 || h.leaveQueue(&c.spv) != 1 {
		t.Fatalf("antrean awal Koordinator/Manager/SPV = %d/%d/%d", h.leaveQueue(&c.koor), h.leaveQueue(&c.mgr), h.leaveQueue(&c.spv))
	}
	expect(t, decide(&c.koor, a, "approve"), 403, "Hanya atasan langsung atau HRD yang bisa memproses pengajuan ini")
	expect(t, decide(&c.mgr, a, "approve"), 403, "Bukan giliran Anda memutus pengajuan ini")
	bySPV := decide(&c.spv, a, "approve")
	expect(t, bySPV, 200, "")
	if bySPV.body["message"] != "Disetujui SPV — menunggu persetujuan Manager" || col(a, "status") != "pending" ||
		col(a, "spv_approved_by") != c.spvID || col(a, "spv_approved_name") != "SPV Toko" {
		t.Fatalf("SPV setuju = %s", bySPV.raw)
	}
	if n := h.scalar(`SELECT count(*)::text FROM public.notifications WHERE user_id = $1 AND title = 'Pengajuan cuti: Crew Toko'
		AND message LIKE '%Disetujui SPV SPV Toko%'`, c.mgr.UserID); n != "1" {
		t.Fatalf("lonceng Manager setelah SPV = %s", n)
	}
	if h.leaveQueue(&c.spv) != 0 || h.leaveQueue(&c.mgr) != 1 {
		t.Fatalf("antrean setelah SPV SPV/Manager = %d/%d", h.leaveQueue(&c.spv), h.leaveQueue(&c.mgr))
	}
	expect(t, decide(&c.spv, a, "approve"), 403, "Bukan giliran Anda memutus pengajuan ini")
	expect(t, decide(&c.mgr, a, "approve"), 200, "")
	if col(a, "status") != "approved" || col(a, "approved_by") != c.mgrID {
		t.Fatalf("Manager setuju = %s/%s", col(a, "status"), col(a, "approved_by"))
	}

	// HR mewakili kedua tahap (atasan tidak available).
	b := file("2031-03-11", false)
	first := decide(&c.hr, b, "approve")
	expect(t, first, 200, "")
	if col(b, "status") != "pending" || col(b, "spv_approved_name") != "Go Test HR (mewakili SPV Toko)" && !strings.Contains(col(b, "spv_approved_name"), "(mewakili SPV Toko)") {
		t.Fatalf("HR mewakili SPV = %s %s", col(b, "status"), col(b, "spv_approved_name"))
	}
	expect(t, decide(&c.hr, b, "approve"), 200, "")
	if col(b, "status") != "approved" || col(b, "decided_as") != "hr" || !strings.Contains(col(b, "decided_by_name"), "(mewakili Manager Area)") {
		t.Fatalf("HR mewakili Manager = %s %s %s", col(b, "status"), col(b, "decided_as"), col(b, "decided_by_name"))
	}

	// SPV menolak di tahapnya.
	r := file("2031-03-18", false)
	expect(t, decide(&c.spv, r, "reject"), 200, "")
	if col(r, "status") != "rejected" {
		t.Fatalf("SPV tolak = %s", col(r, "status"))
	}

	// Berlampiran: setelah tahap Manager baru menunggu verifikasi HR (EPIC-088).
	d := file("2031-03-25", true)
	expect(t, decide(&c.spv, d, "approve"), 200, "")
	if col(d, "attachment_review") != "" {
		t.Fatalf("lampiran menunggu HR setelah SPV saja: %s", col(d, "attachment_review"))
	}
	expect(t, decide(&c.mgr, d, "approve"), 200, "")
	if col(d, "status") != "pending" || col(d, "attachment_review") != "awaiting" {
		t.Fatalf("setelah Manager = %s/%s", col(d, "status"), col(d, "attachment_review"))
	}
}

// EPIC-092 R3/R4: perjalanan dinas berjenjang SPV → Manager; HR mewakili.
func TestTripTieredApproval(t *testing.T) {
	h, _ := newFileHarness(t)
	h.setNow("2026-10-02 10:00")
	c := h.tieredChain()
	file := func(start string) string {
		t.Helper()
		r := h.do(&c.crew, "POST", "/api/hris/business-trips", map[string]any{"start_date": start, "end_date": start,
			"destination": "Gudang Cikarang", "purpose": "Stock opname gudang"})
		expect(t, r, 200, "")
		return r.data()["id"].(string)
	}
	queue := func(s *testutil.Staff) int {
		t.Helper()
		r := h.do(s, "GET", "/api/hris/business-trips?scope=approvals&status=pending", nil)
		expect(t, r, 200, "")
		return len(r.list())
	}
	decide := func(s *testutil.Staff, id string) call {
		return h.do(s, "POST", "/api/hris/business-trips/"+id+"/decide", map[string]any{"action": "approve"})
	}
	col := func(id, column string) string {
		return h.scalar(`SELECT coalesce(`+column+`::text, '') FROM hris.business_trips WHERE id = $1`, id)
	}
	a := file("2026-10-10")
	if queue(&c.koor) != 0 || queue(&c.mgr) != 0 || queue(&c.spv) != 1 {
		t.Fatalf("antrean Koordinator/Manager/SPV = %d/%d/%d", queue(&c.koor), queue(&c.mgr), queue(&c.spv))
	}
	expect(t, decide(&c.koor, a), 403, "Hanya atasan langsung atau HRD yang bisa memproses pengajuan ini")
	expect(t, decide(&c.mgr, a), 403, "Bukan giliran Anda memutus pengajuan ini")
	bySPV := decide(&c.spv, a)
	expect(t, bySPV, 200, "")
	if bySPV.body["message"] != "Disetujui SPV — menunggu persetujuan Manager" || col(a, "status") != "pending" || col(a, "spv_approved_by") != c.spvID {
		t.Fatalf("SPV setuju = %s", bySPV.raw)
	}
	if n := h.scalar(`SELECT count(*)::text FROM public.notifications WHERE user_id = $1 AND title = 'Pengajuan perjalanan dinas: Crew Toko'`, c.mgr.UserID); n != "1" {
		t.Fatalf("lonceng Manager = %s", n)
	}
	if queue(&c.spv) != 0 || queue(&c.mgr) != 1 {
		t.Fatalf("antrean setelah SPV = %d/%d", queue(&c.spv), queue(&c.mgr))
	}
	expect(t, decide(&c.mgr, a), 200, "")
	if col(a, "status") != "approved" {
		t.Fatalf("Manager setuju = %s", col(a, "status"))
	}
	b := file("2026-10-20")
	expect(t, decide(&c.hr, b), 200, "")
	if col(b, "status") != "pending" || !strings.Contains(col(b, "spv_approved_name"), "(mewakili SPV Toko)") {
		t.Fatalf("HR mewakili SPV = %s %s", col(b, "status"), col(b, "spv_approved_name"))
	}
	expect(t, decide(&c.hr, b), 200, "")
	if col(b, "status") != "approved" || !strings.Contains(col(b, "decided_by_name"), "(mewakili Manager Area)") {
		t.Fatalf("HR mewakili Manager = %s %s", col(b, "status"), col(b, "decided_by_name"))
	}
}

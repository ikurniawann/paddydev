package hris

import "testing"

// EPIC-063 R7: antrean Persetujuan Tim = cuti & lembur pending bawahan
// langsung; badge ikut Perjalanan Dinas dan kasbon tahap manager.
func TestTeamApprovalsQueueAndBadge(t *testing.T) {
	h := newHarness(t)
	boss, bossID := h.staff("pos", nil, true, emp{name: "Atasan Tim"})
	memberA, _ := h.staff("pos", nil, true, emp{name: "Anggota A", boss: &bossID})
	memberB, _ := h.staff("pos", nil, true, emp{name: "Anggota B", boss: &bossID})
	outsider, _ := h.staff("pos", nil, true, emp{name: "Bukan Tim"})
	unlinked, _ := h.staff("pos", nil, false)
	// Data live: atasan yang reporting_to-nya dirinya sendiri.
	h.exec(`UPDATE hris.employees SET reporting_to = id WHERE id = $1`, bossID)
	fakeManagerLoans = 2
	t.Cleanup(func() { fakeManagerLoans = 0 })

	expect(t, h.do(&memberA, "POST", "/api/hris/leaves", leaveBody("annual", "2031-03-04", "2031-03-04")), 200, "")
	expect(t, h.do(&outsider, "POST", "/api/hris/leaves", leaveBody("annual", "2031-03-04", "2031-03-04")), 200, "")
	expect(t, h.do(&memberB, "POST", "/api/hris/overtime", otBody("2026-10-06", "17:00", "19:00")), 200, "")
	expect(t, h.do(&boss, "POST", "/api/hris/overtime", otBody("2026-10-06", "17:00", "19:00")), 200, "")
	expect(t, h.do(&memberA, "POST", "/api/hris/business-trips", map[string]any{"start_date": "2026-10-10",
		"end_date": "2026-10-11", "destination": "Gudang Cikarang", "purpose": "Stock opname gudang"}), 200, "")

	q := h.do(&boss, "GET", "/api/hris/me/team-approvals", nil)
	expect(t, q, 200, "")
	leaves, _ := q.data()["leaves"].([]any)
	overtime, _ := q.data()["overtime"].([]any)
	if len(leaves) != 1 || len(overtime) != 1 {
		t.Fatalf("queue = %s", q.raw)
	}
	l := leaves[0].(map[string]any)
	if l["start_date"] != "2031-03-04" || l["leave_type"] != "annual" || l["employee"].(map[string]any)["full_name"] != "Anggota A" {
		t.Fatalf("leave = %v", l)
	}
	o := overtime[0].(map[string]any)
	if o["date"] != "2026-10-06" || o["day_type"] != "regular" || o["employee"].(map[string]any)["full_name"] != "Anggota B" {
		t.Fatalf("overtime = %v", o)
	}

	badges := h.do(&boss, "GET", "/api/hris/nav-badges", nil)
	if got := badges.body["badges"].(map[string]any)["/dashboard/me/persetujuan-tim"]; got != 5.0 {
		t.Fatalf("badge = %v (want 1 cuti + 1 lembur + 1 dinas + 2 kasbon) in %s", got, badges.raw)
	}

	expect(t, h.do(&boss, "POST", "/api/hris/leaves/approve", map[string]any{"leave_id": l["id"], "action": "approve"}), 200, "")
	after := h.do(&boss, "GET", "/api/hris/me/team-approvals", nil)
	if left, _ := after.data()["leaves"].([]any); len(left) != 0 {
		t.Fatalf("cuti yang diputus masih di antrean: %s", after.raw)
	}

	empty := h.do(&unlinked, "GET", "/api/hris/me/team-approvals", nil)
	expect(t, empty, 200, "")
	if empty.raw != `{"data":{"leaves":[],"overtime":[]}}` {
		t.Fatalf("akun tanpa karyawan = %s", empty.raw)
	}
	expect(t, h.do(nil, "GET", "/api/hris/me/team-approvals", nil), 401, "Unauthorized")
}

package hris

import (
	"testing"
)

// EPIC-088 R11 (permintaan user 05-11): atasan langsung mendapat lonceng
// in-app saat bawahannya mengajukan cuti/izin atau lembur.
func TestManagerBellOnRequest(t *testing.T) {
	h, _ := newFileHarness(t)
	h.setNow("2026-10-02 10:00") // lembur 01-10 masih dalam batas H+1 (EPIC-084 R6)
	boss, bossID := h.staff("pos", nil, true, emp{name: "Atasan Bel"})
	staff, _ := h.staff("pos", nil, true, emp{name: "Crew Bel", boss: &bossID})
	loner, _ := h.staff("pos", nil, true, emp{name: "Tanpa Atasan"})
	loop, loopID := h.staff("pos", nil, true, emp{name: "Atasan Sendiri"})
	h.exec(`UPDATE hris.employees SET reporting_to = id WHERE id = $1`, loopID)
	bells := func(userID, title string) string {
		t.Helper()
		return h.scalar(`SELECT count(*)::text FROM public.notifications WHERE user_id = $1 AND type = 'approval'
			AND title = $2 AND link = '/dashboard/me/persetujuan-tim'`, userID, title)
	}

	expect(t, h.do(&staff, "POST", "/api/hris/leaves", leaveBody("marriage", "2031-03-04", "2031-03-05")), 200, "")
	if n := bells(boss.UserID, "Pengajuan cuti: Crew Bel"); n != "1" {
		t.Fatalf("lonceng cuti atasan = %s", n)
	}
	if n := h.scalar(`SELECT count(*)::text FROM public.notifications WHERE user_id = $1 AND message LIKE 'Crew Bel mengajukan Cuti Menikah%'`, boss.UserID); n != "1" {
		t.Fatalf("pesan cuti atasan = %s", n)
	}
	expect(t, h.do(&staff, "POST", "/api/hris/overtime", h.otSelfBody(&staff, "2026-10-01", "17:00", "19:00")), 200, "")
	if n := bells(boss.UserID, "Pengajuan lembur: Crew Bel"); n != "1" {
		t.Fatalf("lonceng lembur atasan = %s", n)
	}

	// Tanpa atasan, atau atasan dirinya sendiri: tidak ada lonceng, pengajuan tetap tersimpan.
	expect(t, h.do(&loner, "POST", "/api/hris/leaves", leaveBody("marriage", "2031-03-04", "2031-03-04")), 200, "")
	expect(t, h.do(&loop, "POST", "/api/hris/leaves", leaveBody("marriage", "2031-03-04", "2031-03-04")), 200, "")
	if n := bells(loop.UserID, "Pengajuan cuti: Atasan Sendiri"); n != "0" {
		t.Fatalf("lonceng ke diri sendiri = %s", n)
	}
}

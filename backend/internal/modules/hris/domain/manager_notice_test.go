package domain

import "testing"

// EPIC-088 R11 (permintaan user 05-11): lonceng ke atasan langsung saat
// cuti/izin atau lembur diajukan.
func TestManagerNotices(t *testing.T) {
	title, msg := LeaveManagerNotice("Budi", "marriage", "2026-11-16", "2026-11-17", 2)
	if title != "Pengajuan cuti: Budi" ||
		msg != "Budi mengajukan Cuti Menikah 16 Nov 2026 s.d. 17 Nov 2026 (2 hari). Tinjau di Area Karyawan → Persetujuan Tim." {
		t.Errorf("cuti = %q / %q", title, msg)
	}
	if _, msg := LeaveManagerNotice("Budi", "personal", "2026-11-16", "2026-11-16", 1); msg !=
		"Budi mengajukan Izin Pribadi 16 Nov 2026 (1 hari). Tinjau di Area Karyawan → Persetujuan Tim." {
		t.Errorf("cuti 1 hari = %q", msg)
	}
	title, msg = OvertimeManagerNotice("Budi", "2026-11-16", "17:00", "19:30", 2.5)
	if title != "Pengajuan lembur: Budi" ||
		msg != "Budi mengajukan lembur 16 Nov 2026 17:00–19:30 (2,5 jam). Tinjau di Area Karyawan → Persetujuan Tim." {
		t.Errorf("lembur = %q / %q", title, msg)
	}
}

package domain

import "testing"

// EPIC-067 R7: teks notifikasi harian ringkasan absen terlewat untuk HR.
func TestGapDigestNotice(t *testing.T) {
	title, message := GapDigestNotice("2026-10-25", 5, 2, 3)
	if title != "Ringkasan Absen Terlewat" {
		t.Fatalf("title = %q", title)
	}
	want := "Kemarin (Min, 25 Okt 2026): 5 karyawan belum absen, 2 belum absen pulang. 3 karyawan tanpa jadwal tidak dihitung."
	if message != want {
		t.Fatalf("message = %q, want %q", message, want)
	}
	_, message = GapDigestNotice("2026-10-26", 0, 1, 0)
	want = "Kemarin (Sen, 26 Okt 2026): 0 karyawan belum absen, 1 belum absen pulang."
	if message != want {
		t.Fatalf("no unscheduled: message = %q, want %q", message, want)
	}
}

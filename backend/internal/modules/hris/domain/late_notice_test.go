package domain

import "testing"

func TestLateNotice(t *testing.T) {
	selfTitle, selfMsg, teamTitle, teamMsg := LateNotice("Ana", 25, 3)
	want := []string{
		"Anda terlambat hari ini",
		"Anda terlambat 25 menit hari ini (3 kali di periode ini). Hati-hati, jika berlanjut akan ada teguran dari HR.",
		"Ana terlambat hari ini",
		"Ana terlambat 25 menit hari ini (3 kali di periode ini).",
	}
	for i, got := range []string{selfTitle, selfMsg, teamTitle, teamMsg} {
		if got != want[i] {
			t.Errorf("LateNotice[%d] = %q, want %q", i, got, want[i])
		}
	}
}

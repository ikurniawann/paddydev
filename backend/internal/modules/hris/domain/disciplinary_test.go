package domain

import "testing"

func TestDisciplinaryLabel(t *testing.T) {
	want := map[string]string{
		"teguran": "Surat Teguran", "sp1": "Surat Peringatan 1",
		"sp2": "Surat Peringatan 2", "sp3": "Surat Peringatan 3", "x": "",
	}
	for code, label := range want {
		if got := DisciplinaryLabel(code); got != label {
			t.Errorf("DisciplinaryLabel(%q) = %q, want %q", code, got, label)
		}
	}
	if len(DisciplinaryCodes) != 4 {
		t.Fatalf("DisciplinaryCodes = %v", DisciplinaryCodes)
	}
}

package domain

import "testing"

func TestKasbonBlockedStatus(t *testing.T) {
	for s, want := range map[string]bool{
		"probation": true, "Freelance": true, "intern": true, " internship ": true,
		"contract": false, "permanent": false, "daily_worker": false, "": false,
	} {
		if got := KasbonBlockedStatus(s); got != want {
			t.Errorf("KasbonBlockedStatus(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestIsManagerLevel(t *testing.T) {
	for l, want := range map[string]bool{
		"Manager": true, "manager": true, "Senior Manager": true, "General Manager": true, "Director": true, "C-Level": true,
		"Assistant Manager": false, "Supervisor": false, "Staff": false, "": false,
	} {
		if got := IsManagerLevel(l); got != want {
			t.Errorf("IsManagerLevel(%q) = %v, want %v", l, got, want)
		}
	}
}

func TestValidateKasbon(t *testing.T) {
	thp := 5_000_000.0
	ok := func(principal float64, tenor int, status string) string {
		return ValidateKasbon(principal, tenor, status, &thp, 0, 1, true)
	}
	if msg := ok(2_500_000, 2, "contract"); msg != "" {
		t.Fatalf("50%% THP tenor 2 harus lolos: %q", msg)
	}
	if msg := ok(2_500_001, 1, "contract"); msg != "Kasbon maksimal 50% THP (maks Rp 2.500.000)" {
		t.Fatalf("melebihi plafon: %q", msg)
	}
	if msg := ValidateKasbon(2_500_001, 1, "contract", &thp, 0, 1, false); msg != "Kasbon maksimal 50% THP" {
		t.Fatalf("tanpa akses Penggajian tidak boleh menyebut angka: %q", msg)
	}
	if msg := ok(1_000_000, 3, "contract"); msg != "Tenor kasbon hanya 1 atau 2 bulan" {
		t.Fatalf("tenor 3: %q", msg)
	}
	if msg := ok(1_000_000, 2, "probation"); msg != "Karyawan berstatus Probation/Freelance/Magang tidak dapat mengajukan kasbon" {
		t.Fatalf("probation: %q", msg)
	}
	if msg := ValidateKasbon(1_000_000, 2, "contract", nil, 0, 1, true); msg != "Karyawan belum punya struktur gaji aktif — atur gaji dulu sebelum mengajukan pinjaman" {
		t.Fatalf("tanpa struktur gaji: %q", msg)
	}
	if msg := ValidateKasbon(1_000_000, 2, "contract", &thp, 1, 1, true); msg != "Karyawan sudah punya 1 pinjaman aktif (maks 1)" {
		t.Fatalf("pinjaman aktif: %q", msg)
	}
}

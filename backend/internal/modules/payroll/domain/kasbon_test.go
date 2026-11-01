package domain

import (
	"testing"
	"time"
)

func TestLoanEligibleStatus(t *testing.T) {
	for s, want := range map[string]bool{
		"contract": true, "pkwt": true, "permanent": true, " Permanent ": true, "PKWT": true,
		"probation": false, "intern": false, "freelance": false, "daily_worker": false, "": false,
	} {
		if got := LoanEligibleStatus(s); got != want {
			t.Errorf("LoanEligibleStatus(%q) = %v, want %v", s, got, want)
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

func TestLoanServiceMonths(t *testing.T) {
	today := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		join   string
		months int
		ok     bool
		known  bool
	}{
		{"2026-08-01", 3, true, true},
		{"2026-08-02", 3, false, true},
		{"2025-11-01", 12, true, true},
		{"2025-11-02", 12, false, true},
		{"2026-11-01T00:00:00Z", 0, true, true},
		{"", 3, false, false},
		{"bukan tanggal", 3, false, false},
	} {
		ok, known := ServiceMonthsReached(c.join, today, c.months)
		if ok != c.ok || known != c.known {
			t.Errorf("ServiceMonthsReached(%q, %d) = %v,%v want %v,%v", c.join, c.months, ok, known, c.ok, c.known)
		}
	}
	// 31 Mei + 3 bulan = 31 Agustus; 30 Agustus belum.
	if ok, _ := ServiceMonthsReached("2026-05-31", time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC), 3); ok {
		t.Error("30 Agustus belum genap 3 bulan sejak 31 Mei")
	}
}

func TestValidateLoanTnC(t *testing.T) {
	today := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	thp := 5_000_000.0
	senior := LoanApplicant{Status: "permanent", JoinDate: "2024-01-15"}
	check := func(loanType string, principal float64, tenor int, emp LoanApplicant, showCap bool) string {
		return ValidateLoanTnC(loanType, principal, tenor, emp, today, &thp, 0, 1, showCap)
	}
	for _, c := range []struct {
		name, loanType string
		principal      float64
		tenor          int
		emp            LoanApplicant
		showCap        bool
		want           string
	}{
		{"kasbon pas 50%", "kasbon", 2_500_000, 2, senior, true, ""},
		{"kasbon lewat plafon", "kasbon", 2_500_001, 1, senior, true, "Kasbon maksimal 50% THP (maks Rp 2.500.000)"},
		{"kasbon plafon tanpa angka", "kasbon", 2_500_001, 1, senior, false, "Kasbon maksimal 50% THP"},
		{"kasbon tenor 3", "kasbon", 1_000_000, 3, senior, true, "Cicilan kasbon hanya 1 atau 2 kali"},
		{"kasbon probation", "kasbon", 1_000_000, 2, LoanApplicant{"probation", "2024-01-15"}, true,
			"Karyawan masa percobaan (probation) tidak dapat mengajukan kasbon atau pinjaman"},
		{"kasbon harian", "kasbon", 1_000_000, 2, LoanApplicant{"daily_worker", "2024-01-15"}, true,
			"Kasbon dan pinjaman hanya untuk karyawan PKWT atau PKWTT"},
		{"kasbon masa kerja 2 bulan", "kasbon", 1_000_000, 2, LoanApplicant{"pkwt", "2026-09-01"}, true,
			"Kasbon hanya untuk karyawan dengan masa kerja minimal 3 bulan"},
		{"kasbon PKWT 3 bulan", "kasbon", 1_000_000, 2, LoanApplicant{"PKWT", "2026-08-01"}, true, ""},
		{"tanpa tanggal masuk", "kasbon", 1_000_000, 2, LoanApplicant{"contract", ""}, true,
			"Tanggal masuk karyawan belum diisi — hubungi HR"},
		{"pinjaman pas 200% tenor 6", "loan", 10_000_000, 6, senior, true, ""},
		{"pinjaman lewat plafon", "loan", 10_000_001, 6, senior, true, "Pinjaman Karyawan maksimal 200% THP (maks Rp 10.000.000)"},
		{"pinjaman plafon tanpa angka", "loan", 10_000_001, 6, senior, false, "Pinjaman Karyawan maksimal 200% THP"},
		{"pinjaman tenor 7", "loan", 1_000_000, 7, senior, true, "Cicilan Pinjaman Karyawan 1 sampai 6 kali"},
		{"pinjaman tenor 0", "loan", 1_000_000, 0, senior, true, "Cicilan Pinjaman Karyawan 1 sampai 6 kali"},
		{"pinjaman masa kerja 11 bulan", "loan", 1_000_000, 6, LoanApplicant{"contract", "2025-12-01"}, true,
			"Pinjaman Karyawan hanya untuk karyawan dengan masa kerja minimal 12 bulan"},
		{"pinjaman probation", "loan", 1_000_000, 2, LoanApplicant{"probation", "2020-01-01"}, true,
			"Karyawan masa percobaan (probation) tidak dapat mengajukan kasbon atau pinjaman"},
		{"jenis tak dikenal", "emergency", 1_000_000, 2, senior, true, "Jenis pinjaman tidak tersedia"},
	} {
		if got := check(c.loanType, c.principal, c.tenor, c.emp, c.showCap); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	if msg := ValidateLoanTnC("loan", 1_000_000, 2, senior, today, nil, 0, 1, true); msg != "Karyawan belum punya struktur gaji aktif — atur gaji dulu sebelum mengajukan pinjaman" {
		t.Errorf("tanpa struktur gaji: %q", msg)
	}
	if msg := ValidateLoanTnC("kasbon", 1_000_000, 2, senior, today, &thp, 1, 1, true); msg != "Karyawan sudah punya 1 pinjaman aktif (maks 1)" {
		t.Errorf("pinjaman aktif: %q", msg)
	}
}

func TestLoanPayrollPeriod(t *testing.T) {
	wib := time.FixedZone("WIB", 7*3600)
	for _, c := range []struct {
		at          time.Time
		month, year int
	}{
		{time.Date(2026, 11, 20, 23, 30, 0, 0, wib), 11, 2026},
		{time.Date(2026, 11, 21, 0, 5, 0, 0, wib), 12, 2026},
		{time.Date(2026, 12, 21, 8, 0, 0, 0, wib), 1, 2027},
		{time.Date(2026, 1, 1, 8, 0, 0, 0, wib), 1, 2026},
		{time.Date(2026, 11, 20, 16, 30, 0, 0, time.UTC), 11, 2026}, // 23.30 WIB
		{time.Date(2026, 11, 20, 17, 30, 0, 0, time.UTC), 12, 2026}, // 21 Nov 00.30 WIB
	} {
		if m, y := LoanPayrollPeriod(c.at); m != c.month || y != c.year {
			t.Errorf("LoanPayrollPeriod(%s) = %d/%d, want %d/%d", c.at, m, y, c.month, c.year)
		}
	}
}

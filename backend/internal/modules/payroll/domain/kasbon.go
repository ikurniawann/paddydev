package domain

import (
	"fmt"
	"slices"
	"strings"
)

// Kasbon rules (MOM 19-10-2026 KP-02, tabel payslip Paddy; EPIC-060).
const KasbonMaxThpPercent = 50

var (
	kasbonTenors          = []int{1, 2}
	kasbonBlockedStatuses = []string{"probation", "freelance", "intern", "internship"}
	// managerLevels: "Manager Departemen" and above approve kasbon step 1
	// (Assistant Manager excluded).
	managerLevels = []string{"manager", "senior manager", "general manager", "director", "c-level"}
)

func normLevel(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// KasbonBlockedStatus: Probation, Freelance and Intern cannot take a kasbon.
func KasbonBlockedStatus(status string) bool {
	return slices.Contains(kasbonBlockedStatuses, normLevel(status))
}

// IsManagerLevel: a position level of Manager or above.
func IsManagerLevel(level string) bool { return slices.Contains(managerLevels, normLevel(level)) }

// ValidateKasbon is "" when the kasbon fits: allowed status, tenor 1 or 2,
// active-loan count, and principal ≤ 50% of THP (4 components). showCap false
// leaves the cap amount out (it reveals salary; EPIC-057).
func ValidateKasbon(principal float64, tenor int, status string, thp *float64, activeCount, maxActive float64, showCap bool) string {
	if KasbonBlockedStatus(status) {
		return "Karyawan berstatus Probation/Freelance/Magang tidak dapat mengajukan kasbon"
	}
	if !slices.Contains(kasbonTenors, tenor) {
		return "Tenor kasbon hanya 1 atau 2 bulan"
	}
	if activeCount >= maxActive {
		return fmt.Sprintf("Karyawan sudah punya %v pinjaman aktif (maks %v)", activeCount, maxActive)
	}
	if thp == nil || *thp <= 0 {
		return "Karyawan belum punya struktur gaji aktif — atur gaji dulu sebelum mengajukan pinjaman"
	}
	limit := *thp * KasbonMaxThpPercent / 100
	if principal <= limit {
		return ""
	}
	if !showCap {
		return "Kasbon maksimal 50% THP"
	}
	return fmt.Sprintf("Kasbon maksimal 50%% THP (maks %s)", FormatRupiah(limit))
}

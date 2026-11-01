package domain

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// Kasbon and Pinjaman Karyawan rules (prosedur 01-11-2026, EPIC-076; MOM
// KP-02/KP-03).

// LoanRule is one requestable loan type's terms.
type LoanRule struct {
	Label            string
	MinServiceMonths int
	MaxTenor         int
	MaxThpPercent    float64
}

var loanRules = map[string]LoanRule{
	"kasbon": {Label: "Kasbon", MinServiceMonths: 3, MaxTenor: 2, MaxThpPercent: 50},
	"loan":   {Label: "Pinjaman Karyawan", MinServiceMonths: 12, MaxTenor: 6, MaxThpPercent: 200},
}

// LoanRuleFor is the rule of a requestable type; false for any other type.
func LoanRuleFor(loanType string) (LoanRule, bool) {
	r, ok := loanRules[loanType]
	return r, ok
}

// LoanPeriodCutoffDay: a request filed on day 1–20 (WIB) is deducted in that
// month's payroll, later ones in the next month's (periode 21–20, PY-02).
const LoanPeriodCutoffDay = 20

var (
	// loanEligibleStatuses: PKWT (contract, pkwt) and PKWTT (permanent) only.
	loanEligibleStatuses = []string{"contract", "pkwt", "permanent"}
	// managerLevels: "Manager Departemen" and above approve loans step 1
	// (Assistant Manager excluded). Mirror: payroll.LoanApproverSQL.
	managerLevels = []string{"manager", "senior manager", "general manager", "director", "c-level"}
	wib           = time.FixedZone("WIB", 7*3600)
)

func normLevel(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// LoanEligibleStatus: only PKWT and PKWTT employees may file a kasbon or loan.
func LoanEligibleStatus(status string) bool {
	return slices.Contains(loanEligibleStatuses, normLevel(status))
}

// IsManagerLevel: a position level of Manager or above.
func IsManagerLevel(level string) bool { return slices.Contains(managerLevels, normLevel(level)) }

// ServiceMonthsReached reports whether months full months have passed from
// joinDate (YYYY-MM-DD…) to today's WIB date; the anniversary day is clamped
// to the target month's last day. known is false without a parseable date.
func ServiceMonthsReached(joinDate string, today time.Time, months int) (ok, known bool) {
	if len(joinDate) < 10 {
		return false, false
	}
	join, err := time.Parse(time.DateOnly, joinDate[:10])
	if err != nil {
		return false, false
	}
	first := time.Date(join.Year(), join.Month()+time.Month(months), 1, 0, 0, 0, 0, time.UTC)
	lastDay := first.AddDate(0, 1, -1).Day()
	reached := first.AddDate(0, 0, min(join.Day(), lastDay)-1)
	y, m, d := today.In(wib).Date()
	return !time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Before(reached), true
}

// LoanApplicant is the employee data the terms read.
type LoanApplicant struct{ Status, JoinDate string }

// ValidateLoanTnC is "" when the request fits its type's terms, checked in
// order: status, service length, installments, active loans, salary
// structure, principal ≤ MaxThpPercent of THP (4 components). showCap false
// leaves the cap amount out (it reveals salary; EPIC-057).
func ValidateLoanTnC(loanType string, principal float64, tenor int, emp LoanApplicant, today time.Time,
	thp *float64, activeCount, maxActive float64, showCap bool) string {
	rule, ok := LoanRuleFor(loanType)
	if !ok {
		return "Jenis pinjaman tidak tersedia"
	}
	if !LoanEligibleStatus(emp.Status) {
		if normLevel(emp.Status) == "probation" {
			return "Karyawan masa percobaan (probation) tidak dapat mengajukan kasbon atau pinjaman"
		}
		return "Kasbon dan pinjaman hanya untuk karyawan PKWT atau PKWTT"
	}
	served, known := ServiceMonthsReached(emp.JoinDate, today, rule.MinServiceMonths)
	if !known {
		return "Tanggal masuk karyawan belum diisi — hubungi HR"
	}
	if !served {
		return fmt.Sprintf("%s hanya untuk karyawan dengan masa kerja minimal %d bulan", rule.Label, rule.MinServiceMonths)
	}
	if tenor < 1 || tenor > rule.MaxTenor {
		if rule.MaxTenor == 2 {
			return "Cicilan kasbon hanya 1 atau 2 kali"
		}
		return fmt.Sprintf("Cicilan %s 1 sampai %d kali", rule.Label, rule.MaxTenor)
	}
	if activeCount >= maxActive {
		return fmt.Sprintf("Karyawan sudah punya %v pinjaman aktif (maks %v)", activeCount, maxActive)
	}
	if thp == nil || *thp <= 0 {
		return "Karyawan belum punya struktur gaji aktif — atur gaji dulu sebelum mengajukan pinjaman"
	}
	limit := *thp * rule.MaxThpPercent / 100
	if principal <= limit {
		return ""
	}
	msg := fmt.Sprintf("%s maksimal %v%% THP", rule.Label, rule.MaxThpPercent)
	if !showCap {
		return msg
	}
	return fmt.Sprintf("%s (maks %s)", msg, FormatRupiah(limit))
}

// LoanPayrollPeriod is the payroll month a request filed at submittedAt is
// first deducted in: day ≤ LoanPeriodCutoffDay (WIB) → that month, else the
// next one.
func LoanPayrollPeriod(submittedAt time.Time) (month, year int) {
	y, m, d := submittedAt.In(wib).Date()
	if d > LoanPeriodCutoffDay {
		m++
	}
	p := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
	return int(p.Month()), p.Year()
}

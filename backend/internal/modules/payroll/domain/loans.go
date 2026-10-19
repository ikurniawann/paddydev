package domain

import (
	"fmt"
	"math"
	"time"
)

// LoanRow is LoanDeductionRow: the loan columns payroll reads. Amounts are
// numeric text (or nil) as node-postgres returns them.
type LoanRow struct {
	ID                    string
	MonthlyInstallment    any
	RemainingBalance      any
	FirstInstallmentMonth *int
	FirstInstallmentYear  *int
	Status                string
	IsActive              bool
	PrincipalAmount       any
	TenorMonths           *int
	LoanType              *string
}

// LoanInstallmentDetail is one row of payroll_details.loan_details.
type LoanInstallmentDetail struct {
	LoanID          string  `json:"loan_id"`
	LoanType        *string `json:"loan_type"`
	InstallmentNo   float64 `json:"installment_no"`
	TenorMonths     *int    `json:"tenor_months"`
	Amount          float64 `json:"amount"`
	RemainingBefore float64 `json:"remaining_before"`
	RemainingAfter  float64 `json:"remaining_after"`
}

// LoanAllocation is one loan's share of a deducted amount.
type LoanAllocation struct {
	LoanID       string
	Amount       float64
	NewRemaining float64
}

// IsLoanDue is isLoanDue.
func IsLoanDue(l LoanRow, month, year int) bool {
	if l.Status != "approved" || !l.IsActive {
		return false
	}
	if JSNumber(l.RemainingBalance) <= 0 || JSNumber(l.MonthlyInstallment) <= 0 {
		return false
	}
	m, y := l.FirstInstallmentMonth, l.FirstInstallmentYear
	if m == nil || y == nil || *m == 0 || *y == 0 {
		return false
	}
	return *y < year || (*y == year && *m <= month)
}

// LoanDeductionForPeriod is loanDeductionForPeriod.
func LoanDeductionForPeriod(loans []LoanRow, month, year int) float64 {
	total := 0.0
	for _, l := range loans {
		if IsLoanDue(l, month, year) {
			total += math.Min(Round(JSNumber(l.MonthlyInstallment)), Round(JSNumber(l.RemainingBalance)))
		}
	}
	return total
}

// AllocateLoanPayment is allocateLoanPayment: oldest first, at most one
// installment and the balance per loan, capped by amount.
func AllocateLoanPayment(loans []LoanRow, amount float64) []LoanAllocation {
	var out []LoanAllocation
	left := Round(amount)
	for _, l := range loans {
		if left <= 0 {
			break
		}
		balance := Round(JSNumber(l.RemainingBalance))
		if balance <= 0 {
			continue
		}
		pay := math.Min(math.Min(Round(JSNumber(l.MonthlyInstallment)), balance), left)
		if pay <= 0 {
			continue
		}
		out = append(out, LoanAllocation{LoanID: l.ID, Amount: pay, NewRemaining: balance - pay})
		left -= pay
	}
	return out
}

// ValidateLoanLimits is validateLoanLimits: "" when the loan fits.
func ValidateLoanLimits(installment float64, baseSalary *float64, maxPct, activeCount, maxActive float64) string {
	return ValidateLoanLimitsFor(installment, baseSalary, maxPct, activeCount, maxActive, true)
}

// ValidateLoanLimitsFor is ValidateLoanLimits; showCap false leaves the cap
// amount out of the over-limit message, since it reveals the base salary to
// accounts without Penggajian (EPIC-057).
func ValidateLoanLimitsFor(installment float64, baseSalary *float64, maxPct, activeCount, maxActive float64, showCap bool) string {
	if activeCount >= maxActive {
		return fmt.Sprintf("Karyawan sudah punya %v pinjaman aktif (maks %v)", activeCount, maxActive)
	}
	if baseSalary == nil || *baseSalary <= 0 {
		return "Karyawan belum punya struktur gaji aktif — atur gaji dulu sebelum mengajukan pinjaman"
	}
	maxInstallment := *baseSalary * maxPct / 100
	if installment > maxInstallment && !showCap {
		return fmt.Sprintf("Cicilan %s/bulan melebihi batas %v%% dari gaji pokok", FormatRupiah(installment), maxPct)
	}
	if installment > maxInstallment {
		return fmt.Sprintf("Cicilan %s/bulan melebihi batas %v%% dari gaji pokok (maks %s)",
			FormatRupiah(installment), maxPct, FormatRupiah(maxInstallment))
	}
	return ""
}

// LoanInstallmentDetails is loanInstallmentDetails: one row per due loan,
// the installment number derived from the principal already paid.
func LoanInstallmentDetails(loans []LoanRow, month, year int) []LoanInstallmentDetail {
	out := []LoanInstallmentDetail{}
	for _, l := range loans {
		if !IsLoanDue(l, month, year) {
			continue
		}
		installment := Round(JSNumber(l.MonthlyInstallment))
		before := Round(JSNumber(l.RemainingBalance))
		amount := math.Min(installment, before)
		principal := 0.0
		if l.PrincipalAmount != nil {
			principal = Round(JSNumber(l.PrincipalAmount))
		}
		no := 1.0
		if installment > 0 {
			no = math.Floor(math.Max(0, principal-before)/installment) + 1
		}
		out = append(out, LoanInstallmentDetail{
			LoanID: l.ID, LoanType: l.LoanType, InstallmentNo: no, TenorMonths: l.TenorMonths,
			Amount: amount, RemainingBefore: before, RemainingAfter: before - amount,
		})
	}
	return out
}

// LoanTerms is computeLoanTerms (flat interest).
func LoanTerms(principal, ratePercent float64, tenor int) (raw, monthly, total float64) {
	t := float64(tenor)
	if ratePercent != 0 {
		raw = principal * (1 + ratePercent/100*t) / t
	} else {
		raw = principal / t
	}
	monthly = Round(raw)
	return raw, monthly, monthly * t
}

// FirstInstallmentPeriod is firstInstallmentPeriod: the month after approval
// (server local time; the Go server runs in the same zone as Deps.Now).
func FirstInstallmentPeriod(approvedAt time.Time) (month, year int) {
	next := time.Date(approvedAt.Year(), approvedAt.Month()+1, 1, 0, 0, 0, 0, approvedAt.Location())
	return int(next.Month()), next.Year()
}

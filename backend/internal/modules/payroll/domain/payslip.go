package domain

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Payslip document helpers (lib/hris/payslip-pdf.ts, lib/payroll/share.ts,
// lib/payroll/loans.ts loanInstallmentLabel).

// ShareOfGross is formatShareOfGross: a deduction's share of gross pay as
// "12,5%", "<0,1%" for tiny shares, nil when gross is not positive or the
// amount is zero.
func ShareOfGross(amount, gross float64) *string {
	if !Finite(amount) || !Finite(gross) || gross <= 0 || amount == 0 {
		return nil
	}
	pct := math.Abs(amount) / gross * 100
	var s string
	if pct > 0 && pct < 0.05 {
		s = "<0,1%"
	} else {
		whole, frac, _ := strings.Cut(strconv.FormatFloat(math.Round(pct*10)/10, 'f', 1, 64), ".")
		n, _ := strconv.ParseInt(whole, 10, 64)
		s = groupThousands(n) + "," + frac + "%"
	}
	return &s
}

var loanTypeLabels = map[string]string{"kasbon": "Kasbon", "loan": "Pinjaman Karyawan", "emergency": "Pinjaman Darurat"}

// LoanInstallmentLabel is loanInstallmentLabel: "Cicilan Kasbon (2/3)",
// without the count when the tenor is unknown.
func LoanInstallmentLabel(d LoanInstallmentDetail) string {
	kind := "Pinjaman"
	if d.LoanType != nil && *d.LoanType != "" {
		kind = *d.LoanType
		if l, ok := loanTypeLabels[kind]; ok {
			kind = l
		}
	}
	if d.TenorMonths != nil && *d.TenorMonths != 0 {
		return "Cicilan " + kind + " (" + strconv.FormatFloat(d.InstallmentNo, 'f', -1, 64) + "/" + strconv.Itoa(*d.TenorMonths) + ")"
	}
	return "Cicilan " + kind
}

// MonthOrNumber is MONTHS[month - 1] ?? month.
func MonthOrNumber(month int) string {
	if month >= 1 && month <= 12 {
		return MonthNamesID[month-1]
	}
	return strconv.Itoa(month)
}

var notWordSpaceDash = regexp.MustCompile(`[^A-Za-z0-9_\s\x0b\x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}-]`)

func isJSSpace(r rune) bool { return unicode.IsSpace(r) || r == '\ufeff' }

// PayslipFileName is payslipFileName: "Slip-Gaji-Budi-Santoso-Juni-2026.pdf".
func PayslipFileName(employeeName string, month, year int) string {
	name := notWordSpaceDash.ReplaceAllString(norm.NFKD.String(employeeName), "")
	name = strings.Join(strings.FieldsFunc(strings.TrimFunc(name, isJSSpace), isJSSpace), "-")
	if name == "" {
		name = "Karyawan"
	}
	return "Slip-Gaji-" + name + "-" + MonthOrNumber(month) + "-" + strconv.Itoa(year) + ".pdf"
}

// PercentID formats a 0..1 ratio as an Indonesian percentage with one
// decimal ("62,5%").
func PercentID(ratio float64) string {
	whole, frac, _ := strings.Cut(strconv.FormatFloat(math.Round(ratio*1000)/10, 'f', 1, 64), ".")
	return whole + "," + frac + "%"
}

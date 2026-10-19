package domain

import (
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
)

func ptr[T any](v T) *T { return &v }

func baseInput(mod func(*Input)) Input {
	in := Input{
		EmployeeID: "emp-1", PeriodMonth: 6, PeriodYear: 2026,
		BaseSalary: 10_000_000, FixedAllowance: 2_000_000,
		WorkingDays: 22, PresentDays: 22, JoinDate: "2024-01-15",
		EmploymentStatus: "permanent", PTKPStatus: "TK/0", IsTaxable: true,
		BpjsTkEnrolled: true, BpjsKesEnrolled: true, TaperaEnrolled: true,
	}
	if mod != nil {
		mod(&in)
	}
	return in
}

func eq(t *testing.T, name string, got, want float64) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func TestPTKPAndBrackets(t *testing.T) {
	c := DefaultConfig()
	eq(t, "TK/0", PTKPAmount("TK/0", c), 54_000_000)
	eq(t, "K/3", PTKPAmount("K/3", c), 72_000_000)
	eq(t, "k/1", PTKPAmount("k/1", c), 63_000_000)
	eq(t, "unknown", PTKPAmount("X/9", c), 54_000_000)

	got := CumulativeLimitsToBrackets([]float64{60e6, 250e6, 500e6, 5e9}, []float64{0.05, 0.15, 0.25, 0.3, 0.35})
	want := []Bracket{{60e6, 0.05}, {190e6, 0.15}, {250e6, 0.25}, {4.5e9, 0.3}, {math.Inf(1), 0.35}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("brackets %v", got)
	}
}

func TestPPh21(t *testing.T) {
	c := DefaultConfig()
	a, m := PPh21(120_000_000, "TK/0", c)
	eq(t, "annual", a, 3_900_000)
	eq(t, "monthly", m, 325_000)
	a, _ = PPh21(114_000_000, "TK/0", c)
	eq(t, "first bracket", a, 3_000_000)
	a, m = PPh21(50_000_000, "TK/0", c)
	eq(t, "below ptkp", a+m, 0)
	a, _ = PPh21(54_000_500, "TK/0", c)
	eq(t, "pkp rounded down", a, 0)
	pkp := 6_054_000_000.0
	a, _ = PPh21(pkp+54_000_000, "TK/0", c)
	eq(t, "all brackets", a, Round(60e6*0.05+190e6*0.15+250e6*0.25+4.5e9*0.3+(pkp-5e9)*0.35))
}

func TestBPJS(t *testing.T) {
	c := DefaultConfig()
	b := CalculateBPJS(20_000_000, true, true, true, c)
	eq(t, "jht capped", b.Employee.TkJht, 208_280)
	eq(t, "kes capped", b.Employee.Kes, 120_000)
	b = CalculateBPJS(4_000_000, true, true, true, c)
	eq(t, "no jp below 5jt", b.Employee.TkJp, 0)
	eq(t, "jht", b.Employee.TkJht, 80_000)
	b = CalculateBPJS(10_000_000, true, true, false, c)
	eq(t, "tapera off", b.Employee.Tapera+b.Employer.Tapera, 0)
	b = CalculateBPJS(10_000_000, false, false, true, c)
	eq(t, "only tapera", b.Employee.Tapera, 250_000)
	eq(t, "only tapera jht", b.Employee.TkJht, 0)
	b = CalculateBPJS(10_000_000, false, false, false, c)
	eq(t, "nothing", b.Employee.Total+b.Employer.Total, 0)
}

func TestSmallRules(t *testing.T) {
	c := DefaultConfig()
	eq(t, "thr full", THR(10_000_000, "2024-03-01", 2026), 10_000_000)
	if v := THR(12_000_000, "2026-07-01", 2026); v <= 5_000_000 || v >= 7_000_000 {
		t.Errorf("thr prorated = %v", v)
	}
	eq(t, "overtime", Overtime(10, 50_000, 1.5), 750_000)
	eq(t, "unpaid", UnpaidLeave(11_000_000, 22, 2), 1_000_000)
	eq(t, "unpaid zero days", UnpaidLeave(11_000_000, 0, 2), 0)
	eq(t, "jabatan cap", JabatanExpense(144_000_000, c), 6_000_000)
	eq(t, "jabatan", JabatanExpense(60_000_000, c), 3_000_000)
}

func TestCalculateReference(t *testing.T) {
	c := DefaultConfig()
	r := Calculate(baseInput(nil), c)
	eq(t, "gross", r.GrossSalary, 12_000_000)
	eq(t, "jht", r.BpjsTkJhtDeduction, 208_280)
	eq(t, "jp", r.BpjsTkJpDeduction, 104_140)
	eq(t, "kes", r.BpjsKesDeduction, 120_000)
	eq(t, "tapera", r.TaperaDeduction, 260_350)
	eq(t, "taxable", r.TaxableIncome, 129_686_760)
	eq(t, "pph annual", r.Pph21Annual, 5_352_900)
	eq(t, "pph", r.Pph21Deduction, 446_075)
	eq(t, "total", r.TotalDeductions, 1_138_845)
	eq(t, "net", r.NetSalary, 10_861_155)

	noTapera := Calculate(baseInput(func(i *Input) { i.TaperaEnrolled = false }), c)
	eq(t, "tapera not double counted", noTapera.TaxableIncome-r.TaxableIncome, r.TaperaDeduction*12)
	eq(t, "not taxable", Calculate(baseInput(func(i *Input) { i.IsTaxable = false }), c).Pph21Deduction, 0)

	eq(t, "thr permanent", Calculate(baseInput(func(i *Input) { i.IncludeThr = true }), c).Thr, 10_000_000)
	eq(t, "thr probation", Calculate(baseInput(func(i *Input) { i.IncludeThr = true; i.EmploymentStatus = "probation" }), c).Thr, 0)
	eq(t, "thr flag off", r.Thr, 0)
	eq(t, "unpaid leave", Calculate(baseInput(func(i *Input) { i.UnpaidLeaveDays = 2; i.WorkingDays = 20 }), c).UnpaidLeaveDeduction, 1_000_000)

	c2 := DefaultConfig()
	c2.Rates.BpjsTkJht = 0.04
	eq(t, "config override", Calculate(baseInput(nil), c2).BpjsTkJhtDeduction, 416_560)

	ot := Calculate(baseInput(func(i *Input) { i.OvertimeHours = 10 }), c)
	eq(t, "overtime pay", ot.OvertimePay, 867_052)
	eq(t, "overtime hours", ot.OvertimeHours, 10)

	late := func(i *Input) { i.LateDays = 3; i.LateMinutes = 45 }
	eq(t, "late off", Calculate(baseInput(late), c).LateDeduction, 0)
	pm := DefaultConfig()
	pm.LateMode, pm.LateAmount = "per_minute", 1000
	eq(t, "late per minute", Calculate(baseInput(late), pm).LateDeduction, 45_000)
	eq(t, "late reduces net", Calculate(baseInput(nil), pm).NetSalary-Calculate(baseInput(late), pm).NetSalary, 45_000)
	flat := DefaultConfig()
	flat.LateMode, flat.LateAmount = "flat", 25_000
	eq(t, "late flat", Calculate(baseInput(late), flat).LateDeduction, 75_000)
}

func TestCalculateHolidayOvertime(t *testing.T) {
	c := DefaultConfig()
	hourly := 10_000_000.0 / 173
	eq(t, "regular", Calculate(baseInput(func(i *Input) { i.OvertimeHours = 4 }), c).OvertimePay, Round(4*hourly*1.5))
	eq(t, "holiday", Calculate(baseInput(func(i *Input) { i.OvertimeHolidayHrs = 4 }), c).OvertimePay, Round(4*hourly*2))
	eq(t, "mixed", Calculate(baseInput(func(i *Input) { i.OvertimeHours = 3; i.OvertimeHolidayHrs = 2 }), c).OvertimePay,
		Round(3*hourly*1.5)+Round(2*hourly*2))
	eq(t, "rate override", Calculate(baseInput(func(i *Input) { i.OvertimeHolidayHrs = 2; i.OvertimeHolidayRate = ptr(3.0) }), c).OvertimePay,
		Round(2*hourly*3))
}

func TestCalculateContracts(t *testing.T) {
	c := DefaultConfig()
	half := Calculate(baseInput(func(i *Input) {
		i.ProrateFactor, i.ContractType, i.TransportAllowance = ptr(0.5), ptr("pkwt"), 500_000
	}), c)
	eq(t, "base", half.BaseSalary, 5_000_000)
	eq(t, "fixed", half.FixedAllowance, 1_000_000)
	eq(t, "transport", half.TransportAllowance, 250_000)
	eq(t, "jht on paid", half.BpjsTkJhtDeduction, 120_000)

	halfOT := Calculate(baseInput(func(i *Input) { i.ProrateFactor, i.ContractType, i.OvertimeHours = ptr(0.5), ptr("pkwt"), 10 }), c)
	fullOT := Calculate(baseInput(func(i *Input) { i.OvertimeHours = 10 }), c)
	eq(t, "overtime on full base", halfOT.OvertimePay, fullOT.OvertimePay)
	eq(t, "thr on full base", Calculate(baseInput(func(i *Input) {
		i.ProrateFactor, i.ContractType, i.IncludeThr = ptr(0.5), ptr("pkwtt"), true
	}), c).Thr, 10_000_000)
	eq(t, "pkwt thr", Calculate(baseInput(func(i *Input) {
		i.IncludeThr, i.ContractType, i.EmploymentStatus = true, ptr("pkwt"), "contract"
	}), c).Thr, 10_000_000)

	loan := Calculate(baseInput(func(i *Input) { i.LoanDeduction = 500_000 }), c)
	base := Calculate(baseInput(nil), c)
	eq(t, "loan", loan.LoanDeduction, 500_000)
	eq(t, "loan net", base.NetSalary-loan.NetSalary, 500_000)
	eq(t, "loan not taxed", loan.Pph21Deduction, base.Pph21Deduction)
	if one := Calculate(baseInput(func(i *Input) { i.ProrateFactor = ptr(1.0) }), c); one != base {
		t.Fatal("prorate 1 is a no-op")
	}
}

func TestSumRunTotals(t *testing.T) {
	if SumRunTotals(nil) != (RunTotals{}) {
		t.Fatal("empty run")
	}
	c := DefaultConfig()
	a := Calculate(baseInput(nil), c)
	b := Calculate(baseInput(func(i *Input) { i.BaseSalary = 6_500_000 }), c)
	got := SumRunTotals([]Result{a, b})
	eq(t, "gross", got.TotalGross, a.GrossSalary+b.GrossSalary)
	eq(t, "employee", got.TotalBjtkEmployee, a.BpjsTkJhtDeduction+a.BpjsTkJpDeduction+a.BpjsKesDeduction+a.TaperaDeduction+
		b.BpjsTkJhtDeduction+b.BpjsTkJpDeduction+b.BpjsKesDeduction+b.TaperaDeduction)
	eq(t, "employer", got.TotalBjtkEmployer, a.TotalEmployerContribution+b.TotalEmployerContribution)
}

func TestBuildConfig(t *testing.T) {
	if !reflect.DeepEqual(BuildConfig(nil, nil), DefaultConfig()) {
		t.Fatal("no rows = defaults")
	}
	c := BuildConfig(Row{"bpjs_tk_jht_employee": "4.00", "late_deduction_mode": "flat", "late_deduction_amount": nil,
		"pph21_bracket_1": "50000000.00"}, nil)
	eq(t, "percent to fraction", c.Rates.BpjsTkJht, 0.04)
	eq(t, "null is zero", c.LateAmount, 0)
	if c.LateMode != "flat" || c.Brackets[0].Width != 50_000_000 || c.Brackets[1].Width != 200_000_000 {
		t.Fatalf("settings brackets %+v", c)
	}
	tax := BuildConfig(nil, Row{"ptkp_tk_0": "60000000.00", "bracket_1_rate": "10.00", "jabatan_expense_max": "5000000"})
	eq(t, "tax ptkp", tax.PTKP["TK/0"], 60_000_000)
	eq(t, "tax rate", tax.Brackets[0].Rate, 0.1)
	eq(t, "jabatan max", tax.JabatanMaxPerYear, 5_000_000)
}

func TestPeriod(t *testing.T) {
	if s, e := PeriodBounds(2, 2028); s != "2028-02-01" || e != "2028-02-29" {
		t.Fatal(s, e)
	}
	if s, e := PeriodBounds(12, 2026); s != "2026-12-01" || e != "2026-12-31" {
		t.Fatal(s, e)
	}
	if got := EachDate("2026-06-28", "2026-07-02"); strings.Join(got, ",") != "2026-06-28,2026-06-29,2026-06-30,2026-07-01,2026-07-02" {
		t.Fatal(got)
	}
	pattern := func(to *string) []ShiftRow {
		var rows []ShiftRow
		for dow := 1; dow <= 7; dow++ {
			var id *string
			if dow <= 5 {
				id = ptr("shift-1")
			}
			rows = append(rows, ShiftRow{DayOfWeek: dow, ShiftID: id, EffectiveFrom: "2026-01-01", EffectiveTo: to})
		}
		return rows
	}
	if n, ok := ScheduledDays(pattern(nil), "2026-06-01", "2026-06-30"); n != 22 || !ok {
		t.Fatal(n, ok)
	}
	if n, ok := ScheduledDays(nil, "2026-06-01", "2026-06-30"); n != 0 || ok {
		t.Fatal("no pattern")
	}
	if n, _ := ScheduledDays(pattern(ptr("2026-06-12")), "2026-06-01", "2026-06-30"); n != 10 {
		t.Fatal(n)
	}
	for _, tc := range []struct {
		l    LeaveRange
		want int
	}{{LeaveRange{"2026-05-28", "2026-06-03"}, 3}, {LeaveRange{"2026-06-10", "2026-06-12"}, 3}, {LeaveRange{"2026-07-01", "2026-07-05"}, 0}} {
		if got := ClampedLeaveDays(tc.l, "2026-06-01", "2026-06-30"); got != tc.want {
			t.Errorf("leave %v = %d", tc.l, got)
		}
	}
	att := func(date string, mod func(*AttendanceRow)) AttendanceRow {
		a := AttendanceRow{Date: date, Status: "present", HasClockOut: true, IsLate: ptr(false), LateMinutes: "0", OvertimeHours: "0"}
		if mod != nil {
			mod(&a)
		}
		return a
	}
	none := map[string]bool{}
	if s := SplitOvertime([]OvertimeRequest{{Date: "2026-06-10", Hours: 3}}, []AttendanceRow{att("2026-06-10", func(a *AttendanceRow) { a.OvertimeHours = "1.5" })}, none); s.Total != 1.5 {
		t.Fatal("capped", s)
	}
	if s := SplitOvertime([]OvertimeRequest{{Date: "2026-06-10", Hours: 2}}, []AttendanceRow{att("2026-06-10", func(a *AttendanceRow) { a.HasClockOut = false })}, none); s.Total != 0 {
		t.Fatal("no clock out", s)
	}
	s := SplitOvertime([]OvertimeRequest{{Date: "2026-06-10", Hours: 2}, {Date: "2026-06-11", Hours: 1.5}, {Date: "2026-06-12", Hours: 4}}, []AttendanceRow{att("2026-06-10", nil), att("2026-06-11", nil)}, none)
	if s.Total != 3.5 {
		t.Fatal("sum", s)
	}
	h := SplitOvertime([]OvertimeRequest{{Date: "2026-08-17", Hours: 3}, {Date: "2026-08-18", Hours: 2}}, []AttendanceRow{att("2026-08-17", nil), att("2026-08-18", nil)}, map[string]bool{"2026-08-17": true})
	if h != (OvertimeSplit{2, 3, 5}) {
		t.Fatal("holiday split", h)
	}
	days, mins := LateStats([]AttendanceRow{
		att("2026-06-01", func(a *AttendanceRow) { a.IsLate, a.LateMinutes, a.Status = ptr(true), "12", "late" }),
		att("2026-06-02", func(a *AttendanceRow) { a.IsLate, a.LateMinutes, a.Status = ptr(true), "30", "late" }),
		att("2026-06-03", nil),
		att("2026-06-04", func(a *AttendanceRow) { a.Status, a.IsLate, a.LateMinutes = "late", nil, nil }),
	})
	if days != 3 || mins != 42 {
		t.Fatal("late stats", days, mins)
	}
	if c := Coverage("2026-06-15", ptr("2026-08-01"), "2026-06-01", "2026-06-30"); *c != (DateRange{"2026-06-15", "2026-06-30"}) {
		t.Fatal(c)
	}
	if c := Coverage("2026-07-01", nil, "2026-06-01", "2026-06-30"); c != nil {
		t.Fatal("miss")
	}
	if m := MergeRanges([]DateRange{{"2026-06-16", "2026-06-30"}, {"2026-06-01", "2026-06-15"}}); len(m) != 1 || m[0] != (DateRange{"2026-06-01", "2026-06-30"}) {
		t.Fatal(m)
	}
	if m := MergeRanges([]DateRange{{"2026-06-01", "2026-06-10"}, {"2026-06-20", "2026-06-30"}}); len(m) != 2 {
		t.Fatal(m)
	}
	if AddDaysISO("2026-06-30", 1) != "2026-07-01" || AddDaysISO("2026-07-01", -1) != "2026-06-30" {
		t.Fatal("add days")
	}
	if DateText(ptr("bukan tanggal")) != nil || *DateText(ptr("2026-06-16T00:00:00.000Z")) != "2026-06-16" {
		t.Fatal("date text")
	}
	if PeriodLabelID(ptr(12), 2026) != "Desember 2026" || PeriodLabelID(nil, 2026) != "Januari 2026" {
		t.Fatal("label")
	}
}

func TestBuildInputCoverage(t *testing.T) {
	emp := Employee{ID: "e", JoinDate: "2024-01-01", EmploymentStatus: "contract"}
	sal := Salary{BaseSalary: "10000000.00", FixedAllowance: nil}
	var schedule []ShiftRow
	for dow := 1; dow <= 5; dow++ {
		schedule = append(schedule, ShiftRow{DayOfWeek: dow, ShiftID: ptr("s"), EffectiveFrom: "2026-01-01"})
	}
	// PKWT ends 15 June and is renewed from 16 June: one full coverage.
	rec := WorkforceRecords{Schedule: schedule, Contracts: []Contract{
		{ContractType: ptr("pkwt"), StartDate: ptr("2026-06-16"), EndDate: ptr("2026-12-31")},
		{ContractType: ptr("pkwtt"), StartDate: ptr("2026-01-01"), EndDate: ptr("2026-06-15")},
	}}
	in := BuildInput(emp, sal, rec, nil, nil, 6, 2026, false)
	if *in.ProrateFactor != 1 || in.WorkingDays != 22 || *in.ContractType != "pkwt" || in.PTKPStatus != "TK/0" || !in.IsTaxable {
		t.Fatalf("renewal %+v", in)
	}
	// Joined mid-month: prorated to the covered scheduled days.
	rec.Contracts = []Contract{{ContractType: ptr("pkwt"), StartDate: ptr("2026-06-15")}}
	in = BuildInput(emp, sal, rec, nil, nil, 6, 2026, false)
	if in.WorkingDays != 12 || *in.ProrateFactor != 12.0/22 {
		t.Fatalf("partial %v %v", in.WorkingDays, *in.ProrateFactor)
	}
	// No pattern, no attendance: the fallback 20 days.
	in = BuildInput(emp, sal, WorkforceRecords{Leaves: []Leave{{ptr("2026-05-30"), ptr("2026-06-02"), "unpaid"}}}, []LoanRow{{
		ID: "l", MonthlyInstallment: "500000.00", RemainingBalance: "1000000.00", FirstInstallmentMonth: ptr(6),
		FirstInstallmentYear: ptr(2026), Status: "approved", IsActive: true, PrincipalAmount: "1500000.00", TenorMonths: ptr(3),
	}}, nil, 6, 2026, false)
	if in.WorkingDays != 20 || in.UnpaidLeaveDays != 2 || in.LoanDeduction != 500_000 || in.LoanBreakdown[0].InstallmentNo != 2 {
		t.Fatalf("fallback %+v", in)
	}
}

func TestLoans(t *testing.T) {
	loan := func(mod func(*LoanRow)) LoanRow {
		l := LoanRow{ID: "loan-1", MonthlyInstallment: "500000", RemainingBalance: "3000000",
			FirstInstallmentMonth: ptr(6), FirstInstallmentYear: ptr(2026), Status: "approved", IsActive: true}
		if mod != nil {
			mod(&l)
		}
		return l
	}
	if !IsLoanDue(loan(nil), 6, 2026) || !IsLoanDue(loan(nil), 1, 2027) || IsLoanDue(loan(nil), 5, 2026) {
		t.Fatal("due window")
	}
	for _, mod := range []func(*LoanRow){
		func(l *LoanRow) { l.Status = "pending" }, func(l *LoanRow) { l.IsActive = false },
		func(l *LoanRow) { l.RemainingBalance = "0" }, func(l *LoanRow) { l.FirstInstallmentMonth = nil },
	} {
		if IsLoanDue(loan(mod), 6, 2026) {
			t.Fatal("not due")
		}
	}
	eq(t, "sum", LoanDeductionForPeriod([]LoanRow{loan(nil), loan(func(l *LoanRow) { l.MonthlyInstallment = "250000" })}, 6, 2026), 750_000)
	eq(t, "cap", LoanDeductionForPeriod([]LoanRow{loan(func(l *LoanRow) { l.RemainingBalance = "200000" })}, 6, 2026), 200_000)

	alloc := AllocateLoanPayment([]LoanRow{
		loan(func(l *LoanRow) { l.ID, l.RemainingBalance = "old", "1000000" }),
		loan(func(l *LoanRow) { l.ID, l.MonthlyInstallment, l.RemainingBalance = "new", "300000", "900000" }),
	}, 800_000)
	if !reflect.DeepEqual(alloc, []LoanAllocation{{"old", 500_000, 500_000}, {"new", 300_000, 600_000}}) {
		t.Fatal(alloc)
	}
	alloc = AllocateLoanPayment([]LoanRow{loan(func(l *LoanRow) { l.RemainingBalance = "400000" })}, 400_000)
	if alloc[0].NewRemaining != 0 {
		t.Fatal(alloc)
	}
	alloc = AllocateLoanPayment([]LoanRow{loan(nil), loan(func(l *LoanRow) { l.ID = "b" })}, 600_000)
	if alloc[1].Amount != 100_000 {
		t.Fatal(alloc)
	}

	if msg := ValidateLoanLimits(2_000_000, ptr(10_000_000.0), 30, 0, 1); msg != "" {
		t.Fatal(msg)
	}
	if msg := ValidateLoanLimits(3_500_000, ptr(10_000_000.0), 30, 0, 1); msg != "Cicilan Rp 3.500.000/bulan melebihi batas 30% dari gaji pokok (maks Rp 3.000.000)" {
		t.Fatal(msg)
	}
	if msg := ValidateLoanLimits(1, ptr(10_000_000.0), 30, 1, 1); msg != "Karyawan sudah punya 1 pinjaman aktif (maks 1)" {
		t.Fatal(msg)
	}
	if msg := ValidateLoanLimits(1, nil, 30, 0, 1); !strings.Contains(msg, "struktur gaji") {
		t.Fatal(msg)
	}

	raw, monthly, total := LoanTerms(1_000_000, 2, 3)
	if math.Abs(raw-353_333.333) > 0.01 || monthly != 353_333 || total != 1_059_999 {
		t.Fatal(raw, monthly, total)
	}
	if _, m, tot := LoanTerms(1_000_000, 0, 3); m != 333_333 || tot != 999_999 {
		t.Fatal(m, tot)
	}
	if m, y := FirstInstallmentPeriod(time.Date(2026, 12, 31, 10, 0, 0, 0, time.UTC)); m != 1 || y != 2027 {
		t.Fatal(m, y)
	}

	base := LoanRow{ID: "loan-1", LoanType: ptr("kasbon"), PrincipalAmount: "3000000", MonthlyInstallment: "1000000",
		TenorMonths: ptr(3), FirstInstallmentMonth: ptr(1), FirstInstallmentYear: ptr(2026), Status: "approved", IsActive: true}
	with := func(rem string) []LoanRow { l := base; l.RemainingBalance = rem; return []LoanRow{l} }
	if d := LoanInstallmentDetails(with("3000000"), 1, 2026); d[0].InstallmentNo != 1 {
		t.Fatal(d)
	}
	if d := LoanInstallmentDetails(with("1000000"), 6, 2026); d[0].InstallmentNo != 3 {
		t.Fatal(d)
	}
	if d := LoanInstallmentDetails(with("400000"), 3, 2026); d[0].Amount != 400_000 || d[0].RemainingAfter != 0 {
		t.Fatal(d)
	}
}

func TestAccessRules(t *testing.T) {
	hr, staff := "hrd", "employee"
	if got := ResolvePayslipScope(staff, false, ptr("emp-1"), ptr("emp-2")); got != (PayslipScope{MeView: true, EmployeeID: "emp-1"}) {
		t.Fatal(got)
	}
	if got := ResolvePayslipScope(hr, true, ptr("emp-hr"), ptr("me")); got != (PayslipScope{MeView: true, EmployeeID: "emp-hr"}) {
		t.Fatal(got)
	}
	if got := ResolvePayslipScope(hr, true, ptr("emp-hr"), nil); got != (PayslipScope{}) {
		t.Fatal(got)
	}
	if got := ResolvePayslipScope(hr, true, nil, ptr("emp-9")); got != (PayslipScope{EmployeeID: "emp-9"}) {
		t.Fatal(got)
	}
	if !ResolvePayslipScope(staff, false, nil, nil).Skip {
		t.Fatal("no employee record")
	}
	if !CanTransitionRun("draft", "processing") || CanTransitionRun("draft", "paid") || CanTransitionRun("paid", "draft") {
		t.Fatal("transitions")
	}
	if CanDeleteRun("paid") || !CanCalculateRun("draft") || CanCalculateRun("processing") {
		t.Fatal("run rules")
	}
	if CanWritePayrollSettings("admin") || !CanWritePayrollSettings("hrd") {
		t.Fatal("settings writers")
	}
}

func TestJSHelpers(t *testing.T) {
	if FormatRupiah(1_500_000) != "Rp 1.500.000" || FormatRupiah(-999.5) != "-Rp 999" || FormatRupiah(0) != "Rp 0" {
		t.Fatal(FormatRupiah(1_500_000), FormatRupiah(-999.5))
	}
	if EncodeURIComponent("Halo Budi, slip → Gaji!") != "Halo%20Budi%2C%20slip%20%E2%86%92%20Gaji!" {
		t.Fatal(EncodeURIComponent("Halo Budi, slip → Gaji!"))
	}
	if l := BuildWaLink(ptr("0812-3456"), "hi"); *l != "https://wa.me/628123456?text=hi" {
		t.Fatal(*l)
	}
	if BuildWaLink(ptr("-"), "x") != nil || BuildWaLink(nil, "x") != nil {
		t.Fatal("empty phone")
	}
}

func TestJSNumber(t *testing.T) {
	for in, want := range map[string]float64{"": 0, " 12 ": 12, "1e3": 1000, ".5": 0.5, "0x10": 16, "Infinity": math.Inf(1)} {
		if got := JSNumber(in); got != want {
			t.Errorf("Number(%q) = %v", in, got)
		}
	}
	for _, in := range []any{"abc", "1_000", "inf", "0x", []any{true}, []any{1.0, 2.0}, map[string]any{}} {
		if !math.IsNaN(JSNumber(in)) {
			t.Errorf("Number(%v) is not NaN", in)
		}
	}
	if JSNumber(nil) != 0 || JSNumber(true) != 1 || JSNumber([]any{"7"}) != 7 || JSNumber([]any{}) != 0 || JSNumber([]any{nil}) != 0 {
		t.Fatal("coercions")
	}
}

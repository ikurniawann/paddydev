package domain

import (
	"math"
	"strings"
	"time"
)

// jpMinSalary is the Jaminan Pensiun participation threshold.
const jpMinSalary = 5_000_000

// Input is PayrollInput in lib/payroll/calculator.ts. Optional TS fields
// are pointers only where `undefined` falls back to a config value.
type Input struct {
	EmployeeID  string
	PeriodMonth int
	PeriodYear  int

	BaseSalary          float64
	FixedAllowance      float64
	VariableAllowance   float64
	TransportAllowance  float64
	MealAllowance       float64
	HousingAllowance    float64
	OvertimeHours       float64
	OvertimeRate        *float64 // nil: config.OvertimeMultiplier
	OvertimeHolidayHrs  float64
	OvertimeHolidayRate *float64 // nil: config.OvertimeHolidayMult
	Bonus               float64

	WorkingDays     float64
	PresentDays     float64
	LateDays        float64
	LateMinutes     float64
	UnpaidLeaveDays float64

	LoanDeduction float64
	LoanBreakdown []LoanInstallmentDetail

	JoinDate         string // YYYY-MM-DD
	EmploymentStatus string
	ContractType     *string // pkwt | pkwtt, nil without a contract record
	ProrateFactor    *float64
	PTKPStatus       string
	IsTaxable        bool

	BpjsTkEnrolled  bool
	BpjsKesEnrolled bool
	TaperaEnrolled  bool

	IncludeThr bool

	// EPIC-055 (rule Odoo). Zero values keep the pre-EPIC behaviour.
	PayBasis          string  // monthly (default) | monthly_prorated | daily
	DivisorDays       float64 // pembagi hari struktur; 0 disables per-day rules
	AttendanceDays    float64
	AllowanceDays     float64
	TransportPerDay   bool
	MealPerDay        bool
	PositionPerDay    bool
	PositionAllowance float64
	HealthAllowance   float64
	HealthEligible    bool
	OvertimeScheme    string    // standard (default) | production | freelance | daily_worker
	LateDayMinutes    []float64 // late minutes of each late day (tier mode)
	PersonalLeaveDays float64
	SickLeaveDays     float64
	// EPIC-059: hari dibayar (hadir + cuti dibayar, hari kerja) untuk gaji
	// pokok monthly_prorated & jabatan per hari; hari cuti melahirkan.
	PaidDays           float64
	MaternityLeaveDays float64
	ManualInputs       []ManualInput
	Ytd                YearToDate
}

// ManualInput is one manual slip component (hris.payroll_inputs).
type ManualInput struct {
	Code   string  `json:"code"`
	Amount float64 `json:"amount"`
	Note   string  `json:"note,omitempty"`
}

// Manual input codes by effect on the slip.
var (
	ManualBonusCodes       = []string{"BONUS"}
	ManualEarningCodes     = []string{"INCENTIVE", "NATURA", "TRANSPORT_LOCATION", "OTHER_EARNING"}
	ManualNontaxCodes      = []string{"OTHER_EARNING_NONTAX"}
	ManualDeductionCodes   = []string{"PENALTY", "ABSENCE", "UNION", "OTHER_DEDUCTION"}
	ManualLateOverrideCode = "LATE_OVERRIDE"
)

// ManualInputCodes are every accepted payroll_inputs code.
func ManualInputCodes() []string {
	out := append([]string{}, ManualBonusCodes...)
	out = append(out, ManualEarningCodes...)
	out = append(out, ManualNontaxCodes...)
	out = append(out, ManualDeductionCodes...)
	return append(out, ManualLateOverrideCode)
}

func manualSum(inputs []ManualInput, codes []string) float64 {
	total := 0.0
	for _, m := range inputs {
		for _, c := range codes {
			if m.Code == c {
				total += m.Amount
			}
		}
	}
	return total
}

func manualFind(inputs []ManualInput, code string) (float64, bool) {
	for _, m := range inputs {
		if m.Code == code {
			return m.Amount, true
		}
	}
	return 0, false
}

// ClearLate drops the late deduction inputs (multi-entity policy, a late
// deduction already on another slip of the period).
func (in *Input) ClearLate() {
	in.LateDays, in.LateMinutes, in.LateDayMinutes = 0, 0, nil
}

// Result is PayrollResult.
type Result struct {
	BaseSalary, FixedAllowance, VariableAllowance, TransportAllowance float64
	MealAllowance, HousingAllowance, OvertimePay, Thr, Bonus          float64
	OtherEarning, GrossSalary                                         float64

	BpjsTkJhtDeduction, BpjsTkJpDeduction, BpjsKesDeduction, TaperaDeduction float64
	Pph21Deduction, UnpaidLeaveDeduction, LateDeduction, LoanDeduction       float64
	OtherDeduction, TotalDeductions                                          float64

	OvertimeHours float64
	NetSalary     float64

	BpjsTkJhtEmployer, BpjsTkJpEmployer, BpjsTkJkkEmployer, BpjsTkJkmEmployer float64
	BpjsKesEmployer, TaperaEmployer, TotalEmployerContribution                float64

	TaxableIncome, PTKPAmount, Pph21Annual, Pph21Monthly float64

	// EPIC-055
	PositionAllowance, HealthAllowance, OtherEarningNontax float64
	PersonalLeaveDeduction, SickLeaveDeduction             float64
	Pph21Method, TERCategory                               string
	Pph21Dpp, TERRate                                      float64
}

// PTKPAmount is getPTKPAmount: unknown statuses fall back to TK/0.
func PTKPAmount(status string, c Config) float64 {
	if v, ok := c.PTKP[strings.ToUpper(status)]; ok {
		return v
	}
	return c.PTKP["TK/0"]
}

// PPh21 is calculatePPh21ETR: progressive tax on annual taxable income
// after PTKP, PKP rounded down to whole thousands.
func PPh21(annualTaxable float64, status string, c Config) (annual, monthly float64) {
	pkp := math.Floor(math.Max(0, annualTaxable-PTKPAmount(status, c))/1000) * 1000
	tax, remaining := 0.0, pkp
	for _, b := range c.Brackets {
		if remaining <= 0 {
			break
		}
		tax += math.Min(remaining, b.Width) * b.Rate
		remaining -= b.Width
	}
	return Round(tax), Round(tax / 12)
}

// BPJS is calculateBPJS, rounded per component.
type BPJS struct {
	Employee struct{ TkJht, TkJp, Kes, Tapera, Total float64 }
	Employer struct{ TkJht, TkJp, TkJkk, TkJkm, Kes, Tapera, Total float64 }
}

// CalculateBPJS computes employee deductions and employer contributions on
// the monthly basis (BPJS TK and Kes capped separately).
func CalculateBPJS(monthly float64, tk, kes, tapera bool, c Config) BPJS {
	r := c.Rates
	tkBase := math.Min(monthly, c.CapBpjsTk)
	kesBase := math.Min(monthly, c.CapBpjsKes)
	on := func(enrolled bool, v float64) float64 {
		if enrolled {
			return v
		}
		return 0
	}
	jht := on(tk, tkBase*r.BpjsTkJht)
	jp := on(tk && monthly >= jpMinSalary, tkBase*r.BpjsTkJp)
	kesE := on(kes, kesBase*r.BpjsKes)
	tap := on(tapera, tkBase*r.Tapera)

	jhtR := on(tk, tkBase*r.BpjsTkJhtEmployer)
	jpR := on(tk, tkBase*r.BpjsTkJpEmployer)
	jkk := on(tk, tkBase*r.BpjsTkJkkEmployer)
	jkm := on(tk, tkBase*r.BpjsTkJkmEmployer)
	kesR := on(kes, kesBase*r.BpjsKesEmployer)
	tapR := on(tapera, tkBase*r.TaperaEmployer)

	var b BPJS
	b.Employee.TkJht, b.Employee.TkJp, b.Employee.Kes, b.Employee.Tapera = Round(jht), Round(jp), Round(kesE), Round(tap)
	b.Employee.Total = Round(jht + jp + kesE + tap)
	b.Employer.TkJht, b.Employer.TkJp, b.Employer.TkJkk, b.Employer.TkJkm = Round(jhtR), Round(jpR), Round(jkk), Round(jkm)
	b.Employer.Kes, b.Employer.Tapera = Round(kesR), Round(tapR)
	b.Employer.Total = Round(jhtR + jpR + jkk + jkm + kesR + tapR)
	return b
}

// THR is calculateTHR: full base salary when joined before the period year,
// otherwise prorated by 30-day months worked until 31 December.
func THR(base float64, joinDate string, periodYear int) float64 {
	join, err := time.Parse("2006-01-02", joinDate)
	if err != nil {
		// new Date("garbage") is Invalid Date: every comparison is false and
		// the month count is NaN, so Math.round(NaN) propagates.
		return math.NaN()
	}
	if join.Before(time.Date(periodYear, 1, 1, 0, 0, 0, 0, time.UTC)) {
		return base
	}
	dec31 := time.Date(periodYear, 12, 31, 0, 0, 0, 0, time.UTC)
	months := math.Floor(float64(dec31.Sub(join).Milliseconds()) / (1000 * 60 * 60 * 24 * 30))
	return Round(base * math.Min(12, math.Max(1, months)) / 12)
}

// Overtime is calculateOvertime.
func Overtime(hours, hourlyRate, multiplier float64) float64 {
	return Round(hours * hourlyRate * multiplier)
}

// UnpaidLeave is calculateUnpaidLeave.
func UnpaidLeave(base, workingDays, days float64) float64 {
	if workingDays <= 0 {
		return 0
	}
	return Round(base / workingDays * days)
}

// LateTierDays is the tier matching a late-day count: days to deduct and
// their basis ("thp" | "base"); zero days below the first tier.
func LateTierDays(count int, tiers []LateTier) (float64, string) {
	for _, t := range tiers {
		if count >= t.Min && (t.Max == nil || count <= *t.Max) {
			return t.Days, t.Basis
		}
	}
	return 0, ""
}

// countLateDays counts the late days of at least minMinutes minutes.
func countLateDays(minutes []float64, minMinutes float64) int {
	n := 0
	for _, m := range minutes {
		if m >= minMinutes {
			n++
		}
	}
	return n
}

// LateDeduction is calculateLateDeduction.
func LateDeduction(minutes, days float64, c Config) float64 {
	switch c.LateMode {
	case "per_minute":
		return Round(minutes * c.LateAmount)
	case "flat":
		return Round(days * c.LateAmount)
	}
	return 0
}

// JabatanExpense is calculateJabatanExpense.
func JabatanExpense(annualGross float64, c Config) float64 {
	return math.Min(annualGross*c.JabatanPercentage, c.JabatanMaxPerYear)
}

// ThpDayBasis is "THP" for a one-day deduction (EPIC-059 R5): the monthly
// base salary + transport + meal + position allowance of the structure.
func ThpDayBasis(in Input) float64 {
	return in.BaseSalary + in.TransportAllowance + in.MealAllowance + in.PositionAllowance
}

// Calculate is calculatePayroll.
func Calculate(in Input, c Config) Result {
	overtimeRate := c.OvertimeMultiplier
	if in.OvertimeRate != nil {
		overtimeRate = *in.OvertimeRate
	}
	holidayRate := c.OvertimeHolidayMult
	if in.OvertimeHolidayRate != nil {
		holidayRate = *in.OvertimeHolidayRate
	}
	factor := 1.0
	if in.ProrateFactor != nil {
		factor = *in.ProrateFactor
	}
	factor = math.Min(1, math.Max(0, factor))
	fullBase := in.BaseSalary
	div := in.DivisorDays
	perDay := func(amount, days float64) float64 {
		if div <= 0 {
			return 0
		}
		return amount / div * days
	}

	// G1 / G8: basis gaji. daily: base_salary adalah upah per hari.
	var base, dayWage float64
	switch in.PayBasis {
	case "daily":
		base = Round(fullBase * in.AttendanceDays)
		dayWage = fullBase
	case "monthly_prorated":
		// Hari kerja aktif yang dibayar (EPIC-059): hadir + cuti dibayar.
		paid := in.PaidDays
		if paid == 0 {
			paid = in.AttendanceDays
		}
		// Tidak pernah melebihi pokok bulanan × faktor cakupan.
		base = math.Min(Round(perDay(fullBase, paid)), Round(fullBase*factor))
		dayWage = perDay(fullBase, 1)
	default:
		base = Round(fullBase * factor)
		dayWage = perDay(fullBase, 1)
	}

	// G4: lembur per skema. Bucket "holiday" = libur nasional + hari istirahat.
	hourly := 0.0
	if c.OvertimeHourlyDivisor > 0 {
		hourly = fullBase / c.OvertimeHourlyDivisor
	}
	var overtimePay float64
	switch in.OvertimeScheme {
	case "production":
		overtimePay = Round(c.OvertimeProductionRate * (in.OvertimeHours + 2*in.OvertimeHolidayHrs))
	case "freelance":
		overtimePay = Round(dayWage / 8 * in.OvertimeHolidayHrs)
	case "daily_worker":
		overtimePay = Round(dayWage / 5 * (in.OvertimeHours + in.OvertimeHolidayHrs))
	default:
		overtimePay = Overtime(in.OvertimeHours, hourly, overtimeRate) + Overtime(in.OvertimeHolidayHrs, hourly, holidayRate)
	}

	thr := 0.0
	if in.IncludeThr && (in.ContractType != nil || in.EmploymentStatus == "permanent") {
		thr = THR(fullBase, in.JoinDate, in.PeriodYear)
	}

	// G2: tunjangan flat (ikut prorata kontrak) atau per hari tunjangan.
	allowance := func(amount float64, daily bool) float64 {
		if daily {
			return Round(perDay(amount, in.AllowanceDays))
		}
		return Round(amount * factor)
	}
	fixed := Round(in.FixedAllowance * factor)
	variable := Round(in.VariableAllowance * factor)
	transport := allowance(in.TransportAllowance, in.TransportPerDay)
	meal := allowance(in.MealAllowance, in.MealPerDay)
	housing := Round(in.HousingAllowance * factor)
	// Jabatan per hari mengikuti hari dibayar (EPIC-059); transport & makan
	// mengikuti hari efektif (AllowanceDays).
	positionDays := in.PaidDays
	if positionDays == 0 {
		positionDays = in.AllowanceDays
	}
	position := Round(in.PositionAllowance * factor)
	if in.PositionPerDay {
		position = math.Min(Round(perDay(in.PositionAllowance, positionDays)), position)
	}
	health := 0.0
	if in.HealthEligible { // G7
		health = Round(in.HealthAllowance * factor)
	}

	// G6: komponen manual.
	bonus := Round(in.Bonus + manualSum(in.ManualInputs, ManualBonusCodes))
	otherEarning := Round(manualSum(in.ManualInputs, ManualEarningCodes))
	nontax := Round(manualSum(in.ManualInputs, ManualNontaxCodes))
	otherDeduction := Round(manualSum(in.ManualInputs, ManualDeductionCodes))

	gross := base + fixed + variable + transport + meal + housing + position + health + overtimePay + thr + bonus + otherEarning

	b := CalculateBPJS(base+fixed, in.BpjsTkEnrolled, in.BpjsKesEnrolled, in.TaperaEnrolled, c)
	// daily / monthly_prorated pay only the days attended: an unpaid leave day
	// is already unpaid (Odoo: unpaid leave just lowers the worked days).
	unpaid := 0.0
	if in.PayBasis != "daily" && in.PayBasis != "monthly_prorated" {
		unpaid = UnpaidLeave(base, in.WorkingDays, in.UnpaidLeaveDays)
	}

	// G3: potongan telat; LATE_OVERRIDE menggantikan hitungan otomatis.
	late := LateDeduction(in.LateMinutes, in.LateDays, c)
	if c.LateMode == "tier" {
		days, basis := LateTierDays(countLateDays(in.LateDayMinutes, c.LateMinMinutes), c.LateTiers)
		if basis == "thp" {
			if in.PayBasis == "daily" {
				// BaseSalary is the daily wage here: one day = wage + allowances/divisor.
				late = Round(dayWage*days + perDay(in.TransportAllowance+in.MealAllowance+in.PositionAllowance, days))
			} else {
				late = Round(perDay(ThpDayBasis(in), days))
			}
		} else {
			late = Round(dayWage * days)
		}
	}
	if v, ok := manualFind(in.ManualInputs, ManualLateOverrideCode); ok {
		late = Round(v)
	}

	// G6: PIP dan PIS (EPIC-059 R6/R7). Komponen per_day sudah tidak dibayar
	// pada hari tidak hadir, jadi yang dipotong hanya bagian yang tetap dibayar:
	// PIP = pokok (bukan upah harian) + jabatan + transport/makan flat;
	// PIS (sakit + melahirkan) = transport/makan flat. Intern tidak dipotong PIP.
	flatTransport, flatMeal := in.TransportAllowance, in.MealAllowance
	if in.TransportPerDay {
		flatTransport = 0
	}
	if in.MealPerDay {
		flatMeal = 0
	}
	personal, sick := 0.0, 0.0
	if c.PersonalLeaveDeduction && in.PersonalLeaveDays > 0 && !IsInternStatus(in.EmploymentStatus) {
		pipBasis := in.PositionAllowance + flatTransport + flatMeal
		if in.PayBasis != "daily" {
			pipBasis += fullBase
		}
		personal = Round(perDay(pipBasis, in.PersonalLeaveDays))
	}
	if sickDays := in.SickLeaveDays + in.MaternityLeaveDays; c.SickLeaveDeduction && sickDays > 0 {
		sick = Round(perDay(flatTransport+flatMeal, sickDays))
	}

	method := c.Pph21Method
	if method == "" {
		method = "annualized"
	}
	var taxable, pphAnnual, pph, dpp, terRate float64
	var category string
	if method == "ter" { // G5
		tkEmployee := b.Employee.TkJht + b.Employee.TkJp
		tkEmployer := b.Employer.TkJht + b.Employer.TkJp + b.Employer.TkJkk + b.Employer.TkJkm
		var bpjs float64
		switch c.Pph21BpjsBasis {
		case "employer": // DJP: hanya premi JKK, JKM, Kesehatan yang dibayar pemberi kerja
			bpjs = b.Employer.TkJkk + b.Employer.TkJkm + b.Employer.Kes
		case "employee":
			bpjs = tkEmployee + b.Employee.Kes
		default:
			bpjs = tkEmployer + b.Employer.Kes + tkEmployee + b.Employee.Kes
		}
		dpp = gross + bpjs
		taxable = dpp
		category = TERCategory(in.PTKPStatus)
		if in.IsTaxable {
			pph, pphAnnual, terRate = pph21TER(in, dpp, tkEmployee, c)
		} else if in.PeriodMonth != 12 {
			terRate = TERRate(category, dpp)
		}
	} else {
		annualGross := gross * 12
		taxable = math.Max(0, annualGross-b.Employee.Total*12-JabatanExpense(annualGross, c))
		if in.IsTaxable {
			pphAnnual, pph = PPh21(taxable, in.PTKPStatus, c)
		}
	}

	total := b.Employee.TkJht + b.Employee.TkJp + b.Employee.Kes + b.Employee.Tapera +
		pph + unpaid + late + in.LoanDeduction + personal + sick + otherDeduction
	net := gross + nontax - total
	employer := b.Employer.Total

	return Result{
		BaseSalary: Round(base), FixedAllowance: fixed, VariableAllowance: variable,
		TransportAllowance: transport, MealAllowance: meal, HousingAllowance: housing,
		OvertimePay: Round(overtimePay), Thr: Round(thr), Bonus: bonus,
		OtherEarning: otherEarning, GrossSalary: Round(gross),

		BpjsTkJhtDeduction: b.Employee.TkJht, BpjsTkJpDeduction: b.Employee.TkJp,
		BpjsKesDeduction: b.Employee.Kes, TaperaDeduction: Round(b.Employee.Tapera),
		Pph21Deduction: Round(pph), UnpaidLeaveDeduction: Round(unpaid),
		LateDeduction: Round(late), LoanDeduction: Round(in.LoanDeduction),
		OtherDeduction: otherDeduction, TotalDeductions: Round(total),

		OvertimeHours: in.OvertimeHours,
		NetSalary:     Round(net),

		BpjsTkJhtEmployer: b.Employer.TkJht, BpjsTkJpEmployer: b.Employer.TkJp,
		BpjsTkJkkEmployer: b.Employer.TkJkk, BpjsTkJkmEmployer: b.Employer.TkJkm,
		BpjsKesEmployer: b.Employer.Kes, TaperaEmployer: b.Employer.Tapera,
		TotalEmployerContribution: Round(employer),

		TaxableIncome: Round(taxable), PTKPAmount: PTKPAmount(in.PTKPStatus, c),
		Pph21Annual: Round(pphAnnual), Pph21Monthly: Round(pph),

		PositionAllowance: position, HealthAllowance: health, OtherEarningNontax: nontax,
		PersonalLeaveDeduction: personal, SickLeaveDeduction: sick,
		Pph21Method: method, TERCategory: category, Pph21Dpp: Round(dpp), TERRate: terRate,
	}
}

// RunTotals is the payroll_runs total columns (sumRunTotals).
type RunTotals struct {
	TotalGross        float64 `json:"total_gross"`
	TotalDeductions   float64 `json:"total_deductions"`
	TotalNet          float64 `json:"total_net"`
	TotalBjtkEmployee float64 `json:"total_bjtk_employee"`
	TotalBjtkEmployer float64 `json:"total_bjtk_employer"`
	TotalPph21        float64 `json:"total_pph21"`
}

// SumRunTotals adds the per-employee results.
func SumRunTotals(results []Result) RunTotals {
	var t RunTotals
	for _, r := range results {
		t.TotalGross += r.GrossSalary + r.OtherEarningNontax // all earnings: gross = net + deductions
		t.TotalDeductions += r.TotalDeductions
		t.TotalNet += r.NetSalary
		t.TotalBjtkEmployee += r.BpjsTkJhtDeduction + r.BpjsTkJpDeduction + r.BpjsKesDeduction + r.TaperaDeduction
		t.TotalBjtkEmployer += r.TotalEmployerContribution
		t.TotalPph21 += r.Pph21Deduction
	}
	return t
}

package domain

import "math"

// fallbackWorkingDays applies without any shift pattern or attendance.
const fallbackWorkingDays = 20

// Employee is the employee columns a payroll calculation reads.
type Employee struct {
	ID               string
	EmploymentStatus string
	JoinDate         string // YYYY-MM-DD
}

// Salary is the active employee_salary row. Amounts are numeric text.
type Salary struct {
	BaseSalary, FixedAllowance, VariableAllowance          any
	TransportAllowance, MealAllowance, HousingAllowance    any
	PTKPStatus                                             *string
	IsTaxable, BpjsTkEnrolled, BpjsKesEnrolled, TaperaEnrl *bool

	// EPIC-055 (Odoo rules); nil = the pre-EPIC behaviour.
	PayBasis                              *string // monthly | monthly_prorated | daily
	WorkingDaysDivisor                    any     // numeric text; nil = computed working days
	TransportMode, MealMode, PositionMode *string // flat | per_day
	PositionAllowance, HealthAllowance    any
	HealthMinServiceDays                  *int
	OvertimeScheme                        *string // standard | production | freelance | daily_worker
}

// BuildOptions are the config switches the input builder needs.
type BuildOptions struct {
	// OvertimeRequireClockout pays approved overtime only on a day with a
	// clock-out (the pre-EPIC-055 behaviour); false pays the requested hours.
	OvertimeRequireClockout bool
	// OvertimeRestFromSchedule pays overtime on a day without a shift in the
	// employee's schedule at the rest-day rate (EPIC-055; off = before).
	OvertimeRestFromSchedule bool
}

// DefaultBuildOptions keep the pre-EPIC-055 behaviour.
func DefaultBuildOptions() BuildOptions { return BuildOptions{OvertimeRequireClockout: true} }

// Contract is an employment contract touching the period (newest first).
type Contract struct {
	ContractType *string
	StartDate    *string
	EndDate      *string
}

// Leave is an approved leave overlapping the period.
type Leave struct {
	StartDate, EndDate *string
	LeaveType          string
}

// WorkforceRecords are the HRIS records of one employee for one period:
// attendance, shift pattern, approved overtime, approved leaves, contracts.
type WorkforceRecords struct {
	Attendance []AttendanceRow
	Schedule   []ShiftRow
	Overtime   []OvertimeRequest
	Leaves     []Leave
	Contracts  []Contract
}

// BuildInput is loadEmployeePayrollInput after its queries: working days
// from the shift pattern (then attendance, then 20), contract coverage
// proration, realized overtime split by holidays, late stats, unpaid leave
// clamped to the period and due loan installments.
func BuildInput(emp Employee, salary Salary, rec WorkforceRecords, loans []LoanRow, holidays map[string]bool, month, year int, includeThr bool) Input {
	start, end := PeriodBounds(month, year)
	return BuildInputRange(emp, salary, rec, loans, holidays, month, year, start, end, includeThr)
}

// BuildInputRange is BuildInput over the attendance period [start, end] of
// the payroll month (the cut-off period from the attendance settings; the
// calendar month by default). Loans stay keyed by month/year.
func BuildInputRange(emp Employee, salary Salary, rec WorkforceRecords, loans []LoanRow, holidays map[string]bool, month, year int,
	start, end string, includeThr bool, opts ...BuildOptions) Input {
	opt := DefaultBuildOptions()
	if len(opts) > 0 {
		opt = opts[0]
	}

	var schedule []ShiftRow
	for _, r := range rec.Schedule {
		if r.EffectiveFrom != "" && (r.EffectiveTo == nil || *r.EffectiveTo >= start) {
			schedule = append(schedule, r)
		}
	}

	var coverages []DateRange
	var latest *Contract
	for i, c := range rec.Contracts {
		cs := DateText(c.StartDate)
		if cs == nil {
			continue
		}
		cov := Coverage(*cs, DateText(c.EndDate), start, end)
		if cov == nil {
			continue
		}
		coverages = append(coverages, *cov)
		if latest == nil {
			latest = &rec.Contracts[i]
		}
	}
	merged := MergeRanges(coverages)
	hasContract := len(merged) > 0
	partial := hasContract && !(len(merged) == 1 && merged[0].Start == start && merged[0].End == end)

	scheduled, hasSchedule := ScheduledDays(schedule, start, end)
	coveredScheduled, coveredCalendar := 0, 0
	for _, r := range merged {
		n, _ := ScheduledDays(schedule, r.Start, r.End)
		coveredScheduled += n
		coveredCalendar += len(EachDate(r.Start, r.End))
	}
	totalCalendar := len(EachDate(start, end))
	inCoverage := func(d string) bool {
		if !hasContract {
			return true
		}
		for _, r := range merged {
			if d >= r.Start && d <= r.End {
				return true
			}
		}
		return false
	}
	attendanceDays := 0
	for _, a := range rec.Attendance {
		if a.Status != "absent" && (!partial || inCoverage(a.Date)) {
			attendanceDays++
		}
	}

	var workingDays float64
	factor := 1.0
	switch {
	case hasSchedule && scheduled > 0:
		workingDays = float64(scheduled)
		if partial {
			workingDays = float64(coveredScheduled)
			factor = float64(coveredScheduled) / float64(scheduled)
		}
	case attendanceDays > 0:
		workingDays = float64(attendanceDays)
		if partial {
			factor = float64(coveredCalendar) / float64(totalCalendar)
		}
	default:
		workingDays = fallbackWorkingDays
		if partial {
			factor = float64(coveredCalendar) / float64(totalCalendar)
			workingDays = math.Max(1, Round(fallbackWorkingDays*factor))
		}
	}
	if workingDays <= 0 {
		workingDays = fallbackWorkingDays
		if partial {
			workingDays = 1
		}
	}

	present := 0
	for _, a := range rec.Attendance {
		if (a.Status == "present" || a.Status == "late") && (!partial || inCoverage(a.Date)) {
			present++
		}
	}
	lateDays, lateMinutes := LateStats(rec.Attendance)
	var lateDayMinutes []float64
	for _, a := range rec.Attendance {
		if ((a.IsLate != nil && *a.IsLate) || a.Status == "late") && (!partial || inCoverage(a.Date)) {
			lateDayMinutes = append(lateDayMinutes, OrZero(a.LateMinutes))
		}
	}
	var restSchedule []ShiftRow
	if opt.OvertimeRestFromSchedule {
		restSchedule = schedule
	}
	ot := SplitOvertimeWith(rec.Overtime, rec.Attendance, holidays, restSchedule, opt.OvertimeRequireClockout)

	unpaid, personal, sick := 0, 0, 0
	for _, l := range rec.Leaves {
		s, e := DateText(l.StartDate), DateText(l.EndDate)
		if s == nil || e == nil {
			continue
		}
		days := ClampedLeaveDays(LeaveRange{*s, *e}, start, end)
		// Hanya unpaid/personal/sick yang memotong. leader (Izin Leader) dan
		// jenis cuti lain sengaja tidak dihitung (EPIC-058).
		switch l.LeaveType {
		case "unpaid":
			unpaid += days
		case "personal":
			personal += days
		case "sick":
			sick += days
		}
	}
	// Pembagi hari struktur; tanpa pembagi: hari kerja hasil hitung, kecuali
	// upah harian (Odoo: struktur kosong = base_days 0, aturan per hari tidak
	// berlaku — hari tidak hadir sudah tidak dibayar).
	divisor := OrZero(salary.WorkingDaysDivisor)
	if divisor <= 0 {
		divisor = workingDays
		if salary.PayBasis != nil && *salary.PayBasis == "daily" {
			divisor = 0
		}
	}
	minService := 0
	if salary.HealthMinServiceDays != nil {
		minService = *salary.HealthMinServiceDays
	}
	str := func(v *string, def string) string {
		if v == nil || *v == "" {
			return def
		}
		return *v
	}

	var contractType *string
	if latest != nil {
		contractType = latest.ContractType
	}
	ptkp := "TK/0"
	if salary.PTKPStatus != nil && *salary.PTKPStatus != "" {
		ptkp = *salary.PTKPStatus
	}
	orTrue := func(b *bool) bool { return b == nil || *b }

	return Input{
		EmployeeID: emp.ID, PeriodMonth: month, PeriodYear: year,
		BaseSalary:         OrZero(salary.BaseSalary),
		FixedAllowance:     OrZero(salary.FixedAllowance),
		VariableAllowance:  OrZero(salary.VariableAllowance),
		TransportAllowance: OrZero(salary.TransportAllowance),
		MealAllowance:      OrZero(salary.MealAllowance),
		HousingAllowance:   OrZero(salary.HousingAllowance),
		OvertimeHours:      ot.Regular,
		OvertimeHolidayHrs: ot.Holiday,
		WorkingDays:        workingDays,
		PresentDays:        float64(present),
		LateDays:           lateDays,
		LateMinutes:        lateMinutes,
		UnpaidLeaveDays:    float64(unpaid),
		PayBasis:           str(salary.PayBasis, "monthly"),
		DivisorDays:        divisor,
		AttendanceDays:     float64(attendanceDays),
		AllowanceDays:      float64(attendanceDays),
		TransportPerDay:    str(salary.TransportMode, "flat") == "per_day",
		MealPerDay:         str(salary.MealMode, "flat") == "per_day",
		PositionPerDay:     str(salary.PositionMode, "flat") == "per_day",
		PositionAllowance:  OrZero(salary.PositionAllowance),
		HealthAllowance:    OrZero(salary.HealthAllowance),
		HealthEligible:     HealthEligible(emp.JoinDate, end, minService),
		OvertimeScheme:     str(salary.OvertimeScheme, "standard"),
		LateDayMinutes:     lateDayMinutes,
		PersonalLeaveDays:  float64(personal),
		SickLeaveDays:      float64(sick),
		LoanDeduction:      LoanDeductionForPeriod(loans, month, year),
		LoanBreakdown:      LoanInstallmentDetails(loans, month, year),
		JoinDate:           emp.JoinDate,
		EmploymentStatus:   emp.EmploymentStatus,
		ContractType:       contractType,
		ProrateFactor:      &factor,
		PTKPStatus:         ptkp,
		IsTaxable:          orTrue(salary.IsTaxable),
		BpjsTkEnrolled:     orTrue(salary.BpjsTkEnrolled),
		BpjsKesEnrolled:    orTrue(salary.BpjsKesEnrolled),
		TaperaEnrolled:     orTrue(salary.TaperaEnrl),
		IncludeThr:         includeThr,
	}
}

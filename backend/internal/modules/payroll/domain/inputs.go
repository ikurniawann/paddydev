package domain

import "math"

// fallbackWorkingDays applies without any shift pattern or attendance.
const fallbackWorkingDays = 20

// Employee is the employee columns a payroll calculation reads.
type Employee struct {
	ID               string
	EmploymentStatus string
	JoinDate         string  // YYYY-MM-DD
	EndDate          *string // YYYY-MM-DD; resign/akhir kerja (EPIC-059 PY-04)
	// IsActive: end_date memotong cakupan hanya untuk karyawan nonaktif (R11).
	IsActive bool
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
	// HealthSubsidyAmount > 0 replaces the structure's health allowance with a
	// global BPJS Kesehatan subsidy for more than one year of service (EPIC-059).
	HealthSubsidyAmount float64
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
	// PY-04: tanggal setelah end_date karyawan NONAKTIF berada di luar cakupan.
	// periodSchedule mengukur periode penuh (pembagi & penyebut faktor): alur
	// Resign menutup jadwal pada end_date, jadi baris yang berakhir tepat pada
	// end_date dianggap terus berlaku sampai akhir periode.
	periodSchedule := schedule
	if ed := DateText(emp.EndDate); !emp.IsActive && ed != nil && *ed < end && *ed >= start {
		periodSchedule = make([]ShiftRow, len(schedule))
		for i, r := range schedule {
			if r.EffectiveTo != nil && *r.EffectiveTo == *ed {
				r.EffectiveTo = nil
			}
			periodSchedule[i] = r
		}
		if len(merged) == 0 {
			merged = []DateRange{{Start: start, End: *ed}}
		} else {
			var clipped []DateRange
			for _, r := range merged {
				if r.Start <= *ed {
					clipped = append(clipped, DateRange{Start: r.Start, End: min(r.End, *ed)})
				}
			}
			merged = clipped
		}
	}
	hasContract := len(merged) > 0
	partial := hasContract && !(len(merged) == 1 && merged[0].Start == start && merged[0].End == end)

	scheduled, hasSchedule := ScheduledDays(periodSchedule, start, end)
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

	// Hari cuti dihitung pada hari kerja (EPIC-059 R3) di dalam cakupan.
	// Hari dibayar = hadir + cuti selain unpaid pada hari kerja tanpa absensi
	// efektif; tanggal unpaid tidak pernah dibayar.
	attended := map[string]bool{}
	for _, a := range rec.Attendance {
		if a.Status != "absent" && (!partial || inCoverage(a.Date)) {
			attended[a.Date] = true
		}
	}
	unpaidDates, paidLeaveDates := map[string]bool{}, map[string]bool{}
	unpaid, personal, sick, maternity := 0, 0, 0, 0
	for _, l := range rec.Leaves {
		s, e := DateText(l.StartDate), DateText(l.EndDate)
		if s == nil || e == nil {
			continue
		}
		from, to := max(*s, start), min(*e, end)
		for _, d := range EachDate(from, to) {
			if !IsWorkDate(d, schedule, holidays) || (partial && !inCoverage(d)) {
				continue
			}
			// Hanya unpaid/personal/sick/maternity yang memotong; leader dan
			// jenis lain full paid (EPIC-058).
			if l.LeaveType == "unpaid" {
				unpaid++
				unpaidDates[d] = true
				continue
			}
			// Tanggal dengan absensi efektif sudah dibayar sebagai hari kerja:
			// tidak dipotong PIP/PIS dan tidak dihitung dua kali.
			if attended[d] {
				continue
			}
			switch l.LeaveType {
			case "personal":
				personal++
			case "sick":
				sick++
			case "maternity":
				maternity++
			}
			paidLeaveDates[d] = true
		}
	}
	paidLeave := 0
	for d := range paidLeaveDates {
		if !unpaidDates[d] {
			paidLeave++
		}
	}
	paidDays := float64(attendanceDays + paidLeave)
	if hasSchedule && scheduled > 0 && paidDays > workingDays {
		paidDays = workingDays
	}
	// Tanpa jadwal: paling banyak hari kerja (Senin–Jumat bukan libur) di
	// dalam cakupan periode.
	if !hasSchedule {
		workDates := 0
		for _, d := range EachDate(start, end) {
			if IsWorkDate(d, nil, holidays) && (!partial || inCoverage(d)) {
				workDates++
			}
		}
		paidDays = math.Min(paidDays, float64(workDates))
	}
	// Pembagi (EPIC-059 K-12): hari kerja terjadwal periode untuk gaji bulanan;
	// tanpa jadwal, pembagi struktur lalu hari kerja hasil hitung. Upah harian
	// tidak berubah (struktur kosong = aturan per hari tidak berlaku).
	isDaily := salary.PayBasis != nil && *salary.PayBasis == "daily"
	divisor := 0.0
	if !isDaily && hasSchedule && scheduled > 0 {
		divisor = float64(scheduled)
	}
	if divisor <= 0 {
		divisor = OrZero(salary.WorkingDaysDivisor)
	}
	if divisor <= 0 {
		divisor = workingDays
		if isDaily {
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

	health := OrZero(salary.HealthAllowance)
	healthEligible := HealthEligible(emp.JoinDate, end, minService)
	if opt.HealthSubsidyAmount > 0 {
		health, healthEligible = opt.HealthSubsidyAmount, ServiceOverOneYear(emp.JoinDate, end)
	}
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
		HealthAllowance:    health,
		HealthEligible:     healthEligible,
		OvertimeScheme:     str(salary.OvertimeScheme, "standard"),
		LateDayMinutes:     lateDayMinutes,
		PersonalLeaveDays:  float64(personal),
		SickLeaveDays:      float64(sick),
		MaternityLeaveDays: float64(maternity),
		PaidDays:           paidDays,
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

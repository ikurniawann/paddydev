package payroll

import (
	"math"
	"testing"

	"nuhabit/backend/internal/modules/payroll/domain"
)

// EPIC-059: slip karyawan monthly_prorated + per_day sesuai acuan payslip,
// lewat alur run yang sama dengan produksi (struktur gaji di DB, absensi dan
// jadwal dari port, kalkulasi lewat API).
func TestPayrollRunAcuanPayslip(t *testing.T) {
	e := setup(t)
	e.employee("HR Payroll", e.hr.UserID, true)
	emp := e.employee("Karyawan Acuan", "", true)
	e.exec(`INSERT INTO hris.employee_salary (employee_id, company_id, base_salary, transport_allowance, meal_allowance,
		position_allowance, pay_basis, transport_mode, meal_mode, position_mode, ptkp_status, effective_date)
		VALUES ($1, $2, 2200000, 220000, 220000, 440000, 'monthly_prorated', 'per_day', 'per_day', 'per_day', 'TK/0', '2026-01-01')`,
		emp, e.co)

	// Juni 2026: 22 hari kerja (Senin–Jumat). Hadir 20 hari, Izin Pribadi 8 Jun, Sakit 9 Jun.
	shift := "shift-pagi"
	var schedule []domain.ShiftRow
	for dow := 1; dow <= 5; dow++ {
		schedule = append(schedule, domain.ShiftRow{DayOfWeek: dow, ShiftID: &shift, EffectiveFrom: "2026-01-01"})
	}
	personalDay, sickDay := "2026-06-08", "2026-06-09"
	var attendance []domain.AttendanceRow
	for _, d := range domain.EachDate("2026-06-01", "2026-06-30") {
		if dow := domain.ISODayOfWeek(d); dow > 5 || d == personalDay || d == sickDay {
			continue
		}
		attendance = append(attendance, domain.AttendanceRow{Date: d, Status: "present", HasClockOut: true})
	}
	if len(attendance) != 20 {
		t.Fatalf("attendance rows = %d, want 20", len(attendance))
	}
	e.work.records[emp] = domain.WorkforceRecords{
		Attendance: attendance,
		Schedule:   schedule,
		Leaves: []domain.Leave{
			{StartDate: &personalDay, EndDate: &personalDay, LeaveType: "personal"},
			{StartDate: &sickDay, EndDate: &sickDay, LeaveType: "sick"},
		},
	}

	// Potongan Izin Pribadi dan Sakit aktif (seperti setting live).
	e.call(&e.hr, "PUT", "/api/hris/payroll-settings", map[string]any{"settings": map[string]any{
		"personal_leave_deduction": true, "sick_leave_deduction": true}}, 200)

	run := e.newRun(6, 2026, e.co)
	e.call(&e.hr, "POST", "/api/hris/payroll/"+run+"/calculate", nil, 200)

	col := func(name string) float64 {
		var v float64
		e.scalar(&v, `SELECT `+name+`::float8 FROM hris.payroll_details WHERE payroll_run_id = $1 AND employee_id = $2`, run, emp)
		return v
	}
	// Pokok 2.200.000/22×22; transport & makan ×20/22; jabatan ×22/22;
	// PIP = (pokok + jabatan)/22 (komponen per_day sudah tidak dibayar di hari izin); PIS = 0.
	want := map[string]float64{
		"base_salary":              2_200_000,
		"transport_allowance":      200_000,
		"meal_allowance":           200_000,
		"position_allowance":       440_000,
		"personal_leave_deduction": 120_000,
		"sick_leave_deduction":     0,
		"unpaid_leave_deduction":   0,
	}
	for name, w := range want {
		if got := col(name); math.Abs(got-w) > 0.005 {
			t.Errorf("%s = %v, want %v", name, got, w)
		}
	}
	if gross := col("gross_salary"); math.Abs(gross-3_040_000) > 0.005 {
		t.Errorf("gross_salary = %v, want 3040000", gross)
	}
}

// PY-04/R11 lewat alur run: karyawan nonaktif yang resign 9 Jun (jadwal ditutup pada
// end_date seperti alur Resign) dibayar 7/22; karyawan aktif dengan end_date di
// dalam periode dibayar penuh.
func TestPayrollRunEndDateClipsLeaversOnly(t *testing.T) {
	e := setup(t)
	e.employee("HR Payroll", e.hr.UserID, true)
	leaver := e.employee("Karyawan Resign", "", false)
	active := e.employee("Karyawan Kontrak Aktif", "", true)
	ed := "2026-06-09"
	shift := "shift-pagi"
	for _, id := range []string{leaver, active} {
		b := e.emps.briefs[id]
		b.EndDate = &ed
		e.emps.briefs[id] = b
		e.exec(`INSERT INTO hris.employee_salary (employee_id, company_id, base_salary, ptkp_status, effective_date)
			VALUES ($1, $2, 2200000, 'TK/0', '2026-01-01')`, id, e.co)
		var schedule []domain.ShiftRow
		for dow := 1; dow <= 5; dow++ {
			row := domain.ShiftRow{DayOfWeek: dow, ShiftID: &shift, EffectiveFrom: "2026-01-01"}
			if id == leaver {
				row.EffectiveTo = &ed
			}
			schedule = append(schedule, row)
		}
		e.work.records[id] = domain.WorkforceRecords{Schedule: schedule}
	}

	run := e.newRun(6, 2026, e.co)
	e.call(&e.hr, "POST", "/api/hris/payroll/"+run+"/calculate", nil, 200)

	base := func(emp string) float64 {
		var v float64
		e.scalar(&v, `SELECT base_salary::float8 FROM hris.payroll_details WHERE payroll_run_id = $1 AND employee_id = $2`, run, emp)
		return v
	}
	if got := base(leaver); math.Abs(got-700_000) > 0.005 {
		t.Errorf("leaver base_salary = %v, want 700000 (7/22)", got)
	}
	if got := base(active); math.Abs(got-2_200_000) > 0.005 {
		t.Errorf("active base_salary = %v, want 2200000 (tidak dipotong)", got)
	}
}

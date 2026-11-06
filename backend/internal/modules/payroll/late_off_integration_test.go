package payroll

import (
	"os"
	"testing"

	"nuhabit/backend/internal/modules/payroll/domain"
)

// EPIC-094 R7: kebijakan Paddy tanpa potongan keterlambatan. Migrasi
// 20261112090100 mematikan potongan telat; Potongan Izin Pribadi tetap.
func TestLateDeductionOffMigration(t *testing.T) {
	e := setup(t)
	e.employee("HR Payroll", e.hr.UserID, true)
	emp := e.employee("Karyawan Sering Telat", "", true)
	e.salary(emp, e.co, 4_400_000)
	e.call(&e.hr, "PUT", "/api/hris/payroll-settings", map[string]any{"settings": map[string]any{
		"late_deduction_mode": "flat", "late_deduction_amount": 50_000, "personal_leave_deduction": true}}, 200)

	sql, err := os.ReadFile("../../../database/migrations/deltas/20261112090100_late_deduction_off.sql")
	if err != nil {
		t.Fatalf("migration: %v", err)
	}
	e.exec(string(sql))
	var mode string
	e.scalar(&mode, `SELECT late_deduction_mode FROM hris.payroll_settings`)
	if mode != "off" {
		t.Fatalf("late_deduction_mode = %q, want off", mode)
	}

	// Juni 2099: 7 hari terlambat dan 1 hari Izin Pribadi yang disetujui.
	late := true
	var attendance []domain.AttendanceRow
	for _, d := range []string{"2099-06-01", "2099-06-02", "2099-06-03", "2099-06-04", "2099-06-05", "2099-06-08", "2099-06-09"} {
		attendance = append(attendance, domain.AttendanceRow{Date: d, Status: "late", IsLate: &late, LateMinutes: "30", HasClockOut: true})
	}
	personal := "2099-06-10"
	e.work.records[emp] = domain.WorkforceRecords{
		Attendance: attendance,
		Leaves:     []domain.Leave{{StartDate: &personal, EndDate: &personal, LeaveType: "personal"}},
	}
	run := e.newRun(6, 2099, e.co)
	e.call(&e.hr, "POST", "/api/hris/payroll/"+run+"/calculate", nil, 200)
	var lateDed, personalDed float64
	e.scalar(&lateDed, `SELECT late_deduction::float8 FROM hris.payroll_details WHERE payroll_run_id = $1 AND employee_id = $2`, run, emp)
	e.scalar(&personalDed, `SELECT personal_leave_deduction::float8 FROM hris.payroll_details WHERE payroll_run_id = $1 AND employee_id = $2`, run, emp)
	if lateDed != 0 {
		t.Errorf("late_deduction = %v, want 0", lateDed)
	}
	if personalDed <= 0 {
		t.Errorf("personal_leave_deduction = %v, want > 0", personalDed)
	}
}

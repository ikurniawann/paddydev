package app

import (
	"context"
	"regexp"
	"slices"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/modules/configuration/kit"
	"nuhabit/backend/internal/modules/hris"
	"nuhabit/backend/internal/modules/payroll"
	"nuhabit/backend/internal/modules/payroll/domain"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/scope"
)

// Payroll reads employees through hris.Employees. The reads that service
// does not cover (email lookup, direct reports, active employees in natural
// order, departments, attendance-side records) run the SQL the TS libs ran
// (lib/payroll/inputs.ts, lib/hris/workforce-auth.ts, lib/hris/holidays-db.ts
// and the PostgREST employee embeds).

func payrollPorts() payroll.Ports {
	return payroll.Ports{Employees: payrollEmployees{}, Departments: payrollDepartmentsSQL{}, Workforce: payrollWorkforceSQL{},
		Settings: kit.AppSettings{}, KPI: payrollKPISQL{}, AttendancePeriods: payrollAttendancePeriods{},
		Entities: payrollEntitiesSQL{}}
}

// payrollEntitiesSQL reads the companies and branches payroll runs per entity.
type payrollEntitiesSQL struct{}

func (payrollEntitiesSQL) List(ctx context.Context, q database.Querier) ([]payroll.Entity, error) {
	rows, err := q.Query(ctx, `SELECT c.id::text, COALESCE(c.code, ''), c.name, h.name, COALESCE(c.is_active, false)
		FROM configuration.companies c JOIN configuration.holdings h ON h.id = c.holding_id
		ORDER BY c.name, h.name`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (payroll.Entity, error) {
		var e payroll.Entity
		return e, r.Scan(&e.ID, &e.Code, &e.Name, &e.HoldingName, &e.IsActive)
	})
}

func (payrollEntitiesSQL) BranchCompanies(ctx context.Context, q database.Querier) (map[string]string, error) {
	rows, err := q.Query(ctx, `SELECT id::text, company_id::text FROM configuration.branches WHERE company_id IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var branch, company string
		if err := rows.Scan(&branch, &company); err != nil {
			return nil, err
		}
		out[branch] = company
	}
	return out, rows.Err()
}

// payrollAttendancePeriods reads the HRIS attendance period of a payroll
// month through hris.AttendancePeriods (cut-off range and closing status).
type payrollAttendancePeriods struct{ hris hris.AttendancePeriods }

var _ payroll.AttendancePeriods = payrollAttendancePeriods{}

func (a payrollAttendancePeriods) ForMonth(ctx context.Context, q database.Querier, year, month int) (*payroll.AttendancePeriod, error) {
	p, err := a.hris.ForMonth(ctx, q, year, month)
	if err != nil || p == nil {
		return nil, err
	}
	return &payroll.AttendancePeriod{ID: p.ID, Start: p.Start, End: p.End, ClosingDate: p.Closing,
		Label: p.Label(), Closed: p.Closed()}, nil
}

type payrollEmployees struct{ hris hris.Employees }

var _ payroll.Employees = payrollEmployees{}

func (e payrollEmployees) IDByUser(ctx context.Context, q database.Querier, userID string) (string, error) {
	emp, err := e.hris.ByUserID(ctx, q, userID)
	if err != nil || emp == nil {
		return "", err
	}
	return emp.ID, nil
}

// LoanApprover: see payroll.LoanApproverSQL.
func (payrollEmployees) LoanApprover(ctx context.Context, q database.Querier, employeeID string) (*string, error) {
	var id string
	err := q.QueryRow(ctx, payroll.LoanApproverSQL, employeeID).Scan(&id)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func (payrollEmployees) IDByEmail(ctx context.Context, q database.Querier, email string) (string, error) {
	var id string
	err := q.QueryRow(ctx, `SELECT id::text FROM hris.employees WHERE email = $1 LIMIT 1`, email).Scan(&id)
	if database.IsNoRows(err) {
		return "", nil
	}
	return id, err
}

// canonicalUUID is the text form of a uuid column. The TS compared
// e.id::text, so any other spelling (upper case, braces) matches nothing.
var canonicalUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func (e payrollEmployees) Briefs(ctx context.Context, q database.Querier, ids []string) (map[string]payroll.EmployeeBrief, error) {
	valid := slices.DeleteFunc(slices.Clone(ids), func(id string) bool { return !canonicalUUID.MatchString(id) })
	list, err := e.hris.ListByIDs(ctx, q, valid)
	if err != nil {
		return nil, err
	}
	out := make(map[string]payroll.EmployeeBrief, len(list))
	for _, emp := range list {
		out[emp.ID] = payroll.EmployeeBrief{
			ID: emp.ID, FullName: emp.FullName, NIP: &emp.Nip, Email: &emp.Email, Phone: &emp.Phone,
			PhotoURL: emp.PhotoURL, IsActive: emp.IsActive, EmploymentStatus: emp.EmploymentStatus,
			JoinDate: emp.JoinDate, DepartmentID: emp.DepartmentID,
			Department: emp.DepartmentName, HasDepartment: emp.DepartmentName != nil,
			Position: emp.PositionTitle, HasPosition: emp.PositionTitle != nil, ReportingTo: emp.ReportingTo,
		}
	}
	return out, nil
}

// Active keeps the TS query shape (no joins) so the rows come back in the
// same natural order the TS loop processed them in.
func (payrollEmployees) Active(ctx context.Context, q database.Querier) ([]payroll.EmployeeBrief, error) {
	rows, err := q.Query(ctx, `SELECT id::text, full_name, nip, is_active, employment_status, join_date::text,
		department_id::text FROM hris.employees WHERE is_active = $1`, true)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (payroll.EmployeeBrief, error) {
		var b payroll.EmployeeBrief
		err := r.Scan(&b.ID, &b.FullName, &b.NIP, &b.IsActive, &b.EmploymentStatus, &b.JoinDate, &b.DepartmentID)
		return b, err
	})
}

// PayrollEmployees is Active plus the inactive employees whose end_date falls
// in [start, end], so a leaver is still paid pro-rata for the days worked.
func (payrollEmployees) PayrollEmployees(ctx context.Context, q database.Querier, start, end string) ([]payroll.EmployeeBrief, error) {
	rows, err := q.Query(ctx, `SELECT id::text, full_name, nip, is_active, employment_status, join_date::text,
		department_id::text, end_date::text FROM hris.employees
		WHERE is_active = true OR (end_date BETWEEN $1::date AND $2::date)`, start, end)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (payroll.EmployeeBrief, error) {
		var b payroll.EmployeeBrief
		err := r.Scan(&b.ID, &b.FullName, &b.NIP, &b.IsActive, &b.EmploymentStatus, &b.JoinDate, &b.DepartmentID, &b.EndDate)
		return b, err
	})
}

func (payrollEmployees) DirectReports(ctx context.Context, q database.Querier, managerID string) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT id::text FROM hris.employees WHERE reporting_to = $1 AND is_active = true`, managerID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// KPIRole reads the linked account's role, as the TS snapshot does;
// "employee" for an employee without an account or role.
func (e payrollEmployees) KPIRole(ctx context.Context, q database.Querier, employeeID string) (string, bool, error) {
	emp, err := e.hris.ByID(ctx, q, employeeID)
	if err != nil || emp == nil {
		return "", false, err
	}
	if emp.UserID == nil {
		return "employee", true, nil
	}
	sc, err := scope.Load(ctx, q, *emp.UserID)
	if err != nil {
		return "", false, err
	}
	if sc.Role == nil {
		return "employee", true, nil
	}
	return *sc.Role, true, nil
}

func (e payrollEmployees) DepartmentMembers(ctx context.Context, q database.Querier, departmentID string) ([]payroll.Member, error) {
	list, err := e.hris.ActiveByDepartment(ctx, q, departmentID)
	if err != nil {
		return nil, err
	}
	out := make([]payroll.Member, len(list))
	for i, emp := range list {
		out[i] = payroll.Member{ID: emp.ID, FullName: emp.FullName}
	}
	return out, nil
}

// Companies resolves each employee's company through its account: the
// user's company, else the company of the user's branch.
func (payrollEmployees) Companies(ctx context.Context, q database.Querier, ids []string) (map[string]string, error) {
	out := map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, `SELECT e.id::text, COALESCE(u.company_id, b.company_id)::text
		FROM hris.employees e
		JOIN configuration.users u ON u.id = e.user_id
		LEFT JOIN configuration.branches b ON b.id = u.branch_id
		WHERE e.id = ANY($1::uuid[]) AND COALESCE(u.company_id, b.company_id) IS NOT NULL`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, company string
		if err := rows.Scan(&id, &company); err != nil {
			return nil, err
		}
		out[id] = company
	}
	return out, rows.Err()
}

type payrollDepartmentsSQL struct{}

var _ payroll.Departments = payrollDepartmentsSQL{}

func (payrollDepartmentsSQL) All(ctx context.Context, q database.Querier) ([]payroll.Department, error) {
	rows, err := q.Query(ctx, `SELECT id::text, name FROM hris.departments ORDER BY name`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[payroll.Department])
}

func (payrollDepartmentsSQL) ByIDs(ctx context.Context, q database.Querier, ids []string) (map[string]payroll.Department, error) {
	rows, err := q.Query(ctx, `SELECT id::text, name FROM hris.departments WHERE id::text = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, pgx.RowToStructByPos[payroll.Department])
	out := make(map[string]payroll.Department, len(list))
	for _, d := range list {
		out[d.ID] = d
	}
	return out, err
}

type payrollWorkforceSQL struct{}

var _ payroll.Workforce = payrollWorkforceSQL{}

func (payrollWorkforceSQL) PeriodRecords(ctx context.Context, q database.Querier, employeeID, start, end string) (domain.WorkforceRecords, error) {
	var rec domain.WorkforceRecords
	rows, err := q.Query(ctx, `SELECT date::text, status::text, clock_out IS NOT NULL, is_late, late_minutes::text, overtime_hours::text,
		branch_id::text, work_hours::float8
		FROM hris.attendance WHERE date >= $1 AND date <= $2 AND employee_id = $3`, start, end, employeeID)
	if err != nil {
		return rec, err
	}
	if rec.Attendance, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.AttendanceRow, error) {
		var a domain.AttendanceRow
		var date, status, late, overtime *string
		err := r.Scan(&date, &status, &a.HasClockOut, &a.IsLate, &late, &overtime, &a.BranchID, &a.WorkHours)
		if d := domain.DateText(date); d != nil {
			a.Date = *d
		}
		if status != nil {
			a.Status = *status
		}
		a.LateMinutes, a.OvertimeHours = payrollText(late), payrollText(overtime)
		return a, err
	}); err != nil {
		return rec, err
	}

	rows, err = q.Query(ctx, `SELECT day_of_week, shift_id::text, effective_from::text, effective_to::text
		FROM hris.employee_shifts WHERE employee_id = $1 AND effective_from <= $2`, employeeID, end)
	if err != nil {
		return rec, err
	}
	if rec.Schedule, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.ShiftRow, error) {
		var s domain.ShiftRow
		var from *string
		err := r.Scan(&s.DayOfWeek, &s.ShiftID, &from, &s.EffectiveTo)
		if d := domain.DateText(from); d != nil {
			s.EffectiveFrom = *d
		}
		return s, err
	}); err != nil {
		return rec, err
	}

	rows, err = q.Query(ctx, `SELECT date::text, hours::text, day_type FROM hris.overtime_requests
		WHERE employee_id = $1 AND status = 'approved' AND date >= $2 AND date <= $3`, employeeID, start, end)
	if err != nil {
		return rec, err
	}
	overtime, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (*domain.OvertimeRequest, error) {
		var date, hours *string
		var dayType string
		if err := r.Scan(&date, &hours, &dayType); err != nil {
			return nil, err
		}
		d := domain.DateText(date)
		if d == nil {
			return nil, nil
		}
		return &domain.OvertimeRequest{Date: *d, Hours: domain.OrZero(payrollText(hours)), DayType: dayType}, nil
	})
	if err != nil {
		return rec, err
	}
	for _, o := range overtime {
		if o != nil {
			rec.Overtime = append(rec.Overtime, *o)
		}
	}

	rows, err = q.Query(ctx, `SELECT start_date::text, end_date::text, leave_type::text FROM hris.leaves
		WHERE employee_id = $1 AND status = 'approved' AND start_date <= $2 AND end_date >= $3`, employeeID, end, start)
	if err != nil {
		return rec, err
	}
	if rec.Leaves, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Leave, error) {
		var l domain.Leave
		var kind *string
		err := r.Scan(&l.StartDate, &l.EndDate, &kind)
		if kind != nil {
			l.LeaveType = *kind
		}
		return l, err
	}); err != nil {
		return rec, err
	}

	rows, err = q.Query(ctx, `SELECT contract_type, start_date::text, end_date::text FROM hris.employment_contracts
		WHERE employee_id = $1 AND status <> 'draft' AND start_date <= $2
		ORDER BY start_date DESC LIMIT 50`, employeeID, end)
	if err != nil {
		return rec, err
	}
	rec.Contracts, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Contract, error) {
		var c domain.Contract
		err := r.Scan(&c.ContractType, &c.StartDate, &c.EndDate)
		return c, err
	})
	return rec, err
}

func (payrollWorkforceSQL) Holidays(ctx context.Context, q database.Querier, start, end string) (map[string]bool, error) {
	rows, err := q.Query(ctx, `SELECT holiday_date::text FROM hris.public_holidays
		WHERE deleted_at IS NULL AND status = 'aktif' AND holiday_date BETWEEN $1::date AND $2::date`, start, end)
	if err != nil {
		return nil, err
	}
	dates, err := pgx.CollectRows(rows, pgx.RowTo[string])
	out := make(map[string]bool, len(dates))
	for _, d := range dates {
		out[d] = true
	}
	return out, err
}

// Placements reads hris.employee_placements (HRIS people) running on any
// day of [start, end].
func (payrollWorkforceSQL) Placements(ctx context.Context, q database.Querier, employeeID, start, end string) ([]domain.Placement, error) {
	rows, err := q.Query(ctx, `SELECT branch_id::text, is_primary, start_date::text, end_date::text
		FROM hris.employee_placements
		WHERE employee_id = $1 AND start_date <= $3::date AND (end_date IS NULL OR end_date >= $2::date)
		ORDER BY start_date, branch_id`, employeeID, start, end)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Placement, error) {
		var p domain.Placement
		return p, r.Scan(&p.BranchID, &p.IsPrimary, &p.Start, &p.End)
	})
}

func payrollText(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

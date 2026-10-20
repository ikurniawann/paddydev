package payroll

import (
	"context"

	"nuhabit/backend/internal/modules/payroll/domain"
	"nuhabit/backend/internal/platform/database"
)

// Employees, departments, positions, attendance, shifts, leaves, overtime,
// contracts and public holidays belong to the HRIS people context
// (module hris). Payroll reads them through these ports; internal/app wires
// the adapters. Every method runs on the caller's Querier.

// EmployeeBrief is the display and payroll subset of an hris.employees row
// with its department and position names.
type EmployeeBrief struct {
	ID               string
	FullName         string
	NIP              *string
	Email            *string
	Phone            *string
	PhotoURL         *string
	IsActive         bool
	EmploymentStatus string
	JoinDate         string // YYYY-MM-DD
	DepartmentID     *string
	// Department is the departments row's name; HasDepartment is false when
	// the employee has none (the embed is null).
	Department    *string
	HasDepartment bool
	Position      *string
	HasPosition   bool
	ReportingTo   *string
}

// Employees reads hris.employees.
type Employees interface {
	// IDByUser is the employee linked to an account (employees.user_id):
	// the first matching row, as the TS query-builder .single() returns it,
	// "" when none.
	IDByUser(ctx context.Context, q database.Querier, userID string) (string, error)
	// IDByEmail is the first employee with this email, "" when none.
	IDByEmail(ctx context.Context, q database.Querier, email string) (string, error)
	// Briefs returns the employees that exist among ids (ids that are not
	// UUIDs simply match nothing).
	Briefs(ctx context.Context, q database.Querier, ids []string) (map[string]EmployeeBrief, error)
	// Active lists is_active employees in the table's natural order.
	Active(ctx context.Context, q database.Querier) ([]EmployeeBrief, error)
	// DirectReports are the active employees whose reporting_to is managerID.
	DirectReports(ctx context.Context, q database.Querier, managerID string) ([]string, error)
	// KPIRole is the role code a scorecard uses: the linked account's
	// configuration.users role, "employee" without one; found is false for
	// an unknown employee.
	KPIRole(ctx context.Context, q database.Querier, employeeID string) (role string, found bool, err error)
	// DepartmentMembers are the department's active employees ordered by
	// full_name (database collation).
	DepartmentMembers(ctx context.Context, q database.Querier, departmentID string) ([]Member, error)
	// Companies maps each employee among ids to the company of its linked
	// account (the user's company, else its branch's company); employees
	// without one are absent.
	Companies(ctx context.Context, q database.Querier, ids []string) (map[string]string, error)
	// LoanApprover is the first active employee with a login account up the
	// reporting_to chain (not the employee) with a Manager-or-above position
	// level; nil without one. Cycles and self-references stop the walk (EPIC-060).
	LoanApprover(ctx context.Context, q database.Querier, employeeID string) (*string, error)
}

// LoanApproverSQL walks reporting_to up to 10 levels; $1 = employee id.
// Mirror: domain.managerLevels (the level list must match).
const LoanApproverSQL = `WITH RECURSIVE chain AS (
	SELECT e.reporting_to AS id, 1 AS depth, ARRAY[e.id] AS seen FROM hris.employees e WHERE e.id = $1::uuid
	UNION ALL
	SELECT e.reporting_to, c.depth + 1, c.seen || e.id FROM chain c JOIN hris.employees e ON e.id = c.id
	WHERE c.depth < 10 AND e.reporting_to IS NOT NULL AND NOT e.reporting_to = ANY(c.seen || e.id))
SELECT m.id::text FROM chain c
JOIN hris.employees m ON m.id = c.id AND m.is_active
JOIN hris.positions p ON p.id = m.job_title_id
WHERE c.id <> $1::uuid
  AND m.user_id IS NOT NULL
  AND lower(trim(p.level)) IN ('manager', 'senior manager', 'general manager', 'director', 'c-level')
ORDER BY c.depth LIMIT 1`

// Member is an employee's id and name.
type Member struct {
	ID       string
	FullName string
}

// Department is an hris.departments row's id and name.
type Department struct {
	ID   string
	Name string
}

// Departments reads hris.departments.
type Departments interface {
	// All lists every department ordered by name.
	All(ctx context.Context, q database.Querier) ([]Department, error)
	// ByIDs returns the departments that exist among ids.
	ByIDs(ctx context.Context, q database.Querier, ids []string) (map[string]Department, error)
}

// Workforce reads the attendance-side records a payroll period needs.
type Workforce interface {
	PeriodRecords(ctx context.Context, q database.Querier, employeeID, start, end string) (domain.WorkforceRecords, error)
	// Holidays are the active public holiday dates in [start, end].
	Holidays(ctx context.Context, q database.Querier, start, end string) (map[string]bool, error)
	// Placements are the employee's store placements
	// (hris.employee_placements) running on any day of [start, end].
	Placements(ctx context.Context, q database.Querier, employeeID, start, end string) ([]domain.Placement, error)
}

// AppSettings reads configuration.app_settings (the company identity on
// payslips): every key, nil when missing or NULL.
type AppSettings interface {
	GetMany(ctx context.Context, q database.Querier, keys []string) (map[string]*string, error)
}

// AttendancePeriod is the HRIS attendance period of a payroll month: the
// date range payroll reads attendance, leave, overtime and placements over,
// and whether it is closed (locked).
type AttendancePeriod struct {
	ID          string
	Start, End  string // YYYY-MM-DD, inclusive
	ClosingDate string
	Label       string // "21 Sep – 20 Okt 2026"
	Closed      bool
}

// AttendancePeriods reads the attendance periods (HRIS people context).
type AttendancePeriods interface {
	// ForMonth is the period of the payroll month (materialized; a period
	// past its closing date is closed).
	ForMonth(ctx context.Context, q database.Querier, year, month int) (*AttendancePeriod, error)
}

// Entity is a configuration.companies row: an entity that pays salaries
// and runs payroll, with its holding's name.
type Entity struct {
	ID          string
	Code        string
	Name        string
	HoldingName string
	IsActive    bool
}

// Entities reads the business hierarchy (configuration context).
type Entities interface {
	// List is every company ordered by company name, then holding name.
	List(ctx context.Context, q database.Querier) ([]Entity, error)
	// BranchCompanies maps every branch id to its company id.
	BranchCompanies(ctx context.Context, q database.Querier) (map[string]string, error)
}

// Ports bundles the adapters the module needs.
type Ports struct {
	Employees   Employees
	Departments Departments
	Workforce   Workforce
	Settings    AppSettings
	Entities    Entities // companies and branches (payroll per entity)
	// KPI reads what the KPI snapshot measures in other contexts.
	KPI KPISources
	// AttendancePeriods gives the attendance period of a run; nil means the
	// calendar month, always treated as closed.
	AttendancePeriods AttendancePeriods
}

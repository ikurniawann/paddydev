package hris

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/outbox"
)

func leaveEmployeeEmbed(alias, cols string) string {
	return embedOne(alias, cols, "hris.employees", "id = leaves.employee_id")
}

func leaveApproverEmbed(cols string) string {
	return embedOne("approver", cols, "hris.employees", "id = leaves.approved_by")
}

// deptName is the nested department:departments(name) embed of employees.
var deptName = embedOne("department", "name", "hris.departments", "id = employees.department_id")

func (f leaveFilter) where() (string, []any) {
	var where []string
	var args []any
	add := func(cond string, v string) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.EmployeeID != nil && *f.EmployeeID != "" {
		add("employee_id = $%d", *f.EmployeeID)
	}
	if f.Status != nil && *f.Status != "" {
		add("status = $%d", *f.Status)
	}
	if f.LeaveType != nil && *f.LeaveType != "" {
		add("leave_type = $%d", *f.LeaveType)
	}
	if f.StartDate != nil && *f.StartDate != "" && f.EndDate != nil && *f.EndDate != "" {
		add("start_date >= $%d", *f.StartDate)
		add("end_date <= $%d", *f.EndDate)
	}
	if len(where) == 0 {
		return "", nil
	}
	return "WHERE " + strings.Join(where, " AND "), args
}

func (s *store) LeavePage(ctx context.Context, f leaveFilter, page, limit int) ([]*Row, int64, error) {
	where, args := f.where()
	total, err := countRows(ctx, s.db, `SELECT count(*) FROM hris.leaves `+where, args...)
	if err != nil {
		return nil, 0, err
	}
	n := len(args)
	rows, err := queryRows(ctx, s.db, fmt.Sprintf(`SELECT *, %s, %s FROM hris.leaves %s
		ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		leaveEmployeeEmbed("employee", "id, full_name, nip, "+deptName), leaveApproverEmbed("id, full_name, nip"),
		where, n+1, n+2), append(args, limit, (page-1)*limit)...)
	return rows, total, err
}

func (s *store) LeaveEmployee(ctx context.Context, id string) (*Row, error) {
	row, err := queryRow(ctx, s.db, `SELECT id, full_name, employment_status, is_active FROM hris.employees WHERE id = $1`, id)
	return row, ignoreBadInput(err)
}

// ActiveHolidays is loadHolidayIndex's query: only aktif rows count.
func (s *store) ActiveHolidays(ctx context.Context, start, end string) ([]domain.Holiday, error) {
	rows, err := s.db.Query(ctx, `SELECT holiday_date::text, name, type, deducts_leave
		FROM hris.public_holidays
		WHERE deleted_at IS NULL AND status = 'aktif'
		  AND holiday_date BETWEEN $1::date AND $2::date
		ORDER BY holiday_date, name`, start, end)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Holiday, error) {
		var h domain.Holiday
		err := r.Scan(&h.Date, &h.Name, &h.Type, &h.DeductsLeave)
		return h, err
	})
}

func (s *store) AnnualRemaining(ctx context.Context, employeeID string, year int) (*Row, error) {
	row, err := queryRow(ctx, s.db, `SELECT annual_leave_remaining FROM hris.leave_balances
		WHERE employee_id = $1 AND year = $2 LIMIT 1`, employeeID, year)
	return row, ignoreBadInput(err)
}

func (s *store) InsertLeave(ctx context.Context, f fields) (*Row, error) {
	return insertRow(ctx, s.db, "hris.leaves", f, "*")
}

func (s *store) Publish(ctx context.Context, topic, key string, payload any) error {
	return outbox.Publish(ctx, s.db, topic, key, payload)
}

func (s *store) LeaveDetail(ctx context.Context, id string) (*Row, error) {
	row, err := queryRow(ctx, s.db, `SELECT *, `+
		leaveEmployeeEmbed("employee", "id, full_name, nip, photo_url, email, phone, reporting_to, "+deptName+", "+
			embedOne("job_title", "title", "hris.positions", "id = employees.job_title_id"))+", "+
		leaveApproverEmbed("id, full_name, nip, email")+` FROM hris.leaves WHERE id = $1`, id)
	return row, ignoreBadInput(err)
}

func (s *store) LeaveForDecision(ctx context.Context, id string) (*Row, error) {
	row, err := queryRow(ctx, s.db, `SELECT *, `+
		leaveEmployeeEmbed("employee", "id, full_name, email, phone, reporting_to, "+
			embedOne("department", "id, name", "hris.departments", "id = employees.department_id")+", "+
			embedOne("job_title", "id, title", "hris.positions", "id = employees.job_title_id"))+
		` FROM hris.leaves WHERE id = $1`, id)
	return row, ignoreBadInput(err)
}

func (s *store) LeaveDecided(ctx context.Context, id string) (*Row, error) {
	row, err := queryRow(ctx, s.db, `SELECT *, `+
		leaveEmployeeEmbed("employee", "id, full_name, email, "+deptName)+", "+
		leaveApproverEmbed("id, full_name, email")+` FROM hris.leaves WHERE id = $1`, id)
	return row, ignoreBadInput(err)
}

func (s *store) PlainLeave(ctx context.Context, id string) (*Row, error) {
	row, err := queryRow(ctx, s.db, `SELECT * FROM hris.leaves WHERE id = $1`, id)
	return row, ignoreBadInput(err)
}

// CancelLeave cancels and refunds the quota (call inside a transaction).
func (s *store) CancelLeave(ctx context.Context, id string, refund *quotaMove) error {
	if _, err := s.db.Exec(ctx, `UPDATE hris.leaves SET status = 'cancelled' WHERE id = $1`, id); err != nil {
		return err
	}
	if refund == nil {
		return nil
	}
	_, err := s.db.Exec(ctx, `UPDATE hris.leave_balances
		SET annual_leave_used = GREATEST(annual_leave_used - $3, 0), updated_at = now()
		WHERE employee_id = $1 AND year = $2`, refund.EmployeeID, refund.Year, refund.Days)
	return err
}

func (s *store) UpdateLeave(ctx context.Context, id string, f fields) (*Row, error) {
	return updateOne(ctx, s.db, "hris.leaves", f, fields{{"id", id}}, "*")
}

func (s *store) DeleteLeave(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM hris.leaves WHERE id = $1`, id)
	return err
}

// DecideLeave records the decision and consumes the quota (call inside a
// transaction).
func (s *store) DecideLeave(ctx context.Context, id, status string, approverID, rejection *string, credit *quotaMove) error {
	if _, err := s.db.Exec(ctx, `UPDATE hris.leaves
		SET status = $2, approved_by = $3, approved_at = now(), rejection_reason = $4
		WHERE id = $1 AND status = 'pending'`, id, status, approverID, rejection); err != nil {
		return err
	}
	if credit == nil {
		return nil
	}
	_, err := s.db.Exec(ctx, `INSERT INTO hris.leave_balances (employee_id, year, annual_leave_total, annual_leave_used)
		VALUES ($1, $2, 12, $3)
		ON CONFLICT (employee_id, year)
		DO UPDATE SET annual_leave_used = leave_balances.annual_leave_used + $3, updated_at = now()`,
		credit.EmployeeID, credit.Year, credit.Days)
	return err
}

func (s *store) LeavesForExport(ctx context.Context, f leaveFilter) ([]*Row, error) {
	where, args := f.where()
	return queryRows(ctx, s.db, `SELECT *, `+
		leaveEmployeeEmbed("employee", "full_name, nip, "+deptName+", "+
			embedOne("job_title", "title", "hris.positions", "id = employees.job_title_id"))+", "+
		leaveApproverEmbed("full_name, nip")+` FROM hris.leaves `+where+` ORDER BY created_at DESC`, args...)
}

func (s *store) LeaveBalance(ctx context.Context, employeeID string, year int) (*Row, error) {
	return queryRow(ctx, s.db, `SELECT *, `+
		embedOne("employee", "id, full_name, nip, photo_url, "+deptName+", "+
			embedOne("job_title", "title", "hris.positions", "id = employees.job_title_id"),
			"hris.employees", "id = leave_balances.employee_id")+
		` FROM hris.leave_balances WHERE employee_id = $1 AND year = $2`, employeeID, year)
}

func (s *store) EmployeeJoinDate(ctx context.Context, id string) (*string, bool, error) {
	var join *string
	err := s.db.QueryRow(ctx, `SELECT join_date::text FROM hris.employees WHERE id = $1`, id).Scan(&join)
	if database.IsNoRows(err) || database.PgCode(err) == "22P02" {
		return nil, false, nil
	}
	return join, err == nil, err
}

func (s *store) InsertLeaveBalance(ctx context.Context, f fields) (*Row, error) {
	return insertRow(ctx, s.db, "hris.leave_balances", f, "*")
}

func (s *store) LeaveBalanceID(ctx context.Context, employeeID string, year int) (*string, error) {
	var id string
	err := s.db.QueryRow(ctx, `SELECT id::text FROM hris.leave_balances WHERE employee_id = $1 AND year = $2 LIMIT 1`,
		employeeID, year).Scan(&id)
	if database.IsNoRows(err) || database.PgCode(err) == "22P02" {
		return nil, nil
	}
	return &id, err
}

func (s *store) UpdateLeaveBalance(ctx context.Context, id string, f fields) (*Row, error) {
	return updateOne(ctx, s.db, "hris.leave_balances", f, fields{{"id", id}}, "*")
}

// ManagerContact is the active direct manager of an employee.
func (s *store) ManagerContact(ctx context.Context, employeeID string) (string, *string, bool, error) {
	var name string
	var phone *string
	err := s.db.QueryRow(ctx, `SELECT a.full_name, a.phone
		FROM hris.employees e
		JOIN hris.employees a ON a.id = e.reporting_to AND a.is_active = true
		WHERE e.id = $1`, employeeID).Scan(&name, &phone)
	if database.IsNoRows(err) {
		return "", nil, false, nil
	}
	return name, phone, err == nil, err
}

// EmployeePositionLevel is the level of the employee's position ("" without
// one); found is false when the employee does not exist.
func (s *store) EmployeePositionLevel(ctx context.Context, employeeID string) (string, bool, error) {
	var level *string
	err := s.db.QueryRow(ctx, `SELECT p.level FROM hris.employees e
		LEFT JOIN hris.positions p ON p.id = e.job_title_id WHERE e.id = $1`, employeeID).Scan(&level)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil || level == nil {
		return "", err == nil, err
	}
	return *level, true, nil
}

// EmployeeEmploymentStatus is hris.employees.employment_status ("" when null).
func (s *store) EmployeeEmploymentStatus(ctx context.Context, employeeID string) (string, error) {
	var status *string
	if err := s.db.QueryRow(ctx, `SELECT employment_status FROM hris.employees WHERE id = $1`, employeeID).Scan(&status); err != nil {
		return "", err
	}
	if status == nil {
		return "", nil
	}
	return *status, nil
}

// LockLeaveEmployee serialises leave requests of one employee so the quota
// and overlap checks cannot both pass for two concurrent requests.
func (s *store) LockLeaveEmployee(ctx context.Context, employeeID string) error {
	_, err := s.db.Exec(ctx, `SELECT 1 FROM hris.employees WHERE id = $1 FOR NO KEY UPDATE`, employeeID)
	return err
}

// OverlappingLeave is a pending/approved request of the employee covering any
// day of [start, end]; nil when none.
func (s *store) OverlappingLeave(ctx context.Context, employeeID, start, end string) (*Row, error) {
	return queryRow(ctx, s.db, `SELECT leave_type::text AS leave_type, status::text AS status,
		       start_date::text AS start_date, end_date::text AS end_date
		FROM hris.leaves
		WHERE employee_id = $1 AND status IN ('pending', 'approved')
		  AND start_date <= $3::date AND end_date >= $2::date
		ORDER BY start_date LIMIT 1`, employeeID, start, end)
}

// MonthlyLeaveUse sums total_days of pending/approved requests per type whose
// start_date falls in [from, to].
func (s *store) MonthlyLeaveUse(ctx context.Context, employeeID, from, to string) (map[string]float64, error) {
	rows, err := s.db.Query(ctx, `SELECT leave_type::text, COALESCE(SUM(total_days), 0)::float8
		FROM hris.leaves
		WHERE employee_id = $1 AND status IN ('pending', 'approved')
		  AND start_date BETWEEN $2::date AND $3::date
		GROUP BY leave_type`, employeeID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	use := map[string]float64{}
	for rows.Next() {
		var kind string
		var days float64
		if err := rows.Scan(&kind, &days); err != nil {
			return nil, err
		}
		use[kind] = days
	}
	return use, rows.Err()
}

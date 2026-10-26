package hris

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
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

// CancelLeave cancels a leave still in fromStatus and refunds the quota (call
// inside a transaction). errNoRow when the status changed since it was read,
// so a lost race neither refunds twice nor loses days an approval consumed.
func (s *store) CancelLeave(ctx context.Context, id, fromStatus string, refund *quotaMove) error {
	tag, err := s.db.Exec(ctx, `UPDATE hris.leaves SET status = 'cancelled' WHERE id = $1 AND status = $2`, id, fromStatus)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errNoRow
	}
	if refund == nil {
		return nil
	}
	// The carry-over part goes back to carry_over_used, the rest to the
	// annual quota; a leave approved before EPIC-068 has no carry-over part.
	carry := refund.CarryDays
	if carry == "" {
		carry = "0"
	}
	_, err = s.db.Exec(ctx, `UPDATE hris.leave_balances
		SET annual_leave_used = GREATEST(annual_leave_used - ($3::numeric - $4::numeric), 0),
		    carry_over_used = GREATEST(carry_over_used - $4::numeric, 0), updated_at = now()
		WHERE employee_id = $1 AND year = $2`, refund.EmployeeID, refund.Year, refund.Days, carry)
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
func (s *store) DecideLeave(ctx context.Context, id string, v leaveVerdict, credit *quotaMove) error {
	tag, err := s.db.Exec(ctx, `UPDATE hris.leaves
		SET status = $2, approved_by = $3, approved_at = now(), rejection_reason = $4,
		    decided_as = $5, decided_by_name = $6
		WHERE id = $1 AND status = 'pending'`, id, v.Status, v.ApproverID, v.Rejection, v.DecidedAs, v.DecidedBy)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errNoRow
	}
	if credit == nil {
		return nil
	}
	return s.consumeAnnualLeave(ctx, id, credit)
}

// consumeAnnualLeave takes an approved leave's days from the carry-over first
// (while it has not expired on the leave's start date) and the rest from the
// annual quota (EPIC-068 R5), recording the carry-over part on the leave. The
// row is created from the default rule (R1) when missing; it is locked, so a
// request that no longer fits is refused with a 400, not a CHECK violation.
func (s *store) consumeAnnualLeave(ctx context.Context, leaveID string, m *quotaMove) error {
	days, err := strconv.ParseFloat(m.Days, 64)
	if err != nil {
		return err
	}
	if err := ensureLeaveBalance(ctx, s.db, m.EmployeeID, m.Year); err != nil {
		return err
	}
	var remaining, carryAvailable float64
	err = s.db.QueryRow(ctx, `SELECT annual_leave_remaining::float8,
		(CASE WHEN carry_over_expires_on >= $3::date THEN carry_over_days - carry_over_used ELSE 0 END)::float8
		FROM hris.leave_balances WHERE employee_id = $1 AND year = $2 FOR UPDATE`,
		m.EmployeeID, m.Year, m.StartDate).Scan(&remaining, &carryAvailable)
	if database.IsNoRows(err) {
		return httpx.BadRequest("Sisa cuti tahunan tidak cukup", obj("remaining", 0, "requested", days))
	}
	if err != nil {
		return err
	}
	carry := min(days, carryAvailable)
	if rest := days - carry; rest > remaining {
		return httpx.BadRequest("Sisa cuti tahunan tidak cukup", obj("remaining", remaining+carryAvailable, "requested", days))
	}
	if _, err := s.db.Exec(ctx, `UPDATE hris.leave_balances
		SET annual_leave_used = annual_leave_used + ($3::numeric - $4::numeric),
		    carry_over_used = carry_over_used + $4::numeric, updated_at = now()
		WHERE employee_id = $1 AND year = $2`, m.EmployeeID, m.Year, days, carry); err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `UPDATE hris.leaves SET carry_over_days_used = $2 WHERE id = $1`, leaveID, carry)
	return err
}

func (s *store) LeavesForExport(ctx context.Context, f leaveFilter) ([]*Row, error) {
	where, args := f.where()
	return queryRows(ctx, s.db, `SELECT *, `+
		leaveEmployeeEmbed("employee", "full_name, nip, "+deptName+", "+
			embedOne("job_title", "title", "hris.positions", "id = employees.job_title_id"))+", "+
		leaveApproverEmbed("full_name, nip")+` FROM hris.leaves `+where+` ORDER BY created_at DESC`, args...)
}

// ManagerContact is the active direct manager of an employee.
func (s *store) ManagerContact(ctx context.Context, employeeID string) (string, *string, bool, error) {
	var name string
	var phone *string
	err := s.db.QueryRow(ctx, `SELECT a.full_name, a.phone
		FROM hris.employees e
		JOIN hris.employees a ON a.id = e.reporting_to AND a.is_active = true AND a.id <> e.id
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

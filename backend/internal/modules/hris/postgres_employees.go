package hris

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/employeekeys"
)

func (s *store) querier() database.Querier { return s.db }

// directoryColumns is DIRECTORY_COLUMNS plus its embeds (no personal data).
var directoryColumns = `id, user_id, full_name, nip, email, phone, photo_url,
  join_date, end_date, employment_status, is_active, is_access_app,
  department_id, section_id, job_title_id, reporting_to, created_at, updated_at, ` +
	embedOne("department", "id, name, code", "hris.departments", "id = employees.department_id") + ", " +
	embedOne("section", "id, name, code", "hris.sections", "id = employees.section_id") + ", " +
	embedOne("job_title", "id, title, department", "hris.positions", "id = employees.job_title_id") + ", " +
	embedOne("manager", "id, full_name, nip", "hris.employees m", "m.id = employees.reporting_to")

func (s *store) DirectoryPage(ctx context.Context, p domain.DirectoryParams) ([]*Row, int64, error) {
	var where []string
	var args []any
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if p.DepartmentID != nil && *p.DepartmentID != "" {
		add("department_id = $%d", *p.DepartmentID)
	}
	if p.SectionID != nil && *p.SectionID != "" {
		add("section_id = $%d", *p.SectionID)
	}
	if p.EmploymentStatus != nil && *p.EmploymentStatus != "" {
		add("employment_status = $%d", *p.EmploymentStatus)
	}
	if p.IsActive != nil {
		add("is_active = $%d", *p.IsActive)
	}
	if p.Search != nil {
		like := "%" + *p.Search + "%"
		args = append(args, like, like, like)
		n := len(args)
		where = append(where, fmt.Sprintf("(full_name ILIKE $%d OR email ILIKE $%d OR nip ILIKE $%d)", n-2, n-1, n))
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "WHERE " + strings.Join(where, " AND ")
	}
	total, err := countRows(ctx, s.db, `SELECT count(*) FROM hris.employees `+whereSQL, args...)
	if err != nil {
		return nil, 0, err
	}
	dir := "ASC"
	if !p.Ascending {
		dir = "DESC"
	}
	n := len(args)
	rows, err := queryRows(ctx, s.db, fmt.Sprintf(`SELECT %s FROM hris.employees %s ORDER BY %s %s LIMIT $%d OFFSET $%d`,
		directoryColumns, whereSQL, p.SortBy, dir, n+1, n+2), append(args, p.Limit, (p.Page-1)*p.Limit)...)
	return rows, total, err
}

func (s *store) EmployeeExists(ctx context.Context, column, value string, excludeID *string) (bool, error) {
	sql := `SELECT EXISTS (SELECT 1 FROM hris.employees WHERE ` + column + ` = $1`
	args := []any{value}
	if excludeID != nil {
		sql += ` AND id <> $2`
		args = append(args, *excludeID)
	}
	var exists bool
	err := s.db.QueryRow(ctx, sql+`)`, args...).Scan(&exists)
	return exists, ignoreBadInput(err)
}

func (s *store) EmployeeKeyConflict(ctx context.Context, k employeekeys.Keys, excludeID string) (*employeekeys.Conflict, error) {
	return employeekeys.Find(ctx, s.db, k, excludeID)
}

func (s *store) AutoNipsTaken(ctx context.Context, year int) ([]string, error) {
	rows, err := s.db.Query(ctx, `SELECT nip FROM hris.employees WHERE nip LIKE $1`, fmt.Sprintf("EMP-%d-%%", year))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func (s *store) InsertEmployee(ctx context.Context, f fields) (*Row, error) {
	return insertRow(ctx, s.db, "hris.employees", f, "*")
}

func (s *store) EmployeeDetail(ctx context.Context, id string) (*Row, error) {
	return queryRow(ctx, s.db, `SELECT *, `+
		embedOne("department", "id, name, code, description", "hris.departments", "id = employees.department_id")+", "+
		embedOne("section", "id, name, code, color", "hris.sections", "id = employees.section_id")+", "+
		embedOne("job_title", "id, title, department, level", "hris.positions", "id = employees.job_title_id")+", "+
		embedMany("direct_reports", "id, full_name, nip", "hris.employees r", "r.reporting_to = employees.id ORDER BY r.full_name")+`
		FROM hris.employees WHERE id = $1`, id)
}

func (s *store) EmployeeBrief(ctx context.Context, id string) (*Row, error) {
	row, err := queryRow(ctx, s.db, `SELECT id, full_name, nip FROM hris.employees WHERE id = $1`, id)
	return row, ignoreBadInput(err)
}

func (s *store) EmployeeTracked(ctx context.Context, id string) (*domain.TrackedState, error) {
	var t domain.TrackedState
	err := s.db.QueryRow(ctx, `SELECT employment_status, department_id::text, section_id::text, job_title_id::text
		FROM hris.employees WHERE id = $1`, id).Scan(&t.EmploymentStatus, &t.DepartmentID, &t.SectionID, &t.JobTitleID)
	if database.IsNoRows(err) || database.PgCode(err) == "22P02" {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *store) EmployeeLogin(ctx context.Context, id string) (string, string, error) {
	var email string
	var userID *string
	err := s.db.QueryRow(ctx, `SELECT email, user_id::text FROM hris.employees WHERE id = $1`, id).Scan(&email, &userID)
	if database.IsNoRows(err) || database.PgCode(err) == "22P02" {
		return "", "", nil
	}
	if err != nil || userID == nil {
		return email, "", err
	}
	return email, *userID, nil
}

func (s *store) UpdateEmployee(ctx context.Context, id string, f fields) (*Row, error) {
	return updateOne(ctx, s.db, "hris.employees", f, fields{{"id", id}}, "*")
}

func (s *store) DeactivateEmployee(ctx context.Context, id, endDate string, at time.Time) (*Row, error) {
	return updateOne(ctx, s.db, "hris.employees",
		fields{{"is_active", false}, {"end_date", endDate}, {"updated_at", at}},
		fields{{"id", id}}, "id, full_name, nip, is_active")
}

func (s *store) InsertEmploymentHistory(ctx context.Context, f fields) (*Row, error) {
	return insertRow(ctx, s.db, "hris.employment_history", f, "*")
}

func (s *store) EmploymentHistory(ctx context.Context, employeeID string) ([]*Row, error) {
	ref := func(alias, cols, table, col string) string {
		return embedOne(alias, cols, table, "id = employment_history."+col)
	}
	return queryRows(ctx, s.db, `SELECT *, `+strings.Join([]string{
		ref("prev_department", "id, name", "hris.departments", "prev_department_id"),
		ref("new_department", "id, name", "hris.departments", "new_department_id"),
		ref("prev_section", "id, name", "hris.sections", "prev_section_id"),
		ref("new_section", "id, name", "hris.sections", "new_section_id"),
		ref("prev_job_title", "id, title", "hris.positions", "prev_job_title_id"),
		ref("new_job_title", "id, title", "hris.positions", "new_job_title_id"),
	}, ", ")+` FROM hris.employment_history WHERE employee_id = $1 ORDER BY effective_date DESC`, employeeID)
}

func (s *store) EmployeeDocuments(ctx context.Context, employeeID string) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT * FROM hris.employee_documents WHERE employee_id = $1 ORDER BY created_at DESC`, employeeID)
}

func (s *store) InsertEmployeeDocument(ctx context.Context, f fields) (*Row, error) {
	return insertRow(ctx, s.db, "hris.employee_documents", f, "*")
}

func (s *store) DeleteEmployeeDocument(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM hris.employee_documents WHERE id = $1`, id)
	return err
}

func (s *store) UpdateEmployeeDocument(ctx context.Context, id string, f fields) (*Row, error) {
	return updateOne(ctx, s.db, "hris.employee_documents", f, fields{{"id", id}}, "*")
}

func (s *store) Departments(ctx context.Context) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT id, name, code, parent_department_id FROM hris.departments ORDER BY name ASC`)
}

func (s *store) LifecycleEmployee(ctx context.Context, id string) (*Row, error) {
	return queryRow(ctx, s.db, `SELECT id, full_name, user_id, join_date, end_date, employment_status, is_active, created_at
		FROM hris.employees WHERE id = $1`, id)
}

func (s *store) OnboardingProgress(ctx context.Context, employeeID string) (*Row, error) {
	return queryRow(ctx, s.db, `SELECT count(*)::int AS total,
		count(*) FILTER (WHERE completed)::int AS completed,
		max(completed_at) AS last_completed_at
		FROM hris.onboarding_checklists WHERE employee_id = $1`, employeeID)
}

func (s *store) LifecycleHistory(ctx context.Context, employeeID string) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT h.id, h.change_type, h.effective_date, h.reason, h.notes,
		h.prev_employment_status, h.new_employment_status,
		pd.name AS prev_department_name, nd.name AS new_department_name,
		pj.title AS prev_job_title, nj.title AS new_job_title
		FROM hris.employment_history h
		LEFT JOIN hris.departments pd ON pd.id = h.prev_department_id
		LEFT JOIN hris.departments nd ON nd.id = h.new_department_id
		LEFT JOIN hris.positions pj ON pj.id = h.prev_job_title_id
		LEFT JOIN hris.positions nj ON nj.id = h.new_job_title_id
		WHERE h.employee_id = $1
		ORDER BY h.effective_date, h.created_at`, employeeID)
}

func (s *store) LatestOffboarding(ctx context.Context, employeeID string) (*Row, error) {
	return queryRow(ctx, s.db, `SELECT id, status, resignation_type, resignation_date, last_working_day,
		clearance_hrd, clearance_it, clearance_finance, clearance_manager, completed_at
		FROM hris.offboarding_checklists WHERE employee_id = $1
		ORDER BY created_at DESC LIMIT 1`, employeeID)
}

func (s *store) EmployeeShiftHistory(ctx context.Context, employeeID string) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT es.id, es.day_of_week, es.shift_id, es.effective_from, es.effective_to,
		s.name AS shift_name, s.start_time, s.end_time
		FROM hris.employee_shifts es
		LEFT JOIN hris.shifts s ON s.id = es.shift_id
		WHERE es.employee_id = $1
		ORDER BY es.effective_from DESC, es.day_of_week ASC`, employeeID)
}

// SaveShiftPattern replaces the patterns starting on/after effectiveFrom and
// closes the running one the day before (call inside a transaction).
func (s *store) SaveShiftPattern(ctx context.Context, employeeID, effectiveFrom string, days []patternDay, actorName string) error {
	if _, err := s.db.Exec(ctx, `DELETE FROM hris.employee_shifts WHERE employee_id = $1 AND effective_from >= $2`,
		employeeID, effectiveFrom); err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, `UPDATE hris.employee_shifts
		SET effective_to = ($2::date - 1)
		WHERE employee_id = $1 AND effective_from < $2
		  AND (effective_to IS NULL OR effective_to >= $2)`, employeeID, effectiveFrom); err != nil {
		return err
	}
	for _, d := range days {
		if _, err := s.db.Exec(ctx, `INSERT INTO hris.employee_shifts
			(employee_id, day_of_week, shift_id, effective_from, created_by_name)
			VALUES ($1, $2, $3, $4, $5)`, employeeID, d.DayOfWeek, d.ShiftID, effectiveFrom, actorName); err != nil {
			return err
		}
	}
	return nil
}

func (s *store) IsDirectSubordinate(ctx context.Context, managerID, employeeID string) (bool, error) {
	var ok bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM hris.employees
		WHERE id = $1 AND reporting_to = $2 AND is_active = true)`, employeeID, managerID).Scan(&ok)
	return ok, err
}

func (s *store) EmployeeName(ctx context.Context, id string) (*string, error) {
	var name *string
	err := s.db.QueryRow(ctx, `SELECT full_name FROM hris.employees WHERE id = $1`, id).Scan(&name)
	if database.IsNoRows(err) {
		return nil, nil
	}
	return name, err
}

package hris

import (
	"context"
	"fmt"
	"strings"
)

var attendanceListColumns = `id, employee_id, date, clock_in, clock_out,
  clock_in_location, clock_out_location, clock_in_photo_url, clock_out_photo_url,
  shift_id, scheduled_start, scheduled_end, work_hours, break_minutes,
  status, is_late, late_minutes, notes, created_at, updated_at, branch_id,
  is_overtime, overtime_hours, validated_at, is_manual, adjusted_at, adjusted_by_name, adjustment_count,
  clock_in_geofence, clock_in_distance_m, clock_out_geofence, clock_out_distance_m,
  min_work_hours, is_short_hours, ` +
	embedOne("shift", "id, name", "hris.shifts", "id = attendance.shift_id") + ", " +
	embedOne("branch", "id, code, name", "configuration.branches", "id = attendance.branch_id") + ", " +
	embedOne("employee", "id, full_name, nip, photo_url, department_id, job_title_id", "hris.employees", "id = attendance.employee_id")

func (s *store) AttendancePage(ctx context.Context, p attendanceListParams) ([]*Row, int64, error) {
	var where []string
	var args []any
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if p.EmployeeID != nil && *p.EmployeeID != "" {
		add("employee_id = $%d", *p.EmployeeID)
	}
	if p.Date != nil && *p.Date != "" {
		add("date = $%d", *p.Date)
	}
	if p.StartDate != nil && *p.StartDate != "" && p.EndDate != nil && *p.EndDate != "" {
		add("date >= $%d", *p.StartDate)
		add("date <= $%d", *p.EndDate)
	}
	if p.Status != nil && *p.Status != "" {
		add("status = $%d", *p.Status)
	}
	if p.LateOnly {
		add("is_late = $%d", true)
	}
	switch b := strOr(p.BranchID); {
	case b == "none":
		where = append(where, "branch_id IS NULL")
	case b != "":
		add("branch_id = $%d", b)
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "WHERE " + strings.Join(where, " AND ")
	}
	total, err := countRows(ctx, s.db, `SELECT count(*) FROM hris.attendance `+whereSQL, args...)
	if err != nil {
		return nil, 0, err
	}
	n := len(args)
	rows, err := queryRows(ctx, s.db, fmt.Sprintf(`SELECT %s FROM hris.attendance %s ORDER BY date DESC LIMIT $%d OFFSET $%d`,
		attendanceListColumns, whereSQL, n+1, n+2), append(args, p.Limit, (p.Page-1)*p.Limit)...)
	return rows, total, err
}

func (s *store) AttendanceDetail(ctx context.Context, id string) (*Row, error) {
	return queryRow(ctx, s.db, `SELECT *, `+
		embedOne("employee", "id, full_name, nip, photo_url, email, department_id, job_title_id", "hris.employees", "id = attendance.employee_id")+
		` FROM hris.attendance WHERE id = $1`, id)
}

func (s *store) UpdateAttendance(ctx context.Context, id string, f fields) (*Row, error) {
	return updateOne(ctx, s.db, "hris.attendance", f, fields{{"id", id}}, "*")
}

func (s *store) DeleteAttendance(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM hris.attendance WHERE id = $1`, id)
	return err
}

// RosterRows is the roster query of loadDailyRoster; super admin account
// shells are excluded by user id.
func (s *store) RosterRows(ctx context.Context, date string, dow int, superAdmins []string) ([]*Row, error) {
	return queryRows(ctx, s.db, `WITH scheduled AS (
		  SELECT DISTINCT ON (es.employee_id) es.employee_id, es.shift_id
		  FROM hris.v_employee_schedule es
		  WHERE es.day_of_week = $2
		    AND es.effective_from <= $1::date
		    AND (es.effective_to IS NULL OR es.effective_to >= $1::date)
		  ORDER BY es.employee_id, es.effective_from DESC, es.priority
		)
		SELECT e.id AS employee_id, e.full_name, e.nip, e.photo_url,
		       d.name AS department_name, p.title AS job_title,
		       (sc.employee_id IS NOT NULL) AS has_schedule,
		       sc.shift_id AS scheduled_shift_id,
		       s.name AS shift_name,
		       s.start_time::text AS shift_start,
		       s.end_time::text AS shift_end,
		       s.late_tolerance_minutes, s.is_overnight,
		       a.id AS attendance_id, a.clock_in, a.clock_out,
		       a.work_hours::text, a.is_late, a.late_minutes,
		       a.clock_in_photo_url, a.clock_out_photo_url,
		       a.shift_id AS attendance_shift_id,
		       a.clock_in_geofence, a.clock_in_distance_m, a.clock_out_geofence, a.clock_out_distance_m,
		       a.notes AS attendance_notes, a.min_work_hours::text AS min_work_hours, a.is_short_hours,
		       l.id AS leave_id, l.leave_type::text
		FROM hris.employees e
		LEFT JOIN scheduled sc ON sc.employee_id = e.id
		LEFT JOIN hris.shifts s ON s.id = sc.shift_id
		LEFT JOIN hris.attendance a ON a.employee_id = e.id AND a.date = $1::date
		LEFT JOIN LATERAL (
		  SELECT lv.id, lv.leave_type
		  FROM hris.leaves lv
		  WHERE lv.employee_id = e.id AND lv.status = 'approved'
		    AND lv.start_date <= $1::date AND lv.end_date >= $1::date
		  ORDER BY lv.created_at DESC LIMIT 1
		) l ON true
		LEFT JOIN hris.departments d ON d.id = e.department_id
		LEFT JOIN hris.positions p ON p.id = e.job_title_id
		WHERE e.is_active
		  AND (e.user_id IS NULL OR NOT (e.user_id = ANY($3::uuid[])))
		ORDER BY e.full_name`, date, dow, nonNil(superAdmins))
}

func (s *store) ActiveShifts(ctx context.Context) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT id, name, start_time::text, end_time::text, is_overnight, sort_order
		FROM hris.shifts WHERE is_active
		ORDER BY sort_order NULLS LAST, start_time`)
}

func (s *store) HolidaysOn(ctx context.Context, date string) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT name, type FROM hris.public_holidays
		WHERE deleted_at IS NULL AND status = 'aktif' AND holiday_date = $1::date
		ORDER BY name`, date)
}

func (s *store) AttendanceMonth(ctx context.Context, month, year float64, employeeID *string) (*Row, error) {
	return queryRow(ctx, s.db, `SELECT count(*) AS total_records,
		count(*) FILTER (WHERE is_late) AS late_count,
		count(*) FILTER (WHERE shift_id IS NULL) AS off_schedule,
		round(avg(work_hours), 1) AS avg_work_hours
		FROM hris.attendance
		WHERE date_part('month', date) = $1 AND date_part('year', date) = $2
		  AND ($3::uuid IS NULL OR employee_id = $3)`, month, year, employeeID)
}

func (s *store) AttendanceToday(ctx context.Context, employeeID *string) (*Row, error) {
	return queryRow(ctx, s.db, `SELECT count(*) AS present_today,
		count(*) FILTER (WHERE is_late) AS late_today
		FROM hris.attendance
		WHERE date = (now() + interval '7 hours')::date
		  AND ($1::uuid IS NULL OR employee_id = $1)`, employeeID)
}

func (s *store) ActiveEmployeeCount(ctx context.Context, superAdmins []string) (int64, error) {
	return countRows(ctx, s.db, `SELECT count(*) FROM hris.employees e
		WHERE e.is_active AND (e.user_id IS NULL OR NOT (e.user_id = ANY($1::uuid[])))`, nonNil(superAdmins))
}

func (s *store) ScheduleRows(ctx context.Context, employeeID string) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT es.day_of_week, es.shift_id,
		es.effective_from::text, es.effective_to::text,
		s.name AS shift_name, s.start_time::text, s.end_time::text,
		s.is_overnight
		FROM hris.v_employee_schedule es
		LEFT JOIN hris.shifts s ON s.id = es.shift_id
		WHERE es.employee_id = $1
		ORDER BY es.effective_from DESC, es.priority, es.day_of_week ASC`, employeeID)
}

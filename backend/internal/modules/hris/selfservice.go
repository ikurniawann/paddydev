package hris

import (
	"context"
	"strconv"
	"time"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/iam"
)

// Employee self service (/api/hris/me, /me/beranda, /me/team), in-app
// notifications and navigation badges: lib/hris/me-profile, team,
// notifications-repo, nav-badges.

// SelfServiceRepo is the ESS storage.
type SelfServiceRepo interface {
	EmployeeCard(ctx context.Context, employeeID string) (*Row, error)
	CurrentLeaveBalance(ctx context.Context, employeeID string) (*Row, error)
	ESSSchedule(ctx context.Context, employeeID string) ([]domain.ESSScheduleRow, error)
	AttendanceSummary(ctx context.Context, employeeID string, month, year int) (*Row, error)
	RecentLeaves(ctx context.Context, employeeID string) ([]*Row, error)
	RecentOvertime(ctx context.Context, employeeID string) ([]*Row, error)
	AnnouncementHeadlines(ctx context.Context, employeeID string) ([]*Row, error)
	DirectReports(ctx context.Context, managerID string) ([]*Row, error)
	TeamScheduleSummary(ctx context.Context, ids []string) ([]*Row, error)
	Notifications(ctx context.Context, userID string, unreadOnly bool, limit int) ([]*Row, int64, error)
	MarkNotificationsRead(ctx context.Context, userID string, id *string) error
	CountPending(ctx context.Context, table string) (int64, error)
	CountESSUpdates(ctx context.Context, table, employeeID, module string) (int64, error)
	LastSeen(ctx context.Context, employeeID, module string) (*Row, error)
	CountUnreadAnnouncements(ctx context.Context, employeeID string) (int64, error)
	MarkESSModuleSeen(ctx context.Context, employeeID, module string) error
}

// Me is loadMe: identity, this year's leave quota, today's shift.
func (s *Service) Me(ctx context.Context, employeeID string) (*Row, error) {
	card, balance, schedule, err := s.essBasics(ctx, employeeID)
	if err != nil {
		return nil, err
	}
	return obj("employee", card, "leave_balance", balance,
		"today_shift", domain.TodayShiftOf(schedule, domain.TodayWIB(s.now())),
		"has_schedule", len(schedule) > 0), nil
}

func (s *Service) essBasics(ctx context.Context, employeeID string) (any, any, []domain.ESSScheduleRow, error) {
	card, err := s.repo.EmployeeCard(ctx, employeeID)
	if err != nil {
		return nil, nil, nil, err
	}
	balance, err := s.repo.CurrentLeaveBalance(ctx, employeeID)
	if err != nil {
		return nil, nil, nil, err
	}
	schedule, err := s.repo.ESSSchedule(ctx, employeeID)
	if err != nil {
		return nil, nil, nil, err
	}
	return nullable(card), nullable(balance), schedule, nil
}

// nullable turns a nil *Row into a JSON null.
func nullable(r *Row) any {
	if r == nil {
		return nil
	}
	return r
}

// numOrNil is `value ? Number(value) : null` for numeric text.
func numOrNil(s *string) any {
	if s == nil || *s == "" {
		return nil
	}
	return domain.JSNumber(*s)
}

// Beranda is loadBeranda: the ESS home in one round trip.
func (s *Service) Beranda(ctx context.Context, emp string) (*Row, error) {
	today := domain.TodayWIB(s.now())
	wib := domain.NowWIB(s.now())
	month, year := int(wib.Month()), wib.Year()

	card, balance, schedule, err := s.essBasics(ctx, emp)
	if err != nil {
		return nil, err
	}
	att, err := s.repo.AttendanceSummary(ctx, emp, month, year)
	if err != nil {
		return nil, err
	}
	q := s.repo.querier()
	payslip, err := s.payroll.LatestPaidPayslip(ctx, q, emp)
	if err != nil {
		return nil, err
	}
	loans, err := s.payroll.ActiveLoans(ctx, q, emp)
	if err != nil {
		return nil, err
	}
	leaves, err := s.repo.RecentLeaves(ctx, emp)
	if err != nil {
		return nil, err
	}
	overtime, err := s.repo.RecentOvertime(ctx, emp)
	if err != nil {
		return nil, err
	}
	loanRequests, err := s.payroll.RecentLoans(ctx, q, emp)
	if err != nil {
		return nil, err
	}
	announcements, err := s.repo.AnnouncementHeadlines(ctx, emp)
	if err != nil {
		return nil, err
	}
	kpi, err := s.payroll.LatestKPISummary(ctx, q, emp)
	if err != nil {
		return nil, err
	}

	var recent []domain.RecentRequest
	for _, l := range leaves {
		recent = append(recent, domain.RecentRequest{Kind: "cuti", ID: l.Str("id"), Label: l.Str("leave_type"),
			Detail: l.Str("start_date") + " → " + l.Str("end_date"), Status: l.Str("status"),
			CreatedAt: l.Str("created_at"), Href: "/dashboard/me/cuti"})
	}
	for _, o := range overtime {
		recent = append(recent, domain.RecentRequest{Kind: "lembur", ID: o.Str("id"), Label: "Lembur " + o.Str("date"),
			Detail: domain.Clock5(o.StrPtr("start_time")) + "–" + domain.Clock5(o.StrPtr("end_time")),
			Status: o.Str("status"), CreatedAt: o.Str("created_at"), Href: "/dashboard/me/lembur"})
	}
	for _, p := range loanRequests {
		recent = append(recent, domain.RecentRequest{Kind: "pinjaman", ID: p.ID, Label: p.LoanType,
			Detail: domain.FormatRupiah(p.PrincipalAmount), Status: p.Status, CreatedAt: p.CreatedAt, Href: "/dashboard/me/pinjaman"})
	}
	recentOut := domain.MergeRecentRequests(recent)

	unread := 0
	for _, a := range announcements {
		if !a.Bool("is_read") {
			unread++
		}
	}
	items := announcements
	if len(items) > 4 {
		items = items[:4]
	}

	var latestPayslip any
	if payslip != nil {
		latestPayslip = obj("net_salary", domain.JSNumber(payslip.NetSalary), "run_name", payslip.RunName,
			"period_month", payslip.PeriodMonth, "period_year", payslip.PeriodYear, "paid_at", payslip.PaidAt)
	}
	var kpiOut any
	if kpi.Count > 0 {
		kpiOut = obj("count", kpi.Count, "avg_achievement", numOrNil(kpi.AvgAchievement), "avg_score", numOrNil(kpi.AvgScore))
	}

	return obj(
		"employee", card,
		"leave_balance", balance,
		"today_shift", domain.TodayShiftOf(schedule, today),
		"has_schedule", len(schedule) > 0,
		"week_schedule", domain.WeekScheduleOf(schedule, today),
		"attendance", obj(
			"month", month, "year", year,
			"present", att.Num("present"), "late", att.Num("late"), "off_schedule", att.Num("off_schedule"),
			"avg_work_hours", numOrNil(att.StrPtr("avg_work_hours")),
			"clocked_in_today", att.Num("clocked_in_today") > 0,
		),
		"latest_payslip", latestPayslip,
		"active_loans", obj("count", loans.Count, "total_remaining", domain.JSNumber(loans.TotalRemaining),
			"monthly_installment", domain.JSNumber(loans.MonthlyInstallment)),
		"recent_requests", recentOut,
		"announcements", obj("items", items, "unread", unread, "total", len(announcements)),
		"kpi", kpiOut,
	), nil
}

// Team is listTeamWithSchedule: active direct reports with today's pattern.
func (s *Service) Team(ctx context.Context, managerID string) ([]*Row, error) {
	members, err := s.repo.DirectReports(ctx, managerID)
	if err != nil || len(members) == 0 {
		return members, err
	}
	ids := make([]string, len(members))
	for i, m := range members {
		ids[i] = m.Str("id")
	}
	summary, err := s.repo.TeamScheduleSummary(ctx, ids)
	if err != nil {
		return nil, err
	}
	byEmployee := map[string]*Row{}
	for _, r := range summary {
		byEmployee[r.Str("employee_id")] = r
	}
	for _, m := range members {
		sum, since := any(nil), any(nil)
		if r := byEmployee[m.Str("id")]; r != nil {
			text := strconv.Itoa(int(r.Num("work_days"))) + " hari kerja/minggu"
			if names := r.Str("shift_names"); names != "" {
				text += " — " + names
			}
			sum, since = text, r.Get("effective_from")
		}
		m.Set("schedule_summary", sum)
		m.Set("schedule_since", since)
	}
	return members, nil
}

/* ── Notifications ───────────────────────────────────────────────────── */

func (s *Service) Notifications(ctx context.Context, userID string, unreadOnly bool, limit int) (*Row, error) {
	rows, total, err := s.repo.Notifications(ctx, userID, unreadOnly, limit)
	if err != nil {
		return nil, asPlainError(err)
	}
	return obj("data", rows, "pagination", obj("limit", limit, "total", total)), nil
}

func (s *Service) MarkNotificationsRead(ctx context.Context, userID string, all bool, id *string) (*Row, error) {
	if all {
		return msgOf("All notifications marked as read"), s.repo.MarkNotificationsRead(ctx, userID, nil)
	}
	return msgOf("Notification marked as read"), s.repo.MarkNotificationsRead(ctx, userID, id)
}

/* ── Navigation badges ───────────────────────────────────────────────── */

var (
	essModules    = []string{"leaves", "overtime", "loans"}
	approvalHrefs = map[string]string{"leaves": "/dashboard/hris/leaves", "overtime": "/dashboard/hris/overtime", "loans": "/dashboard/hris/loans"}
	essHrefs      = map[string]string{"leaves": "/dashboard/me/cuti", "overtime": "/dashboard/me/lembur", "loans": "/dashboard/me/pinjaman"}
	moduleTables  = map[string]string{"leaves": "hris.leaves", "overtime": "hris.overtime_requests"}
)

const announcementsHref = "/dashboard/me/pengumuman"

func isESSModule(m string) bool { return moduleTables[m] != "" || m == "loans" }

// NavBadges is buildNavBadges: HR sees pending approvals, an employee sees
// updates to their own requests since they last opened the page.
func (s *Service) NavBadges(ctx context.Context, a *Actor) (*Row, error) {
	badges := newRow()
	q := s.repo.querier()
	if a.IsHR {
		for _, m := range essModules {
			var n int64
			var err error
			if m == "loans" {
				n, err = s.payroll.PendingLoanCount(ctx, q)
			} else {
				n, err = s.repo.CountPending(ctx, moduleTables[m])
			}
			if err != nil {
				return nil, err
			}
			badges.Set(approvalHrefs[m], n)
		}
	}
	// HR Absensi: employee attendance requests waiting for whoever decides
	// them (EPIC-065 R2; "approve" on the Absensi leaf, EPIC-067 R11).
	if a.CanApprove(iam.LeafAbsensi) {
		n, err := s.repo.PendingAttendanceRequestCount(ctx)
		if err != nil {
			return nil, err
		}
		badges.Set(attendanceHref, n)
	}
	if a.EmployeeID != nil {
		for _, m := range essModules {
			var n int64
			var err error
			if m == "loans" {
				seen, err := s.repo.LastSeen(ctx, *a.EmployeeID, m)
				if err != nil {
					return nil, err
				}
				var since *time.Time
				if seen != nil {
					if t, ok := seen.Time("last_seen_at"); ok {
						since = &t
					}
				}
				n, err = s.payroll.LoanUpdatesSince(ctx, q, *a.EmployeeID, since)
				if err != nil {
					return nil, err
				}
			} else {
				n, err = s.repo.CountESSUpdates(ctx, moduleTables[m], *a.EmployeeID, m)
			}
			if err != nil {
				return nil, err
			}
			badges.Set(essHrefs[m], n)
		}
		n, err := s.repo.CountUnreadAnnouncements(ctx, *a.EmployeeID)
		if err != nil {
			return nil, err
		}
		badges.Set(announcementsHref, n)
		team, err := s.teamPendingTotal(ctx, *a.EmployeeID)
		if err != nil {
			return nil, err
		}
		badges.Set(teamApprovalsHref, team)
	}
	return badges, nil
}

func (s *Service) UnreadAnnouncements(ctx context.Context, employeeID string) (int64, error) {
	return s.repo.CountUnreadAnnouncements(ctx, employeeID)
}

func (s *Service) MarkESSModuleSeen(ctx context.Context, employeeID, module string) error {
	return s.repo.MarkESSModuleSeen(ctx, employeeID, module)
}

/* ── SQL ─────────────────────────────────────────────────────────────── */

func (s *store) EmployeeCard(ctx context.Context, employeeID string) (*Row, error) {
	return queryRow(ctx, s.db, `SELECT e.id, e.full_name, e.nip, e.join_date::text, e.employment_status, e.photo_url,
		p.title AS position_title, d.name AS department_name
		FROM hris.employees e
		LEFT JOIN hris.positions p ON p.id = e.job_title_id
		LEFT JOIN hris.departments d ON d.id = e.department_id
		WHERE e.id = $1`, employeeID)
}

func (s *store) CurrentLeaveBalance(ctx context.Context, employeeID string) (*Row, error) {
	return queryRow(ctx, s.db, `SELECT year, annual_leave_total, annual_leave_used, annual_leave_remaining
		FROM hris.leave_balances
		WHERE employee_id = $1 AND year = date_part('year', now())::int`, employeeID)
}

func (s *store) ESSSchedule(ctx context.Context, employeeID string) ([]domain.ESSScheduleRow, error) {
	rows, err := s.db.Query(ctx, `SELECT es.day_of_week, es.shift_id::text,
		es.effective_from::text, es.effective_to::text,
		s.name, s.start_time::text, s.end_time::text, s.late_tolerance_minutes
		FROM hris.v_employee_schedule es
		LEFT JOIN hris.shifts s ON s.id = es.shift_id
		WHERE es.employee_id = $1
		ORDER BY es.priority`, employeeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ESSScheduleRow{}
	for rows.Next() {
		var r domain.ESSScheduleRow
		var tol *int32
		if err := rows.Scan(&r.DayOfWeek, &r.ShiftID, &r.EffectiveFrom, &r.EffectiveTo,
			&r.ShiftName, &r.StartTime, &r.EndTime, &tol); err != nil {
			return nil, err
		}
		if tol != nil {
			v := int64(*tol)
			r.LateToleranceMinutes = &v
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *store) AttendanceSummary(ctx context.Context, employeeID string, month, year int) (*Row, error) {
	return queryRow(ctx, s.db, `SELECT count(*) AS present,
		count(*) FILTER (WHERE is_late) AS late,
		count(*) FILTER (WHERE shift_id IS NULL) AS off_schedule,
		round(avg(work_hours), 1) AS avg_work_hours,
		count(*) FILTER (WHERE date = (now() + interval '7 hours')::date) AS clocked_in_today
		FROM hris.attendance
		WHERE employee_id = $1
		  AND date_part('month', date) = $2 AND date_part('year', date) = $3`, employeeID, month, year)
}

func (s *store) RecentLeaves(ctx context.Context, employeeID string) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT id, leave_type, start_date::text, end_date::text, status, created_at::text
		FROM hris.leaves WHERE employee_id = $1 ORDER BY created_at DESC LIMIT 5`, employeeID)
}

func (s *store) RecentOvertime(ctx context.Context, employeeID string) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT id, date::text, start_time::text, end_time::text, status, source, created_at::text
		FROM hris.overtime_requests WHERE employee_id = $1 ORDER BY created_at DESC LIMIT 5`, employeeID)
}

// visibleTo is the "published, in its window, targeting the employee"
// predicate of the announcement feed (alias a, employee row e).
const visibleTo = `a.status = 'published'
  AND (a.publish_at IS NULL OR a.publish_at <= now())
  AND (a.expires_at IS NULL OR a.expires_at > now())
  AND (
    a.target_scope = 'global'
    OR EXISTS (
      SELECT 1 FROM hris.announcement_departments ad
      WHERE ad.announcement_id = a.id AND ad.department_id = e.department_id
    )
  )`

func (s *store) AnnouncementHeadlines(ctx context.Context, employeeID string) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT a.id, a.title, a.cover_image_url, a.tags, a.is_pinned,
		a.publish_at::text, a.created_at::text,
		(r.employee_id IS NOT NULL) AS is_read
		FROM hris.announcements a
		JOIN hris.employees e ON e.id = $1
		LEFT JOIN hris.announcement_reads r ON r.announcement_id = a.id AND r.employee_id = $1
		WHERE `+visibleTo+`
		ORDER BY a.is_pinned DESC, COALESCE(a.publish_at, a.created_at) DESC
		LIMIT 20`, employeeID)
}

func (s *store) DirectReports(ctx context.Context, managerID string) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT e.id, e.full_name, e.nip, e.photo_url, e.schedule_type,
		p.title AS position_title, d.name AS department_name
		FROM hris.employees e
		LEFT JOIN hris.positions p ON p.id = e.job_title_id
		LEFT JOIN hris.departments d ON d.id = e.department_id
		WHERE e.reporting_to = $1 AND e.is_active = true
		ORDER BY e.full_name`, managerID)
}

func (s *store) TeamScheduleSummary(ctx context.Context, ids []string) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT es.employee_id,
		string_agg(DISTINCT s.name, ', ' ORDER BY s.name) AS shift_names,
		count(*) FILTER (WHERE es.shift_id IS NOT NULL)::int AS work_days,
		max(es.effective_from)::text AS effective_from
		FROM hris.employee_shifts es
		LEFT JOIN hris.shifts s ON s.id = es.shift_id
		WHERE es.employee_id = ANY($1::uuid[])
		  AND es.effective_from <= CURRENT_DATE
		  AND (es.effective_to IS NULL OR es.effective_to >= CURRENT_DATE)
		GROUP BY es.employee_id`, ids)
}

func (s *store) Notifications(ctx context.Context, userID string, unreadOnly bool, limit int) ([]*Row, int64, error) {
	where := `WHERE user_id = $1`
	if unreadOnly {
		where += ` AND is_read = false`
	}
	total, err := countRows(ctx, s.db, `SELECT count(*) FROM public.notifications `+where, userID)
	if err != nil {
		return nil, 0, err
	}
	rows, err := queryRows(ctx, s.db, `SELECT * FROM public.notifications `+where+` ORDER BY created_at DESC LIMIT $2`, userID, limit)
	return rows, total, err
}

// MarkNotificationsRead marks one notification (id) or every unread one.
func (s *store) MarkNotificationsRead(ctx context.Context, userID string, id *string) error {
	if id == nil {
		_, err := s.db.Exec(ctx, `UPDATE public.notifications SET is_read = true, read_at = now()
			WHERE user_id = $1 AND is_read = false`, userID)
		return err
	}
	_, err := s.db.Exec(ctx, `UPDATE public.notifications SET is_read = true, read_at = now()
		WHERE id = $1 AND user_id = $2`, *id, userID)
	return err
}

func (s *store) CountPending(ctx context.Context, table string) (int64, error) {
	return countRows(ctx, s.db, `SELECT count(*) FROM `+table+` WHERE status = 'pending'`)
}

func (s *store) CountESSUpdates(ctx context.Context, table, employeeID, module string) (int64, error) {
	return countRows(ctx, s.db, `SELECT count(*) FROM `+table+` t
		LEFT JOIN hris.ess_module_reads r ON r.employee_id = t.employee_id AND r.module = $2
		WHERE t.employee_id = $1
		  AND t.status <> 'pending'
		  AND t.updated_at > COALESCE(r.last_seen_at, to_timestamp(0))`, employeeID, module)
}

func (s *store) LastSeen(ctx context.Context, employeeID, module string) (*Row, error) {
	return queryRow(ctx, s.db, `SELECT last_seen_at FROM hris.ess_module_reads WHERE employee_id = $1 AND module = $2`,
		employeeID, module)
}

func (s *store) CountUnreadAnnouncements(ctx context.Context, employeeID string) (int64, error) {
	return countRows(ctx, s.db, `SELECT count(*)
		FROM hris.announcements a
		JOIN hris.employees e ON e.id = $1
		LEFT JOIN hris.announcement_reads r ON r.announcement_id = a.id AND r.employee_id = $1
		WHERE r.employee_id IS NULL AND `+visibleTo, employeeID)
}

func (s *store) MarkESSModuleSeen(ctx context.Context, employeeID, module string) error {
	_, err := s.db.Exec(ctx, `INSERT INTO hris.ess_module_reads (employee_id, module, last_seen_at)
		VALUES ($1, $2, now())
		ON CONFLICT (employee_id, module) DO UPDATE SET last_seen_at = now()`, employeeID, module)
	return err
}

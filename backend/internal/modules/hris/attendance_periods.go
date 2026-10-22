package hris

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
)

// Attendance period closing (closing absensi). Periods are derived from
// hris.attendance_settings and materialized in hris.attendance_periods on
// demand. A period whose closing date has passed is locked even when nobody
// pressed "Closing": the first read or write that touches it marks it
// closed by "Sistem" (close_mode auto). Every function here runs on a
// Querier with single statements, so the payroll adapter can use it on the
// caller's transaction too.

// AttendancePeriod is one hris.attendance_periods row.
type AttendancePeriod struct {
	ID                string
	Year, Month       int
	Start, End        string
	Closing           string
	ClosingCustom     bool
	Status            string
	RangeFrozen       bool
	ClosedAt          *time.Time
	ClosedByName      *string
	CloseMode         *string
	CloseSummary      json.RawMessage
	ReopenedAt        *time.Time
	ReopenedByName    *string
	ReopenReason      *string
	ReopenCount       int
	MaterializedToday bool
}

// Closed reports a locked period.
func (p *AttendancePeriod) Closed() bool { return p.Status == periodClosed }

// Label is "21 Sep – 20 Okt 2026".
func (p *AttendancePeriod) Label() string { return domain.PeriodRangeLabel(p.Start, p.End) }

// Name is the payroll month: "Oktober 2026".
func (p *AttendancePeriod) Name() string {
	return fmt.Sprintf("%s %d", domain.MonthNameID(p.Month), p.Year)
}

const (
	periodOpen   = "open"
	periodClosed = "closed"
	sistemName   = "Sistem"
)

const periodColumns = `id::text, period_year, period_month, start_date::text, end_date::text, closing_date::text,
	closing_date_custom, status, range_frozen, closed_at, closed_by_name, close_mode, close_summary,
	reopened_at, reopened_by_name, reopen_reason, reopen_count`

func scanPeriod(row pgx.Row) (*AttendancePeriod, error) {
	var p AttendancePeriod
	var month int16
	var summary []byte
	err := row.Scan(&p.ID, &p.Year, &month, &p.Start, &p.End, &p.Closing, &p.ClosingCustom, &p.Status, &p.RangeFrozen,
		&p.ClosedAt, &p.ClosedByName, &p.CloseMode, &summary, &p.ReopenedAt, &p.ReopenedByName, &p.ReopenReason, &p.ReopenCount)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.Month = int(month)
	if len(summary) > 0 {
		p.CloseSummary = summary
	}
	return &p, nil
}

// loadPeriodSettings reads the settings row (defaults when missing).
func loadPeriodSettings(ctx context.Context, q database.Querier) (domain.PeriodSettings, *Row, error) {
	s := domain.PeriodSettings{CutoffDay: 0, ClosingOffsetDays: 3}
	row, err := queryRow(ctx, q, `SELECT cutoff_day, closing_offset_days, updated_at, updated_by_name
		FROM hris.attendance_settings WHERE id`)
	if err != nil || row == nil {
		return s, obj("cutoff_day", 0, "closing_offset_days", 3, "updated_at", nil, "updated_by_name", nil), err
	}
	s.CutoffDay = int(row.Num("cutoff_day"))
	s.ClosingOffsetDays = int(row.Num("closing_offset_days"))
	return s, row, nil
}

func periodByMonth(ctx context.Context, q database.Querier, year, month int) (*AttendancePeriod, error) {
	return scanPeriod(q.QueryRow(ctx, `SELECT `+periodColumns+` FROM hris.attendance_periods
		WHERE period_year = $1 AND period_month = $2`, year, month))
}

func periodByID(ctx context.Context, q database.Querier, id string) (*AttendancePeriod, error) {
	if !domain.IsUUID(id) {
		return nil, nil
	}
	return scanPeriod(q.QueryRow(ctx, `SELECT `+periodColumns+` FROM hris.attendance_periods WHERE id = $1`, id))
}

// frozenNeighbour is the period of year/month when its range is frozen.
func frozenNeighbour(ctx context.Context, q database.Querier, year, month int) (*domain.Neighbour, error) {
	var n domain.Neighbour
	err := q.QueryRow(ctx, `SELECT start_date::text, end_date::text FROM hris.attendance_periods
		WHERE period_year = $1 AND period_month = $2 AND range_frozen`, year, month).Scan(&n.Start, &n.End)
	if database.IsNoRows(err) {
		return nil, nil
	}
	return &n, err
}

// resolveRange is the period of year/month from the settings and its
// frozen neighbours.
func resolveRange(ctx context.Context, q database.Querier, s domain.PeriodSettings, year, month int) (domain.PeriodRange, error) {
	py, pm := domain.ShiftMonth(year, month, -1)
	ny, nm := domain.ShiftMonth(year, month, 1)
	prev, err := frozenNeighbour(ctx, q, py, pm)
	if err != nil {
		return domain.PeriodRange{}, err
	}
	next, err := frozenNeighbour(ctx, q, ny, nm)
	if err != nil {
		return domain.PeriodRange{}, err
	}
	return domain.ResolvePeriod(s, year, month, prev, next), nil
}

// materializePeriod returns the period of year/month, inserting it from the
// settings when it has no row yet and closing it automatically when its
// closing date is past today ("YYYY-MM-DD", WIB).
func materializePeriod(ctx context.Context, q database.Querier, s domain.PeriodSettings, year, month int, today string) (*AttendancePeriod, error) {
	p, err := periodByMonth(ctx, q, year, month)
	if err != nil {
		return nil, err
	}
	if p == nil {
		r, err := resolveRange(ctx, q, s, year, month)
		if err != nil {
			return nil, err
		}
		if _, err := q.Exec(ctx, `INSERT INTO hris.attendance_periods (period_year, period_month, start_date, end_date, closing_date)
			VALUES ($1, $2, $3, $4, $5) ON CONFLICT (period_year, period_month) DO NOTHING`,
			year, month, r.Start, r.End, r.Closing); err != nil {
			return nil, err
		}
		if p, err = periodByMonth(ctx, q, year, month); err != nil || p == nil {
			return nil, errOr(err, errors.New("attendance period vanished after insert"))
		}
	}
	if p.Status == periodOpen && domain.PeriodLocked(p.Closing, today) {
		return autoClosePeriod(ctx, q, p, today)
	}
	return p, nil
}

// periodSummarySQL counts the attendance of a period for the closing
// summary: employees, rows, rows without clock-out, unvalidated and
// manually adjusted rows, plus the attendance requests still pending (they
// cannot be approved once the period is closed). $1 and $2 may occur more
// than once; autoClosePeriod renumbers every occurrence.
const periodSummarySQL = `SELECT json_build_object(
	'employees', count(DISTINCT a.employee_id),
	'rows', count(a.id),
	'without_clock_out', count(a.id) FILTER (WHERE a.clock_in IS NOT NULL AND a.clock_out IS NULL),
	'unvalidated', count(a.id) FILTER (WHERE a.validated_at IS NULL),
	'adjusted', count(a.id) FILTER (WHERE a.is_manual OR a.adjusted_at IS NOT NULL),
	'late', count(a.id) FILTER (WHERE a.is_late),
	'pending_requests', (SELECT count(*) FROM hris.attendance_requests q
	                      WHERE q.status = 'pending' AND q.date BETWEEN $1::date AND $2::date))
	FROM hris.attendance a WHERE a.date BETWEEN $1::date AND $2::date`

// autoClosePeriod marks an open period whose closing date passed as closed
// by "Sistem" at the start of the day after the closing date (WIB), with
// its event and audit row, in one statement.
func autoClosePeriod(ctx context.Context, q database.Querier, p *AttendancePeriod, today string) (*AttendancePeriod, error) {
	label := p.Name() + " (" + p.Label() + ")"
	_, err := q.Exec(ctx, `WITH summary AS (`+strings.ReplaceAll(strings.ReplaceAll(periodSummarySQL, "$1", "$3"), "$2", "$4")+`),
		upd AS (
		  UPDATE hris.attendance_periods SET status = 'closed', range_frozen = true,
		         closed_at = ((closing_date + 1)::timestamp AT TIME ZONE 'Asia/Jakarta'),
		         closed_by = NULL, closed_by_name = $5, close_mode = 'auto',
		         close_summary = (SELECT * FROM summary)::jsonb
		   WHERE id = $1 AND status = 'open' AND closing_date < $2::date
		  RETURNING id, closing_date, closed_at, close_summary),
		ev AS (
		  INSERT INTO hris.attendance_period_events (period_id, action, actor_name, created_at, details)
		  SELECT id, 'auto_close', $5, closed_at, jsonb_build_object('closing_date', closing_date, 'summary', close_summary) FROM upd
		  RETURNING period_id)
		INSERT INTO audit.audit_log (actor_name, action, entity, entity_id, entity_label, after)
		SELECT $5, 'attendance_period.auto_close', 'hris.attendance_periods', id::text, $6,
		       jsonb_build_object('status', 'closed', 'close_mode', 'auto', 'summary', close_summary)
		  FROM upd`, p.ID, today, p.Start, p.End, sistemName, label)
	if err != nil {
		return nil, err
	}
	return periodByID(ctx, q, p.ID)
}

// periodContaining is the (materialized) period whose range holds date.
func periodContaining(ctx context.Context, q database.Querier, date, today string) (*AttendancePeriod, error) {
	if _, ok := domain.ParseDate(date); !ok {
		return nil, httpx.BadRequest("Format tanggal tidak valid")
	}
	// A date inside a closed period is answered by that row whatever the
	// settings say now.
	p, err := scanPeriod(q.QueryRow(ctx, `SELECT `+periodColumns+` FROM hris.attendance_periods
		WHERE $1::date BETWEEN start_date AND end_date ORDER BY range_frozen DESC LIMIT 1`, date))
	if err != nil {
		return nil, err
	}
	if p != nil {
		if p.Status == periodOpen && domain.PeriodLocked(p.Closing, today) {
			return autoClosePeriod(ctx, q, p, today)
		}
		return p, nil
	}
	s, _, err := loadPeriodSettings(ctx, q)
	if err != nil {
		return nil, err
	}
	y, m := domain.PeriodKeyFor(s, date)
	for range 4 {
		p, err := materializePeriod(ctx, q, s, y, m, today)
		if err != nil {
			return nil, err
		}
		switch {
		case date < p.Start:
			y, m = domain.ShiftMonth(y, m, -1)
		case date > p.End:
			y, m = domain.ShiftMonth(y, m, 1)
		default:
			return p, nil
		}
	}
	return nil, fmt.Errorf("no attendance period contains %s", date)
}

// closedPeriodError is the message every rejected attendance write gets.
func closedPeriodError(p *AttendancePeriod) error {
	return httpx.Conflict(fmt.Sprintf("Periode absensi %s sudah di-closing. Absensi di periode ini tidak bisa diubah; "+
		"minta super admin/HR manager membuka ulang periode bila perlu koreksi.", p.Label()))
}

// ensureAttendanceOpen rejects a write to an attendance date inside a
// closed period.
func (s *Service) ensureAttendanceOpen(ctx context.Context, date string) error {
	if len(date) >= 10 {
		date = date[:10]
	}
	p, err := periodContaining(ctx, s.repo.querier(), date, domain.TodayWIB(s.now()))
	if err != nil {
		return err
	}
	if p.Closed() {
		return closedPeriodError(p)
	}
	return nil
}

// periodWriteError turns the database guard's error (an attendance write
// that raced a closing) into the same 409.
func periodWriteError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "P0001" && pg.Hint == "attendance_period_closed" {
		return httpx.Conflict(pg.Message)
	}
	return err
}

/* ── Read service for other contexts ─────────────────────────────────── */

// AttendancePeriods is the period lookup other contexts (payroll) use on
// their own Querier.
type AttendancePeriods struct {
	// Now is the clock (nil: time.Now).
	Now func() time.Time
}

// ForMonth is the (materialized) attendance period of a payroll month.
func (a AttendancePeriods) ForMonth(ctx context.Context, q database.Querier, year, month int) (*AttendancePeriod, error) {
	now := a.Now
	if now == nil {
		now = time.Now
	}
	s, _, err := loadPeriodSettings(ctx, q)
	if err != nil {
		return nil, err
	}
	return materializePeriod(ctx, q, s, year, month, domain.TodayWIB(now()))
}

/* ── Use cases ───────────────────────────────────────────────────────── */

// manageRoles may adjust attendance, close periods and change the period
// settings (the roles that may already delete attendance).
var manageRoles = map[string]bool{"super_admin": true, "admin": true, "hrd": true}

func canManageAttendance(a *Actor) bool { return manageRoles[a.Role] }

// periodJSON renders a period for the API.
func periodJSON(p *AttendancePeriod, today string) *Row {
	var summary any
	if len(p.CloseSummary) > 0 {
		summary = p.CloseSummary
	}
	status := p.Status
	days := domain.DaysUntil(today, p.Closing)
	return obj("id", p.ID, "period_year", p.Year, "period_month", p.Month, "name", p.Name(), "label", p.Label(),
		"start_date", p.Start, "end_date", p.End, "closing_date", p.Closing, "closing_date_custom", p.ClosingCustom,
		"status", status, "is_current", p.Start <= today && today <= p.End, "has_ended", today > p.End,
		"days_until_closing", days,
		"closed_at", jsTime(p.ClosedAt), "closed_by_name", p.ClosedByName, "close_mode", p.CloseMode,
		"close_summary", summary, "reopened_at", jsTime(p.ReopenedAt), "reopened_by_name", p.ReopenedByName,
		"reopen_reason", p.ReopenReason, "reopen_count", p.ReopenCount)
}

func jsTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return httpx.JSTime(*t)
}

// periodsOfYear materializes the periods of year that have started by
// today, newest first.
func (s *Service) periodsOfYear(ctx context.Context, year int, today string) ([]*AttendancePeriod, error) {
	q := s.repo.querier()
	settings, _, err := loadPeriodSettings(ctx, q)
	if err != nil {
		return nil, err
	}
	var out []*AttendancePeriod
	for month := 12; month >= 1; month-- {
		existing, err := periodByMonth(ctx, q, year, month)
		if err != nil {
			return nil, err
		}
		start := ""
		if existing != nil {
			start = existing.Start
		} else {
			r, err := resolveRange(ctx, q, settings, year, month)
			if err != nil {
				return nil, err
			}
			start = r.Start
		}
		if start > today {
			continue
		}
		p, err := materializePeriod(ctx, q, settings, year, month, today)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// PeriodStats are the attendance counts of a period.
func (s *Service) periodStats(ctx context.Context, p *AttendancePeriod) (json.RawMessage, error) {
	var raw []byte
	err := s.repo.querier().QueryRow(ctx, periodSummarySQL, p.Start, p.End).Scan(&raw)
	return raw, err
}

func (s *Service) periodEvents(ctx context.Context, ids []string) (map[string][]*Row, error) {
	rows, err := queryRows(ctx, s.repo.querier(), `SELECT id, period_id::text, action, actor_name, reason, details, created_at
		FROM hris.attendance_period_events WHERE period_id = ANY($1::uuid[]) ORDER BY created_at DESC`, nonNil(ids))
	if err != nil {
		return nil, err
	}
	out := map[string][]*Row{}
	for _, r := range rows {
		out[r.Str("period_id")] = append(out[r.Str("period_id")], r)
	}
	return out, nil
}

// blockingRuns are the payroll runs of a month that are past draft
// (processing, completed/approved, paid): their slips are frozen.
func (s *Service) blockingRuns(ctx context.Context, year, month int) ([]PeriodRun, error) {
	if s.payroll == nil {
		return nil, nil
	}
	runs, err := s.payroll.PeriodRuns(ctx, s.repo.querier(), year)
	if err != nil {
		return nil, err
	}
	var out []PeriodRun
	for _, r := range runs {
		if r.Month == month && r.Status != "draft" {
			out = append(out, r)
		}
	}
	return out, nil
}

var runStatusLabel = map[string]string{"draft": "draft", "processing": "diproses", "completed": "disetujui", "paid": "dibayar"}

// ListPeriods is GET /api/hris/attendance-periods.
func (s *Service) ListPeriods(ctx context.Context, a *Actor, year int, canReopen bool) (*Row, error) {
	today := domain.TodayWIB(s.now())
	q := s.repo.querier()
	settings, settingsRow, err := loadPeriodSettings(ctx, q)
	if err != nil {
		return nil, err
	}
	current, err := periodContaining(ctx, q, today, today)
	if err != nil {
		return nil, err
	}
	cy, cm := current.Year, current.Month
	if year == 0 {
		year = cy
	}
	periods, err := s.periodsOfYear(ctx, year, today)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(periods))
	for i, p := range periods {
		ids[i] = p.ID
	}
	events, err := s.periodEvents(ctx, ids)
	if err != nil {
		return nil, err
	}
	var runs []PeriodRun
	if s.payroll != nil {
		if runs, err = s.payroll.PeriodRuns(ctx, q, year); err != nil {
			return nil, err
		}
	}
	list := make([]*Row, 0, len(periods))
	for _, p := range periods {
		row := periodJSON(p, today)
		stats, err := s.periodStats(ctx, p)
		if err != nil {
			return nil, err
		}
		row.Set("stats", stats)
		var periodRuns []*Row
		for _, r := range runs {
			if r.Month == p.Month {
				periodRuns = append(periodRuns, obj("id", r.ID, "run_name", r.Name, "status", r.Status))
			}
		}
		row.Set("payroll_runs", nonNilRows(periodRuns))
		row.Set("events", nonNilRows(events[p.ID]))
		list = append(list, row)
	}
	return obj("settings", settingsJSON(settings, settingsRow), "current", obj("period_year", cy, "period_month", cm, "id", current.ID),
		"year", year, "today", today, "periods", list,
		"permissions", obj("can_manage", canManageAttendance(a), "can_reopen", canReopen)), nil
}

func nonNilRows(rows []*Row) []*Row {
	if rows == nil {
		return []*Row{}
	}
	return rows
}

func settingsJSON(s domain.PeriodSettings, row *Row) *Row {
	return obj("cutoff_day", s.CutoffDay, "closing_offset_days", s.ClosingOffsetDays,
		"updated_at", row.Get("updated_at"), "updated_by_name", row.Get("updated_by_name"))
}

// Period is GET /api/hris/attendance-periods/{id}: the period with its live
// attendance counts, events and payroll runs.
func (s *Service) Period(ctx context.Context, id string) (*Row, error) {
	today := domain.TodayWIB(s.now())
	q := s.repo.querier()
	p, err := periodByID(ctx, q, id)
	if err != nil || p == nil {
		return nil, errOr(err, httpx.NotFound("Periode absensi tidak ditemukan"))
	}
	if p.Status == periodOpen && domain.PeriodLocked(p.Closing, today) {
		if p, err = autoClosePeriod(ctx, q, p, today); err != nil {
			return nil, err
		}
	}
	row := periodJSON(p, today)
	stats, err := s.periodStats(ctx, p)
	if err != nil {
		return nil, err
	}
	row.Set("stats", stats)
	events, err := s.periodEvents(ctx, []string{p.ID})
	if err != nil {
		return nil, err
	}
	row.Set("events", nonNilRows(events[p.ID]))
	blocking, err := s.blockingRuns(ctx, p.Year, p.Month)
	if err != nil {
		return nil, err
	}
	runs := make([]*Row, len(blocking))
	for i, r := range blocking {
		runs[i] = obj("id", r.ID, "run_name", r.Name, "status", r.Status)
	}
	row.Set("blocking_payroll_runs", runs)
	return row, nil
}

func actorName(a *Actor) string {
	if strings.TrimSpace(a.FullName) != "" {
		return a.FullName
	}
	return "HRD"
}

func optionalUser(a *Actor) *string {
	if domain.IsUUID(a.UserID) {
		return &a.UserID
	}
	return nil
}

// ClosePeriod is POST /api/hris/attendance-periods/{id}/close: HR closes an
// ended period before its closing date.
func (s *Service) ClosePeriod(ctx context.Context, a *Actor, id string, meta audit.Entry) (*Row, error) {
	if !canManageAttendance(a) {
		return nil, httpx.Forbidden("Hanya HRD/admin yang bisa melakukan closing periode absensi")
	}
	today := domain.TodayWIB(s.now())
	err := s.repo.InTx(ctx, func(r Repository) error {
		q := r.querier()
		p, err := lockPeriod(ctx, q, id)
		if err != nil {
			return err
		}
		if p.Closed() {
			return httpx.Conflict("Periode absensi " + p.Label() + " sudah di-closing")
		}
		if today <= p.End {
			return httpx.BadRequest("Periode absensi " + p.Label() + " belum berakhir — closing baru bisa dilakukan setelah " +
				domain.ShortDateID(p.End))
		}
		var summary []byte
		if err := q.QueryRow(ctx, periodSummarySQL, p.Start, p.End).Scan(&summary); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `UPDATE hris.attendance_periods SET status = 'closed', range_frozen = true,
			closed_at = now(), closed_by = $2, closed_by_name = $3, close_mode = 'manual', close_summary = $4::jsonb
			WHERE id = $1`, p.ID, optionalUser(a), actorName(a), string(summary)); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `INSERT INTO hris.attendance_period_events (period_id, action, actor_id, actor_name, created_at, details)
			VALUES ($1, 'close', $2, $3, clock_timestamp(), jsonb_build_object('summary', $4::jsonb, 'closing_date', $5::date))`,
			p.ID, optionalUser(a), actorName(a), string(summary), p.Closing); err != nil {
			return err
		}
		meta.ActorID, meta.ActorName = a.UserID, ptr(actorName(a))
		meta.Action, meta.Entity, meta.EntityID = "attendance_period.close", "hris.attendance_periods", &p.ID
		meta.EntityLabel = ptr(p.Name() + " (" + p.Label() + ")")
		meta.Before = map[string]any{"status": "open", "closing_date": p.Closing}
		meta.After = map[string]any{"status": "closed", "close_mode": "manual", "summary": json.RawMessage(summary)}
		return audit.Write(ctx, q, meta)
	})
	if err != nil {
		return nil, err
	}
	return s.Period(ctx, id)
}

// lockPeriod reads a period FOR UPDATE.
func lockPeriod(ctx context.Context, q database.Querier, id string) (*AttendancePeriod, error) {
	if !domain.IsUUID(id) {
		return nil, httpx.NotFound("Periode absensi tidak ditemukan")
	}
	p, err := scanPeriod(q.QueryRow(ctx, `SELECT `+periodColumns+` FROM hris.attendance_periods WHERE id = $1 FOR UPDATE`, id))
	if err != nil || p == nil {
		return nil, errOr(err, httpx.NotFound("Periode absensi tidak ditemukan"))
	}
	return p, nil
}

func ptr[T any](v T) *T { return &v }

// ReopenPeriod is POST /api/hris/attendance-periods/{id}/reopen: a super
// admin / HR manager reopens a closed period with a reason until a new
// closing date. Not allowed once a payroll run of the month left draft.
func (s *Service) ReopenPeriod(ctx context.Context, a *Actor, id, reason, closing string, meta audit.Entry) (*Row, error) {
	reason = strings.TrimSpace(reason)
	if len([]rune(reason)) < 5 {
		return nil, httpx.BadRequest("Alasan buka ulang wajib diisi (minimal 5 karakter)")
	}
	today := domain.TodayWIB(s.now())
	err := s.repo.InTx(ctx, func(r Repository) error {
		q := r.querier()
		p, err := lockPeriod(ctx, q, id)
		if err != nil {
			return err
		}
		if !p.Closed() {
			return httpx.Conflict("Periode absensi " + p.Label() + " masih terbuka")
		}
		blocking, err := s.blockingRuns(ctx, p.Year, p.Month)
		if err != nil {
			return err
		}
		if len(blocking) > 0 {
			run := blocking[0]
			return httpx.Conflict(fmt.Sprintf("Periode tidak bisa dibuka ulang: payroll \"%s\" sudah %s. "+
				"Payroll yang sudah diproses/disetujui/dibayar harus tetap memakai absensi yang sudah di-closing.",
				run.Name, runStatusLabel[run.Status]))
		}
		settings, _, err := loadPeriodSettings(ctx, q)
		if err != nil {
			return err
		}
		if closing == "" {
			closing = domain.AddDaysISO(today, max(1, settings.ClosingOffsetDays))
		}
		if _, ok := domain.ParseDate(closing); !ok {
			return httpx.BadRequest("Tanggal closing baru tidak valid")
		}
		if closing < today || closing < p.End {
			return httpx.BadRequest("Tanggal closing baru minimal hari ini dan tidak sebelum akhir periode")
		}
		if _, err := q.Exec(ctx, `UPDATE hris.attendance_periods SET status = 'open', closing_date = $2, closing_date_custom = true,
			reopened_at = now(), reopened_by = $3, reopened_by_name = $4, reopen_reason = $5, reopen_count = reopen_count + 1
			WHERE id = $1`, p.ID, closing, optionalUser(a), actorName(a), reason); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `INSERT INTO hris.attendance_period_events (period_id, action, actor_id, actor_name, reason, created_at, details)
			VALUES ($1, 'reopen', $2, $3, $4, clock_timestamp(), jsonb_build_object('closing_date', $5::date, 'previous_closing_date', $6::date,
			  'previous_closed_by', $7::text, 'previous_close_mode', $8::text))`,
			p.ID, optionalUser(a), actorName(a), reason, closing, p.Closing, p.ClosedByName, p.CloseMode); err != nil {
			return err
		}
		meta.ActorID, meta.ActorName = a.UserID, ptr(actorName(a))
		meta.Action, meta.Entity, meta.EntityID = "attendance_period.reopen", "hris.attendance_periods", &p.ID
		meta.EntityLabel = ptr(p.Name() + " (" + p.Label() + ")")
		meta.Before = map[string]any{"status": "closed", "closing_date": p.Closing, "closed_by_name": p.ClosedByName}
		meta.After = map[string]any{"status": "open", "closing_date": closing}
		meta.Reason = &reason
		return audit.Write(ctx, q, meta)
	})
	if err != nil {
		return nil, err
	}
	return s.Period(ctx, id)
}

// SetClosingDate is PATCH /api/hris/attendance-periods/{id}: HR moves the
// closing date of an open period (not before today or the period end).
func (s *Service) SetClosingDate(ctx context.Context, a *Actor, id, closing string, reason *string, meta audit.Entry) (*Row, error) {
	if !canManageAttendance(a) {
		return nil, httpx.Forbidden("Hanya HRD/admin yang bisa mengubah tanggal closing")
	}
	if _, ok := domain.ParseDate(closing); !ok {
		return nil, httpx.BadRequest("Tanggal closing tidak valid")
	}
	today := domain.TodayWIB(s.now())
	err := s.repo.InTx(ctx, func(r Repository) error {
		q := r.querier()
		p, err := lockPeriod(ctx, q, id)
		if err != nil {
			return err
		}
		if p.Closed() || domain.PeriodLocked(p.Closing, today) {
			return httpx.Conflict("Periode absensi " + p.Label() + " sudah di-closing — gunakan Buka ulang")
		}
		if closing < today || closing < p.End {
			return httpx.BadRequest("Tanggal closing minimal hari ini dan tidak sebelum akhir periode (" + domain.ShortDateID(p.End) + ")")
		}
		if _, err := q.Exec(ctx, `UPDATE hris.attendance_periods SET closing_date = $2, closing_date_custom = true WHERE id = $1`,
			p.ID, closing); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `INSERT INTO hris.attendance_period_events (period_id, action, actor_id, actor_name, reason, created_at, details)
			VALUES ($1, 'closing_date', $2, $3, $4, clock_timestamp(), jsonb_build_object('from', $5::date, 'to', $6::date))`,
			p.ID, optionalUser(a), actorName(a), nilIfEmpty(reason), p.Closing, closing); err != nil {
			return err
		}
		meta.ActorID, meta.ActorName = a.UserID, ptr(actorName(a))
		meta.Action, meta.Entity, meta.EntityID = "attendance_period.closing_date", "hris.attendance_periods", &p.ID
		meta.EntityLabel = ptr(p.Name() + " (" + p.Label() + ")")
		meta.Before = map[string]any{"closing_date": p.Closing}
		meta.After = map[string]any{"closing_date": closing}
		meta.Reason = reason
		return audit.Write(ctx, q, meta)
	})
	if err != nil {
		return nil, err
	}
	return s.Period(ctx, id)
}

// PeriodSettings is GET /api/hris/attendance-settings with the current
// period under the stored settings.
func (s *Service) PeriodSettings(ctx context.Context) (*Row, error) {
	q := s.repo.querier()
	settings, row, err := loadPeriodSettings(ctx, q)
	if err != nil {
		return nil, err
	}
	today := domain.TodayWIB(s.now())
	current, err := periodContaining(ctx, q, today, today)
	if err != nil {
		return nil, err
	}
	return obj("settings", settingsJSON(settings, row), "current_period", periodJSON(current, today), "today", today), nil
}

// SavePeriodSettings is PUT /api/hris/attendance-settings: the never-closed
// open periods follow the new settings at once (one transaction, so the
// no-overlap constraint is checked on the final ranges); periods that were
// ever closed keep their dates.
func (s *Service) SavePeriodSettings(ctx context.Context, a *Actor, cutoff, offset int, meta audit.Entry) (*Row, error) {
	if !canManageAttendance(a) {
		return nil, httpx.Forbidden("Hanya HRD/admin yang bisa mengubah pengaturan periode absensi")
	}
	cut, ok := domain.NormalizeCutoff(cutoff)
	if !ok {
		return nil, httpx.BadRequest("Tanggal cut-off harus 1–28 atau akhir bulan")
	}
	if offset < 0 || offset > 31 {
		return nil, httpx.BadRequest("Jarak closing harus 0–31 hari setelah akhir periode")
	}
	err := s.repo.InTx(ctx, func(r Repository) error {
		q := r.querier()
		before, _, err := loadPeriodSettings(ctx, q)
		if err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `INSERT INTO hris.attendance_settings (id, cutoff_day, closing_offset_days, updated_at, updated_by, updated_by_name)
			VALUES (true, $1, $2, now(), $3, $4)
			ON CONFLICT (id) DO UPDATE SET cutoff_day = EXCLUDED.cutoff_day, closing_offset_days = EXCLUDED.closing_offset_days,
			  updated_at = now(), updated_by = EXCLUDED.updated_by, updated_by_name = EXCLUDED.updated_by_name`,
			cut, offset, optionalUser(a), actorName(a)); err != nil {
			return err
		}
		next := domain.PeriodSettings{CutoffDay: cut, ClosingOffsetDays: offset}
		rows, err := q.Query(ctx, `SELECT `+periodColumns+` FROM hris.attendance_periods
			WHERE status = 'open' AND NOT range_frozen ORDER BY period_year, period_month`)
		if err != nil {
			return err
		}
		open, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*AttendancePeriod, error) { return scanPeriod(row) })
		if err != nil {
			return err
		}
		for _, p := range open {
			rg, err := resolveRange(ctx, q, next, p.Year, p.Month)
			if err != nil {
				return err
			}
			closing := rg.Closing
			if p.ClosingCustom && p.Closing >= rg.End {
				closing = p.Closing
			}
			if _, err := q.Exec(ctx, `UPDATE hris.attendance_periods SET start_date = $2, end_date = $3, closing_date = $4
				WHERE id = $1`, p.ID, rg.Start, rg.End, closing); err != nil {
				return err
			}
		}
		meta.ActorID, meta.ActorName = a.UserID, ptr(actorName(a))
		meta.Action, meta.Entity = "attendance_settings.update", "hris.attendance_settings"
		meta.EntityLabel = ptr("Pengaturan periode absensi")
		meta.Before = map[string]any{"cutoff_day": before.CutoffDay, "closing_offset_days": before.ClosingOffsetDays}
		meta.After = map[string]any{"cutoff_day": cut, "closing_offset_days": offset}
		return audit.Write(ctx, q, meta)
	})
	if err != nil {
		if database.PgCode(err) == "23P01" {
			return nil, httpx.Conflict("Pengaturan ini membuat periode absensi saling tumpang tindih dengan periode yang sudah di-closing")
		}
		return nil, err
	}
	return s.PeriodSettings(ctx)
}

// PreviewPeriodSettings resolves the current and next period under proposed
// settings without saving: periods that were ever closed keep their dates
// and the ones around them connect to them, exactly as saving would.
func (s *Service) PreviewPeriodSettings(ctx context.Context, cutoff, offset int) (*Row, error) {
	cut, ok := domain.NormalizeCutoff(cutoff)
	if !ok || offset < 0 || offset > 31 {
		return nil, httpx.BadRequest("Tanggal cut-off harus 1–28 atau akhir bulan, jarak closing 0–31 hari")
	}
	q := s.repo.querier()
	settings := domain.PeriodSettings{CutoffDay: cut, ClosingOffsetDays: offset}
	today := domain.TodayWIB(s.now())
	rangeOf := func(y, m int) (*Row, error) {
		var frozen AttendancePeriod
		err := q.QueryRow(ctx, `SELECT start_date::text, end_date::text, closing_date::text, status FROM hris.attendance_periods
			WHERE period_year = $1 AND period_month = $2 AND range_frozen`, y, m).
			Scan(&frozen.Start, &frozen.End, &frozen.Closing, &frozen.Status)
		if err == nil {
			return obj("period_year", y, "period_month", m, "name", fmt.Sprintf("%s %d", domain.MonthNameID(m), y),
				"start_date", frozen.Start, "end_date", frozen.End, "label", domain.PeriodRangeLabel(frozen.Start, frozen.End),
				"closing_date", frozen.Closing, "status", frozen.Status, "frozen", true), nil
		}
		if !database.IsNoRows(err) {
			return nil, err
		}
		r, err := resolveRange(ctx, q, settings, y, m)
		if err != nil {
			return nil, err
		}
		return obj("period_year", y, "period_month", m, "name", fmt.Sprintf("%s %d", domain.MonthNameID(m), y),
			"start_date", r.Start, "end_date", r.End, "label", domain.PeriodRangeLabel(r.Start, r.End),
			"closing_date", r.Closing, "status", periodOpen, "frozen", false), nil
	}
	y, m := domain.PeriodKeyFor(settings, today)
	var current *Row
	for range 4 {
		row, err := rangeOf(y, m)
		if err != nil {
			return nil, err
		}
		switch {
		case today < row.Str("start_date"):
			y, m = domain.ShiftMonth(y, m, -1)
		case today > row.Str("end_date"):
			y, m = domain.ShiftMonth(y, m, 1)
		default:
			current = row
		}
		if current != nil {
			break
		}
	}
	if current == nil {
		return nil, fmt.Errorf("no preview period contains %s", today)
	}
	ny, nm := domain.ShiftMonth(y, m, 1)
	next, err := rangeOf(ny, nm)
	if err != nil {
		return nil, err
	}
	return obj("cutoff_day", cut, "closing_offset_days", offset, "current", current, "next", next), nil
}

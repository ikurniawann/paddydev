package hris

import (
	"context"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
)

// Ketentuan Lembur TnC 03-11-2026 (EPIC-085): level, jam mulai, batas jam.

// overtimeLevelCheck is R1: no overtime from Supervisor up.
func (s *Service) overtimeLevelCheck(ctx context.Context, employeeID string) error {
	level, _, err := s.repo.EmployeePositionLevel(ctx, employeeID)
	if err != nil {
		return err
	}
	if domain.IsLeaderLevel(level) {
		return httpx.BadRequest(domain.OvertimeLeaderRefused)
	}
	return nil
}

// overtimeStartCheck is R2: on a working day overtime starts after the
// employee's last shift of that date ends (not on a business-trip day).
func (s *Service) overtimeStartCheck(ctx context.Context, r Repository, employeeID, date, start, dayType string) error {
	if dayType != domain.OvertimeWeekday {
		return nil
	}
	trip, err := r.OnApprovedTrip(ctx, employeeID, date)
	if err != nil {
		return err
	}
	shift, err := r.OvertimeShiftEndOn(ctx, employeeID, date)
	if err != nil {
		return err
	}
	if msg := domain.OvertimeStartError(start, dayType, shift, trip); msg != "" {
		return httpx.BadRequest(msg)
	}
	return nil
}

// checkOvertimeCaps is R3 on r (inside the caller's transaction): the
// employee row is locked so two requests cannot both pass, then the
// Monday–Sunday week and attendance period totals (pending + approved,
// without excludeID) plus hours must stay within 18 / 72 (EPIC-089: no daily cap).
func (s *Service) checkOvertimeCaps(ctx context.Context, r Repository, employeeID, date string, hours float64, excludeID string) error {
	if err := r.LockLeaveEmployee(ctx, employeeID); err != nil {
		return err
	}
	mon, sun, _ := domain.WeekBounds(date)
	week, err := r.OvertimeHoursUsed(ctx, employeeID, mon, sun, excludeID)
	if err != nil {
		return err
	}
	p, err := periodContaining(ctx, r.querier(), date, domain.TodayWIB(s.now()))
	if err != nil {
		return err
	}
	period, err := r.OvertimeHoursUsed(ctx, employeeID, p.Start, p.End, excludeID)
	if err != nil {
		return err
	}
	if msg := domain.OvertimeCapError(hours, week, period, mon, sun, p.Label()); msg != "" {
		return httpx.BadRequest(msg)
	}
	return nil
}

/* ── Storage ─────────────────────────────────────────────────────────── */

// OvertimeShiftEndOn is the end of the employee's last shift on date (the
// second shift on a two-shift day); nil without a schedule or on a day off.
func (s *store) OvertimeShiftEndOn(ctx context.Context, employeeID, date string) (*domain.OvertimeShiftEnd, error) {
	var end *string
	var overnight *bool
	err := s.db.QueryRow(ctx, `SELECT to_char(coalesce(s2.end_time, s.end_time), 'HH24:MI'),
		       CASE WHEN s2.id IS NOT NULL THEN s2.is_overnight ELSE s.is_overnight END
		FROM (SELECT es.shift_id, es.shift2_id FROM hris.v_employee_schedule es
		      WHERE es.employee_id = $1 AND es.day_of_week = extract(isodow FROM $2::date)::int
		        AND es.effective_from <= $2::date AND (es.effective_to IS NULL OR es.effective_to >= $2::date)
		      ORDER BY es.effective_from DESC, es.priority LIMIT 1) sc
		JOIN hris.shifts s ON s.id = sc.shift_id
		LEFT JOIN hris.shifts s2 ON s2.id = sc.shift2_id`, employeeID, date).Scan(&end, &overnight)
	if database.IsNoRows(err) || (err == nil && end == nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &domain.OvertimeShiftEnd{End: *end, Overnight: overnight != nil && *overnight}, nil
}

// OvertimeHoursUsed sums the paid hours of the employee's pending/approved
// requests dated [from, to], without excludeID ("" = none).
func (s *store) OvertimeHoursUsed(ctx context.Context, employeeID, from, to, excludeID string) (float64, error) {
	var used float64
	err := s.db.QueryRow(ctx, `SELECT coalesce(sum(hours), 0)::float8 FROM hris.overtime_requests
		WHERE employee_id = $1 AND status IN ('pending', 'approved')
		  AND date BETWEEN $2::date AND $3::date AND ($4 = '' OR id <> $4::uuid)`, employeeID, from, to, excludeID).Scan(&used)
	return used, err
}

// OnApprovedTrip: an approved business trip of the employee covers date.
func (s *store) OnApprovedTrip(ctx context.Context, employeeID, date string) (bool, error) {
	var on bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM hris.business_trips
		WHERE employee_id = $1 AND status = 'approved' AND $2::date BETWEEN start_date AND end_date)`,
		employeeID, date).Scan(&on)
	return on, ignoreBadInput(err)
}

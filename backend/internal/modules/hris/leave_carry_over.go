package hris

import (
	"context"
	"fmt"
	"math"
	"time"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
)

// Job sisa bawaan 1 Januari (EPIC-068 R9).

// LeaveCarryOverEvery is how often the background loop checks whether the
// year's carry-over still has to run.
const LeaveCarryOverEvery = 15 * time.Minute

// carryOverResult is one run that processed a year.
type carryOverResult struct {
	Year      int
	Employees int // active employees whose row for Year exists after the run
	Carried   int // employees who got a carry-over
}

// carryAmount is min(remaining, max) rounded down to a multiple of 0,5; a nil
// max carries the whole remainder.
func carryAmount(remaining float64, max *float64) float64 {
	v := remaining
	if max != nil && *max < v {
		v = *max
	}
	if v <= 0 {
		return 0
	}
	return math.Floor(v*2) / 2
}

// RunLeaveCarryOver does the carry-over of the WIB year of now, once per
// year: from 00:00 WIB on 1 January the first run claims the year, makes sure
// every active employee has a row for it (R1) and, when the setting is on,
// carries the previous year's remainder into it. Later runs, restarts and
// other replicas find the claim and do nothing. The claim, the rows, the
// audit and the summary commit together, so a failure releases the claim.
func (s *Service) RunLeaveCarryOver(ctx context.Context, now time.Time) error {
	_, err := s.runLeaveCarryOver(ctx, now)
	return err
}

type carryCandidate struct {
	employeeID, name string
	prevRemaining    float64
	row              balanceRaw
}

func (s *Service) runLeaveCarryOver(ctx context.Context, now time.Time) (*carryOverResult, error) {
	year := domain.NowWIB(now).Year()
	if year < minBalanceYear+1 {
		return nil, nil
	}
	var result *carryOverResult
	err := s.repo.InTx(ctx, func(r Repository) error {
		result = nil
		q := r.querier()
		tag, err := q.Exec(ctx, `INSERT INTO hris.leave_carry_over_runs (year) VALUES ($1) ON CONFLICT (year) DO NOTHING`, year)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		settings, err := loadLeaveSettings(ctx, q, false)
		if err != nil {
			return err
		}

		rows, err := q.Query(ctx, `SELECT id::text FROM hris.employees WHERE is_active ORDER BY id`)
		if err != nil {
			return err
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, id := range ids {
			if err := ensureLeaveBalance(ctx, q, id, year); err != nil {
				return err
			}
		}
		res := &carryOverResult{Year: year, Employees: len(ids)}

		if settings.CarryOverEnabled {
			// A row HR already filled (carry-over or expiry set) is left alone.
			rows, err := q.Query(ctx, `SELECT e.id::text, e.full_name, p.annual_leave_remaining::float8,
					c.annual_leave_total::float8, c.annual_leave_adjustment::float8, c.annual_leave_used::float8
				FROM hris.employees e
				JOIN hris.leave_balances p ON p.employee_id = e.id AND p.year = $1 - 1
				JOIN hris.leave_balances c ON c.employee_id = e.id AND c.year = $1
				WHERE e.is_active AND p.annual_leave_remaining > 0
				  AND c.carry_over_days = 0 AND c.carry_over_expires_on IS NULL
				ORDER BY e.id
				FOR UPDATE OF c`, year)
			if err != nil {
				return err
			}
			var cands []carryCandidate
			for rows.Next() {
				var c carryCandidate
				if err := rows.Scan(&c.employeeID, &c.name, &c.prevRemaining, &c.row.Total, &c.row.Adjustment, &c.row.Used); err != nil {
					rows.Close()
					return err
				}
				cands = append(cands, c)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
			expires := fmt.Sprintf("%04d-%02d-%02d", year, settings.CarryOverExpiryMonth, settings.CarryOverExpiryDay)
			for _, c := range cands {
				carry := carryAmount(c.prevRemaining, settings.CarryOverMaxDays)
				if carry <= 0 {
					continue
				}
				if _, err := q.Exec(ctx, `UPDATE hris.leave_balances
					SET carry_over_days = $3, carry_over_used = 0, carry_over_expires_on = $4::date
					WHERE employee_id = $1 AND year = $2`, c.employeeID, year, carry, expires); err != nil {
					return err
				}
				after := c.row
				after.CarryDays, after.CarryUsed, after.CarryExpires = carry, 0, &expires
				id := c.employeeID
				reason := fmt.Sprintf("Sisa bawaan otomatis 1 Januari %d: %s hari dari sisa jatah %d (%s hari), berlaku sampai %s",
					year, daysText(carry), year-1, daysText(c.prevRemaining), expires)
				if err := audit.Write(ctx, q, audit.Entry{
					ActorName: ptr(sistemName), Action: "leave_balance.carry_over", Entity: "hris.employees", EntityID: &id,
					EntityLabel: ptr(fmt.Sprintf("%s · Saldo cuti %d", c.name, year)),
					Before:      balanceAudit(year, c.row), After: balanceAudit(year, after), Reason: &reason,
				}); err != nil {
					return err
				}
				res.Carried++
			}
		}
		if _, err := q.Exec(ctx, `UPDATE hris.leave_carry_over_runs SET employees = $2, carried = $3, finished_at = now()
			WHERE year = $1`, year, res.Employees, res.Carried); err != nil {
			return err
		}
		result = res
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// LeaveCarryOverLoop runs RunLeaveCarryOver once at start and then every
// interval until ctx ends. Errors and panics are logged; the loop never stops
// because of them.
func (s *Service) LeaveCarryOverLoop(ctx context.Context, every time.Duration) {
	s.leaveCarryOverTick(ctx)
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.leaveCarryOverTick(ctx)
		}
	}
}

func (s *Service) leaveCarryOverTick(ctx context.Context) {
	defer func() {
		if p := recover(); p != nil {
			s.log.Error("hris leave carry-over panicked", "panic", fmt.Sprint(p))
		}
	}()
	res, err := s.runLeaveCarryOver(ctx, s.now())
	if err != nil {
		s.log.Error("hris leave carry-over failed", "error", err)
		return
	}
	if res != nil {
		s.log.Info("hris leave carry-over", "year", res.Year, "employees", res.Employees, "carried", res.Carried)
	}
}

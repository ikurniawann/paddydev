package hris

import (
	"context"
	"fmt"
	"math"
	"time"

	"nuhabit/backend/internal/modules/hris/domain"
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

		// Every auto row of the year is recalculated with the same formula the
		// live hooks use, so a rerun after the claim was deleted gives the same
		// result (and no audit for an unchanged value). A row HR filled by hand
		// is left alone; an inactive employee has no row here.
		if settings.CarryOverEnabled && settings.CarryOverReady {
			for _, id := range ids {
				r, err := recomputeCarryOverWith(ctx, q, settings, id, year, carryCause{Type: causeJob}, nil)
				if err != nil {
					return err
				}
				if r.Changed && r.Carry > 0 {
					res.Carried++
				}
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

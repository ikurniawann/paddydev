package app

import (
	"context"
	"math"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/modules/payroll"
	"nuhabit/backend/internal/modules/payroll/domain"
	"nuhabit/backend/internal/platform/database"
)

// payrollKPISQL reads what the KPI snapshot measures in the HRIS people,
// POS, procurement and identity contexts with the reporting SQL of
// lib/kpi/collectors.ts and collectors-wave2.ts (read-only joins).
type payrollKPISQL struct{}

var _ payroll.KPISources = payrollKPISQL{}

// keyed runs sql and collects each row under the key scan returns.
func keyed[T any](ctx context.Context, q database.Querier, sql string, scan func(pgx.CollectableRow) (string, T, error), args ...any) (map[string]T, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	out := map[string]T{}
	_, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (struct{}, error) {
		k, v, err := scan(r)
		out[k] = v
		return struct{}{}, err
	})
	return out, err
}

// grouped is keyed for many rows per key.
func grouped[T any](ctx context.Context, q database.Querier, sql string, scan func(pgx.CollectableRow) (string, T, error), args ...any) (map[string][]T, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	out := map[string][]T{}
	_, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (struct{}, error) {
		k, v, err := scan(r)
		out[k] = append(out[k], v)
		return struct{}{}, err
	})
	return out, err
}

func (payrollKPISQL) Employees(ctx context.Context, q database.Querier) ([]domain.KPIEmployee, error) {
	rows, err := q.Query(ctx, `SELECT e.id::text, e.user_id::text, e.department_id::text,
		COALESCE(u.role::text, 'employee') AS role_code
		FROM hris.employees e
		LEFT JOIN configuration.users u ON u.id = e.user_id
		WHERE e.is_active = true`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.KPIEmployee, error) {
		var e domain.KPIEmployee
		return e, r.Scan(&e.ID, &e.UserID, &e.DepartmentID, &e.RoleCode)
	})
}

func (payrollKPISQL) ShiftPatterns(ctx context.Context, q database.Querier, start, end string) (map[string][]domain.ShiftRow, error) {
	return grouped(ctx, q, `SELECT employee_id::text, day_of_week, shift_id::text, effective_from::text, effective_to::text, priority
		FROM hris.v_employee_schedule
		WHERE effective_from <= $2 AND (effective_to IS NULL OR effective_to >= $1)`,
		func(r pgx.CollectableRow) (string, domain.ShiftRow, error) {
			var id string
			var s domain.ShiftRow
			return id, s, r.Scan(&id, &s.DayOfWeek, &s.ShiftID, &s.EffectiveFrom, &s.EffectiveTo, &s.Priority)
		}, start, end)
}

func (payrollKPISQL) PresentDays(ctx context.Context, q database.Querier, start, end string) (map[string][]domain.PresentDay, error) {
	return grouped(ctx, q, `SELECT employee_id::text, date::text, is_late
		FROM hris.attendance
		WHERE date BETWEEN $1 AND $2 AND status = 'present'`,
		func(r pgx.CollectableRow) (string, domain.PresentDay, error) {
			var id string
			var p domain.PresentDay
			return id, p, r.Scan(&id, &p.Date, &p.IsLate)
		}, start, end)
}

func (payrollKPISQL) ApprovedLeaves(ctx context.Context, q database.Querier, start, end string) (map[string][]domain.LeaveRange, error) {
	return grouped(ctx, q, `SELECT employee_id::text, start_date::text, end_date::text
		FROM hris.leaves
		WHERE status = 'approved' AND start_date <= $2 AND end_date >= $1`,
		func(r pgx.CollectableRow) (string, domain.LeaveRange, error) {
			var id string
			var l domain.LeaveRange
			return id, l, r.Scan(&id, &l.Start, &l.End)
		}, start, end)
}

func (payrollKPISQL) LateMinutes(ctx context.Context, q database.Querier, start, end string) (map[string]float64, error) {
	return keyed(ctx, q, `SELECT employee_id::text, SUM(COALESCE(late_minutes, 0))::float AS late_minutes
		FROM hris.attendance
		WHERE date BETWEEN $1 AND $2 AND status = 'present'
		GROUP BY employee_id`,
		func(r pgx.CollectableRow) (string, float64, error) {
			var id string
			var v float64
			return id, v, r.Scan(&id, &v)
		}, start, end)
}

func (payrollKPISQL) Shifts(ctx context.Context, q database.Querier) (map[string]domain.ShiftDuration, error) {
	return keyed(ctx, q, `SELECT id::text, start_time::text, end_time::text, break_minutes, is_overnight FROM hris.shifts`,
		func(r pgx.CollectableRow) (string, domain.ShiftDuration, error) {
			var id string
			var s domain.ShiftDuration
			return id, s, r.Scan(&id, &s.StartTime, &s.EndTime, &s.BreakMinutes, &s.IsOvernight)
		})
}

func (payrollKPISQL) LeaveDiscipline(ctx context.Context, q database.Querier, start, end string) (map[string]domain.CollectorValue, error) {
	return keyed(ctx, q, `SELECT employee_id::text,
		COUNT(*) FILTER (WHERE created_at::date <= start_date)::int AS ontime_requests,
		COUNT(*)::int AS total_requests
		FROM hris.leaves
		WHERE start_date BETWEEN $1 AND $2 AND status <> 'cancelled'
		GROUP BY employee_id`,
		func(r pgx.CollectableRow) (string, domain.CollectorValue, error) {
			var id string
			var ontime, total int
			err := r.Scan(&id, &ontime, &total)
			return id, domain.CollectorValue{Actual: domain.SafeRatio(float64(ontime), float64(total)), SampleSize: float64(total),
				Detail: map[string]any{"ontime_requests": ontime, "total_requests": total}}, err
		}, start, end)
}

func (payrollKPISQL) LeaveSLA(ctx context.Context, q database.Querier, start, end string) (*domain.CollectorValue, error) {
	var avg *float64
	var decided int
	err := q.QueryRow(ctx, `SELECT AVG(EXTRACT(EPOCH FROM (approved_at - created_at)) / 86400)::float AS avg_days,
		COUNT(*)::int AS decided
		FROM hris.leaves
		WHERE approved_at IS NOT NULL
		  AND approved_at >= $1::date AND approved_at < ($2::date + 1)`, start, end).Scan(&avg, &decided)
	if err != nil || decided == 0 {
		return nil, err
	}
	return &domain.CollectorValue{Actual: avg, SampleSize: float64(decided),
		Detail: map[string]any{"avg_days": avg, "decided": decided}}, nil
}

func (payrollKPISQL) Logbooks(ctx context.Context, q database.Querier, start, end string) (map[string]payroll.LogbookCount, error) {
	return keyed(ctx, q, `SELECT COALESCE(en.department_id::text, ''),
		COUNT(*) FILTER (WHERE it.is_checked)::int AS checked_items,
		COUNT(*)::int AS total_items
		FROM hris.hris_logbook_entries en
		JOIN hris.hris_logbook_entry_items it ON it.entry_id = en.id
		WHERE en.entry_date BETWEEN $1 AND $2
		GROUP BY en.department_id`,
		func(r pgx.CollectableRow) (string, payroll.LogbookCount, error) {
			var dept string
			var c payroll.LogbookCount
			return dept, c, r.Scan(&dept, &c.Checked, &c.Total)
		}, start, end)
}

// CashierShifts runs the pos_cash_variance and pos_sales_shift queries as
// one: both group the closed shifts of the period by cashier.
func (payrollKPISQL) CashierShifts(ctx context.Context, q database.Querier, start, end string) (variance, sales map[string]domain.CollectorValue, err error) {
	rows, err := q.Query(ctx, `SELECT e.id::text AS employee_id,
		SUM(ABS(COALESCE(s.variance, 0)))::float AS sum_abs_variance,
		SUM(COALESCE(s.expected_cash, 0))::float AS sum_expected,
		AVG(COALESCE(s.total_sales, 0))::float AS avg_sales,
		COUNT(*)::int AS shift_count
		FROM pos.pos_shifts s
		JOIN hris.employees e ON e.user_id = s.cashier_id
		WHERE s.closed_at IS NOT NULL
		  AND s.closed_at >= $1::date
		  AND s.closed_at < ($2::date + 1)
		GROUP BY e.id`, start, end)
	if err != nil {
		return nil, nil, err
	}
	variance, sales = map[string]domain.CollectorValue{}, map[string]domain.CollectorValue{}
	_, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (struct{}, error) {
		var id string
		var absVariance, expected, avgSales float64
		var count int
		if err := r.Scan(&id, &absVariance, &expected, &avgSales, &count); err != nil {
			return struct{}{}, err
		}
		variance[id] = domain.CollectorValue{Actual: domain.SafeRatio(absVariance, expected), SampleSize: float64(count),
			Detail: map[string]any{"sum_abs_variance": absVariance, "sum_expected": expected, "shift_count": count}}
		sales[id] = domain.CollectorValue{Actual: &avgSales, SampleSize: float64(count),
			Detail: map[string]any{"avg_sales": avgSales, "shift_count": count}}
		return struct{}{}, nil
	})
	return variance, sales, err
}

func (payrollKPISQL) POFulfillment(ctx context.Context, q database.Querier, start, end string) (map[string]domain.CollectorValue, error) {
	return keyed(ctx, q, `SELECT e.id::text AS employee_id,
		SUM(COALESCE(poi.qty_received, 0))::float AS sum_received,
		SUM(COALESCE(poi.qty_ordered, 0))::float AS sum_ordered,
		COUNT(DISTINCT po.id)::int AS po_count
		FROM purchasing.purchase_orders po
		JOIN purchasing.purchase_order_items poi ON poi.purchase_order_id = po.id
		JOIN hris.employees e ON e.user_id = po.created_by
		WHERE po.tanggal_po BETWEEN $1 AND $2
		  AND po.status NOT IN ('draft', 'cancelled')
		GROUP BY e.id`,
		func(r pgx.CollectableRow) (string, domain.CollectorValue, error) {
			var id string
			var received, ordered float64
			var count int
			err := r.Scan(&id, &received, &ordered, &count)
			return id, domain.CollectorValue{Actual: domain.SafeRatio(received, ordered), SampleSize: float64(count),
				Detail: map[string]any{"sum_received": received, "sum_ordered": ordered, "po_count": count}}, err
		}, start, end)
}

// settledTerms dates each active payment term's real settlement: the first
// posted payment that brings the running total to the term amount (later
// corrections do not move it).
const settledTerms = `
  cum_pay AS (
    SELECT t.id AS term_id, t.due_date, t.amount, t.created_at,
           vp.payment_date,
           SUM(vp.amount) OVER (PARTITION BY t.id ORDER BY vp.payment_date, vp.id) AS cum
    FROM purchasing.purchase_order_payment_terms t
    JOIN purchasing.vendor_payments vp ON vp.payment_term_id = t.id AND vp.status = 'posted'
    WHERE t.is_active
  ),
  settled AS (
    SELECT term_id, due_date, amount, created_at, MIN(payment_date) AS settled_date
    FROM cum_pay
    WHERE cum >= amount
    GROUP BY term_id, due_date, amount, created_at
  )`

func (payrollKPISQL) VendorPayOntime(ctx context.Context, q database.Querier, start, end string) (*domain.CollectorValue, error) {
	var ontime, total int
	err := q.QueryRow(ctx, `WITH `+settledTerms+`
		SELECT COUNT(*) FILTER (WHERE s.settled_date IS NOT NULL AND s.settled_date <= t.due_date)::int AS ontime,
		       COUNT(*)::int AS total
		FROM purchasing.purchase_order_payment_terms t
		LEFT JOIN settled s ON s.term_id = t.id
		WHERE t.due_date BETWEEN $1 AND $2 AND t.is_active`, start, end).Scan(&ontime, &total)
	if err != nil || total == 0 {
		return nil, err
	}
	return &domain.CollectorValue{Actual: domain.SafeRatio(float64(ontime), float64(total)), SampleSize: float64(total),
		Detail: map[string]any{"ontime_terms": ontime, "total_terms": total}}, nil
}

func (payrollKPISQL) VendorPaySLA(ctx context.Context, q database.Querier, start, end string) (*domain.CollectorValue, error) {
	var avg *float64
	var settled int
	err := q.QueryRow(ctx, `WITH `+settledTerms+`
		SELECT AVG(EXTRACT(EPOCH FROM (settled_date::timestamp - created_at)) / 86400)::float AS avg_days,
		       COUNT(*)::int AS settled
		FROM settled
		WHERE settled_date BETWEEN $1 AND $2`, start, end).Scan(&avg, &settled)
	if err != nil || settled == 0 {
		return nil, err
	}
	actual := 0.0
	if avg != nil {
		actual = math.Max(0, *avg)
	}
	return &domain.CollectorValue{Actual: &actual, SampleSize: float64(settled),
		Detail: map[string]any{"avg_days": avg, "settled_terms": settled}}, nil
}

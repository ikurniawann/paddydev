package payroll

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/modules/payroll/domain"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/validate"
)

// Monthly KPI snapshot (lib/kpi/snapshot.ts runKpiSnapshot), idempotent:
// a re-run upserts snapshots and draft scorecards; final scorecards (and
// their snapshots) are left untouched and reported. performance.kpi_*,
// payroll runs and department tasks belong to this module; everything else
// the collectors measure comes through KPISources.

// KPISources reads the records of other contexts the collectors measure,
// with the SQL of lib/kpi/collectors*.ts. Maps are keyed by employee id
// (department id for logbooks); start and end are ISO dates.
type KPISources interface {
	// Employees are the active employees with their account's role.
	Employees(ctx context.Context, q database.Querier) ([]domain.KPIEmployee, error)
	// ShiftPatterns are the hris.employee_shifts rows overlapping the period.
	ShiftPatterns(ctx context.Context, q database.Querier, start, end string) (map[string][]domain.ShiftRow, error)
	// PresentDays are the present attendance rows of the period.
	PresentDays(ctx context.Context, q database.Querier, start, end string) (map[string][]domain.PresentDay, error)
	// ApprovedLeaves are the approved leaves overlapping the period.
	ApprovedLeaves(ctx context.Context, q database.Querier, start, end string) (map[string][]domain.LeaveRange, error)
	// LateMinutes sums late_minutes of the period's present rows.
	LateMinutes(ctx context.Context, q database.Querier, start, end string) (map[string]float64, error)
	// Shifts are every hris.shifts row's working times.
	Shifts(ctx context.Context, q database.Querier) (map[string]domain.ShiftDuration, error)
	// LeaveDiscipline is leave_request_discipline per employee.
	LeaveDiscipline(ctx context.Context, q database.Querier, start, end string) (map[string]domain.CollectorValue, error)
	// LeaveSLA is leave_sla, nil when no leave was decided in the period.
	LeaveSLA(ctx context.Context, q database.Querier, start, end string) (*domain.CollectorValue, error)
	// Logbooks is logbook_compliance per department.
	Logbooks(ctx context.Context, q database.Querier, start, end string) (map[string]LogbookCount, error)
	// CashierShifts are pos_cash_variance and pos_sales_shift per cashier.
	CashierShifts(ctx context.Context, q database.Querier, start, end string) (variance, sales map[string]domain.CollectorValue, err error)
	// POFulfillment is po_fulfillment per PO creator.
	POFulfillment(ctx context.Context, q database.Querier, start, end string) (map[string]domain.CollectorValue, error)
	// VendorPayOntime and VendorPaySLA are the payment-term indicators,
	// nil without terms in the period.
	VendorPayOntime(ctx context.Context, q database.Querier, start, end string) (*domain.CollectorValue, error)
	VendorPaySLA(ctx context.Context, q database.Querier, start, end string) (*domain.CollectorValue, error)
}

// LogbookCount is a department's checked and total checklist items.
type LogbookCount struct{ Checked, Total int }

// employeeCollectors and orgCollectors are the indicator codes with an
// automatic collector, per employee and organisation wide.
var (
	employeeCollectors = []string{"att_ontime", "pos_cash_variance", "logbook_compliance", "po_fulfillment",
		"team_ontime", "pos_sales_shift", "leave_request_discipline", "att_late_ratio", "task_completion"}
	orgCollectors = []string{"leave_sla", "vendor_pay_ontime", "vendor_pay_sla", "payroll_paid_ontime", "payroll_ready_h2"}
)

// snapshotCounts is SnapshotSummary.collectors.
type snapshotCounts struct {
	AttOntime         int `json:"att_ontime"`
	PosCashVariance   int `json:"pos_cash_variance"`
	LogbookCompliance int `json:"logbook_compliance"`
	POFulfillment     int `json:"po_fulfillment"`
	TeamOntime        int `json:"team_ontime"`
	PosSalesShift     int `json:"pos_sales_shift"`
	LeaveDiscipline   int `json:"leave_request_discipline"`
	AttLateRatio      int `json:"att_late_ratio"`
	LeaveSLA          int `json:"leave_sla"`
	VendorPayOntime   int `json:"vendor_pay_ontime"`
	VendorPaySLA      int `json:"vendor_pay_sla"`
	PayrollPaidOntime int `json:"payroll_paid_ontime"`
	PayrollReadyH2    int `json:"payroll_ready_h2"`
}

// snapshotSummary is SnapshotSummary.
type snapshotSummary struct {
	PeriodYear         int            `json:"period_year"`
	PeriodMonth        int            `json:"period_month"`
	Employees          int            `json:"employees"`
	SnapshotsUpserted  int64          `json:"snapshots_upserted"`
	ScorecardsUpserted int64          `json:"scorecards_upserted"`
	SkippedFinal       int            `json:"scorecards_skipped_final"`
	Collectors         snapshotCounts `json:"collectors"`
}

// POST /api/hris/kpi/snapshot { period_month?, period_year? }: defaults to
// the current month.
func (h *handler) kpiSnapshot(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.requirePerformanceHR(r); err != nil {
		return err
	}
	f := readOptionalJSON(r)
	month := coerceInt(f, "period_month", numRule{Rule: nullish,
		NumOpts: validate.NumOpts{Min: validate.Bound(1), Max: validate.Bound(12)}, Message: periodInvalid})
	year := coerceInt(f, "period_year", numRule{Rule: nullish,
		NumOpts: validate.NumOpts{Min: validate.Bound(2020), Max: validate.Bound(2100)}, Message: periodInvalid})
	if err := issuesErr(f, periodInvalid); err != nil {
		return err
	}
	now := h.svc.clock()
	m, y := int(now.Month()), now.Year()
	if month != nil {
		m = *month
	}
	if year != nil {
		y = *year
	}
	sum, err := h.svc.runKPISnapshot(r.Context(), y, m)
	if err != nil {
		return err
	}
	skipped := ""
	if sum.SkippedFinal > 0 {
		skipped = fmt.Sprintf(", %d final dilewati", sum.SkippedFinal)
	}
	return writeJSON(w, object("data", sum,
		"message", fmt.Sprintf("Snapshot KPI %d/%d: %d scorecard diperbarui%s", m, y, sum.ScorecardsUpserted, skipped)))
}

type indicator struct {
	ID, Code, Direction string
	DefaultTarget       *float64
}

type weighted struct {
	IndicatorID, Code string
	Weight            float64
}

// snapshotRow and scorecardRow are the computed writes.
type snapshotRow struct {
	employeeID, indicatorID string
	actual, target, attain  *float64
	sample                  float64
	detail                  string
}

type scorecardRow struct {
	employeeID, role string
	score            domain.Score
}

func (s *service) runKPISnapshot(ctx context.Context, year, month int) (*snapshotSummary, error) {
	start, end := domain.PeriodBounds(month, year)
	src := s.ports.KPI
	employees, err := src.Employees(ctx, s.db)
	if err != nil {
		return nil, err
	}
	cfg, err := s.kpiConfig(ctx)
	if err != nil {
		return nil, err
	}
	col, err := s.collect(ctx, employees, year, month, start, end)
	if err != nil {
		return nil, err
	}
	finals, manual, err := s.frozenScorecards(ctx, year, month)
	if err != nil {
		return nil, err
	}

	var snaps []snapshotRow
	var cards []scorecardRow
	for _, e := range employees {
		if finals[e.ID] {
			continue // frozen: the snapshots keep matching the locked breakdown
		}
		var comps []weighted
		if e.DepartmentID != nil {
			comps = cfg.byDepartment[*e.DepartmentID]
		}
		if comps == nil {
			comps = cfg.byRole[e.RoleCode]
		}
		if len(comps) == 0 {
			continue // no configuration (e.g. directors)
		}
		var parts []domain.ScoreComponent
		for _, c := range comps {
			ind, ok := cfg.indicators[c.IndicatorID]
			if !ok {
				continue
			}
			value, collected := col.value(e.ID, ind.Code)
			target := domain.ResolveTarget(cfg.targets[ind.ID], domain.TargetContext{EmployeeID: e.ID,
				DepartmentID: e.DepartmentID, RoleCode: e.RoleCode, Month: month, Year: year}, ind.DefaultTarget)
			var attain *float64
			if collected {
				attain = domain.Attainment(ind.Direction, value.Actual, target)
				detail, _ := json.Marshal(value.Detail)
				snaps = append(snaps, snapshotRow{e.ID, ind.ID, value.Actual, target, attain, value.SampleSize, string(detail)})
			} else {
				// Manual indicators (rubric) keep their stored attainment.
				attain = manual[e.ID+":"+ind.ID]
			}
			parts = append(parts, domain.ScoreComponent{Code: ind.Code, Weight: c.Weight, Attainment: attain})
		}
		cards = append(cards, scorecardRow{e.ID, e.RoleCode, domain.ComposeScore(parts)})
	}

	sum := &snapshotSummary{PeriodYear: year, PeriodMonth: month, Employees: len(employees),
		SkippedFinal: len(finals), Collectors: col.counts()}
	// One transaction: a partial failure rolls back cleanly.
	err = database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		snaps, cards, err := keepLiveEmployees(ctx, tx, snaps, cards)
		if err != nil {
			return err
		}
		if sum.SnapshotsUpserted, err = upsertSnapshots(ctx, tx, year, month, snaps); err != nil {
			return err
		}
		sum.ScorecardsUpserted, err = upsertScorecards(ctx, tx, year, month, cards)
		return err
	})
	return sum, err
}

// keepLiveEmployees locks the employees still present (FOR KEY SHARE, held
// until the write commits) and drops the rows of one deleted while the
// collectors ran, instead of failing the whole run on its foreign key.
func keepLiveEmployees(ctx context.Context, tx pgx.Tx, snaps []snapshotRow, cards []scorecardRow) ([]snapshotRow, []scorecardRow, error) {
	ids := make([]string, 0, len(cards))
	for _, c := range cards {
		ids = append(ids, c.employeeID)
	}
	rows, err := tx.Query(ctx, `SELECT id::text FROM hris.employees WHERE id = ANY($1::uuid[]) FOR KEY SHARE`, ids)
	if err != nil {
		return nil, nil, err
	}
	found, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, nil, err
	}
	live := make(map[string]bool, len(found))
	for _, id := range found {
		live[id] = true
	}
	snaps = slices.DeleteFunc(snaps, func(r snapshotRow) bool { return !live[r.employeeID] })
	cards = slices.DeleteFunc(cards, func(r scorecardRow) bool { return !live[r.employeeID] })
	return snaps, cards, nil
}

type kpiConfig struct {
	indicators   map[string]indicator
	byRole       map[string][]weighted
	byDepartment map[string][]weighted
	targets      map[string][]domain.KPITarget
}

// kpiConfig loads the active indicators, the role and department weights
// (a configured department wins over the role mapping) and the targets.
func (s *service) kpiConfig(ctx context.Context) (*kpiConfig, error) {
	cfg := &kpiConfig{indicators: map[string]indicator{}, byRole: map[string][]weighted{},
		byDepartment: map[string][]weighted{}, targets: map[string][]domain.KPITarget{}}
	rows, err := s.db.Query(ctx, `SELECT id::text, code, direction::text, default_target::text
		FROM performance.kpi_indicators WHERE is_active = true`)
	if err != nil {
		return nil, err
	}
	inds, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (indicator, error) {
		var i indicator
		var def *string
		err := r.Scan(&i.ID, &i.Code, &i.Direction, &def)
		if def != nil {
			if v := domain.JSNumber(*def); domain.Finite(v) {
				i.DefaultTarget = &v
			}
		}
		return i, err
	})
	if err != nil {
		return nil, err
	}
	for _, i := range inds {
		cfg.indicators[i.ID] = i
	}
	for sql, into := range map[string]map[string][]weighted{
		`SELECT ri.role_code, ri.indicator_id::text, ri.weight::float, i.code
		 FROM performance.kpi_role_indicators ri JOIN performance.kpi_indicators i ON i.id = ri.indicator_id
		 WHERE i.is_active = true`: cfg.byRole,
		`SELECT di.department_id::text, di.indicator_id::text, di.weight::float, i.code
		 FROM performance.kpi_department_indicators di JOIN performance.kpi_indicators i ON i.id = di.indicator_id
		 WHERE i.is_active = true`: cfg.byDepartment,
	} {
		rows, err := s.db.Query(ctx, sql)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var key string
			var w weighted
			if err := rows.Scan(&key, &w.IndicatorID, &w.Weight, &w.Code); err != nil {
				rows.Close()
				return nil, err
			}
			into[key] = append(into[key], w)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	rows, err = s.db.Query(ctx, `SELECT indicator_id::text, period_year, period_month, role_code,
		department_id::text, employee_id::text, target::float FROM performance.kpi_targets`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var t domain.KPITarget
		if err := rows.Scan(&id, &t.PeriodYear, &t.PeriodMonth, &t.RoleCode, &t.DepartmentID, &t.EmployeeID, &t.Target); err != nil {
			return nil, err
		}
		cfg.targets[id] = append(cfg.targets[id], t)
	}
	return cfg, rows.Err()
}

// collected holds every collector's result.
type collected struct {
	perEmployee map[string]map[string]domain.CollectorValue
	org         map[string]*domain.CollectorValue
}

// value is the collector's measurement for one employee; collected is
// false when the indicator has no collector or no data for them.
func (c collected) value(employeeID, code string) (domain.CollectorValue, bool) {
	if slices.Contains(employeeCollectors, code) {
		v, ok := c.perEmployee[code][employeeID]
		return v, ok
	}
	if v := c.org[code]; v != nil {
		return *v, true
	}
	return domain.CollectorValue{}, false
}

func (c collected) counts() snapshotCounts {
	n := func(code string) int { return len(c.perEmployee[code]) }
	one := func(code string) int {
		if c.org[code] != nil {
			return 1
		}
		return 0
	}
	return snapshotCounts{n("att_ontime"), n("pos_cash_variance"), n("logbook_compliance"), n("po_fulfillment"),
		n("team_ontime"), n("pos_sales_shift"), n("leave_request_discipline"), n("att_late_ratio"),
		one("leave_sla"), one("vendor_pay_ontime"), one("vendor_pay_sla"), one("payroll_paid_ontime"), one("payroll_ready_h2")}
}

// collect runs the wave 1 and 2 collectors.
func (s *service) collect(ctx context.Context, employees []domain.KPIEmployee, year, month int, start, end string) (*collected, error) {
	src, q := s.ports.KPI, s.db
	c := &collected{perEmployee: map[string]map[string]domain.CollectorValue{}, org: map[string]*domain.CollectorValue{}}
	patterns, err := src.ShiftPatterns(ctx, q, start, end)
	if err != nil {
		return nil, err
	}
	present, err := src.PresentDays(ctx, q, start, end)
	if err != nil {
		return nil, err
	}
	leaves, err := src.ApprovedLeaves(ctx, q, start, end)
	if err != nil {
		return nil, err
	}
	ontime := map[string]domain.CollectorValue{}
	for _, e := range employees {
		ontime[e.ID] = domain.ComputeOntime(patterns[e.ID], present[e.ID], domain.LeaveDates(leaves[e.ID], start, end), start, end)
	}
	c.perEmployee["att_ontime"] = ontime
	c.perEmployee["team_ontime"] = domain.TeamOntime(employees, ontime)

	shifts, err := src.Shifts(ctx, q)
	if err != nil {
		return nil, err
	}
	late, err := src.LateMinutes(ctx, q, start, end)
	if err != nil {
		return nil, err
	}
	lateRatio := map[string]domain.CollectorValue{}
	for _, e := range employees {
		if len(patterns[e.ID]) > 0 { // without a schedule it cannot be measured
			lateRatio[e.ID] = domain.LateRatio(patterns[e.ID], shifts, late[e.ID], start, end)
		}
	}
	c.perEmployee["att_late_ratio"] = lateRatio

	logbooks, err := src.Logbooks(ctx, q, start, end)
	if err != nil {
		return nil, err
	}
	logbook := map[string]domain.CollectorValue{}
	for _, e := range employees {
		if e.DepartmentID == nil {
			continue
		}
		if l, ok := logbooks[*e.DepartmentID]; ok {
			logbook[e.ID] = domain.CollectorValue{Actual: domain.SafeRatio(float64(l.Checked), float64(l.Total)),
				SampleSize: float64(l.Total), Detail: map[string]any{"department_id": *e.DepartmentID,
					"checked_items": l.Checked, "total_items": l.Total}}
		}
	}
	c.perEmployee["logbook_compliance"] = logbook

	if c.perEmployee["pos_cash_variance"], c.perEmployee["pos_sales_shift"], err = src.CashierShifts(ctx, q, start, end); err != nil {
		return nil, err
	}
	if c.perEmployee["po_fulfillment"], err = src.POFulfillment(ctx, q, start, end); err != nil {
		return nil, err
	}
	if c.perEmployee["leave_request_discipline"], err = src.LeaveDiscipline(ctx, q, start, end); err != nil {
		return nil, err
	}
	tasks, err := s.taskCounts(ctx, start, end)
	if err != nil {
		return nil, err
	}
	c.perEmployee["task_completion"] = domain.TaskCompletion(employees, tasks)

	if c.org["leave_sla"], err = src.LeaveSLA(ctx, q, start, end); err != nil {
		return nil, err
	}
	if c.org["vendor_pay_ontime"], err = src.VendorPayOntime(ctx, q, start, end); err != nil {
		return nil, err
	}
	if c.org["vendor_pay_sla"], err = src.VendorPaySLA(ctx, q, start, end); err != nil {
		return nil, err
	}
	c.org["payroll_paid_ontime"], c.org["payroll_ready_h2"], err = s.payrollTimeliness(ctx, year, month)
	return c, err
}

// taskCounts groups the period's department task occurrences.
func (s *service) taskCounts(ctx context.Context, start, end string) ([]domain.TaskCount, error) {
	rows, err := s.db.Query(ctx, `SELECT t.department_id::text, t.assignee_employee_id::text, o.done_by::text, o.status::text,
		count(*)::int AS jumlah
		FROM hris.department_task_occurrences o
		JOIN hris.department_tasks t ON t.id = o.task_id
		WHERE o.occurrence_date BETWEEN $1 AND $2
		GROUP BY 1, 2, 3, 4`, start, end)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.TaskCount, error) {
		var t domain.TaskCount
		return t, r.Scan(&t.DepartmentID, &t.AssigneeEmployeeID, &t.DoneBy, &t.Status, &t.Count)
	})
}

// payrollTimeliness compares the period's runs with the payroll day.
func (s *service) payrollTimeliness(ctx context.Context, year, month int) (paid, ready *domain.CollectorValue, err error) {
	var day *string
	if err := s.db.QueryRow(ctx, `SELECT payroll_day::text FROM hris.payroll_settings LIMIT 1`).Scan(&day); err != nil && !database.IsNoRows(err) {
		return nil, nil, err
	}
	payrollDay := 25
	if day != nil {
		if v := domain.OrZero(*day); v != 0 {
			payrollDay = int(v)
		}
	}
	rows, err := s.db.Query(ctx, `SELECT approved_at, paid_at FROM hris.payroll_runs
		WHERE period_year = $1 AND period_month = $2`, year, month)
	if err != nil {
		return nil, nil, err
	}
	runs, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.RunTimes, error) {
		var t domain.RunTimes
		return t, r.Scan(&t.ApprovedAt, &t.PaidAt)
	})
	if err != nil {
		return nil, nil, err
	}
	paid, ready = domain.PayrollTimeliness(payrollDay, year, month, runs)
	return paid, ready, nil
}

// frozenScorecards are the employees with a final scorecard in the period
// and the stored attainments of manual indicators (so a re-run keeps the
// rubric scores).
func (s *service) frozenScorecards(ctx context.Context, year, month int) (map[string]bool, map[string]*float64, error) {
	finals := map[string]bool{}
	rows, err := s.db.Query(ctx, `SELECT employee_id::text FROM performance.kpi_scorecards
		WHERE period_year = $1 AND period_month = $2 AND status = 'final'`, year, month)
	if err != nil {
		return nil, nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, nil, err
	}
	for _, id := range ids {
		finals[id] = true
	}
	manual := map[string]*float64{}
	rows, err = s.db.Query(ctx, `SELECT s.employee_id::text, s.indicator_id::text, s.attainment::float
		FROM performance.kpi_snapshots s
		JOIN performance.kpi_indicators i ON i.id = s.indicator_id
		WHERE s.period_year = $1 AND s.period_month = $2 AND i.source_kind = 'manual'`, year, month)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var emp, ind string
		var v *float64
		if err := rows.Scan(&emp, &ind, &v); err != nil {
			return nil, nil, err
		}
		manual[emp+":"+ind] = v
	}
	return finals, manual, rows.Err()
}

func upsertSnapshots(ctx context.Context, tx pgx.Tx, year, month int, rows []snapshotRow) (int64, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	n := len(rows)
	emp, ind, years, months := make([]string, n), make([]string, n), make([]int, n), make([]int, n)
	actual, target, attain, sample := make([]*float64, n), make([]*float64, n), make([]*float64, n), make([]float64, n)
	detail := make([]string, n)
	for i, r := range rows {
		emp[i], ind[i], years[i], months[i] = r.employeeID, r.indicatorID, year, month
		actual[i], target[i], attain[i], sample[i], detail[i] = r.actual, r.target, r.attain, r.sample, r.detail
	}
	tag, err := tx.Exec(ctx, `INSERT INTO performance.kpi_snapshots
		  (employee_id, indicator_id, period_year, period_month, actual, target, attainment, sample_size, source_detail)
		SELECT * FROM unnest($1::uuid[], $2::uuid[], $3::int[], $4::int[],
		  $5::numeric[], $6::numeric[], $7::numeric[], $8::numeric[], $9::jsonb[])
		ON CONFLICT (employee_id, indicator_id, period_year, period_month)
		DO UPDATE SET actual = EXCLUDED.actual, target = EXCLUDED.target, attainment = EXCLUDED.attainment,
		  sample_size = EXCLUDED.sample_size, source_detail = EXCLUDED.source_detail, updated_at = now()`,
		emp, ind, years, months, actual, target, attain, sample, detail)
	return tag.RowsAffected(), err
}

func upsertScorecards(ctx context.Context, tx pgx.Tx, year, month int, rows []scorecardRow) (int64, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	n := len(rows)
	emp, role, years, months := make([]string, n), make([]string, n), make([]int, n), make([]int, n)
	score, raw, used := make([]*float64, n), make([]*float64, n), make([]float64, n)
	breakdown := make([]string, n)
	for i, r := range rows {
		b, _ := json.Marshal(r.score.Breakdown)
		emp[i], role[i], years[i], months[i] = r.employeeID, r.role, year, month
		score[i], raw[i], used[i], breakdown[i] = r.score.Score, r.score.RawScore, r.score.UsedWeight, string(b)
	}
	// The WHERE keeps a scorecard finalised since the read as it is, and
	// RowsAffected leaves it out.
	tag, err := tx.Exec(ctx, `INSERT INTO performance.kpi_scorecards
		  (employee_id, period_year, period_month, role_code, score, raw_score, used_weight, breakdown, status)
		SELECT *, 'draft' FROM unnest($1::uuid[], $2::int[], $3::int[], $4::varchar[],
		  $5::numeric[], $6::numeric[], $7::numeric[], $8::jsonb[])
		ON CONFLICT (employee_id, period_year, period_month)
		DO UPDATE SET role_code = EXCLUDED.role_code, score = EXCLUDED.score, raw_score = EXCLUDED.raw_score,
		  used_weight = EXCLUDED.used_weight, breakdown = EXCLUDED.breakdown, updated_at = now()
		WHERE performance.kpi_scorecards.status = 'draft'`,
		emp, years, months, role, score, raw, used, breakdown)
	return tag.RowsAffected(), err
}

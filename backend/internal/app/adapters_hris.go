package app

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/modules/hris"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/whatsapp"
)

// Adapters for the hris module's ports. Payroll, salary, recruitment and
// identity tables belong to other contexts; until those modules expose read
// services these run the SQL the TS libs ran (lib/hris/me-profile,
// nav-badges, create-contract, contracts-lifecycle, employees-profile,
// master-data, leave-wa, lib/whatsapp/gateway).

/* ── Payroll reads (payroll module tables) ───────────────────────────── */

type hrisPayrollSQL struct{}

var _ hris.PayrollReads = hrisPayrollSQL{}

// LatestPaidPayslip covers the employee's newest paid period. A period can
// hold one paid run per entity (EPIC-054), so the net salary is the sum of
// that period's slips; the run name and paid-at stay those of its latest paid
// run, so the payslip still names one real run.
func (hrisPayrollSQL) LatestPaidPayslip(ctx context.Context, q database.Querier, employeeID string) (*hris.Payslip, error) {
	var p hris.Payslip
	err := q.QueryRow(ctx, `SELECT sum(pd.net_salary)::text,
		(array_agg(pr.run_name ORDER BY pr.paid_at DESC NULLS LAST, pr.created_at DESC))[1],
		pr.period_month, pr.period_year,
		(array_agg(pr.paid_at ORDER BY pr.paid_at DESC NULLS LAST, pr.created_at DESC))[1]::text
		FROM hris.payroll_details pd
		JOIN hris.payroll_runs pr ON pr.id = pd.payroll_run_id
		WHERE pd.employee_id = $1 AND pr.status = 'paid'
		GROUP BY pr.period_year, pr.period_month
		ORDER BY pr.period_year DESC, pr.period_month DESC
		LIMIT 1`, employeeID).Scan(&p.NetSalary, &p.RunName, &p.PeriodMonth, &p.PeriodYear, &p.PaidAt)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (hrisPayrollSQL) ActiveLoans(ctx context.Context, q database.Querier, employeeID string) (hris.LoanSummary, error) {
	var s hris.LoanSummary
	err := q.QueryRow(ctx, `SELECT count(*),
		COALESCE(sum(remaining_balance), 0)::text,
		COALESCE(sum(monthly_installment)
		         FILTER (WHERE status = 'approved' AND COALESCE(remaining_balance, 0) > 0), 0)::text
		FROM hris.loans
		WHERE employee_id = $1 AND is_active
		  AND status IN ('pending', 'approved')
		  AND (status = 'pending' OR COALESCE(remaining_balance, 0) > 0)`, employeeID).
		Scan(&s.Count, &s.TotalRemaining, &s.MonthlyInstallment)
	return s, err
}

func (hrisPayrollSQL) RecentLoans(ctx context.Context, q database.Querier, employeeID string) ([]hris.RecentLoan, error) {
	rows, err := q.Query(ctx, `SELECT id::text, loan_type, principal_amount::text, status, created_at::text
		FROM hris.loans WHERE employee_id = $1 ORDER BY created_at DESC LIMIT 5`, employeeID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[hris.RecentLoan])
}

func (hrisPayrollSQL) LatestKPISummary(ctx context.Context, q database.Querier, employeeID string) (hris.KPISummary, error) {
	var k hris.KPISummary
	err := q.QueryRow(ctx, `SELECT count(*),
		round(avg(achievement_percentage), 0)::text,
		round(avg(score)::numeric, 1)::text
		FROM hris.employee_kpis
		WHERE employee_id = $1
		  AND review_id = (
		    SELECT review_id FROM hris.employee_kpis
		    WHERE employee_id = $1 ORDER BY created_at DESC LIMIT 1
		  )`, employeeID).Scan(&k.Count, &k.AvgAchievement, &k.AvgScore)
	return k, err
}

// PendingLoanCount is HR's queue: kasbon the manager approved, plus pending
// loans with no manager stage.
func (hrisPayrollSQL) PendingLoanCount(ctx context.Context, q database.Querier) (int64, error) {
	var n int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM hris.loans
		WHERE status = 'manager_approved' OR (status = 'pending' AND manager_approver_id IS NULL)`).Scan(&n)
	return n, err
}

func (hrisPayrollSQL) LoanUpdatesSince(ctx context.Context, q database.Querier, employeeID string, since *time.Time) (int64, error) {
	var n int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM hris.loans
		WHERE employee_id = $1 AND status <> 'pending'
		  AND updated_at > COALESCE($2::timestamptz, to_timestamp(0))`, employeeID, since).Scan(&n)
	return n, err
}

// PeriodRuns are the payroll runs of a year (attendance period closing:
// a run past draft blocks reopening its month, an open period blocks
// processing/approving its run).
func (hrisPayrollSQL) PeriodRuns(ctx context.Context, q database.Querier, year int) ([]hris.PeriodRun, error) {
	rows, err := q.Query(ctx, `SELECT id::text, COALESCE(run_name, ''), status, period_year, period_month
		FROM hris.payroll_runs WHERE period_year = $1 ORDER BY period_month, created_at`, year)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (hris.PeriodRun, error) {
		var p hris.PeriodRun
		return p, r.Scan(&p.ID, &p.Name, &p.Status, &p.Year, &p.Month)
	})
}

/* ── Salary structure (payroll module table) ─────────────────────────── */

type hrisSalarySQL struct{}

var _ hris.SalaryPort = hrisSalarySQL{}

// ActiveBaseSalary is the total base salary over every paying entity.
func (hrisSalarySQL) ActiveBaseSalary(ctx context.Context, q database.Querier, employeeID string) (*string, error) {
	var base *string
	err := q.QueryRow(ctx, `SELECT sum(base_salary)::text FROM hris.employee_salary
		WHERE employee_id = $1 AND is_active`, employeeID).Scan(&base)
	return base, err
}

// SyncFromContract is syncSalaryFromContract: a new salary version at the
// contract's base salary, carrying the paying entity, allowances, PTKP and
// BPJS flags from the current version (table defaults for a first version,
// whose entity is the employee's primary placement company). An employee
// paid by several entities is left alone: HR versions each entity itself. The numeric
// parameters are cast explicitly: the TS sends them untyped, so PostgreSQL
// reads COALESCE($n, 0) as integer and fails on "1500000.00".
func (hrisSalarySQL) SyncFromContract(ctx context.Context, q database.Querier, in hris.SalarySync) error {
	var (
		id                                                     string
		base                                                   float64
		fixed, variable, transport, meal, housing, loan, other *string
		ptkp, company                                          *string
		taxable, bpjsTK, bpjsKes, tapera                       *bool
	)
	var entities int
	if err := q.QueryRow(ctx, `SELECT count(DISTINCT COALESCE(company_id::text, ''))::int FROM hris.employee_salary
		WHERE employee_id = $1 AND is_active`, in.EmployeeID).Scan(&entities); err != nil {
		return err
	}
	if entities > 1 {
		// Paid by several entities: HR versions each entity's salary itself.
		return nil
	}
	err := q.QueryRow(ctx, `SELECT id::text, base_salary::float8, fixed_allowance::text, variable_allowance::text,
		transport_allowance::text, meal_allowance::text, housing_allowance::text, loan_deduction::text,
		other_deduction::text, ptkp_status, is_taxable, bpjs_tk_enrolled, bpjs_kes_enrolled, tapera_enrolled,
		company_id::text
		FROM hris.employee_salary
		WHERE employee_id = $1 AND is_active = true
		ORDER BY effective_date DESC LIMIT 1`, in.EmployeeID).Scan(&id, &base, &fixed, &variable, &transport, &meal,
		&housing, &loan, &other, &ptkp, &taxable, &bpjsTK, &bpjsKes, &tapera, &company)
	// EPIC-055 rule columns are carried to the new version (else a renewal
	// would reset a daily wage to a monthly salary).
	var payBasis, transportMode, mealMode, positionMode, scheme, divisor, position, health *string
	var minService *int
	if hasRow := err == nil; hasRow {
		if rerr := q.QueryRow(ctx, `SELECT pay_basis, transport_mode, meal_mode, position_mode, overtime_scheme,
			working_days_divisor::text, position_allowance::text, health_allowance::text, health_min_service_days
			FROM hris.employee_salary WHERE id = $1`, id).Scan(&payBasis, &transportMode, &mealMode, &positionMode,
			&scheme, &divisor, &position, &health, &minService); rerr != nil {
			return rerr
		}
	}
	hasCurrent := err == nil
	if err != nil && !database.IsNoRows(err) {
		return err
	}
	if hasCurrent && base == in.BaseSalary {
		return nil
	}
	if hasCurrent {
		if _, err := q.Exec(ctx, `UPDATE hris.employee_salary
			SET is_active = false, end_date = ($2::date - INTERVAL '1 day')::date, updated_at = now()
			WHERE id = $1`, id, in.StartDate); err != nil {
			return err
		}
	}
	_, err = q.Exec(ctx, `INSERT INTO hris.employee_salary
		(employee_id, base_salary, fixed_allowance, variable_allowance,
		 transport_allowance, meal_allowance, housing_allowance,
		 loan_deduction, other_deduction, ptkp_status, is_taxable,
		 bpjs_tk_enrolled, bpjs_kes_enrolled, tapera_enrolled,
		 effective_date, is_active, notes, company_id,
		 pay_basis, transport_mode, meal_mode, position_mode, overtime_scheme,
		 working_days_divisor, position_allowance, health_allowance, health_min_service_days)
		VALUES ($1, $2,
		        COALESCE($3::numeric, 0), COALESCE($4::numeric, 0), COALESCE($5::numeric, 0),
		        COALESCE($6::numeric, 0), COALESCE($7::numeric, 0),
		        COALESCE($8::numeric, 0), COALESCE($9::numeric, 0),
		        COALESCE($10::text, 'TK/0'), COALESCE($11::boolean, true),
		        COALESCE($12::boolean, true), COALESCE($13::boolean, true), COALESCE($14::boolean, true),
		        $15::date, true, $16,
		        COALESCE($17::uuid, (SELECT b.company_id FROM hris.employee_placements p
		                               JOIN configuration.branches b ON b.id = p.branch_id
		                              WHERE p.employee_id = $1 AND p.is_primary
		                                AND (p.end_date IS NULL OR p.end_date >= $15::date)
		                              ORDER BY p.start_date DESC LIMIT 1)),
		        COALESCE($18::text, 'monthly'), COALESCE($19::text, 'flat'), COALESCE($20::text, 'flat'),
		        COALESCE($21::text, 'flat'), COALESCE($22::text, 'standard'), $23::numeric,
		        COALESCE($24::numeric, 0), COALESCE($25::numeric, 0), COALESCE($26::int, 0))
		ON CONFLICT (employee_id, company_id, effective_date) DO UPDATE SET
		  base_salary = EXCLUDED.base_salary,
		  is_active = true,
		  end_date = NULL,
		  notes = EXCLUDED.notes,
		  updated_at = now()`,
		in.EmployeeID, in.BaseSalary, fixed, variable, transport, meal, housing, loan, other,
		ptkp, taxable, bpjsTK, bpjsKes, tapera, in.StartDate, "Sinkron dari kontrak "+in.ContractNumber, company,
		payBasis, transportMode, mealMode, positionMode, scheme, divisor, position, health, minService)
	return err
}

/* ── Recruitment reads (recruitment module tables) ───────────────────── */

type hrisRecruitmentSQL struct{}

var _ hris.Recruitment = hrisRecruitmentSQL{}

func (hrisRecruitmentSQL) PromotedCandidate(ctx context.Context, q database.Querier, employeeID string) (*hris.PromotedCandidate, error) {
	var c hris.PromotedCandidate
	err := q.QueryRow(ctx, `SELECT c.id::text, c.cv_url, c.status, c.created_at, c.promotion_date, c.source,
		p.title,
		(SELECT o.responded_at FROM recruitment.candidate_offers o
		 WHERE o.candidate_id = c.id AND o.status = 'accepted'
		 ORDER BY o.responded_at DESC LIMIT 1)
		FROM recruitment.candidates c
		LEFT JOIN hris.positions p ON p.id = c.position_id
		WHERE c.promoted_to_employee_id = $1
		ORDER BY c.created_at DESC LIMIT 1`, employeeID).Scan(&c.ID, &c.CVURL, &c.Status, &c.CreatedAt,
		&c.PromotionDate, &c.Source, &c.PositionTitle, &c.OfferAcceptedAt)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

/* ── Identity and catalog reads ──────────────────────────────────────── */

type hrisDirectorySQL struct{}

var _ hris.Directory = hrisDirectorySQL{}

func (hrisDirectorySQL) Account(ctx context.Context, q database.Querier, userID string) (*hris.Account, error) {
	var a hris.Account
	err := q.QueryRow(ctx, `SELECT email, created_at, last_sign_in_at FROM auth.users WHERE id = $1`, userID).
		Scan(&a.Email, &a.CreatedAt, &a.LastSignInAt)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (hrisDirectorySQL) SuperAdminUserIDs(ctx context.Context, q database.Querier) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT id::text FROM configuration.users WHERE role = 'super_admin'`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func (hrisDirectorySQL) BrandNames(ctx context.Context, q database.Querier, ids []string) (map[string]string, error) {
	rows, err := q.Query(ctx, `SELECT id::text, name FROM item.brands WHERE id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = name
	}
	return out, rows.Err()
}

/* ── WhatsApp gateway (platform/whatsapp) ─────────────────────────────── */

// hrisWhatsApp is lib/whatsapp/gateway (loadGatewayConfig + sendGatewayText)
// plus the configuration.wa_notif_log claim of lib/hris/leave-wa.
type hrisWhatsApp struct {
	db database.Querier
	wa *whatsapp.Client
}

var _ hris.WhatsAppSender = (*hrisWhatsApp)(nil)

func (w *hrisWhatsApp) Claim(ctx context.Context, q database.Querier, notifType, dedupKey, message string, recipients []string) (string, bool, error) {
	id, err := whatsapp.Claim(ctx, q, notifType, dedupKey, message, recipients)
	return id, id != "", err
}

func (w *hrisWhatsApp) Release(ctx context.Context, q database.Querier, id string) error {
	return whatsapp.Release(ctx, q, id)
}

func (w *hrisWhatsApp) Configured(ctx context.Context) bool {
	return w.wa.LoadGateway(ctx, w.db) != nil
}

func (w *hrisWhatsApp) SendText(ctx context.Context, target, message string) (bool, bool, string) {
	gateway := w.wa.LoadGateway(ctx, w.db)
	if gateway == nil {
		return false, false, "gateway belum dikonfigurasi"
	}
	res := gateway.SendText(ctx, target, message)
	return res.Success, res.TimedOut, res.Reason
}

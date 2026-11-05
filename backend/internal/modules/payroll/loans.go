package payroll

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/modules/payroll/domain"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/validate"
)

// Employee loans (lib/payroll/loan-requests.ts): HR/finance manage every
// loan; an employee files and sees only their own (self requests are
// interest free).

const invalidLoan = "Jenis pinjaman, jumlah (> 0), dan tenor (1–60 bulan) wajib valid"

// loanTypes are the requestable loan types (EPIC-060 R1; emergency removed).
var loanTypes = []string{"kasbon", "loan"}

// loanTHP is the THP both loan types are capped by (EPIC-060 R2, EPIC-076
// R4): base + transport + meal + position allowance over every active salary
// row; nil without one.
func (s *service) loanTHP(ctx context.Context, employeeID string) (*float64, error) {
	var thp *float64
	err := s.db.QueryRow(ctx, `SELECT sum(base_salary + coalesce(transport_allowance, 0) + coalesce(meal_allowance, 0)
		+ coalesce(position_allowance, 0))::float8 FROM hris.employee_salary WHERE employee_id = $1 AND is_active`,
		employeeID).Scan(&thp)
	return thp, err
}

var loanListPeople = []embed{{"employee", "employee_id", loanListEmp}, {"approved_by", "approved_by", personFields}}

func (s *service) listLoans(ctx context.Context, a *actor, employeeParam, status *string) ([]*obj, error) {
	employeeID := ""
	if !domain.HasLoanFullAccess(a.Role) || (employeeParam != nil && *employeeParam == "me") {
		if a.EmployeeID == nil {
			return []*obj{}, nil
		}
		employeeID = *a.EmployeeID
	} else if employeeParam != nil {
		employeeID = *employeeParam
	}
	where, args := "", []any{}
	if employeeID != "" {
		args = append(args, employeeID)
		where = " WHERE employee_id = $1"
	}
	if status != nil {
		args = append(args, *status)
		if where == "" {
			where = " WHERE status = $1"
		} else {
			where += " AND status = $2"
		}
	}
	rows, err := queryObjs(ctx, s.db, `SELECT * FROM hris.loans`+where+` ORDER BY created_at DESC`, args...)
	if err != nil {
		return nil, opaque(err)
	}
	return rows, s.stitch(ctx, s.db, rows, loanListPeople...)
}

// loanStageOf is the pending loans whose current stage belongs to the
// employee id param: the SPV stage first, then the Manager (EPIC-091).
func loanStageOf(param string) string {
	return `status = 'pending' AND ((supervisor_approver_id = ` + param + ` AND supervisor_approved_at IS NULL)
		OR (manager_approver_id = ` + param + ` AND (supervisor_approver_id IS NULL OR supervisor_approved_at IS NOT NULL)))`
}

// listLoanApprovals is the atasan queue: pending kasbon and loans at the
// actor's stage (SPV, then Manager), with their attachments. Salary-free
// columns only (EPIC-060 R8).
func (s *service) listLoanApprovals(ctx context.Context, a *actor) ([]*obj, error) {
	if a.EmployeeID == nil {
		return []*obj{}, nil
	}
	rows, err := queryObjs(ctx, s.db, `SELECT id, employee_id, loan_type, principal_amount, tenor_months, monthly_installment,
		purpose, notes, status, created_at, attachments, collateral_attachments FROM hris.loans WHERE `+loanStageOf("$1")+` ORDER BY created_at`,
		*a.EmployeeID)
	if err != nil {
		return nil, opaque(err)
	}
	return rows, s.stitch(ctx, s.db, rows, loanListPeople[0])
}

// loanLimits is loadLoanLimitContext: the configured limits and the loans
// that count against the active-loan limit.
func (s *service) loanLimits(ctx context.Context, employeeID string, includePending bool) (domain.Config, float64, error) {
	settings, tax, err := payrollConfigRows(ctx, s.db, s.clock().Year())
	if err != nil {
		return domain.Config{}, 0, err
	}
	filter := `status IN ('pending', 'manager_approved', 'approved')`
	if !includePending {
		filter = `status = 'approved' AND remaining_balance > 0`
	}
	var count int
	err = s.db.QueryRow(ctx, `SELECT count(*) FROM hris.loans WHERE employee_id = $1 AND is_active = true AND `+filter+`
		AND (status IN ('pending', 'manager_approved') OR remaining_balance > 0)`, employeeID).Scan(&count)
	return domain.BuildConfig(settings, tax), float64(count), err
}

// checkLoanTnC re-reads the employee and THP and applies the type's terms
// (EPIC-076) as of today.
func (s *service) checkLoanTnC(ctx context.Context, emp *EmployeeBrief, loanType string, principal float64, tenor int,
	activeCount, maxActive float64, showCap bool) error {
	thp, err := s.loanTHP(ctx, emp.ID)
	if err != nil {
		return err
	}
	applicant := domain.LoanApplicant{Status: emp.EmploymentStatus, JoinDate: emp.JoinDate}
	if msg := domain.ValidateLoanTnC(loanType, principal, tenor, applicant, s.clock(), thp, activeCount, maxActive, showCap); msg != "" {
		return httpx.BadRequest(msg)
	}
	return nil
}

// runClosed: a payroll run of that month is past draft (processing,
// completed, paid) for any entity; only draft runs are recalculated
// (domain.CanCalculateRun), so its slips can no longer take a deduction
// (EPIC-076 R6).
func (s *service) runClosed(ctx context.Context, month, year int) (bool, error) {
	var closed bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM hris.payroll_runs WHERE period_month = $1 AND period_year = $2
		AND status <> 'draft')`, month, year).Scan(&closed)
	return closed, err
}

// loanFirstInstallment is the first payroll month of a loan filed at
// submittedAt: its 21–20 period, moved on past closed runs.
func (s *service) loanFirstInstallment(ctx context.Context, submittedAt time.Time) (month, year int, err error) {
	month, year = domain.LoanPayrollPeriod(submittedAt)
	for range 24 {
		closed, err := s.runClosed(ctx, month, year)
		if err != nil || !closed {
			return month, year, err
		}
		if month++; month > 12 {
			month, year = 1, year+1
		}
	}
	return month, year, nil
}

// loanTypeLabel names a loan type in messages.
func loanTypeLabel(loanType string) string {
	if rule, ok := domain.LoanRuleFor(loanType); ok {
		return rule.Label
	}
	return "Pinjaman"
}

type loanInput struct {
	EmployeeID   *string
	LoanType     string
	Principal    float64
	InterestRate any
	Tenor        int
	Purpose      *string
	Notes        *string
	// Attachments are supporting documents, Collateral the surat berharga
	// (EPIC-076 R8).
	Attachments, Collateral []string
}

func (s *service) createLoan(ctx context.Context, a *actor, in loanInput) (*obj, error) {
	full := domain.HasLoanFullAccess(a.Role)
	var target string
	if full && in.EmployeeID != nil && *in.EmployeeID != "" && *in.EmployeeID != "me" {
		// Lower case: attachment folders use the id as Postgres prints it.
		target = strings.ToLower(*in.EmployeeID)
	} else if a.EmployeeID != nil {
		target = *a.EmployeeID
	}
	if target == "" {
		return nil, httpx.BadRequest("Akun ini tidak tertaut ke data karyawan")
	}
	rate := 0.0
	if full {
		// Number(interest_rate) || 0: absent, null and NaN are all 0.
		if n := domain.JSNumber(in.InterestRate); !math.IsNaN(n) {
			rate = n
		}
	}
	if rate < 0 || rate > 100 {
		return nil, httpx.BadRequest(invalidLoan)
	}
	emp, err := s.brief(ctx, target)
	if err != nil {
		return nil, err
	}
	if emp == nil {
		return nil, httpx.NotFound("Karyawan tidak ditemukan")
	}
	if !emp.IsActive {
		return nil, httpx.BadRequest("Karyawan sudah tidak aktif")
	}
	if in.LoanType == "emergency" {
		return nil, httpx.BadRequest("Pinjaman Darurat sudah tidak tersedia")
	}
	if !slices.Contains(loanTypes, in.LoanType) {
		return nil, httpx.BadRequest(invalidLoan)
	}
	_, monthly, total := domain.LoanTerms(in.Principal, rate, in.Tenor)
	config, count, err := s.loanLimits(ctx, target, true)
	if err != nil {
		return nil, err
	}
	showCap := a.Payroll || (a.EmployeeID != nil && *a.EmployeeID == target)
	if err := s.checkLoanTnC(ctx, emp, in.LoanType, in.Principal, in.Tenor, count, config.LoanMaxActivePerPerson, showCap); err != nil {
		return nil, err
	}
	kasbon := in.LoanType == "kasbon"
	if kasbon && len(in.Collateral) > 0 {
		return nil, httpx.BadRequest("Jaminan hanya untuk Pinjaman Karyawan")
	}
	if err := s.checkLoanAttachments(target, in.Attachments, kasbon, "Lampirkan minimal 1 dokumen pendukung kasbon"); err != nil {
		return nil, err
	}
	if err := s.checkLoanAttachments(target, in.Collateral, !kasbon, "Lampirkan minimal 1 dokumen jaminan (surat berharga)"); err != nil {
		return nil, err
	}
	// EPIC-091: the nearest atasan from Supervisor up decides first, then the
	// Manager up the chain when that is someone else (EPIC-060 AP-05,
	// EPIC-076 R7), then HR; none means HR decides alone.
	nearest, err := s.ports.Employees.RequestApprover(ctx, s.db, target)
	if err != nil {
		return nil, err
	}
	manager, err := s.ports.Employees.LoanApprover(ctx, s.db, target)
	if err != nil {
		return nil, err
	}
	supervisor, approver := loanStages(nearest, manager)
	now := s.clock()
	var row *obj
	err = database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		var err error
		row, err = queryObj(ctx, tx, `INSERT INTO hris.loans (employee_id, loan_type, principal_amount, interest_rate,
			tenor_months, monthly_installment, remaining_balance, purpose, notes, status, manager_approver_id,
			attachments, collateral_attachments, requested_date, created_at, updated_at, supervisor_approver_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'pending', $10, $11, $12, $13, $13, $13, $14) RETURNING *`,
			target, in.LoanType, numParam(in.Principal), numParam(rate), in.Tenor, numParam(monthly), numParam(total),
			textOrNil(in.Purpose), textOrNil(in.Notes), approver, nonNil(in.Attachments), nonNil(in.Collateral), now, supervisor)
		if err != nil {
			return err
		}
		return notifyLoanRequested(ctx, tx, emp, in.LoanType, in.Principal, in.Tenor, supervisor, approver)
	})
	return row, opaque(err)
}

// loanStages splits the approvers (EPIC-091): the nearest atasan from
// Supervisor up is a separate SPV stage only when a Manager above is someone
// else; otherwise whoever exists is the single atasan stage.
func loanStages(nearest, manager *string) (supervisor, approver *string) {
	switch {
	case nearest == nil:
		return nil, manager
	case manager == nil || *manager == *nearest:
		return nil, nearest
	default:
		return nearest, manager
	}
}

// loanTitle is the bell title of a request: "Pengajuan Kasbon: <nama>".
func loanTitle(loanType, name string) string {
	return "Pengajuan " + loanTypeLabel(loanType) + ": " + name
}

func loanWhat(name, loanType string, principal float64, tenor int) string {
	return fmt.Sprintf("%s mengajukan %s %s (%dx cicilan).", name, loanTypeLabel(loanType), domain.FormatRupiah(principal), tenor)
}

// notifyApprover rings employeeID's login account (if any) to decide.
func notifyApprover(ctx context.Context, q database.Querier, employeeID, title, message string) error {
	_, err := q.Exec(ctx, `SELECT public.notify_user(m.user_id, $2, $3, 'approval', '/dashboard/me/pinjaman')
		FROM hris.employees m WHERE m.id = $1 AND m.user_id IS NOT NULL`, employeeID, title, message)
	return err
}

// notifyLoanRequested is EPIC-091: the first stage approver (SPV, else the
// Manager) gets a bell to decide; the direct atasan, when not an approver
// (e.g. a Staff-level Store Leader), a bell to follow it in Persetujuan Tim.
func notifyLoanRequested(ctx context.Context, q database.Querier, emp *EmployeeBrief, loanType string,
	principal float64, tenor int, supervisor, approver *string) error {
	title := loanTitle(loanType, emp.FullName)
	what := loanWhat(emp.FullName, loanType, principal, tenor)
	var chain []string
	for _, id := range []*string{supervisor, approver} {
		if id == nil {
			continue
		}
		var name string
		if err := q.QueryRow(ctx, `SELECT full_name FROM hris.employees WHERE id = $1`, *id).Scan(&name); err != nil {
			return err
		}
		chain = append(chain, name)
	}
	first := supervisor
	if first == nil {
		first = approver
	}
	if first != nil {
		if err := notifyApprover(ctx, q, *first, title, what+" Menunggu persetujuan Anda di Area Karyawan → Pinjaman."); err != nil {
			return err
		}
	}
	decider := strings.Join(append(chain, "HR"), ", lalu ")
	_, err := q.Exec(ctx, `SELECT public.notify_user(a.user_id, $2, $3, 'approval', '/dashboard/me/persetujuan-tim?tab=riwayat')
		FROM hris.employees e JOIN hris.employees a ON a.id = e.reporting_to AND a.id <> e.id
		WHERE e.id = $1 AND a.user_id IS NOT NULL
		  AND ($4::uuid IS NULL OR a.id <> $4::uuid) AND ($5::uuid IS NULL OR a.id <> $5::uuid)`,
		emp.ID, title, what+" Diputuskan oleh "+decider+"; lihat detail di Persetujuan Tim → Riwayat.", supervisor, approver)
	return err
}

// nonNil is paths, or an empty list for a NOT NULL text[] column.
func nonNil(paths []string) []string {
	if paths == nil {
		return []string{}
	}
	return paths
}

// loanForDecision is the loan columns a decision reads.
type loanForDecision struct {
	EmployeeID, Status, LoanType string
	Principal                    float64
	Tenor                        int
	ManagerApprover              *string
	CreatedAt                    time.Time
	// SupervisorApprover is the SPV stage before the Manager (EPIC-091);
	// SupervisorDone once it approved.
	SupervisorApprover *string
	SupervisorDone     bool
}

// stageOf is who decides the pending loan now: the SPV until it approved,
// then the stored Manager (nil: HR only).
func (l *loanForDecision) stageOf() (approver *string, supervisorStage bool) {
	if l.SupervisorApprover != nil && !l.SupervisorDone {
		return l.SupervisorApprover, true
	}
	return l.ManagerApprover, false
}

func (s *service) loanForDecision(ctx context.Context, loanID string) (*loanForDecision, error) {
	var l loanForDecision
	err := s.db.QueryRow(ctx, `SELECT employee_id::text, COALESCE(status, ''), COALESCE(loan_type, ''),
		principal_amount::float8, tenor_months, manager_approver_id::text, COALESCE(created_at, requested_date, now()),
		supervisor_approver_id::text, supervisor_approved_at IS NOT NULL
		FROM hris.loans WHERE id::text = $1`, loanID).Scan(&l.EmployeeID, &l.Status, &l.LoanType,
		&l.Principal, &l.Tenor, &l.ManagerApprover, &l.CreatedAt, &l.SupervisorApprover, &l.SupervisorDone)
	if database.IsNoRows(err) {
		return nil, httpx.NotFound("Pinjaman tidak ditemukan")
	}
	return &l, err
}

// decideLoanManager is step 1 (EPIC-060, EPIC-076 R7): the stored manager
// approver moves a pending kasbon or loan to manager_approved, or rejects it.
// No salary figures.
func (s *service) decideLoanManager(ctx context.Context, approverEmployeeID, loanID string, approve bool, reason *string) (*messageData, error) {
	now := s.clock()
	var row *obj
	var err error
	if approve {
		row, err = queryObj(ctx, s.db, `UPDATE hris.loans SET status = 'manager_approved', manager_approved_by = $2,
			manager_approved_at = $3, updated_at = $3 WHERE id::text = $1 AND status = 'pending' AND manager_approver_id = $2
			AND (supervisor_approver_id IS NULL OR supervisor_approved_at IS NOT NULL)
			RETURNING id, employee_id, loan_type, status, tenor_months, manager_approver_id, manager_approved_by, manager_approved_at`,
			loanID, approverEmployeeID, now)
	} else {
		why := "Tidak disetujui"
		if reason != nil && *reason != "" {
			why = *reason
		}
		row, err = queryObj(ctx, s.db, `UPDATE hris.loans SET status = 'rejected', rejected_by = $2, rejected_at = $3,
			rejection_reason = $4, is_active = false, updated_at = $3 WHERE id::text = $1 AND status = 'pending'
			AND manager_approver_id = $2 AND (supervisor_approver_id IS NULL OR supervisor_approved_at IS NOT NULL)
			RETURNING id, employee_id, loan_type, status, rejection_reason`,
			loanID, approverEmployeeID, now, why)
	}
	if err != nil {
		return nil, opaque(err)
	}
	if row == nil {
		return nil, httpx.Conflict("Pinjaman sudah diproses oleh orang lain")
	}
	label := loanTypeLabel(row.Str("loan_type"))
	msg := label + " ditolak"
	if approve {
		msg = label + " disetujui Manager — menunggu persetujuan HR"
	}
	return &messageData{Data: row, Message: msg}, nil
}

// decideLoanSupervisor is the SPV stage (EPIC-091): approving leaves the
// loan pending for the Manager, who is rung; rejecting ends it.
func (s *service) decideLoanSupervisor(ctx context.Context, approverEmployeeID, loanID string, approve bool, reason *string) (*messageData, error) {
	now := s.clock()
	var row *obj
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		var err error
		if !approve {
			why := "Tidak disetujui"
			if reason != nil && *reason != "" {
				why = *reason
			}
			row, err = queryObj(ctx, tx, `UPDATE hris.loans SET status = 'rejected', rejected_by = $2, rejected_at = $3,
				rejection_reason = $4, is_active = false, updated_at = $3 WHERE id::text = $1 AND status = 'pending'
				AND supervisor_approver_id = $2 AND supervisor_approved_at IS NULL
				RETURNING id, employee_id, loan_type, status, rejection_reason`, loanID, approverEmployeeID, now, why)
			return err
		}
		row, err = queryObj(ctx, tx, `UPDATE hris.loans SET supervisor_approved_by = $2, supervisor_approved_at = $3,
			updated_at = $3 WHERE id::text = $1 AND status = 'pending' AND supervisor_approver_id = $2
			AND supervisor_approved_at IS NULL
			RETURNING id, employee_id, loan_type, status, tenor_months, principal_amount, manager_approver_id,
			supervisor_approver_id, supervisor_approved_by, supervisor_approved_at`, loanID, approverEmployeeID, now)
		if err != nil || row == nil || row.Str("manager_approver_id") == "" {
			return err
		}
		var name, spv string
		if err := tx.QueryRow(ctx, `SELECT e.full_name, s.full_name FROM hris.employees e, hris.employees s
			WHERE e.id = $1 AND s.id = $2`, row.Str("employee_id"), approverEmployeeID).Scan(&name, &spv); err != nil {
			return err
		}
		principal, _ := strconv.ParseFloat(row.Str("principal_amount"), 64)
		tenor, _ := strconv.Atoi(row.Str("tenor_months"))
		return notifyApprover(ctx, tx, row.Str("manager_approver_id"), loanTitle(row.Str("loan_type"), name),
			loanWhat(name, row.Str("loan_type"), principal, tenor)+" Disetujui SPV "+spv+". Menunggu persetujuan Anda di Area Karyawan → Pinjaman.")
	})
	if err != nil {
		return nil, opaque(err)
	}
	if row == nil {
		return nil, httpx.Conflict("Pinjaman sudah diproses oleh orang lain")
	}
	label := loanTypeLabel(row.Str("loan_type"))
	msg := label + " ditolak"
	if approve {
		msg = label + " disetujui SPV — menunggu persetujuan Manager"
	}
	return &messageData{Data: row, Message: msg}, nil
}

// decideLoanHR is the HR decision: step 2 for a loan with a manager
// approver, the only step without one (EPIC-060, EPIC-076 R7).
func (s *service) decideLoanHR(ctx context.Context, userID, loanID string, approve bool, reason *string, showCap bool) (*messageData, error) {
	approver, err := s.employeeOf(ctx, userID)
	if err != nil {
		return nil, err
	}
	l, err := s.loanForDecision(ctx, loanID)
	if err != nil {
		return nil, err
	}
	label := loanTypeLabel(l.LoanType)
	waitingManager := l.ManagerApprover != nil && l.Status == "pending"
	// HR may still reject a request that waits for its manager; only approve is blocked.
	if waitingManager && approve {
		return nil, httpx.BadRequest(label + " menunggu persetujuan Manager Departemen")
	}
	if l.Status != "pending" && l.Status != "manager_approved" {
		return nil, httpx.BadRequest("Pinjaman sudah diproses")
	}
	now := s.clock()
	var patch runPatch
	patch.set("updated_at", now)
	var month, year int
	if approve {
		config, count, err := s.loanLimits(ctx, l.EmployeeID, false)
		if err != nil {
			return nil, err
		}
		emp, err := s.brief(ctx, l.EmployeeID)
		if err != nil {
			return nil, err
		}
		if emp == nil {
			return nil, httpx.NotFound("Karyawan tidak ditemukan")
		}
		// Pending requests filed before EPIC-076 are held to the new terms
		// too; their attachments are not (R12).
		if err := s.checkLoanTnC(ctx, emp, l.LoanType, l.Principal, l.Tenor, count, config.LoanMaxActivePerPerson, showCap); err != nil {
			return nil, err
		}
		if month, year, err = s.loanFirstInstallment(ctx, l.CreatedAt); err != nil {
			return nil, err
		}
		patch.set("status", "approved")
		patch.set("approved_by", approver)
		patch.set("approved_at", now)
		patch.set("first_installment_month", month)
		patch.set("first_installment_year", year)
	} else {
		why := "Tidak disetujui"
		if reason != nil && *reason != "" {
			why = *reason
		}
		patch.set("status", "rejected")
		patch.set("rejected_by", approver)
		patch.set("rejected_at", now)
		patch.set("rejection_reason", why)
		patch.set("is_active", false)
	}
	sets := ""
	for i, c := range patch.cols {
		if i > 0 {
			sets += ", "
		}
		sets += c + " = $" + itoa(i+1)
	}
	args := append(patch.args, loanID, l.Status)
	row, err := queryObj(ctx, s.db, `UPDATE hris.loans SET `+sets+` WHERE id = $`+itoa(len(args)-1)+
		` AND status = $`+itoa(len(args))+` RETURNING *`, args...)
	if err != nil {
		return nil, opaque(err)
	}
	if row == nil {
		return nil, httpx.Conflict("Pinjaman sudah diproses oleh orang lain")
	}
	msg := label + " ditolak"
	if approve {
		msg = label + " disetujui — dipotong mulai payroll " + domain.PeriodLabelID(&month, year)
	}
	return &messageData{Data: row, Message: msg}, nil
}

// GET /api/hris/loans?employee_id&status&scope=approvals
func (h *handler) listLoans(w http.ResponseWriter, r *http.Request) error {
	a, err := h.requireActor(r)
	if err != nil {
		return err
	}
	q := queryForm(r)
	employee := q.Str("employee_id", optional, validate.StrOpts{})
	status := q.Str("status", optional, validate.StrOpts{})
	scope := q.Str("scope", optional, validate.StrOpts{})
	if err := issuesErr(q, ""); err != nil {
		return err
	}
	if scope != nil && *scope == "approvals" {
		rows, err := h.svc.listLoanApprovals(r.Context(), a)
		if err != nil {
			return err
		}
		return writeJSON(w, dataBody{rows})
	}
	rows, err := h.svc.listLoans(r.Context(), a, employee, status)
	if err != nil {
		return err
	}
	return writeJSON(w, dataBody{rows})
}

// POST /api/hris/loans
func (h *handler) createLoan(w http.ResponseWriter, r *http.Request) error {
	a, err := h.requireActor(r)
	if err != nil {
		return err
	}
	f, err := readJSON(r, "")
	if err != nil {
		return err
	}
	var in loanInput
	in.EmployeeID = f.Str("employee_id", nullish, validate.StrOpts{})
	before := len(f.Issues())
	loanType := f.Str("loan_type", validate.Rule{}, validate.StrOpts{Min: 1})
	for i := before; i < len(f.Issues()); i++ {
		f.Issues()[i].Message = invalidLoan
	}
	principal := coerceNum(f, "principal_amount", numRule{NumOpts: validate.NumOpts{Positive: true}, Message: invalidLoan})
	in.InterestRate, _ = field(f, "interest_rate")
	tenor := coerceInt(f, "tenor_months", numRule{NumOpts: validate.NumOpts{Min: validate.Bound(1), Max: validate.Bound(60)}, Message: invalidLoan})
	in.Purpose = f.Str("purpose", nullish, validate.StrOpts{})
	in.Notes = f.Str("notes", nullish, validate.StrOpts{})
	in.Attachments = f.Strings("attachments", optional, maxLoanAttachments+1, validate.StrOpts{})
	in.Collateral = f.Strings("collateral_attachments", optional, maxLoanAttachments+1, validate.StrOpts{})
	if err := issuesErr(f, ""); err != nil {
		return err
	}
	in.LoanType, in.Principal, in.Tenor = *loanType, *principal, *tenor
	row, err := h.svc.createLoan(r.Context(), a, in)
	if err != nil {
		return err
	}
	return writeJSON(w, messageData{Data: row, Message: "Pengajuan pinjaman berhasil dibuat, menunggu approval"})
}

// POST /api/hris/loans/{id}/approve { approved, rejection_reason? }: the stored
// manager approver decides step 1; HR with "approve" on Pinjaman (and, for
// kasbon, the Penggajian menu) decides the rest (EPIC-060, EPIC-076).
func (h *handler) decideLoan(w http.ResponseWriter, r *http.Request) error {
	a, err := h.requireActor(r)
	if err != nil {
		return err
	}
	f, err := readJSON(r, "")
	if err != nil {
		return err
	}
	approved := f.Bool("approved", optional)
	reason := f.Str("rejection_reason", nullish, validate.StrOpts{})
	if err := issuesErr(f, ""); err != nil {
		return err
	}
	loanID := r.PathValue("id")
	l, err := h.svc.loanForDecision(r.Context(), loanID)
	if err != nil {
		return err
	}
	if stage, supervisorStage := l.stageOf(); l.Status == "pending" && stage != nil && a.EmployeeID != nil && *a.EmployeeID == *stage {
		decide := h.svc.decideLoanManager
		if supervisorStage {
			decide = h.svc.decideLoanSupervisor
		}
		out, err := decide(r.Context(), *a.EmployeeID, loanID, approved != nil && *approved, reason)
		if err != nil {
			return err
		}
		return writeJSON(w, out)
	}
	if a.EmployeeID != nil && ((l.ManagerApprover != nil && *a.EmployeeID == *l.ManagerApprover) ||
		(l.SupervisorApprover != nil && *a.EmployeeID == *l.SupervisorApprover)) && l.Status == "pending" {
		// The other atasan stage is not theirs yet (or any more).
		return httpx.Forbidden("Bukan giliran Anda memutus pengajuan ini")
	}
	u, err := h.auth.RequireMenuAction(r, "approve", iam.HrisLoans...)
	if err != nil {
		return err
	}
	if l.LoanType == "kasbon" && !a.Payroll {
		return httpx.Forbidden("Persetujuan kasbon oleh HR butuh akses Penggajian")
	}
	// The cap reveals the base salary: only Penggajian sees it (EPIC-057).
	showCap, err := h.auth.HasMenuPrefix(r.Context(), u, iam.HrisCompensation...)
	if err != nil {
		return err
	}
	out, err := h.svc.decideLoanHR(r.Context(), u.ID, loanID, approved != nil && *approved, reason, showCap)
	if err != nil {
		return err
	}
	return writeJSON(w, out)
}

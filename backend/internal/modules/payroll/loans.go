package payroll

import (
	"context"
	"math"
	"net/http"
	"slices"

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

// kasbonTHP is the kasbon THP (EPIC-060 R2): base + transport + meal +
// position allowance over every active salary row; nil without one.
func (s *service) kasbonTHP(ctx context.Context, employeeID string) (*float64, error) {
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

// listLoanApprovals is the manager queue: pending kasbon whose stored
// approver is the actor. Salary-free columns only (R8).
func (s *service) listLoanApprovals(ctx context.Context, a *actor) ([]*obj, error) {
	if a.EmployeeID == nil {
		return []*obj{}, nil
	}
	rows, err := queryObjs(ctx, s.db, `SELECT id, employee_id, loan_type, principal_amount, tenor_months, purpose, notes,
		status, created_at FROM hris.loans WHERE status = 'pending' AND manager_approver_id = $1 ORDER BY created_at`,
		*a.EmployeeID)
	if err != nil {
		return nil, opaque(err)
	}
	return rows, s.stitch(ctx, s.db, rows, loanListPeople[0])
}

// loanLimits is loadLoanLimitContext: the configured limits, the active
// base salary (nil without one) and the loans that count against the limit.
func (s *service) loanLimits(ctx context.Context, employeeID string, includePending bool) (domain.Config, *float64, float64, error) {
	settings, tax, err := payrollConfigRows(ctx, s.db, s.clock().Year())
	if err != nil {
		return domain.Config{}, nil, 0, err
	}
	var base *float64
	// Total over every active row, including one with no entity yet.
	if err := s.db.QueryRow(ctx, `SELECT sum(base_salary)::float8 FROM hris.employee_salary
		WHERE employee_id = $1 AND is_active`, employeeID).Scan(&base); err != nil {
		return domain.Config{}, nil, 0, err
	}
	filter := `status IN ('pending', 'manager_approved', 'approved')`
	if !includePending {
		filter = `status = 'approved' AND remaining_balance > 0`
	}
	var count int
	err = s.db.QueryRow(ctx, `SELECT count(*) FROM hris.loans WHERE employee_id = $1 AND is_active = true AND `+filter+`
		AND (status IN ('pending', 'manager_approved') OR remaining_balance > 0)`, employeeID).Scan(&count)
	return domain.BuildConfig(settings, tax), base, float64(count), err
}

type loanInput struct {
	EmployeeID   *string
	LoanType     string
	Principal    float64
	InterestRate any
	Tenor        int
	Purpose      *string
	Notes        *string
}

func (s *service) createLoan(ctx context.Context, a *actor, in loanInput) (*obj, error) {
	full := domain.HasLoanFullAccess(a.Role)
	var target string
	if full && in.EmployeeID != nil && *in.EmployeeID != "" && *in.EmployeeID != "me" {
		target = *in.EmployeeID
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
	raw, monthly, total := domain.LoanTerms(in.Principal, rate, in.Tenor)
	config, base, count, err := s.loanLimits(ctx, target, true)
	if err != nil {
		return nil, err
	}
	showCap := a.Payroll || (a.EmployeeID != nil && *a.EmployeeID == target)
	var msg string
	if in.LoanType == "kasbon" {
		thp, err := s.kasbonTHP(ctx, target)
		if err != nil {
			return nil, err
		}
		msg = domain.ValidateKasbon(in.Principal, in.Tenor, emp.EmploymentStatus, thp, count, config.LoanMaxActivePerPerson, showCap)
	} else {
		msg = domain.ValidateLoanLimitsFor(raw, base, config.LoanMaxInstallmentPct, count, config.LoanMaxActivePerPerson, showCap)
	}
	if msg != "" {
		return nil, httpx.BadRequest(msg)
	}
	// Kasbon step 1 is the Manager up the reporting chain (EPIC-060, AP-05).
	var approver *string
	if in.LoanType == "kasbon" {
		if approver, err = s.ports.Employees.LoanApprover(ctx, s.db, target); err != nil {
			return nil, err
		}
	}
	row, err := queryObj(ctx, s.db, `INSERT INTO hris.loans (employee_id, loan_type, principal_amount, interest_rate,
		tenor_months, monthly_installment, remaining_balance, purpose, notes, status, manager_approver_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'pending', $10) RETURNING *`,
		target, in.LoanType, numParam(in.Principal), numParam(rate), in.Tenor, numParam(monthly), numParam(total),
		textOrNil(in.Purpose), textOrNil(in.Notes), approver)
	return row, opaque(err)
}

// loanForDecision is the loan columns a decision reads.
type loanForDecision struct {
	EmployeeID, Status, LoanType string
	Installment                  *string
	Principal                    float64
	Tenor                        int
	ManagerApprover              *string
}

func (s *service) loanForDecision(ctx context.Context, loanID string) (*loanForDecision, error) {
	var l loanForDecision
	err := s.db.QueryRow(ctx, `SELECT employee_id::text, COALESCE(status, ''), COALESCE(loan_type, ''),
		monthly_installment::text, principal_amount::float8, tenor_months, manager_approver_id::text
		FROM hris.loans WHERE id::text = $1`, loanID).Scan(&l.EmployeeID, &l.Status, &l.LoanType, &l.Installment,
		&l.Principal, &l.Tenor, &l.ManagerApprover)
	if database.IsNoRows(err) {
		return nil, httpx.NotFound("Pinjaman tidak ditemukan")
	}
	return &l, err
}

// decideLoanManager is step 1 (EPIC-060): the stored manager approver moves a
// pending kasbon to manager_approved, or rejects it. No salary figures.
func (s *service) decideLoanManager(ctx context.Context, approverEmployeeID, loanID string, approve bool, reason *string) (*messageData, error) {
	now := s.clock()
	var row *obj
	var err error
	if approve {
		row, err = queryObj(ctx, s.db, `UPDATE hris.loans SET status = 'manager_approved', manager_approved_by = $2,
			manager_approved_at = $3, updated_at = $3 WHERE id::text = $1 AND status = 'pending' AND manager_approver_id = $2
			RETURNING id, employee_id, loan_type, status, tenor_months, manager_approver_id, manager_approved_by, manager_approved_at`,
			loanID, approverEmployeeID, now)
	} else {
		why := "Tidak disetujui"
		if reason != nil && *reason != "" {
			why = *reason
		}
		row, err = queryObj(ctx, s.db, `UPDATE hris.loans SET status = 'rejected', rejected_by = $2, rejected_at = $3,
			rejection_reason = $4, is_active = false, updated_at = $3 WHERE id::text = $1 AND status = 'pending'
			AND manager_approver_id = $2 RETURNING id, employee_id, loan_type, status, rejection_reason`,
			loanID, approverEmployeeID, now, why)
	}
	if err != nil {
		return nil, opaque(err)
	}
	if row == nil {
		return nil, httpx.Conflict("Pinjaman sudah diproses oleh orang lain")
	}
	msg := "Kasbon ditolak"
	if approve {
		msg = "Kasbon disetujui Manager — menunggu persetujuan HR"
	}
	return &messageData{Data: row, Message: msg}, nil
}

// decideLoanHR is the HR decision: the only step for a loan, step 2 for a
// kasbon that has a manager approver (EPIC-060).
func (s *service) decideLoanHR(ctx context.Context, userID, loanID string, approve bool, reason *string, showCap bool) (*messageData, error) {
	approver, err := s.employeeOf(ctx, userID)
	if err != nil {
		return nil, err
	}
	l, err := s.loanForDecision(ctx, loanID)
	if err != nil {
		return nil, err
	}
	employeeID, loanType, installment, principal, tenor := l.EmployeeID, l.LoanType, l.Installment, l.Principal, l.Tenor
	waitingManager := l.LoanType == "kasbon" && l.ManagerApprover != nil && l.Status == "pending"
	// HR may still reject a kasbon that waits for its manager; only approve is blocked.
	if waitingManager && approve {
		return nil, httpx.BadRequest("Kasbon menunggu persetujuan Manager Departemen")
	}
	if l.Status != "pending" && l.Status != "manager_approved" {
		return nil, httpx.BadRequest("Pinjaman sudah diproses")
	}
	now := s.clock()
	var patch runPatch
	patch.set("updated_at", now)
	if approve {
		config, base, count, err := s.loanLimits(ctx, employeeID, false)
		if err != nil {
			return nil, err
		}
		var msg string
		if loanType == "kasbon" {
			emp, err := s.brief(ctx, employeeID)
			if err != nil {
				return nil, err
			}
			if emp == nil {
				return nil, httpx.NotFound("Karyawan tidak ditemukan")
			}
			thp, err := s.kasbonTHP(ctx, employeeID)
			if err != nil {
				return nil, err
			}
			msg = domain.ValidateKasbon(principal, tenor, emp.EmploymentStatus, thp, count, config.LoanMaxActivePerPerson, showCap)
		} else {
			msg = domain.ValidateLoanLimitsFor(domain.JSNumber(textOrNil(installment)), base, config.LoanMaxInstallmentPct,
				count, config.LoanMaxActivePerPerson, showCap)
		}
		if msg != "" {
			return nil, httpx.BadRequest(msg)
		}
		month, year := domain.FirstInstallmentPeriod(now)
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
	msg := "Pinjaman ditolak"
	if approve {
		msg = "Pinjaman disetujui — cicilan mulai bulan depan"
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
// manager approver decides kasbon step 1; HR with "approve" on Pinjaman (and,
// for kasbon, the Penggajian menu) decides the rest (EPIC-060).
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
	if l.Status == "pending" && l.ManagerApprover != nil && a.EmployeeID != nil && *a.EmployeeID == *l.ManagerApprover {
		out, err := h.svc.decideLoanManager(r.Context(), *a.EmployeeID, loanID, approved != nil && *approved, reason)
		if err != nil {
			return err
		}
		return writeJSON(w, out)
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

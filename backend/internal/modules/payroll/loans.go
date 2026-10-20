package payroll

import (
	"context"
	"math"
	"net/http"

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
	filter := `status IN ('pending', 'approved')`
	if !includePending {
		filter = `status = 'approved' AND remaining_balance > 0`
	}
	var count int
	err = s.db.QueryRow(ctx, `SELECT count(*) FROM hris.loans WHERE employee_id = $1 AND is_active = true AND `+filter+`
		AND (status = 'pending' OR remaining_balance > 0)`, employeeID).Scan(&count)
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
	raw, monthly, total := domain.LoanTerms(in.Principal, rate, in.Tenor)
	config, base, count, err := s.loanLimits(ctx, target, true)
	if err != nil {
		return nil, err
	}
	showCap := a.Payroll || (a.EmployeeID != nil && *a.EmployeeID == target)
	if msg := domain.ValidateLoanLimitsFor(raw, base, config.LoanMaxInstallmentPct, count, config.LoanMaxActivePerPerson, showCap); msg != "" {
		return nil, httpx.BadRequest(msg)
	}
	row, err := queryObj(ctx, s.db, `INSERT INTO hris.loans (employee_id, loan_type, principal_amount, interest_rate,
		tenor_months, monthly_installment, remaining_balance, purpose, notes, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'pending') RETURNING *`,
		target, in.LoanType, numParam(in.Principal), numParam(rate), in.Tenor, numParam(monthly), numParam(total),
		textOrNil(in.Purpose), textOrNil(in.Notes))
	return row, opaque(err)
}

func (s *service) decideLoan(ctx context.Context, userID, loanID string, approve bool, reason *string, showCap bool) (*messageData, error) {
	approver, err := s.employeeOf(ctx, userID)
	if err != nil {
		return nil, err
	}
	var employeeID, status string
	var installment *string
	err = s.db.QueryRow(ctx, `SELECT employee_id::text, COALESCE(status, ''), monthly_installment::text
		FROM hris.loans WHERE id::text = $1`, loanID).Scan(&employeeID, &status, &installment)
	if database.IsNoRows(err) {
		return nil, httpx.NotFound("Pinjaman tidak ditemukan")
	}
	if err != nil {
		return nil, err
	}
	if status != "pending" {
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
		if msg := domain.ValidateLoanLimitsFor(domain.JSNumber(textOrNil(installment)), base, config.LoanMaxInstallmentPct,
			count, config.LoanMaxActivePerPerson, showCap); msg != "" {
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
	args := append(patch.args, loanID)
	row, err := queryObj(ctx, s.db, `UPDATE hris.loans SET `+sets+` WHERE id = $`+itoa(len(args))+
		` AND status = 'pending' RETURNING *`, args...)
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

// GET /api/hris/loans?employee_id&status
func (h *handler) listLoans(w http.ResponseWriter, r *http.Request) error {
	a, err := h.requireActor(r)
	if err != nil {
		return err
	}
	q := queryForm(r)
	employee := q.Str("employee_id", optional, validate.StrOpts{})
	status := q.Str("status", optional, validate.StrOpts{})
	if err := issuesErr(q, ""); err != nil {
		return err
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

// POST /api/hris/loans/{id}/approve { approved, rejection_reason? }: the
// "approve" action on Pinjaman, not just the menu (read-only holders list).
func (h *handler) decideLoan(w http.ResponseWriter, r *http.Request) error {
	u, err := h.auth.RequireMenuAction(r, "approve", iam.HrisLoans...)
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
	// The cap reveals the base salary: only Penggajian sees it (EPIC-057).
	showCap, err := h.auth.HasMenuPrefix(r.Context(), u, iam.HrisCompensation...)
	if err != nil {
		return err
	}
	out, err := h.svc.decideLoan(r.Context(), u.ID, r.PathValue("id"), approved != nil && *approved, reason, showCap)
	if err != nil {
		return err
	}
	return writeJSON(w, out)
}

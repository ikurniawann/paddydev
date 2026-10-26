package hris

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/validate"
)

// Leave balances (EPIC-068): the per-year annual allocation HR maintains.
// A balance is total + adjustment - used (annual_leave_remaining) plus an
// optional carry-over that is only usable until its expiry date (R4).

// saldoCuti is the prefix of the Saldo Cuti leaf for the action guards.
var saldoCuti = []string{iam.LeafSaldoCuti}

// maxLeaveDays bounds one balance field so the numeric(5,2) column cannot
// overflow into a database error.
const maxLeaveDays = 366

// LeaveBalanceView is a balance as every reader sees it (R12): the single
// GET, the list, ESS and the PUT answer. Rows that do not exist yet carry
// the default allocation and Exists false.
type LeaveBalanceView struct {
	EmployeeID           string     `json:"employee_id"`
	NIP                  *string    `json:"nip"`
	FullName             string     `json:"full_name"`
	BranchName           *string    `json:"branch_name"`
	Year                 int        `json:"year"`
	AnnualLeaveTotal     float64    `json:"annual_leave_total"`
	AnnualLeaveAdjust    float64    `json:"annual_leave_adjustment"`
	AnnualLeaveUsed      float64    `json:"annual_leave_used"`
	AnnualLeaveRemaining float64    `json:"annual_leave_remaining"`
	CarryOverDays        float64    `json:"carry_over_days"`
	CarryOverUsed        float64    `json:"carry_over_used"`
	CarryOverRemaining   float64    `json:"carry_over_remaining"`
	CarryOverExpiresOn   *string    `json:"carry_over_expires_on"`
	CarryOverActive      bool       `json:"carry_over_active"`
	Available            float64    `json:"available"`
	UpdatedAt            *time.Time `json:"updated_at"`
	Exists               bool       `json:"exists"`
}

// LeaveBalanceInput is what HR sets on one balance (R11); Used is never
// part of it.
type LeaveBalanceInput struct {
	Year                         int
	Total, Adjustment, CarryDays float64
	CarryExpires                 *string
	Reason                       string
	// Keep* mark a figure the caller left out: UpdateLeaveBalanceManual takes
	// the stored value under the row lock instead of a possibly stale read.
	KeepTotal, KeepAdjustment, KeepCarryDays, KeepCarryExpires bool
}

// balanceRaw is the editable part of a stored balance plus its usage.
type balanceRaw struct {
	Total, Adjustment, Used, CarryDays, CarryUsed float64
	CarryExpires                                  *string
	// Source is carry_over_source: 'auto' (recalculated) or 'manual' (HR).
	Source string
}

// LeaveBalanceRepo is the leave-balance storage.
type LeaveBalanceRepo interface {
	// AnnualAvailable is R4 on onDate for the stored row; found is false
	// when the employee has no row for the year.
	AnnualAvailable(ctx context.Context, employeeID string, year int, onDate string) (available float64, found bool, err error)
	// LeaveBalanceViews reads the balances of employees (all active ones, or
	// the one employeeID when set) for year; today decides what is active.
	LeaveBalanceViews(ctx context.Context, f balanceFilter) ([]LeaveBalanceView, error)
	// LockLeaveBalance reads a stored row FOR UPDATE; nil when missing.
	LockLeaveBalance(ctx context.Context, employeeID string, year int) (*balanceRaw, error)
	// SetLeaveBalance writes the editable columns of a stored row.
	SetLeaveBalance(ctx context.Context, employeeID string, year int, in LeaveBalanceInput) error
}

// balanceFilter scopes LeaveBalanceViews.
type balanceFilter struct {
	Year       int
	Today      string // YYYY-MM-DD (WIB)
	EmployeeID string // one employee (active or not) instead of the active list
	BranchID   string
	Search     string
}

const (
	ensureBalanceSQL = `INSERT INTO hris.leave_balances (employee_id, year, annual_leave_total)
		SELECT e.id, $2::int, hris.default_annual_leave(e.join_date, $2::int) FROM hris.employees e WHERE e.id = $1
		ON CONFLICT (employee_id, year) DO NOTHING`

	// availableSQL is R4: remaining plus the carry-over that has not expired
	// on $3 (no expiry date: the carry-over does not apply). A carry-over
	// recalculated below what is used counts as 0, never negative.
	availableSQL = `SELECT (annual_leave_remaining + CASE WHEN carry_over_expires_on >= $3::date
			THEN GREATEST(carry_over_days - carry_over_used, 0) ELSE 0 END)::float8
		FROM hris.leave_balances WHERE employee_id = $1 AND year = $2`
)

// minBalanceYear is the leave_balance_year_valid CHECK.
const minBalanceYear = 2020

// ensureLeaveBalance creates the employee's row for year with the default
// allocation (R1) unless it exists; a missing employee creates nothing.
func ensureLeaveBalance(ctx context.Context, q database.Querier, employeeID string, year int) error {
	if year < minBalanceYear {
		return nil
	}
	_, err := q.Exec(ctx, ensureBalanceSQL, employeeID, year)
	return ignoreBadInput(err)
}

// EnsureLeaveBalance makes sure the employee has a balance row for year,
// created with the default allocation (R1) when missing. Existing rows are
// never recomputed (R2). q is the caller's pool or transaction.
func (s *Service) EnsureLeaveBalance(ctx context.Context, q database.Querier, employeeID string, year int) error {
	return ensureLeaveBalance(ctx, q, employeeID, year)
}

/* ── Storage ─────────────────────────────────────────────────────────── */

func (s *store) AnnualAvailable(ctx context.Context, employeeID string, year int, onDate string) (float64, bool, error) {
	var v float64
	err := s.db.QueryRow(ctx, availableSQL, employeeID, year, onDate).Scan(&v)
	if database.IsNoRows(err) || database.PgCode(err) == "22P02" {
		return 0, false, nil
	}
	return v, err == nil, err
}

func (s *store) LeaveBalanceViews(ctx context.Context, f balanceFilter) ([]LeaveBalanceView, error) {
	return leaveBalanceViews(ctx, s.db, f)
}

// leaveBalanceViews reads balances on q. The branch is the employee's primary
// running placement (any running one without a primary), as of f.Today.
func leaveBalanceViews(ctx context.Context, q database.Querier, f balanceFilter) ([]LeaveBalanceView, error) {
	var branch, employee, search *string
	if f.EmployeeID != "" {
		employee = &f.EmployeeID
	}
	if f.BranchID != "" {
		branch = &f.BranchID
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		pattern := "%" + s + "%"
		search = &pattern
	}
	rows, err := q.Query(ctx, `SELECT e.id::text, e.nip, e.full_name, pb.branch_name, b.id IS NOT NULL,
		COALESCE(b.annual_leave_total, hris.default_annual_leave(e.join_date, $1::int))::float8,
		COALESCE(b.annual_leave_adjustment, 0)::float8,
		COALESCE(b.annual_leave_used, 0)::float8,
		COALESCE(b.annual_leave_remaining, hris.default_annual_leave(e.join_date, $1::int))::float8,
		COALESCE(b.carry_over_days, 0)::float8, COALESCE(b.carry_over_used, 0)::float8,
		b.carry_over_expires_on::text, b.updated_at
		FROM hris.employees e
		LEFT JOIN hris.leave_balances b ON b.employee_id = e.id AND b.year = $1::int
		LEFT JOIN LATERAL (SELECT ep.branch_id, br.name AS branch_name
			FROM hris.employee_placements ep JOIN configuration.branches br ON br.id = ep.branch_id
			WHERE ep.employee_id = e.id AND ep.start_date <= $2::date AND (ep.end_date IS NULL OR ep.end_date >= $2::date)
			ORDER BY ep.is_primary DESC, ep.start_date DESC, br.name LIMIT 1) pb ON true
		WHERE (e.id = $3::uuid OR ($3::uuid IS NULL AND e.is_active))
		  AND ($4::uuid IS NULL OR pb.branch_id = $4::uuid)
		  AND ($5::text IS NULL OR e.full_name ILIKE $5 OR e.nip ILIKE $5)
		ORDER BY e.full_name, e.nip`, f.Year, f.Today, employee, branch, search)
	if err != nil {
		return nil, ignoreBadInput(err)
	}
	defer rows.Close()
	out := []LeaveBalanceView{}
	for rows.Next() {
		v := LeaveBalanceView{Year: f.Year}
		if err := rows.Scan(&v.EmployeeID, &v.NIP, &v.FullName, &v.BranchName, &v.Exists,
			&v.AnnualLeaveTotal, &v.AnnualLeaveAdjust, &v.AnnualLeaveUsed, &v.AnnualLeaveRemaining,
			&v.CarryOverDays, &v.CarryOverUsed, &v.CarryOverExpiresOn, &v.UpdatedAt); err != nil {
			return nil, err
		}
		v.finish(f.Today)
		out = append(out, v)
	}
	return out, rows.Err()
}

// finish derives the carry-over remainder, whether it still applies on today
// and the available quota (R4).
func (v *LeaveBalanceView) finish(today string) {
	v.CarryOverRemaining = max(v.CarryOverDays-v.CarryOverUsed, 0) // below used (a review is open): 0, not negative
	v.CarryOverActive = v.CarryOverExpiresOn != nil && *v.CarryOverExpiresOn >= today
	v.Available = v.AnnualLeaveRemaining
	if v.CarryOverActive {
		v.Available += v.CarryOverRemaining
	}
}

func (s *store) LockLeaveBalance(ctx context.Context, employeeID string, year int) (*balanceRaw, error) {
	var b balanceRaw
	err := s.db.QueryRow(ctx, `SELECT annual_leave_total::float8, annual_leave_adjustment::float8, annual_leave_used::float8,
		carry_over_days::float8, carry_over_used::float8, carry_over_expires_on::text, carry_over_source
		FROM hris.leave_balances WHERE employee_id = $1 AND year = $2 FOR UPDATE`, employeeID, year).
		Scan(&b.Total, &b.Adjustment, &b.Used, &b.CarryDays, &b.CarryUsed, &b.CarryExpires, &b.Source)
	if database.IsNoRows(err) {
		return nil, nil
	}
	return &b, err
}

// SetLeaveBalance also keeps carry_over_source: a carry-over or expiry HR
// changes becomes 'manual' (the recalculation leaves it alone); clearing both
// (0 and no expiry) hands the row back to the formula ('auto').
func (s *store) SetLeaveBalance(ctx context.Context, employeeID string, year int, in LeaveBalanceInput) error {
	_, err := s.db.Exec(ctx, `UPDATE hris.leave_balances
		SET annual_leave_total = $3, annual_leave_adjustment = $4,
		    carry_over_source = CASE
		      WHEN $5::numeric = 0 AND $6::date IS NULL THEN 'auto'
		      WHEN $5::numeric IS DISTINCT FROM carry_over_days OR $6::date IS DISTINCT FROM carry_over_expires_on THEN 'manual'
		      ELSE carry_over_source END,
		    carry_over_days = $5::numeric, carry_over_expires_on = $6::date, updated_at = now()
		WHERE employee_id = $1 AND year = $2`,
		employeeID, year, in.Total, in.Adjustment, in.CarryDays, in.CarryExpires)
	return err
}

/* ── Validation ──────────────────────────────────────────────────────── */

// daysText formats a number of days the Indonesian way (9,5).
func daysText(v float64) string {
	return strings.Replace(strconv.FormatFloat(v, 'f', -1, 64), ".", ",", 1)
}

func isHalfStep(v float64) bool { return math.Mod(v*2, 1) == 0 }

// validateLeaveBalance checks an HR-entered allocation (manual PUT and the
// Excel import): the employee is active, the year is this year or next
// (nowYear is the WIB year), a reason is given, every figure is a multiple
// of 0,5, and neither the annual remainder nor the carry-over falls below
// what is already used. The carry-over rule applies only when the request
// changes the carry-over days or expiry (carryChanged): a row the automatic
// recalculation left below used (a review is open) can still have its total or
// adjustment edited.
func validateLeaveBalance(in LeaveBalanceInput, used, carryUsed float64, active, carryChanged bool, nowYear int) error {
	if !active {
		return httpx.BadRequest("Karyawan nonaktif — saldo cuti tidak bisa diubah")
	}
	if in.Year != nowYear && in.Year != nowYear+1 {
		return httpx.BadRequest(fmt.Sprintf("Tahun saldo hanya boleh %d (tahun berjalan) atau %d (tahun depan)", nowYear, nowYear+1))
	}
	if strings.TrimSpace(in.Reason) == "" {
		return httpx.BadRequest("Alasan perubahan wajib diisi")
	}
	if len([]rune(strings.TrimSpace(in.Reason))) > 500 {
		return httpx.BadRequest("Alasan terlalu panjang (maksimal 500 karakter)")
	}
	for _, c := range []struct {
		label string
		v     float64
		signs bool // may be negative
	}{{"Jatah", in.Total, false}, {"Penyesuaian", in.Adjustment, true}, {"Sisa bawaan", in.CarryDays, false}} {
		if math.IsNaN(c.v) || math.IsInf(c.v, 0) || !isHalfStep(c.v) {
			return httpx.BadRequest(c.label + " harus kelipatan 0,5")
		}
		if !c.signs && c.v < 0 {
			return httpx.BadRequest(c.label + " tidak boleh negatif")
		}
		if math.Abs(c.v) > maxLeaveDays {
			return httpx.BadRequest(fmt.Sprintf("%s tidak boleh lebih dari %d hari", c.label, maxLeaveDays))
		}
	}
	if remaining := in.Total + in.Adjustment - used; remaining < 0 {
		return httpx.BadRequest(fmt.Sprintf("Sisa jatah tidak boleh kurang dari 0 (jatah %s + penyesuaian %s − terpakai %s = %s)",
			daysText(in.Total), daysText(in.Adjustment), daysText(used), daysText(remaining)))
	}
	if carryChanged && in.CarryDays < carryUsed {
		return httpx.BadRequest(fmt.Sprintf("Sisa bawaan (%s) tidak boleh kurang dari bawaan yang sudah terpakai (%s)",
			daysText(in.CarryDays), daysText(carryUsed)))
	}
	if in.CarryExpires != nil {
		if _, ok := domain.ParseDate(*in.CarryExpires); !ok {
			return httpx.BadRequest("Tanggal kedaluwarsa sisa bawaan tidak valid (format YYYY-MM-DD)")
		}
	}
	return nil
}

/* ── Use cases ───────────────────────────────────────────────────────── */

// mayReadLeaveBalance: the owner, an account holding "read" on Saldo Cuti, or
// one holding a Karyawan menu (R7).
func mayReadLeaveBalance(a *Actor, employeeID string) bool {
	return (a.EmployeeID != nil && *a.EmployeeID == employeeID) || a.LeaveBalanceAction("read") || a.EmployeeRecords
}

// LeaveBalanceView is the balance of one employee for year (R12). The row is
// created with the default allocation on first read (R1) for an active
// employee and a year up to next year; the reader must be the owner, hold "read" on Saldo Cuti or a
// Karyawan menu.
func (s *Service) LeaveBalanceView(ctx context.Context, a *Actor, employeeID string, year int) (LeaveBalanceView, error) {
	if !mayReadLeaveBalance(a, employeeID) {
		return LeaveBalanceView{}, httpx.Forbidden("Forbidden: Can only view own leave balance")
	}
	v, found, err := s.leaveBalanceOf(ctx, employeeID, year)
	if err != nil {
		return LeaveBalanceView{}, err
	}
	if !found {
		return LeaveBalanceView{}, httpx.NotFound("Employee not found")
	}
	if v.Exists {
		return v, nil
	}
	// Only an active employee gets a row, and only for this or next year; any
	// other read shows the default allocation without writing it.
	emp, err := s.repo.LeaveEmployee(ctx, employeeID)
	if err != nil {
		return LeaveBalanceView{}, err
	}
	if emp == nil || !emp.Bool("is_active") || year > domain.NowWIB(s.now()).Year()+1 {
		return v, nil
	}
	if err := s.EnsureLeaveBalance(ctx, s.repo.querier(), employeeID, year); err != nil {
		return LeaveBalanceView{}, err
	}
	v, found, err = s.leaveBalanceOf(ctx, employeeID, year)
	if err != nil {
		return LeaveBalanceView{}, err
	}
	if !found {
		return LeaveBalanceView{}, httpx.NotFound("Employee not found")
	}
	return v, nil
}

// leaveBalanceOf reads one employee's view for year as of today (WIB).
func (s *Service) leaveBalanceOf(ctx context.Context, employeeID string, year int) (LeaveBalanceView, bool, error) {
	views, err := s.repo.LeaveBalanceViews(ctx, balanceFilter{Year: year, Today: domain.TodayWIB(s.now()), EmployeeID: employeeID})
	if err != nil || len(views) == 0 {
		return LeaveBalanceView{}, false, err
	}
	return views[0], true, nil
}

// ListLeaveBalances lists the active employees' balances for year, narrowed
// to a branch (primary placement) and a name/NIP search. Rows that do not
// exist yet show the default allocation and are not written.
func (s *Service) ListLeaveBalances(ctx context.Context, year int, branchID, q string) ([]LeaveBalanceView, error) {
	return s.repo.LeaveBalanceViews(ctx, balanceFilter{Year: year, Today: domain.TodayWIB(s.now()), BranchID: branchID, Search: q})
}

// UpdateLeaveBalanceManual sets one balance's allocation (R11): the used
// figures stay as they are. The row is created first when missing; the change
// and its audit row (leave_balance.update with before/after and the reason)
// commit together. meta carries the request's IP and user agent.
func (s *Service) UpdateLeaveBalanceManual(ctx context.Context, a *Actor, employeeID string, in LeaveBalanceInput, meta ...audit.Entry) (LeaveBalanceView, error) {
	emp, err := s.repo.LeaveEmployee(ctx, employeeID)
	if err != nil {
		return LeaveBalanceView{}, err
	}
	if emp == nil {
		return LeaveBalanceView{}, httpx.NotFound("Karyawan tidak ditemukan")
	}
	nowYear := domain.NowWIB(s.now()).Year()
	if in.Year < minBalanceYear { // the year check below refuses it; do not touch the table
		return LeaveBalanceView{}, validateLeaveBalance(in, 0, 0, emp.Bool("is_active"), false, nowYear)
	}
	var entry audit.Entry
	if len(meta) > 0 {
		entry = meta[0]
	}
	err = s.repo.InTx(ctx, func(r Repository) error {
		// Ascending lock order (previous year, this year, next year).
		if err := lockPreviousBalance(ctx, r.querier(), employeeID, in.Year); err != nil {
			return err
		}
		if err := ensureLeaveBalance(ctx, r.querier(), employeeID, in.Year); err != nil {
			return err
		}
		was, err := r.LockLeaveBalance(ctx, employeeID, in.Year)
		if err != nil {
			return err
		}
		if was == nil {
			return httpx.NotFound("Karyawan tidak ditemukan")
		}
		if in.KeepTotal {
			in.Total = was.Total
		}
		if in.KeepAdjustment {
			in.Adjustment = was.Adjustment
		}
		if in.KeepCarryDays {
			in.CarryDays = was.CarryDays
		}
		if in.KeepCarryExpires {
			in.CarryExpires = was.CarryExpires
		}
		if err := validateLeaveBalance(in, was.Used, was.CarryUsed, emp.Bool("is_active"),
			in.CarryDays != was.CarryDays || !sameText(in.CarryExpires, was.CarryExpires), nowYear); err != nil {
			return err
		}
		if err := r.SetLeaveBalance(ctx, employeeID, in.Year, in); err != nil {
			return err
		}
		reason := strings.TrimSpace(in.Reason)
		label := fmt.Sprintf("%s · Saldo cuti %d", emp.Str("full_name"), in.Year)
		entry.ActorID, entry.ActorName = a.UserID, ptr(actorName(a))
		entry.Action, entry.Entity, entry.EntityID, entry.EntityLabel = "leave_balance.update", "hris.employees", &employeeID, &label
		entry.Before = balanceAudit(in.Year, *was)
		after := *was
		after.Total, after.Adjustment, after.CarryDays, after.CarryExpires = in.Total, in.Adjustment, in.CarryDays, in.CarryExpires
		entry.After = balanceAudit(in.Year, after)
		entry.Reason = &reason
		if err := audit.Write(ctx, r.querier(), entry); err != nil {
			return err
		}
		return s.carryAfterHRInput(ctx, r.querier(), employeeID, in, was.Source, userOf(a), carryCause{Type: causeHRUpdate})
	})
	if err != nil {
		return LeaveBalanceView{}, err
	}
	v, _, err := s.leaveBalanceOf(ctx, employeeID, in.Year)
	return v, err
}

// carryAfterHRInput keeps the carry-overs in step after HR set a year's row
// (the PUT and the import): the row's open review closes (HR input never leaves
// carry below used), a row HR just cleared goes back to the formula and is
// recalculated at once, and next year's row follows this year's remainder.
func (s *Service) carryAfterHRInput(ctx context.Context, q database.Querier, employeeID string, in LeaveBalanceInput,
	wasSource string, actor *auth.User, cause carryCause) error {
	if wasSource == carrySourceManual && in.CarryDays == 0 && in.CarryExpires == nil {
		if _, err := recomputeCarryOver(ctx, q, employeeID, in.Year, carryCause{Type: cause.Type,
			Label: cause.reason() + " — sisa bawaan dikembalikan ke perhitungan otomatis"}, actor); err != nil {
			return err
		}
	}
	return afterBalanceChange(ctx, q, employeeID, in.Year, cause, actor)
}

// balanceAudit is the before/after snapshot of a balance in the audit trail.
func balanceAudit(year int, b balanceRaw) map[string]any {
	return map[string]any{"year": year, "annual_leave_total": b.Total, "annual_leave_adjustment": b.Adjustment,
		"annual_leave_used": b.Used, "carry_over_days": b.CarryDays, "carry_over_used": b.CarryUsed,
		"carry_over_expires_on": b.CarryExpires}
}

/* ── Access ──────────────────────────────────────────────────────────── */

// LeaveBalanceAction reports whether the account holds action on Saldo Cuti.
func (a *Actor) LeaveBalanceAction(action string) bool {
	return a != nil && slices.Contains(a.LeaveBalanceActions, action)
}

// leaveBalanceActions are the actions of the Saldo Cuti leaf (R7).
var leaveBalanceActions = []string{"read", "update", "import", "export"}

/* ── Handlers ────────────────────────────────────────────────────────── */

// withLeaveBalanceAction guards a route with an action on the Saldo Cuti
// leaf (requireIamAction) and hands the handler the workforce actor.
func (h handlers) withLeaveBalanceAction(action string, fn func(w http.ResponseWriter, r *http.Request, a *Actor) error) http.Handler {
	return h.withAction(action, saldoCuti, func(w http.ResponseWriter, r *http.Request, u *auth.User) error {
		a, err := h.actorFor(r.Context(), u)
		if err != nil {
			return err
		}
		return fn(w, r, a)
	})
}

// branchParam is the optional ?branch_id (a UUID).
func branchParam(f *validate.Form) string {
	return strOr(f.Str("branch_id", optional, uuidOf))
}

// GET /api/hris/leave-balances?year=&branch_id=&q=
func (h handlers) listLeaveBalances(w http.ResponseWriter, r *http.Request, _ *Actor) error {
	year, err := h.balanceYear(r)
	if err != nil {
		return err
	}
	f := queryForm(r)
	branch := branchParam(f)
	q := strOr(f.Str("q", optional, validate.StrOpts{Trim: true, Max: 100}))
	if err := formErr(f, ""); err != nil {
		return err
	}
	views, err := h.svc.ListLeaveBalances(r.Context(), year, branch, q)
	if err != nil {
		return err
	}
	return ok(w, dataOf(views))
}

// GET /api/hris/leave-balances/{employee_id}?year=
func (h handlers) getLeaveBalance(w http.ResponseWriter, r *http.Request, a *Actor) error {
	year, err := h.balanceYear(r)
	if err != nil {
		return err
	}
	id, err := requireUUID(r.PathValue("employee_id"), "ID karyawan tidak valid")
	if err != nil {
		return err
	}
	view, err := h.svc.LeaveBalanceView(r.Context(), a, id, year)
	if err != nil {
		return err
	}
	return ok(w, dataOf(view))
}

// usedFields are the columns HR never sets through the API (R11).
var usedFields = []string{"annual_leave_used", "sick_leave_used", "unpaid_leave_used"}

// PUT /api/hris/leave-balances/{employee_id}: year, annual_leave_total,
// annual_leave_adjustment, carry_over_days, carry_over_expires_on, reason.
// A figure that is left out keeps its current value; null clears the expiry.
func (h handlers) putLeaveBalance(w http.ResponseWriter, r *http.Request, a *Actor) error {
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	for _, key := range usedFields {
		if sent(f, key) {
			return httpx.BadRequest("Terpakai tidak bisa diubah di sini — hanya berubah lewat persetujuan atau pembatalan cuti")
		}
	}
	halves := validate.NumOpts{}
	total := number(f, "annual_leave_total", optional, halves)
	adjust := number(f, "annual_leave_adjustment", optional, halves)
	carry := number(f, "carry_over_days", optional, halves)
	expires := str(f, "carry_over_expires_on", nullish, validate.StrOpts{Trim: true})
	reason := strOr(f.Str("reason", optional, validate.StrOpts{}))
	year := coerceInt(f, "year", validate.Bound(2000), validate.Bound(2100), 0)
	if err := formErr(f, ""); err != nil {
		return err
	}
	if year == 0 {
		if year, err = h.balanceYear(r); err != nil {
			return err
		}
	}
	id, err := requireUUID(r.PathValue("employee_id"), "ID karyawan tidak valid")
	if err != nil {
		return err
	}
	// Figures left out keep the stored value, read under the row lock (the
	// defaults when there is no row yet).
	in := LeaveBalanceInput{Year: year, Reason: reason,
		KeepTotal: !(total.Sent && total.Val != nil), KeepAdjustment: !(adjust.Sent && adjust.Val != nil),
		KeepCarryDays: !(carry.Sent && carry.Val != nil), KeepCarryExpires: !expires.Sent}
	if total.Sent && total.Val != nil {
		in.Total = *total.Val
	}
	if adjust.Sent && adjust.Val != nil {
		in.Adjustment = *adjust.Val
	}
	if carry.Sent && carry.Val != nil {
		in.CarryDays = *carry.Val
	}
	if expires.Sent {
		in.CarryExpires = nilIfEmpty(expires.Val)
	}
	view, err := h.svc.UpdateLeaveBalanceManual(r.Context(), a, id, in, audit.Entry{}.WithRequest(r))
	if err != nil {
		return err
	}
	return ok(w, dataMsg(view, "Saldo cuti diperbarui"))
}

// GET /api/hris/me/leave-balance-access: the Saldo Cuti actions the account
// holds, so the page shows only the buttons it can use.
func (h handlers) myLeaveBalanceAccess(w http.ResponseWriter, _ *http.Request, a *Actor) error {
	return ok(w, obj("read", a.LeaveBalanceAction("read"), "update", a.LeaveBalanceAction("update"),
		"import", a.LeaveBalanceAction("import"), "export", a.LeaveBalanceAction("export")))
}

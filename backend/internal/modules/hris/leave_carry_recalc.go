package hris

import (
	"context"
	"encoding/json"
	"fmt"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
)

// Sisa bawaan dihitung ulang otomatis (EPIC-068 tindak lanjut).
//
// carry(N) = floor_0,5( min( annual_leave_remaining(N-1), max ) ), where
// annual_leave_remaining is total + adjustment - used of year N-1 (its own
// carry-over is not part of it). Every path that changes year N-1's balance
// calls recomputeCarryOver for year N, so the carry follows the balance
// instead of freezing on 1 January.
//
// Locks are always taken in ascending year order for one employee: N-1 (a
// shared lock, only read), then N, then N+1. A path that changed year Y holds
// Y and recalculates Y+1; the job and the settings read Y-1 first. Two paths
// of one employee therefore never wait on each other in a cycle.

// Cause types of a recalculation (carryCause.Type).
const (
	causeLeaveApprove = "leave_approve"
	causeLeaveCancel  = "leave_cancel"
	causeLeaveReject  = "leave_reject"
	causeHRUpdate     = "hr_update"
	causeImport       = "import"
	causeSettings     = "settings"
	causeJob          = "job"
)

// carrySourceAuto / carrySourceManual are hris.leave_balances.carry_over_source.
const (
	carrySourceAuto   = "auto"
	carrySourceManual = "manual"
)

// reviewResolutionAuto closes a review whose excess went away by itself.
const reviewResolutionAuto = "otomatis"

// carryCause says why a carry-over was recalculated; it goes to the audit row
// and to an opened review. Label overrides the readable reason of the Type.
type carryCause struct {
	Type    string
	LeaveID string
	Label   string
}

// reason is the readable cause shown in the audit trail.
func (c carryCause) reason() string {
	if c.Label != "" {
		return c.Label
	}
	switch c.Type {
	case causeLeaveApprove:
		return "Cuti " + c.LeaveID + " disetujui"
	case causeLeaveCancel:
		return "Cuti " + c.LeaveID + " dibatalkan"
	case causeLeaveReject:
		return "Cuti " + c.LeaveID + " ditolak"
	case causeHRUpdate:
		return "Koreksi HR"
	case causeImport:
		return "Import massal"
	case causeSettings:
		return "Batas sisa bawaan diubah"
	case causeJob:
		return "Job 1 Januari"
	}
	return "Hitung ulang sisa bawaan"
}

// json is the cause stored on audit.after and on a review.
func (c carryCause) json() map[string]any {
	m := map[string]any{"type": c.Type}
	if c.LeaveID != "" {
		m["leave_id"] = c.LeaveID
	}
	return m
}

// userOf adapts the module's actor to the platform user the recalculation
// takes; nil stays nil (the system).
func userOf(a *Actor) *auth.User {
	if a == nil {
		return nil
	}
	return &auth.User{ID: a.UserID, FullName: a.FullName}
}

// carryActor is the actor id (a UUID or "") and name of an audit row; a nil
// user is the system.
func carryActor(u *auth.User) (id, name string) {
	if u == nil {
		return "", sistemName
	}
	if domain.IsUUID(u.ID) {
		id = u.ID
	}
	name = u.FullName
	if name == "" {
		name = "HRD"
	}
	return id, name
}

// carryResult is what one recalculation did.
type carryResult struct {
	Changed  bool
	Carry    float64 // the carry-over after the call (unchanged when !Changed)
	Previous float64 // the carry-over before the call
}

// recomputeCarryOver sets the carry-over of employeeID's row of year from the
// previous year's remainder (formula above) and reports whether the stored
// carry-over (days or expiry) changed. It does nothing - false, nil - when
// carry-over is off or locked, the row does not exist, the employee is not
// active, or HR filled the row by hand (carry_over_source 'manual').
//
// The carry-over may fall below what is already used: it is stored as is and
// a review is opened or updated (excess = used - new); once the carry covers
// the used days again the open review closes itself ('otomatis'). A change of
// value writes one leave_balance.carry_over audit row with the cause and the
// actor (nil = the system); an unchanged value writes nothing.
//
// q must be the caller's transaction: the row is locked FOR UPDATE.
func recomputeCarryOver(ctx context.Context, q database.Querier, employeeID string, year int, cause carryCause, actor *auth.User) (changed bool, err error) {
	settings, err := loadLeaveSettings(ctx, q, false)
	if err != nil {
		return false, err
	}
	res, err := recomputeCarryOverWith(ctx, q, settings, employeeID, year, cause, actor)
	return res.Changed, err
}

// recomputeCarryOverWith is recomputeCarryOver with settings already read, for
// callers that recalculate many rows in one transaction.
func recomputeCarryOverWith(ctx context.Context, q database.Querier, settings LeaveSettings, employeeID string, year int,
	cause carryCause, actor *auth.User) (carryResult, error) {
	if !settings.CarryOverEnabled || !settings.CarryOverReady {
		return carryResult{}, nil
	}
	// Previous year first (shared), then this year (exclusive): ascending order.
	var prevRemaining float64
	err := q.QueryRow(ctx, `SELECT annual_leave_remaining::float8 FROM hris.leave_balances
		WHERE employee_id = $1 AND year = $2 FOR SHARE`, employeeID, year-1).Scan(&prevRemaining)
	if err != nil && !database.IsNoRows(err) {
		return carryResult{}, err
	}

	var name string
	var active bool
	var source string
	var row balanceRaw
	err = q.QueryRow(ctx, `SELECT e.full_name, e.is_active, b.carry_over_source,
			b.annual_leave_total::float8, b.annual_leave_adjustment::float8, b.annual_leave_used::float8,
			b.carry_over_days::float8, b.carry_over_used::float8, b.carry_over_expires_on::text
		FROM hris.leave_balances b JOIN hris.employees e ON e.id = b.employee_id
		WHERE b.employee_id = $1 AND b.year = $2 FOR UPDATE OF b`, employeeID, year).
		Scan(&name, &active, &source, &row.Total, &row.Adjustment, &row.Used, &row.CarryDays, &row.CarryUsed, &row.CarryExpires)
	if database.IsNoRows(err) {
		return carryResult{}, nil
	}
	if err != nil {
		return carryResult{}, err
	}
	row.Source = source
	if !active || source != carrySourceAuto {
		return carryResult{Carry: row.CarryDays, Previous: row.CarryDays}, nil
	}

	carry := carryAmount(prevRemaining, settings.CarryOverMaxDays)
	// The expiry is the setting's date in this year, filled when the row first
	// gets a carry-over; a later change of the setting date leaves it alone.
	expires := row.CarryExpires
	if expires == nil && carry > 0 {
		e := fmt.Sprintf("%04d-%02d-%02d", year, settings.CarryOverExpiryMonth, settings.CarryOverExpiryDay)
		expires = &e
	}
	res := carryResult{Carry: carry, Previous: row.CarryDays}
	if carry != row.CarryDays || !sameText(expires, row.CarryExpires) {
		if _, err := q.Exec(ctx, `UPDATE hris.leave_balances
			SET carry_over_days = $3, carry_over_expires_on = $4::date, updated_at = now()
			WHERE employee_id = $1 AND year = $2`, employeeID, year, carry, expires); err != nil {
			return carryResult{}, err
		}
		after := row
		after.CarryDays, after.CarryExpires = carry, expires
		actorID, who := carryActor(actor)
		afterState := carryState(year, after)
		afterState["cause"], afterState["actor"] = cause.json(), who
		reason := cause.reason()
		if err := audit.Write(ctx, q, audit.Entry{
			ActorID: actorID, ActorName: &who, Action: "leave_balance.carry_over", Entity: "hris.employees", EntityID: &employeeID,
			EntityLabel: ptr(fmt.Sprintf("%s · Saldo cuti %d", name, year)),
			Before:      carryState(year, row), After: afterState, Reason: &reason,
		}); err != nil {
			return carryResult{}, err
		}
		res.Changed = true
	}

	// The review follows the figures whether or not the value changed.
	if carry < row.CarryUsed {
		if err := openCarryReview(ctx, q, employeeID, year, carry, row.CarryUsed, cause); err != nil {
			return carryResult{}, err
		}
	} else if err := syncOpenCarryReview(ctx, q, employeeID, year, actor); err != nil {
		return carryResult{}, err
	}
	return res, nil
}

// openCarryReview opens, or updates, the review of the row whose carry-over
// (days) is below what is used. At most one review is open per employee and
// year. It stays quiet when the latest review of the row already covers these
// exact figures: an open one needs nothing, and one HR resolved must not come
// back for a recalculation that changes nothing. A review that closed by
// itself ('otomatis') does not count as handled: the same shortfall appearing
// again opens a new one.
func openCarryReview(ctx context.Context, q database.Querier, employeeID string, year int, days, used float64, cause carryCause) error {
	var id string
	var lastDays, lastUsed float64
	var open bool
	var resolution *string
	err := q.QueryRow(ctx, `SELECT id::text, carry_over_days::float8, carry_over_used::float8, resolved_at IS NULL, resolution
		FROM hris.leave_carry_over_reviews WHERE employee_id = $1 AND year = $2
		ORDER BY (resolved_at IS NULL) DESC, resolved_at DESC NULLS LAST, opened_at DESC LIMIT 1`, employeeID, year).
		Scan(&id, &lastDays, &lastUsed, &open, &resolution)
	found := err == nil
	if err != nil && !database.IsNoRows(err) {
		return err
	}
	same := found && lastDays == days && lastUsed == used
	if same && (open || (resolution != nil && *resolution != reviewResolutionAuto)) {
		return nil
	}
	causeJSON, err := json.Marshal(cause.json())
	if err != nil {
		return err
	}
	if found && open {
		_, err = q.Exec(ctx, `UPDATE hris.leave_carry_over_reviews
			SET carry_over_days = $2, carry_over_used = $3, excess = $4, cause = $5::jsonb WHERE id = $1`,
			id, days, used, used-days, string(causeJSON))
		return err
	}
	_, err = q.Exec(ctx, `INSERT INTO hris.leave_carry_over_reviews (employee_id, year, carry_over_days, carry_over_used, excess, cause)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb)`, employeeID, year, days, used, used-days, string(causeJSON))
	return err
}

// syncOpenCarryReview brings the open review of the row in step with its
// balance after the balance changed: it closes when the carry-over covers the
// used days again (resolution 'otomatis'), and while the shortfall remains it
// takes the new used figure and excess (a partial refund shrinks it). A no-op
// without the review table or without an open review.
func syncOpenCarryReview(ctx context.Context, q database.Querier, employeeID string, year int, actor *auth.User) error {
	var ready bool
	if err := q.QueryRow(ctx, `SELECT to_regclass('hris.leave_carry_over_reviews') IS NOT NULL`).Scan(&ready); err != nil || !ready {
		return err
	}
	actorID, who := carryActor(actor)
	var by *string
	if actorID != "" {
		by = &actorID
	}
	if _, err := q.Exec(ctx, `UPDATE hris.leave_carry_over_reviews r
		SET resolved_at = now(), resolved_by = $3::uuid, resolved_by_name = $4, resolution = $5
		FROM hris.leave_balances b
		WHERE r.employee_id = $1 AND r.year = $2 AND r.resolved_at IS NULL
		  AND b.employee_id = r.employee_id AND b.year = r.year AND b.carry_over_used <= b.carry_over_days`,
		employeeID, year, by, who, reviewResolutionAuto); err != nil {
		return err
	}
	_, err := q.Exec(ctx, `UPDATE hris.leave_carry_over_reviews r
		SET carry_over_days = b.carry_over_days, carry_over_used = b.carry_over_used,
		    excess = b.carry_over_used - b.carry_over_days
		FROM hris.leave_balances b
		WHERE r.employee_id = $1 AND r.year = $2 AND r.resolved_at IS NULL
		  AND b.employee_id = r.employee_id AND b.year = r.year AND b.carry_over_used > b.carry_over_days
		  AND (r.carry_over_days, r.carry_over_used) IS DISTINCT FROM (b.carry_over_days, b.carry_over_used)`, employeeID, year)
	return err
}

// lockPreviousBalance takes the shared lock on year-1 that recomputeCarryOver
// would take, ahead of locking year itself, so a path that edits a row and may
// recalculate that same row keeps the ascending order.
func lockPreviousBalance(ctx context.Context, q database.Querier, employeeID string, year int) error {
	_, err := q.Exec(ctx, `SELECT 1 FROM hris.leave_balances WHERE employee_id = $1 AND year = $2 FOR SHARE`, employeeID, year-1)
	return ignoreBadInput(err)
}

// afterBalanceChange is the hook of every path that changed year's balance
// (R2): the open review of the year closes when its carry-over covers the used
// days again, and the next year's carry-over is recalculated when that row
// already exists (recomputeCarryOver ignores a missing row). q is the caller's
// transaction, which holds year's row lock.
func afterBalanceChange(ctx context.Context, q database.Querier, employeeID string, year int, cause carryCause, actor *auth.User) error {
	if err := syncOpenCarryReview(ctx, q, employeeID, year, actor); err != nil {
		return err
	}
	_, err := recomputeCarryOver(ctx, q, employeeID, year+1, cause, actor)
	return err
}

// nextCarrySource is carry_over_source after HR set the carry-over days and
// expiry on a row (the same rule SetLeaveBalance applies in SQL): clearing both
// gives the row back to the formula, a changed value makes it manual.
func nextCarrySource(was balanceRaw, carryDays float64, expires *string) string {
	switch {
	case carryDays == 0 && expires == nil:
		return carrySourceAuto
	case carryDays != was.CarryDays || !sameText(expires, was.CarryExpires):
		return carrySourceManual
	}
	return was.Source
}

// carryState is the part of a balance that a carry-over audit row records.
func carryState(year int, b balanceRaw) map[string]any {
	return map[string]any{"year": year, "carry_over_days": b.CarryDays, "carry_over_used": b.CarryUsed,
		"carry_over_expires_on": b.CarryExpires}
}

// sameText compares two optional strings.
func sameText(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

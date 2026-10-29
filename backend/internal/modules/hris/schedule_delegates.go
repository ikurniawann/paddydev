package hris

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/validate"
)

// EPIC-071: pengatur jadwal toko. HR appoints an ordinary employee (a store
// head) who then arranges work schedules, and when can_place also the store
// placements, of the people in the stores they were given (R1/R2). The
// appointment lives in hris.schedule_delegates / schedule_delegate_branches;
// the entry point is the ESS menu "Jadwal Toko" (ess.store-schedule).

// delegateAccess is an active schedule delegate: the signed-in account's
// employee, whether they may also place, and the active stores they cover
// (sorted by store code). A delegate with no active store is not returned
// by loadDelegateAccess at all, so BranchIDs is never empty.
type delegateAccess struct {
	EmployeeID, Name string
	CanPlace         bool
	BranchIDs        []string
}

// storeScope is who is acting on store schedules. Delegate == nil means HR,
// who is not limited to any store.
type storeScope struct {
	Delegate *delegateAccess
}

// AllowsBranch reports whether the scope may act on store id: always for
// HR; for a delegate only when id is one of their stores ("" never is).
func (s storeScope) AllowsBranch(id string) bool {
	if s.Delegate == nil {
		return true
	}
	return id != "" && slices.Contains(s.Delegate.BranchIDs, id)
}

// errOutsideScope is the 403 for a store outside the delegate's area (R4).
const errOutsideScope = "Toko di luar wilayah Anda"

// errNoPlaceRight is the 403 for a delegate without the placement right.
const errNoPlaceRight = "Anda tidak punya hak mengatur penempatan"

// withScheduleScope guards a store-schedule route for HR and for appointed
// delegates (EPIC-071). HR is decided exactly as the route did before:
// HasMenuPrefix when action is "", HasMenuAction(action) otherwise. Anyone
// else must be an active delegate (loadDelegateAccess), else 403 with the
// usual "Insufficient permissions"; needPlace additionally requires
// can_place. fn receives the scope and must check sc.AllowsBranch itself.
func (h handlers) withScheduleScope(action string, prefixes []string, needPlace bool, fn func(w http.ResponseWriter, r *http.Request, u *auth.User, sc storeScope) error) http.Handler {
	return handle(func(w http.ResponseWriter, r *http.Request) error {
		u, err := h.auth.RequireUser(r)
		if err != nil {
			return err
		}
		var isHR bool
		if action == "" {
			isHR, err = h.auth.HasMenuPrefix(r.Context(), u, prefixes...)
		} else {
			isHR, err = h.auth.HasMenuAction(r.Context(), u, action, prefixes...)
		}
		if err != nil {
			return err
		}
		if isHR {
			return fn(w, r, u, storeScope{})
		}
		d, err := loadDelegateAccess(r.Context(), h.svc.repo.querier(), u.ID)
		if err != nil {
			return err
		}
		if d == nil {
			return httpx.Forbidden("Insufficient permissions")
		}
		if needPlace && !d.CanPlace {
			return httpx.Forbidden(errNoPlaceRight)
		}
		return fn(w, r, u, storeScope{Delegate: d})
	})
}

// loadDelegateAccess returns the schedule-delegate access of the account
// userID, or nil when it is not an active delegate: no appointment, the
// employee is inactive, or none of the appointed stores is active. When the
// account is linked to several active appointed employees the lowest id wins,
// so the answer is stable.
func loadDelegateAccess(ctx context.Context, q database.Querier, userID string) (*delegateAccess, error) {
	var d delegateAccess
	err := q.QueryRow(ctx, `
		SELECT e.id::text, e.full_name, sd.can_place
		FROM hris.schedule_delegates sd
		JOIN hris.employees e ON e.id = sd.employee_id
		WHERE e.is_active AND e.user_id = $1
		ORDER BY e.id
		LIMIT 1`, userID).Scan(&d.EmployeeID, &d.Name, &d.CanPlace)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `
		SELECT b.id::text
		FROM hris.schedule_delegate_branches sb
		JOIN configuration.branches b ON b.id = sb.branch_id
		WHERE sb.employee_id = $1 AND b.is_active
		ORDER BY b.code, b.id`, d.EmployeeID)
	if err != nil {
		return nil, err
	}
	d.BranchIDs, err = pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	if len(d.BranchIDs) == 0 {
		return nil, nil
	}
	return &d, nil
}

// delegateSubtree is a CTE prefix `WITH RECURSIVE sub(id, depth)` listing the
// active employees below a delegate in the reporting_to chain, to at most 10
// levels (the delegate themself excluded). It uses $1 = the delegate's
// employee id. A query appends its own CTEs (", other AS (...)") or its main
// SELECT after it and takes any further parameters from $2 on. UNION, not
// UNION ALL, plus the depth cap make a reporting_to cycle terminate.
const delegateSubtree = `WITH RECURSIVE sub(id, depth) AS (
	SELECT e.id, 1 FROM hris.employees e
	WHERE e.reporting_to = $1 AND e.id <> $1 AND e.is_active
	UNION
	SELECT e.id, s.depth + 1 FROM hris.employees e
	JOIN sub s ON e.reporting_to = s.id
	WHERE e.is_active AND e.id <> $1 AND s.depth < 10
)`

// delegateOutside returns the ids, in input order, that are NOT in the
// delegate's territory (R3). In territory means an active employee who has
// a placement running on today (WIB, YYYY-MM-DD) at one of d.BranchIDs, or
// who sits in the delegate's reporting subtree. Unknown and inactive ids
// are outside. The ids must be valid uuids.
func delegateOutside(ctx context.Context, q database.Querier, d *delegateAccess, today string, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return []string{}, nil
	}
	rows, err := q.Query(ctx, delegateSubtree+`
		SELECT i.id::text
		FROM unnest($2::uuid[]) WITH ORDINALITY AS i(id, n)
		WHERE NOT EXISTS (
			SELECT 1 FROM hris.employees e
			WHERE e.id = i.id AND e.is_active AND (
				e.id IN (SELECT id FROM sub)
				OR EXISTS (
					SELECT 1 FROM hris.employee_placements p
					WHERE p.employee_id = e.id AND p.branch_id = ANY($3::uuid[])
					  AND p.start_date <= $4::date AND (p.end_date IS NULL OR p.end_date >= $4::date)
				)
			)
		)
		ORDER BY i.n`, d.EmployeeID, ids, d.BranchIDs, today)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if out == nil {
		out = []string{}
	}
	return out, err
}

// delegateBranches lists the stores (id, code, name) by code.
func delegateBranches(ctx context.Context, q database.Querier, ids []string) ([]*Row, error) {
	rows, err := q.Query(ctx, `
		SELECT id::text, code, name FROM configuration.branches
		WHERE id = ANY($1::uuid[]) ORDER BY code, id`, ids)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (*Row, error) {
		var id, code, name string
		if err := r.Scan(&id, &code, &name); err != nil {
			return nil, err
		}
		return obj("id", id, "code", code, "name", name), nil
	})
}

// GET /api/hris/me/schedule-access: whether the signed-in account is a store
// schedule delegate, whether it may also place, and its active stores. Anyone
// else gets is_delegate false (the ESS menu is granted to every role).
func (h handlers) myScheduleAccess(w http.ResponseWriter, r *http.Request, a *Actor) error {
	q := h.svc.repo.querier()
	d, err := loadDelegateAccess(r.Context(), q, a.UserID)
	if err != nil {
		return err
	}
	if d == nil {
		return ok(w, dataOf(obj("is_delegate", false, "can_place", false, "branches", []*Row{})))
	}
	branches, err := delegateBranches(r.Context(), q, d.BranchIDs)
	if err != nil {
		return err
	}
	return ok(w, dataOf(obj("is_delegate", true, "can_place", d.CanPlace, "branches", branches)))
}

/* ── HR: kelola pengatur (R9) ───────────────────────────────────────── */

// maxDelegateBranches caps the stores of one appointment.
const maxDelegateBranches = 100

// delegateRows lists the appointments as the HR page shows them, by name.
// employeeID "" lists them all, otherwise just that one. Every appointment is
// listed, including those whose employee is now inactive (is_active false,
// R11); stores are the appointed ones that are still active.
func delegateRows(ctx context.Context, q database.Querier, employeeID string) ([]*Row, error) {
	type rec struct {
		id, nip, name       string
		position, updatedBy *string
		hasLogin, canPlace  bool
		active              bool
		reports             int
		updatedAt           time.Time
	}
	rows, err := q.Query(ctx, `
		SELECT e.id::text, e.nip, e.full_name, pos.title, e.user_id IS NOT NULL, sd.can_place,
		       (SELECT count(*) FROM hris.employees r WHERE r.reporting_to = e.id AND r.is_active),
		       e.is_active, sd.updated_at, u.full_name
		FROM hris.schedule_delegates sd
		JOIN hris.employees e ON e.id = sd.employee_id
		LEFT JOIN hris.positions pos ON pos.id = e.job_title_id
		LEFT JOIN configuration.users u ON u.id = sd.updated_by
		WHERE ($1 = '' OR sd.employee_id = $1::uuid)
		ORDER BY e.full_name, e.id`, employeeID)
	if err != nil {
		return nil, err
	}
	recs, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (rec, error) {
		var x rec
		err := r.Scan(&x.id, &x.nip, &x.name, &x.position, &x.hasLogin, &x.canPlace, &x.reports, &x.active, &x.updatedAt, &x.updatedBy)
		return x, err
	})
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(recs))
	for i, x := range recs {
		ids[i] = x.id
	}
	brRows, err := q.Query(ctx, `
		SELECT sb.employee_id::text, b.id::text, b.code, b.name
		FROM hris.schedule_delegate_branches sb
		JOIN configuration.branches b ON b.id = sb.branch_id
		WHERE sb.employee_id = ANY($1::uuid[]) AND b.is_active
		ORDER BY b.code, b.id`, ids)
	if err != nil {
		return nil, err
	}
	defer brRows.Close()
	byEmp := map[string][]*Row{}
	for brRows.Next() {
		var emp, id, code, name string
		if err := brRows.Scan(&emp, &id, &code, &name); err != nil {
			return nil, err
		}
		byEmp[emp] = append(byEmp[emp], obj("id", id, "code", code, "name", name))
	}
	if err := brRows.Err(); err != nil {
		return nil, err
	}
	out := make([]*Row, 0, len(recs))
	for _, x := range recs {
		br := byEmp[x.id]
		if br == nil {
			br = []*Row{}
		}
		out = append(out, obj("employee_id", x.id, "nip", x.nip, "full_name", x.name, "position_title", x.position,
			"has_login", x.hasLogin, "can_place", x.canPlace, "direct_reports", x.reports, "is_active", x.active,
			"branches", br, "updated_at", x.updatedAt, "updated_by_name", x.updatedBy))
	}
	return out, nil
}

// GET /api/hris/schedule-delegates: every appointment, by employee name.
func (h handlers) listScheduleDelegates(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	rows, err := delegateRows(r.Context(), h.svc.repo.querier(), "")
	if err != nil {
		return err
	}
	return ok(w, dataOf(rows))
}

// GET /api/hris/schedule-delegates/suggest?employee_id=: the stores to
// pre-fill when appointing (R2): the active stores of the employee's running
// placements plus those of everyone below them.
func (h handlers) suggestDelegateBranches(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	id, err := requireUUID(r.URL.Query().Get("employee_id"), "ID karyawan tidak valid")
	if err != nil {
		return err
	}
	ctx := r.Context()
	q := h.svc.repo.querier()
	var exists bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM hris.employees WHERE id = $1)`, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return httpx.NotFound("Karyawan tidak ditemukan")
	}
	today := domain.TodayWIB(h.svc.now())
	rows, err := q.Query(ctx, delegateSubtree+`
		SELECT b.id::text
		FROM configuration.branches b
		WHERE b.is_active AND EXISTS (
			SELECT 1 FROM hris.employee_placements p
			WHERE p.branch_id = b.id
			  AND (p.employee_id = $1 OR p.employee_id IN (SELECT id FROM sub))
			  AND p.start_date <= $2::date AND (p.end_date IS NULL OR p.end_date >= $2::date))
		ORDER BY b.code, b.id`, id, today)
	if err != nil {
		return err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	if ids == nil {
		ids = []string{}
	}
	return ok(w, dataOf(obj("branch_ids", ids)))
}

// delegateAuditState is the before/after of a set/revoke audit row.
func delegateAuditState(canPlace bool, branchIDs []string) map[string]any {
	return map[string]any{"can_place": canPlace, "branch_ids": branchIDs}
}

// storedDelegate reads an appointment as stored (every appointed store, active
// or not, by id). nil when there is none.
func storedDelegate(ctx context.Context, q database.Querier, employeeID string) (map[string]any, error) {
	var canPlace bool
	err := q.QueryRow(ctx, `SELECT can_place FROM hris.schedule_delegates WHERE employee_id = $1`, employeeID).Scan(&canPlace)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `SELECT branch_id::text FROM hris.schedule_delegate_branches
		WHERE employee_id = $1 ORDER BY branch_id`, employeeID)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	if ids == nil {
		ids = []string{}
	}
	return delegateAuditState(canPlace, ids), nil
}

// PUT /api/hris/schedule-delegates/{employee_id} { branch_ids[1..100], can_place }:
// appoints the employee, or replaces their stores and flag, in one transaction
// with the audit row.
func (h handlers) setScheduleDelegate(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	id, err := requireUUID(r.PathValue("employee_id"), "ID karyawan tidak valid")
	if err != nil {
		return err
	}
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	branchIDs := f.Strings("branch_ids", nullish, maxDelegateBranches, validate.StrOpts{Check: validate.UUIDCheck})
	canPlace := f.BoolDefault("can_place", false)
	if err := formErr(f, ""); err != nil {
		return err
	}
	slices.Sort(branchIDs)
	branchIDs = slices.Compact(branchIDs)
	if len(branchIDs) == 0 {
		return httpx.BadRequest("Pilih minimal satu toko")
	}
	ctx := r.Context()
	err = h.svc.repo.InTx(ctx, func(repo Repository) error {
		q := repo.querier()
		var name string
		var active, hasLogin bool
		err := q.QueryRow(ctx, `SELECT full_name, is_active, user_id IS NOT NULL
			FROM hris.employees WHERE id = $1 FOR UPDATE`, id).Scan(&name, &active, &hasLogin)
		if database.IsNoRows(err) {
			return httpx.NotFound("Karyawan tidak ditemukan")
		}
		if err != nil {
			return err
		}
		if !active {
			return httpx.BadRequest("Karyawan tidak aktif")
		}
		if !hasLogin {
			return httpx.BadRequest("Karyawan belum punya akun login")
		}
		var valid int
		if err := q.QueryRow(ctx, `SELECT count(*) FROM configuration.branches
			WHERE id = ANY($1::uuid[]) AND is_active`, branchIDs).Scan(&valid); err != nil {
			return err
		}
		if valid != len(branchIDs) {
			return httpx.BadRequest("Toko tidak valid")
		}
		var before any
		if cur, err := storedDelegate(ctx, q, id); err != nil {
			return err
		} else if cur != nil {
			before = cur
		}
		if _, err := q.Exec(ctx, `
			INSERT INTO hris.schedule_delegates (employee_id, can_place, created_by, updated_by)
			VALUES ($1, $2, $3, $3)
			ON CONFLICT (employee_id) DO UPDATE SET can_place = EXCLUDED.can_place,
				updated_by = EXCLUDED.updated_by, updated_at = now()`, id, canPlace, u.ID); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `DELETE FROM hris.schedule_delegate_branches WHERE employee_id = $1`, id); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `INSERT INTO hris.schedule_delegate_branches (employee_id, branch_id)
			SELECT $1, unnest($2::uuid[])`, id, branchIDs); err != nil {
			return err
		}
		return writeEmployeeAudit(ctx, q, u, "schedule_delegate.set", id, name, before, delegateAuditState(canPlace, branchIDs))
	})
	if err != nil {
		return err
	}
	rows, err := delegateRows(ctx, h.svc.repo.querier(), id)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return httpx.NotFound("Pengatur jadwal tidak ditemukan")
	}
	return ok(w, dataOf(rows[0]))
}

// DELETE /api/hris/schedule-delegates/{employee_id}: revokes the appointment
// (also of an employee who is inactive by now), audited.
func (h handlers) revokeScheduleDelegate(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	id, err := requireUUID(r.PathValue("employee_id"), "ID karyawan tidak valid")
	if err != nil {
		return err
	}
	ctx := r.Context()
	err = h.svc.repo.InTx(ctx, func(repo Repository) error {
		q := repo.querier()
		var name string
		err := q.QueryRow(ctx, `SELECT e.full_name FROM hris.schedule_delegates sd
			JOIN hris.employees e ON e.id = sd.employee_id
			WHERE sd.employee_id = $1 FOR UPDATE OF sd`, id).Scan(&name)
		if database.IsNoRows(err) {
			return httpx.NotFound("Pengatur jadwal tidak ditemukan")
		}
		if err != nil {
			return err
		}
		before, err := storedDelegate(ctx, q, id)
		if err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `DELETE FROM hris.schedule_delegates WHERE employee_id = $1`, id); err != nil {
			return err
		}
		return writeEmployeeAudit(ctx, q, u, "schedule_delegate.revoke", id, name, before, nil)
	})
	if err != nil {
		return err
	}
	return ok(w, dataOf(obj("revoked", true)))
}

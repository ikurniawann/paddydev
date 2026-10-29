package hris

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/platform/database"
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

// loadDelegateAccess returns the schedule-delegate access of the account
// userID, or nil when it is not an active delegate: no appointment, the
// employee is inactive, or none of the appointed stores is active.
func loadDelegateAccess(ctx context.Context, q database.Querier, userID string) (*delegateAccess, error) {
	var d delegateAccess
	err := q.QueryRow(ctx, `
		SELECT e.id::text, e.full_name, sd.can_place
		FROM hris.schedule_delegates sd
		JOIN hris.employees e ON e.id = sd.employee_id
		WHERE e.is_active AND e.user_id = $1`, userID).Scan(&d.EmployeeID, &d.Name, &d.CanPlace)
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

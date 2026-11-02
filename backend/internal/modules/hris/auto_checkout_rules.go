package hris

import (
	"context"
	"net/http"

	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/validate"
)

// EPIC-080 R1/R2: aturan checkout otomatis per jabatan dan lokasi. HR
// (Kepegawaian) mengatur semuanya; pengatur jadwal toko hanya membuat,
// menyalakan/mematikan, atau menghapus aturan di tokonya, dan melihat (tanpa
// mengubah) aturan "semua lokasi". Jalannya job ada di auto_checkout.go.

const (
	autoCheckoutRuleEntity = "hris.auto_checkout_rules"
	errRuleDuplicate       = "Aturan untuk jabatan dan lokasi ini sudah ada"
	errRuleNotFound        = "Aturan tidak ditemukan"
	errRuleOutsideScope    = "Hanya untuk toko yang Anda atur"
)

// autoCheckoutRuleSelect is a rule with its position and store names.
const autoCheckoutRuleSelect = `SELECT r.id, r.position_id, p.title AS position_title, r.branch_id,
		b.name AS branch_name, r.is_active, r.created_at, r.updated_at
	FROM hris.auto_checkout_rules r
	JOIN hris.positions p ON p.id = r.position_id
	LEFT JOIN configuration.branches b ON b.id = r.branch_id`

// markEditable sets editable: whether sc may change the rule (R2).
func markEditable(sc storeScope, rows []*Row) []*Row {
	for _, row := range rows {
		branch := ""
		if id := row.StrPtr("branch_id"); id != nil {
			branch = *id
		}
		row.Set("editable", sc.Delegate == nil || sc.AllowsBranch(branch))
	}
	return rows
}

// loadAutoCheckoutRule reads one rule; nil when it does not exist.
func loadAutoCheckoutRule(ctx context.Context, q database.Querier, id string) (*Row, error) {
	return queryRow(ctx, q, autoCheckoutRuleSelect+` WHERE r.id = $1`, id)
}

// autoCheckoutRuleAudit is the before/after of an audit row.
func autoCheckoutRuleAudit(row *Row) map[string]any {
	return map[string]any{
		"position_id": row.Get("position_id"), "position_title": row.Get("position_title"),
		"branch_id": row.Get("branch_id"), "branch_name": row.Get("branch_name"),
		"is_active": row.Get("is_active"),
	}
}

// writeAutoCheckoutRuleAudit records action ("create", "update", "delete") on
// q, so it commits with the change. before/after are nil when absent.
func writeAutoCheckoutRuleAudit(ctx context.Context, q database.Querier, r *http.Request, u *auth.User, action, id string, label *Row, before, after *Row) error {
	e := audit.Entry{ActorID: u.ID, Action: "attendance.auto_checkout_rule." + action, Entity: autoCheckoutRuleEntity, EntityID: &id}.WithRequest(r)
	name := u.FullName
	e.ActorName = &name
	title, where := label.Str("position_title"), "Semua lokasi"
	if b := label.StrPtr("branch_name"); b != nil {
		where = *b
	}
	l := title + " @ " + where
	e.EntityLabel = &l
	if before != nil {
		e.Before = autoCheckoutRuleAudit(before)
	}
	if after != nil {
		e.After = autoCheckoutRuleAudit(after)
	}
	return audit.Write(ctx, q, e)
}

// requireRuleScope is the delegate's 403 for a rule of a store outside their
// area or an all-location rule (R2). HR is never refused.
func requireRuleScope(sc storeScope, branchID *string) error {
	branch := ""
	if branchID != nil {
		branch = *branchID
	}
	if !sc.AllowsBranch(branch) {
		return httpx.Forbidden(errRuleOutsideScope)
	}
	return nil
}

// GET /api/hris/auto-checkout-rules: HR sees every rule; a delegate sees the
// rules of their stores plus the all-location ones (read-only).
func (h handlers) listAutoCheckoutRules(w http.ResponseWriter, r *http.Request, _ *auth.User, sc storeScope) error {
	q := h.svc.repo.querier()
	var rows []*Row
	var err error
	if sc.Delegate == nil {
		rows, err = queryRows(r.Context(), q, autoCheckoutRuleSelect+` ORDER BY p.title, b.name NULLS FIRST, r.id`)
	} else {
		rows, err = queryRows(r.Context(), q, autoCheckoutRuleSelect+`
			WHERE r.branch_id IS NULL OR r.branch_id = ANY($1::uuid[])
			ORDER BY p.title, b.name NULLS FIRST, r.id`, sc.Delegate.BranchIDs)
	}
	if err != nil {
		return err
	}
	return ok(w, dataOf(markEditable(sc, rows)))
}

// GET /api/hris/auto-checkout-rules/options: what the "Tambah aturan" dialog
// offers: the active positions, the stores the caller may pick, and whether
// "semua lokasi" is one of the choices (HR only).
func (h handlers) autoCheckoutRuleOptions(w http.ResponseWriter, r *http.Request, _ *auth.User, sc storeScope) error {
	ctx := r.Context()
	q := h.svc.repo.querier()
	positions, err := queryRows(ctx, q, `SELECT id, title FROM hris.positions WHERE is_active ORDER BY title, id`)
	if err != nil {
		return err
	}
	var branches []*Row
	if sc.Delegate == nil {
		branches, err = queryRows(ctx, q, `SELECT id, code, name FROM configuration.branches WHERE is_active ORDER BY code, id`)
	} else {
		branches, err = queryRows(ctx, q, `SELECT id, code, name FROM configuration.branches
			WHERE is_active AND id = ANY($1::uuid[]) ORDER BY code, id`, sc.Delegate.BranchIDs)
	}
	if err != nil {
		return err
	}
	return ok(w, dataOf(obj("positions", positions, "branches", branches, "all_locations", sc.Delegate == nil)))
}

// POST /api/hris/auto-checkout-rules { position_id, branch_id|null }.
func (h handlers) createAutoCheckoutRule(w http.ResponseWriter, r *http.Request, u *auth.User, sc storeScope) error {
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	positionID := f.UUID("position_id", validate.Rule{})
	branchID := f.UUID("branch_id", nullish)
	if err := formErr(f, ""); err != nil {
		return err
	}
	if err := requireRuleScope(sc, branchID); err != nil {
		return err
	}
	ctx := r.Context()
	var created *Row
	err = h.svc.repo.InTx(ctx, func(repo Repository) error {
		q := repo.querier()
		var exists bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM hris.positions WHERE id = $1 AND is_active)`, *positionID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return httpx.BadRequest("Jabatan tidak ditemukan")
		}
		if branchID != nil {
			if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM configuration.branches WHERE id = $1 AND is_active)`, *branchID).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return httpx.BadRequest("Toko tidak ditemukan")
			}
		}
		var id string
		err := q.QueryRow(ctx, `INSERT INTO hris.auto_checkout_rules (position_id, branch_id, created_by, updated_by)
			VALUES ($1, $2, $3, $3) RETURNING id::text`, *positionID, branchID, u.ID).Scan(&id)
		if database.IsUniqueViolation(err) {
			return httpx.Conflict(errRuleDuplicate)
		}
		if err != nil {
			return err
		}
		if created, err = loadAutoCheckoutRule(ctx, q, id); err != nil {
			return err
		}
		return writeAutoCheckoutRuleAudit(ctx, q, r, u, "create", id, created, nil, created)
	})
	if err != nil {
		return err
	}
	return reply(w, http.StatusCreated, dataOf(markEditable(sc, []*Row{created})[0]))
}

// PATCH /api/hris/auto-checkout-rules/{id} { is_active }.
func (h handlers) patchAutoCheckoutRule(w http.ResponseWriter, r *http.Request, u *auth.User, sc storeScope) error {
	id, err := requireUUID(r.PathValue("id"), "ID aturan tidak valid")
	if err != nil {
		return err
	}
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	active := f.Bool("is_active", validate.Rule{})
	if err := formErr(f, ""); err != nil {
		return err
	}
	ctx := r.Context()
	var after *Row
	err = h.svc.repo.InTx(ctx, func(repo Repository) error {
		q := repo.querier()
		before, err := lockAutoCheckoutRule(ctx, q, id)
		if err != nil {
			return err
		}
		if err := requireRuleScope(sc, before.StrPtr("branch_id")); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `UPDATE hris.auto_checkout_rules SET is_active = $2, updated_by = $3, updated_at = now()
			WHERE id = $1`, id, *active, u.ID); err != nil {
			return err
		}
		if after, err = loadAutoCheckoutRule(ctx, q, id); err != nil {
			return err
		}
		return writeAutoCheckoutRuleAudit(ctx, q, r, u, "update", id, after, before, after)
	})
	if err != nil {
		return err
	}
	return ok(w, dataOf(markEditable(sc, []*Row{after})[0]))
}

// DELETE /api/hris/auto-checkout-rules/{id}.
func (h handlers) deleteAutoCheckoutRule(w http.ResponseWriter, r *http.Request, u *auth.User, sc storeScope) error {
	id, err := requireUUID(r.PathValue("id"), "ID aturan tidak valid")
	if err != nil {
		return err
	}
	ctx := r.Context()
	// HR needs the "delete" action like every sibling kepegawaian delete
	// (hrisdel.Guard), but no reason: rules are config, audited with before.
	// A delegate is held to their stores instead (R2).
	if sc.Delegate == nil {
		can, err := h.auth.HasMenuAction(ctx, u, "delete", iam.HrisKepegawaian...)
		if err != nil {
			return err
		}
		if !can {
			return httpx.Forbidden("Insufficient permissions")
		}
	}
	err = h.svc.repo.InTx(ctx, func(repo Repository) error {
		q := repo.querier()
		before, err := lockAutoCheckoutRule(ctx, q, id)
		if err != nil {
			return err
		}
		if err := requireRuleScope(sc, before.StrPtr("branch_id")); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `DELETE FROM hris.auto_checkout_rules WHERE id = $1`, id); err != nil {
			return err
		}
		return writeAutoCheckoutRuleAudit(ctx, q, r, u, "delete", id, before, before, nil)
	})
	if err != nil {
		return err
	}
	return ok(w, dataOf(obj("deleted", true)))
}

// lockAutoCheckoutRule reads a rule and locks its row for the rest of the
// transaction; 404 when it does not exist.
func lockAutoCheckoutRule(ctx context.Context, q database.Querier, id string) (*Row, error) {
	var locked string
	err := q.QueryRow(ctx, `SELECT id::text FROM hris.auto_checkout_rules WHERE id = $1 FOR UPDATE`, id).Scan(&locked)
	if database.IsNoRows(err) {
		return nil, httpx.NotFound(errRuleNotFound)
	}
	if err != nil {
		return nil, err
	}
	return loadAutoCheckoutRule(ctx, q, id)
}

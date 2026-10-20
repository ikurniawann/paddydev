package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/modules/configuration/kit"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/permaudit"
)

// SQL on the tables this area owns: configuration.users and its approval
// permissions, stalls and audit log, and iam.user_roles.

// permissionsEmbed is the query builder's
// `user_approval_permissions!user_id(*)` embed on users: the permissions
// granted to the user.
const permissionsEmbed = `COALESCE((SELECT json_agg(e) FROM (SELECT * FROM "configuration"."user_approval_permissions" WHERE "user_id" = "users"."id") e), '[]'::json) AS "user_approval_permissions"`

// quietly runs fn in a savepoint and drops its error: a TS call whose
// { error } nobody reads, which must not abort the caller's transaction.
func quietly(ctx context.Context, db database.DB, fn func(q database.Querier) error) {
	_ = database.WithTx(ctx, db, func(tx pgx.Tx) error { return fn(tx) })
}

// set is one column assignment of an INSERT or UPDATE.
type set struct {
	col string
	val any
}

// updateSQL builds UPDATE table SET ... WHERE id = $n with the assignments in
// order. returning is appended verbatim when not empty.
func updateSQL(table string, sets []set, returning string) (string, []any) {
	parts := make([]string, len(sets))
	args := make([]any, len(sets), len(sets)+1)
	for i, s := range sets {
		parts[i] = `"` + s.col + `" = $` + strconv.Itoa(i+1)
		args[i] = s.val
	}
	sql := `UPDATE ` + table + ` SET ` + strings.Join(parts, ", ") + ` WHERE "id" = $` + strconv.Itoa(len(sets)+1) + `::text::uuid`
	if returning != "" {
		sql += ` RETURNING ` + returning
	}
	return sql, args
}

/* ── app user profile (enrichWithAppUser) ────────────────────────────── */

type warehouseRow struct {
	WarehouseID, Name, Code, BranchID string
}

// appUser is the app_user an employee row is enriched with.
type appUser struct {
	ID, Role, Status                             string
	BrandID, BusinessScope, HoldingID, CompanyID *string
	BranchID, DefaultWarehouseID                 *string
	CanSwitchStall, CanCentralCheckout           bool
	Permissions                                  json.RawMessage
	HoldingName, CompanyName, BranchName         *string
	LastSignInAt                                 *time.Time
	Warehouses                                   []warehouseRow
	// HrisRole is the account's admin_hris / hris_* IAM role (EPIC-061).
	HrisRole *string
}

// loadAppUser reads the profile and its scope names, nil without profile.
func loadAppUser(ctx context.Context, q database.Querier, userID string) (*appUser, error) {
	row, err := kit.QueryOne(ctx, q, `SELECT "id", "role", "status", "brand_id", "business_scope", "holding_id",
       "company_id", "branch_id", "can_switch_stall", "can_central_checkout", "default_warehouse_id",
       (SELECT r.code FROM iam.user_roles ur JOIN iam.roles r ON r.id = ur.role_id
         WHERE ur.user_id = "users"."id" AND r.code = ANY($2::text[]) ORDER BY r.code LIMIT 1) AS "hris_role", `+permissionsEmbed+`
  FROM configuration.users WHERE "id" = $1`, userID, iam.HrisStaffRoleCodes)
	if err != nil || row == nil {
		return nil, err
	}
	perms, _ := row.Get("user_approval_permissions").(json.RawMessage)
	u := &appUser{
		ID: row.Str("id"), Role: row.Str("role"), Status: row.Str("status"),
		BrandID: row.StrPtr("brand_id"), BusinessScope: row.StrPtr("business_scope"),
		HoldingID: row.StrPtr("holding_id"), CompanyID: row.StrPtr("company_id"), BranchID: row.StrPtr("branch_id"),
		CanSwitchStall: row.Bool("can_switch_stall"), CanCentralCheckout: row.Bool("can_central_checkout"),
		DefaultWarehouseID: row.StrPtr("default_warehouse_id"), Permissions: perms,
		HrisRole: row.StrPtr("hris_role"),
	}
	for _, s := range []struct {
		id    *string
		table string
		name  **string
	}{
		{u.HoldingID, "holdings", &u.HoldingName},
		{u.CompanyID, "companies", &u.CompanyName},
		{u.BranchID, "branches", &u.BranchName},
	} {
		if s.id == nil {
			continue
		}
		err := q.QueryRow(ctx, `SELECT name FROM configuration.`+s.table+` WHERE id = $1`, *s.id).Scan(s.name)
		if err != nil && !database.IsNoRows(err) {
			return nil, err
		}
	}
	return u, nil
}

// loadWarehouses is loadUserWarehousesBatch: active stalls per user.
func loadWarehouses(ctx context.Context, q database.Querier, userIDs []string) (map[string][]warehouseRow, error) {
	out := map[string][]warehouseRow{}
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, `SELECT uw.user_id::text, uw.warehouse_id::text, w.name, w.code, w.branch_id::text
       FROM configuration.user_warehouses uw
       INNER JOIN configuration.warehouses w ON w.id = uw.warehouse_id
       WHERE uw.user_id = ANY($1::uuid[])
         AND uw.is_active = true
         AND w.is_active = true
       ORDER BY w.is_default DESC, w.name ASC`, userIDs)
	if database.IsUndefinedTable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var userID string
		var w warehouseRow
		if err := rows.Scan(&userID, &w.WarehouseID, &w.Name, &w.Code, &w.BranchID); err != nil {
			return nil, err
		}
		out[userID] = append(out[userID], w)
	}
	return out, rows.Err()
}

// usersWithRole is the role filter of listUserEmployees.
func usersWithRole(ctx context.Context, q database.Querier, ids []string, role string) (map[string]bool, error) {
	rows, err := q.Query(ctx, `SELECT id::text FROM configuration.users WHERE id = ANY($1::uuid[]) AND role = $2`, ids, role)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, pgx.RowTo[string])
	out := make(map[string]bool, len(list))
	for _, id := range list {
		out[id] = true
	}
	return out, err
}

// profileColumn reads one nullable text column of a profile (maybeSingle
// with the error ignored).
func profileColumn(ctx context.Context, db database.DB, userID, col string) *string {
	var v *string
	quietly(ctx, db, func(q database.Querier) error {
		return q.QueryRow(ctx, `SELECT "`+col+`"::text FROM configuration.users WHERE "id" = $1`, userID).Scan(&v)
	})
	return v
}

/* ── writes ──────────────────────────────────────────────────────────── */

// insertApprovals inserts the active permissions in one statement.
func insertApprovals(ctx context.Context, q database.Querier, userID, actorID string, list []approval) error {
	var values []string
	var args []any
	for _, p := range list {
		if !p.IsActive {
			continue
		}
		n := len(args)
		values = append(values, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, true, $%d, $%d)", n+1, n+2, n+3, n+4, n+5, n+6, n+7))
		args = append(args, userID, p.Module, p.Workflow, p.Level, p.Limit, actorID, actorID)
	}
	if len(values) == 0 {
		return nil
	}
	_, err := q.Exec(ctx, `INSERT INTO configuration.user_approval_permissions
  (user_id, module, workflow, approval_level, approval_limit, is_active, created_by, updated_by)
  VALUES `+strings.Join(values, ", "), args...)
	return err
}

// deactivateApprovals switches every permission of the user off.
func deactivateApprovals(ctx context.Context, q database.Querier, userID, actorID string, now time.Time) error {
	_, err := q.Exec(ctx, `UPDATE configuration.user_approval_permissions SET is_active = false, updated_by = $1, updated_at = $2
  WHERE user_id = $3`, actorID, now, userID)
	return err
}

var (
	errStallBranch    = errors.New("Branch must be selected before choosing outlets")
	errStallMissing   = errors.New("Outlet not found or inactive")
	errStallOtherSide = errors.New("Outlet must belong to the same branch as the user scope")
)

// syncWarehouses is syncUserWarehouses: validate the stalls against the
// branch, switch every assignment off, then upsert the chosen ones.
func syncWarehouses(ctx context.Context, db database.DB, userID string, ids []string, branchID *string, now time.Time) error {
	if len(ids) > 0 {
		if branchID == nil || *branchID == "" {
			return errStallBranch
		}
		rows, err := db.Query(ctx, `SELECT id::text, branch_id::text FROM configuration.warehouses
  WHERE id = ANY($1::uuid[]) AND is_active = $2`, ids, true)
		if err != nil {
			return err
		}
		found := map[string]string{}
		for rows.Next() {
			var id, branch string
			if err := rows.Scan(&id, &branch); err != nil {
				rows.Close()
				return err
			}
			found[id] = branch
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, id := range ids {
			branch, ok := found[id]
			if !ok {
				return errStallMissing
			}
			if branch != *branchID {
				return errStallOtherSide
			}
		}
	}
	clearWarehouses(ctx, db, userID, now)
	if len(ids) == 0 {
		return nil
	}
	_, err := db.Exec(ctx, `INSERT INTO configuration.user_warehouses (user_id, warehouse_id, is_active, updated_at)
  SELECT $1, w, true, $3 FROM unnest($2::uuid[]) AS w
  ON CONFLICT (user_id, warehouse_id) DO UPDATE SET is_active = EXCLUDED.is_active, updated_at = EXCLUDED.updated_at`,
		userID, ids, now)
	return err
}

// clearWarehouses is clearUserWarehouses (its error is never read).
func clearWarehouses(ctx context.Context, db database.DB, userID string, now time.Time) {
	quietly(ctx, db, func(q database.Querier) error {
		_, err := q.Exec(ctx, `UPDATE configuration.user_warehouses SET is_active = false, updated_at = $1 WHERE user_id = $2`, now, userID)
		return err
	})
}

// syncIamRole is syncIamPrimaryRole: the role code becomes the user's only
// IAM role (none when the code is unknown); the change is recorded in
// iam.permission_audit_logs (EPIC-061).
func syncIamRole(ctx context.Context, q database.Querier, userID, roleCode, assignedBy string) error {
	return permaudit.SetUserRole(ctx, q, userID, roleCode, assignedBy)
}

// audit writes admin_user_audit_logs; the TS never reads the insert error.
func audit(ctx context.Context, db database.DB, actorID, targetID, action string, details *kit.Row) {
	raw, err := kit.Marshal(details)
	if err != nil {
		return
	}
	quietly(ctx, db, func(q database.Querier) error {
		_, err := q.Exec(ctx, `INSERT INTO configuration.admin_user_audit_logs (actor_id, target_user_id, action, details)
  VALUES ($1, $2, $3, $4::jsonb)`, actorID, targetID, action, string(raw))
		return err
	})
}

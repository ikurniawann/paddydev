package access

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/permaudit"
)

// SQL ported from lib/iam/role-repository.ts and menu-repository.ts.

const roleSelect = `SELECT r.id::text, r.code, r.name, r.description, r.is_system, r.is_active,
      (SELECT COUNT(*)::int FROM iam.role_menu_permissions rmp
        WHERE rmp.role_id = r.id AND rmp.is_active = true),
      r.created_at, r.updated_at, r.version
     FROM iam.roles r`

func scanRole(row pgx.CollectableRow) (roleRow, error) {
	var r roleRow
	err := row.Scan(&r.ID, &r.Code, &r.Name, &r.Description, &r.IsSystem, &r.IsActive,
		&r.MenuPermissionCount, &r.CreatedAt, &r.UpdatedAt, &r.Version)
	return r, err
}

func listRoles(ctx context.Context, q database.Querier) ([]roleItem, error) {
	rows, err := q.Query(ctx, roleSelect+` WHERE r.deleted_at IS NULL ORDER BY r.code ASC`)
	if err != nil {
		return nil, err
	}
	full, err := pgx.CollectRows(rows, scanRole)
	out := make([]roleItem, len(full))
	for i, r := range full {
		out[i] = r.roleItem
	}
	return out, err
}

// getRole returns nil when the role is missing or soft-deleted.
func getRole(ctx context.Context, q database.Querier, id string) (*roleRow, error) {
	rows, err := q.Query(ctx, roleSelect+` WHERE r.id = $1 AND r.deleted_at IS NULL`, id)
	if err != nil {
		return nil, err
	}
	r, err := pgx.CollectExactlyOneRow(rows, scanRole)
	if database.IsNoRows(err) {
		return nil, nil
	}
	return &r, err
}

func permissionMatrix(ctx context.Context, q database.Querier, roleID string) ([]permissionRow, error) {
	rows, err := q.Query(ctx, `SELECT m.id::text, m.code, m.menu_name, m.menu_type, m.parent_id::text, m.level,
      m.order_number, m.permission_context, rmp.granted_actions, rmp.id::text, rmp.is_active
     FROM iam.menus m
     LEFT JOIN iam.role_menu_permissions rmp ON rmp.menu_id = m.id AND rmp.role_id = $1
     WHERE m.deleted_at IS NULL
     ORDER BY m.level ASC, m.order_number ASC, m.menu_name ASC`, roleID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (permissionRow, error) {
		var p permissionRow
		err := row.Scan(&p.MenuID, &p.MenuCode, &p.MenuName, &p.MenuType, &p.ParentID, &p.Level, &p.OrderNumber,
			&p.PermissionContext, &p.GrantedActions, &p.PermissionID, &p.PermissionIsActive)
		return p, err
	})
}

func createRole(ctx context.Context, q database.Querier, code, name string, description *string, isActive bool) (string, error) {
	var id string
	err := q.QueryRow(ctx, `INSERT INTO iam.roles (code, name, description, is_system, is_active)
     VALUES ($1, $2, $3, false, $4)
     RETURNING id::text`, code, name, description, isActive).Scan(&id)
	return id, err
}

// updateRole reports whether a live role was updated.
func updateRole(ctx context.Context, q database.Querier, id string, code, name *string, in roleInput) (bool, error) {
	tag, err := q.Exec(ctx, `UPDATE iam.roles SET
      code = COALESCE($2, code),
      name = COALESCE($3, name),
      description = CASE WHEN $4::boolean THEN $5 ELSE description END,
      is_active = COALESCE($6, is_active),
      updated_at = now(),
      version = version + 1
     WHERE id = $1 AND deleted_at IS NULL`, id, code, name, in.DescriptionSent, in.Description, in.IsActive)
	return tag.RowsAffected() > 0, err
}

// errSystemRole is the plain Error deleteIamRoleInDb throws; the route turns
// it into a 400 with the same message.
const errSystemRole = "System roles cannot be deleted"

// deleteRole soft-deletes a role. found is false for a missing role;
// system is true when the role is a system role (nothing changes).
func deleteRole(ctx context.Context, q database.Querier, id string) (found, system bool, err error) {
	err = q.QueryRow(ctx, `SELECT is_system FROM iam.roles WHERE id = $1 AND deleted_at IS NULL`, id).Scan(&system)
	if database.IsNoRows(err) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if system {
		return true, true, nil
	}
	tag, err := q.Exec(ctx, `UPDATE iam.roles SET
      deleted_at = now(), is_active = false, updated_at = now(), version = version + 1
     WHERE id = $1 AND deleted_at IS NULL`, id)
	return tag.RowsAffected() > 0, false, err
}

// errAdminHrisEmpty: admin_hris is the source of the HR roles; the sync
// refuses an empty source (ruling R10).
const errAdminHrisEmpty = "admin_hris harus punya minimal satu menu: role HR lain diturunkan darinya"

// hrisRoleIDs are the ids of admin_hris and the roles derived from it.
func hrisRoleIDs(ctx context.Context, q database.Querier) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT id::text FROM iam.roles WHERE code = ANY($1) AND deleted_at IS NULL ORDER BY code`, iam.HrisStaffRoleCodes)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// addsHrisDelete reports whether grants add "delete" on an HRIS menu the
// role does not already hold it on (an active grant with "delete").
func addsHrisDelete(ctx context.Context, q database.Querier, roleID string, grants []grant) (bool, error) {
	var ids []string
	for _, g := range grants {
		if slices.Contains(g.Actions, "delete") {
			ids = append(ids, g.MenuID)
		}
	}
	if len(ids) == 0 {
		return false, nil
	}
	var added bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM iam.menus m
		WHERE m.id::text = ANY($2::text[]) AND m.code LIKE 'hris.%'
		  AND NOT EXISTS (SELECT 1 FROM iam.role_menu_permissions p
		    WHERE p.role_id = $1 AND p.menu_id = m.id AND p.is_active AND p.granted_actions ? 'delete'))`, roleID, ids).Scan(&added)
	return added, err
}

// replacePermissions is replaceIamRolePermissionsInDb: deactivate every
// grant of the role, then upsert the checked ones, in one transaction. For
// admin_hris it re-runs the HR role sync; every grant change is recorded in
// iam.permission_audit_logs (EPIC-061).
func replacePermissions(ctx context.Context, db database.DB, roleID string, grants []grant, updatedBy string) error {
	return database.WithTx(ctx, db, func(tx pgx.Tx) error {
		var code string
		if err := tx.QueryRow(ctx, `SELECT code FROM iam.roles WHERE id = $1 FOR UPDATE`, roleID).Scan(&code); err != nil {
			return err
		}
		source := code == "admin_hris"
		watched := []string{roleID}
		if source {
			if len(grants) == 0 {
				return httpx.BadRequest(errAdminHrisEmpty)
			}
			ids, err := hrisRoleIDs(ctx, tx)
			if err != nil {
				return err
			}
			watched = ids
		}
		before, err := permaudit.Snapshot(ctx, tx, watched)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE iam.role_menu_permissions
       SET is_active = false, updated_at = now(), updated_by = $2
       WHERE role_id = $1`, roleID, updatedBy); err != nil {
			return err
		}
		for _, g := range grants {
			actions, _ := json.Marshal(g.Actions)
			if _, err := tx.Exec(ctx, `INSERT INTO iam.role_menu_permissions (
          role_id, menu_id, granted_actions, is_active, created_by, updated_by, updated_at
        ) VALUES ($1, $2, $3::jsonb, true, $4, $4, now())
        ON CONFLICT (role_id, menu_id) DO UPDATE SET
          granted_actions = EXCLUDED.granted_actions,
          is_active = true,
          updated_by = EXCLUDED.updated_by,
          updated_at = now()`, roleID, g.MenuID, string(actions), updatedBy); err != nil {
				return err
			}
		}
		if source {
			if _, err := tx.Exec(ctx, `SELECT iam.sync_hris_staff_roles()`); err != nil {
				return err
			}
		}
		after, err := permaudit.Snapshot(ctx, tx, watched)
		if err != nil {
			return err
		}
		_, err = permaudit.Record(ctx, tx, updatedBy, permaudit.SourceRolePermissions, before, after)
		return err
	})
}

const menuSelect = `SELECT id::text, parent_id::text, code, menu_name, route_path, module, menu_type, icon,
      order_number, level, is_active, is_visible, description, permission_context, open_in_new_tab,
      created_at, updated_at, version
     FROM iam.menus`

func scanMenu(row pgx.CollectableRow) (menuRow, error) {
	var m menuRow
	err := row.Scan(&m.ID, &m.ParentID, &m.Code, &m.MenuName, &m.RoutePath, &m.Module, &m.MenuType, &m.Icon,
		&m.OrderNumber, &m.Level, &m.IsActive, &m.IsVisible, &m.Description, &m.PermissionContext, &m.OpenInNewTab,
		&m.CreatedAt, &m.UpdatedAt, &m.Version)
	return m, err
}

func listMenus(ctx context.Context, q database.Querier) ([]menuItem, error) {
	rows, err := q.Query(ctx, menuSelect+` WHERE deleted_at IS NULL ORDER BY level ASC, order_number ASC, menu_name ASC`)
	if err != nil {
		return nil, err
	}
	full, err := pgx.CollectRows(rows, scanMenu)
	out := make([]menuItem, len(full))
	for i, m := range full {
		out[i] = m.menuItem
	}
	return out, err
}

// getMenu returns nil when the menu is missing or soft-deleted.
func getMenu(ctx context.Context, q database.Querier, id string) (*menuRow, error) {
	rows, err := q.Query(ctx, menuSelect+` WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return nil, err
	}
	m, err := pgx.CollectExactlyOneRow(rows, scanMenu)
	if database.IsNoRows(err) {
		return nil, nil
	}
	return &m, err
}

// permissionContextJSON is JSON.stringify(permissionContext) of the zod
// output, which keeps only `actions`.
func permissionContextJSON(actions []string) *string {
	if actions == nil {
		return nil
	}
	raw, _ := json.Marshal(struct {
		Actions []string `json:"actions"`
	}{actions})
	s := string(raw)
	return &s
}

func createMenu(ctx context.Context, q database.Querier, in menuInput, createdBy string) (string, error) {
	menuType, order, visible, active, newTab := "sidebar", 0, true, true, false
	if in.MenuType != nil {
		menuType = *in.MenuType
	}
	if in.OrderNumber != nil {
		order = *in.OrderNumber
	}
	if in.IsVisible != nil {
		visible = *in.IsVisible
	}
	if in.IsActive != nil {
		active = *in.IsActive
	}
	if in.OpenInNewTab != nil {
		newTab = *in.OpenInNewTab
	}
	ctxJSON := permissionContextJSON(in.PermissionActions)
	if ctxJSON == nil {
		ctxJSON = permissionContextJSON([]string{"read"})
	}
	var id string
	err := q.QueryRow(ctx, `INSERT INTO iam.menus (
      parent_id, code, menu_name, description, route_path, module,
      menu_type, icon, order_number, is_visible, is_active,
      open_in_new_tab, permission_context, created_by
    ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
    RETURNING id::text`,
		in.ParentID, in.Code, in.MenuName, in.Description, in.RoutePath, in.Module,
		menuType, in.Icon, order, visible, active, newTab, *ctxJSON, createdBy).Scan(&id)
	return id, err
}

// updateMenu reports whether a live menu was updated.
func updateMenu(ctx context.Context, q database.Querier, id string, in menuInput, updatedBy string) (bool, error) {
	tag, err := q.Exec(ctx, `UPDATE iam.menus SET
      parent_id = CASE WHEN $2::boolean THEN $3::uuid ELSE parent_id END,
      code = COALESCE($4, code),
      menu_name = COALESCE($5, menu_name),
      description = CASE WHEN $6::boolean THEN $7 ELSE description END,
      route_path = CASE WHEN $8::boolean THEN $9 ELSE route_path END,
      module = CASE WHEN $10::boolean THEN $11 ELSE module END,
      menu_type = COALESCE($12, menu_type),
      icon = CASE WHEN $13::boolean THEN $14 ELSE icon END,
      order_number = COALESCE($15, order_number),
      is_visible = COALESCE($16, is_visible),
      is_active = COALESCE($17, is_active),
      open_in_new_tab = COALESCE($18, open_in_new_tab),
      permission_context = COALESCE($19::jsonb, permission_context),
      updated_by = $20,
      updated_at = now()
    WHERE id = $1 AND deleted_at IS NULL`,
		id, in.sent["parentId"], in.ParentID, in.Code, in.MenuName,
		in.sent["description"], in.Description, in.sent["routePath"], in.RoutePath,
		in.sent["module"], in.Module, in.MenuType, in.sent["icon"], in.Icon,
		in.OrderNumber, in.IsVisible, in.IsActive, in.OpenInNewTab,
		permissionContextJSON(in.PermissionActions), updatedBy)
	return tag.RowsAffected() > 0, err
}

// deleteMenu soft-deletes a menu; false when it is missing.
func deleteMenu(ctx context.Context, q database.Querier, id, deletedBy string) (bool, error) {
	tag, err := q.Exec(ctx, `UPDATE iam.menus SET
      deleted_at = now(), deleted_by = $2, is_active = false, updated_at = now(), updated_by = $2
    WHERE id = $1 AND deleted_at IS NULL`, id, deletedBy)
	return tag.RowsAffected() > 0, err
}

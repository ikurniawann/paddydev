package access

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/permaudit"
)

type hrisRoleRow struct {
	ID, Code, Name string
	IsSystem       bool
}

type scopeMenu struct {
	ID, Code, Name string
	ParentCode     *string
	Level          int
	Actions        []string
	AccessToggle   bool
	DeleteToggle   bool
}

type hrisAccessRole struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type hrisAccessMenu struct {
	Code         string  `json:"code"`
	Name         string  `json:"name"`
	ParentCode   *string `json:"parent_code"`
	Level        int     `json:"level"`
	AccessToggle bool    `json:"access_toggle"`
	DeleteToggle bool    `json:"delete_toggle"`
}

type hrisAccessAccount struct {
	UserID   string  `json:"user_id"`
	FullName string  `json:"full_name"`
	Email    *string `json:"email"`
	HrisRole *string `json:"hris_role"`
}

type hrisAccessView struct {
	Roles    []hrisAccessRole               `json:"roles"`
	Menus    []hrisAccessMenu               `json:"menus"`
	Grants   map[string]map[string][]string `json:"grants"`
	Accounts []hrisAccessAccount            `json:"accounts"`
}

type permissionAuditRow struct {
	ID            string          `json:"id"`
	Source        string          `json:"source"`
	ActorName     *string         `json:"actor_name"`
	RoleCode      *string         `json:"role_code"`
	MenuCode      *string         `json:"menu_code"`
	ActionsBefore json.RawMessage `json:"actions_before"`
	ActionsAfter  json.RawMessage `json:"actions_after"`
	TargetName    *string         `json:"target_name"`
	RolesBefore   []string        `json:"roles_before"`
	RolesAfter    []string        `json:"roles_after"`
	CreatedAt     httpx.JSTime    `json:"created_at"`
}

// loadHrisRoles lists admin_hris and the hris_* roles in HrisStaffRoleCodes order.
func loadHrisRoles(ctx context.Context, q database.Querier) ([]hrisRoleRow, error) {
	rows, err := q.Query(ctx, `SELECT id::text, code, name, is_system FROM iam.roles WHERE code = ANY($1) AND deleted_at IS NULL`, iam.HrisStaffRoleCodes)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (hrisRoleRow, error) {
		var r hrisRoleRow
		err := row.Scan(&r.ID, &r.Code, &r.Name, &r.IsSystem)
		return r, err
	})
	slices.SortFunc(out, func(a, b hrisRoleRow) int {
		return slices.Index(iam.HrisStaffRoleCodes, a.Code) - slices.Index(iam.HrisStaffRoleCodes, b.Code)
	})
	return out, err
}

// loadScopeMenus lists the HRIS menus AdminHRIS may toggle: access in the
// three scopes (groups included), delete on HRIS leaves offering it.
func loadScopeMenus(ctx context.Context, q database.Querier) ([]scopeMenu, error) {
	rows, err := q.Query(ctx, `SELECT m.id::text, m.code, m.menu_name, p.code, m.level,
		  coalesce(m.permission_context->'actions', '[]'::jsonb),
		  NOT EXISTS (SELECT 1 FROM iam.menus c WHERE c.parent_id = m.id AND c.deleted_at IS NULL)
		FROM iam.menus m LEFT JOIN iam.menus p ON p.id = m.parent_id
		WHERE m.deleted_at IS NULL AND m.code LIKE 'hris.%'
		ORDER BY m.code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []scopeMenu
	for rows.Next() {
		var m scopeMenu
		var raw []byte
		var leaf bool
		if err := rows.Scan(&m.ID, &m.Code, &m.Name, &m.ParentCode, &m.Level, &raw, &leaf); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &m.Actions)
		m.AccessToggle = iam.HasAnyMenuPrefix([]string{m.Code}, iam.HrisAccessScopes)
		m.DeleteToggle = leaf && slices.Contains(m.Actions, "delete")
		if m.AccessToggle || m.DeleteToggle {
			out = append(out, m)
		}
	}
	return out, rows.Err()
}

func loadHrisAccounts(ctx context.Context, q database.Querier) ([]hrisAccessAccount, error) {
	rows, err := q.Query(ctx, `SELECT u.id::text, u.full_name, u.email,
		  (SELECT r.code FROM iam.user_roles ur JOIN iam.roles r ON r.id = ur.role_id
		    WHERE ur.user_id = u.id AND r.code = ANY($1) ORDER BY r.code LIMIT 1)
		FROM configuration.users u
		WHERE u.role = 'hrd' OR EXISTS (SELECT 1 FROM iam.user_roles ur JOIN iam.roles r ON r.id = ur.role_id
		  WHERE ur.user_id = u.id AND r.code = ANY($1))
		ORDER BY u.full_name, u.id`, iam.HrisStaffRoleCodes)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (hrisAccessAccount, error) {
		var a hrisAccessAccount
		err := row.Scan(&a.UserID, &a.FullName, &a.Email, &a.HrisRole)
		return a, err
	})
	if out == nil {
		out = []hrisAccessAccount{}
	}
	return out, err
}

func hrisAccess(ctx context.Context, q database.Querier) (*hrisAccessView, error) {
	roles, err := loadHrisRoles(ctx, q)
	if err != nil {
		return nil, err
	}
	menus, err := loadScopeMenus(ctx, q)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(roles))
	for i, r := range roles {
		ids[i] = r.ID
	}
	snap, err := permaudit.Snapshot(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	accounts, err := loadHrisAccounts(ctx, q)
	if err != nil {
		return nil, err
	}
	v := &hrisAccessView{Roles: []hrisAccessRole{}, Menus: []hrisAccessMenu{}, Grants: map[string]map[string][]string{}, Accounts: accounts}
	inScope := map[string]bool{}
	for _, m := range menus {
		inScope[m.Code] = true
		v.Menus = append(v.Menus, hrisAccessMenu{Code: m.Code, Name: m.Name, ParentCode: m.ParentCode, Level: m.Level,
			AccessToggle: m.AccessToggle, DeleteToggle: m.DeleteToggle})
	}
	for _, r := range roles {
		v.Roles = append(v.Roles, hrisAccessRole{Code: r.Code, Name: r.Name})
		g := map[string][]string{}
		for code, acts := range snap[r.ID] {
			if inScope[code] {
				g[code] = acts
			}
		}
		v.Grants[r.Code] = g
	}
	return v, nil
}

// applyAdminHris changes the source role's own grants (ruling R8).
func applyAdminHris(ctx context.Context, tx pgx.Tx, roleID string, m scopeMenu, c hrisAccessChange, actorID string) error {
	if c.Access != nil && *c.Access {
		acts := slices.DeleteFunc(slices.Clone(m.Actions), func(a string) bool { return a == "delete" })
		raw, _ := json.Marshal(acts)
		if _, err := tx.Exec(ctx, `INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions, is_active, created_by, updated_by, updated_at)
			VALUES ($1, $2, $3::jsonb, true, $4, $4, now())
			ON CONFLICT (role_id, menu_id) DO UPDATE SET
			  granted_actions = CASE WHEN role_menu_permissions.is_active THEN role_menu_permissions.granted_actions ELSE EXCLUDED.granted_actions END,
			  is_active = true, updated_by = EXCLUDED.updated_by, updated_at = now()`, roleID, m.ID, string(raw), actorID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions, is_active, created_by, updated_by, updated_at)
			SELECT $1, p.id, '["read"]'::jsonb, true, $3, $3, now() FROM iam.menus p
			WHERE p.deleted_at IS NULL AND $2 LIKE p.code || '.%'
			ON CONFLICT (role_id, menu_id) DO UPDATE SET granted_actions = EXCLUDED.granted_actions, is_active = true,
			  updated_by = EXCLUDED.updated_by, updated_at = now()
			WHERE NOT role_menu_permissions.is_active`, roleID, m.Code, actorID); err != nil {
			return err
		}
	}
	if c.Access != nil && !*c.Access {
		if _, err := tx.Exec(ctx, `UPDATE iam.role_menu_permissions p SET is_active = false, updated_by = $3, updated_at = now()
			FROM iam.menus m WHERE m.id = p.menu_id AND p.role_id = $1 AND p.is_active
			  AND (m.code = $2 OR m.code LIKE $2 || '.%')`, roleID, m.Code, actorID); err != nil {
			return err
		}
	}
	if c.Delete != nil {
		var active bool
		if err := tx.QueryRow(ctx, `SELECT coalesce((SELECT is_active FROM iam.role_menu_permissions WHERE role_id = $1 AND menu_id = $2), false)`,
			roleID, m.ID).Scan(&active); err != nil {
			return err
		}
		if !active {
			return httpx.BadRequest(errHrisAccessFirst)
		}
		sql := `UPDATE iam.role_menu_permissions SET granted_actions = granted_actions - 'delete', updated_by = $3, updated_at = now()
			WHERE role_id = $1 AND menu_id = $2`
		if *c.Delete {
			sql = `UPDATE iam.role_menu_permissions SET granted_actions = granted_actions || '["delete"]'::jsonb, updated_by = $3, updated_at = now()
			WHERE role_id = $1 AND menu_id = $2 AND NOT granted_actions ? 'delete'`
		}
		if _, err := tx.Exec(ctx, sql, roleID, m.ID, actorID); err != nil {
			return err
		}
	}
	return nil
}

// resolveMask is the mask row deciding code for the role (the longest
// matching prefix), the zero mask when none.
func resolveMask(ctx context.Context, q database.Querier, roleCode, code string) (permaudit.Mask, error) {
	var m permaudit.Mask
	err := q.QueryRow(ctx, `SELECT exclude_menu, excluded_actions FROM iam.hris_staff_role_masks
		WHERE role_code = $1 AND ($2 = menu_prefix OR $2 LIKE menu_prefix || '.%')
		ORDER BY length(menu_prefix) DESC LIMIT 1`, roleCode, code).Scan(&m.ExcludeMenu, &m.ExcludedActions)
	if database.IsNoRows(err) {
		return permaudit.Mask{ExcludedActions: []string{}}, nil
	}
	return m, err
}

func exactMask(ctx context.Context, q database.Querier, roleCode, code string) (*permaudit.Mask, error) {
	var m permaudit.Mask
	err := q.QueryRow(ctx, `SELECT exclude_menu, excluded_actions FROM iam.hris_staff_role_masks WHERE role_code = $1 AND menu_prefix = $2`,
		roleCode, code).Scan(&m.ExcludeMenu, &m.ExcludedActions)
	if database.IsNoRows(err) {
		return nil, nil
	}
	return &m, err
}

// inheritedExclusions is every action excluded by a mask row on code or
// one of its parents, for the role.
func inheritedExclusions(ctx context.Context, q database.Querier, roleCode, code string) ([]string, error) {
	var out []string
	err := q.QueryRow(ctx, `SELECT coalesce(array_agg(DISTINCT a ORDER BY a), '{}') FROM iam.hris_staff_role_masks k, unnest(k.excluded_actions) a
		WHERE k.role_code = $1 AND ($2 = k.menu_prefix OR $2 LIKE k.menu_prefix || '.%')`, roleCode, code).Scan(&out)
	return out, err
}

// applyMask changes a derived role through its mask (ruling R8).
func applyMask(ctx context.Context, tx pgx.Tx, roleCode string, c hrisAccessChange, actorID string) error {
	cur, err := resolveMask(ctx, tx, roleCode, c.MenuCode)
	if err != nil {
		return err
	}
	exact, err := exactMask(ctx, tx, roleCode, c.MenuCode)
	if err != nil {
		return err
	}
	next := permaudit.Mask{ExcludeMenu: cur.ExcludeMenu, ExcludedActions: slices.Clone(cur.ExcludedActions)}
	if c.Access != nil {
		next.ExcludeMenu = !*c.Access
		if *c.Access && cur.ExcludeMenu {
			// Showing a hidden menu never grants an action by itself: it keeps
			// every reduction of the masks above it, so "delete" comes back
			// only through an explicit delete toggle (ruling M5, Q-RA-2).
			if next.ExcludedActions, err = inheritedExclusions(ctx, tx, roleCode, c.MenuCode); err != nil {
				return err
			}
		}
		if !*c.Access {
			rows, err := tx.Query(ctx, `DELETE FROM iam.hris_staff_role_masks WHERE role_code = $1 AND menu_prefix LIKE $2 || '.%'
				RETURNING menu_prefix, exclude_menu, excluded_actions`, roleCode, c.MenuCode)
			if err != nil {
				return err
			}
			type dropped struct {
				prefix string
				mask   permaudit.Mask
			}
			gone, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (dropped, error) {
				var d dropped
				err := row.Scan(&d.prefix, &d.mask.ExcludeMenu, &d.mask.ExcludedActions)
				return d, err
			})
			if err != nil {
				return err
			}
			for _, d := range gone {
				if err := permaudit.RecordMask(ctx, tx, actorID, roleCode, d.prefix, &d.mask, nil); err != nil {
					return err
				}
			}
		}
	}
	if c.Delete != nil {
		if next.ExcludeMenu {
			return httpx.BadRequest(errHrisAccessFirst)
		}
		next.ExcludedActions = slices.DeleteFunc(next.ExcludedActions, func(a string) bool { return a == "delete" })
		if !*c.Delete {
			next.ExcludedActions = append(next.ExcludedActions, "delete")
		}
	}
	if next.ExcludedActions == nil {
		next.ExcludedActions = []string{}
	}
	slices.Sort(next.ExcludedActions)
	if _, err := tx.Exec(ctx, `INSERT INTO iam.hris_staff_role_masks (role_code, menu_prefix, exclude_menu, excluded_actions, updated_by, updated_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (role_code, menu_prefix) DO UPDATE SET exclude_menu = EXCLUDED.exclude_menu,
		  excluded_actions = EXCLUDED.excluded_actions, updated_by = EXCLUDED.updated_by, updated_at = now()`,
		roleCode, c.MenuCode, next.ExcludeMenu, next.ExcludedActions, actorID); err != nil {
		return err
	}
	return permaudit.RecordMask(ctx, tx, actorID, roleCode, c.MenuCode, exact, &next)
}

func listPermissionAudit(ctx context.Context, q database.Querier, all bool, limit int) ([]permissionAuditRow, error) {
	rows, err := q.Query(ctx, `SELECT l.id::text, l.source, a.full_name, l.role_code, l.menu_code, l.actions_before, l.actions_after,
		  t.full_name, l.roles_before, l.roles_after, l.created_at
		FROM iam.permission_audit_logs l
		LEFT JOIN configuration.users a ON a.id = l.actor_id
		LEFT JOIN configuration.users t ON t.id = l.target_user_id
		WHERE $1 OR l.role_code = ANY($2::text[]) OR l.roles_before && $2::text[] OR l.roles_after && $2::text[]
		ORDER BY l.created_at DESC, l.id
		LIMIT $3`, all, iam.HrisStaffRoleCodes, limit)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (permissionAuditRow, error) {
		var r permissionAuditRow
		var before, after []byte
		var at time.Time
		err := row.Scan(&r.ID, &r.Source, &r.ActorName, &r.RoleCode, &r.MenuCode, &before, &after,
			&r.TargetName, &r.RolesBefore, &r.RolesAfter, &at)
		r.ActionsBefore, r.ActionsAfter, r.CreatedAt = before, after, httpx.JSTime(at)
		return r, err
	})
	if out == nil {
		out = []permissionAuditRow{}
	}
	return out, err
}

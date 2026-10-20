// Package permaudit records IAM permission changes in
// iam.permission_audit_logs (EPIC-061, Q-RA-8): per role and menu the
// actions before and after, HRIS role mask changes, and user role
// assignments.
package permaudit

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/platform/database"
)

// Sources, the iam.permission_audit_logs.source values.
const (
	SourceRolePermissions = "role_permissions"
	SourceHrisAccess      = "hris_access"
	SourceHrisMask        = "hris_mask"
	SourceUserRole        = "user_role"
)

// Grants is role id → menu code → sorted active actions.
type Grants map[string]map[string][]string

// Mask is one iam.hris_staff_role_masks row.
type Mask struct {
	ExcludeMenu     bool     `json:"exclude_menu"`
	ExcludedActions []string `json:"excluded_actions"`
}

// Snapshot reads the active grants of the roles.
func Snapshot(ctx context.Context, q database.Querier, roleIDs []string) (Grants, error) {
	out := Grants{}
	for _, id := range roleIDs {
		out[id] = map[string][]string{}
	}
	if len(roleIDs) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, `SELECT p.role_id::text, m.code, p.granted_actions
		FROM iam.role_menu_permissions p JOIN iam.menus m ON m.id = p.menu_id
		WHERE p.role_id = ANY($1::uuid[]) AND p.is_active AND m.deleted_at IS NULL`, roleIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var role, code string
		var raw []byte
		if err := rows.Scan(&role, &code, &raw); err != nil {
			return nil, err
		}
		var acts []string
		_ = json.Unmarshal(raw, &acts)
		slices.Sort(acts)
		out[role][code] = slices.Compact(acts)
	}
	return out, rows.Err()
}

// Record writes one row per (role, menu) whose actions changed between
// before and after; a grant that appeared has actions_before NULL, one
// that went away actions_after NULL. It returns the rows written.
func Record(ctx context.Context, q database.Querier, actorID, source string, before, after Grants) (int, error) {
	n := 0
	for _, role := range keys(before, after) {
		b, a := before[role], after[role]
		for _, code := range keys(b, a) {
			ba, inB := b[code]
			aa, inA := a[code]
			if inB == inA && slices.Equal(ba, aa) {
				continue
			}
			if _, err := q.Exec(ctx, `INSERT INTO iam.permission_audit_logs
				(actor_id, source, role_id, role_code, menu_code, actions_before, actions_after)
				VALUES ($1, $2, $3::uuid, (SELECT code FROM iam.roles WHERE id = $3::uuid), $4, $5::jsonb, $6::jsonb)`,
				nullable(actorID), source, role, code, actionsJSON(ba, inB), actionsJSON(aa, inA)); err != nil {
				return n, err
			}
			n++
		}
	}
	return n, nil
}

// RecordMask writes one hris_mask row when a mask row changed (nil = no row).
func RecordMask(ctx context.Context, q database.Querier, actorID, roleCode, prefix string, before, after *Mask) error {
	if sameMask(before, after) {
		return nil
	}
	_, err := q.Exec(ctx, `INSERT INTO iam.permission_audit_logs
		(actor_id, source, role_id, role_code, menu_code, actions_before, actions_after)
		VALUES ($1, 'hris_mask', (SELECT id FROM iam.roles WHERE code = $2), $2, $3, $4::jsonb, $5::jsonb)`,
		nullable(actorID), roleCode, prefix, maskJSON(before), maskJSON(after))
	return err
}

// UserRoleCodes lists the user's iam.user_roles role codes, sorted.
func UserRoleCodes(ctx context.Context, q database.Querier, userID string) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT r.code FROM iam.user_roles ur JOIN iam.roles r ON r.id = ur.role_id
		WHERE ur.user_id = $1 ORDER BY r.code`, userID)
	if err != nil {
		return nil, err
	}
	codes, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if codes == nil {
		codes = []string{}
	}
	return codes, err
}

// SetUserRole makes roleCode the user's only IAM role (none when the code
// is unknown), as syncIamPrimaryRole did, and records the change.
func SetUserRole(ctx context.Context, q database.Querier, userID, roleCode, assignedBy string) error {
	if userID == "" || roleCode == "" {
		return nil
	}
	before, err := UserRoleCodes(ctx, q, userID)
	if err != nil {
		return err
	}
	var roleID *string
	err = q.QueryRow(ctx, `SELECT id::text FROM iam.roles WHERE code = $1 AND deleted_at IS NULL LIMIT 1`, roleCode).Scan(&roleID)
	if err != nil && !database.IsNoRows(err) {
		return err
	}
	if _, err := q.Exec(ctx, `DELETE FROM iam.user_roles WHERE user_id = $1`, userID); err != nil {
		return err
	}
	if roleID != nil {
		if _, err := q.Exec(ctx, `INSERT INTO iam.user_roles (user_id, role_id, is_primary, assigned_by) VALUES ($1, $2, true, $3)`,
			userID, *roleID, nullable(assignedBy)); err != nil {
			return err
		}
	}
	after, err := UserRoleCodes(ctx, q, userID)
	if err != nil || slices.Equal(before, after) {
		return err
	}
	_, err = q.Exec(ctx, `INSERT INTO iam.permission_audit_logs (actor_id, source, target_user_id, roles_before, roles_after)
		VALUES ($1, 'user_role', $2, $3, $4)`, nullable(assignedBy), userID, before, after)
	return err
}

func keys[V any](maps ...map[string]V) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range maps {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	slices.Sort(out)
	return out
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func actionsJSON(acts []string, present bool) *string {
	if !present {
		return nil
	}
	if acts == nil {
		acts = []string{}
	}
	raw, _ := json.Marshal(acts)
	s := string(raw)
	return &s
}

func maskJSON(m *Mask) *string {
	if m == nil {
		return nil
	}
	c := Mask{ExcludeMenu: m.ExcludeMenu, ExcludedActions: slices.Sorted(slices.Values(m.ExcludedActions))}
	if c.ExcludedActions == nil {
		c.ExcludedActions = []string{}
	}
	raw, _ := json.Marshal(c)
	s := string(raw)
	return &s
}

func sameMask(a, b *Mask) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.ExcludeMenu == b.ExcludeMenu &&
		slices.Equal(slices.Sorted(slices.Values(a.ExcludedActions)), slices.Sorted(slices.Values(b.ExcludedActions)))
}

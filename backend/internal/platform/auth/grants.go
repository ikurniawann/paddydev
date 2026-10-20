package auth

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

// ResolveRoleIDs mirrors resolveRoleIds: iam.user_roles rows, else the
// iam.roles row whose code is configuration.users.role. There is no
// super_admin shortcut in the API guards; super_admin passes because its
// role is granted every menu.
func (s *Service) ResolveRoleIDs(ctx context.Context, userID, role string) ([]string, error) {
	ids, err := s.queryStrings(ctx, `SELECT role_id::text FROM iam.user_roles WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	if len(ids) > 0 {
		return ids, nil
	}
	var id string
	err = s.db.QueryRow(ctx, `SELECT id::text FROM iam.roles WHERE code = $1 AND deleted_at IS NULL LIMIT 1`, role).Scan(&id)
	if isNoRows(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	return []string{id}, nil
}

// HasRoleCode reports whether the user's resolved IAM roles include code
// (EPIC-061: system roles change only through super_admin).
func (s *Service) HasRoleCode(ctx context.Context, u *User, code string) (bool, error) {
	roleIDs, err := s.ResolveRoleIDs(ctx, u.ID, u.Role)
	if err != nil || len(roleIDs) == 0 {
		return false, err
	}
	var ok bool
	err = s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM iam.roles WHERE id = ANY($1::uuid[]) AND code = $2 AND deleted_at IS NULL)`,
		roleIDs, code).Scan(&ok)
	return ok, err
}

// GrantedMenuCodes mirrors loadGrantedMenuCodesForUser.
func (s *Service) GrantedMenuCodes(ctx context.Context, userID, role string) ([]string, error) {
	roleIDs, err := s.ResolveRoleIDs(ctx, userID, role)
	if err != nil {
		return nil, err
	}
	return s.GrantedMenuCodesForRoles(ctx, roleIDs)
}

// GrantedMenuCodesForRoles mirrors loadGrantedMenuCodes.
func (s *Service) GrantedMenuCodesForRoles(ctx context.Context, roleIDs []string) ([]string, error) {
	if len(roleIDs) == 0 {
		return []string{}, nil
	}
	return s.queryStrings(ctx, `SELECT DISTINCT m.code
     FROM iam.role_menu_permissions rmp
     JOIN iam.menus m ON m.id = rmp.menu_id
     WHERE rmp.role_id = ANY($1::uuid[])
       AND rmp.is_active = true
       AND m.deleted_at IS NULL
       AND m.is_active = true`, roleIDs)
}

// queryStrings runs a one-column text query; no rows is an empty slice.
func (s *Service) queryStrings(ctx context.Context, sql string, args ...any) ([]string, error) {
	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// GrantedMenuActions mirrors loadGrantedMenuActions: menu code to the union
// of granted_actions strings across the roles.
func (s *Service) GrantedMenuActions(ctx context.Context, roleIDs []string) (map[string][]string, error) {
	out := map[string][]string{}
	if len(roleIDs) == 0 {
		return out, nil
	}
	rows, err := s.db.Query(ctx, `SELECT m.code, rmp.granted_actions
     FROM iam.role_menu_permissions rmp
     JOIN iam.menus m ON m.id = rmp.menu_id
     WHERE rmp.role_id = ANY($1::uuid[])
       AND rmp.is_active = true
       AND m.deleted_at IS NULL
       AND m.is_active = true`, roleIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var code string
		var raw []byte
		if err := rows.Scan(&code, &raw); err != nil {
			return nil, err
		}
		out[code] = mergeUnique(out[code], parseGrantedActions(raw))
	}
	return out, rows.Err()
}

// parseGrantedActions keeps the string items of a JSON array; anything
// else is an empty list.
func parseGrantedActions(raw []byte) []string {
	var items []any
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil
	}
	var out []string
	for _, it := range items {
		if s, ok := it.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func mergeUnique(current, next []string) []string {
	seen := make(map[string]bool, len(current)+len(next))
	out := make([]string, 0, len(current)+len(next))
	for _, v := range append(append([]string{}, current...), next...) {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

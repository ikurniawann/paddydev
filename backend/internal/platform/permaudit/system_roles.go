package permaudit

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/platform/database"
)

// BaseSystemRoles stay system roles even on a database whose iam.roles is
// not seeded (the app role still grants their reach there).
var BaseSystemRoles = []string{"super_admin", "admin"}

// SystemRoleCodes is iam.roles.is_system plus BaseSystemRoles (EPIC-061
// Q-RA-5: system roles are set by IAM super_admin only).
func SystemRoleCodes(ctx context.Context, q database.Querier) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT code FROM iam.roles WHERE is_system AND deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	codes, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	for _, c := range BaseSystemRoles {
		if !slices.Contains(codes, c) {
			codes = append(codes, c)
		}
	}
	return codes, nil
}

// IsSystemRole reports whether role (an app role code) is a system role.
func IsSystemRole(ctx context.Context, q database.Querier, role *string) (bool, error) {
	if role == nil {
		return false, nil
	}
	codes, err := SystemRoleCodes(ctx, q)
	return slices.Contains(codes, *role), err
}

// HoldsSystemRole reports whether the account holds a system role as its
// app role or as an IAM role. The id is compared as text so a malformed id
// is simply not found.
func HoldsSystemRole(ctx context.Context, q database.Querier, userID string) (bool, error) {
	codes, err := SystemRoleCodes(ctx, q)
	if err != nil {
		return false, err
	}
	var held bool
	err = q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM configuration.users WHERE id::text = $1 AND role = ANY($2))
		OR EXISTS (SELECT 1 FROM iam.user_roles ur JOIN iam.roles r ON r.id = ur.role_id
		  WHERE ur.user_id::text = $1 AND r.code = ANY($2))`, userID, codes).Scan(&held)
	return held, err
}

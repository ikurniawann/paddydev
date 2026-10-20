package accounts

import (
	"context"
	"net/http"
	"slices"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
)

// errSystemRoleSuperAdmin: granting or revoking a system role, and editing
// an account that holds one, is for IAM super_admin only (EPIC-061 Q-RA-5:
// "role sistem tetap hanya super_admin").
var errSystemRoleSuperAdmin = httpx.Forbidden("Role Super Admin/Admin hanya bisa diatur oleh Super Admin")

// baseSystemRoles stay system roles even on a database whose iam.roles is
// not seeded (the app role still grants their reach there).
var baseSystemRoles = []string{"super_admin", "admin"}

// systemRoleCodes is iam.roles.is_system plus baseSystemRoles.
func systemRoleCodes(ctx context.Context, q database.Querier) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT code FROM iam.roles WHERE is_system AND deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	codes, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	for _, c := range baseSystemRoles {
		if !slices.Contains(codes, c) {
			codes = append(codes, c)
		}
	}
	return codes, nil
}

// isSystemRole reports whether role (an app role code) is a system role.
func isSystemRole(ctx context.Context, q database.Querier, role *string) (bool, error) {
	if role == nil {
		return false, nil
	}
	codes, err := systemRoleCodes(ctx, q)
	return slices.Contains(codes, *role), err
}

// holdsSystemRole reports whether the account holds a system role as its
// app role or as an IAM role. The id is compared as text so a malformed id
// is simply not found.
func holdsSystemRole(ctx context.Context, q database.Querier, userID string) (bool, error) {
	codes, err := systemRoleCodes(ctx, q)
	if err != nil {
		return false, err
	}
	var held bool
	err = q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM configuration.users WHERE id::text = $1 AND role = ANY($2))
		OR EXISTS (SELECT 1 FROM iam.user_roles ur JOIN iam.roles r ON r.id = ur.role_id
		  WHERE ur.user_id::text = $1 AND r.code = ANY($2))`, userID, codes).Scan(&held)
	return held, err
}

// superAdminGate answers errSystemRoleSuperAdmin unless the caller holds
// the IAM role super_admin. The service calls it only when a system role
// is at stake, always before writing anything.
type superAdminGate func(ctx context.Context) error

func (h handlers) superAdminGate(r *http.Request) superAdminGate {
	return func(ctx context.Context) error {
		u, err := h.d.Auth.RequireUser(r)
		if err != nil {
			return err
		}
		ok, err := h.d.Auth.HasRoleCode(ctx, u, "super_admin")
		if err != nil {
			return err
		}
		if !ok {
			return errSystemRoleSuperAdmin
		}
		return nil
	}
}

// guardSystemRole runs gate when role is a system role.
func (s *service) guardSystemRole(ctx context.Context, gate superAdminGate, role *string) error {
	sys, err := isSystemRole(ctx, s.db, role)
	if err != nil || !sys {
		return err
	}
	return gate(ctx)
}

// guardSystemAccount runs gate when the account holds a system role.
func (s *service) guardSystemAccount(ctx context.Context, gate superAdminGate, userID string) error {
	held, err := holdsSystemRole(ctx, s.db, userID)
	if err != nil || !held {
		return err
	}
	return gate(ctx)
}

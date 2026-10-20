package accounts

import (
	"context"
	"net/http"

	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/permaudit"
)

// errSystemRoleSuperAdmin: granting or revoking a system role, and editing
// an account that holds one, is for IAM super_admin only (EPIC-061 Q-RA-5:
// "role sistem tetap hanya super_admin").
var errSystemRoleSuperAdmin = httpx.Forbidden("Role Super Admin/Admin hanya bisa diatur oleh Super Admin")

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
	sys, err := permaudit.IsSystemRole(ctx, s.db, role)
	if err != nil || !sys {
		return err
	}
	return gate(ctx)
}

// guardSystemAccount runs gate when the account holds a system role.
func (s *service) guardSystemAccount(ctx context.Context, gate superAdminGate, userID string) error {
	held, err := permaudit.HoldsSystemRole(ctx, s.db, userID)
	if err != nil || !held {
		return err
	}
	return gate(ctx)
}

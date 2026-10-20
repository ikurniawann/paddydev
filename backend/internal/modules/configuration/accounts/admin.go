package accounts

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"nuhabit/backend/internal/modules/configuration/accounts/domain"
	"nuhabit/backend/internal/modules/configuration/kit"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
)

// lib/admin/admin-users.ts: staff accounts without an employee record.

const profileColumns = `"id", "full_name", "email", "role", "brand_id", "status", "created_at"`

// authMessage is the { message } the auth shim hands back for a failure:
// PostgreSQL's own message for a query error.
func authMessage(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Message
	}
	return err.Error()
}

// listAdmins is listAdminUsers: profiles with brand and permission embeds,
// then the auth status of each.
func (s *service) listAdmins(ctx context.Context) ([]*kit.Row, error) {
	profiles, err := kit.Query(ctx, s.db, `SELECT `+profileColumns+`, "updated_at",
  (SELECT row_to_json(e) FROM (SELECT "id", "name" FROM "item"."brands" WHERE "id" = "users"."brand_id") e) AS "brands",
  `+permissionsEmbed+`
  FROM configuration.users ORDER BY "created_at" DESC`)
	if err != nil {
		return nil, err
	}
	statuses, err := s.ports.Identity.Statuses(ctx, s.db)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]AuthStatus, len(statuses))
	for _, a := range statuses {
		byID[a.ID] = a
	}
	now := s.now()
	for _, p := range profiles {
		a, ok := byID[p.Str("id")]
		email := p.Str("email")
		if ok {
			email = a.Email
		}
		p.Set("email", email)
		p.Set("auth_status", domain.AuthStatus(a.BannedUntil, now))
		p.Set("last_sign_in_at", a.LastSignInAt)
		p.Set("email_confirmed_at", a.EmailVerifiedAt)
	}
	return profiles, nil
}

// createAdmin is createAdminUser; the savepoint stands in for deleting the
// auth user when a later step fails.
func (s *service) createAdmin(ctx context.Context, actorID string, b *adminBody) (*kit.Row, error) {
	var profile *kit.Row
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		userID, err := s.ports.Identity.CreateUser(ctx, tx, NewAuthUser{Email: *b.email, Password: *b.password, FullName: *b.fullName, Role: *b.role})
		if err != nil {
			return httpx.BadRequest(authMessage(err))
		}
		profile, err = kit.QueryOne(ctx, tx, `INSERT INTO configuration.users (id, full_name, email, role, brand_id, status)
  VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+profileColumns,
			userID, *b.fullName, *b.email, *b.role, b.brand, *b.status)
		if err != nil {
			return err
		}
		if *b.status == "inactive" {
			ban := true
			quietly(ctx, tx, func(q database.Querier) error {
				_, err := s.ports.Identity.UpdateUser(ctx, q, userID, AuthUpdate{Ban: &ban})
				return err
			})
		}
		if err := insertApprovals(ctx, tx, userID, actorID, b.approvals); err != nil {
			return err
		}
		audit(ctx, tx, actorID, userID, "create_user", kit.Obj("email", *b.email, "role", *b.role, "status", *b.status))
		return nil
	})
	return profile, err
}

var errSelfDemotion = httpx.BadRequest("Super admin tidak bisa menonaktifkan atau menurunkan role dirinya sendiri")

// updateAdmin is updateAdminUser. A role change moves the IAM role with it
// and records it (syncIamRole, as Manajemen User does); the writes share
// one transaction.
func (s *service) updateAdmin(ctx context.Context, actorID, id string, b *adminBody, gate superAdminGate) (*kit.Row, error) {
	inactive := b.status != nil && *b.status == "inactive"
	if actorID == id && (inactive || (b.role != nil && *b.role != "super_admin")) {
		return nil, errSelfDemotion
	}
	// Refused before anything is written.
	if err := s.guardSystemRole(ctx, gate, b.role); err != nil {
		return nil, err
	}
	if err := s.guardSystemAccount(ctx, gate, id); err != nil {
		return nil, err
	}
	var upd AuthUpdate
	if b.email != nil && *b.email != "" {
		upd.Email = b.email
	}
	if b.fullName != nil && *b.fullName != "" {
		upd.FullName = b.fullName
	}
	upd.Role = b.role
	if b.status != nil {
		upd.Ban = &inactive
	}

	var profile *kit.Row
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		if upd != (AuthUpdate{}) {
			found, err := s.ports.Identity.UpdateUser(ctx, tx, id, upd)
			if err != nil {
				return httpx.BadRequest(authMessage(err))
			}
			if !found {
				return httpx.BadRequest("User not found")
			}
		}
		prevRole := profileColumn(ctx, tx, id, "role")

		sets := []set{{"updated_at", s.stamp()}}
		if upd.Email != nil {
			sets = append(sets, set{"email", *b.email})
		}
		if upd.FullName != nil {
			sets = append(sets, set{"full_name", *b.fullName})
		}
		if b.role != nil {
			sets = append(sets, set{"role", *b.role})
		}
		if b.has("brand_id") {
			sets = append(sets, set{"brand_id", b.brand})
		}
		if b.status != nil {
			sets = append(sets, set{"status", *b.status})
		}
		sql, args := updateSQL("configuration.users", sets, profileColumns+`, "updated_at"`)
		var err error
		profile, err = kit.QueryOne(ctx, tx, sql, append(args, id)...)
		if err != nil {
			return err
		}
		if profile == nil {
			return errors.New("PGRST116: No rows found")
		}
		if b.role != nil && (prevRole == nil || *prevRole != *b.role) {
			if err := syncIamRole(ctx, tx, id, *b.role, actorID); err != nil {
				return err
			}
		}

		if b.approvals != nil {
			if err := deactivateApprovals(ctx, tx, id, actorID, s.stamp()); err != nil {
				return err
			}
			if err := insertApprovals(ctx, tx, id, actorID, b.approvals); err != nil {
				return err
			}
		}
		details := kit.Obj("fields", b.keys())
		if b.role != nil {
			details.Set("role", *b.role)
		}
		if b.status != nil {
			details.Set("status", *b.status)
		}
		audit(ctx, tx, actorID, id, "update_user", details)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return profile, nil
}

// resetAdminPassword is resetAdminUserPassword.
func (s *service) resetAdminPassword(ctx context.Context, actorID, id string, gate superAdminGate) (string, error) {
	email, err := s.ports.Identity.Email(ctx, s.db, id)
	if err != nil || email == "" {
		return "", httpx.NotFound("Email user tidak ditemukan")
	}
	if err := s.guardSystemAccount(ctx, gate, id); err != nil {
		return "", err
	}
	res, err := s.resetPassword(ctx, id)
	if err != nil {
		return "", err
	}
	audit(ctx, s.db, actorID, id, "reset_password", kit.Obj("email", email))
	return res.TempPassword, nil
}

package access

import (
	"net/http"
	"slices"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/module"
	"nuhabit/backend/internal/platform/permaudit"
)

// "Hak Akses HRIS" (EPIC-061, RA-03 / Q-RA-5/7): AdminHRIS changes, for
// admin_hris and the hris_* roles only, the access to Penggajian,
// Rekrutmen and Kehadiran & Cuti and "delete" on HRIS leaves, and assigns
// those roles to HR accounts. Every change re-runs the HR role sync and is
// recorded in iam.permission_audit_logs.

const (
	errHrisRoleScope    = "Hak Akses HRIS hanya untuk role admin_hris dan hris_*"
	errHrisMenuScope    = "Menu di luar cakupan Hak Akses HRIS"
	errHrisAccessFirst  = "Aktifkan akses menu dulu sebelum memberi hak hapus"
	errHrisAccountScope = "Akun ini bukan akun HR (peran aplikasi hrd)"
	errHrisNoChange     = "Tidak ada perubahan"
)

type hrisAccessChange struct {
	MenuCode string `json:"menu_code"`
	Access   *bool  `json:"access"`
	Delete   *bool  `json:"delete"`
}

func (h handler) hrisAccessRoutes() []module.Route {
	return []module.Route{
		{Pattern: "GET /api/hris/access", Handler: httpx.Handle(h.getHrisAccess)},
		{Pattern: "PUT /api/hris/access/roles/{code}", Handler: httpx.Handle(h.putHrisRoleAccess)},
		{Pattern: "PUT /api/hris/access/accounts/{id}", Handler: httpx.Handle(h.putHrisAccount)},
		{Pattern: "GET /api/hris/access/audit", Handler: httpx.Handle(h.getPermissionAudit)},
	}
}

func (h handler) writeHrisAccess(w http.ResponseWriter, r *http.Request) error {
	v, err := hrisAccess(r.Context(), h.db)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"data": v})
}

func (h handler) getHrisAccess(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.auth.RequireMenuPrefix(r, iam.HrisAccess...); err != nil {
		return err
	}
	return h.writeHrisAccess(w, r)
}

func (h handler) putHrisRoleAccess(w http.ResponseWriter, r *http.Request) error {
	u, err := h.auth.RequireMenuAction(r, "update", iam.HrisAccess...)
	if err != nil {
		return err
	}
	var body struct {
		Changes []hrisAccessChange `json:"changes"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		return err
	}
	if len(body.Changes) == 0 || len(body.Changes) > 200 {
		return httpx.BadRequest(errHrisNoChange)
	}
	ctx := r.Context()
	roles, err := loadHrisRoles(ctx, h.db)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(roles, func(x hrisRoleRow) bool { return x.Code == r.PathValue("code") })
	if i < 0 || roles[i].IsSystem {
		return httpx.Forbidden(errHrisRoleScope)
	}
	role := roles[i]
	menus, err := loadScopeMenus(ctx, h.db)
	if err != nil {
		return err
	}
	byCode := map[string]scopeMenu{}
	for _, m := range menus {
		byCode[m.Code] = m
	}
	for _, c := range body.Changes {
		if c.Access == nil && c.Delete == nil {
			return httpx.BadRequest(errHrisNoChange)
		}
		m, ok := byCode[c.MenuCode]
		if !ok || (c.Access != nil && !m.AccessToggle) || (c.Delete != nil && !m.DeleteToggle) {
			return httpx.Forbidden(errHrisMenuScope)
		}
	}
	err = database.WithTx(ctx, h.db, func(tx pgx.Tx) error {
		// One HR role change at a time: the admin_hris row is the lock a
		// Role Management save of admin_hris also takes before its sync.
		if _, err := tx.Exec(ctx, `SELECT 1 FROM iam.roles WHERE code = 'admin_hris' FOR UPDATE`); err != nil {
			return err
		}
		watched, err := hrisRoleIDs(ctx, tx)
		if err != nil {
			return err
		}
		before, err := permaudit.Snapshot(ctx, tx, watched)
		if err != nil {
			return err
		}
		for _, c := range body.Changes {
			if role.Code == "admin_hris" {
				err = applyAdminHris(ctx, tx, role.ID, byCode[c.MenuCode], c, u.ID)
			} else {
				err = applyMask(ctx, tx, role.Code, c, u.ID)
			}
			if err != nil {
				return err
			}
		}
		var active bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM iam.role_menu_permissions p JOIN iam.roles r ON r.id = p.role_id
			WHERE r.code = 'admin_hris' AND p.is_active)`).Scan(&active); err != nil {
			return err
		}
		if !active {
			return httpx.BadRequest(errAdminHrisEmpty)
		}
		if _, err := tx.Exec(ctx, `SELECT iam.sync_hris_staff_roles()`); err != nil {
			return err
		}
		after, err := permaudit.Snapshot(ctx, tx, watched)
		if err != nil {
			return err
		}
		_, err = permaudit.Record(ctx, tx, u.ID, permaudit.SourceHrisAccess, before, after)
		return err
	})
	if err != nil {
		return err
	}
	return h.writeHrisAccess(w, r)
}

func (h handler) putHrisAccount(w http.ResponseWriter, r *http.Request) error {
	u, err := h.auth.RequireMenuAction(r, "update", iam.HrisAccess...)
	if err != nil {
		return err
	}
	var body struct {
		Role string `json:"role"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		return err
	}
	// Out of scope is 403, as on PUT roles/{code} (ruling M8).
	if !slices.Contains(iam.HrisStaffRoleCodes, body.Role) {
		return httpx.Forbidden(errHrisRoleScope)
	}
	ctx := r.Context()
	userID := r.PathValue("id")
	err = database.WithTx(ctx, h.db, func(tx pgx.Tx) error {
		var appRole string
		err := tx.QueryRow(ctx, `SELECT role FROM configuration.users WHERE id::text = $1`, userID).Scan(&appRole)
		if database.IsNoRows(err) {
			return httpx.NotFound("Akun tidak ditemukan")
		}
		if err != nil {
			return err
		}
		if appRole != "hrd" {
			return httpx.Forbidden(errHrisAccountScope)
		}
		codes, err := permaudit.UserRoleCodes(ctx, tx, userID)
		if err != nil {
			return err
		}
		for _, c := range codes {
			if c != "hrd" && !slices.Contains(iam.HrisStaffRoleCodes, c) {
				return httpx.Forbidden(errHrisAccountScope)
			}
		}
		return permaudit.SetUserRole(ctx, tx, userID, body.Role, u.ID)
	})
	if err != nil {
		return err
	}
	return h.writeHrisAccess(w, r)
}

func (h handler) getPermissionAudit(w http.ResponseWriter, r *http.Request) error {
	u, err := h.auth.RequireMenuPrefix(r, append(append([]string{}, iam.HrisAccess...), iam.SettingsRoles...)...)
	if err != nil {
		return err
	}
	all, err := h.auth.HasMenuPrefix(r.Context(), u, iam.SettingsRoles...)
	if err != nil {
		return err
	}
	rows, err := listPermissionAudit(r.Context(), h.db, all, 200)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"data": rows})
}

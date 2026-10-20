// Package access is Settings → IAM: roles, menus and the role permission
// matrix (frontend/src/app/api/settings/iam/**). The guards read the same
// iam tables through platform/auth, so a grant changed here applies on the
// next request.
package access

import (
	"net/http"
	"slices"

	"nuhabit/backend/internal/modules/configuration/kit"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/module"
)

type handler struct {
	db   database.DB
	auth *auth.Service
}

// Routes mounts the IAM settings routes and HRIS → Hak Akses HRIS.
func Routes(d module.Deps, db database.DB) []module.Route {
	h := handler{db: db, auth: d.Auth}
	return append([]module.Route{
		{Pattern: "GET /api/settings/iam/roles", Handler: httpx.Handle(h.listRoles)},
		{Pattern: "POST /api/settings/iam/roles", Handler: httpx.Handle(h.createRole)},
		{Pattern: "GET /api/settings/iam/roles/{id}", Handler: httpx.Handle(h.getRole)},
		{Pattern: "PUT /api/settings/iam/roles/{id}", Handler: httpx.Handle(h.updateRole)},
		{Pattern: "DELETE /api/settings/iam/roles/{id}", Handler: httpx.Handle(h.deleteRole)},
		{Pattern: "GET /api/settings/iam/roles/{id}/permissions", Handler: httpx.Handle(h.getPermissions)},
		{Pattern: "PUT /api/settings/iam/roles/{id}/permissions", Handler: httpx.Handle(h.putPermissions)},
		{Pattern: "GET /api/settings/iam/menus", Handler: httpx.Handle(h.listMenus)},
		{Pattern: "POST /api/settings/iam/menus", Handler: httpx.Handle(h.createMenu)},
		{Pattern: "GET /api/settings/iam/menus/{id}", Handler: httpx.Handle(h.getMenu)},
		{Pattern: "PUT /api/settings/iam/menus/{id}", Handler: httpx.Handle(h.updateMenu)},
		{Pattern: "DELETE /api/settings/iam/menus/{id}", Handler: httpx.Handle(h.deleteMenu)},
	}, h.hrisAccessRoutes()...)
}

var (
	errRoleNotFound = httpx.NotFound("Role not found")
	errMenuNotFound = httpx.NotFound("Menu not found")
)

type listBody[T any] struct {
	Data  []T `json:"data"`
	Total int `json:"total"`
}

type idBody struct {
	ID string `json:"id"`
}

type successBody struct {
	Success bool `json:"success"`
}

/* ── roles ───────────────────────────────────────────────────────────── */

func (h handler) listRoles(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.auth.RequireMenuPrefix(r, iam.SettingsRoles...); err != nil {
		return err
	}
	rows, err := listRoles(r.Context(), h.db)
	if err != nil {
		return err
	}
	filtered := filterRoles(rows, newListFilter(r.URL.Query().Get))
	return httpx.JSON(w, http.StatusOK, listBody[roleItem]{filtered, len(filtered)})
}

func (h handler) createRole(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.auth.RequireMenuPrefix(r, iam.SettingsRoles...); err != nil {
		return err
	}
	f, err := kit.Form(r)
	if err != nil {
		return err
	}
	in := parseRoleCreate(f)
	if err := f.Err("Validation failed"); err != nil {
		return err
	}
	var description *string
	if d := trimmed(in.Description); d != nil && *d != "" {
		description = d
	}
	isActive := in.IsActive == nil || *in.IsActive
	id, err := createRole(r.Context(), h.db, *lowerTrim(in.Code), *in.Name, description, isActive)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, idBody{id})
}

func (h handler) getRole(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.auth.RequireMenuPrefix(r, iam.SettingsRoles...); err != nil {
		return err
	}
	id := r.PathValue("id")
	row, err := getRole(r.Context(), h.db, id)
	if err != nil {
		return err
	}
	if row == nil {
		return errRoleNotFound
	}
	perms, err := permissionMatrix(r.Context(), h.db, id)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, mapRoleDetail(*row, perms))
}

func (h handler) updateRole(w http.ResponseWriter, r *http.Request) error {
	user, err := h.auth.RequireMenuPrefix(r, iam.SettingsRoles...)
	if err != nil {
		return err
	}
	id := r.PathValue("id")
	f, err := kit.Form(r)
	if err != nil {
		return err
	}
	in := parseRoleUpdate(f)
	if err := f.Err("Validation failed"); err != nil {
		return err
	}
	existing, err := getRole(r.Context(), h.db, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return errRoleNotFound
	}
	if existing.IsSystem {
		if err := h.requireSuperAdmin(r, user); err != nil {
			return err
		}
	}
	newCode := lowerTrim(in.Code)
	if isHrisRole(existing.Code) || (newCode != nil && isHrisRole(*newCode) && *newCode != existing.Code) {
		if err := h.requireSuperAdminFor(r, user, errHrisRoleSuperAdmin); err != nil {
			return err
		}
	}
	// A system role keeps its code: the guards and the configuration.users
	// role fallback resolve roles by code.
	if existing.IsSystem && in.Code != nil && *in.Code != "" && *in.Code != existing.Code {
		return httpx.BadRequest("System role code cannot be changed")
	}
	var code *string
	if !existing.IsSystem {
		code = newCode
	}
	updated, err := updateRole(r.Context(), h.db, id, code, trimmed(in.Name), in)
	if err != nil {
		return err
	}
	if !updated {
		return errRoleNotFound
	}
	return httpx.JSON(w, http.StatusOK, idBody{id})
}

func (h handler) deleteRole(w http.ResponseWriter, r *http.Request) error {
	user, err := h.auth.RequireMenuPrefix(r, iam.SettingsRoles...)
	if err != nil {
		return err
	}
	if existing, err := getRole(r.Context(), h.db, r.PathValue("id")); err != nil {
		return err
	} else if existing != nil && isHrisRole(existing.Code) {
		if err := h.requireSuperAdminFor(r, user, errHrisRoleSuperAdmin); err != nil {
			return err
		}
	}
	found, system, err := deleteRole(r.Context(), h.db, r.PathValue("id"))
	switch {
	case err != nil:
		return err
	case system:
		return httpx.BadRequest(errSystemRole)
	case !found:
		return errRoleNotFound
	}
	return httpx.JSON(w, http.StatusOK, successBody{true})
}

type permissionsBody struct {
	RoleID      string           `json:"roleId"`
	Permissions []permissionView `json:"permissions"`
}

func (h handler) permissionsResponse(w http.ResponseWriter, r *http.Request, id string, row roleRow) error {
	perms, err := permissionMatrix(r.Context(), h.db, id)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, permissionsBody{id, mapRoleDetail(row, perms).Permissions})
}

// errSystemRoleSuperAdmin: system roles change only through super_admin
// (EPIC-061, Q-RA-5).
var errSystemRoleSuperAdmin = httpx.Forbidden("Role sistem hanya bisa diubah oleh Super Admin")

// HR roles (admin_hris and the hris_* roles derived from it) and "delete"
// on HRIS menus change in Role Management only through super_admin;
// AdminHRIS uses Hak Akses HRIS (EPIC-061, Q-RA-2/Q-RA-5).
var (
	errHrisRoleSuperAdmin   = httpx.Forbidden("Role HR hanya bisa diubah oleh Super Admin; AdminHRIS memakai halaman Hak Akses HRIS")
	errHrisDeleteSuperAdmin = httpx.Forbidden("Hak hapus HRIS hanya bisa diberikan oleh Super Admin")
)

func isHrisRole(code string) bool { return slices.Contains(iam.HrisStaffRoleCodes, code) }

func (h handler) requireSuperAdmin(r *http.Request, u *auth.User) error {
	return h.requireSuperAdminFor(r, u, errSystemRoleSuperAdmin)
}

// requireSuperAdminFor returns denied unless u holds the IAM role super_admin.
func (h handler) requireSuperAdminFor(r *http.Request, u *auth.User, denied error) error {
	ok, err := h.auth.HasRoleCode(r.Context(), u, "super_admin")
	if err != nil {
		return err
	}
	if !ok {
		return denied
	}
	return nil
}

func (h handler) roleOr404(r *http.Request, id string) (roleRow, error) {
	row, err := getRole(r.Context(), h.db, id)
	if err != nil {
		return roleRow{}, err
	}
	if row == nil {
		return roleRow{}, errRoleNotFound
	}
	return *row, nil
}

func (h handler) getPermissions(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.auth.RequireMenuPrefix(r, iam.SettingsRoles...); err != nil {
		return err
	}
	id := r.PathValue("id")
	row, err := h.roleOr404(r, id)
	if err != nil {
		return err
	}
	return h.permissionsResponse(w, r, id, row)
}

func (h handler) putPermissions(w http.ResponseWriter, r *http.Request) error {
	user, err := h.auth.RequireMenuPrefix(r, iam.SettingsRoles...)
	if err != nil {
		return err
	}
	id := r.PathValue("id")
	f, err := kit.Form(r)
	if err != nil {
		return err
	}
	grants := parsePermissions(f)
	if err := f.Err("Validation failed"); err != nil {
		return err
	}
	row, err := h.roleOr404(r, id)
	if err != nil {
		return err
	}
	if row.IsSystem {
		if err := h.requireSuperAdmin(r, user); err != nil {
			return err
		}
	}
	if isHrisRole(row.Code) {
		if err := h.requireSuperAdminFor(r, user, errHrisRoleSuperAdmin); err != nil {
			return err
		}
	} else if added, err := addsHrisDelete(r.Context(), h.db, id, grants); err != nil {
		return err
	} else if added {
		if err := h.requireSuperAdminFor(r, user, errHrisDeleteSuperAdmin); err != nil {
			return err
		}
	}
	if err := replacePermissions(r.Context(), h.db, id, grants, user.ID); err != nil {
		return err
	}
	return h.permissionsResponse(w, r, id, row)
}

/* ── menus ───────────────────────────────────────────────────────────── */

func (h handler) listMenus(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.auth.RequireMenuPrefix(r, iam.SettingsMenus...); err != nil {
		return err
	}
	rows, err := listMenus(r.Context(), h.db)
	if err != nil {
		return err
	}
	filtered := filterMenus(rows, newListFilter(r.URL.Query().Get))
	return httpx.JSON(w, http.StatusOK, listBody[menuItem]{filtered, len(filtered)})
}

func (h handler) createMenu(w http.ResponseWriter, r *http.Request) error {
	user, err := h.auth.RequireMenuPrefix(r, iam.SettingsMenus...)
	if err != nil {
		return err
	}
	f, err := kit.Form(r)
	if err != nil {
		return err
	}
	in := parseMenu(f, false)
	if err := f.Err("Validation failed"); err != nil {
		return err
	}
	id, err := createMenu(r.Context(), h.db, in, user.ID)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, idBody{id})
}

func (h handler) getMenu(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.auth.RequireMenuPrefix(r, iam.SettingsMenus...); err != nil {
		return err
	}
	row, err := getMenu(r.Context(), h.db, r.PathValue("id"))
	if err != nil {
		return err
	}
	if row == nil {
		return errMenuNotFound
	}
	return httpx.JSON(w, http.StatusOK, mapMenuDetail(*row))
}

func (h handler) updateMenu(w http.ResponseWriter, r *http.Request) error {
	user, err := h.auth.RequireMenuPrefix(r, iam.SettingsMenus...)
	if err != nil {
		return err
	}
	id := r.PathValue("id")
	f, err := kit.Form(r)
	if err != nil {
		return err
	}
	in := parseMenu(f, true)
	if err := f.Err("Validation failed"); err != nil {
		return err
	}
	updated, err := updateMenu(r.Context(), h.db, id, in, user.ID)
	if err != nil {
		return err
	}
	if !updated {
		return errMenuNotFound
	}
	return httpx.JSON(w, http.StatusOK, idBody{id})
}

func (h handler) deleteMenu(w http.ResponseWriter, r *http.Request) error {
	user, err := h.auth.RequireMenuPrefix(r, iam.SettingsMenus...)
	if err != nil {
		return err
	}
	deleted, err := deleteMenu(r.Context(), h.db, r.PathValue("id"), user.ID)
	if err != nil {
		return err
	}
	if !deleted {
		return errMenuNotFound
	}
	return httpx.JSON(w, http.StatusOK, successBody{true})
}

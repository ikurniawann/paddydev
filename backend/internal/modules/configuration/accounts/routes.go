// Package accounts ports the staff account routes: /api/users (accounts
// linked to hris employees) and /api/admin/users (accounts on their own).
// Both sit behind the settings.users menu. The HRIS employee directory menu
// (hris.kepegawaian.users) may also read /api/users and write employee data
// there — never a login account (account fields, a linked account's email).
package accounts

import (
	"crypto/rand"
	"net/http"
	"slices"
	"strings"

	"nuhabit/backend/internal/modules/configuration/kit"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/module"
)

type handlers struct {
	d   module.Deps
	svc *service
}

// Routes mounts this area.
func Routes(d module.Deps, db database.DB, p Ports) []module.Route {
	h := handlers{d: d, svc: &service{db: db, ports: p, now: d.Now, random: rand.Reader}}
	return []module.Route{
		{Pattern: "GET /api/users", Handler: httpx.Handle(h.listUsers)},
		{Pattern: "POST /api/users", Handler: httpx.Handle(h.createUser)},
		{Pattern: "GET /api/users/email-owner", Handler: httpx.Handle(h.emailOwner)},
		{Pattern: "GET /api/users/{id}", Handler: httpx.Handle(h.getUser)},
		{Pattern: "PUT /api/users/{id}", Handler: httpx.Handle(h.updateUser)},
		{Pattern: "POST /api/users/{id}/reset-password", Handler: httpx.Handle(h.resetUserPassword)},
		{Pattern: "GET /api/admin/users", Handler: httpx.Handle(h.listAdmins)},
		{Pattern: "POST /api/admin/users", Handler: httpx.Handle(h.createAdmin)},
		{Pattern: "PATCH /api/admin/users/{id}", Handler: httpx.Handle(h.updateAdmin)},
		{Pattern: "POST /api/admin/users/{id}/reset-password", Handler: httpx.Handle(h.resetAdminPassword)},
	}
}

type dataMessage struct {
	Data    any    `json:"data"`
	Message string `json:"message"`
}

func (h handlers) guard(r *http.Request) (string, error) {
	u, err := h.d.Auth.RequireMenuPrefix(r, iam.SettingsUsers...)
	if err != nil {
		return "", err
	}
	return u.ID, nil
}

// Writers without Manajemen User (the HRIS employee directory) change employee data only.
var (
	errAccountNeedsSettings    = httpx.Forbidden("Akun login hanya bisa diatur lewat Settings → Manajemen User")
	errLoginEmailNeedsSettings = httpx.Forbidden(
		"Email karyawan yang punya akun login hanya bisa diubah lewat Settings → Manajemen User")
)

// readPrefixes may read /api/users: Manajemen User, or the HRIS employee
// directory (HRIS → Kepegawaian → Karyawan), which lists the same rows.
var readPrefixes = append(append([]string{}, iam.SettingsUsers...), iam.HrisKaryawan...)

func (h handlers) readGuard(r *http.Request) error {
	_, err := h.d.Auth.RequireMenuPrefix(r, readPrefixes...)
	return err
}

// writeGuard is who may write /api/users: Manajemen User writes everything;
// the HRIS employee directory writes employee data only (accounts false) and
// only with the action ("create" or "update") granted on that menu.
func (h handlers) writeGuard(r *http.Request, action string) (actor string, accounts bool, err error) {
	u, err := h.d.Auth.RequireUser(r)
	if err != nil {
		return "", false, err
	}
	granted, err := h.d.Auth.GrantedMenuCodes(r.Context(), u.ID, u.Role)
	if err != nil {
		return "", false, err
	}
	if iam.HasAnyMenuPrefix(granted, iam.SettingsUsers) {
		return u.ID, true, nil
	}
	if iam.HasAnyMenuPrefix(granted, iam.HrisKaryawan) {
		roleIDs, err := h.d.Auth.ResolveRoleIDs(r.Context(), u.ID, u.Role)
		if err != nil {
			return "", false, err
		}
		actions, err := h.d.Auth.GrantedMenuActions(r.Context(), roleIDs)
		if err != nil {
			return "", false, err
		}
		for _, code := range iam.HrisKaryawan {
			if slices.Contains(actions[code], action) {
				return u.ID, false, nil
			}
		}
	}
	return "", false, httpx.Forbidden("Insufficient permissions")
}

/* ── /api/users ──────────────────────────────────────────────────────── */

func (h handlers) listUsers(w http.ResponseWriter, r *http.Request) error {
	if err := h.readGuard(r); err != nil {
		return err
	}
	q, err := parseListQuery(r.URL.Query())
	if err != nil {
		return err
	}
	page, err := h.svc.listUsers(r.Context(), q)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, page)
}

func (h handlers) createUser(w http.ResponseWriter, r *http.Request) error {
	actor, accounts, err := h.writeGuard(r, "create")
	if err != nil {
		return err
	}
	f, err := kit.Form(r)
	if err != nil {
		return err
	}
	b := parseEmployeeBody(f, true)
	// Permission before validation: account fields are refused, not validated.
	if !accounts && b.touchesAccount(true) {
		return errAccountNeedsSettings
	}
	if err := f.Err("Validation failed"); err != nil {
		return err
	}
	data, err := h.svc.createUser(r.Context(), actor, b)
	if err != nil {
		return rethrowUserServiceError(err)
	}
	return httpx.JSON(w, http.StatusCreated, dataMessage{Data: data, Message: "Employee created successfully"})
}

// emailOwner tells the create form who holds an email (EPIC-056 §1.2).
func (h handlers) emailOwner(w http.ResponseWriter, r *http.Request) error {
	if _, _, err := h.writeGuard(r, "create"); err != nil {
		return err
	}
	email := strings.TrimSpace(r.URL.Query().Get("email"))
	if email == "" {
		return httpx.JSON(w, http.StatusOK, map[string]any{"data": nil})
	}
	o, err := h.svc.ports.Employees.EmailOwner(r.Context(), h.svc.db, email)
	if err != nil {
		return err
	}
	if o == nil {
		return httpx.JSON(w, http.StatusOK, map[string]any{"data": nil})
	}
	var end *string
	if o.EndDate != nil {
		d := o.EndDate.Format("2006-01-02")
		end = &d
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"id": o.ID, "nip": o.NIP, "full_name": o.FullName, "is_active": o.IsActive, "end_date": end}})
}

func (h handlers) getUser(w http.ResponseWriter, r *http.Request) error {
	if err := h.readGuard(r); err != nil {
		return err
	}
	data, err := h.svc.getUser(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	if data == nil {
		return httpx.NotFound("Employee not found")
	}
	return httpx.JSON(w, http.StatusOK, struct {
		Data any `json:"data"`
	}{data})
}

func (h handlers) updateUser(w http.ResponseWriter, r *http.Request) error {
	actor, accounts, err := h.writeGuard(r, "update")
	if err != nil {
		return err
	}
	f, err := kit.Form(r)
	if err != nil {
		return err
	}
	b := parseEmployeeBody(f, false)
	// Permission before validation: account fields are refused, not validated.
	if !accounts && b.touchesAccount(false) {
		return errAccountNeedsSettings
	}
	if err := f.Err("Validation failed"); err != nil {
		return err
	}
	data, err := h.svc.updateUser(r.Context(), actor, r.PathValue("id"), b, accounts)
	if err != nil {
		return rethrowUserServiceError(err)
	}
	return httpx.JSON(w, http.StatusOK, dataMessage{Data: data, Message: "Employee updated successfully"})
}

func (h handlers) resetUserPassword(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.guard(r); err != nil {
		return err
	}
	res, err := h.svc.resetEmployeePassword(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, res)
}

/* ── /api/admin/users ────────────────────────────────────────────────── */

func (h handlers) listAdmins(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.guard(r); err != nil {
		return err
	}
	data, err := h.svc.listAdmins(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, struct {
		Data any `json:"data"`
	}{data})
}

func (h handlers) createAdmin(w http.ResponseWriter, r *http.Request) error {
	actor, err := h.guard(r)
	if err != nil {
		return err
	}
	f, err := kit.Form(r)
	if err != nil {
		return err
	}
	b := parseAdminCreate(f)
	if err := f.Err("Validation failed"); err != nil {
		return err
	}
	profile, err := h.svc.createAdmin(r.Context(), actor, b)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, dataMessage{Data: profile, Message: "User berhasil dibuat"})
}

func (h handlers) updateAdmin(w http.ResponseWriter, r *http.Request) error {
	actor, err := h.guard(r)
	if err != nil {
		return err
	}
	f, err := kit.Form(r)
	if err != nil {
		return err
	}
	b := parseAdminUpdate(f)
	if err := f.Err("Validation failed"); err != nil {
		return err
	}
	profile, err := h.svc.updateAdmin(r.Context(), actor, r.PathValue("id"), b)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, dataMessage{Data: profile, Message: "User berhasil diperbarui"})
}

func (h handlers) resetAdminPassword(w http.ResponseWriter, r *http.Request) error {
	actor, err := h.guard(r)
	if err != nil {
		return err
	}
	temp, err := h.svc.resetAdminPassword(r.Context(), actor, r.PathValue("id"))
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, resetResult{Message: "Password sementara berhasil dibuat", TempPassword: temp})
}

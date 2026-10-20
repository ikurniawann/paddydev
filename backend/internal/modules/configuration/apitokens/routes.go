// Package apitokens manages Open API tokens (EPIC-042,
// frontend/src/app/api/admin/api-tokens/**): list, create (the raw token is
// returned once, the database keeps its SHA-256 hash) and soft revoke. Only
// a person signed in to the dashboard may manage tokens; a token cannot mint
// or revoke tokens.
package apitokens

import (
	"errors"
	"math"
	"net/http"
	"strings"
	"time"

	"nuhabit/backend/internal/modules/configuration/kit"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/module"
	"nuhabit/backend/internal/platform/permaudit"
	"nuhabit/backend/internal/platform/validate"
)

type handler struct {
	db   database.DB
	auth *auth.Service
	now  func() time.Time
}

// Routes mounts the token admin routes.
func Routes(d module.Deps, db database.DB) []module.Route {
	h := handler{db: db, auth: d.Auth, now: d.Now}
	if h.now == nil {
		h.now = time.Now
	}
	return []module.Route{
		{Pattern: "GET /api/admin/api-tokens", Handler: httpx.Handle(h.list)},
		{Pattern: "POST /api/admin/api-tokens", Handler: httpx.Handle(h.create)},
		{Pattern: "DELETE /api/admin/api-tokens/{id}", Handler: httpx.Handle(h.revoke)},
	}
}

// errSystemAccountToken: a token runs as its account, so minting one for an
// account holding a system role (super_admin/admin, app or IAM role) is for
// IAM super_admin only (EPIC-061 Q-RA-5).
var errSystemAccountToken = httpx.Forbidden("Token API untuk akun Super Admin/Admin hanya bisa dibuat oleh Super Admin")

// requireHumanTokenAdmin rejects Bearer token sessions before the menu
// check, then requires settings.integrations. It returns the session user
// and the resolved caller.
func (h handler) requireHumanTokenAdmin(r *http.Request) (*auth.SessionUser, *auth.User, error) {
	su, err := h.auth.Session(r)
	if err != nil {
		return nil, nil, err
	}
	if su != nil && su.ViaAPIToken {
		return nil, nil, httpx.Forbidden("Kelola token hanya lewat login dashboard, bukan token")
	}
	u, err := h.auth.RequireMenuPrefix(r, iam.SettingsIntegrations...)
	if err != nil {
		return nil, nil, err
	}
	return su, u, nil
}

// guardSystemAccount refuses a token for another account holding a system
// role unless the caller holds IAM super_admin. A token for the caller's
// own account grants nothing the caller lacks.
func (h handler) guardSystemAccount(r *http.Request, caller *auth.User, userID string) error {
	if caller != nil && caller.ID == userID {
		return nil
	}
	held, err := permaudit.HoldsSystemRole(r.Context(), h.db, userID)
	if err != nil || !held {
		return err
	}
	if caller != nil {
		ok, err := h.auth.HasRoleCode(r.Context(), caller, "super_admin")
		if err != nil || ok {
			return err
		}
	}
	return errSystemAccountToken
}

type envelope struct {
	Success          bool `json:"success"`
	Data             any  `json:"data"`
	MigrationPending bool `json:"migration_pending,omitempty"`
}

func (h handler) list(w http.ResponseWriter, r *http.Request) error {
	if _, _, err := h.requireHumanTokenAdmin(r); err != nil {
		return err
	}
	rows, err := kit.Query(r.Context(), h.db, `SELECT t.id, t.name, t.token_prefix, t.scopes, t.user_id,
            u.full_name AS user_name, t.created_at, t.expires_at,
            t.revoked_at, t.last_used_at
     FROM configuration.api_tokens t
     LEFT JOIN configuration.users u ON u.id = t.user_id
     ORDER BY t.created_at DESC`)
	if database.IsUndefinedTable(err) {
		return httpx.JSON(w, http.StatusOK, envelope{Success: true, Data: []any{}, MigrationPending: true})
	}
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, envelope{Success: true, Data: rows})
}

// jsDateLimit is the largest JS Date in ms; past it toISOString throws.
const jsDateLimit = 8.64e15

func (h handler) create(w http.ResponseWriter, r *http.Request) error {
	admin, caller, err := h.requireHumanTokenAdmin(r)
	if err != nil {
		return err
	}
	raw, err := kit.ReadJSON(r)
	if err != nil {
		return err
	}
	body, _ := raw.(map[string]any) // `?? {}`; a non-object reads as no fields
	adminID := ""
	if admin != nil {
		adminID = admin.ID
	}

	name := validate.JSTrim(jsString(orFallback(body["name"], "")))
	if name == "" {
		return httpx.BadRequest("Nama token wajib diisi")
	}
	scopes := []string{}
	if list, ok := body["scopes"].([]any); ok {
		for _, s := range list {
			scopes = append(scopes, jsString(s))
		}
	}
	valid := len(scopes) > 0
	for _, s := range scopes {
		valid = valid && isValidScope(s)
	}
	if !valid {
		return httpx.BadRequest("Scope tidak valid. Pakai '*' atau '<modul>:read|write' (modul: " + strings.Join(auth.APIScopeModules, ", ") + ")")
	}

	// The token runs as this account; the default is the admin creating it.
	// Canonicalised as a uuid before the guard and the insert (EPIC-061).
	userID, ok := validate.CanonicalUUID(jsString(orFallback(body["user_id"], orFallback(adminID, ""))))
	if !ok {
		return httpx.BadRequest("user_id tidak valid")
	}
	var exists string
	err = h.db.QueryRow(r.Context(), `SELECT id::text FROM configuration.users WHERE id = $1`, userID).Scan(&exists)
	if database.IsNoRows(err) {
		return httpx.BadRequest("user_id tidak dikenal")
	}
	if err != nil {
		return tableMissing(err)
	}
	if err := h.guardSystemAccount(r, caller, userID); err != nil {
		return err
	}

	var expiresAt *string
	if days := jsNumber(body["expires_in_days"]); days > 0 {
		ms := float64(h.now().UnixMilli()) + days*86_400_000
		if math.IsInf(ms, 0) || ms > jsDateLimit {
			return errors.New("apitokens: expires_in_days is past the JS Date range")
		}
		iso := time.UnixMilli(int64(ms)).UTC().Format(httpx.JSTimeLayout)
		expiresAt = &iso
	}

	m, err := mint()
	if err != nil {
		return err
	}
	var createdBy *string
	if adminID != "" {
		createdBy = &adminID
	}
	row, err := kit.QueryOne(r.Context(), h.db, `INSERT INTO configuration.api_tokens
       (name, token_hash, token_prefix, user_id, scopes, created_by, expires_at)
     VALUES ($1, $2, $3, $4, $5, $6, $7)
     RETURNING id, name, token_prefix, scopes, user_id, created_at, expires_at`,
		name, m.hash, m.prefix, userID, scopes, createdBy, expiresAt)
	if err != nil {
		return tableMissing(err)
	}
	// The raw token leaves the server only in this response.
	row.Set("token", m.token)
	return httpx.JSON(w, http.StatusCreated, envelope{Success: true, Data: row})
}

// tableMissing maps 42P01 to the TS 503 that asks for the migration.
func tableMissing(err error) error {
	if database.IsUndefinedTable(err) {
		return httpx.Status(http.StatusServiceUnavailable, "Tabel api_tokens belum ada. Jalankan migrasi database dulu: pnpm db:migrate:apply")
	}
	return err
}

func (h handler) revoke(w http.ResponseWriter, r *http.Request) error {
	if _, _, err := h.requireHumanTokenAdmin(r); err != nil {
		return err
	}
	row, err := kit.QueryOne(r.Context(), h.db, `UPDATE configuration.api_tokens
     SET revoked_at = now()
     WHERE id = $1 AND revoked_at IS NULL
     RETURNING id, name`, r.PathValue("id"))
	if err != nil {
		return err
	}
	if row == nil {
		return httpx.NotFound("Token tidak ditemukan atau sudah dicabut")
	}
	return httpx.JSON(w, http.StatusOK, envelope{Success: true, Data: row})
}

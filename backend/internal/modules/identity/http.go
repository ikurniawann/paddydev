package identity

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/module"
	"nuhabit/backend/internal/platform/stall"
	"nuhabit/backend/internal/platform/validate"
)

type handlers struct {
	svc *Service
	log *slog.Logger
}

// errorBody is the bare {"error": message} of the auth routes' own failures.
type errorBody struct {
	Error string `json:"error"`
}

// handle renders a route's failures the way its TS catch block does: a
// clientError as {"error"}, an ApiError (*httpx.Error) with its envelope, and
// anything else as a logged 500 with the route's own body. The TS routes
// echo err.message there; Go answers the route's fallback text instead so
// database errors do not leak.
func (h handlers) handle(route string, fail any, fn httpx.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := fn(w, r)
		if err == nil {
			return
		}
		var ce *clientError
		var ae *httpx.Error
		switch {
		case errors.As(err, &ce):
			_ = httpx.JSON(w, ce.status, errorBody{ce.message})
		case errors.As(err, &ae):
			httpx.WriteError(w, r, err)
		default:
			h.log.ErrorContext(r.Context(), "["+route+"] failed", "error", err)
			_ = httpx.JSON(w, http.StatusInternalServerError, fail)
		}
	})
}

// Routes lists the module's routes.
func (h handlers) Routes() []module.Route {
	return []module.Route{
		{Pattern: "GET /api/auth/me", Handler: httpx.Handle(h.me)},
		{Pattern: "POST /api/auth/login", Handler: h.handle("auth/login", errorBody{"Login failed"}, h.login)},
		{Pattern: "POST /api/auth/logout", Handler: httpx.Handle(h.logoutPost)},
		{Pattern: "GET /api/auth/logout", Handler: httpx.Handle(h.logoutGet)},
		{Pattern: "POST /api/auth/change-password", Handler: h.handle("api/auth/change-password", errorBody{"Failed to change password"}, h.changePassword)},
		{Pattern: "POST /api/auth/impersonate", Handler: h.handle("api/auth/impersonate", errorBody{"Failed to impersonate user"}, h.impersonate)},
		{Pattern: "POST /api/auth/active-stall", Handler: h.handle("api/auth/active-stall", errorBody{"Failed to set active outlet"}, h.activeStall)},
		{Pattern: "GET /api/auth/stall-options", Handler: h.handle("api/auth/stall-options", errorBody{"Failed to load outlet options"}, h.stallOptions)},
		{Pattern: "GET /api/auth/scope", Handler: h.handle("api/auth/scope", scopeFailure{Message: "Failed to fetch user scope"}, h.scope)},
	}
}

func (h handlers) me(w http.ResponseWriter, r *http.Request) error {
	me, err := h.svc.Me(r)
	if err != nil {
		return err
	}
	return httpx.Data(w, http.StatusOK, me)
}

// ── login / logout ───────────────────────────────────────────────────────

type loginResponse struct {
	Data loginData `json:"data"`
}

type loginData struct {
	User    loginUser    `json:"user"`
	Session loginSession `json:"session"`
}

type loginUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type loginSession struct {
	AccessToken string `json:"access_token"`
	ExpiresAt   string `json:"expires_at"`
}

func (h handlers) login(w http.ResponseWriter, r *http.Request) error {
	// `const { email, password } = await request.json()`: a body that is not
	// JSON, or JSON null, throws and answers 500; any other non-object has
	// no such fields and answers 400.
	var body any
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &body); err != nil || body == nil {
		return errors.New("login body is not JSON")
	}
	fields, _ := body.(map[string]any)
	in := LoginInput{Email: fields["email"], Password: fields["password"], IP: clientIP(r.Header)}
	if ua, ok := r.Header["User-Agent"]; ok && len(ua) > 0 {
		in.UserAgent = &ua[0]
	}
	res, err := h.svc.Login(r.Context(), in)
	if err != nil {
		return err
	}
	setSessionCookie(w, r, res.Session)
	return httpx.JSON(w, http.StatusOK, loginResponse{Data: loginData{
		User:    loginUser{ID: res.UserID, Email: res.Email},
		Session: loginSession{AccessToken: res.Session.Token, ExpiresAt: res.Session.ExpiresAt.UTC().Format("2006-01-02T15:04:05.000Z")},
	}})
}

// endSession deletes the sessions of both cookie names, then expires the
// session cookies and the legacy active stall cookie.
func (h handlers) endSession(w http.ResponseWriter, r *http.Request) error {
	var tokens []string
	for _, name := range []string{auth.SessionCookie, auth.LegacySessionCookie} {
		if c, err := r.Cookie(name); err == nil {
			tokens = append(tokens, c.Value)
		}
	}
	if err := h.svc.Logout(r.Context(), tokens); err != nil {
		return err
	}
	clearSessionCookies(w, r)
	addCookie(w, cookie{name: stall.LegacyCookieName, maxAge: &zeroMaxAge})
	return nil
}

func (h handlers) logoutPost(w http.ResponseWriter, r *http.Request) error {
	if err := h.endSession(w, r); err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, struct {
		Success bool `json:"success"`
	}{true})
}

// logoutGet is what requireUser redirects to when a session cookie is no
// longer valid; the Location stays relative because behind the tunnel the
// request URL carries the internal host.
func (h handlers) logoutGet(w http.ResponseWriter, r *http.Request) error {
	if err := h.endSession(w, r); err != nil {
		return err
	}
	w.Header().Set("Location", "/login")
	w.WriteHeader(http.StatusTemporaryRedirect)
	return nil
}

// ── change password / impersonate ────────────────────────────────────────

func (h handlers) changePassword(w http.ResponseWriter, r *http.Request) error {
	user, err := h.svc.sessions.RequireUser(r)
	if err != nil {
		return err
	}
	// `request.json().catch(() => ({}))`, then typeof checks.
	var body map[string]any
	if raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)); err == nil {
		_ = json.Unmarshal(raw, &body)
	}
	current, _ := body["current_password"].(string)
	next, _ := body["new_password"].(string)
	if err := h.svc.ChangePassword(r.Context(), user.ID, current, next); err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}{true, "Password changed successfully"})
}

func (h handlers) impersonate(w http.ResponseWriter, r *http.Request) error {
	admin, err := h.svc.sessions.RequireMenuPrefix(r, iam.SettingsUsers...)
	if err != nil {
		return err
	}
	f := validate.New(validate.ReadBody(r))
	userID := f.UUID("user_id", validate.Rule{})
	if err := f.Err("Validation failed"); err != nil {
		return err
	}
	ua := ""
	if v, ok := r.Header["User-Agent"]; ok && len(v) > 0 {
		ua = v[0]
	}
	tokenHash := ""
	if token := auth.ReadSessionToken(r); token != "" {
		tokenHash = auth.HashToken(token)
	}
	// Canonical form for the self check and the lookups (EPIC-061).
	targetID, _ := validate.CanonicalUUID(*userID)
	target, session, err := h.svc.Impersonate(r.Context(), admin, tokenHash, targetID, ua)
	if err != nil {
		return err
	}
	setSessionCookie(w, r, session)
	name := "null" // `${target.full_name}` of a NULL name
	if target.FullName != nil {
		name = *target.FullName
	}
	return httpx.DataMessage(w, http.StatusOK, target, "Logged in as "+name)
}

// ── stall ────────────────────────────────────────────────────────────────

type stallData struct {
	Stall *StallView `json:"stall"`
}

func (h handlers) activeStall(w http.ResponseWriter, r *http.Request) error {
	user, err := h.svc.sessions.RequireUser(r)
	if err != nil {
		return err
	}
	f := validate.New(validate.ReadBody(r))
	warehouseID := f.UUID("warehouse_id", validate.Rule{Nullable: true})
	if err := f.Err("Validation failed"); err != nil {
		return err
	}
	choice, err := h.svc.SetActiveStall(r.Context(), user, warehouseID)
	if err != nil {
		return err
	}
	setActiveStallCookie(w, r, choice.CookieValue, h.svc.now())
	return httpx.Data(w, http.StatusOK, stallData{Stall: choice.Stall})
}

func (h handlers) stallOptions(w http.ResponseWriter, r *http.Request) error {
	user, err := h.svc.sessions.RequireUser(r)
	if err != nil {
		return err
	}
	opts, err := h.svc.GetStallOptions(r.Context(), user, stall.Cookie(r))
	if err != nil {
		return err
	}
	return httpx.Data(w, http.StatusOK, opts)
}

// ── scope ────────────────────────────────────────────────────────────────

// scopeFailure is the scope route's error body ({success:false, message}).
type scopeFailure struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func (h handlers) scope(w http.ResponseWriter, r *http.Request) error {
	view, err := h.svc.Scope(r)
	if err != nil {
		return err
	}
	if view == nil {
		return httpx.JSON(w, http.StatusUnauthorized, scopeFailure{Message: "Not authenticated"})
	}
	return httpx.Data(w, http.StatusOK, view)
}

// clientIP is lib/security/client-ip.ts: cf-connecting-ip (set by the
// Cloudflare edge), the left-most x-forwarded-for, x-real-ip, "unknown".
func clientIP(h http.Header) string {
	if ip := strings.TrimSpace(h.Get("Cf-Connecting-Ip")); ip != "" {
		return ip
	}
	if ip := strings.TrimSpace(strings.Split(h.Get("X-Forwarded-For"), ",")[0]); ip != "" {
		return ip
	}
	if ip := strings.TrimSpace(h.Get("X-Real-Ip")); ip != "" {
		return ip
	}
	return "unknown"
}

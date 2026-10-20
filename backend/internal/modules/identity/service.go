// Package identity serves the staff sign-in surface under /api/auth: login,
// logout, change-password, impersonation, the profile (me), the business
// scope and the sidebar's active stall. It owns the writes to auth.sessions,
// auth.login_attempts and auth.users.password_hash; reading a session stays
// in platform/auth. Every rule and message mirrors frontend/src/lib/auth and
// frontend/src/app/api/auth.
package identity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	mathrand "math/rand/v2"
	"net/http"
	"time"

	"nuhabit/backend/internal/modules/identity/domain"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/scope"
	"nuhabit/backend/internal/platform/stall"
	"nuhabit/backend/internal/platform/validate"
)

// Profile is the configuration.users row GET /api/auth/me returns.
type Profile struct {
	ID       string
	FullName *string
	Role     *string
	BrandID  *string
}

// Credentials is the auth.users row a login checks.
type Credentials struct {
	ID           string
	Email        string
	PasswordHash string
	BannedUntil  *time.Time
}

// Target is the account an impersonation switches to.
type Target struct {
	ID       string
	Email    string
	Role     *string
	FullName *string
	IsBanned bool
}

// NewSession is one auth.sessions row.
type NewSession struct {
	UserID    string
	TokenHash string
	UserAgent *string
	ExpiresAt time.Time
}

// Repository is the identity tables.
type Repository interface {
	// Profile returns nil, nil when the user has no configuration.users row.
	Profile(ctx context.Context, userID string) (*Profile, error)
	// Credentials finds auth.users by case-insensitive email; nil when none.
	Credentials(ctx context.Context, email string) (*Credentials, error)
	// CreateSession inserts the session and stamps last_sign_in_at.
	CreateSession(ctx context.Context, s NewSession) error
	DestroySession(ctx context.Context, tokenHash string) error
	// LoginAttempts counts the window's failures for the account and the IP.
	LoginAttempts(ctx context.Context, account, ip string) (accountCount, ipCount int, err error)
	RecordLoginFailure(ctx context.Context, account, ip string) error
	PruneLoginAttempts(ctx context.Context) error
	ClearLoginFailures(ctx context.Context, account string) error
	// PasswordHash returns found=false when the user has no auth.users row.
	PasswordHash(ctx context.Context, userID string) (hash string, found bool, err error)
	SetPasswordHash(ctx context.Context, userID, hash string) error
	// ImpersonationTarget returns nil when the id has no auth.users plus
	// configuration.users pair.
	ImpersonationTarget(ctx context.Context, userID string) (*Target, error)
	// HoldsSystemRole: the account holds super_admin/admin (or another
	// iam.roles is_system role) as app role or IAM role.
	HoldsSystemRole(ctx context.Context, userID string) (bool, error)
	// IsImpersonationSession: the session row was created by Login As.
	IsImpersonationSession(ctx context.Context, tokenHash string) (bool, error)
	Scope(ctx context.Context, userID string) (*scope.Scope, error)
}

// Stalls is the port to the warehouses (configuration context) behind the
// stall switcher; internal/app adapts it.
type Stalls interface {
	// Access is getStallAccess.
	Access(ctx context.Context, userID, role string, branchID *string) (domain.StallAccess, error)
	// Active returns the active warehouse with that id, nil when none.
	Active(ctx context.Context, id string) (*domain.Stall, error)
	// Branch returns the warehouse's branch_id, found=false when it is gone.
	Branch(ctx context.Context, id string) (branchID *string, found bool, err error)
}

// Sessions resolves the caller (platform/auth).
type Sessions interface {
	Session(r *http.Request) (*auth.SessionUser, error)
	RequireUser(r *http.Request) (*auth.User, error)
	RequireMenuPrefix(r *http.Request, prefixes ...string) (*auth.User, error)
	HasRoleCode(ctx context.Context, u *auth.User, code string) (bool, error)
}

// Service holds the use cases.
type Service struct {
	repo     Repository
	stalls   Stalls
	sessions Sessions
	now      func() time.Time
	// pruneRoll returns a number in [0, 1) (Math.random()).
	pruneRoll func() float64
	// warn logs a swallowed throttle failure.
	warn func(ctx context.Context, msg string, err error)
}

// NewService wires the use cases. now and warn may be nil.
func NewService(repo Repository, stalls Stalls, sessions Sessions, now func() time.Time,
	warn func(ctx context.Context, msg string, err error)) *Service {
	if now == nil {
		now = time.Now
	}
	if warn == nil {
		warn = func(context.Context, string, error) {}
	}
	return &Service{repo: repo, stalls: stalls, sessions: sessions, now: now, pruneRoll: mathrand.Float64, warn: warn}
}

// clientError is a failure whose body is the bare {"error": message} the
// auth routes answer with outside ApiError.
type clientError struct {
	status  int
	message string
}

func (e *clientError) Error() string { return e.message }

// ── me ───────────────────────────────────────────────────────────────────

// Me is the response data of GET /api/auth/me, in the TS key order
// ({...profile, id, email}).
type Me struct {
	ID       string  `json:"id"`
	FullName *string `json:"full_name"`
	Role     *string `json:"role"`
	BrandID  *string `json:"brand_id"`
	Email    string  `json:"email"`
}

// Me mirrors frontend/src/app/api/auth/me/route.ts: a session without a
// configuration.users row counts as signed out (401 "Not authenticated").
func (s *Service) Me(r *http.Request) (*Me, error) {
	user, err := s.sessions.Session(r)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, httpx.Unauthorized("Not authenticated")
	}
	p, err := s.repo.Profile(r.Context(), user.ID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, httpx.Unauthorized("Not authenticated")
	}
	return &Me{ID: user.ID, FullName: p.FullName, Role: p.Role, BrandID: p.BrandID, Email: user.Email}, nil
}

// ── sessions ─────────────────────────────────────────────────────────────

// Session is a freshly created session: the raw token for the cookie.
type Session struct {
	Token     string
	ExpiresAt time.Time
}

// createSession is createSession: a random 32-byte hex token stored as its
// SHA-256, valid SessionTTLDays.
func (s *Service) createSession(ctx context.Context, userID string, userAgent *string) (*Session, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(raw)
	// Date carries milliseconds; keep the same precision in the row and body.
	expiresAt := s.now().Add(auth.SessionTTLDays * 24 * time.Hour).Truncate(time.Millisecond)
	err := s.repo.CreateSession(ctx, NewSession{UserID: userID, TokenHash: auth.HashToken(token), UserAgent: userAgent, ExpiresAt: expiresAt})
	if err != nil {
		return nil, err
	}
	return &Session{Token: token, ExpiresAt: expiresAt}, nil
}

// ── login ────────────────────────────────────────────────────────────────

// LoginInput is the parsed POST /api/auth/login request. Email and Password
// are the raw JSON values.
type LoginInput struct {
	Email, Password any
	IP              string
	UserAgent       *string
}

// LoginResult is the signed-in user and the session to set as cookie.
type LoginResult struct {
	UserID, Email string
	Session       *Session
}

// Login mirrors POST /api/auth/login: the durable throttle first (a generic
// message that does not reveal whether the account exists), then the
// credential check, which records a failure, and on success clears the
// account's failures and creates a session.
func (s *Service) Login(ctx context.Context, in LoginInput) (*LoginResult, error) {
	if !jsTruthy(in.Email) || !jsTruthy(in.Password) {
		return nil, &clientError{http.StatusBadRequest, "Email and password required"}
	}
	email, ok := in.Email.(string)
	if !ok {
		// email.trim is not a function: the TS route answers 500.
		return nil, errNonStringCredential
	}
	account := domain.NormalizeAccountKey(email)
	if s.loginBlocked(ctx, account, in.IP) {
		return nil, &clientError{http.StatusTooManyRequests, "Terlalu banyak percobaan masuk. Coba lagi beberapa menit lagi."}
	}

	user, err := s.authenticate(ctx, email, in.Password)
	if err != nil {
		var ce *clientError
		if errors.As(err, &ce) && ce.status == http.StatusUnauthorized {
			s.recordLoginFailure(ctx, account, in.IP)
		}
		return nil, err
	}
	s.clearLoginFailures(ctx, account)

	session, err := s.createSession(ctx, user.ID, in.UserAgent)
	if err != nil {
		return nil, err
	}
	return &LoginResult{UserID: user.ID, Email: user.Email, Session: session}, nil
}

// errNonStringCredential stands for the TypeError the TS route turns into
// a 500.
var errNonStringCredential = errors.New("login credentials are not strings")

// authenticate is authenticateCredentials. The password is checked before
// the ban so a wrong password never reveals that an account is inactive.
func (s *Service) authenticate(ctx context.Context, email string, password any) (*Credentials, error) {
	invalid := &clientError{http.StatusUnauthorized, "Invalid login credentials"}
	row, err := s.repo.Credentials(ctx, validate.JSTrim(email))
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, invalid
	}
	pw, ok := password.(string)
	if !ok {
		// bcrypt.compare throws "Illegal arguments": 500 in the TS route.
		return nil, errNonStringCredential
	}
	if !verifyPassword(pw, row.PasswordHash) {
		return nil, invalid
	}
	if row.BannedUntil != nil && row.BannedUntil.After(s.now()) {
		return nil, &clientError{http.StatusUnauthorized, "This account is inactive. Contact your administrator."}
	}
	return row, nil
}

// loginBlocked is isLoginBlocked: a read failure never blocks a login (the
// password still guards, and a broken database fails the login anyway).
func (s *Service) loginBlocked(ctx context.Context, account, ip string) bool {
	accountCount, ipCount, err := s.repo.LoginAttempts(ctx, account, ip)
	if err != nil {
		s.warn(ctx, "[login-throttle] gagal membaca percobaan", err)
		return false
	}
	return domain.ShouldBlockLogin(accountCount, ipCount)
}

// recordLoginFailure is recordLoginFailure, pruning old rows now and then.
func (s *Service) recordLoginFailure(ctx context.Context, account, ip string) {
	err := s.repo.RecordLoginFailure(ctx, account, ip)
	if err == nil && s.pruneRoll() < domain.PruneChance {
		err = s.repo.PruneLoginAttempts(ctx)
	}
	if err != nil {
		s.warn(ctx, "[login-throttle] gagal mencatat percobaan", err)
	}
}

func (s *Service) clearLoginFailures(ctx context.Context, account string) {
	if err := s.repo.ClearLoginFailures(ctx, account); err != nil {
		s.warn(ctx, "[login-throttle] gagal membersihkan percobaan", err)
	}
}

// ── logout ───────────────────────────────────────────────────────────────

// Logout deletes the sessions behind the given cookie values (new and legacy
// names, deduplicated). No cookie means no query.
func (s *Service) Logout(ctx context.Context, tokens []string) error {
	seen := map[string]bool{}
	for _, token := range tokens {
		if token == "" || seen[token] {
			continue
		}
		seen[token] = true
		if err := s.repo.DestroySession(ctx, auth.HashToken(token)); err != nil {
			return err
		}
	}
	return nil
}

// ── change password ──────────────────────────────────────────────────────

// ChangePassword mirrors POST /api/auth/change-password. Like the TS route
// it keeps the caller's other sessions alive.
func (s *Service) ChangePassword(ctx context.Context, userID, current, next string) error {
	if msg := domain.ChangePasswordProblem(current, next); msg != "" {
		return &clientError{http.StatusBadRequest, msg}
	}
	hash, found, err := s.repo.PasswordHash(ctx, userID)
	if err != nil {
		return err
	}
	if !found || !verifyPassword(current, hash) {
		return &clientError{http.StatusBadRequest, "Current password is incorrect"}
	}
	newHash, err := hashPassword(next)
	if err != nil {
		return err
	}
	return s.repo.SetPasswordHash(ctx, userID, newHash)
}

// ── impersonate ──────────────────────────────────────────────────────────

var (
	errSystemImpersonation = httpx.Forbidden("Login As ke akun Super Admin/Admin hanya untuk Super Admin")
	errNestedImpersonation = httpx.Forbidden("Tidak bisa Login As dari sesi Login As; keluar lalu masuk dengan akun sendiri")
)

// Impersonated is the data of POST /api/auth/impersonate.
type Impersonated struct {
	ID       string  `json:"id"`
	Email    string  `json:"email"`
	Role     *string `json:"role"`
	FullName *string `json:"full_name"`
}

// Impersonate mirrors POST /api/auth/impersonate ("Login As"): never the
// caller's own account, a missing or banned account, or a super admin, and
// never from inside a Login As session. An account holding a system role
// (super_admin/admin as app or IAM role) is for IAM super_admin callers
// only (EPIC-061). The new session's user agent records the admin
// ("impersonated-by:<id> <ua>"), which is the only trail the TS route leaves.
// sessionTokenHash is the caller's session (empty when none).
func (s *Service) Impersonate(ctx context.Context, admin *auth.User, sessionTokenHash, targetID string, userAgent string) (*Impersonated, *Session, error) {
	if sessionTokenHash != "" {
		nested, err := s.repo.IsImpersonationSession(ctx, sessionTokenHash)
		if err != nil {
			return nil, nil, err
		}
		if nested {
			return nil, nil, errNestedImpersonation
		}
	}
	if targetID == admin.ID {
		return nil, nil, httpx.BadRequest("You are already logged in as this account")
	}
	target, err := s.repo.ImpersonationTarget(ctx, targetID)
	if err != nil {
		return nil, nil, err
	}
	if target == nil {
		return nil, nil, httpx.NotFound("User account not found")
	}
	if target.IsBanned {
		return nil, nil, httpx.BadRequest("User account is banned")
	}
	if target.Role != nil && *target.Role == "super_admin" {
		return nil, nil, httpx.Forbidden("Cannot impersonate a super admin account")
	}
	system, err := s.repo.HoldsSystemRole(ctx, target.ID)
	if err != nil {
		return nil, nil, err
	}
	if system {
		super, err := s.sessions.HasRoleCode(ctx, admin, "super_admin")
		if err != nil {
			return nil, nil, err
		}
		if !super {
			return nil, nil, errSystemImpersonation
		}
	}
	ua := validate.JSTrim("impersonated-by:" + admin.ID + " " + userAgent)
	session, err := s.createSession(ctx, target.ID, &ua)
	if err != nil {
		return nil, nil, err
	}
	return &Impersonated{ID: target.ID, Email: target.Email, Role: target.Role, FullName: target.FullName}, session, nil
}

// ── scope ────────────────────────────────────────────────────────────────

// ScopeView is the data of GET /api/auth/scope.
type ScopeView struct {
	Role          *string `json:"role"`
	BusinessScope *string `json:"business_scope"`
	BranchID      *string `json:"branch_id"`
	CompanyID     *string `json:"company_id"`
	HoldingID     *string `json:"holding_id"`
	IsUnscoped    bool    `json:"is_unscoped"`
}

// Scope mirrors GET /api/auth/scope (getApiUserScope): the session's user,
// profile row optional. nil, nil means no session.
func (s *Service) Scope(r *http.Request) (*ScopeView, error) {
	user, err := s.sessions.Session(r)
	if err != nil || user == nil {
		return nil, err
	}
	sc, err := s.repo.Scope(r.Context(), user.ID)
	if err != nil {
		return nil, err
	}
	return &ScopeView{Role: sc.Role, BusinessScope: sc.BusinessScope, BranchID: sc.BranchID,
		CompanyID: sc.CompanyID, HoldingID: sc.HoldingID, IsUnscoped: sc.Unscoped}, nil
}

// ── active stall ─────────────────────────────────────────────────────────

// StallView is a stall in the stall responses.
type StallView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Code string `json:"code"`
}

func viewOf(s domain.Stall) *StallView { return &StallView{ID: s.ID, Name: s.Name, Code: s.Code} }

// stallAccess loads the caller's switcher access within their branch.
func (s *Service) stallAccess(ctx context.Context, user *auth.User) (domain.StallAccess, *scope.Scope, error) {
	sc, err := s.repo.Scope(ctx, user.ID)
	if err != nil {
		return domain.StallAccess{}, nil, err
	}
	access, err := s.stalls.Access(ctx, user.ID, user.Role, sc.BranchID)
	return access, sc, err
}

// findActiveStall is findActiveStall: ids outside UUID_RE are never looked up.
func (s *Service) findActiveStall(ctx context.Context, id string) (*domain.Stall, error) {
	if !domain.IsStallID(id) {
		return nil, nil
	}
	return s.stalls.Active(ctx, id)
}

// ActiveStallChoice is what POST /api/auth/active-stall stores: the cookie
// value and the chosen stall (nil for "Semua Stall").
type ActiveStallChoice struct {
	CookieValue string
	Stall       *StallView
}

// SetActiveStall mirrors POST /api/auth/active-stall. warehouseID nil means
// "Semua Stall", which only users without a placement restriction may pick.
func (s *Service) SetActiveStall(ctx context.Context, user *auth.User, warehouseID *string) (*ActiveStallChoice, error) {
	access, sc, err := s.stallAccess(ctx, user)
	if err != nil {
		return nil, err
	}
	if !domain.CanSwitchStall(user.Role, access) {
		return nil, httpx.Forbidden("Akun ini tidak perlu mengganti outlet")
	}
	if warehouseID == nil {
		if !access.AllAccess {
			return nil, httpx.Forbidden("Anda hanya dapat memilih outlet penempatan Anda")
		}
		return &ActiveStallChoice{CookieValue: stall.All}, nil
	}
	if !access.AllAccess && !access.HasStall(*warehouseID) {
		return nil, httpx.Forbidden("Outlet di luar penempatan Anda")
	}
	chosen, err := s.findActiveStall(ctx, *warehouseID)
	if err != nil {
		return nil, err
	}
	if chosen == nil {
		return nil, httpx.NotFound("Outlet not found or inactive")
	}
	// The stall must sit in the caller's branch when they have one.
	if sc.BranchID != nil && *sc.BranchID != "" {
		branch, found, err := s.stalls.Branch(ctx, chosen.ID)
		if err != nil {
			return nil, err
		}
		if found && (branch == nil || *branch != *sc.BranchID) {
			return nil, httpx.Forbidden("Outlet berada di luar branch Anda")
		}
	}
	return &ActiveStallChoice{CookieValue: chosen.ID, Stall: viewOf(*chosen)}, nil
}

// StallOptions is the data of GET /api/auth/stall-options.
type StallOptions struct {
	AllAccess bool         `json:"all_access"`
	CanSwitch bool         `json:"can_switch"`
	Active    *StallView   `json:"active"`
	Stalls    []*StallView `json:"stalls"`
}

// GetStallOptions mirrors GET /api/auth/stall-options. cookie is the active
// stall cookie value ("" unset, "all", or a stall id). Every signed-in user
// reads their active stall; the list is only sent to users who may switch.
func (s *Service) GetStallOptions(ctx context.Context, user *auth.User, cookie string) (*StallOptions, error) {
	access, _, err := s.stallAccess(ctx, user)
	if err != nil {
		return nil, err
	}
	canSwitch := domain.CanSwitchStall(user.Role, access)

	var resolved *domain.Stall
	if cookie != "" && cookie != stall.All {
		if resolved, err = s.findActiveStall(ctx, cookie); err != nil {
			return nil, err
		}
	}
	out := &StallOptions{AllAccess: access.AllAccess, CanSwitch: canSwitch, Stalls: []*StallView{}}
	switch {
	case resolved != nil:
		out.Active = viewOf(*resolved)
	case cookie == stall.All || access.AllAccess:
		// "Semua Stall": explicit, or the unset fallback of full-access users.
	case len(access.Stalls) > 0:
		out.Active = viewOf(access.Stalls[0])
	}
	if canSwitch {
		for _, st := range access.Stalls {
			out.Stalls = append(out.Stalls, viewOf(st))
		}
	}
	return out, nil
}

// ── helpers ──────────────────────────────────────────────────────────────

// jsTruthy is JS truthiness for a decoded JSON value.
func jsTruthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case float64:
		return x != 0
	}
	return true
}

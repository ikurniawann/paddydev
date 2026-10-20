package identity

import (
	"context"
	"strconv"

	"nuhabit/backend/internal/modules/identity/domain"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/permaudit"
	"nuhabit/backend/internal/platform/scope"
)

// Postgres implements Repository with the SQL of lib/auth/session.ts,
// lib/auth/login-throttle.ts and the auth routes.
type Postgres struct{ db database.Querier }

// NewPostgres builds the repository on a pool or transaction.
func NewPostgres(db database.Querier) *Postgres { return &Postgres{db: db} }

// Profile reads configuration.users. email is not selected: that column is
// optional there, the session (auth.users) supplies it.
func (p *Postgres) Profile(ctx context.Context, userID string) (*Profile, error) {
	var out Profile
	err := p.db.QueryRow(ctx,
		`SELECT id::text, full_name, role, brand_id::text FROM configuration.users WHERE id = $1`, userID).
		Scan(&out.ID, &out.FullName, &out.Role, &out.BrandID)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Credentials is authenticateCredentials' lookup. A NULL hash reads as ""
// so it never verifies.
func (p *Postgres) Credentials(ctx context.Context, email string) (*Credentials, error) {
	var c Credentials
	err := p.db.QueryRow(ctx, `SELECT id::text, email, COALESCE(password_hash, ''), banned_until
		FROM auth.users WHERE lower(email) = lower($1)`, email).
		Scan(&c.ID, &c.Email, &c.PasswordHash, &c.BannedUntil)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// CreateSession is createSession's two statements (ip_address stays NULL:
// no auth route passes it).
func (p *Postgres) CreateSession(ctx context.Context, s NewSession) error {
	if _, err := p.db.Exec(ctx, `INSERT INTO auth.sessions (user_id, token_hash, user_agent, ip_address, expires_at)
		VALUES ($1, $2, $3, NULL, $4)`, s.UserID, s.TokenHash, s.UserAgent, s.ExpiresAt); err != nil {
		return err
	}
	_, err := p.db.Exec(ctx, `UPDATE auth.users SET last_sign_in_at = NOW() WHERE id = $1`, s.UserID)
	return err
}

// DestroySession is destroySession.
func (p *Postgres) DestroySession(ctx context.Context, tokenHash string) error {
	_, err := p.db.Exec(ctx, `DELETE FROM auth.sessions WHERE token_hash = $1`, tokenHash)
	return err
}

// LoginAttempts is isLoginBlocked's count.
func (p *Postgres) LoginAttempts(ctx context.Context, account, ip string) (int, int, error) {
	var accountCount, ipCount int
	err := p.db.QueryRow(ctx, `SELECT
		count(*) FILTER (WHERE account_key = $1)::int AS account_count,
		count(*) FILTER (WHERE ip = $2)::int           AS ip_count
		FROM auth.login_attempts
		WHERE created_at > now() - ($3 || ' minutes')::interval`,
		account, ip, strconv.Itoa(domain.LoginWindowMinutes)).Scan(&accountCount, &ipCount)
	return accountCount, ipCount, err
}

// RecordLoginFailure is recordLoginFailure's insert.
func (p *Postgres) RecordLoginFailure(ctx context.Context, account, ip string) error {
	_, err := p.db.Exec(ctx, `INSERT INTO auth.login_attempts (account_key, ip) VALUES ($1, $2)`, account, ip)
	return err
}

// PruneLoginAttempts is recordLoginFailure's occasional cleanup.
func (p *Postgres) PruneLoginAttempts(ctx context.Context) error {
	_, err := p.db.Exec(ctx, `DELETE FROM auth.login_attempts
		WHERE created_at < now() - ($1 || ' minutes')::interval`, strconv.Itoa(domain.PruneOlderThanMinutes))
	return err
}

// ClearLoginFailures is clearLoginFailures.
func (p *Postgres) ClearLoginFailures(ctx context.Context, account string) error {
	_, err := p.db.Exec(ctx, `DELETE FROM auth.login_attempts WHERE account_key = $1`, account)
	return err
}

// PasswordHash is the change-password lookup.
func (p *Postgres) PasswordHash(ctx context.Context, userID string) (string, bool, error) {
	var hash string
	err := p.db.QueryRow(ctx, `SELECT COALESCE(password_hash, '') FROM auth.users WHERE id = $1`, userID).Scan(&hash)
	if database.IsNoRows(err) {
		return "", false, nil
	}
	return hash, err == nil, err
}

// SetPasswordHash is the change-password update.
func (p *Postgres) SetPasswordHash(ctx context.Context, userID, hash string) error {
	_, err := p.db.Exec(ctx, `UPDATE auth.users SET password_hash = $1, updated_at = NOW() WHERE id = $2`, hash, userID)
	return err
}

// ImpersonationTarget is the impersonate route's lookup.
func (p *Postgres) ImpersonationTarget(ctx context.Context, userID string) (*Target, error) {
	var t Target
	err := p.db.QueryRow(ctx, `SELECT au.id::text, au.email, cu.role, cu.full_name,
		(au.banned_until IS NOT NULL AND au.banned_until > NOW()) AS is_banned
		FROM auth.users au
		JOIN configuration.users cu ON cu.id = au.id
		WHERE au.id = $1`, userID).Scan(&t.ID, &t.Email, &t.Role, &t.FullName, &t.IsBanned)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// HoldsSystemRole is permaudit.HoldsSystemRole.
func (p *Postgres) HoldsSystemRole(ctx context.Context, userID string) (bool, error) {
	return permaudit.HoldsSystemRole(ctx, p.db, userID)
}

// IsImpersonationSession: the session's user agent carries the Login As
// marker ("impersonated-by:<id> <ua>").
func (p *Postgres) IsImpersonationSession(ctx context.Context, tokenHash string) (bool, error) {
	var nested bool
	err := p.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM auth.sessions WHERE token_hash = $1 AND user_agent LIKE 'impersonated-by:%')`,
		tokenHash).Scan(&nested)
	return nested, err
}

// Scope is getApiUserScope's profile read.
func (p *Postgres) Scope(ctx context.Context, userID string) (*scope.Scope, error) {
	return scope.Load(ctx, p.db, userID)
}

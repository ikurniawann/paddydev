package identity_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"nuhabit/backend/internal/modules/identity"
	"nuhabit/backend/internal/modules/identity/domain"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/module"
	"nuhabit/backend/internal/platform/testutil"
)

// tsHash is bcryptjs.hashSync("rahasia-123", 10) from the TS app: Go must
// verify the hashes already in auth.users.
const (
	tsPassword = "rahasia-123"
	tsHash     = "$2b$10$WcqXOREe4/VUZZDAzgnX1OEFkluvFSKukf8/.IUqmCkW7dwO3Ng2e"
)

// noStalls is the stall port for routes that never reach it; the stall
// routes are tested with the real adapter in internal/app.
type noStalls struct{ identity.Stalls }

func newModule(t *testing.T) (module.Deps, http.Handler) {
	deps := testutil.Deps(t, nil)
	return deps, testutil.Mux(identity.New(deps, noStalls{}))
}

func exec(t *testing.T, deps module.Deps, sql string, args ...any) {
	t.Helper()
	if _, err := deps.DB.Exec(context.Background(), sql, args...); err != nil {
		t.Fatal(err)
	}
}

func count(t *testing.T, deps module.Deps, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := deps.DB.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// withPassword gives a throwaway staff account the TS-written hash.
func withPassword(t *testing.T, deps module.Deps, s testutil.Staff) {
	exec(t, deps, `UPDATE auth.users SET password_hash = $1 WHERE id = $2`, tsHash, s.UserID)
}

// loginFrom builds a login request from a test-unique client IP and cleans
// that IP's attempts afterwards.
func loginFrom(t *testing.T, deps module.Deps, ip string, body any) *http.Request {
	t.Cleanup(func() {
		_, _ = deps.DB.Exec(context.Background(), `DELETE FROM auth.login_attempts WHERE ip = $1`, ip)
	})
	r := testutil.Request("POST", "/api/auth/login", body)
	r.Header.Set("Cf-Connecting-Ip", ip)
	r.Header.Set("User-Agent", "go-test-agent")
	return r
}

func testIP() string { return "198.51.100." + testutil.RandomHex(2) }

func setCookies(rec interface{ Result() *http.Response }) []string {
	return rec.Result().Header.Values("Set-Cookie")
}

func TestAuthMe(t *testing.T) {
	deps, mux := newModule(t)
	staff := testutil.CreateStaff(t, testutil.StaffOptions{Role: "admin", FullName: "Ayu Test"})

	rec, body := testutil.Do(t, mux, testutil.Request("GET", "/api/auth/me", nil))
	if rec.Code != 401 || body["error"] != "Not authenticated" || body["success"] != false {
		t.Fatalf("no session: %d %s", rec.Code, rec.Body.String())
	}

	rec, _ = testutil.Do(t, mux, testutil.AsStaff(testutil.Request("GET", "/api/auth/me", nil), staff))
	want := `{"success":true,"data":{"id":"` + staff.UserID + `","full_name":"Ayu Test","role":"admin","brand_id":null,"email":"` + staff.Email + `"}}`
	if rec.Code != 200 || rec.Body.String() != want {
		t.Fatalf("signed in: %d\n got %s\nwant %s", rec.Code, rec.Body.String(), want)
	}

	exec(t, deps, `DELETE FROM configuration.users WHERE id = $1`, staff.UserID)
	rec, body = testutil.Do(t, mux, testutil.AsStaff(testutil.Request("GET", "/api/auth/me", nil), staff))
	if rec.Code != 401 || body["error"] != "Not authenticated" {
		t.Fatalf("no profile: %d %s", rec.Code, rec.Body.String())
	}
}

// Through the full app handler the gate answers first, like the Next proxy.
func TestAuthMeBehindGate(t *testing.T) {
	deps, mux := newModule(t)
	rec, body := testutil.Do(t, deps.Auth.Gate(mux), testutil.Request("GET", "/api/auth/me", nil))
	if rec.Code != http.StatusUnauthorized || body["error"] != "Authentication required" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestLoginSuccessWritesSessionCookie(t *testing.T) {
	deps, mux := newModule(t)
	staff := testutil.CreateStaff(t, testutil.StaffOptions{})
	withPassword(t, deps, staff)
	ip := testIP()
	account := strings.ToLower(staff.Email)
	exec(t, deps, `INSERT INTO auth.login_attempts (account_key, ip) VALUES ($1, $2)`, account, ip)

	// The email is matched case-insensitively after a trim.
	started := time.Now()
	rec, body := testutil.Do(t, mux, loginFrom(t, deps, ip, map[string]any{"email": "  " + strings.ToUpper(staff.Email) + " ", "password": tsPassword}))
	if rec.Code != 200 {
		t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
	}
	data := body["data"].(map[string]any)
	if user := data["user"].(map[string]any); user["id"] != staff.UserID || user["email"] != staff.Email {
		t.Fatalf("user = %v", user)
	}
	session := data["session"].(map[string]any)
	token := session["access_token"].(string)
	expires, err := time.Parse("2006-01-02T15:04:05.000Z", session["expires_at"].(string))
	if err != nil || len(token) != 64 {
		t.Fatalf("session = %v (%v)", session, err)
	}
	if d := expires.Sub(started); d < 14*24*time.Hour-time.Minute || d > 14*24*time.Hour+time.Minute {
		t.Fatalf("expires_at %s is not 14 days out", expires)
	}

	cookies := setCookies(rec)
	wantSession := "nuhabit_session=" + token + "; Path=/; Expires=" + expires.Format(http.TimeFormat) + "; Secure; HttpOnly; SameSite=lax"
	wantLegacy := "arkiv_session=; Path=/; Expires=Thu, 01 Jan 1970 00:00:00 GMT; Max-Age=0; Secure; HttpOnly; SameSite=lax"
	if len(cookies) != 2 || cookies[0] != wantSession || cookies[1] != wantLegacy {
		t.Fatalf("cookies =\n%s\nwant\n%s\n%s", strings.Join(cookies, "\n"), wantSession, wantLegacy)
	}

	if n := count(t, deps, `SELECT count(*) FROM auth.sessions WHERE token_hash = $1 AND user_id = $2
		AND user_agent = 'go-test-agent' AND ip_address IS NULL`, auth.HashToken(token), staff.UserID); n != 1 {
		t.Fatalf("session rows = %d", n)
	}
	if n := count(t, deps, `SELECT count(*) FROM auth.users WHERE id = $1 AND last_sign_in_at > now() - interval '1 minute'`, staff.UserID); n != 1 {
		t.Fatal("last_sign_in_at not stamped")
	}
	if n := count(t, deps, `SELECT count(*) FROM auth.login_attempts WHERE account_key = $1`, account); n != 0 {
		t.Fatalf("failures not cleared: %d", n)
	}

	// The new cookie signs the caller in.
	r := testutil.Request("GET", "/api/auth/me", nil)
	r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: token})
	if rec, _ := testutil.Do(t, mux, r); rec.Code != 200 {
		t.Fatalf("me with new cookie: %d", rec.Code)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	deps, mux := newModule(t)
	staff := testutil.CreateStaff(t, testutil.StaffOptions{})
	withPassword(t, deps, staff)
	ip := testIP()

	for _, email := range []string{staff.Email, "nobody-" + testutil.RandomHex(4) + "@test.local"} {
		t.Cleanup(func() {
			_, _ = deps.DB.Exec(context.Background(), `DELETE FROM auth.login_attempts WHERE account_key = $1`, email)
		})
		rec, _ := testutil.Do(t, mux, loginFrom(t, deps, ip, map[string]any{"email": email, "password": "salah"}))
		if rec.Code != 401 || rec.Body.String() != `{"error":"Invalid login credentials"}` {
			t.Fatalf("%s: %d %s", email, rec.Code, rec.Body.String())
		}
		if c := setCookies(rec); len(c) != 0 {
			t.Fatalf("cookies on failure: %v", c)
		}
		if n := count(t, deps, `SELECT count(*) FROM auth.login_attempts WHERE account_key = $1 AND ip = $2`, email, ip); n != 1 {
			t.Fatalf("%s: recorded failures = %d", email, n)
		}
	}
	if n := count(t, deps, `SELECT count(*) FROM auth.sessions WHERE user_id = $1`, staff.UserID); n != 1 {
		t.Fatalf("sessions = %d, want only the fixture's", n)
	}
}

func TestLoginRejectsBadInput(t *testing.T) {
	_, mux := newModule(t)
	for _, c := range []struct {
		body   any
		status int
		want   string
	}{
		{map[string]any{"email": "a@b.c"}, 400, `{"error":"Email and password required"}`},
		{map[string]any{"email": "", "password": "x"}, 400, `{"error":"Email and password required"}`},
		{[]any{1}, 400, `{"error":"Email and password required"}`},
		{"{not json", 500, `{"error":"Login failed"}`},
		{"null", 500, `{"error":"Login failed"}`},
	} {
		r := testutil.Request("POST", "/api/auth/login", c.body)
		if rec, _ := testutil.Do(t, mux, r); rec.Code != c.status || rec.Body.String() != c.want {
			t.Errorf("%v: %d %s", c.body, rec.Code, rec.Body.String())
		}
	}
}

func TestLoginBannedAccount(t *testing.T) {
	deps, mux := newModule(t)
	staff := testutil.CreateStaff(t, testutil.StaffOptions{})
	withPassword(t, deps, staff)
	exec(t, deps, `UPDATE auth.users SET banned_until = now() + interval '1 day' WHERE id = $1`, staff.UserID)
	t.Cleanup(func() {
		_, _ = deps.DB.Exec(context.Background(), `DELETE FROM auth.login_attempts WHERE account_key = $1`, staff.Email)
	})

	rec, _ := testutil.Do(t, mux, loginFrom(t, deps, testIP(), map[string]any{"email": staff.Email, "password": tsPassword}))
	if rec.Code != 401 || rec.Body.String() != `{"error":"This account is inactive. Contact your administrator."}` {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	// A wrong password on a banned account reveals nothing about the ban.
	rec, _ = testutil.Do(t, mux, loginFrom(t, deps, testIP(), map[string]any{"email": staff.Email, "password": "salah"}))
	if rec.Body.String() != `{"error":"Invalid login credentials"}` {
		t.Fatalf("wrong password on banned: %s", rec.Body.String())
	}
}

func TestLoginLockout(t *testing.T) {
	deps, mux := newModule(t)
	staff := testutil.CreateStaff(t, testutil.StaffOptions{})
	withPassword(t, deps, staff)
	t.Cleanup(func() {
		_, _ = deps.DB.Exec(context.Background(), `DELETE FROM auth.login_attempts WHERE account_key = $1`, staff.Email)
	})
	blocked := `{"error":"Terlalu banyak percobaan masuk. Coba lagi beberapa menit lagi."}`

	// Seven wrong passwords still answer 401; the eighth failure locks the
	// account, even for the right password.
	ip := testIP()
	for i := 0; i < domain.LoginMaxPerAccount; i++ {
		rec, _ := testutil.Do(t, mux, loginFrom(t, deps, ip, map[string]any{"email": staff.Email, "password": "salah"}))
		if rec.Code != 401 {
			t.Fatalf("attempt %d: %d %s", i+1, rec.Code, rec.Body.String())
		}
	}
	rec, _ := testutil.Do(t, mux, loginFrom(t, deps, testIP(), map[string]any{"email": staff.Email, "password": tsPassword}))
	if rec.Code != 429 || rec.Body.String() != blocked {
		t.Fatalf("locked account: %d %s", rec.Code, rec.Body.String())
	}
	if n := count(t, deps, `SELECT count(*) FROM auth.sessions WHERE user_id = $1`, staff.UserID); n != 1 {
		t.Fatalf("a locked login created a session (%d)", n)
	}
	// Failures outside the 5-minute window no longer count.
	exec(t, deps, `UPDATE auth.login_attempts SET created_at = now() - interval '6 minutes' WHERE account_key = $1`, staff.Email)
	if rec, _ := testutil.Do(t, mux, loginFrom(t, deps, testIP(), map[string]any{"email": staff.Email, "password": tsPassword})); rec.Code != 200 {
		t.Fatalf("after the window: %d %s", rec.Code, rec.Body.String())
	}

	// The per-IP limit is looser: 60 failures from one address block it for
	// every account.
	natIP := testIP()
	exec(t, deps, `INSERT INTO auth.login_attempts (account_key, ip)
		SELECT 'go-test-nat-' || g || '@test.local', $1 FROM generate_series(1, $2::int) g`, natIP, domain.LoginMaxPerIP)
	if rec, _ := testutil.Do(t, mux, loginFrom(t, deps, natIP, map[string]any{"email": staff.Email, "password": tsPassword})); rec.Code != 429 || rec.Body.String() != blocked {
		t.Fatalf("locked ip: %d %s", rec.Code, rec.Body.String())
	}
}

func TestLoginCookieOnPlainHTTP(t *testing.T) {
	deps, mux := newModule(t)
	staff := testutil.CreateStaff(t, testutil.StaffOptions{})
	withPassword(t, deps, staff)

	// LAN IP straight to the app: no Secure flag, or the browser drops it.
	r := loginFrom(t, deps, testIP(), map[string]any{"email": staff.Email, "password": tsPassword})
	r.Host = "10.20.89.5:3004"
	r.Header.Set("X-Forwarded-Proto", "http")
	rec, _ := testutil.Do(t, mux, r)
	if c := setCookies(rec); rec.Code != 200 || strings.Contains(strings.Join(c, "\n"), "Secure") {
		t.Fatalf("plain http: %d %v", rec.Code, c)
	}

	// Behind the Next proxy Go's Host is internal; the original host travels
	// in X-Forwarded-Host.
	r = loginFrom(t, deps, testIP(), map[string]any{"email": staff.Email, "password": tsPassword})
	r.Host = "backend:8080"
	r.Header.Set("X-Forwarded-Host", "10.20.89.5:3004")
	r.Header.Set("X-Forwarded-Proto", "http")
	rec, _ = testutil.Do(t, mux, r)
	if c := setCookies(rec); strings.Contains(strings.Join(c, "\n"), "Secure") {
		t.Fatalf("proxied plain http: %v", c)
	}
}

func TestLogout(t *testing.T) {
	deps, mux := newModule(t)
	a := testutil.CreateStaff(t, testutil.StaffOptions{})
	b := testutil.CreateStaff(t, testutil.StaffOptions{})

	r := testutil.Request("POST", "/api/auth/logout", nil)
	r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: a.Token})
	r.AddCookie(&http.Cookie{Name: auth.LegacySessionCookie, Value: b.Token})
	rec, _ := testutil.Do(t, mux, r)
	if rec.Code != 200 || rec.Body.String() != `{"success":true}` {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if n := count(t, deps, `SELECT count(*) FROM auth.sessions WHERE token_hash = ANY($1)`,
		[]string{auth.HashToken(a.Token), auth.HashToken(b.Token)}); n != 0 {
		t.Fatalf("sessions left: %d", n)
	}
	want := []string{
		"nuhabit_session=; Path=/; Expires=Thu, 01 Jan 1970 00:00:00 GMT; Max-Age=0; Secure; HttpOnly; SameSite=lax",
		"arkiv_session=; Path=/; Expires=Thu, 01 Jan 1970 00:00:00 GMT; Max-Age=0; Secure; HttpOnly; SameSite=lax",
		"arkiv-active-stall=; Path=/; Max-Age=0",
	}
	if got := setCookies(rec); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("cookies:\n%s", strings.Join(got, "\n"))
	}

	// GET without cookies redirects to /login and touches nothing.
	rec, _ = testutil.Do(t, mux, testutil.Request("GET", "/api/auth/logout", nil))
	if rec.Code != 307 || rec.Header().Get("Location") != "/login" || rec.Body.Len() != 0 {
		t.Fatalf("GET: %d %v %q", rec.Code, rec.Header(), rec.Body.String())
	}
	// Logout is public: the gate lets a stale cookie through.
	r = testutil.AsStaff(testutil.Request("GET", "/api/auth/logout", nil), a)
	if rec, _ := testutil.Do(t, deps.Auth.Gate(mux), r); rec.Code != 307 {
		t.Fatalf("GET with stale cookie behind gate: %d", rec.Code)
	}
}

func TestChangePassword(t *testing.T) {
	deps, mux := newModule(t)
	staff := testutil.CreateStaff(t, testutil.StaffOptions{})
	withPassword(t, deps, staff)
	change := func(body any) (int, string) {
		rec, _ := testutil.Do(t, mux, testutil.AsStaff(testutil.Request("POST", "/api/auth/change-password", body), staff))
		return rec.Code, rec.Body.String()
	}

	if rec, _ := testutil.Do(t, mux, testutil.Request("POST", "/api/auth/change-password", map[string]any{})); rec.Code != 401 ||
		rec.Body.String() != `{"success":false,"error":"Authentication required"}` {
		t.Fatalf("anonymous: %d %s", rec.Code, rec.Body.String())
	}
	for _, c := range []struct {
		body any
		want string
	}{
		{"not json", `{"error":"Current password and new password are required"}`},
		{map[string]any{"current_password": tsPassword, "new_password": 12345678}, `{"error":"Current password and new password are required"}`},
		{map[string]any{"current_password": tsPassword, "new_password": "pendek"}, `{"error":"New password must be at least 8 characters"}`},
		{map[string]any{"current_password": tsPassword, "new_password": tsPassword}, `{"error":"New password must be different from the current password"}`},
		{map[string]any{"current_password": "salah-sekali", "new_password": "baru-12345"}, `{"error":"Current password is incorrect"}`},
	} {
		if status, body := change(c.body); status != 400 || body != c.want {
			t.Errorf("%v: %d %s", c.body, status, body)
		}
	}

	status, body := change(map[string]any{"current_password": tsPassword, "new_password": "baru-12345"})
	if status != 200 || body != `{"success":true,"message":"Password changed successfully"}` {
		t.Fatalf("change: %d %s", status, body)
	}
	if n := count(t, deps, `SELECT count(*) FROM auth.users WHERE id = $1 AND password_hash LIKE '$2b$10$%'`, staff.UserID); n != 1 {
		t.Fatal("hash is not a bcryptjs-style $2b$10$ hash")
	}
	// The caller's session survives, as in TS, and the new password signs in.
	if status, _ := change(map[string]any{"current_password": "baru-12345", "new_password": "lagi-12345"}); status != 200 {
		t.Fatalf("second change with the new password: %d", status)
	}
	if rec, _ := testutil.Do(t, mux, loginFrom(t, deps, testIP(), map[string]any{"email": staff.Email, "password": "lagi-12345"})); rec.Code != 200 {
		t.Fatalf("login with changed password: %d %s", rec.Code, rec.Body.String())
	}
}

func TestImpersonate(t *testing.T) {
	deps, mux := newModule(t)
	admin := testutil.CreateStaff(t, testutil.StaffOptions{Role: "admin", Menus: map[string][]string{"settings.users": nil}})
	plain := testutil.CreateStaff(t, testutil.StaffOptions{Role: "pos", Menus: map[string][]string{"pos.operations": nil}})
	target := testutil.CreateStaff(t, testutil.StaffOptions{Role: "pos", FullName: "Kasir Satu"})
	superAdmin := testutil.CreateStaff(t, testutil.StaffOptions{Role: "super_admin"})
	banned := testutil.CreateStaff(t, testutil.StaffOptions{Role: "pos"})
	exec(t, deps, `UPDATE auth.users SET banned_until = now() + interval '1 day' WHERE id = $1`, banned.UserID)

	post := func(as testutil.Staff, body any) (int, string, []string) {
		r := testutil.AsStaff(testutil.Request("POST", "/api/auth/impersonate", body), as)
		r.Header.Set("User-Agent", "Mozilla/5.0")
		rec, _ := testutil.Do(t, mux, r)
		return rec.Code, rec.Body.String(), setCookies(rec)
	}

	// Every guard answers without creating a session or a cookie.
	for _, c := range []struct {
		as     testutil.Staff
		body   any
		status int
		want   string
	}{
		{plain, map[string]any{"user_id": target.UserID}, 403, `{"success":false,"error":"Insufficient permissions"}`},
		{admin, map[string]any{"user_id": "nope"}, 400, `{"success":false,"error":"Validation failed","details":[{"code":"invalid_format","path":["user_id"],"message":"Invalid UUID"}]}`},
		{admin, map[string]any{"user_id": admin.UserID}, 400, `{"success":false,"error":"You are already logged in as this account"}`},
		{admin, map[string]any{"user_id": "7b0c1d2e-3f40-4a5b-8c6d-7e8f90a1b2c3"}, 404, `{"success":false,"error":"User account not found"}`},
		{admin, map[string]any{"user_id": banned.UserID}, 400, `{"success":false,"error":"User account is banned"}`},
		{admin, map[string]any{"user_id": superAdmin.UserID}, 403, `{"success":false,"error":"Cannot impersonate a super admin account"}`},
	} {
		status, body, cookies := post(c.as, c.body)
		if status != c.status || body != c.want || len(cookies) != 0 {
			t.Errorf("%v: %d %s %v", c.body, status, body, cookies)
		}
	}
	if n := count(t, deps, `SELECT count(*) FROM auth.sessions WHERE user_agent LIKE 'impersonated-by:%' AND user_id = ANY($1)`,
		[]string{target.UserID, banned.UserID, superAdmin.UserID, admin.UserID}); n != 0 {
		t.Fatalf("denied impersonations created %d sessions", n)
	}

	status, body, cookies := post(admin, map[string]any{"user_id": target.UserID})
	want := `{"success":true,"data":{"id":"` + target.UserID + `","email":"` + target.Email + `","role":"pos","full_name":"Kasir Satu"},"message":"Logged in as Kasir Satu"}`
	if status != 200 || body != want {
		t.Fatalf("impersonate: %d\n got %s\nwant %s", status, body, want)
	}
	if len(cookies) != 2 || !strings.HasPrefix(cookies[0], "nuhabit_session=") || !strings.HasPrefix(cookies[1], "arkiv_session=;") {
		t.Fatalf("cookies: %v", cookies)
	}
	token := strings.TrimPrefix(strings.Split(cookies[0], ";")[0], "nuhabit_session=")
	if n := count(t, deps, `SELECT count(*) FROM auth.sessions WHERE token_hash = $1 AND user_id = $2 AND user_agent = $3`,
		auth.HashToken(token), target.UserID, "impersonated-by:"+admin.UserID+" Mozilla/5.0"); n != 1 {
		t.Fatal("impersonation session missing its impersonated-by marker")
	}
}

func TestScope(t *testing.T) {
	_, mux := newModule(t)
	rec, _ := testutil.Do(t, mux, testutil.Request("GET", "/api/auth/scope", nil))
	if rec.Code != 401 || rec.Body.String() != `{"success":false,"message":"Not authenticated"}` {
		t.Fatalf("anonymous: %d %s", rec.Code, rec.Body.String())
	}
	staff := testutil.CreateStaff(t, testutil.StaffOptions{Role: "super_admin"})
	rec, _ = testutil.Do(t, mux, testutil.AsStaff(testutil.Request("GET", "/api/auth/scope", nil), staff))
	want := `{"success":true,"data":{"role":"super_admin","business_scope":null,"branch_id":null,"company_id":null,"holding_id":null,"is_unscoped":true}}`
	if rec.Code != 200 || rec.Body.String() != want {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

// TestImpersonateSystemAccounts: Login As into an account holding a system
// role (super_admin/admin, as app role or IAM role) is for IAM super_admin
// only, and never from inside a Login As session (temuan EPIC-061).
func TestImpersonateSystemAccounts(t *testing.T) {
	testutil.RequireSeededIAM(t, testutil.IAMBaseline{GrantedRoles: []string{"super_admin"}, Menus: []string{"settings.users"}})
	deps, mux := newModule(t)
	const sysMsg = `{"success":false,"error":"Login As ke akun Super Admin/Admin hanya untuk Super Admin"}`
	hrd := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{"settings.users": nil}})
	superCaller := testutil.CreateStaff(t, testutil.StaffOptions{Role: "super_admin"})
	adminTarget := testutil.CreateStaff(t, testutil.StaffOptions{Role: "admin"})
	iamSuper := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd"})
	exec(t, deps, `INSERT INTO iam.user_roles (user_id, role_id, is_primary) SELECT $1, id, true FROM iam.roles WHERE code = 'super_admin'`, iamSuper.UserID)
	employee := testutil.CreateStaff(t, testutil.StaffOptions{Role: "pos", FullName: "Karyawan Biasa"})

	post := func(as testutil.Staff, target string) (int, string) {
		rec, _ := testutil.Do(t, mux, testutil.AsStaff(testutil.Request("POST", "/api/auth/impersonate", map[string]any{"user_id": target}), as))
		return rec.Code, rec.Body.String()
	}
	for _, target := range []testutil.Staff{adminTarget, iamSuper} {
		if status, body := post(hrd, target.UserID); status != 403 || body != sysMsg {
			t.Errorf("hrd → %s: %d %s", target.Role, status, body)
		}
	}
	if n := count(t, deps, `SELECT count(*) FROM auth.sessions WHERE user_agent LIKE 'impersonated-by:%' AND user_id = ANY($1)`,
		[]string{adminTarget.UserID, iamSuper.UserID}); n != 0 {
		t.Fatalf("refused Login As created %d sessions", n)
	}
	if status, body := post(hrd, employee.UserID); status != 200 {
		t.Fatalf("hrd → employee: %d %s", status, body)
	}
	if status, body := post(superCaller, adminTarget.UserID); status != 200 {
		t.Fatalf("super_admin → admin: %d %s", status, body)
	}

	// Inside a Login As session no further Login As.
	exec(t, deps, `UPDATE auth.sessions SET user_agent = 'impersonated-by:' || $2 || ' Mozilla/5.0' WHERE token_hash = $1`,
		auth.HashToken(superCaller.Token), superCaller.UserID)
	status, body := post(superCaller, employee.UserID)
	if status != 403 || body != `{"success":false,"error":"Tidak bisa Login As dari sesi Login As; keluar lalu masuk dengan akun sendiri"}` {
		t.Fatalf("chained Login As: %d %s", status, body)
	}
}

// TestImpersonateRefusesAPIToken: Login As only from a dashboard session;
// an Open API token can never be turned into a session.
func TestImpersonateRefusesAPIToken(t *testing.T) {
	deps, mux := newModule(t)
	admin := testutil.CreateStaff(t, testutil.StaffOptions{Role: "admin", Menus: map[string][]string{"settings.users": nil}})
	target := testutil.CreateStaff(t, testutil.StaffOptions{Role: "pos"})
	token := "nh_" + testutil.RandomHex(32)
	exec(t, deps, `INSERT INTO configuration.api_tokens (name, token_hash, token_prefix, user_id, scopes)
		VALUES ('go-test', $1, $2, $3, '{*}')`, auth.HashToken(token), token[:15], admin.UserID)
	t.Cleanup(func() {
		exec(t, deps, `DELETE FROM configuration.api_tokens WHERE token_hash = $1`, auth.HashToken(token))
	})
	r := testutil.Request("POST", "/api/auth/impersonate", map[string]any{"user_id": target.UserID})
	r.Header.Set("Authorization", "Bearer "+token)
	rec, _ := testutil.Do(t, mux, r)
	if rec.Code != 403 || rec.Body.String() != `{"success":false,"error":"Login As hanya dari sesi login biasa"}` || len(setCookies(rec)) != 0 {
		t.Fatalf("impersonate with token: %d %s", rec.Code, rec.Body.String())
	}
	if n := count(t, deps, `SELECT count(*) FROM auth.sessions WHERE user_id = $1`, target.UserID); n != 1 {
		t.Fatalf("token Login As created sessions: %d", n-1)
	}
}

package accounts_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"nuhabit/backend/internal/app"
	"nuhabit/backend/internal/modules/configuration/accounts"
	"nuhabit/backend/internal/platform/testutil"
)

const systemRoleMsg = "Role Super Admin/Admin hanya bisa diatur oleh Super Admin"

// setupStaff mounts the routes for an existing caller; targets the test
// creates before this call are cleaned up after the rollback.
func setupStaff(t *testing.T, staff testutil.Staff) *env {
	t.Helper()
	tx := testutil.Tx(t)
	deps := testutil.Deps(t, nil)
	mux := http.NewServeMux()
	for _, r := range accounts.Routes(deps, tx, app.ConfigurationPorts(deps).Accounts) {
		mux.Handle(r.Pattern, r.Handler)
	}
	return &env{t: t, staff: staff, tx: tx, mux: mux}
}

// linkEmployee gives userID an employee record with app access.
func (e *env) linkEmployee(userID string) string {
	e.t.Helper()
	var id string
	if err := e.tx.QueryRow(context.Background(), `INSERT INTO hris.employees (full_name, nip, email, phone, join_date, user_id, is_access_app)
		VALUES ('Akun Sistem', $1, $2, '08', '2026-01-01', $3, true) RETURNING id::text`,
		"SYS-"+testutil.RandomHex(4), uniqueEmail(), userID).Scan(&id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

// account is the login account's state the refused calls must not change.
func (e *env) account(userID string) string {
	e.t.Helper()
	var s string
	if err := e.tx.QueryRow(context.Background(), `SELECT u.role || '|' || u.status || '|' || u.full_name || '|' || u.email || '|' ||
		  (a.banned_until IS NULL)::text || '|' || a.password_hash || '|' ||
		  coalesce((SELECT string_agg(r.code, ',' ORDER BY r.code) FROM iam.user_roles ur JOIN iam.roles r ON r.id = ur.role_id
		    WHERE ur.user_id = u.id), '') || '|' ||
		  (SELECT count(*) FROM iam.permission_audit_logs WHERE target_user_id = u.id)::text
		FROM configuration.users u JOIN auth.users a ON a.id = u.id WHERE u.id = $1`, userID).Scan(&s); err != nil {
		e.t.Fatal(err)
	}
	return s
}

func (e *env) holding() string {
	e.t.Helper()
	var id string
	if err := e.tx.QueryRow(context.Background(), `INSERT INTO configuration.holdings (name, code) VALUES ('Go Sys', $1) RETURNING id::text`,
		"GS"+testutil.RandomHex(4)).Scan(&id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

// TestSystemRoleNeedsSuperAdmin: a Manajemen User holder without IAM
// super_admin may neither grant nor revoke super_admin/admin, nor edit,
// deactivate or reset the password of an account holding one; a refusal
// writes nothing.
func TestSystemRoleNeedsSuperAdmin(t *testing.T) {
	superTarget := testutil.CreateStaff(t, testutil.StaffOptions{Role: "super_admin"})
	adminTarget := testutil.CreateStaff(t, testutil.StaffOptions{Role: "admin"})
	plainTarget := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd"})
	resignedSuper := testutil.CreateStaff(t, testutil.StaffOptions{Role: "super_admin"})
	hrd := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{"settings.users": nil}})
	e := setupStaff(t, hrd)
	ctx := context.Background()
	holding := e.holding()
	scoped := func(m map[string]any) map[string]any {
		m["business_scope"], m["holding_id"] = "holding", holding
		return m
	}

	// Granting on create: /api/users and /api/admin/users.
	for _, role := range []string{"super_admin", "admin"} {
		email := uniqueEmail()
		b := scoped(employeeBody(email))
		b["is_access_app"], b["password"], b["role"] = true, "rahasia123", role
		code, out, _ := e.do("POST", "/api/users", b)
		wantError(t, code, out, 403, systemRoleMsg)
		code, out, _ = e.do("POST", "/api/admin/users", map[string]any{
			"email": email, "password": "rahasia123", "full_name": "Admin Baru", "role": role})
		wantError(t, code, out, 403, systemRoleMsg)
		if n := e.scalar(`SELECT (SELECT count(*) FROM hris.employees WHERE email = $1) + (SELECT count(*) FROM auth.users WHERE email = $1)`, email); n != int64(0) {
			t.Fatalf("refused create (%s) wrote %v rows", role, n)
		}
	}

	// Granting on update: promote a plain account.
	plainEmp := e.linkEmployee(plainTarget.UserID)
	before := e.account(plainTarget.UserID)
	code, out, _ := e.do("PUT", "/api/users/"+plainEmp, map[string]any{"is_access_app": true, "role": "super_admin", "phone": "0899"})
	wantError(t, code, out, 403, systemRoleMsg)
	code, out, _ = e.do("PATCH", "/api/admin/users/"+plainTarget.UserID, map[string]any{"role": "admin"})
	wantError(t, code, out, 403, systemRoleMsg)
	if got := e.account(plainTarget.UserID); got != before {
		t.Fatalf("refused promotion changed account: %s → %s", before, got)
	}
	if phone := e.scalar(`SELECT phone FROM hris.employees WHERE id = $1`, plainEmp); phone != "08" {
		t.Fatalf("refused promotion patched the employee: phone %v", phone)
	}

	// Editing, demoting, deactivating, revoking or resetting a system account.
	for _, target := range []testutil.Staff{superTarget, adminTarget} {
		emp := e.linkEmployee(target.UserID)
		before := e.account(target.UserID)
		for _, c := range []struct {
			method, path string
			body         map[string]any
		}{
			{"PUT", "/api/users/" + emp, scoped(map[string]any{"is_access_app": true, "role": "hrd"})},
			{"PUT", "/api/users/" + emp, map[string]any{"account_status": "inactive"}},
			{"PUT", "/api/users/" + emp, map[string]any{"is_access_app": false}},
			{"PUT", "/api/users/" + emp, map[string]any{"email": uniqueEmail()}},
			{"PUT", "/api/users/" + emp, map[string]any{"is_access_app": true, "password": "baru12345"}},
			{"POST", "/api/users/" + emp + "/reset-password", nil},
			{"PATCH", "/api/admin/users/" + target.UserID, map[string]any{"role": "hrd"}},
			{"PATCH", "/api/admin/users/" + target.UserID, map[string]any{"status": "inactive"}},
			{"PATCH", "/api/admin/users/" + target.UserID, map[string]any{"full_name": "Nama Lain"}},
			{"POST", "/api/admin/users/" + target.UserID + "/reset-password", nil},
		} {
			code, out, raw := e.do(c.method, c.path, c.body)
			if code != 403 || out["error"] != systemRoleMsg {
				t.Fatalf("%s %s %v on %s = %d %s, want 403", c.method, c.path, c.body, target.Role, code, raw)
			}
		}
		if got := e.account(target.UserID); got != before {
			t.Fatalf("refused edits changed the %s account: %s → %s", target.Role, before, got)
		}
		if n := e.scalar(`SELECT count(*) FROM hris.employees WHERE id = $1 AND is_access_app AND email LIKE 'go-acc-%'`, emp); n != int64(1) {
			t.Fatalf("refused edits changed the %s employee", target.Role)
		}
		// Employee data that never reaches the login stays editable.
		if code, _, raw := e.do("PUT", "/api/users/"+emp, map[string]any{"phone": "0812"}); code != 200 {
			t.Fatalf("phone-only edit on %s = %d %s", target.Role, code, raw)
		}
	}

	// Releasing a resigned employee's email reactivates their login: refused for a system account.
	released := uniqueEmail()
	if _, err := e.tx.Exec(ctx, `INSERT INTO hris.employees (full_name, nip, email, phone, join_date, user_id, is_access_app, is_active)
		VALUES ('Sudah Resign', $1, $2, '08', '2025-01-01', $3, false, false)`, "RSG-"+testutil.RandomHex(4), released, resignedSuper.UserID); err != nil {
		t.Fatal(err)
	}
	beforeResigned := e.account(resignedSuper.UserID)
	rb := employeeBody(released)
	rb["release_email"] = true
	code, out, _ = e.do("POST", "/api/users", rb)
	wantError(t, code, out, 403, systemRoleMsg)
	if n := e.scalar(`SELECT count(*) FROM hris.employees WHERE email = $1`, released); n != int64(1) {
		t.Fatalf("refused release wrote employees: %v", n)
	}
	if got := e.account(resignedSuper.UserID); got != beforeResigned {
		t.Fatalf("refused release changed the account: %s → %s", beforeResigned, got)
	}

	// A system role held through IAM only (app role hrd) counts too.
	if _, err := e.tx.Exec(ctx, `INSERT INTO iam.roles (code, name, is_system) VALUES ('super_admin', 'Super Admin', true) ON CONFLICT (code) DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.tx.Exec(ctx, `INSERT INTO iam.user_roles (user_id, role_id, is_primary)
		SELECT $1, id, true FROM iam.roles WHERE code = 'super_admin'`, plainTarget.UserID); err != nil {
		t.Fatal(err)
	}
	code, out, _ = e.do("PATCH", "/api/admin/users/"+plainTarget.UserID, map[string]any{"full_name": "Nama Lain"})
	wantError(t, code, out, 403, systemRoleMsg)
}

// TestAdminUserRoleChangeSyncsIam: PATCH /api/admin/users/{id} moves the IAM
// role with the app role and records it, as Manajemen User does.
func TestAdminUserRoleChangeSyncsIam(t *testing.T) {
	target := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd"})
	hrd := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{"settings.users": nil}})
	e := setupStaff(t, hrd)
	ctx := context.Background()
	if _, err := e.tx.Exec(ctx, `INSERT INTO iam.roles (code, name) VALUES ('hrd', 'HRD'), ('finance_staff', 'Finance Staff') ON CONFLICT (code) DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	if code, _, raw := e.do("PATCH", "/api/admin/users/"+target.UserID, map[string]any{"role": "finance_staff"}); code != 200 {
		t.Fatalf("role change = %d %s", code, raw)
	}
	if got := e.scalar(`SELECT coalesce(string_agg(r.code, ','), '') FROM iam.user_roles ur JOIN iam.roles r ON r.id = ur.role_id
		WHERE ur.user_id = $1 AND ur.is_primary AND ur.assigned_by = $2`, target.UserID, hrd.UserID); got != "finance_staff" {
		t.Fatalf("iam roles = %v, want finance_staff", got)
	}
	var trail string
	if err := e.tx.QueryRow(ctx, `SELECT coalesce(string_agg(array_to_string(roles_before, '+') || '>' || array_to_string(roles_after, '+'), ','), '')
		FROM iam.permission_audit_logs WHERE source = 'user_role' AND target_user_id = $1 AND actor_id = $2`, target.UserID, hrd.UserID).Scan(&trail); err != nil {
		t.Fatal(err)
	}
	if trail != ">finance_staff" {
		t.Fatalf("user role trail = %q", trail)
	}
	// Resending the same role leaves IAM and the trail alone.
	if code, _, raw := e.do("PATCH", "/api/admin/users/"+target.UserID, map[string]any{"role": "finance_staff", "full_name": "Nama Baru"}); code != 200 {
		t.Fatalf("same role = %d %s", code, raw)
	}
	if n := e.scalar(`SELECT count(*) FROM iam.permission_audit_logs WHERE target_user_id = $1`, target.UserID); n != int64(1) {
		t.Fatalf("audit rows after same-role save = %v", n)
	}
}

// TestSuperAdminSetsSystemRoles: IAM super_admin may grant, change and
// revoke the system roles, and the IAM role follows.
func TestSuperAdminSetsSystemRoles(t *testing.T) {
	testutil.RequireSeededIAM(t, testutil.IAMBaseline{GrantedRoles: []string{"super_admin"}, Roles: []string{"admin", "hrd"}})
	superTarget := testutil.CreateStaff(t, testutil.StaffOptions{Role: "super_admin"})
	adminTarget := testutil.CreateStaff(t, testutil.StaffOptions{Role: "admin"})
	caller := testutil.CreateStaff(t, testutil.StaffOptions{Role: "super_admin"})
	e := setupStaff(t, caller)
	holding := e.holding()
	iamRoles := func(userID string) any {
		return e.scalar(`SELECT coalesce(string_agg(r.code, ','), '') FROM iam.user_roles ur JOIN iam.roles r ON r.id = ur.role_id WHERE ur.user_id = $1`, userID)
	}

	b := employeeBody(uniqueEmail())
	b["is_access_app"], b["password"], b["role"] = true, "rahasia123", "super_admin"
	if code, _, raw := e.do("POST", "/api/users", b); code != 201 {
		t.Fatalf("create super_admin = %d %s", code, raw)
	}
	if code, _, raw := e.do("POST", "/api/admin/users", map[string]any{
		"email": uniqueEmail(), "password": "rahasia123", "full_name": "Admin Baru", "role": "admin"}); code != 201 {
		t.Fatalf("create admin = %d %s", code, raw)
	}
	if code, _, raw := e.do("PATCH", "/api/admin/users/"+superTarget.UserID, map[string]any{"role": "admin"}); code != 200 {
		t.Fatalf("super_admin → admin = %d %s", code, raw)
	}
	if got := iamRoles(superTarget.UserID); got != "admin" {
		t.Fatalf("iam roles after PATCH = %v", got)
	}
	if n := e.scalar(`SELECT count(*) FROM iam.permission_audit_logs WHERE source = 'user_role' AND target_user_id = $1 AND roles_after = '{admin}'`, superTarget.UserID); n != int64(1) {
		t.Fatalf("PATCH audit rows = %v", n)
	}
	emp := e.linkEmployee(adminTarget.UserID)
	if code, _, raw := e.do("PUT", "/api/users/"+emp, map[string]any{
		"is_access_app": true, "role": "hrd", "business_scope": "holding", "holding_id": holding}); code != 200 {
		t.Fatalf("admin → hrd = %d %s", code, raw)
	}
	if got := iamRoles(adminTarget.UserID); got != "hrd" {
		t.Fatalf("iam roles after PUT = %v", got)
	}
	if code, _, raw := e.do("POST", "/api/users/"+emp+"/reset-password", nil); code != 200 {
		t.Fatalf("reset = %d %s", code, raw)
	}
}

// TestAdminUserCreateSyncsIam: POST /api/admin/users gives the new account
// its IAM role and records it, in the creating transaction.
func TestAdminUserCreateSyncsIam(t *testing.T) {
	hrd := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{"settings.users": nil}})
	e := setupStaff(t, hrd)
	if _, err := e.tx.Exec(context.Background(), `INSERT INTO iam.roles (code, name) VALUES ('finance_staff', 'Finance Staff') ON CONFLICT (code) DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	email := uniqueEmail()
	if code, _, raw := e.do("POST", "/api/admin/users", map[string]any{
		"email": email, "password": "rahasia123", "full_name": "Staf Baru", "role": "finance_staff"}); code != 201 {
		t.Fatalf("create = %d %s", code, raw)
	}
	if got := e.scalar(`SELECT coalesce(string_agg(r.code, ','), '') FROM iam.user_roles ur JOIN iam.roles r ON r.id = ur.role_id
		JOIN configuration.users u ON u.id = ur.user_id
		WHERE u.email = $1 AND ur.is_primary AND ur.assigned_by = $2`, email, hrd.UserID); got != "finance_staff" {
		t.Fatalf("iam roles = %v, want finance_staff", got)
	}
	if got := e.scalar(`SELECT coalesce(string_agg(array_to_string(roles_before, '+') || '>' || array_to_string(roles_after, '+'), ','), '')
		FROM iam.permission_audit_logs l JOIN configuration.users u ON u.id = l.target_user_id
		WHERE l.source = 'user_role' AND u.email = $1 AND l.actor_id = $2`, email, hrd.UserID); got != ">finance_staff" {
		t.Fatalf("user role trail = %v", got)
	}
}

// TestSystemAccountIDForms: the path id of /api/admin/users is canonicalised
// as a uuid before the guards, so upper case or unhyphenated forms of a
// system account's id are refused like the canonical one; a malformed id
// is a 400 before anything runs (temuan re-review EPIC-061).
func TestSystemAccountIDForms(t *testing.T) {
	superTarget := testutil.CreateStaff(t, testutil.StaffOptions{Role: "super_admin"})
	hrd := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{"settings.users": nil}})
	e := setupStaff(t, hrd)
	before := e.account(superTarget.UserID)
	for _, id := range []string{strings.ToUpper(superTarget.UserID), strings.ReplaceAll(superTarget.UserID, "-", "")} {
		code, out, _ := e.do("PATCH", "/api/admin/users/"+id, map[string]any{"role": "hrd"})
		wantError(t, code, out, 403, systemRoleMsg)
		code, out, _ = e.do("POST", "/api/admin/users/"+id+"/reset-password", nil)
		wantError(t, code, out, 403, systemRoleMsg)
	}
	if got := e.account(superTarget.UserID); got != before {
		t.Fatalf("refused id forms changed the account: %s → %s", before, got)
	}
	for _, c := range []struct{ method, path string }{
		{"PATCH", "/api/admin/users/nope"}, {"POST", "/api/admin/users/nope/reset-password"},
	} {
		code, out, _ := e.do(c.method, c.path, map[string]any{"full_name": "Nama Lain"})
		wantError(t, code, out, 400, "Invalid id")
	}
}

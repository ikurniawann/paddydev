package accounts_test

import (
	"context"
	"net/http"
	"testing"

	"nuhabit/backend/internal/app"
	"nuhabit/backend/internal/modules/configuration/accounts"
	"nuhabit/backend/internal/platform/testutil"
)

// setupAs is setup with explicit staff options (super_admin resolves its
// IAM role by app role, so it has no Menus).
func setupAs(t *testing.T, opts testutil.StaffOptions) *env {
	t.Helper()
	staff := testutil.CreateStaff(t, opts)
	tx := testutil.Tx(t)
	deps := testutil.Deps(t, nil)
	mux := http.NewServeMux()
	for _, r := range accounts.Routes(deps, tx, app.ConfigurationPorts(deps).Accounts) {
		mux.Handle(r.Pattern, r.Handler)
	}
	return &env{t: t, staff: staff, tx: tx, mux: mux}
}

func TestHrisRoleInManajemenUser(t *testing.T) {
	requireHrisAccounts(t)
	admin := testutil.CreateStaff(t, testutil.StaffOptions{Role: "admin", Menus: map[string][]string{"settings.users": nil}})
	e := setupAs(t, testutil.StaffOptions{Role: "super_admin"})
	ctx := context.Background()
	var holding string
	if err := e.tx.QueryRow(ctx, `SELECT id::text FROM configuration.holdings ORDER BY created_at LIMIT 1`).Scan(&holding); err != nil {
		t.Fatal(err)
	}
	scoped := func(m map[string]any) map[string]any {
		m["business_scope"], m["holding_id"] = "holding", holding
		return m
	}
	body := scoped(employeeBody(uniqueEmail()))
	body["is_access_app"], body["password"], body["role"], body["hris_role"] = true, "rahasia123", "hrd", "hris_non_payroll"
	code, created, raw := e.do("POST", "/api/users", body)
	if code != 201 {
		t.Fatalf("create = %d %s", code, raw)
	}
	id := created["data"].(map[string]any)["id"].(string)
	var userID string
	if err := e.tx.QueryRow(ctx, `SELECT user_id::text FROM hris.employees WHERE id = $1`, id).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	roles := func() string {
		var r string
		if err := e.tx.QueryRow(ctx, `SELECT coalesce(string_agg(r.code, ',' ORDER BY r.code), '') FROM iam.user_roles ur JOIN iam.roles r ON r.id = ur.role_id WHERE ur.user_id = $1`, userID).Scan(&r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	if got := roles(); got != "hris_non_payroll" {
		t.Fatalf("after create roles = %s", got)
	}
	_, got, _ := e.do("GET", "/api/users/"+id, nil)
	if acc := got["data"].(map[string]any)["appAccount"].(map[string]any); acc["hrisRole"] != "hris_non_payroll" {
		t.Fatalf("appAccount.hrisRole = %v", acc["hrisRole"])
	}
	// Re-save with the same app role and no hris_role keeps the IAM role.
	if code, _, raw := e.do("PUT", "/api/users/"+id, scoped(map[string]any{"is_access_app": true, "role": "hrd", "phone": "0812"})); code != 200 {
		t.Fatalf("resave = %d %s", code, raw)
	}
	if got := roles(); got != "hris_non_payroll" {
		t.Fatalf("resave changed roles to %s", got)
	}
	// A settings.users holder that is not super_admin may not set it.
	adminEnv := &env{t: t, staff: admin, tx: e.tx, mux: e.mux}
	code, body2, _ := adminEnv.do("PUT", "/api/users/"+id, scoped(map[string]any{"is_access_app": true, "hris_role": "hris_manager"}))
	wantError(t, code, body2, 403, "Role HRIS hanya bisa diatur Super Admin di sini; AdminHRIS memakai halaman Hak Akses HRIS")
	// Only app role hrd.
	code, body2, _ = e.do("PUT", "/api/users/"+id, scoped(map[string]any{"is_access_app": true, "role": "finance_staff", "hris_role": "hris_manager"}))
	wantError(t, code, body2, 400, "Role HRIS hanya untuk akun dengan peran aplikasi hrd")
	// null goes back to the app role.
	if code, _, raw := e.do("PUT", "/api/users/"+id, scoped(map[string]any{"is_access_app": true, "hris_role": nil})); code != 200 {
		t.Fatalf("reset = %d %s", code, raw)
	}
	if got := roles(); got != "hrd" {
		t.Fatalf("reset roles = %s, want hrd", got)
	}
}

// Ruling M3: a refused hris_role (400/403) leaves the employee and the
// account exactly as they were, on create and on update.
func TestHrisRoleRejectedWritesNothing(t *testing.T) {
	requireHrisAccounts(t)
	admin := testutil.CreateStaff(t, testutil.StaffOptions{Role: "admin", Menus: map[string][]string{"settings.users": nil}})
	e := setupAs(t, testutil.StaffOptions{Role: "super_admin"})
	adminEnv := &env{t: t, staff: admin, tx: e.tx, mux: e.mux}
	ctx := context.Background()
	var holding string
	if err := e.tx.QueryRow(ctx, `SELECT id::text FROM configuration.holdings ORDER BY created_at LIMIT 1`).Scan(&holding); err != nil {
		t.Fatal(err)
	}
	scoped := func(m map[string]any) map[string]any {
		m["business_scope"], m["holding_id"] = "holding", holding
		return m
	}
	account := func(email, role string) map[string]any {
		b := scoped(employeeBody(email))
		b["is_access_app"], b["password"], b["role"] = true, "rahasia123", role
		return b
	}
	employees := func(email string) int64 {
		return e.scalar(`SELECT count(*) FROM hris.employees WHERE email = $1`, email).(int64)
	}

	// Create: refused before the employee row is inserted.
	email := uniqueEmail()
	body := account(email, "finance_staff")
	body["hris_role"] = "hris_manager"
	code, out, _ := e.do("POST", "/api/users", body)
	wantError(t, code, out, 400, "Role HRIS hanya untuk akun dengan peran aplikasi hrd")
	body = account(email, "hrd")
	body["hris_role"] = "hris_manager"
	code, out, _ = adminEnv.do("POST", "/api/users", body)
	wantError(t, code, out, 403, "Role HRIS hanya bisa diatur Super Admin di sini; AdminHRIS memakai halaman Hak Akses HRIS")
	if n := employees(email); n != 0 {
		t.Fatalf("refused create left %d employee rows", n)
	}

	// Update: refused before the employee patch and the account sync.
	code, created, raw := e.do("POST", "/api/users", account(email, "hrd"))
	if code != 201 {
		t.Fatalf("create = %d %s", code, raw)
	}
	id := created["data"].(map[string]any)["id"].(string)
	state := func() string {
		var s string
		if err := e.tx.QueryRow(ctx, `SELECT e.phone || '|' || e.full_name || '|' || u.role || '|' ||
			  coalesce((SELECT string_agg(r.code, ',' ORDER BY r.code) FROM iam.user_roles ur JOIN iam.roles r ON r.id = ur.role_id
			    WHERE ur.user_id = u.id), '')
			FROM hris.employees e JOIN configuration.users u ON u.id = e.user_id WHERE e.id = $1`, id).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	before := state()
	code, out, _ = e.do("PUT", "/api/users/"+id, scoped(map[string]any{
		"is_access_app": true, "phone": "0899", "full_name": "Nama Baru", "role": "finance_staff", "hris_role": "hris_manager"}))
	wantError(t, code, out, 400, "Role HRIS hanya untuk akun dengan peran aplikasi hrd")
	if got := state(); got != before {
		t.Fatalf("after refused 400 update state = %s, want %s", got, before)
	}
	code, out, _ = adminEnv.do("PUT", "/api/users/"+id, scoped(map[string]any{
		"is_access_app": true, "phone": "0899", "full_name": "Nama Baru", "hris_role": "hris_manager"}))
	wantError(t, code, out, 403, "Role HRIS hanya bisa diatur Super Admin di sini; AdminHRIS memakai halaman Hak Akses HRIS")
	if got := state(); got != before {
		t.Fatalf("after refused 403 update state = %s, want %s", got, before)
	}
}

// requireHrisAccounts skips a test that needs the seeded app roles
// (hrd, finance_staff, the hris_* roles) and super_admin's grants; CI
// applies migrations without the seeders.
func requireHrisAccounts(t *testing.T) {
	t.Helper()
	testutil.RequireSeededIAM(t, testutil.IAMBaseline{
		GrantedRoles: []string{"super_admin"},
		Roles:        []string{"hrd", "finance_staff", "hris_manager", "hris_non_payroll"},
	})
}

package access_test

import (
	"slices"
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

const (
	errRoleScope    = `{"success":false,"error":"Hak Akses HRIS hanya untuk role admin_hris dan hris_*"}`
	errMenuScope    = `{"success":false,"error":"Menu di luar cakupan Hak Akses HRIS"}`
	errAccountScope = `{"success":false,"error":"Akun ini bukan akun HR (peran aplikasi hrd)"}`
)

func change(menu string, access, del *bool) map[string]any {
	c := map[string]any{"menu_code": menu}
	if access != nil {
		c["access"] = *access
	}
	if del != nil {
		c["delete"] = *del
	}
	return map[string]any{"changes": []any{c}}
}

func ptr(b bool) *bool { return &b }

func hrisManager(t *testing.T) testutil.Staff {
	return testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{"hris.access": {"read", "update"}}})
}

func TestHrisAccessScope(t *testing.T) {
	requireHrisIAM(t)
	manager := hrisManager(t)
	reader := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{"hris.access": {"read"}}})
	outsider := testutil.CreateStaff(t, testutil.StaffOptions{Menus: map[string][]string{"settings.menus": nil}})
	mux, _ := setup(t)

	expect(t, do(t, mux, nil, "GET", "/api/hris/access", nil), 401, "")
	expect(t, do(t, mux, &outsider, "GET", "/api/hris/access", nil), 403, "")
	expect(t, do(t, mux, &reader, "PUT", "/api/hris/access/roles/hris_non_payroll", change("hris.workforce.attendance", nil, ptr(true))), 403, "")
	for _, code := range []string{"super_admin", "admin", "hrd", "pos"} {
		expect(t, do(t, mux, &manager, "PUT", "/api/hris/access/roles/"+code, change("hris.workforce.attendance", ptr(true), nil)), 403, errRoleScope)
	}
	for _, menu := range []string{"settings.roles", "hris.access", "hris.kepegawaian.users", "pos"} {
		expect(t, do(t, mux, &manager, "PUT", "/api/hris/access/roles/hris_non_payroll", change(menu, ptr(true), nil)), 403, errMenuScope)
	}
	expect(t, do(t, mux, &manager, "PUT", "/api/hris/access/roles/hris_non_payroll", change("hris.kepegawaian.loans", nil, ptr(true))), 403, errMenuScope)
	expect(t, do(t, mux, &manager, "PUT", "/api/hris/access/roles/hris_non_payroll", map[string]any{"changes": []any{}}), 400,
		`{"success":false,"error":"Tidak ada perubahan"}`)

	r := do(t, mux, &manager, "GET", "/api/hris/access", nil)
	expect(t, r, 200, "")
	data := r.Body["data"].(map[string]any)
	var roles []string
	for _, x := range data["roles"].([]any) {
		roles = append(roles, x.(map[string]any)["code"].(string))
	}
	if !slices.Equal(roles, []string{"admin_hris", "hris_manager", "hris_non_payroll", "hris_non_rekrutmen_kpi"}) {
		t.Fatalf("roles = %v", roles)
	}
	menus := map[string]map[string]any{}
	for _, x := range data["menus"].([]any) {
		m := x.(map[string]any)
		menus[m["code"].(string)] = m
	}
	for code, want := range map[string][2]bool{
		"hris.workforce.attendance": {true, true}, "hris.kepegawaian.holidays": {false, true}, "hris.compensation": {true, false},
	} {
		m := menus[code]
		if m == nil || m["access_toggle"] != want[0] || m["delete_toggle"] != want[1] {
			t.Errorf("menu %s = %v, want access %v delete %v", code, m, want[0], want[1])
		}
	}
	for _, code := range []string{"hris.access", "hris.kepegawaian.loans", "settings.roles"} {
		if menus[code] != nil {
			t.Errorf("menu %s must not be listed", code)
		}
	}
}

func TestHrisAccessMasksSurviveSync(t *testing.T) {
	requireHrisIAM(t)
	manager := hrisManager(t)
	mux, text := setup(t)
	grant := func(role, menu string) string {
		return text(`SELECT coalesce((SELECT p.granted_actions::text FROM iam.role_menu_permissions p
			JOIN iam.roles r ON r.id = p.role_id JOIN iam.menus m ON m.id = p.menu_id
			WHERE r.code = $1 AND m.code = $2 AND p.is_active), 'none')`, role, menu)
	}
	// Non-Payroll gets delete on Absensi; it survives a later sync.
	expect(t, do(t, mux, &manager, "PUT", "/api/hris/access/roles/hris_non_payroll", change("hris.workforce.attendance", nil, ptr(true))), 200, "")
	text(`SELECT 'ok' FROM (SELECT iam.sync_hris_staff_roles()) s`)
	if got := grant("hris_non_payroll", "hris.workforce.attendance"); !strings.Contains(got, `"delete"`) {
		t.Fatalf("non-payroll attendance = %s, want delete after re-sync", got)
	}
	if got := grant("hris_non_rekrutmen_kpi", "hris.workforce.attendance"); strings.Contains(got, `"delete"`) {
		t.Fatalf("other staff role attendance = %s, want no delete", got)
	}
	// Turning Penggajian off for hris_manager hides the subtree; salary alone comes back with its parent.
	expect(t, do(t, mux, &manager, "PUT", "/api/hris/access/roles/hris_manager", change("hris.compensation", ptr(false), nil)), 200, "")
	if got := grant("hris_manager", "hris.compensation.payroll"); got != "none" {
		t.Fatalf("manager payroll = %s, want none", got)
	}
	expect(t, do(t, mux, &manager, "PUT", "/api/hris/access/roles/hris_manager", change("hris.compensation.salary", ptr(true), nil)), 200, "")
	if got := grant("hris_manager", "hris.compensation"); got != `["read"]` {
		t.Fatalf("manager compensation group = %s, want [\"read\"]", got)
	}
	if got := grant("hris_manager", "hris.compensation.salary"); got == "none" {
		t.Fatal("manager salary not granted back")
	}
	// Delete needs access first.
	expect(t, do(t, mux, &manager, "PUT", "/api/hris/access/roles/hris_non_payroll", change("hris.compensation.salary", nil, ptr(true))), 400,
		`{"success":false,"error":"Aktifkan akses menu dulu sebelum memberi hak hapus"}`)
	// admin_hris (the source): turning its own delete off on Absensi.
	expect(t, do(t, mux, &manager, "PUT", "/api/hris/access/roles/admin_hris", change("hris.workforce.attendance", nil, ptr(false))), 200, "")
	if got := grant("admin_hris", "hris.workforce.attendance"); strings.Contains(got, `"delete"`) {
		t.Fatalf("admin_hris attendance = %s, want no delete", got)
	}
	if got := text(`SELECT count(*)::text FROM iam.permission_audit_logs WHERE source = 'hris_access' AND role_code = 'hris_non_payroll'
		AND menu_code = 'hris.workforce.attendance' AND actions_after ? 'delete'`); got != "1" {
		t.Fatalf("hris_access audit rows = %s", got)
	}
	if got := text(`SELECT count(*)::text FROM iam.permission_audit_logs WHERE source = 'hris_mask' AND role_code = 'hris_manager'`); got == "0" {
		t.Fatal("mask changes not audited")
	}
	r := do(t, mux, &manager, "GET", "/api/hris/access/audit", nil)
	expect(t, r, 200, "")
	if rows, _ := r.Body["data"].([]any); len(rows) == 0 {
		t.Fatal("audit endpoint returned no rows")
	}
}

func TestHrisAccessAccounts(t *testing.T) {
	manager := hrisManager(t)
	hr := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd"})
	boss := testutil.CreateStaff(t, testutil.StaffOptions{Role: "super_admin"})
	fin := testutil.CreateStaff(t, testutil.StaffOptions{Role: "finance_staff"})
	mux, text := setup(t)
	expect(t, do(t, mux, &manager, "PUT", "/api/hris/access/accounts/"+hr.UserID, map[string]any{"role": "hris_non_payroll"}), 200, "")
	if got := text(`SELECT string_agg(r.code, ',') FROM iam.user_roles ur JOIN iam.roles r ON r.id = ur.role_id WHERE ur.user_id = $1`, hr.UserID); got != "hris_non_payroll" {
		t.Fatalf("roles = %s", got)
	}
	for _, s := range []testutil.Staff{boss, fin} {
		expect(t, do(t, mux, &manager, "PUT", "/api/hris/access/accounts/"+s.UserID, map[string]any{"role": "hris_manager"}), 403, errAccountScope)
	}
	expect(t, do(t, mux, &manager, "PUT", "/api/hris/access/accounts/"+hr.UserID, map[string]any{"role": "super_admin"}), 403, errRoleScope)
	if got := text(`SELECT count(*)::text FROM iam.permission_audit_logs WHERE source = 'user_role' AND target_user_id = $1`, hr.UserID); got != "1" {
		t.Fatalf("user role audit rows = %s", got)
	}
}

// Ruling M5 (Q-RA-2): turning access on never grants "delete" by itself;
// the delete reduction of the staff role is kept until an explicit toggle.
func TestHrisAccessEnableKeepsDeleteReduction(t *testing.T) {
	requireHrisIAM(t)
	manager := hrisManager(t)
	mux, text := setup(t)
	grant := func(role, menu string) string {
		return text(`SELECT coalesce((SELECT p.granted_actions::text FROM iam.role_menu_permissions p
			JOIN iam.roles r ON r.id = p.role_id JOIN iam.menus m ON m.id = p.menu_id
			WHERE r.code = $1 AND m.code = $2 AND p.is_active), 'none')`, role, menu)
	}
	put := func(menu string, access, del *bool) {
		t.Helper()
		expect(t, do(t, mux, &manager, "PUT", "/api/hris/access/roles/hris_non_payroll", change(menu, access, del)), 200, "")
	}
	// Penggajian on for Non-Payroll: the menus come back without delete.
	put("hris.compensation", ptr(true), nil)
	for _, leaf := range []string{"hris.compensation.payroll", "hris.compensation.salary"} {
		if got := grant("hris_non_payroll", leaf); got == "none" || strings.Contains(got, `"delete"`) {
			t.Fatalf("after enabling Penggajian %s = %s, want access without delete", leaf, got)
		}
	}
	// Delete comes only with the explicit toggle, on that leaf only.
	put("hris.compensation.payroll", nil, ptr(true))
	if got := grant("hris_non_payroll", "hris.compensation.payroll"); !strings.Contains(got, `"delete"`) {
		t.Fatalf("payroll after delete toggle = %s", got)
	}
	if got := grant("hris_non_payroll", "hris.compensation.salary"); strings.Contains(got, `"delete"`) {
		t.Fatalf("salary = %s, want no delete", got)
	}
	// Hiding and showing again does not bring a delete grant back.
	put("hris.compensation", ptr(false), nil)
	put("hris.compensation", ptr(true), nil)
	if got := grant("hris_non_payroll", "hris.compensation.payroll"); got == "none" || strings.Contains(got, `"delete"`) {
		t.Fatalf("payroll after hide/show = %s, want access without delete", got)
	}
	// Same on a single leaf that had delete granted on its own mask row.
	put("hris.workforce.attendance", nil, ptr(true))
	put("hris.workforce.attendance", ptr(false), nil)
	if got := grant("hris_non_payroll", "hris.workforce.attendance"); got != "none" {
		t.Fatalf("hidden attendance = %s", got)
	}
	put("hris.workforce.attendance", ptr(true), nil)
	if got := grant("hris_non_payroll", "hris.workforce.attendance"); got == "none" || strings.Contains(got, `"delete"`) {
		t.Fatalf("attendance after hide/show = %s, want access without delete", got)
	}
	// Re-sending access on a visible menu keeps an explicit delete grant.
	put("hris.compensation.payroll", nil, ptr(true))
	put("hris.compensation.payroll", ptr(true), nil)
	if got := grant("hris_non_payroll", "hris.compensation.payroll"); !strings.Contains(got, `"delete"`) {
		t.Fatalf("payroll after re-sending access = %s, want delete kept", got)
	}
}

// Turning an admin_hris group off and on again restores the grants it had
// (ruling #4); the derived roles follow after the sync.
func TestHrisAccessAdminHrisReenableRestores(t *testing.T) {
	manager := hrisManager(t)
	mux, text := setup(t)
	grant := func(role, menu string) string {
		return text(`SELECT coalesce((SELECT p.granted_actions::text FROM iam.role_menu_permissions p
			JOIN iam.roles r ON r.id = p.role_id JOIN iam.menus m ON m.id = p.menu_id
			WHERE r.code = $1 AND m.code = $2 AND p.is_active), 'none')`, role, menu)
	}
	leaves := []string{"hris.compensation.payroll", "hris.compensation.salary"}
	before := map[string]string{}
	for _, role := range []string{"admin_hris", "hris_manager"} {
		for _, leaf := range leaves {
			before[role+leaf] = grant(role, leaf)
			if before[role+leaf] == "none" {
				t.Skipf("%s holds no %s on this database", role, leaf)
			}
		}
	}
	if !strings.Contains(before["admin_hris"+leaves[0]], `"delete"`) {
		t.Skip("admin_hris has no delete on payroll on this database")
	}
	expect(t, do(t, mux, &manager, "PUT", "/api/hris/access/roles/admin_hris", change("hris.compensation", ptr(false), nil)), 200, "")
	for _, leaf := range leaves {
		if got := grant("hris_manager", leaf); got != "none" {
			t.Fatalf("hris_manager %s after off = %s", leaf, got)
		}
	}
	expect(t, do(t, mux, &manager, "PUT", "/api/hris/access/roles/admin_hris", change("hris.compensation", ptr(true), nil)), 200, "")
	for _, role := range []string{"admin_hris", "hris_manager"} {
		for _, leaf := range leaves {
			if got := grant(role, leaf); got != before[role+leaf] {
				t.Errorf("%s %s after off/on = %s, want %s", role, leaf, got, before[role+leaf])
			}
		}
	}
}

// A derived role cannot be given a menu its source admin_hris lacks.
func TestHrisAccessNeedsAdminHrisMenu(t *testing.T) {
	requireHrisIAM(t)
	manager := hrisManager(t)
	mux, text := setup(t)
	text(`UPDATE iam.role_menu_permissions p SET is_active = false FROM iam.roles r, iam.menus m
		WHERE r.id = p.role_id AND m.id = p.menu_id AND r.code = 'admin_hris' AND m.code = 'hris.recruitment.psikotes' RETURNING 'ok'`)
	expect(t, do(t, mux, &manager, "PUT", "/api/hris/access/roles/hris_manager", change("hris.recruitment.psikotes", ptr(true), nil)), 400,
		`{"success":false,"error":"Admin HRIS belum punya akses menu ini"}`)
	// Turning it off stays allowed.
	expect(t, do(t, mux, &manager, "PUT", "/api/hris/access/roles/hris_manager", change("hris.recruitment.psikotes", ptr(false), nil)), 200, "")
}

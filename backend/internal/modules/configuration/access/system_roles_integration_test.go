package access_test

import (
	"encoding/json"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

const errSystemRole = `{"success":false,"error":"Role sistem hanya bisa diubah oleh Super Admin"}`

func TestSystemRolesOnlySuperAdmin(t *testing.T) {
	admin := testutil.CreateStaff(t, testutil.StaffOptions{Menus: map[string][]string{"settings.roles": nil}})
	super := testutil.CreateStaff(t, testutil.StaffOptions{Role: "super_admin"})
	mux, text := setup(t)
	code := "go_sys_" + testutil.RandomHex(3)
	system := text(`INSERT INTO iam.roles (code, name, is_system) VALUES ($1, 'Go System', true) RETURNING id::text`, code)
	menuCode := "go_menu_" + testutil.RandomHex(3)
	menuID := text(`INSERT INTO iam.menus (code, menu_name) VALUES ($1, 'Go Menu') RETURNING id::text`, menuCode)
	perms := map[string]any{"permissions": []any{map[string]any{"menuId": menuID, "isGranted": true}}}

	expect(t, do(t, mux, &admin, "PUT", "/api/settings/iam/roles/"+system, map[string]any{"name": "Ubah"}), 403, errSystemRole)
	expect(t, do(t, mux, &admin, "PUT", "/api/settings/iam/roles/"+system+"/permissions", perms), 403, errSystemRole)
	expect(t, do(t, mux, &super, "PUT", "/api/settings/iam/roles/"+system, map[string]any{"name": "Ubah"}), 200, `{"id":"`+system+`"}`)
	expect(t, do(t, mux, &super, "PUT", "/api/settings/iam/roles/"+system, map[string]any{"code": code + "x"}), 400,
		`{"success":false,"error":"System role code cannot be changed"}`)
	expect(t, do(t, mux, &super, "PUT", "/api/settings/iam/roles/"+system+"/permissions", perms), 200, "")
	if got := text(`SELECT count(*)::text FROM iam.permission_audit_logs WHERE role_id = $1 AND source = 'role_permissions'
		AND menu_code = $2 AND actions_before IS NULL AND actions_after = '["read"]'::jsonb`, system, menuCode); got != "1" {
		t.Fatalf("permission audit rows = %s", got)
	}
}

// Saving admin_hris in Role Management re-runs the sync (ruling R10).
// Role Management edits admin_hris only as super_admin (TestHrisRolesOnlySuperAdmin).
func TestAdminHrisEditResyncsDerivedRoles(t *testing.T) {
	admin := testutil.CreateStaff(t, testutil.StaffOptions{Role: "super_admin"})
	mux, text := setup(t)
	adminHris := text(`SELECT id::text FROM iam.roles WHERE code = 'admin_hris'`)
	raw := text(`SELECT json_agg(json_build_object('menuId', p.menu_id, 'isGranted', true, 'grantedActions', p.granted_actions))::text
		FROM iam.role_menu_permissions p JOIN iam.menus m ON m.id = p.menu_id
		WHERE p.role_id = $1 AND p.is_active AND m.deleted_at IS NULL AND m.code <> 'hris.kepegawaian.holidays'`, adminHris)
	var perms []any
	if err := json.Unmarshal([]byte(raw), &perms); err != nil {
		t.Fatal(err)
	}
	expect(t, do(t, mux, &admin, "PUT", "/api/settings/iam/roles/"+adminHris+"/permissions", map[string]any{"permissions": perms}), 200, "")
	if got := text(`SELECT count(*)::text FROM iam.role_menu_permissions p JOIN iam.roles r ON r.id = p.role_id JOIN iam.menus m ON m.id = p.menu_id
		WHERE r.code = 'hris_manager' AND m.code = 'hris.kepegawaian.holidays' AND p.is_active`); got != "0" {
		t.Fatal("hris_manager kept Hari Libur after admin_hris lost it")
	}
	if got := text(`SELECT count(*)::text FROM iam.permission_audit_logs WHERE source = 'role_permissions'
		AND role_code = 'hris_manager' AND menu_code = 'hris.kepegawaian.holidays' AND actions_after IS NULL`); got != "1" {
		t.Fatalf("derived-role audit rows = %s", got)
	}
	expect(t, do(t, mux, &admin, "PUT", "/api/settings/iam/roles/"+adminHris+"/permissions", map[string]any{"permissions": []any{}}), 400,
		`{"success":false,"error":"admin_hris harus punya minimal satu menu: role HR lain diturunkan darinya"}`)
}

const (
	errHrisRoleSuper   = `{"success":false,"error":"Role HR hanya bisa diubah oleh Super Admin; AdminHRIS memakai halaman Hak Akses HRIS"}`
	errHrisDeleteSuper = `{"success":false,"error":"Hak hapus HRIS hanya bisa diberikan oleh Super Admin"}`
)

// HR roles (admin_hris and the derived hris_*) change in Role Management
// only through super_admin; AdminHRIS uses Hak Akses HRIS (Q-RA-2/Q-RA-5).
func TestHrisRolesOnlySuperAdmin(t *testing.T) {
	admin := testutil.CreateStaff(t, testutil.StaffOptions{Menus: map[string][]string{"settings.roles": nil}})
	super := testutil.CreateStaff(t, testutil.StaffOptions{Role: "super_admin"})
	mux, text := setup(t)
	menuID := text(`SELECT id::text FROM iam.menus WHERE code = 'hris.workforce.leaves'`)
	perms := map[string]any{"permissions": []any{map[string]any{"menuId": menuID, "isGranted": true, "grantedActions": []string{"read"}}}}
	for _, code := range []string{"admin_hris", "hris_manager", "hris_non_payroll", "hris_non_rekrutmen_kpi"} {
		id := text(`SELECT id::text FROM iam.roles WHERE code = $1 AND deleted_at IS NULL`, code)
		before := text(`SELECT name || '|' || (SELECT count(*) FROM iam.role_menu_permissions WHERE role_id = $1 AND is_active) FROM iam.roles WHERE id = $1`, id)
		expect(t, do(t, mux, &admin, "PUT", "/api/settings/iam/roles/"+id, map[string]any{"name": "Ubah"}), 403, errHrisRoleSuper)
		expect(t, do(t, mux, &admin, "PUT", "/api/settings/iam/roles/"+id+"/permissions", perms), 403, errHrisRoleSuper)
		expect(t, do(t, mux, &admin, "DELETE", "/api/settings/iam/roles/"+id, nil), 403, errHrisRoleSuper)
		if got := text(`SELECT name || '|' || (SELECT count(*) FROM iam.role_menu_permissions WHERE role_id = $1 AND is_active) FROM iam.roles WHERE id = $1 AND deleted_at IS NULL`, id); got != before {
			t.Fatalf("%s changed: %s -> %s", code, before, got)
		}
	}
	other := text(`INSERT INTO iam.roles (code, name) VALUES ($1, 'Go Other') RETURNING id::text`, "go_other_"+testutil.RandomHex(3))
	expect(t, do(t, mux, &admin, "PUT", "/api/settings/iam/roles/"+other, map[string]any{"code": "hris_manager"}), 403, errHrisRoleSuper)
	manager := text(`SELECT id::text FROM iam.roles WHERE code = 'hris_manager'`)
	expect(t, do(t, mux, &super, "PUT", "/api/settings/iam/roles/"+manager, map[string]any{"name": "HR Manager"}), 200, "")
}

// Adding "delete" on an HRIS menu through Role Management needs super_admin;
// keeping an existing delete or delete outside HRIS does not.
func TestHrisDeleteGrantOnlySuperAdmin(t *testing.T) {
	admin := testutil.CreateStaff(t, testutil.StaffOptions{Menus: map[string][]string{"settings.roles": nil}})
	super := testutil.CreateStaff(t, testutil.StaffOptions{Role: "super_admin"})
	mux, text := setup(t)
	role := text(`INSERT INTO iam.roles (code, name) VALUES ($1, 'Go Role') RETURNING id::text`, "go_role_"+testutil.RandomHex(3))
	attendance := text(`SELECT id::text FROM iam.menus WHERE code = 'hris.workforce.attendance'`)
	leaves := text(`SELECT id::text FROM iam.menus WHERE code = 'hris.workforce.leaves'`)
	plain := text(`INSERT INTO iam.menus (code, menu_name) VALUES ($1, 'Go Menu') RETURNING id::text`, "go_menu_"+testutil.RandomHex(3))
	text(`INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions) VALUES ($1, $2, '["read","delete"]') RETURNING 'ok'`, role, attendance)
	row := func(menu string, actions ...string) map[string]any {
		return map[string]any{"menuId": menu, "isGranted": true, "grantedActions": actions}
	}
	perms := func(rows ...any) map[string]any { return map[string]any{"permissions": rows} }
	path := "/api/settings/iam/roles/" + role + "/permissions"

	expect(t, do(t, mux, &admin, "PUT", path, perms(row(attendance, "read", "delete"), row(leaves, "read"), row(plain, "read", "delete"))), 200, "")
	expect(t, do(t, mux, &admin, "PUT", path, perms(row(attendance, "read", "delete"), row(leaves, "read", "delete"))), 403, errHrisDeleteSuper)
	if got := text(`SELECT granted_actions::text FROM iam.role_menu_permissions WHERE role_id = $1 AND menu_id = $2`, role, leaves); got != `["read"]` {
		t.Fatalf("leaves after refused save = %s", got)
	}
	// Dropping delete and adding it back later is an addition too.
	expect(t, do(t, mux, &admin, "PUT", path, perms(row(attendance, "read"))), 200, "")
	expect(t, do(t, mux, &admin, "PUT", path, perms(row(attendance, "read", "delete"))), 403, errHrisDeleteSuper)
	expect(t, do(t, mux, &super, "PUT", path, perms(row(attendance, "read", "delete"), row(leaves, "read", "delete"))), 200, "")
}

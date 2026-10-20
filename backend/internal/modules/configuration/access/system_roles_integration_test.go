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
func TestAdminHrisEditResyncsDerivedRoles(t *testing.T) {
	admin := testutil.CreateStaff(t, testutil.StaffOptions{Menus: map[string][]string{"settings.roles": nil}})
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

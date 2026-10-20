package access_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"nuhabit/backend/internal/modules/configuration/access"
	"nuhabit/backend/internal/platform/module"
	"nuhabit/backend/internal/platform/testutil"
)

// mod mounts a route list as a module for testutil.Mux.
type mod []module.Route

func (m mod) Name() string           { return "configuration" }
func (m mod) Routes() []module.Route { return m }

type resp struct {
	Status int
	Raw    string
	Body   map[string]any
}

// setup mounts the routes on a rolled-back transaction. Create staff first:
// cleanups run in reverse, so the rollback then releases the FK locks the
// transaction holds on the staff rows before their cleanup deletes them.
func setup(t *testing.T) (*http.ServeMux, func(sql string, args ...any) string) {
	deps := testutil.Deps(t, nil)
	tx := testutil.Tx(t)
	mux := testutil.Mux(mod(access.Routes(deps, tx)))
	text := func(sql string, args ...any) string {
		t.Helper()
		var s string
		if err := tx.QueryRow(context.Background(), sql, args...).Scan(&s); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return s
	}
	return mux, text
}

func do(t *testing.T, mux *http.ServeMux, staff *testutil.Staff, method, path string, body any) resp {
	t.Helper()
	r := testutil.Request(method, path, body)
	if staff != nil {
		r = testutil.AsStaff(r, *staff)
	}
	rec, parsed := testutil.Do(t, mux, r)
	return resp{rec.Code, rec.Body.String(), parsed}
}

func expect(t *testing.T, r resp, status int, raw string) {
	t.Helper()
	if r.Status != status || (raw != "" && r.Raw != raw) {
		t.Fatalf("got %d %s, want %d %s", r.Status, r.Raw, status, raw)
	}
}

func TestRolesCRUDAndPermissions(t *testing.T) {
	admin := testutil.CreateStaff(t, testutil.StaffOptions{Menus: map[string][]string{"settings.roles": nil}})
	outsider := testutil.CreateStaff(t, testutil.StaffOptions{Menus: map[string][]string{"settings.menus": nil}})
	mux, text := setup(t)

	expect(t, do(t, mux, nil, "GET", "/api/settings/iam/roles", nil), 401, `{"success":false,"error":"Authentication required"}`)
	expect(t, do(t, mux, &outsider, "GET", "/api/settings/iam/roles", nil), 403, `{"success":false,"error":"Insufficient permissions"}`)
	expect(t, do(t, mux, &admin, "POST", "/api/settings/iam/roles", "{bad"), 500, `{"success":false,"error":"Terjadi kesalahan server"}`)
	expect(t, do(t, mux, &admin, "POST", "/api/settings/iam/roles", map[string]any{"code": "  ", "name": 5}), 400,
		`{"success":false,"error":"Validation failed","details":[{"code":"too_small","path":["code"],"message":"Code and name are required"},{"code":"invalid_type","path":["name"],"message":"Invalid input: expected string, received number"}]}`)

	code := "Go_Role_" + testutil.RandomHex(3)
	r := do(t, mux, &admin, "POST", "/api/settings/iam/roles", map[string]any{"code": " " + code + " ", "name": " Kasir Malam ", "description": "   "})
	expect(t, r, 201, "")
	id := r.Body["id"].(string)
	if got := text(`SELECT code || '|' || name || '|' || coalesce(description, 'NULL') || '|' || is_active FROM iam.roles WHERE id = $1`, id); got != strings.ToLower(code)+"|Kasir Malam|NULL|true" {
		t.Fatalf("role row = %s", got)
	}

	r = do(t, mux, &admin, "GET", "/api/settings/iam/roles?search="+strings.ToUpper(code)+"&status=active", nil)
	expect(t, r, 200, `{"data":[{"id":"`+id+`","code":"`+strings.ToLower(code)+`","name":"Kasir Malam","description":null,"isSystem":false,"isActive":true,"menuPermissionCount":0}],"total":1}`)
	expect(t, do(t, mux, &admin, "GET", "/api/settings/iam/roles?search="+code+"&status=inactive", nil), 200, `{"data":[],"total":0}`)

	// Grant one menu; isGranted rows carry their actions, others none.
	menuID := text(`INSERT INTO iam.menus (code, menu_name, permission_context) VALUES ($1, 'Go Menu', '{"actions":["read","create"]}') RETURNING id::text`, "go_menu_"+testutil.RandomHex(3))
	r = do(t, mux, &admin, "PUT", "/api/settings/iam/roles/"+id+"/permissions", map[string]any{"permissions": []any{
		map[string]any{"menuId": menuID, "grantedActions": []string{}},
		map[string]any{"menuId": "", "isGranted": true},
	}})
	expect(t, r, 200, "")
	perm := findPermission(t, r.Body["permissions"], menuID)
	if b, _ := json.Marshal(perm); string(b) != `{"availableActions":["read","create"],"grantedActions":["read"],"isGranted":true,"level":1,"menuCode":"`+perm["menuCode"].(string)+`","menuId":"`+menuID+`","menuName":"Go Menu","menuType":"sidebar","orderNumber":0,"parentId":null}` {
		t.Fatalf("permission = %s", b)
	}
	if !strings.HasPrefix(r.Raw, `{"roleId":"`+id+`","permissions":[`) {
		t.Fatalf("permissions body = %.80s", r.Raw)
	}
	expect(t, do(t, mux, &admin, "PUT", "/api/settings/iam/roles/"+id+"/permissions", map[string]any{"permissions": []any{map[string]any{"menuId": menuID, "isGranted": false}}}), 200, "")
	if got := text(`SELECT is_active::text FROM iam.role_menu_permissions WHERE role_id = $1 AND menu_id = $2`, id, menuID); got != "false" {
		t.Fatalf("unchecked grant active = %s", got)
	}

	r = do(t, mux, &admin, "GET", "/api/settings/iam/roles/"+id, nil)
	expect(t, r, 200, "")
	if !strings.HasPrefix(r.Raw, `{"id":"`+id+`","code":"`+strings.ToLower(code)+`","name":"Kasir Malam","description":null,"isSystem":false,"isActive":true,"menuPermissionCount":0,"createdAt":"`) ||
		!strings.Contains(r.Raw, `"updatedAt":null,"version":1,"permissions":[`) {
		t.Fatalf("role detail = %.400s", r.Raw)
	}
	perm = findPermission(t, r.Body["permissions"], menuID)
	if perm["isGranted"] != false || len(perm["grantedActions"].([]any)) != 0 {
		t.Fatalf("revoked permission = %v", perm)
	}

	expect(t, do(t, mux, &admin, "PUT", "/api/settings/iam/roles/"+id, map[string]any{"code": " NEW_Code ", "description": nil, "isActive": false}), 200, `{"id":"`+id+`"}`)
	if got := text(`SELECT code || '|' || coalesce(description, 'NULL') || '|' || is_active || '|' || version FROM iam.roles WHERE id = $1`, id); got != "new_code|NULL|false|2" {
		t.Fatalf("updated role = %s", got)
	}

	systemCode := "go_sys_" + testutil.RandomHex(3)
	system := text(`INSERT INTO iam.roles (code, name, is_system) VALUES ($1, 'Go System', true) RETURNING id::text`, systemCode)
	expect(t, do(t, mux, &admin, "PUT", "/api/settings/iam/roles/"+system, map[string]any{"code": systemCode + "x"}), 403,
		`{"success":false,"error":"Role sistem hanya bisa diubah oleh Super Admin"}`)
	expect(t, do(t, mux, &admin, "DELETE", "/api/settings/iam/roles/"+system, nil), 400,
		`{"success":false,"error":"System roles cannot be deleted"}`)

	expect(t, do(t, mux, &admin, "DELETE", "/api/settings/iam/roles/"+id, nil), 200, `{"success":true}`)
	for _, path := range []string{"/api/settings/iam/roles/" + id, "/api/settings/iam/roles/" + id + "/permissions"} {
		expect(t, do(t, mux, &admin, "GET", path, nil), 404, `{"success":false,"error":"Role not found"}`)
	}
	expect(t, do(t, mux, &admin, "PUT", "/api/settings/iam/roles/"+id, map[string]any{}), 404, `{"success":false,"error":"Role not found"}`)
	expect(t, do(t, mux, &admin, "DELETE", "/api/settings/iam/roles/"+id, nil), 404, `{"success":false,"error":"Role not found"}`)
	expect(t, do(t, mux, &admin, "GET", "/api/settings/iam/roles/nope", nil), 400, `{"success":false,"error":"Format data tidak valid"}`)
}

func findPermission(t *testing.T, list any, menuID string) map[string]any {
	t.Helper()
	for _, p := range list.([]any) {
		if m := p.(map[string]any); m["menuId"] == menuID {
			return m
		}
	}
	t.Fatalf("menu %s missing from the matrix", menuID)
	return nil
}

func TestMenusCRUD(t *testing.T) {
	admin := testutil.CreateStaff(t, testutil.StaffOptions{Menus: map[string][]string{"settings.menus": nil}})
	outsider := testutil.CreateStaff(t, testutil.StaffOptions{Menus: map[string][]string{"settings.roles": nil}})
	mux, text := setup(t)

	expect(t, do(t, mux, &outsider, "GET", "/api/settings/iam/menus", nil), 403, `{"success":false,"error":"Insufficient permissions"}`)
	expect(t, do(t, mux, &admin, "POST", "/api/settings/iam/menus", map[string]any{"code": "", "menuType": "tab", "orderNumber": 1.5, "permissionContext": nil}), 400,
		`{"success":false,"error":"Validation failed","details":[{"code":"too_small","path":["code"],"message":"Too small: expected string to have >=1 characters"},{"code":"invalid_type","path":["menuName"],"message":"Invalid input: expected string, received undefined"},{"code":"invalid_value","path":["menuType"],"message":"Invalid option: expected one of \"sidebar\"|\"group\""},{"code":"invalid_type","path":["orderNumber"],"message":"Invalid input: expected int, received number"},{"code":"invalid_type","path":["permissionContext"],"message":"Invalid input: expected object, received null"}]}`)

	code := "go_menu_" + testutil.RandomHex(4)
	r := do(t, mux, &admin, "POST", "/api/settings/iam/menus", map[string]any{"code": " " + code + " ", "menuName": "Go Laporan", "routePath": "/dashboard/go", "module": "GoMod"})
	expect(t, r, 201, "")
	id := r.Body["id"].(string)
	if got := text(`SELECT menu_type || '|' || order_number || '|' || permission_context::text || '|' || created_by FROM iam.menus WHERE id = $1`, id); got != `sidebar|0|{"actions": ["read"]}|`+admin.UserID {
		t.Fatalf("menu row = %s", got)
	}

	r = do(t, mux, &admin, "GET", "/api/settings/iam/menus/"+id, nil)
	expect(t, r, 200, "")
	if !strings.HasPrefix(r.Raw, `{"id":"`+id+`","parentId":null,"code":"`+code+`","menuName":"Go Laporan","routePath":"/dashboard/go","module":"GoMod","menuType":"sidebar","icon":null,"orderNumber":0,"level":1,"isActive":true,"isVisible":true,"description":null,"permissionContext":{"actions":["read"]},"openInNewTab":false,"createdAt":"`) ||
		!strings.HasSuffix(r.Raw, `","updatedAt":null,"version":1}`) {
		t.Fatalf("menu detail = %s", r.Raw)
	}

	expect(t, do(t, mux, &admin, "GET", "/api/settings/iam/menus?search=gomod&menuType=SIDEBAR", nil), 200, "")
	r = do(t, mux, &admin, "GET", "/api/settings/iam/menus?search=gomod&menuType=group", nil)
	expect(t, r, 200, `{"data":[],"total":0}`)
	r = do(t, mux, &admin, "GET", "/api/settings/iam/menus?search=GOMOD", nil)
	if r.Body["total"] != float64(1) {
		t.Fatalf("menu search = %s", r.Raw)
	}

	expect(t, do(t, mux, &admin, "PUT", "/api/settings/iam/menus/"+id, map[string]any{
		"menuType": "group", "routePath": nil, "icon": "chart", "permissionContext": map[string]any{"actions": []string{"read", "export"}, "extra": 1},
	}), 200, `{"id":"`+id+`"}`)
	if got := text(`SELECT menu_type || '|' || coalesce(route_path, 'NULL') || '|' || module || '|' || icon || '|' || permission_context::text || '|' || updated_by FROM iam.menus WHERE id = $1`, id); got != `group|NULL|GoMod|chart|{"actions": ["read", "export"]}|`+admin.UserID {
		t.Fatalf("updated menu = %s", got)
	}

	expect(t, do(t, mux, &admin, "DELETE", "/api/settings/iam/menus/"+id, nil), 200, `{"success":true}`)
	expect(t, do(t, mux, &admin, "GET", "/api/settings/iam/menus/"+id, nil), 404, `{"success":false,"error":"Menu not found"}`)
	expect(t, do(t, mux, &admin, "PUT", "/api/settings/iam/menus/"+id, map[string]any{}), 404, `{"success":false,"error":"Menu not found"}`)
	expect(t, do(t, mux, &admin, "DELETE", "/api/settings/iam/menus/"+id, nil), 404, `{"success":false,"error":"Menu not found"}`)
}

// A failing statement aborts the test transaction, so the conflict runs alone.
func TestRoleCodeConflict(t *testing.T) {
	admin := testutil.CreateStaff(t, testutil.StaffOptions{Menus: map[string][]string{"settings.roles": nil}})
	mux, text := setup(t)
	code := text(`SELECT code FROM iam.roles WHERE deleted_at IS NULL ORDER BY code LIMIT 1`)
	expect(t, do(t, mux, &admin, "POST", "/api/settings/iam/roles", map[string]any{"code": strings.ToUpper(code), "name": "x"}), 409,
		`{"success":false,"error":"Data sudah ada di sistem"}`)
}

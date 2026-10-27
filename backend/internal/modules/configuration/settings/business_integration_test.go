package settings_test

import (
	"encoding/json"
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

// findNode returns the node with id in a decoded tree level.
func findNode(list any, id string) map[string]any {
	for _, n := range list.([]any) {
		if m := n.(map[string]any); m["id"] == id {
			return m
		}
	}
	return nil
}

func TestBusinessTree(t *testing.T) {
	admin := staff(t, "settings.business")
	e := newEnv(t)
	post := func(body map[string]any) resp { return e.do(&admin, "POST", "/api/settings/business", body) }

	expect(t, post(map[string]any{"type": "region", "name": 5}), 400,
		`{"success":false,"error":"Validation failed","details":[{"code":"invalid_value","path":["type"],"message":"Tipe entitas tidak valid"},{"code":"invalid_type","path":["name"],"message":"Invalid input: expected string, received number"}]}`)
	expect(t, post(map[string]any{"type": "holding", "name": "  "}), 400, `{"success":false,"error":"Nama wajib diisi"}`)
	expect(t, post(map[string]any{"type": "company", "name": "Co", "parentId": nil}), 400,
		`{"success":false,"error":"Holding wajib dipilih"}`)
	// node-postgres's message matches /valid/ ("invalid input syntax"), so it reaches the client.
	expect(t, post(map[string]any{"type": "branch", "name": "Br", "parentId": "nope"}), 400,
		`{"success":false,"error":"invalid input syntax for type uuid: \"nope\""}`)

	suffix := strings.ToUpper(testutil.RandomHex(3))
	r := post(map[string]any{"type": "holding", "name": "  Go Holding " + suffix + " "})
	expect(t, r, 201, "")
	body := r.json(t)
	holdingID := body["data"].(map[string]any)["id"].(string)
	if body["data"].(map[string]any)["type"] != "holding" || !strings.HasPrefix(r.Raw, `{"data":{"id":"`+holdingID+`","type":"holding"},"tree":{"holdings":[`) {
		t.Fatalf("create body %s", r.Raw)
	}
	holding := findNode(body["tree"].(map[string]any)["holdings"], holdingID)
	raw, _ := json.Marshal(holding)
	if string(raw) != `{"code":"GO-HOLDING-`+suffix+`","companies":[],"id":"`+holdingID+`","is_active":true,"name":"Go Holding `+suffix+`"}` {
		t.Fatalf("holding node %s", raw)
	}
	expect(t, post(map[string]any{"type": "holding", "name": "Dup", "code": "go-holding-" + strings.ToLower(suffix)}), 409,
		`{"success":false,"error":"Data sudah ada di sistem"}`)

	r = post(map[string]any{"type": "company", "name": "Go Co", "code": "gc", "parentId": holdingID, "is_active": false})
	expect(t, r, 201, "")
	companyID := r.json(t)["data"].(map[string]any)["id"].(string)
	r = post(map[string]any{"type": "branch", "name": "Go Branch", "parentId": companyID})
	branchID := r.json(t)["data"].(map[string]any)["id"].(string)
	// A trigger gives every new branch its MAIN default warehouse.
	r = post(map[string]any{"type": "warehouse", "name": "Kopi", "parentId": branchID})
	expect(t, r, 201, "")
	stallID := r.json(t)["data"].(map[string]any)["id"].(string)
	post(map[string]any{"type": "warehouse", "name": "Teh", "code": "a-1", "parentId": branchID})

	tree := e.do(&admin, "GET", "/api/settings/business", nil)
	expect(t, tree, 200, "")
	h := findNode(tree.json(t)["data"].(map[string]any)["holdings"], holdingID)
	co := findNode(h["companies"], companyID)
	br := findNode(co["branches"], branchID)
	if co["code"] != "GC" || co["is_active"] != false || br["code"] != "GO-BRANCH" {
		t.Fatalf("company/branch %v %v", co, br)
	}
	var codes []string
	for _, w := range br["warehouses"].([]any) {
		codes = append(codes, w.(map[string]any)["code"].(string))
	}
	if strings.Join(codes, ",") != "MAIN,A-1,STALL-02" {
		t.Fatalf("warehouse order %v", codes)
	}

	patch := func(path string, body any) resp { return e.do(&admin, "PATCH", path, body) }
	expect(t, patch("/api/settings/business/region/"+companyID, map[string]any{}), 400,
		`{"success":false,"error":"Tipe entitas tidak valid"}`)
	expect(t, patch("/api/settings/business/company/"+companyID, map[string]any{}), 400,
		`{"success":false,"error":"Tidak ada data yang diperbarui"}`)
	expect(t, patch("/api/settings/business/company/"+companyID, map[string]any{"is_active": "y"}), 400,
		`{"success":false,"error":"Validation failed","details":[{"code":"invalid_type","path":["is_active"],"message":"Invalid input: expected boolean, received string"}]}`)
	expect(t, patch("/api/settings/business/company/nope", map[string]any{"name": "x"}), 400,
		`{"success":false,"error":"Format data tidak valid"}`)
	r = patch("/api/settings/business/company/"+companyID, map[string]any{"name": " Go Co 2 ", "code": " gc2 ", "is_active": true})
	expect(t, r, 200, "")
	if !strings.HasPrefix(r.Raw, `{"data":{"id":"`+companyID+`"},"tree":{"holdings":[`) {
		t.Fatalf("patch body %s", r.Raw)
	}
	if got := e.text(`SELECT name || '|' || code || '|' || is_active FROM configuration.companies WHERE id = $1`, companyID); got != "Go Co 2|GC2|true" {
		t.Fatalf("company row %s", got)
	}

	del := func(path string) resp { return e.do(&admin, "DELETE", path, nil) }
	expect(t, del("/api/settings/business/warehouse/"+holdingID), 400, `{"success":false,"error":"Outlet tidak ditemukan"}`)
	expect(t, del("/api/settings/business/warehouse/"+stallID), 200, "")
	e.exec(`DELETE FROM configuration.warehouses WHERE branch_id = $1 AND NOT is_default`, branchID)
	mainID := e.text(`SELECT id::text FROM configuration.warehouses WHERE branch_id = $1`, branchID)
	expect(t, del("/api/settings/business/warehouse/"+mainID), 400,
		`{"success":false,"error":"Main Storage tidak dapat dihapus. Setiap cabang wajib memiliki minimal 1 outlet."}`)
	expect(t, del("/api/settings/business/holding/"+holdingID), 200, "")
	if got := e.text(`SELECT count(*)::text FROM configuration.companies WHERE id = $1`, companyID); got != "0" {
		t.Fatal("holding delete cascades")
	}
}

// The Buat Akun Login dialog picks the account's data scope from this tree:
// account managers (Manajemen User) read it, only Business settings writes.
func TestBusinessTreeReadableByAccountManagers(t *testing.T) {
	users := staff(t, "settings.users")
	none := staff(t)
	e := newEnv(t)
	expect(t, e.do(&users, "GET", "/api/settings/business", nil), 200, "")
	expect(t, e.do(&none, "GET", "/api/settings/business", nil), 403, "")
	expect(t, e.do(&users, "POST", "/api/settings/business", map[string]any{"type": "holding", "name": "Tidak Boleh"}), 403, "")
}

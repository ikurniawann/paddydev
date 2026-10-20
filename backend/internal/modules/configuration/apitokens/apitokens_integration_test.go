package apitokens_test

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/modules/configuration/apitokens"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/module"
	"nuhabit/backend/internal/platform/testutil"
)

type mod []module.Route

func (m mod) Name() string           { return "configuration" }
func (m mod) Routes() []module.Route { return m }

var now = time.Date(2026, 10, 4, 3, 0, 0, 0, time.UTC)

// setup mounts the routes on a rolled-back transaction; create staff first
// so the rollback releases FK locks before their cleanup runs.
func setup(t *testing.T) (*http.ServeMux, pgx.Tx) {
	deps := testutil.Deps(t, func() time.Time { return now })
	tx := testutil.Tx(t)
	return testutil.Mux(mod(apitokens.Routes(deps, tx))), tx
}

func call(t *testing.T, mux *http.ServeMux, r *http.Request) (int, string, map[string]any) {
	t.Helper()
	rec, body := testutil.Do(t, mux, r)
	return rec.Code, rec.Body.String(), body
}

func TestTokenLifecycle(t *testing.T) {
	admin := testutil.CreateStaff(t, testutil.StaffOptions{Menus: map[string][]string{"settings.integrations": nil}})
	plain := testutil.CreateStaff(t, testutil.StaffOptions{Menus: map[string][]string{"settings.users": nil}})
	mux, tx := setup(t)
	ctx := context.Background()

	if code, raw, _ := call(t, mux, testutil.Request("GET", "/api/admin/api-tokens", nil)); code != 401 || raw != `{"success":false,"error":"Authentication required"}` {
		t.Fatalf("anonymous = %d %s", code, raw)
	}
	if code, _, _ := call(t, mux, testutil.AsStaff(testutil.Request("GET", "/api/admin/api-tokens", nil), plain)); code != 403 {
		t.Fatalf("without settings.integrations = %d", code)
	}

	post := func(body any) (int, string, map[string]any) {
		return call(t, mux, testutil.AsStaff(testutil.Request("POST", "/api/admin/api-tokens", body), admin))
	}
	if code, raw, _ := post(map[string]any{"name": "Agent", "scopes": []string{"hr:delete"}}); code != 400 ||
		raw != `{"success":false,"error":"Scope tidak valid. Pakai '*' atau '<modul>:read|write' (modul: pos, member, hris, inventory, crm, config, reports, other)"}` {
		t.Fatalf("bad scope = %d %s", code, raw)
	}
	if code, raw, _ := post(map[string]any{"name": "  ", "scopes": []string{"*"}}); code != 400 || raw != `{"success":false,"error":"Nama token wajib diisi"}` {
		t.Fatalf("no name = %d %s", code, raw)
	}
	if code, raw, _ := post(map[string]any{"name": "x", "scopes": []string{"*"}, "user_id": "00000000-0000-4000-8000-000000000000"}); code != 400 ||
		raw != `{"success":false,"error":"user_id tidak dikenal"}` {
		t.Fatalf("unknown user = %d %s", code, raw)
	}
	if code, raw, _ := post("{"); code != 500 || raw != `{"success":false,"error":"Terjadi kesalahan server"}` {
		t.Fatalf("malformed = %d %s", code, raw)
	}

	code, raw, body := post(map[string]any{"name": " Agent ", "scopes": []string{"pos:read", "*"}, "expires_in_days": 0})
	if code != 201 {
		t.Fatalf("create = %d %s", code, raw)
	}
	data := body["data"].(map[string]any)
	id, token := data["id"].(string), data["token"].(string)
	if !regexp.MustCompile(`^nh_[0-9a-f]{64}$`).MatchString(token) ||
		!regexp.MustCompile(`^\{"success":true,"data":\{"id":"[0-9a-f-]{36}","name":"Agent","token_prefix":"`+token[:15]+`","scopes":\["pos:read","\*"\],"user_id":"`+admin.UserID+`","created_at":"[0-9T:.-]+Z","expires_at":null,"token":"`+token+`"\}\}$`).MatchString(raw) {
		t.Fatalf("create body = %s", raw)
	}
	var hash, createdBy string
	if err := tx.QueryRow(ctx, `SELECT token_hash, created_by::text FROM configuration.api_tokens WHERE id = $1`, id).Scan(&hash, &createdBy); err != nil {
		t.Fatal(err)
	}
	if hash != auth.HashToken(token) || createdBy != admin.UserID {
		t.Fatalf("stored hash/created_by = %s %s", hash, createdBy)
	}

	_, _, body = post(map[string]any{"name": 42, "scopes": []string{"crm:write"}, "user_id": plain.UserID, "expires_in_days": "2"})
	other := body["data"].(map[string]any)
	if other["name"] != "42" || other["user_id"] != plain.UserID || other["expires_at"] != "2026-10-06T03:00:00.000Z" {
		t.Fatalf("second token = %v", other)
	}

	code, raw, body = call(t, mux, testutil.AsStaff(testutil.Request("GET", "/api/admin/api-tokens", nil), admin))
	if code != 200 || !strings.HasPrefix(raw, `{"success":true,"data":[`) || body["migration_pending"] != nil {
		t.Fatalf("list = %d %.200s", code, raw)
	}
	found := false
	for _, it := range body["data"].([]any) {
		row := it.(map[string]any)
		if row["id"] == id {
			found = row["user_name"] == admin.FullName && row["revoked_at"] == nil && row["last_used_at"] == nil && row["token_hash"] == nil
		}
	}
	if !found {
		t.Fatalf("token %s missing or wrong in list: %s", id, raw)
	}

	del := func() (int, string) {
		c, r, _ := call(t, mux, testutil.AsStaff(testutil.Request("DELETE", "/api/admin/api-tokens/"+id, nil), admin))
		return c, r
	}
	if c, r := del(); c != 200 || r != `{"success":true,"data":{"id":"`+id+`","name":"Agent"}}` {
		t.Fatalf("revoke = %d %s", c, r)
	}
	if c, r := del(); c != 404 || r != `{"success":false,"error":"Token tidak ditemukan atau sudah dicabut"}` {
		t.Fatalf("second revoke = %d %s", c, r)
	}
}

// A Bearer token session is refused before the menu check, even when its
// account holds settings.integrations.
func TestTokenSessionRefused(t *testing.T) {
	admin := testutil.CreateStaff(t, testutil.StaffOptions{Menus: map[string][]string{"settings.integrations": nil}})
	deps := testutil.Deps(t, nil)
	mux := testutil.Mux(mod(apitokens.Routes(deps, deps.DB)))
	token := "nh_" + testutil.RandomHex(32)
	if _, err := deps.DB.Exec(context.Background(), `INSERT INTO configuration.api_tokens (name, token_hash, token_prefix, user_id, scopes)
		VALUES ('go-test', $1, $2, $3, '{*}')`, auth.HashToken(token), token[:15], admin.UserID); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"GET", "POST"} {
		r := testutil.Request(method, "/api/admin/api-tokens", map[string]any{"name": "x", "scopes": []string{"*"}})
		r.Header.Set("Authorization", "Bearer "+token)
		if code, raw, _ := call(t, mux, r); code != 403 || raw != `{"success":false,"error":"Kelola token hanya lewat login dashboard, bukan token"}` {
			t.Fatalf("%s with token = %d %s", method, code, raw)
		}
	}
}

// Without the table the list is empty with migration_pending and create is
// a 503. Each request gets its own transaction: the 42P01 aborts it.
func TestMigrationPending(t *testing.T) {
	admin := testutil.CreateStaff(t, testutil.StaffOptions{Menus: map[string][]string{"settings.integrations": nil}})
	cases := []struct {
		method, want string
		status       int
	}{
		{"GET", `{"success":true,"data":[],"migration_pending":true}`, 200},
		{"POST", `{"success":false,"error":"Tabel api_tokens belum ada. Jalankan migrasi database dulu: pnpm db:migrate:apply"}`, 503},
	}
	for _, c := range cases {
		t.Run(c.method, func(t *testing.T) {
			mux, tx := setup(t)
			if _, err := tx.Exec(context.Background(), `ALTER TABLE configuration.api_tokens RENAME TO api_tokens_go_test`); err != nil {
				t.Fatal(err)
			}
			r := testutil.AsStaff(testutil.Request(c.method, "/api/admin/api-tokens", map[string]any{"name": "x", "scopes": []string{"*"}}), admin)
			if code, raw, _ := call(t, mux, r); code != c.status || raw != c.want {
				t.Fatalf("%s = %d %s", c.method, code, raw)
			}
		})
	}
}

const systemTokenMsg = `{"success":false,"error":"Token API untuk akun Super Admin/Admin hanya bisa dibuat oleh Super Admin"}`

// TestSystemAccountTokenNeedsSuperAdmin: a token runs as its account, so a
// token for an account holding a system role (super_admin/admin, app or IAM
// role) is minted by IAM super_admin only; a refusal writes nothing.
func TestSystemAccountTokenNeedsSuperAdmin(t *testing.T) {
	superTarget := testutil.CreateStaff(t, testutil.StaffOptions{Role: "super_admin"})
	adminTarget := testutil.CreateStaff(t, testutil.StaffOptions{Role: "admin"})
	iamSuper := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd"})
	plain := testutil.CreateStaff(t, testutil.StaffOptions{Role: "pos"})
	hrd := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{"settings.integrations": nil}})
	mux, tx := setup(t)
	ctx := context.Background()
	if _, err := tx.Exec(ctx, `INSERT INTO iam.roles (code, name, is_system) VALUES ('super_admin', 'Super Admin', true) ON CONFLICT (code) DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO iam.user_roles (user_id, role_id, is_primary) SELECT $1, id, true FROM iam.roles WHERE code = 'super_admin'`, iamSuper.UserID); err != nil {
		t.Fatal(err)
	}
	post := func(as testutil.Staff, userID string) (int, string) {
		code, raw, _ := call(t, mux, testutil.AsStaff(testutil.Request("POST", "/api/admin/api-tokens",
			map[string]any{"name": "Agent", "scopes": []string{"*"}, "user_id": userID}), as))
		return code, raw
	}
	for _, target := range []testutil.Staff{superTarget, adminTarget, iamSuper} {
		if code, raw := post(hrd, target.UserID); code != 403 || raw != systemTokenMsg {
			t.Errorf("hrd → token for %s (%s) = %d %s", target.Role, target.UserID, code, raw)
		}
	}
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM configuration.api_tokens WHERE user_id = ANY($1)`,
		[]string{superTarget.UserID, adminTarget.UserID, iamSuper.UserID}).Scan(&n); err != nil || n != 0 {
		t.Fatalf("refused tokens written: %d %v", n, err)
	}
	if code, raw := post(hrd, plain.UserID); code != 201 {
		t.Fatalf("hrd → token for plain account = %d %s", code, raw)
	}
}

// TestSuperAdminMintsSystemAccountToken: IAM super_admin may.
func TestSuperAdminMintsSystemAccountToken(t *testing.T) {
	testutil.RequireSeededIAM(t, testutil.IAMBaseline{GrantedRoles: []string{"super_admin"}, Menus: []string{"settings.integrations"}})
	adminTarget := testutil.CreateStaff(t, testutil.StaffOptions{Role: "admin"})
	caller := testutil.CreateStaff(t, testutil.StaffOptions{Role: "super_admin"})
	mux, _ := setup(t)
	code, raw, _ := call(t, mux, testutil.AsStaff(testutil.Request("POST", "/api/admin/api-tokens",
		map[string]any{"name": "Agent", "scopes": []string{"*"}, "user_id": adminTarget.UserID}), caller))
	if code != 201 {
		t.Fatalf("super_admin → token for admin = %d %s", code, raw)
	}
}

// TestSystemAccountTokenIDForms: user_id is canonicalised as a uuid before
// the guard and the insert; a malformed one is a 400.
func TestSystemAccountTokenIDForms(t *testing.T) {
	superTarget := testutil.CreateStaff(t, testutil.StaffOptions{Role: "super_admin"})
	plain := testutil.CreateStaff(t, testutil.StaffOptions{Role: "pos"})
	hrd := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{"settings.integrations": nil}})
	mux, tx := setup(t)
	post := func(userID string) (int, string, map[string]any) {
		return call(t, mux, testutil.AsStaff(testutil.Request("POST", "/api/admin/api-tokens",
			map[string]any{"name": "Agent", "scopes": []string{"*"}, "user_id": userID}), hrd))
	}
	for _, id := range []string{strings.ToUpper(superTarget.UserID), strings.ReplaceAll(superTarget.UserID, "-", "")} {
		if code, raw, _ := post(id); code != 403 || raw != systemTokenMsg {
			t.Errorf("token for super_admin as %q = %d %s", id, code, raw)
		}
	}
	var n int
	if err := tx.QueryRow(context.Background(), `SELECT count(*) FROM configuration.api_tokens WHERE user_id = $1`, superTarget.UserID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("refused tokens written: %d %v", n, err)
	}
	if code, raw, _ := post("nope"); code != 400 || raw != `{"success":false,"error":"user_id tidak valid"}` {
		t.Fatalf("malformed user_id = %d %s", code, raw)
	}
	code, raw, body := post(strings.ToUpper(plain.UserID))
	if code != 201 || body["data"].(map[string]any)["user_id"] != plain.UserID {
		t.Fatalf("upper-case plain user_id = %d %s", code, raw)
	}
}

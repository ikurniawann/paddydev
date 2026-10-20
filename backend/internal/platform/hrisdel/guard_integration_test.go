package hrisdel_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/hrisdel"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/testutil"
)

func TestGuard(t *testing.T) {
	leaf := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{"hris.master.departments": {"read", "delete"}}})
	group := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{"hris.master": {"read", "create", "update", "delete"}}})
	sibling := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{"hris.master.positions": {"read", "delete"}}})
	noDelete := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{"hris.master.departments": {"read", "update"}}})
	deps := testutil.Deps(t, nil)
	tx := testutil.Tx(t)
	ctx := context.Background()
	var dept string
	if err := tx.QueryRow(ctx, `INSERT INTO hris.departments (name, code) VALUES ('Dept Hapus', $1) RETURNING id::text`,
		"GD"+testutil.RandomHex(3)).Scan(&dept); err != nil {
		t.Fatal(err)
	}
	var seenBody string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		seenBody = string(raw)
		if r.PathValue("id") == "missing" {
			_ = httpx.JSON(w, http.StatusNotFound, map[string]any{"success": false, "error": "Tidak ada"})
			return
		}
		if _, err := tx.Exec(r.Context(), `DELETE FROM hris.departments WHERE id::text = $1`, r.PathValue("id")); err != nil {
			t.Error(err)
		}
		_ = httpx.JSON(w, http.StatusOK, map[string]any{"message": "ok"})
	})
	mux := http.NewServeMux()
	mux.Handle("DELETE /t/{id}", hrisdel.Guard(deps.Auth, tx,
		hrisdel.Spec{Menu: "hris.master.departments", Entity: "hris.departments", Table: "hris.departments", IDParam: "id"}, next))
	mux.Handle("DELETE /self/{id}", hrisdel.Guard(deps.Auth, tx, hrisdel.Spec{Menu: "hris.master.departments", SelfAudited: true}, next))
	call := func(s *testutil.Staff, path string, body any) (int, string) {
		t.Helper()
		r := testutil.Request(http.MethodDelete, path, body)
		if s != nil {
			r = testutil.AsStaff(r, *s)
		}
		rec, _ := testutil.Do(t, mux, r)
		return rec.Code, rec.Body.String()
	}
	audits := func(id string) string {
		var n string
		if err := tx.QueryRow(ctx, `SELECT count(*)::text FROM audit.audit_log WHERE action = 'hris.departments.delete' AND entity_id = $1`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	if c, b := call(nil, "/t/"+dept+"?reason=Data+ganda", nil); c != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d %s", c, b)
	}
	for name, s := range map[string]*testutil.Staff{"group": &group, "sibling": &sibling, "no delete": &noDelete} {
		if c, b := call(s, "/t/"+dept+"?reason=Data+ganda", nil); c != http.StatusForbidden || b != `{"success":false,"error":"Insufficient permissions"}` {
			t.Errorf("%s = %d %s, want 403", name, c, b)
		}
	}
	reasonErr := `{"success":false,"error":"Alasan hapus wajib diisi (minimal 5 karakter)"}`
	for _, path := range []string{"/t/" + dept, "/t/" + dept + "?reason=abcd", "/t/" + dept + "?reason=%20%20ab%20%20"} {
		if c, b := call(&leaf, path, nil); c != http.StatusBadRequest || b != reasonErr {
			t.Fatalf("%s = %d %s, want 400 reason", path, c, b)
		}
	}
	if c, _ := call(&leaf, "/t/missing?reason=Data+ganda", nil); c != http.StatusNotFound || audits("missing") != "0" {
		t.Fatalf("a failed delete must not be audited (status %d)", c)
	}
	// Reason in the JSON body; the handler still reads the body.
	if c, b := call(&leaf, "/t/"+dept, map[string]any{"reason": "Departemen dobel"}); c != http.StatusOK {
		t.Fatalf("delete = %d %s", c, b)
	}
	if !strings.Contains(seenBody, "Departemen dobel") {
		t.Fatalf("handler body = %q, want it intact", seenBody)
	}
	var reason, name, actor string
	if err := tx.QueryRow(ctx, `SELECT reason, before->>'name', actor_id::text FROM audit.audit_log
		WHERE action = 'hris.departments.delete' AND entity_id = $1`, dept).Scan(&reason, &name, &actor); err != nil {
		t.Fatal(err)
	}
	if reason != "Departemen dobel" || name != "Dept Hapus" || actor != leaf.UserID {
		t.Fatalf("audit = %q %q %q", reason, name, actor)
	}
	// SelfAudited: only the permission; the handler owns reason and audit.
	if c, _ := call(&leaf, "/self/missing", nil); c != http.StatusNotFound {
		t.Fatalf("self-audited = %d, want the handler's 404", c)
	}
	if c, _ := call(&group, "/self/missing", nil); c != http.StatusForbidden {
		t.Fatalf("self-audited group = %d, want 403", c)
	}
}

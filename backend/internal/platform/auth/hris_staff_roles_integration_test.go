package auth_test

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/platform/testutil"
)

func roleGrants(t *testing.T, tx pgx.Tx, role string) map[string][]string {
	t.Helper()
	rows, err := tx.Query(context.Background(), `SELECT m.code, p.granted_actions
		FROM iam.role_menu_permissions p
		JOIN iam.roles r ON r.id = p.role_id
		JOIN iam.menus m ON m.id = p.menu_id
		WHERE r.code = $1 AND p.is_active AND m.deleted_at IS NULL`, role)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var code string
		var raw []byte
		if err := rows.Scan(&code, &raw); err != nil {
			t.Fatal(err)
		}
		var acts []string
		_ = json.Unmarshal(raw, &acts)
		slices.Sort(acts)
		out[code] = acts
	}
	return out
}

func under(code string, prefixes ...string) bool {
	for _, p := range prefixes {
		if code == p || strings.HasPrefix(code, p+".") {
			return true
		}
	}
	return false
}

func TestHrisStaffRolesDeriveFromAdminHris(t *testing.T) {
	db := testutil.DB(t)
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	src := roleGrants(t, tx, "admin_hris")
	if len(src) == 0 {
		t.Skip("admin_hris has no grants in this database")
	}
	if _, err := tx.Exec(ctx, `SELECT iam.sync_hris_staff_roles()`); err != nil {
		t.Fatal(err)
	}
	var menuActs = map[string][]string{}
	rows, _ := tx.Query(ctx, `SELECT code, coalesce(permission_context->'actions', '[]'::jsonb) FROM iam.menus WHERE deleted_at IS NULL`)
	for rows.Next() {
		var code string
		var raw []byte
		_ = rows.Scan(&code, &raw)
		var a []string
		_ = json.Unmarshal(raw, &a)
		menuActs[code] = a
	}
	rows.Close()

	want := func(excluded ...string) map[string][]string {
		out := map[string][]string{}
		for code, acts := range src {
			if under(code, excluded...) {
				continue
			}
			all := slices.Concat(acts, menuActs[code])
			slices.Sort(all)
			out[code] = slices.Compact(all)
		}
		return out
	}
	for role, excluded := range map[string][]string{
		"hris_manager":           nil,
		"hris_non_payroll":       {"hris.compensation"},
		"hris_non_rekrutmen_kpi": {"hris.recruitment", "hris.performance", "hris.insights.analytics"},
	} {
		got, exp := roleGrants(t, tx, role), want(excluded...)
		for code, acts := range exp {
			if !slices.Equal(got[code], acts) {
				t.Errorf("%s %s = %v, want %v", role, code, got[code], acts)
			}
		}
		for code := range got {
			if _, ok := exp[code]; !ok {
				t.Errorf("%s has extra grant %s", role, code)
			}
		}
	}
	// Pinjaman stays for Non-Payroll; Laporan HRIS stays for Non-Rekrutmen & KPI.
	if np := roleGrants(t, tx, "hris_non_payroll"); !slices.Contains(np["hris.kepegawaian.loans"], "approve") {
		t.Errorf("non-payroll loans = %v", np["hris.kepegawaian.loans"])
	}
	if nr := roleGrants(t, tx, "hris_non_rekrutmen_kpi"); nr["hris.insights.reports"] == nil {
		t.Error("non-rekrutmen lost Laporan HRIS")
	}

	// Re-running deactivates a grant the source no longer has.
	if _, err := tx.Exec(ctx, `UPDATE iam.role_menu_permissions p SET is_active = false
		FROM iam.roles r, iam.menus m WHERE r.id = p.role_id AND m.id = p.menu_id
		AND r.code = 'admin_hris' AND m.code = 'hris.kepegawaian.holidays'`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SELECT iam.sync_hris_staff_roles()`); err != nil {
		t.Fatal(err)
	}
	if roleGrants(t, tx, "hris_manager")["hris.kepegawaian.holidays"] != nil {
		t.Error("sync kept a grant admin_hris dropped")
	}
}

// With no active admin_hris grant the sync refuses instead of emptying the
// three HR roles (EPIC-057).
func TestHrisStaffRolesSyncRefusesEmptySource(t *testing.T) {
	db := testutil.DB(t)
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	before := roleGrants(t, tx, "hris_manager")
	if _, err := tx.Exec(ctx, `UPDATE iam.role_menu_permissions SET is_active = false
		WHERE role_id = (SELECT id FROM iam.roles WHERE code = 'admin_hris')`); err != nil {
		t.Fatal(err)
	}
	sp, err := tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = sp.Exec(ctx, `SELECT iam.sync_hris_staff_roles()`)
	if err == nil || !strings.Contains(err.Error(), "admin_hris tidak punya grant aktif") {
		t.Fatalf("sync with empty admin_hris = %v, want refusal", err)
	}
	_ = sp.Rollback(ctx)
	if after := roleGrants(t, tx, "hris_manager"); len(after) != len(before) {
		t.Fatalf("hris_manager grants %d -> %d", len(before), len(after))
	}
}

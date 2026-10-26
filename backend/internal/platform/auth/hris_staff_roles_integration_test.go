package auth_test

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/platform/iam"
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

func menuActions(t *testing.T, tx pgx.Tx) map[string][]string {
	t.Helper()
	rows, err := tx.Query(context.Background(), `SELECT code, coalesce(permission_context->'actions', '[]'::jsonb) FROM iam.menus WHERE deleted_at IS NULL`)
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
		var a []string
		_ = json.Unmarshal(raw, &a)
		out[code] = a
	}
	return out
}

func syncTx(t *testing.T) pgx.Tx {
	t.Helper()
	tx := testutil.Tx(t)
	// The sync refuses an empty source; admin_hris grants come from a seeder.
	if len(roleGrants(t, tx, "admin_hris")) == 0 {
		t.Skip("admin_hris has no grants in this database")
	}
	if _, err := tx.Exec(context.Background(), `SELECT iam.sync_hris_staff_roles()`); err != nil {
		t.Fatal(err)
	}
	return tx
}

// staffRoleSpec is the seeded mask of each derived role (EPIC-057 subtrees,
// EPIC-061: no "delete" on HRIS for the two staff roles; hris.access comes
// with Task 8 and is hidden from them; EPIC-068 R7: Saldo Cuti stays read
// only for the two staff roles).
var staffRoleSpec = map[string]struct {
	excluded []string
	noDelete bool
}{
	"hris_manager":           {nil, false},
	"hris_non_payroll":       {[]string{"hris.compensation", "hris.access"}, true},
	"hris_non_rekrutmen_kpi": {[]string{"hris.recruitment", "hris.performance", "hris.insights.analytics", "hris.access"}, true},
}

func TestHrisStaffRolesDeriveFromAdminHris(t *testing.T) {
	tx := syncTx(t)
	ctx := context.Background()
	src := roleGrants(t, tx, "admin_hris")
	menuActs := menuActions(t, tx)
	for role, spec := range staffRoleSpec {
		exp := map[string][]string{}
		for code, acts := range src {
			if under(code, spec.excluded...) {
				continue
			}
			all := slices.Concat(acts, menuActs[code])
			slices.Sort(all)
			all = slices.Compact(all)
			if spec.noDelete && under(code, "hris") {
				all = slices.DeleteFunc(all, func(a string) bool { return a == "delete" })
			}
			if spec.noDelete && code == "hris.workforce.leave-balances" {
				all = slices.DeleteFunc(all, func(a string) bool { return a == "update" || a == "import" || a == "export" })
			}
			exp[code] = all
		}
		got := roleGrants(t, tx, role)
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

// Every HRIS delete leaf offers "delete"; admin_hris, super_admin and
// hris_manager hold it, the two staff roles do not (Q-RA-2).
func TestHrisDeleteActionOnLeaves(t *testing.T) {
	tx := syncTx(t)
	menuActs := menuActions(t, tx)
	admin, super := roleGrants(t, tx, "admin_hris"), roleGrants(t, tx, "super_admin")
	m, np, nr := roleGrants(t, tx, "hris_manager"), roleGrants(t, tx, "hris_non_payroll"), roleGrants(t, tx, "hris_non_rekrutmen_kpi")
	for _, code := range iam.HrisDeleteLeaves {
		if !slices.Contains(menuActs[code], "delete") {
			t.Errorf("%s does not offer delete: %v", code, menuActs[code])
		}
		for role, g := range map[string]map[string][]string{"admin_hris": admin, "super_admin": super, "hris_manager": m} {
			if acts, ok := g[code]; ok && !slices.Contains(acts, "delete") {
				t.Errorf("%s %s = %v, want delete", role, code, acts)
			}
		}
		if slices.Contains(np[code], "delete") || slices.Contains(nr[code], "delete") {
			t.Errorf("%s: a staff role holds delete (np %v, nr %v)", code, np[code], nr[code])
		}
	}
	if !slices.Contains(m[iam.LeafAbsensi], "delete") {
		t.Errorf("hris_manager Absensi = %v, want delete", m[iam.LeafAbsensi])
	}
}

// A mask row (what "Hak Akses HRIS" writes) survives every sync: the most
// specific prefix wins and the parent group of an allowed leaf stays
// visible with read.
func TestHrisStaffRoleMasksSurviveSync(t *testing.T) {
	tx := syncTx(t)
	ctx := context.Background()
	if _, err := tx.Exec(ctx, `INSERT INTO iam.hris_staff_role_masks (role_code, menu_prefix, exclude_menu, excluded_actions) VALUES
		('hris_non_payroll', 'hris.kepegawaian.holidays', false, '{}'),
		('hris_non_payroll', 'hris.compensation.salary', false, '{delete}')`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := tx.Exec(ctx, `SELECT iam.sync_hris_staff_roles()`); err != nil {
			t.Fatal(err)
		}
	}
	np := roleGrants(t, tx, "hris_non_payroll")
	if !slices.Contains(np[iam.LeafHariLibur], "delete") {
		t.Errorf("holidays = %v, want delete from the leaf mask", np[iam.LeafHariLibur])
	}
	if np[iam.LeafSalary] == nil || slices.Contains(np[iam.LeafSalary], "delete") {
		t.Errorf("salary = %v, want granted without delete", np[iam.LeafSalary])
	}
	if !slices.Equal(np["hris.compensation"], []string{"read"}) {
		t.Errorf("hris.compensation = %v, want [read] as parent of an allowed leaf", np["hris.compensation"])
	}
	if np[iam.LeafRunPenggajian] != nil {
		t.Errorf("payroll = %v, want still hidden", np[iam.LeafRunPenggajian])
	}
	if slices.Contains(np[iam.LeafAbsensi], "delete") {
		t.Errorf("attendance = %v, want no delete", np[iam.LeafAbsensi])
	}
	// Hidden subtrees of the two staff roles keep "delete" excluded, so making
	// one visible later does not lose the reduction (longest prefix wins).
	var bad int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM iam.hris_staff_role_masks
		WHERE role_code IN ('hris_non_payroll', 'hris_non_rekrutmen_kpi')
		AND exclude_menu AND NOT 'delete' = ANY (excluded_actions)`).Scan(&bad); err != nil {
		t.Fatal(err)
	}
	if bad != 0 {
		t.Errorf("%d exclusion rows lack delete in excluded_actions", bad)
	}
	sp, err := tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = sp.Exec(ctx, `INSERT INTO iam.hris_staff_role_masks (role_code, menu_prefix, excluded_actions) VALUES ('hris_manager', 'hris.workforce', '{read}')`)
	if err == nil || !strings.Contains(err.Error(), "hris_staff_role_masks_read_kept") {
		t.Fatalf("masking read = %v, want the CHECK to refuse", err)
	}
	_ = sp.Rollback(ctx)
}

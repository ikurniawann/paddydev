package hris

import (
	"context"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/testutil"
)

const (
	approveMigrationFile = "../../../database/migrations/deltas/20261027090100_iam_hris_approve.sql"
	approveCheckFile     = "../../../database/sql/checks/2026-10-26_epic067_approve_access.sql"
)

// approveNamedRoles are the roles whose existing leaf grants the migration
// extends with "approve" (EPIC-067 R12).
var approveNamedRoles = []string{"super_admin", "admin", "hrd", "admin_hris"}

// hrisDerivedRoles are the roles iam.sync_hris_staff_roles derives from
// admin_hris.
var hrisDerivedRoles = []string{"hris_manager", "hris_non_payroll", "hris_non_rekrutmen_kpi"}

// grantRole gives role an active grant on menu (both created in the fixture
// transaction when missing) with exactly actions.
func (h *harness) grantRole(role, menu, actions string) {
	h.t.Helper()
	h.exec(`INSERT INTO iam.roles (code, name) VALUES ($1, $1)
		ON CONFLICT (code) DO UPDATE SET deleted_at = NULL, is_active = true`, role)
	h.exec(`INSERT INTO iam.menus (code, menu_name, module) VALUES ($1, $1, 'hris') ON CONFLICT (code) DO NOTHING`, menu)
	h.exec(`INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions, is_active)
		SELECT r.id, m.id, $3::jsonb, true FROM iam.roles r, iam.menus m WHERE r.code = $1 AND m.code = $2
		ON CONFLICT (role_id, menu_id) DO UPDATE SET granted_actions = EXCLUDED.granted_actions, is_active = true`,
		role, menu, actions)
}

// approvePreState rewinds the three leaves to their state before the
// migration, inside the fixture transaction: Absensi and Cuti offer
// read+delete, and every named role holds each leaf without "approve". A CI
// database (migrations only, no seeded roles) gets the same rows, so the
// migration body always has work to do.
func (h *harness) approvePreState() {
	h.t.Helper()
	for leaf, ctx := range map[string]string{
		iam.LeafAbsensi: `{"actions":["read","delete"]}`,
		iam.LeafCuti:    `{"actions":["read","delete"]}`,
		iam.LeafLembur:  `{"actions":["read","create","update","approve"]}`,
	} {
		h.exec(`INSERT INTO iam.menus (code, menu_name, module, permission_context) VALUES ($1, $1, 'hris', $2::jsonb)
			ON CONFLICT (code) DO UPDATE SET permission_context = EXCLUDED.permission_context, is_active = true, deleted_at = NULL`,
			leaf, ctx)
	}
	for _, role := range approveNamedRoles {
		h.grantRole(role, iam.LeafAbsensi, `["read","delete"]`)
		h.grantRole(role, iam.LeafCuti, `["read","delete"]`)
		h.grantRole(role, iam.LeafLembur, `["read","create","update"]`)
	}
	// A role outside R12 that reads Absensi and holds the Kehadiran group
	// with every action: neither the migration nor the group grant gives it
	// "approve" on a leaf.
	h.grantRole("hiring_manager", iam.LeafAbsensi, `["read"]`)
	h.grantRole("hiring_manager", "hris.workforce", `["read","create","update","delete","approve"]`)
}

func (h *harness) runApproveMigration() {
	h.t.Helper()
	body, err := os.ReadFile(approveMigrationFile)
	if err != nil {
		h.t.Fatal(err)
	}
	h.exec(string(body))
}

// account is a signed-in account with app role role, holding the IAM roles
// iamRoles through iam.user_roles (none: it resolves through iam.roles by
// role code).
func (h *harness) account(role string, iamRoles ...string) testutil.Staff {
	h.t.Helper()
	s := testutil.CreateStaff(h.t, testutil.StaffOptions{Role: role})
	// Roll the fixture transaction back before testutil deletes the account
	// its iam.user_roles rows reference (cleanups run last-in first-out).
	h.t.Cleanup(func() { _ = h.tx.Rollback(context.Background()) })
	for i, code := range iamRoles {
		h.exec(`INSERT INTO iam.user_roles (user_id, role_id, is_primary) SELECT $1, id, $3 FROM iam.roles WHERE code = $2`,
			s.UserID, code, i == 0)
	}
	return s
}

// approveCheckStatement is the check script's single statement without its
// BEGIN READ ONLY / ROLLBACK wrapper.
func approveCheckStatement(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(approveCheckFile)
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	const begin, end = "BEGIN READ ONLY;", "ROLLBACK;"
	from, to := strings.Index(src, begin), strings.LastIndex(src, end)
	if from < 0 || to < from {
		t.Fatalf("check script lacks %q … %q", begin, end)
	}
	stmt := strings.TrimSuffix(strings.TrimSpace(src[from+len(begin):to]), ";")
	code := regexp.MustCompile(`--[^\n]*`).ReplaceAllString(stmt, "")
	if strings.Contains(code, ";") {
		t.Fatal("check script must hold one statement between BEGIN READ ONLY and ROLLBACK")
	}
	if w := regexp.MustCompile(`(?i)\b(insert|update|delete|merge|create|alter|drop|truncate|grant|call|perform)\b`).FindString(code); w != "" {
		t.Fatalf("check script must only read, found %q", w)
	}
	return stmt
}

// TestApproveAccess (EPIC-067 R10/R12/R13/R14): the migration offers and
// grants "approve" on the Absensi, Cuti and Lembur leaves, the actor reads it
// per leaf, and the check script finds no account losing a decision.
func TestApproveAccess(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	leaves := iam.HrisApproveLeaves
	h.approvePreState()
	h.runApproveMigration()

	// (a) Menus offer "approve"; the named roles and the derived HR roles hold it.
	if got := h.scalar(`SELECT string_agg(code, ',' ORDER BY code) FROM iam.menus
		WHERE code = ANY($1) AND permission_context->'actions' ? 'approve'`, leaves); got != "hris.kepegawaian.overtime,hris.workforce.attendance,hris.workforce.leaves" {
		t.Errorf("menus offering approve = %q", got)
	}
	if got := h.scalar(`SELECT count(*)::text || ':' || coalesce(string_agg(r.code || ' ' || m.code, ',') FILTER (WHERE NOT p.granted_actions ? 'approve'), '')
		FROM iam.role_menu_permissions p JOIN iam.roles r ON r.id = p.role_id JOIN iam.menus m ON m.id = p.menu_id
		WHERE r.code = ANY($1) AND m.code = ANY($2)`, approveNamedRoles, leaves); got != "12:" {
		t.Errorf("named role grants = %q, want 12 rows all with approve", got)
	}
	for _, role := range hrisDerivedRoles {
		if got := h.scalar(`SELECT count(*)::text || ':' || coalesce(string_agg(m.code, ',') FILTER (WHERE NOT p.granted_actions ? 'approve'), '')
			FROM iam.role_menu_permissions p JOIN iam.roles r ON r.id = p.role_id JOIN iam.menus m ON m.id = p.menu_id
			WHERE r.code = $1 AND m.code = ANY($2) AND p.is_active`, role, leaves); got != "3:" {
			t.Errorf("%s leaf grants = %q, want the three leaves with approve", role, got)
		}
	}
	if got := h.scalar(`SELECT p.granted_actions::text FROM iam.role_menu_permissions p JOIN iam.roles r ON r.id = p.role_id
		JOIN iam.menus m ON m.id = p.menu_id WHERE r.code = 'hiring_manager' AND m.code = $1`, iam.LeafAbsensi); got != `["read"]` {
		t.Errorf("hiring_manager Absensi grant = %s, want untouched", got)
	}

	// (d) Running the body again adds nothing.
	h.runApproveMigration()
	if got := h.scalar(`SELECT count(*)::text FROM iam.role_menu_permissions p JOIN iam.menus m ON m.id = p.menu_id
		WHERE m.code = ANY($1) AND jsonb_typeof(p.granted_actions) = 'array'
		  AND (SELECT count(*) FROM jsonb_array_elements_text(p.granted_actions) a WHERE a = 'approve') > 1`, leaves); got != "0" {
		t.Errorf("grants with approve twice = %s", got)
	}
	if got := h.scalar(`SELECT count(*)::text FROM iam.menus m WHERE m.code = ANY($1)
		AND (SELECT count(*) FROM jsonb_array_elements_text(m.permission_context->'actions') a WHERE a = 'approve') <> 1`, leaves); got != "0" {
		t.Errorf("menus without exactly one approve = %s", got)
	}

	// (b) /api/hris/me/approve-access reads the grants per leaf.
	hrd := h.account("hrd")
	adminHris := h.account("hrd", "admin_hris")
	derived := h.account("hrd", "hris_non_payroll")
	super := h.account("super_admin")
	admin := h.account("admin")
	pos := h.account("pos")
	hiring := h.account("hiring_manager")
	for _, c := range []struct {
		name string
		s    testutil.Staff
		want bool
	}{
		{"hrd tanpa user_roles", hrd, true},
		{"admin_hris", adminHris, true},
		{"hris_non_payroll", derived, true},
		{"super_admin", super, true},
		{"admin", admin, true},
		{"pos", pos, false},
		{"hiring_manager", hiring, false},
	} {
		res := h.do(&c.s, http.MethodGet, "/api/hris/me/approve-access", nil)
		expect(t, res, http.StatusOK, "")
		if got := keys(t, res.raw); !slices.Equal(got, []string{"attendance", "leaves", "overtime"}) {
			t.Errorf("%s: keys %v", c.name, got)
		}
		for _, k := range []string{"attendance", "leaves", "overtime"} {
			if res.body[k] != c.want {
				t.Errorf("%s: %s = %v, want %v (%s)", c.name, k, res.body[k], c.want, res.raw)
			}
		}
	}
	expect(t, h.do(nil, http.MethodGet, "/api/hris/me/approve-access", nil), http.StatusUnauthorized, "Unauthorized")

	// (c) The check script: no fixture account loses a decision; an app-role
	// hrd account whose IAM role is not an HR role does.
	stmt := approveCheckStatement(t)
	list := func(kind string, ids ...string) []string {
		t.Helper()
		rows, err := h.tx.Query(ctx, `SELECT x.email FROM (`+stmt+`) x WHERE x.daftar = $1 AND x.user_id = ANY($2::uuid[]) ORDER BY x.email`, kind, ids)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var e string
			if err := rows.Scan(&e); err != nil {
				t.Fatal(err)
			}
			out = append(out, e)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return out
	}
	standard := []string{hrd.UserID, adminHris.UserID, derived.UserID, super.UserID, admin.UserID, pos.UserID, hiring.UserID}
	if lost := list("lost", standard...); len(lost) != 0 {
		t.Errorf("lost on the standard fixture = %v", lost)
	}
	if got := list("akun", standard...); len(got) != 5 {
		t.Errorf("akun rows = %v, want the five HR accounts", got)
	}
	h.grantRole("go_test_bukan_hr", "hris.performance.review", `["read"]`)
	notHR := h.account("hrd", "go_test_bukan_hr")
	if lost := list("lost", append(standard, notHR.UserID)...); !slices.Equal(lost, []string{notHR.Email}) {
		t.Errorf("lost = %v, want only %s", lost, notHR.Email)
	}
	var cols []string
	rows, err := h.tx.Query(ctx, `SELECT * FROM (`+stmt+`) x LIMIT 0`)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range rows.FieldDescriptions() {
		cols = append(cols, f.Name)
	}
	rows.Close()
	want := []string{"daftar", "user_id", "email", "role", "before_attendance", "after_attendance", "before_trip", "after_trip",
		"before_leave", "after_leave", "before_overtime", "after_overtime"}
	if !slices.Equal(cols, want) {
		t.Errorf("columns = %v, want %v", cols, want)
	}
}

func TestActorCanApprove(t *testing.T) {
	var none *Actor
	if none.CanApprove(iam.LeafCuti) {
		t.Error("nil actor approves")
	}
	a := &Actor{Approves: []string{iam.LeafAbsensi}}
	if !a.CanApprove(iam.LeafAbsensi) || a.CanApprove(iam.LeafCuti) || a.CanApprove("hris.workforce") {
		t.Errorf("CanApprove on %v wrong", a.Approves)
	}
}

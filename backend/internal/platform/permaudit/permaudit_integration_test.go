package permaudit_test

import (
	"context"
	"testing"

	"nuhabit/backend/internal/platform/permaudit"
	"nuhabit/backend/internal/platform/testutil"
)

func TestRecordWritesOnlyChangedMenus(t *testing.T) {
	actor := testutil.CreateStaff(t, testutil.StaffOptions{})
	tx := testutil.Tx(t)
	ctx := context.Background()
	var role string
	if err := tx.QueryRow(ctx, `INSERT INTO iam.roles (code, name) VALUES ($1, 'Audit Uji') RETURNING id::text`, "go_audit_"+testutil.RandomHex(3)).Scan(&role); err != nil {
		t.Fatal(err)
	}
	before := permaudit.Grants{role: {"a.menu": {"read"}, "b.menu": {"read", "update"}}}
	after := permaudit.Grants{role: {"a.menu": {"read"}, "b.menu": {"read"}, "c.menu": {"read"}}}
	n, err := permaudit.Record(ctx, tx, actor.UserID, permaudit.SourceRolePermissions, before, after)
	if err != nil || n != 2 {
		t.Fatalf("Record = %d, %v; want 2 rows", n, err)
	}
	var got string
	if err := tx.QueryRow(ctx, `SELECT string_agg(menu_code || ':' || coalesce(actions_before::text, 'null') || '>' || coalesce(actions_after::text, 'null'), ',' ORDER BY menu_code)
		FROM iam.permission_audit_logs WHERE role_id = $1 AND actor_id = $2`, role, actor.UserID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != `b.menu:["read", "update"]>["read"],c.menu:null>["read"]` {
		t.Fatalf("rows = %s", got)
	}
}

func TestSetUserRoleRecordsChange(t *testing.T) {
	actor := testutil.CreateStaff(t, testutil.StaffOptions{})
	target := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd"})
	tx := testutil.Tx(t)
	ctx := context.Background()
	for range 2 {
		if err := permaudit.SetUserRole(ctx, tx, target.UserID, "hris_manager", actor.UserID); err != nil {
			t.Fatal(err)
		}
	}
	codes, err := permaudit.UserRoleCodes(ctx, tx, target.UserID)
	if err != nil || len(codes) != 1 || codes[0] != "hris_manager" {
		t.Fatalf("roles = %v, %v", codes, err)
	}
	var rows string
	if err := tx.QueryRow(ctx, `SELECT count(*)::text || ':' || coalesce(max(array_to_string(roles_after, '+')), '') FROM iam.permission_audit_logs
		WHERE source = 'user_role' AND target_user_id = $1`, target.UserID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != "1:hris_manager" {
		t.Fatalf("audit = %s, want one row (the repeat is no change)", rows)
	}
}

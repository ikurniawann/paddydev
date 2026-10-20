package accounts_test

import (
	"context"
	"testing"
)

func TestUserRoleChangeIsAudited(t *testing.T) {
	e := settingsUsers(t)
	ctx := context.Background()
	var holding string
	if err := e.tx.QueryRow(ctx, `SELECT id::text FROM configuration.holdings ORDER BY created_at LIMIT 1`).Scan(&holding); err != nil {
		t.Fatal(err)
	}
	body := employeeBody(uniqueEmail())
	body["business_scope"], body["holding_id"] = "holding", holding
	body["is_access_app"], body["password"], body["role"] = true, "rahasia123", "hrd"
	code, created, raw := e.do("POST", "/api/users", body)
	if code != 201 {
		t.Fatalf("create = %d %s", code, raw)
	}
	id := created["data"].(map[string]any)["id"].(string)
	var userID string
	if err := e.tx.QueryRow(ctx, `SELECT user_id::text FROM hris.employees WHERE id = $1`, id).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if code, _, raw := e.do("PUT", "/api/users/"+id, map[string]any{"business_scope": "holding", "holding_id": holding,
		"is_access_app": true, "role": "finance_staff"}); code != 200 {
		t.Fatalf("role change = %d %s", code, raw)
	}
	var trail string
	if err := e.tx.QueryRow(ctx, `SELECT string_agg(array_to_string(roles_before, '+') || '>' || array_to_string(roles_after, '+'), ',' ORDER BY created_at)
		FROM iam.permission_audit_logs WHERE source = 'user_role' AND target_user_id = $1`, userID).Scan(&trail); err != nil {
		t.Fatal(err)
	}
	if trail != ">hrd,hrd>finance_staff" {
		t.Fatalf("user role trail = %q", trail)
	}
}

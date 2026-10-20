package testutil

import (
	"context"
	"fmt"
	"testing"
)

// IAMBaseline names the seeded IAM data a test relies on. CI applies the
// migrations only, so roles, grants and most menus (they come from the
// seeders) are absent there; a test that needs them calls RequireSeededIAM
// and is skipped, not failed, on such a database.
type IAMBaseline struct {
	GrantedRoles []string // roles that must exist with at least one active grant
	Roles        []string // roles that must exist
	Menus        []string // menu codes that must exist
	MinHrisMenus int      // minimum count of the "hris" menu and its "hris.*" descendants
}

// RequireSeededIAM skips t when the database lacks any part of b, naming
// what is missing. It never weakens a test on a seeded database.
func RequireSeededIAM(t testing.TB, b IAMBaseline) {
	t.Helper()
	db := DB(t)
	ctx := context.Background()
	var missing []string
	for _, code := range b.GrantedRoles {
		var n int
		if err := db.QueryRow(ctx, `SELECT count(*) FROM iam.role_menu_permissions p JOIN iam.roles r ON r.id = p.role_id
			WHERE r.code = $1 AND r.deleted_at IS NULL AND p.is_active`, code).Scan(&n); err != nil {
			t.Fatalf("check role %s grants: %v", code, err)
		}
		if n == 0 {
			missing = append(missing, "role "+code+" with grants")
		}
	}
	for _, code := range b.Roles {
		var ok bool
		if err := db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM iam.roles WHERE code = $1 AND deleted_at IS NULL)`, code).Scan(&ok); err != nil {
			t.Fatalf("check role %s: %v", code, err)
		}
		if !ok {
			missing = append(missing, "role "+code)
		}
	}
	for _, code := range b.Menus {
		var ok bool
		if err := db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM iam.menus WHERE code = $1 AND deleted_at IS NULL)`, code).Scan(&ok); err != nil {
			t.Fatalf("check menu %s: %v", code, err)
		}
		if !ok {
			missing = append(missing, "menu "+code)
		}
	}
	if b.MinHrisMenus > 0 {
		var n int
		if err := db.QueryRow(ctx, `SELECT count(*) FROM iam.menus WHERE (code = 'hris' OR code LIKE 'hris.%') AND deleted_at IS NULL`).Scan(&n); err != nil {
			t.Fatalf("count hris menus: %v", err)
		}
		if n < b.MinHrisMenus {
			missing = append(missing, fmt.Sprintf("%d HRIS menus (have %d)", b.MinHrisMenus, n))
		}
	}
	if len(missing) > 0 {
		t.Skipf("seeded IAM data missing in this database (run the seeders): %v", missing)
	}
}

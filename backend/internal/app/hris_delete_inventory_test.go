package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nuhabit/backend/internal/platform/testutil"
)

// EPIC-061 Review Focus 5: every DELETE route of the hris, payroll and
// recruitment modules refuses a staff member who holds every HRIS group and
// leaf menu with every action except "delete" — a new delete route that is
// not wrapped in hrisdel.Guard fails here. Inner guards (the 360 feedback
// admin check) pass for this staff, so only the leaf-delete guard can
// reject. The logbook stays ESS (ruling R2).
func TestEveryHrisDeleteNeedsLeafDelete(t *testing.T) {
	testutil.RequireSeededIAM(t, testutil.IAMBaseline{MinHrisMenus: 30})
	notDelete := []string{"read", "create", "update", "approve", "export", "import", "execute"}
	deps := testutil.Deps(t, nil)
	rows, err := deps.DB.Query(context.Background(),
		`SELECT code FROM iam.menus WHERE code = 'hris' OR code LIKE 'hris.%' OR code = 'settings.users'`)
	if err != nil {
		t.Fatal(err)
	}
	menus := map[string][]string{}
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			t.Fatal(err)
		}
		menus[code] = notDelete
	}
	rows.Close()
	if len(menus) < 30 {
		t.Fatalf("only %d HRIS menus in iam.menus", len(menus))
	}
	staff := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: menus})
	deps.Config.Modules = "hris,payroll,recruitment"
	h, _ := Handler(deps, time.Now())
	const id = "00000000-0000-4000-8000-0000000000ac"
	modules := map[string]bool{"hris": true, "payroll": true, "recruitment": true}
	paths := []string{"/api/hris/employees/documents/" + id}
	for _, rt := range Routes() {
		if rt.Method != http.MethodDelete || !modules[rt.Module] || rt.Path == "/api/hris/logbook" {
			continue
		}
		paths = append(paths, strings.NewReplacer("{id}", id, "{doc_id}", id).Replace(rt.Path))
	}
	if len(paths) < 20 {
		t.Fatalf("only %d delete routes found: %v", len(paths), paths)
	}
	for _, p := range paths {
		r := testutil.AsStaff(httptest.NewRequest(http.MethodDelete, p+"?reason=Uji%20inventaris&id="+id, nil), staff)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "Insufficient permissions") {
			t.Errorf("DELETE %s = %d %s, want 403 Insufficient permissions (a group grant must not delete)", p, rec.Code, rec.Body.String())
		}
	}
}

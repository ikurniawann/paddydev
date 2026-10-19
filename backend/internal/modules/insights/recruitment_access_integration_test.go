package insights

import (
	"net/http"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

func TestRecruitmentReadersKeepAnalyticsHolders(t *testing.T) {
	reportsOnly := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{
		"hris.insights": nil, "hris.insights.reports": nil}})
	analytics := testutil.CreateStaff(t, testutil.StaffOptions{Role: "direksi", Menus: map[string][]string{
		"hris.insights.analytics": nil}})
	recruiter := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hiring_manager", Menus: map[string][]string{
		"hris.recruitment": nil}})
	h := newTxHarness(t)
	for _, path := range []string{"/api/dashboard/stats", "/api/dashboard/funnel", "/api/analytics/overview", "/api/analytics/brands"} {
		if c := h.get(path, reportsOnly); c.status != http.StatusForbidden {
			t.Errorf("%s reports-only = %d, want 403", path, c.status)
		}
		if c := h.get(path, analytics); c.status != http.StatusOK {
			t.Errorf("%s analytics = %d, want 200 (%s)", path, c.status, c.raw)
		}
	}
	if c := h.get("/api/dashboard/stats", recruiter); c.status != http.StatusOK {
		t.Errorf("dashboard recruiter = %d", c.status)
	}
}

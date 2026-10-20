package recruitment

import (
	"net/http"
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/hrisdel"
	"nuhabit/backend/internal/platform/testutil"
)

func TestRecruitmentDeletesNeedLeafDelete(t *testing.T) {
	h := newHarness(t)
	group := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{
		"hris.recruitment": {"read", "create", "update", "delete"}}})
	const id = "00000000-0000-4000-8000-0000000000ad"
	routes := []struct{ path string }{
		{"/api/candidates/" + id},
		{"/api/candidates/" + id + "/cv-upload"},
		{"/api/psikotes/questions/" + id},
		{"/api/hris/job-openings/" + id},
	}
	for _, rt := range routes {
		expect(t, h.as(group, "DELETE", rt.path+"?reason=Data%20uji", nil), 403, "Insufficient permissions")
	}
	for _, leaf := range []string{"hris.recruitment.candidates", "hris.recruitment.psikotes", "hris.recruitment.job-portal"} {
		testutil.GrantMenu(t, h.hr.RoleID, leaf, []string{"read", "delete"})
	}
	for _, rt := range routes {
		expect(t, h.as(h.hr, "DELETE", rt.path, nil), 400, hrisdel.ErrReason)
		// A leaf holder passes the guard: never 403, never the reason error
		// and never 500 (a wrong Spec.Table or IDParam would surface here).
		c := h.as(h.hr, "DELETE", rt.path+"?reason=Data%20uji", nil)
		if c.status == http.StatusForbidden || c.status == http.StatusInternalServerError || strings.Contains(c.raw, hrisdel.ErrReason) {
			t.Errorf("leaf holder DELETE %s = %d %s", rt.path, c.status, c.raw)
		}
	}
}

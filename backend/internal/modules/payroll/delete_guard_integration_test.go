package payroll

import (
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/hrisdel"
)

func TestPayrollKinerjaDeletesNeedLeafDelete(t *testing.T) {
	e := setup(t)
	group := hrStaff(e, map[string][]string{
		"hris.compensation": {"read", "create", "update", "delete"}, "hris.performance": {"read", "create", "update", "delete"},
		"hris.performance.kpi-config": {"read", "update"},
	})
	const id = "00000000-0000-4000-8000-0000000000ad"
	for _, p := range []string{
		"/api/hris/payroll/" + id + "?",
		"/api/hris/payroll/" + id + "/inputs?employee_id=" + id + "&code=x&",
		"/api/hris/employee-salary/" + id + "?",
		"/api/hris/kpi/targets?id=" + id + "&",
		"/api/hris/feedback-cycles/" + id + "?",
		"/api/hris/feedback-assignments/" + id + "?",
		"/api/hris/feedback-responses/" + id + "?",
	} {
		wantErr(t, e.call(&group, "DELETE", p+"reason=Data%20uji", nil, 403), "Insufficient permissions")
		// A leaf holder passes the guard: never 403, never the reason error
		// and never 500 (a wrong Spec.Table or IDParam would surface here).
		rec := e.raw(&e.hr, "DELETE", p+"reason=Data%20uji", nil)
		if rec.Code == 403 || rec.Code == 500 || strings.Contains(rec.Body.String(), hrisdel.ErrReason) {
			t.Errorf("leaf holder DELETE %s = %d %s", p, rec.Code, rec.Body.String())
		}
	}
	wantErr(t, e.call(&e.hr, "DELETE", "/api/hris/payroll/x", nil, 400), hrisdel.ErrReason)
	wantErr(t, e.call(&e.hr, "DELETE", "/api/hris/kpi/targets?id=x&reason=abcd", nil, 400), hrisdel.ErrReason)
}

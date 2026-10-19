package hris

import (
	"net/http"
	"testing"

	"nuhabit/backend/internal/platform/iam"
)

func TestEmployeeAuditTrail(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", iam.HrisKepegawaian, false)
	e := h.employee(emp{name: "Audit Satu"})
	h.do(&hr, http.MethodPut, "/api/hris/employees/"+e+"/schedule-type", map[string]any{"schedule_type": "non_shift", "effective_from": "2026-10-06"})
	h.do(&hr, http.MethodPost, "/api/hris/employees/"+e+"/resign", map[string]any{"effective_date": "2026-10-07", "resign_type": "resign"})
	c := h.do(&hr, http.MethodGet, "/api/hris/employees/"+e+"/audit", nil)
	expect(t, c, http.StatusOK, "")
	list := c.list()
	// Satu transaksi test = created_at sama; cek isinya, bukan urutannya.
	actions := map[string]bool{}
	for _, row := range list {
		actions[row.(map[string]any)["action"].(string)] = true
	}
	if len(list) != 2 || !actions["employee.resign"] || !actions["employee.schedule_type"] {
		t.Fatalf("audit = %v", list)
	}
	ess, _ := h.staff("employee", nil, false)
	expect(t, h.do(&ess, http.MethodGet, "/api/hris/employees/"+e+"/audit", nil), http.StatusForbidden, "")
}

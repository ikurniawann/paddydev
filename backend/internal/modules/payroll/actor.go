package payroll

import (
	"net/http"

	"nuhabit/backend/internal/modules/payroll/domain"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
)

// actor is WorkforceActor (lib/hris/workforce-auth.ts): the signed-in
// account, its role and its linked employee record. approved_by and
// reviewer columns reference hris.employees, so writes use EmployeeID,
// never UserID.
type actor struct {
	UserID     string
	Role       string
	EmployeeID *string
	// PerformanceHR: an HR role holding the Kinerja menu (hris.performance,
	// beyond the employee-level Review and Tugas Departemen) sees and manages
	// every employee's KPI, reviews and department tasks.
	PerformanceHR bool
	// Payroll: the account holds the Penggajian menu (hris.compensation).
	Payroll bool
}

// requireActor is requireWorkforceActor: any session (a profile row is not
// required; without one the role is ""), else 401 "Unauthorized".
func (h *handler) requireActor(r *http.Request) (*actor, error) {
	su, err := h.auth.Session(r)
	if err != nil {
		return nil, err
	}
	if su == nil {
		return nil, httpx.Unauthorized("Unauthorized")
	}
	a := &actor{UserID: su.ID}
	if u, err := h.auth.CurrentUser(r); err == nil && u != nil {
		a.Role = u.Role
	}
	id, err := h.svc.ports.Employees.IDByUser(r.Context(), h.svc.db, su.ID)
	if err != nil {
		return nil, err
	}
	if id != "" {
		a.EmployeeID = &id
	}
	granted, err := h.auth.GrantedMenuCodes(r.Context(), su.ID, a.Role)
	if err != nil {
		return nil, err
	}
	a.PerformanceHR = domain.IsHR(a.Role) && iam.HasPerformanceHRMenu(granted)
	a.Payroll = iam.HasAnyMenuPrefix(granted, iam.HrisCompensation)
	return a, nil
}

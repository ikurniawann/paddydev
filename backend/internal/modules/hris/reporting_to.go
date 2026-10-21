package hris

import (
	"context"
	"net/http"

	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/validate"
)

// Kualitas data atasan langsung (EPIC-063 R9). Validasi aplikasi, tanpa
// CHECK DB: satu baris live masih menunjuk dirinya sendiri dan constraint
// akan memblokir update kolom lain pada baris itu.

const errSelfManager = "Atasan langsung tidak boleh karyawan itu sendiri"

// selfManager: reportingTo names employeeID itself (any case or hyphen form).
func selfManager(employeeID string, reportingTo *string) bool {
	if reportingTo == nil {
		return false
	}
	a, okA := validate.CanonicalUUID(employeeID)
	b, okB := validate.CanonicalUUID(*reportingTo)
	return okA && okB && a == b
}

// EmployeesWithoutManager lists active employees whose leave and overtime only
// HR can decide: reporting_to empty, pointing at themselves, or at an
// inactive employee.
func (s *Service) EmployeesWithoutManager(ctx context.Context) ([]*Row, error) {
	return s.repo.EmployeesWithoutManager(ctx)
}

func (s *store) EmployeesWithoutManager(ctx context.Context) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT e.id, e.nip, e.full_name, p.title AS job_title, d.name AS department,
		CASE WHEN e.reporting_to IS NULL THEN 'empty'
		     WHEN e.reporting_to = e.id THEN 'self'
		     ELSE 'inactive' END AS reason
		FROM hris.employees e
		LEFT JOIN hris.employees m ON m.id = e.reporting_to
		LEFT JOIN hris.positions p ON p.id = e.job_title_id
		LEFT JOIN hris.departments d ON d.id = e.department_id
		WHERE e.is_active AND (e.reporting_to IS NULL OR e.reporting_to = e.id OR m.is_active IS NOT TRUE)
		ORDER BY reason, e.full_name LIMIT 500`)
}

// GET /api/hris/employees/without-manager
func (h handlers) employeesWithoutManager(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	rows, err := h.svc.EmployeesWithoutManager(r.Context())
	if err != nil {
		return err
	}
	return ok(w, dataOf(rows))
}

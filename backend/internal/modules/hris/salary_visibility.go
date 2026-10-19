package hris

import (
	"net/http"

	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
)

// Salary figures on contracts and employment history belong to Penggajian
// (hris.compensation): Kepegawaian accounts without it manage contracts but
// never see or set the amounts (keputusan user 19-10-2026, EPIC-057).
var (
	contractSalaryKeys = []string{"base_salary", "allowances", "compensation_amount"}
	historySalaryKeys  = []string{"prev_salary", "new_salary"}
)

const contractDocumentPayrollOnly = "Dokumen kontrak memuat gaji — hanya untuk akun dengan akses Penggajian"

// canSeeSalary: an account without a profile row (u nil) never sees salaries.
func (h handlers) canSeeSalary(r *http.Request, u *auth.User) (bool, error) {
	if u == nil {
		return false, nil
	}
	return h.auth.HasMenuPrefix(r.Context(), u, iam.HrisCompensation...)
}

// requireSalaryAccess guards the contract PDF and signed scan, which print the wage.
func (h handlers) requireSalaryAccess(r *http.Request, u *auth.User) error {
	ok, err := h.canSeeSalary(r, u)
	if err != nil {
		return err
	}
	if !ok {
		return httpx.Forbidden(contractDocumentPayrollOnly)
	}
	return nil
}

func hideKeys(rows []*Row, keys []string) {
	for _, row := range rows {
		if row == nil {
			continue
		}
		for _, k := range keys {
			row.Del(k)
		}
	}
}

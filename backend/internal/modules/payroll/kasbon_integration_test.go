package payroll

import "testing"

// THP kasbon = pokok + transport + makan + jabatan dari semua struktur aktif.
func kasbonWorker(e *env, status string) string {
	e.t.Helper()
	worker := e.employee("Peminjam Kasbon", e.staff.UserID, true)
	e.exec(`UPDATE hris.employees SET employment_status = $2 WHERE id = $1`, worker, status)
	// Fake HRIS port: brief status ikut berubah (createLoan membaca status lewat port).
	b := e.emps.briefs[worker]
	b.EmploymentStatus = status
	e.emps.briefs[worker] = b
	e.exec(`INSERT INTO hris.employee_salary (employee_id, company_id, base_salary, transport_allowance, meal_allowance,
		position_allowance, ptkp_status, effective_date) VALUES ($1, $2, 3000000, 500000, 500000, 1000000, 'TK/0', '2026-01-01')`,
		worker, e.co)
	return worker
}

func TestKasbonCreateRules(t *testing.T) {
	e := setup(t)
	kasbonWorker(e, "contract") // THP 5.000.000 → plafon 2.500.000
	post := func(body map[string]any, status int) map[string]any {
		return e.call(&e.staff, "POST", "/api/hris/loans", body, status)
	}
	wantErr(t, post(map[string]any{"loan_type": "emergency", "principal_amount": 500000, "tenor_months": 1}, 400),
		"Pinjaman Darurat sudah tidak tersedia")
	wantErr(t, post(map[string]any{"loan_type": "kasbon", "principal_amount": 1000000, "tenor_months": 3}, 400),
		"Tenor kasbon hanya 1 atau 2 bulan")
	wantErr(t, post(map[string]any{"loan_type": "kasbon", "principal_amount": 2600000, "tenor_months": 2}, 400),
		"Kasbon maksimal 50% THP (maks Rp 2.500.000)")
	created := post(map[string]any{"loan_type": "kasbon", "principal_amount": 2500000, "tenor_months": 2}, 200)
	if d := created["data"].(map[string]any); d["monthly_installment"] != "1250000.00" || d["status"] != "pending" {
		t.Fatalf("kasbon 2 x 50%%: %v", d)
	}
}

func TestKasbonBlockedForProbation(t *testing.T) {
	e := setup(t)
	kasbonWorker(e, "probation")
	wantErr(t, e.call(&e.staff, "POST", "/api/hris/loans", map[string]any{"loan_type": "kasbon",
		"principal_amount": 500000, "tenor_months": 1}, 400),
		"Karyawan berstatus Probation/Freelance/Magang tidak dapat mengajukan kasbon")
}

// Approving re-checks the kasbon cap against the current THP; an approver
// without the Penggajian menu does not see the cap amount.
func TestKasbonApproveRechecksCap(t *testing.T) {
	e := setup(t)
	loansHR := hrStaff(e, map[string][]string{"hris.kepegawaian.loans": {"read", "create", "update", "approve"}})
	e.employee("HR Pinjaman", loansHR.UserID, true)
	worker := kasbonWorker(e, "contract") // THP 5.000.000
	loan := e.call(&e.staff, "POST", "/api/hris/loans", map[string]any{
		"loan_type": "kasbon", "principal_amount": 2500000, "tenor_months": 2}, 200)["data"].(map[string]any)
	e.exec(`UPDATE hris.employee_salary SET base_salary = 1000000 WHERE employee_id = $1`, worker)
	wantErr(t, e.call(&loansHR, "POST", "/api/hris/loans/"+loan["id"].(string)+"/approve", map[string]any{"approved": true}, 400),
		"Kasbon maksimal 50% THP")
}

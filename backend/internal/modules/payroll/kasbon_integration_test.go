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

// Approving (HR step) re-checks the kasbon cap against the current THP. The
// kasbon HR step needs the Penggajian menu (R6), which also shows the cap amount.
func TestKasbonApproveRechecksCap(t *testing.T) {
	e := setup(t)
	e.employee("HR Penggajian", e.hr.UserID, true)
	worker := kasbonWorker(e, "contract") // THP 5.000.000
	loan := e.call(&e.staff, "POST", "/api/hris/loans", map[string]any{
		"loan_type": "kasbon", "principal_amount": 2500000, "tenor_months": 2}, 200)["data"].(map[string]any)
	e.exec(`UPDATE hris.employee_salary SET base_salary = 1000000 WHERE employee_id = $1`, worker) // THP 3.000.000
	wantErr(t, e.call(&e.hr, "POST", "/api/hris/loans/"+loan["id"].(string)+"/approve", map[string]any{"approved": true}, 400),
		"Kasbon maksimal 50% THP (maks Rp 1.500.000)")
}

// Rantai: pemohon → SPV → Manager. Approver tahap 1 = Manager (bukan SPV).
func TestKasbonTwoStepApproval(t *testing.T) {
	e := setup(t)
	// Akun manager tanpa menu HRIS: hanya bisa memutus sebagai approver manager.
	mgrStaff := hrStaff(e, map[string][]string{"go_test_none": nil})
	mgr := e.employee("Manager Dept", mgrStaff.UserID, true)
	spv := e.employee("Supervisor", "", true)
	worker := kasbonWorker(e, "contract")
	e.exec(`UPDATE hris.employees SET reporting_to = $2 WHERE id = $1`, worker, spv)
	e.exec(`UPDATE hris.employees SET reporting_to = $2 WHERE id = $1`, spv, mgr)
	e.setLevel(mgr, "Manager")
	e.setLevel(spv, "Supervisor")

	loan := e.call(&e.staff, "POST", "/api/hris/loans", map[string]any{"loan_type": "kasbon",
		"principal_amount": 1000000, "tenor_months": 2}, 200)["data"].(map[string]any)
	id := loan["id"].(string)
	if loan["manager_approver_id"] != mgr {
		t.Fatalf("manager_approver_id = %v, want %s", loan["manager_approver_id"], mgr)
	}

	// HR sebelum manager → ditolak.
	wantErr(t, e.call(&e.hr, "POST", "/api/hris/loans/"+id+"/approve", map[string]any{"approved": true}, 400),
		"Kasbon menunggu persetujuan Manager Departemen")
	// Antrean manager memuat kasbon ini, tanpa angka gaji.
	queue := e.call(&mgrStaff, "GET", "/api/hris/loans?scope=approvals", nil, 200)["data"].([]any)
	if len(queue) != 1 || queue[0].(map[string]any)["id"] != id {
		t.Fatalf("antrean manager = %v", queue)
	}
	// EPIC-072 R8: catatan ikut tampil di detail kasbon tim (kolom non-gaji).
	if _, ok := queue[0].(map[string]any)["notes"]; !ok {
		t.Fatalf("antrean manager tanpa notes: %v", queue[0])
	}
	// Manager menyetujui tahap 1.
	step1 := e.call(&mgrStaff, "POST", "/api/hris/loans/"+id+"/approve", map[string]any{"approved": true}, 200)
	if d := step1["data"].(map[string]any); d["status"] != "manager_approved" || d["manager_approved_by"] != mgr {
		t.Fatalf("tahap 1: %v", d)
	}
	// Manager tidak bisa memutus tahap HR.
	e.call(&mgrStaff, "POST", "/api/hris/loans/"+id+"/approve", map[string]any{"approved": true}, 403)
	// HR Penggajian menyetujui tahap 2.
	step2 := e.call(&e.hr, "POST", "/api/hris/loans/"+id+"/approve", map[string]any{"approved": true}, 200)
	if step2["data"].(map[string]any)["status"] != "approved" {
		t.Fatalf("tahap 2: %v", step2)
	}
}

// Tanpa Manager di rantai (atau reporting_to diri sendiri): langsung tahap HR.
func TestKasbonWithoutManagerGoesToHR(t *testing.T) {
	e := setup(t)
	worker := kasbonWorker(e, "contract")
	e.exec(`UPDATE hris.employees SET reporting_to = id WHERE id = $1`, worker) // siklus diri sendiri
	loan := e.call(&e.staff, "POST", "/api/hris/loans", map[string]any{"loan_type": "kasbon",
		"principal_amount": 1000000, "tenor_months": 1}, 200)["data"].(map[string]any)
	if loan["manager_approver_id"] != nil {
		t.Fatalf("approver = %v, want nil", loan["manager_approver_id"])
	}
	e.call(&e.hr, "POST", "/api/hris/loans/"+loan["id"].(string)+"/approve", map[string]any{"approved": true}, 200)
}

// Tahap HR kasbon butuh menu Penggajian (MOM AP-05; R6).
func TestKasbonHRStepNeedsPayrollMenu(t *testing.T) {
	e := setup(t)
	loansOnly := hrStaff(e, map[string][]string{"hris.kepegawaian.loans": {"read", "create", "update", "approve"}})
	kasbonWorker(e, "contract")
	loan := e.call(&e.staff, "POST", "/api/hris/loans", map[string]any{"loan_type": "kasbon",
		"principal_amount": 1000000, "tenor_months": 1}, 200)["data"].(map[string]any)
	wantErr(t, e.call(&loansOnly, "POST", "/api/hris/loans/"+loan["id"].(string)+"/approve", map[string]any{"approved": true}, 403),
		"Persetujuan kasbon oleh HR butuh akses Penggajian")
}

// Manager rejection ends the kasbon; a manager_approved kasbon still counts as
// an active loan, so the employee cannot file another one meanwhile.
func TestKasbonManagerRejectAndActiveCount(t *testing.T) {
	e := setup(t)
	mgrStaff := hrStaff(e, map[string][]string{"go_test_none": nil})
	mgr := e.employee("Manager Dept", mgrStaff.UserID, true)
	worker := kasbonWorker(e, "contract")
	e.exec(`UPDATE hris.employees SET reporting_to = $2 WHERE id = $1`, worker, mgr)
	e.setLevel(mgr, "Manager")
	file := func() string {
		return e.call(&e.staff, "POST", "/api/hris/loans", map[string]any{"loan_type": "kasbon",
			"principal_amount": 500000, "tenor_months": 1}, 200)["data"].(map[string]any)["id"].(string)
	}
	id := file()
	rej := e.call(&mgrStaff, "POST", "/api/hris/loans/"+id+"/approve",
		map[string]any{"approved": false, "rejection_reason": "Belum waktunya"}, 200)["data"].(map[string]any)
	if rej["status"] != "rejected" || rej["rejection_reason"] != "Belum waktunya" {
		t.Fatalf("reject: %v", rej)
	}
	id = file()
	e.call(&mgrStaff, "POST", "/api/hris/loans/"+id+"/approve", map[string]any{"approved": true}, 200)
	e.call(&e.staff, "POST", "/api/hris/loans", map[string]any{"loan_type": "kasbon",
		"principal_amount": 500000, "tenor_months": 1}, 400)
}

// HR boleh MENOLAK kasbon yang masih menunggu manager (mis. manager sudah tidak
// aktif / tidak merespons) — hanya approve yang diblok sampai tahap 1 selesai.
func TestKasbonHRCanRejectWhileWaitingManager(t *testing.T) {
	e := setup(t)
	mgrStaff := hrStaff(e, map[string][]string{"go_test_none": nil})
	mgr := e.employee("Manager Dept", mgrStaff.UserID, true)
	worker := kasbonWorker(e, "contract")
	e.exec(`UPDATE hris.employees SET reporting_to = $2 WHERE id = $1`, worker, mgr)
	e.setLevel(mgr, "Manager")
	loan := e.call(&e.staff, "POST", "/api/hris/loans", map[string]any{"loan_type": "kasbon",
		"principal_amount": 500000, "tenor_months": 1}, 200)["data"].(map[string]any)
	id := loan["id"].(string)
	wantErr(t, e.call(&e.hr, "POST", "/api/hris/loans/"+id+"/approve", map[string]any{"approved": true}, 400),
		"Kasbon menunggu persetujuan Manager Departemen")
	rej := e.call(&e.hr, "POST", "/api/hris/loans/"+id+"/approve",
		map[string]any{"approved": false, "rejection_reason": "Manager tidak merespons"}, 200)["data"].(map[string]any)
	if rej["status"] != "rejected" || rej["rejection_reason"] != "Manager tidak merespons" {
		t.Fatalf("HR reject saat menunggu manager: %v", rej)
	}
}

// THP kasbon menjumlah SEMUA struktur gaji aktif (dua entitas pembayar), dan
// pesan plafon 50% mengikuti THP total.
func TestKasbonTHPSumsAllActiveSalaryRows(t *testing.T) {
	e := setup(t)
	kasbonWorker(e, "contract") // baris 1: THP 5.000.000 di e.co
	worker := e.emps.order[len(e.emps.order)-1]
	co2, _ := e.entity("PT Kedua")
	e.exec(`INSERT INTO hris.employee_salary (employee_id, company_id, base_salary, transport_allowance, meal_allowance,
		position_allowance, ptkp_status, effective_date) VALUES ($1, $2, 2000000, 500000, 500000, 0, 'TK/0', '2026-01-01')`,
		worker, co2) // baris 2: THP 3.000.000 → total 8.000.000, plafon 4.000.000
	post := func(amount int, status int) map[string]any {
		return e.call(&e.staff, "POST", "/api/hris/loans", map[string]any{"loan_type": "kasbon",
			"principal_amount": amount, "tenor_months": 2}, status)
	}
	wantErr(t, post(4100000, 400), "Kasbon maksimal 50% THP (maks Rp 4.000.000)")
	post(4000000, 200)
}

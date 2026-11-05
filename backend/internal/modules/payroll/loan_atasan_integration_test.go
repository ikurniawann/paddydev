package payroll

import (
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

// EPIC-091 (laporan 05-11 + ralat user): kasbon/pinjaman disetujui atasan
// terdekat minimal SPV, lalu Manager (bila ada di atasnya), lalu HR.

// spvChain gives worker the chain worker → SPV → Manager (both with accounts).
func (e *env) spvChain(worker string) (spvStaff, mgrStaff testutil.Staff, spv, mgr string) {
	e.t.Helper()
	spvStaff = e.plainStaff()
	spv = e.employee("SPV Toko", spvStaff.UserID, true)
	e.setLevel(spv, "Supervisor")
	mgrStaff = e.plainStaff()
	mgr = e.employee("Manager Area", mgrStaff.UserID, true)
	e.setLevel(mgr, "Manager")
	e.exec(`UPDATE hris.employees SET reporting_to = $2 WHERE id = $1`, worker, spv)
	e.exec(`UPDATE hris.employees SET reporting_to = $2 WHERE id = $1`, spv, mgr)
	return spvStaff, mgrStaff, spv, mgr
}

func (e *env) bell(userID, title, link string) (n int, msg string) {
	e.t.Helper()
	e.scalar(&n, `SELECT count(*) FROM public.notifications WHERE user_id = $1 AND type = 'approval' AND title = $2 AND link = $3`,
		userID, title, link)
	e.scalar(&msg, `SELECT coalesce(max(message), '') FROM public.notifications WHERE user_id = $1 AND title = $2`, userID, title)
	return n, msg
}

func TestLoanThreeSteps(t *testing.T) {
	e := setup(t)
	worker := kasbonWorker(e, "permanent")
	spvStaff, mgrStaff, spv, mgr := e.spvChain(worker)
	stranger := e.plainStaff()
	e.employee("Rekan", stranger.UserID, true)
	const title = "Pengajuan Kasbon: Peminjam Kasbon"

	loan := e.loan(&e.staff, map[string]any{"loan_type": "kasbon", "principal_amount": 1000000, "tenor_months": 2}, 200)["data"].(map[string]any)
	id := loan["id"].(string)
	if loan["supervisor_approver_id"] != spv || loan["manager_approver_id"] != mgr {
		t.Fatalf("penyetuju = SPV %v, Manager %v", loan["supervisor_approver_id"], loan["manager_approver_id"])
	}
	queue := func(s *testutil.Staff) int {
		t.Helper()
		return len(e.call(s, "GET", "/api/hris/loans?scope=approvals", nil, 200)["data"].([]any))
	}
	if queue(&spvStaff) != 1 || queue(&mgrStaff) != 0 {
		t.Fatalf("antrean awal SPV/Manager = %d/%d", queue(&spvStaff), queue(&mgrStaff))
	}
	if n, msg := e.bell(spvStaff.UserID, title, "/dashboard/me/pinjaman"); n != 1 || !strings.Contains(msg, "Menunggu persetujuan Anda") {
		t.Fatalf("lonceng SPV = %d %q", n, msg)
	}
	if n, _ := e.bell(mgrStaff.UserID, title, "/dashboard/me/pinjaman"); n != 0 {
		t.Fatalf("Manager dilonceng sebelum SPV setuju: %d", n)
	}
	// SPV dan Manager membaca lampiran; orang lain tidak.
	doc := loan["attachments"].([]any)[0].(string)
	for _, s := range []*testutil.Staff{&spvStaff, &mgrStaff} {
		if rec := e.raw(s, "GET", "/api/hris/loans/attachment/"+doc, nil); rec.Code != 200 {
			t.Fatalf("baca lampiran = %d", rec.Code)
		}
	}
	if rec := e.raw(&stranger, "GET", "/api/hris/loans/attachment/"+doc, nil); rec.Code != 403 {
		t.Fatalf("orang lain baca lampiran = %d", rec.Code)
	}

	decide := func(s *testutil.Staff, approved bool, status int) map[string]any {
		t.Helper()
		return e.call(s, "POST", "/api/hris/loans/"+id+"/approve", map[string]any{"approved": approved, "rejection_reason": "x"}, status)
	}
	decide(&mgrStaff, true, 403) // Manager belum gilirannya
	ok := decide(&spvStaff, true, 200)
	if ok["message"] != "Kasbon disetujui SPV — menunggu persetujuan Manager" {
		t.Fatalf("SPV setuju = %v", ok)
	}
	var status string
	e.scalar(&status, `SELECT status || '/' || (supervisor_approved_by::text = $2)::text FROM hris.loans WHERE id = $1`, id, spv)
	if status != "pending/true" {
		t.Fatalf("setelah SPV = %s", status)
	}
	if n, msg := e.bell(mgrStaff.UserID, title, "/dashboard/me/pinjaman"); n != 1 || !strings.Contains(msg, "Disetujui SPV SPV Toko") {
		t.Fatalf("lonceng Manager setelah SPV = %d %q", n, msg)
	}
	if queue(&spvStaff) != 0 || queue(&mgrStaff) != 1 {
		t.Fatalf("antrean setelah SPV SPV/Manager = %d/%d", queue(&spvStaff), queue(&mgrStaff))
	}
	decide(&spvStaff, true, 403) // SPV tidak memutus dua kali
	decide(&mgrStaff, true, 200)
	e.scalar(&status, `SELECT status FROM hris.loans WHERE id = $1`, id)
	if status != "manager_approved" {
		t.Fatalf("setelah Manager = %s", status)
	}
}

// SPV menolak: selesai, Manager tidak dilonceng.
func TestLoanSupervisorRejects(t *testing.T) {
	e := setup(t)
	worker := kasbonWorker(e, "permanent")
	spvStaff, mgrStaff, _, _ := e.spvChain(worker)
	id := e.loan(&e.staff, map[string]any{"loan_type": "kasbon", "principal_amount": 1000000, "tenor_months": 2}, 200)["data"].(map[string]any)["id"].(string)
	e.call(&spvStaff, "POST", "/api/hris/loans/"+id+"/approve", map[string]any{"approved": false, "rejection_reason": "Belum perlu"}, 200)
	var status string
	e.scalar(&status, `SELECT status FROM hris.loans WHERE id = $1`, id)
	if status != "rejected" {
		t.Fatalf("status = %s", status)
	}
	if n, _ := e.bell(mgrStaff.UserID, "Pengajuan Kasbon: Peminjam Kasbon", "/dashboard/me/pinjaman"); n != 0 {
		t.Fatalf("Manager dilonceng setelah SPV menolak: %d", n)
	}
}

// Penyetuju terdekat sudah Manager: dua tahap, satu lonceng.
func TestLoanNotifyManagerOnce(t *testing.T) {
	e := setup(t)
	worker := kasbonWorker(e, "permanent")
	mgrStaff, _ := e.loanManager(worker)
	loan := e.loan(&e.staff, map[string]any{"loan_type": "kasbon", "principal_amount": 1000000, "tenor_months": 2}, 200)["data"].(map[string]any)
	if loan["supervisor_approver_id"] != nil {
		t.Fatalf("tahap SPV tanpa SPV: %v", loan["supervisor_approver_id"])
	}
	var n int
	e.scalar(&n, `SELECT count(*) FROM public.notifications WHERE user_id = $1 AND title LIKE 'Pengajuan Kasbon:%'`, mgrStaff.UserID)
	if n != 1 {
		t.Fatalf("lonceng manager = %d, want 1", n)
	}
}

// Atasan langsung di bawah SPV (Store Leader berlevel Staff) dilewati sebagai
// penyetuju, tetapi tetap dilonceng untuk melihat detail.
func TestLoanStaffLevelAtasanSkipped(t *testing.T) {
	e := setup(t)
	worker := kasbonWorker(e, "permanent")
	leaderStaff := e.plainStaff()
	leader := e.employee("Store Leader", leaderStaff.UserID, true)
	e.setLevel(leader, "Staff")
	spvStaff, _, spv, _ := e.spvChain(leader)
	e.exec(`UPDATE hris.employees SET reporting_to = $2 WHERE id = $1`, worker, leader)
	loan := e.loan(&e.staff, map[string]any{"loan_type": "kasbon", "principal_amount": 1000000, "tenor_months": 2}, 200)["data"].(map[string]any)
	if loan["supervisor_approver_id"] != spv {
		t.Fatalf("SPV = %v, want %s", loan["supervisor_approver_id"], spv)
	}
	if n, _ := e.bell(spvStaff.UserID, "Pengajuan Kasbon: Peminjam Kasbon", "/dashboard/me/pinjaman"); n != 1 {
		t.Fatalf("lonceng SPV = %d", n)
	}
	if n, msg := e.bell(leaderStaff.UserID, "Pengajuan Kasbon: Peminjam Kasbon", "/dashboard/me/persetujuan-tim?tab=riwayat"); n != 1 ||
		!strings.Contains(msg, "SPV Toko") {
		t.Fatalf("lonceng Store Leader = %d %q", n, msg)
	}
}

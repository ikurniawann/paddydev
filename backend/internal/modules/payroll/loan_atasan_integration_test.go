package payroll

import (
	"strings"
	"testing"
)

// EPIC-091 (laporan 05-11: kasbon/pinjaman tidak tampil di atasan SPV):
// Manager tetap penyetuju tahap 1; atasan langsung (SPV) dapat lonceng dan
// boleh membaca lampirannya.
func TestLoanDirectAtasanNotifiedAndReads(t *testing.T) {
	e := setup(t)
	worker := kasbonWorker(e, "permanent")
	spvStaff := e.plainStaff()
	spv := e.employee("SPV Toko", spvStaff.UserID, true)
	e.setLevel(spv, "Supervisor")
	mgrStaff := e.plainStaff()
	mgr := e.employee("Manager Area", mgrStaff.UserID, true)
	e.setLevel(mgr, "Manager")
	e.exec(`UPDATE hris.employees SET reporting_to = $2 WHERE id = $1`, worker, spv)
	e.exec(`UPDATE hris.employees SET reporting_to = $2 WHERE id = $1`, spv, mgr)
	stranger := e.plainStaff()
	e.employee("Rekan", stranger.UserID, true)

	loan := e.loan(&e.staff, map[string]any{"loan_type": "kasbon", "principal_amount": 1000000, "tenor_months": 2}, 200)["data"].(map[string]any)
	if loan["manager_approver_id"] != mgr {
		t.Fatalf("approver = %v, want Manager", loan["manager_approver_id"])
	}
	bell := func(userID, title, link string) (n int, msg string) {
		t.Helper()
		e.scalar(&n, `SELECT count(*) FROM public.notifications WHERE user_id = $1 AND type = 'approval' AND title = $2 AND link = $3`,
			userID, title, link)
		e.scalar(&msg, `SELECT coalesce(max(message), '') FROM public.notifications WHERE user_id = $1 AND title = $2`, userID, title)
		return n, msg
	}
	if n, msg := bell(mgrStaff.UserID, "Pengajuan Kasbon: Peminjam Kasbon", "/dashboard/me/pinjaman"); n != 1 ||
		!strings.Contains(msg, "Menunggu persetujuan Anda") {
		t.Fatalf("lonceng manager = %d %q", n, msg)
	}
	if n, msg := bell(spvStaff.UserID, "Pengajuan Kasbon: Peminjam Kasbon", "/dashboard/me/persetujuan-tim?tab=riwayat"); n != 1 ||
		!strings.Contains(msg, "Manager Area") {
		t.Fatalf("lonceng SPV = %d %q", n, msg)
	}
	doc := loan["attachments"].([]any)[0].(string)
	if rec := e.raw(&spvStaff, "GET", "/api/hris/loans/attachment/"+doc, nil); rec.Code != 200 {
		t.Fatalf("SPV baca lampiran = %d %s", rec.Code, rec.Body)
	}
	if rec := e.raw(&stranger, "GET", "/api/hris/loans/attachment/"+doc, nil); rec.Code != 403 {
		t.Fatalf("orang lain baca lampiran = %d", rec.Code)
	}
}

// Atasan langsung = Manager penyetuju: satu lonceng saja, bukan dua.
func TestLoanNotifyManagerOnce(t *testing.T) {
	e := setup(t)
	worker := kasbonWorker(e, "permanent")
	mgrStaff, _ := e.loanManager(worker)
	e.loan(&e.staff, map[string]any{"loan_type": "kasbon", "principal_amount": 1000000, "tenor_months": 2}, 200)
	var n int
	e.scalar(&n, `SELECT count(*) FROM public.notifications WHERE user_id = $1 AND title LIKE 'Pengajuan Kasbon:%'`, mgrStaff.UserID)
	if n != 1 {
		t.Fatalf("lonceng manager = %d, want 1", n)
	}
}

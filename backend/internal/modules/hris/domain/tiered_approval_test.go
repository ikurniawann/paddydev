package domain

import "testing"

// EPIC-092: approval berjenjang SPV → Manager, HR boleh mewakili tahap berjalan.
func TestApprovalStageCurrent(t *testing.T) {
	spv, mgr := "spv", "mgr"
	cases := []struct {
		name      string
		s         ApprovalStage
		want      *string
		wantFinal bool
	}{
		{"SPV lalu Manager: tahap SPV", ApprovalStage{SPV: &spv, Manager: &mgr}, &spv, false},
		{"SPV sudah: tahap Manager", ApprovalStage{SPV: &spv, Manager: &mgr, SPVDone: true}, &mgr, true},
		{"tanpa Manager", ApprovalStage{SPV: &spv}, &spv, true},
		{"SPV = Manager", ApprovalStage{SPV: &mgr, Manager: &mgr}, &mgr, true},
		{"tanpa SPV", ApprovalStage{Manager: &mgr}, &mgr, true},
		{"tanpa atasan", ApprovalStage{}, nil, true},
	}
	for _, c := range cases {
		got, final := c.s.Current()
		if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) || final != c.wantFinal {
			t.Errorf("%s: got %v/%v, want %v/%v", c.name, got, final, c.want, c.wantFinal)
		}
	}
}

func TestDecideTiered(t *testing.T) {
	spv, mgr, owner, other := "spv", "mgr", "owner", "other"
	stage := ApprovalStage{SPV: &spv, Manager: &mgr}
	if as, onBehalf, final, msg := DecideTiered(Approver{EmployeeID: &spv}, owner, stage, "cuti"); as != DecidedAsManager || onBehalf || final || msg != "" {
		t.Errorf("SPV di tahapnya = %q %v %v %q", as, onBehalf, final, msg)
	}
	if _, _, _, msg := DecideTiered(Approver{EmployeeID: &mgr}, owner, stage, "cuti"); msg != "Bukan giliran Anda memutus pengajuan ini" {
		t.Errorf("Manager sebelum SPV = %q", msg)
	}
	if as, onBehalf, final, msg := DecideTiered(Approver{EmployeeID: &other, IsHR: true}, owner, stage, "cuti"); as != DecidedAsHR || !onBehalf || final || msg != "" {
		t.Errorf("HR mewakili SPV = %q %v %v %q", as, onBehalf, final, msg)
	}
	done := ApprovalStage{SPV: &spv, Manager: &mgr, SPVDone: true}
	if as, _, final, msg := DecideTiered(Approver{EmployeeID: &mgr}, owner, done, "cuti"); as != DecidedAsManager || !final || msg != "" {
		t.Errorf("Manager setelah SPV = %q %v %q", as, final, msg)
	}
	if _, onBehalf, final, _ := DecideTiered(Approver{IsHR: true}, owner, ApprovalStage{}, "cuti"); onBehalf || !final {
		t.Errorf("HR tanpa atasan: onBehalf=%v final=%v", onBehalf, final)
	}
	if _, _, _, msg := DecideTiered(Approver{EmployeeID: &owner, IsHR: true}, owner, stage, "cuti"); msg != "Tidak bisa memutuskan pengajuan cuti sendiri" {
		t.Errorf("diri sendiri = %q", msg)
	}
	if _, _, _, msg := DecideTiered(Approver{EmployeeID: &other}, owner, stage, "cuti"); msg != "Hanya atasan langsung atau HRD yang bisa memproses pengajuan ini" {
		t.Errorf("orang lain = %q", msg)
	}
}

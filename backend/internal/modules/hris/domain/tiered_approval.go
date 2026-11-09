package domain

// Approval berjenjang (EPIC-092): cuti/izin dan perjalanan dinas diputus SPV
// terdekat lalu Manager di atasnya (bila orang lain); HR boleh mewakili tahap
// yang sedang berjalan kapan saja. Lembur tidak berjenjang (EPIC-091 R0c).

// ApprovalStage is a tiered request's atasan: the nearest SPV approver
// (hris.request_approver), the Manager above (hris.request_manager), and
// whether the SPV stage is already done.
type ApprovalStage struct {
	SPV, Manager *string
	SPVDone      bool
}

// Current is who decides now and whether that approval ends the atasan
// stages (nil: no atasan, HR decides alone).
func (s ApprovalStage) Current() (approver *string, final bool) {
	switch {
	case s.SPV == nil:
		return s.Manager, true
	case s.Manager == nil || *s.Manager == *s.SPV:
		return s.SPV, true
	case s.SPVDone:
		return s.Manager, true
	default:
		return s.SPV, false
	}
}

// DecideTiered is who may act on a tiered request and how: the current
// stage's atasan as manager, or HR (onBehalf when an atasan stage is
// running); the other atasan gets "not your turn". final reports whether
// the approval ends the atasan stages.
func DecideTiered(a Approver, ownerID string, s ApprovalStage, noun string) (decidedAs string, onBehalf, final bool, refusal string) {
	if samePerson(a.EmployeeID, &ownerID) {
		return "", false, false, "Tidak bisa memutuskan pengajuan " + noun + " sendiri"
	}
	current, final := s.Current()
	if samePerson(a.EmployeeID, current) {
		return DecidedAsManager, false, final, ""
	}
	if a.IsHR {
		return DecidedAsHR, current != nil, final, ""
	}
	if samePerson(a.EmployeeID, s.SPV) || samePerson(a.EmployeeID, s.Manager) {
		return "", false, false, "Bukan giliran Anda memutus pengajuan ini"
	}
	return "", false, false, "Hanya atasan langsung atau HRD yang bisa memproses pengajuan ini"
}

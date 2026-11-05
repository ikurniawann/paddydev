package hris

import (
	"context"
	"net/http"
	"strconv"

	"nuhabit/backend/internal/modules/hris/domain"
)

// Riwayat Persetujuan Tim (EPIC-090): cuti/izin dan lembur bawahan langsung,
// semua status, sejak awal periode absensi sebelumnya — tanpa batas atas,
// supaya cuti yang tanggalnya setelah periode berjalan (diajukan jauh hari)
// tidak hilang setelah atasan memutusnya (review akhir).

const teamHistoryLimit = 300

// TeamHistory is GET /api/hris/me/team-history.
func (s *Service) TeamHistory(ctx context.Context, a *Actor) (*Row, error) {
	q := s.repo.querier()
	today := domain.TodayWIB(s.now())
	cur, err := periodContaining(ctx, q, today, today)
	if err != nil {
		return nil, err
	}
	prev, err := periodContaining(ctx, q, domain.AddDaysISO(cur.Start, -1), today)
	if err != nil {
		return nil, err
	}
	period := obj("from", prev.Start, "to", cur.End, "label", domain.PeriodRangeLabel(prev.Start, cur.End))
	if a.EmployeeID == nil {
		return obj("period", period, "leaves", []*Row{}, "overtime", []*Row{}, "loans", []*Row{}), nil
	}
	leaves, err := s.repo.TeamLeaveHistory(ctx, *a.EmployeeID, prev.Start)
	if err != nil {
		return nil, err
	}
	overtime, err := s.repo.TeamOvertimeHistory(ctx, *a.EmployeeID, prev.Start)
	if err != nil {
		return nil, err
	}
	teamLoans, err := s.payroll.TeamLoanHistory(ctx, q, *a.EmployeeID, prev.Start)
	if err != nil {
		return nil, err
	}
	loans := make([]*Row, 0, len(teamLoans))
	for _, l := range teamLoans {
		loans = append(loans, teamLoanRow(l))
	}
	return obj("period", period, "leaves", nonNilRows(leaves), "overtime", nonNilRows(overtime), "loans", loans), nil
}

// teamLoanRow is a TeamLoan in the shape of the ESS loan rows (EPIC-091).
func teamLoanRow(l TeamLoan) *Row {
	return obj("id", l.ID, "employee_id", l.EmployeeID, "loan_type", l.LoanType, "principal_amount", l.Principal,
		"monthly_installment", l.Monthly, "tenor_months", l.Tenor, "status", l.Status, "purpose", l.Purpose,
		"rejection_reason", l.RejectionReason, "manager_approver_name", l.ApproverName,
		"attachments", nonNilStrings(l.Attachments), "collateral_attachments", nonNilStrings(l.Collateral),
		"created_at", l.CreatedAt,
		"employee", obj("id", l.EmployeeID, "full_name", l.EmployeeName, "nip", l.EmployeeNIP))
}

func (s *store) TeamLeaveHistory(ctx context.Context, managerID, from string) ([]*Row, error) {
	rows, err := queryRows(ctx, s.db, `SELECT l.id, l.employee_id, l.leave_type, l.start_date::text AS start_date,
		l.end_date::text AS end_date, l.total_days, l.reason, l.attachment_url, l.status, l.attachment_review,
		l.attachment_return_reason, l.rejection_reason, l.decided_by_name, l.manager_approved_name,
		l.attachment_reviewed_name, l.created_at, `+teamEmployee+`
		FROM hris.leaves l `+teamMember("l")+`
		WHERE l.end_date >= $2::date
		ORDER BY l.start_date DESC, l.created_at DESC LIMIT `+strconv.Itoa(teamHistoryLimit), managerID, from)
	return rows, ignoreBadInput(err)
}

func (s *store) TeamOvertimeHistory(ctx context.Context, managerID, from string) ([]*Row, error) {
	rows, err := queryRows(ctx, s.db, `SELECT o.id, o.employee_id, o.date::text AS date, o.start_time, o.end_time,
		o.hours, o.break_minutes, o.attachments, o.day_type, o.reason, o.source, o.status, o.rejection_reason,
		o.decided_by_name, o.created_at, `+teamEmployee+`
		FROM hris.overtime_requests o `+teamMember("o")+`
		WHERE o.date >= $2::date
		ORDER BY o.date DESC, o.created_at DESC LIMIT `+strconv.Itoa(teamHistoryLimit), managerID, from)
	return rows, ignoreBadInput(err)
}

// GET /api/hris/me/team-history
func (h handlers) teamHistory(w http.ResponseWriter, r *http.Request, a *Actor) error {
	out, err := h.svc.TeamHistory(r.Context(), a)
	if err != nil {
		return err
	}
	return ok(w, dataOf(out))
}

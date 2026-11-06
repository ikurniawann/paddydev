package hris

import (
	"context"

	"nuhabit/backend/internal/modules/hris/domain"
)

// EPIC-094: lonceng peringatan saat clock-in terlambat, ke karyawan,
// penyetujunya (request_approver + request_manager) dan HR. Tanpa WhatsApp.
const (
	lateNoticeKind     = "alert"
	lateNoticeSelfLink = "/dashboard/me/absensi#keterlambatan"
	lateNoticeTeamLink = "/dashboard/me/absensi-tim"
	lateNoticeHRLink   = "/dashboard/hris/attendance"
)

// NotifyRequestApprovers is the in-app bell of employeeID's approver
// (hris.request_approver) and manager (hris.request_manager): one row per
// distinct login account, never the employee's own. It returns how many were
// inserted.
func (s *store) NotifyRequestApprovers(ctx context.Context, employeeID, kind, title, message, link string) (int, error) {
	tag, err := s.db.Exec(ctx, `INSERT INTO public.notifications (user_id, title, message, type, link)
		SELECT DISTINCT m.user_id, $2, $3, $4, $5
		FROM hris.employees e
		JOIN hris.employees m ON m.id IN (hris.request_approver(e.id), hris.request_manager(e.id))
		WHERE e.id = $1 AND m.id <> e.id AND m.user_id IS NOT NULL
		  AND m.user_id IS DISTINCT FROM e.user_id`, employeeID, title, message, kind, link)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// notifyLate rings the bells of a late clock-in of employeeID on date (R5).
// The count is the late days of the attendance period holding date (R6). A
// failure is logged and never fails the clock-in (R4).
func (s *Service) notifyLate(ctx context.Context, employeeID, date string, minutes int) {
	warn := func(step string, err error) {
		s.log.WarnContext(ctx, "lonceng terlambat gagal: "+step, "employee", employeeID, "date", date, "err", err)
	}
	q := s.repo.querier()
	p, err := periodContaining(ctx, q, date, domain.TodayWIB(s.now()))
	if err != nil {
		warn("periode", err)
		return
	}
	days, err := lateDays(ctx, q, []string{employeeID}, p.Start, p.End)
	if err != nil {
		warn("hitungan", err)
		return
	}
	var name string
	if err := q.QueryRow(ctx, `SELECT full_name FROM hris.employees WHERE id = $1`, employeeID).Scan(&name); err != nil {
		warn("nama", err)
		return
	}
	selfTitle, selfMsg, teamTitle, teamMsg := domain.LateNotice(name, minutes, len(days))
	if err := s.repo.NotifyEmployee(ctx, employeeID, lateNoticeKind, selfTitle, selfMsg, lateNoticeSelfLink); err != nil {
		warn("karyawan", err)
	}
	if _, err := s.repo.NotifyRequestApprovers(ctx, employeeID, lateNoticeKind, teamTitle, teamMsg, lateNoticeTeamLink); err != nil {
		warn("penyetuju", err)
	}
	if err := s.repo.NotifyHR(ctx, lateNoticeKind, teamTitle, teamMsg, lateNoticeHRLink); err != nil {
		warn("HR", err)
	}
}

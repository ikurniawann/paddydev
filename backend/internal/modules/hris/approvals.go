package hris

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"

	contracts "nuhabit/backend/internal/contracts/hris"
	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/outbox"
)

// Paket Approval atasan (EPIC-063): what leave and overtime decisions share.

// decisionRace is the 409 of a request another approver decided meanwhile.
const decisionRace = "Pengajuan sudah diproses oleh orang lain"

// auditDecision records a decision with its capacity (Q-APV-3: who decided,
// as direct manager or as HR) on q, inside the decision's transaction (LA-01).
func auditDecision(ctx context.Context, q database.Querier, a *Actor, action, entity, id, label string,
	after map[string]any, meta audit.Entry) error {
	meta.ActorID, meta.ActorName = a.UserID, ptr(actorName(a))
	meta.Action, meta.Entity, meta.EntityID, meta.EntityLabel = action, entity, &id, &label
	meta.Before = map[string]any{"status": "pending"}
	meta.After = after
	return audit.Write(ctx, q, meta)
}

// notifyManager sends the direct manager's WhatsApp note for a request (best
// effort, deduplicated per request through configuration.wa_notif_log, never
// failing the delivery). kind is the wa_notif_log type ("cuti", "lembur").
// ManagerContact skips a reporting_to pointing at the requester (EPIC-063 R2).
func (s *Service) notifyManager(ctx context.Context, tx pgx.Tx, kind, dedup, employeeID, employeeName string,
	message func(approver string) string) error {
	if s.whatsapp == nil {
		return nil
	}
	tag := "[wa-" + kind + "]"
	repo := &store{db: tx}
	name, phone, found, err := repo.ManagerContact(ctx, employeeID)
	if err != nil {
		return err
	}
	if !found {
		s.log.WarnContext(ctx, tag+" tidak punya atasan tercatat — notifikasi dilewati", "employee", employeeName)
		return nil
	}
	target := domain.NormalizeWaPhone(phone)
	if target == "" {
		s.log.WarnContext(ctx, tag+" nomor WA atasan kosong/tidak valid — notifikasi dilewati", "manager", name)
		return nil
	}
	text := message(name)
	if len(dedup) > 160 {
		dedup = dedup[:160]
	}
	claimID, claimed, err := s.whatsapp.Claim(ctx, tx, kind, dedup, text, []string{target})
	if err != nil || !claimed {
		return err
	}
	if !s.whatsapp.Configured(ctx) {
		s.log.WarnContext(ctx, tag+" gateway belum dikonfigurasi — notifikasi dilewati")
		return s.whatsapp.Release(ctx, tx, claimID)
	}
	if sent, timedOut, reason := s.whatsapp.SendText(ctx, target, text); !sent && !timedOut {
		s.log.ErrorContext(ctx, tag+" gagal kirim", "target", target, "reason", reason)
		return s.whatsapp.Release(ctx, tx, claimID)
	}
	return nil
}

// notifyOvertimeRequest is the outbox handler of TopicOvertimeRequested
// (EPIC-063 R8): the direct manager learns an overtime request waits for them.
func (s *Service) notifyOvertimeRequest(ctx context.Context, tx pgx.Tx, e outbox.Event) error {
	var p contracts.OvertimeRequested
	if err := e.Decode(&p); err != nil {
		s.log.ErrorContext(ctx, "[wa-lembur] bad payload", "event", e.ID, "error", err)
		return nil
	}
	return s.notifyManager(ctx, tx, "lembur", "overtime:"+p.OvertimeID, p.EmployeeID, p.EmployeeName, func(approver string) string {
		return domain.OvertimeRequestWaMessage(domain.OvertimeRequestWa{
			EmployeeName: p.EmployeeName, Date: p.Date, StartTime: p.StartTime, EndTime: p.EndTime,
			Hours: p.Hours, DayType: p.DayType, Reason: p.Reason, ApproverName: &approver,
		})
	})
}

/* ── Persetujuan Tim (ESS, EPIC-063 R7) ─────────────────────────────── */

// teamApprovalsHref is the ESS menu the team badge belongs to.
const teamApprovalsHref = "/dashboard/me/persetujuan-tim"

// ApprovalRepo reads the direct manager's queue (and, Task 6, the
// reporting_to data quality report).
type ApprovalRepo interface {
	TeamPendingLeaves(ctx context.Context, managerID string) ([]*Row, error)
	TeamPendingOvertime(ctx context.Context, managerID string) ([]*Row, error)
	// TeamPendingCount sums pending leave, employee overtime and business
	// trip requests of managerID's direct reports.
	TeamPendingCount(ctx context.Context, managerID string) (int64, error)
	// EmployeesWithoutManager is the reporting_to data quality report (R9).
	EmployeesWithoutManager(ctx context.Context) ([]*Row, error)
}

// TeamApprovals is GET /api/hris/me/team-approvals: pending leave and
// overtime requests of the actor's direct reports. Business trips and kasbon
// keep their own queues (GET /api/hris/business-trips?scope=approvals,
// GET /api/hris/loans?scope=approvals), which the ESS page reuses.
func (s *Service) TeamApprovals(ctx context.Context, a *Actor) (*Row, error) {
	if a.EmployeeID == nil {
		return obj("leaves", []*Row{}, "overtime", []*Row{}), nil
	}
	leaves, err := s.repo.TeamPendingLeaves(ctx, *a.EmployeeID)
	if err != nil {
		return nil, err
	}
	overtime, err := s.repo.TeamPendingOvertime(ctx, *a.EmployeeID)
	if err != nil {
		return nil, err
	}
	return obj("leaves", leaves, "overtime", overtime), nil
}

// teamPendingTotal is the Persetujuan Tim badge: everything waiting for the
// manager, kasbon at the manager stage included.
func (s *Service) teamPendingTotal(ctx context.Context, managerID string) (int64, error) {
	n, err := s.repo.TeamPendingCount(ctx, managerID)
	if err != nil {
		return 0, err
	}
	loans, err := s.payroll.ManagerPendingLoanCount(ctx, s.repo.querier(), managerID)
	return n + loans, err
}

// teamMember joins the requester (alias m) when they report to $1 directly;
// a reporting_to pointing at oneself (live data) is not a team member (R2).
func teamMember(alias string) string {
	return fmt.Sprintf(`JOIN hris.employees m ON m.id = %s.employee_id AND m.reporting_to = $1 AND m.id <> $1`, alias)
}

const teamEmployee = `json_build_object('id', m.id, 'full_name', m.full_name, 'nip', m.nip) AS employee`

func (s *store) TeamPendingLeaves(ctx context.Context, managerID string) ([]*Row, error) {
	rows, err := queryRows(ctx, s.db, `SELECT l.id, l.employee_id, l.leave_type, l.start_date::text AS start_date,
		l.end_date::text AS end_date, l.total_days, l.reason, l.attachment_url, l.created_at, `+teamEmployee+`
		FROM hris.leaves l `+teamMember("l")+`
		WHERE l.status = 'pending'
		ORDER BY l.start_date, l.created_at LIMIT 200`, managerID)
	return rows, ignoreBadInput(err)
}

func (s *store) TeamPendingOvertime(ctx context.Context, managerID string) ([]*Row, error) {
	rows, err := queryRows(ctx, s.db, `SELECT o.id, o.employee_id, o.date::text AS date, o.start_time, o.end_time,
		o.hours, o.day_type, o.reason, o.created_at, `+teamEmployee+`
		FROM hris.overtime_requests o `+teamMember("o")+`
		WHERE o.status = 'pending' AND o.source = 'employee'
		ORDER BY o.date, o.created_at LIMIT 200`, managerID)
	return rows, ignoreBadInput(err)
}

func (s *store) TeamPendingCount(ctx context.Context, managerID string) (int64, error) {
	var n int64
	err := s.db.QueryRow(ctx, `SELECT
		  (SELECT count(*) FROM hris.leaves l `+teamMember("l")+` WHERE l.status = 'pending')
		+ (SELECT count(*) FROM hris.overtime_requests o `+teamMember("o")+` WHERE o.status = 'pending' AND o.source = 'employee')
		+ (SELECT count(*) FROM hris.business_trips t `+teamMember("t")+` WHERE t.status = 'pending')`,
		managerID).Scan(&n)
	return n, ignoreBadInput(err)
}

// GET /api/hris/me/team-approvals
func (h handlers) teamApprovals(w http.ResponseWriter, r *http.Request, a *Actor) error {
	out, err := h.svc.TeamApprovals(r.Context(), a)
	if err != nil {
		return err
	}
	return ok(w, dataOf(out))
}

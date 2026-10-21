package hris

import (
	"context"

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

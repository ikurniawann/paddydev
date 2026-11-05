package hris

import (
	"context"
	"errors"
	"strings"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/httpx"
)

// Approval berjenjang (EPIC-092): cuti/izin dan perjalanan dinas diputus SPV
// terdekat lalu Manager di atasnya; HR boleh mewakili tahap berjalan.

// stageNames embeds the SPV and Manager names next to approver_id/manager_id
// (decision reads only: one row).
const stageNames = `(SELECT x.full_name FROM hris.employees x WHERE x.id = hris.request_approver(employees.id)) AS approver_name, ` +
	`(SELECT x.full_name FROM hris.employees x WHERE x.id = hris.request_manager(employees.id)) AS manager_name`

// tieredStage is emp's (approver_id, manager_id embed) tiered approval stage.
func tieredStage(emp *Row, spvDone bool) domain.ApprovalStage {
	if emp == nil {
		return domain.ApprovalStage{SPVDone: spvDone}
	}
	return domain.ApprovalStage{SPV: emp.StrPtr("approver_id"), Manager: emp.StrPtr("manager_id"), SPVDone: spvDone}
}

// stageName is the name of the atasan whose stage is running.
func stageName(emp *Row, stage domain.ApprovalStage) string {
	current, _ := stage.Current()
	if emp == nil || current == nil {
		return ""
	}
	if stage.Manager != nil && *current == *stage.Manager && (stage.SPV == nil || *stage.SPV != *stage.Manager) {
		return emp.Str("manager_name")
	}
	return emp.Str("approver_name")
}

// spvApproveLeave is the SPV stage of a leave with a Manager above (R2):
// still pending, the Manager is rung.
func (s *Service) spvApproveLeave(ctx context.Context, a *Actor, leave, emp *Row, onBehalf bool, deciderName string,
	meta audit.Entry) (*Row, error) {
	id := leave.Str("id")
	by := a.EmployeeID
	name := emp.Str("approver_name")
	if onBehalf {
		by, name = nil, deciderName
	}
	crew := leaveEmployeeName(leave)
	title, message := domain.LeaveManagerNotice(crew, leave.Str("leave_type"), dateText(leave, "start_date"),
		dateText(leave, "end_date"), int(leave.Num("total_days")))
	message = strings.Replace(message, " Tinjau di", " Disetujui SPV "+name+". Tinjau di", 1)
	err := s.repo.InTx(ctx, func(r Repository) error {
		if err := r.SpvApproveLeave(ctx, id, by, name); err != nil {
			return err
		}
		if err := auditDecision(ctx, r.querier(), a, "leave.spv_approve", "hris.leaves", id,
			crew+" · "+dateText(leave, "start_date")+" s.d. "+dateText(leave, "end_date"), nil,
			map[string]any{"status": "pending", "spv_approved_name": name}, meta); err != nil {
			return err
		}
		return r.NotifyEmployee(ctx, emp.Str("manager_id"), "approval", title, message, domain.TeamApprovalsLink)
	})
	if errors.Is(err, errNoRow) {
		return nil, httpx.Conflict(decisionRace)
	}
	if err != nil {
		return nil, err
	}
	data, err := s.repo.LeaveDecided(ctx, id)
	if err != nil {
		return nil, err
	}
	return obj("message", "Disetujui SPV — menunggu persetujuan Manager", "action", "approve", "data", data), nil
}

// SpvApproveLeave records the SPV stage; errNoRow when the leave is no longer
// waiting for it.
func (s *store) SpvApproveLeave(ctx context.Context, id string, by *string, name string) error {
	tag, err := s.db.Exec(ctx, `UPDATE hris.leaves SET spv_approved_by = $2, spv_approved_name = $3, spv_approved_at = now()
		WHERE id = $1 AND status = 'pending' AND spv_approved_at IS NULL AND attachment_review IS NULL`, id, by, name)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errNoRow
	}
	return nil
}

// tripRefusal keeps the trip's own noun in the self refusal.
func tripRefusal(msg string) string {
	return strings.Replace(msg, "pengajuan Perjalanan Dinas sendiri", "Perjalanan Dinas sendiri", 1)
}

// spvApproveTrip is the SPV stage of a trip with a Manager above (R3).
func (s *Service) spvApproveTrip(ctx context.Context, a *Actor, trip, emp *Row, deciderName string, onBehalf bool,
	meta audit.Entry) (*Row, error) {
	id := trip.Str("id")
	by := a.EmployeeID
	name := deciderName
	if !onBehalf {
		spv, _, err := s.repo.StageNames(ctx, trip.Str("employee_id"))
		if err != nil {
			return nil, err
		}
		name = spv
	} else {
		by = nil
	}
	crew := ""
	if emp != nil {
		crew = emp.Str("full_name")
	}
	err := s.repo.InTx(ctx, func(r Repository) error {
		tag, err := r.querier().Exec(ctx, `UPDATE hris.business_trips SET spv_approved_by = $2, spv_approved_name = $3,
			spv_approved_at = now() WHERE id = $1 AND status = 'pending' AND spv_approved_at IS NULL`, id, by, name)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errNoRow
		}
		meta.ActorID, meta.ActorName = a.UserID, ptr(actorName(a))
		meta.Action, meta.Entity, meta.EntityID = "business_trip.spv_approve", "hris.business_trips", ptr(id)
		meta.EntityLabel = ptr(crew + " · " + trip.Str("start_date") + " s.d. " + trip.Str("end_date"))
		meta.After = map[string]any{"status": "pending", "spv_approved_name": name}
		if err := audit.Write(ctx, r.querier(), meta); err != nil {
			return err
		}
		if emp == nil || emp.Str("manager_id") == "" {
			return nil
		}
		return r.NotifyEmployee(ctx, emp.Str("manager_id"), "approval", "Pengajuan perjalanan dinas: "+crew,
			crew+" mengajukan Perjalanan Dinas "+domain.ShortDateID(trip.Str("start_date"))+" s.d. "+
				domain.ShortDateID(trip.Str("end_date"))+" ke "+trip.Str("destination")+". Disetujui SPV "+name+
				". Tinjau di Area Karyawan → Persetujuan Tim.", domain.TeamApprovalsLink)
	})
	if errors.Is(err, errNoRow) {
		return nil, httpx.Conflict("Pengajuan sudah diproses oleh orang lain")
	}
	if err != nil {
		return nil, err
	}
	row, err := s.repo.TripByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return obj("data", row, "message", "Disetujui SPV — menunggu persetujuan Manager"), nil
}

// StageNames are the names of the employee's SPV and Manager approvers
// ("" without one).
func (s *store) StageNames(ctx context.Context, employeeID string) (spv, mgr string, err error) {
	err = s.db.QueryRow(ctx, `SELECT coalesce((SELECT full_name FROM hris.employees WHERE id = hris.request_approver($1::uuid)), ''),
		coalesce((SELECT full_name FROM hris.employees WHERE id = hris.request_manager($1::uuid)), '')`, employeeID).Scan(&spv, &mgr)
	return spv, mgr, err
}

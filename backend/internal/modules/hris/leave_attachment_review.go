package hris

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/validate"
)

// Verifikasi lampiran cuti/izin oleh HR (EPIC-088, ES-11, MOM 29-10-2026):
// atasan langsung menyetujui lebih dulu, lalu HR (pemegang "approve" daun
// Cuti) memverifikasi atau mengembalikan lampirannya. Selama itu status
// tetap pending; tahapnya di hris.leaves.attachment_review.

const (
	awaitingReviewMsg  = "Pengajuan sudah disetujui atasan dan menunggu verifikasi lampiran oleh HR"
	notAwaitingMsg     = "Pengajuan ini tidak menunggu verifikasi lampiran"
	leaveReviewKind    = "approval"      // public.notifications.type bagi HR
	leaveDecisionKind  = "status_change" // public.notifications.type bagi karyawan
	leaveReviewTitle   = "Lampiran cuti menunggu verifikasi"
	leaveReviewLink    = "/dashboard/hris/leaves?review=awaiting"
	essLeaveLink       = "/dashboard/me/cuti"
	leaveReturnedTitle = "Lampiran cuti dikembalikan"
)

// attachmentReview is what ReviewLeaveAttachment writes.
type attachmentReview struct {
	Verify                   bool
	ReviewerID, ReviewerName string
	Reason                   *string // return only
}

// attachmentReviewInput is POST /api/hris/leaves/attachment-review.
type attachmentReviewInput struct {
	LeaveID, Action string
	Reason          *string
}

// leaveNotifySummary is "<jenis> <mulai> s.d. <selesai>" for notifications.
func leaveNotifySummary(leave *Row) string {
	return fmt.Sprintf("%s %s s.d. %s", domain.LeaveTypeLabel(leave.Str("leave_type")),
		dateText(leave, "start_date"), dateText(leave, "end_date"))
}

func leaveEmployeeName(leave *Row) string {
	if emp := leave.Child("employee"); emp != nil {
		return emp.Str("full_name")
	}
	return ""
}

// managerApproveLeave is the manager's approval of a request with an
// attachment (R3): the request stays pending for HR, who is notified; the
// annual quota is only consumed at verification.
func (s *Service) managerApproveLeave(ctx context.Context, a *Actor, leave *Row, decidedAs string, meta audit.Entry) (*Row, error) {
	id := leave.Str("id")
	verdict := leaveVerdict{ApproverID: a.EmployeeID, DecidedAs: decidedAs, DecidedBy: actorName(a)}
	name := leaveEmployeeName(leave)
	err := s.repo.InTx(ctx, func(r Repository) error {
		if err := r.ManagerApproveLeave(ctx, id, verdict); err != nil {
			return err
		}
		if err := auditDecision(ctx, r.querier(), a, "leave.manager_approve", "hris.leaves", id,
			name+" · "+dateText(leave, "start_date")+" s.d. "+dateText(leave, "end_date"), nil,
			map[string]any{"status": "pending", "attachment_review": domain.AttachmentAwaiting, "decided_as": decidedAs}, meta); err != nil {
			return err
		}
		return r.NotifyHR(ctx, leaveReviewKind, leaveReviewTitle,
			fmt.Sprintf("%s · %s disetujui atasan, lampiran perlu diverifikasi. Tinjau di HRIS → Cuti & Izin.", name, leaveNotifySummary(leave)),
			leaveReviewLink)
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
	var phone *string
	if emp := leave.Child("employee"); emp != nil {
		phone = emp.StrPtr("phone")
	}
	rangeLabel := fmt.Sprintf("%s s.d. %s (%s hari)", dateText(leave, "start_date"), dateText(leave, "end_date"), leave.Str("total_days"))
	var link any
	if l := domain.BuildWaLink(phone, "Halo "+name+", pengajuan izin/cuti Anda "+rangeLabel+
		" telah disetujui atasan dan menunggu verifikasi lampiran oleh HR."); l != "" {
		link = l
	}
	return obj("message", "Disetujui atasan — menunggu verifikasi lampiran oleh HR", "action", "approve",
		"wa_link", link, "data", data), nil
}

// ReviewLeaveAttachment is POST /api/hris/leaves/attachment-review (R5).
func (s *Service) ReviewLeaveAttachment(ctx context.Context, a *Actor, in attachmentReviewInput, meta audit.Entry) (*Row, error) {
	if !a.CanApprove(iam.LeafCuti) {
		return nil, httpx.Forbidden("Hanya HR yang bisa memverifikasi lampiran")
	}
	leave, err := s.repo.LeaveForDecision(ctx, in.LeaveID)
	if err != nil {
		return nil, err
	}
	if leave == nil {
		return nil, httpx.NotFound("Leave request not found")
	}
	if a.EmployeeID != nil && leave.Str("employee_id") == *a.EmployeeID {
		return nil, httpx.Forbidden("Tidak bisa memverifikasi pengajuan sendiri")
	}
	if leave.Str("status") != "pending" || leave.Str("attachment_review") != domain.AttachmentAwaiting {
		return nil, httpx.BadRequest(notAwaitingMsg)
	}
	verify := in.Action == "verify"
	if !verify && (in.Reason == nil || strings.TrimSpace(*in.Reason) == "") {
		return nil, httpx.BadRequest("Alasan pengembalian wajib diisi")
	}
	rv := attachmentReview{Verify: verify, ReviewerID: a.UserID, ReviewerName: actorName(a)}
	var credit *quotaMove
	if verify {
		if leave.Str("leave_type") == "annual" {
			credit = &quotaMove{EmployeeID: leave.Str("employee_id"), Year: dateYear(leave, "start_date"),
				Days: leave.Str("total_days"), StartDate: dateText(leave, "start_date"), Actor: userOf(a)}
		}
	} else {
		reason := strings.TrimSpace(*in.Reason)
		rv.Reason = &reason
	}
	summary := leaveNotifySummary(leave)
	title, message, action := "Cuti disetujui",
		"Pengajuan "+summary+" disetujui setelah lampiran diverifikasi HR.", "leave.attachment_verify"
	after := map[string]any{"status": "approved", "attachment_review": domain.AttachmentVerified}
	if !verify {
		title, message, action = leaveReturnedTitle,
			"Lampiran pengajuan "+summary+" dikembalikan HR: "+*rv.Reason+". Unggah ulang di ESS → Cuti.", "leave.attachment_return"
		after = map[string]any{"status": "pending", "attachment_review": domain.AttachmentReturned, "reason": *rv.Reason}
	}
	err = s.repo.InTx(ctx, func(r Repository) error {
		if err := r.ReviewLeaveAttachment(ctx, in.LeaveID, rv, credit); err != nil {
			return err
		}
		if credit != nil {
			if err := afterBalanceChange(ctx, r.querier(), credit.EmployeeID, credit.Year,
				carryCause{Type: causeLeaveApprove, LeaveID: in.LeaveID}, userOf(a)); err != nil {
				return err
			}
		}
		if err := auditDecision(ctx, r.querier(), a, action, "hris.leaves", in.LeaveID,
			leaveEmployeeName(leave)+" · "+dateText(leave, "start_date")+" s.d. "+dateText(leave, "end_date"),
			map[string]any{"attachment_review": domain.AttachmentAwaiting}, after, meta); err != nil {
			return err
		}
		return r.NotifyEmployee(ctx, leave.Str("employee_id"), leaveDecisionKind, title, message, essLeaveLink)
	})
	if errors.Is(err, errNoRow) {
		return nil, httpx.Conflict(decisionRace)
	}
	if err != nil {
		return nil, err
	}
	data, err := s.repo.LeaveDecided(ctx, in.LeaveID)
	if err != nil {
		return nil, err
	}
	msg := "Lampiran terverifikasi — cuti disetujui"
	if !verify {
		msg = "Lampiran dikembalikan ke karyawan"
	}
	return obj("message", msg, "data", data), nil
}

// resubmitLeaveAttachment is the owner's new attachment for a returned
// request (R6): back to HR's queue, HR notified again.
func (s *Service) resubmitLeaveAttachment(ctx context.Context, id string, path *string) (*Row, error) {
	leave, err := s.repo.LeaveForDecision(ctx, id)
	if err != nil {
		return nil, err
	}
	if leave == nil {
		return nil, httpx.NotFound("Leave request not found")
	}
	if path == nil || !domain.LeaveAttachmentPathOK(*path, leave.Str("employee_id")) {
		return nil, httpx.BadRequest("Lampiran tidak valid")
	}
	err = s.repo.InTx(ctx, func(r Repository) error {
		if err := r.ResubmitLeaveAttachment(ctx, id, *path); err != nil {
			return err
		}
		return r.NotifyHR(ctx, leaveReviewKind, leaveReviewTitle,
			fmt.Sprintf("%s mengunggah ulang lampiran %s. Tinjau di HRIS → Cuti & Izin.", leaveEmployeeName(leave), leaveNotifySummary(leave)),
			leaveReviewLink)
	})
	if errors.Is(err, errNoRow) {
		return nil, httpx.Conflict("Pengajuan sudah berubah, muat ulang halaman")
	}
	if err != nil {
		return nil, err
	}
	row, err := s.repo.LeaveDecided(ctx, id)
	if err != nil {
		return nil, err
	}
	return obj("message", "Lampiran baru terkirim — menunggu verifikasi HR", "data", row), nil
}

/* ── Storage ─────────────────────────────────────────────────────────── */

func (s *store) ManagerApproveLeave(ctx context.Context, id string, v leaveVerdict) error {
	tag, err := s.db.Exec(ctx, `UPDATE hris.leaves
		SET attachment_review = 'awaiting', manager_approved_by = $2, manager_approved_name = $3,
		    manager_approved_at = now(), decided_as = $4
		WHERE id = $1 AND status = 'pending' AND attachment_review IS NULL`, id, v.ApproverID, v.DecidedBy, v.DecidedAs)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errNoRow
	}
	return nil
}

func (s *store) ReviewLeaveAttachment(ctx context.Context, id string, rv attachmentReview, credit *quotaMove) error {
	query := `UPDATE hris.leaves
		SET attachment_review = 'returned', attachment_return_reason = $4,
		    attachment_reviewed_by = $2, attachment_reviewed_name = $3, attachment_reviewed_at = now()
		WHERE id = $1 AND status = 'pending' AND attachment_review = 'awaiting'`
	if rv.Verify {
		// The manager stays the approver; HR is recorded as the reviewer.
		query = `UPDATE hris.leaves
			SET status = 'approved', approved_by = manager_approved_by, approved_at = now(),
			    decided_by_name = manager_approved_name, rejection_reason = NULL,
			    attachment_review = 'verified', attachment_return_reason = $4,
			    attachment_reviewed_by = $2, attachment_reviewed_name = $3, attachment_reviewed_at = now()
			WHERE id = $1 AND status = 'pending' AND attachment_review = 'awaiting'`
	}
	tag, err := s.db.Exec(ctx, query, id, rv.ReviewerID, rv.ReviewerName, rv.Reason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errNoRow
	}
	if credit == nil {
		return nil
	}
	return s.consumeAnnualLeave(ctx, id, credit)
}

func (s *store) ResubmitLeaveAttachment(ctx context.Context, id, path string) error {
	tag, err := s.db.Exec(ctx, `UPDATE hris.leaves
		SET attachment_url = $2, attachment_review = 'awaiting', attachment_return_reason = NULL
		WHERE id = $1 AND status = 'pending' AND attachment_review = 'returned'`, id, path)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errNoRow
	}
	return nil
}

/* ── HTTP ────────────────────────────────────────────────────────────── */

// POST /api/hris/leaves/attachment-review { leave_id, action, reason? }
func (h handlers) reviewLeaveAttachment(w http.ResponseWriter, r *http.Request, a *Actor) error {
	const message = "Validation failed"
	f, err := readForm(r, message)
	if err != nil {
		return err
	}
	in := attachmentReviewInput{
		LeaveID: strOr(f.UUID("leave_id", validate.Rule{})),
		Action:  strOr(f.Enum("action", validate.Rule{}, []string{"verify", "return"})),
		Reason:  f.Str("reason", optional, validate.StrOpts{}),
	}
	if err := formErr(f, message); err != nil {
		return err
	}
	out, err := h.svc.ReviewLeaveAttachment(r.Context(), a, in, auditMeta(r))
	if err != nil {
		return err
	}
	return ok(w, out)
}

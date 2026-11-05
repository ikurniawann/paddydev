package hris

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	contracts "nuhabit/backend/internal/contracts/hris"
	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/outbox"
)

// Leave requests and balances: lib/hris/leaves-repo, leave-balances-repo,
// leaves-export, leave-wa. Status changes and quota moves share one
// transaction; the manager's WhatsApp note goes through the outbox.

// TopicLeaveRequested is re-exported for the module constructor.
const TopicLeaveRequested = contracts.TopicLeaveRequested

// TopicOvertimeRequested is re-exported for the module constructor (EPIC-063).
const TopicOvertimeRequested = contracts.TopicOvertimeRequested

// LeaveRepo is the leave storage.
type LeaveRepo interface {
	LeavePage(ctx context.Context, f leaveFilter, page, limit int) ([]*Row, int64, error)
	LeaveEmployee(ctx context.Context, id string) (*Row, error)
	FirstContract(ctx context.Context, employeeID string) (*domain.FirstContract, error)
	ActiveHolidays(ctx context.Context, start, end string) ([]domain.Holiday, error)
	InsertLeave(ctx context.Context, f fields) (*Row, error)
	LeaveDetail(ctx context.Context, id string) (*Row, error)
	LeaveForDecision(ctx context.Context, id string) (*Row, error)
	LeaveDecided(ctx context.Context, id string) (*Row, error)
	PlainLeave(ctx context.Context, id string) (*Row, error)
	CancelLeave(ctx context.Context, id, fromStatus string, refund *quotaMove) error
	UpdateLeave(ctx context.Context, id string, f fields) (*Row, error)
	DeleteLeave(ctx context.Context, id string) error
	// DecideLeave writes the verdict of a pending request; errNoRow when it
	// is no longer pending (decided meanwhile).
	DecideLeave(ctx context.Context, id string, v leaveVerdict, credit *quotaMove) error
	// ManagerApproveLeave records the manager's approval of a request with an
	// attachment, which then waits for HR (EPIC-088 R3); errNoRow when it is
	// no longer pending without a review stage.
	ManagerApproveLeave(ctx context.Context, id string, v leaveVerdict) error
	// SpvApproveLeave records the SPV stage of a tiered leave (EPIC-092).
	SpvApproveLeave(ctx context.Context, id string, by *string, name string) error
	// ReviewLeaveAttachment verifies (approving, with the annual quota) or
	// returns a request waiting for HR (R5); errNoRow when it is not waiting.
	ReviewLeaveAttachment(ctx context.Context, id string, rv attachmentReview, credit *quotaMove) error
	// ResubmitLeaveAttachment puts a returned request back in HR's queue
	// with its new attachment (R6); errNoRow when it is not returned.
	ResubmitLeaveAttachment(ctx context.Context, id, path string) error
	LeavesForExport(ctx context.Context, f leaveFilter) ([]*Row, error)
	Publish(ctx context.Context, topic, key string, payload any) error
	ManagerContact(ctx context.Context, employeeID string) (name string, phone *string, found bool, err error)
	// EmployeePositionLevel is the level of the employee's position ("" without
	// one); found is false when the employee does not exist.
	EmployeePositionLevel(ctx context.Context, employeeID string) (level string, found bool, err error)
	EmployeeEmploymentStatus(ctx context.Context, employeeID string) (string, error)
	// LockLeaveEmployee serialises leave requests of one employee (inside a tx).
	LockLeaveEmployee(ctx context.Context, employeeID string) error
	// OverlappingLeave is a pending/approved request covering any day of
	// [start, end] (columns leave_type, status, start_date, end_date as text); nil when none.
	OverlappingLeave(ctx context.Context, employeeID, start, end string) (*Row, error)
	// MonthlyLeaveUse sums total_days of pending/approved requests per type
	// whose start_date falls in [from, to].
	MonthlyLeaveUse(ctx context.Context, employeeID, from, to string) (map[string]float64, error)
}

// quotaMove adds (or refunds) days of a year's annual leave (EPIC-068 R5).
type quotaMove struct {
	EmployeeID string
	Year       int
	Days       string // numeric text of total_days
	// StartDate (YYYY-MM-DD) of the leave: the carry-over counts only when it
	// has not expired by then. Used when consuming.
	StartDate string
	// CarryDays is the part of Days taken from the carry-over at approval
	// (hris.leaves.carry_over_days_used); a refund returns it to the
	// carry-over and the rest to the annual quota. Used when refunding.
	CarryDays string
	// Actor is who approved (nil: the system); a carry-over recalculated for a
	// row this approval creates is audited under it.
	Actor *auth.User
}

// leaveVerdict is what DecideLeave writes (EPIC-063 R1).
type leaveVerdict struct {
	Status                string
	ApproverID, Rejection *string
	DecidedAs, DecidedBy  string
	// Reviewer is set when an attachment verifier approves a leave with an
	// attachment: the approval is also the verification (EPIC-088 R2).
	Reviewer *auth.User
	// ReviewerName is the verifier's own name, never "mewakili" (EPIC-092).
	ReviewerName string
	// HRReject: an attachment verifier rejects, which may land on a request
	// already waiting for HR; any other decision needs no review stage yet,
	// so a second atasan cannot overturn the first one's approval (EPIC-091).
	HRReject bool
}

// leaveFilter is the list/export query.
type leaveFilter struct {
	EmployeeID, Status, LeaveType, StartDate, EndDate *string
	// Review is the EPIC-088 stage of a pending request: awaiting / returned.
	Review *string
}

// asPlainError hides a PostgreSQL error behind a plain one: the TS throws
// new Error(error.message) there, which renders as a 500, never a 4xx.
func asPlainError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s", err.Error())
}

// yearOf is new Date(date).getFullYear() of a YYYY-MM-DD string.
func yearOf(date string) int {
	if len(date) >= 4 {
		var y int
		fmt.Sscanf(date[:4], "%d", &y)
		return y
	}
	return 0
}

func (s *Service) ListLeaves(ctx context.Context, a *Actor, f leaveFilter, page, limit int) (*Row, error) {
	// employee_id=me: the account's own requests (ESS), whatever it may read.
	if f.EmployeeID != nil && *f.EmployeeID == "me" {
		if a.EmployeeID == nil {
			return nil, httpx.Forbidden("Akun ini tidak terhubung ke data karyawan")
		}
		f.EmployeeID = a.EmployeeID
	}
	if !a.hrReads(iam.LeafCuti) {
		if a.EmployeeID == nil {
			return nil, httpx.Forbidden("Akun ini tidak terhubung ke data karyawan")
		}
		f.EmployeeID = a.EmployeeID
	}
	rows, total, err := s.repo.LeavePage(ctx, f, page, limit)
	if err != nil {
		return nil, asPlainError(err)
	}
	return obj("data", rows, "pagination", pagination(page, limit, total)), nil
}

// leaveRequest is leaveRequestSchema.
type leaveRequest struct {
	EmployeeID                            *string
	LeaveType, StartDate, EndDate, Reason string
	AttachmentURL                         *string
}

// maxLeaveSpanDays bounds one leave request (and its preview) so a hostile
// range cannot make the day-by-day count run over millions of dates.
const maxLeaveSpanDays = 366

// leaveSpanTooLong: [start, end] (end >= start) covers more than
// maxLeaveSpanDays calendar days.
func leaveSpanTooLong(start, end string) bool {
	s, ok1 := domain.ParseDate(start)
	e, ok2 := domain.ParseDate(end)
	return ok1 && ok2 && e.Sub(s).Hours()/24 >= maxLeaveSpanDays
}

const errLeaveSpan = "Rentang cuti maksimal 366 hari"

// leaveDays is the number of days a leave over [start, end] consumes, and the
// holidays that were skipped (EPIC-079 R4). Shift staff count from their
// roster and never skip a red date; Non-Shift staff skip weekends and the
// holidays that are a day off for them with deducts_leave = false.
func (s *Service) leaveDays(ctx context.Context, employeeID, scheduleType, start, end string) (int, []domain.ExcludedHoliday, error) {
	if scheduleType == "shift" {
		rows, err := s.repo.ShiftPattern(ctx, employeeID)
		if err != nil {
			return 0, nil, err
		}
		schedule := make([]domain.ScheduleRow, len(rows))
		for i, r := range rows {
			schedule[i] = domain.ScheduleRow{DayOfWeek: int(r.Num("day_of_week")), ShiftID: r.StrPtr("shift_id"),
				EffectiveFrom: r.Str("effective_from"), EffectiveTo: r.StrPtr("effective_to")}
		}
		return domain.DescribeRosterLeaveDays(start, end, schedule), []domain.ExcludedHoliday{}, nil
	}
	holidays, err := s.repo.ActiveHolidays(ctx, start, end)
	if err != nil {
		return 0, nil, err
	}
	dayOff := make([]domain.Holiday, 0, len(holidays))
	for _, h := range holidays {
		if domain.HolidayDayOff(h, scheduleType) {
			dayOff = append(dayOff, h)
		}
	}
	days, excluded := domain.DescribeLeaveDays(start, end, domain.IndexHolidays(dayOff))
	return days, excluded, nil
}

// MyLeaveDays is the preview of the days a leave over [start, end] consumes
// for the signed-in employee, by the same rule CreateLeave applies.
func (s *Service) MyLeaveDays(ctx context.Context, a *Actor, start, end string) (*Row, error) {
	if a.EmployeeID == nil {
		return nil, httpx.BadRequest("Akun ini tidak terhubung ke data karyawan")
	}
	if end < start {
		return nil, httpx.BadRequest("Tanggal selesai tidak boleh sebelum tanggal mulai")
	}
	if leaveSpanTooLong(start, end) {
		return nil, httpx.BadRequest(errLeaveSpan)
	}
	emp, err := s.repo.LeaveEmployee(ctx, *a.EmployeeID)
	if err != nil {
		return nil, err
	}
	if emp == nil {
		return nil, httpx.NotFound("Employee not found or inactive")
	}
	days, excluded, err := s.leaveDays(ctx, *a.EmployeeID, emp.Str("schedule_type"), start, end)
	if err != nil {
		return nil, err
	}
	return obj("days", days, "excluded", excluded), nil
}

// checkBirthdayLeave is EPIC-084 R4: a filled-in birth date, a day in the
// birthday month, and the first contract over by the birthday that year.
func (s *Service) checkBirthdayLeave(ctx context.Context, employeeID string, emp *Row, date string) error {
	birthday, ok := domain.BirthdayInYear(emp.Str("birth_date"), yearOf(date))
	if !ok {
		return httpx.BadRequest("Tanggal lahir belum diisi di data karyawan — hubungi HR")
	}
	if birthday[5:7] != date[5:7] {
		return httpx.BadRequest("Birthday Leave hanya dalam bulan ulang tahun Anda")
	}
	first, err := s.repo.FirstContract(ctx, employeeID)
	if err != nil {
		return err
	}
	if !domain.InitialContractPassed(first, emp.Str("employment_status"), birthday) {
		return httpx.BadRequest("Birthday Leave berlaku setelah masa kontrak awal selesai")
	}
	return nil
}

// CreateLeave is createLeave: days are counted by leaveDays (the roster for
// shift staff; Non-Shift staff skip weekends and day-off holidays); a range
// with no working day is refused.
func (s *Service) CreateLeave(ctx context.Context, a *Actor, in leaveRequest) (*Row, error) {
	lt, known := domain.LookupLeaveType(in.LeaveType)
	if !known || !lt.Selectable {
		return nil, httpx.BadRequest("Jenis cuti ini sudah tidak berlaku untuk pengajuan baru")
	}
	employeeID := a.EmployeeID
	if a.IsHR && in.EmployeeID != nil && *in.EmployeeID != "" {
		employeeID = in.EmployeeID
	}
	if employeeID == nil {
		return nil, httpx.NotFound("Akun ini tidak terhubung ke data karyawan")
	}
	emp, err := s.repo.LeaveEmployee(ctx, *employeeID)
	if err != nil {
		return nil, err
	}
	if emp == nil || !emp.Bool("is_active") {
		return nil, httpx.NotFound("Employee not found or inactive")
	}
	if in.EndDate < in.StartDate {
		return nil, httpx.BadRequest("Tanggal selesai tidak boleh sebelum tanggal mulai")
	}
	if leaveSpanTooLong(in.StartDate, in.EndDate) {
		return nil, httpx.BadRequest(errLeaveSpan)
	}
	if lt.SingleDay && in.StartDate != in.EndDate {
		return nil, httpx.BadRequest(lt.Label + " diajukan per satu hari — tanggal mulai dan selesai harus sama")
	}
	if lt.LeaderOnly {
		level, _, err := s.repo.EmployeePositionLevel(ctx, *employeeID)
		if err != nil {
			return nil, err
		}
		if !domain.IsLeaderLevel(level) {
			return nil, httpx.BadRequest("Izin Leader hanya untuk jabatan Supervisor ke atas")
		}
	}
	// TnC 03-11 (EPIC-084). Batas waktu dan kunci cut off hanya untuk
	// pengajuan milik sendiri; HR yang mencatat untuk karyawan lain tidak.
	isSelf := a.EmployeeID != nil && *employeeID == *a.EmployeeID
	if msg := domain.LeaveGenderError(lt, emp.Str("gender")); msg != "" {
		return nil, httpx.BadRequest(msg)
	}
	if lt.AttachmentRequired && (in.AttachmentURL == nil || strings.TrimSpace(*in.AttachmentURL) == "") {
		return nil, httpx.BadRequest(lt.Label + " wajib melampirkan dokumen")
	}
	if isSelf {
		if msg := domain.LeaveTimingError(lt, in.StartDate, domain.TodayWIB(s.now())); msg != "" {
			return nil, httpx.BadRequest(msg)
		}
		if err := s.ensureSubmissionOpen(ctx, in.StartDate); err != nil {
			return nil, err
		}
	}
	if lt.Code == "birthday" {
		if err := s.checkBirthdayLeave(ctx, *employeeID, emp, in.StartDate); err != nil {
			return nil, err
		}
	}
	totalDays, excluded, err := s.leaveDays(ctx, *employeeID, emp.Str("schedule_type"), in.StartDate, in.EndDate)
	if err != nil {
		return nil, err
	}
	if totalDays == 0 {
		return nil, httpx.BadRequest(domain.NoWorkingDayMessage(excluded, emp.Str("schedule_type")))
	}
	if msg := domain.LeaveSizeError(lt, in.StartDate, in.EndDate, totalDays); msg != "" {
		return nil, httpx.BadRequest(msg)
	}
	if isSelf && lt.Code == "personal" {
		// R5: Izin Pribadi hanya bila belum punya hak cuti atau cutinya habis.
		year := yearOf(in.StartDate)
		if err := s.EnsureLeaveBalance(ctx, *employeeID, year); err != nil {
			return nil, err
		}
		available, _, err := s.repo.AnnualAvailable(ctx, *employeeID, year, in.StartDate)
		if err != nil {
			return nil, err
		}
		if available >= 1 {
			return nil, httpx.BadRequest("Izin Pribadi hanya untuk yang belum punya hak cuti atau cutinya habis — sisa Cuti Tahunan Anda " +
				strings.Replace(strconv.FormatFloat(available, 'f', -1, 64), ".", ",", 1) + " hari")
		}
	}
	if in.LeaveType == "annual" {
		// R6: the year's row is created from the default rule when missing,
		// then the request must fit what is available on its start date
		// (annual remainder plus a carry-over that has not expired by then).
		year := yearOf(in.StartDate)
		if err := s.EnsureLeaveBalance(ctx, *employeeID, year); err != nil {
			return nil, err
		}
		available, found, err := s.repo.AnnualAvailable(ctx, *employeeID, year, in.StartDate)
		if err != nil {
			return nil, err
		}
		if found && available < float64(totalDays) {
			return nil, httpx.BadRequest("Sisa cuti tahunan tidak cukup", obj("remaining", available, "requested", totalDays))
		}
	}

	var f fields
	f.add("employee_id", *employeeID)
	f.add("leave_type", in.LeaveType)
	f.add("start_date", in.StartDate)
	f.add("end_date", in.EndDate)
	f.add("total_days", totalDays)
	f.add("reason", in.Reason)
	f.add("attachment_url", nilIfEmpty(in.AttachmentURL))
	f.add("status", "pending")
	var leave *Row
	err = s.repo.InTx(ctx, func(r Repository) error {
		// Lock, overlap and quota checks share this transaction with the
		// insert, so two concurrent requests cannot both pass them.
		if err := r.LockLeaveEmployee(ctx, *employeeID); err != nil {
			return err
		}
		clash, err := r.OverlappingLeave(ctx, *employeeID, in.StartDate, in.EndDate)
		if err != nil {
			return err
		}
		if clash != nil {
			state := "menunggu persetujuan"
			if clash.Str("status") == "approved" {
				state = "disetujui"
			}
			return httpx.BadRequest(fmt.Sprintf("Tanggal ini bertabrakan dengan pengajuan %s (%s s.d. %s, %s)",
				domain.LeaveTypeLabel(clash.Str("leave_type")), clash.Str("start_date"), clash.Str("end_date"), state))
		}
		if lt.MonthlyQuota > 0 {
			month := domain.MonthOf(in.StartDate)
			from, to, _ := domain.MonthBounds(month)
			use, err := r.MonthlyLeaveUse(ctx, *employeeID, from, to)
			if err != nil {
				return err
			}
			if used := use[lt.Code]; used+float64(totalDays) > float64(lt.MonthlyQuota) {
				return httpx.BadRequest(fmt.Sprintf("Kuota %s bulan %s sudah habis (%s dari %d hari)",
					lt.Label, month, strconv.FormatFloat(used, 'f', -1, 64), lt.MonthlyQuota),
					obj("quota", lt.MonthlyQuota, "used", used, "month", month))
			}
		}
		if lt.Code == "birthday" {
			year := in.StartDate[:4]
			use, err := r.MonthlyLeaveUse(ctx, *employeeID, year+"-01-01", year+"-12-31")
			if err != nil {
				return err
			}
			if use["birthday"] > 0 {
				return httpx.BadRequest("Birthday Leave sudah dipakai tahun " + year)
			}
		}
		if leave, err = r.InsertLeave(ctx, f); err != nil {
			return err
		}
		title, message := domain.LeaveManagerNotice(emp.Str("full_name"), in.LeaveType, in.StartDate, in.EndDate, totalDays)
		if err := r.NotifyManager(ctx, *employeeID, title, message, domain.TeamApprovalsLink); err != nil {
			return err
		}
		reason := in.Reason
		return r.Publish(ctx, TopicLeaveRequested, *employeeID, contracts.LeaveRequested{
			LeaveID: leave.Str("id"), EmployeeID: *employeeID, EmployeeName: emp.Str("full_name"),
			LeaveType: in.LeaveType, StartDate: in.StartDate, EndDate: in.EndDate, TotalDays: totalDays, Reason: &reason,
		})
	})
	if err != nil {
		return nil, err
	}
	return obj("message", "Leave request submitted successfully", "data", leave,
		"meta", obj("total_days", totalDays, "excluded_holidays", excluded)), nil
}

// notifyLeaveRequest is the outbox handler of TopicLeaveRequested
// (notifyLeaveRequestWa): best effort, deduplicated per leave through
// configuration.wa_notif_log, never failing the delivery.
func (s *Service) notifyLeaveRequest(ctx context.Context, tx pgx.Tx, e outbox.Event) error {
	var p contracts.LeaveRequested
	if err := e.Decode(&p); err != nil {
		s.log.ErrorContext(ctx, "[wa-cuti] bad payload", "event", e.ID, "error", err)
		return nil
	}
	return s.notifyManager(ctx, tx, "cuti", "leave:"+p.LeaveID, p.EmployeeID, p.EmployeeName, func(approver string) string {
		return domain.LeaveRequestWaMessage(domain.LeaveRequestWa{
			EmployeeName: p.EmployeeName, LeaveType: p.LeaveType, StartDate: p.StartDate, EndDate: p.EndDate,
			TotalDays: p.TotalDays, Reason: p.Reason, ApproverName: &approver,
		})
	})
}

// Leave is getLeave: HR (or "approve" on Cuti), the owner, or the direct manager.
func (s *Service) Leave(ctx context.Context, a *Actor, id string) (*Row, error) {
	row, err := s.repo.LeaveDetail(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, httpx.NotFound("Leave request not found")
	}
	own := a.EmployeeID != nil && row.Str("employee_id") == *a.EmployeeID
	manager := false
	if emp := row.Child("employee"); emp != nil && a.EmployeeID != nil {
		manager = isApprover(emp, *a.EmployeeID)
	}
	if !a.hrReads(iam.LeafCuti) && !own && !manager {
		return nil, httpx.Forbidden("Insufficient permissions")
	}
	return row, nil
}

// leaveUpdate is leaveUpdateSchema.
type leaveUpdate struct {
	Status                *string
	Reason, AttachmentURL opt[string]
}

// UpdateLeave cancels (owner while pending, HR also once approved, with the
// annual quota refunded in the same transaction) or lets HR edit the reason
// and attachment.
func (s *Service) UpdateLeave(ctx context.Context, a *Actor, id string, in leaveUpdate, meta audit.Entry) (*Row, error) {
	leave, err := s.repo.PlainLeave(ctx, id)
	if err != nil {
		return nil, err
	}
	if leave == nil {
		return nil, httpx.NotFound("Leave request not found")
	}
	if in.Status != nil && *in.Status == "cancelled" {
		status := leave.Str("status")
		isOwner := a.EmployeeID != nil && leave.Str("employee_id") == *a.EmployeeID
		pending := status == "pending" && (isOwner || a.IsHR)
		approved := status == "approved" && a.IsHR
		if !pending && !approved {
			if status == "approved" {
				return nil, httpx.Forbidden("Cuti yang sudah disetujui hanya bisa dibatalkan oleh HRD/admin")
			}
			return nil, httpx.Forbidden("Pengajuan ini tidak bisa dibatalkan")
		}
		var refund *quotaMove
		if approved && leave.Str("leave_type") == "annual" {
			refund = &quotaMove{EmployeeID: leave.Str("employee_id"), Year: dateYear(leave, "start_date"),
				Days: leave.Str("total_days"), CarryDays: leave.Str("carry_over_days_used")}
		}
		err := s.repo.InTx(ctx, func(r Repository) error {
			if err := r.CancelLeave(ctx, id, status, refund); err != nil {
				return err
			}
			if refund == nil {
				return nil
			}
			// The refund changed the year's balance: next year's carry follows.
			return afterBalanceChange(ctx, r.querier(), refund.EmployeeID, refund.Year,
				carryCause{Type: causeLeaveCancel, LeaveID: id}, userOf(a))
		})
		if errors.Is(err, errNoRow) {
			return nil, httpx.Conflict("Pengajuan sudah berubah, muat ulang halaman")
		}
		if err != nil {
			return nil, err
		}
		if refund != nil {
			return msgOf("Cuti dibatalkan — kuota " + refund.Days + " hari dikembalikan"), nil
		}
		return msgOf("Pengajuan dibatalkan"), nil
	}

	if in.AttachmentURL.Sent && !in.Reason.Sent && leave.Str("attachment_review") == domain.AttachmentReturned &&
		leave.Str("status") == "pending" && a.EmployeeID != nil && leave.Str("employee_id") == *a.EmployeeID {
		return s.resubmitLeaveAttachment(ctx, a, id, in.AttachmentURL.Val, meta)
	}

	var f fields
	if in.Reason.Sent {
		f.add("reason", in.Reason.Val)
	}
	if in.AttachmentURL.Sent {
		f.add("attachment_url", in.AttachmentURL.Val)
	}
	if !a.IsHR || len(f) == 0 {
		return nil, httpx.Forbidden("No valid updates or insufficient permissions")
	}
	row, err := s.repo.UpdateLeave(ctx, id, f)
	if err != nil {
		return nil, err
	}
	return obj("message", "Leave request updated successfully", "data", row), nil
}

// dateYear is the year of a date column of a node-postgres row.
func dateYear(r *Row, key string) int {
	if t, ok := r.Time(key); ok {
		return t.Year()
	}
	return 0
}

// dateText is a date column as YYYY-MM-DD.
func dateText(r *Row, key string) string {
	if t, ok := r.Time(key); ok {
		return t.Format(domain.DateLayout)
	}
	return ""
}

// DeleteLeave removes a leave request; the route guard requires "delete"
// on Cuti & Izin, a reason, and writes the audit row (EPIC-061).
func (s *Service) DeleteLeave(ctx context.Context, _ *Actor, id string) error {
	return s.repo.DeleteLeave(ctx, id)
}

// leaveDecision is leaveApprovalSchema.
type leaveDecision struct {
	LeaveID, Action string
	Rejection       *string
}

// DecideLeave is decideLeave with the EPIC-063 rules (Q-APV-3): the direct
// manager decides first, HR is the backup and may override, nobody decides
// their own request; the capacity and the decider's name are stored and
// audited. Approving annual leave consumes the quota in the same
// transaction. The response carries a wa.me link for the approver.
func (s *Service) DecideLeave(ctx context.Context, a *Actor, in leaveDecision, meta audit.Entry) (*Row, error) {
	leave, err := s.repo.LeaveForDecision(ctx, in.LeaveID)
	if err != nil {
		return nil, err
	}
	if leave == nil {
		return nil, httpx.NotFound("Leave request not found")
	}
	emp := leave.Child("employee")
	stage := tieredStage(emp, leave.Get("spv_approved_at") != nil)
	// EPIC-092: SPV then the Manager above; HR (approve on the Cuti leaf,
	// EPIC-067 R11) may act for the running stage at any time.
	decidedAs, onBehalf, final, refusal := domain.DecideTiered(domain.Approver{EmployeeID: a.EmployeeID,
		IsHR: a.CanApprove(iam.LeafCuti)}, leave.Str("employee_id"), stage, "cuti")
	if decidedAs == "" {
		return nil, httpx.Forbidden(refusal)
	}
	// Waiting for HR's attachment review the atasan stages are over: HR acts
	// as itself there (review akhir EPIC-092).
	onBehalf = onBehalf && leave.Str("attachment_review") == ""
	deciderName := actorName(a)
	if onBehalf {
		deciderName += " (mewakili " + stageName(emp, stage) + ")"
	}
	if status := leave.Str("status"); status != "pending" {
		return nil, httpx.BadRequest("Leave request already "+status, obj("current_status", status))
	}
	approve := in.Action == "approve"
	verifier := a.CanApprove(iam.LeafCuti)
	// EPIC-088 R4: once the manager approved, only HR acts — rejecting here,
	// approving through the attachment review.
	if leave.Str("attachment_review") != "" && (approve || !verifier) {
		return nil, httpx.BadRequest(awaitingReviewMsg)
	}
	if in.Action == "reject" && (in.Rejection == nil || *in.Rejection == "") {
		return nil, httpx.BadRequest("Rejection reason is required")
	}
	if approve && !final {
		return s.spvApproveLeave(ctx, a, leave, emp, onBehalf, deciderName, meta)
	}
	if approve && domain.NeedsAttachmentReview(leave.Str("attachment_url"), verifier) {
		return s.managerApproveLeave(ctx, a, leave, decidedAs, meta)
	}
	status, rejection := "rejected", in.Rejection
	var credit *quotaMove
	if approve {
		status, rejection = "approved", nil
		if leave.Str("leave_type") == "annual" {
			credit = &quotaMove{EmployeeID: leave.Str("employee_id"), Year: dateYear(leave, "start_date"),
				Days: leave.Str("total_days"), StartDate: dateText(leave, "start_date"), Actor: userOf(a)}
		}
	}
	verdict := leaveVerdict{Status: status, ApproverID: a.EmployeeID, Rejection: rejection,
		DecidedAs: decidedAs, DecidedBy: deciderName}
	if approve && strings.TrimSpace(leave.Str("attachment_url")) != "" {
		verdict.Reviewer, verdict.ReviewerName = userOf(a), actorName(a)
	}
	verdict.HRReject = !approve && verifier
	label := dateText(leave, "start_date") + " s.d. " + dateText(leave, "end_date")
	if emp != nil {
		label = emp.Str("full_name") + " · " + label
	}
	err = s.repo.InTx(ctx, func(r Repository) error {
		if err := r.DecideLeave(ctx, in.LeaveID, verdict, credit); err != nil {
			return err
		}
		// Approving annual leave used up the year's balance: next year's carry
		// follows. A rejection (credit nil) moves no balance, so there is
		// nothing to recalculate (causeLeaveReject is documented, never run).
		if credit != nil {
			if err := afterBalanceChange(ctx, r.querier(), credit.EmployeeID, credit.Year,
				carryCause{Type: causeLeaveApprove, LeaveID: in.LeaveID}, userOf(a)); err != nil {
				return err
			}
		}
		return auditDecision(ctx, r.querier(), a, "leave."+in.Action, "hris.leaves", in.LeaveID, label, nil,
			map[string]any{"status": status, "decided_as": decidedAs, "rejection_reason": rejection}, meta)
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

	var name, phone *string
	if emp != nil {
		name, phone = emp.StrPtr("full_name"), emp.StrPtr("phone")
	}
	rangeLabel := fmt.Sprintf("%s s.d. %s (%s hari)", dateText(leave, "start_date"), dateText(leave, "end_date"), leave.Str("total_days"))
	message := "Halo " + jsText(name) + ", pengajuan izin/cuti Anda " + rangeLabel + " telah DISETUJUI. Selamat beristirahat!"
	if !approve {
		message = "Halo " + jsText(name) + ", mohon maaf pengajuan izin/cuti Anda " + rangeLabel +
			" DITOLAK. Alasan: " + jsText(in.Rejection) + ". Silakan hubungi HRD untuk diskusi."
	}
	var link any
	if l := domain.BuildWaLink(phone, message); l != "" {
		link = l
	}
	return obj("message", "Leave request "+in.Action+"d successfully", "action", in.Action, "wa_link", link, "data", data), nil
}

// jsText is `${value}` for a value that may be undefined.
func jsText(s *string) string {
	if s == nil {
		return "undefined"
	}
	return *s
}

// LeavesCSV is the HR export; nil when there is nothing to export.
func (s *Service) LeavesCSV(ctx context.Context, f leaveFilter) (string, error) {
	rows, err := s.repo.LeavesForExport(ctx, f)
	if err != nil {
		return "", asPlainError(err)
	}
	if len(rows) == 0 {
		return "", httpx.NotFound("No leave data found")
	}
	out := make([]domain.LeaveCSVRow, len(rows))
	for i, r := range rows {
		c := domain.LeaveCSVRow{
			ID: r.Str("id"), LeaveType: r.Str("leave_type"), Status: r.Str("status"),
			StartDate: dateText(r, "start_date"), EndDate: dateText(r, "end_date"),
			TotalDays: r.StrPtr("total_days"), Reason: r.StrPtr("reason"), RejectionReason: r.StrPtr("rejection_reason"),
		}
		if t, ok := r.Time("approved_at"); ok {
			c.ApprovedAt = &t
		}
		c.CreatedAt, _ = r.Time("created_at")
		if emp := r.Child("employee"); emp != nil {
			c.EmployeeName, c.EmployeeNip = emp.StrPtr("full_name"), emp.StrPtr("nip")
			if d := emp.Child("department"); d != nil {
				c.Department = d.StrPtr("name")
			}
			if j := emp.Child("job_title"); j != nil {
				c.JobTitle = j.StrPtr("title")
			}
		}
		if ap := r.Child("approver"); ap != nil {
			c.HasApprover, c.ApproverName, c.ApproverNip = true, ap.StrPtr("full_name"), ap.StrPtr("nip")
		}
		out[i] = c
	}
	return domain.LeavesCSV(out), nil
}

/* ── Leave balances ──────────────────────────────────────────────────── */

func isLineManagerRole(role string) bool { return role == "hiring_manager" || role == "hrd" }

// LeaveQuota is the monthly quota of Izin Pribadi / Izin Leader (EPIC-058):
// the employee themself, or an HR account holding the Cuti menu. It carries
// days only, never salary amounts.
func (s *Service) LeaveQuota(ctx context.Context, a *Actor, employeeID *string, month string) (*Row, error) {
	target := a.EmployeeID
	if employeeID != nil && *employeeID != "" {
		target = employeeID
	}
	if target == nil {
		return nil, httpx.Forbidden("Akun ini tidak terhubung ke data karyawan")
	}
	own := a.EmployeeID != nil && *target == *a.EmployeeID
	if !own && !(a.IsHR && a.WorkforceMenu) {
		return nil, httpx.Forbidden("Insufficient permissions")
	}
	if month == "" {
		month = domain.NowWIB(s.now()).Format("2006-01")
	}
	from, to, ok := domain.MonthBounds(month)
	if !ok {
		return nil, httpx.BadRequest("Format bulan harus YYYY-MM")
	}
	level, found, err := s.repo.EmployeePositionLevel(ctx, *target)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, httpx.NotFound("Employee not found")
	}
	status, err := s.repo.EmployeeEmploymentStatus(ctx, *target)
	if err != nil {
		return nil, err
	}
	intern := domain.IsInternStatus(status)
	use, err := s.repo.MonthlyLeaveUse(ctx, *target, from, to)
	if err != nil {
		return nil, err
	}
	eligible := domain.IsLeaderLevel(level)
	types := []any{}
	for _, t := range domain.QuotaLeaveTypes() {
		used := use[t.Code]
		types = append(types, obj("leave_type", t.Code, "label", t.Label, "quota", t.MonthlyQuota,
			"used", used, "remaining", max(0, float64(t.MonthlyQuota)-used), "available", !t.LeaderOnly || eligible,
			"deducts_thp", t.DeductsThpDay && !intern))
	}
	return obj("employee_id", *target, "month", month, "position_level", level,
		"employment_status", status, "leader_eligible", eligible, "types", types), nil
}

package hris

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	contracts "nuhabit/backend/internal/contracts/hris"
	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
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
	ActiveHolidays(ctx context.Context, start, end string) ([]domain.Holiday, error)
	AnnualRemaining(ctx context.Context, employeeID string, year int) (*Row, error)
	InsertLeave(ctx context.Context, f fields) (*Row, error)
	LeaveDetail(ctx context.Context, id string) (*Row, error)
	LeaveForDecision(ctx context.Context, id string) (*Row, error)
	LeaveDecided(ctx context.Context, id string) (*Row, error)
	PlainLeave(ctx context.Context, id string) (*Row, error)
	CancelLeave(ctx context.Context, id string, refund *quotaMove) error
	UpdateLeave(ctx context.Context, id string, f fields) (*Row, error)
	DeleteLeave(ctx context.Context, id string) error
	// DecideLeave writes the verdict of a pending request; errNoRow when it
	// is no longer pending (decided meanwhile).
	DecideLeave(ctx context.Context, id string, v leaveVerdict, credit *quotaMove) error
	LeavesForExport(ctx context.Context, f leaveFilter) ([]*Row, error)
	LeaveBalance(ctx context.Context, employeeID string, year int) (*Row, error)
	EmployeeJoinDate(ctx context.Context, id string) (*string, bool, error)
	InsertLeaveBalance(ctx context.Context, f fields) (*Row, error)
	LeaveBalanceID(ctx context.Context, employeeID string, year int) (*string, error)
	UpdateLeaveBalance(ctx context.Context, id string, f fields) (*Row, error)
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

// quotaMove adds (or refunds) days of a year's annual leave.
type quotaMove struct {
	EmployeeID string
	Year       int
	Days       string // numeric text of total_days
}

// leaveVerdict is what DecideLeave writes (EPIC-063 R1).
type leaveVerdict struct {
	Status                string
	ApproverID, Rejection *string
	DecidedAs, DecidedBy  string
}

// leaveFilter is the list/export query.
type leaveFilter struct {
	EmployeeID, Status, LeaveType, StartDate, EndDate *string
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
	if !a.IsHR {
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

// CreateLeave is createLeave: weekends and national holidays do not consume
// leave (cuti bersama does); a range with no working day is refused.
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
	holidays, err := s.repo.ActiveHolidays(ctx, in.StartDate, in.EndDate)
	if err != nil {
		return nil, err
	}
	totalDays, excluded := domain.DescribeLeaveDays(in.StartDate, in.EndDate, domain.IndexHolidays(holidays))
	if totalDays == 0 {
		return nil, httpx.BadRequest(domain.NoWorkingDayMessage(excluded))
	}
	if in.LeaveType == "annual" {
		balance, err := s.repo.AnnualRemaining(ctx, *employeeID, yearOf(in.StartDate))
		if err != nil {
			return nil, err
		}
		if balance != nil && balance.Num("annual_leave_remaining") < float64(totalDays) {
			return nil, httpx.BadRequest("Insufficient annual leave balance",
				obj("remaining", balance.Get("annual_leave_remaining"), "requested", totalDays))
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
		if leave, err = r.InsertLeave(ctx, f); err != nil {
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

// Leave is getLeave: HR, the owner, or the direct manager.
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
		manager = emp.Str("reporting_to") == *a.EmployeeID
	}
	if !a.IsHR && !own && !manager {
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
func (s *Service) UpdateLeave(ctx context.Context, a *Actor, id string, in leaveUpdate) (*Row, error) {
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
			refund = &quotaMove{EmployeeID: leave.Str("employee_id"), Year: dateYear(leave, "start_date"), Days: leave.Str("total_days")}
		}
		if err := s.repo.InTx(ctx, func(r Repository) error { return r.CancelLeave(ctx, id, refund) }); err != nil {
			return nil, err
		}
		if refund != nil {
			return msgOf("Cuti dibatalkan — kuota " + refund.Days + " hari dikembalikan"), nil
		}
		return msgOf("Pengajuan dibatalkan"), nil
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
	var reportingTo *string
	if emp != nil {
		reportingTo = emp.StrPtr("reporting_to")
	}
	// The HR backup holds "approve" on the Cuti leaf (EPIC-067 R11), not IsHR.
	decidedAs, refusal := domain.DecideAs(domain.Approver{EmployeeID: a.EmployeeID, IsHR: a.CanApprove(iam.LeafCuti)},
		leave.Str("employee_id"), reportingTo, "cuti")
	if decidedAs == "" {
		return nil, httpx.Forbidden(refusal)
	}
	if status := leave.Str("status"); status != "pending" {
		return nil, httpx.BadRequest("Leave request already "+status, obj("current_status", status))
	}
	if in.Action == "reject" && (in.Rejection == nil || *in.Rejection == "") {
		return nil, httpx.BadRequest("Rejection reason is required")
	}
	approve := in.Action == "approve"
	status, rejection := "rejected", in.Rejection
	var credit *quotaMove
	if approve {
		status, rejection = "approved", nil
		if leave.Str("leave_type") == "annual" {
			credit = &quotaMove{EmployeeID: leave.Str("employee_id"), Year: dateYear(leave, "start_date"), Days: leave.Str("total_days")}
		}
	}
	verdict := leaveVerdict{Status: status, ApproverID: a.EmployeeID, Rejection: rejection,
		DecidedAs: decidedAs, DecidedBy: actorName(a)}
	label := dateText(leave, "start_date") + " s.d. " + dateText(leave, "end_date")
	if emp != nil {
		label = emp.Str("full_name") + " · " + label
	}
	err = s.repo.InTx(ctx, func(r Repository) error {
		if err := r.DecideLeave(ctx, in.LeaveID, verdict, credit); err != nil {
			return err
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

// LeaveBalance is getLeaveBalance: the owner, HRD or a manager; the year's
// row is created (prorated quota) on first read.
func (s *Service) LeaveBalance(ctx context.Context, a *Actor, employeeID string, year int) (*Row, error) {
	isOwner := a.EmployeeID != nil && employeeID == *a.EmployeeID
	if !isOwner && !isLineManagerRole(a.Role) {
		return nil, httpx.Forbidden("Forbidden: Can only view own leave balance")
	}
	row, err := s.repo.LeaveBalance(ctx, employeeID, year)
	if err != nil {
		return nil, asPlainError(err)
	}
	if row != nil {
		return row, nil
	}
	join, found, err := s.repo.EmployeeJoinDate(ctx, employeeID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, httpx.NotFound("Employee not found")
	}
	var f fields
	f.add("employee_id", employeeID)
	f.add("year", year)
	f.add("annual_leave_total", domain.ProratedAnnualQuota(join, year))
	for _, col := range []string{"annual_leave_used", "sick_leave_used", "unpaid_leave_used", "maternity_leave_used", "paternity_leave_used"} {
		f.add(col, 0)
	}
	return s.repo.InsertLeaveBalance(ctx, f)
}

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

// UpdateLeaveBalance lets HRD set a balance; a new row starts at 12 days.
func (s *Service) UpdateLeaveBalance(ctx context.Context, a *Actor, employeeID string, year int, in fields) (*Row, error) {
	if a.Role != "hrd" {
		return nil, httpx.Forbidden("Forbidden: Only HRD can update leave balances")
	}
	id, err := s.repo.LeaveBalanceID(ctx, employeeID, year)
	if err != nil {
		return nil, err
	}
	if id != nil {
		return s.repo.UpdateLeaveBalance(ctx, *id, in)
	}
	f := fields{{"employee_id", employeeID}, {"year", year}, {"annual_leave_total", 12}}
	for _, c := range in {
		f.set(c.col, c.val)
	}
	return s.repo.InsertLeaveBalance(ctx, f)
}

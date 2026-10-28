package hris

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	contracts "nuhabit/backend/internal/contracts/hris"
	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/validate"
)

// Overtime requests (lib/hris/overtime-repo) and the shift master
// (lib/hris/shifts-repo).

// OvertimeRepo is the overtime storage.
type OvertimeRepo interface {
	SubordinateIDs(ctx context.Context, managerID string) ([]string, error)
	OvertimeList(ctx context.Context, f overtimeFilter) ([]*Row, error)
	OvertimeTarget(ctx context.Context, id string) (*Row, error)
	OvertimeActiveOn(ctx context.Context, employeeID, date string) (bool, error)
	InsertOvertime(ctx context.Context, f fields) (*Row, error)
	OvertimeForDecision(ctx context.Context, id string) (*Row, error)
	DecideOvertime(ctx context.Context, id string, f fields) (*Row, error)
	// CorrectOvertime updates a request still in fromStatus; errNoRow when it
	// changed meanwhile (EPIC-063 R6).
	CorrectOvertime(ctx context.Context, id, fromStatus string, f fields) (*Row, error)
	// OvertimeForUpdate is OvertimeForDecision locking the request row.
	OvertimeForUpdate(ctx context.Context, id string) (*Row, error)
}

type overtimeFilter struct {
	EmployeeIDs      []string
	EmployeeSource   bool
	Status           *string
	DateFrom, DateTo string
}

const overtimeDuplicate = "Sudah ada pengajuan lembur pending/approved di tanggal tersebut"

// overtimeAttachmentRe is a stored overtime documentation path (EPIC-066).
var overtimeAttachmentRe = regexp.MustCompile(`^overtime-attachments/([0-9a-f-]{36})/[0-9]+-[0-9a-f]{16}\.(pdf|jpg|png|webp)$`)

// overtimeQuery is overtimeListQuerySchema.
type overtimeQuery struct{ Status, Month, Year, EmployeeID, Scope *string }

// ListOvertime: HR all (or one employee), the employee their own, a direct
// manager with scope=approvals their reports' own requests.
func (s *Service) ListOvertime(ctx context.Context, a *Actor, q overtimeQuery) ([]*Row, error) {
	var f overtimeFilter
	switch {
	case q.Scope != nil && *q.Scope == "approvals":
		if a.EmployeeID == nil {
			return []*Row{}, nil
		}
		ids, err := s.repo.SubordinateIDs(ctx, *a.EmployeeID)
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			return []*Row{}, nil
		}
		f.EmployeeIDs, f.EmployeeSource = ids, true
	case a.IsHR:
		target := q.EmployeeID
		if target != nil && *target == "me" {
			target = a.EmployeeID
		}
		if target != nil && *target != "" {
			f.EmployeeIDs = []string{*target}
		}
	default:
		if a.EmployeeID == nil {
			return []*Row{}, nil
		}
		f.EmployeeIDs = []string{*a.EmployeeID}
	}
	if q.Status != nil && *q.Status != "" {
		f.Status = q.Status
	}
	if from, to, ok := domain.OvertimeMonthFilter(strOr(q.Month), strOr(q.Year), q.Month != nil, q.Year != nil); ok {
		f.DateFrom, f.DateTo = from, to
	}
	rows, err := s.repo.OvertimeList(ctx, f)
	return rows, asPlainError(err)
}

// overtimeInput is overtimeCreateSchema.
type overtimeInput struct {
	EmployeeID                       *string
	Date, StartTime, EndTime, Reason string
	DayType                          *string // regular (Weekday) | rest_day (Weekend / Libur), K-13; nil = saran jadwal
	BreakMinutes                     int
	Attachments                      []string
}

// overtimeDayTypes: rest_day = hari istirahat mingguan, dibayar tarif libur.
var overtimeDayTypes = []string{"regular", "rest_day"}

// overtimeTimes is the direct manager's correction while approving (EPIC-066).
type overtimeTimes struct {
	Start, End *string
	Break      *int
}

func (t overtimeTimes) any() bool { return t.Start != nil || t.End != nil || t.Break != nil }

// overtimeClock trims a DB-style HH:MM:SS down to HH:MM.
func overtimeClock(v string) string {
	if len(v) == 8 && v[5] == ':' {
		return v[:5]
	}
	return v
}

// CreateOvertime: a request for oneself ("employee", decided by the direct
// manager or HR) or an HR assignment for someone else ("company", confirmed
// by the employee). One's own request must come before the overtime starts
// (AP-02) and its type must match the schedule (K-13); it notifies the
// direct manager through the outbox (EPIC-063).
func (s *Service) CreateOvertime(ctx context.Context, a *Actor, in overtimeInput) (*Row, error) {
	targetID := a.EmployeeID
	if in.EmployeeID != nil && *in.EmployeeID != "" && *in.EmployeeID != "me" {
		canonical, ok := validate.CanonicalUUID(*in.EmployeeID) // ids compare as canonical lowercase (P10)
		if !ok {
			return nil, httpx.BadRequest("ID karyawan tidak valid")
		}
		targetID = &canonical
	}
	if targetID == nil {
		return nil, httpx.BadRequest("Akun ini tidak tertaut ke data karyawan")
	}
	isSelf := a.EmployeeID != nil && *targetID == *a.EmployeeID
	// HR administration of overtime is the HRD set (ruling C2), like its decisions.
	if !isSelf && !a.IsHRD {
		return nil, httpx.Forbidden("Hanya HRD yang bisa membuat penugasan lembur untuk karyawan lain")
	}
	// The start time is checked strictly as HH:MM (a DB-style HH:MM:SS is
	// trimmed) because OvertimeStartsInPast is fail-open on unparsable input (P10).
	in.StartTime, in.EndTime = overtimeClock(in.StartTime), overtimeClock(in.EndTime)
	if _, err := time.Parse(domain.DateLayout, in.Date); err != nil || !overtimeTimeRe.MatchString(in.StartTime) ||
		!overtimeTimeRe.MatchString(in.EndTime) {
		return nil, httpx.BadRequest("Tanggal atau jam lembur tidak valid")
	}
	if isSelf {
		if domain.OvertimeNotFinished(in.Date, in.StartTime, in.EndTime, s.now()) {
			ends, _ := domain.OvertimeEndsAt(in.Date, in.StartTime, in.EndTime)
			return nil, httpx.BadRequest("Lembur baru bisa diinput setelah selesai (" + ends.Format("02-01-2006 15:04") + " WIB)")
		}
		if err := s.ensureAttendanceOpen(ctx, in.Date); err != nil {
			return nil, err
		}
		if err := s.checkOvertimeAttachments(*targetID, in.Attachments); err != nil {
			return nil, err
		}
	} else {
		// HR may record a past assignment for someone else (EPIC-063 R3).
		if len(domain.JSTrim(in.Reason)) < 5 {
			return nil, httpx.BadRequest("Alasan minimal 5 karakter")
		}
		in.BreakMinutes, in.Attachments = 0, nil
	}
	target, err := s.repo.OvertimeTarget(ctx, *targetID)
	if err != nil {
		return nil, err
	}
	if target == nil || (target.Has("is_active") && target.Get("is_active") == false) {
		return nil, httpx.NotFound("Karyawan tidak ditemukan/nonaktif")
	}
	exists, err := s.repo.OvertimeActiveOn(ctx, *targetID, in.Date)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, httpx.BadRequest(overtimeDuplicate)
	}
	if in.StartTime == in.EndTime {
		return nil, httpx.BadRequest("Jam mulai dan selesai tidak boleh sama")
	}
	if domain.OvertimeGrossMinutes(in.StartTime, in.EndTime) > 12*60 {
		return nil, httpx.BadRequest("Durasi lembur maksimal 12 jam")
	}
	hours := domain.PaidOvertimeHours(in.StartTime, in.EndTime, in.BreakMinutes)
	if hours < 1 {
		if in.BreakMinutes > 0 {
			return nil, httpx.BadRequest("Durasi lembur minimal 1 jam setelah dikurangi istirahat")
		}
		return nil, httpx.BadRequest("Durasi lembur minimal 1 jam")
	}
	dayType, err := s.overtimeDayType(ctx, *targetID, in.Date, in.DayType, !isSelf)
	if err != nil {
		return nil, err
	}
	source := "company"
	if isSelf {
		source = "employee"
	}
	var f fields
	f.add("employee_id", *targetID)
	f.add("date", in.Date)
	f.add("start_time", in.StartTime)
	f.add("end_time", in.EndTime)
	f.add("hours", hours)
	f.add("break_minutes", in.BreakMinutes)
	f.add("attachments", nonNilStrings(in.Attachments))
	f.add("source", source)
	f.add("status", "pending")
	f.add("reason", in.Reason)
	f.add("day_type", dayType)
	f.add("requested_by", a.EmployeeID)
	var row *Row
	err = s.repo.InTx(ctx, func(r Repository) error {
		var err error
		if row, err = r.InsertOvertime(ctx, f); err != nil || source != "employee" {
			return err
		}
		reason := in.Reason
		return r.Publish(ctx, TopicOvertimeRequested, *targetID, contracts.OvertimeRequested{
			OvertimeID: row.Str("id"), EmployeeID: *targetID, EmployeeName: target.Str("full_name"),
			Date: in.Date, StartTime: in.StartTime, EndTime: in.EndTime, Hours: hours, DayType: dayType, Reason: &reason,
		})
	})
	if database.IsUniqueViolation(err) { // the active (employee_id, date) index lost a race
		return nil, httpx.BadRequest(overtimeDuplicate)
	}
	if err != nil {
		return nil, err
	}
	message := "Pengajuan lembur terkirim — menunggu persetujuan"
	if source == "company" {
		message = "Penugasan lembur dibuat — menunggu konfirmasi karyawan"
	}
	return obj("data", row, "message", message), nil
}

// checkOvertimeAttachments: 1–3 paths uploaded by the owner and still on disk.
func (s *Service) checkOvertimeAttachments(ownerID string, paths []string) error {
	if len(paths) == 0 {
		return httpx.BadRequest("Lampirkan minimal 1 dokumentasi lembur")
	}
	if len(paths) > maxOvertimeAttachments {
		return httpx.BadRequest("Maksimal 3 dokumentasi lembur")
	}
	const invalid = "Dokumentasi lembur tidak valid — unggah ulang filenya"
	seen := make(map[string]bool, len(paths))
	for _, p := range paths {
		if seen[p] {
			return httpx.BadRequest("Dokumentasi lembur tidak boleh sama")
		}
		seen[p] = true
		m := overtimeAttachmentRe.FindStringSubmatch(p)
		if m == nil || m[1] != ownerID {
			return httpx.BadRequest(invalid)
		}
		if _, _, err := s.files.ReadPrivate(p); err != nil {
			return httpx.BadRequest(invalid)
		}
	}
	return nil
}

func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

var (
	decisionStatus  = map[string]string{"approve": "approved", "reject": "rejected", "cancel": "cancelled"}
	decisionMessage = map[string]string{"approved": "Lembur disetujui", "rejected": "Lembur ditolak", "cancelled": "Pengajuan dibatalkan"}
)

// DecideOvertime approves, rejects or cancels a pending request (rules in
// domain.CanDecideOvertime); the update is guarded on status = pending. An
// employee request's approval or rejection stores the decider's capacity
// (direct manager or HR, Q-APV-3) and is audited (EPIC-063 R1).
func (s *Service) DecideOvertime(ctx context.Context, a *Actor, id, action string, rejection, dayType *string, times overtimeTimes, meta audit.Entry) (*Row, error) {
	ot, err := s.repo.OvertimeForDecision(ctx, id)
	if err != nil {
		return nil, err
	}
	if ot == nil {
		return nil, httpx.NotFound("Pengajuan lembur tidak ditemukan")
	}
	if status := ot.Str("status"); status != "pending" {
		return nil, httpx.BadRequest("Pengajuan sudah "+status, obj("current_status", status))
	}
	emp := ot.Child("employee")
	reportingTo, managerActive := overtimeManager(emp)
	// The HR side of the decision holds "approve" on the Lembur leaf
	// (EPIC-067 R11): backup/override, day type, time correction, cancelling
	// a company assignment.
	isHR := a.CanApprove(iam.LeafLembur)
	allowed, reason := domain.CanDecideOvertime(domain.OvertimeActor{EmployeeID: a.EmployeeID, IsHR: isHR},
		domain.OvertimeRequest{EmployeeID: ot.Str("employee_id"), RequestedBy: ot.StrPtr("requested_by"),
			Source: ot.Str("source"), ReportingTo: reportingTo}, action)
	if !allowed {
		return nil, httpx.Forbidden(reason)
	}
	if action == "reject" && (rejection == nil || domain.JSTrim(*rejection) == "") {
		return nil, httpx.BadRequest("Alasan penolakan wajib diisi")
	}
	status := decisionStatus[action]
	now := s.clock()
	var f fields
	f.add("status", status)
	f.add("decided_by", a.EmployeeID)
	f.add("decided_at", now)
	if action == "reject" {
		f.add("rejection_reason", rejection)
	} else {
		f.add("rejection_reason", nil)
	}
	// HR may set the day type (rest day = tarif libur) while approving an
	// employee request (R11); never on a company assignment, where the target
	// (possibly an HRD) only confirms and must not change their own pay rate.
	// A real change is audited as before/after day_type.
	// HR here is "approve" on the Lembur leaf (EPIC-067 R11), never IsHR.
	auditBefore, auditAfter := map[string]any(nil), map[string]any{}
	if action == "approve" && isHR && dayType != nil && ot.Str("source") == "employee" {
		f.add("day_type", *dayType)
		if stored := ot.Str("day_type"); stored != *dayType {
			auditBefore = map[string]any{"day_type": stored}
			auditAfter["day_type"] = *dayType
		}
	}
	if times.any() {
		if action != "approve" || ot.Str("source") != "employee" {
			return nil, httpx.BadRequest("Koreksi jam hanya saat menyetujui input lembur karyawan")
		}
		if allowed, msg := domain.CanCorrectOvertimeTimes(a.EmployeeID, isHR, ot.Str("employee_id"), reportingTo, managerActive); !allowed {
			return nil, httpx.Forbidden(msg)
		}
		was := struct {
			start, end string
			brk        int
		}{overtimeClock(ot.Str("start_time")), overtimeClock(ot.Str("end_time")), int(ot.Num("break_minutes"))}
		start, end, brk := was.start, was.end, was.brk
		if times.Start != nil {
			start = *times.Start
		}
		if times.End != nil {
			end = *times.End
		}
		if times.Break != nil {
			brk = *times.Break
		}
		if start == end {
			return nil, httpx.BadRequest("Jam mulai dan selesai tidak boleh sama")
		}
		if domain.OvertimeGrossMinutes(start, end) > 12*60 {
			return nil, httpx.BadRequest("Durasi lembur maksimal 12 jam")
		}
		hours := domain.PaidOvertimeHours(start, end, brk)
		if hours < 1 {
			return nil, httpx.BadRequest("Durasi lembur minimal 1 jam")
		}
		if start != was.start || end != was.end || brk != was.brk {
			f.add("start_time", start)
			f.add("end_time", end)
			f.add("break_minutes", brk)
			f.add("hours", hours)
			f.add("corrected_at", now)
			f.add("corrected_by_name", actorName(a))
			if auditBefore == nil {
				auditBefore = map[string]any{}
			}
			auditBefore["start_time"], auditBefore["end_time"], auditBefore["break_minutes"] = was.start, was.end, was.brk
			auditBefore["hours"] = ot.Num("hours")
			auditAfter["start_time"], auditAfter["end_time"], auditAfter["break_minutes"], auditAfter["hours"] = start, end, brk, hours
		}
	}
	decidedAs := ""
	if ot.Str("source") == "employee" && action != "cancel" {
		var refusal string
		decidedAs, refusal = domain.DecideAs(domain.Approver{EmployeeID: a.EmployeeID, IsHR: isHR},
			ot.Str("employee_id"), reportingTo, "lembur")
		if decidedAs == "" { // never store '' (the CHECK would 500)
			return nil, httpx.Forbidden(refusal)
		}
		f.add("decided_as", decidedAs)
		f.add("decided_by_name", actorName(a))
	}
	f.add("updated_at", now)
	var row *Row
	err = s.repo.InTx(ctx, func(r Repository) error {
		var err error
		if row, err = r.DecideOvertime(ctx, id, f); err != nil || decidedAs == "" {
			return err
		}
		label := dateText(ot, "date")
		if emp != nil {
			label = emp.Str("full_name") + " · " + label
		}
		auditAfter["status"], auditAfter["decided_as"], auditAfter["rejection_reason"] = status, decidedAs, rejection
		return auditDecision(ctx, r.querier(), a, "overtime."+action, "hris.overtime_requests", id, label,
			auditBefore, auditAfter, meta)
	})
	if errors.Is(err, errNoRow) {
		return nil, httpx.Conflict(decisionRace)
	}
	if err != nil {
		return nil, err
	}
	return obj("data", row, "message", decisionMessage[status]), nil
}

/* ── Shift master ────────────────────────────────────────────────────── */

const shiftColumns = `id, code, name, start_time, end_time, break_minutes,
  late_tolerance_minutes, is_overnight, is_active, is_default, sort_order`

func (s *store) ListShifts(ctx context.Context) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT `+shiftColumns+` FROM hris.shifts ORDER BY sort_order ASC, name ASC`)
}

// ListShiftsWithUsage is ListShifts plus employees_using: the distinct active
// employees holding the shift on a per-date row from today on, or in a weekly
// pattern still running (or starting later). One aggregate query, no N+1.
func (s *store) ListShiftsWithUsage(ctx context.Context, today string) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT `+shiftColumns+`, COALESCE(u.n, 0)::int AS employees_using
		FROM hris.shifts
		LEFT JOIN (
		  SELECT shift_id, count(DISTINCT employee_id) AS n FROM (
		    SELECT d.shift_id, d.employee_id FROM hris.employee_shift_days d WHERE d.work_date >= $1::date
		    UNION ALL
		    SELECT w.shift_id, w.employee_id FROM hris.employee_shifts w WHERE w.effective_to IS NULL OR w.effective_to >= $1::date
		  ) src
		  JOIN hris.employees e ON e.id = src.employee_id AND e.is_active
		  WHERE shift_id IS NOT NULL
		  GROUP BY shift_id
		) u ON u.shift_id = shifts.id
		ORDER BY sort_order ASC, name ASC`, today)
}

func (s *store) CreateShift(ctx context.Context, in shiftInput) (*Row, error) {
	row, err := queryRow(ctx, s.db, `INSERT INTO hris.shifts
		(code, name, start_time, end_time, break_minutes, late_tolerance_minutes, is_overnight, sort_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING `+shiftColumns,
		strOr(in.Code), strOr(in.Name), strOr(in.StartTime), strOr(in.EndTime), intOr(in.BreakMinutes, 60),
		intOr(in.LateTolerance, 10), in.IsOvernight != nil && *in.IsOvernight, intOr(in.SortOrder, 0))
	return row, shiftCodeConflict(err, strOr(in.Code))
}

func intOr(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

func (s *store) UpdateShift(ctx context.Context, id string, in shiftInput) (bool, error) {
	var name *string
	if in.Name != nil {
		t := domain.JSTrim(*in.Name)
		name = &t
	}
	tag, err := s.db.Exec(ctx, `UPDATE hris.shifts SET
		name                   = COALESCE($2, name),
		start_time             = COALESCE($3::time, start_time),
		end_time               = COALESCE($4::time, end_time),
		break_minutes          = COALESCE($5, break_minutes),
		late_tolerance_minutes = COALESCE($6, late_tolerance_minutes),
		is_overnight           = COALESCE($7, is_overnight),
		is_active              = COALESCE($8, is_active),
		sort_order             = COALESCE($9, sort_order),
		code                   = COALESCE($10, code)
		WHERE id = $1`, id, name, in.StartTime, in.EndTime, in.BreakMinutes, in.LateTolerance,
		in.IsOvernight, in.IsActive, in.SortOrder, in.Code)
	return tag.RowsAffected() > 0, shiftCodeConflict(err, strOr(in.Code))
}

// DeleteShift deletes an unused shift, else deactivates it; "" = not found.
func (s *store) DeleteShift(ctx context.Context, id string) (string, error) {
	var used bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM hris.employee_shifts WHERE shift_id = $1
		UNION ALL
		SELECT 1 FROM hris.attendance WHERE shift_id = $1) AS used`, id).Scan(&used); err != nil {
		return "", err
	}
	if used {
		_, err := s.db.Exec(ctx, `UPDATE hris.shifts SET is_active = false WHERE id = $1`, id)
		return "Shift sudah dipakai jadwal/absensi — dinonaktifkan (tidak dihapus)", err
	}
	tag, err := s.db.Exec(ctx, `DELETE FROM hris.shifts WHERE id = $1`, id)
	if err != nil || tag.RowsAffected() == 0 {
		return "", err
	}
	return "Shift dihapus", nil
}

// ScheduleRepo is the shift master storage.
type ScheduleRepo interface {
	ListShifts(ctx context.Context) ([]*Row, error)
	ListShiftsWithUsage(ctx context.Context, today string) ([]*Row, error)
	CreateShift(ctx context.Context, in shiftInput) (*Row, error)
	UpdateShift(ctx context.Context, id string, in shiftInput) (bool, error)
	DeleteShift(ctx context.Context, id string) (string, error)
}

// shiftInput is createShiftSchema / updateShiftSchema.
type shiftInput struct {
	Code, Name, StartTime, EndTime         *string
	BreakMinutes, LateTolerance, SortOrder *int
	IsOvernight, IsActive                  *bool
}

// Shifts lists the shift master; withUsage adds employees_using per shift
// (counted from today, WIB).
func (s *Service) Shifts(ctx context.Context, withUsage bool) ([]*Row, error) {
	if withUsage {
		return s.repo.ListShiftsWithUsage(ctx, domain.TodayWIB(s.now()))
	}
	return s.repo.ListShifts(ctx)
}

func (s *Service) CreateShift(ctx context.Context, in shiftInput) (*Row, error) {
	return s.repo.CreateShift(ctx, in)
}

func (s *Service) UpdateShift(ctx context.Context, id string, in shiftInput) error {
	if in.Code != nil {
		var cur string
		if err := s.repo.querier().QueryRow(ctx, `SELECT code FROM hris.shifts WHERE id = $1`, id).Scan(&cur); err == nil &&
			(cur == "DEF" || cur == "NS") && *in.Code != cur {
			return httpx.BadRequest("Kode shift bawaan DEF/NS tidak bisa diubah")
		}
	}
	if in.IsActive != nil && !*in.IsActive {
		if err := guardShiftDeactivate(ctx, s.repo.querier(), id, domain.TodayWIB(s.clock())); err != nil {
			return err
		}
	}
	found, err := s.repo.UpdateShift(ctx, id, in)
	if err == nil && !found {
		return httpx.NotFound("Shift tidak ditemukan")
	}
	return err
}

func (s *Service) DeleteShift(ctx context.Context, id string) (string, error) {
	if err := guardShiftDeactivate(ctx, s.repo.querier(), id, domain.TodayWIB(s.clock())); err != nil {
		return "", err
	}
	msg, err := s.repo.DeleteShift(ctx, id)
	if err == nil && msg == "" {
		return "", httpx.NotFound("Shift tidak ditemukan")
	}
	return msg, err
}

/* ── Overtime SQL ────────────────────────────────────────────────────── */

// managerActiveCol is whether the employee's direct manager is active; NULL
// without one. An inactive manager counts as none (EPIC-063 R9, EPIC-066).
const managerActiveCol = `(SELECT m.is_active FROM hris.employees m WHERE m.id = employees.reporting_to) AS reporting_to_active`

// overtimeManager is the owner's reporting_to and whether that manager is active.
func overtimeManager(emp *Row) (reportingTo *string, active bool) {
	if emp == nil {
		return nil, false
	}
	return emp.StrPtr("reporting_to"), emp.Bool("reporting_to_active")
}

var overtimeSelect = `*, ` +
	embedOne("employee", "id, full_name, nip, reporting_to, "+managerActiveCol+", "+deptName, "hris.employees",
		"id = overtime_requests.employee_id") + ", " +
	embedOne("requester", "id, full_name", "hris.employees", "id = overtime_requests.requested_by") + ", " +
	embedOne("decider", "id, full_name", "hris.employees", "id = overtime_requests.decided_by")

func (s *store) SubordinateIDs(ctx context.Context, managerID string) ([]string, error) {
	// A reporting_to pointing at oneself (live data) is not a direct report
	// (EPIC-063 R2): the manager's queues never hold their own requests.
	rows, err := queryRows(ctx, s.db, `SELECT id FROM hris.employees WHERE reporting_to = $1 AND id <> $1`, managerID)
	if err != nil {
		return nil, ignoreBadInput(err)
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.Str("id")
	}
	return ids, nil
}

func (s *store) OvertimeList(ctx context.Context, f overtimeFilter) ([]*Row, error) {
	var where []string
	var args []any
	if len(f.EmployeeIDs) == 1 && !f.EmployeeSource {
		args = append(args, f.EmployeeIDs[0])
		where = append(where, fmt.Sprintf("employee_id = $%d", len(args)))
	} else if len(f.EmployeeIDs) > 0 {
		marks := make([]string, len(f.EmployeeIDs))
		for i, id := range f.EmployeeIDs {
			args = append(args, id)
			marks[i] = fmt.Sprintf("$%d", len(args))
		}
		where = append(where, "employee_id IN ("+strings.Join(marks, ", ")+")")
	}
	if f.EmployeeSource {
		where = append(where, "source = 'employee'")
	}
	if f.Status != nil {
		args = append(args, *f.Status)
		where = append(where, fmt.Sprintf("status = $%d", len(args)))
	}
	if f.DateFrom != "" {
		args = append(args, f.DateFrom, f.DateTo)
		where = append(where, fmt.Sprintf("date >= $%d AND date <= $%d", len(args)-1, len(args)))
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "WHERE " + strings.Join(where, " AND ")
	}
	return queryRows(ctx, s.db, `SELECT `+overtimeSelect+` FROM hris.overtime_requests `+whereSQL+
		` ORDER BY date DESC, created_at DESC LIMIT 200`, args...)
}

func (s *store) OvertimeTarget(ctx context.Context, id string) (*Row, error) {
	row, err := queryRow(ctx, s.db, `SELECT id, full_name, is_active FROM hris.employees WHERE id = $1`, id)
	return row, ignoreBadInput(err)
}

func (s *store) OvertimeActiveOn(ctx context.Context, employeeID, date string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM hris.overtime_requests
		WHERE employee_id = $1 AND date = $2 AND status IN ('pending', 'approved'))`, employeeID, date).Scan(&exists)
	return exists, ignoreBadInput(err)
}

func (s *store) InsertOvertime(ctx context.Context, f fields) (*Row, error) {
	return insertRow(ctx, s.db, "hris.overtime_requests", f, "*")
}

func (s *store) OvertimeForDecision(ctx context.Context, id string) (*Row, error) {
	row, err := queryRow(ctx, s.db, `SELECT *, `+
		embedOne("employee", "id, full_name, reporting_to, "+managerActiveCol, "hris.employees",
			"id = overtime_requests.employee_id")+` FROM hris.overtime_requests WHERE id = $1`, id)
	return row, ignoreBadInput(err)
}

func (s *store) DecideOvertime(ctx context.Context, id string, f fields) (*Row, error) {
	return updateOne(ctx, s.db, "hris.overtime_requests", f, fields{{"id", id}, {"status", "pending"}}, "*")
}

/* ── Shift codes (Spec 2026-10-19 §2.1) ──────────────────────────────── */

const (
	shiftCodeInvalid  = "Kode shift 1–12 huruf/angka tanpa spasi"
	shiftBuiltinFixed = "Shift bawaan DEF/NS tidak bisa dihapus atau dinonaktifkan"
)

var shiftCodeRe = regexp.MustCompile(`^[A-Z0-9]{1,12}$`)

// normalizeShiftCode upper-cases and trims a sent code; nil when absent.
func normalizeShiftCode(p *string) (*string, error) {
	if p == nil {
		return nil, nil
	}
	c := strings.ToUpper(strings.TrimSpace(*p))
	if !shiftCodeRe.MatchString(c) {
		return nil, httpx.BadRequest(shiftCodeInvalid)
	}
	return &c, nil
}

func shiftCodeConflict(err error, code string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "shifts_code_key" {
		return httpx.Conflict("Kode shift " + code + " sudah dipakai")
	}
	return err
}

// guardShiftDeactivate blocks turning off DEF/NS or a shift still on a
// schedule from today on (per-date or a running weekly pattern).
func guardShiftDeactivate(ctx context.Context, q database.Querier, id, today string) error {
	var code string
	var used int
	if err := q.QueryRow(ctx, `SELECT s.code,
		(SELECT count(*) FROM hris.employee_shift_days d WHERE d.shift_id = s.id AND d.work_date >= $2::date)
		+ (SELECT count(DISTINCT es.employee_id) FROM hris.employee_shifts es
		   WHERE es.shift_id = s.id AND (es.effective_to IS NULL OR es.effective_to >= $2::date))
		FROM hris.shifts s WHERE s.id = $1`, id, today).Scan(&code, &used); err != nil {
		if database.IsNoRows(err) {
			return httpx.NotFound("Shift tidak ditemukan")
		}
		return err
	}
	if code == "DEF" || code == "NS" {
		return httpx.BadRequest(shiftBuiltinFixed)
	}
	if used > 0 {
		return httpx.BadRequest(fmt.Sprintf("Shift masih dipakai %d jadwal mulai hari ini — ganti dulu jadwalnya", used))
	}
	return nil
}

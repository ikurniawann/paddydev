package hris

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
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
}

type overtimeFilter struct {
	EmployeeIDs      []string
	EmployeeSource   bool
	Status           *string
	DateFrom, DateTo string
}

const overtimeDuplicate = "Sudah ada pengajuan lembur pending/approved di tanggal tersebut"

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
	DayType                          *string // regular | rest_day (EPIC-055); nil = regular
}

// overtimeDayTypes: rest_day = hari istirahat mingguan, dibayar tarif libur.
var overtimeDayTypes = []string{"regular", "rest_day"}

// CreateOvertime: a request for oneself ("employee", decided by HR or the
// manager) or an HR assignment for someone else ("company", confirmed by
// the employee).
func (s *Service) CreateOvertime(ctx context.Context, a *Actor, in overtimeInput) (*Row, error) {
	targetID := a.EmployeeID
	if in.EmployeeID != nil && *in.EmployeeID != "" && *in.EmployeeID != "me" {
		targetID = in.EmployeeID
	}
	if targetID == nil {
		return nil, httpx.BadRequest("Akun ini tidak tertaut ke data karyawan")
	}
	isSelf := a.EmployeeID != nil && *targetID == *a.EmployeeID
	if !isSelf && !a.IsHR {
		return nil, httpx.Forbidden("Hanya HRD yang bisa membuat penugasan lembur untuk karyawan lain")
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
	hours := domain.OvertimeHoursFromTimes(in.StartTime, in.EndTime)
	if hours > 12 {
		return nil, httpx.BadRequest("Durasi lembur maksimal 12 jam")
	}
	if hours < 1 {
		return nil, httpx.BadRequest("Durasi lembur minimal 1 jam")
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
	f.add("source", source)
	f.add("status", "pending")
	f.add("reason", in.Reason)
	if in.DayType != nil && a.IsHR { // rest day = tarif libur: hanya HR yang menetapkan
		f.add("day_type", *in.DayType)
	}
	f.add("requested_by", a.EmployeeID)
	row, err := s.repo.InsertOvertime(ctx, f)
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

var (
	decisionStatus  = map[string]string{"approve": "approved", "reject": "rejected", "cancel": "cancelled"}
	decisionMessage = map[string]string{"approved": "Lembur disetujui", "rejected": "Lembur ditolak", "cancelled": "Pengajuan dibatalkan"}
)

// DecideOvertime approves, rejects or cancels a pending request (rules in
// domain.CanDecideOvertime); the update is guarded on status = pending.
func (s *Service) DecideOvertime(ctx context.Context, a *Actor, id, action string, rejection *string, dayType ...*string) (*Row, error) {
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
	var reportingTo *string
	if emp := ot.Child("employee"); emp != nil {
		reportingTo = emp.StrPtr("reporting_to")
	}
	allowed, reason := domain.CanDecideOvertime(domain.OvertimeActor{EmployeeID: a.EmployeeID, IsHR: a.IsHR},
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
	// HR may set the day type (rest day = tarif libur) while approving.
	if action == "approve" && a.IsHR && len(dayType) > 0 && dayType[0] != nil {
		f.add("day_type", *dayType[0])
	}
	f.add("updated_at", now)
	row, err := s.repo.DecideOvertime(ctx, id, f)
	if errors.Is(err, errNoRow) {
		return nil, httpx.Conflict("Pengajuan sudah diproses oleh orang lain")
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

func (s *Service) Shifts(ctx context.Context) ([]*Row, error) { return s.repo.ListShifts(ctx) }

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

var overtimeSelect = `*, ` +
	embedOne("employee", "id, full_name, nip, reporting_to, "+deptName, "hris.employees", "id = overtime_requests.employee_id") + ", " +
	embedOne("requester", "id, full_name", "hris.employees", "id = overtime_requests.requested_by") + ", " +
	embedOne("decider", "id, full_name", "hris.employees", "id = overtime_requests.decided_by")

func (s *store) SubordinateIDs(ctx context.Context, managerID string) ([]string, error) {
	rows, err := queryRows(ctx, s.db, `SELECT id FROM hris.employees WHERE reporting_to = $1`, managerID)
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
		embedOne("employee", "id, full_name, reporting_to", "hris.employees", "id = overtime_requests.employee_id")+
		` FROM hris.overtime_requests WHERE id = $1`, id)
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

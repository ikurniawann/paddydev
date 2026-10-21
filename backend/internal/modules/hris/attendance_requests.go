package hris

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/validate"
)

// Pengajuan absen manual / koreksi absen (EPIC-065, MOM AB-04 + AB-06, K-9):
// the employee asks with a reason, HR decides (matriks approval: "Absen
// Manual / Koreksi Absen | HR"); hris.attendance_requests.

// AttendanceRequestRepo is the request storage.
type AttendanceRequestRepo interface {
	InsertAttendanceRequest(ctx context.Context, f fields) (*Row, error)
	AttendanceRequestByID(ctx context.Context, id string) (*Row, error)
	AttendanceRequestList(ctx context.Context, f attendanceRequestFilter) ([]*Row, error)
	// SetAttendanceRequestStatus updates a request still in fromStatus;
	// errNoRow when another decision got there first.
	SetAttendanceRequestStatus(ctx context.Context, id, fromStatus string, f fields) (*Row, error)
	PendingAttendanceRequestOn(ctx context.Context, employeeID, date string) (bool, error)
	// RecordedAttendance is the employee's attendance row on date (id,
	// clock_in, clock_out, status); nil when there is none.
	RecordedAttendance(ctx context.Context, employeeID, date string) (*Row, error)
	// NotifyHR is public.notify_hrd: the in-app notification the HR
	// accounts (app role hrd) already get for attendance events (R6).
	NotifyHR(ctx context.Context, kind, title, message, link string) error
}

type attendanceRequestFilter struct {
	EmployeeID string
	Status     string
}

type attendanceRequestInput struct {
	Date              string
	ClockIn, ClockOut *string
	Reason            string
	Photo             *string // data URL, optional
}

var attendanceRequestStatuses = []string{"pending", "approved", "rejected", "cancelled"}

// attendanceRequestStatusLabels finish "Pengajuan sudah …".
var attendanceRequestStatusLabels = map[string]string{"approved": "disetujui", "rejected": "ditolak", "cancelled": "dibatalkan"}

const (
	// attendanceHref is the HR Absensi menu (badge, HR notification link).
	attendanceHref = "/dashboard/hris/attendance"
	// essAttendanceHref is the ESS Absensi page (employee notifications).
	essAttendanceHref       = "/dashboard/me/absensi"
	attendanceRequestFolder = "attendance-requests"
	pendingRequestExists    = "Masih ada pengajuan absensi yang menunggu untuk tanggal ini"
)

// CreateAttendanceRequest files the actor's own request (R4/R5) and notifies
// HR in the same transaction (R6).
func (s *Service) CreateAttendanceRequest(ctx context.Context, a *Actor, in attendanceRequestInput) (*Row, error) {
	if a.EmployeeID == nil {
		return nil, httpx.BadRequest("Akun ini tidak tertaut ke data karyawan")
	}
	employeeID := *a.EmployeeID
	// A regex-valid but non-existent date (2026-02-30) must not reach the ::date cast.
	if _, valid := domain.ParseDate(in.Date); !valid {
		return nil, httpx.BadRequest("Format tanggal tidak valid")
	}
	recorded, err := s.repo.RecordedAttendance(ctx, employeeID, in.Date)
	if err != nil {
		return nil, err
	}
	check := domain.AttendanceRequestInput{Date: in.Date, ClockIn: in.ClockIn, ClockOut: in.ClockOut, Reason: in.Reason,
		HasRow: recorded != nil}
	if recorded != nil {
		if t, found := recorded.Time("clock_in"); found {
			check.RecordedIn = &t
		}
		if t, found := recorded.Time("clock_out"); found {
			check.RecordedOut = &t
		}
	}
	kind, msg := domain.CheckAttendanceRequest(check, s.now())
	if msg != "" {
		return nil, httpx.BadRequest(msg)
	}
	if err := s.ensureAttendanceOpen(ctx, in.Date); err != nil {
		return nil, err
	}
	pending, err := s.repo.PendingAttendanceRequestOn(ctx, employeeID, in.Date)
	if err != nil {
		return nil, err
	}
	if pending {
		return nil, httpx.BadRequest(pendingRequestExists)
	}
	var path *string
	if in.Photo != nil && *in.Photo != "" {
		saved, err := s.saveImage(*in.Photo, maxAttachmentBytes, attendanceRequestFolder+"/"+employeeID,
			"Lampiran harus berupa gambar JPG/PNG/WebP", "Ukuran lampiran maksimal 5 MB")
		if err != nil {
			return nil, err
		}
		path = &saved
	}
	var f fields
	f.add("employee_id", employeeID)
	f.add("date", in.Date)
	f.add("kind", kind)
	f.add("clock_in", in.ClockIn)
	f.add("clock_out", in.ClockOut)
	f.add("reason", domain.JSTrim(in.Reason))
	f.add("attachment_path", path)
	var row *Row
	err = s.repo.InTx(ctx, func(r Repository) error {
		var err error
		if row, err = r.InsertAttendanceRequest(ctx, f); err != nil {
			return err
		}
		name := ""
		if emp := row.Child("employee"); emp != nil {
			name = emp.Str("full_name")
		}
		title, message := domain.AttendanceRequestNotice(name, kind, in.Date, in.ClockIn, in.ClockOut)
		return r.NotifyHR(ctx, "approval", title, message, attendanceHref)
	})
	if database.PgCode(err) == "23505" {
		return nil, httpx.BadRequest(pendingRequestExists)
	}
	if err != nil {
		return nil, err
	}
	return obj("data", row, "message", "Pengajuan absensi terkirim — menunggu persetujuan HR"), nil
}

// MyAttendanceRequests is the actor's own requests (ESS).
func (s *Service) MyAttendanceRequests(ctx context.Context, a *Actor) ([]*Row, error) {
	if a.EmployeeID == nil {
		return []*Row{}, nil
	}
	return s.repo.AttendanceRequestList(ctx, attendanceRequestFilter{EmployeeID: *a.EmployeeID})
}

// CancelAttendanceRequest: the requester withdraws a pending request (R4).
func (s *Service) CancelAttendanceRequest(ctx context.Context, a *Actor, id string, meta audit.Entry) (*Row, error) {
	req, err := s.repo.AttendanceRequestByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, httpx.NotFound("Pengajuan absensi tidak ditemukan")
	}
	if a.EmployeeID == nil || req.Str("employee_id") != *a.EmployeeID {
		return nil, httpx.Forbidden("Hanya pemohon yang bisa membatalkan pengajuan ini")
	}
	if err := requirePendingRequest(req); err != nil {
		return nil, err
	}
	var row *Row
	err = s.repo.InTx(ctx, func(r Repository) error {
		var f fields
		f.add("status", "cancelled")
		var err error
		if row, err = r.SetAttendanceRequestStatus(ctx, id, "pending", f); err != nil {
			return err
		}
		return auditDecision(ctx, r.querier(), a, "attendance_request.cancel", "hris.attendance_requests", id,
			attendanceRequestLabel(req), nil, map[string]any{"status": "cancelled"}, meta)
	})
	if errors.Is(err, errNoRow) {
		return nil, httpx.Conflict(decisionRace)
	}
	if err != nil {
		return nil, err
	}
	return obj("data", row, "message", "Pengajuan absensi dibatalkan"), nil
}

// requirePendingRequest: only a pending request is cancelled or decided.
func requirePendingRequest(req *Row) error {
	if status := req.Str("status"); status != "pending" {
		return httpx.BadRequest("Pengajuan sudah "+attendanceRequestStatusLabels[status], obj("current_status", status))
	}
	return nil
}

// attendanceRequestLabel is "Nama · 2 Okt 2026" for the audit log.
func attendanceRequestLabel(req *Row) string {
	label := domain.ShortDateID(req.Str("date"))
	if emp := req.Child("employee"); emp != nil {
		label = emp.Str("full_name") + " · " + label
	}
	return label
}

/* ── SQL ─────────────────────────────────────────────────────────────── */

// attendanceRequestColumns embed the requester and the attendance recorded
// on the date now (HR compares it with the requested times).
var attendanceRequestColumns = `q.id, q.employee_id, q.date::text AS date, q.kind, q.clock_in, q.clock_out, q.reason,
	q.attachment_path, q.status, q.attendance_id, q.decided_by, q.decided_by_name, q.decided_at, q.rejection_reason,
	q.created_at, q.updated_at, ` +
	embedOne("employee", "id, full_name, nip", "hris.employees", "id = q.employee_id") + `,
	(SELECT json_build_object('id', a.id, 'clock_in', a.clock_in, 'clock_out', a.clock_out, 'status', a.status)
	   FROM hris.attendance a WHERE a.employee_id = q.employee_id AND a.date = q.date) AS recorded`

func (s *store) InsertAttendanceRequest(ctx context.Context, f fields) (*Row, error) {
	row, err := insertRow(ctx, s.db, "hris.attendance_requests", f, "id")
	if err != nil || row == nil {
		return nil, err
	}
	return s.AttendanceRequestByID(ctx, row.Str("id"))
}

func (s *store) AttendanceRequestByID(ctx context.Context, id string) (*Row, error) {
	row, err := queryRow(ctx, s.db, `SELECT `+attendanceRequestColumns+` FROM hris.attendance_requests q WHERE q.id = $1`, id)
	return row, ignoreBadInput(err)
}

func (s *store) AttendanceRequestList(ctx context.Context, f attendanceRequestFilter) ([]*Row, error) {
	where := []string{"true"}
	var args []any
	if f.EmployeeID != "" {
		args = append(args, f.EmployeeID)
		where = append(where, fmt.Sprintf("q.employee_id = $%d", len(args)))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		where = append(where, fmt.Sprintf("q.status = $%d", len(args)))
	}
	rows, err := queryRows(ctx, s.db, `SELECT `+attendanceRequestColumns+` FROM hris.attendance_requests q WHERE `+
		strings.Join(where, " AND ")+` ORDER BY (q.status = 'pending') DESC, q.date DESC, q.created_at DESC LIMIT 200`, args...)
	return rows, ignoreBadInput(err)
}

func (s *store) SetAttendanceRequestStatus(ctx context.Context, id, fromStatus string, f fields) (*Row, error) {
	if _, err := updateOne(ctx, s.db, "hris.attendance_requests", f, fields{{"id", id}, {"status", fromStatus}}, "id"); err != nil {
		return nil, err
	}
	return s.AttendanceRequestByID(ctx, id)
}

func (s *store) PendingAttendanceRequestOn(ctx context.Context, employeeID, date string) (bool, error) {
	var pending bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM hris.attendance_requests
		WHERE employee_id = $1 AND date = $2::date AND status = 'pending')`, employeeID, date).Scan(&pending)
	return pending, err
}

func (s *store) RecordedAttendance(ctx context.Context, employeeID, date string) (*Row, error) {
	row, err := queryRow(ctx, s.db, `SELECT id::text AS id, clock_in, clock_out, status::text AS status
		FROM hris.attendance WHERE employee_id = $1 AND date = $2::date`, employeeID, date)
	return row, ignoreBadInput(err)
}

func (s *store) NotifyHR(ctx context.Context, kind, title, message, link string) error {
	_, err := s.db.Exec(ctx, `SELECT public.notify_hrd($1, $2, $3, $4)`, title, message, kind, link)
	return err
}

/* ── HTTP ────────────────────────────────────────────────────────────── */

// requestID canonicalises a path id (lower-case UUID) before any lookup.
func requestID(raw string) (string, error) {
	id, valid := validate.CanonicalUUID(raw)
	if !valid {
		return "", httpx.BadRequest("ID pengajuan tidak valid")
	}
	return id, nil
}

var requestClockOpts = validate.StrOpts{Check: func(s string) (string, string, bool) {
	return "invalid_format", "Format jam harus HH:MM", domain.ValidClock(s)
}}

// POST /api/hris/attendance-requests { date, clock_in?, clock_out?, reason, photo? }
func (h handlers) createAttendanceRequest(w http.ResponseWriter, r *http.Request, a *Actor) error {
	f, err := readUploadJSON(r, "")
	if err != nil {
		return err
	}
	in := attendanceRequestInput{
		Date: strOr(reqStr(f, "date", "Tanggal wajib diisi",
			validate.StrOpts{Check: matches(domain.DateRe, "Format tanggal tidak valid")})),
		ClockIn:  f.Str("clock_in", nullish, requestClockOpts),
		ClockOut: f.Str("clock_out", nullish, requestClockOpts),
		Reason:   strOr(f.Str("reason", nullish, validate.StrOpts{Max: 1000})),
		Photo:    f.Str("photo", nullish, validate.StrOpts{}),
	}
	if err := formErr(f, ""); err != nil {
		return err
	}
	out, err := h.svc.CreateAttendanceRequest(r.Context(), a, in)
	if err != nil {
		return err
	}
	return reply(w, http.StatusCreated, out)
}

// GET /api/hris/me/attendance-requests
func (h handlers) myAttendanceRequests(w http.ResponseWriter, r *http.Request, a *Actor) error {
	rows, err := h.svc.MyAttendanceRequests(r.Context(), a)
	if err != nil {
		return err
	}
	return ok(w, dataOf(rows))
}

// POST /api/hris/attendance-requests/{id}/cancel
func (h handlers) cancelAttendanceRequest(w http.ResponseWriter, r *http.Request, a *Actor) error {
	id, err := requestID(r.PathValue("id"))
	if err != nil {
		return err
	}
	out, err := h.svc.CancelAttendanceRequest(r.Context(), a, id, auditMeta(r))
	if err != nil {
		return err
	}
	return ok(w, out)
}

// GET /api/hris/attendance-requests/attachment/attendance-requests/<employeeId>/<file>:
// the requester and HRD read it (EPIC-065 P6: not hiring_manager, who cannot
// list or decide requests either).
func (h handlers) attendanceRequestAttachment(w http.ResponseWriter, r *http.Request, a *Actor) error {
	return h.ownedFileFor(w, r, a, a.IsHRD, "/api/hris/attendance-requests/attachment/", attendanceRequestFolder)
}

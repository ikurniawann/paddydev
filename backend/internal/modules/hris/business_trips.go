package hris

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/validate"
)

// Perjalanan Dinas (EPIC-062 R7/R8, MOM AB-02): hris.business_trips.

// TripRepo is the business trip storage.
type TripRepo interface {
	TripList(ctx context.Context, f tripFilter) ([]*Row, error)
	TripByID(ctx context.Context, id string) (*Row, error)
	InsertTrip(ctx context.Context, f fields) (*Row, error)
	DecideTrip(ctx context.Context, id, fromStatus string, f fields) (*Row, error)
	ApprovedTripOn(ctx context.Context, employeeID, date string) (bool, error)
	RelabelTripAttendance(ctx context.Context, employeeID, start, end string) error
}

type tripFilter struct {
	EmployeeIDs []string
	Status      string
}

type tripInput struct {
	EmployeeID                               *string
	StartDate, EndDate, Destination, Purpose string
}

var (
	tripStatuses = []string{"pending", "approved", "rejected", "cancelled"}
	tripNext     = map[string]string{"approve": "approved", "reject": "rejected", "cancel": "cancelled"}
	tripMessages = map[string]string{"approved": "Perjalanan Dinas disetujui", "rejected": "Perjalanan Dinas ditolak",
		"cancelled": "Perjalanan Dinas dibatalkan"}
)

const tripOverlap = "Sudah ada Perjalanan Dinas menunggu/disetujui yang beririsan dengan tanggal ini"

// ListTrips: scope=approvals → the direct reports' trips; HRD (IsHRD) → all (or one
// employee, "me" = own); others → own.
func (s *Service) ListTrips(ctx context.Context, a *Actor, scope, status, employeeID string) ([]*Row, error) {
	f := tripFilter{Status: status}
	switch {
	case scope == "approvals":
		if a.EmployeeID == nil {
			return []*Row{}, nil
		}
		ids, err := s.repo.SubordinateIDs(ctx, *a.EmployeeID)
		if err != nil || len(ids) == 0 {
			return []*Row{}, err
		}
		f.EmployeeIDs = ids
	case a.IsHRD && employeeID == "me":
		if a.EmployeeID == nil {
			return []*Row{}, nil
		}
		f.EmployeeIDs = []string{*a.EmployeeID}
	case a.IsHRD:
		if employeeID != "" {
			f.EmployeeIDs = []string{employeeID}
		}
	default:
		if a.EmployeeID == nil {
			return []*Row{}, nil
		}
		f.EmployeeIDs = []string{*a.EmployeeID}
	}
	return s.repo.TripList(ctx, f)
}

// CreateTrip: a request for oneself (pending) or an HR assignment for
// another employee (approved at once).
func (s *Service) CreateTrip(ctx context.Context, a *Actor, in tripInput, meta audit.Entry) (*Row, error) {
	target := a.EmployeeID
	if in.EmployeeID != nil && *in.EmployeeID != "" && *in.EmployeeID != "me" {
		target = in.EmployeeID
	}
	if target == nil {
		return nil, httpx.BadRequest("Akun ini tidak tertaut ke data karyawan")
	}
	self := a.EmployeeID != nil && *target == *a.EmployeeID
	if !self && !a.IsHRD {
		return nil, httpx.Forbidden("Hanya HRD yang bisa mencatat Perjalanan Dinas karyawan lain")
	}
	if msg := domain.ValidateBusinessTripDates(in.StartDate, in.EndDate, domain.TodayWIB(s.now())); msg != "" {
		return nil, httpx.BadRequest(msg)
	}
	emp, err := s.repo.OvertimeTarget(ctx, *target)
	if err != nil {
		return nil, err
	}
	if emp == nil || emp.Get("is_active") == false {
		return nil, httpx.NotFound("Karyawan tidak ditemukan/nonaktif")
	}
	status := "pending"
	if !self {
		status = "approved"
	}
	var f fields
	f.add("employee_id", *target)
	f.add("start_date", in.StartDate)
	f.add("end_date", in.EndDate)
	f.add("destination", in.Destination)
	f.add("purpose", in.Purpose)
	f.add("status", status)
	f.add("requested_by", a.EmployeeID)
	if status == "approved" {
		f.add("decided_by", a.EmployeeID)
		f.add("decided_by_name", actorName(a))
		f.add("decided_at", s.clock())
	}
	var row *Row
	err = s.repo.InTx(ctx, func(r Repository) error {
		var err error
		if row, err = r.InsertTrip(ctx, f); err != nil || status != "approved" {
			return err
		}
		if err := r.RelabelTripAttendance(ctx, *target, in.StartDate, in.EndDate); err != nil {
			return err
		}
		return s.auditTrip(ctx, r, a, "business_trip.assign", nil, row, meta)
	})
	if database.PgCode(err) == "23P01" {
		return nil, httpx.BadRequest(tripOverlap)
	}
	if err != nil {
		return nil, err
	}
	message := "Pengajuan Perjalanan Dinas terkirim — menunggu persetujuan atasan"
	if status == "approved" {
		message = "Perjalanan Dinas dicatat dan disetujui"
	}
	return obj("data", row, "message", message), nil
}

// DecideTrip approves, rejects or cancels (domain.CanDecideBusinessTrip);
// the update is guarded on the status read, and an approval relabels the
// range's existing attendance (R8).
func (s *Service) DecideTrip(ctx context.Context, a *Actor, id, action string, rejection *string, meta audit.Entry) (*Row, error) {
	trip, err := s.repo.TripByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if trip == nil {
		return nil, httpx.NotFound("Perjalanan Dinas tidak ditemukan")
	}
	status := trip.Str("status")
	if action != "cancel" && status != "pending" {
		return nil, httpx.BadRequest("Pengajuan sudah "+status, obj("current_status", status))
	}
	var reportingTo *string
	if emp := trip.Child("employee"); emp != nil {
		reportingTo = emp.StrPtr("reporting_to")
	}
	allowed, reason := domain.CanDecideBusinessTrip(domain.TripActor{EmployeeID: a.EmployeeID, IsHR: a.IsHRD},
		domain.TripRef{EmployeeID: trip.Str("employee_id"), ReportingTo: reportingTo, Status: status}, action)
	if !allowed {
		return nil, httpx.Forbidden(reason)
	}
	if action == "reject" && (rejection == nil || domain.JSTrim(*rejection) == "") {
		return nil, httpx.BadRequest("Alasan penolakan wajib diisi")
	}
	next := tripNext[action]
	var f fields
	f.add("status", next)
	f.add("decided_by", a.EmployeeID)
	f.add("decided_by_name", actorName(a))
	f.add("decided_at", s.clock())
	if action == "reject" {
		f.add("rejection_reason", domain.JSTrim(*rejection))
	}
	var row *Row
	err = s.repo.InTx(ctx, func(r Repository) error {
		var err error
		if row, err = r.DecideTrip(ctx, id, status, f); err != nil {
			return err
		}
		if next == "approved" {
			if err := r.RelabelTripAttendance(ctx, trip.Str("employee_id"), trip.Str("start_date"), trip.Str("end_date")); err != nil {
				return err
			}
		}
		return s.auditTrip(ctx, r, a, "business_trip."+action, trip, row, meta)
	})
	if errors.Is(err, errNoRow) {
		return nil, httpx.Conflict("Pengajuan sudah diproses oleh orang lain")
	}
	if err != nil {
		return nil, err
	}
	return obj("data", row, "message", tripMessages[next]), nil
}

// auditTrip records a decision or HR assignment (LA-01).
func (s *Service) auditTrip(ctx context.Context, r Repository, a *Actor, action string, before, after *Row, meta audit.Entry) error {
	meta.ActorID, meta.ActorName = a.UserID, ptr(actorName(a))
	meta.Action, meta.Entity, meta.EntityID = action, "hris.business_trips", ptr(after.Str("id"))
	label := after.Str("start_date") + " s.d. " + after.Str("end_date")
	if emp := after.Child("employee"); emp != nil {
		label = emp.Str("full_name") + " · " + label
	}
	meta.EntityLabel = ptr(label)
	if before != nil {
		meta.Before = map[string]any{"status": before.Str("status")}
	}
	meta.After = map[string]any{"status": after.Str("status"), "rejection_reason": after.Get("rejection_reason")}
	return audit.Write(ctx, r.querier(), meta)
}

/* ── SQL ─────────────────────────────────────────────────────────────── */

var tripColumns = `t.id, t.employee_id, t.start_date::text AS start_date, t.end_date::text AS end_date,
	t.destination, t.purpose, t.status, t.requested_by, t.decided_by, t.decided_by_name, t.decided_at,
	t.rejection_reason, t.created_at, t.updated_at, ` +
	embedOne("employee", "id, full_name, nip, reporting_to", "hris.employees", "id = t.employee_id")

func (s *store) TripList(ctx context.Context, f tripFilter) ([]*Row, error) {
	where := []string{"true"}
	var args []any
	if len(f.EmployeeIDs) > 0 {
		args = append(args, f.EmployeeIDs)
		where = append(where, fmt.Sprintf("t.employee_id = ANY($%d::uuid[])", len(args)))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		where = append(where, fmt.Sprintf("t.status = $%d", len(args)))
	}
	rows, err := queryRows(ctx, s.db, `SELECT `+tripColumns+` FROM hris.business_trips t WHERE `+
		strings.Join(where, " AND ")+` ORDER BY t.start_date DESC, t.created_at DESC LIMIT 200`, args...)
	return rows, ignoreBadInput(err)
}

func (s *store) TripByID(ctx context.Context, id string) (*Row, error) {
	row, err := queryRow(ctx, s.db, `SELECT `+tripColumns+` FROM hris.business_trips t WHERE t.id = $1`, id)
	return row, ignoreBadInput(err)
}

func (s *store) InsertTrip(ctx context.Context, f fields) (*Row, error) {
	row, err := insertRow(ctx, s.db, "hris.business_trips", f, "id")
	if err != nil || row == nil {
		return nil, err
	}
	return s.TripByID(ctx, row.Str("id"))
}

func (s *store) DecideTrip(ctx context.Context, id, fromStatus string, f fields) (*Row, error) {
	if _, err := updateOne(ctx, s.db, "hris.business_trips", f, fields{{"id", id}, {"status", fromStatus}}, "id"); err != nil {
		return nil, err
	}
	return s.TripByID(ctx, id)
}

func (s *store) ApprovedTripOn(ctx context.Context, employeeID, date string) (bool, error) {
	var ok bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM hris.business_trips
		WHERE employee_id = $1 AND status = 'approved' AND $2::date BETWEEN start_date AND end_date)`,
		employeeID, date).Scan(&ok)
	return ok, err
}

// RelabelTripAttendance turns the range's invalid/unchecked geofence
// verdicts into dinas, outside closed attendance periods (EPIC-062 R8).
func (s *store) RelabelTripAttendance(ctx context.Context, employeeID, start, end string) error {
	_, err := s.db.Exec(ctx, `UPDATE hris.attendance a SET
		  clock_in_geofence = CASE WHEN a.clock_in_geofence IN ('invalid', 'unchecked') THEN 'dinas' ELSE a.clock_in_geofence END,
		  clock_out_geofence = CASE WHEN a.clock_out_geofence IN ('invalid', 'unchecked') THEN 'dinas' ELSE a.clock_out_geofence END
		WHERE a.employee_id = $1 AND a.date BETWEEN $2::date AND $3::date
		  AND (a.clock_in_geofence IN ('invalid', 'unchecked') OR a.clock_out_geofence IN ('invalid', 'unchecked'))
		  AND NOT EXISTS (SELECT 1 FROM hris.attendance_periods p
		                  WHERE p.status = 'closed' AND a.date BETWEEN p.start_date AND p.end_date)`,
		employeeID, start, end)
	return err
}

/* ── HTTP ────────────────────────────────────────────────────────────── */

// GET /api/hris/business-trips?scope=approvals&status&employee_id
func (h handlers) listTrips(w http.ResponseWriter, r *http.Request, a *Actor) error {
	q := r.URL.Query()
	status := q.Get("status")
	if status != "" && !slices.Contains(tripStatuses, status) {
		return httpx.BadRequest("Status tidak valid")
	}
	employeeID := q.Get("employee_id")
	if employeeID != "" && employeeID != "me" && !domain.IsUUID(employeeID) {
		return httpx.BadRequest("ID karyawan tidak valid")
	}
	rows, err := h.svc.ListTrips(r.Context(), a, q.Get("scope"), status, employeeID)
	if err != nil {
		return err
	}
	return ok(w, dataOf(rows))
}

// POST /api/hris/business-trips { employee_id?, start_date, end_date, destination, purpose }
func (h handlers) createTrip(w http.ResponseWriter, r *http.Request, a *Actor) error {
	const message = "Validation failed"
	f, err := readForm(r, message)
	if err != nil {
		return err
	}
	var in tripInput
	in.EmployeeID = f.Str("employee_id", optional, validate.StrOpts{Check: func(s string) (string, string, bool) {
		return "invalid_union", "Invalid input", s == "me" || domain.UUIDRe.MatchString(s)
	}})
	in.StartDate = strOr(f.Str("start_date", validate.Rule{}, validate.StrOpts{Check: regexOf(domain.DateRe)}))
	in.EndDate = strOr(f.Str("end_date", validate.Rule{}, validate.StrOpts{Check: regexOf(domain.DateRe)}))
	in.Destination = strOr(f.Str("destination", validate.Rule{}, validate.StrOpts{Trim: true, Max: 200,
		Check: minLen(3, "Tujuan minimal 3 karakter")}))
	in.Purpose = strOr(f.Str("purpose", validate.Rule{}, validate.StrOpts{Trim: true, Max: 1000,
		Check: minLen(5, "Keperluan minimal 5 karakter")}))
	if err := formErr(f, message); err != nil {
		return err
	}
	out, err := h.svc.CreateTrip(r.Context(), a, in, auditMeta(r))
	if err != nil {
		return err
	}
	return ok(w, out)
}

// POST /api/hris/business-trips/{id}/decide { action, rejection_reason? }
func (h handlers) decideTrip(w http.ResponseWriter, r *http.Request, a *Actor) error {
	id, err := requireUUID(r.PathValue("id"), "ID Perjalanan Dinas tidak valid")
	if err != nil {
		return err
	}
	const message = "Validation failed"
	f, err := readForm(r, message)
	if err != nil {
		return err
	}
	action := f.Enum("action", validate.Rule{}, []string{"approve", "reject", "cancel"})
	rejection := f.Str("rejection_reason", optional, validate.StrOpts{Max: 500})
	if err := formErr(f, message); err != nil {
		return err
	}
	out, err := h.svc.DecideTrip(r.Context(), a, id, *action, rejection, auditMeta(r))
	if err != nil {
		return err
	}
	return ok(w, out)
}

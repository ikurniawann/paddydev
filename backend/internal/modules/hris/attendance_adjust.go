package hris

import (
	"context"
	"net/http"
	"strings"
	"time"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/validate"
)

// Manual attendance adjustment by HR before a period is closed: add a
// missing row, correct clock-in/out, status, store, lateness and overtime,
// or delete a row. Every change carries a reason and is logged in
// hris.attendance_adjustments (before/after) and audit.audit_log; the row
// is marked adjusted. Lateness is recomputed with the clock-in rule
// (domain.Lateness on the shift snapshot) unless HR overrides it; work
// hours come from the table trigger, as for clock-out.

var attendanceStatuses = []string{"present", "late", "absent", "half-day", "remote"}

// adjustKeys are the columns an adjustment log snapshots.
var adjustKeys = []string{"date", "clock_in", "clock_out", "status", "branch_id", "shift_id", "is_late", "late_minutes",
	"work_hours", "is_overtime", "overtime_hours", "notes"}

func adjustSnapshot(r *Row) *Row {
	if r == nil {
		return nil
	}
	out := newRow()
	for _, k := range adjustKeys {
		out.Set(k, r.Get(k))
	}
	return out
}

// adjustment is a parsed create/update body. Clock times are "HH:MM" WIB on
// the attendance date; a nil pointer with the Set flag clears the value.
type adjustment struct {
	EmployeeID, Date string
	ClockIn          opt[string]
	ClockOut         opt[string]
	Status           opt[string]
	Branch           opt[string]
	LateMinutes      opt[float64]
	OvertimeHours    opt[float64]
	Notes            opt[string]
	Reason           string
}

func (in adjustment) changesData() bool {
	return in.ClockIn.Sent || in.ClockOut.Sent || in.Status.Sent || in.Branch.Sent || in.LateMinutes.Sent ||
		in.OvertimeHours.Sent || in.Notes.Sent
}

func parseAdjustment(f *validate.Form, create bool) adjustment {
	var in adjustment
	if create {
		if v := f.UUID("employee_id", validate.Rule{}); v != nil {
			in.EmployeeID = *v
		}
		if v := reqStr(f, "date", "Tanggal absensi wajib diisi", validate.StrOpts{Check: matches(domain.DateRe, "Format tanggal tidak valid")}); v != nil {
			in.Date = *v
		}
	}
	clock := validate.StrOpts{Check: func(s string) (string, string, bool) {
		return "invalid_format", "Format jam harus HH:MM", domain.ValidClock(s)
	}}
	in.ClockIn = str(f, "clock_in", nullish, clock)
	in.ClockOut = str(f, "clock_out", nullish, clock)
	in.Status = str(f, "status", optional, enumOf(attendanceStatuses, "Status absensi tidak valid"))
	in.Branch = str(f, "branch_id", nullish, uuidOf)
	in.LateMinutes = number(f, "late_minutes", nullish, validate.NumOpts{Integer: true, Min: validate.Bound(0), Max: validate.Bound(1440)})
	in.OvertimeHours = number(f, "overtime_hours", nullish, validate.NumOpts{Min: validate.Bound(0), Max: validate.Bound(24)})
	in.Notes = str(f, "notes", nullish, validate.StrOpts{Max: 5000})
	if v := f.Str("reason", nullish, validate.StrOpts{Max: 1000}); v != nil {
		in.Reason = domain.JSTrim(*v)
	}
	return in
}

func requireReason(reason string) error {
	if len([]rune(reason)) < 5 {
		return httpx.BadRequest("Alasan penyesuaian wajib diisi (minimal 5 karakter)")
	}
	return nil
}

// shiftTimesByID reads a shift's schedule times.
func (s *Service) shiftTimesByID(ctx context.Context, id string) (*domain.ShiftTimes, error) {
	row, err := queryRow(ctx, s.repo.querier(), `SELECT start_time::text, end_time::text, is_overnight, late_tolerance_minutes
		FROM hris.shifts WHERE id = $1`, id)
	if err != nil || row == nil || row.Str("start_time") == "" || row.Str("end_time") == "" {
		return nil, err
	}
	return &domain.ShiftTimes{StartTime: row.Str("start_time"), EndTime: row.Str("end_time"),
		IsOvernight: row.Bool("is_overnight"), LateToleranceMinutes: int(row.Num("late_tolerance_minutes"))}, nil
}

// applyClockAndLateness resolves the clock times and lateness of an
// adjustment into f. current is the stored row (nil on create).
func (s *Service) applyClockAndLateness(ctx context.Context, f *fields, in adjustment, employeeID, date string, current *Row) error {
	var clockIn *time.Time
	if current != nil {
		if t, ok := current.Time("clock_in"); ok {
			clockIn = &t
		}
	}
	if in.ClockIn.Sent {
		clockIn = nil
		if in.ClockIn.Val != nil {
			t, _ := domain.ClockAt(date, *in.ClockIn.Val)
			clockIn = &t
		}
		f.set("clock_in", clockIn)
	}
	if in.ClockOut.Sent || (in.ClockIn.Sent && current != nil && current.Get("clock_out") != nil) {
		var out *time.Time
		switch {
		case in.ClockOut.Sent && in.ClockOut.Val != nil:
			t, _ := domain.ClockOutAt(date, *in.ClockOut.Val, clockIn)
			out = &t
		case !in.ClockOut.Sent:
			// clock-in moved: keep the stored clock-out wall time.
			stored, _ := current.Time("clock_out")
			t, _ := domain.ClockOutAt(date, domain.ClockWIBText(stored), clockIn)
			out = &t
		}
		if out != nil && clockIn == nil {
			return httpx.BadRequest("Jam pulang butuh jam masuk")
		}
		if out != nil && clockIn != nil && out.Sub(*clockIn) > 24*time.Hour {
			return httpx.BadRequest("Jam pulang lebih dari 24 jam setelah jam masuk")
		}
		f.set("clock_out", out)
		if out == nil {
			f.set("work_hours", nil) // the trigger only fills it when both times are set
		}
	}

	// Lateness: a manual value wins; otherwise the clock-in rule on the
	// row's shift snapshot (or the schedule of the date when it has none).
	if in.LateMinutes.Sent && in.LateMinutes.Val != nil {
		f.set("late_minutes", int(*in.LateMinutes.Val))
		f.set("is_late", *in.LateMinutes.Val > 0)
		return nil
	}
	if current != nil && !in.ClockIn.Sent && !in.LateMinutes.Sent {
		return nil
	}
	var times *domain.ShiftTimes
	if current != nil && current.Str("shift_id") != "" {
		t, err := s.shiftTimesByID(ctx, current.Str("shift_id"))
		if err != nil {
			return err
		}
		times = t
	}
	if times == nil {
		snap, err := s.shiftSnapshot(ctx, employeeID, date)
		if err != nil {
			return err
		}
		if snap != nil {
			times = &snap.times
			start, end := domain.ScheduledWindow(date, snap.times)
			f.set("shift_id", snap.shiftID)
			f.set("scheduled_start", start)
			f.set("scheduled_end", end)
		}
	}
	late, minutes := false, 0
	if times != nil && clockIn != nil {
		late, minutes = domain.Lateness(*clockIn, date, *times)
	}
	f.set("is_late", late)
	f.set("late_minutes", minutes)
	return nil
}

func (s *Service) adjustBranch(ctx context.Context, f *fields, branch opt[string]) error {
	if !branch.Sent {
		return nil
	}
	if branch.Val != nil {
		exists, err := s.repo.BranchExists(ctx, *branch.Val)
		if err != nil {
			return err
		}
		if !exists {
			return httpx.BadRequest("Toko tidak ditemukan")
		}
	}
	f.set("branch_id", branch.Val)
	return nil
}

func applyOvertime(f *fields, in adjustment) {
	if !in.OvertimeHours.Sent {
		return
	}
	hours := 0.0
	if in.OvertimeHours.Val != nil {
		hours = domain.Round2(*in.OvertimeHours.Val)
	}
	f.set("overtime_hours", hours)
	f.set("is_overtime", hours > 0)
}

func changedKeys(before, after *Row) []string {
	var out []string
	for _, k := range adjustKeys {
		var b, a any
		if before != nil {
			b = before.Get(k)
		}
		if after != nil {
			a = after.Get(k)
		}
		bj, _ := marshal(b)
		aj, _ := marshal(a)
		if string(bj) != string(aj) {
			out = append(out, k)
		}
	}
	if out == nil {
		out = []string{}
	}
	return out
}

// logAdjustment writes the adjustment log and the audit row on r.
func (s *Service) logAdjustment(ctx context.Context, r Repository, a *Actor, action, attendanceID, employeeID, date, reason string,
	before, after *Row, meta audit.Entry) error {
	b, af := adjustSnapshot(before), adjustSnapshot(after)
	changed := changedKeys(b, af)
	var bj, aj any
	if b != nil {
		raw, _ := marshal(b)
		bj = string(raw)
	}
	if af != nil {
		raw, _ := marshal(af)
		aj = string(raw)
	}
	if _, err := r.querier().Exec(ctx, `INSERT INTO hris.attendance_adjustments
		(attendance_id, employee_id, date, action, reason, changed_fields, before, after, actor_id, actor_name, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8::jsonb, $9, $10, clock_timestamp())`,
		attendanceID, employeeID, date, action, reason, changed, bj, aj, optionalUser(a), actorName(a)); err != nil {
		return err
	}
	name, _ := r.EmployeeName(ctx, employeeID)
	label := date
	if name != nil {
		label = *name + " · " + date
	}
	meta.ActorID, meta.ActorName = a.UserID, ptr(actorName(a))
	meta.Action, meta.Entity, meta.EntityID = "attendance."+action, "hris.attendance", &attendanceID
	meta.EntityLabel = &label
	if b != nil {
		meta.Before = b
	}
	if af != nil {
		meta.After = af
	}
	meta.Reason = &reason
	return audit.Write(ctx, r.querier(), meta)
}

// mayWriteAttendance: the HR adjustment roles (canManageAttendance), or the
// approval of an employee request (also is set only by
// DecideAttendanceRequest), whose decider holds "approve" on the Absensi
// leaf (EPIC-067 R11) whatever their app role.
func mayWriteAttendance(a *Actor, also func(Repository, *Row) error) bool {
	return canManageAttendance(a) || (also != nil && a.CanApprove(iam.LeafAbsensi))
}

// CreateManualAttendance adds a missing attendance row for an employee.
func (s *Service) CreateManualAttendance(ctx context.Context, a *Actor, in adjustment, meta audit.Entry) (*Row, error) {
	return s.createManualAttendance(ctx, a, in, meta, nil)
}

// createManualAttendance is CreateManualAttendance with also run on the
// same transaction after the row and its logs are written (EPIC-065 R3: an
// approved employee request records its decision atomically with the row).
func (s *Service) createManualAttendance(ctx context.Context, a *Actor, in adjustment, meta audit.Entry,
	also func(Repository, *Row) error) (*Row, error) {
	if !mayWriteAttendance(a, also) {
		return nil, httpx.Forbidden("Hanya HRD/admin yang bisa menambah absensi manual")
	}
	if err := requireReason(in.Reason); err != nil {
		return nil, err
	}
	today := domain.TodayWIB(s.now())
	if in.Date > today {
		return nil, httpx.BadRequest("Tanggal absensi tidak boleh di masa depan")
	}
	status := "present"
	if in.Status.Val != nil {
		status = *in.Status.Val
	}
	if status != "absent" && (in.ClockIn.Val == nil) {
		return nil, httpx.BadRequest("Jam masuk wajib diisi (kecuali status Tidak hadir)")
	}
	name, err := s.repo.EmployeeName(ctx, in.EmployeeID)
	if err != nil {
		return nil, ignoreBadInput(err)
	}
	if name == nil {
		return nil, httpx.NotFound("Karyawan tidak ditemukan")
	}
	if err := s.ensureAttendanceOpen(ctx, in.Date); err != nil {
		return nil, err
	}
	existing, err := s.repo.AttendanceIDOn(ctx, in.EmployeeID, in.Date)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, httpx.Conflict("Absensi karyawan ini pada tanggal " + domain.ShortDateID(in.Date) +
			" sudah ada — ubah baris yang ada")
	}
	var f fields
	f.add("employee_id", in.EmployeeID)
	f.add("date", in.Date)
	f.add("status", status)
	f.add("shift_id", nil)
	f.add("scheduled_start", nil)
	f.add("scheduled_end", nil)
	in.ClockIn.Sent, in.ClockOut.Sent = true, in.ClockOut.Val != nil
	if err := s.applyClockAndLateness(ctx, &f, in, in.EmployeeID, in.Date, nil); err != nil {
		return nil, err
	}
	branch := in.Branch
	if !branch.Sent || branch.Val == nil {
		_, def, err := s.ClockPlacements(ctx, in.EmployeeID, in.Date)
		if err != nil {
			return nil, err
		}
		if !branch.Sent {
			branch = opt[string]{Val: def, Sent: true}
		}
	}
	if err := s.adjustBranch(ctx, &f, branch); err != nil {
		return nil, err
	}
	applyOvertime(&f, in)
	f.set("notes", nilIfEmpty(in.Notes.Val))
	now := s.clock()
	f.set("is_manual", true)
	f.set("adjusted_at", now)
	f.set("adjusted_by", optionalUser(a))
	f.set("adjusted_by_name", actorName(a))
	f.set("adjustment_count", 1)
	var row *Row
	err = s.repo.InTx(ctx, func(r Repository) error {
		var err error
		if row, err = r.InsertAttendance(ctx, f); err != nil {
			return periodWriteError(err)
		}
		if err := s.logAdjustment(ctx, r, a, "create", row.Str("id"), in.EmployeeID, in.Date, in.Reason, nil, row, meta); err != nil {
			return err
		}
		return runAlso(also, r, row)
	})
	if err != nil {
		return nil, err
	}
	return row, nil
}

// AdjustAttendance applies an HR correction. Data changes need the manage
// role and a reason; validated=true stamps the validator as before.
func (s *Service) AdjustAttendance(ctx context.Context, a *Actor, id string, in adjustment, validation fields,
	validated bool, meta audit.Entry) (*Row, error) {
	return s.adjustAttendance(ctx, a, id, in, validation, validated, meta, nil)
}

// adjustAttendance is AdjustAttendance with also run on the same transaction
// after a data change is written and logged (EPIC-065 R3).
func (s *Service) adjustAttendance(ctx context.Context, a *Actor, id string, in adjustment, validation fields,
	validated bool, meta audit.Entry, also func(Repository, *Row) error) (*Row, error) {
	current, err := s.repo.AttendanceRecord(ctx, id)
	if err != nil || current == nil {
		return nil, errOr(err, httpx.NotFound("Attendance not found"))
	}
	data := in.changesData()
	if data {
		if !mayWriteAttendance(a, also) {
			return nil, httpx.Forbidden("Hanya HRD/admin yang bisa menyesuaikan absensi")
		}
		if err := requireReason(in.Reason); err != nil {
			return nil, err
		}
	}
	date := attendanceDate(current)
	if err := s.ensureAttendanceOpen(ctx, date); err != nil {
		return nil, err
	}
	employeeID := current.Str("employee_id")
	f := validation
	if validated {
		f.add("validated_by", a.EmployeeID)
		f.add("validated_at", s.clock())
	}
	if data {
		if in.Status.Sent {
			f.set("status", in.Status.Val)
		}
		if err := s.applyClockAndLateness(ctx, &f, in, employeeID, date, current); err != nil {
			return nil, err
		}
		if err := s.adjustBranch(ctx, &f, in.Branch); err != nil {
			return nil, err
		}
		applyOvertime(&f, in)
		if in.Notes.Sent {
			f.set("notes", in.Notes.Val)
		}
		f.set("adjusted_at", s.clock())
		f.set("adjusted_by", optionalUser(a))
		f.set("adjusted_by_name", actorName(a))
		f.set("adjustment_count", int(current.Num("adjustment_count"))+1)
	}
	if len(f) == 0 {
		return current, nil
	}
	var row *Row
	err = s.repo.InTx(ctx, func(r Repository) error {
		var err error
		if row, err = r.UpdateAttendance(ctx, id, f); err != nil {
			return periodWriteError(err)
		}
		if !data {
			return nil
		}
		if err := s.logAdjustment(ctx, r, a, "update", id, employeeID, date, in.Reason, current, row, meta); err != nil {
			return err
		}
		return runAlso(also, r, row)
	})
	if err != nil {
		return nil, err
	}
	return row, nil
}

// runAlso runs a caller's in-transaction hook (nil: none).
func runAlso(also func(Repository, *Row) error, r Repository, row *Row) error {
	if also == nil {
		return nil
	}
	return also(r, row)
}

// DeleteAttendanceWithReason removes a row (logged with its last state).
func (s *Service) DeleteAttendanceWithReason(ctx context.Context, a *Actor, id, reason string, meta audit.Entry) error {
	if err := requireReason(reason); err != nil {
		return err
	}
	current, err := s.repo.AttendanceRecord(ctx, id)
	if err != nil || current == nil {
		return errOr(err, httpx.NotFound("Attendance not found"))
	}
	date := attendanceDate(current)
	if err := s.ensureAttendanceOpen(ctx, date); err != nil {
		return err
	}
	return s.repo.InTx(ctx, func(r Repository) error {
		if err := r.DeleteAttendance(ctx, id); err != nil {
			return periodWriteError(err)
		}
		return s.logAdjustment(ctx, r, a, "delete", id, current.Str("employee_id"), date, reason, current, nil, meta)
	})
}

// attendanceDate is the row's date as "YYYY-MM-DD" (WIB calendar date).
func attendanceDate(r *Row) string {
	if t, ok := r.Time("date"); ok {
		return t.Format(domain.DateLayout)
	}
	d := r.Str("date")
	if len(d) >= 10 {
		return d[:10]
	}
	return d
}

// AttendanceAdjustments is the adjustment history of a row (newest first).
func (s *Service) AttendanceAdjustments(ctx context.Context, attendanceID string) ([]*Row, error) {
	return queryRows(ctx, s.repo.querier(), `SELECT id, attendance_id, employee_id, date, action, reason, changed_fields,
		before, after, actor_name, created_at
		FROM hris.attendance_adjustments WHERE attendance_id = $1 ORDER BY created_at DESC`, attendanceID)
}

/* ── HTTP ────────────────────────────────────────────────────────────── */

func auditMeta(r *http.Request) audit.Entry { return audit.Entry{}.WithRequest(r) }

// POST /api/hris/attendance/manual
func (h handlers) createManualAttendance(w http.ResponseWriter, r *http.Request, a *Actor) error {
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	in := parseAdjustment(f, true)
	if err := formErr(f, ""); err != nil {
		return err
	}
	row, err := h.svc.CreateManualAttendance(r.Context(), a, in, auditMeta(r))
	if err != nil {
		return err
	}
	return reply(w, http.StatusCreated, obj("message", "Absensi manual ditambahkan", "data", row))
}

// GET /api/hris/attendance-adjustments?attendance_id=
func (h handlers) listAttendanceAdjustments(w http.ResponseWriter, r *http.Request, a *Actor) error {
	if !a.IsHR {
		return httpx.Forbidden("Forbidden")
	}
	id, err := requireUUID(r.URL.Query().Get("attendance_id"), "ID absensi tidak valid")
	if err != nil {
		return err
	}
	rows, err := h.svc.AttendanceAdjustments(r.Context(), id)
	if err != nil {
		return err
	}
	return ok(w, dataOf(rows))
}

// deleteReason reads the reason of a DELETE from ?reason= or a JSON body.
func deleteReason(r *http.Request) string {
	if v := strings.TrimSpace(r.URL.Query().Get("reason")); v != "" {
		return v
	}
	f := optionalForm(r)
	if v := f.Str("reason", nullish, validate.StrOpts{}); v != nil {
		return domain.JSTrim(*v)
	}
	return ""
}

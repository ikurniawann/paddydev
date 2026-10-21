package hris

import (
	"context"
	"errors"
	"net/http"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/validate"
)

// Koreksi lembur oleh AdminHRIS (MOM AP-04, EPIC-063 R6).

// overtimeManagers is the leaf whose "update" action is AdminHRIS's
// correction right (EPIC-061 leaf/action convention).
var overtimeManagers = []string{"hris.kepegawaian.overtime"}

const (
	overtimeCorrectStatus = "Hanya lembur yang menunggu atau disetujui yang bisa dikoreksi"
	overtimeCorrectSelf   = "Tidak bisa mengoreksi lembur sendiri"
	overtimeBadDate       = "Tanggal tidak valid"
)

// overtimeCorrection is the PATCH body; nil fields keep their value.
type overtimeCorrection struct {
	Date, StartTime, EndTime, DayType *string
	Reason                            string
}

// CorrectOvertime fixes the date, times or type of someone else's pending or
// approved request. The status stays; hours are recomputed; a date inside a
// closed attendance period (old or new) is refused; before/after and the
// reason are audited in the same transaction.
func (s *Service) CorrectOvertime(ctx context.Context, a *Actor, id string, in overtimeCorrection, meta audit.Entry) (*Row, error) {
	if in.Date != nil {
		if _, ok := domain.ParseDate(*in.Date); !ok { // 2026-02-30 matches the pattern but is no date (P8)
			return nil, httpx.BadRequest(overtimeBadDate)
		}
	}
	ot, err := s.repo.OvertimeForDecision(ctx, id)
	if err != nil {
		return nil, err
	}
	if ot == nil {
		return nil, httpx.NotFound("Pengajuan lembur tidak ditemukan")
	}
	status := ot.Str("status")
	if status != "pending" && status != "approved" {
		return nil, httpx.BadRequest(overtimeCorrectStatus)
	}
	if a.EmployeeID != nil && *a.EmployeeID == ot.Str("employee_id") {
		return nil, httpx.Forbidden(overtimeCorrectSelf)
	}
	was := struct{ date, start, end, dayType string }{dateText(ot, "date"), overtimeClock(ot.Str("start_time")),
		overtimeClock(ot.Str("end_time")), ot.Str("day_type")}
	pick := func(sent *string, cur string) string {
		if sent != nil {
			return *sent
		}
		return cur
	}
	date, start, end, dayType := pick(in.Date, was.date), pick(in.StartTime, was.start), pick(in.EndTime, was.end),
		pick(in.DayType, was.dayType)
	if date == was.date && start == was.start && end == was.end && dayType == was.dayType {
		return nil, httpx.BadRequest("Tidak ada perubahan")
	}
	if start == end {
		return nil, httpx.BadRequest("Jam mulai dan selesai tidak boleh sama")
	}
	hours := domain.OvertimeHoursFromTimes(start, end)
	if hours > 12 {
		return nil, httpx.BadRequest("Durasi lembur maksimal 12 jam")
	}
	if hours < 1 {
		return nil, httpx.BadRequest("Durasi lembur minimal 1 jam")
	}
	if date != was.date {
		taken, err := s.repo.OvertimeActiveOn(ctx, ot.Str("employee_id"), date)
		if err != nil {
			return nil, err
		}
		if taken {
			return nil, httpx.BadRequest(overtimeDuplicate)
		}
	}
	before := map[string]any{"date": was.date, "start_time": was.start, "end_time": was.end,
		"day_type": was.dayType, "hours": ot.Num("hours")}
	after := map[string]any{"date": date, "start_time": start, "end_time": end, "day_type": dayType, "hours": hours}
	label := was.date
	if emp := ot.Child("employee"); emp != nil {
		label = emp.Str("full_name") + " · " + label
	}
	now := s.clock()
	var row *Row
	err = s.repo.InTx(ctx, func(r Repository) error {
		for _, d := range []string{was.date, date} {
			p, err := r.ClosedPeriodOn(ctx, d)
			if err != nil {
				return err
			}
			if p != nil {
				return httpx.Conflict("Periode absensi " + p.Str("label") + " sudah di-closing — lembur tidak bisa dikoreksi")
			}
		}
		var f fields
		f.add("date", date)
		f.add("start_time", start)
		f.add("end_time", end)
		f.add("hours", hours)
		f.add("day_type", dayType)
		f.add("corrected_at", now)
		f.add("corrected_by_name", actorName(a))
		f.add("updated_at", now)
		var err error
		if row, err = r.CorrectOvertime(ctx, id, status, f); err != nil {
			return err
		}
		reason := in.Reason
		meta.ActorID, meta.ActorName = a.UserID, ptr(actorName(a))
		meta.Action, meta.Entity, meta.EntityID, meta.EntityLabel = "overtime.correct", "hris.overtime_requests", &id, &label
		meta.Before, meta.After, meta.Reason = before, after, &reason
		return audit.Write(ctx, r.querier(), meta)
	})
	if database.IsUniqueViolation(err) { // the active (employee_id, date) index lost a race
		return nil, httpx.BadRequest(overtimeDuplicate)
	}
	if errors.Is(err, errNoRow) { // the status changed since it was read
		return nil, httpx.Conflict(decisionRace)
	}
	if err != nil {
		return nil, err
	}
	return obj("data", row, "message", "Lembur dikoreksi"), nil
}

func (s *store) CorrectOvertime(ctx context.Context, id, fromStatus string, f fields) (*Row, error) {
	return updateOne(ctx, s.db, "hris.overtime_requests", f, fields{{"id", id}, {"status", fromStatus}}, "*")
}

func (s *store) ClosedPeriodOn(ctx context.Context, date string) (*Row, error) {
	return queryRow(ctx, s.db, `SELECT to_char(start_date, 'DD-MM-YYYY') || ' s.d. ' || to_char(end_date, 'DD-MM-YYYY') AS label
		FROM hris.attendance_periods
		WHERE status = 'closed' AND $1::date BETWEEN start_date AND end_date
		ORDER BY start_date LIMIT 1`, date)
}

// PATCH /api/hris/overtime/{id} { date?, start_time?, end_time?, day_type?, reason }
func (h handlers) correctOvertime(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	id, err := requireUUID(r.PathValue("id"), "ID lembur tidak valid")
	if err != nil {
		return err
	}
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	in := overtimeCorrection{
		Date:      f.Str("date", optional, validate.StrOpts{Check: regexOf(domain.DateRe)}),
		StartTime: f.Str("start_time", optional, validate.StrOpts{Check: regexOf(overtimeTimeRe)}),
		EndTime:   f.Str("end_time", optional, validate.StrOpts{Check: regexOf(overtimeTimeRe)}),
		DayType:   f.Enum("day_type", optional, overtimeDayTypes),
		Reason: strOr(f.Str("reason", validate.Rule{}, validate.StrOpts{Trim: true, Max: 500,
			Check: minLen(5, "Alasan koreksi minimal 5 karakter")})),
	}
	if err := formErr(f, ""); err != nil {
		return err
	}
	a, err := h.svc.Actor(r.Context(), u)
	if err != nil {
		return err
	}
	out, err := h.svc.CorrectOvertime(r.Context(), a, id, in, auditMeta(r))
	if err != nil {
		return err
	}
	return ok(w, out)
}

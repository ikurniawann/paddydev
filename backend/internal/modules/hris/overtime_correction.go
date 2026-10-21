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
	var row *Row
	err := s.repo.InTx(ctx, func(r Repository) error {
		// The snapshot is read (and locked) inside the transaction so the audited
		// "before" cannot be stale under concurrent corrections.
		ot, err := r.OvertimeForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if ot == nil {
			return httpx.NotFound("Pengajuan lembur tidak ditemukan")
		}
		status := ot.Str("status")
		if status != "pending" && status != "approved" {
			return httpx.BadRequest(overtimeCorrectStatus)
		}
		if a.EmployeeID != nil && *a.EmployeeID == ot.Str("employee_id") {
			return httpx.Forbidden(overtimeCorrectSelf)
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
			return httpx.BadRequest("Tidak ada perubahan")
		}
		if start == end {
			return httpx.BadRequest("Jam mulai dan selesai tidak boleh sama")
		}
		hours := domain.OvertimeHoursFromTimes(start, end)
		if hours > 12 {
			return httpx.BadRequest("Durasi lembur maksimal 12 jam")
		}
		if hours < 1 {
			return httpx.BadRequest("Durasi lembur minimal 1 jam")
		}
		if date != was.date {
			taken, err := r.OvertimeActiveOn(ctx, ot.Str("employee_id"), date)
			if err != nil {
				return err
			}
			if taken {
				return httpx.BadRequest(overtimeDuplicate)
			}
		}
		// Same lock as attendance: a period past its closing date is closed lazily.
		today := domain.TodayWIB(s.now())
		for _, d := range []string{was.date, date} {
			p, err := periodContaining(ctx, r.querier(), d, today)
			if err != nil {
				return err
			}
			if p.Closed() {
				return httpx.Conflict("Periode absensi " + dmyDate(p.Start) + " s.d. " + dmyDate(p.End) +
					" sudah di-closing — lembur tidak bisa dikoreksi")
			}
		}
		label := was.date
		if emp := ot.Child("employee"); emp != nil {
			label = emp.Str("full_name") + " · " + label
		}
		now := s.clock()
		var f fields
		f.add("date", date)
		f.add("start_time", start)
		f.add("end_time", end)
		f.add("hours", hours)
		f.add("day_type", dayType)
		f.add("corrected_at", now)
		f.add("corrected_by_name", actorName(a))
		f.add("updated_at", now)
		if row, err = r.CorrectOvertime(ctx, id, status, f); err != nil {
			return err
		}
		reason := in.Reason
		meta.ActorID, meta.ActorName = a.UserID, ptr(actorName(a))
		meta.Action, meta.Entity, meta.EntityID, meta.EntityLabel = "overtime.correct", "hris.overtime_requests", &id, &label
		meta.Before = map[string]any{"date": was.date, "start_time": was.start, "end_time": was.end,
			"day_type": was.dayType, "hours": ot.Num("hours")}
		meta.After = map[string]any{"date": date, "start_time": start, "end_time": end, "day_type": dayType, "hours": hours}
		meta.Reason = &reason
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

// dmyDate is YYYY-MM-DD as DD-MM-YYYY.
func dmyDate(iso string) string {
	if t, ok := domain.ParseDate(iso); ok {
		return t.Format("02-01-2006")
	}
	return iso
}

func (s *store) CorrectOvertime(ctx context.Context, id, fromStatus string, f fields) (*Row, error) {
	return updateOne(ctx, s.db, "hris.overtime_requests", f, fields{{"id", id}, {"status", fromStatus}}, "*")
}

// OvertimeForUpdate is OvertimeForDecision with the row locked (FOR UPDATE
// of the request only, not the embedded employee).
func (s *store) OvertimeForUpdate(ctx context.Context, id string) (*Row, error) {
	row, err := queryRow(ctx, s.db, `SELECT *, `+
		embedOne("employee", "id, full_name, reporting_to", "hris.employees", "id = overtime_requests.employee_id")+
		` FROM hris.overtime_requests WHERE id = $1 FOR UPDATE`, id)
	return row, ignoreBadInput(err)
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

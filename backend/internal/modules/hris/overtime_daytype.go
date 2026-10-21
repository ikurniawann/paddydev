package hris

import (
	"context"
	"net/http"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/validate"
)

// Jenis lembur Weekday / Weekend-Libur (MOM AP-03, K-13, EPIC-063 R5).

// suggestOvertimeDayType is the type the employee's schedule and the active
// public holidays imply on date; known is false without a schedule in force.
func (s *Service) suggestOvertimeDayType(ctx context.Context, employeeID, date string) (string, bool, error) {
	holidays, err := s.repo.ActiveHolidays(ctx, date, date)
	if err != nil {
		return "", false, err
	}
	rows, err := s.repo.ShiftPattern(ctx, employeeID)
	if err != nil {
		return "", false, err
	}
	pattern := make([]domain.ScheduleRow, len(rows))
	for i, r := range rows {
		pattern[i] = domain.ScheduleRow{DayOfWeek: int(r.Num("day_of_week")), ShiftID: r.StrPtr("shift_id"),
			EffectiveFrom: r.Str("effective_from"), EffectiveTo: r.StrPtr("effective_to")}
	}
	dayType, known := domain.OvertimeDayTypeFor(date, len(holidays) > 0, pattern)
	return dayType, known, nil
}

// overtimeDayType is the day type stored on a new request. free (an HR
// assignment for someone else) keeps any sent type. One's own request may
// not contradict a known suggestion; without a type it takes the suggestion,
// or Weekday when nothing is known.
func (s *Service) overtimeDayType(ctx context.Context, employeeID, date string, sent *string, free bool) (string, error) {
	if free && sent != nil {
		return *sent, nil
	}
	suggested, known, err := s.suggestOvertimeDayType(ctx, employeeID, date)
	if err != nil {
		return "", err
	}
	switch {
	case sent == nil && known:
		return suggested, nil
	case sent == nil:
		return domain.OvertimeWeekday, nil
	case !known || *sent == suggested:
		return *sent, nil
	}
	return "", httpx.BadRequest("Jenis lembur tanggal ini " + domain.OvertimeDayTypeLabel(suggested) + " sesuai jadwal Anda")
}

// OvertimeDayTypeHint is GET /api/hris/overtime/day-type: the suggestion for
// the ESS form; day_type and label are null when no schedule is in force.
func (s *Service) OvertimeDayTypeHint(ctx context.Context, employeeID, date string) (*Row, error) {
	dayType, known, err := s.suggestOvertimeDayType(ctx, employeeID, date)
	if err != nil {
		return nil, err
	}
	if !known {
		return obj("date", date, "day_type", nil, "label", nil), nil
	}
	return obj("date", date, "day_type", dayType, "label", domain.OvertimeDayTypeLabel(dayType)), nil
}

// GET /api/hris/overtime/day-type?date=YYYY-MM-DD&employee_id= (HR only for
// another employee).
func (h handlers) overtimeDayTypeHint(w http.ResponseWriter, r *http.Request, a *Actor) error {
	q := r.URL.Query()
	date := q.Get("date")
	if _, ok := domain.ParseDate(date); !ok || !domain.DateRe.MatchString(date) {
		return httpx.BadRequest("Tanggal tidak valid")
	}
	target := a.EmployeeID
	if id := q.Get("employee_id"); id != "" && id != "me" {
		canonical, ok := validate.CanonicalUUID(id)
		if !ok {
			return httpx.BadRequest("ID karyawan tidak valid")
		}
		if !a.IsHR && (a.EmployeeID == nil || *a.EmployeeID != canonical) {
			return httpx.Forbidden("Insufficient permissions")
		}
		target = &canonical
	}
	if target == nil {
		return httpx.BadRequest("Akun ini tidak tertaut ke data karyawan")
	}
	out, err := h.svc.OvertimeDayTypeHint(r.Context(), *target, date)
	if err != nil {
		return err
	}
	return ok(w, dataOf(out))
}

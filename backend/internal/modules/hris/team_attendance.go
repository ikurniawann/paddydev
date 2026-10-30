package hris

import (
	"context"
	"net/http"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
)

/* ── Absensi Tim (EPIC-074) ──────────────────────────────────────────── */

// teamAttendee is one employee of a manager's team: their direct manager's
// name and whether they report to the manager directly.
type teamAttendee struct {
	ID, ReportingToName string
	Direct              bool
}

// teamAttendees lists the manager's team (R1): every active employee below
// them in the reporting_to chain to 10 levels (delegateSubtree), the
// manager excluded; never nil. An inactive manager has no team, even when
// the page is opened by URL.
func teamAttendees(ctx context.Context, q database.Querier, managerID string) ([]teamAttendee, error) {
	rows, err := q.Query(ctx, delegateSubtree+` SELECT DISTINCT e.id::text, coalesce(m.full_name, ''), e.reporting_to = $1
		FROM sub JOIN hris.employees e ON e.id = sub.id
		LEFT JOIN hris.employees m ON m.id = e.reporting_to
		WHERE EXISTS (SELECT 1 FROM hris.employees me WHERE me.id = $1 AND me.is_active)`, managerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []teamAttendee{}
	for rows.Next() {
		var m teamAttendee
		if err := rows.Scan(&m.ID, &m.ReportingToName, &m.Direct); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// TeamAttendance is the HR daily roster over the manager's team (R2):
// photos left out, each member with reporting_to_name and direct. No
// manager (an account without an employee) or no team gives no employees.
func (s *Service) TeamAttendance(ctx context.Context, managerID *string, date, today string) (*Row, error) {
	members := []teamAttendee{}
	if managerID != nil {
		var err error
		if members, err = teamAttendees(ctx, s.repo.querier(), *managerID); err != nil {
			return nil, err
		}
	}
	if len(members) == 0 {
		// Most accounts have no team: answer without the company-wide roster
		// query (Beranda asks on every visit).
		return emptyTeamRoster(date, today), nil
	}
	// Must stay non-nil: RosterRows reads nil as "every employee".
	ids := make([]string, len(members))
	byID := make(map[string]teamAttendee, len(members))
	for i, m := range members {
		ids[i] = m.ID
		byID[m.ID] = m
	}
	out, err := s.rosterFor(ctx, date, today, ids)
	if err != nil {
		return nil, err
	}
	employees, _ := out.Get("employees").([]*Row)
	for _, e := range employees {
		m := byID[e.Str("employee_id")]
		e.Set("reporting_to_name", m.ReportingToName)
		e.Set("direct", m.Direct)
		if att, ok := e.Get("attendance").(*Row); ok && att != nil {
			att.Set("clock_in_photo_url", nil)
			att.Set("clock_out_photo_url", nil)
			// Free-text notes only where the page shows them: a check outside the area.
			if att.Str("clock_in_geofence") != "invalid" && att.Str("clock_out_geofence") != "invalid" {
				att.Set("notes", nil)
			}
		}
	}
	return out, nil
}

// emptyTeamRoster is the TeamAttendance envelope of an account without a team.
func emptyTeamRoster(date, today string) *Row {
	summary := newRow()
	for _, k := range rosterOrder {
		summary.Set(k, 0)
	}
	return obj("date", date, "day_of_week", domain.IsoDayOfWeek(date), "is_today", date == today, "summary", summary,
		"shifts", []*Row{}, "holidays", []*Row{}, "employees", []*Row{})
}

// GET /api/hris/me/team-attendance?date=YYYY-MM-DD (default today WIB):
// the signed-in manager's team roster (EPIC-074), read only.
func (h handlers) teamAttendance(w http.ResponseWriter, r *http.Request, a *Actor) error {
	requested := r.URL.Query().Get("date")
	if requested != "" && !domain.DateRe.MatchString(requested) {
		return httpx.BadRequest("Format tanggal tidak valid")
	}
	today := domain.TodayWIB(h.svc.now())
	date := requested
	if date == "" {
		date = today
	}
	out, err := h.svc.TeamAttendance(r.Context(), a.EmployeeID, date, today)
	if err != nil {
		return err
	}
	return ok(w, dataOf(out))
}

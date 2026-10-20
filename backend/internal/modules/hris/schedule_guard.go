package hris

import (
	"context"
	"net/http"
	"strings"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/hrisdel"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
)

// errScheduleNeedsDelete: a schedule change would remove per-date rows and
// the caller lacks "delete" on Jadwal Kerja (EPIC-061, Q-RA-1/3).
const errScheduleNeedsDelete = "Menimpa atau menghapus jadwal yang sudah ada butuh hak hapus Jadwal Kerja"

// clearGuard gates schedule changes that remove per-date rows: "delete" on
// Jadwal Kerja plus a reason, and one audit row. Nothing removed, nothing
// asked (ruling R4).
type clearGuard struct {
	Allowed bool
	Reason  string
	Meta    audit.Entry
}

func (g clearGuard) check(n int) error {
	if n == 0 {
		return nil
	}
	if !g.Allowed {
		return httpx.Forbidden(errScheduleNeedsDelete)
	}
	return hrisdel.CheckReason(g.Reason)
}

// audit writes the audit row when n per-date rows were removed.
func (g clearGuard) audit(ctx context.Context, q database.Querier, actor *auth.User, action, entityID string, before map[string]any, n int) error {
	if n == 0 {
		return nil
	}
	reason := strings.TrimSpace(g.Reason)
	e := g.Meta
	e.ActorID, e.ActorName = actor.ID, &actor.FullName
	e.Action, e.Entity, e.EntityID = action, "hris.employee_shift_days", &entityID
	e.Before, e.Reason = before, &reason
	return audit.Write(ctx, q, e)
}

// clearGuardFor reads "delete" on Jadwal Kerja for the caller.
func (h handlers) clearGuardFor(r *http.Request, u *auth.User, reason string) (clearGuard, error) {
	allowed, err := h.auth.HasMenuAction(r.Context(), u, "delete", iam.LeafJadwalKerja)
	if err != nil {
		return clearGuard{}, err
	}
	return clearGuard{Allowed: allowed, Reason: reason, Meta: auditMeta(r)}, nil
}

// replacedScheduleDays counts the per-date rows an upload would overwrite
// with a different shift (a libur cell has no shift id).
func replacedScheduleDays(ctx context.Context, q database.Querier, as []domain.ScheduleAssignment) (int, error) {
	ids, dates := assignmentKeys(as)
	shifts := make([]string, len(as))
	for i, a := range as {
		if a.ShiftID != nil {
			shifts[i] = *a.ShiftID
		}
	}
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM hris.employee_shift_days d
		JOIN unnest($1::uuid[], $2::date[], $3::text[]) AS k(employee_id, work_date, shift_id)
		  ON k.employee_id = d.employee_id AND k.work_date = d.work_date
		WHERE d.shift_id IS DISTINCT FROM NULLIF(k.shift_id, '')::uuid`, ids, dates, shifts).Scan(&n)
	return n, err
}

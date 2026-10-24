package hris

import (
	"context"
	"net/http"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/validate"
)

// Jam kerja minimum per cabang (EPIC-065 R7–R8, MOM AB-05): the label
// itself is written by the hris.attendance_short_hours trigger.

// WorkHoursRepo writes the store minimum and relabels attendance.
type WorkHoursRepo interface {
	// SaveBranchMinWorkHours writes the store minimum (nil removes the rule),
	// returning id, code, name and min_work_hours.
	SaveBranchMinWorkHours(ctx context.Context, id string, hours *float64) (*Row, error)
	// RelabelShortHours recomputes the label of the store's rows dated from
	// "from" on (the open attendance window), returning the rows changed.
	RelabelShortHours(ctx context.Context, branchID, from string) (int64, error)
}

// SaveBranchMinWorkHours is PUT /api/hris/branches/{id}/min-work-hours: the
// store's minimum presence; nil removes the rule. The store's rows in the
// open attendance window are relabelled; closed periods keep their label.
func (s *Service) SaveBranchMinWorkHours(ctx context.Context, u *auth.User, id string, hours *float64, meta audit.Entry) (*Row, error) {
	if msg := domain.ValidateMinWorkHours(hours); msg != "" {
		return nil, httpx.BadRequest(msg)
	}
	from, _, err := s.openAttendanceWindow(ctx)
	if err != nil {
		return nil, err
	}
	var row *Row
	err = s.repo.InTx(ctx, func(r Repository) error {
		before, err := r.BranchGeofence(ctx, id)
		if err != nil {
			return err
		}
		if before == nil {
			return httpx.NotFound("Toko tidak ditemukan")
		}
		if row, err = r.SaveBranchMinWorkHours(ctx, id, hours); err != nil {
			return err
		}
		n, err := r.RelabelShortHours(ctx, id, from)
		if err != nil {
			return err
		}
		row.Set("relabelled", n)
		meta.ActorID, meta.ActorName = u.ID, ptr(u.FullName)
		meta.Action, meta.Entity, meta.EntityID = "branch_min_hours.update", "configuration.branches", ptr(id)
		meta.EntityLabel = ptr(before.Str("name"))
		meta.Before = map[string]any{"min_work_hours": before.Get("min_work_hours")}
		meta.After = map[string]any{"min_work_hours": row.Get("min_work_hours")}
		return audit.Write(ctx, r.querier(), meta)
	})
	return row, err
}

func (s *store) SaveBranchMinWorkHours(ctx context.Context, id string, hours *float64) (*Row, error) {
	return queryRow(ctx, s.db, `UPDATE configuration.branches SET min_work_hours = $2, updated_at = now()
		WHERE id = $1 RETURNING id, code, name, min_work_hours`, id, hours)
}

// RelabelShortHours sets the label explicitly; the trigger keeps explicit
// values when clock and store do not change.
func (s *store) RelabelShortHours(ctx context.Context, branchID, from string) (int64, error) {
	tag, err := s.db.Exec(ctx, `UPDATE hris.attendance a SET
		  min_work_hours = b.min_work_hours,
		  is_short_hours = b.min_work_hours IS NOT NULL
		    AND a.clock_out - a.clock_in < make_interval(secs => (b.min_work_hours * 3600)::double precision)
		FROM configuration.branches b
		WHERE b.id = $1 AND a.branch_id = b.id AND a.date >= $2::date
		  AND a.clock_in IS NOT NULL AND a.clock_out IS NOT NULL
		  AND a.min_work_hours IS DISTINCT FROM b.min_work_hours`, branchID, from)
	if err != nil {
		return 0, periodWriteError(err)
	}
	return tag.RowsAffected(), nil
}

// PUT /api/hris/branches/{id}/min-work-hours { min_work_hours: number | null }
func (h handlers) putBranchMinWorkHours(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	id, valid := validate.CanonicalUUID(r.PathValue("id"))
	if !valid {
		return httpx.BadRequest("ID toko tidak valid")
	}
	const message = "Validation failed"
	f, err := readForm(r, message)
	if err != nil {
		return err
	}
	hours := f.Num("min_work_hours", nullish, validate.NumOpts{})
	if err := formErr(f, message); err != nil {
		return err
	}
	row, err := h.svc.SaveBranchMinWorkHours(r.Context(), u, id, hours, auditMeta(r))
	if err != nil {
		return err
	}
	return ok(w, obj("data", row, "message", "Jam kerja minimum disimpan"))
}

package hris

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/validate"
)

// EPIC-068 follow-up (R5): the HR review of a carry-over that was recalculated
// below what the employee already used. The rows are opened and closed by the
// recalculation (leave_carry_recalc.go); this file is the API: list them and
// let HR resolve one with a note.

// maxReviewNote is the longest resolution note.
const maxReviewNote = 500

// CarryReview is one row of hris.leave_carry_over_reviews with the employee.
type CarryReview struct {
	ID             string          `json:"id"`
	EmployeeID     string          `json:"employee_id"`
	NIP            *string         `json:"nip"`
	FullName       string          `json:"full_name"`
	BranchName     *string         `json:"branch_name"`
	Year           int             `json:"year"`
	CarryOverDays  float64         `json:"carry_over_days"`
	CarryOverUsed  float64         `json:"carry_over_used"`
	Excess         float64         `json:"excess"`
	Cause          json.RawMessage `json:"cause"`
	OpenedAt       time.Time       `json:"opened_at"`
	ResolvedAt     *time.Time      `json:"resolved_at"`
	ResolvedByName *string         `json:"resolved_by_name"`
	Resolution     *string         `json:"resolution"`
}

const carryReviewSelect = `SELECT r.id::text, r.employee_id::text, e.nip, e.full_name, pb.branch_name, r.year,
		r.carry_over_days::float8, r.carry_over_used::float8, r.excess::float8, COALESCE(r.cause, 'null'::jsonb)::text,
		r.opened_at, r.resolved_at, r.resolved_by_name, r.resolution
	FROM hris.leave_carry_over_reviews r
	JOIN hris.employees e ON e.id = r.employee_id
	LEFT JOIN LATERAL (SELECT br.name AS branch_name
		FROM hris.employee_placements ep JOIN configuration.branches br ON br.id = ep.branch_id
		WHERE ep.employee_id = e.id AND ep.start_date <= $2::date AND (ep.end_date IS NULL OR ep.end_date >= $2::date)
		ORDER BY ep.is_primary DESC, ep.start_date DESC, br.name LIMIT 1) pb ON true`

func scanCarryReview(row interface{ Scan(...any) error }) (CarryReview, error) {
	var v CarryReview
	var cause string
	if err := row.Scan(&v.ID, &v.EmployeeID, &v.NIP, &v.FullName, &v.BranchName, &v.Year, &v.CarryOverDays,
		&v.CarryOverUsed, &v.Excess, &cause, &v.OpenedAt, &v.ResolvedAt, &v.ResolvedByName, &v.Resolution); err != nil {
		return v, err
	}
	v.Cause = json.RawMessage(cause)
	return v, nil
}

// CarryReviews lists the reviews of year, the open ones unless includeResolved.
// Without the review table (carry-over still locked) the list is empty.
func (s *Service) CarryReviews(ctx context.Context, year int, includeResolved bool) ([]CarryReview, error) {
	q := s.repo.querier()
	var ready bool
	if err := q.QueryRow(ctx, `SELECT to_regclass('hris.leave_carry_over_reviews') IS NOT NULL`).Scan(&ready); err != nil {
		return nil, err
	}
	out := []CarryReview{}
	if !ready {
		return out, nil
	}
	rows, err := q.Query(ctx, carryReviewSelect+`
		WHERE r.year = $1 AND ($3::boolean OR r.resolved_at IS NULL)
		ORDER BY r.resolved_at IS NOT NULL, r.opened_at, e.full_name, r.id`,
		year, domain.TodayWIB(s.now()), includeResolved)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		v, err := scanCarryReview(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ResolveCarryReview closes an open review on HR's word: the note is the
// resolution and is required (a blank resolution would read as unresolved to
// openCarryReview). The balance is not touched. Resolving a closed review is a
// 409. Audit: leave_balance.review_resolve on the employee, note as reason.
func (s *Service) ResolveCarryReview(ctx context.Context, a *Actor, id, note string, meta ...audit.Entry) (CarryReview, error) {
	if !a.LeaveBalanceAction("update") {
		return CarryReview{}, httpx.Forbidden("Insufficient permissions")
	}
	note = strings.TrimSpace(note)
	switch {
	case note == "":
		return CarryReview{}, httpx.BadRequest("Catatan wajib diisi")
	case strings.EqualFold(note, reviewResolutionAuto):
		// The word marks a review the system closed; HR's note must not look like one.
		return CarryReview{}, httpx.BadRequest(`Catatan tidak boleh "otomatis" — tulis alasan penyelesaian`)
	case len([]rune(note)) > maxReviewNote:
		return CarryReview{}, httpx.BadRequest("Catatan terlalu panjang (maksimal 500 karakter)")
	}
	var entry audit.Entry
	if len(meta) > 0 {
		entry = meta[0]
	}
	var out CarryReview
	err := s.repo.InTx(ctx, func(r Repository) error {
		q := r.querier()
		var ready bool
		if err := q.QueryRow(ctx, `SELECT to_regclass('hris.leave_carry_over_reviews') IS NOT NULL`).Scan(&ready); err != nil {
			return err
		}
		if !ready {
			return httpx.NotFound("Tinjauan tidak ditemukan")
		}
		var employeeID, name string
		var year int
		var days, used, excess float64
		var resolvedAt *time.Time
		err := q.QueryRow(ctx, `SELECT r.employee_id::text, e.full_name, r.year, r.carry_over_days::float8, r.carry_over_used::float8,
				r.excess::float8, r.resolved_at
			FROM hris.leave_carry_over_reviews r JOIN hris.employees e ON e.id = r.employee_id
			WHERE r.id = $1 FOR UPDATE OF r`, id).Scan(&employeeID, &name, &year, &days, &used, &excess, &resolvedAt)
		if database.IsNoRows(err) {
			return httpx.NotFound("Tinjauan tidak ditemukan")
		}
		if err != nil {
			return err
		}
		if resolvedAt != nil {
			return httpx.Conflict("Tinjauan sudah diselesaikan")
		}
		by, who := carryActor(userOf(a))
		var byID *string
		if by != "" {
			byID = &by
		}
		if _, err := q.Exec(ctx, `UPDATE hris.leave_carry_over_reviews
			SET resolved_at = now(), resolved_by = $2::uuid, resolved_by_name = $3, resolution = $4 WHERE id = $1`,
			id, byID, who, note); err != nil {
			return err
		}
		snap := func(resolved bool) map[string]any {
			m := map[string]any{"year": year, "carry_over_days": days, "carry_over_used": used, "excess": excess, "resolved": resolved}
			if resolved {
				m["resolution"] = note
			}
			return m
		}
		label := fmt.Sprintf("%s · Tinjauan sisa bawaan %d", name, year)
		entry.ActorID, entry.ActorName = a.UserID, ptr(actorName(a))
		entry.Action, entry.Entity, entry.EntityID, entry.EntityLabel = "leave_balance.review_resolve", "hris.employees", &employeeID, &label
		entry.Before, entry.After, entry.Reason = snap(false), snap(true), &note
		if err := audit.Write(ctx, q, entry); err != nil {
			return err
		}
		out, err = scanCarryReview(q.QueryRow(ctx, carryReviewSelect+` WHERE r.id = $1`, id, domain.TodayWIB(s.now())))
		return err
	})
	return out, err
}

/* ── HTTP ────────────────────────────────────────────────────────────── */

// GET /api/hris/leave-balances/reviews?year=&status=open|all
func (h handlers) listCarryReviews(w http.ResponseWriter, r *http.Request, _ *Actor) error {
	year, err := h.balanceYear(r)
	if err != nil {
		return err
	}
	f := queryForm(r)
	status := strOr(f.Str("status", optional, enumOf([]string{"open", "all"}, "Status harus open atau all")))
	if err := formErr(f, ""); err != nil {
		return err
	}
	rows, err := h.svc.CarryReviews(r.Context(), year, status == "all")
	if err != nil {
		return err
	}
	return ok(w, dataOf(rows))
}

// POST /api/hris/leave-balances/reviews/{id}/resolve {note}
func (h handlers) resolveCarryReview(w http.ResponseWriter, r *http.Request, a *Actor) error {
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	note := strOr(f.Str("note", optional, validate.StrOpts{}))
	if err := formErr(f, ""); err != nil {
		return err
	}
	id, err := requireUUID(r.PathValue("id"), "ID tinjauan tidak valid")
	if err != nil {
		return err
	}
	out, err := h.svc.ResolveCarryReview(r.Context(), a, id, note, auditMeta(r))
	if err != nil {
		return err
	}
	return ok(w, dataMsg(out, "Tinjauan ditandai selesai"))
}

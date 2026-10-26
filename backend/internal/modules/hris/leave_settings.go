package hris

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/validate"
)

// Pengaturan sisa cuti bawaan (EPIC-068 R8): satu baris hris.leave_settings,
// pola hris.attendance_settings.

// LeaveSettings is hris.leave_settings. A nil CarryOverMaxDays carries the
// whole remainder.
type LeaveSettings struct {
	CarryOverEnabled     bool       `json:"carry_over_enabled"`
	CarryOverMaxDays     *float64   `json:"carry_over_max_days"`
	CarryOverExpiryMonth int        `json:"carry_over_expiry_month"`
	CarryOverExpiryDay   int        `json:"carry_over_expiry_day"`
	UpdatedAt            *time.Time `json:"updated_at"`
	UpdatedByName        *string    `json:"updated_by_name"`
	// CarryOverReady: the follow-up migration (carry-over recalculation) is
	// installed; until then carry-over cannot be switched on and the job
	// carries nothing.
	CarryOverReady bool `json:"carry_over_ready"`
}

// carryOverLockedMsg refuses switching carry-over on before the follow-up
// migration creates hris.leave_carry_over_reviews (EPIC-068 tindak lanjut).
const carryOverLockedMsg = "Sisa bawaan belum bisa diaktifkan: perbaikan perhitungan ulang sisa bawaan belum terpasang"

// LeaveSettingsPatch is the PUT body: a field left out keeps its value; a
// sent null maximum clears it (carry everything).
type LeaveSettingsPatch struct {
	Enabled *bool
	MaxSent bool
	Max     *float64
	Month   *int
	Day     *int
	// Reason is mandatory on a save (it is the audit reason) and ignored by a
	// preview.
	Reason string
}

// LeaveSettingsImpact is what a settings change does to the current year's
// carry-overs: the preview returns it and the save produces exactly it.
// DaysDecrease is a positive number of days.
type LeaveSettingsImpact struct {
	EmployeesChanged int     `json:"employees_changed"`
	DaysIncrease     float64 `json:"days_increase"`
	DaysDecrease     float64 `json:"days_decrease"`
	// NeedsReview is the number of carry-overs of the year left with an open
	// review once the change is applied.
	NeedsReview int `json:"needs_review"`
}

const leaveSettingsColumns = `carry_over_enabled, carry_over_max_days::float8, carry_over_expiry_month::int,
	carry_over_expiry_day::int, updated_at, updated_by_name`

// loadLeaveSettings reads the settings row (the defaults when it is missing);
// lock holds it FOR UPDATE.
func loadLeaveSettings(ctx context.Context, q database.Querier, lock bool) (LeaveSettings, error) {
	s := LeaveSettings{CarryOverExpiryMonth: 3, CarryOverExpiryDay: 31}
	sql := `SELECT ` + leaveSettingsColumns + ` FROM hris.leave_settings WHERE id`
	if lock {
		sql += ` FOR UPDATE`
	}
	if err := q.QueryRow(ctx, `SELECT to_regclass('hris.leave_carry_over_reviews') IS NOT NULL`).Scan(&s.CarryOverReady); err != nil {
		return s, err
	}
	err := q.QueryRow(ctx, sql).Scan(&s.CarryOverEnabled, &s.CarryOverMaxDays, &s.CarryOverExpiryMonth,
		&s.CarryOverExpiryDay, &s.UpdatedAt, &s.UpdatedByName)
	if database.IsNoRows(err) {
		return s, nil
	}
	return s, err
}

// validateLeaveSettings checks the carry-over rules: a maximum that is a
// multiple of 0,5 and an expiry date that exists in every year.
func validateLeaveSettings(s LeaveSettings) error {
	if m := s.CarryOverMaxDays; m != nil {
		switch {
		case math.IsNaN(*m) || math.IsInf(*m, 0) || !isHalfStep(*m):
			return httpx.BadRequest("Maksimal sisa bawaan harus kelipatan 0,5")
		case *m < 0:
			return httpx.BadRequest("Maksimal sisa bawaan tidak boleh negatif")
		case *m > maxLeaveDays:
			return httpx.BadRequest(fmt.Sprintf("Maksimal sisa bawaan tidak boleh lebih dari %d hari", maxLeaveDays))
		}
	}
	if s.CarryOverExpiryMonth < 1 || s.CarryOverExpiryMonth > 12 {
		return httpx.BadRequest("Bulan kedaluwarsa harus 1–12")
	}
	if s.CarryOverExpiryDay < 1 || s.CarryOverExpiryDay > 31 {
		return httpx.BadRequest("Tanggal kedaluwarsa harus 1–31")
	}
	if s.CarryOverExpiryMonth == 2 && s.CarryOverExpiryDay == 29 {
		return httpx.BadRequest("Tanggal kedaluwarsa tidak valid — 29 Februari tidak diperbolehkan karena tidak ada di setiap tahun")
	}
	// 2025 is not a leap year, so this accepts exactly the dates that exist
	// in every year.
	if d := time.Date(2025, time.Month(s.CarryOverExpiryMonth), s.CarryOverExpiryDay, 0, 0, 0, 0, time.UTC); int(d.Month()) != s.CarryOverExpiryMonth {
		return httpx.BadRequest("Tanggal kedaluwarsa tidak valid")
	}
	return nil
}

// LeaveSettings is GET /api/hris/leave-settings.
func (s *Service) LeaveSettings(ctx context.Context) (LeaveSettings, error) {
	return loadLeaveSettings(ctx, s.repo.querier(), false)
}

// errPreviewDone rolls a preview transaction back.
var errPreviewDone = errors.New("leave settings preview")

// SaveLeaveSettings is PUT /api/hris/leave-settings: merge the patch into the
// stored row, validate, save and audit (leave_settings.update, with the
// reason) in one transaction. A new maximum, or switching carry-over on,
// recalculates the current year's carry-over of every active employee whose
// row is automatic (applyLeaveSettings), in that same transaction.
func (s *Service) SaveLeaveSettings(ctx context.Context, a *Actor, p LeaveSettingsPatch, meta ...audit.Entry) (LeaveSettings, error) {
	if !a.LeaveBalanceAction("update") {
		return LeaveSettings{}, httpx.Forbidden("Insufficient permissions")
	}
	reason := strings.TrimSpace(p.Reason)
	switch {
	case reason == "":
		return LeaveSettings{}, httpx.BadRequest("Alasan perubahan wajib diisi")
	case len([]rune(reason)) > 500:
		return LeaveSettings{}, httpx.BadRequest("Alasan terlalu panjang (maksimal 500 karakter)")
	}
	p.Reason = reason
	var entry audit.Entry
	if len(meta) > 0 {
		entry = meta[0]
	}
	err := s.repo.InTx(ctx, func(r Repository) error {
		q := r.querier()
		before, next, _, err := applyLeaveSettings(ctx, q, a, domain.NowWIB(s.now()).Year(), p)
		if err != nil {
			return err
		}
		entry.ActorID, entry.ActorName = a.UserID, ptr(actorName(a))
		entry.Action, entry.Entity = "leave_settings.update", "hris.leave_settings"
		entry.EntityLabel = ptr("Pengaturan sisa cuti bawaan")
		entry.Before, entry.After = settingsAudit(before), settingsAudit(next)
		entry.Reason = &reason
		return audit.Write(ctx, q, entry)
	})
	if err != nil {
		return LeaveSettings{}, err
	}
	return s.LeaveSettings(ctx)
}

// PreviewLeaveSettings is POST /api/hris/leave-settings/preview: it runs
// applyLeaveSettings, the very code of the save, in a transaction that is
// rolled back, so the figures are what the save then does. The reason is not
// needed.
func (s *Service) PreviewLeaveSettings(ctx context.Context, a *Actor, p LeaveSettingsPatch) (LeaveSettingsImpact, error) {
	if !a.LeaveBalanceAction("update") {
		return LeaveSettingsImpact{}, httpx.Forbidden("Insufficient permissions")
	}
	var impact LeaveSettingsImpact
	err := s.repo.InTx(ctx, func(r Repository) error {
		var err error
		if _, _, impact, err = applyLeaveSettings(ctx, r.querier(), a, domain.NowWIB(s.now()).Year(), p); err != nil {
			return err
		}
		return errPreviewDone
	})
	if err != nil && !errors.Is(err, errPreviewDone) {
		return LeaveSettingsImpact{}, err
	}
	return impact, nil
}

// applyLeaveSettings merges the patch into the stored row (locked), validates
// and stores it, then recalculates year's carry-over when the change calls for
// it: carry-over is on afterwards and either was just switched on or has a new
// maximum. Every active employee gets the row of year (as the 1 January job
// does) and, in ascending id order, an automatic row is recomputed with cause
// 'settings'; a manual row, an earlier year and an inactive employee are left
// alone. Switching off, or changing only the expiry date, recalculates
// nothing. It returns the stored and the new settings and the impact on year.
//
// The caller's transaction carries the result: the save commits it, the
// preview rolls it back.
func applyLeaveSettings(ctx context.Context, q database.Querier, a *Actor, year int, p LeaveSettingsPatch) (before, next LeaveSettings, impact LeaveSettingsImpact, err error) {
	before, err = loadLeaveSettings(ctx, q, true)
	if err != nil {
		return
	}
	next = before
	if p.Enabled != nil {
		next.CarryOverEnabled = *p.Enabled
	}
	if p.MaxSent {
		next.CarryOverMaxDays = p.Max
	}
	if p.Month != nil {
		next.CarryOverExpiryMonth = *p.Month
	}
	if p.Day != nil {
		next.CarryOverExpiryDay = *p.Day
	}
	if err = validateLeaveSettings(next); err != nil {
		return
	}
	if next.CarryOverEnabled && !before.CarryOverEnabled && !before.CarryOverReady {
		err = httpx.BadRequest(carryOverLockedMsg)
		return
	}
	if _, err = q.Exec(ctx, `INSERT INTO hris.leave_settings (id, carry_over_enabled, carry_over_max_days,
			carry_over_expiry_month, carry_over_expiry_day, updated_at, updated_by, updated_by_name)
		VALUES (true, $1, $2, $3, $4, now(), $5, $6)
		ON CONFLICT (id) DO UPDATE SET carry_over_enabled = EXCLUDED.carry_over_enabled,
		  carry_over_max_days = EXCLUDED.carry_over_max_days, carry_over_expiry_month = EXCLUDED.carry_over_expiry_month,
		  carry_over_expiry_day = EXCLUDED.carry_over_expiry_day, updated_at = now(),
		  updated_by = EXCLUDED.updated_by, updated_by_name = EXCLUDED.updated_by_name`,
		next.CarryOverEnabled, next.CarryOverMaxDays, next.CarryOverExpiryMonth, next.CarryOverExpiryDay,
		optionalUser(a), actorName(a)); err != nil {
		return
	}

	if next.CarryOverEnabled && next.CarryOverReady && (!before.CarryOverEnabled || !sameFloatPtr(before.CarryOverMaxDays, next.CarryOverMaxDays)) {
		cause := carryCause{Type: causeSettings}
		if p.Reason != "" {
			cause.Label = cause.reason() + " — " + p.Reason
		}
		var ids []string
		if ids, err = activeEmployeeIDs(ctx, q); err != nil {
			return
		}
		for _, id := range ids {
			if err = ensureLeaveBalance(ctx, q, id, year); err != nil {
				return
			}
			var res carryResult
			if res, err = recomputeCarryOverWith(ctx, q, next, id, year, cause, userOf(a)); err != nil {
				return
			}
			if res.Carry == res.Previous {
				continue
			}
			impact.EmployeesChanged++
			if res.Carry > res.Previous {
				impact.DaysIncrease += res.Carry - res.Previous
			} else {
				impact.DaysDecrease += res.Previous - res.Carry
			}
		}
	}
	if next.CarryOverReady { // the review table exists
		err = q.QueryRow(ctx, `SELECT count(*)::int FROM hris.leave_carry_over_reviews WHERE year = $1 AND resolved_at IS NULL`, year).
			Scan(&impact.NeedsReview)
	}
	return
}

// activeEmployeeIDs lists the active employees in a stable order, so every
// settings run locks their balances in the same sequence.
func activeEmployeeIDs(ctx context.Context, q database.Querier) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT id::text FROM hris.employees WHERE is_active ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func sameFloatPtr(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func settingsAudit(s LeaveSettings) map[string]any {
	return map[string]any{"carry_over_enabled": s.CarryOverEnabled, "carry_over_max_days": s.CarryOverMaxDays,
		"carry_over_expiry_month": s.CarryOverExpiryMonth, "carry_over_expiry_day": s.CarryOverExpiryDay}
}

// GET /api/hris/leave-settings
func (h handlers) getLeaveSettings(w http.ResponseWriter, r *http.Request, _ *Actor) error {
	out, err := h.svc.LeaveSettings(r.Context())
	if err != nil {
		return err
	}
	return ok(w, dataOf(out))
}

// settingsPatchOf reads the PUT / preview body.
func settingsPatchOf(r *http.Request) (LeaveSettingsPatch, error) {
	f, err := readForm(r, "")
	if err != nil {
		return LeaveSettingsPatch{}, err
	}
	enabled := boolean(f, "carry_over_enabled", optional)
	maxDays := number(f, "carry_over_max_days", nullish, validate.NumOpts{})
	month := f.Int("carry_over_expiry_month", optional, validate.NumOpts{})
	day := f.Int("carry_over_expiry_day", optional, validate.NumOpts{})
	reason := strOr(f.Str("reason", optional, validate.StrOpts{}))
	if err := formErr(f, ""); err != nil {
		return LeaveSettingsPatch{}, err
	}
	return LeaveSettingsPatch{Enabled: enabled.Val, MaxSent: maxDays.Sent, Max: maxDays.Val, Month: month, Day: day,
		Reason: reason}, nil
}

// PUT /api/hris/leave-settings { carry_over_enabled, carry_over_max_days,
// carry_over_expiry_month, carry_over_expiry_day, reason }
func (h handlers) putLeaveSettings(w http.ResponseWriter, r *http.Request, a *Actor) error {
	p, err := settingsPatchOf(r)
	if err != nil {
		return err
	}
	out, err := h.svc.SaveLeaveSettings(r.Context(), a, p, auditMeta(r))
	if err != nil {
		return err
	}
	return ok(w, dataMsg(out, "Pengaturan sisa cuti tersimpan"))
}

// POST /api/hris/leave-settings/preview — the PUT body (reason optional);
// answers { employees_changed, days_increase, days_decrease, needs_review }.
func (h handlers) previewLeaveSettings(w http.ResponseWriter, r *http.Request, a *Actor) error {
	p, err := settingsPatchOf(r)
	if err != nil {
		return err
	}
	out, err := h.svc.PreviewLeaveSettings(r.Context(), a, p)
	if err != nil {
		return err
	}
	return ok(w, dataOf(out))
}

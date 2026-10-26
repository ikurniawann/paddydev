package hris

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"time"

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

// SaveLeaveSettings is PUT /api/hris/leave-settings: merge the patch into the
// stored row, validate, save and audit (leave_settings.update) in one
// transaction.
func (s *Service) SaveLeaveSettings(ctx context.Context, a *Actor, p LeaveSettingsPatch, meta ...audit.Entry) (LeaveSettings, error) {
	if !a.LeaveBalanceAction("update") {
		return LeaveSettings{}, httpx.Forbidden("Insufficient permissions")
	}
	var entry audit.Entry
	if len(meta) > 0 {
		entry = meta[0]
	}
	err := s.repo.InTx(ctx, func(r Repository) error {
		q := r.querier()
		before, err := loadLeaveSettings(ctx, q, true)
		if err != nil {
			return err
		}
		next := before
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
		if err := validateLeaveSettings(next); err != nil {
			return err
		}
		if next.CarryOverEnabled && !before.CarryOverEnabled && !before.CarryOverReady {
			return httpx.BadRequest(carryOverLockedMsg)
		}
		if _, err := q.Exec(ctx, `INSERT INTO hris.leave_settings (id, carry_over_enabled, carry_over_max_days,
				carry_over_expiry_month, carry_over_expiry_day, updated_at, updated_by, updated_by_name)
			VALUES (true, $1, $2, $3, $4, now(), $5, $6)
			ON CONFLICT (id) DO UPDATE SET carry_over_enabled = EXCLUDED.carry_over_enabled,
			  carry_over_max_days = EXCLUDED.carry_over_max_days, carry_over_expiry_month = EXCLUDED.carry_over_expiry_month,
			  carry_over_expiry_day = EXCLUDED.carry_over_expiry_day, updated_at = now(),
			  updated_by = EXCLUDED.updated_by, updated_by_name = EXCLUDED.updated_by_name`,
			next.CarryOverEnabled, next.CarryOverMaxDays, next.CarryOverExpiryMonth, next.CarryOverExpiryDay,
			optionalUser(a), actorName(a)); err != nil {
			return err
		}
		entry.ActorID, entry.ActorName = a.UserID, ptr(actorName(a))
		entry.Action, entry.Entity = "leave_settings.update", "hris.leave_settings"
		entry.EntityLabel = ptr("Pengaturan sisa cuti bawaan")
		entry.Before, entry.After = settingsAudit(before), settingsAudit(next)
		return audit.Write(ctx, q, entry)
	})
	if err != nil {
		return LeaveSettings{}, err
	}
	return s.LeaveSettings(ctx)
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

// PUT /api/hris/leave-settings { carry_over_enabled, carry_over_max_days,
// carry_over_expiry_month, carry_over_expiry_day }
func (h handlers) putLeaveSettings(w http.ResponseWriter, r *http.Request, a *Actor) error {
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	enabled := boolean(f, "carry_over_enabled", optional)
	maxDays := number(f, "carry_over_max_days", nullish, validate.NumOpts{})
	month := f.Int("carry_over_expiry_month", optional, validate.NumOpts{})
	day := f.Int("carry_over_expiry_day", optional, validate.NumOpts{})
	if err := formErr(f, ""); err != nil {
		return err
	}
	out, err := h.svc.SaveLeaveSettings(r.Context(), a, LeaveSettingsPatch{Enabled: enabled.Val,
		MaxSent: maxDays.Sent, Max: maxDays.Val, Month: month, Day: day}, auditMeta(r))
	if err != nil {
		return err
	}
	return ok(w, dataMsg(out, "Pengaturan sisa cuti tersimpan"))
}

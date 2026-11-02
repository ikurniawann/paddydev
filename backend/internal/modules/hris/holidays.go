package hris

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
)

// Public holidays (lib/hris/holidays-db, holidays-input, holiday-ics).

// HolidayRepo is the public holiday storage.
type HolidayRepo interface {
	ListHolidays(ctx context.Context, start, end string, includeDraft bool) ([]*Row, error)
	CreateHoliday(ctx context.Context, in domain.HolidayInput, userID string) (*Row, error)
	UpdateHoliday(ctx context.Context, id string, in domain.HolidayInput, userID string) (*Row, error)
	SoftDeleteHoliday(ctx context.Context, id, userID string) (*Row, error)
	HolidayKeys(ctx context.Context, year int) (refs, dateNames map[string]bool, err error)
	ImportHoliday(ctx context.Context, in domain.HolidayInput, userID string) (created bool, err error)
}

const holidayColumns = `id, holiday_date::text AS holiday_date, name, type, deducts_leave, is_day_off, status, source, note`

// duplicateHoliday turns the (date, name) unique violation into a 409.
func duplicateHoliday(row *Row, err error) (*Row, error) {
	if database.IsUniqueViolation(err) {
		return nil, httpx.Conflict("Libur dengan tanggal dan nama yang sama sudah ada")
	}
	return row, err
}

func (s *Service) Holidays(ctx context.Context, start, end string, includeDraft bool) ([]*Row, error) {
	return s.repo.ListHolidays(ctx, start, end, includeDraft)
}

func (s *Service) CreateHoliday(ctx context.Context, in domain.HolidayInput, userID string) (*Row, error) {
	return duplicateHoliday(s.repo.CreateHoliday(ctx, in, userID))
}

func (s *Service) UpdateHoliday(ctx context.Context, id string, in domain.HolidayInput, userID string) (*Row, error) {
	row, err := duplicateHoliday(s.repo.UpdateHoliday(ctx, id, in, userID))
	if err == nil && row == nil {
		return nil, httpx.NotFound("Hari libur tidak ditemukan")
	}
	return row, err
}

func (s *Service) DeleteHoliday(ctx context.Context, id, userID string) (*Row, error) {
	row, err := s.repo.SoftDeleteHoliday(ctx, id, userID)
	if err == nil && row == nil {
		return nil, httpx.NotFound("Hari libur tidak ditemukan")
	}
	return row, err
}

// PreviewHolidayImport fetches the ICS feed and marks candidates already in
// the table. Nothing is written; a fetch failure is a 502.
func (s *Service) PreviewHolidayImport(ctx context.Context, year int) ([]*Row, error) {
	ics, err := s.calendar.FetchICS(ctx)
	if err != nil {
		s.log.ErrorContext(ctx, "[holidays/import] fetch ICS gagal", "error", err)
		return nil, httpx.Status(502, "Tidak bisa mengambil kalender hari libur: "+err.Error()+
			". Hari libur tetap bisa ditambahkan manual.")
	}
	refs, dateNames, err := s.repo.HolidayKeys(ctx, year)
	if err != nil {
		return nil, err
	}
	candidates := domain.ToHolidayCandidates(domain.ParseICS(ics), year)
	out := make([]*Row, len(candidates))
	for i, c := range candidates {
		row := obj("source_ref", c.SourceRef, "holiday_date", c.HolidayDate, "name", c.Name, "type", c.Type,
			"deducts_leave", c.DeductsLeave, "status", c.Status, "suggested", c.Suggested)
		if c.Reason != nil {
			row.Set("reason", *c.Reason)
		}
		row.Set("already_imported", refs[c.SourceRef] || dateNames[c.HolidayDate+"|"+c.Name])
		out[i] = row
	}
	return out, nil
}

// ImportHolidays saves the ticked rows in one transaction.
func (s *Service) ImportHolidays(ctx context.Context, items []domain.HolidayInput, userID string) (created, updated int, err error) {
	err = s.repo.InTx(ctx, func(r Repository) error {
		for _, item := range items {
			isNew, err := r.ImportHoliday(ctx, item, userID)
			if err != nil {
				return err
			}
			if isNew {
				created++
			} else {
				updated++
			}
		}
		return nil
	})
	return created, updated, err
}

/* ── SQL ─────────────────────────────────────────────────────────────── */

func (s *store) ListHolidays(ctx context.Context, start, end string, includeDraft bool) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT `+holidayColumns+`
		FROM hris.public_holidays
		WHERE deleted_at IS NULL
		  AND holiday_date BETWEEN $1::date AND $2::date
		  AND ($3::boolean OR status = 'aktif')
		ORDER BY holiday_date ASC, name ASC`, start, end, includeDraft)
}

func trimmed(p *string) *string {
	if p == nil {
		return nil
	}
	t := domain.JSTrim(*p)
	return &t
}

// dayOff is the requested is_day_off; unset means false.
func dayOff(in domain.HolidayInput) bool { return in.IsDayOff != nil && *in.IsDayOff }

// CreateHoliday inserts the holiday; one that is created as a day off writes
// its holiday.day_off audit row in the same transaction.
func (s *store) CreateHoliday(ctx context.Context, in domain.HolidayInput, userID string) (row *Row, err error) {
	err = database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		q := &store{db: tx}
		if row, err = q.insertHoliday(ctx, in, userID); err != nil || row == nil {
			return err
		}
		if row.Bool("is_day_off") {
			return writeHolidayDayOffAudit(ctx, tx, userID, row.Str("id"), row.Str("name"), nil, true)
		}
		return nil
	})
	return row, err
}

func (s *store) insertHoliday(ctx context.Context, in domain.HolidayInput, userID string) (*Row, error) {
	typ := domain.HolidayTypeOrDefault(in.Type)
	deducts := domain.DefaultDeductsLeave(typ)
	if in.DeductsLeave != nil {
		deducts = *in.DeductsLeave
	}
	status := "aktif"
	if in.Status != nil {
		status = *in.Status
	}
	return queryRow(ctx, s.db, `INSERT INTO hris.public_holidays
		(holiday_date, name, type, deducts_leave, is_day_off, status, source, note, created_by, updated_by)
		VALUES ($1::date, $2, $3, $4, $8, $5, 'manual', $6, $7, $7)
		RETURNING `+holidayColumns,
		in.HolidayDate, trimmed(in.Name), typ, deducts, status, nilIfEmpty(trimmed(in.Note)), userID, dayOff(in))
}

// UpdateHoliday patches the holiday; a change of is_day_off (also the reset
// when the type leaves cuti bersama) writes a holiday.day_off audit row in
// the same transaction.
func (s *store) UpdateHoliday(ctx context.Context, id string, in domain.HolidayInput, userID string) (row *Row, err error) {
	err = database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		before, found, err := holidayDayOff(ctx, tx, id, true)
		if err != nil {
			return err
		}
		q := &store{db: tx}
		if row, err = q.patchHoliday(ctx, id, in, userID); err != nil || row == nil || !found {
			return err
		}
		after, _, err := holidayDayOff(ctx, tx, id, false)
		if err != nil || after == before {
			return err
		}
		return writeHolidayDayOffAudit(ctx, tx, userID, id, row.Str("name"), &before, after)
	})
	return row, err
}

func (s *store) patchHoliday(ctx context.Context, id string, in domain.HolidayInput, userID string) (*Row, error) {
	// Changing the type without deducts_leave takes the type's SKB default.
	deducts := in.DeductsLeave
	if deducts == nil && in.Type != nil && *in.Type != "" {
		v := *in.Type == "cuti_bersama"
		deducts = &v
	}
	// is_day_off is for cuti bersama only: a PATCH that leaves the type alone
	// is checked against the stored type.
	if dayOff(in) && in.Type == nil {
		var typ string
		err := s.db.QueryRow(ctx, `SELECT type FROM hris.public_holidays WHERE id = $1 AND deleted_at IS NULL`, id).Scan(&typ)
		if err == nil && typ != "cuti_bersama" {
			return nil, httpx.BadRequest(domain.ErrDayOffType)
		}
	}
	return queryRow(ctx, s.db, `UPDATE hris.public_holidays SET
		holiday_date  = COALESCE($2::date, holiday_date),
		name          = COALESCE($3, name),
		type          = COALESCE($4, type),
		deducts_leave = COALESCE($5, deducts_leave),
		is_day_off    = CASE WHEN COALESCE($4, type) <> 'cuti_bersama' THEN false ELSE COALESCE($10::boolean, is_day_off) END,
		status        = COALESCE($6, status),
		note          = CASE WHEN $8::boolean THEN $7 ELSE note END,
		updated_by    = $9,
		updated_at    = now()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, name`,
		id, in.HolidayDate, trimmed(in.Name), in.Type, deducts, in.Status, nilIfEmpty(trimmed(in.Note)), in.NoteSent, userID, in.IsDayOff)
}

func (s *store) SoftDeleteHoliday(ctx context.Context, id, userID string) (*Row, error) {
	return queryRow(ctx, s.db, `UPDATE hris.public_holidays
		SET deleted_at = now(), updated_by = $2, updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, name`, id, userID)
}

func (s *store) HolidayKeys(ctx context.Context, year int) (map[string]bool, map[string]bool, error) {
	y := strconv.Itoa(year)
	rows, err := s.db.Query(ctx, `SELECT source_ref, holiday_date::text, name
		FROM hris.public_holidays
		WHERE deleted_at IS NULL AND holiday_date BETWEEN $1::date AND $2::date`, y+"-01-01", y+"-12-31")
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	refs, dateNames := map[string]bool{}, map[string]bool{}
	for rows.Next() {
		var ref *string
		var date, name string
		if err := rows.Scan(&ref, &date, &name); err != nil {
			return nil, nil, err
		}
		if ref != nil && *ref != "" {
			refs[*ref] = true
		}
		dateNames[date+"|"+name] = true
	}
	return refs, dateNames, rows.Err()
}

// ImportHoliday updates the row with the same source_ref, else upserts on
// (date, name) without touching source and note. created reports an insert.
func (s *store) ImportHoliday(ctx context.Context, in domain.HolidayInput, userID string) (created bool, err error) {
	err = database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		// is_day_off of the row the import will touch (same source_ref, else
		// same date and name) is the audit "before"; none = a new row.
		var before *bool
		if in.SourceRef != nil && *in.SourceRef != "" {
			if before, err = holidayDayOffWhere(ctx, tx, `source_ref = $1`, *in.SourceRef); err != nil {
				return err
			}
		}
		if before == nil {
			if before, err = holidayDayOffWhere(ctx, tx, `holiday_date = $1::date AND name = $2`, in.HolidayDate, trimmed(in.Name)); err != nil {
				return err
			}
		}
		var id, name string
		var after bool
		if id, name, after, created, err = (&store{db: tx}).upsertHoliday(ctx, in, userID); err != nil {
			return err
		}
		if (before == nil && !after) || (before != nil && *before == after) {
			return nil
		}
		return writeHolidayDayOffAudit(ctx, tx, userID, id, name, before, after)
	})
	return created, err
}

func (s *store) upsertHoliday(ctx context.Context, in domain.HolidayInput, userID string) (id, name string, isDayOff, created bool, err error) {
	typ := domain.HolidayTypeOrDefault(in.Type)
	deducts := typ == "cuti_bersama"
	if in.DeductsLeave != nil {
		deducts = *in.DeductsLeave
	}
	status := "aktif"
	if in.Status != nil {
		status = *in.Status
	}
	args := []any{in.HolidayDate, trimmed(in.Name), typ, deducts, status, in.SourceRef, userID, in.IsDayOff}
	if in.SourceRef != nil && *in.SourceRef != "" {
		err := s.db.QueryRow(ctx, `UPDATE hris.public_holidays SET
			holiday_date = $1::date, name = $2, type = $3,
			deducts_leave = $4, status = $5,
			is_day_off = CASE WHEN $3 <> 'cuti_bersama' THEN false ELSE COALESCE($8::boolean, is_day_off) END,
			updated_by = $7, updated_at = now()
			WHERE source_ref = $6 AND deleted_at IS NULL
			RETURNING id::text, name, is_day_off`, args...).Scan(&id, &name, &isDayOff)
		if err == nil {
			return id, name, isDayOff, false, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", "", false, false, err
		}
	}
	err = s.db.QueryRow(ctx, `INSERT INTO hris.public_holidays
		(holiday_date, name, type, deducts_leave, is_day_off, status, source, source_ref, created_by, updated_by)
		VALUES ($1::date, $2, $3, $4, COALESCE($8::boolean, false), $5, 'impor', $6, $7, $7)
		ON CONFLICT (holiday_date, name) WHERE deleted_at IS NULL
		DO UPDATE SET
		  type = EXCLUDED.type,
		  deducts_leave = EXCLUDED.deducts_leave,
		  is_day_off = CASE WHEN EXCLUDED.type <> 'cuti_bersama' THEN false
		    WHEN $8::boolean IS NULL THEN hris.public_holidays.is_day_off ELSE EXCLUDED.is_day_off END,
		  status = EXCLUDED.status,
		  source_ref = COALESCE(EXCLUDED.source_ref, hris.public_holidays.source_ref),
		  updated_by = EXCLUDED.updated_by,
		  updated_at = now()
		RETURNING id::text, name, is_day_off, (xmax = 0) AS inserted`, args...).Scan(&id, &name, &isDayOff, &created)
	return id, name, isDayOff, created, err
}

// holidayDayOff reads is_day_off of a live holiday by id (FOR UPDATE when
// lock); found is false when there is none.
func holidayDayOff(ctx context.Context, q database.Querier, id string, lock bool) (dayOff, found bool, err error) {
	sql := `SELECT is_day_off FROM hris.public_holidays WHERE id = $1 AND deleted_at IS NULL`
	if lock {
		sql += ` FOR UPDATE`
	}
	err = q.QueryRow(ctx, sql, id).Scan(&dayOff)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	return dayOff, err == nil, err
}

// holidayDayOffWhere is is_day_off of the first live holiday matching cond
// (a SQL condition over $1, $2…); nil when none.
func holidayDayOffWhere(ctx context.Context, q database.Querier, cond string, args ...any) (*bool, error) {
	var v bool
	err := q.QueryRow(ctx, `SELECT is_day_off FROM hris.public_holidays WHERE deleted_at IS NULL AND `+cond+` LIMIT 1`, args...).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// writeHolidayDayOffAudit records a change of a holiday's is_day_off
// (EPIC-079 R2: it decides who rests on a cuti bersama) on q, so it commits
// with the change. before nil = the holiday is new.
func writeHolidayDayOffAudit(ctx context.Context, q database.Querier, userID, holidayID, name string, before *bool, after bool) error {
	e := audit.Entry{Action: "holiday.day_off", Entity: "hris.public_holidays", EntityID: &holidayID, EntityLabel: &name,
		ActorID: userID, After: map[string]any{"is_day_off": after}}
	if before != nil {
		e.Before = map[string]any{"is_day_off": *before}
	}
	var actor *string
	if err := q.QueryRow(ctx, `SELECT full_name FROM users WHERE id = $1`, userID).Scan(&actor); err == nil {
		e.ActorName = actor
	}
	return audit.Write(ctx, q, e)
}

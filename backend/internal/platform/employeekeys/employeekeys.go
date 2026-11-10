// Package employeekeys finds the employee that already holds a unique
// identity key (KTP, NPWP, NIP) of hris.employees, so every write path can
// tell HR who owns it instead of a generic duplicate error.
package employeekeys

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/nipformat"
	"nuhabit/backend/internal/platform/validate"
)

// CleanNIP is the one NIP cleaner of every write path (EPIC-081): trim, blank
// (or absent/null) is 400 "NIP wajib diisi", over 50 characters is 400
// "NIP maksimal 50 karakter". Nothing ever makes a NIP up.
func CleanNIP(raw *string) (string, error) {
	in := ""
	if raw != nil {
		in = *raw
	}
	nip, problem := validate.CleanNIP(in)
	if problem != "" {
		return "", httpx.BadRequest(problem)
	}
	return nip, nil
}

// Keys are the unique identity columns of hris.employees. A nil, empty or
// whitespace-only key is "not given" and never conflicts.
type Keys struct{ KTP, NPWP, NIP *string }

// Conflict describes the employee already holding one of the keys.
type Conflict struct {
	Field  string // "ktp" | "npwp" | "nip"
	Name   string
	NIP    string
	Active bool
}

var labels = map[string]string{"ktp": "No. KTP", "npwp": "NPWP", "nip": "NIP"}

// Message is the user-facing text. An inactive owner is pointed at the
// reactivate action rather than at creating a second record.
func (c Conflict) Message() string {
	status := "nonaktif"
	if c.Active {
		status = "aktif"
	}
	msg := fmt.Sprintf("%s sudah dipakai karyawan %s (NIP %s, %s)", labels[c.Field], c.Name, c.NIP, status)
	if !c.Active {
		msg += " — aktifkan kembali data tersebut dari halaman detail karyawan"
	}
	return msg
}

// ConstraintMessage maps employees_{ktp,npwp,nip}_key to the text used when a
// concurrent write slips past Find; ok is false for any other constraint.
func ConstraintMessage(constraint string) (string, bool) {
	return httpx.UniqueConstraintMessage(constraint)
}

// digits keeps only the 0-9 characters of s.
func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Find returns the first conflict (order: ktp, npwp, nip) with any employee
// other than excludeID ("" = none), active or not. KTP and NPWP compare
// digits only, so "3273 0101…" and "3273010…" are the same key; NIP compares
// trimmed text. Active owners are preferred when several rows match.
func Find(ctx context.Context, q database.Querier, k Keys, excludeID string) (*Conflict, error) {
	checks := []struct {
		field, value, match string
	}{
		{"ktp", deref(k.KTP), `regexp_replace(coalesce(ktp, ''), '\D', '', 'g') = $1`},
		{"npwp", deref(k.NPWP), `regexp_replace(coalesce(npwp, ''), '\D', '', 'g') = $1`},
		{"nip", strings.TrimSpace(deref(k.NIP)), `btrim(nip) = $1`},
	}
	for _, c := range checks {
		if c.field != "nip" {
			c.value = digits(c.value)
		}
		if c.value == "" {
			continue
		}
		var found Conflict
		err := q.QueryRow(ctx, `SELECT full_name, nip, is_active FROM hris.employees
			WHERE `+c.match+` AND ($2::text = '' OR id::text <> $2::text)
			ORDER BY is_active DESC, created_at LIMIT 1`, c.value, excludeID).Scan(&found.Name, &found.NIP, &found.Active)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		found.Field = c.field
		return &found, nil
	}
	return nil, nil
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// allNIPs is every NIP held, active or not (EPIC-099 numbers never reuse).
func allNIPs(ctx context.Context, q database.Querier) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT btrim(nip) FROM hris.employees WHERE nip IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func lastFor(ctx context.Context, q database.Querier, joinDate string) (nipformat.Last, error) {
	nips, err := allNIPs(ctx, q)
	if err != nil {
		return nipformat.Last{}, err
	}
	year := 0
	if len(joinDate) >= 4 {
		year, _ = strconv.Atoi(joinDate[:4])
	}
	return nipformat.LastFor(nips, year), nil
}

// CheckNewNIP refuses the NIP of a new employee that is not YYYYMM of the
// join date + the next number of its series (EPIC-099), as a 400.
func CheckNewNIP(ctx context.Context, q database.Querier, nip, joinDate, status string) error {
	l, err := lastFor(ctx, q, joinDate)
	if err != nil {
		return err
	}
	if msg := nipformat.CheckNew(nip, joinDate, status, l); msg != "" {
		return httpx.BadRequest(msg)
	}
	return nil
}

// NextNIP is the NIP that follows the latest number of the series.
func NextNIP(ctx context.Context, q database.Querier, joinDate, status string) (string, error) {
	l, err := lastFor(ctx, q, joinDate)
	if err != nil {
		return "", err
	}
	return nipformat.Next(joinDate, nipformat.SeriesFor(status), l), nil
}

// CheckChangedNIP refuses a replaced NIP out of the format or not matching
// the join date's YYYYMM; its number is not checked (typo fixes).
func CheckChangedNIP(nip, joinDate string) error {
	if msg := nipformat.CheckChanged(nip, joinDate); msg != "" {
		return httpx.BadRequest(msg)
	}
	return nil
}

// Package employeekeys finds the employee that already holds a unique
// identity key (KTP, NPWP, NIP) of hris.employees, so every write path can
// tell HR who owns it instead of a generic duplicate error.
package employeekeys

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
)

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

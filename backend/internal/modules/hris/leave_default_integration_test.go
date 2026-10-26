package hris

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"nuhabit/backend/internal/modules/hris/domain"
)

// EPIC-068 R1: hris.default_annual_leave (SQL) and domain.DefaultAnnualLeave
// (Go) must give the same quota for every join date, including leap days,
// year boundaries and a missing join date.
func TestDefaultAnnualLeaveSQLMatchesGo(t *testing.T) {
	h := newHarness(t)
	joins := []string{
		"2020-02-29", "2023-12-31", "2024-01-01", "2024-02-29", "2024-03-01", "2024-12-31",
		"2025-01-01", "2025-01-02", "2025-02-28", "2025-03-10", "2025-06-30", "2025-07-01",
		"2025-12-30", "2025-12-31", "2026-01-01", "2026-02-01", "2026-12-31", "2027-07-01",
		"2027-12-31", "2028-02-29",
	}
	for _, year := range []int{2024, 2025, 2026, 2028} {
		for _, join := range joins {
			got := h.scalar(`SELECT hris.default_annual_leave($1::date, $2::int)::text`, join, year)
			want := domain.DefaultAnnualLeave(&join, year)
			if f, err := strconv.ParseFloat(got, 64); err != nil || f != want {
				t.Errorf("join %s year %d: SQL %s, Go %v", join, year, got, want)
			}
		}
		if got := h.scalar(`SELECT hris.default_annual_leave(NULL, $1::int)::text`, year); got != "12" {
			t.Errorf("join NULL year %d: SQL %s, want 12", year, got)
		}
	}
	// Brief examples.
	for _, c := range []struct {
		join string
		year int
		want string
	}{{"2025-03-10", 2026, "9.5"}, {"2026-02-01", 2026, "0"}, {"2024-12-31", 2026, "12"}} {
		got, _ := strconv.ParseFloat(h.scalar(`SELECT hris.default_annual_leave($1::date, $2::int)::text`, c.join, c.year), 64)
		if want, _ := strconv.ParseFloat(c.want, 64); got != want {
			t.Errorf("%s/%d: got %v, want %s", c.join, c.year, got, c.want)
		}
	}
}

// EPIC-068 R14: a new employee gets a row for the join year and one for the
// current (WIB) year, both from R1; the generated remaining column and the
// R3 CHECKs hold.
func TestNewEmployeeLeaveBalanceRows(t *testing.T) {
	h := newHarness(t)
	curYear, _ := strconv.Atoi(h.scalar(`SELECT extract(year FROM now() AT TIME ZONE 'Asia/Jakarta')::int::text`))
	join := fmt.Sprintf("%d-03-10", curYear-1)
	id := h.employee(emp{join: join})

	for _, year := range []int{curYear - 1, curYear} {
		got, _ := strconv.ParseFloat(h.scalar(`SELECT annual_leave_total::text FROM hris.leave_balances WHERE employee_id = $1 AND year = $2`, id, year), 64)
		if want := domain.DefaultAnnualLeave(&join, year); got != want {
			t.Errorf("year %d: total %v, want %v", year, got, want)
		}
	}
	if n := h.scalar(`SELECT count(*)::text FROM hris.leave_balances WHERE employee_id = $1`, id); n != "2" {
		t.Errorf("rows = %s, want 2", n)
	}

	// Joining this year: one row only.
	thisYear := h.employee(emp{join: fmt.Sprintf("%d-01-15", curYear)})
	if n := h.scalar(`SELECT count(*)::text FROM hris.leave_balances WHERE employee_id = $1`, thisYear); n != "1" {
		t.Errorf("join-year = current-year rows = %s, want 1", n)
	}

	h.exec(`UPDATE hris.leave_balances SET annual_leave_total = 12, annual_leave_adjustment = 1.5, annual_leave_used = 3
		WHERE employee_id = $1 AND year = $2`, id, curYear)
	if got := h.scalar(`SELECT annual_leave_remaining::text FROM hris.leave_balances WHERE employee_id = $1 AND year = $2`, id, curYear); got != "10.50" {
		t.Errorf("remaining = %s, want 10.50 (12 + 1.5 - 3)", got)
	}

	for name, set := range map[string]string{
		"sisa negatif":              "annual_leave_adjustment = -10",
		"jatah bukan kelipatan 0,5": "annual_leave_total = 10.25",
		"penyesuaian 0,25":          "annual_leave_adjustment = 0.25",
		"bawaan 0,3":                "carry_over_days = 0.3",
		"bawaan terpakai negatif":   "carry_over_used = -1",
	} {
		h.expectDBError(t, name, `UPDATE hris.leave_balances SET `+set+` WHERE employee_id = $1 AND year = $2`, id, curYear)
	}
	h.exec(`UPDATE hris.leave_balances SET annual_leave_adjustment = -2.5, carry_over_days = 2, carry_over_used = 1.5,
		carry_over_expires_on = $3::date WHERE employee_id = $1 AND year = $2`, id, curYear, fmt.Sprintf("%d-03-31", curYear))
	// The recalculation may lower the carry-over below what is used (a review
	// is opened): the table stores it.
	h.exec(`UPDATE hris.leave_balances SET carry_over_days = 1, carry_over_used = 1.5 WHERE employee_id = $1 AND year = $2`, id, curYear)
}

// expectDBError runs sql and fails the test unless the database rejects it;
// a savepoint keeps the fixture transaction usable afterwards.
func (h *harness) expectDBError(t *testing.T, name, sql string, args ...any) {
	t.Helper()
	ctx := context.Background()
	h.exec(`SAVEPOINT expect_err`)
	_, err := h.tx.Exec(ctx, sql, args...)
	h.exec(`ROLLBACK TO SAVEPOINT expect_err`)
	if err == nil {
		t.Errorf("%s: database accepted %q", name, sql)
	}
}

// Seed rows and settings of the migration: the single leave_settings row and
// R8 defaults, and the job claim table.
func TestLeaveSettingsDefaults(t *testing.T) {
	h := newHarness(t)
	if got := h.scalar(`SELECT carry_over_enabled::text || '/' || coalesce(carry_over_max_days::text, 'null') || '/' ||
		carry_over_expiry_month || '/' || carry_over_expiry_day FROM hris.leave_settings`); got != "false/null/3/31" {
		t.Errorf("settings default = %q", got)
	}
	h.expectDBError(t, "29 Februari", `UPDATE hris.leave_settings SET carry_over_expiry_month = 2, carry_over_expiry_day = 29`)
	h.expectDBError(t, "31 April", `UPDATE hris.leave_settings SET carry_over_expiry_month = 4, carry_over_expiry_day = 31`)
	h.expectDBError(t, "maks 0,3", `UPDATE hris.leave_settings SET carry_over_max_days = 0.3`)
	h.exec(`UPDATE hris.leave_settings SET carry_over_expiry_month = 2, carry_over_expiry_day = 28, carry_over_max_days = 5.5`)
	h.exec(`INSERT INTO hris.leave_carry_over_runs (year) VALUES ($1) ON CONFLICT DO NOTHING`, time.Now().Year()+50)
}

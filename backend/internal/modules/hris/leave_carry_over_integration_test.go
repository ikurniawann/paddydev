package hris

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"nuhabit/backend/internal/platform/config"
	"nuhabit/backend/internal/platform/module"
)

// EPIC-068 Task 4: job sisa bawaan 1 Januari (R9).

var wibZone = time.FixedZone("WIB", 7*3600)

// jan1 is 00:00 WIB of 1 January year.
func jan1(year int) time.Time { return time.Date(year, 1, 1, 0, 0, 0, 0, wibZone) }

// carrySetup clears the claims and sets the carry-over settings.
func (h *harness) carrySetup(enabled bool, max any, month, day int) {
	h.t.Helper()
	h.exec(`DELETE FROM hris.leave_carry_over_runs`)
	h.exec(`UPDATE hris.leave_settings SET carry_over_enabled = $1, carry_over_max_days = $2,
		carry_over_expiry_month = $3, carry_over_expiry_day = $4`, enabled, max, month, day)
	h.unlockCarryOver()
}

// unlockCarryOver stands in for the follow-up migration that creates
// hris.leave_carry_over_reviews (the carry-over unlock marker); the test
// transaction rolls it back.
func (h *harness) unlockCarryOver() {
	h.t.Helper()
	h.exec(`CREATE TABLE IF NOT EXISTS hris.leave_carry_over_reviews (employee_id uuid)`)
}

func (h *harness) carryOf(employeeID string, year int) string {
	h.t.Helper()
	return h.scalar(`SELECT concat_ws('|', carry_over_days, carry_over_used, coalesce(carry_over_expires_on::text, '-'))
		FROM hris.leave_balances WHERE employee_id = $1 AND year = $2`, employeeID, year)
}

func (h *harness) carryAudits(employeeID string) string {
	h.t.Helper()
	return h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'leave_balance.carry_over' AND entity_id = $1`, employeeID)
}

func TestCarryAmount(t *testing.T) {
	two := 2.0
	five := 5.0
	for _, c := range []struct {
		rem  float64
		max  *float64
		want float64
	}{{8, &five, 5}, {3.5, &five, 3.5}, {8, nil, 8}, {7.75, nil, 7.5}, {0, nil, 0}, {8, &two, 2}, {8.25, &five, 5}} {
		if got := carryAmount(c.rem, c.max); got != c.want {
			t.Fatalf("carryAmount(%v, %v) = %v, want %v", c.rem, c.max, got, c.want)
		}
	}
}

// Setting off: only the year's rows are created, nothing is carried.
func TestLeaveCarryOverDisabled(t *testing.T) {
	h := newHarness(t)
	h.carrySetup(false, nil, 3, 31)
	a := h.employee(emp{name: "Bawaan Nonaktif", join: "2024-01-10"})
	h.setBalance(a, 2026, 12, 0, 4, 0, 0, nil)
	if h.scalar(`SELECT count(*)::text FROM hris.leave_balances WHERE employee_id = $1 AND year = 2027`, a) != "0" {
		t.Fatal("2027 row existed before the run")
	}

	if err := h.svc.RunLeaveCarryOver(context.Background(), jan1(2027)); err != nil {
		t.Fatal(err)
	}
	if got := h.carryOf(a, 2027); got != "0.00|0.00|-" {
		t.Fatalf("2027 carry = %q", got)
	}
	if got := h.scalar(`SELECT annual_leave_total::text FROM hris.leave_balances WHERE employee_id = $1 AND year = 2027`, a); got != "12.00" {
		t.Fatalf("2027 total = %q (R1 default)", got)
	}
	if h.carryAudits(a) != "0" {
		t.Fatal("audit written while the setting is off")
	}
	run := h.scalar(`SELECT concat_ws('|', (finished_at IS NOT NULL)::text, (employees >= 1)::text, carried)
		FROM hris.leave_carry_over_runs WHERE year = 2027`)
	if run != "true|true|0" {
		t.Fatalf("run = %q", run)
	}
}

// Setting on with a maximum: carry = min(remaining, max); a null maximum
// carries everything; HR input is never overwritten; one audit per change.
func TestLeaveCarryOverEnabled(t *testing.T) {
	h := newHarness(t)
	h.carrySetup(true, 5, 6, 30)
	ctx := context.Background()
	over := h.employee(emp{name: "Bawaan Lebih", join: "2024-01-10"})      // remaining 8 > max 5
	under := h.employee(emp{name: "Bawaan Kurang", join: "2024-01-10"})    // remaining 2.5 < max
	none := h.employee(emp{name: "Bawaan Habis", join: "2024-01-10"})      // remaining 0
	hr := h.employee(emp{name: "Bawaan Isian HR", join: "2024-01-10"})     // HR already typed a carry
	adj := h.employee(emp{name: "Bawaan Penyesuaian", join: "2024-01-10"}) // total 12 + adj 2 - used 6 = 8 -> 5
	off := h.employee(emp{name: "Bawaan Nonaktif", join: "2024-01-10", inactive: true})
	newbie := h.employee(emp{name: "Bawaan Tanpa 2026", join: "2024-01-10"}) // no 2026 row
	h.setBalance(over, 2026, 12, 0, 4, 0, 0, nil)
	h.setBalance(under, 2026, 12, 0, 9.5, 0, 0, nil)
	h.setBalance(none, 2026, 12, 0, 12, 0, 0, nil)
	h.setBalance(hr, 2026, 12, 0, 2, 0, 0, nil)
	h.setBalance(hr, 2027, 12, 0, 0, 3, 0, "2027-09-30")
	h.setBalance(adj, 2026, 12, 2, 6, 0, 0, nil)
	h.setBalance(off, 2026, 12, 0, 0, 0, 0, nil)
	h.exec(`DELETE FROM hris.leave_balances WHERE employee_id = $1 AND year = 2026`, newbie)

	if err := h.svc.RunLeaveCarryOver(ctx, jan1(2027)); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, id, want string
	}{
		{"over max", over, "5.00|0.00|2027-06-30"},
		{"under max", under, "2.50|0.00|2027-06-30"},
		{"none left", none, "0.00|0.00|-"},
		{"HR input kept", hr, "3.00|0.00|2027-09-30"},
		{"with adjustment", adj, "5.00|0.00|2027-06-30"},
		{"no 2026 row", newbie, "0.00|0.00|-"},
	} {
		if got := h.carryOf(c.id, 2027); got != c.want {
			t.Fatalf("%s: 2027 carry = %q, want %q", c.name, got, c.want)
		}
	}
	if h.scalar(`SELECT count(*)::text FROM hris.leave_balances WHERE employee_id = $1 AND year = 2027`, off) != "0" {
		t.Fatal("an inactive employee got a 2027 row")
	}

	// One audit per changed employee, none for the others.
	for id, want := range map[string]string{over: "1", under: "1", adj: "1", none: "0", hr: "0", newbie: "0", off: "0"} {
		if got := h.carryAudits(id); got != want {
			t.Fatalf("audits of %s = %s, want %s", id, got, want)
		}
	}
	a := h.scalar(`SELECT concat_ws('|', entity, entity_label, (actor_id IS NULL)::text, actor_name,
		before->>'carry_over_days', after->>'carry_over_days', after->>'carry_over_expires_on', after->>'year', (reason <> '')::text)
		FROM audit.audit_log WHERE action = 'leave_balance.carry_over' AND entity_id = $1`, over)
	if want := "hris.employees|Bawaan Lebih · Saldo cuti 2027|true|Sistem|0|5|2027-06-30|2027|true"; a != want {
		t.Fatalf("audit = %q, want %q", a, want)
	}
	run := h.scalar(`SELECT concat_ws('|', (finished_at IS NOT NULL)::text, (employees >= 6)::text,
		(carried = (SELECT count(*) FROM audit.audit_log WHERE action = 'leave_balance.carry_over'))::text)
		FROM hris.leave_carry_over_runs WHERE year = 2027`)
	if run != "true|true|true" {
		t.Fatalf("run = %q", run)
	}
}

// A null maximum carries the whole remainder.
func TestLeaveCarryOverNoMax(t *testing.T) {
	h := newHarness(t)
	h.carrySetup(true, nil, 3, 31)
	a := h.employee(emp{name: "Bawaan Semua", join: "2024-01-10"})
	h.setBalance(a, 2026, 12, 0, 1.5, 0, 0, nil)
	if err := h.svc.RunLeaveCarryOver(context.Background(), jan1(2027)); err != nil {
		t.Fatal(err)
	}
	if got := h.carryOf(a, 2027); got != "10.50|0.00|2027-03-31" {
		t.Fatalf("carry = %q", got)
	}
}

// Review 4: a second run (a restart, a second replica) does nothing, even
// when the settings changed in between.
func TestLeaveCarryOverRunsOnce(t *testing.T) {
	h := newHarness(t)
	h.carrySetup(true, 5, 3, 31)
	ctx := context.Background()
	a := h.employee(emp{name: "Bawaan Sekali", join: "2024-01-10"})
	h.setBalance(a, 2026, 12, 0, 4, 0, 0, nil)
	if err := h.svc.RunLeaveCarryOver(ctx, jan1(2027)); err != nil {
		t.Fatal(err)
	}
	if h.carryOf(a, 2027) != "5.00|0.00|2027-03-31" {
		t.Fatalf("first run carry = %q", h.carryOf(a, 2027))
	}
	// A leave consumed the carry-over meanwhile; settings now allow more.
	h.exec(`UPDATE hris.leave_balances SET carry_over_used = 2 WHERE employee_id = $1 AND year = 2027`, a)
	h.exec(`UPDATE hris.leave_settings SET carry_over_max_days = 8`)
	h.exec(`UPDATE hris.leave_balances SET carry_over_days = 0, carry_over_used = 0, carry_over_expires_on = NULL
		WHERE employee_id = $1 AND year = 2027`, a)
	for _, at := range []time.Time{jan1(2027), jan1(2027).Add(15 * time.Minute), time.Date(2027, 6, 1, 10, 0, 0, 0, wibZone)} {
		if err := h.svc.RunLeaveCarryOver(ctx, at); err != nil {
			t.Fatal(err)
		}
	}
	if got := h.carryOf(a, 2027); got != "0.00|0.00|-" {
		t.Fatalf("later runs changed the row: %q", got)
	}
	if h.carryAudits(a) != "1" {
		t.Fatalf("audits = %s, want 1", h.carryAudits(a))
	}
	if h.scalar(`SELECT count(*)::text FROM hris.leave_carry_over_runs WHERE year = 2027`) != "1" {
		t.Fatal("more than one claim for 2027")
	}
}

// R9: nothing for a year before 00:00 WIB of 1 January of that year.
func TestLeaveCarryOverNotBeforeJanuary(t *testing.T) {
	h := newHarness(t)
	h.carrySetup(true, nil, 3, 31)
	ctx := context.Background()
	a := h.employee(emp{name: "Bawaan Desember", join: "2024-01-10"})
	h.setBalance(a, 2026, 12, 0, 4, 0, 0, nil)
	h.exec(`INSERT INTO hris.leave_carry_over_runs (year, finished_at) VALUES (2026, now())`)

	// 31 Dec 2026 23:59 WIB is still 2026 (already claimed): nothing at all.
	if err := h.svc.RunLeaveCarryOver(ctx, time.Date(2026, 12, 31, 23, 59, 0, 0, wibZone)); err != nil {
		t.Fatal(err)
	}
	if h.scalar(`SELECT count(*)::text FROM hris.leave_carry_over_runs WHERE year = 2027`) != "0" ||
		h.scalar(`SELECT count(*)::text FROM hris.leave_balances WHERE employee_id = $1 AND year = 2027`, a) != "0" {
		t.Fatal("the job ran for 2027 before 1 January 2027 WIB")
	}
	// One minute later it is 1 January 2027 00:00 WIB.
	if err := h.svc.RunLeaveCarryOver(ctx, time.Date(2026, 12, 31, 23, 59, 0, 0, wibZone).Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if h.carryOf(a, 2027) != "8.00|0.00|2027-03-31" {
		t.Fatalf("carry at 1 Jan 00:00 = %q", h.carryOf(a, 2027))
	}
}

// R9: the claim is part of the transaction; a failure releases it so the next
// tick retries.
func TestLeaveCarryOverRollsBackClaim(t *testing.T) {
	h := newHarness(t)
	h.carrySetup(true, nil, 3, 31)
	a := h.employee(emp{name: "Bawaan Gagal", join: "2024-01-10"})
	h.setBalance(a, 2026, 12, 0, 4, 0, 0, nil)
	h.exec(`CREATE FUNCTION pg_temp.fail_carry_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit gagal'; END $$`)
	h.exec(`CREATE TRIGGER fail_carry_audit BEFORE INSERT ON audit.audit_log FOR EACH ROW
		WHEN (NEW.action = 'leave_balance.carry_over') EXECUTE FUNCTION pg_temp.fail_carry_audit()`)

	// Never leave the trigger behind, even when the test fails early.
	t.Cleanup(func() {
		_, _ = h.tx.Exec(context.Background(), `DROP TRIGGER IF EXISTS fail_carry_audit ON audit.audit_log`)
	})

	err := h.svc.RunLeaveCarryOver(context.Background(), jan1(2027))
	if err == nil || !strings.Contains(err.Error(), "audit gagal") {
		t.Fatalf("run error = %v, want the injected audit failure", err)
	}
	if h.scalar(`SELECT count(*)::text FROM hris.leave_carry_over_runs WHERE year = 2027`) != "0" {
		t.Fatal("the claim survived a failed run")
	}
	if h.scalar(`SELECT count(*)::text FROM hris.leave_balances WHERE employee_id = $1 AND year = 2027`, a) != "0" {
		t.Fatal("rows of a failed run survived")
	}
	h.exec(`DROP TRIGGER fail_carry_audit ON audit.audit_log`)
	if err := h.svc.RunLeaveCarryOver(context.Background(), jan1(2027)); err != nil {
		t.Fatal(err)
	}
	if h.carryOf(a, 2027) != "8.00|0.00|2027-03-31" {
		t.Fatalf("retry carry = %q", h.carryOf(a, 2027))
	}
}

// The loop runs once at start and stops with its context.
func TestLeaveCarryOverLoopRunsAtStart(t *testing.T) {
	h := newHarness(t)
	h.carrySetup(false, nil, 3, 31)
	logged := make(chan string, 8)
	h.svc.log = slog.New(logSignal{ch: logged})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		h.svc.LeaveCarryOverLoop(ctx, time.Hour)
		close(done)
	}()
	select {
	case msg := <-logged:
		if msg != "hris leave carry-over" {
			t.Fatalf("log = %q", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("loop did not run at start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("loop did not stop with its context")
	}
	// fixedNow is 2026-10-05 WIB.
	if h.scalar(`SELECT count(*)::text FROM hris.leave_carry_over_runs WHERE year = 2026`) != "1" {
		t.Fatal("start run left no claim")
	}
}

// app.Routes() builds the module without a database: no loop there.
func TestNewWithoutDBStartsNoCarryOverLoop(t *testing.T) {
	logged := make(chan string, 8)
	deps := module.Deps{Log: slog.New(logSignal{ch: logged}), Now: func() time.Time { return fixedNow },
		Config: config.Config{AppEnv: "development"}}
	New(deps, Ports{})
	select {
	case msg := <-logged:
		t.Fatalf("a loop ran without a database: %q", msg)
	case <-time.After(300 * time.Millisecond):
	}
}

// Flag menyala langsung di DB tanpa migrasi tindak lanjut: job tetap tidak
// membuat sisa bawaan (hanya baris tahun baru).
func TestLeaveCarryOverLockedIgnoresEnabled(t *testing.T) {
	h := newHarness(t)
	h.carrySetup(true, nil, 3, 31)
	h.exec(`DROP TABLE IF EXISTS hris.leave_carry_over_reviews`)
	a := h.employee(emp{name: "Bawaan Terkunci", join: "2024-01-10"})
	h.setBalance(a, 2026, 12, 0, 4, 0, 0, nil)

	if err := h.svc.RunLeaveCarryOver(context.Background(), jan1(2027)); err != nil {
		t.Fatal(err)
	}
	if got := h.carryOf(a, 2027); got != "0.00|0.00|-" {
		t.Fatalf("2027 carry while locked = %q, want none", got)
	}
	if h.carryAudits(a) != "0" {
		t.Fatal("audit written while carry-over is locked")
	}
}

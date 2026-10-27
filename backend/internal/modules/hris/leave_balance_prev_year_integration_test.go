package hris

import (
	"strings"
	"testing"
	"time"
)

// EPIC-068 I3 (user 27-10-2026): HR may correct last year's balance after
// 1 January until this year's carry-over expiry date (setting, default
// 31 March); the correction recalculates this year's automatic carry-over.

// setNow moves the HRIS service clock to a WIB "YYYY-MM-DD HH:MM".
func (h *harness) setNow(wib string) {
	h.t.Helper()
	at, err := time.ParseInLocation("2006-01-02 15:04", wib, wibZone)
	if err != nil {
		h.t.Fatal(err)
	}
	h.svc.now = func() time.Time { return at }
}

func TestPutPreviousYearUntilCarryExpiry(t *testing.T) {
	h := newHarness(t)
	f := h.carryFixture(nil) // 2026: total 12, used 4; 2027 auto carry 8 until 2027-03-31
	editor, _ := h.saldoStaff("pos", false, "read", "update")
	correct := func(adjustment float64) call {
		return h.putSaldo(&editor, f.id, map[string]any{"year": 2026, "annual_leave_adjustment": adjustment, "reason": "Koreksi cuti Desember"})
	}

	h.setNow("2027-02-10 10:00")
	ok := correct(-2)
	expect(t, ok, 200, "")
	if ok.data()["year"] != 2026.0 || ok.data()["annual_leave_remaining"] != 6.0 {
		t.Fatalf("PUT 2026 = %s", ok.raw)
	}
	if got := h.carryOf(f.id, 2027); got != "6.00|0.00|2027-03-31" {
		t.Fatalf("carry 2027 after correcting 2026 = %q, want 6", got)
	}
	if got := h.carryAudit(f.id, "hr_update"); !strings.HasPrefix(got, "hr_update|-|Koreksi HR|true|") || !strings.Contains(got, "|8|6|") {
		t.Fatalf("hr audit = %q", got)
	}
	// Two years back stays closed; the message names the window.
	expect(t, h.putSaldo(&editor, f.id, putBody(2025, 12, 0, 0, nil, "Uji")), 400,
		"Tahun saldo hanya boleh 2026 (tahun lalu, sampai 31-03-2027), 2027 (tahun berjalan), atau 2028 (tahun depan)")

	// The expiry date itself is still open.
	h.setNow("2027-03-31 23:00")
	expect(t, correct(-1), 200, "")
	if got := h.carryOf(f.id, 2027); got != "7.00|0.00|2027-03-31" {
		t.Fatalf("carry 2027 on the expiry date = %q, want 7", got)
	}

	// The day after, last year is closed again.
	h.setNow("2027-04-01 00:30")
	expect(t, correct(0), 400, "Tahun saldo hanya boleh 2027 (tahun berjalan) atau 2028 (tahun depan)")
	if got := h.carryOf(f.id, 2027); got != "7.00|0.00|2027-03-31" {
		t.Fatalf("a refused PUT changed the carry: %q", got)
	}

	// The window follows the expiry date in the setting.
	h.exec(`UPDATE hris.leave_settings SET carry_over_expiry_month = 4, carry_over_expiry_day = 30`)
	expect(t, correct(0), 200, "")
}

func TestImportPreviousYearUntilCarryExpiry(t *testing.T) {
	h := newHarness(t)
	f := h.saldoImportFixture() // Ana 2026: total 12, used 5
	h.carrySetup(true, nil, 3, 31)
	h.setBalance(f.ana, 2027, 12, 0, 0, 0, 0, nil)
	h.setNow("2027-02-10 10:00")

	_, x := h.saldoTemplate(&f.hr, "?year=2026&branch_id="+f.branch)
	ar := rowOf(t, x, f.anaNIP)
	setCell(t, x, colTotal, ar, 14)
	setCell(t, x, colReason, ar, "Koreksi jatah 2026")
	file := fileBytes(t, x)

	pre := h.importStep(t, f, "preview", nil, file)
	expect(t, pre, 200, "")
	if m := issueMessages(pre, "errors"); m != "" {
		t.Fatalf("errors in the window = %s", m)
	}
	expect(t, h.importStep(t, f, "commit", map[string]string{"fingerprint": pre.data()["fingerprint"].(string)}, file), 200, "")
	if got := h.scalar(`SELECT annual_leave_total::text FROM hris.leave_balances WHERE employee_id = $1 AND year = 2026`, f.ana); got != "14.00" {
		t.Fatalf("2026 total = %s", got)
	}
	if got := h.carryOf(f.ana, 2027); got != "9.00|0.00|2027-03-31" {
		t.Fatalf("carry 2027 after import = %q, want 9", got)
	}

	h.setNow("2027-04-01 08:00")
	late := h.importStep(t, f, "preview", nil, file)
	if m := issueMessages(late, "errors"); !strings.Contains(m, "Tahun saldo hanya boleh 2027 (tahun berjalan) atau 2028 (tahun depan)") {
		t.Fatalf("errors after the window = %s", m)
	}
}

package hris

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

// EPIC-068 Task 4: GET/PUT /api/hris/leave-settings (R8).

func settingsBody(enabled any, max any, month, day any) map[string]any {
	return map[string]any{"carry_over_enabled": enabled, "carry_over_max_days": max,
		"carry_over_expiry_month": month, "carry_over_expiry_day": day, "reason": "Penyesuaian kebijakan cuti"}
}

func TestLeaveSettingsGet(t *testing.T) {
	h := newHarness(t)
	h.exec(`UPDATE hris.leave_settings SET carry_over_enabled = false, carry_over_max_days = NULL,
		carry_over_expiry_month = 3, carry_over_expiry_day = 31, updated_by_name = NULL`)
	reader, _ := h.saldoStaff("pos", true, "read")
	none, _ := h.saldoStaff("pos", true)
	oldHRD, _ := h.staff("hrd", []string{menuWorkforce}, true)

	expect(t, h.do(nil, "GET", "/api/hris/leave-settings", nil), 401, "")
	expect(t, h.do(&none, "GET", "/api/hris/leave-settings", nil), 403, "Insufficient permissions")
	expect(t, h.do(&oldHRD, "GET", "/api/hris/leave-settings", nil), 403, "Insufficient permissions")
	c := h.do(&reader, "GET", "/api/hris/leave-settings", nil)
	expect(t, c, 200, "")
	data, _ := c.body["data"].(map[string]any)
	if data["carry_over_enabled"] != false || data["carry_over_max_days"] != nil ||
		data["carry_over_expiry_month"] != float64(3) || data["carry_over_expiry_day"] != float64(31) {
		t.Fatalf("settings = %s", c.raw)
	}
}

func TestLeaveSettingsPut(t *testing.T) {
	h := newHarness(t)
	h.unlockCarryOver()
	h.exec(`UPDATE hris.leave_settings SET carry_over_enabled = false, carry_over_max_days = NULL,
		carry_over_expiry_month = 3, carry_over_expiry_day = 31`)
	viewer, _ := h.saldoStaff("pos", true, "read")
	editor, _ := h.saldoStaff("pos", true, "read", "update")
	path := "/api/hris/leave-settings"

	expect(t, h.do(nil, "PUT", path, settingsBody(true, 5, 6, 30)), 401, "")
	expect(t, h.do(&viewer, "PUT", path, settingsBody(true, 5, 6, 30)), 403, "Insufficient permissions")

	bad := []struct {
		name string
		body map[string]any
		msg  string
	}{
		{"29 Feb", settingsBody(true, 5, 2, 29), "Tanggal kedaluwarsa tidak valid — 29 Februari tidak diperbolehkan karena tidak ada di setiap tahun"},
		{"31 Apr", settingsBody(true, 5, 4, 31), "Tanggal kedaluwarsa tidak valid"},
		{"month 13", settingsBody(true, 5, 13, 1), "Bulan kedaluwarsa harus 1–12"},
		{"month 0", settingsBody(true, 5, 0, 1), "Bulan kedaluwarsa harus 1–12"},
		{"day 0", settingsBody(true, 5, 3, 0), "Tanggal kedaluwarsa harus 1–31"},
		{"max quarter", settingsBody(true, 5.25, 3, 31), "Maksimal sisa bawaan harus kelipatan 0,5"},
		{"max negative", settingsBody(true, -1, 3, 31), "Maksimal sisa bawaan tidak boleh negatif"},
		{"max huge", settingsBody(true, 400, 3, 31), "Maksimal sisa bawaan tidak boleh lebih dari 366 hari"},
	}
	for _, b := range bad {
		c := h.do(&editor, "PUT", path, b.body)
		if c.status != 400 || (b.msg != "" && c.body["error"] != b.msg) {
			t.Fatalf("%s: status %d body %s, want 400 %q", b.name, c.status, c.raw, b.msg)
		}
	}
	if h.scalar(`SELECT concat_ws('|', carry_over_enabled::text, carry_over_max_days, carry_over_expiry_month, carry_over_expiry_day)
		FROM hris.leave_settings`) != "false|3|31" {
		t.Fatal("a refused PUT changed the settings")
	}
	if h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'leave_settings.update' AND actor_id = $1`, editor.UserID) != "0" {
		t.Fatal("a refused PUT wrote an audit")
	}

	c := h.do(&editor, "PUT", path, settingsBody(true, 5.5, 6, 30))
	expect(t, c, 200, "")
	if c.body["message"] != "Pengaturan sisa cuti tersimpan" {
		t.Fatalf("message = %v", c.body["message"])
	}
	data, _ := c.body["data"].(map[string]any)
	if data["carry_over_enabled"] != true || data["carry_over_max_days"] != 5.5 ||
		data["carry_over_expiry_month"] != float64(6) || data["carry_over_expiry_day"] != float64(30) || data["updated_by_name"] == nil {
		t.Fatalf("data = %s", c.raw)
	}
	if got := h.scalar(`SELECT concat_ws('|', carry_over_enabled::text, carry_over_max_days, carry_over_expiry_month, carry_over_expiry_day, (updated_by = $1)::text)
		FROM hris.leave_settings`, editor.UserID); got != "true|5.50|6|30|true" {
		t.Fatalf("stored = %q", got)
	}
	audit := h.scalar(`SELECT concat_ws('|', entity, entity_label, before->>'carry_over_enabled', after->>'carry_over_enabled',
		before->>'carry_over_max_days', after->>'carry_over_max_days', before->>'carry_over_expiry_month', after->>'carry_over_expiry_month')
		FROM audit.audit_log WHERE action = 'leave_settings.update' AND actor_id = $1`, editor.UserID)
	if want := "hris.leave_settings|Pengaturan sisa cuti bawaan|false|true|5.5|3|6"; audit != want {
		t.Fatalf("audit = %q, want %q", audit, want)
	}

	// null max = carry everything; omitted fields keep their value.
	expect(t, h.do(&editor, "PUT", path, map[string]any{"carry_over_max_days": nil, "reason": "Tanpa batas"}), 200, "")
	if got := h.scalar(`SELECT concat_ws('|', carry_over_enabled::text, coalesce(carry_over_max_days::text, 'NULL'), carry_over_expiry_month, carry_over_expiry_day)
		FROM hris.leave_settings`); got != "true|NULL|6|30" {
		t.Fatalf("after null max = %q", got)
	}
	// The expiry day alone must still form a valid date with the stored month.
	expect(t, h.do(&editor, "PUT", path, map[string]any{"carry_over_expiry_month": 2, "reason": "Uji"}), 400, "")
	expect(t, h.do(&editor, "PUT", path, map[string]any{"carry_over_expiry_month": 2, "carry_over_expiry_day": 28, "reason": "Uji"}), 200, "")
	expect(t, h.do(&editor, "PUT", path, settingsBody(false, nil, 12, 31)), 200, "")
	if got := h.scalar(`SELECT concat_ws('|', carry_over_enabled::text, carry_over_expiry_month, carry_over_expiry_day) FROM hris.leave_settings`); got != "false|12|31" {
		t.Fatalf("final = %q", got)
	}
}

// Sisa bawaan terkunci sampai migrasi tindak lanjut (hitung ulang) membuat
// hris.leave_carry_over_reviews: menyalakan ditolak, setting lain tetap bisa disimpan.
func TestLeaveSettingsCarryOverLocked(t *testing.T) {
	h := newHarness(t)
	h.exec(`DROP TABLE IF EXISTS hris.leave_carry_over_reviews`)
	h.exec(`UPDATE hris.leave_settings SET carry_over_enabled = false, carry_over_max_days = NULL,
		carry_over_expiry_month = 3, carry_over_expiry_day = 31`)
	editor, _ := h.saldoStaff("pos", true, "read", "update")
	path := "/api/hris/leave-settings"

	c := h.do(&editor, "GET", path, nil)
	expect(t, c, 200, "")
	if data, _ := c.body["data"].(map[string]any); data["carry_over_ready"] != false {
		t.Fatalf("carry_over_ready = %v, want false", data["carry_over_ready"])
	}
	expect(t, h.do(&editor, "PUT", path, settingsBody(true, nil, 3, 31)), 400, carryOverLockedMsg)
	if h.scalar(`SELECT carry_over_enabled::text FROM hris.leave_settings`) != "false" {
		t.Fatal("refused enable changed the setting")
	}
	expect(t, h.do(&editor, "PUT", path, settingsBody(false, 6, 4, 30)), 200, "")

	h.unlockCarryOver()
	c = h.do(&editor, "GET", path, nil)
	if data, _ := c.body["data"].(map[string]any); data["carry_over_ready"] != true {
		t.Fatalf("carry_over_ready after unlock = %v, want true", data["carry_over_ready"])
	}
	expect(t, h.do(&editor, "PUT", path, settingsBody(true, nil, 3, 31)), 200, "")
}

/* ── EPIC-068 tindak lanjut Task 2: alasan wajib, pratinjau, hitung ulang tahun berjalan ── */

// settingsFixture is the setting off, one active employee (the only one the
// settings recalculation can touch) with 2025 remaining 8 (12 - 4) and an
// auto 2026 row without carry; fixedNow makes 2026 the current year.
func (h *harness) settingsFixture() (editor testutil.Staff, id string) {
	h.t.Helper()
	h.carrySetup(false, nil, 3, 31)
	editor, _ = h.saldoStaff("pos", true, "read", "update")
	f := h.leaveFixture("2024-01-10")
	h.exec(`UPDATE hris.employees SET is_active = false WHERE id <> $1`, f.id)
	h.setBalance(f.id, 2025, 12, 0, 4, 0, 0, nil)
	h.setBalance(f.id, 2026, 12, 0, 0, 0, 0, nil)
	return editor, f.id
}

// openReviews maps employee id to "days|used" of the open review of year.
func (h *harness) openReviews(year int) map[string]string {
	h.t.Helper()
	rows, err := h.tx.Query(context.Background(), `SELECT employee_id::text, carry_over_days::text || '|' || carry_over_used::text
		FROM hris.leave_carry_over_reviews WHERE year = $1 AND resolved_at IS NULL`, year)
	if err != nil {
		h.t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, v string
		if err := rows.Scan(&id, &v); err != nil {
			h.t.Fatal(err)
		}
		out[id] = v
	}
	return out
}

// settingsImpactOf is what a saved settings recalculation really did, read
// from the audit trail and from the open reviews before (a snapshot taken ahead
// of the save) and after: the same four figures a preview returns.
func (h *harness) settingsImpactOf(year int, reviewsBefore map[string]string) map[string]any {
	h.t.Helper()
	row := h.scalar(`SELECT concat_ws('|', count(*), coalesce(sum(greatest((after->>'carry_over_days')::numeric - (before->>'carry_over_days')::numeric, 0)), 0),
			coalesce(sum(greatest((before->>'carry_over_days')::numeric - (after->>'carry_over_days')::numeric, 0)), 0))
		FROM audit.audit_log WHERE action = 'leave_balance.carry_over' AND after->'cause'->>'type' = 'settings'
		  AND (after->>'year')::int = $1`, year)
	var n int
	var up, down float64
	if _, err := fmt.Sscanf(strings.ReplaceAll(row, "|", " "), "%d %g %g", &n, &up, &down); err != nil {
		h.t.Fatalf("impact %q: %v", row, err)
	}
	flagged := 0
	for id, v := range h.openReviews(year) {
		if was, open := reviewsBefore[id]; !open || was != v {
			flagged++
		}
	}
	return map[string]any{"employees_changed": float64(n), "days_increase": up, "days_decrease": down, "needs_review": float64(flagged)}
}

func sameImpact(a, b map[string]any) bool {
	for _, k := range []string{"employees_changed", "days_increase", "days_decrease", "needs_review"} {
		if a[k] != b[k] {
			return false
		}
	}
	return true
}

// A max of 6 cuts the 8 remaining days of 2025 to a carry of 6 in 2026 (the
// current year); the preview returns exactly what the save then does and
// writes nothing; the previous year is untouched; clearing the max brings the
// whole remainder back; a manual row is never touched.
func TestLeaveSettingsRecalcCurrentYear(t *testing.T) {
	h := newHarness(t)
	editor, id := h.settingsFixture()
	manual := h.leaveFixture("2024-02-10")
	h.exec(`UPDATE hris.employees SET is_active = false WHERE id NOT IN ($1, $2)`, id, manual.id)
	h.setBalance(manual.id, 2025, 12, 0, 0, 0, 0, nil)
	h.setBalance(manual.id, 2026, 12, 0, 0, 2, 0, "2026-06-30") // HR filled: manual
	path := "/api/hris/leave-settings"
	body := settingsBody(true, 6, 3, 31)

	// Preview: the figures, and nothing written.
	delete(body, "reason")
	c := h.do(&editor, "POST", path+"/preview", body)
	expect(t, c, 200, "")
	pre := c.data()
	if pre["employees_changed"] != float64(1) || pre["days_increase"] != float64(6) || pre["days_decrease"] != float64(0) || pre["needs_review"] != float64(0) {
		t.Fatalf("preview = %s", c.raw)
	}
	if h.scalar(`SELECT carry_over_enabled::text FROM hris.leave_settings`) != "false" || h.carryOf(id, 2026) != "0.00|0.00|-" ||
		h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action IN ('leave_settings.update', 'leave_balance.carry_over')
			AND (entity_id = $1 OR actor_id = $2)`, id, editor.UserID) != "0" {
		t.Fatal("the preview wrote something")
	}

	// Save: carry 6, expiry of the setting in 2026, 2025 untouched, manual kept.
	body["reason"] = "Kebijakan baru 2026"
	reviewsBefore := h.openReviews(2026)
	expect(t, h.do(&editor, "PUT", path, body), 200, "")
	if got := h.carryOf(id, 2026); got != "6.00|0.00|2026-03-31" {
		t.Fatalf("2026 carry = %q, want 6", got)
	}
	if got := h.carryOf(id, 2025); got != "0.00|0.00|-" {
		t.Fatalf("2025 changed: %q", got)
	}
	if got := h.carryOf(manual.id, 2026); got != "2.00|0.00|2026-06-30" || h.carrySource(manual.id, 2026) != "manual" {
		t.Fatalf("manual row changed: %q", got)
	}
	if got := h.carryAudit(id, "settings"); !strings.HasPrefix(got, "settings|-|Batas sisa bawaan diubah — Kebijakan baru 2026|true|") || !strings.Contains(got, "|0|6|") {
		t.Fatalf("employee audit = %q", got)
	}
	if got := h.scalar(`SELECT reason FROM audit.audit_log WHERE action = 'leave_settings.update' AND actor_id = $1`, editor.UserID); got != "Kebijakan baru 2026" {
		t.Fatalf("settings audit reason = %q", got)
	}
	if real := h.settingsImpactOf(2026, reviewsBefore); !sameImpact(pre, real) {
		t.Fatalf("preview %v != real %v", pre, real)
	}

	// Clearing the max: the whole 8 days return; the preview agrees again.
	body = settingsBody(true, nil, 3, 31)
	delete(body, "reason")
	c = h.do(&editor, "POST", path+"/preview", body)
	expect(t, c, 200, "")
	if d := c.data(); d["employees_changed"] != float64(1) || d["days_increase"] != float64(2) || d["days_decrease"] != float64(0) {
		t.Fatalf("preview 2 = %s", c.raw)
	}
	body["reason"] = "Kembali tanpa batas"
	expect(t, h.do(&editor, "PUT", path, body), 200, "")
	if got := h.carryOf(id, 2026); got != "8.00|0.00|2026-03-31" {
		t.Fatalf("2026 carry after clearing = %q, want 8", got)
	}

	// Switching off recalculates nothing and removes nothing.
	before := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'leave_balance.carry_over' AND entity_id = $1`, id)
	expect(t, h.do(&editor, "PUT", path, map[string]any{"carry_over_enabled": false, "reason": "Dimatikan"}), 200, "")
	if got := h.carryOf(id, 2026); got != "8.00|0.00|2026-03-31" {
		t.Fatalf("carry after switching off = %q", got)
	}
	// A new max while off, and an expiry date alone while on, recalculate nothing either.
	expect(t, h.do(&editor, "PUT", path, map[string]any{"carry_over_max_days": 1, "reason": "Batas saat mati"}), 200, "")
	expect(t, h.do(&editor, "PUT", path, map[string]any{"carry_over_enabled": true, "carry_over_max_days": nil, "carry_over_expiry_month": 6, "carry_over_expiry_day": 30, "reason": "Tanggal"}), 200, "")
	if got := h.carryOf(id, 2026); got != "8.00|0.00|2026-03-31" {
		t.Fatalf("carry after expiry-date change = %q (an existing row keeps its expiry)", got)
	}
	if h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'leave_balance.carry_over' AND entity_id = $1`, id) != before {
		t.Fatal("a change that moves no carry-over wrote a carry-over audit")
	}
}

// A max that drops the carry below what is already used keeps the value, opens
// one review and closes it again when the max is cleared. The preview counts
// only the employees this change newly flags: one whose review opens (A) or
// whose open review gets other figures (C), not one already under review whose
// figures stay the same (B).
func TestLeaveSettingsRecalcBelowUsedOpensReview(t *testing.T) {
	h := newHarness(t)
	editor, id := h.settingsFixture() // A: 2025 remaining 8
	h.carrySetup(true, nil, 3, 31)
	h.setBalance(id, 2026, 12, 0, 0, 8, 5, "2026-03-31")
	b := h.leaveFixture("2024-02-10")
	c := h.leaveFixture("2024-03-10")
	h.exec(`UPDATE hris.employees SET is_active = false WHERE id NOT IN ($1, $2, $3)`, id, b.id, c.id)
	h.setBalance(b.id, 2025, 12, 0, 9, 0, 0, nil) // remaining 3
	h.setBalance(b.id, 2026, 12, 0, 0, 3, 5, "2026-03-31")
	h.setBalance(c.id, 2025, 12, 0, 4, 0, 0, nil) // remaining 8
	h.setBalance(c.id, 2026, 12, 0, 0, 4, 6, "2026-03-31")
	h.exec(`UPDATE hris.leave_balances SET carry_over_source = 'auto' WHERE year = 2026 AND employee_id IN ($1, $2, $3)`, id, b.id, c.id)
	// B and C are already under review, with the figures their rows have now.
	h.exec(`INSERT INTO hris.leave_carry_over_reviews (employee_id, year, carry_over_days, carry_over_used, excess, cause)
		VALUES ($1, 2026, 3, 5, 2, '{"type":"hr_update"}'), ($2, 2026, 4, 6, 2, '{"type":"hr_update"}')`, b.id, c.id)
	path := "/api/hris/leave-settings"

	body := settingsBody(true, 3, 3, 31)
	delete(body, "reason")
	res := h.do(&editor, "POST", path+"/preview", body)
	expect(t, res, 200, "")
	pre := res.data()
	// A: 8 -> 3 and C: 4 -> 3 change (-5, -1); B stays 3. A is newly flagged,
	// C's review gets other figures, B's does not change: needs_review 2, not 3.
	if pre["employees_changed"] != float64(2) || pre["days_decrease"] != float64(6) || pre["days_increase"] != float64(0) || pre["needs_review"] != float64(2) {
		t.Fatalf("preview = %s", res.raw)
	}
	if h.reviews(id, 2026) != "0|0|-|-" || h.reviews(c.id, 2026) != "1|0|2.00|-" {
		t.Fatalf("the preview changed reviews: A %s, C %s", h.reviews(id, 2026), h.reviews(c.id, 2026))
	}

	body["reason"] = "Batas 3 hari"
	reviewsBefore := h.openReviews(2026)
	expect(t, h.do(&editor, "PUT", path, body), 200, "")
	if got := h.carryOf(id, 2026); got != "3.00|5.00|2026-03-31" {
		t.Fatalf("carry = %q, want 3 kept with 5 used", got)
	}
	if h.reviews(id, 2026) != "1|0|2.00|-" {
		t.Fatalf("reviews = %s, want one open with excess 2", h.reviews(id, 2026))
	}
	if h.reviews(b.id, 2026) != "1|0|2.00|-" || h.reviews(c.id, 2026) != "1|0|3.00|-" {
		t.Fatalf("B %s (unchanged), C %s (excess 3)", h.reviews(b.id, 2026), h.reviews(c.id, 2026))
	}
	if real := h.settingsImpactOf(2026, reviewsBefore); !sameImpact(pre, real) {
		t.Fatalf("preview %v != real %v", pre, real)
	}

	// Cleared max: all three get their whole remainder back and the reviews close.
	body = settingsBody(true, nil, 3, 31)
	delete(body, "reason")
	res = h.do(&editor, "POST", path+"/preview", body)
	expect(t, res, 200, "")
	if res.data()["needs_review"] != float64(0) {
		t.Fatalf("preview of clearing = %s, want needs_review 0", res.raw)
	}
	body["reason"] = "Tanpa batas"
	expect(t, h.do(&editor, "PUT", path, body), 200, "")
	if got := h.carryOf(id, 2026); got != "8.00|5.00|2026-03-31" {
		t.Fatalf("carry after clearing = %q", got)
	}
	if h.reviews(id, 2026) != "0|1|-|otomatis" {
		t.Fatalf("reviews after clearing = %s", h.reviews(id, 2026))
	}
}

func TestLeaveSettingsReasonAndPermission(t *testing.T) {
	h := newHarness(t)
	editor, id := h.settingsFixture()
	viewer, _ := h.saldoStaff("pos", true, "read")
	path := "/api/hris/leave-settings"
	body := settingsBody(true, 6, 3, 31)

	// 403 without the update action, for the save and the preview.
	expect(t, h.do(&viewer, "PUT", path, body), 403, "Insufficient permissions")
	expect(t, h.do(&viewer, "POST", path+"/preview", body), 403, "Insufficient permissions")
	expect(t, h.do(nil, "POST", path+"/preview", body), 401, "")

	// The save needs a reason; the preview does not.
	for _, r := range []any{nil, "", "   "} {
		b := settingsBody(true, 6, 3, 31)
		if r == nil {
			delete(b, "reason")
		} else {
			b["reason"] = r
		}
		expect(t, h.do(&editor, "PUT", path, b), 400, "Alasan perubahan wajib diisi")
	}
	long := settingsBody(true, 6, 3, 31)
	long["reason"] = strings.Repeat("a", 501)
	expect(t, h.do(&editor, "PUT", path, long), 400, "Alasan terlalu panjang (maksimal 500 karakter)")
	if h.scalar(`SELECT carry_over_enabled::text FROM hris.leave_settings`) != "false" || h.carryOf(id, 2026) != "0.00|0.00|-" {
		t.Fatal("a refused PUT changed something")
	}
	delete(body, "reason")
	expect(t, h.do(&editor, "POST", path+"/preview", body), 200, "")

	// The preview validates like the save.
	bad := settingsBody(true, 5.25, 3, 31)
	delete(bad, "reason")
	expect(t, h.do(&editor, "POST", path+"/preview", bad), 400, "Maksimal sisa bawaan harus kelipatan 0,5")
}

package hris

import (
	"testing"
)

// EPIC-068 Task 4: GET/PUT /api/hris/leave-settings (R8).

func settingsBody(enabled any, max any, month, day any) map[string]any {
	return map[string]any{"carry_over_enabled": enabled, "carry_over_max_days": max,
		"carry_over_expiry_month": month, "carry_over_expiry_day": day}
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
	expect(t, h.do(&editor, "PUT", path, map[string]any{"carry_over_max_days": nil}), 200, "")
	if got := h.scalar(`SELECT concat_ws('|', carry_over_enabled::text, coalesce(carry_over_max_days::text, 'NULL'), carry_over_expiry_month, carry_over_expiry_day)
		FROM hris.leave_settings`); got != "true|NULL|6|30" {
		t.Fatalf("after null max = %q", got)
	}
	// The expiry day alone must still form a valid date with the stored month.
	expect(t, h.do(&editor, "PUT", path, map[string]any{"carry_over_expiry_month": 2}), 400, "")
	expect(t, h.do(&editor, "PUT", path, map[string]any{"carry_over_expiry_month": 2, "carry_over_expiry_day": 28}), 200, "")
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

package hris

import (
	"strings"
	"testing"
)

func TestOvertimeManagerCorrectsTimesOnApprove(t *testing.T) {
	h, _ := newFileHarness(t)
	hr, _ := h.staff("hrd", []string{menuKepegawaian, "hris.kepegawaian.overtime"}, true)
	boss, bossID := h.staff("pos", nil, true, emp{name: "Atasan Koreksi"})
	staff, _ := h.staff("pos", nil, true, emp{name: "Crew Dikoreksi", boss: &bossID})
	lone, _ := h.staff("pos", nil, true, emp{name: "Crew Tanpa Atasan"})

	created := h.do(&staff, "POST", "/api/hris/overtime", h.otSelfBody(&staff, "2026-10-01", "17:00", "21:00"))
	expect(t, created, 200, "")
	id := created.data()["id"].(string)
	decide := map[string]any{"overtime_id": id, "action": "approve", "end_time": "20:00", "break_minutes": 30}

	expect(t, h.do(&hr, "POST", "/api/hris/overtime/decide", decide), 403, "Koreksi jam lembur hanya oleh atasan langsung")
	expect(t, h.do(&hr, "PATCH", "/api/hris/overtime/"+id, map[string]any{"end_time": "20:00", "reason": "Sesuai absensi"}),
		403, "Koreksi jam lembur hanya oleh atasan langsung")
	bad := map[string]any{"overtime_id": id, "action": "approve", "end_time": "17:30"}
	expect(t, h.do(&boss, "POST", "/api/hris/overtime/decide", bad), 400, "Durasi lembur minimal 1 jam")

	ok := h.do(&boss, "POST", "/api/hris/overtime/decide", decide)
	expect(t, ok, 200, "")
	if d := ok.data(); d["status"] != "approved" || d["hours"] != "2.50" || d["break_minutes"] != 30.0 ||
		d["end_time"] != "20:00:00" || d["corrected_by_name"] != boss.FullName {
		t.Fatalf("approve + koreksi = %s", ok.raw)
	}
	if before := h.scalar(`SELECT before->>'end_time' FROM audit.audit_log
		WHERE entity_id = $1 AND action = 'overtime.approve'`, id); before != "21:00" {
		t.Fatalf("audit before end_time = %q", before)
	}
	// before.hours numerik seperti after.hours (bukan string NUMERIC).
	if typ := h.scalar(`SELECT jsonb_typeof(before->'hours') FROM audit.audit_log
		WHERE entity_id = $1 AND action = 'overtime.approve'`, id); typ != "number" {
		t.Fatalf("audit before hours type = %q", typ)
	}

	// Karyawan tanpa atasan: HR boleh mengoreksi.
	solo := h.do(&lone, "POST", "/api/hris/overtime", h.otSelfBody(&lone, "2026-10-02", "17:00", "20:00"))
	expect(t, solo, 200, "")
	byHR := h.do(&hr, "POST", "/api/hris/overtime/decide",
		map[string]any{"overtime_id": solo.data()["id"], "action": "approve", "break_minutes": 60})
	expect(t, byHR, 200, "")
	if byHR.data()["hours"] != "2.00" {
		t.Fatalf("HR koreksi = %s", byHR.raw)
	}
}

// Atasan nonaktif = tanpa atasan (EPIC-063 R9): HR boleh mengoreksi.
func TestOvertimeHRCorrectsWhenManagerInactive(t *testing.T) {
	h, _ := newFileHarness(t)
	hr, _ := h.staff("hrd", []string{menuKepegawaian, "hris.kepegawaian.overtime"}, true)
	_, bossID := h.staff("pos", nil, true, emp{name: "Atasan Nonaktif"})
	staff, staffID := h.staff("pos", nil, true, emp{name: "Crew Atasan Nonaktif", boss: &bossID})

	first := h.do(&staff, "POST", "/api/hris/overtime", h.otSelfBody(&staff, "2026-10-01", "17:00", "21:00"))
	expect(t, first, 200, "")
	second := h.do(&staff, "POST", "/api/hris/overtime", h.otSelfBody(&staff, "2026-10-02", "17:00", "21:00"))
	expect(t, second, 200, "")
	h.exec(`UPDATE hris.employees SET is_active = false WHERE id = $1`, bossID)

	byHR := h.do(&hr, "POST", "/api/hris/overtime/decide",
		map[string]any{"overtime_id": first.data()["id"], "action": "approve", "end_time": "20:00"})
	expect(t, byHR, 200, "")
	if d := byHR.data(); d["hours"] != "3.00" || d["decided_as"] != "hr" {
		t.Fatalf("HR koreksi saat atasan nonaktif = %s", byHR.raw)
	}
	patched := h.do(&hr, "PATCH", "/api/hris/overtime/"+second.data()["id"].(string),
		map[string]any{"end_time": "20:00", "reason": "Sesuai absensi"})
	expect(t, patched, 200, "")
	if patched.data()["hours"] != "3.00" {
		t.Fatalf("PATCH saat atasan nonaktif = %s", patched.raw)
	}
	list := h.do(&hr, "GET", "/api/hris/overtime?employee_id="+staffID, nil)
	expect(t, list, 200, "")
	if !strings.Contains(list.raw, `"reporting_to_active":false`) {
		t.Fatalf("daftar lembur tanpa reporting_to_active = %s", list.raw)
	}
}

// HR (cadangan, karyawan tanpa atasan) mengoreksi istirahat lewat PATCH.
func TestOvertimeCorrectionBreakMinutes(t *testing.T) {
	h, _ := newFileHarness(t)
	hr, _ := h.staff("hrd", []string{menuKepegawaian, "hris.kepegawaian.overtime"}, true)
	lone, _ := h.staff("pos", nil, true, emp{name: "Crew Istirahat"})

	created := h.do(&lone, "POST", "/api/hris/overtime", h.otSelfBody(&lone, "2026-10-01", "17:00", "21:00"))
	expect(t, created, 200, "")
	id := created.data()["id"].(string)
	url := "/api/hris/overtime/" + id

	expect(t, h.do(&hr, "PATCH", url, map[string]any{"break_minutes": 0, "reason": "Sesuai absensi"}), 400, "Tidak ada perubahan")
	expect(t, h.do(&hr, "PATCH", url, map[string]any{"break_minutes": 721, "reason": "Sesuai absensi"}), 400,
		"Too big: expected number to be <=720")
	expect(t, h.do(&hr, "PATCH", url, map[string]any{"break_minutes": 200, "reason": "Sesuai absensi"}), 400,
		"Durasi lembur minimal 1 jam")
	ok := h.do(&hr, "PATCH", url, map[string]any{"break_minutes": 60, "reason": "Sesuai absensi"})
	expect(t, ok, 200, "")
	if d := ok.data(); d["hours"] != "3.00" || d["break_minutes"] != 60.0 || d["status"] != "pending" {
		t.Fatalf("koreksi istirahat = %s", ok.raw)
	}
	got := h.scalar(`SELECT (before->>'break_minutes') || '>' || (after->>'break_minutes') FROM audit.audit_log
		WHERE entity_id = $1 AND action = 'overtime.correct'`, id)
	if got != "0>60" {
		t.Fatalf("audit break_minutes = %q", got)
	}
}

package hris

import "testing"

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

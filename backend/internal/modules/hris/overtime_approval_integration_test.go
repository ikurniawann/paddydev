package hris

import (
	"context"
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

func otBody(date, start, end string) map[string]any {
	return map[string]any{"date": date, "start_time": start, "end_time": end, "reason": "Closing stok bulanan"}
}

// K-13 (EPIC-063 R5): jenis dari ESS dicek ke jadwal dan hari libur.
func TestOvertimeDayTypeFollowsSchedule(t *testing.T) {
	h, _ := newFileHarness(t)
	staff, own := h.staff("pos", nil, true)
	free, freeID := h.staff("pos", nil, true) // tanpa jadwal
	shift := h.scalar(`INSERT INTO hris.shifts (name, start_time, end_time) VALUES ('Pagi Lembur', '08:00', '16:00') RETURNING id::text`)
	h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from) VALUES ($1, 1, $2, '2026-01-01')`, own, shift)
	h.exec(`INSERT INTO hris.public_holidays (holiday_date, name, type) VALUES ('2026-10-19', 'Libur Uji Lembur', 'nasional')`)

	hint := func(s *testutil.Staff, date string) map[string]any {
		t.Helper()
		c := h.do(s, "GET", "/api/hris/overtime/day-type?date="+date, nil)
		expect(t, c, 200, "")
		return c.data()
	}
	if d := hint(&staff, "2026-10-12"); d["day_type"] != "regular" || d["label"] != "Weekday" {
		t.Fatalf("Senin ber-shift = %v", d)
	}
	if d := hint(&staff, "2026-10-11"); d["day_type"] != "rest_day" || d["label"] != "Weekend / Libur" {
		t.Fatalf("Minggu = %v", d)
	}
	if d := hint(&staff, "2026-10-19"); d["day_type"] != "rest_day" {
		t.Fatalf("libur nasional = %v", d)
	}
	if d := hint(&free, "2026-10-12"); d["day_type"] != nil || d["label"] != nil {
		t.Fatalf("tanpa jadwal = %v", d)
	}
	expect(t, h.do(&staff, "GET", "/api/hris/overtime/day-type?date=12-10-2026", nil), 400, "Tanggal tidak valid")
	expect(t, h.do(&staff, "GET", "/api/hris/overtime/day-type?date=2026-10-12&employee_id="+freeID, nil), 403,
		"Insufficient permissions")

	wrong := h.otSelfBody(&staff, "2026-10-05", "06:00", "08:00") // Senin ber-shift, sudah selesai
	wrong["day_type"] = "rest_day"
	expect(t, h.do(&staff, "POST", "/api/hris/overtime", wrong), 400, "Jenis lembur tanggal ini Weekday sesuai jadwal Anda")
	sunday := h.do(&staff, "POST", "/api/hris/overtime", h.otSelfBody(&staff, "2026-10-04", "09:00", "12:00"))
	expect(t, sunday, 200, "")
	if sunday.data()["day_type"] != "rest_day" {
		t.Fatalf("tanpa jenis memakai saran jadwal: %s", sunday.raw)
	}
	chosen := h.otSelfBody(&free, "2026-10-05", "06:00", "08:00")
	chosen["day_type"] = "rest_day"
	freeRow := h.do(&free, "POST", "/api/hris/overtime", chosen)
	expect(t, freeRow, 200, "")
	if freeRow.data()["day_type"] != "rest_day" {
		t.Fatalf("tanpa jadwal memakai pilihan karyawan: %s", freeRow.raw)
	}
}

// Q-APV-3 (R1) + WA atasan saat pengajuan lembur (R8).
func TestOvertimeDecisionRecordsCapacityAndNotifiesManager(t *testing.T) {
	h, _ := newFileHarness(t)
	hr, _ := h.staff("hrd", []string{menuKepegawaian}, true)
	boss, bossID := h.staff("pos", nil, true, emp{name: "Atasan Lembur", phone: "0812 3456 7000"})
	staff, own := h.staff("pos", nil, true, emp{name: "Crew Lembur", boss: &bossID})
	dispatch := func() {
		t.Helper()
		if _, err := h.deps.Events.Dispatch(context.Background(), h.tx); err != nil {
			t.Fatal(err)
		}
	}

	first := h.do(&staff, "POST", "/api/hris/overtime", h.otSelfBody(&staff, "2026-10-01", "17:00", "19:30"))
	expect(t, first, 200, "")
	dispatch()
	if len(h.wa.sent) != 1 || !strings.HasPrefix(h.wa.sent[0], "6281234567000|Paddy OS — Pengajuan Lembur") ||
		!strings.Contains(h.wa.sent[0], "Kepada: Atasan Lembur") ||
		!strings.Contains(h.wa.sent[0], "Kam, 01 Okt 2026, 17:00–19:30 (2,5 jam, Weekday)") ||
		!strings.Contains(h.wa.sent[0], "Area Karyawan → Persetujuan Tim") {
		t.Fatalf("whatsapp = %v", h.wa.sent)
	}
	// Penugasan HR tidak dikirim ke atasan.
	assign := otBody("2026-10-08", "17:00", "19:00")
	assign["employee_id"] = own
	expect(t, h.do(&hr, "POST", "/api/hris/overtime", assign), 200, "")
	dispatch()
	if len(h.wa.sent) != 1 {
		t.Fatalf("penugasan HR tidak boleh mengirim WA atasan: %v", h.wa.sent)
	}

	firstID := first.data()["id"].(string)
	byBoss := h.do(&boss, "POST", "/api/hris/overtime/decide", map[string]any{"overtime_id": firstID, "action": "approve"})
	expect(t, byBoss, 200, "")
	if d := byBoss.data(); d["decided_as"] != "manager" || d["decided_by_name"] == nil {
		t.Fatalf("manager approve = %s", byBoss.raw)
	}
	second := h.do(&staff, "POST", "/api/hris/overtime", h.otSelfBody(&staff, "2026-10-02", "17:00", "19:00"))
	expect(t, second, 200, "")
	secondID := second.data()["id"].(string)
	byHR := h.do(&hr, "POST", "/api/hris/overtime/decide", map[string]any{"overtime_id": secondID, "action": "approve"})
	expect(t, byHR, 200, "")
	if byHR.data()["decided_as"] != "hr" {
		t.Fatalf("HR approve = %s", byHR.raw)
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE entity = 'hris.overtime_requests'
		AND action = 'overtime.approve' AND entity_id IN ($1, $2)`, firstID, secondID); n != "2" {
		t.Fatalf("audit lembur = %s", n)
	}
}

// Ruling C1/P4: HR tidak memutus lembur miliknya sendiri; hiring_manager yang
// bukan atasan bukan cadangan HR untuk lembur.
func TestOvertimeSelfAndHiringManagerCannotDecide(t *testing.T) {
	h, _ := newFileHarness(t)
	hr, _ := h.staff("hrd", []string{menuKepegawaian}, true, emp{name: "HRD Lembur"})
	hm, _ := h.staff("hiring_manager", []string{menuWorkforce}, true, emp{name: "Manajer Rekrutmen"})
	_, bossID := h.staff("pos", nil, true, emp{name: "Atasan Lembur"})
	staff, _ := h.staff("pos", nil, true, emp{name: "Crew Lembur", boss: &bossID})

	mine := h.do(&hr, "POST", "/api/hris/overtime", h.otSelfBody(&hr, "2026-10-01", "17:00", "19:00"))
	expect(t, mine, 200, "")
	expect(t, h.do(&hr, "POST", "/api/hris/overtime/decide",
		map[string]any{"overtime_id": mine.data()["id"], "action": "approve"}), 403, "Tidak bisa memutuskan pengajuan lembur sendiri")

	req := h.do(&staff, "POST", "/api/hris/overtime", h.otSelfBody(&staff, "2026-10-01", "17:00", "19:00"))
	expect(t, req, 200, "")
	id := req.data()["id"].(string)
	expect(t, h.do(&hm, "POST", "/api/hris/overtime/decide", map[string]any{"overtime_id": id, "action": "approve"}), 403,
		"Hanya atasan langsung atau HRD yang bisa memproses pengajuan ini")
	if st := h.scalar(`SELECT status::text FROM hris.overtime_requests WHERE id = $1`, id); st != "pending" {
		t.Fatalf("status = %s", st)
	}
}

// P10: jam mulai diperiksa ketat HH:MM sebelum aturan "sebelum dimulai".
func TestOvertimeStartTimeStrictAndStaleDecision(t *testing.T) {
	h, _ := newFileHarness(t)
	hr, _ := h.staff("hrd", []string{menuKepegawaian}, true)
	boss, bossID := h.staff("pos", nil, true)
	staff, _ := h.staff("pos", nil, true, emp{boss: &bossID})

	bad := otBody("2026-10-04", "9:00", "11:00")
	expect(t, h.do(&staff, "POST", "/api/hris/overtime", bad), 400, "Validation failed")

	req := h.do(&staff, "POST", "/api/hris/overtime", h.otSelfBody(&staff, "2026-10-01", "17:00", "19:00"))
	expect(t, req, 200, "")
	id := req.data()["id"].(string)
	expect(t, h.do(&hr, "POST", "/api/hris/overtime/decide",
		map[string]any{"overtime_id": id, "action": "reject", "rejection_reason": "Tidak perlu"}), 200, "")
	// Antrean basi: atasan menekan Setujui setelah HR memutus.
	expect(t, h.do(&boss, "POST", "/api/hris/overtime/decide", map[string]any{"overtime_id": id, "action": "approve"}), 400,
		"Pengajuan sudah rejected")
}

// Final review: the HR day_type override applies to employee requests only,
// and a real change is audited (rest_day pays 2× vs 1.5×).
func TestOvertimeHRDayTypeOverrideScopeAndAudit(t *testing.T) {
	h, _ := newFileHarness(t)
	hr, hrID := h.staff("hrd", []string{menuKepegawaian}, true, emp{name: "HRD Penerima"})
	hr2, _ := h.staff("hrd", []string{menuKepegawaian}, true, emp{name: "HRD Penugas"})
	staff, _ := h.staff("pos", nil, true, emp{name: "Crew Jenis"})
	decide := func(s *testutil.Staff, id string, extra map[string]any) {
		t.Helper()
		body := map[string]any{"overtime_id": id, "action": "approve"}
		for k, v := range extra {
			body[k] = v
		}
		expect(t, h.do(s, "POST", "/api/hris/overtime/decide", body), 200, "")
	}
	diff := func(id, side string) string {
		t.Helper()
		return h.scalar(`SELECT COALESCE(`+side+`->>'day_type', '-') FROM audit.audit_log
			WHERE entity = 'hris.overtime_requests' AND action = 'overtime.approve' AND entity_id = $1`, id)
	}

	// (a) HRD confirming a company assignment for themself keeps the stored type.
	assign := otBody("2026-10-06", "17:00", "19:00")
	assign["employee_id"] = hrID
	assign["day_type"] = "regular"
	assigned := h.do(&hr2, "POST", "/api/hris/overtime", assign)
	expect(t, assigned, 200, "")
	selfID := assigned.data()["id"].(string)
	decide(&hr, selfID, map[string]any{"day_type": "rest_day"})
	if got := h.scalar(`SELECT day_type::text FROM hris.overtime_requests WHERE id = $1`, selfID); got != "regular" {
		t.Fatalf("HRD changed own assignment day_type to %s", got)
	}
	if got := h.scalar(`SELECT status::text FROM hris.overtime_requests WHERE id = $1`, selfID); got != "approved" {
		t.Fatalf("status = %s", got)
	}

	// (b) HR changes an employee request's type: stored + audited before/after.
	changed := h.do(&staff, "POST", "/api/hris/overtime", h.otSelfBody(&staff, "2026-10-01", "17:00", "19:00"))
	expect(t, changed, 200, "")
	changedID := changed.data()["id"].(string)
	decide(&hr, changedID, map[string]any{"day_type": "rest_day"})
	if got := h.scalar(`SELECT day_type::text FROM hris.overtime_requests WHERE id = $1`, changedID); got != "rest_day" {
		t.Fatalf("stored day_type = %s", got)
	}
	if b, a := diff(changedID, "before"), diff(changedID, "after"); b != "regular" || a != "rest_day" {
		t.Fatalf("audit day_type before=%s after=%s", b, a)
	}

	// (c) unchanged (or omitted) type leaves day_type out of the audit diff.
	same := h.do(&staff, "POST", "/api/hris/overtime", h.otSelfBody(&staff, "2026-10-02", "17:00", "19:00"))
	expect(t, same, 200, "")
	sameID := same.data()["id"].(string)
	decide(&hr, sameID, map[string]any{"day_type": "regular"})
	omitted := h.do(&staff, "POST", "/api/hris/overtime", h.otSelfBody(&staff, "2026-10-05", "06:00", "08:00"))
	expect(t, omitted, 200, "")
	omittedID := omitted.data()["id"].(string)
	decide(&hr, omittedID, nil)
	for _, id := range []string{sameID, omittedID} {
		if b, a := diff(id, "before"), diff(id, "after"); b != "-" || a != "-" {
			t.Fatalf("unchanged type leaked into audit: before=%s after=%s", b, a)
		}
	}
}

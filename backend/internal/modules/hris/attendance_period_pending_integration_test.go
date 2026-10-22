package hris

import (
	"fmt"
	"testing"
)

// EPIC-065 final review: pengajuan absensi yang masih menunggu muncul di
// statistik periode / ringkasan closing, supaya HR memutusnya sebelum periode
// ditutup (setelah closing pengajuan tidak bisa disetujui).
func TestAttendancePeriodPendingRequests(t *testing.T) {
	h := newHarness(t)
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 0, closing_offset_days = 3`)
	hr, _ := h.staff("hrd", []string{"hris.workforce.attendance"}, true)
	crew, crewID := h.staff("pos", nil, true, emp{name: "Crew Pengajuan Periode"})
	shift := h.scalar(`INSERT INTO hris.shifts (name, start_time, end_time, late_tolerance_minutes)
		VALUES ('Pagi Pengajuan Periode', '08:00', '17:00', 10) RETURNING id::text`)
	for dow := 1; dow <= 7; dow++ {
		h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from) VALUES ($1, $2, $3, '2026-01-01')`,
			crewID, dow, shift)
	}
	// A request left pending in September: the period auto-closes (fixedNow
	// is past its closing date) and the frozen summary must keep the count.
	h.exec(`INSERT INTO hris.attendance_requests (employee_id, date, kind, clock_in, clock_out, reason)
		VALUES ($1, '2026-09-10', 'manual', '08:00', '17:00', 'Pengajuan lama menunggu')`, crewID)

	// The shared test database may hold a September period closed earlier;
	// put it back to open past its closing date so listing auto-closes it now.
	h.exec(`UPDATE hris.attendance_periods SET status = 'open', closing_date = '2026-10-03', close_summary = NULL
		WHERE period_year = 2026 AND period_month = 9`)
	wantSep := h.scalar(`SELECT count(*)::text FROM hris.attendance_requests WHERE status = 'pending'
		AND date BETWEEN '2026-09-01' AND '2026-09-30'`)
	list := h.do(&hr, "GET", "/api/hris/attendance-periods?year=2026", nil)
	expect(t, list, 200, "")
	sep := periodByName(t, list, 2026, 9)
	summary := sep["close_summary"].(map[string]any)
	if sep["status"] != "closed" || sep["close_mode"] != "auto" || wantSep == "0" ||
		fmt.Sprint(summary["pending_requests"]) != wantSep || fmt.Sprint(sep["stats"].(map[string]any)["pending_requests"]) != wantSep {
		t.Fatalf("september = %v (want %s pending)", sep, wantSep)
	}
	pending := func() float64 {
		t.Helper()
		list := h.do(&hr, "GET", "/api/hris/attendance-periods?year=2026", nil)
		expect(t, list, 200, "")
		n, ok := periodByName(t, list, 2026, 10)["stats"].(map[string]any)["pending_requests"].(float64)
		if !ok {
			t.Fatalf("pending_requests missing in %s", list.raw)
		}
		return n
	}
	base := pending()

	filed := h.do(&crew, "POST", "/api/hris/attendance-requests", attReqBody("2026-10-02", "08:30", "17:00", "Internet toko mati pagi itu"))
	expect(t, filed, 201, "")
	if got := pending(); got != base+1 {
		t.Fatalf("october pending = %v, want %v", got, base+1)
	}
	oct := periodByName(t, h.do(&hr, "GET", "/api/hris/attendance-periods?year=2026", nil), 2026, 10)
	one := h.do(&hr, "GET", "/api/hris/attendance-periods/"+oct["id"].(string), nil)
	expect(t, one, 200, "")
	if one.data()["stats"].(map[string]any)["pending_requests"] != base+1 {
		t.Fatalf("period = %s", one.raw)
	}

	expect(t, h.do(&hr, "POST", "/api/hris/attendance-requests/"+filed.data()["id"].(string)+"/decide",
		map[string]any{"action": "reject", "rejection_reason": "Bukti tidak cukup"}), 200, "")
	if got := pending(); got != base {
		t.Fatalf("october after reject = %v, want %v", got, base)
	}
}

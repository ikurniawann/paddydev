package hris

import (
	"testing"
)

// EPIC-094: a late clock-in rings the bell of the employee, their approvers
// (request_approver + request_manager) and HR.

// lateShift schedules employee on date with a 08:00–16:00 shift, 5 minutes' tolerance.
func (h *harness) lateShift(employee, date string) {
	h.t.Helper()
	shift := h.scalar(`INSERT INTO hris.shifts (code, name, start_time, end_time, break_minutes, late_tolerance_minutes)
		VALUES ('LT94' || upper(substr(md5(random()::text), 1, 4)), 'Late 094', '08:00', '16:00', 60, 5) RETURNING id::text`)
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id)
		VALUES ($1, $2::date, $3, gen_random_uuid())`, employee, date, shift)
}

// notice is the bell row of user with title, as "type|link|message" ("" when none).
func (h *harness) notice(userID, title string) string {
	return h.scalar(`SELECT coalesce(string_agg(type || '|' || coalesce(link, '') || '|' || message, ';'), '')
		FROM public.notifications WHERE user_id = $1 AND title = $2`, userID, title)
}

func TestClockInLateNotifiesEmployeeApproversHR(t *testing.T) {
	h, _ := newFileHarness(t)
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 0, closing_offset_days = 3`)
	c := h.tieredChain()
	h.lateShift(c.crewID, "2026-10-06")
	h.setNow("2026-10-06 08:30")
	in := h.do(&c.crew, "POST", "/api/hris/attendance", clockInBody(dataURL("image/png", pngBytes(t)), nil, ""))
	expect(t, in, 200, "")
	if in.data()["is_late"] != true {
		t.Fatalf("clock-in not late: %s", in.raw)
	}
	msg := "Crew Toko terlambat 30 menit hari ini (1 kali di periode ini)."
	if got := h.notice(c.crew.UserID, "Anda terlambat hari ini"); got != "alert|/dashboard/me/absensi#keterlambatan|Anda terlambat 30 menit hari ini (1 kali di periode ini). Hati-hati, jika berlanjut akan ada teguran dari HR." {
		t.Errorf("employee bell = %q", got)
	}
	for name, s := range map[string]string{"SPV": c.spv.UserID, "Manager": c.mgr.UserID} {
		if got := h.notice(s, "Crew Toko terlambat hari ini"); got != "alert|/dashboard/me/absensi-tim|"+msg {
			t.Errorf("%s bell = %q", name, got)
		}
	}
	if got := h.notice(c.hr.UserID, "Crew Toko terlambat hari ini"); got != "alert|/dashboard/hris/attendance|"+msg {
		t.Errorf("HR bell = %q", got)
	}
	// The coordinator (Staff) is not an approver.
	if got := h.notice(c.koor.UserID, "Crew Toko terlambat hari ini"); got != "" {
		t.Errorf("coordinator bell = %q, want none", got)
	}
}

func TestClockInOnTimeNoNotice(t *testing.T) {
	h, _ := newFileHarness(t)
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 0, closing_offset_days = 3`)
	c := h.tieredChain()
	h.lateShift(c.crewID, "2026-10-06")
	h.setNow("2026-10-06 08:03")
	expect(t, h.do(&c.crew, "POST", "/api/hris/attendance", clockInBody(dataURL("image/png", pngBytes(t)), nil, "")), 200, "")
	if got := h.scalar(`SELECT count(*)::text FROM public.notifications WHERE user_id = ANY($1::uuid[]) AND title LIKE '%terlambat hari ini'`,
		[]string{c.crew.UserID, c.spv.UserID, c.mgr.UserID, c.hr.UserID}); got != "0" {
		t.Fatalf("on-time clock-in bells = %s, want 0", got)
	}
}

func TestLateNoticeApproverDedup(t *testing.T) {
	h, _ := newFileHarness(t)
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 0, closing_offset_days = 3`)
	c := h.tieredChain()
	// Directly under the Manager: request_approver = request_manager.
	staff, id := h.staff("pos", nil, true, emp{name: "Admin Area", boss: &c.mgrID})
	h.lateShift(id, "2026-10-06")
	h.late(id, "2026-10-01", 12, true)
	h.setNow("2026-10-06 08:20")
	expect(t, h.do(&staff, "POST", "/api/hris/attendance", clockInBody(dataURL("image/png", pngBytes(t)), nil, "")), 200, "")
	got := h.notice(c.mgr.UserID, "Admin Area terlambat hari ini")
	if got != "alert|/dashboard/me/absensi-tim|Admin Area terlambat 20 menit hari ini (2 kali di periode ini)." {
		t.Fatalf("Manager bells = %q, want exactly one", got)
	}
}

package hris

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

// wibClock is a timestamptz column as "HH:MM" WIB for the attendance row.
func (h *harness) wibClock(col, attendanceID string) string {
	h.t.Helper()
	return h.scalar(`SELECT to_char(`+col+` AT TIME ZONE 'Asia/Jakarta', 'HH24:MI') FROM hris.attendance WHERE id = $1`, attendanceID)
}

// EPIC-080 R8: a two-shift day is one attendance row: lateness from shift 1,
// scheduled_end from shift 2, break = both breaks + the gap between them.
func TestClockInTwoShiftDay(t *testing.T) {
	h, _ := newFileHarness(t)
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 0, closing_offset_days = 3`)
	photo := dataURL("image/png", pngBytes(t))

	shiftA := h.scalar(`INSERT INTO hris.shifts (code, name, start_time, end_time, break_minutes, late_tolerance_minutes)
		VALUES ('PAGI4X', 'PAGI4X', '08:00', '12:00', 0, 3) RETURNING id::text`)
	shiftB := h.scalar(`INSERT INTO hris.shifts (code, name, start_time, end_time, break_minutes, late_tolerance_minutes)
		VALUES ('SORE4X', 'SORE4X', '17:00', '21:00', 0, 120) RETURNING id::text`)

	staff, emp1 := h.staff("pos", nil, true, emp{name: "Crew Dua Shift"})
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, shift2_id, batch_id)
		VALUES ($1, '2026-10-06', $2, $3, gen_random_uuid())`, emp1, shiftA, shiftB)

	h.setNow("2026-10-06 08:05")
	in := h.do(&staff, "POST", "/api/hris/attendance", clockInBody(photo, nil, ""))
	expect(t, in, 200, "")
	d := in.data()
	id := d["id"].(string)
	if d["shift_id"] != shiftA {
		t.Fatalf("shift_id = %v, want the first shift %s (%s)", d["shift_id"], shiftA, in.raw)
	}
	if got := h.wibClock("scheduled_start", id); got != "08:00" {
		t.Errorf("scheduled_start = %s, want 08:00", got)
	}
	if got := h.wibClock("scheduled_end", id); got != "21:00" {
		t.Errorf("scheduled_end = %s, want 21:00 (end of shift 2)", got)
	}
	if d["break_minutes"] != float64(300) {
		t.Errorf("break_minutes = %v, want 300 (0 + 0 + 5h gap) (%s)", d["break_minutes"], in.raw)
	}
	// Late by shift 1's tolerance (3 min), not shift 2's (120 min).
	if d["is_late"] != true || d["late_minutes"] != float64(5) || d["late_tolerance_minutes"] != float64(3) {
		t.Errorf("lateness = is_late %v, late_minutes %v, tolerance %v, want true/5/3 (%s)",
			d["is_late"], d["late_minutes"], d["late_tolerance_minutes"], in.raw)
	}

	// 08:05 → 21:05 is 13h; minus the 5h gap = 8h net.
	h.setNow("2026-10-06 21:05")
	out := h.do(&staff, "POST", "/api/hris/attendance", map[string]any{"action": "clock-out", "attendance_id": id, "photo": photo})
	expect(t, out, 200, "")
	if wh := h.scalar(`SELECT work_hours::text FROM hris.attendance WHERE id = $1`, id); wh != "8.00" {
		t.Errorf("work_hours = %s, want 8.00 (%s)", wh, out.raw)
	}
}

// A one-shift day takes its break from the shift master (OPSSAT = 0, OPS = 60),
// not the attendance table default.
func TestClockInOneShiftBreakFromMaster(t *testing.T) {
	h, _ := newFileHarness(t)
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 0, closing_offset_days = 3`)
	photo := dataURL("image/png", pngBytes(t))

	for _, tc := range []struct {
		code, date, now string
		want            float64
	}{
		{"OPSSAT", "2026-10-10", "2026-10-10 08:00", 0},
		{"OPS", "2026-10-09", "2026-10-09 08:00", 60},
	} {
		staff, e := h.staff("pos", nil, true, emp{name: "Crew " + tc.code})
		h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id)
			SELECT $1, $2::date, id, gen_random_uuid() FROM hris.shifts WHERE code = $3`, e, tc.date, tc.code)
		h.setNow(tc.now)
		in := h.do(&staff, "POST", "/api/hris/attendance", clockInBody(photo, nil, ""))
		expect(t, in, 200, "")
		if got := in.data()["break_minutes"]; got != tc.want {
			t.Errorf("%s break_minutes = %v, want %v (%s)", tc.code, got, tc.want, in.raw)
		}
	}
}

// EPIC-080 R6: the ESS home and the HR daily roster name both shifts and show
// both clock ranges; a one-shift day reads as before.
func TestTwoShiftDayDisplay(t *testing.T) {
	h := newHarness(t)
	h.setNow("2026-10-06 07:00")
	hr, _ := h.staff("hrd", scheduleTestMenus, false)
	shiftA := h.scalar(`INSERT INTO hris.shifts (code, name, start_time, end_time, break_minutes)
		VALUES ('PAGI4X', 'Pagi Empat', '08:00', '12:00', 0) RETURNING id::text`)
	shiftB := h.scalar(`INSERT INTO hris.shifts (code, name, start_time, end_time, break_minutes)
		VALUES ('SORE4X', 'Sore Empat', '17:00', '21:00', 0) RETURNING id::text`)

	staff, two := h.staff("pos", nil, true, emp{name: "Crew Tampil Dua"})
	one := h.employee(emp{name: "Crew Tampil Satu"})
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, shift2_id, batch_id)
		VALUES ($1, '2026-10-06', $2, $3, gen_random_uuid())`, two, shiftA, shiftB)
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id)
		VALUES ($1, '2026-10-06', $2, gen_random_uuid())`, one, shiftA)

	me := h.do(&staff, "GET", "/api/hris/me", nil)
	expect(t, me, 200, "")
	ts, _ := me.data()["today_shift"].(map[string]any)
	if ts["name"] != "PAGI4X + SORE4X" || ts["start_time"] != "08:00:00" || ts["end_time"] != "21:00:00" ||
		ts["time_label"] != "08.00–12.00, 17.00–21.00" {
		t.Fatalf("ESS today_shift = %s", me.raw)
	}

	roster := h.do(&hr, "GET", "/api/hris/attendance/daily-roster?date=2026-10-06", nil)
	expect(t, roster, 200, "")
	shifts := map[string]map[string]any{}
	for _, e := range roster.data()["employees"].([]any) {
		m := e.(map[string]any)
		if sh, ok := m["shift"].(map[string]any); ok {
			shifts[m["employee_id"].(string)] = sh
		}
	}
	if sh := shifts[two]; sh["id"] != shiftA || sh["name"] != "PAGI4X + SORE4X" || sh["end_time"] != "21:00:00" ||
		sh["time_label"] != "08.00–12.00, 17.00–21.00" {
		t.Fatalf("roster two-shift = %v", sh)
	}
	if sh := shifts[one]; sh["name"] != "Pagi Empat" || sh["end_time"] != "12:00:00" || sh["time_label"] != "08.00–12.00" {
		t.Fatalf("roster one-shift = %v", sh)
	}
}

// EPIC-080 R3-R5: the job closes open rows of ruled job titles at the shift
// end; everything else is left alone.
func TestRunAutoCheckout(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	wib := time.FixedZone("WIB", 7*3600)
	at := func(hour, min int) time.Time { return time.Date(2026, 10, 5, hour, min, 0, 0, wib) }

	position := func(title string) string {
		return h.scalar(`INSERT INTO hris.positions (title) VALUES ($1) RETURNING id::text`, title)
	}
	worker := func(name, pos string) string {
		id := h.employee(emp{name: name})
		if pos != "" {
			h.exec(`UPDATE hris.employees SET job_title_id = $2 WHERE id = $1`, id, pos)
		}
		return id
	}
	// row inserts an open attendance row: times are WIB "YYYY-MM-DD HH:MM".
	row := func(employee, date, in, end, branch string) string {
		return h.scalar(`INSERT INTO hris.attendance (employee_id, date, clock_in, scheduled_end, branch_id, break_minutes)
			VALUES ($1, $2::date, ($3 || '+07')::timestamptz, ($4 || '+07')::timestamptz, NULLIF($5, '')::uuid, 120)
			RETURNING id::text`, employee, date, in, end, branch)
	}
	state := func(id string) string {
		return h.scalar(`SELECT coalesce(to_char(clock_out AT TIME ZONE 'Asia/Jakarta', 'HH24:MI'), '-') || '|' || clock_out_auto::text
			FROM hris.attendance WHERE id = $1`, id)
	}
	run := func(now time.Time) int {
		t.Helper()
		n, err := h.svc.RunAutoCheckout(ctx, now)
		if err != nil {
			t.Fatalf("RunAutoCheckout: %v", err)
		}
		return n
	}

	branchA, branchB := h.branch("Toko Checkout A"), h.branch("Toko Checkout B")
	hostPos, shopPos := position("Host Uji Checkout"), position("Shopkeeper Uji Checkout")
	offPos, latePos := position("Jabatan Aturan Mati"), position("Jabatan Pindah Uji")
	h.exec(`INSERT INTO hris.auto_checkout_rules (position_id) VALUES ($1)`, hostPos)
	h.exec(`INSERT INTO hris.auto_checkout_rules (position_id, branch_id) VALUES ($1, $2)`, shopPos, branchA)
	h.exec(`INSERT INTO hris.auto_checkout_rules (position_id, is_active) VALUES ($1, false)`, offPos)
	h.exec(`INSERT INTO hris.auto_checkout_rules (position_id) VALUES ($1)`, latePos)
	hr, _ := h.staff("hrd", nil, false)

	host := row(worker("Host Live Uji", hostPos), "2026-10-05", "2026-10-05 07:58", "2026-10-05 16:00", "")
	shopA := row(worker("Shopkeeper Toko A", shopPos), "2026-10-05", "2026-10-05 08:00", "2026-10-05 16:00", branchA)
	shopB := row(worker("Shopkeeper Toko B", shopPos), "2026-10-05", "2026-10-05 08:00", "2026-10-05 16:00", branchB)
	manualEmp := worker("Host Pulang Manual", hostPos)
	manual := row(manualEmp, "2026-10-05", "2026-10-05 08:00", "2026-10-05 16:00", "")
	h.exec(`UPDATE hris.attendance SET clock_out = '2026-10-05 15:30:00+07' WHERE id = $1`, manual)
	afterEnd := row(worker("Host Masuk Lewat Jam", hostPos), "2026-10-05", "2026-10-05 16:30", "2026-10-05 16:00", "")
	overnight := row(worker("Host CS3 Semalam", hostPos), "2026-10-04", "2026-10-04 16:00", "2026-10-05 00:00", "")
	ruleOff := row(worker("Jabatan Aturan Mati", offPos), "2026-10-05", "2026-10-05 08:00", "2026-10-05 16:00", "")
	noTitle := row(worker("Tanpa Jabatan", ""), "2026-10-05", "2026-10-05 08:00", "2026-10-05 16:00", "")
	noEnd := row(worker("Host Tanpa Jam Selesai", hostPos), "2026-10-05", "2026-10-05 08:00", "2026-10-05 16:00", "")
	h.exec(`UPDATE hris.attendance SET scheduled_end = NULL WHERE id = $1`, noEnd)
	inAtEnd := row(worker("Host Masuk Tepat Jam Selesai", hostPos), "2026-10-05", "2026-10-05 16:00", "2026-10-05 16:00", "")
	tooOld := row(worker("Host Terlalu Lama", hostPos), "2026-10-02", "2026-10-02 08:00", "2026-10-02 16:00", "")
	moved := row(worker("Pindah Jabatan", ""), "2026-10-05", "2026-10-05 08:00", "2026-10-05 16:00", "")
	h.exec(`UPDATE hris.employees e SET job_title_id = $2 FROM hris.attendance a WHERE a.id = $1 AND e.id = a.employee_id`, moved, latePos)
	// A row inside a closed period (created before the period closes).
	closedRow := row(worker("Host Periode Tertutup", hostPos), "2026-10-03", "2026-10-03 08:00", "2026-10-03 16:00", "")
	h.exec(`INSERT INTO hris.attendance_periods (period_year, period_month, start_date, end_date, closing_date, status, closed_at)
		VALUES (2026, 10, '2026-10-03', '2026-10-03', '2026-10-03', 'closed', now())`)

	if got := h.scalar(`SELECT is_short_hours::text FROM hris.attendance WHERE id = $1`, host); got != "false" {
		t.Fatalf("precondition is_short_hours = %s", got)
	}

	// Before 16.00 only the overnight row (its shift ended at 00.00) closes.
	if n := run(at(15, 59)); n != 1 {
		t.Fatalf("run at 15.59 closed %d, want 1 (overnight only)", n)
	}
	if got := state(host); got != "-|false" {
		t.Fatalf("host after 15.59 run = %s", got)
	}

	// At 16.05: host, shopkeeper A and the moved employee.
	if n := run(at(16, 5)); n != 3 {
		t.Fatalf("run at 16.05 closed %d, want 3", n)
	}
	for name, id := range map[string]string{"host": host, "shopkeeper A": shopA, "moved title": moved} {
		if got := state(id); got != "16:00|true" {
			t.Errorf("%s = %s, want 16:00|true", name, got)
		}
	}
	if got := state(overnight); got != "00:00|true" {
		t.Errorf("overnight = %s, want 00:00|true (dated yesterday)", got)
	}
	for name, id := range map[string]string{"shopkeeper B": shopB, "after end": afterEnd, "rule off": ruleOff,
		"no title": noTitle, "no scheduled_end": noEnd, "clock-in at the end": inAtEnd, "closed period": closedRow, "outside lookback": tooOld} {
		if got := state(id); got != "-|false" {
			t.Errorf("%s = %s, want untouched", name, got)
		}
	}
	if got := state(manual); got != "15:30|false" {
		t.Errorf("manual clock-out = %s, want 15:30|false", got)
	}

	// Details of one closed row: note, geofence, hours, audit, no HR notification.
	if got := h.scalar(`SELECT notes || '|' || clock_out_geofence || '|' || work_hours::text FROM hris.attendance WHERE id = $1`, host); got !=
		"Checkout otomatis pukul 16.00 (jam selesai shift)|unchecked|6.03" {
		t.Errorf("host note|geofence|hours = %s", got)
	}
	if got := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE entity = 'hris.attendance'
		AND action = 'attendance.auto_checkout' AND actor_name = 'Sistem (checkout otomatis)' AND actor_id IS NULL
		AND entity_id = $1 AND after->>'clock_out_auto' = 'true' AND (after->>'clock_out')::timestamptz = '2026-10-05 16:00:00+07'`, host); got != "1" {
		t.Errorf("host audit rows = %s, want 1", got)
	}
	if got := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'attendance.auto_checkout'`); got != "4" {
		t.Errorf("audit rows = %s, want 4", got)
	}
	// The only "Jam Kerja Kurang" is the manual 15.30 clock-out's (precondition:
	// the same trigger does fire for a manual clock-out of a short day).
	if got := h.scalar(`SELECT string_agg(title, ';') FROM public.notifications WHERE user_id = $1 AND title LIKE '%Jam Kerja Kurang%'`, hr.UserID); got != "⚠️ Jam Kerja Kurang: Host Pulang Manual" {
		t.Errorf("short-hours notifications = %q, want only the manual clock-out's", got)
	}
	// An existing note is kept in front of the auto note.
	h.exec(`UPDATE hris.attendance SET notes = 'Catatan awal' WHERE id = $1`, shopB)
	h.exec(`INSERT INTO hris.auto_checkout_rules (position_id, branch_id) VALUES ($1, $2)`, shopPos, branchB)
	if n := run(at(16, 6)); n != 1 {
		t.Fatalf("run after rule for branch B closed %d, want 1", n)
	}
	if got := h.scalar(`SELECT notes FROM hris.attendance WHERE id = $1`, shopB); got != "Catatan awal · Checkout otomatis pukul 16.00 (jam selesai shift)" {
		t.Errorf("shopkeeper B notes = %q", got)
	}

	// Idempotent.
	if n := run(at(16, 10)); n != 0 {
		t.Fatalf("second run closed %d, want 0", n)
	}
	// The rule switched off stops it for the next rows.
	h.exec(`UPDATE hris.auto_checkout_rules SET is_active = false WHERE position_id = $1`, latePos)
	later := row(worker("Setelah Aturan Mati", latePos), "2026-10-05", "2026-10-05 09:00", "2026-10-05 16:00", "")
	if n := run(at(16, 15)); n != 0 || state(later) != "-|false" {
		t.Fatalf("switched-off rule closed %d rows, later = %s", n, state(later))
	}
}

// The loop runs once at start (no wait for a tick) and stops with its context.
func TestAutoCheckoutLoopRunsAtStart(t *testing.T) {
	h := newHarness(t)
	logged := make(chan string, 8)
	h.svc.log = slog.New(logSignal{ch: logged})
	pos := h.scalar(`INSERT INTO hris.positions (title) VALUES ('Jabatan Loop Uji') RETURNING id::text`)
	h.exec(`INSERT INTO hris.auto_checkout_rules (position_id) VALUES ($1)`, pos)
	e := h.employee(emp{name: "Pegawai Loop"})
	h.exec(`UPDATE hris.employees SET job_title_id = $2 WHERE id = $1`, e, pos)
	// fixedNow is Monday 10:00 WIB; the shift ended at 09:00.
	id := h.scalar(`INSERT INTO hris.attendance (employee_id, date, clock_in, scheduled_end)
		VALUES ($1, '2026-10-05', '2026-10-05 01:00:00+00', '2026-10-05 02:00:00+00') RETURNING id::text`, e)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		h.svc.AutoCheckoutLoop(ctx, time.Hour)
		close(done)
	}()
	select {
	case msg := <-logged:
		if msg != "hris auto checkout" {
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
	if got := h.scalar(`SELECT clock_out_auto::text FROM hris.attendance WHERE id = $1`, id); got != "true" {
		t.Fatalf("start run did not close the row (clock_out_auto = %s)", got)
	}
}

// The lookback is counted in WIB days: at 00:30 WIB (17:30 UTC the day before)
// today-2 in WIB is still included, today-3 is not.
func TestRunAutoCheckoutLookbackAtWIBMidnight(t *testing.T) {
	h := newHarness(t)
	pos := h.scalar(`INSERT INTO hris.positions (title) VALUES ('Jabatan Lookback Uji') RETURNING id::text`)
	h.exec(`INSERT INTO hris.auto_checkout_rules (position_id) VALUES ($1)`, pos)
	row := func(date, end string) string {
		e := h.employee(emp{name: "Lookback " + date})
		h.exec(`UPDATE hris.employees SET job_title_id = $2 WHERE id = $1`, e, pos)
		return h.scalar(`INSERT INTO hris.attendance (employee_id, date, clock_in, scheduled_end)
			VALUES ($1, $2::date, ($2 || ' 08:00+07')::timestamptz, ($3 || '+07')::timestamptz) RETURNING id::text`, e, date, end)
	}
	inWindow := row("2026-10-03", "2026-10-03 16:00")     // today-2 (WIB)
	overnight := row("2026-10-04", "2026-10-05 00:00")    // dated yesterday
	outside := row("2026-10-02", "2026-10-02 16:00")      // today-3
	now := time.Date(2026, 10, 4, 17, 30, 0, 0, time.UTC) // 2026-10-05 00:30 WIB
	n, err := h.svc.RunAutoCheckout(context.Background(), now)
	if err != nil || n != 2 {
		t.Fatalf("RunAutoCheckout = %d, %v; want 2", n, err)
	}
	auto := func(id string) string {
		return h.scalar(`SELECT clock_out_auto::text FROM hris.attendance WHERE id = $1`, id)
	}
	if auto(inWindow) != "true" || auto(overnight) != "true" || auto(outside) != "false" {
		t.Fatalf("auto = in-window %s, overnight %s, outside %s; want true, true, false", auto(inWindow), auto(overnight), auto(outside))
	}
}

// staleRecordRepo serves the attendance row as it was before the automatic
// checkout, the way a manual clock-out that read the row earlier sees it.
type staleRecordRepo struct {
	Repository
	stale *Row
}

func (r staleRecordRepo) AttendanceRecord(context.Context, string) (*Row, error) { return r.stale, nil }

// EPIC-080 R5: a manual clock-out cannot overwrite an automatic one, neither
// when it reads the closed row nor when it read it just before the job ran.
func TestManualClockOutAfterAutoCheckout(t *testing.T) {
	h, _ := newFileHarness(t)
	photo := dataURL("image/png", pngBytes(t))
	pos := h.scalar(`INSERT INTO hris.positions (title) VALUES ('Jabatan Balapan Uji') RETURNING id::text`)
	h.exec(`INSERT INTO hris.auto_checkout_rules (position_id) VALUES ($1)`, pos)
	staff, e := h.staff("pos", nil, true, emp{name: "Crew Balapan"})
	h.exec(`UPDATE hris.employees SET job_title_id = $2 WHERE id = $1`, e, pos)
	id := h.scalar(`INSERT INTO hris.attendance (employee_id, date, clock_in, scheduled_end, notes)
		VALUES ($1, '2026-10-05', '2026-10-05 08:00:00+07', '2026-10-05 16:00:00+07', 'Catatan awal') RETURNING id::text`, e)
	stale, err := h.svc.repo.AttendanceRecord(context.Background(), id)
	if err != nil || stale == nil || stale.Get("clock_out") != nil {
		t.Fatalf("stale record = %v, %v", stale, err)
	}

	h.setNow("2026-10-05 16:10")
	if n, err := h.svc.RunAutoCheckout(context.Background(), time.Date(2026, 10, 5, 16, 10, 0, 0, time.FixedZone("WIB", 7*3600))); err != nil || n != 1 {
		t.Fatalf("RunAutoCheckout = %d, %v; want 1", n, err)
	}
	unchanged := func() {
		t.Helper()
		got := h.scalar(`SELECT to_char(clock_out AT TIME ZONE 'Asia/Jakarta', 'HH24:MI') || '|' || clock_out_auto::text || '|' || notes
			|| '|' || coalesce(clock_out_photo_url, '-') FROM hris.attendance WHERE id = $1`, id)
		if want := "16:00|true|Catatan awal · Checkout otomatis pukul 16.00 (jam selesai shift)|-"; got != want {
			t.Fatalf("row = %q, want %q", got, want)
		}
	}
	body := map[string]any{"action": "clock-out", "attendance_id": id, "photo": photo}

	// Reads the closed row: the existing pre-check.
	expect(t, h.do(&staff, "POST", "/api/hris/attendance", body), 400, "Sudah clock-out untuk absensi ini")
	unchanged()

	// Read the row before the job ran: the conditional write refuses.
	h.svc.repo = staleRecordRepo{Repository: h.svc.repo, stale: stale}
	expect(t, h.do(&staff, "POST", "/api/hris/attendance", body), 400, "Sudah clock-out untuk absensi ini")
	unchanged()
}

package hris

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"nuhabit/backend/internal/platform/config"
	"nuhabit/backend/internal/platform/module"
)

// EPIC-067 R6, R7: ringkasan harian ke HR pukul 09.00 WIB, sekali per tanggal.
func TestGapDigestRun(t *testing.T) {
	h := newHarness(t)
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 0, closing_offset_days = 3`)
	ctx := context.Background()
	// D = Kamis 1 Okt 2026; ringkasan dikirim Jumat 2 Okt mulai 09.00 WIB.
	const d = "2026-10-01"
	wib := time.FixedZone("WIB", 7*3600)
	before := time.Date(2026, 10, 2, 8, 59, 0, 0, wib)
	at := time.Date(2026, 10, 2, 9, 0, 0, 0, wib)

	shift := h.scalar(`INSERT INTO hris.shifts (name, start_time, end_time, late_tolerance_minutes)
		VALUES ('Pagi Digest', '08:00', '17:00', 10) RETURNING id::text`)
	a := h.employee(emp{name: "Digest A Terlewat"})
	h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from) VALUES ($1, 4, $2, '2026-01-01')`, a, shift)

	hr1, _ := h.staff("hrd", nil, false)
	hr2, _ := h.staff("hrd", nil, false)
	plain, _ := h.staff("pos", nil, false)
	hrdUsers := h.scalar(`SELECT count(*)::text FROM users WHERE role = 'hrd'`)
	notifications := func(userID string) string {
		return h.scalar(`SELECT count(*)::text FROM public.notifications WHERE user_id = $1 AND title = 'Ringkasan Absen Terlewat'`, userID)
	}
	markers := func() string { return h.scalar(`SELECT count(*)::text FROM hris.attendance_gap_digests`) }

	// (a) sebelum 09.00 WIB: tidak ada yang terjadi.
	got, err := h.svc.RunGapDigest(ctx, before)
	if err != nil || got != nil {
		t.Fatalf("before 09:00 = %+v, %v", got, err)
	}
	if markers() != "0" {
		t.Fatalf("markers after 08:59 = %s", markers())
	}

	// (b) tepat 09.00 WIB: satu notifikasi per user hrd.
	got, err = h.svc.RunGapDigest(ctx, at)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Date != d || !got.Sent || got.Summary == nil || got.Summary.Missing != 1 ||
		got.Recipients != atoi(t, hrdUsers) {
		t.Fatalf("at 09:00 = %+v (hrd users %s)", got, hrdUsers)
	}
	for _, s := range []string{hr1.UserID, hr2.UserID} {
		if notifications(s) != "1" {
			t.Fatalf("user %s notifications = %s", s, notifications(s))
		}
	}
	if notifications(plain.UserID) != "0" {
		t.Fatalf("non-hrd got a notification")
	}
	row := h.scalar(`SELECT concat_ws('|', type, link, message) FROM public.notifications WHERE user_id = $1`, hr1.UserID)
	if want := "reminder|/dashboard/hris/attendance|Kemarin (Kam, 1 Okt 2026): 1 karyawan belum absen, 0 belum absen pulang."; row != want {
		t.Fatalf("notification = %q, want %q", row, want)
	}
	marker := h.scalar(`SELECT concat_ws('|', missing, no_clock_out, unscheduled, recipients, (sent_at IS NOT NULL)::text)
		FROM hris.attendance_gap_digests WHERE date = $1`, d)
	if want := "1|0|0|" + hrdUsers + "|true"; marker != want {
		t.Fatalf("marker = %q, want %q", marker, want)
	}

	// (c) panggilan kedua pada waktu yang sama: tidak mengirim lagi.
	got, err = h.svc.RunGapDigest(ctx, at)
	if err != nil || got != nil {
		t.Fatalf("second run = %+v, %v", got, err)
	}
	if notifications(hr1.UserID) != "1" || markers() != "1" {
		t.Fatalf("second run changed state: notifications %s markers %s", notifications(hr1.UserID), markers())
	}

	// (d) tanggal lain tanpa celah: penanda ada, tanpa notifikasi.
	got, err = h.svc.RunGapDigest(ctx, time.Date(2026, 10, 4, 9, 30, 0, 0, wib)) // D = Sabtu 3 Okt
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Date != "2026-10-03" || got.Sent || got.Recipients != 0 || got.Summary == nil {
		t.Fatalf("no gaps run = %+v", got)
	}
	marker = h.scalar(`SELECT concat_ws('|', missing, no_clock_out, recipients, (sent_at IS NULL)::text)
		FROM hris.attendance_gap_digests WHERE date = '2026-10-03'`)
	if marker != "0|0|0|true" {
		t.Fatalf("no gaps marker = %q", marker)
	}
	if notifications(hr1.UserID) != "1" {
		t.Fatalf("no gaps run sent a notification")
	}
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

// logSignal is a slog handler that signals each record on a channel.
type logSignal struct{ ch chan string }

func (l logSignal) Enabled(context.Context, slog.Level) bool { return true }
func (l logSignal) Handle(_ context.Context, r slog.Record) error {
	l.ch <- r.Message
	return nil
}
func (l logSignal) WithAttrs([]slog.Attr) slog.Handler { return l }
func (l logSignal) WithGroup(string) slog.Handler      { return l }

// EPIC-067 R8: the loop runs once at start (without waiting for a tick) and
// stops with its context.
func TestGapDigestLoopRunsAtStart(t *testing.T) {
	h := newHarness(t)
	logged := make(chan string, 8)
	h.svc.log = slog.New(logSignal{ch: logged})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		h.svc.GapDigestLoop(ctx, time.Hour)
		close(done)
	}()
	// fixedNow is Monday 10:00 WIB, so the start run processes Sunday 4 Okt.
	select {
	case msg := <-logged:
		if msg != "hris gap digest" {
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
	if h.scalar(`SELECT count(*)::text FROM hris.attendance_gap_digests WHERE date = '2026-10-04'`) != "1" {
		t.Fatal("start run left no marker")
	}
}

// EPIC-067 C1: app.Routes() / `api -routes` build the module without a
// database; the digest loop must not start there (it would only log a
// failure every tick).
func TestNewWithoutDBStartsNoDigestLoop(t *testing.T) {
	logged := make(chan string, 8)
	deps := module.Deps{Log: slog.New(logSignal{ch: logged}), Now: func() time.Time { return fixedNow },
		Config: config.Config{AppEnv: "development"}}
	New(deps, Ports{})
	select {
	case msg := <-logged:
		t.Fatalf("digest loop ran without a database: %q", msg)
	case <-time.After(300 * time.Millisecond):
	}
}

// EPIC-067 C8: gaps but no HR account to notify → a warning in the log; the
// marker still records the run as sent (sent_at) with 0 recipients.
func TestGapDigestWarnsWithoutRecipients(t *testing.T) {
	h := newHarness(t)
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 0, closing_offset_days = 3`)
	shift := h.scalar(`INSERT INTO hris.shifts (name, start_time, end_time, late_tolerance_minutes)
		VALUES ('Pagi Digest Kosong', '08:00', '17:00', 10) RETURNING id::text`)
	a := h.employee(emp{name: "Digest Kosong Terlewat"})
	h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from) VALUES ($1, 4, $2, '2026-01-01')`, a, shift)
	h.exec(`UPDATE users SET role = 'admin' WHERE role = 'hrd'`)
	logged := make(chan string, 8)
	h.svc.log = slog.New(logSignal{ch: logged})

	wib := time.FixedZone("WIB", 7*3600)
	got, err := h.svc.RunGapDigest(context.Background(), time.Date(2026, 10, 2, 9, 0, 0, 0, wib))
	if err != nil || got == nil || !got.Sent || got.Recipients != 0 || got.Summary.Missing != 1 {
		t.Fatalf("digest = %+v, %v", got, err)
	}
	select {
	case msg := <-logged:
		if msg != "hris gap digest has gaps but no HR recipients" {
			t.Fatalf("log = %q", msg)
		}
	default:
		t.Fatal("no warning for a digest without recipients")
	}
	if m := h.scalar(`SELECT concat_ws('|', recipients, (sent_at IS NOT NULL)::text) FROM hris.attendance_gap_digests
		WHERE date = '2026-10-01'`); m != "0|true" {
		t.Fatalf("marker = %q", m)
	}
}

package domain

import (
	"testing"
	"time"
)

// requestNow is Monday 2026-10-05 10:00 WIB.
var requestNow = time.Date(2026, 10, 5, 3, 0, 0, 0, time.UTC)

func instant(s string) *time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return &t
}

func TestCheckAttendanceRequest(t *testing.T) {
	const why = "Internet toko mati"
	cases := []struct {
		name      string
		in        AttendanceRequestInput
		kind, msg string
	}{
		{"absen manual lengkap", AttendanceRequestInput{Date: "2026-10-02", ClockIn: sp("08:00"), ClockOut: sp("17:00"), Reason: why},
			AttendanceRequestManual, ""},
		{"tanggal tidak valid", AttendanceRequestInput{Date: "2026-13-40", ClockIn: sp("08:00"), Reason: why}, "", "Format tanggal tidak valid"},
		{"masa depan", AttendanceRequestInput{Date: "2026-10-06", ClockIn: sp("08:00"), Reason: why}, "",
			"Tanggal pengajuan tidak boleh di masa depan"},
		{"keterangan pendek", AttendanceRequestInput{Date: "2026-10-02", ClockIn: sp("08:00"), Reason: "  Lupa "}, "",
			"Keterangan wajib diisi (minimal 5 karakter)"},
		{"jam rusak", AttendanceRequestInput{Date: "2026-10-02", ClockIn: sp("8:00"), Reason: why}, "", "Format jam harus HH:MM"},
		{"tanpa jam", AttendanceRequestInput{Date: "2026-10-02", Reason: why}, "", "Isi jam masuk dan/atau jam pulang yang diajukan"},
		{"absen manual tanpa jam masuk", AttendanceRequestInput{Date: "2026-10-02", ClockOut: sp("17:00"), Reason: why}, "",
			"Jam masuk wajib diisi"},
		{"jam masuk belum terjadi", AttendanceRequestInput{Date: "2026-10-05", ClockIn: sp("10:01"), Reason: why}, "",
			"Jam yang diajukan belum terjadi"},
		{"menit berjalan diterima", AttendanceRequestInput{Date: "2026-10-05", ClockIn: sp("10:00"), Reason: why},
			AttendanceRequestManual, ""},
		{"lewat tengah malam, pulang sudah lewat", AttendanceRequestInput{Date: "2026-10-04", ClockIn: sp("22:00"), ClockOut: sp("06:00"), Reason: why},
			AttendanceRequestManual, ""},
		{"lewat tengah malam, pulang belum terjadi", AttendanceRequestInput{Date: "2026-10-04", ClockIn: sp("22:00"), ClockOut: sp("11:00"), Reason: why},
			"", "Jam yang diajukan belum terjadi"},
		{"koreksi jam pulang", AttendanceRequestInput{Date: "2026-10-03", ClockOut: sp("17:00"), Reason: why, HasRow: true,
			RecordedIn: instant("2026-10-03T01:02:37Z")}, AttendanceRequestCorrection, ""},
		{"koreksi baris tanpa jam masuk", AttendanceRequestInput{Date: "2026-10-03", ClockOut: sp("17:00"), Reason: why, HasRow: true},
			"", "Jam masuk wajib diisi"},
		{"koreksi baris tidak hadir dengan jam lengkap", AttendanceRequestInput{Date: "2026-10-03", ClockIn: sp("08:00"), ClockOut: sp("17:00"),
			Reason: why, HasRow: true}, AttendanceRequestCorrection, ""},
		{"koreksi sama dengan catatan (detik diabaikan)", AttendanceRequestInput{Date: "2026-10-03", ClockIn: sp("08:02"), Reason: why,
			HasRow: true, RecordedIn: instant("2026-10-03T01:02:37Z"), RecordedOut: instant("2026-10-03T10:00:00Z")},
			"", "Jam yang diajukan sama dengan absensi tercatat"},
	}
	for _, c := range cases {
		kind, msg := CheckAttendanceRequest(c.in, requestNow)
		if kind != c.kind || msg != c.msg {
			t.Errorf("%s: got (%q, %q), want (%q, %q)", c.name, kind, msg, c.kind, c.msg)
		}
	}
}

func TestAttendanceGaps(t *testing.T) {
	shift := "shift-pagi"
	var schedule []ScheduleRow
	for dow := 1; dow <= 6; dow++ {
		schedule = append(schedule, ScheduleRow{DayOfWeek: dow, ShiftID: &shift, EffectiveFrom: "2026-01-01"})
	}
	schedule = append(schedule, ScheduleRow{DayOfWeek: 7, EffectiveFrom: "2026-01-01"}) // Minggu libur
	in := GapInput{
		Start: "2026-09-28", End: "2026-10-04", Schedule: schedule,
		Holidays: map[string]bool{"2026-10-01": true},
		OnLeave:  map[string]bool{"2026-09-29": true},
		Recorded: map[string]RecordedDay{
			"2026-09-30": {ClockIn: instant("2026-09-30T01:05:00Z")},
			"2026-10-02": {ClockIn: instant("2026-10-02T01:00:00Z"), ClockOut: instant("2026-10-02T10:00:00Z")},
			"2026-10-03": {Absent: true},
		},
		Pending: map[string]bool{"2026-09-28": true},
	}
	// 28 Sep pending, 29 Sep cuti, 30 Sep belum pulang, 1 Okt libur, 2 Okt lengkap, 3 Okt "Tidak hadir" dari HR, 4 Okt Minggu.
	got := AttendanceGaps(in)
	if len(got) != 1 || got[0].Date != "2026-09-30" || got[0].Kind != GapNoClockOut || got[0].ClockIn == nil || *got[0].ClockIn != "08:05" {
		t.Fatalf("gaps = %+v", got)
	}
	delete(in.Pending, "2026-09-28")
	in.OnLeave = map[string]bool{}
	got = AttendanceGaps(in)
	if len(got) != 3 || got[0].Date != "2026-09-28" || got[0].Kind != GapMissing || got[0].ClockIn != nil ||
		got[1].Date != "2026-09-29" || got[2].Kind != GapNoClockOut {
		t.Fatalf("gaps without pending/leave = %+v", got)
	}
	// Tanpa jadwal: hanya absen pulang yang hilang yang diketahui.
	in.Schedule = nil
	if got = AttendanceGaps(in); len(got) != 1 || got[0].Date != "2026-09-30" {
		t.Fatalf("gaps without schedule = %+v", got)
	}
	if got = AttendanceGaps(GapInput{Start: "2026-10-05", End: "2026-10-04"}); got == nil || len(got) != 0 {
		t.Fatalf("empty window = %#v", got)
	}
}

func TestValidateMinWorkHours(t *testing.T) {
	const bad = "Jam kerja minimum 1–24 jam (kelipatan 0,25)"
	f := func(v float64) *float64 { return &v }
	for _, c := range []struct {
		in   *float64
		want string
	}{{nil, ""}, {f(9), ""}, {f(8.25), ""}, {f(1), ""}, {f(24), ""}, {f(0.5), bad}, {f(24.25), bad}, {f(9.1), bad}} {
		if got := ValidateMinWorkHours(c.in); got != c.want {
			t.Errorf("ValidateMinWorkHours(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestAttendanceNotices(t *testing.T) {
	title, msg := AttendanceRequestNotice("Budi", AttendanceRequestManual, "2026-10-02", sp("08:00"), sp("17:00"))
	if title != "Pengajuan Absensi: Budi" ||
		msg != "Budi mengajukan absen manual 2 Okt 2026 (masuk 08:00, pulang 17:00). Tinjau di HRIS → Absensi → Pengajuan Absensi." {
		t.Fatalf("request notice = %q / %q", title, msg)
	}
	if _, msg = AttendanceRequestNotice("Budi", AttendanceRequestCorrection, "2026-10-03", nil, sp("17:00")); msg !=
		"Budi mengajukan koreksi absen 3 Okt 2026 (pulang 17:00). Tinjau di HRIS → Absensi → Pengajuan Absensi." {
		t.Fatalf("correction notice = %q", msg)
	}
	if title, msg = AttendanceDecisionNotice("approved", "2026-10-02", nil); title != "Pengajuan absensi disetujui" ||
		msg != "Pengajuan absensi 2 Okt 2026 disetujui HR. Absensi sudah diperbarui." {
		t.Fatalf("approved notice = %q / %q", title, msg)
	}
	if title, msg = AttendanceDecisionNotice("rejected", "2026-10-02", sp("CCTV tidak sesuai")); title != "Pengajuan absensi ditolak" ||
		msg != "Pengajuan absensi 2 Okt 2026 ditolak HR. Alasan: CCTV tidak sesuai" {
		t.Fatalf("rejected notice = %q / %q", title, msg)
	}
}

package domain

import (
	"testing"
	"time"
)

func wib(s string) time.Time {
	t, _ := time.ParseInLocation("2006-01-02 15:04", s, time.FixedZone("WIB", 7*3600))
	return t
}

func TestOvertimeNotFinished(t *testing.T) {
	for _, c := range []struct {
		date, start, end, now string
		want                  bool
	}{
		{"2026-10-21", "17:00", "19:00", "2026-10-21 18:59", true},
		{"2026-10-21", "17:00", "19:00", "2026-10-21 19:00", false},
		{"2026-10-21", "17:00", "19:00", "2026-10-22 08:00", false},
		{"2026-10-21", "22:00", "02:00", "2026-10-22 01:00", true}, // lewat tengah malam
		{"2026-10-21", "22:00", "02:00", "2026-10-22 02:00", false},
	} {
		if got := OvertimeNotFinished(c.date, c.start, c.end, wib(c.now)); got != c.want {
			t.Errorf("%s %s–%s @%s = %v, want %v", c.date, c.start, c.end, c.now, got, c.want)
		}
	}
	if _, ok := OvertimeEndsAt("2026-02-30", "17:00", "19:00"); ok {
		t.Error("tanggal tidak ada harus false")
	}
}

func TestPaidOvertimeHours(t *testing.T) {
	for _, c := range []struct {
		start, end string
		brk        int
		want       float64
	}{
		{"17:00", "19:00", 0, 2},
		{"17:00", "20:00", 30, 2.5},
		{"22:00", "02:00", 60, 3},
		{"17:00", "18:00", 60, 0},
		{"17:00", "17:45", 0, 0.75},
	} {
		if got := PaidOvertimeHours(c.start, c.end, c.brk); got != c.want {
			t.Errorf("%s–%s istirahat %d = %v, want %v", c.start, c.end, c.brk, got, c.want)
		}
	}
	if got := OvertimeGrossMinutes("22:00", "02:00"); got != 240 {
		t.Errorf("gross = %d, want 240", got)
	}
}

func TestCanCorrectOvertimeTimes(t *testing.T) {
	boss, owner, hr := "boss", "owner", "hr"
	self := owner
	for _, c := range []struct {
		name          string
		actor         *string
		isHRD         bool
		reportingTo   *string
		managerActive bool
		want          bool
	}{
		{"atasan langsung", &boss, false, &boss, true, true},
		{"HR saat ada atasan", &hr, true, &boss, true, false},
		{"HR tanpa atasan", &hr, true, nil, false, true},
		{"HR saat atasan menunjuk diri sendiri", &hr, true, &self, true, true},
		{"HR saat atasan nonaktif", &hr, true, &boss, false, true},
		{"atasan nonaktif", &boss, false, &boss, false, false},
		{"karyawan sendiri", &owner, true, nil, false, false},
		{"orang lain", &hr, false, nil, false, false},
	} {
		got, msg := CanCorrectOvertimeTimes(c.actor, c.isHRD, owner, c.reportingTo, c.managerActive)
		if got != c.want || (!got && msg == "") {
			t.Errorf("%s = %v %q, want %v", c.name, got, msg, c.want)
		}
	}
}

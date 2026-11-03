package domain

import "testing"

func TestWeekBounds(t *testing.T) {
	for _, c := range []struct{ date, mon, sun string }{
		{"2026-11-04", "2026-11-02", "2026-11-08"},
		{"2026-11-08", "2026-11-02", "2026-11-08"},
		{"2026-11-02", "2026-11-02", "2026-11-08"},
	} {
		mon, sun, ok := WeekBounds(c.date)
		if !ok || mon != c.mon || sun != c.sun {
			t.Errorf("WeekBounds(%s) = %s %s %v", c.date, mon, sun, ok)
		}
	}
	if _, _, ok := WeekBounds("bukan"); ok {
		t.Error("tanggal salah harus !ok")
	}
}

func TestOvertimeStartError(t *testing.T) {
	ns := &OvertimeShiftEnd{End: "18:00"}
	msg := "Lembur hari kerja dimulai setelah jam kerja selesai (18:00)"
	cases := []struct {
		name, start, dayType string
		shift                *OvertimeShiftEnd
		trip                 bool
		want                 string
	}{
		{"sebelum selesai", "17:30", OvertimeWeekday, ns, false, msg},
		{"tepat selesai", "18:00", OvertimeWeekday, ns, false, ""},
		{"hari istirahat", "08:00", "rest_day", ns, false, ""},
		{"tanpa jadwal", "08:00", OvertimeWeekday, nil, false, ""},
		{"shift overnight", "01:00", OvertimeWeekday, &OvertimeShiftEnd{End: "07:00", Overnight: true}, false, ""},
		{"perjalanan dinas", "08:00", OvertimeWeekday, ns, true, ""},
		{"selesai shift kedua", "19:00", OvertimeWeekday, &OvertimeShiftEnd{End: "21:30"}, false,
			"Lembur hari kerja dimulai setelah jam kerja selesai (21:30)"},
	}
	for _, c := range cases {
		if got := OvertimeStartError(c.start, c.dayType, c.shift, c.trip); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestOvertimeCapError(t *testing.T) {
	const label = "21 Okt – 20 Nov 2026"
	cases := []struct {
		name                     string
		hours, day, week, period float64
		want                     string
	}{
		{"lebih 4 jam sehari", 4.5, 0, 0, 0, "Lembur maksimal 4 jam per hari"},
		{"tepat 4 jam sehari", 2, 2, 0, 0, ""},
		{"lewat batas minggu", 3, 0, 16, 0,
			"Lembur minggu 02-11 s.d. 08-11 sudah 16 jam; maksimal 18 jam per minggu (sisa 2 jam)"},
		{"lewat batas periode", 2, 0, 0, 70.5,
			"Lembur periode 21 Okt – 20 Nov 2026 sudah 70,5 jam; maksimal 72 jam per periode (sisa 1,5 jam)"},
		{"tepat batas", 2, 0, 16, 70, ""},
		{"sisa dibulatkan 2 desimal", 2, 0, 16.67, 0,
			"Lembur minggu 02-11 s.d. 08-11 sudah 16,67 jam; maksimal 18 jam per minggu (sisa 1,33 jam)"},
		{"sisa tidak negatif", 1, 0, 20, 0,
			"Lembur minggu 02-11 s.d. 08-11 sudah 20 jam; maksimal 18 jam per minggu (sisa 0 jam)"},
	}
	for _, c := range cases {
		if got := OvertimeCapError(c.hours, c.day, c.week, c.period, "2026-11-02", "2026-11-08", label); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

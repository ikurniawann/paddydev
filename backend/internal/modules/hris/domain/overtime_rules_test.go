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
		name                string
		hours, week, period float64
		want                string
	}{
		// EPIC-089 R1: tidak ada lagi batas harian.
		{"6 jam sehari boleh", 6, 0, 0, ""},
		{"lewat batas minggu", 3, 16, 0,
			"Lembur minggu 02-11 s.d. 08-11 sudah 16 jam; maksimal 18 jam per minggu (sisa 2 jam)"},
		{"lewat batas periode", 2, 0, 70.5,
			"Lembur periode 21 Okt – 20 Nov 2026 sudah 70,5 jam; maksimal 72 jam per periode (sisa 1,5 jam)"},
		{"tepat batas", 2, 16, 70, ""},
		{"sisa dibulatkan 2 desimal", 2, 16.67, 0,
			"Lembur minggu 02-11 s.d. 08-11 sudah 16,67 jam; maksimal 18 jam per minggu (sisa 1,33 jam)"},
		{"sisa tidak negatif", 1, 20, 0,
			"Lembur minggu 02-11 s.d. 08-11 sudah 20 jam; maksimal 18 jam per minggu (sisa 0 jam)"},
	}
	for _, c := range cases {
		if got := OvertimeCapError(c.hours, c.week, c.period, "2026-11-02", "2026-11-08", label); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// EPIC-089 R2: lebih dari 4 jam dibayar wajib Keterangan minimal 10 karakter.
func TestOvertimeReasonError(t *testing.T) {
	const msg = "Lembur lebih dari 4 jam: isi Keterangan alasannya (minimal 10 karakter)"
	cases := []struct {
		hours  float64
		reason string
		want   string
	}{
		{4, "", ""},
		{4.5, "", msg},
		{4.5, "   ", msg},
		{4.5, "123456789", msg},
		{4.5, " 123456789 ", msg},
		{4.5, "Stok opname akhir", ""},
	}
	for _, c := range cases {
		if got := OvertimeReasonError(c.hours, c.reason); got != c.want {
			t.Errorf("OvertimeReasonError(%v, %q) = %q, want %q", c.hours, c.reason, got, c.want)
		}
	}
}

// Review akhir EPIC-089 (Important 1): tanpa batas 12 jam, lembur lintas hari
// tidak boleh menabrak lembur hari sebelumnya/berikutnya atau jam masuk besok.
func TestOvertimeOverlapError(t *testing.T) {
	const date = "2026-11-09"
	cases := []struct {
		name       string
		start, end string
		prev, next *OvertimeSpan
		nextShift  string
		want       string
	}{
		{"melewati jam masuk besok", "18:00", "12:00", nil, nil, "09:00",
			"Lembur melewati jam masuk kerja 10 Nov 2026 (09:00)"},
		{"selesai sebelum jam masuk", "18:00", "08:30", nil, nil, "09:00", ""},
		{"tanpa lintas hari", "18:00", "22:00", nil, nil, "09:00", ""},
		{"menabrak lembur besok", "20:00", "08:00", nil, &OvertimeSpan{Start: "06:00", End: "10:00"}, "",
			"Lembur bertabrakan dengan lembur 10 Nov 2026 (06:00–10:00)"},
		{"menabrak lembur kemarin yang lintas hari", "06:00", "10:00", &OvertimeSpan{Start: "20:00", End: "08:00"}, nil, "",
			"Lembur bertabrakan dengan lembur 8 Nov 2026 (20:00–08:00)"},
		{"lembur kemarin tidak lintas hari", "06:00", "10:00", &OvertimeSpan{Start: "18:00", End: "22:00"}, nil, "", ""},
		{"bersambung tepat", "06:00", "10:00", &OvertimeSpan{Start: "22:00", End: "06:00"}, nil, "", ""},
	}
	for _, c := range cases {
		if got := OvertimeOverlapError(date, c.start, c.end, c.prev, c.next, c.nextShift); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

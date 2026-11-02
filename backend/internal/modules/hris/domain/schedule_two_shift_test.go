package domain

import (
	"strings"
	"testing"
	"time"
)

// EPIC-080 R6/R7/R9: two shifts in one day, coded KODE1+KODE2.

// twoShiftCtx is uctx with fixture shifts: PAGI4 08:00–12:00 and SORE4
// 17:00–21:00 without break (8 h net together), SPG2 11:00–19:00 (overlaps
// SPG1 08:00–16:00) and DINI 04:00–08:00.
func twoShiftCtx() ScheduleUploadContext {
	c := uctx()
	c.Shifts["PAGI4"] = UploadShift{ID: "p4", Code: "PAGI4", Start: "08:00:00", End: "12:00:00", Active: true}
	c.Shifts["SORE4"] = UploadShift{ID: "s4", Code: "SORE4", Start: "17:00:00", End: "21:00:00", Active: true}
	c.Shifts["SPG2"] = UploadShift{ID: "s5", Code: "SPG2", Start: "11:00:00", End: "19:00:00", Active: true, BreakMinutes: 60}
	c.Shifts["DINI"] = UploadShift{ID: "s6", Code: "DINI", Start: "04:00:00", End: "08:00:00", Active: true}
	return c
}

func TestParseScheduleCodeTwoShifts(t *testing.T) {
	for _, tc := range []struct{ in, first, second string }{
		{"PAGI4+SORE4", "PAGI4", "SORE4"},
		{" pagi4 + sore4 ", "pagi4", "sore4"},
		{"SPG1", "SPG1", ""},
		{"L", "L", ""},
		{"", "", ""},
	} {
		first, second, err := ParseScheduleCode(tc.in)
		if err != nil || first != tc.first || second != tc.second {
			t.Fatalf("ParseScheduleCode(%q) = (%q, %q, %v), want (%q, %q)", tc.in, first, second, err, tc.first, tc.second)
		}
	}
	for _, in := range []string{"L+SPG1", "SPG1+L", "off+SPG1", "-+SPG1", "SPG1+", "+SPG1", "A+B+C"} {
		if _, _, err := ParseScheduleCode(in); err == nil {
			t.Fatalf("ParseScheduleCode(%q) should fail", in)
		}
	}
}

const overlapMsg = "Shift kedua harus mulai setelah shift pertama selesai (SPG1 selesai 16.00, SPG2 mulai 11.00)"

func TestTwoShiftOverlapRejected(t *testing.T) {
	c := twoShiftCtx()
	res := ValidateScheduleUpload(c, sfile(c, row(4, "111", "L", "SPG1+SPG2", "SPG1", "SPG1", "SPG1", "SPG1", "L"),
		row(5, "222", week("SPG1")...)))
	if !hasIssue(res.Errors, overlapMsg) {
		t.Fatalf("want %q, got %+v", overlapMsg, res.Errors)
	}
	for _, a := range res.Assignments {
		if a.EmployeeID == "e1" {
			t.Fatalf("row with an error still assigned: %+v", a)
		}
	}
	// Per-cell save applies the same rule.
	cc := twoShiftCtx()
	cc.EmployeesByID = map[string]UploadEmployee{"e1": cc.Employees["111"]}
	cc.Current = map[string]map[string]string{"e1": {}}
	cres := ValidateScheduleCells(cc, []ScheduleCellInput{cell("e1", "2026-11-03", "spg1+spg2")})
	if !hasIssue(cres.Errors, overlapMsg) || len(cres.Assignments) != 0 {
		t.Fatalf("cells: errors %+v assignments %+v", cres.Errors, cres.Assignments)
	}
	// Unknown second code and L+… are errors too.
	res = ValidateScheduleUpload(c, sfile(c, row(4, "111", "L", "SPG1+XX", "L+SPG1", "SPG1", "SPG1", "SPG1", "L")))
	if !hasIssue(res.Errors, "Kode 'XX' tidak ada di sheet Kode Shift") || !hasIssue(res.Errors, "L+SPG1") {
		t.Fatalf("errors %+v", res.Errors)
	}
}

func TestTwoShiftIssue(t *testing.T) {
	sh := func(code, start, end string, overnight bool) UploadShift {
		return UploadShift{Code: code, Start: start + ":00", End: end + ":00", Overnight: overnight}
	}
	for _, tc := range []struct {
		name          string
		first, second UploadShift
		want          string
	}{
		{"berurutan", sh("PAGI4", "08:00", "12:00", false), sh("SORE4", "17:00", "21:00", false), ""},
		{"tepat setelah selesai", sh("PAGI4", "08:00", "12:00", false), sh("SIANG", "12:00", "16:00", false), ""},
		{"tumpang tindih", sh("SPG1", "08:00", "16:00", false), sh("SPG2", "11:00", "19:00", false), overlapMsg},
		{"shift sama", sh("PAGI4", "08:00", "12:00", false), sh("PAGI4", "08:00", "12:00", false),
			"Shift kedua harus mulai setelah shift pertama selesai (PAGI4 selesai 12.00, PAGI4 mulai 08.00)"},
		{"shift kedua lewat tengah malam", sh("SORE", "16:00", "20:00", false), sh("MALAM", "22:00", "02:00", true), ""},
		// Shift 2 may not start, by clock, before shift 1 starts (final review 1).
		{"shift kedua dini hari esoknya ditolak", sh("SORE", "18:00", "22:00", false), sh("DINI", "00:00", "04:00", false),
			"Shift kedua harus mulai setelah shift pertama selesai (SORE selesai 22.00, DINI mulai 00.00)"},
		{"urutan terbalik", sh("SORE4", "17:00", "21:00", false), sh("PAGI4", "06:00", "10:00", false),
			"Shift kedua harus mulai setelah shift pertama selesai (SORE4 selesai 21.00, PAGI4 mulai 06.00)"},
		{"shift kedua lewat tengah malam setelah shift sore", sh("SORE", "18:00", "22:00", false), sh("MALAM", "22:00", "02:00", true), ""},
		{"melewati mulai shift pertama esoknya", sh("SPG1", "08:00", "16:00", false), sh("PAGI", "07:00", "11:00", false),
			"Shift kedua harus mulai setelah shift pertama selesai (SPG1 selesai 16.00, PAGI mulai 07.00)"},
		{"setelah shift malam", sh("MALAM", "22:00", "06:00", true), sh("PAGI", "07:00", "09:00", false), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := TwoShiftIssue(tc.first, tc.second); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTwoShiftDayCountsBothForFortyHours(t *testing.T) {
	const ps = "PAGI4+SORE4"
	c := twoShiftCtx()
	res := ValidateScheduleUpload(c, sfile(c, row(4, "111", "L", ps, ps, ps, ps, ps, "L")))
	if len(res.Errors) != 0 {
		t.Fatalf("errors %+v", res.Errors)
	}
	if hasIssue(res.Warnings, "sebelum 40 jam kerja") {
		t.Fatalf("5 × 8 h = 40 h should not warn: %+v", res.Warnings)
	}
	// Both shifts are stored for each two-shift day.
	n := 0
	for _, a := range res.Assignments {
		if a.ShiftID != nil && *a.ShiftID == "p4" {
			if a.Shift2ID == nil || *a.Shift2ID != "s4" {
				t.Fatalf("assignment %+v: want shift2 s4", a)
			}
			n++
		}
	}
	if n != 5 {
		t.Fatalf("two-shift assignments = %d, want 5", n)
	}

	res = ValidateScheduleUpload(c, sfile(c, row(4, "111", "L", ps, ps, ps, ps, "L", "L")))
	want := "Ana libur 07/11/2026 sebelum 40 jam kerja (baru 32 jam sejak libur 02/11/2026)"
	if !hasIssue(res.Warnings, want) {
		t.Fatalf("want %q, got %+v", want, res.Warnings)
	}
}

func TestTwoShiftGapFromSecondShiftEnd(t *testing.T) {
	c := twoShiftCtx()
	// SORE4 ends 21:00, DINI starts 04:00 the next day: 7 h.
	res := ValidateScheduleUpload(c, sfile(c, row(4, "111", "L", "PAGI4+SORE4", "DINI", "L", "L", "L", "L")))
	want := "Ana: jeda 7 jam antara 03/11/2026 (PAGI4+SORE4) dan 04/11/2026 (DINI)"
	if !hasIssue(res.Warnings, want) {
		t.Fatalf("want %q, got %+v", want, res.Warnings)
	}
	// PAGI4 alone ends 12:00: 16 h, no gap warning.
	res = ValidateScheduleUpload(c, sfile(c, row(4, "111", "L", "PAGI4", "DINI", "L", "L", "L", "L")))
	if hasIssue(res.Warnings, "jeda") {
		t.Fatalf("unexpected gap warning %+v", res.Warnings)
	}
}

func TestTwoShiftCellsReadCurrentTwoShiftDays(t *testing.T) {
	c := twoShiftCtx()
	c.EmployeesByID = map[string]UploadEmployee{"e1": c.Employees["111"]}
	c.Current = map[string]map[string]string{"e1": {"2026-11-03": "PAGI4+SORE4"}}
	res := ValidateScheduleCells(c, []ScheduleCellInput{cell("e1", "2026-11-04", "DINI"), cell("e1", "2026-11-05", "pagi4+sore4")})
	if len(res.Errors) != 0 {
		t.Fatalf("errors %+v", res.Errors)
	}
	if !hasIssue(res.Warnings, "Ana: jeda 7 jam antara 03/11/2026 (PAGI4+SORE4) dan 04/11/2026 (DINI)") {
		t.Fatalf("current two-shift day not read: %+v", res.Warnings)
	}
	var got []string
	for _, a := range res.Assignments {
		s2 := ""
		if a.Shift2ID != nil {
			s2 = *a.Shift2ID
		}
		got = append(got, a.Date+"="+*a.ShiftID+"+"+s2)
	}
	if strings.Join(got, ",") != "2026-11-04=s6+,2026-11-05=p4+s4" {
		t.Fatalf("assignments %v", got)
	}
}

func TestTwoShiftWindow(t *testing.T) {
	pagi := ShiftTimes{StartTime: "08:00:00", EndTime: "12:00:00"}
	sore := ShiftTimes{StartTime: "17:00:00", EndTime: "21:00:00"}
	malam := ShiftTimes{StartTime: "22:00:00", EndTime: "06:00:00", IsOvernight: true}
	dini := ShiftTimes{StartTime: "04:00:00", EndTime: "08:00:00"}
	cases := []struct {
		name          string
		first, second ShiftTimes
		start, end    string
		gap           int
	}{
		{"same day", pagi, sore, "2026-10-06T01:00:00Z", "2026-10-06T14:00:00Z", 300},
		{"second overnight", pagi, malam, "2026-10-06T01:00:00Z", "2026-10-06T23:00:00Z", 600},
		// 04:00 starts before 08:00: the second shift runs the next day.
		{"second wraps to next day", pagi, dini, "2026-10-06T01:00:00Z", "2026-10-07T01:00:00Z", 960},
		{"back to back", pagi, ShiftTimes{StartTime: "12:00:00", EndTime: "16:00:00"}, "2026-10-06T01:00:00Z", "2026-10-06T09:00:00Z", 0},
	}
	for _, c := range cases {
		start, end, gap := TwoShiftWindow("2026-10-06", c.first, c.second)
		if got := start.Format(time.RFC3339); got != c.start {
			t.Errorf("%s: start = %s, want %s", c.name, got, c.start)
		}
		if got := end.Format(time.RFC3339); got != c.end {
			t.Errorf("%s: end = %s, want %s", c.name, got, c.end)
		}
		if gap != c.gap {
			t.Errorf("%s: gap = %d, want %d", c.name, gap, c.gap)
		}
	}
}

func TestESSTwoShiftDisplay(t *testing.T) {
	s := func(v string) *string { return &v }
	tol := int64(3)
	id1, id2 := "p4", "s4"
	row := func(dow int, second bool) ESSScheduleRow {
		r := ESSScheduleRow{ScheduleRow: ScheduleRow{DayOfWeek: dow, ShiftID: &id1, EffectiveFrom: "2026-01-01"},
			ShiftCode: s("PAGI4"), ShiftName: s("Pagi Empat"), StartTime: s("08:00:00"), EndTime: s("12:00:00"), LateToleranceMinutes: &tol}
		if second {
			r.Shift2ID, r.Shift2Code, r.Shift2Name = &id2, s("SORE4"), s("Sore Empat")
			r.Shift2Start, r.Shift2End = s("17:00:00"), s("21:00:00")
		}
		return r
	}
	// Monday has two shifts, Tuesday only the first: same first shift, so the
	// second shift must come from the winning row, not from any row.
	rows := []ESSScheduleRow{row(1, true), row(2, false)}

	mon := TodayShiftOf(rows, "2026-10-05")
	if mon.Name != "PAGI4 + SORE4" || *mon.StartTime != "08:00:00" || *mon.EndTime != "21:00:00" ||
		mon.TimeLabel != "08.00–12.00, 17.00–21.00" || mon.LateToleranceMinutes != 3 {
		t.Errorf("two-shift today = %+v", mon)
	}
	tue := TodayShiftOf(rows, "2026-10-06")
	if tue.Name != "Pagi Empat" || *tue.EndTime != "12:00:00" || tue.TimeLabel != "08.00–12.00" {
		t.Errorf("one-shift today = %+v", tue)
	}
	week := WeekScheduleOf(rows, "2026-10-07")
	if *week[0].ShiftName != "PAGI4 + SORE4" || *week[0].TimeLabel != "08.00–12.00, 17.00–21.00" || *week[1].ShiftName != "Pagi Empat" {
		t.Errorf("week = %+v / %+v", week[0], week[1])
	}
}

package domain

import "testing"

// cctx is uctx with the employees keyed by ID and Ana's current schedule
// for the week before and the week of the test range (all SPG1 except the
// default weekend libur), so edits are checked against their neighbours.
func cctx() ScheduleUploadContext {
	c := uctx()
	c.EmployeesByID = map[string]UploadEmployee{}
	for _, e := range c.Employees {
		c.EmployeesByID[e.ID] = e
	}
	c.Current = map[string]map[string]string{"e1": {}}
	return c
}

func cell(emp, date, code string) ScheduleCellInput {
	return ScheduleCellInput{EmployeeID: emp, Date: date, Code: code}
}

func TestValidateScheduleCells(t *testing.T) {
	cases := []struct {
		name    string
		edit    func(*ScheduleUploadContext)
		cells   []ScheduleCellInput
		err     string
		warn    string
		column  string
		empName string
	}{
		{name: "tanggal lewat", edit: func(c *ScheduleUploadContext) { c.Today = "2026-11-03" },
			cells: []ScheduleCellInput{cell("e1", "2026-11-02", "SPG1")}, err: "Tanggal 02/11/2026 sudah lewat", column: "Sen 02/11"},
		{name: "periode closing", edit: func(c *ScheduleUploadContext) { c.ClosedDates["2026-11-04"] = true },
			cells: []ScheduleCellInput{cell("e1", "2026-11-04", "SPG1")}, err: "Periode absensi 04/11/2026 sudah closing", column: "Rab 04/11"},
		{name: "tidak ditempatkan", cells: []ScheduleCellInput{cell("e5", "2026-11-03", "SPG1")},
			err: "Eka tidak ditempatkan di PPJA pada 03/11/2026", column: "Sel 03/11", empName: "Eka"},
		{name: "kode tidak ada", cells: []ScheduleCellInput{cell("e1", "2026-11-03", "XX")},
			err: "Kode 'XX' tidak ada di sheet Kode Shift", column: "Sel 03/11", empName: "Ana"},
		{name: "kode salah ketik", cells: []ScheduleCellInput{cell("e1", "2026-11-03", "SPG 1")},
			err: "Kode 'SPG 1' tidak ada; maksud Anda SPG1?"},
		{name: "shift nonaktif", cells: []ScheduleCellInput{cell("e1", "2026-11-03", "old")},
			err: "Shift OLD sudah nonaktif", empName: "Ana"},
		{name: "Non-Shift", cells: []ScheduleCellInput{cell("e3", "2026-11-03", "SPG1")},
			err: "Cici Non-Shift, pakai jadwal tetap", empName: "Cici"},
		{name: "karyawan nonaktif", cells: []ScheduleCellInput{cell("e4", "2026-11-03", "SPG1")},
			err: "NIP 444 tidak dikenal atau sudah nonaktif"},
		{name: "karyawan tidak dikenal", cells: []ScheduleCellInput{cell("e9", "2026-11-03", "SPG1")},
			err: "Karyawan tidak dikenal atau sudah nonaktif"},
		{name: "kotak ganda", cells: []ScheduleCellInput{cell("e1", "2026-11-03", "SPG1"), cell("e1", "2026-11-03", "L")},
			err: "Ana 03/11/2026 dikirim lebih dari sekali"},
		{name: "jeda dengan kotak tetangga", edit: func(c *ScheduleUploadContext) { c.Current["e1"]["2026-11-03"] = "SPG1" },
			cells: []ScheduleCellInput{cell("e1", "2026-11-02", "MALAM")},
			warn:  "Ana: jeda 2 jam antara 02/11/2026 (MALAM) dan 03/11/2026 (SPG1)"},
		{name: "7 hari gabungan edit + berlaku", edit: func(c *ScheduleUploadContext) {
			for _, d := range []string{"2026-11-02", "2026-11-03", "2026-11-04", "2026-11-05", "2026-11-06", "2026-11-07"} {
				c.Current["e1"][d] = "SPG1"
			}
			c.Current["e1"]["2026-11-01"] = "L"
		}, cells: []ScheduleCellInput{cell("e1", "2026-11-08", "SPG1")},
			warn: "Ana masuk 7 hari berturut-turut (02/11/2026–08/11/2026)"},
		{name: "libur nasional", edit: func(c *ScheduleUploadContext) { c.Holidays["2026-11-04"] = "Libur Uji" },
			cells: []ScheduleCellInput{cell("e1", "2026-11-04", "SPG1"), cell("e2", "2026-11-04", "L")},
			warn:  "04/11/2026 libur nasional (Libur Uji): 1 karyawan diberi shift"},
		{name: "cuti", edit: func(c *ScheduleUploadContext) { c.OnLeave["e1"] = map[string]bool{"2026-11-03": true} },
			cells: []ScheduleCellInput{cell("e1", "2026-11-03", "SPG1")}, warn: "Ana cuti 03/11/2026 tapi diberi SPG1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := cctx()
			if tc.edit != nil {
				tc.edit(&c)
			}
			res := ValidateScheduleCells(c, tc.cells)
			if tc.err != "" {
				if !hasIssue(res.Errors, tc.err) {
					t.Fatalf("want error %q, got %+v", tc.err, res.Errors)
				}
				for _, i := range res.Errors {
					if i.Line != 0 {
						t.Fatalf("cell issue line = %d, want 0", i.Line)
					}
					if tc.column != "" && i.Message == tc.err && i.Column != tc.column {
						t.Fatalf("column = %q, want %q", i.Column, tc.column)
					}
					if tc.empName != "" && i.Message == tc.err && i.Name != tc.empName {
						t.Fatalf("name = %q, want %q", i.Name, tc.empName)
					}
				}
				return
			}
			if len(res.Errors) != 0 {
				t.Fatalf("unexpected errors %+v", res.Errors)
			}
			if !hasIssue(res.Warnings, tc.warn) {
				t.Fatalf("want warning %q, got %+v", tc.warn, res.Warnings)
			}
		})
	}
}

func TestValidateScheduleCellsAssignments(t *testing.T) {
	c := cctx()
	res := ValidateScheduleCells(c, []ScheduleCellInput{
		cell("e1", "2026-11-03", ""), cell("e1", "2026-11-04", "off"), cell("e1", "2026-11-05", " l "),
		cell("e2", "2026-11-05", "spg1"),
	})
	if len(res.Errors) != 0 {
		t.Fatalf("errors %+v", res.Errors)
	}
	if len(res.EmployeeIDs) != 2 || res.EmployeeIDs[0] != "e1" || res.EmployeeIDs[1] != "e2" {
		t.Fatalf("employee ids %v", res.EmployeeIDs)
	}
	want := []struct {
		emp, date string
		shift     string
		clear     bool
	}{{"e1", "2026-11-03", "", true}, {"e1", "2026-11-04", "", false}, {"e1", "2026-11-05", "", false}, {"e2", "2026-11-05", "s1", false}}
	if len(res.Assignments) != len(want) {
		t.Fatalf("assignments %+v", res.Assignments)
	}
	for i, w := range want {
		a := res.Assignments[i]
		got := ""
		if a.ShiftID != nil {
			got = *a.ShiftID
		}
		if a.EmployeeID != w.emp || a.Date != w.date || got != w.shift || a.Clear != w.clear {
			t.Fatalf("assignment %d = %+v (shift %q), want %+v", i, a, got, w)
		}
	}
	// A cleared or libur cell is not a working day: no gap/streak noise.
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings %+v", res.Warnings)
	}
}

func TestScheduleCellWindow(t *testing.T) {
	w := ScheduleCellWindow([]string{"2026-11-04", "2026-11-02"})
	if len(w) != 2+2*maxStreak+1 || w[0] != "2026-10-27" || w[len(w)-1] != "2026-11-10" {
		t.Fatalf("window %v", w)
	}
}

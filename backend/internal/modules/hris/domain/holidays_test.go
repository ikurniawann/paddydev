package domain

import "testing"

// EPIC-079 R2/R3: staf shift tidak mengenal tanggal merah; Non-Shift libur di
// nasional/perusahaan, dan cuti bersama hanya bila ditetapkan libur.
func TestHolidayDayOff(t *testing.T) {
	cases := []struct {
		name string
		h    Holiday
		non  bool
	}{
		{"nasional", Holiday{Type: "nasional"}, true},
		{"perusahaan", Holiday{Type: "perusahaan"}, true},
		{"cuti bersama biasa", Holiday{Type: "cuti_bersama", IsDayOff: false}, false},
		{"cuti bersama ditetapkan libur", Holiday{Type: "cuti_bersama", IsDayOff: true}, true},
	}
	for _, c := range cases {
		eq(t, HolidayDayOff(c.h, "non_shift"), c.non)
		eq(t, HolidayDayOff(c.h, "shift"), false)
	}

	rows := []Holiday{
		{Date: "2026-12-24", Type: "cuti_bersama"},
		{Date: "2026-12-25", Type: "nasional"},
	}
	eq(t, DayOffDates(rows, "non_shift"), map[string]bool{"2026-12-25": true})
	eq(t, DayOffDates(rows, "shift"), map[string]bool{})

	both := []Holiday{
		{Date: "2026-12-25", Type: "cuti_bersama"},
		{Date: "2026-12-25", Type: "nasional"},
	}
	eq(t, DayOffDates(both, "non_shift"), map[string]bool{"2026-12-25": true})
}

func TestIsDayOffOnlyForCutiBersama(t *testing.T) {
	yes := true
	const msg = "Hanya cuti bersama yang bisa ditetapkan libur"
	eq(t, ValidateHolidayBody(HolidayInput{HolidayDate: sp("2026-12-25"), Name: sp("Natal"), Type: sp("nasional"), IsDayOff: &yes}), msg)
	eq(t, ValidateHolidayBody(HolidayInput{HolidayDate: sp("2026-12-25"), Name: sp("Natal"), IsDayOff: &yes}), msg)
	eq(t, ValidateHolidayBody(HolidayInput{HolidayDate: sp("2026-12-24"), Name: sp("CB"), Type: sp("cuti_bersama"), IsDayOff: &yes}), "")
	eq(t, ValidateHolidayPatch(HolidayInput{Type: sp("perusahaan"), IsDayOff: &yes}), msg)
	eq(t, ValidateHolidayPatch(HolidayInput{IsDayOff: &yes}), "") // tipe tersimpan dicek di store
	eq(t, ValidateImportItem(HolidayInput{HolidayDate: sp("2026-12-25"), Name: sp("Natal"), IsDayOff: &yes}), msg)
}

package domain

import "testing"

// EPIC-085 R4: lembur saat perjalanan dinas dibayar tanpa syarat clock-out.
func TestSplitOvertimeTripSkipsClockout(t *testing.T) {
	trip := []OvertimeRequest{{Date: "2026-11-10", Hours: 3, DayType: "rest_day", OnTrip: true}}
	got := SplitOvertimeWith(trip, nil, nil, nil, true)
	if got.Holiday != 3 {
		t.Fatalf("dinas: holiday = %v, want 3", got.Holiday)
	}
	plain := []OvertimeRequest{{Date: "2026-11-10", Hours: 3, DayType: "rest_day"}}
	if got := SplitOvertimeWith(plain, nil, nil, nil, true); got.Holiday != 0 {
		t.Fatalf("tanpa dinas & clock-out: holiday = %v, want 0", got.Holiday)
	}
}

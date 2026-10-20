package domain

import "testing"

func TestValidateBusinessTripDates(t *testing.T) {
	today := "2026-10-05"
	eq(t, ValidateBusinessTripDates("2026-10-05", "2026-10-07", today), "")
	eq(t, ValidateBusinessTripDates("2026-10-05", "2026-11-04", today), "") // 31 hari
	eq(t, ValidateBusinessTripDates("2026-10-05", "2026-11-05", today), "Perjalanan Dinas maksimal 31 hari per pengajuan")
	eq(t, ValidateBusinessTripDates("2026-10-04", "2026-10-05", today), "Tanggal mulai tidak boleh sebelum hari ini")
	eq(t, ValidateBusinessTripDates("2026-10-07", "2026-10-06", today), "Tanggal selesai tidak boleh sebelum tanggal mulai")
	eq(t, ValidateBusinessTripDates("2026-02-31", "2026-03-01", today), "Format tanggal tidak valid")
}

func TestCanDecideBusinessTrip(t *testing.T) {
	emp, mgr, hr, other := "emp-1", "mgr-1", "hr-1", "emp-2"
	ref := func(status string) TripRef { return TripRef{EmployeeID: emp, ReportingTo: &mgr, Status: status} }
	ok := func(a TripActor, r TripRef, action string) bool {
		v, _ := CanDecideBusinessTrip(a, r, action)
		return v
	}
	hrA, mgrA, selfA, otherA := TripActor{&hr, true}, TripActor{&mgr, false}, TripActor{&emp, false}, TripActor{&other, false}
	eq(t, ok(mgrA, ref("pending"), "approve"), true)
	eq(t, ok(hrA, ref("pending"), "reject"), true)
	eq(t, ok(TripActor{nil, true}, ref("pending"), "approve"), true) // HRD tanpa data karyawan
	eq(t, ok(otherA, ref("pending"), "approve"), false)
	allowed, reason := CanDecideBusinessTrip(TripActor{&emp, true}, ref("pending"), "approve")
	eq(t, allowed, false)
	eq(t, reason, "Tidak bisa memutuskan Perjalanan Dinas sendiri")
	eq(t, ok(selfA, ref("pending"), "cancel"), true)
	eq(t, ok(selfA, ref("approved"), "cancel"), false)
	eq(t, ok(hrA, ref("approved"), "cancel"), true)
	eq(t, ok(mgrA, ref("pending"), "cancel"), false)
	eq(t, ok(hrA, ref("rejected"), "cancel"), false)
	eq(t, ok(hrA, ref("pending"), "delete"), false)
}

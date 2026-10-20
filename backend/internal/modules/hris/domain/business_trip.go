package domain

import "time"

// Perjalanan Dinas (EPIC-062, MOM AB-02): an employee request over a date
// range, decided by the direct manager or HR; approved dates are exempt
// from the attendance geofence.

// BusinessTripMaxDays is the longest request, inclusive (EPIC-062 R7).
const BusinessTripMaxDays = 31

// ValidateBusinessTripDates: real dates, start not before today (WIB), end
// not before start, at most BusinessTripMaxDays days; "" when valid.
func ValidateBusinessTripDates(start, end, today string) string {
	s, err1 := time.Parse(DateLayout, start)
	e, err2 := time.Parse(DateLayout, end)
	if err1 != nil || err2 != nil {
		return "Format tanggal tidak valid"
	}
	if start < today {
		return "Tanggal mulai tidak boleh sebelum hari ini"
	}
	if e.Before(s) {
		return "Tanggal selesai tidak boleh sebelum tanggal mulai"
	}
	if int(e.Sub(s).Hours()/24)+1 > BusinessTripMaxDays {
		return "Perjalanan Dinas maksimal 31 hari per pengajuan"
	}
	return ""
}

// TripActor is who decides.
type TripActor struct {
	EmployeeID *string
	IsHR       bool
}

// TripRef is the trip being decided.
type TripRef struct {
	EmployeeID  string
	ReportingTo *string
	Status      string
}

// CanDecideBusinessTrip: approve/reject by the direct manager or HR, never
// the traveller (Q-APV-3 pattern); cancel by the traveller while pending,
// or by HR while pending or approved. "" reason means allowed.
func CanDecideBusinessTrip(a TripActor, t TripRef, action string) (bool, string) {
	self := samePerson(a.EmployeeID, &t.EmployeeID)
	switch action {
	case "cancel":
		if (t.Status == "pending" && (self || a.IsHR)) || (t.Status == "approved" && a.IsHR) {
			return true, ""
		}
		return false, "Hanya pemohon (saat menunggu) atau HRD yang bisa membatalkan Perjalanan Dinas"
	case "approve", "reject":
		if self {
			return false, "Tidak bisa memutuskan Perjalanan Dinas sendiri"
		}
		if !a.IsHR && !samePerson(a.EmployeeID, t.ReportingTo) {
			return false, "Hanya atasan langsung atau HRD yang bisa memproses pengajuan ini"
		}
		return true, ""
	}
	return false, "Aksi tidak dikenal"
}

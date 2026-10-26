package domain

import "fmt"

// GapDigestNotice is the in-app notification the HR receives every morning
// about the previous day's missed attendance (EPIC-067 R7). date is that
// previous day; the last sentence appears only when some employees had no
// schedule and were therefore not counted as missing.
func GapDigestNotice(date string, missing, noClockOut, unscheduled int) (title, message string) {
	day := ""
	if t, ok := ParseDate(date); ok {
		day = dayShort[t.Weekday()] + ", "
	}
	message = fmt.Sprintf("Kemarin (%s%s): %d karyawan belum absen, %d belum absen pulang.",
		day, ShortDateID(date), missing, noClockOut)
	if unscheduled > 0 {
		message += fmt.Sprintf(" %d karyawan tanpa jadwal tidak dihitung.", unscheduled)
	}
	return "Ringkasan Absen Terlewat", message
}

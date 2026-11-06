package domain

import "fmt"

// LateNotice is the bell text of a late clock-in (EPIC-094): the warning for
// the employee and the notice for their approvers and HR. count is the late
// days of the attendance period, today included.
func LateNotice(name string, minutes, count int) (selfTitle, selfMsg, teamTitle, teamMsg string) {
	selfTitle = "Anda terlambat hari ini"
	selfMsg = fmt.Sprintf("Anda terlambat %d menit hari ini (%d kali di periode ini). "+
		"Hati-hati, jika berlanjut akan ada teguran dari HR.", minutes, count)
	teamTitle = name + " terlambat hari ini"
	teamMsg = fmt.Sprintf("%s terlambat %d menit hari ini (%d kali di periode ini).", name, minutes, count)
	return selfTitle, selfMsg, teamTitle, teamMsg
}

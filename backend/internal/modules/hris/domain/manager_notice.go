package domain

import (
	"fmt"
	"strconv"
	"strings"
)

// Lonceng in-app ke atasan langsung saat cuti/izin atau lembur diajukan
// (EPIC-088 R11, permintaan user 05-11-2026). WhatsApp ke atasan tetap jalan.

const managerNoticeTail = " Tinjau di Area Karyawan → Persetujuan Tim."

// TeamApprovalsLink is the ESS page where the manager decides.
const TeamApprovalsLink = "/dashboard/me/persetujuan-tim"

// LeaveManagerNotice is the bell title and message of a leave request.
func LeaveManagerNotice(employeeName, leaveType, start, end string, days int) (title, message string) {
	span := ShortDateID(start)
	if end != "" && end != start {
		span += " s.d. " + ShortDateID(end)
	}
	return "Pengajuan cuti: " + employeeName, fmt.Sprintf("%s mengajukan %s %s (%d hari).%s",
		employeeName, LeaveTypeLabel(leaveType), span, days, managerNoticeTail)
}

// OvertimeManagerNotice is the bell title and message of an overtime request.
func OvertimeManagerNotice(employeeName, date, start, end string, hours float64) (title, message string) {
	h := strings.Replace(strconv.FormatFloat(hours, 'f', -1, 64), ".", ",", 1)
	return "Pengajuan lembur: " + employeeName, fmt.Sprintf("%s mengajukan lembur %s %s–%s (%s jam).%s",
		employeeName, ShortDateID(date), clockHM(start), clockHM(end), h, managerNoticeTail)
}

// clockHM trims "17:00:00" to "17:00".
func clockHM(s string) string {
	if len(s) > 5 {
		return s[:5]
	}
	return s
}

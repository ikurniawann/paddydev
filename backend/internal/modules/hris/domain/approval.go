package domain

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Approval atasan langsung (EPIC-063, MOM AP-01..AP-03, Q-APV-3, K-13).

// Decision capacities stored in decided_as.
const (
	DecidedAsManager = "manager"
	DecidedAsHR      = "hr"
)

// Approver is who decides a leave or overtime request. IsHR carries the
// controller ruling C1: callers pass Actor.IsHRD (super_admin/admin/hrd), not
// Actor.IsHR (which also includes hiring_manager).
type Approver struct {
	EmployeeID *string
	IsHR       bool
}

// DecideAs is the capacity an approver decides ownerID's request in (Q-APV-3,
// EPIC-063 R1/R2): "manager" for the direct manager (reportingTo), else "hr"
// for HR, the backup that may also override. Nobody decides their own
// request, and a reporting_to pointing at the owner never makes the owner a
// manager. decidedAs is "" with the refusal otherwise; noun names the
// request ("cuti", "lembur").
func DecideAs(a Approver, ownerID string, reportingTo *string, noun string) (decidedAs, refusal string) {
	if samePerson(a.EmployeeID, &ownerID) {
		return "", "Tidak bisa memutuskan pengajuan " + noun + " sendiri"
	}
	if samePerson(a.EmployeeID, reportingTo) {
		return DecidedAsManager, ""
	}
	if a.IsHR {
		return DecidedAsHR, ""
	}
	return "", "Hanya atasan langsung atau HRD yang bisa memproses pengajuan ini"
}

// OvertimeStartsInPast is MOM AP-02 (EPIC-063 R3): date + start ("HH:MM")
// on the WIB wall clock is before the start of the current WIB minute. A
// start in the current minute has not passed; unparsable input is left to
// the input validation (false).
func OvertimeStartsInPast(date, start string, now time.Time) bool {
	t, err := time.Parse("2006-01-02 15:04", date+" "+start)
	if err != nil {
		return false
	}
	return t.Before(NowWIB(now).Truncate(time.Minute))
}

// Overtime day types (K-13): Weekday = regular, Weekend / Libur = rest_day.
const (
	OvertimeWeekday = "regular"
	OvertimeWeekend = "rest_day"
)

// OvertimeDayTypeFor suggests the overtime type of date with the rule payroll
// pays by (payroll/domain SplitOvertimeWith, EPIC-063 R5): an active public
// holiday, or a day without a shift in a schedule in force, is Weekend /
// Libur; a scheduled shift is Weekday. known is false when no schedule is in
// force on date and it is not a holiday.
func OvertimeDayTypeFor(date string, holiday bool, schedule []ScheduleRow) (dayType string, known bool) {
	if holiday {
		return OvertimeWeekend, true
	}
	inForce := false
	for _, r := range schedule {
		if r.EffectiveFrom <= date && (r.EffectiveTo == nil || *r.EffectiveTo >= date) {
			inForce = true
			break
		}
	}
	if !inForce {
		return "", false
	}
	if i := ResolveScheduleRow(schedule, date); i < 0 || schedule[i].ShiftID == nil {
		return OvertimeWeekend, true
	}
	return OvertimeWeekday, true
}

// OvertimeDayTypeLabel is the ESS label of a day type.
func OvertimeDayTypeLabel(dayType string) string {
	if dayType == OvertimeWeekend {
		return "Weekend / Libur"
	}
	return "Weekday"
}

// ApprovalWaFooter points the direct manager to the ESS queue (EPIC-063 R8).
const ApprovalWaFooter = "Mohon ditinjau di dashboard Paddy OS (Area Karyawan → Persetujuan Tim)."

// OvertimeRequestWa is the input of the manager's overtime notification.
type OvertimeRequestWa struct {
	EmployeeName, Date, StartTime, EndTime, DayType string
	Hours                                           float64
	Reason, ApproverName                            *string
}

// OvertimeRequestWaMessage tells the direct manager an overtime request waits
// for them (EPIC-063 R8).
func OvertimeRequestWaMessage(in OvertimeRequestWa) string {
	lines := []string{"Paddy OS — Pengajuan Lembur"}
	if in.ApproverName != nil && *in.ApproverName != "" {
		lines = append(lines, "Kepada: "+*in.ApproverName)
	}
	hours := strings.ReplaceAll(strconv.FormatFloat(in.Hours, 'f', -1, 64), ".", ",")
	lines = append(lines, "Karyawan : "+in.EmployeeName,
		fmt.Sprintf("Tanggal : %s, %s–%s (%s jam, %s)", tanggalID(in.Date), in.StartTime, in.EndTime,
			hours, OvertimeDayTypeLabel(in.DayType)))
	if in.Reason != nil && JSTrim(*in.Reason) != "" {
		lines = append(lines, "Alasan : "+JSTrim(*in.Reason))
	}
	lines = append(lines, "", ApprovalWaFooter)
	return strings.Join(lines, "\n")
}

package domain

import (
	"fmt"
	"time"
)

// Aturan pengajuan cuti TnC 03-11-2026 (EPIC-084). Setiap fungsi
// mengembalikan pesan penolakan, atau "" bila lolos.

// LeaveTimingError is R2: LeadDays n needs start − today ≥ n days;
// SameDayOnly needs start = today. today is the WIB date.
func LeaveTimingError(lt LeaveType, start, today string) string {
	if lt.SameDayOnly {
		if start != today {
			return lt.Label + " diajukan pada hari sakit (tanggal mulai = hari ini)"
		}
		return ""
	}
	if lt.LeadDays <= 0 {
		return ""
	}
	t, ok := ParseDate(today)
	if !ok {
		return ""
	}
	earliest := t.AddDate(0, 0, lt.LeadDays)
	if start < earliest.Format(DateLayout) {
		return fmt.Sprintf("%s diajukan paling lambat H-%d (mulai %s atau sesudahnya)",
			lt.Label, lt.LeadDays, earliest.Format("02-01-2006"))
	}
	return ""
}

// LeaveSizeError is the MaxDays (working days) and MaxSpanDays (calendar
// days, both ends included) limit of one request.
func LeaveSizeError(lt LeaveType, start, end string, workDays int) string {
	if lt.MaxDays > 0 && workDays > lt.MaxDays {
		return fmt.Sprintf("%s maksimal %d hari kerja", lt.Label, lt.MaxDays)
	}
	if lt.MaxSpanDays > 0 {
		s, ok1 := ParseDate(start)
		e, ok2 := ParseDate(end)
		if ok1 && ok2 && int(e.Sub(s).Hours()/24)+1 > lt.MaxSpanDays {
			return fmt.Sprintf("%s maksimal %d hari kalender", lt.Label, lt.MaxSpanDays)
		}
	}
	return ""
}

// LeaveGenderError refuses a gender-bound type only when the employee's
// gender is filled in and differs (most records have no gender yet).
func LeaveGenderError(lt LeaveType, gender string) string {
	if lt.Gender == "" || gender == "" || gender == lt.Gender {
		return ""
	}
	if lt.Gender == "female" {
		return lt.Label + " hanya untuk karyawan perempuan"
	}
	return lt.Label + " hanya untuk karyawan laki-laki"
}

// FirstContract is the earliest non-draft employment contract ("" = empty).
type FirstContract struct{ EndDate, ProbationEnd string }

// BirthdayInYear is the employee's birthday in year; 29 Feb falls on 28 Feb
// in a non-leap year.
func BirthdayInYear(birthDate string, year int) (string, bool) {
	b, ok := ParseDate(birthDate)
	if !ok {
		return "", false
	}
	month, day := b.Month(), b.Day()
	if month == time.February && day == 29 && !isLeapYear(year) {
		day = 28
	}
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC).Format(DateLayout), true
}

// InitialContractPassed is R4: the first contract's end date (or, without
// one, its probation end) lies before onDate. Without any contract only a
// permanent employee qualifies.
func InitialContractPassed(first *FirstContract, employmentStatus, onDate string) bool {
	switch {
	case first == nil:
		return employmentStatus == "permanent"
	case first.EndDate != "":
		return first.EndDate < onDate
	default:
		return first.ProbationEnd == "" || first.ProbationEnd < onDate
	}
}

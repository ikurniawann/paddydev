package domain

import (
	"strings"
	"testing"
	"time"
)

func TestDecideAs(t *testing.T) {
	boss, hr, crew := sp("boss"), sp("hr"), "crew"
	check := func(name string, a Approver, reportingTo *string, as, refusal string) {
		t.Helper()
		gotAs, gotRefusal := DecideAs(a, crew, reportingTo, "cuti")
		if gotAs != as || gotRefusal != refusal {
			t.Errorf("%s: got (%q, %q), want (%q, %q)", name, gotAs, gotRefusal, as, refusal)
		}
	}
	check("atasan langsung", Approver{EmployeeID: boss}, boss, DecidedAsManager, "")
	check("atasan yang juga HR tetap manager", Approver{EmployeeID: boss, IsHR: true}, boss, DecidedAsManager, "")
	check("HR cadangan, pemohon tanpa atasan", Approver{EmployeeID: hr, IsHR: true}, nil, DecidedAsHR, "")
	check("HR override", Approver{EmployeeID: hr, IsHR: true}, boss, DecidedAsHR, "")
	check("HR tanpa data karyawan", Approver{IsHR: true}, boss, DecidedAsHR, "")
	check("bukan atasan, bukan HR", Approver{EmployeeID: sp("lain")}, boss, "",
		"Hanya atasan langsung atau HRD yang bisa memproses pengajuan ini")
	check("HR untuk pengajuannya sendiri", Approver{EmployeeID: sp(crew), IsHR: true}, boss, "",
		"Tidak bisa memutuskan pengajuan cuti sendiri")
	check("reporting_to menunjuk diri sendiri", Approver{EmployeeID: sp(crew)}, sp(crew), "",
		"Tidak bisa memutuskan pengajuan cuti sendiri")
}

func TestOvertimeStartsInPast(t *testing.T) {
	now := time.Date(2026, 10, 5, 3, 0, 30, 0, time.UTC)           // Senin 10:00:30 WIB
	eq(t, OvertimeStartsInPast("2026-10-05", "10:00", now), false) // menit berjalan masih boleh
	eq(t, OvertimeStartsInPast("2026-10-05", "09:59", now), true)
	eq(t, OvertimeStartsInPast("2026-10-04", "23:00", now), true)
	eq(t, OvertimeStartsInPast("2026-10-06", "00:00", now), false)
	eq(t, OvertimeStartsInPast("2026-10-05", "1000", now), false) // format diserahkan ke validasi input
	late := time.Date(2026, 10, 5, 16, 45, 0, 0, time.UTC)        // 23:45 WIB, tanggal UTC masih 05-10
	eq(t, OvertimeStartsInPast("2026-10-06", "00:15", late), false)
	eq(t, OvertimeStartsInPast("2026-10-05", "23:30", late), true)
	eq(t, OvertimeStartsInPast("2026-10-05", "23:45", late), false)
}

func TestOvertimeDayTypeFor(t *testing.T) {
	mondays := []ScheduleRow{{DayOfWeek: 1, ShiftID: sp("pagi"), EffectiveFrom: "2026-01-01"}}
	check := func(name, date string, holiday bool, rows []ScheduleRow, want string, known bool) {
		t.Helper()
		got, ok := OvertimeDayTypeFor(date, holiday, rows)
		if got != want || ok != known {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", name, got, ok, want, known)
		}
	}
	check("Senin ada shift", "2026-10-12", false, mondays, OvertimeWeekday, true)
	check("Minggu tanpa baris jadwal", "2026-10-11", false, mondays, OvertimeWeekend, true)
	check("libur nasional di hari kerja", "2026-10-12", true, mondays, OvertimeWeekend, true)
	check("libur nasional tanpa jadwal", "2026-10-12", true, nil, OvertimeWeekend, true)
	check("tanpa jadwal", "2026-10-12", false, nil, "", false)
	check("jadwal belum berlaku", "2025-12-29", false, mondays, "", false)
	dayOff := append([]ScheduleRow{{DayOfWeek: 1, EffectiveFrom: "2026-10-12", EffectiveTo: sp("2026-10-12")}}, mondays...)
	check("libur per tanggal mengalahkan pola", "2026-10-12", false, dayOff, OvertimeWeekend, true)
	eq(t, OvertimeDayTypeLabel(OvertimeWeekday), "Weekday")
	eq(t, OvertimeDayTypeLabel(OvertimeWeekend), "Weekend / Libur")
}

func TestOvertimeRequestWaMessage(t *testing.T) {
	msg := OvertimeRequestWaMessage(OvertimeRequestWa{EmployeeName: "Crew Lembur", Date: "2026-10-06",
		StartTime: "17:00", EndTime: "19:30", Hours: 2.5, DayType: OvertimeWeekday,
		Reason: sp(" Stock opname "), ApproverName: sp("Atasan Lembur")})
	eq(t, msg, "Paddy OS — Pengajuan Lembur\nKepada: Atasan Lembur\nKaryawan : Crew Lembur\n"+
		"Tanggal : Sel, 06 Okt 2026, 17:00–19:30 (2,5 jam, Weekday)\nAlasan : Stock opname\n\n"+ApprovalWaFooter)
	leave := LeaveRequestWaMessage(LeaveRequestWa{EmployeeName: "Crew", LeaveType: "annual",
		StartDate: "2026-10-06", EndDate: "2026-10-06", TotalDays: 1})
	if !strings.HasSuffix(leave, "\n\n"+ApprovalWaFooter) {
		t.Fatalf("pesan cuti harus mengarah ke Persetujuan Tim: %q", leave)
	}
}

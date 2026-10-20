package domain

import (
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

func sp(s string) *string { return &s }

func query(raw string) Query {
	v, _ := url.ParseQuery(raw)
	return func(k string) (string, bool) {
		if !v.Has(k) {
			return "", false
		}
		return v.Get(k), true
	}
}

func eq[T any](t *testing.T, got, want T) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestCalendar(t *testing.T) {
	eq(t, TodayWIB(time.Date(2026, 10, 3, 18, 0, 0, 0, time.UTC)), "2026-10-04")
	eq(t, TodayWIB(time.Date(2026, 10, 3, 16, 0, 0, 0, time.UTC)), "2026-10-03")
	eq(t, IsoDayOfWeek("2026-07-20"), 1)
	eq(t, IsoDayOfWeek("2026-10-11"), 7)
	eq(t, EachDateISO("2026-07-30", "2026-08-02"), []string{"2026-07-30", "2026-07-31", "2026-08-01", "2026-08-02"})
	eq(t, EachDateISO("2026-08-19", "2026-08-17"), []string{})
	eq(t, IntOr("abc", 20), 20)
	eq(t, IntOr("0", 1), 1)
	eq(t, IntOr("12px", 1), 12)
	eq(t, JSNumber(""), 0.0)
	eq(t, JSNumber(" 45.9 "), 45.9)
	eq(t, IsFinite(JSNumber("abc")), false)
}

func TestSchedule(t *testing.T) {
	pagi, malam := "shift-pagi", "shift-malam"
	row := func(dow int, shift *string, from string, to *string) ScheduleRow {
		return ScheduleRow{DayOfWeek: dow, ShiftID: shift, EffectiveFrom: from, EffectiveTo: to}
	}
	rows := []ScheduleRow{row(1, &pagi, "2026-01-01", nil), row(2, &malam, "2026-01-01", nil)}
	eq(t, *rows[ResolveScheduleRow(rows, "2026-07-20")].ShiftID, pagi)
	eq(t, *rows[ResolveScheduleRow(rows, "2026-07-21")].ShiftID, malam)

	latest := []ScheduleRow{row(1, &pagi, "2026-01-01", nil), row(1, &malam, "2026-07-01", nil)}
	eq(t, *latest[ResolveScheduleRow(latest, "2026-07-20")].ShiftID, malam)
	eq(t, *latest[ResolveScheduleRow(latest, "2026-03-02")].ShiftID, pagi)

	gap := []ScheduleRow{row(1, &pagi, "2026-01-01", sp("2026-06-30")), row(1, &malam, "2026-09-01", nil)}
	eq(t, ResolveScheduleRow(gap, "2026-07-20"), -1)
	off := []ScheduleRow{row(1, nil, "2026-01-01", nil)}
	if i := ResolveScheduleRow(off, "2026-07-20"); i != 0 || off[i].ShiftID != nil {
		t.Fatal("day-off row must win")
	}

	day := ShiftTimes{StartTime: "08:00:00", EndTime: "16:00:00", LateToleranceMinutes: 10}
	night := ShiftTimes{StartTime: "22:00:00", EndTime: "06:00:00", IsOvernight: true, LateToleranceMinutes: 10}
	s, e := ScheduledWindow("2026-07-20", day)
	eq(t, s.Format(time.RFC3339), "2026-07-20T01:00:00Z")
	eq(t, e.Format(time.RFC3339), "2026-07-20T09:00:00Z")
	s, e = ScheduledWindow("2026-07-20", night)
	eq(t, s.Format(time.RFC3339), "2026-07-20T15:00:00Z")
	eq(t, e.Format(time.RFC3339), "2026-07-20T23:00:00Z")

	eq(t, IsOverdue(time.Date(2026, 7, 17, 1, 5, 0, 0, time.UTC), "2026-07-17", &day, RosterBelumAbsen), false)
	eq(t, IsOverdue(time.Date(2026, 7, 17, 1, 11, 0, 0, time.UTC), "2026-07-17", &day, RosterBelumAbsen), true)
	eq(t, IsOverdue(time.Date(2026, 7, 17, 5, 0, 0, 0, time.UTC), "2026-07-17", &day, RosterHadir), false)
	eq(t, IsOverdue(time.Date(2026, 7, 17, 5, 0, 0, 0, time.UTC), "2026-07-17", nil, RosterBelumAbsen), false)
	eq(t, IsOverdue(time.Date(2026, 7, 17, 15, 15, 0, 0, time.UTC), "2026-07-17", &night, RosterBelumAbsen), true)
}

func TestRosterStatus(t *testing.T) {
	shift := "shift-1"
	base := RosterStatusInput{HasSchedule: true, ShiftID: &shift}
	with := func(f func(*RosterStatusInput)) RosterStatusInput { in := base; f(&in); return in }
	cases := []struct {
		in   RosterStatusInput
		want string
	}{
		{with(func(i *RosterStatusInput) { i.HasAttendance = true }), RosterHadir},
		{with(func(i *RosterStatusInput) { i.HasAttendance, i.IsLate = true, true }), RosterTerlambat},
		{with(func(i *RosterStatusInput) { i.HasAttendance, i.OnApprovedLeave = true, true }), RosterHadir},
		{with(func(i *RosterStatusInput) { i.OnApprovedLeave, i.ShiftID = true, nil }), RosterCuti},
		{base, RosterBelumAbsen},
		{with(func(i *RosterStatusInput) { i.IsPastDate = true }), RosterAbsen},
		{with(func(i *RosterStatusInput) { i.ShiftID, i.IsPastDate = nil, true }), RosterLibur},
		{with(func(i *RosterStatusInput) { i.HasSchedule, i.ShiftID = false, nil }), RosterTanpaJadwal},
		{with(func(i *RosterStatusInput) { i.IsPublicHoliday, i.IsPastDate = true, true }), RosterLiburNasional},
		{with(func(i *RosterStatusInput) { i.IsPublicHoliday, i.OnApprovedLeave = true, true }), RosterCuti},
		{with(func(i *RosterStatusInput) { i.IsPublicHoliday, i.HasSchedule, i.ShiftID = true, false, nil }), RosterLiburNasional},
	}
	for i, c := range cases {
		if got := DeriveRosterStatus(c.in); got != c.want {
			t.Fatalf("case %d: got %s, want %s", i, got, c.want)
		}
	}
}

func TestLeaveDays(t *testing.T) {
	idx := IndexHolidays([]Holiday{
		{Date: "2026-03-20", Name: "Cuti Bersama Idul Fitri", Type: "cuti_bersama", DeductsLeave: true},
		{Date: "2026-03-21", Name: "Hari Raya Idul Fitri 1447 H", Type: "nasional"},
		{Date: "2026-03-23", Name: "Cuti Bersama Idul Fitri (2)", Type: "cuti_bersama", DeductsLeave: true},
		{Date: "2026-03-24", Name: "Cuti Bersama Idul Fitri (3)", Type: "cuti_bersama", DeductsLeave: true},
		{Date: "2026-08-17", Name: "Hari Kemerdekaan Republik Indonesia", Type: "nasional"},
	})
	days := func(a, b string) int { n, _ := DescribeLeaveDays(a, b, idx); return n }
	eq(t, days("2026-08-10", "2026-08-14"), 5)
	eq(t, days("2026-08-14", "2026-08-17"), 1)
	eq(t, days("2026-08-17", "2026-08-19"), 2)
	eq(t, days("2026-03-20", "2026-03-20"), 1)
	eq(t, days("2026-03-20", "2026-03-24"), 3)
	eq(t, days("2026-08-15", "2026-08-17"), 0)
	_, excluded := DescribeLeaveDays("2026-03-20", "2026-03-24", idx)
	eq(t, excluded, []ExcludedHoliday{{Date: "2026-03-21", Name: "Hari Raya Idul Fitri 1447 H"}})
	n, excluded := DescribeLeaveDays("2026-08-19", "2026-08-17", idx)
	eq(t, n, 0)
	eq(t, excluded, []ExcludedHoliday{})
	clash := IndexHolidays([]Holiday{
		{Date: "2026-09-01", Name: "Libur X", Type: "nasional"},
		{Date: "2026-09-01", Name: "Cuti Bersama X", Type: "cuti_bersama", DeductsLeave: true},
	})
	n, _ = DescribeLeaveDays("2026-09-01", "2026-09-01", clash)
	eq(t, n, 0)
	eq(t, NoWorkingDayMessage(nil), "Rentang tanggal ini jatuh pada akhir pekan — tidak perlu mengajukan cuti")
}

func TestHolidayInput(t *testing.T) {
	eq(t, ValidateHolidayBody(HolidayInput{Name: sp("HUT RI")}), "Tanggal wajib diisi (YYYY-MM-DD)")
	eq(t, ValidateHolidayBody(HolidayInput{HolidayDate: sp("2026-13-45"), Name: sp("X")}), "Tanggal tidak valid")
	eq(t, ValidateHolidayBody(HolidayInput{HolidayDate: sp("2026-08-17"), Name: sp(" ")}), "Nama libur wajib diisi")
	eq(t, ValidateHolidayBody(HolidayInput{HolidayDate: sp("2026-08-17"), Name: sp("HUT"), Type: sp("regional")}), "Tipe libur tidak valid")
	eq(t, ValidateHolidayBody(HolidayInput{HolidayDate: sp("2026-08-17"), Name: sp("HUT"), Status: sp("batal")}), "Status tidak valid")
	eq(t, ValidateHolidayBody(HolidayInput{HolidayDate: sp("2026-08-17"), Name: sp("HUT RI"), Type: sp("nasional")}), "")
	eq(t, ValidateHolidayPatch(HolidayInput{}), "")
	eq(t, ValidateHolidayPatch(HolidayInput{HolidayDate: sp("17/08/2026")}), "Tanggal tidak valid (YYYY-MM-DD)")
	eq(t, ValidateHolidayPatch(HolidayInput{Name: sp("")}), "Nama libur wajib diisi")
	eq(t, ValidateImportItem(HolidayInput{Name: sp("X")}), "Tanggal tidak valid")
	eq(t, ValidateImportItem(HolidayInput{HolidayDate: sp("2026-01-01")}), "Nama libur wajib diisi")
	eq(t, DefaultDeductsLeave("cuti_bersama"), true)
	eq(t, HolidayTypeOrDefault(nil), "nasional")
	eq(t, HolidayTypeOrDefault(sp("perusahaan")), "perusahaan")

	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	s, e, ok := ResolveHolidayRange(nil, nil, "", now)
	eq(t, []any{s, e, ok}, []any{"2026-01-01", "2026-12-31", true})
	s, e, _ = ResolveHolidayRange(nil, nil, "2027", now)
	eq(t, []any{s, e}, []any{"2027-01-01", "2027-12-31"})
	_, _, ok = ResolveHolidayRange(sp("2026-01-01"), nil, "", now)
	eq(t, ok, false)
	eq(t, ResolveImportYear("20xx", now), 2026)
}

const sampleICS = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\n" +
	"BEGIN:VEVENT\r\nDTSTART;VALUE=DATE:20260817\r\nDTEND;VALUE=DATE:20260818\r\nUID:20260817_hari-kemerdekaan@google.com\r\nSUMMARY:Hari Kemerdekaan Republik Indonesia\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nDTSTART;VALUE=DATE:20260320\r\nUID:20260320_cuti-bersama@google.com\r\nSUMMARY:Cuti Bersama Idul Fitri\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nDTSTART;VALUE=DATE:20260531\r\nUID:20260531_waisak@google.com\r\nSUMMARY:Hari Raya Waisak 2570 B\r\n E (belum pasti)\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nDTSTART;VALUE=DATE:20260219\r\nUID:20260219_ramadan@google.com\r\nSUMMARY:1 Ramadan\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nDTSTART;VALUE=DATE:20261231\r\nUID:20261231_malam-tahun-baru@google.com\r\nSUMMARY:Malam Tahun Baru\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nDTSTART;VALUE=DATE:20260101\r\nUID:20260101_tahun-baru@google.com\r\nSUMMARY:Tahun Baru 2026 Masehi\\, Hari Libur Nasional\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nDTSTART;VALUE=DATE:20270101\r\nUID:20270101_tahun-baru@google.com\r\nSUMMARY:Tahun Baru 2027 Masehi\r\nEND:VEVENT\r\nEND:VCALENDAR"

func TestICS(t *testing.T) {
	events := ParseICS(sampleICS)
	eq(t, len(events), 7)
	eq(t, events[0], IcsEvent{UID: "20260817_hari-kemerdekaan@google.com", Date: "2026-08-17", Name: "Hari Kemerdekaan Republik Indonesia"})
	eq(t, events[2].Name, "Hari Raya Waisak 2570 BE (belum pasti)")
	eq(t, events[5].Name, "Tahun Baru 2026 Masehi, Hari Libur Nasional")
	eq(t, ParseICS(""), []IcsEvent{})

	c := ToHolidayCandidates(events, 2026)
	eq(t, len(c), 6)
	for i := 1; i < len(c); i++ {
		if c[i-1].HolidayDate > c[i].HolidayDate {
			t.Fatal("candidates not sorted")
		}
	}
	byDate := map[string]HolidayCandidate{}
	for _, x := range c {
		byDate[x.HolidayDate] = x
	}
	eq(t, byDate["2026-03-20"].Type, "cuti_bersama")
	eq(t, byDate["2026-03-20"].DeductsLeave, true)
	eq(t, byDate["2026-08-17"].Suggested, true)
	eq(t, byDate["2026-08-17"].Status, "aktif")
	eq(t, byDate["2026-02-19"].Suggested, false)
	eq(t, byDate["2026-12-31"].Reason != nil, true)
	eq(t, byDate["2026-05-31"].Status, "draft")
	eq(t, strings.Contains(*byDate["2026-05-31"].Reason, "belum pasti"), true)
}

func TestContracts(t *testing.T) {
	eq(t, MonthsWorked("2026-01-01", "2026-07-01"), 6.0)
	eq(t, MonthsWorked("2026-01-01", "2027-01-01"), 12.0)
	eq(t, MonthsWorked("2026-01-15", "2026-01-30"), 0.5)
	eq(t, MonthsWorked("2026-07-01", "2026-01-01"), 0.0)
	eq(t, MonthsWorked("not-a-date", "2026-01-01"), 0.0)
	eq(t, ComputeKompensasi(4_800_000, "2026-01-01", "2027-01-01"), 4_800_000.0)
	eq(t, ComputeKompensasi(4_800_000, "2026-01-01", "2026-07-01"), 2_400_000.0)
	eq(t, ComputeKompensasi(4_800_000, "2026-01-01", "2026-01-20"), 0.0)
	eq(t, ComputeKompensasi(0, "2026-01-01", "2027-01-01"), 0.0)

	eq(t, ValidateContractDates(ContractDates{ContractType: "pkwt", StartDate: "2026-08-01", EndDate: sp("2027-08-01")}), []string{})
	join := func(d ContractDates) string { return strings.Join(ValidateContractDates(d), " ") }
	if !strings.Contains(join(ContractDates{ContractType: "pkwt", StartDate: "2026-08-01"}), "tanggal berakhir") {
		t.Fatal("pkwt end date required")
	}
	if !strings.Contains(join(ContractDates{ContractType: "pkwt", StartDate: "2026-08-01", EndDate: sp("2027-08-01"), ProbationEndDate: sp("2026-11-01")}), "masa percobaan") {
		t.Fatal("pkwt probation forbidden")
	}
	eq(t, ValidateContractDates(ContractDates{ContractType: "pkwtt", StartDate: "2026-08-01", ProbationEndDate: sp("2026-11-01")}), []string{})
	if !strings.Contains(join(ContractDates{ContractType: "pkwtt", StartDate: "2026-08-01", ProbationEndDate: sp("2026-12-15")}), "maksimal 3 bulan") {
		t.Fatal("probation cap")
	}
	if !strings.Contains(join(ContractDates{ContractType: "pkwt", StartDate: "2026-08-01", EndDate: sp("2026-07-01")}), "setelah tanggal mulai") {
		t.Fatal("end after start")
	}
	eq(t, PkwtChainTotalMonths([]ContractPeriod{{"2024-01-01", sp("2025-01-01")}, {"2025-01-01", sp("2026-01-01")}}), 24.0)
	eq(t, ValidatePkwtTotal(48, 12), "")
	if !strings.Contains(ValidatePkwtTotal(54, 12), "5 tahun") {
		t.Fatal("pkwt cap message")
	}
	eq(t, AddMonthsISO("2026-01-31", 1), "2026-02-28")
	eq(t, AddMonthsISO("2026-11-30", 3), "2027-02-28")
	eq(t, RomanMonth(7), "VII")
	eq(t, BuildContractNumber("pkwt", 1, time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)), "0001/PKWT/VII/2026")
	eq(t, BuildContractNumber("pkwtt", 42, time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)), "0042/PKWTT/XII/2026")

	eq(t, EmploymentStatusOnActivate("pkwt", nil, "2026-01-01"), "contract")
	eq(t, EmploymentStatusOnActivate("pkwtt", sp("2026-03-31"), "2026-03-31"), "probation")
	eq(t, EmploymentStatusOnActivate("pkwtt", sp("2026-03-31"), "2026-04-01"), "permanent")
	eq(t, RenewalStartDate("2026-12-31"), "2027-01-01")
	eq(t, RenewalStartDate("2028-02-28"), "2028-02-29")
}

func TestContractListParams(t *testing.T) {
	p := ParseContractListParams(query(""))
	eq(t, *p.Status, "active")
	eq(t, []any{p.SortBy, p.SortOrder, p.Page, p.Limit, p.ExpiringWithin == nil, p.ContractType == nil, p.Search == nil},
		[]any{"end_date", "asc", 1, 15, true, true, true})
	p = ParseContractListParams(query("days=30&type=pkwt&status=ended&search=+budi+&sort_by=employee_name&sort_order=desc&page=2&limit=50"))
	eq(t, []any{*p.ExpiringWithin, *p.ContractType, *p.Status, *p.Search, p.SortBy, p.SortOrder, p.Page, p.Limit},
		[]any{30, "pkwt", "ended", "budi", "employee_name", "desc", 2, 50})
	eq(t, ParseContractListParams(query("sort_by=base_salary")).SortBy, "end_date")
	eq(t, ParseContractListParams(query("status=all")).Status == nil, true)
	eq(t, *ParseContractListParams(query("days=9999")).ExpiringWithin, 365)
	eq(t, *ParseContractListParams(query("days=-3")).ExpiringWithin, 1)
	p = ParseContractListParams(query("page=0&limit=1000"))
	eq(t, []int{p.Page, p.Limit}, []int{1, 100})
	eq(t, ClampExpiringDays("", false), 30)
	eq(t, ClampExpiringDays("abc", true), 30)
	eq(t, ClampExpiringDays("0", true), 1)
	eq(t, ClampExpiringDays("45.9", true), 45)
	eq(t, ClampExpiringDays("365", true), 90)
}

func TestEmployees(t *testing.T) {
	eq(t, NextAutoNip(2026, nil), "EMP-2026-00001")
	eq(t, NextAutoNip(2026, []string{"EMP-2026-00001", "EMP-2026-00002", "EMP-2026-00004"}), "EMP-2026-00003")
	p := ParseDirectoryParams(query("search=budi,(x)&limit=9999&page=0&sort_by=bank_account&sort_order=desc&is_active=false"))
	eq(t, *p.Search, "budi  x")
	eq(t, []any{p.Limit, p.Page, p.SortBy, p.Ascending, *p.IsActive}, []any{500, 1, "full_name", false, false})
	eq(t, ParseDirectoryParams(query("")).IsActive == nil, true)

	cur := TrackedState{EmploymentStatus: sp("probation"), DepartmentID: sp("d1"), SectionID: sp("s1"), JobTitleID: sp("j1")}
	eq(t, EmploymentHistoryChange(cur, TrackedUpdate{}) == nil, true)
	c := EmploymentHistoryChange(cur, TrackedUpdate{EmploymentStatus: sp("permanent"), SentStatus: true, DepartmentID: sp("d2"), SentDepartment: true})
	eq(t, c.Notes, "Status: probation → permanent, Departemen berubah")
	eq(t, *c.NewDepartmentID, "d2")
	eq(t, *c.NewSectionID, "s1")
	eq(t, EmployeeUniqueMessage("employees_email_key"), "Email sudah terdaftar")
	eq(t, EmployeeUniqueMessage("employees_ktp_key"), "NIK/KTP sudah terdaftar")
	eq(t, EmployeeUniqueMessage("lainnya"), "")
}

func TestOvertime(t *testing.T) {
	emp, mgr, hr, other := "emp-1", "mgr-1", "hr-1", "emp-2"
	request := OvertimeRequest{EmployeeID: emp, RequestedBy: &emp, Source: "employee", ReportingTo: &mgr}
	company := OvertimeRequest{EmployeeID: emp, RequestedBy: &hr, Source: "company", ReportingTo: &mgr}
	ok := func(a OvertimeActor, r OvertimeRequest, action string) bool {
		v, _ := CanDecideOvertime(a, r, action)
		return v
	}
	hrActor, mgrActor := OvertimeActor{&hr, true}, OvertimeActor{&mgr, false}
	self, others := OvertimeActor{&emp, false}, OvertimeActor{&other, false}
	eq(t, ok(hrActor, request, "approve"), true)
	eq(t, ok(mgrActor, request, "approve"), true)
	eq(t, ok(others, request, "approve"), false)
	allowed, reason := CanDecideOvertime(OvertimeActor{&emp, true}, request, "approve")
	eq(t, allowed, false)
	eq(t, strings.Contains(reason, "sendiri"), true)
	eq(t, ok(self, request, "cancel"), true)
	eq(t, ok(others, request, "cancel"), false)
	eq(t, ok(self, company, "approve"), true)
	eq(t, ok(hrActor, company, "approve"), false)
	eq(t, ok(OvertimeActor{sp("hr-2"), true}, company, "cancel"), true)
	eq(t, ok(OvertimeActor{nil, true}, request, "approve"), true)
	eq(t, ok(OvertimeActor{nil, false}, request, "cancel"), false)

	eq(t, OvertimeHoursFromTimes("17:00", "19:30"), 2.5)
	eq(t, OvertimeHoursFromTimes("22:00", "02:00"), 4.0)
	s, e := PeriodBounds(2, 2028)
	eq(t, []string{s, e}, []string{"2028-02-01", "2028-02-29"})
	_, _, valid := OvertimeMonthFilter("13", "2026", true, true)
	eq(t, valid, false)
}

func TestLeaveRules(t *testing.T) {
	eq(t, ProratedAnnualQuota(sp("2024-05-10"), 2026), 12)
	eq(t, ProratedAnnualQuota(nil, 2026), 12)
	eq(t, ProratedAnnualQuota(sp("2026-01-20"), 2026), 12)
	eq(t, ProratedAnnualQuota(sp("2026-07-01"), 2026), 6)
	eq(t, ProratedAnnualQuota(sp("2026-12-15"), 2026), 1)

	eq(t, LeaveTypeLabel("annual"), "Cuti Tahunan")
	eq(t, LeaveTypeLabel("lainnya"), "lainnya")
	msg := LeaveRequestWaMessage(LeaveRequestWa{EmployeeName: "Adi Suryaningrat", LeaveType: "annual",
		StartDate: "2026-09-07", EndDate: "2026-09-09", TotalDays: 3, Reason: sp("Acara keluarga"), ApproverName: sp("Erik Hidayat")})
	for _, want := range []string{"Pengajuan Cuti Tahunan", "Kepada: Erik Hidayat", "Karyawan : Adi Suryaningrat", "s.d.", "(3 hari kerja)", "Alasan : Acara keluarga", "Mohon ditinjau", "Sen, 07 Sep 2026"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message lacks %q:\n%s", want, msg)
		}
	}
	single := LeaveRequestWaMessage(LeaveRequestWa{EmployeeName: "Adi", LeaveType: "sick", StartDate: "2026-09-07", EndDate: "2026-09-07", TotalDays: 1})
	eq(t, strings.Contains(single, "s.d.") || strings.Contains(single, "Alasan"), false)

	eq(t, BuildWaLink(sp("0812-3456"), "Halo Budi, (ok)!"), "https://wa.me/628123456?text=Halo%20Budi%2C%20(ok)!")
	eq(t, BuildWaLink(sp("0812"), "a b"), "https://wa.me/62812?text=a%20b")
	eq(t, BuildWaLink(nil, "x"), "")
	eq(t, NormalizeWaPhone(sp("0812 3456 7890")), "6281234567890")
	eq(t, NormalizeWaPhone(sp("123")), "")
}

func TestLeavesCSV(t *testing.T) {
	approved := time.Date(2026, 6, 1, 3, 0, 0, 0, time.UTC)
	row := LeaveCSVRow{ID: "lv-1", LeaveType: "sick", Status: "approved", StartDate: "2026-06-01", EndDate: "2026-06-02",
		TotalDays: sp("2"), Reason: sp(`Demam "tinggi"`), ApprovedAt: &approved, CreatedAt: time.Date(2026, 5, 31, 10, 15, 0, 0, time.UTC),
		EmployeeName: sp("Budi"), EmployeeNip: sp("EMP-1"), Department: sp("Bar"), JobTitle: sp("Barista"),
		HasApprover: true, ApproverName: sp("Sari"), ApproverNip: sp("EMP-9")}
	lines := strings.Split(LeavesCSV([]LeaveCSVRow{row}), "\n")
	eq(t, strings.HasPrefix(lines[0], "ID Cuti,NIP,Nama Karyawan"), true)
	for _, want := range []string{`"Cuti Sakit"`, `"Disetujui"`, `"Demam ""tinggi"""`, `"Sari (EMP-9)"`, "10.00.00"} {
		if !strings.Contains(lines[1], want) {
			t.Fatalf("csv line lacks %s: %s", want, lines[1])
		}
	}
	bare := row
	bare.LeaveType, bare.EmployeeName, bare.EmployeeNip, bare.Department, bare.JobTitle = "lainnya", nil, nil, nil, nil
	bare.HasApprover, bare.ApprovedAt, bare.Reason = false, nil, nil
	line := strings.Split(LeavesCSV([]LeaveCSVRow{bare}), "\n")[1]
	eq(t, strings.Contains(line, `"lainnya"`), true)
	if strings.Count(line, `"-"`) < 6 {
		t.Fatalf("expected dash placeholders: %s", line)
	}
	eq(t, CSVCell("=HYPERLINK()"), `"'=HYPERLINK()"`)
	eq(t, CSVCell("-"), `"-"`)
	eq(t, WIBDateTime(time.Date(2026, 10, 4, 3, 0, 0, 0, time.UTC)), "4/10/2026, 10.00.00")
}

func TestLogbook(t *testing.T) {
	eq(t, HasFullLogbookAccess("admin", true), true)
	eq(t, HasFullLogbookAccess("purchasing_manager", true), false)
	eq(t, CanReviewLogbook("admin", true), false)
	allowed, dept := ResolveDepartmentScope(true, nil, "")
	eq(t, allowed, true)
	eq(t, dept == nil, true)
	allowed, dept = ResolveDepartmentScope(false, sp("dept-1"), "")
	eq(t, []any{allowed, *dept}, []any{true, "dept-1"})
	allowed, _ = ResolveDepartmentScope(false, sp("dept-1"), "dept-2")
	eq(t, allowed, false)
	allowed, _ = ResolveDepartmentScope(false, nil, "")
	eq(t, allowed, false)

	v, ok := NormalizeLogbookNote("")
	eq(t, []any{v == nil, ok}, []any{true, true})
	_, ok = NormalizeLogbookNote(5.0)
	eq(t, ok, false)
	_, ok = NormalizeLogbookNote(strings.Repeat("x", LogbookNoteMaxLength+1))
	eq(t, ok, false)
	eq(t, TemplateItemWeight(0.0), 0.0)
	eq(t, TemplateItemWeight(nil), 1.0)
	eq(t, TemplateItemWeight("abc"), 1.0)
	eq(t, TemplateItemWeight("2.5"), 2.5)
	eq(t, TemplateItemWeight(-3.0), 0.0)

	d1 := map[string]any{"id": "d1", "name": "Bar"}
	rows := SummarizeLogbookEntries([]LogbookSummaryEntry{
		{"d1", d1, "submitted", 50, 80}, {"d1", d1, "reviewed", 100, 0}, {"d1", d1, "draft", 0, 70}, {"d2", nil, "draft", 0, 0},
	})
	eq(t, rows, []LogbookDepartmentSummary{
		{Department: d1, TotalEntries: 3, SubmittedEntries: 1, ReviewedEntries: 1, AvgCompletion: 50, AvgKPIScore: 50},
		{Department: nil, TotalEntries: 1},
	})
}

func TestReport(t *testing.T) {
	s, e := MonthRange(12, 2026)
	eq(t, []string{s, e}, []string{"2026-12-01", "2027-01-01"})
	d1 := "d1"
	r := BuildHrisReport(
		[]ReportEmployee{
			{"permanent", true, &d1, "2025-01-10", nil},
			{"contract", true, &d1, "2026-06-05", nil},
			{"resigned", false, sp("d2"), "2024-03-01", sp("2026-04-30")},
			{"permanent", true, nil, "2026-02-01", nil},
		},
		[]ReportDepartment{{"d1", "Bar"}, {"d2", "Kitchen"}},
		[]ReportAttendance{{"present", true, 8, "2026-06-02"}, {"present", false, 7, "2026-06-01"}, {"absent", false, 0, "2026-06-01"}},
		[]ReportLeave{{"annual", "approved", 2}, {"sick", "approved", 3}, {"annual", "pending", 1}},
		6, 2026)
	h := r.Headcount
	eq(t, []any{h.TotalActive, h.NewHires, h.TurnoverCount, h.TurnoverRate}, []any{3, 1, 1, 25.0})
	eq(t, h.ByStatus, []StatusCount{{"permanent", 2}, {"contract", 1}})
	eq(t, h.ByDepartment, []NameCount{{"Bar", 2}})
	var trend []int
	for _, m := range h.MonthlyTrend {
		trend = append(trend, m.Count)
	}
	eq(t, trend, []int{2, 3, 3, 3, 2, 3})
	eq(t, h.MonthlyTrend[0].Month, "Jan 26")
	a := r.Attendance
	eq(t, []any{a.TotalRecords, a.PresentCount, a.AbsentCount, a.LateCount, a.PresentRate, a.LateRate, a.AvgWorkHours},
		[]any{3, 2, 1, 1, 66.7, 50.0, 5.0})
	eq(t, a.DailyTrend, []DailyAttendance{{"01 Jun", 1, 1, 0}, {"02 Jun", 1, 0, 1}})
	eq(t, r.Leaves.ByType, []LeaveDays{{"Sakit", 3}, {"Tahunan", 2}})
	eq(t, []any{r.Leaves.ApprovedCount, r.Leaves.PendingCount, r.Leaves.TotalDays}, []any{2, 1, 5.0})
	empty := BuildHrisReport(nil, nil, nil, nil, 1, 2026)
	eq(t, []float64{empty.Headcount.TurnoverRate, empty.Attendance.PresentRate, empty.Attendance.AvgWorkHours}, []float64{0, 0, 0})
}

func TestESS(t *testing.T) {
	pagi, name, start, end := "s-pagi", "Pagi", "08:00:00", "16:00:00"
	tol := int64(10)
	var rows []ESSScheduleRow
	for d := 1; d <= 5; d++ {
		rows = append(rows, ESSScheduleRow{ScheduleRow{d, &pagi, "2026-01-01", nil}, &name, &start, &end, &tol})
	}
	rows = append(rows, ESSScheduleRow{ScheduleRow: ScheduleRow{6, nil, "2026-01-01", nil}})
	eq(t, *TodayShiftOf(rows, "2026-10-05"), TodayShift{"Pagi", &start, &end, 10})
	eq(t, TodayShiftOf(rows, "2026-10-10") == nil, true)
	eq(t, TodayShiftOf(rows, "2026-10-11") == nil, true)
	week := WeekScheduleOf(rows, "2026-10-07")
	var dates, statuses []string
	for _, d := range week {
		dates, statuses = append(dates, d.Date), append(statuses, d.Status)
	}
	eq(t, dates, []string{"2026-10-05", "2026-10-06", "2026-10-07", "2026-10-08", "2026-10-09", "2026-10-10", "2026-10-11"})
	eq(t, statuses, []string{"shift", "shift", "shift", "shift", "shift", "libur", "none"})
	eq(t, []any{week[2].IsToday, week[2].DayOfWeek, *week[2].ShiftName}, []any{true, 3, "Pagi"})
	eq(t, week[5].ShiftName == nil, true)

	eq(t, FormatRupiah("1500000"), "Rp 1.500.000")
	eq(t, FormatRupiah("-950"), "-Rp 950")
	eq(t, Clock5(&start), "08:00")
	merged := MergeRecentRequests([]RecentRequest{{ID: "l1", CreatedAt: "2026-10-01"}, {ID: "p7", CreatedAt: "2026-10-07"}, {ID: "o5", CreatedAt: "2026-10-05"}})
	eq(t, []string{merged[0].ID, merged[1].ID, merged[2].ID}, []string{"p7", "o5", "l1"})
}

func TestVideo(t *testing.T) {
	check := func(raw, provider, id string) {
		t.Helper()
		p, i, ok := ParseVideoURL(raw)
		if provider == "" {
			if ok {
				t.Fatalf("%s accepted", raw)
			}
			return
		}
		eq(t, []any{p, i, ok}, []any{provider, id, true})
	}
	check("https://www.youtube.com/watch?v=dQw4w9WgXcQ", "youtube", "dQw4w9WgXcQ")
	check("https://youtu.be/dQw4w9WgXcQ", "youtube", "dQw4w9WgXcQ")
	check("https://youtube.com/embed/dQw4w9WgXcQ", "youtube", "dQw4w9WgXcQ")
	check("https://www.youtube.com/shorts/dQw4w9WgXcQ", "youtube", "dQw4w9WgXcQ")
	check("https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=42s", "youtube", "dQw4w9WgXcQ")
	check("https://www.youtube.com/watch?v=short", "", "")
	check("https://vimeo.com/123456789", "vimeo", "123456789")
	check("https://player.vimeo.com/video/123456789", "vimeo", "123456789")
	check("https://vimeo.com/abc", "", "")
	check("https://evil.example.com/watch?v=dQw4w9WgXcQ", "", "")
	check("javascript:alert(1)", "", "")
	check("bukan url", "", "")
	check("", "", "")
}

package domain

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

/* ── Logbook (lib/hris/logbook) ──────────────────────────────────────── */

// LogbookNoteMaxLength caps notes and review notes.
const LogbookNoteMaxLength = 20000

// HasFullLogbookAccess: super_admin, admin and hrd holding the Kinerja menu
// (performance) see every department; without it they see their own only.
func HasFullLogbookAccess(role string, performance bool) bool {
	return performance && (role == "super_admin" || role == "admin" || role == "hrd")
}

// CanReviewLogbook: super_admin and hrd holding the Kinerja menu review entries.
func CanReviewLogbook(role string, performance bool) bool {
	return performance && (role == "super_admin" || role == "hrd")
}

// ResolveDepartmentScope: full access may pick any department (or all);
// everyone else is locked to their own and denied without one.
func ResolveDepartmentScope(fullAccess bool, own *string, requested string) (bool, *string) {
	if fullAccess {
		if requested == "" {
			return true, nil
		}
		return true, &requested
	}
	if own == nil || *own == "" {
		return false, nil
	}
	if requested != "" && requested != *own {
		return false, nil
	}
	return true, own
}

// NormalizeLogbookNote: absent/null is nil, "" is nil, a string within the
// cap is kept; anything else fails.
func NormalizeLogbookNote(raw any) (*string, bool) {
	if raw == nil {
		return nil, true
	}
	s, ok := raw.(string)
	if !ok || utf16Len(s) > LogbookNoteMaxLength {
		return nil, false
	}
	if s == "" {
		return nil, true
	}
	return &s, true
}

// TemplateItemWeight: an explicit 0 stays, missing or non-numeric is 1,
// negatives become 0. raw is a JSON value (float64, string, bool, nil).
func TemplateItemWeight(raw any) float64 {
	var n float64
	switch v := raw.(type) {
	case nil:
		return 1
	case float64:
		n = v
	case string:
		n = JSNumber(v)
	case bool:
		n = 0
		if v {
			n = 1
		}
	default:
		return 1 // arrays/objects: Number([]) is 0 but Number({}) NaN; keep the default
	}
	if !IsFinite(n) {
		return 1
	}
	return math.Max(0, n)
}

// LogbookSummaryEntry is one entry of the summary resource.
type LogbookSummaryEntry struct {
	DepartmentID string
	Department   any
	Status       string
	Completion   float64
	KPIScore     float64
}

// LogbookDepartmentSummary is one department row of the summary.
type LogbookDepartmentSummary struct {
	Department       any     `json:"department"`
	TotalEntries     int     `json:"total_entries"`
	SubmittedEntries int     `json:"submitted_entries"`
	ReviewedEntries  int     `json:"reviewed_entries"`
	AvgCompletion    float64 `json:"avg_completion"`
	AvgKPIScore      float64 `json:"avg_kpi_score"`
}

// SummarizeLogbookEntries groups by department in first-seen order.
func SummarizeLogbookEntries(entries []LogbookSummaryEntry) []LogbookDepartmentSummary {
	out := []LogbookDepartmentSummary{}
	index := map[string]int{}
	for _, e := range entries {
		i, ok := index[e.DepartmentID]
		if !ok {
			i = len(out)
			index[e.DepartmentID] = i
			out = append(out, LogbookDepartmentSummary{Department: e.Department})
		}
		row := &out[i]
		row.TotalEntries++
		switch e.Status {
		case "submitted":
			row.SubmittedEntries++
		case "reviewed":
			row.ReviewedEntries++
		}
		row.AvgCompletion += e.Completion
		row.AvgKPIScore += e.KPIScore
	}
	for i := range out {
		n := float64(out[i].TotalEntries)
		out[i].AvgCompletion = ToFixed(out[i].AvgCompletion/n, 2)
		out[i].AvgKPIScore = ToFixed(out[i].AvgKPIScore/n, 2)
	}
	return out
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

/* ── Monthly HRIS report (lib/hris/reports-summary) ──────────────────── */

// MonthRange is [first of month, first of next month).
func MonthRange(month, year int) (string, string) {
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	return start.Format(DateLayout), start.AddDate(0, 1, 0).Format(DateLayout)
}

// ReportEmployee, ReportAttendance and ReportLeave are the report inputs;
// dates are "YYYY-MM-DD".
type ReportEmployee struct {
	EmploymentStatus string
	IsActive         bool
	DepartmentID     *string
	JoinDate         string
	EndDate          *string
}

type ReportDepartment struct{ ID, Name string }

type ReportAttendance struct {
	Status    string
	IsLate    bool
	WorkHours float64
	Date      string
}

type ReportLeave struct {
	LeaveType string
	Status    string
	TotalDays float64
}

var reportLeaveLabels = map[string]string{
	"annual": "Tahunan", "sick": "Sakit", "maternity": "Melahirkan", "paternity": "Ayah",
	"unpaid": "Tidak Dibayar", "emergency": "Darurat", "pilgrimage": "Haji/Umrah",
	"menstrual": "Haid", "marriage": "Pernikahan", "bereavement": "Duka Cita",
	"personal": "Izin Pribadi",
}

type StatusCount struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type NameCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type MonthCount struct {
	Month string `json:"month"`
	Count int    `json:"count"`
}

type DailyAttendance struct {
	Date    string `json:"date"`
	Present int    `json:"present"`
	Absent  int    `json:"absent"`
	Late    int    `json:"late"`
}

type LeaveDays struct {
	Type string  `json:"type"`
	Days float64 `json:"days"`
}

// HrisReport is the GET /api/hris/reports body.
type HrisReport struct {
	Period struct {
		Month int `json:"month"`
		Year  int `json:"year"`
	} `json:"period"`
	Headcount struct {
		TotalActive   int           `json:"total_active"`
		NewHires      int           `json:"new_hires"`
		TurnoverCount int           `json:"turnover_count"`
		TurnoverRate  float64       `json:"turnover_rate"`
		ByStatus      []StatusCount `json:"by_status"`
		ByDepartment  []NameCount   `json:"by_department"`
		MonthlyTrend  []MonthCount  `json:"monthly_trend"`
	} `json:"headcount"`
	Attendance struct {
		TotalRecords int               `json:"total_records"`
		PresentCount int               `json:"present_count"`
		AbsentCount  int               `json:"absent_count"`
		LateCount    int               `json:"late_count"`
		PresentRate  float64           `json:"present_rate"`
		LateRate     float64           `json:"late_rate"`
		AvgWorkHours float64           `json:"avg_work_hours"`
		DailyTrend   []DailyAttendance `json:"daily_trend"`
	} `json:"attendance"`
	Leaves struct {
		ApprovedCount int         `json:"approved_count"`
		PendingCount  int         `json:"pending_count"`
		TotalDays     float64     `json:"total_days"`
		ByType        []LeaveDays `json:"by_type"`
	} `json:"leaves"`
}

func pct1(part, whole int) float64 {
	if whole <= 0 {
		return 0
	}
	return ToFixed(float64(part)/float64(whole)*100, 1)
}

// BuildHrisReport is buildHrisReport.
func BuildHrisReport(employees []ReportEmployee, departments []ReportDepartment,
	attendance []ReportAttendance, leaves []ReportLeave, month, year int) HrisReport {
	var r HrisReport
	r.Period.Month, r.Period.Year = month, year
	start, end := MonthRange(month, year)

	var active []ReportEmployee
	for _, e := range employees {
		if e.IsActive {
			active = append(active, e)
		}
	}
	h := &r.Headcount
	h.TotalActive = len(active)
	yearStart, yearEnd := fmt.Sprintf("%d-01-01", year), fmt.Sprintf("%d-01-01", year+1)
	for _, e := range employees {
		if e.JoinDate >= start && e.JoinDate < end {
			h.NewHires++
		}
		if (e.EmploymentStatus == "resigned" || e.EmploymentStatus == "terminated") &&
			e.EndDate != nil && *e.EndDate != "" && *e.EndDate >= yearStart && *e.EndDate < yearEnd {
			h.TurnoverCount++
		}
	}
	if h.TotalActive > 0 {
		h.TurnoverRate = pct1(h.TurnoverCount, h.TotalActive+h.TurnoverCount)
	}
	h.ByStatus = []StatusCount{}
	statusIndex := map[string]int{}
	for _, e := range active {
		i, ok := statusIndex[e.EmploymentStatus]
		if !ok {
			i = len(h.ByStatus)
			statusIndex[e.EmploymentStatus] = i
			h.ByStatus = append(h.ByStatus, StatusCount{Status: e.EmploymentStatus})
		}
		h.ByStatus[i].Count++
	}
	h.ByDepartment = []NameCount{}
	for _, d := range departments {
		n := 0
		for _, e := range active {
			if e.DepartmentID != nil && *e.DepartmentID == d.ID {
				n++
			}
		}
		if n > 0 {
			h.ByDepartment = append(h.ByDepartment, NameCount{Name: d.Name, Count: n})
		}
	}
	h.MonthlyTrend = make([]MonthCount, 6)
	for i := range 6 {
		d := time.Date(year, time.Month(month-(5-i)), 1, 0, 0, 0, 0, time.UTC)
		mStart, mEnd := d.Format(DateLayout), d.AddDate(0, 1, 0).Format(DateLayout)
		n := 0
		for _, e := range employees {
			if e.JoinDate < mEnd && (e.EndDate == nil || *e.EndDate == "" || *e.EndDate >= mStart) {
				n++
			}
		}
		h.MonthlyTrend[i] = MonthCount{Month: fmt.Sprintf("%s %02d", idMonths[d.Month()-1], d.Year()%100), Count: n}
	}

	a := &r.Attendance
	a.TotalRecords = len(attendance)
	daily := map[string]*DailyAttendance{}
	var hours float64
	for _, at := range attendance {
		day, ok := daily[at.Date]
		if !ok {
			day = &DailyAttendance{}
			daily[at.Date] = day
		}
		switch at.Status {
		case "present":
			a.PresentCount++
			day.Present++
		case "absent":
			a.AbsentCount++
			day.Absent++
		}
		if at.IsLate {
			a.LateCount++
			day.Late++
		}
		hours += at.WorkHours
	}
	if a.TotalRecords > 0 {
		a.AvgWorkHours = ToFixed(hours/float64(a.TotalRecords), 1)
	}
	a.PresentRate = pct1(a.PresentCount, a.TotalRecords)
	a.LateRate = pct1(a.LateCount, a.PresentCount)
	dates := make([]string, 0, len(daily))
	for d := range daily {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	a.DailyTrend = make([]DailyAttendance, len(dates))
	for i, d := range dates {
		row := *daily[d]
		t, _ := time.Parse(DateLayout, d)
		row.Date = fmt.Sprintf("%02d %s", t.Day(), idMonths[t.Month()-1])
		a.DailyTrend[i] = row
	}

	l := &r.Leaves
	var typeOrder []string
	byType := map[string]float64{}
	for _, lv := range leaves {
		switch lv.Status {
		case "approved":
			l.ApprovedCount++
			l.TotalDays += lv.TotalDays
			if _, ok := byType[lv.LeaveType]; !ok {
				typeOrder = append(typeOrder, lv.LeaveType)
			}
			byType[lv.LeaveType] += lv.TotalDays
		case "pending":
			l.PendingCount++
		}
	}
	l.ByType = make([]LeaveDays, len(typeOrder))
	for i, t := range typeOrder {
		label := t
		if v, ok := reportLeaveLabels[t]; ok {
			label = v
		}
		l.ByType[i] = LeaveDays{Type: label, Days: byType[t]}
	}
	sort.SliceStable(l.ByType, func(i, j int) bool { return l.ByType[i].Days > l.ByType[j].Days })
	return r
}

/* ── Employee self service (lib/hris/me-profile) ─────────────────────── */

// ESSScheduleRow is a pattern row joined with its shift.
type ESSScheduleRow struct {
	ScheduleRow
	ShiftName            *string
	StartTime            *string
	EndTime              *string
	LateToleranceMinutes *int64
}

func rowsOf(rows []ESSScheduleRow) []ScheduleRow {
	out := make([]ScheduleRow, len(rows))
	for i, r := range rows {
		out[i] = r.ScheduleRow
	}
	return out
}

// shiftDetail is the first row naming the winner's shift (else the winner).
func shiftDetail(rows []ESSScheduleRow, winner int) *ESSScheduleRow {
	if winner < 0 || rows[winner].ShiftID == nil {
		return nil
	}
	for i := range rows {
		if rows[i].ShiftID != nil && *rows[i].ShiftID == *rows[winner].ShiftID && rows[i].ShiftName != nil && *rows[i].ShiftName != "" {
			return &rows[i]
		}
	}
	return &rows[winner]
}

// TodayShift is the shift that applies today.
type TodayShift struct {
	Name                 string  `json:"name"`
	StartTime            *string `json:"start_time"`
	EndTime              *string `json:"end_time"`
	LateToleranceMinutes int64   `json:"late_tolerance_minutes"`
}

// TodayShiftOf: the shift on today, nil on a day off or without a pattern.
func TodayShiftOf(rows []ESSScheduleRow, today string) *TodayShift {
	winner := ResolveScheduleRow(rowsOf(rows), today)
	d := shiftDetail(rows, winner)
	if d == nil || d.ShiftName == nil || *d.ShiftName == "" {
		return nil
	}
	t := &TodayShift{Name: *d.ShiftName, StartTime: d.StartTime, EndTime: d.EndTime}
	if d.LateToleranceMinutes != nil {
		t.LateToleranceMinutes = *d.LateToleranceMinutes
	}
	return t
}

// DaySchedule is one day of the ESS week view.
type DaySchedule struct {
	Date      string  `json:"date"`
	DayOfWeek int     `json:"day_of_week"`
	IsToday   bool    `json:"is_today"`
	Status    string  `json:"status"`
	ShiftName *string `json:"shift_name"`
	StartTime *string `json:"start_time"`
	EndTime   *string `json:"end_time"`
}

// WeekScheduleOf is Monday–Sunday of the week containing today.
func WeekScheduleOf(rows []ESSScheduleRow, today string) []DaySchedule {
	monday := AddDaysISO(today, -(IsoDayOfWeek(today) - 1))
	plain := rowsOf(rows)
	out := make([]DaySchedule, 7)
	for i := range 7 {
		date := AddDaysISO(monday, i)
		winner := ResolveScheduleRow(plain, date)
		day := DaySchedule{Date: date, DayOfWeek: IsoDayOfWeek(date), IsToday: date == today, Status: "none"}
		if winner >= 0 {
			day.Status = "libur"
			if rows[winner].ShiftID != nil {
				day.Status = "shift"
				if d := shiftDetail(rows, winner); d != nil {
					day.ShiftName, day.StartTime, day.EndTime = d.ShiftName, d.StartTime, d.EndTime
				}
			}
		}
		out[i] = day
	}
	return out
}

// RecentRequest is one row of the ESS "recent requests" list.
type RecentRequest struct {
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	Label     string `json:"label"`
	Detail    string `json:"detail"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	Href      string `json:"href"`
}

// MergeRecentRequests: newest first by created_at text, at most 6.
func MergeRecentRequests(rows []RecentRequest) []RecentRequest {
	out := append([]RecentRequest{}, rows...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

// FormatRupiah is formatRupiah: "Rp 1.500.000", "-Rp…", rounded.
func FormatRupiah(value string) string {
	n := JSNumber(value)
	if !IsFinite(n) {
		n = 0
	}
	n = JSRound(n)
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	digits := strconv.FormatFloat(n, 'f', 0, 64)
	var b strings.Builder
	for i, c := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	return sign + "Rp " + b.String()
}

// Clock5 is time.slice(0, 5) on a nullable "HH:MM:SS" ("undefined" for null).
func Clock5(s *string) string {
	if s == nil {
		return "undefined"
	}
	if len(*s) > 5 {
		return (*s)[:5]
	}
	return *s
}

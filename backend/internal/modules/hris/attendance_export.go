package hris

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/imageproc"
	"nuhabit/backend/internal/platform/pdfgen"
	"nuhabit/backend/internal/platform/xlsx"
)

// Attendance recap export (app/api/hris/attendance/export, lib/hris/
// attendance-report.ts): CSV by default, Excel and a PDF with compressed
// selfies on request.

// exportFile is a generated download.
type exportFile struct {
	Data        []byte
	ContentType string
	FileName    string
}

// reportMeta is AttendanceReportMeta.
type reportMeta struct {
	CompanyName, PeriodLabel string
	EmployeeLabel            *string
	GeneratedAt              time.Time
}

// ExportAttendance builds the export of the rows matching f in format
// (csv unless "xlsx" or "pdf").
func (s *Service) ExportAttendance(ctx context.Context, f exportFilter, format string) (*exportFile, error) {
	records, err := s.repo.AttendanceExport(ctx, f)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, httpx.NotFound("No attendance data found")
	}
	now := s.clock()
	stamp := now.Format(domain.DateLayout)
	if format != "xlsx" && format != "pdf" {
		return &exportFile{[]byte(attendanceCSV(records)), "text/csv;charset=utf-8",
			"attendance_export_" + stamp + ".csv"}, nil
	}

	rows := toReportRows(records)
	company, err := s.company.FirstCompanyName(ctx, s.repo.querier())
	if err != nil {
		return nil, err
	}
	meta := reportMeta{CompanyName: "Paddy OS", PeriodLabel: "Semua tanggal", GeneratedAt: now}
	if company != nil {
		meta.CompanyName = *company
	}
	if f.StartDate != "" && f.EndDate != "" {
		meta.PeriodLabel = domain.PeriodLabel(f.StartDate, f.EndDate)
	}
	if f.EmployeeID != "" {
		meta.EmployeeLabel = &rows[0].EmployeeName
	}
	if format == "xlsx" {
		data, err := attendanceXlsx(rows, meta)
		return &exportFile{data, xlsx.ContentType, "rekap-absensi-" + stamp + ".xlsx"}, err
	}
	data, err := attendancePDF(rows, meta, s.attendancePhoto)
	return &exportFile{data, "application/pdf", "rekap-absensi-" + stamp + ".pdf"}, err
}

// attendancePhoto loads a selfie for the PDF as a JPEG thumbnail
// (compressAttendancePhoto); a file imageproc cannot read is used as is
// when it is a JPEG or PNG, else nil (placeholder).
func (s *Service) attendancePhoto(path string) ([]byte, string) {
	data, mime, err := s.files.ReadPrivate(path)
	if err != nil {
		return nil, ""
	}
	if thumb, err := imageproc.JPEGThumbnail(data, imageproc.ThumbnailOptions{MaxWidth: 480, Quality: 62}); err == nil {
		return thumb, "image/jpeg"
	}
	if embeddable.MatchString(mime) {
		return data, mime
	}
	return nil, ""
}

// embeddable is the MIME pdfkit embeds: JPEG and PNG.
var embeddable = regexp.MustCompile(`(?i)jpe?g|png`)

// toReportRows maps the export rows to recap lines ordered by employee,
// then date.
func toReportRows(records []*Row) []domain.ReportRow {
	rows := make([]domain.ReportRow, len(records))
	for i, r := range records {
		name := r.Str("full_name")
		if name == "" {
			name = "-"
		}
		row := domain.ReportRow{
			Date: r.Str("date"), EmployeeName: name,
			NIP: r.StrPtr("nip"), Department: r.StrPtr("department"), Position: r.StrPtr("job_title"),
			Status: r.StrPtr("status"), IsLate: r.Bool("is_late"), IsShortHours: r.Bool("is_short_hours"), Notes: r.StrPtr("notes"),
			ClockInPhoto: r.StrPtr("clock_in_photo_url"), ClockOutPhoto: r.StrPtr("clock_out_photo_url"),
		}
		if t, ok := r.Time("clock_in"); ok {
			row.ClockIn = &t
		}
		if t, ok := r.Time("clock_out"); ok {
			row.ClockOut = &t
		}
		if r.Get("work_hours") != nil {
			h := r.Num("work_hours")
			row.WorkHours = &h
		}
		if r.Get("min_work_hours") != nil {
			m := r.Num("min_work_hours")
			row.MinWorkHours = &m
		}
		row.LateMinutes = int(r.Num("late_minutes"))
		rows[i] = row
	}
	domain.SortReportRows(rows)
	return rows
}

/* ── CSV ─────────────────────────────────────────────────────────────── */

const attendanceCSVHeader = "NIP,Nama Karyawan,Departemen,Jabatan,Tanggal,Clock In,Clock Out," +
	"Lokasi Clock In,Lokasi Clock Out,Jam Kerja (jam),Istirahat (menit),Status,Terlambat," +
	"Keterlambatan (menit),Kurang Jam,Jam Minimum (jam),Catatan"

// attendanceCSV is buildAttendanceCsv: every cell quoted, rows in query
// order (newest first).
func attendanceCSV(records []*Row) string {
	lines := []string{attendanceCSVHeader}
	orDash := func(r *Row, key string) string {
		if v := r.Str(key); v != "" {
			return v
		}
		return "-"
	}
	orZero := func(r *Row, key string) string {
		switch v := r.Get(key).(type) {
		case string: // numeric text is truthy even when "0.00"
			return v
		case int64:
			if v != 0 {
				return fmt.Sprint(v)
			}
		}
		return "0"
	}
	when := func(r *Row, key string) string {
		t, ok := r.Time(key)
		if !ok {
			return "-"
		}
		return domain.DateTimeWIB(&t, "-")
	}
	for _, r := range records {
		late := "Tidak"
		if r.Bool("is_late") {
			late = "Ya"
		}
		short := "-"
		if r.Bool("is_short_hours") {
			short = "Ya"
		}
		minHours := "-"
		if v, ok := r.Get("min_work_hours").(string); ok { // numeric text, like work_hours
			minHours = v
		}
		cells := []string{
			orDash(r, "nip"), orDash(r, "full_name"), orDash(r, "department"), orDash(r, "job_title"),
			r.Str("date"), when(r, "clock_in"), when(r, "clock_out"),
			csvLocation(r.Child("clock_in_location")), csvLocation(r.Child("clock_out_location")),
			orZero(r, "work_hours"), orZero(r, "break_minutes"), orDash(r, "status"), late,
			orZero(r, "late_minutes"), short, minHours, orDash(r, "notes"),
		}
		for i, c := range cells {
			cells[i] = `"` + strings.ReplaceAll(c, `"`, `""`) + `"`
		}
		lines = append(lines, strings.Join(cells, ","))
	}
	return strings.Join(lines, "\n")
}

// csvLocation is "lat,lng (address)", "-" without a location.
func csvLocation(loc *Row) string {
	if loc == nil {
		return "-"
	}
	out := jsString(loc, "latitude") + "," + jsString(loc, "longitude")
	if addr := loc.Get("address"); addr != nil && addr != "" && addr != false && addr != 0.0 {
		out += " (" + jsString(loc, "address") + ")"
	}
	return out
}

// jsString is the template-literal form of a JSON value.
func jsString(r *Row, key string) string {
	if !r.Has(key) {
		return "undefined"
	}
	switch v := r.Get(key).(type) {
	case nil:
		return "null"
	case float64:
		return xlsx.JSNumber(v)
	case string:
		return v
	case *Row:
		return "[object Object]"
	default:
		return fmt.Sprint(v)
	}
}

/* ── Excel ───────────────────────────────────────────────────────────── */

// printedAt is the "Dicetak" stamp: "28 Agu 2026, 10.00 WIB".
func printedAt(t time.Time) string { return domain.DateTimeWIB(&t, "") + " WIB" }

func employeeLabel(m reportMeta) string {
	if m.EmployeeLabel == nil {
		return "Semua Karyawan"
	}
	return *m.EmployeeLabel
}

func orHyphen(s *string) string {
	if s == nil {
		return "-"
	}
	return *s
}

// attendanceXlsx is buildAttendanceXlsx.
func attendanceXlsx(rows []domain.ReportRow, m reportMeta) ([]byte, error) {
	header := []any{"Tanggal", "Nama Karyawan", "NIP", "Departemen", "Jabatan",
		"Jam Masuk", "Jam Pulang", "Jam Kerja", "Status", "Terlambat (menit)", "Kurang Jam", "Jam Minimum", "Catatan"}
	aoa := [][]any{
		{m.CompanyName + " — Rekap Absensi"},
		{"Periode: " + m.PeriodLabel},
		{"Karyawan: " + employeeLabel(m)},
		{"Dicetak: " + printedAt(m.GeneratedAt)},
		{},
		header,
	}
	for _, r := range rows {
		hours, late, notes := 0.0, 0, ""
		var short, minHours any = "-", "-"
		if r.WorkHours != nil {
			hours = *r.WorkHours
		}
		if r.IsLate {
			late = r.LateMinutes
		}
		if r.Notes != nil {
			notes = *r.Notes
		}
		if r.IsShortHours {
			short = "Ya"
		}
		if r.MinWorkHours != nil {
			minHours = *r.MinWorkHours
		}
		aoa = append(aoa, []any{domain.TanggalShort(r.Date), r.EmployeeName, orHyphen(r.NIP), orHyphen(r.Department),
			orHyphen(r.Position), domain.ClockWIB(r.ClockIn, "—"), domain.ClockWIB(r.ClockOut, "—"), hours,
			domain.StatusLabel(r.Status), late, short, minHours, notes})
	}
	merges := make([]xlsx.Merge, 4)
	for i := range merges {
		merges[i] = xlsx.Merge{R1: i, R2: i, C2: len(header) - 1}
	}
	return xlsx.Build(xlsx.SheetSpec{Name: "Rekap Absensi", Rows: aoa,
		ColumnWidths: []float64{18, 26, 20, 18, 18, 10, 11, 9, 12, 16, 11, 13, 30}, Merges: merges})
}

/* ── PDF with selfies ────────────────────────────────────────────────── */

const (
	reportText  = "#111827"
	reportMuted = "#6b7280"
	reportLine  = "#d1d5db"
	reportHead  = "#1f3864"
	reportZebra = "#f3f5fa"
	reportLate  = "#b91c1c"

	reportMargin = 36.0
	reportRowH   = 74.0 // 62pt photo + padding
	reportHeadH  = 22.0
	reportPhotoH = 62.0
)

// reportCols total 770pt: A4 landscape inside 36pt margins.
var reportCols = []struct {
	label string
	w     float64
}{
	{"Tanggal", 78}, {"Karyawan", 128}, {"Masuk", 46}, {"Pulang", 46}, {"Jam", 34},
	{"Status", 74}, {"Foto Masuk", 92}, {"Foto Pulang", 92}, {"Catatan", 180},
}

const reportTableW = 770.0

func drawReportHeader(d *pdfgen.Doc, y float64) float64 {
	d.Rect(reportMargin, y, reportTableW, reportHeadH, reportHead)
	d.Font(pdfgen.HelveticaBold, 8).Color("#ffffff")
	x := reportMargin
	for _, c := range reportCols {
		d.Text(c.label, x+4, y+7, pdfgen.TextOpts{Width: c.w - 8, NoWrap: true})
		x += c.w
	}
	return y + reportHeadH
}

// attendancePDF is buildAttendancePdf; photo returns a selfie ready to embed
// and its MIME, nil for a placeholder.
func attendancePDF(rows []domain.ReportRow, m reportMeta, photo func(path string) ([]byte, string)) ([]byte, error) {
	d := pdfgen.New(pdfgen.Options{Size: "A4", Landscape: true, Margin: reportMargin,
		Title: "Rekap Absensi — " + m.PeriodLabel, Now: m.GeneratedAt})
	d.Font(pdfgen.HelveticaBold, 14).Color(reportText)
	d.Text(m.CompanyName+" — Rekap Absensi", reportMargin, reportMargin, pdfgen.TextOpts{})
	d.Font(pdfgen.Helvetica, 9).Color(reportMuted)
	d.Text(fmt.Sprintf("Periode: %s   ·   Karyawan: %s   ·   Dicetak: %s", m.PeriodLabel, employeeLabel(m), printedAt(m.GeneratedAt)),
		reportMargin, reportMargin+20, pdfgen.TextOpts{})
	y := drawReportHeader(d, reportMargin+40)

	center := func(text string, size, x, y, w float64) {
		d.Font(pdfgen.Helvetica, size).Color(reportMuted)
		d.Text(text, x+4, y, pdfgen.TextOpts{Width: w - 8, Align: pdfgen.AlignCenter})
	}
	drawPhoto := func(path *string, x, rowY, w float64) {
		mid := rowY + reportRowH/2
		if path == nil {
			center("—", 8, x, mid-4, w)
			return
		}
		data, mime := photo(*path)
		if data == nil || !embeddable.MatchString(mime) {
			center("foto tidak dapat", 7, x, mid-9, w)
			center("ditampilkan", 7, x, mid+1, w)
			return
		}
		if err := d.Image(data, x+4, rowY+(reportRowH-reportPhotoH)/2, w-8, reportPhotoH); err != nil {
			center("foto rusak", 7, x, mid-4, w)
		}
	}

	for i, r := range rows {
		if y+reportRowH > d.Bottom() {
			d.AddPage()
			y = drawReportHeader(d, reportMargin)
		}
		if i%2 == 1 {
			d.Rect(reportMargin, y, reportTableW, reportRowH, reportZebra)
		}
		x, textY := reportMargin, y+8
		cell := func(text string, w float64, font pdfgen.Font, color, sub string) {
			d.Font(font, 8).Color(color)
			d.Text(text, x+4, textY, pdfgen.TextOpts{Width: w - 8, Height: reportRowH - 8})
			if sub != "" {
				d.Font(pdfgen.Helvetica, 7).Color(reportMuted)
				d.Text(sub, x+4, textY+11, pdfgen.TextOpts{Width: w - 8, Height: reportRowH - 19})
			}
			x += w
		}
		var sub []string
		for _, p := range []*string{r.NIP, r.Department} {
			if p != nil && *p != "" {
				sub = append(sub, *p)
			}
		}
		hours := "—"
		if r.WorkHours != nil {
			hours = xlsx.JSNumber(*r.WorkHours)
		}
		status, statusColor := domain.StatusLabel(r.Status), reportText
		if r.IsLate {
			status, statusColor = fmt.Sprintf("%s +%dm", status, r.LateMinutes), reportLate
		}
		if r.IsShortHours { // EPIC-067 R2: the table is full, so the label rides on Status
			status += " · kurang jam"
		}
		cell(domain.TanggalShort(r.Date), reportCols[0].w, pdfgen.Helvetica, reportText, "")
		cell(r.EmployeeName, reportCols[1].w, pdfgen.HelveticaBold, reportText, strings.Join(sub, " · "))
		cell(domain.ClockWIB(r.ClockIn, "—"), reportCols[2].w, pdfgen.Helvetica, reportText, "")
		cell(domain.ClockWIB(r.ClockOut, "—"), reportCols[3].w, pdfgen.Helvetica, reportText, "")
		cell(hours, reportCols[4].w, pdfgen.Helvetica, reportText, "")
		cell(status, reportCols[5].w, pdfgen.Helvetica, statusColor, "")
		drawPhoto(r.ClockInPhoto, x, y, reportCols[6].w)
		x += reportCols[6].w
		drawPhoto(r.ClockOutPhoto, x, y, reportCols[7].w)
		x += reportCols[7].w
		notes := ""
		if r.Notes != nil {
			notes = *r.Notes
		}
		d.Font(pdfgen.Helvetica, 8).Color(reportText)
		d.Text(notes, x+4, textY, pdfgen.TextOpts{Width: reportCols[8].w - 8, Height: reportRowH - 14, Ellipsis: true})

		y += reportRowH
		d.Line(reportMargin, y, reportMargin+reportTableW, y, reportLine, 0.5)
	}
	return d.Bytes()
}

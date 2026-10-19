package hris

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/xlsx"
)

// Jadwal Kerja per cabang (Spec 2026-10-19 §2.4–2.6): grid of the per-date
// schedule, the xlsx template HR fills in, and the upload that replaces the
// per-date rows of the employees in the file for the chosen range.

type scheduleBranch struct{ ID, Name, Code string }

type scheduleRange struct {
	Branch scheduleBranch
	Dates  []string
}

func (r *scheduleRange) from() string { return r.Dates[0] }
func (r *scheduleRange) to() string   { return r.Dates[len(r.Dates)-1] }

// parseScheduleRange checks the 7–45 day range and that the branch exists.
func parseScheduleRange(ctx context.Context, q database.Querier, branchID, from, to string) (*scheduleRange, error) {
	dates, msg := domain.ScheduleDates(from, to)
	if msg != "" {
		return nil, httpx.BadRequest(msg)
	}
	if !domain.UUIDRe.MatchString(branchID) {
		return nil, httpx.BadRequest("Pilih cabang")
	}
	var b scheduleBranch
	err := q.QueryRow(ctx, `SELECT id::text, name, coalesce(code, '') FROM configuration.branches WHERE id = $1`, branchID).
		Scan(&b.ID, &b.Name, &b.Code)
	if database.IsNoRows(err) {
		return nil, httpx.BadRequest("Cabang tidak ditemukan")
	}
	if err != nil {
		return nil, err
	}
	return &scheduleRange{Branch: b, Dates: dates}, nil
}

type gridEmployee struct {
	ID, NIP, Name, Position, ScheduleType string
	Active                                bool
	Placed                                map[string]bool
}

// branchEmployees: everyone (any status/type) with a placement at the
// branch overlapping the range, with the dates they are placed there.
func branchEmployees(ctx context.Context, q database.Querier, branchID string, dates []string) ([]gridEmployee, error) {
	rows, err := q.Query(ctx, `SELECT e.id::text, e.nip, e.full_name, coalesce(p2.title, ''), e.schedule_type, e.is_active,
		  p.start_date::text, coalesce(p.end_date::text, '')
		FROM hris.employee_placements p
		JOIN hris.employees e ON e.id = p.employee_id
		LEFT JOIN hris.positions p2 ON p2.id = e.job_title_id
		WHERE p.branch_id = $1::uuid AND p.start_date <= $3::date AND (p.end_date IS NULL OR p.end_date >= $2::date)
		ORDER BY coalesce(p2.title, ''), e.full_name`, branchID, dates[0], dates[len(dates)-1])
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := map[string]*gridEmployee{}
	var order []string
	for rows.Next() {
		var g gridEmployee
		var start, end string
		if err := rows.Scan(&g.ID, &g.NIP, &g.Name, &g.Position, &g.ScheduleType, &g.Active, &start, &end); err != nil {
			return nil, err
		}
		cur, ok := byID[g.ID]
		if !ok {
			g.Placed = map[string]bool{}
			cur = &g
			byID[g.ID] = cur
			order = append(order, g.ID)
		}
		for _, d := range dates {
			if d >= start && (end == "" || d <= end) {
				cur.Placed[d] = true
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]gridEmployee, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out, nil
}

type resolvedCell struct {
	Date   string `json:"date"`
	Code   string `json:"code"`   // kode shift, "L", "-" atau ""
	Source string `json:"source"` // day | weekly | none
}

// resolveCells resolves each date through hris.v_employee_schedule with the
// same rule as clock-in (domain.ResolveScheduleRow).
func resolveCells(ctx context.Context, q database.Querier, e gridEmployee, dates []string) ([]resolvedCell, error) {
	rows, err := q.Query(ctx, `SELECT es.day_of_week, es.shift_id::text, es.effective_from::text, es.effective_to::text,
		es.priority, coalesce(s.code, '') FROM hris.v_employee_schedule es
		LEFT JOIN hris.shifts s ON s.id = es.shift_id WHERE es.employee_id = $1 ORDER BY es.priority`, e.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pattern []domain.ScheduleRow
	var codes []string
	var prio []int
	for rows.Next() {
		var r domain.ScheduleRow
		var p int
		var code string
		if err := rows.Scan(&r.DayOfWeek, &r.ShiftID, &r.EffectiveFrom, &r.EffectiveTo, &p, &code); err != nil {
			return nil, err
		}
		pattern, codes, prio = append(pattern, r), append(codes, code), append(prio, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]resolvedCell, len(dates))
	for i, d := range dates {
		c := resolvedCell{Date: d, Source: "none"}
		switch idx := domain.ResolveScheduleRow(pattern, d); {
		case !e.Placed[d]:
			c.Code = domain.ScheduleNotPlaced
		case idx < 0:
		case pattern[idx].ShiftID == nil:
			c.Code, c.Source = domain.ScheduleOff, sourceOf(prio[idx])
		default:
			c.Code, c.Source = codes[idx], sourceOf(prio[idx])
		}
		out[i] = c
	}
	return out, nil
}

func sourceOf(priority int) string {
	if priority == 0 {
		return "day"
	}
	return "weekly"
}

func holidaysBetween(ctx context.Context, q database.Querier, from, to string) (map[string]string, error) {
	rows, err := q.Query(ctx, `SELECT holiday_date::text, name FROM hris.public_holidays
		WHERE deleted_at IS NULL AND status = 'aktif' AND holiday_date BETWEEN $1::date AND $2::date
		ORDER BY holiday_date, name`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var d, name string
		if err := rows.Scan(&d, &name); err != nil {
			return nil, err
		}
		if _, dup := out[d]; !dup {
			out[d] = name
		}
	}
	return out, rows.Err()
}

type shiftLite struct {
	ID        string `json:"-"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Break     int    `json:"break_minutes"`
	Overnight bool   `json:"-"`
	Active    bool   `json:"-"`
}

func loadShifts(ctx context.Context, q database.Querier, activeOnly bool) ([]shiftLite, error) {
	rows, err := q.Query(ctx, `SELECT id::text, code, name, start_time::text, end_time::text, break_minutes, is_overnight, is_active
		FROM hris.shifts WHERE ($1 = false OR is_active) ORDER BY sort_order NULLS LAST, code`, activeOnly)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []shiftLite
	for rows.Next() {
		var s shiftLite
		if err := rows.Scan(&s.ID, &s.Code, &s.Name, &s.StartTime, &s.EndTime, &s.Break, &s.Overnight, &s.Active); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

/* ── Grid ────────────────────────────────────────────────────────────── */

type gridDate struct {
	Date    string  `json:"date"`
	Label   string  `json:"label"`
	Holiday *string `json:"holiday"`
}

type gridRow struct {
	ID       string         `json:"id"`
	NIP      string         `json:"nip"`
	Name     string         `json:"name"`
	Position string         `json:"position"`
	Cells    []resolvedCell `json:"cells"`
}

func (s *Service) ScheduleGrid(ctx context.Context, branchID, from, to string) (map[string]any, error) {
	q := s.repo.querier()
	rng, err := parseScheduleRange(ctx, q, branchID, from, to)
	if err != nil {
		return nil, err
	}
	emps, err := branchEmployees(ctx, q, rng.Branch.ID, rng.Dates)
	if err != nil {
		return nil, err
	}
	rows := []gridRow{}
	nonShift := 0
	for _, e := range emps {
		if !e.Active {
			continue
		}
		if e.ScheduleType != scheduleShift {
			nonShift++
			continue
		}
		cells, err := resolveCells(ctx, q, e, rng.Dates)
		if err != nil {
			return nil, err
		}
		rows = append(rows, gridRow{ID: e.ID, NIP: e.NIP, Name: e.Name, Position: e.Position, Cells: cells})
	}
	hol, err := holidaysBetween(ctx, q, rng.from(), rng.to())
	if err != nil {
		return nil, err
	}
	dates := make([]gridDate, len(rng.Dates))
	for i, d := range rng.Dates {
		dates[i] = gridDate{Date: d, Label: domain.ScheduleDateHeader(d)}
		if name, ok := hol[d]; ok {
			n := name
			dates[i].Holiday = &n
		}
	}
	shifts, err := loadShifts(ctx, q, true)
	if err != nil {
		return nil, err
	}
	if shifts == nil {
		shifts = []shiftLite{}
	}
	return map[string]any{
		"branch":          map[string]string{"id": rng.Branch.ID, "name": rng.Branch.Name, "code": rng.Branch.Code},
		"dates":           dates,
		"employees":       rows,
		"non_shift_count": nonShift,
		"shifts":          shifts,
	}, nil
}

/* ── Template ────────────────────────────────────────────────────────── */

const (
	scheduleSheet     = "Jadwal"
	scheduleInfoSheet = "Info"
)

func isSunday(date string) bool {
	t, err := time.Parse(domain.DateLayout, date)
	return err == nil && t.Weekday() == time.Sunday
}

func (s *Service) ScheduleTemplate(ctx context.Context, branchID, from, to string) (*exportFile, error) {
	q := s.repo.querier()
	rng, err := parseScheduleRange(ctx, q, branchID, from, to)
	if err != nil {
		return nil, err
	}
	emps, err := branchEmployees(ctx, q, rng.Branch.ID, rng.Dates)
	if err != nil {
		return nil, err
	}
	hol, err := holidaysBetween(ctx, q, rng.from(), rng.to())
	if err != nil {
		return nil, err
	}
	shifts, err := loadShifts(ctx, q, true)
	if err != nil {
		return nil, err
	}
	options := make([]string, 0, len(shifts)+2)
	for _, sh := range shifts {
		options = append(options, sh.Code)
	}
	options = append(options, domain.ScheduleOff, domain.ScheduleNotPlaced)

	n := len(rng.Dates)
	firstDate, _ := colName(3)
	lastDate, _ := colName(3 + n - 1)
	cols := []xlsx.TemplateColumn{
		{Header: "NIP", ReadOnly: true, Width: 14},
		{Header: "Nama Karyawan", ReadOnly: true, Width: 26},
		{Header: "Jabatan", ReadOnly: true, Width: 20},
	}
	for _, d := range rng.Dates {
		c := xlsx.TemplateColumn{Header: domain.ScheduleDateHeader(d), Kind: xlsx.KindList, Options: options, Width: 10}
		if _, holiday := hol[d]; holiday || isSunday(d) {
			c.Fill = "#FDECEC"
		}
		cols = append(cols, c)
	}
	rangeOf := func(r int) string { return fmt.Sprintf("%s%d:%s%d", firstDate, r, lastDate, r) }
	cols = append(cols,
		xlsx.TemplateColumn{Header: "Hari Masuk", Kind: xlsx.KindFormula, Width: 11, Formula: func(r int) string {
			return fmt.Sprintf(`COUNTA(%s)-COUNTIF(%s,"L")-COUNTIF(%s,"-")`, rangeOf(r), rangeOf(r), rangeOf(r))
		}},
		xlsx.TemplateColumn{Header: "Hari Libur", Kind: xlsx.KindFormula, Width: 11, Formula: func(r int) string {
			return fmt.Sprintf(`COUNTIF(%s,"L")`, rangeOf(r))
		}},
	)
	var rows [][]any
	for _, e := range emps {
		if !e.Active || e.ScheduleType != scheduleShift {
			continue
		}
		cells, err := resolveCells(ctx, q, e, rng.Dates)
		if err != nil {
			return nil, err
		}
		row := []any{e.NIP, e.Name, e.Position}
		for _, c := range cells {
			code := c.Code
			if code == "" {
				code = "DEF"
			}
			row = append(row, code)
		}
		rows = append(rows, append(row, nil, nil))
	}
	title := fmt.Sprintf("Jadwal Kerja — %s — %s s/d %s", rng.Branch.Name, domain.FormatDateID(rng.from()), domain.FormatDateID(rng.to()))
	jadwal := xlsx.TemplateSheet{
		Name:       scheduleSheet,
		TitleLines: []string{title, "Isi dengan kode shift (lihat sheet Kode Shift). L = Libur. Tanda - = tidak ditempatkan di cabang ini (biarkan)."},
		Columns:    cols,
		Rows:       rows,
		FreezeCols: 3,
		Protect:    true,
		CellFill:   map[string]string{"L": "#E5E7EB", "-": "#F3F4F6"},
	}
	codeRows := [][]any{}
	for _, sh := range shifts {
		codeRows = append(codeRows, []any{sh.Code, sh.Name, sh.StartTime[:5], sh.EndTime[:5], fmt.Sprint(sh.Break)})
	}
	codeRows = append(codeRows, []any{"L", "Libur", "", "", ""}, []any{"-", "Tidak ditempatkan di cabang ini", "", "", ""})
	kode := xlsx.TemplateSheet{
		Name: "Kode Shift",
		Columns: []xlsx.TemplateColumn{
			{Header: "Kode", ReadOnly: true, Width: 10}, {Header: "Nama Shift", ReadOnly: true, Width: 30},
			{Header: "Jam Masuk", ReadOnly: true, Width: 11}, {Header: "Jam Pulang", ReadOnly: true, Width: 11},
			{Header: "Istirahat (menit)", ReadOnly: true, Width: 16},
		},
		Rows:    codeRows,
		Protect: true,
	}
	notes := []xlsx.SheetSpec{{Name: "Petunjuk", ColumnWidths: []float64{110}, Rows: [][]any{
		{"Cara mengisi jadwal kerja"},
		{"1. Buka sheet \"Jadwal\". Setiap baris = satu karyawan Shift di cabang ini, setiap kolom = satu tanggal."},
		{"2. Isi sel tanggal dengan kode shift dari sheet \"Kode Shift\" (bisa dipilih dari daftar), atau L untuk libur."},
		{"3. Jangan ubah NIP, nama, jabatan, atau judul tanggal. Tanda - berarti karyawan tidak ditempatkan di cabang ini pada tanggal itu."},
		{"4. Simpan file, lalu unggah di halaman Jadwal Kerja. Periksa hasil pengecekan sebelum menyimpan."},
		{""},
		{"File akan ditolak bila:"},
		{"- file bukan template cabang/tanggal yang dipilih, atau kolom tanggal bergeser"},
		{"- tanggal sudah lewat atau periode absensinya sudah closing"},
		{"- NIP tidak dikenal, sudah resign, tidak ditempatkan di cabang ini, ganda, atau karyawan Non-Shift"},
		{"- kode shift salah ketik / sudah nonaktif, atau sel dikosongkan"},
		{""},
		{"Peringatan (boleh lanjut setelah diperiksa):"},
		{"- masuk lebih dari 6 hari berturut-turut, jeda antar shift kurang dari 8 jam"},
		{"- diberi shift saat cuti/izin yang disetujui atau saat libur nasional"},
		{"- tidak ada yang masuk di suatu tanggal, atau jadwal hari ini diubah padahal sudah ada yang clock-in"},
		{""},
		{"Contoh baris yang benar: 202505272 | Feti | Kasir | SPG1 | SPG1 | L | SPG2 | ..."},
	}}}
	stamp := time.Now().UTC().Add(domain.WIBOffset).Format("2006-01-02 15:04")
	info := &xlsx.InfoSheet{Name: scheduleInfoSheet, Pairs: [][2]string{
		{"Cabang ID", rng.Branch.ID}, {"Dari", rng.from()}, {"Sampai", rng.to()}, {"Diunduh", stamp},
	}}
	data, err := xlsx.BuildTemplate([]xlsx.TemplateSheet{jadwal, kode}, notes, info)
	if err != nil {
		return nil, err
	}
	code := rng.Branch.Code
	if code == "" {
		code = rng.Branch.Name
	}
	name := fmt.Sprintf("Jadwal Kerja - %s - %s sd %s.xlsx", code, dashDate(rng.from(), false), dashDate(rng.to(), true))
	return &exportFile{data, xlsx.ContentType, name}, nil
}

// dashDate is "03-11" or "03-11-2026" for file names.
func dashDate(iso string, withYear bool) string {
	t, err := time.Parse(domain.DateLayout, iso)
	if err != nil {
		return iso
	}
	if withYear {
		return t.Format("02-01-2006")
	}
	return t.Format("02-01")
}

// colName is the A1 column letter of a 0-based index.
func colName(i int) (string, error) {
	name := ""
	for n := i + 1; n > 0; n = (n - 1) / 26 {
		name = string(rune('A'+(n-1)%26)) + name
	}
	return name, nil
}

/* ── Upload ──────────────────────────────────────────────────────────── */

func uploadContext(ctx context.Context, q database.Querier, rng *scheduleRange, today string, nips []string) (domain.ScheduleUploadContext, map[string]string, error) {
	c := domain.ScheduleUploadContext{
		BranchID: rng.Branch.ID, BranchName: rng.Branch.Name, Dates: rng.Dates, Today: today,
		Employees: map[string]domain.UploadEmployee{}, Shifts: map[string]domain.UploadShift{},
		ClosedDates: map[string]bool{}, OnLeave: map[string]map[string]bool{}, ClockedIn: map[string]map[string]bool{},
	}
	names := map[string]string{} // employee id → name
	emps, err := branchEmployees(ctx, q, rng.Branch.ID, rng.Dates)
	if err != nil {
		return c, nil, err
	}
	for _, e := range emps {
		c.Employees[e.NIP] = domain.UploadEmployee{ID: e.ID, NIP: e.NIP, Name: e.Name, Position: e.Position,
			ScheduleType: e.ScheduleType, Active: e.Active, Placed: e.Placed}
		names[e.ID] = e.Name
	}
	if len(nips) > 0 {
		rows, err := q.Query(ctx, `SELECT id::text, nip, full_name, schedule_type, is_active FROM hris.employees WHERE nip = ANY($1)`, nips)
		if err != nil {
			return c, nil, err
		}
		for rows.Next() {
			var e domain.UploadEmployee
			if err := rows.Scan(&e.ID, &e.NIP, &e.Name, &e.ScheduleType, &e.Active); err != nil {
				rows.Close()
				return c, nil, err
			}
			if _, placed := c.Employees[e.NIP]; !placed {
				e.Placed = map[string]bool{}
				c.Employees[e.NIP] = e
				names[e.ID] = e.Name
			}
		}
		rows.Close()
	}
	shifts, err := loadShifts(ctx, q, false)
	if err != nil {
		return c, nil, err
	}
	for _, sh := range shifts {
		c.Shifts[strings.ToUpper(sh.Code)] = domain.UploadShift{ID: sh.ID, Code: sh.Code, Name: sh.Name,
			Start: sh.StartTime, End: sh.EndTime, Overnight: sh.Overnight, Active: sh.Active}
	}
	for _, d := range rng.Dates {
		p, err := periodContaining(ctx, q, d, today)
		if err != nil {
			return c, nil, err
		}
		if p != nil && p.Closed() {
			c.ClosedDates[d] = true
		}
	}
	if c.Holidays, err = holidaysBetween(ctx, q, rng.from(), rng.to()); err != nil {
		return c, nil, err
	}
	leaves, err := q.Query(ctx, `SELECT employee_id::text, g::date::text
		FROM hris.leaves, generate_series(greatest(start_date, $1::date), least(end_date, $2::date), interval '1 day') g
		WHERE status = 'approved' AND start_date <= $2::date AND end_date >= $1::date`, rng.from(), rng.to())
	if err != nil {
		return c, nil, err
	}
	for leaves.Next() {
		var id, d string
		if err := leaves.Scan(&id, &d); err != nil {
			leaves.Close()
			return c, nil, err
		}
		if c.OnLeave[id] == nil {
			c.OnLeave[id] = map[string]bool{}
		}
		c.OnLeave[id][d] = true
	}
	leaves.Close()
	clocked, err := q.Query(ctx, `SELECT employee_id::text FROM hris.attendance WHERE date = $1::date AND clock_in IS NOT NULL`, today)
	if err != nil {
		return c, nil, err
	}
	for clocked.Next() {
		var id string
		if err := clocked.Scan(&id); err != nil {
			clocked.Close()
			return c, nil, err
		}
		c.ClockedIn[id] = map[string]bool{today: true}
	}
	clocked.Close()
	return c, names, nil
}

// assignmentKeys are the (employee, date) pairs an upload writes: only the
// dates each employee is placed at this branch, so per-date rows another
// branch uploaded for its own days are never touched (review C1).
func assignmentKeys(as []domain.ScheduleAssignment) ([]string, []string) {
	ids, dates := make([]string, len(as)), make([]string, len(as))
	for i, a := range as {
		ids[i], dates[i] = a.EmployeeID, a.Date
	}
	return ids, dates
}

func scheduleFingerprint(ctx context.Context, q database.Querier, as []domain.ScheduleAssignment) (string, error) {
	ids, dates := assignmentKeys(as)
	var fp string
	err := q.QueryRow(ctx, `SELECT coalesce(md5(string_agg(d.employee_id::text || d.work_date::text || coalesce(d.shift_id::text, 'L'), ','
		ORDER BY d.employee_id, d.work_date)), '')
		FROM hris.employee_shift_days d
		JOIN unnest($1::uuid[], $2::date[]) AS k(employee_id, work_date)
		  ON k.employee_id = d.employee_id AND k.work_date = d.work_date`, ids, dates).Scan(&fp)
	return fp, err
}

type scheduleChange struct {
	NIP    string `json:"nip"`
	Name   string `json:"name"`
	Date   string `json:"date"`
	Before string `json:"before"`
	After  string `json:"after"`
}

type scheduleSummary struct {
	Employees int `json:"employees"`
	New       int `json:"new"`
	Changed   int `json:"changed"`
	Same      int `json:"same"`
}

type scheduleImportInput struct {
	BranchID, From, To string
	Data               []byte
	FileName           string
	Step               string // preview | commit | error_file
	Fingerprint        string
	AckWarnings        bool
}

const maxScheduleChanges = 500

func (s *Service) ImportSchedule(ctx context.Context, actor *auth.User, in scheduleImportInput) (map[string]any, *exportFile, error) {
	if !strings.HasSuffix(strings.ToLower(in.FileName), ".xlsx") {
		return nil, nil, httpx.BadRequest("Unggah file .xlsx dari template Jadwal Kerja")
	}
	matrix, err := xlsx.ParseMatrix(in.Data, xlsx.ReadOptions{PreferSheet: scheduleSheet})
	if err != nil {
		return nil, nil, httpx.BadRequest("File tidak bisa dibaca — pastikan formatnya .xlsx")
	}
	info, err := xlsx.ParseMatrix(in.Data, xlsx.ReadOptions{PreferSheet: scheduleInfoSheet})
	if err != nil || len(info) == 0 || len(info[0]) == 0 || domain.NormalizeHeader(info[0][0]) != "cabang id" {
		info = nil
	}
	file, msg := domain.ReadScheduleFile(matrix, info)
	if msg != "" {
		return nil, nil, httpx.BadRequest(msg)
	}
	today := domain.TodayWIB(s.clock())
	var out map[string]any
	var errFile *exportFile
	err = s.repo.InTx(ctx, func(r Repository) error {
		q := r.querier()
		rng, err := parseScheduleRange(ctx, q, in.BranchID, in.From, in.To)
		if err != nil {
			return err
		}
		nips := make([]string, 0, len(file.Rows))
		for _, row := range file.Rows {
			if nip, m := domain.CleanNIP(row.NIP); m == "" && nip != "" {
				nips = append(nips, nip)
			}
		}
		uctx, names, err := uploadContext(ctx, q, rng, today, nips)
		if err != nil {
			return err
		}
		res := domain.ValidateScheduleUpload(uctx, file)
		if res.Errors == nil {
			res.Errors = []domain.UploadIssue{}
		}
		if res.Warnings == nil {
			res.Warnings = []domain.UploadIssue{}
		}
		if in.Step == "error_file" {
			data, err := errorWorkbook(matrix, file.HeaderLine-1, append(append([]domain.UploadIssue{}, res.Errors...), res.Warnings...), scheduleSheet)
			if err != nil {
				return err
			}
			errFile = &exportFile{data, xlsx.ContentType, "Error - " + in.FileName}
			return nil
		}
		summary, changes, err := scheduleDiff(ctx, q, uctx, names, res)
		if err != nil {
			return err
		}
		fp, err := scheduleFingerprint(ctx, q, res.Assignments)
		if err != nil {
			return err
		}
		if in.Step != "commit" {
			out = map[string]any{"summary": summary, "errors": res.Errors, "warnings": res.Warnings,
				"fingerprint": fp, "changes": changes}
			return nil
		}
		switch {
		case len(res.Errors) > 0:
			return httpx.BadRequest("File masih punya error — perbaiki lalu unggah ulang")
		case len(res.Warnings) > 0 && !in.AckWarnings:
			return httpx.BadRequest(`Centang "Saya sudah memeriksa peringatan" sebelum menyimpan`)
		case fp != in.Fingerprint:
			return httpx.Conflict("Jadwal sudah diubah orang lain sejak preview — unggah ulang file")
		}
		var batch string
		if err := q.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&batch); err != nil {
			return err
		}
		ids, dates := assignmentKeys(res.Assignments)
		if _, err := q.Exec(ctx, `DELETE FROM hris.employee_shift_days d
			USING unnest($1::uuid[], $2::date[]) AS k(employee_id, work_date)
			WHERE d.employee_id = k.employee_id AND d.work_date = k.work_date`, ids, dates); err != nil {
			return err
		}
		days := map[string]int{}
		for _, a := range res.Assignments {
			if _, err := q.Exec(ctx, `INSERT INTO hris.employee_shift_days
				(employee_id, work_date, shift_id, branch_id, batch_id, created_by_name)
				VALUES ($1, $2::date, $3, $4, $5, $6)`, a.EmployeeID, a.Date, a.ShiftID, rng.Branch.ID, batch, actor.FullName); err != nil {
				return err
			}
			days[a.EmployeeID]++
		}
		for _, id := range res.EmployeeIDs {
			if err := writeEmployeeAudit(ctx, q, actor, "schedule.bulk_upload", id, names[id], nil, map[string]any{
				"branch": rng.Branch.Name, "from": rng.from(), "to": rng.to(), "batch_id": batch, "days": days[id]}); err != nil {
				return err
			}
		}
		out = map[string]any{"summary": summary, "message": fmt.Sprintf("Jadwal %d karyawan disimpan (%s–%s)",
			len(res.EmployeeIDs), domain.FormatDateID(rng.from()), domain.FormatDateID(rng.to()))}
		return nil
	})
	return out, errFile, err
}

// scheduleDiff compares the upload with the schedule that applies today.
func scheduleDiff(ctx context.Context, q database.Querier, c domain.ScheduleUploadContext, names map[string]string,
	res domain.ScheduleUploadResult) (scheduleSummary, []scheduleChange, error) {
	sum := scheduleSummary{Employees: len(res.EmployeeIDs)}
	changes := []scheduleChange{}
	codeOf := map[string]string{}
	for _, sh := range c.Shifts {
		codeOf[sh.ID] = sh.Code
	}
	nipOf := map[string]gridEmployee{}
	for _, e := range c.Employees {
		nipOf[e.ID] = gridEmployee{ID: e.ID, NIP: e.NIP, Name: e.Name, Placed: e.Placed}
	}
	current := map[string]map[string]string{}
	for _, id := range res.EmployeeIDs {
		cells, err := resolveCells(ctx, q, nipOf[id], c.Dates)
		if err != nil {
			return sum, nil, err
		}
		current[id] = map[string]string{}
		for _, cell := range cells {
			current[id][cell.Date] = cell.Code
		}
	}
	for _, a := range res.Assignments {
		after := domain.ScheduleOff
		if a.ShiftID != nil {
			after = codeOf[*a.ShiftID]
		}
		before := current[a.EmployeeID][a.Date]
		switch {
		case before == "":
			sum.New++
		case before == after:
			sum.Same++
			continue
		default:
			sum.Changed++
		}
		if len(changes) < maxScheduleChanges {
			changes = append(changes, scheduleChange{NIP: nipOf[a.EmployeeID].NIP, Name: names[a.EmployeeID],
				Date: a.Date, Before: before, After: after})
		}
	}
	return sum, changes, nil
}

/* ── HTTP ────────────────────────────────────────────────────────────── */

func (h handlers) getSchedules(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	q := r.URL.Query()
	data, err := h.svc.ScheduleGrid(r.Context(), q.Get("branch_id"), q.Get("from"), q.Get("to"))
	if err != nil {
		return err
	}
	return ok(w, dataOf(data))
}

func writeFile(w http.ResponseWriter, f *exportFile) error {
	hd := w.Header()
	hd.Set("Content-Type", f.ContentType)
	hd.Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(f.FileName, `"`, "")+
		`"; filename*=UTF-8''`+url.PathEscape(f.FileName))
	w.WriteHeader(http.StatusOK)
	_, err := w.Write(f.Data)
	return err
}

func (h handlers) getScheduleTemplate(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	q := r.URL.Query()
	f, err := h.svc.ScheduleTemplate(r.Context(), q.Get("branch_id"), q.Get("from"), q.Get("to"))
	if err != nil {
		return err
	}
	return writeFile(w, f)
}

func (h handlers) importSchedules(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		return httpx.BadRequest("Unggah file .xlsx dari template Jadwal Kerja")
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return httpx.BadRequest("Unggah file .xlsx dari template Jadwal Kerja")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 16<<20))
	if err != nil {
		return err
	}
	in := scheduleImportInput{
		BranchID: r.FormValue("branch_id"), From: r.FormValue("from"), To: r.FormValue("to"),
		Data: data, FileName: header.Filename, Step: r.FormValue("step"),
		Fingerprint: r.FormValue("fingerprint"), AckWarnings: r.FormValue("ack_warnings") == "true",
	}
	out, errFile, err := h.svc.ImportSchedule(r.Context(), u, in)
	if err != nil {
		return err
	}
	if errFile != nil {
		return writeFile(w, errFile)
	}
	if in.Step == "commit" {
		msg, _ := out["message"].(string)
		return ok(w, dataMsg(map[string]any{"summary": out["summary"]}, msg))
	}
	return ok(w, dataOf(out))
}

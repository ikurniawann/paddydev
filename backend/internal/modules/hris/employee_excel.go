package hris

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/xlsx"
)

// Data Karyawan export (and the empty import template) — Spec 2026-10-19
// §1.4 and §3.2: columns and values in the words HR uses, never database
// names, so an exported file can be edited and imported back.

const employeeSheet = "Data Karyawan"

type employeeExportFilter struct {
	Search, DepartmentID, EmploymentStatus, ScheduleType string
	IsActive                                             *bool
}

func exportFilterFrom(r *http.Request) employeeExportFilter {
	q := r.URL.Query()
	f := employeeExportFilter{
		Search: strings.TrimSpace(q.Get("search")), DepartmentID: q.Get("department_id"),
		EmploymentStatus: q.Get("employment_status"), ScheduleType: q.Get("schedule_type"),
	}
	switch q.Get("is_active") {
	case "true":
		t := true
		f.IsActive = &t
	case "false":
		v := false
		f.IsActive = &v
	}
	return f
}

// employeeLists are the dropdown values of the file, from active masters.
type employeeLists struct {
	statusName  map[string]string // code → name (all statuses)
	statusCodes map[string]string // lower(name) → code (selectable only)
	options     map[string][]string
	branches    map[string]string // lower(name) → id
	departments map[string]string
	positions   map[string]string
}

func loadEmployeeLists(ctx context.Context, q database.Querier) (*employeeLists, error) {
	l := &employeeLists{statusName: map[string]string{}, statusCodes: map[string]string{}, options: map[string][]string{},
		branches: map[string]string{}, departments: map[string]string{}, positions: map[string]string{}}
	type src struct {
		key, sql string
		into     map[string]string
	}
	// Built-in names first, the master overrides them; inactive master
	// statuses and resign/terminated are not selectable.
	selectable := map[string]bool{}
	var order []string
	for _, c := range domain.EmploymentStatusLabels {
		l.statusName[c.Value] = c.Label
		selectable[c.Value] = c.Value != "resigned" && c.Value != "terminated"
		order = append(order, c.Value)
	}
	rows, err := q.Query(ctx, `SELECT code, name, is_active FROM hris.employment_statuses ORDER BY name`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var code, name string
		var active bool
		if err := rows.Scan(&code, &name, &active); err != nil {
			rows.Close()
			return nil, err
		}
		if _, known := l.statusName[code]; !known {
			order = append(order, code)
		}
		l.statusName[code] = name
		selectable[code] = active && code != "resigned" && code != "terminated"
	}
	rows.Close()
	for _, code := range order {
		if selectable[code] {
			l.statusCodes[strings.ToLower(l.statusName[code])] = code
			l.options["employment_status"] = append(l.options["employment_status"], l.statusName[code])
		}
	}
	for _, s := range []src{
		{"branch", `SELECT id::text, name FROM configuration.branches WHERE is_active ORDER BY name`, l.branches},
		{"department", `SELECT id::text, name FROM hris.departments WHERE coalesce(is_active, true) ORDER BY name`, l.departments},
		{"position", `SELECT id::text, title FROM hris.positions WHERE coalesce(is_active, true) ORDER BY title`, l.positions},
	} {
		rows, err := q.Query(ctx, s.sql)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, name string
			if err := rows.Scan(&id, &name); err != nil {
				rows.Close()
				return nil, err
			}
			if _, dup := s.into[strings.ToLower(name)]; !dup {
				s.into[strings.ToLower(name)] = id
				l.options[s.key] = append(l.options[s.key], name)
			}
		}
		rows.Close()
	}
	l.options["schedule_type"] = domain.Labels(domain.ScheduleTypeLabels)
	l.options["gender"] = domain.Labels(domain.GenderLabels)
	l.options["marital_status"] = domain.Labels(domain.MaritalLabels)
	return l, nil
}

func parseISODate(s string) any {
	if s == "" {
		return nil
	}
	t, err := time.Parse(domain.DateLayout, s[:min(len(s), 10)])
	if err != nil {
		return s
	}
	return t
}

func (s *Service) exportEmployeeRows(ctx context.Context, q database.Querier, f employeeExportFilter, l *employeeLists) ([][]any, error) {
	var where []string
	var args []any
	param := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if f.Search != "" {
		p := param("%" + f.Search + "%")
		where = append(where, "(e.full_name ILIKE "+p+" OR e.email ILIKE "+p+" OR e.nip ILIKE "+p+")")
	}
	if domain.UUIDRe.MatchString(f.DepartmentID) {
		where = append(where, "e.department_id = "+param(f.DepartmentID)+"::uuid")
	}
	if f.EmploymentStatus != "" {
		where = append(where, "e.employment_status = "+param(f.EmploymentStatus))
	}
	if f.ScheduleType == scheduleShift || f.ScheduleType == scheduleNonShift {
		where = append(where, "e.schedule_type = "+param(f.ScheduleType))
	}
	if f.IsActive != nil {
		where = append(where, "e.is_active = "+param(*f.IsActive))
	}
	clause := ""
	if len(where) > 0 {
		clause = "WHERE " + strings.Join(where, " AND ")
	}
	rows, err := q.Query(ctx, `SELECT e.nip, e.full_name, e.email, coalesce(e.phone, ''), e.join_date::text, e.employment_status,
		e.schedule_type, coalesce(b.name, ''), coalesce(d.name, ''), coalesce(p.title, ''), coalesce(m.nip, ''),
		coalesce(e.ktp, ''), coalesce(e.npwp, ''), coalesce(e.birth_date::text, ''), coalesce(e.gender::text, ''),
		coalesce(e.marital_status::text, ''), coalesce(e.address, ''), coalesce(e.city, ''), coalesce(e.province, ''),
		coalesce(e.postal_code, ''), coalesce(e.bank_name, ''), coalesce(e.bank_account, ''), coalesce(e.bpjs_tk, ''),
		coalesce(e.bpjs_kesehatan, ''), coalesce(e.emergency_contact_name, ''), coalesce(e.emergency_contact_phone, ''),
		coalesce(e.emergency_contact_relationship, ''), coalesce(m.full_name, ''), e.is_active,
		coalesce(e.end_date::text, ''), coalesce((e.updated_at AT TIME ZONE 'Asia/Jakarta')::date::text, '')
		FROM hris.employees e
		LEFT JOIN LATERAL (SELECT br.name FROM hris.employee_placements ep JOIN configuration.branches br ON br.id = ep.branch_id
		                   WHERE ep.employee_id = e.id AND ep.is_primary AND ep.start_date <= current_date
		                     AND (ep.end_date IS NULL OR ep.end_date >= current_date)
		                   ORDER BY ep.start_date DESC LIMIT 1) b ON true
		LEFT JOIN hris.departments d ON d.id = e.department_id
		LEFT JOIN hris.positions p ON p.id = e.job_title_id
		LEFT JOIN hris.employees m ON m.id = e.reporting_to
		`+clause+` ORDER BY e.full_name LIMIT 5000`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]any
	for rows.Next() {
		v := make([]string, 31)
		var active bool
		ptrs := make([]any, 0, 31)
		for i := 0; i < 28; i++ {
			ptrs = append(ptrs, &v[i])
		}
		ptrs = append(ptrs, &active, &v[29], &v[30])
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		status := l.statusName[v[5]]
		if status == "" {
			status = v[5]
		}
		activeLabel := "Resign"
		if active {
			activeLabel = "Aktif"
		}
		out = append(out, []any{
			v[0], v[1], v[2], v[3], parseISODate(v[4]), status,
			domain.LabelOf(domain.ScheduleTypeLabels, v[6]), v[7], v[8], v[9], v[10],
			v[11], v[12], parseISODate(v[13]), domain.LabelOf(domain.GenderLabels, v[14]),
			domain.LabelOf(domain.MaritalLabels, v[15]), v[16], v[17], v[18],
			v[19], v[20], v[21], v[22],
			v[23], v[24], v[25],
			v[26], v[27], activeLabel, parseISODate(v[29]), parseISODate(v[30]),
		})
	}
	return out, rows.Err()
}

// employeeWorkbook builds the Data Karyawan file: Petunjuk, the data sheet
// with dropdowns from the masters, hidden option sheet.
func employeeWorkbook(rows [][]any, l *employeeLists) ([]byte, error) {
	cols := make([]xlsx.TemplateColumn, len(domain.EmployeeColumns))
	listKey := map[string]string{"employment_status": "employment_status", "schedule_type": "schedule_type",
		"branch": "branch", "department": "department", "position": "position", "gender": "gender",
		"marital_status": "marital_status"}
	for i, c := range domain.EmployeeColumns {
		col := xlsx.TemplateColumn{Header: c.Header, Required: c.Required, Width: c.Width, ReadOnly: c.ExportOnly}
		switch c.Kind {
		case "date":
			col.Kind = xlsx.KindDate
		case "list":
			col.Kind = xlsx.KindList
			col.Options = l.options[listKey[c.Key]]
		}
		cols[i] = col
	}
	data := xlsx.TemplateSheet{Name: employeeSheet, Columns: cols, Rows: rows, FreezeCols: 2, DataRows: max(len(rows), 200)}
	notes := [][]any{
		{"Cara mengisi Data Karyawan"},
		{"1. Isi sheet \"Data Karyawan\": satu baris = satu karyawan. Kolom bertanda * wajib untuk karyawan baru."},
		{"2. Kolom pilihan (Status Kepegawaian, Tipe Jadwal, Cabang Utama, Departemen, Jabatan, Jenis Kelamin, Status Pernikahan) pilih dari daftar."},
		{"3. Tanggal ditulis DD/MM/YYYY, mis. 21/10/2026. NIK, NPWP, No. HP, No. Rekening, No. BPJS ditulis sebagai teks (jangan diubah ke angka)."},
		{"4. Simpan lalu unggah di halaman Karyawan → Import. Periksa hasil pengecekan sebelum menyimpan."},
		{""},
		{"Mode Tambah: hanya karyawan baru; NIP yang sudah ada ditolak. NIP kosong = dibuat otomatis."},
		{"Mode Timpa: NIP yang ada diperbarui sesuai file; sel kosong TIDAK mengubah data; NIP baru ditambahkan."},
		{"Kolom abu-abu di paling kanan (Nama Atasan, Status Aktif, Tanggal Keluar, Terakhir Diubah) hanya informasi, tidak dibaca saat import."},
		{""},
		{"Kolom", "Wajib", "Isi"},
	}
	hints := map[string]string{
		"nip": "Teks. Kosong di mode Tambah = dibuat otomatis; wajib di mode Timpa", "phone": "Diawali 08 / 62 / +62",
		"join_date": "DD/MM/YYYY", "birth_date": "DD/MM/YYYY", "ktp": "16 angka", "npwp": "15–16 angka",
		"manager_nip": "NIP atasan langsung", "email": "Alamat email aktif",
	}
	for _, c := range domain.EmployeeColumns {
		req := ""
		if c.Required {
			req = "Ya"
		}
		hint := hints[c.Key]
		if opts := l.options[listKey[c.Key]]; c.Kind == "list" && len(opts) > 0 && len(opts) <= 8 {
			hint = "Pilih: " + strings.Join(opts, ", ")
		} else if c.Kind == "list" {
			hint = "Pilih dari daftar"
		}
		if c.ExportOnly {
			hint = "Hanya informasi (export)"
		}
		notes = append(notes, []any{c.Header, req, hint})
	}
	notes = append(notes, []any{""}, []any{"Contoh baris:", "", "202505272 | Feti Puspitasari | feti@gmail.com | 081234567890 | 01/06/2025 | Kontrak (PKWT) | Shift | PPLN | Retail Operational | Shopkeeper Staff"})
	return xlsx.BuildTemplate([]xlsx.TemplateSheet{data}, []xlsx.SheetSpec{{Name: "Petunjuk", Rows: notes, ColumnWidths: []float64{28, 8, 90}}}, nil)
}

func (s *Service) ExportEmployees(ctx context.Context, f employeeExportFilter, template bool) (*exportFile, error) {
	q := s.repo.querier()
	l, err := loadEmployeeLists(ctx, q)
	if err != nil {
		return nil, err
	}
	var rows [][]any
	if !template {
		if rows, err = s.exportEmployeeRows(ctx, q, f, l); err != nil {
			return nil, err
		}
	}
	data, err := employeeWorkbook(rows, l)
	if err != nil {
		return nil, err
	}
	name := "Template Import Karyawan.xlsx"
	if !template {
		name = "Data Karyawan - " + time.Now().UTC().Add(domain.WIBOffset).Format("02-01-2006") + ".xlsx"
	}
	return &exportFile{data, xlsx.ContentType, name}, nil
}

func (h handlers) exportEmployees(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	f, err := h.svc.ExportEmployees(r.Context(), exportFilterFrom(r), false)
	if err != nil {
		return err
	}
	return writeFile(w, f)
}

func (h handlers) employeeImportTemplate(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	f, err := h.svc.ExportEmployees(r.Context(), employeeExportFilter{}, true)
	if err != nil {
		return err
	}
	return writeFile(w, f)
}

/* ── Import (Spec §1.5) ──────────────────────────────────────────────── */

type employeeImportInput struct {
	Data          []byte
	FileName      string
	Mode          string // add | replace
	Step          string // preview | commit | error_file
	ReleaseEmails bool
}

type employeeImportChange struct {
	Line   int    `json:"line"`
	NIP    string `json:"nip"`
	Name   string `json:"name"`
	Field  string `json:"field"`
	Before string `json:"before"`
	After  string `json:"after"`
}

func readEmployeeMatrix(data []byte, fileName string) ([][]string, error) {
	if strings.HasSuffix(strings.ToLower(fileName), ".csv") {
		return xlsx.SpreadsheetMatrix(data, fileName)
	}
	return xlsx.ParseMatrix(data, xlsx.ReadOptions{PreferSheet: employeeSheet})
}

func importLookups(ctx context.Context, q database.Querier, l *employeeLists, today string) (domain.EmployeeLookups, error) {
	lk := domain.EmployeeLookups{Statuses: l.statusCodes, Branches: l.branches, Departments: l.departments,
		Positions: l.positions, ExistingNIP: map[string]domain.EmployeeRef{}, EmailOwner: map[string]domain.EmployeeRef{},
		ManagerNIP: map[string]string{}, Today: today}
	rows, err := q.Query(ctx, `SELECT id::text, nip, full_name, lower(email), is_active FROM hris.employees ORDER BY is_active, updated_at NULLS FIRST`)
	if err != nil {
		return lk, err
	}
	defer rows.Close()
	for rows.Next() {
		var r domain.EmployeeRef
		var email string
		if err := rows.Scan(&r.ID, &r.NIP, &r.Name, &email, &r.Active); err != nil {
			return lk, err
		}
		lk.ExistingNIP[r.NIP] = r
		lk.EmailOwner[email] = r // active rows come last, so they win
		if r.Active {
			lk.ManagerNIP[r.NIP] = r.ID
		}
	}
	return lk, rows.Err()
}

// displayValue turns a column value into the label the file shows.
func displayValue(key string, v any, l *employeeLists, names map[string]string) string {
	if v == nil {
		return ""
	}
	s := fmt.Sprint(v)
	switch key {
	case "employment_status":
		if n := l.statusName[s]; n != "" {
			return n
		}
	case "schedule_type":
		return domain.LabelOf(domain.ScheduleTypeLabels, s)
	case "gender":
		return domain.LabelOf(domain.GenderLabels, s)
	case "marital_status":
		return domain.LabelOf(domain.MaritalLabels, s)
	case "join_date", "birth_date":
		return domain.FormatDateID(s[:min(len(s), 10)])
	case "department_id", "job_title_id", "reporting_to", "branch":
		if n := names[s]; n != "" {
			return n
		}
	}
	return s
}

var fieldHeader = map[string]string{"department_id": "Departemen", "job_title_id": "Jabatan", "reporting_to": "NIP Atasan", "branch": "Cabang Utama"}

func headerOf(key string) string {
	if h := fieldHeader[key]; h != "" {
		return h
	}
	for _, c := range domain.EmployeeColumns {
		if c.Key == key {
			return c.Header
		}
	}
	return key
}

// currentValues are the hris.employees columns (as text) and primary branch.
func currentValues(ctx context.Context, q database.Querier, id string) (map[string]any, string, error) {
	var raw []byte
	var branch *string
	err := q.QueryRow(ctx, `SELECT to_jsonb(e), (SELECT ep.branch_id::text FROM hris.employee_placements ep
		WHERE ep.employee_id = e.id AND ep.is_primary AND ep.start_date <= current_date
		  AND (ep.end_date IS NULL OR ep.end_date >= current_date) ORDER BY ep.start_date DESC LIMIT 1)
		FROM hris.employees e WHERE e.id = $1`, id).Scan(&raw, &branch)
	if err != nil {
		return nil, "", err
	}
	m := map[string]any{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, "", err
	}
	b := ""
	if branch != nil {
		b = *branch
	}
	return m, b, nil
}

func sameValue(cur any, next any) bool {
	if cur == nil {
		return next == nil || fmt.Sprint(next) == ""
	}
	c := fmt.Sprint(cur)
	n := fmt.Sprint(next)
	if len(c) >= 10 && len(n) == 10 && c[4] == '-' && n[4] == '-' {
		return c[:10] == n
	}
	return c == n
}

func (s *Service) ImportEmployees(ctx context.Context, actor *auth.User, in employeeImportInput) (map[string]any, *exportFile, error) {
	if in.Mode != "add" && in.Mode != "replace" {
		return nil, nil, httpx.BadRequest("Pilih mode Tambah atau Timpa")
	}
	matrix, err := readEmployeeMatrix(in.Data, in.FileName)
	if err != nil || len(matrix) == 0 {
		return nil, nil, httpx.BadRequest("File tidak bisa dibaca — pakai template .xlsx dari halaman Karyawan")
	}
	today := domain.TodayWIB(s.clock())
	var out map[string]any
	var errFile *exportFile
	err = s.repo.InTx(ctx, func(r Repository) error {
		q := r.querier()
		l, err := loadEmployeeLists(ctx, q)
		if err != nil {
			return err
		}
		lk, err := importLookups(ctx, q, l, today)
		if err != nil {
			return err
		}
		rows, errs, warns := domain.ValidateEmployeeImport(in.Mode, matrix[0], matrix[1:], 2, lk)
		if in.Step == "error_file" {
			data, err := errorWorkbook(matrix, 0, append(append([]domain.UploadIssue{}, errs...), warns...), employeeSheet)
			if err != nil {
				return err
			}
			errFile = &exportFile{data, xlsx.ContentType, "Error - " + in.FileName}
			return nil
		}
		names := map[string]string{}
		for n, id := range l.branches {
			names[id] = n
		}
		for n, id := range l.departments {
			names[id] = n
		}
		for n, id := range l.positions {
			names[id] = n
		}
		for nip, id := range lk.ManagerNIP {
			names[id] = nip
		}
		// Diff each update row against today's data: unchanged cells drop out.
		summary := map[string]int{"create": 0, "update": 0, "skip": 0}
		changes := []employeeImportChange{}
		list := []map[string]any{}
		type plan struct {
			row    domain.EmployeeImportRow
			id     string
			before map[string]any
			branch string // new branch to place, "" = keep
		}
		var plans []plan
		for _, row := range rows {
			p := plan{row: row}
			if row.Action == "update" {
				p.id = lk.ExistingNIP[row.NIP].ID
				cur, curBranch, err := currentValues(ctx, q, p.id)
				if err != nil {
					return err
				}
				set := map[string]any{}
				before := map[string]any{}
				for k, v := range row.Set {
					if !sameValue(cur[k], v) {
						set[k], before[k] = v, cur[k]
					}
				}
				if row.Branch != "" && row.Branch != curBranch {
					p.branch = row.Branch
					before["branch"], set["branch"] = curBranch, row.Branch
				}
				p.row.Set, p.before = set, before
				if len(set) == 0 {
					summary["skip"]++
					continue
				}
				summary["update"]++
				for k, v := range set {
					if len(changes) < 500 {
						changes = append(changes, employeeImportChange{Line: row.Line, NIP: row.NIP, Name: row.Name, Field: headerOf(k),
							Before: displayValue(k, before[k], l, names), After: displayValue(k, v, l, names)})
					}
				}
			} else {
				p.branch = row.Branch
				summary["create"]++
			}
			plans = append(plans, p)
			if len(list) < 200 {
				list = append(list, map[string]any{"line": row.Line, "action": row.Action, "nip": row.NIP, "name": row.Name})
			}
		}
		if in.Step != "commit" {
			out = map[string]any{"summary": summary, "errors": errs, "warnings": warns, "rows": list, "changes": changes}
			return nil
		}
		if len(errs) > 0 {
			return httpx.BadRequest("File masih punya error — perbaiki lalu unggah ulang")
		}
		for _, p := range plans {
			if p.row.ReleaseEmailFrom != "" && !in.ReleaseEmails {
				return httpx.BadRequest("Konfirmasi lepas email karyawan resign dulu")
			}
		}
		for _, p := range plans {
			if err := s.applyImportRow(ctx, r, actor, p.row, p.id, p.before, p.branch, today); err != nil {
				return err
			}
		}
		out = map[string]any{"summary": summary, "message": fmt.Sprintf("%d karyawan ditambahkan, %d diperbarui",
			summary["create"], summary["update"])}
		return nil
	})
	return out, errFile, err
}

// applyImportRow writes one validated row inside the import transaction.
func (s *Service) applyImportRow(ctx context.Context, r Repository, actor *auth.User, row domain.EmployeeImportRow,
	id string, before map[string]any, branch, today string) error {
	q := r.querier()
	set := map[string]any{}
	for k, v := range row.Set {
		if k != "branch" {
			set[k] = v
		}
	}
	var movedUser *string
	if row.ReleaseEmailFrom != "" {
		var oldNIP, oldName string
		if err := q.QueryRow(ctx, `SELECT nip, full_name, user_id::text FROM hris.employees WHERE id = $1 FOR UPDATE`,
			row.ReleaseEmailFrom).Scan(&oldNIP, &oldName, &movedUser); err != nil {
			return err
		}
		archive := archiveEmail(oldNIP)
		if _, err := q.Exec(ctx, `UPDATE hris.employees SET email = $2, user_id = NULL, is_access_app = false, updated_at = now() WHERE id = $1`,
			row.ReleaseEmailFrom, archive); err != nil {
			return err
		}
		if err := writeEmployeeAudit(ctx, q, actor, "employee.email_released", row.ReleaseEmailFrom, oldName,
			map[string]any{"email": row.Set["email"]}, map[string]any{"email": archive}); err != nil {
			return err
		}
	}
	scheduleTo, _ := set["schedule_type"].(string)
	if row.Action == "create" {
		if _, ok := set["nip"]; !ok {
			nip, err := nextEmployeeNIP(ctx, q, today[:4])
			if err != nil {
				return err
			}
			set["nip"] = nip
		}
		if _, ok := set["phone"]; !ok {
			set["phone"] = ""
		}
		cols, args, marks := []string{}, []any{}, []string{}
		for k, v := range set {
			cols = append(cols, k)
			args = append(args, v)
			marks = append(marks, fmt.Sprintf("$%d", len(args)))
		}
		if err := q.QueryRow(ctx, `INSERT INTO hris.employees (`+strings.Join(cols, ", ")+`) VALUES (`+strings.Join(marks, ", ")+`) RETURNING id::text`,
			args...).Scan(&id); err != nil {
			return httpx.BadRequest(fmt.Sprintf("Baris %d: gagal disimpan (%s)", row.Line, err.Error()))
		}
		if scheduleTo == scheduleNonShift {
			if _, err := applyFixedSchedule(ctx, r, id, today, actor.FullName); err != nil {
				return err
			}
		}
	} else {
		delete(set, "schedule_type")
		if len(set) > 0 {
			parts, args := []string{}, []any{id}
			for k, v := range set {
				args = append(args, v)
				parts = append(parts, fmt.Sprintf("%s = $%d", k, len(args)))
			}
			if _, err := q.Exec(ctx, `UPDATE hris.employees SET `+strings.Join(parts, ", ")+`, updated_at = now() WHERE id = $1`, args...); err != nil {
				return httpx.BadRequest(fmt.Sprintf("Baris %d: gagal disimpan (%s)", row.Line, err.Error()))
			}
		}
		if scheduleTo != "" {
			if _, err := changeScheduleTypeTx(ctx, r, actor, []string{id}, scheduleTo, today, false); err != nil {
				return err
			}
		}
	}
	if movedUser != nil {
		for _, sql := range []string{
			`UPDATE hris.employees SET user_id = $2, is_access_app = true WHERE id = $1`,
		} {
			if _, err := q.Exec(ctx, sql, id, *movedUser); err != nil {
				return err
			}
		}
		if _, err := q.Exec(ctx, `UPDATE auth.users SET banned_until = NULL WHERE id = $1`, *movedUser); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `UPDATE configuration.users SET status = 'active', updated_at = now() WHERE id = $1`, *movedUser); err != nil {
			return err
		}
	}
	if branch != "" {
		steps := []string{
			`DELETE FROM hris.employee_placements WHERE employee_id = $1 AND is_primary AND start_date >= $2::date`,
			`UPDATE hris.employee_placements SET end_date = $2::date - 1 WHERE employee_id = $1 AND is_primary
			   AND start_date < $2::date AND (end_date IS NULL OR end_date >= $2::date)`,
		}
		for _, sql := range steps {
			if _, err := q.Exec(ctx, sql, id, today); err != nil {
				return err
			}
		}
		if _, err := q.Exec(ctx, `INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date, notes)
			VALUES ($1, $2, true, $3::date, 'Import Data Karyawan')`, id, branch, today); err != nil {
			return httpx.BadRequest(fmt.Sprintf("Baris %d: penempatan cabang bentrok dengan penempatan lain", row.Line))
		}
	}
	after := map[string]any{}
	for k, v := range row.Set {
		after[k] = v
	}
	return writeEmployeeAudit(ctx, q, actor, "employee.import", id, row.Name, before, after)
}

// nextEmployeeNIP is the first free EMP-<year>-<seq> (same scheme as /api/users).
func nextEmployeeNIP(ctx context.Context, q database.Querier, year string) (string, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT coalesce(max(substring(nip from '^EMP-'||$1||'-(\d+)$')::int), 0) + 1
		FROM hris.employees WHERE nip LIKE 'EMP-'||$1||'-%'`, year).Scan(&n)
	return fmt.Sprintf("EMP-%s-%05d", year, n), err
}

// archiveEmail mirrors the accounts rule (Spec §1.2).
func archiveEmail(nip string) string {
	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			return r
		case r >= 'A' && r <= 'Z':
			return r + 32
		}
		return -1
	}, nip)
	return "arsip-" + clean + "@resign.invalid"
}

func (h handlers) importEmployees(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		return httpx.BadRequest("Unggah file .xlsx atau .csv data karyawan (maks 5 MB)")
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return httpx.BadRequest("Unggah file .xlsx atau .csv data karyawan (maks 5 MB)")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 5<<20+1))
	if err != nil {
		return err
	}
	if len(data) > 5<<20 {
		return httpx.BadRequest("Ukuran file maksimal 5 MB")
	}
	in := employeeImportInput{Data: data, FileName: header.Filename, Mode: r.FormValue("mode"), Step: r.FormValue("step"),
		ReleaseEmails: r.FormValue("release_emails") == "true"}
	out, errFile, err := h.svc.ImportEmployees(r.Context(), u, in)
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

package hris

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
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

package hris

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/xlsx"
)

// Pengisian massal data kerja (EPIC-086): perintah ops hris-data-kerja
// memanggil fungsi di file ini langsung ke database (tanpa sesi login).

// workDataGapSQL lists active employees with what is missing as of $1
// (today): a running primary placement, a schedule (weekly pattern in
// force or later, or a per-date roster from today), a department, a
// position.
const workDataGapSQL = `
	WITH pp AS (SELECT DISTINCT ON (employee_id) employee_id, branch_id FROM hris.employee_placements
	            WHERE is_primary AND start_date <= $1::date AND (end_date IS NULL OR end_date >= $1::date)
	            ORDER BY employee_id, start_date DESC),
	     pat AS (SELECT DISTINCT employee_id FROM hris.employee_shifts
	             WHERE effective_to IS NULL OR effective_to >= $1::date),
	     ros AS (SELECT employee_id, min(work_date) AS first_day FROM hris.employee_shift_days
	             WHERE work_date >= $1::date GROUP BY employee_id)
	SELECT e.id::text, e.nip, e.full_name, e.schedule_type, coalesce(d.name, ''), coalesce(p.title, ''),
	       coalesce(b.code, ''), pp.employee_id IS NULL, pat.employee_id IS NULL AND ros.employee_id IS NULL,
	       e.department_id IS NULL, e.job_title_id IS NULL
	FROM hris.employees e
	LEFT JOIN hris.departments d ON d.id = e.department_id
	LEFT JOIN hris.positions p ON p.id = e.job_title_id
	LEFT JOIN pp ON pp.employee_id = e.id
	LEFT JOIN configuration.branches b ON b.id = pp.branch_id
	LEFT JOIN pat ON pat.employee_id = e.id
	LEFT JOIN ros ON ros.employee_id = e.id
	WHERE e.is_active
	ORDER BY coalesce(b.code, 'zzz'), e.full_name`

type workDataGapRow struct {
	ID, NIP, Name, ScheduleType, Dept, Position, Branch string
	NoPrimary, NoSchedule, NoDept, NoPosition           bool
}

func (r workDataGapRow) missing() string {
	var parts []string
	for _, p := range []struct {
		on   bool
		text string
	}{{r.NoSchedule, "jadwal kerja"}, {r.NoPrimary, "penempatan"}, {r.NoDept, "departemen"}, {r.NoPosition, "jabatan"}} {
		if p.on {
			parts = append(parts, p.text)
		}
	}
	return strings.Join(parts, ", ")
}

func workDataGaps(ctx context.Context, q database.Querier, today string) ([]workDataGapRow, error) {
	rows, err := q.Query(ctx, workDataGapSQL, today)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (workDataGapRow, error) {
		var g workDataGapRow
		err := r.Scan(&g.ID, &g.NIP, &g.Name, &g.ScheduleType, &g.Dept, &g.Position, &g.Branch,
			&g.NoPrimary, &g.NoSchedule, &g.NoDept, &g.NoPosition)
		return g, err
	})
}

// WorkDataGap counts the active employees of one schedule type and what
// they are missing (audit mode).
type WorkDataGap struct {
	ScheduleType                                          string
	Total, NoPrimary, NoSchedule, NoDept, NoPosition, Any int
}

// WorkDataAudit is the read-only audit of the gaps (R5).
func WorkDataAudit(ctx context.Context, q database.Querier, today string) ([]WorkDataGap, error) {
	rows, err := workDataGaps(ctx, q, today)
	if err != nil {
		return nil, err
	}
	byType := map[string]*WorkDataGap{}
	var order []string
	for _, r := range rows {
		g := byType[r.ScheduleType]
		if g == nil {
			g = &WorkDataGap{ScheduleType: r.ScheduleType}
			byType[r.ScheduleType] = g
			order = append(order, r.ScheduleType)
		}
		g.Total++
		count := func(on bool, n *int) {
			if on {
				*n++
			}
		}
		count(r.NoPrimary, &g.NoPrimary)
		count(r.NoSchedule, &g.NoSchedule)
		count(r.NoDept, &g.NoDept)
		count(r.NoPosition, &g.NoPosition)
		count(r.missing() != "", &g.Any)
	}
	out := make([]WorkDataGap, 0, len(order))
	for _, t := range order {
		out = append(out, *byType[t])
	}
	return out, nil
}

// loadWorkDataMasters reads the live masters a file is checked against.
// A position title used twice maps to "" (ambiguous).
func loadWorkDataMasters(ctx context.Context, q database.Querier, today string) (domain.WorkDataMasters, error) {
	m := domain.WorkDataMasters{Employees: map[string]domain.WorkDataEmployee{}, Branches: map[string]string{},
		Shifts: map[string]domain.UploadShift{}, Departments: map[string]string{}, Positions: map[string]string{}, Today: today}
	type pair struct{ k, v string }
	collect := func(sql string, args ...any) ([]pair, error) {
		rows, err := q.Query(ctx, sql, args...)
		if err != nil {
			return nil, err
		}
		return pgx.CollectRows(rows, func(r pgx.CollectableRow) (pair, error) {
			var p pair
			return p, r.Scan(&p.k, &p.v)
		})
	}
	rows, err := q.Query(ctx, `SELECT e.id::text, e.nip, e.full_name, e.schedule_type, e.is_active,
		       coalesce((SELECT min(work_date)::text FROM hris.employee_shift_days s
		                 WHERE s.employee_id = e.id AND s.work_date >= $1::date), '')
		FROM hris.employees e WHERE e.nip IS NOT NULL AND btrim(e.nip) <> ''`, today)
	if err != nil {
		return m, err
	}
	emps, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.WorkDataEmployee, error) {
		var e domain.WorkDataEmployee
		return e, r.Scan(&e.ID, &e.NIP, &e.Name, &e.ScheduleType, &e.Active, &e.RosterFrom)
	})
	if err != nil {
		return m, err
	}
	for _, e := range emps {
		m.Employees[strings.TrimSpace(e.NIP)] = e
	}
	branches, err := collect(`SELECT upper(btrim(code)), id::text FROM configuration.branches
		WHERE is_active AND code IS NOT NULL AND btrim(code) <> ''`)
	if err != nil {
		return m, err
	}
	for _, b := range branches {
		m.Branches[b.k] = b.v
	}
	shifts, err := loadShifts(ctx, q, true)
	if err != nil {
		return m, err
	}
	for _, s := range shifts {
		m.Shifts[strings.ToUpper(s.Code)] = domain.UploadShift{ID: s.ID, Code: strings.ToUpper(s.Code), Name: s.Name,
			Start: s.StartTime, End: s.EndTime, Overnight: s.Overnight, Active: true, BreakMinutes: s.Break}
	}
	depts, err := collect(`SELECT lower(btrim(name)), id::text FROM hris.departments WHERE coalesce(is_active, true)`)
	if err != nil {
		return m, err
	}
	for _, d := range depts {
		m.Departments[d.k] = d.v
	}
	positions, err := collect(`SELECT lower(btrim(title)), id::text FROM hris.positions WHERE coalesce(is_active, true)`)
	if err != nil {
		return m, err
	}
	for _, p := range positions {
		if _, dup := m.Positions[p.k]; dup {
			m.Positions[p.k] = ""
			continue
		}
		m.Positions[p.k] = p.v
	}
	closed, err := collect(`SELECT start_date::text, end_date::text FROM hris.attendance_periods
		WHERE status = 'closed' AND end_date >= $1::date`, today)
	if err != nil {
		return m, err
	}
	m.ClosedFrom = func(date string) bool {
		for _, c := range closed {
			if date >= c.k && date <= c.v {
				return true
			}
		}
		return false
	}
	return m, nil
}

// WorkDataTemplate is the xlsx HR fills (R2): the active employees with a
// gap, dropdowns from the live masters.
func WorkDataTemplate(ctx context.Context, q database.Querier, today string) ([]byte, error) {
	gaps, err := workDataGaps(ctx, q, today)
	if err != nil {
		return nil, err
	}
	shifts, err := loadShifts(ctx, q, true)
	if err != nil {
		return nil, err
	}
	type named struct{ a, b string }
	list := func(sql string) ([]named, error) {
		rows, err := q.Query(ctx, sql)
		if err != nil {
			return nil, err
		}
		return pgx.CollectRows(rows, func(r pgx.CollectableRow) (named, error) {
			var n named
			return n, r.Scan(&n.a, &n.b)
		})
	}
	stores, err := list(`SELECT upper(btrim(code)), name FROM configuration.branches
		WHERE is_active AND code IS NOT NULL AND btrim(code) <> '' ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	depts, err := list(`SELECT name, coalesce(code, '') FROM hris.departments WHERE coalesce(is_active, true) ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	positions, err := list(`SELECT DISTINCT ON (lower(btrim(title))) btrim(title), coalesce(level, '')
		FROM hris.positions WHERE coalesce(is_active, true) ORDER BY lower(btrim(title)), title`)
	if err != nil {
		return nil, err
	}
	codes := make([]string, 0, len(shifts)+1)
	for _, s := range shifts {
		codes = append(codes, strings.ToUpper(s.Code))
	}
	codes = append(codes, domain.WorkDataDayOff)
	storeCodes := make([]string, len(stores))
	for i, s := range stores {
		storeCodes[i] = s.a
	}
	pick := func(ns []named) []string {
		out := make([]string, len(ns))
		for i, n := range ns {
			out[i] = n.a
		}
		return out
	}
	locked := func(h string, w float64) xlsx.TemplateColumn {
		return xlsx.TemplateColumn{Header: h, ReadOnly: true, Width: w}
	}
	cols := []xlsx.TemplateColumn{locked("NIP", 18), locked("Nama", 28), locked("Departemen Saat Ini", 22),
		locked("Jabatan Saat Ini", 24), locked("Penempatan Saat Ini", 14), locked("Tipe Saat Ini", 12),
		locked("Yang Perlu Dilengkapi", 26),
		{Header: "Penempatan Utama (Kode Toko)", Kind: xlsx.KindList, Options: storeCodes, Width: 16},
		{Header: "Tipe Jadwal", Kind: xlsx.KindList, Options: []string{"Shift", "Non-Shift"}, Width: 12}}
	for _, d := range domain.WorkDataHeader[9:16] {
		cols = append(cols, xlsx.TemplateColumn{Header: d, Kind: xlsx.KindList, Options: codes, Width: 11,
			OtherNote: "Dua shift sehari: tulis KODE1+KODE2, contoh PAGI8+SIANG14."})
	}
	cols = append(cols,
		xlsx.TemplateColumn{Header: "Berlaku Mulai", Width: 14},
		xlsx.TemplateColumn{Header: "Departemen Baru", Kind: xlsx.KindList, Options: pick(depts), Width: 22},
		xlsx.TemplateColumn{Header: "Jabatan Baru", Kind: xlsx.KindList, Options: pick(positions), Width: 26},
		xlsx.TemplateColumn{Header: "Catatan", Width: 30})
	typeLabel := map[string]string{"shift": "Shift", "non_shift": "Non-Shift"}
	var rows [][]any
	for _, g := range gaps {
		if g.missing() == "" {
			continue
		}
		row := []any{g.NIP, g.Name, g.Dept, g.Position, g.Branch, typeLabel[g.ScheduleType], g.missing()}
		for len(row) < len(cols) {
			row = append(row, nil)
		}
		rows = append(rows, row)
	}
	data := xlsx.TemplateSheet{Name: "Data", Columns: cols, Rows: rows, FreezeCols: 2, DataRows: len(rows) + 50, Protect: true}
	ref := func(name string, headers []string, body [][]any) xlsx.TemplateSheet {
		cs := make([]xlsx.TemplateColumn, len(headers))
		for i, h := range headers {
			cs[i] = xlsx.TemplateColumn{Header: h, ReadOnly: true, Width: 30}
		}
		return xlsx.TemplateSheet{Name: name, Columns: cs, Rows: body, Protect: true}
	}
	var storeRows, shiftRows, deptRows, posRows [][]any
	for _, s := range stores {
		storeRows = append(storeRows, []any{s.a, s.b})
	}
	for _, s := range shifts {
		shiftRows = append(shiftRows, []any{strings.ToUpper(s.Code), s.Name, clockHM(s.StartTime), clockHM(s.EndTime)})
	}
	shiftRows = append(shiftRows, []any{domain.WorkDataDayOff, "Libur", "", ""})
	for _, d := range depts {
		deptRows = append(deptRows, []any{d.a})
	}
	for _, p := range positions {
		posRows = append(posRows, []any{p.a, p.b})
	}
	notes := []xlsx.SheetSpec{{Name: "Petunjuk", ColumnWidths: []float64{120}, Rows: workDataGuide}}
	info := &xlsx.InfoSheet{Name: scheduleInfoSheet, Pairs: [][2]string{{"Template", workDataTemplateVersion}, {"Dibuat", today}}}
	return xlsx.BuildTemplate([]xlsx.TemplateSheet{data,
		ref("Kode Toko", []string{"Kode", "Nama Toko/Cabang"}, storeRows),
		ref("Kode Shift", []string{"Kode", "Nama Shift", "Jam Masuk", "Jam Pulang"}, shiftRows),
		ref("Departemen", []string{"Departemen"}, deptRows),
		ref("Jabatan", []string{"Jabatan", "Level"}, posRows)}, notes, info)
}

const workDataTemplateVersion = "data-kerja-v1"

func clockHM(hms string) string {
	if len(hms) >= 5 {
		return hms[:5]
	}
	return hms
}

var workDataGuide = [][]any{
	{"Template Massal Data Kerja — Jadwal, Shift & Penempatan (Paddy HRIS)"},
	{"Berisi karyawan aktif yang datanya belum lengkap. Satu file untuk semua cabang."},
	{""},
	{"Cara mengisi sheet \"Data\":"},
	{"1. Kolom abu-abu (NIP s.d. Yang Perlu Dilengkapi) adalah data sekarang — jangan diubah. Sel yang dikosongkan = tidak berubah."},
	{"2. Penempatan Utama: pilih kode toko/cabang (sheet \"Kode Toko\"). Penempatan utama lama ditutup sehari sebelum Berlaku Mulai."},
	{"3. Tipe Jadwal: Shift (toko, host live, CRM, produksi) atau Non-Shift (HQ, Operations). Kosongkan bila tidak berubah."},
	{"4. Senin s.d. Minggu (khusus karyawan Shift): kode shift dari sheet \"Kode Shift\" atau LIBUR; isi ketujuh hari."},
	{"   Dua shift sehari: KODE1+KODE2. Karyawan Non-Shift: kosongkan — jadwalnya mengikuti departemen (HQ = NS, Operations = OPS/OPSSAT)."},
	{"5. Berlaku Mulai: tanggal mulai (YYYY-MM-DD, misalnya 2026-11-10), wajib bila mengubah penempatan, tipe, atau pola. Tidak boleh tanggal lampau."},
	{"6. Departemen Baru / Jabatan Baru: isi bila ingin mengubah; pilih dari daftar."},
	{"7. Karyawan lain boleh ditambahkan di baris kosong (cukup NIP + kolom yang diubah)."},
	{""},
	{"Setelah selesai, kirim file kembali. Tim menjalankan pratinjau (daftar baris salah dan peringatan) sebelum menyimpan ke HRIS."},
}

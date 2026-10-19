package domain

import (
	"fmt"
	"strings"
)

// Columns of the Data Karyawan file (Spec 2026-10-19 §3.2): export and
// import share them so an exported file can be edited and imported back.

type EmployeeColumn struct {
	Key        string
	Header     string
	Required   bool
	Kind       string
	Width      float64
	ExportOnly bool
}

var EmployeeColumns = []EmployeeColumn{
	{Key: "nip", Header: "NIP", Kind: "text", Width: 16},
	{Key: "full_name", Header: "Nama Lengkap", Required: true, Kind: "text", Width: 28},
	{Key: "email", Header: "Email", Required: true, Kind: "text", Width: 28},
	{Key: "phone", Header: "No. HP", Kind: "text", Width: 16},
	{Key: "join_date", Header: "Tanggal Masuk", Required: true, Kind: "date", Width: 14},
	{Key: "employment_status", Header: "Status Kepegawaian", Required: true, Kind: "list", Width: 20},
	{Key: "schedule_type", Header: "Tipe Jadwal", Required: true, Kind: "list", Width: 12},
	{Key: "branch", Header: "Cabang Utama", Required: true, Kind: "list", Width: 22},
	{Key: "department", Header: "Departemen", Kind: "list", Width: 22},
	{Key: "position", Header: "Jabatan", Kind: "list", Width: 24},
	{Key: "manager_nip", Header: "NIP Atasan", Kind: "text", Width: 16},
	{Key: "ktp", Header: "NIK (KTP)", Kind: "text", Width: 20},
	{Key: "npwp", Header: "NPWP", Kind: "text", Width: 20},
	{Key: "birth_date", Header: "Tanggal Lahir", Kind: "date", Width: 14},
	{Key: "gender", Header: "Jenis Kelamin", Kind: "list", Width: 14},
	{Key: "marital_status", Header: "Status Pernikahan", Kind: "list", Width: 18},
	{Key: "address", Header: "Alamat", Kind: "text", Width: 36},
	{Key: "city", Header: "Kota", Kind: "text", Width: 16},
	{Key: "province", Header: "Provinsi", Kind: "text", Width: 16},
	{Key: "postal_code", Header: "Kode Pos", Kind: "text", Width: 10},
	{Key: "bank_name", Header: "Nama Bank", Kind: "text", Width: 14},
	{Key: "bank_account", Header: "No. Rekening", Kind: "text", Width: 18},
	{Key: "bpjs_tk", Header: "No. BPJS Ketenagakerjaan", Kind: "text", Width: 22},
	{Key: "bpjs_kesehatan", Header: "No. BPJS Kesehatan", Kind: "text", Width: 20},
	{Key: "emergency_contact_name", Header: "Kontak Darurat (Nama)", Kind: "text", Width: 22},
	{Key: "emergency_contact_phone", Header: "Kontak Darurat (No. HP)", Kind: "text", Width: 18},
	{Key: "emergency_contact_relationship", Header: "Kontak Darurat (Hubungan)", Kind: "text", Width: 18},
	{Key: "manager_name", Header: "Nama Atasan", Kind: "text", Width: 24, ExportOnly: true},
	{Key: "active", Header: "Status Aktif", Kind: "text", Width: 12, ExportOnly: true},
	{Key: "end_date", Header: "Tanggal Keluar", Kind: "date", Width: 14, ExportOnly: true},
	{Key: "updated_at", Header: "Terakhir Diubah", Kind: "date", Width: 14, ExportOnly: true},
}

func EmployeeHeaderKey(header string) (string, bool) {
	h := NormalizeHeader(header)
	for _, c := range EmployeeColumns {
		if NormalizeHeader(c.Header) == h {
			return c.Key, true
		}
	}
	return "", false
}

/* ── Import validation (Spec §1.5, §3.2) ─────────────────────────────── */

const MaxEmployeeImportRows = 1000

// EmployeeRef is an existing employee the import refers to.
type EmployeeRef struct {
	ID, NIP, Name string
	Active        bool
}

// EmployeeLookups are the masters and existing rows an import is checked
// against; map keys are lower-case names/emails.
type EmployeeLookups struct {
	Statuses    map[string]string
	Branches    map[string]string
	Departments map[string]string
	Positions   map[string]string
	ExistingNIP map[string]EmployeeRef
	EmailOwner  map[string]EmployeeRef
	ManagerNIP  map[string]string
	Today       string // "" skips the birth-date-in-the-future check
	// Current is each existing employee's values as the export writes them
	// (NIP → column key → text; dates ISO). In Replace mode a cell equal
	// to it is unchanged: not validated again, not overwritten.
	Current map[string]map[string]string
}

// sameAsCurrent: an export cell put back unchanged (dates compared as dates,
// labels case-insensitively).
func sameAsCurrent(key, cell, current string) bool {
	if key == "join_date" || key == "birth_date" {
		d, ok := ParseDateCell(cell)
		return ok && d == current
	}
	return strings.EqualFold(strings.TrimSpace(cell), strings.TrimSpace(current))
}

// EmployeeImportRow is one file row: Set holds only the filled cells as
// hris.employees columns; Branch is the primary branch id ("" = unchanged).
type EmployeeImportRow struct {
	Line             int
	Action           string // create | update
	NIP              string
	Name             string
	Set              map[string]any
	Branch           string
	ReleaseEmailFrom string
}

var columnHeader = func() map[string]string {
	m := map[string]string{}
	for _, c := range EmployeeColumns {
		m[c.Key] = c.Header
	}
	return m
}()

var plainText = map[string]bool{"address": true, "city": true, "province": true, "bank_name": true,
	"emergency_contact_name": true, "emergency_contact_relationship": true}

var digitsOnly = map[string]bool{"bank_account": true, "bpjs_tk": true, "bpjs_kesehatan": true, "postal_code": true}

func isDigits(s string) bool { return s != "" && strings.Trim(s, "0123456789") == "" }

func validEmail(s string) bool {
	at := strings.LastIndex(s, "@")
	return at > 0 && strings.Contains(s[at+1:], ".") && !strings.ContainsAny(s, " ,;")
}

// ValidateEmployeeImport checks every row of the Data Karyawan sheet
// (header = file row firstLine-1). It returns one entry per non-blank row,
// the errors (commit is refused while any) and the warnings.
func ValidateEmployeeImport(mode string, header []string, rows [][]string, firstLine int, lk EmployeeLookups) ([]EmployeeImportRow, []UploadIssue, []UploadIssue) {
	var out []EmployeeImportRow
	errs, warns := []UploadIssue{}, []UploadIssue{}
	keys := make([]string, len(header))
	hasName := false
	for i, h := range header {
		if k, ok := EmployeeHeaderKey(h); ok {
			for _, c := range EmployeeColumns {
				if c.Key == k && !c.ExportOnly {
					keys[i] = k
				}
			}
			hasName = hasName || k == "full_name"
		}
	}
	if !hasName {
		errs = append(errs, UploadIssue{Message: `Judul kolom "Nama Lengkap" tidak ditemukan — pakai template dari halaman Karyawan`})
		return out, errs, warns
	}
	if len(rows) > MaxEmployeeImportRows {
		errs = append(errs, UploadIssue{Message: "Maksimal 1.000 baris per file"})
		return out, errs, warns
	}
	seen := map[string]int{}
	for i, cells := range rows {
		line := firstLine + i
		v := map[string]string{}
		blank := true
		for j, k := range keys {
			if k == "" || j >= len(cells) {
				continue
			}
			val := strings.TrimSpace(cells[j])
			v[k] = val
			blank = blank && val == ""
		}
		if blank {
			continue
		}
		fail := func(key, msg string) {
			errs = append(errs, UploadIssue{Line: line, Column: columnHeader[key], Message: msg})
		}
		row := EmployeeImportRow{Line: line, Set: map[string]any{}}
		nip, msg := CleanNIP(v["nip"])
		if msg != "" {
			fail("nip", msg)
		}
		row.NIP = nip
		existing, exists := lk.ExistingNIP[nip]
		switch {
		case nip != "" && seen[nip] > 0:
			fail("nip", fmt.Sprintf("NIP %s ganda (baris %d & %d)", nip, seen[nip], line))
		case mode == "add" && nip != "" && exists:
			errs = append(errs, UploadIssue{Line: line, Message: "NIP " + nip + " sudah ada (mode Tambah hanya untuk karyawan baru)"})
		case mode == "replace" && nip == "" && msg == "":
			fail("nip", "wajib diisi di mode Timpa")
		}
		if nip != "" {
			seen[nip] = line
		}
		row.Action = "create"
		if mode == "replace" && exists {
			row.Action = "update"
			row.Name = existing.Name
		}
		if row.Action == "create" {
			if nip != "" {
				row.Set["nip"] = nip
			}
			for _, k := range []string{"full_name", "email", "join_date", "employment_status", "schedule_type", "branch"} {
				if v[k] == "" {
					fail(k, "wajib diisi untuk karyawan baru")
				}
			}
		}
		current := lk.Current[nip]
		for _, c := range EmployeeColumns {
			k, val := c.Key, v[c.Key]
			if val == "" || c.ExportOnly || k == "nip" {
				continue
			}
			if row.Action == "update" && current != nil && sameAsCurrent(k, val, current[k]) {
				if k == "full_name" {
					row.Name = val
				}
				continue
			}
			switch {
			case k == "full_name":
				row.Set[k] = val
				row.Name = val
			case k == "email":
				if !validEmail(val) {
					fail(k, "format email tidak valid")
					continue
				}
				row.Set[k] = val
				if o, ok := lk.EmailOwner[strings.ToLower(val)]; ok && o.NIP != nip {
					if o.Active {
						fail(k, fmt.Sprintf("sudah dipakai %s (NIP %s)", o.Name, o.NIP))
					} else {
						warns = append(warns, UploadIssue{Line: line, Column: c.Header,
							Message: fmt.Sprintf("%s milik %s (NIP %s, resign) — akan dilepas bila dikonfirmasi", val, o.Name, o.NIP)})
						row.ReleaseEmailFrom = o.ID
					}
				}
			case k == "phone" || k == "emergency_contact_phone":
				p, m := NormalizePhone(val)
				if m != "" {
					fail(k, m)
					continue
				}
				row.Set[k] = p
			case k == "join_date" || k == "birth_date":
				d, ok := ParseDateCell(val)
				if !ok {
					fail(k, "format tanggal harus DD/MM/YYYY")
					continue
				}
				if k == "birth_date" && lk.Today != "" && d > lk.Today {
					fail(k, "tidak boleh di masa depan")
					continue
				}
				row.Set[k] = d
			case k == "employment_status":
				code, ok := lk.Statuses[strings.ToLower(val)]
				if !ok {
					fail(k, fmt.Sprintf("'%s' tidak ada di daftar pilihan", val))
					continue
				}
				row.Set[k] = code
			case k == "schedule_type" || k == "gender" || k == "marital_status":
				choices := map[string][]Choice{"schedule_type": ScheduleTypeLabels, "gender": GenderLabels, "marital_status": MaritalLabels}[k]
				code, ok := ValueOf(choices, val)
				if !ok {
					fail(k, fmt.Sprintf("'%s' tidak ada di daftar pilihan", val))
					continue
				}
				row.Set[k] = code
			case k == "branch" || k == "department" || k == "position":
				src := map[string]map[string]string{"branch": lk.Branches, "department": lk.Departments, "position": lk.Positions}[k]
				id, ok := src[strings.ToLower(val)]
				if !ok {
					fail(k, fmt.Sprintf("'%s' tidak ada di daftar pilihan", val))
					continue
				}
				switch k {
				case "branch":
					row.Branch = id
				case "department":
					row.Set["department_id"] = id
				default:
					row.Set["job_title_id"] = id
				}
			case k == "manager_nip":
				id, ok := lk.ManagerNIP[val]
				if !ok {
					fail(k, "NIP atasan "+val+" tidak ditemukan / tidak aktif")
					continue
				}
				row.Set["reporting_to"] = id
			case k == "ktp":
				d := CleanDigits(val)
				if !isDigits(d) || len(d) != 16 {
					fail(k, "harus 16 angka")
					continue
				}
				row.Set[k] = d
			case k == "npwp":
				d := CleanDigits(val)
				if !isDigits(d) || len(d) < 15 || len(d) > 16 {
					fail(k, "harus 15–16 angka")
					continue
				}
				row.Set[k] = d
			case digitsOnly[k]:
				d := CleanDigits(val)
				if !isDigits(d) {
					fail(k, "hanya boleh angka")
					continue
				}
				row.Set[k] = d
			case plainText[k]:
				row.Set[k] = val
			}
		}
		out = append(out, row)
	}
	return out, errs, warns
}

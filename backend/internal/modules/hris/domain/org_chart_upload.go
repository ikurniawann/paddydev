package domain

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

// Impor versi bagan dari template Excel (EPIC-103 R2–R4): sheet "Bagan",
// satu baris per node. Validasi baris di sini; aturan struktur (BR) tetap
// ValidateOrgChart, dipetakan ke baris node-nya.

const (
	OrgChartSheet   = "Bagan"
	MaxOrgChartRows = 1000
)

var OrgChartHeaders = []string{"Kode", "Kode Atasan", "Jabatan", "Posisi", "Kode Divisi", "Divisi", "Level",
	"Jenis Kerja", "Vendor", "Region", "Toko", "Status", "Perlu Konfirmasi"}

// OrgUploadNode is one valid row of the Bagan sheet.
type OrgUploadNode struct {
	OrgNode
	DivisionName string
	NeedsConfirm bool
	Line         int // 1-based sheet row
}

// OrgUploadResult is the outcome of a Bagan sheet.
type OrgUploadResult struct {
	Nodes            []OrgUploadNode
	Errors, Warnings []UploadIssue
	HeaderLine       int
}

var (
	orgCodePattern     = regexp.MustCompile(`^[A-Z0-9][A-Z0-9-]{1,29}$`)
	orgDivisionPattern = regexp.MustCompile(`^[A-Z]{2,6}$`)
	orgStorePattern    = regexp.MustCompile(`^[A-Z0-9]{2,10}$`)
	orgLevelPattern    = regexp.MustCompile(`^L[0-8]$`)
)

var employmentTypes = map[string]string{
	"tetap/kontrak": "PERMANENT_OR_CONTRACT", "tetap": "PERMANENT_OR_CONTRACT", "kontrak": "PERMANENT_OR_CONTRACT",
	"permanent_or_contract": "PERMANENT_OR_CONTRACT", "magang": "INTERN", "intern": "INTERN",
	"freelance": "FREELANCE", "outsource": "OUTSOURCE",
}

var nodeStatuses = map[string]string{
	"": "active", "aktif": "active", "active": "active", "rencana": "planned", "planned": "planned",
	"kosong": "vacant", "vacant": "vacant", "dibekukan": "frozen", "frozen": "frozen",
}

// EmploymentTypeLabel is the template wording of an employment type code.
func EmploymentTypeLabel(code string) string {
	switch code {
	case "PERMANENT_OR_CONTRACT":
		return "Tetap/Kontrak"
	case "INTERN":
		return "Magang"
	case "FREELANCE":
		return "Freelance"
	case "OUTSOURCE":
		return "Outsource"
	}
	return code
}

// NodeStatusLabel is the template wording of a node status.
func NodeStatusLabel(status string) string {
	switch status {
	case "active":
		return "Aktif"
	case "planned":
		return "Rencana"
	case "vacant":
		return "Kosong"
	case "frozen":
		return "Dibekukan"
	}
	return status
}

// yesNo reads Ya/Tidak; ok is false for anything else (blank included).
func yesNo(s string) (yes, ok bool) {
	switch strings.ToLower(s) {
	case "ya", "y":
		return true, true
	case "tidak", "t", "n":
		return false, true
	}
	return false, false
}

// brColumn is the template column of the field a BR rule checks.
var brColumn = map[string]string{"BR-01": "Kode Atasan", "BR-02": "Kode Atasan", "BR-03": "Kode Atasan",
	"BR-04": "Level", "BR-05": "Toko", "BR-07": "Jabatan", "BR-09": "Level", "BR-10": "Vendor"}

// ParseOrgChartUpload reads a Bagan sheet. problem is non-empty when the
// sheet is not the template, has no rows, or has too many rows; activeStores
// are the short codes of active HRIS stores (unknown codes only warn).
func ParseOrgChartUpload(matrix [][]string, activeStores []string) (res OrgUploadResult, problem string) {
	res = OrgUploadResult{Nodes: []OrgUploadNode{}, Errors: []UploadIssue{}, Warnings: []UploadIssue{}}
	header := -1
	for i, r := range matrix {
		if len(r) > 0 && NormalizeHeader(r[0]) == "kode" {
			header = i
			break
		}
	}
	if header < 0 {
		return res, `Judul kolom "Kode" tidak ditemukan — pakai template dari halaman Struktur Org`
	}
	res.HeaderLine = header + 1
	col := map[string]int{}
	for j, h := range matrix[header] {
		col[NormalizeHeader(h)] = j
	}
	var missing []string
	for _, h := range OrgChartHeaders {
		if _, ok := col[NormalizeHeader(h)]; !ok {
			missing = append(missing, h)
		}
	}
	if len(missing) > 0 {
		return res, "Kolom tidak ada di file: " + strings.Join(missing, ", ") + " — pakai template dari halaman Struktur Org"
	}
	cell := func(r []string, name string) string {
		j, ok := col[NormalizeHeader(name)]
		if !ok || j >= len(r) {
			return ""
		}
		return strings.TrimSpace(r[j])
	}
	var rows []int
	for i := header + 1; i < len(matrix); i++ {
		if strings.TrimSpace(strings.Join(matrix[i], "")) != "" {
			rows = append(rows, i)
		}
	}
	if len(rows) == 0 {
		return res, "File tidak berisi baris bagan"
	}
	if len(rows) > MaxOrgChartRows {
		return res, fmt.Sprintf("Maksimal %d baris per file", MaxOrgChartRows)
	}
	active := map[string]bool{}
	for _, s := range activeStores {
		active[strings.ToUpper(s)] = true
	}
	seen := map[string]int{}
	lineOf := map[string]int{}
	// Codes of rows with their own error, and whether one of them is a top:
	// BR issues they would cause are not reported again.
	badCodes, badRoot := map[string]bool{}, false
	for _, i := range rows {
		r, line := matrix[i], i+1
		bad := false
		fail := func(column, msg string) {
			res.Errors = append(res.Errors, UploadIssue{Line: line, Column: column, Name: cell(r, "Kode"), Message: msg})
			bad = true
		}
		tooLong := func(column, v string, max int) {
			if utf8.RuneCountInString(v) > max {
				fail(column, fmt.Sprintf("%s maksimal %d karakter", column, max))
			}
		}
		n := OrgUploadNode{Line: line}
		n.Code = strings.ToUpper(cell(r, "Kode"))
		switch {
		case n.Code == "":
			fail("Kode", "Kode wajib diisi")
		case !orgCodePattern.MatchString(n.Code):
			fail("Kode", "Format kode tidak valid")
		case seen[n.Code] > 0:
			fail("Kode", fmt.Sprintf("Kode %s dipakai lebih dari sekali (baris %d)", n.Code, seen[n.Code]))
		default:
			seen[n.Code] = line
		}
		n.ParentCode = strings.ToUpper(cell(r, "Kode Atasan"))
		n.Title = cell(r, "Jabatan")
		if n.Title == "" {
			fail("Jabatan", "Jabatan wajib diisi")
		}
		tooLong("Jabatan", n.Title, 150)
		if pos, ok := yesNo(cell(r, "Posisi")); ok {
			n.IsPosition = pos
		} else {
			fail("Posisi", "Isi Posisi dengan Ya atau Tidak")
		}
		n.DivisionCode = strings.ToUpper(cell(r, "Kode Divisi"))
		switch {
		case n.DivisionCode == "":
			fail("Kode Divisi", "Kode divisi wajib diisi")
		case !orgDivisionPattern.MatchString(n.DivisionCode):
			fail("Kode Divisi", "Format kode divisi tidak valid")
		}
		n.DivisionName = cell(r, "Divisi")
		if n.DivisionName == "" {
			fail("Divisi", "Divisi wajib diisi")
		}
		tooLong("Divisi", n.DivisionName, 100)
		n.Level = strings.ToUpper(cell(r, "Level"))
		if n.Level != "" && !orgLevelPattern.MatchString(n.Level) {
			fail("Level", "Level harus L0 sampai L8")
		}
		if et := cell(r, "Jenis Kerja"); et != "" {
			if code, ok := employmentTypes[strings.ToLower(et)]; ok {
				n.EmploymentType = code
			} else {
				fail("Jenis Kerja", "Jenis kerja tidak dikenal")
			}
		}
		n.Vendor = cell(r, "Vendor")
		tooLong("Vendor", n.Vendor, 100)
		n.Region = cell(r, "Region")
		tooLong("Region", n.Region, 60)
		for _, s := range strings.FieldsFunc(cell(r, "Toko"), func(c rune) bool { return c == ',' || c == ';' }) {
			s = strings.ToUpper(strings.TrimSpace(s))
			switch {
			case s == "":
			case !orgStorePattern.MatchString(s):
				fail("Toko", fmt.Sprintf("Kode toko %s tidak valid", s))
			case !slices.Contains(n.StoreCodes, s):
				n.StoreCodes = append(n.StoreCodes, s)
			}
		}
		if st, ok := nodeStatuses[strings.ToLower(cell(r, "Status"))]; ok {
			n.Status = st
		} else {
			fail("Status", "Status harus Aktif, Rencana, Kosong, atau Dibekukan")
		}
		if c := cell(r, "Perlu Konfirmasi"); c != "" {
			if yes, ok := yesNo(c); ok {
				n.NeedsConfirm = yes
			} else {
				fail("Perlu Konfirmasi", "Isi Perlu Konfirmasi dengan Ya atau Tidak")
			}
		}
		if bad {
			if n.Code != "" {
				badCodes[n.Code] = true
			}
			badRoot = badRoot || n.ParentCode == ""
			continue
		}
		for _, s := range n.StoreCodes {
			if !active[s] {
				res.Warnings = append(res.Warnings, UploadIssue{Line: line, Column: "Toko", Name: n.Code,
					Message: fmt.Sprintf("Toko %s tidak ada sebagai toko aktif di HRIS", s)})
			}
		}
		lineOf[n.Code] = line
		res.Nodes = append(res.Nodes, n)
	}
	nodes := make([]OrgNode, len(res.Nodes))
	for i, n := range res.Nodes {
		nodes[i] = n.OrgNode
	}
	errs, warns := ValidateOrgChart(nodes)
	parentOf := map[string]string{}
	for _, n := range nodes {
		parentOf[n.Code] = n.ParentCode
	}
	errs = slices.DeleteFunc(errs, func(i OrgIssue) bool {
		return (i.Rule == "BR-03" && badCodes[parentOf[i.Code]]) || (i.Rule == "BR-01" && badRoot)
	})
	toIssue := func(i OrgIssue) UploadIssue {
		return UploadIssue{Line: lineOf[i.Code], Column: brColumn[i.Rule], Name: i.Code, Message: i.Message}
	}
	for _, i := range errs {
		res.Errors = append(res.Errors, toIssue(i))
	}
	for _, i := range warns {
		res.Warnings = append(res.Warnings, toIssue(i))
	}
	return res, ""
}

// OrgNodeChange is a code present in both versions with different fields.
type OrgNodeChange struct {
	Code   string   `json:"code"`
	Title  string   `json:"title"`
	Fields []string `json:"fields"`
}

// OrgChartDiff compares a new version with its base (EPIC-103 R6).
type OrgChartDiff struct {
	Added   []string        `json:"added"`
	Removed []string        `json:"removed"`
	Changed []OrgNodeChange `json:"changed"`
}

// DiffOrgCharts lists added, removed and changed codes, each sorted.
func DiffOrgCharts(base, next []OrgNode) OrgChartDiff {
	wrap := func(ns []OrgNode) []OrgUploadNode {
		out := make([]OrgUploadNode, len(ns))
		for i, n := range ns {
			out[i] = OrgUploadNode{OrgNode: n}
		}
		return out
	}
	return DiffOrgUpload(wrap(base), wrap(next))
}

// DiffOrgUpload is DiffOrgCharts that also compares the division name and
// Perlu Konfirmasi, which only upload nodes carry.
func DiffOrgUpload(base, next []OrgUploadNode) OrgChartDiff {
	d := OrgChartDiff{Added: []string{}, Removed: []string{}, Changed: []OrgNodeChange{}}
	old := map[string]OrgUploadNode{}
	for _, n := range base {
		old[n.Code] = n
	}
	now := map[string]bool{}
	for _, n := range next {
		now[n.Code] = true
		o, ok := old[n.Code]
		if !ok {
			d.Added = append(d.Added, n.Code)
			continue
		}
		var f []string
		add := func(changed bool, name string) {
			if changed {
				f = append(f, name)
			}
		}
		add(o.Title != n.Title, "Jabatan")
		add(o.ParentCode != n.ParentCode, "Atasan")
		add(o.IsPosition != n.IsPosition, "Posisi")
		add(o.DivisionCode != n.DivisionCode || o.DivisionName != n.DivisionName, "Divisi")
		add(o.Level != n.Level, "Level")
		add(o.EmploymentType != n.EmploymentType, "Jenis Kerja")
		add(o.Vendor != n.Vendor, "Vendor")
		add(o.Region != n.Region, "Region")
		add(!slices.Equal(o.StoreCodes, n.StoreCodes), "Toko")
		add(o.Status != n.Status, "Status")
		add(o.NeedsConfirm != n.NeedsConfirm, "Perlu Konfirmasi")
		if len(f) > 0 {
			d.Changed = append(d.Changed, OrgNodeChange{Code: n.Code, Title: n.Title, Fields: f})
		}
	}
	for _, n := range base {
		if !now[n.Code] {
			d.Removed = append(d.Removed, n.Code)
		}
	}
	sort.Strings(d.Added)
	sort.Strings(d.Removed)
	sort.Slice(d.Changed, func(i, j int) bool { return d.Changed[i].Code < d.Changed[j].Code })
	return d
}

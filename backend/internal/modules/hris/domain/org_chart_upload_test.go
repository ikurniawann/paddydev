package domain

import (
	"reflect"
	"strings"
	"testing"
)

// orgRow is one Bagan row in OrgChartHeaders order.
type orgRow [13]string

func orgMatrix(rows ...orgRow) [][]string {
	m := [][]string{append([]string{}, OrgChartHeaders...)}
	for _, r := range rows {
		m = append(m, append([]string{}, r[:]...))
	}
	return m
}

func uploadNode(t *testing.T, res OrgUploadResult, code string) OrgUploadNode {
	t.Helper()
	for _, n := range res.Nodes {
		if n.Code == code {
			return n
		}
	}
	t.Fatalf("node %s missing: %+v", code, res.Nodes)
	return OrgUploadNode{}
}

func hasUploadIssue(list []UploadIssue, line int, column, msg string) bool {
	for _, i := range list {
		if i.Line == line && i.Column == column && i.Message == msg {
			return true
		}
	}
	return false
}

var (
	ceoRow = orgRow{"EXE-001", "", "CEO", "Ya", "EXE", "Executive", "L0", "Tetap/Kontrak", "", "", "", "", ""}
	gmRow  = orgRow{"EXE-002", "EXE-001", "General Manager", "Ya", "EXE", "Executive", "L1", "Tetap/Kontrak", "", "", "", "", ""}
)

func TestParseOrgChartUploadValid(t *testing.T) {
	m := orgMatrix(ceoRow, gmRow,
		orgRow{"sup-001", "exe-001", "Support", "tidak", "SUP", "Support", "", "", "", "", "", "", ""},
		orgRow{},
		orgRow{" RTL-010 ", "EXE-002", "Shopkeeper", "Ya", "RTL", "Retail Operational", "L7", "Freelance", "", "Bandung", "ppga; PPGI,ppga", "Rencana", "Ya"})
	res, problem := ParseOrgChartUpload(m, []string{"PPGA"})
	if problem != "" {
		t.Fatalf("problem %q", problem)
	}
	if len(res.Nodes) != 4 || len(res.Errors) != 0 || res.HeaderLine != 1 {
		t.Fatalf("nodes %d errors %+v header %d", len(res.Nodes), res.Errors, res.HeaderLine)
	}
	r := uploadNode(t, res, "RTL-010")
	if !reflect.DeepEqual(r.StoreCodes, []string{"PPGA", "PPGI"}) || r.Status != "planned" || !r.NeedsConfirm ||
		r.EmploymentType != "FREELANCE" || r.Line != 6 || r.DivisionName != "Retail Operational" || r.Region != "Bandung" {
		t.Fatalf("RTL-010 %+v", r)
	}
	if s := uploadNode(t, res, "SUP-001"); s.IsPosition || s.ParentCode != "EXE-001" {
		t.Fatalf("SUP-001 %+v", s)
	}
	if c := uploadNode(t, res, "EXE-001"); c.EmploymentType != "PERMANENT_OR_CONTRACT" || c.Status != "active" || c.Level != "L0" {
		t.Fatalf("EXE-001 %+v", c)
	}
	if len(res.Warnings) != 1 || !hasUploadIssue(res.Warnings, 6, "Toko", "Toko PPGI tidak ada sebagai toko aktif di HRIS") {
		t.Fatalf("warnings %+v", res.Warnings)
	}
}

func TestParseOrgChartUploadRowErrors(t *testing.T) {
	long := strings.Repeat("J", 151)
	m := orgMatrix(ceoRow, gmRow,
		orgRow{"EXE-002", "EXE-001", "Dup", "Ya", "EXE", "Executive", "L2", "", "", "", "", "", ""},             // line 4
		orgRow{"ex 1", "EXE-001", "Staf", "Ya", "EXE", "Executive", "L3", "", "", "", "", "", ""},               // 5
		orgRow{"EXE-010", "EXE-001", "", "Ya", "EXE", "Executive", "L3", "", "", "", "", "", ""},                // 6
		orgRow{"EXE-011", "EXE-001", "Staf", "Mungkin", "EXE", "Executive", "L3", "", "", "", "", "", ""},       // 7
		orgRow{"EXE-012", "EXE-001", "Staf", "Ya", "EXE", "Executive", "L9", "", "", "", "", "", ""},            // 8
		orgRow{"EXE-013", "EXE-001", "Staf", "Ya", "EXE", "Executive", "L3", "Kontrakan", "", "", "", "", ""},   // 9
		orgRow{"EXE-014", "EXE-001", "Staf", "Ya", "EXE", "Executive", "L3", "", "", "", "", "Hilang", ""},      // 10
		orgRow{"EXE-015", "EXE-001", "Staf", "Ya", "EXE", "Executive", "L3", "", "", "", "PP GA", "", ""},       // 11
		orgRow{"EXE-016", "EXE-001", "Staf", "Ya", "", "Executive", "L3", "", "", "", "", "", ""},               // 12
		orgRow{"EXE-017", "EXE-001", "Staf", "Ya", "EXE", "", "L3", "", "", "", "", "", ""},                     // 13
		orgRow{"EXE-018", "EXE-001", long, "Ya", "EXE", "Executive", "L3", "", "", "", "", "", ""},              // 14
		orgRow{"EXE-019", "EXE-001", "Staf", "Ya", "EXE", "Executive", "L3", "", "", "", "", "Aktif", "Kadang"}, // 15
	)
	res, problem := ParseOrgChartUpload(m, nil)
	if problem != "" {
		t.Fatal(problem)
	}
	for _, w := range []struct {
		line     int
		col, msg string
	}{
		{4, "Kode", "Kode EXE-002 dipakai lebih dari sekali (baris 3)"},
		{5, "Kode", "Format kode tidak valid"},
		{6, "Jabatan", "Jabatan wajib diisi"},
		{7, "Posisi", "Isi Posisi dengan Ya atau Tidak"},
		{8, "Level", "Level harus L0 sampai L8"},
		{9, "Jenis Kerja", "Jenis kerja tidak dikenal"},
		{10, "Status", "Status harus Aktif, Rencana, Kosong, atau Dibekukan"},
		{11, "Toko", "Kode toko PP GA tidak valid"},
		{12, "Kode Divisi", "Kode divisi wajib diisi"},
		{13, "Divisi", "Divisi wajib diisi"},
		{14, "Jabatan", "Jabatan maksimal 150 karakter"},
		{15, "Perlu Konfirmasi", "Isi Perlu Konfirmasi dengan Ya atau Tidak"},
	} {
		if !hasUploadIssue(res.Errors, w.line, w.col, w.msg) {
			t.Errorf("missing line %d %s %q in %+v", w.line, w.col, w.msg, res.Errors)
		}
	}
	// A row with an error is not a node, so it adds no BR noise.
	for _, n := range res.Nodes {
		if n.Line >= 4 {
			t.Fatalf("bad row kept as node %+v", n)
		}
	}
}

func TestParseOrgChartUploadBRIssues(t *testing.T) {
	m := orgMatrix(ceoRow,
		orgRow{"FIN-001", "", "Finance Head", "Ya", "FIN", "Finance", "L2", "", "", "", "", "", ""},         // 3: 2nd root
		orgRow{"EXE-003", "EXE-001", "Senior Staf", "Ya", "EXE", "Executive", "L0", "", "", "", "", "", ""}, // 4: same level → warning
		orgRow{"FIN-002", "FIN-001", "Kepala", "Ya", "FIN", "Finance", "L1", "", "", "", "", "", ""},        // 5: senior to parent → error
	)
	res, _ := ParseOrgChartUpload(m, nil)
	if !hasUploadIssue(res.Errors, 0, "Kode Atasan", "Bagan harus punya tepat satu puncak (ditemukan 2: EXE-001, FIN-001)") {
		t.Errorf("BR-01 %+v", res.Errors)
	}
	if !hasUploadIssue(res.Errors, 5, "Level", "Level L1 lebih senior dari atasan FIN-001 (L2)") {
		t.Errorf("BR-04 error %+v", res.Errors)
	}
	if !hasUploadIssue(res.Warnings, 4, "Level", "Level L0 sama dengan atasan EXE-001 (L0)") {
		t.Errorf("BR-04 warning %+v", res.Warnings)
	}
}

// Review Focus 5: removing a node that still has children reports them.
func TestParseOrgChartUploadRemovedParent(t *testing.T) {
	m := orgMatrix(ceoRow, gmRow,
		orgRow{"RTL-054", "RTL-053", "Store Leader", "Ya", "RTL", "Retail Operational", "L5", "", "", "Makassar", "PPMR", "", ""})
	res, _ := ParseOrgChartUpload(m, []string{"PPMR"})
	if !hasUploadIssue(res.Errors, 4, "Kode Atasan", "Atasan RTL-053 tidak ada di versi ini") {
		t.Fatalf("BR-03 %+v", res.Errors)
	}
}

func TestParseOrgChartUploadProblems(t *testing.T) {
	if _, p := ParseOrgChartUpload([][]string{{"Nama", "Jabatan"}, {"x", "y"}}, nil); p != `Judul kolom "Kode" tidak ditemukan — pakai template dari halaman Struktur Org` {
		t.Errorf("no header: %q", p)
	}
	if _, p := ParseOrgChartUpload(orgMatrix(orgRow{}), nil); p != "File tidak berisi baris bagan" {
		t.Errorf("empty: %q", p)
	}
	rows := make([]orgRow, 1001)
	for i := range rows {
		rows[i] = orgRow{"X-" + strings.Repeat("1", 1) + string(rune('A'+i%26)), "", "J", "Ya", "EXE", "E", "", "", "", "", "", "", ""}
	}
	if _, p := ParseOrgChartUpload(orgMatrix(rows...), nil); p != "Maksimal 1000 baris per file" {
		t.Errorf("too many: %q", p)
	}
}

func TestOrgLabels(t *testing.T) {
	for code, want := range map[string]string{"PERMANENT_OR_CONTRACT": "Tetap/Kontrak", "INTERN": "Magang", "FREELANCE": "Freelance", "OUTSOURCE": "Outsource", "": ""} {
		if got := EmploymentTypeLabel(code); got != want {
			t.Errorf("EmploymentTypeLabel(%q) = %q", code, got)
		}
	}
	for st, want := range map[string]string{"active": "Aktif", "planned": "Rencana", "vacant": "Kosong", "frozen": "Dibekukan"} {
		if got := NodeStatusLabel(st); got != want {
			t.Errorf("NodeStatusLabel(%q) = %q", st, got)
		}
	}
}

func TestDiffOrgCharts(t *testing.T) {
	base := []OrgNode{{Code: "A", Title: "Kepala"}, {Code: "B", ParentCode: "A", Title: "Staf"}, {Code: "C", ParentCode: "A", Title: "Kurir"}}
	next := []OrgNode{{Code: "A", Title: "Kepala"}, {Code: "B", ParentCode: "D", Title: "Staf Senior"}, {Code: "D", ParentCode: "A", Title: "Wakil"}}
	d := DiffOrgCharts(base, next)
	want := OrgChartDiff{Added: []string{"D"}, Removed: []string{"C"},
		Changed: []OrgNodeChange{{Code: "B", Title: "Staf Senior", Fields: []string{"Jabatan", "Atasan"}}}}
	if !reflect.DeepEqual(d, want) {
		t.Fatalf("diff %+v", d)
	}
	full := DiffOrgCharts(
		[]OrgNode{{Code: "X", Title: "T", IsPosition: true, DivisionCode: "A", Level: "L1", EmploymentType: "INTERN", Region: "R", StoreCodes: []string{"S1"}, Status: "active"}},
		[]OrgNode{{Code: "X", Title: "T", IsPosition: false, DivisionCode: "B", Level: "L2", EmploymentType: "FREELANCE", Region: "Q", StoreCodes: []string{"S2"}, Status: "planned"}})
	if f := full.Changed[0].Fields; !reflect.DeepEqual(f, []string{"Posisi", "Divisi", "Level", "Jenis Kerja", "Region", "Toko", "Status"}) {
		t.Fatalf("fields %v", f)
	}
	e := DiffOrgCharts(nil, nil)
	if e.Added == nil || e.Removed == nil || e.Changed == nil {
		t.Fatalf("nil slices %#v", e)
	}
}

// A row with its own error does not also make its children report a
// missing parent (BR-03): HR fixes the one row, not every child.
func TestParseOrgChartUploadBadParentRowNoCascade(t *testing.T) {
	m := orgMatrix(ceoRow,
		orgRow{"RTL-053", "EXE-001", "Store Supervisor", "Ya", "RTL", "Retail Operational", "L99", "", "", "", "", "", ""},
		orgRow{"RTL-054", "RTL-053", "Store Leader", "Ya", "RTL", "Retail Operational", "L5", "", "", "", "PPMR", "", ""})
	res, _ := ParseOrgChartUpload(m, []string{"PPMR"})
	if len(res.Errors) != 1 || !hasUploadIssue(res.Errors, 3, "Level", "Level harus L0 sampai L8") {
		t.Fatalf("errors %+v", res.Errors)
	}
}

// Review fix: a missing or renamed column is refused instead of read as
// blank (Status would silently become Aktif).
func TestParseOrgChartUploadMissingColumns(t *testing.T) {
	m := [][]string{{"Kode", "Kode Atasan", "Jabatan", "Posisi", "Kode Divisi", "Divisi", "Level", "Jenis Kerja", "Vendor", "Region", "Toko"},
		{"EXE-001", "", "CEO", "Ya", "EXE", "Executive", "L0", "", "", "", ""}}
	if _, p := ParseOrgChartUpload(m, nil); p != "Kolom tidak ada di file: Status, Perlu Konfirmasi — pakai template dari halaman Struktur Org" {
		t.Fatalf("problem %q", p)
	}
}

// Review fix: the upload diff also sees Vendor, the division name and
// Perlu Konfirmasi.
func TestDiffOrgUpload(t *testing.T) {
	base := []OrgUploadNode{{OrgNode: OrgNode{Code: "A", Title: "T", Vendor: "V1", DivisionCode: "OPS"}, DivisionName: "Operations"}}
	next := []OrgUploadNode{{OrgNode: OrgNode{Code: "A", Title: "T", Vendor: "V2", DivisionCode: "OPS"}, DivisionName: "Operasional", NeedsConfirm: true}}
	d := DiffOrgUpload(base, next)
	if len(d.Changed) != 1 || !reflect.DeepEqual(d.Changed[0].Fields, []string{"Divisi", "Vendor", "Perlu Konfirmasi"}) {
		t.Fatalf("diff %+v", d)
	}
}

// EPIC-104 review: a Region typed as a dash means "no region", so it never
// clashes with the "Tanpa region" filter (region=-).
func TestParseOrgChartUploadDashRegion(t *testing.T) {
	for _, dash := range []string{"-", "–", "—", " - "} {
		m := orgMatrix(ceoRow, orgRow{"EXE-002", "EXE-001", "GM", "Ya", "EXE", "Executive", "L1", "", "", dash, "", "", ""})
		res, _ := ParseOrgChartUpload(m, nil)
		if len(res.Errors) != 0 || uploadNode(t, res, "EXE-002").Region != "" {
			t.Fatalf("region %q → %+v %+v", dash, uploadNode(t, res, "EXE-002"), res.Errors)
		}
	}
}

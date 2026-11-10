package domain

import (
	"reflect"
	"testing"
)

var testBranches = []BranchRef{
	{ID: "b1", Code: "ODOO-PPGA", Name: "PPGA - Paddy Playstore Backyard", Active: true},
	{ID: "b2", Code: "ODOO-PPBI", Name: "PPBI - Paddy Playstore Rainforest", Active: true},
	{ID: "b3", Code: "ODOO-EVENT-PVJ", Name: "Event PVJ", Active: false},
	{ID: "b4", Code: "TWEM", Name: "TWEM - Paddy Playstore Tomorrow Lombok Epicentrum Mall", Active: true},
}

var testUsage = map[string]BranchUsage{
	"b1": {Employees: []string{"Ani", "Budi"}, FutureDays: 4},
	"b3": {Employees: []string{"Citra"}},
}

// codeRow is one Data Toko row: Kode Saat Ini, Kode Baru, Nama Baru, Status Baru.
func codeRow(code, newCode, newName, status string) []string {
	return []string{code, "Paddy", "", "", newCode, newName, status}
}

func codeFile(rows ...[]string) [][]string {
	return append([][]string{BranchCodeHeaders}, rows...)
}

func TestBranchNameForCode(t *testing.T) {
	cases := [][4]string{
		{"ODOO-PPGA", "PPGX", "PPGA - Paddy Playstore Backyard", "PPGX - Paddy Playstore Backyard"},
		{"ODOO-PPGA", "PPGA", "PPGA - Paddy Playstore Backyard", "PPGA - Paddy Playstore Backyard"},
		{"ODOO-EVENT-PVJ", "EVPVJ", "Event PVJ", "Event PVJ"},
	}
	for _, c := range cases {
		if got := BranchNameForCode(c[0], c[1], c[2]); got != c[3] {
			t.Errorf("BranchNameForCode(%q,%q,%q) = %q, want %q", c[0], c[1], c[2], got, c[3])
		}
	}
}

func TestValidateBranchCodeUpload(t *testing.T) {
	res, problem := ValidateBranchCodeUpload(codeFile(
		codeRow(" odoo-ppga ", "ppga", "", ""),
		codeRow("ODOO-PPBI", "", "", "Nonaktif"),
		codeRow("ODOO-EVENT-PVJ", "", "", "nonaktif"),
		codeRow("TWEM", "TWEM", "", ""),
		codeRow("XXXX", "", "", ""),
		codeRow("", "", "", ""),
	), testBranches, testUsage)
	if problem != "" {
		t.Fatal(problem)
	}
	if res.HeaderLine != 1 {
		t.Errorf("HeaderLine = %d", res.HeaderLine)
	}
	want := []BranchCodeChange{
		{Line: 2, ID: "b1", OldCode: "ODOO-PPGA", NewCode: "PPGA", OldName: "PPGA - Paddy Playstore Backyard",
			NewName: "PPGA - Paddy Playstore Backyard", OldActive: true, NewActive: true},
		{Line: 3, ID: "b2", OldCode: "ODOO-PPBI", NewCode: "ODOO-PPBI", OldName: "PPBI - Paddy Playstore Rainforest",
			NewName: "PPBI - Paddy Playstore Rainforest", OldActive: true, NewActive: false},
	}
	if !reflect.DeepEqual(res.Changes, want) {
		t.Errorf("changes\n got %+v\nwant %+v", res.Changes, want)
	}
	wantErr := []string{"Baris 6, kolom Kode Saat Ini: Kode XXXX tidak ditemukan"}
	for _, w := range wantErr {
		if !issueText(res.Errors, w) {
			t.Errorf("missing error %q in %+v", w, res.Errors)
		}
	}
	if len(res.Errors) != len(wantErr) {
		t.Errorf("errors %+v", res.Errors)
	}
	warn := "Baris 2: Kode ODOO-PPGA → PPGA: file Data Kerja/Jadwal yang diunduh sebelumnya memakai kode lama — unduh ulang"
	if len(res.Warnings) != 1 || !issueText(res.Warnings, warn) {
		t.Errorf("warnings %+v", res.Warnings)
	}
}

func TestValidateBranchCodeUploadErrors(t *testing.T) {
	res, _ := ValidateBranchCodeUpload(codeFile(
		codeRow("ODOO-PPGA", "PP GA", "", ""),
		codeRow("ODOO-PPBI", "NEW1", "", ""),
		codeRow("TWEM", "NEW1", "", ""),
		codeRow("ODOO-PPBI", "", "", "Tutup"),
	), testBranches, testUsage)
	for _, w := range []string{
		"Baris 2, kolom Kode Baru: Kode baru hanya huruf besar, angka, dan tanda -, 2–30 karakter",
		"Baris 4, kolom Kode Baru: Kode baru NEW1 ganda di file (baris 3 & 4)",
		"Baris 5, kolom Kode Saat Ini: Kode ODOO-PPBI ganda di file (baris 3 & 5)",
	} {
		if !issueText(res.Errors, w) {
			t.Errorf("missing error %q in %+v", w, res.Errors)
		}
	}

	res, _ = ValidateBranchCodeUpload(codeFile(
		codeRow("ODOO-PPGA", "TWEM", "", ""),
		codeRow("ODOO-PPBI", "", "", "Tutup"),
	), testBranches, testUsage)
	for _, w := range []string{
		"Baris 2, kolom Kode Baru: Kode TWEM sudah dipakai toko TWEM - Paddy Playstore Tomorrow Lombok Epicentrum Mall",
		"Baris 3, kolom Status Baru: Status harus Aktif atau Nonaktif",
	} {
		if !issueText(res.Errors, w) {
			t.Errorf("missing error %q in %+v", w, res.Errors)
		}
	}

	res, _ = ValidateBranchCodeUpload(codeFile(codeRow("ODOO-PPGA", "", "", "Nonaktif")), testBranches, testUsage)
	want := "Baris 2, kolom Status Baru: Toko ODOO-PPGA masih dipakai: 2 karyawan aktif (Ani, Budi) dan 4 jadwal mulai hari ini — pindahkan dulu lewat Penempatan"
	if !issueText(res.Errors, want) || len(res.Changes) != 0 {
		t.Errorf("errors %+v changes %+v", res.Errors, res.Changes)
	}

	many := map[string]BranchUsage{"b1": {Employees: []string{"A", "B", "C", "D", "E", "F", "G"}}}
	res, _ = ValidateBranchCodeUpload(codeFile(codeRow("ODOO-PPGA", "", "", "Nonaktif")), testBranches, many)
	want = "Baris 2, kolom Status Baru: Toko ODOO-PPGA masih dipakai: 7 karyawan aktif (A, B, C, D, E, dan 2 lainnya) — pindahkan dulu lewat Penempatan"
	if !issueText(res.Errors, want) {
		t.Errorf("errors %+v", res.Errors)
	}

	if _, problem := ValidateBranchCodeUpload([][]string{{"Kode", "Nama"}}, testBranches, testUsage); problem != `Judul kolom "Kode Saat Ini" tidak ditemukan — pakai template dari halaman Penempatan` {
		t.Errorf("header problem %q", problem)
	}
}

func TestValidateBranchCodeUploadNewName(t *testing.T) {
	res, _ := ValidateBranchCodeUpload(codeFile(codeRow("ODOO-PPGA", "PPGX", " Toko Baru ", "")), testBranches, testUsage)
	if len(res.Changes) != 1 || res.Changes[0].NewName != "Toko Baru" || res.Changes[0].NewCode != "PPGX" {
		t.Fatalf("changes %+v", res.Changes)
	}
	long := make([]byte, 151)
	for i := range long {
		long[i] = 'a'
	}
	res, _ = ValidateBranchCodeUpload(codeFile(codeRow("ODOO-PPGA", "", string(long), "")), testBranches, testUsage)
	if !issueText(res.Errors, "Baris 2, kolom Nama Baru: Nama baru maksimal 150 karakter") {
		t.Fatalf("errors %+v", res.Errors)
	}
}

// A code held by branches of two companies (the DB is unique per company
// only) must not send one row's edit to the other branch (final review).
func TestValidateBranchCodeUploadAmbiguousCode(t *testing.T) {
	branches := append(append([]BranchRef{}, testBranches...),
		BranchRef{ID: "k1", Code: "KANTOR", Name: "Kantor Pusat A", Active: true},
		BranchRef{ID: "k2", Code: "kantor", Name: "Kantor Pusat B", Active: true})
	res, _ := ValidateBranchCodeUpload(codeFile(
		codeRow("KANTOR", "KTRA", "", ""),
		codeRow("ODOO-PPGA", "KANTOR", "", ""),
	), branches, testUsage)
	if len(res.Changes) != 0 {
		t.Fatalf("an ambiguous code must change nothing: %+v", res.Changes)
	}
	want := "Baris 2, kolom Kode Saat Ini: Kode KANTOR dipakai 2 toko (Kantor Pusat A, Kantor Pusat B) — ubah lewat Settings → Business"
	if !issueText(res.Errors, want) {
		t.Errorf("missing %q in %+v", want, res.Errors)
	}
	if !issueText(res.Errors, "Baris 3, kolom Kode Baru: Kode KANTOR sudah dipakai toko Kantor Pusat A") {
		t.Errorf("errors %+v", res.Errors)
	}
}

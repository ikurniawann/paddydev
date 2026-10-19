package domain

import (
	"regexp"
	"strconv"
	"testing"
)

func TestEmployeeColumnsAreHuman(t *testing.T) {
	want := []string{"NIP", "Nama Lengkap", "Email", "No. HP", "Tanggal Masuk", "Status Kepegawaian", "Tipe Jadwal",
		"Cabang Utama", "Departemen", "Jabatan", "NIP Atasan", "NIK (KTP)", "NPWP", "Tanggal Lahir", "Jenis Kelamin",
		"Status Pernikahan", "Alamat", "Kota", "Provinsi", "Kode Pos", "Nama Bank", "No. Rekening",
		"No. BPJS Ketenagakerjaan", "No. BPJS Kesehatan", "Kontak Darurat (Nama)", "Kontak Darurat (No. HP)",
		"Kontak Darurat (Hubungan)", "Nama Atasan", "Status Aktif", "Tanggal Keluar", "Terakhir Diubah"}
	if len(EmployeeColumns) != len(want) {
		t.Fatalf("columns = %d, want %d", len(EmployeeColumns), len(want))
	}
	technical := regexp.MustCompile(`_|\bid\b|true|false`)
	for i, c := range EmployeeColumns {
		if c.Header != want[i] {
			t.Fatalf("column %d = %q, want %q", i, c.Header, want[i])
		}
		if technical.MatchString(c.Header) {
			t.Fatalf("technical header %q", c.Header)
		}
	}
	if k, ok := EmployeeHeaderKey("  nama lengkap * "); !ok || k != "full_name" {
		t.Fatalf("header key = %q", k)
	}
}

func lk() EmployeeLookups {
	return EmployeeLookups{
		Statuses:    map[string]string{"kontrak (pkwt)": "contract", "masa percobaan": "probation"},
		Branches:    map[string]string{"ppln": "b1"},
		Departments: map[string]string{"retail operational": "d1"},
		Positions:   map[string]string{"shopkeeper staff": "p1"},
		ExistingNIP: map[string]EmployeeRef{"202505272": {ID: "e1", NIP: "202505272", Name: "Feti", Active: true}},
		EmailOwner: map[string]EmployeeRef{
			"feti@x.id": {ID: "e1", NIP: "202505272", Name: "Feti", Active: true},
			"lama@x.id": {ID: "e9", NIP: "201901001", Name: "Lama", Active: false},
		},
		ManagerNIP: map[string]string{"202310037": "m1"},
	}
}

var hdr = []string{"NIP", "Nama Lengkap *", "Email *", "No. HP", "Tanggal Masuk *", "Status Kepegawaian *", "Tipe Jadwal *", "Cabang Utama *", "Jenis Kelamin", "NIK (KTP)", "Status Aktif"}

func TestValidateEmployeeImportAdd(t *testing.T) {
	rows := [][]string{
		{"", "Baru Satu", "baru@x.id", "0812-3456-7890", "21/10/2026", "Kontrak (PKWT)", "Shift", "PPLN", "Perempuan", "3273012345678901", "Aktif"},
		{"202505272", "Feti", "feti@x.id", "", "01/06/2025", "Kontrak (PKWT)", "Shift", "PPLN", "", "", ""},
		{"", "Tanpa Email", "", "", "21/10/2026", "Kontrak (PKWT)", "Shift", "PPLN", "", "", ""},
		{"", "Status Salah", "s@x.id", "", "21/10/2026", "Karyawan Tetap", "Shift", "PPLN", "", "", ""},
		{"", "NIK Pendek", "n@x.id", "", "2026-10-21", "Kontrak (PKWT)", "shift", "ppln", "", "3273", ""},
		{"", "Email Lama", "lama@x.id", "", "21/10/2026", "Kontrak (PKWT)", "Shift", "PPLN", "", "", ""},
	}
	out, errs, warns := ValidateEmployeeImport("add", hdr, rows, 2, lk())
	wantErr := []string{
		"Baris 3: NIP 202505272 sudah ada (mode Tambah hanya untuk karyawan baru)",
		"Baris 4, kolom Email: wajib diisi untuk karyawan baru",
		"Baris 5, kolom Status Kepegawaian: 'Karyawan Tetap' tidak ada di daftar pilihan",
		"Baris 6, kolom NIK (KTP): harus 16 angka",
	}
	for _, w := range wantErr {
		if !issueText(errs, w) {
			t.Errorf("missing error %q in %+v", w, errs)
		}
	}
	if !issueText(warns, "Baris 7, kolom Email: lama@x.id milik Lama (NIP 201901001, resign) — akan dilepas bila dikonfirmasi") {
		t.Errorf("warnings %+v", warns)
	}
	if out[0].Action != "create" || out[0].Set["phone"] != "081234567890" || out[0].Set["join_date"] != "2026-10-21" ||
		out[0].Set["gender"] != "female" || out[0].Set["schedule_type"] != "shift" || out[0].Branch != "b1" {
		t.Fatalf("row 1 = %+v", out[0])
	}
	if _, ok := out[0].Set["active"]; ok {
		t.Fatal("export-only column must be ignored")
	}
	if out[5].ReleaseEmailFrom != "e9" {
		t.Fatalf("release = %q", out[5].ReleaseEmailFrom)
	}
}

func TestValidateEmployeeImportReplaceKeepsBlankCells(t *testing.T) {
	rows := [][]string{{"202505272", "Feti P.", "", "", "", "", "Non-Shift", "", "", "", ""}}
	out, errs, _ := ValidateEmployeeImport("replace", hdr, rows, 2, lk())
	if len(errs) != 0 {
		t.Fatalf("errors %+v", errs)
	}
	if out[0].Action != "update" || len(out[0].Set) != 2 || out[0].Set["full_name"] != "Feti P." || out[0].Set["schedule_type"] != "non_shift" {
		t.Fatalf("set = %+v (blank cells must not overwrite)", out[0].Set)
	}
	missing, errs2, _ := ValidateEmployeeImport("replace", hdr, [][]string{{"", "X", "x@x.id", "", "21/10/2026", "Kontrak (PKWT)", "Shift", "PPLN", "", "", ""}}, 2, lk())
	_ = missing
	if !issueText(errs2, "Baris 2, kolom NIP: wajib diisi di mode Timpa") {
		t.Fatalf("errors %+v", errs2)
	}
}

func issueText(list []UploadIssue, text string) bool {
	for _, i := range list {
		got := "Baris " + strconv.Itoa(i.Line)
		if i.Column != "" {
			got += ", kolom " + i.Column
		}
		if got+": "+i.Message == text {
			return true
		}
	}
	return false
}

func TestValidateEmployeeImportReplaceSkipsUnchangedCells(t *testing.T) {
	l := lk()
	// Data sekarang persis seperti di file export (No. HP lama format 62…, NIK lama 15 digit).
	l.Current = map[string]map[string]string{"202505272": {
		"full_name": "Feti", "phone": "6289522422791", "ktp": "327301234567890", "join_date": "2025-06-01",
		"schedule_type": "Shift", "branch": "PPLN",
	}}
	h := []string{"NIP", "Nama Lengkap *", "No. HP", "NIK (KTP)", "Tanggal Masuk *", "Tipe Jadwal *", "Cabang Utama *"}
	out, errs, _ := ValidateEmployeeImport("replace", h, [][]string{
		{"202505272", "Feti", "6289522422791", "327301234567890", "2025-06-01T00:00:00.000Z", "shift", "PPLN"},
	}, 2, l)
	if len(errs) != 0 || len(out[0].Set) != 0 || out[0].Branch != "" {
		t.Fatalf("unchanged row: errs %+v set %+v branch %q", errs, out[0].Set, out[0].Branch)
	}
	// Sel yang benar-benar berubah tetap divalidasi.
	_, errs, _ = ValidateEmployeeImport("replace", h, [][]string{
		{"202505272", "Feti", "12345", "327301234567890", "01/06/2025", "Shift", "PPLN"},
	}, 2, l)
	if !issueText(errs, "Baris 2, kolom No. HP: No. HP harus diawali 08 / 62 / +62 dan berisi 10–14 angka") {
		t.Fatalf("changed invalid phone must fail: %+v", errs)
	}
}

func TestValidateEmployeeImportAmbiguousMasterName(t *testing.T) {
	l := lk()
	l.Positions["store manager"] = "" // dua jabatan bernama sama
	h := []string{"NIP", "Nama Lengkap *", "Jabatan"}
	_, errs, _ := ValidateEmployeeImport("replace", h, [][]string{{"202505272", "Feti", "Store Manager"}}, 2, l)
	if !issueText(errs, "Baris 2, kolom Jabatan: 'Store Manager' ada lebih dari satu di master — pilih lewat form karyawan") {
		t.Fatalf("errs = %+v", errs)
	}
}

func TestValidateEmployeeImportEmailAndIdentityRules(t *testing.T) {
	l := lk()
	l.ExistingNIP["202505272"] = EmployeeRef{ID: "e1", NIP: "202505272", Name: "Feti", Active: true, HasLogin: true}
	l.ExistingNIP["202310037"] = EmployeeRef{ID: "e2", NIP: "202310037", Name: "Nanda", Active: true}
	l.KTPOwner = map[string]EmployeeRef{"3273012345678901": {ID: "e2", NIP: "202310037", Name: "Nanda", Active: true}}
	l.NPWPOwner = map[string]EmployeeRef{}
	h := []string{"NIP", "Nama Lengkap *", "Email *", "NIK (KTP)", "NPWP", "Tanggal Masuk *", "Status Kepegawaian *", "Tipe Jadwal *", "Cabang Utama *"}
	newRow := func(email, ktp, npwp string) []string {
		return []string{"", "Baru", email, ktp, npwp, "21/10/2026", "Kontrak (PKWT)", "Shift", "PPLN"}
	}
	_, errs, _ := ValidateEmployeeImport("replace", h, [][]string{
		{"202505272", "Feti", "feti.baru@x.id", "", "", "", "", "", ""}, // punya akun login: email tidak boleh diubah lewat import
		{"202310037", "Nanda", "lama@x.id", "", "", "", "", "", ""},     // email karyawan resign untuk karyawan yang sudah ada
	}, 2, l)
	for _, want := range []string{
		"Baris 2, kolom Email: karyawan ini punya akun login — ubah email lewat Manajemen User",
		"Baris 3, kolom Email: lama@x.id milik Lama (NIP 201901001, resign) — untuk karyawan yang sudah ada, ubah lewat form karyawan",
	} {
		if !issueText(errs, want) {
			t.Errorf("missing %q in %+v", want, errs)
		}
	}
	_, errs, _ = ValidateEmployeeImport("add", h, [][]string{
		newRow("a@x.id", "3273012345678901", ""),
		newRow("b@x.id", "3273012345678999", "123456789012345"),
		newRow("b@x.id", "3273012345678999", "123456789012345"),
	}, 2, l)
	for _, want := range []string{
		"Baris 2, kolom NIK (KTP): NIK 3273012345678901 sudah dipakai Nanda (NIP 202310037)",
		"Baris 4, kolom Email: b@x.id ganda di file (baris 3 & 4)",
		"Baris 4, kolom NIK (KTP): NIK 3273012345678999 ganda di file (baris 3 & 4)",
		"Baris 4, kolom NPWP: NPWP 123456789012345 ganda di file (baris 3 & 4)",
	} {
		if !issueText(errs, want) {
			t.Errorf("missing %q in %+v", want, errs)
		}
	}
}

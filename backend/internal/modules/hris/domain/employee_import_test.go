package domain

import (
	"regexp"
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

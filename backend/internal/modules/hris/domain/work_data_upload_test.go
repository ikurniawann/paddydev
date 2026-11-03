package domain

import (
	"strings"
	"testing"
)

func workDataMasters() WorkDataMasters {
	return WorkDataMasters{
		Employees: map[string]WorkDataEmployee{
			"A1": {ID: "ea", NIP: "A1", Name: "Ana Shift", ScheduleType: "shift", Active: true},
			"B1": {ID: "eb", NIP: "B1", Name: "Budi HQ", ScheduleType: "non_shift", Active: true},
			"C1": {ID: "ec", NIP: "C1", Name: "Cici Keluar", ScheduleType: "shift", Active: false},
			"D1": {ID: "ed", NIP: "D1", Name: "Dodi Roster", ScheduleType: "shift", Active: true, RosterLast: "2026-11-30"},
			"E1": {ID: "ee", NIP: "E1", Name: "Eka Pola", ScheduleType: "non_shift", Active: true},
		},
		Branches:    map[string]string{"BXST": "br-bxst", "DUA": ""},
		Shifts:      map[string]UploadShift{"SPG1": {ID: "s1", Code: "SPG1", Start: "08:00:00", End: "16:00:00", Active: true}, "SPG3": {ID: "s3", Code: "SPG3", Start: "13:30:00", End: "21:30:00", Active: true}},
		Departments: map[string]string{"retail operational": "d-retail"},
		Positions:   map[string]string{"shopkeeper staff": "p-shop", "supervisor": ""},
		Today:       "2026-11-03",
		ClosedFrom:  func(d string) bool { return d <= "2026-10-31" },
	}
}

// wdRow builds a Data row: nip, toko, tipe, 7 hari, berlaku, dept, jabatan.
func wdRow(nip, toko, tipe string, days []string, from, dept, pos string) []string {
	r := make([]string, len(WorkDataHeader))
	r[0] = nip
	r[7], r[8] = toko, tipe
	for i, d := range days {
		r[9+i] = d
	}
	r[16], r[17], r[18] = from, dept, pos
	return r
}

func wdWeek(codes ...string) []string { return codes }

func messages(issues []UploadIssue) string {
	var out []string
	for _, i := range issues {
		out = append(out, i.Message)
	}
	return strings.Join(out, " | ")
}

func TestValidateWorkData(t *testing.T) {
	m := workDataMasters()
	full := wdWeek("SPG1", "SPG1", "SPG1", "SPG1", "SPG1", "SPG1", "LIBUR")
	check := func(name string, r []string, wantErr, wantWarn string) []WorkDataRow {
		t.Helper()
		valid, errs, warns := ValidateWorkData(WorkDataHeader, [][]string{r}, 2, m)
		if got := messages(errs); got != wantErr {
			t.Errorf("%s: errors %q, want %q", name, got, wantErr)
		}
		if got := messages(warns); got != wantWarn {
			t.Errorf("%s: warnings %q, want %q", name, got, wantWarn)
		}
		return valid
	}

	valid := check("valid", wdRow("A1", "BXST", "Shift", full, "2026-11-10", "Retail Operational", "Shopkeeper Staff"), "", "")
	if len(valid) != 1 || *valid[0].BranchID != "br-bxst" || valid[0].Pattern[6].Shift1 != nil ||
		*valid[0].Pattern[0].Shift1 != "s1" || valid[0].From != "2026-11-10" || *valid[0].DepartmentID != "d-retail" {
		t.Fatalf("valid = %+v", valid)
	}
	check("kode toko", wdRow("A1", "ZZZ", "", nil, "2026-11-10", "", ""), "Kode toko ZZZ tidak dikenal/nonaktif", "")
	check("non-shift berpola", wdRow("B1", "", "", wdWeek("SPG1"), "2026-11-10", "", ""),
		"Karyawan Non-Shift mengikuti jadwal tetap departemen — kosongkan Senin–Minggu", "")
	check("ubah ke non-shift berpola", wdRow("A1", "", "Non-Shift", full, "2026-11-10", "", ""),
		"Karyawan Non-Shift mengikuti jadwal tetap departemen — kosongkan Senin–Minggu", "")
	check("pola tidak lengkap", wdRow("A1", "", "", wdWeek("SPG1", "SPG1", "SPG1", "SPG1", "SPG1"), "2026-11-10", "", ""),
		"Pola mingguan harus diisi 7 hari (Senin–Minggu)", "")
	two := wdWeek("SPG1+SPG3", "SPG1", "SPG1", "SPG1", "SPG1", "SPG1", "LIBUR")
	check("dua shift tumpang tindih", wdRow("A1", "", "", two, "2026-11-10", "", ""),
		"Senin: Shift kedua harus mulai setelah shift pertama selesai (SPG1 selesai 16.00, SPG3 mulai 13.30)", "")
	check("kode shift salah", wdRow("A1", "", "", wdWeek("XX1", "SPG1", "SPG1", "SPG1", "SPG1", "SPG1", "LIBUR"), "2026-11-10", "", ""),
		"Senin: kode shift XX1 tidak dikenal/nonaktif", "")
	check("lampau", wdRow("A1", "BXST", "", nil, "2026-11-01", "", ""), "Berlaku Mulai sudah lewat (01-11-2026)", "")
	check("tanggal salah", wdRow("A1", "BXST", "", nil, "10/11/2026", "", ""), "Berlaku Mulai harus berformat YYYY-MM-DD", "")
	check("tanpa tanggal", wdRow("A1", "BXST", "", nil, "", "", ""), "Berlaku Mulai wajib diisi bila ada perubahan", "")
	check("nonaktif", wdRow("C1", "BXST", "", nil, "2026-11-10", "", ""), "NIP C1 nonaktif", "")
	check("tidak dikenal", wdRow("Z9", "BXST", "", nil, "2026-11-10", "", ""), "NIP Z9 tidak dikenal", "")
	check("tipe salah", wdRow("A1", "", "Pagi", nil, "2026-11-10", "", ""), "Tipe Jadwal harus Shift atau Non-Shift", "")
	check("jabatan salah", wdRow("A1", "", "", nil, "", "", "Raja"), "Jabatan Raja tidak dikenal", "")
	check("tanpa libur", wdRow("A1", "", "", wdWeek("SPG1", "SPG1", "SPG1", "SPG1", "SPG1", "SPG1", "SPG1"), "2026-11-10", "", ""),
		"", "Pola tanpa hari LIBUR (masuk 7 hari)")
	check("roster sudah ada", wdRow("D1", "", "", full, "2026-11-10", "", ""),
		"", "Sudah ada jadwal per tanggal sampai 30-11-2026; jadwal per tanggal tetap berlaku di atas pola")
	check("ke non-shift hapus roster", wdRow("D1", "", "Non-Shift", nil, "2026-11-10", "", ""),
		"", "Ubah ke Non-Shift menghapus jadwal per tanggal mulai 10-11-2026 (sampai 30-11-2026)")
	check("ke shift tanpa pola", wdRow("E1", "", "Shift", nil, "2026-11-10", "", ""),
		"Ubah ke Shift wajib mengisi pola Senin–Minggu (jadwal tetap berhenti mulai Berlaku Mulai)", "")
	if v := check("ke shift berpola", wdRow("E1", "", "Shift", full, "2026-11-10", "", ""), "", ""); len(v) != 1 {
		t.Fatalf("ke shift berpola = %+v", v)
	}
	check("kode toko ganda", wdRow("A1", "DUA", "", nil, "2026-11-10", "", ""),
		"Kode toko DUA ada lebih dari satu di master — hubungi tim", "")

	check("jabatan ganda di master", wdRow("A1", "", "", nil, "", "", "Supervisor"),
		"Jabatan Supervisor ada lebih dari satu di master — tulis maksudnya di Catatan dan hubungi tim", "")
	// Sel tanggal Excel terbaca sebagai timestamp ISO (Review Focus 5).
	if v := check("tanggal excel", wdRow("A1", "BXST", "", nil, "2026-11-10T00:00:00.000Z", "", ""), "", ""); len(v) != 1 || v[0].From != "2026-11-10" {
		t.Fatalf("tanggal excel = %+v", v)
	}
	// Hanya departemen/jabatan: tanpa tanggal tetap boleh.
	if v := check("jabatan saja", wdRow("A1", "", "", nil, "", "", "Shopkeeper Staff"), "", ""); len(v) != 1 || *v[0].PositionID != "p-shop" {
		t.Fatalf("jabatan saja = %+v", v)
	}
	// Baris tanpa perubahan diabaikan.
	if v := check("kosong", wdRow("A1", "", "", nil, "", "", ""), "", ""); len(v) != 0 {
		t.Fatalf("baris kosong = %+v", v)
	}
	// NIP ganda.
	_, errs, _ := ValidateWorkData(WorkDataHeader, [][]string{
		wdRow("A1", "BXST", "", nil, "2026-11-10", "", ""), wdRow("A1", "BXST", "", nil, "2026-11-10", "", "")}, 2, m)
	if got := messages(errs); got != "NIP A1 ganda (juga di baris 2)" || errs[0].Line != 3 {
		t.Fatalf("ganda = %q line %d", got, errs[0].Line)
	}
	// Header salah.
	_, errs, _ = ValidateWorkData([]string{"NIP", "Nama"}, nil, 2, m)
	if got := messages(errs); got != "File bukan template data kerja (kolom tidak sesuai)" {
		t.Fatalf("header = %q", got)
	}
}

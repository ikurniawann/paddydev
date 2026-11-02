package employeekeys

import (
	"strings"
	"testing"
)

func TestConflictMessage(t *testing.T) {
	c := Conflict{Field: "ktp", Name: "Alya Nur Hikmah", NIP: "202504229", Active: false}
	want := "No. KTP sudah dipakai karyawan Alya Nur Hikmah (NIP 202504229, nonaktif) — aktifkan kembali data tersebut dari halaman detail karyawan"
	if c.Message() != want {
		t.Fatalf("got %q", c.Message())
	}
	c = Conflict{Field: "nip", Name: "Budi", NIP: "1", Active: true}
	if c.Message() != "NIP sudah dipakai karyawan Budi (NIP 1, aktif)" {
		t.Fatal(c.Message())
	}
	c = Conflict{Field: "npwp", Name: "Sari", NIP: "9", Active: false}
	if got, want := c.Message(), "NPWP sudah dipakai karyawan Sari (NIP 9, nonaktif) — aktifkan kembali data tersebut dari halaman detail karyawan"; got != want {
		t.Fatalf("got %q", got)
	}
	for constraint, want := range map[string]string{
		"employees_ktp_key": "No. KTP sudah dipakai karyawan lain", "employees_npwp_key": "NPWP sudah dipakai karyawan lain",
		"employees_nip_key": "NIP sudah dipakai karyawan lain"} {
		if got, ok := ConstraintMessage(constraint); !ok || got != want {
			t.Fatalf("%s: %q", constraint, got)
		}
	}
	if _, ok := ConstraintMessage("other_key"); ok {
		t.Fatal("unknown constraint must not map")
	}
}

func TestDigits(t *testing.T) {
	for in, want := range map[string]string{
		"3273 0101 9900 0001": "3273010199000001", "12.345.678.9-012.000": "123456789012000", "   ": "", "-": "", "": "",
	} {
		if got := digits(in); got != want {
			t.Fatalf("digits(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCleanNIP(t *testing.T) {
	s := func(v string) *string { return &v }
	long := strings.Repeat("9", 51)
	for _, tc := range []struct {
		in   *string
		want string
		err  string
	}{
		{s("  202511001 "), "202511001", ""},
		{s(strings.Repeat("9", 50)), strings.Repeat("9", 50), ""},
		{s(" " + strings.Repeat("9", 50) + " "), strings.Repeat("9", 50), ""},
		{nil, "", "NIP wajib diisi"},
		{s(""), "", "NIP wajib diisi"},
		{s(" \t "), "", "NIP wajib diisi"},
		{s(long), "", "NIP maksimal 50 karakter"},
	} {
		got, err := CleanNIP(tc.in)
		if got != tc.want || (tc.err == "") != (err == nil) || (err != nil && err.Error() != tc.err) {
			t.Errorf("CleanNIP(%v) = %q, %v; want %q, %q", tc.in, got, err, tc.want, tc.err)
		}
	}
}

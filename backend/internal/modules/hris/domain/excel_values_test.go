package domain

import "testing"

func TestParseDateCell(t *testing.T) {
	cases := map[string]string{
		"2026-10-21":               "2026-10-21",
		"2026-10-21T00:00:00.000Z": "2026-10-21",
		"21/10/2026":               "2026-10-21",
		"21-10-2026":               "2026-10-21",
		" 1/6/2025 ":               "2025-06-01",
	}
	for in, want := range cases {
		got, ok := ParseDateCell(in)
		if !ok || got != want {
			t.Errorf("ParseDateCell(%q) = %q,%v want %q", in, got, ok, want)
		}
	}
	for _, bad := range []string{"31/02/2026", "2026/13/01", "kemarin", "45123"} {
		if _, ok := ParseDateCell(bad); ok {
			t.Errorf("ParseDateCell(%q) accepted", bad)
		}
	}
	if FormatDateID("2026-10-21") != "21/10/2026" || FormatDateID("") != "" {
		t.Fatal("FormatDateID")
	}
}

func TestCleanNIP(t *testing.T) {
	if nip, msg := CleanNIP(" 202505272 "); nip != "202505272" || msg != "" {
		t.Fatalf("got %q %q", nip, msg)
	}
	if nip, _ := CleanNIP("EMP-2026-00004"); nip != "EMP-2026-00004" {
		t.Fatalf("got %q", nip)
	}
	if _, msg := CleanNIP("2.02505E+08"); msg == "" {
		t.Fatal("scientific notation must be rejected")
	}
}

func TestNormalizePhoneAndDigits(t *testing.T) {
	for in, want := range map[string]string{"0812-3456-7890": "081234567890", "+62 812 3456 7890": "081234567890", "6281234567890": "081234567890"} {
		if got, msg := NormalizePhone(in); got != want || msg != "" {
			t.Errorf("NormalizePhone(%q) = %q %q", in, got, msg)
		}
	}
	if _, msg := NormalizePhone("12345"); msg == "" {
		t.Fatal("bad phone accepted")
	}
	if CleanDigits("3273.0123 4567-8901") != "3273012345678901" {
		t.Fatal("CleanDigits")
	}
}

func TestChoices(t *testing.T) {
	if v, ok := ValueOf(MaritalLabels, "  belum   menikah "); !ok || v != "single" {
		t.Fatalf("got %q %v", v, ok)
	}
	if LabelOf(ScheduleTypeLabels, "non_shift") != "Non-Shift" {
		t.Fatal("LabelOf")
	}
	if NormalizeHeader("Nama  Lengkap *") != "nama lengkap" {
		t.Fatal("NormalizeHeader")
	}
}

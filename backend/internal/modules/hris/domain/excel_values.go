package domain

import (
	"regexp"
	"strings"
	"time"
)

// Values of the HR-facing Excel files (Spec 2026-10-19 §3): labels people
// read and type, never database values.

type Choice struct{ Value, Label string }

var (
	GenderLabels       = []Choice{{"male", "Laki-laki"}, {"female", "Perempuan"}}
	MaritalLabels      = []Choice{{"single", "Belum Menikah"}, {"married", "Menikah"}, {"divorced", "Cerai Hidup"}, {"widowed", "Cerai Mati"}}
	ScheduleTypeLabels = []Choice{{"shift", "Shift"}, {"non_shift", "Non-Shift"}}
)

var spaces = regexp.MustCompile(`\s+`)

func squash(s string) string {
	return strings.ToLower(strings.TrimSpace(spaces.ReplaceAllString(s, " ")))
}

func LabelOf(choices []Choice, value string) string {
	for _, c := range choices {
		if c.Value == value {
			return c.Label
		}
	}
	return ""
}

func ValueOf(choices []Choice, label string) (string, bool) {
	want := squash(label)
	for _, c := range choices {
		if squash(c.Label) == want {
			return c.Value, true
		}
	}
	return "", false
}

func Labels(choices []Choice) []string {
	out := make([]string, len(choices))
	for i, c := range choices {
		out[i] = c.Label
	}
	return out
}

var dmy = regexp.MustCompile(`^(\d{1,2})[/-](\d{1,2})[/-](\d{4})$`)

// ParseDateCell accepts an ISO date, an ExcelJS-style ISO timestamp (date
// cells as read by platform/xlsx), DD/MM/YYYY and DD-MM-YYYY.
func ParseDateCell(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		if t, err := time.Parse(DateLayout, s[:10]); err == nil {
			return t.Format(DateLayout), true
		}
		return "", false
	}
	m := dmy.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	t, err := time.Parse("2/1/2006", m[1]+"/"+m[2]+"/"+m[3])
	if err != nil {
		return "", false
	}
	return t.Format(DateLayout), true
}

func FormatDateID(iso string) string {
	t, err := time.Parse(DateLayout, iso)
	if err != nil {
		return ""
	}
	return t.Format("02/01/2006")
}

var digitNoise = strings.NewReplacer(" ", "", ".", "", "-", "", "/", "", "'", "")

func CleanDigits(s string) string { return digitNoise.Replace(strings.TrimSpace(s)) }

var scientific = regexp.MustCompile(`(?i)^\d+(\.\d+)?e\+?\d+$`)

// CleanNIP trims a NIP; a NIP Excel turned into scientific notation is an
// error the person can fix (format the column as text and retype it).
func CleanNIP(s string) (string, string) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "'"))
	if scientific.MatchString(s) {
		return "", "NIP berubah jadi format angka Excel (mis. 2,02E+08). Ubah kolom NIP ke format Teks lalu ketik ulang."
	}
	return s, ""
}

// NormalizePhone returns 08… from 08…, 62… or +62….
func NormalizePhone(s string) (string, string) {
	d := CleanDigits(strings.TrimPrefix(strings.TrimSpace(s), "+"))
	if strings.HasPrefix(d, "62") {
		d = "0" + d[2:]
	}
	if !strings.HasPrefix(d, "08") || len(d) < 10 || len(d) > 14 || strings.Trim(d, "0123456789") != "" {
		return "", "No. HP harus diawali 08 / 62 / +62 dan berisi 10–14 angka"
	}
	return d, ""
}

func NormalizeHeader(s string) string {
	return squash(strings.TrimSuffix(strings.TrimSpace(s), "*"))
}

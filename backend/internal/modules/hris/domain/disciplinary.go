package domain

// Surat Teguran / Surat Peringatan yang diterbitkan HR (EPIC-094 R9).

// DisciplinaryCodes are the letter types in order of severity.
var DisciplinaryCodes = []string{"teguran", "sp1", "sp2", "sp3"}

var disciplinaryLabels = map[string]string{
	"teguran": "Surat Teguran",
	"sp1":     "Surat Peringatan 1",
	"sp2":     "Surat Peringatan 2",
	"sp3":     "Surat Peringatan 3",
}

// DisciplinaryLabel is the label of a letter type, "" when unknown.
func DisciplinaryLabel(code string) string { return disciplinaryLabels[code] }

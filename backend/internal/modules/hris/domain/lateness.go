package domain

// Keterlambatan (EPIC-077 R1): the warning level a count of late days in one
// attendance period falls under, per the company rules of 02-11-2026. The
// level is information for the employee and their manager; HR still issues
// the actual warning letters.

// LatenessLevel is one rung of the late-day ladder; Max nil is open-ended.
type LatenessLevel struct {
	Code  string `json:"code"`
	Label string `json:"label"`
	Range string `json:"range"`
	Min   int    `json:"min"`
	Max   *int   `json:"max"`
}

func intPtr(v int) *int { return &v }

// LatenessLevels is the ladder in order. The rules say "0 – 2x" for a
// reprimand; zero late days reads as none here (R1).
var LatenessLevels = []LatenessLevel{
	{"none", "Tidak ada keterlambatan", "0 kali", 0, intPtr(0)},
	{"teguran", "Teguran lisan dan tertulis", "1–2 kali", 1, intPtr(2)},
	{"sp1", "Surat Peringatan 1", "3–4 kali", 3, intPtr(4)},
	{"sp2", "Surat Peringatan 2", "5–6 kali", 5, intPtr(6)},
	{"sp3", "Surat Peringatan 3 dan/atau PHK", "Lebih dari 6 kali", 7, nil},
}

// LatenessLevelFor is the level of count late days (negative counts as 0).
func LatenessLevelFor(count int) LatenessLevel {
	level := LatenessLevels[0]
	for _, l := range LatenessLevels {
		if count >= l.Min {
			level = l
		}
	}
	return level
}

// NextLatenessLevel is the level after count's and how many more late days
// reach it; ok is false on the last level.
func NextLatenessLevel(count int) (level LatenessLevel, remaining int, ok bool) {
	count = max(count, 0)
	for _, l := range LatenessLevels {
		if l.Min > count {
			return l, l.Min - count, true
		}
	}
	return LatenessLevel{}, 0, false
}

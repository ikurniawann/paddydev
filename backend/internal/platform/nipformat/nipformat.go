// Package nipformat is the NIP rule of EPIC-099: YYYYMM of the join date
// followed by the next number of the employee's series (3 digits, 4 from
// 1000 on). HR still types the NIP; this only says whether it is right.
//
// Two series exist in the data. The main series is one running number since
// 2016 that never restarts. The harian series (Daily Worker, Freelance,
// Magang) restarts at 001 every year and stays at or below the highest
// number used before that year (its cap), so the two never mix.
package nipformat

import (
	"fmt"
	"regexp"
	"strconv"
)

type Series string

const (
	Main   Series = "main"
	Harian Series = "harian"
)

const harianLabel = "Daily Worker/Freelance/Magang"

// SeriesFor is the series of a new employee by its starting status.
func SeriesFor(status string) Series {
	switch status {
	case "daily_worker", "freelance", "intern":
		return Harian
	}
	return Main
}

var pattern = regexp.MustCompile(`^(\d{4})(0[1-9]|1[0-2])(\d{3}|[1-9]\d{3})$`)

// Parse splits a NIP of the format into YYYYMM and its number.
func Parse(nip string) (ym string, seq int, ok bool) {
	m := pattern.FindStringSubmatch(nip)
	if m == nil {
		return "", 0, false
	}
	seq, _ = strconv.Atoi(m[3])
	if seq == 0 {
		return "", 0, false
	}
	return m[1] + m[2], seq, true
}

// Last holds the latest numbers for a join year. HarianCap 0 = no cap (no
// NIP of an earlier year exists).
type Last struct{ Main, Harian, HarianCap int }

// LastFor computes Last for the given year from every NIP held; NIPs not
// in the format are ignored.
func LastFor(nips []string, year int) Last {
	type parsed struct{ year, seq int }
	var ps []parsed
	var l Last
	for _, n := range nips {
		ym, seq, ok := Parse(n)
		if !ok {
			continue
		}
		y, _ := strconv.Atoi(ym[:4])
		ps = append(ps, parsed{y, seq})
		l.Main = max(l.Main, seq)
		if y < year {
			l.HarianCap = max(l.HarianCap, seq)
		}
	}
	for _, p := range ps {
		if p.year == year && (l.HarianCap == 0 || p.seq <= l.HarianCap) {
			l.Harian = max(l.Harian, p.seq)
		}
	}
	return l
}

// yearMonth is YYYYMM of a YYYY-MM-DD date ("" when too short).
func yearMonth(joinDate string) string {
	if len(joinDate) < 7 {
		return ""
	}
	return joinDate[:4] + joinDate[5:7]
}

func last(s Series, l Last) int {
	if s == Harian {
		return l.Harian
	}
	return l.Main
}

// Next is the NIP that follows the latest number of the series.
func Next(joinDate string, s Series, l Last) string {
	return fmt.Sprintf("%s%03d", yearMonth(joinDate), last(s, l)+1)
}

// CheckNew is the message refusing nip for a new employee ("" = accepted).
func CheckNew(nip, joinDate, status string, l Last) string {
	if msg := CheckChanged(nip, joinDate); msg != "" {
		return msg
	}
	_, seq, _ := Parse(nip)
	s := SeriesFor(status)
	next := Next(joinDate, s, l)
	if s == Main {
		if seq <= l.Main {
			return fmt.Sprintf("NIP %s: nomor urut terakhir karyawan %03d — pakai nomor berikutnya, mis. %s", nip, l.Main, next)
		}
		return ""
	}
	year := joinDate[:4]
	if seq <= l.Harian {
		return fmt.Sprintf("NIP %s: nomor urut terakhir %s %s adalah %03d — pakai nomor berikutnya, mis. %s", nip, harianLabel, year, l.Harian, next)
	}
	if l.HarianCap > 0 && seq > l.HarianCap {
		return fmt.Sprintf("NIP %s: nomor urut %s %s maksimal %03d — mis. %s", nip, harianLabel, year, l.HarianCap, next)
	}
	return ""
}

// CheckChanged checks only the format and the YYYYMM of the join date: a
// NIP replaced on edit may correct a typo, so its number is not checked.
func CheckChanged(nip, joinDate string) string {
	ym, _, ok := Parse(nip)
	want := yearMonth(joinDate)
	if !ok {
		return "NIP harus angka: 6 digit tahun-bulan join date + 3 digit nomor urut, mis. 202611677"
	}
	if ym != want {
		return fmt.Sprintf("NIP %s: 6 digit depan harus %s (tahun-bulan join date)", nip, want)
	}
	return ""
}

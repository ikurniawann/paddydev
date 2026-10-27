package domain

import "testing"

// EPIC-068 R1: jatah cuti tahunan default. A = join + 12 bulan; A setelah
// 31-12-Y -> 0; A pada/ sebelum 01-01-Y -> 12; selain itu prorata hari,
// dibulatkan ke bawah ke kelipatan 0,5.
func TestDefaultAnnualLeave(t *testing.T) {
	cases := []struct {
		name string
		join *string
		year int
		want float64
	}{
		{"contoh brief: join 2025-03-10 untuk 2026", sp("2025-03-10"), 2026, 9.5},
		{"join 2026-02-01 belum genap 12 bulan di 2026", sp("2026-02-01"), 2026, 0},
		{"join 2024-12-31 sudah lewat 12 bulan", sp("2024-12-31"), 2026, 12},
		{"join lama", sp("2020-05-10"), 2026, 12},
		{"join NULL", nil, 2026, 12},
		{"join kosong", sp(""), 2026, 12},
		{"join tidak valid", sp("bukan-tanggal"), 2026, 12},
		{"A tepat 01-01-Y", sp("2025-01-01"), 2026, 12},
		{"A 02-01-Y: 364/365 hari", sp("2025-01-02"), 2026, 11.5},
		{"A tepat 31-12-Y: 1 hari", sp("2025-12-31"), 2026, 0},
		{"join 31-12-Y-1 + 12 bulan = 31-12-Y", sp("2025-12-31"), 2026, 0},
		{"join 01-01-Y: A tahun depan", sp("2026-01-01"), 2026, 0},
		{"join 31-12-Y", sp("2026-12-31"), 2026, 0},
		{"kabisat 29-02: A klem ke 28-02", sp("2024-02-29"), 2025, 10},
		{"kabisat 29-02: tahun berikutnya penuh", sp("2024-02-29"), 2026, 12},
		{"tahun kabisat 366 hari", sp("2027-07-01"), 2028, 6},
		{"tahun sebelum join", sp("2026-03-01"), 2025, 0},
		{"timestamp penuh", sp("2025-03-10T00:00:00Z"), 2026, 9.5},
	}
	for _, c := range cases {
		if got := DefaultAnnualLeave(c.join, c.year); got != c.want {
			t.Errorf("%s: DefaultAnnualLeave(%v, %d) = %v, want %v", c.name, c.join, c.year, got, c.want)
		}
	}
}

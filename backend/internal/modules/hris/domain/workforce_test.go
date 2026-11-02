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
		{"2028 memakai prorata bulan (EPIC-084): Agu-Des", sp("2027-07-01"), 2028, 5},
		{"tahun sebelum join", sp("2026-03-01"), 2025, 0},
		{"timestamp penuh", sp("2025-03-10T00:00:00Z"), 2026, 9.5},
	}
	for _, c := range cases {
		if got := DefaultAnnualLeave(c.join, c.year, nil); got != c.want {
			t.Errorf("%s: DefaultAnnualLeave(%v, %d) = %v, want %v", c.name, c.join, c.year, got, c.want)
		}
	}
}

// EPIC-084 R7: mulai 2027 prorata = bulan sesudah bulan genap 1 tahun,
// dikurangi cuti bersama di bulan-bulan itu; genap 1 tahun di Januari = 12.
func TestDefaultAnnualLeaveMonthly2027(t *testing.T) {
	cases := []struct {
		name string
		join *string
		year int
		cb   []string
		want float64
	}{
		{"contoh user: join 10-05-2026", sp("2026-05-10"), 2027, nil, 7},
		{"cuti bersama di bulan genap tidak dihitung", sp("2026-05-10"), 2027, []string{"2027-05-20", "2027-06-15"}, 6},
		{"cuti bersama tahun lain tidak dihitung", sp("2026-05-10"), 2027, []string{"2026-06-15", "2028-06-15"}, 7},
		{"genap di Januari penuh", sp("2026-01-15"), 2027, []string{"2027-04-10"}, 12},
		{"genap di Desember", sp("2026-12-01"), 2027, nil, 0},
		{"sudah lewat 1 tahun", sp("2025-03-01"), 2027, []string{"2027-04-10"}, 12},
		{"lantai 0", sp("2026-11-10"), 2027, []string{"2027-12-24", "2027-12-25", "2027-12-26"}, 0},
		{"belum genap di 2027", sp("2027-02-01"), 2027, nil, 0},
		{"2026 tetap rumus lama", sp("2025-05-10"), 2026, []string{"2026-06-15"}, 7.5},
		{"join kosong", nil, 2027, []string{"2027-06-15"}, 12},
		{"tanggal cuti bersama ganda dihitung sekali", sp("2026-05-10"), 2027, []string{"2027-06-15", "2027-06-15"}, 6},
	}
	for _, c := range cases {
		if got := DefaultAnnualLeave(c.join, c.year, c.cb); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

# Cross-check potongan Izin Pribadi dan Sakit, periode 21 Okt – 20 Nov 2026 (EPIC-117)

Disiapkan 18-11-2026 untuk tes payroll Paddy hari Senin. Data production dibaca dengan sesi read-only; tidak ada yang diubah.

## Aturan vs kode

| Aturan (acuan MOM 19-10) | Kode | Tes yang mengunci |
|---|---|---|
| Izin Pribadi memotong 1 hari THP 4 komponen (pokok + makan + transport + jabatan) per hari | Sesuai (`calculator.go`, PIP) | `TestPersonalLeaveDeductsOneThpDay`, `TestPersonalLeaveNoDoubleDeduction` |
| Intern tidak dipotong Izin Pribadi | Sesuai | `TestInternPersonalLeaveNotDeducted` |
| Sakit memotong (transport + makan) per hari | Sesuai (PIS, hanya bagian flat) | `TestSickDeductsTransportAndMeal`, `TestSickAndMaternityDeductFlatAllowancesOnly` |
| Hari izin/sakit yang tetap punya absen tidak dipotong | Sesuai | `TestLeaveDayWithAttendanceNotDeducted` |
| Sakit ≥ 2 hari wajib surat (K-3) | Ditegakkan di pengajuan (lampiran), bukan di payroll | — |
| Izin Pribadi maks. 3 hari/bulan | Kuota di modul cuti | — |
| Cuti Melahirkan memotong (transport + makan) | **Berbeda.** Kode: full paid bila masa kerja ≥ 1 tahun; < 1 tahun dihitung Izin Pribadi (ES-16, EPIC-084 R8) | — |

Contoh angka (pokok 4.000.000, makan 500.000, transport 500.000, jabatan 300.000, 22 hari kerja):
- 1 hari Izin Pribadi = 5.300.000 / 22 = **Rp 240.909**
- 2 hari Sakit = 1.000.000 / 22 × 2 = **Rp 90.909**

## Setting production

`hris.payroll_settings`: `personal_leave_deduction = true`, `sick_leave_deduction = true`. Kedua potongan aktif.

## Data periode di production

| Jenis | Disetujui | Pending | Ditolak |
|---|---|---|---|
| Tahunan | 51 (47 karyawan) | 2 | 3 |
| Sakit | 39 (31 karyawan) | 4 | 1 |
| Izin Pribadi | 17 (17 karyawan) | 1 | 0 |
| Izin Leader | 4 | 0 | 0 |
| Duka | 2 | 0 | 0 |

## Yang perlu HR tindak sebelum payroll dihitung

1. **7 pengajuan masih pending** (2 tahunan, 4 sakit, 1 izin pribadi). Payroll hanya membaca cuti yang disetujui, jadi putuskan ketujuhnya sebelum run dihitung. Kalau tidak, hari itu terhitung tidak hadir.
2. **4 dari 5 sakit ≥ 2 hari** sudah punya surat, tapi suratnya belum diverifikasi HR. Potongan payroll tidak bergantung pada verifikasi; ini kelengkapan K-3 saja.
3. Tidak ada karyawan dengan Izin Pribadi lebih dari 3 hari, dan tidak ada cuti melahirkan di periode ini, jadi selisih aturan melahirkan tidak memengaruhi payroll November.
4. Absen yang masuk ke staging sesudah 10-11 belum ada di production. Rencana EPIC-117 Task 8 memindahkannya sebelum payroll dihitung.

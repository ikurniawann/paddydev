# EPIC-124: Slip Izin Sakit & Izin Pribadi Sesuai Hitungan HR, Tunjangan Penuh (`MODULE-PAYROLL`)

status: review
environment: dev
retries: 0

## Goal
Slip gaji menampilkan Tunjangan Transport dan Makan penuh, lalu memotongnya lewat baris Izin Sakit dan Izin Pribadi, persis seperti sheet manual HR. Sebelumnya tunjangan `per_day` "hilang" lewat hari hadir, sehingga potongan di slip lebih kecil dari hitungan HR dan HR bertanya dari mana angka 78.261.

Rencana: [2026-11-20-slip-izin-sakit-pribadi-tunjangan-penuh.md](../superpowers/plans/2026-11-20-slip-izin-sakit-pribadi-tunjangan-penuh.md).

Kasus acuan: Shafa Triya Auliya, November 2026. Gaji pokok 1.800.000, makan 600.000, transport 600.000 (keduanya `per_day`), 23 hari terjadwal, hadir 20, sakit 2, Izin Pribadi 1. Hitungan HR: Izin Sakit 104.348, Izin Pribadi 130.435, THP 2.765.217. Slip lama: transport dan makan masing-masing 521.739, Izin Pribadi 78.261, THP 2.765.217.

## Keputusan user (20-11-2026)
| No | Pertanyaan | Keputusan |
| --- | --- | --- |
| Izin Sakit | Apa yang dipotong | Hanya Tunjangan Transport + Makan per hari. Kutipan: "sakit motong tunjangan transport dan makan aja" |
| Izin Pribadi | Apa yang dipotong | 1 hari THP (Gaji Pokok + Jabatan + Transport + Makan). Subsidi BPJS tidak dipotong. Kutipan: "jika ada izin pribadi yang dipotong dari THP tp untuk subsidi BPJS tidak dipotong ya" |
| K1 | Intern yang Izin Pribadi | Dibayar penuh, termasuk transport + makan hari itu. Kutipan: "Yes, pay in full" |
| Ruling R2 | Batas THP | Berubah hanya karena pembulatan (≤ Rp3); hasil golden: selisih 0 |

## Temuan
- Hari sakit dan hari Izin Pribadi tidak dihitung hadir, jadi transport/makan `per_day` ikut berkurang di baris tunjangan. Baris potongan hanya mengambil bagian komponen `flat`.
- Akibatnya baris Izin Pribadi (78.261 = 1.800.000 / 23) tampak jauh di bawah hitungan manual (130.435), padahal THP akhir sama.

## Perubahan
- Hari sakit dan hari Izin Pribadi yang dipotong dihitung sebagai hari tunjangan untuk komponen `per_day`; baris Izin Sakit = (Transport + Makan) per hari × hari sakit; baris Izin Pribadi = 1 hari dari 4 komponen untuk semua mode.
- Subsidi BPJS tidak dipotong dan tidak ikut hari tunjangan. Cuti Melahirkan tetap full paid.
- Bruto slip naik sebesar pergeseran tunjangan. Kolom baru `leave_allowance_shift` (per slip dan per alokasi toko) menyimpan angkanya. Dasar PPh21 (fallback YTD TER Desember) dan jurnal `PAYROLL_ACCRUAL` (perusahaan dan per toko) memakai bruto dikurangi pergeseran, jadi pajak dan jurnal sama dengan sebelumnya.

## Tasks
- Task 1: kalkulator (erp-paddy `d658cf3`).
- Task 1b (ruling R1): persist `leave_allowance_shift`, YTD Desember dan jurnal akrual memakai bruto − pergeseran (erp-paddy `49e460e`).
- Task 2: golden 17 kasus dan bukti invarian (erp-paddy `035f95b`).
- Task 3: dokumen acuan dan epic ini (repo paddydev).

## Acceptance Criteria
- Slip Shafa: Transport 600.000, Makan 600.000, Potongan Izin Sakit 104.348, Potongan Izin Pribadi 130.435, THP 2.765.217 (sama dengan sheet manual HR).
- Izin Sakit hanya memotong transport + makan; Subsidi BPJS tidak terpotong oleh Izin Sakit maupun Izin Pribadi.
- PPh21, `TaxableIncome`, DPP TER, dan BPJS sama persis dengan sebelum perubahan.
- THP berubah hanya karena pembulatan (≤ Rp3), kecuali intern K1 (+ transport + makan untuk hari Izin Pribadi).
- Pengaturan `sick_leave_deduction` / `personal_leave_deduction` off: hari itu tidak menjadi hari tunjangan berbayar.
- Mode `flat` tidak berubah. Jurnal `PAYROLL_ACCRUAL` sama dengan sebelumnya.

## Automation Log
- 20-11-2026: keputusan Izin Sakit, Izin Pribadi, Subsidi BPJS, dan K1 dikonfirmasi user (kutipan di tabel keputusan). Rencana ditulis.
- 20-11-2026: implementasi di erp-paddy branch `fix/epic124-slip-izin-tunjangan-penuh`: Task 1 `d658cf3` (kalkulator), Task 1b `49e460e` (kolom `leave_allowance_shift` di `hris.payroll_details` dan `hris.payroll_detail_allocations`, migrasi `20261221090000_payroll_leave_allowance_shift.sql`), Task 2 `035f95b` (golden). Ruling R1: review Task 1 menemukan YTD TER Desember dan jurnal `PAYROLL_ACCRUAL` ikut naik oleh pergeseran, maka Task 1b menyimpan pergeseran dan keduanya memakai bruto − pergeseran. Review tiap task: Approved, tanpa Critical/Important tersisa.
- 20-11-2026: invarian golden (17 kasus; 6 baru: TER, annualized, campuran, intern, `daily`, setting off) dibandingkan dengan kalkulator `d6d90ec`: PPh21, TaxableIncome, DPP, BPJS TK dan Kes identik di semua kasus; bruto baru − pergeseran = bruto lama; Δnet 0 di semua kasus kecuali intern K1 (+52.174 = transport + makan sehari); 11 kasus lama hanya mendapat kunci baru bernilai 0. Ruling R2: batas dokumen ≤ Rp3 (tiga pembulatan terpisah), hasil teramati 0.
- 20-11-2026: PR ke development: (menyusul). Sebelum PR, rebase ke `origin/development` (sudah di 778d0bd, EPIC-123; merge-tree bersih) lalu jalankan ulang tes payroll dan golden. Catatan minor yang ditunda: komentar usang di `golden_test.go`, tes integrasi acuan belum menegaskan net/PPh, pergeseran multi-toko non-nol baru diuji di domain.
- Langkah rilis (setelah PR disetujui):
  1. Merge PR ke `development`.
  2. Gladi di salinan production. Migrasi `20261221090000_payroll_leave_allowance_shift.sql` jalan bersama deploy (aditif, default 0).
  3. Setelah rilis, HR menghitung ulang run payroll November yang masih draft supaya slip menampilkan pembagian baru. Run final/dibayar tidak diubah: pergeseran tersimpannya 0, jadi YTD pajak dan jurnal mereka tetap.
  4. QA HR membandingkan slip Shafa Triya Auliya dengan sheet manual (Izin Sakit 104.348, Izin Pribadi 130.435, THP 2.765.217).
  Rilis digabung dengan EPIC-118 s.d. 123.

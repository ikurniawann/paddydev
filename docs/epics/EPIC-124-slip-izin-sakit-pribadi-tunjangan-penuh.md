# EPIC-124: Slip Izin Sakit & Izin Pribadi Sesuai Hitungan HR, Tunjangan Penuh (`MODULE-PAYROLL`)

status: ready-for-qa
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
- Bruto di slip, total run, dan kolom "Gaji kotor" per toko memakai bruto baru (termasuk pergeseran). Jurnal akrual, YTD Desember, dan "Biaya tenaga kerja" per toko (`labor_cost` = bruto − pergeseran + beban perusahaan) tidak memuat pergeseran, jadi tetap cocok dengan buku. Akibatnya di laporan "Biaya Gaji per Toko", bruto + beban perusahaan lebih besar dari biaya tenaga kerja sebesar pergeseran (ada catatan kaki di UI).

## Tasks
- Task 1: kalkulator (erp-paddy `5c24eab`).
- Task 1b (ruling R1): persist `leave_allowance_shift`, YTD Desember dan jurnal akrual memakai bruto − pergeseran (erp-paddy `17d598d`).
- Task 2: golden 17 kasus dan bukti invarian (erp-paddy `d96014a`).
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
- 20-11-2026: implementasi di erp-paddy branch `fix/epic124-slip-izin-tunjangan-penuh`: Task 1 `5c24eab` (kalkulator), Task 1b `17d598d` (kolom `leave_allowance_shift` di `hris.payroll_details` dan `hris.payroll_detail_allocations`, migrasi `20261221090000_payroll_leave_allowance_shift.sql`), Task 2 `d96014a` (golden). Ruling R1: review Task 1 menemukan YTD TER Desember dan jurnal `PAYROLL_ACCRUAL` ikut naik oleh pergeseran, maka Task 1b menyimpan pergeseran dan keduanya memakai bruto − pergeseran. Review tiap task: Approved, tanpa Critical/Important tersisa.
- 20-11-2026: invarian golden (17 kasus; 6 baru: TER, annualized, campuran, intern, `daily`, setting off) dibandingkan dengan kalkulator `d6d90ec`: PPh21, TaxableIncome, DPP, BPJS TK dan Kes identik di semua kasus; bruto baru − pergeseran = bruto lama; Δnet 0 di semua kasus kecuali intern K1 (+52.174 = transport + makan sehari); 11 kasus lama hanya mendapat kunci baru bernilai 0. Ruling R2: batas dokumen ≤ Rp3 (tiga pembulatan terpisah), hasil teramati 0.
- 20-11-2026: PR ke development: [erp-paddy#37](https://github.com/wit-id-indonesia/erp-paddy/pull/37) (fix wave review akhir `0d0b105` ikut di PR ini). Branch sudah di-rebase ke `origin/development` 778d0bd (EPIC-123); hash commit di atas adalah hash setelah rebase, tes payroll dan akuntansi lulus tanpa DB. Catatan minor yang ditunda: komentar usang di `golden_test.go`, tes integrasi acuan belum menegaskan net/PPh, pergeseran multi-toko non-nol baru diuji di domain.
- 20-11-2026 20.55 WIB: atas perintah user ("merge PR #37 lalu deploy ke staging"), PR #37 di-merge ke `development` (8fb548b). Backup `/mnt/hdd1/backup/paddydev/staging-db-before-epic124-20261120-205459.dump` (mode 600), deploy staging image `local-20261120-205512` (sebelumnya `local-20261120-183733`), 1 migrasi (`20261221090000_payroll_leave_allowance_shift.sql`), verify 12.395 objek 0 selisih, `/api/ready` 200; kolom `leave_allowance_shift` ada di kedua tabel. Run payroll November di staging masih `draft` dan belum dihitung ulang: QA HR menghitung ulang run itu lalu membandingkan slip Shafa Triya Auliya dengan sheet manual. Production belum; rilis berikutnya butuh gladi ulang.
- 20-11-2026: Review akhir seluruh branch: tanpa Critical, siap merge dengan perbaikan. Diperbaiki: (1) `labor_cost` per toko (laporan Biaya Gaji per Toko, ekspor CSV/XLSX, ringkasan toko di halaman run) = bruto − pergeseran + beban perusahaan, kolom bruto tetap bruto slip, catatan kaki di UI, tes integrasi menegaskan angkanya; (2) teks bantuan pengaturan Izin Sakit/Izin Pribadi di UI memakai aturan baru; (3) komentar usang `golden_test.go`; (4) dokumen: kata-kata baris Izin Sakit (tanpa pembagian ganda), baris 19/20 menyebut intern (K1), langkah rilis menambah urutan migrasi dan catatan rollback. Minor yang ditunda: tes integrasi acuan belum menegaskan net/PPh (sudah dijaga tes domain, golden, dan uji properti 200.000 kasus, selisih net maks Rp2); pergeseran multi-toko non-nol baru diuji di domain; gaya COALESCE di `branchCosts`. `allowance_days` tetap menyimpan hari hadir saja.
- Langkah rilis (setelah PR disetujui):
  1. Merge PR ke `development`.
  2. Gladi di salinan production. Migrasi `20261221090000_payroll_leave_allowance_shift.sql` jalan bersama deploy (aditif, default 0). Migrasi harus diterapkan sebelum API baru dijalankan: `tools/deploy-paddydev.sh` sudah berurutan begitu; pada deploy manual jalankan migrasi dulu, karena API baru tanpa kolom itu gagal (500) saat hitung, saat tandai dibayar, dan saat YTD Desember.
  Rollback: jika biner lama dipulihkan, hitung ulang run draft sebelum ditandai dibayar. Slip yang dihitung biner baru membawa bruto lebih besar, yang akan dibukukan biner lama ke jurnal akrual dan fallback YTD.
  3. Setelah rilis, HR menghitung ulang run payroll November yang masih draft supaya slip menampilkan pembagian baru. Run final/dibayar tidak diubah: pergeseran tersimpannya 0, jadi YTD pajak dan jurnal mereka tetap.
  4. QA HR membandingkan slip Shafa Triya Auliya dengan sheet manual (Izin Sakit 104.348, Izin Pribadi 130.435, THP 2.765.217).
  Rilis digabung dengan EPIC-118 s.d. 123.

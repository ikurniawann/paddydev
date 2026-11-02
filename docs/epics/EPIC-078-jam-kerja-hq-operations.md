# EPIC-078: Jam Kerja HQ & Operations untuk Penilaian Terlambat (`MODULE-HRIS`)

status: ready-for-qa
environment: dev
retries: 0

## Goal
Hitungan terlambat di EPIC-077 harus sejalan dengan jam kerja perusahaan. Ketentuan dari user (02-11-2026), dicek sebelum PR #62 di-merge:

> **HARI DAN JAM KERJA**
> Hari dan Waktu Kerja Karyawan Headquarter (mencakup seluruh staff kantor pusat, baik fungsi inti maupun fungsi pendukung, termasuk namun tidak terbatas pada departemen Marketing, Product Research dan Development, Retail Operation, Finance & Accounting, HRGA, Support dan SCM).
> - Hari Senin – Jumat
> - Jam Masuk kerja: 08.00 – 09.00 WIB
> - Waktu istirahat: 12.00 – 13.00 WIB
> - Jam Keluar kerja: 17.00 - 18.00 WIB
>
> Hari dan Waktu Kerja Karyawan Operations (mencakup seluruh staff Warehouse).
> - Hari Senin – Jumat: 08.00 – 16.00 WIB
> - Waktu Istirahat: 12.00 - 13.00 WIB
> - Hari Sabtu: 08.00 – 14.00 WIB

Selisih yang ditemukan di data staging (salinan data karyawan asli; akses baca production ditolak izin):
- Semua Non-Shift memakai jadwal tetap NS 09.00–18.00 dengan **toleransi 15 menit**. Akibatnya staf HQ baru dianggap terlambat setelah 09.15, padahal batas masuk 09.00.
- **56 staf Warehouse** juga memakai NS 09.00–18.00 dan libur Sabtu. Datang 09.10 tidak terlambat, dan Sabtu tidak dinilai sama sekali.
- **48 staf HQ bertipe Shift tanpa jadwal**: Finance 13, HRGA 8, Product RnD 6, Marketing 5, Support 4, Branding 1, Eksekutif 1, dan 10 tanpa departemen. Keterlambatan mereka tidak pernah dihitung.

Jawaban user 02-11:
- **Warehouse toleransi 10 menit**, jadi terlambat mulai 08.11. HQ terlambat mulai 09.01.
- **Penerapan otomatis saat deploy**, mulai tanggal deploy.
- Absensi yang sudah tercatat **tidak dihitung ulang**.

## Ruling
- **R1 HQ**: shift `NS` tetap 09.00–18.00, istirahat 60 menit, toleransi **0**, nama "Non-Shift HQ (masuk 08.00–09.00)".
  - Masuk 08.00–09.00 tidak terlambat. Lewat 09.00 terlambat, dan menitnya dihitung dari 09.00.
  - Pulang fleksibel 17.00–18.00 bukan bagian penilaian terlambat. Itu ranah jam minimum kerja EPIC-065: `min_work_hours` cabang HQ = 9 masih keputusan HR.
- **R2 Operations**: dua shift bawaan baru.
  - `OPS`: 08.00–16.00, istirahat 60, toleransi 10.
  - `OPSSAT`: 08.00–14.00, istirahat 0, toleransi 10. Istirahat Sabtu tidak disebut TnC, jadi tidak dipotong (my call).
- **R3 Jadwal tetap menurut departemen**:
  - Non-Shift di departemen **Warehouse** mendapat Sen–Jum `OPS`, Sab `OPSSAT`, Min libur.
  - Departemen lain mendapat Sen–Jum `NS`, Sab–Min libur.
  - Departemen dicocokkan dari namanya ("Warehouse"), tanpa membedakan huruf besar-kecil dan spasi tepi. Mengganti nama departemen Warehouse akan mematikan aturan ini, jadi namanya jangan diganti tanpa mengubah fungsi.
  - Satu sumber aturan: fungsi SQL `hris.apply_fixed_week(employee, from, actor)`. Fungsi ini dipanggil Go (`applyFixedPattern`, `schedule_type.go`) di semua jalur: Ubah tipe jadwal (satu/massal), Terapkan jadwal tetap per cabang, impor Excel, dan aktifkan kembali. Label pola di audit `schedule.fixed_apply` ikut jenisnya.
- **R4 Bawaan terkunci**: `DEF`, `NS`, `OPS` dan `OPSSAT` tidak bisa diubah kodenya, dihapus, atau dinonaktifkan (Go `builtinShift`, UI `isProtectedShiftCode`).
- **R5 Penerapan sekali saat deploy**: migrasi `20261102090000_hris_work_hours_hq_operations.sql` memanggil `hris.apply_work_hours_rule(hari ini WIB, 'Sistem (aturan jam kerja EPIC-078)')`.
  - Sasarannya karyawan aktif (`hris.work_hours_rule_targets`, bisa dipakai sebagai pratinjau baca-saja) yang:
    - Non-Shift di Warehouse; atau
    - bertipe Shift **tanpa jadwal apa pun mulai tanggal itu**, di departemen mana pun kecuali Retail Operational (kru toko). Jabatan pekerja shift dikecualikan: pengecualian go-live 21-10 (Shopkeeper, Pasaraya, Crew Experience, Store Supervisor, Retail Operation Manager), event, dan Customer Service (memakai shift CS). Akun sistem juga dikecualikan.
  - Kelompok kedua dijadikan Non-Shift.
  - Untuk tiap sasaran:
    - jadwal per tanggal ≥ tanggal itu dihapus;
    - pola mingguan lama ditutup sehari sebelumnya;
    - pola tetap baru disimpan;
    - audit `employee.schedule_type` (bila tipe berubah) dan `schedule.fixed_apply` dicatat.
  - Yang tidak disentuh:
    - staf HQ Non-Shift yang sudah NS (cukup perubahan toleransi);
    - staf yang sudah punya jadwal;
    - kru toko;
    - karyawan tanpa departemen;
    - karyawan nonaktif.
- **R6 Riwayat tetap**: `is_late` dan `late_minutes` absensi yang sudah tercatat tidak dihitung ulang. Penilaian baru berlaku untuk clock-in setelah deploy.
- **R7 ESS**: panel lipat di kartu Keterlambatan menjadi "Jam Kerja & Ketentuan Keterlambatan". Isinya teks Hari dan Jam Kerja HQ & Operations apa adanya (`lib/hris/work-hours.ts`), di atas ketentuan keterlambatan.
- **R9 Pindah departemen**: trigger `employees_department_fixed_week` berjalan saat departemen karyawan Non-Shift aktif berubah dan karyawan itu masuk atau keluar Warehouse. Jadwal tetapnya diganti mulai hari ini (WIB) dan diaudit dengan alasan "Pindah departemen (EPIC-078)". Trigger ini berlaku di semua jalur (profil, Excel, akun). Pindah antar-departemen HQ tidak mengubah apa pun. Karyawan Shift tidak disentuh.
- **R10 Toleransi tersimpan**: kolom baru `hris.attendance.late_tolerance_minutes` diisi saat clock-in dan saat HR menambah absen yang memakai jadwal hari itu.
  - Koreksi HR atas jam masuk memakai toleransi yang tersimpan.
  - Baris lama (NULL) memakai toleransi shift sekarang.
  - Migrasi membekukan baris NS yang sudah ada di 15 menit (termasuk periode yang sudah closing, lewat `hris.allow_closed_period_write`). Dengan begitu koreksi absen Oktober tidak tiba-tiba menjadi terlambat.
- **R8 Teks HR**: konfirmasi ubah tipe jadwal, dialog Terapkan jadwal tetap, dan tab Shift karyawan kini menyebut jadwal per departemen (`FIXED_SCHEDULE_SUMMARY`), tidak lagi "Senin–Jumat 09.00–18.00".

## Tasks
- [x] Migrasi: NS toleransi 0 + nama; shift OPS/OPSSAT; fungsi `hris.apply_work_hours_rule` + panggilan sekali.
- [x] Go: `fixedPattern` per departemen, `applyFixedPattern` (label audit), `builtinShift`.
- [x] Frontend: `work-hours.ts`, panel ESS, teks jadwal tetap, kode bawaan terkunci.
- [x] Tes Go: `TestWorkHoursShifts` (batas 09.00 / 08.10, Sabtu), `TestFixedScheduleByDepartment`, `TestApplyWorkHoursRule` (sasaran, pengecualian, penutupan pola lama, audit). Tes kode bawaan diperbarui.
- [x] Perbaikan review: pengecualian jabatan, toleransi tersimpan (`attendance_clock.go`, `attendance_adjust.go`), trigger pindah departemen, upsert OPS/OPSSAT, nama NS hanya diganti bila masih bawaan, baris jam kerja tidak terpotong di HP. Tes: `TestDepartmentMoveSwitchesFixedWeek`, `TestAdjustKeepsClockInTolerance` (gagal bila toleransi tersimpan diabaikan), `TestClockInStoresTolerance`, pengecualian Customer Service di `TestApplyWorkHoursRule`.
- [x] Tes UI: panel jam kerja (`my-lateness-card.test.tsx`), teks konfirmasi (`schedule-type.test.ts`), grup Bawaan (`shift-list.test.ts`).

## Acceptance Criteria
- [x] Staf HQ masuk 09.00 tidak terlambat; 09.00:30 terlambat 1 menit; 09.25 terlambat 25 menit.
- [x] Staf Warehouse Sen–Jum masuk 08.10 tidak terlambat, 08.11 terlambat 11 menit; Sabtu dinilai terhadap 08.00–14.00.
- [x] Setelah deploy: Warehouse memakai pola Operations; staf HQ tanpa jadwal memakai pola HQ; kru toko dan karyawan tanpa departemen tidak berubah; riwayat absensi tidak berubah.
- [x] Karyawan bisa membaca jam kerja HQ & Operations di ESS Absensi (HP dan laptop).

## Dampak yang perlu diketahui HR
- Warehouse kini bekerja hari Sabtu. Pembagi payroll per hari (`ScheduledDays`) dan hari kerja untuk absen/cuti ikut menghitung Sabtu, sesuai TnC. Cabang Warehouse Paddy belum punya jam minimum (`min_work_hours` kosong), jadi Sabtu 6 jam tidak diberi label kurang jam.
- ESS Absensi menampilkan "toleransi 0 mnt" untuk HQ, karena batas masuknya memang 09.00.
- 10 karyawan tanpa departemen tetap tanpa jadwal. Mereka perlu diberi departemen/jadwal oleh HR.

## Automation Log
- 2026-11-02 — **Gladi pertama di staging** (transaksi `ROLLBACK`): 94 karyawan.
- 2026-11-02 — **Review independen** (Opus): tidak ada yang rusak. Temuan yang diperbaiki:
  1. Pengecualian go-live 21-10 belum ikut.
  2. Koreksi HR atas absen lama memakai toleransi baru.
  3. Pindah departemen tidak mengganti pola.
  4. OPS/OPSSAT yang sudah ada dipakai apa adanya.
  5. Baris jam kerja di HP terpotong.

  Dampak Sabtu dicatat di atas.
- 2026-11-02 — **Gladi ulang setelah perbaikan** (staging, `ROLLBACK`): 92 karyawan asli diterapkan, ditambah 2 akun demo QA078.
  - Warehouse 56 → Operations.
  - HQ 36 Shift → Non-Shift HQ: Finance 13, HRGA 8, Product RnD 6, Support 4, Marketing 3, Branding 1, Eksekutif 1.
  - 2 Customer Service Marketing dikecualikan.
  - 157 absen NS dibekukan di toleransi 15.
- 2026-11-02 — **Gates lokal hijau.**
  - Go: `go vet ./...` bersih; semua paket `./internal/...` lulus.
  - Frontend: tsc 0, lint bersih, vitest 878 file / 6352 tes lulus.
- 2026-11-02 — **Staging** (branch `34354e9e`, lalu `1cfd74de`).
  - Backup: `~/Desktop/paddydev-backups/staging-db-before-epic078-20261102-055832.dump`.
  - Migrasi `20261102090000` diterapkan; verifikasi skema 10.733 objek, 0 selisih.
  - Hasil di staging sama dengan gladi: 94 `schedule.fixed_apply`, 38 `employee.schedule_type` (36 karyawan asli + 2 demo), 157 absen NS dibekukan.
  - Image web `local-20261102-070547`, API `local-20261102-061231`. Rollback staging = image `local-20261102-010235` (EPIC-077).
- 2026-11-02 — **Insiden disk host penuh (100%)** saat build pertama. Build web selesai, tetapi skrip berhenti di "no space left on device"; container lama tetap jalan dan staging tetap sehat.
  - Dibebaskan: build cache Docker (cache saja) dan image staging paddydev lama. Image rollback 010235 dan 220036 disimpan.
  - Deploy dilanjutkan dengan langkah skrip yang sama memakai image yang sudah jadi.
  - Sisa ruang ±8 GB. Sebagian besar disk dipakai image Docker project lain; itu keputusan user.
- 2026-11-02 — **QA staging dengan data demo QA078**: staf HQ, staf Warehouse, kru toko, akun HR. Hasil 35/35 di HP 360 px dan laptop: 0 error console, 0 API ≥ 400, tidak ada luberan horizontal.
  - Halaman pertama setelah login = Beranda ESS: kartu Absensi Hari Ini (masuk 09.25 → terlambat 25 mnt) dan kartu Keterlambatan (1x → Teguran).
  - Panel Jam Kerja HQ & Operations; nilai jam tidak terpotong di HP.
  - Master Shift menampilkan OPS/OPSSAT dan NS "HQ"; profil Non-Shift menyebut jadwal per departemen.
  - Migrasi: Gudang → OPS/OPSSAT, Kantor → NS, Kru Toko tetap Shift.
  - **Bug ditemukan dan diperbaiki**: nama di kartu sapaan Beranda HP terjepit menjadi "G / F". Badge status dan NIP kini turun ke baris sendiri (`beranda-page.tsx`, commit `1cfd74de`).
  - Tangkapan layar ada di `.share/qa-epic078/`.
  - Data demo dihapus: 0 sisa akun, karyawan, absen dan audit; 0 notifikasi QA078.
- **Langkah production (perlu izin user)**: merge PR #62, lalu gladi migrasi di salinan DB production (lihat daftar sasaran lewat `hris.work_hours_rule_targets`), lalu deploy jam sepi dan verifikasi baca-saja. QA akun asli oleh HR.
- 2026-11-02 14.31 WIB — **LIVE production** (main `cfff8811`, PR #62).
  - Image `local-20261102-143126`; backup `~/backups/prod-db-before-main-e078-20261102-143015.dump`.
  - Migrasi `20261102090000` diterapkan; skema 10.733 objek, 0 selisih.
  - Hasil: 88 `schedule.fixed_apply` (Warehouse → OPS/OPSSAT, HQ → NS), 36 Shift → Non-Shift; sama dengan gladi 07.21. 609 absen NS dibekukan.
  - 0 error API; `/login` 200.
  - Rollback: image `local-20261101-221140` + restore backup.
  - Sisa: QA akun asli oleh HR.


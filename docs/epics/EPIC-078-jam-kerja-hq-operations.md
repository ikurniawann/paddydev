# EPIC-078: Jam Kerja HQ & Operations untuk Penilaian Terlambat (`MODULE-HRIS`)

status: review
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
  - Departemen dicocokkan dari namanya, tanpa membedakan huruf besar-kecil (`operationsDepartment`, `schedule_type.go`).
  - Ini dipakai di semua jalur: Ubah tipe jadwal (satu/massal), Terapkan jadwal tetap per cabang, impor Excel, dan aktifkan kembali. Label pola di audit `schedule.fixed_apply` ikut jenisnya.
- **R4 Bawaan terkunci**: `DEF`, `NS`, `OPS` dan `OPSSAT` tidak bisa diubah kodenya, dihapus, atau dinonaktifkan (Go `builtinShift`, UI `isProtectedShiftCode`).
- **R5 Penerapan sekali saat deploy**: migrasi `20261102090000_hris_work_hours_hq_operations.sql` memanggil `hris.apply_work_hours_rule(hari ini WIB, 'Sistem (aturan jam kerja EPIC-078)')`.
  - Sasarannya karyawan aktif yang:
    - Non-Shift di Warehouse; atau
    - bertipe Shift **tanpa jadwal apa pun mulai tanggal itu**, di departemen mana pun kecuali Retail Operational (kru toko).
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
- **R8 Teks HR**: konfirmasi ubah tipe jadwal, dialog Terapkan jadwal tetap, dan tab Shift karyawan kini menyebut jadwal per departemen (`FIXED_SCHEDULE_SUMMARY`), tidak lagi "Senin–Jumat 09.00–18.00".

## Tasks
- [x] Migrasi: NS toleransi 0 + nama; shift OPS/OPSSAT; fungsi `hris.apply_work_hours_rule` + panggilan sekali.
- [x] Go: `fixedPattern` per departemen, `applyFixedPattern` (label audit), `builtinShift`.
- [x] Frontend: `work-hours.ts`, panel ESS, teks jadwal tetap, kode bawaan terkunci.
- [x] Tes Go: `TestWorkHoursShifts` (batas 09.00 / 08.10, Sabtu), `TestFixedScheduleByDepartment`, `TestApplyWorkHoursRule` (sasaran, pengecualian, penutupan pola lama, audit). Tes kode bawaan diperbarui.
- [x] Tes UI: panel jam kerja (`my-lateness-card.test.tsx`), teks konfirmasi (`schedule-type.test.ts`), grup Bawaan (`shift-list.test.ts`).

## Acceptance Criteria
- [x] Staf HQ masuk 09.00 tidak terlambat; 09.00:30 terlambat 1 menit; 09.25 terlambat 25 menit.
- [x] Staf Warehouse Sen–Jum masuk 08.10 tidak terlambat, 08.11 terlambat 11 menit; Sabtu dinilai terhadap 08.00–14.00.
- [x] Setelah deploy: Warehouse memakai pola Operations; staf HQ tanpa jadwal memakai pola HQ; kru toko dan karyawan tanpa departemen tidak berubah; riwayat absensi tidak berubah.
- [x] Karyawan bisa membaca jam kerja HQ & Operations di ESS Absensi (HP dan laptop).

## Automation Log
- 2026-11-02 — **Gladi migrasi di staging** (transaksi `ROLLBACK`): 94 karyawan diterapkan.
  - Warehouse 56 dipindah ke Operations.
  - HQ 38 Shift → Non-Shift: Finance 13, HRGA 8, Product RnD 6, Marketing 5, Support 4, Branding 1, Eksekutif 1.
  - Shift NS/OPS/OPSSAT sesuai R1–R2.
  - Sisa tanpa jadwal: 10 karyawan tanpa departemen (perlu HR) dan kru toko Retail Operational (ranah Jadwal Toko, EPIC-070/071).

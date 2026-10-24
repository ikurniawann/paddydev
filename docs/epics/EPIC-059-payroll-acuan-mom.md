# EPIC-059: Payroll Sesuai Acuan Payslip & MOM 19-10-2026

status: ready-for-qa
environment: dev
retries: 0

## Goal
Kalkulator payroll Go mengikuti tabel payslip Paddy dan keputusan MOM 19-10-2026, yang diputuskan user 20-10-2026 ("ikuti semua rekomendasi"). Cakupan: hari dibayar vs hari efektif, pembagi = hari kerja terjadwal, "1 hari THP" = 4 komponen tanpa potong ganda, Sakit & Cuti Melahirkan memotong transport+makan, minimum lembur 1 jam, subsidi BPJS Kesehatan Rp100.000 untuk masa kerja > 1 tahun, dan prorata karyawan yang keluar di tengah periode. Requirement: PY-01, PY-03, PY-04, ES-06.

Acuan: `docs/hris/acuan-mom-20261019/` (aturan-payroll.md, pertanyaan-rekomendasi.md).

## Tasks
Lihat `docs/superpowers/plans/2026-10-20-payroll-acuan-mom.md`.

## Acceptance Criteria
- Gaji pokok (`monthly_prorated`) dan Tunjangan Jabatan per hari dibayar menurut **hari kerja aktif yang dibayar**: hadir + cuti dibayar. Transport & makan per hari menurut **hari hadir**.
- Pembagi harian = hari kerja terjadwal karyawan dalam periode. Pembagi struktur gaji hanya dipakai bila jadwal tidak ada.
- Potongan Izin Pribadi = 1 hari dari 4 komponen (Gaji Pokok, Transport, Makan, Jabatan) tanpa memotong dua kali komponen yang sudah tidak dibayar lewat hari hadir. Intern tidak dipotong.
- Sakit dan Cuti Melahirkan memotong transport & makan per hari: lewat hari hadir bila mode `per_day`, lewat baris potongan bila `flat`.
- Lembur di bawah 1 jam ditolak saat pengajuan dan tidak dibayar.
- Subsidi BPJS Kesehatan dapat diatur global: nominal (Rp100.000) dan syarat masa kerja > 1 tahun pada tanggal cutoff.
- Karyawan yang `end_date`-nya jatuh di dalam periode tetap ikut payroll run; hanya karyawan **nonaktif** (leaver) yang dibayar prorata sampai `end_date`, karyawan aktif dengan `end_date` di periode dibayar penuh.
- Tidak ada perubahan setting atau data live tanpa persetujuan user. Skrip ops perpindahan `pay_basis` dan mode tunjangan tersedia dengan mode pratinjau.

## Automation Log
- 2026-10-20: epic dibuat. Keputusan user: "Ikuti semua rekomendasi di pertanyaan-rekomendasi.md (K-12, K-10, Q-PY-1..4, K-6/Q-KP-1..2, Q-APV-1..3, K-13, K-3, K-14, K-9, Q-AB-1). K-7 tetap menunggu Paddy." Prasyarat: PR ikurniawann/paddydev#3 (intern tanpa PIP) di-merge dulu.
- 2026-10-20: implementasi selesai (Task 1-7) dan diverifikasi (Task 8); status `review`. Branch `feat/epic059-payroll-acuan` (bertumpuk di atas EPIC-060/PR #4 dan merge `main` berisi PR #3, jadi PR #4 di-merge dulu).
- 2026-10-20: gelombang fix review akhir (TDD, `go test ./internal/modules/payroll/... ./internal/app` hijau): (1) Critical — karyawan keluar lewat alur Resign (jadwal `effective_to` = `end_date`) tidak lagi dibayar sebulan penuh: ukuran periode penuh (pembagi & penyebut faktor) menganggap baris jadwal yang berakhir tepat pada `end_date` tetap berlaku sampai akhir periode, cakupan tetap berhenti di `end_date` (`TestLeaverResignFlowScheduleTruncated`); (2) `Calculate` menjepit pokok `monthly_prorated` <= pokok x faktor serta jabatan `per_day` <= jabatan x faktor (`TestPaidDaysCappedWithoutSchedule`; batas hari dibayar tanpa jadwal di input kemudian dihapus, lihat B1 di bawah); (3) clip `end_date` hanya untuk karyawan **nonaktif**, `Employee.IsActive` diisi dari `EmployeeBrief` (`TestActiveEmployeeEndDateNotClipped`, `TestPayrollRunEndDateClipsLeaversOnly`); (4) Izin Pribadi/Sakit/Melahirkan pada tanggal yang punya absensi efektif tidak dipotong PIP/PIS (`TestLeaveOnAttendedDateNotDeducted`); (5) pratinjau skrip Task 7 menampilkan daftar tiap struktur dan karyawan tanpa jadwal, diuji di DB test saja (pratinjau + terapkan di dalam `BEGIN...ROLLBACK`, `UPDATE 24`, md5 `hris.employee_salary` sebelum/sesudah sama). Laporan: `.superpowers/sdd/2026-10-20-payroll-acuan-mom/final-fix-report.md`.
- 2026-10-20: B1 diperbaiki (keputusan user 20-10-2026: hapus batas hari dibayar tanpa jadwal; clamp <=100% di Calculate). Sebelumnya `PaidDays` tanpa jadwal dibatasi hari kerja Senin-Jumat sehingga pekerja 6 hari (pembagi struktur 26/25) kurang bayar ~15% pada pokok `monthly_prorated` dan jabatan `per_day`. Test: `TestSixDayWorkerWithoutScheduleNotCapped` (26 hari Senin-Sabtu: pokok 2.600.000, jabatan 260.000; RED 2.200.000/220.000 sebelum fix); `TestPaidDaysCappedWithoutSchedule` kini menegaskan uang tepat 100% lewat clamp Calculate. Laporan: `.superpowers/sdd/2026-10-20-payroll-acuan-mom/b1-fix-report.md`.

### Commit
| Task | Commit | Isi |
|---|---|---|
| 1 | `7a176b6` | hari dibayar (`PaidDays`) dan hari cuti per jenis dihitung pada hari kerja (`IsWorkDate`) |
| 2 | `0f3a0d3` | pembagi harian = hari kerja terjadwal periode (K-12) |
| 3 | `0e590ed`, `9d7f0e5` | "1 hari THP" = 4 komponen tanpa potongan ganda; Sakit & Cuti Melahirkan memotong transport+makan `flat`; fix telat tier basis THP untuk upah harian |
| 4 | `03557a4` | lembur minimal 1 jam (tolak pengajuan di hris, tidak dibayar di payroll) |
| 5 | `2af0248` | setting global `bpjs_kes_subsidy_amount` (subsidi BPJS Kes masa kerja > 1 tahun); migrasi `20261021110000_payroll_bpjs_kes_subsidy.sql` |
| 6 | `f8968b2` | karyawan keluar di tengah periode ikut run, dibayar prorata sampai `end_date` (port baru `PayrollEmployees`) |
| 7 | `f0013b7` | skrip ops `backend/database/sql/fixes/2026-10-21_payroll_struktur_gaji_acuan.sql` (pratinjau/terapkan struktur gaji ke acuan; **belum dijalankan**) |
| 8 | `40863b2`, `2768f63` | verifikasi menyeluruh, test simulasi `TestPayrollRunAcuanPayslip`, pembaruan dokumen |

### Ruling
- R1 hari efektif = absensi berstatus bukan `absent` (dasar Transport & Makan `per_day`).
- R2 hari dibayar (`PaidDays`) = hari efektif + cuti approved selain `unpaid` pada hari kerja tanpa absensi efektif, dibatasi hari kerja terjadwal; dasar Gaji Pokok `monthly_prorated` dan Jabatan `per_day`. Tanggal `unpaid` tidak pernah dibayar.
- R3 hari kerja = hari ber-shift menurut jadwal yang berlaku; tanpa jadwal: Senin-Jumat bukan libur nasional. Semua hitungan hari cuti payroll memakai hari kerja.
- R4 pembagi (K-12) `monthly`/`monthly_prorated` = hari kerja terjadwal periode; tanpa jadwal pakai `working_days_divisor` struktur, lalu hari kerja hasil hitung. `daily` tidak berubah.
- R5 "1 hari THP" = Gaji Pokok + Transport + Makan + Jabatan (nominal struktur), tanpa lembur/bonus/insentif/THR.
- R6 Izin Pribadi tanpa potongan ganda: PIP = (Gaji Pokok [kecuali `daily`] + Jabatan + Transport `flat` + Makan `flat`) / pembagi x hari; yang `per_day` sudah hilang lewat hari efektif. Intern tidak dipotong.
- R7 Sakit & Cuti Melahirkan: PIS hanya memotong Transport/Makan `flat`; saklar `sick_leave_deduction` mengatur keduanya; slip berlabel "Potongan Sakit & Cuti Melahirkan".
- R8 potongan telat tier basis `thp` memakai THP 4 komponen.
- R9 lembur < 1 jam ditolak saat pengajuan dan tidak dibayar di payroll.
- R10 subsidi BPJS Kes global: bila `bpjs_kes_subsidy_amount` > 0 menggantikan `health_allowance` struktur, syarat masa kerja > 1 tahun pada akhir periode; bila 0 perilaku lama.
- R11 karyawan keluar: run mengambil aktif + nonaktif yang `end_date`-nya di dalam periode; hanya yang **nonaktif** (leaver) diprorata sampai `end_date`, yang aktif dibayar penuh. PPh21 Pasal 17 masa pajak terakhir tidak termasuk (follow-up).
- R12 data live tidak disentuh; perpindahan struktur gaji dan `cutoff_day = 20` hanya setelah persetujuan user.

Ruling pengendali selama eksekusi:
- Migrasi Task 5 memakai timestamp `20261021110000` (bukan `20261021090000`) agar berurutan setelah migrasi kasbon `20261021100000`.
- `Employees.Active` tidak diubah (dipakai performance.go untuk karyawan aktif saja); payroll run memakai port baru `PayrollEmployees(ctx, q, start, end)`.
- Potongan telat tier basis `thp` untuk `pay_basis = daily`: 1 hari = upah harian + (transport + makan + jabatan) / pembagi.
- Skema zod Next lama (`lib/payroll/settings.ts`) tidak diubah karena route payroll-settings dilayani Go; bila fallback Next aktif, key subsidi terbuang saat PUT (follow-up).
- Branch dibuat dari EPIC-060 + merge `main` (PR #3) karena ports.go dan fake Employees diubah kedua epic.

### Bukti test (20-10-2026, DB test `127.0.0.1:55499`)
- `go test ./... -count=1`: 27 nama test gagal, **identik** dengan baseline `main` (kegagalan lama di 13 paket: configuration/master, configuration/settings, crm/engagement, crm/partners, gymcredits, gymscheduling, gymtraining, hris, salesfunnel, site, spa, storedvalue, platform/auth; `diff` daftar nama kosong, tidak ada nama baru). Paket `payroll` dan `payroll/domain` hijau.
- `frontend`: `npx vitest run` 761 file lulus (2 skip), 5535 test lulus, 10 skip, 0 gagal; `npx tsc --noEmit -p .` tanpa output.
- Simulasi angka `TestPayrollRunAcuanPayslip` (`backend/internal/modules/payroll/acuan_integration_test.go`, test integrasi permanen lewat API `/calculate`): struktur `monthly_prorated` + `per_day`, pokok 2.200.000, transport 220.000, makan 220.000, jabatan 440.000, jadwal Senin-Jumat, Juni 2026 (22 hari kerja), hadir 20 hari + 1 Izin Pribadi + 1 Sakit. Hasil: Gaji Pokok 2.200.000 (= /22 x 22), Transport 200.000 dan Makan 200.000 (x 20/22), Jabatan 440.000, Potongan Izin Pribadi 120.000 ((2.200.000 + 440.000) / 22), Potongan Sakit 0, bruto 3.040.000.

### Catatan rilis
- Migrasi baru `20261021110000_payroll_bpjs_kes_subsidy.sql` (kolom setting `bpjs_kes_subsidy_amount`, default 0 = perilaku lama). Berlaku setelah migrasi kasbon `20261021100000`.
- **Skrip ops Task 7 dan `cutoff_day = 20` BELUM dijalankan di live** dan menunggu persetujuan user setelah melihat pratinjau/simulasi.
- Yang **langsung berubah setelah rilis, juga untuk struktur `monthly` live** (240 struktur, tanpa skrip): pembagi per hari = hari kerja terjadwal periode bila karyawan punya jadwal (K-12); basis Potongan Izin Pribadi = 4 komponen (pokok + jabatan + transport/makan `flat`) tanpa potongan ganda; Potongan Sakit hanya transport/makan `flat` dan kini termasuk Cuti Melahirkan; potongan telat tier basis `thp` = THP 4 komponen; hari cuti dihitung pada hari kerja saja (Izin Pribadi/Sakit/Melahirkan pada tanggal yang punya absensi efektif tidak dipotong); tunjangan jabatan `per_day` mengikuti hari dibayar; label slip "Potongan Sakit & Cuti Melahirkan".
- Yang **baru berlaku setelah skrip Task 7 dijalankan**: gaji pokok `monthly_prorated` menurut hari dibayar, serta tunjangan transport, makan dan jabatan `per_day`.
- Lembur < 1 jam yang sudah approved di periode yang masih terbuka **tidak akan dibayar lagi** setelah rilis.
- Pembagi per hari untuk karyawan bulanan berjadwal berubah dari pembagi struktur (mis. 26/25) menjadi jumlah hari kerja terjadwal periode.
- Potongan telat tier basis THP untuk 18 struktur `daily` live berubah (upah harian + tunjangan, sebelumnya basis keliru ~96 ribu per hari terlalu kecil).
- Karyawan yang `end_date`-nya jatuh di dalam periode kini ikut payroll run; yang nonaktif (leaver) dibayar prorata sampai `end_date`, yang aktif dibayar penuh.

### Minor yang ditunda (tidak memblokir)
- `ClampedLeaveDays`/`LeaveRange` tidak lagi punya pemanggil produksi (hanya test).
- Tanggal hadir sekaligus cuti `unpaid` tetap masuk `PaidDays`; "unpaid menang" hanya terhadap cuti (sesuai R2, sama dengan perilaku lama).
- Peta libur payroll memuat cuti bersama yang memotong cuti (hris `DeductsLeave`); tanpa jadwal, hari itu bukan hari kerja payroll.
- Belum ada test pembagi untuk kontrak parsial (`DivisorDays` tetap 22) dan `daily` + jadwal + pembagi struktur 26.
- Test selisih neto bersifat rapuh bila default PPh/BPJS `parityInput` berubah; belum ada test transport `per_day` + makan `flat` campuran, PIP `daily`, PIP karyawan prorata.
- Teks bantuan PIP menyebut "hari kerja terjadwal" padahal ada fallback pembagi struktur.
- Lembur: test hris hanya jalur pengajuan sendiri (bukan penugasan HR); test payroll hanya bucket reguler.
- Subsidi BPJS Kes masih dikali faktor prorata kontrak (konsisten dengan tunjangan lain); tanya user bila ingin flat. Belum ada test join 29 Feb, setting 0 + senior < masa minimum, dan hasil `Calculate` 100.000.
- Karyawan dengan semua kontrak dimulai setelah `end_date`: cakupan kosong sehingga dibayar penuh (sama dengan perilaku lama tanpa overlap). Belum ada test cabang clip dengan kontrak, `ed == end`, dan `end_date` NULL nonaktif.
- Skrip ops Task 7: jendela peringatan 30 hari bergulir (bukan periode payroll); mode apply tanpa `ON_ERROR_STOP` bisa commit setelah gagal; header tidak menyebut psql >= 13.

### Follow-up (di luar epic ini)
- PPh21 Pasal 17 tahunan pada masa pajak terakhir karyawan yang keluar di tengah tahun (sekarang hanya Desember).
- Baris slip terpisah untuk Insentif dan Denda (sekarang tergabung di "Penghasilan Lain" / "Potongan Lain").
- Payroll mingguan daily worker (PY-05): periode dan run baru.
- Menjalankan skrip Task 7 dan mengubah `cutoff_day = 20` di live: keputusan user setelah pratinjau dan simulasi.
- 2026-10-21 07:14 WIB: **Deploy live** main `5035ae5` (PR #4–#8), image `local-20261021-071158`. Backup sebelum deploy: `~/Desktop/paddydev-backups/paddydev-db-before-epic059-062-deploy-20261021-071142.dump`. 9 migrasi diterapkan (20261020150000 s.d. 20261023090100), cek skema 0 berbeda; `SELECT iam.sync_hris_staff_roles()` dijalankan (grant aktif admin_hris 57, hris_manager 57, hris_non_payroll 52, hris_non_rekrutmen_kpi 40; hak hapus daun HRIS hanya hris_manager = 18). Container sehat tanpa restart; /login, /api/ready 200. Skrip ops struktur gaji, `cutoff_day = 20`, dan `bpjs_kes_subsidy_amount` **belum** dijalankan (tetap keputusan user).
- 2026-10-21 23:00 WIB: **Fix pembagi jadwal tengah periode** (branch `fix/payroll-pembagi-jadwal`, belum deploy). Simulasi `cutoff_day = 20` di salinan DB live: 103 karyawan punya jadwal (`v_employee_schedule`) yang baru berlaku 2026-10-20, sehingga pembagi = hari terjadwal sejak 20 Okt saja (1 hari pada periode 1–20 Okt, 9 pada 1–31 Okt) sedangkan hari hadir sebelumnya ikut dikali → transport/makan per hari dan potongan per hari melonjak (cutoff 20: tunjangan ±×13). Keputusan: pola jadwal pertama dianggap berlaku sejak awal periode untuk pembagi, hari kerja, faktor cakupan kontrak, dan hari kerja cuti (simetris dengan aturan resign yang memperpanjang jadwal ke akhir periode); ini juga memprorata karyawan baru yang jadwalnya mulai di tanggal masuk. Bukti: test `TestScheduleStartingMidPeriodMeasuresWholePeriod`, `TestNewJoinerScheduleFromJoinDateProrated`; verifikasi salinan DB (cutoff 20, run Okt): 85 karyawan berjadwal → pembagi 14, 0 karyawan tunjangan > 100%, tunjangan 713,2 → 85,4 jt; 178 karyawan tanpa jadwal tidak berubah. Lembur hari istirahat (`overtime_rest_from_schedule`) sebelum jadwal mulai tidak diubah.
- 2026-10-22 13:12 WIB: **Deploy live fix pembagi jadwal tengah periode.** PR [ikurniawann/paddydev#15](https://github.com/ikurniawann/paddydev/pull/15) di-merge ke main (`71fc616`); CI hanya gagal di `TestOrderAlerts` dan `TestResendingSameRoleKeepsIamRoles` (baseline main), Unit test, Lint, dan Audit lulus. Backup `~/Desktop/paddydev-backups/paddydev-db-before-payroll-divisor-deploy-20261022-131123.dump` (6,5 MB, 442 tabel). `tools/deploy-paddydev.sh` dari worktree main terpisah: tanpa migrasi baru, verify skema 10.600 objek 0 selisih; image web & API `local-20261022-131131` sehat tanpa restart (rollback: `local-20261021-223048`). Cek pasca-deploy: `/login`, `/api/ready` 200; `/api/hris/payroll`, `/api/hris/attendance-settings` 401 tanpa sesi; log 10 menit tanpa error. Setting live tidak diubah (`cutoff_day = 0`); payroll run yang sudah dihitung sebelumnya perlu **Hitung ulang** agar memakai pembagi baru.

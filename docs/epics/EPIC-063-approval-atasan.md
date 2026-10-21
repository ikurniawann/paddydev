# EPIC-063: Paket Approval Atasan — Persetujuan Tim, Lembur Sebelum Dilaksanakan, Jenis Weekday/Weekend, Koreksi Lembur AdminHRIS

status: ready-for-qa
environment: dev
retries: 0

## Goal
Approval time-off dan lembur mengikuti MOM 19-10-2026 (paket eksekusi langkah 5 "Paket Approval atasan" di `pertanyaan-rekomendasi.md`). Cakupan:
- **AP-01 Time-off:** cuti/izin diputus atasan langsung (`hris.employees.reporting_to`); SPV ke atasannya. Sesuai Q-APV-3, atasan langsung adalah approver utama, HR menjadi cadangan (karyawan tanpa atasan, atasan berhalangan) dan tetap bisa override. Setiap keputusan mencatat kapasitas pemutus (`manager` / `hr`), nama pemutus, dan baris audit.
- **AP-02 Lembur sebelum dilaksanakan:** pengajuan lembur milik sendiri ditolak bila jam mulainya (tanggal + jam mulai, jam WIB) sudah lewat. Approver mengikuti pola Q-APV-3 yang sama.
- **AP-03 Jenis lembur Weekday / Weekend:** K-13 diputuskan: Weekday 1,5×, Weekend/libur 2×, tarif tetap dari setting payroll. ESS hanya memilih jenis. Jenis dipetakan ke `day_type` yang sudah ada (`regular` = Weekday, `rest_day` = Weekend / Libur), dengan saran dari jadwal karyawan dan hari libur pada tanggal itu.
- **AP-04 Koreksi lembur oleh AdminHRIS:** endpoint dan UI untuk mengoreksi tanggal, jam, dan jenis pengajuan lembur. Dijaga aksi `update` pada menu daun `hris.kepegawaian.overtime` (konvensi EPIC-061), dengan audit sebelum/sesudah dan alasan.
- **ESS "Persetujuan Tim":** satu halaman antrean untuk bawahan langsung atasan yang memuat cuti, lembur, dan Perjalanan Dinas (komponen antrean EPIC-062 dipakai ulang), ditambah kartu tautan ke antrean kasbon di halaman Pinjaman. Badge jumlah pending memakai pola `nav-badges` yang sudah ada.
- **Guard:** tidak ada yang memutus pengajuannya sendiri, termasuk HR dan kasus data live `reporting_to` yang menunjuk diri sendiri.
- **Kualitas data (kode saja):** menolak penyimpanan karyawan yang atasannya dirinya sendiri (form karyawan, Manajemen User, import Excel), serta laporan baca-saja "karyawan aktif tanpa atasan yang valid" di halaman Karyawan.
- **Notifikasi:** WA ke atasan langsung saat pengajuan cuti (sudah ada) dan lembur (belum ada) lewat jalur outbox + `wa_notif_log` yang sama. Tidak ada kanal baru.

Di luar cakupan: AP-06 (approval CEO/GM, menunggu K-7 dari Paddy) dan perbaikan data live (`reporting_to` 55 karyawan kosong dan 1 yang menunjuk diri sendiri tetap keputusan user).

Acuan: `docs/hris/acuan-mom-20261019/` — `status-requirement.md` (AP-01..AP-04), `pertanyaan-rekomendasi.md` (Q-APV-3, K-13, urutan eksekusi langkah 5), `matriks-approval.md`.

## Tasks
Lihat `docs/superpowers/plans/2026-10-21-approval-atasan.md` (11 task):
1. Aturan approval & lembur (domain Go murni).
2. Migrasi + keputusan cuti: atasan dulu, HR cadangan tercatat, tanpa self-approval.
3. Lembur: wajib sebelum dimulai, jenis Weekday/Weekend, kapasitas pemutus, WA atasan.
4. Koreksi lembur AdminHRIS (`PATCH /api/hris/overtime/{id}`) + audit.
5. Antrean Persetujuan Tim + badge (Go).
6. Kualitas data `reporting_to`: tolak atasan = diri sendiri, laporan karyawan tanpa atasan (Go).
7. Paritas fallback Next untuk route yang melayani path yang sama.
8. Halaman ESS "Persetujuan Tim".
9. Form lembur ESS (jenis + jam lewat) dan koreksi lembur di HRIS.
10. Kartu "Tanpa atasan" di Karyawan + form karyawan tanpa pilihan diri sendiri.
11. Verifikasi menyeluruh & dokumentasi.

## Acceptance Criteria
- Atasan langsung bisa menyetujui/menolak cuti dan lembur bawahan langsungnya dari ESS → Persetujuan Tim. Keputusan menyimpan `decided_as = 'manager'`.
- HR (`Actor.IsHRD`: `super_admin`, `admin`, `hrd`; ruling C1, `hiring_manager` **bukan** cadangan HR) tetap bisa memutus pengajuan siapa pun yang bukan miliknya. Keputusannya tersimpan `decided_as = 'hr'` beserta `decided_by_name`, dan `audit.audit_log` mendapat `leave.approve|reject` / `overtime.approve|reject`.
- Tidak ada yang memutus pengajuannya sendiri: HR menerima 403, dan karyawan dengan `reporting_to = id` sendiri juga menerima 403. Karyawan seperti itu tidak muncul di antrean timnya sendiri dan tidak menerima WA "atasan" untuk pengajuannya sendiri.
- Pengajuan lembur milik sendiri dengan tanggal + jam mulai (WIB) sebelum menit berjalan ditolak 400 `Lembur wajib diajukan sebelum dimulai — jam mulai sudah lewat`. Penugasan HR untuk karyawan lain boleh tanggal lampau.
- ESS memilih jenis lembur Weekday / Weekend / Libur. Bila jadwal atau hari libur menentukan jenisnya, pilihan dikunci ke saran itu dan server menolak jenis yang bertentangan. Tanpa jadwal yang berlaku, karyawan memilih sendiri (default Weekday). Tarif dan kalkulator payroll tidak berubah.
- AdminHRIS (aksi `update` di `hris.kepegawaian.overtime`) bisa mengoreksi tanggal, jam mulai/selesai, dan jenis lembur yang `pending`/`approved` milik orang lain. Jam dihitung ulang (1–12 jam), alasan wajib minimal 5 karakter, dan tanggal di periode absensi yang sudah closing ditolak. Audit `overtime.correct` menyimpan sebelum, sesudah, dan alasan.
- Badge menu "Persetujuan Tim" = cuti + lembur + Perjalanan Dinas pending bawahan langsung + kasbon yang menunggu tahap manager.
- WA atasan dikirim saat karyawan mengajukan lembur (bukan saat HR menugaskan), lewat outbox dan dedup `wa_notif_log`. Pesan cuti dan lembur mengarah ke "Area Karyawan → Persetujuan Tim".
- Menyimpan karyawan dengan atasan = dirinya sendiri ditolak 400 `Atasan langsung tidak boleh karyawan itu sendiri` (Go dan fallback Next). Import Excel menandai baris dengan error `NIP atasan tidak boleh NIP karyawan itu sendiri`.
- HR melihat daftar karyawan aktif tanpa atasan yang valid (kosong / diri sendiri / atasan nonaktif) di halaman Karyawan. Data live tidak diubah.

## Pertanyaan terbuka
Tidak ada. Q-APV-3 dan K-13 sudah diputuskan user pada 20-10-2026. Butir turunan yang tidak dijawab langsung oleh keduanya diputuskan sebagai ruling R1–R11 di rencana, mengikuti rekomendasi sesuai instruksi user untuk mengikuti rekomendasi.

## Catatan
- Kode `main` `5035ae5` (yang berjalan di live) dan DB test (port 55499) diperiksa hanya baca pada 21-10-2026:
  - `DecideLeave` (`backend/internal/modules/hris/leaves.go`) mengizinkan HR **atau** `reporting_to` dalam satu tahap dan **tidak** menolak self-approval: karyawan yang `reporting_to`-nya dirinya sendiri bisa menyetujui cutinya sendiri, begitu pula HR untuk cutinya sendiri. Lembur sudah menolak self-approval (`domain.CanDecideOvertime`).
  - `ListLeaves` non-HR hanya data sendiri, sehingga atasan tidak punya antrean cuti. `GET /api/hris/overtime?scope=approvals` ada, tetapi tanpa UI, dan `SubordinateIDs` ikut memasukkan diri sendiri bila `reporting_to = id`.
  - `CreateOvertime` menerima tanggal lampau; `day_type` hanya diisi HR.
  - Payroll (`payroll/domain/period.go` `SplitOvertimeWith`) memasukkan lembur ke tarif libur bila tanggalnya libur nasional, `day_type = 'rest_day'`, **atau** hari tanpa shift di jadwal yang berlaku. `regular` tidak pernah menurunkan tarif hari libur. Memetakan Weekend ke `rest_day` tidak mengubah bayaran lembur yang sudah benar menurut jadwal.
  - WA atasan hanya untuk cuti (`notifyLeaveRequest`, outbox). Pengajuan lembur tidak mengirim WA apa pun. Watcher Next `approvalMenginap` hanya rekap ke owner untuk pending > 2 hari.
  - Menu daun lembur adalah `hris.kepegawaian.overtime` (aksi menu `read, create, update, approve`). Role HR live (`hris_manager`, `hris_non_payroll`, `hris_non_rekrutmen_kpi`, `admin_hris`, `hrd`) sudah memegang `update`, jadi tidak perlu migrasi grant untuk koreksi.
  - Antrean kasbon manager ada di ESS → Pinjaman (EPIC-060), dan antrean Perjalanan Dinas atasan ada di ESS → Perjalanan Dinas (EPIC-062).
  - DB test: 35 dari 36 karyawan aktif tanpa `reporting_to`. Live (20-10): 55 dari 374, ditambah 1 yang menunjuk diri sendiri.
- Migrasi baru: `20261024090000_approval_atasan.sql` (setelah `20261023090100` EPIC-062).
- Rilis butuh `SELECT iam.sync_hris_staff_roles();` setelah migrasi, karena `admin_hris` mendapat menu `ess.team-approvals`.

## Automation Log
- 2026-10-20: User menerima rekomendasi `pertanyaan-rekomendasi.md`, termasuk **Q-APV-3** (atasan langsung approver utama; HR cadangan dan tetap bisa override, dengan audit log) dan **K-13** (Weekday 1,5×, Weekend/libur 2×; ESS cukup memilih jenis, tarif tetap dari setting).
- 2026-10-21: **Instruksi user: "lakukan paket approval atasan".** Epic dibuat dan langsung `on-progress`. Kode `main` `5035ae5` dan DB test (55499, hanya baca) diperiksa; temuan ada di Catatan. Rencana ditulis di `docs/superpowers/plans/2026-10-21-approval-atasan.md` (11 task, ruling R1–R11, tanpa pertanyaan terbuka). `status-requirement.md` AP-01..AP-04 kini menunjuk epic ini. Branch `feat/epic063-approval-atasan` dari `main` `5035ae5`.
- 2026-10-21: **Implementasi selesai (Task 1–11), status `review`.** Subagent-driven development, satu review per task. Commit di branch `feat/epic063-approval-atasan` (dari `main` `5035ae5`; dasar kerja `853d507`):

  | Task | Commit |
  |---|---|
  | 1 Aturan approval & lembur (domain) | `85be35e` |
  | 2 Migrasi + keputusan cuti | `cde1a89` |
  | 3 Lembur: sebelum dimulai, jenis, kapasitas pemutus, WA atasan | `03691b7` |
  | 4 Koreksi lembur AdminHRIS | `da9a5c8`, fix 1 `1254f2a` |
  | 5 Antrean Persetujuan Tim + badge (Go) | `f1cfdb5` |
  | 6 Kualitas data `reporting_to` | `9761c20` |
  | 7 Paritas fallback Next | `3b70825`, fix 1 `c2f81a9` |
  | 8 Halaman ESS Persetujuan Tim | `09dddc2` |
  | 9 Form lembur ESS + koreksi di HRIS | `6dc332f`, fix 1 `74baf7f` |
  | 10 Kartu "tanpa atasan" + form karyawan | `2fc8ecd` |
  | 11 Verifikasi & dokumentasi | commit docs ini |

- **Ruling yang dipakai.** R1–R11 rencana berlaku seperti ditulis, dengan perubahan dari pra-flight (9 selisih) dan review:
  - **C1 (controller)**: HR cadangan/override untuk cuti dan lembur = `Actor.IsHRD` (`super_admin`/`admin`/`hrd`), **bukan** `IsHR` (yang memuat `hiring_manager`). Mengubah R1 ("`Actor.IsHR`"); alasannya konsistensi dengan Perjalanan Dinas EPIC-062 dan mencegah jalur `decided_as = ''` yang melanggar CHECK. Live: 0 pemegang `hiring_manager`. Biaya bila salah: `hiring_manager` kehilangan cadangan HR untuk cuti/lembur.
  - **C2**: administrasi HR atas lembur (menugaskan lembur ke karyawan lain, tanggal lampau untuk mereka, membatalkan penugasan perusahaan milik orang lain) juga `IsHRD`. Pembacaan (daftar lembur, saran jenis) tetap `IsHR` (baca, bukan keputusan).
  - **P1** helper tes `leaveBody` memakai versi 3-argumen yang ada. **P2** C1 berlaku di semua jalur (`DecideLeave`, `CanDecideOvertime`, override `day_type` saat approve). **P3** `WorkforceActor.isHrd` di fallback Next. **P4** tes tambahan: `hiring_manager` memutus cuti/lembur → 403, HR memutus lembur sendiri → 403; teks AC dan rencana "hiring_manager tetap HR cadangan" diganti jadi "bukan cadangan HR". **P5** `lib/hris/reporting-to.ts` bebas impor server-only (pemindaian impor `"use client"`: tidak ada yang mencapai modul server lewat berkas Task 7; 5 temuan lama di ticketing tidak disentuh). **P6** tes `hris-a2-routes` memakai tanggal masa depan. **P7** badge Persetujuan Tim juga di `buildNavBadges` fallback Next. **P8** koreksi lembur menolak tanggal kalender tidak valid dengan 400. **P9** label "Hari istirahat" → "Weekend / Libur". **P10** handler memvalidasi HH:MM ketat sebelum `OvertimeStartsInPast` (yang fail-open) dan semua id dikanonikkan huruf kecil. **P11** form Manajemen User/Karyawan (satu form) tidak menawarkan diri sendiri sebagai atasan dan tidak mengirim ulang nilai diri sendiri (dikosongkan menjadi `null` saat disimpan).
  - R11 (atasan tidak mengubah jenis saat menyetujui): HR tetap bisa mengubah jenis di tombol Setujui (perilaku lama); fix Task 9 membuat baris tabel mengikuti nilai server setelah koreksi dan hanya mengirim `day_type` bila HR mengubahnya, supaya Setujui tidak menimpa koreksi.
  - Perubahan lain dari review: Task 4 fix 1 (koreksi memakai `periodContaining` agar penutupan periode otomatis berlaku untuk tanggal lama dan baru, snapshot `FOR UPDATE`); Task 7 fix 1 (id karyawan lembur dikanonikkan sebelum `isSelf`, label audit tanggal WIB, nama pemutus seragam); Task 8 (invalidasi antrean di `onSettled` agar baris basi hilang setelah 409).
- **Bukti test (21-10-2026, DB test 55499 saja).**
  - Go: `go test -p 1 ./... -count=1`: 27 test gagal, **nama identik dengan baseline** (`baseline.txt`, dibuat sebelum Task 1; tidak ada kegagalan baru; semuanya di luar HRIS approval kecuali `TestAttendancePeriodClosing` yang bergantung tanggal). `gofmt -l backend/internal` bersih. Test epic ini lulus: `TestDecideAs`, `TestOvertimeStartsInPast`, `TestOvertimeDayTypeFor`, `TestLeaveDecisionManagerFirstHRBackup`, `TestLeaveHiringManagerIsNotHRBackup`, `TestLeaveSelfManagerCannotApproveOwn`, `TestOvertimeMustBeRequestedBeforeStart`, `TestOvertimeDayTypeFollowsSchedule`, `TestOvertimeDecisionRecordsCapacityAndNotifiesManager`, `TestOvertimeCorrectionByAdminHRIS`, `TestTeamApprovalsQueueAndBadge`, `TestEmployeeCannotReportToSelf`.
  - Frontend: `npx vitest run`: 798 berkas lulus, 2 dilewati; 5804 test lulus, 10 dilewati, 0 gagal. `npx tsc --noEmit -p .` tanpa output.
  - Smoke API (pengganti cek browser, Go API lokal port 18463 terhadap DB test 55499, tiga akun uji SMOKE063 dihapus sesudahnya): lembur sendiri tanggal kemarin → 400 `Lembur wajib diajukan sebelum dimulai — jam mulai sudah lewat`; lembur besok diterima (`day_type = regular`); `GET /api/hris/me/team-approvals` atasan memuat cuti dan lembur bawahannya dan `GET /api/hris/nav-badges` mengembalikan `/dashboard/me/persetujuan-tim = 2`; atasan menyetujui cuti → `decided_as = manager`, `decided_by_name` terisi, audit `leave.approve`; pemohon memutus lembur sendiri → 403; tolak tanpa alasan → 400; HR menyetujui lembur → `decided_as = hr`, audit `overtime.approve`; HR `PATCH /api/hris/overtime/{id}` (jam selesai 19:00 → 20:00, 3,00 jam) → "Lembur dikoreksi", audit `overtime.correct`, alasan <5 karakter → 400, pemohon tanpa menu → 403; `GET /api/hris/employees/without-manager` HR → 200 (daftar), tanpa menu → 403.
  - **Tidak diverifikasi**: UI di browser (form lembur ESS, kunci jenis Weekend, halaman dan badge Persetujuan Tim, dialog Koreksi, badge "Dikoreksi"/"oleh HR", kartu "tanpa atasan", pilihan atasan tanpa diri sendiri) hanya lewat vitest, belum dilihat → **QA manusia sebelum rilis**; kunci jenis Weekend/Libur terhadap jadwal diuji di Go (`TestOvertimeDayTypeFollowsSchedule`), tidak di smoke; WA ke atasan hanya dengan pengirim palsu (tidak ada gateway uji); audit log halaman UI tidak dibuka. Catatan DB test: audit log append-only, jadi 3 baris audit smoke (`leave.approve`, `overtime.approve`, `overtime.correct`, aktor SMOKE063) tetap tertinggal di DB test.
- **Catatan minor yang diparkir** (tidak memblokir, bisa jadi follow-up kecil): kondisi balapan 409 dan beberapa percabangan (HRD yang juga atasan, `day_type` eksplisit HR, pembatalan lembur) belum punya test integrasi; `UpdateLeave` memberi `IsHR` (termasuk `hiring_manager`) hak membatalkan cuti pending siapa pun tanpa audit (pintu samping lama); laporan tanpa atasan diurutkan per alasan dengan batas 500 tanpa total; antrean Persetujuan Tim dibatasi 200 baris sedangkan badge tidak; hook keputusan Perjalanan Dinas tidak meng-invalidate badge; kegagalan muat antrean tampil sebagai kosong; `createLeave` Next tidak mengkanonikkan `employee_id`; tombol Koreksi tampil pada lembur HR sendiri (server 403).
- **Final review fix wave (21-10-2026).** `ae21e0a` override `day_type` HRD saat Setujui hanya untuk lembur `source='employee'` (HRD yang mengonfirmasi penugasan perusahaan untuk dirinya tidak bisa mengubah tarif sendiri) dan perubahan jenis tercatat di audit `overtime.approve` (`before.day_type`/`after.day_type`, hanya bila berubah); `6ff9b09` keputusan Perjalanan Dinas meng-invalidate `nav-badges` dan antrean di `onSettled`; `f1dcd66` Persetujuan Tim menampilkan pesan error saat antrean gagal dimuat (bukan keadaan kosong) dan tautan lampiran cuti, dengan `0ec7ae8` agar atasan langsung boleh membaca lampiran bawahannya (sebelumnya hanya pemilik dan HR); `c78e125` pilihan jenis hari di tabel lembur hanya untuk HRD; referensi AB-02 di `status-requirement.md` dikoreksi ke `routes.go:91-93`. Dua catatan parkir di atas (badge hook Perjalanan Dinas, kegagalan muat tampil kosong) sudah selesai. Fallback Next tidak berubah: skema keputusan lembur Next tidak menerima `day_type` (R10), dan route lampiran Next tetap hanya pemilik/HR karena path itu dilayani Go.
- **Langkah rilis** (jalankan hanya atas keputusan user; agent tidak mengubah DB live):
  1. Backup DB live, lalu terapkan migrasi `backend/database/migrations/deltas/20261024090000_approval_atasan.sql` (setelah EPIC-062 `20261023090100`).
  2. Jalankan `SELECT iam.sync_hris_staff_roles();` (`admin_hris` mendapat `ess.team-approvals`; role lain dengan `ess.team-shifts` sudah disalin migrasi).
  3. Build ulang API (Go) dan web (Next).
  4. Perbaikan `reporting_to` live (55 karyawan aktif kosong, 1 menunjuk diri sendiri) memakai kartu "tanpa atasan" yang baru, oleh HR atau atas persetujuan user. Agent tidak mengubah data live. Menyimpan form karyawan yang menunjuk diri sendiri akan mengosongkannya (P11).
  5. QA manusia atas daftar "Tidak diverifikasi" di atas.
- **Follow-up (di luar epic):** AP-06 (approval CEO/GM) menunggu K-7 dari Paddy. `hiring_manager` sengaja bukan cadangan HR untuk cuti/lembur (C1); bila peran rekrutmen perlu memutus, ubah di satu tempat (`Actor.IsHRD` → himpunan baru) setelah konfirmasi user. Keputusan HR atas cuti/lembur masih berbasis peran aplikasi (`IsHRD`), bukan aksi `approve` di menu daun; menyelaraskannya dengan konvensi EPIC-061 adalah perubahan hak akses tersendiri. Pengingat/eskalasi otomatis bila atasan tidak memutus dalam N hari belum ada (sekarang hanya rekap `approvalMenginap` > 2 hari ke owner). Fallback Next tidak memvalidasi jenis lembur terhadap jadwal (R10).
- 2026-10-21 17:58 WIB: **DEPLOY LIVE bersama EPIC-063 + EPIC-064.**
  - PR [ikurniawann/paddydev#10](https://github.com/ikurniawann/paddydev/pull/10) (EPIC-063) dan [#11](https://github.com/ikurniawann/paddydev/pull/11) (EPIC-064) di-merge ke main (`d57ece4`, `92a9a2c`). CI keduanya hanya gagal di `TestOrderAlerts` dan `TestResendingSameRoleKeepsIamRoles` (baseline main); Unit test, Lint, dan Audit dependensi lulus.
  - Backup `~/Desktop/paddydev-backups/paddydev-db-before-epic063-064-deploy-20261021-175506.dump` (6,5 MB, 442 tabel).
  - `tools/deploy-paddydev.sh`: migrasi `20261024090000_approval_atasan.sql` diterapkan (EPIC-064 tanpa migrasi); verify skema 10.600 objek, 0 hilang / 0 berbeda / 0 hanya di DB. Container web & API `local-20261021-175513` sehat (rollback: image `local-20261021-071158`, tag `paddydev:previous` / `paddydev-api:previous`).
  - `SELECT iam.sync_hris_staff_roles();`: grant admin_hris 57→58, hris_manager 57→58, hris_non_payroll 52→53, hris_non_rekrutmen_kpi 40→41, employee 13→14. Menu `ess.team-approvals` dipegang admin, admin_hris, employee, hris_manager, hris_non_payroll, hris_non_rekrutmen_kpi, marketing, sales, spa_frontdesk, spa_therapist, super_admin.
  - Cek pasca-deploy (tanpa login): `/api/ready`, `/login`, dan domain publik 200. `/api/hris/me/team-approvals`, `/api/hris/employees/without-manager`, `POST …/employees/{id}/rehire`, `PATCH /api/hris/overtime/{id}`, dan `/api/hris/overtime/day-type` menjawab 401 tanpa sesi. Log API & web 10 menit pertama tanpa error.
  - Data live `reporting_to` saat deploy: 342 karyawan aktif, 22 tanpa atasan, 2 menunjuk diri sendiri. Perbaikan oleh HR memakai kartu "Tanpa atasan" di halaman Karyawan.
  - QA manusia: halaman ESS Persetujuan Tim + badge, form lembur ESS (Weekday/Weekend, tolak jam lewat), koreksi lembur HRIS, kartu Tanpa atasan, tombol & dialog Aktifkan kembali, pesan bentrok KTP/NPWP/NIP.

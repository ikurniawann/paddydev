# EPIC-063: Paket Approval Atasan — Persetujuan Tim, Lembur Sebelum Dilaksanakan, Jenis Weekday/Weekend, Koreksi Lembur AdminHRIS

status: on-progress
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
- HR (peran yang sekarang `Actor.IsHR`) tetap bisa memutus pengajuan siapa pun yang bukan miliknya. Keputusannya tersimpan `decided_as = 'hr'` beserta `decided_by_name`, dan `audit.audit_log` mendapat `leave.approve|reject` / `overtime.approve|reject`.
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

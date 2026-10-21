# EPIC-065: Paket Absensi Lanjutan — Pengajuan Absen Manual/Koreksi, Absen Terlewat, Kurang Jam HQ

status: on-progress
environment: dev
retries: 0

## Goal
Menyelesaikan sisa "Paket Absensi" MOM 19-10-2026 (langkah eksekusi 6 di `pertanyaan-rekomendasi.md`; geofence dan Perjalanan Dinas sudah selesai di EPIC-062). Cakupan:
- **AB-04 + AB-06 (K-9 diterima 20-10-2026):** karyawan mengajukan dari ESS → Absensi sebuah **absen manual** (tanggal tanpa absen) atau **koreksi absen** (jam masuk/pulang hilang atau salah, mis. internet mati), dengan keterangan wajib dan foto bukti opsional. **HR** (bukan atasan langsung, sesuai `matriks-approval.md` baris "Absen Manual / Koreksi Absen | HR") menyetujui atau menolak dengan alasan di antrean HRIS → Absensi → Pengajuan Absensi. Persetujuan menulis absensi lewat **jalur yang sama** dengan penyesuaian langsung HR hari ini (`hris/attendance_adjust.go`: alasan, riwayat penyesuaian, audit, terkunci setelah periode di-closing). HR menerima notifikasi in-app lewat kanal yang sudah ada (`public.notify_hrd`, kanal yang sama dengan "Jam Kerja Kurang"), karyawan menerima notifikasi keputusan, dan karyawan melihat status pengajuannya.
- **"Alert absen terlewat" (AB-06):** bentuk terkecil yang berguna, yaitu kartu "Absen terlewat" di ESS → Absensi yang mendaftar hari kerja milik karyawan di periode absensi yang masih terbuka tanpa absen lengkap (tidak ada absen / belum absen pulang), masing-masing dengan tombol "Ajukan koreksi". HR tidak mendapat alert terpisah: setiap pengajuan sudah memicu notifikasi HR dan badge menu Absensi, dan roster Monitoring sudah menampilkan "Belum Absen" / "Tidak Hadir".
- **AB-05:** label **"Kurang jam / pulang lebih cepat"** pada baris absensi bila durasi masuk–pulang di bawah **jam kerja minimum cabang**. Jam minimum adalah setting per cabang (`configuration.branches.min_work_hours`): HQ = 9, cabang lain kosong (tanpa label). Label tersimpan di baris absensi dan tampil di kalender/detail hari ESS serta Monitoring dan Rekap HR. Notifikasi lama "Jam Kerja Kurang" (< 7 jam, semua cabang) disesuaikan agar mengikuti label. Tanpa efek payroll.
- **LA-01:** setiap keputusan dan pembatalan pengajuan, serta perubahan jam minimum cabang, tercatat di `audit.audit_log`.

Di luar cakupan: perubahan data atau setting live (mengisi jam minimum HQ = 9 adalah langkah rilis oleh HR atau atas keputusan user), perubahan payroll, dan AB-07 (rules keterlambatan, menunggu TnC).

Acuan: `docs/hris/acuan-mom-20261019/` — `status-requirement.md` (AB-04, AB-05, AB-06, LA-01), `pertanyaan-rekomendasi.md` (K-9, Q-AB-1, langkah eksekusi 6 "Paket Absensi"), `matriks-approval.md` (Absen Manual / Koreksi Absen).

## Tasks
Lihat `docs/superpowers/plans/2026-10-21-absensi-lanjutan.md` (10 task):
1. Aturan pengajuan, absen terlewat, dan jam minimum (domain Go murni).
2. Migrasi + pengajuan karyawan (buat, daftar sendiri, batal, lampiran, notifikasi HR).
3. Antrean & keputusan HR lewat jalur penyesuaian yang sama + badge (Go).
4. Absen terlewat di periode terbuka (Go).
5. Jam kerja minimum per cabang + label kurang jam + notifikasi (migrasi + Go).
6. Lib & lapisan data frontend (pengajuan, absen terlewat, kurang jam).
7. ESS → Absensi: absen terlewat, form pengajuan, status pengajuan.
8. HRIS → Absensi → tab Pengajuan Absensi + paritas badge Next.
9. Label kurang jam di kalender, Monitoring, Rekap + jam minimum di Lokasi Toko.
10. Verifikasi menyeluruh & dokumentasi.

## Acceptance Criteria
- Karyawan (akun tertaut data karyawan) bisa mengajukan absen manual/koreksi untuk **hari ini atau tanggal lampau** di periode absensi yang belum di-closing, dengan jam masuk dan/atau jam pulang (HH:MM WIB), keterangan wajib ≥ 5 karakter, dan foto bukti opsional (JPG/PNG/WebP ≤ 5 MB). Jenis (absen manual / koreksi) ditentukan server dari ada-tidaknya baris absensi pada tanggal itu.
- Ditolak 400: tanggal di masa depan, jam yang belum terjadi (termasuk jam pulang lewat tengah malam), absen manual tanpa jam masuk, koreksi yang sama dengan catatan, keterangan pendek. Tanggal di periode yang sudah di-closing → 409 (pesan periode yang sudah ada). Satu pengajuan menunggu per karyawan per tanggal (400 `Masih ada pengajuan absensi yang menunggu untuk tanggal ini`, dijaga juga oleh unique index parsial).
- Pemohon bisa membatalkan pengajuannya selama masih menunggu. Pengajuan baru untuk tanggal yang sama boleh setelah dibatalkan/ditolak.
- Antrean HR (`GET /api/hris/attendance-requests`) dan keputusan (`POST /api/hris/attendance-requests/{id}/decide`) hanya untuk pemegang menu `hris.workforce.attendance` **dan** `Actor.IsHRD` (`super_admin`/`admin`/`hrd`; `hiring_manager` 403). Atasan langsung bukan pemutus. Tidak ada yang memutus pengajuannya sendiri (403).
- Setujui menulis absensi lewat `createManualAttendance`/`adjustAttendance` (fungsi yang sama dengan `POST /api/hris/attendance/manual` dan `PUT /api/hris/attendance/{id}`): keterlambatan dihitung ulang dari shift, alasan `Pengajuan karyawan: <keterangan>` di riwayat penyesuaian, audit `attendance.create|update`, dan status pengajuan + audit `attendance_request.approve` + notifikasi karyawan dalam **satu transaksi**. Bila karyawan sudah absen setelah mengajukan absen manual, persetujuan menjadi koreksi baris itu (bukan 409). Periode yang di-closing setelah pengajuan → setujui 409, pengajuan tetap menunggu, tolak tetap bisa.
- Tolak wajib alasan; audit `attendance_request.reject`; karyawan menerima notifikasi berisi alasan; tidak ada absensi yang ditulis.
- HR menerima notifikasi in-app `Pengajuan Absensi: <nama>` setiap pengajuan baru; badge menu `/dashboard/hris/attendance` = jumlah pengajuan menunggu (Go dan fallback Next).
- ESS → Absensi menampilkan kartu "Absen terlewat" (hari kerja terjadwal tanpa absen, atau absen tanpa jam pulang, di periode terbuka sampai kemarin; libur nasional, cuti disetujui, dan tanggal yang sudah punya pengajuan menunggu dikecualikan) dengan tombol "Ajukan koreksi", serta daftar pengajuan miliknya dengan status dan alasan penolakan.
- HR (menu Penempatan) mengisi jam kerja minimum per cabang (1–24 jam, kelipatan 0,25; kosong = tanpa aturan) di dialog Lokasi Toko; audit `branch_min_hours.update`. Baris absensi cabang itu di periode terbuka dilabel ulang; periode yang sudah di-closing tidak berubah.
- Baris absensi dengan `clock_out − clock_in` < jam minimum cabang absensinya berlabel `is_short_hours = true` (snapshot `min_work_hours`), termasuk yang ditulis fallback Next, penyesuaian HR, dan persetujuan pengajuan. Tepat 9 jam tidak berlabel. Label tampil di kalender & detail hari ESS, Monitoring, dan Rekap.
- Notifikasi HR "⚠️ Jam Kerja Kurang" saat clock-out hanya untuk baris berlabel; cabang tanpa jam minimum tidak lagi memicu notifikasi < 7 jam.
- Payroll, setting live, dan data live tidak berubah.

## Pertanyaan terbuka
Tidak ada. K-9 dan Q-AB-1 sudah diputuskan user pada 20-10-2026. Butir turunan yang tidak dijawab langsung diputuskan sebagai ruling R1–R11 di rencana, mengikuti rekomendasi sesuai instruksi user untuk mengikuti rekomendasi. R7 menyempurnakan Q-AB-1 ("flag `is_hq`") menjadi jam minimum per cabang (HQ = 9), dengan efek yang sama tanpa angka 9 di kode.

## Catatan
- Kode `main` `556a7f6` (EPIC-058..064, live sejak 21-10) dan DB test (port 55499, hanya baca) diperiksa pada 21-10-2026:
  - `hris/attendance_adjust.go`: `CreateManualAttendance` dan `AdjustAttendance` hanya untuk `canManageAttendance` (peran `super_admin`/`admin`/`hrd`, himpunan yang sama dengan `Actor.IsHRD`), alasan ≥ 5 karakter, `ensureAttendanceOpen` (periode di-closing → 409), riwayat `hris.attendance_adjustments` + audit `attendance.create|update|delete`. Keduanya menjalankan transaksinya sendiri, sehingga persetujuan pengajuan perlu kait di dalam transaksi itu (Task 3), bukan salinan logika.
  - Tidak ada pengajuan absensi dari karyawan; roster HR menandai "Belum Absen" / "Tidak Hadir".
  - Trigger `public.fn_notify_attendance_changes` (bootstrap) mengirim `notify_hrd('⚠️ Jam Kerja Kurang: …')` saat clock-out bila `work_hours < 7`, semua cabang. `notify_hrd` menulis `public.notifications` untuk semua akun `configuration.users.role = 'hrd'` (tipe yang diizinkan: `approval`, `status_change`, `reminder`, `alert`). `work_hours` dari trigger `calculate_work_hours` = (pulang − masuk) − `break_minutes` (default 60).
  - `configuration.branches` tidak punya penanda HQ. DB test: 3 cabang — "Paddy HQ Bandung" (kode `MIZU-BANDUNG`, perusahaan "Paddy Kantor Pusat"), "Paddy Store Kopo", "Paddy Store PVJ". Shift "Non-Shift 09.00–18.00" = 9 jam termasuk istirahat 1 jam, sehingga ambang 9 jam dibandingkan dengan durasi masuk–pulang, bukan `work_hours` (R8).
  - Menu `hris.workforce.attendance` beraksi `read, delete`; role HR live (`admin_hris`, `hrd`, `hris_manager`, `hris_non_payroll`, `hris_non_rekrutmen_kpi`) memegang `read`. Tidak perlu aksi/grant baru, jadi tidak ada migrasi grant dan tidak perlu `iam.sync_hris_staff_roles()`. Tidak ada menu IAM baru.
  - Periode DB test: September 2026 `closed`, Oktober `open`, `cutoff_day = 0`.
  - Migrasi terakhir di `main`: `20261024090000_approval_atasan.sql`. Migrasi baru: `20261025090000_attendance_requests.sql` dan `20261025090100_short_hours.sql`.
- Label kurang jam memakai `branch_id` baris absensi. Absen HQ tanpa penempatan (branch kosong) tidak berlabel; HR perlu memastikan karyawan HQ punya penempatan di cabang HQ.

## Automation Log
- 2026-10-20: User menerima rekomendasi `pertanyaan-rekomendasi.md`, termasuk **K-9** ("Gabungkan dengan pengajuan absen manual (AB-04): karyawan mengajukan dengan keterangan, HR menerima notifikasi dan menyetujui") dan **Q-AB-1** (penanda HQ untuk aturan < 9 jam, label di baris absensi, notifikasi lama < 7 jam dihapus atau disesuaikan).
- 2026-10-21: **Instruksi user: "Lanjut paket berikutnya dari acuan MOM: Paket Absensi lanjutan (AB-04/05/06)".** Epic dibuat dan langsung `on-progress`. Kode `main` `556a7f6` dan DB test (55499, hanya baca) diperiksa; temuan ada di Catatan. Rencana ditulis di `docs/superpowers/plans/2026-10-21-absensi-lanjutan.md` (10 task, ruling R1–R11, tanpa pertanyaan terbuka). `status-requirement.md` AB-04..AB-06 kini menunjuk epic ini. Branch `feat/epic065-absensi-lanjutan` dari `main` `556a7f6`.

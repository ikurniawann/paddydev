# EPIC-080: Checkout Otomatis per Jabatan & 2 Shift dalam Sehari (`MODULE-HRIS`)

status: ready-for-qa
environment: dev
retries: 0

## Goal
Permintaan user 02-11-2026, melanjutkan aturan sistem shift EPIC-079:

> menyambung dengan shift ada bbrp role checkout otomatis (disetting per role) yaitu
> 1. host live 2. CRM 3. Shop keeper (per cabang / per toko mana untuk clockout otomatis)
> … ada pola kemungkinan 3 jabatan tsb ada 1 hari itu 2 shift bisa di cek udh jadwal kerja pola.

Jawaban user 02-11:
- **Waktu**: checkout otomatis terjadi **di jam selesai shift**. Bila karyawan belum clock-out, sistem mencatat clock-out = jam selesai shift terjadwal. Clock-out manual tetap bisa dilakukan lebih dulu.
- **2 shift sehari**: **1 absen gabungan**. Clock-in di awal shift pertama, checkout otomatis di akhir shift terakhir, absen tetap 1 baris per hari.
- **Pengatur**: **HR + pengatur jadwal toko** (EPIC-071). Pengatur hanya untuk toko yang mereka atur.

Bergantung pada EPIC-079 (PR #63): sistem shift, Host Live/CRM bertipe Shift, peringatan roster 40 jam.

## Kondisi kode per 02-11 (analisis `file:line` di Automation Log)
- **Belum ada checkout otomatis.** Absen yang lupa clock-out hanya muncul sebagai "absen terlewat" (`no_clock_out`) dan digest 09.00. Perbaikannya lewat pengajuan absensi atau koreksi HR.
- **Job latar** memakai pola goroutine + `time.Ticker`, didaftarkan di `hris/module.go:85-89` (contoh: `GapDigestLoop` tiap 5 menit, `RunGapDigest(ctx, now)` bisa diuji langsung).
- **Batas satu-per-hari**: `hris.attendance` unik per (karyawan, tanggal); `hris.employee_shift_days` unik per (karyawan, tanggal) dengan satu `shift_id`. Clock-in menyimpan `shift_id`, `scheduled_start`, dan `scheduled_end` (`attendance_clock.go:140-165`).
- **Jam kerja** dihitung trigger: `clock_out − clock_in − break_minutes`, dengan `break_minutes` default 60 dan tidak diisi saat clock-in.
- **Trigger notifikasi** `fn_notify_attendance_changes` memberi tahu HR "Jam Kerja Kurang" bila `work_hours < 7` saat clock-out diisi.
- **Setting per jabatan belum ada**; yang ada hanya per cabang (`branches.min_work_hours`, `geofence_radius_m`) dan pengatur per toko (`schedule_delegates` + `schedule_delegate_branches`).

## Ruling
- **R1 Aturan checkout otomatis** disimpan di tabel baru `hris.auto_checkout_rules`, berisi jabatan (`position_id`), cabang (`branch_id`; NULL = semua lokasi), dan `is_active`.
  - Sebuah absen kena aturan bila jabatan karyawan = `position_id` **dan** (`branch_id` NULL atau = cabang tempat clock-in).
  - Migrasi mengisi aturan "semua lokasi" untuk jabatan yang cocok `host live` dan `crm`, sesuai permintaan user.
  - Shopkeeper tidak diisi otomatis: HR atau pengatur memilih tokonya.
- **R2 Siapa mengatur.**
  - HR (menu Kepegawaian) mengatur semua aturan.
  - Pengatur jadwal toko hanya membuat, menyalakan/mematikan, atau menghapus aturan yang `branch_id`-nya termasuk toko mereka. Mereka tidak bisa mengubah aturan "semua lokasi".
  - Setiap perubahan diaudit (`attendance.auto_checkout_rule.*`).
- **R3 Kapan.** Job tiap 5 menit menutup absen terbuka (`clock_out` NULL) yang memenuhi semua syarat berikut:
  - kena aturan aktif saat job berjalan;
  - `scheduled_end` ≤ sekarang;
  - `clock_in` < `scheduled_end`;
  - tanggal ≥ hari ini − 2 (untuk shift malam);
  - periode absensi belum ditutup;
  - baris yang dibuka kembali HR dilewati: bila `adjusted_at` terisi dan ≥ `scheduled_end` (misalnya HR mengosongkan `clock_out`), job tidak menutupnya lagi.

  Absen yang ditutup otomatis mendapat:
  - `clock_out = scheduled_end`, bukan waktu job berjalan;
  - `clock_out_auto = true` (kolom baru);
  - `clock_out_geofence = 'unchecked'`;
  - catatan "Checkout otomatis pukul HH.MM (jam selesai shift)".

  Update memakai `WHERE clock_out IS NULL` supaya aman bila dijalankan berkali-kali atau oleh beberapa instance. Absen tanpa jadwal (`scheduled_end` NULL) tidak disentuh.
- **R4 Penanda, tanpa notifikasi** (my call; user belum menjawab pertanyaan penanda):
  - Absen bertanda "Checkout otomatis" di kalender ESS/HR, Monitoring, dan Rekap.
  - Absen otomatis tidak memicu notifikasi "Jam Kerja Kurang" (trigger dilewati bila `clock_out_auto`).
  - Audit `attendance.auto_checkout` dicatat dengan aktor "Sistem (checkout otomatis)".
- **R5 Setelah ditutup otomatis**, karyawan tidak bisa clock-out lagi di baris itu (pesan: "Absen sudah ditutup otomatis pukul HH.MM (jam selesai shift). Ajukan koreksi bila Anda pulang lebih lambat."). Koreksi tetap lewat pengajuan absensi (EPIC-065/067) atau koreksi HR. Label kurang jam (`attendance_short_hours`) tetap berlaku seperti biasa.
- **R6 Dua shift sehari (1 absen gabungan).**
  - `employee_shift_days` dan `employee_shifts` mendapat `shift2_id` (opsional). `v_employee_schedule` ikut membawa `shift2_id`.
  - Aturan validasi:
    - shift kedua harus mulai setelah shift pertama selesai;
    - shift kedua tidak boleh mulai, menurut jam dinding, sebelum shift pertama mulai, kecuali shift pertama melewati tengah malam (jadi SORE4 17.00–21.00 + PAGI4 06.00–10.00 ditolak, bukan dibaca sebagai esok hari). Shift kedua sendiri boleh melewati tengah malam (18.00–22.00 + 22.00–02.00 diterima). Aturan yang sama dipakai grid, unggah xlsx, API sel, dan pola mingguan;
    - pola mingguan menolak shift kedua yang sudah nonaktif;
    - shift kedua hanya boleh ada bila ada shift pertama.
  - Berlaku untuk semua staf shift (tidak dibatasi 3 jabatan); my call.
- **R7 Kode jadwal 2 shift** di grid, upload xlsx, dan pola mingguan: `KODE1+KODE2` (contoh `PAGI4+SORE4`). Tampilan: "PAGI4 + SORE4 · 08.00–12.00, 17.00–21.00".
- **R8 Clock-in pada hari 2 shift.**
  - `scheduled_start` = mulai shift 1; `scheduled_end` = selesai shift 2.
  - Terlambat dinilai terhadap shift 1 (toleransi shift 1).
  - `break_minutes` = istirahat shift 1 + istirahat shift 2 + jeda antar-shift. Dengan begitu `work_hours` = jam kerja bersih kedua shift.
- **R9 Peringatan roster** (EPIC-079):
  - Hari dengan 2 shift menghitung jam bersih keduanya untuk aturan 40 jam.
  - Jeda < 8 jam dihitung dari selesai shift 2 ke shift berikutnya.
  - Shift kedua yang tumpang tindih adalah **error**, bukan peringatan.
- **R10 Payroll**: hari dengan 2 shift tetap dihitung 1 hari kerja; tidak ada perubahan rumus.

## Tasks
Lihat `docs/superpowers/plans/2026-11-02-checkout-otomatis.md`.

## Acceptance Criteria
- [ ] Host Live clock-in 07.58, shift 08.00–16.00, tidak clock-out → setelah 16.00 absen tertutup pukul 16.00, bertanda "Checkout otomatis", tanpa notifikasi ke HR, dan ada audit.
- [ ] Shopkeeper toko A (aturan untuk toko A) ditutup otomatis; Shopkeeper toko B tanpa aturan tidak disentuh.
- [ ] Karyawan yang clock-out manual 15.30 tidak diubah job.
- [ ] Jadwal 2 shift (mis. `PAGI4+SORE4`) tersimpan dari grid dan dari upload. Shift kedua yang tumpang tindih (mis. `SPG1+SPG3`, 08–16 dan 13.30–21.30) ditolak. Untuk dua shift terpisah (08.00–12.00 + 17.00–21.00), clock-in 08.05 → `scheduled_end` 21.00, dan `work_hours` setelah checkout otomatis = 8 jam bersih.
- [ ] Shift kedua yang tumpang tindih ditolak dengan pesan jelas.
- [ ] Pengatur jadwal toko hanya bisa mengatur aturan untuk tokonya; HR bisa semua.
- [ ] HP 360 px: halaman aturan dan sel 2 shift tidak meluber.

## Automation Log
- 2026-11-02 — Analisis kode (scheduler, unik per hari, alur clock-out, pola setting, trigger notifikasi). Jawaban user untuk 3 keputusan dicatat di atas. Rencana disusun. Branch ditumpuk di atas EPIC-079 (`docs/epic079-jam-kerja-shift`).
- 2026-11-02 — Acuan diperbarui (status-requirement AB-08/AB-09, pertanyaan-rekomendasi CO-OTOMATIS, skill paddy-hris-acuan); status `review`. Temuan minor dirapikan (guard `EndTime` nil, hapus aturan dicek di leaf Shift Kerja, toast 403 hapus, format jam Beranda). Staging/QA dan PR menyusul.
- 2026-11-02 — **Implementasi subagent-driven T1–T7**, masing-masing direview spec + kualitas:
  - T1 `b4f82cf8`: skema.
  - T2 `faf646a9`: jadwal `A+B`.
  - T3 `8c57e505` + `4f935537`: shift kedua tetap terhitung terpakai; clock-in 2 shift.
  - T4 `2fa4d193` + `26330766`: job checkout otomatis; clock-out manual bersyarat (`clock_out IS NULL`) menutup race dengan job.
  - T5 `1d74075e` + `0d0b8f01`: API dan halaman aturan; hapus oleh HR butuh aksi delete `LeafShift`.
  - T6 `4bcc80ff`: UI 2 shift dan penanda; koreksi HR/pengajuan menghapus penanda.
  - T7 `5e9a9ac9` + `3057fd3d`: acuan AB-08/AB-09.
- 2026-11-02 — **Review akhir** (Opus): siap merge dengan perbaikan. Perbaikan `a558a7ea`:
  - Urutan 2 shift terbalik ditolak server, sama seperti UI (R6 diperjelas).
  - Pesan clock-out pada absen yang sudah ditutup otomatis.
  - Penanda dihapus saat jam pulang dikoreksi, termasuk saat race.
  - Job melewati absen yang dibuka ulang HR.
  - Istirahat master pada absen buatan HR.
  - Label audit.
  - Catatan "2 hari terakhir".
  - Shift 2 nonaktif ditolak di pola mingguan.

  Re-review 8/8 addressed.
- Gates:
  - Go: gofmt 0, vet, seluruh `./internal/...` lulus.
  - Frontend: tsc 0, eslint, vitest 882 file / 6417 tes.
- 2026-11-02 14.13 — **Staging** (`a558a7ea`, image `local-20261102-141258`; backup `staging-db-before-epic080-20261102-141257.dump`): 1 migrasi, skema 10.758 objek 0 selisih. Aturan awal: Host Live Staff, Host Live Coordinator, CRM Supervisor (semua lokasi).
- 2026-11-02 — **QA staging 6/6 lulus** (HP 360 px + laptop; data demo QA080 dibersihkan), 0 error console, 0 API ≥ 400 tak terduga:
  - halaman aturan untuk HR dan pengatur toko (403 untuk toko lain);
  - jadwal `QPAGI4+QSORE4` tersimpan dan urutan terbalik ditolak;
  - loop menutup Host Live dan Shopkeeper toko A pukul 16.00 dengan penanda, catatan, dan audit, tanpa notifikasi; toko B dan clock-out manual tidak disentuh;
  - badge di kalender ESS, Monitoring, dan Rekap;
  - pesan clock-out pada absen yang sudah ditutup otomatis.
  - Clock-in nyata hari 2 shift hanya diuji di integrasi Go (agar tidak memicu notifikasi HR asli).
  - Tangkapan layar di `.share/qa-epic080/`.
- **Gerbang production** (gladi di salinan prod, baca-saja):
  - `SELECT title FROM hris.positions WHERE is_active AND title ~* '(host live|crm)'`;
  - daftar absen terbuka yang akan ditutup pada run pertama (kueri di review akhir).

  Merge EPIC-079 (#63) dulu.


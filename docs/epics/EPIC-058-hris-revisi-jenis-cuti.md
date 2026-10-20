# EPIC-058: Revisi Jenis Cuti HRIS

status: review
environment: dev
retries: 0

## Goal
Jenis cuti untuk pengajuan baru disesuaikan dengan permintaan user:
- **Dihapus:** Cuti Haid, Izin Darurat, Izin Tanpa Gaji.
- **Ditambah:**
  - Izin Leader: Supervisor ke atas, 1x per bulan, tidak memotong gaji/THP.
  - Izin Pribadi: 3x per bulan, memotong 1 hari THP.
- Antar jenis cuti dapat dikombinasikan.

Riwayat cuti dengan jenis yang dihapus tetap terbaca di daftar, filter, ekspor, laporan, dan payroll.

## Tasks
Lihat docs/superpowers/plans/2026-10-20-hris-revisi-jenis-cuti.md (Task 1–8):
1. Enum `leader`/`marriage`/`bereavement` + label notifikasi in-app.
2. Katalog jenis cuti Go (satu sumber label, kuota, syarat jabatan).
3. Validasi pengajuan: jenis dihapus, Izin Leader (Supervisor ke atas), satu hari, kuota bulanan, tabrakan tanggal.
4. Endpoint `GET /api/hris/leave-quota` (pemilik, atau HR dengan menu `hris.workforce`; tanpa angka gaji).
5. Test payroll: Izin Leader tanpa potongan, Izin Pribadi = THP / pembagi × 1 hari (rule PIP EPIC-055).
6. Katalog jenis cuti frontend + ganti peta label lama.
7. Form ESS & HR: opsi sesuai kelayakan, aturan satu hari, sisa kuota.
8. Cek read-only level jabatan & riwayat, uji stack salinan DB, dokumentasi. Tanpa deploy.

## Acceptance Criteria
- Form ESS (`/dashboard/me/cuti`) dan HR (`/dashboard/hris/leaves/insert`) tidak menawarkan Cuti Haid, Izin Darurat, Izin Tanpa Gaji. API menolak ketiganya untuk pengajuan baru.
- Izin Leader hanya bisa diajukan untuk karyawan berlevel jabatan Supervisor ke atas, satu hari per pengajuan, maksimal 1 hari per bulan, dan tidak menambah potongan payroll.
- Izin Pribadi: satu hari per pengajuan, maksimal 3 hari per bulan, memotong THP / pembagi × hari (setting `personal_leave_deduction`).
- Pengajuan ditolak/dibatalkan mengembalikan kuota; kuota tiap bulan terpisah.
- Jenis berbeda boleh berdampingan; satu tanggal tidak boleh tercakup dua pengajuan aktif.
- Riwayat jenis yang dihapus tetap tampil dan bisa difilter; pengajuan lama yang pending tetap bisa diproses; ekspor CSV memakai label lama.
- Form dan endpoint kuota tidak menampilkan angka rupiah.

## Pertanyaan terbuka
Dijawab user 20-10-2026: "lakukan sesuai rekomen". Semua dari 12 pertanyaan memakai rekomendasi rencana (Ruling R1–R9). Rinciannya ada di Automation Log.

## Automation Log
- 2026-10-20: rencana ditulis (`docs/superpowers/plans/2026-10-20-hris-revisi-jenis-cuti.md`). Temuan investigasi:
  - Jenis cuti adalah enum `public.leave_type`; tidak ada tabel master. `personal` (Izin Pribadi) sudah ada sejak EPIC-055 dan dipotong lewat PIP.
  - Label tersebar di 4 tempat Go, 7 tempat frontend, dan 1 fungsi SQL.
  - Form ESS menawarkan Menikah/Duka padahal enum belum punya nilainya (pengajuan gagal).
  - Tidak ada validasi tabrakan tanggal.
  - Level jabatan hasil import Odoo kemungkinan besar masih `Staff`.
  - DB test: 0 baris cuti. Live tidak disentuh; dari paket import diperkirakan ≈5 baris `emergency` (Cuti Menikah Odoo) dan 0 `unpaid`/`menstrual`.
  - Ruling R1–R9 tercatat di rencana, menunggu konfirmasi user.
- 2026-10-20: **Keputusan user**: "lakukan sesuai rekomen". Q1–Q12 dijawab sesuai rekomendasi rencana:
  - Kombinasi: jenis berbeda boleh berdampingan dengan kuota terpisah; satu pengajuan satu jenis; satu tanggal hanya untuk satu pengajuan aktif.
  - SPV ke atas: level jabatan Supervisor/Assistant Manager/Manager/Senior Manager/General Manager/Director/C-Level. Coordinator dan Lead tidak termasuk. Level di live diisi HR lewat master Posisi.
  - Kuota dalam hari. Izin Leader dan Izin Pribadi satu hari per pengajuan. Bulan kalender; pengajuan pending ikut menghabiskan kuota.
  - Payroll tidak diubah: Izin Leader diperlakukan seperti Cuti Tahunan; Izin Pribadi memakai PIP; tunjangan harian tetap mengikuti hari hadir.
  - Menikah dan Duka ditambahkan ke enum. Riwayat `emergency` dari Odoo dibiarkan. Pengajuan pending dengan jenis yang dihapus tetap bisa diproses.
  - Tanpa override HR. Kuota 1/3 ditanam di katalog. Haid diajukan sebagai Sakit.
- 2026-10-20: **Implementasi selesai** di branch `feat/epic058-revisi-jenis-cuti`, 15 commit `aa820b0..720f11b`, 45 file. Dikerjakan subagent per task, dengan review spec dan kualitas per task, lalu review akhir seluruh branch.
  - Task 1 `aa820b0`: migrasi `20261020150000_leave_type_leader` (enum leader/marriage/bereavement) dan `20261020150100_leave_notify_labels` (label notifikasi in-app; `personal` sebelumnya tampil "Lainnya"). Sudah diterapkan di DB test.
  - Task 2 `d1f0814`: katalog Go `hris/domain/leave_types.go` menggantikan 3 peta label.
  - Task 3 `9b89ae8`: validasi `CreateLeave` mencakup jenis dihapus (400), Izin Leader hanya Supervisor ke atas, satu hari, kuota bulanan, dan tabrakan tanggal. Lock baris karyawan, cek kuota/tabrakan, dan insert berada dalam satu transaksi.
  - Task 4 `2a8ca37`: `GET /api/hris/leave-quota`, untuk pemilik atau HR yang memegang menu `hris.workforce`. Isinya hari saja, tanpa angka gaji.
  - Task 5 `4e6a3e7..fb6f9f9`: test payroll. Izin Leader dan Menikah tanpa potongan dari ujung ke ujung; PIP satu hari = THP/pembagi (literal 110.000).
  - Task 6 `ab04943`: katalog TS `lib/hris/leave-types.ts` menggantikan 7 peta label. Fallback Next hanya menerima jenis yang bisa diajukan.
  - Task 7 `ca671bc..69fa008`: form ESS dan HR. Opsi mengikuti kelayakan, tanggal selesai dikunci untuk jenis satu hari, sisa kuota tampil, filter HR menandai jenis lama dengan "(lama)". Satu bug review diperbaiki: jenis yang tampil sempat berbeda dengan jenis yang dikirim saat kelayakan hilang.
  - Task 8 `81ae711..242d729`: skrip cek read-only `backend/database/sql/checks/2026-10-20_epic058_leave_types_readonly.sql`. `/api/hris/leave-quota` didaftarkan sebagai route Go-native di `backend-routes.test.ts`.
  - Perbaikan dari review akhir `4e9e9b4..720f11b`: test render (RTL) untuk kedua form; inbox desktop menampilkan label jenis (sebelumnya kode mentah); skrip cek ditambah query pemilik/md5 fungsi notifikasi.
- **Bukti:**
  - Go full suite: 27 gagal, nama sama persis dengan baseline main (tidak ada kegagalan baru).
  - Vitest: 5522+ lolos, 0 gagal (setelah test render: 454 lolos di hris/desktop). tsc dan eslint bersih.
  - Skrip cek di DB test: 0 riwayat jenis dihapus, 0 tabrakan; level karyawan aktif: staff 21, tanpa jabatan 10, manager 3.
  - Review akhir: tidak ada Critical. Temuan Important sudah diperbaiki.
- **Belum dilakukan (sengaja):**
  - Uji stack salinan DB live dan Playwright (Task 8 Step 3–4), karena membaca live butuh izin user.
  - Deploy dan migrasi live.
- **Prasyarat deploy (keputusan terpisah):**
  - Backup.
  - Jalankan skrip cek read-only di live dan simpan hasilnya.
  - Jalankan migrasi `20261020150000` lalu `20261020150100`, baru kemudian ganti API dan web. Insert `leader` sebelum enum ada akan 500.
  - Setelah migrasi, md5 `fn_notify_leave_changes` di live harus sama dengan DB test (`3711ed5f664b621621915a574a828c04`, nilai sesudah migrasi).
  - HR mengisi `positions.level` jabatan SPV ke atas (kandidat dari Q4 skrip cek). Tanpa itu belum ada yang bisa mengajukan Izin Leader.
- **Minor yang ditunda:**
  - `fetchLeaveQuota` mengubah error (403/500) menjadi "tidak layak" tanpa pesan.
  - Label cuti kini ada di 3 salinan (Go hris, TS, Go desktop) tanpa test paritas otomatis.
  - Belum ada test konkurensi untuk slot kuota terakhir.
  - Cek pemilik employee di endpoint kuota case-sensitive (UUID huruf besar ditolak, gagal-tertutup).
  - Fallback Next tidak mengecek kuota/tabrakan (Go melayani `/api/hris`).
  - Trailer commit 9b89ae8/2a8ca37/ab04943.. memakai nama model subagent.
- 2026-10-20 15:29 WIB: **Level jabatan live diisi dari judul** (keputusan user: Supervisor/SPV → Supervisor, Assistant Manager, Manager, General Manager, CEO → C-Level; Coordinator, Lead, dan Art Director tetap Staff).
  - Skrip: `backend/database/sql/fixes/2026-10-20_positions_level_dari_judul.sql`. Mode `-v preview=1` berjalan read-only lalu ROLLBACK. Skrip hanya menaikkan level Staff/Senior Staff/kosong dan tidak pernah menurunkan.
  - Uji di DB test: 15 jabatan contoh; 10 dinaikkan, pengecualian tetap; terapkan ulang mengubah 0 baris. DB test kemudian dipulihkan.
  - Live: backup `~/Desktop/paddydev-backups/paddydev-db-before-positions-level-20261020-152906.dump`, lalu `UPDATE 41` COMMIT. Hasilnya, karyawan aktif yang berhak Izin Leader: Supervisor 20, Manager 9, Assistant Manager 1, General Manager 1, C-Level 1 (total 32). 17 jabatan Coordinator/Lead/Art Director tetap Staff. Pratinjau ulang: 0 baris tersisa.
  - Catatan: karyawan uji "QA Dimas Dua Entitas" masih aktif di live sebagai Store Manager.
  - Level jabatan juga tampil di lowongan dan PDF CV rekrutmen (kosmetik).

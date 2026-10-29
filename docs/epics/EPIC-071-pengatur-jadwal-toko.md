# EPIC-071: Pengatur Jadwal Toko — Jabatan Tertentu Mengatur Jadwal & Penempatan (`MODULE-HRIS`)

status: ready-for-qa
environment: production
retries: 0

## Goal
Permintaan user 29-10-2026, sebelum EPIC-070 di-deploy ke production: **jabatan tertentu** bisa mengunggah atau mengatur **jadwal kerja** dan **penempatan kerja** karyawan, tidak hanya HR. Daftar yang diberikan user (screenshot):

| Baris di daftar user | Data production (read-only 29-10) |
|---|---|
| Store Supervisor - Jabodetabek 1 | 1 orang, toko sendiri PPJA, 28 bawahan langsung di 11 toko |
| PPBA Shopkeeper Coordinator | Shopkeeper Coordinator di ODOO-PPBA (5 bawahan) |
| PBSA Shopkeeper Coordinator | Shopkeeper Coordinator di ODOO-PBSA (6 bawahan) |
| Store Supervisor - Makassar | 1 orang, PPMR, 11 bawahan (PPMR, PLMR) |
| General Affair Supervisor | "GA Supervisor", PADDY-PUDAK, 4 bawahan (PUDAK, SALAM, WAREHOUSE) |
| Warehouse Supervisor | 1 orang, WAREHOUSE-PADDY, 56 bawahan |
| PPSU Shopkeeper (Coordinator) | "Shopkeeper Staff Coordinator" di ODOO-PPSU (5 bawahan) |
| PPAY Shopkeeper Staff | 5 Shopkeeper Staff di PPAY; hanya 1 yang punya bawahan (8) |
| PPPA Shopkeeper Staff | 4 Shopkeeper Staff di PPPA; hanya 1 yang punya bawahan (4) |
| PPVJ Shopkeeper Staff (×2) | 5 Shopkeeper Staff di PPVJ; 1 punya bawahan (5) |

Semua orang di atas sudah punya akun login, dengan role aplikasi `employee` (Area Karyawan).

Temuan penting: nama jabatan di sistem **tidak** memuat kode toko. Ada 121 "Shopkeeper Staff" dan 7 "Shopkeeper Coordinator". Akses berbasis judul jabatan akan membuka akses ke ratusan orang yang tidak dimaksud. Jadi akses diberikan **per orang**, dan HR memilih orangnya.

Yang sudah ada sebelumnya: ESS **Shift Tim** (`/dashboard/me/shift-tim`). Atasan langsung mengatur pola mingguan bawahannya **satu per satu**. Tidak ada grid, tidak ada unggah Excel, dan tidak ada penempatan.

## Ruling (dibuat tanpa tanya; mohon dicek saat review rencana)
- **R1 — Akses per orang, bukan per judul jabatan.**
  - HR menunjuk karyawan sebagai **Pengatur Jadwal Toko**, dengan daftar **toko wilayahnya** dan centang **boleh atur penempatan**.
  - Alasan: lihat temuan di atas.
  - Bila salah: HR menambah orang satu per satu (±11 orang), tidak ada data yang rusak.
- **R2 — Wilayah = daftar toko yang dipilih HR.**
  - Saat menambah, toko diisi otomatis: toko penempatan berjalan orang itu ∪ toko penempatan berjalan seluruh bawahannya (langsung maupun tidak langsung, sampai 10 tingkat).
  - HR bisa menambah atau mengurangi toko.
  - Toko nonaktif tidak bisa dipilih dan tidak ikut terhitung.
- **R3 — Karyawan dalam wilayah** = karyawan aktif yang punya penempatan berjalan di salah satu toko wilayah, **atau** bawahan (langsung/tidak langsung) si pengatur.
  - Bawahan yang belum ditempatkan bisa ditempatkan oleh atasannya.
  - Karyawan lain yang belum ditempatkan tetap urusan HR.
- **R4 — Jadwal.** Pengatur memakai grid Jadwal Kerja EPIC-070 yang sama (kotak, rentang, pola, salin minggu lalu, unduh template, unggah Excel), tetapi hanya untuk toko wilayahnya.
  - Validasi server tetap sama: tanggal lampau, periode closing, dan hari karyawan tidak ditempatkan di toko itu ditolak.
  - Akibatnya pengatur tidak bisa menjadwalkan orang di luar tokonya.
- **R5 — Menimpa/mengosongkan jadwal per tanggal.**
  - Pengatur **boleh** dalam wilayahnya, dengan **alasan wajib** dan baris audit yang sama (`clearGuard`). Ia tidak perlu hak hapus Jadwal Kerja.
  - Alasan: tanpa ini pengatur tidak bisa memperbaiki jadwal yang sudah diisi. Itu keluhan utama EPIC-070 ("tidak cepat perubahannya").
  - Bila salah: jadwal mendatang di toko itu bisa ditimpa pengatur. Semuanya teraudit beserta alasan, dan HR bisa mencabut aksesnya.
- **R6 — Penempatan** (hanya bila dicentang "boleh atur penempatan"):
  - Pindahkan / Tambah toko: semua karyawan terpilih harus dalam wilayah, dan toko tujuan harus toko wilayah. Bila tidak → 403 untuk seluruh permintaan.
  - Pindah: karyawan yang punya penempatan berjalan di toko **di luar** wilayah dan **bukan** bawahannya dilewati dengan alasan "Punya penempatan di toko lain di luar wilayah Anda — hubungi HR".
  - Akhiri: hanya mengakhiri penempatan di toko wilayah. Karyawan tanpa penempatan di toko wilayah dilewati.
  - Ubah/hapus satu baris penempatan, lokasi toko, dan jam minimum tetap khusus HR.
- **R7 — Yang tetap khusus HR:** jadwal tetap Non-Shift, ubah tipe jadwal, master Shift Kerja, lokasi/geofence toko, dan halaman HR `/dashboard/hris/*`.
- **R8 — Tempat kerja pengatur:** menu baru Area Karyawan **Jadwal Toko** (`/dashboard/me/jadwal-toko`) dengan tab **Jadwal** dan **Penempatan** (tab Penempatan hanya bila boleh). Halaman ini bisa dipakai dari HP.
  - Menu hanya tampil untuk pengatur aktif.
  - Menu ESS lain tetap seperti sekarang. Shift Tim tetap ada.
- **R9 — Kelola pengatur oleh HR:** halaman **Pengatur Jadwal Toko** (`/dashboard/hris/placements/pengatur`), dibuka dari tombol di halaman Penempatan Karyawan. Hak yang dipakai sama dengan penempatan (`placementManagers`); simpan/cabut memakai aksi `update`.
  - Syarat: karyawan aktif dan punya akun login. Bila tidak → pesan "Karyawan belum punya akun login".
  - Setiap simpan/cabut diaudit (`schedule_delegate.set`, `schedule_delegate.revoke`).
- **R10 — Data awal:** tidak ada skrip seed.
  - HR menambah orang dari daftar user lewat halaman R9, karena untuk Shopkeeper Staff PPAY/PPPA/PPVJ hanya HR yang tahu orang yang dimaksud.
  - Halaman R9 menampilkan jabatan, toko, dan jumlah bawahan supaya mudah dicocokkan.
- **R11 — Karyawan nonaktif:** pengatur yang dinonaktifkan otomatis kehilangan akses, karena dicek `is_active` setiap permintaan. Barisnya tetap tersimpan. Bila diaktifkan kembali, aksesnya kembali.
- **R12 — Deploy:** EPIC-070 dan EPIC-071 di-deploy ke production bersama, setelah EPIC-071 lulus QA staging dan user memberi izin deploy production.

### Ruling saat implementasi (R13+)
- **R13 — "Penempatan berjalan"** (R3) = `start_date <= hari ini` dan belum berakhir. Pencarian, daftar id, hitungan chip, dan pemeriksaan "di luar wilayah" memakai definisi yang sama.
- **R14 — Tambah toko sebagai toko utama** oleh pengatur: karyawan yang **bukan** bawahannya dan punya penempatan utama yang tumpang tindih di toko **di luar** wilayah dilewati, dengan alasan yang sama seperti Pindah ("Punya penempatan di toko lain di luar wilayah Anda — hubungi HR"). Tanpa centang toko utama, tidak ada yang dilewati.
- **R15 — Akhiri massal** menerima `branch_id` huruf besar/kecil (dinormalkan ke huruf kecil). Berlaku juga untuk HR.
- **R16 — Baris penempatan di toko luar wilayah** milik karyawan dalam wilayah tetap terlihat oleh pengatur (supaya ia tahu orang itu juga bekerja di toko lain), tetapi tidak bisa diubahnya.
- **R17 — Menu Jadwal Toko** juga disembunyikan dari sidebar layout POS untuk akun yang bukan pengatur aktif.
- **R18 — Unggah & simpan kotak oleh pengatur** hanya memvalidasi karyawan roster toko itu. Karyawan lain mendapat pesan umum "… tidak ditemukan di <toko>", tanpa membocorkan data di luar wilayah.
- **R19 — Pengatur tidak bisa memundurkan penempatan:** tanggal mulai (pindah/tambah) paling awal hari ini → 400 "Tanggal mulai tidak boleh sebelum hari ini"; tanggal akhir (akhiri massal, dan akhir opsional tambah/pindah) paling awal kemarin → 400 "Tanggal akhir paling awal kemarin". Dialog mode toko memasang batas `min` yang sama dan tidak menampilkan peringatan tanggal lampau. HR tetap boleh tanggal lampau (koreksi).
- **R20 — Karyawan multi-toko:** pengatur boleh menimpa jadwal per tanggal karyawan yang juga bekerja di toko lain, pada hari karyawan itu ditempatkan di toko si pengatur (konsekuensi R4; teraudit dengan alasan).
- **R21 — Akses dicabut saat halaman terbuka:** 403 umum dari API mode toko (penunjukan dicabut atau karyawan dinonaktifkan) memuat ulang akses dan halaman Jadwal Toko menampilkan kartu "Anda bukan pengatur jadwal toko. Hubungi HR bila seharusnya punya akses." Teks server berbahasa Inggris tidak pernah tampil. Grid dan pencarian mode toko tidak mencoba ulang 403. 403 wilayah ("… di luar wilayah Anda") dan hak penempatan tetap menampilkan pesannya.
- **R22 — Halaman HR Pengatur Jadwal Toko** bisa dibaca dengan prefix menu penempatan; Tambah/Ubah/Cabut hanya tampil dengan aksi `update` (sama dengan guard PUT/DELETE).
- **Catatan tes:** `TestGapDigestRun` kadang gagal saat seluruh paket berjalan (DB uji dipakai bersama), lulus bila dijalankan sendiri.

## Tasks
Lihat rencana `docs/superpowers/plans/2026-10-29-pengatur-jadwal-toko.md`.

## Acceptance Criteria
- HR menunjuk seorang Shopkeeper Coordinator sebagai pengatur untuk tokonya. Toko terisi otomatis dari penempatan dan bawahan, dan bisa diubah.
- Pengatur login di HP dan melihat menu **Jadwal Toko**. Karyawan lain tidak melihat menu itu.
- Pengatur bisa mengubah satu kotak, menerapkan pola mingguan, salin minggu lalu, dan mengunggah Excel untuk tokonya. Toko lain tidak tersedia, dan permintaan langsung ke API untuk toko lain → 403.
- Menimpa jadwal per tanggal oleh pengatur meminta alasan dan tercatat di audit.
- Pengatur dengan hak penempatan bisa memindahkan karyawan antar toko wilayahnya dan menempatkan bawahan yang belum ditempatkan. Karyawan/toko di luar wilayah → 403 atau dilewati dengan alasan, sesuai R6.
- HR mencabut akses → menu hilang dan API → 403.
- Endpoint dan halaman HR EPIC-070 berperilaku sama (tes lama tetap hijau).
- `go test ./...`, `pnpm test`, `pnpm lint`, dan `tsc` hijau; CI 4/4 hijau; QA staging lulus.

## Automation Log
- 2026-10-29: **Permintaan user** + screenshot daftar 11 jabatan.
  - Pemetaan kode main `940be473`:
    - akses jadwal/penempatan hanya lewat menu IAM HR (global, tanpa wilayah);
    - akun punya scope satu cabang saja;
    - ESS Shift Tim = atasan langsung, pola mingguan per orang.
  - Data production dibaca read-only (tabel di Goal).
  - Rencana ditulis, status `backlog` menunggu review user.
- 2026-10-29: **Implementasi** (subagent per task + review per task), branch `feat/epic071-jadwal-per-jabatan`:
  - **T1** `b3b07484`: migrasi `hris.schedule_delegates` + `schedule_delegate_branches`, menu ESS `ess.store-schedule` (semua role), seeder, `GET /api/hris/me/schedule-access`, akses inti (`withScheduleScope`: HR atau pengatur).
  - **T2** `ec5f5358`: API HR kelola pengatur `schedule-delegates` (list/suggest/PUT/DELETE, audit `schedule_delegate.set|revoke`).
  - **T3** `2b63d40c..b7b65a30`: jadwal mode pengatur (grid, kotak, unggah, template) dibatasi toko wilayah; R13, R18, R20.
  - **T4** `e053540c..f27e9ce9`: penempatan mode pengatur (cari/ids/bulk/bulk-end) dengan R6; R14, R15, R16.
  - **T5** `76369281..444fafbe`: halaman HR Pengatur Jadwal Toko (daftar, dialog tambah/ubah dengan saran toko, cabut).
  - **T6** `58d0aaea..75c7ea70`: halaman ESS Jadwal Toko (tab Jadwal/Penempatan, mode toko), menu hanya untuk pengatur aktif; R17.
- 2026-10-29: **Review akhir seluruh branch:** With fixes. Diperbaiki dalam satu gelombang:
  - `2dafcb66` Go: pengatur tidak bisa memundurkan tanggal penempatan (R19); ikon menu Jadwal Toko `calendar` (ikon `calendar-days` tidak dikenal sidebar).
  - `4fd01103` halaman HR pengatur: aksi hanya untuk hak `update` (R22); Simpan nonaktif bila daftar pengatur gagal dimuat.
  - `570eab5a` Jadwal Toko: akses dicabut → kartu (R21); batas tanggal dialog mode toko (R19); grid kosong mengarahkan ke tab Penempatan bila boleh.
- 2026-10-29: **Gerbang lokal** (sesudah fix wave):
  - `go vet ./internal/modules/hris/...` bersih; `go test ./internal/modules/hris/... -count=1` hijau;
  - `make -C backend generate` tanpa diff;
  - frontend `tsc` + `lint` bersih, `pnpm test` 861 file / 6256 tes lulus (sebelum fix wave: 860 file / 6235 tes).
- 2026-10-29: **PR #46 merged** (`fb055275`), CI 4/4 hijau. Deploy staging 09:06 WIB (backup `~/Desktop/paddydev-backups/staging-db-before-epic071-20261029-090625.dump`).
- 2026-10-29: **QA staging** 7/7 skenario lulus. Dua perbaikan dari cek di HP:
  - PR #47 (`7e3af267`): pilih banyak kotak lewat sentuhan, legenda kode shift bisa dilipat.
  - PR #48 (`f270c919`): tombol "Kode shift" tidak terlipat di HP.
  - Staging akhir `local-20261029-100312` = main `f270c919`. Laporan + screenshot: `.share/qa071/`.
- 2026-10-29 12:38: **Uji coba di staging**: 10 pengatur ditunjuk (actor audit 'Sistem (uji staging EPIC-071)'). Toko disalin dari saran production, minus ODOO-TWLK yang tidak ada di staging. Backup: `staging-db-before-epic071-delegates-20261029-123822.dump`.
- 2026-10-29: **Video tutorial koordinator** (laptop 1080p + HP 780×1688, ±2,5 menit, teks di layar) di `.share/video071/`. Direkam di staging dengan data contoh; data contoh sudah dihapus, tersisa 34 baris audit (append-only).
- 2026-10-29: **Deploy production** 23:29–23:37 WIB bersama EPIC-070, atas izin eksplisit user.
  - Image `local-20261029-233049` = main `f270c919`. Migrasi `20261029120000_schedule_delegates.sql` diterapkan; verify 0 beda; 2 tabel + menu `ess.store-schedule` ada.
  - Backup, log, dan rollback: lihat Automation Log EPIC-070 (stamp `20261029-232927`).
- 2026-10-29 23:37: **Penunjukan 21 pengatur di production** dari Excel HR "Data Pemberian Akses untuk Input Jadwal Kerja Shift (2)". Baris PPVJ kedua ternyata Puspa lagi (duplikat), jadi tidak ada orang kedua.
  - Semua `can_place = true`; 55 tautan toko; 21 baris audit `schedule_delegate.set` (actor 'Ops (Claude Code atas persetujuan user)').
  - Toko = saran sistem (penempatan berjalan orang itu + bawahannya) yang dibaca read-only malam itu. Uji kering (ROLLBACK) dulu, baru COMMIT.
  - Farhan: TWLK masuk, PPLN tidak (penempatan PPLN kini di bawah Asriani).
  - **Catatan untuk HR:** Dhimas (ODOO-WAREHOUSE-PADDY) dan Siti Asyah (MIZU-BANDUNG) tidak punya bawahan. Wilayah mereka = seluruh karyawan di toko itu (R3: toko ATAU bawahan). HR dapat mempersempit atau mencabut di Penempatan → Pengatur jadwal toko.
  - Rollback penunjukan saja: cabut di aplikasi, atau `DELETE FROM hris.schedule_delegates` (toko ikut terhapus lewat cascade).
  - Status `ready-for-qa`: koordinator/HR mencoba menu Jadwal Toko di production.

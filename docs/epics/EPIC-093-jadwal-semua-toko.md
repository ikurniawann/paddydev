# EPIC-093: Koordinator Mengatur Jadwal Semua Toko (`MODULE-HRIS`)

status: review
environment: dev
retries: 0

## Goal
Revisi user 06-11-2026 atas EPIC-071:

> "Setiap Koordinator bisa membuat jadwal toko lain, tidak hanya toko yang dia pegang. dan tidak perlu persetujuan SPV, karena sifatnya hanya setting jadwal"

Kondisi saat ini (main `614d3694`): pengatur jadwal toko hanya boleh membuka grid, template, unggah, dan simpan kotak untuk **toko wilayahnya**. Toko lain ditolak 403 "Toko di luar wilayah Anda" (EPIC-071 R4). Perubahan jadwal oleh pengatur sudah **langsung berlaku**: tidak ada alur persetujuan jadwal di sistem, hanya audit.

Data production (read-only 06-11):

| Kelompok | Jumlah |
|---|---|
| Pengatur aktif | 21 |
| Pengatur berjabatan Coordinator | 9: 6 Shopkeeper Coordinator, 1 Shopkeeper Staff Coordinator, 1 Host Live Coordinator, 1 Case Production Coordinator |
| Berjabatan Coordinator/Koordinator tapi bukan pengatur | 4, jabatan non-toko: Content Creator Koordinator, Inbound, Logistics, Outbound Coordinator |
| Level jabatan semua koordinator | Staff (tidak ada level "Koordinator") |
| Toko aktif | 45 |

Keputusan user (06-11):
- **Siapa:** HR memberi centang **"Boleh atur jadwal semua toko"** per pengatur. Saat deploy, centang dipasang untuk 9 pengatur koordinator, dengan izin user.
- **Cakupan:** **jadwal saja.** Penempatan tetap hanya di toko wilayahnya.

## Ruling
- **R1 — Centang per pengatur.** Kolom baru `schedule_all_branches`, default mati. Akses tetap per orang, tidak per judul jabatan (sama dengan EPIC-071 R1). Bila salah: HR mencentang atau mencabut sendiri, tanpa data rusak.
- **R2 — "Jadwal" mencakup** grid, ubah kotak, pola, salin minggu lalu, unduh template, dan unggah Excel, untuk **semua toko aktif**. Validasi server tetap sama: tanggal lampau, closing, roster toko itu (R18), dan alasan wajib saat menimpa (R5).
- **R3 — Tanpa persetujuan.** Jadwal yang disimpan pengatur di toko mana pun langsung berlaku. Jejaknya tetap di audit `schedule.cells_update` / `hris.schedule.replace` atas nama pengatur. Tidak ada notifikasi ke SPV toko tujuan, karena user tidak memintanya.
- **R4 — Tetap wilayah sendiri:** penempatan (cari, pindah, tambah, akhiri) dan aturan **checkout otomatis**. Checkout otomatis adalah aturan absensi, bukan jadwal. Bila salah: satu baris kode untuk membukanya.
- **R5 — Wilayah tetap wajib.** Pengatur bercentang tetap harus punya minimal satu toko wilayah aktif. Tanpa itu ia bukan pengatur, sama dengan aturan menu sidebar EPIC-071.
- **R6 — Pilihan toko di tab Jadwal** memuat semua toko aktif, urut kode. Pengatur dengan satu toko wilayah tetap langsung melihat tokonya. Tautan "ke tab Penempatan" hanya muncul di toko wilayah.
- **R7 — Toko nonaktif** tidak muncul dan tidak bisa dibuka (403).
- **R8 — Data production.** Centang untuk 9 pengatur koordinator dipasang sesudah deploy production, atas izin terpisah: satu transaksi, audit per orang, uji kering dulu. 3 Shopkeeper Staff kepala toko (PPAY/PPPA/PPVJ) dan Store Supervisor **tidak** dicentang, kecuali HR menambahkannya.

## Tasks
Lihat rencana `docs/superpowers/plans/2026-11-06-jadwal-semua-toko.md`.

## Acceptance Criteria
- [x] HR mencentang "Boleh atur jadwal semua toko" pada seorang pengatur. Kolom "Jadwal semua toko" menampilkan Ya, dan audit memuat nilainya.
- [x] Pengatur bercentang membuka Jadwal Toko di HP. Daftar toko memuat toko lain, dan tokonya sendiri terpilih otomatis.
- [x] Pengatur bercentang mengubah jadwal di toko lain. Jadwal langsung tersimpan tanpa persetujuan, dan tercatat di audit.
- [x] Penempatan dan checkout otomatis tetap hanya toko wilayah. API penempatan untuk toko lain → 403.
- [x] Pengatur tanpa centang berperilaku persis seperti EPIC-071 (tes lama hijau).
- [ ] `go test`, `pnpm test`, `pnpm lint`, dan `tsc` hijau; CI hijau; QA staging lulus.

## Automation Log
- 2026-11-06: **Permintaan user** (revisi Koordinator). Kode dipetakan (`withScheduleScope`, `AllowsBranch` di 4 rute jadwal, penempatan, checkout otomatis). Data production dibaca read-only (tabel di Goal). User memilih centang per pengatur oleh HR dan cakupan jadwal saja. Rencana ditulis; status `backlog` menunggu review user.
- 2026-11-06: **User menyetujui rencana** ("Sesuai, Native"). Implementasi inline di branch `feat/epic093-jadwal-semua-toko`:
  - **T1** `2c9c772b`: migrasi `20261111090000_schedule_delegate_all_branches.sql`; `delegateAccess.ScheduleBranchIDs` (semua toko aktif bila dicentang); `storeScope.AllowsScheduleBranch` di 4 rute jadwal (grid, template, unggah, kotak); schedule-access mengirim `schedule_all_branches` + `schedule_branches`. Penempatan dan checkout otomatis tetap `AllowsBranch` (R4).
  - **T2** `99af942b`: API HR pengatur menyimpan/menampilkan `schedule_all_branches`; PUT tanpa field = mati; audit before/after memuat nilainya.
  - **T3** `efaba7ec`: tab Jadwal memakai `schedule_branches`, toko wilayah tunggal tetap terpilih otomatis, tautan Penempatan hanya di toko wilayah, catatan biru untuk pengatur bercentang (R6).
  - **T4** `67b7035f`: centang "Boleh atur jadwal semua toko" + petunjuk di dialog HR, kolom "Jadwal semua toko" di tabel.
  - Ruling implementasi: migrasi diterapkan ke DB uji bersama lewat `cmd/migrate -apply`; tes audit memilih baris lewat `before IS NOT NULL` (satu transaksi uji); `SchedulesPage` fallback `schedule_branches ?? branches`; petunjuk lama di dialog tetap, petunjuk baru di bawah centang; "semua toko aktif" = semua cabang aktif (sama dengan mode HR).
- 2026-11-06: **Gerbang lokal**: `go vet` + `gofmt` bersih; `go test -p 1 ./internal/modules/hris/...` ok; `make -C backend generate` tanpa diff; frontend `pnpm test` 888 file / 6467 tes lulus, `tsc` + `lint` bersih.
- 2026-11-06: **Review akhir seluruh branch** (reviewer segar): Critical 0, Important 0. Minor ditunda:
  - tes toko nonaktif hanya grid; bulk-end dan create/patch/delete checkout otomatis non-wilayah tidak dites untuk pengatur bercentang (cek yang sama sudah dites);
  - bila centang dicabut saat toko lain terbuka, pilihan toko tampak kosong dengan pesan 403;
  - teks Beranda "di 1 toko Anda" tidak menyebut akses jadwal semua toko;
  - doc comment `withScheduleScope` belum menyebut `AllowsScheduleBranch`;
  - satu query cabang aktif tambahan per permintaan pengatur bercentang (45 baris).
- 2026-11-06: **PR #81** dibuka (base `main`).
- 2026-11-06 13:18: **Deploy staging**.
  - Backup `~/Desktop/paddydev-backups/staging-db-before-epic093-20261106-131816.dump`.
  - Merge ke tumpukan staging `00fb08ce` (EPIC-087..092) menjadi `44cb8747`. Konflik baris registry README diselesaikan dengan mempertahankan keduanya.
  - Image `local-20261106-131841`, 1 migrasi diterapkan, verify 0 berbeda, container healthy.
- 2026-11-06: **QA staging 18/18 lulus** (Playwright, HR desktop + koordinator HP 360 px). Skenario yang diuji:
  - HR mencentang lewat dialog Ubah: kolom "Ya", DB true, audit after true.
  - Koordinator HP: catatan biru tampil dan PPAY langsung terpilih. Ke-52 toko aktif staging ada di daftar jadwal, wilayahnya tetap 1. Grid PPBA memuat karyawan PPBA.
  - Simpan kotak PPBA 200/200: jadwal langsung tersimpan dan tampil di grid HR.
  - Penempatan dan cari penempatan PPBA → 403. Checkout otomatis hanya PPAY.
  - HR mencabut centang: tersisa 1 toko jadwal, grid PPBA → 403. Di desktop tanpa centang catatan tidak tampil.
  - 0 error konsol, 0 API ≥ 400 di luar 403 yang diharapkan.
  - Akun demo `qa093-*` dihapus (0 tersisa). 3 baris audit tetap (append-only). Bukti di `.share/qa093/`.
- 2026-11-06: **Dua temuan minor dirapikan sebelum production** atas permintaan user (`3f22ca8e`):
  - Centang dicabut saat toko lain terbuka: pilihan toko kini kembali ke toko wilayah, tidak lagi kosong dengan pesan 403. Tes `centang dicabut saat toko lain terbuka …` gagal di kode lama (pilihan `''`) dan lulus sesudahnya.
  - Teks untuk pengatur bercentang: kartu Beranda "Atur jadwal kerja karyawan di semua toko, penempatan di N toko Anda" (tanpa hak penempatan: "… di semua toko"); subjudul Jadwal Toko "Atur jadwal kerja karyawan di semua toko. …".
  - Bukti: frontend `pnpm test` 888 file / 6471 tes lulus; `tsc` + `lint` bersih.
- 2026-11-06 13:38: **Deploy staging ulang** dengan kedua perbaikan.
  - Backup `staging-db-before-epic093b-20261106-133808.dump`; deploy-e071 `fd9e49f3`; image `local-20261106-133815`; migrasi sudah lengkap, verify 0 berbeda.
  - **QA ulang 21/21 lulus**: 18 skenario sebelumnya ditambah subjudul "semua toko", kartu Beranda "… di semua toko, penempatan di 1 toko Anda", dan pencabutan centang saat PPBA terbuka di HP (pindah minggu → pilihan kembali ke PPAY tanpa pesan 403 tertinggal).
  - Data demo dihapus (0 tersisa; 10 pengatur uji coba staging utuh). Bukti `.share/qa093/` (`t2.log`, `2b-…`, `5-…`).
  - Siap production: menunggu izin user untuk merge PR #81 + deploy, lalu izin terpisah untuk centang 9 pengatur Coordinator (R8).

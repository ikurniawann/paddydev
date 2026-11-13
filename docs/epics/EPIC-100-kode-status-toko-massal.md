# EPIC-100: Kode & Status Toko Massal lewat Template (`MODULE-HRIS`, `MODULE-CONFIGURATION`)

status: ready-for-qa
environment: dev
retries: 0

## Goal
Permintaan user (10-11-2026):

> ada task untuk epic baru yaitu dimana branch Update kode branch secara massal melalui template. lalu Nonaktifkan store tutup/ganti kode, histori tetap tersimpan.
> update bulky code per branch juga cek

**Data staging per 10-11** (salinan production): 56 branch.
- Paddy: 52; Paddy Kantor Pusat: 4.
- 44 branch masih berkode `ODOO-…` (mis. `ODOO-PPGA`), padahal namanya sudah `PPGA - Paddy Playstore Backyard` (format EPIC-069).
- 11 branch nonaktif. 3 penempatan aktif masih menunjuk toko nonaktif (mis. Event PVJ).

**Kode sekarang:**
- 90 foreign key menunjuk `configuration.branches(id)`. Tidak ada tabel yang menyalin kode sebagai teks, jadi mengganti kode tidak memutus histori.
- Kode dipakai sebagai kunci teks di template Data Kerja massal (`hris/work_data.go`, `upper(btrim(code))` tanpa melihat perusahaan).
- Settings → Business bisa mengubah kode dan status satu per satu. Tombol **Hapus** menjalankan `DELETE`. Akibatnya:
  - `hris.attendance.branch_id` menjadi NULL;
  - `auto_checkout_rules` dan `schedule_delegate_branches` ikut terhapus;
  - penempatan menolak penghapusan (RESTRICT).

## Ruling (keputusan user 10-11)
- **R1 — Ganti kode di toko yang sama.** Baris `configuration.branches` tetap sama, hanya `code` yang berubah. Semua histori ikut tampil dengan kode baru.
- **R2 — Template.** Halaman **Penempatan Karyawan** punya tombol **Kode & Status Toko**. Tombol ini membuka dialog: unduh template, unggah, pratinjau, simpan.
  - Akses sama dengan tombol Lokasi Toko (`placementManagers`); menyimpan butuh aksi `update`.
  - Sheet `Data Toko` berisi semua branch, dengan kolom: **Kode Saat Ini** (kunci), Perusahaan, Nama Saat Ini, Status Saat Ini, **Kode Baru**, **Nama Baru**, **Status Baru** (`Aktif`/`Nonaktif`).
  - Sel kosong = tidak diubah.
- **R3 — Nama ikut kode.** Bila Kode Baru diisi dan Nama Baru kosong, awalan nama `<kode lama tanpa ODOO-> - ` diganti `<kode baru tanpa ODOO-> - `. Contoh: kode `ODOO-PPGA` → `PPGX` membuat nama `PPGA - …` menjadi `PPGX - …`. Nama tanpa awalan itu tidak disentuh. Nama Baru yang diisi selalu menang.
- **R4 — Nonaktif ditolak sampai dipindah.** Status Baru `Nonaktif` ditolak bila toko masih punya:
  - penempatan aktif (tanpa tanggal akhir, atau berakhir hari ini atau sesudahnya), atau
  - jadwal (`employee_shift_days`) mulai hari ini (WIB).

  Pesannya menyebut jumlah dan nama karyawan, lalu mengarahkan ke Penempatan. Mengaktifkan kembali selalu boleh. Histori absen, penempatan, jadwal, dan payroll tidak disentuh.
- **R5 — Hapus cabang dijaga.** Di Settings → Business, cabang yang punya data terkait di tabel mana pun (selain `configuration.warehouses`) **tidak dapat dihapus**. Pesannya mengarahkan ke Nonaktifkan. Cabang tanpa data terkait tetap bisa dihapus.

## Ruling (tanpa tanya)
- **R6 — Kode baru diperiksa:**
  - dipangkas dan dijadikan huruf besar;
  - pola `^[A-Z0-9][A-Z0-9-]{1,29}$`;
  - unik terhadap semua branch semua perusahaan, karena template Data Kerja mencari kode tanpa perusahaan;
  - tidak ganda di file;
  - tidak boleh sama dengan kode saat ini milik toko lain, jadi tukar kode dalam satu file tidak bisa.
- **R7 — Pratinjau baru ditulis, dengan pengaman.**
  - Pratinjau tidak menulis apa pun.
  - Simpan berjalan dalam satu transaksi. Ia ditolak bila masih ada error, atau bila fingerprint (file + kode/nama/status branch yang diubah) berbeda dari saat pratinjau.
  - Cek R4 diulang saat menyimpan, dengan baris branch dikunci.
  - Setiap toko yang berubah menulis audit `branch.update` (before/after: code, name, is_active).
- **R8** — Peringatan, bukan error, untuk setiap kode yang berubah: file Data Kerja/Jadwal yang diunduh sebelum perubahan memakai kode lama, jadi harus diunduh ulang. Simpan butuh centang "Saya sudah memeriksa peringatan".
- **R9** — Tidak ada perubahan skema. Nama toko tetap dicerminkan ke `item.brands` oleh trigger yang sudah ada.

## Tasks
Lihat `docs/superpowers/plans/2026-11-10-kode-status-toko-massal.md`.

## Acceptance Criteria
- [x] Template berisi 56 branch dengan kode, perusahaan, nama, dan status saat ini.
- [x] `ODOO-PPGA` → `PPGA` (nama tetap `PPGA - Paddy Playstore Backyard`) tersimpan. Histori absen dan penempatan toko itu tetap ada dan tampil dengan kode baru.
- [x] `ODOO-PPGA` → `PPGX` tanpa Nama Baru membuat nama menjadi `PPGX - Paddy Playstore Backyard`.
- [x] Kode baru yang sudah dipakai toko lain, ganda di file, atau salah pola ditolak per baris. Kode Saat Ini yang tidak dikenal juga ditolak.
- [x] `Nonaktif` untuk toko dengan karyawan aktif atau jadwal mendatang ditolak dengan nama karyawan. `Nonaktif` untuk toko kosong (mis. `ODOO-PPBI`) tersimpan, dan toko itu hilang dari pilihan toko.
- [x] Hapus cabang yang punya histori di Settings → Business ditolak. Cabang baru yang kosong tetap bisa dihapus.
- [x] Gate hijau: Go `./internal/...`, vitest dialog, tsc, eslint, dan CI PR.

## Automation Log
- 2026-11-10: Permintaan user. Skema dan data branch staging diperiksa (read-only). Keputusan user lewat AskUserQuestion: ubah kode di toko yang sama, tolak nonaktif sampai karyawan dipindah, nama ikut kode + kolom Nama Baru, hapus cabang yang punya histori ditolak. Rencana ditulis. Branch `feat/branch-kode-massal` dari main `5e3aa6e4`.
- 2026-11-10: **Implementasi Native** di branch `feat/branch-kode-massal`.
  - Task 1 (4d319968): `domain.ValidateBranchCodeUpload` + `BranchNameForCode`.
  - Task 2 (76a47b0e): `GET /api/hris/branches/template` dan `POST /api/hris/branches/import` (preview/commit/error_file). Simpan mengunci hanya toko yang berubah lalu memvalidasi ulang; audit `branch.update`.
  - Task 3 (eab10691): penjaga hapus di Settings → Business berlaku untuk cabang, serta perusahaan/holding yang cascade ke cabang berhistori. Daftar FK dibaca dari `pg_constraint`.
  - Task 4 (c457b102): dialog **Kode & Status Toko** di halaman Penempatan.
  - Ruling pelaksana:
    - kunci hanya toko yang berubah;
    - penjaga juga untuk hapus perusahaan/holding;
    - template diunduh lewat tautan.
- Review akhir (opus): Critical 0, Important 1, Minor 9.
  - Important: kode yang dipakai dua perusahaan membuat edit tersimpan ke toko lain. Kini kode itu ditolak per baris dengan arahan ke Settings → Business (10a554f0, `TestValidateBranchCodeUploadAmbiguousCode` RED→GREEN). Data staging: 0 kode ganda.
  - Minor ditunda:
    - impor bersamaan tanpa advisory lock;
    - deadlock jarang dengan penempatan massal (500);
    - penempatan baru di toko nonaktif tidak dicek (perilaku lama);
    - penjaga hapus menjalankan ±84 EXISTS per cabang, di luar transaksi DELETE;
    - pratinjau lama tetap ada setelah dialog ditutup (simpan → 409 aman);
    - 400 saat simpan tidak membersihkan pratinjau;
    - pratinjau butuh aksi update;
    - unduh template saat 401 menyimpan JSON sebagai file;
    - fallback GRN memakai kode `MIZU-BANDUNG` yang tertanam di kode. **Jangan ganti kode MIZU-BANDUNG lewat template sebelum fallback ini dibenahi.**
- Gate:
  - Go: gofmt 0, vet, `./internal/...` 137 ok.
  - Frontend: vitest placements 51/51, backend-routes 54/54, tsc 0, eslint bersih.
- Staging 13.08 (image `local-20261110-130827`, backup `/mnt/hdd1/backup/paddydev/staging-before-e100-20261110-130818.dump`):
  - template berisi 56 toko;
  - `ODOO-PPGA` Nonaktif ditolak: 11 karyawan aktif disebut namanya;
  - `ODOO-PPBI` Nonaktif tersimpan dan hilang dari `/api/hris/branches` (44 toko aktif);
  - `ODOO-TWLK` → `TWLK`: 17 absen dan 3 penempatan tetap di toko yang sama;
  - hapus `ODOO-PPGA` di Settings ditolak;
  - semua perubahan dikembalikan (dibandingkan dengan diff: sama). 4 baris audit uji tetap ada karena `audit.audit_log` append-only. Sesi uji dihapus.
- 2026-11-13: Registry dirapikan, status review → ready-for-qa. Kode sudah ada di erp-paddy `development` (impor monorepo e815cee) dan jalan di staging (image `local-20261112-234046`). Sisa: QA UI oleh HR, lalu rilis production dengan izin user.

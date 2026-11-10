# EPIC-099: Format NIP YYYYMM + Nomor Urut (`MODULE-HRIS`)

status: review
environment: dev
retries: 0

## Goal
Permintaan user (10-11-2026):

> untuk format NIP YYYYMM000 (Eg: 202603518), ini tahun dan bulan join date, 3 angka terakhir adalah nomor urutan karyawan terbaru

Lanjutan EPIC-081 (NIP diketik manual, wajib) dan EPIC-098 (impor NIP Atasan).

**Data staging per 10-11** (salinan production 05.55), total 644 karyawan:
- 520 NIP berbentuk 9 angka. 464 di antaranya 6 digit depannya sama dengan tahun-bulan join date. 56 berbeda, kebanyakan karena join date diubah kemudian (mis. perpanjangan kontrak ke 21-07-2026).
- 124 NIP bentuk lain: `2026-05-025`, `EMP-…`, `000000`.

Nomor urut terdiri dari **dua deret**:
- **Deret utama**: satu urutan berjalan sejak 2016, tidak pernah mulai ulang. Akhir 2025 di 507; 2026 berjalan 508 → **676** (`202611676`).
- **Deret harian**: dimulai ulang dari 001 setiap tahun, sejak 2026. Isinya terutama Daily Worker, Freelance, dan Magang. Sekarang di **095** (`202611095`).

Nomor pernah dipakai dua kali dengan bulan berbeda. Contohnya `202601518` (sudah resign) dan `202603518` (aktif).

## Ruling (keputusan user 10-11)
- **R1 — Tetap manual, sistem mengecek format.** HR tetap mengetik NIP; tidak ada isian otomatis. Karyawan **baru** ditolak jika NIP-nya tidak memenuhi aturan berikut:
  1. Hanya angka: 6 digit `YYYYMM` (bulan 01–12) diikuti 3 digit nomor urut (001–999). Nomor ≥ 1000 ditulis 4 digit tanpa nol di depan.
  2. `YYYYMM` sama dengan tahun-bulan join date.
  3. Nomor urut **lebih besar dari nomor terakhir** di deretnya. Celah nomor boleh, tetapi nomor yang sudah lewat tidak boleh dipakai ulang.

  Pesan penolakan menyebut nomor terakhir dan contoh NIP berikutnya.
  Berlaku di: `POST /api/users` (dengan data karyawan), `POST /api/hris/employees`, `POST /api/hris/promote`, dan baris *create* impor Excel (mode Tambah maupun Timpa).
- **R2 — Dua deret.**
  - Status awal `daily_worker`, `freelance`, `intern`, atau `internship` (Magang dari Promosi) → **deret harian** tahun join date. Status lain → **deret utama**.
  - **Deret utama** = nomor urut terbesar dari semua NIP berformat, dari tahun mana pun. Nomor baru selalu **≥ 500**.
  - **Deret harian tahun Y** = nomor urut terbesar dari NIP berformat bertahun Y yang nomornya **≤ 499**. Nomor harian baru harus di atas nomor itu dan **maksimal 499**.
  - *Revisi 10-11 (review akhir).* Rancangan awal memakai "batas harian = nomor terbesar tahun-tahun sebelumnya" (2026: 507). Rancangan itu rusak bila ada karyawan reguler back-dated, mis. `202512677` untuk join Desember 2025 yang dimasukkan pada 2026: batas harian 2026 naik ke 677, nomor utama 508–676 ikut terhitung harian, dan Daily Worker/Freelance/Magang tidak bisa dibuat sampai akhir tahun. User memilih **batas tetap 499**.
- **R3 — Lewat 999 lanjut 4 digit.** Contoh: nomor 1000 di Januari 2028 menjadi `2028011000` (10 angka). Nomor tidak pernah mulai ulang.
- **R4 — NIP lama dibiarkan.** Aturan R1.1 dan R1.2 hanya dicek saat ubah karyawan jika **NIP-nya diganti**, dibandingkan dengan join date baru (bila dikirim) atau join date lama. Saat ubah tidak ada cek nomor urut, karena koreksi salah ketik harus tetap bisa. Mengubah join date saja tidak mengecek NIP.
- **R5 — Satu file impor.** Dua baris baru di satu file tidak boleh memakai nomor urut yang sama dalam satu deret. Contoh: `202611677` dan `202612677` → error di baris kedua.
- **R6** — Tidak ada perubahan skema DB. Semua pesan NIP diawali `NIP `, supaya form Tambah Karyawan dan dialog Promosi menampilkannya di kolom NIP.

## Tasks
Lihat `docs/superpowers/plans/2026-11-10-format-nip-urut.md`.

## Acceptance Criteria
- [x] Tambah karyawan probation, join 2026-11-10, NIP `202611677` → tersimpan (data staging: deret utama terakhir 676).
- [x] `202611650` ditolak dengan pesan "nomor urut terakhir karyawan 676 … mis. 202611677".
- [x] `202610677` dengan join 2026-11-10 ditolak karena 6 digit depan harus `202611`.
- [x] `2026-11-677`, `20261167`, dan `202613001` ditolak dengan pesan format.
- [x] Daily Worker join 2026-11 dengan `202611096` → tersimpan. `202611090` ditolak (terakhir 095). `202611700` ditolak karena melewati batas 499.
- [x] Karyawan reguler back-dated (`202512677`, join 2025-12) tidak mengubah deret harian 2026.
- [x] Impor Excel menerapkan aturan yang sama per baris, termasuk nomor ganda dalam satu file (R5).
- [x] Ubah NIP karyawan lama ke bentuk yang salah format → ditolak. Ubah data lain tanpa mengganti NIP → tetap tersimpan, walau NIP lamanya `EMP-…`.
- [ ] Gate hijau: Go `./internal/...`, vitest yang terdampak, dan CI PR.

## Automation Log
- 2026-11-10: Permintaan user. Pola NIP diperiksa di DB staging (read-only). Keputusan user lewat AskUserQuestion: tetap manual + cek format, dua deret, lewat 999 lanjut 4 digit, NIP lama dibiarkan. Rencana ditulis. Branch `feat/nip-format-urut` ditumpuk di atas `fix/impor-karyawan-nip-atasan` (PR #93) karena sama-sama mengubah `employee_import.go`.
- 2026-11-10: **Implementasi Native** di branch `feat/nip-format-urut`.
  - Task 1 (aabe992a): paket murni `platform/nipformat` (Parse, SeriesFor, LastFor, Next, CheckNew, CheckChanged).
  - Task 2 (bbc3a911): `employeekeys.CheckNewNIP/NextNIP/CheckChangedNIP` dipakai `POST /api/users`, `POST /api/hris/employees`, `POST /api/hris/promote`, serta PUT kedua endpoint (cek hanya bila NIP diganti). Tes lama memakai NIP valid.
  - Task 3 (8a6b773f): impor Excel mengecek baris *create*, termasuk nomor ganda dalam satu file (R5). Petunjuk template diperbarui.
  - Task 4 (a0599d5e): placeholder form Tambah/Ubah Karyawan dan dialog Promosi.
  - Ruling pelaksana: cek bentrok NIP sebelum cek format; `internship` masuk deret harian.
- Review akhir (opus): Critical 1, Important 0, Minor 6.
  - Critical: karyawan reguler back-dated merusak deret harian (lihat revisi R2). User memilih batas tetap 499. Diperbaiki di 94862b36, tes `TestBackdatedMainKeepsHarian` dan `TestMainStartsAt500` RED→GREEN.
  - Minor ditunda:
    - perbandingan NIP lama tanpa trim saat edit (0 NIP berspasi di data);
    - kunci `ExistingNIP` impor tanpa btrim;
    - NIP ganda di file mendapat 2 error;
    - dua pembuatan bersamaan beda bulan bisa memakai nomor sama (tanpa advisory lock);
    - join date default promosi memakai tanggal UTC;
    - scan semua NIP per pembuatan (~650 baris).
- Gate:
  - Go: gofmt 0, vet, `./internal/...` 136 ok. `TestIntegrationRedeemBrakeIsSharedAcrossReplicas` (memberportal, `Retry-After` 56 vs 60) gagal saat beban paralel dan lulus saat dijalankan terpisah; tidak terkait.
  - Frontend: vitest 13/13 (dua file terdampak), tsc 0, eslint bersih.
- Staging 11.50 (image `local-20261110-115008`, backup `/mnt/hdd1/backup/paddydev/staging-before-e099-20261110-114956.dump`). Data staging per QA: utama terakhir 676, harian 2026 terakhir 095.
  - Ditolak dengan pesan yang benar: `202611650`, `202610677`, `2026-11-677`, DW `202611090`, DW `202611700` (maks 499), PUT `nip: EMP-X`.
  - Diterima: `202611677` (probation), DW `202611096`, PUT phone saja.
  - Pratinjau impor: nomor 678 dua kali → error baris 3; `202611650` → error.
  - Data dan sesi uji sudah dihapus.

# EPIC-107: Pindah Toko Karyawan Massal — Penempatan, Akun, dan Gaji Sekaligus (`MODULE-HRIS`, `MODULE-CONFIGURATION`)

status: backlog
environment: dev
retries: 0

## Goal
Permintaan user 12-11-2026:

> bulky Branch HR / admin HR karena skg harus satu satu di ubah dan cukup waste time
> pokoknya intinya adalah supaya mudah Update kode branch secara massal melalui template. lalu Nonaktifkan store tutup/ganti kode, histori tetap tersimpan juga mudah dan UI friendly

Keputusan user (AskUserQuestion): **penempatan utama dan cabang akun berubah sekaligus**, lewat template.

## Yang sudah ada (tidak dibangun ulang)
- **Kode & Status Toko** (EPIC-100, halaman Penempatan): template untuk ganti kode, nama, dan aktif/nonaktif toko massal; histori tetap.
  - Menonaktifkan toko **ditolak selama masih ada karyawan aktif** ("pindahkan dulu lewat Penempatan").
- **Pindahkan ke toko** (EPIC-070): pilih banyak karyawan, satu toko tujuan, pratinjau lalu simpan (`POST /api/hris/employee-placements/bulk`, mode `move`).

## Celah yang membuat HR tetap mengubah satu per satu
1. **Pindah penempatan tidak menyentuh akun.** `users.branch_id`, `company_id`, `holding_id`, outlet default, dan `user_warehouses` tetap di toko lama, sehingga akun harus diedit satu per satu di Pengguna.
   - Sejak EPIC-106 ini juga merusak akses: user ber-scope cabang hanya melihat baris yang perusahaannya sama (`scope.RowInScope`).
2. **Satu toko tujuan per aksi.** Menutup atau memecah toko dengan karyawan yang tersebar ke banyak toko butuh banyak aksi.
3. **Pindah lintas perusahaan** (contoh: staf HQ/ERP ke Pudak/Paddy) tidak memindahkan gaji aktif. Payroll memilih karyawan menurut perusahaan gaji (`employee_salary.company_id`), sehingga karyawan tetap digaji perusahaan lama.

## Ruling
- **R1 — Satu aksi "pindah toko" = penempatan utama + akun + gaji.**
  - Penempatan lama diakhiri H-1 (tidak dihapus) dan penempatan utama baru dibuat, dengan aturan EPIC-070 (`domain.PlanPlacementMove`).
  - Akun dan gaji ikut menurut R2–R4.
  - Berlaku untuk **template baru (R6)** dan **"Pindahkan ke toko" yang sudah ada**.
- **R2 — Akun yang ikut** hanya akun karyawan itu (`hris.employees.user_id`) dengan `business_scope = 'branch'` dan role bukan `super_admin`. Nilai barunya:
  - `branch_id` = toko baru;
  - `company_id` = perusahaan toko baru;
  - `holding_id` = holding perusahaan itu.

  Akun ber-scope perusahaan/holding dan karyawan tanpa akun tidak diubah. Ini dilaporkan sebagai PERHATIAN per baris: "akun scope perusahaan — ubah di Pengguna bila perlu".
- **R3 — Outlet.**
  - Outlet default menjadi outlet default yang aktif di toko baru (`configuration.warehouses` dengan `is_default` lebih dulu, lalu yang dibuat paling awal); outlet itu ditambahkan ke `user_warehouses`.
  - Outlet di toko lama dinonaktifkan (`is_active = false`, tidak dihapus).
  - Toko tujuan tanpa outlet aktif membuat baris itu **error**: "Toko <kode> belum punya outlet aktif".
- **R4 — Gaji lintas perusahaan.**
  - Bila perusahaan toko baru berbeda, gaji **aktif** karyawan di perusahaan lama pindah ke perusahaan baru. Gaji tidak aktif adalah riwayat dan tetap.
  - **Error per baris:**
    - `gaji ganda`: sudah ada gaji aktif di perusahaan baru, atau baris dengan tanggal berlaku sama;
    - payroll perusahaan lama berstatus `processing`/`completed` memuat karyawan itu;
    - perusahaan baru punya run yang belum `paid` untuk periode yang sama dengan run perusahaan lama yang memuat karyawan itu.
  - **PERHATIAN, bukan error:** run `draft` perusahaan lama memuat karyawan itu ("hitung ulang draft").
- **R5 — Tanggal berlaku.**
  - Kosong berarti hari ini (WIB).
  - Boleh di masa lalu, asalkan tidak jatuh di periode absensi yang sudah ditutup (`periodContaining`, seperti Data Kerja EPIC-086).
  - **Masa depan:** penempatan dibuat sekarang. Akun dan gaji **ikut pada tanggal itu** lewat antrean `hris.account_branch_syncs`, yang diproses job harian (pola `ScheduledResignLoop`). Karyawan yang dipindah lagi sebelum tanggal itu membatalkan antrean lamanya.
- **R6 — Template "Pindah Toko Massal"**, tombol baru di halaman Penempatan di samping "Kode & Status Toko". Dialognya: unduh, unggah, pratinjau, simpan.
  - **Unduhan:** semua karyawan aktif, atau hanya karyawan satu toko (pilih toko di dialog). Sheet `Pindah Toko`:
    - **terkunci:** NIP, Nama, Departemen, Jabatan, Toko Utama Saat Ini (kode), Perusahaan Saat Ini, Cabang Akun Saat Ini;
    - **diisi:** Kode Toko Baru (dropdown kode toko aktif semua perusahaan), Berlaku Mulai (YYYY-MM-DD), Catatan.
  - **Aturan baris:**
    - Kode Toko Baru kosong berarti baris dilewati;
    - kode toko sama dengan toko utama saat ini berarti "tidak berubah", dilewati;
    - NIP tidak dikenal, NIP ganda di file, karyawan nonaktif, atau kode toko tidak dikenal/nonaktif adalah error per baris.
  - **Batas:** 1.000 baris berisi per file.
- **R7 — Pratinjau dan simpan** (pola EPIC-100 `branch_code_import.go` dan EPIC-070 `runBulkPlacement`).
  - **Pratinjau:**
    - menjalankan penulisan yang sama di dalam transaksi lalu di-rollback;
    - per baris menampilkan toko lama → baru, cabang/perusahaan akun lama → baru, outlet, gaji (pindah/tidak), status (dipindah, dilewati, atau error), alasan, dan PERHATIAN;
    - menyediakan file error.
  - **Simpan:**
    - ditolak bila masih ada error, atau bila fingerprint (isi file + status karyawan terkait) berbeda dari saat pratinjau;
    - berjalan dalam satu transaksi dengan advisory lock;
    - butuh centang "Saya sudah memeriksa peringatan" bila ada PERHATIAN.
  - **Audit per karyawan:**
    - `placement.bulk_move` (sudah ada);
    - `user.branch_sync`, dengan before/after branch, company, holding, dan outlet;
    - `salary.company_move`, dengan before/after company per baris gaji.
- **R8 — Akses** sama dengan Kode & Status Toko: `placementManagers` (HR, admin HR), dan simpan butuh aksi `update`. Pengatur jadwal toko (delegasi EPIC-071) tidak mendapat template. "Pindahkan ke toko" milik delegasi ikut R1–R4 di dalam wilayahnya.
- **R9 — Tutup toko jadi mudah.** Pesan tolak Nonaktif EPIC-100 diubah menjadi "Toko X masih dipakai: … — pindahkan dulu lewat **Pindah Toko Massal**". Dialog Kode & Status Toko menautkan ke dialog Pindah Toko Massal yang sudah tersaring ke toko itu.
- **R10 — Histori tetap.**
  - Tidak ada baris yang dihapus. Absensi, jadwal, dan payroll lama tetap di toko/perusahaan lama.
  - Penempatan lama berakhir H-1. Outlet lama nonaktif.
  - Snapshot before/after ada di audit.

## Tasks
Rencana: `docs/superpowers/plans/2026-11-12-pindah-toko-massal.md`.
- [ ] T1 Keputusan sinkron akun (domain, unit test)
- [ ] T2 Sinkron akun + gaji dalam transaksi (integrasi)
- [ ] T3 "Pindahkan ke toko" ikut sinkron akun/gaji
- [ ] T4 Antrean + job harian untuk tanggal berlaku masa depan
- [ ] T5 Template Pindah Toko Massal (unduh, pratinjau, simpan)
- [ ] T6 UI: dialog Pindah Toko Massal + kolom akun di pratinjau + tautan dari Kode & Status Toko
- [ ] T7 Gate, review akhir, PR ke erp-paddy development, QA staging

## Acceptance Criteria
- [ ] Template (semua / per toko) berisi karyawan aktif dengan toko, perusahaan, dan cabang akun saat ini, plus dropdown kode toko aktif.
- [ ] 3 karyawan Pudak dipindah ke 3 toko berbeda dalam satu file. Penempatan lama berakhir H-1, penempatan utama baru dimulai hari ini, akun ikut toko baru (cabang, perusahaan, holding, outlet), dan absensi lama tetap di Pudak.
- [ ] Staf HQ (ERP) dipindah ke Pudak (PADDY-RETAIL): akun ke PADDY-RETAIL dan gaji aktif ke PADDY-RETAIL. Bila ada di payroll ERP `completed`, baris itu error dengan alasan.
- [ ] Berlaku Mulai = besok: penempatan tercatat sekarang, akun belum berubah, dan job besok mengubah akun dan gaji.
- [ ] Toko tujuan tanpa outlet, NIP tidak dikenal, NIP ganda, dan kode toko nonaktif menjadi error per baris; simpan ditolak sampai bersih.
- [ ] Setelah semua karyawannya dipindah, toko bisa dinonaktifkan lewat Kode & Status Toko.
- [ ] "Pindahkan ke toko" (pilih banyak) juga memperbarui akun dan gaji.
- [ ] Gate hijau: Go `./...` 2× di DB uji baru, vitest, tsc, eslint, dan CI PR.

## Automation Log
- 2026-11-12: Permintaan user. EPIC-070, EPIC-100 dan kode `placements.go`, `branch_code_import.go`, `accounts/input.go`, `scheduled_resign.go` diperiksa. Keputusan user: penempatan + akun sekaligus, lewat template, UI ramah. Rencana ditulis; belum dieksekusi.

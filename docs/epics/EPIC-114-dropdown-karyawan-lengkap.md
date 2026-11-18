# EPIC-114: Dropdown Karyawan Menampilkan Semua Data (`MODULE-HRIS`)

status: review
environment: dev
retries: 0

## Goal
Laporan user 18-11-2026:

> "ada BUG dropdown untuk di tambah salary khusus karyawan tidak tampil semua , sama dengan dropdown karyawan di payroll semua dropdown karyawan pastikan harus semua data tertampil"

## Akar masalah
`useSalaryEmployees` (dropdown Tambah Salary dan Input Manual payroll) memanggil `GET /api/hris/employees` tanpa `limit` dan tanpa paging. Backend memakai default `limit = 20`. Akibatnya hanya 20 dari 648 karyawan (315 aktif) yang tampil.

## Tasks
Rencana: `docs/superpowers/plans/2026-11-18-dropdown-karyawan-lengkap.md` (berisi peta semua dropdown karyawan).
1. Tambah Salary dan Input Manual memuat semua karyawan lewat `fetchAllEmployees`, ditambah tes pemindai pencegah regresi.
2. Pemilih berbasis pencarian (surat peringatan, pengatur jadwal) diberi keterangan cara mencari.
3. Review akhir, PR, dan QA.

## Keputusan
- **K-1:** pemilih berbasis pencarian server tetap berbasis pencarian. Setiap karyawan bisa dicari lewat nama atau NIP, dan saat hasilnya terpotong tampil keterangan cara mencari. Staff pass tiket tidak diberi keterangan karena placeholder-nya sudah meminta mengetik.
- **K-2:** Tambah Salary dan Input Manual memuat karyawan aktif dan nonaktif, sama seperti filter sebelumnya, supaya karyawan resign yang masih digaji prorata tetap bisa dipilih. Karyawan nonaktif diberi label "(nonaktif)".
- Paging diurutkan menurut NIP (unik), lalu nama di klien. Dengan begitu nama kembar di batas halaman tidak hilang atau tampil dua kali.

## Acceptance Criteria
- [x] Tambah Salary dan Input Manual memuat semua karyawan (tes 515 baris, 2 halaman).
- [x] Karyawan nonaktif berlabel "(nonaktif)".
- [x] Tes pemindai gagal bila ada daftar karyawan tanpa paging atau pencarian.
- [x] Pemilih pencarian menampilkan keterangan saat hasil terpotong.
- [ ] QA HR di staging.

## Automation Log
- 2026-11-18: Rencana ditulis; eksekusi Native dengan TDD. Commit di erp-paddy `fix/epic114-dropdown-karyawan`:
  - 0164927: dropdown memuat semua karyawan;
  - bd1d31f: keterangan pencarian;
  - 996ee2b: perbaikan review akhir.
- 2026-11-18: Review akhir oleh reviewer segar (opus): tidak ada temuan kritis, 2 penting, 3 minor. Kedua temuan penting diperbaiki dengan tes RED→GREEN:
  - **I1:** urutan halaman hanya menurut nama, sehingga tidak stabil di batas halaman;
  - **I2:** pemindai melewatkan kutip tunggal, `limit` tetap, dan `employee_id=`.
- **Minor yang ditunda:**
  - karyawan nonaktif bercampur dengan yang aktif di urutan abjad;
  - keterangan pencarian juga muncul bila hasil pencarian tepat 25;
  - sekitar 650 opsi tanpa virtualisasi.
- 2026-11-18: Gerbang: vitest 6874 lulus, tsc 0 error, lint ok. PR [wit-id-indonesia/erp-paddy#17](https://github.com/wit-id-indonesia/erp-paddy/pull/17) ke `development` dibuka. Merge, deploy staging, dan QA HR menunggu izin user. Production tidak disentuh.

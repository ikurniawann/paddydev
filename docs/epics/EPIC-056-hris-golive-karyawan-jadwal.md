# EPIC-056: HRIS Go-Live — Data Karyawan & Jadwal Shift (`MODULE-HRIS`)

status: review
environment: dev
retries: 0

## Goal
Menyiapkan modul Karyawan dan Jadwal Kerja untuk go-live 21-10-2026:
- **Data Karyawan:** export/import (Tambah/Timpa), aksi Resign, email karyawan resign bisa dipakai karyawan baru, audit log.
- **Jadwal Shift:** kode shift, checklist Shift/Non-Shift, jadwal tetap HQ & Warehouse, jadwal per tanggal dengan bulk upload per cabang (7–45 hari) dan validasi.

Payroll tidak termasuk go-live ini.

- Spec: `docs/superpowers/specs/2026-10-19-hris-karyawan-jadwal-golive-design.md`
- Rencana: `docs/superpowers/plans/2026-10-19-hris-karyawan-jadwal-golive.md`

## Keputusan user (2026-10-19)
- **Mode import:**
  - Add = hanya karyawan baru.
  - Replacement (Timpa) = NIP yang ada diperbarui; sel kosong tidak menimpa; karyawan yang tidak ada di file tidak disentuh.
- **Resign:** ada aksi Resign. Karyawan resign hilang dari daftar bawaan dan dari semua pemilih karyawan.
- **Karyawan resign masuk kerja lagi:** dibuat sebagai data baru. Email dilepas dari data lama (`arsip-{nip}@resign.invalid`), dan akun login dipindah ke data baru.
- **Jadwal tetap HQ & Warehouse (Non-Shift):** Senin–Jumat 09.00–18.00, istirahat 60 menit.
- **Shift default:** `DEF` 08.00–17.00 (isi bawaan template).
- **Checklist Shift/Non-Shift:** wajib di data karyawan. Karyawan Non-Shift tidak muncul di Jadwal Kerja. Ada validasi pencegah human error.
- **File Excel:** judul kolom dan isi pilihan memakai bahasa user (tanpa nama field), dengan dropdown, petunjuk, dan file keterangan error.
- **Konfirmasi penggantian:**
  - import mode Timpa menampilkan "N karyawan akan diperbarui" beserta Sebelum → Sesudah;
  - upload jadwal yang mengganti jadwal yang sudah ada menampilkan "N jadwal yang sudah ada akan diganti".

## Tasks
1. Migrasi: kode shift + `DEF`/`NS`, `employees.schedule_type`, `employee_shift_days` + view `v_employee_schedule`, menu Jadwal Kerja.
2. Builder template xlsx (`platform/xlsx/template.go`).
3. Label & parser nilai Excel.
4. Kode shift di master Shift Kerja.
5. Backend tipe jadwal & jadwal tetap.
6. `schedule_type` di `/api/users`.
7. UI Shift/Non-Shift: form, detail, daftar, ubah massal, tab terkunci, Shift Tim.
8. Resign + pemilih cuti hanya karyawan aktif.
9. Clock-in, roster, dan ESS membaca jadwal per tanggal.
10. Validasi file jadwal.
11. Endpoint Jadwal Kerja: grid, template, upload, file error.
12. Halaman Jadwal Kerja.
13. Lepas email.
14. Audit log + tab Riwayat Perubahan.
15. Export.
16. Import.
17. Finalisasi.

## Acceptance Criteria
- [ ] Semua karyawan punya tipe jadwal; Non-Shift tidak muncul di grid/template/upload Jadwal Kerja.
- [ ] Upload jadwal 7–45 hari per cabang dengan preview: error menahan simpan; peringatan wajib dicentang; penggantian jadwal dikonfirmasi.
- [ ] Clock-in memakai jadwal per tanggal untuk menghitung telat.
- [ ] Resign menutup penempatan & jadwal dan memblokir login; karyawan resign hilang dari pemilih.
- [ ] Export → import Timpa tanpa perubahan = 0 karyawan diperbarui.
- [ ] File Excel tanpa nama field database; NIK/NIP tetap teks.
- [ ] Audit perubahan tampil di tab Riwayat Perubahan.

## Tahap 2 (di luar go-live)
- Reader payroll, KPI on-time, dan roster spa membaca `hris.v_employee_schedule`.

## Automation Log
- 2026-10-19 — Spec & rencana disetujui user. Eksekusi native di branch `feat/epic-056-hris-golive`; 17 task, setiap task TDD (test gagal → lulus). Keputusan eksekusi tercatat di ledger `.superpowers/sdd/2026-10-19-hris-karyawan-jadwal-golive/progress.md`. Ringkasan:
  - **Kode shift otomatis:** kode `S{n}` untuk insert lama tanpa kode (migrasi `20261019090400`); API tetap mewajibkan kode.
  - **Jadwal per tanggal:** disatukan lewat view `hris.v_employee_schedule` (priority 0 menang bila tanggal sama), sehingga resolver lama tidak berubah.
  - **Pola mingguan:** ditolak untuk karyawan Non-Shift di backend (bukan hanya di UI).
  - **Import Timpa:** baris tanpa perubahan dihitung "tidak berubah", tidak ikut diperbarui maupun diaudit.
- Kegagalan test lama (sudah ada di `main`, bukan regresi): hris `TestAttendancePeriodClosing`, spa `TestTherapistPayrollTwiceAMonth`.

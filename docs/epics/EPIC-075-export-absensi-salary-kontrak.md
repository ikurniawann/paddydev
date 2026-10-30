# EPIC-075: Export Absensi per Toko, Cari & Export Salary, Bug Kontrak dan Struktur Gaji (`MODULE-HRIS`)

status: on-progress
environment: dev
retries: 0

## Goal
Permintaan user 30-10-2026, tiga hal:
1. **Export absensi** (Super Admin): rekap sudah difilter per toko, tetapi hasil export tetap berisi semua karyawan. Default export tetap semua karyawan, tetapi export harus mengikuti filter. Hasil export juga diberi kolom toko.
2. **Salary Structure**: tambahkan pencarian karyawan dan export. Nama tidak tersusun menurut abjad, jadi sulit dicari.
3. **Bug**: tambah struktur gaji terhalang. Kontrak baru tidak bisa ditambahkan ("Data sudah ada di sistem"), jadi user tidak bisa menambah struktur gaji.

Penyelidikan (kode `main` `36b374f5`, data staging read-only):
- **Export absensi.** `exportAttendanceCsv` (`frontend/src/features/hris/attendance/api.ts`) hanya mengirim karyawan dan periode. Filter toko (`branch_id`) dan "hanya terlambat" (`is_late`) di tab Rekap tidak ikut. Server (`exportFilter`, `AttendanceExport`) juga tidak mengenalnya. Filter toko di tabel memakai `hris.attendance.branch_id` (toko tempat absen; `none` = tanpa toko).
- **Salary.** Server mengurutkan `effective_date DESC`. Halaman tanpa pencarian dan tanpa export; hanya menampilkan versi aktif per entitas.
- **Kontrak.** `insertNumbered` memberi nomor `jumlah kontrak jenis itu tahun ini + 1`, dengan satu kali coba ulang yang menghitung angka yang sama. Di staging: PKWT 2026 berjumlah 567 (566 impor `ODOO-…` + 1 manual), sehingga nomor berikutnya `0568/PKWT/X/2026`. Nomor itu sudah dipakai sebuah draf, karena dulu ada kontrak yang dihapus. Akibatnya setiap PKWT baru gagal dengan 23505 "Data sudah ada di sistem". Reproduksi pasti.
- **Struktur gaji.** `createSalary` menonaktifkan versi aktif lalu menyisipkan versi baru **tanpa transaksi**. Bila penyisipan gagal (unik `employee_id, company_id, effective_date`, misalnya tambah ulang dengan tanggal efektif sama → "Data sudah ada di sistem"), versi lama sudah terlanjur nonaktif dan karyawan **tidak punya gaji aktif**. Inilah sebab kasbon ditolak "belum punya struktur gaji aktif". Di staging ada 26 karyawan dengan baris gaji yang semuanya nonaktif.

## Ruling (dibuat tanpa tanya; user minta langsung direncanakan)
- **R1** Export (CSV/Excel/PDF) mengikuti filter yang sama dengan tabel Rekap: karyawan, periode, **toko** (termasuk "Tanpa toko") dan **hanya terlambat**. "Semua toko" = semua karyawan, seperti sekarang.
- **R2** Hasil export mendapat kolom **Toko** (`<kode> - <nama>` toko tempat absen, `-` bila kosong) setelah kolom Jabatan, di CSV, Excel, dan PDF. Judul Excel/PDF menyebut toko yang difilter.
- **R3** Daftar Salary Structure diurutkan nama A–Z (lalu NIP).
- **R4** Kotak cari nama/NIP di atas tabel. Pencarian berlaku pada tab entitas yang terbuka.
- **R5** Tombol **Export Excel** mengunduh baris yang sedang tampil (entitas terbuka, sesudah pencarian, urut nama). Kolom: NIP, Nama, Jabatan, Departemen, Entitas, Gaji Pokok, Tunjangan Tetap, Transport, Makan, Tunjangan Jabatan, Tunjangan Kesehatan, Status PTKP, Efektif. Nama file `struktur-gaji-<entitas>-<YYYY-MM-DD>.xlsx`. Dibuat di browser (`exceljs-safe`), tanpa endpoint baru: halaman ini sudah khusus pemegang menu Salary.
- **R6** Nomor kontrak berikutnya = **nomor manual terbesar** jenis itu tahun ini + 1. Hanya nomor berpola `NNNN/<JENIS>/<bulan romawi>/<tahun>` yang dihitung; nomor impor `ODOO-…` diabaikan. Bila bentrok, coba lagi dengan nomor berikutnya, sampai 5 kali.
- **R7** Tambah struktur gaji berjalan dalam **satu transaksi**: bila gagal, versi lama tetap aktif.
- **R8** Bila sudah ada struktur gaji untuk karyawan + entitas + tanggal efektif yang sama, server menjawab 409 dengan pesan jelas tanpa mengubah apa pun: "Struktur gaji karyawan ini untuk entitas tersebut dengan tanggal efektif <tanggal> sudah ada. Ubah lewat Edit atau pilih tanggal efektif lain."
- **R9** Data yang sudah terlanjur rusak (karyawan tanpa gaji aktif) **tidak** diperbaiki otomatis oleh migrasi. Daftar read-only untuk HR menyusul atas izin user.

## Tasks
Rencana: `docs/superpowers/plans/2026-10-30-export-absensi-salary-kontrak.md`.

## Acceptance Criteria
- Rekap absensi difilter toko X lalu Export (CSV/Excel/PDF): isinya hanya absen di toko X, dengan kolom Toko. Tanpa filter toko: semua karyawan, dengan kolom Toko.
- Salary Structure: urut nama A–Z, bisa dicari nama/NIP, dan Export Excel mengunduh baris yang tampil.
- Kontrak PKWT/PKWTT baru bisa dibuat walau jumlah baris tidak sama dengan nomor terbesar (kasus staging `0568/PKWT/X/2026`).
- Tambah struktur gaji yang gagal tidak menonaktifkan versi lama. Tanggal efektif kembar → pesan R8.
- go test, pnpm test, lint, tsc hijau; CI 4/4; QA staging.

## Automation Log
- 2026-10-30: Epic dibuat. Penyelidikan kode dan data staging (read-only). Reproduksi nomor kontrak bentrok di staging.

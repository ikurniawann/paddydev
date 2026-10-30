# EPIC-075: Export Absensi per Toko, Cari & Export Salary, Bug Kontrak dan Struktur Gaji (`MODULE-HRIS`)

status: ready-for-qa
environment: production
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
- **R10** Draft kontrak otomatis saat **promosi kandidat** (Rekrutmen) memakai penomoran yang sama (`hris.MaxContractSeq`, maksimal 5 percobaan). Sebelumnya jalur itu masih memakai jumlah+1 dan gagal dengan bug yang sama.
- **R11** Nomor berawalan lebih dari 9 digit tidak ikut dihitung, supaya satu baris aneh tidak memblokir penomoran setahun.
- **R12** Di PDF, "Toko: …" berada di baris sendiri (dipotong satu baris) dan tabel turun 12 pt, supaya nama toko panjang tidak menimpa header tabel.
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
- 2026-10-30: **Implementasi** (eksekusi inline):
  - `16dd0499`: export server.
  - `7cc09706`: export frontend.
  - `3f7c7d27`, `70112bdb`: Salary.
  - `477853d0`: nomor kontrak.
  - `95e58153`: struktur gaji dalam transaksi.
- 2026-10-30: **Review akhir** (opus): Ready with fixes, 0 kritis. Diperbaiki di `f21969fd` (R10–R12).
  - Ditunda (minor): menghapus draft bernomor tertinggi membuat nomor itu dipakai lagi oleh kontrak berikutnya (nomor tetap unik di antara kontrak yang ada).
  - Ditunda (minor): pesan R8 menyarankan "Ubah lewat Edit" juga bila versi bertanggal sama sudah nonaktif.
- 2026-10-30: **Gerbang lokal:** `go vet ./...` bersih; `go test -p 1` app/hris/payroll/recruitment/platform ok; frontend `tsc` 0, `lint` 0, `pnpm test` 872 file / 6313 tes lulus.
- 2026-10-30 17:43: **Staging** image `local-20261030-174347` (branch `f21969fd`), tanpa migrasi. Backup `~/Desktop/paddydev-backups/staging-db-before-epic075-20261030-173419.dump`.
- 2026-10-30: **QA staging** (akun `qa075-hr` hrd + `hris_manager`, karyawan demo), 18/18 lulus.
  - Export Oktober: semua toko 3.975 baris dengan kolom Toko; toko ODOO-WAREHOUSE-PADDY 874 baris, satu label toko. Excel 200.
  - Kontrak PKWT baru → `0569/PKWT/X/2026`. Sebelumnya bentrok dengan draf `0568`.
  - Gaji kembar → pesan R8; versi pertama tetap aktif.
  - Salary: 135 baris urut A–Z, pencarian, unduh `struktur-gaji-paddy-2026-10-30.xlsx` (laptop dan HP); tidak meluber; 0 error konsol / API.
  - Data demo (akun, karyawan, kontrak, gaji) dihapus.
- 2026-10-30: **PR #56 merged** (`ded5c888`), CI 4/4 hijau.
- 2026-10-30: **Catatan tertunda:**
  - `48409760`: tanggal efektif yang sama dengan versi gaji lama yang sudah **tidak aktif** kini mendapat pesan sendiri ("… sudah dipakai versi struktur gaji lama yang sudah tidak aktif. Pilih tanggal efektif lain."), bukan saran "Ubah lewat Edit".
  - Nomor kontrak dipakai lagi bila draft bernomor tertinggi dihapus: **diterima**. Nomor tetap unik di antara kontrak yang ada (constraint UNIK), dan draft yang dihapus tidak punya dokumen resmi.
- 2026-10-30 20:02 WIB: **Deploy production** bersama EPIC-073, 074, 075 + PR #57 (catatan tertunda) + PR #58 (tab HP & redirect menu 404), atas izin eksplisit user ("Ya, deploy prod").
  - Image `local-20261030-200228` = main `589ac8c8`. Migrasi `20261030120000_ess_team_attendance_menu.sql` diterapkan (566 migrasi, verify 0 beda; menu Absensi Tim grant 26 peran). Container healthy, log API 0 error, absensi tetap masuk, 21 pengatur utuh, redirect `kpi-templates` → `kpi-config` dan `/dashboard/shop` → `/dashboard/shop/orders` aktif.
  - Backup `~/backups/prod-db-before-main-e073-075-20261030-200119.dump`. Source lama `~/paddydev-src.before-main-e073-075-20261030-200119` (= `28721f40`). Rollback: kembalikan folder source itu lalu jalankan `tools/deploy-paddydev.sh`. Migrasi menu boleh tetap ada: menunya tersembunyi bagi akun tanpa bawahan.
  - Status tetap `ready-for-qa` sampai HR/atasan mencoba di production (tanpa sesi uji di production).

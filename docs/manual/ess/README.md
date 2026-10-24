# Manual Book ESS Karyawan (Area Karyawan)

Panduan bergambar untuk karyawan toko dan kantor: masuk, absen masuk/pulang,
di luar area toko, riwayat absensi, izin & cuti, pinjaman & kasbon,
perjalanan dinas, lembur, slip gaji, dan tanya jawab. Semua gambar adalah
screenshot asli aplikasi (tampilan HP 390×844, skala 2×) dengan data demo
fiktif.

| File | Isi |
|------|-----|
| `manual.html` | Sumber manual (sampul, daftar isi, 11 bagian, CSS cetak A4). |
| `screens/*.png` | Screenshot hasil `capture.cjs` (jangan diedit manual). |
| `capture.cjs` | Script Playwright: satu fungsi per bagian, membuat `screens/`. |
| `build-pdf.cjs` | Membuat PDF A4: sampul + isi bernomor halaman + daftar isi otomatis. |
| `seed-demo.sql` | Data demo di DB **test**: Rina Lestari (staf) dan Andi Pratama (Store Manager, atasan Rina). |
| `cleanup-demo.sql` | Menghapus semua data demo dan mengecek sisa = 0. |

## Aturan penting

- Hanya pakai **DB test** (`paddy_test`, port 55499). Jangan pernah ke DB live
  atau port 55432. `capture.cjs` menolak `TEST_DB` lain.
- Password akun demo **tidak** disimpan di repo. Buat acak, simpan di file
  `chmod 600` di luar repo, hapus setelah selesai.
- Jalankan `cleanup-demo.sql` sebelum seed ulang dan setelah selesai.

## Membuat ulang semua gambar dan PDF

Butuh: `psql`, poppler-utils (`pdftotext`, `pdfunite`, `pdfinfo`, `pdftoppm`),
dan folder `frontend` yang punya `node_modules` (Playwright + Chromium, sharp,
bcryptjs). Bila menjalankan dari worktree tanpa `node_modules`, set
`FRONTEND_DIR` ke folder `frontend` checkout utama.

```bash
cd docs/manual/ess
export TEST_DB='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable'
export FRONTEND_DIR=/home/wit/Desktop/paddydev/frontend
SCRATCH=$(mktemp -d)

# 1. Password demo (acak, hanya di file sementara)
( umask 077; node -e "console.log(require('crypto').randomBytes(12).toString('base64url'))" > "$SCRATCH/pass.txt" )
HASH=$(node -e "const b=require(process.env.FRONTEND_DIR+'/node_modules/bcryptjs');console.log(b.hashSync(require('fs').readFileSync(process.argv[1],'utf8').trim(),10))" "$SCRATCH/pass.txt")

# 2. Data demo
psql "$TEST_DB" -f cleanup-demo.sql
psql "$TEST_DB" -v pw_hash="$HASH" -f seed-demo.sql

# 3. Server lokal ke DB test (dua terminal / background)
#    Go API (dari backend/):
#      DATABASE_URL="$TEST_DB" PORT=8197 APP_ENV=development TZ=Asia/Jakarta STORAGE_DIR="$SCRATCH/storage" go run ./cmd/api
#    Next dev (dari frontend/):
#      DATABASE_URL="$TEST_DB" BACKEND_URL=http://127.0.0.1:8197 NEXT_PUBLIC_APP_URL=http://localhost:3497 TZ=Asia/Jakarta npx next dev -p 3497
#    Buka lewat http://localhost:3497 (bukan 127.0.0.1). Muat halaman pertama kali bisa ±3 menit (kompilasi).
#    STORAGE_DIR mengarahkan foto selfie demo ke folder sementara.

# 4. Screenshot (semua bagian, atau sebut nama bagian)
DEMO_PASSWORD_FILE="$SCRATCH/pass.txt" node capture.cjs
DEMO_PASSWORD_FILE="$SCRATCH/pass.txt" node capture.cjs cuti lembur   # sebagian saja

# 5. PDF
node build-pdf.cjs /path/Manual-Book-ESS-Karyawan-Paddy-v2.pdf
pdftoppm -r 50 -png /path/Manual-Book-ESS-Karyawan-Paddy-v2.pdf /tmp/cek   # periksa tiap halaman

# 6. Bereskan
psql "$TEST_DB" -f cleanup-demo.sql      # baris terakhir harus 0 semua
# hentikan Go API dan Next dev (kill PID yang mendengarkan port 8197 dan 3497)
rm -rf "$SCRATCH"
```

Nama bagian untuk `capture.cjs`: `masuk`, `absen-masuk`, `luar-area`,
`absen-pulang`, `riwayat`, `cuti`, `pinjaman`, `perjalanan-dinas`, `lembur`,
`slip-gaji`, `masalah`.

Catatan:

- `absen-masuk` dan `absen-pulang` benar-benar absen untuk Rina hari ini,
  `luar-area` absen untuk Andi, `pinjaman` membuat Andi menyetujui kasbon.
  Mengulang bagian itu butuh cleanup + seed ulang.
- `luar-area` mengisi koordinat Paddy Store PVJ sementara (-6.889221,
  107.595821, radius 250 m) lalu mengembalikannya ke NULL, juga bila gagal.
- Seed butuh minimal ±14 hari kerja di periode absensi yang belum di-closing.
  Bila gagal di awal bulan, jalankan setelah tanggal ±17 atau buka periode
  bulan lalu di DB test.
- Hasil absen bergantung jam saat capture (sebelum 09:10 tampil "Tepat
  waktu"). Data lain dihitung relatif dari hari ini.
- Callout oranye digambar oleh `mark()` di `capture.cjs` (kotak + lencana
  nomor). Nomor di gambar harus sama dengan nomor di `manual.html`.

## Memperbarui manual

**Jenis cuti baru** (mis. ditambah di `frontend/src/lib/hris/leave-types.ts`
dan `backend/internal/modules/hris/domain/leave_types.go`):

1. Jalankan ulang `capture.cjs cuti` supaya `06-jenis.png` memuat jenis baru.
2. Tambah baris di tabel jenis pada Bagian 6 `manual.html` (kuota, potong gaji,
   siapa yang boleh).
3. Bila jenis itu punya catatan khusus di form (seperti Izin Pribadi), tambah
   satu shot di fungsi `cuti()` dan satu blok `.blk` di `manual.html`.

**Bagian baru**:

1. Tulis fungsi `async function namaBagian(browser)` di `capture.cjs` (pola:
   `newContext` → `open` → `shot(page, "NN-nama", region, [callout...])`),
   lalu daftarkan di `SECTIONS`.
2. Tambah `<section class="bab">` di `manual.html` dengan penanda
   `<div class="kicker">BAGIAN NN</div>` dan baris daftar isi
   `<span class="pg" data-toc="NN"></span>`. Nomor halaman diisi otomatis.
3. Butuh data? Tambah di `seed-demo.sql` (tanggal relatif `CURRENT_DATE`) dan
   pastikan `cleanup-demo.sql` menghapusnya (sapu-bersih kolom uuid sudah
   menangkap baris yang merujuk id demo).

**Versi baru**: ganti teks `Versi 2 · 21 Oktober 2026` di sampul dan `Versi 2`
di footer (`build-pdf.cjs`), lalu nama file PDF.

## Temuan saat membuat versi 2 (kode main 5035ae5)

- Halaman Izin & Cuti di HP: bila ada pengajuan **Ditolak** dengan alasan,
  kartu kuota dan riwayat melebar keluar layar (item grid tanpa `min-w-0`),
  tombol dan status terpotong. `capture.cjs` menambahkan CSS sementara
  (`main .grid > div { min-width: 0 }`) hanya untuk foto; hapus setelah
  aplikasi diperbaiki.
- Header Beranda di HP memotong nama dan jabatan karyawan ("Rin…", "Sales …")
  karena kolom status/NIP/masa kerja di kanan (terlihat di `01-beranda.png`).
  Sampul memakai `00-sampul.png` tanpa header.

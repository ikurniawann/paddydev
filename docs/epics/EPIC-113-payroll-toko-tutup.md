# EPIC-113: Payroll saat Toko Tutup di Tengah Periode (`MODULE-HRIS`)

status: review
environment: dev
retries: 0

## Goal
Pertanyaan user 18-11-2026:

> "ketika akan payroll tapi retail / toko di tengah periode berjalan itu nonaktif apa bisa terhitung gaji tiap jadwal karyawan yang telah ter assign?"

Toko bisa dinonaktifkan dengan **tanggal tutup**. Jadwal dan absen sebelum tanggal itu tetap bisa dikoreksi. Karyawan yang tertinggal di toko tutup digaji **prorata** sampai sehari sebelum tutup, dan slipnya diberi **keterangan**.

## Tasks
Rencana: `docs/superpowers/plans/2026-11-18-payroll-toko-tutup-tengah-periode.md`. Kode di erp-paddy branch `feat/epic113-toko-tutup-payroll`.
1. **Tanggal tutup toko.** Kolom `configuration.branches.inactive_from` dan form Struktur Bisnis "Tanggal tutup". Fallback Next ikut diperbarui.
2. **Toko tetap terbuka sebelum tanggal tutup.** Berlaku untuk absen masuk, `GET /api/hris/branches?on=`, cakupan pengatur jadwal, dan Jadwal Kerja. Jadwal, koreksi absen, dan penempatan pada atau sesudah tanggal tutup ditolak.
3. **Domain payroll.** Tanggal sesudah tutup dikeluarkan dari cakupan bila karyawan tidak punya penempatan di toko lain yang buka.
4. **Wiring dan tampilan.** Kalkulasi run, kolom `payroll_details.store_closure_note`, baris "Keterangan" di slip PDF/web, dan peringatan `store_closures` di ringkasan kalkulasi.
5. **Verifikasi, PR, dan catatan.**

## Keputusan
- **Dasar:** sebelum EPIC-113, menonaktifkan toko hanya membalik flag. Payroll tetap menghitung semua jadwal, dan pokok `monthly` dibayar penuh. Tetapi absen di toko itu tidak bisa lagi, sehingga makan/transport per hari hilang. Toko juga hilang dari semua pilihan untuk semua tanggal.
- **K-1 (diputuskan user 18-11):** hari terjadwal sesudah toko tutup **tidak dibayar**. Slip diberi keterangan, dan gaji dibayar **prorata**.
- **K-2 (diputuskan user 18-11):** toko punya **tanggal tutup**. Untuk tanggal sebelum itu, toko tetap muncul di koreksi absen, jadwal, dan delegasi; sesudahnya toko hilang.
- **Kapan karyawan dianggap tertinggal:** ia punya penempatan di toko itu yang masih berjalan pada H-1 tutup. Tanggal yang tidak dibayar adalah tanggal sejak tutup yang tidak tercakup penempatan di toko lain yang buka. Akibatnya:
  - karyawan multi-toko tidak diprorata selama toko lain masih buka;
  - karyawan yang dipindah dengan jeda hanya kehilangan hari jedanya.
- **Toko nonaktif lama** (tanpa tanggal) tetap berperilaku seperti dulu: tertutup untuk semua tanggal dan tanpa prorata.
- **Aturan tanggal tutup:** tidak boleh sesudah hari ini (WIB) dan tidak boleh jatuh di periode absensi yang sudah ditutup. Mengaktifkan kembali toko mengosongkan tanggal tutup. Menyimpan ulang toko nonaktif tanpa tanggal baru tidak mengubah tanggalnya.

## Acceptance Criteria
- [x] Toko dinonaktifkan dengan tanggal; default hari ini; tanggal masa depan dan tanggal di periode closed ditolak.
- [x] Absen, koreksi absen, jadwal, dan delegasi tetap bisa memakai toko untuk tanggal sebelum tutup, dan ditolak mulai tanggal tutup.
- [x] Karyawan tertinggal diprorata (contoh uji: 14/31 hari, 3.100.000 → 1.400.000), slip memuat "Keterangan", dan HR melihat peringatan di kalkulasi.
- [x] Karyawan multi-toko atau yang dipindah tepat waktu tidak diprorata.
- [x] Toko diaktifkan kembali, lalu hitung ulang draft: tidak lagi diprorata.
- [ ] QA HR di staging.

## Automation Log
- 2026-11-18: Rencana ditulis setelah investigasi kode `origin/development`. K-1 dan K-2 diputuskan user. Eksekusi Native dengan TDD per task:
  - 7ec0c74: tanggal tutup toko;
  - 92be2b1: HRIS terbuka sebelum tanggal tutup;
  - 35a1e4d: domain payroll;
  - 2d80fd5: wiring, slip, dan peringatan.
- 2026-11-18: Hasil gerbang:
  - Go `go test ./...`: 143 paket ok.
  - tsc: 0 error. Lint: ok.
  - vitest: 6879 lulus. Satu gagal, `backend-routes.test.ts`, untuk route request-orders. Kegagalan itu sudah merah di base 71537df dari commit lain.
- 2026-11-18: Review akhir oleh reviewer segar (opus): tidak ada temuan kritis, 3 penting, 7 minor. Ketiga temuan penting diperbaiki di 95b9a9a, masing-masing dengan tes RED→GREEN:
  - **I1:** roster per tanggal yang berhenti sebelum tanggal tutup membuat keterangan tertulis "0 hari". Sekarang dihitung hari kalender ("n hari sesudahnya").
  - **I2:** impor Kode & Status Toko melewati `inactive_from`, sehingga mengaktifkan kembali toko bertanggal gagal. Sekarang impor ikut mengisi atau mengosongkan tanggal tutup.
  - **I3:** hari yang tercatat hadir sesudah toko tutup ikut tidak dibayar. Sekarang hari itu tetap dibayar (ruling).
  
  Gerbang sesudah perbaikan: Go 143/143 ok.
- **Minor yang ditunda:**
  - resign: tanggal sesudah `end_date` ikut terhitung di keterangan;
  - guard penempatan hanya memeriksa `start_date`;
  - guard jadwal berjalan di commit, bukan di preview;
  - regex tanggal di fallback Next menerima 30 Feb;
  - baris PDF "Hari tidak dibayar" tidak memuat hari tutup;
  - karyawan yang pindah ke HQ tanpa penempatan HQ terbaca "tertinggal";
  - format bulan duplikat.
- **Ruling verifikasi:** paritas di salinan production diganti argumen kode. Production belum punya toko nonaktif, jadi `BranchClosures` kosong dan `StoreClosedDates` kosong untuk semua karyawan. `BuildInputRange` lalu identik dengan sebelumnya, dan hal ini dikunci `TestBuildInputNoClosureUnchanged`. Paritas data nyata ikut di gladi wajib sebelum rilis production berikutnya.
- 2026-11-18: PR [wit-id-indonesia/erp-paddy#16](https://github.com/wit-id-indonesia/erp-paddy/pull/16) ke `development` dibuka. Merge, deploy staging, dan QA HR menunggu izin user.
- Production tidak disentuh. Production per 16-11 belum punya toko nonaktif, jadi tidak ada backfill.

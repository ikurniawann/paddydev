# EPIC-126: Semua List Lembur Tampil Utuh, dengan Cari dan Halaman (`MODULE-HRIS`)

status: review
environment: dev
retries: 0

## Goal
HR, karyawan, dan atasan melihat semua lembur di setiap list lembur. List tidak lagi memotong baris lama, dan tampilan dibagi per halaman dengan filter.

Rencana: [2026-11-21-list-lembur-tidak-terpotong.md](../superpowers/plans/2026-11-21-list-lembur-tidak-terpotong.md).

## Analisa (read-only, 21-11-2026)
- **Production:** HRIS → Lembur bulan November menampilkan 200 dari 281 lembur. Go mengurutkan dari tanggal terbaru dan memotong di `LIMIT 200`, jadi 81 baris terlama hilang dari layar: semua tanggal 1–5 Nov (65 baris) dan 16 baris tanggal 6 Nov.
- **Staging:** hanya 114 lembur November, sehingga tidak ada yang terpotong.
- **Contoh Sri Wardani:**
  - ESS menampilkan lembur 1 dan 3 Nov.
  - Di list HR keduanya ada di urutan ke-279 dan ke-249, jadi tidak tampil.
  - Slip November tetap memuat 25 jam (13 jam biasa, 12 jam libur), karena payroll membaca tabel langsung, bukan list.
- **Batas bawah list bergeser:** setiap pengajuan baru menyembunyikan satu baris lama lagi. Baris terakhir di screenshot 19.36 (Kania Dewi Lastari, 6 Nov) turun ke urutan 203 setelah 3 pengajuan masuk.
- **Isi baris yang tersembunyi:** 74 `approved` dan 7 `cancelled`, tanpa `pending`. Data dan payroll aman; yang hilang hanya tampilannya (Detail/Koreksi tidak bisa dibuka).

## Keputusan user (21-11-2026)
| No | Permintaan | Keputusan |
| --- | --- | --- |
| 1 | Cara eksekusi | "Native, cepat" |
| 2 | Cakupan | "pastikan semua list data lembur terlihat semua jangan ada lagi yang terpotong" → batas baris dicabut di semua list lembur, termasuk tanpa filter bulan |
| 3 | UI | "tambahkan filter dan pagination jika belum ada" |
| 4 | Rilis | "jika sudah deploy aman dan tes QA aman deploy ke production jangan lupa QA juga dan backup" |

## Tasks
- [x] Task 1: list lembur per bulan tidak dipotong (Go `OvertimeList` + fallback Next); tes `TestOvertimeListMonthNotCapped` (a46fc34)
- [x] Task 1b: semua batas dicabut (3145b80):
  - `OvertimeList` tanpa bulan (200);
  - antrean Persetujuan Tim `TeamPendingOvertime` (200);
  - riwayat lembur tim `TeamOvertimeHistory` (300);
  - fallback Next;
  - tes `TestTeamOvertimeListsNotCapped`.
- [x] Task 3: cari dan halaman di klien (7a2ec29):
  - HR Lembur 25 per halaman, dengan cari nama/NIP/alasan;
  - Riwayat Lembur ESS 10 per halaman, dengan filter bulan;
  - lembur di Riwayat Tim 10 per halaman.
- [x] Task 2: epic dan registry

## Acceptance Criteria
- [x] `GET /api/hris/overtime` (HR per bulan, HR tanpa bulan, ESS `employee_id=me`, atasan `scope=approvals`) mengembalikan semua baris yang cocok, tanpa batas.
- [x] Antrean Persetujuan Tim dan Riwayat Tim memuat semua lembur bawahan.
- [x] Rentang bulan tetap bulan kalender (31 Okt dan 1 Des tidak ikut November).
- [x] HR Lembur, Riwayat Lembur ESS, dan Riwayat Tim membagi tampilan per halaman. Mengubah filter kembali ke halaman 1.
- [ ] QA staging: list November HR menampilkan semua baris di staging.
- [ ] QA production: list November HR menampilkan 281+ baris termasuk 1–5 Nov (contoh Sri Wardani 1 dan 3 Nov).

## Di luar cakupan
- Widget Beranda ESS "5 lembur terbaru" tetap 5: itu ringkasan, dan riwayat lengkap ada di menu Lembur.
- Pengajuan Absensi (`attendance_requests.go`) masih `LIMIT 200` tanpa filter bulan (production 471 baris, pending selalu di atas). Ini follow-up terpisah.

## Automation Log
- 21-11-2026: analisa production/staging read-only, plan ditulis, eksekusi native di branch `erp-paddy` `fix/epic126-list-lembur-tidak-terpotong` (base `bdd86d7`).
  - Tes paket `hris` di DB tes sementara: baseline lulus; tes baru RED → GREEN; paket lulus.
  - Vitest penuh 7089 lulus; tsc dan eslint bersih.
  - Tanpa migrasi.

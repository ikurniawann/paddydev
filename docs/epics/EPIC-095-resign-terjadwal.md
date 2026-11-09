# EPIC-095: Resign Terjadwal — Tanggal Efektif di Masa Depan (`MODULE-HRIS`)

status: done
environment: dev
retries: 0

## Goal
Laporan bug user (06-11-2026):

> ada bug ketika akan mengset tgl resign karena tgl 20 nya belum terjadi, jadi belum bisa di resign-kan di sistem, di sistem muncul-nya hanya tgl hari aja yg bisa diisi, tanggal mendatang tdk bisa di set

**Penyebab** (main `45305bdd`):
- **Server.** `ResignEmployee` (`backend/internal/modules/hris/employee_resign.go`) menolak tanggal setelah hari ini, karena resign langsung menonaktifkan karyawan, menutup penempatan dan jadwal, serta memblokir login. Penolakan ini disengaja sejak EPIC-056 (review I8).
- **UI.** Dialog Resign (`frontend/src/features/users/components/resign-dialog.tsx`) memasang `max = hari ini`.

HR perlu mencatat resign begitu karyawan mengajukan, dengan tanggal efektif di masa depan, tanpa harus menunggu sampai hari terakhir kerja.

## Ruling
- **R1 — Arti tanggal efektif tidak berubah:** tanggal efektif = **hari terakhir kerja**.
  - Tanggal ≤ hari ini (WIB): resign langsung, persis seperti sekarang.
  - Tanggal > hari ini: **resign terjadwal**.
- **R2 — Batas jadwal:** paling jauh **180 hari** dari hari ini. Lebih dari itu → 400 "Tanggal efektif paling jauh 180 hari dari hari ini".
- **R3 — Saat dijadwalkan,** yang terjadi hanya dua hal:
  - satu baris di tabel baru `hris.scheduled_resignations` (tanggal, jenis, alasan, dibuat oleh);
  - audit `employee.resign_schedule`.

  Karyawan tetap aktif, login tetap jalan, dan penempatan, jadwal, serta riwayat kepegawaian tidak berubah sampai jadwal dijalankan. Pesan respons: "Resign dijadwalkan efektif <tgl>".
- **R4 — Eksekusi otomatis.** Job latar berjalan tiap 5 menit (pola `AutoCheckoutLoop`). Jadwal dengan tanggal efektif **sebelum hari ini (WIB)** dijalankan.
  - Langkahnya sama persis dengan resign langsung: status, nonaktif, `end_date`, penempatan/jadwal ditutup, riwayat kepegawaian, login diblokir, audit `employee.resign`.
  - Tanggal efektif = tanggal jadwal. Aktor audit "Sistem (resign terjadwal)". `recorded_by` riwayat = karyawan pembuat jadwal.
  - Hasilnya: karyawan masih bisa absen pada hari terakhirnya, lalu nonaktif mulai pukul 00.00 WIB hari berikutnya.
- **R5 — Satu jadwal aktif per karyawan.**
  - Resign (langsung maupun terjadwal) saat masih ada jadwal aktif → 400 "Sudah ada resign terjadwal efektif <tgl> — batalkan jadwal itu dulu".
- **R6 — Batalkan jadwal.**
  - Dilakukan oleh HR dengan aksi `update` yang sama dengan resign.
  - Alasan opsional; tercatat di audit `employee.resign_schedule_cancel`.
  - Baris jadwal tetap ada dengan `cancelled_at` terisi.
- **R7 — Kasus tepi saat job berjalan.**
  - (Review akhir) Jadwal yang masih menunggu dibatalkan oleh trigger `trg_cancel_scheduled_resign` begitu karyawan dinonaktifkan lewat jalur apa pun (alasan "Karyawan dinonaktifkan"), supaya rehire sesudahnya tidak ikut di-resign. Jadwal yang sudah jatuh tempo tetapi ditolak validasi (mis. tanggal masuk diubah) dibatalkan dengan pesan validasinya, tidak diulang.
  - Karyawan sudah nonaktif lewat cara lain sebelum jadwal dijalankan → jadwal ditandai batal dengan alasan "Karyawan sudah nonaktif", tanpa perubahan lain.
  - Satu jadwal yang gagal tidak menghentikan jadwal lain; galatnya di-log.
- **R8 — UI.**
  - Dialog Resign mengizinkan tanggal sampai hari ini + 180. Teks peringatan mengikuti tanggal: langsung (merah, seperti sekarang) atau terjadwal (biru: "Karyawan tetap aktif sampai <tgl>. Akun dinonaktifkan dan login diblokir otomatis pada <tgl+1> 00.00 WIB."). Tombol berbunyi "Resign" atau "Jadwalkan resign".
  - Profil karyawan menampilkan banner "Resign terjadwal: <jenis> efektif <tgl> (hari terakhir kerja)" dengan tombol "Batalkan jadwal" (hanya HR).
  - Selama ada jadwal, tombol Resign di header tidak tampil.
- **R9 — Payroll tidak diubah.** Setelah jadwal dijalankan, karyawan nonaktif dengan `end_date` ikut run dan diprorata sampai `end_date` (EPIC-059). Bila payroll dihitung sebelum jadwal dijalankan, slipnya masih penuh, jadi HR menghitung ulang setelah tanggal efektif. Catatan ini masuk teks banner.
- **R10 — Base `main`.** Fix ini tidak bergantung pada tumpukan PR #75–#82.

## Tasks
Lihat rencana `docs/superpowers/plans/2026-11-06-resign-terjadwal.md`.

## Acceptance Criteria
- [x] HR memilih tanggal efektif mendatang (mis. 20 Nov) → tersimpan sebagai jadwal; karyawan tetap aktif dan bisa login serta absen sampai tanggal itu.
- [x] Sehari setelah tanggal efektif, karyawan otomatis nonaktif, login diblokir, serta penempatan dan jadwal sesudah tanggal itu ditutup, sama dengan resign langsung.
- [x] Profil menampilkan jadwal resign; HR bisa membatalkannya.
- [x] Resign dengan tanggal hari ini atau lampau tetap berjalan seperti sebelumnya (tes lama hijau).
- [x] `go test`, `pnpm test`, `pnpm lint`, dan `tsc` hijau; CI hijau; QA staging lulus.

## Automation Log
- 2026-11-06: **Laporan bug user.** Penyebab dipetakan (validasi server EPIC-056 I8 + `max` di dialog). Rencana ditulis; status `backlog`, menunggu review user.
- 2026-11-06: **User menyetujui rencana** ("Sesuai, Native"). Implementasi inline di branch `fix/resign-terjadwal`:
  - T1 `a0387a93`: tabel `hris.scheduled_resignations`, resign tanggal mendatang jadi jadwal (≤ 180 hari), GET/DELETE jadwal, `applyResign` dipisah.
  - T2: job `ScheduledResignLoop` (5 menit), dijalankan sehari setelah hari terakhir kerja.
  - T3 `d1b8657d`: dialog ("Jadwalkan resign", catatan biru, batas 180 hari) dan banner profil dengan tombol "Batalkan jadwal".
- 2026-11-06: **Gerbang**: go hris ok; `pnpm test` 889 file / 6466 tes; `tsc` + lint bersih.
- 2026-11-06: **Review akhir** (reviewer segar): Critical 1, Important 2, diperbaiki dalam satu gelombang:
  - **C1:** karyawan yang dinonaktifkan lalu di-rehire bisa ikut di-resign oleh job. Diperbaiki dengan trigger pembatal jadwal saat nonaktif, dan `applied_at` diisi sebelum resign dijalankan. Tes RED → GREEN.
  - **I2:** jadwal yang ditolak validasi diulang tiap 5 menit. Kini dibatalkan dengan pesannya. Tes RED → GREEN.
  - **I1:** baris karyawan dikunci sebelum cek jadwal ganda.
  - Suite hris, payroll, dan configuration hijau.
  - Minor ditunda: pembatalan otomatis tanpa baris audit; tombol Resign tersembunyi saat data jadwal gagal dimuat; prop `canCancel` redundan; `fetch` manual untuk DELETE; handler GET membaca DB langsung.
- 2026-11-06: **PR #84** dibuka (base `main`).
- 2026-11-06 17:29: **Deploy staging**.
  - Backup `staging-db-before-epic095-20261106-172928.dump`.
  - Merge ke tumpukan staging, menjadi `6f928935`.
  - Image `local-20261106-172946`, 1 migrasi, verify 0 berbeda.
- 2026-11-06: **QA staging 15/15 lulus** (HR desktop, akun demo `qa095-*`).
  - **Dialog jadwal.** Tanggal +14 hari menampilkan catatan biru. "Jadwalkan resign" memunculkan toast "Resign dijadwalkan efektif 20-11-2026" dan banner di profil. Tombol Resign tersembunyi, dan karyawan masih aktif serta bisa login.
  - **Batal.** "Batalkan jadwal" → "Ya, batalkan" → banner hilang, tombol Resign kembali.
  - **Job.** Jadwal kedua dibuat jatuh tempo lewat SQL demo, lalu job menjalankannya pada tick berikutnya (17:37, log `hris resign terjadwal applied=1`). Hasilnya: nonaktif, `end_date` = hari terakhir kerja, status resigned, login diblokir dan ditolak di halaman masuk, audit oleh "Sistem (resign terjadwal)".
  - **Resign langsung.** Resign dengan tanggal hari ini tetap langsung, tanpa baris jadwal.
  - Data demo dihapus. Bukti di `.share/qa095/`.
- 2026-11-09 13.18 WIB: **LIVE production** lewat rilis gabungan PR #86 (main `bd9ff4db`, image `local-20261109-130944`; 10 migrasi EPIC-087..095, verify 0 berbeda). Backup `~/backups/prod-db-before-rilis-20261109-125740.dump` (VPS + lokal); rollback image `local-20261103-150738`, source lama `~/paddydev-src.before-rilis-20261109-130930`. Verifikasi read-only: `/login` dan `/api/ready` 200, container sehat, log API bersih. Sisa: QA HR di production.

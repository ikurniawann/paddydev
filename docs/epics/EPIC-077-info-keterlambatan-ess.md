# EPIC-077: Info Keterlambatan & Tingkat Teguran/SP di ESS (`MODULE-HRIS`)

status: review
environment: dev
retries: 0

## Goal
Karyawan tahu berapa kali dirinya terlambat dalam periode berjalan dan tingkat teguran/surat peringatan yang berlaku menurut ketentuan perusahaan, supaya lebih aware. Atasan melihat hal yang sama untuk timnya. Ketentuan dari user (02-11-2026):

> **KETERLAMBATAN DAN ABSENSI.** Jika karyawan datang absensi lebih dari jadwal yang ditentukan, maka akan mengikuti ketentuan sebagai berikut:
> - Jumlah Keterlambatan 0 – 2x dalam 1 bulan diberikan teguran lisan dan tertulis
> - Jumlah Keterlambatan 3x – 4x dalam 1 bulan diberikan surat peringatan 1
> - Jumlah Keterlambatan 5x – 6x dalam 1 bulan diberikan surat peringatan 2
> - Jumlah Keterlambatan di atas 6x dalam 1 bulan diberikan surat peringatan 3 dan/atau Pemutusan Hubungan Kerja.
>
> Seluruh karyawan melakukan absensi melalui website ERP PADDY (dapat berubah sesuai HRIS yang digunakan) dengan menggunakan device tablet yang telah disiapkan kantor.
> - Pastikan kamu sudah terdaftar di website absensi PADDY.
> - Pastikan kamu berada dalam area Lokasi kerja yang telah ditentukan.
> - Di Fitur "Area Karyawan" klik "check-in" saat jam masuk, dan "check-out" saat jam kerja sudah selesai.

Jawaban user 02-11:
- "1 bulan" = **periode absensi**, yaitu periode yang sama dengan payroll dan kunci absensi. Setting live `cutoff_day = 0` sama dengan bulan kalender; kalau nanti cutoff 20 dinyalakan, otomatis 21–20.
- "Terlambat" = **lewat toleransi shift**, yaitu label `is_late` yang sama dengan kalender absensi. Absen yang dikoreksi HR mengikuti hasil koreksinya.
- **Potongan gaji telat** (mode `tier` live: 4–6 = 1 hari THP, 7–9 = 2 hari gaji pokok, 10+ = 3 hari, min 15 menit) **tetap berlaku dan tidak ditampilkan** di ESS.
- Yang melihat: **karyawan sendiri + atasan** (Absensi Tim). Informasi saja; SP tetap diterbitkan HR secara manual.

Kondisi kode per 02-11 (main `8de3e547`):
- `hris.attendance.is_late` dan `late_minutes` diisi saat clock-in (`domain.Lateness`, toleransi `shifts.late_tolerance_minutes`) dan saat penyesuaian HR (`attendance_adjust.go`).
- ESS Beranda menampilkan "x terlambat" per **bulan kalender** (`AttendanceSummary`). ESS Absensi belum punya ringkasan keterlambatan.
- Data staging Oktober: 670 baris terlambat; 27 karyawan 3x, 17 4x, 15 5x, 16 6x, 27 lebih dari 6x.

## Ruling
- **R1 Tingkat** (domain Go, satu sumber; dikirim ke UI sebagai `rules`):

  | Kode | Jumlah dalam periode | Label |
  |---|---|---|
  | `none` | 0 | Tidak ada keterlambatan |
  | `teguran` | 1–2 | Teguran lisan dan tertulis |
  | `sp1` | 3–4 | Surat Peringatan 1 |
  | `sp2` | 5–6 | Surat Peringatan 2 |
  | `sp3` | lebih dari 6 | Surat Peringatan 3 dan/atau PHK |

  "0–2x" di ketentuan dibaca **1–2** untuk badge, karena 0 kali berarti tidak terlambat (keputusan agent). Teks ketentuan tetap ditampilkan apa adanya.
- **R2 Hitungan:** jumlah baris `hris.attendance` milik karyawan dengan `is_late = true` dan `date` di dalam rentang periode. Satu hari dihitung satu kali. Ikut ditampilkan: total menit terlambat dan daftar harinya (tanggal, shift, jadwal masuk, jam masuk, menit terlambat).
- **R3 Periode:** periode absensi berjalan (`periodContaining(today)`), ditambah navigasi mundur `back` = 0–5 (6 periode). Tiap langkah mundur memakai `periodContaining(start − 1 hari)`. Nilai di luar 0–5 → 400 "Periode tidak tersedia".
- **R4 Tingkat berikutnya:** `next` = {kode, label, `at` (jumlah minimum), `remaining` (sisa kali)}, null di `sp3`. UI menulis "1 kali terlambat lagi → Surat Peringatan 1".
- **R5 Endpoint karyawan** `GET /api/hris/me/lateness?back=N`: akun tanpa data karyawan mendapat `count` 0 dan `days` kosong. Hanya Go, tanpa fallback Next.
- **R6 Endpoint atasan** `GET /api/hris/me/team-lateness?back=N`: tim = `teamAttendees` (EPIC-074, rantai `reporting_to` sampai 10 level, hanya karyawan aktif). Per anggota: id, nama, jabatan, `direct`, `reporting_to_name`, `count`, `total_minutes`, `level`, `days`. Diurutkan `count` turun lalu nama. Ditambah `summary` jumlah anggota per kode tingkat. Tanpa tim → daftar kosong.
- **R7 Teks UI:** tingkat ditulis sebagai *ketentuan yang berlaku*, bukan sanksi yang sudah terbit ("Sesuai ketentuan: Surat Peringatan 1"), dengan catatan "Informasi pengingat. Surat peringatan resmi diterbitkan oleh HR." Potongan gaji tidak disebut.
- **R8 ESS Absensi:** kartu "Keterlambatan" di bawah panel clock berisi:
  - pemilih periode dan jumlah besar beserta badge tingkat berwarna (hijau/kuning/oranye/merah/merah tua);
  - tangga 5 tingkat dengan posisi saat ini, petunjuk tingkat berikutnya, dan total menit;
  - daftar hari terlambat;
  - bagian lipat "Ketentuan Keterlambatan & Absensi" berisi teks user.

  Karyawan tanpa jadwal shift mendapat catatan "Jadwal shift belum diatur HR — keterlambatan belum dinilai".
- **R9 ESS Beranda:** kartu ringkas "Keterlambatan periode ini" (jumlah, badge, petunjuk tingkat berikutnya) yang menaut ke `/dashboard/me/absensi#keterlambatan`. Stat card Kehadiran tidak diubah.
- **R10 Absensi Tim:** kartu "Keterlambatan Tim" di atas roster harian berisi:
  - pemilih periode dan chip ringkasan per tingkat yang bisa diklik sebagai filter;
  - daftar anggota dengan jumlah ≥ 1 (badge, jumlah, total menit, atasan langsung bila tidak langsung), yang bisa dibuka untuk melihat hari terlambatnya;
  - teks "Semua anggota tepat waktu" bila tidak ada.
- **R11 Mobile:** semua kartu nyaman di layar 360–390 px:
  - tidak ada scroll horizontal;
  - target sentuh ≥ 40 px;
  - daftar memakai kartu, bukan tabel lebar;
  - pemilih periode memakai Select penuh lebar di HP.
- **R12** Tanpa migrasi dan tanpa perubahan data atau setting. Payroll tidak disentuh.
- **R13** Acuan MOM: AB-07 diperbarui (TnC 02-11: tingkat teguran/SP informasi di ESS; potongan tier tetap), ditambah baris di `pertanyaan-rekomendasi.md` dan tabel aturan di SKILL acuan.

## Tasks
Rencana: `docs/superpowers/plans/2026-11-02-info-keterlambatan-ess.md`.

## Acceptance Criteria
- Jumlah 0/1/2/3/4/5/6/7 → none/teguran/teguran/sp1/sp1/sp2/sp2/sp3. `next` benar, null di sp3.
- Hitungan hanya `is_late` di dalam rentang periode milik karyawan itu. Baris tidak terlambat, karyawan lain, dan tanggal di luar periode tidak ikut.
- `back=1` memberi periode sebelumnya; `back=6` atau `abc` → 400.
- Atasan melihat seluruh tim (langsung dan tidak langsung), tidak melihat orang di luar tim. Akun tanpa tim mendapat daftar kosong.
- ESS Absensi, Beranda, dan Absensi Tim tampil rapi di laptop dan HP (360 px), tanpa scroll horizontal.
- `go test`, `pnpm test`, lint, dan tsc hijau; CI 4/4; QA staging laptop + HP dengan data demo, dibersihkan sesudahnya.

## Automation Log
- 2026-11-02: Epic dibuat dari ketentuan user. Jawaban user: periode absensi; lewat toleransi shift; potongan tier tetap tanpa ditampilkan; karyawan + atasan. User meminta dikerjakan sampai selesai tanpa bug, dengan layout mobile ESS rapi. Status `coding`.
- 2026-11-02: **Implementasi** (eksekusi inline):
  - `2a0b7503` backend: domain tingkat (`hris/domain/lateness.go`), `GET /api/hris/me/lateness` dan `/api/hris/me/team-lateness`, tes integrasi.
  - `ed258610` frontend: fitur `features/hris/lateness`, dengan kartu ESS Absensi, Beranda, dan Absensi Tim.
- 2026-11-02: **Review akhir** (opus): 0 kritis, 2 penting, 6 minor. Semua diperbaiki di `41146841`:
  - Periode sebelumnya dibaca tanpa insert/auto-close. Versi awal membuat dan menutup periode lama lewat GET, melanggar R12; kini tes `TestMyLatenessCutoff20` menangkapnya.
  - Ganti periode meredupkan isi (`aria-busy`).
  - Pemilih periode tetap ada saat galat. Absensi Tim menampilkan pesan galat, tidak menghilang.
  - Nama tombol anggota memuat jumlah dan tingkat.
  - Region ketentuan selalu ada di DOM.
  - Lompat `#keterlambatan` setelah data pertama.
  - Yang tepat waktu tidak diberi petunjuk "1 kali terlambat lagi".
  - Urutan nama tidak peka huruf besar.
- 2026-11-02: **Polish dari QA visual** `b3065dff`:
  - Catatan "Jadwal shift belum diatur" hanya muncul saat 0 terlambat; sebelumnya kontradiktif dengan 8 hari terlambat.
  - Kotak angka di Beranda dilebarkan.
- 2026-11-02: **Gerbang lokal:**
  - `go vet ./...` bersih; `go test -p 1` hris + app ok.
  - Mutasi diverifikasi: tes gagal bila filter `is_late` dihapus, bila periode di-materialize, atau bila urutan nama peka huruf besar.
  - Frontend: `tsc` 0, `lint` 0, `pnpm test` 878 file / 6347 tes lulus. Suite lateness dan ESS diulang sesudah perbaikan: lulus.
- 2026-11-02 01:02: **Staging** image `local-20261102-010235` (branch `b3065dff`), tanpa migrasi. Backup `~/Desktop/paddydev-backups/staging-db-before-epic077-20261102-005112.dump`.
- 2026-11-02: **QA staging** 54/54 lulus, dua kali (sebelum dan sesudah polish), 0 error console, 0 API ≥ 400. Data demo: atasan → koordinator → staf, plus anggota tepat waktu.
  - Staf, HP 360 px dan laptop:
    - November 1x → Teguran, dengan petunjuk "2 kali lagi → SP 1";
    - Oktober 8x → SP 3, tanpa tingkat berikutnya; 65 menit tampil sebagai "1 jam 5 mnt";
    - ketentuan bisa dibuka; tidak menyebut potongan gaji;
    - tautan Beranda melompat ke kartu;
    - staf tanpa tim → daftar kosong; `back=9` → 400.
  - Koordinator, HP:
    - November 0x;
    - Oktober 4x → SP 1, hari tepat waktu tidak dihitung;
    - Absensi Tim hanya menampilkan stafnya.
  - Atasan, laptop dan HP: staf 8x via koordinator, koordinator 4x; chip Tepat waktu dan SP 3 berfungsi; 8 hari staf bisa dibuka.
  - Tidak ada scroll horizontal di semua halaman.
  - Data demo disisipkan dan dihapus dengan trigger absensi dimatikan, jadi tidak ada notifikasi "Karyawan Terlambat" atau "Data Absensi Dihapus" ke HR staging. Sisa data 0.

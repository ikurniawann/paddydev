# EPIC-079: Jam Kerja Sistem Shift, Libur Nasional & Cuti Bersama (`MODULE-HRIS`)

status: review
environment: dev
retries: 0

## Goal
Absensi, roster, cuti, lembur, dan payroll mengikuti TnC jam kerja sistem shift dan aturan hari libur dari user (02-11-2026). Aturan ini melengkapi EPIC-077 (keterlambatan) dan EPIC-078 (jam kerja HQ & Operations):

> **Hari dan Waktu Kerja Sistem Shift (Retail Operasional, Host Live, CRM dan Produksi).**
> Hari Senin – Minggu (dengan libur 1 hari setiap minggunya)
> - Shift 1: 08.00 – 16.00 WIB
> - Shift 2: 11.00 – 19.00 WIB
> - Shift 3: 13.30 – 21.30 WIB
>
> Sebagai catatan jam mulai dan berakhirnya shift dapat berbeda sesuai dengan unit kerja masing-masing.
> Karyawan wajib menaati ketentuan jam kerja yang telah ditetapkan oleh perusahaan. Apabila terdapat perubahan atau penyesuaian terhadap waktu kerja karena kebutuhan operasional perusahaan, maka perubahan tersebut akan diinformasikan terlebih dahulu kepada karyawan secara layak.
> Hari libur untuk karyawan Operations dan Headquarter mengikuti hari libur nasional yang ditetapkan oleh pemerintah Republik Indonesia dan hari libur sesuai dengan shift yang telah ditentukan. Hari cuti bersama tidak secara otomatis menjadi hari libur, kecuali jika ditetapkan demikian oleh perusahaan.
> Hari libur untuk karyawan dengan sistem kerja shift diberikan begitu tercapai 40 jam kerja dan tidak mengikuti hari libur nasional atau tanggal merah yang ditetapkan oleh Pemerintah Republik Indonesia. Penetapan hari libur disesuaikan dengan kebijakan internal perusahaan dan/atau jadwal kerja (shift) yang telah ditentukan.

Jawaban user 02-11:
- **40 jam**: libur 1 hari setelah 6 hari kerja (6 shift × 7 jam bersih = 42 ≥ 40). Ditegakkan sebagai **peringatan** roster yang bisa dilewati dengan centang "sudah diperiksa", bukan blokir.
- **Cuti bersama**: default **hari kerja biasa**. HR menyalakan tombol **"Ditetapkan libur oleh perusahaan"**; bila menyala, cuti bersama menjadi libur HQ/Operations dan memotong cuti tahunan seperti sekarang.
- **Grup shift**:
  - Retail Operational (toko, sudah Shift).
  - **Host Live** (7, Marketing).
  - **Produksi** (Production Staff + Case Production Coordinator, 7, Warehouse).
  - **CRM** (2 Customer Service Staff sudah Shift, ditambah CRM Supervisor).
  - Yang masih Non-Shift diubah menjadi **Shift**.
- **Payroll** ikut diperbaiki di epic ini: pembagi hari kerja membaca jadwal per tanggal (roster), bukan hanya pola mingguan. Perubahan disimulasikan dulu di salinan data.

## Kondisi kode per 02-11 (main `cfff8811`; analisis `file:line` di Automation Log)
- **Libur sama untuk semua orang.** Semua hari libur aktif (`nasional`, `cuti_bersama`, `perusahaan`) diperlakukan sebagai libur, tanpa membedakan `schedule_type`:
  - Kalender absen: `DeriveRosterStatus` mendahulukan libur atas shift, jadi tampil `libur_nasional`.
  - Absen terlewat (`AttendanceGaps`) disembunyikan pada hari libur.
  - Jenis hari lembur (`OvertimeDayTypeFor`) menjadi `rest_day`.
  - Payroll: lembur di hari libur dibayar 2× (`SplitOvertimeWith`).
- **Hitung hari cuti** (`DescribeLeaveDays`) selalu melewati Sabtu–Minggu, juga untuk staf shift yang bekerja 7 hari.
- **Peringatan roster** (`schedule_upload.go`) sudah memeriksa:
  - lebih dari 6 hari kerja berturut-turut (`maxStreak = 6`, memenuhi "libur 1 hari setiap minggunya");
  - jeda kurang dari 8 jam antar-shift;
  - shift pada hari libur nasional (`holidayIssue`).

  Peringatan 40 jam belum ada, dan `UploadShift` tidak membawa jam istirahat.
- **Payroll** membaca jadwal hanya dari `hris.employee_shifts` (`app/adapters_payroll.go:285`). Roster per tanggal (`employee_shift_days`) tidak masuk ke `ScheduledDays` maupun `IsWorkDate`.
- Master shift sudah memuat Shift 1–3 TnC: SPG1 08.00–16.00, SPG2 11.00–19.00, SPG3 13.30–21.30, semuanya toleransi 10. Shift lain per unit kerja (CS, PRF, SPG lain) tetap berlaku.

## Ruling
- **R1 Sistem shift = `schedule_type = 'shift'`.** Non-Shift = HQ dan Operations (EPIC-078). Semua aturan libur memakai pembeda ini.
- **R2 Libur HQ/Operations (Non-Shift).** Hari yang berlaku sebagai libur:
  - `nasional` dan `perusahaan`;
  - `cuti_bersama` **hanya bila** `is_day_off = true` (kolom baru, default `false`).

  Cuti bersama yang tidak ditetapkan libur adalah hari kerja biasa:
  - absen dinilai dan absen terlewat dihitung;
  - lembur dibayar tarif hari kerja;
  - cuti di tanggal itu dihitung 1 hari, sama seperti hari kerja biasa.
- **R3 Libur staf shift.** Tanggal merah **tidak** berlaku:
  - Kalender absen mengikuti roster. Shift tetap tampil dan nama hari libur hanya label.
  - Absen terlewat tetap dihitung.
  - Jenis hari lembur mengikuti roster: tidak ada shift berarti `rest_day` (2×); ada shift berarti `weekday` (1,5×), meskipun tanggal merah.
  - Peringatan "libur nasional: N karyawan diberi shift" dihapus, karena grid jadwal hanya memuat staf shift.
- **R4 Hitung hari cuti staf shift** memakai roster, bukan Senin–Jumat:
  - tanggal dengan shift dihitung 1 hari;
  - tanggal libur roster (sel kosong/libur) tidak dihitung;
  - tanggal tanpa jadwal berlaku sama sekali (roster belum dibuat) dihitung **hanya bila Senin–Jumat**, tanpa melewati tanggal merah untuk staf shift. Ini sama dengan `IsWorkDate` payroll (peta libur kosong untuk staf shift), sehingga hari yang mengurangi kuota sama dengan hari yang dipotong payroll. (Keputusan controller, review akhir: sebelumnya semua tanggal tanpa roster dihitung, termasuk Sabtu–Minggu.) Jadwal "berlaku" bila ada baris pola mingguan atau sel per tanggal yang berlaku pada tanggal itu, hari apa pun; bila berlaku, barisnya untuk hari itu yang menentukan;
  - nol hari ditolak; staf shift mendapat pesan sendiri ("Rentang tanggal ini jatuh pada hari libur di jadwal kerja Anda — tidak ada hari yang dipotong dari jatah cuti"), bukan "akhir pekan";
  - rentang pengajuan dan pratinjau dibatasi 366 hari ("Rentang cuti maksimal 366 hari").

  Non-Shift tetap memakai aturan lama, ditambah R2 untuk cuti bersama.
- **R5 Peringatan 40 jam** (roster upload dan simpan sel; bisa dilewati dengan `ack_warnings`):
  - **Libur terlalu cepat**: hari libur yang diberikan sebelum 40 jam kerja bersih sejak libur sebelumnya. Hanya berlaku bila libur sebelumnya ada di jendela, dan salah satu hari di antaranya diedit. Contoh pesan: "Ana libur 05/11 sebelum 40 jam kerja (baru 28 jam sejak libur 01/11)".
  - Jam bersih = durasi shift − `break_minutes`. Shift malam dihitung melewati tengah malam.
  - Peringatan "masuk N hari berturut-turut" (> 6) yang sudah ada menegakkan "libur 1 hari setiap minggunya".
- **R6 Pola mingguan staf shift.** Dialog pola mingguan memberi peringatan inline (tidak memblokir) bila 7 hari terisi semua tanpa libur, dan menampilkan total jam bersih per minggu.
- **R7 Payroll membaca roster.**
  - Sumber jadwal adalah `hris.v_employee_schedule`: jadwal per tanggal (prioritas 0) mengalahkan pola mingguan pada tanggal yang sama.
  - Payroll domain `ShiftRow` mendapat `Priority`; `ResolveScheduleRow` memilih `effective_from` terbaru, lalu prioritas terkecil.
  - `IsWorkDate` dan `SplitOvertimeWith` menerima peta libur **per karyawan**: kosong untuk staf shift, R2 untuk Non-Shift.
  - Adapter KPI memakai query jadwal yang sama.
- **R8 Ubah ke Shift** (migrasi sekali, mulai tanggal deploy, diaudit):
  - Sasarannya karyawan aktif bertipe `non_shift` dengan jabatan cocok `host live|production|crm`.
  - Pola tetap mereka dihentikan sehari sebelumnya (seperti `stopWeeklyPattern`).
  - `hris.work_hours_rule_targets` dan trigger pindah departemen mengecualikan jabatan yang sama, supaya tidak dikembalikan ke jadwal tetap.
  - Setelah migrasi mereka **tanpa jadwal**, sehingga keterlambatan baru dinilai setelah HR/pengatur membuat roster. Daftar nama diserahkan ke HR.
- **R9 Toleransi shift** tetap 10 menit (sama dengan Warehouse, keputusan EPIC-078). TnC shift tidak menyebut toleransi.
- **R10 ESS**:
  - Panel "Jam Kerja & Ketentuan Keterlambatan" mendapat kelompok **"Karyawan Sistem Shift"** (Shift 1–3, Senin–Minggu dengan 1 libur/minggu, catatan unit kerja) dan paragraf hari libur, apa adanya (`work-hours.ts`).
  - HR Hari Libur: cuti bersama punya tombol "Ditetapkan libur oleh perusahaan".
  - Badge di daftar hari libur: "Libur HQ/Operations" atau "Hari kerja".

## Tasks
Lihat `docs/superpowers/plans/2026-11-02-jam-kerja-sistem-shift.md`.

## Acceptance Criteria
- [ ] Staf toko dijadwalkan SPG1 pada 25-12 (Natal): kalender menampilkan shift, tidak hadir terhitung absen terlewat, lembur di hari itu 1,5×.
- [ ] Staf HQ pada 25-12 libur. Pada cuti bersama 24-12 tanpa penetapan: hari kerja (absen dinilai). Dengan penetapan: libur dan cuti tahunan dipotong.
- [ ] Cuti staf toko 5 hari kalender dengan 1 libur roster di dalamnya terhitung 4 hari. Untuk staf HQ Senin–Jumat tetap 5 hari.
- [ ] Roster yang memberi libur setelah 4 shift (28 jam) memunculkan peringatan 40 jam. Roster 6 kerja + 1 libur tidak memunculkannya.
- [ ] Pembagi payroll staf toko = hari ber-shift di roster periode itu. Simulasi sebelum/sesudah di salinan data dilampirkan.
- [ ] Host Live, Produksi, dan CRM Supervisor menjadi Shift setelah deploy dan tidak dikembalikan oleh trigger/aturan EPIC-078.
- [ ] ESS (HP 360 px) menampilkan jam kerja sistem shift dan aturan libur.

## Automation Log
- 2026-11-02 — Analisis kode dan data staging. Jawaban user untuk 4 keputusan dicatat di atas. Rencana disusun.
- 2026-11-02 — Review akhir seluruh branch, perbaikan: (1) dialog sukses Ajukan Cuti HR memakai `total_days` dari server, angka sebelum kirim berlabel perkiraan; (2) simulasi `payroll_divisor.sql`: kolom `sebelum` meniru kode lama (tanpa syarat kelengkapan), ditambah daftar R1 (staf shift di luar toko yang kehilangan tanggal merah, perlu konfirmasi HR sebelum 24/25-12) dan daftar sasaran jabatan sistem shift dengan penempatan utamanya; (3) **R4 berubah (keputusan controller)**: tanggal tanpa jadwal berlaku dihitung hanya bila Senin–Jumat, sama dengan `IsWorkDate` payroll, sehingga hari kuota = hari dipotong payroll (`TestShiftLeaveQuotaDaysEqualPayrollWorkDates`); (4) kalender absen ESS/HR mengikuti tipe jadwal (`GET /api/hris/attendance/schedule` menambah `meta.schedule_type`); (5) pesan nol hari khusus staf shift; (6) rentang cuti dan pratinjau maksimal 366 hari; (9) perubahan `is_day_off` diaudit (`holiday.day_off`, `TestHolidayDayOffAudit`).

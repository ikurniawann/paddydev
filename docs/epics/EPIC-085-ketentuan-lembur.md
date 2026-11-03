# EPIC-085: Ketentuan Lembur + Batas 72 Jam per Cut Off (`MODULE-HRIS`)

status: backlog
environment: dev
retries: 0

## Goal
Permintaan user 03-11-2026: menerapkan TnC "KETENTUAN LEMBUR" dan "batas lembur per cut off (72 jam)" (MOM 29-10 AP-07 / K-23).

TnC (ringkas):
- Lembur memerlukan perintah atau persetujuan atasan langsung.
- Dihitung hanya bila minimal 1 jam penuh.
- Tidak berlaku untuk level Supervisor dan Manager.
- Jam lembur = di atas 40 jam kerja seminggu, atau di atas jam 16:00 (Warehouse) dan 17:00/18:00 (HQ) pada Senin–Jumat, serta di atas jam 14:00 pada Sabtu (Warehouse).
- Wajib mengajukan, mencatat, dan mendokumentasikan.
- Upah lembur: hari kerja 1,5 × Gaji Pokok/173; hari libur nasional 2 × Gaji Pokok/173.
- Maksimal 4 jam/hari dan 18 jam/minggu.
- Berlaku juga untuk perjalanan dinas di hari libur atau tanggal merah.

### Jawaban user 03-11
- **Level**: SPV ke atas (Supervisor, Assistant Manager, Manager, Senior Manager, General Manager, Director, C-Level), sama dengan syarat Izin Leader.
- **Jam mulai**: ikut jam selesai shift/jadwal karyawan hari itu. Di hari libur/istirahat bebas. Aturan 40 jam/minggu tidak dihitung terpisah.
- **Batas 4 jam/hari, 18 jam/minggu (Senin–Minggu), 72 jam/periode**: **blokir**. Yang dihitung = pengajuan menunggu + disetujui.
- **Dinas**: ajukan lembur biasa untuk tanggal libur yang tercakup perjalanan dinas disetujui. Syarat clock-out dilewati untuk tanggal itu, dibayar 2×.

## Kondisi kode per 03-11 (branch EPIC-084 `cee8abd5`)
- `CreateOvertime` (`hris/overtime.go:130`):
  - Self: input setelah selesai (EPIC-066), H+1 dan kunci cut off (EPIC-084), lampiran wajib.
  - HRD: penugasan untuk karyawan lain (`source='company'`).
  - Satu pengajuan aktif per tanggal; durasi kotor ≤ 12 jam; jam dibayar ≥ 1 (`PaidOvertimeHours`, 2 desimal, 1,5 tetap 1,5).
  - **Tidak ada** cek level, cek jam mulai vs jadwal, atau batas mingguan/periode.
- Keputusan: `DecideOvertime` (atasan langsung / HR berhak approve daun Lembur; atasan boleh koreksi jam). Koreksi HR: `CorrectOvertime` (`overtime_correction.go`).
- Jadwal per tanggal: `hris.v_employee_schedule` (pola + roster per tanggal) + `hris.shifts` (`end_time`, `is_overnight`, `shift2_id`).
- Payroll:
  - `SplitOvertimeWith` (`payroll/domain/period.go:191`): hari libur, `rest_day`, atau tanpa shift → 2×; selain itu 1,5×.
  - `overtime_requires_clockout` (default true) melewati lembur tanpa absensi clock-out.
  - Pembagi 173 dan pengali 1,5/2 sudah sesuai TnC.
- Perjalanan dinas: `hris.business_trips` (`status='approved'`, `start_date..end_date`); `ApprovedTripRanges` di `attendance_gaps.go:165`.

## Ruling
- **R1 Level**: pengajuan sendiri dan penugasan HR untuk karyawan berlevel `domain.IsLeaderLevel` ditolak 400 "Lembur tidak berlaku untuk jabatan Supervisor ke atas". Persetujuan pengajuan lama (sebelum level berubah) tidak diblokir. Halaman Lembur ESS menampilkan pemberitahuan yang sama dan menyembunyikan form.
- **R2 Jam mulai**: pada hari kerja (jenis `regular`) yang jadwalnya diketahui, `start_time` harus ≥ jam selesai shift hari itu (shift kedua bila ada 2 shift). Bila tidak: 400 "Lembur hari kerja dimulai setelah jam kerja selesai ({HH:MM})".
  - Shift overnight, tanpa jadwal, `rest_day`, dan tanggal perjalanan dinas → tidak dicek.
  - Berlaku untuk pengajuan sendiri, koreksi atasan, dan koreksi HR. Penugasan HR untuk karyawan lain bebas (HR menetapkan jam).
- **R3 Batas**: total jam dibayar (pending + approved, tanpa baris yang sedang dikoreksi) + jam baru:
  - ≤ 4 per tanggal → "Lembur maksimal 4 jam per hari";
  - ≤ 18 per minggu Senin–Minggu → "Lembur minggu {dd-mm} s.d. {dd-mm} sudah {x} jam; maksimal 18 jam per minggu (sisa {y} jam)";
  - ≤ 72 per periode absensi (`periodContaining`) → "Lembur periode {label} sudah {x} jam; maksimal 72 jam per periode (sisa {y} jam)".

  Dicek di create (self dan HR), approve dengan koreksi jam, dan koreksi HR, dalam transaksi dengan kunci baris karyawan. Batas 12 jam kotor tetap.
- **R4 Dinas**: tanggal yang tercakup perjalanan dinas `approved` milik karyawan:
  - jenis lembur yang disarankan = `rest_day` bila tanggal itu tanggal merah aktif atau hari tanpa shift (juga untuk staf shift);
  - payroll tidak mensyaratkan clock-out untuk tanggal itu (`OvertimeRequest.OnTrip`).
- **R5 Tetap**:
  - Minimal 1 jam dan 1,5 jam dibayar 1,5 (TnC tidak meminta pembulatan).
  - Hari istirahat (`rest_day`) tetap 2× seperti AP-03.
  - Perintah/persetujuan atasan memakai alur EPIC-063/066.
- **Di luar cakupan**: aturan 40 jam/minggu terpisah; menyembunyikan menu Lembur di navigasi (cukup pemberitahuan di halaman); fallback route Next.

## Tasks
Rencana: `docs/superpowers/plans/2026-11-03-ketentuan-lembur.md`.

## Acceptance Criteria
- [ ] SPV+ tidak bisa mengajukan/ditugaskan lembur; ESS menampilkan pemberitahuan.
- [ ] Lembur hari kerja yang mulai sebelum jam selesai shift ditolak; hari libur/dinas bebas.
- [ ] Batas 4/18/72 jam memblokir create, approve-koreksi, dan koreksi HR dengan sisa jam di pesan.
- [ ] Lembur di tanggal merah saat dinas disarankan Weekend/Libur dan dibayar 2× tanpa clock-out.
- [ ] Gates hijau; staging QA.

## Automation Log
- 2026-11-03 — Epic + rencana ditulis setelah 4 jawaban user. Worktree `.worktrees/epic085`, branch `feat/epic085-ketentuan-lembur`, ditumpuk di atas `feat/epic084-ketentuan-cuti` (PR #69) karena memakai H+1 dan kunci cut off EPIC-084.

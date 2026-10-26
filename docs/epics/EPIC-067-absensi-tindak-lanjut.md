# EPIC-067: HRIS Absensi — Tindak Lanjut (Export Kurang Jam, Ringkasan Absen Terlewat HR, Approve per Menu Daun)

status: on-progress
environment: dev
retries: 0

## Goal
Menutup tindak lanjut yang diparkir dari EPIC-065:
1. Export Rekap absensi memuat kolom "Kurang Jam" (Ya/-) dan "Jam Minimum".
2. HR menerima ringkasan harian absen terlewat: notifikasi in-app pukul 09.00 WIB untuk kemarin dan kartu ringkasan di HRIS → Absensi. Karyawan tanpa jadwal tidak dihitung; jumlahnya ditampilkan terpisah.
3. Keputusan HR (pengajuan absensi, lembur cadangan, cuti cadangan, Perjalanan Dinas) memakai aksi `approve` per menu daun (konvensi EPIC-061), bukan peran aplikasi `IsHRD`. Tidak ada akun yang kehilangan akses.
4. "HR mengajukan absen atas nama karyawan": **tidak dikerjakan** (lihat Automation Log).

## Tasks
Lihat `docs/superpowers/plans/2026-10-26-absensi-tindak-lanjut.md` (Task 1–8, ruling R1–R15).

## Acceptance Criteria
- CSV/XLSX Rekap memuat `Kurang Jam` dan `Jam Minimum` sebelum `Catatan`; PDF menandai `kurang jam` di kolom Status; fallback Next sama dengan Go.
- Setiap hari mulai 09.00 WIB, akun HR (`role = 'hrd'`, penerima notifikasi HR yang sudah ada) menerima satu notifikasi "Ringkasan Absen Terlewat" untuk kemarin bila ada absen terlewat; tidak pernah dua kali untuk tanggal yang sama.
- Halaman HRIS → Absensi menampilkan kartu ringkasan (belum absen, belum absen pulang, tanpa jadwal tidak dihitung, daftar nama) untuk kemarin atau tanggal pilihan.
- Keputusan HR di keempat alur hanya bisa oleh akun yang punya `approve` di daun (`hris.workforce.attendance`, `hris.workforce.leaves`, `hris.kepegawaian.overtime`); atasan langsung tetap memutus bawahan; tanpa self-approval.
- Skrip cek `backend/database/sql/checks/2026-10-26_epic067_approve_access.sql` menunjukkan 0 akun kehilangan akses (DB test, staging, gladi production).

## Automation Log
- 2026-10-26: **Instruksi user (brief "HRIS Absensi — tindak lanjut", dikerjakan di sesi ini).** Keputusan user yang dipakai tanpa ditanya ulang: ringkasan absen terlewat = notifikasi in-app ke semua akun HR setiap 09.00 WIB untuk hari kerja kemarin (sumber data sama dengan `AttendanceGaps`) + kartu di HRIS → Absensi, tanpa WA, karyawan tanpa jadwal tidak dihitung dan jumlahnya ditampilkan terpisah; aksi `approve`: perilaku hari ini dipertahankan, ketiga akun HR (`admin_hris` + `hris_manager`, `hris_non_payroll`, `hris_non_rekrutmen_kpi`) mendapat `approve` di menu relevan, super_admin/admin tetap bisa, atasan langsung tetap memutus bawahan, tidak boleh ada akun kehilangan akses (bukti grant sebelum/sesudah).
- 2026-10-26: **Butir 4 tidak dikerjakan (keputusan user):** "HR mengajukan absen atas nama karyawan" tidak dibuat karena HR sudah punya absen manual dan koreksi langsung (`attendance_adjust.go`, riwayat `hris.attendance_adjustments` + audit).
- 2026-10-26: Rencana ditulis (`docs/superpowers/plans/2026-10-26-absensi-tindak-lanjut.md`). Branch `feat/epic067-absensi-tindak-lanjut` dari `main` `3b8559b` di worktree `.worktrees/epic067-absensi`. Baseline Go: DB test lama `paddy_test` (55499) memberi 28 kegagalan 403 di `main` karena data IAM DB test sudah melenceng dari migrasi (CI memakai DB segar); dibuat DB segar ala CI `paddy_test_e067` (role `authenticated/anon/service_role` + 560 migrasi) → `go test -p 1 -count=1 ./...` 136 paket ok, 0 gagal. Gerbang Go epic ini memakai `paddy_test_e067`.

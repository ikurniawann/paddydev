# EPIC-067: HRIS Absensi — Tindak Lanjut (Export Kurang Jam, Ringkasan Absen Terlewat HR, Approve per Menu Daun)

status: review
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
- 2026-10-26: **Implementasi Task 1–7 selesai (subagent-driven: satu implementer per task, TDD, review spesifikasi + kualitas per task, perbaikan sampai bersih; review akhir seluruh branch model terkuat).** Commit:
  - Task 1 `ac3a533` kolom `Kurang Jam`/`Jam Minimum` di export Rekap (Go + fallback Next; PDF menandai ` · kurang jam`).
  - Task 2 `370e351` migrasi `20261027090000_attendance_gap_digests.sql` + ringkasan absen terlewat per tanggal set-based (aturan sama dengan `domain.AttendanceGaps`) + `GET /api/hris/attendance/gap-summary`.
  - Task 3 `33fc974` notifikasi harian 09.00 WIB (`RunGapDigest`, loop 5 menit di modul `hris`, tepat sekali per tanggal lewat PK penanda).
  - Task 4 `2672937` kartu ringkasan di HRIS → Absensi.
  - Task 5 `b667f67` migrasi `20261027090100_iam_hris_approve.sql` (aksi `approve` di daun Absensi/Cuti/Lembur untuk super_admin, admin, hrd, admin_hris + `iam.sync_hris_staff_roles()` berpenjaga), `Actor.CanApprove`, `GET /api/hris/me/approve-access`, skrip cek `backend/database/sql/checks/2026-10-26_epic067_approve_access.sql`.
  - Task 6 `3d02b64` + `86a9213` jalur HR keempat keputusan memakai `approve` (Go + fallback Next cuti/lembur).
  - Task 7 `9b0deb0` + `0dcfbd2` UI tab/tombol mengikuti `approve-access` (fail-closed).
  - Gelombang perbaikan review akhir `5f6ac2c`..`f08559c`: GET ringkasan tidak lagi menulis periode/audit (status periode dibaca saja, batas 62 hari ke belakang, tahun aneh → 400 bukan 500), header skrip cek (gerbang = run SESUDAH migrasi: `lost` 0 + setiap `gained` ditinjau), loop digest hanya bila ada DB, peringatan bila tidak ada penerima HR, komentar domain, tombol Batalkan penugasan lembur HR mengikuti `approve`, badge fallback Next, hapus helper berbasis peran (`canManageTrips`, `canDecideAttendanceRequests`).
- 2026-10-26: **Ruling yang dipakai** (rencana R1–R15 + ruling eksekusi):
  - `unscheduled` = semua karyawan yang memenuhi syarat tanpa baris jadwal pada tanggal itu, walau hadir (R5 harfiah).
  - `actorFor` membaca grant lewat koneksi modul (`auth.ResolveRoleIDsOn`/`GrantedMenuActionsOn`, SQL sama; metode auth lama mendelegasikan).
  - Bukti "tidak ada akun kehilangan akses" = skrip cek dijalankan SESUDAH migrasi `approve`; run sebelum migrasi hanya informasi.
  - Endpoint koreksi jam lembur (`PATCH /api/hris/overtime/{id}`) dan pembuatan penugasan lembur oleh HR tetap `IsHRD` (bukan keputusan).
  - `attendance_adjust.go` menerima `approve` Absensi hanya di jalur penulisan persetujuan pengajuan.
  - Daftar/detail HR cuti dan lembur tetap `IsHR` (visibilitas ≠ keputusan; perilaku lama dipertahankan) — tindak lanjut bila akun non-HR diberi `approve`.
  - `approve-access` hanya di Go; UI fail-closed (mode fallback Next menyembunyikan tab/tombol HR).
  - `period_closed` kartu = true juga untuk periode terbuka yang sudah lewat tanggal closing (dibaca saja).
  - Tombol Batalkan penugasan lembur HR disembunyikan tanpa `approve` Lembur.
  - Trailer commit `33fc974` menyebut model implementer (Sonnet); tidak menulis ulang riwayat.
- 2026-10-26: **Bukti verifikasi.**
  - Go (`f08559c`, DB segar ala CI `paddy_test_e067`): `gofmt -l` kosong, `go vet ./...` bersih, `go test -p 1 -count=1 ./...` 136 paket ok, 0 FAIL; `make -C backend generate` tanpa diff; migrasi `-apply` + `-verify` (0 hilang/berbeda).
  - Frontend: `npx vitest run` 831 file lulus (2 dilewati), 6003 test lulus, 0 gagal; `npx tsc --noEmit` bersih; eslint file yang disentuh bersih; `pnpm build` sukses + `pnpm check:safari` 452 chunk OK untuk Safari 15.
  - Test inti: `TestAttendanceExport`, `TestAttendanceGapSummary`, `TestAttendanceGapSummaryReadOnly`, `TestGapDigestNotice`, `TestGapDigestRun`, `TestGapDigestLoopRunsAtStart`, `TestGapDigestWarnsWithoutRecipients`, `TestNewWithoutDBStartsNoDigestLoop`, `TestApproveAccess`, `TestActorCanApprove`, `*HRSideNeedsApprove` (4 keputusan, kasus i–iv); vitest `attendance-gap-summary-card`, `approve-access`, `approval-parity`, `hris-a2-routes`, `nav-badges`, `overtime-table`, `overtime-page`, `leaves-page`, `attendance-page`, `attendance-report`.
  - Staging lokal (DB `paddy_smoke_e067` = salinan DB test ber-seed + 2 migrasi baru; API Go :18767 + `next dev` :3067; bukan live):
    - Skrip cek akses: sebelum migrasi 12 `lost` (informasi); sesudah migrasi 8 `lost` — semuanya akun sisa test `go-test-*@test.local` (peran aplikasi admin/hrd dengan role IAM sekali-pakai `go_test_*`), tidak ada akun nyata; 3 akun HR (`paddy.peopledevelopment`, `paddyhroperation`, `paddyhumanresources`), `qa065-hrd`, dan super_admin tetap bisa memutus keempat keputusan; 0 `gained`.
    - Digest: loop saat start mengirim "Ringkasan Absen Terlewat" untuk 25-10-2026 ke 5 akun `hrd` dengan teks `Kemarin (Min, 25 Okt 2026): 19 karyawan belum absen, 0 belum absen pulang. 9 karyawan tanpa jadwal tidak dihitung.`; baris penanda tercatat.
    - API: `approve-access` HR semua true, HR tanpa approve dan karyawan semua false; gap-summary 200 (HR, HR baca-saja yang punya menu), 403 karyawan tanpa menu, 400 tanggal tidak valid / masa depan / > 62 hari / `0002-01-01`; HR tanpa approve → 403 antrean & keputusan pengajuan absensi; keputusan pada periode closing → 409; HR ber-approve menyetujui → baris absensi 08:00–17:00 tercatat.
    - Playwright 14/14: kartu "Absen terlewat kemarin" (19 belum absen, 9 tanpa jadwal), daftar nama, tab Pengajuan Absensi & Perjalanan Dinas tampil untuk HR ber-approve dan tidak tampil untuk HR tanpa approve, export CSV memuat `Kurang Jam`/`Jam Minimum (jam)`, notifikasi tampil di API notifikasi, gap-summary 403 untuk karyawan.
- 2026-10-26: **Minor yang ditunda (tidak memblokir rilis):** default "kemarin" di kartu tidak diperbarui lewat tengah malam; hook ringkasan tidak dites di bawah mock; banner 403 bila akun tanpa menu Absensi membuka halaman; `actorFor` menambah 2–3 query IAM per request HRIS (muat malas sebagai tindak lanjut); badge fallback Next menjalankan 2 query IAM per poll; pembuat penugasan lembur tanpa approve masih bisa membatalkan lewat API (tombol disembunyikan); `period_closed` false bila tanggal belum punya baris periode; sel PDF fallback Next tanpa assert teks; `sent_at` diisi walau 0 penerima (kini ada log peringatan); uji klaim bersamaan dua instance digest belum ada (dijamin PK). Kolom CSV `Catatan` bergeser dari posisi 15 ke 17 (catat di catatan rilis).
- 2026-10-26: **Langkah rilis:** (1) terapkan migrasi `20261027090000_attendance_gap_digests.sql` lalu `20261027090100_iam_hris_approve.sql` SEBELUM API baru (kode baru di DB lama: semua keputusan HR 403 dan loop digest mencatat error tabel); migrasi sudah memanggil `iam.sync_hris_staff_roles()` bila `admin_hris` punya grant; (2) jalankan skrip cek akses SESUDAH migrasi: gerbang `lost` = 0 untuk akun nyata dan setiap `gained` ditinjau user; (3) build ulang API + web; (4) cek pasca-deploy: `approve-access` akun HR, kartu ringkasan, notifikasi 09.00 keesokan harinya.

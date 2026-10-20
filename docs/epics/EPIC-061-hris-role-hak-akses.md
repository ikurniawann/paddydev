# EPIC-061: Role & Hak Akses HRIS — Hak Hapus, Akses per Akun, AdminHRIS (RA-01..03)

status: backlog
environment: dev
retries: 0

## Goal
Menyelesaikan RA-01..03 MOM 19-10-2026:
- Hak hapus data per role (aksi `delete` ditegakkan, beralasan dan teraudit).
- Hak akses per akun email yang bertahan dari sync role turunan.
- AdminHRIS dapat mengatur akses Payroll, Recruitment, dan Absensi dalam cakupan terbatas.
- Audit perubahan hak akses.

Analisis & rekomendasi: `docs/hris/acuan-mom-20261019/role-hak-akses.md`.

## Tasks
Lihat `docs/superpowers/plans/2026-10-20-hris-role-hak-akses.md`.

## Acceptance Criteria
- Semua endpoint hapus/timpa HRIS butuh aksi `delete` pada menu daunnya, wajib alasan, dan tercatat di audit log.
- `hris_manager` punya hak hapus; `hris_non_payroll` dan `hris_non_rekrutmen_kpi` tidak; pengaturan ini bertahan setelah `iam.sync_hris_staff_roles()`.
- AdminHRIS bisa mengubah akses Payroll/Recruitment/Absensi untuk role `admin_hris`/`hris_*` tanpa bisa menyentuh role sistem atau menu non-HRIS.
- Perubahan grant dan penugasan role tercatat (before/after).

## Automation Log
- 2026-10-20: analisis RA-01..03 (hanya membaca kode). RA-01 ❌, RA-02 🟡 (menu per akun selesai di EPIC-057), RA-03 ❌. Menunggu keputusan user atas Q-RA-1..8.
- 2026-10-20: **Keputusan user**: "lanjut", yaitu ikuti semua rekomendasi Q-RA-1..8 (termasuk K-8). Hak hapus untuk `hris_manager` dan super_admin; hapus wajib alasan dan audit; AdminHRIS lewat halaman "Hak Akses HRIS" terbatas; masker per role bertahan dari sync; Manajemen User bisa memilih role `hris_*`; audit perubahan hak akses.

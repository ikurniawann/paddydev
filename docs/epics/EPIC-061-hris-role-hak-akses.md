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
- 2026-10-20: Task 1–9 selesai di branch `docs/epic061-role-hak-akses` (DB test saja). Hak hapus HRIS per menu daun + alasan + audit (`hrisdel.Guard`: 24 route DELETE + impor jadwal, ubah tipe jadwal, jadwal tetap, impor karyawan), masker role HRIS (`iam.hris_staff_role_masks`) bertahan dari sync, halaman Hak Akses HRIS (`hris.access`), Role HRIS di Manajemen User (super_admin), audit perubahan hak akses (`iam.permission_audit_logs`), role sistem hanya super_admin. Grant DB test: admin_hris 56, hris_manager 56, hris_non_payroll 51, hris_non_rekrutmen_kpi 39. Full suite Go = baseline. Live belum disentuh; cek R3 sebelum rollout.
- 2026-10-20: **Final review — perbaikan.** (1) Seeder `paddy-hris-admins.js` tidak lagi membangun ulang `admin_hris` dari `hrd` bila `admin_hris` sudah punya grant aktif: grant yang ada (termasuk `hris.access` dan hak hapus) dipertahankan, hanya menu HRD yang belum pernah dimiliki ditambahkan dan aksi non-hapus digabung; menu yang sengaja dimatikan tetap mati. (2) Role Management (Go + fallback Next): role `admin_hris`/`hris_*` (grant, nama/kode, hapus) dan penambahan `delete` di menu HRIS ke role mana pun hanya oleh Super Admin. (3) Hak Akses HRIS: menyalakan lagi grup di kolom Admin HRIS memulihkan grant turunan sebelumnya; menyalakan akses role `hris_*` pada menu yang tidak dimiliki `admin_hris` ditolak 400. (4) Seeder `iam-menus.sql` menawarkan `delete` di menu daun HRIS (sama dengan migrasi 20261022090000); `iam-role-permissions.sql` tidak memberi `hrd`/`hiring_manager` hak hapus di daun itu maupun `hris.access`. (5) Kolom "Admin HRIS (sumber)" di halaman Hak Akses HRIS.
- 2026-10-20: **Rollout live (belum dijalankan; keputusan user).** Dampak: pemegang role IAM `hrd` atau `hiring_manager` (termasuk akun berperan aplikasi `hrd` tanpa role IAM HRIS) kehilangan hak hapus di semua menu daun HRIS kecuali Hari Libur, Pengumuman, Penempatan: nonaktifkan karyawan, dokumen, kontrak, shift, master data, absensi, cuti, penggajian, salary, rekrutmen, KPI, impor jadwal yang menimpa jadwal, dan impor Excel karyawan yang mengubah tipe jadwal. Cek read-only sebelum rollout:
  - (a) `SELECT u.id, u.full_name, u.email FROM configuration.users u WHERE u.role = 'hrd' AND NOT EXISTS (SELECT 1 FROM iam.user_roles ur JOIN iam.roles r ON r.id = ur.role_id WHERE ur.user_id = u.id AND r.deleted_at IS NULL AND r.code IN ('admin_hris', 'hris_manager', 'hris_non_payroll', 'hris_non_rekrutmen_kpi', 'super_admin', 'admin')) ORDER BY u.full_name;`
  - (b) `SELECT u.id, u.full_name, u.email, u.role FROM iam.user_roles ur JOIN iam.roles r ON r.id = ur.role_id JOIN configuration.users u ON u.id = ur.user_id WHERE r.code = 'hiring_manager' ORDER BY u.full_name;`
  - Urutan: jalankan migrasi, lalu `SELECT iam.sync_hris_staff_roles();` di live hanya setelah `admin_hris` punya grant aktif.
  - Catatan: `hris.access` dan `hris.kepegawaian.schedules` sudah masuk daftar menu kanonik `seeders/iam-menus.sql`, jadi `db:seed:iam-menus` di DB yang sudah dimigrasi tidak lagi menghapus-lunak keduanya (dicek di transaksi yang di-rollback pada DB test).

# Role & Hak Akses HRIS — Analisis RA-01 s.d. RA-03

Dicek terhadap kode (branch `docs/epic061-role-hak-akses`, di atas EPIC-057/059/060) pada 20-10-2026. Hanya membaca kode, DB tidak disentuh. Acuan: MOM 19-10-2026 §2.3, §4.1, §5 (RA-01..03), K-8.

## Status

| ID | Requirement | Status MOM | Kode | Temuan |
|---|---|---|---|---|
| RA-01 | Pengaturan role untuk hak hapus data | Konfirmasi K-8 | ❌ | `granted_actions` bisa berisi `delete` dan ada helper `RequireMenuAction` (`backend/internal/platform/auth/auth.go`). Tetapi tidak ada endpoint DELETE HRIS yang mengeceknya: semuanya hanya mengecek prefiks menu atau peran `hrd`. Menu HRIS tidak menawarkan aksi `delete` (`seeders/iam-menus.sql`), sehingga UI Role tidak punya chip "hapus" untuk dinyalakan atau dimatikan. |
| RA-02 | Hak akses per akun email (bukan per role) sesuai §4.1 | Disepakati | 🟡 | Menu sudah selesai lewat EPIC-057: tiap akun HR punya role IAM sendiri. Celahnya: grant hanya bisa per role (tidak ada override per user); Manajemen User tidak bisa memilih role `hris_*` (hanya lewat skrip ops/seeder); dan beberapa cek masih memakai peran `hrd` (hapus absensi & cuti), sehingga melewati matriks per akun. |
| RA-03 | AdminHRIS dapat mengatur akses Payroll, Recruitment, Absensi | Disepakati | ❌ | `admin_hris`/`hris_manager` tidak punya menu `settings.*`, sehingga HR tidak bisa membuka Role/User Management (`configuration/access/routes.go` memakai `iam.SettingsRoles`). Sebaliknya, siapa pun yang memegang `settings.roles` (super_admin, admin, hrd) bisa mengubah role **apa pun**, termasuk role sistem. Tidak ada cek `is_system` maupun pembatasan cakupan. |

### Endpoint hapus HRIS dan guard-nya sekarang

| Endpoint | Guard sekarang |
|---|---|
| `DELETE /api/hris/employees/{id}` (nonaktifkan) | menu `hris.kepegawaian` atau `settings.users` |
| `DELETE /api/hris/employees/documents/{id}`, `/shifts/{id}`, `/holidays/{id}`, `/contracts/{id}` (draft), `/contracts/{id}/signed-document`, `/announcements/{id}` | prefiks menu `hris.kepegawaian` (daun apa pun cukup) |
| `DELETE /api/hris/employee-placements/{id}` | pengelola karyawan + `hris.master.placements` |
| `DELETE /api/hris/attendance/{id}` | peran super_admin/admin/hrd + alasan wajib + audit (satu-satunya yang diaudit) |
| `DELETE /api/hris/leaves/{id}` | peran hrd/admin/super_admin, tanpa audit |
| `DELETE /api/master/{departments,positions,employment-statuses}/{id}` | prefiks menu `hris.master` |
| `DELETE /api/hris/logbook` | menu + kepemilikan |
| Payroll (run, input manual, struktur gaji) | prefiks menu `hris.compensation` |
| Rekrutmen | menu `hris.recruitment` |
| Lembur, pinjaman | tidak ada endpoint DELETE |
| Import jadwal (`schedule_days.go`) & ubah tipe jadwal | menghapus & mengganti hari jadwal hanya dengan aksi `update` |

### Masalah tambahan

- **Sync role turunan menimpa pembatasan.** `iam.sync_hris_staff_roles()` menyalin grant `admin_hris` ditambah **semua** aksi yang tersedia di tiap menu. Akibatnya, `delete` yang dicabut dari role turunan kembali lagi saat sync berikutnya.
- **Tidak ada audit perubahan hak akses.** `replacePermissions` (`configuration/access/repo.go`) menonaktifkan lalu meng-upsert ulang grant tanpa riwayat before/after. Penugasan role ke user juga tidak tercatat.

## Pertanyaan & rekomendasi (K-8 dan turunannya)

| No | Pertanyaan | Rekomendasi |
|---|---|---|
| Q-RA-1 (K-8) | Data apa yang tercakup "hak hapus"? | Semua endpoint DELETE HRIS di tabel atas, ditambah impor jadwal mode timpa. Tiap endpoint dijaga aksi `delete` pada **menu daunnya** (bukan prefiks `hris.kepegawaian`). Payroll & rekrutmen ikut. |
| Q-RA-2 | Akun/role mana yang mendapat hak hapus? | `hris_manager` (Manager HR) dan super_admin. `hris_non_payroll` dan `hris_non_rekrutmen_kpi` tanpa hapus; mereka tetap bisa membuat & mengubah. |
| Q-RA-3 | Apakah "hapus" termasuk menonaktifkan karyawan dan impor jadwal yang menimpa? | Ya, keduanya dijaga `delete`. Alur Resign tetap alur sendiri (aksi `update` + persetujuan HR), bukan hapus. |
| Q-RA-4 | Wajib alasan & audit? | Ya: setiap hapus wajib alasan (≥ 5 karakter, seperti hapus absensi) dan tercatat di `audit.audit_log` (LA-01). |
| Q-RA-5 | Siapa boleh memberi/mencabut hak hapus dan mengatur akses Payroll/Recruitment/Absensi (RA-03)? | AdminHRIS (`hris_manager`) lewat halaman baru **"Hak Akses HRIS"** yang terbatas pada role `admin_hris` dan `hris_*`, dan hanya pada menu HRIS (`hris.compensation`, `hris.recruitment`, `hris.workforce`, plus aksi `delete`). Role sistem dan menu non-HRIS tetap hanya super_admin. Server menolak perubahan di luar cakupan itu. |
| Q-RA-6 | Pengaturan per akun email harus bertahan dari sync? | Ya. Ubah sync agar memakai **masker per role** yang disimpan di tabel (bukan daftar tetap di fungsi). Perubahan dari "Hak Akses HRIS" mengubah masker itu, sehingga hasilnya bertahan saat sync berikutnya. |
| Q-RA-7 | Manajemen User bisa memilih role `hris_*`? | Ya, untuk super_admin dan AdminHRIS. Peran aplikasi tetap `hrd`. Simpan ulang dengan peran yang sama tidak mengubah role IAM. |
| Q-RA-8 | Audit perubahan hak akses? | Ya: tabel `iam.permission_audit_logs` (aktor, role, menu, aksi before/after, waktu), ditulis setiap perubahan grant dan penugasan role ke user. |

## Rekomendasi urutan eksekusi (EPIC-061)

1. Aksi `delete` di menu daun HRIS + guard `delete` di semua endpoint hapus/timpa + alasan & audit wajib. Ganti cek peran `hrd` di hapus absensi & cuti.
2. Masker per role untuk sync role turunan (`hris_manager` penuh; dua role lain tanpa `delete`), disimpan di tabel.
3. Halaman "Hak Akses HRIS" untuk AdminHRIS (cakupan terbatas, ditegakkan server) + Manajemen User bisa memilih role `hris_*`.
4. Audit perubahan hak akses + batasi edit role sistem hanya untuk super_admin.

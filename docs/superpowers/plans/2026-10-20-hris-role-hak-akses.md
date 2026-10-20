# Role & Hak Akses HRIS (EPIC-061) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Hak hapus HRIS ditegakkan per menu daun (beralasan dan teraudit), hak akses per akun HR bertahan dari sync role turunan, AdminHRIS mengatur akses Payroll/Recruitment/Absensi dalam cakupan terbatas lewat halaman "Hak Akses HRIS", dan setiap perubahan hak akses tercatat.

**Architecture:**
- **Hak hapus (RA-01).** Setiap endpoint hapus/timpa HRIS dibungkus `hrisdel.Guard` (paket platform baru). Guard memeriksa aksi `delete` pada **menu daun** route itu (bukan prefiks grup), mewajibkan alasan ≥ 5 karakter (`?reason=` atau body JSON `{reason}`), lalu menulis `audit.audit_log` (`<entity>.delete`, baris yang dihapus sebagai `before`). Hapus absensi tetap memakai validasi dan auditnya sendiri; hanya cek peran `hrd` yang diganti. Impor jadwal yang menimpa dan perubahan tipe jadwal yang menghapus hari jadwal memakai `clearGuard` di modul HRIS dengan aturan yang sama.
- **Masker per role (RA-02, Q-RA-6).** Tabel `iam.hris_staff_role_masks` menyimpan, per role turunan, prefiks menu yang disembunyikan (`exclude_menu`) atau aksinya dikurangi (`excluded_actions`). `iam.sync_hris_staff_roles()` membaca tabel itu (prefiks terpanjang menang), sehingga `hris_manager` memegang `delete` dan dua role staf tidak. Perubahan dari halaman Hak Akses HRIS menulis masker, jadi bertahan setiap sync.
- **Hak Akses HRIS (RA-03, Q-RA-5/7).** Menu baru `hris.access` dan endpoint `/api/hris/access/**` di modul `configuration/access`. Server hanya menerima role `admin_hris` dan `hris_*`, toggle akses hanya di subtree `hris.compensation`/`hris.recruitment`/`hris.workforce`, dan toggle `delete` hanya di menu daun HRIS yang menawarkan `delete`. AdminHRIS juga menetapkan role HRIS ke akun HR (peran aplikasi tetap `hrd`). Manajemen User menampilkan field "Role HRIS" yang hanya bisa diubah super_admin.
- **Audit hak akses (Q-RA-8).** Tabel `iam.permission_audit_logs` diisi paket platform `permaudit`: diff grant per role × menu (Role Management, Hak Akses HRIS), perubahan masker, dan penugasan role ke user. Role sistem hanya bisa diubah super_admin.

**Tech Stack:** Go modular monolith (`backend/internal/{platform,modules}`), PostgreSQL 16 (migrasi `backend/database/migrations/deltas`), Next.js 16.3.8 (baca `frontend/AGENTS.md` dan `frontend/node_modules/next/dist/docs/` sebelum menulis kode Next), TanStack Query, vitest + testing-library.

**Spec:** `docs/hris/acuan-mom-20261019/role-hak-akses.md` (keputusan user 20-10-2026, Q-RA-1..8 semua diterima). Epic: `docs/epics/EPIC-061-hris-role-hak-akses.md`. Latar: `docs/epics/EPIC-057-hris-akses-per-akun.md`, migrasi `backend/database/migrations/deltas/20261020120000_iam_hris_staff_roles_guard.sql`.

## Global Constraints

- **DB hanya DB test:** `TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable'` (container `paddydev-sdd-testdb`). **Jangan** port 55432, jangan DB live, jangan container docker lain. Migrasi ke DB test: `cd backend && DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go run ./cmd/migrate -apply`.
- **Go:** `$HOME/.local/go/bin/go`. `make -C backend generate` dijalankan dengan `PATH=$HOME/.local/go/bin:$PATH`.
- **Nama migrasi:** `deltas/YYYYMMDDHHMMSS_*.sql`, harus urut setelah `20261021110000`: `20261022090000_iam_hris_delete_masks.sql` (Task 1), `20261022090100_iam_permission_audit_logs.sql` (Task 7), `20261022090200_iam_hris_access_menu.sql` (Task 8). Seeder `iam-menus.sql` tidak disentuh (menu baru sejak EPIC-056 hanya hidup di deltas).
- **Alasan hapus:** minimal 5 karakter (rune) setelah trim. Pesan persis `Alasan hapus wajib diisi (minimal 5 karakter)` = Go `hrisdel.ErrReason` = TS `HRIS_DELETE_REASON_ERROR`. Hapus absensi tetap `Alasan penyesuaian wajib diisi (minimal 5 karakter)`.
- **Audit hapus:** `audit.audit_log.action = "<entity>.delete"`, `entity_id` = id baris, `reason` = alasan, `before` = baris (`to_jsonb`) atau `{path, query}` bila tanpa id tunggal.
- **Siapa boleh hapus (Q-RA-2):** `hris_manager`, `admin_hris`, `super_admin`. `hris_non_payroll` dan `hris_non_rekrutmen_kpi` tidak, kecuali AdminHRIS memberinya lewat halaman Hak Akses HRIS. Role `admin` sudah memegang semua aksi (tidak diubah).
- **Pesan error baru (persis):**
  - 403 jadwal: `Menimpa atau menghapus jadwal yang sudah ada butuh hak hapus Jadwal Kerja`
  - 403 role sistem: `Role sistem hanya bisa diubah oleh Super Admin`
  - 400 admin_hris kosong: `admin_hris harus punya minimal satu menu: role HR lain diturunkan darinya`
  - Hak Akses HRIS: `Hak Akses HRIS hanya untuk role admin_hris dan hris_*` (403), `Menu di luar cakupan Hak Akses HRIS` (403), `Aktifkan akses menu dulu sebelum memberi hak hapus` (400), `Akun ini bukan akun HR (peran aplikasi hrd)` (403), `Tidak ada perubahan` (400)
  - Manajemen User: `Role HRIS hanya untuk akun dengan peran aplikasi hrd` (400), `Role HRIS hanya bisa diatur Super Admin di sini; AdminHRIS memakai halaman Hak Akses HRIS` (403)
  - Guard tanpa grant: pesan yang sudah ada `Insufficient permissions`.
- **Aturan user:** yang sudah benar tidak diubah. Jangan refaktor di luar file yang disebut tiap task.
- **Fixture test:** buat staff **sebelum** transaksi harness, atau daftarkan `t.Cleanup(rollback)` **setelah** `CreateStaff` (helper `h.staff`, `hrStaff`, `h.staffActions` sudah melakukannya). `testutil.GrantMenu` setelah transaksi aman: tidak ada baris di transaksi yang mengunci `iam.role_menu_permissions`.
- **Baseline:** sebelum Task 1 simpan daftar test Go yang sudah gagal di branch ini: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./... 2>&1 | grep -- '--- FAIL' | sort > /tmp/epic061-baseline.txt`. Kegagalan yang sama di akhir bukan regresi (mis. `TestAttendancePeriodClosing`).
- **Commit:** satu per task, di branch `docs/epic061-role-hak-akses` (tetap di branch ini), pesan diakhiri baris kosong lalu `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. **Tanpa push.**
- **Live:** tidak ada migrasi, skrip, atau deploy ke live. Rollout live adalah keputusan user terpisah (cek dulu dampak R3).

## Ruling (keputusan turunan, mohon dikoreksi saat review)

- **R1 — Peta menu daun.** Setiap route dijaga `delete` pada daunnya sendiri:

  | Route | Daun (`iam.Leaf*`) | Entity audit |
  |---|---|---|
  | `DELETE /api/hris/employees/{id}` (nonaktifkan) | `hris.kepegawaian.users` | `hris.employees` |
  | `DELETE /api/hris/employees/documents/{id}` | `hris.kepegawaian.users` | `hris.employee_documents` |
  | `DELETE /api/hris/employee-placements/{id}` | `hris.master.placements` | `hris.employee_placements` |
  | `DELETE /api/hris/attendance/{id}` | `hris.workforce.attendance` | (audit sendiri: `attendance.delete`) |
  | `DELETE /api/hris/leaves/{id}` | `hris.workforce.leaves` | `hris.leaves` |
  | `DELETE /api/hris/shifts/{id}` | `hris.kepegawaian.shifts` | `hris.shifts` |
  | `DELETE /api/hris/holidays/{id}` | `hris.kepegawaian.holidays` | `hris.public_holidays` |
  | `DELETE /api/hris/contracts/{id}` | `hris.kepegawaian.contracts` | `hris.employment_contracts` |
  | `DELETE /api/hris/contracts/{id}/signed-document` | `hris.kepegawaian.contracts` | `hris.contract_signed_document` |
  | `DELETE /api/hris/announcements/{id}` | `hris.kepegawaian.announcements` | `hris.announcements` |
  | `DELETE /api/master/{departments,employment-statuses,positions}/{id}` | `hris.master.{departments,employment-statuses,positions}` | `hris.{departments,employment_statuses,positions}` |
  | `DELETE /api/hris/payroll/{id}`, `/payroll/{id}/inputs` | `hris.compensation.payroll` | `hris.payroll_runs`, `hris.payroll_inputs` |
  | `DELETE /api/hris/employee-salary/{id}` | `hris.compensation.salary` | `hris.employee_salary` |
  | `DELETE /api/hris/kpi/targets`, `/feedback-{cycles,assignments,responses}/{id}` | `hris.performance.kpi-scorecard` | `performance.*` |
  | `DELETE /api/candidates/{id}`, `/candidates/{id}/cv-upload` | `hris.recruitment.candidates` | `recruitment.candidates`, `recruitment.candidate_cv` |
  | `DELETE /api/psikotes/questions/{id}` | `hris.recruitment.psikotes` | `recruitment.psikotes_questions` |
  | `DELETE /api/hris/job-openings/{id}` | `hris.recruitment.job-portal` | `recruitment.job_openings` |
  | Impor jadwal yang menimpa, ubah tipe jadwal / jadwal tetap yang menghapus hari | `hris.kepegawaian.schedules` | `hris.schedule.replace` / `hris.schedule.clear` |

  Hapus Kinerja (target KPI, 360 feedback) tidak ada di tabel spec, tetapi ikut dijaga. Tanpa itu `hris_non_payroll` tetap bisa menghapus di Kinerja, bertentangan dengan Q-RA-2. Daunnya `kpi-scorecard`, karena route-route itu memang hanya untuk pengelola Kinerja.
- **R2 — Logbook tidak diubah.** `DELETE /api/hris/logbook` adalah ESS: karyawan menghapus draft logbook-nya sendiri. Spec mencatat guard-nya "menu + kepemilikan". Kalau dijaga `delete`, semua karyawan kehilangan fitur itu.
- **R3 — Grant `delete`.** Aksi `delete` diberikan ke `admin_hris` dan `super_admin` (grant aktif yang sudah ada). `hris_manager` mendapatnya lewat sync.
  - Role IAM `hrd` tidak ditambah. Grant `delete` yang sudah dimilikinya (Hari Libur, Pengumuman, Penempatan) tidak dicabut.
  - Pemegang role IAM `hrd` atau `hiring_manager` (termasuk akun berperan aplikasi `hrd` tanpa role IAM HRIS, yang jatuh ke role IAM `hrd`) kehilangan hak hapus di **semua** menu daun HRIS kecuali Hari Libur, Pengumuman, dan Penempatan: nonaktifkan karyawan, dokumen, kontrak, shift, master data (departemen, jabatan, status karyawan), absensi, cuti, penggajian, salary, rekrutmen, KPI, impor jadwal yang menimpa jadwal, dan impor Excel karyawan yang mengubah tipe jadwal. Ini sesuai Q-RA-2. *(Dikoreksi di final review: sebelumnya hanya disebut absensi dan cuti.)*
  - **Cek live sebelum rollout (read-only):**
    - (a) Akun berperan aplikasi `hrd` tanpa role HR/super_admin/admin:
      `SELECT u.id, u.full_name, u.email FROM configuration.users u WHERE u.role = 'hrd' AND NOT EXISTS (SELECT 1 FROM iam.user_roles ur JOIN iam.roles r ON r.id = ur.role_id WHERE ur.user_id = u.id AND r.deleted_at IS NULL AND r.code IN ('admin_hris', 'hris_manager', 'hris_non_payroll', 'hris_non_rekrutmen_kpi', 'super_admin', 'admin')) ORDER BY u.full_name;`
    - (b) Pemegang role IAM `hiring_manager`:
      `SELECT u.id, u.full_name, u.email, u.role FROM iam.user_roles ur JOIN iam.roles r ON r.id = ur.role_id JOIN configuration.users u ON u.id = ur.user_id WHERE r.code = 'hiring_manager' ORDER BY u.full_name;`
    - *(Dikoreksi di final review: query lama `NOT EXISTS (... iam.user_roles ...)` tanpa filter kode role melewatkan akun `hrd` yang memegang role IAM `hrd` sendiri.)*
  - Urutan live: jalankan migrasi, lalu `SELECT iam.sync_hris_staff_roles();` hanya setelah `admin_hris` punya grant aktif.
- **R4 — Jalur timpa jadwal.** Guard hanya berlaku bila ada data yang benar-benar hilang:
  - **Impor jadwal:** butuh `delete` + alasan hanya bila mengganti ≥ 1 hari jadwal yang sudah ada dengan shift berbeda. Pratinjau mengembalikan `replaced_days`.
  - **Ubah tipe jadwal & jadwal tetap:** butuh `delete` + alasan hanya bila menghapus ≥ 1 hari jadwal per tanggal. Mengakhiri pola mingguan (Non-Shift → Shift) bukan "hapus".
  - **Impor data karyawan (Excel):** memakai guard yang sama. Bila form tidak mengirim `reason`, alasannya otomatis `Impor data karyawan: <nama file>`, jadi UI impor tidak berubah.
- **R5 — Nonaktifkan karyawan** = `DELETE /api/hris/employees/{id}`, dijaga `delete` di Karyawan. Alur Resign tetap aksi `update` (Q-RA-3).
- **R6 — Waktu tulis audit.**
  - Guard menulis audit **setelah** handler menjawab 2xx, memakai DB modul (pool di produksi, transaksi di test). Handler dan audit tidak dalam satu transaksi karena handler punya transaksinya sendiri. Bila tulis audit gagal, error dicatat ke log dan respons tetap 2xx.
  - Hapus absensi tetap menulis audit di dalam transaksinya sendiri (`attendance.delete`, seperti sekarang).
- **R7 — Semantik masker.**
  - Prefiks terpanjang yang cocok menang. `read` tidak bisa dimasker (CHECK).
  - Role turunan = role yang punya ≥ 1 baris masker.
  - Grup induk dari daun yang diizinkan otomatis mendapat `["read"]`, supaya menu tetap tampil.
  - Seed (setara EPIC-057, ditambah tanpa `delete`):
    - `hris_manager`: `hris` (tanpa pengurangan);
    - `hris_non_payroll`: `hris` −`delete`, `hris.compensation` disembunyikan;
    - `hris_non_rekrutmen_kpi`: `hris` −`delete`, `hris.recruitment`/`hris.performance`/`hris.insights.analytics` disembunyikan;
    - Task 8 menambah `hris.access` disembunyikan untuk kedua role staf.
- **R8 — Hak Akses HRIS.**
  - **Menu:** `hris.access` "Hak Akses HRIS" (sidebar di bawah HRIS, `/dashboard/hris/hak-akses`, aksi `read`, `update`). Diberikan ke `admin_hris` (→ `hris_manager` lewat sync) dan `super_admin`.
  - **Perubahan pada `admin_hris`** menulis grant-nya langsung, karena role ini sumber.
    - Akses on → aksi menu tanpa `delete`, ditambah `read` untuk induk yang belum ada.
    - Akses off → nonaktifkan menu beserta anaknya.
  - **Perubahan pada `hris_*`** menulis masker:
    - akses off → hapus juga masker yang lebih spesifik di bawahnya;
    - akses on dan delete mewarisi pengurangan dari masker induk terdekat.
  - Keduanya menjalankan sync di transaksi yang sama.
- **R9 — Penugasan role HRIS.**
  - **AdminHRIS** menetapkan role di halaman Hak Akses HRIS, khusus akun yang peran aplikasinya `hrd` dan role IAM-nya hanya `hrd`/role HRIS.
  - **Manajemen User** menampilkan "Role HRIS", tetapi hanya super_admin yang boleh mengubahnya. AdminHRIS tidak punya `settings.users`, dan pemegang `settings.users` lain (admin, hrd) tidak boleh menaikkan dirinya sendiri.
  - Fallback Next `/api/users` mengabaikan field ini, karena Go yang melayani `/api/users`.
- **R10 — Role Management.**
  - Menyimpan grant `admin_hris` langsung menjalankan sync di transaksi yang sama, supaya role turunan dan masker tetap konsisten. Daftar kosong untuk admin_hris ditolak 400.
  - Ubah role sistem (`is_system`): nama/kode lewat PUT role dan grant lewat PUT permissions, hanya untuk pengguna dengan role IAM `super_admin`.
  - Hapus role sistem tetap ditolak untuk semua orang (perilaku lama).
- **R11 — Cakupan audit hak akses.**
  - Tercatat: perubahan lewat aplikasi (Role Management, Hak Akses HRIS, masker, penugasan role lewat Manajemen User / Hak Akses HRIS).
  - Tidak tercatat: migrasi SQL dan skrip ops, karena tidak punya aktor.
  - Tampil di bagian "Riwayat perubahan" halaman Hak Akses HRIS. Pemegang `settings.roles` melihat semua baris; pemegang `hris.access` saja melihat baris role HRIS.
- **R12 — UX alasan hapus.**
  - Satu dialog alasan global (`askDeleteReason`) muncul setelah konfirmasi yang sudah ada di tiap halaman. Batal menghasilkan toast `Penghapusan dibatalkan`.
  - Alur jadwal meminta alasan hanya bila server menjawab `ErrReason` (`retryWithReason`).
  - Mutasi hapus memakai `retry: false`, supaya dialog tidak muncul dua kali.
- **R13 — Fallback Next.** Route Next untuk path DELETE yang sama (22 file) memakai guard yang setara (`requireHrisDelete` + `auditHrisDelete`). Endpoint baru `/api/hris/access/**` hanya ada di Go.

## Review Focus

1. **Sync menimpa pengaturan per akun.** Setelah "Hak Akses HRIS" memberi `hris_non_payroll` hak hapus Absensi, `SELECT iam.sync_hris_staff_roles()` berikutnya (migrasi, skrip ops, simpan admin_hris di Role Management) harus mempertahankannya. Test: `TestHrisStaffRoleMasksSurviveSync` (Task 1), `TestHrisAccessMasksSurviveSync` (Task 8), `TestAdminHrisEditResyncsDerivedRoles` (Task 7).
2. **Prefiks bocor.** Grant `delete` pada grup (`hris.kepegawaian`, `hris`) atau pada daun saudara tidak boleh meloloskan hapus di daun lain. Test: `TestGuard` (Task 2), `TestHrisDeletesNeedLeafDeleteAndReason` (Task 3), `TestEveryHrisDeleteNeedsLeafDelete` (Task 4).
3. **Eskalasi AdminHRIS.** PUT ke role `super_admin`/`admin`/`hrd`/`pos`, ke menu non-HRIS (`settings.roles`), ke `hris.access` sendiri, atau toggle akses Kepegawaian harus 403. Menetapkan role ke akun super_admin atau non-`hrd` juga 403. Pemegang `settings.users` non-super_admin yang mengirim `hris_role` harus 403. Test: `TestHrisAccessScope`, `TestHrisAccessAccounts`, `TestHrisRoleInManajemenUser` (Task 8).
4. **Simpan ulang di Manajemen User menghapus role `hris_*`.** Menyimpan akun HR dengan peran aplikasi yang sama tanpa mengubah "Role HRIS" tidak boleh mengubah role IAM. Frontend mengirim `hris_role` hanya bila nilainya berubah. Test: `TestHrisRoleInManajemenUser` (Task 8), `users-form.test.ts` (Task 9).
5. **Jalur hapus terlewat.** Setiap route DELETE modul `hris`/`payroll`/`recruitment` (kecuali logbook, R2) harus menolak staf yang hanya memegang grup HRIS dengan semua aksi. Test inventaris: `TestEveryHrisDeleteNeedsLeafDelete` (Task 4) membaca `app.Routes()`, jadi route DELETE baru yang lupa dibungkus akan gagal.

---

### Task 1: Aksi `delete` di menu daun HRIS + masker role turunan

**Files:**
- Create: `backend/database/migrations/deltas/20261022090000_iam_hris_delete_masks.sql`
- Create: `backend/internal/platform/iam/hris.go`
- Modify (ganti seluruh isi): `backend/internal/platform/auth/hris_staff_roles_integration_test.go`

**Interfaces:**
- Produces:
  - Konstanta Go `iam.LeafKaryawan`, `iam.LeafKontrak`, `iam.LeafShift`, `iam.LeafJadwalKerja`, `iam.LeafHariLibur`, `iam.LeafPengumuman`, `iam.LeafPenempatan`, `iam.LeafDepartemen`, `iam.LeafJabatan`, `iam.LeafStatusKaryawan`, `iam.LeafAbsensi`, `iam.LeafCuti`, `iam.LeafRunPenggajian`, `iam.LeafSalary`, `iam.LeafKPIScorecard`, `iam.LeafKandidat`, `iam.LeafPsikotes`, `iam.LeafJobPortal` (string);
  - `iam.HrisDeleteLeaves []string`, `iam.HrisStaffRoleCodes []string` (`admin_hris`, `hris_manager`, `hris_non_payroll`, `hris_non_rekrutmen_kpi`), `iam.HrisAccessScopes []string` (`hris.compensation`, `hris.recruitment`, `hris.workforce`);
  - tabel `iam.hris_staff_role_masks(role_code, menu_prefix, exclude_menu bool, excluded_actions text[], updated_at, updated_by)` PK `(role_code, menu_prefix)`, CHECK `hris_staff_role_masks_read_kept`;
  - `iam.sync_hris_staff_roles()` versi masker (pengaman admin_hris kosong tetap).

- [ ] **Step 1: Tulis konstanta IAM**

`backend/internal/platform/iam/hris.go`:

```go
package iam

// HRIS leaf menus whose deletes and destructive overwrites need the
// "delete" action (EPIC-061, Q-RA-1). Each route checks its own leaf,
// never a group prefix, so a delete grant on a sibling leaf or a group
// does not leak.
const (
	LeafKaryawan       = "hris.kepegawaian.users"
	LeafKontrak        = "hris.kepegawaian.contracts"
	LeafShift          = "hris.kepegawaian.shifts"
	LeafJadwalKerja    = "hris.kepegawaian.schedules"
	LeafHariLibur      = "hris.kepegawaian.holidays"
	LeafPengumuman     = "hris.kepegawaian.announcements"
	LeafPenempatan     = "hris.master.placements"
	LeafDepartemen     = "hris.master.departments"
	LeafJabatan        = "hris.master.positions"
	LeafStatusKaryawan = "hris.master.employment-statuses"
	LeafAbsensi        = "hris.workforce.attendance"
	LeafCuti           = "hris.workforce.leaves"
	LeafRunPenggajian  = "hris.compensation.payroll"
	LeafSalary         = "hris.compensation.salary"
	LeafKPIScorecard   = "hris.performance.kpi-scorecard"
	LeafKandidat       = "hris.recruitment.candidates"
	LeafPsikotes       = "hris.recruitment.psikotes"
	LeafJobPortal      = "hris.recruitment.job-portal"
)

// HrisDeleteLeaves lists every leaf above; the EPIC-061 migration offers
// and grants "delete" on them.
var HrisDeleteLeaves = []string{
	LeafKaryawan, LeafKontrak, LeafShift, LeafJadwalKerja, LeafHariLibur, LeafPengumuman,
	LeafPenempatan, LeafDepartemen, LeafJabatan, LeafStatusKaryawan, LeafAbsensi, LeafCuti,
	LeafRunPenggajian, LeafSalary, LeafKPIScorecard, LeafKandidat, LeafPsikotes, LeafJobPortal,
}

// HrisStaffRoleCodes are the roles "Hak Akses HRIS" may change and assign:
// admin_hris (the source) and the roles iam.sync_hris_staff_roles derives
// from it. Keep in step with iam.hris_staff_role_masks.
var HrisStaffRoleCodes = []string{"admin_hris", "hris_manager", "hris_non_payroll", "hris_non_rekrutmen_kpi"}

// HrisAccessScopes are the subtrees whose menu access AdminHRIS may toggle
// (Q-RA-5: Penggajian, Rekrutmen, Kehadiran & Cuti).
var HrisAccessScopes = []string{"hris.compensation", "hris.recruitment", "hris.workforce"}
```

- [ ] **Step 2: Tulis test yang gagal**

Ganti seluruh isi `backend/internal/platform/auth/hris_staff_roles_integration_test.go`:

```go
package auth_test

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/testutil"
)

func roleGrants(t *testing.T, tx pgx.Tx, role string) map[string][]string {
	t.Helper()
	rows, err := tx.Query(context.Background(), `SELECT m.code, p.granted_actions
		FROM iam.role_menu_permissions p
		JOIN iam.roles r ON r.id = p.role_id
		JOIN iam.menus m ON m.id = p.menu_id
		WHERE r.code = $1 AND p.is_active AND m.deleted_at IS NULL`, role)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var code string
		var raw []byte
		if err := rows.Scan(&code, &raw); err != nil {
			t.Fatal(err)
		}
		var acts []string
		_ = json.Unmarshal(raw, &acts)
		slices.Sort(acts)
		out[code] = acts
	}
	return out
}

func under(code string, prefixes ...string) bool {
	for _, p := range prefixes {
		if code == p || strings.HasPrefix(code, p+".") {
			return true
		}
	}
	return false
}

func menuActions(t *testing.T, tx pgx.Tx) map[string][]string {
	t.Helper()
	rows, err := tx.Query(context.Background(), `SELECT code, coalesce(permission_context->'actions', '[]'::jsonb) FROM iam.menus WHERE deleted_at IS NULL`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var code string
		var raw []byte
		if err := rows.Scan(&code, &raw); err != nil {
			t.Fatal(err)
		}
		var a []string
		_ = json.Unmarshal(raw, &a)
		out[code] = a
	}
	return out
}

func syncTx(t *testing.T) pgx.Tx {
	t.Helper()
	tx := testutil.Tx(t)
	if _, err := tx.Exec(context.Background(), `SELECT iam.sync_hris_staff_roles()`); err != nil {
		t.Fatal(err)
	}
	return tx
}

// staffRoleSpec is the seeded mask of each derived role (EPIC-057 subtrees,
// EPIC-061: no "delete" on HRIS for the two staff roles; hris.access comes
// with Task 8 and is hidden from them).
var staffRoleSpec = map[string]struct {
	excluded []string
	noDelete bool
}{
	"hris_manager":           {nil, false},
	"hris_non_payroll":       {[]string{"hris.compensation", "hris.access"}, true},
	"hris_non_rekrutmen_kpi": {[]string{"hris.recruitment", "hris.performance", "hris.insights.analytics", "hris.access"}, true},
}

func TestHrisStaffRolesDeriveFromAdminHris(t *testing.T) {
	tx := syncTx(t)
	ctx := context.Background()
	src := roleGrants(t, tx, "admin_hris")
	if len(src) == 0 {
		t.Skip("admin_hris has no grants in this database")
	}
	menuActs := menuActions(t, tx)
	for role, spec := range staffRoleSpec {
		exp := map[string][]string{}
		for code, acts := range src {
			if under(code, spec.excluded...) {
				continue
			}
			all := slices.Concat(acts, menuActs[code])
			slices.Sort(all)
			all = slices.Compact(all)
			if spec.noDelete && under(code, "hris") {
				all = slices.DeleteFunc(all, func(a string) bool { return a == "delete" })
			}
			exp[code] = all
		}
		got := roleGrants(t, tx, role)
		for code, acts := range exp {
			if !slices.Equal(got[code], acts) {
				t.Errorf("%s %s = %v, want %v", role, code, got[code], acts)
			}
		}
		for code := range got {
			if _, ok := exp[code]; !ok {
				t.Errorf("%s has extra grant %s", role, code)
			}
		}
	}
	// Pinjaman stays for Non-Payroll; Laporan HRIS stays for Non-Rekrutmen & KPI.
	if np := roleGrants(t, tx, "hris_non_payroll"); !slices.Contains(np["hris.kepegawaian.loans"], "approve") {
		t.Errorf("non-payroll loans = %v", np["hris.kepegawaian.loans"])
	}
	if nr := roleGrants(t, tx, "hris_non_rekrutmen_kpi"); nr["hris.insights.reports"] == nil {
		t.Error("non-rekrutmen lost Laporan HRIS")
	}
	// Re-running deactivates a grant the source no longer has.
	if _, err := tx.Exec(ctx, `UPDATE iam.role_menu_permissions p SET is_active = false
		FROM iam.roles r, iam.menus m WHERE r.id = p.role_id AND m.id = p.menu_id
		AND r.code = 'admin_hris' AND m.code = 'hris.kepegawaian.holidays'`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SELECT iam.sync_hris_staff_roles()`); err != nil {
		t.Fatal(err)
	}
	if roleGrants(t, tx, "hris_manager")["hris.kepegawaian.holidays"] != nil {
		t.Error("sync kept a grant admin_hris dropped")
	}
}

// With no active admin_hris grant the sync refuses instead of emptying the
// three HR roles (EPIC-057).
func TestHrisStaffRolesSyncRefusesEmptySource(t *testing.T) {
	db := testutil.DB(t)
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	before := roleGrants(t, tx, "hris_manager")
	if _, err := tx.Exec(ctx, `UPDATE iam.role_menu_permissions SET is_active = false
		WHERE role_id = (SELECT id FROM iam.roles WHERE code = 'admin_hris')`); err != nil {
		t.Fatal(err)
	}
	sp, err := tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = sp.Exec(ctx, `SELECT iam.sync_hris_staff_roles()`)
	if err == nil || !strings.Contains(err.Error(), "admin_hris tidak punya grant aktif") {
		t.Fatalf("sync with empty admin_hris = %v, want refusal", err)
	}
	_ = sp.Rollback(ctx)
	if after := roleGrants(t, tx, "hris_manager"); len(after) != len(before) {
		t.Fatalf("hris_manager grants %d -> %d", len(before), len(after))
	}
}

// Every HRIS delete leaf offers "delete"; admin_hris, super_admin and
// hris_manager hold it, the two staff roles do not (Q-RA-2).
func TestHrisDeleteActionOnLeaves(t *testing.T) {
	tx := syncTx(t)
	menuActs := menuActions(t, tx)
	admin, super := roleGrants(t, tx, "admin_hris"), roleGrants(t, tx, "super_admin")
	m, np, nr := roleGrants(t, tx, "hris_manager"), roleGrants(t, tx, "hris_non_payroll"), roleGrants(t, tx, "hris_non_rekrutmen_kpi")
	for _, code := range iam.HrisDeleteLeaves {
		if !slices.Contains(menuActs[code], "delete") {
			t.Errorf("%s does not offer delete: %v", code, menuActs[code])
		}
		for role, g := range map[string]map[string][]string{"admin_hris": admin, "super_admin": super, "hris_manager": m} {
			if acts, ok := g[code]; ok && !slices.Contains(acts, "delete") {
				t.Errorf("%s %s = %v, want delete", role, code, acts)
			}
		}
		if slices.Contains(np[code], "delete") || slices.Contains(nr[code], "delete") {
			t.Errorf("%s: a staff role holds delete (np %v, nr %v)", code, np[code], nr[code])
		}
	}
	if !slices.Contains(m[iam.LeafAbsensi], "delete") {
		t.Errorf("hris_manager Absensi = %v, want delete", m[iam.LeafAbsensi])
	}
}

// A mask row (what "Hak Akses HRIS" writes) survives every sync: the most
// specific prefix wins and the parent group of an allowed leaf stays
// visible with read.
func TestHrisStaffRoleMasksSurviveSync(t *testing.T) {
	tx := syncTx(t)
	ctx := context.Background()
	if _, err := tx.Exec(ctx, `INSERT INTO iam.hris_staff_role_masks (role_code, menu_prefix, exclude_menu, excluded_actions) VALUES
		('hris_non_payroll', 'hris.kepegawaian.holidays', false, '{}'),
		('hris_non_payroll', 'hris.compensation.salary', false, '{delete}')`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := tx.Exec(ctx, `SELECT iam.sync_hris_staff_roles()`); err != nil {
			t.Fatal(err)
		}
	}
	np := roleGrants(t, tx, "hris_non_payroll")
	if !slices.Contains(np[iam.LeafHariLibur], "delete") {
		t.Errorf("holidays = %v, want delete from the leaf mask", np[iam.LeafHariLibur])
	}
	if np[iam.LeafSalary] == nil || slices.Contains(np[iam.LeafSalary], "delete") {
		t.Errorf("salary = %v, want granted without delete", np[iam.LeafSalary])
	}
	if !slices.Equal(np["hris.compensation"], []string{"read"}) {
		t.Errorf("hris.compensation = %v, want [read] as parent of an allowed leaf", np["hris.compensation"])
	}
	if np[iam.LeafRunPenggajian] != nil {
		t.Errorf("payroll = %v, want still hidden", np[iam.LeafRunPenggajian])
	}
	if slices.Contains(np[iam.LeafAbsensi], "delete") {
		t.Errorf("attendance = %v, want no delete", np[iam.LeafAbsensi])
	}
	sp, err := tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = sp.Exec(ctx, `INSERT INTO iam.hris_staff_role_masks (role_code, menu_prefix, excluded_actions) VALUES ('hris_manager', 'hris.workforce', '{read}')`)
	if err == nil || !strings.Contains(err.Error(), "hris_staff_role_masks_read_kept") {
		t.Fatalf("masking read = %v, want the CHECK to refuse", err)
	}
	_ = sp.Rollback(ctx)
}
```

- [ ] **Step 3: Jalankan test, pastikan gagal**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/platform/auth -run 'TestHrisStaffRole|TestHrisDeleteAction' -count=1 -v`
Expected: FAIL. Contoh pesan:
- `hris.kepegawaian.users does not offer delete`;
- `hris_non_payroll hris.kepegawaian.holidays = [approve create delete ...], want [...]`;
- `relation "iam.hris_staff_role_masks" does not exist`.

- [ ] **Step 4: Tulis migrasi**

`backend/database/migrations/deltas/20261022090000_iam_hris_delete_masks.sql`:

```sql
-- =============================================================================
-- EPIC-061 (RA-01/RA-02, Q-RA-1/2/6): hak hapus HRIS per menu daun dan
-- masker per role untuk role HR turunan admin_hris.
--
-- 1) Aksi "delete" ditawarkan di semua menu daun HRIS yang punya endpoint
--    hapus/timpa, lalu diberikan ke admin_hris dan super_admin (grant aktif
--    yang sudah ada). Role admin sudah memegang semua aksi; hrd tidak diubah.
-- 2) iam.hris_staff_role_masks: per role turunan, prefix menu yang
--    disembunyikan (exclude_menu) atau aksinya dikurangi (excluded_actions).
--    Prefix terpanjang yang cocok menang; "read" tidak bisa dimasker.
-- 3) iam.sync_hris_staff_roles() membaca masker itu. hris_manager penuh,
--    hris_non_payroll dan hris_non_rekrutmen_kpi tanpa "delete". Halaman
--    Hak Akses HRIS menulis masker, jadi perubahannya bertahan setiap sync.
-- =============================================================================
UPDATE iam.menus m
SET permission_context = jsonb_set(m.permission_context, '{actions}',
      coalesce(m.permission_context->'actions', '["read"]'::jsonb) || '["delete"]'::jsonb),
    updated_at = now()
WHERE m.deleted_at IS NULL
  AND m.code IN ('hris.kepegawaian.users', 'hris.kepegawaian.contracts', 'hris.kepegawaian.shifts',
                 'hris.kepegawaian.schedules', 'hris.master.departments', 'hris.master.positions',
                 'hris.master.employment-statuses', 'hris.workforce.attendance', 'hris.workforce.leaves',
                 'hris.compensation.payroll', 'hris.compensation.salary', 'hris.performance.kpi-scorecard',
                 'hris.recruitment.candidates', 'hris.recruitment.psikotes', 'hris.recruitment.job-portal')
  AND NOT coalesce(m.permission_context->'actions', '[]'::jsonb) ? 'delete';

UPDATE iam.role_menu_permissions p
SET granted_actions = p.granted_actions || '["delete"]'::jsonb, updated_at = now()
FROM iam.roles r, iam.menus m
WHERE r.id = p.role_id AND m.id = p.menu_id AND p.is_active
  AND r.code IN ('admin_hris', 'super_admin')
  AND m.code IN ('hris.kepegawaian.users', 'hris.kepegawaian.contracts', 'hris.kepegawaian.shifts',
                 'hris.kepegawaian.schedules', 'hris.kepegawaian.holidays', 'hris.kepegawaian.announcements',
                 'hris.master.placements', 'hris.master.departments', 'hris.master.positions',
                 'hris.master.employment-statuses', 'hris.workforce.attendance', 'hris.workforce.leaves',
                 'hris.compensation.payroll', 'hris.compensation.salary', 'hris.performance.kpi-scorecard',
                 'hris.recruitment.candidates', 'hris.recruitment.psikotes', 'hris.recruitment.job-portal')
  AND NOT p.granted_actions ? 'delete';

CREATE TABLE IF NOT EXISTS iam.hris_staff_role_masks (
  role_code        varchar(50)  NOT NULL REFERENCES iam.roles(code) ON UPDATE CASCADE ON DELETE CASCADE,
  menu_prefix      varchar(100) NOT NULL,
  exclude_menu     boolean      NOT NULL DEFAULT false,
  excluded_actions text[]       NOT NULL DEFAULT '{}',
  updated_at       timestamptz  NOT NULL DEFAULT now(),
  updated_by       uuid         REFERENCES auth.users(id) ON DELETE SET NULL,
  PRIMARY KEY (role_code, menu_prefix),
  CONSTRAINT hris_staff_role_masks_read_kept CHECK (NOT ('read' = ANY (excluded_actions)))
);

INSERT INTO iam.hris_staff_role_masks (role_code, menu_prefix, exclude_menu, excluded_actions) VALUES
  ('hris_manager', 'hris', false, '{}'),
  ('hris_non_payroll', 'hris', false, '{delete}'),
  ('hris_non_payroll', 'hris.compensation', true, '{}'),
  ('hris_non_rekrutmen_kpi', 'hris', false, '{delete}'),
  ('hris_non_rekrutmen_kpi', 'hris.recruitment', true, '{}'),
  ('hris_non_rekrutmen_kpi', 'hris.performance', true, '{}'),
  ('hris_non_rekrutmen_kpi', 'hris.insights.analytics', true, '{}')
ON CONFLICT (role_code, menu_prefix) DO NOTHING;

CREATE OR REPLACE FUNCTION iam.sync_hris_staff_roles() RETURNS void
LANGUAGE plpgsql AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM iam.role_menu_permissions rmp
    JOIN iam.roles r ON r.id = rmp.role_id AND r.code = 'admin_hris'
    JOIN iam.menus m ON m.id = rmp.menu_id AND m.deleted_at IS NULL
    WHERE rmp.is_active) THEN
    RAISE EXCEPTION 'admin_hris tidak punya grant aktif: sync role HR dibatalkan agar akun HR tidak kehilangan akses';
  END IF;
  WITH derived AS (
    SELECT DISTINCT r.id AS role_id, r.code
    FROM iam.hris_staff_role_masks k
    JOIN iam.roles r ON r.code = k.role_code AND r.deleted_at IS NULL),
  src AS (
    SELECT m.id AS menu_id, m.code,
      (SELECT jsonb_agg(DISTINCT a ORDER BY a)
         FROM jsonb_array_elements_text(rmp.granted_actions || coalesce(m.permission_context->'actions', '[]'::jsonb)) a) AS actions
    FROM iam.role_menu_permissions rmp
    JOIN iam.roles r ON r.id = rmp.role_id AND r.code = 'admin_hris'
    JOIN iam.menus m ON m.id = rmp.menu_id AND m.deleted_at IS NULL
    WHERE rmp.is_active),
  resolved AS (
    SELECT d.role_id, s.menu_id, s.code, s.actions,
      coalesce(mk.exclude_menu, false) AS exclude_menu,
      coalesce(mk.excluded_actions, '{}'::text[]) AS excluded
    FROM derived d
    CROSS JOIN src s
    LEFT JOIN LATERAL (
      SELECT x.exclude_menu, x.excluded_actions
      FROM iam.hris_staff_role_masks x
      WHERE x.role_code = d.code AND (s.code = x.menu_prefix OR s.code LIKE x.menu_prefix || '.%')
      ORDER BY length(x.menu_prefix) DESC
      LIMIT 1) mk ON true),
  kept AS (
    SELECT role_id, menu_id, code,
      (SELECT coalesce(jsonb_agg(a ORDER BY a), '[]'::jsonb)
         FROM jsonb_array_elements_text(actions) a WHERE NOT (a = ANY (excluded))) AS actions
    FROM resolved
    WHERE NOT exclude_menu),
  ancestors AS (
    SELECT DISTINCT k.role_id, s.menu_id, '["read"]'::jsonb AS actions
    FROM kept k
    JOIN src s ON k.code LIKE s.code || '.%'
    WHERE NOT EXISTS (SELECT 1 FROM kept k2 WHERE k2.role_id = k.role_id AND k2.menu_id = s.menu_id)),
  want AS (
    SELECT role_id, menu_id, actions FROM kept
    UNION ALL
    SELECT role_id, menu_id, actions FROM ancestors),
  upserted AS (
    INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions, is_active)
    SELECT role_id, menu_id, actions, true FROM want
    ON CONFLICT ON CONSTRAINT role_menu_permissions_unique
    DO UPDATE SET granted_actions = EXCLUDED.granted_actions, is_active = true, updated_at = now()
    RETURNING 1)
  UPDATE iam.role_menu_permissions p SET is_active = false, updated_at = now()
  FROM derived d
  WHERE p.role_id = d.role_id AND p.is_active
    AND NOT EXISTS (SELECT 1 FROM want w WHERE w.role_id = p.role_id AND w.menu_id = p.menu_id);
END
$$;

SELECT iam.sync_hris_staff_roles();
```

Terapkan ke DB test:

Run: `cd backend && DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go run ./cmd/migrate -apply`
Expected: `  [ok]    20261022090000_iam_hris_delete_masks.sql` lalu `Selesai. 1 migrasi diterapkan.`

- [ ] **Step 5: Jalankan test, pastikan lolos**

Run: perintah Step 3.
Expected: PASS (4 test).

Cek jumlah grant tidak berubah (hanya aksinya):

Run: `psql 'postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' -Atc "SELECT r.code, count(*) FROM iam.role_menu_permissions p JOIN iam.roles r ON r.id = p.role_id WHERE p.is_active AND r.code IN ('admin_hris','hris_manager','hris_non_payroll','hris_non_rekrutmen_kpi') GROUP BY 1 ORDER BY 1"`
Expected:
```
admin_hris|55
hris_manager|55
hris_non_payroll|51
hris_non_rekrutmen_kpi|39
```

- [ ] **Step 6: Commit**

```bash
git add backend/database/migrations/deltas/20261022090000_iam_hris_delete_masks.sql backend/internal/platform/iam/hris.go backend/internal/platform/auth/hris_staff_roles_integration_test.go
git commit -m "feat(iam): aksi delete di menu daun HRIS dan masker role turunan admin_hris, EPIC-061

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: Guard hapus HRIS di platform (`hrisdel`)

**Files:**
- Create: `backend/internal/platform/hrisdel/guard.go`
- Test: `backend/internal/platform/hrisdel/guard_integration_test.go`

**Interfaces:**
- Consumes: `(*auth.Service).RequireMenuAction(r, action, prefixes...) (*auth.User, error)`, `audit.Write`, `audit.Entry.WithRequest`, `httpx.WriteError`.
- Produces:
  - `const hrisdel.MinReason = 5`, `const hrisdel.ErrReason = "Alasan hapus wajib diisi (minimal 5 karakter)"`;
  - `type hrisdel.Checker interface { RequireMenuAction(r *http.Request, action string, prefixes ...string) (*auth.User, error) }`;
  - `type hrisdel.Spec struct { Menu, Entity, Table, IDParam, IDQuery string; SelfAudited bool }`;
  - `func hrisdel.Guard(c Checker, q database.Querier, spec Spec, next http.Handler) http.Handler`;
  - `func hrisdel.CheckReason(reason string) error`, `func hrisdel.Reason(r *http.Request) string`.

- [ ] **Step 1: Tulis test yang gagal**

`backend/internal/platform/hrisdel/guard_integration_test.go`:

```go
package hrisdel_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/hrisdel"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/testutil"
)

func TestGuard(t *testing.T) {
	leaf := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{"hris.master.departments": {"read", "delete"}}})
	group := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{"hris.master": {"read", "create", "update", "delete"}}})
	sibling := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{"hris.master.positions": {"read", "delete"}}})
	noDelete := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{"hris.master.departments": {"read", "update"}}})
	deps := testutil.Deps(t, nil)
	tx := testutil.Tx(t)
	ctx := context.Background()
	var dept string
	if err := tx.QueryRow(ctx, `INSERT INTO hris.departments (name, code) VALUES ('Dept Hapus', $1) RETURNING id::text`,
		"GD"+testutil.RandomHex(3)).Scan(&dept); err != nil {
		t.Fatal(err)
	}
	var seenBody string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		seenBody = string(raw)
		if r.PathValue("id") == "missing" {
			_ = httpx.JSON(w, http.StatusNotFound, map[string]any{"success": false, "error": "Tidak ada"})
			return
		}
		if _, err := tx.Exec(r.Context(), `DELETE FROM hris.departments WHERE id::text = $1`, r.PathValue("id")); err != nil {
			t.Error(err)
		}
		_ = httpx.JSON(w, http.StatusOK, map[string]any{"message": "ok"})
	})
	mux := http.NewServeMux()
	mux.Handle("DELETE /t/{id}", hrisdel.Guard(deps.Auth, tx,
		hrisdel.Spec{Menu: "hris.master.departments", Entity: "hris.departments", Table: "hris.departments", IDParam: "id"}, next))
	mux.Handle("DELETE /self/{id}", hrisdel.Guard(deps.Auth, tx, hrisdel.Spec{Menu: "hris.master.departments", SelfAudited: true}, next))
	call := func(s *testutil.Staff, path string, body any) (int, string) {
		t.Helper()
		r := testutil.Request(http.MethodDelete, path, body)
		if s != nil {
			r = testutil.AsStaff(r, *s)
		}
		rec, _ := testutil.Do(t, mux, r)
		return rec.Code, rec.Body.String()
	}
	audits := func(id string) string {
		var n string
		if err := tx.QueryRow(ctx, `SELECT count(*)::text FROM audit.audit_log WHERE action = 'hris.departments.delete' AND entity_id = $1`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	if c, b := call(nil, "/t/"+dept+"?reason=Data+ganda", nil); c != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d %s", c, b)
	}
	for name, s := range map[string]*testutil.Staff{"group": &group, "sibling": &sibling, "no delete": &noDelete} {
		if c, b := call(s, "/t/"+dept+"?reason=Data+ganda", nil); c != http.StatusForbidden || b != `{"success":false,"error":"Insufficient permissions"}` {
			t.Errorf("%s = %d %s, want 403", name, c, b)
		}
	}
	reasonErr := `{"success":false,"error":"Alasan hapus wajib diisi (minimal 5 karakter)"}`
	for _, path := range []string{"/t/" + dept, "/t/" + dept + "?reason=abcd", "/t/" + dept + "?reason=%20%20ab%20%20"} {
		if c, b := call(&leaf, path, nil); c != http.StatusBadRequest || b != reasonErr {
			t.Fatalf("%s = %d %s, want 400 reason", path, c, b)
		}
	}
	if c, _ := call(&leaf, "/t/missing?reason=Data+ganda", nil); c != http.StatusNotFound || audits("missing") != "0" {
		t.Fatalf("a failed delete must not be audited (status %d)", c)
	}
	// Reason in the JSON body; the handler still reads the body.
	if c, b := call(&leaf, "/t/"+dept, map[string]any{"reason": "Departemen dobel"}); c != http.StatusOK {
		t.Fatalf("delete = %d %s", c, b)
	}
	if !strings.Contains(seenBody, "Departemen dobel") {
		t.Fatalf("handler body = %q, want it intact", seenBody)
	}
	var reason, name, actor string
	if err := tx.QueryRow(ctx, `SELECT reason, before->>'name', actor_id::text FROM audit.audit_log
		WHERE action = 'hris.departments.delete' AND entity_id = $1`, dept).Scan(&reason, &name, &actor); err != nil {
		t.Fatal(err)
	}
	if reason != "Departemen dobel" || name != "Dept Hapus" || actor != leaf.UserID {
		t.Fatalf("audit = %q %q %q", reason, name, actor)
	}
	// SelfAudited: only the permission; the handler owns reason and audit.
	if c, _ := call(&leaf, "/self/missing", nil); c != http.StatusNotFound {
		t.Fatalf("self-audited = %d, want the handler's 404", c)
	}
	if c, _ := call(&group, "/self/missing", nil); c != http.StatusForbidden {
		t.Fatalf("self-audited group = %d, want 403", c)
	}
}
```

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/platform/hrisdel -count=1 -v`
Expected: FAIL (build) — `no non-test Go files` / `undefined: hrisdel.Guard`.

- [ ] **Step 3: Tulis implementasi**

`backend/internal/platform/hrisdel/guard.go`:

```go
// Package hrisdel guards HRIS deletes (EPIC-061, Q-RA-1/4): the "delete"
// action on the route's own leaf menu (a group prefix or a sibling leaf
// does not count), a reason of at least five characters, and an
// audit.audit_log row with the deleted row as before.
package hrisdel

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
)

// MinReason is the shortest accepted reason, in characters.
const MinReason = 5

// ErrReason is the 400 for a missing or short reason. The frontend matches
// it (frontend/src/lib/hris/delete-reason.ts) to ask for one.
const ErrReason = "Alasan hapus wajib diisi (minimal 5 karakter)"

// Checker is the part of *auth.Service the guard needs.
type Checker interface {
	RequireMenuAction(r *http.Request, action string, prefixes ...string) (*auth.User, error)
}

// Spec describes one guarded route.
type Spec struct {
	Menu    string // leaf menu that must grant "delete"
	Entity  string // audit entity; the action is Entity + ".delete"
	Table   string // optional table snapshotted by id into before (a constant, never input)
	IDParam string // path value holding the row id ("id", "doc_id")
	IDQuery string // query key holding the id when the path has none
	// SelfAudited: the handler validates the reason and writes its own
	// audit (attendance); the guard checks the permission only.
	SelfAudited bool
}

func (s Spec) id(r *http.Request) string {
	if s.IDParam != "" {
		return r.PathValue(s.IDParam)
	}
	if s.IDQuery != "" {
		return r.URL.Query().Get(s.IDQuery)
	}
	return ""
}

// CheckReason enforces MinReason on the trimmed reason.
func CheckReason(reason string) error {
	if utf8.RuneCountInString(strings.TrimSpace(reason)) < MinReason {
		return httpx.BadRequest(ErrReason)
	}
	return nil
}

// Reason reads ?reason= or a JSON body {"reason": "..."}; the body is put
// back so the handler can still read it.
func Reason(r *http.Request) string {
	if v := strings.TrimSpace(r.URL.Query().Get("reason")); v != "" {
		return v
	}
	if r.Body == nil {
		return ""
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(raw))
	if err != nil || len(raw) == 0 {
		return ""
	}
	var body struct {
		Reason any `json:"reason"`
	}
	if json.Unmarshal(raw, &body) != nil {
		return ""
	}
	s, _ := body.Reason.(string)
	return strings.TrimSpace(s)
}

// Guard runs next only for a caller holding "delete" on spec.Menu and,
// unless SelfAudited, a valid reason; after a 2xx answer it writes the
// audit row on q (the module's database: the pool, or a test transaction).
func Guard(c Checker, q database.Querier, spec Spec, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := c.RequireMenuAction(r, "delete", spec.Menu)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		if spec.SelfAudited {
			next.ServeHTTP(w, r)
			return
		}
		reason := Reason(r)
		if err := CheckReason(reason); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		id := spec.id(r)
		before, err := snapshot(r.Context(), q, spec.Table, id)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		if sw.status < 200 || sw.status > 299 {
			return
		}
		e := audit.Entry{ActorID: u.ID, ActorName: &u.FullName, Action: spec.Entity + ".delete",
			Entity: spec.Entity, Reason: &reason}.WithRequest(r)
		if id != "" {
			e.EntityID = &id
		}
		if before != nil {
			e.Before = before
		} else {
			e.Before = target(r)
		}
		if err := audit.Write(r.Context(), q, e); err != nil {
			slog.ErrorContext(r.Context(), "hrisdel: audit write failed", "entity", spec.Entity, "id", id, "error", err)
		}
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// snapshot is the row about to be deleted, nil when there is no table, no
// id, or no such row (the handler answers the 404).
func snapshot(ctx context.Context, q database.Querier, table, id string) (json.RawMessage, error) {
	if table == "" || id == "" {
		return nil, nil
	}
	var raw []byte
	err := q.QueryRow(ctx, `SELECT to_jsonb(t) FROM `+table+` t WHERE t.id::text = $1`, id).Scan(&raw)
	if database.IsNoRows(err) {
		return nil, nil
	}
	return raw, err
}

// target is before for a delete without a row snapshot: the path and the
// query without the reason.
func target(r *http.Request) map[string]any {
	query := r.URL.Query()
	query.Del("reason")
	return map[string]any{"path": r.URL.Path, "query": query.Encode()}
}
```

- [ ] **Step 4: Jalankan test, pastikan lolos**

Run: perintah Step 2, lalu `cd backend && $HOME/.local/go/bin/go vet ./internal/platform/hrisdel`
Expected: `--- PASS: TestGuard`, `ok  nuhabit/backend/internal/platform/hrisdel`; vet tanpa output.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/platform/hrisdel
git commit -m "feat(platform): hrisdel.Guard — hapus HRIS butuh delete di menu daun, alasan, dan audit, EPIC-061

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: Modul HRIS — semua hapus dijaga daun + alasan + audit

**Files:**
- Create: `backend/internal/modules/hris/delete_specs.go`
- Create (test): `backend/internal/modules/hris/delete_guard_integration_test.go`
- Modify: `backend/internal/modules/hris/transport.go` (tambah `withDelete` setelah `withAction`)
- Modify: `backend/internal/modules/hris/routes.go:30,39,50,69,78,93,99,124,142,146,150` dan `employeeSubroutes` (map `documents`)
- Modify: `backend/internal/modules/hris/http_files.go:39`
- Modify: `backend/internal/modules/hris/attendance_adjust.go:423-426` (hapus cek peran)
- Modify: `backend/internal/modules/hris/leaves.go:367-372` (hapus cek peran)
- Modify (test yang sudah ada): `workforce_integration_test.go`, `employees_integration_test.go`, `people_integration_test.go`, `placements_integration_test.go`, `files_integration_test.go`, `attendance_periods_integration_test.go` (semua di `backend/internal/modules/hris/`)

**Interfaces:**
- Consumes: `hrisdel.Guard`, `hrisdel.Spec`, `hrisdel.ErrReason` (Task 2); `iam.Leaf*`, `iam.HrisDeleteLeaves` (Task 1).
- Produces:
  - `func (h handlers) withDelete(spec hrisdel.Spec, next http.Handler) http.Handler`;
  - `func (h *harness) grantDelete(s testutil.Staff, leaves ...string)` (helper test);
  - variabel spec `delEmployee`, `delDocument`, `delPlacement`, `delAttendance`, `delLeave`, `delShift`, `delHoliday`, `delContract`, `delSignedDoc`, `delAnnouncement`, `delDepartment`, `delStatus`, `delPosition`.

- [ ] **Step 1: Tulis test yang gagal**

`backend/internal/modules/hris/delete_guard_integration_test.go`:

```go
package hris

import (
	"net/http"
	"testing"

	"nuhabit/backend/internal/platform/hrisdel"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/testutil"
)

// grantDelete gives s "read" and "delete" on leaf menus its role lacks.
func (h *harness) grantDelete(s testutil.Staff, leaves ...string) {
	h.t.Helper()
	for _, leaf := range leaves {
		testutil.GrantMenu(h.t, s.RoleID, leaf, []string{"read", "delete"})
	}
}

// EPIC-061 Q-RA-1/4: every HRIS delete needs "delete" on its own leaf (a
// group grant with every action or a sibling leaf is refused) and a reason.
func TestHrisDeletesNeedLeafDeleteAndReason(t *testing.T) {
	h := newHarness(t)
	const id = "00000000-0000-4000-8000-0000000000ab"
	const attendanceReason = "Alasan penyesuaian wajib diisi (minimal 5 karakter)"
	routes := []struct{ path, leaf, reasonMsg string }{
		{"/api/hris/employees/" + id, iam.LeafKaryawan, hrisdel.ErrReason},
		{"/api/hris/employees/documents/" + id, iam.LeafKaryawan, hrisdel.ErrReason},
		{"/api/hris/employee-placements/" + id, iam.LeafPenempatan, hrisdel.ErrReason},
		{"/api/hris/attendance/" + id, iam.LeafAbsensi, attendanceReason},
		{"/api/hris/leaves/" + id, iam.LeafCuti, hrisdel.ErrReason},
		{"/api/hris/shifts/" + id, iam.LeafShift, hrisdel.ErrReason},
		{"/api/hris/holidays/" + id, iam.LeafHariLibur, hrisdel.ErrReason},
		{"/api/hris/contracts/" + id, iam.LeafKontrak, hrisdel.ErrReason},
		{"/api/hris/contracts/" + id + "/signed-document", iam.LeafKontrak, hrisdel.ErrReason},
		{"/api/hris/announcements/" + id, iam.LeafPengumuman, hrisdel.ErrReason},
		{"/api/master/departments/" + id, iam.LeafDepartemen, hrisdel.ErrReason},
		{"/api/master/employment-statuses/" + id, iam.LeafStatusKaryawan, hrisdel.ErrReason},
		{"/api/master/positions/" + id, iam.LeafJabatan, hrisdel.ErrReason},
	}
	// Group menus with every action: the old prefix guards let these through.
	group, _ := h.staff("hrd", []string{"hris.kepegawaian", "hris.master", "hris.workforce", "settings.users"}, false)
	for _, rt := range routes {
		var others []string
		for _, l := range iam.HrisDeleteLeaves {
			if l != rt.leaf {
				others = append(others, l)
			}
		}
		sibling, _ := h.staff("hrd", others, false)
		own, _ := h.staff("hrd", []string{rt.leaf}, false)
		expect(t, h.do(&group, http.MethodDelete, rt.path+"?reason=Data%20uji%20hapus", nil), http.StatusForbidden, "Insufficient permissions")
		expect(t, h.do(&sibling, http.MethodDelete, rt.path+"?reason=Data%20uji%20hapus", nil), http.StatusForbidden, "Insufficient permissions")
		expect(t, h.do(&own, http.MethodDelete, rt.path, nil), http.StatusBadRequest, rt.reasonMsg)
		expect(t, h.do(&own, http.MethodDelete, rt.path+"?reason=abcd", nil), http.StatusBadRequest, rt.reasonMsg)
		if c := h.do(&own, http.MethodDelete, rt.path+"?reason=Data%20uji%20hapus", nil); c.status == http.StatusForbidden || c.body["error"] == rt.reasonMsg {
			t.Errorf("%s with leaf delete + reason = %d %s", rt.path, c.status, c.raw)
		}
	}
}
```

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris -run TestHrisDeletesNeedLeafDeleteAndReason -count=1 -v`
Expected: FAIL. Contoh pesan: `status 404, want 403` atau `status 200, want 403` untuk staf `group` (guard prefiks lama meloloskannya).

- [ ] **Step 3: Tambah spec dan helper**

`backend/internal/modules/hris/delete_specs.go`:

```go
package hris

import (
	"nuhabit/backend/internal/platform/hrisdel"
	"nuhabit/backend/internal/platform/iam"
)

// HRIS deletes (EPIC-061, Q-RA-1): each needs "delete" on its own leaf
// menu and a reason, and leaves an audit.audit_log row with the row.
var (
	delEmployee  = hrisdel.Spec{Menu: iam.LeafKaryawan, Entity: "hris.employees", Table: "hris.employees", IDParam: "id"}
	delDocument  = hrisdel.Spec{Menu: iam.LeafKaryawan, Entity: "hris.employee_documents", Table: "hris.employee_documents", IDParam: "doc_id"}
	delPlacement = hrisdel.Spec{Menu: iam.LeafPenempatan, Entity: "hris.employee_placements", Table: "hris.employee_placements", IDParam: "id"}
	// Attendance validates its reason and writes its adjustment log and
	// audit row itself (DeleteAttendanceWithReason).
	delAttendance   = hrisdel.Spec{Menu: iam.LeafAbsensi, SelfAudited: true}
	delLeave        = hrisdel.Spec{Menu: iam.LeafCuti, Entity: "hris.leaves", Table: "hris.leaves", IDParam: "id"}
	delShift        = hrisdel.Spec{Menu: iam.LeafShift, Entity: "hris.shifts", Table: "hris.shifts", IDParam: "id"}
	delHoliday      = hrisdel.Spec{Menu: iam.LeafHariLibur, Entity: "hris.public_holidays", Table: "hris.public_holidays", IDParam: "id"}
	delContract     = hrisdel.Spec{Menu: iam.LeafKontrak, Entity: "hris.employment_contracts", Table: "hris.employment_contracts", IDParam: "id"}
	delSignedDoc    = hrisdel.Spec{Menu: iam.LeafKontrak, Entity: "hris.contract_signed_document", Table: "hris.employment_contracts", IDParam: "id"}
	delAnnouncement = hrisdel.Spec{Menu: iam.LeafPengumuman, Entity: "hris.announcements", Table: "hris.announcements", IDParam: "id"}
	delDepartment   = hrisdel.Spec{Menu: iam.LeafDepartemen, Entity: "hris.departments", Table: "hris.departments", IDParam: "id"}
	delStatus       = hrisdel.Spec{Menu: iam.LeafStatusKaryawan, Entity: "hris.employment_statuses", Table: "hris.employment_statuses", IDParam: "id"}
	delPosition     = hrisdel.Spec{Menu: iam.LeafJabatan, Entity: "hris.positions", Table: "hris.positions", IDParam: "id"}
)
```

Di `backend/internal/modules/hris/transport.go`, setelah fungsi `withAction`, tambahkan (dan import `"nuhabit/backend/internal/platform/hrisdel"`):

```go
// withDelete is hrisdel.Guard on this module's database (EPIC-061): the
// "delete" action on the route's leaf menu, a reason, an audit row.
func (h handlers) withDelete(spec hrisdel.Spec, next http.Handler) http.Handler {
	return hrisdel.Guard(h.auth, h.svc.repo.querier(), spec, next)
}
```

- [ ] **Step 4: Bungkus route**

Di `backend/internal/modules/hris/routes.go` ganti baris berikut (handler di dalamnya tetap):

```go
		r("DELETE /api/hris/employees/{id}", h.withDelete(delEmployee, h.withMenu(employeeRecordManagers, h.deleteEmployee))),
```
```go
		r("DELETE /api/hris/employee-placements/{id}", h.withDelete(delPlacement, h.withMenu(placementManagers, h.deletePlacement))),
```
```go
		r("DELETE /api/hris/attendance/{id}", h.withDelete(delAttendance, h.withActor(h.deleteAttendance))),
```
```go
		r("DELETE /api/hris/leaves/{id}", h.withDelete(delLeave, h.withActor(h.deleteLeave))),
```
```go
		r("DELETE /api/hris/shifts/{id}", h.withDelete(delShift, h.withMenu(kepegawaian, h.deleteShift))),
```
```go
		r("DELETE /api/hris/holidays/{id}", h.withDelete(delHoliday, h.withMenu(kepegawaian, h.deleteHoliday))),
```
```go
		r("DELETE /api/hris/contracts/{id}", h.withDelete(delContract, h.withMenu(kepegawaian, h.deleteContract))),
```
```go
		r("DELETE /api/hris/announcements/{id}", h.withDelete(delAnnouncement, h.withMenu(kepegawaian, h.deleteAnnouncement))),
```
```go
		r("DELETE /api/master/departments/{id}", h.withDelete(delDepartment, h.masterDelete(departmentsKind))),
```
```go
		r("DELETE /api/master/employment-statuses/{id}", h.withDelete(delStatus, h.masterDelete(statusesKind))),
```
```go
		r("DELETE /api/master/positions/{id}", h.withDelete(delPosition, h.masterDelete(positionsKind))),
```

Di `employeeSubroutes` (file yang sama) map `documents` menjadi:

```go
	documents := map[string]http.Handler{
		http.MethodPatch:  h.withMenu(kepegawaian, h.patchEmployeeDocument),
		http.MethodDelete: h.withDelete(delDocument, h.withMenu(kepegawaian, h.deleteEmployeeDocument)),
	}
```

Di `backend/internal/modules/hris/http_files.go:39`:

```go
		r("DELETE /api/hris/contracts/{id}/signed-document", h.withDelete(delSignedDoc, h.withMenu(kepegawaian, h.deleteSignedDocument))),
```

- [ ] **Step 5: Ganti cek peran `hrd` di hapus absensi & cuti**

`backend/internal/modules/hris/attendance_adjust.go` — awal `DeleteAttendanceWithReason` menjadi (cek `canManageAttendance` dihapus; route guard memeriksa `delete` di Absensi):

```go
func (s *Service) DeleteAttendanceWithReason(ctx context.Context, a *Actor, id, reason string, meta audit.Entry) error {
	if err := requireReason(reason); err != nil {
		return err
	}
	current, err := s.repo.AttendanceRecord(ctx, id)
```

`backend/internal/modules/hris/leaves.go` — `DeleteLeave` menjadi:

```go
// DeleteLeave removes a leave request; the route guard requires "delete"
// on Cuti & Izin, a reason, and writes the audit row (EPIC-061).
func (s *Service) DeleteLeave(ctx context.Context, _ *Actor, id string) error {
	return s.repo.DeleteLeave(ctx, id)
}
```

- [ ] **Step 6: Sesuaikan test lama (hapus kini butuh daun + alasan)**

Edit persis berikut (path relatif `backend/internal/modules/hris/`):

`workforce_integration_test.go`
- `TestAttendance`, setelah `staff, own := h.staff("pos", nil, true, emp{name: "Absen Test"})` tambahkan `h.grantDelete(hr, "hris.workforce.attendance")`.
- Baris `expect(t, h.do(&staff, "DELETE", "/api/hris/attendance/"+att, nil), 403, "Forbidden: Only HRD can delete attendance records")` → `expect(t, h.do(&staff, "DELETE", "/api/hris/attendance/"+att, nil), 403, "Insufficient permissions")`.
- `TestLeaves`, setelah `staff, own := h.staff("pos", nil, true, emp{name: "Pemohon Cuti"})` tambahkan `h.grantDelete(hr, "hris.workforce.leaves")`.
- `expect(t, h.do(&staff, "DELETE", "/api/hris/leaves/"+leaveID, nil), 403, "Forbidden: Only HRD can delete leave requests")` → `expect(t, h.do(&staff, "DELETE", "/api/hris/leaves/"+leaveID, nil), 403, "Insufficient permissions")`.
- `expect(t, h.do(&hr, "DELETE", "/api/hris/leaves/"+leaveID, nil), 200, "")` → `expect(t, h.do(&hr, "DELETE", "/api/hris/leaves/"+leaveID+"?reason=Pengajuan%20ganda", nil), 200, "")`.
- `TestOvertimeAndShifts`, setelah `staff, own := h.staff("pos", nil, true)` (baris 162) tambahkan `h.grantDelete(hr, "hris.kepegawaian.shifts")`.
- Ganti blok hapus shift menjadi:

```go
	del := h.do(&hr, "DELETE", "/api/hris/shifts/"+sid+"?reason=Shift%20uji%20dihapus", nil)
	expect(t, del, 200, "")
	if del.body["message"] != "Shift dihapus" {
		t.Fatalf("delete = %s", del.raw)
	}
	if got := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'hris.shifts.delete' AND entity_id = $1 AND reason = 'Shift uji dihapus'`, sid); got != "1" {
		t.Fatalf("shift delete audit rows = %s", got)
	}
	expect(t, h.do(&hr, "DELETE", "/api/hris/shifts/"+sid+"?reason=Shift%20uji%20dihapus", nil), 404, "Shift tidak ditemukan")
```

- `TestHolidays`, setelah `staff, _ := h.staff("pos", nil, false)` tambahkan `h.grantDelete(hr, "hris.kepegawaian.holidays")`. Kedua baris `DELETE /api/hris/holidays/"+id` diberi `+"?reason=Libur%20salah%20input"` setelah `id`.

`employees_integration_test.go`
- `TestEmployeeDirectoryAndCRUD`: setelah `hr, _ := h.staff("hrd", []string{menuKepegawaian}, false)` tambahkan `h.grantDelete(hr, "hris.kepegawaian.users")`; `del := h.do(&hr, "DELETE", "/api/hris/employees/"+id, nil)` → `del := h.do(&hr, "DELETE", "/api/hris/employees/"+id+"?reason=Karyawan%20uji%20dihapus", nil)`.
- `TestEmployeeDocumentsAndHistory`: setelah `self, own := h.staff("pos", nil, true)` tambahkan `h.grantDelete(hr, "hris.kepegawaian.users")`; `expect(t, h.do(&hr, "DELETE", "/api/hris/employees/documents/"+docID, nil), 200, "")` → `expect(t, h.do(&hr, "DELETE", "/api/hris/employees/documents/"+docID+"?reason=Dokumen%20salah%20unggah", nil), 200, "")`. Baris `self` (403) tetap.
- `TestEmployeeContracts`: setelah `hr, _ := h.staff("hrd", []string{menuKepegawaian, "hris.compensation"}, false)` tambahkan `h.grantDelete(hr, "hris.kepegawaian.contracts")`; `"/api/hris/contracts/"+id, nil), 409` → `"/api/hris/contracts/"+id+"?reason=Draft%20salah%20input", nil), 409`.

`people_integration_test.go`
- `TestSelfService`: setelah `hr, _ := h.staff("hrd", []string{menuKepegawaian}, true)` tambahkan `h.grantDelete(hr, "hris.kepegawaian.announcements")`; `"/api/hris/announcements/"+id, nil), 200` → `"/api/hris/announcements/"+id+"?reason=Pengumuman%20kedaluwarsa", nil), 200`.
- `TestLogbookReportsMaster`: setelah `staff, _ := h.staff("pos", nil, true, emp{dept: &dept})` tambahkan `h.grantDelete(hr, "hris.master.departments", "hris.master.employment-statuses")`; `"/api/master/departments/"+dept, nil), 400` → `"/api/master/departments/"+dept+"?reason=Departemen%20dobel", nil), 400`; `"/api/master/employment-statuses/"+st.data()["id"].(string), nil), 200` → `"/api/master/employment-statuses/"+st.data()["id"].(string)+"?reason=Status%20tidak%20dipakai", nil), 200`. Baris logbook tidak diubah (R2).

`placements_integration_test.go` (`hr` sudah memegang daun Penempatan dengan semua aksi): kedua baris `"/api/hris/employee-placements/"+kopoID, nil)` → `"/api/hris/employee-placements/"+kopoID+"?reason=Penempatan%20berakhir", nil)`.

`files_integration_test.go`
- `TestContractDocuments`: setelah `_, worker := h.staff("pos", nil, true, emp{name: "Budi Santoso"})` tambahkan `h.grantDelete(hr, "hris.kepegawaian.contracts")`.
- Tiga baris DELETE milik `hr` diberi alasan:
  - `base+"/signed-document", nil), 200` → `base+"/signed-document?reason=Scan%20salah%20unggah", nil), 200`;
  - `base+"/signed-document", nil), 409` → `base+"/signed-document?reason=Scan%20salah%20unggah", nil), 409`;
  - `"/api/hris/contracts/11111111-1111-4111-8111-111111111111/signed-document", nil), 404` → `"/api/hris/contracts/11111111-1111-4111-8111-111111111111/signed-document?reason=Scan%20salah%20unggah", nil), 404`.
- Baris `staff` (403) tetap.

`attendance_periods_integration_test.go`
- `TestAttendancePeriodClosing`: setelah `crew, crewID := h.staff("pos", nil, true, emp{name: "Crew Periode"})` tambahkan `h.grantDelete(hr, "hris.workforce.attendance")`.

- [ ] **Step 7: Jalankan test paket, pastikan lolos**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris/... -count=1 2>&1 | tail -30`
Expected:
- `TestHrisDeletesNeedLeafDeleteAndReason` PASS.
- Kegagalan paket hris hanya yang ada di `/tmp/epic061-baseline.txt` (mis. `TestAttendancePeriodClosing`). Tidak ada kegagalan baru.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/modules/hris
git commit -m "feat(hris): hapus karyawan, dokumen, absensi, cuti, shift, libur, kontrak, pengumuman, master butuh delete di menu daun + alasan + audit, EPIC-061

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: Payroll, Kinerja, dan Rekrutmen — hapus dijaga daun + test inventaris

**Files:**
- Create: `backend/internal/modules/payroll/delete_specs.go`
- Modify: `backend/internal/modules/payroll/module.go:83,87,102,110,138,145,150`
- Create: `backend/internal/modules/recruitment/delete_specs.go`
- Modify: `backend/internal/modules/recruitment/http.go:15-17` (interface `Guard`), `:52`, `:89`
- Modify: `backend/internal/modules/recruitment/files.go:45,47`
- Create (test): `backend/internal/app/hris_delete_inventory_test.go`
- Create (test): `backend/internal/modules/payroll/delete_guard_integration_test.go`
- Create (test): `backend/internal/modules/recruitment/delete_guard_integration_test.go`
- Modify (test lama): `backend/internal/modules/payroll/payroll_integration_test.go`, `payroll/hris_integration_test.go`, `recruitment/files_integration_test.go`, `recruitment/recruitment_integration_test.go`

**Interfaces:**
- Consumes: `hrisdel.Guard`, `hrisdel.Spec`, `hrisdel.ErrReason` (Task 2); `iam.Leaf*` (Task 1); `app.Routes()`, `app.Handler(deps, started)`.
- Produces:
  - `func (h *handler) del(pattern string, spec hrisdel.Spec, fn httpx.HandlerFunc) module.Route` (payroll);
  - `func (h *handler) del(spec hrisdel.Spec, next http.Handler) http.Handler` (recruitment);
  - interface `recruitment.Guard` bertambah `RequireMenuAction`.

- [ ] **Step 1: Tulis test inventaris yang gagal**

`backend/internal/app/hris_delete_inventory_test.go`:

```go
package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nuhabit/backend/internal/platform/testutil"
)

// EPIC-061 Review Focus 5: every DELETE route of the hris, payroll and
// recruitment modules refuses a staff member who holds the HRIS groups
// with every action but no leaf "delete" — a new delete route that is not
// wrapped in hrisdel.Guard fails here. The logbook stays ESS (ruling R2).
func TestEveryHrisDeleteNeedsLeafDelete(t *testing.T) {
	all := []string{"read", "create", "update", "delete", "approve", "export", "import", "execute"}
	menus := map[string][]string{}
	for _, g := range []string{"hris", "hris.kepegawaian", "hris.master", "hris.workforce", "hris.compensation",
		"hris.performance", "hris.recruitment", "settings.users"} {
		menus[g] = all
	}
	staff := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: menus})
	deps := testutil.Deps(t, nil)
	deps.Config.Modules = "hris,payroll,recruitment"
	h, _ := Handler(deps, time.Now())
	const id = "00000000-0000-4000-8000-0000000000ac"
	modules := map[string]bool{"hris": true, "payroll": true, "recruitment": true}
	paths := []string{"/api/hris/employees/documents/" + id}
	for _, rt := range Routes() {
		if rt.Method != http.MethodDelete || !modules[rt.Module] || rt.Path == "/api/hris/logbook" {
			continue
		}
		paths = append(paths, strings.NewReplacer("{id}", id, "{doc_id}", id).Replace(rt.Path))
	}
	if len(paths) < 20 {
		t.Fatalf("only %d delete routes found: %v", len(paths), paths)
	}
	for _, p := range paths {
		r := testutil.AsStaff(httptest.NewRequest(http.MethodDelete, p+"?reason=Uji%20inventaris&id="+id, nil), staff)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != http.StatusForbidden {
			t.Errorf("DELETE %s = %d %s, want 403 (a group grant must not delete)", p, rec.Code, rec.Body.String())
		}
	}
}
```

- [ ] **Step 2: Jalankan, pastikan gagal**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/app -run TestEveryHrisDeleteNeedsLeafDelete -count=1 -v`
Expected: FAIL, hanya untuk route payroll/Kinerja/rekrutmen. Contoh: `DELETE /api/hris/payroll/00000000-... = 404 ...`, `DELETE /api/candidates/... = 404 ...`, `DELETE /api/hris/feedback-cycles/... = 200 ...`. Route modul hris (Task 3) sudah 403.

- [ ] **Step 3: Payroll + Kinerja**

`backend/internal/modules/payroll/delete_specs.go`:

```go
package payroll

import (
	"nuhabit/backend/internal/platform/hrisdel"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/module"
)

// Payroll and Kinerja deletes (EPIC-061, Q-RA-1): "delete" on the leaf,
// a reason, an audit row. Kinerja deletes are KPI admin work, so they
// check the KPI Scorecard leaf (ruling R1).
var (
	delRun        = hrisdel.Spec{Menu: iam.LeafRunPenggajian, Entity: "hris.payroll_runs", Table: "hris.payroll_runs", IDParam: "id"}
	delInput      = hrisdel.Spec{Menu: iam.LeafRunPenggajian, Entity: "hris.payroll_inputs", IDParam: "id"}
	delSalary     = hrisdel.Spec{Menu: iam.LeafSalary, Entity: "hris.employee_salary", Table: "hris.employee_salary", IDParam: "id"}
	delKPITarget  = hrisdel.Spec{Menu: iam.LeafKPIScorecard, Entity: "performance.kpi_targets", Table: "performance.kpi_targets", IDQuery: "id"}
	delAssignment = hrisdel.Spec{Menu: iam.LeafKPIScorecard, Entity: "performance.feedback_assignments", Table: "performance.feedback_assignments", IDParam: "id"}
	delCycle      = hrisdel.Spec{Menu: iam.LeafKPIScorecard, Entity: "performance.feedback_cycles", Table: "performance.feedback_cycles", IDParam: "id"}
	delResponse   = hrisdel.Spec{Menu: iam.LeafKPIScorecard, Entity: "performance.feedback_responses", Table: "performance.feedback_responses", IDParam: "id"}
)

// del is a route behind hrisdel.Guard on this module's database.
func (h *handler) del(pattern string, spec hrisdel.Spec, fn httpx.HandlerFunc) module.Route {
	return module.Route{Pattern: pattern, Handler: hrisdel.Guard(h.auth, h.svc.db, spec, httpx.Handle(fn))}
}
```

Di `backend/internal/modules/payroll/module.go`, ganti tujuh baris route DELETE:

```go
		h.del("DELETE /api/hris/payroll/{id}", delRun, h.deleteRun),
```
```go
		h.del("DELETE /api/hris/payroll/{id}/inputs", delInput, h.deleteInput),
```
```go
		h.del("DELETE /api/hris/employee-salary/{id}", delSalary, h.deleteSalary),
```
```go
		h.del("DELETE /api/hris/kpi/targets", delKPITarget, h.deleteTarget),
```
```go
		h.del("DELETE /api/hris/feedback-assignments/{id}", delAssignment, h.deleteAssignment),
```
```go
		h.del("DELETE /api/hris/feedback-cycles/{id}", delCycle, h.deleteFeedbackCycle),
```
```go
		h.del("DELETE /api/hris/feedback-responses/{id}", delResponse, h.deleteResponse),
```

- [ ] **Step 4: Rekrutmen**

`backend/internal/modules/recruitment/http.go` — interface `Guard` menjadi:

```go
type Guard interface {
	RequireMenuPrefix(r *http.Request, prefixes ...string) (*auth.User, error)
	RequireMenuAction(r *http.Request, action string, prefixes ...string) (*auth.User, error)
}
```

`backend/internal/modules/recruitment/delete_specs.go`:

```go
package recruitment

import (
	"net/http"

	"nuhabit/backend/internal/platform/hrisdel"
	"nuhabit/backend/internal/platform/iam"
)

// Recruitment deletes (EPIC-061, Q-RA-1): "delete" on the leaf, a reason,
// an audit row.
var (
	delCandidate  = hrisdel.Spec{Menu: iam.LeafKandidat, Entity: "recruitment.candidates", Table: "recruitment.candidates", IDParam: "id"}
	delCV         = hrisdel.Spec{Menu: iam.LeafKandidat, Entity: "recruitment.candidate_cv", Table: "recruitment.candidates", IDParam: "id"}
	delQuestion   = hrisdel.Spec{Menu: iam.LeafPsikotes, Entity: "recruitment.psikotes_questions", Table: "recruitment.psikotes_questions", IDParam: "id"}
	delJobOpening = hrisdel.Spec{Menu: iam.LeafJobPortal, Entity: "recruitment.job_openings", Table: "recruitment.job_openings", IDParam: "id"}
)

// del wraps next in hrisdel.Guard on this module's database.
func (h *handler) del(spec hrisdel.Spec, next http.Handler) http.Handler {
	return hrisdel.Guard(h.guard, h.svc.db, spec, next)
}
```

Ganti route:

`recruitment/http.go`:
```go
		{Pattern: "DELETE /api/psikotes/questions/{id}", Handler: h.del(delQuestion, h.staff(rec, h.deleteQuestion))},
```
```go
		{Pattern: "DELETE /api/hris/job-openings/{id}", Handler: h.del(delJobOpening, h.staff(rec, h.deleteJobOpening))},
```

`recruitment/files.go`:
```go
		{Pattern: "DELETE /api/candidates/{id}", Handler: f.del(delCandidate, f.staff(rec, f.deleteCandidate))},
```
```go
		{Pattern: "DELETE /api/candidates/{id}/cv-upload", Handler: f.del(delCV, f.staff(rec, f.removeCV))},
```

- [ ] **Step 5: Test guard per modul**

`backend/internal/modules/payroll/delete_guard_integration_test.go`:

```go
package payroll

import (
	"testing"

	"nuhabit/backend/internal/platform/hrisdel"
)

func TestPayrollKinerjaDeletesNeedLeafDelete(t *testing.T) {
	e := setup(t)
	group := hrStaff(e, map[string][]string{
		"hris.compensation": {"read", "create", "update", "delete"}, "hris.performance": {"read", "create", "update", "delete"},
		"hris.performance.kpi-config": {"read", "update"},
	})
	for _, p := range []string{
		"/api/hris/payroll/x?reason=Data%20uji",
		"/api/hris/payroll/x/inputs?employee_id=x&code=x&reason=Data%20uji",
		"/api/hris/employee-salary/x?reason=Data%20uji",
		"/api/hris/kpi/targets?id=x&reason=Data%20uji",
		"/api/hris/feedback-cycles/x?reason=Data%20uji",
		"/api/hris/feedback-assignments/x?reason=Data%20uji",
		"/api/hris/feedback-responses/x?reason=Data%20uji",
	} {
		wantErr(t, e.call(&group, "DELETE", p, nil, 403), "Insufficient permissions")
	}
	wantErr(t, e.call(&e.hr, "DELETE", "/api/hris/payroll/x", nil, 400), hrisdel.ErrReason)
	wantErr(t, e.call(&e.hr, "DELETE", "/api/hris/kpi/targets?id=x&reason=abcd", nil, 400), hrisdel.ErrReason)
}
```

`backend/internal/modules/recruitment/delete_guard_integration_test.go`:

```go
package recruitment

import (
	"context"
	"testing"

	"nuhabit/backend/internal/platform/hrisdel"
	"nuhabit/backend/internal/platform/testutil"
)

func TestRecruitmentDeletesNeedLeafDelete(t *testing.T) {
	h := newHarness(t)
	group := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{
		"hris.recruitment": {"read", "create", "update", "delete"}}})
	t.Cleanup(func() { _ = h.tx.Rollback(context.Background()) })
	const id = "00000000-0000-4000-8000-0000000000ad"
	for _, p := range []string{"/api/candidates/" + id, "/api/candidates/" + id + "/cv-upload",
		"/api/psikotes/questions/" + id, "/api/hris/job-openings/" + id} {
		expect(t, h.as(group, "DELETE", p+"?reason=Data%20uji", nil), 403, "Insufficient permissions")
	}
	testutil.GrantMenu(t, h.hr.RoleID, "hris.recruitment.candidates", []string{"read", "delete"})
	expect(t, h.as(h.hr, "DELETE", "/api/candidates/"+id, nil), 400, hrisdel.ErrReason)
}
```

- [ ] **Step 6: Sesuaikan test lama**

`payroll/payroll_integration_test.go` — di `setup`, menu `hr` menjadi:

```go
	hr := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{
		"hris.compensation": nil, "hris.performance": nil, "hris.performance.kpi-config": nil,
		"hris.kepegawaian.loans": {"read", "create", "update", "approve"},
		// EPIC-061: deletes need "delete" on the leaf.
		"hris.compensation.payroll": {"read", "delete"}, "hris.compensation.salary": {"read", "delete"},
		"hris.performance.kpi-scorecard": {"read", "create", "update", "approve", "delete"},
	}})
```

dan beri alasan pada path DELETE `e.hr`:
- `TestRunLifecycle`: `"/api/hris/payroll/"+runID, nil, 400)` → `"/api/hris/payroll/"+runID+"?reason=Run%20uji%20dihapus", nil, 400)`.
- `TestRunDeleteAndPaidMismatch`:
  - `"/api/hris/payroll/nope", nil, 404)` → `"/api/hris/payroll/nope?reason=Run%20uji%20dihapus", nil, 404)`;
  - `"/api/hris/payroll/"+runID, nil, 200)` → `"/api/hris/payroll/"+runID+"?reason=Run%20uji%20dihapus", nil, 200)`.
- `TestEmployeeSalary`: `"/api/hris/employee-salary/"+second["id"].(string), nil, 200)` → `"/api/hris/employee-salary/"+second["id"].(string)+"?reason=Salary%20uji%20dihapus", nil, 200)`.

`payroll/hris_integration_test.go`:
- `TestKPITargetsAndScorecards`:
  - `"/api/hris/kpi/targets", nil, 400), "id wajib")` → `"/api/hris/kpi/targets?reason=Target%20uji%20dihapus", nil, 400), "id wajib")`;
  - `"/api/hris/kpi/targets?id="+created["data"].(map[string]any)["id"].(string), nil, 200)` → `"/api/hris/kpi/targets?reason=Target%20uji%20dihapus&id="+created["data"].(map[string]any)["id"].(string), nil, 200)`.
- `TestFeedback360`: tiga path DELETE (`/feedback-responses/`, `/feedback-assignments/`, `/feedback-cycles/`) diberi `+"?reason=Feedback%20uji%20dihapus"` setelah id-nya.

`recruitment/files_integration_test.go`:
- `TestCVUploadAndRemove`: sebelum baris `c = h.as(h.hr, "DELETE", "/api/candidates/"+id+"/cv-upload", nil)` tambahkan `testutil.GrantMenu(t, h.hr.RoleID, "hris.recruitment.candidates", []string{"read", "delete"})`; path menjadi `"/api/candidates/"+id+"/cv-upload?reason=CV%20salah%20unggah"`.
- `TestDeleteCandidatePurgesPsikotesFiles`:
  - sebelum baris `h.none` tambahkan `testutil.GrantMenu(t, h.hr.RoleID, "hris.recruitment.candidates", []string{"read", "delete"})`;
  - tiga path DELETE milik `h.hr` diberi `+"?reason=Kandidat%20uji%20dihapus"` (`"/api/candidates/x"` menjadi `"/api/candidates/x?reason=Kandidat%20uji%20dihapus"`).

`recruitment/recruitment_integration_test.go`:
- `TestPsikotesBank`: sebelum DELETE pertama tambahkan `testutil.GrantMenu(t, h.hr.RoleID, "hris.recruitment.psikotes", []string{"read", "delete"})`; kedua path `"/api/psikotes/questions/"+qid` diberi `+"?reason=Soal%20dobel"`.
- `TestJobOpeningsAndPositions`: sebelum DELETE tambahkan `testutil.GrantMenu(t, h.hr.RoleID, "hris.recruitment.job-portal", []string{"read", "delete"})`; path `"/api/hris/job-openings/"+id` diberi `+"?reason=Lowongan%20ditutup"`.

- [ ] **Step 7: Jalankan semua, pastikan lolos**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/app ./internal/modules/payroll/... ./internal/modules/recruitment/... -count=1 2>&1 | tail -30`
Expected:
- `TestEveryHrisDeleteNeedsLeafDelete`, `TestPayrollKinerjaDeletesNeedLeafDelete`, `TestRecruitmentDeletesNeedLeafDelete` PASS.
- Tidak ada kegagalan di luar `/tmp/epic061-baseline.txt`.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/modules/payroll backend/internal/modules/recruitment backend/internal/app/hris_delete_inventory_test.go
git commit -m "feat(payroll,recruitment): hapus run, input, salary, KPI, feedback, kandidat, CV, soal, lowongan butuh delete di menu daun + alasan + audit, EPIC-061

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: Impor jadwal yang menimpa & ubah tipe jadwal yang menghapus hari

**Files:**
- Create: `backend/internal/modules/hris/schedule_guard.go`
- Modify: `backend/internal/modules/hris/schedule_type.go` (`applyFixedSchedule`, `ChangeScheduleType`, `ApplyFixedSchedule`, `changeScheduleTypeTx`, `readScheduleTypeBody`, `putScheduleType`, `bulkScheduleType`, `applyFixedSchedule` handler)
- Modify: `backend/internal/modules/hris/schedule_days.go` (`scheduleImportInput`, `ImportSchedule`, `importSchedules`)
- Modify: `backend/internal/modules/hris/employee_excel.go` (`employeeImportInput`, `ImportEmployees`, `applyImportRow`, `importEmployees`)
- Create (test): `backend/internal/modules/hris/schedule_delete_guard_integration_test.go`
- Modify (test): `backend/internal/modules/hris/schedule_type_integration_test.go:27-28`

**Interfaces:**
- Consumes: `hrisdel.CheckReason`, `hrisdel.ErrReason` (Task 2); `iam.LeafJadwalKerja` (Task 1); `(*auth.Service).HasMenuAction(ctx, u, action, prefixes...)`.
- Produces:
  - `type clearGuard struct { Allowed bool; Reason string; Meta audit.Entry }`, dengan method `check(n int) error` dan `audit(ctx, q, actor *auth.User, action, entityID string, before map[string]any, n int) error`;
  - `func (h handlers) clearGuardFor(r *http.Request, u *auth.User, reason string) (clearGuard, error)`;
  - `func replacedScheduleDays(ctx context.Context, q database.Querier, as []domain.ScheduleAssignment) (int, error)`;
  - field JSON pratinjau baru `replaced_days`;
  - field request `reason` (form impor jadwal, body ubah tipe jadwal / jadwal tetap);
  - signature baru:
    - `applyFixedSchedule(ctx, r Repository, employeeID, from string, actor *auth.User, g clearGuard) (int, error)`
    - `changeScheduleTypeTx(ctx, r, actor, ids, to, from string, dryRun bool, g clearGuard)`
    - `(*Service).ChangeScheduleType(ctx, actor, ids, to, from string, dryRun bool, g clearGuard)`
    - `(*Service).ApplyFixedSchedule(ctx, actor, branchID string, ids []string, from string, g clearGuard)`
    - `(*Service).applyImportRow(..., today string, g clearGuard)`.

- [ ] **Step 1: Tulis test yang gagal**

`backend/internal/modules/hris/schedule_delete_guard_integration_test.go`:

```go
package hris

import (
	"bytes"
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/xuri/excelize/v2"

	"nuhabit/backend/internal/platform/hrisdel"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/testutil"
)

const scheduleNeedsDelete = "Menimpa atau menghapus jadwal yang sudah ada butuh hak hapus Jadwal Kerja"

// scheduleWriter edits Jadwal Kerja and employees without "delete".
var scheduleWriter = map[string][]string{
	"hris.kepegawaian":  {"read", "update"},
	iam.LeafJadwalKerja: {"read", "create", "update", "import", "export"},
}

// staffActions is h.staff with explicit actions per menu.
func (h *harness) staffActions(role string, grants map[string][]string) testutil.Staff {
	h.t.Helper()
	s := testutil.CreateStaff(h.t, testutil.StaffOptions{Role: role, Menus: grants})
	h.t.Cleanup(func() { _ = h.tx.Rollback(context.Background()) })
	return s
}

// anaWeek is the Jadwal Kerja file for one employee over 05–11/10.
func anaWeek(branch, nip string, cells []string) []byte {
	f := excelize.NewFile()
	_ = f.SetSheetName("Sheet1", "Jadwal")
	head := []string{"NIP", "Nama Karyawan", "Jabatan", "Sen 05/10", "Sel 06/10", "Rab 07/10", "Kam 08/10", "Jum 09/10", "Sab 10/10", "Min 11/10"}
	row := append([]string{nip, "Ana Shift", ""}, cells...)
	for i := range head {
		c1, _ := excelize.CoordinatesToCellName(i+1, 3)
		c2, _ := excelize.CoordinatesToCellName(i+1, 4)
		_ = f.SetCellStr("Jadwal", c1, head[i])
		_ = f.SetCellStr("Jadwal", c2, row[i])
	}
	_, _ = f.NewSheet("Info")
	for i, kv := range [][2]string{{"Cabang ID", branch}, {"Dari", "2026-10-05"}, {"Sampai", "2026-10-11"}} {
		_ = f.SetCellStr("Info", "A"+strconv.Itoa(i+1), kv[0])
		_ = f.SetCellStr("Info", "B"+strconv.Itoa(i+1), kv[1])
	}
	var buf bytes.Buffer
	_ = f.Write(&buf)
	return buf.Bytes()
}

func TestScheduleUploadReplacingDaysNeedsDelete(t *testing.T) {
	h := newHarness(t)
	writer := h.staffActions("hrd", scheduleWriter)
	deleter, _ := h.staff("hrd", scheduleTestMenus, false)
	branch, ana, _ := scheduleFixture(h)
	nip := h.scalar(`SELECT nip FROM hris.employees WHERE id = $1`, ana)
	def := h.scalar(`SELECT id::text FROM hris.shifts WHERE code = 'DEF'`)
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id)
		SELECT $1, d::date, $2, gen_random_uuid() FROM generate_series('2026-10-05'::date, '2026-10-11', interval '1 day') d`, ana, def)
	file := anaWeek(branch, nip, []string{"DEF", "L", "DEF", "DEF", "DEF", "L", "DEF"})
	fields := map[string]string{"branch_id": branch, "from": "2026-10-05", "to": "2026-10-11", "step": "preview"}
	pre := upload(t, h, &writer, fields, file)
	expect(t, pre, http.StatusOK, "")
	if pre.data()["replaced_days"] != float64(2) {
		t.Fatalf("replaced_days = %v, want 2 (06/10 & 10/10 DEF→L)", pre.data()["replaced_days"])
	}
	fields["step"], fields["fingerprint"], fields["ack_warnings"] = "commit", pre.data()["fingerprint"].(string), "true"
	expect(t, upload(t, h, &writer, fields, file), http.StatusForbidden, scheduleNeedsDelete)
	expect(t, upload(t, h, &deleter, fields, file), http.StatusBadRequest, hrisdel.ErrReason)
	fields["reason"] = "Revisi jadwal minggu ini"
	expect(t, upload(t, h, &deleter, fields, file), http.StatusOK, "")
	if got := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'hris.schedule.replace'
		AND reason = 'Revisi jadwal minggu ini' AND (before->>'replaced_days')::int = 2`); got != "1" {
		t.Fatalf("replace audit rows = %s", got)
	}
}

func TestScheduleTypeClearingDaysNeedsDelete(t *testing.T) {
	h := newHarness(t)
	writer := h.staffActions("hrd", scheduleWriter)
	deleter, _ := h.staff("hrd", scheduleTestMenus, false)
	e := h.employee(emp{})
	free := h.employee(emp{})
	def := h.scalar(`SELECT id::text FROM hris.shifts WHERE code = 'DEF'`)
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id) VALUES
		($1, '2026-10-07', $2, gen_random_uuid()), ($1, '2026-10-08', $2, gen_random_uuid())`, e, def)
	body := map[string]any{"schedule_type": "non_shift", "effective_from": "2026-10-06"}
	// Nothing to clear: no delete needed.
	expect(t, h.do(&writer, http.MethodPut, "/api/hris/employees/"+free+"/schedule-type", body), http.StatusOK, "")
	expect(t, h.do(&writer, http.MethodPut, "/api/hris/employees/"+e+"/schedule-type", body), http.StatusForbidden, scheduleNeedsDelete)
	if h.scalar(`SELECT schedule_type FROM hris.employees WHERE id = $1`, e) != "shift" {
		t.Fatal("a refused change must not apply")
	}
	expect(t, h.do(&deleter, http.MethodPut, "/api/hris/employees/"+e+"/schedule-type", body), http.StatusBadRequest, hrisdel.ErrReason)
	body["reason"] = "Pindah ke kantor pusat"
	expect(t, h.do(&deleter, http.MethodPut, "/api/hris/employees/"+e+"/schedule-type", body), http.StatusOK, "")
	if got := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'hris.schedule.clear' AND entity_id = $1
		AND reason = 'Pindah ke kantor pusat' AND (before->>'cleared_days')::int = 2`, e); got != "1" {
		t.Fatalf("clear audit rows = %s", got)
	}
	// Jadwal tetap on a Non-Shift employee who still has a per-date day.
	ns := h.employee(emp{})
	h.exec(`UPDATE hris.employees SET schedule_type = 'non_shift' WHERE id = $1`, ns)
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id) VALUES ($1, '2026-10-09', $2, gen_random_uuid())`, ns, def)
	fixed := map[string]any{"employee_ids": []string{ns}, "effective_from": "2026-10-06"}
	expect(t, h.do(&writer, http.MethodPost, "/api/hris/schedules/fixed", fixed), http.StatusForbidden, scheduleNeedsDelete)
	fixed["reason"] = "Bersihkan jadwal lama"
	expect(t, h.do(&deleter, http.MethodPost, "/api/hris/schedules/fixed", fixed), http.StatusOK, "")
}
```

- [ ] **Step 2: Jalankan, pastikan gagal**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris -run 'TestScheduleUploadReplacingDaysNeedsDelete|TestScheduleTypeClearingDaysNeedsDelete' -count=1 -v`
Expected: FAIL. Contoh: `replaced_days = <nil>, want 2` dan `status 200, want 403`.

- [ ] **Step 3: Tulis `clearGuard`**

`backend/internal/modules/hris/schedule_guard.go`:

```go
package hris

import (
	"context"
	"net/http"
	"strings"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/hrisdel"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
)

// errScheduleNeedsDelete: a schedule change would remove per-date rows and
// the caller lacks "delete" on Jadwal Kerja (EPIC-061, Q-RA-1/3).
const errScheduleNeedsDelete = "Menimpa atau menghapus jadwal yang sudah ada butuh hak hapus Jadwal Kerja"

// clearGuard gates schedule changes that remove per-date rows: "delete" on
// Jadwal Kerja plus a reason, and one audit row. Nothing removed, nothing
// asked (ruling R4).
type clearGuard struct {
	Allowed bool
	Reason  string
	Meta    audit.Entry
}

func (g clearGuard) check(n int) error {
	if n == 0 {
		return nil
	}
	if !g.Allowed {
		return httpx.Forbidden(errScheduleNeedsDelete)
	}
	return hrisdel.CheckReason(g.Reason)
}

// audit writes the audit row when n per-date rows were removed.
func (g clearGuard) audit(ctx context.Context, q database.Querier, actor *auth.User, action, entityID string, before map[string]any, n int) error {
	if n == 0 {
		return nil
	}
	reason := strings.TrimSpace(g.Reason)
	e := g.Meta
	e.ActorID, e.ActorName = actor.ID, &actor.FullName
	e.Action, e.Entity, e.EntityID = action, "hris.employee_shift_days", &entityID
	e.Before, e.Reason = before, &reason
	return audit.Write(ctx, q, e)
}

// clearGuardFor reads "delete" on Jadwal Kerja for the caller.
func (h handlers) clearGuardFor(r *http.Request, u *auth.User, reason string) (clearGuard, error) {
	allowed, err := h.auth.HasMenuAction(r.Context(), u, "delete", iam.LeafJadwalKerja)
	if err != nil {
		return clearGuard{}, err
	}
	return clearGuard{Allowed: allowed, Reason: reason, Meta: auditMeta(r)}, nil
}

// replacedScheduleDays counts the per-date rows an upload would overwrite
// with a different shift (a libur cell has no shift id).
func replacedScheduleDays(ctx context.Context, q database.Querier, as []domain.ScheduleAssignment) (int, error) {
	ids, dates := assignmentKeys(as)
	shifts := make([]string, len(as))
	for i, a := range as {
		if a.ShiftID != nil {
			shifts[i] = *a.ShiftID
		}
	}
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM hris.employee_shift_days d
		JOIN unnest($1::uuid[], $2::date[], $3::text[]) AS k(employee_id, work_date, shift_id)
		  ON k.employee_id = d.employee_id AND k.work_date = d.work_date
		WHERE d.shift_id IS DISTINCT FROM NULLIF(k.shift_id, '')::uuid`, ids, dates, shifts).Scan(&n)
	return n, err
}
```

- [ ] **Step 4: Tipe jadwal & jadwal tetap**

Di `backend/internal/modules/hris/schedule_type.go`:

`applyFixedSchedule` menjadi:

```go
// applyFixedSchedule clears per-date schedules from `from` on and saves the
// fixed weekly pattern from `from`; it returns the cleared per-date rows.
// Clearing rows needs the clear guard and is audited (EPIC-061).
func applyFixedSchedule(ctx context.Context, r Repository, employeeID, from string, actor *auth.User, g clearGuard) (int, error) {
	q := r.querier()
	tag, err := q.Exec(ctx, `DELETE FROM hris.employee_shift_days WHERE employee_id = $1 AND work_date >= $2::date`, employeeID, from)
	if err != nil {
		return 0, err
	}
	n := int(tag.RowsAffected())
	if err := g.check(n); err != nil {
		return 0, err
	}
	if err := g.audit(ctx, q, actor, "hris.schedule.clear", employeeID, map[string]any{"cleared_days": n, "from": from}, n); err != nil {
		return 0, err
	}
	ns, err := shiftIDByCode(ctx, q, "NS")
	if err != nil {
		return 0, err
	}
	return n, r.SaveShiftPattern(ctx, employeeID, from, fixedPatternDays(ns), actor.FullName)
}
```

`ChangeScheduleType`:
- signature → `func (s *Service) ChangeScheduleType(ctx context.Context, actor *auth.User, ids []string, to, from string, dryRun bool, g clearGuard) (*scheduleTypeResult, error)`;
- panggilan di dalamnya → `res, err = changeScheduleTypeTx(ctx, r, actor, ids, to, from, dryRun, g)`.

`ApplyFixedSchedule`:
- signature → `func (s *Service) ApplyFixedSchedule(ctx context.Context, actor *auth.User, branchID string, ids []string, from string, g clearGuard) (*scheduleTypeResult, error)`;
- panggilan di loop-nya → `n, err := applyFixedSchedule(ctx, r, t.ID, from, actor, g)`.

`changeScheduleTypeTx`:
- signature → `func changeScheduleTypeTx(ctx context.Context, r Repository, actor *auth.User, ids []string, to, from string, dryRun bool, g clearGuard) (*scheduleTypeResult, error)`;
- panggilan di cabang non-dry → `n, err := applyFixedSchedule(ctx, r, t.ID, from, actor, g)`.

`scheduleTypeBody` dapat field `reason string`. Di `readScheduleTypeBody`, sebelum `if bulk {`, tambahkan:

```go
	b.reason = strOr(f.Str("reason", optional, validate.StrOpts{}))
```

Handler `putScheduleType`, `bulkScheduleType`, `applyFixedSchedule` menjadi:

```go
func (h handlers) putScheduleType(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	id, err := requireUUID(r.PathValue("id"), "ID karyawan tidak valid")
	if err != nil {
		return err
	}
	b, err := readScheduleTypeBody(r, false)
	if err != nil {
		return err
	}
	g, err := h.clearGuardFor(r, u, b.reason)
	if err != nil {
		return err
	}
	res, err := h.svc.ChangeScheduleType(r.Context(), u, []string{id}, b.to, b.from, false, g)
	if err != nil {
		return err
	}
	return ok(w, dataMsg(res, "Tipe jadwal disimpan"))
}

func (h handlers) bulkScheduleType(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	b, err := readScheduleTypeBody(r, true)
	if err != nil {
		return err
	}
	g, err := h.clearGuardFor(r, u, b.reason)
	if err != nil {
		return err
	}
	res, err := h.svc.ChangeScheduleType(r.Context(), u, b.ids, b.to, b.from, b.dryRun, g)
	if err != nil {
		return err
	}
	return ok(w, dataMsg(res, fmt.Sprintf("Tipe jadwal %d karyawan disimpan", res.Changed)))
}

func (h handlers) applyFixedSchedule(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	branch := f.UUID("branch_id", nullish)
	ids := f.Strings("employee_ids", optional, 1000, validate.StrOpts{Check: validate.UUIDCheck})
	from := strOr(reqStr(f, "effective_from", "Tanggal mulai wajib diisi (YYYY-MM-DD)", validate.StrOpts{Check: matches(domain.DateRe, "Tanggal mulai wajib diisi (YYYY-MM-DD)")}))
	reason := strOr(f.Str("reason", optional, validate.StrOpts{}))
	if branch == nil && len(ids) == 0 {
		f.Fail("branch_id", "custom", "Pilih cabang atau karyawan")
	}
	if err := formErr(f, ""); err != nil {
		return err
	}
	g, err := h.clearGuardFor(r, u, reason)
	if err != nil {
		return err
	}
	res, err := h.svc.ApplyFixedSchedule(r.Context(), u, strOr(branch), ids, from, g)
	if err != nil {
		return err
	}
	return ok(w, dataMsg(res, fmt.Sprintf("Jadwal tetap diterapkan ke %d karyawan", res.Changed)))
}
```

- [ ] **Step 5: Impor jadwal**

Di `backend/internal/modules/hris/schedule_days.go`:

1. `scheduleImportInput` dapat field `Guard clearGuard`.
2. Di `ImportSchedule`, tepat setelah blok `fp, err := scheduleFingerprint(...)`, ganti blok pratinjau dengan:

```go
		replaced, err := replacedScheduleDays(ctx, q, res.Assignments)
		if err != nil {
			return err
		}
		if in.Step != "commit" {
			out = map[string]any{"summary": summary, "errors": res.Errors, "warnings": res.Warnings,
				"fingerprint": fp, "changes": changes, "replaced_days": replaced}
			return nil
		}
```

3. Setelah `switch` (errors / ack / fingerprint) dan sebelum `var batch string`, sisipkan:

```go
		if err := in.Guard.check(replaced); err != nil {
			return err
		}
```

4. Setelah loop `for _, id := range res.EmployeeIDs { writeEmployeeAudit(...) }` dan sebelum `out = map[string]any{"summary": ...}`, sisipkan:

```go
		if err := in.Guard.audit(ctx, q, actor, "hris.schedule.replace", batch, map[string]any{
			"replaced_days": replaced, "branch": rng.Branch.Name, "from": rng.from(), "to": rng.to()}, replaced); err != nil {
			return err
		}
```

5. Handler `importSchedules`, setelah `defer file.Close()`/`io.ReadAll`, sebelum `in := scheduleImportInput{...}`:

```go
	g, err := h.clearGuardFor(r, u, strings.TrimSpace(r.FormValue("reason")))
	if err != nil {
		return err
	}
```

lalu tambahkan `Guard: g` ke literal `scheduleImportInput`. Pastikan `strings` sudah diimpor di file ini (ia dipakai `ImportSchedule`).

- [ ] **Step 6: Impor data karyawan**

Di `backend/internal/modules/hris/employee_excel.go`:

1. `employeeImportInput` dapat field `Guard clearGuard`.
2. `applyImportRow` dapat parameter terakhir `g clearGuard` (signature: `func (s *Service) applyImportRow(ctx context.Context, r Repository, actor *auth.User, row domain.EmployeeImportRow, id string, before map[string]any, branch, today string, g clearGuard) error`).
3. Di dalam `applyImportRow`:
   - `applyFixedSchedule(ctx, r, id, today, actor.FullName)` → `applyFixedSchedule(ctx, r, id, today, actor, g)`;
   - `changeScheduleTypeTx(ctx, r, actor, []string{id}, scheduleTo, today, false)` → `changeScheduleTypeTx(ctx, r, actor, []string{id}, scheduleTo, today, false, g)`.
4. Pemanggil di `ImportEmployees`: `s.applyImportRow(ctx, r, actor, p.row, p.id, p.before, p.branch, today)` → `s.applyImportRow(ctx, r, actor, p.row, p.id, p.before, p.branch, today, in.Guard)`.
5. Handler `importEmployees`, sebelum `in := employeeImportInput{...}`:

```go
	// Ruling R4: an import that clears per-date days needs "delete" on Jadwal
	// Kerja; without a reason from the form the file name is the reason.
	reason := strings.TrimSpace(r.FormValue("reason"))
	if reason == "" {
		reason = "Impor data karyawan: " + header.Filename
	}
	g, err := h.clearGuardFor(r, u, reason)
	if err != nil {
		return err
	}
```

lalu tambahkan `Guard: g` ke literal `employeeImportInput`.

- [ ] **Step 7: Sesuaikan test lama**

`backend/internal/modules/hris/schedule_type_integration_test.go`, `TestScheduleTypeToNonShiftClearsFutureDays`: PUT pertama (menghapus 2 hari) diberi alasan:

```go
	c := h.do(&hr, http.MethodPut, "/api/hris/employees/"+e+"/schedule-type", map[string]any{
		"schedule_type": "non_shift", "effective_from": "2026-10-06", "reason": "Pindah ke jadwal tetap kantor"})
```

- [ ] **Step 8: Jalankan, pastikan lolos**

Run: `cd backend && $HOME/.local/go/bin/go build ./... && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris/... -count=1 2>&1 | tail -30`
Expected:
- build bersih;
- `TestScheduleUploadReplacingDaysNeedsDelete` dan `TestScheduleTypeClearingDaysNeedsDelete` PASS;
- `TestScheduleUploadPreviewCommit`, `TestScheduleUploadKeepsOtherBranchDays`, `TestScheduleTypeToNonShiftClearsFutureDays`, test `employee_excel_*` tetap PASS;
- tidak ada kegagalan baru di luar baseline.

- [ ] **Step 9: Commit**

```bash
git add backend/internal/modules/hris
git commit -m "feat(hris): impor jadwal yang menimpa dan ubah tipe jadwal yang menghapus hari butuh delete Jadwal Kerja + alasan + audit, EPIC-061

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 6: Frontend & fallback Next — alasan untuk setiap hapus

**Files:**
- Create: `frontend/src/lib/hris/delete-reason.ts`, `frontend/src/lib/hris/delete-reason.test.ts`
- Create: `frontend/src/components/hris/delete-reason-host.tsx`, `frontend/src/components/hris/delete-reason-host.test.tsx`
- Modify: `frontend/src/components/providers/query-provider.tsx`
- Modify (api + mutations): `frontend/src/features/hris/{shifts,holidays,job-portal,kpi,placements,payroll,leaves,psikotes,announcements,candidates}/{api,mutations}.ts`, `frontend/src/features/master-data/{departments,positions,employment-statuses}/{api,mutations}.ts`, `frontend/src/features/users/{api,mutations}.ts`
- Modify (alur jadwal): `frontend/src/features/hris/schedules/api.ts`, `frontend/src/features/hris/schedules/components/schedule-upload-dialog.tsx`, `frontend/src/features/hris/schedules/components/fixed-schedule-dialog.tsx`, `frontend/src/features/users/components/schedule-type-dialog.tsx`, `frontend/src/features/users/components/bulk-schedule-type-dialog.tsx`, `frontend/src/features/users/components/employee-shifts-tab.tsx`
- Create: `frontend/src/lib/hris/delete-guard.ts`, `frontend/src/lib/hris/delete-guard.test.ts`
- Modify: `frontend/src/lib/audit/index.ts` (tipe), `frontend/src/lib/hris/leaves-repo.ts`
- Modify (22 route Next, R13): lihat Step 7.

**Interfaces:**
- Consumes: pesan `hrisdel.ErrReason`; daun dari Task 1.
- Produces (TS):
  - `HRIS_DELETE_REASON_ERROR`, `MIN_DELETE_REASON`, `class DeleteCancelledError`;
  - `askDeleteReason(req): Promise<string>`, `withDeleteReason<A, T>(title, fn: (arg: A, reason: string) => Promise<T>): (arg: A) => Promise<T>`, `retryWithReason<T>(title, run: (reason?: string) => Promise<T>): Promise<T>`;
  - `withReasonQuery(url, reason)`, `isValidDeleteReason(reason)`;
  - `subscribeDeleteReason`, `currentDeleteReason`, `type PendingDeleteReason`;
  - `<DeleteReasonHost />`;
  - `HRIS_LEAF`, `requireHrisDelete(request, leaf): Promise<{ user: ApiUser; reason: string }>`, `auditHrisDelete(request, user, entity, entityId, reason)`;
  - tipe `HrisDeleteEntity`.

- [ ] **Step 1: Tulis test yang gagal**

`frontend/src/lib/hris/delete-reason.test.ts`:

```ts
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  DeleteCancelledError,
  HRIS_DELETE_REASON_ERROR,
  askDeleteReason,
  currentDeleteReason,
  isValidDeleteReason,
  retryWithReason,
  withDeleteReason,
  withReasonQuery,
} from "./delete-reason";

afterEach(() => currentDeleteReason()?.reject(new DeleteCancelledError()));

describe("delete-reason", () => {
  it("alasan minimal 5 karakter setelah trim", () => {
    expect(isValidDeleteReason("  abcd ")).toBe(false);
    expect(isValidDeleteReason("abcde")).toBe(true);
  });

  it("menambah reason ke query", () => {
    expect(withReasonQuery("/api/hris/shifts/1", "Shift dobel")).toBe("/api/hris/shifts/1?reason=Shift%20dobel");
    expect(withReasonQuery("/api/hris/kpi/targets?id=1", "a b")).toBe("/api/hris/kpi/targets?id=1&reason=a%20b");
  });

  it("askDeleteReason menunggu dialog, lalu bersih", async () => {
    const p = askDeleteReason({ title: "Hapus shift" });
    expect(currentDeleteReason()?.title).toBe("Hapus shift");
    currentDeleteReason()!.resolve("Shift dobel");
    await expect(p).resolves.toBe("Shift dobel");
    expect(currentDeleteReason()).toBeNull();
  });

  it("permintaan baru membatalkan yang lama", async () => {
    const first = askDeleteReason({ title: "A" });
    const second = askDeleteReason({ title: "B" });
    await expect(first).rejects.toBeInstanceOf(DeleteCancelledError);
    currentDeleteReason()!.resolve("Alasan kedua");
    await expect(second).resolves.toBe("Alasan kedua");
  });

  it("withDeleteReason meneruskan alasan ke fungsi hapus", async () => {
    const del = vi.fn(async (id: string, reason: string) => `${id}:${reason}`);
    const run = withDeleteReason("Hapus shift", del)("s1");
    currentDeleteReason()!.resolve("Shift dobel");
    await expect(run).resolves.toBe("s1:Shift dobel");
  });

  it("retryWithReason bertanya hanya bila server minta alasan", async () => {
    const calls: (string | undefined)[] = [];
    const run = retryWithReason("Timpa jadwal", async (reason) => {
      calls.push(reason);
      if (!reason) throw new Error(HRIS_DELETE_REASON_ERROR);
      return "ok";
    });
    await vi.waitFor(() => expect(currentDeleteReason()).not.toBeNull());
    currentDeleteReason()!.resolve("Revisi jadwal");
    await expect(run).resolves.toBe("ok");
    expect(calls).toEqual([undefined, "Revisi jadwal"]);
    await expect(retryWithReason("x", async () => { throw new Error("lain"); })).rejects.toThrow("lain");
    expect(currentDeleteReason()).toBeNull();
  });
});
```

`frontend/src/components/hris/delete-reason-host.test.tsx`:

```tsx
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { DeleteCancelledError, askDeleteReason } from "@/lib/hris/delete-reason";
import { DeleteReasonHost } from "./delete-reason-host";

afterEach(cleanup);

it("meminta alasan ≥ 5 karakter lalu mengembalikannya", async () => {
  render(<DeleteReasonHost />);
  let pending!: Promise<string>;
  act(() => {
    pending = askDeleteReason({ title: "Hapus shift" });
  });
  expect(screen.getByText("Hapus shift")).toBeTruthy();
  const confirm = screen.getByRole("button", { name: "Hapus" }) as HTMLButtonElement;
  expect(confirm.disabled).toBe(true);
  fireEvent.change(screen.getByLabelText(/Alasan/), { target: { value: "Shift dobel" } });
  expect(confirm.disabled).toBe(false);
  act(() => confirm.click());
  await expect(pending).resolves.toBe("Shift dobel");
});

it("Batal menolak dengan DeleteCancelledError", async () => {
  render(<DeleteReasonHost />);
  let pending!: Promise<string>;
  act(() => {
    pending = askDeleteReason({ title: "Hapus libur" });
  });
  act(() => screen.getByRole("button", { name: "Batal" }).click());
  await expect(pending).rejects.toBeInstanceOf(DeleteCancelledError);
});
```

`frontend/src/lib/hris/delete-guard.test.ts`:

```ts
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/auth";

const iam = vi.hoisted(() => ({ allowed: true, calls: [] as unknown[] }));
vi.mock("@/lib/api/auth", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/auth")>()),
  requireIamAction: vi.fn(async (prefixes: readonly string[], action: string) => {
    iam.calls.push([prefixes, action]);
    if (!iam.allowed) throw ApiError.forbidden("Insufficient permissions");
    return { id: "u1", full_name: "Rina HR", role: "hrd", brand_id: null };
  }),
}));
const audit = vi.hoisted(() => ({ recordAuditAfterCommit: vi.fn(async () => {}) }));
vi.mock("@/lib/audit", async (orig) => ({
  ...(await orig<typeof import("@/lib/audit")>()),
  recordAuditAfterCommit: audit.recordAuditAfterCommit,
}));

import { HRIS_LEAF, auditHrisDelete, requireHrisDelete } from "./delete-guard";
import { HRIS_DELETE_REASON_ERROR } from "./delete-reason";

describe("requireHrisDelete", () => {
  beforeEach(() => {
    iam.allowed = true;
    iam.calls = [];
    audit.recordAuditAfterCommit.mockClear();
  });

  it("memeriksa aksi delete pada menu daun", async () => {
    const out = await requireHrisDelete(new Request("http://x/api/hris/shifts/1?reason=Shift%20dobel", { method: "DELETE" }), HRIS_LEAF.shift);
    expect(iam.calls).toEqual([[["hris.kepegawaian.shifts"], "delete"]]);
    expect(out.reason).toBe("Shift dobel");
  });

  it("menolak tanpa hak hapus atau tanpa alasan", async () => {
    iam.allowed = false;
    await expect(requireHrisDelete(new Request("http://x/a?reason=Shift%20dobel"), HRIS_LEAF.shift)).rejects.toMatchObject({ status: 403 });
    iam.allowed = true;
    await expect(requireHrisDelete(new Request("http://x/a?reason=abcd"), HRIS_LEAF.shift)).rejects.toMatchObject({
      status: 400,
      message: HRIS_DELETE_REASON_ERROR,
    });
  });

  it("alasan dari body JSON", async () => {
    const req = new Request("http://x/a", { method: "DELETE", body: JSON.stringify({ reason: "Data ganda sistem" }) });
    expect((await requireHrisDelete(req, HRIS_LEAF.absensi)).reason).toBe("Data ganda sistem");
  });

  it("audit memakai entity.delete", async () => {
    const user = { id: "u1", full_name: "Rina HR", role: "hrd", brand_id: null } as const;
    await auditHrisDelete(new Request("http://x/a"), user, "hris.shifts", "s1", "Shift dobel");
    expect(audit.recordAuditAfterCommit).toHaveBeenCalledWith(
      expect.objectContaining({ action: "hris.shifts.delete", entity: "hris.shifts", entityId: "s1", reason: "Shift dobel", actor: { id: "u1", name: "Rina HR" } })
    );
  });
});
```

- [ ] **Step 2: Jalankan, pastikan gagal**

Run: `cd frontend && npx vitest run src/lib/hris/delete-reason.test.ts src/components/hris/delete-reason-host.test.tsx src/lib/hris/delete-guard.test.ts`
Expected: FAIL — `Failed to resolve import "./delete-reason"` (dan dua lainnya).

- [ ] **Step 3: Tulis `delete-reason.ts` dan host**

`frontend/src/lib/hris/delete-reason.ts`:

```ts
/**
 * Alasan hapus HRIS (EPIC-061, Q-RA-4): setiap hapus/timpa data HRIS wajib
 * alasan ≥ 5 karakter. Satu dialog global (<DeleteReasonHost/>) dibuka lewat
 * askDeleteReason; mutasi hapus memakai withDeleteReason, alur jadwal memakai
 * retryWithReason (bertanya hanya bila server meminta).
 */

/** Sama persis dengan Go hrisdel.ErrReason. */
export const HRIS_DELETE_REASON_ERROR = "Alasan hapus wajib diisi (minimal 5 karakter)";
export const MIN_DELETE_REASON = 5;

export class DeleteCancelledError extends Error {
  constructor() {
    super("Penghapusan dibatalkan");
    this.name = "DeleteCancelledError";
  }
}

export interface DeleteReasonRequest {
  title: string;
  description?: string;
  confirmLabel?: string;
}

export interface PendingDeleteReason extends DeleteReasonRequest {
  id: number;
  resolve: (reason: string) => void;
  reject: (error: Error) => void;
}

let pending: PendingDeleteReason | null = null;
let nextId = 1;
const listeners = new Set<() => void>();

function publish(next: PendingDeleteReason | null) {
  pending = next;
  listeners.forEach((listener) => listener());
}

export function subscribeDeleteReason(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function currentDeleteReason(): PendingDeleteReason | null {
  return pending;
}

export function isValidDeleteReason(reason: string): boolean {
  return [...reason.trim()].length >= MIN_DELETE_REASON;
}

/** Buka dialog alasan; permintaan sebelumnya (bila ada) dibatalkan. */
export function askDeleteReason(request: DeleteReasonRequest): Promise<string> {
  pending?.reject(new DeleteCancelledError());
  return new Promise<string>((resolve, reject) => {
    const id = nextId++;
    const done = () => {
      if (pending?.id === id) publish(null);
    };
    publish({
      ...request,
      id,
      resolve: (reason) => {
        done();
        resolve(reason);
      },
      reject: (error) => {
        done();
        reject(error);
      },
    });
  });
}

export function withReasonQuery(url: string, reason: string): string {
  return `${url}${url.includes("?") ? "&" : "?"}reason=${encodeURIComponent(reason)}`;
}

/** mutationFn hapus: tanya alasan dulu, lalu panggil fn(arg, alasan). */
export function withDeleteReason<A, T>(title: string, fn: (arg: A, reason: string) => Promise<T>): (arg: A) => Promise<T> {
  return async (arg: A) => fn(arg, await askDeleteReason({ title }));
}

/** Jalankan run(); bila server meminta alasan, tanya lalu ulangi sekali. */
export async function retryWithReason<T>(title: string, run: (reason?: string) => Promise<T>): Promise<T> {
  try {
    return await run();
  } catch (error) {
    if (!(error instanceof Error) || error.message !== HRIS_DELETE_REASON_ERROR) throw error;
    return run(await askDeleteReason({ title }));
  }
}
```

`frontend/src/components/hris/delete-reason-host.tsx`:

```tsx
"use client";

import { useState, useSyncExternalStore } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogFooter,
  DialogPanel,
  DialogPanelBody,
  DialogPanelDescription,
  DialogPanelHeader,
  DialogPanelTitle,
} from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import {
  DeleteCancelledError,
  MIN_DELETE_REASON,
  currentDeleteReason,
  isValidDeleteReason,
  subscribeDeleteReason,
  type PendingDeleteReason,
} from "@/lib/hris/delete-reason";

/** Dialog alasan hapus HRIS untuk seluruh aplikasi (EPIC-061, Q-RA-4). */
export function DeleteReasonHost() {
  const pending = useSyncExternalStore(subscribeDeleteReason, currentDeleteReason, () => null);
  if (!pending) return null;
  return <ReasonDialog key={pending.id} pending={pending} />;
}

function ReasonDialog({ pending }: { pending: PendingDeleteReason }) {
  const [reason, setReason] = useState("");
  const ok = isValidDeleteReason(reason);
  const cancel = () => pending.reject(new DeleteCancelledError());
  return (
    <Dialog open onOpenChange={(open) => (open ? undefined : cancel())}>
      <DialogPanel size="xs">
        <DialogPanelHeader>
          <DialogPanelTitle>{pending.title}</DialogPanelTitle>
          <DialogPanelDescription>
            {pending.description ?? "Penghapusan dicatat di audit log beserta alasannya."}
          </DialogPanelDescription>
        </DialogPanelHeader>
        <DialogPanelBody>
          <label htmlFor="hris-delete-reason" className="mb-1 block text-sm font-medium text-gray-700">
            Alasan <span className="text-red-500">*</span>
          </label>
          <Textarea
            id="hris-delete-reason"
            rows={2}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder={`Minimal ${MIN_DELETE_REASON} karakter`}
          />
        </DialogPanelBody>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={cancel}>
            Batal
          </Button>
          <Button
            type="button"
            disabled={!ok}
            onClick={() => pending.resolve(reason.trim())}
            className="bg-red-600 text-white hover:bg-red-700"
          >
            {pending.confirmLabel ?? "Hapus"}
          </Button>
        </DialogFooter>
      </DialogPanel>
    </Dialog>
  );
}
```

`frontend/src/components/providers/query-provider.tsx`:
- tambahkan `import { DeleteReasonHost } from "@/components/hris/delete-reason-host";`;
- di dalam `<QueryClientProvider client={queryClient}>`, tepat setelah `{children}`, tambahkan `<DeleteReasonHost />`.

- [ ] **Step 4: Fungsi API & mutasi hapus**

Setiap file `api.ts` di bawah: tambahkan `import { withReasonQuery } from "@/lib/hris/delete-reason";` lalu ganti fungsi hapusnya.

```ts
// features/hris/shifts/api.ts
export const deleteShift = (id: string, reason: string) => apiDelete(withReasonQuery(`/api/hris/shifts/${id}`, reason));
// features/hris/holidays/api.ts
export const deleteHoliday = (id: string, reason: string) => apiDelete(withReasonQuery(`/api/hris/holidays/${id}`, reason));
// features/hris/job-portal/api.ts
export const deleteJobOpening = (id: string, reason: string) =>
  apiDelete(withReasonQuery(`/api/hris/job-openings/${id}`, reason));
// features/hris/kpi/api.ts
export const deleteKpiTarget = (id: string, reason: string) =>
  apiDelete(withReasonQuery(buildListUrl(`${BASE}/targets`, { id }), reason));
// features/hris/placements/api.ts
export const deletePlacement = (id: string, reason: string) =>
  apiDelete(withReasonQuery(`/api/hris/employee-placements/${id}`, reason));
// features/hris/payroll/api.ts
export const deletePayrollRun = (runId: string, reason: string) =>
  apiDelete(withReasonQuery(`/api/hris/payroll/${runId}`, reason));
export const deletePayrollInput = (runId: string, employeeId: string, code: string, reason: string) =>
  apiDelete(
    withReasonQuery(
      `/api/hris/payroll/${runId}/inputs?employee_id=${encodeURIComponent(employeeId)}&code=${encodeURIComponent(code)}`,
      reason
    )
  );
// features/hris/leaves/api.ts
export const deleteLeave = (id: string, reason: string) => apiDelete(withReasonQuery(`${BASE}/${id}`, reason));
// features/hris/psikotes/api.ts
export const deleteQuestion = (id: string, reason: string) =>
  apiDelete(withReasonQuery(`/api/psikotes/questions/${id}`, reason));
// features/hris/announcements/api.ts
export const deleteAnnouncement = (id: string, reason: string) =>
  apiDelete(withReasonQuery(`/api/hris/announcements/${id}`, reason));
// features/hris/candidates/api.ts
export const deleteCandidate = (id: string, reason: string) => apiDelete(withReasonQuery(`/api/candidates/${id}`, reason));
// features/master-data/departments/api.ts, positions/api.ts, employment-statuses/api.ts (BASE masing-masing)
export const deleteDepartment = (id: string, reason: string) => apiDelete(withReasonQuery(`${BASE}/${id}`, reason));
export const deletePosition = (id: string, reason: string) => apiDelete(withReasonQuery(`${BASE}/${id}`, reason));
export const deleteEmploymentStatus = (id: string, reason: string) => apiDelete(withReasonQuery(`${BASE}/${id}`, reason));
// features/users/api.ts
export const deleteEmployeeContract = (contractId: string, reason: string) =>
  apiDelete(withReasonQuery(`/api/hris/contracts/${contractId}`, reason));
export const deleteContractSignedDocument = (contractId: string, reason: string) =>
  apiDelete(withReasonQuery(`/api/hris/contracts/${contractId}/signed-document`, reason));
export const deleteEmployeeDocument = (docId: string, reason: string) =>
  apiDelete(withReasonQuery(`/api/hris/employees/documents/${docId}`, reason));
```

Setiap `mutations.ts` di bawah:
- tambahkan `import { withDeleteReason } from "@/lib/hris/delete-reason";`;
- ganti `mutationFn` hook hapusnya (bagian lain hook tetap);
- tambahkan `retry: false`.

```ts
// hris/shifts — useDeleteShift
    mutationFn: withDeleteReason("Hapus shift", (id: string, reason) => deleteShift(id, reason)),
    retry: false,
// hris/holidays — useDeleteHoliday
  return useMutation({ mutationFn: withDeleteReason("Hapus hari libur", (id: string, reason) => deleteHoliday(id, reason)), retry: false, onSuccess });
// hris/job-portal — useDeleteJobOpening
    mutationFn: withDeleteReason("Hapus lowongan", (id: string, reason) => deleteJobOpening(id, reason)),
    retry: false,
// hris/kpi — useDeleteKpiTarget
    mutationFn: withDeleteReason("Hapus target KPI", (id: string, reason) => deleteKpiTarget(id, reason)),
    retry: false,
// hris/placements — useDeletePlacement
  return useMutation({ mutationFn: withDeleteReason("Hapus penempatan", (id: string, reason) => deletePlacement(id, reason)), retry: false, onSuccess });
// hris/payroll — useDeletePayrollInput(runId)
    mutationFn: withDeleteReason(
      "Hapus komponen manual",
      ({ employeeId, code }: { employeeId: string; code: string }, reason) => deletePayrollInput(runId, employeeId, code, reason)
    ),
    retry: false,
// hris/payroll — useDeletePayrollRun
    mutationFn: withDeleteReason("Hapus payroll run", (runId: string, reason) => deletePayrollRun(runId, reason)),
    retry: false,
// hris/leaves — useDeleteLeave
    mutationFn: withDeleteReason("Hapus pengajuan cuti/izin", (id: string, reason) => deleteLeave(id, reason)),
    retry: false,
// hris/psikotes — useDeleteQuestion
    mutationFn: withDeleteReason("Hapus soal psikotes", ({ id }: { id: string; instrumentId: string }, reason) => deleteQuestion(id, reason)),
    retry: false,
// hris/announcements — useDeleteAnnouncement
    mutationFn: withDeleteReason("Hapus pengumuman", (id: string, reason) => deleteAnnouncement(id, reason)),
    retry: false,
// hris/candidates — useDeleteCandidate
    mutationFn: withDeleteReason("Hapus kandidat", (id: string, reason) => deleteCandidate(id, reason)),
    retry: false,
// master-data/departments — useDeleteDepartment
    mutationFn: withDeleteReason("Hapus departemen", (id: string, reason) => deleteDepartment(id, reason)),
    retry: false,
// master-data/positions — useDeletePosition
    mutationFn: withDeleteReason("Hapus jabatan", (id: string, reason) => deletePosition(id, reason)),
    retry: false,
// master-data/employment-statuses — useDeleteEmploymentStatus
    mutationFn: withDeleteReason("Hapus status karyawan", (id: string, reason) => deleteEmploymentStatus(id, reason)),
    retry: false,
// users — useDeleteEmployeeContract / useDeleteContractSignedDocument / useDeleteEmployeeDocument
    mutationFn: withDeleteReason("Hapus draft kontrak", (contractId: string, reason) => deleteEmployeeContract(contractId, reason)),
    retry: false,
    mutationFn: withDeleteReason("Hapus scan kontrak bertanda tangan", (contractId: string, reason) => deleteContractSignedDocument(contractId, reason)),
    retry: false,
    mutationFn: withDeleteReason("Hapus dokumen karyawan", (docId: string, reason) => deleteEmployeeDocument(docId, reason)),
    retry: false,
```

Komponen halaman tidak diubah: mereka tetap memanggil `mutate(arg, {...})`. Batal di dialog memicu `onError` halaman dengan pesan `Penghapusan dibatalkan` (R12).

- [ ] **Step 5: Alur jadwal**

`frontend/src/features/hris/schedules/api.ts`:
- tipe input `formOf` dan `importScheduleFile` dapat `reason?: string`;
- di `formOf`, setelah baris `ack_warnings`, tambahkan `if (input.reason) form.append("reason", input.reason);`.

`frontend/src/features/users/api.ts` — body tiga fungsi dapat `reason?: string`:

```ts
export const saveScheduleType = (id: string, body: { schedule_type: ScheduleType; effective_from: string; reason?: string }) =>
  apiPut<{ data: ScheduleTypeResult; message?: string }>(`/api/hris/employees/${id}/schedule-type`, body);

export const bulkScheduleType = (body: {
  employee_ids: string[];
  schedule_type: ScheduleType;
  effective_from: string;
  dry_run?: boolean;
  reason?: string;
}) => apiPost<{ data: ScheduleTypeResult; message?: string }>("/api/hris/employees/schedule-type/bulk", body);

export const applyFixedSchedule = (body: { branch_id?: string; employee_ids?: string[]; effective_from: string; reason?: string }) =>
  apiPost<{ data: ScheduleTypeResult; message?: string }>("/api/hris/schedules/fixed", body);
```

Di kelima komponen berikut:
- tambahkan `import { retryWithReason } from "@/lib/hris/delete-reason";`;
- ganti panggilan API-nya.

Panggilan pertama tidak membawa `reason`, jadi body sama dengan sekarang dan test komponen yang ada tetap lolos.

```ts
// schedule-upload-dialog.tsx — di commit()
      const res = await retryWithReason("Timpa jadwal yang sudah ada", (reason) =>
        importScheduleFile({ ...range, file, step: "commit", fingerprint: preview.fingerprint, ack_warnings: ack, ...(reason ? { reason } : {}) })
      );
// schedule-type-dialog.tsx — di save()
      const res = await retryWithReason("Hapus jadwal per tanggal", (reason) =>
        saveScheduleType(employeeId, { schedule_type: target, effective_from: from, ...(reason ? { reason } : {}) })
      );
// bulk-schedule-type-dialog.tsx — di save()
      const res = await retryWithReason("Hapus jadwal per tanggal", (reason) =>
        bulkScheduleType({ ...body(false), ...(reason ? { reason } : {}) })
      );
// fixed-schedule-dialog.tsx — di apply()
      const res = await retryWithReason("Hapus jadwal per tanggal", (reason) =>
        applyFixedSchedule({ branch_id: branchId, effective_from: from, ...(reason ? { reason } : {}) })
      );
// employee-shifts-tab.tsx — di handleApplyFixed()
      const res = await retryWithReason("Hapus jadwal per tanggal", (reason) =>
        applyFixedSchedule({ employee_ids: [employeeId], effective_from: effectiveFrom, ...(reason ? { reason } : {}) })
      );
```

- [ ] **Step 6: Guard fallback Next**

`frontend/src/lib/audit/index.ts` — tambahkan sebelum `export type AuditAction`:

```ts
/** Entitas HRIS yang penghapusannya diaudit (EPIC-061; sama dengan Go hrisdel). */
export type HrisDeleteEntity =
  | "hris.employees"
  | "hris.employee_documents"
  | "hris.attendance"
  | "hris.leaves"
  | "hris.shifts"
  | "hris.public_holidays"
  | "hris.employment_contracts"
  | "hris.contract_signed_document"
  | "hris.announcements"
  | "hris.departments"
  | "hris.employment_statuses"
  | "hris.positions"
  | "hris.payroll_runs"
  | "hris.employee_salary"
  | "performance.kpi_targets"
  | "performance.feedback_assignments"
  | "performance.feedback_cycles"
  | "performance.feedback_responses"
  | "recruitment.candidates"
  | "recruitment.candidate_cv"
  | "recruitment.psikotes_questions"
  | "recruitment.job_openings";
```

lalu tambahkan anggota `` | `${HrisDeleteEntity}.delete` `` ke union `AuditAction` dan `| HrisDeleteEntity` ke union `AuditEntity`.

`frontend/src/lib/hris/delete-guard.ts`:

```ts
/**
 * Guard hapus HRIS untuk route fallback Next (EPIC-061, R13): sama dengan Go
 * hrisdel.Guard — aksi "delete" pada menu daun, alasan ≥ 5 karakter, audit.
 */
import { ApiError, requireIamAction, type ApiUser } from "@/lib/api/auth";
import { recordAuditAfterCommit, requestMeta, type HrisDeleteEntity } from "@/lib/audit";
import { HRIS_DELETE_REASON_ERROR, isValidDeleteReason } from "./delete-reason";

/** Menu daun HRIS yang menjaga hapus (sama dengan Go iam.Leaf*). */
export const HRIS_LEAF = {
  karyawan: "hris.kepegawaian.users",
  kontrak: "hris.kepegawaian.contracts",
  shift: "hris.kepegawaian.shifts",
  jadwalKerja: "hris.kepegawaian.schedules",
  hariLibur: "hris.kepegawaian.holidays",
  pengumuman: "hris.kepegawaian.announcements",
  penempatan: "hris.master.placements",
  departemen: "hris.master.departments",
  jabatan: "hris.master.positions",
  statusKaryawan: "hris.master.employment-statuses",
  absensi: "hris.workforce.attendance",
  cuti: "hris.workforce.leaves",
  runPenggajian: "hris.compensation.payroll",
  salary: "hris.compensation.salary",
  kpiScorecard: "hris.performance.kpi-scorecard",
  kandidat: "hris.recruitment.candidates",
  psikotes: "hris.recruitment.psikotes",
  jobPortal: "hris.recruitment.job-portal",
} as const;

async function readDeleteReason(request: Request): Promise<string> {
  const fromQuery = new URL(request.url).searchParams.get("reason")?.trim();
  if (fromQuery) return fromQuery;
  const body = (await request.clone().json().catch(() => null)) as { reason?: unknown } | null;
  return typeof body?.reason === "string" ? body.reason.trim() : "";
}

export async function requireHrisDelete(request: Request, leaf: string): Promise<{ user: ApiUser; reason: string }> {
  const user = await requireIamAction([leaf], "delete");
  const reason = await readDeleteReason(request);
  if (!isValidDeleteReason(reason)) throw ApiError.badRequest(HRIS_DELETE_REASON_ERROR);
  return { user, reason };
}

export async function auditHrisDelete(
  request: Request,
  user: ApiUser,
  entity: HrisDeleteEntity,
  entityId: string | null,
  reason: string
): Promise<void> {
  await recordAuditAfterCommit({
    actor: { id: user.id, name: user.full_name },
    action: `${entity}.delete`,
    entity,
    entityId,
    reason,
    ...requestMeta(request),
  });
}
```

`frontend/src/lib/hris/leaves-repo.ts`:
- hapus konstanta `LEAVE_DELETE_ROLES`;
- ganti `deleteLeave` menjadi:

```ts
/** Hapus pengajuan; route memeriksa aksi "delete" di Cuti & Izin (EPIC-061). */
export async function deleteLeave(id: string) {
  const db = await createServerPgClient();
  unwrap(await db.from("leaves").delete().eq("id", id));
}
```

- [ ] **Step 7: 22 route fallback Next**

Di setiap file berikut (di bawah `frontend/src/app/api/`):
- tambahkan `import { HRIS_LEAF, auditHrisDelete, requireHrisDelete } from "@/lib/hris/delete-guard";`;
- ganti `export const DELETE` dengan versi di bawah;
- buang import yang jadi tidak terpakai (eslint di Step 8 menandainya).

```ts
// hris/shifts/[id]/route.ts
export const DELETE = apiHandler(async (request: NextRequest, { params }: Ctx) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.shift);
  const id = requireUuid((await params).id, "ID shift tidak valid");
  const message = await deleteShift(id);
  await auditHrisDelete(request, user, "hris.shifts", id, reason);
  return NextResponse.json({ message });
}, "hris/shifts/[id].DELETE");

// hris/holidays/[id]/route.ts
export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.hariLibur);
  const id = requireUuid((await params).id, "ID libur tidak valid");
  const deleted = await softDeleteHoliday(id, user.id);
  if (!deleted) throw ApiError.notFound("Hari libur tidak ditemukan");
  await auditHrisDelete(request, user, "hris.public_holidays", id, reason);
  return NextResponse.json({ message: `Libur "${deleted.name}" dihapus` });
}, "hris/holidays DELETE");

// hris/job-openings/[id]/route.ts
export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.jobPortal);
  const { id } = await params;
  await deleteJobOpening(id);
  await auditHrisDelete(request, user, "recruitment.job_openings", id, reason);
  return NextResponse.json({ message: "Lowongan berhasil dihapus" });
}, "hris/job-openings DELETE");

// hris/attendance/[id]/route.ts (hapus juga konstanta DELETE_ROLES)
export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.absensi);
  const { id } = await params;
  await deleteAttendance(id);
  await auditHrisDelete(request, user, "hris.attendance", id, reason);
  return NextResponse.json({ message: "Attendance deleted successfully" });
}, "hris/attendance/[id] DELETE");

// hris/payroll/[id]/route.ts
export const DELETE = apiHandler(async (request: NextRequest, { params }: Ctx) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.runPenggajian);
  const { id } = await params;
  await deleteRun(await createServerPgClient(), id);
  await auditHrisDelete(request, user, "hris.payroll_runs", id, reason);
  return NextResponse.json({ message: "Payroll run berhasil dihapus" });
}, "hris/payroll/[id].DELETE");

// hris/leaves/[id]/route.ts
export const DELETE = apiHandler(async (request: NextRequest, { params }: Ctx) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.cuti);
  const { id } = await params;
  await deleteLeave(id);
  await auditHrisDelete(request, user, "hris.leaves", id, reason);
  return NextResponse.json({ message: "Leave request deleted successfully" });
}, "hris/leaves/[id].DELETE");

// hris/employee-salary/[id]/route.ts
export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.salary);
  const { id } = await params;
  await deactivateSalary(id);
  await auditHrisDelete(request, user, "hris.employee_salary", id, reason);
  return NextResponse.json({ message: "Data salary berhasil dihapus" });
}, "hris/employee-salary/[id] DELETE");

// hris/contracts/[id]/route.ts
export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.kontrak);
  const id = requireUuid((await params).id, "ID kontrak tidak valid");
  await deleteDraftContract(id);
  await auditHrisDelete(request, user, "hris.employment_contracts", id, reason);
  return NextResponse.json({ message: "Draft kontrak dihapus" });
}, "hris/contracts DELETE");

// hris/contracts/[id]/signed-document/route.ts
export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.kontrak);
  const contract = await guardAndLoad(params);
  if (!contract) throw ApiError.notFound("Kontrak tidak ditemukan");
  await removeSignedDocument(contract);
  await auditHrisDelete(request, user, "hris.contract_signed_document", (await params).id, reason);
  return NextResponse.json({ message: "Dokumen bertanda tangan dihapus" });
}, "hris/contracts/signed-document DELETE");

// hris/announcements/[id]/route.ts
export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.pengumuman);
  const id = requireUuid((await params).id);
  await deleteAnnouncement(id);
  await auditHrisDelete(request, user, "hris.announcements", id, reason);
  return NextResponse.json({ message: "Pengumuman dihapus" });
}, "hris/announcements/[id] DELETE");

// hris/employees/documents/[doc_id]/route.ts
export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.karyawan);
  const { doc_id } = await params;
  await deleteEmployeeDocument(doc_id);
  await auditHrisDelete(request, user, "hris.employee_documents", doc_id, reason);
  return NextResponse.json({ message: "Dokumen berhasil dihapus" });
}, "hris/employees/documents DELETE");

// hris/employees/[id]/route.ts
export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.karyawan);
  const { id } = await params;
  const data = await deactivateEmployee(id);
  await auditHrisDelete(request, user, "hris.employees", id, reason);
  return NextResponse.json({ data, message: "Karyawan berhasil dihapus (non-aktif)" });
}, "hris/employees/[id] DELETE");

// master/departments/[id]/route.ts
export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.departemen);
  const { id } = await params;
  if (!isUuid(id)) throw ApiError.badRequest("ID tidak valid");
  await deleteDepartment(id);
  await auditHrisDelete(request, user, "hris.departments", id, reason);
  return NextResponse.json({ message: "Departemen berhasil dihapus" });
}, "master/departments/[id]");

// master/employment-statuses/[id]/route.ts
export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.statusKaryawan);
  const { id } = await params;
  if (!isUuid(id)) throw ApiError.badRequest("ID tidak valid");
  await deleteEmploymentStatus(id);
  await auditHrisDelete(request, user, "hris.employment_statuses", id, reason);
  return NextResponse.json({ message: "Status kepegawaian berhasil dihapus" });
}, "master/employment-statuses/[id]");

// master/positions/[id]/route.ts
export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.jabatan);
  const { id } = await params;
  if (!isUuid(id)) throw ApiError.badRequest("ID tidak valid");
  await deletePosition(id);
  await auditHrisDelete(request, user, "hris.positions", id, reason);
  return NextResponse.json({ message: "Jabatan berhasil dihapus" });
}, "master/positions/[id]");

// candidates/[id]/route.ts — hanya baris guard pertama dan audit yang berubah
export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.kandidat);
  const { id } = await params;
  await requireCandidate(id);

  // retensi psikotes: baris DB terhapus via CASCADE, tapi file di
  // storage/private (gambar tes + snapshot proctoring) harus dibersihkan manual
  const sessions = await query<{ id: string }>(
    "SELECT id FROM recruitment.psikotes_sessions WHERE candidate_id = $1",
    [id]
  );
  await query("DELETE FROM recruitment.candidates WHERE id = $1", [id]);

  if (sessions.length > 0) {
    console.info(
      `[candidates] DELETE ${id} oleh ${user.full_name} (${user.id}): purge ${sessions.length} folder bukti psikotes`
    );
  }
  for (const session of sessions) {
    await deletePrivateFolder(`psikotes/${session.id}`);
  }
  await auditHrisDelete(request, user, "recruitment.candidates", id, reason);
  return NextResponse.json({ message: "Kandidat dihapus" });
}, "api/candidates/[id]");

// candidates/[id]/cv-upload/route.ts
export const DELETE = apiHandler(async (request: Request, { params }: RouteParams) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.kandidat);
  const { id } = await params;
  await requireCandidate(id);
  await setCvUrl(id, null);
  await auditHrisDelete(request, user, "recruitment.candidate_cv", id, reason);
  return NextResponse.json({ message: "CV berhasil dihapus" });
}, "cv-upload");

// psikotes/questions/[id]/route.ts
export const DELETE = apiHandler(async (request: NextRequest, { params }: Ctx) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.psikotes);
  const { id } = await params;
  assertUuid(id, "ID soal tidak valid");
  enforceRateLimit(`psikotes_question_delete_${user.id}`);
  const deleted = await deleteQuestion(id);
  await auditHrisDelete(request, user, "recruitment.psikotes_questions", id, reason);
  return NextResponse.json({ data: deleted, message: "Soal dihapus" });
}, "psikotes-question");

// hris/kpi/targets/route.ts
export const DELETE = apiHandler(async (request: NextRequest) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.kpiScorecard);
  await requireIamMenuPrefix(IAM.hrisPerformanceAdmin);
  const { id } = parseInput(deleteQuerySchema, searchParamsOf(request));
  await deleteTarget(await createServerPgClient(), id);
  await auditHrisDelete(request, user, "performance.kpi_targets", id, reason);
  return NextResponse.json({ message: "Target dihapus" });
}, "hris/kpi/targets.DELETE");

// hris/feedback-responses/[id]/route.ts
export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.kpiScorecard);
  await requireIamMenuPrefix(IAM.hrisPerformanceAdmin);
  const { id } = await params;
  await deleteResponse(id);
  await auditHrisDelete(request, user, "performance.feedback_responses", id, reason);
  return NextResponse.json({ success: true });
}, "hris/feedback-responses/[id] DELETE");

// hris/feedback-cycles/[id]/route.ts
export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteContext) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.kpiScorecard);
  await requireIamMenuPrefix(IAM.hrisPerformanceAdmin);
  const { id } = await params;
  await deleteCycle(id);
  await auditHrisDelete(request, user, "performance.feedback_cycles", id, reason);
  return NextResponse.json({ message: "Cycle deleted successfully" });
}, "hris/feedback-cycles/[id] DELETE");

// hris/feedback-assignments/[id]/route.ts
export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteContext) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.kpiScorecard);
  await requireIamMenuPrefix(IAM.hrisPerformanceAdmin);
  const { id } = await params;
  await deleteAssignment(id);
  await auditHrisDelete(request, user, "performance.feedback_assignments", id, reason);
  return NextResponse.json({ message: "Assignment deleted successfully" });
}, "hris/feedback-assignments/[id] DELETE");
```

- [ ] **Step 8: Jalankan, pastikan lolos**

Run: `cd frontend && npx vitest run src/lib/hris src/components/hris src/features/hris src/features/users src/features/master-data src/lib/audit && npx tsc --noEmit -p . && npx eslint src/lib/hris src/components/hris src/components/providers src/features/hris src/features/users src/features/master-data src/lib/audit src/app/api/hris src/app/api/master src/app/api/candidates src/app/api/psikotes`
Expected:
- vitest: semua lolos, termasuk 3 file test baru, `schedule-type-dialog.test.tsx`, `bulk-schedule-type-dialog.test.tsx`, dan `employee-shifts-tab.test.tsx`;
- `tsc` tanpa output;
- `eslint` tanpa error.

- [ ] **Step 9: Commit**

```bash
git add frontend/src
git commit -m "feat(frontend): dialog alasan untuk setiap hapus HRIS dan guard setara di route fallback Next, EPIC-061

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 7: Audit perubahan hak akses + role sistem hanya super_admin

**Files:**
- Create: `backend/database/migrations/deltas/20261022090100_iam_permission_audit_logs.sql`
- Create: `backend/internal/platform/permaudit/permaudit.go`, test `backend/internal/platform/permaudit/permaudit_integration_test.go`
- Modify: `backend/internal/platform/auth/grants.go` (tambah `HasRoleCode`)
- Modify: `backend/internal/modules/configuration/access/repo.go` (`replacePermissions`, tambah `hrisRoleIDs`)
- Modify: `backend/internal/modules/configuration/access/routes.go` (`updateRole`, `putPermissions`, `requireSuperAdmin`)
- Modify: `backend/internal/modules/configuration/accounts/store.go` (`syncIamRole`)
- Create (test): `backend/internal/modules/configuration/access/system_roles_integration_test.go`, `backend/internal/modules/configuration/accounts/user_role_audit_integration_test.go`
- Modify (test): `backend/internal/modules/configuration/access/access_integration_test.go:122-123`

**Interfaces:**
- Consumes: `iam.HrisStaffRoleCodes` (Task 1); `iam.sync_hris_staff_roles()` (Task 1).
- Produces:
  - tabel `iam.permission_audit_logs(id, actor_id, source, role_id, role_code, menu_code, actions_before jsonb, actions_after jsonb, target_user_id, roles_before text[], roles_after text[], created_at)`;
  - `permaudit.SourceRolePermissions|SourceHrisAccess|SourceHrisMask|SourceUserRole`;
  - `type permaudit.Grants map[string]map[string][]string` (role id → menu → aksi);
  - `type permaudit.Mask struct{ ExcludeMenu bool; ExcludedActions []string }`;
  - fungsi:
    - `permaudit.Snapshot(ctx, q, roleIDs []string) (Grants, error)`
    - `permaudit.Record(ctx, q, actorID, source string, before, after Grants) (int, error)`
    - `permaudit.RecordMask(ctx, q, actorID, roleCode, prefix string, before, after *Mask) error`
    - `permaudit.UserRoleCodes(ctx, q, userID) ([]string, error)`
    - `permaudit.SetUserRole(ctx, q, userID, roleCode, assignedBy string) error`
    - `(*auth.Service).HasRoleCode(ctx, u *auth.User, code string) (bool, error)`
    - `access.hrisRoleIDs(ctx, q) ([]string, error)`.

- [ ] **Step 1: Tulis test yang gagal**

`backend/internal/platform/permaudit/permaudit_integration_test.go`:

```go
package permaudit_test

import (
	"context"
	"testing"

	"nuhabit/backend/internal/platform/permaudit"
	"nuhabit/backend/internal/platform/testutil"
)

func TestRecordWritesOnlyChangedMenus(t *testing.T) {
	actor := testutil.CreateStaff(t, testutil.StaffOptions{})
	tx := testutil.Tx(t)
	ctx := context.Background()
	var role string
	if err := tx.QueryRow(ctx, `INSERT INTO iam.roles (code, name) VALUES ($1, 'Audit Uji') RETURNING id::text`, "go_audit_"+testutil.RandomHex(3)).Scan(&role); err != nil {
		t.Fatal(err)
	}
	before := permaudit.Grants{role: {"a.menu": {"read"}, "b.menu": {"read", "update"}}}
	after := permaudit.Grants{role: {"a.menu": {"read"}, "b.menu": {"read"}, "c.menu": {"read"}}}
	n, err := permaudit.Record(ctx, tx, actor.UserID, permaudit.SourceRolePermissions, before, after)
	if err != nil || n != 2 {
		t.Fatalf("Record = %d, %v; want 2 rows", n, err)
	}
	var got string
	if err := tx.QueryRow(ctx, `SELECT string_agg(menu_code || ':' || coalesce(actions_before::text, 'null') || '>' || coalesce(actions_after::text, 'null'), ',' ORDER BY menu_code)
		FROM iam.permission_audit_logs WHERE role_id = $1 AND actor_id = $2`, role, actor.UserID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != `b.menu:["read", "update"]>["read"],c.menu:null>["read"]` {
		t.Fatalf("rows = %s", got)
	}
}

func TestSetUserRoleRecordsChange(t *testing.T) {
	actor := testutil.CreateStaff(t, testutil.StaffOptions{})
	target := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd"})
	tx := testutil.Tx(t)
	ctx := context.Background()
	for range 2 {
		if err := permaudit.SetUserRole(ctx, tx, target.UserID, "hris_manager", actor.UserID); err != nil {
			t.Fatal(err)
		}
	}
	codes, err := permaudit.UserRoleCodes(ctx, tx, target.UserID)
	if err != nil || len(codes) != 1 || codes[0] != "hris_manager" {
		t.Fatalf("roles = %v, %v", codes, err)
	}
	var rows string
	if err := tx.QueryRow(ctx, `SELECT count(*)::text || ':' || coalesce(max(array_to_string(roles_after, '+')), '') FROM iam.permission_audit_logs
		WHERE source = 'user_role' AND target_user_id = $1`, target.UserID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != "1:hris_manager" {
		t.Fatalf("audit = %s, want one row (the repeat is no change)", rows)
	}
}
```

`backend/internal/modules/configuration/access/system_roles_integration_test.go`:

```go
package access_test

import (
	"encoding/json"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

const errSystemRole = `{"success":false,"error":"Role sistem hanya bisa diubah oleh Super Admin"}`

func TestSystemRolesOnlySuperAdmin(t *testing.T) {
	admin := testutil.CreateStaff(t, testutil.StaffOptions{Menus: map[string][]string{"settings.roles": nil}})
	super := testutil.CreateStaff(t, testutil.StaffOptions{Role: "super_admin"})
	mux, text := setup(t)
	code := "go_sys_" + testutil.RandomHex(3)
	system := text(`INSERT INTO iam.roles (code, name, is_system) VALUES ($1, 'Go System', true) RETURNING id::text`, code)
	menuCode := "go_menu_" + testutil.RandomHex(3)
	menuID := text(`INSERT INTO iam.menus (code, menu_name) VALUES ($1, 'Go Menu') RETURNING id::text`, menuCode)
	perms := map[string]any{"permissions": []any{map[string]any{"menuId": menuID, "isGranted": true}}}

	expect(t, do(t, mux, &admin, "PUT", "/api/settings/iam/roles/"+system, map[string]any{"name": "Ubah"}), 403, errSystemRole)
	expect(t, do(t, mux, &admin, "PUT", "/api/settings/iam/roles/"+system+"/permissions", perms), 403, errSystemRole)
	expect(t, do(t, mux, &super, "PUT", "/api/settings/iam/roles/"+system, map[string]any{"name": "Ubah"}), 200, `{"id":"`+system+`"}`)
	expect(t, do(t, mux, &super, "PUT", "/api/settings/iam/roles/"+system, map[string]any{"code": code + "x"}), 400,
		`{"success":false,"error":"System role code cannot be changed"}`)
	expect(t, do(t, mux, &super, "PUT", "/api/settings/iam/roles/"+system+"/permissions", perms), 200, "")
	if got := text(`SELECT count(*)::text FROM iam.permission_audit_logs WHERE role_id = $1 AND source = 'role_permissions'
		AND menu_code = $2 AND actions_before IS NULL AND actions_after = '["read"]'::jsonb`, system, menuCode); got != "1" {
		t.Fatalf("permission audit rows = %s", got)
	}
}

// Saving admin_hris in Role Management re-runs the sync (ruling R10).
func TestAdminHrisEditResyncsDerivedRoles(t *testing.T) {
	admin := testutil.CreateStaff(t, testutil.StaffOptions{Menus: map[string][]string{"settings.roles": nil}})
	mux, text := setup(t)
	adminHris := text(`SELECT id::text FROM iam.roles WHERE code = 'admin_hris'`)
	raw := text(`SELECT json_agg(json_build_object('menuId', p.menu_id, 'isGranted', true, 'grantedActions', p.granted_actions))::text
		FROM iam.role_menu_permissions p JOIN iam.menus m ON m.id = p.menu_id
		WHERE p.role_id = $1 AND p.is_active AND m.deleted_at IS NULL AND m.code <> 'hris.kepegawaian.holidays'`, adminHris)
	var perms []any
	if err := json.Unmarshal([]byte(raw), &perms); err != nil {
		t.Fatal(err)
	}
	expect(t, do(t, mux, &admin, "PUT", "/api/settings/iam/roles/"+adminHris+"/permissions", map[string]any{"permissions": perms}), 200, "")
	if got := text(`SELECT count(*)::text FROM iam.role_menu_permissions p JOIN iam.roles r ON r.id = p.role_id JOIN iam.menus m ON m.id = p.menu_id
		WHERE r.code = 'hris_manager' AND m.code = 'hris.kepegawaian.holidays' AND p.is_active`); got != "0" {
		t.Fatal("hris_manager kept Hari Libur after admin_hris lost it")
	}
	if got := text(`SELECT count(*)::text FROM iam.permission_audit_logs WHERE source = 'role_permissions'
		AND role_code = 'hris_manager' AND menu_code = 'hris.kepegawaian.holidays' AND actions_after IS NULL`); got != "1" {
		t.Fatalf("derived-role audit rows = %s", got)
	}
	expect(t, do(t, mux, &admin, "PUT", "/api/settings/iam/roles/"+adminHris+"/permissions", map[string]any{"permissions": []any{}}), 400,
		`{"success":false,"error":"admin_hris harus punya minimal satu menu: role HR lain diturunkan darinya"}`)
}
```

`backend/internal/modules/configuration/accounts/user_role_audit_integration_test.go`:

```go
package accounts_test

import (
	"context"
	"testing"
)

func TestUserRoleChangeIsAudited(t *testing.T) {
	e := settingsUsers(t)
	ctx := context.Background()
	var holding string
	if err := e.tx.QueryRow(ctx, `SELECT id::text FROM configuration.holdings ORDER BY created_at LIMIT 1`).Scan(&holding); err != nil {
		t.Fatal(err)
	}
	body := employeeBody(uniqueEmail())
	body["business_scope"], body["holding_id"] = "holding", holding
	body["is_access_app"], body["password"], body["role"] = true, "rahasia123", "hrd"
	code, created, raw := e.do("POST", "/api/users", body)
	if code != 201 {
		t.Fatalf("create = %d %s", code, raw)
	}
	id := created["data"].(map[string]any)["id"].(string)
	var userID string
	if err := e.tx.QueryRow(ctx, `SELECT user_id::text FROM hris.employees WHERE id = $1`, id).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if code, _, raw := e.do("PUT", "/api/users/"+id, map[string]any{"business_scope": "holding", "holding_id": holding,
		"is_access_app": true, "role": "finance_staff"}); code != 200 {
		t.Fatalf("role change = %d %s", code, raw)
	}
	var trail string
	if err := e.tx.QueryRow(ctx, `SELECT string_agg(array_to_string(roles_before, '+') || '>' || array_to_string(roles_after, '+'), ',' ORDER BY created_at)
		FROM iam.permission_audit_logs WHERE source = 'user_role' AND target_user_id = $1`, userID).Scan(&trail); err != nil {
		t.Fatal(err)
	}
	if trail != ">hrd,hrd>finance_staff" {
		t.Fatalf("user role trail = %q", trail)
	}
}
```

Ubah juga `access_integration_test.go`. Admin non-super kini 403:

```go
	expect(t, do(t, mux, &admin, "PUT", "/api/settings/iam/roles/"+system, map[string]any{"code": systemCode + "x"}), 403,
		`{"success":false,"error":"Role sistem hanya bisa diubah oleh Super Admin"}`)
```

- [ ] **Step 2: Jalankan, pastikan gagal**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/platform/permaudit ./internal/modules/configuration/access ./internal/modules/configuration/accounts -run 'TestRecordWritesOnlyChangedMenus|TestSetUserRoleRecordsChange|TestSystemRolesOnlySuperAdmin|TestAdminHrisEditResyncsDerivedRoles|TestRolesCRUDAndPermissions|TestUserRoleChangeIsAudited' -count=1 -v`
Expected: FAIL. Paket `permaudit` belum ada; di paket access ada `got 200 ..., want 403` dan `relation "iam.permission_audit_logs" does not exist`.

- [ ] **Step 3: Migrasi**

`backend/database/migrations/deltas/20261022090100_iam_permission_audit_logs.sql`:

```sql
-- =============================================================================
-- EPIC-061 (Q-RA-8): riwayat perubahan hak akses. Satu baris per role × menu
-- yang aksinya berubah (actions_before/after, NULL = tidak ada grant), per
-- perubahan masker role HRIS (source hris_mask: {exclude_menu,
-- excluded_actions}), dan per penugasan role ke user (roles_before/after).
-- Ditulis aplikasi (paket platform/permaudit); migrasi dan skrip ops tidak.
-- =============================================================================
CREATE TABLE IF NOT EXISTS iam.permission_audit_logs (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  actor_id       uuid REFERENCES auth.users(id) ON DELETE SET NULL,
  source         varchar(40) NOT NULL
                 CHECK (source IN ('role_permissions', 'hris_access', 'hris_mask', 'user_role')),
  role_id        uuid REFERENCES iam.roles(id) ON DELETE SET NULL,
  role_code      varchar(50),
  menu_code      varchar(100),
  actions_before jsonb,
  actions_after  jsonb,
  target_user_id uuid REFERENCES auth.users(id) ON DELETE SET NULL,
  roles_before   text[],
  roles_after    text[],
  created_at     timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX IF NOT EXISTS permission_audit_logs_created_idx ON iam.permission_audit_logs (created_at DESC);
CREATE INDEX IF NOT EXISTS permission_audit_logs_role_idx ON iam.permission_audit_logs (role_code, created_at DESC);
```

Run: `cd backend && DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go run ./cmd/migrate -apply`
Expected: `  [ok]    20261022090100_iam_permission_audit_logs.sql`, `Selesai. 1 migrasi diterapkan.`

- [ ] **Step 4: Paket `permaudit`**

`backend/internal/platform/permaudit/permaudit.go`:

```go
// Package permaudit records IAM permission changes in
// iam.permission_audit_logs (EPIC-061, Q-RA-8): per role and menu the
// actions before and after, HRIS role mask changes, and user role
// assignments.
package permaudit

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/platform/database"
)

// Sources, the iam.permission_audit_logs.source values.
const (
	SourceRolePermissions = "role_permissions"
	SourceHrisAccess      = "hris_access"
	SourceHrisMask        = "hris_mask"
	SourceUserRole        = "user_role"
)

// Grants is role id → menu code → sorted active actions.
type Grants map[string]map[string][]string

// Mask is one iam.hris_staff_role_masks row.
type Mask struct {
	ExcludeMenu     bool     `json:"exclude_menu"`
	ExcludedActions []string `json:"excluded_actions"`
}

// Snapshot reads the active grants of the roles.
func Snapshot(ctx context.Context, q database.Querier, roleIDs []string) (Grants, error) {
	out := Grants{}
	for _, id := range roleIDs {
		out[id] = map[string][]string{}
	}
	if len(roleIDs) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, `SELECT p.role_id::text, m.code, p.granted_actions
		FROM iam.role_menu_permissions p JOIN iam.menus m ON m.id = p.menu_id
		WHERE p.role_id = ANY($1::uuid[]) AND p.is_active AND m.deleted_at IS NULL`, roleIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var role, code string
		var raw []byte
		if err := rows.Scan(&role, &code, &raw); err != nil {
			return nil, err
		}
		var acts []string
		_ = json.Unmarshal(raw, &acts)
		slices.Sort(acts)
		out[role][code] = slices.Compact(acts)
	}
	return out, rows.Err()
}

// Record writes one row per (role, menu) whose actions changed between
// before and after; a grant that appeared has actions_before NULL, one
// that went away actions_after NULL. It returns the rows written.
func Record(ctx context.Context, q database.Querier, actorID, source string, before, after Grants) (int, error) {
	n := 0
	for _, role := range keys(before, after) {
		b, a := before[role], after[role]
		for _, code := range keys(b, a) {
			ba, inB := b[code]
			aa, inA := a[code]
			if inB == inA && slices.Equal(ba, aa) {
				continue
			}
			if _, err := q.Exec(ctx, `INSERT INTO iam.permission_audit_logs
				(actor_id, source, role_id, role_code, menu_code, actions_before, actions_after)
				VALUES ($1, $2, $3::uuid, (SELECT code FROM iam.roles WHERE id = $3::uuid), $4, $5::jsonb, $6::jsonb)`,
				nullable(actorID), source, role, code, actionsJSON(ba, inB), actionsJSON(aa, inA)); err != nil {
				return n, err
			}
			n++
		}
	}
	return n, nil
}

// RecordMask writes one hris_mask row when a mask row changed (nil = no row).
func RecordMask(ctx context.Context, q database.Querier, actorID, roleCode, prefix string, before, after *Mask) error {
	if sameMask(before, after) {
		return nil
	}
	_, err := q.Exec(ctx, `INSERT INTO iam.permission_audit_logs
		(actor_id, source, role_id, role_code, menu_code, actions_before, actions_after)
		VALUES ($1, 'hris_mask', (SELECT id FROM iam.roles WHERE code = $2), $2, $3, $4::jsonb, $5::jsonb)`,
		nullable(actorID), roleCode, prefix, maskJSON(before), maskJSON(after))
	return err
}

// UserRoleCodes lists the user's iam.user_roles role codes, sorted.
func UserRoleCodes(ctx context.Context, q database.Querier, userID string) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT r.code FROM iam.user_roles ur JOIN iam.roles r ON r.id = ur.role_id
		WHERE ur.user_id = $1 ORDER BY r.code`, userID)
	if err != nil {
		return nil, err
	}
	codes, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if codes == nil {
		codes = []string{}
	}
	return codes, err
}

// SetUserRole makes roleCode the user's only IAM role (none when the code
// is unknown), as syncIamPrimaryRole did, and records the change.
func SetUserRole(ctx context.Context, q database.Querier, userID, roleCode, assignedBy string) error {
	if userID == "" || roleCode == "" {
		return nil
	}
	before, err := UserRoleCodes(ctx, q, userID)
	if err != nil {
		return err
	}
	var roleID *string
	err = q.QueryRow(ctx, `SELECT id::text FROM iam.roles WHERE code = $1 AND deleted_at IS NULL LIMIT 1`, roleCode).Scan(&roleID)
	if err != nil && !database.IsNoRows(err) {
		return err
	}
	if _, err := q.Exec(ctx, `DELETE FROM iam.user_roles WHERE user_id = $1`, userID); err != nil {
		return err
	}
	if roleID != nil {
		if _, err := q.Exec(ctx, `INSERT INTO iam.user_roles (user_id, role_id, is_primary, assigned_by) VALUES ($1, $2, true, $3)`,
			userID, *roleID, nullable(assignedBy)); err != nil {
			return err
		}
	}
	after, err := UserRoleCodes(ctx, q, userID)
	if err != nil || slices.Equal(before, after) {
		return err
	}
	_, err = q.Exec(ctx, `INSERT INTO iam.permission_audit_logs (actor_id, source, target_user_id, roles_before, roles_after)
		VALUES ($1, 'user_role', $2, $3, $4)`, nullable(assignedBy), userID, before, after)
	return err
}

func keys[V any](maps ...map[string]V) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range maps {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	slices.Sort(out)
	return out
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func actionsJSON(acts []string, present bool) *string {
	if !present {
		return nil
	}
	if acts == nil {
		acts = []string{}
	}
	raw, _ := json.Marshal(acts)
	s := string(raw)
	return &s
}

func maskJSON(m *Mask) *string {
	if m == nil {
		return nil
	}
	c := Mask{ExcludeMenu: m.ExcludeMenu, ExcludedActions: slices.Sorted(slices.Values(m.ExcludedActions))}
	if c.ExcludedActions == nil {
		c.ExcludedActions = []string{}
	}
	raw, _ := json.Marshal(c)
	s := string(raw)
	return &s
}

func sameMask(a, b *Mask) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.ExcludeMenu == b.ExcludeMenu &&
		slices.Equal(slices.Sorted(slices.Values(a.ExcludedActions)), slices.Sorted(slices.Values(b.ExcludedActions)))
}
```

- [ ] **Step 5: `HasRoleCode` di auth**

Di `backend/internal/platform/auth/grants.go`, setelah `ResolveRoleIDs`:

```go
// HasRoleCode reports whether the user's resolved IAM roles include code
// (EPIC-061: system roles change only through super_admin).
func (s *Service) HasRoleCode(ctx context.Context, u *User, code string) (bool, error) {
	roleIDs, err := s.ResolveRoleIDs(ctx, u.ID, u.Role)
	if err != nil || len(roleIDs) == 0 {
		return false, err
	}
	var ok bool
	err = s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM iam.roles WHERE id = ANY($1::uuid[]) AND code = $2 AND deleted_at IS NULL)`,
		roleIDs, code).Scan(&ok)
	return ok, err
}
```

- [ ] **Step 6: Role Management**

Di `backend/internal/modules/configuration/access/repo.go`, ganti `replacePermissions` dan tambahkan `hrisRoleIDs` (import `"nuhabit/backend/internal/platform/httpx"`, `"nuhabit/backend/internal/platform/iam"`, `"nuhabit/backend/internal/platform/permaudit"`):

```go
// errAdminHrisEmpty: admin_hris is the source of the HR roles; the sync
// refuses an empty source (ruling R10).
const errAdminHrisEmpty = "admin_hris harus punya minimal satu menu: role HR lain diturunkan darinya"

// hrisRoleIDs are the ids of admin_hris and the roles derived from it.
func hrisRoleIDs(ctx context.Context, q database.Querier) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT id::text FROM iam.roles WHERE code = ANY($1) AND deleted_at IS NULL ORDER BY code`, iam.HrisStaffRoleCodes)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// replacePermissions is replaceIamRolePermissionsInDb: deactivate every
// grant of the role, then upsert the checked ones, in one transaction. For
// admin_hris it re-runs the HR role sync; every grant change is recorded in
// iam.permission_audit_logs (EPIC-061).
func replacePermissions(ctx context.Context, db database.DB, roleID string, grants []grant, updatedBy string) error {
	return database.WithTx(ctx, db, func(tx pgx.Tx) error {
		var code string
		if err := tx.QueryRow(ctx, `SELECT code FROM iam.roles WHERE id = $1`, roleID).Scan(&code); err != nil {
			return err
		}
		source := code == "admin_hris"
		watched := []string{roleID}
		if source {
			if len(grants) == 0 {
				return httpx.BadRequest(errAdminHrisEmpty)
			}
			ids, err := hrisRoleIDs(ctx, tx)
			if err != nil {
				return err
			}
			watched = ids
		}
		before, err := permaudit.Snapshot(ctx, tx, watched)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE iam.role_menu_permissions
       SET is_active = false, updated_at = now(), updated_by = $2
       WHERE role_id = $1`, roleID, updatedBy); err != nil {
			return err
		}
		for _, g := range grants {
			actions, _ := json.Marshal(g.Actions)
			if _, err := tx.Exec(ctx, `INSERT INTO iam.role_menu_permissions (
          role_id, menu_id, granted_actions, is_active, created_by, updated_by, updated_at
        ) VALUES ($1, $2, $3::jsonb, true, $4, $4, now())
        ON CONFLICT (role_id, menu_id) DO UPDATE SET
          granted_actions = EXCLUDED.granted_actions,
          is_active = true,
          updated_by = EXCLUDED.updated_by,
          updated_at = now()`, roleID, g.MenuID, string(actions), updatedBy); err != nil {
				return err
			}
		}
		if source {
			if _, err := tx.Exec(ctx, `SELECT iam.sync_hris_staff_roles()`); err != nil {
				return err
			}
		}
		after, err := permaudit.Snapshot(ctx, tx, watched)
		if err != nil {
			return err
		}
		_, err = permaudit.Record(ctx, tx, updatedBy, permaudit.SourceRolePermissions, before, after)
		return err
	})
}
```

Di `backend/internal/modules/configuration/access/routes.go`:

```go
// errSystemRoleSuperAdmin: system roles change only through super_admin
// (EPIC-061, Q-RA-5).
var errSystemRoleSuperAdmin = httpx.Forbidden("Role sistem hanya bisa diubah oleh Super Admin")

func (h handler) requireSuperAdmin(r *http.Request, u *auth.User) error {
	ok, err := h.auth.HasRoleCode(r.Context(), u, "super_admin")
	if err != nil {
		return err
	}
	if !ok {
		return errSystemRoleSuperAdmin
	}
	return nil
}
```

Perubahan di dua handler:
- `updateRole`: baris pertama → `user, err := h.auth.RequireMenuPrefix(r, iam.SettingsRoles...)`. Setelah blok `if existing == nil { return errRoleNotFound }` sisipkan:

```go
	if existing.IsSystem {
		if err := h.requireSuperAdmin(r, user); err != nil {
			return err
		}
	}
```

- `putPermissions`: setelah `row, err := h.roleOr404(r, id)` beserta cek err-nya, sisipkan:

```go
	if row.IsSystem {
		if err := h.requireSuperAdmin(r, user); err != nil {
			return err
		}
	}
```

- [ ] **Step 7: Penugasan role di Manajemen User tercatat**

Di `backend/internal/modules/configuration/accounts/store.go`, ganti isi `syncIamRole` (import `"nuhabit/backend/internal/platform/permaudit"`):

```go
// syncIamRole is syncIamPrimaryRole: the role code becomes the user's only
// IAM role (none when the code is unknown); the change is recorded in
// iam.permission_audit_logs (EPIC-061).
func syncIamRole(ctx context.Context, q database.Querier, userID, roleCode, assignedBy string) error {
	return permaudit.SetUserRole(ctx, q, userID, roleCode, assignedBy)
}
```

- [ ] **Step 8: Jalankan, pastikan lolos**

Run: perintah Step 2, lalu seluruh paket: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/platform/... ./internal/modules/configuration/... -count=1 2>&1 | tail -20`
Expected:
- 6 test baru dan `TestRolesCRUDAndPermissions` PASS;
- `TestResendingSameRoleKeepsIamRoles` tetap PASS;
- tidak ada kegagalan baru di luar baseline.

- [ ] **Step 9: Commit**

```bash
git add backend/database/migrations/deltas/20261022090100_iam_permission_audit_logs.sql backend/internal/platform/permaudit backend/internal/platform/auth/grants.go backend/internal/modules/configuration/access backend/internal/modules/configuration/accounts
git commit -m "feat(iam): audit perubahan hak akses dan penugasan role, role sistem hanya super_admin, simpan admin_hris menjalankan sync, EPIC-061

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 8: API "Hak Akses HRIS" + Role HRIS di Manajemen User (backend)

**Files:**
- Create: `backend/database/migrations/deltas/20261022090200_iam_hris_access_menu.sql`
- Modify: `frontend/src/lib/iam/prefixes.ts` (tambah `hrisAccess`); regenerate `backend/internal/platform/iam/prefixes_gen.go` dan `frontend/src/lib/go-routes.generated.json`
- Create: `backend/internal/modules/configuration/access/hris_access.go`, `backend/internal/modules/configuration/access/hris_access_repo.go`
- Modify: `backend/internal/modules/configuration/access/routes.go` (`Routes` menambah route Hak Akses HRIS)
- Create (test): `backend/internal/modules/configuration/access/hris_access_integration_test.go`
- Modify: `backend/internal/modules/configuration/accounts/input.go`, `routes.go`, `employees.go`, `store.go`, `view.go`
- Create (test): `backend/internal/modules/configuration/accounts/hris_role_integration_test.go`

**Interfaces:**
- Consumes:
  - `iam.HrisStaffRoleCodes`, `iam.HrisAccessScopes` (Task 1);
  - dari Task 7: `permaudit.Snapshot/Record/RecordMask/SetUserRole/UserRoleCodes`, `permaudit.Mask`, `(*auth.Service).HasRoleCode`, `access.hrisRoleIDs`.
- Produces (HTTP):
  - `GET /api/hris/access` → `{"data": {"roles":[{code,name}], "menus":[{code,name,parent_code,level,access_toggle,delete_toggle}], "grants":{role:{menu:[actions]}}, "accounts":[{user_id,full_name,email,hris_role}]}}`;
  - `PUT /api/hris/access/roles/{code}` body `{"changes":[{"menu_code":string,"access"?:bool,"delete"?:bool}]}` → view yang sama;
  - `PUT /api/hris/access/accounts/{id}` body `{"role":string}` → view;
  - `GET /api/hris/access/audit` → `{"data":[{id,source,actor_name,role_code,menu_code,actions_before,actions_after,target_name,roles_before,roles_after,created_at}]}`;
  - `/api/users` menerima `hris_role` (string|null); `appAccount.hrisRole` di GET.
- Produces (Go): `iam.HrisAccess = []string{"hris.access"}`; TS `IAM.hrisAccess`.

- [ ] **Step 1: Prefix menu + migrasi**

Di `frontend/src/lib/iam/prefixes.ts`, tepat setelah baris `hrisAnalytics: ["hris.insights.analytics"],`:

```ts
  /** HRIS → Hak Akses HRIS (EPIC-061): AdminHRIS mengatur akses role HR. */
  hrisAccess: ["hris.access"],
```

`backend/database/migrations/deltas/20261022090200_iam_hris_access_menu.sql`:

```sql
-- =============================================================================
-- EPIC-061 (RA-03, Q-RA-5): menu HRIS → Hak Akses HRIS untuk AdminHRIS.
-- Diberikan ke admin_hris (→ hris_manager lewat sync) dan super_admin;
-- disembunyikan dari hris_non_payroll dan hris_non_rekrutmen_kpi lewat masker.
-- =============================================================================
INSERT INTO iam.menus (code, menu_name, route_path, icon, menu_type, order_number, module, level, permission_context)
VALUES ('hris.access', 'Hak Akses HRIS', '/dashboard/hris/hak-akses', 'identification', 'sidebar', 70, 'hris', 2,
        '{"actions":["read","update"]}'::jsonb)
ON CONFLICT (code) DO UPDATE SET
  menu_name = EXCLUDED.menu_name, route_path = EXCLUDED.route_path, icon = EXCLUDED.icon,
  menu_type = EXCLUDED.menu_type, order_number = EXCLUDED.order_number, module = EXCLUDED.module,
  level = EXCLUDED.level, permission_context = EXCLUDED.permission_context,
  is_active = true, is_visible = true, deleted_at = NULL, updated_at = now();

UPDATE iam.menus child SET parent_id = parent.id
FROM iam.menus parent
WHERE child.code = 'hris.access' AND parent.code = 'hris';

INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions)
SELECT r.id, m.id, '["read","update"]'::jsonb
FROM iam.roles r CROSS JOIN iam.menus m
WHERE r.code IN ('admin_hris', 'super_admin') AND m.code = 'hris.access'
ON CONFLICT ON CONSTRAINT role_menu_permissions_unique
DO UPDATE SET granted_actions = EXCLUDED.granted_actions, is_active = true, updated_at = now();

INSERT INTO iam.hris_staff_role_masks (role_code, menu_prefix, exclude_menu) VALUES
  ('hris_non_payroll', 'hris.access', true),
  ('hris_non_rekrutmen_kpi', 'hris.access', true)
ON CONFLICT (role_code, menu_prefix) DO UPDATE SET exclude_menu = true, updated_at = now();

SELECT iam.sync_hris_staff_roles();
```

Run: `cd backend && DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go run ./cmd/migrate -apply && PATH=$HOME/.local/go/bin:$PATH make generate`
Expected:
- `  [ok]    20261022090200_iam_hris_access_menu.sql`;
- `prefixes_gen.go` memuat `HrisAccess = []string{"hris.access"}`.

Run: `psql 'postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' -Atc "SELECT r.code, count(*) FROM iam.role_menu_permissions p JOIN iam.roles r ON r.id = p.role_id WHERE p.is_active AND r.code IN ('admin_hris','hris_manager','hris_non_payroll','hris_non_rekrutmen_kpi') GROUP BY 1 ORDER BY 1"`
Expected: `admin_hris|56`, `hris_manager|56`, `hris_non_payroll|51`, `hris_non_rekrutmen_kpi|39`.

- [ ] **Step 2: Tulis test yang gagal**

`backend/internal/modules/configuration/access/hris_access_integration_test.go`:

```go
package access_test

import (
	"slices"
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

const (
	errRoleScope    = `{"success":false,"error":"Hak Akses HRIS hanya untuk role admin_hris dan hris_*"}`
	errMenuScope    = `{"success":false,"error":"Menu di luar cakupan Hak Akses HRIS"}`
	errAccountScope = `{"success":false,"error":"Akun ini bukan akun HR (peran aplikasi hrd)"}`
)

func change(menu string, access, del *bool) map[string]any {
	c := map[string]any{"menu_code": menu}
	if access != nil {
		c["access"] = *access
	}
	if del != nil {
		c["delete"] = *del
	}
	return map[string]any{"changes": []any{c}}
}

func ptr(b bool) *bool { return &b }

func hrisManager(t *testing.T) testutil.Staff {
	return testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{"hris.access": {"read", "update"}}})
}

func TestHrisAccessScope(t *testing.T) {
	manager := hrisManager(t)
	reader := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{"hris.access": {"read"}}})
	outsider := testutil.CreateStaff(t, testutil.StaffOptions{Menus: map[string][]string{"settings.menus": nil}})
	mux, _ := setup(t)

	expect(t, do(t, mux, nil, "GET", "/api/hris/access", nil), 401, "")
	expect(t, do(t, mux, &outsider, "GET", "/api/hris/access", nil), 403, "")
	expect(t, do(t, mux, &reader, "PUT", "/api/hris/access/roles/hris_non_payroll", change("hris.workforce.attendance", nil, ptr(true))), 403, "")
	for _, code := range []string{"super_admin", "admin", "hrd", "pos"} {
		expect(t, do(t, mux, &manager, "PUT", "/api/hris/access/roles/"+code, change("hris.workforce.attendance", ptr(true), nil)), 403, errRoleScope)
	}
	for _, menu := range []string{"settings.roles", "hris.access", "hris.kepegawaian.users", "pos"} {
		expect(t, do(t, mux, &manager, "PUT", "/api/hris/access/roles/hris_non_payroll", change(menu, ptr(true), nil)), 403, errMenuScope)
	}
	expect(t, do(t, mux, &manager, "PUT", "/api/hris/access/roles/hris_non_payroll", change("hris.kepegawaian.loans", nil, ptr(true))), 403, errMenuScope)
	expect(t, do(t, mux, &manager, "PUT", "/api/hris/access/roles/hris_non_payroll", map[string]any{"changes": []any{}}), 400,
		`{"success":false,"error":"Tidak ada perubahan"}`)

	r := do(t, mux, &manager, "GET", "/api/hris/access", nil)
	expect(t, r, 200, "")
	data := r.Body["data"].(map[string]any)
	var roles []string
	for _, x := range data["roles"].([]any) {
		roles = append(roles, x.(map[string]any)["code"].(string))
	}
	if !slices.Equal(roles, []string{"admin_hris", "hris_manager", "hris_non_payroll", "hris_non_rekrutmen_kpi"}) {
		t.Fatalf("roles = %v", roles)
	}
	menus := map[string]map[string]any{}
	for _, x := range data["menus"].([]any) {
		m := x.(map[string]any)
		menus[m["code"].(string)] = m
	}
	for code, want := range map[string][2]bool{
		"hris.workforce.attendance": {true, true}, "hris.kepegawaian.holidays": {false, true}, "hris.compensation": {true, false},
	} {
		m := menus[code]
		if m == nil || m["access_toggle"] != want[0] || m["delete_toggle"] != want[1] {
			t.Errorf("menu %s = %v, want access %v delete %v", code, m, want[0], want[1])
		}
	}
	for _, code := range []string{"hris.access", "hris.kepegawaian.loans", "settings.roles"} {
		if menus[code] != nil {
			t.Errorf("menu %s must not be listed", code)
		}
	}
}

func TestHrisAccessMasksSurviveSync(t *testing.T) {
	manager := hrisManager(t)
	mux, text := setup(t)
	grant := func(role, menu string) string {
		return text(`SELECT coalesce((SELECT p.granted_actions::text FROM iam.role_menu_permissions p
			JOIN iam.roles r ON r.id = p.role_id JOIN iam.menus m ON m.id = p.menu_id
			WHERE r.code = $1 AND m.code = $2 AND p.is_active), 'none')`, role, menu)
	}
	// Non-Payroll gets delete on Absensi; it survives a later sync.
	expect(t, do(t, mux, &manager, "PUT", "/api/hris/access/roles/hris_non_payroll", change("hris.workforce.attendance", nil, ptr(true))), 200, "")
	text(`SELECT 'ok' FROM (SELECT iam.sync_hris_staff_roles()) s`)
	if got := grant("hris_non_payroll", "hris.workforce.attendance"); !strings.Contains(got, `"delete"`) {
		t.Fatalf("non-payroll attendance = %s, want delete after re-sync", got)
	}
	if got := grant("hris_non_rekrutmen_kpi", "hris.workforce.attendance"); strings.Contains(got, `"delete"`) {
		t.Fatalf("other staff role attendance = %s, want no delete", got)
	}
	// Turning Penggajian off for hris_manager hides the subtree; salary alone comes back with its parent.
	expect(t, do(t, mux, &manager, "PUT", "/api/hris/access/roles/hris_manager", change("hris.compensation", ptr(false), nil)), 200, "")
	if got := grant("hris_manager", "hris.compensation.payroll"); got != "none" {
		t.Fatalf("manager payroll = %s, want none", got)
	}
	expect(t, do(t, mux, &manager, "PUT", "/api/hris/access/roles/hris_manager", change("hris.compensation.salary", ptr(true), nil)), 200, "")
	if got := grant("hris_manager", "hris.compensation"); got != `["read"]` {
		t.Fatalf("manager compensation group = %s, want [\"read\"]", got)
	}
	if got := grant("hris_manager", "hris.compensation.salary"); got == "none" {
		t.Fatal("manager salary not granted back")
	}
	// Delete needs access first.
	expect(t, do(t, mux, &manager, "PUT", "/api/hris/access/roles/hris_non_payroll", change("hris.compensation.salary", nil, ptr(true))), 400,
		`{"success":false,"error":"Aktifkan akses menu dulu sebelum memberi hak hapus"}`)
	// admin_hris (the source): turning its own delete off on Absensi.
	expect(t, do(t, mux, &manager, "PUT", "/api/hris/access/roles/admin_hris", change("hris.workforce.attendance", nil, ptr(false))), 200, "")
	if got := grant("admin_hris", "hris.workforce.attendance"); strings.Contains(got, `"delete"`) {
		t.Fatalf("admin_hris attendance = %s, want no delete", got)
	}
	if got := text(`SELECT count(*)::text FROM iam.permission_audit_logs WHERE source = 'hris_access' AND role_code = 'hris_non_payroll'
		AND menu_code = 'hris.workforce.attendance' AND actions_after ? 'delete'`); got != "1" {
		t.Fatalf("hris_access audit rows = %s", got)
	}
	if got := text(`SELECT count(*)::text FROM iam.permission_audit_logs WHERE source = 'hris_mask' AND role_code = 'hris_manager'`); got == "0" {
		t.Fatal("mask changes not audited")
	}
	r := do(t, mux, &manager, "GET", "/api/hris/access/audit", nil)
	expect(t, r, 200, "")
	if rows, _ := r.Body["data"].([]any); len(rows) == 0 {
		t.Fatal("audit endpoint returned no rows")
	}
}

func TestHrisAccessAccounts(t *testing.T) {
	manager := hrisManager(t)
	hr := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd"})
	boss := testutil.CreateStaff(t, testutil.StaffOptions{Role: "super_admin"})
	fin := testutil.CreateStaff(t, testutil.StaffOptions{Role: "finance_staff"})
	mux, text := setup(t)
	expect(t, do(t, mux, &manager, "PUT", "/api/hris/access/accounts/"+hr.UserID, map[string]any{"role": "hris_non_payroll"}), 200, "")
	if got := text(`SELECT string_agg(r.code, ',') FROM iam.user_roles ur JOIN iam.roles r ON r.id = ur.role_id WHERE ur.user_id = $1`, hr.UserID); got != "hris_non_payroll" {
		t.Fatalf("roles = %s", got)
	}
	for _, s := range []testutil.Staff{boss, fin} {
		expect(t, do(t, mux, &manager, "PUT", "/api/hris/access/accounts/"+s.UserID, map[string]any{"role": "hris_manager"}), 403, errAccountScope)
	}
	expect(t, do(t, mux, &manager, "PUT", "/api/hris/access/accounts/"+hr.UserID, map[string]any{"role": "super_admin"}), 400, errRoleScope)
	if got := text(`SELECT count(*)::text FROM iam.permission_audit_logs WHERE source = 'user_role' AND target_user_id = $1`, hr.UserID); got != "1" {
		t.Fatalf("user role audit rows = %s", got)
	}
}
```

`backend/internal/modules/configuration/accounts/hris_role_integration_test.go`:

```go
package accounts_test

import (
	"context"
	"net/http"
	"testing"

	"nuhabit/backend/internal/app"
	"nuhabit/backend/internal/modules/configuration/accounts"
	"nuhabit/backend/internal/platform/testutil"
)

// setupAs is setup with explicit staff options (super_admin resolves its
// IAM role by app role, so it has no Menus).
func setupAs(t *testing.T, opts testutil.StaffOptions) *env {
	t.Helper()
	staff := testutil.CreateStaff(t, opts)
	tx := testutil.Tx(t)
	deps := testutil.Deps(t, nil)
	mux := http.NewServeMux()
	for _, r := range accounts.Routes(deps, tx, app.ConfigurationPorts(deps).Accounts) {
		mux.Handle(r.Pattern, r.Handler)
	}
	return &env{t: t, staff: staff, tx: tx, mux: mux}
}

func TestHrisRoleInManajemenUser(t *testing.T) {
	admin := testutil.CreateStaff(t, testutil.StaffOptions{Role: "admin", Menus: map[string][]string{"settings.users": nil}})
	e := setupAs(t, testutil.StaffOptions{Role: "super_admin"})
	ctx := context.Background()
	var holding string
	if err := e.tx.QueryRow(ctx, `SELECT id::text FROM configuration.holdings ORDER BY created_at LIMIT 1`).Scan(&holding); err != nil {
		t.Fatal(err)
	}
	scoped := func(m map[string]any) map[string]any {
		m["business_scope"], m["holding_id"] = "holding", holding
		return m
	}
	body := scoped(employeeBody(uniqueEmail()))
	body["is_access_app"], body["password"], body["role"], body["hris_role"] = true, "rahasia123", "hrd", "hris_non_payroll"
	code, created, raw := e.do("POST", "/api/users", body)
	if code != 201 {
		t.Fatalf("create = %d %s", code, raw)
	}
	id := created["data"].(map[string]any)["id"].(string)
	var userID string
	if err := e.tx.QueryRow(ctx, `SELECT user_id::text FROM hris.employees WHERE id = $1`, id).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	roles := func() string {
		var r string
		if err := e.tx.QueryRow(ctx, `SELECT coalesce(string_agg(r.code, ',' ORDER BY r.code), '') FROM iam.user_roles ur JOIN iam.roles r ON r.id = ur.role_id WHERE ur.user_id = $1`, userID).Scan(&r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	if got := roles(); got != "hris_non_payroll" {
		t.Fatalf("after create roles = %s", got)
	}
	_, got, _ := e.do("GET", "/api/users/"+id, nil)
	if acc := got["data"].(map[string]any)["appAccount"].(map[string]any); acc["hrisRole"] != "hris_non_payroll" {
		t.Fatalf("appAccount.hrisRole = %v", acc["hrisRole"])
	}
	// Re-save with the same app role and no hris_role keeps the IAM role.
	if code, _, raw := e.do("PUT", "/api/users/"+id, scoped(map[string]any{"is_access_app": true, "role": "hrd", "phone": "0812"})); code != 200 {
		t.Fatalf("resave = %d %s", code, raw)
	}
	if got := roles(); got != "hris_non_payroll" {
		t.Fatalf("resave changed roles to %s", got)
	}
	// A settings.users holder that is not super_admin may not set it.
	adminEnv := &env{t: t, staff: admin, tx: e.tx, mux: e.mux}
	code, body2, _ := adminEnv.do("PUT", "/api/users/"+id, scoped(map[string]any{"is_access_app": true, "hris_role": "hris_manager"}))
	wantError(t, code, body2, 403, "Role HRIS hanya bisa diatur Super Admin di sini; AdminHRIS memakai halaman Hak Akses HRIS")
	// Only app role hrd.
	code, body2, _ = e.do("PUT", "/api/users/"+id, scoped(map[string]any{"is_access_app": true, "role": "finance_staff", "hris_role": "hris_manager"}))
	wantError(t, code, body2, 400, "Role HRIS hanya untuk akun dengan peran aplikasi hrd")
	// null goes back to the app role.
	if code, _, raw := e.do("PUT", "/api/users/"+id, scoped(map[string]any{"is_access_app": true, "hris_role": nil})); code != 200 {
		t.Fatalf("reset = %d %s", code, raw)
	}
	if got := roles(); got != "hrd" {
		t.Fatalf("reset roles = %s, want hrd", got)
	}
}
```

- [ ] **Step 3: Jalankan, pastikan gagal**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/configuration/access ./internal/modules/configuration/accounts -run 'TestHrisAccess|TestHrisRoleInManajemenUser' -count=1 -v`
Expected: FAIL. Contoh: `got 404 ..., want 401` (route belum ada) dan `after create roles = hrd`.

- [ ] **Step 4: Repo Hak Akses HRIS**

`backend/internal/modules/configuration/access/hris_access_repo.go`:

```go
package access

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/permaudit"
)

type hrisRoleRow struct {
	ID, Code, Name string
	IsSystem       bool
}

type scopeMenu struct {
	ID, Code, Name string
	ParentCode     *string
	Level          int
	Actions        []string
	AccessToggle   bool
	DeleteToggle   bool
}

type hrisAccessRole struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type hrisAccessMenu struct {
	Code         string  `json:"code"`
	Name         string  `json:"name"`
	ParentCode   *string `json:"parent_code"`
	Level        int     `json:"level"`
	AccessToggle bool    `json:"access_toggle"`
	DeleteToggle bool    `json:"delete_toggle"`
}

type hrisAccessAccount struct {
	UserID   string  `json:"user_id"`
	FullName string  `json:"full_name"`
	Email    *string `json:"email"`
	HrisRole *string `json:"hris_role"`
}

type hrisAccessView struct {
	Roles    []hrisAccessRole               `json:"roles"`
	Menus    []hrisAccessMenu               `json:"menus"`
	Grants   map[string]map[string][]string `json:"grants"`
	Accounts []hrisAccessAccount            `json:"accounts"`
}

type permissionAuditRow struct {
	ID            string          `json:"id"`
	Source        string          `json:"source"`
	ActorName     *string         `json:"actor_name"`
	RoleCode      *string         `json:"role_code"`
	MenuCode      *string         `json:"menu_code"`
	ActionsBefore json.RawMessage `json:"actions_before"`
	ActionsAfter  json.RawMessage `json:"actions_after"`
	TargetName    *string         `json:"target_name"`
	RolesBefore   []string        `json:"roles_before"`
	RolesAfter    []string        `json:"roles_after"`
	CreatedAt     httpx.JSTime    `json:"created_at"`
}

// loadHrisRoles lists admin_hris and the hris_* roles in HrisStaffRoleCodes order.
func loadHrisRoles(ctx context.Context, q database.Querier) ([]hrisRoleRow, error) {
	rows, err := q.Query(ctx, `SELECT id::text, code, name, is_system FROM iam.roles WHERE code = ANY($1) AND deleted_at IS NULL`, iam.HrisStaffRoleCodes)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (hrisRoleRow, error) {
		var r hrisRoleRow
		err := row.Scan(&r.ID, &r.Code, &r.Name, &r.IsSystem)
		return r, err
	})
	slices.SortFunc(out, func(a, b hrisRoleRow) int {
		return slices.Index(iam.HrisStaffRoleCodes, a.Code) - slices.Index(iam.HrisStaffRoleCodes, b.Code)
	})
	return out, err
}

// loadScopeMenus lists the HRIS menus AdminHRIS may toggle: access in the
// three scopes (groups included), delete on HRIS leaves offering it.
func loadScopeMenus(ctx context.Context, q database.Querier) ([]scopeMenu, error) {
	rows, err := q.Query(ctx, `SELECT m.id::text, m.code, m.menu_name, p.code, m.level,
		  coalesce(m.permission_context->'actions', '[]'::jsonb),
		  NOT EXISTS (SELECT 1 FROM iam.menus c WHERE c.parent_id = m.id AND c.deleted_at IS NULL)
		FROM iam.menus m LEFT JOIN iam.menus p ON p.id = m.parent_id
		WHERE m.deleted_at IS NULL AND m.code LIKE 'hris.%'
		ORDER BY m.code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []scopeMenu
	for rows.Next() {
		var m scopeMenu
		var raw []byte
		var leaf bool
		if err := rows.Scan(&m.ID, &m.Code, &m.Name, &m.ParentCode, &m.Level, &raw, &leaf); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &m.Actions)
		m.AccessToggle = iam.HasAnyMenuPrefix([]string{m.Code}, iam.HrisAccessScopes)
		m.DeleteToggle = leaf && slices.Contains(m.Actions, "delete")
		if m.AccessToggle || m.DeleteToggle {
			out = append(out, m)
		}
	}
	return out, rows.Err()
}

func loadHrisAccounts(ctx context.Context, q database.Querier) ([]hrisAccessAccount, error) {
	rows, err := q.Query(ctx, `SELECT u.id::text, u.full_name, u.email,
		  (SELECT r.code FROM iam.user_roles ur JOIN iam.roles r ON r.id = ur.role_id
		    WHERE ur.user_id = u.id AND r.code = ANY($1) ORDER BY r.code LIMIT 1)
		FROM configuration.users u
		WHERE u.role = 'hrd' OR EXISTS (SELECT 1 FROM iam.user_roles ur JOIN iam.roles r ON r.id = ur.role_id
		  WHERE ur.user_id = u.id AND r.code = ANY($1))
		ORDER BY u.full_name, u.id`, iam.HrisStaffRoleCodes)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (hrisAccessAccount, error) {
		var a hrisAccessAccount
		err := row.Scan(&a.UserID, &a.FullName, &a.Email, &a.HrisRole)
		return a, err
	})
	if out == nil {
		out = []hrisAccessAccount{}
	}
	return out, err
}

func hrisAccess(ctx context.Context, q database.Querier) (*hrisAccessView, error) {
	roles, err := loadHrisRoles(ctx, q)
	if err != nil {
		return nil, err
	}
	menus, err := loadScopeMenus(ctx, q)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(roles))
	for i, r := range roles {
		ids[i] = r.ID
	}
	snap, err := permaudit.Snapshot(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	accounts, err := loadHrisAccounts(ctx, q)
	if err != nil {
		return nil, err
	}
	v := &hrisAccessView{Roles: []hrisAccessRole{}, Menus: []hrisAccessMenu{}, Grants: map[string]map[string][]string{}, Accounts: accounts}
	inScope := map[string]bool{}
	for _, m := range menus {
		inScope[m.Code] = true
		v.Menus = append(v.Menus, hrisAccessMenu{Code: m.Code, Name: m.Name, ParentCode: m.ParentCode, Level: m.Level,
			AccessToggle: m.AccessToggle, DeleteToggle: m.DeleteToggle})
	}
	for _, r := range roles {
		v.Roles = append(v.Roles, hrisAccessRole{Code: r.Code, Name: r.Name})
		g := map[string][]string{}
		for code, acts := range snap[r.ID] {
			if inScope[code] {
				g[code] = acts
			}
		}
		v.Grants[r.Code] = g
	}
	return v, nil
}

// applyAdminHris changes the source role's own grants (ruling R8).
func applyAdminHris(ctx context.Context, tx pgx.Tx, roleID string, m scopeMenu, c hrisAccessChange, actorID string) error {
	if c.Access != nil && *c.Access {
		acts := slices.DeleteFunc(slices.Clone(m.Actions), func(a string) bool { return a == "delete" })
		raw, _ := json.Marshal(acts)
		if _, err := tx.Exec(ctx, `INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions, is_active, created_by, updated_by, updated_at)
			VALUES ($1, $2, $3::jsonb, true, $4, $4, now())
			ON CONFLICT (role_id, menu_id) DO UPDATE SET
			  granted_actions = CASE WHEN role_menu_permissions.is_active THEN role_menu_permissions.granted_actions ELSE EXCLUDED.granted_actions END,
			  is_active = true, updated_by = EXCLUDED.updated_by, updated_at = now()`, roleID, m.ID, string(raw), actorID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions, is_active, created_by, updated_by, updated_at)
			SELECT $1, p.id, '["read"]'::jsonb, true, $3, $3, now() FROM iam.menus p
			WHERE p.deleted_at IS NULL AND $2 LIKE p.code || '.%'
			ON CONFLICT (role_id, menu_id) DO UPDATE SET granted_actions = EXCLUDED.granted_actions, is_active = true,
			  updated_by = EXCLUDED.updated_by, updated_at = now()
			WHERE NOT role_menu_permissions.is_active`, roleID, m.Code, actorID); err != nil {
			return err
		}
	}
	if c.Access != nil && !*c.Access {
		if _, err := tx.Exec(ctx, `UPDATE iam.role_menu_permissions p SET is_active = false, updated_by = $3, updated_at = now()
			FROM iam.menus m WHERE m.id = p.menu_id AND p.role_id = $1 AND p.is_active
			  AND (m.code = $2 OR m.code LIKE $2 || '.%')`, roleID, m.Code, actorID); err != nil {
			return err
		}
	}
	if c.Delete != nil {
		var active bool
		if err := tx.QueryRow(ctx, `SELECT coalesce((SELECT is_active FROM iam.role_menu_permissions WHERE role_id = $1 AND menu_id = $2), false)`,
			roleID, m.ID).Scan(&active); err != nil {
			return err
		}
		if !active {
			return httpx.BadRequest(errHrisAccessFirst)
		}
		sql := `UPDATE iam.role_menu_permissions SET granted_actions = granted_actions - 'delete', updated_by = $3, updated_at = now()
			WHERE role_id = $1 AND menu_id = $2`
		if *c.Delete {
			sql = `UPDATE iam.role_menu_permissions SET granted_actions = granted_actions || '["delete"]'::jsonb, updated_by = $3, updated_at = now()
			WHERE role_id = $1 AND menu_id = $2 AND NOT granted_actions ? 'delete'`
		}
		if _, err := tx.Exec(ctx, sql, roleID, m.ID, actorID); err != nil {
			return err
		}
	}
	return nil
}

// resolveMask is the mask row deciding code for the role (the longest
// matching prefix), the zero mask when none.
func resolveMask(ctx context.Context, q database.Querier, roleCode, code string) (permaudit.Mask, error) {
	var m permaudit.Mask
	err := q.QueryRow(ctx, `SELECT exclude_menu, excluded_actions FROM iam.hris_staff_role_masks
		WHERE role_code = $1 AND ($2 = menu_prefix OR $2 LIKE menu_prefix || '.%')
		ORDER BY length(menu_prefix) DESC LIMIT 1`, roleCode, code).Scan(&m.ExcludeMenu, &m.ExcludedActions)
	if database.IsNoRows(err) {
		return permaudit.Mask{ExcludedActions: []string{}}, nil
	}
	return m, err
}

func exactMask(ctx context.Context, q database.Querier, roleCode, code string) (*permaudit.Mask, error) {
	var m permaudit.Mask
	err := q.QueryRow(ctx, `SELECT exclude_menu, excluded_actions FROM iam.hris_staff_role_masks WHERE role_code = $1 AND menu_prefix = $2`,
		roleCode, code).Scan(&m.ExcludeMenu, &m.ExcludedActions)
	if database.IsNoRows(err) {
		return nil, nil
	}
	return &m, err
}

// applyMask changes a derived role through its mask (ruling R8).
func applyMask(ctx context.Context, tx pgx.Tx, roleCode string, c hrisAccessChange, actorID string) error {
	cur, err := resolveMask(ctx, tx, roleCode, c.MenuCode)
	if err != nil {
		return err
	}
	exact, err := exactMask(ctx, tx, roleCode, c.MenuCode)
	if err != nil {
		return err
	}
	next := permaudit.Mask{ExcludeMenu: cur.ExcludeMenu, ExcludedActions: slices.Clone(cur.ExcludedActions)}
	if c.Access != nil {
		next.ExcludeMenu = !*c.Access
		if !*c.Access {
			rows, err := tx.Query(ctx, `DELETE FROM iam.hris_staff_role_masks WHERE role_code = $1 AND menu_prefix LIKE $2 || '.%'
				RETURNING menu_prefix, exclude_menu, excluded_actions`, roleCode, c.MenuCode)
			if err != nil {
				return err
			}
			type dropped struct {
				prefix string
				mask   permaudit.Mask
			}
			gone, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (dropped, error) {
				var d dropped
				err := row.Scan(&d.prefix, &d.mask.ExcludeMenu, &d.mask.ExcludedActions)
				return d, err
			})
			if err != nil {
				return err
			}
			for _, d := range gone {
				if err := permaudit.RecordMask(ctx, tx, actorID, roleCode, d.prefix, &d.mask, nil); err != nil {
					return err
				}
			}
		}
	}
	if c.Delete != nil {
		if next.ExcludeMenu {
			return httpx.BadRequest(errHrisAccessFirst)
		}
		next.ExcludedActions = slices.DeleteFunc(next.ExcludedActions, func(a string) bool { return a == "delete" })
		if !*c.Delete {
			next.ExcludedActions = append(next.ExcludedActions, "delete")
		}
	}
	if next.ExcludedActions == nil {
		next.ExcludedActions = []string{}
	}
	slices.Sort(next.ExcludedActions)
	if _, err := tx.Exec(ctx, `INSERT INTO iam.hris_staff_role_masks (role_code, menu_prefix, exclude_menu, excluded_actions, updated_by, updated_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (role_code, menu_prefix) DO UPDATE SET exclude_menu = EXCLUDED.exclude_menu,
		  excluded_actions = EXCLUDED.excluded_actions, updated_by = EXCLUDED.updated_by, updated_at = now()`,
		roleCode, c.MenuCode, next.ExcludeMenu, next.ExcludedActions, actorID); err != nil {
		return err
	}
	return permaudit.RecordMask(ctx, tx, actorID, roleCode, c.MenuCode, exact, &next)
}

func listPermissionAudit(ctx context.Context, q database.Querier, all bool, limit int) ([]permissionAuditRow, error) {
	rows, err := q.Query(ctx, `SELECT l.id::text, l.source, a.full_name, l.role_code, l.menu_code, l.actions_before, l.actions_after,
		  t.full_name, l.roles_before, l.roles_after, l.created_at
		FROM iam.permission_audit_logs l
		LEFT JOIN configuration.users a ON a.id = l.actor_id
		LEFT JOIN configuration.users t ON t.id = l.target_user_id
		WHERE $1 OR l.role_code = ANY($2::text[]) OR l.roles_before && $2::text[] OR l.roles_after && $2::text[]
		ORDER BY l.created_at DESC, l.id
		LIMIT $3`, all, iam.HrisStaffRoleCodes, limit)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (permissionAuditRow, error) {
		var r permissionAuditRow
		var before, after []byte
		var at time.Time
		err := row.Scan(&r.ID, &r.Source, &r.ActorName, &r.RoleCode, &r.MenuCode, &before, &after,
			&r.TargetName, &r.RolesBefore, &r.RolesAfter, &at)
		r.ActionsBefore, r.ActionsAfter, r.CreatedAt = before, after, httpx.JSTime(at)
		return r, err
	})
	if out == nil {
		out = []permissionAuditRow{}
	}
	return out, err
}
```

- [ ] **Step 5: Handler Hak Akses HRIS**

`backend/internal/modules/configuration/access/hris_access.go`:

```go
package access

import (
	"net/http"
	"slices"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/module"
	"nuhabit/backend/internal/platform/permaudit"
)

// "Hak Akses HRIS" (EPIC-061, RA-03 / Q-RA-5/7): AdminHRIS changes, for
// admin_hris and the hris_* roles only, the access to Penggajian,
// Rekrutmen and Kehadiran & Cuti and "delete" on HRIS leaves, and assigns
// those roles to HR accounts. Every change re-runs the HR role sync and is
// recorded in iam.permission_audit_logs.

const (
	errHrisRoleScope    = "Hak Akses HRIS hanya untuk role admin_hris dan hris_*"
	errHrisMenuScope    = "Menu di luar cakupan Hak Akses HRIS"
	errHrisAccessFirst  = "Aktifkan akses menu dulu sebelum memberi hak hapus"
	errHrisAccountScope = "Akun ini bukan akun HR (peran aplikasi hrd)"
	errHrisNoChange     = "Tidak ada perubahan"
)

type hrisAccessChange struct {
	MenuCode string `json:"menu_code"`
	Access   *bool  `json:"access"`
	Delete   *bool  `json:"delete"`
}

func (h handler) hrisAccessRoutes() []module.Route {
	return []module.Route{
		{Pattern: "GET /api/hris/access", Handler: httpx.Handle(h.getHrisAccess)},
		{Pattern: "PUT /api/hris/access/roles/{code}", Handler: httpx.Handle(h.putHrisRoleAccess)},
		{Pattern: "PUT /api/hris/access/accounts/{id}", Handler: httpx.Handle(h.putHrisAccount)},
		{Pattern: "GET /api/hris/access/audit", Handler: httpx.Handle(h.getPermissionAudit)},
	}
}

func (h handler) writeHrisAccess(w http.ResponseWriter, r *http.Request) error {
	v, err := hrisAccess(r.Context(), h.db)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"data": v})
}

func (h handler) getHrisAccess(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.auth.RequireMenuPrefix(r, iam.HrisAccess...); err != nil {
		return err
	}
	return h.writeHrisAccess(w, r)
}

func (h handler) putHrisRoleAccess(w http.ResponseWriter, r *http.Request) error {
	u, err := h.auth.RequireMenuAction(r, "update", iam.HrisAccess...)
	if err != nil {
		return err
	}
	var body struct {
		Changes []hrisAccessChange `json:"changes"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		return err
	}
	if len(body.Changes) == 0 || len(body.Changes) > 200 {
		return httpx.BadRequest(errHrisNoChange)
	}
	ctx := r.Context()
	roles, err := loadHrisRoles(ctx, h.db)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(roles, func(x hrisRoleRow) bool { return x.Code == r.PathValue("code") })
	if i < 0 || roles[i].IsSystem {
		return httpx.Forbidden(errHrisRoleScope)
	}
	role := roles[i]
	menus, err := loadScopeMenus(ctx, h.db)
	if err != nil {
		return err
	}
	byCode := map[string]scopeMenu{}
	for _, m := range menus {
		byCode[m.Code] = m
	}
	for _, c := range body.Changes {
		if c.Access == nil && c.Delete == nil {
			return httpx.BadRequest(errHrisNoChange)
		}
		m, ok := byCode[c.MenuCode]
		if !ok || (c.Access != nil && !m.AccessToggle) || (c.Delete != nil && !m.DeleteToggle) {
			return httpx.Forbidden(errHrisMenuScope)
		}
	}
	err = database.WithTx(ctx, h.db, func(tx pgx.Tx) error {
		watched, err := hrisRoleIDs(ctx, tx)
		if err != nil {
			return err
		}
		before, err := permaudit.Snapshot(ctx, tx, watched)
		if err != nil {
			return err
		}
		for _, c := range body.Changes {
			if role.Code == "admin_hris" {
				err = applyAdminHris(ctx, tx, role.ID, byCode[c.MenuCode], c, u.ID)
			} else {
				err = applyMask(ctx, tx, role.Code, c, u.ID)
			}
			if err != nil {
				return err
			}
		}
		var active bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM iam.role_menu_permissions p JOIN iam.roles r ON r.id = p.role_id
			WHERE r.code = 'admin_hris' AND p.is_active)`).Scan(&active); err != nil {
			return err
		}
		if !active {
			return httpx.BadRequest(errAdminHrisEmpty)
		}
		if _, err := tx.Exec(ctx, `SELECT iam.sync_hris_staff_roles()`); err != nil {
			return err
		}
		after, err := permaudit.Snapshot(ctx, tx, watched)
		if err != nil {
			return err
		}
		_, err = permaudit.Record(ctx, tx, u.ID, permaudit.SourceHrisAccess, before, after)
		return err
	})
	if err != nil {
		return err
	}
	return h.writeHrisAccess(w, r)
}

func (h handler) putHrisAccount(w http.ResponseWriter, r *http.Request) error {
	u, err := h.auth.RequireMenuAction(r, "update", iam.HrisAccess...)
	if err != nil {
		return err
	}
	var body struct {
		Role string `json:"role"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		return err
	}
	if !slices.Contains(iam.HrisStaffRoleCodes, body.Role) {
		return httpx.BadRequest(errHrisRoleScope)
	}
	ctx := r.Context()
	userID := r.PathValue("id")
	err = database.WithTx(ctx, h.db, func(tx pgx.Tx) error {
		var appRole string
		err := tx.QueryRow(ctx, `SELECT role FROM configuration.users WHERE id::text = $1`, userID).Scan(&appRole)
		if database.IsNoRows(err) {
			return httpx.NotFound("Akun tidak ditemukan")
		}
		if err != nil {
			return err
		}
		if appRole != "hrd" {
			return httpx.Forbidden(errHrisAccountScope)
		}
		codes, err := permaudit.UserRoleCodes(ctx, tx, userID)
		if err != nil {
			return err
		}
		for _, c := range codes {
			if c != "hrd" && !slices.Contains(iam.HrisStaffRoleCodes, c) {
				return httpx.Forbidden(errHrisAccountScope)
			}
		}
		return permaudit.SetUserRole(ctx, tx, userID, body.Role, u.ID)
	})
	if err != nil {
		return err
	}
	return h.writeHrisAccess(w, r)
}

func (h handler) getPermissionAudit(w http.ResponseWriter, r *http.Request) error {
	u, err := h.auth.RequireMenuPrefix(r, append(append([]string{}, iam.HrisAccess...), iam.SettingsRoles...)...)
	if err != nil {
		return err
	}
	all, err := h.auth.HasMenuPrefix(r.Context(), u, iam.SettingsRoles...)
	if err != nil {
		return err
	}
	rows, err := listPermissionAudit(r.Context(), h.db, all, 200)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"data": rows})
}
```

Di `routes.go`, fungsi `Routes` mengembalikan `append([]module.Route{ ...route lama... }, h.hrisAccessRoutes()...)`.

- [ ] **Step 6: Role HRIS di Manajemen User**

`backend/internal/modules/configuration/accounts/input.go`:
- `employeeSchemaKeys`: tambahkan `"hris_role"` di akhir. `accountKeys` ikut otomatis, sehingga `touchesAccount` menolaknya bagi penulis tanpa `settings.users`.
- `employeeBody`: tambahkan field `hrisRole *string`.
- `parseEmployeeBody`: setelah `b.role = f.Enum("role", optional, domain.AdminUserRoles)`, tambahkan:

```go
	b.hrisRole = f.Enum("hris_role", optionalNull, iam.HrisStaffRoleCodes)
```

(import `"nuhabit/backend/internal/platform/iam"` bila belum).

`backend/internal/modules/configuration/accounts/routes.go`:
- tambahkan helper berikut;
- di `createUser` dan `updateUser`, setelah blok `if !accounts && b.touchesAccount(...) { ... }`, sisipkan pemanggilannya.

```go
// errHrisRoleSuperAdmin: in Manajemen User only super_admin sets the HRIS
// role; AdminHRIS uses HRIS → Hak Akses HRIS (EPIC-061, ruling R9).
var errHrisRoleSuperAdmin = httpx.Forbidden("Role HRIS hanya bisa diatur Super Admin di sini; AdminHRIS memakai halaman Hak Akses HRIS")

func (h handlers) requireHrisRoleSetter(r *http.Request, b *employeeBody) error {
	if !b.has("hris_role") {
		return nil
	}
	u, err := h.d.Auth.RequireUser(r)
	if err != nil {
		return err
	}
	ok, err := h.d.Auth.HasRoleCode(r.Context(), u, "super_admin")
	if err != nil {
		return err
	}
	if !ok {
		return errHrisRoleSuperAdmin
	}
	return nil
}
```

```go
	if err := h.requireHrisRoleSetter(r, b); err != nil {
		return err
	}
```

`backend/internal/modules/configuration/accounts/employees.go`:
- tambahkan dua fungsi berikut;
- di jalur pembuatan akun, tepat setelah blok `if err := syncIamRole(ctx, tx, userID, role, actorID); err != nil { return err }` (±baris 434), sisipkan pemanggilan pertama. Jalur ini transaksional, jadi error membatalkan semuanya.
- di `syncAccount`, tepat setelah baris `prevRole := profileColumn(ctx, s.db, userID, "role")`, sisipkan pemeriksaan awal. `syncAccount` tidak transaksional (port TS), jadi penolakan harus terjadi sebelum apa pun ditulis.
- di `syncAccount`, setelah blok `if b.role != nil && (prevRole == nil || *prevRole != *b.role) { ... syncIamRole ... }`, sisipkan pemanggilan kedua.

```go
const errHrisRoleNeedsHrd = "Role HRIS hanya untuk akun dengan peran aplikasi hrd"

// checkHrisRole refuses an HRIS role for an account whose app role is not
// hrd (EPIC-061, Q-RA-7). null (back to the app role) is always allowed.
func checkHrisRole(b *employeeBody, prevRole *string) error {
	if !b.has("hris_role") || b.hrisRole == nil {
		return nil
	}
	app := roleOf(b.role)
	if app == "" && prevRole != nil {
		app = *prevRole
	}
	if app != "hrd" {
		return httpx.BadRequest(errHrisRoleNeedsHrd)
	}
	return nil
}

// applyHrisRole sets the IAM role of an HR account: the HRIS role, or the
// app role again when hris_role is null.
func applyHrisRole(ctx context.Context, q database.Querier, userID string, b *employeeBody, prevRole *string, actorID string) error {
	if err := checkHrisRole(b, prevRole); err != nil {
		return err
	}
	code := roleOf(b.role)
	if code == "" && prevRole != nil {
		code = *prevRole
	}
	if b.hrisRole != nil {
		code = *b.hrisRole
	}
	return syncIamRole(ctx, q, userID, code, actorID)
}
```

Pemanggilan pertama (jalur pembuatan akun):

```go
		if b.has("hris_role") {
			if err := applyHrisRole(ctx, tx, userID, b, &role, actorID); err != nil {
				return err
			}
		}
```

Pemeriksaan awal di `syncAccount`:

```go
	if err := checkHrisRole(b, prevRole); err != nil {
		return err
	}
```

Pemanggilan kedua (di `syncAccount`, setelah sinkron peran aplikasi):

```go
	if b.has("hris_role") {
		if err := applyHrisRole(ctx, s.db, userID, b, prevRole, actorID); err != nil {
			return err
		}
	}
```

`backend/internal/modules/configuration/accounts/store.go`:
- `appUser` dapat field `HrisRole *string`;
- di `loadAppUser`, query menjadi kode di bawah;
- isian struct ditambah `HrisRole: row.StrPtr("hris_role")`.

```go
	row, err := kit.QueryOne(ctx, q, `SELECT "id", "role", "status", "brand_id", "business_scope", "holding_id",
       "company_id", "branch_id", "can_switch_stall", "can_central_checkout", "default_warehouse_id",
       (SELECT r.code FROM iam.user_roles ur JOIN iam.roles r ON r.id = ur.role_id
         WHERE ur.user_id = "users"."id" AND r.code = ANY($2::text[]) ORDER BY r.code LIMIT 1) AS "hris_role", `+permissionsEmbed+`
  FROM configuration.users WHERE "id" = $1`, userID, iam.HrisStaffRoleCodes)
```

`backend/internal/modules/configuration/accounts/view.go`:
- `appAccountView` dapat field terakhir (setelah `Warehouses`) `HrisRole *string \`json:"hrisRole"\``;
- `mapEmployee` mengisi `HrisRole: app.HrisRole`.

- [ ] **Step 7: Regenerate manifest route & jalankan test**

Run: `cd backend && PATH=$HOME/.local/go/bin:$PATH make generate && git diff --stat ../frontend/src/lib/go-routes.generated.json`
Expected: manifest bertambah 4 route `/api/hris/access...` (module `configuration`).

Run: perintah Step 3, lalu `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/configuration/... ./internal/platform/auth -count=1 2>&1 | tail -20`
Expected:
- `TestHrisAccessScope`, `TestHrisAccessMasksSurviveSync`, `TestHrisAccessAccounts`, `TestHrisRoleInManajemenUser` PASS;
- test lama di paket itu (termasuk `TestHrisStaffRolesDeriveFromAdminHris` dengan `hris.access`) PASS;
- tidak ada kegagalan baru.

- [ ] **Step 8: Commit**

```bash
git add backend/database/migrations/deltas/20261022090200_iam_hris_access_menu.sql backend/internal/platform/iam/prefixes_gen.go frontend/src/lib/iam/prefixes.ts frontend/src/lib/go-routes.generated.json backend/internal/modules/configuration
git commit -m "feat(iam): API Hak Akses HRIS untuk AdminHRIS (cakupan terbatas, lewat masker) dan Role HRIS di Manajemen User, EPIC-061

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 9: Halaman "Hak Akses HRIS" + field Role HRIS di Manajemen User

**Files:**
- Create: `frontend/src/features/hris/access/{index.ts,types.ts,api.ts,query-keys.ts}`
- Create: `frontend/src/features/hris/access/components/hris-access-page.tsx`, test `hris-access-page.test.tsx`
- Create: `frontend/src/app/dashboard/(dashboard)/hris/hak-akses/page.tsx`
- Modify: `frontend/src/lib/hris/users-form.ts`, `frontend/src/lib/hris/users-form.test.ts`
- Modify: `frontend/src/lib/users/user-mapper.ts`
- Modify: `frontend/src/features/users/constants/index.ts`, `frontend/src/features/users/components/app-access-form-section.tsx`
- Modify: `docs/epics/EPIC-061-hris-role-hak-akses.md` (Automation Log)

**Interfaces:**
- Consumes: endpoint Task 8; `IAM.hrisAccess` (Task 8).
- Produces:
  - `HrisAccessPage`;
  - `fetchHrisAccess()`, `saveHrisRoleAccess(role, changes)`, `saveHrisAccount(userId, role)`, `fetchPermissionAudit()`;
  - field form `hris_role`, `hris_role_loaded`;
  - `HRIS_ROLE_OPTIONS`.

- [ ] **Step 1: Tulis test yang gagal**

`frontend/src/features/hris/access/components/hris-access-page.test.tsx`:

```tsx
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { HrisAccessPage } from "./hris-access-page";

const api = vi.hoisted(() => {
  const view = {
    roles: [
      { code: "hris_manager", name: "HR Manager" },
      { code: "hris_non_payroll", name: "HR Staff (Non-Payroll)" },
    ],
    menus: [
      { code: "hris.workforce.attendance", name: "Absensi", parent_code: "hris.workforce", level: 3, access_toggle: true, delete_toggle: true },
    ],
    grants: {
      hris_manager: { "hris.workforce.attendance": ["delete", "read"] },
      hris_non_payroll: { "hris.workforce.attendance": ["read"] },
    },
    accounts: [{ user_id: "u1", full_name: "Rina HR", email: "rina@paddy.test", hris_role: "hris_non_payroll" }],
  };
  return {
    fetchHrisAccess: vi.fn(async () => view),
    fetchPermissionAudit: vi.fn(async () => []),
    saveHrisRoleAccess: vi.fn(async () => view),
    saveHrisAccount: vi.fn(async () => view),
  };
});
vi.mock("../api", () => api);
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
afterEach(cleanup);

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <HrisAccessPage />
    </QueryClientProvider>
  );
}

it("memberi hak hapus Absensi ke Non-Payroll", async () => {
  renderPage();
  const box = (await screen.findByLabelText("HR Staff (Non-Payroll) — hapus Absensi")) as HTMLInputElement;
  expect(box.checked).toBe(false);
  expect((screen.getByLabelText("HR Manager — hapus Absensi") as HTMLInputElement).checked).toBe(true);
  fireEvent.click(box);
  await waitFor(() =>
    expect(api.saveHrisRoleAccess).toHaveBeenCalledWith("hris_non_payroll", [{ menu_code: "hris.workforce.attendance", delete: true }])
  );
});

it("mengganti role HRIS sebuah akun", async () => {
  renderPage();
  fireEvent.change(await screen.findByLabelText("Role HRIS Rina HR"), { target: { value: "hris_manager" } });
  await waitFor(() => expect(api.saveHrisAccount).toHaveBeenCalledWith("u1", "hris_manager"));
});
```

Tambahkan di `frontend/src/lib/hris/users-form.test.ts`, di dalam `describe("buildUserPayload", ...)`:

```ts
  it("role HRIS dikirim hanya bila berubah dan peran aplikasi hrd", () => {
    const loaded = withAccess({ role: "hrd", hris_role: "hris_non_payroll", hris_role_loaded: "hris_non_payroll" });
    expect(buildUserPayload(loaded, true)).not.toHaveProperty("hris_role");
    expect(buildUserPayload({ ...loaded, hris_role: "hris_manager" }, true)).toHaveProperty("hris_role", "hris_manager");
    expect(buildUserPayload({ ...loaded, hris_role: "" }, true)).toHaveProperty("hris_role", null);
    expect(buildUserPayload({ ...loaded, role: "admin", hris_role: "hris_manager" }, true)).not.toHaveProperty("hris_role");
  });
```

- [ ] **Step 2: Jalankan, pastikan gagal**

Run: `cd frontend && npx vitest run src/features/hris/access src/lib/hris/users-form.test.ts`
Expected: FAIL. Contoh: `Failed to resolve import "./hris-access-page"` dan `expected { ... } to have property "hris_role"`.

- [ ] **Step 3: API, tipe, query key**

`frontend/src/features/hris/access/types.ts`:

```ts
export interface HrisAccessRole {
  code: string;
  name: string;
}

export interface HrisAccessMenu {
  code: string;
  name: string;
  parent_code: string | null;
  level: number;
  access_toggle: boolean;
  delete_toggle: boolean;
}

export interface HrisAccessAccount {
  user_id: string;
  full_name: string;
  email: string | null;
  hris_role: string | null;
}

export interface HrisAccessView {
  roles: HrisAccessRole[];
  menus: HrisAccessMenu[];
  grants: Record<string, Record<string, string[]>>;
  accounts: HrisAccessAccount[];
}

export interface HrisAccessChange {
  menu_code: string;
  access?: boolean;
  delete?: boolean;
}

export interface PermissionAuditRow {
  id: string;
  source: "role_permissions" | "hris_access" | "hris_mask" | "user_role";
  actor_name: string | null;
  role_code: string | null;
  menu_code: string | null;
  actions_before: unknown;
  actions_after: unknown;
  target_name: string | null;
  roles_before: string[] | null;
  roles_after: string[] | null;
  created_at: string;
}
```

`frontend/src/features/hris/access/api.ts`:

```ts
import { apiGet, apiPut } from "@/lib/api-client";
import type { HrisAccessChange, HrisAccessView, PermissionAuditRow } from "./types";

export const fetchHrisAccess = () => apiGet<{ data: HrisAccessView }>("/api/hris/access").then((res) => res.data);

export const saveHrisRoleAccess = (role: string, changes: HrisAccessChange[]) =>
  apiPut<{ data: HrisAccessView }>(`/api/hris/access/roles/${role}`, { changes }).then((res) => res.data);

export const saveHrisAccount = (userId: string, role: string) =>
  apiPut<{ data: HrisAccessView }>(`/api/hris/access/accounts/${userId}`, { role }).then((res) => res.data);

export const fetchPermissionAudit = () =>
  apiGet<{ data: PermissionAuditRow[] }>("/api/hris/access/audit").then((res) => res.data);
```

`frontend/src/features/hris/access/query-keys.ts`:

```ts
export const hrisAccessQueryKeys = {
  view: () => ["hris", "access"] as const,
  audit: () => ["hris", "access", "audit"] as const,
};
```

`frontend/src/features/hris/access/index.ts`:

```ts
export { HrisAccessPage } from "./components/hris-access-page";
```

- [ ] **Step 4: Komponen halaman**

`frontend/src/features/hris/access/components/hris-access-page.tsx`:

```tsx
"use client";

import { useEffect } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { PageHeader } from "@/components/ui/page-header";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { fetchHrisAccess, fetchPermissionAudit, saveHrisAccount, saveHrisRoleAccess } from "../api";
import { hrisAccessQueryKeys } from "../query-keys";
import type { HrisAccessChange, HrisAccessMenu, HrisAccessRole, HrisAccessView, PermissionAuditRow } from "../types";

const SOURCE_LABEL: Record<PermissionAuditRow["source"], string> = {
  role_permissions: "Role Management",
  hris_access: "Hak Akses HRIS",
  hris_mask: "Masker role HRIS",
  user_role: "Penugasan role",
};

function actionsText(value: unknown): string {
  if (value == null) return "tidak ada";
  if (Array.isArray(value)) return value.join(", ") || "-";
  return JSON.stringify(value);
}

function auditText(row: PermissionAuditRow): string {
  if (row.source === "user_role") {
    return `${row.target_name ?? "Akun"}: ${(row.roles_before ?? []).join(", ") || "-"} → ${(row.roles_after ?? []).join(", ") || "-"}`;
  }
  return `${row.role_code ?? "-"} · ${row.menu_code ?? "-"}: ${actionsText(row.actions_before)} → ${actionsText(row.actions_after)}`;
}

function AccessCell({
  role,
  menu,
  actions,
  disabled,
  onChange,
}: {
  role: HrisAccessRole;
  menu: HrisAccessMenu;
  actions: string[] | undefined;
  disabled: boolean;
  onChange: (change: HrisAccessChange) => void;
}) {
  const has = actions !== undefined;
  return (
    <div className="flex flex-col gap-1 text-xs text-gray-700">
      {menu.access_toggle ? (
        <label className="inline-flex items-center gap-1.5">
          <input
            type="checkbox"
            checked={has}
            disabled={disabled}
            aria-label={`${role.name} — akses ${menu.name}`}
            onChange={(e) => onChange({ menu_code: menu.code, access: e.target.checked })}
          />
          Akses
        </label>
      ) : null}
      {menu.delete_toggle ? (
        <label className="inline-flex items-center gap-1.5">
          <input
            type="checkbox"
            checked={actions?.includes("delete") ?? false}
            disabled={disabled || !has}
            aria-label={`${role.name} — hapus ${menu.name}`}
            onChange={(e) => onChange({ menu_code: menu.code, delete: e.target.checked })}
          />
          Hapus
        </label>
      ) : null}
    </div>
  );
}

/**
 * HRIS → Hak Akses HRIS (EPIC-061, RA-03): AdminHRIS mengatur akses
 * Penggajian, Rekrutmen, Kehadiran & Cuti dan hak hapus untuk role HR, serta
 * role HRIS tiap akun HR. Role lain dan menu non-HRIS diatur Super Admin.
 */
export function HrisAccessPage() {
  const qc = useQueryClient();
  const view = useQuery({ queryKey: hrisAccessQueryKeys.view(), queryFn: fetchHrisAccess });
  const audit = useQuery({ queryKey: hrisAccessQueryKeys.audit(), queryFn: fetchPermissionAudit });
  const saved = (data: HrisAccessView) => {
    qc.setQueryData(hrisAccessQueryKeys.view(), data);
    void qc.invalidateQueries({ queryKey: hrisAccessQueryKeys.audit() });
    toast.success("Hak akses disimpan");
  };
  const roleMutation = useMutation({
    mutationFn: ({ role, change }: { role: string; change: HrisAccessChange }) => saveHrisRoleAccess(role, [change]),
    retry: false,
    onSuccess: saved,
    onError: (err) => toast.error(err.message || "Gagal menyimpan hak akses"),
  });
  const accountMutation = useMutation({
    mutationFn: ({ userId, role }: { userId: string; role: string }) => saveHrisAccount(userId, role),
    retry: false,
    onSuccess: saved,
    onError: (err) => toast.error(err.message || "Gagal menyimpan role akun"),
  });

  useEffect(() => {
    if (view.isError) toast.error("Gagal memuat Hak Akses HRIS");
  }, [view.isError]);

  const data = view.data;
  const busy = roleMutation.isPending || accountMutation.isPending;

  return (
    <div className="space-y-6">
      <PageHeader
        title="Hak Akses HRIS"
        description="Atur akses Penggajian, Rekrutmen, Kehadiran & Cuti, dan hak hapus untuk role HR. Role lain diatur Super Admin di Role Management."
      />
      {!data ? (
        <p className="text-sm text-muted-foreground">{view.isLoading ? "Memuat..." : "Data tidak tersedia"}</p>
      ) : (
        <>
          <section className="rounded-lg border border-gray-200/70 bg-white">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Menu</TableHead>
                  {data.roles.map((role) => (
                    <TableHead key={role.code}>{role.name}</TableHead>
                  ))}
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.menus.map((menu) => (
                  <TableRow key={menu.code}>
                    <TableCell className={menu.level > 2 ? "pl-6" : "font-medium"}>
                      {menu.name}
                      <span className="block text-xs text-gray-400">{menu.code}</span>
                    </TableCell>
                    {data.roles.map((role) => (
                      <TableCell key={role.code}>
                        <AccessCell
                          role={role}
                          menu={menu}
                          actions={data.grants[role.code]?.[menu.code]}
                          disabled={busy}
                          onChange={(change) => roleMutation.mutate({ role: role.code, change })}
                        />
                      </TableCell>
                    ))}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </section>

          <section className="space-y-2">
            <h2 className="text-base font-semibold text-gray-900">Akun HR</h2>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Nama</TableHead>
                  <TableHead>Email</TableHead>
                  <TableHead>Role HRIS</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.accounts.map((account) => (
                  <TableRow key={account.user_id}>
                    <TableCell>{account.full_name}</TableCell>
                    <TableCell>{account.email ?? "-"}</TableCell>
                    <TableCell>
                      <select
                        aria-label={`Role HRIS ${account.full_name}`}
                        className="h-9 rounded-lg border border-gray-200 px-2 text-sm"
                        value={account.hris_role ?? ""}
                        disabled={busy}
                        onChange={(e) => {
                          if (e.target.value) accountMutation.mutate({ userId: account.user_id, role: e.target.value });
                        }}
                      >
                        <option value="" disabled>
                          Belum ada role HRIS
                        </option>
                        {data.roles.map((role) => (
                          <option key={role.code} value={role.code}>
                            {role.name}
                          </option>
                        ))}
                      </select>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </section>
        </>
      )}

      <section className="space-y-2">
        <h2 className="text-base font-semibold text-gray-900">Riwayat perubahan</h2>
        {(audit.data ?? []).length === 0 ? (
          <p className="text-sm text-muted-foreground">Belum ada perubahan tercatat.</p>
        ) : (
          <ul className="divide-y divide-gray-100 rounded-lg border border-gray-200/70 bg-white text-sm">
            {(audit.data ?? []).map((row) => (
              <li key={row.id} className="px-4 py-2">
                <span className="text-gray-500">{new Date(row.created_at).toLocaleString("id-ID")}</span>
                {" · "}
                <span className="font-medium">{row.actor_name ?? "Sistem"}</span>
                {" · "}
                {SOURCE_LABEL[row.source]}
                <span className="block text-gray-700">{auditText(row)}</span>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
```

`frontend/src/app/dashboard/(dashboard)/hris/hak-akses/page.tsx` (pola sama dengan halaman HRIS lain; Server Component, tanpa `params`):

```tsx
import { requireIamPage } from "@/lib/auth/require-user";
import { IAM } from "@/lib/iam/prefixes";
import { HrisAccessPage } from "@/features/hris/access";

export default async function Page() {
  await requireIamPage(IAM.hrisAccess, "/dashboard/me");
  return <HrisAccessPage />;
}
```

- [ ] **Step 5: Field "Role HRIS" di Manajemen User**

`frontend/src/lib/hris/users-form.ts`:
- `UserEmployeeFormValues`: setelah `role: UserRole;`, tambahkan:

```ts
  /** Role IAM HRIS (EPIC-061); "" = ikuti peran aplikasi. */
  hris_role: string;
  /** Nilai hris_role saat dimuat: dikirim hanya bila berubah. */
  hris_role_loaded: string;
```

- `emptyUserForm`: `hris_role: "", hris_role_loaded: "",`.
- `userFormFromItem`: `hris_role: account?.hrisRole ?? "", hris_role_loaded: account?.hrisRole ?? "",`.
- `APP_ACCESS_FIELDS`: tambahkan `"hris_role"`, `"hris_role_loaded"`.
- `buildUserPayload`: sebelum `// edit: password kosong = tidak diganti`, tambahkan:

```ts
  // Role HRIS hanya untuk peran hrd dan hanya bila diubah: simpan ulang tidak menimpa role IAM.
  if (form.role === "hrd" && form.hris_role !== form.hris_role_loaded) {
    payload.hris_role = form.hris_role || null;
  }
```

`frontend/src/lib/users/user-mapper.ts`:
- di tipe `app_user` (dekat `default_warehouse_id?: string | null;`), tambahkan `hris_role?: string | null;`;
- di objek `appAccount`, setelah `defaultWarehouseId`, tambahkan `hrisRole: row.app_user.hris_role ?? null,`.

`frontend/src/features/users/constants/index.ts`:

```ts
/** Role IAM HRIS untuk akun peran hrd (EPIC-061). "auto" = ikuti peran aplikasi. */
export const HRIS_ROLE_OPTIONS = [
  { value: "auto", label: "Ikuti peran aplikasi (hrd)" },
  { value: "admin_hris", label: "Admin HRIS" },
  { value: "hris_manager", label: "HR Manager" },
  { value: "hris_non_payroll", label: "HR Staff (Non-Payroll)" },
  { value: "hris_non_rekrutmen_kpi", label: "HR Staff (Non-Rekrutmen & KPI)" },
];
```

`frontend/src/features/users/components/app-access-form-section.tsx`:
- import `HRIS_ROLE_OPTIONS` dari `"../constants"`;
- tepat setelah `<div>` yang memuat Combobox **Role**, tambahkan:

```tsx
            {form.role === "hrd" ? (
              <div>
                <FormFieldLabel>Role HRIS</FormFieldLabel>
                <Combobox
                  options={HRIS_ROLE_OPTIONS}
                  value={form.hris_role || "auto"}
                  onChange={(value) => onChange({ hris_role: value === "auto" ? "" : value })}
                  placeholder="Pilih role HRIS"
                  searchPlaceholder="Cari role..."
                  emptyMessage="Role tidak ditemukan"
                  className={formComboboxClassName}
                />
                <p className="mt-1 text-xs text-gray-500">
                  Hanya Super Admin yang bisa mengubah di sini; AdminHRIS memakai HRIS → Hak Akses HRIS.
                </p>
              </div>
            ) : null}
```

- [ ] **Step 6: Jalankan, pastikan lolos**

Run: `cd frontend && npx vitest run src/features/hris/access src/lib/hris src/lib/users src/features/users && npx tsc --noEmit -p . && npx eslint src/features/hris/access "src/app/dashboard/(dashboard)/hris/hak-akses" src/lib/hris src/lib/users src/features/users`
Expected: vitest semua lolos (2 test halaman + test users-form baru); `tsc` dan `eslint` bersih.

- [ ] **Step 7: Verifikasi menyeluruh**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./... 2>&1 | grep -- '--- FAIL' | sort > /tmp/epic061-after.txt; diff /tmp/epic061-baseline.txt /tmp/epic061-after.txt && echo SAMA`
Expected: `SAMA` (kegagalan Go = baseline).

Run: `cd frontend && npx vitest run 2>&1 | tail -5`
Expected: semua test lolos (jumlah ≥ jumlah sebelum EPIC-061).

- [ ] **Step 8: Catat di epic**

`## Automation Log` adalah bagian terakhir `docs/epics/EPIC-061-hris-role-hak-akses.md`, jadi entri ditambahkan di akhir file (tanggal = hari eksekusi):

Run:
```bash
printf -- '- %s: Task 1–9 selesai di branch `docs/epic061-role-hak-akses` (DB test saja). Hak hapus HRIS per menu daun + alasan + audit (`hrisdel.Guard`: 24 route DELETE + impor jadwal, ubah tipe jadwal, jadwal tetap, impor karyawan), masker role HRIS (`iam.hris_staff_role_masks`) bertahan dari sync, halaman Hak Akses HRIS (`hris.access`), Role HRIS di Manajemen User (super_admin), audit perubahan hak akses (`iam.permission_audit_logs`), role sistem hanya super_admin. Grant DB test: admin_hris 56, hris_manager 56, hris_non_payroll 51, hris_non_rekrutmen_kpi 39. Full suite Go = baseline. Live belum disentuh; cek R3 sebelum rollout.\n' "$(date +%F)" >> docs/epics/EPIC-061-hris-role-hak-akses.md && tail -1 docs/epics/EPIC-061-hris-role-hak-akses.md
```
Expected: baris terakhir file adalah entri baru dengan tanggal hari ini.

- [ ] **Step 9: Commit**

```bash
git add frontend/src docs/epics/EPIC-061-hris-role-hak-akses.md
git commit -m "feat(hris): halaman Hak Akses HRIS dan field Role HRIS di Manajemen User, EPIC-061

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

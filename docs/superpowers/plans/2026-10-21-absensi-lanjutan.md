# Paket Absensi Lanjutan — Implementation Plan (EPIC-065)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Also load the project skill `paddy-hris-acuan` before starting.

**Goal:** Karyawan mengajukan absen manual / koreksi absen dari ESS (dengan keterangan dan foto bukti opsional), melihat absen yang terlewat di periode terbuka, dan HR memutus di satu antrean yang menulis absensi lewat jalur penyesuaian HR yang sama; baris absensi di cabang dengan jam kerja minimum (HQ = 9 jam) berlabel "Kurang jam / pulang lebih cepat".

**Architecture:**
- Aturan murni di Go `backend/internal/modules/hris/domain/attendance_request.go`: validasi pengajuan (`CheckAttendanceRequest`, jam WIB, lewat tengah malam via `ClockOutAt`), pemindaian absen terlewat (`AttendanceGaps`, jadwal via `ResolveScheduleRow`), validasi jam minimum, dan teks notifikasi.
- Tabel `hris.attendance_requests` (satu pending per karyawan per tanggal lewat unique index parsial). `attendance_requests.go` memuat pengajuan karyawan, antrean HR, dan keputusan. Persetujuan memanggil `createManualAttendance` / `adjustAttendance` (fungsi di balik `CreateManualAttendance` / `AdjustAttendance`, ditambah kait `also` yang berjalan di transaksi yang sama) sehingga alasan, riwayat penyesuaian, audit, dan kunci periode tidak diduplikasi.
- Notifikasi memakai kanal in-app yang sudah ada: `public.notify_hrd` (HR, sama dengan "Jam Kerja Kurang") dan `public.notify_user` (karyawan). Badge menu `/dashboard/hris/attendance` di `nav-badges` (Go + fallback Next).
- `attendance_gaps.go`: `GET /api/hris/me/attendance-gaps` atas jendela periode terbuka (`openAttendanceWindow` memakai `periodContaining`).
- AB-05: kolom `configuration.branches.min_work_hours`, snapshot `hris.attendance.min_work_hours` + `is_short_hours` yang diisi trigger DB (berlaku untuk semua jalur tulis, termasuk fallback Next), notifikasi clock-out mengikuti label, dan `PUT /api/hris/branches/{id}/min-work-hours` yang melabel ulang baris di jendela periode terbuka.
- Frontend: fitur baru `frontend/src/features/hris/attendance-requests` (lapisan data, komponen ESS, tab HR), lib murni `lib/hris/attendance-requests.ts` dan `lib/hris/short-hours.ts`, badge `ShortHoursBadge`, field jam minimum di dialog Lokasi Toko.

**Tech Stack:** Go (pgx, `audit.Write`, `database.WithTx`), migrasi PostgreSQL 16 (plpgsql trigger), Next.js 16.3.8 (Client Component dalam halaman yang sudah ada, React Query), vitest + Testing Library.

**Spec:** `docs/epics/EPIC-065-absensi-lanjutan.md` (Goal, Acceptance Criteria, Catatan) dan acuan `docs/hris/acuan-mom-20261019/` — `status-requirement.md` baris AB-04, AB-05, AB-06, LA-01; `pertanyaan-rekomendasi.md` K-9, Q-AB-1, urutan eksekusi langkah 6 "Paket Absensi"; `matriks-approval.md` baris "Absen Manual / Koreksi Absen".

**Prasyarat:**
- Branch kerja `feat/epic065-absensi-lanjutan` (dari `main` `556a7f6`, berisi EPIC-058..064 yang live sejak 21-10-2026). Tetap di branch ini.
- K-9 dan Q-AB-1 sudah diputuskan user (20-10-2026). Ruling R1–R11 di bawah adalah turunan yang diputuskan mengikuti rekomendasi; tidak ada pertanyaan terbuka.
- Bila memakai worktree tanpa `node_modules`: `ln -s /home/wit/Desktop/paddydev/frontend/node_modules frontend/node_modules` (tidak di-commit).

## Ruling

- **R1 — K-9: alert absen terlewat digabung ke pengajuan (AB-06 → AB-04).** Bentuk terkecil yang berguna: kartu "Absen terlewat" di ESS → Absensi berisi hari milik karyawan di jendela periode terbuka (sampai kemarin) yang (a) punya absen masuk tanpa absen pulang, atau (b) hari kerja terjadwal (shift di jadwal yang berlaku) tanpa baris absensi — kecuali libur nasional aktif, cuti disetujui, baris "Tidak hadir" dari HR, dan tanggal yang sudah punya pengajuan menunggu. Tiap hari punya tombol "Ajukan koreksi" yang membuka form bertanggal. Tanpa jadwal yang berlaku hanya (a) yang diketahui. HR **tidak** mendapat alert/badge roster baru: notifikasi per pengajuan + badge menu Absensi + tab antrean sudah memberi tahu HR, dan roster Monitoring sudah menandai "Belum Absen"/"Tidak Hadir". Alasan: K-9 meminta karyawan mengajukan dan HR menerima notifikasi; alert proaktif ke HR untuk setiap hari bolong hanya menambah kebisingan sebelum karyawan sempat menjelaskan. Cost if wrong: menambah ringkasan harian untuk HR adalah pekerjaan terpisah tanpa mengubah skema.
- **R2 — Pemutus = HR saja.** Matriks: "Absen Manual / Koreksi Absen | HR". Atasan langsung **bukan** pemutus dan tidak diberi notifikasi. HR = `Actor.IsHRD` (`super_admin`/`admin`/`hrd`, himpunan yang sama dengan `canManageAttendance` yang menjaga penyesuaian langsung) **dan** menu `hris.workforce.attendance` (aksi `read`, sudah dipegang semua role HR live). Tidak ada aksi menu baru (penyesuaian langsung hari ini juga tidak memakai aksi menu), jadi tidak ada migrasi grant, tidak ada menu IAM baru, dan tidak perlu `iam.sync_hris_staff_roles()`. `hiring_manager` → 403. Tidak ada yang memutus pengajuannya sendiri (403).
- **R3 — Satu jalur tulis.** Setujui memanggil `createManualAttendance` (tanggal tanpa baris **pada saat keputusan**) atau `adjustAttendance` (baris ada), yaitu fungsi yang sama dengan `POST /api/hris/attendance/manual` dan `PUT /api/hris/attendance/{id}`, dengan kait `also` yang menulis status pengajuan, audit `attendance_request.approve`, dan notifikasi karyawan di transaksi yang sama. Alasan penyesuaian = `Pengajuan karyawan: <keterangan>`. Jenis yang tersimpan di pengajuan hanya informasi; jalur ditentukan ulang saat keputusan, sehingga karyawan yang sempat absen setelah mengajukan absen manual tidak membuat persetujuan gagal 409.
- **R4 — Tanggal & periode.** Tanggal ≤ hari ini (WIB), periode yang memuatnya belum di-closing (`ensureAttendanceOpen` saat pengajuan **dan** saat persetujuan), jam yang diajukan tidak di masa depan (jam pulang ≤ jam masuk = hari berikutnya, `domain.ClockOutAt`). Satu pengajuan `pending` per karyawan per tanggal (cek + unique index parsial). Pemohon boleh membatalkan selama `pending`.
- **R5 — Isi pengajuan.** `clock_in`/`clock_out` "HH:MM" WIB, minimal satu; absen manual wajib jam masuk; koreksi hanya mengubah jam yang diajukan (jam lain tetap), dan koreksi yang sama dengan catatan (per menit) ditolak. Keterangan wajib ≥ 5 karakter (setara alasan penyesuaian HR). Foto bukti opsional, JPG/PNG/WebP ≤ 5 MB, disimpan privat di `attendance-requests/<employeeId>/` dan dibaca pemilik + HR (`ownedFile`). Baris "Tidak hadir" yang dikoreksi menjadi `present`; baris baru dari pengajuan bercatatan `Absen manual dari pengajuan karyawan`.
- **R6 — Notifikasi.** HR: `public.notify_hrd(title, message, 'approval', '/dashboard/hris/attendance')` di transaksi pengajuan (kanal in-app yang sama dengan "Jam Kerja Kurang"; penerima = akun `configuration.users.role = 'hrd'`). Karyawan: `public.notify_user(user_id, …, 'status_change', '/dashboard/me/absensi')` saat disetujui/ditolak. Tidak ada WA (tidak ada nomor tujuan HR di sistem dan K-9 hanya meminta notifikasi).
- **R7 — Penanda HQ = jam kerja minimum per cabang (menyempurnakan Q-AB-1).** Kolom `configuration.branches.min_work_hours numeric(4,2)` (NULL = tanpa aturan, 1–24, kelipatan 0,25). HQ = 9, diisi HR lewat dialog Lokasi Toko setelah rilis (langkah data, keputusan user; migrasi tidak mengisi). Efeknya sama dengan flag `is_hq` + ambang 9 tanpa angka 9 di kode, dan cabang lain bisa diberi aturan kelak tanpa rilis. Audit `branch_min_hours.update`.
- **R8 — Ukuran & penyimpanan label.** Kurang jam = `clock_out − clock_in` (istirahat **termasuk**) < `min_work_hours` cabang **baris absensi** (`branch_id`). Alasannya jam kantor HQ 09.00–18.00 = 9 jam termasuk istirahat 1 jam, sedangkan `work_hours` mengurangi istirahat (09.00–18.00 = 8 jam dan akan salah berlabel). Tepat 9 jam tidak berlabel. Label disimpan di baris (`is_short_hours` + snapshot `min_work_hours`) oleh trigger `hris.attendance_short_hours` setiap kali jam masuk/pulang/cabang berubah, sehingga clock-out Go, fallback Next, penyesuaian HR, dan persetujuan pengajuan semuanya konsisten. Mengubah jam minimum cabang melabel ulang baris cabang itu sejak awal jendela periode terbuka (`openAttendanceWindow` → `periodContaining`); periode yang sudah di-closing tidak berubah. Baris tanpa `branch_id` tidak berlabel. Tanpa efek payroll.
- **R9 — Notifikasi lama < 7 jam.** Disesuaikan (bukan dipertahankan): notifikasi HR "⚠️ Jam Kerja Kurang" saat clock-out kini dikirim bila baris berlabel kurang jam, dengan durasi dan jam minimum di pesan. Cabang tanpa jam minimum tidak lagi memicu notifikasi. Alasan: MOM menetapkan satu aturan (< 9 jam, HQ saja); dua ambang berbeda membingungkan. Cost if wrong: HR mengisi jam minimum toko (mis. 8) dan toko itu kembali mendapat notifikasi beserta label.
  - **Catatan implementasi (ruling C1/P1, 21-10-2026): R9 dibatalkan.** Notifikasi "Jam Kerja Kurang" (< 7 jam, semua cabang) tidak diubah dan `fn_notify_attendance_changes` tidak disentuh migrasi `20261025090100`; label kurang jam tidak mengirim notifikasi baru. Kode SQL/test R9 di bawah adalah rancangan awal yang tidak dipakai.
- **R10 — Paritas fallback Next.** Endpoint baru hanya di Go (`goNativePrefixes`): `/api/hris/attendance-requests`, `/api/hris/me/attendance-requests`, `/api/hris/me/attendance-gaps`; `/api/hris/branches/{id}/min-work-hours` sudah tercakup prefix `/api/hris/branches`. Route Next yang melayani path yang sama dan berubah perilakunya: `/api/hris/nav-badges` → badge pengajuan absensi untuk `isHrd` (paritas). Label kurang jam ditulis trigger DB, jadi fallback Next clock-out ikut berlabel; route fallback `GET /api/hris/attendance` dan daily-roster tidak diubah, dan field label opsional di tipe TS (pola EPIC-062 untuk geofence).
- **R11 — Audit (LA-01).** `attendance_request.approve|reject|cancel` lewat `auditDecision` (entity `hris.attendance_requests`), penulisan absensi lewat `logAdjustment` (`attendance.create|update`, alasan), jam minimum lewat `audit.Write` (`branch_min_hours.update`, sebelum/sesudah). Pembuatan pengajuan tidak diaudit (sama dengan cuti/lembur), karena belum mengubah data absensi.

## Global Constraints

- **DB hanya DB test:** `TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable'`. **Jangan** port 55432 (juga default `backend/Makefile`), jangan DB live, jangan container docker lain. Migrasi ke DB test: `cd backend && MIGRATE_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go run ./cmd/migrate -apply`.
- **Go:** `$HOME/.local/go/bin/go`; `gofmt -l backend/internal` bersih sebelum commit. `make generate` dijalankan dengan `PATH=$HOME/.local/go/bin:$PATH`.
- **Route Go baru:** setiap task yang menambah route menjalankan `cd backend && PATH=$HOME/.local/go/bin:$PATH make generate`, meng-commit `frontend/src/lib/go-routes.generated.json`, memastikan path baru tercakup `goNativePrefixes` di `frontend/src/lib/backend-routes.test.ts`, lalu `cd frontend && npx vitest run src/lib/backend-routes.test.ts` PASS.
- **Nama migrasi:** `backend/database/migrations/deltas/20261025090000_attendance_requests.sql` (Task 2) dan `20261025090100_short_hours.sql` (Task 5), keduanya setelah `20261024090000_approval_atasan.sql` (delta terakhir di `main`). Tidak ada menu IAM baru, jadi `backend/database/seeders/iam-menus.sql` tidak berubah dan tidak ada migrasi yang memanggil `iam.sync_hris_staff_roles()`.
- **Test butuh IAM seeded:** CI memigrasi tanpa seeder. Test di rencana ini membuat menu/grant sendiri lewat `h.staff` (`testutil.CreateStaff` membuat menu yang belum ada); bila menambah test yang bergantung pada role/menu seeded, panggil `testutil.RequireSeededIAM` dulu.
- **Baseline:** sebelum Task 1, simpan test Go yang sudah gagal: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test -p 1 ./... -count=1 2>&1 | grep -- '--- FAIL' | sed 's/ (.*//' | sort -u > /tmp/epic065-baseline.txt`. Kegagalan yang sama di akhir bukan regresi.
- **UUID dari request:** id di path dan `employee_id` di query dikanonikkan dengan `validate.CanonicalUUID` (huruf kecil, tervalidasi) sebelum lookup atau perbandingan pemilik/diri sendiri.
- **Kunci periode:** hanya lewat `ensureAttendanceOpen` / `periodContaining` (dan error `periodWriteError` dari guard DB). Jangan menulis query status `closed` sendiri.
- **Next.js:** baca `frontend/AGENTS.md`. Tidak ada halaman baru: komponen baru adalah Client Component (`"use client"`) di halaman yang sudah ada (`node_modules/next/dist/docs/01-app/01-getting-started/05-server-and-client-components.md`). Komponen klien **tidak boleh** mengimpor modul server (`@/lib/db`, `@/lib/audit`, `*-repo.ts`, `server-only`). Cek: `grep -rnE 'from "@/lib/(db|audit)"|-repo"|server-only' frontend/src/features/hris/attendance-requests frontend/src/lib/hris/attendance-requests.ts frontend/src/lib/hris/short-hours.ts frontend/src/features/hris/attendance/components/short-hours-badge.tsx` → tanpa output.
- **Pesan (persis, test mengunci string):**
  - Pengajuan: `Akun ini tidak tertaut ke data karyawan` · `Tanggal wajib diisi` · `Format tanggal tidak valid` · `Tanggal pengajuan tidak boleh di masa depan` · `Keterangan wajib diisi (minimal 5 karakter)` · `Format jam harus HH:MM` · `Isi jam masuk dan/atau jam pulang yang diajukan` · `Jam masuk wajib diisi` · `Jam yang diajukan belum terjadi` · `Jam yang diajukan sama dengan absensi tercatat` · `Masih ada pengajuan absensi yang menunggu untuk tanggal ini` · `Lampiran harus berupa gambar JPG/PNG/WebP` · `Ukuran lampiran maksimal 5 MB` (400); sukses `Pengajuan absensi terkirim — menunggu persetujuan HR` (201).
  - Batal/putus: `ID pengajuan tidak valid` (400) · `Pengajuan absensi tidak ditemukan` (404) · `Hanya pemohon yang bisa membatalkan pengajuan ini` (403) · `Pengajuan sudah disetujui|ditolak|dibatalkan` (400) · `Hanya HRD yang bisa memproses pengajuan absensi` (403) · `Tidak bisa memutuskan pengajuan absensi sendiri` (403) · `Alasan penolakan wajib diisi` (400) · `Pengajuan sudah diproses oleh orang lain` (409, `decisionRace` yang sudah ada); sukses `Pengajuan absensi dibatalkan` · `Pengajuan absensi disetujui — absensi diperbarui` · `Pengajuan absensi ditolak`. Periode di-closing: pesan 409 `closedPeriodError` yang sudah ada.
  - Jam minimum: `Jam kerja minimum 1–24 jam (kelipatan 0,25)` · `ID toko tidak valid` (400) · `Toko tidak ditemukan` (404); sukses `Jam kerja minimum disimpan`.
  - Notifikasi: HR `Pengajuan Absensi: <nama>` / `<nama> mengajukan absen manual|koreksi absen <tgl> (masuk HH:MM, pulang HH:MM). Tinjau di HRIS → Absensi → Pengajuan Absensi.`; karyawan `Pengajuan absensi disetujui` / `Pengajuan absensi ditolak`; clock-out `⚠️ Jam Kerja Kurang: <nama>` / `<nama> hadir J jam M menit, di bawah jam minimum N jam (kurang jam / pulang lebih cepat).`
  - Label UI: `Kurang jam / pulang lebih cepat` · `Kurang jam` (sel kalender) · `Absen terlewat` · `Ajukan koreksi` · `Tidak ada absen` · `Belum absen pulang` · `Ajukan absen manual / koreksi` · `Absen manual` · `Koreksi absen` · tab `Pengajuan Absensi` · `Jam kerja minimum (jam)` · `Simpan jam minimum`.
- **Aturan user:** yang sudah benar tidak diubah; jangan refaktor di luar file yang disebut tiap task. Setting dan data live adalah keputusan user: rencana ini tidak mengubah data live dan tidak menyentuh payroll.
- **Commit:** satu per task, pesan diakhiri baris kosong lalu `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Tanpa push.

## Review Focus

1. **Karyawan sempat absen setelah mengajukan absen manual** (internet kembali hidup, lalu clock-in jam 09.15): HR menyetujui → baris yang ada dikoreksi ke jam pengajuan, tidak 409 "sudah ada" dan tidak ada baris ganda. Diuji di Task 3 (`TestAttendanceRequestDecision`, kasus "absen setelah mengajukan").
2. **Periode di-closing di antara pengajuan dan keputusan:** setujui → 409 pesan periode, pengajuan tetap `pending`, tolak tetap bisa. Diuji di Task 3 (kasus "periode di-closing").
3. **Jam lewat tengah malam / jam yang belum terjadi:** masuk 22.00 kemarin + pulang 06.00 diterima pukul 10.00; pulang 11.00 (besok) ditolak `Jam yang diajukan belum terjadi`; jam masuk tepat menit berjalan diterima. Diuji di Task 1 (`TestCheckAttendanceRequest`).
4. **UUID huruf besar di path/query:** batal, putus, filter `employee_id`, dan jam minimum cabang dengan id huruf besar tetap bekerja (dan audit menyimpan id kanonik). Diuji di Task 2 (batal), Task 3 (putus + filter), Task 5 (cabang).
5. **Antrean basi / persetujuan ganda:** HR kedua menekan Setujui pada pengajuan yang sudah disetujui → 400 `Pengajuan sudah disetujui` tanpa menulis ulang absensi (riwayat penyesuaian tetap satu). Diuji di Task 3 (kasus "antrean basi"); balapan murni dijaga `SetAttendanceRequestStatus` (`errNoRow` → 409, transaksi absensi ikut batal).

## Peta file

| File | Tanggung jawab |
|---|---|
| `backend/internal/modules/hris/domain/attendance_request.go` (baru) | `CheckAttendanceRequest`, `AttendanceGaps`, `ValidateMinWorkHours`, teks notifikasi |
| `backend/database/migrations/deltas/20261025090000_attendance_requests.sql` (baru) | tabel `hris.attendance_requests` |
| `backend/internal/modules/hris/attendance_requests.go` (baru) | pengajuan karyawan, antrean & keputusan HR (repo, service, SQL, HTTP) |
| `backend/internal/modules/hris/attendance_adjust.go` | `createManualAttendance` / `adjustAttendance` dengan kait transaksi |
| `backend/internal/modules/hris/attendance_gaps.go` (baru) | `openAttendanceWindow`, absen terlewat |
| `backend/database/migrations/deltas/20261025090100_short_hours.sql` (baru) | jam minimum cabang, label + trigger, notifikasi clock-out |
| `backend/internal/modules/hris/branch_work_hours.go` (baru) | simpan jam minimum + label ulang |
| `frontend/src/lib/hris/attendance-requests.ts`, `short-hours.ts` (baru) | label, validasi, payload (murni, aman untuk klien) |
| `frontend/src/features/hris/attendance-requests/**` (baru) | lapisan data, komponen ESS, tab HR |
| `frontend/src/features/hris/attendance/components/short-hours-badge.tsx` (baru) | badge label kurang jam |

---

### Task 1: Aturan pengajuan, absen terlewat, dan jam minimum (domain Go murni)

**Files:**
- Create: `backend/internal/modules/hris/domain/attendance_request.go`
- Test: `backend/internal/modules/hris/domain/attendance_request_test.go`

**Interfaces:**
- Consumes: `ParseDate`, `TodayWIB`, `JSTrim`, `EachDateISO` (`domain/calendar.go`); `ValidClock`, `ClockAt`, `ClockOutAt`, `ClockWIBText`, `ShortDateID` (`domain/attendance_period.go`); `ScheduleRow`, `ResolveScheduleRow` (`domain/schedule.go`).
- Produces:
  - `const AttendanceRequestManual = "manual"`, `AttendanceRequestCorrection = "correction"`, `GapMissing = "missing"`, `GapNoClockOut = "no_clock_out"`
  - `type AttendanceRequestInput struct { Date string; ClockIn, ClockOut *string; Reason string; HasRow bool; RecordedIn, RecordedOut *time.Time }`
  - `func CheckAttendanceRequest(in AttendanceRequestInput, now time.Time) (kind, msg string)`
  - `type RecordedDay struct { ClockIn, ClockOut *time.Time; Absent bool }`
  - `type GapInput struct { Start, End string; Schedule []ScheduleRow; Holidays, OnLeave map[string]bool; Recorded map[string]RecordedDay; Pending map[string]bool }`
  - `type AttendanceGap struct { Date string \`json:"date"\`; Kind string \`json:"kind"\`; ClockIn *string \`json:"clock_in"\` }`
  - `func AttendanceGaps(in GapInput) []AttendanceGap` (tidak pernah nil)
  - `func ValidateMinWorkHours(hours *float64) string`
  - `func AttendanceRequestNotice(employeeName, kind, date string, clockIn, clockOut *string) (title, message string)`
  - `func AttendanceDecisionNotice(status, date string, rejection *string) (title, message string)`

- [ ] **Step 1: Simpan baseline Go** (lihat Global Constraints) ke `/tmp/epic065-baseline.txt`.

- [ ] **Step 2: Tulis test yang gagal**

`backend/internal/modules/hris/domain/attendance_request_test.go`:

```go
package domain

import (
	"testing"
	"time"
)

// requestNow is Monday 2026-10-05 10:00 WIB.
var requestNow = time.Date(2026, 10, 5, 3, 0, 0, 0, time.UTC)

func instant(s string) *time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return &t
}

func TestCheckAttendanceRequest(t *testing.T) {
	const why = "Internet toko mati"
	cases := []struct {
		name      string
		in        AttendanceRequestInput
		kind, msg string
	}{
		{"absen manual lengkap", AttendanceRequestInput{Date: "2026-10-02", ClockIn: sp("08:00"), ClockOut: sp("17:00"), Reason: why},
			AttendanceRequestManual, ""},
		{"tanggal tidak valid", AttendanceRequestInput{Date: "2026-13-40", ClockIn: sp("08:00"), Reason: why}, "", "Format tanggal tidak valid"},
		{"masa depan", AttendanceRequestInput{Date: "2026-10-06", ClockIn: sp("08:00"), Reason: why}, "",
			"Tanggal pengajuan tidak boleh di masa depan"},
		{"keterangan pendek", AttendanceRequestInput{Date: "2026-10-02", ClockIn: sp("08:00"), Reason: "  Lupa "}, "",
			"Keterangan wajib diisi (minimal 5 karakter)"},
		{"jam rusak", AttendanceRequestInput{Date: "2026-10-02", ClockIn: sp("8:00"), Reason: why}, "", "Format jam harus HH:MM"},
		{"tanpa jam", AttendanceRequestInput{Date: "2026-10-02", Reason: why}, "", "Isi jam masuk dan/atau jam pulang yang diajukan"},
		{"absen manual tanpa jam masuk", AttendanceRequestInput{Date: "2026-10-02", ClockOut: sp("17:00"), Reason: why}, "",
			"Jam masuk wajib diisi"},
		{"jam masuk belum terjadi", AttendanceRequestInput{Date: "2026-10-05", ClockIn: sp("10:01"), Reason: why}, "",
			"Jam yang diajukan belum terjadi"},
		{"menit berjalan diterima", AttendanceRequestInput{Date: "2026-10-05", ClockIn: sp("10:00"), Reason: why},
			AttendanceRequestManual, ""},
		{"lewat tengah malam, pulang sudah lewat", AttendanceRequestInput{Date: "2026-10-04", ClockIn: sp("22:00"), ClockOut: sp("06:00"), Reason: why},
			AttendanceRequestManual, ""},
		{"lewat tengah malam, pulang belum terjadi", AttendanceRequestInput{Date: "2026-10-04", ClockIn: sp("22:00"), ClockOut: sp("11:00"), Reason: why},
			"", "Jam yang diajukan belum terjadi"},
		{"koreksi jam pulang", AttendanceRequestInput{Date: "2026-10-03", ClockOut: sp("17:00"), Reason: why, HasRow: true,
			RecordedIn: instant("2026-10-03T01:02:37Z")}, AttendanceRequestCorrection, ""},
		{"koreksi baris tanpa jam masuk", AttendanceRequestInput{Date: "2026-10-03", ClockOut: sp("17:00"), Reason: why, HasRow: true},
			"", "Jam masuk wajib diisi"},
		{"koreksi baris tidak hadir dengan jam lengkap", AttendanceRequestInput{Date: "2026-10-03", ClockIn: sp("08:00"), ClockOut: sp("17:00"),
			Reason: why, HasRow: true}, AttendanceRequestCorrection, ""},
		{"koreksi sama dengan catatan (detik diabaikan)", AttendanceRequestInput{Date: "2026-10-03", ClockIn: sp("08:02"), Reason: why,
			HasRow: true, RecordedIn: instant("2026-10-03T01:02:37Z"), RecordedOut: instant("2026-10-03T10:00:00Z")},
			"", "Jam yang diajukan sama dengan absensi tercatat"},
	}
	for _, c := range cases {
		kind, msg := CheckAttendanceRequest(c.in, requestNow)
		if kind != c.kind || msg != c.msg {
			t.Errorf("%s: got (%q, %q), want (%q, %q)", c.name, kind, msg, c.kind, c.msg)
		}
	}
}

func TestAttendanceGaps(t *testing.T) {
	shift := "shift-pagi"
	var schedule []ScheduleRow
	for dow := 1; dow <= 6; dow++ {
		schedule = append(schedule, ScheduleRow{DayOfWeek: dow, ShiftID: &shift, EffectiveFrom: "2026-01-01"})
	}
	schedule = append(schedule, ScheduleRow{DayOfWeek: 7, EffectiveFrom: "2026-01-01"}) // Minggu libur
	in := GapInput{
		Start: "2026-09-28", End: "2026-10-04", Schedule: schedule,
		Holidays: map[string]bool{"2026-10-01": true},
		OnLeave:  map[string]bool{"2026-09-29": true},
		Recorded: map[string]RecordedDay{
			"2026-09-30": {ClockIn: instant("2026-09-30T01:05:00Z")},
			"2026-10-02": {ClockIn: instant("2026-10-02T01:00:00Z"), ClockOut: instant("2026-10-02T10:00:00Z")},
			"2026-10-03": {Absent: true},
		},
		Pending: map[string]bool{"2026-09-28": true},
	}
	// 28 Sep pending, 29 Sep cuti, 30 Sep belum pulang, 1 Okt libur, 2 Okt lengkap, 3 Okt "Tidak hadir" dari HR, 4 Okt Minggu.
	got := AttendanceGaps(in)
	if len(got) != 1 || got[0].Date != "2026-09-30" || got[0].Kind != GapNoClockOut || got[0].ClockIn == nil || *got[0].ClockIn != "08:05" {
		t.Fatalf("gaps = %+v", got)
	}
	delete(in.Pending, "2026-09-28")
	in.OnLeave = map[string]bool{}
	got = AttendanceGaps(in)
	if len(got) != 3 || got[0].Date != "2026-09-28" || got[0].Kind != GapMissing || got[0].ClockIn != nil ||
		got[1].Date != "2026-09-29" || got[2].Kind != GapNoClockOut {
		t.Fatalf("gaps without pending/leave = %+v", got)
	}
	// Tanpa jadwal: hanya absen pulang yang hilang yang diketahui.
	in.Schedule = nil
	if got = AttendanceGaps(in); len(got) != 1 || got[0].Date != "2026-09-30" {
		t.Fatalf("gaps without schedule = %+v", got)
	}
	if got = AttendanceGaps(GapInput{Start: "2026-10-05", End: "2026-10-04"}); got == nil || len(got) != 0 {
		t.Fatalf("empty window = %#v", got)
	}
}

func TestValidateMinWorkHours(t *testing.T) {
	const bad = "Jam kerja minimum 1–24 jam (kelipatan 0,25)"
	f := func(v float64) *float64 { return &v }
	for _, c := range []struct {
		in   *float64
		want string
	}{{nil, ""}, {f(9), ""}, {f(8.25), ""}, {f(1), ""}, {f(24), ""}, {f(0.5), bad}, {f(24.25), bad}, {f(9.1), bad}} {
		if got := ValidateMinWorkHours(c.in); got != c.want {
			t.Errorf("ValidateMinWorkHours(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestAttendanceNotices(t *testing.T) {
	title, msg := AttendanceRequestNotice("Budi", AttendanceRequestManual, "2026-10-02", sp("08:00"), sp("17:00"))
	if title != "Pengajuan Absensi: Budi" ||
		msg != "Budi mengajukan absen manual 2 Okt 2026 (masuk 08:00, pulang 17:00). Tinjau di HRIS → Absensi → Pengajuan Absensi." {
		t.Fatalf("request notice = %q / %q", title, msg)
	}
	if _, msg = AttendanceRequestNotice("Budi", AttendanceRequestCorrection, "2026-10-03", nil, sp("17:00")); msg !=
		"Budi mengajukan koreksi absen 3 Okt 2026 (pulang 17:00). Tinjau di HRIS → Absensi → Pengajuan Absensi." {
		t.Fatalf("correction notice = %q", msg)
	}
	if title, msg = AttendanceDecisionNotice("approved", "2026-10-02", nil); title != "Pengajuan absensi disetujui" ||
		msg != "Pengajuan absensi 2 Okt 2026 disetujui HR. Absensi sudah diperbarui." {
		t.Fatalf("approved notice = %q / %q", title, msg)
	}
	if title, msg = AttendanceDecisionNotice("rejected", "2026-10-02", sp("CCTV tidak sesuai")); title != "Pengajuan absensi ditolak" ||
		msg != "Pengajuan absensi 2 Okt 2026 ditolak HR. Alasan: CCTV tidak sesuai" {
		t.Fatalf("rejected notice = %q / %q", title, msg)
	}
}
```

- [ ] **Step 3: Jalankan test, pastikan gagal**

Run: `cd backend && $HOME/.local/go/bin/go test ./internal/modules/hris/domain/ -run 'TestCheckAttendanceRequest|TestAttendanceGaps|TestValidateMinWorkHours|TestAttendanceNotices' -count=1`
Expected: FAIL — `undefined: AttendanceRequestInput` (dan simbol lain).

- [ ] **Step 4: Implementasi minimal**

`backend/internal/modules/hris/domain/attendance_request.go`:

```go
package domain

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Paket Absensi lanjutan (EPIC-065, MOM AB-04/AB-05/AB-06, K-9): pengajuan
// absen manual / koreksi absen yang diputus HR, absen terlewat di periode
// absensi terbuka, dan jam kerja minimum per cabang.

// Request kinds: manual = no attendance row on the date, correction = a row
// exists. Gap kinds: missing = a scheduled working day without a row,
// no_clock_out = a row with a clock-in and no clock-out.
const (
	AttendanceRequestManual     = "manual"
	AttendanceRequestCorrection = "correction"
	GapMissing                  = "missing"
	GapNoClockOut               = "no_clock_out"
)

// AttendanceRequestInput is a request as the employee entered it, with the
// attendance row recorded on its date (HasRow false: none).
type AttendanceRequestInput struct {
	Date              string
	ClockIn, ClockOut *string // "HH:MM" WIB; nil = not requested
	Reason            string
	HasRow            bool
	RecordedIn        *time.Time
	RecordedOut       *time.Time
}

// CheckAttendanceRequest returns the request kind, or the 400 message when
// the request is invalid (EPIC-065 R4/R5). now is the server clock.
func CheckAttendanceRequest(in AttendanceRequestInput, now time.Time) (kind, msg string) {
	if _, ok := ParseDate(in.Date); !ok {
		return "", "Format tanggal tidak valid"
	}
	if in.Date > TodayWIB(now) {
		return "", "Tanggal pengajuan tidak boleh di masa depan"
	}
	if len([]rune(JSTrim(in.Reason))) < 5 {
		return "", "Keterangan wajib diisi (minimal 5 karakter)"
	}
	for _, c := range []*string{in.ClockIn, in.ClockOut} {
		if c != nil && !ValidClock(*c) {
			return "", "Format jam harus HH:MM"
		}
	}
	if in.ClockIn == nil && in.ClockOut == nil {
		return "", "Isi jam masuk dan/atau jam pulang yang diajukan"
	}
	clockIn := in.RecordedIn
	if in.ClockIn != nil {
		t, _ := ClockAt(in.Date, *in.ClockIn)
		clockIn = &t
	}
	if clockIn == nil {
		return "", "Jam masuk wajib diisi"
	}
	clockOut := in.RecordedOut
	if in.ClockOut != nil {
		t, _ := ClockOutAt(in.Date, *in.ClockOut, clockIn)
		clockOut = &t
	}
	if clockIn.After(now) || (clockOut != nil && clockOut.After(now)) {
		return "", "Jam yang diajukan belum terjadi"
	}
	if !in.HasRow {
		return AttendanceRequestManual, ""
	}
	if sameMinute(clockIn, in.RecordedIn) && sameMinute(clockOut, in.RecordedOut) {
		return "", "Jam yang diajukan sama dengan absensi tercatat"
	}
	return AttendanceRequestCorrection, ""
}

// sameMinute compares two optional instants to the minute (a request
// carries "HH:MM", a clock-in carries seconds).
func sameMinute(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Truncate(time.Minute).Equal(b.Truncate(time.Minute))
}

// RecordedDay is the attendance row of a date in the gap scan.
type RecordedDay struct {
	ClockIn, ClockOut *time.Time
	// Absent: HR marked the day "Tidak hadir"; not a gap.
	Absent bool
}

// GapInput is what AttendanceGaps scans, [Start, End] inclusive.
type GapInput struct {
	Start, End string
	Schedule   []ScheduleRow
	Holidays   map[string]bool
	OnLeave    map[string]bool
	Recorded   map[string]RecordedDay
	Pending    map[string]bool
}

// AttendanceGap is a day the employee is invited to request a correction for.
type AttendanceGap struct {
	Date string `json:"date"`
	Kind string `json:"kind"`
	// ClockIn is the recorded clock-in, "HH:MM" WIB (no_clock_out only).
	ClockIn *string `json:"clock_in"`
}

// AttendanceGaps lists, oldest first, the days of the window without a
// complete attendance (MOM AB-06, EPIC-065 R1): a row with no clock-out, or
// a working day of the schedule in force without a row — unless the date is
// an active public holiday, an approved leave day, a day HR marked absent,
// or already has a pending request. Without a schedule only missing
// clock-outs are known.
func AttendanceGaps(in GapInput) []AttendanceGap {
	out := []AttendanceGap{}
	for _, date := range EachDateISO(in.Start, in.End) {
		if in.Pending[date] {
			continue
		}
		if rec, ok := in.Recorded[date]; ok {
			if !rec.Absent && rec.ClockIn != nil && rec.ClockOut == nil {
				clock := ClockWIBText(*rec.ClockIn)
				out = append(out, AttendanceGap{Date: date, Kind: GapNoClockOut, ClockIn: &clock})
			}
			continue
		}
		if in.Holidays[date] || in.OnLeave[date] {
			continue
		}
		if i := ResolveScheduleRow(in.Schedule, date); i >= 0 && in.Schedule[i].ShiftID != nil && *in.Schedule[i].ShiftID != "" {
			out = append(out, AttendanceGap{Date: date, Kind: GapMissing})
		}
	}
	return out
}

// ValidateMinWorkHours: nil (no rule) or 1–24 hours in quarter hours
// (EPIC-065 R7); "" when valid.
func ValidateMinWorkHours(hours *float64) string {
	if hours == nil {
		return ""
	}
	if h := *hours; h < 1 || h > 24 || math.Mod(h*4, 1) != 0 {
		return "Jam kerja minimum 1–24 jam (kelipatan 0,25)"
	}
	return ""
}

// AttendanceRequestNotice is HR's in-app notification of a new request (K-9).
func AttendanceRequestNotice(employeeName, kind, date string, clockIn, clockOut *string) (title, message string) {
	what := "absen manual"
	if kind == AttendanceRequestCorrection {
		what = "koreksi absen"
	}
	var clocks []string
	if clockIn != nil {
		clocks = append(clocks, "masuk "+*clockIn)
	}
	if clockOut != nil {
		clocks = append(clocks, "pulang "+*clockOut)
	}
	return "Pengajuan Absensi: " + employeeName, fmt.Sprintf(
		"%s mengajukan %s %s (%s). Tinjau di HRIS → Absensi → Pengajuan Absensi.",
		employeeName, what, ShortDateID(date), strings.Join(clocks, ", "))
}

// AttendanceDecisionNotice is the employee's in-app notification of HR's
// decision.
func AttendanceDecisionNotice(status, date string, rejection *string) (title, message string) {
	if status == "approved" {
		return "Pengajuan absensi disetujui", "Pengajuan absensi " + ShortDateID(date) + " disetujui HR. Absensi sudah diperbarui."
	}
	message = "Pengajuan absensi " + ShortDateID(date) + " ditolak HR."
	if rejection != nil && *rejection != "" {
		message += " Alasan: " + *rejection
	}
	return "Pengajuan absensi ditolak", message
}
```

- [ ] **Step 5: Jalankan test, pastikan lulus**

Run: `cd backend && $HOME/.local/go/bin/go test ./internal/modules/hris/domain/ -count=1 && gofmt -l internal/modules/hris/domain`
Expected: `ok  nuhabit/backend/internal/modules/hris/domain`; `gofmt` tanpa output.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/modules/hris/domain/attendance_request.go backend/internal/modules/hris/domain/attendance_request_test.go
git commit -m "feat(hris): aturan pengajuan absensi, absen terlewat, jam minimum (EPIC-065)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: Migrasi + pengajuan karyawan (buat, daftar sendiri, batal, lampiran, notifikasi HR)

**Files:**
- Create: `backend/database/migrations/deltas/20261025090000_attendance_requests.sql`
- Create: `backend/internal/modules/hris/attendance_requests.go`
- Modify: `backend/internal/modules/hris/service.go:15-37` (embed `AttendanceRequestRepo` di `Repository`)
- Modify: `backend/internal/modules/hris/routes.go` (setelah blok business trips dan di blok self service)
- Modify: `backend/internal/modules/hris/http_files.go:29-41` (`fileRoutes`: lampiran)
- Modify: `frontend/src/lib/go-routes.generated.json` (hasil `make generate`), `frontend/src/lib/backend-routes.test.ts` (`goNativePrefixes`)
- Test: `backend/internal/modules/hris/attendance_requests_integration_test.go`

**Interfaces:**
- Consumes: `domain.CheckAttendanceRequest`, `domain.AttendanceRequestNotice` (Task 1); `ensureAttendanceOpen` (`attendance_periods.go`), `saveImage`, `readUploadJSON`, `maxAttachmentBytes`, `ownedFile` (`files.go`, `http_files.go`), `auditDecision`, `decisionRace` (`approvals.go`), `auditMeta` (`attendance_adjust.go`), `errNoRow`, `insertRow`, `updateOne` (`sqlbuild.go`); helper test `newFileHarness`, `h.send`, `pngBytes`, `dataURL` (`files_integration_test.go`).
- Produces:
  - Tabel `hris.attendance_requests` (kolom: `id, employee_id, date, kind, clock_in, clock_out, reason, attachment_path, status, attendance_id, decided_by, decided_by_name, decided_at, rejection_reason, created_at, updated_at`), index `attendance_requests_one_pending`.
  - `type AttendanceRequestRepo interface` dengan `InsertAttendanceRequest`, `AttendanceRequestByID`, `AttendanceRequestList(ctx, attendanceRequestFilter)`, `SetAttendanceRequestStatus(ctx, id, fromStatus string, f fields) (*Row, error)`, `PendingAttendanceRequestOn`, `RecordedAttendance(ctx, employeeID, date string) (*Row, error)`, `NotifyHR(ctx, kind, title, message, link string) error`
  - `type attendanceRequestFilter struct { EmployeeID, Status string }`
  - `const attendanceHref = "/dashboard/hris/attendance"`, `essAttendanceHref = "/dashboard/me/absensi"`
  - `func requirePendingRequest(req *Row) error`, `func attendanceRequestLabel(req *Row) string`, `func requestID(raw string) (string, error)`
  - Routes: `POST /api/hris/attendance-requests`, `POST /api/hris/attendance-requests/{id}/cancel`, `GET /api/hris/me/attendance-requests`, `GET /api/hris/attendance-requests/attachment/{path...}`
  - Helper test `func attReqBody(date, in, out, reason string) map[string]any` (dipakai Task 3 dan 4)

- [ ] **Step 1: Tulis migrasi dan terapkan ke DB test**

`backend/database/migrations/deltas/20261025090000_attendance_requests.sql`:

```sql
-- =============================================================================
-- Pengajuan absen manual / koreksi absen (EPIC-065, MOM AB-04 + AB-06, K-9).
-- Karyawan mengajukan dari ESS dengan keterangan (dan foto bukti opsional);
-- HR memutus (matriks approval: "Absen Manual / Koreksi Absen | HR").
-- Persetujuan menulis hris.attendance lewat jalur penyesuaian HR yang sama
-- (riwayat hris.attendance_adjustments + audit) dan menautkan attendance_id.
-- Satu pengajuan pending per karyawan per tanggal.
-- =============================================================================

CREATE TABLE IF NOT EXISTS hris.attendance_requests (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  employee_id      uuid NOT NULL REFERENCES hris.employees(id) ON DELETE CASCADE,
  date             date NOT NULL,
  kind             text NOT NULL,
  clock_in         text,
  clock_out        text,
  reason           text NOT NULL,
  attachment_path  text,
  status           text NOT NULL DEFAULT 'pending',
  attendance_id    uuid REFERENCES hris.attendance(id) ON DELETE SET NULL,
  decided_by       uuid REFERENCES hris.employees(id) ON DELETE SET NULL,
  decided_by_name  text,
  decided_at       timestamptz,
  rejection_reason text,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT attendance_requests_kind_check CHECK (kind IN ('manual', 'correction')),
  CONSTRAINT attendance_requests_status_check CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
  CONSTRAINT attendance_requests_clock_check CHECK (
    (clock_in IS NOT NULL OR clock_out IS NOT NULL)
    AND (clock_in IS NULL OR clock_in ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$')
    AND (clock_out IS NULL OR clock_out ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$')),
  CONSTRAINT attendance_requests_reason_check CHECK (length(btrim(reason)) >= 5)
);

CREATE UNIQUE INDEX IF NOT EXISTS attendance_requests_one_pending
  ON hris.attendance_requests (employee_id, date) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_attendance_requests_status ON hris.attendance_requests (status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_attendance_requests_employee ON hris.attendance_requests (employee_id, date DESC);

DROP TRIGGER IF EXISTS attendance_requests_updated_at ON hris.attendance_requests;
CREATE TRIGGER attendance_requests_updated_at BEFORE UPDATE ON hris.attendance_requests
  FOR EACH ROW EXECUTE FUNCTION public.update_hris_updated_at();

COMMENT ON TABLE hris.attendance_requests IS
  'EPIC-065: pengajuan absen manual/koreksi karyawan; HR memutus, persetujuan menulis hris.attendance lewat penyesuaian HR.';
```

Run: `cd backend && MIGRATE_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go run ./cmd/migrate -apply`
Expected: `20261025090000_attendance_requests.sql` diterapkan tanpa error. Cek: `psql 'postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' -Atc "SELECT count(*) FROM pg_indexes WHERE indexname = 'attendance_requests_one_pending'"` → `1`.

- [ ] **Step 2: Tulis test yang gagal**

`backend/internal/modules/hris/attendance_requests_integration_test.go`:

```go
package hris

import (
	"net/http/httptest"
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

// attReqBody is a request body; "" leaves a clock out.
func attReqBody(date, in, out, reason string) map[string]any {
	body := map[string]any{"date": date, "reason": reason}
	if in != "" {
		body["clock_in"] = in
	}
	if out != "" {
		body["clock_out"] = out
	}
	return body
}

// EPIC-065 R4/R5/R6 (MOM AB-04, K-9): karyawan mengajukan absen manual atau
// koreksi dengan keterangan; HR menerima notifikasi in-app.
func TestAttendanceRequestCreate(t *testing.T) {
	h, _ := newFileHarness(t)
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 0, closing_offset_days = 3`)
	hr, _ := h.staff("hrd", []string{"hris.workforce.attendance"}, true, emp{name: "HR Absensi"})
	crew, crewID := h.staff("pos", nil, true, emp{name: "Crew Pengajuan"})
	other, _ := h.staff("pos", nil, true, emp{name: "Crew Lain"})
	unlinked, _ := h.staff("pos", nil, false)
	const url = "/api/hris/attendance-requests"
	const why = "Internet toko mati, tidak bisa absen"

	// Validasi (R4/R5).
	expect(t, h.do(&unlinked, "POST", url, attReqBody("2026-10-02", "08:00", "17:00", why)), 400,
		"Akun ini tidak tertaut ke data karyawan")
	expect(t, h.do(&crew, "POST", url, map[string]any{"reason": why, "clock_in": "08:00"}), 400, "Tanggal wajib diisi")
	expect(t, h.do(&crew, "POST", url, attReqBody("2026-10-06", "08:00", "", why)), 400,
		"Tanggal pengajuan tidak boleh di masa depan")
	expect(t, h.do(&crew, "POST", url, attReqBody("2026-10-02", "08:00", "", "Lupa")), 400,
		"Keterangan wajib diisi (minimal 5 karakter)")
	expect(t, h.do(&crew, "POST", url, attReqBody("2026-10-02", "8:00", "", why)), 400, "Format jam harus HH:MM")
	expect(t, h.do(&crew, "POST", url, attReqBody("2026-10-02", "", "", why)), 400,
		"Isi jam masuk dan/atau jam pulang yang diajukan")
	expect(t, h.do(&crew, "POST", url, attReqBody("2026-10-02", "", "17:00", why)), 400, "Jam masuk wajib diisi")
	expect(t, h.do(&crew, "POST", url, attReqBody("2026-10-05", "11:00", "", why)), 400, "Jam yang diajukan belum terjadi")
	closed := h.do(&crew, "POST", url, attReqBody("2026-09-30", "08:00", "17:00", why))
	expect(t, closed, 409, "")
	if !strings.HasPrefix(closed.body["error"].(string), "Periode absensi 1–30 Sep 2026 sudah di-closing") {
		t.Fatalf("closed period = %s", closed.raw)
	}
	expect(t, h.do(&crew, "POST", url, map[string]any{"date": "2026-10-02", "clock_in": "08:00", "reason": why,
		"photo": "data:image/gif;base64,R0lG"}), 400, "Lampiran harus berupa gambar JPG/PNG/WebP")

	// Absen manual dengan foto bukti.
	created := h.do(&crew, "POST", url, map[string]any{"date": "2026-10-02", "clock_in": "08:00", "clock_out": "17:00",
		"reason": "  " + why + "  ", "photo": dataURL("image/png", pngBytes(t))})
	expect(t, created, 201, "")
	d := created.data()
	if d["kind"] != "manual" || d["status"] != "pending" || d["reason"] != why || d["clock_in"] != "08:00" ||
		d["clock_out"] != "17:00" || d["recorded"] != nil || d["employee"].(map[string]any)["full_name"] != "Crew Pengajuan" ||
		created.body["message"] != "Pengajuan absensi terkirim — menunggu persetujuan HR" {
		t.Fatalf("created = %s", created.raw)
	}
	path, _ := d["attachment_path"].(string)
	if !strings.HasPrefix(path, "attendance-requests/"+crewID+"/") {
		t.Fatalf("attachment path = %q", path)
	}
	for _, c := range []struct {
		who  *testutil.Staff
		want int
	}{{&crew, 200}, {&hr, 200}, {&other, 403}, {nil, 401}} {
		if rec := h.send(c.who, httptest.NewRequest("GET", "/api/hris/attendance-requests/attachment/"+path, nil)); rec.Code != c.want {
			t.Fatalf("attachment read = %d, want %d", rec.Code, c.want)
		}
	}
	// Satu pengajuan menunggu per tanggal.
	expect(t, h.do(&crew, "POST", url, attReqBody("2026-10-02", "08:05", "", why)), 400,
		"Masih ada pengajuan absensi yang menunggu untuk tanggal ini")
	// HR menerima notifikasi in-app (kanal notify_hrd yang sudah ada, R6).
	if n := h.scalar(`SELECT count(*)::text FROM public.notifications WHERE user_id = $1 AND type = 'approval'
		AND title = 'Pengajuan Absensi: Crew Pengajuan'
		AND message = 'Crew Pengajuan mengajukan absen manual 2 Okt 2026 (masuk 08:00, pulang 17:00). Tinjau di HRIS → Absensi → Pengajuan Absensi.'`,
		hr.UserID); n != "1" {
		t.Fatalf("hr notifications = %s", n)
	}

	// Koreksi: baris absensi sudah ada (belum absen pulang).
	h.exec(`INSERT INTO hris.attendance (employee_id, date, clock_in, status) VALUES ($1, '2026-10-03', '2026-10-03 08:02+07', 'present')`, crewID)
	corr := h.do(&crew, "POST", url, attReqBody("2026-10-03", "", "17:00", "Lupa absen pulang, HP mati"))
	expect(t, corr, 201, "")
	if corr.data()["kind"] != "correction" || corr.data()["recorded"] == nil {
		t.Fatalf("correction = %s", corr.raw)
	}
	corrID := corr.data()["id"].(string)
	h.exec(`INSERT INTO hris.attendance (employee_id, date, clock_in, clock_out, status)
		VALUES ($1, '2026-10-04', '2026-10-04 08:00:41+07', '2026-10-04 17:00+07', 'present')`, crewID)
	expect(t, h.do(&crew, "POST", url, attReqBody("2026-10-04", "08:00", "", "Jam masuk saya benar")), 400,
		"Jam yang diajukan sama dengan absensi tercatat")

	// Daftar milik sendiri.
	mine := h.do(&crew, "GET", "/api/hris/me/attendance-requests", nil)
	expect(t, mine, 200, "")
	if l := mine.list(); len(l) != 2 {
		t.Fatalf("mine = %s", mine.raw)
	}
	if l := h.do(&other, "GET", "/api/hris/me/attendance-requests", nil).list(); len(l) != 0 {
		t.Fatalf("other sees %v", l)
	}

	// Batal: hanya pemohon, hanya saat menunggu; id huruf besar dikanonikkan.
	expect(t, h.do(&crew, "POST", url+"/bukan-uuid/cancel", nil), 400, "ID pengajuan tidak valid")
	expect(t, h.do(&crew, "POST", url+"/ffffffff-ffff-ffff-ffff-ffffffffffff/cancel", nil), 404, "Pengajuan absensi tidak ditemukan")
	expect(t, h.do(&other, "POST", url+"/"+corrID+"/cancel", nil), 403, "Hanya pemohon yang bisa membatalkan pengajuan ini")
	cancelled := h.do(&crew, "POST", url+"/"+strings.ToUpper(corrID)+"/cancel", nil)
	expect(t, cancelled, 200, "")
	if cancelled.data()["status"] != "cancelled" || cancelled.body["message"] != "Pengajuan absensi dibatalkan" {
		t.Fatalf("cancel = %s", cancelled.raw)
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'attendance_request.cancel'
		AND entity_id = $1 AND before->>'status' = 'pending' AND after->>'status' = 'cancelled'`, corrID); n != "1" {
		t.Fatalf("cancel audit = %s", n)
	}
	expect(t, h.do(&crew, "POST", url+"/"+corrID+"/cancel", nil), 400, "Pengajuan sudah dibatalkan")
	// Setelah dibatalkan, tanggal yang sama boleh diajukan lagi.
	expect(t, h.do(&crew, "POST", url, attReqBody("2026-10-03", "", "17:05", "Lupa absen pulang, HP mati")), 201, "")
	// Tanggal kalender yang tidak ada: 400, bukan 500 dari cast ::date.
	expect(t, h.do(&crew, "POST", url, attReqBody("2026-02-30", "08:00", "", why)), 400, "Format tanggal tidak valid")
}
```

- [ ] **Step 3: Jalankan test, pastikan gagal**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris/ -run TestAttendanceRequestCreate -count=1`
Expected: FAIL — status 404/405 pada `POST /api/hris/attendance-requests` (route belum ada).

- [ ] **Step 4: Implementasi**

`backend/internal/modules/hris/attendance_requests.go`:

```go
package hris

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/validate"
)

// Pengajuan absen manual / koreksi absen (EPIC-065, MOM AB-04 + AB-06, K-9):
// the employee asks with a reason, HR decides (matriks approval: "Absen
// Manual / Koreksi Absen | HR"); hris.attendance_requests.

// AttendanceRequestRepo is the request storage.
type AttendanceRequestRepo interface {
	InsertAttendanceRequest(ctx context.Context, f fields) (*Row, error)
	AttendanceRequestByID(ctx context.Context, id string) (*Row, error)
	AttendanceRequestList(ctx context.Context, f attendanceRequestFilter) ([]*Row, error)
	// SetAttendanceRequestStatus updates a request still in fromStatus;
	// errNoRow when another decision got there first.
	SetAttendanceRequestStatus(ctx context.Context, id, fromStatus string, f fields) (*Row, error)
	PendingAttendanceRequestOn(ctx context.Context, employeeID, date string) (bool, error)
	// RecordedAttendance is the employee's attendance row on date (id,
	// clock_in, clock_out, status); nil when there is none.
	RecordedAttendance(ctx context.Context, employeeID, date string) (*Row, error)
	// NotifyHR is public.notify_hrd: the in-app notification the HR
	// accounts (app role hrd) already get for attendance events (R6).
	NotifyHR(ctx context.Context, kind, title, message, link string) error
}

type attendanceRequestFilter struct {
	EmployeeID string
	Status     string
}

type attendanceRequestInput struct {
	Date              string
	ClockIn, ClockOut *string
	Reason            string
	Photo             *string // data URL, optional
}

var attendanceRequestStatuses = []string{"pending", "approved", "rejected", "cancelled"}

// attendanceRequestStatusLabels finish "Pengajuan sudah …".
var attendanceRequestStatusLabels = map[string]string{"approved": "disetujui", "rejected": "ditolak", "cancelled": "dibatalkan"}

const (
	// attendanceHref is the HR Absensi menu (badge, HR notification link).
	attendanceHref = "/dashboard/hris/attendance"
	// essAttendanceHref is the ESS Absensi page (employee notifications).
	essAttendanceHref       = "/dashboard/me/absensi"
	attendanceRequestFolder = "attendance-requests"
	pendingRequestExists    = "Masih ada pengajuan absensi yang menunggu untuk tanggal ini"
)

// CreateAttendanceRequest files the actor's own request (R4/R5) and notifies
// HR in the same transaction (R6).
func (s *Service) CreateAttendanceRequest(ctx context.Context, a *Actor, in attendanceRequestInput) (*Row, error) {
	if a.EmployeeID == nil {
		return nil, httpx.BadRequest("Akun ini tidak tertaut ke data karyawan")
	}
	employeeID := *a.EmployeeID
	// A regex-valid but non-existent date (2026-02-30) must not reach the ::date cast.
	if _, valid := domain.ParseDate(in.Date); !valid {
		return nil, httpx.BadRequest("Format tanggal tidak valid")
	}
	recorded, err := s.repo.RecordedAttendance(ctx, employeeID, in.Date)
	if err != nil {
		return nil, err
	}
	check := domain.AttendanceRequestInput{Date: in.Date, ClockIn: in.ClockIn, ClockOut: in.ClockOut, Reason: in.Reason,
		HasRow: recorded != nil}
	if recorded != nil {
		if t, found := recorded.Time("clock_in"); found {
			check.RecordedIn = &t
		}
		if t, found := recorded.Time("clock_out"); found {
			check.RecordedOut = &t
		}
	}
	kind, msg := domain.CheckAttendanceRequest(check, s.now())
	if msg != "" {
		return nil, httpx.BadRequest(msg)
	}
	if err := s.ensureAttendanceOpen(ctx, in.Date); err != nil {
		return nil, err
	}
	pending, err := s.repo.PendingAttendanceRequestOn(ctx, employeeID, in.Date)
	if err != nil {
		return nil, err
	}
	if pending {
		return nil, httpx.BadRequest(pendingRequestExists)
	}
	var path *string
	if in.Photo != nil && *in.Photo != "" {
		saved, err := s.saveImage(*in.Photo, maxAttachmentBytes, attendanceRequestFolder+"/"+employeeID,
			"Lampiran harus berupa gambar JPG/PNG/WebP", "Ukuran lampiran maksimal 5 MB")
		if err != nil {
			return nil, err
		}
		path = &saved
	}
	var f fields
	f.add("employee_id", employeeID)
	f.add("date", in.Date)
	f.add("kind", kind)
	f.add("clock_in", in.ClockIn)
	f.add("clock_out", in.ClockOut)
	f.add("reason", domain.JSTrim(in.Reason))
	f.add("attachment_path", path)
	var row *Row
	err = s.repo.InTx(ctx, func(r Repository) error {
		var err error
		if row, err = r.InsertAttendanceRequest(ctx, f); err != nil {
			return err
		}
		name := ""
		if emp := row.Child("employee"); emp != nil {
			name = emp.Str("full_name")
		}
		title, message := domain.AttendanceRequestNotice(name, kind, in.Date, in.ClockIn, in.ClockOut)
		return r.NotifyHR(ctx, "approval", title, message, attendanceHref)
	})
	if database.PgCode(err) == "23505" {
		return nil, httpx.BadRequest(pendingRequestExists)
	}
	if err != nil {
		return nil, err
	}
	return obj("data", row, "message", "Pengajuan absensi terkirim — menunggu persetujuan HR"), nil
}

// MyAttendanceRequests is the actor's own requests (ESS).
func (s *Service) MyAttendanceRequests(ctx context.Context, a *Actor) ([]*Row, error) {
	if a.EmployeeID == nil {
		return []*Row{}, nil
	}
	return s.repo.AttendanceRequestList(ctx, attendanceRequestFilter{EmployeeID: *a.EmployeeID})
}

// CancelAttendanceRequest: the requester withdraws a pending request (R4).
func (s *Service) CancelAttendanceRequest(ctx context.Context, a *Actor, id string, meta audit.Entry) (*Row, error) {
	req, err := s.repo.AttendanceRequestByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, httpx.NotFound("Pengajuan absensi tidak ditemukan")
	}
	if a.EmployeeID == nil || req.Str("employee_id") != *a.EmployeeID {
		return nil, httpx.Forbidden("Hanya pemohon yang bisa membatalkan pengajuan ini")
	}
	if err := requirePendingRequest(req); err != nil {
		return nil, err
	}
	var row *Row
	err = s.repo.InTx(ctx, func(r Repository) error {
		var f fields
		f.add("status", "cancelled")
		var err error
		if row, err = r.SetAttendanceRequestStatus(ctx, id, "pending", f); err != nil {
			return err
		}
		return auditDecision(ctx, r.querier(), a, "attendance_request.cancel", "hris.attendance_requests", id,
			attendanceRequestLabel(req), nil, map[string]any{"status": "cancelled"}, meta)
	})
	if errors.Is(err, errNoRow) {
		return nil, httpx.Conflict(decisionRace)
	}
	if err != nil {
		return nil, err
	}
	return obj("data", row, "message", "Pengajuan absensi dibatalkan"), nil
}

// requirePendingRequest: only a pending request is cancelled or decided.
func requirePendingRequest(req *Row) error {
	if status := req.Str("status"); status != "pending" {
		return httpx.BadRequest("Pengajuan sudah "+attendanceRequestStatusLabels[status], obj("current_status", status))
	}
	return nil
}

// attendanceRequestLabel is "Nama · 2 Okt 2026" for the audit log.
func attendanceRequestLabel(req *Row) string {
	label := domain.ShortDateID(req.Str("date"))
	if emp := req.Child("employee"); emp != nil {
		label = emp.Str("full_name") + " · " + label
	}
	return label
}

/* ── SQL ─────────────────────────────────────────────────────────────── */

// attendanceRequestColumns embed the requester and the attendance recorded
// on the date now (HR compares it with the requested times).
var attendanceRequestColumns = `q.id, q.employee_id, q.date::text AS date, q.kind, q.clock_in, q.clock_out, q.reason,
	q.attachment_path, q.status, q.attendance_id, q.decided_by, q.decided_by_name, q.decided_at, q.rejection_reason,
	q.created_at, q.updated_at, ` +
	embedOne("employee", "id, full_name, nip", "hris.employees", "id = q.employee_id") + `,
	(SELECT json_build_object('id', a.id, 'clock_in', a.clock_in, 'clock_out', a.clock_out, 'status', a.status)
	   FROM hris.attendance a WHERE a.employee_id = q.employee_id AND a.date = q.date) AS recorded`

func (s *store) InsertAttendanceRequest(ctx context.Context, f fields) (*Row, error) {
	row, err := insertRow(ctx, s.db, "hris.attendance_requests", f, "id")
	if err != nil || row == nil {
		return nil, err
	}
	return s.AttendanceRequestByID(ctx, row.Str("id"))
}

func (s *store) AttendanceRequestByID(ctx context.Context, id string) (*Row, error) {
	row, err := queryRow(ctx, s.db, `SELECT `+attendanceRequestColumns+` FROM hris.attendance_requests q WHERE q.id = $1`, id)
	return row, ignoreBadInput(err)
}

func (s *store) AttendanceRequestList(ctx context.Context, f attendanceRequestFilter) ([]*Row, error) {
	where := []string{"true"}
	var args []any
	if f.EmployeeID != "" {
		args = append(args, f.EmployeeID)
		where = append(where, fmt.Sprintf("q.employee_id = $%d", len(args)))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		where = append(where, fmt.Sprintf("q.status = $%d", len(args)))
	}
	rows, err := queryRows(ctx, s.db, `SELECT `+attendanceRequestColumns+` FROM hris.attendance_requests q WHERE `+
		strings.Join(where, " AND ")+` ORDER BY (q.status = 'pending') DESC, q.date DESC, q.created_at DESC LIMIT 200`, args...)
	return rows, ignoreBadInput(err)
}

func (s *store) SetAttendanceRequestStatus(ctx context.Context, id, fromStatus string, f fields) (*Row, error) {
	if _, err := updateOne(ctx, s.db, "hris.attendance_requests", f, fields{{"id", id}, {"status", fromStatus}}, "id"); err != nil {
		return nil, err
	}
	return s.AttendanceRequestByID(ctx, id)
}

func (s *store) PendingAttendanceRequestOn(ctx context.Context, employeeID, date string) (bool, error) {
	var pending bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM hris.attendance_requests
		WHERE employee_id = $1 AND date = $2::date AND status = 'pending')`, employeeID, date).Scan(&pending)
	return pending, err
}

func (s *store) RecordedAttendance(ctx context.Context, employeeID, date string) (*Row, error) {
	row, err := queryRow(ctx, s.db, `SELECT id::text AS id, clock_in, clock_out, status::text AS status
		FROM hris.attendance WHERE employee_id = $1 AND date = $2::date`, employeeID, date)
	return row, ignoreBadInput(err)
}

func (s *store) NotifyHR(ctx context.Context, kind, title, message, link string) error {
	_, err := s.db.Exec(ctx, `SELECT public.notify_hrd($1, $2, $3, $4)`, title, message, kind, link)
	return err
}

/* ── HTTP ────────────────────────────────────────────────────────────── */

// requestID canonicalises a path id (lower-case UUID) before any lookup.
func requestID(raw string) (string, error) {
	id, valid := validate.CanonicalUUID(raw)
	if !valid {
		return "", httpx.BadRequest("ID pengajuan tidak valid")
	}
	return id, nil
}

var requestClockOpts = validate.StrOpts{Check: func(s string) (string, string, bool) {
	return "invalid_format", "Format jam harus HH:MM", domain.ValidClock(s)
}}

// POST /api/hris/attendance-requests { date, clock_in?, clock_out?, reason, photo? }
func (h handlers) createAttendanceRequest(w http.ResponseWriter, r *http.Request, a *Actor) error {
	f, err := readUploadJSON(r, "")
	if err != nil {
		return err
	}
	in := attendanceRequestInput{
		Date: strOr(reqStr(f, "date", "Tanggal wajib diisi",
			validate.StrOpts{Check: matches(domain.DateRe, "Format tanggal tidak valid")})),
		ClockIn:  f.Str("clock_in", nullish, requestClockOpts),
		ClockOut: f.Str("clock_out", nullish, requestClockOpts),
		Reason:   strOr(f.Str("reason", nullish, validate.StrOpts{Max: 1000})),
		Photo:    f.Str("photo", nullish, validate.StrOpts{}),
	}
	if err := formErr(f, ""); err != nil {
		return err
	}
	out, err := h.svc.CreateAttendanceRequest(r.Context(), a, in)
	if err != nil {
		return err
	}
	return reply(w, http.StatusCreated, out)
}

// GET /api/hris/me/attendance-requests
func (h handlers) myAttendanceRequests(w http.ResponseWriter, r *http.Request, a *Actor) error {
	rows, err := h.svc.MyAttendanceRequests(r.Context(), a)
	if err != nil {
		return err
	}
	return ok(w, dataOf(rows))
}

// POST /api/hris/attendance-requests/{id}/cancel
func (h handlers) cancelAttendanceRequest(w http.ResponseWriter, r *http.Request, a *Actor) error {
	id, err := requestID(r.PathValue("id"))
	if err != nil {
		return err
	}
	out, err := h.svc.CancelAttendanceRequest(r.Context(), a, id, auditMeta(r))
	if err != nil {
		return err
	}
	return ok(w, out)
}

// GET /api/hris/attendance-requests/attachment/attendance-requests/<employeeId>/<file>:
// the requester and HR read it.
func (h handlers) attendanceRequestAttachment(w http.ResponseWriter, r *http.Request, a *Actor) error {
	return h.ownedFile(w, r, a, "/api/hris/attendance-requests/attachment/", attendanceRequestFolder)
}
```

`backend/internal/modules/hris/service.go`, di `type Repository interface`, setelah `ApprovalRepo`:

```go
	ApprovalRepo
	AttendanceRequestRepo
```

`backend/internal/modules/hris/routes.go`, setelah `r("POST /api/hris/business-trips/{id}/decide", h.withActor(h.decideTrip)),`:

```go
		// attendance requests (EPIC-065): the employee asks, HR decides;
		// the attachment read is in fileRoutes
		r("POST /api/hris/attendance-requests", h.withActor(h.createAttendanceRequest)),
		r("POST /api/hris/attendance-requests/{id}/cancel", h.withActor(h.cancelAttendanceRequest)),
```

dan di blok self service, setelah `r("GET /api/hris/me/team-approvals", h.withActor(h.teamApprovals)),`:

```go
		r("GET /api/hris/me/attendance-requests", h.withActor(h.myAttendanceRequests)),
```

`backend/internal/modules/hris/http_files.go`, di `fileRoutes`, setelah baris `GET /api/hris/leaves/attachment/{path...}`:

```go
		r("GET /api/hris/attendance-requests/attachment/{path...}", h.withActor(h.attendanceRequestAttachment)),
```

`frontend/src/lib/backend-routes.test.ts`, ganti:

```ts
      // Aktifkan kembali karyawan (EPIC-064).
      "/api/hris/employees/{id}/rehire"];
```

dengan:

```ts
      // Aktifkan kembali karyawan (EPIC-064).
      "/api/hris/employees/{id}/rehire",
      // Pengajuan absen manual/koreksi (EPIC-065): hanya di Go, tanpa fallback Next.
      "/api/hris/attendance-requests", "/api/hris/me/attendance-requests"];
```

Run: `cd backend && PATH=$HOME/.local/go/bin:$PATH make generate`
Expected: `frontend/src/lib/go-routes.generated.json` memuat keempat route baru.

- [ ] **Step 5: Jalankan test, pastikan lulus**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris/ -run 'TestAttendanceRequestCreate|TestAttendancePeriodClosing|TestLeaveAttachment' -count=1 && gofmt -l internal/modules/hris`
Expected: `TestAttendanceRequestCreate` PASS (kegagalan `TestAttendancePeriodClosing` hanya bila ada di baseline); `gofmt` tanpa output.

Run: `cd frontend && npx vitest run src/lib/backend-routes.test.ts`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/database/migrations/deltas/20261025090000_attendance_requests.sql backend/internal/modules/hris/attendance_requests.go \
  backend/internal/modules/hris/attendance_requests_integration_test.go backend/internal/modules/hris/service.go \
  backend/internal/modules/hris/routes.go backend/internal/modules/hris/http_files.go \
  frontend/src/lib/go-routes.generated.json frontend/src/lib/backend-routes.test.ts
git commit -m "feat(hris): pengajuan absen manual/koreksi karyawan + notifikasi HR (EPIC-065)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: Antrean & keputusan HR lewat jalur penyesuaian yang sama + badge (Go)

**Files:**
- Modify: `backend/internal/modules/hris/attendance_adjust.go` (`CreateManualAttendance` → pembungkus `createManualAttendance`; `AdjustAttendance` → pembungkus `adjustAttendance`; helper `runAlso`)
- Modify: `backend/internal/modules/hris/attendance_requests.go` (antrean, keputusan, `NotifyEmployee`, `PendingAttendanceRequestCount`, handler)
- Modify: `backend/internal/modules/hris/routes.go`
- Modify: `backend/internal/modules/hris/selfservice.go:246-263` (`NavBadges`)
- Modify: `frontend/src/lib/go-routes.generated.json` (hasil `make generate`)
- Test: `backend/internal/modules/hris/attendance_request_decision_integration_test.go`

**Interfaces:**
- Consumes: Task 2 (`AttendanceRequestRepo`, `requirePendingRequest`, `attendanceRequestLabel`, `requestID`, `attendanceHref`, `essAttendanceHref`, `attReqBody`); Task 1 `domain.AttendanceDecisionNotice`; `adjustment`, `opt[T]`, `canManageAttendance`, `logAdjustment` (`attendance_adjust.go`); `auditDecision`, `decisionRace` (`approvals.go`); `ptr`, `actorName` (`attendance_periods.go`).
- Produces:
  - `func (s *Service) createManualAttendance(ctx context.Context, a *Actor, in adjustment, meta audit.Entry, also func(Repository, *Row) error) (*Row, error)`
  - `func (s *Service) adjustAttendance(ctx context.Context, a *Actor, id string, in adjustment, validation fields, validated bool, meta audit.Entry, also func(Repository, *Row) error) (*Row, error)`
  - `var attendanceManagers = []string{"hris.workforce.attendance"}`
  - `AttendanceRequestRepo` bertambah `PendingAttendanceRequestCount(ctx) (int64, error)` dan `NotifyEmployee(ctx, employeeID, kind, title, message, link string) error`
  - `func (s *Service) ListAttendanceRequests(ctx, a *Actor, status, employeeID string) ([]*Row, error)`
  - `func (s *Service) DecideAttendanceRequest(ctx, a *Actor, id, action string, rejection *string, meta audit.Entry) (*Row, error)`
  - Routes: `GET /api/hris/attendance-requests`, `POST /api/hris/attendance-requests/{id}/decide`; badge `/dashboard/hris/attendance` di `GET /api/hris/nav-badges` untuk `IsHRD`.

- [ ] **Step 1: Tulis test yang gagal**

`backend/internal/modules/hris/attendance_request_decision_integration_test.go`:

```go
package hris

import (
	"fmt"
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

// EPIC-065 R2/R3 (matriks approval: Absen Manual / Koreksi Absen | HR): HRD
// memutus; persetujuan menulis absensi lewat penyesuaian HR yang sama.
func TestAttendanceRequestDecision(t *testing.T) {
	h := newHarness(t)
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 0, closing_offset_days = 3`)
	hr, hrID := h.staff("hrd", []string{"hris.workforce.attendance"}, true, emp{name: "HR Pemutus"})
	recruiter, _ := h.staff("hiring_manager", []string{"hris.workforce.attendance"}, true)
	noMenu, _ := h.staff("hrd", nil, true)
	boss, bossID := h.staff("pos", nil, true, emp{name: "Atasan Crew"})
	crew, crewID := h.staff("pos", nil, true, emp{name: "Crew Diputus", boss: &bossID})
	shift := h.scalar(`INSERT INTO hris.shifts (name, start_time, end_time, late_tolerance_minutes)
		VALUES ('Pagi Pengajuan', '08:00', '17:00', 10) RETURNING id::text`)
	for dow := 1; dow <= 7; dow++ {
		h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from) VALUES ($1, $2, $3, '2026-01-01')`,
			crewID, dow, shift)
	}
	const url = "/api/hris/attendance-requests"
	file := func(date, in, out, reason string) string {
		t.Helper()
		c := h.do(&crew, "POST", url, attReqBody(date, in, out, reason))
		expect(t, c, 201, "")
		return c.data()["id"].(string)
	}
	decide := func(s *testutil.Staff, id, action, reason string) call {
		body := map[string]any{"action": action}
		if reason != "" {
			body["rejection_reason"] = reason
		}
		return h.do(s, "POST", url+"/"+id+"/decide", body)
	}

	// Antrean HR: menu Absensi + HRD; filter employee_id dikanonikkan.
	manual := file("2026-10-02", "08:30", "17:00", "Internet toko mati pagi itu")
	queue := h.do(&hr, "GET", url+"?status=pending&employee_id="+strings.ToUpper(crewID), nil)
	expect(t, queue, 200, "")
	if l := queue.list(); len(l) != 1 || l[0].(map[string]any)["id"] != manual || l[0].(map[string]any)["recorded"] != nil {
		t.Fatalf("queue = %s", queue.raw)
	}
	expect(t, h.do(&hr, "GET", url+"?status=menunggu", nil), 400, "Status tidak valid")
	expect(t, h.do(&crew, "GET", url, nil), 403, "")
	expect(t, h.do(&recruiter, "GET", url, nil), 403, "Hanya HRD yang bisa memproses pengajuan absensi")
	expect(t, decide(&noMenu, manual, "approve", ""), 403, "")
	expect(t, decide(&recruiter, manual, "approve", ""), 403, "Hanya HRD yang bisa memproses pengajuan absensi")
	expect(t, decide(&boss, manual, "approve", ""), 403, "") // atasan langsung bukan pemutus
	badges := h.do(&hr, "GET", "/api/hris/nav-badges", nil)
	want := h.scalar(`SELECT count(*)::text FROM hris.attendance_requests WHERE status = 'pending'`)
	if got := badges.body["badges"].(map[string]any)["/dashboard/hris/attendance"]; fmt.Sprint(got) != want {
		t.Fatalf("badge = %v, want %s (%s)", got, want, badges.raw)
	}
	if _, has := h.do(&crew, "GET", "/api/hris/nav-badges", nil).body["badges"].(map[string]any)["/dashboard/hris/attendance"]; has {
		t.Fatal("crew got the HR attendance badge")
	}

	// Setujui absen manual: jalur penyesuaian HR (keterlambatan dari shift, riwayat, audit).
	approved := decide(&hr, strings.ToUpper(manual), "approve", "")
	expect(t, approved, 200, "")
	d := approved.data()
	if d["status"] != "approved" || d["decided_by"] != hrID || d["decided_by_name"] != hr.FullName ||
		approved.body["message"] != "Pengajuan absensi disetujui — absensi diperbarui" {
		t.Fatalf("approve = %s", approved.raw)
	}
	attID, _ := d["attendance_id"].(string)
	if got := h.scalar(`SELECT is_manual::text || '|' || is_late::text || '|' || late_minutes::text || '|' || shift_id::text || '|' ||
		(clock_in AT TIME ZONE 'Asia/Jakarta')::time::text || '|' || notes FROM hris.attendance WHERE id = $1`, attID); got !=
		"true|true|30|"+shift+"|08:30:00|Absen manual dari pengajuan karyawan" {
		t.Fatalf("attendance = %q", got)
	}
	if got := h.scalar(`SELECT action || '|' || reason FROM hris.attendance_adjustments WHERE attendance_id = $1`, attID); got !=
		"create|Pengajuan karyawan: Internet toko mati pagi itu" {
		t.Fatalf("adjustment = %q", got)
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'attendance_request.approve'
		AND entity_id = $1 AND after->>'attendance_id' = $2 AND before->>'status' = 'pending'`, manual, attID); n != "1" {
		t.Fatalf("approve audit = %s", n)
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'attendance.create' AND entity_id = $1`, attID); n != "1" {
		t.Fatalf("attendance audit = %s", n)
	}
	if n := h.scalar(`SELECT count(*)::text FROM public.notifications n JOIN hris.employees e ON e.user_id = n.user_id
		WHERE e.id = $1 AND n.type = 'status_change' AND n.title = 'Pengajuan absensi disetujui'`, crewID); n != "1" {
		t.Fatalf("employee notification = %s", n)
	}
	// Antrean basi: keputusan kedua ditolak tanpa menulis ulang absensi.
	expect(t, decide(&hr, manual, "approve", ""), 400, "Pengajuan sudah disetujui")
	if n := h.scalar(`SELECT count(*)::text FROM hris.attendance_adjustments WHERE attendance_id = $1`, attID); n != "1" {
		t.Fatalf("adjustments after stale approve = %s", n)
	}

	// Koreksi baris "Tidak hadir": jam diisi, status menjadi Hadir.
	h.exec(`INSERT INTO hris.attendance (employee_id, date, status) VALUES ($1, '2026-10-03', 'absent')`, crewID)
	absent := file("2026-10-03", "08:00", "17:00", "Saya hadir, aplikasi error")
	expect(t, decide(&hr, absent, "approve", ""), 200, "")
	if got := h.scalar(`SELECT status::text || '|' || adjustment_count::text FROM hris.attendance
		WHERE employee_id = $1 AND date = '2026-10-03'`, crewID); got != "present|1" {
		t.Fatalf("absent corrected = %q", got)
	}

	// Absen setelah mengajukan absen manual → disetujui sebagai koreksi baris itu.
	late := file("2026-10-04", "08:00", "17:00", "Absen masuk gagal terkirim")
	h.exec(`INSERT INTO hris.attendance (employee_id, date, clock_in, status) VALUES ($1, '2026-10-04', '2026-10-04 09:15+07', 'present')`, crewID)
	expect(t, decide(&hr, late, "approve", ""), 200, "")
	if got := h.scalar(`SELECT count(*)::text || '|' || max((clock_in AT TIME ZONE 'Asia/Jakarta')::time)::text
		FROM hris.attendance WHERE employee_id = $1 AND date = '2026-10-04'`, crewID); got != "1|08:00:00" {
		t.Fatalf("clocked after request = %q", got)
	}
	if got := h.scalar(`SELECT string_agg(action, ',') FROM hris.attendance_adjustments
		WHERE employee_id = $1 AND date = '2026-10-04'`, crewID); got != "update" {
		t.Fatalf("adjustments = %q", got)
	}

	// Tolak: alasan wajib; tidak ada absensi yang ditulis; karyawan diberi tahu alasannya.
	rejected := file("2026-10-01", "08:00", "17:00", "Lupa absen karena sibuk")
	expect(t, decide(&hr, rejected, "reject", ""), 400, "Alasan penolakan wajib diisi")
	rej := decide(&hr, rejected, "reject", "CCTV menunjukkan tidak hadir")
	expect(t, rej, 200, "")
	if rej.data()["status"] != "rejected" || rej.data()["rejection_reason"] != "CCTV menunjukkan tidak hadir" ||
		rej.body["message"] != "Pengajuan absensi ditolak" {
		t.Fatalf("reject = %s", rej.raw)
	}
	if n := h.scalar(`SELECT count(*)::text FROM hris.attendance WHERE employee_id = $1 AND date = '2026-10-01'`, crewID); n != "0" {
		t.Fatalf("rejected request wrote attendance: %s", n)
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'attendance_request.reject' AND entity_id = $1`, rejected); n != "1" {
		t.Fatalf("reject audit = %s", n)
	}
	if n := h.scalar(`SELECT count(*)::text FROM public.notifications n JOIN hris.employees e ON e.user_id = n.user_id
		WHERE e.id = $1 AND n.title = 'Pengajuan absensi ditolak'
		AND n.message = 'Pengajuan absensi 1 Okt 2026 ditolak HR. Alasan: CCTV menunjukkan tidak hadir'`, crewID); n != "1" {
		t.Fatalf("reject notification = %s", n)
	}

	// HR tidak memutus pengajuannya sendiri.
	own := h.do(&hr, "POST", url, attReqBody("2026-10-02", "08:00", "17:00", "Internet kantor mati"))
	expect(t, own, 201, "")
	expect(t, decide(&hr, own.data()["id"].(string), "approve", ""), 403, "Tidak bisa memutuskan pengajuan absensi sendiri")

	// Periode di-closing setelah pengajuan: setujui 409, tetap menunggu, tolak tetap bisa.
	stale := h.scalar(`INSERT INTO hris.attendance_requests (employee_id, date, kind, clock_in, reason)
		VALUES ($1, '2026-09-29', 'manual', '08:00', 'Diajukan sebelum closing') RETURNING id::text`, crewID)
	blocked := decide(&hr, stale, "approve", "")
	expect(t, blocked, 409, "")
	if !strings.HasPrefix(blocked.body["error"].(string), "Periode absensi 1–30 Sep 2026 sudah di-closing") {
		t.Fatalf("closed period approve = %s", blocked.raw)
	}
	if s := h.scalar(`SELECT status FROM hris.attendance_requests WHERE id = $1`, stale); s != "pending" {
		t.Fatalf("stale status = %s", s)
	}
	expect(t, decide(&hr, stale, "reject", "Periode sudah ditutup"), 200, "")
	expect(t, decide(&hr, "bukan-uuid", "approve", ""), 400, "ID pengajuan tidak valid")
	expect(t, decide(&hr, "ffffffff-ffff-ffff-ffff-ffffffffffff", "approve", ""), 404, "Pengajuan absensi tidak ditemukan")
	expect(t, decide(&hr, rejected, "batal", ""), 400, "")
}
```

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris/ -run TestAttendanceRequestDecision -count=1`
Expected: FAIL — `GET /api/hris/attendance-requests` 405/404 (route belum ada).

- [ ] **Step 3: Implementasi — kait transaksi di jalur penyesuaian HR**

`backend/internal/modules/hris/attendance_adjust.go`. Ganti kepala fungsi:

```go
// CreateManualAttendance adds a missing attendance row for an employee.
func (s *Service) CreateManualAttendance(ctx context.Context, a *Actor, in adjustment, meta audit.Entry) (*Row, error) {
	if !canManageAttendance(a) {
```

dengan:

```go
// CreateManualAttendance adds a missing attendance row for an employee.
func (s *Service) CreateManualAttendance(ctx context.Context, a *Actor, in adjustment, meta audit.Entry) (*Row, error) {
	return s.createManualAttendance(ctx, a, in, meta, nil)
}

// createManualAttendance is CreateManualAttendance with also run on the
// same transaction after the row and its logs are written (EPIC-065 R3: an
// approved employee request records its decision atomically with the row).
func (s *Service) createManualAttendance(ctx context.Context, a *Actor, in adjustment, meta audit.Entry,
	also func(Repository, *Row) error) (*Row, error) {
	if !canManageAttendance(a) {
```

dan di fungsi yang sama ganti:

```go
		if row, err = r.InsertAttendance(ctx, f); err != nil {
			return periodWriteError(err)
		}
		return s.logAdjustment(ctx, r, a, "create", row.Str("id"), in.EmployeeID, in.Date, in.Reason, nil, row, meta)
	})
```

dengan:

```go
		if row, err = r.InsertAttendance(ctx, f); err != nil {
			return periodWriteError(err)
		}
		if err := s.logAdjustment(ctx, r, a, "create", row.Str("id"), in.EmployeeID, in.Date, in.Reason, nil, row, meta); err != nil {
			return err
		}
		return runAlso(also, r, row)
	})
```

Ganti kepala `AdjustAttendance`:

```go
// AdjustAttendance applies an HR correction. Data changes need the manage
// role and a reason; validated=true stamps the validator as before.
func (s *Service) AdjustAttendance(ctx context.Context, a *Actor, id string, in adjustment, validation fields,
	validated bool, meta audit.Entry) (*Row, error) {
	current, err := s.repo.AttendanceRecord(ctx, id)
```

dengan:

```go
// AdjustAttendance applies an HR correction. Data changes need the manage
// role and a reason; validated=true stamps the validator as before.
func (s *Service) AdjustAttendance(ctx context.Context, a *Actor, id string, in adjustment, validation fields,
	validated bool, meta audit.Entry) (*Row, error) {
	return s.adjustAttendance(ctx, a, id, in, validation, validated, meta, nil)
}

// adjustAttendance is AdjustAttendance with also run on the same transaction
// after a data change is written and logged (EPIC-065 R3).
func (s *Service) adjustAttendance(ctx context.Context, a *Actor, id string, in adjustment, validation fields,
	validated bool, meta audit.Entry, also func(Repository, *Row) error) (*Row, error) {
	current, err := s.repo.AttendanceRecord(ctx, id)
```

dan di fungsi itu ganti:

```go
		if !data {
			return nil
		}
		return s.logAdjustment(ctx, r, a, "update", id, employeeID, date, in.Reason, current, row, meta)
	})
```

dengan:

```go
		if !data {
			return nil
		}
		if err := s.logAdjustment(ctx, r, a, "update", id, employeeID, date, in.Reason, current, row, meta); err != nil {
			return err
		}
		return runAlso(also, r, row)
	})
```

Tambahkan di akhir bagian service (sebelum `// DeleteAttendanceWithReason`):

```go
// runAlso runs a caller's in-transaction hook (nil: none).
func runAlso(also func(Repository, *Row) error, r Repository, row *Row) error {
	if also == nil {
		return nil
	}
	return also(r, row)
}
```

- [ ] **Step 4: Implementasi — antrean, keputusan, badge**

`backend/internal/modules/hris/attendance_requests.go`. Tambah impor `"slices"` dan `"nuhabit/backend/internal/platform/auth"`. Di `AttendanceRequestRepo` tambahkan:

```go
	// PendingAttendanceRequestCount is the HR Absensi badge.
	PendingAttendanceRequestCount(ctx context.Context) (int64, error)
	// NotifyEmployee is public.notify_user on the employee's login account.
	NotifyEmployee(ctx context.Context, employeeID, kind, title, message, link string) error
```

Tambahkan setelah `CancelAttendanceRequest`:

```go
// attendanceManagers hold the HR Absensi menu: the request queue and its
// decisions, together with Actor.IsHRD (R2).
var attendanceManagers = []string{"hris.workforce.attendance"}

const notHRD = "Hanya HRD yang bisa memproses pengajuan absensi"

// ListAttendanceRequests is the HR queue (R2): every employee's requests.
func (s *Service) ListAttendanceRequests(ctx context.Context, a *Actor, status, employeeID string) ([]*Row, error) {
	if !a.IsHRD {
		return nil, httpx.Forbidden(notHRD)
	}
	return s.repo.AttendanceRequestList(ctx, attendanceRequestFilter{EmployeeID: employeeID, Status: status})
}

// DecideAttendanceRequest approves or rejects a pending request (R2/R3).
// Approval writes the attendance through the functions HR's direct
// adjustment uses (reason, adjustment log, audit, closed-period lock) and
// records the decision, its audit row and the employee's notification in
// that transaction.
func (s *Service) DecideAttendanceRequest(ctx context.Context, a *Actor, id, action string, rejection *string,
	meta audit.Entry) (*Row, error) {
	if !a.IsHRD {
		return nil, httpx.Forbidden(notHRD)
	}
	req, err := s.repo.AttendanceRequestByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, httpx.NotFound("Pengajuan absensi tidak ditemukan")
	}
	if a.EmployeeID != nil && req.Str("employee_id") == *a.EmployeeID {
		return nil, httpx.Forbidden("Tidak bisa memutuskan pengajuan absensi sendiri")
	}
	if err := requirePendingRequest(req); err != nil {
		return nil, err
	}
	var decided *Row
	record := func(r Repository, status string, attendanceID, reason *string) error {
		var err error
		decided, err = s.recordAttendanceDecision(ctx, r, a, req, status, attendanceID, reason, meta)
		return err
	}
	message := "Pengajuan absensi disetujui — absensi diperbarui"
	if action == "reject" {
		reason := domain.JSTrim(strOr(rejection))
		if reason == "" {
			return nil, httpx.BadRequest("Alasan penolakan wajib diisi")
		}
		message = "Pengajuan absensi ditolak"
		err = s.repo.InTx(ctx, func(r Repository) error { return record(r, "rejected", nil, &reason) })
	} else {
		err = s.approveAttendanceRequest(ctx, a, req, meta, func(r Repository, att *Row) error {
			return record(r, "approved", ptr(att.Str("id")), nil)
		})
	}
	if errors.Is(err, errNoRow) {
		return nil, httpx.Conflict(decisionRace)
	}
	if err != nil {
		return nil, err
	}
	return obj("data", decided, "message", message), nil
}

// approveAttendanceRequest writes the requested times: a new row when the
// date has none at decision time, otherwise a correction of the row (the
// employee may have clocked in after asking; R3).
func (s *Service) approveAttendanceRequest(ctx context.Context, a *Actor, req *Row, meta audit.Entry,
	also func(Repository, *Row) error) error {
	recorded, err := s.repo.RecordedAttendance(ctx, req.Str("employee_id"), req.Str("date"))
	if err != nil {
		return err
	}
	in := requestAdjustment(req, recorded)
	if recorded == nil {
		_, err = s.createManualAttendance(ctx, a, in, meta, also)
		return err
	}
	_, err = s.adjustAttendance(ctx, a, recorded.Str("id"), in, nil, false, meta, also)
	return err
}

// requestAdjustment is the HR adjustment an approved request makes (R5):
// only the requested clock times change, a day HR had marked absent becomes
// present, and a new row notes that it came from a request.
func requestAdjustment(req, recorded *Row) adjustment {
	in := adjustment{EmployeeID: req.Str("employee_id"), Date: req.Str("date"),
		Reason: "Pengajuan karyawan: " + req.Str("reason")}
	if v := req.StrPtr("clock_in"); v != nil {
		in.ClockIn = opt[string]{Val: v, Sent: true}
	}
	if v := req.StrPtr("clock_out"); v != nil {
		in.ClockOut = opt[string]{Val: v, Sent: true}
	}
	switch {
	case recorded == nil:
		in.Notes = opt[string]{Val: ptr("Absen manual dari pengajuan karyawan"), Sent: true}
	case recorded.Str("status") == "absent":
		in.Status = opt[string]{Val: ptr("present"), Sent: true}
	}
	return in
}

// recordAttendanceDecision moves the request out of pending on r, with its
// audit row (LA-01, R11) and the employee's in-app notification (R6).
func (s *Service) recordAttendanceDecision(ctx context.Context, r Repository, a *Actor, req *Row, status string,
	attendanceID, rejection *string, meta audit.Entry) (*Row, error) {
	var f fields
	f.add("status", status)
	f.add("decided_by", a.EmployeeID)
	f.add("decided_by_name", actorName(a))
	f.add("decided_at", s.clock())
	f.add("attendance_id", attendanceID)
	f.add("rejection_reason", rejection)
	row, err := r.SetAttendanceRequestStatus(ctx, req.Str("id"), "pending", f)
	if err != nil {
		return nil, err
	}
	action := "attendance_request.approve"
	if status == "rejected" {
		action = "attendance_request.reject"
	}
	after := map[string]any{"status": status, "kind": req.Str("kind"), "attendance_id": attendanceID, "rejection_reason": rejection}
	if err := auditDecision(ctx, r.querier(), a, action, "hris.attendance_requests", req.Str("id"),
		attendanceRequestLabel(req), nil, after, meta); err != nil {
		return nil, err
	}
	title, message := domain.AttendanceDecisionNotice(status, req.Str("date"), rejection)
	return row, r.NotifyEmployee(ctx, req.Str("employee_id"), "status_change", title, message, essAttendanceHref)
}
```

Di bagian SQL tambahkan:

```go
func (s *store) PendingAttendanceRequestCount(ctx context.Context) (int64, error) {
	return countRows(ctx, s.db, `SELECT count(*) FROM hris.attendance_requests WHERE status = 'pending'`)
}

func (s *store) NotifyEmployee(ctx context.Context, employeeID, kind, title, message, link string) error {
	_, err := s.db.Exec(ctx, `SELECT public.notify_user(e.user_id, $2, $3, $4, $5)
		FROM hris.employees e WHERE e.id = $1`, employeeID, title, message, kind, link)
	return err
}
```

Di bagian HTTP tambahkan:

```go
// GET /api/hris/attendance-requests?status=&employee_id= (HR queue)
func (h handlers) listAttendanceRequests(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	q := r.URL.Query()
	status := q.Get("status")
	if status != "" && !slices.Contains(attendanceRequestStatuses, status) {
		return httpx.BadRequest("Status tidak valid")
	}
	employeeID := ""
	if raw := q.Get("employee_id"); raw != "" {
		id, valid := validate.CanonicalUUID(raw)
		if !valid {
			return httpx.BadRequest("ID karyawan tidak valid")
		}
		employeeID = id
	}
	a, err := h.svc.Actor(r.Context(), u)
	if err != nil {
		return err
	}
	rows, err := h.svc.ListAttendanceRequests(r.Context(), a, status, employeeID)
	if err != nil {
		return err
	}
	return ok(w, dataOf(rows))
}

// POST /api/hris/attendance-requests/{id}/decide { action: "approve" | "reject", rejection_reason? }
func (h handlers) decideAttendanceRequest(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	id, err := requestID(r.PathValue("id"))
	if err != nil {
		return err
	}
	const message = "Validation failed"
	f, err := readForm(r, message)
	if err != nil {
		return err
	}
	action := f.Enum("action", validate.Rule{}, []string{"approve", "reject"})
	rejection := f.Str("rejection_reason", nullish, validate.StrOpts{Max: 500})
	if err := formErr(f, message); err != nil {
		return err
	}
	a, err := h.svc.Actor(r.Context(), u)
	if err != nil {
		return err
	}
	out, err := h.svc.DecideAttendanceRequest(r.Context(), a, id, *action, rejection, auditMeta(r))
	if err != nil {
		return err
	}
	return ok(w, out)
}
```

`backend/internal/modules/hris/routes.go`, di blok attendance requests (Task 2) tambahkan:

```go
		r("GET /api/hris/attendance-requests", h.withMenu(attendanceManagers, h.listAttendanceRequests)),
		r("POST /api/hris/attendance-requests/{id}/decide", h.withMenu(attendanceManagers, h.decideAttendanceRequest)),
```

`backend/internal/modules/hris/selfservice.go`, di `NavBadges` setelah blok `if a.IsHR { … }`:

```go
	// HR Absensi: employee attendance requests waiting for HRD (EPIC-065 R2).
	if a.IsHRD {
		n, err := s.repo.PendingAttendanceRequestCount(ctx)
		if err != nil {
			return nil, err
		}
		badges.Set(attendanceHref, n)
	}
```

Run: `cd backend && PATH=$HOME/.local/go/bin:$PATH make generate`
Expected: `go-routes.generated.json` memuat `GET /api/hris/attendance-requests` dan `POST /api/hris/attendance-requests/{id}/decide` (sudah tercakup `goNativePrefixes` dari Task 2).

- [ ] **Step 5: Jalankan test, pastikan lulus**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris/ -run 'TestAttendanceRequest|TestAttendancePeriodClosing|TestTeamApprovalsQueueAndBadge' -count=1 && gofmt -l internal/modules/hris`
Expected: `TestAttendanceRequestCreate`, `TestAttendanceRequestDecision`, `TestTeamApprovalsQueueAndBadge` PASS; `TestAttendancePeriodClosing` sama dengan baseline (penyesuaian langsung HR tidak berubah perilakunya); `gofmt` tanpa output.

Run: `cd frontend && npx vitest run src/lib/backend-routes.test.ts`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/modules/hris/attendance_adjust.go backend/internal/modules/hris/attendance_requests.go \
  backend/internal/modules/hris/attendance_request_decision_integration_test.go backend/internal/modules/hris/routes.go \
  backend/internal/modules/hris/selfservice.go frontend/src/lib/go-routes.generated.json
git commit -m "feat(hris): antrean & keputusan HR pengajuan absensi lewat jalur penyesuaian HR (EPIC-065)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: Absen terlewat di periode terbuka (Go)

**Files:**
- Create: `backend/internal/modules/hris/attendance_gaps.go`
- Modify: `backend/internal/modules/hris/service.go` (embed `AttendanceGapRepo`)
- Modify: `backend/internal/modules/hris/routes.go` (blok self service)
- Modify: `frontend/src/lib/go-routes.generated.json`, `frontend/src/lib/backend-routes.test.ts`
- Test: `backend/internal/modules/hris/attendance_gaps_integration_test.go`

**Interfaces:**
- Consumes: `domain.AttendanceGaps`, `domain.GapInput`, `domain.RecordedDay`, `domain.AttendanceGap` (Task 1); `periodContaining` (`attendance_periods.go`); `ShiftPattern` (`files.go`), `ActiveHolidays` (`postgres_leaves.go`); `attReqBody` (Task 2 test).
- Produces:
  - `func (s *Service) openAttendanceWindow(ctx context.Context) (start, today string, err error)` — awal periode tertua yang belum di-closing di antara periode sebelumnya dan periode berjalan (dipakai Task 5).
  - `type AttendanceGapRepo interface { AttendanceDaysInRange(ctx, employeeID, start, end string) ([]*Row, error); ApprovedLeaveRanges(ctx, employeeID, start, end string) ([]*Row, error); PendingAttendanceRequestDates(ctx, employeeID, start, end string) ([]string, error) }`
  - `func (s *Service) MyAttendanceGaps(ctx context.Context, a *Actor) (*Row, error)` → `{start, end, days: []domain.AttendanceGap}`
  - Route `GET /api/hris/me/attendance-gaps`.

- [ ] **Step 1: Tulis test yang gagal**

`backend/internal/modules/hris/attendance_gaps_integration_test.go`:

```go
package hris

import "testing"

// EPIC-065 R1 (MOM AB-06, K-9): karyawan melihat absen terlewat miliknya di
// periode absensi yang masih terbuka, sampai kemarin.
func TestAttendanceGaps(t *testing.T) {
	h := newHarness(t)
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 0, closing_offset_days = 3`)
	crew, crewID := h.staff("pos", nil, true, emp{name: "Crew Terlewat"})
	loose, looseID := h.staff("pos", nil, true, emp{name: "Crew Tanpa Jadwal"})
	unlinked, _ := h.staff("pos", nil, false)
	shift := h.scalar(`INSERT INTO hris.shifts (name, start_time, end_time, late_tolerance_minutes)
		VALUES ('Pagi Terlewat', '08:00', '17:00', 10) RETURNING id::text`)
	for dow := 1; dow <= 6; dow++ {
		h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from) VALUES ($1, $2, $3, '2026-01-01')`,
			crewID, dow, shift)
	}
	h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from) VALUES ($1, 7, NULL, '2026-01-01')`, crewID)
	h.exec(`INSERT INTO hris.public_holidays (holiday_date, name) VALUES ('2026-10-02', 'Libur Uji Terlewat')`)
	h.exec(`INSERT INTO hris.attendance (employee_id, date, clock_in, status) VALUES ($1, '2026-10-03', '2026-10-03 08:05+07', 'present')`, crewID)
	h.exec(`INSERT INTO hris.attendance (employee_id, date, clock_in, status) VALUES ($1, '2026-10-01', '2026-10-01 08:00+07', 'present')`, looseID)
	const url = "/api/hris/me/attendance-gaps"
	days := func(c call) []map[string]any {
		t.Helper()
		expect(t, c, 200, "")
		var out []map[string]any
		for _, d := range c.data()["days"].([]any) {
			out = append(out, d.(map[string]any))
		}
		return out
	}

	// Kam 1 Okt tanpa absen, Jum 2 Okt libur, Sab 3 Okt belum pulang, Min 4 Okt libur jadwal;
	// hari ini (Sen 5 Okt) tidak dihitung; September sudah di-closing.
	gaps := h.do(&crew, "GET", url, nil)
	if d := gaps.data(); d["start"] != "2026-10-01" || d["end"] != "2026-10-04" {
		t.Fatalf("window = %s", gaps.raw)
	}
	got := days(gaps)
	if len(got) != 2 || got[0]["date"] != "2026-10-01" || got[0]["kind"] != "missing" || got[0]["clock_in"] != nil ||
		got[1]["date"] != "2026-10-03" || got[1]["kind"] != "no_clock_out" || got[1]["clock_in"] != "08:05" {
		t.Fatalf("gaps = %s", gaps.raw)
	}
	// Cuti yang disetujui menghapus hari itu.
	h.exec(`INSERT INTO hris.leaves (employee_id, leave_type, start_date, end_date, total_days, reason, status)
		VALUES ($1, 'annual', '2026-10-01', '2026-10-01', 1, 'Cuti uji terlewat', 'approved')`, crewID)
	if got = days(h.do(&crew, "GET", url, nil)); len(got) != 1 || got[0]["date"] != "2026-10-03" {
		t.Fatalf("after leave = %v", got)
	}
	// Pengajuan yang menunggu menghapus hari itu.
	expect(t, h.do(&crew, "POST", "/api/hris/attendance-requests", attReqBody("2026-10-03", "", "17:00", "Lupa absen pulang")), 201, "")
	if got = days(h.do(&crew, "GET", url, nil)); len(got) != 0 {
		t.Fatalf("after request = %v", got)
	}
	// Tanpa jadwal: hanya absen pulang yang hilang.
	if got = days(h.do(&loose, "GET", url, nil)); len(got) != 1 || got[0]["date"] != "2026-10-01" || got[0]["kind"] != "no_clock_out" {
		t.Fatalf("no schedule = %v", got)
	}
	// Akun tanpa data karyawan: kosong.
	empty := h.do(&unlinked, "GET", url, nil)
	if got = days(empty); len(got) != 0 || empty.data()["start"] != nil {
		t.Fatalf("unlinked = %s", empty.raw)
	}
	expect(t, h.do(nil, "GET", url, nil), 401, "")
}
```

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris/ -run TestAttendanceGaps -count=1`
Expected: FAIL — status 404 pada `GET /api/hris/me/attendance-gaps`.

- [ ] **Step 3: Implementasi**

`backend/internal/modules/hris/attendance_gaps.go`:

```go
package hris

import (
	"context"
	"net/http"

	"nuhabit/backend/internal/modules/hris/domain"
)

// Absen terlewat (EPIC-065 R1, MOM AB-06, K-9): the employee's own days of
// the open attendance periods without a complete attendance, each an
// invitation to file a request (AB-04).

// AttendanceGapRepo reads what the gap scan needs besides the schedule and
// holidays.
type AttendanceGapRepo interface {
	// AttendanceDaysInRange: date, clock_in, clock_out, status of the
	// employee's rows in [start, end].
	AttendanceDaysInRange(ctx context.Context, employeeID, start, end string) ([]*Row, error)
	// ApprovedLeaveRanges: start_date, end_date of approved leaves
	// overlapping [start, end].
	ApprovedLeaveRanges(ctx context.Context, employeeID, start, end string) ([]*Row, error)
	PendingAttendanceRequestDates(ctx context.Context, employeeID, start, end string) ([]string, error)
}

// openAttendanceWindow is [start, today]: start is the first day of the
// current attendance period, or of the previous one while that is not
// closed yet (between its end and its closing date). Closing goes through
// periodContaining, which also auto-closes a period past its closing date.
func (s *Service) openAttendanceWindow(ctx context.Context) (start, today string, err error) {
	q := s.repo.querier()
	today = domain.TodayWIB(s.now())
	cur, err := periodContaining(ctx, q, today, today)
	if err != nil {
		return "", "", err
	}
	start = cur.Start
	prev, err := periodContaining(ctx, q, domain.AddDaysISO(cur.Start, -1), today)
	if err != nil {
		return "", "", err
	}
	if !prev.Closed() {
		start = prev.Start
	}
	return start, today, nil
}

// MyAttendanceGaps is GET /api/hris/me/attendance-gaps: the window up to
// yesterday (today is still running) and its gaps.
func (s *Service) MyAttendanceGaps(ctx context.Context, a *Actor) (*Row, error) {
	none := []domain.AttendanceGap{}
	if a.EmployeeID == nil {
		return obj("start", nil, "end", nil, "days", none), nil
	}
	start, today, err := s.openAttendanceWindow(ctx)
	if err != nil {
		return nil, err
	}
	end := domain.AddDaysISO(today, -1)
	if end < start {
		return obj("start", start, "end", end, "days", none), nil
	}
	id := *a.EmployeeID
	pattern, err := s.repo.ShiftPattern(ctx, id)
	if err != nil {
		return nil, err
	}
	holidays, err := s.repo.ActiveHolidays(ctx, start, end)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.AttendanceDaysInRange(ctx, id, start, end)
	if err != nil {
		return nil, err
	}
	leaves, err := s.repo.ApprovedLeaveRanges(ctx, id, start, end)
	if err != nil {
		return nil, err
	}
	pending, err := s.repo.PendingAttendanceRequestDates(ctx, id, start, end)
	if err != nil {
		return nil, err
	}
	in := domain.GapInput{Start: start, End: end, Holidays: map[string]bool{}, OnLeave: map[string]bool{},
		Recorded: map[string]domain.RecordedDay{}, Pending: map[string]bool{}}
	for _, r := range pattern {
		in.Schedule = append(in.Schedule, domain.ScheduleRow{DayOfWeek: int(r.Num("day_of_week")), ShiftID: r.StrPtr("shift_id"),
			EffectiveFrom: r.Str("effective_from"), EffectiveTo: r.StrPtr("effective_to")})
	}
	for _, hd := range holidays {
		in.Holidays[hd.Date] = true
	}
	for _, r := range rows {
		rec := domain.RecordedDay{Absent: r.Str("status") == "absent"}
		if t, found := r.Time("clock_in"); found {
			rec.ClockIn = &t
		}
		if t, found := r.Time("clock_out"); found {
			rec.ClockOut = &t
		}
		in.Recorded[r.Str("date")] = rec
	}
	for _, l := range leaves {
		for _, d := range domain.EachDateISO(l.Str("start_date"), l.Str("end_date")) {
			in.OnLeave[d] = true
		}
	}
	for _, d := range pending {
		in.Pending[d] = true
	}
	return obj("start", start, "end", end, "days", domain.AttendanceGaps(in)), nil
}

/* ── SQL ─────────────────────────────────────────────────────────────── */

func (s *store) AttendanceDaysInRange(ctx context.Context, employeeID, start, end string) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT date::text AS date, clock_in, clock_out, status::text AS status
		FROM hris.attendance WHERE employee_id = $1 AND date BETWEEN $2::date AND $3::date`, employeeID, start, end)
}

func (s *store) ApprovedLeaveRanges(ctx context.Context, employeeID, start, end string) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT start_date::text AS start_date, end_date::text AS end_date
		FROM hris.leaves WHERE employee_id = $1 AND status = 'approved'
		  AND start_date <= $3::date AND end_date >= $2::date`, employeeID, start, end)
}

func (s *store) PendingAttendanceRequestDates(ctx context.Context, employeeID, start, end string) ([]string, error) {
	rows, err := queryRows(ctx, s.db, `SELECT date::text AS date FROM hris.attendance_requests
		WHERE employee_id = $1 AND status = 'pending' AND date BETWEEN $2::date AND $3::date`, employeeID, start, end)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Str("date"))
	}
	return out, nil
}

/* ── HTTP ────────────────────────────────────────────────────────────── */

// GET /api/hris/me/attendance-gaps
func (h handlers) myAttendanceGaps(w http.ResponseWriter, r *http.Request, a *Actor) error {
	out, err := h.svc.MyAttendanceGaps(r.Context(), a)
	if err != nil {
		return err
	}
	return ok(w, dataOf(out))
}
```

`backend/internal/modules/hris/service.go`, di `Repository` setelah `AttendanceRequestRepo`:

```go
	AttendanceGapRepo
```

`backend/internal/modules/hris/routes.go`, di blok self service setelah `GET /api/hris/me/attendance-requests`:

```go
		r("GET /api/hris/me/attendance-gaps", h.withActor(h.myAttendanceGaps)),
```

`frontend/src/lib/backend-routes.test.ts`, ganti:

```ts
      "/api/hris/attendance-requests", "/api/hris/me/attendance-requests"];
```

dengan:

```ts
      "/api/hris/attendance-requests", "/api/hris/me/attendance-requests", "/api/hris/me/attendance-gaps"];
```

Run: `cd backend && PATH=$HOME/.local/go/bin:$PATH make generate`
Expected: manifest memuat `GET /api/hris/me/attendance-gaps`.

- [ ] **Step 4: Jalankan test, pastikan lulus**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris/ -run 'TestAttendanceGaps|TestAttendanceRequest' -count=1 && gofmt -l internal/modules/hris`
Expected: PASS; `gofmt` tanpa output.

Run: `cd frontend && npx vitest run src/lib/backend-routes.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/hris/attendance_gaps.go backend/internal/modules/hris/attendance_gaps_integration_test.go \
  backend/internal/modules/hris/service.go backend/internal/modules/hris/routes.go \
  frontend/src/lib/go-routes.generated.json frontend/src/lib/backend-routes.test.ts
git commit -m "feat(hris): absen terlewat di periode absensi terbuka untuk ESS (EPIC-065)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: Jam kerja minimum per cabang + label kurang jam + notifikasi (migrasi + Go)

**Files:**
- Create: `backend/database/migrations/deltas/20261025090100_short_hours.sql`
- Create: `backend/internal/modules/hris/branch_work_hours.go`
- Modify: `backend/internal/modules/hris/service.go` (embed `WorkHoursRepo`)
- Modify: `backend/internal/modules/hris/placements.go:151-169` (`ActiveBranches`, `BranchGeofence`: kolom `min_work_hours`)
- Modify: `backend/internal/modules/hris/postgres_attendance.go:9-17` (`attendanceListColumns`), `:100` (`RosterRows`)
- Modify: `backend/internal/modules/hris/attendance.go:126-135` (`buildRoster`)
- Modify: `backend/internal/modules/hris/routes.go` (blok placements)
- Modify: `frontend/src/lib/go-routes.generated.json`
- Test: `backend/internal/modules/hris/short_hours_integration_test.go`

**Interfaces:**
- Consumes: `domain.ValidateMinWorkHours` (Task 1); `openAttendanceWindow` (Task 4); `placementManagers`, `BranchGeofence` (`placements.go`); `periodWriteError` (`attendance_periods.go`); `audit.Write`.
- Produces:
  - Kolom `configuration.branches.min_work_hours numeric(4,2)`; `hris.attendance.min_work_hours numeric(4,2)`, `hris.attendance.is_short_hours boolean NOT NULL DEFAULT false`; trigger `attendance_short_hours`; `public.fn_notify_attendance_changes` versi baru.
  - `type WorkHoursRepo interface { SaveBranchMinWorkHours(ctx, id string, hours *float64) (*Row, error); RelabelShortHours(ctx, branchID, from string) (int64, error) }`
  - `func (s *Service) SaveBranchMinWorkHours(ctx context.Context, u *auth.User, id string, hours *float64, meta audit.Entry) (*Row, error)`
  - Route `PUT /api/hris/branches/{id}/min-work-hours` → `{ data: { id, code, name, min_work_hours, relabelled }, message }`
  - JSON: `GET /api/hris/attendance` baris + `min_work_hours` (string numeric | null), `is_short_hours`; daily-roster `attendance.is_short_hours` (bool), `attendance.min_work_hours` (number | null); `GET /api/hris/branches` + `min_work_hours`.

- [ ] **Step 1: Tulis migrasi dan terapkan ke DB test**

`backend/database/migrations/deltas/20261025090100_short_hours.sql`:

```sql
-- =============================================================================
-- Kurang jam / pulang lebih cepat (EPIC-065, MOM AB-05, Q-AB-1 → ruling R7–R9).
--   configuration.branches.min_work_hours: jam hadir minimum cabang, dihitung
--     clock-in s.d. clock-out (istirahat termasuk). NULL = tanpa aturan.
--     HQ = 9 diisi HR lewat HRIS → Penempatan → Lokasi Toko, bukan oleh migrasi.
--   hris.attendance.min_work_hours / is_short_hours: label tersimpan di baris
--     absensi, dihitung trigger setiap kali jam masuk/pulang/cabang berubah.
--   public.fn_notify_attendance_changes: notifikasi HR "Jam Kerja Kurang" saat
--     clock-out kini mengikuti label (menggantikan aturan < 7 jam semua cabang).
-- =============================================================================

ALTER TABLE configuration.branches ADD COLUMN IF NOT EXISTS min_work_hours numeric(4,2);

DO $$ BEGIN
  ALTER TABLE configuration.branches ADD CONSTRAINT branches_min_work_hours_check
    CHECK (min_work_hours IS NULL OR (min_work_hours >= 1 AND min_work_hours <= 24));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

COMMENT ON COLUMN configuration.branches.min_work_hours IS
  'EPIC-065 AB-05: jam hadir minimum (clock-in s.d. clock-out); di bawahnya absen berlabel "Kurang jam / pulang lebih cepat". NULL = tanpa aturan.';

ALTER TABLE hris.attendance
  ADD COLUMN IF NOT EXISTS min_work_hours numeric(4,2),
  ADD COLUMN IF NOT EXISTS is_short_hours boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN hris.attendance.is_short_hours IS
  'EPIC-065 AB-05: clock_out - clock_in < jam minimum cabang absen (snapshot di min_work_hours).';

CREATE OR REPLACE FUNCTION hris.attendance_short_hours()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
BEGIN
  -- Baris yang jam & cabangnya tidak berubah mempertahankan labelnya,
  -- termasuk pelabelan ulang eksplisit saat HR mengubah jam minimum cabang.
  IF TG_OP = 'UPDATE'
     AND NEW.clock_in IS NOT DISTINCT FROM OLD.clock_in
     AND NEW.clock_out IS NOT DISTINCT FROM OLD.clock_out
     AND NEW.branch_id IS NOT DISTINCT FROM OLD.branch_id THEN
    RETURN NEW;
  END IF;
  NEW.min_work_hours := NULL;
  NEW.is_short_hours := false;
  IF NEW.clock_in IS NOT NULL AND NEW.clock_out IS NOT NULL AND NEW.branch_id IS NOT NULL THEN
    NEW.min_work_hours := (SELECT b.min_work_hours FROM configuration.branches b WHERE b.id = NEW.branch_id);
    NEW.is_short_hours := NEW.min_work_hours IS NOT NULL
      AND NEW.clock_out - NEW.clock_in < make_interval(secs => (NEW.min_work_hours * 3600)::double precision);
  END IF;
  RETURN NEW;
END;
$function$;

DROP TRIGGER IF EXISTS attendance_short_hours ON hris.attendance;
CREATE TRIGGER attendance_short_hours BEFORE INSERT OR UPDATE ON hris.attendance
  FOR EACH ROW EXECUTE FUNCTION hris.attendance_short_hours();

-- Sama dengan bootstrap 00000000001000_functions.sql kecuali cabang clock-out.
CREATE OR REPLACE FUNCTION public.fn_notify_attendance_changes()
 RETURNS trigger
 LANGUAGE plpgsql
 SECURITY DEFINER
AS $function$
DECLARE
  v_employee_name text;
  v_employee_user_id uuid;
BEGIN
  SELECT full_name, user_id INTO v_employee_name, v_employee_user_id
  FROM employees WHERE id = COALESCE(NEW.employee_id, OLD.employee_id);

  -- Clock-in baru (terlambat saja yang di-notify agar tidak noisy)
  IF TG_OP = 'INSERT' AND NEW.clock_in IS NOT NULL AND NEW.is_late = true THEN
    PERFORM notify_hrd(
      '⏰ Karyawan Terlambat',
      v_employee_name || ' clock-in terlambat ' || COALESCE(NEW.late_minutes, 0) || ' menit (' ||
        to_char(NEW.clock_in AT TIME ZONE 'Asia/Jakarta', 'HH24:MI') || ' WIB).',
      'alert',
      '/dashboard/hris/attendance'
    );

  -- Clock-out: hanya baris berlabel kurang jam (cabang dengan jam minimum,
  -- EPIC-065 R9), menggantikan aturan lama work_hours < 7 semua cabang.
  ELSIF TG_OP = 'UPDATE' AND OLD.clock_out IS NULL AND NEW.clock_out IS NOT NULL THEN
    IF NEW.is_short_hours THEN
      PERFORM notify_hrd(
        '⚠️ Jam Kerja Kurang: ' || v_employee_name,
        v_employee_name || ' hadir ' ||
          floor(EXTRACT(EPOCH FROM (NEW.clock_out - NEW.clock_in)) / 3600)::int || ' jam ' ||
          (floor(EXTRACT(EPOCH FROM (NEW.clock_out - NEW.clock_in)) / 60)::int % 60) ||
          ' menit, di bawah jam minimum ' || replace(trim_scale(NEW.min_work_hours)::text, '.', ',') ||
          ' jam (kurang jam / pulang lebih cepat).',
        'alert',
        '/dashboard/hris/attendance'
      );
    END IF;

  -- DELETE absensi → notify HRD
  ELSIF TG_OP = 'DELETE' THEN
    PERFORM notify_hrd(
      '🗑️ Data Absensi Dihapus',
      'Data absensi ' || v_employee_name || ' tanggal ' ||
        to_char(OLD.date::date, 'DD Mon YYYY') || ' telah dihapus.',
      'alert',
      '/dashboard/hris/attendance'
    );
  END IF;

  RETURN COALESCE(NEW, OLD);
END;
$function$;
```

Run: `cd backend && MIGRATE_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go run ./cmd/migrate -apply`
Expected: `20261025090100_short_hours.sql` diterapkan tanpa error. Cek: `psql 'postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' -Atc "SELECT tgname FROM pg_trigger WHERE tgrelid = 'hris.attendance'::regclass AND tgname = 'attendance_short_hours'"` → `attendance_short_hours`.

- [ ] **Step 2: Tulis test yang gagal**

`backend/internal/modules/hris/short_hours_integration_test.go`:

```go
package hris

import (
	"strings"
	"testing"
)

// EPIC-065 R7–R9 (MOM AB-05): jam kerja minimum per cabang; label di baris
// absensi; notifikasi HR mengikuti label.
func TestShortHoursLabel(t *testing.T) {
	h := newHarness(t)
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 0, closing_offset_days = 3`)
	hr, _ := h.staff("hrd", []string{"hris.master.placements"}, true)
	staff, _ := h.staff("pos", nil, true)
	_, crewID := h.staff("pos", nil, true, emp{name: "Crew Kantor Pusat"})
	hq, store := h.branch("Kantor Pusat Jam"), h.branch("Toko Tanpa Minimum")
	path := "/api/hris/branches/" + hq + "/min-work-hours"
	att := func(date, in string, out *string, branch string) string {
		t.Helper()
		return h.scalar(`INSERT INTO hris.attendance (employee_id, date, clock_in, clock_out, status, branch_id)
			VALUES ($1, $2::text::date, $3::text::timestamptz, $4::text::timestamptz, 'present', $5) RETURNING id::text`,
			crewID, date, in, out, branch)
	}
	label := func(id string) string {
		t.Helper()
		return h.scalar(`SELECT is_short_hours::text || '|' || COALESCE(min_work_hours::text, '') FROM hris.attendance WHERE id = $1`, id)
	}

	// Baris sebelum aturan ada: September (sudah di-closing) dan Oktober (terbuka).
	h.exec(`SET LOCAL hris.allow_closed_period_write = 'on'`)
	sep := att("2026-09-29", "2026-09-29 09:00+07", ptr("2026-09-29 17:00+07"), hq)
	h.exec(`SET LOCAL hris.allow_closed_period_write = 'off'`)
	early := att("2026-10-01", "2026-10-01 09:00+07", ptr("2026-10-01 17:30+07"), hq)
	if got := label(early); got != "false|" {
		t.Fatalf("before rule = %q", got)
	}

	// Guard & validasi.
	expect(t, h.do(&staff, "PUT", path, map[string]any{"min_work_hours": 9}), 403, "")
	expect(t, h.do(&hr, "PUT", path, map[string]any{"min_work_hours": 0.5}), 400, "Jam kerja minimum 1–24 jam (kelipatan 0,25)")
	expect(t, h.do(&hr, "PUT", path, map[string]any{"min_work_hours": 9.1}), 400, "Jam kerja minimum 1–24 jam (kelipatan 0,25)")
	expect(t, h.do(&hr, "PUT", "/api/hris/branches/bukan-uuid/min-work-hours", map[string]any{"min_work_hours": 9}), 400,
		"ID toko tidak valid")
	expect(t, h.do(&hr, "PUT", "/api/hris/branches/ffffffff-ffff-ffff-ffff-ffffffffffff/min-work-hours",
		map[string]any{"min_work_hours": 9}), 404, "Toko tidak ditemukan")

	// HQ = 9 (id huruf besar dikanonikkan): baris periode terbuka dilabel ulang, September tidak.
	saved := h.do(&hr, "PUT", "/api/hris/branches/"+strings.ToUpper(hq)+"/min-work-hours", map[string]any{"min_work_hours": 9})
	expect(t, saved, 200, "")
	if d := saved.data(); d["min_work_hours"] != "9.00" || d["relabelled"] != 1.0 || d["id"] != hq ||
		saved.body["message"] != "Jam kerja minimum disimpan" {
		t.Fatalf("saved = %s", saved.raw)
	}
	if got := label(early); got != "true|9.00" {
		t.Fatalf("relabelled = %q", got)
	}
	if got := label(sep); got != "false|" {
		t.Fatalf("closed period relabelled: %q", got)
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'branch_min_hours.update'
		AND entity_id = $1 AND after->>'min_work_hours' = '9.00' AND before->>'min_work_hours' IS NULL`, hq); n != "1" {
		t.Fatalf("audit = %s", n)
	}

	// Batas saat clock-out (trigger): tepat 9 jam tidak berlabel, 8 jam 59 menit berlabel.
	exact := att("2026-10-02", "2026-10-02 09:00+07", nil, hq)
	short := att("2026-10-03", "2026-10-03 09:00+07", nil, hq)
	h.exec(`UPDATE hris.attendance SET clock_out = '2026-10-02 18:00+07' WHERE id = $1`, exact)
	h.exec(`UPDATE hris.attendance SET clock_out = '2026-10-03 17:59+07' WHERE id = $1`, short)
	if got := label(exact); got != "false|9.00" {
		t.Fatalf("exactly 9 h = %q", got)
	}
	if got := label(short); got != "true|9.00" {
		t.Fatalf("8 h 59 m = %q", got)
	}
	// Cabang tanpa jam minimum: tidak berlabel, juga di bawah 7 jam (R9).
	shop := att("2026-10-04", "2026-10-04 09:00+07", nil, store)
	h.exec(`UPDATE hris.attendance SET clock_out = '2026-10-04 15:00+07' WHERE id = $1`, shop)
	if got := label(shop); got != "false|" {
		t.Fatalf("store without minimum = %q", got)
	}
	// Notifikasi HR hanya untuk baris berlabel.
	if n := h.scalar(`SELECT count(*)::text FROM public.notifications WHERE user_id = $1 AND title = '⚠️ Jam Kerja Kurang: Crew Kantor Pusat'`,
		hr.UserID); n != "1" {
		t.Fatalf("short-hours notifications = %s", n)
	}
	if msg := h.scalar(`SELECT message FROM public.notifications WHERE user_id = $1 AND title = '⚠️ Jam Kerja Kurang: Crew Kantor Pusat'`,
		hr.UserID); msg != "Crew Kantor Pusat hadir 8 jam 59 menit, di bawah jam minimum 9 jam (kurang jam / pulang lebih cepat)." {
		t.Fatalf("notification message = %q", msg)
	}

	// API: Rekap, Monitoring, dan daftar toko memuat label/jam minimum.
	list := h.do(&hr, "GET", "/api/hris/attendance?employee_id="+crewID+"&start_date=2026-10-01&end_date=2026-10-04", nil)
	expect(t, list, 200, "")
	found := false
	for _, r := range list.list() {
		if row := r.(map[string]any); row["id"] == short {
			found = row["is_short_hours"] == true && row["min_work_hours"] == "9.00"
		}
	}
	if !found {
		t.Fatalf("list = %s", list.raw)
	}
	roster := h.do(&hr, "GET", "/api/hris/attendance/daily-roster?date=2026-10-03", nil)
	expect(t, roster, 200, "")
	var rosterAtt map[string]any
	for _, e := range roster.data()["employees"].([]any) {
		if row := e.(map[string]any); row["employee_id"] == crewID {
			rosterAtt, _ = row["attendance"].(map[string]any)
		}
	}
	if rosterAtt == nil || rosterAtt["is_short_hours"] != true || rosterAtt["min_work_hours"] != 9.0 {
		t.Fatalf("roster attendance = %v", rosterAtt)
	}
	branchMin := ""
	for _, b := range h.do(&staff, "GET", "/api/hris/branches", nil).list() {
		if row := b.(map[string]any); row["id"] == hq {
			branchMin, _ = row["min_work_hours"].(string)
		}
	}
	if branchMin != "9.00" {
		t.Fatalf("branch list min = %q", branchMin)
	}

	// Dikosongkan: baris periode terbuka kembali tanpa label.
	cleared := h.do(&hr, "PUT", path, map[string]any{"min_work_hours": nil})
	expect(t, cleared, 200, "")
	if d := cleared.data(); d["min_work_hours"] != nil || d["relabelled"] != 3.0 {
		t.Fatalf("cleared = %s", cleared.raw)
	}
	if got := label(short); got != "false|" {
		t.Fatalf("after clearing = %q", got)
	}
}
```

- [ ] **Step 3: Jalankan test, pastikan gagal**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris/ -run TestShortHoursLabel -count=1`
Expected: FAIL — `PUT /api/hris/branches/{id}/min-work-hours` 404/405 (trigger di Step 1 sudah aktif, endpoint belum).

- [ ] **Step 4: Implementasi**

`backend/internal/modules/hris/branch_work_hours.go`:

```go
package hris

import (
	"context"
	"net/http"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/validate"
)

// Jam kerja minimum per cabang (EPIC-065 R7–R8, MOM AB-05): the label
// itself is written by the hris.attendance_short_hours trigger.

// WorkHoursRepo writes the store minimum and relabels attendance.
type WorkHoursRepo interface {
	SaveBranchMinWorkHours(ctx context.Context, id string, hours *float64) (*Row, error)
	// RelabelShortHours recomputes the label of the store's rows dated from
	// "from" on (the open attendance window), returning the rows changed.
	RelabelShortHours(ctx context.Context, branchID, from string) (int64, error)
}

// SaveBranchMinWorkHours is PUT /api/hris/branches/{id}/min-work-hours: the
// store's minimum presence; nil removes the rule. The store's rows in the
// open attendance window are relabelled; closed periods keep their label.
func (s *Service) SaveBranchMinWorkHours(ctx context.Context, u *auth.User, id string, hours *float64, meta audit.Entry) (*Row, error) {
	if msg := domain.ValidateMinWorkHours(hours); msg != "" {
		return nil, httpx.BadRequest(msg)
	}
	from, _, err := s.openAttendanceWindow(ctx)
	if err != nil {
		return nil, err
	}
	var row *Row
	err = s.repo.InTx(ctx, func(r Repository) error {
		before, err := r.BranchGeofence(ctx, id)
		if err != nil {
			return err
		}
		if before == nil {
			return httpx.NotFound("Toko tidak ditemukan")
		}
		if row, err = r.SaveBranchMinWorkHours(ctx, id, hours); err != nil {
			return err
		}
		n, err := r.RelabelShortHours(ctx, id, from)
		if err != nil {
			return err
		}
		row.Set("relabelled", n)
		meta.ActorID, meta.ActorName = u.ID, ptr(u.FullName)
		meta.Action, meta.Entity, meta.EntityID = "branch_min_hours.update", "configuration.branches", ptr(id)
		meta.EntityLabel = ptr(before.Str("name"))
		meta.Before = map[string]any{"min_work_hours": before.Get("min_work_hours")}
		meta.After = map[string]any{"min_work_hours": row.Get("min_work_hours")}
		return audit.Write(ctx, r.querier(), meta)
	})
	if err != nil {
		return nil, err
	}
	return obj("data", row, "message", "Jam kerja minimum disimpan"), nil
}

func (s *store) SaveBranchMinWorkHours(ctx context.Context, id string, hours *float64) (*Row, error) {
	return queryRow(ctx, s.db, `UPDATE configuration.branches SET min_work_hours = $2, updated_at = now()
		WHERE id = $1 RETURNING id, code, name, min_work_hours`, id, hours)
}

// RelabelShortHours sets the label explicitly; the trigger keeps explicit
// values when clock and store do not change.
func (s *store) RelabelShortHours(ctx context.Context, branchID, from string) (int64, error) {
	tag, err := s.db.Exec(ctx, `UPDATE hris.attendance a SET
		  min_work_hours = b.min_work_hours,
		  is_short_hours = b.min_work_hours IS NOT NULL
		    AND a.clock_out - a.clock_in < make_interval(secs => (b.min_work_hours * 3600)::double precision)
		FROM configuration.branches b
		WHERE b.id = $1 AND a.branch_id = b.id AND a.date >= $2::date
		  AND a.clock_in IS NOT NULL AND a.clock_out IS NOT NULL
		  AND a.min_work_hours IS DISTINCT FROM b.min_work_hours`, branchID, from)
	if err != nil {
		return 0, periodWriteError(err)
	}
	return tag.RowsAffected(), nil
}

// PUT /api/hris/branches/{id}/min-work-hours { min_work_hours: number | null }
func (h handlers) putBranchMinWorkHours(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	id, valid := validate.CanonicalUUID(r.PathValue("id"))
	if !valid {
		return httpx.BadRequest("ID toko tidak valid")
	}
	const message = "Validation failed"
	f, err := readForm(r, message)
	if err != nil {
		return err
	}
	hours := f.Num("min_work_hours", nullish, validate.NumOpts{})
	if err := formErr(f, message); err != nil {
		return err
	}
	out, err := h.svc.SaveBranchMinWorkHours(r.Context(), u, id, hours, auditMeta(r))
	if err != nil {
		return err
	}
	return ok(w, out)
}
```

`backend/internal/modules/hris/service.go`, di `Repository` setelah `AttendanceGapRepo`:

```go
	WorkHoursRepo
```

`backend/internal/modules/hris/placements.go`, ganti:

```go
	return queryRows(ctx, s.db, `SELECT b.id, b.code, b.name, b.company_id, c.name AS company_name, b.lat, b.lng,
		b.geofence_radius_m
```

dengan:

```go
	return queryRows(ctx, s.db, `SELECT b.id, b.code, b.name, b.company_id, c.name AS company_name, b.lat, b.lng,
		b.geofence_radius_m, b.min_work_hours
```

dan ganti:

```go
	row, err := queryRow(ctx, s.db, `SELECT name, lat, lng, geofence_radius_m FROM configuration.branches WHERE id::text = $1`, id)
```

dengan:

```go
	row, err := queryRow(ctx, s.db, `SELECT name, lat, lng, geofence_radius_m, min_work_hours FROM configuration.branches WHERE id::text = $1`, id)
```

`backend/internal/modules/hris/postgres_attendance.go`, ganti:

```go
  clock_in_geofence, clock_in_distance_m, clock_out_geofence, clock_out_distance_m, ` +
```

dengan:

```go
  clock_in_geofence, clock_in_distance_m, clock_out_geofence, clock_out_distance_m,
  min_work_hours, is_short_hours, ` +
```

dan di `RosterRows` ganti:

```go
		       a.clock_in_geofence, a.clock_in_distance_m, a.clock_out_geofence, a.clock_out_distance_m,
		       a.notes AS attendance_notes,
```

dengan:

```go
		       a.clock_in_geofence, a.clock_in_distance_m, a.clock_out_geofence, a.clock_out_distance_m,
		       a.notes AS attendance_notes, a.min_work_hours::text AS min_work_hours, a.is_short_hours,
```

`backend/internal/modules/hris/attendance.go`, di `buildRoster` ganti:

```go
			var hours any
			if wh := row.Str("work_hours"); wh != "" {
				hours = domain.JSNumber(wh)
			}
```

dengan:

```go
			var hours, minHours any
			if wh := row.Str("work_hours"); wh != "" {
				hours = domain.JSNumber(wh)
			}
			if mh := row.Str("min_work_hours"); mh != "" {
				minHours = domain.JSNumber(mh)
			}
```

dan ganti:

```go
				"notes", row.Get("attendance_notes"))
```

dengan:

```go
				"notes", row.Get("attendance_notes"),
				"is_short_hours", row.Bool("is_short_hours"), "min_work_hours", minHours)
```

`backend/internal/modules/hris/routes.go`, setelah `r("PUT /api/hris/branches/{id}/location", h.withMenu(placementManagers, h.putBranchLocation)),`:

```go
		r("PUT /api/hris/branches/{id}/min-work-hours", h.withMenu(placementManagers, h.putBranchMinWorkHours)),
```

Run: `cd backend && PATH=$HOME/.local/go/bin:$PATH make generate`
Expected: manifest memuat `PUT /api/hris/branches/{id}/min-work-hours` (tercakup prefix `/api/hris/branches`).

- [ ] **Step 5: Jalankan test, pastikan lulus**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris/ -run 'TestShortHoursLabel|TestBranchLocation|TestGeofenceInRosterAndList|TestClockInOut|TestAttendanceRequest|TestAttendanceGaps' -count=1 && gofmt -l internal/modules/hris`
Expected: PASS; `gofmt` tanpa output.

Run: `cd frontend && npx vitest run src/lib/backend-routes.test.ts`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/database/migrations/deltas/20261025090100_short_hours.sql backend/internal/modules/hris/branch_work_hours.go \
  backend/internal/modules/hris/short_hours_integration_test.go backend/internal/modules/hris/service.go \
  backend/internal/modules/hris/placements.go backend/internal/modules/hris/postgres_attendance.go \
  backend/internal/modules/hris/attendance.go backend/internal/modules/hris/routes.go frontend/src/lib/go-routes.generated.json
git commit -m "feat(hris): jam kerja minimum per cabang + label kurang jam di absensi (EPIC-065 AB-05)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 6: Lib & lapisan data frontend (pengajuan, absen terlewat, kurang jam)

**Files:**
- Create: `frontend/src/lib/hris/attendance-requests.ts`, `frontend/src/lib/hris/attendance-requests.test.ts`
- Create: `frontend/src/lib/hris/short-hours.ts`, `frontend/src/lib/hris/short-hours.test.ts`
- Create: `frontend/src/features/hris/attendance-requests/types.ts`, `api.ts`, `query-keys.ts`, `queries.ts`, `mutations.ts`, `index.ts`
- Test: `frontend/src/features/hris/attendance-requests/attendance-requests.api.test.tsx`

**Interfaces:**
- Consumes: `canManageTrips` (`lib/hris/business-trips.ts`); `apiGet`, `apiPost`, `buildListUrl` (`lib/api-client`); `attendanceQueryKeys` (`features/hris/attendance/query-keys`); endpoint Go Task 2–4.
- Produces:
  - `lib/hris/attendance-requests.ts`: `type AttendanceRequestStatus`, `AttendanceRequestKind`, `AttendanceGapKind`; `ATTENDANCE_REQUEST_STATUS_META`, `ATTENDANCE_REQUEST_KIND_LABEL`, `ATTENDANCE_GAP_LABEL`; `canDecideAttendanceRequests(role)`; `interface AttendanceRequestForm`, `EMPTY_ATTENDANCE_REQUEST_FORM`; `validateAttendanceRequestForm(form, today): string`; `toAttendanceRequestPayload(form, photo?)`; `attendanceRequestClocks(r)`; `gapHint(kind, clockIn)`.
  - `lib/hris/short-hours.ts`: `SHORT_HOURS_LABEL`, `MIN_WORK_HOURS_ERROR`, `formatHours(v)`, `shortHoursDetail(min)`, `validateMinWorkHours(input)`, `toMinWorkHoursPayload(input)`.
  - Fitur: tipe `AttendanceRequest`, `AttendanceRequestPayload`, `AttendanceRequestAction`, `AttendanceRequestFilter`, `AttendanceGapDay`, `AttendanceGaps`; API `fetchMyAttendanceRequests`, `fetchAttendanceRequests`, `fetchMyAttendanceGaps`, `createAttendanceRequest`, `decideAttendanceRequest`, `cancelAttendanceRequest`, `attendanceRequestAttachmentUrl`; `attendanceRequestQueryKeys`; hook `useMyAttendanceRequests`, `useAttendanceRequests`, `useMyAttendanceGaps`, `useCreateAttendanceRequest`, `useCancelAttendanceRequest`, `useDecideAttendanceRequest`.

- [ ] **Step 1: Tulis test yang gagal**

`frontend/src/lib/hris/attendance-requests.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import {
  attendanceRequestClocks,
  canDecideAttendanceRequests,
  gapHint,
  toAttendanceRequestPayload,
  validateAttendanceRequestForm,
} from "./attendance-requests";

const valid = { date: "2026-10-02", clock_in: "08:00", clock_out: "", reason: "Internet toko mati" };

describe("validateAttendanceRequestForm (pesan sama dengan server)", () => {
  it.each([
    [{ ...valid, date: "" }, "Tanggal wajib diisi"],
    [{ ...valid, date: "2026-10-06" }, "Tanggal pengajuan tidak boleh di masa depan"],
    [{ ...valid, reason: "  Lupa " }, "Keterangan wajib diisi (minimal 5 karakter)"],
    [{ ...valid, clock_in: "" }, "Isi jam masuk dan/atau jam pulang yang diajukan"],
    [valid, ""],
    [{ ...valid, date: "2026-10-05" }, ""],
  ])("%o → %s", (form, msg) => {
    expect(validateAttendanceRequestForm(form, "2026-10-05")).toBe(msg);
  });
});

describe("toAttendanceRequestPayload", () => {
  it("jam kosong menjadi null, keterangan dipangkas, foto hanya bila ada", () => {
    expect(toAttendanceRequestPayload({ ...valid, reason: "  Internet toko mati  " })).toEqual({
      date: "2026-10-02",
      clock_in: "08:00",
      clock_out: null,
      reason: "Internet toko mati",
    });
    expect(toAttendanceRequestPayload(valid, "data:image/png;base64,AA").photo).toBe("data:image/png;base64,AA");
  });
});

describe("label & hak", () => {
  it("hanya HRD (super_admin/admin/hrd) yang memutus", () => {
    expect(["super_admin", "admin", "hrd"].map(canDecideAttendanceRequests)).toEqual([true, true, true]);
    expect([canDecideAttendanceRequests("hiring_manager"), canDecideAttendanceRequests(null)]).toEqual([false, false]);
  });

  it("ringkasan jam dan petunjuk absen terlewat", () => {
    expect(attendanceRequestClocks({ clock_in: "08:00", clock_out: "17:00" })).toBe("Masuk 08:00 · Pulang 17:00");
    expect(attendanceRequestClocks({ clock_in: null, clock_out: "17:00" })).toBe("Pulang 17:00");
    expect(gapHint("no_clock_out", "08:05")).toBe("Tercatat masuk 08:05 tanpa absen pulang — isi jam pulang yang benar.");
    expect(gapHint("missing", null)).toBe("Tidak ada absen tercatat — isi jam masuk dan jam pulang yang benar.");
  });
});
```

`frontend/src/lib/hris/short-hours.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import {
  MIN_WORK_HOURS_ERROR,
  SHORT_HOURS_LABEL,
  formatHours,
  shortHoursDetail,
  toMinWorkHoursPayload,
  validateMinWorkHours,
} from "./short-hours";

describe("kurang jam (AB-05)", () => {
  it("memformat jam numeric pg", () => {
    expect(formatHours("9.00")).toBe("9");
    expect(formatHours("8.50")).toBe("8,5");
    expect(formatHours(9)).toBe("9");
    expect(formatHours(null)).toBe("");
    expect(formatHours(undefined)).toBe("");
  });

  it("keterangan label", () => {
    expect(SHORT_HOURS_LABEL).toBe("Kurang jam / pulang lebih cepat");
    expect(shortHoursDetail("9.00")).toBe("Hadir kurang dari 9 jam (jam minimum toko)");
    expect(shortHoursDetail(null)).toBe(SHORT_HOURS_LABEL);
  });

  it.each([
    ["", ""],
    ["9", ""],
    ["8,5", ""],
    ["8.25", ""],
    ["0.5", MIN_WORK_HOURS_ERROR],
    ["25", MIN_WORK_HOURS_ERROR],
    ["9.1", MIN_WORK_HOURS_ERROR],
    ["abc", MIN_WORK_HOURS_ERROR],
  ])("validasi %s → %s", (input, msg) => {
    expect(validateMinWorkHours(input)).toBe(msg);
  });

  it("payload: kosong = tanpa aturan", () => {
    expect(toMinWorkHoursPayload(" ")).toEqual({ min_work_hours: null });
    expect(toMinWorkHoursPayload("8,5")).toEqual({ min_work_hours: 8.5 });
  });
});
```

`frontend/src/features/hris/attendance-requests/attendance-requests.api.test.tsx`:

```tsx
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { attendanceQueryKeys } from "@/features/hris/attendance/query-keys";
import {
  attendanceRequestAttachmentUrl,
  cancelAttendanceRequest,
  createAttendanceRequest,
  decideAttendanceRequest,
  fetchAttendanceRequests,
  fetchMyAttendanceGaps,
  fetchMyAttendanceRequests,
} from "./api";
import { useCreateAttendanceRequest, useDecideAttendanceRequest } from "./mutations";
import { attendanceRequestQueryKeys } from "./query-keys";

const apiGet = vi.fn();
const apiPost = vi.fn();
vi.mock("@/lib/api-client", async (orig) => ({
  ...(await orig<typeof import("@/lib/api-client")>()),
  apiGet: (u: string) => apiGet(u),
  apiPost: (u: string, b: unknown) => apiPost(u, b),
}));

afterEach(() => {
  apiGet.mockReset();
  apiPost.mockReset();
});

function wrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const spy = vi.spyOn(qc, "invalidateQueries");
  const Wrap = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  return { Wrap, spy };
}

describe("attendance-requests api", () => {
  it("menyusun URL antrean HR, milik sendiri, dan absen terlewat", async () => {
    apiGet.mockResolvedValue({ data: [] });
    await fetchAttendanceRequests({ status: "pending" });
    expect(apiGet).toHaveBeenLastCalledWith("/api/hris/attendance-requests?status=pending");
    await fetchMyAttendanceRequests();
    expect(apiGet).toHaveBeenLastCalledWith("/api/hris/me/attendance-requests");
    apiGet.mockResolvedValue({ data: { start: "2026-10-01", end: "2026-10-04", days: [] } });
    expect(await fetchMyAttendanceGaps()).toEqual({ start: "2026-10-01", end: "2026-10-04", days: [] });
    expect(apiGet).toHaveBeenLastCalledWith("/api/hris/me/attendance-gaps");
  });

  it("mengirim pengajuan, keputusan, dan pembatalan ke endpoint Go", async () => {
    apiPost.mockResolvedValue({ data: {} });
    const payload = { date: "2026-10-03", clock_in: null, clock_out: "17:00", reason: "Lupa absen pulang" };
    await createAttendanceRequest(payload);
    expect(apiPost).toHaveBeenLastCalledWith("/api/hris/attendance-requests", payload);
    await decideAttendanceRequest("r1", "reject", "Tidak sesuai CCTV");
    expect(apiPost).toHaveBeenLastCalledWith("/api/hris/attendance-requests/r1/decide", {
      action: "reject",
      rejection_reason: "Tidak sesuai CCTV",
    });
    await cancelAttendanceRequest("r1");
    expect(apiPost).toHaveBeenLastCalledWith("/api/hris/attendance-requests/r1/cancel", {});
    expect(attendanceRequestAttachmentUrl("attendance-requests/e1/a.png")).toBe(
      "/api/hris/attendance-requests/attachment/attendance-requests/e1/a.png"
    );
  });
});

describe("attendance-requests mutations", () => {
  it("keputusan menyegarkan pengajuan, absensi, dan badge menu, juga saat gagal (antrean basi)", async () => {
    apiPost.mockRejectedValue(new Error("Pengajuan sudah diproses oleh orang lain"));
    const { Wrap, spy } = wrapper();
    const { result } = renderHook(() => useDecideAttendanceRequest(), { wrapper: Wrap });
    result.current.mutate({ id: "r1", action: "approve" });
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(spy).toHaveBeenCalledWith({ queryKey: attendanceRequestQueryKeys.all });
    expect(spy).toHaveBeenCalledWith({ queryKey: attendanceQueryKeys.all });
    expect(spy).toHaveBeenCalledWith({ queryKey: ["hris", "nav-badges"] });
  });

  it("pengajuan baru menyegarkan daftar & absen terlewat", async () => {
    apiPost.mockResolvedValue({ data: {} });
    const { Wrap, spy } = wrapper();
    const { result } = renderHook(() => useCreateAttendanceRequest(), { wrapper: Wrap });
    result.current.mutate({ date: "2026-10-03", clock_in: null, clock_out: "17:00", reason: "Lupa absen pulang" });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(spy).toHaveBeenCalledWith({ queryKey: attendanceRequestQueryKeys.all });
  });
});
```

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd frontend && npx vitest run src/lib/hris/attendance-requests.test.ts src/lib/hris/short-hours.test.ts src/features/hris/attendance-requests`
Expected: FAIL — `Failed to resolve import "./attendance-requests"` (dan modul lain).

- [ ] **Step 3: Implementasi**

`frontend/src/lib/hris/attendance-requests.ts`:

```ts
import { canManageTrips } from "./business-trips";

/**
 * Pengajuan absen manual / koreksi absen (EPIC-065, MOM AB-04/AB-06, K-9):
 * status, label, dan validasi form. Pesan sama dengan server
 * (domain.CheckAttendanceRequest). Murni, aman diimpor komponen klien.
 */

export type AttendanceRequestStatus = "pending" | "approved" | "rejected" | "cancelled";
export type AttendanceRequestKind = "manual" | "correction";
export type AttendanceGapKind = "missing" | "no_clock_out";

export const ATTENDANCE_REQUEST_STATUS_META: Record<AttendanceRequestStatus, { label: string; className: string }> = {
  pending: { label: "Menunggu HR", className: "bg-amber-100 text-amber-700" },
  approved: { label: "Disetujui", className: "bg-green-100 text-green-700" },
  rejected: { label: "Ditolak", className: "bg-red-100 text-red-700" },
  cancelled: { label: "Dibatalkan", className: "bg-gray-100 text-gray-600" },
};

export const ATTENDANCE_REQUEST_KIND_LABEL: Record<AttendanceRequestKind, string> = {
  manual: "Absen manual",
  correction: "Koreksi absen",
};

export const ATTENDANCE_GAP_LABEL: Record<AttendanceGapKind, string> = {
  missing: "Tidak ada absen",
  no_clock_out: "Belum absen pulang",
};

/** Pemutus = HRD (Actor.IsHRD: super_admin/admin/hrd), sama dengan Perjalanan Dinas (R2). */
export const canDecideAttendanceRequests = canManageTrips;

export interface AttendanceRequestForm {
  date: string;
  /** "HH:MM" atau "" (tidak diajukan) */
  clock_in: string;
  clock_out: string;
  reason: string;
}

export const EMPTY_ATTENDANCE_REQUEST_FORM: AttendanceRequestForm = { date: "", clock_in: "", clock_out: "", reason: "" };

/** "" bila valid; urutan dan pesan sama dengan server. */
export function validateAttendanceRequestForm(form: AttendanceRequestForm, today: string): string {
  if (!form.date) return "Tanggal wajib diisi";
  if (form.date > today) return "Tanggal pengajuan tidak boleh di masa depan";
  if (form.reason.trim().length < 5) return "Keterangan wajib diisi (minimal 5 karakter)";
  if (!form.clock_in && !form.clock_out) return "Isi jam masuk dan/atau jam pulang yang diajukan";
  return "";
}

/** Body POST /api/hris/attendance-requests: jam kosong = null (tidak diajukan). */
export function toAttendanceRequestPayload(form: AttendanceRequestForm, photo?: string) {
  return {
    date: form.date,
    clock_in: form.clock_in || null,
    clock_out: form.clock_out || null,
    reason: form.reason.trim(),
    ...(photo ? { photo } : {}),
  };
}

/** "Masuk 08:00 · Pulang 17:00" (hanya jam yang diajukan). */
export function attendanceRequestClocks(r: { clock_in: string | null; clock_out: string | null }): string {
  return [r.clock_in && `Masuk ${r.clock_in}`, r.clock_out && `Pulang ${r.clock_out}`].filter(Boolean).join(" · ");
}

/** Petunjuk form saat dibuka dari kartu "Absen terlewat". */
export function gapHint(kind: AttendanceGapKind, clockIn: string | null): string {
  return kind === "no_clock_out"
    ? `Tercatat masuk ${clockIn ?? "—"} tanpa absen pulang — isi jam pulang yang benar.`
    : "Tidak ada absen tercatat — isi jam masuk dan jam pulang yang benar.";
}
```

`frontend/src/lib/hris/short-hours.ts`:

```ts
/**
 * Label "Kurang jam / pulang lebih cepat" dan jam kerja minimum cabang
 * (EPIC-065, MOM AB-05). Murni, aman diimpor komponen klien. Label sendiri
 * dihitung dan disimpan server (trigger hris.attendance_short_hours).
 */

export const SHORT_HOURS_LABEL = "Kurang jam / pulang lebih cepat";
export const MIN_WORK_HOURS_ERROR = "Jam kerja minimum 1–24 jam (kelipatan 0,25)";

/** "9" / "8,5" dari numeric pg ("9.00") atau angka; "" bila kosong. */
export function formatHours(value: string | number | null | undefined): string {
  if (value === null || value === undefined || value === "") return "";
  const n = Number(value);
  if (!Number.isFinite(n)) return "";
  return n.toLocaleString("id-ID", { maximumFractionDigits: 2 });
}

/** Keterangan badge: "Hadir kurang dari 9 jam (jam minimum toko)". */
export function shortHoursDetail(min: string | number | null | undefined): string {
  const hours = formatHours(min);
  return hours ? `Hadir kurang dari ${hours} jam (jam minimum toko)` : SHORT_HOURS_LABEL;
}

function parseHours(input: string): number {
  return Number(input.trim().replace(",", "."));
}

/** "" bila valid; input kosong = tanpa aturan (sama dengan server). */
export function validateMinWorkHours(input: string): string {
  if (input.trim() === "") return "";
  const n = parseHours(input);
  if (!Number.isFinite(n) || n < 1 || n > 24 || Math.round(n * 4) !== n * 4) return MIN_WORK_HOURS_ERROR;
  return "";
}

/** Body PUT /api/hris/branches/{id}/min-work-hours. */
export function toMinWorkHoursPayload(input: string): { min_work_hours: number | null } {
  return { min_work_hours: input.trim() === "" ? null : parseHours(input) };
}
```

`frontend/src/features/hris/attendance-requests/types.ts`:

```ts
import type { AttendanceGapKind, AttendanceRequestKind, AttendanceRequestStatus } from "@/lib/hris/attendance-requests";

/** Baris hris.attendance_requests (GET /api/hris/attendance-requests, /api/hris/me/attendance-requests). */
export interface AttendanceRequest {
  id: string;
  employee_id: string;
  date: string; // YYYY-MM-DD
  kind: AttendanceRequestKind;
  /** "HH:MM" WIB yang diajukan; null = tidak diajukan */
  clock_in: string | null;
  clock_out: string | null;
  reason: string;
  /** Path privat foto bukti (dibaca lewat attendanceRequestAttachmentUrl) */
  attachment_path: string | null;
  status: AttendanceRequestStatus;
  attendance_id: string | null;
  decided_by: string | null;
  decided_by_name: string | null;
  decided_at: string | null;
  rejection_reason: string | null;
  created_at: string;
  employee: { id: string; full_name: string; nip: string | null } | null;
  /** Absensi yang tercatat pada tanggal itu saat ini; null bila belum ada. */
  recorded: { id: string; clock_in: string | null; clock_out: string | null; status: string } | null;
}

export interface AttendanceRequestPayload {
  date: string;
  clock_in: string | null;
  clock_out: string | null;
  reason: string;
  /** Data URL JPG/PNG/WebP, maks 5 MB (opsional) */
  photo?: string;
}

export type AttendanceRequestAction = "approve" | "reject";

export interface AttendanceRequestFilter {
  status?: AttendanceRequestStatus;
  employee_id?: string;
}

export interface AttendanceGapDay {
  date: string;
  kind: AttendanceGapKind;
  /** Jam masuk tercatat (WIB "HH:MM") untuk "Belum absen pulang". */
  clock_in: string | null;
}

export interface AttendanceGaps {
  start: string | null;
  end: string | null;
  days: AttendanceGapDay[];
}
```

`frontend/src/features/hris/attendance-requests/api.ts`:

```ts
import { apiGet, apiPost, buildListUrl } from "@/lib/api-client";
import type {
  AttendanceGaps,
  AttendanceRequest,
  AttendanceRequestAction,
  AttendanceRequestFilter,
  AttendanceRequestPayload,
} from "./types";

const BASE = "/api/hris/attendance-requests";

export const fetchMyAttendanceRequests = () =>
  apiGet<{ data: AttendanceRequest[] }>("/api/hris/me/attendance-requests").then((res) => res.data ?? []);

export const fetchAttendanceRequests = (filter: AttendanceRequestFilter = {}) =>
  apiGet<{ data: AttendanceRequest[] }>(buildListUrl(BASE, { status: filter.status, employee_id: filter.employee_id })).then(
    (res) => res.data ?? []
  );

export const fetchMyAttendanceGaps = () =>
  apiGet<{ data: AttendanceGaps }>("/api/hris/me/attendance-gaps").then((res) => res.data);

export const createAttendanceRequest = (payload: AttendanceRequestPayload) =>
  apiPost<{ data: AttendanceRequest; message?: string }>(BASE, payload);

export const decideAttendanceRequest = (id: string, action: AttendanceRequestAction, rejectionReason?: string) =>
  apiPost<{ data: AttendanceRequest; message?: string }>(`${BASE}/${id}/decide`, {
    action,
    rejection_reason: rejectionReason,
  });

export const cancelAttendanceRequest = (id: string) =>
  apiPost<{ data: AttendanceRequest; message?: string }>(`${BASE}/${id}/cancel`, {});

/** URL foto bukti (dibaca pemohon dan HR); path dari server. */
export const attendanceRequestAttachmentUrl = (path: string) => `${BASE}/attachment/${path}`;
```

`frontend/src/features/hris/attendance-requests/query-keys.ts`:

```ts
import type { AttendanceRequestFilter } from "./types";

export const attendanceRequestQueryKeys = {
  all: ["hris", "attendance-requests"] as const,
  mine: () => ["hris", "attendance-requests", "mine"] as const,
  list: (filter: AttendanceRequestFilter) => ["hris", "attendance-requests", "list", filter] as const,
  gaps: () => ["hris", "attendance-requests", "gaps"] as const,
};
```

`frontend/src/features/hris/attendance-requests/queries.ts`:

```ts
"use client";

import { useQuery } from "@tanstack/react-query";
import { fetchAttendanceRequests, fetchMyAttendanceGaps, fetchMyAttendanceRequests } from "./api";
import { attendanceRequestQueryKeys } from "./query-keys";
import type { AttendanceRequestFilter } from "./types";

export const useMyAttendanceRequests = () =>
  useQuery({ queryKey: attendanceRequestQueryKeys.mine(), queryFn: fetchMyAttendanceRequests });

export const useAttendanceRequests = (filter: AttendanceRequestFilter = {}) =>
  useQuery({ queryKey: attendanceRequestQueryKeys.list(filter), queryFn: () => fetchAttendanceRequests(filter) });

export const useMyAttendanceGaps = () =>
  useQuery({ queryKey: attendanceRequestQueryKeys.gaps(), queryFn: fetchMyAttendanceGaps });
```

`frontend/src/features/hris/attendance-requests/mutations.ts`:

```ts
"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { attendanceQueryKeys } from "@/features/hris/attendance/query-keys";
import { cancelAttendanceRequest, createAttendanceRequest, decideAttendanceRequest } from "./api";
import { attendanceRequestQueryKeys } from "./query-keys";
import type { AttendanceRequestAction, AttendanceRequestPayload } from "./types";

/** Pengajuan, absensi (persetujuan menulis absensi), dan badge menu. */
function useInvalidateAttendanceRequests() {
  const qc = useQueryClient();
  return () => {
    void qc.invalidateQueries({ queryKey: attendanceRequestQueryKeys.all });
    void qc.invalidateQueries({ queryKey: attendanceQueryKeys.all });
    void qc.invalidateQueries({ queryKey: ["hris", "nav-badges"] });
  };
}

export function useCreateAttendanceRequest() {
  const onSuccess = useInvalidateAttendanceRequests();
  return useMutation({
    mutationFn: (payload: AttendanceRequestPayload) => createAttendanceRequest(payload),
    retry: false,
    onSuccess,
  });
}

/** Disegarkan juga saat gagal (mis. 400 "sudah dibatalkan") agar daftar basi hilang. */
export function useCancelAttendanceRequest() {
  const onSettled = useInvalidateAttendanceRequests();
  return useMutation({ mutationFn: (id: string) => cancelAttendanceRequest(id), retry: false, onSettled });
}

/** Disegarkan juga saat gagal (409 "sudah diproses orang lain") agar antrean basi hilang. */
export function useDecideAttendanceRequest() {
  const onSettled = useInvalidateAttendanceRequests();
  return useMutation({
    mutationFn: (vars: { id: string; action: AttendanceRequestAction; rejectionReason?: string }) =>
      decideAttendanceRequest(vars.id, vars.action, vars.rejectionReason),
    retry: false,
    onSettled,
  });
}
```

`frontend/src/features/hris/attendance-requests/index.ts`:

```ts
export * from "./types";
export * from "./api";
export * from "./query-keys";
export * from "./queries";
export * from "./mutations";
```

- [ ] **Step 4: Jalankan test, pastikan lulus**

Run: `cd frontend && npx vitest run src/lib/hris/attendance-requests.test.ts src/lib/hris/short-hours.test.ts src/features/hris/attendance-requests && npx tsc --noEmit -p .`
Expected: semua PASS; `tsc` tanpa output.

Run: `grep -rnE 'from "@/lib/(db|audit)"|-repo"|server-only' frontend/src/features/hris/attendance-requests frontend/src/lib/hris/attendance-requests.ts frontend/src/lib/hris/short-hours.ts`
Expected: tanpa output.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/hris/attendance-requests.ts frontend/src/lib/hris/attendance-requests.test.ts \
  frontend/src/lib/hris/short-hours.ts frontend/src/lib/hris/short-hours.test.ts frontend/src/features/hris/attendance-requests
git commit -m "feat(hris): lib & lapisan data pengajuan absensi dan kurang jam (EPIC-065)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 7: ESS → Absensi: absen terlewat, form pengajuan, status pengajuan

**Files:**
- Create: `frontend/src/features/hris/attendance-requests/components/attendance-request-dialog.tsx`
- Create: `frontend/src/features/hris/attendance-requests/components/attendance-request-status-badge.tsx`
- Create: `frontend/src/features/hris/attendance-requests/components/attendance-gaps-card.tsx`
- Create: `frontend/src/features/hris/attendance-requests/components/my-attendance-requests.tsx`
- Create: `frontend/src/features/hris/attendance-requests/components/ess-attendance-requests.tsx`
- Modify: `frontend/src/features/hris/attendance-requests/index.ts`
- Modify: `frontend/src/features/hris/ess/components/absensi-page.tsx`
- Test: `frontend/src/features/hris/attendance-requests/components/attendance-request-dialog.test.tsx`, `ess-attendance-requests.test.tsx`

**Interfaces:**
- Consumes: Task 6 (lib, hook, API); `todayWib` (`lib/dates`); `shortDateId` (`lib/hris/placements`); UI `Dialog*`, `Button`, `Input`, `Textarea`, `Card`, `Badge`.
- Produces:
  - `interface AttendanceRequestPrefill { date: string; hint?: string }`
  - `function AttendanceRequestDialog(props: { prefill: AttendanceRequestPrefill; onClose: () => void })` (dirender hanya saat terbuka)
  - `function AttendanceRequestStatusBadge(props: { status: AttendanceRequestStatus })` (dipakai Task 8)
  - `function AttendanceGapsCard(props: { onRequest: (p: AttendanceRequestPrefill) => void })`
  - `function MyAttendanceRequests()`
  - `function EssAttendanceRequests()` (diekspor dari `index.ts`)

- [ ] **Step 1: Tulis test yang gagal**

`frontend/src/features/hris/attendance-requests/components/attendance-request-dialog.test.tsx`:

```tsx
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AttendanceRequestDialog } from "./attendance-request-dialog";

const mutate = vi.fn();
const toastError = vi.fn();
vi.mock("sonner", () => ({ toast: { error: (m: string) => toastError(m), success: vi.fn() } }));
vi.mock("../../mutations", () => ({ useCreateAttendanceRequest: () => ({ mutate, isPending: false }) }));
vi.mock("@/lib/dates", async (orig) => ({ ...(await orig<typeof import("@/lib/dates")>()), todayWib: () => "2026-10-05" }));

afterEach(() => {
  cleanup();
  mutate.mockReset();
  toastError.mockReset();
});

describe("AttendanceRequestDialog", () => {
  it("diisi dari absen terlewat dan hanya mengirim jam yang diisi", () => {
    render(
      <AttendanceRequestDialog
        prefill={{ date: "2026-10-03", hint: "Tercatat masuk 08:05 tanpa absen pulang — isi jam pulang yang benar." }}
        onClose={() => {}}
      />
    );
    expect((screen.getByLabelText("Tanggal") as HTMLInputElement).value).toBe("2026-10-03");
    expect(screen.getByText(/Tercatat masuk 08:05/)).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Jam pulang"), { target: { value: "17:00" } });
    fireEvent.change(screen.getByLabelText("Keterangan"), { target: { value: "  Lupa absen pulang, HP mati " } });
    fireEvent.click(screen.getByRole("button", { name: "Kirim pengajuan" }));
    expect(mutate).toHaveBeenCalledWith(
      { date: "2026-10-03", clock_in: null, clock_out: "17:00", reason: "Lupa absen pulang, HP mati" },
      expect.anything()
    );
  });

  it("menolak tanggal masa depan dan keterangan pendek sebelum mengirim", () => {
    render(<AttendanceRequestDialog prefill={{ date: "2026-10-06" }} onClose={() => {}} />);
    fireEvent.change(screen.getByLabelText("Jam masuk"), { target: { value: "08:00" } });
    fireEvent.change(screen.getByLabelText("Keterangan"), { target: { value: "Internet mati" } });
    fireEvent.click(screen.getByRole("button", { name: "Kirim pengajuan" }));
    expect(toastError).toHaveBeenLastCalledWith("Tanggal pengajuan tidak boleh di masa depan");
    fireEvent.change(screen.getByLabelText("Tanggal"), { target: { value: "2026-10-02" } });
    fireEvent.change(screen.getByLabelText("Keterangan"), { target: { value: "Lupa" } });
    fireEvent.click(screen.getByRole("button", { name: "Kirim pengajuan" }));
    expect(toastError).toHaveBeenLastCalledWith("Keterangan wajib diisi (minimal 5 karakter)");
    expect(mutate).not.toHaveBeenCalled();
  });
});
```

`frontend/src/features/hris/attendance-requests/components/ess-attendance-requests.test.tsx`:

```tsx
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AttendanceGaps, AttendanceRequest } from "../../types";
import { EssAttendanceRequests } from "./ess-attendance-requests";

const create = vi.fn();
const cancel = vi.fn();
let gaps: AttendanceGaps = { start: null, end: null, days: [] };
let mine: AttendanceRequest[] = [];
vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));
vi.mock("@/lib/dates", async (orig) => ({ ...(await orig<typeof import("@/lib/dates")>()), todayWib: () => "2026-10-05" }));
vi.mock("../../queries", () => ({
  useMyAttendanceGaps: () => ({ data: gaps }),
  useMyAttendanceRequests: () => ({ data: mine }),
}));
vi.mock("../../mutations", () => ({
  useCreateAttendanceRequest: () => ({ mutate: create, isPending: false }),
  useCancelAttendanceRequest: () => ({ mutate: cancel, isPending: false }),
}));

function req(over: Partial<AttendanceRequest>): AttendanceRequest {
  return {
    id: "r1", employee_id: "e1", date: "2026-10-03", kind: "correction", clock_in: null, clock_out: "17:00",
    reason: "Lupa absen pulang, HP mati", attachment_path: null, status: "pending", attendance_id: null, decided_by: null,
    decided_by_name: null, decided_at: null, rejection_reason: null, created_at: "2026-10-04T01:00:00Z",
    employee: { id: "e1", full_name: "Budi Santoso", nip: "N01" }, recorded: null, ...over,
  };
}

afterEach(() => {
  cleanup();
  create.mockReset();
  cancel.mockReset();
  gaps = { start: null, end: null, days: [] };
  mine = [];
});

describe("EssAttendanceRequests", () => {
  it("kartu Absen terlewat membuka form bertanggal dengan petunjuk", () => {
    gaps = {
      start: "2026-10-01",
      end: "2026-10-04",
      days: [
        { date: "2026-10-01", kind: "missing", clock_in: null },
        { date: "2026-10-03", kind: "no_clock_out", clock_in: "08:05" },
      ],
    };
    render(<EssAttendanceRequests />);
    expect(screen.getByText("Absen terlewat (2)")).toBeTruthy();
    expect(screen.getByText(/Tidak ada absen/)).toBeTruthy();
    fireEvent.click(screen.getAllByRole("button", { name: "Ajukan koreksi" })[1]);
    expect((screen.getByLabelText("Tanggal") as HTMLInputElement).value).toBe("2026-10-03");
    expect(screen.getByText("Tercatat masuk 08:05 tanpa absen pulang — isi jam pulang yang benar.")).toBeTruthy();
  });

  it("tanpa absen terlewat hanya ada tombol umum, form bertanggal hari ini", () => {
    render(<EssAttendanceRequests />);
    expect(screen.queryByText(/Absen terlewat/)).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Ajukan absen manual / koreksi" }));
    expect((screen.getByLabelText("Tanggal") as HTMLInputElement).value).toBe("2026-10-05");
  });

  it("status pengajuan: alasan ditolak tampil, batal hanya saat menunggu", () => {
    mine = [req({ id: "r1" }), req({ id: "r2", status: "rejected", rejection_reason: "Tidak sesuai CCTV" })];
    render(<EssAttendanceRequests />);
    expect(screen.getByText("Menunggu HR")).toBeTruthy();
    expect(screen.getByText("Alasan ditolak: Tidak sesuai CCTV")).toBeTruthy();
    const buttons = screen.getAllByRole("button", { name: "Batalkan" });
    expect(buttons).toHaveLength(1);
    fireEvent.click(buttons[0]);
    expect(cancel).toHaveBeenCalledWith("r1", expect.anything());
  });
});
```

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd frontend && npx vitest run src/features/hris/attendance-requests/components`
Expected: FAIL — `Failed to resolve import "./attendance-request-dialog"`.

- [ ] **Step 3: Implementasi**

`frontend/src/features/hris/attendance-requests/components/attendance-request-dialog.tsx`:

```tsx
"use client";

import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { todayWib } from "@/lib/dates";
import {
  EMPTY_ATTENDANCE_REQUEST_FORM,
  toAttendanceRequestPayload,
  validateAttendanceRequestForm,
  type AttendanceRequestForm,
} from "@/lib/hris/attendance-requests";
import { useCreateAttendanceRequest } from "../mutations";

/** Isian awal: dari kartu "Absen terlewat" (tanggal + petunjuk) atau tombol umum (hari ini). */
export interface AttendanceRequestPrefill {
  date: string;
  hint?: string;
}

const MAX_PHOTO_BYTES = 5 * 1024 * 1024;

/**
 * ESS → Absensi → Ajukan absen manual / koreksi (EPIC-065, MOM AB-04/AB-06):
 * tanggal (hari ini atau sebelumnya di periode terbuka), jam yang benar,
 * keterangan wajib, foto bukti opsional. HR yang memutus. Dirender hanya
 * saat terbuka, sehingga setiap pembukaan mulai dari isian awal.
 */
export function AttendanceRequestDialog({ prefill, onClose }: { prefill: AttendanceRequestPrefill; onClose: () => void }) {
  const [form, setForm] = useState<AttendanceRequestForm>({ ...EMPTY_ATTENDANCE_REQUEST_FORM, date: prefill.date });
  const [photo, setPhoto] = useState<{ name: string; dataUrl: string } | null>(null);
  const create = useCreateAttendanceRequest();
  const set =
    (key: keyof AttendanceRequestForm) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) =>
      setForm((f) => ({ ...f, [key]: e.target.value }));

  function pickPhoto(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    e.target.value = "";
    if (!file) return;
    if (!file.type.startsWith("image/")) {
      toast.error("Lampiran harus berupa gambar JPG/PNG/WebP");
      return;
    }
    if (file.size > MAX_PHOTO_BYTES) {
      toast.error("Ukuran lampiran maksimal 5 MB");
      return;
    }
    const reader = new FileReader();
    reader.onloadend = () => setPhoto({ name: file.name, dataUrl: reader.result as string });
    reader.readAsDataURL(file);
  }

  function submit() {
    const error = validateAttendanceRequestForm(form, todayWib());
    if (error) {
      toast.error(error);
      return;
    }
    create.mutate(toAttendanceRequestPayload(form, photo?.dataUrl), {
      onSuccess: (json) => {
        toast.success(json.message || "Pengajuan absensi terkirim");
        onClose();
      },
      onError: (e) => toast.error(e.message || "Gagal mengirim pengajuan absensi"),
    });
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Ajukan absen manual / koreksi</DialogTitle>
          <DialogDescription>
            Untuk absen yang terlewat atau jamnya salah (mis. internet mati). HR yang menyetujui; jam yang disetujui
            menggantikan catatan absen.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          {prefill.hint && <p className="rounded-md bg-amber-50 px-3 py-2 text-xs text-amber-800">{prefill.hint}</p>}
          <label className="block text-xs font-medium text-gray-600">
            Tanggal
            <Input type="date" max={todayWib()} value={form.date} onChange={set("date")} />
          </label>
          <div className="grid grid-cols-2 gap-3">
            <label className="text-xs font-medium text-gray-600">
              Jam masuk
              <Input type="time" value={form.clock_in} onChange={set("clock_in")} />
            </label>
            <label className="text-xs font-medium text-gray-600">
              Jam pulang
              <Input type="time" value={form.clock_out} onChange={set("clock_out")} />
            </label>
          </div>
          <p className="text-xs text-gray-500">
            Kosongkan jam yang sudah benar. Bila belum ada absen sama sekali, jam masuk wajib diisi.
          </p>
          <label className="block text-xs font-medium text-gray-600">
            Keterangan
            <Textarea
              rows={3}
              maxLength={1000}
              placeholder="Mis. internet toko mati saat jam masuk"
              value={form.reason}
              onChange={set("reason")}
            />
          </label>
          <label className="block text-xs font-medium text-gray-600">
            Foto bukti (opsional, maks 5 MB)
            <Input type="file" accept="image/jpeg,image/png,image/webp" onChange={pickPhoto} />
          </label>
          {photo && <p className="text-xs text-gray-500">📎 {photo.name}</p>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Batal
          </Button>
          <Button disabled={create.isPending} onClick={submit}>
            Kirim pengajuan
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
```

`frontend/src/features/hris/attendance-requests/components/attendance-request-status-badge.tsx`:

```tsx
import { Badge } from "@/components/ui/badge";
import { ATTENDANCE_REQUEST_STATUS_META, type AttendanceRequestStatus } from "@/lib/hris/attendance-requests";

export function AttendanceRequestStatusBadge({ status }: { status: AttendanceRequestStatus }) {
  const meta = ATTENDANCE_REQUEST_STATUS_META[status];
  return <Badge className={meta.className}>{meta.label}</Badge>;
}
```

`frontend/src/features/hris/attendance-requests/components/attendance-gaps-card.tsx`:

```tsx
"use client";

import { AlertTriangle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { ATTENDANCE_GAP_LABEL, gapHint } from "@/lib/hris/attendance-requests";
import { shortDateId } from "@/lib/hris/placements";
import { useMyAttendanceGaps } from "../queries";
import type { AttendanceRequestPrefill } from "./attendance-request-dialog";

/**
 * "Absen terlewat" (MOM AB-06, K-9 digabung dengan AB-04): hari kerja di periode
 * absensi yang masih terbuka tanpa absen lengkap, masing-masing dengan tombol
 * "Ajukan koreksi". Tidak dirender bila tidak ada.
 */
export function AttendanceGapsCard({ onRequest }: { onRequest: (prefill: AttendanceRequestPrefill) => void }) {
  const days = useMyAttendanceGaps().data?.days ?? [];
  if (days.length === 0) return null;
  return (
    <Card className="border-amber-200 bg-amber-50/60">
      <CardContent className="space-y-2 p-4">
        <p className="flex items-center gap-1.5 text-sm font-semibold text-amber-800">
          <AlertTriangle className="h-4 w-4" />
          Absen terlewat ({days.length})
        </p>
        <p className="text-xs text-amber-700">
          Hari kerja di periode absensi yang masih terbuka tanpa absen lengkap. Ajukan koreksi dengan keterangan; HR yang
          menyetujui.
        </p>
        <ul className="divide-y divide-amber-100">
          {days.map((day) => (
            <li key={day.date} className="flex items-center justify-between gap-3 py-2 text-sm">
              <span className="text-gray-800">
                <b>{shortDateId(day.date)}</b> · {ATTENDANCE_GAP_LABEL[day.kind]}
                {day.clock_in ? ` (masuk ${day.clock_in})` : ""}
              </span>
              <Button
                size="sm"
                variant="outline"
                onClick={() => onRequest({ date: day.date, hint: gapHint(day.kind, day.clock_in) })}
              >
                Ajukan koreksi
              </Button>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  );
}
```

`frontend/src/features/hris/attendance-requests/components/my-attendance-requests.tsx`:

```tsx
"use client";

import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { ATTENDANCE_REQUEST_KIND_LABEL, attendanceRequestClocks } from "@/lib/hris/attendance-requests";
import { shortDateId } from "@/lib/hris/placements";
import { attendanceRequestAttachmentUrl } from "../api";
import { useCancelAttendanceRequest } from "../mutations";
import { useMyAttendanceRequests } from "../queries";
import { AttendanceRequestStatusBadge } from "./attendance-request-status-badge";

/** Pengajuan absensi milik karyawan beserta status dan alasan penolakan; batal selama menunggu. */
export function MyAttendanceRequests() {
  const rows = useMyAttendanceRequests().data ?? [];
  const cancel = useCancelAttendanceRequest();
  if (rows.length === 0) return null;
  return (
    <Card>
      <CardContent className="p-0">
        <p className="px-4 pt-4 text-sm font-semibold text-gray-900">Pengajuan absensi saya</p>
        <ul className="divide-y divide-gray-100">
          {rows.map((r) => (
            <li key={r.id} className="flex flex-wrap items-start justify-between gap-2 px-4 py-3 text-sm">
              <div className="min-w-0">
                <p className="font-medium text-gray-900">
                  {shortDateId(r.date)} · {ATTENDANCE_REQUEST_KIND_LABEL[r.kind]}
                </p>
                <p className="text-xs text-gray-500">
                  {attendanceRequestClocks(r)} — {r.reason}
                </p>
                {r.attachment_path && (
                  <a
                    href={attendanceRequestAttachmentUrl(r.attachment_path)}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-xs text-blue-600 underline"
                  >
                    Lihat foto bukti
                  </a>
                )}
                {r.rejection_reason && <p className="text-xs text-red-600">Alasan ditolak: {r.rejection_reason}</p>}
              </div>
              <div className="flex items-center gap-2">
                <AttendanceRequestStatusBadge status={r.status} />
                {r.status === "pending" && (
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={cancel.isPending}
                    onClick={() =>
                      cancel.mutate(r.id, {
                        onSuccess: (json) => toast.success(json.message || "Pengajuan absensi dibatalkan"),
                        onError: (e) => toast.error(e.message || "Gagal membatalkan pengajuan"),
                      })
                    }
                  >
                    Batalkan
                  </Button>
                )}
              </div>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  );
}
```

`frontend/src/features/hris/attendance-requests/components/ess-attendance-requests.tsx`:

```tsx
"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { todayWib } from "@/lib/dates";
import { AttendanceGapsCard } from "./attendance-gaps-card";
import { AttendanceRequestDialog, type AttendanceRequestPrefill } from "./attendance-request-dialog";
import { MyAttendanceRequests } from "./my-attendance-requests";

/** ESS → Absensi (EPIC-065): absen terlewat, pengajuan absen manual/koreksi, dan statusnya. */
export function EssAttendanceRequests() {
  const [prefill, setPrefill] = useState<AttendanceRequestPrefill | null>(null);
  return (
    <div className="space-y-3">
      <AttendanceGapsCard onRequest={setPrefill} />
      <div className="flex justify-end">
        <Button variant="outline" onClick={() => setPrefill({ date: todayWib() })}>
          Ajukan absen manual / koreksi
        </Button>
      </div>
      <MyAttendanceRequests />
      {prefill && <AttendanceRequestDialog prefill={prefill} onClose={() => setPrefill(null)} />}
    </div>
  );
}
```

`frontend/src/features/hris/attendance-requests/index.ts`, tambahkan di akhir:

```ts
export { EssAttendanceRequests } from "./components/ess-attendance-requests";
export { AttendanceRequestStatusBadge } from "./components/attendance-request-status-badge";
```

`frontend/src/features/hris/ess/components/absensi-page.tsx`, tambah impor:

```tsx
import { EssAttendanceRequests } from "@/features/hris/attendance-requests";
```

dan ganti:

```tsx
      {/* Terapis spa: hari dengan jadwal treatment ditandai "Spa" (shift tetap shift). */}
```

dengan:

```tsx
      {/* EPIC-065 (MOM AB-04/AB-06): absen terlewat + pengajuan absen manual/koreksi ke HR. */}
      <EssAttendanceRequests />

      {/* Terapis spa: hari dengan jadwal treatment ditandai "Spa" (shift tetap shift). */}
```

Ubah juga komentar fungsi halaman menjadi:

```tsx
/**
 * ESS → Absensi (/dashboard/me/absensi): shift hari ini, clock-in/out
 * dengan selfie + GPS, absen terlewat & pengajuan absen manual/koreksi
 * (EPIC-065), dan kalender absensi milik sendiri.
 */
```

- [ ] **Step 4: Jalankan test, pastikan lulus**

Run: `cd frontend && npx vitest run src/features/hris/attendance-requests src/features/hris/ess && npx tsc --noEmit -p .`
Expected: semua PASS; `tsc` tanpa output.

Run: `grep -rnE 'from "@/lib/(db|audit)"|-repo"|server-only' frontend/src/features/hris/attendance-requests`
Expected: tanpa output.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/features/hris/attendance-requests frontend/src/features/hris/ess/components/absensi-page.tsx
git commit -m "feat(hris): ESS absen terlewat dan pengajuan absen manual/koreksi (EPIC-065)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 8: HRIS → Absensi → tab Pengajuan Absensi + paritas badge Next

**Files:**
- Create: `frontend/src/features/hris/attendance-requests/use-attendance-request-decision.tsx`
- Create: `frontend/src/features/hris/attendance-requests/components/attendance-requests-tab.tsx`
- Modify: `frontend/src/features/hris/attendance-requests/index.ts`
- Modify: `frontend/src/features/hris/attendance/components/attendance-page.tsx`, `attendance-page.test.tsx`
- Modify: `frontend/src/lib/hris/nav-badges.ts`
- Test: `frontend/src/features/hris/attendance-requests/components/attendance-requests-tab.test.tsx`, `frontend/src/lib/hris/nav-badges.attendance-requests.test.ts`

**Interfaces:**
- Consumes: Task 6 (`useAttendanceRequests`, `useDecideAttendanceRequest`, `attendanceRequestAttachmentUrl`, lib label), Task 7 (`AttendanceRequestStatusBadge`); `useEssMe` (`features/hris/ess/queries`); `canManageTrips`; `ConfirmDialog`; `formatTime`.
- Produces:
  - `function useAttendanceRequestDecision(): { request(r: AttendanceRequest, action: AttendanceRequestAction): void; busyId: string | null | undefined; dialog: ReactNode }`
  - `function AttendanceRequestsTab()` (diekspor dari `index.ts`)
  - `export const ATTENDANCE_REQUESTS_HREF = "/dashboard/hris/attendance"` di `lib/hris/nav-badges.ts`; `buildNavBadges` mengisi kunci itu untuk `actor.isHrd`.

- [ ] **Step 1: Tulis test yang gagal**

`frontend/src/features/hris/attendance-requests/components/attendance-requests-tab.test.tsx`:

```tsx
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AttendanceRequest } from "../../types";
import { AttendanceRequestsTab } from "./attendance-requests-tab";

const decide = vi.fn();
const toastError = vi.fn();
const rows: AttendanceRequest[] = [];
vi.mock("sonner", () => ({ toast: { error: (m: string) => toastError(m), success: vi.fn() } }));
vi.mock("@/features/hris/ess/queries", () => ({ useEssMe: () => ({ data: { employee: { id: "hr1" } } }) }));
vi.mock("../../queries", () => ({ useAttendanceRequests: () => ({ data: rows, isError: false }) }));
vi.mock("../../mutations", () => ({
  useDecideAttendanceRequest: () => ({ mutate: decide, isPending: false, variables: undefined }),
}));
vi.mock("@/components/ui/select", () => ({
  Select: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  SelectTrigger: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  SelectValue: () => null,
  SelectContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  SelectItem: ({ children }: { children: React.ReactNode }) => <span>{children}</span>,
}));

function req(over: Partial<AttendanceRequest>): AttendanceRequest {
  return {
    id: "r1", employee_id: "e1", date: "2026-10-03", kind: "correction", clock_in: null, clock_out: "17:00",
    reason: "Lupa absen pulang, HP mati", attachment_path: "attendance-requests/e1/a.png", status: "pending",
    attendance_id: null, decided_by: null, decided_by_name: null, decided_at: null, rejection_reason: null,
    created_at: "2026-10-04T01:00:00Z", employee: { id: "e1", full_name: "Budi Santoso", nip: "N01" },
    recorded: { id: "a1", clock_in: "2026-10-03T01:05:00Z", clock_out: null, status: "present" }, ...over,
  };
}

afterEach(() => {
  cleanup();
  decide.mockReset();
  toastError.mockReset();
  rows.length = 0;
});

describe("AttendanceRequestsTab", () => {
  it("menampilkan pengajuan, jam yang diajukan, dan foto bukti; Setujui langsung dikirim", () => {
    rows.push(req({}));
    render(<AttendanceRequestsTab />);
    expect(screen.getByText("Budi Santoso")).toBeTruthy();
    expect(screen.getByText("Koreksi absen")).toBeTruthy();
    expect(screen.getByText("Pulang 17:00")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Lihat foto bukti" }).getAttribute("href")).toBe(
      "/api/hris/attendance-requests/attachment/attendance-requests/e1/a.png"
    );
    fireEvent.click(screen.getByRole("button", { name: "Setujui" }));
    expect(decide).toHaveBeenCalledWith({ id: "r1", action: "approve", rejectionReason: undefined }, expect.anything());
  });

  it("Tolak wajib alasan", () => {
    rows.push(req({}));
    render(<AttendanceRequestsTab />);
    fireEvent.click(screen.getByRole("button", { name: "Tolak" }));
    fireEvent.click(screen.getByRole("button", { name: "Tolak pengajuan" }));
    expect(toastError).toHaveBeenCalledWith("Alasan penolakan wajib diisi");
    expect(decide).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText("Alasan penolakan"), { target: { value: "CCTV menunjukkan tidak hadir" } });
    fireEvent.click(screen.getByRole("button", { name: "Tolak pengajuan" }));
    expect(decide).toHaveBeenCalledWith(
      { id: "r1", action: "reject", rejectionReason: "CCTV menunjukkan tidak hadir" },
      expect.anything()
    );
  });

  it("tanpa tombol keputusan untuk pengajuan HRD sendiri atau yang sudah diputus", () => {
    rows.push(req({ id: "own", employee_id: "hr1" }), req({ id: "done", status: "approved", decided_by_name: "HR Pemutus" }));
    render(<AttendanceRequestsTab />);
    expect(screen.queryByRole("button", { name: "Setujui" })).toBeNull();
    expect(screen.getByText("oleh HR Pemutus")).toBeTruthy();
  });
});
```

`frontend/src/lib/hris/nav-badges.attendance-requests.test.ts`:

```ts
import { beforeEach, describe, expect, it, vi } from "vitest";

const queryOne = vi.fn();
vi.mock("@/lib/db", () => ({ query: vi.fn(), queryOne: (...args: unknown[]) => queryOne(...args) }));

import { ATTENDANCE_REQUESTS_HREF, buildNavBadges } from "./nav-badges";
import type { WorkforceActor } from "./workforce-auth";

const actor = (over: Partial<WorkforceActor>): WorkforceActor => ({
  userId: "u1",
  role: "employee",
  employeeId: null,
  isHr: false,
  isHrd: false,
  fullName: "Akun Uji",
  ...over,
});

beforeEach(() => {
  queryOne.mockReset();
  queryOne.mockImplementation(async (sql: string) =>
    sql.includes("hris.attendance_requests") ? { total: "3" } : { total: "0" }
  );
});

describe("buildNavBadges — pengajuan absensi (EPIC-065, paritas Go)", () => {
  it("HRD mendapat jumlah pengajuan absensi yang menunggu di menu Absensi", async () => {
    const badges = await buildNavBadges(actor({ role: "hrd", isHr: true, isHrd: true }));
    expect(ATTENDANCE_REQUESTS_HREF).toBe("/dashboard/hris/attendance");
    expect(badges[ATTENDANCE_REQUESTS_HREF]).toBe(3);
    const sql = String(queryOne.mock.calls.map((c) => c[0]).find((s: string) => s.includes("hris.attendance_requests")));
    expect(sql).toContain("status = 'pending'");
  });

  it("hiring_manager (HR tanpa HRD) dan karyawan tidak mendapat badge itu", async () => {
    expect(await buildNavBadges(actor({ role: "hiring_manager", isHr: true }))).not.toHaveProperty(ATTENDANCE_REQUESTS_HREF);
    expect(await buildNavBadges(actor({ employeeId: "e1" }))).not.toHaveProperty(ATTENDANCE_REQUESTS_HREF);
  });
});
```

`frontend/src/features/hris/attendance/components/attendance-page.test.tsx`, tambahkan mock setelah mock `BusinessTripsTab`:

```tsx
vi.mock("@/features/hris/attendance-requests", () => ({ AttendanceRequestsTab: () => <div>tab-pengajuan</div> }));
```

dan blok test baru di akhir file:

```tsx
describe("AttendancePage: tab Pengajuan Absensi (EPIC-065)", () => {
  it.each(["super_admin", "admin", "hrd"])("ditawarkan untuk %s", (r) => {
    role = r;
    render(<AttendancePage />);
    expect(screen.getByRole("tab", { name: "Pengajuan Absensi" })).toBeTruthy();
  });

  it.each(["hiring_manager", "employee", null])("tidak ditawarkan untuk %s", (r) => {
    role = r;
    render(<AttendancePage />);
    expect(screen.queryByRole("tab", { name: "Pengajuan Absensi" })).toBeNull();
  });
});
```

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd frontend && npx vitest run src/features/hris/attendance-requests src/features/hris/attendance/components/attendance-page.test.tsx src/lib/hris/nav-badges.attendance-requests.test.ts`
Expected: FAIL — modul `./attendance-requests-tab` tidak ditemukan, tab "Pengajuan Absensi" tidak ada, `ATTENDANCE_REQUESTS_HREF` undefined.

- [ ] **Step 3: Implementasi**

`frontend/src/features/hris/attendance-requests/use-attendance-request-decision.tsx`:

```tsx
"use client";

import { useState, type ReactNode } from "react";
import { toast } from "sonner";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { Textarea } from "@/components/ui/textarea";
import { useDecideAttendanceRequest } from "./mutations";
import type { AttendanceRequest, AttendanceRequestAction } from "./types";

/**
 * Setujui / tolak pengajuan absensi (tab HRD). Setujui langsung dikirim; tolak
 * lewat dialog dengan alasan wajib. Pesan server ditampilkan apa adanya.
 * Render `dialog` sekali.
 */
export function useAttendanceRequestDecision(): {
  request: (r: AttendanceRequest, action: AttendanceRequestAction) => void;
  busyId: string | null | undefined;
  dialog: ReactNode;
} {
  const decide = useDecideAttendanceRequest();
  const [target, setTarget] = useState<AttendanceRequest | null>(null);
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const busyId = decide.isPending ? decide.variables?.id : null;

  function run(r: AttendanceRequest, action: AttendanceRequestAction, rejectionReason?: string) {
    decide.mutate(
      { id: r.id, action, rejectionReason },
      {
        onSuccess: (json) => toast.success(json.message || "Berhasil diproses"),
        onError: (e) => toast.error(e.message || "Gagal memproses"),
      }
    );
  }

  function request(r: AttendanceRequest, action: AttendanceRequestAction) {
    if (action === "approve") {
      run(r, "approve");
      return;
    }
    setReason("");
    setTarget(r);
    setOpen(true);
  }

  function confirm() {
    if (!target) return;
    const rejectionReason = reason.trim();
    if (!rejectionReason) {
      toast.error("Alasan penolakan wajib diisi");
      return;
    }
    setOpen(false);
    run(target, "reject", rejectionReason);
  }

  const dialog = (
    <ConfirmDialog
      open={open}
      onOpenChange={setOpen}
      title="Tolak pengajuan absensi?"
      confirmLabel="Tolak pengajuan"
      cancelLabel="Kembali"
      onConfirm={confirm}
    >
      <div className="space-y-1.5">
        <label htmlFor="attendance-request-rejection" className="block text-xs font-medium text-gray-600">
          Alasan penolakan
        </label>
        <Textarea
          id="attendance-request-rejection"
          rows={3}
          maxLength={500}
          value={reason}
          onChange={(e) => setReason(e.target.value)}
        />
      </div>
    </ConfirmDialog>
  );

  return { request, busyId, dialog };
}
```

`frontend/src/features/hris/attendance-requests/components/attendance-requests-tab.tsx`:

```tsx
"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useEssMe } from "@/features/hris/ess/queries";
import { formatTime } from "@/lib/format";
import {
  ATTENDANCE_REQUEST_KIND_LABEL,
  ATTENDANCE_REQUEST_STATUS_META,
  attendanceRequestClocks,
  type AttendanceRequestStatus,
} from "@/lib/hris/attendance-requests";
import { shortDateId } from "@/lib/hris/placements";
import { attendanceRequestAttachmentUrl } from "../api";
import { useAttendanceRequests } from "../queries";
import { useAttendanceRequestDecision } from "../use-attendance-request-decision";
import { AttendanceRequestStatusBadge } from "./attendance-request-status-badge";

const ALL = "all";

/**
 * Absensi HRD → tab Pengajuan Absensi (EPIC-065, MOM AB-04/AB-06): pengajuan absen
 * manual/koreksi karyawan. HRD menyetujui (absensi ditulis lewat jalur penyesuaian
 * HR yang sama: alasan, riwayat penyesuaian, audit) atau menolak dengan alasan.
 * Pengajuan HRD sendiri tidak bisa ia putus (server juga menolak).
 */
export function AttendanceRequestsTab() {
  const [status, setStatus] = useState<string>("pending");
  const requests = useAttendanceRequests(status === ALL ? {} : { status: status as AttendanceRequestStatus });
  const myEmployeeId = useEssMe().data?.employee?.id;
  const { request, busyId, dialog } = useAttendanceRequestDecision();
  const rows = requests.data ?? [];

  return (
    <div className="space-y-4">
      <div className="w-48">
        <label className="mb-1 block text-xs font-medium text-gray-600">Status</label>
        <Select value={status} onValueChange={(v) => v && setStatus(v)}>
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Semua status</SelectItem>
            {(Object.keys(ATTENDANCE_REQUEST_STATUS_META) as AttendanceRequestStatus[]).map((s) => (
              <SelectItem key={s} value={s}>
                {ATTENDANCE_REQUEST_STATUS_META[s].label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <Card>
        <CardContent className="p-0">
          {requests.isError ? (
            <p className="p-6 text-center text-sm text-red-600">Gagal memuat pengajuan absensi.</p>
          ) : rows.length === 0 ? (
            <p className="p-6 text-center text-sm text-gray-500">Tidak ada pengajuan absensi untuk filter ini.</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-gray-100 bg-gray-50 text-left text-gray-600">
                    <th className="p-3">Karyawan</th>
                    <th className="p-3">Tanggal</th>
                    <th className="p-3">Diajukan</th>
                    <th className="p-3">Tercatat</th>
                    <th className="p-3">Keterangan</th>
                    <th className="p-3">Status</th>
                    <th className="p-3 text-right">Aksi</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((r) => (
                    <tr key={r.id} className="border-b border-gray-50 align-top">
                      <td className="p-3">
                        <p className="font-medium text-gray-900">{r.employee?.full_name ?? "—"}</p>
                        {r.employee?.nip && <p className="text-xs text-gray-400">{r.employee.nip}</p>}
                      </td>
                      <td className="whitespace-nowrap p-3 text-gray-700">{shortDateId(r.date)}</td>
                      <td className="p-3 text-gray-700">
                        <p className="text-xs text-gray-500">{ATTENDANCE_REQUEST_KIND_LABEL[r.kind]}</p>
                        <p>{attendanceRequestClocks(r)}</p>
                      </td>
                      <td className="whitespace-nowrap p-3 text-gray-700">
                        {r.recorded
                          ? `${formatTime(r.recorded.clock_in, "—")} – ${formatTime(r.recorded.clock_out, "—")}`
                          : "Belum ada absen"}
                      </td>
                      <td className="p-3 text-gray-700">
                        <p className="max-w-xs whitespace-pre-line">{r.reason}</p>
                        {r.attachment_path && (
                          <a
                            href={attendanceRequestAttachmentUrl(r.attachment_path)}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="text-xs text-blue-600 underline"
                          >
                            Lihat foto bukti
                          </a>
                        )}
                      </td>
                      <td className="p-3">
                        <AttendanceRequestStatusBadge status={r.status} />
                        {r.decided_by_name && <p className="mt-1 text-xs text-gray-400">oleh {r.decided_by_name}</p>}
                        {r.rejection_reason && <p className="text-xs text-red-600">{r.rejection_reason}</p>}
                      </td>
                      <td className="whitespace-nowrap p-3 text-right">
                        {r.status === "pending" && r.employee_id !== myEmployeeId && (
                          <>
                            <Button size="sm" disabled={busyId === r.id} onClick={() => request(r, "approve")}>
                              Setujui
                            </Button>{" "}
                            <Button
                              size="sm"
                              variant="outline"
                              disabled={busyId === r.id}
                              onClick={() => request(r, "reject")}
                            >
                              Tolak
                            </Button>
                          </>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>
      {dialog}
    </div>
  );
}
```

`frontend/src/features/hris/attendance-requests/index.ts`, tambahkan:

```ts
export { AttendanceRequestsTab } from "./components/attendance-requests-tab";
```

`frontend/src/features/hris/attendance/components/attendance-page.tsx` — tambah impor:

```tsx
import { AttendanceRequestsTab } from "@/features/hris/attendance-requests";
```

ganti:

```tsx
  // Kuasa HR Perjalanan Dinas hanya super_admin/admin/hrd (bukan hiring_manager);
  // server tetap menolak aksi di luar itu.
  const { user } = useAuth();
  const showTrips = canManageTrips(user?.role);
```

dengan:

```tsx
  // Kuasa HRD (super_admin/admin/hrd; bukan hiring_manager): Perjalanan Dinas dan
  // Pengajuan Absensi (EPIC-065). Server tetap menolak aksi di luar itu.
  const { user } = useAuth();
  const hrd = canManageTrips(user?.role);
```

ganti blok `TabsList` … `</Tabs>`:

```tsx
        <TabsList className={`grid h-9 w-full max-w-xl ${showTrips ? "grid-cols-4" : "grid-cols-3"}`}>
          <TabsTrigger value="monitoring">Monitoring</TabsTrigger>
          <TabsTrigger value="rekap">Rekap</TabsTrigger>
          <TabsTrigger value="periode">Periode Absensi</TabsTrigger>
          {showTrips && <TabsTrigger value="dinas">Perjalanan Dinas</TabsTrigger>}
        </TabsList>
```

dengan:

```tsx
        <TabsList className={`grid h-9 w-full max-w-3xl ${hrd ? "grid-cols-5" : "grid-cols-3"}`}>
          <TabsTrigger value="monitoring">Monitoring</TabsTrigger>
          <TabsTrigger value="rekap">Rekap</TabsTrigger>
          <TabsTrigger value="periode">Periode Absensi</TabsTrigger>
          {hrd && <TabsTrigger value="pengajuan">Pengajuan Absensi</TabsTrigger>}
          {hrd && <TabsTrigger value="dinas">Perjalanan Dinas</TabsTrigger>}
        </TabsList>
```

dan ganti:

```tsx
        {showTrips && (
          <TabsContent value="dinas" className="mt-4">
            <BusinessTripsTab />
          </TabsContent>
        )}
```

dengan:

```tsx
        {hrd && (
          <TabsContent value="pengajuan" className="mt-4">
            <AttendanceRequestsTab />
          </TabsContent>
        )}
        {hrd && (
          <TabsContent value="dinas" className="mt-4">
            <BusinessTripsTab />
          </TabsContent>
        )}
```

Tambahkan di komentar fungsi baris: ` * - Pengajuan Absensi: absen manual/koreksi dari ESS (EPIC-065), HRD menyetujui/menolak.` dan ganti "empat tab" menjadi "lima tab".

`frontend/src/lib/hris/nav-badges.ts` — setelah `TEAM_APPROVALS_HREF` tambahkan:

```ts
/** Menu HR Absensi: pengajuan absen manual/koreksi menunggu HRD (EPIC-065, sama dengan Go). */
export const ATTENDANCE_REQUESTS_HREF = "/dashboard/hris/attendance";
```

dan di `buildNavBadges`, setelah blok `if (actor.isHr) { … }`:

```ts
  // Hanya HRD (super_admin/admin/hrd) yang memutus pengajuan absensi (EPIC-065 R2).
  if (actor.isHrd) {
    badges[ATTENDANCE_REQUESTS_HREF] = await countOne(
      `SELECT count(*) AS total FROM hris.attendance_requests WHERE status = 'pending'`,
      []
    );
  }
```

- [ ] **Step 4: Jalankan test, pastikan lulus**

Run: `cd frontend && npx vitest run src/features/hris/attendance-requests src/features/hris/attendance src/lib/hris/nav-badges.attendance-requests.test.ts src/lib/hris/approval-parity.test.ts && npx tsc --noEmit -p .`
Expected: semua PASS (termasuk test Perjalanan Dinas lama di `attendance-page.test.tsx`); `tsc` tanpa output.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/features/hris/attendance-requests frontend/src/features/hris/attendance/components/attendance-page.tsx \
  frontend/src/features/hris/attendance/components/attendance-page.test.tsx frontend/src/lib/hris/nav-badges.ts \
  frontend/src/lib/hris/nav-badges.attendance-requests.test.ts
git commit -m "feat(hris): tab Pengajuan Absensi untuk HRD + badge menu (Go & fallback Next) (EPIC-065)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 9: Label kurang jam di kalender, Monitoring, Rekap + jam minimum di Lokasi Toko

**Files:**
- Create: `frontend/src/features/hris/attendance/components/short-hours-badge.tsx`
- Modify: `frontend/src/features/hris/attendance/types.ts` (`RosterAttendance`, `AttendanceListRow`, `CalendarAttendance`)
- Modify: `frontend/src/features/hris/attendance/components/roster-row.tsx:81-84`, `attendance-recap-tab.tsx:398`
- Modify: `frontend/src/components/hris/AttendanceDayCell.tsx`, `frontend/src/components/hris/AttendanceDayDialog.tsx`
- Modify: `frontend/src/features/hris/placements/types.ts`, `api.ts`, `mutations.ts`, `components/branch-location-dialog.tsx`, `components/branch-location-dialog.test.tsx`
- Test: `frontend/src/features/hris/attendance/components/short-hours-badge.test.tsx`, `frontend/src/components/hris/AttendanceDayCell.test.tsx`

**Interfaces:**
- Consumes: Task 6 `lib/hris/short-hours.ts`; JSON Task 5 (`is_short_hours`, `min_work_hours` di list/roster/branches, `PUT /api/hris/branches/{id}/min-work-hours`); `attendanceQueryKeys` (`features/hris/attendance/query-keys`).
- Produces:
  - `function ShortHoursBadge(props: { isShort?: boolean | null; minHours?: string | number | null })`
  - Field opsional TS `is_short_hours?: boolean`, `min_work_hours?: string | number | null` di tiga tipe absensi; `StoreBranch.min_work_hours?: string | null`.
  - `saveBranchMinWorkHours(id, payload)`, `useSaveBranchMinWorkHours()`.

- [ ] **Step 1: Tulis test yang gagal**

`frontend/src/features/hris/attendance/components/short-hours-badge.test.tsx`:

```tsx
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { RosterEmployee } from "../types";
import { RosterRow } from "./roster-row";
import { ShortHoursBadge } from "./short-hours-badge";

vi.mock("next/image", () => ({ default: ({ alt }: { alt: string }) => <span>{alt}</span> }));
vi.mock("./selfie-photo-dialog", () => ({ SelfiePhotoDialog: () => null }));

afterEach(cleanup);

const emp = (isShort: boolean): RosterEmployee => ({
  employee_id: "e1", full_name: "Crew HQ", nip: null, photo_url: null, department_name: null, job_title: null,
  shift: null, status: "hadir", is_overdue: false, leave: null,
  attendance: {
    id: "a1", clock_in: "2026-10-03T02:00:00Z", clock_out: "2026-10-03T10:59:00Z", work_hours: 7.98, is_late: false,
    late_minutes: 0, clock_in_photo_url: null, clock_out_photo_url: null, shift_id: null, clock_in_geofence: null,
    clock_in_distance_m: null, clock_out_geofence: null, clock_out_distance_m: null, notes: null,
    is_short_hours: isShort, min_work_hours: isShort ? 9 : null,
  },
});

describe("label kurang jam (AB-05)", () => {
  it("badge hanya untuk baris berlabel, dengan keterangan jam minimum", () => {
    render(<ShortHoursBadge isShort minHours="9.00" />);
    expect(screen.getByText("Kurang jam / pulang lebih cepat").getAttribute("title")).toBe(
      "Hadir kurang dari 9 jam (jam minimum toko)"
    );
    cleanup();
    const { container } = render(<ShortHoursBadge isShort={false} minHours="9.00" />);
    expect(container.textContent).toBe("");
  });

  it("Monitoring (roster) menampilkan label", () => {
    render(<RosterRow emp={emp(true)} isToday={false} />);
    expect(screen.getByText("Kurang jam / pulang lebih cepat")).toBeTruthy();
    cleanup();
    render(<RosterRow emp={emp(false)} isToday={false} />);
    expect(screen.queryByText("Kurang jam / pulang lebih cepat")).toBeNull();
  });
});
```

`frontend/src/components/hris/AttendanceDayCell.test.tsx`:

```tsx
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { CalendarAttendance } from "@/features/hris/attendance/types";
import { AttendanceDayCell } from "./AttendanceDayCell";
import { AttendanceDayDialog } from "./AttendanceDayDialog";

afterEach(cleanup);

const attendance = (isShort: boolean): CalendarAttendance => ({
  id: "a1", date: "2026-10-03", clock_in: "2026-10-03T02:00:00Z", clock_out: "2026-10-03T10:59:00Z",
  work_hours: 7.98, status: "present", is_late: false, is_short_hours: isShort, min_work_hours: isShort ? "9.00" : null,
});

describe("kalender ESS: label kurang jam (AB-05)", () => {
  it("sel kalender menandai baris kurang jam", () => {
    render(
      <AttendanceDayCell day={3} isToday={false} isSunday={false} attendance={attendance(true)} scheduledShift={null}
        isDayOff={false} holidays={[]} onClick={() => {}} />
    );
    expect(screen.getByText("Kurang jam").getAttribute("title")).toBe("Hadir kurang dari 9 jam (jam minimum toko)");
  });

  it("tanpa label bila tidak kurang jam", () => {
    render(
      <AttendanceDayCell day={3} isToday={false} isSunday={false} attendance={attendance(false)} scheduledShift={null}
        isDayOff={false} holidays={[]} onClick={() => {}} />
    );
    expect(screen.queryByText("Kurang jam")).toBeNull();
  });

  it("detail hari menuliskan keterangan lengkap", () => {
    render(
      <AttendanceDayDialog
        info={{ date: "2026-10-03", attendance: attendance(true), scheduledShift: null, isDayOff: false, holidays: [] }}
        hasSchedule={false}
        onClose={() => {}}
      />
    );
    expect(
      screen.getByText("Kurang jam / pulang lebih cepat · Hadir kurang dari 9 jam (jam minimum toko)")
    ).toBeTruthy();
  });
});
```

`frontend/src/features/hris/placements/components/branch-location-dialog.test.tsx` — ganti mock mutations:

```tsx
vi.mock("../mutations", () => ({ useSaveBranchLocation: () => ({ mutate, isPending: false }) }));
```

dengan:

```tsx
const mutateHours = vi.fn();
vi.mock("../mutations", () => ({
  useSaveBranchLocation: () => ({ mutate, isPending: false }),
  useSaveBranchMinWorkHours: () => ({ mutate: mutateHours, isPending: false }),
}));
```

ganti fixture `b2` agar menyimpan jam minimum:

```tsx
  { id: "b2", code: "KPO", name: "Paddy Kopo", company_id: "c1", company_name: "Paddy", lat: "-6.946000", lng: "107.592000", geofence_radius_m: 200 },
```

dengan:

```tsx
  { id: "b2", code: "KPO", name: "Paddy Kopo", company_id: "c1", company_name: "Paddy", lat: "-6.946000", lng: "107.592000", geofence_radius_m: 200, min_work_hours: "9.00" },
```

tambahkan `mutateHours.mockReset();` di `afterEach`, dan test baru di dalam `describe("BranchLocationDialog", …)`:

```tsx
  it("menyimpan jam kerja minimum dan menolak di luar 1–24 jam (AB-05)", () => {
    render(<BranchLocationDialog open onOpenChange={() => {}} branches={branches} />);
    fireEvent.click(screen.getByRole("button", { name: /Paddy PVJ/ }));
    expect((screen.getByLabelText("Jam kerja minimum (jam)") as HTMLInputElement).value).toBe("");
    fireEvent.change(screen.getByLabelText("Jam kerja minimum (jam)"), { target: { value: "0.5" } });
    fireEvent.click(screen.getByRole("button", { name: "Simpan jam minimum" }));
    expect(mutateHours).not.toHaveBeenCalled();
    expect(toastError).toHaveBeenCalledWith("Jam kerja minimum 1–24 jam (kelipatan 0,25)");
    fireEvent.change(screen.getByLabelText("Jam kerja minimum (jam)"), { target: { value: "9" } });
    fireEvent.click(screen.getByRole("button", { name: "Simpan jam minimum" }));
    expect(mutateHours).toHaveBeenCalledWith({ id: "b1", payload: { min_work_hours: 9 } }, expect.anything());
  });

  it("memuat jam minimum tersimpan dan mengosongkannya menjadi null", () => {
    render(<BranchLocationDialog open onOpenChange={() => {}} branches={branches} />);
    fireEvent.click(screen.getByRole("button", { name: /Paddy Kopo/ }));
    expect((screen.getByLabelText("Jam kerja minimum (jam)") as HTMLInputElement).value).toBe("9");
    fireEvent.change(screen.getByLabelText("Jam kerja minimum (jam)"), { target: { value: "" } });
    fireEvent.click(screen.getByRole("button", { name: "Simpan jam minimum" }));
    expect(mutateHours).toHaveBeenCalledWith({ id: "b2", payload: { min_work_hours: null } }, expect.anything());
  });
```

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd frontend && npx vitest run src/features/hris/attendance/components/short-hours-badge.test.tsx src/components/hris/AttendanceDayCell.test.tsx src/features/hris/placements/components/branch-location-dialog.test.tsx`
Expected: FAIL — `./short-hours-badge` tidak ditemukan, teks "Kurang jam" tidak ada, label "Jam kerja minimum (jam)" tidak ada.

- [ ] **Step 3: Implementasi**

`frontend/src/features/hris/attendance/components/short-hours-badge.tsx`:

```tsx
import { Clock } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { SHORT_HOURS_LABEL, shortHoursDetail } from "@/lib/hris/short-hours";

/** Label AB-05 pada baris absensi (EPIC-065); tidak dirender bila baris tidak berlabel. */
export function ShortHoursBadge({ isShort, minHours }: { isShort?: boolean | null; minHours?: string | number | null }) {
  if (!isShort) return null;
  return (
    <Badge className="gap-1 bg-orange-100 text-orange-700" title={shortHoursDetail(minHours)}>
      <Clock className="h-3 w-3" />
      {SHORT_HOURS_LABEL}
    </Badge>
  );
}
```

`frontend/src/features/hris/attendance/types.ts`:
- di `RosterAttendance`, setelah `notes: string | null;`:

```ts
  /** Kurang jam / pulang lebih cepat (EPIC-065 AB-05); opsional karena fallback Next tidak mengisinya */
  is_short_hours?: boolean;
  /** Jam minimum cabang saat dilabel (jam) */
  min_work_hours?: number | null;
```

- di `AttendanceListRow`, setelah `clock_out_distance_m?: number | null;`:

```ts
  /** Kurang jam / pulang lebih cepat (EPIC-065 AB-05) */
  is_short_hours?: boolean;
  /** numeric(4,2) sebagai string, jam minimum cabang saat dilabel */
  min_work_hours?: string | number | null;
```

- di `CalendarAttendance`, setelah `is_late: boolean;`:

```ts
  /** Kurang jam / pulang lebih cepat (EPIC-065 AB-05) */
  is_short_hours?: boolean;
  min_work_hours?: string | number | null;
```

`frontend/src/features/hris/attendance/components/roster-row.tsx` — tambah impor `import { ShortHoursBadge } from "./short-hours-badge";` dan ganti:

```tsx
            <GeofenceBadge status={emp.attendance.clock_out_geofence} distanceM={emp.attendance.clock_out_distance_m} prefix="Pulang" />
          </div>
```

dengan:

```tsx
            <GeofenceBadge status={emp.attendance.clock_out_geofence} distanceM={emp.attendance.clock_out_distance_m} prefix="Pulang" />
            <ShortHoursBadge isShort={emp.attendance.is_short_hours} minHours={emp.attendance.min_work_hours} />
          </div>
```

`frontend/src/features/hris/attendance/components/attendance-recap-tab.tsx` — tambah impor `import { ShortHoursBadge } from "./short-hours-badge";` dan ganti:

```tsx
                          <AdjustmentBadge row={row} />
```

dengan:

```tsx
                          <AdjustmentBadge row={row} />
                          <ShortHoursBadge isShort={row.is_short_hours} minHours={row.min_work_hours} />
```

`frontend/src/components/hris/AttendanceDayCell.tsx` — tambah impor `import { shortHoursDetail } from "@/lib/hris/short-hours";`; di titik ringkas mobile ganti:

```tsx
          {scheduledShift && <span className="h-1.5 w-1.5 rounded-full bg-indigo-400" />}
```

dengan:

```tsx
          {scheduledShift && <span className="h-1.5 w-1.5 rounded-full bg-indigo-400" />}
          {attendance?.is_short_hours && <span className="h-1.5 w-1.5 rounded-full bg-orange-500" />}
```

dan di blok realisasi ganti:

```tsx
                : "Hadir"}
          </p>
        </div>
      )}
```

dengan:

```tsx
                : "Hadir"}
          </p>
          {attendance.is_short_hours && (
            <p
              className="text-[10px] font-semibold leading-tight text-orange-600"
              title={shortHoursDetail(attendance.min_work_hours)}
            >
              Kurang jam
            </p>
          )}
        </div>
      )}
```

`frontend/src/components/hris/AttendanceDayDialog.tsx` — tambah impor `import { SHORT_HOURS_LABEL, shortHoursDetail } from "@/lib/hris/short-hours";` dan ganti:

```tsx
                      : ""}
                  </p>
                </div>
```

dengan:

```tsx
                      : ""}
                  </p>
                  {attendance.is_short_hours && (
                    <p className="text-orange-600">
                      {SHORT_HOURS_LABEL} · {shortHoursDetail(attendance.min_work_hours)}
                    </p>
                  )}
                </div>
```

`frontend/src/features/hris/placements/types.ts` — di `StoreBranch` setelah `geofence_radius_m: number | null;`:

```ts
  /** Jam kerja minimum (EPIC-065 AB-05), numeric pg sebagai string; null/absen = tanpa aturan */
  min_work_hours?: string | null;
```

dan setelah `SavedBranchLocation`:

```ts
/** Respons PUT /api/hris/branches/{id}/min-work-hours. */
export type SavedBranchMinWorkHours = Pick<StoreBranch, "id" | "code" | "name"> & {
  min_work_hours: string | null;
  /** Jumlah baris absensi periode terbuka yang dilabel ulang */
  relabelled: number;
};
```

`frontend/src/features/hris/placements/api.ts` — tambahkan `SavedBranchMinWorkHours` ke impor tipe dan setelah `saveBranchLocation`:

```ts
/** PUT penuh: null = tanpa aturan jam minimum (EPIC-065 AB-05). */
export const saveBranchMinWorkHours = (id: string, payload: { min_work_hours: number | null }) =>
  apiPut<{ data: SavedBranchMinWorkHours; message?: string }>(`/api/hris/branches/${id}/min-work-hours`, payload);
```

`frontend/src/features/hris/placements/mutations.ts` — tambah impor `import { attendanceQueryKeys } from "@/features/hris/attendance/query-keys";`, tambahkan `saveBranchMinWorkHours` ke impor `./api`, lalu di akhir file:

```ts
export function useSaveBranchMinWorkHours() {
  const qc = useQueryClient();
  const invalidatePlacements = useInvalidatePlacements();
  return useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: { min_work_hours: number | null } }) =>
      saveBranchMinWorkHours(id, payload),
    onSuccess: () => {
      void invalidatePlacements();
      // Label kurang jam di absensi periode terbuka ikut berubah.
      void qc.invalidateQueries({ queryKey: attendanceQueryKeys.all });
    },
  });
}
```

`frontend/src/features/hris/placements/components/branch-location-dialog.tsx`:
- impor tambahan:

```tsx
import { formatHours, toMinWorkHoursPayload, validateMinWorkHours } from "@/lib/hris/short-hours";
import { useSaveBranchLocation, useSaveBranchMinWorkHours } from "../mutations";
```

  (ganti impor `useSaveBranchLocation` yang lama).
- di dalam komponen, setelah `const save = useSaveBranchLocation();`:

```tsx
  const [minHours, setMinHours] = useState("");
  const saveHours = useSaveBranchMinWorkHours();
```

- di `choose(b)`, setelah `setPaste("");`:

```tsx
    setMinHours(b.min_work_hours == null ? "" : String(Number(b.min_work_hours)));
```

- setelah fungsi `submit()`:

```tsx
  function submitHours() {
    if (!selected) return;
    const error = validateMinWorkHours(minHours);
    if (error) {
      toast.error(error);
      return;
    }
    saveHours.mutate(
      { id: selected.id, payload: toMinWorkHoursPayload(minHours) },
      {
        onSuccess: () => toast.success(`Jam kerja minimum ${selected.name} disimpan`),
        onError: (e) => toast.error(e.message || "Gagal menyimpan jam kerja minimum"),
      }
    );
  }
```

- di daftar toko, ganti:

```tsx
                  <span className="truncate">{b.name}</span>
```

dengan:

```tsx
                  <span className="truncate">
                    {b.name}
                    {b.min_work_hours != null && (
                      <span className="ml-1 text-[10px] text-indigo-600">min {formatHours(b.min_work_hours)} jam</span>
                    )}
                  </span>
```

- setelah paragraf `Radius kosong = …` (masih di dalam panel toko terpilih):

```tsx
              <div className="rounded-lg border border-gray-100 p-3">
                <label className="block text-xs font-medium text-gray-600">
                  Jam kerja minimum (jam)
                  <Input
                    type="number"
                    min={1}
                    max={24}
                    step={0.25}
                    placeholder="Kosong = tanpa aturan"
                    value={minHours}
                    onChange={(e) => setMinHours(e.target.value)}
                  />
                </label>
                <p className="mt-1 text-xs text-gray-500">
                  Absen dengan durasi masuk–pulang di bawah angka ini diberi label &quot;Kurang jam / pulang lebih
                  cepat&quot;. Isi 9 untuk kantor pusat (HQ); kosongkan untuk toko.
                </p>
                <Button size="sm" variant="outline" className="mt-2" disabled={saveHours.isPending} onClick={submitHours}>
                  Simpan jam minimum
                </Button>
              </div>
```

- [ ] **Step 4: Jalankan test, pastikan lulus**

Run: `cd frontend && npx vitest run src/features/hris/attendance src/components/hris src/features/hris/placements src/features/hris/ess && npx tsc --noEmit -p .`
Expected: semua PASS (termasuk test lama `branch-location-dialog`); `tsc` tanpa output.

Run: `grep -rnE 'from "@/lib/(db|audit)"|-repo"|server-only' frontend/src/features/hris/attendance/components/short-hours-badge.tsx frontend/src/lib/hris/short-hours.ts`
Expected: tanpa output.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/features/hris/attendance frontend/src/components/hris/AttendanceDayCell.tsx \
  frontend/src/components/hris/AttendanceDayDialog.tsx frontend/src/components/hris/AttendanceDayCell.test.tsx \
  frontend/src/features/hris/placements
git commit -m "feat(hris): label kurang jam di kalender/Monitoring/Rekap + jam minimum di Lokasi Toko (EPIC-065)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 10: Verifikasi menyeluruh & dokumentasi

**Files:**
- Modify: `docs/epics/EPIC-065-absensi-lanjutan.md` (status, Automation Log)
- Modify: `docs/epics/README.md` (baris EPIC-065)
- Modify: `docs/hris/acuan-mom-20261019/status-requirement.md` (AB-04, AB-05, AB-06, LA-01 + ringkasan)
- Modify: `docs/hris/acuan-mom-20261019/matriks-approval.md` (baris Absen Manual / Koreksi Absen)
- Modify: `docs/hris/acuan-mom-20261019/pertanyaan-rekomendasi.md` (Q-AB-1: catatan implementasi R7)

- [ ] **Step 1: Full suite**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test -p 1 ./... -count=1 > /tmp/epic065-go.txt 2>&1; grep -- '--- FAIL' /tmp/epic065-go.txt | sed 's/ (.*//' | sort -u | diff /tmp/epic065-baseline.txt -`
Expected: `diff` kosong (tidak ada kegagalan baru). `gofmt -l backend/internal` tanpa output.

Run: `cd frontend && npx vitest run 2>&1 | tail -5 && npx tsc --noEmit -p .`
Expected: 0 gagal; `tsc` tanpa output.

Run: `grep -rnE 'from "@/lib/(db|audit)"|-repo"|server-only' frontend/src/features/hris/attendance-requests frontend/src/lib/hris/attendance-requests.ts frontend/src/lib/hris/short-hours.ts frontend/src/features/hris/attendance/components/short-hours-badge.tsx`
Expected: tanpa output.

- [ ] **Step 2: Cek manual (DB test, bukan live)**

Jalankan API Go dan `next dev` terhadap DB test sesuai skill `run` proyek. Siapkan di DB test: karyawan A (akun ESS, penempatan di cabang "Paddy HQ Bandung", jadwal shift), akun HR (`hrd` dengan menu Absensi dan Penempatan).
1. HR: HRIS → Penempatan → Lokasi Toko → "Paddy HQ Bandung" → Jam kerja minimum 9 → Simpan; daftar toko menampilkan "min 9 jam".
2. A: ESS → Absensi → kartu "Absen terlewat" memuat hari kerja tanpa absen; "Ajukan koreksi" → form bertanggal → isi jam + keterangan + foto → toast terkirim; daftar "Pengajuan absensi saya" berstatus "Menunggu HR"; hari itu hilang dari kartu.
3. HR: lonceng notifikasi memuat "Pengajuan Absensi: A"; menu Absensi berbadge; tab "Pengajuan Absensi" → foto bukti terbuka → Setujui → Rekap memuat baris "Manual" dengan riwayat penyesuaian berisi "Pengajuan karyawan: …"; Settings → Audit memuat `attendance_request.approve`.
4. A: pengajuan lain → HR Tolak tanpa alasan (toast) lalu dengan alasan; A melihat "Alasan ditolak: …" dan notifikasi.
5. A clock-in, lalu clock-out < 9 jam di HQ: kalender ESS menandai "Kurang jam"; Monitoring dan Rekap menampilkan badge; HR mendapat notifikasi "⚠️ Jam Kerja Kurang".

- [ ] **Step 3: Dokumentasi**

- `status-requirement.md`: AB-04, AB-06 → ✅ "(EPIC-065, menunggu review & rilis)" dengan bukti `file:line` (`attendance_requests.go`, `attendance_gaps.go`, `attendance_adjust.go` kait `also`) dan nama test (`TestCheckAttendanceRequest`, `TestAttendanceGaps` domain & integrasi, `TestAttendanceRequestCreate`, `TestAttendanceRequestDecision`; vitest `attendance-request-dialog`, `ess-attendance-requests`, `attendance-requests-tab`, `nav-badges.attendance-requests`). AB-05 → ✅ (kode; jam minimum HQ live diisi HR saat rilis) dengan bukti migrasi `20261025090100_short_hours.sql`, `branch_work_hours.go`, `TestShortHoursLabel`, vitest `short-hours-badge`, `AttendanceDayCell`, `branch-location-dialog`. LA-01: tambahkan `attendance_request.approve|reject|cancel` dan `branch_min_hours.update` ke daftar tercakup. Perbarui ringkasan (✅ +3: AB-04, AB-05, AB-06; ⚠️ dan ❌ berkurang) dan baris "Diperbarui …" di kepala file.
- `matriks-approval.md`, baris "Absen Manual / Koreksi Absen", kolom kode sekarang: "EPIC-065: pengajuan karyawan dari ESS → antrean HR (Absensi → Pengajuan Absensi), HRD + menu Absensi, tanpa self-approval, persetujuan menulis lewat penyesuaian HR, audit `attendance_request.*`".
- `pertanyaan-rekomendasi.md`, baris Q-AB-1: tambahkan "Diimplementasikan EPIC-065 R7–R9: jam minimum per cabang (`min_work_hours`, HQ = 9 diisi HR), label tersimpan di baris, notifikasi clock-out mengikuti label".
- Epic: Automation Log berisi commit Task 1–9, ruling R1–R11 yang dipakai (dan perubahannya, bila ada), bukti test, dan langkah rilis:
  1. Backup DB live, terapkan migrasi `20261025090000_attendance_requests.sql` dan `20261025090100_short_hours.sql` (setelah `20261024090000_approval_atasan.sql`). Tidak perlu `iam.sync_hris_staff_roles()` (tidak ada menu/grant baru).
  2. Build ulang API (Go) dan web (Next).
  3. HR (atau atas keputusan user) mengisi jam kerja minimum 9 pada cabang HQ lewat Lokasi Toko, dan memastikan karyawan HQ punya penempatan di cabang HQ. Agent tidak mengubah data live.
  4. QA manusia atas langkah Step 2.
- Set epic `status: review` dan baris registry README `review`, tanggal hari itu.

- [ ] **Step 4: Commit**

```bash
git add docs/epics docs/hris/acuan-mom-20261019
git commit -m "docs: EPIC-065 paket absensi lanjutan selesai diimplementasikan, menunggu review

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

## Follow-up (di luar rencana ini)

- AB-07 (rules keterlambatan) menunggu TnC.
- Ringkasan harian absen terlewat untuk HR (R1 sengaja tidak membuatnya); bila diminta, sumber datanya sama (`AttendanceGaps` per karyawan).
- Pengajuan oleh HR atas nama karyawan tidak dibuat: HR memakai penyesuaian langsung yang sudah ada.
- Keputusan HR masih berbasis peran aplikasi (`IsHRD`) + menu `read`, bukan aksi `approve` di menu daun; menyelaraskannya dengan konvensi EPIC-061 adalah perubahan hak akses tersendiri (sama dengan catatan EPIC-063).
- Export Rekap belum memuat kolom label kurang jam.

## Self-review

**Cakupan spec:**
- AB-04 (pengajuan karyawan, keterangan wajib, foto opsional, HR memutus, jalur tulis sama, tolak dengan alasan, status terlihat) → Task 1 (`CheckAttendanceRequest`), Task 2 (buat/daftar/batal/lampiran), Task 3 (antrean/putus lewat `createManualAttendance`/`adjustAttendance`), Task 6–8 (UI).
- AB-06 / K-9 (alert absen terlewat + detail request, HR menerima notifikasi) → Task 1 (`AttendanceGaps`), Task 2 (`notify_hrd`), Task 3 (badge), Task 4 (endpoint), Task 7 (kartu), Task 8 (badge Next).
- Tanggal ≤ hari ini, periode terbuka, satu pending per tanggal → Task 1, Task 2 (index + cek), Task 3 (cek ulang saat setujui).
- Atasan langsung bukan pemutus → Task 3 (`boss` 403), R2.
- AB-05 (HQ 9 jam, label di baris, tampil di ESS & HR, notifikasi < 7 jam disesuaikan, tanpa payroll) → Task 5 (migrasi + endpoint + JSON), Task 9 (UI + Lokasi Toko).
- LA-01 → Task 2 (cancel), Task 3 (approve/reject + `attendance.*`), Task 5 (`branch_min_hours.update`).
- Dokumentasi & rilis → Task 10.

**Pemindaian placeholder:** tidak ada "TBD"/"TODO"; setiap langkah kode memuat kodenya; setiap perintah memuat Expected.

**Konsistensi tipe:** `CheckAttendanceRequest(AttendanceRequestInput, time.Time) (kind, msg string)` dipakai Task 2; `AttendanceGaps(GapInput) []AttendanceGap` dipakai Task 4; `ValidateMinWorkHours(*float64) string` dipakai Task 5; `AttendanceRequestRepo` dibuat Task 2 dan diperluas Task 3; `createManualAttendance`/`adjustAttendance` (Task 3) dipanggil `approveAttendanceRequest`; `openAttendanceWindow` (Task 4) dipakai Task 5; `attReqBody` (Task 2) dipakai Task 3 dan 4; TS `AttendanceRequest`, `AttendanceGaps`, `useDecideAttendanceRequest`, `attendanceRequestAttachmentUrl` (Task 6) dipakai Task 7–8; `AttendanceRequestStatusBadge`, `AttendanceRequestPrefill` (Task 7) dipakai Task 7–8; `ShortHoursBadge` (Task 9) memakai `shortHoursDetail` (Task 6); `ATTENDANCE_REQUESTS_HREF` (Task 8) = `attendanceHref` Go (Task 2).

**Review Focus:** kelima butir punya test di task pemiliknya (Task 1, 2, 3, 5).

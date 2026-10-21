# Paket Approval Atasan — Implementation Plan (EPIC-063)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Also load the project skill `paddy-hris-acuan` before starting.

**Goal:** Atasan langsung memutus cuti dan lembur bawahannya dari satu halaman ESS "Persetujuan Tim", dengan HR sebagai cadangan/override yang tercatat. Lembur wajib diajukan sebelum dimulai dan memilih jenis Weekday / Weekend-Libur, AdminHRIS bisa mengoreksi lembur dengan audit, dan tidak ada yang bisa menyetujui pengajuannya sendiri.

**Architecture:**
- Aturan murni di Go `backend/internal/modules/hris/domain/approval.go`: kapasitas pemutus (`DecideAs`: `manager` / `hr` / ditolak), aturan "sebelum dimulai" (jam WIB), saran jenis lembur dari jadwal + hari libur (aturan yang sama dengan payroll `SplitOvertimeWith`), dan pesan WA lembur.
- `DecideLeave` / `DecideOvertime` memakai `DecideAs`, menyimpan `decided_as` + `decided_by_name` (kolom baru), dan menulis `audit.audit_log` dalam transaksi yang sama. `SubordinateIDs`, antrean tim, dan `ManagerContact` mengabaikan `reporting_to` yang menunjuk diri sendiri.
- `CreateOvertime` menolak jam mulai yang lewat (pengajuan sendiri), menyimpan `day_type` dari ESS yang dicek ke jadwal (`GET /api/hris/overtime/day-type` untuk saran), dan menerbitkan event outbox `hris.overtime.requested` → WA atasan (helper `notifyManager` dipakai bersama dengan cuti).
- Koreksi lembur `PATCH /api/hris/overtime/{id}` dijaga `withAction("update", ["hris.kepegawaian.overtime"])`.
- Antrean `GET /api/hris/me/team-approvals` (cuti + lembur), badge `/dashboard/me/persetujuan-tim` di `nav-badges` (cuti + lembur + Perjalanan Dinas + kasbon tahap manager lewat port payroll baru).
- Kualitas data: tolak `reporting_to` = diri sendiri di tiga penulis (PUT `/api/hris/employees/{id}`, PUT `/api/users/{id}`, import "NIP Atasan"); laporan `GET /api/hris/employees/without-manager`.
- Frontend: fitur baru `frontend/src/features/hris/team-approvals` + halaman `/dashboard/me/persetujuan-tim`; komponen antrean Perjalanan Dinas diekstrak (`TeamTripApprovals`) dan dipakai dua halaman; form lembur ESS dan tabel Lembur HRIS diperbarui; kartu "Tanpa atasan" di Karyawan.
- Fallback Next (route TS yang melayani path yang sama) mendapat guard yang sama; endpoint baru hanya di Go (`goNativePrefixes`).

**Tech Stack:** Go (pgx, outbox, `audit.Write`), migrasi PostgreSQL, Next.js 16.3.8 (App Router, server page + client component, React Query), vitest + Testing Library.

**Spec:** `docs/epics/EPIC-063-approval-atasan.md` (Goal, Acceptance Criteria, Catatan) dan acuan `docs/hris/acuan-mom-20261019/` — `status-requirement.md` baris AP-01..AP-04; `pertanyaan-rekomendasi.md` Q-APV-3, K-13, urutan eksekusi langkah 5 "Paket Approval atasan"; `matriks-approval.md` (Time-off, Overtime, Koreksi Overtime).

**Prasyarat:**
- Branch kerja `feat/epic063-approval-atasan` (dari `main` `5035ae5`, yang berjalan di live). Tetap di branch ini.
- Q-APV-3 dan K-13 sudah diputuskan user (20-10-2026). Ruling R1–R11 di bawah adalah turunan yang diputuskan mengikuti rekomendasi; tidak ada pertanyaan terbuka.
- Bila memakai worktree tanpa `node_modules`: `ln -s /home/wit/Desktop/paddydev/frontend/node_modules frontend/node_modules` (tidak di-commit).

## Ruling

- **R1 — Kapasitas pemutus (Q-APV-3).** Approve/reject cuti dan lembur `source='employee'`: atasan langsung (`reporting_to`) → `decided_as='manager'`; selain itu HR (`Actor.IsHR`, himpunan role yang sama dengan sekarang) → `decided_as='hr'`. HR tetap boleh memutus walau atasan ada (override); tidak ada eskalasi waktu otomatis. Bila pemutus sekaligus atasan dan HR → `manager`. Setiap keputusan menyimpan `decided_by_name` (nama akun; `"HRD"` bila kosong) dan menulis audit `leave.approve|reject` / `overtime.approve|reject` (`after.decided_as`). Alasan: Q-APV-3 meminta HR cadangan **dan** override dengan audit; "atasan cuti" tidak bisa dideteksi andal, jadi override manual lebih aman daripada aturan waktu. Cost if wrong: bila Paddy ingin HR hanya saat atasan kosong, cukup ubah `DecideAs`.
- **R2 — Tanpa self-approval.** Tidak ada yang memutus pengajuannya sendiri, termasuk HR (403 `Tidak bisa memutuskan pengajuan cuti sendiri`; lembur sudah `Tidak bisa memutuskan pengajuan lembur sendiri`). `reporting_to` yang menunjuk diri sendiri tidak pernah membuat seseorang menjadi atasan: dikecualikan dari `SubordinateIDs`, antrean tim, badge, dan WA atasan.
- **R3 — "Sebelum dilaksanakan" (AP-02).** Pengajuan untuk diri sendiri (karyawan, juga HR untuk dirinya) ditolak bila `tanggal + jam mulai` (jam dinding WIB) **sebelum awal menit berjalan WIB**. Jam mulai di menit yang sedang berjalan masih diterima. Acuannya jam mulai lembur yang diisi, bukan jam shift: lembur bisa sebelum shift atau di hari tanpa shift. Penugasan HR untuk karyawan **lain** (`source='company'`) boleh tanggal/jam lampau, karena HR adalah pemegang data (mencatat lembur atas instruksi lisan saat ESS tidak bisa dipakai), karyawan tetap mengonfirmasi, payroll tetap mensyaratkan clock-out, dan koreksi AdminHRIS (AP-04) tersedia. Lembur malam lewat tengah malam memakai tanggal mulai.
- **R4 — Keputusan setelah jam mulai tetap boleh.** Aturan AP-02 berlaku saat pengajuan; atasan sering menyetujui setelah lembur dimulai.
- **R5 — Jenis lembur (K-13).** Weekday = `regular`, Weekend / Libur = `rest_day`. Tidak ada perubahan skema maupun kalkulator: payroll sudah membayar libur nasional dan hari tanpa shift di jadwal yang berlaku dengan tarif libur, dan `regular` tidak pernah menurunkannya. Saran jenis = aturan payroll (libur aktif → Weekend; jadwal berlaku: ada shift → Weekday, libur/tanpa baris → Weekend; tanpa jadwal berlaku → tidak diketahui). Pengajuan sendiri: jenis yang bertentangan dengan saran yang diketahui ditolak 400; tanpa jenis → saran, atau Weekday bila tidak diketahui; tanpa jadwal → pilihan karyawan dipakai. Penugasan HR untuk orang lain dan koreksi AdminHRIS bebas memilih. Alasan: mencegah Weekend (2×) diklaim di hari kerja terjadwal tanpa membuat karyawan tanpa jadwal terkunci.
- **R6 — Koreksi lembur (AP-04).** "AdminHRIS" = aksi `update` pada menu daun `hris.kepegawaian.overtime` (role HR live sudah memegangnya; tidak ada migrasi grant). Bisa dikoreksi: `date`, `start_time`, `end_time`, `day_type` milik pengajuan `pending`/`approved` orang lain (bukan milik sendiri). Jam dihitung ulang (1–12 jam); alasan wajib ≥ 5 karakter; status tidak berubah. Tanggal lama **atau** baru di periode absensi `closed` → 409. Tanggal baru yang sudah punya lembur aktif → 400 duplikat. Kolom `corrected_at`, `corrected_by_name`; audit `overtime.correct` (before/after/reason).
- **R7 — Persetujuan Tim = satu halaman, tanpa duplikasi.** Antrean cuti dan lembur baru (`GET /api/hris/me/team-approvals`). Perjalanan Dinas memakai komponen `TeamTripApprovals` yang diekstrak dari halaman Perjalanan Dinas (API + hook keputusan yang sama, follow-up EPIC-062). Kasbon tetap diputus di ESS → Pinjaman (dua tahap, nominal, tenor), Persetujuan Tim hanya menampilkan kartu tautan + jumlah. Menu `ess.team-approvals` di bawah `ess` dengan grant yang sama dengan `ess.team-shifts`. Badge = cuti + lembur + Perjalanan Dinas pending bawahan langsung + kasbon `pending` dengan `manager_approver_id` = aktor.
- **R8 — Notifikasi.** Kanal yang sama (outbox + `wa_notif_log`, `WhatsAppSender`). Pengajuan lembur sendiri (`source='employee'`) kini mengirim WA ke atasan langsung (sebelumnya tidak ada); penugasan HR tidak. Pesan cuti dan lembur menunjuk "Area Karyawan → Persetujuan Tim". WA tidak dikirim ke "atasan" yang sama dengan pemohon.
- **R9 — Kualitas data.** Validasi aplikasi di tiga penulis, tanpa CHECK DB: constraint `NOT VALID` tetap memblokir update kolom lain pada satu baris live yang menunjuk diri sendiri. Laporan baca-saja: karyawan aktif dengan `reporting_to` kosong, menunjuk diri sendiri, atau menunjuk karyawan nonaktif. Data live tidak diubah.
- **R10 — Paritas fallback Next.** Route TS yang melayani path yang sama mendapat guard R1–R3/R9: `POST /api/hris/leaves/approve`, `POST /api/hris/overtime`, `POST /api/hris/overtime/decide`, `GET /api/hris/overtime?scope=approvals`, `PUT /api/hris/employees/[id]`, `PUT /api/users/[id]`, WA cuti. Endpoint baru (team-approvals, day-type, koreksi, laporan) hanya di Go dan masuk `goNativePrefixes`. Pemilihan jenis lembur tidak diporting ke fallback Next (skema zod Next sudah tidak menerima `day_type`, perilaku lama).
- **R11 — Atasan tidak mengubah jenis saat menyetujui.** `day_type` saat approve tetap hanya untuk HR (perilaku lama). Jenis sudah dicek saat pengajuan; bila salah, atasan menolak atau AdminHRIS mengoreksi.

## Global Constraints

- **DB hanya DB test:** `TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable'`. **Jangan** port 55432, jangan DB live, jangan container docker lain. Migrasi ke DB test: `cd backend && MIGRATE_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go run ./cmd/migrate -apply`.
- **Go:** `$HOME/.local/go/bin/go`; `gofmt -l` bersih sebelum commit. `make -C backend generate` dijalankan dengan `PATH=$HOME/.local/go/bin:$PATH`.
- **Route Go baru:** setiap task yang menambah route menjalankan `cd backend && PATH=$HOME/.local/go/bin:$PATH make generate`, meng-commit `frontend/src/lib/go-routes.generated.json`, menambah path ke `goNativePrefixes` di `frontend/src/lib/backend-routes.test.ts`, lalu `cd frontend && npx vitest run src/lib/backend-routes.test.ts` PASS.
- **Nama migrasi:** satu file `backend/database/migrations/deltas/20261024090000_approval_atasan.sql` (setelah `20261023090100_business_trips.sql`). Menu baru wajib masuk daftar pertahanan `backend/database/seeders/iam-menus.sql`.
- **Test butuh IAM seeded:** CI memigrasi tanpa seeder. Test di rencana ini membuat menu/grant sendiri lewat `h.staff` / `testutil.CreateStaff`; bila menambah test yang bergantung pada role/menu seeded, panggil `testutil.RequireSeededIAM` dulu.
- **Baseline:** sebelum Task 1, simpan test Go yang sudah gagal: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test -p 1 ./... -count=1 2>&1 | grep -- '--- FAIL' | sed 's/ (.*//' | sort -u > /tmp/epic063-baseline.txt`. Kegagalan yang sama di akhir bukan regresi.
- **Next.js:** baca `frontend/AGENTS.md`. Halaman = Server Component `async` (`node_modules/next/dist/docs/01-app/01-getting-started/03-layouts-and-pages.md`), interaktivitas di Client Component `"use client"` (`05-server-and-client-components.md`), tautan internal `next/link` (`04-linking-and-navigating.md`).
- **Pesan (persis, test mengunci string):**
  - `Tidak bisa memutuskan pengajuan cuti sendiri` (403) · `Tidak bisa memutuskan pengajuan lembur sendiri` (403, sudah ada) · `Hanya atasan langsung atau HRD yang bisa memproses pengajuan ini` (403)
  - `Pengajuan sudah diproses oleh orang lain` (409)
  - `Lembur wajib diajukan sebelum dimulai — jam mulai sudah lewat` (400)
  - `Jenis lembur tanggal ini Weekday sesuai jadwal Anda` / `Jenis lembur tanggal ini Weekend / Libur sesuai jadwal Anda` (400) · `Tanggal tidak valid` (400)
  - `Alasan koreksi minimal 5 karakter` · `Tidak ada perubahan` · `Hanya lembur yang menunggu atau disetujui yang bisa dikoreksi` (400) · `Tidak bisa mengoreksi lembur sendiri` (403) · `Periode absensi DD-MM-YYYY s.d. DD-MM-YYYY sudah di-closing — lembur tidak bisa dikoreksi` (409) · sukses `Lembur dikoreksi`
  - `Atasan langsung tidak boleh karyawan itu sendiri` (400) · import: `NIP atasan tidak boleh NIP karyawan itu sendiri`
  - Label: `Weekday`, `Weekend / Libur`; menu `Persetujuan Tim`; footer WA `Mohon ditinjau di dashboard Paddy OS (Area Karyawan → Persetujuan Tim).` (fallback Next: `Arkiv OS`).
- **Aturan user:** yang sudah benar tidak diubah; jangan refaktor di luar file yang disebut tiap task. Setting dan data live (`reporting_to`, tarif lembur) adalah keputusan user: rencana ini tidak mengubah data live.
- **Commit:** satu per task, pesan diakhiri baris kosong lalu `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Tanpa push.

## Review Focus

1. **`reporting_to` menunjuk diri sendiri (1 baris live, Retail Operation Manager):** orang itu tidak boleh melihat pengajuannya di antrean timnya, tidak boleh menyetujuinya, dan tidak menerima WA "atasan" untuk pengajuannya sendiri. Diuji di Task 2 (`TestLeaveSelfManagerCannotApproveOwn`) dan Task 5 (`TestTeamApprovalsQueueAndBadge`).
2. **HR mengajukan cuti/lembur untuk dirinya sendiri lalu mencoba menyetujui:** harus 403, bukan lolos sebagai "HR". Diuji di Task 2 (`TestLeaveDecisionManagerFirstHRBackup`) dan Task 3 (`TestOvertimeMustBeRequestedBeforeStart`, kasus HR lampau).
3. **Batas waktu WIB vs UTC:** pengajuan pukul 23:45 WIB untuk lembur 00:15 tanggal berikutnya (tanggal UTC masih hari sebelumnya) harus diterima; 23:30 hari yang sama ditolak; menit berjalan diterima. Diuji di Task 1 (`TestOvertimeStartsInPast`).
4. **Koreksi memindahkan lembur ke tanggal yang sudah punya lembur aktif:** harus 400 duplikat, bukan 500 dari unique index. Diuji di Task 4 (`TestOvertimeCorrectionByAdminHRIS`, kasus "tanggal bentrok").
5. **Antrean basi:** HR sudah memutus, lalu atasan menekan Setujui dari halaman yang belum disegarkan → 400 `Leave request already rejected` (atau 409 saat balapan), bukan menimpa keputusan HR. Diuji di Task 2 (`TestLeaveDecisionManagerFirstHRBackup`, kasus basi).

## Peta file

| File | Tanggung jawab |
|---|---|
| `backend/internal/modules/hris/domain/approval.go` (baru) | `DecideAs`, `OvertimeStartsInPast`, `OvertimeDayTypeFor`, label & pesan WA lembur |
| `backend/internal/modules/hris/approvals.go` (baru) | `decisionRace`, `auditDecision`, `notifyManager` + handler outbox lembur, antrean tim (repo/service/handler) |
| `backend/internal/modules/hris/overtime_daytype.go` (baru) | saran & validasi jenis lembur, `GET /api/hris/overtime/day-type` |
| `backend/internal/modules/hris/overtime_correction.go` (baru) | koreksi AdminHRIS (service, SQL, handler) |
| `backend/internal/modules/hris/reporting_to.go` (baru) | `selfManager`, laporan karyawan tanpa atasan |
| `backend/database/migrations/deltas/20261024090000_approval_atasan.sql` (baru) | kolom `decided_as`/`decided_by_name`/`corrected_*`, menu `ess.team-approvals` |
| `frontend/src/features/hris/team-approvals/**` (baru) | halaman ESS Persetujuan Tim |
| `frontend/src/features/hris/business-trips/components/team-trip-approvals.tsx` (baru) | antrean Perjalanan Dinas tim (diekstrak) |
| `frontend/src/features/hris/overtime/components/overtime-correction-dialog.tsx` (baru) | dialog koreksi HRIS |
| `frontend/src/features/users/components/no-manager-card.tsx` (baru) | kartu + dialog "Tanpa atasan" |
| `frontend/src/lib/hris/reporting-to.ts` (baru) | `isSelfManager`, `managerOptions` |

---

### Task 1: Aturan approval & lembur (domain Go murni)

**Files:**
- Create: `backend/internal/modules/hris/domain/approval.go`
- Modify: `backend/internal/modules/hris/domain/workforce.go:162` (footer pesan WA cuti)
- Test: `backend/internal/modules/hris/domain/approval_test.go`

**Interfaces:**
- Consumes: `samePerson`, `tanggalID`, `JSTrim`, `LeaveRequestWaMessage` (`domain/workforce.go`), `NowWIB` (`domain/calendar.go`), `ScheduleRow`, `ResolveScheduleRow` (`domain/schedule.go`).
- Produces:
  - `const DecidedAsManager = "manager"`, `DecidedAsHR = "hr"`
  - `type Approver struct { EmployeeID *string; IsHR bool }`
  - `func DecideAs(a Approver, ownerID string, reportingTo *string, noun string) (decidedAs, refusal string)`
  - `func OvertimeStartsInPast(date, start string, now time.Time) bool`
  - `const OvertimeWeekday = "regular"`, `OvertimeWeekend = "rest_day"`
  - `func OvertimeDayTypeFor(date string, holiday bool, schedule []ScheduleRow) (dayType string, known bool)`
  - `func OvertimeDayTypeLabel(dayType string) string`
  - `type OvertimeRequestWa struct { EmployeeName, Date, StartTime, EndTime, DayType string; Hours float64; Reason, ApproverName *string }`
  - `func OvertimeRequestWaMessage(in OvertimeRequestWa) string`
  - `const ApprovalWaFooter = "Mohon ditinjau di dashboard Paddy OS (Area Karyawan → Persetujuan Tim)."`

- [ ] **Step 1: Tulis test yang gagal**

`backend/internal/modules/hris/domain/approval_test.go`:

```go
package domain

import (
	"strings"
	"testing"
	"time"
)

func TestDecideAs(t *testing.T) {
	boss, hr, crew := sp("boss"), sp("hr"), "crew"
	check := func(name string, a Approver, reportingTo *string, as, refusal string) {
		t.Helper()
		gotAs, gotRefusal := DecideAs(a, crew, reportingTo, "cuti")
		if gotAs != as || gotRefusal != refusal {
			t.Errorf("%s: got (%q, %q), want (%q, %q)", name, gotAs, gotRefusal, as, refusal)
		}
	}
	check("atasan langsung", Approver{EmployeeID: boss}, boss, DecidedAsManager, "")
	check("atasan yang juga HR tetap manager", Approver{EmployeeID: boss, IsHR: true}, boss, DecidedAsManager, "")
	check("HR cadangan, pemohon tanpa atasan", Approver{EmployeeID: hr, IsHR: true}, nil, DecidedAsHR, "")
	check("HR override", Approver{EmployeeID: hr, IsHR: true}, boss, DecidedAsHR, "")
	check("HR tanpa data karyawan", Approver{IsHR: true}, boss, DecidedAsHR, "")
	check("bukan atasan, bukan HR", Approver{EmployeeID: sp("lain")}, boss, "",
		"Hanya atasan langsung atau HRD yang bisa memproses pengajuan ini")
	check("HR untuk pengajuannya sendiri", Approver{EmployeeID: sp(crew), IsHR: true}, boss, "",
		"Tidak bisa memutuskan pengajuan cuti sendiri")
	check("reporting_to menunjuk diri sendiri", Approver{EmployeeID: sp(crew)}, sp(crew), "",
		"Tidak bisa memutuskan pengajuan cuti sendiri")
}

func TestOvertimeStartsInPast(t *testing.T) {
	now := time.Date(2026, 10, 5, 3, 0, 30, 0, time.UTC) // Senin 10:00:30 WIB
	eq(t, OvertimeStartsInPast("2026-10-05", "10:00", now), false) // menit berjalan masih boleh
	eq(t, OvertimeStartsInPast("2026-10-05", "09:59", now), true)
	eq(t, OvertimeStartsInPast("2026-10-04", "23:00", now), true)
	eq(t, OvertimeStartsInPast("2026-10-06", "00:00", now), false)
	eq(t, OvertimeStartsInPast("2026-10-05", "1000", now), false) // format diserahkan ke validasi input
	late := time.Date(2026, 10, 5, 16, 45, 0, 0, time.UTC) // 23:45 WIB, tanggal UTC masih 05-10
	eq(t, OvertimeStartsInPast("2026-10-06", "00:15", late), false)
	eq(t, OvertimeStartsInPast("2026-10-05", "23:30", late), true)
	eq(t, OvertimeStartsInPast("2026-10-05", "23:45", late), false)
}

func TestOvertimeDayTypeFor(t *testing.T) {
	mondays := []ScheduleRow{{DayOfWeek: 1, ShiftID: sp("pagi"), EffectiveFrom: "2026-01-01"}}
	check := func(name, date string, holiday bool, rows []ScheduleRow, want string, known bool) {
		t.Helper()
		got, ok := OvertimeDayTypeFor(date, holiday, rows)
		if got != want || ok != known {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", name, got, ok, want, known)
		}
	}
	check("Senin ada shift", "2026-10-12", false, mondays, OvertimeWeekday, true)
	check("Minggu tanpa baris jadwal", "2026-10-11", false, mondays, OvertimeWeekend, true)
	check("libur nasional di hari kerja", "2026-10-12", true, mondays, OvertimeWeekend, true)
	check("libur nasional tanpa jadwal", "2026-10-12", true, nil, OvertimeWeekend, true)
	check("tanpa jadwal", "2026-10-12", false, nil, "", false)
	check("jadwal belum berlaku", "2025-12-29", false, mondays, "", false)
	dayOff := append([]ScheduleRow{{DayOfWeek: 1, EffectiveFrom: "2026-10-12", EffectiveTo: sp("2026-10-12")}}, mondays...)
	check("libur per tanggal mengalahkan pola", "2026-10-12", false, dayOff, OvertimeWeekend, true)
	eq(t, OvertimeDayTypeLabel(OvertimeWeekday), "Weekday")
	eq(t, OvertimeDayTypeLabel(OvertimeWeekend), "Weekend / Libur")
}

func TestOvertimeRequestWaMessage(t *testing.T) {
	msg := OvertimeRequestWaMessage(OvertimeRequestWa{EmployeeName: "Crew Lembur", Date: "2026-10-06",
		StartTime: "17:00", EndTime: "19:30", Hours: 2.5, DayType: OvertimeWeekday,
		Reason: sp(" Stock opname "), ApproverName: sp("Atasan Lembur")})
	eq(t, msg, "Paddy OS — Pengajuan Lembur\nKepada: Atasan Lembur\nKaryawan : Crew Lembur\n"+
		"Tanggal : Sel, 06 Okt 2026, 17:00–19:30 (2,5 jam, Weekday)\nAlasan : Stock opname\n\n"+ApprovalWaFooter)
	leave := LeaveRequestWaMessage(LeaveRequestWa{EmployeeName: "Crew", LeaveType: "annual",
		StartDate: "2026-10-06", EndDate: "2026-10-06", TotalDays: 1})
	if !strings.HasSuffix(leave, "\n\n"+ApprovalWaFooter) {
		t.Fatalf("pesan cuti harus mengarah ke Persetujuan Tim: %q", leave)
	}
}
```

(`eq` dan `sp` sudah ada di `domain_test.go`.)

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd backend && $HOME/.local/go/bin/go test ./internal/modules/hris/domain -run 'TestDecideAs|TestOvertimeStartsInPast|TestOvertimeDayTypeFor|TestOvertimeRequestWaMessage' -count=1`
Expected: FAIL kompilasi `undefined: Approver` / `undefined: DecideAs`.

- [ ] **Step 3: Tulis implementasi**

`backend/internal/modules/hris/domain/approval.go`:

```go
package domain

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Approval atasan langsung (EPIC-063, MOM AP-01..AP-03, Q-APV-3, K-13).

// Decision capacities stored in decided_as.
const (
	DecidedAsManager = "manager"
	DecidedAsHR      = "hr"
)

// Approver is who decides a leave or overtime request.
type Approver struct {
	EmployeeID *string
	IsHR       bool
}

// DecideAs is the capacity an approver decides ownerID's request in (Q-APV-3,
// EPIC-063 R1/R2): "manager" for the direct manager (reportingTo), else "hr"
// for HR, the backup that may also override. Nobody decides their own
// request, and a reporting_to pointing at the owner never makes the owner a
// manager. decidedAs is "" with the refusal otherwise; noun names the
// request ("cuti", "lembur").
func DecideAs(a Approver, ownerID string, reportingTo *string, noun string) (decidedAs, refusal string) {
	if samePerson(a.EmployeeID, &ownerID) {
		return "", "Tidak bisa memutuskan pengajuan " + noun + " sendiri"
	}
	if samePerson(a.EmployeeID, reportingTo) {
		return DecidedAsManager, ""
	}
	if a.IsHR {
		return DecidedAsHR, ""
	}
	return "", "Hanya atasan langsung atau HRD yang bisa memproses pengajuan ini"
}

// OvertimeStartsInPast is MOM AP-02 (EPIC-063 R3): date + start ("HH:MM")
// on the WIB wall clock is before the start of the current WIB minute. A
// start in the current minute has not passed; unparsable input is left to
// the input validation (false).
func OvertimeStartsInPast(date, start string, now time.Time) bool {
	t, err := time.Parse("2006-01-02 15:04", date+" "+start)
	if err != nil {
		return false
	}
	return t.Before(NowWIB(now).Truncate(time.Minute))
}

// Overtime day types (K-13): Weekday = regular, Weekend / Libur = rest_day.
const (
	OvertimeWeekday = "regular"
	OvertimeWeekend = "rest_day"
)

// OvertimeDayTypeFor suggests the overtime type of date with the rule payroll
// pays by (payroll/domain SplitOvertimeWith, EPIC-063 R5): an active public
// holiday, or a day without a shift in a schedule in force, is Weekend /
// Libur; a scheduled shift is Weekday. known is false when no schedule is in
// force on date and it is not a holiday.
func OvertimeDayTypeFor(date string, holiday bool, schedule []ScheduleRow) (dayType string, known bool) {
	if holiday {
		return OvertimeWeekend, true
	}
	inForce := false
	for _, r := range schedule {
		if r.EffectiveFrom <= date && (r.EffectiveTo == nil || *r.EffectiveTo >= date) {
			inForce = true
			break
		}
	}
	if !inForce {
		return "", false
	}
	if i := ResolveScheduleRow(schedule, date); i < 0 || schedule[i].ShiftID == nil {
		return OvertimeWeekend, true
	}
	return OvertimeWeekday, true
}

// OvertimeDayTypeLabel is the ESS label of a day type.
func OvertimeDayTypeLabel(dayType string) string {
	if dayType == OvertimeWeekend {
		return "Weekend / Libur"
	}
	return "Weekday"
}

// ApprovalWaFooter points the direct manager to the ESS queue (EPIC-063 R8).
const ApprovalWaFooter = "Mohon ditinjau di dashboard Paddy OS (Area Karyawan → Persetujuan Tim)."

// OvertimeRequestWa is the input of the manager's overtime notification.
type OvertimeRequestWa struct {
	EmployeeName, Date, StartTime, EndTime, DayType string
	Hours                                          float64
	Reason, ApproverName                           *string
}

// OvertimeRequestWaMessage tells the direct manager an overtime request waits
// for them (EPIC-063 R8).
func OvertimeRequestWaMessage(in OvertimeRequestWa) string {
	lines := []string{"Paddy OS — Pengajuan Lembur"}
	if in.ApproverName != nil && *in.ApproverName != "" {
		lines = append(lines, "Kepada: "+*in.ApproverName)
	}
	hours := strings.ReplaceAll(strconv.FormatFloat(in.Hours, 'f', -1, 64), ".", ",")
	lines = append(lines, "Karyawan : "+in.EmployeeName,
		fmt.Sprintf("Tanggal : %s, %s–%s (%s jam, %s)", tanggalID(in.Date), in.StartTime, in.EndTime,
			hours, OvertimeDayTypeLabel(in.DayType)))
	if in.Reason != nil && JSTrim(*in.Reason) != "" {
		lines = append(lines, "Alasan : "+JSTrim(*in.Reason))
	}
	lines = append(lines, "", ApprovalWaFooter)
	return strings.Join(lines, "\n")
}
```

`backend/internal/modules/hris/domain/workforce.go:162`, ganti:

```go
	lines = append(lines, "", "Mohon ditinjau di dashboard Paddy OS (Kehadiran & Cuti → Cuti & Izin).")
```

dengan:

```go
	lines = append(lines, "", ApprovalWaFooter)
```

- [ ] **Step 4: Jalankan test, pastikan lulus**

Run: `cd backend && $HOME/.local/go/bin/go test ./internal/modules/hris/domain -count=1`
Expected: PASS (seluruh paket domain).

- [ ] **Step 5: Commit**

```bash
gofmt -l backend/internal/modules/hris/domain
git add backend/internal/modules/hris/domain/approval.go backend/internal/modules/hris/domain/approval_test.go backend/internal/modules/hris/domain/workforce.go
git commit -m "feat(hris): aturan approval atasan, lembur sebelum dimulai, jenis Weekday/Weekend (EPIC-063 Task 1)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: Migrasi + keputusan cuti — atasan dulu, HR cadangan tercatat, tanpa self-approval

**Files:**
- Create: `backend/database/migrations/deltas/20261024090000_approval_atasan.sql`
- Modify: `backend/database/seeders/iam-menus.sql:248` (daftar pertahanan menu)
- Create: `backend/internal/modules/hris/approvals.go`
- Modify: `backend/internal/modules/hris/leaves.go` (interface `LeaveRepo.DecideLeave`, `DecideLeave`, tipe `leaveVerdict`)
- Modify: `backend/internal/modules/hris/postgres_leaves.go:157-172` (`DecideLeave`), `:218-229` (`ManagerContact`)
- Modify: `backend/internal/modules/hris/overtime.go:346` (`SubordinateIDs`)
- Modify: `backend/internal/modules/hris/http_leaves.go:132` (teruskan `auditMeta(r)`)
- Test: `backend/internal/modules/hris/leave_approval_integration_test.go`

**Interfaces:**
- Consumes: `domain.DecideAs`, `domain.Approver` (Task 1); `actorName`, `ptr` (`attendance_periods.go`), `auditMeta` (`attendance_adjust.go`), `errNoRow` (`sqlbuild.go`).
- Produces:
  - Kolom `hris.leaves.decided_as text CHECK IN ('manager','hr')`, `hris.leaves.decided_by_name text`; `hris.overtime_requests.decided_as`, `decided_by_name`, `corrected_at timestamptz`, `corrected_by_name text`; menu `ess.team-approvals` (`/dashboard/me/persetujuan-tim`).
  - `const decisionRace = "Pengajuan sudah diproses oleh orang lain"`
  - `func auditDecision(ctx context.Context, q database.Querier, a *Actor, action, entity, id, label string, after map[string]any, meta audit.Entry) error`
  - `type leaveVerdict struct { Status string; ApproverID, Rejection *string; DecidedAs, DecidedBy string }`
  - `LeaveRepo.DecideLeave(ctx context.Context, id string, v leaveVerdict, credit *quotaMove) error` (mengembalikan `errNoRow` bila 0 baris)
  - `func (s *Service) DecideLeave(ctx context.Context, a *Actor, in leaveDecision, meta audit.Entry) (*Row, error)`
  - Helper test `func leaveBody(start, end string) map[string]any` (dipakai Task 5)

- [ ] **Step 1: Tulis migrasi dan terapkan ke DB test**

`backend/database/migrations/deltas/20261024090000_approval_atasan.sql`:

```sql
-- =============================================================================
-- Paket Approval atasan (EPIC-063, MOM AP-01..AP-04, Q-APV-3, K-13).
--   decided_as: kapasitas pemutus pengajuan karyawan — 'manager' (atasan
--     langsung) atau 'hr' (HR sebagai cadangan / override). NULL = keputusan
--     sebelum EPIC-063, konfirmasi atau pembatalan penugasan lembur.
--   decided_by_name: nama akun pemutus (HR tanpa data karyawan tetap tercatat).
--   overtime_requests.corrected_at / corrected_by_name: koreksi AdminHRIS
--     terakhir; rincian sebelum/sesudah dan alasan ada di audit.audit_log.
--   Menu ESS → Persetujuan Tim (ess.team-approvals).
-- =============================================================================

ALTER TABLE hris.leaves
  ADD COLUMN IF NOT EXISTS decided_as      text,
  ADD COLUMN IF NOT EXISTS decided_by_name text;

DO $$ BEGIN
  ALTER TABLE hris.leaves ADD CONSTRAINT leaves_decided_as_check
    CHECK (decided_as IN ('manager', 'hr'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

ALTER TABLE hris.overtime_requests
  ADD COLUMN IF NOT EXISTS decided_as        text,
  ADD COLUMN IF NOT EXISTS decided_by_name   text,
  ADD COLUMN IF NOT EXISTS corrected_at      timestamptz,
  ADD COLUMN IF NOT EXISTS corrected_by_name text;

DO $$ BEGIN
  ALTER TABLE hris.overtime_requests ADD CONSTRAINT overtime_requests_decided_as_check
    CHECK (decided_as IN ('manager', 'hr'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

COMMENT ON COLUMN hris.leaves.decided_as IS
  'EPIC-063: manager = atasan langsung, hr = HR cadangan/override (Q-APV-3).';
COMMENT ON COLUMN hris.overtime_requests.decided_as IS
  'EPIC-063: manager = atasan langsung, hr = HR cadangan/override (Q-APV-3).';

-- ── Menu ESS → Persetujuan Tim ───────────────────────────────────────────────
INSERT INTO iam.menus (code, menu_name, route_path, icon, menu_type, order_number, permission_context)
VALUES ('ess.team-approvals', 'Persetujuan Tim', '/dashboard/me/persetujuan-tim', 'check-circle', 'sidebar', 44,
        '{"actions":["read"]}'::jsonb)
ON CONFLICT (code) DO UPDATE SET
  menu_name = EXCLUDED.menu_name, route_path = EXCLUDED.route_path,
  icon = EXCLUDED.icon, menu_type = EXCLUDED.menu_type,
  order_number = EXCLUDED.order_number, is_active = true, is_visible = true,
  deleted_at = NULL, updated_at = now();

UPDATE iam.menus SET module = 'ess', level = 2 WHERE code = 'ess.team-approvals';
UPDATE iam.menus child SET parent_id = parent.id
FROM iam.menus parent
WHERE child.code = 'ess.team-approvals' AND parent.code = 'ess';

-- Hak akses mengikuti Shift Tim (menu atasan lain di ESS): role yang sama, aksi read.
INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions)
SELECT rmp.role_id, m.id, '["read"]'::jsonb
FROM iam.role_menu_permissions rmp
JOIN iam.menus src ON src.id = rmp.menu_id AND src.code = 'ess.team-shifts'
CROSS JOIN iam.menus m
WHERE m.code = 'ess.team-approvals' AND rmp.is_active
ON CONFLICT (role_id, menu_id) DO UPDATE SET
  is_active = true, granted_actions = EXCLUDED.granted_actions, updated_at = now();
```

`backend/database/seeders/iam-menus.sql:248`, ganti:

```sql
    'ess.announcements', 'ess.team-shifts', 'ess.business-trip',
```

dengan:

```sql
    'ess.announcements', 'ess.team-shifts', 'ess.business-trip', 'ess.team-approvals',
```

Run: `cd backend && MIGRATE_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go run ./cmd/migrate -apply`
Expected: `20261024090000_approval_atasan.sql` diterapkan tanpa error. Cek: `psql 'postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' -Atc "SELECT count(*) FROM iam.role_menu_permissions p JOIN iam.menus m ON m.id = p.menu_id WHERE m.code = 'ess.team-approvals'"` → sama dengan jumlah grant `ess.team-shifts` (11 di DB test 21-10-2026).

- [ ] **Step 2: Tulis test yang gagal**

`backend/internal/modules/hris/leave_approval_integration_test.go`:

```go
package hris

import (
	"context"
	"testing"
)

func leaveBody(start, end string) map[string]any {
	return map[string]any{"leave_type": "annual", "start_date": start, "end_date": end, "reason": "Acara keluarga di kampung"}
}

func decideLeaveBody(id any, action, reason string) map[string]any {
	body := map[string]any{"leave_id": id, "action": action}
	if reason != "" {
		body["rejection_reason"] = reason
	}
	return body
}

// EPIC-063 R1/R2 (Q-APV-3): atasan langsung memutus lebih dulu, HR cadangan &
// override tercatat; tidak ada yang memutus cutinya sendiri.
func TestLeaveDecisionManagerFirstHRBackup(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuWorkforce}, true, emp{name: "HR Cadangan"})
	boss, bossID := h.staff("pos", nil, true, emp{name: "Atasan Langsung"})
	staff, _ := h.staff("pos", nil, true, emp{name: "Crew Cuti", boss: &bossID})
	stranger, _ := h.staff("pos", nil, true, emp{name: "Rekan Lain"})

	first := h.do(&staff, "POST", "/api/hris/leaves", leaveBody("2031-03-04", "2031-03-04"))
	expect(t, first, 200, "")
	firstID := first.data()["id"].(string)
	expect(t, h.do(&stranger, "POST", "/api/hris/leaves/approve", decideLeaveBody(firstID, "approve", "")), 403,
		"Hanya atasan langsung atau HRD yang bisa memproses pengajuan ini")
	approved := h.do(&boss, "POST", "/api/hris/leaves/approve", decideLeaveBody(firstID, "approve", ""))
	expect(t, approved, 200, "")
	if d := approved.data(); d["decided_as"] != "manager" || d["decided_by_name"] == nil || d["approved_by"] != bossID {
		t.Fatalf("manager approve = %s", approved.raw)
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'leave.approve'
		AND entity = 'hris.leaves' AND entity_id = $1 AND after->>'decided_as' = 'manager'`, firstID); n != "1" {
		t.Fatalf("audit manager approve = %s", n)
	}

	// HR override walau atasan ada; atasan yang telat menekan Setujui ditolak.
	second := h.do(&staff, "POST", "/api/hris/leaves", leaveBody("2031-03-11", "2031-03-11"))
	expect(t, second, 200, "")
	secondID := second.data()["id"].(string)
	override := h.do(&hr, "POST", "/api/hris/leaves/approve", decideLeaveBody(secondID, "reject", "Stok opname akhir bulan"))
	expect(t, override, 200, "")
	if d := override.data(); d["decided_as"] != "hr" || d["status"] != "rejected" {
		t.Fatalf("HR override = %s", override.raw)
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'leave.reject'
		AND entity_id = $1 AND after->>'decided_as' = 'hr' AND actor_id::text = $2`, secondID, hr.UserID); n != "1" {
		t.Fatalf("audit HR reject = %s", n)
	}
	expect(t, h.do(&boss, "POST", "/api/hris/leaves/approve", decideLeaveBody(secondID, "approve", "")), 400,
		"Leave request already rejected")

	// HR tidak memutus cutinya sendiri.
	mine := h.do(&hr, "POST", "/api/hris/leaves", leaveBody("2031-03-18", "2031-03-18"))
	expect(t, mine, 200, "")
	expect(t, h.do(&hr, "POST", "/api/hris/leaves/approve", decideLeaveBody(mine.data()["id"], "approve", "")), 403,
		"Tidak bisa memutuskan pengajuan cuti sendiri")
}

// Data live: satu karyawan reporting_to-nya dirinya sendiri (EPIC-063 R2).
func TestLeaveSelfManagerCannotApproveOwn(t *testing.T) {
	h := newHarness(t)
	loop, loopID := h.staff("pos", nil, true, emp{name: "Retail Ops Loop", phone: "0812 1111 2222"})
	h.exec(`UPDATE hris.employees SET reporting_to = id WHERE id = $1`, loopID)

	leave := h.do(&loop, "POST", "/api/hris/leaves", leaveBody("2031-03-04", "2031-03-04"))
	expect(t, leave, 200, "")
	if _, err := h.deps.Events.Dispatch(context.Background(), h.tx); err != nil {
		t.Fatal(err)
	}
	if len(h.wa.sent) != 0 {
		t.Fatalf("pemohon tidak boleh menerima WA sebagai atasannya sendiri: %v", h.wa.sent)
	}
	expect(t, h.do(&loop, "POST", "/api/hris/leaves/approve", decideLeaveBody(leave.data()["id"], "approve", "")), 403,
		"Tidak bisa memutuskan pengajuan cuti sendiri")

	expect(t, h.do(&loop, "POST", "/api/hris/overtime", map[string]any{
		"date": "2031-03-05", "start_time": "17:00", "end_time": "19:00", "reason": "Closing stok"}), 200, "")
	team := h.do(&loop, "GET", "/api/hris/overtime?scope=approvals", nil)
	expect(t, team, 200, "")
	if len(team.list()) != 0 {
		t.Fatalf("lembur sendiri tidak boleh masuk antrean atasan: %s", team.raw)
	}
}
```

- [ ] **Step 3: Jalankan test, pastikan gagal**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris -run 'TestLeaveDecisionManagerFirstHRBackup|TestLeaveSelfManagerCannotApproveOwn' -count=1 -v`
Expected: FAIL — `manager approve = ...` (tidak ada `decided_as`) dan pada test kedua status 200, bukan 403.

- [ ] **Step 4: Tulis implementasi**

`backend/internal/modules/hris/approvals.go`:

```go
package hris

import (
	"context"

	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/database"
)

// Paket Approval atasan (EPIC-063): what leave and overtime decisions share.

// decisionRace is the 409 of a request another approver decided meanwhile.
const decisionRace = "Pengajuan sudah diproses oleh orang lain"

// auditDecision records a decision with its capacity (Q-APV-3: who decided,
// as direct manager or as HR) on q, inside the decision's transaction (LA-01).
func auditDecision(ctx context.Context, q database.Querier, a *Actor, action, entity, id, label string,
	after map[string]any, meta audit.Entry) error {
	meta.ActorID, meta.ActorName = a.UserID, ptr(actorName(a))
	meta.Action, meta.Entity, meta.EntityID, meta.EntityLabel = action, entity, &id, &label
	meta.Before = map[string]any{"status": "pending"}
	meta.After = after
	return audit.Write(ctx, q, meta)
}
```

`backend/internal/modules/hris/leaves.go` — tambahkan `"errors"` dan `"nuhabit/backend/internal/platform/audit"` ke import; ganti baris interface

```go
	DecideLeave(ctx context.Context, id, status string, approverID, rejection *string, credit *quotaMove) error
```

dengan

```go
	// DecideLeave writes the verdict of a pending request; errNoRow when it
	// is no longer pending (decided meanwhile).
	DecideLeave(ctx context.Context, id string, v leaveVerdict, credit *quotaMove) error
```

dan tambahkan di bawah `type quotaMove`:

```go
// leaveVerdict is what DecideLeave writes (EPIC-063 R1).
type leaveVerdict struct {
	Status                string
	ApproverID, Rejection *string
	DecidedAs, DecidedBy  string
}
```

Ganti seluruh fungsi `DecideLeave` (komentar sampai sebelum `var name, phone *string`) dengan:

```go
// DecideLeave is decideLeave with the EPIC-063 rules (Q-APV-3): the direct
// manager decides first, HR is the backup and may override, nobody decides
// their own request; the capacity and the decider's name are stored and
// audited. Approving annual leave consumes the quota in the same
// transaction. The response carries a wa.me link for the approver.
func (s *Service) DecideLeave(ctx context.Context, a *Actor, in leaveDecision, meta audit.Entry) (*Row, error) {
	leave, err := s.repo.LeaveForDecision(ctx, in.LeaveID)
	if err != nil {
		return nil, err
	}
	if leave == nil {
		return nil, httpx.NotFound("Leave request not found")
	}
	emp := leave.Child("employee")
	var reportingTo *string
	if emp != nil {
		reportingTo = emp.StrPtr("reporting_to")
	}
	decidedAs, refusal := domain.DecideAs(domain.Approver{EmployeeID: a.EmployeeID, IsHR: a.IsHR},
		leave.Str("employee_id"), reportingTo, "cuti")
	if decidedAs == "" {
		return nil, httpx.Forbidden(refusal)
	}
	if status := leave.Str("status"); status != "pending" {
		return nil, httpx.BadRequest("Leave request already "+status, obj("current_status", status))
	}
	if in.Action == "reject" && (in.Rejection == nil || *in.Rejection == "") {
		return nil, httpx.BadRequest("Rejection reason is required")
	}
	approve := in.Action == "approve"
	status, rejection := "rejected", in.Rejection
	var credit *quotaMove
	if approve {
		status, rejection = "approved", nil
		if leave.Str("leave_type") == "annual" {
			credit = &quotaMove{EmployeeID: leave.Str("employee_id"), Year: dateYear(leave, "start_date"), Days: leave.Str("total_days")}
		}
	}
	verdict := leaveVerdict{Status: status, ApproverID: a.EmployeeID, Rejection: rejection,
		DecidedAs: decidedAs, DecidedBy: actorName(a)}
	label := dateText(leave, "start_date") + " s.d. " + dateText(leave, "end_date")
	if emp != nil {
		label = emp.Str("full_name") + " · " + label
	}
	err = s.repo.InTx(ctx, func(r Repository) error {
		if err := r.DecideLeave(ctx, in.LeaveID, verdict, credit); err != nil {
			return err
		}
		return auditDecision(ctx, r.querier(), a, "leave."+in.Action, "hris.leaves", in.LeaveID, label,
			map[string]any{"status": status, "decided_as": decidedAs, "rejection_reason": rejection}, meta)
	})
	if errors.Is(err, errNoRow) {
		return nil, httpx.Conflict(decisionRace)
	}
	if err != nil {
		return nil, err
	}
	data, err := s.repo.LeaveDecided(ctx, in.LeaveID)
	if err != nil {
		return nil, err
	}
```

(sisa fungsi mulai `var name, phone *string` tidak berubah.)

`backend/internal/modules/hris/postgres_leaves.go`, ganti `DecideLeave` (baris 157–172) dengan:

```go
func (s *store) DecideLeave(ctx context.Context, id string, v leaveVerdict, credit *quotaMove) error {
	tag, err := s.db.Exec(ctx, `UPDATE hris.leaves
		SET status = $2, approved_by = $3, approved_at = now(), rejection_reason = $4,
		    decided_as = $5, decided_by_name = $6
		WHERE id = $1 AND status = 'pending'`, id, v.Status, v.ApproverID, v.Rejection, v.DecidedAs, v.DecidedBy)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errNoRow
	}
	if credit == nil {
		return nil
	}
	_, err = s.db.Exec(ctx, `INSERT INTO hris.leave_balances (employee_id, year, annual_leave_total, annual_leave_used)
		VALUES ($1, $2, 12, $3)
		ON CONFLICT (employee_id, year)
		DO UPDATE SET annual_leave_used = leave_balances.annual_leave_used + $3, updated_at = now()`,
		credit.EmployeeID, credit.Year, credit.Days)
	return err
}
```

dan pada `ManagerContact` ganti join:

```go
		JOIN hris.employees a ON a.id = e.reporting_to AND a.is_active = true
```

dengan:

```go
		JOIN hris.employees a ON a.id = e.reporting_to AND a.is_active = true AND a.id <> e.id
```

`backend/internal/modules/hris/overtime.go`, pada `SubordinateIDs` ganti query:

```go
	rows, err := queryRows(ctx, s.db, `SELECT id FROM hris.employees WHERE reporting_to = $1`, managerID)
```

dengan:

```go
	// A reporting_to pointing at oneself (live data) is not a direct report
	// (EPIC-063 R2): the manager's queues never hold their own requests.
	rows, err := queryRows(ctx, s.db, `SELECT id FROM hris.employees WHERE reporting_to = $1 AND id <> $1`, managerID)
```

`backend/internal/modules/hris/http_leaves.go:132`, ganti:

```go
	out, err := h.svc.DecideLeave(r.Context(), a, in)
```

dengan:

```go
	out, err := h.svc.DecideLeave(r.Context(), a, in, auditMeta(r))
```

- [ ] **Step 5: Jalankan test, pastikan lulus**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris -run 'TestLeaveDecisionManagerFirstHRBackup|TestLeaveSelfManagerCannotApproveOwn|TestLeaves|TestOvertimeAndShifts|TestBusinessTrip' -count=1 -v`
Expected: PASS.

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris/... -count=1`
Expected: PASS (atau hanya nama di baseline).

- [ ] **Step 6: Commit**

```bash
gofmt -l backend/internal/modules/hris
git add backend/database/migrations/deltas/20261024090000_approval_atasan.sql backend/database/seeders/iam-menus.sql backend/internal/modules/hris/approvals.go backend/internal/modules/hris/leaves.go backend/internal/modules/hris/postgres_leaves.go backend/internal/modules/hris/overtime.go backend/internal/modules/hris/http_leaves.go backend/internal/modules/hris/leave_approval_integration_test.go
git commit -m "feat(hris): keputusan cuti atasan dulu, HR cadangan tercatat & diaudit, tanpa self-approval (EPIC-063 Task 2)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---
### Task 3: Lembur — wajib sebelum dimulai, jenis Weekday/Weekend, kapasitas pemutus, WA atasan

**Files:**
- Modify: `backend/internal/contracts/hris/hris.go` (topik + payload lembur)
- Create: `backend/internal/modules/hris/overtime_daytype.go`
- Modify: `backend/internal/modules/hris/overtime.go` (`CreateOvertime`, `DecideOvertime`, konstanta `overtimeTooLate`)
- Modify: `backend/internal/modules/hris/approvals.go` (`notifyManager`, `notifyOvertimeRequest`)
- Modify: `backend/internal/modules/hris/leaves.go` (`notifyLeaveRequest` memakai `notifyManager`, `TopicOvertimeRequested`)
- Modify: `backend/internal/modules/hris/http_schedules.go:167-185` (`decideOvertime` meneruskan `auditMeta(r)`)
- Modify: `backend/internal/modules/hris/routes.go` (route day-type), `backend/internal/modules/hris/module.go:73-76` (subscribe)
- Modify: `backend/internal/modules/hris/harness_test.go:38` (subscribe di harness)
- Modify: `frontend/src/lib/go-routes.generated.json` (regenerate), `frontend/src/lib/backend-routes.test.ts` (`goNativePrefixes`)
- Test: `backend/internal/modules/hris/overtime_approval_integration_test.go`

**Interfaces:**
- Consumes: `domain.OvertimeStartsInPast`, `domain.OvertimeDayTypeFor`, `domain.OvertimeDayTypeLabel`, `domain.OvertimeRequestWaMessage`, `domain.DecideAs` (Task 1); `decisionRace`, `auditDecision` (Task 2); `s.repo.ActiveHolidays` (`LeaveRepo`), `s.repo.ShiftPattern` (`files.go`), `ManagerContact`.
- Produces:
  - `contracts.TopicOvertimeRequested = "hris.overtime.requested"`, `contracts.OvertimeRequested{OvertimeID, EmployeeID, EmployeeName, Date, StartTime, EndTime string; Hours float64; DayType string; Reason *string}`; `hris.TopicOvertimeRequested`
  - `const overtimeTooLate = "Lembur wajib diajukan sebelum dimulai — jam mulai sudah lewat"`
  - `func (s *Service) suggestOvertimeDayType(ctx context.Context, employeeID, date string) (dayType string, known bool, err error)`
  - `func (s *Service) OvertimeDayTypeHint(ctx context.Context, employeeID, date string) (*Row, error)` → `{date, day_type: "regular"|"rest_day"|null, label: string|null}`
  - Route `GET /api/hris/overtime/day-type?date=YYYY-MM-DD[&employee_id=<uuid> (HR)]` → `{"data": {...}}`
  - `func (s *Service) DecideOvertime(ctx context.Context, a *Actor, id, action string, rejection, dayType *string, meta audit.Entry) (*Row, error)`
  - `func (s *Service) notifyManager(ctx context.Context, tx pgx.Tx, kind, dedup, employeeID, employeeName string, message func(approver string) string) error`
  - Respons baris lembur memuat `decided_as`, `decided_by_name` (via `SELECT *`).
  - Helper test `func otBody(date, start, end string) map[string]any` (dipakai Task 4 dan 5)

- [ ] **Step 1: Tulis test yang gagal**

`backend/internal/modules/hris/overtime_approval_integration_test.go`:

```go
package hris

import (
	"context"
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

func otBody(date, start, end string) map[string]any {
	return map[string]any{"date": date, "start_time": start, "end_time": end, "reason": "Closing stok bulanan"}
}

// MOM AP-02 (EPIC-063 R3). fixedNow = Senin 05-10-2026 10:00 WIB.
func TestOvertimeMustBeRequestedBeforeStart(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuKepegawaian}, true)
	staff, own := h.staff("pos", nil, true)
	const late = "Lembur wajib diajukan sebelum dimulai — jam mulai sudah lewat"

	expect(t, h.do(&staff, "POST", "/api/hris/overtime", otBody("2026-10-04", "18:00", "20:00")), 400, late)
	expect(t, h.do(&staff, "POST", "/api/hris/overtime", otBody("2026-10-05", "09:00", "11:00")), 400, late)
	expect(t, h.do(&staff, "POST", "/api/hris/overtime", otBody("2026-10-05", "10:00", "12:00")), 200, "")
	// HR juga tidak boleh mengajukan lembur lampau untuk dirinya sendiri…
	expect(t, h.do(&hr, "POST", "/api/hris/overtime", otBody("2026-10-04", "18:00", "20:00")), 400, late)
	// …tetapi boleh mencatat penugasan lampau untuk karyawan lain (R3).
	assign := otBody("2026-10-02", "18:00", "20:00")
	assign["employee_id"] = own
	assigned := h.do(&hr, "POST", "/api/hris/overtime", assign)
	expect(t, assigned, 200, "")
	if assigned.data()["source"] != "company" {
		t.Fatalf("assignment = %s", assigned.raw)
	}
}

// K-13 (EPIC-063 R5): jenis dari ESS dicek ke jadwal dan hari libur.
func TestOvertimeDayTypeFollowsSchedule(t *testing.T) {
	h := newHarness(t)
	staff, own := h.staff("pos", nil, true)
	free, freeID := h.staff("pos", nil, true) // tanpa jadwal
	shift := h.scalar(`INSERT INTO hris.shifts (name, start_time, end_time) VALUES ('Pagi Lembur', '08:00', '16:00') RETURNING id::text`)
	h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from) VALUES ($1, 1, $2, '2026-01-01')`, own, shift)
	h.exec(`INSERT INTO hris.public_holidays (holiday_date, name, type) VALUES ('2026-10-19', 'Libur Uji Lembur', 'nasional')`)

	hint := func(s *testutil.Staff, date string) map[string]any {
		t.Helper()
		c := h.do(s, "GET", "/api/hris/overtime/day-type?date="+date, nil)
		expect(t, c, 200, "")
		return c.data()
	}
	if d := hint(&staff, "2026-10-12"); d["day_type"] != "regular" || d["label"] != "Weekday" {
		t.Fatalf("Senin ber-shift = %v", d)
	}
	if d := hint(&staff, "2026-10-11"); d["day_type"] != "rest_day" || d["label"] != "Weekend / Libur" {
		t.Fatalf("Minggu = %v", d)
	}
	if d := hint(&staff, "2026-10-19"); d["day_type"] != "rest_day" {
		t.Fatalf("libur nasional = %v", d)
	}
	if d := hint(&free, "2026-10-12"); d["day_type"] != nil || d["label"] != nil {
		t.Fatalf("tanpa jadwal = %v", d)
	}
	expect(t, h.do(&staff, "GET", "/api/hris/overtime/day-type?date=12-10-2026", nil), 400, "Tanggal tidak valid")
	expect(t, h.do(&staff, "GET", "/api/hris/overtime/day-type?date=2026-10-12&employee_id="+freeID, nil), 403,
		"Insufficient permissions")

	wrong := otBody("2026-10-12", "17:00", "19:00")
	wrong["day_type"] = "rest_day"
	expect(t, h.do(&staff, "POST", "/api/hris/overtime", wrong), 400, "Jenis lembur tanggal ini Weekday sesuai jadwal Anda")
	sunday := h.do(&staff, "POST", "/api/hris/overtime", otBody("2026-10-11", "09:00", "12:00"))
	expect(t, sunday, 200, "")
	if sunday.data()["day_type"] != "rest_day" {
		t.Fatalf("tanpa jenis memakai saran jadwal: %s", sunday.raw)
	}
	chosen := otBody("2026-10-12", "17:00", "19:00")
	chosen["day_type"] = "rest_day"
	freeRow := h.do(&free, "POST", "/api/hris/overtime", chosen)
	expect(t, freeRow, 200, "")
	if freeRow.data()["day_type"] != "rest_day" {
		t.Fatalf("tanpa jadwal memakai pilihan karyawan: %s", freeRow.raw)
	}
}

// Q-APV-3 (R1) + WA atasan saat pengajuan lembur (R8).
func TestOvertimeDecisionRecordsCapacityAndNotifiesManager(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuKepegawaian}, true)
	boss, bossID := h.staff("pos", nil, true, emp{name: "Atasan Lembur", phone: "0812 3456 7000"})
	staff, own := h.staff("pos", nil, true, emp{name: "Crew Lembur", boss: &bossID})
	dispatch := func() {
		t.Helper()
		if _, err := h.deps.Events.Dispatch(context.Background(), h.tx); err != nil {
			t.Fatal(err)
		}
	}

	first := h.do(&staff, "POST", "/api/hris/overtime", otBody("2026-10-06", "17:00", "19:30"))
	expect(t, first, 200, "")
	dispatch()
	if len(h.wa.sent) != 1 || !strings.HasPrefix(h.wa.sent[0], "6281234567000|Paddy OS — Pengajuan Lembur") ||
		!strings.Contains(h.wa.sent[0], "Kepada: Atasan Lembur") ||
		!strings.Contains(h.wa.sent[0], "Sel, 06 Okt 2026, 17:00–19:30 (2,5 jam, Weekday)") ||
		!strings.Contains(h.wa.sent[0], "Area Karyawan → Persetujuan Tim") {
		t.Fatalf("whatsapp = %v", h.wa.sent)
	}
	// Penugasan HR tidak dikirim ke atasan.
	assign := otBody("2026-10-08", "17:00", "19:00")
	assign["employee_id"] = own
	expect(t, h.do(&hr, "POST", "/api/hris/overtime", assign), 200, "")
	dispatch()
	if len(h.wa.sent) != 1 {
		t.Fatalf("penugasan HR tidak boleh mengirim WA atasan: %v", h.wa.sent)
	}

	firstID := first.data()["id"].(string)
	byBoss := h.do(&boss, "POST", "/api/hris/overtime/decide", map[string]any{"overtime_id": firstID, "action": "approve"})
	expect(t, byBoss, 200, "")
	if d := byBoss.data(); d["decided_as"] != "manager" || d["decided_by_name"] == nil {
		t.Fatalf("manager approve = %s", byBoss.raw)
	}
	second := h.do(&staff, "POST", "/api/hris/overtime", otBody("2026-10-07", "17:00", "19:00"))
	expect(t, second, 200, "")
	secondID := second.data()["id"].(string)
	byHR := h.do(&hr, "POST", "/api/hris/overtime/decide", map[string]any{"overtime_id": secondID, "action": "approve"})
	expect(t, byHR, 200, "")
	if byHR.data()["decided_as"] != "hr" {
		t.Fatalf("HR approve = %s", byHR.raw)
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE entity = 'hris.overtime_requests'
		AND action = 'overtime.approve' AND entity_id IN ($1, $2)`, firstID, secondID); n != "2" {
		t.Fatalf("audit lembur = %s", n)
	}
}
```

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris -run 'TestOvertimeMustBeRequestedBeforeStart|TestOvertimeDayTypeFollowsSchedule|TestOvertimeDecisionRecordsCapacityAndNotifiesManager' -count=1 -v`
Expected: FAIL — tanggal lampau diterima 200, `GET /api/hris/overtime/day-type` 404/405, dan `whatsapp = []`.

- [ ] **Step 3: Tulis implementasi**

`backend/internal/contracts/hris/hris.go`, tambahkan di akhir file:

```go
// TopicOvertimeRequested fires when an employee submits their own overtime
// request (EPIC-063). The HRIS module sends the WhatsApp note to the
// employee's direct manager; HR assignments do not fire it.
const TopicOvertimeRequested = "hris.overtime.requested"

// OvertimeRequested is the TopicOvertimeRequested payload.
type OvertimeRequested struct {
	OvertimeID   string  `json:"overtime_id"`
	EmployeeID   string  `json:"employee_id"`
	EmployeeName string  `json:"employee_name"`
	Date         string  `json:"date"`
	StartTime    string  `json:"start_time"`
	EndTime      string  `json:"end_time"`
	Hours        float64 `json:"hours"`
	DayType      string  `json:"day_type"`
	Reason       *string `json:"reason"`
}
```

`backend/internal/modules/hris/leaves.go`, di bawah `const TopicLeaveRequested = …` tambahkan:

```go
// TopicOvertimeRequested is re-exported for the module constructor (EPIC-063).
const TopicOvertimeRequested = contracts.TopicOvertimeRequested
```

lalu ganti seluruh fungsi `notifyLeaveRequest` dengan:

```go
// notifyLeaveRequest is the outbox handler of TopicLeaveRequested
// (notifyLeaveRequestWa): best effort, deduplicated per leave through
// configuration.wa_notif_log, never failing the delivery.
func (s *Service) notifyLeaveRequest(ctx context.Context, tx pgx.Tx, e outbox.Event) error {
	var p contracts.LeaveRequested
	if err := e.Decode(&p); err != nil {
		s.log.ErrorContext(ctx, "[wa-cuti] bad payload", "event", e.ID, "error", err)
		return nil
	}
	return s.notifyManager(ctx, tx, "cuti", "leave:"+p.LeaveID, p.EmployeeID, p.EmployeeName, func(approver string) string {
		return domain.LeaveRequestWaMessage(domain.LeaveRequestWa{
			EmployeeName: p.EmployeeName, LeaveType: p.LeaveType, StartDate: p.StartDate, EndDate: p.EndDate,
			TotalDays: p.TotalDays, Reason: p.Reason, ApproverName: &approver,
		})
	})
}
```

`backend/internal/modules/hris/approvals.go` — ganti blok import dengan:

```go
import (
	"context"

	"github.com/jackc/pgx/v5"

	contracts "nuhabit/backend/internal/contracts/hris"
	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/outbox"
)
```

dan tambahkan di akhir file:

```go
// notifyManager sends the direct manager's WhatsApp note for a request (best
// effort, deduplicated per request through configuration.wa_notif_log, never
// failing the delivery). kind is the wa_notif_log type ("cuti", "lembur").
// ManagerContact skips a reporting_to pointing at the requester (EPIC-063 R2).
func (s *Service) notifyManager(ctx context.Context, tx pgx.Tx, kind, dedup, employeeID, employeeName string,
	message func(approver string) string) error {
	if s.whatsapp == nil {
		return nil
	}
	tag := "[wa-" + kind + "]"
	repo := &store{db: tx}
	name, phone, found, err := repo.ManagerContact(ctx, employeeID)
	if err != nil {
		return err
	}
	if !found {
		s.log.WarnContext(ctx, tag+" tidak punya atasan tercatat — notifikasi dilewati", "employee", employeeName)
		return nil
	}
	target := domain.NormalizeWaPhone(phone)
	if target == "" {
		s.log.WarnContext(ctx, tag+" nomor WA atasan kosong/tidak valid — notifikasi dilewati", "manager", name)
		return nil
	}
	text := message(name)
	if len(dedup) > 160 {
		dedup = dedup[:160]
	}
	claimID, claimed, err := s.whatsapp.Claim(ctx, tx, kind, dedup, text, []string{target})
	if err != nil || !claimed {
		return err
	}
	if !s.whatsapp.Configured(ctx) {
		s.log.WarnContext(ctx, tag+" gateway belum dikonfigurasi — notifikasi dilewati")
		return s.whatsapp.Release(ctx, tx, claimID)
	}
	if sent, timedOut, reason := s.whatsapp.SendText(ctx, target, text); !sent && !timedOut {
		s.log.ErrorContext(ctx, tag+" gagal kirim", "target", target, "reason", reason)
		return s.whatsapp.Release(ctx, tx, claimID)
	}
	return nil
}

// notifyOvertimeRequest is the outbox handler of TopicOvertimeRequested
// (EPIC-063 R8): the direct manager learns an overtime request waits for them.
func (s *Service) notifyOvertimeRequest(ctx context.Context, tx pgx.Tx, e outbox.Event) error {
	var p contracts.OvertimeRequested
	if err := e.Decode(&p); err != nil {
		s.log.ErrorContext(ctx, "[wa-lembur] bad payload", "event", e.ID, "error", err)
		return nil
	}
	return s.notifyManager(ctx, tx, "lembur", "overtime:"+p.OvertimeID, p.EmployeeID, p.EmployeeName, func(approver string) string {
		return domain.OvertimeRequestWaMessage(domain.OvertimeRequestWa{
			EmployeeName: p.EmployeeName, Date: p.Date, StartTime: p.StartTime, EndTime: p.EndTime,
			Hours: p.Hours, DayType: p.DayType, Reason: p.Reason, ApproverName: &approver,
		})
	})
}
```

`backend/internal/modules/hris/overtime_daytype.go`:

```go
package hris

import (
	"context"
	"net/http"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/validate"
)

// Jenis lembur Weekday / Weekend-Libur (MOM AP-03, K-13, EPIC-063 R5).

// suggestOvertimeDayType is the type the employee's schedule and the active
// public holidays imply on date; known is false without a schedule in force.
func (s *Service) suggestOvertimeDayType(ctx context.Context, employeeID, date string) (string, bool, error) {
	holidays, err := s.repo.ActiveHolidays(ctx, date, date)
	if err != nil {
		return "", false, err
	}
	rows, err := s.repo.ShiftPattern(ctx, employeeID)
	if err != nil {
		return "", false, err
	}
	pattern := make([]domain.ScheduleRow, len(rows))
	for i, r := range rows {
		pattern[i] = domain.ScheduleRow{DayOfWeek: int(r.Num("day_of_week")), ShiftID: r.StrPtr("shift_id"),
			EffectiveFrom: r.Str("effective_from"), EffectiveTo: r.StrPtr("effective_to")}
	}
	dayType, known := domain.OvertimeDayTypeFor(date, len(holidays) > 0, pattern)
	return dayType, known, nil
}

// overtimeDayType is the day type stored on a new request. free (an HR
// assignment for someone else) keeps any sent type. One's own request may
// not contradict a known suggestion; without a type it takes the suggestion,
// or Weekday when nothing is known.
func (s *Service) overtimeDayType(ctx context.Context, employeeID, date string, sent *string, free bool) (string, error) {
	if free && sent != nil {
		return *sent, nil
	}
	suggested, known, err := s.suggestOvertimeDayType(ctx, employeeID, date)
	if err != nil {
		return "", err
	}
	switch {
	case sent == nil && known:
		return suggested, nil
	case sent == nil:
		return domain.OvertimeWeekday, nil
	case !known || *sent == suggested:
		return *sent, nil
	}
	return "", httpx.BadRequest("Jenis lembur tanggal ini " + domain.OvertimeDayTypeLabel(suggested) + " sesuai jadwal Anda")
}

// OvertimeDayTypeHint is GET /api/hris/overtime/day-type: the suggestion for
// the ESS form; day_type and label are null when no schedule is in force.
func (s *Service) OvertimeDayTypeHint(ctx context.Context, employeeID, date string) (*Row, error) {
	dayType, known, err := s.suggestOvertimeDayType(ctx, employeeID, date)
	if err != nil {
		return nil, err
	}
	if !known {
		return obj("date", date, "day_type", nil, "label", nil), nil
	}
	return obj("date", date, "day_type", dayType, "label", domain.OvertimeDayTypeLabel(dayType)), nil
}

// GET /api/hris/overtime/day-type?date=YYYY-MM-DD&employee_id= (HR only for
// another employee).
func (h handlers) overtimeDayTypeHint(w http.ResponseWriter, r *http.Request, a *Actor) error {
	q := r.URL.Query()
	date := q.Get("date")
	if _, ok := domain.ParseDate(date); !ok || !domain.DateRe.MatchString(date) {
		return httpx.BadRequest("Tanggal tidak valid")
	}
	target := a.EmployeeID
	if id := q.Get("employee_id"); id != "" && id != "me" {
		canonical, ok := validate.CanonicalUUID(id)
		if !ok {
			return httpx.BadRequest("ID karyawan tidak valid")
		}
		if !a.IsHR && (a.EmployeeID == nil || *a.EmployeeID != canonical) {
			return httpx.Forbidden("Insufficient permissions")
		}
		target = &canonical
	}
	if target == nil {
		return httpx.BadRequest("Akun ini tidak tertaut ke data karyawan")
	}
	out, err := h.svc.OvertimeDayTypeHint(r.Context(), *target, date)
	if err != nil {
		return err
	}
	return ok(w, dataOf(out))
}
```

`backend/internal/modules/hris/overtime.go`:

1. Import menjadi:

```go
import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	contracts "nuhabit/backend/internal/contracts/hris"
	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
)
```

2. Di bawah `const overtimeDuplicate = …` tambahkan:

```go
// overtimeTooLate is MOM AP-02's refusal (EPIC-063 R3).
const overtimeTooLate = "Lembur wajib diajukan sebelum dimulai — jam mulai sudah lewat"
```

3. Ganti komentar `overtimeInput.DayType` menjadi `// regular (Weekday) | rest_day (Weekend / Libur), K-13; nil = saran jadwal`.

4. Ganti seluruh `CreateOvertime` dengan:

```go
// CreateOvertime: a request for oneself ("employee", decided by the direct
// manager or HR) or an HR assignment for someone else ("company", confirmed
// by the employee). One's own request must come before the overtime starts
// (AP-02) and its type must match the schedule (K-13); it notifies the
// direct manager through the outbox (EPIC-063).
func (s *Service) CreateOvertime(ctx context.Context, a *Actor, in overtimeInput) (*Row, error) {
	targetID := a.EmployeeID
	if in.EmployeeID != nil && *in.EmployeeID != "" && *in.EmployeeID != "me" {
		targetID = in.EmployeeID
	}
	if targetID == nil {
		return nil, httpx.BadRequest("Akun ini tidak tertaut ke data karyawan")
	}
	isSelf := a.EmployeeID != nil && *targetID == *a.EmployeeID
	if !isSelf && !a.IsHR {
		return nil, httpx.Forbidden("Hanya HRD yang bisa membuat penugasan lembur untuk karyawan lain")
	}
	if isSelf && domain.OvertimeStartsInPast(in.Date, in.StartTime, s.now()) {
		return nil, httpx.BadRequest(overtimeTooLate)
	}
	target, err := s.repo.OvertimeTarget(ctx, *targetID)
	if err != nil {
		return nil, err
	}
	if target == nil || (target.Has("is_active") && target.Get("is_active") == false) {
		return nil, httpx.NotFound("Karyawan tidak ditemukan/nonaktif")
	}
	exists, err := s.repo.OvertimeActiveOn(ctx, *targetID, in.Date)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, httpx.BadRequest(overtimeDuplicate)
	}
	if in.StartTime == in.EndTime {
		return nil, httpx.BadRequest("Jam mulai dan selesai tidak boleh sama")
	}
	hours := domain.OvertimeHoursFromTimes(in.StartTime, in.EndTime)
	if hours > 12 {
		return nil, httpx.BadRequest("Durasi lembur maksimal 12 jam")
	}
	if hours < 1 {
		return nil, httpx.BadRequest("Durasi lembur minimal 1 jam")
	}
	dayType, err := s.overtimeDayType(ctx, *targetID, in.Date, in.DayType, !isSelf)
	if err != nil {
		return nil, err
	}
	source := "company"
	if isSelf {
		source = "employee"
	}
	var f fields
	f.add("employee_id", *targetID)
	f.add("date", in.Date)
	f.add("start_time", in.StartTime)
	f.add("end_time", in.EndTime)
	f.add("hours", hours)
	f.add("source", source)
	f.add("status", "pending")
	f.add("reason", in.Reason)
	f.add("day_type", dayType)
	f.add("requested_by", a.EmployeeID)
	var row *Row
	err = s.repo.InTx(ctx, func(r Repository) error {
		var err error
		if row, err = r.InsertOvertime(ctx, f); err != nil || source != "employee" {
			return err
		}
		reason := in.Reason
		return r.Publish(ctx, TopicOvertimeRequested, *targetID, contracts.OvertimeRequested{
			OvertimeID: row.Str("id"), EmployeeID: *targetID, EmployeeName: target.Str("full_name"),
			Date: in.Date, StartTime: in.StartTime, EndTime: in.EndTime, Hours: hours, DayType: dayType, Reason: &reason,
		})
	})
	if database.IsUniqueViolation(err) { // the active (employee_id, date) index lost a race
		return nil, httpx.BadRequest(overtimeDuplicate)
	}
	if err != nil {
		return nil, err
	}
	message := "Pengajuan lembur terkirim — menunggu persetujuan"
	if source == "company" {
		message = "Penugasan lembur dibuat — menunggu konfirmasi karyawan"
	}
	return obj("data", row, "message", message), nil
}
```

5. Ganti seluruh `DecideOvertime` dengan:

```go
// DecideOvertime approves, rejects or cancels a pending request (rules in
// domain.CanDecideOvertime); the update is guarded on status = pending. An
// employee request's approval or rejection stores the decider's capacity
// (direct manager or HR, Q-APV-3) and is audited (EPIC-063 R1).
func (s *Service) DecideOvertime(ctx context.Context, a *Actor, id, action string, rejection, dayType *string, meta audit.Entry) (*Row, error) {
	ot, err := s.repo.OvertimeForDecision(ctx, id)
	if err != nil {
		return nil, err
	}
	if ot == nil {
		return nil, httpx.NotFound("Pengajuan lembur tidak ditemukan")
	}
	if status := ot.Str("status"); status != "pending" {
		return nil, httpx.BadRequest("Pengajuan sudah "+status, obj("current_status", status))
	}
	var reportingTo *string
	emp := ot.Child("employee")
	if emp != nil {
		reportingTo = emp.StrPtr("reporting_to")
	}
	allowed, reason := domain.CanDecideOvertime(domain.OvertimeActor{EmployeeID: a.EmployeeID, IsHR: a.IsHR},
		domain.OvertimeRequest{EmployeeID: ot.Str("employee_id"), RequestedBy: ot.StrPtr("requested_by"),
			Source: ot.Str("source"), ReportingTo: reportingTo}, action)
	if !allowed {
		return nil, httpx.Forbidden(reason)
	}
	if action == "reject" && (rejection == nil || domain.JSTrim(*rejection) == "") {
		return nil, httpx.BadRequest("Alasan penolakan wajib diisi")
	}
	status := decisionStatus[action]
	now := s.clock()
	var f fields
	f.add("status", status)
	f.add("decided_by", a.EmployeeID)
	f.add("decided_at", now)
	if action == "reject" {
		f.add("rejection_reason", rejection)
	} else {
		f.add("rejection_reason", nil)
	}
	// HR may set the day type (rest day = tarif libur) while approving (R11).
	if action == "approve" && a.IsHR && dayType != nil {
		f.add("day_type", *dayType)
	}
	decidedAs := ""
	if ot.Str("source") == "employee" && action != "cancel" {
		decidedAs, _ = domain.DecideAs(domain.Approver{EmployeeID: a.EmployeeID, IsHR: a.IsHR},
			ot.Str("employee_id"), reportingTo, "lembur")
		f.add("decided_as", decidedAs)
		f.add("decided_by_name", actorName(a))
	}
	f.add("updated_at", now)
	var row *Row
	err = s.repo.InTx(ctx, func(r Repository) error {
		var err error
		if row, err = r.DecideOvertime(ctx, id, f); err != nil || decidedAs == "" {
			return err
		}
		label := dateText(ot, "date")
		if emp != nil {
			label = emp.Str("full_name") + " · " + label
		}
		return auditDecision(ctx, r.querier(), a, "overtime."+action, "hris.overtime_requests", id, label,
			map[string]any{"status": status, "decided_as": decidedAs, "rejection_reason": rejection}, meta)
	})
	if errors.Is(err, errNoRow) {
		return nil, httpx.Conflict(decisionRace)
	}
	if err != nil {
		return nil, err
	}
	return obj("data", row, "message", decisionMessage[status]), nil
}
```

`backend/internal/modules/hris/http_schedules.go`, di `decideOvertime` ganti:

```go
	out, err := h.svc.DecideOvertime(r.Context(), a, *id, *action, rejection, dayType)
```

dengan:

```go
	out, err := h.svc.DecideOvertime(r.Context(), a, *id, *action, rejection, dayType, auditMeta(r))
```

`backend/internal/modules/hris/routes.go`, di bawah `r("POST /api/hris/overtime/decide", …),` tambahkan:

```go
		r("GET /api/hris/overtime/day-type", h.withActor(h.overtimeDayTypeHint)),
```

`backend/internal/modules/hris/module.go`, di `New` setelah subscribe cuti:

```go
		deps.Events.Subscribe(TopicOvertimeRequested, "hris.overtime-request-whatsapp", svc.notifyOvertimeRequest)
```

`backend/internal/modules/hris/harness_test.go:38`, setelah subscribe cuti tambahkan:

```go
	deps.Events.Subscribe(TopicOvertimeRequested, "hris.overtime-request-whatsapp", svc.notifyOvertimeRequest)
```

Regenerate route manifest dan daftar Go-native:

Run: `cd backend && PATH=$HOME/.local/go/bin:$PATH make generate && git diff --stat ../frontend/src/lib/go-routes.generated.json`
Expected: `go-routes.generated.json` bertambah `GET /api/hris/overtime/day-type`.

`frontend/src/lib/backend-routes.test.ts`, di akhir array `goNativePrefixes` (setelah `"/api/hris/business-trips"`) ganti `"/api/hris/business-trips"];` dengan:

```ts
      "/api/hris/business-trips",
      // Paket Approval atasan (EPIC-063): hanya di Go, tanpa fallback Next.
      "/api/hris/overtime/day-type"];
```

- [ ] **Step 4: Jalankan test, pastikan lulus**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris -run 'TestOvertime|TestLeaves|TestLeaveSelfManager' -count=1 -v`
Expected: PASS (termasuk `TestOvertimeAndShifts`, `TestOvertimeMinimumOneHour` lama).

Run: `cd frontend && npx vitest run src/lib/backend-routes.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -l backend/internal
git add backend/internal/contracts/hris/hris.go backend/internal/modules/hris/overtime_daytype.go backend/internal/modules/hris/overtime.go backend/internal/modules/hris/approvals.go backend/internal/modules/hris/leaves.go backend/internal/modules/hris/http_schedules.go backend/internal/modules/hris/routes.go backend/internal/modules/hris/module.go backend/internal/modules/hris/harness_test.go backend/internal/modules/hris/overtime_approval_integration_test.go frontend/src/lib/go-routes.generated.json frontend/src/lib/backend-routes.test.ts
git commit -m "feat(hris): lembur wajib sebelum dimulai, jenis Weekday/Weekend, kapasitas pemutus & WA atasan (EPIC-063 Task 3)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: Koreksi lembur oleh AdminHRIS

**Files:**
- Create: `backend/internal/modules/hris/overtime_correction.go`
- Modify: `backend/internal/modules/hris/overtime.go` (interface `OvertimeRepo`)
- Modify: `backend/internal/modules/hris/routes.go` (route PATCH)
- Modify: `frontend/src/lib/go-routes.generated.json`, `frontend/src/lib/backend-routes.test.ts`
- Test: `backend/internal/modules/hris/overtime_correction_integration_test.go`

**Interfaces:**
- Consumes: `otBody` (Task 3), `decisionRace` (Task 2), `overtimeDuplicate`, `overtimeTimeRe`, `overtimeDayTypes`, `OvertimeForDecision`, `domain.OvertimeHoursFromTimes`, `dateText`, `actorName`, `ptr`, `auditMeta`, `h.svc.Actor`.
- Produces:
  - `OvertimeRepo.CorrectOvertime(ctx context.Context, id, fromStatus string, f fields) (*Row, error)` (errNoRow bila status berubah)
  - `OvertimeRepo.ClosedPeriodOn(ctx context.Context, date string) (*Row, error)` → `{label: "DD-MM-YYYY s.d. DD-MM-YYYY"}` atau nil
  - `type overtimeCorrection struct { Date, StartTime, EndTime, DayType *string; Reason string }`
  - `func (s *Service) CorrectOvertime(ctx context.Context, a *Actor, id string, in overtimeCorrection, meta audit.Entry) (*Row, error)` → `{data: row, message: "Lembur dikoreksi"}`
  - Route `PATCH /api/hris/overtime/{id}` body `{date?, start_time?, end_time?, day_type?, reason}`, guard `update` pada `hris.kepegawaian.overtime`
  - Baris lembur memuat `corrected_at`, `corrected_by_name`

- [ ] **Step 1: Tulis test yang gagal**

`backend/internal/modules/hris/overtime_correction_integration_test.go`:

```go
package hris

import (
	"context"
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

// MOM AP-04 (EPIC-063 R6): AdminHRIS = aksi update di menu Lembur.
func TestOvertimeCorrectionByAdminHRIS(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.staff("hrd", []string{"hris.kepegawaian.overtime"}, true, emp{name: "Admin HRIS"})
	viewer := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd",
		Menus: map[string][]string{"hris.kepegawaian.overtime": {"read", "approve"}}})
	t.Cleanup(func() { _ = h.tx.Rollback(context.Background()) })
	staff, _ := h.staff("pos", nil, true, emp{name: "Crew Koreksi"})

	created := h.do(&staff, "POST", "/api/hris/overtime", otBody("2026-10-06", "17:00", "19:00"))
	expect(t, created, 200, "")
	id := created.data()["id"].(string)
	path := "/api/hris/overtime/" + id

	expect(t, h.do(&viewer, "PATCH", path, map[string]any{"end_time": "20:00", "reason": "Jam pulang sebenarnya"}), 403,
		"Insufficient permissions")
	expect(t, h.do(&admin, "PATCH", path, map[string]any{"end_time": "20:00", "reason": " abc "}), 400,
		"Alasan koreksi minimal 5 karakter")
	expect(t, h.do(&admin, "PATCH", path, map[string]any{"reason": "Tidak ada yang diubah"}), 400, "Tidak ada perubahan")
	expect(t, h.do(&admin, "PATCH", path, map[string]any{"end_time": "17:30", "reason": "Lembur cuma setengah jam"}), 400,
		"Durasi lembur minimal 1 jam")

	fixed := h.do(&admin, "PATCH", path, map[string]any{"date": "2026-10-07", "end_time": "20:30",
		"day_type": "rest_day", "reason": "Sesuai absensi"})
	expect(t, fixed, 200, "")
	if d := fixed.data(); d["hours"] != "3.50" || d["day_type"] != "rest_day" || d["status"] != "pending" ||
		!strings.HasPrefix(d["date"].(string), "2026-10-07") || d["corrected_by_name"] == nil || d["corrected_at"] == nil ||
		fixed.body["message"] != "Lembur dikoreksi" {
		t.Fatalf("corrected = %s", fixed.raw)
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'overtime.correct' AND entity_id = $1
		AND reason = 'Sesuai absensi' AND before->>'end_time' = '19:00' AND before->>'date' = '2026-10-06'
		AND after->>'end_time' = '20:30' AND after->>'date' = '2026-10-07' AND after->>'day_type' = 'rest_day'`, id); n != "1" {
		t.Fatalf("audit koreksi = %s", n)
	}

	// Tanggal bentrok dengan lembur aktif lain → 400 duplikat, bukan 500.
	expect(t, h.do(&staff, "POST", "/api/hris/overtime", otBody("2026-10-09", "17:00", "19:00")), 200, "")
	expect(t, h.do(&admin, "PATCH", path, map[string]any{"date": "2026-10-09", "reason": "Pindah ke Jumat"}), 400,
		"Sudah ada pengajuan lembur pending/approved di tanggal tersebut")

	// Lembur sendiri tidak bisa dikoreksi sendiri.
	mine := h.do(&admin, "POST", "/api/hris/overtime", otBody("2026-10-08", "17:00", "19:00"))
	expect(t, mine, 200, "")
	expect(t, h.do(&admin, "PATCH", "/api/hris/overtime/"+mine.data()["id"].(string),
		map[string]any{"end_time": "21:00", "reason": "Tambah jam sendiri"}), 403, "Tidak bisa mengoreksi lembur sendiri")

	// Yang ditolak tidak bisa dikoreksi.
	other := h.do(&staff, "POST", "/api/hris/overtime", otBody("2026-10-13", "17:00", "19:00"))
	expect(t, other, 200, "")
	otherID := other.data()["id"].(string)
	expect(t, h.do(&admin, "POST", "/api/hris/overtime/decide", map[string]any{"overtime_id": otherID, "action": "reject",
		"rejection_reason": "Tidak perlu lembur"}), 200, "")
	expect(t, h.do(&admin, "PATCH", "/api/hris/overtime/"+otherID, map[string]any{"end_time": "20:00", "reason": "Koreksi jam"}),
		400, "Hanya lembur yang menunggu atau disetujui yang bisa dikoreksi")

	// Periode absensi yang sudah closing.
	h.exec(`DELETE FROM hris.attendance_periods WHERE start_date <= '2026-11-30' AND end_date >= '2026-09-01'`)
	h.exec(`INSERT INTO hris.attendance_periods (period_year, period_month, start_date, end_date, closing_date,
		status, closed_at, close_mode) VALUES (2026, 10, '2026-09-21', '2026-10-20', '2026-10-21', 'closed', now(), 'manual')`)
	expect(t, h.do(&admin, "PATCH", path, map[string]any{"end_time": "21:00", "reason": "Koreksi sesudah closing"}), 409,
		"Periode absensi 21-09-2026 s.d. 20-10-2026 sudah di-closing — lembur tidak bisa dikoreksi")
}
```

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris -run TestOvertimeCorrectionByAdminHRIS -count=1 -v`
Expected: FAIL — `status 405, want 403` (route PATCH belum ada).

- [ ] **Step 3: Tulis implementasi**

`backend/internal/modules/hris/overtime.go`, tambahkan ke `OvertimeRepo`:

```go
	// CorrectOvertime updates a request still in fromStatus; errNoRow when it
	// changed meanwhile (EPIC-063 R6).
	CorrectOvertime(ctx context.Context, id, fromStatus string, f fields) (*Row, error)
	// ClosedPeriodOn is the closed attendance period holding date ({label}),
	// nil when none.
	ClosedPeriodOn(ctx context.Context, date string) (*Row, error)
```

`backend/internal/modules/hris/overtime_correction.go`:

```go
package hris

import (
	"context"
	"errors"
	"net/http"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/validate"
)

// Koreksi lembur oleh AdminHRIS (MOM AP-04, EPIC-063 R6).

// overtimeManagers is the leaf whose "update" action is AdminHRIS's
// correction right (EPIC-061 leaf/action convention).
var overtimeManagers = []string{"hris.kepegawaian.overtime"}

const (
	overtimeCorrectStatus = "Hanya lembur yang menunggu atau disetujui yang bisa dikoreksi"
	overtimeCorrectSelf   = "Tidak bisa mengoreksi lembur sendiri"
)

// overtimeCorrection is the PATCH body; nil fields keep their value.
type overtimeCorrection struct {
	Date, StartTime, EndTime, DayType *string
	Reason                            string
}

// hhmm is a time column ("17:00:00") as HH:MM.
func hhmm(t string) string {
	if len(t) >= 5 {
		return t[:5]
	}
	return t
}

// CorrectOvertime fixes the date, times or type of someone else's pending or
// approved request. The status stays; hours are recomputed; a date inside a
// closed attendance period (old or new) is refused; before/after and the
// reason are audited.
func (s *Service) CorrectOvertime(ctx context.Context, a *Actor, id string, in overtimeCorrection, meta audit.Entry) (*Row, error) {
	ot, err := s.repo.OvertimeForDecision(ctx, id)
	if err != nil {
		return nil, err
	}
	if ot == nil {
		return nil, httpx.NotFound("Pengajuan lembur tidak ditemukan")
	}
	status := ot.Str("status")
	if status != "pending" && status != "approved" {
		return nil, httpx.BadRequest(overtimeCorrectStatus)
	}
	if a.EmployeeID != nil && *a.EmployeeID == ot.Str("employee_id") {
		return nil, httpx.Forbidden(overtimeCorrectSelf)
	}
	before := map[string]any{"date": dateText(ot, "date"), "start_time": hhmm(ot.Str("start_time")),
		"end_time": hhmm(ot.Str("end_time")), "day_type": ot.Str("day_type"), "hours": ot.Num("hours")}
	pick := func(sent *string, key string) string {
		if sent != nil {
			return *sent
		}
		return before[key].(string)
	}
	date, start, end, dayType := pick(in.Date, "date"), pick(in.StartTime, "start_time"), pick(in.EndTime, "end_time"),
		pick(in.DayType, "day_type")
	if date == before["date"] && start == before["start_time"] && end == before["end_time"] && dayType == before["day_type"] {
		return nil, httpx.BadRequest("Tidak ada perubahan")
	}
	if start == end {
		return nil, httpx.BadRequest("Jam mulai dan selesai tidak boleh sama")
	}
	hours := domain.OvertimeHoursFromTimes(start, end)
	if hours > 12 {
		return nil, httpx.BadRequest("Durasi lembur maksimal 12 jam")
	}
	if hours < 1 {
		return nil, httpx.BadRequest("Durasi lembur minimal 1 jam")
	}
	after := map[string]any{"date": date, "start_time": start, "end_time": end, "day_type": dayType, "hours": hours}
	label := before["date"].(string)
	if emp := ot.Child("employee"); emp != nil {
		label = emp.Str("full_name") + " · " + label
	}
	now := s.clock()
	var row *Row
	err = s.repo.InTx(ctx, func(r Repository) error {
		for _, d := range []string{before["date"].(string), date} {
			p, err := r.ClosedPeriodOn(ctx, d)
			if err != nil {
				return err
			}
			if p != nil {
				return httpx.Conflict("Periode absensi " + p.Str("label") + " sudah di-closing — lembur tidak bisa dikoreksi")
			}
		}
		var f fields
		f.add("date", date)
		f.add("start_time", start)
		f.add("end_time", end)
		f.add("hours", hours)
		f.add("day_type", dayType)
		f.add("corrected_at", now)
		f.add("corrected_by_name", actorName(a))
		f.add("updated_at", now)
		var err error
		if row, err = r.CorrectOvertime(ctx, id, status, f); err != nil {
			return err
		}
		reason := in.Reason
		meta.ActorID, meta.ActorName = a.UserID, ptr(actorName(a))
		meta.Action, meta.Entity, meta.EntityID, meta.EntityLabel = "overtime.correct", "hris.overtime_requests", &id, &label
		meta.Before, meta.After, meta.Reason = before, after, &reason
		return audit.Write(ctx, r.querier(), meta)
	})
	if database.IsUniqueViolation(err) {
		return nil, httpx.BadRequest(overtimeDuplicate)
	}
	if errors.Is(err, errNoRow) {
		return nil, httpx.Conflict(decisionRace)
	}
	if err != nil {
		return nil, err
	}
	return obj("data", row, "message", "Lembur dikoreksi"), nil
}

func (s *store) CorrectOvertime(ctx context.Context, id, fromStatus string, f fields) (*Row, error) {
	return updateOne(ctx, s.db, "hris.overtime_requests", f, fields{{"id", id}, {"status", fromStatus}}, "*")
}

func (s *store) ClosedPeriodOn(ctx context.Context, date string) (*Row, error) {
	return queryRow(ctx, s.db, `SELECT to_char(start_date, 'DD-MM-YYYY') || ' s.d. ' || to_char(end_date, 'DD-MM-YYYY') AS label
		FROM hris.attendance_periods
		WHERE status = 'closed' AND $1::date BETWEEN start_date AND end_date
		ORDER BY start_date LIMIT 1`, date)
}

// PATCH /api/hris/overtime/{id} { date?, start_time?, end_time?, day_type?, reason }
func (h handlers) correctOvertime(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	id, err := requireUUID(r.PathValue("id"), "ID lembur tidak valid")
	if err != nil {
		return err
	}
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	in := overtimeCorrection{
		Date:      f.Str("date", optional, validate.StrOpts{Check: regexOf(domain.DateRe)}),
		StartTime: f.Str("start_time", optional, validate.StrOpts{Check: regexOf(overtimeTimeRe)}),
		EndTime:   f.Str("end_time", optional, validate.StrOpts{Check: regexOf(overtimeTimeRe)}),
		DayType:   f.Enum("day_type", optional, overtimeDayTypes),
		Reason: strOr(f.Str("reason", validate.Rule{}, validate.StrOpts{Trim: true, Max: 500,
			Check: minLen(5, "Alasan koreksi minimal 5 karakter")})),
	}
	if err := formErr(f, ""); err != nil {
		return err
	}
	a, err := h.svc.Actor(r.Context(), u)
	if err != nil {
		return err
	}
	out, err := h.svc.CorrectOvertime(r.Context(), a, id, in, auditMeta(r))
	if err != nil {
		return err
	}
	return ok(w, out)
}
```

`backend/internal/modules/hris/routes.go`, di bawah route day-type:

```go
		r("PATCH /api/hris/overtime/{id}", h.withAction("update", overtimeManagers, h.correctOvertime)),
```

Regenerate dan daftar Go-native:

Run: `cd backend && PATH=$HOME/.local/go/bin:$PATH make generate`
Expected: `go-routes.generated.json` bertambah `PATCH /api/hris/overtime/{id}`.

`frontend/src/lib/backend-routes.test.ts`, ganti `"/api/hris/overtime/day-type"];` dengan:

```ts
      "/api/hris/overtime/day-type", "/api/hris/overtime/{id}"];
```

- [ ] **Step 4: Jalankan test, pastikan lulus**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris -run 'TestOvertime' -count=1 -v`
Expected: PASS.

Run: `cd frontend && npx vitest run src/lib/backend-routes.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -l backend/internal/modules/hris
git add backend/internal/modules/hris/overtime_correction.go backend/internal/modules/hris/overtime.go backend/internal/modules/hris/routes.go backend/internal/modules/hris/overtime_correction_integration_test.go frontend/src/lib/go-routes.generated.json frontend/src/lib/backend-routes.test.ts
git commit -m "feat(hris): koreksi lembur oleh AdminHRIS dengan audit sebelum/sesudah (EPIC-063 Task 4)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---
### Task 5: Antrean Persetujuan Tim + badge (Go)

**Files:**
- Modify: `backend/internal/modules/hris/approvals.go` (`ApprovalRepo`, `TeamApprovals`, `teamPendingTotal`, SQL, handler)
- Modify: `backend/internal/modules/hris/service.go:15-36` (`Repository` menyertakan `ApprovalRepo`)
- Modify: `backend/internal/modules/hris/selfservice.go:291-296` (`NavBadges`)
- Modify: `backend/internal/modules/hris/ports.go:29-33` (`PayrollReads.ManagerPendingLoanCount`)
- Modify: `backend/internal/app/adapters_hris.go:88-95` (adapter SQL)
- Modify: `backend/internal/modules/hris/harness_test.go` (`fakePayroll`), `backend/internal/modules/hris/people_integration_test.go:171` (badge ESS)
- Modify: `backend/internal/modules/hris/routes.go` (self service)
- Modify: `frontend/src/lib/go-routes.generated.json`, `frontend/src/lib/backend-routes.test.ts`
- Test: `backend/internal/modules/hris/team_approvals_integration_test.go`

**Interfaces:**
- Consumes: `leaveBody` (Task 2), `otBody` (Task 3), `SubordinateIDs` tanpa diri sendiri (Task 2).
- Produces:
  - `type ApprovalRepo interface { TeamPendingLeaves(ctx context.Context, managerID string) ([]*Row, error); TeamPendingOvertime(ctx context.Context, managerID string) ([]*Row, error); TeamPendingCount(ctx context.Context, managerID string) (int64, error) }` (Task 6 menambah `EmployeesWithoutManager`)
  - `PayrollReads.ManagerPendingLoanCount(ctx context.Context, q database.Querier, managerID string) (int64, error)`
  - `func (s *Service) TeamApprovals(ctx context.Context, a *Actor) (*Row, error)` → `{leaves: [...], overtime: [...]}`
  - Route `GET /api/hris/me/team-approvals` → `{"data": {"leaves": [{id, employee_id, leave_type, start_date, end_date, total_days, reason, attachment_url, created_at, employee: {id, full_name, nip}}], "overtime": [{id, employee_id, date, start_time, end_time, hours, day_type, reason, created_at, employee}]}}` (tanggal sebagai `YYYY-MM-DD`)
  - Badge `nav-badges` kunci `/dashboard/me/persetujuan-tim` (setelah `/dashboard/me/pengumuman`)
  - `var fakeManagerLoans int64` (test)

- [ ] **Step 1: Tulis test yang gagal**

`backend/internal/modules/hris/team_approvals_integration_test.go`:

```go
package hris

import "testing"

// EPIC-063 R7: antrean Persetujuan Tim = cuti & lembur pending bawahan
// langsung; badge ikut Perjalanan Dinas dan kasbon tahap manager.
func TestTeamApprovalsQueueAndBadge(t *testing.T) {
	h := newHarness(t)
	boss, bossID := h.staff("pos", nil, true, emp{name: "Atasan Tim"})
	memberA, _ := h.staff("pos", nil, true, emp{name: "Anggota A", boss: &bossID})
	memberB, _ := h.staff("pos", nil, true, emp{name: "Anggota B", boss: &bossID})
	outsider, _ := h.staff("pos", nil, true, emp{name: "Bukan Tim"})
	unlinked, _ := h.staff("pos", nil, false)
	// Data live: atasan yang reporting_to-nya dirinya sendiri.
	h.exec(`UPDATE hris.employees SET reporting_to = id WHERE id = $1`, bossID)
	fakeManagerLoans = 2
	t.Cleanup(func() { fakeManagerLoans = 0 })

	expect(t, h.do(&memberA, "POST", "/api/hris/leaves", leaveBody("2031-03-04", "2031-03-04")), 200, "")
	expect(t, h.do(&outsider, "POST", "/api/hris/leaves", leaveBody("2031-03-04", "2031-03-04")), 200, "")
	expect(t, h.do(&memberB, "POST", "/api/hris/overtime", otBody("2026-10-06", "17:00", "19:00")), 200, "")
	expect(t, h.do(&boss, "POST", "/api/hris/overtime", otBody("2026-10-06", "17:00", "19:00")), 200, "")
	expect(t, h.do(&memberA, "POST", "/api/hris/business-trips", map[string]any{"start_date": "2026-10-10",
		"end_date": "2026-10-11", "destination": "Gudang Cikarang", "purpose": "Stock opname gudang"}), 200, "")

	q := h.do(&boss, "GET", "/api/hris/me/team-approvals", nil)
	expect(t, q, 200, "")
	leaves, _ := q.data()["leaves"].([]any)
	overtime, _ := q.data()["overtime"].([]any)
	if len(leaves) != 1 || len(overtime) != 1 {
		t.Fatalf("queue = %s", q.raw)
	}
	l := leaves[0].(map[string]any)
	if l["start_date"] != "2031-03-04" || l["leave_type"] != "annual" || l["employee"].(map[string]any)["full_name"] != "Anggota A" {
		t.Fatalf("leave = %v", l)
	}
	o := overtime[0].(map[string]any)
	if o["date"] != "2026-10-06" || o["day_type"] != "regular" || o["employee"].(map[string]any)["full_name"] != "Anggota B" {
		t.Fatalf("overtime = %v", o)
	}

	badges := h.do(&boss, "GET", "/api/hris/nav-badges", nil)
	if got := badges.body["badges"].(map[string]any)["/dashboard/me/persetujuan-tim"]; got != 5.0 {
		t.Fatalf("badge = %v (want 1 cuti + 1 lembur + 1 dinas + 2 kasbon) in %s", got, badges.raw)
	}

	expect(t, h.do(&boss, "POST", "/api/hris/leaves/approve", map[string]any{"leave_id": l["id"], "action": "approve"}), 200, "")
	after := h.do(&boss, "GET", "/api/hris/me/team-approvals", nil)
	if left, _ := after.data()["leaves"].([]any); len(left) != 0 {
		t.Fatalf("cuti yang diputus masih di antrean: %s", after.raw)
	}

	empty := h.do(&unlinked, "GET", "/api/hris/me/team-approvals", nil)
	expect(t, empty, 200, "")
	if empty.raw != `{"data":{"leaves":[],"overtime":[]}}` {
		t.Fatalf("akun tanpa karyawan = %s", empty.raw)
	}
	expect(t, h.do(nil, "GET", "/api/hris/me/team-approvals", nil), 401, "Unauthorized")
}
```

`backend/internal/modules/hris/people_integration_test.go:171`, ganti string yang diharapkan menjadi:

```go
	if badges.raw != `{"badges":{"/dashboard/me/cuti":0,"/dashboard/me/lembur":0,"/dashboard/me/pinjaman":0,"/dashboard/me/pengumuman":0,"/dashboard/me/persetujuan-tim":0}}` {
```

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris -run 'TestTeamApprovalsQueueAndBadge|TestSelfService' -count=1 -v`
Expected: FAIL kompilasi `undefined: fakeManagerLoans`.

- [ ] **Step 3: Tulis implementasi**

`backend/internal/modules/hris/ports.go`, di `PayrollReads` setelah `PendingLoanCount`:

```go
	// ManagerPendingLoanCount counts kasbon waiting for managerID's stage
	// (EPIC-060 manager_approver_id), for the ESS Persetujuan Tim badge.
	ManagerPendingLoanCount(ctx context.Context, q database.Querier, managerID string) (int64, error)
```

`backend/internal/app/adapters_hris.go`, setelah `PendingLoanCount`:

```go
// ManagerPendingLoanCount is the manager's kasbon queue (the scope=approvals
// list of payroll/loans.go): pending loans naming them as manager approver.
func (hrisPayrollSQL) ManagerPendingLoanCount(ctx context.Context, q database.Querier, managerID string) (int64, error) {
	var n int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM hris.loans
		WHERE status = 'pending' AND manager_approver_id = $1`, managerID).Scan(&n)
	return n, err
}
```

`backend/internal/modules/hris/harness_test.go`, setelah `func (fakePayroll) PendingLoanCount…`:

```go
// fakeManagerLoans is the kasbon count fakePayroll reports for a manager's
// stage (tests set it).
var fakeManagerLoans int64

func (fakePayroll) ManagerPendingLoanCount(context.Context, database.Querier, string) (int64, error) {
	return fakeManagerLoans, nil
}
```

`backend/internal/modules/hris/service.go`, di `Repository` setelah `TripRepo`:

```go
	ApprovalRepo
```

`backend/internal/modules/hris/approvals.go` — tambahkan `"fmt"` dan `"net/http"` ke import, lalu tambahkan di akhir file:

```go
/* ── Persetujuan Tim (ESS, EPIC-063 R7) ─────────────────────────────── */

// teamApprovalsHref is the ESS menu the team badge belongs to.
const teamApprovalsHref = "/dashboard/me/persetujuan-tim"

// ApprovalRepo reads the direct manager's queue (and, Task 6, the
// reporting_to data quality report).
type ApprovalRepo interface {
	TeamPendingLeaves(ctx context.Context, managerID string) ([]*Row, error)
	TeamPendingOvertime(ctx context.Context, managerID string) ([]*Row, error)
	// TeamPendingCount sums pending leave, employee overtime and business
	// trip requests of managerID's direct reports.
	TeamPendingCount(ctx context.Context, managerID string) (int64, error)
}

// TeamApprovals is GET /api/hris/me/team-approvals: pending leave and
// overtime requests of the actor's direct reports. Business trips and kasbon
// keep their own queues (GET /api/hris/business-trips?scope=approvals,
// GET /api/hris/loans?scope=approvals), which the ESS page reuses.
func (s *Service) TeamApprovals(ctx context.Context, a *Actor) (*Row, error) {
	if a.EmployeeID == nil {
		return obj("leaves", []*Row{}, "overtime", []*Row{}), nil
	}
	leaves, err := s.repo.TeamPendingLeaves(ctx, *a.EmployeeID)
	if err != nil {
		return nil, err
	}
	overtime, err := s.repo.TeamPendingOvertime(ctx, *a.EmployeeID)
	if err != nil {
		return nil, err
	}
	return obj("leaves", leaves, "overtime", overtime), nil
}

// teamPendingTotal is the Persetujuan Tim badge: everything waiting for the
// manager, kasbon at the manager stage included.
func (s *Service) teamPendingTotal(ctx context.Context, managerID string) (int64, error) {
	n, err := s.repo.TeamPendingCount(ctx, managerID)
	if err != nil {
		return 0, err
	}
	loans, err := s.payroll.ManagerPendingLoanCount(ctx, s.repo.querier(), managerID)
	return n + loans, err
}

// teamMember joins the requester (alias m) when they report to $1 directly;
// a reporting_to pointing at oneself (live data) is not a team member (R2).
func teamMember(alias string) string {
	return fmt.Sprintf(`JOIN hris.employees m ON m.id = %s.employee_id AND m.reporting_to = $1 AND m.id <> $1`, alias)
}

const teamEmployee = `json_build_object('id', m.id, 'full_name', m.full_name, 'nip', m.nip) AS employee`

func (s *store) TeamPendingLeaves(ctx context.Context, managerID string) ([]*Row, error) {
	rows, err := queryRows(ctx, s.db, `SELECT l.id, l.employee_id, l.leave_type, l.start_date::text AS start_date,
		l.end_date::text AS end_date, l.total_days, l.reason, l.attachment_url, l.created_at, `+teamEmployee+`
		FROM hris.leaves l `+teamMember("l")+`
		WHERE l.status = 'pending'
		ORDER BY l.start_date, l.created_at LIMIT 200`, managerID)
	return rows, ignoreBadInput(err)
}

func (s *store) TeamPendingOvertime(ctx context.Context, managerID string) ([]*Row, error) {
	rows, err := queryRows(ctx, s.db, `SELECT o.id, o.employee_id, o.date::text AS date, o.start_time, o.end_time,
		o.hours, o.day_type, o.reason, o.created_at, `+teamEmployee+`
		FROM hris.overtime_requests o `+teamMember("o")+`
		WHERE o.status = 'pending' AND o.source = 'employee'
		ORDER BY o.date, o.created_at LIMIT 200`, managerID)
	return rows, ignoreBadInput(err)
}

func (s *store) TeamPendingCount(ctx context.Context, managerID string) (int64, error) {
	var n int64
	err := s.db.QueryRow(ctx, `SELECT
		  (SELECT count(*) FROM hris.leaves l `+teamMember("l")+` WHERE l.status = 'pending')
		+ (SELECT count(*) FROM hris.overtime_requests o `+teamMember("o")+` WHERE o.status = 'pending' AND o.source = 'employee')
		+ (SELECT count(*) FROM hris.business_trips t `+teamMember("t")+` WHERE t.status = 'pending')`,
		managerID).Scan(&n)
	return n, ignoreBadInput(err)
}

// GET /api/hris/me/team-approvals
func (h handlers) teamApprovals(w http.ResponseWriter, r *http.Request, a *Actor) error {
	out, err := h.svc.TeamApprovals(r.Context(), a)
	if err != nil {
		return err
	}
	return ok(w, dataOf(out))
}
```

`backend/internal/modules/hris/selfservice.go`, di `NavBadges` ganti:

```go
		badges.Set(announcementsHref, n)
	}
	return badges, nil
```

dengan:

```go
		badges.Set(announcementsHref, n)
		team, err := s.teamPendingTotal(ctx, *a.EmployeeID)
		if err != nil {
			return nil, err
		}
		badges.Set(teamApprovalsHref, team)
	}
	return badges, nil
```

`backend/internal/modules/hris/routes.go`, di blok self service setelah `r("GET /api/hris/me/team", …),`:

```go
		r("GET /api/hris/me/team-approvals", h.withActor(h.teamApprovals)),
```

Run: `cd backend && PATH=$HOME/.local/go/bin:$PATH make generate`
Expected: manifest bertambah `GET /api/hris/me/team-approvals`.

`frontend/src/lib/backend-routes.test.ts`, ganti `"/api/hris/overtime/day-type", "/api/hris/overtime/{id}"];` dengan:

```ts
      "/api/hris/overtime/day-type", "/api/hris/overtime/{id}", "/api/hris/me/team-approvals"];
```

- [ ] **Step 4: Jalankan test, pastikan lulus**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris/... ./internal/app/... -count=1`
Expected: PASS (atau hanya nama di baseline).

Run: `cd frontend && npx vitest run src/lib/backend-routes.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -l backend/internal
git add backend/internal/modules/hris/approvals.go backend/internal/modules/hris/service.go backend/internal/modules/hris/selfservice.go backend/internal/modules/hris/ports.go backend/internal/app/adapters_hris.go backend/internal/modules/hris/harness_test.go backend/internal/modules/hris/people_integration_test.go backend/internal/modules/hris/routes.go backend/internal/modules/hris/team_approvals_integration_test.go frontend/src/lib/go-routes.generated.json frontend/src/lib/backend-routes.test.ts
git commit -m "feat(hris): antrean Persetujuan Tim dan badge pending atasan (EPIC-063 Task 5)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 6: Kualitas data `reporting_to` — tolak atasan = diri sendiri, laporan karyawan tanpa atasan (Go)

**Files:**
- Create: `backend/internal/modules/hris/reporting_to.go`
- Modify: `backend/internal/modules/hris/approvals.go` (`ApprovalRepo.EmployeesWithoutManager`)
- Modify: `backend/internal/modules/hris/employees.go:130-137` (`UpdateEmployee`)
- Modify: `backend/internal/modules/hris/domain/employee_import.go:321-327` (kolom "NIP Atasan")
- Modify: `backend/internal/modules/configuration/accounts/routes.go:240-243` (`updateUser`)
- Modify: `backend/internal/modules/hris/routes.go` (route laporan)
- Modify: `frontend/src/lib/go-routes.generated.json`, `frontend/src/lib/backend-routes.test.ts`
- Test: `backend/internal/modules/hris/reporting_to_integration_test.go`, `backend/internal/modules/hris/domain/employee_import_test.go`, `backend/internal/modules/configuration/accounts/accounts_integration_test.go`

**Interfaces:**
- Consumes: `employeeRecordManagers` (`http_employees.go`), `validate.CanonicalUUID`, `CleanNIP`.
- Produces:
  - `const errSelfManager = "Atasan langsung tidak boleh karyawan itu sendiri"`
  - `func selfManager(employeeID string, reportingTo *string) bool`
  - `ApprovalRepo.EmployeesWithoutManager(ctx context.Context) ([]*Row, error)`
  - Route `GET /api/hris/employees/without-manager` (menu `employeeRecordManagers`) → `{"data": [{id, nip, full_name, job_title, department, reason: "empty"|"self"|"inactive"}]}`, maks. 500 baris

- [ ] **Step 1: Tulis test yang gagal**

`backend/internal/modules/hris/reporting_to_integration_test.go`:

```go
package hris

import (
	"strings"
	"testing"
)

// EPIC-063 R9: atasan langsung tidak boleh diri sendiri; laporan baca-saja.
func TestEmployeeCannotReportToSelf(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuKepegawaian}, false)
	plain, _ := h.staff("pos", nil, true)
	loop := h.employee(emp{name: "Loop Uji Atasan"})
	boss := h.employee(emp{name: "Atasan Uji Laporan"})
	const msg = "Atasan langsung tidak boleh karyawan itu sendiri"

	expect(t, h.do(&hr, "PUT", "/api/hris/employees/"+loop, map[string]any{"reporting_to": loop}), 400, msg)
	expect(t, h.do(&hr, "PUT", "/api/hris/employees/"+loop, map[string]any{"reporting_to": strings.ToUpper(loop)}), 400, msg)
	expect(t, h.do(&hr, "PUT", "/api/hris/employees/"+loop, map[string]any{"reporting_to": boss}), 200, "")

	h.exec(`UPDATE hris.employees SET reporting_to = id WHERE id = $1`, loop) // data live lama
	h.employee(emp{name: "Tanpa Atasan Uji"})
	gone := h.employee(emp{name: "Atasan Keluar Uji", inactive: true})
	h.employee(emp{name: "Anak Atasan Keluar", boss: &gone})

	rep := h.do(&hr, "GET", "/api/hris/employees/without-manager", nil)
	expect(t, rep, 200, "")
	reasons := map[string]any{}
	for _, item := range rep.list() {
		row := item.(map[string]any)
		reasons[row["full_name"].(string)] = row["reason"]
	}
	if reasons["Loop Uji Atasan"] != "self" || reasons["Tanpa Atasan Uji"] != "empty" ||
		reasons["Anak Atasan Keluar"] != "inactive" {
		t.Fatalf("reasons = %v", reasons)
	}
	if _, listed := reasons["Atasan Keluar Uji"]; listed {
		t.Fatal("karyawan nonaktif tidak masuk laporan")
	}
	if _, listed := reasons["Atasan Uji Laporan"]; !listed {
		t.Fatal("Atasan Uji Laporan (tanpa atasan) harus terdaftar sebagai empty")
	}
	expect(t, h.do(&plain, "GET", "/api/hris/employees/without-manager", nil), 403, "Insufficient permissions")
}
```

`backend/internal/modules/hris/domain/employee_import_test.go`, tambahkan di akhir file:

```go
func TestValidateEmployeeImportRefusesSelfManager(t *testing.T) {
	header := append(append([]string{}, hdr...), "NIP Atasan")
	rows := [][]string{{"202505272", "Feti", "", "", "", "", "", "", "", "", "", "202505272"}}
	_, errs, _ := ValidateEmployeeImport("replace", header, rows, 2, lk())
	if !issueText(errs, "Baris 2, kolom NIP Atasan: NIP atasan tidak boleh NIP karyawan itu sendiri") {
		t.Fatalf("errors %+v", errs)
	}
}
```

`backend/internal/modules/configuration/accounts/accounts_integration_test.go`, tambahkan di akhir file:

```go
// EPIC-063 R9: Manajemen User / Karyawan tidak boleh menyimpan atasan = diri sendiri.
func TestUpdateUserRefusesSelfManager(t *testing.T) {
	e := setup(t, map[string][]string{"hris.kepegawaian.users": {"read", "create", "update"}})
	plain := employeeBody(uniqueEmail())
	plain["is_access_app"] = false
	code, body, raw := e.do("POST", "/api/users", plain)
	if code != 201 {
		t.Fatalf("POST employee = %d %s", code, raw)
	}
	id := body["data"].(map[string]any)["id"].(string)
	code, body, _ = e.do("PUT", "/api/users/"+id, map[string]any{"reporting_to": id})
	wantError(t, code, body, 400, "Atasan langsung tidak boleh karyawan itu sendiri")
}
```

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris/... ./internal/modules/configuration/accounts -run 'TestEmployeeCannotReportToSelf|TestValidateEmployeeImportRefusesSelfManager|TestUpdateUserRefusesSelfManager' -count=1 -v`
Expected: FAIL — PUT diterima 200; import memberi `NIP atasan 202505272 tidak ditemukan / tidak aktif`.

- [ ] **Step 3: Tulis implementasi**

`backend/internal/modules/hris/reporting_to.go`:

```go
package hris

import (
	"context"
	"net/http"

	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/validate"
)

// Kualitas data atasan langsung (EPIC-063 R9). Validasi aplikasi, tanpa
// CHECK DB: satu baris live masih menunjuk dirinya sendiri dan constraint
// akan memblokir update kolom lain pada baris itu.

const errSelfManager = "Atasan langsung tidak boleh karyawan itu sendiri"

// selfManager: reportingTo names employeeID itself (any case or hyphen form).
func selfManager(employeeID string, reportingTo *string) bool {
	if reportingTo == nil {
		return false
	}
	a, okA := validate.CanonicalUUID(employeeID)
	b, okB := validate.CanonicalUUID(*reportingTo)
	return okA && okB && a == b
}

// EmployeesWithoutManager lists active employees whose leave and overtime only
// HR can decide: reporting_to empty, pointing at themselves, or at an
// inactive employee.
func (s *Service) EmployeesWithoutManager(ctx context.Context) ([]*Row, error) {
	return s.repo.EmployeesWithoutManager(ctx)
}

func (s *store) EmployeesWithoutManager(ctx context.Context) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT e.id, e.nip, e.full_name, p.title AS job_title, d.name AS department,
		CASE WHEN e.reporting_to IS NULL THEN 'empty'
		     WHEN e.reporting_to = e.id THEN 'self'
		     ELSE 'inactive' END AS reason
		FROM hris.employees e
		LEFT JOIN hris.employees m ON m.id = e.reporting_to
		LEFT JOIN hris.positions p ON p.id = e.job_title_id
		LEFT JOIN hris.departments d ON d.id = e.department_id
		WHERE e.is_active AND (e.reporting_to IS NULL OR e.reporting_to = e.id OR m.is_active IS NOT TRUE)
		ORDER BY reason, e.full_name LIMIT 500`)
}

// GET /api/hris/employees/without-manager
func (h handlers) employeesWithoutManager(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	rows, err := h.svc.EmployeesWithoutManager(r.Context())
	if err != nil {
		return err
	}
	return ok(w, dataOf(rows))
}
```

`backend/internal/modules/hris/approvals.go`, tambahkan ke `ApprovalRepo`:

```go
	// EmployeesWithoutManager is the reporting_to data quality report (R9).
	EmployeesWithoutManager(ctx context.Context) ([]*Row, error)
```

`backend/internal/modules/hris/employees.go`, di `UpdateEmployee` setelah blok `if current == nil { … }`:

```go
	if in.reportingTo.Sent && selfManager(id, in.reportingTo.Val) {
		return nil, httpx.BadRequest(errSelfManager)
	}
```

`backend/internal/modules/hris/domain/employee_import.go`, ganti blok `case k == "manager_nip":` dengan:

```go
			case k == "manager_nip":
				if m, _ := CleanNIP(val); m != "" && m == nip {
					fail(k, "NIP atasan tidak boleh NIP karyawan itu sendiri")
					continue
				}
				id, ok := lk.ManagerNIP[val]
				if !ok {
					fail(k, "NIP atasan "+val+" tidak ditemukan / tidak aktif")
					continue
				}
				row.Set["reporting_to"] = id
```

`backend/internal/modules/configuration/accounts/routes.go`, di `updateUser` setelah blok `if err := f.Err("Validation failed"); err != nil { return err }`:

```go
	// EPIC-063 R9: atasan langsung tidak boleh karyawan itu sendiri.
	if to := b.text["reporting_to"]; to != nil {
		id, okID := validate.CanonicalUUID(r.PathValue("id"))
		boss, okBoss := validate.CanonicalUUID(*to)
		if okID && okBoss && id == boss {
			return httpx.BadRequest("Atasan langsung tidak boleh karyawan itu sendiri")
		}
	}
```

`backend/internal/modules/hris/routes.go`, di blok employees setelah `r("GET /api/hris/employees/import-template", …),`:

```go
		r("GET /api/hris/employees/without-manager", h.withMenu(employeeRecordManagers, h.employeesWithoutManager)),
```

Run: `cd backend && PATH=$HOME/.local/go/bin:$PATH make generate`
Expected: manifest bertambah `GET /api/hris/employees/without-manager`.

`frontend/src/lib/backend-routes.test.ts`, ganti `"/api/hris/me/team-approvals"];` dengan:

```ts
      "/api/hris/me/team-approvals", "/api/hris/employees/without-manager"];
```

- [ ] **Step 4: Jalankan test, pastikan lulus**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris/... ./internal/modules/configuration/accounts -count=1`
Expected: PASS (atau hanya nama di baseline).

Run: `cd frontend && npx vitest run src/lib/backend-routes.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -l backend/internal
git add backend/internal/modules/hris/reporting_to.go backend/internal/modules/hris/approvals.go backend/internal/modules/hris/employees.go backend/internal/modules/hris/domain/employee_import.go backend/internal/modules/configuration/accounts/routes.go backend/internal/modules/hris/routes.go backend/internal/modules/hris/reporting_to_integration_test.go backend/internal/modules/hris/domain/employee_import_test.go backend/internal/modules/configuration/accounts/accounts_integration_test.go frontend/src/lib/go-routes.generated.json frontend/src/lib/backend-routes.test.ts
git commit -m "feat(hris): tolak atasan = diri sendiri dan laporan karyawan tanpa atasan (EPIC-063 Task 6)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 7: Paritas fallback Next untuk route yang melayani path yang sama

**Files:**
- Modify: `frontend/src/lib/hris/overtime-rules.ts` (aturan R1/R3 + jenis lembur)
- Modify: `frontend/src/lib/hris/overtime-rules.test.ts`
- Create: `frontend/src/lib/hris/reporting-to.ts`, `frontend/src/lib/hris/reporting-to.test.ts`
- Modify: `frontend/src/lib/hris/workforce-auth.ts` (`fullName`)
- Modify: `frontend/src/lib/audit/index.ts` (aksi & entitas audit)
- Modify: `frontend/src/lib/hris/leaves-repo.ts:271-349` (`decideLeave`)
- Modify: `frontend/src/lib/hris/overtime-repo.ts` (`listOvertime`, `createOvertime`, `decideOvertime`)
- Modify: `frontend/src/lib/hris/leave-wa.ts:58,73` (footer + tanpa diri sendiri)
- Modify: `frontend/src/app/api/hris/employees/[id]/route.ts`, `frontend/src/app/api/users/[id]/route.ts`
- Modify: `frontend/src/features/hris/overtime/types.ts:3` (`OvertimeDayType` dari lib)

**Interfaces:**
- Consumes: `canonicalUuid` (`lib/api/canonical-uuid.ts`), `recordAudit`, `recordAuditAfterCommit` (`lib/audit`), `withTransaction`.
- Produces (dipakai Task 8–10):
  - `export type OvertimeDayType = "regular" | "rest_day"`
  - `export const OVERTIME_DAY_TYPE_OPTIONS: readonly { value: OvertimeDayType; label: string }[]` (`Weekday`, `Weekend / Libur`)
  - `export function overtimeDayTypeLabel(dayType: string): string`
  - `export const OVERTIME_TOO_LATE: string`
  - `export function overtimeStartsInPast(date: string, start: string, now?: Date): boolean`
  - `export function decidedAs(actor: OvertimeDecisionActor, ownerId: string, reportingTo: string | null): "manager" | "hr" | null`
  - `export const SELF_MANAGER_ERROR`, `export function isSelfManager(employeeId: string, reportingTo: string | null | undefined): boolean`
  - `WorkforceActor.fullName?: string | null`

- [ ] **Step 1: Tulis test yang gagal**

`frontend/src/lib/hris/overtime-rules.test.ts` — ubah import di atas menjadi:

```ts
import { describe, it, expect } from "vitest";
import {
  canDecideOvertime,
  decidedAs,
  overtimeDayTypeLabel,
  overtimeStartsInPast,
  type OvertimeDecisionActor,
  type OvertimeDecisionRequest,
} from "./overtime-rules";
```

lalu tambahkan di akhir file:

```ts
describe("overtimeStartsInPast — MOM AP-02 (EPIC-063 R3)", () => {
  const now = new Date("2026-10-05T03:00:30Z"); // Senin 10:00:30 WIB

  it("menit berjalan masih boleh, sebelumnya sudah lewat", () => {
    expect(overtimeStartsInPast("2026-10-05", "10:00", now)).toBe(false);
    expect(overtimeStartsInPast("2026-10-05", "09:59", now)).toBe(true);
    expect(overtimeStartsInPast("2026-10-04", "23:00", now)).toBe(true);
    expect(overtimeStartsInPast("2026-10-06", "00:00", now)).toBe(false);
  });

  it("memakai jam WIB, bukan tanggal UTC", () => {
    const late = new Date("2026-10-05T16:45:00Z"); // 23:45 WIB
    expect(overtimeStartsInPast("2026-10-06", "00:15", late)).toBe(false);
    expect(overtimeStartsInPast("2026-10-05", "23:30", late)).toBe(true);
  });

  it("format tidak dikenal diserahkan ke validasi input", () => {
    expect(overtimeStartsInPast("05-10-2026", "10:00", now)).toBe(false);
  });
});

describe("decidedAs — Q-APV-3 (EPIC-063 R1/R2)", () => {
  it("atasan langsung lebih dulu, HR sebagai cadangan", () => {
    expect(decidedAs(managerActor, EMPLOYEE, MANAGER)).toBe("manager");
    expect(decidedAs(hrActor, EMPLOYEE, MANAGER)).toBe("hr");
    expect(decidedAs({ employeeId: null, isHr: true }, EMPLOYEE, null)).toBe("hr");
    expect(decidedAs(otherActor, EMPLOYEE, MANAGER)).toBeNull();
  });

  it("tidak ada yang memutus pengajuannya sendiri", () => {
    expect(decidedAs({ employeeId: EMPLOYEE, isHr: true }, EMPLOYEE, MANAGER)).toBeNull();
    expect(decidedAs(selfActor, EMPLOYEE, EMPLOYEE)).toBeNull();
  });
});

describe("overtimeDayTypeLabel — K-13", () => {
  it("Weekday / Weekend-Libur", () => {
    expect(overtimeDayTypeLabel("regular")).toBe("Weekday");
    expect(overtimeDayTypeLabel("rest_day")).toBe("Weekend / Libur");
  });
});
```

`frontend/src/lib/hris/reporting-to.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { isSelfManager } from "./reporting-to";

const ID = "3f2b8c1e-4a5d-4e6f-8a9b-0c1d2e3f4a5b";

describe("isSelfManager (EPIC-063 R9)", () => {
  it("menolak atasan = diri sendiri dalam bentuk uuid apa pun", () => {
    expect(isSelfManager(ID, ID)).toBe(true);
    expect(isSelfManager(ID, ID.toUpperCase())).toBe(true);
    expect(isSelfManager(ID, ID.replaceAll("-", ""))).toBe(true);
  });

  it("atasan lain, kosong, atau bukan uuid lolos", () => {
    expect(isSelfManager(ID, "9f2b8c1e-4a5d-4e6f-8a9b-0c1d2e3f4a5b")).toBe(false);
    expect(isSelfManager(ID, null)).toBe(false);
    expect(isSelfManager(ID, undefined)).toBe(false);
    expect(isSelfManager(ID, "")).toBe(false);
  });
});
```

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd frontend && npx vitest run src/lib/hris/overtime-rules.test.ts src/lib/hris/reporting-to.test.ts`
Expected: FAIL — `decidedAs is not a function` / `Failed to resolve import "./reporting-to"`.

- [ ] **Step 3: Tulis implementasi**

`frontend/src/lib/hris/overtime-rules.ts`, tambahkan di akhir file:

```ts
/* ── EPIC-063: approval atasan & lembur sebelum dimulai ─────────────── */

/** K-13: Weekday = regular (1,5×), Weekend / Libur = rest_day (2×); tarif dari setting payroll. */
export type OvertimeDayType = "regular" | "rest_day";

export const OVERTIME_DAY_TYPE_OPTIONS: readonly { value: OvertimeDayType; label: string }[] = [
  { value: "regular", label: "Weekday" },
  { value: "rest_day", label: "Weekend / Libur" },
];

export function overtimeDayTypeLabel(dayType: string): string {
  return dayType === "rest_day" ? "Weekend / Libur" : "Weekday";
}

export const OVERTIME_TOO_LATE = "Lembur wajib diajukan sebelum dimulai — jam mulai sudah lewat";

/**
 * MOM AP-02 (EPIC-063 R3): tanggal + jam mulai (jam dinding WIB) sebelum awal
 * menit berjalan WIB. Sama dengan Go domain.OvertimeStartsInPast; format yang
 * tidak dikenal = false (diserahkan ke validasi input).
 */
export function overtimeStartsInPast(date: string, start: string, now: Date = new Date()): boolean {
  const d = /^(\d{4})-(\d{2})-(\d{2})$/.exec(date);
  const t = /^(\d{2}):(\d{2})$/.exec(start);
  if (!d || !t) return false;
  const startUtc = Date.UTC(Number(d[1]), Number(d[2]) - 1, Number(d[3]), Number(t[1]) - 7, Number(t[2]));
  const minuteNow = Math.floor(now.getTime() / 60_000) * 60_000;
  return startUtc < minuteNow;
}

/**
 * Kapasitas pemutus cuti/lembur (Q-APV-3, EPIC-063 R1/R2): atasan langsung →
 * "manager", HR → "hr" (cadangan & override), null = tidak boleh. Pemohon
 * tidak pernah memutus pengajuannya sendiri. Sama dengan Go domain.DecideAs.
 */
export function decidedAs(
  actor: OvertimeDecisionActor,
  ownerId: string,
  reportingTo: string | null
): "manager" | "hr" | null {
  if (actor.employeeId !== null && actor.employeeId === ownerId) return null;
  if (actor.employeeId !== null && reportingTo === actor.employeeId) return "manager";
  return actor.isHr ? "hr" : null;
}
```

`frontend/src/features/hris/overtime/types.ts:1-3`, ganti:

```ts
import type { OvertimeDecisionAction } from "@/lib/hris/overtime-rules";

export type OvertimeDayType = "regular" | "rest_day";
```

dengan:

```ts
import type { OvertimeDayType, OvertimeDecisionAction } from "@/lib/hris/overtime-rules";

export type { OvertimeDayType };
```

`frontend/src/lib/hris/reporting-to.ts`:

```ts
import { canonicalUuid } from "@/lib/api/canonical-uuid";

/** EPIC-063 R9: sama dengan Go errSelfManager. */
export const SELF_MANAGER_ERROR = "Atasan langsung tidak boleh karyawan itu sendiri";

/** reportingTo menunjuk karyawan itu sendiri (huruf/tanda hubung apa pun). */
export function isSelfManager(employeeId: string, reportingTo: string | null | undefined): boolean {
  if (!reportingTo) return false;
  const self = canonicalUuid(employeeId);
  return self !== null && self === canonicalUuid(reportingTo);
}
```

`frontend/src/lib/hris/workforce-auth.ts`:
- di `interface WorkforceActor` tambahkan setelah `isHr`:

```ts
  /** nama akun (configuration.users.full_name) untuk decided_by_name (EPIC-063) */
  fullName?: string | null;
```

- ganti `db.from("users").select("role").eq("id", user.id).single(),` dengan `db.from("users").select("role, full_name").eq("id", user.id).single(),`
- di objek yang dikembalikan tambahkan `fullName: userData?.full_name ?? null,` setelah `isHr: …`.

`frontend/src/lib/audit/index.ts`:
- di union `AuditAction` setelah `| "attendance.delete"` tambahkan:

```ts
  | "leave.approve"
  | "leave.reject"
  | "overtime.approve"
  | "overtime.reject"
```

- di union `AuditEntity` setelah `| "purchase_return"` tambahkan `| "hris.overtime_requests"`.

`frontend/src/lib/hris/leaves-repo.ts`:
- tambahkan import `import { recordAudit } from "@/lib/audit";` dan `import { decidedAs } from "./overtime-rules";`
- di `decideLeave`, ganti blok:

```ts
  const isDirectManager =
    actor.employeeId !== null && leave.employee?.reporting_to === actor.employeeId;
  if (!actor.isHr && !isDirectManager) {
    throw ApiError.forbidden("Forbidden: hanya HRD/admin atau atasan langsung yang bisa memproses");
  }
```

dengan:

```ts
  // Q-APV-3 (EPIC-063 R1/R2), sama dengan Go DecideLeave.
  const capacity = decidedAs(
    { employeeId: actor.employeeId, isHr: actor.isHr },
    leave.employee_id,
    leave.employee?.reporting_to ?? null
  );
  if (!capacity) {
    throw ApiError.forbidden(
      actor.employeeId !== null && actor.employeeId === leave.employee_id
        ? "Tidak bisa memutuskan pengajuan cuti sendiri"
        : "Hanya atasan langsung atau HRD yang bisa memproses pengajuan ini"
    );
  }
```

- ganti seluruh blok `await withTransaction(async (client) => { … });` dengan:

```ts
  const isApprove = input.action === "approve";
  const status = isApprove ? "approved" : "rejected";
  const rejection = isApprove ? null : (input.rejection_reason ?? null);
  await withTransaction(async (client) => {
    const updated = await client.query(
      `UPDATE hris.leaves
       SET status = $2, approved_by = $3, approved_at = now(),
           rejection_reason = $4, decided_as = $5, decided_by_name = $6
       WHERE id = $1 AND status = 'pending'`,
      [input.leave_id, status, actor.employeeId, rejection, capacity, actor.fullName || "HRD"]
    );
    if (updated.rowCount === 0) throw ApiError.conflict("Pengajuan sudah diproses oleh orang lain");
    if (isApprove && leave.leave_type === "annual") {
      await client.query(
        `INSERT INTO hris.leave_balances (employee_id, year, annual_leave_total, annual_leave_used)
         VALUES ($1, $2, 12, $3)
         ON CONFLICT (employee_id, year)
         DO UPDATE SET annual_leave_used = leave_balances.annual_leave_used + $3,
                       updated_at = now()`,
        [leave.employee_id, new Date(leave.start_date).getFullYear(), leave.total_days]
      );
    }
    await recordAudit(client, {
      actor: { id: actor.userId, name: actor.fullName ?? null },
      action: isApprove ? "leave.approve" : "leave.reject",
      entity: "hris.leaves",
      entityId: input.leave_id,
      entityLabel: `${leave.employee?.full_name ?? ""} · ${leave.start_date} s.d. ${leave.end_date}`,
      before: { status: "pending" },
      after: { status, decided_as: capacity, rejection_reason: rejection },
    });
  });
```

(hapus baris `const isApprove = input.action === "approve";` lama yang berada tepat di atas blok transaksi.)

`frontend/src/lib/hris/overtime-repo.ts`:
- ubah import aturan menjadi `import { canDecideOvertime, decidedAs, OVERTIME_TOO_LATE, overtimeStartsInPast } from "./overtime-rules";` dan tambahkan `import { recordAuditAfterCommit } from "@/lib/audit";`
- di `listOvertime`, ganti `.eq("reporting_to", actor.employeeId);` dengan:

```ts
      .eq("reporting_to", actor.employeeId)
      .neq("id", actor.employeeId); // reporting_to = diri sendiri bukan bawahan (EPIC-063 R2)
```

- di `createOvertime`, setelah blok `if (!isSelf && !actor.isHr) { … }` tambahkan:

```ts
  // MOM AP-02 (EPIC-063 R3): pengajuan sendiri sebelum lembur dimulai.
  if (isSelf && overtimeStartsInPast(input.date, input.start_time)) {
    throw ApiError.badRequest(OVERTIME_TOO_LATE);
  }
```

- di `decideOvertime`, ganti blok dari `const status = DECISION_STATUS[input.action];` sampai `return { data, message: DECISION_MESSAGE[status] };` dengan:

```ts
  const status = DECISION_STATUS[input.action];
  const capacity =
    overtime.source === "employee" && input.action !== "cancel"
      ? decidedAs(
          { employeeId: actor.employeeId, isHr: actor.isHr },
          overtime.employee_id,
          overtime.employee?.reporting_to ?? null
        )
      : null;
  const rejection = input.action === "reject" ? (input.rejection_reason ?? null) : null;
  const now = new Date().toISOString();
  const { data, error } = await db
    .from("overtime_requests")
    .update({
      status,
      decided_by: actor.employeeId,
      decided_at: now,
      rejection_reason: rejection,
      ...(capacity ? { decided_as: capacity, decided_by_name: actor.fullName || "HRD" } : {}),
      updated_at: now,
    })
    .eq("id", input.overtime_id)
    .eq("status", "pending")
    .select(`
      *,
      employee:employees!employee_id ( id, full_name, nip ),
      decider:employees!decided_by ( id, full_name )
    `)
    .single();
  // 0 baris ter-update = sudah diputuskan proses lain (guard status pending)
  if (error?.code === "PGRST116") throw ApiError.conflict("Pengajuan sudah diproses oleh orang lain");
  unwrap({ data, error });

  if (capacity) {
    await recordAuditAfterCommit({
      actor: { id: actor.userId, name: actor.fullName ?? null },
      action: input.action === "approve" ? "overtime.approve" : "overtime.reject",
      entity: "hris.overtime_requests",
      entityId: input.overtime_id,
      entityLabel: `${overtime.employee?.full_name ?? ""} · ${overtime.date}`,
      before: { status: "pending" },
      after: { status, decided_as: capacity, rejection_reason: rejection },
    });
  }
  return { data, message: DECISION_MESSAGE[status] };
```

`frontend/src/lib/hris/leave-wa.ts`:
- baris 58: ganti `"Mohon ditinjau di dashboard Arkiv OS (Kehadiran & Cuti → Cuti & Izin)."` dengan `"Mohon ditinjau di dashboard Arkiv OS (Area Karyawan → Persetujuan Tim)."`
- baris 73: ganti `JOIN hris.employees a ON a.id = e.reporting_to AND a.is_active = true` dengan `JOIN hris.employees a ON a.id = e.reporting_to AND a.is_active = true AND a.id <> e.id`

`frontend/src/app/api/hris/employees/[id]/route.ts`:
- ganti `import { requireIamMenuPrefix } from "@/lib/api/auth";` dengan `import { ApiError, requireIamMenuPrefix } from "@/lib/api/auth";` dan tambahkan `import { isSelfManager, SELF_MANAGER_ERROR } from "@/lib/hris/reporting-to";`
- di `PUT`, setelah `const body = await readJson(…);` tambahkan:

```ts
  if (isSelfManager(id, body.reporting_to)) throw ApiError.badRequest(SELF_MANAGER_ERROR);
```

`frontend/src/app/api/users/[id]/route.ts`:
- tambahkan `import { isSelfManager, SELF_MANAGER_ERROR } from "@/lib/hris/reporting-to";`
- di `PUT`, setelah `const body = await validateBody(request, updateUserEmployeeSchema);` tambahkan:

```ts
  if (isSelfManager(id, body.reporting_to)) throw ApiError.badRequest(SELF_MANAGER_ERROR);
```

- [ ] **Step 4: Jalankan test, tsc, eslint**

Run: `cd frontend && npx vitest run src/lib/hris src/lib/audit src/lib/backend-routes.test.ts`
Expected: PASS.

Run: `cd frontend && npx tsc --noEmit -p . && npx eslint src/lib/hris/overtime-rules.ts src/lib/hris/reporting-to.ts src/lib/hris/leaves-repo.ts src/lib/hris/overtime-repo.ts src/lib/hris/leave-wa.ts src/lib/hris/workforce-auth.ts src/lib/audit/index.ts "src/app/api/hris/employees/[id]/route.ts" "src/app/api/users/[id]/route.ts" src/features/hris/overtime/types.ts`
Expected: tanpa output error. (Bila `body.reporting_to` bukan properti skema update, tsc menunjuk barisnya: periksa nama kolom di `lib/hris/employee-update-schema.ts`/`lib/users/schemas.ts`, keduanya memakai `reporting_to`.)

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/hris/overtime-rules.ts frontend/src/lib/hris/overtime-rules.test.ts frontend/src/lib/hris/reporting-to.ts frontend/src/lib/hris/reporting-to.test.ts frontend/src/lib/hris/workforce-auth.ts frontend/src/lib/audit/index.ts frontend/src/lib/hris/leaves-repo.ts frontend/src/lib/hris/overtime-repo.ts frontend/src/lib/hris/leave-wa.ts "frontend/src/app/api/hris/employees/[id]/route.ts" "frontend/src/app/api/users/[id]/route.ts" frontend/src/features/hris/overtime/types.ts
git commit -m "feat(hris): paritas fallback Next untuk approval atasan, lembur sebelum dimulai, atasan bukan diri sendiri (EPIC-063 Task 7)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---
### Task 8: Halaman ESS "Persetujuan Tim"

**Files:**
- Create: `frontend/src/features/hris/business-trips/components/team-trip-approvals.tsx`
- Modify: `frontend/src/features/hris/business-trips/components/perjalanan-dinas-page.tsx` (pakai `TeamTripApprovals`)
- Modify: `frontend/src/features/hris/business-trips/index.ts`
- Create: `frontend/src/features/hris/team-approvals/{types.ts,api.ts,query-keys.ts,queries.ts,mutations.ts,index.ts}`
- Create: `frontend/src/features/hris/team-approvals/components/persetujuan-tim-page.tsx`
- Create: `frontend/src/app/dashboard/(dashboard)/me/persetujuan-tim/page.tsx`
- Test: `frontend/src/features/hris/team-approvals/components/persetujuan-tim-page.test.tsx`; regresi `frontend/src/features/hris/business-trips/components/perjalanan-dinas-page.test.tsx`

**Interfaces:**
- Consumes: `GET /api/hris/me/team-approvals` (Task 5), `POST /api/hris/leaves/approve`, `POST /api/hris/overtime/decide` (Task 2–3), `useBusinessTrips`, `useTripDecision` (EPIC-062), `useEssMe`, `useTeamLoanApprovals` (`features/hris/ess/queries.ts`), `overtimeDayTypeLabel`, `OvertimeDayType` (Task 7), `leaveTypeLabel`, `formatDate`, `hhmm`.
- Produces:
  - `export function TeamTripApprovals(): JSX.Element | null` (dari `@/features/hris/business-trips`)
  - `export interface TeamApprovals { leaves: TeamLeaveRequest[]; overtime: TeamOvertimeRequest[] }`, `type TeamRequestKind = "leave" | "overtime"`, `type TeamDecision = "approve" | "reject"`
  - `export const teamApprovalsQueryKeys = { all: ["hris", "team-approvals"] }`
  - `export function useTeamApprovals()`, `export function useDecideTeamRequest()` (mutasi `{kind, id, action, rejectionReason?}`; invalidasi antrean + `["hris", "nav-badges"]`)
  - `export function EssPersetujuanTimPage()`; halaman `/dashboard/me/persetujuan-tim`

- [ ] **Step 1: Tulis test yang gagal**

`frontend/src/features/hris/team-approvals/components/persetujuan-tim-page.test.tsx`:

```tsx
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { TeamApprovals, TeamLeaveRequest, TeamOvertimeRequest } from "../types";
import { EssPersetujuanTimPage } from "./persetujuan-tim-page";

const decide = vi.fn();
const toastError = vi.fn();
const toastSuccess = vi.fn();
let queue: TeamApprovals = { leaves: [], overtime: [] };
let loans: { id: string }[] = [];
let trips: { id: string }[] = [];

vi.mock("sonner", () => ({
  toast: { error: (m: string) => toastError(m), success: (m: string) => toastSuccess(m) },
}));
vi.mock("@/features/hris/ess/queries", () => ({
  useEssMe: () => ({ data: { employee: { id: "boss" } }, isLoading: false }),
  useTeamLoanApprovals: () => ({ data: loans }),
}));
vi.mock("@/features/hris/business-trips", () => ({
  TeamTripApprovals: () => (trips.length > 0 ? <p>Antrean Perjalanan Dinas</p> : null),
  useBusinessTrips: () => ({ data: trips }),
}));
vi.mock("../queries", () => ({ useTeamApprovals: () => ({ data: queue, isLoading: false }) }));
vi.mock("../mutations", () => ({
  useDecideTeamRequest: () => ({ mutate: decide, isPending: false, variables: undefined }),
}));

const leave: TeamLeaveRequest = {
  id: "l1", employee_id: "e1", leave_type: "annual", start_date: "2031-03-04", end_date: "2031-03-05",
  total_days: "2.00", reason: "Acara keluarga", attachment_url: null, created_at: "2031-03-01T00:00:00Z",
  employee: { id: "e1", full_name: "Anggota A", nip: "N01" },
};
const overtime: TeamOvertimeRequest = {
  id: "o1", employee_id: "e2", date: "2026-10-06", start_time: "17:00:00", end_time: "19:30:00", hours: "2.50",
  day_type: "rest_day", reason: "Stock opname", created_at: "2026-10-05T00:00:00Z",
  employee: { id: "e2", full_name: "Anggota B", nip: "N02" },
};

afterEach(() => {
  cleanup();
  decide.mockReset();
  toastError.mockReset();
  toastSuccess.mockReset();
  queue = { leaves: [], overtime: [] };
  loans = [];
  trips = [];
});

describe("EssPersetujuanTimPage", () => {
  it("menampilkan cuti & lembur tim dan menyetujui cuti langsung", () => {
    queue = { leaves: [leave], overtime: [overtime] };
    decide.mockImplementation((_v, opts) => opts.onSuccess());
    render(<EssPersetujuanTimPage />);
    expect(screen.getByText(/Anggota A · Cuti Tahunan/)).toBeTruthy();
    expect(screen.getByText(/17:00–19:30 \(2,5 jam\) · Weekend \/ Libur/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Setujui cuti Anggota A" }));
    expect(decide).toHaveBeenCalledWith(
      { kind: "leave", id: "l1", action: "approve", rejectionReason: undefined },
      expect.anything()
    );
    expect(toastSuccess).toHaveBeenCalledWith("Cuti disetujui");
  });

  it("menolak lembur wajib alasan lewat dialog", () => {
    queue = { leaves: [], overtime: [overtime] };
    render(<EssPersetujuanTimPage />);
    fireEvent.click(screen.getByRole("button", { name: "Tolak lembur Anggota B" }));
    fireEvent.click(screen.getByRole("button", { name: "Tolak pengajuan" }));
    expect(toastError).toHaveBeenCalledWith("Alasan penolakan wajib diisi");
    expect(decide).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText("Alasan penolakan"), { target: { value: "  Tidak ada stok masuk " } });
    fireEvent.click(screen.getByRole("button", { name: "Tolak pengajuan" }));
    expect(decide).toHaveBeenCalledWith(
      { kind: "overtime", id: "o1", action: "reject", rejectionReason: "Tidak ada stok masuk" },
      expect.anything()
    );
  });

  it("menampilkan pesan error server apa adanya", () => {
    queue = { leaves: [leave], overtime: [] };
    decide.mockImplementation((_v, opts) => opts.onError(new Error("Tidak bisa memutuskan pengajuan cuti sendiri")));
    render(<EssPersetujuanTimPage />);
    fireEvent.click(screen.getByRole("button", { name: "Setujui cuti Anggota A" }));
    expect(toastError).toHaveBeenCalledWith("Tidak bisa memutuskan pengajuan cuti sendiri");
  });

  it("Perjalanan Dinas memakai antrean bersama, kasbon ditautkan ke Pinjaman", () => {
    loans = [{ id: "k1" }, { id: "k2" }];
    trips = [{ id: "t1" }];
    render(<EssPersetujuanTimPage />);
    expect(screen.getByText("Antrean Perjalanan Dinas")).toBeTruthy();
    const link = screen.getByRole("link", { name: /2 kasbon tim menunggu persetujuan Anda/ });
    expect(link.getAttribute("href")).toBe("/dashboard/me/pinjaman");
    expect(screen.queryByText("Tidak ada pengajuan tim yang menunggu persetujuan Anda.")).toBeNull();
  });

  it("menampilkan keadaan kosong", () => {
    render(<EssPersetujuanTimPage />);
    expect(screen.getByText("Tidak ada pengajuan tim yang menunggu persetujuan Anda.")).toBeTruthy();
  });
});
```

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd frontend && npx vitest run src/features/hris/team-approvals`
Expected: FAIL — `Failed to resolve import "./persetujuan-tim-page"`.

- [ ] **Step 3: Tulis implementasi**

`frontend/src/features/hris/business-trips/components/team-trip-approvals.tsx`:

```tsx
"use client";

import { UserGroupIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { tripRangeLabel } from "@/lib/hris/business-trips";
import { useBusinessTrips } from "../queries";
import { useTripDecision } from "../use-trip-decision";

/**
 * Antrean Perjalanan Dinas anggota tim untuk atasan langsung (EPIC-062),
 * dipakai halaman Perjalanan Dinas dan Persetujuan Tim (EPIC-063 R7).
 * Tidak dirender bila kosong.
 */
export function TeamTripApprovals() {
  const team = useBusinessTrips({ scope: "approvals", status: "pending" }).data ?? [];
  const { request, busyId, dialog } = useTripDecision();
  if (team.length === 0) return null;
  return (
    <div className="rounded-xl border border-sky-200 bg-sky-50/70 p-5 shadow-sm">
      <h3 className="flex items-center gap-2 text-sm font-semibold text-sky-800">
        <UserGroupIcon className="h-4 w-4" /> Persetujuan Perjalanan Dinas Tim
      </h3>
      <ul className="mt-3 divide-y divide-sky-100">
        {team.map((trip) => (
          <li key={trip.id} className="flex flex-wrap items-center justify-between gap-3 py-2.5">
            <div className="min-w-0">
              <p className="text-sm font-medium text-gray-900">
                {trip.employee?.full_name ?? "—"} · {tripRangeLabel(trip.start_date, trip.end_date)}
              </p>
              <p className="truncate text-xs text-gray-600">
                {trip.destination} — {trip.purpose}
              </p>
            </div>
            <div className="flex shrink-0 gap-2">
              <Button size="sm" disabled={busyId === trip.id} onClick={() => request(trip, "approve")}>
                Setujui
              </Button>
              <Button size="sm" variant="outline" disabled={busyId === trip.id} onClick={() => request(trip, "reject")}>
                Tolak
              </Button>
            </div>
          </li>
        ))}
      </ul>
      {dialog}
    </div>
  );
}
```

`frontend/src/features/hris/business-trips/components/perjalanan-dinas-page.tsx`:
- ganti import `import { BriefcaseIcon, UserGroupIcon } from "@heroicons/react/24/outline";` dengan `import { BriefcaseIcon } from "@heroicons/react/24/outline";` dan tambahkan `import { TeamTripApprovals } from "./team-trip-approvals";`
- hapus baris `const team = useBusinessTrips({ scope: "approvals", status: "pending" }).data ?? [];`
- ganti seluruh blok `{team.length > 0 && ( … )}` (div `border-sky-200` sampai penutupnya) dengan `<TeamTripApprovals />`.

`frontend/src/features/hris/business-trips/index.ts`, tambahkan:

```ts
export { TeamTripApprovals } from "./components/team-trip-approvals";
```

`frontend/src/features/hris/team-approvals/types.ts`:

```ts
import type { OvertimeDayType } from "@/lib/hris/overtime-rules";

/** GET /api/hris/me/team-approvals (EPIC-063): pengajuan pending bawahan langsung. */
export interface TeamMemberRef {
  id: string;
  full_name: string;
  nip: string | null;
}

export interface TeamLeaveRequest {
  id: string;
  employee_id: string;
  leave_type: string;
  start_date: string;
  end_date: string;
  total_days: string | number;
  reason: string | null;
  attachment_url: string | null;
  created_at: string;
  employee: TeamMemberRef;
}

export interface TeamOvertimeRequest {
  id: string;
  employee_id: string;
  date: string;
  start_time: string;
  end_time: string;
  hours: string | number;
  day_type: OvertimeDayType;
  reason: string | null;
  created_at: string;
  employee: TeamMemberRef;
}

export interface TeamApprovals {
  leaves: TeamLeaveRequest[];
  overtime: TeamOvertimeRequest[];
}

export type TeamRequestKind = "leave" | "overtime";
export type TeamDecision = "approve" | "reject";
```

`frontend/src/features/hris/team-approvals/api.ts`:

```ts
import { apiGet, apiPost } from "@/lib/api-client";
import type { TeamApprovals, TeamDecision, TeamRequestKind } from "./types";

const EMPTY: TeamApprovals = { leaves: [], overtime: [] };

export const fetchTeamApprovals = () =>
  apiGet<{ data: TeamApprovals }>("/api/hris/me/team-approvals").then((res) => res.data ?? EMPTY);

/** Keputusan memakai endpoint yang sama dengan HRIS; server menentukan atasan/HR. */
export function decideTeamRequest(kind: TeamRequestKind, id: string, action: TeamDecision, rejectionReason?: string) {
  if (kind === "leave") {
    return apiPost<{ message?: string }>("/api/hris/leaves/approve", {
      leave_id: id,
      action,
      rejection_reason: rejectionReason,
    });
  }
  return apiPost<{ message?: string }>("/api/hris/overtime/decide", {
    overtime_id: id,
    action,
    rejection_reason: rejectionReason,
  });
}
```

`frontend/src/features/hris/team-approvals/query-keys.ts`:

```ts
export const teamApprovalsQueryKeys = {
  all: ["hris", "team-approvals"] as const,
};
```

`frontend/src/features/hris/team-approvals/queries.ts`:

```ts
"use client";

import { useQuery } from "@tanstack/react-query";
import { fetchTeamApprovals } from "./api";
import { teamApprovalsQueryKeys } from "./query-keys";

export const useTeamApprovals = () =>
  useQuery({ queryKey: teamApprovalsQueryKeys.all, queryFn: fetchTeamApprovals });
```

`frontend/src/features/hris/team-approvals/mutations.ts`:

```ts
"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { decideTeamRequest } from "./api";
import { teamApprovalsQueryKeys } from "./query-keys";
import type { TeamDecision, TeamRequestKind } from "./types";

/** Setujui/tolak; segarkan antrean dan badge menu (nav-badges). */
export function useDecideTeamRequest() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { kind: TeamRequestKind; id: string; action: TeamDecision; rejectionReason?: string }) =>
      decideTeamRequest(v.kind, v.id, v.action, v.rejectionReason),
    retry: false,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: teamApprovalsQueryKeys.all });
      void qc.invalidateQueries({ queryKey: ["hris", "nav-badges"] });
    },
  });
}
```

`frontend/src/features/hris/team-approvals/index.ts`:

```ts
export * from "./types";
export { EssPersetujuanTimPage } from "./components/persetujuan-tim-page";
```

`frontend/src/features/hris/team-approvals/components/persetujuan-tim-page.tsx`:

```tsx
"use client";

import Link from "next/link";
import { useState } from "react";
import { toast } from "sonner";
import { BanknotesIcon, CalendarDaysIcon, ClockIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { Textarea } from "@/components/ui/textarea";
import { formatDate } from "@/lib/format";
import { leaveTypeLabel } from "@/lib/hris/leave-types";
import { overtimeDayTypeLabel } from "@/lib/hris/overtime-rules";
import { hhmm } from "@/lib/hris/shifts-view";
import { TeamTripApprovals, useBusinessTrips } from "@/features/hris/business-trips";
import { EssLoading, EssNotLinked } from "@/features/hris/ess/components/ess-states";
import { useEssMe, useTeamLoanApprovals } from "@/features/hris/ess/queries";
import { useDecideTeamRequest } from "../mutations";
import { useTeamApprovals } from "../queries";
import type { TeamDecision, TeamRequestKind } from "../types";

const DONE: Record<TeamRequestKind, Record<TeamDecision, string>> = {
  leave: { approve: "Cuti disetujui", reject: "Cuti ditolak" },
  overtime: { approve: "Lembur disetujui", reject: "Lembur ditolak" },
};

const NOUN: Record<TeamRequestKind, string> = { leave: "cuti", overtime: "lembur" };

interface Rejecting {
  kind: TeamRequestKind;
  id: string;
  name: string;
}

/**
 * ESS → Persetujuan Tim (/dashboard/me/persetujuan-tim, EPIC-063): antrean
 * atasan langsung untuk cuti, lembur, dan Perjalanan Dinas bawahan langsung.
 * Kasbon tetap diputus di Pinjaman (dua tahap). Anggota tim (reporting_to)
 * dan larangan memutus pengajuan sendiri ditegakkan server; HR tetap bisa
 * memutus sebagai cadangan.
 */
export function EssPersetujuanTimPage() {
  const { data: me, isLoading } = useEssMe();
  const queue = useTeamApprovals();
  const trips = useBusinessTrips({ scope: "approvals", status: "pending" }).data ?? [];
  const loans = useTeamLoanApprovals().data ?? [];
  const decide = useDecideTeamRequest();
  const [rejecting, setRejecting] = useState<Rejecting | null>(null);
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const busyId = decide.isPending ? decide.variables?.id : null;

  function run(kind: TeamRequestKind, id: string, action: TeamDecision, rejectionReason?: string) {
    decide.mutate(
      { kind, id, action, rejectionReason },
      {
        onSuccess: () => toast.success(DONE[kind][action]),
        onError: (e) => toast.error(e.message || "Gagal memproses"),
      }
    );
  }

  function askReject(kind: TeamRequestKind, id: string, name: string) {
    setReason("");
    setRejecting({ kind, id, name });
    setOpen(true);
  }

  function confirmReject() {
    if (!rejecting) return;
    const text = reason.trim();
    if (!text) {
      toast.error("Alasan penolakan wajib diisi");
      return;
    }
    setOpen(false);
    run(rejecting.kind, rejecting.id, "reject", text);
  }

  function actions(kind: TeamRequestKind, id: string, name: string) {
    return (
      <div className="flex shrink-0 gap-2">
        <Button size="sm" aria-label={`Setujui ${NOUN[kind]} ${name}`} disabled={busyId === id}
          onClick={() => run(kind, id, "approve")}>
          Setujui
        </Button>
        <Button size="sm" variant="outline" aria-label={`Tolak ${NOUN[kind]} ${name}`} disabled={busyId === id}
          onClick={() => askReject(kind, id, name)}>
          Tolak
        </Button>
      </div>
    );
  }

  if (isLoading) return <EssLoading />;
  if (!me?.employee) return <EssNotLinked feature="Persetujuan Tim" />;

  const leaves = queue.data?.leaves ?? [];
  const overtime = queue.data?.overtime ?? [];
  const empty = leaves.length + overtime.length + trips.length + loans.length === 0;

  return (
    <div className="space-y-6">
      <div className="border-b border-gray-200/70 pb-4">
        <h1 className="text-2xl font-bold text-gray-900">Persetujuan Tim</h1>
        <p className="text-sm text-gray-500">
          Pengajuan anggota tim yang atasan langsungnya Anda. HR tetap bisa memutus sebagai cadangan.
        </p>
      </div>

      {empty && !queue.isLoading && (
        <div className="rounded-xl border border-gray-200/70 bg-white p-6 text-center text-sm text-gray-500 shadow-sm">
          Tidak ada pengajuan tim yang menunggu persetujuan Anda.
        </div>
      )}

      {leaves.length > 0 && (
        <section className="rounded-xl border border-gray-200/70 bg-white p-5 shadow-sm">
          <h3 className="flex items-center gap-2 text-sm font-semibold text-gray-800">
            <CalendarDaysIcon className="h-4 w-4 text-pink-500" /> Cuti & Izin ({leaves.length})
          </h3>
          <ul className="mt-3 divide-y divide-gray-100">
            {leaves.map((row) => (
              <li key={row.id} className="flex flex-wrap items-center justify-between gap-3 py-2.5">
                <div className="min-w-0">
                  <p className="text-sm font-medium text-gray-900">
                    {row.employee.full_name} · {leaveTypeLabel(row.leave_type)}
                  </p>
                  <p className="truncate text-xs text-gray-600">
                    {formatDate(row.start_date, "—")}
                    {row.end_date !== row.start_date ? ` s.d. ${formatDate(row.end_date, "—")}` : ""} (
                    {Number(row.total_days)} hari) · {row.reason || "—"}
                  </p>
                </div>
                {actions("leave", row.id, row.employee.full_name)}
              </li>
            ))}
          </ul>
        </section>
      )}

      {overtime.length > 0 && (
        <section className="rounded-xl border border-gray-200/70 bg-white p-5 shadow-sm">
          <h3 className="flex items-center gap-2 text-sm font-semibold text-gray-800">
            <ClockIcon className="h-4 w-4 text-pink-500" /> Lembur ({overtime.length})
          </h3>
          <ul className="mt-3 divide-y divide-gray-100">
            {overtime.map((row) => (
              <li key={row.id} className="flex flex-wrap items-center justify-between gap-3 py-2.5">
                <div className="min-w-0">
                  <p className="text-sm font-medium text-gray-900">
                    {row.employee.full_name} · {formatDate(row.date, "—")}
                  </p>
                  <p className="truncate text-xs text-gray-600">
                    {hhmm(row.start_time)}–{hhmm(row.end_time)} ({String(Number(row.hours)).replace(".", ",")} jam) ·{" "}
                    {overtimeDayTypeLabel(row.day_type)} · {row.reason || "—"}
                  </p>
                </div>
                {actions("overtime", row.id, row.employee.full_name)}
              </li>
            ))}
          </ul>
        </section>
      )}

      <TeamTripApprovals />

      {loans.length > 0 && (
        <Link
          href="/dashboard/me/pinjaman"
          className="flex items-center justify-between gap-3 rounded-xl border border-amber-200 bg-amber-50/70 p-4 text-sm text-amber-900 shadow-sm hover:bg-amber-50"
        >
          <span className="flex items-center gap-2">
            <BanknotesIcon className="h-4 w-4" /> {loans.length} kasbon tim menunggu persetujuan Anda
          </span>
          <span className="font-medium">Buka Pinjaman →</span>
        </Link>
      )}

      <ConfirmDialog
        open={open}
        onOpenChange={setOpen}
        title={`Tolak ${rejecting ? NOUN[rejecting.kind] : "pengajuan"} ${rejecting?.name ?? ""}?`}
        confirmLabel="Tolak pengajuan"
        cancelLabel="Kembali"
        onConfirm={confirmReject}
      >
        <div className="space-y-1.5">
          <label htmlFor="team-rejection-reason" className="block text-xs font-medium text-gray-600">
            Alasan penolakan
          </label>
          <Textarea
            id="team-rejection-reason"
            rows={3}
            maxLength={500}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
          />
        </div>
      </ConfirmDialog>
    </div>
  );
}
```

`frontend/src/app/dashboard/(dashboard)/me/persetujuan-tim/page.tsx`:

```tsx
import { requireUser } from "@/lib/auth/require-user";
import { EssPersetujuanTimPage } from "@/features/hris/team-approvals";

// ESS → Persetujuan Tim (EPIC-063): antrean atasan langsung; datanya per akun dari API.
export default async function PersetujuanTimPage() {
  await requireUser();
  return <EssPersetujuanTimPage />;
}
```

- [ ] **Step 4: Jalankan test, tsc, eslint**

Run: `cd frontend && npx vitest run src/features/hris/team-approvals src/features/hris/business-trips`
Expected: PASS (termasuk `perjalanan-dinas-page.test.tsx` lama tanpa perubahan).

Run: `cd frontend && npx tsc --noEmit -p . && npx eslint src/features/hris/team-approvals src/features/hris/business-trips "src/app/dashboard/(dashboard)/me/persetujuan-tim"`
Expected: tanpa error.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/features/hris/team-approvals frontend/src/features/hris/business-trips "frontend/src/app/dashboard/(dashboard)/me/persetujuan-tim"
git commit -m "feat(ess): halaman Persetujuan Tim untuk cuti, lembur, Perjalanan Dinas, dan tautan kasbon (EPIC-063 Task 8)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 9: Form lembur ESS (jenis + jam lewat) dan koreksi lembur di HRIS

**Files:**
- Modify: `frontend/src/features/hris/ess/types.ts` (`EssOvertimeRow.day_type`, `EssOvertimeForm.day_type`, `EssOvertimeDayTypeHint`)
- Modify: `frontend/src/features/hris/ess/api.ts`, `query-keys.ts`, `queries.ts` (saran jenis)
- Modify: `frontend/src/features/hris/ess/components/lembur-page.tsx`
- Modify: `frontend/src/features/hris/overtime/types.ts`, `api.ts`, `mutations.ts`
- Create: `frontend/src/features/hris/overtime/components/overtime-correction-dialog.tsx`
- Modify: `frontend/src/features/hris/overtime/components/overtime-table.tsx`, `overtime-page.tsx`
- Modify: `frontend/src/app/dashboard/(dashboard)/hris/overtime/page.tsx`
- Test: `frontend/src/features/hris/ess/components/lembur-page.test.tsx`, `frontend/src/features/hris/overtime/components/overtime-correction-dialog.test.tsx`

**Interfaces:**
- Consumes: `GET /api/hris/overtime/day-type` (Task 3), `PATCH /api/hris/overtime/{id}` (Task 4), `OVERTIME_DAY_TYPE_OPTIONS`, `OVERTIME_TOO_LATE`, `overtimeStartsInPast`, `overtimeDayTypeLabel`, `OvertimeDayType` (Task 7), `userHasIamAction` (`lib/iam/has-menu.ts`), `todayWib` (`lib/dates.ts`), `hhmm` (`lib/hris/shifts-view.ts`).
- Produces:
  - `export interface EssOvertimeDayTypeHint { date: string; day_type: OvertimeDayType | null; label: string | null }`, `fetchOvertimeDayType(date)`, `useOvertimeDayType(date)`
  - `export interface OvertimeCorrection { id: string; date: string; start_time: string; end_time: string; day_type: OvertimeDayType; reason: string }`, `correctOvertime`, `useCorrectOvertime()`
  - `OvertimeRow` + `decided_as?`, `decided_by_name?`, `corrected_at?`, `corrected_by_name?`
  - `export function OvertimeCorrectionDialog(props: { row: OvertimeRow | null; onClose: () => void })`
  - `OvertimePage({ canCorrect?: boolean })`, `OvertimeTable` props `canCorrect`, `onCorrect`

- [ ] **Step 1: Tulis test yang gagal**

`frontend/src/features/hris/ess/components/lembur-page.test.tsx`:

```tsx
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { OVERTIME_TOO_LATE } from "@/lib/hris/overtime-rules";
import type { EssOvertimeDayTypeHint } from "../types";
import { EssLemburPage } from "./lembur-page";

const submit = vi.fn();
const toastError = vi.fn();
let hint: EssOvertimeDayTypeHint | null = null;

vi.mock("sonner", () => ({ toast: { error: (m: string) => toastError(m), success: vi.fn() } }));
vi.mock("../queries", () => ({
  useEssMe: () => ({ data: { employee: { id: "me1" } }, isLoading: false }),
  useMyOvertime: () => ({ data: [] }),
  useOvertimeDayType: (date: string) => ({ data: date ? (hint ?? undefined) : undefined }),
}));
vi.mock("../mutations", () => ({
  useSubmitOvertime: () => ({ mutate: submit, isPending: false }),
  useDecideOvertime: () => ({ mutate: vi.fn(), isPending: false, variables: undefined }),
}));

function fill(date: string) {
  fireEvent.click(screen.getByRole("button", { name: /Ajukan Lembur/ }));
  fireEvent.change(screen.getByLabelText("Tanggal"), { target: { value: date } });
  fireEvent.change(screen.getByLabelText("Jam Mulai"), { target: { value: "17:00" } });
  fireEvent.change(screen.getByLabelText("Jam Selesai"), { target: { value: "19:00" } });
  fireEvent.change(screen.getByLabelText(/Alasan/), { target: { value: "Closing stok bulanan" } });
}

afterEach(() => {
  cleanup();
  submit.mockReset();
  toastError.mockReset();
  hint = null;
});

describe("EssLemburPage — EPIC-063", () => {
  it("menolak jam mulai yang sudah lewat sebelum mengirim", () => {
    render(<EssLemburPage />);
    fill("2020-01-06");
    fireEvent.click(screen.getByRole("button", { name: "Kirim Pengajuan" }));
    expect(toastError).toHaveBeenCalledWith(OVERTIME_TOO_LATE);
    expect(submit).not.toHaveBeenCalled();
  });

  it("jenis dikunci ke saran jadwal dan ikut terkirim", () => {
    hint = { date: "2099-01-04", day_type: "rest_day", label: "Weekend / Libur" };
    render(<EssLemburPage />);
    fill("2099-01-04");
    const select = screen.getByLabelText("Jenis lembur") as HTMLSelectElement;
    expect(select.disabled).toBe(true);
    expect(select.value).toBe("rest_day");
    expect(screen.getByText("Sesuai jadwal Anda: Weekend / Libur")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Kirim Pengajuan" }));
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({ date: "2099-01-04", start_time: "17:00", day_type: "rest_day" }),
      expect.anything()
    );
  });

  it("tanpa jadwal karyawan memilih sendiri", () => {
    hint = { date: "2099-01-05", day_type: null, label: null };
    render(<EssLemburPage />);
    fill("2099-01-05");
    const select = screen.getByLabelText("Jenis lembur") as HTMLSelectElement;
    expect(select.disabled).toBe(false);
    fireEvent.change(select, { target: { value: "rest_day" } });
    fireEvent.click(screen.getByRole("button", { name: "Kirim Pengajuan" }));
    expect(submit).toHaveBeenCalledWith(expect.objectContaining({ day_type: "rest_day" }), expect.anything());
  });
});
```

`frontend/src/features/hris/overtime/components/overtime-correction-dialog.test.tsx`:

```tsx
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { OvertimeRow } from "../types";
import { OvertimeCorrectionDialog } from "./overtime-correction-dialog";

const correct = vi.fn();
const toastError = vi.fn();
const toastSuccess = vi.fn();

vi.mock("sonner", () => ({
  toast: { error: (m: string) => toastError(m), success: (m: string) => toastSuccess(m) },
}));
vi.mock("../mutations", () => ({ useCorrectOvertime: () => ({ mutate: correct, isPending: false }) }));

const row: OvertimeRow = {
  id: "o1", employee_id: "e1", date: "2026-10-06T00:00:00.000Z", start_time: "17:00:00", end_time: "19:00:00",
  hours: "2.00", source: "employee", status: "approved", reason: "Closing", rejection_reason: null,
  day_type: "regular", employee: { id: "e1", full_name: "Crew Koreksi", nip: "N01" },
};

afterEach(() => {
  cleanup();
  correct.mockReset();
  toastError.mockReset();
  toastSuccess.mockReset();
});

describe("OvertimeCorrectionDialog — AP-04", () => {
  it("alasan koreksi wajib minimal 5 karakter", () => {
    render(<OvertimeCorrectionDialog row={row} onClose={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Alasan koreksi"), { target: { value: " abc " } });
    fireEvent.click(screen.getByRole("button", { name: "Simpan koreksi" }));
    expect(toastError).toHaveBeenCalledWith("Alasan koreksi minimal 5 karakter");
    expect(correct).not.toHaveBeenCalled();
  });

  it("mengirim tanggal, jam, jenis, dan alasan lalu menutup", () => {
    const onClose = vi.fn();
    correct.mockImplementation((_v, opts) => opts.onSuccess({ message: "Lembur dikoreksi" }));
    render(<OvertimeCorrectionDialog row={row} onClose={onClose} />);
    expect((screen.getByLabelText("Tanggal") as HTMLInputElement).value).toBe("2026-10-06");
    fireEvent.change(screen.getByLabelText("Jam Selesai"), { target: { value: "20:30" } });
    fireEvent.change(screen.getByLabelText("Jenis lembur"), { target: { value: "rest_day" } });
    fireEvent.change(screen.getByLabelText("Alasan koreksi"), { target: { value: " Sesuai absensi " } });
    fireEvent.click(screen.getByRole("button", { name: "Simpan koreksi" }));
    expect(correct).toHaveBeenCalledWith(
      { id: "o1", date: "2026-10-06", start_time: "17:00", end_time: "20:30", day_type: "rest_day", reason: "Sesuai absensi" },
      expect.anything()
    );
    expect(toastSuccess).toHaveBeenCalledWith("Lembur dikoreksi");
    expect(onClose).toHaveBeenCalled();
  });
});
```

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd frontend && npx vitest run src/features/hris/ess/components/lembur-page.test.tsx src/features/hris/overtime/components/overtime-correction-dialog.test.tsx`
Expected: FAIL — `Unable to find a label with the text of: Tanggal` (label belum terhubung) dan `Failed to resolve import "./overtime-correction-dialog"`.

- [ ] **Step 3: Tulis implementasi — ESS**

`frontend/src/features/hris/ess/types.ts`:
- tambahkan import di atas: `import type { OvertimeDayType } from "@/lib/hris/overtime-rules";`
- di `EssOvertimeRow` setelah `rejection_reason`: `day_type?: OvertimeDayType;`
- di `EssOvertimeForm` setelah `reason: string;`: `day_type: OvertimeDayType;`
- tambahkan:

```ts
/** GET /api/hris/overtime/day-type (EPIC-063): saran jenis dari jadwal; null = jadwal tidak diketahui. */
export interface EssOvertimeDayTypeHint {
  date: string;
  day_type: OvertimeDayType | null;
  label: string | null;
}
```

`frontend/src/features/hris/ess/api.ts` — tambahkan `EssOvertimeDayTypeHint` ke import tipe, lalu di bawah `submitOvertime`:

```ts
export const fetchOvertimeDayType = (date: string) =>
  apiGet<{ data: EssOvertimeDayTypeHint }>(`/api/hris/overtime/day-type?date=${encodeURIComponent(date)}`).then(
    (res) => res.data
  );
```

`frontend/src/features/hris/ess/query-keys.ts`, tambahkan entri:

```ts
  overtimeDayType: (date: string) => ["hris", "ess", "overtime-day-type", date] as const,
```

`frontend/src/features/hris/ess/queries.ts` — tambahkan `fetchOvertimeDayType` ke import, lalu:

```ts
export const useOvertimeDayType = (date: string) =>
  useQuery({
    queryKey: essQueryKeys.overtimeDayType(date),
    queryFn: () => fetchOvertimeDayType(date),
    enabled: /^\d{4}-\d{2}-\d{2}$/.test(date),
  });
```

`frontend/src/features/hris/ess/components/lembur-page.tsx`:

1. Import: tambahkan

```tsx
import { todayWib } from "@/lib/dates";
import {
  OVERTIME_DAY_TYPE_OPTIONS,
  OVERTIME_TOO_LATE,
  overtimeDayTypeLabel,
  overtimeStartsInPast,
  type OvertimeDayType,
} from "@/lib/hris/overtime-rules";
```

dan ganti `import { useEssMe, useMyOvertime } from "../queries";` dengan `import { useEssMe, useMyOvertime, useOvertimeDayType } from "../queries";`

2. Ganti `const EMPTY_FORM: EssOvertimeForm = { date: "", start_time: "", end_time: "", reason: "" };` dengan:

```tsx
const EMPTY_FORM: EssOvertimeForm = { date: "", start_time: "", end_time: "", reason: "", day_type: "regular" };
```

3. Setelah `const decidingId = …;` tambahkan:

```tsx
  // K-13 (EPIC-063 R5): jenis dikunci ke saran jadwal bila server mengetahuinya.
  const hint = useOvertimeDayType(form.date).data ?? null;
  const lockedType = hint?.day_type ?? null;
  const dayType: OvertimeDayType = lockedType ?? form.day_type;
```

4. Di `handleSubmit`, setelah cek alasan (`if (form.reason.trim().length < 5) { … }`) tambahkan:

```tsx
    if (overtimeStartsInPast(form.date, form.start_time)) {
      toast.error(OVERTIME_TOO_LATE);
      return;
    }
```

dan ganti `submitMutation.mutate(form, {` dengan `submitMutation.mutate({ ...form, day_type: dayType }, {`.

5. Di daftar riwayat, setelah blok `{row.source === "company" && ( … Penugasan … )}` tambahkan:

```tsx
                      {row.day_type === "rest_day" && (
                        <span className="ml-2 rounded-full bg-violet-100 px-2 py-0.5 text-xs font-medium text-violet-700">
                          {overtimeDayTypeLabel(row.day_type)}
                        </span>
                      )}
```

6. Ganti isi `<div className="space-y-3">` di dialog (dari field Tanggal sampai paragraf info biru) dengan:

```tsx
          <div className="space-y-3">
            <div>
              <label htmlFor="ot-date" className="mb-1 block text-xs font-medium text-gray-600">Tanggal</label>
              <Input
                id="ot-date"
                type="date"
                min={todayWib()}
                value={form.date}
                onChange={(e) => setForm((f) => ({ ...f, date: e.target.value }))}
              />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label htmlFor="ot-start" className="mb-1 block text-xs font-medium text-gray-600">Jam Mulai</label>
                <Input
                  id="ot-start"
                  type="time"
                  value={form.start_time}
                  onChange={(e) => setForm((f) => ({ ...f, start_time: e.target.value }))}
                />
              </div>
              <div>
                <label htmlFor="ot-end" className="mb-1 block text-xs font-medium text-gray-600">Jam Selesai</label>
                <Input
                  id="ot-end"
                  type="time"
                  value={form.end_time}
                  onChange={(e) => setForm((f) => ({ ...f, end_time: e.target.value }))}
                />
              </div>
            </div>
            <div>
              <label htmlFor="ot-day-type" className="mb-1 block text-xs font-medium text-gray-600">Jenis lembur</label>
              <select
                id="ot-day-type"
                value={dayType}
                disabled={lockedType !== null}
                onChange={(e) => setForm((f) => ({ ...f, day_type: e.target.value as OvertimeDayType }))}
                className="h-9 w-full rounded-md border border-gray-200 bg-white px-2 text-sm disabled:bg-gray-50"
              >
                {OVERTIME_DAY_TYPE_OPTIONS.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
              {form.date && (
                <p className="mt-1 text-xs text-gray-500">
                  {lockedType
                    ? `Sesuai jadwal Anda: ${overtimeDayTypeLabel(lockedType)}`
                    : "Jadwal tanggal ini belum ada — pilih sesuai hari lemburnya"}
                </p>
              )}
            </div>
            <div>
              <label htmlFor="ot-reason" className="mb-1 block text-xs font-medium text-gray-600">
                Alasan / pekerjaan yang dilembur (min. 5 karakter)
              </label>
              <Input
                id="ot-reason"
                placeholder="cth. closing stok bulanan"
                value={form.reason}
                onChange={(e) => setForm((f) => ({ ...f, reason: e.target.value }))}
              />
            </div>
            <p className="rounded-lg bg-sky-50 px-3 py-2 text-xs text-sky-700">
              Lembur wajib diajukan sebelum jam mulai. Lembur yang disetujui dibayar sesuai jam yang
              terealisasi (wajib absen clock-in/out pada hari tersebut).
            </p>
          </div>
```

- [ ] **Step 4: Tulis implementasi — HRIS**

`frontend/src/features/hris/overtime/types.ts`, di `OvertimeRow` setelah `day_type?: OvertimeDayType;` tambahkan:

```ts
  /** EPIC-063: kapasitas pemutus (atasan langsung / HR cadangan) dan koreksi AdminHRIS terakhir. */
  decided_as?: "manager" | "hr" | null;
  decided_by_name?: string | null;
  corrected_at?: string | null;
  corrected_by_name?: string | null;
```

dan di akhir file:

```ts
/** PATCH /api/hris/overtime/{id} (AP-04). */
export interface OvertimeCorrection {
  id: string;
  date: string;
  start_time: string;
  end_time: string;
  day_type: OvertimeDayType;
  reason: string;
}
```

`frontend/src/features/hris/overtime/api.ts` — ganti import pertama dengan `import { apiGet, apiPatch, apiPost, buildListUrl } from "@/lib/api-client";`, tambahkan `OvertimeCorrection` ke import tipe, lalu:

```ts
export const correctOvertime = ({ id, ...body }: OvertimeCorrection) =>
  apiPatch<{ message?: string; data: OvertimeRow }>(`/api/hris/overtime/${id}`, body);
```

`frontend/src/features/hris/overtime/mutations.ts` — tambahkan `correctOvertime` ke import, lalu:

```ts
export function useCorrectOvertime() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: correctOvertime,
    retry: false,
    onSuccess: () => qc.invalidateQueries({ queryKey: overtimeQueryKeys.lists() }),
  });
}
```

`frontend/src/features/hris/overtime/components/overtime-correction-dialog.tsx`:

```tsx
"use client";

import { useState } from "react";
import { toast } from "sonner";
import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { OVERTIME_DAY_TYPE_OPTIONS } from "@/lib/hris/overtime-rules";
import { hhmm } from "@/lib/hris/shifts-view";
import { useCorrectOvertime } from "../mutations";
import type { OvertimeDayType, OvertimeRow } from "../types";

interface OvertimeCorrectionDialogProps {
  row: OvertimeRow | null;
  onClose: () => void;
}

/**
 * Koreksi lembur oleh AdminHRIS (MOM AP-04, EPIC-063 R6): tanggal, jam, dan
 * jenis; jam dihitung ulang server, alasan wajib dan tercatat di audit log.
 */
export function OvertimeCorrectionDialog({ row, onClose }: OvertimeCorrectionDialogProps) {
  return (
    <Dialog open={row !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-w-md">{row && <CorrectionForm key={row.id} row={row} onClose={onClose} />}</DialogContent>
    </Dialog>
  );
}

function CorrectionForm({ row, onClose }: { row: OvertimeRow; onClose: () => void }) {
  const correct = useCorrectOvertime();
  const [date, setDate] = useState(row.date.slice(0, 10));
  const [start, setStart] = useState(hhmm(row.start_time, ""));
  const [end, setEnd] = useState(hhmm(row.end_time, ""));
  const [dayType, setDayType] = useState<OvertimeDayType>(row.day_type ?? "regular");
  const [reason, setReason] = useState("");

  function submit() {
    const text = reason.trim();
    if (text.length < 5) {
      toast.error("Alasan koreksi minimal 5 karakter");
      return;
    }
    correct.mutate(
      { id: row.id, date, start_time: start, end_time: end, day_type: dayType, reason: text },
      {
        onSuccess: (res) => {
          toast.success(res.message || "Lembur dikoreksi");
          onClose();
        },
        onError: (e) => toast.error(e.message || "Gagal mengoreksi lembur"),
      }
    );
  }

  return (
    <>
      <DialogHeader>
        <DialogTitle>Koreksi Lembur — {row.employee?.full_name ?? "Karyawan"}</DialogTitle>
      </DialogHeader>
      <div className="space-y-3">
        <div>
          <label htmlFor="otc-date" className="mb-1 block text-xs font-medium text-gray-600">Tanggal</label>
          <Input id="otc-date" type="date" value={date} onChange={(e) => setDate(e.target.value)} />
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label htmlFor="otc-start" className="mb-1 block text-xs font-medium text-gray-600">Jam Mulai</label>
            <Input id="otc-start" type="time" value={start} onChange={(e) => setStart(e.target.value)} />
          </div>
          <div>
            <label htmlFor="otc-end" className="mb-1 block text-xs font-medium text-gray-600">Jam Selesai</label>
            <Input id="otc-end" type="time" value={end} onChange={(e) => setEnd(e.target.value)} />
          </div>
        </div>
        <div>
          <label htmlFor="otc-day-type" className="mb-1 block text-xs font-medium text-gray-600">Jenis lembur</label>
          <select
            id="otc-day-type"
            value={dayType}
            onChange={(e) => setDayType(e.target.value as OvertimeDayType)}
            className="h-9 w-full rounded-md border border-gray-200 bg-white px-2 text-sm"
          >
            {OVERTIME_DAY_TYPE_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label htmlFor="otc-reason" className="mb-1 block text-xs font-medium text-gray-600">Alasan koreksi</label>
          <Textarea id="otc-reason" rows={3} maxLength={500} value={reason} onChange={(e) => setReason(e.target.value)} />
        </div>
      </div>
      <DialogFooter>
        <Button variant="outline" onClick={onClose}>
          Batal
        </Button>
        <Button onClick={submit} disabled={correct.isPending}>
          {correct.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : "Simpan koreksi"}
        </Button>
      </DialogFooter>
    </>
  );
}
```

`frontend/src/features/hris/overtime/components/overtime-table.tsx`:
- import: tambahkan `import { OVERTIME_DAY_TYPE_OPTIONS, overtimeDayTypeLabel } from "@/lib/hris/overtime-rules";`
- `OvertimeTableProps` dan `OvertimeTableRowProps`: tambahkan

```ts
  canCorrect: boolean;
  onCorrect: (row: OvertimeRow) => void;
```

- `OvertimeTable({ rows, loading, decidingId, onDecide, canCorrect, onCorrect }`, dan `<OvertimeTableRow … onDecide={onDecide} canCorrect={canCorrect} onCorrect={onCorrect} />`.
- `function OvertimeTableRow({ row, deciding, onDecide, canCorrect, onCorrect }: OvertimeTableRowProps)`.
- ganti badge tanggal:

```tsx
        {row.day_type === "rest_day" && (
          <span className="ml-1 rounded-full bg-violet-100 px-2 py-0.5 text-xs font-medium text-violet-700">
            Hari istirahat
          </span>
        )}
```

dengan:

```tsx
        {row.day_type === "rest_day" && (
          <span className="ml-1 rounded-full bg-violet-100 px-2 py-0.5 text-xs font-medium text-violet-700">
            {overtimeDayTypeLabel(row.day_type)}
          </span>
        )}
        {row.corrected_at && (
          <span
            className="ml-1 rounded-full bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-700"
            title={`Dikoreksi ${row.corrected_by_name ?? "HR"}`}
          >
            Dikoreksi
          </span>
        )}
```

- ganti sel status `<span className={`rounded-full px-2 py-0.5 text-xs font-medium ${badge.className}`}>{badge.label}</span>` dengan:

```tsx
        <span className={`rounded-full px-2 py-0.5 text-xs font-medium ${badge.className}`}>{badge.label}</span>
        {row.decided_as === "hr" && <span className="ml-1 text-xs text-gray-500">oleh HR</span>}
```

- ganti dua `<option>` lama (`Hari biasa`, `Hari istirahat`) dengan:

```tsx
              {OVERTIME_DAY_TYPE_OPTIONS.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
```

- di sel aksi, sebelum `</td>` penutup tambahkan:

```tsx
        {canCorrect && (row.status === "pending" || row.status === "approved") && (
          <Button size="sm" variant="ghost" className="ml-2 text-xs" onClick={() => onCorrect(row)}>
            Koreksi
          </Button>
        )}
```

`frontend/src/features/hris/overtime/components/overtime-page.tsx`:
- import: tambahkan `import type { OvertimeRow } from "../types";` (gabungkan dengan import `OvertimeDayType` yang sudah ada) dan `import { OvertimeCorrectionDialog } from "./overtime-correction-dialog";`
- ganti `export function OvertimePage() {` dengan `export function OvertimePage({ canCorrect = false }: { canCorrect?: boolean }) {` dan tambahkan `const [correcting, setCorrecting] = useState<OvertimeRow | null>(null);` setelah `assignOpen`.
- pada `<OvertimeTable …>` tambahkan `canCorrect={canCorrect}` dan `onCorrect={setCorrecting}`.
- setelah `<OvertimeAssignDialog … />` tambahkan `<OvertimeCorrectionDialog row={correcting} onClose={() => setCorrecting(null)} />`.

`frontend/src/app/dashboard/(dashboard)/hris/overtime/page.tsx`:

```tsx
import { requireIamPage } from "@/lib/auth/require-user";
import { userHasIamAction } from "@/lib/iam/has-menu";
import { IAM } from "@/lib/iam/prefixes";
import { OvertimePage } from "@/features/hris/overtime";

/** Aksi update di menu daun Lembur = hak koreksi AdminHRIS (EPIC-063 R6), sama dengan guard Go. */
const OVERTIME_LEAF = ["hris.kepegawaian.overtime"] as const;

// HRIS → Kepegawaian → Lembur: approval pengajuan + penugasan perusahaan + koreksi.
export default async function HrisOvertimePage() {
  const user = await requireIamPage(IAM.hrisKepegawaian);
  const canCorrect = await userHasIamAction(user.id, user.role, OVERTIME_LEAF, "update");
  return <OvertimePage canCorrect={canCorrect} />;
}
```

- [ ] **Step 5: Jalankan test, tsc, eslint**

Run: `cd frontend && npx vitest run src/features/hris/ess src/features/hris/overtime src/lib/hris/overtime-rules.test.ts`
Expected: PASS.

Run: `cd frontend && npx tsc --noEmit -p . && npx eslint src/features/hris/ess src/features/hris/overtime "src/app/dashboard/(dashboard)/hris/overtime"`
Expected: tanpa error.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/features/hris/ess frontend/src/features/hris/overtime "frontend/src/app/dashboard/(dashboard)/hris/overtime/page.tsx"
git commit -m "feat(hris): form lembur ESS Weekday/Weekend + cek jam mulai, koreksi lembur AdminHRIS di HRIS (EPIC-063 Task 9)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 10: Kartu "Tanpa atasan" di Karyawan + form karyawan tanpa pilihan diri sendiri

**Files:**
- Modify: `frontend/src/lib/hris/reporting-to.ts`, `frontend/src/lib/hris/reporting-to.test.ts`
- Modify: `frontend/src/features/users/types.ts`, `api.ts`, `query-keys.ts`, `queries.ts`
- Create: `frontend/src/features/users/components/no-manager-card.tsx`
- Modify: `frontend/src/features/users/components/users-list-page.tsx:166`
- Modify: `frontend/src/features/users/components/user-form/employment-section.tsx`, `frontend/src/features/users/components/user-form-page.tsx:183`
- Test: `frontend/src/features/users/components/no-manager-card.test.tsx`

**Interfaces:**
- Consumes: `GET /api/hris/employees/without-manager` (Task 6), `isSelfManager` (Task 7).
- Produces:
  - `export type NoManagerReason = "empty" | "self" | "inactive"`, `export const NO_MANAGER_REASON_LABELS: Record<NoManagerReason, string>`
  - `export function managerOptions(managers: readonly { id: string; full_name: string; nip: string }[], selfId: string | null): { value: string; label: string; description: string }[]`
  - `export interface EmployeeWithoutManager { id: string; nip: string | null; full_name: string; job_title: string | null; department: string | null; reason: NoManagerReason }`, `useEmployeesWithoutManager()`
  - `export function NoManagerCard()`; `EmploymentSection` prop `employeeId?: string | null`

- [ ] **Step 1: Tulis test yang gagal**

`frontend/src/lib/hris/reporting-to.test.ts`, ubah import menjadi `import { isSelfManager, managerOptions } from "./reporting-to";` lalu tambahkan:

```ts
describe("managerOptions (EPIC-063 R9)", () => {
  const managers = [
    { id: ID, full_name: "Diri Sendiri", nip: "N1" },
    { id: "9f2b8c1e-4a5d-4e6f-8a9b-0c1d2e3f4a5b", full_name: "Atasan", nip: "N2" },
  ];

  it("karyawan yang diedit tidak muncul sebagai pilihan atasannya sendiri", () => {
    expect(managerOptions(managers, ID.toUpperCase()).map((o) => o.label)).toEqual(["Atasan"]);
  });

  it("karyawan baru melihat semua pilihan", () => {
    expect(managerOptions(managers, null)).toEqual([
      { value: ID, label: "Diri Sendiri", description: "N1" },
      { value: "9f2b8c1e-4a5d-4e6f-8a9b-0c1d2e3f4a5b", label: "Atasan", description: "N2" },
    ]);
  });
});
```

`frontend/src/features/users/components/no-manager-card.test.tsx`:

```tsx
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { EmployeeWithoutManager } from "../types";
import { NoManagerCard } from "./no-manager-card";

let rows: EmployeeWithoutManager[] | undefined;

vi.mock("../queries", () => ({ useEmployeesWithoutManager: () => ({ data: rows }) }));

afterEach(() => {
  cleanup();
  rows = undefined;
});

describe("NoManagerCard", () => {
  it("tidak tampil bila semua karyawan punya atasan", () => {
    rows = [];
    const { container } = render(<NoManagerCard />);
    expect(container.innerHTML).toBe("");
  });

  it("menampilkan jumlah dan daftar dengan alasannya", () => {
    rows = [
      { id: "a1", nip: "N1", full_name: "Retail Ops", job_title: "Retail Operation Manager", department: "Retail", reason: "self" },
      { id: "b2", nip: null, full_name: "Crew Baru", job_title: null, department: null, reason: "empty" },
    ];
    render(<NoManagerCard />);
    fireEvent.click(screen.getByRole("button", { name: /2 karyawan aktif tanpa atasan langsung yang valid/ }));
    expect(screen.getByText("Atasan = diri sendiri")).toBeTruthy();
    expect(screen.getByText("Atasan belum diisi")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Retail Ops" }).getAttribute("href")).toBe("/dashboard/employees/a1");
  });
});
```

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd frontend && npx vitest run src/lib/hris/reporting-to.test.ts src/features/users/components/no-manager-card.test.tsx`
Expected: FAIL — `managerOptions is not a function` dan `Failed to resolve import "./no-manager-card"`.

- [ ] **Step 3: Tulis implementasi**

`frontend/src/lib/hris/reporting-to.ts`, tambahkan:

```ts
/** Alasan karyawan aktif masuk laporan "tanpa atasan" (GET /api/hris/employees/without-manager). */
export type NoManagerReason = "empty" | "self" | "inactive";

export const NO_MANAGER_REASON_LABELS: Record<NoManagerReason, string> = {
  empty: "Atasan belum diisi",
  self: "Atasan = diri sendiri",
  inactive: "Atasan nonaktif",
};

/** Pilihan Atasan di form karyawan, tanpa karyawan yang sedang diedit (EPIC-063 R9). */
export function managerOptions(
  managers: readonly { id: string; full_name: string; nip: string }[],
  selfId: string | null
): { value: string; label: string; description: string }[] {
  return managers
    .filter((m) => !selfId || !isSelfManager(selfId, m.id))
    .map((m) => ({ value: m.id, label: m.full_name, description: m.nip }));
}
```

`frontend/src/features/users/types.ts`, tambahkan:

```ts
import type { NoManagerReason } from "@/lib/hris/reporting-to";

/** GET /api/hris/employees/without-manager (EPIC-063 R9). */
export interface EmployeeWithoutManager {
  id: string;
  nip: string | null;
  full_name: string;
  job_title: string | null;
  department: string | null;
  reason: NoManagerReason;
}
```

(letakkan baris `import type` di bagian import paling atas file.)

`frontend/src/features/users/api.ts`, tambahkan `EmployeeWithoutManager` ke import tipe dan:

```ts
export const fetchEmployeesWithoutManager = () =>
  apiGet<{ data: EmployeeWithoutManager[] }>("/api/hris/employees/without-manager").then((res) => res.data ?? []);
```

`frontend/src/features/users/query-keys.ts`, tambahkan entri di `usersQueryKeys`:

```ts
  withoutManager: () => ["users", "without-manager"] as const,
```

`frontend/src/features/users/queries.ts`, tambahkan `fetchEmployeesWithoutManager` ke import dari `./api`, lalu:

```ts
export const useEmployeesWithoutManager = () =>
  useQuery({ queryKey: usersQueryKeys.withoutManager(), queryFn: fetchEmployeesWithoutManager });
```

`frontend/src/features/users/components/no-manager-card.tsx`:

```tsx
"use client";

import Link from "next/link";
import { useState } from "react";
import { UserMinusIcon } from "@heroicons/react/24/outline";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { NO_MANAGER_REASON_LABELS } from "@/lib/hris/reporting-to";
import { useEmployeesWithoutManager } from "../queries";

/**
 * Kualitas data atasan langsung (EPIC-063 R9): karyawan aktif yang cuti dan
 * lemburnya hanya bisa diputus HR karena atasannya kosong, menunjuk dirinya
 * sendiri, atau nonaktif. Baca saja; perbaikan lewat edit karyawan.
 */
export function NoManagerCard() {
  const { data } = useEmployeesWithoutManager();
  const [open, setOpen] = useState(false);
  if (!data || data.length === 0) return null;

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="flex w-full items-center gap-3 rounded-xl border border-amber-200 bg-amber-50/70 p-4 text-left text-sm text-amber-900 shadow-sm hover:bg-amber-50"
      >
        <UserMinusIcon className="h-5 w-5 shrink-0 text-amber-600" />
        <span>
          <strong>{data.length} karyawan aktif tanpa atasan langsung yang valid</strong> — cuti & lembur mereka hanya
          bisa diputus HR · Lihat daftar →
        </span>
      </button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>Karyawan tanpa atasan langsung</DialogTitle>
          </DialogHeader>
          <ul className="max-h-[60vh] divide-y divide-gray-100 overflow-y-auto">
            {data.map((row) => (
              <li key={row.id} className="flex items-center justify-between gap-3 py-2 text-sm">
                <div className="min-w-0">
                  <Link href={`/dashboard/employees/${row.id}`} className="font-medium text-gray-900 hover:underline">
                    {row.full_name}
                  </Link>
                  <p className="truncate text-xs text-gray-500">
                    {[row.nip, row.job_title, row.department].filter(Boolean).join(" · ") || "—"}
                  </p>
                </div>
                <span className="shrink-0 rounded-full bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-800">
                  {NO_MANAGER_REASON_LABELS[row.reason]}
                </span>
              </li>
            ))}
          </ul>
        </DialogContent>
      </Dialog>
    </>
  );
}
```

`frontend/src/features/users/components/users-list-page.tsx`:
- tambahkan `import { NoManagerCard } from "./no-manager-card";`
- setelah `{!isAccountsView && <ContractExpiryBanner from={NAV_FROM_EMPLOYEES} />}` tambahkan `{!isAccountsView && <NoManagerCard />}`.

`frontend/src/features/users/components/user-form/employment-section.tsx`:
- tambahkan `import { managerOptions } from "@/lib/hris/reporting-to";`
- di `EmploymentSectionProps` tambahkan:

```ts
  /** Edit: id karyawan, supaya ia tidak bisa memilih dirinya sendiri sebagai atasan (EPIC-063). */
  employeeId?: string | null;
```

- ganti signature menjadi `export function EmploymentSection({ form, lookups, onChange, isEdit = false, employeeId = null }: EmploymentSectionProps) {`
- ganti prop `options` Combobox Manager:

```tsx
            options={(lookups?.managers ?? []).map((m) => ({
              value: m.id,
              label: m.full_name,
              description: m.nip,
            }))}
```

dengan:

```tsx
            options={managerOptions(lookups?.managers ?? [], employeeId)}
```

`frontend/src/features/users/components/user-form-page.tsx:183`, ganti:

```tsx
        <EmploymentSection form={form} lookups={lookups} onChange={setField} isEdit={isEdit} />
```

dengan:

```tsx
        <EmploymentSection form={form} lookups={lookups} onChange={setField} isEdit={isEdit} employeeId={editId} />
```

- [ ] **Step 4: Jalankan test, tsc, eslint**

Run: `cd frontend && npx vitest run src/lib/hris/reporting-to.test.ts src/features/users`
Expected: PASS (termasuk `user-form-page.test.tsx` lama).

Run: `cd frontend && npx tsc --noEmit -p . && npx eslint src/lib/hris/reporting-to.ts src/features/users`
Expected: tanpa error.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/hris/reporting-to.ts frontend/src/lib/hris/reporting-to.test.ts frontend/src/features/users
git commit -m "feat(hris): kartu karyawan tanpa atasan dan form tanpa pilihan atasan diri sendiri (EPIC-063 Task 10)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 11: Verifikasi menyeluruh & dokumentasi

**Files:**
- Modify: `docs/epics/EPIC-063-approval-atasan.md` (status, Automation Log)
- Modify: `docs/epics/README.md` (baris EPIC-063)
- Modify: `docs/hris/acuan-mom-20261019/status-requirement.md` (AP-01..AP-04 + ringkasan)
- Modify: `docs/hris/acuan-mom-20261019/matriks-approval.md` (Time-off, Overtime, Koreksi Overtime, data live)

- [ ] **Step 1: Full suite**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test -p 1 ./... -count=1 > /tmp/epic063-go.txt 2>&1; grep -- '--- FAIL' /tmp/epic063-go.txt | sed 's/ (.*//' | sort -u | diff /tmp/epic063-baseline.txt -`
Expected: `diff` kosong (tidak ada kegagalan baru). `gofmt -l backend/internal` tanpa output.

Run: `cd frontend && npx vitest run 2>&1 | tail -5 && npx tsc --noEmit -p .`
Expected: 0 gagal; tsc tanpa output.

- [ ] **Step 2: Cek manual di browser (DB test, bukan live)**

Jalankan API Go dan `next dev` terhadap DB test sesuai skill `run` proyek. Siapkan di DB test: karyawan A (akun ESS) dengan `reporting_to` = karyawan B (akun ESS), dan akun HR.
1. A: ESS → Lembur → Ajukan, pilih tanggal kemarin → toast "Lembur wajib diajukan sebelum dimulai — jam mulai sudah lewat". Pilih hari libur jadwal → Jenis terkunci "Weekend / Libur".
2. B: menu "Persetujuan Tim" berbadge; halaman memuat cuti/lembur A; Setujui → hilang dari antrean dan badge berkurang. Tolak tanpa alasan → toast.
3. HR: HRIS → Lembur → lembur A bertanda "oleh HR" setelah HR menyetujui; tombol Koreksi → ubah jam selesai + alasan → badge "Dikoreksi"; HRIS → Audit log memuat `overtime.correct`.
4. HR: Karyawan → kartu "karyawan aktif tanpa atasan langsung" → dialog daftar; edit karyawan → pilihan Manager tidak memuat dirinya sendiri.
5. WA (gateway uji, bila ada): pengajuan lembur A mengirim pesan ke B dengan footer "Area Karyawan → Persetujuan Tim".

- [ ] **Step 3: Dokumentasi**

- `status-requirement.md`: AP-01, AP-02, AP-03, AP-04 → ✅ "(EPIC-063, menunggu review & rilis)" dengan bukti `file:line` dan nama test (`TestLeaveDecisionManagerFirstHRBackup`, `TestLeaveSelfManagerCannotApproveOwn`, `TestOvertimeMustBeRequestedBeforeStart`, `TestOvertimeDayTypeFollowsSchedule`, `TestOvertimeDecisionRecordsCapacityAndNotifiesManager`, `TestOvertimeCorrectionByAdminHRIS`, `TestTeamApprovalsQueueAndBadge`, `TestEmployeeCannotReportToSelf`). Perbarui ringkasan: ✅ 20 (tambah AP-01..AP-04), 🟡 4 (ES-03, ES-05, PY-02, LA-01). AP-01 tetap mencatat bahwa data `reporting_to` live (55 kosong, 1 menunjuk diri sendiri) adalah keputusan user.
- `matriks-approval.md`, kolom "Kode sekarang":
  - Time-off: "EPIC-063: atasan langsung approver utama (`decided_as = manager`), HR cadangan/override (`decided_as = hr`, audit `leave.approve|reject`), tanpa self-approval; antrean ESS Persetujuan Tim".
  - Overtime: "EPIC-063: wajib sebelum jam mulai (WIB) untuk pengajuan sendiri; jenis Weekday/Weekend dari ESS dicek ke jadwal; atasan langsung + HR cadangan, audit; WA atasan saat pengajuan".
  - Koreksi Overtime: "EPIC-063: `PATCH /api/hris/overtime/{id}`, aksi `update` menu `hris.kepegawaian.overtime`, audit `overtime.correct`".
  - Baris data live "`reporting_to` menunjuk diri sendiri": dampak menjadi "Kode menolak self-approval dan tidak mengirim WA ke dirinya sendiri (EPIC-063); data tetap perlu diperbaiki user".
- Epic: Automation Log berisi commit Task 1–10, ruling R1–R11 yang dipakai (dan perubahannya, bila ada), bukti test, dan langkah rilis:
  1. Backup DB, terapkan migrasi `20261024090000_approval_atasan.sql` (setelah EPIC-062 `20261023090100`).
  2. Jalankan `SELECT iam.sync_hris_staff_roles();` (`admin_hris` mendapat `ess.team-approvals`).
  3. Build ulang API dan web.
  4. Perbaikan `reporting_to` live (55 kosong, 1 menunjuk diri sendiri) memakai laporan baru, oleh HR atau atas persetujuan user. Agent tidak mengubah data live.
- Set epic `status: review` dan baris registry README `review`, tanggal hari itu.

- [ ] **Step 4: Commit**

```bash
git add docs/epics docs/hris/acuan-mom-20261019
git commit -m "docs: EPIC-063 approval atasan selesai diimplementasikan, menunggu review

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

## Follow-up (di luar rencana ini)

- AP-06 (approval CEO/GM) menunggu K-7 dari Paddy.
- `hiring_manager` tetap dihitung HR cadangan untuk cuti/lembur (`Actor.IsHR`, perilaku lama), berbeda dengan Perjalanan Dinas (`IsHRD`). Konfirmasi ke user bila peran rekrutmen tidak boleh memutus.
- Keputusan HR atas cuti/lembur masih berbasis peran aplikasi (`IsHR`), bukan aksi `approve` di menu daun; menyelaraskannya dengan konvensi EPIC-061 adalah perubahan hak akses tersendiri.
- Pengingat/eskalasi otomatis bila atasan tidak memutus dalam N hari (sekarang hanya rekap `approvalMenginap` > 2 hari ke owner).
- Fallback Next tidak memvalidasi jenis lembur terhadap jadwal (R10).

## Self-review

**Cakupan spec:**
- AP-01 (atasan langsung, SPV ke atasannya, HR cadangan/override + audit) → Task 1 (`DecideAs`), Task 2 (cuti), Task 5 (antrean), Task 8 (UI).
- AP-02 (sebelum dilaksanakan + approval atasan) → Task 1 (`OvertimeStartsInPast`), Task 3 (server), Task 7 (fallback), Task 9 (ESS).
- AP-03 (Weekday/Weekend, K-13, tarif tetap) → Task 1 (`OvertimeDayTypeFor`), Task 3 (validasi + saran), Task 9 (form); payroll tidak disentuh (R5).
- AP-04 (koreksi AdminHRIS + audit, aksi menu) → Task 4 (API), Task 9 (UI).
- Persetujuan Tim + badge, tautan kasbon, reuse Perjalanan Dinas → Task 5, Task 8.
- Guard self-approval termasuk `reporting_to` = diri sendiri; HR override tercatat → Task 2, Task 3, Task 5, Task 7.
- Kualitas data (tolak diri sendiri; laporan kecil) → Task 6, Task 7, Task 10.
- Notifikasi WA atasan (cuti sudah ada, lembur baru; tanpa kanal baru) → Task 1 (pesan), Task 3 (outbox).
- Dokumentasi & rilis → Task 11.

**Pemindaian placeholder:** tidak ada "TBD"/"TODO"; setiap langkah kode memuat kodenya; perintah memuat Expected.

**Konsistensi tipe:** `DecideAs(a Approver, ownerID string, reportingTo *string, noun string)` dipakai Task 2 & 3; `leaveVerdict` hanya di Task 2; `DecideOvertime(..., rejection, dayType *string, meta audit.Entry)` di Task 3 cocok dengan handler; `ApprovalRepo` dibuat Task 5 dan diperluas Task 6; `otBody`/`leaveBody` dibuat Task 2/3 dan dipakai Task 4/5; TS `OvertimeDayType`, `OVERTIME_DAY_TYPE_OPTIONS`, `overtimeDayTypeLabel`, `OVERTIME_TOO_LATE`, `overtimeStartsInPast`, `decidedAs` dibuat Task 7 dan dipakai Task 8–9; `isSelfManager` (Task 7) dipakai `managerOptions` (Task 10).

**Review Focus:** kelima butir punya test di task pemiliknya (Task 1, 2, 3, 4, 5).

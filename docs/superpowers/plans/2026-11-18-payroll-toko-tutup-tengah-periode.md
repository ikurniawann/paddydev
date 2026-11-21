# EPIC-113 Payroll saat Toko Tutup di Tengah Periode — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Toko bisa dinonaktifkan dengan tanggal tutup. Jadwal dan absen sebelum tanggal itu tetap bisa dikoreksi. Karyawan yang tertinggal di toko tutup digaji prorata sampai sehari sebelum tutup, dan slipnya diberi keterangan.

**Architecture:** Toko mendapat kolom `configuration.branches.inactive_from` (tanggal pertama tutup).
- **Sisi HRIS:** semua pengecekan `b.is_active` yang menyangkut tanggal tertentu diganti satu helper SQL `branchOpenOn`, sehingga toko tetap terbuka untuk tanggal sebelum `inactive_from`.
- **Sisi payroll:** tanggal sesudah tutup yang tidak tercakup penempatan di toko lain dikeluarkan dari cakupan kerja. Mekanismenya sama dengan resign PY-04 (`BuildInputRange`, `merged`/`partial`), jadi faktor prorata, pembagi, dan tunjangan per hari mengikuti jalur yang sudah teruji.

**Tech Stack:** Go (`apps/backend`, pgx, migrasi delta SQL), Next.js (`apps/frontend`, vitest), PostgreSQL 16.

**Spec:** Pertanyaan user 18-11-2026: "ketika akan payroll tapi retail / toko di tengah periode berjalan itu nonaktif apa bisa terhitung gaji tiap jadwal karyawan yang telah ter assign?" Keputusan user:
- **K-1:** hari terjadwal sesudah toko tutup **tidak dibayar**. Perilaku "tidak hadir" dipertahankan, tetapi slip diberi **keterangan** dan gaji **dibayar prorata**.
- **K-2:** toko punya **tanggal tutup**. Untuk tanggal sebelum itu, toko tetap muncul di koreksi absen, jadwal, dan delegasi; sesudahnya toko hilang.

Fakta kode (origin/development 18-11, setelah `ea5197b`):
- Nonaktif toko hanya membalik flag (`configuration/settings/business.go:211-247`). Tidak ada cascade.
- Payroll membaca absen, jadwal, dan penempatan tanpa melihat status toko (`app/adapters_payroll.go:268-405`).
- `ActivePlacements` (`hris/placements.go:231`), `ActiveBranches` (`:287`), dan cakupan delegasi (`hris/schedule_delegates.go:142,315,375`) menyaring `b.is_active` untuk **semua** tanggal.
- Untuk gaji `monthly`, hari tidak hadir tidak memotong pokok. Akibatnya sekarang karyawan yang tertinggal di toko tutup dibayar pokok penuh, tetapi kehilangan makan/transport per hari. K-1 mengubah ini menjadi prorata.

Production per 16-11 belum punya toko nonaktif, jadi tidak ada backfill.

## Global Constraints

- Kode di repo `wit-id-indonesia/erp-paddy`, branch `feat/epic113-toko-tutup-payroll` dari `origin/development`, di worktree `paddydev/.worktrees/erp-epic113`. Jangan bekerja di `~/WORK/erp-paddy`.
- Merge hanya lewat PR ke `development`. Dokumen dan seeder tidak masuk erp-paddy (.gitignore). Rencana dan epic tinggal di repo paddydev.
- Production tidak disentuh. Merge ke `development` membatalkan gladi rilis sebelumnya, jadi rilis berikutnya wajib gladi ulang di salinan production.
- Go: `/home/wit/.local/toolchains/go/bin`. Test DB: `TEST_DATABASE_URL=postgres://postgres@127.0.0.1:55499/cookie_test?sslmode=disable`, migrasi dulu dengan `MIGRATE_DATABASE_URL=$TEST_DATABASE_URL go run ./cmd/migrate -apply`.
- Teks UI dan pesan error berbahasa Indonesia, persis seperti yang tertulis di rencana ini.
- Toko nonaktif lama (`is_active = false`, `inactive_from IS NULL`) tetap dianggap tutup untuk semua tanggal (perilaku sekarang) dan tidak memicu prorata.
- Endpoint Go baru tidak boleh ditambahkan. Tes manifest route (`apps/frontend/src/lib/backend-routes.test.ts`) mewajibkan fallback TypeScript untuk setiap route Go, dan per 18-11 tes itu sudah merah di base karena commit lain. Jangan perparah.
- `GET /api/hris/branches` tanpa parameter harus tetap mengembalikan daftar yang sama dengan sekarang.

## Review Focus

1. **Multi-penempatan (EPIC-112):** karyawan di dua toko, salah satunya tutup. Ia tidak boleh diprorata selama toko lain masih terbuka. Diuji di Task 3 (`TestStoreClosureKeepsOtherOpenPlacement`).
2. **Pindah toko dengan jeda:** penempatan lama diakhiri pada H-1 tutup, penempatan baru mulai beberapa hari sesudah tutup. Hanya hari jeda yang tidak dibayar. Diuji di Task 3 (`TestStoreClosureGapUntilNewPlacement`).
3. **Toko tutup sebelum periode dan karyawan tidak dipindah:** semua hari di luar cakupan, faktor 0, pokok 0. Slip harus memberi keterangan, dan ringkasan kalkulasi harus memperingatkan HR, bukan diam. Diuji di Task 3 dan Task 4.
4. **Toko diaktifkan kembali:** `inactive_from` kembali NULL, lalu hitung ulang draft tidak lagi memprorata. Diuji di Task 1 dan Task 4.
5. **Tanggal tutup di dalam periode absensi yang sudah ditutup:** harus ditolak, karena periode beku tidak boleh berubah diam-diam. Diuji di Task 1.

---

### Task 1: Tanggal tutup toko (`inactive_from`)

**Files:**
- Create: `apps/backend/database/migrations/deltas/20261218090000_branches_inactive_from.sql`
- Modify: `apps/backend/internal/modules/configuration/settings/business.go` (input + `updateBusinessEntity`)
- Modify: `apps/frontend/src/lib/configuration/business-repository.ts` (fallback Next, paritas)
- Modify: `apps/frontend/src/features/configuration/business/components/business-configuration-page.tsx` (form)
- Modify: `apps/frontend/src/features/configuration/business/types.ts`
- Test: `apps/backend/internal/modules/configuration/settings/business_integration_test.go`, `business-configuration-page` vitest yang sudah ada

**Interfaces:**
- Produces: kolom `configuration.branches.inactive_from date NULL` dengan `CHECK (is_active OR inactive_from IS NULL)`. Body PUT entitas bisnis tipe `branch` menerima `inactive_from` (`YYYY-MM-DD`). Respons list business menyertakan `inactive_from`.

Aturan:
- Untuk `is_active=false` pada branch, `inactive_from` boleh dikosongkan; default-nya hari ini (WIB).
- `inactive_from` tidak boleh sesudah hari ini.
- `inactive_from` tidak boleh jatuh pada atau sebelum tanggal akhir periode absensi terakhir yang `closed`.
- `is_active=true` selalu mengosongkan `inactive_from`.
- Tipe selain branch yang mengirim `inactive_from` ditolak.

- [ ] **Step 1: Tulis tes integrasi yang gagal** di `business_integration_test.go`:
  - `TestBranchDeactivateSetsInactiveFrom`: PUT `{is_active:false, inactive_from:"<kemarin>"}` → 200, kolom berisi kemarin.
  - `TestBranchDeactivateDefaultsToday`: PUT `{is_active:false}` → `inactive_from` = hari ini WIB.
  - `TestBranchReactivateClearsInactiveFrom`: PUT `{is_active:true}` → `inactive_from` NULL.
  - `TestBranchInactiveFromFutureRejected`: besok → 400 `"Tanggal tutup tidak boleh sesudah hari ini"`.
  - `TestBranchInactiveFromInClosedPeriodRejected`: periode absensi closed sampai X, kirim X → 400 `"Tanggal tutup jatuh di periode absensi yang sudah ditutup"`.
  - `TestInactiveFromOnlyForBranch`: tipe company → 400 `"Tanggal tutup hanya untuk toko/cabang"`.
- [ ] **Step 2: Jalankan, pastikan gagal.** `go test ./internal/modules/configuration/settings/ -run 'Branch.*InactiveFrom|InactiveFromOnly|BranchReactivate|BranchDeactivate' -count=1`. Expected: FAIL (kolom belum ada).
- [ ] **Step 3: Tulis migrasi** (kolom + CHECK + `COMMENT ON COLUMN … 'Tanggal pertama toko tutup (EPIC-113). NULL = aktif, atau nonaktif lama tanpa tanggal.'`). Lalu implementasikan aturan di `business.go`. Untuk periode closed, pakai query baca `hris.attendance_periods` yang setara dengan `periodContaining`. Perhatikan arah dependensi: modul configuration tidak boleh mengimpor hris, jadi tulis query SQL langsung.
- [ ] **Step 4: Fallback Next** `business-repository.ts`: terima dan simpan `inactive_from` dengan aturan yang sama (pesan error identik). Tambahkan tes vitest yang ada pasangannya untuk fallback ini.
- [ ] **Step 5: UI.** Saat checkbox Aktif dimatikan pada branch, tampilkan input tanggal "Tanggal tutup" (default hari ini, `max` = hari ini). Badge baris nonaktif menjadi "Nonaktif sejak 25 Nov 2026" bila tanggalnya ada. Tambahkan tes vitest bahwa body PUT memuat `inactive_from`.
- [ ] **Step 6: Jalankan.** Tes Go di Step 2 harus PASS. `pnpm --filter frontend vitest run src/features/configuration/business src/lib/configuration` juga PASS.
- [ ] **Step 7: Commit.** `feat(configuration): tanggal tutup toko inactive_from (EPIC-113)`

### Task 2: Toko terbuka untuk tanggal sebelum tutup (HRIS)

**Files:**
- Create: `apps/backend/internal/modules/hris/branch_open.go`
- Modify: `hris/placements.go` (`ActivePlacements`, `ActiveBranches`, handler `GET /api/hris/branches`, validasi insert/update penempatan)
- Modify: `hris/schedule_delegates.go:142,315,375`, `hris/schedule_days.go` (tulis jadwal), `hris/attendance_adjust.go` (validasi `branch_id`), `hris/auto_checkout_rules.go:125-162`
- Modify: `apps/frontend/src/features/hris/placements/api.ts` + `queries.ts` (`useStoreBranches(on?: string)`), `attendance-adjust-dialog.tsx`
- Test: `hris/attendance_multi_store_integration_test.go`, `hris/schedule_delegate_scope_integration_test.go`, `hris/schedule_days_integration_test.go`, tes baru `hris/branch_open_test.go`

**Interfaces:**
- Consumes: kolom `inactive_from` (Task 1).
- Produces:
  - `func branchOpenOn(alias, dateExpr string) string`, which returns `(<a>.is_active OR (<a>.inactive_from IS NOT NULL AND <date> < <a>.inactive_from))`.
  - `func branchVisibleSince(alias, dateExpr string) string`, which returns `(<a>.is_active OR (<a>.inactive_from IS NOT NULL AND <a>.inactive_from > <date>))`.
  - `GET /api/hris/branches?on=YYYY-MM-DD` returns the stores open on that date plus the `inactive_from` field. Without `on`, the output is unchanged.

Tanggal acuan setiap pemakaian:

| Lokasi | Tanggal acuan |
|---|---|
| `ActivePlacements` (absen masuk) | tanggal absen `$2` |
| Cakupan delegasi `:142`, `:315` | `branchVisibleSince` terhadap awal periode absensi yang masih terbuka |
| Cakupan delegasi `:375` | hari ini WIB, diganti `branchVisibleSince` awal periode terbuka |
| Auto-checkout | tanggal absen yang di-checkout |

Guard tulis:
- Menulis jadwal pada tanggal ≥ `inactive_from` ditolak dengan pesan `"Toko {nama} tutup sejak {tgl}; jadwal mulai tanggal itu tidak bisa diisi"`.
- Koreksi absen ke toko yang tidak terbuka pada tanggal absen ditolak dengan `"Toko {nama} tutup sejak {tgl}"`.
- Penempatan baru atau perpanjangan dengan tanggal ≥ `inactive_from` ditolak dengan `"Toko {nama} tutup sejak {tgl}; penempatan tidak bisa melewati tanggal itu"`.
- `{tgl}` memakai format tanggal Indonesia yang sudah dipakai modul hris (contoh "25 Nov 2026").

- [ ] **Step 1: Tulis tes unit** `TestBranchOpenOnSQL` di `branch_open_test.go` untuk string kedua helper.
- [ ] **Step 2: Tulis tes integrasi yang gagal.** Gunakan toko tutup `inactive_from = D` dengan D di dalam periode terbuka.
  - `TestClockInStoreOpenBeforeClosure`: `ActivePlacements(emp, D-1)` memuat toko; `(emp, D)` tidak.
  - `TestBranchesOnDateIncludesClosedBefore`: `GET /api/hris/branches?on=D-1` memuat toko, `?on=D` tidak, dan tanpa `on` tidak memuat.
  - `TestDelegateScopeKeepsStoreClosedInOpenPeriod`: delegasi masih bisa membuka grid toko untuk minggu yang memuat D-1.
  - `TestShiftDayOnOrAfterClosureRejected`: tulis jadwal tanggal D → 400 dengan pesan di atas; tanggal D-1 → 200.
  - `TestAttendanceAdjustToClosedStoreRejected`: koreksi tanggal D ke toko itu → 400; tanggal D-1 → 200.
  - `TestLegacyInactiveStoreStillHidden`: `is_active=false, inactive_from NULL` tetap tersaring untuk semua tanggal.
- [ ] **Step 3: Jalankan, pastikan gagal.** `go test ./internal/modules/hris/ -run 'BranchOpenOn|ClockInStoreOpen|BranchesOnDate|DelegateScopeKeeps|ShiftDayOnOrAfter|AttendanceAdjustToClosed|LegacyInactive' -count=1`. Expected: FAIL.
- [ ] **Step 4: Implementasi** helper dan semua lokasi di atas. `scheduleBranchRange` (`schedule_days.go:50`) tetap hanya cek keberadaan toko; guard tanggal diletakkan di jalur tulis.
- [ ] **Step 5: Frontend.** `useStoreBranches(on)` meneruskan `?on=`. Query key memuat `on`. Dialog koreksi absen memanggil `useStoreBranches(row.date)` dan menampilkan label "(tutup sejak 25 Nov 2026)" bila `inactive_from` ada. Tambahkan tes vitest bahwa dialog memanggil endpoint dengan `on=<tanggal baris>`.
- [ ] **Step 6: Jalankan** tes Step 3 dan seluruh `./internal/modules/hris/...`, lalu `pnpm --filter frontend vitest run src/features/hris/placements src/features/hris/attendance`. Expected: PASS.
- [ ] **Step 7: Commit.** `feat(hris): toko tetap terbuka untuk tanggal sebelum tutup (EPIC-113)`

### Task 3: Domain payroll — tanggal tutup keluar dari cakupan

**Files:**
- Create: `apps/backend/internal/modules/payroll/domain/store_closure.go`
- Modify: `apps/backend/internal/modules/payroll/domain/inputs.go` (`WorkforceRecords`, `BuildInputRange`, `Input`)
- Modify: `apps/backend/internal/modules/payroll/domain/calculator.go` (teruskan catatan ke `Result`)
- Test: `apps/backend/internal/modules/payroll/domain/store_closure_test.go`, `inputs_test.go`

**Interfaces:**
- Consumes: `domain.Placement` (`allocation.go:72`).
- Produces:

```go
type BranchClosure struct{ BranchID, Name, InactiveFrom string }
// StoreClosure: tanggal periode yang keluar dari cakupan karena toko tutup.
type StoreClosure struct {
	Dates    []string        // urut, unik, di dalam [start, end]
	Branches []BranchClosure // toko tutup yang menyebabkannya, urut nama
}
func StoreClosedDates(placements []Placement, closures map[string]BranchClosure, start, end string) StoreClosure
```

`WorkforceRecords.StoreClosure StoreClosure`, `Input.StoreClosureNote string`, and `Result.StoreClosureNote string`.

Aturan `StoreClosedDates`:
- Karyawan **terdampak** toko B bila B ada di `closures`, `B.InactiveFrom <= end`, dan ada penempatan di B yang berjalan pada `InactiveFrom - 1`.
- Tanggal d terhitung tutup bila d berada di `[max(start, InactiveFrom), end]` dan pada d **tidak ada** penempatan di toko yang terbuka. Toko terbuka pada d artinya tidak ada di `closures`, atau d < `InactiveFrom`-nya.
- Karyawan tanpa penempatan di periode itu (staf kantor pusat) menghasilkan nilai kosong.

Di `BuildInputRange`, setelah blok PY-04:
- Bila `len(rec.StoreClosure.Dates) > 0`, kurangkan tanggal itu dari `merged`. Jika `merged` kosong, pakai `[start, end]` sebagai dasar.
- `partial`, `factor`, `coveredScheduled`, dan `inCoverage` lalu mengikuti jalur yang sudah ada.
- `periodSchedule` tidak dipotong, sehingga pembagi tetap seluruh periode dan faktor = hari terjadwal tercakup / hari terjadwal periode.

Catatan yang dihasilkan:
- Bentuknya `"Toko {nama} tutup sejak {tgl}: {n} hari terjadwal sesudahnya tidak dibayar (gaji prorata)"`.
- `n` = hari terjadwal (`ScheduledDays` pada `periodSchedule`) di antara `Dates`; tanpa jadwal, `n` = jumlah tanggal.
- Beberapa toko digabung dengan `"; "`.
- Format `{tgl}` "25 Nov 2026".

- [ ] **Step 1: Tulis tes unit yang gagal** di `store_closure_test.go`:
  - `TestStoreClosureSingleStore`: penempatan B 1 Nov–terbuka, B tutup 25 Nov, periode 21 Okt–20 Nov. Hasil kosong, karena tutup sesudah periode.
  - `TestStoreClosureMidPeriod`: periode 21 Nov–20 Des, B tutup 5 Des. Dates = 5..20 Des.
  - `TestStoreClosureKeepsOtherOpenPlacement`: penempatan B dan C sejak 1 Nov, B tutup 5 Des. Hasil kosong.
  - `TestStoreClosureGapUntilNewPlacement`: B berakhir 4 Des, B tutup 5 Des, C mulai 8 Des. Dates = 5, 6, 7 Des.
  - `TestStoreClosureBeforePeriod`: B tutup 1 Nov, penempatan B tetap terbuka, periode 21 Nov–20 Des. Dates = seluruh periode.
  - `TestStoreClosureNotAffectedWhenPlacementEndedEarlier`: penempatan B berakhir 20 Nov, B tutup 5 Des. Hasil kosong.
- [ ] **Step 2: Tulis tes yang gagal** di `inputs_test.go`. Pakai pola mingguan 6 hari, gaji `monthly` 3.000.000, makan per hari, dan `StoreClosure.Dates` = 5..20 Des di periode 21 Nov–20 Des.
  - `TestBuildInputStoreClosureProrates`: `ProrateFactor` = hari terjadwal 21 Nov–4 Des / hari terjadwal periode. Absen "absent" sesudah 5 Des tidak dihitung. `StoreClosureNote` sesuai format.
  - `TestBuildInputStoreClosureWithResign`: resign 10 Des dan toko tutup 5 Des. Cakupan berakhir 4 Des, tanpa panik atau rentang negatif.
  - `TestBuildInputNoClosureUnchanged`: `StoreClosure` kosong menghasilkan `Input` identik dengan sebelum perubahan (tes paritas).
- [ ] **Step 3: Jalankan, pastikan gagal.** `go test ./internal/modules/payroll/domain/ -run 'StoreClosure|BuildInputStoreClosure|BuildInputNoClosure' -count=1`. Expected: FAIL (tipe belum ada).
- [ ] **Step 4: Implementasi** `store_closure.go` dan perubahan `inputs.go`. Teruskan catatan `Input` → `Result` di `calculator.go`.
- [ ] **Step 5: Jalankan** tes Step 3 dan seluruh `./internal/modules/payroll/...`. Expected: PASS, dan tes paritas EPIC-110/111 tetap hijau.
- [ ] **Step 6: Commit.** `feat(payroll): tanggal sesudah toko tutup keluar dari cakupan gaji (EPIC-113)`

### Task 4: Wiring payroll, slip, dan peringatan kalkulasi

**Files:**
- Create: `apps/backend/database/migrations/deltas/20261218090100_payroll_details_store_closure_note.sql`
- Modify: `apps/backend/internal/modules/payroll/ports.go` (`BranchClosures`), `apps/backend/internal/app/adapters_payroll.go`
- Modify: `apps/backend/internal/modules/payroll/service_runs.go:420-560` (isi `records.StoreClosure`, ringkasan)
- Modify: `apps/backend/internal/modules/payroll/postgres_runs.go:410-430` (kolom insert)
- Modify: `apps/backend/internal/modules/payroll/payslip_pdf.go` (`payslipInfo`), `domain/slip_lines.go` (`SlipAttendance`)
- Modify: `apps/frontend/src/features/hris/payroll/components/slip-lines-view.tsx`, `payslip-page.tsx`, `types.ts`, plus komponen ringkasan kalkulasi run yang menampilkan `Skipped`/`AllocationWarnings`
- Test: `apps/backend/internal/app/payroll_roster_test.go` atau tes integrasi baru `apps/backend/internal/modules/payroll/store_closure_integration_test.go`, `payslip_pdf` test yang ada, vitest slip

**Interfaces:**
- Consumes: `StoreClosedDates`, `Result.StoreClosureNote` (Task 3), and `inactive_from` (Task 1).
- Produces:
  - `BranchClosures(ctx, q) (map[string]domain.BranchClosure, error)` = `SELECT id, name, inactive_from FROM configuration.branches WHERE NOT is_active AND inactive_from IS NOT NULL`.
  - New column `hris.payroll_details.store_closure_note text NULL`.
  - New JSON field `store_closure_note` in `SlipAttendance`.
  - `calcSummary.StoreClosures []storeClosureWarning{EmployeeID, FullName, Note}` with JSON `store_closures`.

UI behaviour:
- **PDF slip:** `payslipInfo` adds the row `{"Keterangan", note}` when the note is not empty. Slips without a note keep exactly the same rows.
- **Web slip:** shows the same "Keterangan" row.
- **Calculation summary:** HR sees "Karyawan di toko yang tutup: {nama} — {catatan}" for each employee in `store_closures`.

- [ ] **Step 1: Tulis tes integrasi yang gagal** `TestPayrollRunProratesClosedStore`:
  - Data: dua karyawan bergaji `monthly` di toko B. A dipindah ke toko C mulai tanggal tutup; Z tidak dipindah. B dinonaktifkan dengan `inactive_from` di tengah periode terbuka. Hitung run.
  - Z: `prorate_factor < 1`, `base_salary` = round(pokok × faktor), `store_closure_note` sesuai format, dan Z muncul di `summary.store_closures`.
  - A: `prorate_factor = 1`, note NULL, tidak ada di ringkasan.
  - Aktifkan ulang B lalu hitung ulang draft: Z kembali `prorate_factor = 1` dan note NULL.
- [ ] **Step 2: Tulis tes yang gagal** di tes `payslip_pdf` yang ada: `payslipInfo` dengan note memuat baris "Keterangan"; tanpa note, barisnya identik dengan sekarang. Tulis juga vitest `slip-lines-view` yang menampilkan "Keterangan" hanya bila `store_closure_note` ada.
- [ ] **Step 3: Jalankan, pastikan gagal.** `go test ./internal/modules/payroll/ ./internal/app/ -run 'ProratesClosedStore|PayslipInfo' -count=1`. Expected: FAIL.
- [ ] **Step 4: Implementasi.**
  - Migrasi kolom beserta COMMENT.
  - Port dan adapter.
  - Di `service_runs.go`, `BranchClosures` dibaca sekali per run bersama `BranchCompanies`. Untuk tiap karyawan, `StoreClosedDates(placements, closures, start, end)` diisi ke `records` sebelum `BuildInputRange`. Ini berarti `payrollInput` menerima `placements` dan `closures`, atau `records` diisi di pemanggil; pilih satu dan pastikan jalur preview/hitung ulang lain yang memanggil `payrollInput` ikut terisi.
  - Insert kolom.
  - Field slip, lalu frontend.
- [ ] **Step 5: Jalankan** seluruh gate: `go test ./... -count=1` (ulangi paket flaky secara terpisah bila perlu), `pnpm --filter frontend lint`, `pnpm --filter frontend exec tsc --noEmit`, `pnpm --filter frontend vitest run`. Expected: semua PASS kecuali `backend-routes.test.ts` yang sudah merah di base. Catat statusnya; jangan sampai bertambah.
- [ ] **Step 6: Commit.** `feat(payroll): keterangan slip dan peringatan toko tutup (EPIC-113)`

### Task 5: Verifikasi di salinan data dan serah terima

**Files:**
- Create (paddydev): `docs/epics/EPIC-113-payroll-toko-tutup.md` (status, keputusan K-1/K-2, Automation Log)
- Modify (paddydev): `.claude/skills/paddy-hris-acuan/SKILL.md` (baris aturan: toko tutup → prorata sampai H-1, keterangan slip)

- [ ] **Step 1: Paritas di salinan production** (restore lokal di network internal, seperti gladi 0.8). Hitung ulang draft payroll November dengan kode baru. Tanpa toko nonaktif, semua `payroll_details` harus identik dengan engine lama. Pakai pola query paritas EPIC-110: 0 baris selisih.
- [ ] **Step 2: Skenario** di salinan yang sama:
  - nonaktifkan satu toko dengan `inactive_from` di tengah periode, lalu hitung ulang;
  - cek karyawan yang tertinggal diprorata dan slipnya punya "Keterangan";
  - cek karyawan multi-toko tidak diprorata.
  
  Salinan dihapus sesudahnya. File dump mode 600 dan tidak pernah dicetak.
- [ ] **Step 3: Review akhir** seluruh branch oleh reviewer segar, lalu buka PR ke `development`. Merge, deploy staging, dan QA HR di staging menunggu izin user.
- [ ] **Step 4: Catat** keputusan dan hasil di Automation Log EPIC-113, lalu perbarui skill acuan HRIS di commit yang sama.

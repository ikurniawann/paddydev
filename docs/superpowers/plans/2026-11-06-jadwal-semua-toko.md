# Pengatur Jadwal: Jadwal Semua Toko (EPIC-093) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** HR bisa memberi pengatur jadwal toko centang **"Boleh atur jadwal semua toko"**. Pengatur bercentang itu (para Koordinator) dapat mengisi jadwal di semua toko aktif. Perubahannya langsung berlaku tanpa persetujuan SPV, sedangkan penempatan dan checkout otomatis tetap hanya untuk toko wilayahnya.

**Architecture:**
- Kolom baru `hris.schedule_delegates.schedule_all_branches`.
- `delegateAccess` membawa dua daftar toko:
  - `BranchIDs`: wilayah, tetap dipakai penempatan dan checkout otomatis.
  - `ScheduleBranchIDs`: semua toko aktif bila dicentang, selain itu sama dengan `BranchIDs`.
- Keempat rute jadwal memakai `storeScope.AllowsScheduleBranch`.
- `GET /api/hris/me/schedule-access` mengirim `schedule_branches`, yang dipakai daftar toko tab Jadwal.

**Tech Stack:** Go (pgx, `backend/internal/modules/hris`), PostgreSQL migration delta, Next.js + React Query + Vitest (`frontend/src/features/hris`).

**Spec:** `docs/epics/EPIC-093-jadwal-semua-toko.md`. Ruling R1–R8 di file itu mengikat. Latar belakang: EPIC-071 (`docs/epics/EPIC-071-pengatur-jadwal-toko.md`, R1–R22).

## Global Constraints

- Worktree `.worktrees/epic093`, branch `feat/epic093-jadwal-semua-toko`, base `paddydev/main` (`614d3694`). Tidak bergantung pada PR #75–#80.
- Nama migrasi: `backend/database/migrations/deltas/20261111090000_schedule_delegate_all_branches.sql`. Timestamp ini sengaja setelah `20261110090300` milik EPIC-092.
- Tes Go:
  - `cd backend && PATH=$HOME/.local/go/bin:$PATH TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test_e084?sslmode=disable' go test -p 1 ./internal/modules/hris/ -count=1 -run '<pola>'`.
  - `TestGapDigestRun` kadang gagal saat seluruh paket berjalan. Ulangi tes itu sendirian sebelum menyimpulkan.
- Tes frontend: `cd frontend && pnpm vitest run <path>`. Gerbang akhir: `pnpm test`, `pnpm lint`, `pnpm exec tsc --noEmit`.
- Untuk `frontend/node_modules`, symlink ke checkout utama boleh dipakai, tetapi **jangan commit symlink `node_modules`, `.swarm`, atau `.claude-flow`**.
- Teks UI persis seperti berikut:
  - centang: `Boleh atur jadwal semua toko`;
  - petunjuk centang: `Jadwal toko lain langsung berlaku tanpa persetujuan. Penempatan tetap hanya di toko yang diatur di atas.`;
  - kolom tabel: `Jadwal semua toko` (isi `Ya` / `Tidak`);
  - catatan di tab Jadwal: `Anda bisa mengatur jadwal semua toko. Penempatan dan checkout otomatis hanya untuk toko wilayah Anda.`
- Pesan 403 tetap `Toko di luar wilayah Anda` (`errOutsideScope`), tanpa teks baru.
- Commit diakhiri `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Production read-only. Tanpa sesi atau akun uji di production. Merge dan deploy production hanya dengan izin user. Backup sebelum setiap deploy staging.

## Review Focus

1. **Toko nonaktif.** Pengatur bercentang tidak bisa membuka grid, template, atau unggah untuk toko nonaktif (403). Tes ada di Task 1.
2. **Penempatan.** Pengatur bercentang dengan `can_place` tetap ditolak 403 untuk penempatan di toko non-wilayah (cari, pindah). Tes ada di Task 1.
3. **Checkout otomatis.** Opsi aturan checkout otomatis pengatur bercentang tetap hanya toko wilayah. Tes ada di Task 1.
4. **PUT tanpa field.** PUT pengatur tanpa `schedule_all_branches` mematikan centang. Audit before/after memuat nilainya. Tes ada di Task 2.
5. **Pilihan toko di HP.** Pengatur bercentang dengan satu toko wilayah tetap langsung melihat tokonya, meski daftar memuat 45 toko. Tautan ke tab Penempatan tidak muncul di toko non-wilayah. Tes ada di Task 3.

---

### Task 1: Akses jadwal semua toko (Go: migrasi, scope, rute jadwal, schedule-access)

**Files:**
- Create: `backend/database/migrations/deltas/20261111090000_schedule_delegate_all_branches.sql`
- Modify: `backend/internal/modules/hris/schedule_delegates.go`
  - `delegateAccess`, `storeScope`, `loadDelegateAccess`, `myScheduleAccess`.
- Modify: `backend/internal/modules/hris/schedule_days.go:785,807,821,1122`
  - Ganti `sc.AllowsBranch(` dengan `sc.AllowsScheduleBranch(` di empat handler jadwal.
- Test (baru): `backend/internal/modules/hris/schedule_all_branches_integration_test.go`.
- Test: `backend/internal/modules/hris/schedule_delegates_integration_test.go`, perluas `TestScheduleAccessDelegate`.

**Interfaces:**
- Produces:
  - Kolom `hris.schedule_delegates.schedule_all_branches boolean NOT NULL DEFAULT false`.
  - `delegateAccess.ScheduleAll bool` dan `delegateAccess.ScheduleBranchIDs []string`. Isinya semua toko aktif urut `code, id` bila `ScheduleAll`, selain itu salinan `BranchIDs`.
  - `func (s storeScope) AllowsScheduleBranch(id string) bool`. Bernilai true untuk HR. Untuk pengatur, true hanya bila `id` ada di `ScheduleBranchIDs`, dan `""` tidak pernah lolos.
  - Field respons schedule-access:
    - `schedule_all_branches` (bool);
    - `schedule_branches` (`[]{id, code, name}`, urut kode). Untuk non-pengatur nilainya `false` dan `[]`.

- [ ] **Step 1: Tulis tes yang gagal** di `schedule_all_branches_integration_test.go`.
  - Gunakan `newScopeFixture`.
  - Helper `func (h *harness) scheduleAll(employeeID string)` menjalankan `UPDATE hris.schedule_delegates SET schedule_all_branches = true WHERE employee_id = $1`.
  - `TestAllBranchesDelegateSchedulesOtherStore`:
    1. Y ditempatkan di `tokoB` (`h.place`), lalu `h.scheduleAll(f.pID)`.
    2. `gridURL(f.tokoB)` → 200 dan memuat Y.
    3. Template `tokoB` → 200.
    4. `commitCells` untuk `cellsBody(f.tokoB, "", anaCell(y, "2026-10-06", "DEF"))` → 200, lalu `employee_shift_days` Y tanggal itu = `"1"`. Artinya jadwal langsung tersimpan, tanpa status menunggu.
    5. Unggah `anaWeek(f.tokoB, nipY, ...)` preview+commit → 200.
  - `TestAllBranchesDelegateInactiveStore`: `tokoC := h.branch(...)` lalu dinonaktifkan dengan `UPDATE configuration.branches SET is_active = false`. Dengan centang, `gridURL(tokoC)` → 403 `errNotInArea`.
  - `TestAllBranchesWithoutFlagUnchanged`: tanpa centang, `gridURL(f.tokoB)` → 403 `errNotInArea`. Ini regresi untuk EPIC-071 R4.
  - `TestAllBranchesPlacementStaysInArea`:
    1. Gunakan `newPlacementScopeFixture(h, true)` + `h.scheduleAll(f.pID)`.
    2. `POST placementBulkURL` dengan `{"branch_id": f.tokoC, "employee_ids": [f.x], "mode": "move", "step": "preview"}` → 403 `errNotInArea`.
    3. `GET searchPath(placementSearchURL, "branch_id", f.tokoC)` → 403 `errNotInArea`.
  - `TestAllBranchesAutoCheckoutStaysInArea`: dengan centang, `GET /api/hris/auto-checkout-rules/options` → `branches` hanya berisi `tokoA`, dan `all_locations` false.
  - `TestScheduleAccessAllBranches`:
    1. Dengan centang, `GET /api/hris/me/schedule-access` → `schedule_all_branches` true.
    2. `schedule_branches` memuat `tokoA` dan `tokoB`, tidak memuat toko nonaktif, dan urut kode.
    3. `branches` tetap `[tokoA]`.
  - Di `TestScheduleAccessDelegate` tambahkan dua assertion: `schedule_all_branches` false, dan id `schedule_branches` == id `branches`.

- [ ] **Step 2: Jalankan dan pastikan gagal**
  - Run: pola `-run 'AllBranches|TestScheduleAccessDelegate'`.
  - Expected: FAIL. Penyebab yang diharapkan: kolom `schedule_all_branches` belum ada, atau grid toko B = 403.

- [ ] **Step 3: Implementasi**
  - **Migrasi.** Isinya `ALTER TABLE hris.schedule_delegates ADD COLUMN IF NOT EXISTS schedule_all_branches boolean NOT NULL DEFAULT false;` dengan komentar kepala `-- EPIC-093: ...` seperti delta lain.
  - **`loadDelegateAccess`.** Ikut membaca `sd.schedule_all_branches`. Bila wilayah kosong, tetap kembalikan `nil` (R5). Setelah itu isi `ScheduleBranchIDs`:
    - bila `ScheduleAll`: `SELECT id::text FROM configuration.branches WHERE is_active ORDER BY code, id`;
    - selain itu: `slices.Clone(BranchIDs)`.
  - **`myScheduleAccess`.** Kirim `schedule_branches` lewat `delegateBranches(ctx, q, d.ScheduleBranchIDs)`.
  - **Komentar.** Perbarui doc comment `storeScope.AllowsBranch`: "wilayah — penempatan & checkout otomatis".

- [ ] **Step 4: Jalankan ulang**
  - Run: pola yang sama, lalu `-run 'Delegate|ScheduleScope|ScheduleAccess|AutoCheckout'` untuk regresi EPIC-071/080.
  - Expected: PASS semua.

- [ ] **Step 5: Commit**
  - `git add backend/database/migrations/deltas/20261111090000_schedule_delegate_all_branches.sql backend/internal/modules/hris/schedule_delegates.go backend/internal/modules/hris/schedule_days.go backend/internal/modules/hris/*_integration_test.go`.
  - Pesan: `feat(hris): pengatur bercentang mengatur jadwal semua toko (EPIC-093)`.

### Task 2: HR menyimpan centang "jadwal semua toko" (Go API pengatur)

**Files:**
- Modify: `backend/internal/modules/hris/schedule_delegates.go`
  - `delegateRows`, `delegateAuditState`, `storedDelegate`, `setScheduleDelegate`.
- Test: `backend/internal/modules/hris/schedule_delegates_admin_integration_test.go`.

**Interfaces:**
- Consumes: kolom dari Task 1.
- Produces:
  - Baris `GET /api/hris/schedule-delegates` dan respons PUT punya `schedule_all_branches` (bool).
  - Body PUT menerima `schedule_all_branches`. Bila tidak dikirim, nilainya false (`f.BoolDefault`, sama seperti `can_place`).
  - `func delegateAuditState(canPlace, scheduleAll bool, branchIDs []string) map[string]any`, dengan kunci `can_place`, `schedule_all_branches`, `branch_ids`.

- [ ] **Step 1: Tulis tes yang gagal** `TestDelegatesAdminScheduleAllBranches`. Pakai pola `TestDelegatesAdminSetListRevoke` dan `delegateHR`.
  1. PUT `{branch_ids:[A], can_place:false, schedule_all_branches:true}` → 200, `data.schedule_all_branches == true`.
  2. Daftar memuat true untuk orang itu.
  3. `audit.audit_log` terakhir `schedule_delegate.set` untuk employee itu punya `after->>'schedule_all_branches' = 'true'` (kolom `audit.audit_log.before`/`after`, jsonb).
  4. PUT kedua tanpa field → 200 dan false. Audit before-nya true.
  5. Kemudian `GET /api/hris/me/schedule-access` sebagai orang itu → `schedule_all_branches` false.

- [ ] **Step 2: Jalankan dan pastikan gagal**
  - Run: `-run TestDelegatesAdminScheduleAllBranches`.
  - Expected: FAIL. Respons belum punya `schedule_all_branches`.

- [ ] **Step 3: Implementasi**
  - **INSERT … ON CONFLICT.** Ikut menulis `schedule_all_branches`.
  - **`storedDelegate`.** Ikut membaca kolom itu.
  - **`delegateRows`.** SELECT dan objek baris memuat field itu.
  - **Pemanggil `delegateAuditState`.** Semuanya diperbarui.
  - **Doc comment PUT.** Menyebut field baru.

- [ ] **Step 4: Jalankan ulang**
  - Run: `-run 'TestDelegates'`.
  - Expected: PASS. Ini termasuk `TestDelegatesAdminSetListRevoke` lama. Bila tes lama membandingkan before/after secara utuh, tambahkan kunci baru di ekspektasinya, lalu catat Ruling di ledger.

- [ ] **Step 5: Commit**
  - Pesan: `feat(hris): HR mencentang pengatur boleh atur jadwal semua toko (EPIC-093)`.

### Task 3: Tab Jadwal memakai daftar toko jadwal (frontend ESS Jadwal Toko)

**Files:**
- Modify: `frontend/src/features/hris/schedule-access/types.ts`
  - Tambah `schedule_all_branches?: boolean` dan `schedule_branches?: ScheduleAccessBranch[]`, masing-masing dengan komentar JSDoc Indonesia.
- Modify: `frontend/src/features/hris/schedule-access/api.ts`
  - `EMPTY` mendapat `schedule_all_branches: false, schedule_branches: []`.
  - Hasil fetch: `schedule_branches: res.data?.schedule_branches ?? res.data?.branches ?? []`, supaya API lama tetap aman.
- Modify: `frontend/src/features/hris/schedules/components/schedules-page.tsx:61-64,194-206`.
- Test: `frontend/src/features/hris/schedules/components/schedules-page.test.tsx`.

**Interfaces:**
- Consumes: field schedule-access dari Task 1.
- Produces: tidak ada untuk task lain.

- [ ] **Step 1: Tulis tes yang gagal** di `schedules-page.test.tsx`. Ikuti cara mock schedule-access yang sudah ada di file itu untuk `mode="store"`.
  - `"pengatur semua toko melihat semua toko jadwal dan toko wilayahnya terpilih"`:
    - access: `branches:[A]`, `schedule_all_branches:true`, `schedule_branches:[A,B,C]`.
    - Combobox memuat opsi A, B, dan C.
    - Grid diminta dengan `branch_id=A`.
    - Teks catatan Global Constraints tampil.
  - `"tautan Penempatan tidak tampil di toko non-wilayah"`:
    - Pilih B dengan `onOpenPlacements` diberikan. Tombol atau tautan Penempatan pada grid kosong tidak ada.
    - Di A tombol itu ada.
  - `"tanpa centang daftar toko tetap wilayah"`:
    - `schedule_branches` sama dengan `branches` → hanya A, dan catatan tidak tampil.

- [ ] **Step 2: Jalankan dan pastikan gagal**
  - Run: `pnpm vitest run src/features/hris/schedules/components/schedules-page.test.tsx`.
  - Expected: FAIL. Opsi B tidak ada.

- [ ] **Step 3: Implementasi**
  - **Daftar toko.** Mode toko memakai `branchList = access.data?.schedule_branches`.
  - **Toko awal.** Pilihan otomatis memakai `access.data?.branches` dengan panjang 1: `branches[0].id`, bila id itu ada di `branchList`.
  - **Tautan Penempatan.** `const placementsHere = !store || !!access.data?.branches.some((b) => b.id === branchId)`. Kedua render `onOpenPlacements` (baris 194 dan 203) mensyaratkan `placementsHere`.
  - **Catatan.** Tampil di atas pemilih toko bila `store && access.data?.schedule_all_branches`. Gunakan teks Global Constraints dengan gaya `rounded-lg border border-sky-200 bg-sky-50 px-3 py-2 text-sm text-sky-800`, sama seperti catatan Mode HR di `jadwal-toko-page.tsx`.

- [ ] **Step 4: Jalankan ulang**
  - Run: file tes yang sama, lalu `pnpm vitest run src/features/hris/schedules src/features/hris/ess src/features/hris/schedule-access`.
  - Expected: PASS.

- [ ] **Step 5: Commit**
  - Pesan: `feat(hris): Jadwal Toko menampilkan semua toko untuk pengatur bercentang (EPIC-093)`.

### Task 4: Centang di halaman HR Pengatur Jadwal Toko (frontend HR)

**Files:**
- Modify: `frontend/src/features/hris/schedule-delegates/types.ts`
  - `ScheduleDelegate.schedule_all_branches: boolean`.
  - `ScheduleDelegatePayload.schedule_all_branches: boolean`, yang selalu dikirim.
- Modify: `frontend/src/features/hris/schedule-delegates/components/delegate-form-dialog.tsx:83,129,237-246`.
- Modify: `frontend/src/features/hris/schedule-delegates/components/schedule-delegates-page.tsx:98,182`.
- Test: `delegate-form-dialog.test.tsx`, `schedule-delegates-page.test.tsx`, dan `mutations.test.tsx` bila payload di-assert di sana.

**Interfaces:**
- Consumes: field API dari Task 2.

- [ ] **Step 1: Tulis tes yang gagal**
  - Dialog, `"mengirim schedule_all_branches"`:
    1. Centang `Boleh atur jadwal semua toko` (aria-label sama dengan teks).
    2. Simpan → payload `{branch_ids:[...], can_place:false, schedule_all_branches:true}`.
    3. Petunjuk centang tampil.
  - Dialog ubah: membuka baris dengan `schedule_all_branches:true` menampilkan centang tercentang.
  - Halaman: kolom header `Jadwal semua toko`. Baris bernilai true menampilkan `Ya`, false menampilkan `Tidak`.
  - Sesuaikan fixture `ScheduleDelegate` lama dengan `schedule_all_branches: false`, supaya `tsc` lulus.

- [ ] **Step 2: Jalankan dan pastikan gagal**
  - Run: `pnpm vitest run src/features/hris/schedule-delegates`.
  - Expected: FAIL. Centang tidak ditemukan.

- [ ] **Step 3: Implementasi**
  - **State centang.** Tambah `scheduleAllChoice`, dengan pola yang sama seperti `canPlaceChoice` (baris 83).
  - **Payload.** Kirim di payload baris 129.
  - **Render centang.** Letakkan di bawah centang penempatan, dengan struktur `<label>` yang sama dan petunjuk `text-xs text-gray-500`.
  - **Kolom tabel.** Kolom baru di sebelah `Penempatan`, dan tabel tetap `min-w-[880px]`.

- [ ] **Step 4: Jalankan ulang**
  - Run: `pnpm vitest run src/features/hris/schedule-delegates && pnpm exec tsc --noEmit && pnpm lint`.
  - Expected: PASS dan bersih.

- [ ] **Step 5: Commit**
  - Pesan: `feat(hris): centang jadwal semua toko di halaman Pengatur Jadwal Toko (EPIC-093)`.

### Task 5: Gerbang, dokumen, staging, QA

**Files:**
- Modify: `docs/epics/EPIC-093-jadwal-semua-toko.md`. Isi status, centang AC, dan Automation Log dengan commit, gerbang, dan QA.
- Modify: `docs/epics/README.md`, baris EPIC-093.

- [ ] **Step 1: Gerbang penuh**
  - Run: `go vet ./internal/modules/hris/...`, `go test -p 1 ./internal/modules/hris/... -count=1`, `make -C backend generate` (tanpa diff), lalu frontend `pnpm test`, `pnpm lint`, dan `pnpm exec tsc --noEmit`.
  - Expected: semua hijau. Bila `TestGapDigestRun` gagal, ulangi sendirian dan catat.
- [ ] **Step 2: Review akhir seluruh branch** (reviewer segar, model paling mampu). Satu gelombang perbaikan untuk temuan Critical/Important.
- [ ] **Step 3: Push dan PR** ke `ikurniawann/paddydev` dengan base `main`. Body diakhiri `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
- [ ] **Step 4: Deploy staging**
  1. Backup: `(umask 077; docker exec paddydev-db sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > ~/Desktop/paddydev-backups/staging-db-before-epic093-<stamp>.dump)`.
  2. Di `.worktrees/deploy-e071`: commit staging saat ini + `git merge --no-edit feat/epic093-jadwal-semua-toko`. Staging menjalankan tumpukan EPIC-087..092, dan tumpukan itu tetap ikut.
  3. `umask 022; bash tools/deploy-paddydev.sh` di background.
  4. Expected: migrasi diterapkan, verify "0 berbeda".
- [ ] **Step 5: QA staging** (Playwright, akun demo staging lalu dibersihkan; audit append-only):
  1. HR mencentang pengatur koordinator → kolom `Ya`.
  2. Koordinator di HP: daftar toko jadwal memuat toko lain dan tokonya terpilih otomatis.
  3. Ubah satu kotak di toko lain → tersimpan langsung, dan grid HR menampilkannya.
  4. Tab Penempatan tidak memuat toko lain, dan API penempatan toko lain → 403.
  5. Checkout otomatis hanya toko wilayah.
  6. HR mencabut centang → toko lain hilang dari daftar.
  7. 0 error konsol dan 0 API ≥ 500.
- [ ] **Step 6: Production (hanya dengan izin user)**
  1. Hitung ulang secara read-only 9 pengatur berjabatan Coordinator.
  2. Deploy pola `production-paddyadorable` dengan backup VPS dulu.
  3. Atas izin terpisah, set centang untuk 9 pengatur dalam satu transaksi. Tiap orang mendapat baris audit `schedule_delegate.set` dengan actor `Ops (Claude Code atas persetujuan user)`. Uji kering dengan ROLLBACK dulu, baru COMMIT. Alternatifnya, HR mencentang sendiri di aplikasi.

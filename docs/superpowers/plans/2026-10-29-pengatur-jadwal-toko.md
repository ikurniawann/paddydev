# Pengatur Jadwal Toko (EPIC-071) — Rencana Implementasi

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Karyawan yang ditunjuk HR (Store Supervisor, Shopkeeper Coordinator, dll.) bisa mengatur jadwal kerja (grid, pola, unggah Excel) dan, bila diizinkan, penempatan karyawan untuk toko wilayahnya, dari Area Karyawan di HP.

**Architecture:**
- Satu tabel penunjukan (`hris.schedule_delegates` + `hris.schedule_delegate_branches`) dan satu pembungkus akses Go, `withScheduleScope`. Pembungkus ini meloloskan HR apa adanya (tanpa wilayah) atau pengatur aktif dengan `storeScope` berisi daftar tokonya.
- Endpoint EPIC-070 yang sama (`/schedules`, `/schedules/template|import|cells`, `/employee-placements/search|ids|bulk|bulk-end`) memakai pembungkus itu dan memeriksa wilayah. Tidak ada endpoint jadwal/penempatan kedua.
- Frontend memakai ulang `SchedulesPage` dan `PlacementsPage` dengan prop `mode="store"`, di halaman ESS baru `/dashboard/me/jadwal-toko`. HR mengelola penunjukan di `/dashboard/hris/placements/pengatur`.

**Tech Stack:** Go 1.26 + pgx, PostgreSQL 16; Next.js (baca `frontend/AGENTS.md` dan `node_modules/next/dist/docs/` sebelum menulis kode Next), React Query, shadcn/ui, vitest.

**Spec:** `docs/epics/EPIC-071-pengatur-jadwal-toko.md` (Goal, Ruling R1–R12, Acceptance Criteria). Konteks EPIC-070: `docs/epics/EPIC-070-penempatan-jadwal-massal.md`.

## Global Constraints

- Teks UI berbahasa Indonesia, kalimat biasa, tanpa istilah teknis. Pesan 403 wilayah persis: `"Toko di luar wilayah Anda"`. Pesan karyawan di luar wilayah: `"%d karyawan di luar wilayah Anda"`. Pesan tanpa hak penempatan: `"Anda tidak punya hak mengatur penempatan"`.
- Perilaku HR tidak berubah:
  - pemeriksaan HR tetap persis seperti sekarang: baca jadwal = prefix `iam.HrisKepegawaian`; tulis jadwal = aksi `update` pada `scheduleManagers`; penempatan = prefix `placementManagers`;
  - semua tes integrasi EPIC-070 yang ada harus tetap hijau tanpa diubah.
- Pengatur = baris `hris.schedule_delegates` milik karyawan **aktif** yang `user_id`-nya = user sesi. Toko nonaktif (`configuration.branches.is_active = false`) tidak dihitung sebagai wilayah.
- Bawahan = subtree `hris.employees.reporting_to` dari karyawan pengatur, hanya karyawan aktif, kedalaman ≤ 10 (CTE rekursif dengan `UNION`).
- Tanggal = WIB (`domain.TodayWIB(s.now())`); frontend `todayWib()`.
- Setiap route Go baru wajib masuk `frontend/src/lib/go-routes.generated.json` (`export PATH=$HOME/.local/go/bin:$PATH; make -C backend generate`), dan tes `backend-routes.test.ts` diperbarui. Tanpa ini proxy Next 404.
- Menu ESS baru wajib ditambahkan ke `backend/database/seeders/iam-menus.sql` di tiga tempat: baris insert canonical, daftar `code NOT IN (...)` anti-retire, dan grant ESS ke semua role. Polanya mengikuti `ess.team-shifts`.
- Gerbang lokal (wajib hijau sebelum commit tiap task):
  - Go: `export PATH=$HOME/.local/go/bin:$PATH; cd backend && go vet ./internal/modules/hris/... && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test_e070?sslmode=disable' go test ./internal/modules/hris/... -count=1`. Migrasi baru diterapkan ke DB uji dulu, dengan cara yang sama seperti DB uji itu dibuat.
  - Frontend: `cd frontend && pnpm exec tsc --noEmit -p . && pnpm lint && pnpm test -- --run <folder yang disentuh>`.
- Jangan menyentuh production. Jangan mengubah file di luar cakupan task kecuali disebut.

## Review Focus

- **Pengatur memanggil API langsung untuk toko lain** (ubah `branch_id` di URL/body). Yang diharapkan: 403 `"Toko di luar wilayah Anda"` di setiap endpoint (GET grid, template, import, cells, search dengan `branch_id`, bulk tujuan). Tes ada di T3/T4.
- **Pengatur memilih karyawan dari toko wilayah, lalu karyawan itu dipindah HR ke toko lain sebelum simpan.** Yang diharapkan: commit penempatan → 403 dengan jumlah karyawan di luar wilayah; commit jadwal → error pratinjau "tidak ditempatkan di …" (validasi lama). Bukan 500. Tes di T4.
- **Karyawan pengatur dinonaktifkan atau aksesnya dicabut saat halamannya terbuka.** Yang diharapkan: permintaan berikutnya → 403, halaman menampilkan "Anda bukan pengatur jadwal toko", dan menu hilang setelah muat ulang. Tes di T1 (nonaktif) dan T2 (cabut).
- **Reporting_to berputar (A→B→A) atau sangat dalam.** Yang diharapkan: CTE berhenti (UNION + depth ≤ 10), tanpa hang. Tes di T1.
- **HR yang juga tercatat sebagai pengatur.** Yang diharapkan: diperlakukan sebagai HR (tanpa wilayah) di endpoint HR. Tes di T3.

---

### Task 1: Tabel penunjukan, menu ESS, akses inti

**Files:**
- Create: `backend/database/migrations/deltas/20261029120000_schedule_delegates.sql`
- Modify: `backend/database/seeders/iam-menus.sql`
- Create: `backend/internal/modules/hris/schedule_delegates.go`
- Modify: `backend/internal/modules/hris/routes.go` (route `GET /api/hris/me/schedule-access`)
- Modify: `frontend/src/lib/go-routes.generated.json`, `frontend/src/lib/backend-routes.test.ts` (atau lokasi tes manifest yang dipakai EPIC-070)
- Test: `backend/internal/modules/hris/schedule_delegates_integration_test.go`

**Interfaces:**
- Produces (Go, package `hris`):
  - `type delegateAccess struct { EmployeeID, Name string; CanPlace bool; BranchIDs []string }`
  - `type storeScope struct { Delegate *delegateAccess }`. `Delegate == nil` berarti HR.
  - `func (s storeScope) AllowsBranch(id string) bool`: true untuk HR; untuk pengatur, true hanya bila `id` ∈ `BranchIDs`. `""` → false untuk pengatur.
  - `func loadDelegateAccess(ctx context.Context, q database.Querier, userID string) (*delegateAccess, error)`: nil bila bukan pengatur aktif.
  - `const delegateSubtree string`: CTE `WITH RECURSIVE sub(id, depth)` dengan parameter `$1` = employee id pengatur.
  - `func delegateOutside(ctx context.Context, q database.Querier, d *delegateAccess, today string, ids []string) ([]string, error)`: id karyawan yang **tidak** dalam wilayah (R3: penempatan berjalan di toko wilayah, atau ada di subtree).
  - `GET /api/hris/me/schedule-access` → `{"data":{"is_delegate":bool,"can_place":bool,"branches":[{"id","code","name"}]}}`. Bukan pengatur → `is_delegate:false`, `branches:[]`.
- Produces (DB):
  - `hris.schedule_delegates(employee_id uuid PK REFERENCES hris.employees ON DELETE CASCADE, can_place boolean NOT NULL DEFAULT false, created_by uuid, created_at timestamptz NOT NULL DEFAULT now(), updated_by uuid, updated_at timestamptz NOT NULL DEFAULT now())`
  - `hris.schedule_delegate_branches(employee_id uuid REFERENCES hris.schedule_delegates ON DELETE CASCADE, branch_id uuid REFERENCES configuration.branches ON DELETE CASCADE, PRIMARY KEY (employee_id, branch_id))`
  - Menu `ess.store-schedule`, nama `Jadwal Toko`, route `/dashboard/me/jadwal-toko`, ikon `calendar-days`, order 46, actions `["read","update"]`, parent `ess`, di-grant ke semua role. Polanya meniru `20260829160000_ess_shift_tim_menu.sql`.

- [ ] **Step 1: Tulis tes integrasi yang gagal** di `schedule_delegates_integration_test.go`, memakai `harness` + `testutil.CreateStaff`:
  - `TestScheduleAccessNonDelegate`: karyawan biasa → 200, `is_delegate=false`, `branches=[]`.
  - `TestScheduleAccessDelegate`: insert penunjukan (toko A aktif + toko B nonaktif, `can_place=true`) → `is_delegate=true`, `can_place=true`, branches hanya A.
  - `TestScheduleAccessInactiveEmployee`: karyawan pengatur `is_active=false` → `is_delegate=false`.
  - `TestDelegateOutside`:
    - fixture: pengatur P di toko A; X ditempatkan di A; Y di toko C dan melapor ke Z (belum ditempatkan) yang melapor ke P (Y bawahan tidak langsung); W di C bukan bawahan; U belum ditempatkan dan melapor ke P;
    - `delegateOutside(P, [X,Y,W,U])` → `[W]`.
  - `TestDelegateSubtreeCycle`: P→Q→P (reporting_to berputar) → `delegateOutside` selesai, tanpa error.
- [ ] **Step 2: Jalankan** `go test ./internal/modules/hris/ -run 'TestScheduleAccess|TestDelegate' -count=1`. Expected: FAIL (route/func belum ada).
- [ ] **Step 3: Tulis migrasi + seeder**, terapkan ke DB uji. Lalu implementasikan `schedule_delegates.go` dan route `withActor`. Branch disortir `code`.
- [ ] **Step 4: Regenerasi manifest route** (`make -C backend generate`) dan perbarui tes manifest frontend. Jalankan gerbang Go + `pnpm test -- --run src/lib`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(hris): EPIC-071 tabel pengatur jadwal toko, menu ESS Jadwal Toko, akses inti`

### Task 2: API HR kelola pengatur

**Files:**
- Modify: `backend/internal/modules/hris/schedule_delegates.go`, `routes.go`
- Modify: manifest route + tesnya
- Test: `backend/internal/modules/hris/schedule_delegates_admin_integration_test.go`

**Interfaces:**
- Consumes: tabel T1, `delegateSubtree`.
- Produces (semuanya `h.withMenu(placementManagers, …)` untuk GET, `h.withAction("update", placementManagers, …)` untuk PUT/DELETE):
  - `GET /api/hris/schedule-delegates` → `{"data":[{"employee_id","nip","full_name","position_title","has_login":bool,"can_place":bool,"direct_reports":int,"branches":[{"id","code","name"}],"updated_at","updated_by_name"}]}`, urut `full_name`.
  - `GET /api/hris/schedule-delegates/suggest?employee_id=` → `{"data":{"branch_ids":[...]}}`: toko aktif dari penempatan berjalan karyawan itu ∪ subtree-nya (R2).
  - `PUT /api/hris/schedule-delegates/{employee_id}` body `{"branch_ids":[uuid…] (1..100), "can_place":bool}` → 200 baris seperti list. Upsert; daftar toko diganti seluruhnya.
    - Karyawan tidak ada → 404 `"Karyawan tidak ditemukan"`.
    - Karyawan nonaktif → 400 `"Karyawan tidak aktif"`.
    - Tanpa `user_id` → 400 `"Karyawan belum punya akun login"`.
    - `branch_ids` kosong → 400 `"Pilih minimal satu toko"`.
    - Toko tidak ada/nonaktif → 400 `"Toko tidak valid"`.
    - Audit `schedule_delegate.set` lewat `writeEmployeeAudit`, before/after = `{can_place, branch_ids}`.
  - `DELETE /api/hris/schedule-delegates/{employee_id}` → 200 `{"data":{"revoked":true}}`; tidak ada → 404. Audit `schedule_delegate.revoke`.

- [ ] **Step 1: Tes yang gagal:**
  - `TestDelegatesAdminSetListRevoke`: HR PUT (A,B, can_place) → GET list memuat 1 baris dengan 2 toko → PUT lagi (A saja) → 1 toko, audit 2 baris `schedule_delegate.set` → DELETE → list kosong, audit `schedule_delegate.revoke`, dan `GET /me/schedule-access` milik pengatur → `is_delegate=false`.
  - `TestDelegatesAdminValidation`: tanpa login, nonaktif, branch kosong, branch nonaktif → pesan persis di atas.
  - `TestDelegatesAdminForbidden`: karyawan biasa → 403 untuk keempat route.
  - `TestDelegatesSuggest`: fixture `TestDelegateOutside` → branch_ids = {A, C}.
- [ ] **Step 2: Jalankan**, expected FAIL.
- [ ] **Step 3: Implementasi** + regenerasi manifest.
- [ ] **Step 4: Gerbang Go + tes manifest.** Expected PASS.
- [ ] **Step 5: Commit** `feat(hris): EPIC-071 API HR kelola pengatur jadwal toko`

### Task 3: Jadwal Kerja berwilayah

**Files:**
- Modify: `backend/internal/modules/hris/schedule_delegates.go` (pembungkus), `schedule_days.go` (handler `getSchedules`, `getScheduleTemplate`, `importSchedules`, `saveScheduleCells`), `schedule_guard.go`, `routes.go`
- Test: `backend/internal/modules/hris/schedule_delegate_scope_integration_test.go`

**Interfaces:**
- Consumes: `loadDelegateAccess`, `storeScope` (T1).
- Produces:
  - `func (h handlers) withScheduleScope(action string, prefixes []string, needPlace bool, fn func(w http.ResponseWriter, r *http.Request, u *auth.User, sc storeScope) error) http.Handler`
    - `action == ""` → HR bila `HasMenuPrefix(prefixes)`, selain itu HR bila `HasMenuAction(action, prefixes)`;
    - bukan HR → `loadDelegateAccess`; nil → 403 `"Insufficient permissions"`; `needPlace && !CanPlace` → 403 `"Anda tidak punya hak mengatur penempatan"`.
  - `func (h handlers) clearGuardScoped(r *http.Request, u *auth.User, reason string, sc storeScope) (clearGuard, error)`: pengatur → `Allowed: true` (R5, alasan tetap wajib lewat `check`); HR → `clearGuardFor`.
  - Route: `GET /schedules` dan `GET /schedules/template` → `withScheduleScope("", kepegawaian, false, …)`; `POST /schedules/import` dan `POST /schedules/cells` → `withScheduleScope("update", scheduleManagers, false, …)`. Setiap handler memanggil `sc.AllowsBranch(branchID)` sebelum service; gagal → 403 `"Toko di luar wilayah Anda"`.
  - `POST /schedules/fixed` tidak berubah (HR saja, R7).

- [ ] **Step 1: Tes yang gagal** (pengatur P di toko A; karyawan Shift X ditempatkan di A; toko B):
  - `TestDelegateScheduleGrid`: P GET grid A → 200 berisi X; GET grid B → 403 pesan wilayah; GET tanpa `branch_id` → 403.
  - `TestDelegateScheduleTemplate`: A → 200 xlsx; B → 403.
  - `TestDelegateScheduleCells`:
    - P cells A (X, besok, shift aktif) preview → commit 200, baris `employee_shift_days` ada;
    - commit ulang kode lain tanpa `reason` → 400 alasan, bukan 403;
    - dengan reason → 200 + audit `schedule.cells_update` berisi reason;
    - cells dengan `branch_id` B → 403.
  - `TestDelegateScheduleImport`: unggah template A terisi → preview/commit 200; `branch_id` B → 403.
  - `TestScheduleScopeNonDelegate`: karyawan biasa → 403 `"Insufficient permissions"` di keempat route.
  - `TestScheduleScopeHRAlsoDelegate`: akun HR yang juga punya baris penunjukan untuk A → grid B tetap 200.
- [ ] **Step 2: Jalankan**, expected FAIL.
- [ ] **Step 3: Implementasi.**
- [ ] **Step 4: Gerbang Go penuh paket hris** (termasuk semua tes EPIC-070 lama). Expected PASS.
- [ ] **Step 5: Commit** `feat(hris): EPIC-071 Jadwal Kerja berwilayah untuk pengatur toko`

### Task 4: Penempatan berwilayah

**Files:**
- Modify: `backend/internal/modules/hris/placements.go` (`placementFilter`, `placementSearchFilter`, `searchPlacements`, `placementIDs`, `bulkPlacement`, `bulkEndPlacement`, `BulkPlacementPlan`, `BulkEndPlacement`, query `PlacementSearch`/`PlacementCounts`/`PlacementIDs`), `routes.go`
- Test: `backend/internal/modules/hris/placement_delegate_scope_integration_test.go`

**Interfaces:**
- Consumes: `withScheduleScope` (T3), `delegateOutside`, `delegateSubtree` (T1).
- Produces:
  - `placementFilter` mendapat field `Scope *delegateAccess`. Bila diisi, ketiga query (search, counts, ids) menambah syarat R3 (penempatan berjalan di `Scope.BranchIDs` **atau** `e.id` dalam subtree). `Branch` di luar wilayah → 403 pesan wilayah.
  - `bulkPlacementRequest` mendapat `Scope *delegateAccess`. `BulkEndPlacement` mendapat parameter terakhir `scope *delegateAccess`.
  - Route search/ids/bulk/bulk-end → `withScheduleScope("", placementManagers, true, …)`.
  - Aturan pengatur (R6):
    - bulk tanpa `mode` → 400 `"Pilih mode pindah atau tambah"`;
    - toko tujuan ∉ wilayah → 403 pesan wilayah;
    - `delegateOutside` tidak kosong → 403 `"%d karyawan di luar wilayah Anda"`;
    - mode move: karyawan dengan penempatan berjalan di toko ∉ wilayah dan bukan subtree → `outcome:"skipped"`, reason `"Punya penempatan di toko lain di luar wilayah Anda — hubungi HR"`;
    - bulk-end: `branch_id` diisi ∉ wilayah → 403; tanpa `branch_id` → hanya penempatan di toko wilayah yang diakhiri; karyawan tanpa penempatan di toko wilayah → skipped `"Tidak ada penempatan di toko Anda"`.

- [ ] **Step 1: Tes yang gagal** (fixture `TestDelegateOutside` + toko D di wilayah P, P `can_place=true`):
  - `TestDelegatePlacementSearch`: P search tanpa filter (Shift/Non-Shift, karyawan uji saja) → tepat {P, X, Y, Z, U}, tanpa W; counts.all = 5; `branch_id=C` → 403; ids → 5 id.
  - `TestDelegatePlacementMove`:
    - P move [X, U] ke D commit → X pindah (A berakhir kemarin), U ditempatkan, audit `placement.bulk_move`;
    - [X] ke C → 403 wilayah;
    - [W] ke D → 403 `"1 karyawan di luar wilayah Anda"`.
  - `TestDelegatePlacementMoveForeignPlacement`: X punya penempatan tambahan di C dan bukan bawahan → move ke D, X skipped dengan reason persis. Y (bawahan, di C) → dipindah.
  - `TestDelegatePlacementEnd`: X di A + C → bulk-end tanpa branch → hanya A berakhir; W → 403; `branch_id=C` → 403.
  - `TestDelegatePlacementNoRight`: P `can_place=false` → 403 `"Anda tidak punya hak mengatur penempatan"` di keempat route.
  - `TestDelegatePlacementStale`: X dipindah HR ke C sesudah preview → commit P → 403 `"1 karyawan di luar wilayah Anda"`.
- [ ] **Step 2: Jalankan**, expected FAIL.
- [ ] **Step 3: Implementasi.**
- [ ] **Step 4: Gerbang Go penuh paket hris.** Expected PASS, termasuk tes EPIC-070 lama.
- [ ] **Step 5: Commit** `feat(hris): EPIC-071 Penempatan berwilayah untuk pengatur toko`

### Task 5: Halaman HR "Pengatur Jadwal Toko"

**Files:**
- Create: `frontend/src/features/hris/schedule-delegates/{api.ts,types.ts,query-keys.ts,queries.ts,mutations.ts,index.ts}`
- Create: `frontend/src/features/hris/schedule-delegates/components/{schedule-delegates-page.tsx,delegate-form-dialog.tsx}` + tes `*.test.tsx`
- Create: `frontend/src/app/dashboard/(dashboard)/hris/placements/pengatur/page.tsx`
- Modify: `frontend/src/features/hris/placements/components/placements-page.tsx` (tombol header "Pengatur jadwal toko" → `/dashboard/hris/placements/pengatur`, hanya bila `canManage`)
- Modify: `frontend/src/app/dashboard/(dashboard)/hris/page-guards.test.ts`

**Interfaces:**
- Consumes: API T2.
- Produces:
  - `type ScheduleDelegate` sesuai respons list T2.
  - `scheduleDelegateKeys = { all: ["hris","schedule-delegates"] }`.
  - `useScheduleDelegates()`, `useDelegateSuggest(employeeId)`, `useSaveDelegate()`, `useRevokeDelegate()`. Mutasi menginvalidasi `["hris","schedule-delegates"]` dan `["hris","schedule-access"]`.
  - Page guard: `requireIamPage([...IAM.hrisPlacements, ...IAM.hrisKepegawaian, ...IAM.settingsUsers])`, sama dengan halaman Penempatan.
- UI:
  - Tabel: Nama + NIP, Jabatan, Toko (chip kode; lebih dari 3 → "+n"), Penempatan (Ya/Tidak), Bawahan, Diubah. Aksi Ubah / Cabut (konfirmasi).
  - Kosong: "Belum ada pengatur. Tambahkan karyawan yang boleh mengatur jadwal tokonya."
  - Dialog Tambah/Ubah:
    - Combobox karyawan, cari lewat `GET /api/hris/employee-placements/search?q=` (tampilkan nama, NIP, jabatan, toko utama);
    - begitu dipilih, toko terisi dari suggest;
    - multi-pilih toko dari `useStoreBranches()`;
    - centang "Boleh atur penempatan karyawan";
    - teks bantuan "Pengatur hanya bisa mengubah jadwal dan penempatan di toko yang dipilih";
    - error server ditampilkan di dialog.

- [ ] **Step 1: Tes yang gagal:**
  - `schedule-delegates-page.test.tsx`: render list mock 2 baris → chip toko + "+n"; klik Cabut → konfirmasi → `DELETE` dipanggil dengan id.
  - `delegate-form-dialog.test.tsx`: pilih karyawan → suggest mengisi 2 toko → simpan memanggil `PUT` dengan `{branch_ids, can_place:false}`; respons 400 "Karyawan belum punya akun login" tampil di dialog.
  - page-guards: `pengatur/page` memanggil `requireIamPage` dengan prefix di atas.
- [ ] **Step 2: Jalankan** `pnpm test -- --run src/features/hris/schedule-delegates src/app/dashboard`. Expected FAIL.
- [ ] **Step 3: Implementasi.**
- [ ] **Step 4: Gerbang frontend.** Expected PASS.
- [ ] **Step 5: Commit** `feat(hris): EPIC-071 halaman HR Pengatur Jadwal Toko`

### Task 6: Halaman ESS "Jadwal Toko" + menu bersyarat

**Files:**
- Create: `frontend/src/features/hris/schedule-access/{api.ts,types.ts,queries.ts,index.ts}`
- Create: `frontend/src/features/hris/ess/components/jadwal-toko-page.tsx` + `jadwal-toko-page.test.tsx`; ekspor di `ess/index.ts`
- Create: `frontend/src/app/dashboard/(dashboard)/me/jadwal-toko/page.tsx` (`requireUser()`, seperti Shift Tim)
- Modify: `frontend/src/features/hris/schedules/components/schedules-page.tsx`, `frontend/src/features/hris/placements/components/{placements-page.tsx,placement-filters.tsx,placements-table.tsx}`
- Create: `frontend/src/lib/iam/schedule-delegate.ts` (`isScheduleDelegate(userId: string): Promise<boolean>`, memakai `iamDbQueryOne`: ada baris `hris.schedule_delegates` milik karyawan **aktif** dengan `user_id = userId` dan minimal satu toko aktif; error query → false, dicatat `console.error`) + tes
- Modify: `frontend/src/app/dashboard/(dashboard)/layout.tsx` (buang nav item href `/dashboard/me/jadwal-toko` bila `!isScheduleDelegate(user.id)`, rekursif lewat helper baru `dropNavHref(items, href)` di `get-user-menus.ts`)

**Interfaces:**
- Consumes: `GET /api/hris/me/schedule-access` (T1), endpoint berwilayah T3/T4.
- Produces:
  - `type ScheduleAccess = { is_delegate: boolean; can_place: boolean; branches: { id: string; code: string; name: string }[] }`; `useScheduleAccess()` key `["hris","schedule-access"]`.
  - `SchedulesPage` prop baru `mode?: "hr" | "store"` (default `"hr"`). Di `"store"`:
    - daftar toko dari `useScheduleAccess().branches`; satu toko → langsung terpilih;
    - tombol Jadwal tetap Non-Shift dan tautan ke halaman HR (`/dashboard/employees`, `/dashboard/hris/placements`) disembunyikan;
    - tampilan awal 7 hari.
  - `PlacementsPage` prop baru `mode?: "hr" | "store"` dan `onOpenSchedule?: (branchId: string, employeeIds: string[]) => void`. Di `"store"`:
    - `canManage = access.can_place`;
    - filter toko dibatasi toko wilayah;
    - filter departemen/jabatan disembunyikan (API master khusus HR);
    - tombol lokasi toko disembunyikan; tautan nama ke profil karyawan jadi teks biasa;
    - "Atur jadwal" memanggil `onOpenSchedule`, bukan `router.push` ke halaman HR.
  - `JadwalTokoPage`:
    - `access.is_delegate === false` → kartu "Anda bukan pengatur jadwal toko. Hubungi HR bila seharusnya punya akses.";
    - selain itu tab **Jadwal** (`SchedulesPage mode="store"`) dan **Penempatan** (hanya bila `can_place`);
    - `onOpenSchedule` berpindah ke tab Jadwal dengan toko + karyawan tercentang (prop `initialBranchId`/`initialEmployeeIds` yang sudah ada).

- [ ] **Step 1: Tes yang gagal:**
  - `jadwal-toko-page.test.tsx`: akses `is_delegate:false` → teks kartu persis; akses 1 toko, `can_place:false` → tab Penempatan tidak ada, grid memanggil `/api/hris/schedules?branch_id=<toko>`; `can_place:true` → tab Penempatan ada, filter departemen tidak dirender.
  - `schedules-page.test.tsx` (tambahan): `mode="store"` tidak merender tombol jadwal tetap Non-Shift dan hanya menawarkan toko dari akses.
  - `placements-page.test.tsx` (tambahan): `mode="store"` → "Atur jadwal" memanggil `onOpenSchedule`, bukan `router.push`.
  - `schedule-delegate.test.ts`: `dropNavHref` membuang item bersarang dan mempertahankan group yang masih punya anak.
  - Semua tes `schedules`/`placements` lama tetap hijau tanpa diubah (mode default `"hr"`).
- [ ] **Step 2: Jalankan** `pnpm test -- --run src/features/hris src/lib/iam`. Expected FAIL.
- [ ] **Step 3: Implementasi.**
- [ ] **Step 4: Gerbang frontend penuh** (`pnpm test` seluruhnya + `tsc` + `lint`). Expected PASS.
- [ ] **Step 5: Commit** `feat(hris): EPIC-071 halaman ESS Jadwal Toko untuk pengatur`

### Task 7: Dokumen + gerbang akhir

**Files:**
- Modify: `docs/epics/EPIC-071-pengatur-jadwal-toko.md` (status `review`, Automation Log per task + commit), `docs/epics/README.md` (status), `docs/hris/acuan-mom-20261019/status-requirement.md` (catatan akses pengatur toko di baris jadwal/penempatan yang relevan, bila ada)

- [ ] **Step 1:** Jalankan gerbang penuh: `go vet ./... && go test ./...` (dengan `TEST_DATABASE_URL`), `pnpm exec tsc --noEmit -p .`, `pnpm lint`, `pnpm test`. Expected: hijau. Tes flaky lama yang diketahui (`TestRulesRoutes`, `TestLeaveBalanceImportLocksInIDOrder`) dijalankan ulang terpisah dan dicatat.
- [ ] **Step 2:** Pastikan `git diff paddydev/main -- frontend/src/lib/go-routes.generated.json` memuat 5 route baru: `GET /api/hris/me/schedule-access`, `GET /api/hris/schedule-delegates`, `GET /api/hris/schedule-delegates/suggest`, `PUT /api/hris/schedule-delegates/{employee_id}`, `DELETE /api/hris/schedule-delegates/{employee_id}`.
- [ ] **Step 3: Commit** `docs: EPIC-071 Automation Log implementasi`

# Bagan Organisasi Draft 6 (Fase 1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Draft 6 bagan organisasi Paddy masuk HRIS sebagai master data berversi. Strukturnya divalidasi, ditampilkan sebagai tab di halaman Struktur Org, dan dibandingkan dengan data karyawan nyata dalam laporan selisih untuk HR.

**Architecture:**
- Tiga tabel baru di schema `hris`: versi, node, dan alias jabatan. Seed Draft 6 masuk lewat migrasi data hasil generator dari CSV klien.
- Aturan murni ada di `hris/domain/org_chart.go`:
  - validasi BR;
  - pencocokan karyawan → node.
- Service `hris/org_chart.go` membaca DB dan menyajikan 4 endpoint `GET`. Laporan Excel dibuat dengan `platform/xlsx`.
- Frontend menambah dua tab di halaman org chart yang sudah ada.
- Tabel karyawan, jabatan, departemen, toko, dan approval tidak disentuh.

**Tech Stack:** Postgres migration (`backend/database/migrations/deltas`), Go (`internal/modules/hris`), Python 3 (generator SQL), Next.js + React Query + shadcn `Tabs`.

**Spec:** `docs/epics/EPIC-101-bagan-organisasi-draft6.md` (ruling R1–R9, tabel BR). Data: `docs/hris/org-draft6/*.csv`.

## Global Constraints
- Tidak ada UPDATE/INSERT/DELETE ke `hris.employees`, `hris.positions`, `hris.departments`, `configuration.branches`, maupun fungsi approval (R1).
- Level `L0`–`L8`, rank = angka setelah `L`, makin kecil makin senior.
- `employment_type` ∈ `PERMANENT_OR_CONTRACT`, `INTERN`, `FREELANCE`, `OUTSOURCE`; `status` ∈ `active`, `planned`, `vacant`, `frozen`.
- Kode toko node → branch: `upper(regexp_replace(b.code, '^ODOO-', ''))` (R6).
- Akses semua endpoint: `withMenu(iam.HrisOrganization, …)` (R8).
- Pesan dan label UI berbahasa Indonesia.
- Next.js: baca panduan di `frontend/node_modules/next/dist/docs/` sebelum menulis kode halaman (lihat `frontend/AGENTS.md`).
- Commit diakhiri `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Review Focus
1. Karyawan dengan judul yang cocok ke banyak node tanpa toko pembeda (mis. "Shopkeeper" yang ditempatkan di HQ): ditandai "ambigu", bukan dipaksa ke node pertama (Task 2).
2. Atasan langsung di node induk yang kosong (toko tanpa Store Leader, atau SPV vacant): tidak dilaporkan "atasan berbeda" (Task 2).
3. Kode toko yang sudah berubah lewat EPIC-100 (`ODOO-PPGA` → `PPGA`) tetap cocok dengan node `PPGA` (Task 3).
4. Seed dijalankan ulang (migrasi idempoten, atau baris sudah ada di staging): tidak ada duplikat dan tidak ada error (Task 1).
5. Filter tree dengan nilai yang tidak ada (`division=XXX`): kembali 200 dengan daftar kosong, bukan 500 (Task 3).

---

### Task 1: Skema + seed Draft 6

**Files:**
- Create: `backend/database/migrations/deltas/20261115090000_org_chart.sql` (tabel)
- Create: `tools/orgchart/draft6_seed_sql.py` (generator)
- Create: `backend/database/migrations/deltas/20261115090100_org_chart_draft6_seed.sql` (hasil generator, di-commit)
- Test: `backend/internal/modules/hris/org_chart_seed_integration_test.go`

**Interfaces:**
- Produces: tabel
  - `hris.org_chart_versions(id uuid PK default gen_random_uuid(), label text NOT NULL UNIQUE, source_file text, status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','approved','archived')), valid_from date, imported_at timestamptz NOT NULL DEFAULT now())`;
  - `hris.org_chart_nodes(id uuid PK, version_id uuid NOT NULL REFERENCES hris.org_chart_versions ON DELETE CASCADE, code text NOT NULL, parent_code text, sort_order int NOT NULL, title text NOT NULL, raw_title text NOT NULL, is_position bool NOT NULL DEFAULT true, division_code text NOT NULL, division_name text NOT NULL, job_level text CHECK (job_level ~ '^L[0-8]$'), level_confidence text, employment_type text CHECK (…empat nilai…), vendor text, region text, store_codes text[] NOT NULL DEFAULT '{}', status text NOT NULL DEFAULT 'active' CHECK (…), needs_confirm bool NOT NULL DEFAULT false, flags text[] NOT NULL DEFAULT '{}', UNIQUE (version_id, code), FOREIGN KEY (version_id, parent_code) REFERENCES hris.org_chart_nodes (version_id, code) DEFERRABLE INITIALLY DEFERRED)`;
  - `hris.org_title_aliases(alias text PRIMARY KEY, title text NOT NULL, region text, needs_review bool NOT NULL DEFAULT true, note text)`, dengan `alias` disimpan `lower(btrim())`.

  Grant SELECT ke role yang sama dengan tabel `hris` lain di migrasi terbaru (contoh: `20261114090000_attendance_geofence_imprecise.sql`).

- [ ] **Step 1: Tulis tes yang gagal**

`TestOrgChartDraft6Seed` (`testutil.Tx`) assert:
- `SELECT count(*) FROM hris.org_chart_versions WHERE label = 'Draft 6' AND status = 'draft'` = 1;
- node 150; `is_position` 149;
- `count(DISTINCT c) FROM nodes, unnest(store_codes) c` = 33;
- `needs_confirm` 20; `status = 'planned'` 9;
- root (`parent_code IS NULL`) tepat satu: `EXE-001` `CEO`;
- `SUP-002.parent_code = 'EXE-002'`; `RTL-030.store_codes = {PPAY,PLAY}`;
- `HRG-012`: `employment_type = OUTSOURCE`, `vendor = 'Red Guard'`;
- alias 43; `SELECT title FROM hris.org_title_aliases WHERE alias = 'patcher staff'` = `Patcher`.

- [ ] **Step 2: Jalankan dan pastikan gagal**

Run: `cd backend && go test ./internal/modules/hris/ -run TestOrgChartDraft6Seed -count=1` (DB uji sudah dimigrasi tanpa tabel ini)
Expected: FAIL (`relation "hris.org_chart_versions" does not exist`).

- [ ] **Step 3: Implementasi**

Generator `python3 tools/orgchart/draft6_seed_sql.py docs/hris/org-draft6 > backend/database/migrations/deltas/20261115090100_org_chart_draft6_seed.sql` menghasilkan satu transaksi:
- `INSERT` versi `('Draft 6', 'Org_Chart_Paddy_2026_-_Draft_6_1.xlsx')`, `ON CONFLICT (label) DO NOTHING`;
- `INSERT … SELECT v.id, …` untuk setiap node dalam urutan CSV (`sort_order` = nomor baris), `ON CONFLICT (version_id, code) DO NOTHING`;
- alias dari `title_aliases.csv`, `ON CONFLICT (alias) DO NOTHING`.

Aturan pengisian:
- `needs_confirm` dan `status` mengikuti R5;
- `job_level`, `level_confidence`, dan `employment_type` NULL untuk `is_position = N`;
- `flags` = `string_to_array(flags, ';')`;
- `store_codes` dari `;`.

String di-escape `'` → `''`. Jalankan migrasi ke DB uji: `MIGRATE_DATABASE_URL=… go run ./cmd/migrate -apply`.

- [ ] **Step 4: Jalankan dan pastikan lulus**

Run: migrasi diterapkan **dua kali**; run kedua tanpa error dan tanpa perubahan. Lalu `go test ./internal/modules/hris/ -run TestOrgChartDraft6Seed -count=1`.
Expected: PASS.

- [ ] **Step 5: Commit** `feat(hris): skema bagan organisasi berversi + seed Draft 6 (EPIC-101)`. Sertakan `docs/hris/org-draft6/` dan `tools/orgchart/`.

### Task 2: Aturan bagan (domain)

**Files:**
- Create: `backend/internal/modules/hris/domain/org_chart.go`
- Test: `backend/internal/modules/hris/domain/org_chart_test.go`
- Create: `backend/internal/modules/hris/domain/testdata/org_draft6.csv` (salinan `paddy_org_positions_seed.csv`)

**Interfaces:**
- Produces:
  - `type OrgNode struct{ Code, ParentCode, Title string; IsPosition bool; Level, EmploymentType, Vendor, DivisionCode, Region, Status string; StoreCodes []string }`
  - `type OrgIssue struct{ Rule, Code, Message string }`
  - `func ValidateOrgChart(nodes []OrgNode) (errs, warns []OrgIssue)`: BR-01, 02, 03, 04, 05, 07, 09, 10 sesuai tabel BR di epic. Pesan BR-04 peringatan: `Level {child} sama dengan atasan {parentCode} ({parentLevel})`.
  - `type OrgAlias struct{ Title, Region string; NeedsReview bool }`
  - `type OrgEmployee struct{ ID, Name, NIP, Title, ManagerID, StoreCode string }`
  - `type OrgMatch struct{ EmployeeID, Name, NIP, Title, Canonical, NodeCode, Problem, ManagerProblem string; AliasNeedsReview bool }`
  - `type OrgGap struct{ Matches []OrgMatch; EmptyNodes []string; TitlesOutside map[string]int; StoresOutside, ChartStoresMissing []string }`
  - `func MatchOrgChart(nodes []OrgNode, emps []OrgEmployee, aliases map[string]OrgAlias, activeStores []string) OrgGap`: algoritma R9.
  - `func LoadOrgSeedCSV(r io.Reader) ([]OrgNode, error)`: hanya untuk tes dan generator. Kolom CSV klien; `is_position` `Y`/`N`.

- [ ] **Step 1: Tulis tes yang gagal**

- `TestValidateOrgChartDraft6`: `LoadOrgSeedCSV(testdata/org_draft6.csv)` → `errs` kosong; `warns` berisi tepat 3 dengan Code `HRG-004`, `HRG-010`, `MKT-004`, semuanya Rule `BR-04`.
- `TestValidateOrgChartRules` dengan fixture kecil, satu kasus per aturan. Pesan yang diharapkan persis:
  - dua root → `BR-01` `Bagan harus punya tepat satu puncak (ditemukan 2: A, B)`;
  - A→B→A → `BR-02` `Siklus garis lapor di A`;
  - induk `ZZZ` tidak ada → `BR-03` `Atasan ZZZ tidak ada di versi ini`;
  - anak L2 di bawah induk L4 → `BR-04` error `Level L2 lebih senior dari atasan X (L4)`;
  - Shopkeeper RTL tanpa toko → `BR-05` `Shopkeeper di Retail wajib punya kode toko`;
  - judul `Intern` → `BR-07` `Judul "Intern" bukan jabatan — jadikan tipe kepegawaian`;
  - grup dengan level → `BR-09` `Node grup tidak boleh punya level`;
  - OUTSOURCE tanpa vendor → `BR-10` `Posisi outsource wajib punya vendor`.
- `TestMatchOrgChart`. Fixture node:
  - `S1` Store Supervisor, region `Jawa Barat 1`;
  - `L1` Store Leader `{PPGA}`, induk S1;
  - `K1` Shopkeeper `{PPGA}`, induk L1;
  - `K2` Shopkeeper `{MPVJ}`, induk S1;
  - `K3` Shopkeeper `{PPGI}`, induk S1;
  - `F1` Finance Staff.

  Alias:
  - `shopkeeper staff` → Shopkeeper;
  - `store supervisor - jawa barat 1` → Store Supervisor/`Jawa Barat 1`;
  - `shopkeeper coordinator` → Store Leader (`NeedsReview`).

  Karyawan dan hasil yang diharapkan:
  - Dewi `Store Supervisor - Jawa Barat 1` → `S1`;
  - Lia `Shopkeeper Coordinator` toko PPGA, atasan Dewi → `L1`, `AliasNeedsReview`, tanpa `ManagerProblem`;
  - Ani `Shopkeeper Staff` toko PPGA, atasan Lia → `K1`;
  - Budi `Shopkeeper Staff` toko MPVJ, atasan Dewi → `K2`;
  - Cici `Shopkeeper Staff` tanpa toko → `Problem = "Cocok ke 3 posisi bagan — tidak bisa dipastikan"`;
  - Eko `Warehouse Staff` → `Problem = "Jabatan tidak ada di bagan"`, dan `TitlesOutside["Warehouse Staff"] = 1`;
  - Fani `Shopkeeper Staff` toko PPGA, atasan **Dewi** → `K1`, `ManagerProblem = "Atasan di bagan: Store Leader (L1); di data: Dewi"`.

  `EmptyNodes` = `[K3, F1]`. `activeStores` `[PPGA, MPVJ, PPGI, BXST]` → `StoresOutside = [BXST]`.

  Tambah satu kasus: node induk L1 kosong (tanpa Lia), Ani beratasan Dewi → tanpa `ManagerProblem` (Review Focus 2).

- [ ] **Step 2: Jalankan dan pastikan gagal**

Run: `cd backend && go test ./internal/modules/hris/domain/ -run 'TestValidateOrgChart|TestMatchOrgChart' -count=1`
Expected: FAIL (compile error).

- [ ] **Step 3: Implementasi** sesuai Interfaces dan R9.

- Pemegang node dihitung dari hasil cocok.
- Induk efektif melewati node grup (`IsPosition = false`).
- Induk tanpa pemegang naik ke leluhur pertama yang punya pemegang.
- `ManagerProblem` hanya muncul bila node atasan data ≠ induk efektif itu.

- [ ] **Step 4: Jalankan dan pastikan lulus**

Run: `cd backend && go test ./internal/modules/hris/domain/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit** `feat(hris): validasi bagan organisasi dan pencocokan karyawan (EPIC-101)`

### Task 3: API bagan + laporan selisih

**Files:**
- Create: `backend/internal/modules/hris/org_chart.go`
- Modify: `backend/internal/modules/hris/routes.go`
- Modify: `frontend/src/lib/go-routes.generated.json` (hasil `go run ./cmd/api -routes`)
- Test: `backend/internal/modules/hris/org_chart_integration_test.go`

**Interfaces:**
- Consumes: Task 1 (tabel), Task 2 (`ValidateOrgChart`, `MatchOrgChart`, tipe).
- Produces (semua `withMenu(iam.HrisOrganization)`; `version` = label, default versi terbaru menurut `imported_at`):
  - `GET /api/hris/org-chart/versions` → `data: [{id,label,status,source_file,imported_at,nodes}]`.
  - `GET /api/hris/org-chart/tree?version=&division=&region=` → `data: {version, nodes: [{code,parent_code,title,raw_title,is_position,job_level,level_confidence,employment_type,vendor,division_code,division_name,region,store_codes,status,needs_confirm,flags,holders:[{id,name,nip}]}]}`, urut `sort_order`. Filter menyaring node, bukan memangkas pohon. Versi tidak ada → 404 `Versi bagan tidak ditemukan`.
  - `GET /api/hris/org-chart/validation?version=` → `data: {errors:[OrgIssue], warnings:[OrgIssue]}`.
  - `GET /api/hris/org-chart/gap?version=` → `data: {summary:{employees, matched, ambiguous, outside, manager_diff, alias_review, empty_nodes, stores_outside}, matches, empty_nodes, titles_outside, stores_outside, chart_stores_missing}`.
  - `GET /api/hris/org-chart/gap?format=xlsx` → `Selisih Bagan {label}.xlsx` dengan sheet:
    - `Ringkasan`;
    - `Karyawan` (NIP, Nama, Jabatan HRIS, Jabatan Bagan, Kode Posisi, Masalah, Masalah Atasan, Alias Perlu Ditinjau);
    - `Posisi Kosong` (Kode, Jabatan, Divisi, Region, Toko, Status);
    - `Jabatan di Luar Bagan` (Jabatan, Jumlah Karyawan);
    - `Toko`.
- Data karyawan:
  - karyawan aktif;
  - `p.title` dari `hris.positions` via `job_title_id`;
  - `reporting_to`;
  - toko penempatan utama aktif (`is_primary`, `end_date IS NULL OR >= hari ini WIB`), kode dari aturan R6.

- [ ] **Step 1: Tulis tes integration yang gagal**

`TestOrgChartAPI` memakai harness HRIS dan staff dengan `iam.HrisOrganization`.

Asserts:
- versions memuat `Draft 6`, `nodes = 150`;
- `tree?division=RTL&region=Makassar` → 4 node: `RTL-053`, `RTL-054`, `RTL-055`, `RTL-056`;
- `tree?division=XXX` → 200, 0 node;
- `tree?version=Draft 99` → 404;
- validation → 0 error, 3 peringatan;
- tanpa menu → 403.

Untuk Review Focus 3, buat di dalam tx (DB uji tidak punya toko Paddy):
- branch berkode `ODOO-PLBM`;
- jabatan `Perfumer Staff` di `hris.positions`;
- karyawan aktif berjabatan itu dengan penempatan utama di branch tadi.

Alias `perfumer staff` → `Perfumer`, ditambah toko `PLBM`, harus membawa karyawan itu ke node `RTL-023`.

Assert gap: karyawan itu `node_code = RTL-023`; `holders` RTL-023 di tree memuat namanya; `gap?format=xlsx` 200 dengan sheet `Karyawan`.

- [ ] **Step 2: Jalankan dan pastikan gagal**

Run: `cd backend && go test ./internal/modules/hris/ -run TestOrgChartAPI -count=1`
Expected: FAIL (404).

- [ ] **Step 3: Implementasi** service + handler + rute, lalu generator rute.

- [ ] **Step 4: Jalankan dan pastikan lulus**

Run: `cd backend && go vet ./internal/modules/hris/ && go test ./internal/modules/hris/... -count=1`
Expected: PASS.

- [ ] **Step 5: Commit** `feat(hris): API bagan organisasi, validasi, dan laporan selisih (EPIC-101)`

### Task 4: Tab Bagan Draft 6 + Selisih, gate, staging

**Files:**
- Modify: `frontend/src/features/hris/org-chart/components/org-chart-page.tsx`: bungkus isi lama dalam tab `Struktur Aktual`, lalu tambah tab `Bagan Draft 6` dan `Selisih` (`@/components/ui/tabs`).
- Create: `frontend/src/features/hris/org-chart/components/chart-tree.tsx`
- Create: `frontend/src/features/hris/org-chart/components/chart-gap.tsx`
- Modify: `frontend/src/features/hris/org-chart/{api.ts,types.ts,query-keys.ts,queries.ts}`
- Test: `frontend/src/features/hris/org-chart/components/chart-tree.test.tsx`, `chart-gap.test.tsx`
- Modify: `docs/epics/EPIC-101-bagan-organisasi-draft6.md`, `docs/epics/README.md`

**Interfaces:**
- Consumes: endpoint Task 3.

- [ ] **Step 1: Tulis tes vitest yang gagal**

`chart-tree.test.tsx` dengan mock API 5 node (CEO, GM, RTL Manager, Store Supervisor Makassar, Store Leader `PPMR;PLMR` `planned` `needs_confirm`):
- pohon menampilkan `CEO`;
- tombol `Lipat`/`Buka` pada node yang punya anak (`aria-expanded`);
- node Store Leader menampilkan badge `Rencana` dan `Perlu konfirmasi` serta teks `PPMR · PLMR`;
- legenda memuat `Aktif`, `Rencana`, `Perlu konfirmasi`;
- memilih filter divisi `RTL` memanggil API dengan `division=RTL`.

`chart-gap.test.tsx`:
- ringkasan menampilkan angka dari `summary`;
- tautan `Unduh Excel` menunjuk `/api/hris/org-chart/gap?format=xlsx`;
- daftar 3 peringatan validasi tampil.

- [ ] **Step 2: Jalankan dan pastikan gagal**

Run: `cd frontend && npx vitest run src/features/hris/org-chart`
Expected: FAIL (modul belum ada).

- [ ] **Step 3: Implementasi**

- Isi tab lama tidak berubah.
- Pohon: daftar bersarang `role="tree"`/`treeitem`, bisa dilipat dengan Enter/Space. Di HP 360 px, indentasi dibatasi dan teks dibungkus, tanpa scroll horizontal halaman.
- Badge:
  - `active` → `Aktif`;
  - `planned` → `Rencana`;
  - `vacant` → `Kosong`;
  - `frozen` → `Dibekukan`;
  - `needs_confirm` → `Perlu konfirmasi`.
- Pemegang tampil sebagai nama (maks 3 + "dan N lainnya").

- [ ] **Step 4: Jalankan dan pastikan lulus**

Run: perintah Step 2, lalu `npx tsc --noEmit -p .` dan `npx eslint src/features/hris/org-chart`.
Expected: PASS, tsc 0, eslint bersih.

- [ ] **Step 5: Gate**

Run: `cd backend && gofmt -l ./internal && go vet ./... && go test ./internal/... -count=1 -p 4`
Expected: tidak ada berkas dari gofmt; PASS.

- [ ] **Step 6: Staging**

- Backup DB staging ke `/mnt/hdd1/backup/paddydev/`.
- `umask 022; bash tools/deploy-paddydev.sh` dari worktree. Migrasi seed ikut berjalan.

QA API (sesi uji sementara):
- versi Draft 6 berisi 150 node;
- validasi 0 error dan 3 peringatan;
- tree Makassar berisi 4 node;
- unduh `gap?format=xlsx`.

Kirim xlsx ke user sebagai laporan HR di `.share/org-draft6-<tanggal>/selisih-bagan.xlsx`. Pastikan tabel karyawan/jabatan/toko tidak berubah: bandingkan hitungan dan checksum `hris.employees`, `hris.positions`, `configuration.branches` sebelum dan sesudah deploy. Hapus sesi uji.

- [ ] **Step 7: Catat hasil**

Isi Automation Log EPIC-101 dengan ringkasan selisih (jumlah cocok, ambigu, di luar bagan, beda atasan). Set status `review`, tambah baris di README, lalu commit `docs: EPIC-101 hasil gate dan QA staging`.

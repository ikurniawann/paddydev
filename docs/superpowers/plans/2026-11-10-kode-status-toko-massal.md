# Kode & Status Toko Massal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** HR mengganti kode, nama, dan status banyak toko sekaligus lewat template Excel. Toko yang masih dipakai ditolak dinonaktifkan, dan cabang yang punya histori tidak bisa dihapus.

**Architecture:**
- Validasi file adalah fungsi murni di `hris/domain/branch_code_upload.go`.
- Service `hris/branch_code_import.go` membuat template, lalu menjalankan pratinjau → simpan → file error dalam satu transaksi. Polanya meniru impor Saldo Cuti (`leave_balance_import.go`).
- Penjaga hapus cabang ada di `configuration/settings/business.go`.
- Dialog frontend ditambahkan di halaman Penempatan Karyawan.

**Tech Stack:** Go (excelize lewat `platform/xlsx`, Postgres), Next.js + React Query (vitest).

**Spec:** `docs/epics/EPIC-100-kode-status-toko-massal.md` (ruling R1–R9).

## Global Constraints
- Ganti kode = UPDATE baris yang sama. Tidak ada branch baru dan tidak ada migrasi (R1, R9).
- Kode baru: trim + huruf besar, `^[A-Z0-9][A-Z0-9-]{1,29}$`, unik lintas semua branch (R6).
- Nonaktif ditolak bila toko masih punya penempatan aktif (`end_date IS NULL OR end_date >= hari ini WIB`) atau `employee_shift_days.work_date >= hari ini WIB` (R4).
- Pesan berbahasa Indonesia. Error per baris memakai `domain.UploadIssue{Line, Column, Message}`.
- Akses: template `withMenu(placementManagers)`, impor `withAction("update", placementManagers)`.
- Sesudah menambah rute, jalankan `go run ./cmd/api -routes > ../frontend/src/lib/go-routes.generated.json` dari `backend/`.
- Commit diakhiri `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Review Focus
1. Kode Saat Ini diketik huruf kecil atau berspasi (` odoo-ppga `) tetap dikenali (Task 1).
2. Baris yang tidak mengubah apa pun (template diunggah ulang apa adanya) tidak menghasilkan error, peringatan, atau audit (Task 1, Task 2).
3. Kode Baru sama dengan kode saat ini toko itu sendiri dianggap "tidak berubah", bukan "sudah dipakai" (Task 1).
4. Toko yang sudah nonaktif dengan Status Baru `Nonaktif` tidak dicek pemakaian (Event PVJ dengan penempatan aktif lama tetap lolos) (Task 1).
5. Dua HR menyimpan bersamaan, atau penempatan dibuat sesudah pratinjau: fingerprint atau cek ulang di bawah kunci menolak simpan (Task 2).

---

### Task 1: Validasi file (domain)

**Files:**
- Create: `backend/internal/modules/hris/domain/branch_code_upload.go`
- Test: `backend/internal/modules/hris/domain/branch_code_upload_test.go`

**Interfaces:**
- Produces:
  - `const BranchCodeSheet = "Data Toko"`;
  - `var BranchCodeHeaders = []string{"Kode Saat Ini", "Perusahaan", "Nama Saat Ini", "Status Saat Ini", "Kode Baru", "Nama Baru", "Status Baru"}`;
  - `type BranchRef struct{ ID, Code, Name string; Active bool }`;
  - `type BranchUsage struct{ Employees []string; FutureDays int }`, dengan `Employees` = nama karyawan dengan penempatan aktif;
  - `type BranchCodeChange struct{ Line int; ID, OldCode, NewCode, OldName, NewName string; OldActive, NewActive bool }`;
  - `func ValidateBranchCodeUpload(matrix [][]string, branches []BranchRef, usage map[string]BranchUsage) (changes []BranchCodeChange, errs, warns []UploadIssue, headerProblem string)`. Key `usage` = branch ID. Baris header ditemukan lewat `NormalizeHeader` = `kode saat ini`.
  - `func BranchNameForCode(oldCode, newCode, name string) string` (R3).

- [ ] **Step 1: Tulis tes yang gagal**

Fixture `branches`:
- `{b1, ODOO-PPGA, "PPGA - Paddy Playstore Backyard", true}`;
- `{b2, ODOO-PPBI, "PPBI - Paddy Playstore Rainforest", true}`;
- `{b3, ODOO-EVENT-PVJ, "Event PVJ", false}`;
- `{b4, TWEM, "TWEM - Paddy Playstore Tomorrow Lombok Epicentrum Mall", true}`.

`usage`:
- `b1: {Employees: ["Ani","Budi"], FutureDays: 4}`;
- `b3: {Employees: ["Citra"]}`.

`TestBranchNameForCode`:
- `("ODOO-PPGA","PPGX","PPGA - Paddy Playstore Backyard")` → `"PPGX - Paddy Playstore Backyard"`;
- `("ODOO-PPGA","PPGA",…)` → nama tetap;
- `("ODOO-EVENT-PVJ","EVPVJ","Event PVJ")` → `"Event PVJ"`.

`TestValidateBranchCodeUpload` (header di baris 1):
- baris 2 ` odoo-ppga `, Kode Baru `ppga` → change `{OldCode ODOO-PPGA, NewCode PPGA, NewName "PPGA - Paddy Playstore Backyard", NewActive true}` + warning `Baris 2: Kode ODOO-PPGA → PPGA: file Data Kerja/Jadwal yang diunduh sebelumnya memakai kode lama — unduh ulang`;
- baris 3 `ODOO-PPBI`, Status Baru `Nonaktif` → change `NewActive false`, tanpa error;
- baris 4 `ODOO-EVENT-PVJ`, Status Baru `nonaktif` → tidak ada change dan tidak ada error;
- baris 5 `TWEM`, Kode Baru `TWEM` → tidak ada change;
- baris 6 `XXXX` → `Baris 6, kolom Kode Saat Ini: Kode XXXX tidak ditemukan`.

`TestValidateBranchCodeUploadErrors`:
- Kode Baru `PP GA` → kolom Kode Baru `Kode baru hanya huruf besar, angka, dan tanda -, 2–30 karakter`;
- Kode Baru `TWEM` untuk ODOO-PPGA → `Kode TWEM sudah dipakai toko TWEM - Paddy Playstore Tomorrow Lombok Epicentrum Mall`;
- dua baris dengan Kode Baru `NEW1` → baris kedua `Kode baru NEW1 ganda di file (baris 2 & 3)`;
- Kode Saat Ini ganda → `Kode ODOO-PPBI ganda di file (baris 2 & 4)`;
- Status Baru `Tutup` → kolom Status Baru `Status harus Aktif atau Nonaktif`;
- ODOO-PPGA `Nonaktif` → kolom Status Baru `Toko ODOO-PPGA masih dipakai: 2 karyawan aktif (Ani, Budi) dan 4 jadwal mulai hari ini — pindahkan dulu lewat Penempatan`;
- tanpa kolom Kode Saat Ini → `headerProblem` = `Judul kolom "Kode Saat Ini" tidak ditemukan — pakai template dari halaman Penempatan`.

Daftar nama dipotong 5 dengan akhiran `, dan N lainnya`.

- [ ] **Step 2: Jalankan dan pastikan gagal**

Run: `cd backend && go test ./internal/modules/hris/domain/ -run 'TestBranch|TestValidateBranchCode' -count=1`
Expected: FAIL (compile error, fungsi belum ada).

- [ ] **Step 3: Implementasi** di `branch_code_upload.go`

- Pencarian memakai `strings.ToUpper(strings.TrimSpace(..))`.
- Pemakaian hanya dicek untuk branch yang aktif → nonaktif.
- Nama Baru diisi menang atas R3. Nama Baru lebih dari 150 karakter → `Nama baru maksimal 150 karakter`.
- Baris kosong dilewati.

- [ ] **Step 4: Jalankan dan pastikan lulus**

Run: `cd backend && go test ./internal/modules/hris/domain/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit** `feat(hris): validasi template kode & status toko (EPIC-100)`

### Task 2: Template + impor (service, HTTP, rute)

**Files:**
- Create: `backend/internal/modules/hris/branch_code_import.go`
- Modify: `backend/internal/modules/hris/routes.go` (dekat rute `/api/hris/branches`)
- Modify: `frontend/src/lib/go-routes.generated.json` (hasil generator)
- Test: `backend/internal/modules/hris/branch_code_import_integration_test.go`

**Interfaces:**
- Consumes: Task 1 (`ValidateBranchCodeUpload`, `BranchCodeSheet`, `BranchCodeHeaders`, `BranchRef`, `BranchUsage`, `BranchCodeChange`).
- Produces:
  - `GET /api/hris/branches/template` → xlsx `Kode & Status Toko.xlsx`. Isinya satu baris per branch, urut perusahaan lalu kode. Kolom 5–7 kosong; Status Saat Ini `Aktif`/`Nonaktif`.
  - `POST /api/hris/branches/import` (multipart `file`, `step` = `preview|commit|error_file`, `fingerprint`, `ack_warnings`).
    - Pratinjau `data`: `{summary: {rows, changed, code, name, deactivate, activate}, errors, warnings, fingerprint, changes: [{line, old_code, new_code, old_name, new_name, old_active, new_active}]}`.
    - Simpan: pesan `N toko diperbarui` atau `Tidak ada perubahan toko`.

- [ ] **Step 1: Tulis tes integration yang gagal**

`TestBranchCodeImport` memakai harness HRIS dan staff dengan `placementManagers`. Di dalam tx, buat 3 branch:
- A: berisi penempatan aktif;
- B: kosong;
- C: dengan satu baris `hris.attendance`.

Kode semuanya unik per run.

Assert:
- (a) Template 200, sheet `Data Toko` berisi kode A, B, C.
- (b) Pratinjau file `A→A2`, `B Nonaktif`, `C→C2` dengan Nama Baru `Toko C Baru`:
  - `errors` kosong, 2 peringatan kode, `summary.changed == 3`;
  - DB belum berubah.
- (c) Simpan tanpa `ack_warnings` → 400 `Centang "Saya sudah memeriksa peringatan" sebelum menyimpan`.
- (d) Simpan dengan `ack_warnings=true` dan fingerprint pratinjau → 200 `3 toko diperbarui`:
  - kode A = A2, B `is_active = false`, nama C = `Toko C Baru`;
  - baris attendance C masih menunjuk id C;
  - 3 baris `audit.audit_log` `branch.update`.
- (e) File `A2 Nonaktif` → error berisi `masih dipakai: 1 karyawan aktif`.
- (f) Simpan dengan fingerprint lama setelah kode diubah lewat SQL → 409 `Data toko sudah diubah sejak preview — unggah ulang file`.
- (g) `error_file` → xlsx dengan kolom `Keterangan Error`.

- [ ] **Step 2: Jalankan dan pastikan gagal**

Run: `cd backend && go test ./internal/modules/hris/ -run TestBranchCodeImport -count=1` (DB uji :55499)
Expected: FAIL (404 rute belum ada).

- [ ] **Step 3: Implementasi**

`BranchCodeTemplate(ctx)` dan `ImportBranchCodes(ctx, a *Actor, in branchCodeImportInput)` meniru `ImportLeaveBalances`:
- `.xlsx` saja; `xlsx.ParseMatrix(PreferSheet: BranchCodeSheet)`; `errorWorkbook`.
- Batas file 16 MB.
- Data branch dimuat lewat `SELECT … FROM configuration.branches`.
- Pemakaian per branch:
  - nama karyawan dari penempatan aktif yang join `hris.employees` aktif;
  - jumlah `employee_shift_days` dengan `work_date >= hari ini WIB`.
- Fingerprint = sha256(file + `id|code|name|is_active` branch yang berubah, diurutkan per id).

Saat simpan:
- `SELECT … FOR UPDATE` branch yang berubah, urut id;
- hitung ulang pemakaian untuk yang dinonaktifkan; bila masih dipakai → 400 dengan pesan Task 1;
- UPDATE `code`, `name`, `is_active`, `updated_at = now()`;
- `audit.Write` dengan Action `branch.update`, Entity `configuration.branches`, before/after `{code,name,is_active}`.

Tambahkan rute, lalu jalankan generator rute.

- [ ] **Step 4: Jalankan dan pastikan lulus**

Run: `cd backend && go vet ./internal/modules/hris/ && go test ./internal/modules/hris/ -run TestBranchCodeImport -count=1`
Expected: PASS.

- [ ] **Step 5: Commit** `feat(hris): template dan impor kode & status toko massal (EPIC-100)`

### Task 3: Penjaga hapus cabang (Settings → Business)

**Files:**
- Modify: `backend/internal/modules/configuration/settings/business.go` (`deleteBusinessEntity` ~baris 236)
- Test: `backend/internal/modules/configuration/settings/business_integration_test.go`

**Interfaces:**
- Produces: `func branchHistory(ctx, q database.Querier, id string) ([]string, error)`.
  - Mengembalikan nama tabel (`schema.table`) yang punya baris untuk branch itu. Datanya dari `pg_constraint`: FK dengan `confrelid = 'configuration.branches'::regclass`, kecuali `configuration.warehouses`.
  - Dicek per tabel dengan `SELECT EXISTS`.

- [ ] **Step 1: Tulis tes yang gagal**

`TestDeleteBranchWithHistory`:
- branch dengan satu baris `hris.attendance` → DELETE `/api/settings/business/branch/{id}` → 400, pesan diawali `Cabang ini tidak dapat dihapus karena sudah punya histori (hris.attendance)` dan memuat `nonaktifkan`;
- baris branch masih ada;
- branch baru tanpa data lain → DELETE 200.

- [ ] **Step 2: Jalankan dan pastikan gagal**

Run: `cd backend && go test ./internal/modules/configuration/settings/ -run TestDeleteBranchWithHistory -count=1`
Expected: FAIL (200, cabang terhapus).

- [ ] **Step 3: Implementasi**

Di `deleteBusinessEntity` untuk `typ == "branch"`: bila `branchHistory` tidak kosong, kembalikan `errors.New("Cabang ini tidak dapat dihapus karena sudah punya histori (" + maks 3 tabel + ") — nonaktifkan saja")`. Kata `tidak dapat` membuat `deleteErrors` mengembalikan 400.

- [ ] **Step 4: Jalankan dan pastikan lulus**

Run: `cd backend && go test ./internal/modules/configuration/settings/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit** `fix(settings): cabang yang punya histori tidak bisa dihapus, arahkan ke nonaktif (EPIC-100)`

### Task 4: Dialog "Kode & Status Toko" + gate + staging

**Files:**
- Create: `frontend/src/features/hris/placements/components/branch-code-dialog.tsx`
- Modify: `frontend/src/features/hris/placements/components/placements-page.tsx` (tombol di sebelah Lokasi Toko, baris ~203)
- Modify: `frontend/src/features/hris/placements/api.ts`: `downloadBranchCodeTemplate()`, `importBranchCodeFile(file, step, fingerprint?, ack?)`, `downloadBranchCodeErrors(file)`
- Test: `frontend/src/features/hris/placements/components/branch-code-dialog.test.tsx`
- Modify: `docs/epics/EPIC-100-kode-status-toko-massal.md`, `docs/epics/README.md`

**Interfaces:**
- Consumes: rute dan bentuk respons Task 2.

- [ ] **Step 1: Tulis tes vitest yang gagal**

Tiru `leave-balance-upload-dialog.test.tsx`. Mock API mengembalikan pratinjau dengan 1 error dan 1 peringatan. Assert:
- tombol **Simpan** nonaktif selama ada error;
- pada pratinjau tanpa error, tombol Simpan baru aktif setelah checkbox `Saya sudah memeriksa peringatan` dicentang;
- tabel perubahan menampilkan `ODOO-PPGA → PPGA` dan `Aktif → Nonaktif`;
- simpan memanggil `importBranchCodeFile(file, "commit", fingerprint, true)` lalu toast pesan server;
- query `branches` di-invalidate.

- [ ] **Step 2: Jalankan dan pastikan gagal**

Run: `cd frontend && npx vitest run src/features/hris/placements/components/branch-code-dialog.test.tsx`
Expected: FAIL (modul belum ada).

- [ ] **Step 3: Implementasi**

Dialog meniru `leave-balance-upload-dialog.tsx`. Tombol `Kode & Status Toko` (ikon `FileSpreadsheet`) hanya muncul bila `canManage`. HP 360 px: tabel perubahan bisa di-scroll horizontal di dalam kartu, halaman tidak ikut bergeser.

- [ ] **Step 4: Jalankan dan pastikan lulus**

Run: perintah Step 2, lalu `npx tsc --noEmit -p .` dan `npx eslint` pada berkas yang diubah.
Expected: PASS, tsc 0, eslint bersih.

- [ ] **Step 5: Gate**

Run: `cd backend && gofmt -l ./internal && go vet ./... && go test ./internal/... -count=1`
Expected: tidak ada berkas dari gofmt; PASS.

- [ ] **Step 6: Deploy staging**

- Backup DB staging ke `/mnt/hdd1/backup/paddydev/`.
- `umask 022; bash tools/deploy-paddydev.sh` dari worktree.

QA API (sesi uji sementara) untuk setiap butir Acceptance Criteria:
- template 56 baris;
- `ODOO-PPBI` Nonaktif tersimpan;
- `ODOO-PPGA` Nonaktif ditolak dengan nama karyawan;
- ganti kode `ODOO-TWLK` → `TWLK`, lalu kembalikan;
- hapus cabang berhistori ditolak.

Sesudahnya kembalikan semua perubahan uji: kode dan status semula, lewat file kedua. Hapus sesi uji.

- [ ] **Step 7: Catat hasil**

Isi Automation Log EPIC-100, set status `review`, tambah baris di `docs/epics/README.md`, lalu commit `docs: EPIC-100 hasil gate dan QA staging`.

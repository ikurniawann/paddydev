# Impor Karyawan — NIP Atasan di File yang Sama Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Impor Excel karyawan tidak lagi ditolak ketika NIP Atasan menunjuk karyawan baru di file yang sama atau diketik dengan tanda kutip, dan pesan "NIP sudah ada" di mode Tambah menjelaskan jalan keluarnya.

**Architecture:** Validasi (`domain.ValidateEmployeeImport`) mengenali NIP Atasan yang merupakan baris *create* di file yang sama dan menandainya di field baru `EmployeeImportRow.ManagerNIPNew`. Setelah semua baris diterapkan dalam transaksi impor, `Service.ImportEmployees` memasang `reporting_to` untuk baris yang ditandai, memakai id hasil insert. Tidak ada perubahan skema.

**Tech Stack:** Go (`backend/internal/modules/hris`), Postgres, tes unit dan integration Go.

**Spec:** `docs/epics/EPIC-098-impor-karyawan-nip-atasan.md` (ruling R1–R4).

## Global Constraints
- NIP tidak pernah dibuat otomatis (EPIC-081); baris baru tanpa NIP tetap ditolak.
- Mode Tambah tetap menolak NIP yang sudah ada (R3); yang diubah hanya teks pesannya.
- Satu error di file tetap memblokir commit seluruh file (perilaku yang sudah ada).
- Pesan berbahasa Indonesia, gaya pesan impor yang ada.
- Commit diakhiri baris `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Review Focus
1. Atasan baru berada di baris **sesudah** stafnya → `reporting_to` tetap terpasang (diuji di Task 2).
2. NIP Atasan menunjuk baris baru yang **punya error sendiri** → file tetap ditolak oleh error baris itu, tanpa error tambahan yang menyesatkan pada baris staf (diuji di Task 1).
3. Mode Timpa dengan NIP baru (bertindak sebagai *create*) dijadikan atasan oleh baris lain → diperlakukan sama seperti mode Tambah (diuji di Task 1).
4. NIP Atasan berisi notasi ilmiah Excel (`2,02E+08`) → ditolak dengan pesan format, bukan "tidak ditemukan" (diuji di Task 1).
5. Pratinjau (step `preview`) tidak menulis apa pun; pemasangan atasan hanya terjadi saat `commit` (diuji di Task 2).

---

### Task 1: Validasi NIP Atasan (domain)

**Files:**
- Modify: `backend/internal/modules/hris/domain/employee_import.go` (struct `EmployeeImportRow` ~baris 106; cabang `mode == "add" && exists` ~baris 204; cabang `k == "manager_nip"` ~baris 324)
- Test: `backend/internal/modules/hris/domain/employee_import_test.go`

**Interfaces:**
- Produces: field `EmployeeImportRow.ManagerNIPNew string`, berisi NIP atasan yang dibuat di file yang sama. Bila terisi, `Set["reporting_to"]` **tidak** diisi.
- Produces: teks pesan mode Tambah `NIP <nip> sudah dipakai <Nama>[ (nonaktif)] — mode Tambah hanya untuk karyawan baru; pakai mode Timpa untuk memperbarui`.

- [ ] **Step 1: Tulis tes yang gagal**

`TestValidateEmployeeImportManagerInSameFile`, mode `add`, header `hdr` ditambah kolom `"NIP Atasan"`. Baris yang dipakai:
- staf `202611011` dengan NIP Atasan `202611012`, ditaruh di baris **sebelum** atasannya;
- atasan `202611012`;
- staf kedua `202611013` dengan NIP Atasan `202611012`;
- staf `202611014` dengan NIP Atasan `202611099` (tidak ada di mana pun).

Assert:
- `errs` hanya berisi `"Baris 5, kolom NIP Atasan: NIP atasan 202611099 tidak ditemukan / tidak aktif"`;
- `out[0].ManagerNIPNew == "202611012"` dan `out[0].Set` tidak punya `reporting_to`;
- `out[2].ManagerNIPNew == "202611012"`.

`TestValidateEmployeeImportManagerNIPQuoted`: NIP Atasan `'202310037` (ada di `lk().ManagerNIP`) → tanpa error, dan `Set["reporting_to"] == "m1"`. Tambah satu baris dengan NIP Atasan `2,02E+08` → error memuat `format angka Excel`.

`TestValidateEmployeeImportManagerRowHasError`: atasan baru di file dengan Email kosong (error di barisnya sendiri), stafnya menunjuk atasan itu → baris staf tidak punya error NIP Atasan.

`TestValidateEmployeeImportReplaceNewManager`: mode `replace` dengan baris NIP baru `202611021` (dianggap *create*) yang dijadikan atasan baris NIP baru lain → `ManagerNIPNew` terisi, tanpa error.

Ubah ekspektasi di `TestValidateEmployeeImportAdd` dari `"Baris 3: NIP 202505272 sudah ada (mode Tambah hanya untuk karyawan baru)"` menjadi `"Baris 3: NIP 202505272 sudah dipakai Feti — mode Tambah hanya untuk karyawan baru; pakai mode Timpa untuk memperbarui"`. Tambah satu kasus NIP milik karyawan `Active:false` di `lk().ExistingNIP` yang pesannya memuat `(nonaktif)`.

- [ ] **Step 2: Jalankan dan pastikan gagal**

Run: `cd backend && go test ./internal/modules/hris/domain/ -run 'TestValidateEmployeeImport' -count=1`
Expected: FAIL. `ManagerNIPNew` belum didefinisikan (compile error), atau pesan masih teks lama.

- [ ] **Step 3: Implementasi di `ValidateEmployeeImport`**

- Sebelum loop utama, lakukan pre-pass atas `rows`. Hasilnya `newInFile map[string]int`, berisi NIP bersih (`CleanNIP`) → nomor baris, untuk baris yang akan menjadi *create*: NIP terisi dan tidak ada di `lk.ExistingNIP` (berlaku untuk mode `add` dan `replace`). Baris kosong dilewati, aturannya sama seperti loop utama.
- Di cabang `manager_nip`: bersihkan dengan `CleanNIP(val)`; pesan format dari `CleanNIP` dipakai sebagai error kolom. Cek self-manager terhadap NIP bersih. Cari di `lk.ManagerNIP[nipBersih]` → `reporting_to`; bila tidak ada tapi ada di `newInFile` → `row.ManagerNIPNew = nipBersih`. Selain itu tetap pakai error lama `NIP atasan <nip> tidak ditemukan / tidak aktif`.
- Pesan mode Tambah memakai `existing.Name` dan `existing.Active` sesuai teks di blok Interfaces.

- [ ] **Step 4: Jalankan dan pastikan lulus**

Run: `cd backend && go test ./internal/modules/hris/domain/ -count=1`
Expected: PASS (semua tes domain, termasuk tes impor lama).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/hris/domain/employee_import.go backend/internal/modules/hris/domain/employee_import_test.go
git commit -m "fix(hris): impor karyawan mengenali NIP Atasan baru di file yang sama dan bertanda kutip (EPIC-098)"
```

### Task 2: Pasang atasan setelah insert (service) + petunjuk template

**Files:**
- Modify: `backend/internal/modules/hris/employee_excel.go`: `ImportEmployees` (loop apply ~baris 560), `applyImportRow` (~baris 574), dan teks Petunjuk (~baris 248–257)
- Test: `backend/internal/modules/hris/employee_excel_integration_test.go`

**Interfaces:**
- Consumes: `EmployeeImportRow.ManagerNIPNew` (Task 1).
- Produces: `applyImportRow(...) (string, error)`, yang mengembalikan id karyawan (id baru untuk *create*, id lama untuk *update*).

- [ ] **Step 1: Tulis tes integration yang gagal**

`TestEmployeeImportManagerInSameFile` memakai harness seperti `TestEmployeeImportAddAndReplace`. Header ditambah `"NIP Atasan"`. File berisi dua baris NIP unik per run, **staf dulu, atasan kemudian**: staf (NIP Atasan = NIP atasan) dan atasan.

Assert:
- Preview: `errors` kosong dan `summary.create == 2`. Sesudah preview, `SELECT count(*) FROM hris.employees WHERE nip IN (…)` = 0.
- Commit: 200, pesan `2 karyawan ditambahkan, 0 diperbarui`.
- `SELECT m.nip FROM hris.employees e JOIN hris.employees m ON m.id = e.reporting_to WHERE e.nip = <staf>` = NIP atasan.
- Ada baris `audit.audit_log` untuk staf dengan `after->>'reporting_to'` = id atasan.

- [ ] **Step 2: Jalankan dan pastikan gagal**

Run: `cd backend && go test ./internal/modules/hris/ -run TestEmployeeImportManagerInSameFile -count=1` (butuh DB uji `paddydev-sdd-testdb` di port 55499 seperti tes integration lain).
Expected: FAIL; `reporting_to` staf NULL.

- [ ] **Step 3: Implementasi**

- `applyImportRow` mengembalikan id. Semua pemanggil diperbarui.
- Di `ImportEmployees` saat `commit`: kumpulkan `created := map[nip]id` dari baris `create`. Setelah loop apply, untuk setiap plan dengan `row.ManagerNIPNew != ""`, jalankan `UPDATE hris.employees SET reporting_to = $2, updated_at = now() WHERE id = $1` dengan `created[row.ManagerNIPNew]`. Lalu `writeEmployeeAudit(ctx, q, actor, "employee.update", id, row.Name, map[string]any{"reporting_to": nil}, map[string]any{"reporting_to": managerID})`. Kalau id atasan tidak ditemukan, kembalikan `httpx.BadRequest("Baris N: NIP atasan … gagal dibuat")`; transaksi batal.
- Teks Petunjuk untuk kolom NIP Atasan: `"NIP atasan langsung (boleh NIP karyawan baru di file yang sama)"`.

- [ ] **Step 4: Jalankan dan pastikan lulus**

Run: `cd backend && go test ./internal/modules/hris/ -run 'TestEmployeeImport|TestEmployeeExport' -count=1`
Expected: PASS (tes baru dan tes impor/ekspor lama).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/hris/employee_excel.go backend/internal/modules/hris/employee_excel_integration_test.go
git commit -m "fix(hris): impor karyawan memasang atasan yang dibuat di file yang sama (EPIC-098)"
```

### Task 3: Verifikasi staging + catatan epic

- [ ] **Step 1:** Jalankan gate: `cd backend && go vet ./internal/modules/hris/... && go test ./internal/modules/hris/... -count=1`. Expected: PASS.
- [ ] **Step 2:** Deploy branch ke staging (backup DB staging dulu, `umask 022`, `bash tools/deploy-paddydev.sh`). Unggah ulang file uji B1/B2/B3 lewat API impor di staging. Expected: B1 dan B2 masuk dengan atasan benar; B3 menampilkan pesan baru. Hapus data dan sesi uji sesudahnya.
- [ ] **Step 3:** Isi Automation Log EPIC-098 dengan hasil gate dan staging, set status `review`, lalu commit `docs: EPIC-098 hasil gate dan QA staging`.

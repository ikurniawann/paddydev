# Pengisian Massal Data Kerja (EPIC-086) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Satu perintah ops (`audit` / `template` / `preview` / `apply`) untuk mengisi penempatan, tipe jadwal, pola mingguan, departemen dan jabatan banyak karyawan dari satu file Excel, dengan pratinjau, backup, transaksi tunggal dan audit.

**Architecture:**
- Validasi murni di `hris/domain/work_data_upload.go`.
- Baca dan tulis DB di `hris/work_data.go`. Fungsi yang diekspor dipanggil oleh `backend/cmd/hris-data-kerja`. Logika yang sudah ada dipakai ulang: `changeScheduleTypeTx`, `SaveShiftPattern`, `periodContaining`, `writeEmployeeAudit`, `xlsx` template/reader.
- Runner bash menyalin binary statis ke container `paddydev-api` dan menjalankannya di sana.

**Tech Stack:** Go (hris, platform/xlsx, pgx), bash, docker.

**Spec:** `docs/epics/EPIC-086-data-kerja-massal.md` (R1–R5).

## Global Constraints
- Nama sheet & header persis:
  - Sheet `Data` berkolom `NIP | Nama | Departemen Saat Ini | Jabatan Saat Ini | Penempatan Saat Ini | Tipe Saat Ini | Yang Perlu Dilengkapi | Penempatan Utama (Kode Toko) | Tipe Jadwal | Senin | Selasa | Rabu | Kamis | Jumat | Sabtu | Minggu | Berlaku Mulai | Departemen Baru | Jabatan Baru | Catatan`.
  - Sheet referensi: `Petunjuk`, `Kode Toko`, `Kode Shift`, `Departemen`, `Jabatan`. Sheet `Info` menyimpan versi template `data-kerja-v1`.
- Kode libur `LIBUR`. Dua shift: `KODE1+KODE2`. Tipe: `Shift` atau `Non-Shift`.
- Actor audit: `actor_name = 'Sistem (unggah massal data kerja)'`.
  - Aksi ringkasan: `hris.bulk_work_data`.
  - Aksi per karyawan memakai aksi yang sudah ada: `employee.schedule_type`, `employee.update`. Aksi baru `employee.shift_pattern` dan `employee.placement` bila belum ada.
- "Hari ini" = `domain.TodayWIB(now)`. Production hanya lewat runner, dengan izin user dan HR; tanpa akun atau sesi uji.
- Tes integrasi memakai harness hris (`TEST_DATABASE_URL=…/paddy_test_e084`, `go test -p 1`).
- Trailer commit: `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Review Focus
1. **Penempatan utama baru yang tanggalnya bertabrakan** dengan penempatan utama lama yang dimulai pada/sesudah Berlaku Mulai: penempatan lama dipotong atau dihapus tanpa melanggar EXCLUDE `one_primary`/`no_overlap`. Diuji di Task 3.
2. **Ubah ke Non-Shift**: pola tetap departemen terpasang (lewat `changeScheduleTypeTx`), dan sel Senin–Minggu dilarang. Diuji di Task 1 dan Task 3.
3. **File diubah setelah pratinjau**: `apply` dengan sha lama ditolak. Diuji di Task 4.
4. **Satu baris error di tengah file**: `apply` menolak seluruh file, tanpa ada yang tertulis. Diuji di Task 3.
5. **Sel tanggal Excel berupa angka serial**: Berlaku Mulai terbaca sebagai YYYY-MM-DD (reader xlsx sudah mengonversi tanggal). Diuji di Task 3 lewat file yang dibuat oleh writer template.

---

### Task 1: Validasi baris (domain murni)

**Files:**
- Create: `backend/internal/modules/hris/domain/work_data_upload.go`
- Test: `backend/internal/modules/hris/domain/work_data_upload_test.go`

**Interfaces:**
- Produces:
  - `type WorkDataMasters struct { Employees map[string]WorkDataEmployee /*by NIP*/; Branches map[string]string /*kode→id, aktif*/; Shifts map[string]UploadShift /*kode aktif*/; Departments, Positions map[string]string /*nama lower→id*/; Today string; ClosedFrom func(date string) bool }`
  - `type WorkDataEmployee struct { ID, NIP, Name, ScheduleType string; Active bool; HasRosterFrom func(date string) bool }`
  - `type WorkDataRow struct { Line int; EmployeeID, NIP, Name string; BranchID *string; ScheduleType *string; Pattern *[7]PatternCell; From string; DepartmentID, PositionID *string }`
  - `type PatternCell struct { Shift1, Shift2 *string }` (nil Shift1 = LIBUR)
  - `func ValidateWorkData(header []string, rows [][]string, firstLine int, m WorkDataMasters) (valid []WorkDataRow, errs, warns []UploadIssue)` — reuse the existing `UploadIssue` type from schedule_upload.go.

- [ ] **Step 1: Failing tests** `TestValidateWorkData`. Fixture: masters with 3 employees (A shift aktif, B non_shift aktif, C nonaktif), toko `BXST`, shifts `SPG1` 08–16 and `SPG3` 13:30–21:30, Today `2026-11-03`, ClosedFrom(d) = `d <= "2026-10-31"`.

  | Input | Hasil |
  |---|---|
  | A: toko BXST, Shift, SPG1×6 + LIBUR, from 2026-11-10 | valid; Pattern[6].Shift1 nil |
  | A: kode toko `ZZZ` | error "Baris N: kode toko ZZZ tidak dikenal/nonaktif" |
  | B: Non-Shift dengan Senin `SPG1` | error "Baris N: karyawan Non-Shift mengikuti jadwal tetap departemen — kosongkan Senin–Minggu" |
  | A: pola hanya 5 hari terisi | error "Baris N: pola mingguan harus diisi 7 hari (Senin–Minggu)" |
  | A: Senin `SPG1+SPG3` (tumpang tindih) | error berisi pesan `TwoShiftIssue` |
  | A: from `2026-11-01` | error "Baris N: Berlaku Mulai sudah lewat (01-11-2026)" |
  | A: from `2026-10-20` | error "Baris N: Berlaku Mulai sudah lewat (20-10-2026)" (lampau mendahului closing) |
  | A: penempatan diisi tanpa from | error "Baris N: Berlaku Mulai wajib diisi bila ada perubahan" |
  | C | error "Baris N: NIP X nonaktif" |
  | NIP ganda | error "Baris N: NIP X ganda (juga di baris M)" |
  | A: SPG1×7 tanpa LIBUR | warning "Baris N: pola tanpa hari LIBUR (masuk 7 hari)" |
  | header salah | satu error "File bukan template data kerja (kolom tidak sesuai)" |
  | baris semua kosong kecuali NIP | diabaikan (tidak ada perubahan), tidak masuk valid |

- [ ] **Step 2: Run** `go test ./internal/modules/hris/domain/ -run TestValidateWorkData`. Expect FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run.** Expect PASS.
- [ ] **Step 5: Commit** `feat(hris): validasi file data kerja massal (EPIC-086)`.

### Task 2: Audit & template dari DB

**Files:**
- Create: `backend/internal/modules/hris/work_data.go` (bagian audit/template/master loader)
- Test: `backend/internal/modules/hris/work_data_integration_test.go`

**Interfaces:**
- Produces:
  - `func WorkDataAudit(ctx context.Context, q database.Querier, today string) ([]WorkDataGap, error)` — kolom seperti tabel audit di epic, per tipe.
  - `func WorkDataTemplate(ctx context.Context, q database.Querier, today string) ([]byte, error)` — writer `xlsx` yang sama dengan `ScheduleTemplate` (TemplateSheet + dropdown + Protect).
  - `func loadWorkDataMasters(ctx, q, today) (domain.WorkDataMasters, error)`.

- [ ] **Step 1: Failing test** `TestWorkDataAuditAndTemplate`. Seed:
  - 1 karyawan Shift tanpa jadwal;
  - 1 Non-Shift dengan pola;
  - 1 tanpa penempatan;
  - 1 nonaktif.

  Expect:
  - audit menghitung masing-masing tepat;
  - template:
    - sheet `Data` hanya berisi karyawan aktif yang belum lengkap;
    - header persis sesuai Global Constraints;
    - sheet `Kode Shift` memuat `LIBUR`;
    - `Kode Toko` hanya memuat cabang aktif;
    - dibaca balik dengan `xlsx.ParseMatrix` menghasilkan NIP yang sama.
- [ ] **Step 2: Run** `go test -p 1 ./internal/modules/hris/ -run TestWorkDataAuditAndTemplate`. Expect FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run.** Expect PASS.
- [ ] **Step 5: Commit** `feat(hris): audit dan template data kerja massal dari master live (EPIC-086)`.

### Task 3: Pratinjau & terapkan

**Files:**
- Modify: `backend/internal/modules/hris/work_data.go`
- Test: `backend/internal/modules/hris/work_data_integration_test.go`

**Interfaces:**
- Produces:
  - `type WorkDataReport struct { SHA string; Valid, Errors, Warnings int; Issues []domain.UploadIssue; Changes []string }`
  - `func WorkDataPreview(ctx, q database.Querier, file []byte, today string) (*WorkDataReport, error)`
  - `func WorkDataApply(ctx, db database.Pool /*or the hris Repository constructor*/, file []byte, today, sha string) (*WorkDataReport, error)` — menolak bila `sha` ≠ sha256(file) atau ada error; satu transaksi.

- [ ] **Step 1: Failing test** `TestWorkDataApply`. File dibuat dari `WorkDataTemplate` lalu diisi lewat writer xlsx di tes; Berlaku Mulai diisi sebagai tanggal (Review Focus 5). Expect:
  - pratinjau 0 error;
  - apply:
    - karyawan Shift mendapat 7 baris `employee_shifts` mulai tanggal itu;
    - penempatan utama baru, dan yang lama ditutup H-1 (Review Focus 1: penempatan lama yang dimulai ≥ tanggal itu dihapus);
    - karyawan yang diubah ke Non-Shift mendapat pola tetap departemen (Review Focus 2);
    - departemen dan jabatan baru tersimpan;
    - audit per perubahan + 1 ringkasan `hris.bulk_work_data` dengan sha;
  - file dengan 1 baris error → `apply` error dan tidak ada baris yang berubah (Review Focus 4).
- [ ] **Step 2: Run** `go test -p 1 ./internal/modules/hris/ -run TestWorkDataApply`. Expect FAIL.
- [ ] **Step 3: Implement.** Penempatan utama memakai pola SQL yang sama dengan `BulkPlacement` / `CreatePlacement` (`placements.go`). Bila ada fungsi tingkat repo yang bisa dipakai ulang, pakai itu.
- [ ] **Step 4: Run** the test, then `go test -p 1 ./internal/modules/hris/`. Expect PASS.
- [ ] **Step 5: Commit** `feat(hris): pratinjau dan terapkan data kerja massal dalam satu transaksi (EPIC-086)`.

### Task 4: Perintah & runner

**Files:**
- Create:
  - `backend/cmd/hris-data-kerja/main.go`: flags `-mode audit|template|preview|apply`, `-file`, `-out`, `-sha`; DB dari `DATABASE_URL`; laporan Markdown ke stdout.
  - `tools/ops/data-kerja.sh`: `ENV=staging|prod`, mode sama. Langkahnya:
    - build `CGO_ENABLED=0 GOOS=linux`;
    - `docker cp` binary dan file ke `paddydev-api:/tmp/`;
    - `docker exec`;
    - salin hasil balik.

    Untuk prod lewat `ssh paddy-prod`. `apply` mewajibkan `DATA_KERJA_APPROVED=yes` dan menjalankan `pg_dump` (umask 077) ke `~/backups/data-kerja-<stamp>.dump` sebelum `apply`.
- Test:
  - `backend/cmd/hris-data-kerja/main_test.go`: flag tanpa `-sha` untuk `apply` → exit non-zero.
  - `tools/ops/data-kerja.test.sh`: `apply` tanpa `DATA_KERJA_APPROVED` → exit non-zero sebelum menyentuh docker (Review Focus 3 lewat `-sha` salah di tes Task 3 atau tes ini).

- [ ] Steps: failing test → implement → run `go test ./cmd/hris-data-kerja/` + `bash tools/ops/data-kerja.test.sh` → commit `ops(hris): perintah dan runner data kerja massal (EPIC-086)`.

### Task 5: Gladi staging, review akhir, PR

- [ ] Final review (reviewer terpisah) → perbaikan.
- [ ] Backup staging, lalu deploy branch.
- [ ] Jalankan di staging:
  1. `ENV=staging tools/ops/data-kerja.sh audit` (catat angka);
  2. `template`;
  3. isi contoh untuk 5 karyawan:
     - 3 Shift dengan pola;
     - 1 pindah toko;
     - 1 baris salah kode;
  4. `preview` (1 error);
  5. perbaiki;
  6. `preview` (0 error);
  7. `apply` dengan sha;
  8. `audit` ulang.
- [ ] Cek UI staging: Jadwal Kerja dan Penempatan Karyawan menampilkan hasilnya; kalender ESS karyawan contoh menampilkan shift.
- [ ] Kembalikan data contoh dari backup bila perlu. Data contoh adalah karyawan staging nyata (salinan prod), jadi restore tabel yang terdampak dari dump atau hapus baris yang dibuat. Catat caranya.
- [ ] Dokumen:
  - Automation Log;
  - `docs/epics/README.md`;
  - petunjuk ops di epic.
- [ ] PR (bertumpuk di atas #70). Merge hanya dengan izin user.

### Task 6: Production (berizin)

- [ ] Jalankan `ENV=prod audit` dan `template` (baca saja). Kirim template dan angka audit ke user; ini menggantikan template 03-11.
- [ ] Setelah file terisi: `preview` di prod, lalu kirim laporan.
- [ ] Minta izin tertulis user dan HR beserta jam eksekusi.
- [ ] `apply` (backup otomatis), lalu `audit` ulang.
- [ ] Salin backup ke `~/Desktop/paddydev-backups/`. Catat di epic log dan memory.

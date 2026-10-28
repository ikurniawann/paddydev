# Penempatan & Jadwal Shift Massal (EPIC-070) — Rencana Implementasi

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** HR bisa menempatkan dan menjadwalkan banyak karyawan sekaligus (dan tetap per orang) dari halaman yang jelas, berfilter, berhalaman, dan langsung ter-update.

**Architecture:**
- Backend Go (`backend/internal/modules/hris`):
  - endpoint baru untuk pencarian penempatan berhalaman dan aksi massal pindah/akhiri dengan pratinjau;
  - endpoint penyimpanan jadwal per kotak yang memakai ulang aturan validasi unggah Excel;
  - jumlah pemakai shift.
- Frontend Next (`frontend/src/features/hris/{placements,schedules,shifts}`): halaman ditulis ulang di atas endpoint itu, dengan invalidasi react-query lintas halaman.
- Tidak ada migrasi DB.

**Tech Stack:** Go 1.26 + pgx, PostgreSQL 16; Next.js (lihat `frontend/AGENTS.md`), React Query, shadcn/ui, vitest.

**Spec:** `docs/epics/EPIC-070-penempatan-jadwal-massal.md` (Goal, Ruling R1–R6, Acceptance Criteria).

## Global Constraints

- Semua teks UI berbahasa Indonesia, kalimat biasa (bukan Title Case), tanpa istilah teknis.
- Tidak ada migrasi skema. Endpoint lama (`GET /api/hris/employee-placements`, `POST /api/hris/employee-placements/bulk` tanpa `mode`, `POST /api/hris/schedules/import`) tetap berperilaku sama.
- Hak akses:
  - penempatan memakai `placementManagers` (seperti sekarang);
  - baca jadwal memakai `kepegawaian`;
  - tulis jadwal memakai `h.withAction("update", scheduleManagers, …)`;
  - timpa/hapus baris jadwal per tanggal memakai `clearGuard` (hak hapus Jadwal Kerja + alasan; R3).
- Tanggal = WIB (`domain.TodayWIB(s.now())`/`s.clock()`). Frontend memakai `todayWib()`.
- Setiap perubahan data massal: satu transaksi (`s.repo.InTx`) + audit per karyawan (`writeEmployeeAudit`).
- Gerbang lokal (wajib hijau sebelum commit tiap task):
  - Go: `export PATH=$HOME/.local/go/bin:$PATH; cd backend && go vet ./... && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test_e070?sslmode=disable' go test ./internal/modules/hris/... -count=1`
  - Frontend: `cd frontend && pnpm exec tsc --noEmit -p . && pnpm lint && pnpm test -- --run <folder fitur>`
- Jangan mengubah file di luar cakupan task kecuali disebut.

## Review Focus

- **Karyawan dipilih lintas halaman lalu filter diganti.** Yang diharapkan: pilihan tetap, bilah aksi menampilkan jumlah total. "Pilih semua hasil filter" memakai endpoint ids (maks 2000) dan memberi tahu bila terpotong.
- **Pindah massal dengan karyawan yang sudah di toko tujuan atau punya penempatan masa depan.** Yang diharapkan: dilewati dengan alasan per orang di pratinjau, sisanya tetap diproses dalam satu transaksi.
- **Pola mingguan melewati tanggal lampau, periode closing, atau hari karyawan tidak ditempatkan.** Yang diharapkan: tanggal lampau/closing → error pratinjau yang jelas. Hari tidak ditempatkan dilewati diam-diam oleh klien (kotak "-" tidak dikirim), bukan error.
- **Dua HR mengubah grid yang sama.** Yang diharapkan: commit memakai fingerprint; bila berubah → 409 "Jadwal sudah diubah orang lain…", grid dimuat ulang.
- **Shift diubah jamnya di Shift Kerja.** Yang diharapkan: grid Jadwal Kerja, dropdown shift di profil, dan Shift Tim ter-update tanpa tunggu 2 menit.

---

### Task 1: Pencarian penempatan berhalaman + ids (Go)

**Files:**
- Modify: `backend/internal/modules/hris/placements.go`, `backend/internal/modules/hris/routes.go`
- Test: `backend/internal/modules/hris/placements_search_integration_test.go`

**Interfaces — Produces:**
- `GET /api/hris/employee-placements/search?q&branch_id&department_id&position_id&status&page&page_size&include_ended`
  - `status` ∈ `""|unplaced|no_schedule|multi|placed`;
  - `page` ≥1 (default 1), `page_size` ∈ 25|50|100 (default 25; nilai lain → 400 "Jumlah per halaman harus 25, 50, atau 100");
  - urutan `full_name`.
  - Respons `data`:
    ```
    { rows: Row[], total: int, page: int, page_size: int,
      counts: { all, unplaced, no_schedule, multi } }
    ```
    `counts` dihitung dengan filter q/branch/department/position yang sama tetapi tanpa `status`.
  - Row = kolom overview lama (`employee_id, full_name, nip, photo_url, department_name, position_title, placements`) + `department_id, position_id, schedule_type, has_schedule (bool, R5), schedule_until (date|null: tanggal terakhir ≥ hari ini yang punya baris per tanggal; null bila tidak ada)`.
- `GET /api/hris/employee-placements/ids?<filter yang sama tanpa page>` → `data: { ids: string[], total: int, truncated: bool }`, maksimal 2000 id.
- Kedua route: `h.withMenu(placementManagers, …)`. `department_id`/`position_id`/`branch_id` bukan UUID → 400.

- [ ] **Step 1:** Tulis `TestPlacementSearch`. Data: 3 toko, 6 karyawan dengan variasi (tanpa penempatan, multi-toko, Non-Shift, punya baris jadwal hari ini+2, punya pola mingguan berjalan, tanpa jadwal), 2 departemen. Assert:
  - `status=unplaced` total 1;
  - `status=no_schedule` hanya karyawan Shift tanpa jadwal 7 hari;
  - `status=multi` 1;
  - `counts` konsisten;
  - `page_size=25&page=2` kosong dengan total benar;
  - `page_size=30` → 400;
  - `q` cocok NIP;
  - `department_id` menyaring;
  - `has_schedule`/`schedule_until` benar;
  - non-HR → 403.
- [ ] **Step 2:** Tulis `TestPlacementIDs`: ids = semua hasil filter, `truncated` false.
- [ ] **Step 3:** Jalankan tes, pastikan gagal (404 route).
- [ ] **Step 4:** Implementasi.
  - `placementFilter` ditambah `Department, Position, Status string`.
  - Builder WHERE dipakai bersama oleh search, ids, dan counts. Counts memakai satu query dengan `count(*) FILTER (WHERE …)`.
  - `has_schedule` = `EXISTS (SELECT 1 FROM hris.v_employee_schedule es WHERE es.employee_id = e.id AND es.effective_from <= $today::date + 6 AND (es.effective_to IS NULL OR es.effective_to >= $today::date))`, hanya bermakna untuk `schedule_type='shift'`.
  - Pagination `LIMIT/OFFSET`.
- [ ] **Step 5:** Gerbang Go hijau, lalu commit `feat(hris): pencarian penempatan berhalaman + filter status (EPIC-070)`.

### Task 2: Aksi massal pindah / tambah / akhiri dengan pratinjau (Go)

**Files:**
- Modify: `backend/internal/modules/hris/placements.go`, `routes.go`
- Test: `backend/internal/modules/hris/placements_bulk_integration_test.go`

**Interfaces — Produces:**
- `POST /api/hris/employee-placements/bulk`, body:
  ```
  { branch_id, employee_ids[≤500], mode?: "add"|"move", step?: "preview"|"commit",
    is_primary?, start_date?, end_date?, notes? }
  ```
  - Tanpa `mode` → perilaku lama persis (kompatibel, R6).
  - Dengan `mode`: respons
    ```
    data: { step, created: int, moved: int, skipped: int,
            results: [{ employee_id, name, nip, outcome: "created"|"moved"|"skipped",
                        reason: string|null, ended: [{branch_name, end_date}] }] }
    ```
    Message: `"N dipindahkan, M ditambahkan, K dilewati"` (bagian 0 dihilangkan).
- `mode:"add"`: menambah penempatan (utama bila `is_primary`).
  - Lewati bila sudah ada penempatan di toko itu yang tumpang tindih (alasan "Sudah ditempatkan di toko ini").
- `mode:"move"` (R4): `is_primary` dipaksa true, `start` = `start_date` atau hari ini.
  - Penempatan berjalan (`end_date IS NULL OR end_date >= start`) yang `start_date < start` → `end_date = start-1`.
  - Lewati karyawan bila ada penempatan berjalan di toko tujuan yang mencakup `start` ("Sudah di toko ini"), atau ada penempatan lain dengan `start_date >= start` ("Punya penempatan mulai {tgl} — ubah lewat Kelola").
- Commit = satu transaksi untuk semua. Preview = hitung tanpa menulis (transaksi di-rollback).
- Audit per karyawan diproses: `placement.bulk_move` / `placement.bulk_add` (before: daftar toko berjalan, after: toko baru + start).
- `POST /api/hris/employee-placements/bulk-end`: `{ employee_ids[≤500], end_date, branch_id?, step }`.
  - Mengakhiri penempatan berjalan (semua, atau hanya di `branch_id`): `end_date` = nilai.
  - Penempatan yang `start_date > end_date` → karyawan dilewati ("Penempatan mulai sesudah tanggal akhir").
  - Respons sama bentuknya (`outcome: "ended"|"skipped"`, field `ended`), audit `placement.bulk_end`.

- [ ] **Step 1:** Tulis `TestBulkPlacementMovePreviewCommit`:
  - 4 karyawan: A di toko X sejak 2026-01-01; B tanpa penempatan; C sudah di toko tujuan Y; D punya penempatan masa depan.
  - Preview move ke Y mulai hari ini → A moved (ended X tgl kemarin), B created, C skipped, D skipped; DB tidak berubah.
  - Commit → hasil sama; X milik A berakhir kemarin; A punya tepat 1 primary berjalan di Y; audit 2 baris.
- [ ] **Step 2:** Tulis `TestBulkPlacementLegacyUnchanged`: body tanpa `mode` → respons `{created, skipped}` dan pesan lama.
- [ ] **Step 3:** Tulis `TestBulkPlacementEnd`: akhiri di toko X → berakhir; karyawan dengan start > end_date dilewati.
- [ ] **Step 4:** Implementasi.
  - Logika per karyawan dibuat sebagai fungsi murni di `domain` (`domain.PlanPlacementMove(existing []PlacementSpan, target, start string) MoveDecision`) dengan unit test tabel `TestPlanPlacementMove`. Tujuannya agar aturan R4 teruji tanpa DB.
- [ ] **Step 5:** Gerbang Go hijau, lalu commit `feat(hris): penempatan massal pindah/tambah/akhiri dengan pratinjau (EPIC-070)`.

### Task 3: Simpan jadwal per kotak (Go)

**Files:**
- Modify: `backend/internal/modules/hris/domain/schedule_upload.go`, `backend/internal/modules/hris/schedule_days.go`, `routes.go`
- Test: `backend/internal/modules/hris/domain/schedule_cells_test.go`, `backend/internal/modules/hris/schedule_cells_integration_test.go`

**Interfaces — Produces:**
- `domain.ScheduleCellInput{ EmployeeID, Date, Code string }`. Arti `Code`:
  - kode shift;
  - `"L"`/`"OFF"` = libur;
  - `""` = hapus baris per tanggal, kembali ke pola mingguan/kosong.
- `domain.ValidateScheduleCells(c ScheduleUploadContext, cells []ScheduleCellInput) ScheduleUploadResult`.
  - `ScheduleAssignment` diberi field `Clear bool`.
  - `UploadIssue.Line` = 0; `Column` = header tanggal; `Name` = nama karyawan.
  - Aturan per kotak sama dengan `ValidateScheduleUpload`: tanggal lewat, periode closing, karyawan tidak dikenal/nonaktif/Non-Shift, tidak ditempatkan di cabang pada tanggal itu, kode tidak ada/nonaktif → error.
  - Peringatan cuti, jeda, beruntun, dan libur nasional dihitung terhadap jadwal hasil akhir. Jadwal hasil akhir = sel yang diedit, ditimpa di atas jadwal berlaku untuk tanggal di sekitarnya. Kirimkan kode berlaku lewat context field baru `Current map[employeeID]map[date]code` yang diisi handler dengan `resolveCells`.
  - Refactor bersama: ekstrak helper aturan dari `ValidateScheduleUpload` agar dipakai keduanya. `ValidateScheduleUpload` harus tetap lolos semua tes lama.
- `POST /api/hris/schedules/cells` (`withAction("update", scheduleManagers)`), body JSON:
  ```
  { branch_id, step: "preview"|"commit", fingerprint?, ack_warnings?, reason?,
    cells: [{employee_id, date, code}] ≤ 6000 }
  ```
  - Tanggal: 1..62 hari unik, rentang min..max ≤ 62 hari, selain itu 400.
  - Respons preview:
    ```
    data: { summary{employees,new,changed,same,cleared}, errors, warnings, fingerprint, changes(≤500), replaced_days }
    ```
  - Commit:
    - error → 400;
    - peringatan tanpa ack → 400;
    - fingerprint beda → 409;
    - `clearGuard.check(replaced + cleared_existing)`;
    - DELETE baris (employee, date) yang dikirim, lalu INSERT yang bukan Clear (satu `batch_id`);
    - audit `schedule.cells_update` per karyawan + guard audit `hris.schedule.replace`;
    - pesan `"Jadwal N karyawan disimpan"`.
- Kontekstual: `uploadContext` dipakai ulang. Tambah map karyawan per ID (bukan NIP) untuk cells; tanggal = daftar tanggal sel.

- [ ] **Step 1:** Unit test `TestValidateScheduleCells` (tabel):
  - past date;
  - closed;
  - not placed;
  - unknown code;
  - inactive code;
  - Non-Shift;
  - clear valid;
  - OFF;
  - gap warning terhadap kotak tetangga yang tidak diedit (dari `Current`);
  - streak 7 hari gabungan edit + current;
  - holiday warning.
- [ ] **Step 2:** Integration `TestScheduleCellsPreviewCommit`:
  - preview → commit → grid menampilkan kode baru dengan source "day";
  - clear mengembalikan source "weekly";
  - fingerprint basi → 409;
  - menimpa baris lama tanpa hak hapus → 403 errScheduleNeedsDelete;
  - dengan hak + alasan → 200 + audit.
- [ ] **Step 3:** Pastikan tes lama `schedule_days_*` tetap hijau.
- [ ] **Step 4:** Implementasi, gerbang hijau, lalu commit `feat(hris): simpan jadwal per kotak dengan aturan unggah yang sama (EPIC-070)`.

### Task 4: Pemakaian shift (Go)

**Files:**
- Modify: shift list query (`ListShifts` repo + `listShifts` handler), `overtime.go` bila perlu
- Test: `backend/internal/modules/hris/shift_usage_integration_test.go`

**Interfaces — Produces:**
- `GET /api/hris/shifts?with_usage=true` → setiap baris ditambah `employees_using: int`.
  - Nilainya = jumlah karyawan aktif yang memakai shift di baris per tanggal ≥ hari ini, atau di pola mingguan yang berjalan/akan datang (`effective_to IS NULL OR effective_to >= today`).
  - Tanpa param → bentuk lama.
- [ ] **Step 1:** Tes:
  - shift dipakai 2 karyawan (1 per tanggal, 1 pola) → 2;
  - shift tidak dipakai → 0;
  - karyawan nonaktif tidak dihitung.
- [ ] **Step 2:** Implementasi (satu query agregat, bukan N+1), gerbang hijau, lalu commit.

### Task 5: Halaman Penempatan Karyawan baru (Frontend)

**Files:**
- Modify/Create di `frontend/src/features/hris/placements/`:
  - `api.ts`, `types.ts`, `queries.ts`, `mutations.ts`, `query-keys.ts`;
  - `components/placements-page.tsx` (tulis ulang);
  - `components/bulk-placement-dialog.tsx` (tulis ulang: mode Pindahkan/Tambah/Akhiri, langkah pratinjau → simpan);
  - `components/placement-filters.tsx`, `components/placements-table.tsx`, `components/bulk-action-bar.tsx`.
- Test: `components/placements-page.test.tsx`, `components/bulk-placement-dialog.test.tsx`

**Interfaces — Consumes:** endpoint Task 1 & 2. **Produces:** `usePlacementSearch(filter)`, `fetchPlacementIds(filter)`, `useBulkPlacement()`, `useBulkEndPlacement()`.

- [ ] **Step 1:** Tes `placements-page.test.tsx` (mock api):
  - chip "Belum ditempatkan · N" mengirim `status=unplaced` dan reset page 1;
  - filter departemen mengirim `department_id`;
  - tombol halaman berikutnya mengirim `page=2`;
  - mencentang 2 baris menampilkan bilah "2 karyawan dipilih";
  - "Pilih semua hasil filter (N)" memanggil ids dan menampilkan N;
  - bilah aksi punya tombol Pindahkan ke toko / Tambah toko / Akhiri / Atur jadwal.
- [ ] **Step 2:** Tes `bulk-placement-dialog.test.tsx`:
  - mode Pindahkan → pilih toko + tanggal → "Lihat pratinjau" memanggil `step=preview`;
  - tabel hasil menampilkan dipindah/ditambah/dilewati + alasan;
  - "Simpan" memanggil `step=commit`;
  - sukses menutup, mengosongkan pilihan, dan invalidasi `["hris","placements"]` + `["hris","schedules"]`.
- [ ] **Step 3:** Implementasi sesuai mockup EPIC (chip atas, filter 4 kolom, tabel kolom: centang, Karyawan (nama, NIP · jabatan), Toko utama (+N toko lain), Sejak, Jadwal (badge Shift s/d tgl / Belum ada jadwal / Non-Shift), Aksi Kelola).
  - Footer "1–25 dari 341", pilih 25/50/100, prev/next.
  - Cari dengan debounce 300 ms.
  - Pilihan disimpan di `Set` lintas halaman.
  - "Atur jadwal" → `router.push('/dashboard/hris/schedules?branch_id=<toko utama terbanyak>&employees=<ids, maks 50>')`.
  - Tombol Lokasi Toko dan dialog Kelola tetap.
  - Kartu stat per toko (47 kartu) diganti chip + filter toko.
- [ ] **Step 4:** Gerbang frontend hijau, lalu commit `feat(hris): halaman penempatan berfilter, berhalaman, aksi massal (EPIC-070)`.

### Task 6: Jadwal Kerja yang bisa diedit + massal (Frontend)

**Files:**
- Modify/Create di `frontend/src/features/hris/schedules/`:
  - `api.ts`, `types.ts`, `queries.ts`, `mutations.ts` (baru);
  - `components/schedules-page.tsx` (tulis ulang);
  - `components/schedule-grid.tsx` (tulis ulang jadi editable);
  - `components/shift-picker.tsx`, `components/pattern-dialog.tsx`, `components/cells-confirm-dialog.tsx`;
  - `schedule-cells.ts` (logika murni).
- Test: `schedule-cells.test.ts`, `components/schedule-grid.test.tsx`, `components/pattern-dialog.test.tsx`

**Interfaces — Consumes:** `POST /api/hris/schedules/cells` (Task 3), grid lama `GET /api/hris/schedules`. **Produces:**
- `expandPattern(employeeIds, pattern: (code|"")[7] Mon..Sun, from, to, grid) → CellEdit[]` (lewati kotak "-" tidak ditempatkan);
- `copyWeek(prevGrid, curGrid) → CellEdit[]` (salin per hari dalam minggu untuk karyawan yang ada di keduanya; lewati "-");
- `fillCells(selected, code) → CellEdit[]`.

- [ ] **Step 1:** Unit test `schedule-cells.test.ts`: `expandPattern` (Sen–Jum P, Sab–Min L untuk 2 karyawan × 14 hari = 28 sel, sel "-" dilewati), `copyWeek`, `fillCells`, dedupe kunci sama (edit terakhir menang).
- [ ] **Step 2:** Tes komponen:
  - klik kotak membuka pemilih shift; memilih kode memanggil preview cells dengan 1 sel; tanpa error/peringatan/timpaan langsung commit;
  - dengan peringatan → dialog konfirmasi berisi peringatan + checkbox "Saya sudah memeriksa peringatan";
  - dengan `replaced_days>0` → field alasan wajib;
  - shift-klik memilih rentang kotak dalam satu baris; Ctrl/Cmd-klik menambah kotak;
  - "Isi kotak terpilih" dan "Kosongkan" mengirim sel yang benar.
- [ ] **Step 3:** Tes `pattern-dialog.test.tsx`: 7 pilihan hari + rentang (default minggu tampil s/d +27 hari, maks 62 hari); "Terapkan" → preview → konfirmasi → commit.
- [ ] **Step 4:** Implementasi halaman.
  - Pemilih cabang (Combobox); bisa dari query `branch_id`/`employees` (centang awal).
  - Navigasi minggu (‹ Minggu ini ›) dengan opsi tampilan 7/14 hari; rentang tetap memenuhi 7–45.
  - Legenda kode shift (kode = nama + jam).
  - Kotak berwarna per kode; libur abu-abu; "-" (tidak ditempatkan) bergaris dan tidak bisa diklik; kosong kuning "Belum ada".
  - Kolom kiri sticky dengan checkbox baris.
  - Bilah aksi saat ada pilihan: Terapkan pola (karyawan tercentang), Isi kotak terpilih, Kosongkan, Batal pilih.
  - Tombol atas: Salin minggu lalu, Unduh template, Unggah jadwal, Terapkan jadwal tetap (Non-Shift).
  - Setelah commit: `invalidateQueries(["hris","schedules"])` + `["hris","placements"]` + `["users","shift-schedule"]`; toast pesan server.
  - Error 409 → toast + refetch.
- [ ] **Step 5:** Gerbang frontend hijau, lalu commit `feat(hris): jadwal kerja bisa diedit per kotak dan massal (EPIC-070)`.

### Task 7: Shift Kerja rapi + invalidasi lintas halaman (Frontend)

**Files:**
- Modify:
  - `frontend/src/features/hris/shifts/{api,types,queries,mutations}.ts`;
  - `components/shifts-page.tsx`, `components/shift-form-dialog.tsx`;
  - `frontend/src/features/users/mutations.ts` (save pola mingguan juga invalidasi `["hris","schedules"]` dan `["hris","placements"]`).
- Test: `components/shifts-page.test.tsx`, `components/shift-form-dialog.test.tsx`

- [ ] **Step 1:** Tes:
  - daftar dikelompokkan per awalan kode (huruf sebelum angka pertama; DEF/NS di grup "Bawaan");
  - pencarian menyaring kode/nama/jam;
  - kolom "Dipakai" menampilkan `employees_using`;
  - tombol hapus disembunyikan untuk shift dipakai, diganti "Nonaktifkan";
  - form menampilkan ringkasan "Masuk 11:00 · terlambat setelah 11:10 · pulang 19:00" dan peringatan "Dipakai N karyawan…" saat edit;
  - nama otomatis disarankan dari kode + jam bila nama kosong;
  - save menginvalidasi `["hris","shifts"]`, `["users","shift-options"]`, `["hris","schedules"]`.
- [ ] **Step 2:** Implementasi tampilan tabel ringkas (Kode, Nama, Jam, Istirahat, Toleransi, Dipakai, Status, Aksi), gerbang hijau, lalu commit.

### Task 8: Dokumen + manual

- [ ] Perbarui Automation Log EPIC-070.
- [ ] Perbarui `docs/hris/acuan-mom-20261019/status-requirement.md` hanya bila ada ID terkait.
- [ ] Tambah bagian ringkas "Penempatan & Jadwal massal" di manual HRIS admin (`.share/manual-hris/` bab penempatan/jadwal) bila file ada.
- [ ] Commit `docs: EPIC-070 …`.

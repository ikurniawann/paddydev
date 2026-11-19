# Redesign Detail Run Payroll Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Halaman detail run payroll HR (`/dashboard/hris/payroll/[id]`) menjadi tempat kerja memeriksa slip: daftar karyawan bisa dicari, difilter, diurutkan, dipaginasi, menjelaskan kehadiran dan tanda per baris, dan bagian lain pindah ke tab.

**Architecture:** Backend Go menambah kolom ringkasan kehadiran dan tanda ke `payroll_details` di `GET /api/hris/payroll/{id}` (Next fallback ikut). Frontend memakai fungsi murni (tanda, filter, urut, halaman) yang diuji unit, state filter di query string URL, lalu halaman disusun ulang: header, bilah status, kartu angka, dan empat tab. Semua penyaringan di klien (run terbesar 237 baris).

**Tech Stack:** Go (`apps/backend/internal/modules/payroll`), Next.js App Router + React Query + Tailwind (`apps/frontend`), Vitest + Testing Library, Go integration test (`-p 1`, DB tes 127.0.0.1:55499).

**Spec:** `docs/epics/EPIC-120-redesign-detail-run-payroll.md` (repo paddydev; kritik dan keputusan desain). Kode ada di repo erp-paddy.

## Global Constraints

- Branch `feat/epic120-detail-run-payroll` dari `origin/development` di worktree `.worktrees/erp-epic120` (repo erp-paddy). Jangan bekerja di `~/WORK/erp-paddy`.
- Tidak ada kode rule `R-` di teks tampilan (keputusan EPIC-118 "Semua tampilan"). Ekspor CSV tetap memakai kode.
- Teks UI berbahasa Indonesia, mengikuti skill `stop-slop`; tanpa em dash di teks baru.
- Tidak menambah dependency. Pakai `@/components/ui/{tabs,stat-card,input,select,badge,dialog,button,card}` dan `DsDataTablePagination` dari `@/components/design-system/ds-data-table`.
- Baca panduan Next di `apps/frontend/node_modules/next/dist/docs/` sebelum memakai `useSearchParams`/`useRouter` (AGENTS.md).
- Label tanda (persis):
  `potongan` "Ada potongan" · `manual` "Komponen manual" · `cuti_unpaid` "Cuti tanpa bayaran" · `terlambat` "Terlambat" · `toko_tutup` "Toko tutup" · `tanpa_toko` "Tanpa toko" · `multi_toko` "Lebih dari 1 toko" · `thp_nol` "THP Rp 0".
- "Perlu dicek" = `tanpa_toko`, `toko_tutup`, `thp_nol`, `cuti_unpaid`, `manual`.
- Ukuran halaman 25 / 50 / 100, bawaan 25. Urutan: `nama` (A-Z, bawaan), `thp_desc`, `thp_asc`, `potongan_desc`.
- Parameter URL: `tab` (`karyawan` bawaan | `komponen` | `toko` | `rekap`), `q`, `dept`, `toko` (branch_id atau `none`), `tanda` (satu kode tanda atau `perlu_dicek`), `urut`, `hal`, `per`.
- Tidak ada perubahan production tanpa izin user; rilis production mengikuti aturan rilis (backup, hindari 07-08, 13, 16-17, 21 WIB).

## Review Focus

1. Run draft yang belum dihitung (0 detail): tab Karyawan menampilkan ajakan "Hitung payroll", toolbar filter disembunyikan, kartu "Perlu dicek" 0.
2. Slip format lama (`slip_version` null, kolom kehadiran 0/null): kolom Kehadiran menampilkan "–" dan tidak ada tanda palsu.
3. URL basi (halaman di luar rentang sesudah filter, `dept`/`toko`/`tanda` tak dikenal, `per=7`): halaman dijepit ke halaman terakhir, nilai tak dikenal diabaikan.
4. Pencarian " 2022060 " atau "raDEN": dipangkas, tidak peka huruf besar, cocok ke NIP sebagian dan nama.
5. Simpan & hitung ulang dari dialog Edit: baris diperbarui dan filter/halaman di URL tetap.

---

### Task 1: Ringkasan kehadiran dan tanda di detail run (Go + Next fallback)

**Files:**
- Modify: `apps/backend/internal/modules/payroll/postgres_runs.go` (`runDetailsJSON`)
- Modify: `apps/backend/internal/modules/payroll/embeds.go` (`runDetailEmp` tambah `"position"`)
- Modify: `apps/frontend/src/lib/payroll/runs.ts` (`RUN_DETAIL_SELECT`, `getRun`)
- Modify: `apps/frontend/src/features/hris/payroll/types.ts` (`PayrollRunDetailRow`)
- Test: `apps/backend/internal/modules/payroll/slip_lines_integration_test.go`

**Interfaces:**
- Produces: setiap elemen `payroll_details` membawa `working_days, present_days, late_days, late_minutes, paid_leave_days, unpaid_leave_days, personal_leave_days, sick_leave_days, overtime_regular_hours, overtime_holiday_hours, store_closure_note, manual_input_count` dan `employee.position`. Di TS: numeric Postgres bertipe `number | string | null`, `store_closure_note: string | null`, `manual_input_count: number`, `employee.position?: { name: string } | null` (bentuk mengikuti embed `position` yang sudah dipakai `payslipEmp`).

- [ ] **Step 1: Tes gagal** `TestRunDetailsCarryAttendanceSummary`: pakai `slipSplitRun(e)`, simpan satu komponen manual untuk `emp` (endpoint inputs yang dipakai tes EPIC-118), hitung ulang, lalu `GET /api/hris/payroll/{run}`. Assert untuk baris `emp`: kunci `present_days`, `working_days`, `late_days`, `paid_leave_days`, `unpaid_leave_days`, `store_closure_note` ada; `manual_input_count == 1`; `employee` punya kunci `position`.
- [ ] **Step 2:** `go test -p 1 ./internal/modules/payroll -run TestRunDetailsCarryAttendanceSummary` → FAIL (kunci tidak ada).
- [ ] **Step 3:** Tambah kolom ke SELECT `runDetailsJSON`; `manual_input_count` dari kolom jsonb `manual_inputs` (cek bentuknya di DB tes dulu: array → `jsonb_array_length`, objek → jumlah kunci; null → 0). Tambah `"position"` ke `runDetailEmp`. Samakan `RUN_DETAIL_SELECT` di Next untuk kolom tabel; `manual_input_count` di Next dihitung di `getRun` sesudah query.
- [ ] **Step 4:** Tes Step 2 → PASS; `go test -p 1 ./internal/modules/payroll` hijau.
- [ ] **Step 5:** Commit `feat(payroll): detail run membawa ringkasan kehadiran dan tanda (EPIC-120)`.

### Task 2: Fungsi murni tanda, filter, urut, halaman

**Files:**
- Create: `apps/frontend/src/lib/payroll/run-detail-view.ts`
- Test: `apps/frontend/src/lib/payroll/run-detail-view.test.ts`

**Interfaces:**
- Consumes: `PayrollRunDetailRow` (Task 1).
- Produces:
  - `type DetailFlag = "potongan" | "manual" | "cuti_unpaid" | "terlambat" | "toko_tutup" | "tanpa_toko" | "multi_toko" | "thp_nol"`
  - `const DETAIL_FLAG_LABELS: Record<DetailFlag, string>`; `const NEEDS_REVIEW_FLAGS: DetailFlag[]`
  - `detailFlags(row: PayrollRunDetailRow): DetailFlag[]`
  - `interface RunDetailFilters { q: string; dept: string | null; toko: string | null; tanda: DetailFlag | "perlu_dicek" | null; urut: "nama" | "thp_desc" | "thp_asc" | "potongan_desc" }`
  - `filterAndSortDetails(rows: PayrollRunDetailRow[], f: RunDetailFilters): PayrollRunDetailRow[]`
  - `paginate<T>(rows: T[], hal: number, per: number): { items: T[]; hal: number; totalPages: number }` (hal dijepit 1..totalPages)
  - `summarizeDetails(rows: PayrollRunDetailRow[]): { count: number; gross: number; deductions: number; net: number; needsReview: number }` (gross = `gross_salary + other_earning_nontax`, sama dengan tabel sekarang)
  - `attendanceText(row: PayrollRunDetailRow): string | null` → "Hadir 20/22 · Terlambat 2× · Cuti 1 · Unpaid 1"; bagian bernilai 0 dihilangkan; null bila `slip_version` null atau `working_days` 0.

- [ ] **Step 1: Tes gagal** (satu `it` per perilaku):
  - `detailFlags`: `total_deductions 50000` → berisi `potongan`; `allocations: []` → `tanpa_toko`; dua alokasi → `multi_toko`; alokasi `basis "none"` → `tanpa_toko`; `net_salary 0` → `thp_nol`; `store_closure_note "Toko tutup 15-11"` → `toko_tutup`; `manual_input_count 2` → `manual`; baris slip lama dengan semua angka 0/null dan satu alokasi → `[]`.
  - `filterAndSortDetails`: `q " 2022060 "` cocok NIP `202206012`; `q "raDEN"` cocok "Raden Riski"; `dept "Marketing"`; `toko "none"` hanya baris tanpa toko; `tanda "perlu_dicek"` = gabungan `NEEDS_REVIEW_FLAGS`; `urut "thp_desc"` urut THP turun, seri diurut nama.
  - `paginate`: 60 baris, `per 25`, `hal 9` → `hal 3`, 10 item; 0 baris → `hal 1`, `totalPages 1`.
  - `attendanceText`: present 20, working 22, late 2, paid_leave 1, unpaid 1 → `"Hadir 20/22 · Terlambat 2× · Cuti 1 · Unpaid 1"`; late 0 dan cuti 0 → `"Hadir 22/22"`.
- [ ] **Step 2:** `pnpm --filter frontend exec vitest run src/lib/payroll/run-detail-view.test.ts` → FAIL (modul tidak ada).
- [ ] **Step 3:** Implementasi. Angka Postgres bisa string: pakai `Number(x) || 0`.
- [ ] **Step 4:** Tes → PASS.
- [ ] **Step 5:** Commit `feat(payroll): fungsi tanda, filter, dan halaman detail run (EPIC-120)`.

### Task 3: State filter di URL

**Files:**
- Create: `apps/frontend/src/features/hris/payroll/hooks/use-run-detail-params.ts`
- Test: `apps/frontend/src/features/hris/payroll/hooks/use-run-detail-params.test.ts`

**Interfaces:**
- Consumes: `RunDetailFilters`, `DetailFlag` (Task 2).
- Produces:
  - `type RunDetailTab = "karyawan" | "komponen" | "toko" | "rekap"`
  - `interface RunDetailParams extends RunDetailFilters { tab: RunDetailTab; hal: number; per: 25 | 50 | 100 }`
  - `parseRunDetailParams(sp: URLSearchParams): RunDetailParams` (nilai tak dikenal → bawaan)
  - `serializeRunDetailParams(p: RunDetailParams): string` (nilai bawaan tidak ditulis)
  - `useRunDetailParams(): [RunDetailParams, (patch: Partial<RunDetailParams>) => void]` memakai `router.replace(..., { scroll: false })`; patch selain `hal` mengembalikan `hal` ke 1.

- [ ] **Step 1: Tes gagal** untuk parse/serialize: string kosong → semua bawaan; `per=7` → 25; `tanda=xyz` → null; `tab=toko&hal=3` bolak-balik utuh; serialize bawaan → `""`.
- [ ] **Step 2:** vitest → FAIL.
- [ ] **Step 3:** Implementasi (fungsi murni + hook tipis).
- [ ] **Step 4:** vitest → PASS.
- [ ] **Step 5:** Commit `feat(payroll): filter detail run tersimpan di URL (EPIC-120)`.

### Task 4: Tab Karyawan

**Files:**
- Create: `apps/frontend/src/features/hris/payroll/components/run-employees-tab.tsx`
- Modify: `apps/frontend/src/features/hris/payroll/components/payslip-page.tsx` (tombol Kembali memakai `?kembali=`)
- Test: `apps/frontend/src/features/hris/payroll/components/run-employees-tab.test.tsx`

**Interfaces:**
- Consumes: Task 2, Task 3 (`params`, `setParams` diterima sebagai props agar mudah diuji).
- Produces: `RunEmployeesTab({ runId, status, details, params, setParams, onEdit, onNotify, notifyingId })`.

Isi: toolbar (Input cari "Cari nama atau NIP", Select Departemen dari data, Select Toko dari alokasi + "Tanpa toko", Select Tanda dengan "Perlu dicek" di urutan pertama, Select Urutkan, tombol "Reset filter" bila ada filter aktif), teks "Menampilkan 26-50 dari 61 karyawan", tabel, `DsDataTablePagination` dengan pilihan 25/50/100, baris kaki "Total tersaring" (pendapatan, potongan, THP dari `summarizeDetails`).
Kolom: Karyawan (nama; NIP · departemen · jabatan), Toko (`AllocationChips`), Kehadiran (`attendanceText` atau "–"), Pendapatan, Potongan, THP, Aksi (WA bila `paid`, Edit bila `draft`, Slip). Badge tanda di bawah nama; tanda kelompok perlu dicek bernada peringatan. Kolom Karyawan sticky kiri di layar sempit.
Tautan Slip menyertakan query string sekarang sebagai `?kembali=` (hanya path `/dashboard/hris/payroll/{runId}` + query; tolak nilai lain) sehingga tombol Kembali di slip membuka posisi yang sama.

- [ ] **Step 1: Tes gagal:** 60 baris → 25 baris tampil dan "Menampilkan 1-25 dari 60 karyawan"; ketik di pencarian memanggil `setParams({ q })`; `params.tanda = "perlu_dicek"` hanya menampilkan baris bertanda; detail kosong menampilkan "Belum ada detail payroll" tanpa toolbar; status `draft` menampilkan Edit, `paid` menampilkan WA; tidak ada teks yang cocok `/R-\d/`; `kembali` berisi URL luar diabaikan di slip.
- [ ] **Step 2:** vitest → FAIL.
- [ ] **Step 3:** Implementasi.
- [ ] **Step 4:** vitest → PASS.
- [ ] **Step 5:** Commit `feat(payroll): tab Karyawan dengan cari, filter, dan pagination (EPIC-120)`.

### Task 5: Tab Biaya per Toko sebagai tabel

**Files:**
- Create: `apps/frontend/src/features/hris/payroll/components/run-store-cost-table.tsx`
- Modify: `apps/frontend/src/features/hris/payroll/components/store-allocation-card.tsx` (hapus bila tidak dipakai lagi; peringatan "tanpa toko" pindah ke komponen baru)
- Test: `apps/frontend/src/features/hris/payroll/components/run-store-cost-table.test.tsx`

**Interfaces:**
- Consumes: `RunStoreAllocation`.
- Produces: `RunStoreCostTable({ runId, allocation, slipCount, isDraft })`.

Kolom: Toko, Karyawan, FTE, Gaji kotor, Beban perusahaan (hanya bila ada nilai > 0), Total biaya. Klik header mengurutkan (bawaan Total biaya turun). Baris total di bawah. Peringatan tanpa toko tetap di atas tabel. Tautan "Laporan lengkap" ke `/dashboard/hris/payroll/store-costs?run={runId}`.

- [ ] **Step 1: Tes gagal:** 3 toko → urut total biaya turun dan baris total = jumlah; semua beban perusahaan 0 → kolom tidak ada; klik header "Toko" → urut nama.
- [ ] **Step 2:** vitest → FAIL. **Step 3:** implementasi. **Step 4:** PASS.
- [ ] **Step 5:** Commit `feat(payroll): biaya per toko di detail run jadi tabel (EPIC-120)`.

### Task 6: Tab Komponen Manual

**Files:**
- Modify: `apps/frontend/src/features/hris/payroll/components/manual-inputs-card.tsx` (jadi `ManualInputsTab`)
- Test: `apps/frontend/src/features/hris/payroll/components/manual-inputs-tab.test.tsx`

**Interfaces:**
- Produces: `ManualInputsTab({ runId, editable })`; `editable` = run draft.

Daftar komponen dengan pencarian nama/NIP dan total nominal per jenis (pendapatan/potongan dari `manualInputKind`). Tombol "Tambah komponen" membuka dialog berisi form yang sekarang (karyawan, komponen, nominal, catatan). "Impor CSV" tetap. Sesudah simpan: toast "Komponen tersimpan. Klik Hitung ulang agar masuk ke slip." Run bukan draft: hanya daftar, tanpa tombol.

- [ ] **Step 1: Tes gagal:** form tidak tampil sebelum klik "Tambah komponen"; pencarian menyaring daftar; `editable=false` tanpa tombol tambah/hapus.
- [ ] **Step 2-4:** FAIL → implementasi → PASS (sesuaikan tes lama yang merender `ManualInputsCard`).
- [ ] **Step 5:** Commit `feat(payroll): komponen manual pindah ke tab dengan dialog tambah (EPIC-120)`.

### Task 7: Susun ulang halaman detail

**Files:**
- Modify: `apps/frontend/src/features/hris/payroll/components/payroll-detail-page.tsx`
- Modify: `apps/frontend/src/features/hris/payroll/components/run-stepper.tsx` (alasan terkunci tampil sebagai teks)
- Modify: `apps/frontend/src/features/hris/payroll/components/calculation-panel.tsx` (baris ringkas di bilah status: centang THR + tombol Hitung/Hitung ulang; peringatan hasil tampil di bawah bilah)
- Test: `apps/frontend/src/features/hris/payroll/components/payroll-detail-page.test.tsx`

**Interfaces:**
- Consumes: Task 3-6.

Urutan: Kembali → judul, badge entitas, badge status ("Draft", "Diproses", "Disetujui", "Dibayar"), "Dihitung terakhir 19-11-2026 12.47" dari `calculated_at` → bilah status (stepper + tombol berikut + alasan terkunci + status periode absensi satu baris + Hitung ulang) → 5 `StatCard`: Karyawan, Total pendapatan, Total potongan (hint PPh 21 dan BPJS employer bila > 0), THP, Perlu dicek (klik → `setParams({ tab: "karyawan", tanda: "perlu_dicek" })`) → `Tabs`: Karyawan, Komponen Manual (jumlah), Biaya per Toko (jumlah toko), Rekap Rule (`SlipRecapCard`).

- [ ] **Step 1: Tes gagal:** tab bawaan Karyawan; `?tab=toko` membuka tabel toko; klik kartu "Perlu dicek" memanggil replace dengan `tanda=perlu_dicek`; periode terbuka → teks "Closing periode absensi dulu" terlihat tanpa hover; `calculated_at` tampil "Dihitung terakhir".
- [ ] **Step 2-4:** FAIL → implementasi → PASS; jalankan seluruh tes folder `features/hris/payroll`.
- [ ] **Step 5:** Commit `feat(payroll): susun ulang halaman detail run (EPIC-120)`.

### Task 8: Gerbang akhir dan staging

- [ ] `pnpm --filter frontend exec tsc --noEmit`, `pnpm --filter frontend lint`, `pnpm --filter frontend test`, `go test -p 1 ./...` hijau (flake `TestGapDigestWarnsWithoutRecipients` dicatat bila muncul).
- [ ] Review akhir seluruh branch (model paling mampu), perbaiki Critical/Important.
- [ ] Cek visual di browser lokal: 1440px dan 390px, tema terang dan gelap.
- [ ] PR ke `development`; merge dan deploy staging hanya dengan izin user; QA oleh HR di run November staging.
- [ ] Perbarui EPIC-120 Automation Log dan status.

# EPIC-114 Dropdown Karyawan Menampilkan Semua Data — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Semua dropdown karyawan di HRIS menampilkan atau bisa menjangkau seluruh karyawan. Yang utama: dropdown "Tambah Salary" dan "Input Manual" payroll tidak lagi berhenti di 20 orang.

**Architecture:**
- **Akar masalah:** `fetchSalaryEmployees` (`apps/frontend/src/features/hris/salary/api.ts:14`) memanggil `GET /api/hris/employees` tanpa `limit` dan tanpa paging. Backend lalu memakai default `limit = 20` (`apps/backend/internal/modules/hris/domain/employees.go:121`).
- **Pemakai yang terkena:** hook `useSalaryEmployees` dipakai oleh `new-salary-page.tsx` (Tambah Salary) dan `manual-inputs-card.tsx` (Input Manual payroll).
- **Perbaikan:** pakai helper yang sudah ada, `fetchAllEmployees` (`apps/frontend/src/lib/hris/fetch-all-employees.ts`, paging 500 per halaman).
- **Pencegah regresi:** tes yang menolak panggilan daftar `/api/hris/employees` tanpa paging.

**Tech Stack:** Next.js (`apps/frontend`, React Query, vitest).

**Spec:** Laporan user 18-11-2026: "ada BUG dropdown untuk di tambah salary khusus karyawan tidak tampil semua , sama dengan dropdown karyawan di payroll semua dropdown karyawan pastikan harus semua data tertampil". Data production 18-11: 315 karyawan aktif dari 648 total; dropdown hanya memuat 20.

Peta dropdown karyawan (origin/development 361e220):

| Dropdown | Sumber | Status |
|---|---|---|
| Tambah Salary (`salary/components/new-salary-page.tsx`) | `useSalaryEmployees` → `/api/hris/employees` tanpa limit | **Terpotong 20** |
| Input Manual payroll (`payroll/components/manual-inputs-card.tsx`) | `useSalaryEmployees` | **Terpotong 20** |
| Koreksi/absen manual, Dinas luar | `useActiveEmployees` → `fetchAllEmployees` | Lengkap |
| Pengajuan cuti | `useLeaveEmployees` → `fetchAllEmployees` | Lengkap |
| Pinjaman, Lembur | `useEmployeeOptions` → `fetchAllEmployees` | Lengkap |
| Struktur organisasi | `fetchOrgEmployees` → `fetchAllEmployees` | Lengkap |
| Surat peringatan, Pengatur jadwal | `searchPlacements` 25 teratas + cari nama/NIP di server | Terjangkau lewat ketikan |
| Staff pass tiket | `searchEmployees` 20 teratas + cari di server | Terjangkau lewat ketikan |
| Tugas departemen KPI | anggota departemen (props) | Lengkap untuk departemennya |

Keputusan yang diambil rencana ini (konfirmasi saat review):
- **K-1:** pemilih berbasis pencarian (surat peringatan, pengatur jadwal, staff pass) tetap berbasis pencarian server. Setiap karyawan bisa dicari lewat nama atau NIP tanpa memuat ratusan baris tiap kali dibuka. Yang ditambahkan hanya keterangan cara mencari ketika hasilnya terpotong.
- **K-2:** Tambah Salary dan Input Manual memuat **semua** karyawan, aktif maupun nonaktif, sama seperti filter sekarang. Ini supaya karyawan resign yang masih digaji prorata di periodenya tetap bisa diberi input manual. Karyawan nonaktif diberi label "(nonaktif)".

## Global Constraints

- Kode di `wit-id-indonesia/erp-paddy`, branch `fix/epic114-dropdown-karyawan` dari `origin/development`, di worktree `paddydev/.worktrees/erp-epic114`. Merge hanya lewat PR. Production tidak disentuh.
- Teks UI berbahasa Indonesia persis seperti di rencana ini.
- Backend tidak diubah. Batas `limit` 1–500 tetap.
- `apps/frontend/src/lib/backend-routes.test.ts` sudah merah di base karena commit lain; pastikan tidak bertambah.

## Review Focus

1. **Lebih dari 500 karyawan** (lebih dari satu halaman): dropdown tetap memuat semua. Diuji di Task 1.
2. **Karyawan nonaktif dengan nama sama dengan yang aktif:** label "(nonaktif)" dan NIP membedakan keduanya. Diuji di Task 1.
3. **Kode baru memanggil `/api/hris/employees` tanpa paging lagi:** tes pencegah di Task 1 gagal.
4. **Pemilih pencarian tanpa ketikan:** keterangan cara mencari muncul bila hasil = batas halaman. Diuji di Task 2.
5. **Endpoint gagal atau lambat:** dropdown menampilkan "Memuat karyawan…" lalu placeholder, tidak crash. Perilaku React Query yang ada dipertahankan; cek manual di staging (Task 3).

---

### Task 1: Tambah Salary dan Input Manual memuat semua karyawan

**Files:**
- Modify: `apps/frontend/src/features/hris/salary/api.ts` (`fetchSalaryEmployees`)
- Modify: `apps/frontend/src/features/hris/salary/types.ts` (`SalaryEmployeeLite.is_active?: boolean`)
- Create: `apps/frontend/src/features/hris/salary/employee-option.ts` (`salaryEmployeeLabel`)
- Modify: `apps/frontend/src/features/hris/salary/components/new-salary-page.tsx`, `apps/frontend/src/features/hris/payroll/components/manual-inputs-card.tsx`
- Test: `apps/frontend/src/features/hris/salary/salary-employees.test.ts`, `apps/frontend/src/lib/hris/employee-list-calls.test.ts`

**Interfaces:**
- Produces:
  - `fetchSalaryEmployees(): Promise<SalaryEmployeeLite[]>` = `fetchAllEmployees<SalaryEmployeeLite>("sort_by=full_name&sort_order=asc")`;
  - `salaryEmployeeLabel(e: SalaryEmployeeLite): string`, which keeps the current label format of the two dropdowns and appends `" (nonaktif)"` when `is_active === false`.

- [ ] **Step 1: Tulis tes yang gagal.**
  - `salary-employees.test.ts`, dengan `apiGet` dari `@/lib/api-client` di-mock:
    - `"memuat semua halaman karyawan"`: halaman 1 berisi 500 baris (`total: 515`) dan halaman 2 berisi 15 baris. Hasilnya `toHaveLength(515)`. Panggilan pertama `toHaveBeenCalledWith("/api/hris/employees?sort_by=full_name&sort_order=asc&limit=500&page=1")`.
    - `"label karyawan nonaktif"`: karyawan `is_active: false` mendapat akhiran `" (nonaktif)"`; karyawan aktif tidak.
  - `employee-list-calls.test.ts`, `"daftar karyawan selalu lewat fetchAllEmployees atau pencarian"`:
    - Pindai semua `.ts`/`.tsx` di `apps/frontend/src` kecuali `*.test.*`, `app/api/**/route.ts`, `*.generated.json`, dan `lib/hris/fetch-all-employees.ts`.
    - Pelanggaran: literal `/api/hris/employees` yang langsung diikuti `"`, `` ` `` atau `?`, tanpa `search=`, `limit=`, atau `employee_id=` di query.
    - Kumpulkan `path:baris`, lalu `expect(violations).toEqual([])`.
- [ ] **Step 2: Jalankan, pastikan gagal.** `pnpm --filter frontend exec vitest run src/features/hris/salary/salary-employees.test.ts src/lib/hris/employee-list-calls.test.ts`. Expected: FAIL. Tes paging gagal karena panggilan tanpa `limit`, tes label gagal karena `salaryEmployeeLabel` belum ada, dan tes pemindai menyebut `features/hris/salary/api.ts`.
- [ ] **Step 3: Implementasi** sesuai Interfaces. Kedua komponen memakai `salaryEmployeeLabel` untuk label opsi. `SalaryEmployeeLite` menambah `is_active?: boolean`; endpoint sudah mengirim kolom itu.
- [ ] **Step 4: Jalankan** tes Step 2 dan `pnpm --filter frontend exec vitest run src/features/hris/salary src/features/hris/payroll`. Expected: PASS.
- [ ] **Step 5: Commit.** `fix(hris): dropdown Tambah Salary dan Input Manual payroll memuat semua karyawan (EPIC-114)`

### Task 2: Keterangan cara mencari di pemilih berbasis pencarian

**Files:**
- Create: `apps/frontend/src/features/hris/shared/search-cap-hint.tsx` (`SearchCapHint`)
- Modify: `apps/frontend/src/features/hris/disciplinary-letters/components/issue-letter-dialog.tsx`, `apps/frontend/src/features/hris/schedule-delegates/components/delegate-form-dialog.tsx`, `apps/frontend/src/features/ticketing/masters/components/staff-passes-section.tsx`
- Test: `apps/frontend/src/features/hris/shared/search-cap-hint.test.tsx`

**Interfaces:**
- Produces: `SearchCapHint({ shown, cap }: { shown: number; cap: number }): JSX.Element | null`. It renders `"Menampilkan {cap} karyawan teratas. Ketik nama atau NIP untuk mencari karyawan lain."` when `shown >= cap`, and `null` otherwise.

- [ ] **Step 1: Tulis tes yang gagal:** `shown=25, cap=25` menampilkan teks persis di atas; `shown=7, cap=25` tidak menampilkan apa-apa.
- [ ] **Step 2: Jalankan, pastikan gagal.** `pnpm --filter frontend exec vitest run src/features/hris/shared/search-cap-hint.test.tsx`. Expected: FAIL (modul belum ada).
- [ ] **Step 3: Implementasi** komponen. Pasang di bawah combobox ketiga pemilih dengan `cap` = batas halaman masing-masing (25, 25, 20).
- [ ] **Step 4: Jalankan** tes Step 2, vitest folder ketiga komponen, lalu gerbang penuh:
  - `pnpm --filter frontend exec tsc --noEmit`
  - `pnpm --filter frontend lint`
  - `pnpm --filter frontend exec vitest run`
  
  Expected: hijau kecuali `backend-routes.test.ts` yang sudah merah di base.
- [ ] **Step 5: Commit.** `feat(hris): keterangan pencarian di pemilih karyawan (EPIC-114)`

### Task 3: Serah terima

- [ ] **Step 1:** Review akhir branch oleh reviewer segar, lalu PR ke `development`. Merge, deploy staging, dan QA HR menunggu izin user. Cek QA:
  - dropdown Tambah Salary dan Input Manual memuat semua karyawan (cari nama berawalan huruf belakang, mis. "Yuli");
  - karyawan nonaktif berlabel "(nonaktif)";
  - keterangan pencarian muncul di surat peringatan saat belum mengetik.
- [ ] **Step 2:** Catat di `docs/epics/EPIC-114-dropdown-karyawan-lengkap.md` (paddydev): akar masalah, peta dropdown, K-1, K-2, dan hasil gerbang.

# Info Keterlambatan & Tingkat Teguran/SP di ESS — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Karyawan melihat jumlah terlambat per periode absensi beserta tingkat teguran/SP menurut ketentuan 02-11 di ESS Absensi dan Beranda. Atasan melihat hal yang sama untuk timnya di Absensi Tim.

**Architecture:**
- Aturan tingkat ada di satu fungsi domain Go (`hris/domain/lateness.go`). Dua endpoint baca-saja di modul HRIS menghitung baris `is_late` dalam rentang periode absensi; periode ditentukan oleh `periodContaining`, yang sudah ada.
- UI membaca `rules`, `level` dan `next` dari server, tanpa menyalin batas angka.
- Fitur frontend baru `features/hris/lateness` memuat tiga kartu: Absensi (lengkap), Beranda (ringkas), dan Absensi Tim.

**Tech Stack:** Go (`backend/internal/modules/hris`), Next.js + TanStack Query + Vitest, Tailwind, base-ui Select.

**Spec:** `docs/epics/EPIC-077-info-keterlambatan-ess.md` (R1–R13).

## Global Constraints
- Tingkat:
  - `none` 0: "Tidak ada keterlambatan"
  - `teguran` 1–2: "Teguran lisan dan tertulis"
  - `sp1` 3–4: "Surat Peringatan 1"
  - `sp2` 5–6: "Surat Peringatan 2"
  - `sp3` ≥ 7: "Surat Peringatan 3 dan/atau PHK"
- Hitungan = baris `hris.attendance` dengan `is_late` dan `date BETWEEN period.start AND period.end`.
- `back` 0–5 (default 0); selain itu 400 "Periode tidak tersedia".
- Endpoint hanya di Go (masuk `goNativePrefixes`); `make generate` memperbarui `go-routes.generated.json`.
- Tanpa migrasi, tanpa perubahan payroll. UI tidak menyebut potongan gaji.
- Catatan wajib di UI: "Informasi pengingat. Surat peringatan resmi diterbitkan oleh HR."
- Mobile 360 px: tanpa scroll horizontal, daftar berbentuk kartu, target sentuh ≥ 40 px.
- Tes integrasi Go: `go test -p 1` dengan `TEST_DATABASE_URL`.

## Review Focus
- Periode cutoff 20: baris tanggal 21 masuk periode bulan berikutnya. Tes dengan `cutoff_day = 20`.
- Baris yang dikoreksi HR menjadi `is_late = false` tidak dihitung lagi. Tes langsung memakai `is_late` false.
- Karyawan tanpa akun karyawan atau atasan tanpa tim → 200 dengan daftar kosong, bukan 403/500.
- Nama panjang dan 12+ hari terlambat di HP tidak memicu overflow. Diperiksa di QA probe 360 px.
- Anggota tim tidak langsung menampilkan atasan langsungnya. Diuji di integrasi tim.

---

### Task 1: Backend — domain tingkat, dua endpoint, tes

**Files:**
- Create: `backend/internal/modules/hris/domain/lateness.go`, `domain/lateness_test.go`
- Create: `backend/internal/modules/hris/lateness.go` (service, SQL, handler)
- Create: `backend/internal/modules/hris/lateness_integration_test.go`
- Modify: `backend/internal/modules/hris/routes.go` (2 route di blok `/api/hris/me/...`)
- Modify: `frontend/src/lib/go-routes.generated.json` (via `make generate`), `frontend/src/lib/backend-routes.test.ts`

**Interfaces (Produces):**
- `type LatenessLevel struct { Code, Label, Range string; Min int; Max *int }`, ditambah JSON `code,label,range,min,max`
- `var LatenessLevels []LatenessLevel` (urutan R1; Range: "0 kali", "1–2 kali", "3–4 kali", "5–6 kali", "Lebih dari 6 kali")
- `func LatenessLevelFor(count int) LatenessLevel`
- `func NextLatenessLevel(count int) (level LatenessLevel, remaining int, ok bool)`
- JSON `GET /api/hris/me/lateness?back=N`:
  - `{ period, periods, rules, count, total_minutes, level, next, days }`
  - `period` = `periodJSON(p, today)`
  - `periods` = `[{back, name, label}]` × 6
  - `next` = `{code, label, at, remaining}` | null
  - `days` = `[{date, shift_name, scheduled_start, clock_in, late_minutes}]` (jam "HH:MM" WIB, urut tanggal)
- JSON `GET /api/hris/me/team-lateness?back=N`:
  - `{ period, periods, rules, summary: {none,teguran,sp1,sp2,sp3}, employees: [...] }`
  - `employees` = `[{employee_id, full_name, position_title, direct, reporting_to_name, count, total_minutes, level, days}]`

- [ ] **Step 1: Tes domain gagal.** `TestLatenessLevelFor`:
  - tabel count→code: 0 none, 1 teguran, 2 teguran, 3 sp1, 4 sp1, 5 sp2, 6 sp2, 7 sp3, 30 sp3; negatif → none.
  - `TestNextLatenessLevel`: 0 → (teguran, 1); 2 → (sp1, 1); 3 → (sp2, 2); 6 → (sp3, 1); 7 → ok=false.
- [ ] **Step 2:** Jalankan `go test ./internal/modules/hris/domain -run Lateness`. Harus FAIL (undefined).
- [ ] **Step 3:** Implementasikan domain di `domain/lateness.go`. Jalankan ulang; harus PASS.
- [ ] **Step 4: Tes integrasi gagal** (`lateness_integration_test.go`, pola `newHarness`, `h.setNow`, `cutoff_day = 0`):
  - `TestMyLatenessCountsPeriod`:
    - Karyawan dengan akun, now 2026-10-20 10:00 WIB.
    - Data: 3 baris `is_late` (Okt 2/5/9, `late_minutes` 12/20/45), 1 baris tidak terlambat, 1 baris terlambat 30 Sep, dan 1 baris terlambat milik karyawan lain.
    - Expect: count 3, total_minutes 77, level `sp1`, next `{sp2, at 5, remaining 2}`, days urut 02/05/09, `periods` panjang 6 dengan `back` 0..5.
  - `TestMyLatenessBack`: `?back=1` → period September, count 1. `?back=6` dan `?back=x` → 400 "Periode tidak tersedia".
  - `TestMyLatenessCutoff20`: `cutoff_day = 20`, now 2026-11-02, terlambat 21 Okt dan 20 Okt. `back=0` → count 1 (21 Okt), `back=1` → count 1.
  - `TestMyLatenessUnlinked`: akun staf tanpa karyawan → 200, count 0, days `[]`.
  - `TestTeamLateness`:
    - Atasan → koordinator → staf; ditambah orang luar.
    - Staf terlambat 7x, koordinator 1x.
    - Expect:
      - employees diurutkan staf (sp3, `direct` false, `reporting_to_name` = koordinator) lalu koordinator (teguran);
      - orang luar tidak ada;
      - summary `{sp3:1, teguran:1, none:0, ...}`;
      - akun tanpa tim → employees `[]`.
- [ ] **Step 5: Implementasikan `lateness.go`.**
  - `latenessPeriods(ctx, back)` memakai `periodContaining(today)`, lalu mundur `periodContaining(AddDaysISO(p.Start, -1))`.
  - SQL hari terlambat:
    ```sql
    SELECT a.employee_id::text, a.date::text, s.name,
           to_char(a.scheduled_start AT TIME ZONE 'Asia/Jakarta','HH24:MI'),
           to_char(a.clock_in AT TIME ZONE 'Asia/Jakarta','HH24:MI'),
           a.late_minutes
    FROM hris.attendance a
    LEFT JOIN hris.shifts s ON s.id = a.shift_id
    WHERE a.employee_id = ANY($1::uuid[]) AND a.is_late AND a.date BETWEEN $2 AND $3
    ORDER BY a.date
    ```
  - Nama dan jabatan tim: `teamAttendees` + `SELECT e.id, e.full_name, p.title FROM hris.employees e LEFT JOIN hris.positions p ON p.id = e.job_title_id WHERE e.id = ANY($1)`.
  - Handler: `h.withActor`. Rute `GET /api/hris/me/lateness` dan `GET /api/hris/me/team-lateness`.
- [ ] **Step 6:** Jalankan `cd backend && make generate`. Tambahkan kedua path ke `goNativePrefixes` beserta komentar EPIC-077, dan ke `it.each` forward-ke-Go di `backend-routes.test.ts`.
- [ ] **Step 7:** Jalankan `go vet ./internal/modules/hris/... && go test -p 1 ./internal/modules/hris/... ./internal/app/...` dan `pnpm vitest run src/lib/backend-routes.test.ts`. Harus PASS.
- [ ] **Step 8: Commit** `feat(hris): jumlah & tingkat keterlambatan per periode untuk ESS dan atasan (EPIC-077)`.

### Task 2: Frontend — fitur `lateness`, tiga kartu, tes

**Files:**
- Create: `frontend/src/features/hris/lateness/{types.ts,api.ts,query-keys.ts,queries.ts,index.ts}`
- Create: `frontend/src/lib/hris/lateness-view.ts` (+ `lateness-view.test.ts`)
- Create: `frontend/src/features/hris/lateness/components/{lateness-level-badge.tsx,lateness-ladder.tsx,lateness-period-select.tsx,lateness-days-list.tsx,my-lateness-card.tsx,lateness-beranda-card.tsx,team-lateness-card.tsx}` + tes `my-lateness-card.test.tsx`, `team-lateness-card.test.tsx`, `lateness-beranda-card.test.tsx`
- Modify: `features/hris/ess/components/absensi-page.tsx` (kartu di bawah header clock, `id="keterlambatan"`), `beranda-page.tsx` (sesudah `TeamAttendanceCard`), `absensi-tim-page.tsx` (di atas `AttendanceMonitoringTab`)

**Interfaces:**
- Consumes: JSON Task 1.
- Produces:
  - `useMyLateness(back: number)` dan `useTeamLateness(back: number)`, dengan query key `["hris","lateness","me",back]` / `["hris","lateness","team",back]`.
  - `LATENESS_TONE: Record<LatenessCode, {badge, bar, text, soft}>` (kelas Tailwind): none emerald, teguran amber, sp1 orange, sp2 red, sp3 rose-900/rose-950.
  - `nextLevelHint(next): string | null` → "1 kali terlambat lagi → Surat Peringatan 1"; null → null.
  - `LATENESS_POLICY`: teks user verbatim, dibagi `{ levels: string[], intro: string, steps: string[] }`.
  - `formatLateMinutes(min: number): string`: "45 mnt", "1 jam 5 mnt".

- [ ] **Step 1: Tes view gagal** (`lateness-view.test.ts`):
  - `nextLevelHint({code:"sp1",label:"Surat Peringatan 1",at:3,remaining:1})` = "1 kali terlambat lagi → Surat Peringatan 1"; null → null.
  - `formatLateMinutes`: 45 → "45 mnt", 65 → "1 jam 5 mnt", 120 → "2 jam".
  - `LATENESS_POLICY.levels` panjang 4; elemen pertama memuat "0 – 2x".
- [ ] **Step 2: Implementasikan view dan modul fitur.** Jalankan ulang tes; harus PASS.
- [ ] **Step 3: Tes komponen gagal.**
  - `my-lateness-card.test.tsx` (mock `useMyLateness`, `useEssMe`):
    - count 3/sp1 menampilkan "3", "kali terlambat", badge "Surat Peringatan 1", hint "2 kali terlambat lagi → Surat Peringatan 2", 3 baris hari, dan catatan HR;
    - count 0 menampilkan "Tidak ada keterlambatan" tanpa daftar hari;
    - `has_schedule` false menampilkan "Jadwal shift belum diatur HR";
    - tombol "Ketentuan Keterlambatan & Absensi" membuka teks yang memuat "check-in";
    - mengganti periode memanggil hook dengan `back` 1.
  - `team-lateness-card.test.tsx`:
    - chip "SP 3 · 1" memfilter daftar;
    - anggota tidak langsung menampilkan "via Koordinator K";
    - daftar kosong menampilkan "Semua anggota tepat waktu";
    - tanpa tim (summary semua 0, employees `[]`) kartu tidak dirender.
  - `lateness-beranda-card.test.tsx`: tautan `/dashboard/me/absensi#keterlambatan` dan teks jumlah.
- [ ] **Step 4: Implementasikan komponen.**
  - Card dengan `min-w-0`.
  - Jumlah ditulis `text-4xl font-bold tabular-nums`.
  - Tangga = 5 segmen `grid grid-cols-5`; label pendek "0", "1–2", "3–4", "5–6", ">6" di bawah, segmen aktif ditebalkan.
  - Pemilih periode: `Select` `w-full sm:w-56`.
  - Daftar hari: `<ul>` kartu baris (tanggal kiri, "masuk 08:12 · telat 12 mnt" kanan, wrap di HP).
  - Ketentuan: tombol toggle (`aria-expanded`).
  - Tim: chip `flex flex-wrap gap-2` `min-h-10`, anggota dalam `<details>`/toggle yang menampilkan hari.
- [ ] **Step 5:** Pasang kartu ke tiga halaman. Jalankan `pnpm vitest run src/features/hris/lateness src/lib/hris/lateness-view.test.ts src/features/hris/ess`; harus PASS.
- [ ] **Step 6:** Jalankan `pnpm lint && pnpm exec tsc --noEmit`; keduanya harus bersih.
- [ ] **Step 7: Commit** `feat(hris): kartu keterlambatan ESS, Beranda dan Absensi Tim (EPIC-077)`.

### Task 3: Acuan MOM + QA

- [ ] Update dokumen acuan:
  - `docs/hris/acuan-mom-20261019/status-requirement.md`: AB-07 menjadi "Disepakati (TnC 02-11)", bukti ditulis dengan `file:line`.
  - `pertanyaan-rekomendasi.md`: tambah baris TnC 02-11.
  - `.claude/skills/paddy-hris-acuan/SKILL.md`: tambah baris "Keterlambatan" di tabel aturan kunci.
- [ ] Deploy staging, lalu QA dengan data demo:
  - Akun karyawan: 3x dan 7x terlambat.
  - Akun atasan dengan tim.
  - Viewport laptop dan HP 360 px.
  - Cek: tanpa scroll horizontal, angka benar, filter tim berfungsi.
  - Data demo dibersihkan sesudahnya.
- [ ] Commit `docs: EPIC-077 acuan AB-07`, lalu PR.

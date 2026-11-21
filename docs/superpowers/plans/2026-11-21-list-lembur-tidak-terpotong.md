# List Lembur per Bulan Tidak Terpotong 200 Baris (EPIC-126) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** HR yang membuka HRIS → Kepegawaian → Lembur untuk satu bulan melihat semua lembur bulan itu, termasuk tanggal-tanggal awal bulan.

**Architecture:** `store.OvertimeList` (repo `erp-paddy`, `apps/backend/internal/modules/hris/overtime.go:666-697`) selalu menutup query dengan `ORDER BY date DESC, created_at DESC LIMIT 200`. Saat satu bulan berisi lebih dari 200 lembur, baris tanggal paling awal terbuang. Perbaikannya: batas 200 hanya berlaku bila tidak ada filter tanggal. Filter bulan sendiri sudah membatasi jumlah baris. Fallback Next (`apps/frontend/src/lib/hris/overtime-repo.ts` `listOvertime`) diubah sama supaya paritas Go/Next terjaga.

**Tech Stack:** Go (`apps/backend`), tes integrasi DB paket `hris`. Next.js fallback (TypeScript). UI tidak berubah.

**Spec:** [EPIC-126](../../epics/EPIC-126-list-lembur-tidak-terpotong.md) (dibuat di Task 2). Laporan user 21-11-2026: "lembur list data tgl 1-5 november tidak ada di list nya".

Analisa read-only 21-11-2026:
- **Production:** ada 281 lembur bertanggal 1–21 Nov. UI mengirim `month=11&year=2026` (status "Semua"), dan Go mengembalikan 200 baris terbaru. 81 baris terbuang: semua tanggal 1–5 Nov (65 baris) dan 16 dari 18 baris tanggal 6 Nov.
- **Staging:** hanya ada 114 lembur November, jadi tidak ada yang terpotong dan bug tidak muncul di staging.
- Datanya ada di DB. Yang tidak ada hanya tampilannya, dan payroll tidak terpengaruh.
- **Contoh Sri Wardani (screenshot user 21-11 19.36):**
  - ESS Riwayat Lembur menampilkan lembur 1 dan 3 Nov, karena ESS memanggil `employee_id=me` tanpa bulan dan memakai batas 200 per karyawan.
  - Di list HR, kedua baris itu ada di urutan ke-279 dan ke-249, jadi terpotong. Lembur 14 dan 15 Nov ada di urutan 110 dan 92, jadi tampil.
  - Slip November (run draft, dihitung 21-11 08.29) memuat 25 jam: 13 jam biasa (22, 23, 29 Okt; 1, 3 Nov) dan 12 jam libur (25 Okt; 14, 15 Nov). Payroll membaca tabel langsung, bukan list ini.
- **Batas bawah list bergeser:**
  - Saat screenshot diambil, baris terakhir yang tampil (Kania Dewi Lastari, 6 Nov) tepat di urutan ke-200.
  - Tiga pengajuan tanggal 21 Nov yang masuk pukul 20.12, 20.20, dan 21.32 menurunkannya ke urutan ke-203.
  - Setiap pengajuan baru menyembunyikan satu baris lama lagi.
- **Isi 81 baris tersembunyi:**
  - 74 `approved` (2 di antaranya pernah dikoreksi HR) dan 7 `cancelled`.
  - Tidak ada `pending`, jadi belum ada pengajuan yang tidak bisa diputus. Pengajuan pending lama akan ikut tersembunyi begitu volume bertambah.
  - HR juga tidak bisa membuka Detail atau Koreksi baris yang tersembunyi.

## Global Constraints

- `GET /api/hris/overtime` dengan `month` dan `year` yang valid mengembalikan semua baris di rentang bulan itu (`OvertimeMonthFilter` → 1 s.d. akhir bulan kalender). Batas 200 tidak berlaku.
- Tanpa filter bulan (ESS `employee_id=me`, atasan `scope=approvals` tanpa bulan, HR yang mengosongkan bulan) perilaku tetap: 200 baris terbaru.
- Urutan tetap `date DESC, created_at DESC`.
- Filter bulan tetap bulan kalender. Tidak ada perubahan ke periode 21–20.
- Tidak ada migrasi, tidak ada perubahan setting, tidak ada perubahan UI.

## Review Focus

- HR mengosongkan input bulan: list kembali dibatasi 200 baris terbaru. Ini perilaku lama dan sengaja dipertahankan. Tes tanpa bulan di Task 1 Step 1 mengunci perilaku ini.
- Lembur di luar bulan (31 Okt, 1 Des) tidak ikut masuk walaupun batasnya dicabut. Tes Task 1 Step 1 mengunci rentang ini.
- Atasan `scope=approvals` dengan bulan: ikut tanpa batas, karena jumlahnya hanya lembur anak buah sebulan. Filter `source = 'employee'` tetap berlaku. Tes approvals yang sudah ada tidak diubah.
- Ukuran respons: satu bulan di production sekarang sekitar 300 baris. Batas teoretisnya baris aktif unik per karyawan per tanggal, ditambah baris yang ditolak atau dibatalkan. Volume ini aman tanpa paginasi.
- Pengajuan Absensi (`attendance_requests.go:393`) punya `LIMIT 200` tanpa filter bulan: di production 471 baris, dengan pending selalu di atas. Itu di luar plan ini; catat sebagai follow-up di epic dan jangan ubah di sini.

---

### Task 1: Batas 200 hanya tanpa filter bulan (Go + fallback Next)

**Files:**
- Create: `apps/backend/internal/modules/hris/overtime_list_integration_test.go`
- Modify: `apps/backend/internal/modules/hris/overtime.go:666-697` (`store.OvertimeList`)
- Modify: `apps/frontend/src/lib/hris/overtime-repo.ts` (`listOvertime`, `.limit(200)` di sekitar baris 73)

**Interfaces:**
- Produces: tanda tangan `func (s *store) OvertimeList(ctx context.Context, f overtimeFilter) ([]*Row, error)` tidak berubah. Konstanta paket `overtimeListCap = 200` dipakai hanya bila `f.DateFrom == ""`.

- [ ] **Step 1: Tulis tes yang gagal**

Buat `TestOvertimeListMonthNotCapped` di file baru, memakai pola `TestApproverReadsRequestDetails` (`approver_read_integration_test.go`):

- `hr, _ := h.staff("pos", []string{iam.LeafLembur}, true, emp{name: "HR Lembur"})` dan `_, crew := h.staff("pos", nil, true, emp{name: "Crew Lembur"})`.
- Isi data langsung lewat `h.exec` ke `hris.overtime_requests` untuk `crew`:
  - 1 baris `approved` tanggal `2026-11-01`, alasan `"Lembur awal bulan"`;
  - 225 baris `rejected` tanggal `2026-11-06` s.d. `2026-11-30`, 9 baris per tanggal (pakai `generate_series`; indeks unik `overtime_requests_active_unique` hanya berlaku untuk pending/approved);
  - 1 baris `approved` tanggal `2026-10-31` dan 1 baris `approved` tanggal `2026-12-01`.
  - Kolom wajib lain diisi seperti INSERT di tes yang ada (`start_time '18:00'`, `end_time '20:00'`, `hours 2`).
- `GET /api/hris/overtime?month=11&year=2026` sebagai `hr`:
  - status 200 dan `len(list()) == 226`;
  - ada baris dengan `date == "2026-11-01"`;
  - tidak ada baris `2026-10-31` atau `2026-12-01`.
- `GET /api/hris/overtime` (tanpa bulan) sebagai `hr` → `len(list()) == 200`.

- [ ] **Step 2: Jalankan dan pastikan gagal**

Run: `cd apps/backend && TEST_DATABASE_URL=<DB tes> $HOME/.local/go/bin/go test ./internal/modules/hris/ -run TestOvertimeListMonthNotCapped -count=1`
Expected: FAIL. Request dengan bulan mengembalikan 200 baris dan tidak ada baris `2026-11-01`.

- [ ] **Step 3: Ubah `store.OvertimeList`**

Tambah `const overtimeListCap = 200`. Sambung `LIMIT` ke query hanya bila `f.DateFrom == ""`. Komentar fungsi menyebut EPIC-126: filter bulan sudah membatasi baris, jadi batas hanya untuk list tanpa bulan.

- [ ] **Step 4: Jalankan tes paket**

Run: `cd apps/backend && TEST_DATABASE_URL=<DB tes> $HOME/.local/go/bin/go test ./internal/modules/hris/ -count=1`
Expected: lulus, kecuali kegagalan yang sudah ada di baseline `development` (catat namanya sebelum mulai).

- [ ] **Step 5: Samakan fallback Next**

Di `listOvertime` (`overtime-repo.ts`): `.limit(200)` dipasang hanya bila `monthFilter(q.month, q.year)` bernilai `null`. Rentang `date` tetap dipasang bila filter valid.

Run: `cd apps/frontend && npx tsc --noEmit -p . && npx vitest run src/lib/hris src/features/hris/overtime`
Expected: tanpa error tipe dan tes lulus.

- [ ] **Step 6: Commit**

```bash
git add apps/backend/internal/modules/hris apps/frontend/src/lib/hris/overtime-repo.ts
git commit -m "fix(hris): list lembur per bulan tidak terpotong 200 baris (EPIC-126)"
```

### Task 2: Epic dan dokumen

**Files (repo paddydev, branch `docs/rencana-rilis-prod-erp-paddy`):**
- Create: `docs/epics/EPIC-126-list-lembur-tidak-terpotong.md` (format EPIC-125). Status `review`.
- Modify: `docs/epics/README.md`: baris registry `MODULE-HRIS`.

- [ ] **Step 1:** Tulis epic. Isinya:
  - analisa production/staging di atas (281 vs 114 baris, 81 terbuang, tanggal 1–6 Nov);
  - penegasan bahwa data dan payroll aman;
  - aturan baru dari Global Constraints;
  - follow-up Pengajuan Absensi `LIMIT 200`;
  - langkah rilis: PR ke `development`, staging, gladi di salinan production (cek 281 baris November tampil), rilis production atas izin user. Rilis ini tanpa migrasi.
- [ ] **Step 2: Commit**

```bash
git add docs/epics
git commit -m "docs: EPIC-126 list lembur per bulan tidak terpotong 200 baris"
```

## Self-review

- **Cakupan:**
  - tanggal 1–5 Nov tampil → Task 1 Step 1 (baris `2026-11-01` di antara 226 baris);
  - rentang bulan tetap → Step 1 (31 Okt dan 1 Des tidak masuk);
  - tanpa bulan tetap 200 → Step 1;
  - paritas Next → Step 5;
  - dokumen → Task 2.
- **Review Focus:** HR mengosongkan bulan dan rentang bulan diuji di Step 1; approvals dicakup tes yang ada; ukuran respons sudah dianalisa; Pengajuan Absensi dicatat sebagai follow-up.
- **Proporsi:** satu kondisi di Go, satu di Next, satu tes baru.

---

## Perubahan atas permintaan user (21-11-2026, saat eksekusi)

- "pastikan semua list data lembur terlihat semua jangan ada lagi yang terpotong": batas dicabut di semua list lembur, baik dengan maupun tanpa bulan:
  - `OvertimeList`;
  - antrean Persetujuan Tim (`TeamPendingOvertime`, sebelumnya 200);
  - riwayat lembur tim (`TeamOvertimeHistory`, sebelumnya 300);
  - fallback Next.
  - Constraint "tanpa bulan tetap 200" gugur.
  - Widget Beranda "5 lembur terbaru" tetap, karena itu ringkasan.
- "tambahkan filter dan pagination jika belum ada": Task 3.
- Rilis: staging + QA, lalu backup + production + QA, atas izin user di pesan yang sama.

### Task 3: Filter dan pagination list lembur (klien)

**Files:**
- Create: `apps/frontend/src/lib/hris/overtime-list.ts` + `overtime-list.test.ts`
- Create: `apps/frontend/src/features/hris/overtime/components/overtime-pager.tsx`
- Modify: `features/hris/overtime/components/overtime-page.tsx` (+ test), `features/hris/ess/components/lembur-page.tsx` (+ test), `features/hris/team-approvals/components/team-history.tsx` (+ test)

**Interfaces:**
- `searchOvertimeRows<T>(rows: T[], query: string): T[]`: cocokkan nama karyawan, NIP, dan alasan, tanpa membedakan huruf besar/kecil. Query kosong mengembalikan semua baris.
- `filterOvertimeMonth<T extends { date: string }>(rows: T[], month: string): T[]`: `month` berformat `YYYY-MM`; `""` berarti semua.
- `paginate<T>(rows: T[], page: number, size: number): { rows: T[]; page: number; totalPages: number; total: number }`: halaman di-clamp ke `[1, totalPages]`, dan `totalPages` minimal 1.
- `OvertimePager({ page, totalPages, total, onPageChange })`: menampilkan "Halaman x dari y · N lembur" dengan tombol Sebelumnya/Berikutnya. Tidak dirender bila hanya ada 1 halaman.

**Nilai:**
- HR: 25 baris per halaman, pencarian "Cari nama / NIP / alasan…".
- ESS dan Riwayat Tim: 10 baris per halaman; ESS mendapat filter bulan (kosong = semua).
- Mengubah filter mengembalikan list ke halaman 1.

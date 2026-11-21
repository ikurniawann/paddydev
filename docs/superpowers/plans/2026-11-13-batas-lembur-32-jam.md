# Batas Lembur 32 Jam per Periode — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Batas akumulasi lembur periode berjalan turun dari 72 jam menjadi 32 jam. Batas 18 jam/minggu tetap. Perubahan ini masuk sebelum rilis production Fase 1.

**Architecture:** Batasnya satu konstanta, `domain.OvertimeMaxPeriodHours` (`apps/backend/internal/modules/hris/domain/overtime_rules.go:16`). `checkOvertimeCaps` (`overtime_tnc.go:49`) sudah memakainya di tiga jalur: pengajuan baru, approve dengan koreksi jam, dan koreksi HR yang menambah jam. Tidak ada migrasi dan tidak ada perubahan data. Yang berubah: konstanta, pesan dan hint UI, test, dan dokumen acuan.

**Tech Stack:** Go (modul `hris`, test integrasi `newFileHarness`), Next.js + vitest (`lib/hris/overtime-rules.ts`).

**Spec:** Permintaan user 13-11-2026: *"sebelum nya ada perubahan terkait maksimal jam lembur periode berjalan yaitu request perubahan: menjadi 32 jam perbulan ya mas"*. Epic: `docs/epics/EPIC-109-batas-lembur-32-jam.md`. Aturan asal: EPIC-085 R3 dan EPIC-089 R1.

## Global Constraints

- Batas periode **32 jam**. "Per bulan" berarti per **periode absensi**: dengan cutoff live 21, periodenya 22 s.d. 21. Yang dihitung adalah pengajuan **pending + approved**, tanpa baris yang sedang diubah. Cara hitung ini sama dengan sekarang.
- Batas minggu tetap **18 jam** (Senin–Minggu). Urutan cek tetap minggu dulu, lalu periode.
- Pesan tolak periode: `Lembur periode {label} sudah {x} jam; maksimal 32 jam per periode (sisa {y} jam)`. Format `jam()` tidak berubah (koma desimal, 2 desimal, sisa tidak negatif).
- Hint UI (`OVERTIME_RULES_HINT`), persis: `Lembur hari kerja dimulai setelah jam kerja selesai · lebih dari 4 jam/hari wajib keterangan · maks. 18 jam/minggu, 32 jam/periode`
- Baris lembur lama tidak diubah. Koreksi yang **mengurangi** jam tetap boleh walau total periodenya sudah di atas 32. Ini perilaku yang ada (`overtime_correction.go:124`, `hours > ot.Num("hours")`) dan wajib dipertahankan.
- Kode dikerjakan di worktree baru `paddydev/.worktrees/lembur-32`, dari `origin/development` erp-paddy, branch `feat/lembur-batas-32-jam`. Hasilnya PR ke `development`, tidak push langsung. `~/WORK/erp-paddy` tidak disentuh.
- Dokumen (epic, rencana, acuan) hanya ditaruh di repo paddydev, karena .gitignore erp-paddy menolak docs.
- Production tidak diubah oleh rencana ini. Perubahan naik ke production lewat Fase 1 rencana rilis, setelah gladi ulang dan izin eksplisit user.

## Keputusan

**K-1 (diputuskan user 13-11: A):** pengajuan pending yang sudah ada dan membuat total di atas 32.** Saat ini approve **tanpa** koreksi jam tidak mengecek batas sama sekali (`overtime.go:446`, hanya bila `correctedHours > 0`). Data live untuk periode 22 Okt – 21 Nov (read-only, 13-11 17.40 WIB):
- prod: 1 karyawan, 37,75 jam, 8 baris **pending**;
- staging: 1 karyawan, 36,25 jam.

Pilihan:
- **A (rekomendasi): pengajuan yang sudah masuk tetap bisa di-approve.** Atasan/HR masih bisa menolak atau mengoreksi jamnya. Batas 32 berlaku untuk pengajuan baru dan koreksi yang menambah jam. Pilihan ini tidak menambah kode; Task 1 Step 6 mengunci perilakunya dengan test.
- **B: approve tanpa koreksi juga dicek.** Hitungannya: total **approved** + jam baris itu ≤ 32 (dan ≤ 18 per minggu). Pending tidak ikut dihitung, supaya atasan bisa memilih baris mana yang di-approve. Ini dikerjakan di Task 3. Dampaknya di prod: dari 37,75 jam pending, atasan hanya bisa meng-approve sampai 32 jam.

User memilih **A** (13-11-2026, "A: tetap bisa approve"). Task 3 tidak dikerjakan.

## Review Focus

1. **Tepat 32 jam:** `periodUsed + hours == 32` harus diterima. Ini dipin di Task 1 Step 1, kasus "tepat batas".
2. **Karyawan yang sudah di atas 32 mengoreksi turun:** PATCH HR yang mengurangi jam harus 200, dan yang menambah jam harus 400 dengan pesan periode. Dipin di Task 1 Step 3 (`TestOvertimeTnCPeriodCap32`).
3. **Baris di luar periode atau berstatus ditolak tidak dihitung:** baris Oktober (periode lalu) dan baris `rejected` tidak boleh menambah total. Dipin di Task 1 Step 3, kasus (f).
4. **Pesan minggu tetap muncul lebih dulu** bila batas minggu dan periode sama-sama terlewati. Kasus unit "lewat batas minggu" yang sudah ada tetap hijau; tidak ada test baru.
5. **Approve pending lama (K-1):** dengan pilihan A, approve tanpa koreksi tetap 200 walau total di atas 32. Dipin di Task 1 Step 6.

---

### Task 1: Batas periode 32 jam di backend

**Files:**
- Modify: `apps/backend/internal/modules/hris/domain/overtime_rules.go:12,16,76` (konstanta dan komentar 72 → 32)
- Modify: `apps/backend/internal/modules/hris/overtime_tnc.go:48`, `overtime.go:206` (komentar 72 → 32)
- Test: `apps/backend/internal/modules/hris/domain/overtime_rules_test.go` (`TestOvertimeCapError`)
- Test: `apps/backend/internal/modules/hris/overtime_tnc_integration_test.go` (`TestOvertimeTnC` kasus (f), plus `TestOvertimeTnCPeriodCap32` baru)

**Interfaces:**
- Consumes: `domain.OvertimeCapError(hours, weekUsed, periodUsed float64, weekStart, weekEnd, periodLabel string) string` (tidak berubah); helper test `h.otRow(employeeID, date string, hours float64, status string)`, yang menyisipkan baris 18:00–23:00 `company`/`regular`.
- Produces: `domain.OvertimeMaxPeriodHours = 32`. Konstanta ini tidak dipakai frontend.

- [ ] **Step 1: Ubah unit test ke batas 32**

Di `TestOvertimeCapError`, ubah dua kasus berikut. Kasus lainnya biarkan.

```go
{"lewat batas periode", 2, 0, 30.5,
    "Lembur periode 21 Okt – 20 Nov 2026 sudah 30,5 jam; maksimal 32 jam per periode (sisa 1,5 jam)"},
{"tepat batas", 2, 16, 30, ""},
```

Tambahkan satu kasus:

```go
{"periode lama 72 kini ditolak", 1, 0, 40,
    "Lembur periode 21 Okt – 20 Nov 2026 sudah 40 jam; maksimal 32 jam per periode (sisa 0 jam)"},
```

- [ ] **Step 2: Jalankan dan pastikan gagal**

Run: `cd apps/backend && go test ./internal/modules/hris/domain -run TestOvertimeCapError`
Expected: FAIL. Kasus "lewat batas periode" dan "periode lama 72 kini ditolak" mendapat `""`, karena 32,5 dan 41 masih di bawah 72.

- [ ] **Step 3: Ubah test integrasi**

Di `TestOvertimeTnC` kasus (f) (`overtime_tnc_integration_test.go:68-77`), ganti 14 baris `busy` × 5 jam dengan isi berikut. Harness memakai cutoff 0, jadi periodenya November kalender.
- 6 baris `pending` × 5 jam: `2026-11-09`, `11-10`, `11-11`, `11-12`, `11-13`, `11-16`, total 30 jam;
- 2 baris `approved` × 5 jam di `2026-10-26` dan `2026-10-27` (periode lalu, tidak dihitung);
- 1 baris `rejected` × 5 jam di `2026-11-17` (tidak dihitung).

Assertion yang diharapkan:

```go
expectContains(t, post(&busy, "2026-11-02", "18:00", "21:00"), 400,
    "sudah 30 jam; maksimal 32 jam per periode (sisa 2 jam)")
```

Tambahkan juga test baru:

```go
// EPIC-109: karyawan yang sudah di atas 32 jam tetap bisa dikoreksi turun,
// tetapi tidak bisa ditambah.
func TestOvertimeTnCPeriodCap32(t *testing.T)
```

Isinya:
- `newFileHarness`, lalu `h.setNow("2026-11-24 09:00")`;
- HR `hrd` dengan permission `hris.kepegawaian.overtime`, dan staf `own`;
- `h.otRow(own, d, 5, "approved")` untuk `2026-11-02`, `11-03`, `11-09`, `11-10`, `11-16`, `11-17`, total 30 jam;
- satu baris `11-23` 5 jam lewat `h.scalar(... RETURNING id::text)`, dengan bentuk yang sama seperti `TestOvertimeTnCLegacyCorrection`, start 18:00, end 23:00, `approved`. Total menjadi 35 jam.

Assertion:
- PATCH baris 11-23 `{"end_time": "23:30", "reason": "Menambah jam"}` → 400, berisi `sudah 30 jam; maksimal 32 jam per periode (sisa 2 jam)`;
- PATCH `{"end_time": "22:00", "reason": "Jam pulang sebenarnya"}` → 200.

- [ ] **Step 4: Jalankan dan pastikan gagal**

Run: `cd apps/backend && TEST_DATABASE_URL=… go test ./internal/modules/hris -run 'TestOvertimeTnC$|TestOvertimeTnCPeriodCap32' -count=1`
Expected:
- (f) FAIL dengan status 200, bukan 400 (33 jam masih di bawah 72);
- `TestOvertimeTnCPeriodCap32` FAIL di PATCH 23:30 dengan 200, bukan 400.

- [ ] **Step 5: Ubah konstanta**

Set `OvertimeMaxPeriodHours = 32` di `overtime_rules.go`. Komentar `72` di `overtime_rules.go:12,76`, `overtime_tnc.go:48` dan `overtime.go:206` diganti `32`, ditambah catatan `(EPIC-109)`.

- [ ] **Step 6: Pin K-1 pilihan A**

Di akhir `TestOvertimeTnCPeriodCap32`, tambahkan:
- staf kedua `crew` dengan `h.otRow(crewID, d, 5, "pending")` di 7 tanggal yang sama (35 jam pending);
- HR `POST /api/hris/overtime/decide` `{"overtime_id": <id baris 11-23>, "action": "approve"}` tanpa `end_time` → 200.

Ambil id-nya dengan `h.scalar("SELECT id::text FROM hris.overtime_requests WHERE employee_id=$1 AND date='2026-11-23'", crewID)`.

Kalau user memilih **B**, assertion ini diganti di Task 3.

- [ ] **Step 7: Jalankan dan pastikan lulus**

Run: `cd apps/backend && go test ./internal/modules/hris/domain -run TestOvertimeCapError && TEST_DATABASE_URL=… go test ./internal/modules/hris -run 'TestOvertime' -count=1`
Expected: PASS. Kalau ada test lembur lain yang gagal karena totalnya > 32, tanggalnya dipindah ke periode lain. Batasnya jangan dilonggarkan.

- [ ] **Step 8: Commit**

```bash
git add apps/backend/internal/modules/hris
git commit -m "feat(hris): batas lembur periode 32 jam (EPIC-109)"
```

### Task 2: Hint UI 32 jam

**Files:**
- Modify: `apps/frontend/src/lib/hris/overtime-rules.ts:207-208`
- Test: `apps/frontend/src/lib/hris/overtime-rules.test.ts:198-202`

**Interfaces:**
- Produces: `OVERTIME_RULES_HINT`, dipakai di `lembur-page.tsx:278`. Test halaman membaca konstanta yang sama, jadi tidak perlu diubah.

- [ ] **Step 1: Ubah test**

Di `overtime-rules.test.ts`:
- string yang diharapkan diganti dengan hint persis dari Global Constraints;
- judul `it` diganti menjadi `"merangkum jam mulai, keterangan > 4 jam, dan batas 18/32 jam"`.

- [ ] **Step 2: Jalankan dan pastikan gagal**

Run: `cd apps/frontend && npx vitest run src/lib/hris/overtime-rules.test.ts`
Expected: FAIL, karena string masih berisi `72 jam/periode`.

- [ ] **Step 3: Ubah `OVERTIME_RULES_HINT`** ke hint persis dari Global Constraints.

- [ ] **Step 4: Jalankan dan pastikan lulus**

Run: `cd apps/frontend && npx vitest run src/lib/hris/overtime-rules.test.ts src/features/hris/ess/components/lembur-page.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/frontend/src/lib/hris/overtime-rules.ts apps/frontend/src/lib/hris/overtime-rules.test.ts
git commit -m "feat(hris-ui): hint batas lembur 32 jam/periode (EPIC-109)"
```

### Task 3: (TIDAK DIKERJAKAN — K-1 = A) Approve tanpa koreksi ikut dicek

**Files:**
- Modify: `apps/backend/internal/modules/hris/overtime.go:446-454`, `overtime_tnc.go`, repository lembur (lokasi `OvertimeHoursUsed`)
- Test: `overtime_tnc_integration_test.go` (`TestOvertimeTnCPeriodCap32` Step 6)

**Interfaces:**
- Produces:
  - `Repository.OvertimeHoursApproved(ctx context.Context, employeeID, from, to, excludeID string) (float64, error)`, sama seperti `OvertimeHoursUsed` tetapi hanya `status = 'approved'`;
  - `(*Service).checkOvertimeApproveCaps(ctx, r Repository, employeeID, date string, hours float64, id string) error`, dengan lock dan pesan yang sama seperti `checkOvertimeCaps`, tetapi memakai total approved.

- [ ] **Step 1: Balik test Step 6 Task 1**

Untuk 7 baris pending `crew` (35 jam):
- approve 6 baris pertama (30 jam) → masing-masing 200;
- approve baris ke-7 → 400, berisi `sudah 30 jam; maksimal 32 jam per periode (sisa 2 jam)`;
- reject baris ke-7 → 200.

- [ ] **Step 2: Jalankan dan pastikan gagal**

Expected: approve ke-7 mendapat 200, bukan 400.

- [ ] **Step 3: Implementasi**

Di `overtime.go`:
- untuk `action == "approve"` dengan `correctedHours == 0`, panggil `checkOvertimeApproveCaps` dengan `ot.Num("hours")` di dalam `InTx` yang sama, setelah `OvertimeForUpdate`;
- approve **dengan** koreksi tetap memakai `checkOvertimeCaps`;
- reject dan cancel tidak dicek.

- [ ] **Step 4: Jalankan dan pastikan lulus**

Run: `TEST_DATABASE_URL=… go test ./internal/modules/hris -run 'TestOvertime' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit** `feat(hris): approve lembur dicek batas 18/32 jam dari total approved (EPIC-109 K-1 B)`.

### Task 4: Gerbang, PR, staging

- [ ] **Step 1:** Jalankan gerbang lokal penuh:
  - `gofmt -l .` kosong;
  - `go vet ./...`;
  - `go test ./...` dengan `TEST_DATABASE_URL` ke `paddydev-sdd-testdb`;
  - di `apps/frontend`: `npx vitest run`, `npx tsc --noEmit`, `npx eslint .`.

  Expected: semua hijau, sama seperti baseline PR #4 (vitest ±6.675 passed, tsc 0, eslint 0).
- [ ] **Step 2:** Lakukan review independen atas diff penuh, di model paling kuat.
- [ ] **Step 3:** Push branch dan buka PR ke `development` (judul `feat(hris): batas lembur 32 jam per periode (EPIC-109)`). Tunggu CI hijau. **Merge hanya dengan izin user.**
- [ ] **Step 4:** Setelah merge, tunggu deploy staging otomatis dari CI push. Cek di staging dengan akun karyawan asli: hint di halaman Lembur menampilkan "32 jam/periode". Lalu QA HR: pengajuan yang membuat total > 32 ditolak dengan sisa jam di pesan.

### Task 5: Dokumen dan rilis

- [ ] **Step 1:** Di paddydev, catat Automation Log di `docs/epics/EPIC-109-batas-lembur-32-jam.md` dengan commit, hasil gerbang, dan keputusan K-1. Di `.claude/skills/paddy-hris-acuan/SKILL.md:40`, ubah `72 jam/periode` → `32 jam/periode (EPIC-109)`. Di EPIC-085 dan EPIC-089, tambahkan catatan "direvisi EPIC-109: 32 jam/periode".
- [ ] **Step 2:** Perbarui rencana rilis (`2026-11-13-rilis-production-erp-paddy.md`): head `development` yang akan dirilis sekarang memuat EPIC-108 (#5, #6) dan EPIC-109. Gladi 0.8 **wajib diulang** untuk head itu sebelum gerbang go/no-go Fase 1. Isinya: migrasi + verify, crawl 3 role + anon terhadap baseline, dan cek cookie lama.

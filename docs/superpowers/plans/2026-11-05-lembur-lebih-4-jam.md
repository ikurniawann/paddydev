# Lembur Lebih dari 4 Jam (EPIC-089) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Lembur lebih dari 4 jam boleh diajukan bila Keterangan diisi. Dokumentasi menjadi opsional. Batas keras tinggal 18 jam/minggu dan 72 jam/periode.

**Architecture:**
- Aturan murni di `hris/domain/overtime_rules.go`:
  - batas harian dihapus dari `OvertimeCapError`;
  - fungsi baru `OvertimeReasonError` untuk aturan Keterangan > 4 jam.
- Service `CreateOvertime` memakai keduanya. Cek 12 jam kotor dihapus dan lampiran menjadi opsional.
- Form ESS Lembur mencerminkan aturan yang sama di klien.

**Tech Stack:** Go (hris), Next.js/TS (vitest).

**Spec:** `docs/epics/EPIC-089-lembur-lebih-4-jam.md` (R1–R5).

## Global Constraints
- Pesan persis:
  - R2 "Lembur lebih dari 4 jam: isi Keterangan alasannya (minimal 10 karakter)";
  - hint "Lembur hari kerja dimulai setelah jam kerja selesai · lebih dari 4 jam/hari wajib keterangan · maks. 18 jam/minggu, 72 jam/periode";
  - label "Keterangan (wajib — lebih dari 4 jam)";
  - petunjuk "Jelaskan kenapa lembur lebih dari 4 jam";
  - catatan "Lembur diinput setelah selesai; dokumentasi opsional. Lembur yang disetujui atasan dibayar sesuai jam setelah dikurangi istirahat."
- Ambang: jam dibayar **> 4** (`OvertimeReasonThresholdHours = 4`); Keterangan minimal **10** karakter setelah trim (`OvertimeLongReasonMin = 10`).
- Pesan batas mingguan/periode EPIC-085 tidak berubah.
- Tes integrasi: `TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test_e084?sslmode=disable'`, `go test -p 1`, `PATH=$HOME/.local/go/bin:$PATH`.
- Trailer commit `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Jangan commit `.swarm`, `.claude-flow`, atau symlink `node_modules`.

## Review Focus
1. **Tepat 4,00 jam dibayar** (mis. 17:00–21:00, atau 17:00–22:00 dengan istirahat 60 menit): Keterangan tidak wajib. 4,5 jam wajib. Uji di Task 1 dan Task 2.
2. **Lembur lintas tengah malam 20:00–05:00** (9 jam): tidak lagi ditolak "12 jam"; Keterangan wajib; batas minggu tetap berlaku. Uji di Task 2.
3. **Minggu sudah 15 jam + pengajuan baru 5 jam** dengan Keterangan: tetap ditolak pesan 18 jam/minggu. Uji di Task 2.
4. **Keterangan berisi spasi saja / 9 karakter** pada > 4 jam: ditolak, karena trim. Uji di Task 1.
5. **Tanpa lampiran diterima**, tetapi lampiran milik orang lain tetap ditolak "Dokumentasi lembur tidak valid — unggah ulang filenya". Uji di Task 2.

---

### Task 1: Aturan domain

**Files:**
- Modify: `backend/internal/modules/hris/domain/overtime_rules.go`
- Test: `backend/internal/modules/hris/domain/overtime_rules_test.go`

**Interfaces:**
- Produces:
  - `func OvertimeCapError(hours, weekUsed, periodUsed float64, weekStart, weekEnd, periodLabel string) string` (parameter `dayUsed` dihapus; `OvertimeMaxDailyHours` dihapus);
  - `const OvertimeReasonThresholdHours = 4`, `OvertimeLongReasonMin = 10`;
  - `func OvertimeReasonError(hours float64, reason string) string`: "" bila `hours <= 4` atau `len([]rune(strings.TrimSpace(reason))) >= 10`; selain itu pesan R2.

- [ ] **Step 1: Failing tests.**
  - `TestOvertimeCapError`: kasus "lebih 4 jam sehari" diganti: `(hours 6, week 0, period 0)` → `""`. Kasus 18/72 dipertahankan dengan signature baru.
  - `TestOvertimeReasonError`:
    - `(4, "")` → "";
    - `(4.5, "")` → pesan R2;
    - `(4.5, "   ")` → pesan R2;
    - `(4.5, "123456789")` → pesan R2;
    - `(4.5, "Stok opname akhir")` → "".
- [ ] **Step 2: Run** `go test ./internal/modules/hris/domain/ -run 'OvertimeCapError|OvertimeReasonError'` (dari `backend/`). Expect FAIL (build error).
- [ ] **Step 3: Implement.** Perbarui pemanggil `checkOvertimeCaps` (`overtime_tnc.go`): hapus query harian.
- [ ] **Step 4: Run.** Expect PASS. `go build ./...` bersih.
- [ ] **Step 5: Commit** `feat(hris): batas lembur harian dihapus, aturan keterangan > 4 jam (EPIC-089)`.

### Task 2: Service lembur

**Files:**
- Modify:
  - `backend/internal/modules/hris/overtime.go` (create: lampiran opsional, R2, hapus cek 12 jam; approve-koreksi: hapus cek 12 jam)
  - `overtime_correction.go` (hapus cek 12 jam)
- Test:
  - `backend/internal/modules/hris/overtime_tnc_integration_test.go` (perbarui ekspektasi 4 jam/hari menjadi kasus > 4 jam)
  - `overtime_input_integration_test.go` (tanpa lampiran → 200)

**Interfaces:**
- Consumes: Task 1.
- `checkOvertimeAttachments(ownerID string, paths []string) error`: `len(paths)==0` → nil; validasi lain tetap.

- [ ] **Step 1: Failing tests.**
  - `TestOvertimeTnC`:
    - 18:00–22:30 (4,5 jam) tanpa Keterangan → 400 pesan R2;
    - dengan reason "Closing stok akhir bulan" → 200.
    - Assertion lama "Lembur maksimal 4 jam per hari" pada koreksi atasan (baris ~101) menjadi: koreksi atasan ke > 4 jam → 200 (tanpa cek Keterangan).
    - Kasus 20:00–05:00 dengan Keterangan → 200 (Review Focus 2).
    - Minggu 15 jam + 5 jam dengan Keterangan → 400 pesan mingguan (Review Focus 3).
  - `TestOvertimeTnCLegacyCorrection`: ekspektasi "Lembur maksimal 4 jam per hari" diganti dengan 200, atau dengan pesan batas mingguan bila datanya memang melewati 18 jam. Pilih sesuai fixture, dan catat dalam ruling.
  - `TestOvertimeInputAfterFinished`:
    - `post(noFile)` → 200;
    - lampiran milik orang lain → 400 "Dokumentasi lembur tidak valid — unggah ulang filenya" (Review Focus 5).
  - Penugasan HR 6 jam dengan alasan 5 karakter → 200 (R2 tidak berlaku).
- [ ] **Step 2: Run** `go test -p 1 ./internal/modules/hris/ -run 'OvertimeTnC|OvertimeInput'`. Expect FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** the tests, then `go test -p 1 ./internal/modules/hris/... ./internal/app/`, `go vet ./...`, `gofmt -l internal`. Expect PASS and no gofmt output.
- [ ] **Step 5: Commit** `feat(hris): lembur > 4 jam wajib keterangan, dokumentasi opsional, tanpa batas 12 jam (EPIC-089)`.

### Task 3: ESS Lembur + acuan

**Files:**
- Modify:
  - `frontend/src/features/hris/ess/components/lembur-page.tsx`
  - `frontend/src/lib/hris/overtime-rules.ts` (`OVERTIME_RULES_HINT`; tambah `overtimeReasonError(hours: number, reason: string): string | null` dengan aturan dan pesan yang sama dengan Task 1)
  - `.claude/skills/paddy-hris-acuan/SKILL.md`
  - `docs/epics/EPIC-085-ketentuan-lembur.md` (catatan R3 direvisi)
  - `frontend/src/features/hris/ess/components/overtime-attachments-field.tsx` (label "(opsional)", bila labelnya ada di sana)
- Test:
  - `frontend/src/lib/hris/overtime-rules.test.ts`
  - `frontend/src/features/hris/ess/components/lembur-page.test.tsx`

**Interfaces:**
- Consumes: pesan R2 (Global Constraints).

- [ ] **Step 1: Failing tests.**
  - `overtime-rules.test`: hint baru persis. `overtimeReasonError(4, "")` → null; `(4.5, "  ")` → pesan R2; `(4.5, "Stok opname akhir")` → null.
  - `lembur-page.test`:
    - tanpa lampiran, 17:00–19:00 → `submit` dipanggil (toast lampiran tidak ada);
    - 17:00–22:00 tanpa Keterangan → toast pesan R2 dan `submit` tidak dipanggil; label "Keterangan (wajib — lebih dari 4 jam)" tampil;
    - dengan Keterangan ≥ 10 karakter → `submit` dipanggil.
- [ ] **Step 2: Run** `npx vitest run src/lib/hris/overtime-rules.test.ts src/features/hris/ess/components/lembur-page.test.tsx` (dari `frontend/`). Expect FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** the tests, then:
  - `npx vitest run src/features/hris src/lib`;
  - `npx tsc --noEmit -p .`;
  - `npx eslint --quiet` pada file yang berubah.

  Expect PASS.
- [ ] **Step 5: Commit** `feat(frontend): form lembur — keterangan wajib > 4 jam, dokumentasi opsional (EPIC-089)`.

### Task 4: Review akhir, staging, PR
- [ ] Final review (reviewer terpisah, model paling mampu) → perbaikan.
- [ ] Backup staging, lalu deploy gabungan lokal EPIC-087 + EPIC-089 (EPIC-089 sudah memuat EPIC-088).
- [ ] QA HP dengan akun demo (dihapus sesudahnya):
  - lembur 2 jam tanpa dokumentasi → terkirim;
  - 5 jam tanpa Keterangan → ditolak;
  - 5 jam dengan Keterangan → terkirim, atasan melihat Keterangan;
  - akumulasi minggu > 18 → ditolak.
- [ ] Docs: Automation Log epic, `docs/epics/README.md`.
- [ ] PR ke `main` (bertumpuk di atas #76). Merge dan production hanya dengan izin user.

# Sisa Bawaan Dihitung Ulang Otomatis — Rencana (EPIC-068 tindak lanjut)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Status implementasi (27-10-2026):** Tugas 1-4 selesai di kode pada branch `feat/epic068-sisa-bawaan` (commit `efed744`..`ec39b82` + dokumen); **belum deploy**, setting sisa bawaan tetap mati. Catatan resmi: Automation Log EPIC-068.

**Goal:** Sisa bawaan tahun N selalu sama dengan sisa akhir tahun N−1 yang terbaru, dibatasi oleh batas maksimal yang diatur admin HRIS. Nilainya dihitung ulang otomatis di setiap perubahan saldo N−1, tanpa nilai negatif, dengan audit, dan karyawan yang terdampak ditandai "perlu ditinjau HR".

**Architecture:**
- **Fungsi inti** `recomputeCarryOver(ctx, q, employeeID, year, cause)` dipanggil di transaksi yang sama oleh semua jalur yang mengubah saldo N−1:
  - persetujuan, pembatalan, dan keputusan tolak cuti;
  - PUT manual HR dan import massal;
  - penyimpanan batas maksimal;
  - job 1 Januari.
- **Penanda tinjau:** baris di `hris.leave_carry_over_reviews`. Migrasi yang membuat tabel ini sekaligus membuka kunci setting yang dipasang di PR #33 (`to_regclass('hris.leave_carry_over_reviews')`).
- **CHECK** `carry_over_used ≤ carry_over_days` dilepas, sehingga nilai yang benar bisa disimpan walau di bawah yang sudah terpakai. Sisa bawaan yang tersedia dihitung `greatest(days − used, 0)`.

**Tech Stack:** Go (pgx), PostgreSQL 16, Next.js 16.4 + React Query + Vitest.

**Spec:** brief user 26-10-2026 "Perbaikan & pengaturan sisa bawaan (carry-over) cuti, EPIC-068 tindak lanjut" + jawaban:
- kedaluwarsa dipertahankan (diatur admin HRIS, default 31 Maret, bawaan dipakai lebih dulu);
- karyawan bertanda ditinjau HR dengan koreksi manual atau tandai selesai, tanpa pemotongan otomatis;
- batas awal kosong (tanpa batas);
- PR #33 dirilis dulu dengan setting terkunci.

Basis kode: branch `feat/epic068-saldo-cuti` (`e4b4ab6`). Epic: `docs/epics/EPIC-068-alokasi-saldo-cuti.md`.

## Global Constraints
- **Rumus:** `carry(N) = floor_0,5( min( annual_leave_remaining(N−1), max ) )`, dengan `annual_leave_remaining = total + adjustment − used`, tanpa memasukkan bawaan N−1 sendiri. `max` NULL = tanpa batas; 0 = tidak ada bawaan. Nilai negatif dari N−1 dianggap 0.
- **Berlaku hanya bila** setting `carry_over_enabled` menyala dan kunci terbuka (tabel review ada). Bila setting mati: tidak ada perhitungan ulang dan job tidak membuat bawaan. Nilai yang sudah ada tidak dihapus saat setting dimatikan.
- **Hanya baris N yang dikelola otomatis** (`carry_over_source = 'auto'`) yang dihitung ulang. Bawaan yang diisi HR secara manual atau lewat import (`'manual'`) tidak ditimpa.
- **Hanya karyawan aktif.** Karyawan resign/nonaktif tidak dibuatkan baris tahun baru dan tidak dihitung ulang.
- **Kedaluwarsa:** baris otomatis memakai tanggal setting di tahun N, diisi saat pertama dibuat. Perubahan tanggal setting tidak mengubah baris yang sudah ada.
- **Tidak negatif:** bila `carry_new < carry_over_used`, simpan `carry_over_days = carry_new` dan buka/perbarui baris review (`excess = used − new`). Transaksi asal tetap jalan. Bila kemudian `carry_new ≥ used`, review terbuka otomatis ditutup (`resolution = 'otomatis'`).
- **Audit setiap perubahan bawaan:**
  - `audit.Write` dengan Entity `hris.employees`, EntityID karyawan, action `leave_balance.carry_over`;
  - Before/After `{carry_over_days, carry_over_used, carry_over_expires_on}`;
  - Reason = penyebab terbaca (mis. "Cuti <id> disetujui", "Koreksi HR", "Import massal", "Batas sisa bawaan diubah", "Job 1 Januari");
  - After memuat `cause: {type, leave_id?}` dan aktor (user atau "sistem").
  - Tanpa perubahan nilai = tanpa audit.
- **Setting batas:** kelipatan 0,5, ≥ 0, ≤ 366, kosong = tanpa batas. Hanya aksi `update` pada daun `hris.workforce.leave-balances`. **Alasan wajib** untuk setiap perubahan setting (audit `leave_settings.update` dengan Reason).
- **Pratinjau dampak** (`POST /api/hris/leave-settings/preview`): memakai fungsi yang sama dengan penyimpanan dalam transaksi yang di-rollback, jadi hasil pratinjau = hasil nyata.
- **Pesan Indonesia;** `go vet`/gofmt; route manifest diperbarui; test DB `postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable`, `go test -p 1`.

## Ruling
- **R1 Sumber bawaan.** Kolom `carry_over_source text NOT NULL DEFAULT 'auto' CHECK (in ('auto','manual'))`.
  - PUT manual / import yang **mengubah** `carry_over_days` atau `carry_over_expires_on` → `'manual'`.
  - Mengosongkan bawaan lewat PUT (`carry_over_days = 0` dan kedaluwarsa null) mengembalikan ke `'auto'`.
  - Migrasi: baris yang sudah punya bawaan atau kedaluwarsa → `'manual'`; sisanya `'auto'`.
- **R2 Hook.** Setelah perubahan saldo tahun Y di jalur mana pun, panggil `recomputeCarryOver(..., Y+1, cause)`, tetapi hanya bila baris Y+1 sudah ada. Job dan setting tetap membuat baris lewat `ensureLeaveBalance`. Jalurnya:
  - `consumeAnnualLeave` (approve);
  - `CancelLeave` (refund);
  - keputusan tolak (tanpa perubahan saldo → no-op);
  - `UpdateLeaveBalanceManual`;
  - commit import (per karyawan yang berubah).
- **R3 Job 1 Januari.** Klaim per tahun tetap. Di dalam klaim:
  - `ensureLeaveBalance` untuk karyawan aktif;
  - `recomputeCarryOver(cause=job)` untuk setiap baris auto;
  - hasilnya deterministik (rumus yang sama), jadi menjalankan ulang setelah klaim dihapus memberi hasil sama.
- **R4 Setting batas.** Perubahan batas (atau menyalakan setting) menghitung ulang **tahun berjalan** untuk semua karyawan aktif dengan baris auto, dalam satu transaksi. Tahun sebelumnya tidak diubah. Mematikan setting tidak menghitung ulang.
  - Pratinjau: `{employees_changed, days_increase, days_decrease, needs_review}`.
- **R5 Review.**
  - `hris.leave_carry_over_reviews(id uuid PK, employee_id, year, carry_over_days, carry_over_used, excess, cause jsonb, opened_at, resolved_at NULL, resolved_by uuid NULL, resolved_by_name text NULL, resolution text NULL)`, unik `(employee_id, year) WHERE resolved_at IS NULL`.
  - API: `GET /api/hris/leave-balances/reviews?year=` (read) dan `POST /api/hris/leave-balances/reviews/{id}/resolve {note}` (update; catatan wajib, audit `leave_balance.review_resolve`).
  - Koreksi manual memakai PUT yang ada; review tertutup otomatis bila excess hilang.
- **R6 Tampilan.**
  - Halaman Saldo Cuti: kartu peringatan "Perlu ditinjau HR (N)" + daftar dengan aksi tandai selesai dan buka dialog edit; kolom sumber (Otomatis/Manual) di daftar.
  - Kartu setting: input batas + alasan wajib + tombol Pratinjau dampak sebelum Simpan.
  - ESS/tab karyawan: rincian jatah, sisa bawaan, penyesuaian, terpakai, sisa (sudah ada); pastikan sisa bawaan tersedia di-clamp ≥ 0.
- **R7 Migrasi tindak lanjut** `20261029090000_leave_carry_over_recalc.sql`:
  - buat tabel review (membuka kunci);
  - kolom source + backfill R1;
  - drop CHECK `leave_balances_carry_over_valid` bagian `carry_over_used ≤ carry_over_days`, CHECK lain tetap;
  - setting tetap **mati**.

## Review Focus
1. **Persetujuan cuti Desember tahun lalu ketika setting mati** → tidak ada perubahan bawaan dan tidak ada audit bawaan.
2. **Dua perubahan bersamaan** untuk karyawan yang sama (approve tahun lalu + PUT tahun berjalan) → kunci baris N−1 lalu N, urutan tetap, supaya tidak ada deadlock atau nilai basi.
3. **Bawaan manual HR** tidak tertimpa oleh hitung ulang apa pun.
4. **Pratinjau batas = hasil simpan,** termasuk jumlah "perlu ditinjau".
5. **Review tidak dobel** untuk karyawan dan tahun yang sama, dan tertutup otomatis bila kelebihan hilang.

---

### Task 1: Migrasi + fungsi inti `recomputeCarryOver` + hook cuti/manual/import/job
**Files:**
- Create: `backend/database/migrations/deltas/20261029090000_leave_carry_over_recalc.sql`
- Create: `backend/internal/modules/hris/leave_carry_recalc.go`
- Modify: `postgres_leaves.go` (`consumeAnnualLeave`, `CancelLeave`), `leaves.go` (keputusan tolak bila relevan), `leave_balances.go` (`UpdateLeaveBalanceManual`, R1 source, view clamp `carry_over_remaining ≥ 0`), `leave_balance_import.go` (commit: R1 + hook), `leave_carry_over.go` (job R3)
- Test: `backend/internal/modules/hris/leave_carry_recalc_integration_test.go`

**Interfaces:** `func recomputeCarryOver(ctx context.Context, q database.Querier, employeeID string, year int, cause carryCause, actor *auth.User) (changed bool, err error)`; `type carryCause struct{ Type string; LeaveID string; Label string }` dengan Type `leave_approve|leave_cancel|leave_reject|hr_update|import|settings|job`.

- [ ] **Test dulu** (wajib dari user):
  - cuti Desember disetujui setelah 1 Januari → bawaan N turun;
  - dibatalkan → naik;
  - ditolak → tetap (no-op terdokumentasi);
  - HR mengoreksi alokasi N−1 manual → bawaan ikut;
  - import massal N−1 → ikut;
  - koreksi membuat bawaan < terpakai → tersimpan, tidak negatif di tampilan, review terbuka, lalu tertutup otomatis setelah koreksi balik;
  - job dua kali (klaim dihapus di antara) → hasil sama;
  - setting mati → tidak ada bawaan;
  - karyawan nonaktif → tidak dibuatkan baris tahun baru;
  - bawaan manual tidak tertimpa (Review 3);
  - audit memuat penyebab dan aktor.
- [ ] Implementasi. Migrasi diterapkan ke DB test, `migrate -verify` 0. Commit.

### Task 2: Setting batas — alasan wajib, pratinjau dampak, hitung ulang tahun berjalan
**Files:**
- Modify: `leave_settings.go`, `routes.go`
- Test: `leave_settings_integration_test.go`

**Interfaces:**
- `POST /api/hris/leave-settings/preview` (update) body = patch PUT → `{data:{employees_changed, days_increase, days_decrease, needs_review}}`
- PUT kini mewajibkan `reason`.

- [ ] **Test dulu:**
  - tanpa batas → 6 hari memotong bawaan;
  - pratinjau = hasil nyata;
  - audit setting dengan alasan + audit per karyawan;
  - batas dikosongkan → kembali penuh;
  - batas membuat bawaan < terpakai → review;
  - tanpa izin → 403;
  - tanpa alasan → 400;
  - tahun sebelumnya tidak berubah.
- [ ] Implementasi. Manifest. Commit.

### Task 3: Review API + frontend (kartu peringatan, daftar, setting pratinjau + alasan, kolom sumber)
**Files:**
- Backend: review list/resolve di `leave_carry_recalc.go` atau `leave_carry_reviews.go`; routes
- Frontend: `features/hris/leave-balances/{api,queries,mutations,types}.ts`, `components/leave-carry-reviews-card.tsx` (+test), `leave-settings-card.tsx` (alasan + pratinjau), `leave-balances-page.tsx` (kolom sumber, kartu review)
- Test: Go integration review list/resolve (403, catatan wajib, audit); vitest kartu, setting, halaman

- [ ] Test dulu, implementasi, `tsc`/`eslint`/`vitest`. Commit.

### Task 4: Dokumen + cek akhir
- [ ] EPIC-068 Automation Log, bagian "Tindak lanjut" ditandai selesai di kode; `go test -p 1 ./...`, vitest penuh, `tsc`, `eslint`, `next build`, `pnpm check:safari`. Commit.

# Database Cleanup per 20 Oktober 2026 (DM-01) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Hapus permanen data transaksi HRIS bertanggal ≤ 20 Oktober 2026 (absensi, pengajuan absensi, cuti/izin, lembur, perjalanan dinas, pinjaman lunas/ditolak/pending), dengan backup dulu, tanpa notifikasi, dan dengan saldo cuti tetap.

**Architecture:**
- Satu skrip SQL inti tanpa kontrol transaksi, dijalankan dengan `psql -1` (satu transaksi). Skrip inti juga dieksekusi oleh tes integrasi Go di dalam transaksi tes.
- Skrip pratinjau baca-saja terpisah menghasilkan angka per tabel dan daftar file lampiran.
- Skrip shell menangani backup (dump DB dan tar file), eksekusi, dan penghapusan file di host.

**Tech Stack:** PostgreSQL (psql), Go integration test (harness hris), bash.

**Spec:** `docs/epics/EPIC-083-cleanup-data-20-oktober.md` (R1–R8). Sumber: MOM 29 Okt 2026 (DM-01, K-22).

## Global Constraints
- Batas: `'2026-10-20'::date` inklusif. Satu konstanta di skrip (`WITH cutoff AS (SELECT DATE '2026-10-20' AS d)` atau setara).
- Kolom tanggal:
  - `attendance.date`, `attendance_requests.date`, `overtime_requests.date`;
  - `leaves.end_date`, `business_trips.end_date`;
  - `loans.requested_date`.
- Pinjaman yang dihapus hanya `status IN ('rejected','cancelled','pending')` atau (`status='approved'` dan `coalesce(remaining_balance,0) <= 0`). Cek status lain yang ada (mis. `paid`) dan anggap lunas.
- Tidak menyentuh: `leave_balances`, `leave_carry_over_*`, payroll, `attendance_periods`, `audit.audit_log`, `public.notifications`, data master.
- Tanpa notifikasi: `ALTER TABLE hris.attendance DISABLE TRIGGER trg_notify_attendance_changes` dan `ALTER TABLE hris.leaves DISABLE TRIGGER trg_notify_leave_changes` di awal, lalu `ENABLE` di akhir skrip inti. Tambahan: `SET LOCAL hris.allow_closed_period_write = 'on'`. Dilarang `session_replication_role`.
- Satu baris audit: action `data_cleanup.dm01`, actor_name `Sistem (cleanup DM-01)`, entity `hris`, after = jumlah per tabel.
- Production: gladi di salinan prod, persetujuan tertulis HR atas angka gladi, dan izin user untuk jam eksekusi. Read prod dari mesin ini ditolak classifier, jadi gladi dan eksekusi dijalankan di VPS lewat skrip yang disetujui user.

## Review Focus
1. **Pengajuan absensi yang menunjuk absensi > 20 Okt** (koreksi diajukan untuk tanggal ≤ 20 tapi baris absensi terkait sesudahnya, atau sebaliknya): yang dihapus ditentukan oleh tanggal milik tabel itu sendiri. FK SET NULL tidak error. Diuji di Task 1.
2. **Cuti 18–22 Okt** (melintasi): tetap ada. Diuji di Task 1.
3. **Absensi di periode tertutup (September)**: terhapus tanpa error guard. Diuji di Task 1.
4. **Trigger notifikasi aktif kembali setelah skrip**: insert absensi sesudahnya tetap memicu notifikasi seperti biasa. Diuji di Task 1.
5. **File lampiran dipakai bersama** (path sama dirujuk baris yang tetap ada): tidak ikut dihapus. Diuji di Task 2.

---

### Task 1: Skrip SQL inti + pratinjau + tes integrasi

**Files:**
- Create: `backend/database/sql/fixes/2026-11-03_dm01_cleanup_core.sql`. Hanya DELETE + toggle trigger + audit, tanpa BEGIN/COMMIT. Urutan: `attendance_requests` → `attendance` → `leaves` → `overtime_requests` → `business_trips` → `loans`. Cek FK dari tabel lain ke `leaves`/`overtime_requests`/`loans` (mis. lampiran atau tabel approval) dan hapus anaknya dulu bila FK-nya `NO ACTION`.
- Create: `backend/database/sql/fixes/2026-11-03_dm01_preview.sql`. Diawali `BEGIN READ ONLY`, diakhiri `ROLLBACK`. Isinya:
  - jumlah per tabel yang akan dihapus dan yang tetap (dengan rincian status);
  - checksum `leave_balances` (`md5(string_agg(row::text, '' ORDER BY id))`);
  - daftar path file lampiran milik baris yang akan dihapus, termasuk foto absensi. Path yang juga dirujuk baris yang tetap ada dikecualikan. Satu path per baris dengan prefix `FILE\t`.
- Test: `backend/internal/modules/hris/dm01_cleanup_integration_test.go`

**Interfaces:**
- Produces: the two SQL files; the preview's `FILE\t<path>` lines (consumed by Task 2).

- [ ] **Step 1: Failing test** `TestDM01Cleanup`. Seed in the harness tx (use `h.employee`; attendance with `SET LOCAL hris.allow_closed_period_write='on'` if needed; mark an `attendance_periods` row for 2026-09 `closed`):
  - attendance 2026-09-15 (closed period), 2026-10-20, 2026-10-21;
  - attendance_request date 2026-10-19 pointing to the 2026-10-21 attendance row, and one dated 2026-10-22;
  - leaves 2026-10-01..03 (approved), 2026-10-18..22 (approved), 2026-10-25..25 (pending);
  - overtime 2026-10-20 and 2026-10-21;
  - loans requested 2026-10-01: approved remaining 0, approved remaining 500000, pending, rejected; and one approved requested 2026-10-25;
  - a `leave_balances` row with known values.

  Run the preview SQL (minus BEGIN/ROLLBACK lines — strip them in the test or split the file so the test loads only the SELECT body) and assert the counts: attendance 2, requests 1, leaves 1, overtime 1, loans 3.

  Exec the core SQL. Assert:
  - attendance only 2026-10-21 left;
  - the 2026-10-22 request is left;
  - leaves 18–22 and 25 are left;
  - overtime 21 is left;
  - loans "approved remaining 500000" and "requested 10-25" are left;
  - `leave_balances` row unchanged;
  - no new `public.notifications` rows;
  - one audit row `data_cleanup.dm01` whose `after` has the per-table counts;
  - triggers re-enabled: `pg_trigger.tgenabled = 'O'` for both notify triggers; a new attendance insert afterwards still produces a notification only when the trigger condition holds (assert the trigger is enabled via `pg_trigger`).
- [ ] **Step 2: Run, verify FAIL** (files missing).
- [ ] **Step 3: Write both SQL files** (Indonesian header: purpose, MOM DM-01/K-22, user answers 02-11, usage `psql -1 -v ON_ERROR_STOP=1 -f …`).
- [ ] **Step 4: Run** `go test -p 1 ./internal/modules/hris/ -run TestDM01Cleanup -count=3`. Expect PASS. Then run the hris package once.
- [ ] **Step 5: Commit** `ops(hris): skrip cleanup DM-01 per 20 Oktober 2026 + pratinjau (EPIC-083)`.

### Task 2: Skrip runner (backup → pratinjau → eksekusi → file)

**Files:**
- Create: `tools/ops/dm01-cleanup.sh`. Mode `preview` | `execute` | `files`; run on the DB host where the `paddydev-db` container and the `paddydev-storage` volume live.
  - `preview`: runs the preview SQL, writes the output and the `FILE` list to `~/backups/dm01-<stamp>/`.
  - `execute`:
    - refuses unless `DM01_APPROVED=yes` and a preview from the same day exists;
    - `pg_dump -Fc` to `~/backups/dm01-<stamp>/db.dump` (umask 077) and `pg_restore -l` sanity check;
    - restores the dump into a temp DB `paddydev_dm01_check`, compares row counts of the 6 tables, then drops it;
    - then `psql -1 -v ON_ERROR_STOP=1 -f core.sql`;
    - post-check: re-run the preview and expect 0 to delete, plus the same `leave_balances` checksum.
  - `files`:
    - tars the listed files from the storage volume (via `docker run --rm -v paddydev-storage:/s … tar`) to `~/backups/dm01-<stamp>/files.tgz`;
    - verifies the archive lists every path;
    - deletes the files, skipping paths that no longer exist, and prints counts.
- Test: `tools/ops/dm01-cleanup.test.sh` (bash, runs against the local test DB and a temp docker volume): seeds 2 files referenced by a to-delete row and 1 file referenced by a kept row; asserts `files` deletes exactly 2, the tar contains 2, and the kept file remains (Review Focus 5). Also asserts `execute` without `DM01_APPROVED=yes` exits non-zero.

- [ ] **Step 1: Failing test; Step 2: implement; Step 3: run** `bash tools/ops/dm01-cleanup.test.sh` → `PASS`.
- [ ] **Step 4: Commit** `ops(hris): runner cleanup DM-01 dengan backup dan cek restore (EPIC-083)`.

### Task 3: Gladi di staging + dokumen

- [ ] Run on staging (same host):
  - `preview`; record counts in the epic Automation Log.
  - `execute` with `DM01_APPROVED=yes` (staging approval = this plan).
  - `files`.
- [ ] Verify on staging:
  - payroll November run page opens;
  - ESS absensi of an employee shows only data from 21 Oct;
  - leave balances unchanged (checksum);
  - 0 new notifications.
- [ ] Docs:
  - `docs/hris/acuan-mom-20261019/status-requirement.md`: row DM-01 with evidence.
  - `pertanyaan-rekomendasi.md`: row K-22 with the 4 answers.
  - `docs/epics/README.md`: EPIC-083 row.
  - Epic Automation Log.
- [ ] Commit `docs: EPIC-083 gladi staging DM-01`; PR to `main`.

### Task 4: Production (berizin)

- [ ] On the prod VPS: `preview`, then send the counts to the user for **written HR approval** (K-22).
- [ ] With approval and an agreed time (quiet hours): `execute` and `files`. Copy `~/backups/dm01-<stamp>/` to local `~/Desktop/paddydev-backups/`.
- [ ] Read-only post-checks: the 6 tables have 0 rows ≤ 20 Oct apart from the exceptions; `leave_balances` checksum is the same; the app is healthy (`/login` 200); no API errors in logs.
- [ ] Epic log + memory.

# Pemetaan Data Karyawan HR ke Production Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Data kerja karyawan production (penempatan, departemen, jabatan, atasan, status kontrak, join date, NIP) mengikuti file HR. Ketidaksesuaian yang tidak bisa dipastikan dilaporkan, bukan dipaksakan.

**Architecture:**
- Generator Python (stdlib saja) membaca file HR dan snapshot read-only production, lalu menghasilkan tiga keluaran:
  1. `01-jabatan-baru.sql`: jabatan baru;
  2. `02-data-kerja.xlsx`: template EPIC-086 terisi untuk penempatan, departemen, dan jabatan, dijalankan dengan `tools/ops/data-kerja.sh` (preview → apply, backup otomatis);
  3. `03-data-karyawan.sql`: atasan, status, join date, dan NIP dalam satu transaksi, dengan guard nilai lama dan audit `employee.update` per karyawan.
- Generator juga menulis `rencana.csv` (setiap perubahan dan setiap baris yang dilewati, beserta alasannya).
- Menjalankan ulang generator setelah eksekusi = verifikasi: perubahan dalam cakupan harus 0.

**Tech Stack:** Python 3 stdlib (pola `backend/database/sql/delta-staging-20261026/generate.py`), PostgreSQL, alat EPIC-086 (`backend/cmd/hris-data-kerja`, `tools/ops/data-kerja.sh`).

**Spec:** `docs/epics/EPIC-096-pemetaan-data-karyawan-hr.md` (R1–R7).

## Global Constraints

- Production hanya read-only sampai Task 5. Tanpa sesi atau akun uji.
- Eksekusi production hanya atas izin user di jam sepi (23.30–05.00 WIB), dengan backup lebih dulu.
- Data pribadi (file HR, snapshot, SQL hasil generate) **tidak** di-commit. Tempatnya `~/Desktop/paddydev-backups/data-karyawan-hr/<stamp>/` (umask 077). Yang di-commit hanya kode generator, tes, dan README.
- Aktor audit "Ops (Claude Code atas persetujuan user)", alasan "EPIC-096: data kerja mengikuti file HR 09-11".
- Karyawan nonaktif tidak diubah (R4).
- Commit diakhiri `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`; badan PR diakhiri `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.

## Review Focus

1. **Snapshot basi.** Data production bisa berubah antara gladi dan eksekusi. Setiap UPDATE di `03` memakai guard `WHERE id = … AND kolom IS NOT DISTINCT FROM <nilai lama>`, dan jumlah baris yang berubah dicocokkan dengan rencana. Selisih → RAISE EXCEPTION, seluruh transaksi batal. → Task 2, tes `test_sql_guards_old_value`.
2. **Tukar NIP berantai** (A pindah ke NIP yang kini milik B, dan B juga berubah) akan menabrak unique index `employees_nip_key`. Perubahan NIP dikerjakan dua langkah: semua NIP target diberi prefix sementara, lalu nilai akhir. NIP target yang dipakai karyawan di luar rencana dilewati (R5). → Task 2, tes `test_nip_swap_two_phase` dan `test_nip_taken_by_other_skipped`.
3. **Roster harian sesudah Berlaku Mulai.** Alat EPIC-086 menolak penempatan baru bila roster staf Shift sudah terisi melewati tanggal itu. Berlaku Mulai = max(tanggal eksekusi, hari roster terakhir + 1) per karyawan, dan yang tertunda dilaporkan. → Task 1, tes `test_from_after_roster`.
4. **Pengajuan pending pindah penyetuju.** Atasan baru otomatis menjadi penyetuju pengajuan yang masih pending (`request_approver` dinamis). → Task 4 menghitung jumlahnya dan Task 6 melaporkannya ke HR.
5. **Karyawan muncul dua kali di file** dengan isi berbeda (mis. Kayla Olivia Berliana) → dilewati dan dilaporkan, bukan diambil salah satu. → Task 1, tes `test_duplicate_rows_conflict`.

---

### Task 1: Generator — pencocokan dan rencana perubahan

**Files:**
- Create: `backend/database/sql/data-karyawan-hr/generate.py`
- Create: `backend/database/sql/data-karyawan-hr/test_generate.py`
- Create: `backend/database/sql/data-karyawan-hr/snapshot.sql` (COPY read-only: karyawan, cabang, jabatan, departemen, toko pengatur, roster terakhir per karyawan)
- Create: `backend/database/sql/data-karyawan-hr/README.md`

**Interfaces:**
- Produces: `plan(hr_rows, snap, today) -> Plan`, dengan `Plan.changes: list[Change(employee_id, nip, name, field, old, new)]`, `Plan.skipped: list[Skip(sheet, row, nik, name, reason)]`, `Plan.new_positions: list[(title, level, department)]`, `Plan.placement_from: dict[employee_id, date]`. `field` ∈ {`branch`, `department`, `position`, `reporting_to`, `employment_status`, `join_date`, `nip`}.
- CLI: `generate.py --hr <xlsx> --snapshot <dir> --today YYYY-MM-DD --out <dir>`.

- [ ] **Step 1: Tes gagal untuk aturan pencocokan dan pemetaan**

Di `test_generate.py` (unittest, fixture dibuat di kode):
- `test_match_by_nip_then_name`: NIK `201602001` (angka) cocok dengan NIP `201602001`; NIK `2025-05-268` cocok dengan NIP `202505268` atau `2025-05-268`; nama dipakai bila NIP tidak cocok; nama ganda → Skip "nama ganda".
- `test_inactive_match_prefers_active_by_name`: NIP cocok ke record nonaktif dan ada record aktif bernama sama → pakai yang aktif.
- `test_inactive_untouched`: hanya ada record nonaktif → Skip "nonaktif di HRIS", tanpa Change.
- `test_branch_mapping`: `Headquarter` → `MIZU-BANDUNG`, `Warehouse` → `ODOO-WAREHOUSE-PADDY`, `PPGA` → `ODOO-PPGA`, `TWEM` → `TWEM`, kode tak dikenal → Skip.
- `test_position_strip_store_code`: `PPMR Shopkeeper Staff` → `Shopkeeper Staff`; `Penjahit PBSA` → `Penjahit`; `KOL Specialist SPV` → `KOL Specialist Supervisor` (alias); judul tak dikenal → `new_positions` berisi `(judul file, level file, departemen file)`.
- `test_status_mapping`: PKWT → contract, PKWTT → permanent, Probation → probation, Freelance → freelance, Daily Worker → daily_worker, Internship → intern.
- `test_approver_alias_and_region`: `Zaidane` → Zaidane Gusma; `Ricky / Azka` + toko PPGA → Ricky Pratama Sidik; + PPBA → Azka Hanifah Hamidah; + Headquarter → Skip "approver tidak pasti".
- `test_duplicate_rows_conflict`: satu karyawan di dua baris dengan nilai berbeda → Skip untuk keduanya; dua baris identik → satu set Change.
- `test_from_after_roster`: roster terakhir 2026-11-15, today 2026-11-10 → `placement_from` = 2026-11-16, dan dilaporkan.
- `test_no_change_when_equal`: nilai file = nilai HRIS → tanpa Change.

- [ ] **Step 2: Jalankan, pastikan gagal**

Run: `python3 -m unittest backend/database/sql/data-karyawan-hr/test_generate.py -v`
Expected: FAIL/ERROR karena `generate` belum ada.

- [ ] **Step 3: Implementasi `plan()` dan pembaca xlsx stdlib (`zipfile` + `xml.etree`)**

Pembaca xlsx menangani shared strings, angka (NIK sebagai float → string digit), dan tanggal serial Excel. Aturan pemetaan diambil dari tabel di EPIC-096 (R3 dan alias). Tabel alias jabatan ada di kode, dihimpun dari laporan pencocokan 09-11.

- [ ] **Step 4: Jalankan tes**

Expected: semua lulus.

- [ ] **Step 5: Commit**

### Task 2: Generator — keluaran SQL dan template EPIC-086

**Files:**
- Modify: `backend/database/sql/data-karyawan-hr/generate.py`, `test_generate.py`

**Interfaces:**
- Consumes: `Plan` dari Task 1.
- Produces:
  - `01-jabatan-baru.sql`;
  - `02-data-kerja.xlsx`: template dari `data-kerja.sh template` yang diisi kolom NIP, Penempatan Utama, Berlaku Mulai, Departemen Baru, dan Jabatan Baru;
  - `03-data-karyawan.sql`;
  - `rencana.csv`;
  - `ringkasan.txt` (jumlah per field, Skip per alasan).

- [ ] **Step 1: Tes gagal**

- `test_positions_sql_idempotent`: `01` memakai `INSERT … SELECT … WHERE NOT EXISTS (… lower(title) = lower($t))` dan diakhiri cek jumlah jabatan target ada = N, dengan RAISE EXCEPTION bila kurang.
- `test_sql_guards_old_value`: setiap UPDATE di `03` memuat `IS NOT DISTINCT FROM` nilai lama, lalu `GET DIAGNOSTICS` dan RAISE EXCEPTION bila baris ≠ 1.
- `test_nip_swap_two_phase`: A: `X`→`Y`, B: `Y`→`Z` menghasilkan fase 1 (`'tmp096-' || id`) untuk keduanya, lalu fase 2 nilai akhir.
- `test_nip_taken_by_other_skipped`: NIP target dipakai karyawan di luar rencana → Skip "NIP dipakai <nama>", tanpa UPDATE.
- `test_audit_one_row_per_employee`: satu INSERT `audit.audit_log` `employee.update` per karyawan, `before`/`after` hanya berisi field yang berubah.
- `test_template_fill`: baris template diisi untuk karyawan dengan Change branch/department/position; kolom lain tetap kosong; karyawan tanpa perubahan tidak diisi.

- [ ] **Step 2: Jalankan, pastikan gagal** → **Step 3: Implementasi** → **Step 4: Jalankan, lulus**

- [ ] **Step 5: Commit**

### Task 3: Gladi di salinan DB production

**Files:** tidak ada di repo. Artefak di `~/Desktop/paddydev-backups/data-karyawan-hr/gladi-<stamp>/`.

- [ ] **Step 1: Snapshot dan dump production (read-only)**

Run: `snapshot.sql` lewat psql `default_transaction_read_only = on`, plus `pg_dump -Fc`. Restore dump ke `paddy_gladi_e096` (klien pg16).

- [ ] **Step 2: Generate dengan `--today` = tanggal eksekusi yang direncanakan**

Expected: `ringkasan.txt` mendekati laporan 09-11, yaitu atasan ±81, join date ±68, status ±66, jabatan ±36 (+ jabatan baru), penempatan ±25, departemen ±6, NIP ±16, ditambah daftar Skip.

- [ ] **Step 3: Terapkan ke DB gladi berurutan**

Urutan: `01` (psql `-1 -v ON_ERROR_STOP=1`) → `hris-data-kerja -mode preview` lalu `-mode apply -sha <sha>` (`go run ./cmd/hris-data-kerja` dengan `DATABASE_URL` gladi) → `03`. Catat durasi tiap langkah.
Expected: semua rc=0; preview tanpa galat. Galat preview (mis. roster, periode closing) → perbaiki generator lewat tes RED → GREEN, ulangi.

- [ ] **Step 4: Verifikasi = generate ulang dari snapshot DB gladi**

Expected: `ringkasan.txt` baru menunjukkan 0 Change untuk semua field; Skip tetap sama. Hitung pengajuan pending yang penyetujunya berubah (Review Focus 4).

- [ ] **Step 5: Uji jalur gagal**

Ubah satu nilai lama di DB gladi, lalu jalankan `03` versi awal. Expected: RAISE EXCEPTION, tidak ada baris yang berubah. Setelah itu DB gladi dihapus.

### Task 4: PR + CI + merge (izin user)

- [ ] **Step 1:** Push branch `ops/data-karyawan-hr-20261109`, buka PR berisi kode generator, tes, README, epic, dan rencana. Tes Python dijalankan lokal; CI repo memeriksa sisanya.
- [ ] **Step 2:** CI hijau → merge atas izin user (persetujuan rencana ini).

### Task 5: Eksekusi production (izin user, jam sepi)

- [ ] **Step 1:** Snapshot ulang production read-only, generate dengan `--today` hari itu, lalu bandingkan `ringkasan.txt` dengan gladi. Selisih di luar aktivitas wajar (mis. jumlah perubahan naik > 5) → berhenti dan laporkan.
- [ ] **Step 2:** Backup `pg_dump` production → `~/backups/prod-db-before-e096-<stamp>.dump` (+ salinan lokal).
- [ ] **Step 3:** Jalankan `01` → `ENV=prod tools/ops/data-kerja.sh preview` lalu `apply` (sha) → `03`. Setiap langkah berhenti bila rc ≠ 0.
- [ ] **Step 4:** Verifikasi: snapshot + generate ulang → 0 Change. `/login` dan `/api/ready` 200, log API bersih. Jumlah karyawan aktif tidak berubah.
- [ ] **Step 5:** Cetak perintah rollback: restore tabel `hris.employees`, `hris.employee_placements`, dan `hris.positions` dari dump.

### Task 6: Laporan HR, dokumentasi, dan memori

- [ ] **Step 1:** `.share/data-karyawan-20261109/hasil-pemetaan.xlsx` berisi: perubahan yang diterapkan, Skip beserta alasan, nonaktif/baru/tidak di file (R4), level jabatan berbeda, dan pengajuan pending yang pindah penyetuju.
- [ ] **Step 2:** EPIC-096 Automation Log + status `done`; PR docs; memori `hris-epic096-pemetaan-data-karyawan`.

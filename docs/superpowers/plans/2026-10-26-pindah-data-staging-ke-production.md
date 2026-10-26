# Pindah Data Staging (25–26 Okt) ke Production — Rencana Implementasi

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Memasukkan data HRIS yang tercatat di staging paddydev setelah cutover (24-10-2026 23:30 WIB) sampai dump `staging-db-before-freeze-20261026-110218` ke production paddyadorable.biz.id, tanpa downtime dan tanpa menimpa data production yang lebih baru.

**Architecture:** Generator Python (`backend/database/sql/delta-staging-20261026/generate.py`) membandingkan dua DB test lokal per primary key + md5 baris:
- **baseline**: dump live saat cutover `paddydev-db-cutover-execute-20261024-233012.dump`;
- **staging**: dump `staging-db-before-freeze-20261026-110218.dump`.

Dari perbandingan itu generator menulis satu file SQL delta. Isinya data sebagai JSON + `json_populate_recordset` dalam satu transaksi, dengan:
- precondition yang membatalkan bila production sudah berubah;
- `session_replication_role = replica` saat menyalin, supaya trigger notifikasi tidak menggandakan notifikasi yang ikut disalin;
- pemeriksaan FK manual dan ringkasan di akhir.

File foto/lampiran yang dirujuk diambil dari arsip `staging-storage-before-freeze-20261026-110220.tgz`. File SQL dan arsip hasil generator berisi data pribadi (hash password, lokasi, alasan), jadi **tidak masuk repo**: disimpan chmod 600 di `~/Desktop/paddydev-backups/`.

**Tech Stack:** Python 3 (stdlib), psql/pg_restore Postgres 16, ssh `paddy-prod`, tar.

**Spec:** Keputusan user 26-10-2026 di sesi ini:
- sumber data = dump tetap `staging-db-before-freeze-20261026-110218`, **bukan** DB staging yang terus berubah;
- absen ganda digabung (jam masuk paling awal + pulang paling akhir);
- persetujuan atasan di staging ikut;
- password baru 2 akun ikut;
- notifikasi + audit ikut;
- sesi login, penanda baca modul, job latar, dan rate limit tidak dibawa.

## Global Constraints

- Sumber tetap: `~/Desktop/paddydev-backups/staging-db-before-freeze-20261026-110218.dump` dan `staging-storage-before-freeze-20261026-110220.tgz`. Baseline: `paddydev-db-cutover-execute-20261024-233012.dump`.
- Sebelum eksekusi: backup DB production (`pg_dump -Fc`) di VPS `~/backups/`, chmod 600.
- Eksekusi hanya atas izin eksplisit user.
- Satu transaksi. Semua precondition dan verifikasi `RAISE EXCEPTION`; gagal = tidak ada perubahan.
- File storage tidak boleh menimpa file yang sudah ada di production (`tar --skip-old-files`).
- Isi password/hash, isi file SQL delta, dan isi env tidak dicetak ke terminal atau log.
- Restore lokal memakai klien Postgres 16 (`docker run --network host postgres:16-alpine pg_restore`).

## Cakupan (hasil diff 26-10-2026)

| Tabel | Aksi | Jumlah | Catatan |
|---|---|---|---|
| `hris.attendance` | insert | 105 | id dan karyawan+tanggal belum ada di production |
| `hris.attendance` | gabung ke baris production | 2 | Shafira (202601530) dan Alif (EMP-2026-00008), 25-10. Prod tanpa jam pulang; staging masuk ≤ prod dan punya jam pulang, jadi hasilnya = nilai baris staging (id/created_at production dipertahankan) |
| `hris.attendance_requests` | insert | 1 | `dc791d8a…`. `22efc71d…` (Alif 24-10) dilewati karena sudah diajukan ulang di production |
| `hris.leaves` | insert | 2 | pending |
| `hris.leaves` | update keputusan | 1 | `2da5c375…` Connyetta, sakit 25–26 Jul: approved oleh Jusmiati, total hari 2→1 |
| `hris.overtime_requests` | insert | 4 | pending, dengan lampiran |
| `hris.overtime_requests` | update keputusan | 1 | `9fb5e194…` Ikrimah 23-10: approved oleh Jusmiati |
| `hris.loans` | insert | 1 | kasbon Rp1.500.000, pending |
| `auth.users` | password_hash | 2 | angelinafitriyani644@, pashamauludfilesmana@ |
| `public.notifications` | insert | 52 | 55 dikurangi 3 notifikasi milik pengajuan Alif yang dilewati |
| `audit.audit_log` | insert | 2 | `overtime.approve`, `leave.approve` |
| storage | salin | 207 file | foto absensi, lampiran lembur, lampiran pengajuan (tanpa lampiran pengajuan Alif) |

## Review Focus

1. **Production berubah antara pemeriksaan dan eksekusi** (karyawan absen 25-10 di prod, atasan memutuskan cuti/lembur yang sama, user mengganti password). Precondition: id insert belum ada; karyawan+tanggal absensi insert belum ada; baris update di prod masih sama md5-nya dengan baseline; baris absensi gabungan masih tanpa jam pulang. Diuji di Task 2, langkah jalur gagal.
2. **Trigger membuat notifikasi ganda** (`fn_notify_attendance_changes`: terlambat, < 7 jam). Penyalinan memakai `replica`; Task 2 memverifikasi jumlah notifikasi bertambah tepat 52.
3. **FK yatim karena replica** (shift/cabang/user tidak ada di prod). Verifikasi FK generik untuk semua baris yang disentuh. Diuji di Task 2.
4. **Periode absensi sudah di-closing.** Trigger guard tidak berjalan di replica, jadi precondition memeriksa `hris.attendance_periods` closed untuk tanggal yang disentuh.
5. **File lampiran hilang atau menimpa.** Daftar 207 file dicek ada di arsip; setelah ekstrak, cek setiap path ada di volume production.

---

### Task 1: Generator delta

**Files:**
- Create: `backend/database/sql/delta-staging-20261026/generate.py`
- Create: `backend/database/sql/delta-staging-20261026/README.md`

**Interfaces:**
- Produces: `python3 -I generate.py --base mig_base --stg mig_stg --out <delta.sql> --files <files.txt>`. File SQL mencetak `NOTICE: delta staging OK: ...` di akhir. `files.txt` berisi path relatif terhadap volume storage (`private/...`).

- [ ] Step 1: Restore baseline dan staging ke DB test `mig_base` / `mig_stg` (pg16).
- [ ] Step 2: Tulis generator: diff per PK, keputusan di atas sebagai konstanta (id dilewati, nip absen ganda, akun password), lalu keluarkan SQL dan daftar file.
- [ ] Step 3: Jalankan; Expected: ringkasan jumlah sama dengan tabel Cakupan.

### Task 2: Gladi lokal di salinan production

- [ ] Step 1: Ambil dump production terbaru (read-only `pg_dump` via ssh), restore ke `prod_rehearsal` lokal.
- [ ] Step 2: Jalankan delta; Expected `delta staging OK` dan selisih jumlah per tabel sama dengan Cakupan.
- [ ] Step 3: Jalankan delta kedua kali; Expected: gagal precondition (id sudah ada), tanpa perubahan.
- [ ] Step 4: Ubah satu baris precondition (mis. isi `clock_out` absen Shafira di `prod_rehearsal`), jalankan di salinan baru; Expected: gagal.
- [ ] Step 5: `migrate -verify` 0 hilang / 0 berbeda.

### Task 3: Skrip eksekusi + gladi VPS

**Files:**
- Create: `tools/vps/apply-delta.sh` (`rehearse|execute <delta.sql> <files.tgz>`)

- [ ] `rehearse`: restore dump production ke `delta_rehearsal` di VPS, jalankan delta, lalu hapus DB itu.
- [ ] `execute`: backup production, jalankan delta ke DB production, ekstrak file dengan `--skip-old-files` + `chown 1001`, lalu cek setiap file ada.
- [ ] `bash -n` + shellcheck bersih. PR, CI hijau, merge.

### Task 4: Eksekusi (atas izin user) dan verifikasi

- [ ] Jalankan `execute`; cek ringkasan, jumlah per tabel, file, dan `/api/ready`.
- [ ] Laporkan ke user. HR perlu tahu bahwa 2 cuti, 4 lembur, 1 pengajuan absensi, dan 1 kasbon yang pending sekarang muncul di production.

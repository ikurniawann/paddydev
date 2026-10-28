# Work Location Toko (EPIC-069) — Rencana Implementasi

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `configuration.branches` di production dan penempatan utama 22 staf toko sesuai Excel HR "20261001 - Work Location - New".

**Architecture:** Satu skrip SQL ops dalam satu transaksi. Ada mode pratinjau read-only, pengaman `RAISE EXCEPTION` untuk setiap selisih, dan satu baris audit per perubahan. Tidak ada perubahan kode aplikasi dan tidak ada migrasi. Skrip dijalankan dengan `psql` di container DB production sesudah backup, mengikuti alur migrasi production.

**Tech Stack:** PostgreSQL 16, psql, VPS `paddy-prod` (container `paddydev-db`).

**Spec:** `docs/epics/EPIC-069-work-location-toko.md` (keputusan user 28-10-2026).

## Global Constraints

- Data production hanya diubah atas izin eksplisit user, di jam sepi (tidak ada absen berjalan; utamakan 23:30–05:00 WIB).
- Backup tepat sebelum perubahan: VPS `~/backups/prod-db-before-epic069-<STAMP>.dump`.
- Jangan membuat sesi atau akun sementara di production. Verifikasi memakai query read-only.
- Jangan mencetak password atau token.
- Skrip dijalankan dari `main` yang bersih (sesudah PR di-merge).

## Review Focus

- Data berubah antara gladi dan eksekusi (penempatan baru, toko ditambah lewat UI). Yang diharapkan: pengaman membatalkan seluruh skrip, dan pratinjau menampilkan selisihnya.
- Karyawan absen di toko baru sebelum skrip berjalan. Yang diharapkan: tidak terpengaruh, karena baris absensi menyimpan `branch_id` sendiri.
- Nama cabang berubah di dropdown, template Excel, dan brand (trigger). Yang diharapkan: kode tetap di depan nama, jadi pencarian dan urutan tidak berubah.
- Geofence aktif di 37 toko + Warehouse. Yang diharapkan: karyawan di luar radius mendapat popup keterangan dan absennya ditandai Invalid; absen tidak diblokir.

---

### Task 1: Skrip + gladi (SELESAI 28-10)

**Files:**
- Create: `backend/database/sql/fixes/2026-10-28_work_location_toko.sql`

- [x] Gladi di salinan production (`e069_prod`): pratinjau OK, terapkan OK (3 / 41 / 22), jalur gagal membatalkan.

### Task 2: PR

- [x] Commit skrip, epic, baris registry, dan rencana ini di branch `ops/epic069-work-location-toko`. Buka PR, tunggu CI hijau (4 cek), lalu merge.

### Task 3: Eksekusi production (perlu izin user)

- [x] Salin skrip dari `main` yang sudah di-merge ke VPS: `scp backend/database/sql/fixes/2026-10-28_work_location_toko.sql paddy-prod:~/epic069.sql`
- [x] Backup: `ssh paddy-prod 'docker exec paddydev-db sh -c "pg_dump -U \$POSTGRES_USER -d \$POSTGRES_DB -Fc" > ~/backups/prod-db-before-epic069-<STAMP>.dump'`. Pastikan ukurannya lebih dari 6 MB.
- [x] Pratinjau: `ssh paddy-prod 'docker exec -i paddydev-db sh -c "psql -U \$POSTGRES_USER -d \$POSTGRES_DB -v ON_ERROR_STOP=1 -v preview=1 -v expected_stores=44 -v expected_moves=22 -X -q" < ~/epic069.sql'`. Yang diharapkan: 44 baris toko, 22 pindah (penempatan sekarang = kolom "dari" untuk semuanya), tanpa error.
- [x] Terapkan: perintah yang sama tanpa `-v preview=1`. Yang diharapkan: `NOTICE` Toko baru 3, Toko dilengkapi 41, Penempatan dipindah 22.
- [x] Verifikasi read-only:
  - 66 baris audit;
  - TWSP/TWMS/TWEM aktif;
  - brand +3 dan gudang +3;
  - masing-masing dari 22 karyawan punya satu penempatan utama aktif di toko Excel;
  - HTTP 302 (ke login) di `/` dan 401 di `/api/hris/branches`;
  - tidak ada error API di `docker logs paddydev-api --since 10m`.
- Rollback bila perlu (tidak dipakai): `pg_restore --clean` tabel `configuration.branches`, `hris.employee_placements`, `item.brands`, dan `configuration.warehouses` dari backup, atau restore penuh.

### Task 4: Dokumen dan serah terima

- [x] Catat hasil di Automation Log EPIC-069 dan ubah status menjadi `ready-for-qa`.
- [x] Perbarui AB-01 di `docs/hris/acuan-mom-20261019/status-requirement.md`: koordinat toko live sejak 28-10, kecuali toko-toko yang tercantum.
- [x] Serahkan daftar "Tindak lanjut HR" dari epic ke HR.

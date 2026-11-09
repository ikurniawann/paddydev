# EPIC-083: Database Cleanup per 20 Oktober 2026 — DM-01 (`MODULE-HRIS`)

status: done
environment: dev
retries: 0

## Goal
MOM 29 Oktober 2026 (WIT × Paddy, "Adjustment HRIS PaddyDev (Lanjutan)"):
- §2 No. 6: "Database cleanup per tanggal 20 Oktober 2026 untuk data: pengajuan cuti & izin, lembur, transport, dan pinjaman."
- **DM-01**: "… didahului backup."
- **K-22**: cakupan, hapus permanen/arsip, definisi "transport", dan persetujuan tertulis sebelum eksekusi.

Permintaan user 02-11-2026: "cleanup dump data semua pengajuan, absensi dan lainnya terhitung dari tgl 20 (19, 18, 17, dst.) Oktober ke bawah karena semua sudah ada catatannya".

Jawaban user 02-11 (menjawab K-22):
- **Cakupan**: data bertanggal **≤ 20 Oktober 2026**, yaitu absensi, pengajuan absensi, cuti & izin, lembur, perjalanan dinas ("transport" = perjalanan dinas), dan pinjaman/kasbon.
- **Metode**: **hapus permanen**, didahului backup penuh database dan file lampiran.
- **Pinjaman yang masih berjalan** (disetujui, sisa > 0) **tidak dihapus**.
- **Saldo cuti tetap**: hanya riwayat pengajuan yang dihapus; `leave_balances` tidak diubah.

Runbook (perintah staging → gladi salinan production → production, kegagalan, pemulihan):
[`docs/hris/runbook-dm01-cleanup.md`](../hris/runbook-dm01-cleanup.md). Runner: `tools/ops/dm01-cleanup.sh`.

## Data di staging (salinan struktur production; angka production dilihat saat gladi)
| Data ≤ 20 Okt | Staging |
|---|---|
| `hris.attendance` (`date`) | 56.156 dari 56.972 |
| `hris.attendance_requests` | 11 |
| `hris.leaves` | 660 (approved 480, pending 152, rejected 28); 1 melintasi 20/21 Okt |
| `hris.overtime_requests` (`date`) | 2.694 |
| `hris.business_trips` | 0 |
| `hris.loans` (`requested_date`) | 33: lunas 14, berjalan 15, pending 3, ditolak 1 |

Trigger yang ikut terpicu saat DELETE:
- `trg_notify_attendance_changes` dan `trg_notify_leave_changes` mengirim notifikasi "Data … Dihapus" ke HR **per baris**.
- `attendance_closed_period_guard` menolak penghapusan absensi di periode yang sudah ditutup.

FK `attendance_requests.attendance_id` → `attendance` (SET NULL).

## Ruling
- **R1 Batas tanggal** (inklusif 20 Okt):
  - absensi `date`; pengajuan absensi `date`; lembur `date`;
  - cuti/izin dan perjalanan dinas: `end_date ≤ 20 Okt`. Pengajuan yang melintasi 21 Okt tetap ada (my call: sebagian harinya masuk periode berjalan).
  - pinjaman `requested_date`.
- **R2 Pinjaman**: dihapus bila status `pending`, `manager_approved` (masih menunggu HR), `rejected`, `cancelled`, `paid_off` (lunas lewat payroll), atau `approved` dengan `remaining_balance ≤ 0` (lunas). **Disetujui dan sisa > 0 tetap ada**; status lain dan `requested_date` kosong juga tetap (muncul di PENGECUALIAN).
- **R3 Saldo cuti**: `hris.leave_balances`, `leave_carry_over_*` dan setting tidak disentuh. Penghapusan langsung SQL (bukan lewat aplikasi), jadi hook penghitung ulang EPIC-068 tidak berjalan.
  - Efek samping yang diterima: kuota bulanan Izin Pribadi/Izin Leader dihitung dari baris `hris.leaves` (MonthlyLeaveUse, menurut `start_date` dalam bulan kalender). Setelah cuti/izin 1–20 Okt dihapus, kuota **Oktober** yang terpakai oleh pengajuan itu "terbebas": pengajuan baru bertanggal 21–31 Okt tidak lagi terhitung bersama pengajuan 1–20 Okt.
- **R4 Tanpa notifikasi**: kedua trigger notifikasi dinonaktifkan di dalam transaksi (`ALTER TABLE … DISABLE TRIGGER`), lalu diaktifkan lagi di transaksi yang sama. `SET LOCAL hris.allow_closed_period_write = 'on'` untuk periode tertutup. Tidak memakai `session_replication_role = replica`, supaya FK dan cascade tetap berjalan.
- **R5 Satu transaksi**: semua DELETE berjalan dalam satu transaksi; gagal → tidak ada yang terhapus. Satu baris audit `data_cleanup.dm01` (aktor "Sistem (cleanup DM-01)") berisi jumlah per tabel.
- **R6 File lampiran**: lampiran cuti, lembur, pinjaman, pengajuan absensi, dan foto absensi milik baris yang dihapus ikut dihapus dari volume storage. Sebelumnya file di-tar ke backup. My call: data pribadi ikut hilang bersama datanya.
  - Hanya nilai berbentuk `<folder kolom>/<employee_id baris>/<nama>` (FILE) yang dihapus otomatis, dan hanya bila tidak dirujuk baris yang tetap.
  - Nilai lain (FILE_LAIN: folder lain, karyawan lain, URL, path absolut) **hanya ditinjau manual**, tidak dihapus runner.
  - Folder/berkas symlink dilewati.
- **R7 Yang tidak disentuh**:
  - payroll run/detail dan periode absensi;
  - `audit.audit_log` (append-only);
  - notifikasi lama;
  - data master (karyawan, jadwal, saldo);
  - `hris.attendance_adjustments` (log koreksi absensi, sengaja tanpa FK): barisnya tetap ada dan menjadi yatim, karena absensi yang dirujuknya sudah dihapus. Diterima sebagai jejak audit.
- **R8 Production** hanya dieksekusi setelah:
  - gladi di salinan production (angka per tabel dan daftar file);
  - **persetujuan tertulis HR** (K-22, Mba Risky/Manager HR) atas angka gladi;
  - izin eksplisit user untuk jam eksekusi (jam sepi).
- **R9 Gerbang payroll/periode** (final review 02-11): `execute` menolak bila:
  - ada payroll run `draft` yang mencakup ≤ 20 Okt (tidak bisa di-ACK);
  - ada perusahaan dengan gaji aktif tanpa payroll run selain draft pada periode payroll yang beririsan dengan data yang dihapus;
  - ada baris `hris.attendance_periods` ≤ 20 Okt yang belum `closed`.
  
  Dua butir terakhir bisa dilewati dengan `DM01_PAYROLL_ACK=yes`, hanya dengan pernyataan tertulis HR. Peringatan dicetak dan dicatat (`payroll-ack.txt`). Ketiganya juga tampil di pratinjau.
- **R10 Angka yang disetujui di dalam transaksi**:
  - runner meneruskan angka HAPUS pratinjau yang disetujui ke core (`psql -v expect_<tabel>` → `dm01.expect_<tabel>`);
  - core `RAISE` sebelum COMMIT bila jumlah terhapus berbeda, atau bila angkanya tidak ada (kecuali `dm01.expect_skip = 'on'`, khusus tes);
  - pratinjau yang disetujui wajib ditunjuk (`DM01_PREVIEW_DIR`), dicetak SHA-256-nya (opsional dicocokkan dengan `DM01_PREVIEW_SHA256`), dan berumur ≤ 36 jam;
  - sebelum database asli disentuh, core dijalankan dulu pada DB uji restore (gladi); durasinya menjadi perkiraan lama kunci.
- **R11 Pemulihan**: `tools/ops/dm01-cleanup.sh restore <folder eksekusi>` (`DM01_RESTORE_APPROVED=yes`). Cara kerjanya:
  - baris berkriteria cleanup dari `db.dump` dikembalikan dalam satu transaksi (`ON CONFLICT DO NOTHING`: id atau kunci unik yang bentrok dengan baris baru dilewati dan dicatat `LEWAT`; trigger dimatikan sementara; tautan pengajuan absensi dipulihkan);
  - berkas dari `files-*.tgz` dikembalikan tanpa menimpa, dengan pemilik 1001:1001;
  - baris yang dibuat sesudah cleanup tidak disentuh.

## Acceptance Criteria
- [ ] Pratinjau baca-saja menampilkan jumlah per tabel, rincian status, dan daftar file yang akan dihapus.
- [ ] Setelah eksekusi:
  - 0 baris ≤ 20 Okt di semua tabel cakupan, kecuali pinjaman berjalan dan cuti yang melintasi 21 Okt;
  - data > 20 Okt utuh;
  - `leave_balances` identik dengan sebelum eksekusi (checksum);
  - 0 notifikasi baru.
- [ ] Backup DB (`pg_dump -Fc`) dan tar file lampiran tersimpan di server dan disalin ke lokal, dan uji restore ke DB sementara berhasil.
- [ ] Payroll run November dan slip lama tetap terbuka tanpa error.

## Automation Log
- 2026-11-02 — MOM 29 Okt dibaca (DM-01, K-22). Data staging diinventaris. Jawaban user dicatat. Rencana: `docs/superpowers/plans/2026-11-02-cleanup-data-20-oktober.md`.
- 2026-11-02 — Task 1 (SQL core + pratinjau, tes Go `TestDM01*`) dan Task 2 (runner `tools/ops/dm01-cleanup.sh`, tes `tools/ops/dm01-cleanup.test.sh` di DB uji lokal + volume sementara) selesai. Keputusan review:
  - pinjaman `manager_approved` dan `paid_off` ikut dihapus (R2);
  - FILE_LAIN hanya ditinjau manual (R6);
  - `attendance_adjustments` dibiarkan yatim (R7);
  - kuota bulanan Izin Pribadi/Leader Oktober terbebas (R3);
  - gerbang payroll/periode (R9);
  - angka disetujui dicek di dalam transaksi dan gladi core di DB uji restore (R10);
  - mode pemulihan (R11).
  
  Runbook: `docs/hris/runbook-dm01-cleanup.md`. Belum dijalankan di staging maupun production.
- 2026-11-03 — **Gladi staging** (paddydev.reddie.id, host WIT; dijalankan controller dengan runner + runbook).
  - Pratinjau, dihapus:
    - attendance_requests 11;
    - attendance 56.156;
    - leaves 659;
    - overtime_requests 2.694;
    - business_trips 0;
    - loans 18.
  - Pratinjau, tetap: leaves melintasi 21 Okt 1, loans berjalan 15. Berkas: FILE 38, FILE_LAIN 0.
  - Gerbang payroll/periode:
    - `PAYROLL_BELUM` 2025-08..2026-10 untuk kedua perusahaan;
    - `PERIODE_TERBUKA` 2026-10.
    
    Di staging dilewati dengan `DM01_PAYROLL_ACK=yes`: staging, persetujuan = rencana.
  - Execute pertama **berhenti aman di uji restore**, sebelum ada yang dihapus. Staging punya baris FK yatim, yaitu sisa demo QA077/QA078:
    - `leave_balances` 5;
    - `employee_shifts` 14;
    - `employee_placements` 1.
    
    Baris itu dibuat cleanup QA sebelumnya yang memakai `session_replication_role=replica`. Baris yatim dihapus, pratinjau diulang, lalu execute lolos: jumlah audit `data_cleanup.dm01` = angka pratinjau.
  - Cek pasca-eksekusi:
    - absensi ≤ 20 Okt = 0, absensi > 20 Okt = 816 utuh;
    - payroll run utuh;
    - pinjaman tersisa 17;
    - 0 notifikasi baru;
    - `/login` dan `/api/ready` 200;
    - 0 error API.
  - Berkas: 38 diarsipkan, 38 dihapus.
  - Backup lokal: `~/Desktop/paddydev-backups/dm01-staging/dm01-20261103-000403-eksekusi`.
  - Pelajaran: gladi production **wajib** juga dijalankan di salinan production yang di-restore (runbook §4), karena uji restore gagal pada setiap FK yatim. Masalah seperti ini harus ketemu sebelum jam eksekusi.
  - Perbaikan menyusul: restore mengembalikan pengajuan absensi yang absensi induknya dilewati (bentrok) tanpa tautan (`LEWAT_TAUT`). Runbook diperbarui: gladi salinan production dengan `pg_isready` dan `docker rm -fv`, folder gladi dihapus, persetujuan HR mengutip baris gerbang, payroll Oktober wajib final sebelum execute production.
  - Status → `ready-for-qa`. Production menunggu tiga hal: gladi di salinan production, persetujuan tertulis HR (K-22) atas pratinjau production + SHA-256, dan izin jam eksekusi.
- 2026-11-03 — **Cakupan diubah (keputusan user 03-11):**
  - Batas menjadi **≤ 30 September 2026** (semula 20 Oktober). Alasan: run payroll aktif di HRIS hanya November 2026 (periode 21 Okt–20 Nov); data sebelumnya tidak dipakai payroll HRIS mana pun.
  - Pinjaman **pending** ≤ 30 Sep ikut dihapus ("oke delete aja"). Pinjaman yang masih bersisa tetap.
  - Hitungan prod baca-saja 03-11:
    - absensi 52.927 (tertua 20-02-2026);
    - cuti/izin 560;
    - lembur 2.455;
    - pinjaman lunas 14 + pending 3 (10 bersisa tetap);
    - perjalanan dinas dan pengajuan absensi 0.
  - Skrip inti, pratinjau, dan runner diubah ke 2026-09-30 (gerbang draft `(2026, 9)`). Tes:
    - Go DM-01 7/7 PASS;
    - runner 96 cek PASS.
  - Berikutnya:
    1. merge;
    2. salin skrip ke `~/paddydev-src` prod;
    3. pratinjau prod;
    4. konfirmasi akhir user/HR + jam;
    5. execute (backup otomatis).
- 2026-11-03 14.40 WIB — **DIEKSEKUSI di production** (izin user: "oke delete aja" untuk cakupan ≤ 30 Sep termasuk pinjaman pending; "Run now" atas pertanyaan "HR konfirmasi payroll Feb–Sep sudah dicatat di luar HRIS → `DM01_PAYROLL_ACK=yes`").
  - PR #73 merged (`22ac894b`). Skrip disalin ke `~/dm01-src` di VPS; `~/paddydev-src` tidak disentuh.
  - Pratinjau prod 12.58: `~/backups/dm01-20261103-125822-pratinjau`, SHA-256 `c3311d99…086d5`.
  - Execute: `~/backups/dm01-20261103-143956-eksekusi`.
    - Backup `db.dump` (7,0 MB + sha256 + toc), uji restore lolos.
    - Terhapus: absensi 52.927, cuti/izin 560, lembur 2.455, pinjaman 17 (14 lunas + 3 pending), pengajuan absensi 0, perjalanan dinas 0.
    - Notifikasi "Dihapus": 0. Trigger notifikasi aktif kembali.
    - Langkah `files`: 0 berkas.
    - Audit `data_cleanup.dm01` 1 baris.
  - Verifikasi:
    - sisa ≤ 30 Sep hanya 10 pinjaman bersisa (sengaja tetap);
    - absensi ≥ 1 Okt 5.498;
    - `leave_balances` tidak disentuh cleanup (2 perubahan hari ini terjadi 11.49 dan 13.19, sebelum eksekusi; tidak ada perubahan sesudah 14.40);
    - `/login` dan `/api/ready` 200, 0 error API.
  - Pemulihan bila perlu: `DM01_RESTORE_APPROVED=yes bash ~/dm01-src/tools/ops/dm01-cleanup.sh restore ~/backups/dm01-20261103-143956-eksekusi` (runbook §9).

- 2026-11-06: **Eksekusi kedua direncanakan** (permintaan user: "clean data absensi, pinjaman, pengajuan lainnya untuk tanggal 20 oktober ke bawah karena sudah konfirmasi HR kalo dihitung payroll dari tanggal 21 Oktober", lalu diralat: "jangan semua tapi semua pengajuan dan absensi saja").
  - **Keputusan user:**
    - batas ≤ 20 Okt;
    - pinjaman tidak disentuh;
    - cuti yang melintasi 21 Okt (sakit 19–23 Okt) tetap utuh;
    - saldo cuti tetap;
    - ringkasan closing periode Oktober dinolkan seperti Jan–Sep;
    - eksekusi malam ini 23.30 WIB dengan konfirmasi akhir.
  - **Pratinjau production read-only 06-11 15.3x WIB.** Akan dihapus:
    - 3.229 absensi;
    - 13 pengajuan absensi (semua pending);
    - 94 cuti/izin (79 approved, 12 pending, 3 rejected);
    - 236 lembur (191 approved, 35 pending, 2 cancelled, 8 rejected);
    - 0 perjalanan dinas;
    - 34 berkas.
  - **Kondisi payroll dan periode.** Periode absensi Oktober = 1–20 Okt (tertutup); November = 21 Okt–21 Nov (terbuka). Run payroll draft November mulai 21 Okt, jadi gerbang DRAFT lolos. PAYROLL_BELUM periode Oktober di-ACK atas konfirmasi HR.
  - **Kode** (branch `fix/dm01-cleanup-20-okt`):
    - runner, SQL, dan tes dikembalikan ke versi 20 Okt yang teruji (`57aa4cf4`, membatalkan perubahan batas di `ec7c4f95`);
    - pinjaman dikeluarkan dari core dan pratinjau (R2');
    - script baru `2026-11-06_dm01_periode_oktober.sql`.
    - Tes Go DM-01 dan tes runner (96 cek) lulus.
  - **Gladi di salinan production pada VPS 06-11 15.49 WIB** (runbook §4, kode branch di `~/dm01b-src`): preview, lalu execute (`DM01_PAYROLL_ACK=yes`), lalu `2026-11-06_dm01_periode_oktober.sql`, lalu files, lalu restore. Semua rc=0.
    - Durasi: execute 19 dtk (termasuk backup dan uji restore), core 208 ms (perkiraan lama kunci tulis).
    - Sesudah execute: sisa ≤ 20 Okt 0/0/0/0 (absensi, cuti, lembur, pengajuan absensi), pinjaman 22 utuh, `close_summary` Oktober semua 0.
    - Uji restore: 3.229 / 94 / 236 / 13 kembali persis.
    - Container, volume, dump, dan folder gladi dibersihkan.
- 2026-11-06 23.31 WIB: **Eksekusi DM-01 tahap kedua di production** (izin user "Ya, eksekusi 23.30"; kode main `45305bdd` di `~/dm01-src`).
  - **Pratinjau** `dm01-20261106-233127-pratinjau`, SHA-256 `e26c01d5…08433`. Angkanya sama dengan pratinjau 15.3x: 13 / 3.229 / 94 / 236 / 0 / 0 (pengajuan absensi, absensi, cuti, lembur, perjalanan dinas, pinjaman), 34 berkas.
  - **Execute** (`DM01_PAYROLL_ACK=yes`) `dm01-20261106-233141-eksekusi`: terhapus 13 / 3.229 / 94 / 236 / 0 / 0, audit `data_cleanup.dm01`, cek pasca-eksekusi lolos.
  - **Periode Oktober**: `2026-11-06_dm01_periode_oktober.sql` dijalankan 23.32. `close_summary` semua 0, event closing 0, audit `data_cleanup.dm01_periods`.
  - **Berkas**: 34 diarsipkan (`files-20261106-233226.tgz`) lalu dihapus, 0 gagal.
  - **Verifikasi**: sisa ≤ 20 Okt 0 (absensi, pengajuan absensi, cuti, lembur); cuti sakit yang melintasi 21 Okt tetap 1; pinjaman tetap 22; absensi tersisa 3.027; `/login` dan `/api/ready` 200; container sehat; log API bersih.
  - **Backup**: `db.dump` (SHA-256 `9ee8343e…4ca06`) dan arsip berkas ada di VPS `~/backups/dm01-20261106-233141-eksekusi`, salinannya di lokal `~/Desktop/paddydev-backups/prod-dm01-20261106/`. Pemulihan: runbook §9 (`restore`).
  - Folder log gladi `~/dm01b-gladi-log-*` di VPS dihapus.

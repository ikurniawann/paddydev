# Delta staging → production (25–26 Okt 2026)

Setelah cutover 24-10-2026 23:30 WIB, sebagian karyawan tetap memakai paddydev.reddie.id (staging). `generate.py` mengambil data yang masuk ke staging sejak cutover sampai dump tetap `staging-db-before-freeze-20261026-110218` dan menulis satu file SQL untuk production. Rencana: `docs/superpowers/plans/2026-10-26-pindah-data-staging-ke-production.md`.

## Revisi 26-10-2026 siang (instruksi user)

- **Sumber:** dump staging **baru** yang diambil `prepare.sh` (±22.45). Delta dihitung ulang dari baseline cutover; dump 11.02 tidak dipakai lagi.
- **Yang dibawa:** absensi + foto, pengajuan absensi, cuti, lembur, kasbon, dan keputusan atasan (perubahan baris lama). Hanya untuk karyawan yang sudah ada saat cutover dan ada di production.
- **Dibuang sebagai data uji developer:** baris yang merujuk karyawan atau akun yang dibuat setelah cutover (`employee_id`, `requested_by`, `decided_by`, dan sebagainya). Akun, password, notifikasi, audit, master, dan struktur tidak dibawa; perubahannya hanya dicatat di laporan.
- **Bentrok tidak diterapkan otomatis, semuanya masuk laporan:**
  - absen ganda digabung hanya bila hasilnya = baris staging (production tanpa jam pulang, staging masuk ≤ production dan punya jam pulang); bentuk lain dilewati;
  - baris lama yang juga berubah di production dilewati;
  - pengajuan yang sudah diajukan ulang di production dilewati.

## Cara pakai

```bash
bash prepare.sh        # dump baru, delta, arsip file, gladi lokal + VPS, laporan delta-<STAMP>.md
# setelah laporan dibaca dan user memberi izin:
bash ../../../../tools/vps/apply-delta.sh execute ~/Desktop/paddydev-backups/delta-<STAMP>.sql ~/Desktop/paddydev-backups/delta-<STAMP>.files.tgz
```

File SQL, arsip, dan laporan berisi data pribadi. Semuanya chmod 600 di `~/Desktop/paddydev-backups/`. Jangan di-commit atau dicetak.

## Jaminan di dalam file SQL

Satu transaksi; gagal = tidak ada perubahan.
- **Precondition:** id baru belum ada; karyawan+tanggal absensi baru belum ada; baris gabungan masih tanpa jam pulang; baris lama yang diperbarui masih sama dengan saat cutover (md5); tidak muncul pengajuan ganda baru; periode absensi belum di-closing.
- **Penyalinan memakai `session_replication_role = replica`:** nilai turunan ikut dari staging dan tidak ada notifikasi baru.
- **Verifikasi:** jumlah baris bertambah tepat, dan tidak ada FK yatim di tabel yang disentuh.

## Uji (26-10-2026 siang)

- **Data sintetis:** absensi karyawan uji dan lembur yang diajukan karyawan uji dibuang; perubahan master cabang tidak dibawa; koreksi yang juga diubah di production dilewati; koreksi yang aman diterapkan; jumlah karyawan production tetap; dijalankan ulang tertolak.
- **`prepare.sh` penuh (dump 12.31):** lulus dalam 213 detik. Absensi +108, pengajuan +1, cuti +2/~1, lembur +4/~1, kasbon +1, 2 absen gabungan, 1 pengajuan dilewati (sudah diajukan ulang). `migrate -verify` 0/0. Gladi VPS OK.

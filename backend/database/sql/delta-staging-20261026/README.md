# Delta staging → production (25–26 Okt 2026)

Setelah cutover 24-10-2026 23:30 WIB, sebagian karyawan tetap memakai paddydev.reddie.id (staging). `generate.py` mengambil data yang masuk ke staging sejak cutover sampai dump tetap `staging-db-before-freeze-20261026-110218` dan menulis satu file SQL untuk production. Rencana: `docs/superpowers/plans/2026-10-26-pindah-data-staging-ke-production.md`.

## Cara pakai

```bash
B=~/Desktop/paddydev-backups
T='postgres://postgres@127.0.0.1:55499'
for db in mig_base mig_stg; do psql "$T/postgres" -Xq -c "DROP DATABASE IF EXISTS $db" -c "CREATE DATABASE $db"; done
docker run --rm -i --network host postgres:16-alpine pg_restore --no-owner --no-acl --exit-on-error -d "$T/mig_base" < $B/paddydev-db-cutover-execute-20261024-233012.dump
docker run --rm -i --network host postgres:16-alpine pg_restore --no-owner --no-acl --exit-on-error -d "$T/mig_stg" < $B/staging-db-before-freeze-20261026-110218.dump
python3 -I generate.py --db "$T/" --base mig_base --stg mig_stg --out $B/delta-staging-20261026.sql --files $B/delta-staging-20261026-files.txt
# arsip file dari arsip storage staging pada batas yang sama
W=$(mktemp -d); sed 's#^#./#' $B/delta-staging-20261026-files.txt > $W.list
tar -xzf $B/staging-storage-before-freeze-20261026-110220.tgz -C $W -T $W.list && tar -czf $B/delta-staging-20261026-files.tgz -C $W private
bash ../../../../tools/vps/apply-delta.sh rehearse $B/delta-staging-20261026.sql $B/delta-staging-20261026-files.tgz
```

`delta-staging-20261026.sql` dan arsip file berisi data pribadi (hash password, lokasi, alasan). Simpan chmod 600 di `~/Desktop/paddydev-backups/`. Jangan di-commit atau dicetak.

## Isi delta

Keputusan user 26-10-2026 ditulis sebagai konstanta di `generate.py`.

- **Insert:**
  - 105 absensi, 1 pengajuan absensi, 2 cuti, 4 lembur, 1 kasbon;
  - 52 notifikasi, 2 audit log;
  - pengajuan absensi Alif 24-10 beserta 3 notifikasinya dilewati, karena sudah diajukan ulang di production.
- **Gabung:** absen 25-10 Shafira (202601530) dan Alif (EMP-2026-00008) memakai jam masuk paling awal dan jam pulang paling akhir. Hasilnya sama dengan baris staging; id baris production dipertahankan.
- **Keputusan atasan:** cuti Connyetta dan lembur Ikrimah (approved oleh Jusmiati) diterapkan ke baris production yang masih pending.
- **Password:** 2 akun yang mengganti password di staging.
- **Storage:** 207 file dari arsip staging. File yang sudah ada tidak ditimpa (`cp -n`).

Jaminan di dalam file SQL (satu transaksi; gagal = tidak ada perubahan):
- **Precondition:** id dan karyawan+tanggal belum ada di production; baris absen gabungan masih tanpa jam pulang; baris cuti/lembur dan password di production masih sama dengan saat cutover (md5); periode absensi untuk tanggal itu belum di-closing.
- **Penyalinan memakai `session_replication_role = replica`:** trigger notifikasi tidak menggandakan notifikasi yang ikut disalin.
- **Verifikasi di akhir:** jumlah baris bertambah tepat, dan tidak ada FK yatim di tabel yang disentuh.

## Hasil gladi (26-10-2026)

- **Salinan production lokal:** `delta staging OK`. Absensi +105, pengajuan +1, cuti +2, lembur +4, kasbon +1, notifikasi +52, audit +2. Hasil gabungan: Shafira 08:04–16:15, Alif 12:30–21:03. Persetujuan dan password terpasang. `migrate -verify` 0 hilang / 0 berbeda.
- **Jalur gagal, semuanya tertolak tanpa perubahan:** dijalankan ulang; Shafira sudah absen pulang di production; lembur Ikrimah sudah diputuskan di production; password sudah diganti di production.
- **Gladi VPS (`apply-delta.sh rehearse`):** lulus dalam 100 detik. 1 file sudah ada di production (lampiran lembur Ikrimah) dengan md5 identik, jadi tidak ditimpa.

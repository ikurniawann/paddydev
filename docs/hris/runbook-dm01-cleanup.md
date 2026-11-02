# Runbook DM-01 — cleanup data HRIS ≤ 20 Oktober 2026 (EPIC-083)

Dasar: MOM 29 Oktober 2026 §2 No. 6, DM-01 dan K-22. Ruling ada di
[`docs/epics/EPIC-083-cleanup-data-20-oktober.md`](../epics/EPIC-083-cleanup-data-20-oktober.md).

Skrip yang dipakai:
- runner: `tools/ops/dm01-cleanup.sh` (mode `preview | execute | files | restore`);
- SQL: `backend/database/sql/fixes/2026-11-03_dm01_preview.sql` (baca-saja) dan
  `2026-11-03_dm01_cleanup_core.sql` (hapus permanen).

Runner dijalankan di **host tempat container DB dan volume storage berada**:
- **staging** (paddydev.reddie.id): host WIT ini, container `paddydev-db`, volume `paddydev-storage`;
- **production** (paddyadorable.biz.id): VPS `ssh paddy-prod`, nama container dan volume sama.

Urutan wajib:
1. gladi di staging;
2. gladi di salinan production;
3. pratinjau production;
4. **persetujuan tertulis HR** (K-22, Manager HR) atas angka pratinjau dan SHA-256-nya;
5. **izin user** untuk jam eksekusi (jam sepi);
6. `execute`, lalu `files`;
7. salin backup ke lokal;
8. cek manual;
9. catat di epic.

---

## 1. Kode ke host

Gunakan alur `git archive` yang sama dengan deploy. Bedanya, kode diekstrak ke folder **terpisah** `~/dm01-src`, supaya `~/paddydev-src` di VPS tetap utuh: Caddyfile production di-bind-mount dari sana dan `.deployed-commit` tidak berubah. Ambil dari `main` setelah PR EPIC-083 di-merge.

```bash
# di laptop/host WIT, dari checkout repo yang sudah 'git fetch' main
KOMIT=$(git rev-parse --short origin/main)
BERKAS="tools/ops/dm01-cleanup.sh backend/database/sql/fixes/2026-11-03_dm01_preview.sql \
  backend/database/sql/fixes/2026-11-03_dm01_cleanup_core.sql docs/hris/runbook-dm01-cleanup.md"

# staging (host ini)
rm -rf ~/dm01-src && mkdir ~/dm01-src && git archive --format=tar origin/main $BERKAS | tar -x -C ~/dm01-src
echo "$KOMIT" > ~/dm01-src/.komit

# production
git archive --format=tar origin/main $BERKAS |
  ssh paddy-prod "rm -rf ~/dm01-src && mkdir ~/dm01-src && tar -x -C ~/dm01-src && echo $KOMIT > ~/dm01-src/.komit"
```

Path skrip di kedua host: `~/dm01-src/tools/ops/dm01-cleanup.sh`. Skrip mencari SQL-nya sendiri di `~/dm01-src/backend/database/sql/fixes`.

## 2. Sesi kerja

Selalu pakai `tmux`, supaya putus SSH tidak menghentikan proses. Setiap keluaran disimpan ke log.

```bash
ssh paddy-prod            # staging: langsung di host ini
tmux new -s dm01          # sambung lagi: tmux attach -t dm01
mkdir -p ~/backups
R=~/dm01-src/tools/ops/dm01-cleanup.sh
log() { echo ~/backups/dm01-$1-$(TZ=Asia/Jakarta date +%Y%m%d-%H%M%S).log; }
df -h ~ /var/lib/docker   # cek disk dulu (lihat §8 "Ruang disk")
```

Pola setiap perintah: `… 2>&1 | tee "$(log <mode>)"; echo "rc=${PIPESTATUS[0]}"`. Lihat `rc=` di akhir: hanya `rc=0` yang berarti lolos.

## 3. Gladi di staging

```bash
bash $R preview 2>&1 | tee "$(log preview)"; echo "rc=${PIPESTATUS[0]}"
# staging: persetujuan = rencana EPIC-083; tetap pakai folder pratinjau yang baru dibuat
P=$(ls -1d ~/backups/dm01-*-pratinjau* | tail -1)
DM01_APPROVED=yes DM01_PREVIEW_DIR=$P bash $R execute 2>&1 | tee "$(log execute)"; echo "rc=${PIPESTATUS[0]}"
E=$(ls -1td ~/backups/dm01-*-eksekusi* | head -1)
DM01_APPROVED=yes DM01_EXEC_DIR=$E bash $R files 2>&1 | tee "$(log files)"; echo "rc=${PIPESTATUS[0]}"
```

Bila execute menolak karena gerbang payroll/periode, lihat §8. Di staging, `DM01_PAYROLL_ACK=yes` boleh dipakai setelah dicatat di epic.

## 4. Gladi di salinan production (R8)

Gladi memakai DB dan volume salinan di VPS. Production tidak disentuh.

```bash
ST=$(TZ=Asia/Jakarta date +%Y%m%d-%H%M%S)
IMG=$(docker inspect -f '{{.Config.Image}}' paddydev-db)
docker exec paddydev-db sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > ~/backups/dm01-gladi-$ST.dump
docker run -d --name dm01-gladi-db -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=gladi "$IMG"
sleep 5
docker exec -i dm01-gladi-db pg_restore -U postgres -d gladi --no-owner --no-acl < ~/backups/dm01-gladi-$ST.dump
docker volume create dm01-gladi-storage
docker run --rm --network none -v paddydev-storage:/dari:ro -v dm01-gladi-storage:/ke "$IMG" sh -c 'cp -a /dari/. /ke/'

G="env DM01_DB_CONTAINER=dm01-gladi-db DM01_DB_NAME=gladi DM01_DB_USER=postgres DM01_VOLUME=dm01-gladi-storage DM01_BACKUP_DIR=$HOME/backups/dm01-gladi"
$G bash $R preview 2>&1 | tee "$(log gladi-preview)"; echo "rc=${PIPESTATUS[0]}"
P=$(ls -1d ~/backups/dm01-gladi/dm01-*-pratinjau* | tail -1)
$G DM01_APPROVED=yes DM01_PREVIEW_DIR=$P bash $R execute 2>&1 | tee "$(log gladi-execute)"; echo "rc=${PIPESTATUS[0]}"
E=$(ls -1td ~/backups/dm01-gladi/dm01-*-eksekusi* | head -1)
$G DM01_APPROVED=yes DM01_EXEC_DIR=$E bash $R files 2>&1 | tee "$(log gladi-files)"; echo "rc=${PIPESTATUS[0]}"
$G DM01_RESTORE_APPROVED=yes bash $R restore $E 2>&1 | tee "$(log gladi-restore)"; echo "rc=${PIPESTATUS[0]}"   # uji pemulihan

# bersihkan
docker rm -f dm01-gladi-db && docker volume rm dm01-gladi-storage && rm -f ~/backups/dm01-gladi-$ST.dump
```

Dari gladi, catat:
- angka per tabel;
- jumlah FILE / FILE_LAIN / berkas yang dihapus;
- durasi "Gladi core lolos dalam … ms", sebagai perkiraan lama kunci tulis absensi/cuti saat eksekusi production.

## 5. Pratinjau production → persetujuan HR

```bash
bash $R preview 2>&1 | tee "$(log preview)"; echo "rc=${PIPESTATUS[0]}"
```

Contoh keluaran (ilustrasi: angka tabel dari inventaris staging 02-11, angka berkas dan waktu rekaan):

```
• Target: host VM-…, container paddydev-db, database paddy (user …), volume paddydev-storage, backup /home/wit/backups
Akan dihapus (≤ 20 Okt 2026):
  attendance_requests    11
  attendance             56156
  leaves                 659
  overtime_requests      2694
  business_trips         0
  loans                  18
Tetap ada (seluruh tabel):
  …
Pengecualian (≤ 20 Okt tetapi TIDAK dihapus):
  leaves                 melintasi 21 Okt: 1
  loans                  berjalan (approved, sisa > 0): 15
Berkas: 51234 FILE dihapus otomatis, 3 FILE_LAIN ditinjau manual
Inventaris: 1 FK merujuk enam tabel, 12 trigger
PERINGATAN: periode absensi ≤ 20 Okt yang belum ditutup (execute menolak kecuali DM01_PAYROLL_ACK=yes):
  2026-10 open (2026-09-21..2026-10-20)
• Pratinjau: /home/wit/backups/dm01-20261103-221500-pratinjau/pratinjau.txt
• SHA-256 pratinjau: 3f1c…
```

Kirim ke HR (Manager HR):
- angka "Akan dihapus" dan pengecualian;
- jumlah berkas;
- daftar peringatan payroll/periode, bila ada;
- **SHA-256 pratinjau**.

Minta persetujuan tertulis (K-22) yang menyebut SHA-256 itu.

Angka ini harus sama dengan gladi di salinan production, kecuali ada data baru di antaranya. Peringatan payroll/periode harus diselesaikan dulu atau disetujui tertulis oleh HR (lihat §8).

## 6. Eksekusi production

Syarat sebelum eksekusi:
- persetujuan tertulis HR atas SHA-256 tersebut;
- izin user untuk jam eksekusi (jam sepi);
- pratinjau berumur **≤ 36 jam**. Bila lebih tua, ulangi §5.

```bash
P=~/backups/dm01-<stamp>-pratinjau                 # folder yang disetujui
SHA=<SHA-256 yang disetujui HR>
DM01_APPROVED=yes DM01_PREVIEW_DIR=$P DM01_PREVIEW_SHA256=$SHA bash $R execute 2>&1 | tee "$(log execute)"; echo "rc=${PIPESTATUS[0]}"
# hanya bila HR menyetujui tertulis gerbang payroll/periode: tambahkan DM01_PAYROLL_ACK=yes
```

Contoh keluaran (dipendekkan):

```
• Umur pratinjau: 2 jam 10 menit (batas 36 jam)
• Gerbang payroll/periode: bersih
• Folder backup: kosong 41230 MiB, perlu ≥ 3120 MiB (ukuran DB + 2 GiB)
• Volume data Postgres (sebelum dump): kosong 41230 MiB, perlu ≥ 3330 MiB (1,2 × DB + 2 GiB)
• Folder eksekusi: /home/wit/backups/dm01-20261104-000500-eksekusi
• pg_dump -Fc → …/db.dump
• Uji restore ke DB sementara paddydev_dm01_check
• Uji restore cocok
• Gladi core lolos dalam 8400 ms (perkiraan lama kunci tulis absensi/cuti di database asli)
Angka yang disetujui HR (…/pratinjau.txt):
  attendance             56156  …
• Menjalankan 2026-11-03_dm01_cleanup_core.sql (psql -1)
Terhapus (audit data_cleanup.dm01):
  attendance             56156  …
• Eksekusi selesai dan cek pasca-eksekusi lolos (notifikasi Dihapus: 0). Backup: …/db.dump
Langkah berikutnya: DM01_APPROVED=yes DM01_EXEC_DIR=… files
```

Lalu hapus berkas:

```bash
E=~/backups/dm01-<stamp>-eksekusi
DM01_APPROVED=yes DM01_EXEC_DIR=$E bash $R files 2>&1 | tee "$(log files)"; echo "rc=${PIPESTATUS[0]}"
```

```
• Mengarsipkan 51200 berkas → …/files-20261104-001200.tgz
• Isi arsip cocok dengan daftar (51200 path)
Ringkasan berkas:
  daftar FILE         : 51234
  diarsipkan (tar)    : 51200
  dihapus             : 51200
  sudah tidak ada     : 34
  dilewati (symlink)  : 0
  gagal               : 0
```

Cek kesehatan aplikasi: `curl -fsS https://paddyadorable.biz.id/api/ready` dan `/login` 200.

## 7. Salin backup ke lokal

```bash
# di host WIT
mkdir -p ~/Desktop/paddydev-backups
scp -r paddy-prod:~/backups/dm01-<stamp>-* ~/Desktop/paddydev-backups/
scp paddy-prod:~/backups/dm01-*-<stamp>*.log ~/Desktop/paddydev-backups/
chmod -R go-rwx ~/Desktop/paddydev-backups/dm01-*
(cd ~/Desktop/paddydev-backups/dm01-<stamp>-eksekusi && sha256sum -c db.dump.sha256 && sha256sum -c files-*.tgz.sha256)
```

`db.dump` dan `files-*.tgz` berisi data pribadi: jangan dibagikan, simpan mode 600/700.

## 8. Tabel kegagalan

| Pesan / gejala | Apa yang terjadi | Tindakan |
|---|---|---|
| `DM01_PREVIEW_DIR wajib`, `berumur … jam (> 36 jam)`, `SHA-256 … beda` | Belum ada yang berubah | Pakai folder pratinjau yang disetujui. Bila lebih dari 36 jam atau SHA beda: ulangi preview dan minta persetujuan HR lagi. |
| `dibuat untuk target lain` | Folder dari host/DB/volume lain (mis. gladi) | Pakai pratinjau dari target ini. |
| `payroll run berstatus draft` | Ada run draft yang mencakup ≤ 20 Okt; ACK tidak berlaku | Selesaikan (completed) atau hapus run draft itu lewat aplikasi, lalu ulangi execute. |
| `payroll belum final: <bulan> <perusahaan>` | Perusahaan bergaji aktif tanpa run final pada periode ≤ 20 Okt | Finalkan payroll periode itu. Bila payroll bulan itu memang dibuat di luar sistem (sebelum go-live), mintalah pernyataan tertulis HR, lalu jalankan dengan `DM01_PAYROLL_ACK=yes`. Peringatan dicatat di `payroll-ack.txt`. |
| `periode absensi belum ditutup: <bulan>` | Periode ≤ 20 Okt masih open | Tutup periode di menu periode absensi, atau ACK seperti di atas. |
| `data atau struktur berubah sejak pratinjau` (diff) | Drift: baris ≤ 20 Okt, status, berkas, FK, atau trigger berubah | Ulangi preview, kirim angka baru, minta persetujuan HR lagi. |
| `ruang kosong folder backup … kurang` / `volume data Postgres … kurang` | Disk tidak cukup; pada pengecekan "sesudah dump", dump sudah ada tetapi data belum disentuh | Kosongkan disk (backup lama yang sudah disalin ke lokal, `docker image prune`), lalu ulangi execute. |
| `pg_dump gagal`, `pg_restore -l gagal`, `restore … gagal`, `hasil restore berbeda` | Tidak ada yang dihapus | Periksa `restore.log`, disk, dan versi Postgres. Ulangi. |
| `gladi core SQL gagal di DB sementara` | Database asli TIDAK disentuh | Baca `core-gladi.log`: angka beda (data berubah), trigger tidak aktif, atau lock. Ulangi preview bila perlu. |
| `core SQL gagal (psql rc …)` | Transaksi `-1` dibatalkan: **tidak ada yang terhapus** | Baca `core.log`. Penyebab umum: `lock_timeout` (ulangi di jam lebih sepi), angka beda (`jumlah terhapus beda dari angka yang disetujui` → ulangi preview + persetujuan), atau trigger notifikasi tidak aktif. Pastikan dengan `preview`: HAPUS sama seperti sebelumnya. |
| `data SUDAH dihapus (COMMIT) tetapi cek pasca-eksekusi tidak lolos` | Data sudah terhapus, `execute.ok` tidak dibuat, `files` terkunci | Jangan ulangi execute. Periksa di folder eksekusi (daftar di bawah tabel). Setelah dipastikan aman: `touch $E/execute.ok`, lalu `files`. Bila salah hapus: §9 restore. |
| `files`: `masih dirujuk baris di database` | Ada baris yang kembali merujuk berkas | Jangan hapus. Selidiki baris itu dulu. |
| `files`: `dilewati (symlink)` > 0 | Folder atau berkas berupa symlink | Tinjau manual baris LEWATI di `files-*-status.txt` / `-hasil.txt`. |
| `files`: `gagal` > 0 | Sebagian berkas tidak terhapus | Perbaiki izin, lalu jalankan `files` lagi. Arsip baru dibuat. Hanya berkas yang masih ada yang diarsipkan. |
| `restore`: `… baris dilewati karena bentrok` | Baris lama tidak dikembalikan karena sudah ada baris dengan id/kunci unik yang sama (dibuat sesudah cleanup) | Tinjau daftar `LEWAT` bersama HR. Baris yang ada dipertahankan; bila baris lama yang benar, perbaiki manual. |
| FILE_LAIN di pratinjau | Lampiran di luar folder milik barisnya | Tidak dihapus otomatis. Tinjau manual bersama HR. |

Yang diperiksa bila cek pasca-eksekusi tidak lolos:
- `pratinjau-sesudah.txt`: HAPUS harus 0 dan tidak ada FILE;
- `audit-hapus.txt` dibanding `pratinjau-sebelum.txt`: jumlahnya harus sama;
- `notifikasi.txt`: harus 0;
- CHECKSUM `leave_balances` di `pratinjau-sebelum.txt` dibanding `pratinjau-sesudah.txt`.

Checksum `leave_balances` bisa positif palsu. Cleanup tidak mengubah `leave_balances` (R3), tetapi aktivitas yang sah di antara pratinjau-sebelum dan pratinjau-sesudah ikut mengubahnya. Contohnya karyawan mengajukan cuti, HR menyetujui cuti, atau job saldo cuti berjalan. Rentang waktunya beberapa menit: dari sebelum dump sampai sesudah core. Untuk memastikan:
- cari perubahan di rentang itu: `SELECT … FROM audit.audit_log WHERE created_at BETWEEN <mulai> AND <selesai>`;
- cari baris `leave_balances` dengan `updated_at` di rentang itu.

Bila perubahan berasal dari aktivitas yang sah, aman untuk membuat `execute.ok`.

## 9. Pemulihan (restore)

Pakai bila HR/user memutuskan data harus kembali (salah cakupan, kebutuhan audit):

```bash
E=~/backups/dm01-<stamp>-eksekusi
DM01_RESTORE_APPROVED=yes bash $R restore $E 2>&1 | tee "$(log restore)"; echo "rc=${PIPESTATUS[0]}"
```

Cara kerjanya:
- `db.dump` (dicek SHA-256-nya) di-restore ke DB sementara `paddydev_dm01_restore`. DB ini selalu dihapus di akhir.
- Baris yang cocok dengan kriteria cleanup disalin ke database asli dalam **satu transaksi**:
  - urutannya induk lebih dulu: absensi, lalu pengajuan absensi, cuti, lembur, perjalanan dinas, pinjaman;
  - `INSERT … ON CONFLICT DO NOTHING`: bila id sudah ada, atau baris bentrok dengan kunci unik baris baru sesudah cleanup (mis. absensi karyawan + tanggal yang sama), baris yang ada dipertahankan. Baris lama dicatat sebagai `LEWAT` di `restore-<stamp>-db.txt` dan dihitung di peringatan;
  - semua trigger aktif pada enam tabel dimatikan sementara, sehingga tidak ada notifikasi dan tidak ada hitung ulang;
  - tautan `attendance_requests.attendance_id` yang dikosongkan `ON DELETE SET NULL` dipulihkan.
- Baris yang dibuat atau diubah **sesudah** cleanup tidak disentuh.
- Berkas dari `files-*.tgz` dikembalikan hanya bila belum ada (tanpa menimpa), dengan pemilik `1001:1001` (uid aplikasi) dan mode 644.
- Satu baris audit `data_cleanup.dm01_restore` ditulis, lalu `restore.ok`.
- Restore idempoten: aman diulang bila terhenti. Setelah `restore.ok` ada, restore menolak dijalankan lagi.

```
Baris dikembalikan (dimasukkan / ada di dump; selisih = id yang sudah ada):
  attendance                   56156 / 56156
  …
  tautan_attendance_requests   3
Berkas: dikembalikan 51200, sudah ada (tidak ditimpa) 0, dilewati (symlink) 0
```

## 10. Cek manual sesudah eksekusi (staging dan production)

- [ ] Payroll → run **November 2026** terbuka tanpa error; slip lama (mis. September/Oktober) terbuka dan PDF terunduh.
- [ ] ESS seorang karyawan: riwayat absensi, cuti, dan lembur hanya menampilkan data mulai 21 Okt (kecuali cuti yang melintasi 21 Okt).
- [ ] Saldo cuti seorang karyawan sama dengan sebelum eksekusi (checksum di `pratinjau-sebelum.txt` = `pratinjau-sesudah.txt`).
- [ ] 0 notifikasi baru "Dihapus" (`notifikasi.txt`; lonceng notifikasi HR tidak bertambah).
- [ ] Pinjaman berjalan masih tampil dengan sisa yang sama.
- [ ] `https://paddyadorable.biz.id/api/ready` OK, `/login` 200, log API tanpa error baru.

## 11. Pencatatan

Catat di **Automation Log** epic EPIC-083:
- tanggal/jam WIB;
- operator;
- komit `~/dm01-src/.komit`;
- SHA-256 pratinjau yang disetujui, dan referensi persetujuan HR;
- angka per tabel dari `audit-hapus.txt`;
- pengecualian;
- berkas: dihapus / tidak ada / dilewati, dan jumlah FILE_LAIN;
- durasi gladi core dan core;
- path backup di VPS dan lokal;
- ACK payroll, bila ada;
- hasil cek manual §10.

Catat juga status DM-01 di `docs/hris/acuan-mom-20261019/status-requirement.md`.

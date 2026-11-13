# Rencana Rilis Production Pertama dari erp-paddy

**Status:** Fase 0 dijalankan 13-11 (lihat Log Fase 0); Fase 1 dan 2 belum. Setiap fase butuh izin eksplisit user.
**Dibuat:** 13-11-2026, atas permintaan user ("rencana rilis production").

## Tujuan

Membawa kode yang sudah ada di `wit-id-indonesia/erp-paddy` branch `development` ke production (`paddyadorable.biz.id`, VPS `paddy-prod`) dengan aman. Setelah itu operasi data yang terkait dijalankan satu per satu dengan izin terpisah.

## Apa yang ikut rilis

Ini rilis production **pertama** dari repo erp-paddy. Rilis terakhir (09-11, EPIC-087 sampai 097) masih dari repo paddydev. Kode tidak bisa dirilis per epic: satu deploy membawa seluruh isi `development`.

| Kelompok | Isi | Dampak ke data saat deploy |
|---|---|---|
| HRIS (kita) | EPIC-081?, 098, 099, 100, 102, 103, 104, 105, 107. 081 perlu dicek apakah sudah live sejak 03-11 | Migrasi: grant role Struktur Org (102), indeks unik label bagan (104), tabel `hris.account_branch_syncs` (107) |
| Struktur bisnis (EPIC-106) | Hanya kode pendukung (pemilih scope). **Skrip SQL struktur tidak jalan otomatis** | Tidak ada |
| Tim WIT (impor monorepo e815cee) | Master bahan baku dan produk, varian, price list, produksi bertahap, sales order, distribusi lintas perusahaan, laporan dan print | Sekitar 30 migrasi baru, termasuk **`20261109090000_rename_legacy_codes_to_erp`**: MIZU → ERP, MIZU-BANDUNG → ERP-BANDUNG, email onboarding `@arkiv.co.id` → `@paddy.id` |

Ada 32 migrasi yang ada di erp-paddy dan tidak ada di paddydev main (`20261108090000` sampai `20261206090000`). Jumlah yang belum jalan di production dipastikan di Fase 0.

## Fase 0 — Persiapan (production tidak diubah)

| # | Langkah | Hasil yang diharapkan | Siapa |
|---|---|---|---|
| 0.1 | **Cek baca-saja di production:** image yang jalan, migrasi terakhir, isi `~/paddydev-src` (layout lama `frontend/`/`backend/`), dari mana Caddyfile di-mount, sisa disk | Daftar migrasi yang akan jalan; tahu apakah `rsync --delete` ke `paddydev-src` aman untuk Caddy | Claude, **butuh izin SSH baca-saja** (13-11 ditolak pengaman otomatis) |
| 0.2 | **Cek data yang bisa menggagalkan migrasi:** label versi bagan ganda kalau huruf besar/kecil diabaikan (indeks 104), NIP ganda setelah trim (081), kode `ERP`/`ERP-BANDUNG` yang sudah dipakai (rename MIZU) | Semua 0 | Claude, baca saja |
| 0.3 | **Jalur deploy:** `deploy-vps.yml` (workflow_dispatch) butuh secret `VPS_SSH_KEY` dan `VPS_KNOWN_HOSTS`, dan per 13-11 secret itu belum terlihat. Pilihannya: isi secret, **atau** jalankan `tools/deploy-paddydev.sh` lewat SSH seperti rilis sebelumnya | Satu jalur dipilih user | User |
| 0.4 | **PR `development` → `production` di erp-paddy**, lalu tunggu CI hijau. Production dideploy dari branch `production` | PR hijau, belum di-merge | Claude |
| 0.5 | **Gladi di salinan DB production:** restore dump prod terbaru ke DB sementara, jalankan migrasi dengan image baru (`migrate -verify`), jalankan pratinjau skrip EPIC-106, lalu hapus DB sementara | Migrasi lulus, waktu migrasi tercatat, daftar BLOKIR EPIC-106 di data prod | Claude, **butuh izin** (pola `tools/vps/apply-delta.sh rehearse`) |
| 0.6 | **QA UI oleh HR di staging** dengan akun sendiri: impor karyawan/NIP (098/099/105), Kode & Status Toko + Pindah Toko Massal (100/107), bagan organisasi (102–104) | Sign-off HR | HR |
| 0.7 | **Sign-off tim WIT** untuk fitur produksi, sales order dan distribusi, karena ikut naik ke production | Sign-off | User / tim WIT |

**Keputusan terbuka sebelum Fase 1:**
- **EPIC-105 R3:** GRN ke gudang yang tokonya nonaktif akan ditolak. Di salinan prod 09-11 ada 13 dari 58 gudang aktif yang berada di toko nonaktif. Saat itu `purchasing.grn` masih kosong. Apakah ini diterima?
- **Jam rilis:** usulan Selasa/Rabu **22.00–23.00 WIB**, di luar jam 07–08, 13, 16–17 dan 21.

## Fase 1 — Rilis kode (izin user, jam sepi)

1. Cek tulis terakhir di production (tidak ada proses payroll atau impor yang sedang berjalan).
2. Backup DB production (`pg_dump -Fc` ke `~/backups/prod-db-before-erp-paddy-<stamp>.dump`) dan salin folder source lama (`~/paddydev-src.before-erp-paddy-<stamp>`).
3. Merge PR `development` → `production`, lalu deploy sesuai jalur 0.3. `deploy-paddydev.sh` membangun image, menjalankan migrasi sebelum container baru, cek kesehatan, dan otomatis kembali ke image lama kalau gagal.
4. **Smoke test:** `/`, `/login`, `/api/health`, `/api/ready` 200, dan 0 error API di log 10 menit pertama. Log job latar menunjukkan `hris pindah toko terjadwal` dan resign terjadwal berjalan tanpa error.
5. **Cek baca-saja setelah rilis:** kode `ERP` dan `ERP-BANDUNG` ada, `MIZU` tidak ada; jumlah karyawan, gaji aktif, dan payroll run sama dengan sebelum rilis; 49 kode `ODOO-` masih utuh (belum diubah).

**Rollback:**
- Kalau masalahnya hanya di aplikasi, jalankan image lama dan folder source lama.
- Kalau migrasi data harus dibatalkan (mis. rename MIZU → ERP), restore dari backup langkah 2. Tulisan yang masuk setelah backup akan hilang, jadi keputusan rollback diambil dalam 30 menit pertama.

## Fase 2 — Operasi data (setiap butir izin terpisah, setelah Fase 1 stabil minimal 1 hari)

| # | Operasi | Kapan | Catatan |
|---|---|---|---|
| 2a | **EPIC-106 struktur bisnis** (skrip `2026-11-12_struktur_bisnis_paddy_group.sql`: pratinjau, lalu eksekusi) | Setelah Fase 1 stabil. Gladi 13-11: gaji yang pindah 0, jadi payroll tidak menghalangi; draft November dihitung ulang sesudahnya (lihat Log Fase 0) | BLOKIR `payroll_belum_dibayar` menahan eksekusi kalau payroll belum final memuat karyawan yang gajinya pindah. Keputusan untuk orang yang terblokir (seperti Febri/Adrian di staging) diambil dari gladi 0.5 |
| 2b | **EPIC-107 hapus awalan ODOO-** (49 toko) | Setelah 2a | Dijalankan oleh HR di UI: Kode & Status Toko → "Hapus awalan ODOO-" → periksa → Simpan. Setelah itu file Data Kerja/Jadwal diunduh ulang |
| 2c | **EPIC-086 apply data kerja** | Saat file HR siap | Alur yang sama seperti sebelumnya: template → pratinjau → izin tertulis user + HR → apply |
| 2d | **QA HR di production** dengan akun asli | Setelah 2a dan 2b | Tanpa akun atau sesi sementara di production |

## Fase 3 — Pencatatan

- Automation Log setiap epic: waktu, image, backup, hasil smoke.
- Registry: epic → `done` setelah QA HR di production lulus.
- Memori proyek diperbarui; dokumen disinkron ke erp-paddy (`tools/sync-docs-erp-paddy.sh`).

## Log Fase 0 (13-11-2026, izin user "sesuai jalan kan rencana")

Production tidak diubah: hanya `docker ps`/`inspect`, `SELECT` dalam `BEGIN READ ONLY`, dan `pg_dump`. Gladi berjalan lokal.

- **0.1 Kondisi production:**
  - Image `local-20261109-221156` (paddydev main 4fcac55e), container sehat, disk 41% terpakai.
  - 587 migrasi terpasang. **38 migrasi** dari erp-paddy belum jalan, dan tidak ada migrasi production yang hilang dari erp-paddy.
  - Caddy me-mount `~/paddydev-src/tools/vps/Caddyfile`, dan isinya **identik** dengan erp-paddy, jadi `rsync --delete` aman untuk Caddy. Storage di volume `paddydev-storage`, tidak di folder source.
  - **EPIC-081 sudah live:** migrasi `20261105090000` sudah terpasang sejak rilis 03-11.
- **0.2 Data:**
  - NIP ganda setelah trim: 0. Tabel bagan belum ada (dibuat oleh migrasi). Kode `ERP-BANDUNG` belum dipakai; `MIZU-BANDUNG` ada (akan di-rename).
  - 49 kode `ODOO-`. Karyawan aktif 319, gaji aktif 266. `purchasing.grn` 0.
  - Payroll: **MIZU 2026-11 `draft`**.
- **0.3 Jalur deploy:** repo erp-paddy **tidak punya secret** (`VPS_SSH_KEY`, `VPS_KNOWN_HOSTS`), sehingga `deploy-vps.yml` belum bisa dipakai. Usulan: deploy lewat SSH seperti rilis 09-11 (rsync source erp-paddy ke `~/paddydev-src`, lalu `tools/deploy-paddydev.sh` di VPS), tanpa menyimpan kunci di GitHub.
- **0.4:** PR wit-id-indonesia/erp-paddy#3 (`development` → `production`) dibuka, **tidak di-merge**.
- **0.5 Gladi lokal** di salinan production `/mnt/hdd1/backup/paddydev/prod-copy-gladi-erp-paddy-20261113-083343.dump` (mode 600, berisi data pribadi). Salinan di-restore ke container uji lokal, dan DB gladi sudah dihapus setelah gladi.
  - Migrasi: **38 diterapkan dalam 10 detik**, exit 0. `migrate -verify`: 11.832 objek, 0 hilang, 0 berbeda.
  - Setelah migrasi: perusahaan `ERP` dan `PADDY-RETAIL`, `ERP-BANDUNG` ada, gaji aktif tetap 266.
  - **EPIC-106 pratinjau:**
    - 157 user dan 2 branch; gaji yang pindah **0**.
    - **BLOKIR 2** (orang yang sama dengan staging): outlet Febri Ayu Astiani (Pudak) dan Adrianus Adrian Chandra (Salam).
    - Simulasi keputusan R16 (a), outlet dipindah ke Gudang 1 HQ: **162 baris, 0 penghalang, semua INVARIAN OK**. Eksekusi lulus, jalan kedua 0 baris, dan pembalik mengembalikan 162 baris dari 1 snapshot.
    - PERHATIAN: payroll draft ERP November punya 37 baris alokasi ke Pudak/Salam (perlu dihitung ulang), dan 6 karyawan gajinya berada di perusahaan lain dari penempatannya.
  - Karena gaji yang pindah 0, BLOKIR `payroll_belum_dibayar` tidak muncul. Artinya 2a **tidak harus menunggu** payroll November dibayar, asal draft November dihitung ulang sesudahnya.
  - **Urutan wajib:** 2a sebelum 2b. Skrip EPIC-106 mencari `ODOO-PADDY-PUDAK`/`ODOO-PADDY-SALAM`. Kalau 2b lebih dulu, skrip butuh `struktur.cabang_pindah=PADDY-PUDAK,PADDY-SALAM`.

**Keputusan yang dibutuhkan sebelum Fase 1:**
1. Jalur deploy: SSH manual (usulan) atau mengisi secret GitHub.
2. Keputusan R16 untuk production: outlet Febri dan Adrian dipindah ke Gudang 1 HQ, sama seperti staging.
3. EPIC-105 R3: GRN ke gudang di toko nonaktif ditolak (GRN production masih 0).
4. Waktu 2a: segera setelah Fase 1 lalu draft November dihitung ulang (usulan), atau setelah payroll November dibayar.
5. Sign-off QA HR (0.6) dan tim WIT (0.7).

## Risiko utama

| Risiko | Pencegahan |
|---|---|
| `rsync --delete` ke `paddydev-src` menghapus Caddyfile atau file yang di-mount container production | Fase 0.1 memeriksa mount; source lama disalin dulu |
| Migrasi tim WIT gagal di data production | Gladi 0.5 di salinan prod; `migrate -verify` menghentikan deploy |
| Rename MIZU → ERP mengubah tampilan kode di laporan/akun yang masih memakai kode lama | Cek 0.2; komunikasikan ke HR/finance sebelum rilis |
| Fitur tim WIT belum siap untuk pengguna production | Sign-off 0.7; menu bisa disembunyikan lewat IAM kalau perlu |
| EPIC-106 mengubah gaji di tengah periode payroll | Jadwal 2a setelah payroll November `paid`; skrip punya BLOKIR dan skrip balik |

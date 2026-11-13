# Rencana Rilis Production Pertama dari erp-paddy

**Status:** rencana, belum ada yang dijalankan. Setiap fase butuh izin eksplisit user.
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
| 2a | **EPIC-106 struktur bisnis** (skrip `2026-11-12_struktur_bisnis_paddy_group.sql`: pratinjau, lalu eksekusi) | **Setelah payroll November dibayar** (status `paid`), sebelum draft Desember dibuat | BLOKIR `payroll_belum_dibayar` menahan eksekusi kalau payroll belum final memuat karyawan yang gajinya pindah. Keputusan untuk orang yang terblokir (seperti Febri/Adrian di staging) diambil dari gladi 0.5 |
| 2b | **EPIC-107 hapus awalan ODOO-** (49 toko) | Setelah 2a | Dijalankan oleh HR di UI: Kode & Status Toko → "Hapus awalan ODOO-" → periksa → Simpan. Setelah itu file Data Kerja/Jadwal diunduh ulang |
| 2c | **EPIC-086 apply data kerja** | Saat file HR siap | Alur yang sama seperti sebelumnya: template → pratinjau → izin tertulis user + HR → apply |
| 2d | **QA HR di production** dengan akun asli | Setelah 2a dan 2b | Tanpa akun atau sesi sementara di production |

## Fase 3 — Pencatatan

- Automation Log setiap epic: waktu, image, backup, hasil smoke.
- Registry: epic → `done` setelah QA HR di production lulus.
- Memori proyek diperbarui; dokumen disinkron ke erp-paddy (`tools/sync-docs-erp-paddy.sh`).

## Risiko utama

| Risiko | Pencegahan |
|---|---|
| `rsync --delete` ke `paddydev-src` menghapus Caddyfile atau file yang di-mount container production | Fase 0.1 memeriksa mount; source lama disalin dulu |
| Migrasi tim WIT gagal di data production | Gladi 0.5 di salinan prod; `migrate -verify` menghentikan deploy |
| Rename MIZU → ERP mengubah tampilan kode di laporan/akun yang masih memakai kode lama | Cek 0.2; komunikasikan ke HR/finance sebelum rilis |
| Fitur tim WIT belum siap untuk pengguna production | Sign-off 0.7; menu bisa disembunyikan lewat IAM kalau perlu |
| EPIC-106 mengubah gaji di tengah periode payroll | Jadwal 2a setelah payroll November `paid`; skrip punya BLOKIR dan skrip balik |

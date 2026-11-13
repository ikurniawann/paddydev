# Rencana Rilis Production Pertama dari erp-paddy

**Status:** Fase 0, gladi 0.8, dan perbaikan cookie (erp-paddy #4, c4c3c8a, gladi ulang lulus) selesai 13-11. Fase 1 menunggu izin user untuk jam rilis. Setiap fase butuh izin eksplisit user.
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

**Gerbang go/no-go (semua wajib ya, kalau tidak, rilis ditunda):**
- [ ] Gladi 0.8 lulus untuk **commit yang persis akan dirilis**. Kalau ada commit baru (mis. perbaikan cookie), gladi diulang.
- [ ] Keputusan soal logout massal sudah diambil: diperbaiki dulu, atau diterima dan HR mengumumkan "login ulang" sebelum jam kerja.
- [ ] Backup DB production dan folder source lama sudah dibuat dan bisa dibaca (`pg_restore -l` berhasil).
- [ ] Image build di VPS sukses. Build terjadi sebelum container diganti, jadi kalau gagal production tetap jalan.
- [ ] Ada izin eksplisit user untuk jam rilis tersebut.

1. Cek tulis terakhir di production (tidak ada proses payroll atau impor yang sedang berjalan).
2. Backup DB production (`pg_dump -Fc` ke `~/backups/prod-db-before-erp-paddy-<stamp>.dump`) dan salin folder source lama (`~/paddydev-src.before-erp-paddy-<stamp>`).
3. Merge PR `development` → `production`, lalu deploy sesuai jalur 0.3. `deploy-paddydev.sh` membangun image, menjalankan migrasi sebelum container baru, cek kesehatan, dan otomatis kembali ke image lama kalau gagal.
4. **Smoke test:** `/`, `/login`, `/api/health`, `/api/ready` 200, dan 0 error API di log 10 menit pertama. Crawler publik (tanpa login, tanpa akun sementara) dijalankan ke 67 halaman publik production: target 0 respons 5xx. Halaman dashboard dicek HR dengan akun asli: login, Data Karyawan, Absensi, Payroll, Kode & Status Toko. Log job latar menunjukkan `hris pindah toko terjadwal` dan resign terjadwal berjalan tanpa error.
5. **Cek baca-saja setelah rilis:** kode `ERP` dan `ERP-BANDUNG` ada, `MIZU` tidak ada; jumlah karyawan, gaji aktif, dan payroll run sama dengan sebelum rilis; 49 kode `ODOO-` masih utuh (belum diubah).

**Rollback:**
- Kalau masalahnya hanya di aplikasi, jalankan image lama dan folder source lama. `deploy-paddydev.sh` melakukannya otomatis bila health check gagal, dan migrasi tidak dibatalkan. Gladi 0.8 menguji image lama di atas skema baru (sesudah EPIC-106): hasilnya dicatat di bawah.
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

**Keputusan user (13-11-2026 ±11.30 WIB):**
1. Jalur deploy: **SSH** (rsync source erp-paddy ke `~/paddydev-src`, lalu `tools/deploy-paddydev.sh` di VPS). Secret GitHub tidak diisi.
2. R16 production: outlet Febri Ayu Astiani dan Adrianus Adrian Chandra dipindah ke **Gudang 1 HQ (kantor pusat)**, sama seperti staging.
3. EPIC-105 R3: **disetujui**. GRN ke gudang yang tokonya nonaktif ditolak.
4. Waktu 2a: **segera** setelah Fase 1 stabil, lalu draft payroll ERP November dihitung ulang.
5. Sign-off: **disetujui** untuk fitur HRIS dan fitur tim WIT (produksi, sales order, distribusi).

Syarat tambahan dari user: "rilis production harus QA dulu, jangan sampai tidak bisa diakses dan error atau terjadi bug". Ini dijawab dengan gladi rilis penuh 0.8 dan gerbang go/no-go di Fase 1.

## 0.8 Gladi rilis penuh (13-11-2026, lokal, production tidak diubah)

**Cara:**
- Image kandidat rilis adalah image staging `local-20261112-234046`, yang dibangun dari erp-paddy `8631e8f`, sama dengan head PR #3.
- Image itu dijalankan di atas salinan production 08.33 dalam jaringan Docker `--internal`. Container tidak punya akses internet (sudah dicek), sehingga tidak ada WA, email, atau push yang terkirim ke karyawan.
- Pembanding (baseline) memakai image production lama (paddydev `4fcac55e`, di-build lokal) di salinan yang belum dimigrasi.
- Crawler Chromium membuka setiap halaman, lalu mencatat status, error JavaScript, respons 5xx/4xx, dan teks error.

| Uji | Hasil |
|---|---|
| Migrasi seperti `deploy-paddydev.sh` (`migrate -apply`, `-verify` dari image) | 38 migrasi dalam **5 detik**; verify 11.832 objek, 0 selisih |
| Health (`/`, `/login`, `/api/health`, `/api/ready`, `api -healthcheck`) | Semua 200 |
| Login sungguhan (salinan akun HR, password diganti di DB gladi) | Login 200 dan cookie `erp_session` terpasang; `/api/auth/me` 200; password salah 401 |
| Crawl semua halaman: super admin 432, HR 430, karyawan 429, publik 67, ditambah 10 halaman detail HR dengan ID asli | Semua 200, **0 respons 5xx**. Satu halaman bermasalah: `/dashboard/crm/partners` (React #418 dan 403), tetapi **sudah sama di production sekarang** |
| Dibandingkan dengan baseline production lama (414–416 halaman yang sama) | **0 regresi** dan 0 respons 4xx baru, untuk super admin, HR, dan karyawan |
| EPIC-106 setelah R16 (skrip `r16-outlet-hq.sql` mencari Gudang 1 lewat kode cabang dan menolak bila orangnya bukan tepat 2) | Pratinjau 162 baris, 0 penghalang, INVARIAN OK. Eksekusi 162 baris; pratinjau ulang 0 |
| Crawl ulang dashboard setelah EPIC-106 (HR 363, super admin 364) | 0 masalah baru; log API tanpa error |
| 2b hapus `ODOO-` lewat API sebagai HR (template, pratinjau, commit) | Saran 49, pratinjau 49 kode, commit "49 toko diperbarui", sisa `ODOO-` 0 |
| **Rollback**: image production lama di atas skema baru setelah EPIC-106 dan 2b (crawl HR 363, super admin 363) | 0 regresi dibanding baseline dan log API tanpa error, jadi rollback aplikasi aman tanpa membatalkan migrasi. Catatan: setelah rollback, pengguna yang sudah login di versi baru harus login ulang (cookie `erp_session` tidak dikenal versi lama) |
| Env production (hanya nama variabel, dibaca lewat SSH) | Tidak ada env yang hilang. `MIZU_LEGACY_HOST_REDIRECTS` (dihapus di kode baru) tidak dipakai production. `XENDIT_MOCK` dan OTP demo tidak ada di production |

**Temuan yang perlu keputusan sebelum Fase 1:**
- **Semua pengguna ter-logout saat rilis.**
  - Nama cookie sesi berganti dari `nuhabit_session` (production lama) menjadi `erp_session`, dan kode baru tidak lagi membaca cookie lama. Kode lama punya cadangan untuk `arkiv_session`; cadangan itu hilang saat rename.
  - Di salinan production ada **584 sesi aktif milik 280 user**, dan absensi 7 hari terakhir 1.363. Setelah rilis semuanya harus login ulang, dan yang lupa password tidak bisa absen.
  - Usulan perbaikan: kode baru membaca `erp_session`, lalu `nuhabit_session`, lalu `arkiv_session`; logout menghapus ketiganya. Perubahannya kecil (Go `platform/auth`, `identity`; Next `lib/auth`, route logout) dan disertai tes. Jalurnya PR ke `development`, staging, lalu gladi ulang.
- **Keputusan user (13-11): "jangan sampe logout massal pakai cookies lama".**
  - Perbaikan ada di PR wit-id-indonesia/erp-paddy#4 (`fix/cookie-sesi-lama`, 374e149): Go dan Next membaca `erp_session`, lalu `nuhabit_session`, lalu `arkiv_session`. Logout menghapus sesi dari ketiga nama cookie. Cookie stall lama juga tetap dibaca.
  - Bukti: tes gagal lebih dulu, lalu lulus. Gerbang lokal: Go 140 paket ok, vitest 6.675 lulus, tsc dan eslint 0. CI GitHub hijau. Review independen tidak menemukan temuan material.
  - Minor yang ditunda: komentar `setSessionCookie` dan route active-stall masih menyebut cookie lama dihapus saat login; `signOut` di `pg/create-client.ts` hanya menghapus token pertama (fungsi itu tidak dipakai).
  - 13-11 ±13.35 WIB: user mengizinkan merge ("Ya, merge"). Squash ke `development` sebagai **c4c3c8a**. Staging ter-deploy otomatis dengan image `local-20261113-135008` dan sehat. PR #3 sekarang ber-head c4c3c8a dengan CI hijau.
  - **Gladi ulang (14.00 WIB)** di salinan production 08.33, dengan urutan seperti malam rilis:
    1. Versi production lama berjalan, dan sesi `nuhabit_session` milik super admin, HR, dan karyawan dibuat. `/api/auth/me` 200.
    2. Migrasi memakai image c4c3c8a: 38 migrasi dalam 1 detik, verify 0 selisih. Container diganti ke image baru.
    3. **Cookie yang sama tetap login:** `/api/auth/me` 200 dan dashboard 200 untuk ketiga peran.
    4. Logout dengan cookie lama: ketiga cookie di-expire, sesi di DB terhapus, lalu `/me` 401.
    5. Crawl penuh dengan cookie lama: super admin 432, HR 430, karyawan 429, publik 67 halaman. 0 halaman terlempar ke login, **0 regresi** dibanding baseline production lama, dan log API tanpa error. Satu-satunya temuan tetap CRM Partners, yang sudah ada di production sekarang.
  - Gerbang go/no-go baris 1 dan 2 terpenuhi untuk c4c3c8a.
- **Kunci penyimpanan browser POS berganti nama** (`arkiv-pos-db` menjadi `erp-pos-db`, begitu juga setelan printer). Production punya 0 order POS, jadi tidak ada antrean offline yang hilang; setelan printer struk hanya perlu diisi ulang bila nanti POS dipakai.

## Risiko utama

| Risiko | Pencegahan |
|---|---|
| `rsync --delete` ke `paddydev-src` menghapus Caddyfile atau file yang di-mount container production | Fase 0.1 memeriksa mount; source lama disalin dulu |
| Migrasi tim WIT gagal di data production | Gladi 0.5 di salinan prod; `migrate -verify` menghentikan deploy |
| Rename MIZU → ERP mengubah tampilan kode di laporan/akun yang masih memakai kode lama | Cek 0.2; komunikasikan ke HR/finance sebelum rilis |
| Fitur tim WIT belum siap untuk pengguna production | Sign-off 0.7; menu bisa disembunyikan lewat IAM kalau perlu |
| EPIC-106 mengubah gaji di tengah periode payroll | Jadwal 2a setelah payroll November `paid`; skrip punya BLOKIR dan skrip balik |

## Lampiran A: skrip R16 untuk production (dijalankan sesaat sebelum 2a, setelah backup)

Sudah diuji di gladi 0.8. Skrip ini gagal (ROLLBACK) bila Gudang 1 HQ tidak ketemu atau orangnya bukan tepat 2.

```sql
BEGIN;
DO $o$
DECLARE
  v_hq uuid;
  r record;
  n int := 0;
BEGIN
  SELECT w.id INTO STRICT v_hq FROM configuration.warehouses w JOIN configuration.branches b ON b.id = w.branch_id
   WHERE b.code IN ('ERP-BANDUNG','MIZU-BANDUNG') AND w.name = 'Gudang 1' AND w.is_active;
  FOR r IN SELECT u.id, u.full_name, u.default_warehouse_id,
                  (SELECT jsonb_agg(jsonb_build_object('warehouse_id', uw.warehouse_id, 'is_active', uw.is_active)) FROM configuration.user_warehouses uw WHERE uw.user_id = u.id) AS outlets
           FROM configuration.users u WHERE u.full_name IN ('Febri Ayu Astiani', 'Adrianus Adrian Chandra')
  LOOP
    INSERT INTO configuration.user_warehouses (user_id, warehouse_id, is_active)
    SELECT r.id, v_hq, true WHERE NOT EXISTS (SELECT 1 FROM configuration.user_warehouses WHERE user_id = r.id AND warehouse_id = v_hq);
    UPDATE configuration.user_warehouses SET is_active = true, updated_at = now() WHERE user_id = r.id AND warehouse_id = v_hq AND NOT is_active;
    UPDATE configuration.users SET default_warehouse_id = v_hq, updated_at = now() WHERE id = r.id;
    UPDATE configuration.user_warehouses uw SET is_active = false, updated_at = now()
    FROM configuration.warehouses w JOIN configuration.branches b ON b.id = w.branch_id
    WHERE uw.user_id = r.id AND w.id = uw.warehouse_id AND b.code IN ('ODOO-PADDY-PUDAK', 'ODOO-PADDY-SALAM') AND uw.is_active;
    INSERT INTO audit.audit_log (actor_id, actor_name, action, entity, entity_id, entity_label, before, after, reason)
    VALUES (NULL, 'Ops (Claude Code atas persetujuan user)', 'user.outlet_update', 'configuration.users', r.id::text, r.full_name,
            jsonb_build_object('default_warehouse_id', r.default_warehouse_id, 'outlets', r.outlets),
            jsonb_build_object('default_warehouse_id', v_hq),
            'EPIC-106 R16: outlet staf HQ dipindah dari toko Pudak/Salam ke Gudang 1 HQ (keputusan user 13-11-2026)');
    n := n + 1;
    RAISE NOTICE 'OUTLET % -> Gudang 1 HQ', r.full_name;
  END LOOP;
  IF n <> 2 THEN RAISE EXCEPTION 'R16: harus tepat 2 orang, ketemu %', n; END IF;
END
$o$;
COMMIT;
```

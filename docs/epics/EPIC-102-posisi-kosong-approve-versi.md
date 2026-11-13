# EPIC-102: Posisi Kosong & Persetujuan Versi Bagan — Fase 2a (`MODULE-HRIS`)

status: ready-for-qa
environment: dev
retries: 0

## Goal
Lanjutan EPIC-101. Pilihan user 10-11-2026: dari sisa fase 2, kerjakan dulu **dashboard posisi kosong (ORG-08)** dan **persetujuan versi bagan**.
- **Posisi Kosong:** HR melihat slot bagan yang belum ada pemegangnya, per divisi dan region, lalu mengunduhnya sebagai Excel.
- **Persetujuan versi:** HR menyetujui satu versi bagan sebagai versi resmi, dengan tanggal berlaku.

Masih ditunda (bergantung pada jawaban klien): importer Draft 7 (ORG-06, Q12) dan approver berbasis posisi (ORG-04, Q6).

## Ruling
- **R1 — Tidak ada data lain yang berubah.** Karyawan, jabatan, departemen, toko, `reporting_to`, dan alur approval tetap seperti EPIC-101 R1. Persetujuan versi hanya mengubah `hris.org_chart_versions`.
- **R2 — Keadaan setiap posisi dihitung, tidak disimpan.** Keadaan dihitung dari pencocokan R9 EPIC-101. Kolom `org_chart_nodes.status` tidak ditulis. Urutan penilaian:
  1. node grup → tidak dihitung;
  2. `frozen` → **Dibekukan**;
  3. punya pemegang → **Terisi**;
  4. `planned` → **Rencana**;
  5. ada karyawan ambigu yang salah satu kandidatnya node ini → **Mungkin terisi**;
  6. selain itu (`active` atau `vacant`) → **Kosong**.
- **R3 — Karyawan ambigu tidak membuat posisi tampak kosong.** Kode kandidat disimpan di hasil pencocokan (`candidate_codes`). Nama karyawannya ditampilkan di posisi yang **Mungkin terisi**, supaya HR tidak membuka lowongan untuk slot yang sebenarnya sudah ada orangnya.
- **R4 — Tanpa target headcount** sampai klien menjawab Q10. Satu node = satu slot. Node dengan lebih dari satu pemegang ditampilkan "Terisi N orang" tanpa dinilai kelebihan.
- **R5 — Satu versi disetujui pada satu waktu.**
  - Hanya versi `draft` yang bisa disetujui. Versi `approved` sebelumnya menjadi `archived` dalam transaksi yang sama.
  - Indeks unik parsial menjaga hanya ada satu `approved`. Persetujuan bersamaan menghasilkan 409.
  - Versi yang masih punya **error** validasi BR ditolak (400, daftar error). Peringatan tidak menghalangi.
- **R6 — Tanggal berlaku** (`valid_from`) wajib berupa tanggal, bawaannya hari ini (WIB). Alasan opsional dan ikut ke audit.
- **R7 — Versi bawaan** untuk pohon, selisih, dan posisi kosong adalah versi yang disetujui. Bila belum ada, dipakai impor terbaru (perilaku EPIC-101). Versi lain tetap bisa dipilih lewat `?version=`.
- **R8 — Akses.**
  - Membaca tetap memakai menu `hris.organization`.
  - Menyetujui butuh aksi `approve` pada menu itu (`withAction`). Tidak ada menu baru.
- **R8a — Hak "approve" (keputusan user 11-11: "Admin HR + Manager").**
  - Menu daun `hris.organization.org-chart` sebelumnya hanya menawarkan `read`. Migrasi `20261116090000` menambah `approve` ke menu itu dan ke grant yang sudah ada milik `super_admin`, `admin`, `hrd`, `admin_hris` (pola EPIC-067).
  - Migrasi `20261116090100` menambah masker Hak Akses HRIS untuk `hris_non_payroll` dan `hris_non_rekrutmen_kpi` pada daun itu. Masker itu mewarisi masker yang sedang berlaku (menu tersembunyi tetap tersembunyi) ditambah pengecualian `approve`.
  - Yang bisa menyetujui: super_admin, admin, hrd, admin_hris, hris_manager. Dua role staf HR hanya membaca. HR bisa mengubahnya per role lewat Hak Akses HRIS.
- **R9 — Audit.** `audit.audit_log` mendapat action `hris.org_chart.version.approve`, entity `hris.org_chart_versions`, dengan isi:
  - before: status lama;
  - after: status baru, `valid_from`, label versi yang diarsipkan.

## Tasks
Rencana: `docs/superpowers/plans/2026-11-10-posisi-kosong-approve-versi.md`.
- [x] T1 Domain: kandidat karyawan ambigu + keadaan posisi + ringkasan per divisi/region
- [x] T2 API Posisi Kosong (JSON + Excel)
- [x] T3 Migrasi + API persetujuan versi + versi bawaan
- [x] T4 Frontend: pemilih versi, dialog Setujui, tab Posisi Kosong
- [x] T5 Gate, QA staging, catatan epic

## Automation Log
- 2026-11-10: Epic dibuat dari sisa EPIC-101 R2. User memilih "Vacancy + approve versi". Rencana ditulis, menunggu review user.
- 2026-11-11: Implementasi selesai di branch `feat/epic102-vacancy-versi` (executing-plans, inline).
  - Commit: 519cb1a T1 domain, b11788b T2 API Posisi Kosong, 15c2893 T3 migrasi + persetujuan, 7888a7a T4 frontend, eed5eb8 perbaikan review.
  - Keputusan eksekusi:
    - hak `approve` (R8a);
    - grup posisi kosong membawa `division_name`;
    - pemilih versi memakai `<select>` biasa dengan badge status di sampingnya;
    - region kosong tampil "Tanpa region";
    - id versi bukan UUID → 404.
  - Review akhir seluruh branch (reviewer terpisah): 0 Critical, 2 Important, 8 Minor.
    - Sheet "Posisi Kosong" di Excel Selisih (arti berbeda) diganti "Tanpa Pemegang" + kolom Keadaan.
    - Ubin Dibekukan ditambahkan, pesan filter Keadaan tanpa hasil diperbaiki, toast 403 dalam bahasa Indonesia.
    - Ditunda (minor):
      - pesan 409 saat kalah balapan;
      - region "Tanpa region" belum bisa dipilih di filter;
      - belum ada tes konkuren dua transaksi;
      - tanggal bawaan memakai zona waktu perangkat;
      - sisa baris `_ = iam.HrisOrganization` di tes;
      - tombol Setujui tetap tampil untuk akun baca saja (server menolak 403).
  - Gate:
    - backend di DB baru (590 migrasi): gofmt dan vet bersih, `go test ./internal/...` 137 paket lulus, 0 gagal;
    - frontend: tsc OK, eslint OK, vitest 906 file / 6579 tes lulus.
    - Catatan: `TestRewardRedeemRateLimit` (memberportal) sempat gagal sekali karena timing di bawah beban paralel; tidak terkait (lulus saat diulang).
  - Staging 11-11 11.35 WIB: image `local-20261111-113530`, backup `/mnt/hdd1/backup/paddydev/staging-before-e102-20261111-113519.dump`, migrasi `20261116090000` diterapkan.
  - QA staging (salinan data prod 10-11):
    - Draft 6 → 149 posisi: 131 terisi, 9 kosong, 0 mungkin terisi, 9 rencana, 0 dibekukan; jumlahnya cocok.
    - 22 karyawan ambigu hanya cocok ke slot yang sudah punya pemegang (572 kandidat), jadi tidak ada yang "Mungkin terisi".
    - Makassar 4 posisi (RTL-054 Store Leader kosong).
    - Excel Posisi Kosong (Ringkasan, Posisi) dan Selisih (Tanpa Pemegang) terbuka.
    - Tanggal salah → 400. Setujui (akun hrd) → 200 + audit; setujui ulang → 409.
    - Akun baca saja: baca 200, setujui 403. Versi dengan error BR → 400.
    - Playwright desktop + HP 10/10 lulus, tanpa scroll horizontal; bukti di `.share/qa-epic102/`.
  - Setelah QA:
    - Draft 6 dikembalikan ke `draft`, supaya HR yang menyetujui di prod.
    - Role, user, sesi, dan versi QA sementara dihapus.
    - 1 baris audit QA tetap ada (audit append-only).
  - 11-11 ±11.50: user memilih "Admin HR + Manager" untuk hak setujui. Ditambahkan migrasi `20261116090100` dan tes `TestOrgChartApproveRoles`.
    - RED di salinan staging: kedua role staf memegang approve. GREEN setelah migrasi.
    - Skenario menu tersembunyi tetap tersembunyi; migrasi bisa dijalankan ulang.
    - Gate DB baru: 591 migrasi, 137 paket lulus.
    - Staging 11.53: backup `staging-before-e102b-20261111-115350.dump`, image `local-20261111-115358`. Grant Struktur Org: super_admin, admin, hrd, admin_hris, hris_manager memegang approve; hris_non_payroll dan hris_non_rekrutmen_kpi hanya read. Draft 6 tetap draft.
    - `TestIAMGuards` gagal di salinan prod karena `gym.packages` sudah dihapus di data prod. Juga gagal tanpa EPIC-102, jadi tidak terkait.
  - Prod: belum, perlu izin user.
- 2026-11-13: Registry dirapikan, status review → ready-for-qa. Kode sudah ada di erp-paddy `development` (impor monorepo e815cee) dan jalan di staging (image `local-20261112-234046`). Sisa: QA UI oleh HR, lalu rilis production dengan izin user.

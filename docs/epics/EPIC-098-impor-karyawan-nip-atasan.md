# EPIC-098: Impor Karyawan Excel Ditolak karena NIP Atasan (`MODULE-HRIS`)

status: ready-for-qa
environment: dev
retries: 0

## Goal
Permintaan user (10-11-2026):

> NIP manual epic baru tp ada bug ketika bulky lewat excel data tidak masuk karena NIP sama pada sudah di isi tapi tetep tidak masuk

Konteks: sejak EPIC-081, NIP diketik manual di file impor (Karyawan → Import Excel) dan tidak lagi dibuat otomatis.

**Reproduksi di staging** (10-11; data staging = salinan production per 10-11 05.55).

Jalur yang berfungsi:
- NIP baru diisi sebagai teks atau angka, mode Tambah: masuk.
- Mode Timpa untuk NIP yang sudah ada: diperbarui.
- NIP dengan tanda kutip di depan atau spasi tak terlihat: diterima.

Yang gagal: satu baris bermasalah memblokir **seluruh file** ("File masih punya error — perbaiki lalu unggah ulang").

| # | Isi file | Hasil sekarang |
|---|---|---|
| B1 | NIP Atasan = NIP karyawan **baru di file yang sama** (mis. SPV baru + stafnya) | "NIP atasan 2026xxxxx tidak ditemukan / tidak aktif" — seluruh file ditolak |
| B2 | NIP Atasan diketik dengan tanda kutip di depan (`'202611676`, pola Google Sheets/CSV) | "NIP atasan '202611676 tidak ditemukan / tidak aktif". Kolom NIP sendiri sudah dibersihkan, kolom NIP Atasan belum |
| B3 | Mode Tambah berisi NIP yang sudah ada (termasuk karyawan nonaktif) | "NIP … sudah ada (mode Tambah hanya untuk karyawan baru)". Tidak menyebut pemilik NIP, status nonaktifnya, atau jalan keluarnya |

## Ruling
- **R1** — NIP Atasan boleh menunjuk karyawan baru di file yang sama. Atasan dipasang setelah semua baris baru dibuat, dalam transaksi impor yang sama. Urutan baris di file tidak berpengaruh.
- **R2** — NIP Atasan dibersihkan dengan aturan yang sama seperti kolom NIP (`CleanNIP`: trim, tanda kutip di depan, notasi ilmiah Excel).
- **R3** — Mode Tambah **tetap menolak** NIP yang sudah ada, demi keamanan. Pesannya diperjelas: menyebut nama pemilik, status nonaktif bila ada, dan arahan memakai mode Timpa.
- **R4** — Tidak ada perubahan skema DB. Jalur Next fallback tidak ada untuk impor ini (hanya Go).

## Tasks
Lihat `docs/superpowers/plans/2026-11-10-impor-karyawan-nip-atasan.md`.

## Acceptance Criteria
- [x] File berisi SPV baru dan staf baru dengan NIP Atasan = NIP SPV tersebut lolos pratinjau dan masuk. `reporting_to` staf menunjuk SPV, baik baris SPV di atas maupun di bawah stafnya.
- [x] NIP Atasan `'202611676` dikenali sama dengan `202611676`.
- [x] Mode Tambah dengan NIP yang sudah ada menampilkan pemilik, status nonaktif bila ada, dan arahan ke mode Timpa.
- [x] NIP Atasan yang tidak ada di HRIS maupun di file tetap ditolak.
- [x] Gate hijau: unit dan integration `backend/internal/modules/hris`, serta CI PR.

## Automation Log
- 2026-11-10: Laporan user. Reproduksi di staging: B1, B2, B3 terbukti; jalur NIP teks, angka, kutip, dan NBSP sudah benar. Data uji dan sesi QA staging sudah dihapus. Rencana ditulis.
- 2026-11-10: **Implementasi Native** di branch `fix/impor-karyawan-nip-atasan`.
  - Task 1 (domain, 7f64f523): NIP Atasan memakai `CleanNIP`; pre-pass `newInFile` menandai `ManagerNIPNew`; pesan mode Tambah memuat pemilik, `(nonaktif)`, dan arahan Timpa.
  - Task 2 (service, dad3155a): `reporting_to` dipasang setelah semua insert dalam transaksi impor, ditambah audit `employee.update`. Baris Timpa yang perubahannya hanya atasan baru dihitung update.
  - Review akhir (opus): Critical 0, Important 1, Minor 5.
    - Important diperbaiki (916493ff): baris karyawan resign tetap ditolak walau atasannya dibuat di file yang sama. Tes `TestValidateEmployeeImportReplaceResignedNewManager` RED→GREEN.
    - Minor ditunda: pesan notasi ilmiah menyebut "kolom NIP", label audit, error DB jadi 400, siklus atasan.
  - Gate: `go test ./internal/modules/hris/...` hijau (DB uji migrasi 587).
  - Staging 07.13 (image `local-20261110-071340`, backup `/mnt/hdd1/backup/paddydev/staging-before-e098-20261110-070757.dump`):
    - B1: SPV + staf di satu file masuk, atasan staf = SPV;
    - B2: NIP Atasan `'202611676` diterima;
    - B3: pesan "sudah dipakai Naisya Septya Wardhani (nonaktif) — … pakai mode Timpa".
    - Data dan sesi uji sudah dihapus.
  - Deploy staging pertama gagal karena cache build Docker modul Go rusak (excelize `lib.go`), kemungkinan sisa disk penuh 09-11. Build `--no-cache` berhasil dan deploy ulang lancar.
- 2026-11-13: Registry dirapikan, status review → ready-for-qa. Kode sudah ada di erp-paddy `development` (impor monorepo e815cee) dan jalan di staging (image `local-20261112-234046`). Sisa: QA UI oleh HR, lalu rilis production dengan izin user.

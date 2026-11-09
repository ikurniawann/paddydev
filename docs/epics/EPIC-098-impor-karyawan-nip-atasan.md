# EPIC-098: Impor Karyawan Excel Ditolak karena NIP Atasan (`MODULE-HRIS`)

status: on-progress
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
- [ ] File berisi SPV baru dan staf baru dengan NIP Atasan = NIP SPV tersebut lolos pratinjau dan masuk. `reporting_to` staf menunjuk SPV, baik baris SPV di atas maupun di bawah stafnya.
- [ ] NIP Atasan `'202611676` dikenali sama dengan `202611676`.
- [ ] Mode Tambah dengan NIP yang sudah ada menampilkan pemilik, status nonaktif bila ada, dan arahan ke mode Timpa.
- [ ] NIP Atasan yang tidak ada di HRIS maupun di file tetap ditolak.
- [ ] Gate hijau: unit dan integration `backend/internal/modules/hris`, serta CI PR.

## Automation Log
- 2026-11-10: Laporan user. Reproduksi di staging: B1, B2, B3 terbukti; jalur NIP teks, angka, kutip, dan NBSP sudah benar. Data uji dan sesi QA staging sudah dihapus. Rencana ditulis.

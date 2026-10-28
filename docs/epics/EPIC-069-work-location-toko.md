# EPIC-069: Work Location Toko — Lengkapi Profil, Tambah Toko Baru, Benahi Penempatan (`MODULE-HRIS`)

status: ready-for-qa
environment: dev
retries: 0

## Goal
Data toko di production (`configuration.branches`) sesuai Excel HR "20261001 - Work Location - New" (Sheet1: 287 karyawan + daftar lokasi, Sheet2: 38 toko):
1. **Lengkapi 39 toko yang sudah ada** + Paddy HQ Bandung + Warehouse Paddy: nama `KODE - Nama Store`, alamat, kota, kode pos, koordinat dari pin Google Maps.
2. **Tambah 3 toko baru, aktif:** TWSP (Medan), TWMS (Sidoarjo), TWEM (Mataram, status VM, buka 20-11-2026).
3. **Benahi penempatan utama 22 staf toko** yang berbeda dengan Excel.
4. Koordinat terisi = **geofence absen 100 m aktif** (AB-01, K-14: data dari Paddy).

## Keputusan user (28-10-2026)
- Nama toko: **`KODE - Nama`** (mis. `PPGA - Paddy Playstore Backyard`). Nama ini juga dicerminkan ke `item.brands` (trigger) dan tampil di semua dropdown.
- Koordinat: **isi sekarang, sesudah penempatan dibenahi** (satu transaksi). TWMS memakai titik MLMS (gedung sama, Lippo Plaza Sidoarjo). 5 toko tanpa pin (link `share.google`): MUBM, TWSP, MBRM, TWEM, TWLK → HR mengisi lewat **Lokasi Toko**.
- Toko baru: **ketiganya aktif**.
- Penempatan: **hanya 22 staf toko**. Staf "Headquarter" di Excel yang di production tercatat di Paddy Pudak/Salam/Warehouse (51 orang) dan 4 orang "mobile" **tidak diubah**.

## Ruling (tanpa tanya)
- Pemindahan penempatan meniru pola riwayat: penempatan lama diakhiri **kemarin (WIB)**, penempatan utama baru mulai **hari ini**; bila penempatan lama mulai hari ini, tokonya diganti. Setiap karyawan hanya punya satu penempatan aktif sesudahnya.
- Kode toko baru tanpa awalan `ODOO-` (bukan dari Odoo); tidak ada kode yang bergantung pada awalan itu.
- Paddy HQ Bandung (`MIZU-BANDUNG`) tetap memakai nama dan koordinat lama (selisih ±50 m dari pin Excel); hanya alamat yang diisi.
- Toko di production yang tidak ada di Excel (EVENT-PVJ, MATS, MCPZ, MCSB, MFCB, MSMB, MUMS, PLPJ), Paddy Pudak/Salam, dan PD-KOPO/PD-PVJ (situs publik) tidak disentuh. Sesudah pindah, EVENT-PVJ dan MCPZ tidak punya karyawan aktif.
- Koordinat dari URL Google Maps (`!3d`/`!4d` = titik tempat), dibulatkan 6 desimal.

## Tasks
- [x] T1 Selidiki read-only: Excel vs production (53 cabang, 631 karyawan, 336 penempatan aktif).
- [x] T2 Skrip `backend/database/sql/fixes/2026-10-28_work_location_toko.sql` (satu transaksi, pratinjau read-only, `RAISE EXCEPTION`, audit).
- [x] T3 Gladi di salinan production 28-10 12:14.
- [x] T4 PR #43 CI hijau 4/4, merged 28-10 (`515a7b29`).
- [x] T5 Eksekusi production 28-10 12:38 WIB atas izin user.
- [x] T6 Catat hasil di log ini + acuan AB-01. Daftar tindak lanjut di bawah diserahkan ke HR.

## Penempatan yang dipindah (22)
| NIP | Nama | Sekarang | Baru (Excel) |
|---|---|---|---|
| 202405059 | Ricky Pratama Sidik | MIZU-BANDUNG | PPGA
 |
| 202412128 | Gritt Alvrin | MIZU-BANDUNG | PPTN
 |
| 202501138 | Azka Hanifah Hamidah | MIZU-BANDUNG | PLRM
 |
| 202502168 | Syifa Listia | MBIP | PPBA
 |
| 202503192 | Ivan Sendina | MCPZ | PPGA
 |
| 202503225 | Devina Hanida | PPVJ | PPGA
 |
| 202507358 | Irvan Ramadhana Putra | MIZU-BANDUNG | PPYA
 |
| 202509413 | Diandra Pradnadya | PPAY | PLAY
 |
| 202509414 | Siti Maudi | PPAY | PLAY
 |
| 202510442 | Farhan Farras Prawira | MIZU-BANDUNG | PPJA
 |
| 202512505 | Riswandi | MIZU-BANDUNG | PBSA
 |
| 202601521 | Muhammad Taufik Hidayat | MPVJ | PBSA
 |
| 202601522 | Riki Juliana | MIZU-BANDUNG | PBSA
 |
| 202601523 | Gaza Fiqih Aulia | EVENT-PVJ | PBSA
 |
| 202601524 | Tya Nur Azizah | EVENT-PVJ | PPVJ
 |
| 202601525 | Muhamad Daniel | PPLN | PLPK
 |
| 202601528 | Ryan Fazar | PPLN | PLBM
 |
| 202602545 | Jusmiati | MIZU-BANDUNG | PPMR
 |
| 202603552 | Annisa Rahma | MBIP | MTSM
 |
| 202604591 | Stevendino Marselino | PPVJ | PBSA
 |
| 202605596 | Adelia Puspita Sari | PPMR | PLMR
 |
| 202607626 | Lala Oliviana | PPPA | MDTS
 |

## Tindak lanjut HR (di luar skrip)
- Pin koordinat 5 toko lewat Lokasi Toko: MUBM, TWSP, MBRM, TWEM, TWLK.
- 51 staf "Headquarter" di Excel tercatat di Paddy Pudak (22), Paddy Salam (15), Warehouse (14): tetap seperti sekarang; HR memastikan.
- 2 staf "Warehouse" di Excel tercatat di Paddy HQ Bandung (Syafira Nurizka Budiman, Kania Dewi Lastari).
- 4 staf "mobile": Elvan Sabiq (MPSL), Raya Juniar Nugraha (PPGA), Shelvira Ramadhani (PPTN), Kallia Amala Sagwiangsa (HQ). Penempatan tetap; untuk absen di toko lain perlu penempatan tambahan.
- Tidak ditemukan di production (karyawan aktif): Elsa Ratu Wimala, Naila Fatiyah Salsabila. Kezia Imanuella Hendra tidak punya penempatan.
- 56 karyawan aktif di production tidak ada di Excel.
- 8 toko production yang tidak ada di Excel: nonaktifkan atau biarkan (keputusan HR).

## Acceptance Criteria
- Pratinjau di production menampilkan 44 baris toko dan 22 pindah, semua sesuai; jalur gagal (data berubah, expected salah) membatalkan tanpa perubahan.
- Sesudah eksekusi: 3 toko baru aktif (+ gudang bawaan + brand), 41 toko dilengkapi, 22 karyawan punya tepat satu penempatan utama aktif di toko Excel, 66 entri audit.
- Toko aktif tanpa koordinat hanya: 5 toko di atas, 8 toko di luar Excel, Paddy Pudak/Salam, PD-KOPO/PD-PVJ.

## Automation Log
- 2026-10-28: **Instruksi user:** "ada worklocation / store paddy coba lengkapi dan tambah jika tidak ada di list store nya cek pada paddy-prod dulu sebelum eksekusi … sebelum jadi epic". Keputusan di atas (satu kali tanya jawab, semua opsi rekomendasi).
- 2026-10-28: **Gladi** di salinan production `~/Desktop/paddydev-backups/prod-copy-epic069-20261028-121446.dump` (DB uji `e069_prod` di 55499, pg_restore pg16): pratinjau sesuai; terapkan 0,13 dtk → toko baru 3, dilengkapi 41, dipindah 22, audit 44 + 22, brand 53→56, gudang 55→58, penempatan 629→651; jadwal shift mendatang di toko lama 0. Jalur gagal: jalankan ulang → "3 toko baru sudah ada", `expected_stores=43` → batal, tanpa `-v` → berhenti.
- 2026-10-28: **Instruksi user:** "Jalankan sekarang" (izin merge PR #43 + eksekusi production saat itu juga).
- 2026-10-28: **Production DIEKSEKUSI 12:38:49–12:38:50 WIB** dari main `515a7b29` (sha256 skrip sama dengan gladi):
  - backup VPS `~/backups/prod-db-before-epic069-20261028-123821.dump` (6,5 MB); log `~/backups/epic069-execute-20261028-123821.log`;
  - pratinjau production = gladi (44 toko, 22 pindah, penempatan sekarang = kolom "dari" semuanya, jadwal shift mendatang di toko lama 0);
  - terapkan: toko baru 3, dilengkapi 41, dipindah 22, tanpa downtime (tanpa deploy).
- 2026-10-28: **Verifikasi read-only production:**
  - audit `branch.work_location_import` 44 + `placement.work_location_moved` 22;
  - TWSP/TWMS/TWEM aktif, masing-masing punya 1 gudang bawaan + 1 brand; brand 56, gudang 58, cabang 56;
  - cabang aktif berkoordinat 39 dari 56;
  - 22 karyawan punya penempatan utama aktif di toko Excel; karyawan dengan >1 penempatan aktif 0;
  - HTTP `/` 302 (ke login), `/api/hris/branches` 401; error API 10 menit terakhir 0.
- Rollback: restore `~/backups/prod-db-before-epic069-20261028-123821.dump`, atau balikkan per entri audit (`before`).

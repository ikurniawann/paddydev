# EPIC-116: Hapus Perusahaan CV Cipta Kreasi Kreatif (ERP) (`MODULE-CONFIGURATION`)

status: done
environment: production
retries: 0

## Goal
Laporan user 18-11-2026: perusahaan CV Cipta Kreasi Kreatif (`ERP`, nonaktif) tidak bisa dihapus dari Struktur Bisnis, dan pesannya "Referensi data tidak valid". Holding sudah bernama CV Cipta Kreasi Kreatif, kodenya `PADDY-GROUP` di staging dan `PADDY` di production.

Rencana: `docs/superpowers/plans/2026-11-18-hapus-perusahaan-erp.md`. Kode: erp-paddy PR [#20](https://github.com/wit-id-indonesia/erp-paddy/pull/20) (cbcf50a).

## Akar masalah
Sesudah EPIC-115, ERP masih dirujuk oleh riwayat gaji nonaktif (FK RESTRICT): 125 baris di staging, 127 di production. `historyGuard` hanya memeriksa histori branch, sehingga error FK lolos sebagai pesan umum.

## Keputusan
- **K-1 (user 18-11):** riwayat gaji nonaktif dipindah ke Paddy, lalu ERP dihapus. Pemetaan jurnal ERP (26) ikut terhapus; salinannya sudah ada di Paddy sejak EPIC-115.
- **K-2 (user 18-11):** pesan hapus perusahaan/holding menyebut tabel dan jumlah baris yang masih memakainya. Rujukan pada baris anak yang ikut terhapus juga dipetakan dari error 23503.

## Acceptance Criteria
- [x] Hapus perusahaan/holding yang masih dipakai ditolak dengan pesan jelas (tes Go dan Next).
- [x] Skrip hapus ERP: pratinjau, BLOKIR (dipakai, payroll, gaji bentrok, pengaturan CRM, tabel `*company_id`), snapshot, dan pembalik.
- [x] Staging dan production dieksekusi.
- [ ] Pesan hapus yang baru sampai di production pada rilis berikutnya. Kodenya baru ada di `development`, dan rilis itu wajib gladi ulang.

## Automation Log
- 2026-11-18: Eksekusi Native dengan TDD. Commit:
  - 635c9c3: pesan hapus;
  - a28fb0a: skrip;
  - 919eb3c: pembalik;
  - 6865046: perbaikan review.
  
  Review akhir oleh reviewer segar (opus): tidak ada temuan kritis, 2 penting, 4 minor. Kedua temuan penting diperbaiki dengan tes RED→GREEN:
  - 23503 dari baris anak cascade;
  - BLOKIR `crm_settings default_company_id`; nilainya di staging dan production menunjuk PADDY-RETAIL.
  
  Gerbang: `go test` hris+configuration ok; vitest 12/12; tsc 0 error. PR #20 di-merge sesudah CI hijau.
- **Minor yang ditunda:**
  - guard melakukan count(*) pada kolom FK tanpa indeks;
  - pembalik memindah gaji kembali walau sudah diedit di Paddy;
  - `jsonb_populate_record` mengisi NULL untuk kolom yang ditambah sesudah snapshot;
  - nama tabel bisa ganda bila ada dua FK.
- 2026-11-18 22.10 WIB: Staging:
  - backup `/mnt/hdd1/backup/paddydev/staging-before-hapus-erp-*.dump` (4.608 entri);
  - pratinjau 0 penghalang;
  - eksekusi 22.10.55: 125 gaji pindah, 26 pemetaan dan 1 perusahaan terhapus, invarian OK.
  
  Holding PADDY-GROUP kini hanya berisi PADDY-RETAIL; `/api/ready` 200; 0 gaji yatim.
- 2026-11-18 22.11: Gladi di salinan production terbaru (sesudah EPIC-115): pratinjau dan eksekusi 0 penghalang (127 gaji / 26 pemetaan), pembalik md5 identik (e077969d…). Container dibongkar.
- 2026-11-18 22.11.51 WIB: **Production dieksekusi**:
  - backup `~/backups/prod-db-before-hapus-erp-*.dump` beserta salinannya di `/mnt/hdd1/backup/paddydev/` (sha 6393266a…, 4.412 entri);
  - pratinjau sama dengan gladi;
  - eksekusi: 128 baris, invarian OK, COMMIT.
  
  Smoke:
  - `/login` dan `/api/ready` 200, 0 error API;
  - holding PADDY "CV Cipta Kreasi Kreatif" hanya berisi PADDY-RETAIL;
  - 0 gaji yatim, 0 user yatim;
  - PADDY-RETAIL 311 gaji aktif / 656 baris gaji.

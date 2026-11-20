# EPIC-119: Gaji HQ Tidak Dibagi, Cuti Ulang Tahun 2 Bulan, Koreksi Terlambat Bungsu (`MODULE-HRIS`, `MODULE-PAYROLL`)

status: ready-for-qa
environment: dev
retries: 0

## Goal
Laporan user 19-11-2026:

> Mas Bungsu HQ (2 Penempatan Warehouse) ke detect Warehouse jadi ke detect terlambat
> Yudho (seting dlu jadwal kerja) => clock in lagi
> Yana cuti salah input malah di server reddie bukan yang di paddy
> cuti ulang tahun (valid 2 bulan dari tgl ulang tahunnya)
> karyawan yang sudah lebih dari 1 tahun tidak bisa mengajukan izin pribadi
> perhitungan gaji di kantor pusat tidak di bagi

Keputusan user (AskUserQuestion 19-11):
- Gaji HQ: "Penempatan utama HQ: 100% HQ". Staf toko tetap dibagi per jam kerja per toko.
- Cuti ulang tahun: "Tgl ultah s.d. 2 bulan" (ultah 15 Nov → 15 Nov s.d. 14 Jan), tetap 1x per ulang tahun, H-3, lampiran wajib, sesudah kontrak awal.
- Bungsu: "Koreksi via skrip" (backup dulu, pratinjau disetujui user sebelum eksekusi).
- Rilis production: "rilis prod nanti aja after office hours".

Rencana: [2026-11-19-gaji-hq-cuti-ultah-koreksi-terlambat.md](../superpowers/plans/2026-11-19-gaji-hq-cuti-ultah-koreksi-terlambat.md).

## Temuan (dicek read-only di production 19-11)
- **Bungsu**: departemennya sempat Warehouse 09-11 s.d. 13-11 (pola OPS 08.00, toleransi 10), lalu HR mengembalikannya ke Operations (pola HQ NS, masuk sampai 09.00) pada 13-11 09.13 WIB. Terlambat 10-11 (08.14), 11-11 (08.10), 13-11 (08.11) terjadi karena jadwal OPS, bukan karena penempatan Warehouse. Hanya Bungsu yang terdampak.
- **Yudho**: pola jadwal NS dari go-live EPIC-056 berakhir 02-11; sejak 03-11 tidak ada jadwal (18 hari kosong di periode). HR perlu mengisi roster sebelum clock-in ulang.
- **Jadwal kosong**: 190 karyawan shift aktif punya minimal satu tanggal tanpa jadwal di periode 21-10 s.d. 20-11; 59 tanpa jadwal hari ini (19-11). Daftar untuk HR: `/mnt/hdd1/backup/paddydev/karyawan-shift-tanpa-jadwal-20261119.csv` (mode 600, tidak di-commit).
- **Yana**: tidak ada pengajuan cuti Yana di staging maupun production. Data Yana di staging sesudah salinan 10-11 hanya clock-in 10-11 09.07 (bentrok dengan production 18.12, salah satu dari 8 bentrok EPIC-117). HR perlu menyebut tanggal dan jenis cuti untuk dicatat di production.
- **Izin Pribadi > 1 tahun**: syarat "Cuti Tahunan habis" sudah dihapus EPIC-112, live di production sejak 19-11 01.55 WIB. Tes regresi ditambah.
- **Gaji HQ dibagi**: di staging, 51 slip staf HQ terbagi HQ + Pudak/Salam/Warehouse karena absen sebelum 10-11 tercatat di cabang lama Odoo.

## Tasks
- [x] Task 1: Gaji staf kantor pusat 100% ke HQ (`is_head_office`, basis `head_office`)
- [x] Task 2: Cuti Ulang Tahun berlaku sejak tanggal ulang tahun sampai 2 bulan
- [x] Task 3: Skrip koreksi terlambat Bungsu + pembalik (dieksekusi di production 20-11-2026 20.47 WIB)
- [x] Task 4: Tes regresi Izin Pribadi untuk karyawan lebih dari 1 tahun
- [x] Task 5: Daftar jadwal kosong untuk HR
- [ ] Task 6: Gerbang akhir, review, PR, staging, rilis production sesudah jam kantor

## Acceptance Criteria
- Slip staf yang penempatan utamanya HQ punya satu baris alokasi HQ 100%; staf toko tetap dibagi.
- Cuti Ulang Tahun ditolak sebelum tanggal ulang tahun dan sesudah 2 bulan, dengan pesan berisi rentangnya; 1x per ulang tahun.
- Skrip Bungsu: pratinjau tidak mengubah data; eksekusi menghapus 3 status terlambat; pembalik mengembalikan persis; menolak bila data berubah.

## Automation Log
- 19-11-2026: rencana ditulis dan disetujui user ("sesuai"); eksekusi native (pilihan "118 subagent, 119 native").
- 19-11-2026: commit `493813f` (alokasi HQ), `e82eebb` (cuti ulang tahun), `69e6f7c` (skrip Bungsu), tes Izin Pribadi. Branch `feat/epic119-hq-ultah` (erp-paddy).
- Ruling: `AllocateHeadOffice` menerima rentang periode (start, end) agar hari dihitung sama dengan `AllocateSlip`.
- Ruling: pembalik skrip Bungsu memakai sidik md5 baris (jam, jadwal, status terlambat; timestamp sebagai epoch) karena `updated_at` tidak berubah dalam satu transaksi dan bergantung TimeZone sesi.
- Ruling: daftar HR memakai "tanggal tanpa jadwal per karyawan", bukan "tanpa jadwal sama sekali", supaya Yudho (pola berakhir 02-11) ikut.
- Review akhir: label ekspor kantor pusat, kuota cuti ulang tahun lintas aturan lama, dan pengaman skrip Bungsu (lock_timeout, tolak eksekusi kedua, tolak roster) diperbaiki di `9de4ee0`.
- Merge EPIC-118 ke branch ini (`d5b266c`): aturan kantor pusat dipindah ke `calcEnv`/`calcOne` supaya hitung penuh dan hitung ulang satu karyawan sama (`TestRecalcEmployeeHeadOfficeNotSplit`).
- PR [#24](https://github.com/wit-id-indonesia/erp-paddy/pull/24), di-merge sesudah #25.
- 19-11-2026: #23, #25, #24 di-merge ke development; deploy staging manual (workflow_dispatch, CI GitHub mati karena billing) di `691b29f`, smoke test lulus.
- Temuan staging: kantor pusat di staging berkode `HQ-PADDY`, bukan `ERP-BANDUNG`, jadi tidak ada cabang bertanda kantor pusat. Migrasi `20261219100100` menandai `HQ-PADDY` (production tidak punya kode itu). PR #27, staging `6cf7944`. Run November staging (draft) perlu dihitung ulang HR.
- 20-11-2026 19.13–19.17 WIB: **rilis production** (rilis ke-4, gabungan EPIC-118 s.d. 123) atas izin user ("Ya, rilis sekarang"), setelah gladi 18.50 lulus. Backup `~/backups/prod-db-before-rilis-epic123-20261120-1913.dump` + `~/paddydev-src.before-rilis-epic123-20261120-1913`. PR #36 (development → production) di-merge (bbf7dfc, tree = development 778d0bd). Source dari `git archive origin/production` di-rsync ke `paddydev-src` (147 file berubah, 2 file lama dihapus), lalu `bootstrap.sh` + `deploy-paddydev.sh`. Log `~/backups/deploy-prod-epic123-20261120-1913.log`: 4 migrasi, verify 12.393 objek 0 selisih, image `local-20261120-191457`, EXIT=0. Smoke: `/api/ready`, `/api/health`, `/login` 200; `/api/hris/overtime/quota` tanpa login 401; 0 error log; 1.203 sesi tetap; head office ERP-BANDUNG. Rollback: deploy ulang dari folder `.before-rilis-epic123-20261120-1913` (image lama `local-20261120-090127`), tanpa membatalkan migrasi. Sisa: HR menghitung ulang run November (draft); skrip Bungsu menunggu izin; PR #34 tergantikan.
- 20-11-2026 20.47 WIB: **skrip Bungsu dieksekusi di production** atas izin user ("Ya, eksekusi") setelah pratinjau di production lulus (0 penghalang). Backup `~/backups/prod-db-before-bungsu-epic119-20261120-2047.dump` (mode 600). Hasil: 7 pola 09-11 s.d. 12-11 dari OPS ke NS (Sen–Jum NS, Sab–Min kosong; pola itu hanya berlaku Sen–Kam), 4 absensi 10–13 Nov ke jadwal NS 09.00, 3 status terlambat (10-11 08.14, 11-11 08.10, 13-11 08.11) dihapus, invarian OK, 0 notifikasi, 0 error log API. Audit `e66ad666-b511-40c7-90c4-de2ca3044a33` (`data_restructure.bungsu_late_fix`). Pembalik: `2026-11-19_bungsu_jadwal_hq_terlambat_balik.sql`.

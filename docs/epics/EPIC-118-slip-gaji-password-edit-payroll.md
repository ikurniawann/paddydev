# EPIC-118: Slip Gaji Ringkas, Password ESS, Edit Payroll per Karyawan (`MODULE-PAYROLL`)

status: review
environment: dev
retries: 0

## Goal
Permintaan user 19-11-2026:

> payroll ketika akan edit karyawan satu satu harus nya bisa ya, skg di staging dan production payroll aksi cuman liat slip gaji aja lalu apa di slip gaji rincian toko di hilangkan aja dan tambahan di ringkasan bagian atas dengan info cuti unpaid dan cuti paid di tambahan diatas jadi card yang di bawah hilang aja
>
> ketika gaji di proses dan sudah disetujui, slip gaji karyawan ketika akan unduh harus ada keamanan input password karyawan bersangkutan demi keamanan ess karyawan lain agar tidak bisa di baca oleh user lain
>
> hilangkan keterangan label ini semua yang di ada keterangan di payslip contoh: R-01, R-02, R-03, R-05, R-90a

Keputusan user (AskUserQuestion 19-11):
- Edit per karyawan: "Komponen + hitung ulang" (hitung ulang karyawan itu saja).
- Password: "Rincian dan unduh" (daftar ESS hanya periode; angka, rincian, PDF sesudah password login benar; diperiksa server).
- Rilis slip ESS: "Sejak Disetujui" (status `completed` atau `paid`).
- Kode R-: "Semua tampilan" (ekspor CSV tetap memakai kode).
- Rilis production: "rilis prod nanti aja after office hours".

Rencana: [2026-11-19-slip-gaji-password-edit-payroll.md](../superpowers/plans/2026-11-19-slip-gaji-password-edit-payroll.md). Eksekusi subagent-driven (pilihan "118 subagent, 119 native").

## Tasks
- [x] Task 1: Hapus kode R- dari tampilan frontend
- [x] Task 2: Slip HR tanpa Rincian per toko dan tanpa empat kartu bawah
- [x] Task 3: PDF slip tanpa kode R- dan tanpa Rincian per Toko
- [x] Task 4: Backend hitung ulang satu karyawan
- [x] Task 5: Dialog Edit per karyawan
- [x] Task 6: Slip terbit di ESS sejak Disetujui
- [x] Task 7: Kunci slip ESS dengan password login (backend)
- [x] Task 8: ESS Slip Gaji dengan dialog password
- [ ] Task 9: Gerbang akhir, review, PR, staging, rilis production sesudah jam kantor

## Automation Log
- 19-11-2026: rencana disetujui user ("sesuai"). Branch `feat/epic118-slip-gaji` (erp-paddy).
- Task 1 `7d5c586`, Task 2 `8e51ff7`, Task 3 `79f2e65`: lolos review per task.
- Ruling R4: tooltip `showSource` di slip HR masih memuat kode rule ("…; R-01"); slip HR berhenti mengirim `showSource` sehingga tooltip hilang ("Semua tampilan").
- Task 4 `34557fa..430b08c`: dua putaran perbaikan. (1) Total run satu sumber (`refreshRunTotals`), sebelumnya selisih Rp1/karyawan pada gaji tidak bulat. (2) Kunci per run dengan `pg_try_advisory_xact_lock` di hitung penuh, hitung satu karyawan, dan ubah status; 409 bila sibuk. Kunci yang menunggu sempat bisa menghabiskan pool koneksi production (4 koneksi, 4 CPU).
- Ruling R5: tidak ada backfill `include_thr` (run staging November tanpa THR, production 0 run).
- Ruling R6: kunci tidak pernah menunggu sambil memegang koneksi.
- Ruling R7: kartu "Slip Gaji Terakhir" di beranda ESS dan `latest_payslip` tidak lagi membawa nominal (keputusan "Rincian dan unduh").
- Task 5 `dadca22`, Task 6 `58fc1a2..7e6ffbc`, Task 7 `136f6a5` (review keamanan bersih), Task 8 `07209c2`.
- Review akhir: 4 temuan Important (layar ESS terkunci lagi saat masa buka habis, cache slip HR sesudah Edit, unduh PDF di iPhone, password di cache mutasi) diperbaiki di `68d5929` dan direview ulang bersih.
- PR [#25](https://github.com/wit-id-indonesia/erp-paddy/pull/25). GitHub Actions tidak berjalan (billing); gerbang CI dijalankan lokal dengan perintah yang sama.
- Belum: QA manual di iPhone Safari dan browser WhatsApp Android.
- 19-11-2026: di-merge ke development (#25) dan live di staging `6cf7944`. Tombol tutup banner STAGING (#26) ikut.
- 20-11-2026 19.13–19.17 WIB: **rilis production** (rilis ke-4, gabungan EPIC-118 s.d. 123) atas izin user ("Ya, rilis sekarang"), setelah gladi 18.50 lulus. Backup `~/backups/prod-db-before-rilis-epic123-20261120-1913.dump` + `~/paddydev-src.before-rilis-epic123-20261120-1913`. PR #36 (development → production) di-merge (bbf7dfc, tree = development 778d0bd). Source dari `git archive origin/production` di-rsync ke `paddydev-src` (147 file berubah, 2 file lama dihapus), lalu `bootstrap.sh` + `deploy-paddydev.sh`. Log `~/backups/deploy-prod-epic123-20261120-1913.log`: 4 migrasi, verify 12.393 objek 0 selisih, image `local-20261120-191457`, EXIT=0. Smoke: `/api/ready`, `/api/health`, `/login` 200; `/api/hris/overtime/quota` tanpa login 401; 0 error log; 1.203 sesi tetap; head office ERP-BANDUNG. Rollback: deploy ulang dari folder `.before-rilis-epic123-20261120-1913` (image lama `local-20261120-090127`), tanpa membatalkan migrasi. Sisa: HR menghitung ulang run November (draft); skrip Bungsu menunggu izin; PR #34 tergantikan.

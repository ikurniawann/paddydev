# EPIC-057: Hak Akses HRIS per Akun Email

status: ready-for-qa
environment: dev
retries: 0

## Goal
Tiga akun HR mendapat hak akses masing-masing (Manager seluruh HRIS; Staff Non-Payroll; Staff Non-Rekrutmen & KPI), dengan menu, halaman, dan API yang konsisten.

## Tasks
Lihat docs/superpowers/plans/2026-10-19-hris-akses-per-akun.md (Task 1–8).

## Acceptance Criteria
- paddyhumanresources@gmail.com membuka seluruh menu HRIS.
- paddy.peopledevelopment@gmail.com tidak melihat Penggajian; Pinjaman tetap; kontrak tampil tanpa angka gaji dan tanpa PDF; slip gaji orang lain tertutup.
- paddyhroperation@gmail.com tidak melihat Rekrutmen, Kinerja, maupun Analytics; beranda ke daftar karyawan; KPI/Review/Task Departemen/Logbook hanya data sendiri.
- admin_hris dan peran lain tidak berubah.

## Automation Log
- 2026-10-19: rencana ditulis; keputusan user & ruling R1–R7 tercatat di rencana.
- 2026-10-20: Task 1–7 selesai di branch `feat/epic057-hris-akses-per-akun`. Role turunan di DB test: admin_hris 55 grant, hris_manager 55, hris_non_payroll 51, hris_non_rekrutmen_kpi 39. Skrip ops diuji di DB test (guard menolak bila akun kurang dari 3; penugasan 3 akun benar, app_role tetap hrd). Seeder memakai `email|Nama|role` dan menyinkronkan role turunan.
- 2026-10-20: Review akhir (reviewer independen) — diperbaiki: konfigurasi & rubrik KPI kini butuh menu Kinerja; pesan batas pinjaman tidak menyebut angka maksimum (= gaji pokok) untuk akun tanpa Penggajian (ajukan & approve); slip finance_staff tetap seperti dulu; skrip ops menyinkronkan role, memeriksa grant aktif, dan gagal dengan kode error. Full suite Go = baseline main (27, nama sama); vitest 5497 lolos.
- 2026-10-20: Uji stack salinan DB (dump live 20-10, 3 akun uji per role, akun asli tidak diubah passwordnya): Manager semua terbuka; Non-Payroll tanpa Penggajian (halaman → Area Karyawan, slip orang lain 0, PDF 403, kontrak tanpa gaji/PDF, Pinjaman jalan); Non-Rekrutmen & KPI beranda → Karyawan, Rekrutmen/Analytics/KPI config 403, KPI hanya data sendiri. Screenshot: .share/epic057/.
- Keputusan turunan: hiring_manager tanpa menu Kinerja kehilangan tampilan KPI semua karyawan (live: 0 pengguna); beranda finance_staff tidak diubah (live: 0 pengguna).
- 2026-10-20 05:47 WIB: DEPLOY LIVE. Backup `~/Desktop/paddydev-backups/paddydev-db-before-epic057-20261020-054721.dump`. main 8775d5a di-push ke paddydev. Migrasi 20261019100000 (grant: admin_hris 55, hris_manager 55, hris_non_payroll 51, hris_non_rekrutmen_kpi 39). Skrip ops: Sebelum ketiga akun admin_hris → Sesudah hris_manager / hris_non_payroll / hris_non_rekrutmen_kpi, peran aplikasi tetap hrd. Container API & web `local-20261020-054738-epic057` sehat (rollback: image `local-20261019-201743-epic056`).
- Verifikasi live paddyhroperation@gmail.com: beranda → /dashboard/employees; sidebar tanpa Rekrutmen & Kinerja (Penggajian, Laporan, Pinjaman ada); /dashboard/hris/candidates & analytics → /dashboard/me; API dashboard rekrutmen, analytics, kpi-config 403; KPI is_hr false; gaji & PDF kontrak, slip semua karyawan, Pinjaman 200; tanpa error console. Dua akun lain diverifikasi lewat grant DB (non-payroll: Penggajian 0, Pinjaman approve 1; manager: semua).
- QA manusia: login paddy.peopledevelopment@gmail.com dan paddyhumanresources@gmail.com untuk konfirmasi tampilan.

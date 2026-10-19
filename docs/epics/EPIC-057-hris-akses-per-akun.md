# EPIC-057: Hak Akses HRIS per Akun Email

status: on-progress
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

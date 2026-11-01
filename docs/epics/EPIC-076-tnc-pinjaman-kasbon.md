# EPIC-076: Syarat & Ketentuan Pinjaman dan Kasbon (`MODULE-HRIS`)

status: backlog
environment: dev
retries: 0

## Goal
Terms & conditions dari user 01-11-2026 untuk **Pinjaman dan Kasbon**:

> Masa probation tidak dapat mengajukan, PKWT dan PKWTT bisa mengajukan dengan catatan max. 50% dari THP dengan 1 s.d 2x cicilan dipotong dari Payroll. Pengajuan dalam masa periode payroll 21 s.d 20 yang akan terpotong di periode payroll bulan berjalan.

Ini adalah TnC yang ditunggu acuan MOM **KP-03 "Aturan Pinjaman"** (status "Menunggu TnC"). Kondisi kode per 01-11 (main `b97e10d3`):

| Aturan | Kasbon (`kasbon`) | Pinjaman (`loan`) |
|---|---|---|
| Status yang boleh | Semua kecuali `probation`, `freelance`, `intern`, `internship` (daftar blokir, EPIC-060) | Semua status |
| Plafon | Pokok ≤ 50% THP (Gaji Pokok + Transport + Makan + Jabatan, struktur gaji aktif) | Cicilan/bulan ≤ `loan_max_installment_percent` (default 30%) dari **gaji pokok** |
| Tenor | 1 atau 2 | 1–60 bulan |
| Cicilan pertama | Bulan **sesudah persetujuan** (bulan kalender, `domain.FirstInstallmentPeriod`) | sama |
| Approval | Manager Departemen lalu HR Penggajian (AP-05) | HR satu tahap |

Payroll mengambil cicilan pada run bulan `M` bila `first_installment ≤ M` (`domain.IsLoanDue`), dihitung saat run diproses dan dilunasi saat run `paid`. Status run: `draft → processing → completed → paid`. Pengaturan cutoff absensi live masih `cutoff_day = 0` (PY-02).

## Ruling (dibuat tanpa tanya; mohon dicek saat review rencana)
- **R1** TnC berlaku sama untuk **Pinjaman dan Kasbon**: eligibilitas, plafon 50% THP, dan tenor 1–2. Alur approval **tidak** berubah: kasbon dua tahap, pinjaman satu tahap HR. Bunga flat pinjaman tetap hanya bisa diisi HR (TnC tidak menyebutnya).
- **R2** Eligibilitas memakai **daftar izin**: hanya status `contract` (Kontrak/PKWT), `pkwt`, dan `permanent` (Tetap/PKWTT) yang boleh mengajukan. Pesannya: probation → "Karyawan masa percobaan (probation) tidak dapat mengajukan pinjaman atau kasbon"; status lain (`intern`, `freelance`, `daily_worker`, kosong) → "Pinjaman dan kasbon hanya untuk karyawan PKWT atau PKWTT". Catatan: Pekerja Harian (`daily_worker`), yang sebelumnya boleh kasbon, kini tidak boleh.
- **R3** Plafon: **pokok pinjaman ≤ 50% THP** (THP 4 komponen, seperti KP-02). Pengaturan `loan_max_installment_percent` tidak lagi dipakai untuk pengajuan baru (nilainya tidak dihapus). Tanpa struktur gaji aktif → pesan yang ada ("belum punya struktur gaji aktif").
- **R4** Tenor hanya **1 atau 2** kali cicilan, untuk kedua jenis. Pinjaman lama dengan tenor > 2 yang sudah berjalan tidak diubah.
- **R5** Periode potong: tanggal **pengajuan** (`created_at`, WIB) menentukan periode payroll. Tanggal 1–20 → payroll bulan itu; tanggal 21–akhir bulan → payroll bulan berikutnya (periode 21–20, MOM PY-02). Batas tanggal 20 ditetapkan di kode sesuai TnC, tidak bergantung pada `cutoff_day` absensi (live 0).
- **R6** Saat disetujui, cicilan pertama = periode R5. Bila run payroll bulan itu sudah **`completed` atau `paid`** untuk entitas mana pun, cicilan pertama mundur ke bulan berikutnya, karena slip yang sudah selesai tidak bisa memotong lagi. Respons persetujuan menyebut bulan potong yang dipakai.
- **R7** Pengajuan (ESS dan HR) menampilkan TnC singkat dan **perkiraan bulan potong** ("Dipotong mulai payroll November 2026"). Isian tenor menjadi pilihan 1 atau 2 untuk kedua jenis.
- **R8** Acuan MOM diperbarui: KP-03 → Disepakati (TnC 01-11), KP-02 mengikuti daftar izin R2, PY-02 mencatat periode potong pinjaman.
- **R9** Tanpa migrasi data. Pengajuan pending yang sudah ada divalidasi ulang dengan aturan baru saat diputus (perilaku approve sekarang memang memvalidasi ulang).

## Tasks
Rencana: `docs/superpowers/plans/2026-11-01-tnc-pinjaman-kasbon.md`.

## Acceptance Criteria
- Karyawan probation (dan intern/freelance/harian) ditolak saat mengajukan pinjaman maupun kasbon. PKWT dan PKWTT bisa.
- Pokok > 50% THP atau tenor selain 1–2 ditolak untuk kedua jenis.
- Diajukan 20 Nov → cicilan pertama payroll November; diajukan 21 Nov → Desember. Bila run November sudah `completed`/`paid` saat disetujui → Desember.
- Form ESS/HR menampilkan TnC dan perkiraan bulan potong; tenor hanya 1/2.
- go test, pnpm test, lint, tsc hijau; CI 4/4; QA staging.

## Automation Log
- 2026-11-01: Epic dibuat dari TnC user. Pemetaan kode (`payroll/domain/kasbon.go`, `domain/loans.go`, `loans.go`, `service_runs.go`) dan acuan MOM (KP-02/03, PY-02). Belum dikerjakan (user: "sbg epic nanti").

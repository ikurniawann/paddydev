# EPIC-076: Syarat & Ketentuan Pinjaman dan Kasbon (`MODULE-HRIS`)

status: ready-for-qa
environment: dev
retries: 0

## Goal
Terms & conditions dari user 01-11-2026 untuk **dua jenis pinjaman: Kasbon (`kasbon`) dan Pinjaman Karyawan (`loan`)**. TnC berikut berlaku untuk keduanya (dikonfirmasi user 01-11):

> Masa probation tidak dapat mengajukan, PKWT dan PKWTT bisa mengajukan dengan catatan max. 50% dari THP dengan 1 s.d 2x cicilan dipotong dari Payroll. Pengajuan dalam masa periode payroll 21 s.d 20 yang akan terpotong di periode payroll bulan berjalan.

Ini adalah TnC yang ditunggu acuan MOM **KP-03 "Aturan Pinjaman"** (status "Menunggu TnC"). Kondisi kode per 01-11 (main `b97e10d3`):

| Aturan | Kasbon (`kasbon`) | Pinjaman Karyawan (`loan`) |
|---|---|---|
| Status yang boleh | Semua kecuali `probation`, `freelance`, `intern`, `internship` (daftar blokir, EPIC-060) | Semua status |
| Plafon | Pokok ≤ 50% THP (Gaji Pokok + Transport + Makan + Jabatan, struktur gaji aktif) | Cicilan/bulan ≤ `loan_max_installment_percent` (default 30%) dari **gaji pokok** |
| Tenor | 1 atau 2 | 1–60 bulan |
| Cicilan pertama | Bulan **sesudah persetujuan** (bulan kalender, `domain.FirstInstallmentPeriod`) | sama |
| Approval | Manager Departemen lalu HR Penggajian (AP-05) | HR satu tahap |

Payroll mengambil cicilan pada run bulan `M` bila `first_installment ≤ M` (`domain.IsLoanDue`), dihitung saat run diproses dan dilunasi saat run `paid`. Status run: `draft → processing → completed → paid`. Pengaturan cutoff absensi live masih `cutoff_day = 0` (PY-02).

## Prosedur 01-11 (gambar dari user) — menggantikan sebagian TnC di atas
> PROSEDUR PENGAJUAN KASBON DAN PINJAMAN KARYAWAN. Kasbon: karyawan yang sudah bekerja minimal 3 bulan berturut-turut; nominal maksimal 50% dari total THP, cicilan maksimal 2 kali. Pinjaman Karyawan: karyawan yang sudah bekerja minimal 1 tahun (12 bulan) berturut-turut; nominal maksimal 200% THP, cicilan maksimal 6 kali; wajib menyertakan surat berharga sebagai dokumen pendukung. Seluruh cicilan otomatis terpotong dari gaji setiap bulan.

Jawaban user 01-11: ikuti prosedur (plafon/tenor per jenis); atasan menyetujui Pinjaman Karyawan dulu (sama seperti kasbon), lalu HR; dokumen wajib untuk keduanya (kasbon: dokumen pendukung; pinjaman: jaminan).

## Ruling
| Aturan | Kasbon (`kasbon`) | Pinjaman Karyawan (`loan`) |
|---|---|---|
| Status | Hanya `contract`/`pkwt` (PKWT) dan `permanent` (PKWTT) | sama |
| Masa kerja (dari `join_date` s.d. hari ini WIB) | ≥ 3 bulan | ≥ 12 bulan |
| Plafon pokok | ≤ 50% THP | ≤ 200% THP |
| Cicilan | 1 atau 2 kali | 1 sampai 6 kali |
| Lampiran wajib | Dokumen pendukung 1–3 file | Jaminan (surat berharga) 1–3 file; dokumen pendukung opsional (0–3) |
| Approval | Manager atasan (rantai `reporting_to`) → HR Penggajian | Manager atasan → HR (approve Pinjaman) |
| Mulai potong | Payroll periode pengajuan (21–20) | sama |

- **R1** Kedua jenis memakai satu validator dengan aturan per jenis (tabel di atas). Urutan cek: status → masa kerja → cicilan → jumlah pinjaman aktif → struktur gaji → plafon.
- **R2** Daftar izin status: `contract`, `pkwt`, `permanent` (trim, case-insensitive). Pesan: probation → "Karyawan masa percobaan (probation) tidak dapat mengajukan kasbon atau pinjaman"; status lain (`intern`, `freelance`, `daily_worker`, kosong) → "Kasbon dan pinjaman hanya untuk karyawan PKWT atau PKWTT". Pekerja Harian yang dulu boleh kasbon kini ditolak.
- **R3** Masa kerja: bulan penuh sejak `join_date` (`join_date` + N bulan ≤ hari ini, WIB). Tanpa `join_date` → "Tanggal masuk karyawan belum diisi — hubungi HR". Pesan: "Kasbon hanya untuk karyawan dengan masa kerja minimal 3 bulan" / "Pinjaman Karyawan hanya untuk karyawan dengan masa kerja minimal 12 bulan".
- **R4** THP = 4 komponen struktur gaji aktif (Gaji Pokok + Transport + Makan + Jabatan, seperti KP-02). Plafon dari **pokok**. Pesan "Kasbon maksimal 50% THP" / "Pinjaman Karyawan maksimal 200% THP" (+ " (maks Rp…)" hanya untuk pemohon sendiri atau pemegang Penggajian, EPIC-057). `loan_max_installment_percent` tidak lagi dipakai (nilai tidak dihapus).
- **R5** Cicilan: pesan "Cicilan kasbon hanya 1 atau 2 kali" / "Cicilan Pinjaman Karyawan 1 sampai 6 kali". Input server tetap menerima 1–60 lalu divalidasi per jenis.
- **R6** Periode potong: tanggal **pengajuan** (`created_at`, WIB) 1–20 → payroll bulan itu; 21–akhir → bulan berikutnya. Batas 20 ditetapkan di kode. Saat HR menyetujui, bila run payroll bulan itu sudah lewat draft (`processing`/`completed`/`paid`, karena hanya run draft yang dihitung ulang), mundur ke bulan berikutnya. Pesan persetujuan menyebut bulannya ("Pinjaman disetujui — dipotong mulai payroll November 2026").
- **R7** Approval: approver Manager disimpan saat pengajuan untuk **kedua** jenis (`LoanApprover`, seperti kasbon EPIC-060). Tanpa Manager di rantai → langsung ke HR. HR hanya bisa menyetujui setelah Manager setuju (boleh menolak kapan saja). Kasbon tetap butuh Penggajian di sisi HR; Pinjaman Karyawan cukup approve Pinjaman.
- **R8** Lampiran: kolom baru `attachments text[]` (dokumen pendukung) dan `collateral_attachments text[]` (jaminan), maks 3 masing-masing. Unggah `POST /api/hris/loans/attachments` (multipart `file`, PDF/JPG/PNG/WebP ≤ 5 MB, folder `loan-attachments/<employeeId>`; HR pemegang akses penuh boleh mengunggah untuk karyawan lain lewat `employee_id`). Path divalidasi saat pengajuan: pola folder, pemilik = karyawan pemohon, file ada. Kasbon menolak `collateral_attachments`. Pesan: "Lampirkan minimal 1 dokumen pendukung kasbon" / "Lampirkan minimal 1 dokumen jaminan (surat berharga)".
- **R9** Baca lampiran `GET /api/hris/loans/attachment/{path…}`: pemohon, Manager approver tersimpan pada pinjaman yang memuat file itu, dan peran akses penuh pinjaman (HR/super admin).
- **R10** Detail: atasan (antrean ESS dan banner Persetujuan Tim), HR, dan super admin membuka detail berisi jenis, pokok, cicilan, perkiraan bulan potong, keperluan, dokumen pendukung, dan jaminan sebelum memutus.
- **R11** Form ESS dan HR: teks syarat per jenis, pilihan cicilan sesuai jenis (kasbon 1/2, pinjaman 1–6), perkiraan bulan potong, unggah lampiran wajib. Label jenis `loan` = "Pinjaman Karyawan" (form, tabel, slip).
- **R12** Migrasi hanya menambah dua kolom (default `{}`); data lama tidak diubah. Pengajuan pending lama divalidasi ulang dengan aturan baru saat HR memutus, **kecuali** syarat lampiran (hanya saat pengajuan). Pending lama tanpa approver Manager tetap langsung ke HR.
- **R13** Acuan MOM diperbarui: KP-02/KP-03 (Disepakati, prosedur 01-11), PY-02 (periode potong pinjaman), matriks approval Pinjaman Karyawan.

## Tasks
Rencana: `docs/superpowers/plans/2026-11-01-tnc-pinjaman-kasbon.md`.

## Acceptance Criteria
- Probation/intern/freelance/harian ditolak untuk kedua jenis; PKWT/PKWTT diterima bila memenuhi masa kerja.
- Kasbon: masa kerja < 3 bulan, pokok > 50% THP, cicilan ≠ 1/2, atau tanpa dokumen pendukung → ditolak.
- Pinjaman Karyawan: masa kerja < 12 bulan, pokok > 200% THP, cicilan > 6, atau tanpa jaminan → ditolak.
- Pinjaman Karyawan masuk antrean Manager atasan dulu, lalu HR.
- Atasan, HR, dan super admin melihat detail beserta lampiran; karyawan lain tidak bisa membuka file.
- Diajukan 20 Nov → potong payroll November; 21 Nov → Desember; run November sudah `completed`/`paid` saat disetujui → Desember.
- go test, pnpm test, lint, tsc hijau; CI 4/4; QA staging laptop + HP.

## Automation Log
- 2026-11-01: Epic dibuat dari TnC user. Pemetaan kode (`payroll/domain/kasbon.go`, `domain/loans.go`, `loans.go`, `service_runs.go`) dan acuan MOM (KP-02/03, PY-02). Belum dikerjakan (user: "sbg epic nanti").
- 2026-11-01: User mengonfirmasi ruling R1–R9 ("sesuai") dan menegaskan ada dua jenis, Kasbon dan Pinjaman Karyawan, keduanya mengikuti TnC ini. Status tetap `backlog` sampai dijadwalkan.
- 2026-11-01: User meminta dikerjakan sekarang, dengan prosedur baru (gambar): syarat masa kerja, plafon/tenor per jenis, jaminan untuk pinjaman, atasan melihat detail. Jawaban user: ikuti prosedur; atasan menyetujui Pinjaman Karyawan dulu; dokumen wajib untuk keduanya. Ruling diganti R1–R13 (menggantikan R1–R9 lama); status `coding`.
- 2026-11-01: **Implementasi** (eksekusi inline): `cace6bf3` backend (validator per jenis, approval Manager untuk kedua jenis, periode 21–20, lampiran + migrasi `20261101090000_loans_attachments.sql`), `14d20497` frontend (form ESS/HR, unggah dokumen/jaminan, detail lampiran untuk atasan & HR, label "Pinjaman Karyawan"), `37cdee00` acuan MOM.
- 2026-11-01: **Review akhir** (opus): Ready with fixes, 0 kritis. Diperbaiki di `4dca73c9`:
  - R6 diperluas: run `processing` juga dianggap tutup (hanya run draft yang dihitung ulang), supaya bulan potong & pesan tidak salah.
  - Dialog form ESS/HR bisa di-scroll di HP (`max-h-[90dvh]`).
  - Lampiran tidak basi saat unggah (ganti karyawan / hapus di tengah unggahan, unggahan sebagian gagal); dua bidang unggah berbagi status sibuk.
  - UUID karyawan dari HR di-lowercase; antrean Manager menyertakan cicilan/bulan.
  - Ditunda (minor): pengecekan run tutup belum per entitas (run selesai entitas mana pun memundurkan bulan potong); akun di luar `LoanManageRoles` yang membuka halaman HR Pinjaman tetap mengajukan untuk diri sendiri (perilaku lama).
- 2026-11-01: **Gerbang lokal:** `go vet ./...` bersih; `go test -p 1` app/payroll/hris/desktop ok; frontend `tsc` 0, `lint` 0, `pnpm test` 873 file / 6327 tes lulus (sebelum perbaikan review; suite terkait diulang sesudahnya, lulus).
- 2026-11-01 21:40: **Staging** image `local-20261101-214002` (branch `4dca73c9`), 1 migrasi. Backup `~/Desktop/paddydev-backups/staging-db-before-epic076-20261101-213954.dump`.
- 2026-11-01: **QA staging** (akun demo Manager, PKWT, Tetap, Probation, HR), 35/35 lulus:
  - kasbon via laptop dengan dokumen pendukung; Pinjaman Karyawan 200% THP × 6 via HP dengan jaminan;
  - probation ditolak, PKWT < 12 bulan tidak bisa pinjaman, lewat plafon ditolak;
  - Manager membuka detail + pratinjau lampiran (HP & laptop) lalu menyetujui; HR menyetujui → dipotong mulai payroll November 2026;
  - karyawan lain 403 saat membuka jaminan, HR 200;
  - tidak meluber di HP, 0 error konsol / API.
  - Data demo dan file lampiran dihapus.
- 2026-11-01: CI PR #60 sempat merah pada audit: advisory baru `follow-redirects` ≤1.16.1 (GHSA-mrwp-f3q8-3g9p), tidak terkait epic. Override dinaikkan ke `^1.16.2` (`d0de3af0`), lalu CI 4/4 hijau. **PR #60 squash-merged → main `1493bbd3`.** Staging = main (`local-20261101-220036`, backup `staging-db-before-main-e076-20261101-220035.dump`).
- 2026-11-01 22:11: **PRODUCTION** = main `1493bbd3`, image `local-20261101-221140`, atas izin user ("Ya, deploy prod").
  - Migrasi `20261101090000_loans_attachments.sql` diterapkan (total 567).
  - Backup `~/backups/prod-db-before-main-e076-20261101-221019.dump` (VPS). Source lama `~/paddydev-src.before-main-e076-20261101-221019` (= `589ac8c8`).
  - Verifikasi: container healthy, 0 error API, kolom lampiran ada, rute halaman 307 (login) dan API 401 tanpa sesi.
  - Saat rilis ada 6 pengajuan pinjaman pending. Saat HR memutusnya, aturan baru berlaku (R12), jadi yang tidak memenuhi syarat ditolak dengan pesan syarat.
  - Status `ready-for-qa` menunggu QA HR.

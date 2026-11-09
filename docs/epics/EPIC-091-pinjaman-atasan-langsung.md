# EPIC-091: Penyetuju Minimal SPV untuk Semua Pengajuan + Kasbon/Pinjaman Tiga Tahap (`MODULE-HRIS`, `MODULE-PAYROLL`)

status: done
environment: dev
retries: 0

## Goal
Laporan user 05-11-2026: karyawan tidak bisa mengajukan pinjaman, atau pinjamannya tidak muncul di atasannya (Yosianti, Didit, Firdaus), padahal HR sudah mengatur atasan mereka.

### Temuan (production, read-only atas izin user 05-11)
- **Approver tahap 1 bukan atasan langsung.** Kasbon/pinjaman memakai "Manager Departemen" (EPIC-060 Q-APV-2, EPIC-076 R7): orang pertama berlevel Manager ke atas yang punya akun di rantai `reporting_to`.
  - Atasan langsung yang Supervisor dilewati, sehingga pengajuan tidak muncul di atasan langsung.
  - Yosianti → Regha (atasan langsung Alinda, SCM Supervisor). Firdaus Eka → Dimas (atasan langsung Asriani, Store Supervisor).
  - 244 karyawan aktif punya atasan langsung berlevel di bawah Manager.
- **Loan tidak mengirim notifikasi apa pun** saat diajukan. Manager hanya melihat angka badge.
- **Didit**: berstatus Probation sampai 29-10 13:21, jadi ditolak sesuai aturan. Sekarang Kontrak (masuk 21-06-2026): kasbon bisa; Pinjaman Karyawan butuh masa kerja 12 bulan.
- **Firdaus Nuzula**: sudah resign.

### Keputusan user 05-11
- Keputusan awal: approver tetap Manager, SPV ikut melihat. **Diralat user** ("Approver minimal atasan dengan jabatan SPV untuk semua pengajuan bisa menyetujui"):
  - **semua pengajuan** (cuti/izin, lembur, perjalanan dinas, kasbon/pinjaman): penyetuju = **atasan terdekat di rantai `reporting_to` yang berjabatan minimal Supervisor**. Atasan langsung berlevel di bawah SPV (Staff, mis. Store Leader) dilewati;
  - **kasbon/pinjaman: SPV → Manager (bila ada) → HR**.
- Probation tetap tidak boleh mengajukan.

## Ruling
- **R0 Penyetuju minimal SPV.** Fungsi `hris.request_approver(karyawan)` (delta `20261110090000_request_approver_spv.sql`) mengembalikan atasan terdekat yang memenuhi semua syarat berikut:
  - aktif dan punya akun login;
  - level jabatan `supervisor`, `assistant manager`, `manager`, `senior manager`, `general manager`, `director`, atau `c-level`, **atau level kosong** (tidak terbukti di bawah SPV);
  - maksimal 10 tingkat; siklus dan atasan = dirinya berhenti; tidak ada → HR saja.

  Dipakai untuk:
  - antrean dan badge Persetujuan Tim, keputusan cuti, lembur (termasuk koreksi), dan perjalanan dinas;
  - baca lampiran, lonceng/WA pengajuan;
  - Riwayat tim.

  `approver_id` menggantikan `reporting_to_active` di data lembur HR (tombol Koreksi). Absensi Tim tetap memakai subtree `reporting_to`.
- **R0c Dua atasan (ralat user 05-11, "route tetap bisa 2 atasan").** Cuti/izin, lembur, dan perjalanan dinas boleh diputus **SPV terdekat** (`request_approver`, mis. Farhan) **atau Manager di atasnya** (`hris.request_manager`: atasan terdekat berlevel Manager ke atas, aktif, berakun, mis. Dimas). Cukup salah satu; yang terlambat mendapat "sudah disetujui/ditolak".
  - Implementasi: delta `20261110090100_request_manager.sql` (`request_manager`, `is_request_approver`), dipakai di antrean/badge, baca lampiran, Riwayat, dan keputusan (`approverFor`).
  - Lonceng/WA pengajuan tetap ke SPV saja agar Manager yang membawahi banyak toko tidak dibanjiri; Manager melihat antrean dan badge.
- **R0b Kasbon/pinjaman tiga tahap.**
  - **Penyetuju** (`loanStages`): tahap SPV = `request_approver`, dan tahap Manager = `LoanApprover` (≥ Manager) bila orangnya berbeda. Bila keduanya sama, atau salah satunya tidak ada, hanya satu tahap atasan.
  - **Kolom** `supervisor_approver_id/approved_by/approved_at`. Status tetap `pending` sampai Manager setuju (`manager_approved`), lalu HR.
  - **SPV menyetujui**: "{Jenis} disetujui SPV — menunggu persetujuan Manager", dan Manager dilonceng. **SPV menolak**: selesai.
  - **Giliran**: yang belum gilirannya → 403 "Bukan giliran Anda memutus pengajuan ini".
  - **Pengajuan lama** yang pending tanpa tahap SPV tetap di Manager.
- **R1 Notifikasi** (dalam transaksi pengajuan, `notify_user` type `approval`), judul "Pengajuan {Kasbon|Pinjaman Karyawan}: {nama}":
  - **Penyetuju tahap pertama** (SPV, atau Manager bila tanpa tahap SPV): "{nama} mengajukan {jenis} Rp… ({n}x cicilan). Menunggu persetujuan Anda di Area Karyawan → Pinjaman." Tautan `/dashboard/me/pinjaman`. Manager dilonceng setelah SPV setuju.
  - **Atasan langsung** yang bukan penyetuju (mis. Store Leader berlevel Staff): "… Diputuskan oleh {SPV}, lalu {Manager}, lalu HR; lihat detail di Persetujuan Tim → Riwayat." Tautan `/dashboard/me/persetujuan-tim?tab=riwayat`.
- **R2 Baca lampiran.** Selain pemohon, SPV dan Manager penyetuju, dan HR, **atasan langsung pemohon** boleh membuka dokumen pendukung dan jaminan.
- **R3 Riwayat.** `GET /api/hris/me/team-history` menambah `loans` lewat port payroll `TeamLoanHistory`:
  - kasbon/pinjaman bawahan langsung yang diajukan sejak awal periode lalu, ditambah yang masih menunggu (pending/manager_approved) berapa pun umurnya;
  - maksimal 300, terbaru dulu.

  Tab Riwayat menampilkan bagian "Kasbon & Pinjaman" dengan lencana:
  - Menunggu SPV {nama} / Menunggu Manager {nama} / Menunggu HR / Disetujui Manager — menunggu HR;
  - Berjalan / Lunas / Ditolak.

  Detail baca-saja: jenis, pokok, cicilan, keperluan, dokumen, jaminan, alasan penolakan. `?tab=riwayat` membuka tab Riwayat.
- **Di luar cakupan**:
  - membuka probation;
  - data HR (status/level jabatan) tetap urusan HR.

## Acceptance Criteria
- [x] Semua pengajuan diputus atasan terdekat minimal SPV; atasan langsung berlevel Staff tidak memutus.
- [x] Kasbon/pinjaman SPV → Manager (bila ada) → HR; lonceng per tahap tanpa duplikasi; atasan langsung non-penyetuju dilonceng untuk melihat detail.
- [x] Atasan langsung dan penyetuju melihat kasbon/pinjaman tim di Riwayat beserta lampirannya.
- [x] Gates hijau; QA staging (HP + desktop).

## Automation Log
- 2026-11-05 — Investigasi laporan (staging + production read-only atas izin) dan 3 jawaban user. Worktree `.worktrees/epic091`, branch `feat/epic091-pinjaman-atasan`, ditumpuk di atas EPIC-090.
- 2026-11-05 — Implementasi.
  - Lonceng + Riwayat kasbon (`52a7b9f3`, `1c2035a5`). Setelah ralat user: `hris.request_approver` untuk semua pengajuan (`ac1d88d0`), tiga tahap kasbon (`7c8ba419`), status SPV di Riwayat (`986169b8`), Riwayat untuk atasan langsung/penyetuju tersimpan (`42a7c7db`).
  - Gates: `go test -p 1` hris + payroll + app ok, vet + gofmt bersih, vitest 4594 lulus, tsc + eslint bersih.
- 2026-11-05 — Review akhir (reviewer terpisah): 0 Critical, 4 Important, diperbaiki dengan tes (`a3e2dfbd`):
  - lonceng Manager "(0x cicilan)";
  - dokumen 403 bagi penyetuju saat ini pada pengajuan lama;
  - tahap SPV dilewati bila SPV tersimpan nonaktif/tanpa akun;
  - HR yang juga penyetuju tersimpan tetap bisa bertindak sebagai HR;
  - laporan "tanpa atasan" mengikuti `request_approver` (alasan baru: `no_account`, `below_spv`).

  Ditunda:
  - fallback Next masih `reporting_to`;
  - lonceng ke atasan langsung nonaktif;
  - performa fungsi per baris (EXPLAIN di staging);
  - `?tab=` hanya saat halaman dibuka;
  - teks penolakan "Hanya atasan langsung…" dan "Manager Departemen";
  - "SPV SPV Toko" ganda;
  - tes HR di tahap SPV untuk approve.
- 2026-11-05 — **Staging** `local-20261105-162804` (gabungan lokal EPIC-087 + EPIC-091, yang memuat #76–#78; merge `10254196` tidak di-push).
  - Backup `staging-db-before-epic091b-20261105-162803.dump`. Migrasi `20261110090000` ok, verify 10780 objek, 0 berbeda.
  - Data staging: 4 karyawan berganti penyetuju (atasan langsung berlevel Staff), 2 karyawan tanpa penyetuju (ke HR).
  - QA 18/18 lulus (karyawan → Leader Staff → SPV → Manager; HP + desktop; sebelumnya keliru ditulis 19/19):
    - cuti: lonceng ke SPV, bukan Leader; Leader 403; SPV menyetujui dari Persetujuan Tim;
    - kasbon: tahap SPV lalu Manager; Manager 403 sebelum SPV;
    - lonceng Manager setelah SPV; status `manager_approved`;
    - Leader melihat Riwayat "Menunggu SPV" + dokumen;
    - 0 error API dan console.
  - Data demo dan berkas dihapus.
- 2026-11-05 — Cek ulang setelah instruksi lanjutan user.
  - Gates di HEAD `4336220b` hijau: gofmt bersih, `go vet`, `go test -count=1 -p 1` hris + payroll + app ok, vitest 4594, tsc + eslint.
  - Penyetuju hasil aturan baru (production, read-only, logika `request_approver` dihitung inline karena fungsi belum ada di prod; staging pakai fungsi): Yosianti → **Alinda (Supervisor)**, Didit → **Farhan (Supervisor)**, Firdaus Eka → **Asriani (Supervisor)**. Sama dengan atasan langsung masing-masing; user menegaskan "minimal Jabatan SPV/supervisor bisa menyetujui karyawan atasannya langsung".
  - Pengajuan pending lama: kasbon Firdaus Eka (28-10) di Dimas, pinjaman Yosianti (04-11) di Regha. Keduanya aktif dan berakun, jadi **tidak menggantung**; total 5 pinjaman pending di prod.

    Setelah rilis, pengajuan ini tetap di Manager tersimpan (tanpa tahap SPV, R0b). Bila user ingin SPV ikut memutus pengajuan lama, perlu skrip ops terpisah yang mengisi `supervisor_approver_id` untuk pinjaman pending; skrip itu belum dibuat dan butuh izin.
- 2026-11-05 — Keputusan user: pinjaman pending lama **tetap di Manager tersimpan** (tanpa skrip pengisian tahap SPV). Hanya pengajuan baru setelah rilis yang mengikuti SPV → Manager → HR.
- 2026-11-05 — Ralat user "route tetap bisa 2 atasan… jabatan seperti Farhan bisa menyetujui semua pengajuan" → R0c.
  - TDD `TestRequestApproverMinimumSupervisor` diperluas RED→GREEN:
    - Manager di atas SPV melihat antrean dan memutus cuti; SPV yang telat ditolak;
    - Manager memutus lembur dan perjalanan dinas;
    - Leader Staff 403;
    - Riwayat untuk SPV dan Manager.
  - Gates: Go hris + payroll + app ok, vitest 4594, tsc + eslint.
- 2026-11-05 — Review R0c (reviewer terpisah): 0 Critical, 0 Important.
  - **Minor 1** dinaikkan karena berdampak: penolakan atasan kedua bisa mendarat di cuti berlampiran yang baru saja disetujui atasan lain. Diberi guard SQL: hanya penolakan HR yang boleh mendarat saat menunggu HR. Tes `TestLeaveRejectRaceAfterOtherApproverApproved` RED→GREEN.
  - **Kinerja**: `SubordinateIDs` untuk Manager ±300 bawahan butuh ±0,9 dtk per buka lampiran / daftar lembur-dinas. Diganti `hris.request_team` (delta `20261110090200`), satu penelusuran atas-ke-bawah, 5,6 ms.
    - Diverifikasi setara `is_request_approver` di data staging: 25 atasan, 934 pasangan, selisih 0 dua arah.
    - Antrean (`teamMember`) ikut memakai `request_team`.
  - Ditunda:
    - `manager_id` ikut dihitung di daftar HR lembur/dinas;
    - Manager berlevel kosong dilewati oleh `request_manager` (data HR);
    - lonceng SPV basi bila Manager memutus duluan;
    - tes GET detail/lampiran oleh Manager;
    - komentar "dua tahap" di halaman Persetujuan Tim.
- 2026-11-05 — QA staging setelah R0c (`local-20261105-165917`): 2 atasan 10/10 lulus.
  - Manager melihat dan menyetujui cuti dari Persetujuan Tim, lalu cuti hilang dari antrean SPV.
  - SPV menyetujui lembur; Leader Staff tidak punya antrean.
  - Regresi kasbon/penyetuju 18/18 lulus. Staging: Yosianti → Alinda/Regha, Didit → Farhan/Dimas, Firdaus Eka → Asriani/Dimas.
- 2026-11-05 — **Staging final** `local-20261105-170736` (merge lokal `2d1808a1`; backup `staging-db-before-epic091d-20261105-170728.dump`).
  - Migrasi `20261110090200` ok, verify 10783 objek, 0 berbeda.
  - QA ulang di data bersih: kasbon/penyetuju 18/18 dan dua atasan 10/10, 0 error API dan console. Satu kali cek gagal karena timing toast di run pertama, lulus saat diulang.
  - Data demo qa091 dan berkasnya dihapus.

- 2026-11-09 13.18 WIB: **LIVE production** lewat rilis gabungan PR #86 (main `bd9ff4db`, image `local-20261109-130944`; 10 migrasi EPIC-087..095, verify 0 berbeda). Backup `~/backups/prod-db-before-rilis-20261109-125740.dump` (VPS + lokal); rollback image `local-20261103-150738`, source lama `~/paddydev-src.before-rilis-20261109-130930`. Verifikasi read-only: `/login` dan `/api/ready` 200, container sehat, log API bersih. Sisa: QA HR di production.

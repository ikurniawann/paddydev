# EPIC-094: Keterlambatan Tanpa Potongan + Peringatan Lonceng + Surat Teguran/SP oleh HR (`MODULE-HRIS`, `MODULE-PAYROLL`)

status: review
environment: dev
retries: 0

## Goal
Permintaan user (06-11-2026):

> lanjut untuk ess tampilan beranda karyawan dan absensi untuk label SP1, SP2 dan SP3 di tampilan Karyawan di hilangkan
> Lalu untuk payroll ada Kebijakan paddy tidak ada potongan keterlambatan absensi, maka dari diputuskan akan muncul tindakan disipliner berjenjang (Surat teguran, SP1, SP2, SP3). Potongan absensi hanya berlaku untuk unpaid leave (sakit, izin pribadi, yang sudah di validasi sebelumnya cek jika tidak ada) yang terlampir di file excel yang sudah dikirim untuk komponen payslip line.
> pastikan Muncul di notif yang gambar lonceng aja mas di ESS masing2 approver
> Pastikan SLA batas pengajuan: Cuti H-3, Cuti Ibadah H-30, Sakit hari H.

Keputusan user setelah ditanya:
- **ESS karyawan:** yang hilang hanya label tingkat (badge Teguran/SP, tangga, petunjuk "x kali lagi → SP", teks ketentuan SP). Jumlah terlambat, total menit, dan daftar hari tetap tampil. Atasan di Absensi Tim tetap melihat tingkat.
- **Surat Teguran/SP:** tidak otomatis. Surat muncul hanya bila **HR menerbitkannya** di HRIS (jenis, tanggal, catatan, lampiran PDF opsional). Saat terbit, lonceng masuk ke karyawan, penyetujunya, dan HR. Riwayat surat ada di profil karyawan.
- **Peringatan otomatis:** setiap clock-in tercatat terlambat, lonceng masuk ke **karyawan** ("Anda terlambat … Hati-hati, jika berlanjut akan ada teguran"), **penyetuju** (atasan minimal SPV + Manager di atasnya), dan **HR**. Hanya lonceng, tanpa WhatsApp.
- **Payroll:** potongan keterlambatan dimatikan, dan baris "Potongan Keterlambatan" tidak tampil bila nol. Potongan absensi hanya Izin Sakit dan Izin Pribadi (cuti yang sudah disetujui), sesuai tabel payslip 20-10-2026.

Kondisi kode dan data (06-11):
- **ESS Absensi (`MyLatenessCard`).** Tampil badge tingkat, tangga 5 tingkat, petunjuk tingkat berikutnya, dan teks ketentuan SP (EPIC-077 R8).
- **Beranda (`LatenessBerandaCard`).** Tampil badge dan petunjuk (R9).
- **Notifikasi keterlambatan.** Hanya ke HR: trigger `public.fn_notify_attendance_changes` sudah mengirim "⏰ Karyawan Terlambat" ke akun HR untuk setiap clock-in terlambat. Karyawan dan atasan belum mendapat apa pun. (Koreksi dari review akhir; semula ditulis "belum ada sama sekali".)
- **Payroll production.** `late_deduction_mode = tier` (4–6 = 1 hari THP, 7–9 = 2 hari pokok, 10+ = 3 hari pokok). Satu-satunya run (Nov 2026) masih draft tanpa detail, jadi belum ada slip yang terpotong. Staging juga `tier`.
- **Tabel payslip acuan.** Tidak punya baris potongan telat (`docs/hris/acuan-mom-20261019/aturan-payroll.md`).
- **Potongan Izin Sakit dan Izin Pribadi sudah ada.**
  - `sick_leave_deduction` dan `personal_leave_deduction` on di production.
  - `LeaveDayKind`, `domain/period.go`.
  - Hanya cuti berstatus approved yang dihitung.
- **SLA sudah ditegakkan server untuk pengajuan sendiri** (`domain.LeaveTimingError`, EPIC-084):
  - Cuti Tahunan, Menikah, Menikahkan Anak, Khitanan/Baptis, Ulang Tahun, dan Melahirkan: H-3.
  - Haji/Umrah: H-30.
  - Sakit: hari H.
- **Volume production.** ±490 baris terlambat per 30 hari, sekitar 16 per hari. 1 akun HR aktif (`role = hrd`).

## Ruling
- **R1 — Label tingkat hanya disembunyikan di UI karyawan.**
  - Endpoint `GET /api/hris/me/lateness` tidak diubah, karena Absensi Tim memakai tipe yang sama.
  - Bila salah: karyawan yang membuka JSON melihat kode tingkat. Biaya perbaikan: satu field di respons.
- **R2 — Pengganti teks ketentuan SP di ESS:** "Hati-hati, keterlambatan yang berlanjut dapat berujung teguran dari HR."
- **R3 — Peringatan otomatis hanya dari clock-in.**
  - Koreksi absensi oleh HR tidak memicu lonceng.
  - Pada hari dua shift, keterlambatan hanya dinilai dari shift pertama (sesuai EPIC-080), jadi paling banyak satu lonceng per baris absensi.
- **R4 — Gagal kirim lonceng tidak menggagalkan clock-in.** Galat dicatat di log.
- **R5 — Penerima peringatan dan surat:**
  - karyawan sendiri;
  - `hris.request_approver` dan `hris.request_manager` milik karyawan (tanpa duplikat, tidak termasuk dirinya sendiri);
  - semua akun `role = hrd` (`public.notify_hrd`). Untuk peringatan terlambat, HR memakai lonceng trigger yang sudah ada ("⏰ Karyawan Terlambat"), jadi tidak dikirim dua kali. Untuk surat, HR mendapat lonceng baru.
  - Jenis notifikasi `alert`.
- **R6 — Hitungan "x kali di periode ini"** memakai periode absensi yang sama dengan kartu keterlambatan (`periodContaining`), termasuk baris yang baru dicatat.
- **R7 — Potongan telat dimatikan lewat migrasi data** `late_deduction_mode = 'off'`.
  - Alasan: deploy production sudah butuh izin, dan keputusan sudah dari user.
  - Pengaturan tetap bisa diubah HR di halaman Pengaturan Payroll.
  - Bila salah: HR menyalakan kembali di pengaturan.
- **R8 — Baris "Potongan Keterlambatan"** disembunyikan bila nol di halaman slip HR. PDF Go, PDF Next, dan dialog slip ESS sudah menyembunyikan baris nol.
- **R9 — Surat:** tabel baru `hris.disciplinary_letters`.
  - Kolom: jenis `teguran | sp1 | sp2 | sp3`, tanggal terbit, catatan, lampiran PDF opsional (privat, maks. 5 MB).
  - Label: "Surat Teguran", "Surat Peringatan 1", "Surat Peringatan 2", "Surat Peringatan 3".
  - Surat yang salah **dibatalkan** dengan alasan wajib, bukan dihapus. Baris tetap ada, ikut diaudit, dan tidak tampil lagi di ESS karyawan.
- **R10 — Menu HR baru:** **Kehadiran & Cuti → Surat Peringatan** (`hris.workforce.disciplinary-letters`, `/dashboard/hris/surat-peringatan`).
  - Aksi: `read`, `create` (terbitkan), `update` (batalkan).
  - Diberikan ke admin_hris, lalu diturunkan ke role HR lewat sync, tanpa masker.
  - Profil karyawan HR mendapat tab **Surat Peringatan** dengan daftar dan tombol terbitkan yang sama.
- **R11 — Bantuan untuk HR:** dialog terbitkan menampilkan "Terlambat N kali di periode ini (ketentuan: Surat Peringatan 1)". Ini hanya informasi; HR tetap memilih jenis sendiri.
- **R12 — ESS karyawan:**
  - Halaman Absensi mendapat bagian **Surat dari HR** (`#surat`) berisi surat yang belum dibatalkan dan tautan lampirannya. Bagian ini hanya muncul bila ada surat.
  - Lonceng surat untuk karyawan menaut ke bagian itu.
  - Penyetuju ditautkan ke Absensi Tim, HR ke halaman Surat Peringatan.
- **R13 — Lampiran surat** dibuka oleh HR (menu `read`), karyawan itu sendiri, serta penyetuju dan manajernya.
- **R14 — SLA** sudah sesuai permintaan. Tidak ada perubahan kode, hanya tes regresi katalog dan uji di QA staging.
- **R15 — Base branch** `feat/epic092-approval-berjenjang`, karena `request_approver`/`request_manager` ada di tumpukan EPIC-091/092. Deploy production menunggu tumpukan #75–#80.

## Tasks
Lihat rencana `docs/superpowers/plans/2026-11-06-disiplin-keterlambatan.md`.

## Acceptance Criteria
- [x] ESS Beranda dan Absensi karyawan tidak menampilkan Teguran/SP (badge, tangga, petunjuk, teks ketentuan). Jumlah, menit, dan hari terlambat tetap tampil. Absensi Tim atasan tidak berubah.
- [x] Clock-in terlambat → lonceng ke karyawan ("Anda terlambat … Hati-hati …"), penyetuju + Manager, dan HR. Clock-in tepat waktu tidak mengirim lonceng.
- [x] HR menerbitkan Surat Teguran/SP1/SP2/SP3 (dengan atau tanpa PDF) dari menu Surat Peringatan atau profil karyawan → lonceng ke karyawan, penyetuju, dan HR. Karyawan melihat surat di ESS Absensi.
- [x] HR membatalkan surat dengan alasan → surat hilang dari ESS karyawan dan tercatat di audit.
- [x] Payroll tidak lagi memotong keterlambatan. Baris "Potongan Keterlambatan" tidak tampil bila nol. Potongan Izin Sakit dan Izin Pribadi tetap.
- [x] SLA: Cuti Tahunan < H-3 ditolak, Haji/Umrah < H-30 ditolak, Sakit selain hari H ditolak.
- [ ] `go test`, `pnpm test`, `pnpm lint`, dan `tsc` hijau; CI hijau; QA staging lulus.

## Automation Log
- 2026-11-06: **Permintaan user** (4 butir) + dua putaran tanya-jawab (keputusan di Goal).
  - Kode dipetakan; data production dibaca read-only (setting telat `tier`, 1 run draft tanpa detail, ±490 terlambat per 30 hari, 1 akun HR).
  - Rencana ditulis; status `backlog`, menunggu review user.
- 2026-11-06: **User menyetujui rencana** ("eksekusi aja dengan native"). Implementasi inline di branch `feat/epic094-disiplin-keterlambatan` (base `e09fee9e` = EPIC-092):
  - **T1** `da400268`: `domain.LateNotice`, `NotifyRequestApprovers` (request_approver ∪ request_manager, tanpa duplikat), `notifyLate` sesudah `ClockIn`.
  - **T2** `e6dff2ae`: `LatenessSummary showLevel={false}` di Beranda dan Absensi karyawan. Tangga dan blok tingkat SP di ketentuan dihapus; judul jadi "Jam Kerja & Ketentuan Absensi"; pengingat R2. Kartu tim tidak berubah.
  - **T3** `6158fefa`: migrasi `20261112090100_late_deduction_off.sql`. Halaman, PDF, dan dialog slip sudah menyembunyikan baris telat nol (tes regresi ditambah).
  - **T4** `bb8d4f39` + `c5dbccda`: tabel `hris.disciplinary_letters` + menu `hris.workforce.disciplinary-letters` (migrasi `20261112090000`), API terbit/batal/daftar/lampiran/hint keterlambatan/ESS, audit `disciplinary_letter.issue|revoke`, rute hanya di Go.
  - **T5** `10b104bc`: halaman HR `/dashboard/hris/surat-peringatan`, dialog terbitkan (hint keterlambatan, unggah lampiran), tabel dengan batal beralasan, tab profil "Surat Peringatan".
  - **T6** `51ca7bf0`: bagian ESS "Surat dari HR" (`#surat`), `TestLeaveSLACatalog` (regresi; langsung hijau sesuai R14).
  - Ruling implementasi:
    - `LatenessPolicy` tetap (jam kerja + langkah absensi), hanya blok SP dihapus; `lateness-ladder.tsx` dihapus.
    - HR yang clock-in atas nama karyawan pada hari itu juga memicu lonceng.
    - Tab profil menampilkan tombol terbit/batal untuk pemegang menu, dan server menegakkan aksinya.
    - Pemilih karyawan memakai pencarian penempatan.
    - Lampiran menerima PDF/JPG/PNG/WebP.
    - Migrasi diterapkan ke DB uji lewat psql karena ledger DB uji belum mencatat EPIC-088..092.
- 2026-11-06: **Review akhir seluruh branch** (reviewer segar): Critical 0, Important 2, satu Minor dinaikkan. Diperbaiki dalam satu gelombang (`4e69bbf1`):
  - HR menerima dua lonceng per keterlambatan (trigger lama + lonceng baru). Lonceng HR baru dihapus. Tes menghitung semua lonceng HR = 1 (RED 2 → GREEN 1).
  - Clock-in HR untuk tanggal lampau mengirim lonceng "hari ini". Kini hanya untuk tanggal hari ini (WIB). `TestHRBackfillClockInNoNotice` RED 3 → GREEN 0.
  - Tautan lonceng `#surat` tidak menggulir karena bagian dirender sesudah data. Kini menggulir sekali sesudah data tampil (tes RED → GREEN).
- **Minor ditunda:**
  - pembatalan surat tidak mengirim lonceng ke karyawan;
  - lampiran yang dibatalkan masih bisa terbuka dari cache browser hingga 1 jam (`max-age=3600`);
  - unggah lampiran sambil mengganti karyawan menampilkan galat "unggah ulang";
  - hint keterlambatan selalu periode berjalan, termasuk untuk surat bertanggal mundur;
  - penyetuju belum punya tampilan untuk membuka lampiran surat (akses API ada);
  - daftar HR dibatasi 500 baris tanpa paging;
  - berkas unggahan yang batal dipakai tidak dibersihkan;
  - cabang `showLevel=true` di `LatenessSummary` kini tanpa pemanggil;
  - `/me/lateness` masih mengirim `level`/`next` di JSON (R1).

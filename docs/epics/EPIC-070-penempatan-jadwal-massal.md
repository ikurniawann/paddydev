# EPIC-070: Penempatan & Jadwal Shift — Massal, Manual, Lebih Jelas (`MODULE-HRIS`)

status: deploying-dev
environment: dev
retries: 0

## Goal
Revisi user HR (29-10-2026): halaman **Penempatan Karyawan** dan **Jadwal Kerja** membingungkan, tidak ada filter, halaman, atau keterangan yang jelas, dan semuanya harus dikerjakan **satu per satu**. Halaman **Shift Kerja** juga membingungkan saat diubah, dan perubahannya lambat terlihat di halaman lain. Targetnya: HR bisa menempatkan dan menjadwalkan banyak karyawan sekaligus, dan tetap bisa mengubah satu per satu dengan mudah.

1. **Penempatan Karyawan:**
   - chip "yang perlu diurus" (Belum ditempatkan, Belum ada jadwal, Multi-toko);
   - filter cari, toko, departemen, jabatan;
   - pembagian halaman dari server (25/50/100);
   - kolom yang informatif: toko utama + jumlah toko lain, tanggal mulai, status jadwal;
   - bilah aksi massal: Pindahkan ke toko, Tambah toko, Akhiri, Atur jadwal;
   - pratinjau sebelum simpan;
   - pilih semua hasil filter.
2. **Jadwal Kerja:**
   - kotak karyawan × hari yang bisa diedit langsung;
   - klik satu kotak untuk ubah satu hari;
   - pilih banyak kotak lalu Isi atau Kosongkan;
   - centang karyawan lalu Terapkan pola mingguan untuk rentang tanggal;
   - Salin minggu lalu;
   - navigasi per minggu;
   - pratinjau + peringatan dengan aturan yang sama seperti unggah Excel;
   - unggah Excel tetap ada.
3. **Shift Kerja:**
   - daftar ringkas dikelompokkan per awalan kode, bisa dicari, menampilkan jumlah karyawan yang memakai;
   - form dengan contoh hasil;
   - nonaktifkan shift yang dipakai.
4. **Perubahan langsung terlihat** di semua halaman terkait (invalidasi cache lintas halaman).

## Keputusan user (29-10-2026)
- Prioritas: Penempatan massal, lalu Jadwal massal, lalu rapikan Shift Kerja.
- "lakukan epic nya jangan berhenti … jangan tanyakan lg lakukan sampai semua benar merged dan deploy": tidak ada tanya jawab lagi; izin merge + deploy staging lalu production sesudah QA lulus.

## Ruling (tanpa tanya)
- **R1:** Pola tim sama vs bergiliran tidak ditanyakan. Keduanya didukung: pola mingguan diterapkan ke banyak karyawan, plus ubah per kotak.
- **R2:** Semua penulisan jadwal dari UI baru menjadi baris per tanggal (`hris.employee_shift_days`), sama dengan unggah Excel. Validasinya juga sama (tanggal lewat, periode closing, tidak ditempatkan, kode tidak dikenal/nonaktif, Non-Shift ditolak; peringatan cuti, jeda <minShiftGap, >6 hari berturut, libur nasional). Pola mingguan di profil karyawan tetap ada untuk ESS Shift Tim.
- **R3:** Menimpa atau menghapus jadwal per tanggal yang sudah ada tetap butuh hak hapus Jadwal Kerja + alasan (EPIC-061 R4). UI meminta alasan hanya bila ada yang tertimpa.
- **R4:** Pindah massal = semua penempatan berjalan yang mulai sebelum tanggal pindah diakhiri sehari sebelumnya, lalu penempatan utama baru dibuat. Karyawan dengan penempatan yang mulai pada/sesudah tanggal pindah, atau yang sudah di toko tujuan, dilewati dengan alasan. Satu transaksi untuk semua karyawan, dan setiap karyawan diaudit.
- **R5:** "Belum ada jadwal" = karyawan aktif bertipe Shift tanpa baris jadwal (per tanggal atau pola mingguan) yang berlaku pada hari ini s/d +6 hari.
- **R6:** Endpoint lama tetap kompatibel. Halaman baru memakai endpoint/parameter baru.
- **R7:**
  - Chip "Belum ada jadwal" ikut menghitung karyawan Shift yang belum ditempatkan (R5 apa adanya).
  - Di tabel, karyawan itu tampil "Belum ditempatkan" supaya HR menempatkan dulu.
  - Karyawan Non-Shift tampil "Non-Shift", bukan status jadwal.
- **R8:** "Atur jadwal" dari Penempatan memakai toko utama terbanyak di antara karyawan terpilih yang diketahui. Bila tidak ada, dipakai filter toko. Bila itu pun kosong, halaman Jadwal Kerja meminta HR memilih toko. Paling banyak 50 karyawan dibawa lewat tautan.
- **R9:**
  - Jadwal Kerja punya tampilan 1, 2, dan 4 minggu (4 minggu supaya hasil pola sebulan terlihat).
  - Template dan unggah Excel memakai rentang mulai hari ini, minimal 7 hari.
  - "Salin minggu lalu" hanya mengisi minggu pertama yang tampil dan tidak pernah mengosongkan kotak.
- **R10:**
  - Simpan per kotak dibatasi 300 karyawan dan 6000 kotak dalam rentang ≤62 hari.
  - Kesalahan tingkat tanggal (lewat/closing) ditampilkan sebagai daftar umum.
  - Kotak yang dikosongkan berlabel "Ikut pola mingguan".
  - Simpan tanpa perubahan tidak dikirim ("Tidak ada perubahan").
- **R11:**
  - Shift yang masih dipakai tidak punya tombol Nonaktifkan/Hapus, karena server menolaknya selama ada jadwal mulai hari ini. Sebagai gantinya tampil petunjuk "Ganti dulu jadwal karyawan yang memakainya".
  - Angka "Dipakai" memakai aturan yang sama dengan penolakan server, termasuk karyawan nonaktif yang masih punya jadwal mendatang.
- **R12:** Penempatan massal dengan tanggal sebelum hari ini tetap boleh, tetapi diberi peringatan karena riwayat dan pembagian biaya gaji ikut berubah.

## Tasks
Lihat rencana `docs/superpowers/plans/2026-10-29-penempatan-jadwal-massal.md`.

## Acceptance Criteria
- HR bisa menyaring karyawan belum ditempatkan atau belum berjadwal per toko/departemen/jabatan, berpindah halaman, lalu memindahkan 50 karyawan ke satu toko dalam satu aksi dengan pratinjau.
- HR bisa menerapkan pola mingguan ke banyak karyawan untuk 4 minggu dalam satu aksi, mengubah satu kotak, menyalin minggu lalu, dan melihat hasilnya langsung di grid tanpa memuat ulang halaman.
- Aturan validasi jadwal sama dengan unggah Excel (tes Go).
- Shift Kerja menampilkan jumlah pemakai; shift yang dipakai tidak bisa dihapus/nonaktif tanpa pesan jelas.
- `go test ./...`, `pnpm test`, `pnpm lint`, dan `tsc` hijau; CI 4/4 hijau; QA staging lulus; deploy production tanpa error.

## Automation Log
- 2026-10-29: **Instruksi user:** revisi HR (screenshot Penempatan Karyawan + Shift Kerja), lalu "lakukan epic nya … sampai semua benar merged dan deploy". Pemetaan kode main `cfb78e60`:
  - overview penempatan memuat semua karyawan tanpa halaman, filter di klien;
  - aksi massal hanya "tambah toko" (utama tidak mengakhiri yang lama);
  - grid jadwal hanya-baca, jadwal diubah lewat unggah Excel atau pola mingguan per karyawan;
  - mutasi shift/pola tidak menginvalidasi grid (cache 2 menit).
- 2026-10-29: **Implementasi** (subagent per task + review per task, `superpowers:subagent-driven-development`), branch `feat/epic070-penempatan-jadwal`:
  - **T1** `5e7d72ba`: `GET /employee-placements/search` + `/ids`, filter status/departemen/jabatan, berhalaman, `counts`. Ikut memperbaiki 500 lama pada `include_ended=true`.
  - **T2** `fb3af530`: `/bulk` dengan `mode` add|move dan `step` preview|commit dalam satu transaksi + audit; `/bulk-end`; `domain.PlanPlacementMove`.
  - **T3** `dd8d64ac`: `POST /schedules/cells` dengan validasi bersama `ValidateScheduleUpload` (refactor tanpa ubah perilaku), guard timpa/hapus, fingerprint 409, audit `schedule.cells_update`.
  - **T4** `6dc0e642`: `GET /shifts?with_usage=true` → `employees_using`.
  - **T5** `ca28dc3a..516ed11d`: halaman Penempatan baru (chip, 4 filter, halaman 25/50/100, kolom Jadwal, bilah aksi massal, dialog pratinjau, pilih semua hasil filter).
  - **T6** `9f48cb88..3b974145`: Jadwal Kerja bisa diedit (kotak, rentang, isi/kosongkan, pola, salin minggu lalu, navigasi minggu).
  - **T7** `7097b070..272b594b`: Shift Kerja tabel ringkas berkelompok, cari, jumlah pemakai, invalidasi lintas halaman.
- 2026-10-29: **Review akhir seluruh branch** (Opus): With fixes.
  - Kritis: 4 route Go baru belum ada di `go-routes.generated.json`. Proxy Next akan 404 dan CI gagal.
  - Penting: edit penempatan tunggal tidak menyegarkan grid; angka pemakai shift ≠ aturan penolakan server; dokumen.
  - Diperbaiki `098b9248..35ad14dd` (manifest + tes `servedByGo`, invalidasi termasuk Shift Tim ESS, aturan `employees_using` = guard, tes 403 cells/bulk-end, "Tidak ada perubahan", peringatan tanggal lampau). Re-review lalu menyusul.
- 2026-10-29: **Gerbang lokal:**
  - `go vet` + `go test ./...` hijau, kecuali 2 tes flaky di paket lain (gymcredits `TestRulesRoutes`, hris `TestLeaveBalanceImportLocksInIDOrder`) yang lulus 5/5 saat dijalankan terpisah (DB uji dipakai bersama paralel);
  - frontend `tsc` + `lint` bersih, `pnpm test` 853 file / 6177 tes lulus (sebelum fix wave).
- **Minor ditunda (backlog):**
  - count + baris tanpa snapshot sama;
  - ILIKE `%`/`_` tidak di-escape;
  - audit massal tanpa daftar primer yang diturunkan;
  - kotak dikosongkan dihitung tidak kerja untuk peringatan (seharusnya pola mingguan);
  - beruntun dipotong jendela ±6 hari;
  - multi-pilih keyboard;
  - sel grid belum di-memo;
  - teks "error" lama di dialog unggah;
  - hapus shift tanpa penjaga klik ganda.
- 2026-10-29: **PR #45 merged** (`940be473`) setelah CI 4/4 hijau (Audit, Backend Go, Lint, Unit test).
- 2026-10-29: **Deploy staging** (paddydev.reddie.id) 03:07 WIB, image `local-20261029-030739` = main `940be473`.
  - Backup: `~/Desktop/paddydev-backups/staging-db-before-epic070-20261029-030726.dump`. Rollback image: `local-20261027-210041`.
  - Tanpa migrasi baru; verify 10705 objek, 0 beda.
  - Route baru tanpa login: 401 (bukan 404).
- 2026-10-29: **QA staging di browser** (Playwright, akun dan karyawan uji `qa070-*` di toko PD-PVJ/PD-KOPO tanpa karyawan nyata): **8/8 skenario, 31/31 cek lulus**, 0 panic/5xx.
  - Penempatan: chip, filter, halaman, kolom Jadwal; pindah massal dengan pratinjau dan audit; akhiri massal; pilih semua hasil filter → Atur jadwal.
  - Jadwal: satu kotak, rentang, kosongkan dengan alasan, pola 2 minggu, salin minggu lalu, tanggal lampau terkunci, template mulai hari ini.
  - Shift: kelompok, cari, angka Dipakai, tidak ada hapus untuk shift dipakai, perubahan jam langsung terlihat.
  - Penolakan API: tanggal lampau, Non-Shift, kode tak dikenal.
  - Bersih-bersih: semua data qa070 = 0, kecuali 12 baris `audit.audit_log` (tabel append-only). Laporan + screenshot: `.share/qa070/`.
  - **Temuan di luar EPIC-070 (backlog):**
    - header aplikasi menampilkan tanggal UTC sebelum 07.00 WIB;
    - template Excel mengisi kotak kosong dengan DEF, sehingga unggah tanpa diubah menulis DEF (perilaku EPIC-056);
    - pesan kode tak dikenal di simpan per kotak masih menyebut "sheet Kode Shift" (hanya lewat API langsung).
- 2026-10-29: **Production belum di-deploy.** Pukul 03.23 WIB (0 absen 30 menit terakhir) langkah backup + kirim source ke VPS ditolak oleh pengaman izin otomatis Claude Code (kategori Production Deploy). Menunggu user menjalankan atau mengizinkan langkah production.

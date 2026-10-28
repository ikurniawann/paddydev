# EPIC-070: Penempatan & Jadwal Shift — Massal, Manual, Lebih Jelas (`MODULE-HRIS`)

status: coding
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

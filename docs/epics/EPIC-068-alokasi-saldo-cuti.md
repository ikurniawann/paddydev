# EPIC-068: Alokasi Saldo Cuti oleh HR — Manual, Massal, Sisa Bawaan (`MODULE-HRIS`)

status: on-progress
environment: dev
retries: 0

## Goal
HR bisa mengatur saldo Cuti Tahunan karyawan dengan aman dan tercatat:
1. **Hak akses:** aksi per menu pada menu daun baru **Saldo Cuti** (`hris.workforce.leave-balances`: read, update, import, export). Aksinya bisa diberikan ke role HRIS lewat Hak Akses HRIS. Default: HR Manager + Admin HRIS (+ Super Admin/Admin).
2. **Ubah manual** di tab Leave Balance (detail karyawan) dan halaman Saldo Cuti:
   - kolom: jatah, penyesuaian (boleh negatif), sisa bawaan + tanggal kedaluwarsa;
   - alasan wajib;
   - pratinjau sisa sebelum simpan.
3. **Import Excel:**
   - template per cabang penempatan utama atau semua karyawan aktif;
   - pratinjau → simpan dalam satu transaksi, plus file error;
   - pola sama dengan import Jadwal Kerja.
4. **"Terpakai" tetap otomatis** dari cuti yang disetujui atau dibatalkan. Saldo awal dan koreksi lewat kolom **penyesuaian**; sisa tahun lalu lewat **sisa bawaan**.
5. **Validasi:**
   - sisa ≥ 0;
   - kelipatan 0,5;
   - hanya karyawan aktif;
   - hanya tahun berjalan atau tahun depan.
6. **Audit log** per karyawan: sebelum → sesudah + alasan, terlihat di Riwayat Perubahan karyawan.
7. **ESS** menampilkan dan memakai saldo baru (jatah + penyesuaian − terpakai + sisa bawaan yang belum kedaluwarsa).

## Keputusan user (26-10-2026)
- **Jatah default karyawan baru:**
  - 0 sampai genap 12 bulan kerja (MOM §3.1);
  - di tahun genap 12 bulan: prorata hari dari tanggal genap sampai 31 Des, dibulatkan ke bawah ke kelipatan 0,5;
  - tahun berikutnya: 12 penuh.
- **Sisa bawaan (carry-over):**
  - HR mengatur di setting: aktif/tidak, maksimal hari (kosong = semua sisa), dan tanggal kedaluwarsa di tahun baru;
  - diproses **otomatis 1 Januari**;
  - dipakai lebih dulu dan hangus setelah tanggal kedaluwarsa.
- **Import:**
  - filter per cabang atau semua;
  - kolom jatah, penyesuaian, alasan, sisa bawaan, kedaluwarsa;
  - boleh membuat baris saldo yang belum ada.
- **Akses default:** HR Manager + Admin HRIS.

## Tasks
Lihat `docs/superpowers/plans/2026-10-26-alokasi-saldo-cuti.md` (Task 1–7, ruling R1–R14).

## Acceptance Criteria
- Hanya akun dengan aksi `update` di `hris.workforce.leave-balances` yang bisa mengubah saldo (manual, setting). Hanya `import` yang bisa meng-import, dan hanya `read`/`export` yang bisa melihat atau mengunduh template. Pemilik saldo tetap bisa melihat saldonya sendiri.
- Perubahan tanpa alasan, bukan kelipatan 0,5, membuat sisa < 0, untuk karyawan nonaktif, atau untuk tahun selain berjalan/depan ditolak dengan pesan jelas (manual dan import).
- **Import:** pratinjau menampilkan per karyawan sebelum → sesudah, galat, dan peringatan. Simpan memakai fingerprint (409 bila data berubah sejak pratinjau). File error berisi kolom "Keterangan Error". Semua baris tersimpan dalam satu transaksi atau tidak sama sekali.
- **Cuti:**
  - cuti tahunan yang disetujui memakai sisa bawaan yang masih berlaku dulu (berlaku pada tanggal mulai cuti), lalu jatah;
  - pembatalan mengembalikan ke sumber yang sama;
  - pengajuan ditolak bila sisa yang tersedia kurang, juga ketika baris saldo belum ada (baris dibuat dengan jatah default).
- Setiap 1 Januari (WIB) job membuat baris tahun baru untuk semua karyawan aktif (jatah default) dan, bila setting aktif, sisa bawaan = min(sisa jatah tahun lalu, maks) dengan kedaluwarsa sesuai setting. Job tidak pernah jalan dua kali untuk tahun yang sama dan tidak menimpa sisa bawaan yang sudah diisi HR.
- ESS (Beranda, Cuti, form pengajuan) menampilkan sisa tersedia, rincian sisa bawaan + kedaluwarsa, tanpa angka "12" palsu.
- Migrasi lulus gladi di salinan DB production: baris lama tidak berubah nilainya, karyawan aktif tanpa saldo tahun berjalan dibuatkan dengan jatah default, `migrate -verify` 0 selisih, skrip cek akses 0 akun kehilangan akses.

## Tindak lanjut
- **Snapshot sisa bawaan vs cuti tahun lalu yang diputuskan setelah job 1 Januari: SELESAI di kode (belum deploy).** Sisa bawaan kini dihitung ulang otomatis di setiap perubahan saldo tahun lalu, dengan batas maksimal dari admin HRIS dan penanda "perlu ditinjau HR" (rincian di Automation Log, entri 27-10-2026). Setting sisa bawaan tetap **mati** sampai user mengizinkan mengaktifkannya; deploy mengikuti alur migrasi production.

## Automation Log
- 2026-10-26: **Instruksi user:** "Mulai EPIC-068 … mengikuti alur migrasi", ditambah keputusan di atas (dua kali tanya jawab). Kondisi awal (pemetaan kode main `e6554fa`):
  - `PUT /api/hris/leave-balances` hanya untuk role `hrd`, tanpa audit dan tanpa UI;
  - tab Leave Balance rusak (bentuk respons tidak cocok);
  - trigger jatah awal (12 − bulan) berbeda dengan Go (13 − bulan);
  - pengajuan tanpa baris saldo tidak dicek;
  - ESS menampilkan "12" bila saldo kosong;
  - belum ada carry-over.
  
  Data production 26-10: 254 baris saldo 2026 (jatah 2–24), 126 karyawan aktif tanpa saldo 2026.
- 2026-10-26: **Task 1-7 selesai** (branch `feat/epic068-saldo-cuti`, basis `acb887a`). Status epic diubah controller, bukan di entri ini.
  - **Task 1 `3e39f8e`:** migrasi (kolom adjustment/carry-over, `annual_leave_remaining` generated, `hris.leave_settings`, `leave_carry_over_runs`, `leaves.carry_over_days_used`), fungsi jatah default R1, backfill 2026, trigger karyawan baru, menu daun Saldo Cuti + grant + masker.
  - **Task 2 `d1d0a84`:** `LeaveBalanceView` bersama (GET, daftar, PUT, ESS), pembuatan baris lazy, PUT manual beralasan beraudit, persetujuan memakai sisa bawaan dulu, pembatalan mengembalikan per sumber, `CreateLeave` memeriksa sisa tersedia.
  - **Task 3 `f513b70`:** template dan import Excel (pratinjau, fingerprint, ack peringatan, satu transaksi, file error).
  - **Task 4 `a70ef1a`:** setting sisa bawaan (`GET/PUT /api/hris/leave-settings`) dan job 1 Januari (`LeaveCarryOverLoop`, klaim per tahun).
  - **Task 5 `41b8a13`:** frontend HR: tab Leave Balance karyawan, halaman Saldo Cuti, ubah manual, setting bawaan.
  - **Task 6 `3806a97`:** dialog import Excel.
  - **Task 7 (commit ini):** ESS (Beranda, Izin & Cuti, form pengajuan) menampilkan `available` dengan rincian jatah/penyesuaian/terpakai/sisa bawaan + kedaluwarsa, "12" palsu dihapus; acuan ES-03 diperbarui.
  - **Keputusan controller yang berlaku:**
    - akun role `hrd` tidak lagi bisa mengubah saldo lewat cek role; harus diberi aksi `update` di Saldo Cuti lewat Hak Akses HRIS;
    - mengaktifkan sisa bawaan di tengah tahun baru berlaku pada 1 Januari berikutnya (klaim tahun berjalan sudah terpakai); sisa bawaan tahun berjalan diisi manual atau lewat import;
    - cabang saldo = cabang penempatan utama (`employee_placements`);
    - fingerprint import mencakup karyawan yang ada di file (bukan seluruh cakupan cabang).
  - **Bukti:** lihat `.superpowers/sdd/2026-10-26-alokasi-saldo-cuti/task-7-report.md` untuk perintah dan hasil verifikasi penuh. Memori hanya cache; keputusan ini adalah catatan resminya.
- 2026-10-26: **Gelombang perbaikan review akhir seluruh branch** (15 butir; laporan: `.superpowers/sdd/2026-10-26-alokasi-saldo-cuti/final-fix-report.md`).
  - **Backend:** pembatalan cuti memakai `AND status = <dibaca>` dalam satu transaksi (409 "Pengajuan sudah berubah, muat ulang halaman"; tidak ada refund ganda atau hari hilang saat berpacu dengan persetujuan); fingerprint import = hash file + kolom yang ditulis commit (tanpa "terpakai"/exists), baris dikunci urut `employee_id`; `step` tak dikenal = 400; unggahan dibatasi 16 MB (`MaxBytesReader`); PUT parsial membaca angka yang tak dikirim dari baris terkunci; GET saldo hanya membuat baris untuk karyawan aktif dan tahun ≤ tahun depan; snapshot skema `leave_balances` diselaraskan (`migrate -verify` 0 beda).
  - **Frontend:** subtitle Beranda memuat penyesuaian bertanda; tombol "Unduh template" tampil dengan `export` saja; dialog import terkunci saat memeriksa dan pratinjau terikat pada file yang diperiksa; kartu setting menaruh respons PUT ke cache; PUT fallback Next saldo cuti mengembalikan 501.
  - **Tindak lanjut (belum dikerjakan, wajib sebelum HR mengaktifkan sisa bawaan / sebelum 1 Januari 2027):** snapshot sisa bawaan job 1 Januari dihitung dari `annual_leave_remaining` tahun Y-1 pada saat job jalan. Cuti tahun Y-1 yang disetujui atau dibatalkan SETELAH job (mis. cuti 28 Des yang baru disetujui 3 Januari) tidak mengubah bawaan yang sudah tercatat, sehingga bawaan bisa terlalu besar atau terlalu kecil. Putuskan aturannya (hitung ulang selisih saat persetujuan/pembatalan tahun lalu, atau tutup persetujuan cuti tahun lalu sebelum job) dan uji sebelum fitur bawaan diaktifkan.
- 2026-10-27: **Tindak lanjut "sisa bawaan dihitung ulang otomatis" selesai di kode** (branch `feat/epic068-sisa-bawaan`, basis `e4b4ab6`; rencana: `docs/superpowers/plans/2026-10-27-sisa-bawaan-hitung-ulang.md`; laporan per tugas: `.superpowers/sdd/2026-10-27-sisa-bawaan-hitung-ulang/`). **Belum deploy.**
  - **Keputusan user (26-10-2026, jawaban 4 pertanyaan):**
    - tanggal kedaluwarsa dipertahankan (diatur admin HRIS, bawaan dipakai lebih dulu);
    - admin HRIS mengatur batas maksimal sisa bawaan; awalnya kosong = tanpa batas;
    - karyawan yang terdampak ditinjau HR: koreksi manual atau tandai selesai dengan catatan, tanpa pemotongan otomatis;
    - sisa bawaan dihitung ulang otomatis pada setiap perubahan saldo tahun lalu;
    - setting sisa bawaan **tetap mati** sampai user mengizinkan mengaktifkannya (PR #33 dirilis dulu dengan setting terkunci).
  - **Rumus:** `carry(N) = floor_0,5( min( annual_leave_remaining(N-1), max ) )`, nilai negatif dianggap 0; hanya bila setting menyala dan kunci terbuka; hanya karyawan aktif; hanya baris `carry_over_source = 'auto'`.
  - **Ringkasan implementasi:**
    - `efed744` + `54b8904` + `7aaf4d3` (Tugas 1): migrasi `20261029090000_leave_carry_over_recalc.sql` (tabel `hris.leave_carry_over_reviews` yang membuka kunci setting, kolom `carry_over_source` dengan backfill, CHECK `carry_over_used <= carry_over_days` dilepas, setting tetap mati), `recomputeCarryOver` dan hook di persetujuan, pembatalan, PUT manual, commit import, dan job 1 Januari; audit `leave_balance.carry_over`; kunci baris N-1 lalu N dengan urutan tetap.
    - `d161bc2` + `1810344` (Tugas 2): setting batas dengan alasan wajib (audit `leave_settings.update`), `POST /api/hris/leave-settings/preview` (fungsi yang sama dengan simpan dalam transaksi yang di-rollback), hitung ulang tahun berjalan saat batas berubah atau setting dinyalakan.
    - `2ea9709` + `ec39b82` (Tugas 3): `GET /api/hris/leave-balances/reviews`, `POST .../reviews/{id}/resolve` (catatan wajib, audit `leave_balance.review_resolve`), `carry_over_source` di saldo, UI kartu "Perlu ditinjau HR", kolom sumber, setting dengan alasan dan pratinjau dampak.
  - **Ruling controller yang berlaku:**
    - input HR (PUT/import) menolak bawaan < bawaan terpakai **hanya bila permintaan mengubah bawaan atau kedaluwarsa**; mengubah jatah/penyesuaian pada baris yang sedang di bawah terpakai tetap boleh;
    - baris tahun depan yang sudah ada dihitung dari sisa berjalan tahun ini (proyeksi) dan terus diperbarui; job 1 Januari memfinalkan;
    - review yang diselesaikan HR tidak dibuka lagi oleh angka yang sama persis; review yang tertutup otomatis tidak menahan kekurangan identik yang muncul lagi;
    - `needs_review` di pratinjau = jumlah karyawan yang review-nya baru dibuka atau berubah oleh perubahan batas (bukan total review terbuka);
    - bawaan manual/import (`'manual'`) tidak pernah ditimpa; mengosongkan bawaan lewat PUT mengembalikan ke `'auto'`.
  - **Bukti:** perintah dan hasil verifikasi penuh di `.superpowers/sdd/2026-10-27-sisa-bawaan-hitung-ulang/task-4-report.md`. Memori hanya cache; entri ini catatan resminya.
  - **Sebelum aktif di production:** deploy lewat alur migrasi production (gladi salinan, gladi VPS, jam sepi), lalu user memutuskan kapan setting dinyalakan dan berapa batasnya.

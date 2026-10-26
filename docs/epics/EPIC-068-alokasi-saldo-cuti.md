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

## Automation Log
- 2026-10-26: **Instruksi user:** "Mulai EPIC-068 … mengikuti alur migrasi", ditambah keputusan di atas (dua kali tanya jawab). Kondisi awal (pemetaan kode main `e6554fa`):
  - `PUT /api/hris/leave-balances` hanya untuk role `hrd`, tanpa audit dan tanpa UI;
  - tab Leave Balance rusak (bentuk respons tidak cocok);
  - trigger jatah awal (12 − bulan) berbeda dengan Go (13 − bulan);
  - pengajuan tanpa baris saldo tidak dicek;
  - ESS menampilkan "12" bila saldo kosong;
  - belum ada carry-over.
  
  Data production 26-10: 254 baris saldo 2026 (jatah 2–24), 126 karyawan aktif tanpa saldo 2026.

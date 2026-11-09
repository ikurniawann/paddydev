# EPIC-096: Pemetaan Data Karyawan dari File HR ke Production (`MODULE-HRIS`)

status: done
environment: dev
retries: 0

## Goal
Permintaan user (09-11-2026):

> selanjutnya adalah me mappingkan data karyawan production dengan penempatan kerja dan jadwal kerja yang ada di file ini … jika ada ketidak sesuaian data tlong rapihkan dan laporkan

**Sumber.** File HR "DATA KARYAWAN - BULKY UPLOAD REQUEST FOR WIT" berisi dua sheet:
- `PKWTPKWTT`: 265 baris;
- `FREELANCEDW`: 49 baris.

Kolomnya NIK, Nama, Join Date, Branch, Departemen, Level Jabatan, Posisi, Approver di HRIS, dan Status/Tipe Kontrak. File ini **tidak** memuat jadwal kerja.

**Pencocokan read-only (09-11).** 313 dari 314 baris cocok dengan karyawan production; 16 di antaranya lewat nama. Laporan lengkap ada di `.share/data-karyawan-20261109/laporan-pencocokan-data-karyawan.xlsx`.

| Perbedaan | Jumlah |
|---|---|
| Atasan (approver) | 81 |
| Join date | 68 |
| Status kontrak | 66 |
| Jabatan (setelah nama jabatan berawalan kode toko disamakan) | 36 |
| Penempatan utama | 25 |
| Departemen | 6 |
| NIK file ≠ NIP HRIS | 16 |
| Jabatan belum ada di master | 46 baris |
| Approver tidak unik (alias / "Ricky / Azka") | 28 |

**Jadwal kerja di production.** Non-Shift memakai pola mingguan (122 dari 124 terisi). Shift (192 staf toko) dijadwalkan harian oleh pengatur jadwal toko. Karena file tidak memuat jadwal, pola tidak diubah.

## Ruling (keputusan user 09-11)
- **R1 — Cakupan yang mengikuti file:**
  - penempatan utama, departemen, jabatan;
  - atasan;
  - status kontrak dan join date;
  - NIP mengikuti NIK file.
- **R2 — Jabatan yang belum ada di master** dibuat baru dengan nama dan level dari file, lalu dipasang.
- **R3 — Approver "Ricky / Azka"** mengikuti wilayah toko:
  - toko yang dipegang Ricky Pratama Sidik di Pengatur jadwal (MBRM, MPVJ, PBSA, PPGA, PPVJ) → Ricky;
  - toko Azka Hanifah Hamidah (MBIP, MPSL, MTSM, PLRM, PPBA) → Azka;
  - selain itu dilaporkan.

  Alias ejaan yang jelas dirapikan: Zaidane → Zaidane Gusma, Azka → Azka Hanifah Hamidah, Risky Melianda → Risky Melinda, Bungsu Tri Sutisna → Bungsu Tri Sutrisna, Adhimukti → Adhimukti Nurrahman.
- **R4 — Aktif/nonaktif tidak diubah**, hanya dilaporkan ke HR:
  - 5 karyawan di file yang nonaktif di HRIS;
  - Elen Fitrianti, yang belum ada di HRIS;
  - 4 karyawan aktif HRIS yang tidak ada di file.

  Baris yang cocok ke karyawan nonaktif tidak diubah sama sekali.
- **R5 — Konflik dilewati dan dilaporkan, tidak dipaksakan:**
  - NIK file yang sudah dipakai karyawan lain (mis. record ganda nonaktif);
  - karyawan yang muncul dua kali di file dengan isi berbeda;
  - approver yang tidak bisa dipastikan.
- **R6 — Saldo cuti tidak dihitung ulang.** Saldo 2026 sudah tersimpan untuk semua karyawan aktif. Perubahan join date hanya berlaku untuk perhitungan berikutnya.
- **R7 — Eksekusi production** hanya dengan izin user di jam sepi, setelah gladi di salinan DB production.

## Tasks
Lihat `docs/superpowers/plans/2026-11-09-pemetaan-data-karyawan-hr.md`.

## Acceptance Criteria
- [x] Setelah eksekusi, pencocokan ulang file HR vs production tidak menyisakan perbedaan dalam cakupan R1, kecuali yang dilaporkan sebagai konflik (R5).
- [x] Setiap perubahan tercatat di audit per karyawan, dan ada backup sebelum eksekusi.
- [x] HR menerima laporan sisa: konflik, nonaktif, karyawan baru, dan level jabatan yang berbeda.

## Automation Log
- 2026-11-09: Pencocokan read-only dan laporan ke user. User memutuskan R1–R4. Rencana ditulis.
- 2026-11-09: **Implementasi + gladi** (Native) di branch `ops/data-karyawan-hr-20261109`.
  - Generator `backend/database/sql/data-karyawan-hr/` dengan 22 tes.
  - Hasil gladi di salinan production: 196 karyawan berubah (atasan 86, jabatan 74, join date 66, status 61, penempatan 20, NIP 13, departemen 4) dan 28 jabatan baru. Generate ulang menghasilkan 0 perubahan.
  - Dilewati dan dilaporkan (14 baris):
    - 5 nonaktif di HRIS;
    - 4 "Ricky / Azka" di Headquarter;
    - Kayla Olivia Berliana muncul 2 kali dengan isi berbeda;
    - 3 NIP bentrok dengan record nonaktif (Dominic, Kezia, Syakira).
  - Dirapikan: jabatan berawalan kode toko, ejaan setara, dan ejaan jabatan baru (Patnership → Partnership, Assisstant → Assistant, SPV → Supervisor).
- 2026-11-09: **Eksekusi production 23.31–23.32 WIB** (izin user: "Malam ini 23.30 WIB"; sheet FREELANCE/DW ikut, status mengikuti file). PR #88 merged → main 11660d40.
  - Ringkasan sama dengan gladi, kecuali "aktif tidak di file" 9 → 10: Kurnia Devi Lestari, karyawan baru 06-11.
  - Backup: `~/backups/prod-db-before-e096-20261109-233115.dump`, salinan lokal di `~/Desktop/paddydev-backups/data-karyawan-hr/prod-20261109-233115/`.
  - Diterapkan:
    - 01: 28 jabatan baru;
    - 02: penempatan 20 karyawan, berlaku 10-11 (laporan `DITERAPKAN`, error 0);
    - 03: 188 audit `employee.update`.
  - Total 196 karyawan berubah. Pencocokan ulang: **0 perubahan tersisa**; `/login` dan `/api/ready` 200.
  - Insiden kecil: pratinjau 02 gagal `permission denied` karena skrip eksekusi berjalan dengan `umask 077` (file input 0600 tidak terbaca user container). Saat itu baru 01 yang masuk. Dilanjutkan dengan `umask 022` + `chmod 644`, tanpa ulang 01. Pelajaran: file yang di-`docker cp` ke container harus terbaca (≥ 0644).
  - Laporan HR `.share/data-karyawan-20261109/hasil-pemetaan.xlsx`:
    - 14 dilewati;
    - 10 aktif tidak di file;
    - 20 level file ≠ master;
    - 22 pengajuan menunggu yang penyetujunya berganti (5 cuti, 16 lembur, 1 pinjaman);
    - roster Elvan Sabiq s.d. 15-11 di toko lama;
    - 2 jabatan baru berlevel "Coordinator" (Packing/Production Coordinator) yang belum ada di daftar level master, sehingga dihitung bukan leader. HR yang menentukan levelnya.

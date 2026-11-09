# EPIC-096: Pemetaan Data Karyawan dari File HR ke Production (`MODULE-HRIS`)

status: backlog
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
- [ ] Setelah eksekusi, pencocokan ulang file HR vs production tidak menyisakan perbedaan dalam cakupan R1, kecuali yang dilaporkan sebagai konflik (R5).
- [ ] Setiap perubahan tercatat di audit per karyawan, dan ada backup sebelum eksekusi.
- [ ] HR menerima laporan sisa: konflik, nonaktif, karyawan baru, dan level jabatan yang berbeda.

## Automation Log
- 2026-11-09: Pencocokan read-only dan laporan ke user. User memutuskan R1–R4. Rencana ditulis.

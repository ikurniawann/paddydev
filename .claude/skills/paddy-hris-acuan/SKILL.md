---
name: paddy-hris-acuan
description: Use when planning, implementing, reviewing, or answering questions about Paddy HRIS rules — cuti/izin quotas, approval (atasan langsung, kasbon, lembur, CEO/GM), payroll formulas (gaji pokok, tunjangan, lembur, Izin Pribadi, Izin Sakit, PPh21, THP), kasbon/pinjaman, absensi (geofence, koreksi absen), or audit log — or when a task cites MOM requirement IDs like ES-, AP-, PY-, KP-, AB-, LA- or K-n.
---

# Acuan HRIS Paddy

## Overview

Aturan HRIS Paddy disepakati di MOM 19-10-2026 dan tabel payslip Paddy, **bukan** di default kode. Kode sering berbeda (mis. plafon kasbon 30% gaji pokok, cut off default bulan kalender). Jawab dan bangun dari acuan, lalu tunjukkan selisihnya dengan kode.

Detail lengkap ada di `docs/hris/acuan-mom-20261019/`:
- `status-requirement.md`: status tiap ID beserta bukti `file:line`.
- `aturan-payroll.md`: rumus payslip vs kalkulator Go.
- `matriks-approval.md`: approver per pengajuan dan org chart.
- `pertanyaan-rekomendasi.md`: poin terbuka beserta rekomendasinya.

## Cara pakai

1. Cari ID requirement atau topiknya di `status-requirement.md`. Kolom "Status MOM" menentukan boleh tidaknya dikerjakan.
2. Jika statusnya **Konfirmasi (K-n)** atau **Menunggu TnC**, buka `pertanyaan-rekomendasi.md`. Rekomendasi di sana sudah diputuskan user pada 20-10-2026, kecuali K-7 dan semua butir yang statusnya "Menunggu TnC". Untuk yang belum diputuskan, tanyakan dengan menyertakan rekomendasinya. Jangan menebak.
3. Rumus payroll diambil dari `aturan-payroll.md`, bukan dari kode yang ada.
4. Approver diambil dari `matriks-approval.md`. "Atasan langsung" = `hris.employees.reporting_to`.
5. Cek setting & data live (read-only) sebelum menyimpulkan perilaku. Banyak aturan bergantung pada setting, bukan kode.
6. Setiap keputusan baru dicatat di epic, lalu file acuan diperbarui di commit yang sama.

## Aturan kunci

| Topik | Acuan |
|---|---|
| Periode | Cut off tgl 20, periode 21 → 20 (live masih `cutoff_day = 0`) |
| THP untuk "1 hari THP" | 4 komponen: Gaji Pokok + Tunj. Makan + Tunj. Transport + Tunj. Jabatan |
| Izin Pribadi | 3 hari/bulan, hanya bila cuti tahunan habis/belum berhak (EPIC-084), potong 1 hari THP; intern tidak dipotong |
| Izin Leader | 1 hari/bulan, hangus, level Supervisor ke atas, full paid |
| Sakit | Potong (Transport + Makan) per hari (PIS); diajukan sendiri pada hari H (EPIC-084) |
| Cuti Melahirkan / Keguguran / Haji | Melahirkan 90, keguguran 45 (lampiran wajib), haji 40 hari kalender (H-30). Full paid bila masa kerja ≥ 1 tahun; < 1 tahun → PIP (haji hanya lewat hari ke-14), untuk cuti mulai ≥ 03-11-2026 (EPIC-084) |
| Cuti khusus & Birthday | Menikah 3 (H-3), Istri Melahirkan 2, Istri Keguguran 2, Menikahkan Anak 2 (H-3), Khitanan/Baptis 2 (H-3), Berkabung 2 hari kerja; Birthday Leave 1 hari di bulan lahir, 1x/tahun, lampiran wajib, setelah kontrak awal (EPIC-084) |
| SLA pengajuan | Pengajuan sendiri: Cuti H-3, Ibadah H-30, Sakit hari H, Lembur H+1; ditolak (409) bila periode tanggalnya sudah lewat `end_date` (tgl 21 setelah cutoff 20 aktif). HR untuk karyawan lain bebas H-n (EPIC-084) |
| Cuti Tahunan | 12/tahun setelah 12 bulan kerja: jatah default 0 → prorata tahun genap 12 bulan → 12 (ditegakkan saat baris saldo dibuat, EPIC-068; mulai saldo 2027 prorata = bulan sesudah bulan genap dikurangi cuti bersama, genap di Januari = 12, EPIC-084); HR atur jatah/penyesuaian/sisa bawaan di menu Saldo Cuti; maks. 3/bulan belum ditegakkan |
| Lembur | Weekday 1,5×, libur/istirahat 2× Gaji Pokok/173 per jam; min. 1 jam; diinput sendiri setelah selesai, paling lambat H+1 (EPIC-066/084); approver atasan langsung; SPV ke atas tidak lembur; hari kerja mulai ≥ jam selesai shift; maks. 4 jam/hari, 18 jam/minggu, 72 jam/periode (blokir); dinas di tanggal merah = tarif libur tanpa clock-out (EPIC-085) |
| Keterlambatan | Terlambat = lewat toleransi shift (`is_late`), dihitung per periode absensi: 1–2 teguran lisan & tertulis, 3–4 SP 1, 5–6 SP 2, > 6 SP 3 dan/atau PHK; info di ESS & Absensi Tim, SP diterbitkan HR. Potongan telat `tier` payroll tetap terpisah (EPIC-077) |
| Jam kerja | HQ Sen–Jum masuk 08.00–09.00, istirahat 12–13, pulang 17.00–18.00 → shift NS 09.00 toleransi 0. Operations (Warehouse) Sen–Jum 08.00–16.00, Sab 08.00–14.00 → OPS/OPSSAT toleransi 10. Jadwal tetap Non-Shift mengikuti departemen (EPIC-078) |
| Sistem shift | `schedule_type = 'shift'` (Retail Operasional, Host Live, CRM, Produksi): Senin–Minggu, libur 1 hari/minggu; Shift 1 08.00–16.00, Shift 2 11.00–19.00, Shift 3 13.30–21.30 (toleransi 10). Libur diberikan setelah 40 jam kerja bersih (6 hari × 7 jam) sejak libur sebelumnya: **peringatan** roster, bisa dilewati dengan "sudah diperiksa". Cuti dihitung dari roster (tanggal ber-shift dihitung, sel libur tidak; tanggal tanpa jadwal berlaku dihitung hanya bila Senin–Jumat, sama dengan hari kerja payroll); pembagi payroll dari roster per tanggal hanya bila ada tanggal ber-shift (`scheduled > 0`) dan setiap tanggal periode punya jadwal berlaku, selain itu jalur lama tanpa jadwal (EPIC-079) |
| Hari libur | Staf shift **tidak** mengikuti tanggal merah (kalender, absen terlewat, jenis hari lembur mengikuti roster; pembagi payroll dari roster hanya bila ada tanggal ber-shift dan jadwal periode lengkap). HQ/Operations (Non-Shift): `nasional` + `perusahaan` libur; `cuti_bersama` hari kerja biasa kecuali HR menyalakan "Ditetapkan libur oleh perusahaan" (`is_day_off`) (`domain.HolidayDayOff`, EPIC-079) |
| Checkout otomatis | Per jabatan + lokasi (`hris.auto_checkout_rules`, `branch_id` NULL = semua lokasi); seed Host Live & CRM semua lokasi, Shopkeeper dipilih per toko. Job tiap 5 menit `RunAutoCheckout` (`hris/auto_checkout.go`) menutup absen terbuka di jam selesai shift (`clock_out = scheduled_end`, `clock_out_auto = true`, catatan + audit `attendance.auto_checkout`); clock-out manual lebih dulu tidak ditimpa. Penanda di kalender/Monitoring/Rekap, **tanpa notifikasi**; koreksi HR/pengajuan yang mengubah `clock_out` menghapus penanda. HR atur semua (hapus butuh aksi `delete` Shift Kerja), pengatur jadwal toko hanya tokonya (EPIC-080) |
| 2 shift sehari | Kode jadwal `KODE1+KODE2` (mis. `PAGI4+SORE4`) di grid/upload/pola mingguan (`shift2_id`; `domain.ParseScheduleCode`, `TwoShiftIssue`, `TwoShiftWindow`). Shift kedua mulai setelah shift pertama selesai (tumpang tindih = error). 1 absen gabungan: `scheduled_start` shift 1, `scheduled_end` shift 2, terlambat dari shift 1, `break_minutes` = kedua istirahat + jeda sehingga `work_hours` = jam bersih. Berlaku semua staf shift; payroll tetap 1 hari kerja; aturan 40 jam menghitung kedua shift (EPIC-080) |
| Kasbon & Pinjaman Karyawan | Hanya PKWT/PKWTT (probation dll. tidak bisa). Kasbon: masa kerja ≥ 3 bln, maks. ½ THP, 1–2 kali, wajib dokumen pendukung. Pinjaman Karyawan: masa kerja ≥ 12 bln, maks. 200% THP, 1–6 kali, wajib jaminan. Keduanya: approver Manager Departemen lalu HR (kasbon: HR Penggajian); dipotong payroll periode pengajuan (21–20). Pinjaman Darurat dihapus (EPIC-076) |
| Subsidi BPJS Kes | Rp100.000 bila masa kerja > 1 tahun pada tanggal cutoff |
| Saklar PPh 21 & BPJS | `pph21_enabled`, `bpjs_tk_enabled`, `bpjs_kes_enabled` di Pengaturan Payroll (live: false sejak EPIC-082, keputusan user 02-11 "tidak dipakai dulu"). Nonaktif: PPh 21 = 0 (termasuk Pasal 17 Desember), BPJS karyawan **dan** iuran perusahaan = 0, baris disembunyikan di slip; Subsidi BPJS Kes tidak terpengaruh; berlaku untuk run yang dihitung setelah saklar diubah |
| Time-off | Approver atasan langsung; SPV ke atasannya |
| Lampiran cuti/izin | Atasan langsung lalu HR verifikasi lampiran; dikembalikan = unggah ulang (ES-11, EPIC-088) |

## Kesalahan umum

| Kesalahan | Yang benar |
|---|---|
| Menjawab dari default kode ("kasbon maks 30%", "tenor sampai 60") | Default kode bukan aturan. Pakai acuan dan sebutkan selisihnya |
| Menganggap ID MOM tidak bisa dipetakan karena tidak ada di repo | Petanya ada di `status-requirement.md` |
| Menghitung "1 hari THP" dari seluruh slip | Hanya 4 komponen |
| Menyamakan semua "Lead" dengan Staff atau Supervisor | Brand/Marketing/Commercial Lead di atas SPV; Store Leader di bawah SPV (Q-APV-1) |
| Mengubah setting atau data live tanpa izin | Setting live (cutoff, plafon, koordinat) adalah keputusan user: usulkan, jangan jalankan |

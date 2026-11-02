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
| Izin Pribadi | 3 hari/bulan, potong 1 hari THP; intern tidak dipotong |
| Izin Leader | 1 hari/bulan, hangus, level Supervisor ke atas, full paid |
| Sakit / Cuti Melahirkan | Potong (Transport + Makan) per hari; Sakit ≥ 2 hari wajib surat (K-3) |
| Cuti Tahunan | 12/tahun setelah 12 bulan kerja: jatah default 0 → prorata tahun genap 12 bulan → 12 (ditegakkan saat baris saldo dibuat, EPIC-068); HR atur jatah/penyesuaian/sisa bawaan di menu Saldo Cuti; maks. 3/bulan belum ditegakkan |
| Lembur | Weekday 1,5×, libur/istirahat 2× Gaji Pokok/173 per jam; min. 1 jam; diajukan **sebelum**; approver atasan langsung |
| Keterlambatan | Terlambat = lewat toleransi shift (`is_late`), dihitung per periode absensi: 1–2 teguran lisan & tertulis, 3–4 SP 1, 5–6 SP 2, > 6 SP 3 dan/atau PHK; info di ESS & Absensi Tim, SP diterbitkan HR. Potongan telat `tier` payroll tetap terpisah (EPIC-077) |
| Jam kerja | HQ Sen–Jum masuk 08.00–09.00, istirahat 12–13, pulang 17.00–18.00 → shift NS 09.00 toleransi 0. Operations (Warehouse) Sen–Jum 08.00–16.00, Sab 08.00–14.00 → OPS/OPSSAT toleransi 10. Jadwal tetap Non-Shift mengikuti departemen (EPIC-078) |
| Sistem shift | `schedule_type = 'shift'` (Retail Operasional, Host Live, CRM, Produksi): Senin–Minggu, libur 1 hari/minggu; Shift 1 08.00–16.00, Shift 2 11.00–19.00, Shift 3 13.30–21.30 (toleransi 10). Libur diberikan setelah 40 jam kerja bersih (6 hari × 7 jam): **peringatan** roster, bisa dilewati dengan "sudah diperiksa". Cuti dihitung dari roster, payroll membaca roster per tanggal (EPIC-079) |
| Hari libur | Staf shift **tidak** mengikuti tanggal merah (kalender, absen terlewat, lembur, payroll mengikuti roster). HQ/Operations (Non-Shift): `nasional` + `perusahaan` libur; `cuti_bersama` hari kerja biasa kecuali HR menyalakan "Ditetapkan libur oleh perusahaan" (`is_day_off`) (`domain.HolidayDayOff`, EPIC-079) |
| Kasbon & Pinjaman Karyawan | Hanya PKWT/PKWTT (probation dll. tidak bisa). Kasbon: masa kerja ≥ 3 bln, maks. ½ THP, 1–2 kali, wajib dokumen pendukung. Pinjaman Karyawan: masa kerja ≥ 12 bln, maks. 200% THP, 1–6 kali, wajib jaminan. Keduanya: approver Manager Departemen lalu HR (kasbon: HR Penggajian); dipotong payroll periode pengajuan (21–20). Pinjaman Darurat dihapus (EPIC-076) |
| Subsidi BPJS Kes | Rp100.000 bila masa kerja > 1 tahun pada tanggal cutoff |
| Time-off | Approver atasan langsung; SPV ke atasannya |

## Kesalahan umum

| Kesalahan | Yang benar |
|---|---|
| Menjawab dari default kode ("kasbon maks 30%", "tenor sampai 60") | Default kode bukan aturan. Pakai acuan dan sebutkan selisihnya |
| Menganggap ID MOM tidak bisa dipetakan karena tidak ada di repo | Petanya ada di `status-requirement.md` |
| Menghitung "1 hari THP" dari seluruh slip | Hanya 4 komponen |
| Menyamakan semua "Lead" dengan Staff atau Supervisor | Brand/Marketing/Commercial Lead di atas SPV; Store Leader di bawah SPV (Q-APV-1) |
| Mengubah setting atau data live tanpa izin | Setting live (cutoff, plafon, koordinat) adalah keputusan user: usulkan, jangan jalankan |

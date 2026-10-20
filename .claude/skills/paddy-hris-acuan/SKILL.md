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
2. Jika statusnya **Konfirmasi (K-n)** atau **Menunggu TnC**, buka `pertanyaan-rekomendasi.md`. Kalau jawaban user belum tercatat di epic, tanyakan dengan menyertakan rekomendasinya. Jangan menebak.
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
| Cuti Tahunan | 12/tahun, maks. 3/bulan, setelah 12 bulan kerja |
| Lembur | Weekday 1,5×, libur/istirahat 2× Gaji Pokok/173 per jam; min. 1 jam; diajukan **sebelum**; approver atasan langsung |
| Kasbon | Maks. ½ THP, tenor 1 atau 2 (default 50/50); Probation/Freelance/Intern tidak bisa; approver Manager Departemen **dan** HR Penggajian; Pinjaman Darurat dihapus |
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

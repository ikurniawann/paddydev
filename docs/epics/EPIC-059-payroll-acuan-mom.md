# EPIC-059: Payroll Sesuai Acuan Payslip & MOM 19-10-2026

status: backlog
environment: dev
retries: 0

## Goal
Kalkulator payroll Go mengikuti tabel payslip Paddy dan keputusan MOM 19-10-2026, yang diputuskan user 20-10-2026 ("ikuti semua rekomendasi"). Cakupan: hari dibayar vs hari efektif, pembagi = hari kerja terjadwal, "1 hari THP" = 4 komponen tanpa potong ganda, Sakit & Cuti Melahirkan memotong transport+makan, minimum lembur 1 jam, subsidi BPJS Kesehatan Rp100.000 untuk masa kerja > 1 tahun, dan prorata karyawan yang keluar di tengah periode. Requirement: PY-01, PY-03, PY-04, ES-06.

Acuan: `docs/hris/acuan-mom-20261019/` (aturan-payroll.md, pertanyaan-rekomendasi.md).

## Tasks
Lihat `docs/superpowers/plans/2026-10-20-payroll-acuan-mom.md`.

## Acceptance Criteria
- Gaji pokok (`monthly_prorated`) dan Tunjangan Jabatan per hari dibayar menurut **hari kerja aktif yang dibayar**: hadir + cuti dibayar. Transport & makan per hari menurut **hari hadir**.
- Pembagi harian = hari kerja terjadwal karyawan dalam periode. Pembagi struktur gaji hanya dipakai bila jadwal tidak ada.
- Potongan Izin Pribadi = 1 hari dari 4 komponen (Gaji Pokok, Transport, Makan, Jabatan) tanpa memotong dua kali komponen yang sudah tidak dibayar lewat hari hadir. Intern tidak dipotong.
- Sakit dan Cuti Melahirkan memotong transport & makan per hari: lewat hari hadir bila mode `per_day`, lewat baris potongan bila `flat`.
- Lembur di bawah 1 jam ditolak saat pengajuan dan tidak dibayar.
- Subsidi BPJS Kesehatan dapat diatur global: nominal (Rp100.000) dan syarat masa kerja > 1 tahun pada tanggal cutoff.
- Karyawan yang `end_date`-nya jatuh di dalam periode tetap ikut payroll run, dibayar prorata sampai `end_date`.
- Tidak ada perubahan setting atau data live tanpa persetujuan user. Skrip ops perpindahan `pay_basis` dan mode tunjangan tersedia dengan mode pratinjau.

## Automation Log
- 2026-10-20: epic dibuat. Keputusan user: "Ikuti semua rekomendasi di pertanyaan-rekomendasi.md (K-12, K-10, Q-PY-1..4, K-6/Q-KP-1..2, Q-APV-1..3, K-13, K-3, K-14, K-9, Q-AB-1). K-7 tetap menunggu Paddy." Prasyarat: PR ikurniawann/paddydev#3 (intern tanpa PIP) di-merge dulu.

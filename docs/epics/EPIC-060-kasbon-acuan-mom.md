# EPIC-060: Kasbon Sesuai Acuan MOM 19-10-2026

status: backlog
environment: dev
retries: 0

## Goal
Kasbon dan pinjaman mengikuti MOM 19-10-2026 dan tabel payslip Paddy, diputuskan user 20-10-2026. Cakupan: Pinjaman Darurat dihapus, plafon kasbon 50% THP (4 komponen), tenor 1 atau 2 (default 2, 50/50), Probation/Freelance/Intern tidak bisa mengajukan, approval dua tahap (Manager Departemen lalu HR Penggajian). Requirement: KP-01, KP-02, AP-05.

Acuan: `docs/hris/acuan-mom-20261019/` (aturan-payroll.md, matriks-approval.md, pertanyaan-rekomendasi.md).

## Tasks
Lihat `docs/superpowers/plans/2026-10-20-kasbon-acuan-mom.md`.

## Acceptance Criteria
- Pengajuan baru `emergency` ditolak 400 dan tidak muncul di form ESS/HR. Riwayat tetap terbaca dengan label lama.
- Kasbon: pokok ≤ 50% THP (Gaji Pokok + Transport + Makan + Jabatan dari struktur gaji aktif); tenor hanya 1 atau 2; default 2.
- Kasbon ditolak untuk status kepegawaian probation, freelance, intern/internship.
- Approval kasbon dua tahap: (1) atasan berlevel Manager ke atas pertama di rantai `reporting_to`; (2) HR dengan aksi `approve` di menu Pinjaman **dan** menu Penggajian. Bila rantai tidak punya Manager, tahap 1 dilewati.
- Manager punya antrean persetujuan kasbon tim di ESS.
- Pinjaman jenis `loan` tidak berubah (aturan menunggu TnC, KP-03).

## Automation Log
- 2026-10-20: epic dibuat. Keputusan user: lihat EPIC-059 (keputusan yang sama). Q-KP-1: 1 Pinjaman Darurat pending di live diputus HR secara manual sebelum rilis.

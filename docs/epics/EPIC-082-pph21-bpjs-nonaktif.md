# EPIC-082: Saklar Nonaktif PPh 21 & BPJS di Pengaturan Gaji (`MODULE-PAYROLL`)

status: review
environment: dev
retries: 0

## Goal
Permintaan user 02-11-2026: "di pengaturan gaji PPH21 dan BPJS tidak dipakai dulu jadi harus ada non aktif untuk komponen tersebut, sesuai dengan gaji yang ada di MOM dan artefak dokumentasi".

Acuan:
- tabel payslip Paddy (20-10-2026);
- MOM 19-10-2026 §2.6–§3;
- `docs/hris/acuan-mom-20261019/aturan-payroll.md`.

Di acuan, PPh Pasal 21 masuk Deduction, Subsidi BPJS Kesehatan Rp100.000 masuk Allowance, dan potongan BPJS karyawan tidak ada di daftar Total Potongan (kode menambahkannya, "lebih luas").

Jawaban user 02-11:
- **Cakupan BPJS**: potongan karyawan **dan** iuran perusahaan (TK: JHT, JP, JKK, JKM; Kesehatan) tidak dihitung. **Subsidi BPJS Kesehatan** (tunjangan acuan) tetap mengikuti setting `bpjs_kes_subsidy_amount` sendiri.
- **Saklar**: 3 terpisah: **PPh 21**, **BPJS Ketenagakerjaan**, **BPJS Kesehatan**.
- **Slip**: baris komponen yang nonaktif **disembunyikan**.
- **Berlaku**: untuk run yang dihitung setelah saklar diubah (run baru atau run draft yang dihitung ulang). Run yang sudah disetujui/dibayar tidak berubah.

## Kondisi kode & data per 02-11 (main `0d3afc63`; data staging = salinan production)
- `hris.payroll_settings` punya tarif BPJS, PTKP/bracket, `pph21_method` (`annualized`|`ter`), `pph21_bpjs_basis`, dan `bpjs_kes_subsidy_amount`. **Tidak ada saklar aktif/nonaktif.**
- Kepesertaan ada per struktur gaji (`hris.employee_salary`: `is_taxable`, `bpjs_tk_enrolled`, `bpjs_kes_enrolled`, `tapera_enrolled`; NULL = ikut).
- Staging: 263 struktur aktif, **semuanya** kena pajak dan ikut BPJS TK + Kesehatan (Tapera 0). Jadi saat ini PPh 21 dan BPJS ikut dihitung di setiap run.
- `domain.Calculate` (`payroll/domain/calculator.go:290`) memanggil `CalculateBPJS(base+fixed, in.BpjsTkEnrolled, in.BpjsKesEnrolled, in.TaperaEnrolled, c)` (`:390`) lalu PPh 21 (`:440-472`: TER dengan DPP + BPJS menurut `pph21_bpjs_basis`, atau annualized).
- Slip:
  - PDF (`payslip_pdf.go:299-335`) sudah menyembunyikan baris bernilai 0.
  - Halaman slip HR (`frontend/src/features/hris/payroll/components/payslip-page.tsx:189-193`) selalu menampilkan baris BPJS/PPh.
  - Slip ESS (`frontend/src/features/hris/ess/components/payslip-detail-dialog.tsx`) dan detail run (`payroll-detail-page.tsx`) juga menampilkan kolom/baris ini.

## Ruling
- **R1** Tiga kolom baru di `hris.payroll_settings`: `pph21_enabled`, `bpjs_tk_enabled`, `bpjs_kes_enabled` (boolean NOT NULL DEFAULT true).
  - Migrasi langsung mengisi **false** untuk baris setting yang ada, sesuai keputusan user "tidak dipakai dulu".
  - HR bisa menyalakannya kembali di Pengaturan Payroll.
- **R2** Kalkulasi, saklar dibaca saat run dihitung:
  - `pph21_enabled = false` → PPh 21 = 0, termasuk Pasal 17 tahunan Desember. DPP/TER/PTKP di hasil = 0; metode tercatat `"nonaktif"`.
  - `bpjs_tk_enabled = false` → JHT/JP karyawan dan JHT/JP/JKK/JKM perusahaan = 0.
  - `bpjs_kes_enabled = false` → BPJS Kes karyawan dan perusahaan = 0.
  - Tapera tidak berubah (tetap mengikuti kepesertaan per struktur; saat ini 0 peserta).
  - Subsidi BPJS Kesehatan tidak berubah.
  - Kepesertaan per struktur gaji dan tarif tetap tersimpan; saklar global hanya mengalahkan.
- **R3** Saklar menyala lagi → perhitungan kembali persis seperti sekarang (regresi nol, dibuktikan tes).
- **R4** Pengaturan Payroll mendapat 3 saklar di atas bagian BPJS dan PPh 21:
  - label: "PPh 21 dihitung", "BPJS Ketenagakerjaan dihitung", "BPJS Kesehatan dihitung";
  - catatan: "Nonaktif: tidak dipotong dari gaji dan tidak ada iuran perusahaan. Berlaku untuk payroll yang dihitung setelah disimpan.";
  - input tarif bagian yang nonaktif ditampilkan redup (disabled), nilainya tidak hilang;
  - perubahan diaudit seperti setting payroll lain.
- **R5** Slip (PDF, halaman slip HR, slip ESS) dan detail run menyembunyikan baris PPh 21, BPJS TK (JHT/JP), dan BPJS Kesehatan bila nilainya 0. Bagian iuran perusahaan juga disembunyikan bila totalnya 0.
- **R6** Run yang sudah disetujui/dibayar tidak dihitung ulang. Laporan biaya toko dan export memakai angka yang tersimpan, jadi tidak perlu perubahan.
- **R7** Struktur gaji (form Tambah/Ubah) menampilkan catatan kecil "Dinonaktifkan di Pengaturan Payroll" di samping kepesertaan BPJS/PPh bila saklar global mati. My call.

## Acceptance Criteria
- [ ] Setelah migrasi, payroll run baru untuk karyawan bergaji 5.000.000 (TK/0, ikut BPJS):
  - PPh 21 = 0, BPJS karyawan = 0, iuran perusahaan = 0;
  - THP = bruto − potongan lain;
  - Subsidi BPJS Kes tetap mengikuti setting.
- [ ] Menyalakan ketiga saklar → hasil sama persis dengan sebelum EPIC ini.
- [ ] Slip PDF, HR, dan ESS tidak menampilkan baris PPh 21/BPJS saat nonaktif.
- [ ] Pengaturan Payroll: 3 saklar tersimpan dan diaudit; HP 360 px tanpa scroll horizontal.

## Automation Log
- 2026-11-02 — Analisis acuan dan kode; data staging dicek (263 struktur aktif semuanya kena pajak dan BPJS). Jawaban user untuk 4 keputusan dicatat. Rencana: `docs/superpowers/plans/2026-11-02-pph21-bpjs-nonaktif.md`.
- 2026-11-02 — Kode selesai di branch `feat/epic082-pph21-bpjs-nonaktif`: saklar di kalkulator (`domain.Config.Pph21Enabled/BpjsTkEnabled/BpjsKesEnabled`), API + audit Pengaturan Payroll, UI saklar, slip tanpa baris nonaktif, migrasi `20261106090000` (ketiganya false). Regresi nol saat saklar menyala dibuktikan tes (`pph21_bpjs_toggle_test.go`, `pph21_bpjs_toggle_integration_test.go`). Acuan diperbarui (aturan-payroll, status-requirement PY-06, pertanyaan PPH-BPJS, skill paddy-hris-acuan). Keputusan: laporan biaya toko dan CSV run tetap punya kolom BPJS/PPh bernilai 0 (hanya slip yang menyembunyikan baris). Konsekuensi R2 yang diterima: bila HR menyalakan PPh 21 lagi di tengah tahun, Pasal 17 Desember menyusul PPh setahun penuh sekaligus (satu potongan besar di Desember). Sisa: staging (backup, deploy, simulasi run November sebelum/sesudah), PR, produksi hanya dengan persetujuan user.

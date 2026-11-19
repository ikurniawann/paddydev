# EPIC-118: Slip Gaji Ringkas, Password ESS, Edit Payroll per Karyawan (`MODULE-PAYROLL`)

status: coding
environment: dev
retries: 0

## Goal
Permintaan user 19-11-2026:

> payroll ketika akan edit karyawan satu satu harus nya bisa ya, skg di staging dan production payroll aksi cuman liat slip gaji aja lalu apa di slip gaji rincian toko di hilangkan aja dan tambahan di ringkasan bagian atas dengan info cuti unpaid dan cuti paid di tambahan diatas jadi card yang di bawah hilang aja
>
> ketika gaji di proses dan sudah disetujui, slip gaji karyawan ketika akan unduh harus ada keamanan input password karyawan bersangkutan demi keamanan ess karyawan lain agar tidak bisa di baca oleh user lain
>
> hilangkan keterangan label ini semua yang di ada keterangan di payslip contoh: R-01, R-02, R-03, R-05, R-90a

Keputusan user (AskUserQuestion 19-11):
- Edit per karyawan: "Komponen + hitung ulang" (hitung ulang karyawan itu saja).
- Password: "Rincian dan unduh" (daftar ESS hanya periode; angka, rincian, PDF sesudah password login benar; diperiksa server).
- Rilis slip ESS: "Sejak Disetujui" (status `completed` atau `paid`).
- Kode R-: "Semua tampilan" (ekspor CSV tetap memakai kode).
- Rilis production: "rilis prod nanti aja after office hours".

Rencana: [2026-11-19-slip-gaji-password-edit-payroll.md](../superpowers/plans/2026-11-19-slip-gaji-password-edit-payroll.md). Eksekusi subagent-driven (pilihan "118 subagent, 119 native").

## Tasks
- [x] Task 1: Hapus kode R- dari tampilan frontend
- [x] Task 2: Slip HR tanpa Rincian per toko dan tanpa empat kartu bawah
- [x] Task 3: PDF slip tanpa kode R- dan tanpa Rincian per Toko
- [ ] Task 4: Backend hitung ulang satu karyawan
- [ ] Task 5: Dialog Edit per karyawan
- [ ] Task 6: Slip terbit di ESS sejak Disetujui
- [ ] Task 7: Kunci slip ESS dengan password login (backend)
- [ ] Task 8: ESS Slip Gaji dengan dialog password
- [ ] Task 9: Gerbang akhir, review, PR, staging, rilis production sesudah jam kantor

## Automation Log
- 19-11-2026: rencana disetujui user ("sesuai"). Branch `feat/epic118-slip-gaji` (erp-paddy).
- Task 1 `7d5c586`, Task 2 `8e51ff7`, Task 3 `79f2e65`: lolos review per task.
- Ruling R4: tooltip `showSource` di slip HR masih memuat kode rule ("…; R-01"); slip HR berhenti mengirim `showSource` sehingga tooltip hilang ("Semua tampilan").

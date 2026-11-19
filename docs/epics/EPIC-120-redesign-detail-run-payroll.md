# EPIC-120: Redesign Halaman Detail Run Payroll (`MODULE-PAYROLL`)

status: review
environment: dev
retries: 0

## Goal
Permintaan user 19-11-2026 (dengan 4 tangkapan layar staging, run November 2026 Paddy Group, 237 karyawan):

> saya masih kurang enak dengan UI di sini https://paddydev.reddie.id/dashboard/hris/payroll/fdbcd542-d45e-46b0-95e4-51097490e084
> masih seperti UI generate banget ga informatif gada pagination filter ga lengkap list di detail payroll nya
> yang perlu saya kritisi UI ini dan bisa coba lebih enak informatif dan cara user handle

Rencana: [2026-11-19-redesign-detail-run-payroll.md](../superpowers/plans/2026-11-19-redesign-detail-run-payroll.md).

## Kritik UI saat ini
Kode: `apps/frontend/src/features/hris/payroll/components/payroll-detail-page.tsx` (erp-paddy, `6cf7944`).

1. **Urutan halaman tidak mengikuti kerja HR.** Isi halaman bertumpuk dengan bobot sama: peringatan periode, 3 total, langkah, form Komponen Manual, Hitung Payroll, 30+ kartu toko, tabel karyawan, Rekap per Kode Rule. Daftar karyawan (pekerjaan utama HR: memeriksa slip) baru muncul sesudah form dan semua kartu toko.
2. **Form Komponen Manual memakan layar paling atas.** Form tambah selalu terbuka, padahal sejak EPIC-118 tombol Edit per karyawan sudah mengerjakan hal yang sama untuk satu orang. Daftar komponen yang sudah diisi tidak bisa dicari.
3. **Alokasi Biaya per Toko berupa 30+ kartu.** Tidak ada urutan, tidak ada total, dan "+ beban perusahaan Rp 0" berulang di setiap kartu. Membandingkan toko berarti menggulir.
4. **Tabel karyawan tanpa alat kerja.** 237 baris sekaligus, tanpa pencarian nama/NIP, filter departemen/toko, urutan, atau pagination. Kembali dari slip mengulang dari atas halaman.
5. **Baris tidak menjelaskan angkanya.** Hanya nama, departemen, toko, pendapatan, potongan, THP. Kehadiran, terlambat, cuti dibayar/tanpa bayaran, komponen manual, dan toko tutup sudah tersimpan di `hris.payroll_details` tetapi tidak dikirim ke halaman. HR tidak bisa melihat kenapa THP seseorang turun tanpa membuka slipnya satu per satu.
6. **Hal yang perlu dicek tidak terkumpul.** Peringatan hasil Hitung (toko tutup, tanpa toko, dilewati) hanya tampil sesaat sesudah klik Hitung dan hilang saat halaman dimuat ulang.
7. **Tombol Process terkunci tanpa alasan terlihat.** Alasannya hanya ada di tooltip; peringatan periode absensi ada di kotak terpisah jauh di atas.
8. **Waktu hitung terakhir tidak tampil.** HR tidak tahu apakah angka sudah memuat perubahan komponen manual.

## Keputusan desain (diusulkan, menunggu review user)
- Urutan baru: header (status, waktu hitung terakhir) → bilah status (langkah + tombol berikut + alasan terkunci + Hitung ulang) → 5 kartu angka (Karyawan, Pendapatan, Potongan, THP, Perlu dicek) → tab **Karyawan** | **Komponen Manual** | **Biaya per Toko** | **Rekap Rule**.
- Tab Karyawan: cari nama/NIP, filter departemen, toko, dan tanda; urutan; 25/50/100 baris per halaman; total baris yang tersaring; kolom Kehadiran; badge tanda per baris.
- Filter, halaman, dan tab tersimpan di URL, jadi kembali dari slip membuka posisi yang sama.
- "Perlu dicek" = tanpa toko, toko tutup, THP Rp 0, cuti tanpa bayaran, komponen manual.
- Komponen Manual: daftar dengan pencarian; form tambah pindah ke dialog; impor CSV tetap.
- Biaya per Toko: tabel yang bisa diurutkan dengan baris total; kolom beban perusahaan hanya bila ada nilainya.
- Tidak ada kode R- di tampilan (EPIC-118).
- Di luar cakupan: daftar karyawan yang dilewati saat Hitung tidak tersimpan di database, jadi tetap hanya tampil sesudah klik Hitung. Perbandingan dengan bulan lalu juga tidak termasuk.

## Tasks
- [x] Task 1: Ringkasan kehadiran dan tanda di detail run (Go + Next fallback)
- [x] Task 2: Fungsi murni tanda, filter, urut, halaman
- [x] Task 3: State filter di URL
- [x] Task 4: Tab Karyawan
- [x] Task 5: Tab Biaya per Toko sebagai tabel
- [x] Task 6: Tab Komponen Manual
- [x] Task 7: Susun ulang halaman detail
- [ ] Task 8: Gerbang akhir, PR, staging, QA HR

## Automation Log
- 19-11-2026: kritik dan rencana ditulis; mockup interaktif (`docs/mockup/epic120-detail-run-payroll.html`) disetujui user ("sesuai, kerjakan pakai subagent").
- Branch `feat/epic120-detail-run-payroll` (erp-paddy) dari `6cf7944`: Task 1 `6c43198`, Task 2 `ff4e5df`, Task 3 `67b3bf7`, Task 4 `1732d27..aa3dea0`, Task 5 `2a6d68f`, Task 6 `0f80bc9`, Task 7 `7af96bd..1a94b2f`, perbaikan review akhir `f0be0c8..0a7eb22`. Setiap task lolos review; review akhir 1 Important diperbaiki.
- Ruling: `employee.position` berbentuk `{title}` mengikuti embed yang ada.
- Ruling: alokasi kosong (`[]` atau tidak dikirim) = tidak diketahui, bukan "Tanpa toko"; slip sebelum fitur alokasi toko tidak lagi ditandai.
- Ruling: chip tanda dengan jumlah (sesuai mockup) menggantikan pilihan Tanda di rencana; "termasuk manual Rp X" per baris tidak dibuat karena nominal manual tidak ada di data baris.
- Ruling: filter disimpan lewat `history.replaceState` (tanpa request server per klik); ganti tab mempertahankan halaman.

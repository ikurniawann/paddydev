# EPIC-071: Pengatur Jadwal Toko — Jabatan Tertentu Mengatur Jadwal & Penempatan (`MODULE-HRIS`)

status: backlog
environment: dev
retries: 0

## Goal
Permintaan user 29-10-2026, sebelum EPIC-070 di-deploy ke production: **jabatan tertentu** bisa mengunggah atau mengatur **jadwal kerja** dan **penempatan kerja** karyawan, tidak hanya HR. Daftar yang diberikan user (screenshot):

| Baris di daftar user | Data production (read-only 29-10) |
|---|---|
| Store Supervisor - Jabodetabek 1 | 1 orang, toko sendiri PPJA, 28 bawahan langsung di 11 toko |
| PPBA Shopkeeper Coordinator | Shopkeeper Coordinator di ODOO-PPBA (5 bawahan) |
| PBSA Shopkeeper Coordinator | Shopkeeper Coordinator di ODOO-PBSA (6 bawahan) |
| Store Supervisor - Makassar | 1 orang, PPMR, 11 bawahan (PPMR, PLMR) |
| General Affair Supervisor | "GA Supervisor", PADDY-PUDAK, 4 bawahan (PUDAK, SALAM, WAREHOUSE) |
| Warehouse Supervisor | 1 orang, WAREHOUSE-PADDY, 56 bawahan |
| PPSU Shopkeeper (Coordinator) | "Shopkeeper Staff Coordinator" di ODOO-PPSU (5 bawahan) |
| PPAY Shopkeeper Staff | 5 Shopkeeper Staff di PPAY; hanya 1 yang punya bawahan (8) |
| PPPA Shopkeeper Staff | 4 Shopkeeper Staff di PPPA; hanya 1 yang punya bawahan (4) |
| PPVJ Shopkeeper Staff (×2) | 5 Shopkeeper Staff di PPVJ; 1 punya bawahan (5) |

Semua orang di atas sudah punya akun login, dengan role aplikasi `employee` (Area Karyawan).

Temuan penting: nama jabatan di sistem **tidak** memuat kode toko. Ada 121 "Shopkeeper Staff" dan 7 "Shopkeeper Coordinator". Akses berbasis judul jabatan akan membuka akses ke ratusan orang yang tidak dimaksud. Jadi akses diberikan **per orang**, dan HR memilih orangnya.

Yang sudah ada sebelumnya: ESS **Shift Tim** (`/dashboard/me/shift-tim`). Atasan langsung mengatur pola mingguan bawahannya **satu per satu**. Tidak ada grid, tidak ada unggah Excel, dan tidak ada penempatan.

## Ruling (dibuat tanpa tanya; mohon dicek saat review rencana)
- **R1 — Akses per orang, bukan per judul jabatan.**
  - HR menunjuk karyawan sebagai **Pengatur Jadwal Toko**, dengan daftar **toko wilayahnya** dan centang **boleh atur penempatan**.
  - Alasan: lihat temuan di atas.
  - Bila salah: HR menambah orang satu per satu (±11 orang), tidak ada data yang rusak.
- **R2 — Wilayah = daftar toko yang dipilih HR.**
  - Saat menambah, toko diisi otomatis: toko penempatan berjalan orang itu ∪ toko penempatan berjalan seluruh bawahannya (langsung maupun tidak langsung, sampai 10 tingkat).
  - HR bisa menambah atau mengurangi toko.
  - Toko nonaktif tidak bisa dipilih dan tidak ikut terhitung.
- **R3 — Karyawan dalam wilayah** = karyawan aktif yang punya penempatan berjalan di salah satu toko wilayah, **atau** bawahan (langsung/tidak langsung) si pengatur.
  - Bawahan yang belum ditempatkan bisa ditempatkan oleh atasannya.
  - Karyawan lain yang belum ditempatkan tetap urusan HR.
- **R4 — Jadwal.** Pengatur memakai grid Jadwal Kerja EPIC-070 yang sama (kotak, rentang, pola, salin minggu lalu, unduh template, unggah Excel), tetapi hanya untuk toko wilayahnya.
  - Validasi server tetap sama: tanggal lampau, periode closing, dan hari karyawan tidak ditempatkan di toko itu ditolak.
  - Akibatnya pengatur tidak bisa menjadwalkan orang di luar tokonya.
- **R5 — Menimpa/mengosongkan jadwal per tanggal.**
  - Pengatur **boleh** dalam wilayahnya, dengan **alasan wajib** dan baris audit yang sama (`clearGuard`). Ia tidak perlu hak hapus Jadwal Kerja.
  - Alasan: tanpa ini pengatur tidak bisa memperbaiki jadwal yang sudah diisi. Itu keluhan utama EPIC-070 ("tidak cepat perubahannya").
  - Bila salah: jadwal mendatang di toko itu bisa ditimpa pengatur. Semuanya teraudit beserta alasan, dan HR bisa mencabut aksesnya.
- **R6 — Penempatan** (hanya bila dicentang "boleh atur penempatan"):
  - Pindahkan / Tambah toko: semua karyawan terpilih harus dalam wilayah, dan toko tujuan harus toko wilayah. Bila tidak → 403 untuk seluruh permintaan.
  - Pindah: karyawan yang punya penempatan berjalan di toko **di luar** wilayah dan **bukan** bawahannya dilewati dengan alasan "Punya penempatan di toko lain di luar wilayah Anda — hubungi HR".
  - Akhiri: hanya mengakhiri penempatan di toko wilayah. Karyawan tanpa penempatan di toko wilayah dilewati.
  - Ubah/hapus satu baris penempatan, lokasi toko, dan jam minimum tetap khusus HR.
- **R7 — Yang tetap khusus HR:** jadwal tetap Non-Shift, ubah tipe jadwal, master Shift Kerja, lokasi/geofence toko, dan halaman HR `/dashboard/hris/*`.
- **R8 — Tempat kerja pengatur:** menu baru Area Karyawan **Jadwal Toko** (`/dashboard/me/jadwal-toko`) dengan tab **Jadwal** dan **Penempatan** (tab Penempatan hanya bila boleh). Halaman ini bisa dipakai dari HP.
  - Menu hanya tampil untuk pengatur aktif.
  - Menu ESS lain tetap seperti sekarang. Shift Tim tetap ada.
- **R9 — Kelola pengatur oleh HR:** halaman **Pengatur Jadwal Toko** (`/dashboard/hris/placements/pengatur`), dibuka dari tombol di halaman Penempatan Karyawan. Hak yang dipakai sama dengan penempatan (`placementManagers`); simpan/cabut memakai aksi `update`.
  - Syarat: karyawan aktif dan punya akun login. Bila tidak → pesan "Karyawan belum punya akun login".
  - Setiap simpan/cabut diaudit (`schedule_delegate.set`, `schedule_delegate.revoke`).
- **R10 — Data awal:** tidak ada skrip seed.
  - HR menambah orang dari daftar user lewat halaman R9, karena untuk Shopkeeper Staff PPAY/PPPA/PPVJ hanya HR yang tahu orang yang dimaksud.
  - Halaman R9 menampilkan jabatan, toko, dan jumlah bawahan supaya mudah dicocokkan.
- **R11 — Karyawan nonaktif:** pengatur yang dinonaktifkan otomatis kehilangan akses, karena dicek `is_active` setiap permintaan. Barisnya tetap tersimpan. Bila diaktifkan kembali, aksesnya kembali.
- **R12 — Deploy:** EPIC-070 dan EPIC-071 di-deploy ke production bersama, setelah EPIC-071 lulus QA staging dan user memberi izin deploy production.

## Tasks
Lihat rencana `docs/superpowers/plans/2026-10-29-pengatur-jadwal-toko.md`.

## Acceptance Criteria
- HR menunjuk seorang Shopkeeper Coordinator sebagai pengatur untuk tokonya. Toko terisi otomatis dari penempatan dan bawahan, dan bisa diubah.
- Pengatur login di HP dan melihat menu **Jadwal Toko**. Karyawan lain tidak melihat menu itu.
- Pengatur bisa mengubah satu kotak, menerapkan pola mingguan, salin minggu lalu, dan mengunggah Excel untuk tokonya. Toko lain tidak tersedia, dan permintaan langsung ke API untuk toko lain → 403.
- Menimpa jadwal per tanggal oleh pengatur meminta alasan dan tercatat di audit.
- Pengatur dengan hak penempatan bisa memindahkan karyawan antar toko wilayahnya dan menempatkan bawahan yang belum ditempatkan. Karyawan/toko di luar wilayah → 403 atau dilewati dengan alasan, sesuai R6.
- HR mencabut akses → menu hilang dan API → 403.
- Endpoint dan halaman HR EPIC-070 berperilaku sama (tes lama tetap hijau).
- `go test ./...`, `pnpm test`, `pnpm lint`, dan `tsc` hijau; CI 4/4 hijau; QA staging lulus.

## Automation Log
- 2026-10-29: **Permintaan user** + screenshot daftar 11 jabatan.
  - Pemetaan kode main `940be473`:
    - akses jadwal/penempatan hanya lewat menu IAM HR (global, tanpa wilayah);
    - akun punya scope satu cabang saja;
    - ESS Shift Tim = atasan langsung, pola mingguan per orang.
  - Data production dibaca read-only (tabel di Goal).
  - Rencana ditulis, status `backlog` menunggu review user.

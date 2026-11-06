# EPIC-093: Koordinator Mengatur Jadwal Semua Toko (`MODULE-HRIS`)

status: backlog
environment: dev
retries: 0

## Goal
Revisi user 06-11-2026 atas EPIC-071:

> "Setiap Koordinator bisa membuat jadwal toko lain, tidak hanya toko yang dia pegang. dan tidak perlu persetujuan SPV, karena sifatnya hanya setting jadwal"

Kondisi saat ini (main `614d3694`): pengatur jadwal toko hanya boleh membuka grid, template, unggah, dan simpan kotak untuk **toko wilayahnya**. Toko lain ditolak 403 "Toko di luar wilayah Anda" (EPIC-071 R4). Perubahan jadwal oleh pengatur sudah **langsung berlaku**: tidak ada alur persetujuan jadwal di sistem, hanya audit.

Data production (read-only 06-11):

| Kelompok | Jumlah |
|---|---|
| Pengatur aktif | 21 |
| Pengatur berjabatan Coordinator | 9: 6 Shopkeeper Coordinator, 1 Shopkeeper Staff Coordinator, 1 Host Live Coordinator, 1 Case Production Coordinator |
| Berjabatan Coordinator/Koordinator tapi bukan pengatur | 4, jabatan non-toko: Content Creator Koordinator, Inbound, Logistics, Outbound Coordinator |
| Level jabatan semua koordinator | Staff (tidak ada level "Koordinator") |
| Toko aktif | 45 |

Keputusan user (06-11):
- **Siapa:** HR memberi centang **"Boleh atur jadwal semua toko"** per pengatur. Saat deploy, centang dipasang untuk 9 pengatur koordinator, dengan izin user.
- **Cakupan:** **jadwal saja.** Penempatan tetap hanya di toko wilayahnya.

## Ruling
- **R1 — Centang per pengatur.** Kolom baru `schedule_all_branches`, default mati. Akses tetap per orang, tidak per judul jabatan (sama dengan EPIC-071 R1). Bila salah: HR mencentang atau mencabut sendiri, tanpa data rusak.
- **R2 — "Jadwal" mencakup** grid, ubah kotak, pola, salin minggu lalu, unduh template, dan unggah Excel, untuk **semua toko aktif**. Validasi server tetap sama: tanggal lampau, closing, roster toko itu (R18), dan alasan wajib saat menimpa (R5).
- **R3 — Tanpa persetujuan.** Jadwal yang disimpan pengatur di toko mana pun langsung berlaku. Jejaknya tetap di audit `schedule.cells_update` / `hris.schedule.replace` atas nama pengatur. Tidak ada notifikasi ke SPV toko tujuan, karena user tidak memintanya.
- **R4 — Tetap wilayah sendiri:** penempatan (cari, pindah, tambah, akhiri) dan aturan **checkout otomatis**. Checkout otomatis adalah aturan absensi, bukan jadwal. Bila salah: satu baris kode untuk membukanya.
- **R5 — Wilayah tetap wajib.** Pengatur bercentang tetap harus punya minimal satu toko wilayah aktif. Tanpa itu ia bukan pengatur, sama dengan aturan menu sidebar EPIC-071.
- **R6 — Pilihan toko di tab Jadwal** memuat semua toko aktif, urut kode. Pengatur dengan satu toko wilayah tetap langsung melihat tokonya. Tautan "ke tab Penempatan" hanya muncul di toko wilayah.
- **R7 — Toko nonaktif** tidak muncul dan tidak bisa dibuka (403).
- **R8 — Data production.** Centang untuk 9 pengatur koordinator dipasang sesudah deploy production, atas izin terpisah: satu transaksi, audit per orang, uji kering dulu. 3 Shopkeeper Staff kepala toko (PPAY/PPPA/PPVJ) dan Store Supervisor **tidak** dicentang, kecuali HR menambahkannya.

## Tasks
Lihat rencana `docs/superpowers/plans/2026-11-06-jadwal-semua-toko.md`.

## Acceptance Criteria
- [ ] HR mencentang "Boleh atur jadwal semua toko" pada seorang pengatur. Kolom "Jadwal semua toko" menampilkan Ya, dan audit memuat nilainya.
- [ ] Pengatur bercentang membuka Jadwal Toko di HP. Daftar toko memuat toko lain, dan tokonya sendiri terpilih otomatis.
- [ ] Pengatur bercentang mengubah jadwal di toko lain. Jadwal langsung tersimpan tanpa persetujuan, dan tercatat di audit.
- [ ] Penempatan dan checkout otomatis tetap hanya toko wilayah. API penempatan untuk toko lain → 403.
- [ ] Pengatur tanpa centang berperilaku persis seperti EPIC-071 (tes lama hijau).
- [ ] `go test`, `pnpm test`, `pnpm lint`, dan `tsc` hijau; CI hijau; QA staging lulus.

## Automation Log
- 2026-11-06: **Permintaan user** (revisi Koordinator). Kode dipetakan (`withScheduleScope`, `AllowsBranch` di 4 rute jadwal, penempatan, checkout otomatis). Data production dibaca read-only (tabel di Goal). User memilih centang per pengatur oleh HR dan cakupan jadwal saja. Rencana ditulis; status `backlog` menunggu review user.

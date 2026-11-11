# EPIC-105: Tutup Risiko Data dari Minor yang Ditunda (`MODULE-HRIS`, `MODULE-PROCUREMENT`)

status: backlog
environment: dev
retries: 0

## Goal
Pilihan user 11-11-2026: "Risiko data dulu". Menutup minor yang ditunda dari EPIC-095, 098, 099 dan 100 yang bisa merusak data atau gagal di production, plus dua tes flaky. Tidak ada fitur baru dan tidak bergantung pada jawaban klien.

ORG-04 (approver posisi, Q6) dan minor kosmetik lainnya tetap ditunda.

## Ruling
- **R1 — Nomor urut NIP tidak boleh ganda** (EPIC-099).
  - Nomor urut NIP berlaku global, bukan per bulan. Saat ini dua pembuatan bersamaan di bulan berbeda bisa sama-sama lolos (misalnya 202601521 dan 202602521), karena indeks unik hanya melindungi string NIP utuh.
  - Perbaikan: setiap penulisan NIP baru (buat karyawan HRIS, buat akun, rekrutmen → karyawan, impor Excel) mengambil `pg_advisory_xact_lock` dengan kunci tetap sebelum membaca nomor terakhir, di transaksi yang sama dengan INSERT-nya. Proses kedua menunggu, lalu ditolak oleh aturan nomor urut yang sudah ada.
  - **Koreksi 11-11 (temuan review):** jalur ubah NIP tidak termasuk. Jalur itu hanya mengecek format dan YYYYMM (`CheckChanged`), tanpa aturan nomor urut, sehingga lock tidak menambah perlindungan. Kalau klien ingin nomor urut ditegakkan juga saat NIP diubah, itu keputusan baru.
- **R2 — Impor Kode & Status Toko berurutan** (EPIC-100).
  - Langkah simpan mengambil advisory lock `branch-code-import`, sehingga dua impor bersamaan berjalan bergiliran.
  - Deadlock (`40P01`) atau lock timeout (`55P03`) di impor toko dan penempatan massal dijawab 409 `Data sedang diubah proses lain — coba lagi sebentar`, bukan 500.
- **R3 — Tidak ada kode toko yang tertanam di penerimaan barang (GRN)** (EPIC-100).
  - Fallback `MIZU` / `MIZU-BANDUNG` dihapus dari `receivingScope` (Go) dan `grn-create.ts` (TS).
  - Bila cakupan gudang kosong (gudang, toko, atau perusahaannya tidak aktif atau tidak ada), GRN ditolak 400 `Gudang tidak terhubung ke toko dan perusahaan yang aktif`. Gudang aktif di toko aktif tidak berubah perilakunya.
  - Setelah ini kode `MIZU-BANDUNG` aman diganti lewat template.
  - Cek read-only salinan production 09-11:
    - 13 dari 58 gudang aktif berada di toko nonaktif (mis. ODOO-MATS, PD-KOPO). GRN ke gudang-gudang ini akan ditolak.
    - Tabel `purchasing.grn` masih kosong, jadi belum ada alur kerja yang terdampak. **Keputusan perencanaan, perlu dicek user:** sebelumnya GRN untuk gudang di toko nonaktif diberi cakupan MIZU-BANDUNG.
- **R4 — Pembatalan otomatis resign terjadwal tercatat di audit** (EPIC-095). Job yang membatalkan jadwal karena karyawan sudah nonaktif, atau karena validasinya gagal, menulis `employee.resign_schedule_cancel` dengan aktor `Sistem (resign terjadwal)` dan alasannya. Pembatalan oleh trigger saat karyawan dinonaktifkan tetap tanpa baris audit tambahan, karena penonaktifan itu sendiri sudah tercatat.
- **R5 — Impor karyawan menolak siklus atasan** (EPIC-098).
  - Rantai atasan dihitung dari isi file ditambah atasan yang sudah ada di data.
  - Baris yang membuat siklus (A→B→A, atau rantai yang kembali ke dirinya) mendapat error `NIP Atasan membentuk siklus: <NIP> → <NIP> → <NIP>`.
  - Tidak ada yang tersimpan selama masih ada error, sama seperti error impor lainnya.
- **R6 — Tes flaky distabilkan, tanpa mengubah perilaku produk.**
  - Tes rate limit memberportal memakai jam tetap.
  - `TestPruneDropsExpiredKeys` mengulang Prune + Peek sampai kunci hilang (maks. ±1 detik), karena Prune melewati baris yang sedang dikunci proses lain (`SKIP LOCKED`).

## Tasks
Rencana: `docs/superpowers/plans/2026-11-11-risiko-data-minor.md`.
- [ ] T1 Kunci nomor urut NIP
- [ ] T2 Impor toko berurutan + deadlock → 409
- [ ] T3 GRN tanpa kode toko tertanam
- [ ] T4 Audit pembatalan resign terjadwal
- [ ] T5 Impor karyawan menolak siklus atasan
- [ ] T6 Tes flaky
- [ ] T7 Gate, review, QA staging, catatan, PR

## Automation Log
- 2026-11-11: Epic dibuat dari minor EPIC-095/098/099/100 (pilihan "Risiko data dulu"). Fakta dikumpulkan agen pencari. Branch `feat/epic105-risiko-data` dari `main` 4b113831.

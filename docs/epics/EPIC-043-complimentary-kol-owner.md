# EPIC-043 — Complimentary Orders (KOL & Owner)

status: ready-for-qa

**Status:** Fase 1+2 selesai (2026-08-24)
**Keputusan owner:** 2026-08-23 — transaksi gratis harus tetap tercatat penuh,
teridentifikasi jenisnya, dan ada jejak siapa yang menyetujui.

## Masalah

Transaksi gratis selama ini memakai "diskon manual 100%" — order tercatat,
tapi tanpa identitas (KOL? owner? salah pencet?), tanpa persetujuan tercatat,
dan laporan tidak bisa memisahkan omzet asli dari barang komplimen.

## Desain

Order komplimen tercatat LENGKAP: item, subtotal gross, diskon = subtotal,
total 0, dibayar 0 — plus `comp_type` dan `comp_approved_by/name`.

### KOL Comp (`kol_comp`)
- Customer bertanda `pos_customers.is_kol` (dikelola via SQL/agent — bukan
  dari kasir) + kuota bulanan opsional `kol_monthly_limit_idr` (nilai GROSS,
  NULL = tanpa batas).
- Kasir menggratiskan order KOL (diskon 100%) → checkout otomatis distempel
  `kol_comp` (use-pos-checkout mengirim `comp_type` bila customer KOL dan
  total 0). Server MEMVALIDASI: flag is_kol, kuota bulan berjalan, dan total
  harus 0 — bukan kepercayaan ke klien. Berlaku utk order biasa (POST
  /api/pos/orders) dan checkout multi-stall (POST /api/pos/checkouts).
- Lewat kuota → ditolak dengan angka jelas (terpakai/limit/order ini).

### Owner Comp (`owner_comp`)
- Alur: open bill (dapur jalan) → struk PREVIEW BILL kini memuat blok tanda
  tangan ("Disetujui: ____ / Nama: ____") → bila diputuskan gratis, kasir
  memakai tombol **Owner Comp** di detail order (halaman Orders, bill belum
  bayar) → wajib PIN supervisor (padanan digital tanda tangan; struk fisik
  ber-TTD diarsip).
- Server (PATCH /api/pos/orders/:id): verifikasi PIN, gratiskan seluruh bill
  (diskon = subtotal, total & dibayar 0), stamp `owner_comp` + penyetuju.
  Guard nominal pelunasan dilewati khusus jalur ini (total memang berubah).

### Struk
- Customer copy komplimen: baris Bayar/Kembalian diganti label
  "KOL COMPLIMENTARY — GRATIS" / "OWNER COMP — Disetujui: <nama>".
  Reprint (termasuk keluarga checkout multi-stall) membawa label yang sama.
- PREVIEW BILL: blok tanda tangan selalu tercetak.

### Laporan
- Daily Flash Report: baris "KOL Comp : Rp X (N Trx)" dan "Owner Comp : …"
  (nilai GROSS; muncul hanya bila ada) di bawah "Disc 100%".

## Migrasi

`database/migrations/deltas/20260916100500_pos_comp_orders_port_from_legacy.sql`
(dulu `migrations/015_comp_orders.sql`; folder legacy dihapus 2026-09-14 tetapi
berkas ini baru diport 2026-09-16 setelah live gagal "column is_kol does not
exist") — dijalankan lewat `node database/scripts/apply-migrations.js --apply`:
pos_customers.is_kol + kol_monthly_limit_idr; pos_orders.comp_type +
comp_approved_by + comp_approved_name (+index partial).
Menandai KOL: `UPDATE pos.pos_customers SET is_kol=true,
kol_monthly_limit_idr=<plafon|NULL> WHERE phone='…';`

## Belum dikerjakan (fase berikutnya)
- UI kelola flag KOL & kuota (saat ini via SQL / Open API token).
- Badge "KOL — GRATIS" di layar kasir saat customer KOL dipilih
  (auto-diskon di UI; kini kasir tetap menekan diskon 100%, stempel &
  validasi otomatis).
- Laporan bulanan komplimen per orang (evaluasi owner).

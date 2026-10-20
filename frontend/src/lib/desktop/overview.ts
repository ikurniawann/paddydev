import { query, queryOne } from "@/lib/db";
import {
  projectRunRate,
  summarizePeriod,
  type Comparison,
  type Period,
  type PeriodKind,
  type PeriodSummary,
} from "./period";

import {
  breakdownRevenue,
  promoEfficiency,
  type RevenueBreakdown,
} from "./revenue";

export type { PeriodSummary } from "./period";

/**
 * Data papan monitoring desktop NüHabit OS (EPIC-019 Fase A).
 *
 * Prinsip:
 * - Definisi angka MENGIKUTI modulnya, bukan menulis ulang: pending approval
 *   memakai definisi nav-badges (`status='pending'` pada tabel yang sama),
 *   penjualan memakai filter yang sama dengan tool Do `penjualan_periode`,
 *   stok memakai query `stok_menipis`. Selisih angka desktop vs halaman modul
 *   adalah bug, bukan "beda definisi".
 * - Tiap seksi gagal-aman: satu query error → seksi itu bernilai null dan
 *   dicatat, seksi lain tetap terisi. Desktop tidak boleh mati karena satu tabel.
 */

export interface SalesPulse {
  hariIni: { omzet: number; pesanan: number; rataRata: number };
  /**
   * Laba kotor hari ini = omzet − COGS dari snapshot harga modal per item.
   * BUKAN laba bersih: beban operasional (gaji/sewa/listrik) hanya ada di
   * jurnal akuntansi, tidak bisa dihitung harian.
   * `itemTanpaModal` > 0 berarti sebagian produk belum punya harga modal,
   * sehingga labanya optimistis palsu — widget menandainya, tidak diam saja.
   */
  labaKotorHariIni: { laba: number; itemTanpaModal: number } | null;
  kemarin: { omzet: number; pesanan: number };
  /** Hari yang sama minggu lalu (H-7) — pembanding pola mingguan (Fase C). */
  mingguLalu: { omzet: number; pesanan: number };
  /** Omzet per hari, 7 hari terakhir (indeks 6 = hari ini) — bahan sparkline. */
  tujuhHari: Array<{ tanggal: string; omzet: number }>;
}

export interface TeamToday {
  aktif: number;
  hadir: number;
  terlambat: number;
  cuti: number;
  belum: number;
}

export interface PendingDecisions {
  cuti: number;
  lembur: number;
  pinjaman: number;
  poDraft: number;
  kandidatBaru: number;
  total: number;
}

export interface LowStockItem {
  bahan: string;
  tersedia: number;
  minimum: number;
}

export interface LowStock {
  jumlah: number;
  teratas: LowStockItem[];
}

export interface CrmPulse {
  memberBaru7Hari: number;
  xpTerdistribusi7Hari: number;
  rewardDitukar7Hari: number;
}

/**
 * Omzet sepanjang periode terpilih — dasar Revenue Overview (Fase B).
 *
 * `adaData` sengaja dipisah dari `omzet: 0`: tanpa itu, periode yang belum punya
 * transaksi terbaca sebagai "penjualan nol" oleh owner, padahal artinya "belum
 * ada yang tercatat". Dua hal berbeda dengan tindak lanjut berbeda.
 */
export interface RevenuePeriod {
  omzet: number;
  pesanan: number;
  /** Komposisi sumber pendapatan berikut porsinya (Fase B). */
  sumber: RevenueBreakdown["sumber"];
  banding: { omzet: number; pesanan: number };
  /** Proyeksi akhir periode dari laju berjalan. */
  proyeksi: number;
  adaData: boolean;
}

/**
 * Dampak promo: diskon yang dikeluarkan vs omzet yang dibawanya (Fase B).
 *
 * Hanya redemption `captured` yang dihitung — `held` masih reservasi yang bisa
 * batal, `released` sudah batal. Menjumlahkan ketiganya melebih-lebihkan diskon
 * yang benar-benar keluar.
 */
export interface PromoImpact {
  diskon: number;
  omzetTerbawa: number;
  redemption: number;
  /** Rupiah omzet per rupiah diskon; null bila belum ada diskon sama sekali. */
  efisiensi: number | null;
  teratas: Array<{ kampanye: string; diskon: number; omzet: number; redemption: number }>;
  adaData: boolean;
}

/**
 * Tamu yang sedang duduk (EPIC-038).
 *
 * Asumsi yang dipakai: satu bill terbuka = satu rombongan. Kalau satu meja
 * punya dua bill terpisah, tamunya dijumlahkan — itu memang dua rombongan yang
 * kebetulan berbagi meja. `meja` menghitung meja UNIK supaya tidak ikut ganda.
 */
export interface GuestsSeated {
  tamu: number;
  meja: number;
  /** Total kursi pada meja-meja yang sedang terisi — pembanding okupansi. */
  kapasitas: number;
  adaData: boolean;
}

export interface DesktopOverview {
  dibuatPada: string;
  /** Metadata periode papan — dipakai UI untuk melabeli angka & pembandingnya. */
  periode: PeriodSummary;
  omzetPeriode: RevenuePeriod | null;
  dampakPromo: PromoImpact | null;
  tamuDiMeja: GuestsSeated | null;
  pulsaBisnis: SalesPulse | null;
  timHariIni: TeamToday | null;
  perluKeputusan: PendingDecisions | null;
  stokMenipis: LowStock | null;
  member: CrmPulse | null;
  /** Seksi yang gagal dimuat — dipakai UI untuk menandai kartunya. */
  gagal: string[];
}

/** Tanggal operasional WIB — tanggal server belum tentu sama. */
export function todayJakarta(now = new Date()): string {
  return new Date(now.getTime() + 7 * 60 * 60 * 1000).toISOString().slice(0, 10);
}

async function fetchSalesPulse(): Promise<SalesPulse> {
  const today = todayJakarta();
  // Omzet = uang yang BENAR-BENAR masuk (keputusan owner 2026-08-19).
  // Definisi disamakan dengan Laporan Profit & tool Do `penjualan_periode`:
  //   payment_status='paid' AND status NOT IN (cancelled, voided, merged)
  // Sebelumnya filter di sini tidak memeriksa pembayaran sama sekali, jadi
  // order yang belum dibayar ikut terhitung sebagai omzet — dan 'merged'
  // (checkout multi-stall yang digabung) terhitung dobel.
  // Memakai ordered_at, bukan created_at: laporan profit memakai kolom itu,
  // dan itu waktu transaksi kasir yang sebenarnya.
  // Rentang -7 hari (bukan -6) supaya hari yang sama minggu lalu ikut terambil
  // untuk pembanding mingguan; sparkline tetap memakai 7 titik terakhir.
  const rows = await query<{ tanggal: string; omzet: string; pesanan: string }>(
    // ordered_at NULLABLE: COALESCE ke created_at supaya order lama tidak
    // hilang diam-diam dari omzet hanya karena kolomnya kosong.
    `SELECT (COALESCE(ordered_at, created_at) AT TIME ZONE 'Asia/Jakarta')::date::text AS tanggal,
            COALESCE(sum(total_amount), 0)::float8 AS omzet,
            count(*)::int AS pesanan
       FROM pos.pos_orders
      WHERE COALESCE(ordered_at, created_at) >= ($1::date - interval '7 days')
        AND COALESCE(ordered_at, created_at) < ($1::date + interval '1 day')
        AND payment_status = 'paid'
        AND status::text NOT IN ('cancelled', 'voided', 'merged')
      GROUP BY 1
      ORDER BY 1`,
    [today]
  );

  const byDate = new Map(rows.map((r) => [r.tanggal, { omzet: Number(r.omzet), pesanan: Number(r.pesanan) }]));
  const tujuhHari: SalesPulse["tujuhHari"] = [];
  for (let i = 6; i >= 0; i--) {
    const d = new Date(new Date(`${today}T00:00:00Z`).getTime() - i * 86_400_000)
      .toISOString()
      .slice(0, 10);
    tujuhHari.push({ tanggal: d, omzet: byDate.get(d)?.omzet ?? 0 });
  }

  const hariIni = byDate.get(today) ?? { omzet: 0, pesanan: 0 };
  const kemarinTgl = tujuhHari[5]?.tanggal ?? today;
  const kemarin = byDate.get(kemarinTgl) ?? { omzet: 0, pesanan: 0 };
  const mingguLaluTgl = new Date(new Date(`${today}T00:00:00Z`).getTime() - 7 * 86_400_000)
    .toISOString()
    .slice(0, 10);
  const mingguLalu = byDate.get(mingguLaluTgl) ?? { omzet: 0, pesanan: 0 };

  // Laba kotor hari ini dari snapshot biaya per item (kolom yang sama dipakai
  // Laporan Profit). Gagal query = null, bukan 0: "belum bisa dihitung" beda
  // artinya dengan "tidak untung", dan widget harus jujur soal itu.
  let labaKotorHariIni: SalesPulse["labaKotorHariIni"] = null;
  try {
    const [laba] = await query<{ laba: string; item_tanpa_modal: string }>(
      `SELECT COALESCE(sum(i.total_amount - COALESCE(i.cost_total, 0)), 0)::float8 AS laba,
              count(*) FILTER (WHERE COALESCE(i.cost_total, 0) = 0)::int AS item_tanpa_modal
         FROM pos.pos_order_items i
         JOIN pos.pos_orders o ON o.id = i.order_id
        WHERE (COALESCE(o.ordered_at, o.created_at) AT TIME ZONE 'Asia/Jakarta')::date = $1::date
          AND o.payment_status = 'paid'
          AND o.status::text NOT IN ('cancelled', 'voided', 'merged')`,
      [today]
    );
    if (laba) {
      labaKotorHariIni = {
        laba: Number(laba.laba) || 0,
        itemTanpaModal: Number(laba.item_tanpa_modal) || 0,
      };
    }
  } catch {
    labaKotorHariIni = null;
  }

  return {
    hariIni: {
      omzet: hariIni.omzet,
      pesanan: hariIni.pesanan,
      rataRata: hariIni.pesanan > 0 ? hariIni.omzet / hariIni.pesanan : 0,
    },
    labaKotorHariIni,
    kemarin: { omzet: kemarin.omzet, pesanan: kemarin.pesanan },
    mingguLalu: { omzet: mingguLalu.omzet, pesanan: mingguLalu.pesanan },
    tujuhHari,
  };
}

/**
 * Omzet periode + jendela pembandingnya dalam satu query.
 *
 * Filter mengikuti definisi modul POS (batal & void tidak dihitung), sama
 * seperti `fetchSalesPulse` dan tool Do `penjualan_periode` — selisih angka
 * papan vs halaman modul adalah bug, bukan beda definisi.
 *
 * Tanggal dibandingkan sebagai tanggal WIB (`AT TIME ZONE`), bukan rentang
 * timestamp mentah, supaya batas hari mengikuti hari operasional Jakarta.
 *
 * Tidak difilter tenant: papan digate ke super_admin/direksi dan memang
 * dimaksudkan lintas-unit — konsisten dengan seluruh query di modul ini.
 */
async function fetchRevenuePeriod(
  period: Period,
  banding: Comparison
): Promise<RevenuePeriod> {
  const rentang = [period.mulai, period.selesai, banding.mulai, banding.selesai];

  const [fnb, b2b] = await Promise.all([
    queryOne<{
      omzet: string;
      pesanan: number;
      banding_omzet: string;
      banding_pesanan: number;
      baris: number;
    }>(
      `WITH terpakai AS (
         SELECT total_amount,
                (created_at AT TIME ZONE 'Asia/Jakarta')::date AS tanggal
           FROM pos.pos_orders
          WHERE status <> 'cancelled'
            AND voided_at IS NULL
       )
       SELECT
         COALESCE(sum(total_amount) FILTER (WHERE tanggal BETWEEN $1::date AND $2::date), 0)::float8 AS omzet,
         count(*) FILTER (WHERE tanggal BETWEEN $1::date AND $2::date)::int AS pesanan,
         COALESCE(sum(total_amount) FILTER (WHERE tanggal BETWEEN $3::date AND $4::date), 0)::float8 AS banding_omzet,
         count(*) FILTER (WHERE tanggal BETWEEN $3::date AND $4::date)::int AS banding_pesanan,
         count(*) FILTER (WHERE tanggal BETWEEN $3::date AND $2::date)::int AS baris
         FROM terpakai`,
      rentang
    ),
    // B2B diakui SAAT DIBAYAR (keputusan owner 2026-07-31), memakai `paid_on`
    // dari pembayaran — bukan status invoice: `crm_sales_invoices.status` hanya
    // mengenal diajukan/draft/terkirim/batal, tidak punya status lunas maupun
    // kolom tanggal bayar. Pembayaran juga menangani pelunasan sebagian dengan
    // benar: invoice yang baru dibayar separuh menyumbang separuhnya saja.
    // Pembayaran tanpa `invoice_id` (pembayaran level deal) tetap dihitung —
    // uangnya nyata masuk.
    queryOne<{ omzet: string; banding_omzet: string; baris: number }>(
      `SELECT
         COALESCE(sum(amount) FILTER (WHERE paid_on BETWEEN $1::date AND $2::date), 0)::float8 AS omzet,
         COALESCE(sum(amount) FILTER (WHERE paid_on BETWEEN $3::date AND $4::date), 0)::float8 AS banding_omzet,
         count(*) FILTER (WHERE paid_on BETWEEN $3::date AND $2::date)::int AS baris
         FROM crm.crm_sales_deal_payments
        WHERE deleted_at IS NULL`,
      rentang
    ),
  ]);

  const fnbNilai = Number(fnb?.omzet ?? 0);
  const b2bNilai = Number(b2b?.omzet ?? 0);
  const rincian = breakdownRevenue([
    { kunci: "fnb", label: "F&B", nilai: fnbNilai },
    { kunci: "b2b", label: "B2B", nilai: b2bNilai },
  ]);

  return {
    omzet: rincian.total,
    pesanan: Number(fnb?.pesanan ?? 0),
    sumber: rincian.sumber,
    banding: {
      omzet: Number(fnb?.banding_omzet ?? 0) + Number(b2b?.banding_omzet ?? 0),
      pesanan: Number(fnb?.banding_pesanan ?? 0),
    },
    proyeksi: projectRunRate(rincian.total, period),
    // Sepanjang rentang pembanding s/d sekarang tidak ada satu pun transaksi di
    // KEDUA sumber → "belum ada data", bukan "penjualan nol".
    adaData: Number(fnb?.baris ?? 0) + Number(b2b?.baris ?? 0) > 0,
  };
}

/**
 * Tamu yang sedang duduk saat ini — TIDAK mengikuti periode papan, karena ini
 * keadaan sekarang. "Tamu di meja YTD" bukan pertanyaan yang punya arti.
 */
async function fetchGuestsSeated(): Promise<GuestsSeated> {
  const row = await queryOne<{ tamu: string; meja: number; kapasitas: string }>(
    `SELECT COALESCE(sum(o.guest_count), 0)::float8 AS tamu,
            count(DISTINCT o.table_id)::int AS meja,
            COALESCE(sum(DISTINCT t.capacity), 0)::float8 AS kapasitas
       FROM pos.pos_orders o
       LEFT JOIN pos.pos_tables t ON t.id::text = o.table_id
      WHERE o.table_id IS NOT NULL
        AND o.voided_at IS NULL
        AND o.status IN ('pending', 'confirmed', 'preparing', 'ready', 'served')`
  );

  const meja = Number(row?.meja ?? 0);
  return {
    tamu: Number(row?.tamu ?? 0),
    meja,
    kapasitas: Number(row?.kapasitas ?? 0),
    // Nol meja terisi adalah FAKTA (restoran sedang kosong), bukan "belum ada
    // data" — berbeda dari widget periode. Karena itu adaData selalu true.
    adaData: true,
  };
}

/** Diskon yang dikeluarkan vs omzet yang dibawanya, per kampanye (Fase B). */
async function fetchPromoImpact(period: Period): Promise<PromoImpact> {
  const rows = await query<{
    kampanye: string | null;
    diskon: string;
    omzet: string;
    redemption: number;
  }>(
    `SELECT COALESCE(r.campaign_name, '(tanpa nama)') AS kampanye,
            COALESCE(sum(r.discount_amount), 0)::float8 AS diskon,
            COALESCE(sum(o.total_amount), 0)::float8 AS omzet,
            count(*)::int AS redemption
       FROM promo.promo_redemptions r
       LEFT JOIN pos.pos_orders o
              ON o.id = r.context_id
             AND o.status <> 'cancelled'
             AND o.voided_at IS NULL
      WHERE r.status = 'captured'
        AND r.context_type = 'pos_order'
        AND (r.created_at AT TIME ZONE 'Asia/Jakarta')::date BETWEEN $1::date AND $2::date
      GROUP BY 1
      ORDER BY 2 DESC
      LIMIT 5`,
    [period.mulai, period.selesai]
  );

  const teratas = rows.map((r) => ({
    kampanye: r.kampanye ?? "(tanpa nama)",
    diskon: Number(r.diskon),
    omzet: Number(r.omzet),
    redemption: Number(r.redemption),
  }));

  const diskon = teratas.reduce((acc, r) => acc + r.diskon, 0);
  const omzetTerbawa = teratas.reduce((acc, r) => acc + r.omzet, 0);

  return {
    diskon,
    omzetTerbawa,
    redemption: teratas.reduce((acc, r) => acc + r.redemption, 0),
    efisiensi: promoEfficiency(diskon, omzetTerbawa),
    teratas,
    adaData: teratas.length > 0,
  };
}

async function fetchTeamToday(): Promise<TeamToday> {
  const today = todayJakarta();
  const row = await queryOne<{
    aktif: number;
    hadir: number;
    terlambat: number;
    cuti: number;
  }>(
    `SELECT
       (SELECT count(*) FROM hris.employees WHERE is_active)::int AS aktif,
       (SELECT count(*) FROM hris.attendance WHERE date = $1::date)::int AS hadir,
       (SELECT count(*) FROM hris.attendance WHERE date = $1::date AND is_late)::int AS terlambat,
       (SELECT count(DISTINCT l.employee_id)
          FROM hris.leaves l
          JOIN hris.employees e ON e.id = l.employee_id AND e.is_active
         WHERE l.status = 'approved'
           AND $1::date BETWEEN l.start_date AND l.end_date)::int AS cuti`,
    [today]
  );

  const aktif = row?.aktif ?? 0;
  const hadir = row?.hadir ?? 0;
  const cuti = row?.cuti ?? 0;
  return {
    aktif,
    hadir,
    terlambat: row?.terlambat ?? 0,
    cuti,
    belum: Math.max(0, aktif - hadir - cuti),
  };
}

async function fetchPendingDecisions(): Promise<PendingDecisions> {
  // Definisi 'pending' identik dengan nav-badges (lib/hris/nav-badges.ts);
  // pinjaman = menunggu HR (manager_approved, atau pending tanpa manager — EPIC-060);
  // PO memakai status 'draft' sesuai enum purchase_orders (draft → approved).
  const row = await queryOne<{
    cuti: number;
    lembur: number;
    pinjaman: number;
    po_draft: number;
    kandidat: number;
  }>(
    `SELECT
       (SELECT count(*) FROM hris.leaves WHERE status = 'pending')::int AS cuti,
       (SELECT count(*) FROM hris.overtime_requests WHERE status = 'pending')::int AS lembur,
       (SELECT count(*) FROM hris.loans WHERE status = 'manager_approved' OR (status = 'pending' AND manager_approver_id IS NULL))::int AS pinjaman,
       (SELECT count(*) FROM purchase_orders WHERE status = 'draft')::int AS po_draft,
       (SELECT count(*) FROM recruitment.candidates WHERE status = 'applied')::int AS kandidat`
  );

  const cuti = row?.cuti ?? 0;
  const lembur = row?.lembur ?? 0;
  const pinjaman = row?.pinjaman ?? 0;
  const poDraft = row?.po_draft ?? 0;
  const kandidatBaru = row?.kandidat ?? 0;
  return {
    cuti,
    lembur,
    pinjaman,
    poDraft,
    kandidatBaru,
    total: cuti + lembur + pinjaman + poDraft + kandidatBaru,
  };
}

async function fetchLowStock(): Promise<LowStock> {
  // Query identik dengan tool Do `stok_menipis`.
  const rows = await query<{ bahan: string; tersedia: number; minimum: number }>(
    `SELECT rm.nama AS bahan,
            i.qty_available::float8 AS tersedia,
            i.qty_minimum::float8 AS minimum
       FROM inventory.inventory i
       JOIN item.raw_materials rm ON rm.id = i.raw_material_id
      WHERE i.is_active
        AND rm.deleted_at IS NULL
        AND i.qty_available <= i.qty_minimum
      ORDER BY (i.qty_minimum - i.qty_available) DESC`
  );
  return {
    jumlah: rows.length,
    teratas: rows.slice(0, 3).map((r) => ({
      bahan: r.bahan,
      tersedia: Number(r.tersedia),
      minimum: Number(r.minimum),
    })),
  };
}

async function fetchCrmPulse(): Promise<CrmPulse> {
  const row = await queryOne<{ member: number; xp: number; redeem: number }>(
    `SELECT
       (SELECT count(*) FROM crm_member_profiles
         WHERE created_at > now() - interval '7 days')::int AS member,
       (SELECT COALESCE(sum(xp_delta), 0) FROM crm_xp_ledger
         WHERE created_at > now() - interval '7 days' AND xp_delta > 0)::int AS xp,
       (SELECT count(*) FROM crm_redemptions
         WHERE created_at > now() - interval '7 days')::int AS redeem`
  );
  return {
    memberBaru7Hari: row?.member ?? 0,
    xpTerdistribusi7Hari: row?.xp ?? 0,
    rewardDitukar7Hari: row?.redeem ?? 0,
  };
}

/**
 * Nilai-kosong `DesktopOverview` — semua seksi null, periode wajar.
 *
 * Ada supaya penambahan seksi baru tidak lagi memecahkan setiap fixture tes
 * yang membangun objek ini secara literal (sudah terjadi dua kali: `periode`
 * lalu `dampakPromo`). Pemanggil cukup menyebar dan menimpa yang relevan.
 */
export function emptyDesktopOverview(
  kind: PeriodKind = "today",
  now = new Date()
): DesktopOverview {
  return {
    dibuatPada: now.toISOString(),
    periode: summarizePeriod(kind, now),
    omzetPeriode: null,
    dampakPromo: null,
    tamuDiMeja: null,
    pulsaBisnis: null,
    timHariIni: null,
    perluKeputusan: null,
    stokMenipis: null,
    member: null,
    gagal: [],
  };
}

/** Satu seksi gagal → null + tercatat; jangan menjatuhkan seksi lain. */
async function safeSection<T>(
  name: string,
  gagal: string[],
  fn: () => Promise<T>
): Promise<T | null> {
  try {
    return await fn();
  } catch (error) {
    console.error(`[desktop:overview] seksi ${name} gagal:`, error);
    gagal.push(name);
    return null;
  }
}

/**
 * @param kind Periode papan (EPIC-037 Fase A). Default `today` — nilai lama,
 *   supaya pemanggil yang belum diperbarui tidak berubah artinya.
 *
 * Catatan sengaja: `timHariIni`, `perluKeputusan`, dan `stokMenipis` TIDAK
 * mengikuti periode. Ketiganya menggambarkan keadaan SEKARANG (siapa hadir hari
 * ini, apa yang menunggu approval, stok apa yang menipis) — "stok menipis YTD"
 * bukan pertanyaan yang punya arti. Yang mengikuti periode hanya angka yang
 * memang berbentuk akumulasi.
 */
export async function buildDesktopOverview(
  kind: PeriodKind = "today"
): Promise<DesktopOverview> {
  const gagal: string[] = [];
  const ringkasan = summarizePeriod(kind);

  const [
    omzetPeriode,
    dampakPromo,
    tamuDiMeja,
    pulsaBisnis,
    timHariIni,
    perluKeputusan,
    stokMenipis,
    member,
  ] = await Promise.all([
      safeSection("omzetPeriode", gagal, () =>
        fetchRevenuePeriod(ringkasan.periode, ringkasan.banding)
      ),
      safeSection("dampakPromo", gagal, () => fetchPromoImpact(ringkasan.periode)),
      safeSection("tamuDiMeja", gagal, fetchGuestsSeated),
      safeSection("pulsaBisnis", gagal, fetchSalesPulse),
      safeSection("timHariIni", gagal, fetchTeamToday),
      safeSection("perluKeputusan", gagal, fetchPendingDecisions),
      safeSection("stokMenipis", gagal, fetchLowStock),
      safeSection("member", gagal, fetchCrmPulse),
    ]);

  return {
    dibuatPada: new Date().toISOString(),
    periode: ringkasan,
    omzetPeriode,
    dampakPromo,
    tamuDiMeja,
    pulsaBisnis,
    timHariIni,
    perluKeputusan,
    stokMenipis,
    member,
    gagal,
  };
}

/** Role yang boleh melihat papan monitoring (keputusan owner 2026-07-21).
 *  `owner` belum ada di iam.roles; `direksi` adalah padanannya. */
export const DESKTOP_OVERVIEW_ROLES = ["super_admin", "direksi"] as const;

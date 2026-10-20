/**
 * EPIC-020 Fase B+D — pengawas berkala notifikasi WA owner:
 * - Ringkasan harian: terkirim sekali per hari WIB begitu jam menyentuh
 *   `digestHour` (dedup key = tanggal — restart server tidak mengirim ulang).
 * - Stok habis: bahan aktif dengan qty_available ≤ 0 → satu pesan gabungan;
 *   dedup 1×/bahan/hari (bahan yang masih nol besok diingatkan lagi).
 * - Ambang (Fase D, hanya jam 8-21 WIB — non-kritis tidak menyela tidur):
 *   omzet MTD anjlok (target-first, fallback bulan lalu — keputusan owner
 *   2026-07-22), approval menginap >2 hari, kontrak PKWT habis ≤30 hari.
 *
 * Void besar TIDAK di sini — dia event, dikait langsung di route void POS.
 */

import { query } from "@/lib/db";
import { loadGatewayConfig, sendGatewayText } from "@/lib/whatsapp/gateway";
import { buildDesktopOverview } from "@/lib/desktop/overview";
import { buildFlashReportMessage, gatherFlashReportData } from "./flash-report";
import {
  SALES_TARGET_SETTING_KEY,
  parseSalesTarget,
} from "@/lib/dashboard/sales-target";
import { getSetting } from "@/lib/settings/app-settings";
import {
  approvalMenginapDedupKey,
  buildApprovalMenginapMessage,
  buildDigestMessage,
  buildKontrakHabisMessage,
  buildOmzetAnjlokMessage,
  buildStokHabisMessage,
  digestDedupKey,
  hourWib,
  isoWeekWib,
  kontrakHabisDedupKey,
  omzetAnjlokDedupKey,
  stokHabisDedupKey,
  todayWib,
  type KontrakHabisItem,
} from "./notifications-messages";
import {
  claimNotifKeys,
  deliverToRecipients,
  getWaNotifConfig,
  releaseNotifKeys,
  sendOwnerNotification,
} from "./notifications-sender";
import type { WaNotifConfig } from "./notifications-config";
import { appOrigin } from "@/lib/app-origin";

const CHECK_INTERVAL_MS = 5 * 60_000;
// Jam layak-ganggu utk notifikasi ambang (non-kritis) — jam tenang ditahan.
const AMBANG_HOUR_MIN = 8;
const AMBANG_HOUR_MAX = 21;
// Di bawah ini pembanding terlalu kecil utk disimpulkan "anjlok".
const OMZET_BASELINE_MIN_RP = 500_000;
// Tanggal 1-4 datanya belum cukup — mulai evaluasi tanggal 5.
const OMZET_MIN_DAY_OF_MONTH = 5;

const inAmbangWindow = () => {
  const h = hourWib();
  return h >= AMBANG_HOUR_MIN && h <= AMBANG_HOUR_MAX;
};

async function maybeSendDigest(): Promise<void> {
  const config = await getWaNotifConfig();
  if (!config.enabled || !config.types.digest) return;
  if (hourWib() < config.digestHour) return;

  const date = todayWib();
  // Cek murah sebelum membangun overview (5 query agregasi) — klaim di
  // sender tetap penjaga akhirnya.
  const existing = await query<{ id: string }>(
    `SELECT id FROM configuration.wa_notif_log
      WHERE notif_type = 'digest' AND dedup_key = $1`,
    [digestDedupKey(date)]
  );
  if (existing.length > 0) return;

  const overview = await buildDesktopOverview();

  // Daily Flash Report (owner 2026-08-23, dikoreksi 2026-08-30): data
  // HARI INI — digest terkirim di digestHour (default 22:00 WIB, jam
  // tutup) sehingga tanggal laporan = tanggal terima. Dulu keliru H-1
  // (asumsi digest pagi) sehingga tanggal laporan selalu mundur. Gagal membangun flash report
  // tidak menggagalkan digest (fallback ke ringkasan lama saja).
  let flashSection = "";
  try {
    const flashData = await gatherFlashReportData(date);
    flashSection = `${buildFlashReportMessage(flashData, date)}\n\n`;
  } catch (err) {
    console.error("[wa-notif] gagal membangun flash report:", err);
  }

  const result = await sendOwnerNotification({
    type: "digest",
    dedupKey: digestDedupKey(date),
    message: `${flashSection}${buildDigestMessage(overview, date)}`,
    config,
  });
  if (result.sent) {
    console.log(`[wa-notif] ringkasan harian ${date} terkirim`);
  }
}

async function maybeSendStokHabis(): Promise<void> {
  const config = await getWaNotifConfig();
  if (!config.enabled || !config.types.stokHabis) return;

  // Definisi mengikuti fetchLowStock desktop, dipersempit ke NOL persis.
  // Kolom "satuan" tidak ada di item.raw_materials — JOIN ke item.units.
  const rows = await query<{ id: string; nama: string; satuan: string | null }>(
    `SELECT rm.id, rm.nama, u.nama AS satuan
       FROM inventory.inventory i
       JOIN item.raw_materials rm ON rm.id = i.raw_material_id
       LEFT JOIN item.units u ON u.id = rm.satuan_besar_id
      WHERE i.is_active
        AND rm.deleted_at IS NULL
        AND i.qty_available <= 0
      ORDER BY rm.nama`
  );
  if (rows.length === 0) return;
  if (config.recipients.length === 0) return;

  const date = todayWib();
  // Klaim per bahan (dedup 1×/bahan/hari), lalu SATU pesan gabungan berisi
  // hanya bahan yang berhasil diklaim — 20 bahan habis ≠ 20 pesan WA, dan
  // bahan yang baru habis siang hari tetap diberitakan susulan.
  const byKey = new Map(rows.map((r) => [stokHabisDedupKey(r.id, date), r]));
  const message = buildStokHabisMessage(
    rows.map((r) => ({ nama: r.nama, satuan: r.satuan }))
  );
  const claimed = await claimNotifKeys(
    "stokHabis",
    [...byKey.keys()].map((dedupKey) => ({ dedupKey, message })),
    config.recipients
  );
  if (claimed.length === 0) return;

  const freshItems = claimed.map((c) => {
    const row = byKey.get(c.dedupKey)!;
    return { nama: row.nama, satuan: row.satuan };
  });
  const { delivered, timedOut } = await deliverToRecipients(
    config,
    buildStokHabisMessage(freshItems)
  );
  if (delivered === 0 && !timedOut) {
    // Gagal jelas → lepas klaim, dicoba lagi tick berikut.
    await releaseNotifKeys(claimed.map((c) => c.id));
    return;
  }
  console.log(
    `[wa-notif] peringatan stok habis terkirim (${freshItems.length} bahan)`
  );
}

// ---------- Fase D — ambang ----------

/**
 * Omzet MTD anjlok (keputusan owner): omzet tanggal 1..kemarin dibanding
 * pace target bulanan (bila target diisi) atau MTD bulan lalu di titik hari
 * yang sama. Maks 1 pesan/minggu selama masih anjlok.
 */
async function maybeSendOmzetAnjlok(config: WaNotifConfig): Promise<void> {
  if (!config.types.omzetAnjlok) return;

  const today = todayWib();
  const dayOfMonth = Number(today.slice(8, 10));
  if (dayOfMonth < OMZET_MIN_DAY_OF_MONTH) return;
  const fullDays = dayOfMonth - 1; // tanggal 1..kemarin (hari penuh saja)

  const week = isoWeekWib();
  const existing = await query<{ id: string }>(
    `SELECT id FROM configuration.wa_notif_log
      WHERE notif_type = 'omzetAnjlok' AND dedup_key = $1`,
    [omzetAnjlokDedupKey(week)]
  );
  if (existing.length > 0) return;

  // Filter identik dgn pulsa bisnis desktop: batal & void tidak dihitung.
  const monthStart = `${today.slice(0, 7)}-01`;
  const mtdRow = await query<{ omzet: string }>(
    `SELECT COALESCE(sum(total_amount), 0)::float8 AS omzet
       FROM pos.pos_orders
      WHERE (created_at AT TIME ZONE 'Asia/Jakarta')::date >= $1::date
        AND (created_at AT TIME ZONE 'Asia/Jakarta')::date < $2::date
        AND status <> 'cancelled'
        AND voided_at IS NULL`,
    [monthStart, today]
  );
  const mtd = Number(mtdRow[0]?.omzet ?? 0);

  // Baseline: target dulu, fallback MTD bulan lalu (keputusan owner).
  const target = parseSalesTarget(await getSetting(SALES_TARGET_SETTING_KEY));
  let baseline: number;
  let source: "target" | "bulan-lalu";
  if (target.bulananRp > 0) {
    const daysInMonth = new Date(
      Number(today.slice(0, 4)),
      Number(today.slice(5, 7)),
      0
    ).getDate();
    baseline = Math.round((target.bulananRp * fullDays) / daysInMonth);
    source = "target";
  } else {
    const prev = new Date(`${monthStart}T00:00:00Z`);
    prev.setUTCMonth(prev.getUTCMonth() - 1);
    const prevStart = prev.toISOString().slice(0, 10);
    const prevDays = new Date(
      prev.getUTCFullYear(),
      prev.getUTCMonth() + 1,
      0
    ).getDate();
    // Bulan lalu lebih pendek → bandingkan sebanyak hari yang ada
    const cmpDays = Math.min(fullDays, prevDays);
    const prevRow = await query<{ omzet: string }>(
      `SELECT COALESCE(sum(total_amount), 0)::float8 AS omzet
         FROM pos.pos_orders
        WHERE (created_at AT TIME ZONE 'Asia/Jakarta')::date >= $1::date
          AND (created_at AT TIME ZONE 'Asia/Jakarta')::date < $1::date + $2::int
          AND status <> 'cancelled'
          AND voided_at IS NULL`,
      [prevStart, cmpDays]
    );
    baseline = Number(prevRow[0]?.omzet ?? 0);
    source = "bulan-lalu";
  }

  if (baseline < OMZET_BASELINE_MIN_RP) return;
  const pct = Math.round((mtd / baseline) * 100);
  if (pct >= config.omzetAnjlokPct) return;

  const result = await sendOwnerNotification({
    type: "omzetAnjlok",
    dedupKey: omzetAnjlokDedupKey(week),
    message: buildOmzetAnjlokMessage({
      mtd,
      baseline,
      pct,
      source,
      hariBerjalan: fullDays,
    }),
    config,
  });
  if (result.sent) {
    console.log(`[wa-notif] omzet anjlok terkirim (${pct}% dari ${source})`);
  }
}

/** Pengajuan pending >2 hari — definisi status sama dgn desktop/nav-badges. */
async function maybeSendApprovalMenginap(config: WaNotifConfig): Promise<void> {
  if (!config.types.approvalMenginap) return;

  const date = todayWib();
  const existing = await query<{ id: string }>(
    `SELECT id FROM configuration.wa_notif_log
      WHERE notif_type = 'approvalMenginap' AND dedup_key = $1`,
    [approvalMenginapDedupKey(date)]
  );
  if (existing.length > 0) return;

  const row = await query<{
    cuti: number;
    lembur: number;
    pinjaman: number;
    po_draft: number;
  }>(
    `SELECT
       (SELECT count(*) FROM hris.leaves
         WHERE status = 'pending' AND created_at < now() - interval '2 days')::int AS cuti,
       (SELECT count(*) FROM hris.overtime_requests
         WHERE status = 'pending' AND created_at < now() - interval '2 days')::int AS lembur,
       (SELECT count(*) FROM hris.loans
         WHERE (status = 'manager_approved' OR (status = 'pending' AND manager_approver_id IS NULL))
           AND created_at < now() - interval '2 days')::int AS pinjaman,
       (SELECT count(*) FROM purchase_orders
         WHERE status = 'draft' AND created_at < now() - interval '2 days')::int AS po_draft`
  );
  const counts = row[0];
  if (!counts) return;
  const total =
    Number(counts.cuti) +
    Number(counts.lembur) +
    Number(counts.pinjaman) +
    Number(counts.po_draft);
  if (total === 0) return;

  const result = await sendOwnerNotification({
    type: "approvalMenginap",
    dedupKey: approvalMenginapDedupKey(date),
    message: buildApprovalMenginapMessage({
      cuti: Number(counts.cuti),
      lembur: Number(counts.lembur),
      pinjaman: Number(counts.pinjaman),
      poDraft: Number(counts.po_draft),
    }),
    config,
  });
  if (result.sent) {
    console.log(`[wa-notif] approval menginap terkirim (${total} pengajuan)`);
  }
}

/**
 * Kontrak PKWT aktif berakhir ≤30 hari — sekali per kontrak (perpanjangan
 * mengubah end_date = kejadian baru). Pola klaim-gabungan spt stok habis.
 */
async function maybeSendKontrakHabis(config: WaNotifConfig): Promise<void> {
  if (!config.types.kontrakHabis) return;
  if (config.recipients.length === 0) return;

  // Definisi mengikuti /api/hris/contracts/expiring; -7 hari menjaring yang
  // baru saja lewat tapi belum diakhiri.
  const rows = await query<{
    id: string;
    full_name: string;
    end_date: string;
    days_left: number;
  }>(
    `SELECT c.id, e.full_name, c.end_date::text AS end_date,
            (c.end_date - current_date)::int AS days_left
       FROM hris.employment_contracts c
       JOIN hris.employees e ON e.id = c.employee_id
      WHERE c.status = 'active' AND c.contract_type = 'pkwt'
        AND c.end_date IS NOT NULL
        AND c.end_date <= current_date + 30
        AND c.end_date >= current_date - 7
      ORDER BY c.end_date`
  );
  if (rows.length === 0) return;

  const byKey = new Map(
    rows.map((r) => [kontrakHabisDedupKey(r.id, r.end_date), r])
  );
  const previewMessage = buildKontrakHabisMessage(
    rows.map((r) => ({
      employeeName: r.full_name,
      endDate: r.end_date,
      daysLeft: Number(r.days_left),
    }))
  );
  const claimed = await claimNotifKeys(
    "kontrakHabis",
    [...byKey.keys()].map((dedupKey) => ({ dedupKey, message: previewMessage })),
    config.recipients
  );
  if (claimed.length === 0) return;

  const items: KontrakHabisItem[] = claimed.map((c) => {
    const row = byKey.get(c.dedupKey)!;
    return {
      employeeName: row.full_name,
      endDate: row.end_date,
      daysLeft: Number(row.days_left),
    };
  });
  const { delivered, timedOut } = await deliverToRecipients(
    config,
    buildKontrakHabisMessage(items)
  );
  if (delivered === 0 && !timedOut) {
    await releaseNotifKeys(claimed.map((c) => c.id));
    return;
  }
  console.log(`[wa-notif] kontrak habis terkirim (${items.length} kontrak)`);
}

/**
 * EPIC-028 Fase D — reminder ke PEMEGANG Season Pass yang berlaku ≤14 hari lagi
 * (dorong renewal). Kirim langsung ke holder_phone (bukan recipients owner);
 * klaim-dulu via wa_notif_log (notif_type='passExpiring', dedup pass+valid_until)
 * → sekali per (pass, tanggal-berakhir). Gagal kirim → lepas klaim utk retry.
 */
const PASS_EXPIRY_REMINDER_DAYS = 14;

async function maybeSendPassExpiring(): Promise<void> {
  const gw = await loadGatewayConfig();
  if (!gw) return;

  const rows = await query<{
    id: string;
    pass_code: string;
    holder_name: string;
    holder_phone: string;
    valid_until: string;
    access_token: string;
  }>(
    `SELECT id, pass_code, holder_name, holder_phone,
            valid_until::text AS valid_until, access_token
       FROM ticketing.ticket_season_passes
      WHERE status = 'active' AND holder_phone IS NOT NULL
        AND valid_until IS NOT NULL
        AND valid_until BETWEEN current_date AND current_date + $1`,
    [PASS_EXPIRY_REMINDER_DAYS]
  );
  if (rows.length === 0) return;

  const baseUrl = appOrigin();
  let sent = 0;
  for (const r of rows) {
    const dedupKey = `${r.id}:${r.valid_until}`;
    const statusUrl = `${baseUrl}/pass/status/${r.access_token}`;
    const message =
      `*Season Pass akan berakhir* ⏰\n\n` +
      `Halo ${r.holder_name}, pass *${r.pass_code}* berlaku sampai ` +
      `${r.valid_until}. Perpanjang agar tetap bisa masuk.\n\n` +
      `Cek status pass: ${statusUrl}`;

    // Klaim-dulu: baris baru = belum pernah dikirim untuk tanggal berakhir ini
    const claim = await query<{ id: string }>(
      `INSERT INTO configuration.wa_notif_log (notif_type, dedup_key, message, recipients)
       VALUES ('passExpiring', $1, $2, $3::jsonb)
       ON CONFLICT (notif_type, dedup_key) DO NOTHING
       RETURNING id`,
      [dedupKey, message, JSON.stringify([r.holder_phone])]
    );
    if (claim.length === 0) continue;

    const res = await sendGatewayText(gw, { target: r.holder_phone, message });
    if (!res.success) {
      // Lepas klaim supaya dicoba lagi tick berikutnya
      await query(
        `DELETE FROM configuration.wa_notif_log
         WHERE notif_type = 'passExpiring' AND dedup_key = $1`,
        [dedupKey]
      );
      continue;
    }
    sent += 1;
  }
  if (sent > 0) {
    console.log(`[wa-notif] reminder pass berakhir terkirim (${sent} pass)`);
  }
}

async function runAmbangChecks(): Promise<void> {
  if (!inAmbangWindow()) return;
  const config = await getWaNotifConfig();
  if (!config.enabled) return;

  await maybeSendPassExpiring().catch((error) => {
    console.error("[wa-notif] reminder pass berakhir gagal:", error);
  });

  await maybeSendOmzetAnjlok(config).catch((error) => {
    console.error("[wa-notif] cek omzet anjlok gagal:", error);
  });
  await maybeSendApprovalMenginap(config).catch((error) => {
    console.error("[wa-notif] cek approval menginap gagal:", error);
  });
  await maybeSendKontrakHabis(config).catch((error) => {
    console.error("[wa-notif] cek kontrak habis gagal:", error);
  });
}

let started = false;

/** Daftarkan pengecekan berkala — sekali per proses server (pola watcher lain). */
export function startWaNotifWatcher(): void {
  if (started) return;
  started = true;

  const tick = () => {
    maybeSendDigest().catch((error) => {
      console.error("[wa-notif] digest gagal:", error);
    });
    maybeSendStokHabis().catch((error) => {
      console.error("[wa-notif] cek stok habis gagal:", error);
    });
    runAmbangChecks().catch((error) => {
      console.error("[wa-notif] cek ambang gagal:", error);
    });
  };
  // Delay saat boot agar pool & gateway siap, lalu tiap 5 menit.
  setTimeout(tick, 45_000);
  setInterval(tick, CHECK_INTERVAL_MS);
}

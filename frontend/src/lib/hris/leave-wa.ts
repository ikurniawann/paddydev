import { getPool, queryOne } from "@/lib/db";
import { loadGatewayConfig, sendGatewayText } from "@/lib/whatsapp/gateway";
import { normalizeWaPhone } from "@/lib/pos/receipt-wa";
import { leaveTypeLabel } from "./leave-types";

/**
 * Notifikasi WA pengajuan izin/cuti (permintaan owner 2026-08-30):
 * setiap karyawan mengajukan izin/cuti, ATASAN LANGSUNGNYA (menurut
 * employees.reporting_to — sumber yang sama dengan Shift Tim) langsung
 * dikabari via WA supaya persetujuan tidak menunggu dibuka di dashboard.
 *
 * Best-effort & fire-and-forget (pola comp-notification): gagal kirim
 * tidak pernah menggagalkan pengajuan cutinya sendiri. Dedup lewat
 * configuration.wa_notif_log (notif_type 'cuti') — satu pengajuan satu
 * pesan walau route terpanggil ulang.
 */

const LEAVE_NOTIF_TYPE = "cuti";

export { LEAVE_TYPE_LABELS, leaveTypeLabel } from "./leave-types";

const tanggalId = (iso: string): string => {
  const d = new Date(`${iso}T00:00:00+07:00`);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString("id-ID", {
    weekday: "short",
    day: "2-digit",
    month: "short",
    year: "numeric",
    timeZone: "Asia/Jakarta",
  });
};

export interface LeaveRequestWaInput {
  leaveId: string;
  employeeName: string;
  leaveType: string;
  startDate: string; // YYYY-MM-DD
  endDate: string;
  totalDays: number;
  reason?: string | null;
  approverName?: string | null;
}

/** Pure & unit-testable — isi pesan WA ke atasan. */
export function buildLeaveRequestWaMessage(input: LeaveRequestWaInput): string {
  const rentang =
    input.startDate === input.endDate
      ? tanggalId(input.startDate)
      : `${tanggalId(input.startDate)} s.d. ${tanggalId(input.endDate)}`;
  const baris = [
    `Arkiv OS — Pengajuan ${leaveTypeLabel(input.leaveType)}`,
    ...(input.approverName ? [`Kepada: ${input.approverName}`] : []),
    `Karyawan : ${input.employeeName}`,
    `Tanggal : ${rentang} (${input.totalDays} hari kerja)`,
  ];
  if (input.reason?.trim()) baris.push(`Alasan : ${input.reason.trim()}`);
  baris.push("", "Mohon ditinjau di dashboard Arkiv OS (Area Karyawan → Persetujuan Tim).");
  return baris.join("\n");
}

/**
 * Kirim notifikasinya — aman dipanggil fire-and-forget:
 * `void notifyLeaveRequestWa(...)` (semua error tertelan ke console).
 */
export async function notifyLeaveRequestWa(
  input: Omit<LeaveRequestWaInput, "approverName"> & { employeeId: string }
): Promise<void> {
  try {
    const atasan = await queryOne<{ full_name: string; phone: string | null }>(
      `SELECT a.full_name, a.phone
       FROM hris.employees e
       JOIN hris.employees a ON a.id = e.reporting_to AND a.is_active = true AND a.id <> e.id
       WHERE e.id = $1`,
      [input.employeeId]
    );
    if (!atasan) {
      console.warn(`[wa-cuti] ${input.employeeName}: tidak punya atasan tercatat — notifikasi dilewati`);
      return;
    }
    const target = normalizeWaPhone(atasan.phone);
    if (!target) {
      console.warn(`[wa-cuti] atasan ${atasan.full_name}: nomor WA kosong/tidak valid — notifikasi dilewati`);
      return;
    }

    const message = buildLeaveRequestWaMessage({ ...input, approverName: atasan.full_name });
    const dedupKey = `leave:${input.leaveId}`.slice(0, 160);

    const claim = await getPool().query<{ id: string }>(
      `INSERT INTO configuration.wa_notif_log (notif_type, dedup_key, message, recipients)
       VALUES ($1, $2, $3, $4::jsonb)
       ON CONFLICT (notif_type, dedup_key) DO NOTHING
       RETURNING id`,
      [LEAVE_NOTIF_TYPE, dedupKey, message, JSON.stringify([target])]
    );
    if (claim.rows.length === 0) return; // duplikat — sudah pernah terkirim

    const release = () =>
      getPool()
        .query(`DELETE FROM configuration.wa_notif_log WHERE id = $1`, [claim.rows[0].id])
        .catch(() => {});

    const gateway = await loadGatewayConfig();
    if (!gateway) {
      console.warn("[wa-cuti] gateway belum dikonfigurasi — notifikasi dilewati");
      await release();
      return;
    }

    const result = await sendGatewayText(gateway, { target, message });
    if (!result.success && !result.timedOut) {
      console.error(`[wa-cuti] gagal kirim ke ${target}: ${result.reason}`);
      await release();
    }
  } catch (err) {
    console.error("[wa-cuti] notifikasi pengajuan cuti error:", err);
  }
}

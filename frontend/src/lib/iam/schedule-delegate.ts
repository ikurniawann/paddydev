import "server-only";
import { cache } from "react";
import { iamDbQueryOne, isIamDbConfigured } from "./pg-client";

/** Halaman ESS Jadwal Toko; menunya hanya tampil untuk pengatur aktif. */
export const JADWAL_TOKO_PATH = "/dashboard/me/jadwal-toko";

/**
 * Akun ini pengatur jadwal toko aktif (EPIC-071 R8/R11), persis seperti
 * loadDelegateAccess di Go: dari karyawan AKTIF ber-`user_id` ini yang punya
 * penunjukan, ambil SATU dengan id terkecil (ORDER BY e.id LIMIT 1), lalu
 * penunjukan itu harus punya minimal satu toko aktif. Dengan begitu menu dan
 * halaman (GET /api/hris/me/schedule-access) selalu sepakat. Hanya untuk
 * menampilkan menu Jadwal Toko; aksesnya tetap diperiksa API. Gagal dibaca →
 * false (menu disembunyikan).
 */
export const isScheduleDelegate = cache(async (userId: string): Promise<boolean> => {
  if (!isIamDbConfigured()) return false;
  try {
    const row = await iamDbQueryOne<{ ok: boolean }>(
      `WITH picked AS (
         SELECT sd.employee_id
         FROM hris.schedule_delegates sd
         JOIN hris.employees e ON e.id = sd.employee_id
         WHERE e.is_active AND e.user_id = $1
         ORDER BY e.id
         LIMIT 1
       )
       SELECT EXISTS (
         SELECT 1
         FROM picked p
         JOIN hris.schedule_delegate_branches sb ON sb.employee_id = p.employee_id
         JOIN configuration.branches b ON b.id = sb.branch_id
         WHERE b.is_active
       ) AS ok`,
      [userId]
    );
    return row?.ok === true;
  } catch (error) {
    console.error("[iam] isScheduleDelegate failed:", error);
    return false;
  }
});

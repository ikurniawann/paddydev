import "server-only";
import { cache } from "react";
import { iamDbQueryOne, isIamDbConfigured } from "./pg-client";

/** Halaman ESS Jadwal Toko; menunya hanya tampil untuk pengatur aktif. */
export const JADWAL_TOKO_PATH = "/dashboard/me/jadwal-toko";

/**
 * Akun ini pengatur jadwal toko aktif (EPIC-071 R8/R11): penunjukan milik
 * karyawan AKTIF dengan `user_id` ini dan minimal satu toko aktif — sama
 * dengan loadDelegateAccess di Go. Hanya untuk menampilkan menu Jadwal Toko;
 * aksesnya sendiri tetap diperiksa API. Gagal dibaca → false (menu disembunyikan).
 */
export const isScheduleDelegate = cache(async (userId: string): Promise<boolean> => {
  if (!isIamDbConfigured()) return false;
  try {
    const row = await iamDbQueryOne<{ ok: boolean }>(
      `SELECT EXISTS (
         SELECT 1
         FROM hris.schedule_delegates sd
         JOIN hris.employees e ON e.id = sd.employee_id
         JOIN hris.schedule_delegate_branches sb ON sb.employee_id = sd.employee_id
         JOIN configuration.branches b ON b.id = sb.branch_id
         WHERE e.is_active AND e.user_id = $1 AND b.is_active
       ) AS ok`,
      [userId]
    );
    return row?.ok === true;
  } catch (error) {
    console.error("[iam] isScheduleDelegate failed:", error);
    return false;
  }
});

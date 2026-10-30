import "server-only";
import { cache } from "react";
import { iamDbQueryOne, isIamDbConfigured } from "./pg-client";

/** Halaman ESS Absensi Tim (EPIC-074); menunya hanya untuk akun yang punya bawahan aktif. */
export const ABSENSI_TIM_PATH = "/dashboard/me/absensi-tim";

/**
 * Akun ini atasan (EPIC-074 R6): karyawan AKTIF ber-`user_id` ini (satu,
 * id terkecil, sama seperti isScheduleDelegate) punya minimal satu bawahan
 * aktif langsung. Subtree tim kosong ⇔ tidak ada bawahan langsung aktif,
 * jadi cukup satu tingkat. Hanya untuk menampilkan menu; datanya tetap
 * dibatasi API. Gagal dibaca → false (menu disembunyikan).
 */
export const hasActiveTeam = cache(async (userId: string): Promise<boolean> => {
  if (!isIamDbConfigured()) return false;
  try {
    const row = await iamDbQueryOne<{ ok: boolean }>(
      `WITH me AS (
         SELECT e.id FROM hris.employees e
         WHERE e.is_active AND e.user_id = $1
         ORDER BY e.id
         LIMIT 1
       )
       SELECT EXISTS (
         SELECT 1 FROM me JOIN hris.employees r ON r.reporting_to = me.id
         WHERE r.is_active AND r.id <> me.id
       ) AS ok`,
      [userId]
    );
    return row?.ok === true;
  } catch (error) {
    console.error("[iam] hasActiveTeam failed:", error);
    return false;
  }
});

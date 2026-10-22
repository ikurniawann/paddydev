import { query, queryOne } from "@/lib/db";
import type { WorkforceActor } from "@/lib/hris/workforce-auth";

/**
 * Badge notifikasi pada menu navigasi.
 *
 * Dua arah, dengan arti yang sengaja berbeda:
 *
 * - **Sisi admin/HRD**: ada pengajuan karyawan yang MENUNGGU KEPUTUSAN.
 *   Cukup dihitung dari status 'pending', tanpa perlu penanda baca: badge
 *   hilang dengan sendirinya begitu pengajuan diputuskan.
 *
 * - **Sisi karyawan (ESS)**: ada PEMBARUAN pada pengajuan miliknya sejak
 *   terakhir ia membuka halaman itu. Perlu penanda baca
 *   (`hris.ess_module_reads`) karena "sudah dilihat" tidak bisa disimpulkan
 *   dari status mana pun.
 *
 * Kunci map adalah `route_path` menu, sehingga komponen navigasi tinggal
 * mencocokkan href tanpa tahu-menahu soal modul.
 */

/** Menu admin yang menampilkan antrean persetujuan. */
export const APPROVAL_HREFS = {
  leaves: "/dashboard/hris/leaves",
  overtime: "/dashboard/hris/overtime",
  loans: "/dashboard/hris/loans",
} as const;

/** Menu ESS yang menampilkan pembaruan pengajuan milik karyawan. */
export const ESS_HREFS = {
  leaves: "/dashboard/me/cuti",
  overtime: "/dashboard/me/lembur",
  loans: "/dashboard/me/pinjaman",
} as const;

export const ANNOUNCEMENTS_HREF = "/dashboard/me/pengumuman";

/** Menu ESS Persetujuan Tim (EPIC-063 R7): antrean atasan langsung. */
export const TEAM_APPROVALS_HREF = "/dashboard/me/persetujuan-tim";

/** Menu HR Absensi: pengajuan absen manual/koreksi menunggu HRD (EPIC-065, sama dengan Go). */
export const ATTENDANCE_REQUESTS_HREF = "/dashboard/hris/attendance";

export type EssModule = keyof typeof ESS_HREFS;

export const ESS_MODULES = Object.keys(ESS_HREFS) as EssModule[];

export function isEssModule(value: unknown): value is EssModule {
  return typeof value === "string" && (ESS_MODULES as string[]).includes(value);
}

/** Tabel sumber tiap modul. Dipakai untuk menyusun query per modul. */
const MODULE_TABLES: Record<EssModule, string> = {
  leaves: "hris.leaves",
  overtime: "hris.overtime_requests",
  loans: "hris.loans",
};

async function countOne(sql: string, params: unknown[]): Promise<number> {
  const row = await queryOne<{ total: string }>(sql, params);
  return Number(row?.total ?? 0);
}

/**
 * Pengumuman terbit yang menyasar karyawan dan belum ia baca.
 * Dipakai bersama oleh badge navigasi dan endpoint unread-count lama.
 */
export async function countUnreadAnnouncements(employeeId: string): Promise<number> {
  return countOne(
    `SELECT count(*) AS total
       FROM hris.announcements a
       JOIN hris.employees e ON e.id = $1
       LEFT JOIN hris.announcement_reads r
         ON r.announcement_id = a.id AND r.employee_id = $1
      WHERE a.status = 'published'
        AND r.employee_id IS NULL
        AND (a.publish_at IS NULL OR a.publish_at <= now())
        AND (a.expires_at IS NULL OR a.expires_at > now())
        AND (
          a.target_scope = 'global'
          OR EXISTS (
            SELECT 1 FROM hris.announcement_departments ad
            WHERE ad.announcement_id = a.id AND ad.department_id = e.department_id
          )
        )`,
    [employeeId]
  );
}

/** Jumlah pengajuan menunggu keputusan, per modul. */
async function countPendingApprovals(): Promise<Record<EssModule, number>> {
  const entries = await Promise.all(
    ESS_MODULES.map(async (module) => {
      const total = await countOne(
        `SELECT count(*) AS total FROM ${MODULE_TABLES[module]} WHERE status = 'pending'`,
        []
      );
      return [module, total] as const;
    })
  );

  return Object.fromEntries(entries) as Record<EssModule, number>;
}

/**
 * Badge Persetujuan Tim (sama dengan Go teamPendingTotal): cuti + lembur
 * karyawan + perjalanan dinas pending bawahan langsung + kasbon `pending`
 * yang menunjuk aktor sebagai atasan. reporting_to yang menunjuk diri
 * sendiri bukan bawahan (R2).
 */
async function countTeamPending(managerId: string): Promise<number> {
  const member = (alias: string) =>
    `JOIN hris.employees m ON m.id = ${alias}.employee_id AND m.reporting_to = $1 AND m.id <> $1`;
  return countOne(
    `SELECT (
        (SELECT count(*) FROM hris.leaves l ${member("l")} WHERE l.status = 'pending')
      + (SELECT count(*) FROM hris.overtime_requests o ${member("o")}
          WHERE o.status = 'pending' AND o.source = 'employee')
      + (SELECT count(*) FROM hris.business_trips t ${member("t")} WHERE t.status = 'pending')
      + (SELECT count(*) FROM hris.loans WHERE status = 'pending' AND manager_approver_id = $1)
    ) AS total`,
    [managerId]
  );
}

/**
 * Pengajuan milik karyawan yang berubah sejak terakhir ia membuka halamannya.
 *
 * Pengajuan yang masih 'pending' sengaja dikecualikan: karyawan sendiri yang
 * baru saja mengajukannya, jadi menandainya sebagai "pembaruan" hanya akan
 * memberi badge atas perbuatannya sendiri.
 */
async function countEssUpdates(employeeId: string): Promise<Record<EssModule, number>> {
  const entries = await Promise.all(
    ESS_MODULES.map(async (module) => {
      const total = await countOne(
        `SELECT count(*) AS total
           FROM ${MODULE_TABLES[module]} t
           LEFT JOIN hris.ess_module_reads r
                  ON r.employee_id = t.employee_id AND r.module = $2
          WHERE t.employee_id = $1
            AND t.status <> 'pending'
            AND t.updated_at > COALESCE(r.last_seen_at, to_timestamp(0))`,
        [employeeId, module]
      );
      return [module, total] as const;
    })
  );

  return Object.fromEntries(entries) as Record<EssModule, number>;
}

/**
 * Susun seluruh badge untuk satu aktor.
 *
 * Hitungan antrean persetujuan hanya dikembalikan kepada role HR: bukan
 * sekadar disembunyikan di UI: supaya endpoint ini tidak membocorkan keadaan
 * modul yang memang tidak boleh diakses karyawan biasa.
 */
export async function buildNavBadges(actor: WorkforceActor): Promise<Record<string, number>> {
  const badges: Record<string, number> = {};

  if (actor.isHr) {
    const pending = await countPendingApprovals();
    for (const essModule of ESS_MODULES) {
      badges[APPROVAL_HREFS[essModule]] = pending[essModule];
    }
  }

  // Hanya HRD (super_admin/admin/hrd) yang memutus pengajuan absensi (EPIC-065 R2).
  // Sama dengan Go PendingAttendanceRequestCount: semua pengajuan pending, termasuk milik HRD sendiri.
  if (actor.isHrd) {
    badges[ATTENDANCE_REQUESTS_HREF] = await countOne(
      `SELECT count(*) AS total FROM hris.attendance_requests WHERE status = 'pending'`,
      []
    );
  }

  if (actor.employeeId) {
    const [updates, announcements, team] = await Promise.all([
      countEssUpdates(actor.employeeId),
      countUnreadAnnouncements(actor.employeeId),
      countTeamPending(actor.employeeId),
    ]);
    for (const essModule of ESS_MODULES) {
      badges[ESS_HREFS[essModule]] = updates[essModule];
    }
    badges[ANNOUNCEMENTS_HREF] = announcements;
    badges[TEAM_APPROVALS_HREF] = team;
  }

  return badges;
}

/** Tandai satu modul ESS sudah dilihat karyawan (badge pembaruan hilang). */
export async function markEssModuleSeen(employeeId: string, module: EssModule): Promise<void> {
  await query(
    `INSERT INTO hris.ess_module_reads (employee_id, module, last_seen_at)
     VALUES ($1, $2, now())
     ON CONFLICT (employee_id, module)
     DO UPDATE SET last_seen_at = now()`,
    [employeeId, module]
  );
}

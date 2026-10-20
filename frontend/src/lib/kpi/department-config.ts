/**
 * Konfigurasi KPI per DEPARTEMEN (owner 2026-08-31): indikator yang
 * mempengaruhi KPI tiap departemen + bobotnya. Snapshot mengutamakan
 * pemetaan departemen; yang belum dikonfigurasi memakai pemetaan peran lama.
 * Perubahan hanya mempengaruhi snapshot BERIKUTNYA.
 */

import { z } from "zod";
import { ApiError, requireIamMenuPrefix, type ApiUser } from "@/lib/api/auth";
import { query, queryOne, withTransaction } from "@/lib/db";
import { UUID_RE } from "@/lib/hris/workforce-route";
import { IAM } from "@/lib/iam/prefixes";
import { KPI_MANAGE_ROLES } from "./roles";

/** Grant menu HRIS + role pengelola KPI. */
export async function requireKpiManager(): Promise<ApiUser> {
  // Menu pengelola Kinerja, bukan Review/Tugas Departemen karyawan (EPIC-057).
  const user = await requireIamMenuPrefix(IAM.hrisPerformanceAdmin);
  if (!(KPI_MANAGE_ROLES as readonly string[]).includes(user.role)) {
    throw ApiError.forbidden("Hanya pengelola KPI (HRD/admin) yang boleh mengubah konfigurasi");
  }
  return user;
}

const WEIGHT_RANGE = "Bobot indikator aktif harus 1–100";

export const departmentConfigSchema = z.object({
  department_id: z
    .string({ error: "Departemen tidak valid" })
    .regex(UUID_RE, "Departemen tidak valid"),
  items: z
    .array(
      z
        .object({
          indicator_id: z
            .string({ error: "ID indikator tidak valid" })
            .regex(UUID_RE, "ID indikator tidak valid"),
          enabled: z.boolean().optional(),
          weight: z.unknown().optional(),
        })
        .refine(
          (item) => {
            if (!item.enabled) return true;
            const w = Number(item.weight);
            return Number.isFinite(w) && w > 0 && w <= 100;
          },
          { message: WEIGHT_RANGE }
        )
    )
    .default([])
    .refine((items) => items.some((i) => i.enabled), {
      message: "Minimal satu indikator harus aktif untuk departemen ini",
    }),
});

export async function loadDepartmentConfig() {
  const [departments, indicators, mappings] = await Promise.all([
    query(`SELECT id, name FROM hris.departments ORDER BY name`),
    query(
      `SELECT id, code, name, description, unit, direction, default_target
       FROM performance.kpi_indicators WHERE is_active = true ORDER BY name`
    ),
    query(
      `SELECT department_id, indicator_id, weight, updated_by, updated_at
       FROM performance.kpi_department_indicators`
    ),
  ]);
  return { departments, indicators, mappings };
}

/** Tulis ulang konfigurasi satu departemen; indikator tak tercentang dihapus. */
export async function saveDepartmentConfig(
  editorName: string,
  input: z.infer<typeof departmentConfigSchema>
): Promise<string> {
  const dept = await queryOne<{ name: string }>(`SELECT name FROM hris.departments WHERE id = $1`, [
    input.department_id,
  ]);
  if (!dept) throw ApiError.notFound("Departemen tidak ditemukan");

  const enabled = input.items.filter((i) => i.enabled);
  await withTransaction(async (client) => {
    await client.query(
      `DELETE FROM performance.kpi_department_indicators WHERE department_id = $1`,
      [input.department_id]
    );
    for (const item of enabled) {
      await client.query(
        `INSERT INTO performance.kpi_department_indicators
           (department_id, indicator_id, weight, updated_by, updated_at)
         VALUES ($1, $2, $3, $4, now())`,
        [input.department_id, item.indicator_id, Number(item.weight), editorName]
      );
    }
  });
  return dept.name;
}

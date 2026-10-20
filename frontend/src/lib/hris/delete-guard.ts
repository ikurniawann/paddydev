/**
 * Guard hapus HRIS untuk route fallback Next (EPIC-061, R13): sama dengan Go
 * hrisdel.Guard — aksi "delete" pada menu daun, alasan ≥ 5 karakter, audit.
 */
import { ApiError, requireIamAction, type ApiUser } from "@/lib/api/auth";
import { recordAuditAfterCommit, requestMeta, type AuditAction, type HrisDeleteEntity } from "@/lib/audit";
import { HRIS_DELETE_REASON_ERROR, isValidDeleteReason } from "./delete-reason";

/** Menu daun HRIS yang menjaga hapus (sama dengan Go iam.Leaf*). */
export const HRIS_LEAF = {
  karyawan: "hris.kepegawaian.users",
  kontrak: "hris.kepegawaian.contracts",
  shift: "hris.kepegawaian.shifts",
  jadwalKerja: "hris.kepegawaian.schedules",
  hariLibur: "hris.kepegawaian.holidays",
  pengumuman: "hris.kepegawaian.announcements",
  penempatan: "hris.master.placements",
  departemen: "hris.master.departments",
  jabatan: "hris.master.positions",
  statusKaryawan: "hris.master.employment-statuses",
  absensi: "hris.workforce.attendance",
  cuti: "hris.workforce.leaves",
  runPenggajian: "hris.compensation.payroll",
  salary: "hris.compensation.salary",
  kpiScorecard: "hris.performance.kpi-scorecard",
  kandidat: "hris.recruitment.candidates",
  psikotes: "hris.recruitment.psikotes",
  jobPortal: "hris.recruitment.job-portal",
} as const;

async function readDeleteReason(request: Request): Promise<string> {
  const fromQuery = new URL(request.url).searchParams.get("reason")?.trim();
  if (fromQuery) return fromQuery;
  const body = (await request.clone().json().catch(() => null)) as { reason?: unknown } | null;
  return typeof body?.reason === "string" ? body.reason.trim() : "";
}

export async function requireHrisDelete(request: Request, leaf: string): Promise<{ user: ApiUser; reason: string }> {
  const user = await requireIamAction([leaf], "delete");
  const reason = await readDeleteReason(request);
  if (!isValidDeleteReason(reason)) throw ApiError.badRequest(HRIS_DELETE_REASON_ERROR);
  return { user, reason };
}

/** Go memakai "attendance.delete" (bukan hris.attendance.delete) untuk hapus absensi. */
function auditActionOf(entity: HrisDeleteEntity): AuditAction {
  return entity === "hris.attendance" ? "attendance.delete" : `${entity}.delete`;
}

export async function auditHrisDelete(
  request: Request,
  user: ApiUser,
  entity: HrisDeleteEntity,
  entityId: string | null,
  reason: string
): Promise<void> {
  await recordAuditAfterCommit({
    actor: { id: user.id, name: user.full_name },
    action: auditActionOf(entity),
    entity,
    entityId,
    reason,
    ...requestMeta(request),
  });
}

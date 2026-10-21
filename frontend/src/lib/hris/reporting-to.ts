/**
 * Atasan langsung (reporting_to) — helper murni EPIC-063 R9.
 *
 * Modul ini dipakai juga oleh komponen klien (formulir karyawan/pengguna),
 * jadi TIDAK BOLEH mengimpor modul server-only (mis. `@/lib/api/canonical-uuid`
 * yang menarik `@/lib/api/auth`; ruling P5). Normalisasi uuid dilakukan lokal.
 */

/** EPIC-063 R9: sama dengan Go errSelfManager. */
export const SELF_MANAGER_ERROR = "Atasan langsung tidak boleh karyawan itu sendiri";

/** Bentuk 32 heksa huruf kecil dari uuid (tanda hubung/kurung kurawal diabaikan); null bila bukan uuid. */
function uuidDigits(value: string): string | null {
  let s = value;
  if (s.startsWith("{") && s.endsWith("}")) s = s.slice(1, -1);
  s = s.replaceAll("-", "");
  return /^[0-9a-fA-F]{32}$/.test(s) ? s.toLowerCase() : null;
}

/** reportingTo menunjuk karyawan itu sendiri (huruf/tanda hubung apa pun). */
export function isSelfManager(employeeId: string, reportingTo: string | null | undefined): boolean {
  if (!reportingTo) return false;
  const self = uuidDigits(employeeId);
  return self !== null && self === uuidDigits(reportingTo);
}

/** Alasan karyawan aktif masuk laporan "tanpa atasan" (GET /api/hris/employees/without-manager). */
export type NoManagerReason = "empty" | "self" | "inactive";

export const NO_MANAGER_REASON_LABELS: Record<NoManagerReason, string> = {
  empty: "Atasan belum diisi",
  self: "Atasan = diri sendiri",
  inactive: "Atasan nonaktif",
};

/** Pilihan Atasan di form karyawan, tanpa karyawan yang sedang diedit (EPIC-063 R9). */
export function managerOptions(
  managers: readonly { id: string; full_name: string; nip: string }[],
  selfId: string | null
): { value: string; label: string; description: string }[] {
  return managers
    .filter((m) => !selfId || !isSelfManager(selfId, m.id))
    .map((m) => ({ value: m.id, label: m.full_name, description: m.nip }));
}

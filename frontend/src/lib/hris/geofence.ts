/**
 * Geofence absensi (EPIC-062, MOM AB-01/AB-03): label status hasil penilaian
 * server untuk ESS dan halaman Absensi HRD. Status dihitung di server; file
 * ini tidak menghitung jarak.
 */

export type GeofenceStatus = "valid" | "invalid" | "unchecked" | "dinas";

export const GEOFENCE_META: Record<GeofenceStatus, { label: string; className: string }> = {
  valid: { label: "Dalam area", className: "bg-green-100 text-green-700" },
  invalid: { label: "Di luar area", className: "bg-red-100 text-red-700" },
  unchecked: { label: "Belum dicek", className: "bg-gray-100 text-gray-600" },
  dinas: { label: "Perjalanan Dinas", className: "bg-sky-100 text-sky-700" },
};

export function isGeofenceStatus(v: unknown): v is GeofenceStatus {
  return typeof v === "string" && Object.hasOwn(GEOFENCE_META, v);
}

/** "Di luar area · 334 m"; null bila absen tidak dinilai (sebelum EPIC-062 atau dicatat HRD). */
export function geofenceLabel(
  status: string | null | undefined,
  distanceM: number | null | undefined
): string | null {
  if (!isGeofenceStatus(status)) return null;
  const label = GEOFENCE_META[status].label;
  if ((status === "valid" || status === "invalid") && distanceM != null) return `${label} · ${distanceM} m`;
  return label;
}

/** Radius bawaan (MOM AB-01); dipakai hanya bila server tidak mengirim `radius_m`. */
export const GEOFENCE_DEFAULT_RADIUS_M = 100;

/** Panjang minimal keterangan absen di luar area (MOM AB-03), dihitung per karakter (code point). */
export const GEOFENCE_NOTE_MIN = 5;

export interface GeofenceNoteInfo {
  message: string;
  distanceM: number | null;
  radiusM: number;
  branchName: string | null;
}

/**
 * Respons 422 `geofence_note_required` dari POST /api/hris/attendance: absen
 * di luar area belum tercatat dan perlu dikirim ulang dengan keterangan.
 * null untuk respons lain.
 */
export function geofenceNoteRequired(status: number, body: unknown): GeofenceNoteInfo | null {
  if (status !== 422 || !body || typeof body !== "object") return null;
  const { error, details } = body as { error?: unknown; details?: unknown };
  if (!details || typeof details !== "object") return null;
  const d = details as { code?: unknown; distance_m?: unknown; radius_m?: unknown; branch_name?: unknown };
  if (d.code !== "geofence_note_required") return null;
  return {
    message: typeof error === "string" ? error : "Anda di luar area toko. Isi keterangan untuk tetap absen.",
    distanceM: typeof d.distance_m === "number" ? d.distance_m : null,
    radiusM: typeof d.radius_m === "number" ? d.radius_m : GEOFENCE_DEFAULT_RADIUS_M,
    branchName: typeof d.branch_name === "string" ? d.branch_name : null,
  };
}

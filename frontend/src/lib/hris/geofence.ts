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

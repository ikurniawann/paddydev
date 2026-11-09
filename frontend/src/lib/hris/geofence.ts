/**
 * Geofence absensi (EPIC-062, MOM AB-01/AB-03): label status hasil penilaian
 * server untuk ESS dan halaman Absensi HRD. Status dihitung di server; file
 * ini tidak menghitung jarak.
 */

import { GEOFENCE_DEFAULT_RADIUS_M, isUsableCoord } from "@/lib/maps/coords";

export type GeofenceStatus = "valid" | "invalid" | "imprecise" | "unchecked" | "dinas";

export const GEOFENCE_META: Record<GeofenceStatus, { label: string; className: string }> = {
  valid: { label: "Dalam area", className: "bg-green-100 text-green-700" },
  invalid: { label: "Di luar area", className: "bg-red-100 text-red-700" },
  imprecise: { label: "Lokasi tidak akurat", className: "bg-amber-100 text-amber-800" },
  unchecked: { label: "Belum dicek", className: "bg-gray-100 text-gray-600" },
  dinas: { label: "Perjalanan Dinas", className: "bg-sky-100 text-sky-700" },
};

export function isGeofenceStatus(v: unknown): v is GeofenceStatus {
  return typeof v === "string" && Object.hasOwn(GEOFENCE_META, v);
}

/** "Di luar area · 334 m"; tanpa jarak untuk lokasi tidak akurat; null bila absen tidak dinilai (sebelum EPIC-062 atau dicatat HRD). */
export function geofenceLabel(
  status: string | null | undefined,
  distanceM: number | null | undefined
): string | null {
  if (!isGeofenceStatus(status)) return null;
  const label = GEOFENCE_META[status].label;
  if ((status === "valid" || status === "invalid") && distanceM != null) return `${label} · ${distanceM} m`;
  return label;
}

/** Status yang membawa keterangan karyawan (di luar area atau lokasi tidak akurat). */
export function geofenceHasNote(status: string | null | undefined): boolean {
  return status === "invalid" || status === "imprecise";
}

/**
 * Akurasi terburuk yang masih dinilai dalam/luar area (sama dengan server,
 * GeofenceImpreciseAccuracyM). Di atasnya HP mengirim lokasi perkiraan:
 * Android dengan izin "Perkiraan" atau iPhone dengan Lokasi Akurat mati
 * (titik dipatok ke kisi ±1 km, akurasi ±2000 m).
 */
export const GEOFENCE_IMPRECISE_ACCURACY_M = 500;

export function isImpreciseAccuracy(accuracy: number | null | undefined): boolean {
  return typeof accuracy === "number" && accuracy > GEOFENCE_IMPRECISE_ACCURACY_M;
}

/** Didefinisikan di lib/maps/coords (agar komponen peta tidak mengimpor lib/hris); diekspor ulang di sini. */
export { GEOFENCE_DEFAULT_RADIUS_M };

/** Panjang minimal keterangan absen di luar area (MOM AB-03), dihitung per karakter (code point). */
export const GEOFENCE_NOTE_MIN = 5;

export interface GeofenceNoteInfo {
  message: string;
  /** outside = di luar area; imprecise = lokasi HP tidak akurat (perkiraan). */
  reason: "outside" | "imprecise";
  distanceM: number | null;
  radiusM: number;
  /** Akurasi lokasi HP (meter) untuk reason imprecise. */
  accuracyM: number | null;
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
  const d = details as {
    code?: unknown;
    reason?: unknown;
    distance_m?: unknown;
    radius_m?: unknown;
    accuracy_m?: unknown;
    branch_name?: unknown;
  };
  if (d.code !== "geofence_note_required") return null;
  return {
    message: typeof error === "string" ? error : "Anda di luar area toko. Isi keterangan untuk tetap absen.",
    reason: d.reason === "imprecise" ? "imprecise" : "outside",
    distanceM: typeof d.distance_m === "number" ? d.distance_m : null,
    radiusM: typeof d.radius_m === "number" ? d.radius_m : GEOFENCE_DEFAULT_RADIUS_M,
    accuracyM: typeof d.accuracy_m === "number" ? d.accuracy_m : null,
    branchName: typeof d.branch_name === "string" ? d.branch_name : null,
  };
}

/** Batas override radius absensi per toko (meter); sama dengan server. */
const RADIUS_MIN_M = 25;
const RADIUS_MAX_M = 1000;

/** Isian dialog Lokasi Toko (HRIS), semuanya teks input. */
export interface BranchLocationForm {
  lat: string;
  lng: string;
  /** Override radius; kosong = radius bawaan */
  radius: string;
}

/** Body PUT /api/hris/branches/{id}/location: PUT penuh, kunci yang hilang menjadi null. */
export interface BranchLocationPayload {
  lat: number | null;
  lng: number | null;
  geofence_radius_m: number | null;
}

/** Validasi form Lokasi Toko; pesan sama dengan server (EPIC-062 R9). "" bila valid. */
export function validateBranchLocation(form: BranchLocationForm): string {
  const hasLat = form.lat.trim() !== "";
  const hasLng = form.lng.trim() !== "";
  if (hasLat !== hasLng) return "Latitude dan longitude wajib diisi berdua atau dikosongkan berdua";
  if (hasLat && !isUsableCoord(Number(form.lat), Number(form.lng))) {
    return "Koordinat tidak valid (latitude -90..90, longitude -180..180, bukan 0,0)";
  }
  if (form.radius.trim() !== "") {
    const r = Number(form.radius);
    if (!Number.isInteger(r) || r < RADIUS_MIN_M || r > RADIUS_MAX_M) return "Radius absensi 25–1000 meter";
  }
  return "";
}

export function toBranchLocationPayload(form: BranchLocationForm): BranchLocationPayload {
  const num = (v: string) => (v.trim() === "" ? null : Number(v));
  return { lat: num(form.lat), lng: num(form.lng), geofence_radius_m: num(form.radius) };
}

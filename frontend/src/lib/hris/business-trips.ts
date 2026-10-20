import { shortDateId } from "./placements";

/** Perjalanan Dinas (EPIC-062, MOM AB-02): label & validasi form, sama dengan server. */

export type TripStatus = "pending" | "approved" | "rejected" | "cancelled";

export const TRIP_STATUS_META: Record<TripStatus, { label: string; className: string }> = {
  pending: { label: "Menunggu", className: "bg-amber-100 text-amber-700" },
  approved: { label: "Disetujui", className: "bg-green-100 text-green-700" },
  rejected: { label: "Ditolak", className: "bg-red-100 text-red-700" },
  cancelled: { label: "Dibatalkan", className: "bg-gray-100 text-gray-600" },
};

export const TRIP_MAX_DAYS = 31;

/**
 * Peran dengan kuasa HR Perjalanan Dinas (EPIC-062 R7 "HRD"): lihat semua,
 * memutus sebagai cadangan, membatalkan yang disetujui, mencatat untuk karyawan
 * lain. Sengaja lebih sempit dari HR_ROLES — hiring_manager (peran rekrutmen)
 * tidak termasuk. Cermin tripHRRoles di backend (server tetap menentukan).
 */
export const TRIP_HR_ROLES = ["super_admin", "admin", "hrd"] as const;

export function canManageTrips(role: string | null | undefined): boolean {
  return !!role && (TRIP_HR_ROLES as readonly string[]).includes(role);
}

export interface TripForm {
  start_date: string;
  end_date: string;
  destination: string;
  purpose: string;
}

export const EMPTY_TRIP_FORM: TripForm = { start_date: "", end_date: "", destination: "", purpose: "" };

/** Jumlah hari inklusif; 0 bila tanggal kosong/tidak valid atau selesai sebelum mulai. */
export function tripDays(start: string, end: string): number {
  const s = Date.parse(`${start}T00:00:00Z`);
  const e = Date.parse(`${end}T00:00:00Z`);
  if (Number.isNaN(s) || Number.isNaN(e) || e < s) return 0;
  return Math.round((e - s) / 86_400_000) + 1;
}

/** "" bila valid; pesan sama dengan server (EPIC-062 R7). */
export function validateTripForm(form: TripForm, today: string): string {
  if (!form.start_date || !form.end_date) return "Tanggal mulai dan selesai wajib diisi";
  if (form.start_date < today) return "Tanggal mulai tidak boleh sebelum hari ini";
  if (form.end_date < form.start_date) return "Tanggal selesai tidak boleh sebelum tanggal mulai";
  if (tripDays(form.start_date, form.end_date) > TRIP_MAX_DAYS) return "Perjalanan Dinas maksimal 31 hari per pengajuan";
  if (form.destination.trim().length < 3) return "Tujuan minimal 3 karakter";
  if (form.purpose.trim().length < 5) return "Keperluan minimal 5 karakter";
  return "";
}

/** "5 Okt 2026 – 7 Okt 2026 (3 hari)". */
export function tripRangeLabel(start: string, end: string): string {
  const days = tripDays(start, end);
  const range = start === end ? shortDateId(start) : `${shortDateId(start)} – ${shortDateId(end)}`;
  return `${range} (${days} hari)`;
}

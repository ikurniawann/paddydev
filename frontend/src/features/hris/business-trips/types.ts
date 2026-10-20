import type { TripForm, TripStatus } from "@/lib/hris/business-trips";

/** Baris hris.business_trips (GET /api/hris/business-trips). */
export interface BusinessTrip {
  id: string;
  employee_id: string;
  start_date: string; // YYYY-MM-DD
  end_date: string;
  destination: string;
  purpose: string;
  status: TripStatus;
  requested_by: string | null;
  decided_by: string | null;
  decided_by_name: string | null;
  decided_at: string | null;
  rejection_reason: string | null;
  created_at: string;
  employee: { id: string; full_name: string; nip: string | null; reporting_to: string | null } | null;
}

export type TripAction = "approve" | "reject" | "cancel";

export interface TripPayload extends TripForm {
  /** HRD mencatat untuk karyawan lain (langsung disetujui) */
  employee_id?: string;
}

export interface TripListFilter {
  scope?: "approvals";
  status?: TripStatus;
  /** "me" = milik sendiri (HRD default-nya melihat semua) */
  employee_id?: string;
}

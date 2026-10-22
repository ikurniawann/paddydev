/** Toko aktif (configuration.branches) untuk pemilih penempatan & absensi. */
export interface StoreBranch {
  id: string;
  code: string;
  name: string;
  company_id: string;
  company_name: string;
  /** numeric pg sebagai string, null bila belum diisi */
  lat: string | null;
  lng: string | null;
  /** Override radius absensi (meter); null = radius bawaan */
  geofence_radius_m: number | null;
  /** Jam kerja minimum (EPIC-065 AB-05), numeric pg sebagai string; null/absen = tanpa aturan */
  min_work_hours?: string | null;
}

/** Respons PUT /api/hris/branches/{id}/location: hanya kolom yang disimpan. */
export type SavedBranchLocation = Pick<StoreBranch, "id" | "code" | "name" | "lat" | "lng" | "geofence_radius_m">;

/** Respons PUT /api/hris/branches/{id}/min-work-hours. */
export type SavedBranchMinWorkHours = Pick<StoreBranch, "id" | "code" | "name"> & {
  min_work_hours: string | null;
  /** Jumlah baris absensi periode terbuka yang dilabel ulang */
  relabelled: number;
};

/** Baris hris.employee_placements beserta nama toko. */
export interface Placement {
  id: string;
  employee_id: string;
  branch_id: string;
  branch_code: string;
  branch_name: string;
  is_primary: boolean;
  start_date: string; // YYYY-MM-DD
  end_date: string | null;
  notes: string | null;
  created_at?: string;
  updated_at?: string;
}

/** Satu karyawan aktif dengan penempatannya (halaman Penempatan Karyawan). */
export interface PlacementOverviewRow {
  employee_id: string;
  full_name: string;
  nip: string | null;
  photo_url: string | null;
  department_name: string | null;
  position_title: string | null;
  placements: Placement[];
}

export interface PlacementOverviewFilter {
  branch_id?: string;
  unplaced?: boolean;
  q?: string;
  include_ended?: boolean;
}

export interface PlacementPayload {
  branch_id?: string;
  is_primary?: boolean;
  start_date?: string;
  end_date?: string | null;
  notes?: string | null;
}

export interface BulkPlacementPayload {
  branch_id: string;
  employee_ids: string[];
  is_primary?: boolean;
  start_date?: string;
  end_date?: string | null;
}

/** Pilihan toko clock-in (GET /api/hris/me/placements). */
export interface ClockPlacement {
  id: string;
  branch_id: string;
  branch_code: string;
  branch_name: string;
  is_primary: boolean;
  lat: string | null;
  lng: string | null;
}

export interface ClockPlacements {
  placements: ClockPlacement[];
  default_branch_id: string | null;
}

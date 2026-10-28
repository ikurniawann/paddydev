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

export interface PlacementPayload {
  branch_id?: string;
  is_primary?: boolean;
  start_date?: string;
  end_date?: string | null;
  notes?: string | null;
}

/** Chip status halaman Penempatan (EPIC-070); "" = semua. */
export type PlacementStatusFilter = "" | "unplaced" | "no_schedule" | "multi" | "placed";

/** Filter GET /api/hris/employee-placements/search dan /ids (ids tanpa page/page_size). */
export interface PlacementSearchFilter {
  q?: string;
  branch_id?: string;
  department_id?: string;
  position_id?: string;
  status?: PlacementStatusFilter;
  include_ended?: boolean;
  page?: number;
  page_size?: 25 | 50 | 100;
}

/** Satu baris hasil pencarian penempatan; placements = yang berjalan kecuali include_ended. */
export interface PlacementSearchRow extends PlacementOverviewRow {
  department_id: string | null;
  position_id: string | null;
  schedule_type: "shift" | "non_shift";
  /** Ada jadwal (per tanggal/pola mingguan) yang berlaku hari ini s/d +6 hari (R5). */
  has_schedule: boolean;
  /** Tanggal jadwal per tanggal terakhir (YYYY-MM-DD); null bila tidak ada. */
  schedule_until: string | null;
}

export interface PlacementSearchCounts {
  all: number;
  unplaced: number;
  no_schedule: number;
  multi: number;
}

export interface PlacementSearchResult {
  rows: PlacementSearchRow[];
  total: number;
  page: number;
  page_size: number;
  counts: PlacementSearchCounts;
}

export interface PlacementIdsResult {
  ids: string[];
  total: number;
  /** true bila hasil filter melebihi batas 2000 id. */
  truncated: boolean;
}

export type BulkStep = "preview" | "commit";
export type BulkPlacementMode = "add" | "move";

/**
 * POST /api/hris/employee-placements/bulk. Tanpa `mode` = perilaku lama
 * (tambah langsung); dengan `mode`, `step` bawaan server adalah "preview".
 */
export interface BulkPlacementPayload {
  branch_id: string;
  employee_ids: string[];
  is_primary?: boolean;
  start_date?: string;
  end_date?: string | null;
  mode?: BulkPlacementMode;
  step?: BulkStep;
  notes?: string | null;
}

/** POST /api/hris/employee-placements/bulk-end; branch_id null = semua toko berjalan. */
export interface BulkEndPlacementPayload {
  employee_ids: string[];
  end_date: string;
  branch_id: string | null;
  step: BulkStep;
}

export type BulkOutcome = "created" | "moved" | "ended" | "skipped";

export interface BulkPlacementResultRow {
  employee_id: string;
  name: string | null;
  nip: string | null;
  outcome: BulkOutcome;
  reason: string | null;
  /** Penempatan lama yang diakhiri (pindah/akhiri). */
  ended: { branch_name: string; end_date: string }[];
}

export interface BulkPlacementResponse {
  data: {
    step?: BulkStep;
    created?: number;
    moved?: number;
    ended?: number;
    skipped: number;
    results?: BulkPlacementResultRow[];
  };
  message?: string;
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

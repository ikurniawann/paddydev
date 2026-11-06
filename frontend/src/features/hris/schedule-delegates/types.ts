/** Toko aktif dalam wilayah seorang pengatur. */
export interface DelegateBranch {
  id: string;
  code: string;
  name: string;
}

/** Satu baris GET /api/hris/schedule-delegates (EPIC-071). */
export interface ScheduleDelegate {
  employee_id: string;
  nip: string | null;
  full_name: string;
  position_title: string | null;
  has_login: boolean;
  /** Boleh memindah/menambah/mengakhiri penempatan di tokonya. */
  can_place: boolean;
  /** EPIC-093: boleh atur jadwal semua toko aktif (penempatan tetap toko di atas). */
  schedule_all_branches: boolean;
  /** Jumlah bawahan aktif. */
  direct_reports: number;
  /** false = karyawan sudah nonaktif, aksesnya otomatis tidak berlaku. */
  is_active: boolean;
  /** Hanya toko yang masih aktif. */
  branches: DelegateBranch[];
  updated_at: string | null;
  updated_by_name: string | null;
}

/** Body PUT; kedua centang selalu dikirim (server menganggap kosong sebagai false). */
export interface ScheduleDelegatePayload {
  branch_ids: string[];
  can_place: boolean;
  schedule_all_branches: boolean;
}

/** GET /api/hris/schedule-delegates/suggest. */
export interface DelegateSuggest {
  branch_ids: string[];
}

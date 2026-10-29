/** Toko dalam wilayah pengatur jadwal (hanya toko aktif). */
export interface ScheduleAccessBranch {
  id: string;
  code: string;
  name: string;
}

/** GET /api/hris/me/schedule-access (EPIC-071): penunjukan pengatur jadwal toko milik akun ini. */
export interface ScheduleAccess {
  is_delegate: boolean;
  /** Boleh memindah/menambah/mengakhiri penempatan di tokonya. */
  can_place: boolean;
  branches: ScheduleAccessBranch[];
}

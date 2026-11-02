/** Satu aturan checkout otomatis (EPIC-080 R1): jabatan + lokasi; `branch_id` null = semua lokasi. */
export interface AutoCheckoutRule {
  id: string;
  position_id: string;
  position_title: string;
  branch_id: string | null;
  branch_name: string | null;
  is_active: boolean;
  /** false = hanya lihat (pengatur jadwal toko tidak boleh mengubah aturan "semua lokasi" / toko lain). */
  editable: boolean;
}

export interface AutoCheckoutPositionOption {
  id: string;
  title: string;
}

export interface AutoCheckoutBranchOption {
  id: string;
  code: string;
  name: string;
}

/** Pilihan dialog "Tambah aturan": jabatan aktif, toko yang boleh dipilih, dan apakah "semua lokasi" boleh. */
export interface AutoCheckoutOptions {
  positions: AutoCheckoutPositionOption[];
  branches: AutoCheckoutBranchOption[];
  all_locations: boolean;
}

export interface AutoCheckoutRulePayload {
  position_id: string;
  /** null = semua lokasi */
  branch_id: string | null;
}

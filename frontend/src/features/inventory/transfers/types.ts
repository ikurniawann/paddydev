export type StockTransferKind =
  "main_to_stall" | "stall_to_stall" | "stall_to_main";

export type WarehouseOption = {
  id: string;
  name: string;
  code: string;
  is_default: boolean;
  branch_id?: string;
  branch_name?: string;
  label?: string;
};

export type StockTransferRecord = {
  id: string;
  reference_id: string;
  transfer_number: string;
  raw_material_id: string;
  material_kode: string;
  material_nama: string;
  qty: number;
  source_warehouse_id: string;
  source_warehouse_name: string;
  source_warehouse_code: string;
  dest_warehouse_id: string;
  dest_warehouse_name: string;
  dest_warehouse_code: string;
  transfer_kind: StockTransferKind | null;
  notes: string | null;
  created_at: string;
  created_by_name: string | null;
};

export type CreateStockTransferInput = {
  transfer_kind: StockTransferKind;
  source_warehouse_id: string;
  dest_warehouse_id: string;
  raw_material_id: string;
  qty: number;
  notes?: string;
};

export type StockTransferListParams = {
  page?: number;
  limit?: number;
};

export const STOCK_TRANSFER_KIND_LABELS: Record<StockTransferKind, string> = {
  main_to_stall: "Main Storage → Outlet",
  stall_to_stall: "Outlet → Outlet",
  stall_to_main: "Outlet → Main Storage",
};

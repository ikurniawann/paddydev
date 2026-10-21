import type { ProductWarehouseOption } from "./api";

/**
 * Gudang bawaan produk baru: Main Storage (is_default), selain itu baris
 * pertama. Daftar yang melintasi beberapa branch (super admin) tidak punya
 * bawaan, supaya produk tidak diam-diam masuk ke branch pertama; user harus
 * memilih.
 */
export function defaultWarehouseId(warehouses: ProductWarehouseOption[]): string {
  const first = warehouses[0];
  if (!first) return "";
  if (warehouses.some((w) => (w.branch_id ?? "") !== (first.branch_id ?? ""))) return "";
  return (warehouses.find((w) => w.is_default) ?? first).id;
}

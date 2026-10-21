import { formatNumber } from "@/lib/format";
import { stallLabel } from "@/lib/auth/stall-label";
import { sortWarehouses } from "@/lib/configuration/sort-warehouses";
import {
  isMainStorageCode,
  isStallCode,
} from "@/lib/configuration/stall-labels";
import type { WarehouseOption } from "./types";

/** Aturan murni form transfer stok (diuji unit). */

export type TransferLine = {
  key: string;
  raw_material_id: string;
  material_kode: string;
  material_nama: string;
  satuan: string | null;
  qty_available: number;
  qty_transfer_input: string;
};

const isMainStorage = (w: WarehouseOption) =>
  isMainStorageCode(w.code) || w.is_default;

function branchOf(w: { branch_id?: string }) {
  return w.branch_id ?? "";
}

/** true bila daftar outlet berasal dari lebih dari satu branch (super admin). */
export function warehousesSpanBranches(warehouses: WarehouseOption[]) {
  return warehouses.some((w) => branchOf(w) !== branchOf(warehouses[0]));
}

/**
 * Main Storage yang dikunci untuk transfer. Satu branch: Main Storage-nya,
 * seperti sebelumnya. Lintas branch: hanya Main Storage branch yang sama
 * dengan stall terpilih (server menolak lintas branch); tanpa stall terpilih
 * tidak ada yang dikunci.
 */
export function mainStorageFor(
  warehouses: WarehouseOption[],
  branchId: string | null,
): WarehouseOption | null {
  const mains = sortWarehouses(warehouses).filter(isMainStorage);
  if (!warehousesSpanBranches(warehouses)) return mains[0] ?? null;
  if (!branchId) return null;
  return mains.find((w) => branchOf(w) === branchId) ?? null;
}

/** Stall kandidat yang satu branch dengan asal (lintas branch); selain itu semua. */
export function stallsInSameBranch(
  stalls: WarehouseOption[],
  sourceId: string,
  all: WarehouseOption[],
) {
  if (!sourceId || !warehousesSpanBranches(all)) return stalls;
  const source = all.find((w) => w.id === sourceId);
  return source ? stalls.filter((w) => branchOf(w) === branchOf(source)) : stalls;
}

/** Main Storage (kode main atau default) + daftar stall, terurut. */
export function partitionWarehouses(warehouses: WarehouseOption[]) {
  const sorted = sortWarehouses(warehouses);
  const main = mainStorageFor(warehouses, null);
  const stalls = sorted.filter((w) => isStallCode(w.code));
  return { main, stalls, sorted };
}

export function toComboboxOptions(items: WarehouseOption[]) {
  return items.map((w) => ({
    value: w.id,
    label: stallLabel(w),
    description: w.code,
  }));
}

export function resolveTransferQty(line: TransferLine): number | null {
  if (line.qty_transfer_input === "") return null;
  const n = Number(line.qty_transfer_input);
  return Number.isFinite(n) ? n : null;
}

/** Baris dengan qty transfer > 0. */
export function linesToTransfer(lines: TransferLine[]) {
  return lines.filter((line) => {
    const qty = resolveTransferQty(line);
    return qty !== null && qty > 0;
  });
}

export function transferProgress(lines: TransferLine[]) {
  return {
    filled: lines.filter((line) => line.qty_transfer_input !== "").length,
    toTransfer: linesToTransfer(lines).length,
    total: lines.length,
  };
}

/** Pesan galat pertama, atau null bila transfer siap dikirim. */
export function transferInputError(
  lines: TransferLine[],
  sourceId: string,
  destId: string,
): string | null {
  if (!sourceId) return "Silakan pilih outlet asal";
  if (!destId) return "Silakan pilih outlet tujuan";
  if (sourceId === destId) return "Outlet asal dan outlet tujuan harus berbeda";
  if (linesToTransfer(lines).length === 0)
    return "Isi qty transfer lebih dari nol untuk minimal satu bahan baku";

  for (const line of lines) {
    if (line.qty_transfer_input === "") continue;
    const n = Number(line.qty_transfer_input);
    if (!Number.isFinite(n) || n < 0) {
      return "Qty transfer harus berupa angka valid yang lebih besar atau sama dengan nol";
    }
    if (n > line.qty_available) {
      return `${line.material_kode}: qty melebihi stok tersedia (${formatNumber(line.qty_available, 4)})`;
    }
  }
  return null;
}

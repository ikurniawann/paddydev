"use client";

import { useEffect, useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { MagnifyingGlassIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Combobox } from "@/components/ui/combobox";
import { PurchasingFormHeader } from "@/features/purchasing/components/shared/purchasing-page-header";
import { RM_ROUTES } from "@/lib/purchasing/item-routes";
import { createStockTransfer } from "../api";
import { useTransferSourceStock, useTransferWarehouses } from "../queries";
import { stockTransferQueryKeys } from "../query-keys";
import {
  linesToTransfer,
  mainStorageFor,
  partitionWarehouses,
  stallsInSameBranch,
  warehousesSpanBranches,
  resolveTransferQty,
  toComboboxOptions,
  transferInputError,
  transferProgress,
  type TransferLine,
} from "../transfer-lines";
import { TransferHistorySection } from "./transfer-history-section";
import { STOCK_TRANSFER_KIND_LABELS, type StockTransferKind } from "../types";
import { toast } from "sonner";
import { formatNumber } from "@/lib/format";

const TRANSFER_KIND_OPTIONS: { value: StockTransferKind; label: string }[] = [
  { value: "main_to_stall", label: STOCK_TRANSFER_KIND_LABELS.main_to_stall },
  { value: "stall_to_stall", label: STOCK_TRANSFER_KIND_LABELS.stall_to_stall },
  { value: "stall_to_main", label: STOCK_TRANSFER_KIND_LABELS.stall_to_main },
];

export function InventoryTransfersPage() {
  const queryClient = useQueryClient();
  const [transferKind, setTransferKind] =
    useState<StockTransferKind>("main_to_stall");
  const [pickedSourceId, setSourceWarehouseId] = useState("");
  const [pickedDestId, setDestWarehouseId] = useState("");
  const [notes, setNotes] = useState("");
  const [itemSearch, setItemSearch] = useState("");
  const [qtyInputs, setQtyInputs] = useState<Record<string, string>>({});
  const [submitting, setSubmitting] = useState(false);

  const warehousesQuery = useTransferWarehouses();
  const warehouses = useMemo(
    () => warehousesQuery.data || [],
    [warehousesQuery.data],
  );
  const { stalls } = useMemo(() => partitionWarehouses(warehouses), [warehouses]);
  const spansBranches = warehousesSpanBranches(warehouses);
  // Main Storage terkunci sebagai asal (main_to_stall) atau tujuan (stall_to_main).
  // Lintas branch: Main Storage milik branch stall yang dipilih (server menolak
  // transfer lintas branch); sebelum stall dipilih belum ada yang terkunci.
  const chosenStallBranch =
    (transferKind === "main_to_stall"
      ? warehouses.find((w) => w.id === pickedDestId)
      : transferKind === "stall_to_main"
        ? warehouses.find((w) => w.id === pickedSourceId)
        : undefined
    )?.branch_id ?? null;
  const main = useMemo(
    () => mainStorageFor(warehouses, chosenStallBranch),
    [warehouses, chosenStallBranch],
  );
  const sourceWarehouseId =
    transferKind === "main_to_stall" ? (main?.id ?? "") : pickedSourceId;
  const destWarehouseId =
    transferKind === "stall_to_main" ? (main?.id ?? "") : pickedDestId;

  const sourceStockQuery = useTransferSourceStock(sourceWarehouseId);
  const loading = !!sourceWarehouseId && sourceStockQuery.isLoading;

  useEffect(() => {
    if (!sourceStockQuery.isError || !sourceWarehouseId) return;
    toast.error(
      sourceStockQuery.error instanceof Error
        ? sourceStockQuery.error.message
        : "Gagal memuat bahan baku",
    );
  }, [sourceStockQuery.isError, sourceStockQuery.error, sourceWarehouseId]);

  const sourceOptions = useMemo(() => {
    if (transferKind === "main_to_stall" && main) {
      return toComboboxOptions([main]);
    }
    if (transferKind === "stall_to_stall" || transferKind === "stall_to_main") {
      return toComboboxOptions(stalls);
    }
    return [];
  }, [transferKind, main, stalls]);

  const destOptions = useMemo(() => {
    if (transferKind === "stall_to_main" && main) {
      return toComboboxOptions([main]);
    }
    if (transferKind === "main_to_stall" || transferKind === "stall_to_stall") {
      const available = stallsInSameBranch(
        stalls,
        transferKind === "stall_to_stall" ? sourceWarehouseId : "",
        warehouses,
      ).filter((stall) => stall.id !== sourceWarehouseId);
      return toComboboxOptions(available);
    }
    return [];
  }, [transferKind, main, stalls, sourceWarehouseId, warehouses]);

  const selectedSource = sourceOptions.find(
    (w) => w.value === sourceWarehouseId,
  );

  const lines = useMemo<TransferLine[]>(() => {
    if (!sourceWarehouseId || !sourceStockQuery.data) return [];
    return sourceStockQuery.data.map((item) => ({
      key: item.raw_material_id,
      raw_material_id: item.raw_material_id,
      material_kode: item.material_kode,
      material_nama: item.material_nama,
      satuan: item.satuan_besar_nama ?? item.satuan ?? null,
      qty_available: item.qty_system,
      qty_transfer_input: qtyInputs[item.raw_material_id] ?? "",
    }));
  }, [sourceWarehouseId, sourceStockQuery.data, qtyInputs]);

  const filteredLines = useMemo(() => {
    const q = itemSearch.trim().toLowerCase();
    if (!q) return lines;
    return lines.filter(
      (line) =>
        line.material_nama.toLowerCase().includes(q) ||
        line.material_kode.toLowerCase().includes(q),
    );
  }, [lines, itemSearch]);

  const progress = useMemo(() => transferProgress(lines), [lines]);

  const hasItems = lines.length > 0;
  const canSubmit = hasItems && progress.toTransfer > 0;

  const resetLines = () => {
    setQtyInputs({});
    setItemSearch("");
  };

  const handleKindChange = (kind: StockTransferKind) => {
    setTransferKind(kind);
    setSourceWarehouseId("");
    setDestWarehouseId("");
    resetLines();
  };

  const handleSourceChange = (value: string) => {
    setSourceWarehouseId(value);
    resetLines();
    if (transferKind === "stall_to_stall") {
      const sameBranch = stallsInSameBranch(stalls, value, warehouses).some(
        (stall) => stall.id === destWarehouseId,
      );
      if (value === destWarehouseId || !sameBranch) setDestWarehouseId("");
    }
  };

  const handleDestChange = (value: string) => {
    setDestWarehouseId(value);
    // Lintas branch: memilih tujuan lain mengganti Main Storage asal (stok lain).
    if (transferKind === "main_to_stall" && spansBranches) resetLines();
  };

  const handleLineChange = (key: string, value: string) => {
    setQtyInputs((prev) => ({ ...prev, [key]: value }));
  };

  const validateInputs = () => {
    const error = transferInputError(lines, sourceWarehouseId, destWarehouseId);
    if (error) toast.error(error);
    return !error;
  };

  const handleSubmit = async () => {
    if (!validateInputs()) return;

    const toTransfer = linesToTransfer(lines);

    setSubmitting(true);
    const note = notes.trim() || undefined;
    let saved = 0;
    let failed = 0;
    const savedIds: string[] = [];

    try {
      for (const line of toTransfer) {
        const qty = resolveTransferQty(line)!;
        try {
          await createStockTransfer({
            transfer_kind: transferKind,
            source_warehouse_id: sourceWarehouseId,
            dest_warehouse_id: destWarehouseId,
            raw_material_id: line.raw_material_id,
            qty,
            notes: note,
          });
          saved += 1;
          savedIds.push(line.raw_material_id);
        } catch {
          failed += 1;
        }
      }

      if (saved > 0) {
        setQtyInputs((prev) => {
          const next = { ...prev };
          for (const id of savedIds) {
            delete next[id];
          }
          return next;
        });
        await Promise.all([
          sourceStockQuery.refetch(),
          queryClient.invalidateQueries({
            queryKey: stockTransferQueryKeys.all,
          }),
        ]);
      }

      if (saved > 0 && failed === 0) {
        toast.success(`${saved} bahan baku berhasil ditransfer`);
      } else if (saved > 0) {
        toast.warning(`${saved} baris berhasil ditransfer, ${failed} gagal`);
      } else {
        toast.error("Gagal melakukan transfer stok");
      }
    } finally {
      setSubmitting(false);
    }
  };

  const sourceDisabled =
    transferKind === "main_to_stall" || submitting || warehousesQuery.isLoading;
  const destDisabled =
    transferKind === "stall_to_main" || submitting || warehousesQuery.isLoading;

  return (
    <div className="space-y-6">
      <PurchasingFormHeader
        backHref={RM_ROUTES.inventoryStock}
        title="Transfer Stok"
        description="Pindahkan stok bahan baku antara Main Storage dan outlet"
        actions={
          canSubmit ? (
            <Button
              type="button"
              className="purchasing-main-button"
              onClick={handleSubmit}
              disabled={submitting}
            >
              {submitting ? "Menyimpan..." : "Transfer Stok"}
            </Button>
          ) : undefined
        }
      />

      <Card className="border-gray-200/70 shadow-xs">
        <CardHeader className="pb-3">
          <CardTitle className="text-base">Informasi Transfer</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label className="text-xs">Jenis Transfer</Label>
            <div className="flex flex-wrap gap-2">
              {TRANSFER_KIND_OPTIONS.map((option) => (
                <Button
                  key={option.value}
                  type="button"
                  size="sm"
                  variant={
                    transferKind === option.value ? "default" : "outline"
                  }
                  className={
                    transferKind === option.value
                      ? "purchasing-main-button"
                      : "purchasing-secondary-button"
                  }
                  onClick={() => handleKindChange(option.value)}
                  disabled={submitting}
                >
                  {option.label}
                </Button>
              ))}
            </div>
          </div>

          <div className="grid grid-cols-1 gap-4 md:grid-cols-12">
            <div className="min-w-0 space-y-1.5 md:col-span-4">
              <Label className="text-xs">
                Outlet Asal <span className="text-red-500">*</span>
              </Label>
              <Combobox
                value={sourceWarehouseId}
                onChange={handleSourceChange}
                options={sourceOptions}
                placeholder={
                  warehousesQuery.isLoading
                    ? "Memuat outlet..."
                    : transferKind === "main_to_stall" && spansBranches
                      ? "Pilih outlet tujuan dulu"
                      : "Pilih outlet asal"
                }
                disabled={sourceDisabled}
                className="w-full! h-9 border-gray-200/80 text-sm"
              />
            </div>

            <div className="min-w-0 space-y-1.5 md:col-span-4">
              <Label className="text-xs">
                Outlet Tujuan <span className="text-red-500">*</span>
              </Label>
              <Combobox
                value={destWarehouseId}
                onChange={handleDestChange}
                options={destOptions}
                placeholder={
                  warehousesQuery.isLoading
                    ? "Memuat outlet..."
                    : transferKind === "stall_to_main" && spansBranches
                      ? "Pilih outlet asal dulu"
                      : "Pilih outlet tujuan"
                }
                disabled={destDisabled}
                className="w-full! h-9 border-gray-200/80 text-sm"
              />
            </div>

            <div className="min-w-0 space-y-1.5 md:col-span-4">
              <Label htmlFor="transfer-notes" className="text-xs">
                Catatan
              </Label>
              <Input
                id="transfer-notes"
                value={notes}
                onChange={(e) => setNotes(e.target.value)}
                placeholder="Catatan transfer (opsional)..."
                disabled={submitting}
                className="h-9 border-gray-200/80 text-sm"
              />
            </div>
          </div>

          {hasItems && (
            <div className="grid grid-cols-3 gap-3 border-t border-gray-200/70 pt-4">
              <div className="rounded-lg border border-gray-200/70 bg-gray-50/50 px-3 py-2">
                <p className="text-xs font-medium text-gray-500">Total Baris</p>
                <p className="text-lg font-bold text-gray-900">
                  {progress.total}
                </p>
              </div>
              <div className="rounded-lg border border-gray-200/70 bg-gray-50/50 px-3 py-2">
                <p className="text-xs font-medium text-gray-500">Terisi</p>
                <p className="text-lg font-bold text-amber-600">
                  {progress.filled}
                </p>
              </div>
              <div className="rounded-lg border border-gray-200/70 bg-gray-50/50 px-3 py-2">
                <p className="text-xs font-medium text-gray-500">
                  Akan Ditransfer
                </p>
                <p className="text-lg font-bold text-pink-600">
                  {progress.toTransfer}
                </p>
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      <Card className="border-gray-200/70 shadow-xs">
        <CardContent className="p-0">
          <div className="flex flex-col gap-3 border-b border-gray-200/70 px-4 py-4 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h2 className="text-base font-semibold text-gray-900">
                Item Transfer
              </h2>
              <p className="text-sm text-gray-500">
                {!sourceWarehouseId
                  ? "Pilih outlet asal untuk memuat bahan baku"
                  : loading
                    ? "Memuat bahan baku..."
                    : hasItems
                      ? `Isi qty transfer (kosongkan untuk melewati)${
                          selectedSource ? ` — ${selectedSource.label}` : ""
                        }`
                      : "Tidak ada bahan baku aktif di outlet ini"}
              </p>
            </div>
            {hasItems && (
              <div className="relative w-full sm:max-w-xs">
                <MagnifyingGlassIcon className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
                <Input
                  value={itemSearch}
                  onChange={(e) => setItemSearch(e.target.value)}
                  placeholder="Cari bahan baku..."
                  className="h-10 border-gray-200/80 pl-9"
                  disabled={submitting}
                />
              </div>
            )}
          </div>

          <div className="overflow-x-auto px-4 pb-4">
            <table className="w-full min-w-[800px] text-sm">
              <thead>
                <tr className="border-b border-gray-200/70 text-left text-xs font-medium uppercase tracking-wide text-gray-500">
                  <th className="px-3 py-3">Kode</th>
                  <th className="px-3 py-3">Nama Bahan Baku</th>
                  <th className="px-3 py-3">Satuan</th>
                  <th className="px-3 py-3 text-right">Stok Tersedia</th>
                  <th className="px-3 py-3 text-right">Qty Transfer</th>
                </tr>
              </thead>
              <tbody>
                {!sourceWarehouseId ? (
                  <tr>
                    <td
                      colSpan={5}
                      className="px-3 py-10 text-center text-gray-400"
                    >
                      Silakan pilih outlet asal terlebih dahulu
                    </td>
                  </tr>
                ) : loading ? (
                  <tr>
                    <td
                      colSpan={5}
                      className="px-3 py-10 text-center text-gray-400"
                    >
                      Memuat item...
                    </td>
                  </tr>
                ) : sourceStockQuery.isError ? (
                  <tr>
                    <td
                      colSpan={5}
                      className="px-3 py-10 text-center text-red-500"
                    >
                      Gagal memuat bahan baku
                    </td>
                  </tr>
                ) : filteredLines.length === 0 ? (
                  <tr>
                    <td
                      colSpan={5}
                      className="px-3 py-10 text-center text-gray-400"
                    >
                      {hasItems
                        ? "Tidak ada item yang cocok dengan pencarian"
                        : "Bahan baku aktif tidak ditemukan"}
                    </td>
                  </tr>
                ) : (
                  filteredLines.map((line) => {
                    const canTransfer = line.qty_available > 0;

                    return (
                      <tr
                        key={line.key}
                        className="border-b border-gray-200/70 hover:bg-gray-50/80"
                      >
                        <td className="px-3 py-3 font-mono text-xs text-gray-600">
                          {line.material_kode}
                        </td>
                        <td className="px-3 py-3 font-medium text-gray-900">
                          {line.material_nama}
                        </td>
                        <td className="px-3 py-3 text-gray-600">
                          {line.satuan || "—"}
                        </td>
                        <td
                          className={`px-3 py-3 text-right ${
                            canTransfer ? "text-gray-700" : "text-gray-400"
                          }`}
                        >
                          {formatNumber(line.qty_available, 4)}
                        </td>
                        <td className="px-3 py-3 text-right">
                          <Input
                            type="number"
                            min={0}
                            max={line.qty_available}
                            step="any"
                            value={line.qty_transfer_input}
                            onChange={(e) =>
                              handleLineChange(line.key, e.target.value)
                            }
                            placeholder="—"
                            disabled={submitting || !canTransfer}
                            className="ml-auto h-9 w-28 border-gray-200/80 text-right text-sm disabled:bg-gray-50/80"
                          />
                        </td>
                      </tr>
                    );
                  })
                )}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>

      <TransferHistorySection />
    </div>
  );
}

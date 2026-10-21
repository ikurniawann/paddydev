"use client";

import { useMemo, useState } from "react";
import { stallLabel } from "@/lib/auth/stall-label";
import { MagnifyingGlassIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Combobox } from "@/components/ui/combobox";
import { DsDateTimePicker } from "@/components/design-system";
import { PurchasingFormHeader } from "@/features/purchasing/components/shared/purchasing-page-header";
import { RM_ROUTES } from "@/lib/purchasing/item-routes";
import {
  useStockOpnamePreview,
  useStockOpnameWarehouses,
} from "@/features/inventory/stock-opname/queries";
import { toast } from "sonner";
import { formatNumber } from "@/lib/format";
import { fetchJson } from "@/features/inventory/shared/inventory-lookups";

type AdjustLine = {
  key: string;
  raw_material_id: string;
  material_kode: string;
  material_nama: string;
  satuan: string | null;
  qty_system: number;
  qty_actual_input: string;
};

export function ManualAdjustmentPage() {
  const warehousesQuery = useStockOpnameWarehouses();

  const [warehouseId, setWarehouseId] = useState("");
  const [adjustDate, setAdjustDate] = useState(
    new Date().toISOString().slice(0, 10),
  );
  const [notes, setNotes] = useState("");
  const [itemSearch, setItemSearch] = useState("");
  const [qtyInputs, setQtyInputs] = useState<Record<string, string>>({});
  const [submitting, setSubmitting] = useState(false);

  const previewQuery = useStockOpnamePreview(warehouseId);
  const loading = !!warehouseId && previewQuery.isLoading;

  const warehouseOptions = (warehousesQuery.data || []).map((w) => ({
    value: w.id,
    label: stallLabel(w),
    description: w.code,
  }));
  const selectedWarehouse = warehouseOptions.find(
    (w) => w.value === warehouseId,
  );

  const lines = useMemo<AdjustLine[]>(() => {
    if (!warehouseId || !previewQuery.data) return [];
    return previewQuery.data.map((item) => ({
      key: item.raw_material_id,
      raw_material_id: item.raw_material_id,
      material_kode: item.material_kode,
      material_nama: item.material_nama,
      satuan: item.satuan_besar_nama ?? item.satuan ?? null,
      qty_system: item.qty_system,
      qty_actual_input: qtyInputs[item.raw_material_id] ?? "",
    }));
  }, [warehouseId, previewQuery.data, qtyInputs]);

  const filteredLines = useMemo(() => {
    const q = itemSearch.trim().toLowerCase();
    if (!q) return lines;
    return lines.filter(
      (line) =>
        line.material_nama.toLowerCase().includes(q) ||
        line.material_kode.toLowerCase().includes(q),
    );
  }, [lines, itemSearch]);

  const progress = useMemo(() => {
    const filled = lines.filter((line) => line.qty_actual_input !== "").length;
    const variance = lines.filter((line) => {
      if (line.qty_actual_input === "") return false;
      const n = Number(line.qty_actual_input);
      return Number.isFinite(n) && n !== line.qty_system;
    }).length;
    return { filled, variance, total: lines.length };
  }, [lines]);

  const hasItems = lines.length > 0;

  const handleWarehouseChange = (value: string) => {
    setWarehouseId(value);
    setQtyInputs({});
    setItemSearch("");
  };

  const handleLineChange = (key: string, value: string) => {
    setQtyInputs((prev) => ({ ...prev, [key]: value }));
  };

  const handleFillSystem = () => {
    setQtyInputs((prev) => {
      const next = { ...prev };
      for (const line of lines) {
        next[line.raw_material_id] = String(line.qty_system);
      }
      return next;
    });
  };

  const resolveQty = (line: AdjustLine): number | null => {
    if (line.qty_actual_input === "") return null;
    const n = Number(line.qty_actual_input);
    return Number.isFinite(n) ? n : null;
  };

  const validateInputs = () => {
    if (!warehouseId) {
      toast.error("Pilih outlet terlebih dahulu");
      return false;
    }

    const toSave = lines.filter((line) => line.qty_actual_input !== "");
    if (toSave.length === 0) {
      toast.error("Isi stok baru untuk minimal satu bahan baku");
      return false;
    }

    const invalid = lines.find((line) => {
      if (line.qty_actual_input === "") return false;
      const n = Number(line.qty_actual_input);
      return !Number.isFinite(n) || n < 0;
    });
    if (invalid) {
      toast.error(
        "Stok baru harus berupa angka lebih besar atau sama dengan nol",
      );
      return false;
    }

    const withVariance = toSave.filter(
      (line) => resolveQty(line) !== line.qty_system,
    );
    if (withVariance.length === 0) {
      toast.error("Tidak ada selisih stok untuk disimpan");
      return false;
    }

    return true;
  };

  const buildNote = () => {
    const base = notes.trim();
    const dateLabel = adjustDate
      ? `Penyesuaian ${adjustDate}`
      : "Penyesuaian Stok";
    return base ? `${dateLabel}: ${base}` : dateLabel;
  };

  const handleSubmit = async () => {
    if (!validateInputs()) return;

    const toSave = lines.filter((line) => {
      const qty = resolveQty(line);
      return qty !== null && qty !== line.qty_system;
    });

    setSubmitting(true);
    const note = buildNote();
    let saved = 0;
    let failed = 0;
    const savedIds: string[] = [];

    try {
      for (const line of toSave) {
        const qty = resolveQty(line)!;
        try {
          await fetchJson("/api/purchasing/inventory/adjustment", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
              raw_material_id: line.raw_material_id,
              warehouse_id: warehouseId,
              qty_actual: qty,
              notes: note,
            }),
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
        await previewQuery.refetch();
      }

      if (saved > 0 && failed === 0) {
        toast.success(`${saved} bahan baku berhasil disesuaikan`);
      } else if (saved > 0) {
        toast.warning(`${saved} baris tersimpan, ${failed} gagal`);
      } else {
        toast.error("Gagal menyimpan penyesuaian stok");
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="space-y-6">
      <PurchasingFormHeader
        backHref={RM_ROUTES.inventoryStock}
        title="Penyesuaian Stok"
        description="Koreksi stok bahan baku secara manual per outlet"
        actions={
          hasItems ? (
            <>
              <Button
                type="button"
                variant="outline"
                className="purchasing-secondary-button"
                onClick={handleFillSystem}
                disabled={submitting}
              >
                Isi dengan Stok Saat Ini
              </Button>
              <Button
                type="button"
                className="purchasing-main-button"
                onClick={handleSubmit}
                disabled={submitting}
              >
                {submitting ? "Menyimpan..." : "Simpan Penyesuaian"}
              </Button>
            </>
          ) : undefined
        }
      />

      <Card className="border-gray-200/70 shadow-xs">
        <CardHeader className="pb-3">
          <CardTitle className="text-base">Informasi Penyesuaian</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid grid-cols-1 gap-4 md:grid-cols-12">
            <div className="min-w-0 space-y-1.5 md:col-span-4">
              <Label className="text-xs">
                Outlet <span className="text-red-500">*</span>
              </Label>
              <Combobox
                value={warehouseId}
                onChange={handleWarehouseChange}
                options={warehouseOptions}
                placeholder={
                  warehousesQuery.isLoading ? "Memuat outlet..." : "Pilih outlet"
                }
                disabled={warehousesQuery.isLoading || submitting}
                className="w-full! h-9 border-gray-200/80 text-sm"
              />
            </div>

            <div className="min-w-0 md:col-span-3">
              <DsDateTimePicker
                label="Tanggal Penyesuaian"
                value={adjustDate}
                onChange={setAdjustDate}
                placeholder="Pilih tanggal penyesuaian..."
                dateOnly
                disabled={submitting}
              />
            </div>

            <div className="min-w-0 space-y-1.5 md:col-span-5">
              <Label htmlFor="notes" className="text-xs">
                Catatan
              </Label>
              <Input
                id="notes"
                value={notes}
                onChange={(e) => setNotes(e.target.value)}
                placeholder="Alasan penyesuaian (opsional)..."
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
                <p className="text-xs font-medium text-gray-500">Ada Selisih</p>
                <p className="text-lg font-bold text-pink-600">
                  {progress.variance}
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
                Koreksi Stok
              </h2>
              <p className="text-sm text-gray-500">
                {!warehouseId
                  ? "Pilih outlet untuk memuat bahan baku"
                  : loading
                    ? "Memuat bahan baku..."
                    : hasItems
                      ? `Isi stok baru${
                          selectedWarehouse
                            ? ` — ${selectedWarehouse.label}`
                            : ""
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
                  <th className="px-3 py-3 text-right">Stok Saat Ini</th>
                  <th className="px-3 py-3 text-right">Stok Baru</th>
                  <th className="px-3 py-3 text-right">Selisih</th>
                </tr>
              </thead>
              <tbody>
                {!warehouseId ? (
                  <tr>
                    <td
                      colSpan={6}
                      className="px-3 py-10 text-center text-gray-400"
                    >
                      Pilih outlet terlebih dahulu
                    </td>
                  </tr>
                ) : loading ? (
                  <tr>
                    <td
                      colSpan={6}
                      className="px-3 py-10 text-center text-gray-400"
                    >
                      Memuat item...
                    </td>
                  </tr>
                ) : previewQuery.isError ? (
                  <tr>
                    <td
                      colSpan={6}
                      className="px-3 py-10 text-center text-red-500"
                    >
                      Gagal memuat bahan baku
                    </td>
                  </tr>
                ) : filteredLines.length === 0 ? (
                  <tr>
                    <td
                      colSpan={6}
                      className="px-3 py-10 text-center text-gray-400"
                    >
                      {hasItems
                        ? "Data tidak ditemukan"
                        : "Tidak ada bahan baku aktif"}
                    </td>
                  </tr>
                ) : (
                  filteredLines.map((line) => {
                    const actual =
                      line.qty_actual_input === ""
                        ? null
                        : Number(line.qty_actual_input);
                    const variance =
                      actual === null || !Number.isFinite(actual)
                        ? null
                        : actual - line.qty_system;

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
                        <td className="px-3 py-3 text-right text-gray-700">
                          {formatNumber(line.qty_system, 4)}
                        </td>
                        <td className="px-3 py-3 text-right">
                          <Input
                            type="number"
                            min={0}
                            step="any"
                            value={line.qty_actual_input}
                            onChange={(e) =>
                              handleLineChange(line.key, e.target.value)
                            }
                            placeholder="0"
                            disabled={submitting}
                            className="ml-auto h-9 w-28 border-gray-200/80 text-right text-sm"
                          />
                        </td>
                        <td
                          className={`px-3 py-3 text-right font-medium ${
                            variance === null
                              ? "text-gray-400"
                              : variance === 0
                                ? "text-gray-600"
                                : variance > 0
                                  ? "text-emerald-600"
                                  : "text-red-600"
                          }`}
                        >
                          {variance === null ? "—" : formatNumber(variance, 4)}
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
    </div>
  );
}

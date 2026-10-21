"use client";

import { useMemo, useState } from "react";
import { stallLabel } from "@/lib/auth/stall-label";
import { MagnifyingGlassIcon } from "@heroicons/react/24/outline";
import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Combobox } from "@/components/ui/combobox";
import { DsDateTimePicker } from "@/components/design-system";
import { STALL_LABELS } from "@/lib/configuration/stall-labels";
import { PurchasingFormHeader } from "@/features/purchasing/components/shared/purchasing-page-header";
import { PRODUCT_ROUTES } from "@/lib/purchasing/item-routes";
import {
  useWarehouseLookup,
  type WarehouseLookup,
} from "@/features/inventory/shared/inventory-lookups";
import { adjustProductStock, type AdjustLine } from "../api";
import { useStallAdjustLines } from "../queries";
import { toast } from "sonner";
import { formatNumber } from "@/lib/format";

const NO_WAREHOUSES: WarehouseLookup[] = [];
const NO_LINES: AdjustLine[] = [];

export function ProductManualAdjustmentPage() {
  const [warehouseId, setWarehouseId] = useState("");
  const warehousesQuery = useWarehouseLookup();
  const warehouses = warehousesQuery.data ?? NO_WAREHOUSES;
  const loadingWarehouses = warehousesQuery.isLoading;
  const [adjustDate, setAdjustDate] = useState(
    new Date().toISOString().slice(0, 10),
  );
  const [notes, setNotes] = useState("");
  const [itemSearch, setItemSearch] = useState("");
  const linesQuery = useStallAdjustLines(warehouseId);
  const loading = linesQuery.isFetching;
  // Suntingan per stall; null = belum diubah sejak data stall dimuat.
  const [edited, setEdited] = useState<{
    warehouseId: string;
    lines: AdjustLine[];
  } | null>(null);
  const lines =
    edited?.warehouseId === warehouseId
      ? edited.lines
      : warehouseId
        ? (linesQuery.data ?? NO_LINES)
        : NO_LINES;
  const setLines = (update: (prev: AdjustLine[]) => AdjustLine[]) =>
    setEdited((current) => ({
      warehouseId,
      lines: update(
        current?.warehouseId === warehouseId
          ? current.lines
          : (linesQuery.data ?? NO_LINES),
      ),
    }));
  const [submitting, setSubmitting] = useState(false);

  const warehouseOptions = warehouses.map((w) => ({
    value: w.id,
    label: stallLabel(w),
    description: w.code,
  }));
  const selectedWarehouse = warehouseOptions.find(
    (w) => w.value === warehouseId,
  );

  const filteredLines = useMemo(() => {
    const q = itemSearch.trim().toLowerCase();
    if (!q) return lines;
    return lines.filter(
      (line) =>
        line.product_nama.toLowerCase().includes(q) ||
        line.product_kode.toLowerCase().includes(q),
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

  const hasItems = Boolean(warehouseId) && lines.length > 0;

  const handleLineChange = (key: string, value: string) => {
    setLines((prev) =>
      prev.map((line) =>
        line.key === key ? { ...line, qty_actual_input: value } : line,
      ),
    );
  };

  const handleFillSystem = () => {
    setLines((prev) =>
      prev.map((line) => ({
        ...line,
        qty_actual_input: String(line.qty_system),
      })),
    );
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
      toast.error("Isi stok baru untuk minimal satu produk");
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
    const stallLabel = selectedWarehouse?.label || STALL_LABELS.singular;
    const base = notes.trim();
    const dateLabel = adjustDate
      ? `Penyesuaian ${adjustDate}`
      : "Penyesuaian Stok";
    const prefix = `${dateLabel} (${stallLabel})`;
    return base ? `${prefix}: ${base}` : prefix;
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

    try {
      for (const line of toSave) {
        const qty = resolveQty(line)!;
        try {
          await adjustProductStock({
            product_id: line.product_id,
            qty_actual: qty,
            notes: note,
          });
          saved += 1;
          setLines((prev) =>
            prev.map((row) =>
              row.key === line.key
                ? { ...row, qty_system: qty, qty_actual_input: "" }
                : row,
            ),
          );
        } catch {
          failed += 1;
        }
      }

      if (saved > 0 && failed === 0) {
        toast.success(`${saved} produk berhasil disesuaikan`);
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
        backHref={PRODUCT_ROUTES.inventoryStock}
        title="Penyesuaian Stok Produk"
        description="Koreksi stok produk jadi secara manual per outlet"
        actions={
          hasItems ? (
            <>
              <Button
                type="button"
                variant="outline"
                className="purchasing-secondary-button w-full sm:w-auto"
                onClick={handleFillSystem}
                disabled={submitting}
              >
                Isi dengan Stok Saat Ini
              </Button>
              <Button
                type="button"
                className="purchasing-main-button w-full sm:w-auto"
                onClick={handleSubmit}
                disabled={submitting}
              >
                {submitting ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    Menyimpan...
                  </>
                ) : (
                  "Simpan Penyesuaian"
                )}
              </Button>
            </>
          ) : undefined
        }
      />

      <Card className="border-gray-200/70 shadow-xs">
        <CardHeader className="border-b border-gray-200/70 pb-3">
          <CardTitle className="text-base">Informasi Penyesuaian</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4 p-4">
          <div className="grid grid-cols-1 gap-4 md:grid-cols-12">
            <div className="min-w-0 md:col-span-4">
              <Label className="text-xs">
                Outlet <span className="text-red-500">*</span>
              </Label>
              <Combobox
                options={warehouseOptions}
                value={warehouseId}
                onChange={setWarehouseId}
                placeholder={
                  loadingWarehouses
                    ? STALL_LABELS.loading
                    : STALL_LABELS.selectPlaceholder
                }
                searchPlaceholder={STALL_LABELS.search}
                emptyMessage={STALL_LABELS.empty}
                disabled={loadingWarehouses || submitting}
                className="mt-1.5 h-9 text-sm"
              />
              {selectedWarehouse && (
                <p className="mt-1 text-xs text-gray-500">
                  {selectedWarehouse.description}
                </p>
              )}
            </div>
            <div className="min-w-0 md:col-span-4">
              <DsDateTimePicker
                label="Tanggal Penyesuaian"
                value={adjustDate}
                onChange={setAdjustDate}
                placeholder="Pilih tanggal penyesuaian..."
                dateOnly
                disabled={submitting}
              />
            </div>

            <div className="min-w-0 space-y-1.5 md:col-span-4">
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
                  ? "Pilih outlet untuk memuat produk"
                  : loading
                    ? "Memuat daftar produk..."
                    : hasItems
                      ? "Isi stok baru untuk produk yang perlu dikoreksi"
                      : "Tidak ada produk aktif di outlet ini"}
              </p>
            </div>
            {hasItems && (
              <div className="relative w-full sm:max-w-xs">
                <MagnifyingGlassIcon className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
                <Input
                  value={itemSearch}
                  onChange={(e) => setItemSearch(e.target.value)}
                  placeholder="Cari produk..."
                  className="h-10 border-gray-200/80 pl-9 text-sm"
                  disabled={submitting}
                />
              </div>
            )}
          </div>

          <div className="overflow-x-auto px-4 pb-4">
            <table className="min-w-full text-sm">
              <thead className="border-b border-gray-100 bg-gray-50 text-xs uppercase tracking-wide text-gray-500">
                <tr>
                  <th className="px-4 py-3 text-left font-semibold">Kode</th>
                  <th className="px-4 py-3 text-left font-semibold">
                    Nama Produk
                  </th>
                  <th className="px-4 py-3 text-left font-semibold">Satuan</th>
                  <th className="px-4 py-3 text-right font-semibold">
                    Stok Saat Ini
                  </th>
                  <th className="px-4 py-3 text-right font-semibold">
                    Stok Baru
                  </th>
                  <th className="px-4 py-3 text-right font-semibold">
                    Selisih
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {!warehouseId ? (
                  <tr>
                    <td
                      colSpan={6}
                      className="px-4 py-10 text-center text-gray-400"
                    >
                      Pilih outlet terlebih dahulu
                    </td>
                  </tr>
                ) : loading ? (
                  <tr>
                    <td
                      colSpan={6}
                      className="px-4 py-10 text-center text-gray-400"
                    >
                      <Loader2 className="mx-auto mb-2 h-5 w-5 animate-spin text-pink-600" />
                      Memuat item...
                    </td>
                  </tr>
                ) : filteredLines.length === 0 ? (
                  <tr>
                    <td
                      colSpan={6}
                      className="px-4 py-10 text-center text-gray-400"
                    >
                      {hasItems
                        ? "Data tidak ditemukan"
                        : "Tidak ada produk aktif untuk outlet ini"}
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
                      <tr key={line.key} className="hover:bg-gray-50">
                        <td className="px-4 py-3 font-mono text-xs text-gray-600">
                          {line.product_kode}
                        </td>
                        <td className="px-4 py-3 font-medium text-gray-900">
                          {line.product_nama}
                        </td>
                        <td className="px-4 py-3 text-gray-600">
                          {line.satuan || "—"}
                        </td>
                        <td className="px-4 py-3 text-right text-gray-700">
                          {formatNumber(line.qty_system, 4)}
                        </td>
                        <td className="px-4 py-3 text-right">
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
                          className={`px-4 py-3 text-right font-medium ${
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

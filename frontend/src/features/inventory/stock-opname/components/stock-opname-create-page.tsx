"use client";

import { useEffect, useMemo, useState } from "react";
import { stallLabel } from "@/lib/auth/stall-label";
import { useRouter } from "next/navigation";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Combobox } from "@/components/ui/combobox";
import { DsDateTimePicker } from "@/components/design-system";
import { PurchasingFormHeader } from "@/features/purchasing/components/shared/purchasing-page-header";
import {
  hasSmallUnit,
  toDisplayQty,
  type RawMaterialUnitMode,
} from "@/lib/inventory/raw-material-units";
import { RM_ROUTES } from "@/lib/purchasing/item-routes";
import { RawMaterialUnitSelect } from "./raw-material-unit-select";
import { UnitModeToggle } from "./unit-mode-toggle";
import {
  OpnameCountList,
  type OpnameCountItem,
} from "@/features/inventory/opname-shared";
import {
  countProgress,
  countedLineUpdates,
  lineUnitInfo,
  linesFromDetail,
  linesFromPreview,
  modeFor,
  qtyInputError,
  resolveBaseQty,
  withSystemQty,
  withUnitMode,
  type CountLine,
} from "../count-lines";
import type { StockOpnameDetail, StockOpnamePreviewLine } from "../types";
import {
  useStockOpname,
  useStockOpnamePreview,
  useStockOpnameWarehouses,
} from "../queries";
import {
  useCompleteStockOpname,
  useCreateStockOpname,
  useUpdateStockOpname,
} from "../mutations";
import { toast } from "sonner";
import {
  OpnameDesktopActions,
  OpnameMobileActions,
  OpnameProgressTiles,
} from "@/features/inventory/opname-shared/opname-actions";

/**
 * Satuan hitung default untuk SEMUA bahan yang punya konversi (mis. Karung ↔
 * Kg). Dipilih sebelum mulai menghitung; masih bisa diubah per item. Diingat
 * di perangkat (localStorage) supaya petugas gudang tidak memilih ulang.
 */
const UNIT_MODE_STORAGE_KEY = "opname:unit-mode";

function readStoredUnitMode(): RawMaterialUnitMode {
  if (typeof window === "undefined") return "besar";
  try {
    return window.localStorage.getItem(UNIT_MODE_STORAGE_KEY) === "kecil"
      ? "kecil"
      : "besar";
  } catch {
    return "besar";
  }
}

const NO_LINES: CountLine[] = [];
const NO_PREVIEW: StockOpnamePreviewLine[] = [];

interface StockOpnameCreatePageProps {
  opnameId?: string;
}

export function StockOpnameCreatePage({
  opnameId,
}: StockOpnameCreatePageProps) {
  const router = useRouter();
  const detailQuery = useStockOpname(opnameId || "");
  const detail = detailQuery.data;
  const isClosed =
    detail?.status === "completed" || detail?.status === "cancelled";

  // Sesi yang sudah selesai/batal tidak bisa dilanjutkan: arahkan ke detail.
  useEffect(() => {
    if (detail && isClosed)
      router.replace(RM_ROUTES.inventoryOpnameDetail(detail.id));
  }, [detail, isClosed, router]);

  if (!opnameId) return <StockOpnameEditor />;
  if (detailQuery.isLoading) {
    return (
      <div className="py-16 text-center text-sm text-gray-400">
        Memuat sesi stok opname...
      </div>
    );
  }
  if (!detail || isClosed) return null;
  return <StockOpnameEditor key={detail.id} detail={detail} />;
}

function StockOpnameEditor({ detail }: { detail?: StockOpnameDetail }) {
  const router = useRouter();
  const opnameId = detail?.id;
  const isContinue = Boolean(detail);

  const warehousesQuery = useStockOpnameWarehouses();
  const createMutation = useCreateStockOpname();
  const updateMutation = useUpdateStockOpname();
  const completeMutation = useCompleteStockOpname();

  const [warehouseId, setWarehouseId] = useState(detail?.warehouse_id || "");
  const [opnameDate, setOpnameDate] = useState(
    () =>
      detail?.opname_date?.slice(0, 10) ||
      new Date().toISOString().slice(0, 10),
  );
  const [notes, setNotes] = useState(detail?.notes || "");
  const [itemSearch, setItemSearch] = useState("");
  const [unitMode, setUnitMode] =
    useState<RawMaterialUnitMode>(readStoredUnitMode);

  const previewQuery = useStockOpnamePreview(
    !isContinue && warehouseId ? warehouseId : "",
  );

  // Sesi lanjutan memakai baris server; sesi baru memakai pratinjau stall terpilih.
  // Suntingan disimpan per sumber supaya ganti stall memuat baris stall itu.
  const sourceKey = isContinue ? "detail" : warehouseId;
  const [edited, setEdited] = useState<{
    source: string;
    lines: CountLine[];
  } | null>(() =>
    detail
      ? {
          source: "detail",
          lines: linesFromDetail(detail.lines || [], unitMode),
        }
      : null,
  );
  const baseLines = useMemo(
    () =>
      isContinue || !warehouseId
        ? NO_LINES
        : linesFromPreview(previewQuery.data ?? NO_PREVIEW, unitMode),
    [isContinue, warehouseId, previewQuery.data, unitMode],
  );
  const lines = edited?.source === sourceKey ? edited.lines : baseLines;
  const setLines = (update: (prev: CountLine[]) => CountLine[]) =>
    setEdited((current) => ({
      source: sourceKey,
      lines: update(current?.source === sourceKey ? current.lines : baseLines),
    }));

  const isBusy =
    createMutation.isPending ||
    updateMutation.isPending ||
    completeMutation.isPending;

  const warehouseOptions = (warehousesQuery.data || []).map((w) => ({
    value: w.id,
    label: stallLabel(w),
    description: w.code,
  }));

  const selectedWarehouse = warehouseOptions.find(
    (w) => w.value === warehouseId,
  );

  /** Ganti satuan hitung untuk semua bahan yang punya konversi; input yang sudah diisi dikonversi. */
  const handleUnitModeAll = (next: RawMaterialUnitMode) => {
    setUnitMode(next);
    try {
      window.localStorage.setItem(UNIT_MODE_STORAGE_KEY, next);
    } catch {
      /* penyimpanan lokal tidak tersedia — abaikan */
    }
    setLines((prev) =>
      prev.map((line) => withUnitMode(line, modeFor(lineUnitInfo(line), next))),
    );
  };
  const linesWithSmallUnit = lines.filter((line) =>
    hasSmallUnit(lineUnitInfo(line)),
  ).length;

  const handleFillSystemLine = (key: string) => {
    setLines((prev) =>
      prev.map((line) => (line.key === key ? withSystemQty(line) : line)),
    );
  };

  const countItems: OpnameCountItem[] = lines.map((line) => {
    const unit = lineUnitInfo(line);
    const displaySystem = toDisplayQty(
      line.qty_system,
      line.input_unit_mode,
      unit,
    );
    const counted =
      line.qty_counted_input === "" ? null : Number(line.qty_counted_input);
    return {
      key: line.key,
      code: line.material_kode,
      name: line.material_nama,
      unit: (
        <RawMaterialUnitSelect
          info={unit}
          value={line.input_unit_mode}
          onChange={(mode) => handleRowUnitModeChange(line.key, mode)}
          disabled={isBusy}
        />
      ),
      qtySystem: displaySystem,
      qtyInput: line.qty_counted_input,
      variance:
        counted === null || !Number.isFinite(counted)
          ? null
          : counted - displaySystem,
    };
  });

  const progress = useMemo(() => countProgress(lines), [lines]);

  const hasItems = lines.length > 0;
  const isPreviewLoading =
    !isContinue && !!warehouseId && previewQuery.isLoading;

  const handleWarehouseChange = (value: string) => {
    setWarehouseId(value);
    setItemSearch("");
  };

  const handleLineChange = (key: string, value: string) => {
    setLines((prev) =>
      prev.map((line) =>
        line.key === key ? { ...line, qty_counted_input: value } : line,
      ),
    );
  };

  const handleRowUnitModeChange = (key: string, next: RawMaterialUnitMode) => {
    setLines((prev) =>
      prev.map((line) => (line.key === key ? withUnitMode(line, next) : line)),
    );
  };

  const handleFillSystem = () => {
    setLines((prev) => prev.map(withSystemQty));
  };

  const validateQtyInputs = (requireAll: boolean) => {
    const error = qtyInputError(lines, requireAll);
    if (error) toast.error(error);
    return !error;
  };

  const handleSaveDraft = async () => {
    if (!warehouseId) {
      toast.error("Pilih outlet terlebih dahulu");
      return;
    }
    if (!hasItems) {
      toast.error("Tidak ada bahan baku yang tersedia untuk stok opname");
      return;
    }
    if (!validateQtyInputs(false)) return;

    try {
      if (isContinue && opnameId) {
        await updateMutation.mutateAsync({
          id: opnameId,
          input: {
            notes: notes.trim() || undefined,
            lines: lines.map((line) => ({
              id: line.lineId!,
              qty_counted: resolveBaseQty(line),
            })),
          },
        });
        toast.success("Draf stok opname berhasil disimpan");
        return;
      }

      const created = await createMutation.mutateAsync({
        warehouse_id: warehouseId,
        opname_date: opnameDate,
        notes: notes.trim() || undefined,
        reason: "stock_opname",
      });

      const updates = countedLineUpdates(lines, created.lines || []);

      if (updates.length > 0) {
        await updateMutation.mutateAsync({
          id: created.id,
          input: { lines: updates },
        });
      }

      toast.success("Draf stok opname berhasil disimpan");
      router.replace(RM_ROUTES.inventoryOpnameContinue(created.id));
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Gagal menyimpan draf",
      );
    }
  };

  const handleComplete = async () => {
    if (!warehouseId) {
      toast.error("Pilih outlet terlebih dahulu");
      return;
    }
    if (!hasItems) {
      toast.error("Tidak ada bahan baku yang tersedia untuk stok opname");
      return;
    }
    if (!validateQtyInputs(true)) return;

    try {
      let sessionId = opnameId;

      if (!sessionId) {
        const created = await createMutation.mutateAsync({
          warehouse_id: warehouseId,
          opname_date: opnameDate,
          notes: notes.trim() || undefined,
          reason: "stock_opname",
        });
        sessionId = created.id;

        await updateMutation.mutateAsync({
          id: sessionId,
          input: {
            lines: lines.map((line) => {
              const createdLine = created.lines?.find(
                (l) => l.raw_material_id === line.raw_material_id,
              );
              if (!createdLine) {
                throw new Error(
                  `Baris tidak ditemukan untuk ${line.material_kode}`,
                );
              }
              return {
                id: createdLine.id,
                qty_counted: resolveBaseQty(line) ?? 0,
              };
            }),
          },
        });
      } else {
        await updateMutation.mutateAsync({
          id: sessionId,
          input: {
            notes: notes.trim() || undefined,
            lines: lines.map((line) => ({
              id: line.lineId!,
              qty_counted: resolveBaseQty(line) ?? 0,
            })),
          },
        });
      }

      await completeMutation.mutateAsync(sessionId);
      toast.success(
        "Stok opname berhasil diselesaikan dan persediaan telah disesuaikan",
      );
      router.push(RM_ROUTES.inventoryOpnameDetail(sessionId!));
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : "Gagal menyelesaikan stok opname",
      );
    }
  };

  const handleCancel = async () => {
    if (!opnameId || !window.confirm("Batalkan sesi stok opname ini?")) return;
    try {
      await updateMutation.mutateAsync({
        id: opnameId,
        input: { status: "cancelled" },
      });
      toast.success("Stok opname berhasil dibatalkan");
      router.push(RM_ROUTES.inventoryOpname);
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Gagal membatalkan sesi",
      );
    }
  };

  return (
    <div className="space-y-6 pb-24 md:pb-0">
      <PurchasingFormHeader
        backHref={RM_ROUTES.inventoryOpname}
        title={isContinue ? "Lanjutkan Stok Opname" : "Buat Stok Opname"}
        description="Pilih outlet, masukkan qty fisik, lalu simpan sebagai draf atau selesaikan opname"
        actions={
          hasItems ? (
            <OpnameDesktopActions
              busy={isBusy}
              disabled={!warehouseId}
              saving={updateMutation.isPending}
              completing={completeMutation.isPending}
              onFillSystem={handleFillSystem}
              onCancel={isContinue ? handleCancel : undefined}
              onSaveDraft={handleSaveDraft}
              onComplete={handleComplete}
            />
          ) : undefined
        }
      />

      <Card className="border-gray-200/70 shadow-xs">
        <CardHeader className="pb-3">
          <CardTitle className="text-base">Informasi Opname</CardTitle>
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
                disabled={warehousesQuery.isLoading || isBusy || isContinue}
                className="w-full! h-9 border-gray-200/80 text-sm"
              />
            </div>

            <div className="min-w-0 md:col-span-3">
              <DsDateTimePicker
                label="Tanggal Opname"
                value={opnameDate}
                onChange={setOpnameDate}
                placeholder="Pilih tanggal opname..."
                dateOnly
                disabled={isBusy}
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
                placeholder="Catatan tambahan (opsional)..."
                disabled={isBusy}
                className="h-9 border-gray-200/80 text-sm"
              />
            </div>
          </div>

          <div className="space-y-1.5 border-t border-gray-200/70 pt-4">
            <Label className="text-xs">Satuan hitung</Label>
            <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:gap-3">
              <UnitModeToggle
                unitMode={unitMode}
                disabled={isBusy}
                onChange={handleUnitModeAll}
              />
              <p className="text-xs text-gray-500">
                {hasItems
                  ? linesWithSmallUnit > 0
                    ? `Berlaku untuk ${linesWithSmallUnit} bahan yang punya konversi satuan; bahan lain tetap satuan dasarnya. Masih bisa diubah per item.`
                    : "Tidak ada bahan dengan konversi satuan di outlet ini."
                  : "Pilih sebelum mulai menghitung; bisa diubah per item nanti."}
              </p>
            </div>
          </div>

          {hasItems && <OpnameProgressTiles progress={progress} />}
        </CardContent>
      </Card>

      {warehouseId && (
        <Card className="border-gray-200/70 shadow-xs">
          <CardContent className="p-0">
            <div className="flex flex-col gap-3 border-b border-gray-200/70 px-4 py-4 sm:flex-row sm:items-center sm:justify-between">
              <div>
                <h2 className="text-base font-semibold text-gray-900">
                  Perhitungan Stok Fisik
                  {selectedWarehouse ? ` — ${selectedWarehouse.label}` : ""}
                </h2>
                <p className="text-sm text-gray-500">
                  {isPreviewLoading
                    ? "Memuat bahan baku..."
                    : hasItems
                      ? "Masukkan qty fisik dari hasil perhitungan outlet"
                      : "Tidak ada bahan baku aktif di cabang outlet ini"}
                </p>
              </div>
            </div>

            <div className="px-4 pb-4">
              <OpnameCountList
                items={countItems}
                nameLabel="Nama Bahan Baku"
                search={itemSearch}
                onSearchChange={setItemSearch}
                searchPlaceholder="Cari bahan baku..."
                busy={isBusy}
                loading={isPreviewLoading}
                emptyText="Pilih outlet untuk memuat bahan baku"
                onQtyChange={handleLineChange}
                onFillSystem={handleFillSystemLine}
              />
            </div>
          </CardContent>
        </Card>
      )}

      {hasItems && (
        <OpnameMobileActions
          busy={isBusy}
          disabled={!warehouseId}
          saving={updateMutation.isPending}
          completing={completeMutation.isPending}
          onFillSystem={handleFillSystem}
          onCancel={isContinue ? handleCancel : undefined}
          onSaveDraft={handleSaveDraft}
          onComplete={handleComplete}
        />
      )}
    </div>
  );
}

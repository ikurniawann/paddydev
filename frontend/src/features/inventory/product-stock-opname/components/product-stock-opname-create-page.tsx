"use client";

import { useEffect, useMemo, useState } from "react";
import { stallLabel } from "@/lib/auth/stall-label";
import { useRouter } from "next/navigation";
import { Loader2 } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Combobox } from "@/components/ui/combobox";
import { DsDateTimePicker } from "@/components/design-system";
import { STALL_LABELS } from "@/lib/configuration/stall-labels";
import { PurchasingFormHeader } from "@/features/purchasing/components/shared/purchasing-page-header";
import { PRODUCT_ROUTES } from "@/lib/purchasing/item-routes";
import {
  OpnameCountList,
  type OpnameCountItem,
} from "@/features/inventory/opname-shared";
import {
  useProductStockOpname,
  useProductStockOpnamePreview,
  useProductStockOpnameWarehouses,
} from "../queries";
import {
  useCompleteProductStockOpname,
  useCreateProductStockOpname,
  useUpdateProductStockOpname,
} from "../mutations";
import { toast } from "sonner";
import {
  productCountProgress,
  productCountedUpdates,
  productLinesFromDetail,
  productLinesFromPreview,
  productQtyInputError,
  resolveQty,
  type ProductCountLine,
} from "../count-lines";
import type {
  ProductStockOpnameDetail,
  ProductStockOpnamePreviewLine,
} from "../types";
import {
  OpnameDesktopActions,
  OpnameMobileActions,
  OpnameProgressTiles,
} from "@/features/inventory/opname-shared/opname-actions";

const NO_LINES: ProductCountLine[] = [];
const NO_PREVIEW: ProductStockOpnamePreviewLine[] = [];

interface ProductStockOpnameCreatePageProps {
  opnameId?: string;
}

export function ProductStockOpnameCreatePage({
  opnameId,
}: ProductStockOpnameCreatePageProps) {
  const router = useRouter();
  const detailQuery = useProductStockOpname(opnameId || "");
  const detail = detailQuery.data;
  const isClosed =
    detail?.status === "completed" || detail?.status === "cancelled";

  // Sesi yang sudah selesai/batal tidak bisa dilanjutkan: arahkan ke detail.
  useEffect(() => {
    if (detail && isClosed)
      router.replace(PRODUCT_ROUTES.inventoryOpnameDetail(detail.id));
  }, [detail, isClosed, router]);

  if (!opnameId) return <ProductStockOpnameEditor />;
  if (detailQuery.isLoading) return <OpnameSessionLoading />;
  if (!detail || isClosed) return null;
  return <ProductStockOpnameEditor key={detail.id} detail={detail} />;
}

function OpnameSessionLoading() {
  return (
    <div className="flex items-center justify-center py-16 text-sm text-gray-500">
      <Loader2 className="mr-2 h-5 w-5 animate-spin text-pink-600" />
      Memuat sesi stok opname produk...
    </div>
  );
}

function ProductStockOpnameEditor({
  detail,
}: {
  detail?: ProductStockOpnameDetail;
}) {
  const router = useRouter();
  const opnameId = detail?.id;
  const isContinue = Boolean(detail);

  const warehousesQuery = useProductStockOpnameWarehouses();
  const createMutation = useCreateProductStockOpname();
  const updateMutation = useUpdateProductStockOpname();
  const completeMutation = useCompleteProductStockOpname();

  const [warehouseId, setWarehouseId] = useState(
    detail?.warehouse_id || detail?.warehouse?.id || "",
  );
  const [opnameDate, setOpnameDate] = useState(
    () =>
      detail?.opname_date?.slice(0, 10) ||
      new Date().toISOString().slice(0, 10),
  );
  const [notes, setNotes] = useState(detail?.notes || "");
  const [itemSearch, setItemSearch] = useState("");

  const previewQuery = useProductStockOpnamePreview(
    !isContinue && warehouseId ? warehouseId : "",
  );

  // Sesi lanjutan memakai baris server; sesi baru memakai pratinjau stall terpilih.
  const sourceKey = isContinue ? "detail" : warehouseId;
  const [edited, setEdited] = useState<{
    source: string;
    lines: ProductCountLine[];
  } | null>(() =>
    detail
      ? { source: "detail", lines: productLinesFromDetail(detail.lines || []) }
      : null,
  );
  const baseLines = useMemo(
    () =>
      isContinue || !warehouseId
        ? NO_LINES
        : productLinesFromPreview(previewQuery.data ?? NO_PREVIEW),
    [isContinue, warehouseId, previewQuery.data],
  );
  const lines = edited?.source === sourceKey ? edited.lines : baseLines;
  const setLines = (update: (prev: ProductCountLine[]) => ProductCountLine[]) =>
    setEdited((current) => ({
      source: sourceKey,
      lines: update(current?.source === sourceKey ? current.lines : baseLines),
    }));

  const warehouseOptions = (warehousesQuery.data || []).map((w) => ({
    value: w.id,
    label: stallLabel(w),
    description: w.code,
  }));
  const selectedWarehouse = warehouseOptions.find(
    (w) => w.value === warehouseId,
  );

  const isBusy =
    createMutation.isPending ||
    updateMutation.isPending ||
    completeMutation.isPending;

  const handleFillSystemLine = (key: string) => {
    setLines((prev) =>
      prev.map((line) =>
        line.key === key
          ? { ...line, qty_counted_input: String(line.qty_system) }
          : line,
      ),
    );
  };

  const countItems = useMemo<OpnameCountItem[]>(
    () =>
      lines.map((line) => {
        const counted =
          line.qty_counted_input === "" ? null : Number(line.qty_counted_input);
        return {
          key: line.key,
          code: line.product_kode,
          name: line.product_nama,
          subtitle: line.pos_sku_id
            ? `Varian: ${line.pos_sku_code || "—"} — ${line.pos_sku_name || "—"}`
            : null,
          unit: line.satuan || "—",
          qtySystem: line.qty_system,
          qtyInput: line.qty_counted_input,
          variance:
            counted === null || !Number.isFinite(counted)
              ? null
              : counted - line.qty_system,
        };
      }),
    [lines],
  );

  const progress = useMemo(() => productCountProgress(lines), [lines]);

  const hasItems = lines.length > 0;
  const isPreviewLoading = !isContinue && previewQuery.isLoading;

  const handleLineChange = (key: string, value: string) => {
    setLines((prev) =>
      prev.map((line) =>
        line.key === key ? { ...line, qty_counted_input: value } : line,
      ),
    );
  };

  const handleFillSystem = () => {
    setLines((prev) =>
      prev.map((line) => ({
        ...line,
        qty_counted_input: String(line.qty_system),
      })),
    );
  };

  const validateQtyInputs = (requireAll: boolean) => {
    const error = productQtyInputError(lines, requireAll);
    if (error) toast.error(error);
    return !error;
  };

  const handleSaveDraft = async () => {
    if (!warehouseId) {
      toast.error("Pilih outlet terlebih dahulu");
      return;
    }
    if (!hasItems) {
      toast.error("Tidak ada produk yang tersedia untuk stok opname");
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
              qty_counted: resolveQty(line),
            })),
          },
        });
        toast.success("Draf stok opname produk berhasil disimpan");
        return;
      }

      const created = await createMutation.mutateAsync({
        warehouse_id: warehouseId,
        opname_date: opnameDate,
        notes: notes.trim() || undefined,
        reason: "stock_opname",
      });

      const updates = productCountedUpdates(lines, created.lines || []);

      if (updates.length > 0) {
        await updateMutation.mutateAsync({
          id: created.id,
          input: { lines: updates },
        });
      }

      toast.success("Draf stok opname produk berhasil disimpan");
      router.replace(PRODUCT_ROUTES.inventoryOpnameContinue(created.id));
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
      toast.error("Tidak ada produk yang tersedia untuk stok opname");
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
                (l) =>
                  l.product_id === line.product_id &&
                  (l.pos_sku_id ?? null) === (line.pos_sku_id ?? null),
              );
              if (!createdLine) {
                throw new Error(
                  `Baris tidak ditemukan untuk ${line.product_kode}`,
                );
              }
              return {
                id: createdLine.id,
                qty_counted: resolveQty(line) ?? 0,
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
              qty_counted: resolveQty(line) ?? 0,
            })),
          },
        });
      }

      await completeMutation.mutateAsync(sessionId);
      toast.success(
        "Stok opname produk berhasil diselesaikan dan persediaan telah disesuaikan",
      );
      router.push(PRODUCT_ROUTES.inventoryOpnameDetail(sessionId!));
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
      toast.success("Stok opname produk berhasil dibatalkan");
      router.push(PRODUCT_ROUTES.inventoryOpname);
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Gagal membatalkan sesi",
      );
    }
  };

  return (
    <div className="space-y-6 pb-24 md:pb-0">
      <PurchasingFormHeader
        backHref={PRODUCT_ROUTES.inventoryOpname}
        title={
          isContinue
            ? "Lanjutkan Stok Opname Produk"
            : "Buat Stok Opname Produk"
        }
        description="Masukkan qty fisik produk jadi, lalu simpan sebagai draf atau selesaikan opname"
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
        <CardHeader className="border-b border-gray-200/70 pb-3">
          <CardTitle className="text-base">Informasi Opname</CardTitle>
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
                  warehousesQuery.isLoading
                    ? STALL_LABELS.loading
                    : STALL_LABELS.selectPlaceholder
                }
                searchPlaceholder={STALL_LABELS.search}
                emptyMessage={STALL_LABELS.empty}
                disabled={isBusy || isContinue || warehousesQuery.isLoading}
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
                label="Tanggal Opname"
                value={opnameDate}
                onChange={setOpnameDate}
                placeholder="Pilih tanggal opname..."
                dateOnly
                disabled={isBusy}
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
                placeholder="Catatan tambahan (opsional)..."
                disabled={isBusy}
                className="h-9 border-gray-200/80 text-sm"
              />
            </div>
          </div>

          {hasItems && <OpnameProgressTiles progress={progress} />}
        </CardContent>
      </Card>

      <Card className="border-gray-200/70 shadow-xs">
        <CardContent className="p-0">
          <div className="flex flex-col gap-3 border-b border-gray-200/70 px-4 py-4 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h2 className="text-base font-semibold text-gray-900">
                Perhitungan Stok Fisik
              </h2>
              <p className="text-sm text-gray-500">
                {isPreviewLoading
                  ? "Memuat produk..."
                  : hasItems
                    ? "Masukkan qty fisik dari hasil perhitungan produk"
                    : "Tidak ada produk aktif di outlet ini"}
              </p>
            </div>
          </div>

          <div className="px-4 pb-4">
            <OpnameCountList
              items={countItems}
              nameLabel="Nama Produk"
              search={itemSearch}
              onSearchChange={setItemSearch}
              searchPlaceholder="Cari produk..."
              busy={isBusy}
              loading={isPreviewLoading}
              emptyText="Tidak ada produk aktif"
              onQtyChange={handleLineChange}
              onFillSystem={handleFillSystemLine}
            />
          </div>
        </CardContent>
      </Card>

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

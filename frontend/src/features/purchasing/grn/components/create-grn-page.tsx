"use client";

import { useEffect, useState } from "react";
import { stallLabel } from "@/lib/auth/stall-label";
import { useRouter, useSearchParams } from "next/navigation";
import { ClipboardCheck, Loader2Icon, TruckIcon } from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Combobox } from "@/components/ui/combobox";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { DsDateTimePicker } from "@/components/design-system";
import {
  PurchasingFormFooter,
  PurchasingFormHeader,
} from "@/features/purchasing/components/shared/purchasing-page-header";
import {
  buildCreateGrnLines,
  buildCreateGrnPayload,
  createGrnTotals,
  validateCreateGrn,
  type CreateGrnLineEdit,
} from "@/lib/purchasing/grn-ui-lines";
import { PRODUCT_ROUTES, RM_ROUTES } from "@/lib/purchasing/item-routes";
import { todayIsoDate } from "@/lib/purchasing/po-ui-detail";
import type { PurchasingModuleType } from "../api";
import { useCreateGrn } from "../mutations";
import { CreateGrnItemsTable } from "./create-grn-items-table";
import { GuidelinesBox, InfoField, ItemPreviewBox, SummaryRow } from "./receiving-form-parts";
import { useCreateGrnSources } from "./use-create-grn-sources";

const GUIDELINES = [
  "Pilih pengiriman yang belum pernah diterima.",
  "Gudang tujuan dan tanggal penerimaan wajib diisi.",
  "Qty Diterima / QC tidak boleh melebihi sisa qty PO.",
  "Kekurangan vs sisa PO otomatis masuk kolom Tolak / QC Gagal.",
  "Stok diposting dari qty Diterima / QC.",
  "Isi nomor batch dan tanggal kedaluwarsa dari label supplier. Tanggal kosong memakai umur simpan bahan baku.",
];

export function CreateGrnPage({ moduleType = "raw_material" }: { moduleType?: PurchasingModuleType }) {
  const isProduct = moduleType === "product";
  const listRoute = isProduct ? PRODUCT_ROUTES.purchasingReceive : RM_ROUTES.purchasingGrn;

  const router = useRouter();
  const presetDeliveryId = useSearchParams().get("delivery_id");
  const createMutation = useCreateGrn();

  const [deliveryId, setDeliveryId] = useState(presetDeliveryId ?? "");
  // null = belum dipilih: gudang tunggal dipilih otomatis.
  const [warehouseChoice, setWarehouseChoice] = useState<string | null>(null);
  const [tanggal, setTanggal] = useState(todayIsoDate);
  const [catatan, setCatatan] = useState("");
  const [edits, setEdits] = useState<Record<string, CreateGrnLineEdit>>({});

  const sources = useCreateGrnSources(moduleType, deliveryId);
  const { selectedDelivery, warehouses } = sources;
  const warehouseId = warehouseChoice ?? (warehouses.length === 1 ? warehouses[0].id : "");
  const lines = buildCreateGrnLines(sources.poLines, edits);
  const totals = createGrnTotals(lines);

  const presetMissing =
    Boolean(presetDeliveryId) &&
    sources.deliveriesLoaded &&
    !sources.deliveries.some((delivery) => delivery.id === presetDeliveryId);
  useEffect(() => {
    if (presetMissing) {
      toast.error("Pengiriman ini sudah punya GRN atau tidak memenuhi syarat.", { id: "grn-preset-delivery" });
    }
  }, [presetMissing]);

  const selectDelivery = (value: string) => {
    setDeliveryId(value);
    setWarehouseChoice(null);
    setEdits({});
  };

  const editLine = (poItemId: string, edit: CreateGrnLineEdit) =>
    setEdits((prev) => ({ ...prev, [poItemId]: { ...prev[poItemId], ...edit } }));

  const form = {
    deliveryId: selectedDelivery?.id ?? "",
    warehouseId,
    tanggal_penerimaan: tanggal,
    catatan,
  };
  const canSubmit = Boolean(form.deliveryId && form.warehouseId && form.tanggal_penerimaan && lines.length > 0);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const error = validateCreateGrn(form, lines);
    if (error) {
      toast.error(error);
      return;
    }
    try {
      const result = await createMutation.mutateAsync(buildCreateGrnPayload(form, lines, moduleType));
      toast.success("GRN berhasil dibuat. QC selesai dan stok sudah diposting.");
      const createdId = result.data?.id;
      if (!createdId) router.push(listRoute);
      else router.push(isProduct ? PRODUCT_ROUTES.purchasingReceiveDetail(createdId) : RM_ROUTES.purchasingGrnDetail(createdId));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Gagal membuat GRN.");
    }
  };

  return (
    <div className="space-y-6">
      <PurchasingFormHeader
        backHref={listRoute}
        title="Buat GRN"
        description="Catat penerimaan fisik dan QC dalam satu langkah. Stok diposting dari qty Diterima / QC."
      />

      <form id="create-grn-form" onSubmit={handleSubmit} className="space-y-6">
        <div className="grid grid-cols-1 gap-6 xl:grid-cols-12">
          <div className="space-y-6 xl:col-span-8">
            <Card className="border-gray-200/70 shadow-xs">
              <CardHeader className="border-b border-gray-200/70 pb-3">
                <CardTitle className="flex items-center gap-2 text-base">
                  <TruckIcon className="h-4 w-4" />
                  Informasi Penerimaan
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-4 pt-4">
                <div className="min-w-0 space-y-1.5">
                  <Label className="text-xs">
                    Pengiriman <span className="text-red-500">*</span>
                  </Label>
                  <Combobox
                    options={sources.deliveries.map((delivery) => ({
                      value: delivery.id,
                      label: delivery.no_resi || delivery.no_surat_jalan,
                      description: `${delivery.supplier_name || delivery.kurir || "-"} · ${delivery.po_number || delivery.no_surat_jalan || "-"}`,
                    }))}
                    value={form.deliveryId}
                    onChange={selectDelivery}
                    placeholder={sources.fetchingDeliveries ? "Memuat pengiriman..." : "Pilih pengiriman"}
                    searchPlaceholder="Cari no. resi atau surat jalan..."
                    emptyMessage="Tidak ada pengiriman yang memenuhi syarat"
                    allowClear
                    disabled={sources.fetchingDeliveries}
                    className="w-full! h-9 text-sm"
                  />
                </div>

                {selectedDelivery && (
                  <div className="grid grid-cols-1 gap-3 rounded-xl border border-gray-200/70 bg-gray-50/60 p-4 text-sm md:grid-cols-2">
                    <InfoField label="No. Resi">{selectedDelivery.no_resi || "-"}</InfoField>
                    <InfoField label="No. Surat Jalan">{selectedDelivery.no_surat_jalan || "-"}</InfoField>
                    <InfoField label="Kurir">{selectedDelivery.kurir || "-"}</InfoField>
                    <InfoField label="Status">
                      <Badge variant="outline" className="mt-1 font-medium capitalize">
                        {selectedDelivery.status}
                      </Badge>
                    </InfoField>
                  </div>
                )}

                <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                  <div className="min-w-0 space-y-1.5">
                    <Label className="text-xs">
                      Gudang Tujuan <span className="text-red-500">*</span>
                    </Label>
                    <Combobox
                      options={warehouses.map((warehouse) => ({
                        value: warehouse.id,
                        label: stallLabel(warehouse),
                        description: warehouse.code,
                      }))}
                      value={warehouseId}
                      onChange={setWarehouseChoice}
                      placeholder={sources.fetchingWarehouses ? "Memuat gudang..." : "Pilih gudang tujuan"}
                      searchPlaceholder="Cari gudang..."
                      emptyMessage={sources.fetchingWarehouses ? "Memuat..." : "Gudang tidak ditemukan"}
                      allowClear
                      disabled={!selectedDelivery || sources.fetchingWarehouses}
                      className="w-full! h-9 text-sm"
                    />
                  </div>

                  <DsDateTimePicker
                    label="Tanggal Penerimaan"
                    value={tanggal}
                    onChange={setTanggal}
                    placeholder="Pilih tanggal penerimaan..."
                    dateOnly
                    required
                    disabled={!selectedDelivery}
                  />
                </div>

                <div className="min-w-0 space-y-1.5">
                  <Label htmlFor="catatan" className="text-xs">
                    Catatan
                  </Label>
                  <Textarea
                    id="catatan"
                    value={catatan}
                    onChange={(e) => setCatatan(e.target.value)}
                    placeholder="Tambahkan catatan jika perlu..."
                    rows={3}
                    className="resize-none text-sm"
                    disabled={!selectedDelivery}
                  />
                </div>
              </CardContent>
            </Card>
          </div>

          <div className="xl:col-span-4">
            <Card className="border-gray-200/70 shadow-xs xl:sticky xl:top-6">
              <CardHeader className="border-b border-gray-200/70 pb-3">
                <CardTitle className="text-base">Ringkasan</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4 pt-4">
                {selectedDelivery ? (
                  <dl className="space-y-3 text-sm">
                    <SummaryRow label="Purchase Order">{selectedDelivery.po_number || "-"}</SummaryRow>
                    <SummaryRow label={isProduct ? "Vendor" : "Supplier"}>{selectedDelivery.supplier_name || "-"}</SummaryRow>
                    <SummaryRow label="Diterima / QC" strong>{totals.accepted}</SummaryRow>
                    <SummaryRow label="Tolak / QC Gagal">{totals.rejected}</SummaryRow>
                  </dl>
                ) : (
                  <p className="text-sm text-gray-500">Pilih pengiriman untuk melihat ringkasan penerimaan.</p>
                )}
                <GuidelinesBox lines={GUIDELINES} />
                <ItemPreviewBox
                  items={lines.map((line) => ({
                    key: line.purchase_order_item_id,
                    name: line.nama_bahan,
                    qty: line.qty_diterima,
                  }))}
                />
              </CardContent>
            </Card>
          </div>
        </div>

        <Card className="border-gray-200/70 shadow-xs">
          <CardHeader className="border-b border-gray-200/70 pb-3">
            <CardTitle className="flex items-center gap-2 text-base">
              <ClipboardCheck className="h-4 w-4" />
              Konfirmasi Item Diterima
            </CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            {!selectedDelivery ? (
              <div className="py-12 text-center text-sm text-gray-500">
                Pilih pengiriman untuk memuat item purchase order secara otomatis.
              </div>
            ) : sources.fetchingPoLines ? (
              <div className="flex items-center justify-center py-12 text-sm text-gray-500">
                <Loader2Icon className="mr-2 h-4 w-4 animate-spin" />
                Memuat item purchase order...
              </div>
            ) : lines.length === 0 ? (
              <div className="py-12 text-center text-sm text-gray-500">
                {sources.poLines.length > 0
                  ? "Semua item PO sudah terpenuhi — tidak ada sisa untuk diterima."
                  : "Tidak ada item purchase order untuk pengiriman ini."}
              </div>
            ) : (
              <CreateGrnItemsTable lines={lines} isProduct={isProduct} onEdit={editLine} />
            )}
          </CardContent>
        </Card>

        <PurchasingFormFooter
          formId="create-grn-form"
          onCancel={() => router.push(listRoute)}
          submitLabel="Simpan GRN"
          loading={createMutation.isPending}
          disabled={!canSubmit}
        />
      </form>
    </div>
  );
}

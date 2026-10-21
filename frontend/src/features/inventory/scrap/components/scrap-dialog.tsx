"use client";

import { useState } from "react";
import { stallLabel } from "@/lib/auth/stall-label";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Combobox } from "@/components/ui/combobox";
import {
  Dialog,
  DialogFooter,
  DialogPanel,
  DialogPanelBody,
  DialogPanelDescription,
  DialogPanelForm,
  DialogPanelHeader,
  DialogPanelTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import {
  SCRAP_REASON_OPTIONS,
  type ScrapReason,
} from "@/lib/inventory/scrap-reasons";
import {
  fetchJson,
  useMaterialLookup,
  useWarehouseLookup,
} from "../../shared/inventory-lookups";
import { formatDate, formatNumber } from "@/lib/format";

type OpenBatch = {
  id: string;
  batch_number: string | null;
  expiry_date: string | null;
  qty_remaining: number;
};

/** Batch yang dikunci dari halaman Stok Kedaluwarsa (write-off satu batch). */
export interface ScrapBatchPreset {
  batchId: string;
  batchNumber: string | null;
  expiryDate: string | null;
  rawMaterialId: string;
  materialName: string;
  warehouseId: string;
  warehouseName: string | null;
  qtyRemaining: number;
  satuan: string | null;
}

const NO_BATCH = "__fefo__";

/**
 * Form scrap/write-off. Tanpa `preset` user memilih gudang, bahan, dan
 * (opsional) batch; dengan `preset` batch sudah terkunci dan alasan default
 * "Kedaluwarsa".
 */
export function ScrapDialog({
  preset,
  onClose,
  onDone,
}: {
  preset?: ScrapBatchPreset | null;
  onClose: () => void;
  onDone?: () => void;
}) {
  const queryClient = useQueryClient();
  const warehouses = useWarehouseLookup();
  const materials = useMaterialLookup();
  const [warehouseId, setWarehouseId] = useState(preset?.warehouseId ?? "");
  const [materialId, setMaterialId] = useState(preset?.rawMaterialId ?? "");
  const [batchId, setBatchId] = useState(preset?.batchId ?? NO_BATCH);
  const [qty, setQty] = useState(preset ? String(preset.qtyRemaining) : "");
  const [reason, setReason] = useState<ScrapReason>(
    preset ? "expired" : "damaged",
  );
  const [notes, setNotes] = useState("");

  const batches = useQuery({
    queryKey: ["inventory-batches", materialId, warehouseId],
    queryFn: async () =>
      (
        await fetchJson<{ data: OpenBatch[] }>(
          `/api/inventory/batches?raw_material_id=${materialId}&warehouse_id=${warehouseId}`,
        )
      ).data,
    enabled: !preset && Boolean(materialId && warehouseId),
  });

  const selectedBatch = batches.data?.find((b) => b.id === batchId);
  const maxQty = preset?.qtyRemaining ?? selectedBatch?.qty_remaining;
  const qtyNumber = Number(qty);
  const valid =
    Boolean(warehouseId && materialId) &&
    qtyNumber > 0 &&
    (maxQty === undefined || qtyNumber <= maxQty);

  const save = useMutation({
    mutationFn: () =>
      fetchJson<{ message: string }>("/api/inventory/scrap", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          raw_material_id: materialId,
          warehouse_id: warehouseId,
          qty: qtyNumber,
          reason,
          notes: notes.trim() || null,
          batch_id: batchId === NO_BATCH ? null : batchId,
        }),
      }),
    onSuccess: (data) => {
      toast.success(data.message);
      void queryClient.invalidateQueries({ queryKey: ["inventory-scrap"] });
      void queryClient.invalidateQueries({ queryKey: ["inventory-expiry"] });
      void queryClient.invalidateQueries({ queryKey: ["inventory-batches"] });
      onDone?.();
      onClose();
    },
    onError: (error) =>
      toast.error("Scrap gagal dicatat", { description: error.message }),
  });

  return (
    <Dialog open onOpenChange={(open) => !open && !save.isPending && onClose()}>
      <DialogPanel size="lg">
        <DialogPanelForm
          onSubmit={(e) => {
            e.preventDefault();
            if (valid) save.mutate();
          }}
        >
          <DialogPanelHeader>
            <DialogPanelTitle>
              {preset
                ? `Write-off batch ${preset.batchNumber ?? "tanpa nomor"}`
                : "Catat scrap"}
            </DialogPanelTitle>
            <DialogPanelDescription>
              Stok gudang berkurang lewat mutasi scrap, jurnal selisih
              persediaan diposting, dan tindakan tercatat di audit trail.
            </DialogPanelDescription>
          </DialogPanelHeader>
          <DialogPanelBody className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            {preset ? (
              <div className="rounded-2xl bg-surface px-4 py-3 text-sm sm:col-span-2">
                <p className="font-medium">{preset.materialName}</p>
                <p className="text-muted-foreground">
                  {preset.warehouseName ?? "Gudang"} · kedaluwarsa{" "}
                  {formatDate(preset.expiryDate)} · sisa{" "}
                  {formatNumber(preset.qtyRemaining, 3)} {preset.satuan ?? ""}
                </p>
              </div>
            ) : (
              <>
                <label className="flex min-w-0 flex-col gap-1.5">
                  <span className="text-sm font-medium">Gudang</span>
                  <Combobox
                    options={(warehouses.data ?? []).map((w) => ({
                      value: w.id,
                      label: stallLabel(w),
                      description: w.code,
                    }))}
                    value={warehouseId}
                    onChange={(value) => {
                      setWarehouseId(value);
                      setBatchId(NO_BATCH);
                    }}
                    placeholder="Pilih gudang"
                  />
                </label>
                <label className="flex min-w-0 flex-col gap-1.5">
                  <span className="text-sm font-medium">Bahan baku</span>
                  <Combobox
                    options={(materials.data ?? []).map((m) => ({
                      value: m.id,
                      label: m.nama,
                      description: m.kode,
                    }))}
                    value={materialId}
                    onChange={(value) => {
                      setMaterialId(value);
                      setBatchId(NO_BATCH);
                    }}
                    placeholder="Cari bahan baku"
                  />
                </label>
                <label className="flex min-w-0 flex-col gap-1.5 sm:col-span-2">
                  <span className="text-sm font-medium">Batch</span>
                  <Select
                    value={batchId}
                    onValueChange={(value) =>
                      setBatchId(String(value ?? NO_BATCH))
                    }
                    disabled={!materialId || !warehouseId}
                  >
                    <SelectTrigger aria-label="Batch">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value={NO_BATCH}>
                        Otomatis (FEFO, kedaluwarsa terdekat dulu)
                      </SelectItem>
                      {(batches.data ?? []).map((b) => (
                        <SelectItem key={b.id} value={b.id}>
                          {b.batch_number ?? "Tanpa nomor"} ·{" "}
                          {b.expiry_date
                            ? formatDate(b.expiry_date)
                            : "tanpa tanggal"}{" "}
                          · sisa {formatNumber(b.qty_remaining, 3)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </label>
              </>
            )}
            <label className="flex min-w-0 flex-col gap-1.5">
              <span className="text-sm font-medium">Qty (satuan dasar)</span>
              <Input
                type="number"
                min={0}
                step="any"
                max={maxQty}
                value={qty}
                onChange={(e) => setQty(e.target.value)}
                required
              />
              {maxQty !== undefined && (
                <span className="text-xs text-muted-foreground">
                  Maksimal {formatNumber(maxQty, 3)} dari batch ini.
                </span>
              )}
            </label>
            <label className="flex min-w-0 flex-col gap-1.5">
              <span className="text-sm font-medium">Alasan</span>
              <Select
                value={reason}
                onValueChange={(value) => setReason(value as ScrapReason)}
              >
                <SelectTrigger aria-label="Alasan scrap">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {SCRAP_REASON_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </label>
            <label className="flex min-w-0 flex-col gap-1.5 sm:col-span-2">
              <span className="text-sm font-medium">Catatan</span>
              <Textarea
                value={notes}
                onChange={(e) => setNotes(e.target.value)}
                placeholder="Contoh: kemasan bocor saat bongkar muat"
                maxLength={500}
              />
            </label>
          </DialogPanelBody>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={onClose}
              disabled={save.isPending}
            >
              Batal
            </Button>
            <Button
              type="submit"
              variant="destructive"
              disabled={!valid || save.isPending}
            >
              {preset ? "Write-off" : "Catat scrap"}
            </Button>
          </DialogFooter>
        </DialogPanelForm>
      </DialogPanel>
    </Dialog>
  );
}

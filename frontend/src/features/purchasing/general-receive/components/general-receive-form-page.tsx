"use client";

import { useState } from "react";
import { stallLabel } from "@/lib/auth/stall-label";
import { useRouter } from "next/navigation";
import { ArrowLeft, Boxes, PackageCheck, Receipt } from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Combobox } from "@/components/ui/combobox";
import { Input } from "@/components/ui/input";
import { PurchasingListSection } from "@/features/purchasing/components/shared/purchasing-list-section";
import { PurchasingPageHeader } from "@/features/purchasing/components/shared/purchasing-page-header";
import { GENERAL_ROUTES } from "@/lib/purchasing/item-routes";
import {
  buildGeneralGrnItems,
  buildGeneralReceiveLines,
  totalGeneralReceived,
} from "@/lib/purchasing/receiving-ui-general";
import { useGeneralPurchaseOrder } from "../../general-po/queries";
import type { GeneralPODetail } from "../../general-po/types";
import type { Warehouse } from "../../grn/api";
import { useWarehouses } from "../../grn/queries";
import { useCreateGeneralGrn } from "../queries";

export function GeneralReceiveFormPage({ poId }: { poId: string }) {
  const poQuery = useGeneralPurchaseOrder(poId);
  const warehousesQuery = useWarehouses(null);

  if (poQuery.isLoading || warehousesQuery.isLoading) {
    return <div className="py-16 text-center text-sm text-gray-500">Memuat data penerimaan...</div>;
  }
  if (!poQuery.data) {
    return <div className="py-16 text-center text-sm text-gray-500">Purchase order tidak ditemukan.</div>;
  }
  // key: baris form diisi ulang dari PO yang dibuka.
  return <GeneralReceiveForm key={poQuery.data.id} po={poQuery.data} warehouses={warehousesQuery.data ?? []} />;
}

function GeneralReceiveForm({ po, warehouses }: { po: GeneralPODetail; warehouses: Warehouse[] }) {
  const router = useRouter();
  const createMutation = useCreateGeneralGrn();
  const [lines, setLines] = useState(() => buildGeneralReceiveLines(po.items));
  // null = belum dipilih: gudang tunggal dipilih otomatis.
  const [warehouseChoice, setWarehouseChoice] = useState<string | null>(null);
  const [catatan, setCatatan] = useState("");

  const warehouseId = warehouseChoice ?? (warehouses.length === 1 ? warehouses[0].id : "");
  const totalDiterima = totalGeneralReceived(lines);
  const listRoute = GENERAL_ROUTES.purchasingReceive;

  function updateQty(poItemId: string, value: string) {
    setLines((prev) => prev.map((line) => (line.poItemId === poItemId ? { ...line, qtyDiterima: value } : line)));
  }

  async function handleSubmit() {
    if (!warehouseId) {
      toast.error("Pilih gudang penerimaan terlebih dahulu");
      return;
    }
    const built = buildGeneralGrnItems(lines);
    if ("error" in built) {
      toast.error(built.error);
      return;
    }
    try {
      const result = await createMutation.mutateAsync({
        po_id: po.id,
        warehouse_id: warehouseId,
        catatan: catatan || undefined,
        items: built.items,
      });
      toast.success(`Penerimaan ${result.nomor_grn ?? ""} tercatat`);
      router.push(listRoute);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Gagal menyimpan penerimaan");
    }
  }

  return (
    <div className="space-y-6">
      <PurchasingPageHeader
        title={`Terima Barang — ${po.nomor_po}`}
        description={`Vendor ${po.vendor_name ?? "-"}. Barang operasional dicatat diterima tanpa langkah pengiriman terpisah.`}
        actions={
          <Button variant="outline" onClick={() => router.push(listRoute)}>
            <ArrowLeft className="mr-2 h-4 w-4" />
            Kembali
          </Button>
        }
      />

      <PurchasingListSection
        icon={PackageCheck}
        title="Item Diterima"
        description="Isi jumlah barang yang benar-benar diterima. Item stok bertambah di fase lanjut; item expense cukup ditandai diterima."
      >
        <div className="space-y-4 px-5 py-4">
          <div className="max-w-md">
            <label className="mb-1 block text-sm font-medium text-gray-700">
              Gudang Penerimaan <span className="text-red-500">*</span>
            </label>
            <Combobox
              options={warehouses.map((w) => ({ value: w.id, label: w.code ? `${stallLabel(w)} (${w.code})` : stallLabel(w) }))}
              value={warehouseId}
              onChange={setWarehouseChoice}
              placeholder="Pilih gudang..."
              className="h-10 text-sm"
            />
          </div>

          <div className="overflow-x-auto">
            <table className="min-w-full text-sm">
              <thead className="border-b border-gray-100 bg-gray-50 text-xs uppercase tracking-wide text-gray-500">
                <tr>
                  <th className="px-4 py-3 text-left font-semibold">Barang</th>
                  <th className="px-4 py-3 text-center font-semibold">Jenis</th>
                  <th className="px-4 py-3 text-right font-semibold">Dipesan</th>
                  <th className="px-4 py-3 text-right font-semibold">Sudah Diterima</th>
                  <th className="px-4 py-3 text-right font-semibold">Sisa</th>
                  <th className="px-4 py-3 text-right font-semibold">Qty Diterima</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {lines.map((line) => (
                  <tr key={line.poItemId} className="hover:bg-gray-50">
                    <td className="px-4 py-3">
                      <div className="font-medium text-gray-900">{line.nama}</div>
                      <div className="text-xs text-gray-500">{line.kode}</div>
                    </td>
                    <td className="px-4 py-3 text-center">
                      {line.stockable ? (
                        <Badge className="bg-indigo-100 text-indigo-800">
                          <Boxes className="mr-1 h-3 w-3" /> Stok
                        </Badge>
                      ) : (
                        <Badge className="bg-gray-100 text-gray-700">
                          <Receipt className="mr-1 h-3 w-3" /> Expense
                        </Badge>
                      )}
                    </td>
                    <td className="px-4 py-3 text-right text-gray-600">{line.ordered}</td>
                    <td className="px-4 py-3 text-right text-gray-600">{line.received}</td>
                    <td className="px-4 py-3 text-right font-medium text-gray-900">{line.remaining}</td>
                    <td className="px-4 py-3 text-right">
                      <Input
                        type="number"
                        min={0}
                        max={line.remaining}
                        step="any"
                        value={line.qtyDiterima}
                        onChange={(e) => updateQty(line.poItemId, e.target.value)}
                        disabled={line.remaining <= 0}
                        className="h-9 w-28 text-right text-sm"
                      />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <div className="max-w-md">
            <label className="mb-1 block text-sm font-medium text-gray-700">Catatan</label>
            <Input
              placeholder="Catatan penerimaan (opsional)"
              value={catatan}
              onChange={(e) => setCatatan(e.target.value)}
              className="h-10 text-sm"
            />
          </div>

          <div className="flex items-center justify-between border-t border-gray-100 pt-4">
            <span className="text-sm text-gray-600">
              Total qty diterima: <span className="font-semibold text-gray-900">{totalDiterima}</span>
            </span>
            <Button
              className="purchasing-main-button"
              onClick={handleSubmit}
              disabled={createMutation.isPending || totalDiterima <= 0}
            >
              <PackageCheck className="mr-2 h-4 w-4" />
              {createMutation.isPending ? "Menyimpan..." : "Simpan Penerimaan"}
            </Button>
          </div>
        </div>
      </PurchasingListSection>
    </div>
  );
}

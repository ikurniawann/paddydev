"use client";

import { useState } from "react";
import { stallLabel } from "@/lib/auth/stall-label";
import { useSearchParams } from "next/navigation";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Combobox } from "@/components/ui/combobox";
import { STALL_LABELS } from "@/lib/configuration/stall-labels";
import { formatNumber, formatRupiah } from "@/lib/format";
import { dateStamp, downloadCsv } from "@/lib/purchasing/report-ui-shared";
import {
  MOVEMENT_TYPE_LABELS,
  STOCK_CARD_CSV_HEADERS,
  stockCardCsvRows,
  stockCardFiltersFromUrl,
  stockCardTotals,
} from "@/lib/purchasing/report-ui-stock-card";
import { useStockCard, useStockWarehouses } from "../queries";
import type { StockCardItemType, StockMovementType } from "../types";
import { useErrorToast } from "../use-error-toast";
import { DateFilterField, ExportButton, ReportFilterField, ReportHeader, ReportStatCard, ReportTableCard } from "./report-ui";
import { StockCardMovementsTable } from "./stock-card-movements-table";

const MOVEMENT_TYPE_OPTIONS = Object.entries(MOVEMENT_TYPE_LABELS).map(([value, label]) => ({ value, label }));

const ITEM_TYPES: Array<{ value: StockCardItemType; label: string }> = [
  { value: "raw_material", label: "Bahan Baku" },
  { value: "product", label: "Produk" },
];

export function StockCardPage() {
  const searchParams = useSearchParams();
  const [initial] = useState(() => stockCardFiltersFromUrl(searchParams));
  const [itemType, setItemType] = useState(initial.itemType);
  const [selectedItem, setSelectedItem] = useState(initial.selectedItem);
  const [warehouseId, setWarehouseId] = useState(initial.warehouseId);
  const [movementType, setMovementType] = useState<StockMovementType>("all");
  const [dateFrom, setDateFrom] = useState("");
  const [dateTo, setDateTo] = useState("");

  const warehousesQuery = useStockWarehouses();
  useErrorToast(warehousesQuery.error, "Gagal memuat daftar outlet");
  const stockCardQuery = useStockCard({
    item_type: itemType,
    item_id: selectedItem !== "all" ? selectedItem : undefined,
    warehouse_id: warehouseId !== "all" ? warehouseId : undefined,
    tipe: movementType !== "all" ? movementType : undefined,
    date_from: dateFrom || undefined,
    date_to: dateTo || undefined,
    limit: 500,
  });
  useErrorToast(stockCardQuery.error, "Gagal memuat stock card");

  const data = stockCardQuery.data;
  const loading = stockCardQuery.isLoading || stockCardQuery.isFetching;
  const selected = data?.selected_item || data?.selected_material || null;
  const movements = data?.movements ?? [];
  const items = data?.items || data?.materials || [];
  const { totalIn, totalOut } = stockCardTotals(data?.summary);
  const itemLabel = itemType === "product" ? "Produk" : "Bahan Baku";

  const itemOptions = [
    { value: "all", label: `Semua ${itemLabel}` },
    ...items.slice(0, 500).map((item) => ({
      value: item.id,
      label: `${item.kode} - ${item.nama}`,
      description: item.kategori || undefined,
    })),
  ];
  const stallOptions = [
    { value: "all", label: STALL_LABELS.allBranchTotal },
    ...(warehousesQuery.data ?? []).map((warehouse) => ({
      value: warehouse.id,
      label: stallLabel(warehouse),
      description: warehouse.code,
    })),
  ];

  const changeItemType = (next: StockCardItemType) => {
    setItemType(next);
    setSelectedItem("all");
  };

  const exportCsv = () => {
    downloadCsv(
      `stock-card-${itemType}-${selected?.kode || "all"}-${dateStamp()}.csv`,
      STOCK_CARD_CSV_HEADERS,
      stockCardCsvRows(movements)
    );
    toast.success("Stock card berhasil diexport");
  };

  return (
    <div className="space-y-6 p-6">
      <ReportHeader
        title="Stock Card"
        description="Kartu stok bahan baku & produk — saldo awal, mutasi, dan saldo akhir."
        loading={loading}
        onRefresh={() => void stockCardQuery.refetch()}
      >
        <ExportButton label="Export CSV" disabled={movements.length === 0} onClick={exportCsv} />
      </ReportHeader>

      <Card className="border-border shadow-xs">
        <CardContent className="space-y-4 pt-4">
          <div className="inline-flex rounded-lg border border-gray-200/70 bg-muted/40 p-1">
            {ITEM_TYPES.map((option) => (
              <Button
                key={option.value}
                type="button"
                size="sm"
                variant={itemType === option.value ? "default" : "ghost"}
                className="h-8"
                onClick={() => changeItemType(option.value)}
              >
                {option.label}
              </Button>
            ))}
          </div>

          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-5">
            <ReportFilterField label={STALL_LABELS.singular}>
              <Combobox
                options={stallOptions}
                value={warehouseId}
                onChange={setWarehouseId}
                placeholder={warehousesQuery.isLoading ? STALL_LABELS.loading : STALL_LABELS.allBranchTotal}
                searchPlaceholder={STALL_LABELS.search}
                emptyMessage={STALL_LABELS.empty}
                className="h-10"
              />
            </ReportFilterField>
            <ReportFilterField label={itemLabel}>
              <Combobox
                options={itemOptions}
                value={selectedItem}
                onChange={setSelectedItem}
                placeholder={`Pilih ${itemLabel.toLowerCase()}...`}
                searchPlaceholder={`Cari ${itemLabel.toLowerCase()}...`}
                emptyMessage={`${itemLabel} tidak ditemukan`}
                className="h-10"
              />
            </ReportFilterField>
            <ReportFilterField label="Tipe Mutasi">
              <Combobox
                options={MOVEMENT_TYPE_OPTIONS}
                value={movementType}
                onChange={(value) => setMovementType(value as StockMovementType)}
                placeholder="Semua Tipe"
                searchPlaceholder="Cari tipe..."
                emptyMessage="Tipe tidak ditemukan"
                className="h-10"
              />
            </ReportFilterField>
            <DateFilterField label="Dari Tanggal" value={dateFrom} onChange={setDateFrom} />
            <DateFilterField label="Sampai Tanggal" value={dateTo} onChange={setDateTo} />
          </div>
        </CardContent>
      </Card>

      {selected ? (
        <Card className="border-border shadow-xs">
          <CardContent className="grid gap-4 p-4 md:grid-cols-4">
            <div className="md:col-span-1">
              <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{itemLabel} Terpilih</p>
              <h2 className="mt-1 text-lg font-semibold text-foreground">{selected.nama}</h2>
              <p className="text-sm text-muted-foreground">
                {selected.kode} · {selected.kategori}
              </p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">Stok Saat Ini</p>
              <p className="mt-1 text-xl font-bold text-foreground">
                {formatNumber(selected.qty_onhand, 3)}
                {selected.satuan ? (
                  <span className="ml-1 text-sm font-normal text-muted-foreground">{selected.satuan}</span>
                ) : null}
              </p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">Avg Cost</p>
              <p className="mt-1 text-xl font-bold text-foreground">{formatRupiah(selected.avg_cost)}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">Lokasi / Outlet</p>
              <p className="mt-1 text-sm font-semibold text-foreground">
                {selected.warehouse_name || selected.lokasi_rak || "-"}
              </p>
              <Badge className="mt-2 border-border bg-muted/50 text-muted-foreground">{selected.status_stok}</Badge>
            </div>
          </CardContent>
        </Card>
      ) : null}

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-5">
        <ReportStatCard title="Saldo Awal" value={formatNumber(data?.summary?.opening_balance, 3)} />
        <ReportStatCard title="Total Masuk" value={formatNumber(totalIn, 3)} valueClassName="text-emerald-600" />
        <ReportStatCard title="Total Keluar" value={formatNumber(totalOut, 3)} valueClassName="text-red-600" />
        <ReportStatCard
          title="Saldo Akhir"
          value={formatNumber(data?.summary?.closing_balance, 3)}
          valueClassName="text-brand-text"
        />
        <ReportStatCard title="Jumlah Mutasi" value={data?.summary?.movement_count || 0} />
      </div>

      <ReportTableCard
        title="Riwayat Mutasi"
        count={movements.length}
        footer={`Menampilkan ${movements.length} mutasi${selected ? ` untuk ${selected.nama}` : ""}`}
      >
        <StockCardMovementsTable movements={movements} loading={loading} itemType={itemType} />
      </ReportTableCard>
    </div>
  );
}

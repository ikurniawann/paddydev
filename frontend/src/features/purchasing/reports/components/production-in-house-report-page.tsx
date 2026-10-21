"use client";

import { useMemo, useState } from "react";
import { stallLabel } from "@/lib/auth/stall-label";
import Link from "next/link";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Combobox } from "@/components/ui/combobox";
import { useProductList } from "@/features/purchasing/products/queries";
import { STALL_LABELS } from "@/lib/configuration/stall-labels";
import { formatDate, formatNumber, formatRupiah, formatRupiahCompact } from "@/lib/format";
import { PRODUCT_ROUTES } from "@/lib/purchasing/item-routes";
import { PRODUCTION_STATUS_LABELS, productionStatusLabel } from "@/lib/purchasing/production-ui-display";
import { dateStamp, downloadBlob } from "@/lib/purchasing/report-ui-shared";
import { exportProductionInHouseReport } from "../api";
import { useProductionInHouseReport, useStockWarehouses } from "../queries";
import type { ProductionDateField, ProductionInHouseParams, ProductionOutputTypeFilter } from "../types";
import { useErrorToast } from "../use-error-toast";
import {
  DateFilterField,
  ExportButton,
  ReportBreakdownCard,
  ReportFilterField,
  ReportHeader,
  ReportStatCard,
  ReportTableCard,
  ReportTableMessage,
} from "./report-ui";

const DATE_FIELD_OPTIONS = [
  { value: "completed_at", label: "Tanggal Selesai" },
  { value: "created_at", label: "Tanggal Dibuat" },
];

const STATUS_OPTIONS = [
  { value: "all", label: "Semua Status" },
  ...Object.entries(PRODUCTION_STATUS_LABELS).map(([value, label]) => ({ value, label })),
];

const OUTPUT_OPTIONS = [
  { value: "all", label: "Semua Output" },
  { value: "FINISHED_GOOD", label: "Barang Jadi" },
  { value: "WIP", label: "WIP" },
];

const STATUS_STYLES: Record<string, string> = {
  DRAFT: "border-gray-200/80 bg-gray-50 text-gray-700",
  RELEASED: "border-blue-200/80 bg-blue-50 text-blue-700",
  IN_PROGRESS: "border-amber-200/80 bg-amber-50 text-amber-700",
  COMPLETED: "border-emerald-200/80 bg-emerald-50 text-emerald-700",
  CANCELLED: "border-red-200/80 bg-red-50 text-red-700",
};

const EMPTY_SUMMARY = {
  total_orders: 0,
  total_planned_qty: 0,
  total_actual_qty: 0,
  total_hpp_value: 0,
  completed_orders: 0,
};

export function ProductionInHouseReportPage() {
  const [dateFrom, setDateFrom] = useState("");
  const [dateTo, setDateTo] = useState("");
  const [dateField, setDateField] = useState<ProductionDateField>("completed_at");
  const [status, setStatus] = useState("all");
  const [outputType, setOutputType] = useState<ProductionOutputTypeFilter>("all");
  const [productId, setProductId] = useState("all");
  const [warehouseId, setWarehouseId] = useState("all");
  const [exporting, setExporting] = useState(false);

  const productsQuery = useProductList({ is_active: true, limit: 200 });
  const products = productsQuery.data?.data;
  const productOptions = useMemo(
    () => [
      { value: "all", label: "Semua Produk" },
      ...(products ?? []).map((product) => ({ value: product.id, label: `${product.kode || "-"} — ${product.nama}` })),
    ],
    [products]
  );
  const warehousesQuery = useStockWarehouses();
  useErrorToast(warehousesQuery.error, `Gagal memuat daftar ${STALL_LABELS.singular.toLowerCase()}`);
  const warehouseOptions = [
    { value: "all", label: STALL_LABELS.allBranchTotal },
    ...(warehousesQuery.data ?? []).map((warehouse) => ({
      value: warehouse.id,
      label: `${warehouse.code} — ${stallLabel(warehouse)}`,
    })),
  ];

  const params: ProductionInHouseParams = {
    date_from: dateFrom || undefined,
    date_to: dateTo || undefined,
    date_field: dateField,
    status: status === "all" ? undefined : status,
    output_type: outputType,
    product_id: productId === "all" ? undefined : productId,
    warehouse_id: warehouseId === "all" ? undefined : warehouseId,
  };
  const reportQuery = useProductionInHouseReport(params);
  useErrorToast(reportQuery.error, "Gagal memuat laporan Produksi Internal");
  const orders = reportQuery.data?.orders ?? [];
  const byStatus = reportQuery.data?.byStatus ?? [];
  const summary = reportQuery.data?.summary ?? EMPTY_SUMMARY;
  const loading = reportQuery.isLoading || reportQuery.isFetching;
  const dateFieldLabel = dateField === "completed_at" ? "Tanggal Selesai" : "Tanggal Dibuat";

  const exportCsv = async () => {
    setExporting(true);
    try {
      downloadBlob(await exportProductionInHouseReport(params), `production-in-house-${dateStamp()}.csv`);
      toast.success("CSV berhasil diexport");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Gagal mengekspor CSV");
    } finally {
      setExporting(false);
    }
  };

  return (
    <div className="space-y-6 p-6">
      <ReportHeader
        title="Produksi Internal"
        description="Rekap order produksi produk per periode, status, tipe output, dan nilai HPP."
        loading={loading}
        onRefresh={() => void reportQuery.refetch()}
        refreshLabel="Muat Ulang"
      >
        <ExportButton
          label="Ekspor CSV"
          busyLabel="Mengekspor..."
          busy={exporting}
          disabled={exporting || orders.length === 0}
          onClick={() => void exportCsv()}
        />
      </ReportHeader>

      <Card className="border-border shadow-xs">
        <CardContent className="pt-4">
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            <DateFilterField label="Dari Tanggal" value={dateFrom} onChange={setDateFrom} />
            <DateFilterField label="Sampai Tanggal" value={dateTo} onChange={setDateTo} />
            <ReportFilterField label="Mode Tanggal">
              <Combobox
                className="h-10"
                options={DATE_FIELD_OPTIONS}
                value={dateField}
                onChange={(value) => setDateField(value as ProductionDateField)}
                placeholder="Tanggal Selesai"
                searchPlaceholder="Cari mode..."
                emptyMessage="Tidak ditemukan"
              />
            </ReportFilterField>
            <ReportFilterField label="Status">
              <Combobox
                className="h-10"
                options={STATUS_OPTIONS}
                value={status}
                onChange={setStatus}
                placeholder="Semua Status"
                searchPlaceholder="Cari status..."
                emptyMessage="Status tidak ditemukan"
              />
            </ReportFilterField>
            <ReportFilterField label="Tipe Output">
              <Combobox
                className="h-10"
                options={OUTPUT_OPTIONS}
                value={outputType}
                onChange={(value) => setOutputType(value as ProductionOutputTypeFilter)}
                placeholder="Semua Output"
                searchPlaceholder="Cari output..."
                emptyMessage="Tidak ditemukan"
              />
            </ReportFilterField>
            <ReportFilterField label="Produk">
              <Combobox
                className="h-10"
                options={productOptions}
                value={productId}
                onChange={setProductId}
                placeholder="Semua Produk"
                searchPlaceholder="Cari produk..."
                emptyMessage="Produk tidak ditemukan"
              />
            </ReportFilterField>
            <ReportFilterField label={STALL_LABELS.singular} className="md:col-span-2 xl:col-span-3">
              <Combobox
                className="h-10"
                options={warehouseOptions}
                value={warehouseId}
                onChange={setWarehouseId}
                placeholder={warehousesQuery.isLoading ? STALL_LABELS.loading : STALL_LABELS.allBranchTotal}
                searchPlaceholder={STALL_LABELS.search}
                emptyMessage={STALL_LABELS.empty}
              />
            </ReportFilterField>
          </div>
        </CardContent>
      </Card>

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <ReportStatCard
          title="Total Order"
          value={summary.total_orders}
          hint={`${summary.completed_orders} completed`}
        />
        <ReportStatCard
          title="Qty Planned"
          value={formatNumber(summary.total_planned_qty, 3)}
          hint="Rencana produksi"
        />
        <ReportStatCard
          title="Qty Actual"
          value={formatNumber(summary.total_actual_qty, 3)}
          hint="Hasil aktual"
          valueClassName="text-emerald-600"
        />
        <ReportStatCard
          title="Total HPP"
          value={formatRupiahCompact(summary.total_hpp_value)}
          hint="Nilai produksi"
          valueClassName="text-brand-text"
        />
      </div>

      <div className="grid gap-6 xl:grid-cols-[320px_minmax(0,1fr)]">
        <ReportBreakdownCard
          title="Breakdown Status"
          loading={loading}
          rows={byStatus.map((row) => ({
            key: row.status,
            label: productionStatusLabel(row.status),
            caption: `${row.count} order · ${formatNumber(row.actual_qty, 3)} qty`,
            value: row.hpp_value,
          }))}
          total={summary.total_hpp_value}
          formatValue={formatRupiah}
        />

        <ReportTableCard
          title="Daftar Order Produksi"
          count={orders.length}
          footer={`Total HPP: ${formatRupiah(summary.total_hpp_value)} · Filter tanggal: ${dateFieldLabel}`}
        >
          <table className="w-full min-w-280 text-sm">
            <thead className="bg-muted/50 text-left text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              <tr>
                <th className="px-3 py-3">No Produksi</th>
                <th className="px-3 py-3">Produk</th>
                <th className="px-3 py-3">Output</th>
                <th className="px-3 py-3">Status</th>
                <th className="px-3 py-3 text-right">Planned</th>
                <th className="px-3 py-3 text-right">Actual</th>
                <th className="px-3 py-3 text-right">HPP/Unit</th>
                <th className="px-3 py-3 text-right">Total HPP</th>
                <th className="px-3 py-3">{STALL_LABELS.singular}</th>
                <th className="px-3 py-3">Tanggal</th>
                <th className="px-3 py-3 text-right">Aksi</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <ReportTableMessage colSpan={11}>Memuat data...</ReportTableMessage>
              ) : orders.length === 0 ? (
                <ReportTableMessage colSpan={11}>Tidak ada data produksi untuk filter ini</ReportTableMessage>
              ) : (
                orders.map((order) => {
                  const statusKey = order.status.toUpperCase();
                  return (
                    <tr key={order.id} className="border-t border-gray-200/70 hover:bg-muted/30">
                      <td className="px-3 py-3 font-medium text-brand-text">{order.nomor_produksi}</td>
                      <td className="px-3 py-3">
                        <div className="text-foreground">{order.product_nama}</div>
                        <div className="text-xs text-muted-foreground">{order.product_kode}</div>
                      </td>
                      <td className="px-3 py-3 text-muted-foreground">
                        {order.output_type === "WIP" ? "WIP" : "Barang Jadi"}
                      </td>
                      <td className="px-3 py-3">
                        <Badge
                          variant="outline"
                          className={STATUS_STYLES[statusKey] || "border-border bg-muted/50 text-muted-foreground"}
                        >
                          {productionStatusLabel(statusKey)}
                        </Badge>
                      </td>
                      <td className="px-3 py-3 text-right text-muted-foreground">{formatNumber(order.planned_qty, 3)}</td>
                      <td className="px-3 py-3 text-right font-medium text-foreground">
                        {formatNumber(order.actual_qty, 3)}
                      </td>
                      <td className="px-3 py-3 text-right text-muted-foreground">{formatRupiah(order.hpp_per_unit)}</td>
                      <td className="px-3 py-3 text-right font-medium text-foreground">
                        {formatRupiah(order.total_hpp_value)}
                      </td>
                      <td className="px-3 py-3 text-muted-foreground">
                        {order.warehouse_name || order.warehouse_code || "-"}
                      </td>
                      <td className="px-3 py-3 text-muted-foreground">
                        <div>{formatDate(dateField === "created_at" ? order.created_at : order.completed_at)}</div>
                        {dateField === "completed_at" && order.created_at ? (
                          <div className="text-xs">buat {formatDate(order.created_at)}</div>
                        ) : null}
                      </td>
                      <td className="px-3 py-3 text-right">
                        <Link href={PRODUCT_ROUTES.productionOrder(order.id)}>
                          <Button variant="outline" size="sm" className="h-8">
                            Detail
                          </Button>
                        </Link>
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </ReportTableCard>
      </div>
    </div>
  );
}

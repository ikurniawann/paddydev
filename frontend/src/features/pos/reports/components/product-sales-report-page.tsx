"use client";

import { useMemo, useState } from "react";
import { AlertCircle, Loader2, Package, Store, TrendingUp } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { PageTransition } from "@/components/motion";
import { PurchasingListSection } from "@/features/purchasing/components/shared/purchasing-list-section";
import { PurchasingPageHeader } from "@/features/purchasing/components/shared/purchasing-page-header";
import { downloadProductSalesReportXlsx } from "../api";
import { useProductSalesReport } from "../queries";
import { firstDayOfMonthWib, todayWib } from "@/lib/pos/report-dates";
import { formatReportStallLabel } from "@/lib/pos/reports/transaction-labels";
import { ReportExportActions, ReportPrintStyles } from "./report-export-actions";
import { stallFilterValue } from "../utils/stall-filter";
import { formatNumber, formatRupiah } from "@/lib/format";
import { stallLabel } from "@/lib/auth/stall-label";

const formatQty = (value: number) => formatNumber(value, 2);

export function ProductSalesReportPage() {
  const [dateFrom, setDateFrom] = useState(firstDayOfMonthWib);
  const [dateTo, setDateTo] = useState(todayWib);
  const [pickedWarehouseId, setWarehouseId] = useState("");
  const [applied, setApplied] = useState({
    date_from: firstDayOfMonthWib(),
    date_to: todayWib(),
    warehouse_id: undefined as string | undefined,
  });

  const { data, isLoading, isFetching, error } = useProductSalesReport(applied);
  const [exporting, setExporting] = useState(false);

  const warehouseId = stallFilterValue(pickedWarehouseId, data);

  const stallOptions = data?.stall_options ?? [];
  const stallLocked = Boolean(data?.stall_locked);

  // Top produk (owner 2026-09-01): urutkan berdasarkan Omzet atau
  // Quantity, dengan pilihan Top N. Diurutkan di klien dari data yang sama.
  const [sortBy, setSortBy] = useState<"revenue" | "quantity">("revenue");
  const [topN, setTopN] = useState<number>(0); // 0 = semua
  const rows = useMemo(() => {
    const sorted = [...(data?.rows ?? [])].sort((a, b) =>
      sortBy === "revenue" ? b.revenue - a.revenue : b.quantity - a.quantity
    );
    return topN > 0 ? sorted.slice(0, topN) : sorted;
  }, [data?.rows, sortBy, topN]);

  function applyFilter() {
    setApplied({
      date_from: dateFrom,
      date_to: dateTo,
      warehouse_id: warehouseId || undefined,
    });
  }

  async function exportExcel() {
    setExporting(true);
    try {
      await downloadProductSalesReportXlsx(applied);
      toast.success("Excel penjualan produk diunduh");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Gagal mengunduh Excel");
    } finally {
      setExporting(false);
    }
  }

  const selectedStallLabel = stallLocked
    ? stallOptions[0]
      ? `${stallLabel(stallOptions[0])} (${stallOptions[0].code})`
      : "Outlet"
    : warehouseId
      ? (() => {
          const row = stallOptions.find((o) => o.id === warehouseId);
          return row ? stallLabel(row) : "Outlet";
        })()
      : "Semua outlet";

  return (
    <PageTransition>
      <div className="space-y-5">
        <div className="print:hidden">
          <PurchasingPageHeader
            title="Laporan Penjualan Produk"
            description="Ringkasan qty dan omzet produk POS berdasarkan rentang tanggal dan outlet."
            actions={
              <ReportExportActions
                canExport={Boolean(data) && !error}
                exporting={exporting}
                onPrint={() => window.print()}
                onExportExcel={() => void exportExcel()}
              />
            }
          />
        </div>

        <div className="hidden print:block">
          <h1 className="text-lg font-bold">Laporan Penjualan Produk</h1>
          <p className="text-sm text-muted-foreground">
            Periode {applied.date_from} s/d {applied.date_to} · {selectedStallLabel}
          </p>
        </div>

        <Card className="border-gray-200/70 shadow-xs print:hidden">
          <CardContent className="grid gap-4 p-4 md:grid-cols-4">
            <div className="space-y-1.5">
              <Label htmlFor="ps-date-from">Tanggal dari</Label>
              <Input
                id="ps-date-from"
                type="date"
                value={dateFrom}
                onChange={(e) => setDateFrom(e.target.value)}
                className="border-border"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="ps-date-to">Tanggal sampai</Label>
              <Input
                id="ps-date-to"
                type="date"
                value={dateTo}
                onChange={(e) => setDateTo(e.target.value)}
                className="border-border"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="ps-stall">Outlet</Label>
              <select
                id="ps-stall"
                value={warehouseId}
                disabled={stallLocked && stallOptions.length <= 1}
                onChange={(e) => setWarehouseId(e.target.value)}
                className="flex h-9 w-full rounded-md border border-border bg-background px-3 text-sm outline-none focus-visible:ring-1 focus-visible:ring-primary/30"
              >
                {!stallLocked ? <option value="">Semua outlet</option> : null}
                {stallOptions.map((stall) => (
                  <option key={stall.id} value={stall.id}>
                    {stallLabel(stall)} ({stall.code})
                  </option>
                ))}
              </select>
            </div>
            <div className="flex items-end">
              <Button type="button" onClick={applyFilter} disabled={isFetching} className="w-full gap-2">
                {(isLoading || isFetching) && <Loader2 className="size-4 animate-spin" />}
                Terapkan filter
              </Button>
            </div>
          </CardContent>
        </Card>

        {error ? (
          <div className="flex items-center gap-2 rounded-lg border border-red-200/80 bg-red-50 px-4 py-3 text-sm text-red-700 print:hidden">
            <AlertCircle className="size-4" />
            {error instanceof Error ? error.message : "Gagal memuat laporan"}
          </div>
        ) : null}

        <div className="grid gap-3 sm:grid-cols-3">
          <Metric title="Produk terjual" value={String(data?.summary.products ?? 0)} icon={Package} />
          <Metric title="Total qty" value={formatQty(data?.summary.quantity ?? 0)} icon={Store} />
          <Metric title="Total Omzet" value={formatRupiah(data?.summary.revenue ?? 0)} icon={TrendingUp} />
        </div>

        <PurchasingListSection
          icon={Package}
          title="Detail penjualan produk"
          description={`${rows.length} produk untuk periode ${applied.date_from} s/d ${applied.date_to} · diurutkan berdasarkan ${sortBy === "revenue" ? "Omzet" : "Quantity"} tertinggi`}
        >
          <div className="flex flex-wrap items-center gap-3 px-4 pb-3 print:hidden">
            <div className="flex items-center gap-2">
              <span className="text-xs font-medium text-muted-foreground">Top produk berdasarkan</span>
              <div className="flex overflow-hidden rounded-md border border-border">
                <button
                  type="button"
                  onClick={() => setSortBy("revenue")}
                  className={
                    sortBy === "revenue"
                      ? "bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground"
                      : "bg-background px-3 py-1.5 text-xs font-medium text-muted-foreground hover:bg-muted"
                  }
                >
                  Omzet
                </button>
                <button
                  type="button"
                  onClick={() => setSortBy("quantity")}
                  className={
                    sortBy === "quantity"
                      ? "bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground"
                      : "bg-background px-3 py-1.5 text-xs font-medium text-muted-foreground hover:bg-muted"
                  }
                >
                  Quantity
                </button>
              </div>
            </div>
            <div className="flex items-center gap-2">
              <span className="text-xs font-medium text-muted-foreground">Tampilkan</span>
              <select
                value={topN}
                onChange={(e) => setTopN(Number(e.target.value))}
                className="flex h-8 rounded-md border border-border bg-background px-2 text-xs outline-none focus-visible:ring-1 focus-visible:ring-primary/30"
              >
                <option value={0}>Semua produk</option>
                <option value={10}>Top 10</option>
                <option value={25}>Top 25</option>
                <option value={50}>Top 50</option>
              </select>
            </div>
          </div>
          <div className="overflow-x-auto px-4">
            <table className="min-w-full text-sm">
              <thead className="border-b border-gray-200/70 bg-muted/40 text-xs uppercase tracking-wide text-muted-foreground">
                <tr>
                  <th className="px-3 py-3 text-left font-semibold">Produk</th>
                  <th className="px-3 py-3 text-left font-semibold">SKU</th>
                  <th className="px-3 py-3 text-left font-semibold">Outlet</th>
                  <th className="px-3 py-3 text-right font-semibold">Qty</th>
                  <th className="px-3 py-3 text-right font-semibold">Orders</th>
                  <th className="px-3 py-3 text-right font-semibold">Omzet</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200/70">
                {isLoading ? (
                  <tr>
                    <td colSpan={6} className="px-3 py-8 text-center text-muted-foreground">
                      <Loader2 className="mx-auto size-5 animate-spin" />
                    </td>
                  </tr>
                ) : rows.length === 0 ? (
                  <tr>
                    <td colSpan={6} className="px-3 py-8 text-center text-muted-foreground">
                      Tidak ada penjualan produk pada filter ini
                    </td>
                  </tr>
                ) : (
                  rows.map((row) => (
                    <tr
                      key={`${row.product_id || row.product_name}-${row.warehouse_id || "none"}`}
                      className="hover:bg-muted/30"
                    >
                      <td className="px-3 py-3 font-medium text-foreground">{row.product_name}</td>
                      <td className="px-3 py-3 text-muted-foreground">{row.product_sku || "—"}</td>
                      <td className="px-3 py-3 text-muted-foreground">
                        {formatReportStallLabel(row)}
                      </td>
                      <td className="px-3 py-3 text-right">{formatQty(row.quantity)}</td>
                      <td className="px-3 py-3 text-right text-muted-foreground">
                        {formatQty(row.order_count)}
                      </td>
                      <td className="px-3 py-3 text-right font-medium">
                        {formatRupiah(row.revenue)}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </PurchasingListSection>
        <ReportPrintStyles />
      </div>
    </PageTransition>
  );
}

function Metric({
  title,
  value,
  icon: Icon,
}: {
  title: string;
  value: string;
  icon: typeof Package;
}) {
  return (
    <Card className="border-gray-200/70 shadow-xs">
      <CardContent className="p-4">
        <div className="mb-2 w-fit rounded-lg bg-primary/10 p-2 text-brand-text">
          <Icon className="h-4 w-4" />
        </div>
        <div className="text-xl font-bold text-foreground">{value}</div>
        <div className="mt-1 text-sm text-muted-foreground">{title}</div>
      </CardContent>
    </Card>
  );
}

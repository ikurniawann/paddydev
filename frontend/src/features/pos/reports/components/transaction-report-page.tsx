"use client";

import { useMemo, useState } from "react";
import {
  AlertCircle,
  CalendarDays,
  Eye,
  Loader2,
  ReceiptText,
  Store,
  Wallet,
} from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogFooter,
  DialogPanel,
  DialogPanelBody,
  DialogPanelDescription,
  DialogPanelHeader,
  DialogPanelTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { PageTransition } from "@/components/motion";
import { PurchasingListSection } from "@/features/purchasing/components/shared/purchasing-list-section";
import { PurchasingPageHeader } from "@/features/purchasing/components/shared/purchasing-page-header";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { ApexChart } from "./apex-chart";
import { downloadTransactionReportXlsx } from "../api";
import { useTransactionReport } from "../queries";
import type { TransactionReportRow } from "@/lib/pos/reports/types";
import {
  TransactionDetailBody,
  type TransactionOrderDetail,
} from "./transaction-detail-body";
import { loadOrderTransactionDetail } from "../utils/load-order-detail";
import {
  formatCompTypeLabel,
  formatPaymentMethodLabel,
  formatPaymentStatusLabel,
  formatReportStallLabel,
  isPaidPaymentStatus,
} from "@/lib/pos/reports/transaction-labels";
import {
  sortTopProducts,
  topProductBarValue,
  type TopProductSort,
} from "../utils/top-products";
import { firstDayOfMonthWib, todayWib } from "@/lib/pos/report-dates";
import { ReportExportActions, ReportPrintStyles } from "./report-export-actions";
import { stallFilterValue } from "../utils/stall-filter";
import { formatDateTime, formatNumber, formatRupiah, formatRupiahCompact } from "@/lib/format";
import { stallLabel } from "@/lib/auth/stall-label";

const formatQty = (value: number) => formatNumber(value, 2);

export function TransactionReportPage() {
  const [dateFrom, setDateFrom] = useState(firstDayOfMonthWib);
  const [dateTo, setDateTo] = useState(todayWib);
  const [pickedWarehouseId, setWarehouseId] = useState("");
  const [applied, setApplied] = useState({
    date_from: firstDayOfMonthWib(),
    date_to: todayWib(),
    warehouse_id: undefined as string | undefined,
  });
  const [topProductSort, setTopProductSort] = useState<TopProductSort>("omset");
  const [selectedRow, setSelectedRow] = useState<TransactionReportRow | null>(null);
  const [detailOpen, setDetailOpen] = useState(false);
  const [detailLoading, setDetailLoading] = useState(false);
  const [orderDetail, setOrderDetail] = useState<TransactionOrderDetail | null>(null);

  const { data, isLoading, isFetching, error } = useTransactionReport(applied);
  const [exporting, setExporting] = useState(false);

  const warehouseId = stallFilterValue(pickedWarehouseId, data);

  const stallOptions = data?.stall_options ?? [];
  const stallLocked = Boolean(data?.stall_locked);

  const rows = useMemo(() => data?.rows ?? [], [data?.rows]);
  const topProducts = useMemo(
    () => sortTopProducts(data?.top_products ?? [], topProductSort),
    [data?.top_products, topProductSort]
  );
  const detailItems = orderDetail?.items ?? [];

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
      await downloadTransactionReportXlsx(applied);
      toast.success("Excel transaksi diunduh");
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

  async function openItemDetail(row: TransactionReportRow) {
    setSelectedRow(row);
    setDetailOpen(true);
    setDetailLoading(true);
    setOrderDetail(null);
    try {
      const detail = await loadOrderTransactionDetail(row.id, row.checkout_id);
      detail.checkout_number = detail.checkout_number || row.checkout_number;
      detail.xendit_external_id = detail.xendit_external_id || row.xendit_external_id;
      detail.xendit_qr_id = detail.xendit_qr_id || row.xendit_qr_id;
      setOrderDetail(detail);
    } catch (err) {
      const message = err instanceof Error ? err.message : "Gagal memuat detail item";
      toast.error(message);
      setDetailOpen(false);
      setSelectedRow(null);
    } finally {
      setDetailLoading(false);
    }
  }

  return (
    <PageTransition>
      <div className="space-y-5">
        <div className="print:hidden">
          <PurchasingPageHeader
            title="Laporan Transaksi"
            description="Omzet yang sudah tercatat. Kolom Pembayaran = lunas/belum, bukan antrian dapur."
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
          <h1 className="text-lg font-bold">Laporan Transaksi POS</h1>
          <p className="text-sm text-muted-foreground">
            Periode {applied.date_from} s/d {applied.date_to} · {selectedStallLabel}
          </p>
        </div>

        <Card className="border-gray-200/70 shadow-xs print:hidden">
          <CardContent className="grid gap-4 p-4 md:grid-cols-4">
            <div className="space-y-1.5">
              <Label htmlFor="tx-date-from">Tanggal dari</Label>
              <Input
                id="tx-date-from"
                type="date"
                value={dateFrom}
                onChange={(e) => setDateFrom(e.target.value)}
                className="border-border"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="tx-date-to">Tanggal sampai</Label>
              <Input
                id="tx-date-to"
                type="date"
                value={dateTo}
                onChange={(e) => setDateTo(e.target.value)}
                className="border-border"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="tx-stall">Outlet</Label>
              <select
                id="tx-stall"
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

        {/* Rincian keuangan (permintaan owner): Revenue − Diskon + Pajak +
            Service = Nett — identitasnya bisa diperiksa pembaca sendiri. */}
        <div className="grid gap-3 grid-cols-2 sm:grid-cols-3 xl:grid-cols-6">
          <Metric title="Transaksi" value={String(data?.summary.transactions ?? 0)} icon={ReceiptText} />
          <Metric title="Revenue (kotor)" value={formatRupiah(data?.summary.revenue ?? 0)} icon={Wallet} />
          <Metric title="Diskon" value={`− ${formatRupiah(data?.summary.discount ?? 0)}`} icon={Wallet} />
          <Metric title="Pajak" value={formatRupiah(data?.summary.tax ?? 0)} icon={Wallet} />
          <Metric title="Service" value={formatRupiah(data?.summary.service ?? 0)} icon={Wallet} />
          <Metric title="Nett" value={formatRupiah(data?.summary.nett ?? 0)} icon={Store} />
        </div>

        {/* Tren nett harian + top produk pada filter yang sama */}
        <div className="grid gap-4 lg:grid-cols-2 print:hidden">
          <Card className="border-gray-200/70 shadow-xs">
            <CardContent className="p-4">
              <div className="mb-3 text-sm font-semibold text-foreground">Tren Penjualan Harian (Nett)</div>
              {(data?.daily?.length ?? 0) === 0 ? (
                <div className="py-10 text-center text-sm text-muted-foreground">
                  Belum ada data pada filter ini
                </div>
              ) : (
                <ApexChart
                  type="area"
                  height={240}
                  series={[
                    {
                      name: "Nett",
                      data: (data?.daily ?? []).map((d) => ({ x: d.date, y: d.nett })),
                    },
                  ]}
                  options={{
                    chart: { toolbar: { show: false } },
                    dataLabels: { enabled: false },
                    stroke: { curve: "smooth", width: 2 },
                    xaxis: { type: "category" },
                    yaxis: {
                      labels: {
                        formatter: (v: number) => formatRupiahCompact(v),
                      },
                    },
                    tooltip: { y: { formatter: (v: number) => formatRupiah(v) } },
                  }}
                />
              )}
            </CardContent>
          </Card>

          <Card className="border-gray-200/70 shadow-xs">
            <CardContent className="p-4">
              <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
                <div className="text-sm font-semibold text-foreground">Top Produk</div>
                <div className="flex flex-wrap gap-1.5">
                  {(
                    [
                      ["omset", "Omset"],
                      ["qty", "Qty"],
                    ] as const
                  ).map(([key, label]) => (
                    <Button
                      key={key}
                      type="button"
                      variant="outline"
                      size="sm"
                      className={cn(
                        "border-gray-200/80",
                        topProductSort === key &&
                          "border-primary/40 bg-primary/5 text-brand-text"
                      )}
                      onClick={() => setTopProductSort(key)}
                    >
                      {label}
                    </Button>
                  ))}
                </div>
              </div>
              {topProducts.length === 0 ? (
                <div className="py-10 text-center text-sm text-muted-foreground">
                  Belum ada data pada filter ini
                </div>
              ) : (
                <div className="space-y-2.5">
                  {topProducts.map((prod, index) => {
                    const max = topProductBarValue(topProducts[0], topProductSort) || 1;
                    const bar = topProductBarValue(prod, topProductSort);
                    return (
                      <div key={prod.product_name}>
                        <div className="flex items-center justify-between gap-2 text-sm">
                          <span className="truncate font-medium text-foreground">
                            {index + 1}. {prod.product_name}
                          </span>
                          <span className="shrink-0 text-muted-foreground">
                            {formatQty(prod.quantity)} pcs · {formatRupiah(prod.revenue)}
                          </span>
                        </div>
                        <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-muted">
                          <div
                            className="h-full rounded-full bg-primary"
                            style={{ width: `${Math.max(4, (bar / max) * 100)}%` }}
                          />
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </CardContent>
          </Card>
        </div>

        {/* Rekap per stall — jawaban langsung 'laporan transaksi per stall' */}
        <PurchasingListSection
          icon={Store}
          title="Rekap per Outlet"
          description="Berbasis item: order lintas outlet menyumbang ke tiap outlet sesuai itemnya (nilai setelah diskon item, sebelum diskon/pajak transaksi)"
        >
          <div className="overflow-x-auto px-4">
            <table className="min-w-full text-sm">
              <thead className="border-b border-gray-200/70 bg-muted/40 text-xs uppercase tracking-wide text-muted-foreground">
                <tr>
                  <th className="px-3 py-3 text-left font-semibold">Outlet</th>
                  <th className="px-3 py-3 text-right font-semibold">Transaksi</th>
                  <th className="px-3 py-3 text-right font-semibold">Item Terjual</th>
                  <th className="px-3 py-3 text-right font-semibold">Penjualan</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200/70">
                {(data?.per_stall?.length ?? 0) === 0 ? (
                  <tr>
                    <td colSpan={4} className="px-3 py-8 text-center text-muted-foreground">
                      Belum ada data pada filter ini
                    </td>
                  </tr>
                ) : (
                  (data?.per_stall ?? []).map((stall) => (
                    <tr key={stall.stall_code ?? stall.stall_name} className="hover:bg-muted/30">
                      <td className="px-3 py-3 font-medium text-foreground">
                        {formatReportStallLabel(stall)}
                      </td>
                      <td className="px-3 py-3 text-right">{stall.transactions}</td>
                      <td className="px-3 py-3 text-right">{stall.quantity}</td>
                      <td className="px-3 py-3 text-right font-semibold">{formatRupiah(stall.sales)}</td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </PurchasingListSection>

        <PurchasingListSection
          icon={CalendarDays}
          title="Detail transaksi"
          description={`${rows.length} baris untuk periode ${applied.date_from} s/d ${applied.date_to}`}
        >
          <div className="overflow-x-auto px-4">
            <table className="min-w-full text-sm">
              <thead className="border-b border-gray-200/70 bg-muted/40 text-xs uppercase tracking-wide text-muted-foreground">
                <tr>
                  <th className="px-3 py-3 text-left font-semibold">Order</th>
                  <th className="px-3 py-3 text-left font-semibold">Waktu</th>
                  <th className="px-3 py-3 text-left font-semibold">Outlet</th>
                  <th className="px-3 py-3 text-left font-semibold">Pembayaran</th>
                  <th className="px-3 py-3 text-left font-semibold">Metode</th>
                  <th className="px-3 py-3 text-right font-semibold">Total</th>
                  <th className="px-3 py-3 text-right font-semibold">ARK</th>
                  <th className="px-3 py-3 text-right font-semibold print:hidden">Aksi</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200/70">
                {isLoading ? (
                  <tr>
                    <td colSpan={8} className="px-3 py-8 text-center text-muted-foreground">
                      <Loader2 className="mx-auto size-5 animate-spin" />
                    </td>
                  </tr>
                ) : rows.length === 0 ? (
                  <tr>
                    <td colSpan={8} className="px-3 py-8 text-center text-muted-foreground">
                      Tidak ada transaksi pada filter ini
                    </td>
                  </tr>
                ) : (
                  rows.map((row) => (
                    <tr key={row.id} className="hover:bg-muted/30">
                      <td className="px-3 py-3 font-medium text-foreground">
                        <div>{row.order_number || row.id.slice(0, 8)}</div>
                        {row.checkout_number ? (
                          <div className="text-xs font-normal text-muted-foreground">
                            {row.checkout_number}
                            {row.sold_from === "central" ? " · Pusat" : ""}
                          </div>
                        ) : null}
                      </td>
                      <td className="px-3 py-3 text-muted-foreground">{formatDateTime(row.ordered_at)}</td>
                      <td className="px-3 py-3 text-muted-foreground">
                        {formatReportStallLabel(row)}
                      </td>
                      <td className="px-3 py-3">
                        <Badge
                          variant="outline"
                          className={
                            isPaidPaymentStatus(row.payment_status, row.status)
                              ? "border-emerald-200/80 bg-emerald-50 text-emerald-800"
                              : "border-amber-200/80 bg-amber-50 text-amber-800"
                          }
                        >
                          {formatPaymentStatusLabel(row.payment_status, row.status)}
                        </Badge>
                      </td>
                      <td className="px-3 py-3 text-muted-foreground">
                        {row.comp_type ? (
                          <Badge
                            variant="outline"
                            className="border-amber-300/80 bg-amber-50 text-amber-800"
                            title={
                              row.comp_approved_name
                                ? `Disetujui: ${row.comp_approved_name}`
                                : undefined
                            }
                          >
                            {formatCompTypeLabel(row.comp_type) || "Comp"}
                          </Badge>
                        ) : (
                          formatPaymentMethodLabel(row.payment_method, {
                            code: row.payment_method_code,
                            name: row.payment_method_name,
                          })
                        )}
                      </td>
                      <td className="px-3 py-3 text-right font-medium">
                        {formatRupiah(row.total_amount)}
                      </td>
                      <td className="px-3 py-3 text-right text-amber-700">
                        {formatRupiah(row.ark_coins_used)}
                      </td>
                      <td className="px-3 py-3 text-right print:hidden">
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          className="gap-1.5 border-border"
                          onClick={() => void openItemDetail(row)}
                          disabled={detailLoading && selectedRow?.id === row.id}
                        >
                          {detailLoading && selectedRow?.id === row.id ? (
                            <Loader2 className="size-3.5 animate-spin" />
                          ) : (
                            <Eye className="size-3.5" />
                          )}
                          Detail
                        </Button>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </PurchasingListSection>

        <Dialog
          open={detailOpen}
          onOpenChange={(open) => {
            setDetailOpen(open);
            if (!open) {
              setSelectedRow(null);
              setOrderDetail(null);
            }
          }}
        >
          <DialogPanel size="xl">
            <DialogPanelHeader>
              <DialogPanelTitle>
                Detail transaksi{" "}
                {selectedRow?.order_number || selectedRow?.id.slice(0, 8) || ""}
              </DialogPanelTitle>
              <DialogPanelDescription>
                {selectedRow
                  ? `${formatDateTime(selectedRow.ordered_at)} · ${formatReportStallLabel(selectedRow)}`
                  : "Ringkasan pembayaran, item, dan status Xendit"}
              </DialogPanelDescription>
            </DialogPanelHeader>
            <DialogPanelBody className="space-y-4">
              {detailLoading ? (
                <div className="flex items-center justify-center py-10 text-muted-foreground">
                  <Loader2 className="size-5 animate-spin" />
                </div>
              ) : (
                <TransactionDetailBody
                  row={selectedRow}
                  detail={orderDetail}
                  items={detailItems}
                />
              )}
            </DialogPanelBody>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setDetailOpen(false)}
                className="border-border"
              >
                Tutup
              </Button>
            </DialogFooter>
          </DialogPanel>
        </Dialog>
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
  icon: typeof ReceiptText;
}) {
  return (
    <Card className="border-gray-200/70 shadow-xs">
      <CardContent className="p-4">
        <div className="mb-2 rounded-lg bg-primary/10 p-2 text-brand-text w-fit">
          <Icon className="h-4 w-4" />
        </div>
        <div className="text-xl font-bold text-foreground">{value}</div>
        <div className="mt-1 text-sm text-muted-foreground">{title}</div>
      </CardContent>
    </Card>
  );
}

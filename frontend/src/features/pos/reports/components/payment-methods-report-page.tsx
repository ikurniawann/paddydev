"use client";

import { useMemo, useState } from "react";
import {
  AlertCircle,
  CalendarDays,
  CreditCard,
  Loader2,
  ReceiptText,
  Wallet,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { PageTransition } from "@/components/motion";
import { PurchasingListSection } from "@/features/purchasing/components/shared/purchasing-list-section";
import { PurchasingPageHeader } from "@/features/purchasing/components/shared/purchasing-page-header";
import { firstDayOfMonthWib, todayWib } from "@/lib/pos/report-dates";
import { ApexChart } from "./apex-chart";
import { usePaymentMethodsReport } from "../queries";
import type { PaymentMethodsReportGranularity } from "@/lib/pos/reports/types";
import { stallFilterValue } from "../utils/stall-filter";
import { formatRupiah, formatRupiahCompact } from "@/lib/format";
import { stallLabel } from "@/lib/auth/stall-label";

const GRANULARITY_OPTIONS: Array<{
  value: PaymentMethodsReportGranularity;
  label: string;
}> = [
  { value: "day", label: "Hari" },
  { value: "month", label: "Bulan" },
  { value: "year", label: "Tahun" },
];

const CHART_COLORS = [
  "#b8045a",
  "#0ea5e9",
  "#10b981",
  "#f59e0b",
  "#8b5cf6",
  "#ef4444",
  "#64748b",
  "#14b8a6",
];

export function PaymentMethodsReportPage() {
  const [dateFrom, setDateFrom] = useState(firstDayOfMonthWib);
  const [dateTo, setDateTo] = useState(todayWib);
  const [pickedWarehouseId, setWarehouseId] = useState("");
  const [granularity, setGranularity] =
    useState<PaymentMethodsReportGranularity>("day");
  const [applied, setApplied] = useState({
    date_from: firstDayOfMonthWib(),
    date_to: todayWib(),
    warehouse_id: undefined as string | undefined,
    granularity: "day" as PaymentMethodsReportGranularity,
  });

  const { data, isLoading, isFetching, error } = usePaymentMethodsReport(applied);

  const warehouseId = stallFilterValue(pickedWarehouseId, data);

  const stallOptions = data?.stall_options ?? [];
  const stallLocked = Boolean(data?.stall_locked);
  const byMethod = useMemo(() => data?.by_method ?? [], [data?.by_method]);
  const series = useMemo(() => data?.series ?? [], [data?.series]);
  const methodColumns = useMemo(
    () => data?.method_columns ?? [],
    [data?.method_columns]
  );

  const chartSeries = useMemo(() => {
    const keys =
      methodColumns.length > 0
        ? methodColumns
        : byMethod.map((row) => ({ method_key: row.method_key, label: row.label }));
    return keys.slice(0, 8).map((col) => ({
      name: col.label,
      data: series.map((row) => {
        const cell = row.by_method.find((m) => m.method_key === col.method_key);
        return { x: row.label, y: cell?.amount ?? 0 };
      }),
    }));
  }, [byMethod, methodColumns, series]);

  function applyFilter() {
    setApplied({
      date_from: dateFrom,
      date_to: dateTo,
      warehouse_id: warehouseId || undefined,
      granularity,
    });
  }

  return (
    <PageTransition>
      <div className="space-y-5">
        <PurchasingPageHeader
          title="Laporan Jenis Pembayaran"
          description="Ringkasan dan rincian pembayaran POS per metode. Split payment dipecah per jenis."
        />

        <Card className="border-gray-200/70 shadow-xs">
          <CardContent className="grid gap-4 p-4 md:grid-cols-5">
            <div className="space-y-1.5">
              <Label htmlFor="pay-date-from">Tanggal dari</Label>
              <Input
                id="pay-date-from"
                type="date"
                value={dateFrom}
                onChange={(e) => setDateFrom(e.target.value)}
                className="border-border"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="pay-date-to">Tanggal sampai</Label>
              <Input
                id="pay-date-to"
                type="date"
                value={dateTo}
                onChange={(e) => setDateTo(e.target.value)}
                className="border-border"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="pay-stall">Outlet</Label>
              <select
                id="pay-stall"
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
            <div className="space-y-1.5">
              <Label htmlFor="pay-granularity">Detail per</Label>
              <select
                id="pay-granularity"
                value={granularity}
                onChange={(e) =>
                  setGranularity(e.target.value as PaymentMethodsReportGranularity)
                }
                className="flex h-9 w-full rounded-md border border-border bg-background px-3 text-sm outline-none focus-visible:ring-1 focus-visible:ring-primary/30"
              >
                {GRANULARITY_OPTIONS.map((opt) => (
                  <option key={opt.value} value={opt.value}>
                    {opt.label}
                  </option>
                ))}
              </select>
            </div>
            <div className="flex items-end">
              <Button
                type="button"
                onClick={applyFilter}
                disabled={isFetching}
                className="w-full gap-2"
              >
                {(isLoading || isFetching) && <Loader2 className="size-4 animate-spin" />}
                Terapkan filter
              </Button>
            </div>
          </CardContent>
        </Card>

        {error ? (
          <div className="flex items-center gap-2 rounded-lg border border-red-200/80 bg-red-50 px-4 py-3 text-sm text-red-700">
            <AlertCircle className="size-4" />
            {error instanceof Error ? error.message : "Gagal memuat laporan"}
          </div>
        ) : null}

        <div className="grid gap-3 grid-cols-2 lg:grid-cols-4">
          <Metric
            title="Total pembayaran"
            value={formatRupiah(data?.summary.total_amount ?? 0)}
            icon={Wallet}
          />
          <Metric
            title="Jumlah pembayaran"
            value={String(data?.summary.payment_count ?? 0)}
            icon={CreditCard}
          />
          <Metric
            title="Order"
            value={String(data?.summary.order_count ?? 0)}
            icon={ReceiptText}
          />
          <Metric
            title="Jenis terpakai"
            value={String(data?.summary.method_count ?? 0)}
            icon={CalendarDays}
          />
        </div>

        <div className="grid gap-4 lg:grid-cols-2">
          <PurchasingListSection
            icon={CreditCard}
            title="Summary per jenis"
            description={`Periode ${applied.date_from} s/d ${applied.date_to}`}
          >
            <div className="overflow-x-auto px-4">
              <table className="min-w-full text-sm">
                <thead className="border-b border-gray-200/70 bg-muted/40 text-xs uppercase tracking-wide text-muted-foreground">
                  <tr>
                    <th className="px-3 py-3 text-left font-semibold">Jenis</th>
                    <th className="px-3 py-3 text-right font-semibold">Pembayaran</th>
                    <th className="px-3 py-3 text-right font-semibold">Order</th>
                    <th className="px-3 py-3 text-right font-semibold">Nominal</th>
                    <th className="px-3 py-3 text-right font-semibold">%</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-200/70">
                  {isLoading ? (
                    <tr>
                      <td colSpan={5} className="px-3 py-8 text-center text-muted-foreground">
                        <Loader2 className="mx-auto size-5 animate-spin" />
                      </td>
                    </tr>
                  ) : byMethod.length === 0 ? (
                    <tr>
                      <td colSpan={5} className="px-3 py-8 text-center text-muted-foreground">
                        Belum ada pembayaran pada filter ini
                      </td>
                    </tr>
                  ) : (
                    byMethod.map((row) => (
                      <tr key={row.method_key} className="hover:bg-muted/30">
                        <td className="px-3 py-3 font-medium text-foreground">{row.label}</td>
                        <td className="px-3 py-3 text-right text-muted-foreground">
                          {row.payment_count}
                        </td>
                        <td className="px-3 py-3 text-right text-muted-foreground">
                          {row.order_count}
                        </td>
                        <td className="px-3 py-3 text-right font-medium">
                          {formatRupiah(row.amount)}
                        </td>
                        <td className="px-3 py-3 text-right text-muted-foreground">
                          {row.pct}%
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </PurchasingListSection>

          <Card className="border-gray-200/70 shadow-xs">
            <CardContent className="p-4">
              <div className="mb-3 text-sm font-semibold text-foreground">
                Tren nominal per{" "}
                {GRANULARITY_OPTIONS.find((o) => o.value === applied.granularity)?.label.toLowerCase()}
              </div>
              {isLoading ? (
                <div className="flex items-center justify-center py-16 text-muted-foreground">
                  <Loader2 className="size-5 animate-spin" />
                </div>
              ) : chartSeries.length === 0 || series.every((row) => row.total_amount === 0) ? (
                <div className="py-10 text-center text-sm text-muted-foreground">
                  Belum ada data pada filter ini
                </div>
              ) : (
                <ApexChart
                  type="area"
                  height={280}
                  series={chartSeries}
                  options={{
                    chart: { stacked: true, toolbar: { show: false } },
                    colors: CHART_COLORS,
                    dataLabels: { enabled: false },
                    stroke: { curve: "smooth", width: 2 },
                    xaxis: { type: "category" },
                    yaxis: {
                      labels: {
                        formatter: (v: number) => formatRupiahCompact(v),
                      },
                    },
                    legend: { position: "bottom" },
                    tooltip: {
                      y: {
                        formatter: (v: number) => formatRupiah(v || 0),
                      },
                    },
                  }}
                />
              )}
            </CardContent>
          </Card>
        </div>

        <PurchasingListSection
          icon={CalendarDays}
          title={`Detail per ${GRANULARITY_OPTIONS.find((o) => o.value === applied.granularity)?.label.toLowerCase()}`}
          description="Tanggal/bulan/tahun tanpa transaksi ditampilkan sebagai 0"
        >
          <div className="overflow-x-auto px-4 pb-4">
            <table className="min-w-full text-sm">
              <thead className="border-b border-gray-200/70 bg-muted/40 text-xs uppercase tracking-wide text-muted-foreground">
                <tr>
                  <th className="sticky left-0 z-10 bg-muted/40 px-3 py-3 text-left font-semibold">
                    Periode
                  </th>
                  {methodColumns.map((col) => (
                    <th key={col.method_key} className="px-3 py-3 text-right font-semibold">
                      {col.label}
                    </th>
                  ))}
                  <th className="px-3 py-3 text-right font-semibold">Total</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200/70">
                {isLoading ? (
                  <tr>
                    <td
                      colSpan={Math.max(methodColumns.length, 0) + 2}
                      className="px-3 py-8 text-center text-muted-foreground"
                    >
                      <Loader2 className="mx-auto size-5 animate-spin" />
                    </td>
                  </tr>
                ) : series.length === 0 ? (
                  <tr>
                    <td
                      colSpan={Math.max(methodColumns.length, 0) + 2}
                      className="px-3 py-8 text-center text-muted-foreground"
                    >
                      Tidak ada periode pada filter ini
                    </td>
                  </tr>
                ) : (
                  series.map((row) => (
                    <tr key={row.period} className="hover:bg-muted/30">
                      <td className="sticky left-0 z-10 bg-card px-3 py-3 font-medium text-foreground">
                        {row.label}
                      </td>
                      {methodColumns.map((col) => {
                        const cell = row.by_method.find(
                          (m) => m.method_key === col.method_key
                        );
                        return (
                          <td
                            key={col.method_key}
                            className="px-3 py-3 text-right text-muted-foreground"
                          >
                            {formatRupiah(cell?.amount ?? 0)}
                          </td>
                        );
                      })}
                      <td className="px-3 py-3 text-right font-medium">
                        {formatRupiah(row.total_amount)}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </PurchasingListSection>
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
  icon: typeof Wallet;
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

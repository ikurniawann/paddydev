"use client";

import { useMemo, useState } from "react";
import type { ApexOptions } from "apexcharts";
import {
  AlertCircle,
  Clock,
  FileSpreadsheet,
  Loader2,
  ReceiptText,
  TrendingUp,
  Wallet,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { PageTransition } from "@/components/motion";
import { PurchasingListSection } from "@/features/purchasing/components/shared/purchasing-list-section";
import { PurchasingPageHeader } from "@/features/purchasing/components/shared/purchasing-page-header";
import { cn } from "@/lib/utils";
import {
  RUSH_HOUR_HEATMAP_HOURS,
  RUSH_HOUR_WEEKDAYS,
  buildHourRangeContribution,
  formatHourLabel,
  rushHourHeatIntensity,
} from "@/lib/pos/rush-hour";
import { ApexChart } from "./apex-chart";
import { useRushHourReport } from "../queries";
import { firstDayOfMonthWib, todayWib } from "@/lib/pos/report-dates";
import { stallFilterValue } from "../utils/stall-filter";
import { formatNumber, formatRupiah } from "@/lib/format";
import { stallLabel } from "@/lib/auth/stall-label";

function heatClass(intensity: number) {
  if (intensity <= 0) return "bg-muted/40 text-muted-foreground";
  if (intensity >= 0.75) return "bg-primary text-primary-foreground";
  if (intensity >= 0.5) return "bg-primary/70 text-primary-foreground";
  if (intensity >= 0.25) return "bg-primary/40 text-foreground";
  return "bg-primary/15 text-foreground";
}

export function RushHourReportPage() {
  const [dateFrom, setDateFrom] = useState(firstDayOfMonthWib);
  const [dateTo, setDateTo] = useState(todayWib);
  const [pickedWarehouseId, setWarehouseId] = useState("");
  const [applied, setApplied] = useState({
    date_from: firstDayOfMonthWib(),
    date_to: todayWib(),
    warehouse_id: undefined as string | undefined,
  });

  const { data, isLoading, isFetching, error } = useRushHourReport(applied);

  // Kontribusi sales per rentang jam (owner 2026-09-01): pilih jam mulai/
  // selesai, lihat sumbangannya ke total — mode Amount (omzet) / Quantity
  // (jumlah item terjual). Dihitung di klien dari data hourly yang sama.
  const [rangeFrom, setRangeFrom] = useState(7);
  const [rangeTo, setRangeTo] = useState(12);
  const [rangeMode, setRangeMode] = useState<"amount" | "quantity">("amount");
  const rangeContribution = useMemo(() => {
    if (!data) return null;
    const summary = {
      transactions: data.summary.transactions,
      revenue: data.summary.revenue,
      quantity: data.summary.quantity ?? 0,
    };
    const hourly = data.hourly.map((row) => ({ ...row, quantity: row.quantity ?? 0 }));
    return buildHourRangeContribution(hourly, summary, rangeFrom, rangeTo);
  }, [data, rangeFrom, rangeTo]);

  const warehouseId = stallFilterValue(pickedWarehouseId, data);

  const stallOptions = data?.stall_options ?? [];
  const stallLocked = Boolean(data?.stall_locked);
  const maxHeat = useMemo(
    () => Math.max(0, ...(data?.heatmap ?? []).map((cell) => cell.transactions)),
    [data?.heatmap]
  );

  const chartOptions = useMemo(
    (): ApexOptions => ({
      chart: { toolbar: { show: false }, fontFamily: "inherit" },
      stroke: { width: [0, 3], curve: "smooth" as const },
      dataLabels: { enabled: false },
      colors: ["#6b7280", "#9ca3af"],
      xaxis: {
        categories: (data?.hourly ?? []).map((row) => row.label),
        labels: { rotate: -45, style: { fontSize: "11px" } },
      },
      yaxis: [
        { title: { text: "Transaksi" }, labels: { formatter: (v: number) => String(Math.round(v)) } },
        {
          opposite: true,
          title: { text: "Omzet" },
          labels: { formatter: (v: number) => formatRupiah(v) },
        },
      ],
      legend: { position: "top" as const },
      grid: { borderColor: "rgba(0,0,0,0.06)" },
      tooltip: {
        shared: true,
        y: {
          formatter: (value: number, opts?: { seriesIndex: number }) =>
            opts?.seriesIndex === 1 ? formatRupiah(value) : String(value),
        },
      },
    }),
    [data?.hourly]
  );

  function applyFilter() {
    setApplied({
      date_from: dateFrom,
      date_to: dateTo,
      warehouse_id: warehouseId || undefined,
    });
  }

  return (
    <PageTransition>
      <div className="space-y-5">
        <PurchasingPageHeader
          title="Rush Hour"
          description="Jam dan hari tersibuk dari transaksi lunas. Void tidak dihitung. Jam memakai WIB."
        />

        <Card className="border-gray-200/70 shadow-xs">
          <CardContent className="grid gap-4 p-4 md:grid-cols-4">
            <div className="space-y-1.5">
              <Label htmlFor="rh-date-from">Tanggal dari</Label>
              <Input
                id="rh-date-from"
                type="date"
                value={dateFrom}
                onChange={(e) => setDateFrom(e.target.value)}
                className="border-border"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="rh-date-to">Tanggal sampai</Label>
              <Input
                id="rh-date-to"
                type="date"
                value={dateTo}
                onChange={(e) => setDateTo(e.target.value)}
                className="border-border"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="rh-stall">Outlet</Label>
              <select
                id="rh-stall"
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
            <div className="flex items-end gap-2">
              <Button type="button" onClick={applyFilter} disabled={isFetching} className="flex-1 gap-2">
                {(isLoading || isFetching) && <Loader2 className="size-4 animate-spin" />}
                Terapkan filter
              </Button>
              {/* Export memakai filter TERPASANG + rentang jam terpilih,
                  supaya isi file = apa yang sedang dilihat. */}
              <Button
                type="button"
                variant="outline"
                className="gap-2"
                title="Unduh Excel (Ringkasan, Per Jam, Per Hari, Heatmap)"
                onClick={() => {
                  const params = new URLSearchParams({
                    date_from: applied.date_from,
                    date_to: applied.date_to,
                    range_from: String(rangeFrom),
                    range_to: String(rangeTo),
                  });
                  if (applied.warehouse_id) params.set("warehouse_id", applied.warehouse_id);
                  const a = document.createElement("a");
                  a.href = `/api/pos/reports/rush-hour/export?${params.toString()}`;
                  a.click();
                }}
              >
                <FileSpreadsheet className="size-4" />
                Excel
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

        <div className="grid gap-3 grid-cols-2 xl:grid-cols-4">
          <Metric
            title="Jam tersibuk"
            value={data?.peak_hour.hour_label ?? "—"}
            helper={
              data?.peak_hour.hour_label
                ? `${data.peak_hour.transactions} transaksi`
                : "Belum ada data"
            }
            icon={Clock}
          />
          <Metric
            title="Omzet jam tertinggi"
            value={data?.peak_revenue_hour.hour_label ?? "—"}
            helper={formatRupiah(data?.peak_revenue_hour.revenue ?? 0)}
            icon={TrendingUp}
          />
          <Metric
            title="Hari tersibuk"
            value={data?.peak_day.dow_label ?? "—"}
            helper={
              data?.peak_day.dow_label
                ? `${data.peak_day.transactions} transaksi`
                : "Belum ada data"
            }
            icon={ReceiptText}
          />
          <Metric
            title="Rata-rata bill"
            value={formatRupiah(data?.summary.average_ticket ?? 0)}
            helper={`${data?.summary.transactions ?? 0} transaksi`}
            icon={Wallet}
          />
        </div>

        <PurchasingListSection
          icon={Clock}
          title="Kontribusi Sales — Rentang Jam"
          description="Pilih rentang jam untuk melihat sumbangannya ke total penjualan, berdasarkan Amount (omzet) atau Quantity (jumlah item terjual)."
        >
          <div className="space-y-4 px-4 pb-4">
            <div className="flex flex-wrap items-end gap-3">
              <div className="space-y-1.5">
                <Label htmlFor="rh-range-from">Dari jam</Label>
                <select
                  id="rh-range-from"
                  value={rangeFrom}
                  onChange={(e) => {
                    const v = Number(e.target.value);
                    setRangeFrom(v);
                    if (v > rangeTo) setRangeTo(v);
                  }}
                  className="flex h-9 w-28 rounded-md border border-border bg-background px-3 text-sm outline-none focus-visible:ring-1 focus-visible:ring-primary/30"
                >
                  {Array.from({ length: 24 }, (_, h) => (
                    <option key={h} value={h}>{formatHourLabel(h)}</option>
                  ))}
                </select>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="rh-range-to">Sampai jam</Label>
                <select
                  id="rh-range-to"
                  value={rangeTo}
                  onChange={(e) => {
                    const v = Number(e.target.value);
                    setRangeTo(v);
                    if (v < rangeFrom) setRangeFrom(v);
                  }}
                  className="flex h-9 w-28 rounded-md border border-border bg-background px-3 text-sm outline-none focus-visible:ring-1 focus-visible:ring-primary/30"
                >
                  {Array.from({ length: 24 }, (_, h) => (
                    <option key={h} value={h}>{formatHourLabel(h)}</option>
                  ))}
                </select>
              </div>
              <div className="space-y-1.5">
                <Label>Berdasarkan</Label>
                <div className="flex overflow-hidden rounded-md border border-border">
                  <button
                    type="button"
                    onClick={() => setRangeMode("amount")}
                    className={cn(
                      "px-4 py-2 text-sm font-medium transition-colors",
                      rangeMode === "amount"
                        ? "bg-primary text-primary-foreground"
                        : "bg-background text-muted-foreground hover:bg-muted"
                    )}
                  >
                    Amount
                  </button>
                  <button
                    type="button"
                    onClick={() => setRangeMode("quantity")}
                    className={cn(
                      "px-4 py-2 text-sm font-medium transition-colors",
                      rangeMode === "quantity"
                        ? "bg-primary text-primary-foreground"
                        : "bg-background text-muted-foreground hover:bg-muted"
                    )}
                  >
                    Quantity
                  </button>
                </div>
              </div>
            </div>

            {rangeContribution ? (
              <>
                <div className="grid gap-3 sm:grid-cols-3">
                  <Card className="border-primary/30 bg-primary/5 shadow-xs">
                    <CardContent className="p-4">
                      <div className="text-3xl font-bold text-brand-text">
                        {rangeMode === "amount"
                          ? rangeContribution.share_revenue
                          : rangeContribution.share_quantity}
                        %
                      </div>
                      <div className="mt-1 text-sm font-medium text-foreground">
                        Kontribusi {rangeMode === "amount" ? "omzet" : "quantity"} ·{" "}
                        {formatHourLabel(rangeContribution.from_hour)}–
                        {formatHourLabel(rangeContribution.to_hour)}
                      </div>
                      <div className="mt-0.5 text-xs text-muted-foreground">
                        {rangeMode === "amount"
                          ? `${formatRupiah(rangeContribution.revenue)} dari ${formatRupiah(data?.summary.revenue ?? 0)}`
                          : `${formatNumber(rangeContribution.quantity)} item dari ${formatNumber(data?.summary.quantity ?? 0)} item`}
                      </div>
                    </CardContent>
                  </Card>
                  <Card className="border-gray-200/70 shadow-xs">
                    <CardContent className="p-4">
                      <div className="text-3xl font-bold text-foreground">
                        {rangeMode === "amount"
                          ? formatRupiah(rangeContribution.revenue)
                          : formatNumber(rangeContribution.quantity)}
                      </div>
                      <div className="mt-1 text-sm text-muted-foreground">
                        {rangeMode === "amount" ? "Omzet dalam rentang" : "Item terjual dalam rentang"}
                      </div>
                      <div className="mt-0.5 text-xs text-muted-foreground">
                        {rangeContribution.transactions} transaksi ({rangeContribution.share_transactions}% dari total)
                      </div>
                    </CardContent>
                  </Card>
                  <Card className="border-gray-200/70 shadow-xs">
                    <CardContent className="p-4">
                      <div className="h-2 w-full overflow-hidden rounded-full bg-muted">
                        <div
                          className="h-2 rounded-full bg-primary transition-all"
                          style={{
                            width: `${Math.min(100, rangeMode === "amount" ? rangeContribution.share_revenue : rangeContribution.share_quantity)}%`,
                          }}
                        />
                      </div>
                      <div className="mt-3 text-sm text-muted-foreground">
                        Sisanya{" "}
                        {Math.round(
                          (100 -
                            (rangeMode === "amount"
                              ? rangeContribution.share_revenue
                              : rangeContribution.share_quantity)) * 10
                        ) / 10}
                        % terjadi di luar {formatHourLabel(rangeContribution.from_hour)}–
                        {formatHourLabel(rangeContribution.to_hour)}
                      </div>
                    </CardContent>
                  </Card>
                </div>

                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b border-gray-200/70 text-left text-xs font-medium uppercase tracking-wide text-muted-foreground">
                        <th className="px-3 py-2">Jam</th>
                        <th className="px-3 py-2 text-right">
                          {rangeMode === "amount" ? "Omzet" : "Item terjual"}
                        </th>
                        <th className="px-3 py-2 text-right">Transaksi</th>
                        <th className="w-1/3 px-3 py-2">Kontribusi ke total</th>
                      </tr>
                    </thead>
                    <tbody>
                      {rangeContribution.hours.map((row) => {
                        const value = rangeMode === "amount" ? row.revenue : row.quantity;
                        const total =
                          rangeMode === "amount"
                            ? data?.summary.revenue ?? 0
                            : data?.summary.quantity ?? 0;
                        const share = total > 0 ? Math.round((value / total) * 1000) / 10 : 0;
                        return (
                          <tr key={row.hour} className="border-b border-gray-200/70 last:border-0">
                            <td className="px-3 py-2 font-medium">{row.label}</td>
                            <td className="px-3 py-2 text-right tabular-nums">
                              {rangeMode === "amount"
                                ? formatRupiah(row.revenue)
                                : formatNumber(row.quantity)}
                            </td>
                            <td className="px-3 py-2 text-right tabular-nums">{row.transactions}</td>
                            <td className="px-3 py-2">
                              <div className="flex items-center gap-2">
                                <div className="h-2 flex-1 overflow-hidden rounded-full bg-muted">
                                  <div
                                    className="h-2 rounded-full bg-primary/70"
                                    style={{ width: `${Math.min(100, share)}%` }}
                                  />
                                </div>
                                <span className="w-12 text-right text-xs tabular-nums text-muted-foreground">
                                  {share}%
                                </span>
                              </div>
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              </>
            ) : (
              <p className="text-sm text-muted-foreground">Memuat…</p>
            )}
          </div>
        </PurchasingListSection>

        <PurchasingListSection
          icon={Clock}
          title="Heatmap jam × hari"
          description="08:00–22:00. Warna lebih tua = lebih banyak transaksi."
        >
          <div className="overflow-x-auto px-4 pb-4">
            <div
              className="grid min-w-[720px] gap-1"
              style={{ gridTemplateColumns: `3rem repeat(${RUSH_HOUR_HEATMAP_HOURS.length}, minmax(0, 1fr))` }}
            >
              <div />
              {RUSH_HOUR_HEATMAP_HOURS.map((hour) => (
                <div
                  key={hour}
                  className="pb-1 text-center text-[10px] font-medium text-muted-foreground"
                >
                  {formatHourLabel(hour)}
                </div>
              ))}
              {RUSH_HOUR_WEEKDAYS.map((day) => (
                <div key={day.dow} className="contents">
                  <div className="flex items-center text-xs font-medium text-muted-foreground">
                    {day.label}
                  </div>
                  {RUSH_HOUR_HEATMAP_HOURS.map((hour) => {
                    const cell = data?.heatmap.find(
                      (item) => item.dow === day.dow && item.hour === hour
                    );
                    const count = cell?.transactions ?? 0;
                    return (
                      <div
                        key={`${day.dow}-${hour}`}
                        title={`${day.label} ${formatHourLabel(hour)} · ${count} trx · ${formatRupiah(cell?.revenue ?? 0)}`}
                        className={cn(
                          "grid h-9 place-items-center rounded-md text-[11px] font-medium",
                          heatClass(rushHourHeatIntensity(count, maxHeat))
                        )}
                      >
                        {count > 0 ? count : ""}
                      </div>
                    );
                  })}
                </div>
              ))}
            </div>
          </div>
        </PurchasingListSection>

        <PurchasingListSection
          icon={TrendingUp}
          title="Tren per jam"
          description="Jumlah transaksi dan omzet sepanjang hari, digabung untuk rentang tanggal."
        >
          <div className="px-4 pb-4">
            {isLoading && !data ? (
              <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">
                <Loader2 className="mr-2 size-4 animate-spin" />
                Memuat…
              </div>
            ) : (
              <ApexChart
                type="line"
                height={320}
                series={[
                  {
                    name: "Transaksi",
                    type: "column",
                    data: (data?.hourly ?? []).map((row) => row.transactions),
                  },
                  {
                    name: "Omzet",
                    type: "line",
                    data: (data?.hourly ?? []).map((row) => row.revenue),
                  },
                ]}
                options={chartOptions}
              />
            )}
          </div>
        </PurchasingListSection>

        <PurchasingListSection
          icon={ReceiptText}
          title="Rekap per jam"
          description="Rata-rata bill = omzet ÷ jumlah transaksi."
        >
          <div className="overflow-x-auto px-4 pb-4">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-gray-200/70 text-left text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  <th className="px-3 py-2.5">Jam</th>
                  <th className="px-3 py-2.5 text-right">Transaksi</th>
                  <th className="px-3 py-2.5 text-right">Omzet</th>
                  <th className="px-3 py-2.5 text-right">Rata-rata bill</th>
                  <th className="px-3 py-2.5 text-right">% transaksi</th>
                </tr>
              </thead>
              <tbody>
                {(data?.hourly ?? []).map((row) => {
                  const share =
                    (data?.summary.transactions ?? 0) > 0
                      ? Math.round((row.transactions / data!.summary.transactions) * 1000) / 10
                      : 0;
                  const isPeak = data?.peak_hour.hour === row.hour && row.transactions > 0;
                  return (
                    <tr
                      key={row.hour}
                      className={cn(
                        "border-b border-gray-200/70 last:border-0",
                        isPeak ? "bg-primary/5" : "hover:bg-muted/30"
                      )}
                    >
                      <td className="px-3 py-2.5 font-medium">
                        {row.label}
                        {isPeak ? (
                          <span className="ml-2 text-xs font-medium text-brand-text">Puncak</span>
                        ) : null}
                      </td>
                      <td className="px-3 py-2.5 text-right tabular-nums">{row.transactions}</td>
                      <td className="px-3 py-2.5 text-right tabular-nums">
                        {formatRupiah(row.revenue)}
                      </td>
                      <td className="px-3 py-2.5 text-right tabular-nums">
                        {formatRupiah(row.average_ticket)}
                      </td>
                      <td className="px-3 py-2.5 text-right tabular-nums text-muted-foreground">
                        {share}%
                      </td>
                    </tr>
                  );
                })}
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
  helper,
  icon: Icon,
}: {
  title: string;
  value: string;
  helper: string;
  icon: typeof Clock;
}) {
  return (
    <Card className="border-gray-200/70 shadow-xs">
      <CardContent className="p-4">
        <div className="mb-2 w-fit rounded-lg bg-primary/10 p-2 text-brand-text">
          <Icon className="h-4 w-4" />
        </div>
        <div className="text-xl font-bold text-foreground">{value}</div>
        <div className="mt-1 text-sm text-muted-foreground">{title}</div>
        <div className="mt-0.5 text-xs text-muted-foreground">{helper}</div>
      </CardContent>
    </Card>
  );
}

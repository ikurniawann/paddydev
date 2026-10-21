"use client";

import { useMemo, useState } from "react";
import { AlertCircle, Ban, Eye, Loader2, ReceiptText } from "lucide-react";
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
import { firstDayOfMonthWib, todayWib } from "@/lib/pos/report-dates";
import { useVoidReport } from "../queries";
import type { VoidReportRow } from "@/lib/pos/reports/types";
import {
  TransactionDetailBody,
  type TransactionOrderDetail,
} from "./transaction-detail-body";
import { loadOrderTransactionDetail } from "../utils/load-order-detail";
import { stallFilterValue } from "../utils/stall-filter";
import { formatDateTime, formatRupiah } from "@/lib/format";
import { stallLabel } from "@/lib/auth/stall-label";

export function VoidReportPage() {
  const [dateFrom, setDateFrom] = useState(firstDayOfMonthWib);
  const [dateTo, setDateTo] = useState(todayWib);
  const [pickedWarehouseId, setWarehouseId] = useState("");
  const [applied, setApplied] = useState({
    date_from: firstDayOfMonthWib(),
    date_to: todayWib(),
    warehouse_id: undefined as string | undefined,
  });
  const [selectedRow, setSelectedRow] = useState<VoidReportRow | null>(null);
  const [detailOpen, setDetailOpen] = useState(false);
  const [detailLoading, setDetailLoading] = useState(false);
  const [orderDetail, setOrderDetail] = useState<TransactionOrderDetail | null>(null);

  const { data, isLoading, isFetching, error } = useVoidReport(applied);

  const warehouseId = stallFilterValue(pickedWarehouseId, data);

  const stallOptions = data?.stall_options ?? [];
  const stallLocked = Boolean(data?.stall_locked);
  const rows = useMemo(() => data?.rows ?? [], [data?.rows]);

  function applyFilter() {
    setApplied({
      date_from: dateFrom,
      date_to: dateTo,
      warehouse_id: warehouseId || undefined,
    });
  }

  async function openDetail(row: VoidReportRow) {
    setSelectedRow(row);
    setDetailOpen(true);
    setDetailLoading(true);
    setOrderDetail(null);
    try {
      const detail = await loadOrderTransactionDetail(row.id, row.checkout_id);
      detail.void_reason = detail.void_reason || row.void_reason;
      detail.voided_at = detail.voided_at || row.voided_at;
      detail.created_by_name = row.created_by_name;
      detail.voided_by_name = row.voided_by_name;
      detail.checkout_number = detail.checkout_number || row.checkout_number;
      if (!detail.items?.length && row.items.length) {
        detail.items = row.items;
      }
      setOrderDetail(detail);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Gagal memuat detail void");
      setOrderDetail({
        id: row.id,
        order_number: row.order_number,
        ordered_at: row.ordered_at,
        voided_at: row.voided_at,
        void_reason: row.void_reason,
        created_by_name: row.created_by_name,
        voided_by_name: row.voided_by_name,
        checkout_number: row.checkout_number,
        checkout_id: row.checkout_id,
        total_amount: row.total_amount,
        payment_method: row.payment_method,
        payment_method_code: row.payment_method_code,
        payment_method_name: row.payment_method_name,
        sold_from: row.sold_from,
        status: "voided",
        items: row.items,
      });
    } finally {
      setDetailLoading(false);
    }
  }

  return (
    <PageTransition>
      <div className="space-y-5">
        <PurchasingPageHeader
          title="Laporan Void"
          description="Transaksi yang dibatalkan: item, alasan, kasir pembuat, dan supervisor yang void."
        />

        <Card className="border-gray-200/70 shadow-xs">
          <CardContent className="grid gap-4 p-4 md:grid-cols-4">
            <div className="space-y-1.5">
              <Label htmlFor="void-date-from">Tanggal dari</Label>
              <Input
                id="void-date-from"
                type="date"
                value={dateFrom}
                onChange={(e) => setDateFrom(e.target.value)}
                className="border-border"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="void-date-to">Tanggal sampai</Label>
              <Input
                id="void-date-to"
                type="date"
                value={dateTo}
                onChange={(e) => setDateTo(e.target.value)}
                className="border-border"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="void-stall">Outlet</Label>
              <select
                id="void-stall"
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
          <div className="flex items-center gap-2 rounded-lg border border-red-200/80 bg-red-50 px-4 py-3 text-sm text-red-700">
            <AlertCircle className="size-4" />
            {error instanceof Error ? error.message : "Gagal memuat laporan void"}
          </div>
        ) : null}

        <div className="grid gap-3 sm:grid-cols-2">
          <Metric title="Transaksi void" value={String(data?.summary.voids ?? 0)} icon={Ban} />
          <Metric
            title="Nilai void"
            value={formatRupiah(data?.summary.amount ?? 0)}
            icon={ReceiptText}
          />
        </div>

        <PurchasingListSection
          icon={Ban}
          title="Daftar void"
          description={`${rows.length} transaksi void untuk ${applied.date_from} s/d ${applied.date_to}`}
        >
          <div className="overflow-x-auto px-4">
            <table className="min-w-full text-sm">
              <thead className="border-b border-gray-200/70 bg-muted/40 text-xs uppercase tracking-wide text-muted-foreground">
                <tr>
                  <th className="px-3 py-3 text-left font-semibold">Order</th>
                  <th className="px-3 py-3 text-left font-semibold">Waktu void</th>
                  <th className="px-3 py-3 text-left font-semibold">Dibuat oleh</th>
                  <th className="px-3 py-3 text-left font-semibold">Divoid oleh</th>
                  <th className="px-3 py-3 text-left font-semibold">Alasan</th>
                  <th className="px-3 py-3 text-right font-semibold">Nilai</th>
                  <th className="w-px px-3 py-3 text-right font-semibold">Aksi</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200/70">
                {isLoading ? (
                  <tr>
                    <td colSpan={7} className="px-3 py-8 text-center text-muted-foreground">
                      <Loader2 className="mx-auto size-5 animate-spin" />
                    </td>
                  </tr>
                ) : rows.length === 0 ? (
                  <tr>
                    <td colSpan={7} className="px-3 py-8 text-center text-muted-foreground">
                      Tidak ada transaksi void pada filter ini
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
                          </div>
                        ) : null}
                      </td>
                      <td className="px-3 py-3 text-muted-foreground">
                        {formatDateTime(row.voided_at || row.ordered_at)}
                      </td>
                      <td className="px-3 py-3 text-muted-foreground">{row.created_by_name}</td>
                      <td className="px-3 py-3 text-muted-foreground">{row.voided_by_name}</td>
                      <td className="max-w-xs px-3 py-3 text-muted-foreground">
                        <span className="line-clamp-2">{row.void_reason || "—"}</span>
                      </td>
                      <td className="px-3 py-3 text-right font-medium">
                        {formatRupiah(row.total_amount)}
                      </td>
                      <td className="px-3 py-3 text-right">
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          className="gap-1.5 border-border"
                          onClick={() => void openDetail(row)}
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
                Detail void {selectedRow?.order_number || ""}
              </DialogPanelTitle>
              <DialogPanelDescription>
                {selectedRow
                  ? `${formatDateTime(selectedRow.voided_at || selectedRow.ordered_at)} · ${selectedRow.voided_by_name}`
                  : "Item, alasan, dan pelaku void"}
              </DialogPanelDescription>
            </DialogPanelHeader>
            <DialogPanelBody>
              {detailLoading ? (
                <div className="flex items-center justify-center py-10 text-muted-foreground">
                  <Loader2 className="size-5 animate-spin" />
                </div>
              ) : (
                <TransactionDetailBody
                  row={null}
                  detail={orderDetail}
                  items={orderDetail?.items ?? selectedRow?.items ?? []}
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
  icon: typeof Ban;
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

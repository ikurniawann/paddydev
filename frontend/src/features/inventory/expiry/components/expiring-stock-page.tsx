"use client";

import { useState } from "react";
import { stallLabel } from "@/lib/auth/stall-label";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { AlarmClock, Ban, PackageX } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Combobox } from "@/components/ui/combobox";
import { Input } from "@/components/ui/input";
import { PageHeader } from "@/components/ui/page-header";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { StatCard } from "@/components/ui/stat-card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { ExpiringBatchRow } from "@/lib/inventory/stock-queries";
import type { ExpirySummary } from "@/lib/inventory/batches";
import { fetchJson, useWarehouseLookup } from "../../shared/inventory-lookups";
import {
  ScrapDialog,
  type ScrapBatchPreset,
} from "../../scrap/components/scrap-dialog";
import { formatDate, formatNumber, formatRupiah } from "@/lib/format";

const WINDOWS = [7, 14, 30, 60, 90];
type StatusFilter = "all" | "near" | "expired";

function daysLabel(days: number): string {
  if (days < 0) return `lewat ${Math.abs(days)} hari`;
  if (days === 0) return "hari ini";
  return `${days} hari lagi`;
}

/**
 * Inventori → Stok Kedaluwarsa: batch yang habis masa simpannya dalam N hari
 * dan yang sudah lewat, per gudang, dengan nilai berisiko dan write-off per batch.
 */
export function ExpiringStockPage() {
  const warehouses = useWarehouseLookup();
  const [days, setDays] = useState(30);
  const [status, setStatus] = useState<StatusFilter>("all");
  const [warehouseId, setWarehouseId] = useState("");
  const [search, setSearch] = useState("");
  const [writeOff, setWriteOff] = useState<ScrapBatchPreset | null>(null);

  const params = new URLSearchParams({ days: String(days), status });
  if (warehouseId) params.set("warehouse_id", warehouseId);
  if (search.trim()) params.set("search", search.trim());

  const expiry = useQuery({
    queryKey: ["inventory-expiry", days, status, warehouseId, search.trim()],
    queryFn: () =>
      fetchJson<{
        data: ExpiringBatchRow[];
        summary: ExpirySummary;
        meta: { today: string; horizon: string };
      }>(`/api/inventory/expiry?${params.toString()}`),
    placeholderData: keepPreviousData,
  });

  const rows = expiry.data?.data ?? [];
  const summary = expiry.data?.summary;

  return (
    <div className="space-y-4">
      <PageHeader
        kicker="Inventori · Bahan Baku"
        title="Stok Kedaluwarsa"
        description="Batch yang akan kedaluwarsa dan yang sudah lewat tanggal, dinilai dengan biaya rata-rata. Keluarkan stok terdekat lebih dulu (FEFO); yang sudah lewat langsung di-write-off."
      />

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-3 sm:gap-4">
        <StatCard
          label="Sudah kedaluwarsa"
          value={formatRupiah(summary?.expiredValue)}
          hint={`${formatNumber(summary?.expiredBatches, 3)} batch · ${formatNumber(summary?.expiredQty, 3)} unit`}
          icon={<Ban />}
          tone={summary?.expiredBatches ? "danger" : "default"}
        />
        <StatCard
          label={`Kedaluwarsa ≤ ${days} hari`}
          value={formatRupiah(summary?.nearValue)}
          hint={`${formatNumber(summary?.nearBatches, 3)} batch · ${formatNumber(summary?.nearQty, 3)} unit`}
          icon={<AlarmClock />}
          tone={summary?.nearBatches ? "warning" : "default"}
        />
        <StatCard
          label="Total nilai berisiko"
          value={formatRupiah(
            (summary?.expiredValue ?? 0) + (summary?.nearValue ?? 0),
          )}
          icon={<PackageX />}
          tone="ink"
        />
      </div>

      <Card className="grid grid-cols-1 gap-3 p-4 sm:grid-cols-2 lg:grid-cols-4">
        <Select
          value={String(days)}
          onValueChange={(value) => setDays(Number(value) || 30)}
        >
          <SelectTrigger aria-label="Jendela kedaluwarsa">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {WINDOWS.map((window) => (
              <SelectItem key={window} value={String(window)}>
                Dalam {window} hari
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select
          value={status}
          onValueChange={(value) => setStatus((value as StatusFilter) ?? "all")}
        >
          <SelectTrigger aria-label="Status kedaluwarsa">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">Hampir & sudah kedaluwarsa</SelectItem>
            <SelectItem value="near">Hampir kedaluwarsa</SelectItem>
            <SelectItem value="expired">Sudah kedaluwarsa</SelectItem>
          </SelectContent>
        </Select>
        <Combobox
          options={(warehouses.data ?? []).map((w) => ({
            value: w.id,
            label: stallLabel(w),
            description: w.code,
          }))}
          value={warehouseId}
          onChange={setWarehouseId}
          placeholder="Semua gudang / outlet"
          allowClear
        />
        <Input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Cari bahan atau nomor batch"
        />
      </Card>

      <Card className="py-0">
        {expiry.isLoading ? (
          <p className="px-5 py-10 text-center text-sm text-muted-foreground">
            Memuat batch…
          </p>
        ) : expiry.error ? (
          <p className="px-5 py-10 text-center text-sm text-danger">
            {expiry.error.message}
          </p>
        ) : rows.length === 0 ? (
          <p className="px-5 py-10 text-center text-sm text-muted-foreground">
            Tidak ada batch yang kedaluwarsa dalam {days} hari ke depan.
          </p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Bahan & batch</TableHead>
                <TableHead>Kedaluwarsa</TableHead>
                <TableHead className="text-right">Sisa</TableHead>
                <TableHead className="hidden md:table-cell text-right">
                  Nilai berisiko
                </TableHead>
                <TableHead className="text-right">Aksi</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((row) => {
                const expired = row.days_left < 0;
                return (
                  <TableRow key={row.id}>
                    <TableCell className="max-w-[18rem] whitespace-normal">
                      <p className="font-medium">{row.material_nama}</p>
                      <p className="text-xs text-muted-foreground">
                        <span className="font-mono">
                          {row.batch_number ?? "Tanpa nomor batch"}
                        </span>{" "}
                        · {row.warehouse_nama ?? "Tanpa gudang"}
                      </p>
                    </TableCell>
                    <TableCell>
                      <p className="text-sm">{formatDate(row.expiry_date)}</p>
                      <Badge
                        variant={
                          expired
                            ? "destructive"
                            : row.days_left <= 7
                              ? "warning"
                              : "info"
                        }
                      >
                        {daysLabel(row.days_left)}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatNumber(row.qty_remaining, 3)}{" "}
                      <span className="text-xs text-muted-foreground">
                        {row.satuan ?? ""}
                      </span>
                    </TableCell>
                    <TableCell className="hidden md:table-cell text-right tabular-nums">
                      {formatRupiah(row.value_at_risk)}
                      <span className="block text-xs text-muted-foreground">
                        @ {formatRupiah(row.avg_cost)}
                      </span>
                    </TableCell>
                    <TableCell className="text-right">
                      <Button
                        variant={expired ? "destructive" : "ghost"}
                        size="sm"
                        disabled={!row.warehouse_id}
                        onClick={() =>
                          setWriteOff({
                            batchId: row.id,
                            batchNumber: row.batch_number,
                            expiryDate: row.expiry_date,
                            rawMaterialId: row.raw_material_id,
                            materialName: row.material_nama,
                            warehouseId: row.warehouse_id ?? "",
                            warehouseName: row.warehouse_nama,
                            qtyRemaining: row.qty_remaining,
                            satuan: row.satuan,
                          })
                        }
                      >
                        Write-off
                      </Button>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        )}
      </Card>

      {writeOff && (
        <ScrapDialog preset={writeOff} onClose={() => setWriteOff(null)} />
      )}
    </div>
  );
}

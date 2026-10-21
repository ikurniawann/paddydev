"use client";

import { useState } from "react";
import { stallLabel } from "@/lib/auth/stall-label";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { ArrowDownLeft, ArrowUpRight, Download, RotateCcw } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button, buttonVariants } from "@/components/ui/button";
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  MOVEMENT_REFERENCE_LABELS,
  MOVEMENT_TIPE_LABELS,
  movementReferenceLabel,
} from "@/lib/inventory/movement-labels";
import {
  fetchJson,
  useMaterialLookup,
  useWarehouseLookup,
} from "../../shared/inventory-lookups";
import type { MovementFilters, MovementRow } from "../types";
import { formatDateTime, formatNumber, formatRupiah } from "@/lib/format";

const PAGE_SIZE = 50;
const ALL = "all";
const EMPTY_FILTERS: MovementFilters = {
  raw_material_id: "",
  warehouse_id: "",
  tipe: ALL,
  reference_type: ALL,
  reference: "",
  date_from: "",
  date_to: "",
};

function toQueryString(
  filters: MovementFilters,
  extra: Record<string, string> = {},
): string {
  const sp = new URLSearchParams();
  for (const [key, value] of Object.entries(filters)) {
    if (value && value !== ALL) sp.set(key, value.trim());
  }
  for (const [key, value] of Object.entries(extra)) sp.set(key, value);
  return sp.toString();
}

/** Inventori → Mutasi Stok: semua pergerakan bahan baku lintas item, filter, paging, ekspor CSV. */
export function StockMovementsPage() {
  const [filters, setFilters] = useState<MovementFilters>(EMPTY_FILTERS);
  const [page, setPage] = useState(1);
  const warehouses = useWarehouseLookup();
  const materials = useMaterialLookup();

  const set = <K extends keyof MovementFilters>(
    key: K,
    value: MovementFilters[K],
  ) => {
    setFilters((current) => ({ ...current, [key]: value }));
    setPage(1);
  };

  const movements = useQuery({
    queryKey: ["inventory-movements", filters, page],
    queryFn: () =>
      fetchJson<{
        data: MovementRow[];
        meta: { total: number; totalPages: number };
      }>(
        `/api/inventory/movements?${toQueryString(filters, { page: String(page), limit: String(PAGE_SIZE) })}`,
      ),
    placeholderData: keepPreviousData,
  });

  const rows = movements.data?.data ?? [];
  const total = movements.data?.meta.total ?? 0;
  const totalPages = movements.data?.meta.totalPages ?? 1;

  return (
    <div className="space-y-4">
      <PageHeader
        kicker="Inventori · Bahan Baku"
        title="Mutasi Stok"
        description="Semua pergerakan stok bahan baku: penerimaan, pemakaian, transfer, opname, penyesuaian, dan scrap. Ekspor CSV mengikuti filter yang aktif."
        actions={
          <a
            className={buttonVariants({ variant: "outline" })}
            href={`/api/inventory/movements?${toQueryString(filters, { format: "csv" })}`}
            download
          >
            <Download /> Ekspor CSV
          </a>
        }
      />

      <Card className="grid grid-cols-1 gap-3 p-4 sm:grid-cols-2 lg:grid-cols-4">
        <Combobox
          options={(materials.data ?? []).map((m) => ({
            value: m.id,
            label: m.nama,
            description: m.kode,
          }))}
          value={filters.raw_material_id}
          onChange={(value) => set("raw_material_id", value)}
          placeholder="Semua bahan baku"
          allowClear
        />
        <Combobox
          options={(warehouses.data ?? []).map((w) => ({
            value: w.id,
            label: stallLabel(w),
            description: w.code,
          }))}
          value={filters.warehouse_id}
          onChange={(value) => set("warehouse_id", value)}
          placeholder="Semua gudang"
          allowClear
        />
        <Select
          value={filters.tipe}
          onValueChange={(value) => set("tipe", String(value ?? ALL))}
        >
          <SelectTrigger aria-label="Tipe mutasi">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Semua tipe</SelectItem>
            {Object.entries(MOVEMENT_TIPE_LABELS).map(([value, label]) => (
              <SelectItem key={value} value={value}>
                {label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select
          value={filters.reference_type}
          onValueChange={(value) => set("reference_type", String(value ?? ALL))}
        >
          <SelectTrigger aria-label="Sumber mutasi">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Semua sumber</SelectItem>
            {Object.entries(MOVEMENT_REFERENCE_LABELS).map(([value, label]) => (
              <SelectItem key={value} value={value}>
                {label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Input
          value={filters.reference}
          onChange={(e) => set("reference", e.target.value)}
          placeholder="No. referensi (GRN-, TRF-, SCR-…)"
          aria-label="Nomor referensi"
        />
        <Input
          type="date"
          value={filters.date_from}
          onChange={(e) => set("date_from", e.target.value)}
          aria-label="Dari tanggal"
        />
        <Input
          type="date"
          value={filters.date_to}
          onChange={(e) => set("date_to", e.target.value)}
          aria-label="Sampai tanggal"
        />
        <Button
          variant="ghost"
          onClick={() => {
            setFilters(EMPTY_FILTERS);
            setPage(1);
          }}
        >
          <RotateCcw /> Reset filter
        </Button>
      </Card>

      <Card className="py-0">
        {movements.isLoading ? (
          <p className="px-5 py-10 text-center text-sm text-muted-foreground">
            Memuat mutasi…
          </p>
        ) : movements.error ? (
          <p className="px-5 py-10 text-center text-sm text-danger">
            {movements.error.message}
          </p>
        ) : rows.length === 0 ? (
          <p className="px-5 py-10 text-center text-sm text-muted-foreground">
            Tidak ada mutasi untuk filter ini.
          </p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Bahan</TableHead>
                <TableHead className="text-right">Qty</TableHead>
                <TableHead className="hidden md:table-cell text-right">
                  Stok
                </TableHead>
                <TableHead className="hidden lg:table-cell">Sumber</TableHead>
                <TableHead className="hidden xl:table-cell text-right">
                  Nilai
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((row) => (
                <TableRow key={row.id}>
                  <TableCell className="max-w-[18rem] whitespace-normal">
                    <p className="font-medium">{row.material_nama}</p>
                    <p className="text-xs text-muted-foreground">
                      {formatDateTime(row.created_at)} ·{" "}
                      {row.warehouse_nama ?? "Tanpa gudang"}
                    </p>
                    <p className="mt-1 text-xs text-muted-foreground lg:hidden">
                      {movementReferenceLabel(row.reference_type)}{" "}
                      {row.reference_number ?? ""}
                    </p>
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    <span
                      className={
                        row.direction === "out"
                          ? "text-danger"
                          : row.direction === "in"
                            ? "text-success"
                            : undefined
                      }
                    >
                      {row.direction === "out" ? (
                        <ArrowUpRight className="mr-1 inline size-3.5" />
                      ) : null}
                      {row.direction === "in" ? (
                        <ArrowDownLeft className="mr-1 inline size-3.5" />
                      ) : null}
                      {row.direction === "out"
                        ? "-"
                        : row.direction === "in"
                          ? "+"
                          : ""}
                      {formatNumber(row.jumlah, 3)}
                    </span>
                    <span className="block text-xs text-muted-foreground">
                      {row.satuan ?? ""}
                    </span>
                  </TableCell>
                  <TableCell className="hidden md:table-cell text-right tabular-nums text-sm text-muted-foreground">
                    {formatNumber(row.qty_before, 3)} →{" "}
                    <span className="text-foreground">
                      {formatNumber(row.qty_after, 3)}
                    </span>
                  </TableCell>
                  <TableCell className="hidden lg:table-cell max-w-[16rem] whitespace-normal">
                    <Badge
                      variant={
                        row.direction === "out"
                          ? "warning"
                          : row.direction === "in"
                            ? "success"
                            : "muted"
                      }
                    >
                      {MOVEMENT_TIPE_LABELS[row.tipe] ?? row.tipe}
                    </Badge>
                    <p className="mt-1 text-xs">
                      {movementReferenceLabel(row.reference_type)}
                      {row.reference_number ? (
                        <span className="font-mono">
                          {" "}
                          · {row.reference_number}
                        </span>
                      ) : null}
                    </p>
                    {row.batch_numbers && (
                      <p className="text-xs text-muted-foreground">
                        Batch {row.batch_numbers}
                      </p>
                    )}
                  </TableCell>
                  <TableCell className="hidden xl:table-cell text-right tabular-nums">
                    {formatRupiah(row.total_cost)}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </Card>

      <div className="flex flex-wrap items-center justify-between gap-2 text-sm text-muted-foreground">
        <span>{formatNumber(total, 3)} mutasi</span>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={page <= 1}
            onClick={() => setPage((p) => p - 1)}
          >
            Sebelumnya
          </Button>
          <span>
            Halaman {page} / {Math.max(1, totalPages)}
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={page >= totalPages}
            onClick={() => setPage((p) => p + 1)}
          >
            Berikutnya
          </Button>
        </div>
      </div>
    </div>
  );
}

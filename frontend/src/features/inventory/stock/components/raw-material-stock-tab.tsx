"use client";

import { useEffect, useMemo, useState } from "react";
import { stallLabel } from "@/lib/auth/stall-label";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Combobox } from "@/components/ui/combobox";
import { AlertCircle, Eye, Package, Search } from "lucide-react";
import { RM_ROUTES } from "@/lib/purchasing/item-routes";
import { PurchasingListSection } from "@/features/purchasing/components/shared/purchasing-list-section";
import { PurchasingTablePagination } from "@/features/purchasing/components/shared/purchasing-table-pagination";
import { useRawMaterialStock } from "../queries";
import { useWarehouseLookup } from "@/features/inventory/shared/inventory-lookups";
import type { RawStockStatus } from "../types";
import { formatNumber } from "@/lib/format";

const STATUS_STYLES: Record<string, string> = {
  AMAN: "border-emerald-200 bg-emerald-50 text-emerald-700",
  MENIPIS: "border-amber-200 bg-amber-50 text-amber-700",
  HABIS: "border-red-200 bg-red-50 text-red-700",
};

const STATUS_LABELS: Record<string, string> = {
  AMAN: "Aman",
  MENIPIS: "Stok Menipis",
  HABIS: "Stok Habis",
};

type UnitMode = "besar" | "kecil";

const UNIT_OPTIONS = [
  { value: "besar", label: "Satuan Besar" },
  { value: "kecil", label: "Satuan Kecil" },
];

const STATUS_OPTIONS = [
  { value: "all", label: "Semua Status" },
  { value: "normal", label: "Aman" },
  { value: "low_stock", label: "Stok Menipis" },
  { value: "out_of_stock", label: "Stok Habis" },
];

function resolveDisplay(
  item: {
    qty_onhand: number;
    min_stock: number;
    unit_cost: number;
    satuan?: string | null;
    satuan_besar_nama?: string | null;
    satuan_kecil_nama?: string | null;
    konversi_factor?: number | null;
  },
  mode: UnitMode,
) {
  const largeLabel = item.satuan_besar_nama || item.satuan || "—";
  const factor = Number(item.konversi_factor) || 0;
  const hasSmall = Boolean(item.satuan_kecil_nama) && factor > 0;

  if (mode === "kecil" && hasSmall) {
    return {
      qty: Number(item.qty_onhand) * factor,
      min: Number(item.min_stock) * factor,
      unitCost: Number(item.unit_cost) / factor,
      unitLabel: item.satuan_kecil_nama as string,
    };
  }
  return {
    qty: Number(item.qty_onhand) || 0,
    min: Number(item.min_stock) || 0,
    unitCost: Number(item.unit_cost) || 0,
    unitLabel: largeLabel,
  };
}

export function RawMaterialStockTab() {
  const warehousesQuery = useWarehouseLookup();
  const warehouses = useMemo(
    () => warehousesQuery.data ?? [],
    [warehousesQuery.data],
  );
  const loadingWarehouses = warehousesQuery.isLoading;
  const [searchQuery, setSearchQuery] = useState("");
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState("all");
  const [warehouseFilter, setWarehouseFilter] = useState("all");
  const [unitMode, setUnitMode] = useState<UnitMode>("besar");
  const [page, setPage] = useState(1);
  const limit = 20;

  useEffect(() => {
    const timeout = window.setTimeout(() => {
      setSearch(searchQuery.trim());
      setPage(1);
    }, 300);
    return () => window.clearTimeout(timeout);
  }, [searchQuery]);

  const warehouseOptions = useMemo(
    () => [
      { value: "all", label: "Semua Outlet (Total Cabang)" },
      ...warehouses.map((w) => ({
        value: w.id,
        label: stallLabel(w),
        description: w.code,
      })),
    ],
    [warehouses],
  );

  const selectedWarehouseLabel = useMemo(() => {
    if (warehouseFilter === "all") return "Semua Outlet (Total Cabang)";
    const selected = warehouses.find((w) => w.id === warehouseFilter);
    return selected ? stallLabel(selected) : "Outlet";
  }, [warehouseFilter, warehouses]);

  const listQuery = useRawMaterialStock({
    page,
    limit,
    status: statusFilter,
    search: search || undefined,
    warehouse_id: warehouseFilter,
  });

  const items = useMemo(() => listQuery.data?.items ?? [], [listQuery.data]);
  const loading = listQuery.isLoading;
  const total = listQuery.data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / limit));

  const summary = useMemo(() => {
    const attention = items.filter(
      (i) => i.status_stok === "MENIPIS" || i.status_stok === "HABIS",
    ).length;
    const totalValue = items.reduce(
      (s, i) => s + (Number(i.total_value) || 0),
      0,
    );
    return { attention, totalValue };
  }, [items]);

  const getStockStatusBadge = (status: RawStockStatus) => {
    const normalized = status || "AMAN";
    return (
      <Badge
        variant="outline"
        className={STATUS_STYLES[normalized] || STATUS_STYLES.AMAN}
      >
        {normalized === "MENIPIS" || normalized === "HABIS" ? (
          <AlertCircle className="mr-1 inline h-3 w-3" />
        ) : null}
        {STATUS_LABELS[normalized] || normalized}
      </Badge>
    );
  };

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 gap-3 md:grid-cols-3">
        <Card className="border-gray-200/70 shadow-xs">
          <CardContent className="p-4">
            <p className="text-xs font-medium text-gray-500">
              Total Bahan Baku
            </p>
            <p className="mt-1 text-2xl font-bold text-gray-900">{total}</p>
          </CardContent>
        </Card>
        <Card className="border-gray-200/70 shadow-xs">
          <CardContent className="p-4">
            <p className="text-xs font-medium text-gray-500">
              Perlu Perhatian (Halaman Ini)
            </p>
            <p className="mt-1 text-2xl font-bold text-amber-700">
              {summary.attention}
            </p>
          </CardContent>
        </Card>
        <Card className="border-gray-200/70 shadow-xs">
          <CardContent className="p-4">
            <p className="text-xs font-medium text-gray-500">
              Nilai Stok (Halaman Ini)
            </p>
            <p className="mt-1 text-2xl font-bold text-gray-900">
              {formatNumber(summary.totalValue)}
            </p>
          </CardContent>
        </Card>
      </div>

      <PurchasingListSection
        icon={Package}
        title="Daftar Stok Bahan Baku"
        description="Tinjau kode bahan, kategori, jumlah tersedia, stok minimum, harga satuan, nilai stok, dan status stok."
        toolbar={
          <div className="flex w-full flex-col gap-3 lg:flex-row lg:items-center">
            <label className="relative w-full lg:w-80">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
              <Input
                placeholder="Cari kode atau nama bahan..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="h-10 border-gray-200/80 pl-9"
              />
            </label>
            <Combobox
              options={UNIT_OPTIONS}
              value={unitMode}
              onChange={(v) => setUnitMode((v || "besar") as UnitMode)}
              placeholder="Satuan"
              className="h-10 w-full lg:w-40"
            />
            <Combobox
              options={STATUS_OPTIONS}
              value={statusFilter}
              onChange={(v) => {
                setStatusFilter(v || "all");
                setPage(1);
              }}
              placeholder="Semua Status"
              className="h-10 w-full lg:w-44"
            />
            <Combobox
              options={warehouseOptions}
              value={warehouseFilter}
              onChange={(v) => {
                setWarehouseFilter(v || "all");
                setPage(1);
              }}
              placeholder={
                loadingWarehouses ? "Memuat outlet..." : "Semua Outlet"
              }
              searchPlaceholder="Cari outlet..."
              emptyMessage={
                loadingWarehouses ? "Memuat..." : "Outlet tidak ditemukan"
              }
              disabled={loadingWarehouses}
              className="h-10 w-full lg:w-52"
            />
          </div>
        }
      >
        {warehouseFilter === "all" ? (
          <p className="border-b border-gray-200/70 px-5 py-2 text-xs text-gray-500">
            Menampilkan total stok dari seluruh outlet di cabang Anda (bukan per
            lokasi).
          </p>
        ) : (
          <p className="border-b border-gray-200/70 px-5 py-2 text-xs text-gray-500">
            Menampilkan stok per lokasi outlet:{" "}
            <span className="font-medium text-gray-700">
              {selectedWarehouseLabel}
            </span>
          </p>
        )}

        <div className="overflow-x-auto px-4">
          <table className="min-w-full text-sm">
            <thead>
              <tr className="border-b border-gray-200/70 text-xs uppercase tracking-wide text-gray-500">
                <th className="py-3 pr-4 text-left font-semibold">Kode</th>
                <th className="px-3 py-3 text-left font-semibold">
                  Nama Bahan
                </th>
                <th className="px-3 py-3 text-left font-semibold">Kategori</th>
                <th className="px-3 py-3 text-right font-semibold">Tersedia</th>
                <th className="px-3 py-3 text-right font-semibold">Minimum</th>
                <th className="px-3 py-3 text-left font-semibold">Satuan</th>
                <th className="px-3 py-3 text-right font-semibold">
                  Harga Satuan
                </th>
                <th className="px-3 py-3 text-right font-semibold">
                  Nilai Stok
                </th>
                <th className="px-3 py-3 text-center font-semibold">
                  Status Stok
                </th>
                <th className="py-3 pl-3 text-right font-semibold">Aksi</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200/70">
              {loading ? (
                <tr>
                  <td colSpan={10} className="py-12 text-center text-gray-400">
                    Memuat stok bahan baku...
                  </td>
                </tr>
              ) : items.length === 0 ? (
                <tr>
                  <td colSpan={10} className="py-16 text-center text-gray-400">
                    <Package className="mx-auto mb-3 h-12 w-12 opacity-30" />
                    <p>Data stok bahan baku tidak ditemukan</p>
                  </td>
                </tr>
              ) : (
                items.map((item) => {
                  const disp = resolveDisplay(item, unitMode);
                  return (
                    <tr
                      key={item.id}
                      className="transition-colors hover:bg-gray-50/80"
                    >
                      <td className="py-3 pr-4 font-mono text-xs text-gray-600">
                        {item.kode}
                      </td>
                      <td className="px-3 py-3 font-medium text-gray-900">
                        {item.nama}
                      </td>
                      <td className="px-3 py-3 text-xs text-gray-500">
                        {item.kategori || "—"}
                      </td>
                      <td className="px-3 py-3 text-right font-semibold text-blue-700">
                        {formatNumber(disp.qty, 4)}
                      </td>
                      <td className="px-3 py-3 text-right text-gray-700">
                        {formatNumber(disp.min, 4)}
                      </td>
                      <td className="px-3 py-3 text-gray-600">
                        {disp.unitLabel}
                      </td>
                      <td className="px-3 py-3 text-right text-gray-700">
                        {formatNumber(disp.unitCost, 2)}
                      </td>
                      <td className="px-3 py-3 text-right font-medium text-gray-800">
                        {formatNumber(Number(item.total_value) || 0)}
                      </td>
                      <td className="px-3 py-3 text-center">
                        {getStockStatusBadge(item.status_stok)}
                      </td>
                      <td className="py-3 pl-3 text-right">
                        <Link href={RM_ROUTES.materialsDetail(item.id)}>
                          <Button
                            variant="ghost"
                            size="sm"
                            className="cursor-pointer"
                            title="Lihat Detail"
                          >
                            <Eye className="h-4 w-4 text-pink-600" />
                          </Button>
                        </Link>
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>

        <PurchasingTablePagination
          page={page}
          totalPages={totalPages}
          totalItems={total}
          pageSize={limit}
          onPageChange={setPage}
        />
      </PurchasingListSection>
    </div>
  );
}

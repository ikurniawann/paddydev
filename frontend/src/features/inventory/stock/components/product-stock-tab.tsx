"use client";

import { Fragment, useEffect, useMemo, useState } from "react";
import { stallLabel } from "@/lib/auth/stall-label";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Combobox } from "@/components/ui/combobox";
import { STALL_LABELS } from "@/lib/configuration/stall-labels";
import {
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Package,
  Search,
  ShoppingBag,
  X,
  XCircle,
} from "lucide-react";
import { PRODUCT_ROUTES } from "@/lib/purchasing/item-routes";
import { PurchasingListSection } from "@/features/purchasing/components/shared/purchasing-list-section";
import { PurchasingTablePagination } from "@/features/purchasing/components/shared/purchasing-table-pagination";
import type { ProductStockVariant } from "../types";
import { useProductStock } from "../queries";
import { useWarehouseLookup } from "@/features/inventory/shared/inventory-lookups";
import { formatDate, formatNumber } from "@/lib/format";

/** EPIC-047 Fase 1C — label baris varian: nilai opsi (mis. "M / Hitam"), fallback nama SKU. */
function variantLabel(variant: ProductStockVariant) {
  const values = Object.values(variant.options ?? {}).filter(Boolean);
  return values.length > 0 ? values.join(" / ") : variant.name;
}

const STATUS_OPTIONS = [
  { value: "all", label: "Semua Status" },
  { value: "in_stock", label: "Tersedia" },
  { value: "out_of_stock", label: "Habis" },
];

export function ProductStockTab() {
  const warehousesQuery = useWarehouseLookup();
  const warehouses = useMemo(
    () => warehousesQuery.data ?? [],
    [warehousesQuery.data],
  );
  const loadingWarehouses = warehousesQuery.isLoading;
  const [searchQuery, setSearchQuery] = useState("");
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState("all");
  const [stallFilter, setStallFilter] = useState("all");
  const [page, setPage] = useState(1);
  const limit = 10;
  // EPIC-047 Fase 1C — baris produk mana yang expand rincian per varian.
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set());

  const toggleVariants = (id: string) => {
    setExpandedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  useEffect(() => {
    const timeout = window.setTimeout(() => {
      setSearch(searchQuery.trim());
      setPage(1);
    }, 300);
    return () => window.clearTimeout(timeout);
  }, [searchQuery]);

  const listQuery = useProductStock({
    page,
    limit,
    status: statusFilter,
    search: search || undefined,
    warehouse_id: stallFilter !== "all" ? stallFilter : undefined,
  });

  const stallOptions = useMemo(
    () => [
      { value: "all", label: STALL_LABELS.allBranchTotal },
      ...warehouses.map((w) => ({
        value: w.id,
        label: stallLabel(w),
        description: w.code,
      })),
    ],
    [warehouses],
  );

  const items = useMemo(() => listQuery.data?.items ?? [], [listQuery.data]);
  const loading = listQuery.isLoading;
  const total = listQuery.data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / limit));

  const summary = useMemo(() => {
    const out = items.filter((i) => (Number(i.qty_available) || 0) <= 0).length;
    const totalValue = items.reduce(
      (s, i) => s + (Number(i.total_value) || 0),
      0,
    );
    return { out, totalValue };
  }, [items]);

  const handleResetFilters = () => {
    setSearchQuery("");
    setSearch("");
    setStatusFilter("all");
    setStallFilter("all");
    setPage(1);
  };

  const hasActiveFilters =
    Boolean(search) ||
    statusFilter !== "all" ||
    stallFilter !== "all" ||
    page > 1;

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 gap-3 md:grid-cols-3">
        <Card className="border-gray-200/70 shadow-xs">
          <CardContent className="p-4">
            <p className="text-xs font-medium text-gray-500">Total Produk</p>
            <p className="mt-1 text-2xl font-bold text-gray-900">{total}</p>
          </CardContent>
        </Card>
        <Card className="border-gray-200/70 shadow-xs">
          <CardContent className="p-4">
            <p className="text-xs font-medium text-gray-500">
              Habis (Halaman Ini)
            </p>
            <p className="mt-1 text-2xl font-bold text-red-600">
              {summary.out}
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
        title="Daftar Stok Produk"
        description="Tinjau kode produk, outlet, kategori, qty tersedia, harga pokok, harga jual, nilai stok, dan status ketersediaan."
        toolbar={
          <div className="flex w-full flex-col gap-3 lg:flex-row lg:items-center">
            <Combobox
              options={stallOptions}
              value={stallFilter}
              onChange={(value) => {
                setStallFilter(value || "all");
                setPage(1);
              }}
              placeholder={
                loadingWarehouses
                  ? STALL_LABELS.loading
                  : STALL_LABELS.allBranchTotal
              }
              searchPlaceholder={STALL_LABELS.search}
              emptyMessage={STALL_LABELS.empty}
              disabled={loadingWarehouses}
              className="h-10 w-full lg:w-48"
            />
            <label className="relative w-full lg:w-80">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
              <Input
                placeholder="Cari kode atau nama produk..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="h-10 border-gray-200/80 pl-9 pr-10 text-sm focus:border-pink-400 focus:ring-2 focus:ring-pink-100"
              />
              {searchQuery && (
                <button
                  type="button"
                  onClick={() => setSearchQuery("")}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 transition-colors hover:text-gray-700"
                  aria-label="Hapus pencarian"
                >
                  <X className="h-4 w-4" />
                </button>
              )}
            </label>
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
            {hasActiveFilters && (
              <Button
                variant="outline"
                onClick={handleResetFilters}
                className="h-10 shrink-0 rounded-lg"
              >
                Atur Ulang
              </Button>
            )}
          </div>
        }
      >
        <div className="overflow-x-auto">
          <table className="min-w-full text-sm">
            <thead className="border-b border-gray-100 bg-gray-50 text-xs uppercase tracking-wide text-gray-500">
              <tr>
                <th className="px-4 py-3 text-left font-semibold">Kode</th>
                <th className="px-4 py-3 text-left font-semibold">
                  Nama Produk
                </th>
                <th className="px-4 py-3 text-left font-semibold">Outlet</th>
                <th className="px-4 py-3 text-left font-semibold">Kategori</th>
                <th className="px-4 py-3 text-right font-semibold">Tersedia</th>
                <th className="px-4 py-3 text-left font-semibold">Satuan</th>
                <th className="px-4 py-3 text-right font-semibold">
                  HPP / Satuan
                </th>
                <th className="px-4 py-3 text-right font-semibold">
                  Harga Jual
                </th>
                <th className="px-4 py-3 text-right font-semibold">
                  Nilai Stok
                </th>
                <th className="px-4 py-3 text-center font-semibold">Status</th>
                <th className="px-4 py-3 text-left font-semibold">
                  Pembaruan Terakhir
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {loading ? (
                <tr>
                  <td
                    colSpan={11}
                    className="px-4 py-12 text-center text-gray-400"
                  >
                    Memuat stok produk...
                  </td>
                </tr>
              ) : items.length === 0 ? (
                <tr>
                  <td
                    colSpan={11}
                    className="px-4 py-16 text-center text-gray-400"
                  >
                    <ShoppingBag className="mx-auto mb-3 h-12 w-12 opacity-30" />
                    <p>Data stok produk tidak ditemukan</p>
                    <p className="mt-1 text-xs text-gray-500">
                      Stok muncul otomatis setelah produksi atau penyelesaian
                      barang jadi
                    </p>
                  </td>
                </tr>
              ) : (
                items.map((item) => {
                  const qty = Number(item.qty_available) || 0;
                  const isOut = qty <= 0;
                  const variants = item.variants ?? [];
                  const variantCount = item.variant_count ?? variants.length;
                  const hasVariants = variantCount > 0;
                  const isExpanded = hasVariants && expandedIds.has(item.id);
                  const variantSum = variants.reduce(
                    (sum, v) => sum + (Number(v.stock_quantity) || 0),
                    0,
                  );
                  const sumMismatch = hasVariants && variantSum !== qty;

                  return (
                    <Fragment key={item.id}>
                      <tr className="hover:bg-gray-50">
                        <td className="px-4 py-3 font-mono text-xs text-gray-600">
                          {item.product_kode}
                        </td>
                        <td className="px-4 py-3">
                          <div className="flex items-center gap-1.5">
                            {hasVariants && (
                              <button
                                type="button"
                                onClick={() => toggleVariants(item.id)}
                                className="shrink-0 rounded p-0.5 text-gray-400 transition hover:bg-gray-100 hover:text-gray-700"
                                aria-label={
                                  isExpanded
                                    ? "Sembunyikan varian"
                                    : "Tampilkan varian"
                                }
                              >
                                {isExpanded ? (
                                  <ChevronDown className="h-3.5 w-3.5" />
                                ) : (
                                  <ChevronRight className="h-3.5 w-3.5" />
                                )}
                              </button>
                            )}
                            {item.product_id ? (
                              <Link
                                href={PRODUCT_ROUTES.productsDetail(
                                  item.product_id,
                                )}
                                className="font-medium text-pink-700 hover:underline"
                              >
                                {item.product_nama}
                              </Link>
                            ) : (
                              <span className="font-medium text-gray-900">
                                {item.product_nama}
                              </span>
                            )}
                            {hasVariants && (
                              <button
                                type="button"
                                onClick={() => toggleVariants(item.id)}
                                className="shrink-0 rounded-full bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-500 transition hover:bg-gray-200"
                              >
                                {variantCount} varian
                              </button>
                            )}
                          </div>
                          {sumMismatch && (
                            <p className="mt-0.5 text-[10px] text-amber-600">
                              Σ varian ≠ total
                            </p>
                          )}
                        </td>
                        <td className="px-4 py-3 text-xs text-gray-600">
                          {item.warehouse_name || item.warehouse_code || "—"}
                        </td>
                        <td className="px-4 py-3 text-xs text-gray-500">
                          {item.product_kategori || "—"}
                        </td>
                        <td className="px-4 py-3 text-right font-semibold text-pink-700">
                          {formatNumber(qty, 4)}
                        </td>
                        <td className="px-4 py-3 text-gray-600">
                          {item.satuan_nama || "—"}
                        </td>
                        <td className="px-4 py-3 text-right text-gray-700">
                          {formatNumber(Number(item.unit_cost) || 0)}
                        </td>
                        <td className="px-4 py-3 text-right text-gray-700">
                          {formatNumber(Number(item.harga_jual) || 0)}
                        </td>
                        <td className="px-4 py-3 text-right font-medium text-gray-800">
                          {formatNumber(Number(item.total_value) || 0)}
                        </td>
                        <td className="px-4 py-3 text-center">
                          {isOut ? (
                            <Badge
                              variant="outline"
                              className="border-red-200 bg-red-50 text-red-700"
                            >
                              <XCircle className="mr-1 inline h-3 w-3" />
                              Habis
                            </Badge>
                          ) : (
                            <Badge
                              variant="outline"
                              className="border-emerald-200 bg-emerald-50 text-emerald-700"
                            >
                              <CheckCircle2 className="mr-1 inline h-3 w-3" />
                              Tersedia
                            </Badge>
                          )}
                        </td>
                        <td className="px-4 py-3 text-xs text-gray-500">
                          {item.last_movement_at
                            ? formatDate(item.last_movement_at)
                            : "—"}
                        </td>
                      </tr>
                      {isExpanded &&
                        variants.map((variant) => (
                          <tr
                            key={variant.sku_id}
                            className="bg-gray-50/60 text-xs text-gray-500"
                          >
                            <td className="px-4 py-2 pl-8 font-mono">
                              {variant.sku}
                            </td>
                            <td className="px-4 py-2" colSpan={3}>
                              {variantLabel(variant)}
                            </td>
                            <td className="px-4 py-2 text-right font-medium text-gray-600">
                              {formatNumber(variant.stock_quantity, 4)}
                            </td>
                            <td colSpan={6} />
                          </tr>
                        ))}
                    </Fragment>
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

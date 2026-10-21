"use client";

import { useState, useEffect } from "react";
import { defaultWarehouseId } from "../default-warehouse";
import { stallLabel } from "@/lib/auth/stall-label";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Package, Calculator, Plus, Trash2, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Combobox } from "@/components/ui/combobox";
import { NumericInput } from "@/components/ui/numeric-input";
import type { BOMItemFormData, ProductFormData } from "@/types/purchasing";
import { PRODUCT_ROUTES } from "@/lib/purchasing/item-routes";
import {
  PurchasingFormFooter,
  PurchasingFormHeader,
} from "@/features/purchasing/components/shared/purchasing-page-header";
import { useProductFormData, useProductCategoryOptions, useProductWarehouses } from "../queries";
import { useCreateProduct, useCreateBOMItem } from "../mutations";
import { mapUnitComboboxOptions } from "../product-unit";
import { ProductInfoFields } from "./product-info-fields";
import { ProductPriceFields } from "./product-price-fields";
import { STALL_LABELS } from "@/lib/configuration/stall-labels";
import { formatNumber } from "@/lib/format";
import {
  EMPTY_PRODUCT_FORM,
  bomLineCost,
  calculateMarkupFromPrice,
  materialSmallUnitLabel,
} from "@/lib/purchasing/product-ui-form";

interface BOMFormItem extends Partial<BOMItemFormData> {
  id: string;
  raw_material_name?: string;
  raw_material_unit?: string;
  subtotal: number;
}

export function NewProductPage() {
  const router = useRouter();

  const formDataQuery = useProductFormData();
  const materials = formDataQuery.data?.materials ?? [];
  const units = formDataQuery.data?.units ?? [];
  const unitOptions = mapUnitComboboxOptions(units);
  const loading = formDataQuery.isLoading;

  const categoriesQuery = useProductCategoryOptions();
  const warehousesQuery = useProductWarehouses();
  const categoryOptions = (categoriesQuery.data ?? []).map((row) => ({
    value: row.code,
    label: row.nama,
    description: row.deskripsi || undefined,
  }));

  const createMutation = useCreateProduct();
  const createBomMutation = useCreateBOMItem();
  const isSubmitting = createMutation.isPending || createBomMutation.isPending;

  const [formData, setFormData] = useState<ProductFormData>(EMPTY_PRODUCT_FORM);
  const stallOptions = (warehousesQuery.data ?? []).map((w) => ({
    value: w.id,
    label: stallLabel(w),
    description: w.code,
  }));
  const [bomItems, setBomItems] = useState<BOMFormItem[]>([]);

  useEffect(() => {
    if (formDataQuery.isError) {
      toast.error(formDataQuery.error instanceof Error ? formDataQuery.error.message : "Gagal memuat data formulir");
    }
  }, [formDataQuery.isError, formDataQuery.error]);

  // Gudang bawaan dipakai selama user belum memilih sendiri; lintas branch tidak
  // ada bawaan (wajib dipilih, divalidasi saat submit).
  const warehouses = warehousesQuery.data ?? [];
  const warehouseId = formData.warehouse_id || defaultWarehouseId(warehouses);
  const effectiveForm = { ...formData, warehouse_id: warehouseId };

  const addBOMItem = () => {
    setBomItems([
      ...bomItems,
      {
        id: crypto.randomUUID(),
        raw_material_id: "",
        qty_needed: 0,
        waste_persen: 0,
        subtotal: 0,
      },
    ]);
  };

  const removeBOMItem = (id: string) => {
    setBomItems(bomItems.filter((item) => item.id !== id));
  };

  const updateBOMItem = (id: string, updates: Partial<BOMFormItem>) => {
    setBomItems(
      bomItems.map((item) => {
        if (item.id === id) {
          const updated = { ...item, ...updates };
          const material = materials.find((m) => m.id === updated.raw_material_id);
          updated.subtotal = bomLineCost(material, updated.qty_needed || 0, updated.waste_persen || 0);
          return updated;
        }
        return item;
      })
    );
  };

  const totalCost = bomItems.reduce((sum, item) => sum + (item.subtotal || 0), 0);

  const markupPersen = calculateMarkupFromPrice(totalCost, formData.harga_jual || 0);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!formData.nama) {
      toast.error("Nama produk wajib diisi");
      return;
    }
    if (!formData.satuan_id) {
      toast.error("Satuan wajib diisi");
      return;
    }
    if (!warehouseId) {
      toast.error(`${STALL_LABELS.singular} wajib dipilih`);
      return;
    }
    if (!formData.station) {
      toast.error("Station wajib dipilih");
      return;
    }

    try {
      const productData = {
        ...effectiveForm,
        markup_persen: markupPersen,
        harga_modal: totalCost,
      };
      const product = await createMutation.mutateAsync(productData);

      for (const item of bomItems) {
        if (item.raw_material_id) {
          await createBomMutation.mutateAsync({
            productId: product.id,
            payload: {
              raw_material_id: item.raw_material_id,
              qty_needed: item.qty_needed || 0,
              waste_persen: item.waste_persen || 0,
            },
          });
        }
      }

      toast.success("Produk berhasil ditambahkan");
      router.push(PRODUCT_ROUTES.products);
    } catch (error: unknown) {
      toast.error(error instanceof Error ? error.message : "Gagal menambahkan produk");
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center py-16 text-sm text-gray-500">
        <Loader2 className="mr-2 h-5 w-5 animate-spin text-brand-text" />
        Memuat data formulir...
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <PurchasingFormHeader
        backHref={PRODUCT_ROUTES.products}
        title="Tambah Produk"
        description="Isi detail produk dan resep (BOM)"
      />

      <form id="new-product-form" onSubmit={handleSubmit} className="space-y-6">
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-12">
          <div className="space-y-6 lg:col-span-8">
            <Card className="border-gray-200/70 shadow-xs">
              <CardHeader className="pb-3">
                <CardTitle className="flex items-center gap-2 text-base">
                  <Package className="h-4 w-4" />
                  Informasi Produk
                </CardTitle>
              </CardHeader>
              <CardContent>
                <ProductInfoFields
                  formData={effectiveForm}
                  onChange={(patch) => setFormData((prev) => ({ ...prev, ...patch }))}
                  stallOptions={stallOptions}
                  categoryOptions={categoryOptions}
                  unitOptions={unitOptions}
                  warehousesLoading={warehousesQuery.isLoading}
                  categoriesLoading={categoriesQuery.isLoading}
                  unitsLoading={loading}
                />
              </CardContent>
            </Card>

            <Card className="border-gray-200/70 shadow-xs">
              <CardHeader className="flex flex-row items-center justify-between pb-3">
                <CardTitle className="flex items-center gap-2 text-base">
                  <Package className="h-4 w-4" />
                  Resep (BOM)
                </CardTitle>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={addBOMItem}
                  className="h-8 border-primary/20 text-xs text-brand-text hover:bg-primary/5"
                >
                  <Plus className="mr-1 h-3 w-3" />
                  Tambah Bahan
                </Button>
              </CardHeader>
              {bomItems.length === 0 ? (
                <CardContent>
                  <div className="py-8 text-center text-sm text-muted-foreground">
                    Belum ada bahan baku. Klik &quot;Tambah Bahan&quot; untuk memulai.
                  </div>
                </CardContent>
              ) : (
                <CardContent className="p-0">
                  <div className="overflow-x-auto px-4">
                    <table className="w-full min-w-180 text-sm">
                      <thead>
                        <tr className="border-b border-gray-200/70 text-left text-xs text-muted-foreground">
                          <th className="py-2.5 pr-3 font-medium">Bahan Baku</th>
                          <th className="w-44 py-2.5 pr-3 font-medium">Qty</th>
                          <th className="w-32 py-2.5 pr-3 font-medium">Susut</th>
                          <th className="w-36 py-2.5 pr-3 text-right font-medium">Subtotal</th>
                          <th className="w-12 py-2.5 text-right font-medium">
                            <span className="sr-only">Aksi</span>
                          </th>
                        </tr>
                      </thead>
                      <tbody>
                        {bomItems.map((item) => {
                          const selectedMaterial = materials.find(
                            (material) => material.id === item.raw_material_id
                          );
                          const smallUnitLabel = materialSmallUnitLabel(selectedMaterial);

                          return (
                            <tr key={item.id} className="border-b border-gray-200/70 last:border-0">
                              <td className="py-2.5 pr-3 align-middle">
                                <Combobox
                                  options={materials.map((m) => ({
                                    value: m.id,
                                    label: m.nama,
                                    description: m.kode,
                                  }))}
                                  value={item.raw_material_id}
                                  onChange={(v) => updateBOMItem(item.id, { raw_material_id: v })}
                                  placeholder="Pilih bahan..."
                                  searchPlaceholder="Cari..."
                                  emptyMessage="Bahan tidak ditemukan"
                                  allowClear
                                  className="h-9 text-sm"
                                />
                              </td>
                              <td className="py-2.5 pr-3 align-middle">
                                <div className="flex rounded-lg border border-gray-200/70 bg-card focus-within:border-border focus-within:ring-1 focus-within:ring-primary/30">
                                  <NumericInput
                                    step="0.01"
                                    min="0"
                                    value={item.qty_needed}
                                    onValueChange={(value) =>
                                      updateBOMItem(item.id, { qty_needed: value })
                                    }
                                    decimalScale={4}
                                    className="h-9 rounded-r-none border-0 text-sm shadow-none focus-visible:ring-0"
                                  />
                                  <div className="flex min-w-14 items-center justify-center rounded-r-lg border-l border-gray-200/70 bg-muted/50 px-3 text-xs font-semibold uppercase text-muted-foreground">
                                    {smallUnitLabel}
                                  </div>
                                </div>
                              </td>
                              <td className="py-2.5 pr-3 align-middle">
                                <div className="flex rounded-lg border border-gray-200/70 bg-card focus-within:border-border focus-within:ring-1 focus-within:ring-primary/30">
                                  <NumericInput
                                    min="0"
                                    max="100"
                                    value={item.waste_persen}
                                    onValueChange={(value) =>
                                      updateBOMItem(item.id, { waste_persen: value })
                                    }
                                    decimalScale={2}
                                    className="h-9 rounded-r-none border-0 text-sm shadow-none focus-visible:ring-0"
                                  />
                                  <div className="flex min-w-10 items-center justify-center rounded-r-lg border-l border-gray-200/70 bg-muted/50 px-3 text-xs font-semibold text-muted-foreground">
                                    %
                                  </div>
                                </div>
                              </td>
                              <td className="py-2.5 pr-3 align-middle">
                                <div className="flex h-9 items-center justify-end rounded-lg border border-gray-200/70 bg-muted/50 px-3 font-mono text-sm">
                                  {formatNumber(item.subtotal)}
                                </div>
                              </td>
                              <td className="py-2.5 text-right align-middle">
                                <Button
                                  type="button"
                                  variant="ghost"
                                  size="icon"
                                  onClick={() => removeBOMItem(item.id)}
                                  title="Hapus bahan"
                                  className="h-9 w-9 text-red-600 hover:bg-red-50 hover:text-red-700"
                                >
                                  <Trash2 className="h-4 w-4" />
                                </Button>
                              </td>
                            </tr>
                          );
                        })}
                      </tbody>
                    </table>
                  </div>
                  <div className="flex justify-end border-t border-gray-200/70 px-4 py-3">
                    <div className="text-right">
                      <p className="text-xs text-muted-foreground">Total Estimasi HPP</p>
                      <p className="text-lg font-semibold text-foreground">{formatNumber(totalCost)}</p>
                    </div>
                  </div>
                </CardContent>
              )}
            </Card>
          </div>

          <Card className="h-fit border-gray-200/70 shadow-xs lg:col-span-4">
            <CardHeader className="pb-3">
              <CardTitle className="flex items-center gap-2 text-base">
                <Calculator className="h-4 w-4" />
                Harga &amp; HPP
              </CardTitle>
            </CardHeader>
            <CardContent>
              <ProductPriceFields
                totalCost={totalCost}
                markupPersen={markupPersen}
                hargaJual={formData.harga_jual}
                onHargaJualChange={(harga_jual) => setFormData((prev) => ({ ...prev, harga_jual }))}
              />
            </CardContent>
          </Card>
        </div>

        <PurchasingFormFooter
          formId="new-product-form"
          onCancel={() => router.back()}
          submitLabel="Simpan Produk"
          loading={isSubmitting}
        />
      </form>
    </div>
  );
}

"use client";

import { useState, useEffect } from "react";
import { stallLabel } from "@/lib/auth/stall-label";
import { useRouter, useParams } from "next/navigation";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Package, Calculator, Loader2 } from "lucide-react";
import { toast } from "sonner";
import type { ProductFormData } from "@/types/purchasing";
import { PRODUCT_ROUTES } from "@/lib/purchasing/item-routes";
import {
  PurchasingFormFooter,
  PurchasingFormHeader,
} from "@/features/purchasing/components/shared/purchasing-page-header";
import { useProductEditData, useProductCategoryOptions, useProductWarehouses } from "../queries";
import { useUpdateProduct } from "../mutations";
import { mapUnitComboboxOptions } from "../product-unit";
import { ProductInfoFields } from "./product-info-fields";
import { ProductPriceFields } from "./product-price-fields";
import { STALL_LABELS } from "@/lib/configuration/stall-labels";
import {
  EMPTY_PRODUCT_FORM,
  bomLineCost,
  calculateMarkupFromPrice,
  getBomQty,
  getBomWastePercent,
  materialSmallUnitLabel,
  productFormFromProduct,
} from "@/lib/purchasing/product-ui-form";
import { formatNumber } from "@/lib/format";

export function EditProductPage() {
  const params = useParams();
  const productId = params.id as string;
  const editQuery = useProductEditData(productId);

  useEffect(() => {
    if (editQuery.isError) toast.error("Gagal memuat data produk");
  }, [editQuery.isError]);

  if (editQuery.isLoading) {
    return (
      <div className="flex items-center justify-center py-16 text-sm text-gray-500">
        <Loader2 className="mr-2 h-5 w-5 animate-spin text-brand-text" />
        Memuat produk...
      </div>
    );
  }

  const data = editQuery.data;
  // key: form diinisialisasi ulang dari data server yang baru dimuat.
  return <EditProductForm key={editQuery.dataUpdatedAt} productId={productId} data={data} />;
}

interface EditProductFormProps {
  productId: string;
  data: ReturnType<typeof useProductEditData>["data"];
}

function EditProductForm({ productId, data }: EditProductFormProps) {
  const router = useRouter();
  const product = data?.product ?? null;
  const materials = data?.materials ?? [];
  const unitOptions = mapUnitComboboxOptions(data?.units ?? []);
  const bomItems = data?.bom ?? [];

  const categoriesQuery = useProductCategoryOptions();
  const warehousesQuery = useProductWarehouses();
  const categoryOptions = (categoriesQuery.data ?? []).map((row) => ({
    value: row.code,
    label: row.nama,
    description: row.deskripsi || undefined,
  }));

  const updateMutation = useUpdateProduct();
  const isSubmitting = updateMutation.isPending;

  const [formData, setFormData] = useState<ProductFormData>(() =>
    product ? productFormFromProduct(product) : EMPTY_PRODUCT_FORM
  );
  const stallOptions = (warehousesQuery.data ?? []).map((w) => ({
    value: w.id,
    label: stallLabel(w),
    description: w.code,
  }));

  const totalCost = bomItems.reduce((sum, item) => {
    const material = materials.find((m) => m.id === item.raw_material_id);
    return sum + bomLineCost(material, getBomQty(item), getBomWastePercent(item));
  }, 0);
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
    if (!formData.warehouse_id) {
      toast.error(`${STALL_LABELS.singular} wajib dipilih`);
      return;
    }
    if (!formData.station) {
      toast.error("Station wajib dipilih");
      return;
    }

    try {
      await updateMutation.mutateAsync({
        id: productId,
        payload: {
          ...formData,
          harga_jual: Number(formData.harga_jual) || 0,
          markup_persen: markupPersen,
          harga_modal: Number(totalCost) || 0,
        },
      });
      toast.success("Produk berhasil diperbarui");
      router.push(PRODUCT_ROUTES.productsDetail(productId));
    } catch (error: unknown) {
      toast.error(error instanceof Error ? error.message : "Gagal memperbarui produk");
    }
  };

  return (
    <div className="space-y-6">
      <PurchasingFormHeader
        backHref={PRODUCT_ROUTES.productsDetail(productId)}
        title="Ubah Produk"
        description={product?.nama ? `Perbarui detail untuk ${product.nama}` : "Perbarui detail produk"}
        actions={
          <Link href={PRODUCT_ROUTES.productsBom(productId)}>
            <Button variant="outline" className="purchasing-secondary-button w-full sm:w-auto">
              <Calculator className="mr-2 h-4 w-4" />
              Ubah Resep (BOM)
            </Button>
          </Link>
        }
      />

      <form id="edit-product-form" onSubmit={handleSubmit} className="space-y-6">
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
                  formData={formData}
                  onChange={(patch) => setFormData((prev) => ({ ...prev, ...patch }))}
                  stallOptions={stallOptions}
                  categoryOptions={categoryOptions}
                  unitOptions={unitOptions}
                  warehousesLoading={warehousesQuery.isLoading}
                  categoriesLoading={categoriesQuery.isLoading}
                  unitsLoading={false}
                />
              </CardContent>
            </Card>

            <Card className="border-gray-200/70 shadow-xs">
              <CardHeader className="flex flex-row items-center justify-between pb-3">
                <CardTitle className="flex items-center gap-2 text-base">
                  <Package className="h-4 w-4" />
                  Resep (BOM)
                </CardTitle>
                <Badge variant="secondary" className="text-xs">
                  {bomItems.length} bahan
                </Badge>
              </CardHeader>
              {bomItems.length === 0 ? (
                <CardContent>
                  <div className="py-8 text-center text-sm text-muted-foreground">
                    Belum ada resep (BOM).{" "}
                    <Link
                      href={PRODUCT_ROUTES.productsBom(productId)}
                      className="font-medium text-brand-text hover:underline"
                    >
                      Ubah resep (BOM)
                    </Link>{" "}
                    untuk menambah komponen.
                  </div>
                </CardContent>
              ) : (
                <CardContent className="p-0">
                  <div className="overflow-x-auto px-4">
                    <table className="w-full min-w-160 text-sm">
                      <thead>
                        <tr className="border-b border-gray-200/70 text-left text-xs text-muted-foreground">
                          <th className="py-2.5 pr-3 font-medium">Bahan Baku</th>
                          <th className="w-36 py-2.5 pr-3 text-right font-medium">Qty</th>
                          <th className="w-28 py-2.5 pr-3 text-right font-medium">Susut</th>
                          <th className="w-36 py-2.5 text-right font-medium">Subtotal</th>
                        </tr>
                      </thead>
                      <tbody>
                        {bomItems.map((item) => {
                          const material = materials.find((m) => m.id === item.raw_material_id);
                          const qty = getBomQty(item);
                          const wastePercent = getBomWastePercent(item);
                          const smallUnitLabel = materialSmallUnitLabel(material);
                          const subtotal = bomLineCost(material, qty, wastePercent);

                          return (
                            <tr key={item.id} className="border-b border-gray-200/70 last:border-0">
                              <td className="py-2.5 pr-3 align-middle">
                                <p className="font-medium text-foreground">
                                  {material?.nama || "Tidak diketahui"}
                                </p>
                                <p className="text-xs text-muted-foreground">{material?.kode}</p>
                              </td>
                              <td className="py-2.5 pr-3 text-right align-middle tabular-nums text-foreground">
                                {formatNumber(qty, 4)}{" "}
                                <span className="text-xs uppercase text-muted-foreground">
                                  {smallUnitLabel}
                                </span>
                              </td>
                              <td className="py-2.5 pr-3 text-right align-middle tabular-nums text-foreground">
                                {formatNumber(wastePercent, 2)}%
                              </td>
                              <td className="py-2.5 text-right align-middle font-mono text-foreground">
                                {formatNumber(subtotal)}
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
          formId="edit-product-form"
          onCancel={() => router.back()}
          submitLabel="Simpan Perubahan"
          loading={isSubmitting}
        />
      </form>
    </div>
  );
}

import { listProducts, getProduct, createProduct, updateProduct, applyProductRecipeHpp, updateProductStatus, deleteProduct, listBOMItems, createBOMItem, updateBOMItem, deleteBOMItem } from "@/lib/purchasing/api-client/products";
import { listRawMaterials } from "@/lib/purchasing/api-client/raw-materials";
import { listUnits } from "@/lib/purchasing/api-client/units";
import type {
  BOMItem,
  ProductWithCOGS,
  RawMaterialWithStock,
  Unit,
} from "@/types/purchasing";

export {
  listProducts,
  getProduct,
  createProduct,
  updateProduct,
  applyProductRecipeHpp,
  updateProductStatus,
  deleteProduct,
  listBOMItems,
  createBOMItem,
  updateBOMItem,
  deleteBOMItem,
};

export interface ProductFormDeps {
  materials: RawMaterialWithStock[];
  units: Unit[];
}

export async function getProductFormData(): Promise<ProductFormDeps> {
  const [materialsData, unitsData] = await Promise.all([
    listRawMaterials({ limit: 100 }),
    listUnits(),
  ]);
  return {
    materials: materialsData.data,
    units: unitsData.data || [],
  };
}

export interface ProductEditData {
  product: ProductWithCOGS;
  bom: BOMItem[];
  materials: RawMaterialWithStock[];
  units: Unit[];
}

export async function getProductEditData(id: string): Promise<ProductEditData> {
  const [product, bom, materialsData, unitsData] = await Promise.all([
    getProduct(id),
    listBOMItems(id),
    listRawMaterials({ limit: 100 }),
    listUnits(),
  ]);
  return {
    product,
    bom,
    materials: materialsData.data,
    units: unitsData.data || [],
  };
}

export interface ProductBomEditorData {
  product: ProductWithCOGS;
  bom: BOMItem[];
  materials: RawMaterialWithStock[];
}

export async function getProductBomEditorData(
  id: string
): Promise<ProductBomEditorData> {
  const [product, bom, materialsData] = await Promise.all([
    getProduct(id),
    listBOMItems(id),
    listRawMaterials({ is_active: true, limit: 200 }),
  ]);
  return {
    product,
    bom,
    materials: materialsData.data,
  };
}

export interface ProductWarehouseOption {
  id: string;
  name: string;
  code: string;
  is_default?: boolean;
  branch_name?: string;
  label?: string;
}

export async function listProductWarehouses(): Promise<ProductWarehouseOption[]> {
  const res = await fetch("/api/purchasing/warehouses");
  const json = (await res.json()) as {
    success?: boolean;
    data?: ProductWarehouseOption[];
    message?: string;
    error?: string;
  };
  if (!res.ok) {
    throw new Error(json.error || json.message || "Failed to load outlets");
  }
  return json.data ?? [];
}

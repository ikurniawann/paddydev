/**
 * POST /api/purchasing/grn — catat penerimaan barang dari delivery (atau PO
 * general tanpa langkah delivery), lalu untuk raw_material/product langsung
 * selesaikan QC + posting stok dalam permintaan yang sama.
 */
import { ApiError, type ApiUser } from "@/lib/api/auth";
import {
  getApiUserScope,
  resolveBusinessScopeFromWarehouse,
  validateWarehouseForReceivingScope,
} from "@/lib/api/scope";
import { recordAuditAfterCommit } from "@/lib/audit";
import type { DbClient } from "@/lib/pg/types";
import { validatePOCanDelivery } from "@/lib/purchasing/delivery";
import {
  calculateGrnTotals,
  generateGrnNumber,
  updateDeliveryStatusAfterGrn,
  updatePOStatusAfterGrn,
  validateDeliveryCanReceive,
  type GrnStatus,
} from "@/lib/purchasing/grn";
import { resolveGrnLineBatch } from "@/lib/purchasing/grn-batch";
import {
  buildInlineQcItemsFromCreatedGrn,
  resolveOverallQcStatus,
  submitGrnQcInspection,
} from "@/lib/purchasing/grn-qc";
import {
  grnCreatedMessage,
  normalizeQcOnItem,
  resolveInitialGrnStatus,
  validateReceiveLines,
  type PoItemForReceive,
} from "@/lib/purchasing/grn-receive-rules";
import type { CreateGrnInput, CreateGrnItemInput } from "@/lib/purchasing/grn-schemas";
import { parsePurchasingModuleType, type PurchasingModuleType } from "@/lib/purchasing/module-scope";
import { addSupplyStockFromGrn } from "@/lib/purchasing/supply-inventory";
import { toQty } from "@/lib/purchasing/utils";
import { syncReceiveRejectCredits } from "@/lib/purchasing/vendor-credit-service";

type BusinessScope = { company_id: string | null; branch_id: string | null } | null;

type GrnLine = CreateGrnItemInput & { qty_accepted?: number; qty_rejected?: number };

const WAREHOUSE_SCOPE_ERRORS = {
  not_found: "Gudang tidak ditemukan",
  inactive: "Gudang tidak aktif",
  branch_mismatch: "Gudang tidak sesuai cabang yang diizinkan untuk penerimaan ini",
} as const;

function todayIso(): string {
  return new Date().toISOString().split("T")[0];
}

/** Scope general: delivery dibuat otomatis dari PO (tanpa langkah delivery manual). */
async function createAutoDelivery(db: DbClient, poId: string, userId: string): Promise<string> {
  const poCanDeliver = await validatePOCanDelivery(db, poId);
  if (!poCanDeliver.valid) {
    throw ApiError.badRequest(poCanDeliver.errors.join(" ") || "Purchase order belum bisa diterima");
  }

  const { data: po, error: poError } = await db
    .from("purchase_orders")
    .select("id, vendor_id, company_id, branch_id, module_type")
    .eq("id", poId)
    .maybeSingle();

  if (poError || !po) throw ApiError.badRequest("Purchase order tidak ditemukan");
  if (po.module_type !== "general") {
    throw ApiError.badRequest("Penerimaan otomatis hanya untuk purchase order barang operasional");
  }

  const today = todayIso();
  const { data: delivery, error } = await db
    .from("deliveries")
    .insert({
      purchase_order_id: poId,
      supplier_id: null,
      vendor_id: po.vendor_id,
      tanggal_kirim: today,
      no_surat_jalan: `AUTO-${today}`,
      tanggal_estimasi_tiba: today,
      status: "pending",
      company_id: po.company_id ?? null,
      branch_id: po.branch_id ?? null,
      created_by: userId,
    })
    .select("id")
    .single();

  if (error || !delivery) {
    console.error("Auto delivery insert error:", error);
    throw ApiError.server("Gagal menyiapkan penerimaan barang operasional");
  }
  return delivery.id as string;
}

/**
 * EPIC-047 Fase 2 — module_type mengikuti PO induk delivery, bukan body.
 * Body tanpa module_type pada PO produk dulu menulis supplier_id DAN vendor_id
 * null (melanggar grn_party_check). Jalur auto-delivery sudah memastikan PO general.
 */
async function resolveModuleTypeFromPo(
  db: DbClient,
  poId: string,
  requested: CreateGrnInput["module_type"]
): Promise<PurchasingModuleType> {
  const { data: po, error } = await db
    .from("purchase_orders")
    .select("module_type")
    .eq("id", poId)
    .maybeSingle();

  if (error || !po) throw ApiError.badRequest("Purchase order tidak ditemukan untuk delivery ini");

  const poModuleType = parsePurchasingModuleType(po.module_type);
  if (requested && requested !== poModuleType) {
    throw ApiError.badRequest(`module_type tidak sesuai purchase order (${poModuleType})`);
  }
  return poModuleType;
}

/**
 * Scope bisnis mengikuti gudang penerimaan. Tanpa fallback kode toko yang
 * tertanam (EPIC-105 R3); rute ini dilayani Go, yang menolak gudang tanpa
 * toko/perusahaan aktif.
 */
async function resolveReceivingScope(warehouseId: string): Promise<BusinessScope> {
  const scope = await getApiUserScope();
  const businessScope = await resolveBusinessScopeFromWarehouse(warehouseId);

  const warehouseCheck = await validateWarehouseForReceivingScope(
    warehouseId,
    scope,
    businessScope?.branch_id ?? null
  );
  if ("error" in warehouseCheck) {
    throw ApiError.badRequest(WAREHOUSE_SCOPE_ERRORS[warehouseCheck.error]);
  }
  return businessScope;
}

/** Isi company/branch PO & delivery lama yang belum punya scope (non-fatal). */
async function backfillScope(
  db: DbClient,
  scope: NonNullable<BusinessScope>,
  poId: string,
  deliveryId: string
): Promise<void> {
  const values = { company_id: scope.company_id, branch_id: scope.branch_id };
  const { error: poError } = await db.from("purchase_orders").update(values).eq("id", poId);
  if (poError) console.error("Failed to backfill PO business scope:", poError);

  const { error: deliveryError } = await db.from("deliveries").update(values).eq("id", deliveryId);
  if (deliveryError) console.error("Failed to backfill delivery business scope:", deliveryError);
}

/** Batch & kedaluwarsa bahan baku: tanggal kosong → tanggal terima + shelf life. */
async function loadShelfLifeById(
  db: DbClient,
  lines: GrnLine[]
): Promise<Map<string, number | null>> {
  const ids = lines.map((line) => line.raw_material_id).filter((id): id is string => Boolean(id));
  if (ids.length === 0) return new Map();

  const { data } = await db.from("raw_materials").select("id, shelf_life_days").in("id", ids);
  const rows = (data || []) as { id: string; shelf_life_days: number | null }[];
  return new Map(rows.map((row) => [row.id, row.shelf_life_days]));
}

/**
 * EPIC-026 C1 — stok riil barang operasional (general + stockable=true);
 * item stockable=false di-expense. Non-fatal: gagal posting tidak membatalkan penerimaan.
 */
async function postSupplyStock(
  db: DbClient,
  params: {
    lines: GrnLine[];
    poItems: PoItemForReceive[];
    warehouseId: string;
    grnId: string;
    grnNumber: string;
    scope: BusinessScope;
    userId: string;
  }
): Promise<void> {
  const { lines, poItems } = params;
  try {
    const supplyIds = Array.from(
      new Set(
        lines
          .filter((line) => line.supply_item_id && toQty(line.qty_diterima) > 0)
          .map((line) => line.supply_item_id as string)
      )
    );
    if (supplyIds.length === 0) return;

    const { data: supplyRows } = await db.from("supply_items").select("id, stockable").in("id", supplyIds);
    const stockable = new Set(
      ((supplyRows ?? []) as { id: string; stockable?: boolean }[])
        .filter((row) => row.stockable === true)
        .map((row) => row.id)
    );

    for (const line of lines) {
      const supplyItemId = line.supply_item_id;
      const qty = toQty(line.qty_diterima);
      if (!supplyItemId || qty <= 0 || !stockable.has(supplyItemId)) continue;

      const poItem = poItems.find(
        (p) =>
          (line.purchase_order_item_id && p.id === line.purchase_order_item_id) ||
          p.supply_item_id === supplyItemId
      );

      await addSupplyStockFromGrn(db, {
        supplyItemId,
        warehouseId: params.warehouseId,
        qtyReceived: qty,
        unitCost: toQty(poItem?.harga_satuan),
        grnId: params.grnId,
        grnNumber: params.grnNumber,
        companyId: params.scope?.company_id ?? null,
        branchId: params.scope?.branch_id ?? null,
        userId: params.userId,
      });
    }
  } catch (stockErr) {
    console.error("[GRN] Supply stock posting error (non-fatal):", stockErr);
  }
}

/** RM/product: QC + posting stok langsung (GRN pending lama tetap lewat /grn/[id]/qc). */
async function finalizeInlineQc(
  db: DbClient,
  params: {
    grnId: string;
    createdItems: Parameters<typeof buildInlineQcItemsFromCreatedGrn>[0]["createdItems"];
    lines: GrnLine[];
    catatan: string | null;
    userId: string;
  }
): Promise<{ status: GrnStatus; accountingNote: string | null } | null> {
  const qcItems = buildInlineQcItemsFromCreatedGrn({
    createdItems: params.createdItems,
    requestItems: params.lines.map((line) => ({
      purchase_order_item_id: line.purchase_order_item_id,
      raw_material_id: line.raw_material_id,
      product_id: line.product_id,
      qty_diterima: line.qty_diterima,
      qty_accepted: line.qty_accepted,
      qty_rejected: line.qty_rejected,
      catatan: line.catatan,
    })),
  });
  if (qcItems.length === 0) return null;

  try {
    const result = await submitGrnQcInspection(db, {
      grnId: params.grnId,
      status: resolveOverallQcStatus(qcItems),
      catatan: params.catatan,
      items: qcItems,
      userId: params.userId,
    });
    return { status: result.grnStatus, accountingNote: result.accountingNote ?? null };
  } catch (qcErr) {
    console.error("[GRN] Inline QC finalize error:", qcErr);
    if (qcErr instanceof ApiError) throw qcErr;
    throw ApiError.server(
      qcErr instanceof Error ? qcErr.message : "Gagal menyelesaikan QC dan posting stok pada penerimaan"
    );
  }
}

export async function createGrn(
  db: DbClient,
  input: CreateGrnInput,
  user: ApiUser,
  meta: { ip: string | null; userAgent: string | null }
): Promise<{ grn: Record<string, unknown>; status: GrnStatus; message: string }> {
  const requestedModuleType = parsePurchasingModuleType(input.module_type);

  let deliveryId = input.delivery_id ?? null;
  const autoDelivery = !deliveryId;
  if (!deliveryId) {
    if (requestedModuleType !== "general" || !input.po_id) {
      throw ApiError.badRequest("Delivery wajib dipilih untuk penerimaan ini");
    }
    deliveryId = await createAutoDelivery(db, input.po_id, user.id);
  }

  const { valid, errors, delivery, items: fallbackPoItems } = await validateDeliveryCanReceive(
    db,
    deliveryId
  );
  if (!delivery?.purchase_order_id) {
    throw ApiError.badRequest(errors.join("; ") || "Delivery tidak valid untuk penerimaan barang");
  }
  const poId = delivery.purchase_order_id;

  const moduleType = autoDelivery
    ? requestedModuleType
    : await resolveModuleTypeFromPo(db, poId, input.module_type);
  const lines: GrnLine[] =
    moduleType === "general" ? input.items : input.items.map((item) => normalizeQcOnItem(item));
  // general/product menerima lewat vendor; raw_material lewat supplier.
  const usesVendor = moduleType !== "raw_material";

  const businessScope = await resolveReceivingScope(input.warehouse_id);
  if (businessScope) await backfillScope(db, businessScope, poId, deliveryId);

  const { data: freshPoItems, error: poItemsError } = await db
    .from("purchase_order_items")
    .select("id, raw_material_id, product_id, supply_item_id, pos_sku_id, qty_ordered, qty_received, harga_satuan")
    .eq("purchase_order_id", poId)
    .eq("is_active", true);

  if (poItemsError) {
    throw ApiError.badRequest(poItemsError.message || "Gagal memuat item PO untuk penerimaan");
  }
  const poItems = (freshPoItems || fallbackPoItems || []) as PoItemForReceive[];

  if (!valid && !errors.some((e) => e.includes("status"))) {
    throw ApiError.badRequest(errors.join("; "));
  }

  // EPIC-047 Fase 2 — pos_sku_id per baris diwarisi dari item PO yang dirujuk.
  const posSkuIdByIndex = validateReceiveLines(lines, poItems);

  const grnNumber = await generateGrnNumber(db);
  const totals = calculateGrnTotals(lines);

  // Penerimaan ke-N untuk delivery ini (N = GRN aktif yang sudah ada + 1).
  const { count: previousGrnCount, error: countError } = await db
    .from("grn")
    .select("*", { count: "exact", head: true })
    .eq("delivery_id", deliveryId)
    .eq("is_active", true);
  if (countError) console.error("Error counting GRNs:", countError);

  const initialStatus = resolveInitialGrnStatus(moduleType, totals);
  const receivedOn = input.tanggal_penerimaan || todayIso();

  const { data: grn, error: grnError } = await db
    .from("grn")
    .insert({
      nomor_grn: grnNumber,
      delivery_id: deliveryId,
      purchase_order_id: poId,
      supplier_id: usesVendor ? null : delivery.supplier_id,
      vendor_id: usesVendor ? delivery.vendor_id : null,
      company_id: businessScope?.company_id ?? null,
      branch_id: businessScope?.branch_id ?? null,
      tanggal_penerimaan: receivedOn,
      no_surat_jalan: delivery.no_surat_jalan,
      catatan: input.catatan || null,
      status: initialStatus,
      total_item_diterima: totals.total_diterima,
      total_item_ditolak: totals.total_ditolak,
      receive_count: (previousGrnCount || 0) + 1,
      penerima_id: user.id,
      created_by: user.id,
    })
    .select()
    .single();

  if (grnError) {
    console.error("GRN insert error:", grnError);
    throw ApiError.server(grnError.message || "Gagal menyimpan dokumen penerimaan barang");
  }

  const shelfLifeById = await loadShelfLifeById(db, lines);
  const grnItemsPayload = lines.map((line, index) => ({
    grn_id: grn.id,
    delivery_id: deliveryId,
    purchase_order_item_id: line.purchase_order_item_id,
    raw_material_id: line.raw_material_id || null,
    product_id: line.product_id || null,
    supply_item_id: line.supply_item_id || null,
    pos_sku_id: posSkuIdByIndex[index],
    qty_diterima: line.qty_diterima,
    qty_ditolak: line.qty_ditolak,
    satuan_id: line.satuan_id,
    kondisi: line.kondisi,
    catatan: line.catatan || null,
    warehouse_id: input.warehouse_id,
    qc_status: "pending",
    ...(line.raw_material_id
      ? resolveGrnLineBatch(line, shelfLifeById.get(line.raw_material_id), receivedOn)
      : {}),
  }));

  const { data: insertedGrnItems, error: itemsError } = await db
    .from("grn_items")
    .insert(grnItemsPayload)
    .select("id, purchase_order_item_id, raw_material_id, product_id, qty_diterima");

  if (itemsError) {
    console.error("GRN items insert error:", itemsError);
    throw ApiError.server(itemsError.message || "Gagal menyimpan item penerimaan barang");
  }

  // qty_received item PO dihitung ulang dari GRN/QC (updatePOStatusAfterGrn → recalculatePoReceivedQty).
  if (moduleType === "general" && initialStatus !== "rejected") {
    await postSupplyStock(db, {
      lines,
      poItems,
      warehouseId: input.warehouse_id,
      grnId: grn.id,
      grnNumber,
      scope: businessScope,
      userId: user.id,
    });
  }

  let status: GrnStatus = initialStatus;
  let accountingNote: string | null = null;
  if (moduleType !== "general" && initialStatus === "pending") {
    const qc = await finalizeInlineQc(db, {
      grnId: grn.id,
      createdItems: insertedGrnItems || [],
      lines,
      catatan: input.catatan || null,
      userId: user.id,
    });
    if (qc) {
      status = qc.status;
      accountingNote = qc.accountingNote;
    }
  }

  // Setelah QC inline, submitGrnQcInspection sudah memperbarui status delivery/PO.
  if (status === "pending" || moduleType === "general") {
    if (status !== "rejected") {
      await db
        .from("deliveries")
        .update({ status: "delivered", updated_at: new Date().toISOString() })
        .eq("id", deliveryId);
    } else {
      await updateDeliveryStatusAfterGrn(db, deliveryId, status);
    }
    await updatePOStatusAfterGrn(db, poId);
  } else if (status === "rejected" && initialStatus === "rejected") {
    // Ditolak semua di pintu (tanpa QC).
    await updateDeliveryStatusAfterGrn(db, deliveryId, status);
    await updatePOStatusAfterGrn(db, poId);
  }

  try {
    await syncReceiveRejectCredits(db, grn.id, user.id);
  } catch (creditErr) {
    console.error("[GRN] Vendor credit sync error (non-fatal):", creditErr);
  }

  // General: jurnal di sini (RM/product sudah lewat submitGrnQcInspection).
  if (moduleType === "general" && status !== "rejected") {
    const { postGrnAccountingJournals } = await import("@/lib/purchasing/accounting-posting");
    const accounting = await postGrnAccountingJournals({ db, grnId: grn.id, userId: user.id });
    accountingNote = accounting.note;
  }

  await recordAuditAfterCommit({
    actor: { id: user.id, name: user.full_name },
    action: "grn.post",
    entity: "grn",
    entityId: grn.id,
    entityLabel: grnNumber,
    after: {
      status,
      warehouse_id: input.warehouse_id,
      purchase_order_id: poId,
      items: grnItemsPayload.map((item) => ({
        raw_material_id: item.raw_material_id,
        product_id: item.product_id,
        supply_item_id: item.supply_item_id,
        qty_diterima: item.qty_diterima,
        qty_ditolak: item.qty_ditolak,
        batch_number: "batch_number" in item ? item.batch_number : null,
        expiry_date: "expiry_date" in item ? item.expiry_date : null,
      })),
    },
    ...meta,
  });

  return {
    grn,
    status,
    message: grnCreatedMessage({ grnNumber, status, moduleType, accountingNote }),
  };
}

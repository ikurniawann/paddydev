// EPIC-047 Fase 2 retry — module_type pada POST /api/purchasing/grn harus
// mengikuti PO induk delivery (bukan body request). Body yang tidak mengirim
// module_type (default raw_material) pada GRN product-PO sebelumnya menulis
// supplier_id DAN vendor_id null sekaligus, melanggar constraint
// grn_party_check. Lihat src/app/api/purchasing/grn/route.ts.
import { describe, expect, it, vi, beforeEach } from "vitest";
import type { NextRequest } from "next/server";

// ---- Auth: bypass IAM lookup (route calls requireIamMenuPrefix directly,
// unlike POST /api/purchasing/po which only reads scope). Keep ApiError /
// createdResponse / paginatedResponse real — pure helpers, no I/O. ---------
vi.mock("@/lib/api/auth", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/auth")>();
  return {
    ...actual,
    requireIamMenuPrefix: vi.fn(async () => ({
      id: "user-1",
      full_name: "Test User",
      role: "admin",
      brand_id: null,
    })),
  };
});

vi.mock("@/lib/api/scope", () => ({
  getApiUserScope: vi.fn(async () => ({
    userId: "user-1",
    role: "admin",
    businessScope: "branch",
    holdingId: "holding-1",
    companyId: "company-1",
    branchId: "branch-1",
    isUnscoped: false,
  })),
  companyScopeOr: vi.fn(() => null),
  branchScopeOr: vi.fn(() => null),
  // null on both → route skips the PO/delivery business-scope backfill block,
  // keeping the db.from(...) call sequence short and predictable below.
  resolveBusinessScopeFromWarehouse: vi.fn(async () => null),
  resolveBusinessScopeByCodes: vi.fn(async () => null),
  validateWarehouseForReceivingScope: vi.fn(async () => ({ ok: true })),
}));

// ---- Delivery/GRN state-machine helpers — mocked so the test only exercises
// the module_type resolution + party-column logic, not their own DB I/O. ---
let deliveryResultRef: {
  valid: boolean;
  errors: string[];
  delivery: {
    id: string;
    purchase_order_id: string;
    supplier_id: string | null;
    vendor_id: string | null;
    no_surat_jalan?: string | null;
    status?: string | null;
  };
  items: unknown[];
};

vi.mock("@/lib/purchasing/grn", () => ({
  generateGrnNumber: vi.fn(async () => "GRN-20260911-0001"),
  validateDeliveryCanReceive: vi.fn(async () => deliveryResultRef),
  calculateGrnTotals: vi.fn((items: { qty_diterima: number; qty_ditolak: number }[]) =>
    items.reduce(
      (acc, item) => ({
        total_diterima: acc.total_diterima + (item.qty_diterima || 0),
        total_ditolak: acc.total_ditolak + (item.qty_ditolak || 0),
      }),
      { total_diterima: 0, total_ditolak: 0 }
    )
  ),
  updateDeliveryStatusAfterGrn: vi.fn(async () => {}),
  updatePOStatusAfterGrn: vi.fn(async () => {}),
}));

vi.mock("@/lib/purchasing/supply-inventory", () => ({
  addSupplyStockFromGrn: vi.fn(async () => {}),
}));

vi.mock("@/lib/purchasing/vendor-credit-service", () => ({
  syncReceiveRejectCredits: vi.fn(async () => {}),
}));

vi.mock("@/lib/purchasing/grn-qc", () => ({
  buildInlineQcItemsFromCreatedGrn: vi.fn(({ createdItems }) => createdItems),
  resolveOverallQcStatus: vi.fn(() => "accepted"),
  submitGrnQcInspection: vi.fn(async () => ({
    inspectionId: "qc-1",
    grnStatus: "received",
    totalAccepted: 0,
    totalRejected: 0,
    accountingNote: null,
  })),
}));

// module-scope (parsePurchasingModuleType) and variant-po-lines
// (resolveGrnItemSku) are pure/no-I/O — kept real.

// ---- Fake query-builder db (FIFO respons per tabel, meniru urutan
// pemanggilan db.from(table) persis seperti di route). Sama gaya dengan
// src/app/api/purchasing/po/route.test.ts, ditambah dukungan `count`. ------
type FakeResult = { data: unknown; error: unknown; count?: number };

function createFakeDb(responses: Record<string, FakeResult[]>) {
  const calls: Array<{ table: string; action: string; payload?: unknown }> = [];

  function nextResponse(table: string): FakeResult {
    const queue = responses[table];
    if (!queue || queue.length === 0) {
      throw new Error(`Tidak ada mock response tersisa untuk tabel "${table}"`);
    }
    return queue.shift()!;
  }

  function builder(table: string) {
    const state: { action: string; payload?: unknown } = { action: "select" };
    const b: Record<string, unknown> = {
      select: () => b,
      eq: () => b,
      in: () => b,
      or: () => b,
      is: () => b,
      neq: () => b,
      ilike: () => b,
      order: () => b,
      limit: () => b,
      range: () => b,
      single: () => b,
      maybeSingle: () => b,
      insert: (payload: unknown) => {
        state.action = "insert";
        state.payload = payload;
        return b;
      },
      update: (payload: unknown) => {
        state.action = "update";
        state.payload = payload;
        return b;
      },
      then: (resolve: (v: FakeResult) => unknown, reject: (e: unknown) => unknown) => {
        calls.push({ table, action: state.action, payload: state.payload });
        try {
          const result = nextResponse(table);
          return Promise.resolve(result).then(resolve, reject);
        } catch (err) {
          return Promise.reject(err).catch(reject);
        }
      },
    };
    return b;
  }

  const db = { from: vi.fn((table: string) => builder(table)) };
  return { db, calls };
}

vi.mock("@/lib/pg/create-client", () => ({
  createServerPgClient: vi.fn(async () => fakeDbRef.db),
  createPgClient: vi.fn(() => fakeDbRef.db),
}));

let fakeDbRef: ReturnType<typeof createFakeDb>;

function makeRequest(body: unknown): NextRequest {
  return { json: async () => body } as unknown as NextRequest;
}

const PO_ID = "11111111-1111-4111-8111-111111111111";
const DELIVERY_ID = "22222222-2222-4222-8222-222222222222";
const PO_ITEM_ID = "33333333-3333-4333-8333-333333333333";
const PRODUCT_ID = "44444444-4444-4444-8444-444444444444";
const RM_PO_ID = "55555555-5555-4555-8555-555555555555";
const RM_DELIVERY_ID = "66666666-6666-4666-8666-666666666666";
const RM_PO_ITEM_ID = "77777777-7777-4777-8777-777777777777";
const RAW_MATERIAL_ID = "88888888-8888-4888-8888-888888888888";
const VENDOR_ID = "99999999-9999-4999-8999-999999999999";
const SUPPLIER_ID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";
const WAREHOUSE_ID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb";

const productDelivery = {
  id: DELIVERY_ID,
  purchase_order_id: PO_ID,
  supplier_id: null,
  vendor_id: VENDOR_ID,
  no_surat_jalan: "SJ-001",
  status: "pending",
};

const rmDelivery = {
  id: RM_DELIVERY_ID,
  purchase_order_id: RM_PO_ID,
  supplier_id: SUPPLIER_ID,
  vendor_id: null,
  no_surat_jalan: "SJ-002",
  status: "pending",
};

describe("POST /api/purchasing/grn — module_type follows the parent PO (EPIC-047 retry)", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  it("product PO + delivery, body WITHOUT module_type → vendor_id set, supplier_id null (201)", async () => {
    deliveryResultRef = {
      valid: true,
      errors: [],
      delivery: productDelivery,
      items: [],
    };

    fakeDbRef = createFakeDb({
      purchase_orders: [{ data: { module_type: "product" }, error: null }],
      purchase_order_items: [
        {
          data: [
            {
              id: PO_ITEM_ID,
              raw_material_id: null,
              product_id: PRODUCT_ID,
              supply_item_id: null,
              pos_sku_id: null,
              qty_ordered: 4,
              qty_received: 0,
              harga_satuan: 1000,
            },
          ],
          error: null,
        },
      ],
      grn: [
        { data: null, error: null, count: 0 }, // previousGrnCount
        { data: { id: "grn-1", nomor_grn: "GRN-20260911-0001" }, error: null }, // insert
      ],
      grn_items: [
        {
          data: [
            {
              id: "grn-item-1",
              purchase_order_item_id: PO_ITEM_ID,
              raw_material_id: null,
              product_id: PRODUCT_ID,
              qty_diterima: 4,
            },
          ],
          error: null,
        },
      ],
    });

    const { POST } = await import("./route");
    const res = await POST(
      makeRequest({
        delivery_id: DELIVERY_ID,
        warehouse_id: WAREHOUSE_ID,
        items: [
          {
            purchase_order_item_id: PO_ITEM_ID,
            product_id: PRODUCT_ID,
            qty_diterima: 4,
            qty_ditolak: 0,
            qty_accepted: 4,
            qty_rejected: 0,
            kondisi: "baik",
          },
        ],
      })
    );

    expect(res.status).toBe(201);

    const grnInsertCall = fakeDbRef.calls.find(
      (c) => c.table === "grn" && c.action === "insert"
    );
    expect(grnInsertCall).toBeDefined();
    const insertedGrn = grnInsertCall!.payload as {
      supplier_id: string | null;
      vendor_id: string | null;
    };
    expect(insertedGrn.vendor_id).toBe(VENDOR_ID);
    expect(insertedGrn.supplier_id).toBeNull();
  });

  it("no hard-coded store fallback: MIZU / MIZU-BANDUNG is never looked up (EPIC-105 R3)", async () => {
    const scopeMod = await import("@/lib/api/scope");
    vi.mocked(scopeMod.resolveBusinessScopeByCodes).mockClear();
    vi.mocked(scopeMod.resolveBusinessScopeFromWarehouse).mockClear();
    deliveryResultRef = { valid: true, errors: [], delivery: productDelivery, items: [] };
    fakeDbRef = createFakeDb({ purchase_orders: [{ data: { module_type: "product" }, error: null }] });
    const { POST } = await import("./route");
    await POST(
      makeRequest({
        delivery_id: DELIVERY_ID,
        warehouse_id: WAREHOUSE_ID,
        items: [{ purchase_order_item_id: PO_ITEM_ID, product_id: PRODUCT_ID, qty_diterima: 1, qty_ditolak: 0, kondisi: "baik" }],
      })
    );
    expect(vi.mocked(scopeMod.resolveBusinessScopeFromWarehouse)).toHaveBeenCalled();
    expect(vi.mocked(scopeMod.resolveBusinessScopeByCodes)).not.toHaveBeenCalled();
  });

  it("product PO, body module_type: raw_material → 400 tidak sesuai purchase order, no grn insert", async () => {
    deliveryResultRef = {
      valid: true,
      errors: [],
      delivery: productDelivery,
      items: [],
    };

    fakeDbRef = createFakeDb({
      purchase_orders: [{ data: { module_type: "product" }, error: null }],
    });

    const { POST } = await import("./route");
    const res = await POST(
      makeRequest({
        delivery_id: DELIVERY_ID,
        warehouse_id: WAREHOUSE_ID,
        module_type: "raw_material",
        items: [
          {
            purchase_order_item_id: PO_ITEM_ID,
            product_id: PRODUCT_ID,
            qty_diterima: 4,
            qty_ditolak: 0,
            qty_accepted: 4,
            qty_rejected: 0,
            kondisi: "baik",
          },
        ],
      })
    );

    expect(res.status).toBe(400);
    const json = await res.json();
    expect(json.error).toContain("tidak sesuai purchase order");
    expect(fakeDbRef.calls.some((c) => c.table === "grn" && c.action === "insert")).toBe(false);
  });

  it("raw-material PO, body omitted module_type → supplier_id set, vendor_id null (unchanged, 201)", async () => {
    deliveryResultRef = {
      valid: true,
      errors: [],
      delivery: rmDelivery,
      items: [],
    };

    fakeDbRef = createFakeDb({
      purchase_orders: [{ data: { module_type: "raw_material" }, error: null }],
      raw_materials: [{ data: [{ id: RAW_MATERIAL_ID, shelf_life_days: 10 }], error: null }],
      purchase_order_items: [
        {
          data: [
            {
              id: RM_PO_ITEM_ID,
              raw_material_id: RAW_MATERIAL_ID,
              product_id: null,
              supply_item_id: null,
              pos_sku_id: null,
              qty_ordered: 2,
              qty_received: 0,
              harga_satuan: 5000,
            },
          ],
          error: null,
        },
      ],
      grn: [
        { data: null, error: null, count: 0 },
        { data: { id: "grn-2", nomor_grn: "GRN-20260911-0002" }, error: null },
      ],
      grn_items: [
        {
          data: [
            {
              id: "grn-item-2",
              purchase_order_item_id: RM_PO_ITEM_ID,
              raw_material_id: RAW_MATERIAL_ID,
              product_id: null,
              qty_diterima: 2,
            },
          ],
          error: null,
        },
      ],
    });

    const { POST } = await import("./route");
    const res = await POST(
      makeRequest({
        delivery_id: RM_DELIVERY_ID,
        warehouse_id: WAREHOUSE_ID,
        tanggal_penerimaan: "2026-10-03",
        items: [
          {
            purchase_order_item_id: RM_PO_ITEM_ID,
            raw_material_id: RAW_MATERIAL_ID,
            qty_diterima: 2,
            qty_ditolak: 0,
            qty_accepted: 2,
            qty_rejected: 0,
            kondisi: "baik",
            batch_number: " LOT-77 ",
          },
        ],
      })
    );

    expect(res.status).toBe(201);

    const grnInsertCall = fakeDbRef.calls.find(
      (c) => c.table === "grn" && c.action === "insert"
    );
    expect(grnInsertCall).toBeDefined();
    const insertedGrn = grnInsertCall!.payload as {
      supplier_id: string | null;
      vendor_id: string | null;
    };
    expect(insertedGrn.supplier_id).toBe(SUPPLIER_ID);
    expect(insertedGrn.vendor_id).toBeNull();

    // Batch dari supplier disimpan; tanggal kosong → tanggal terima + shelf life.
    const itemsInsert = fakeDbRef.calls.find((c) => c.table === "grn_items" && c.action === "insert");
    expect(itemsInsert!.payload).toEqual([
      expect.objectContaining({ batch_number: "LOT-77", expiry_date: "2026-10-13" }),
    ]);
  });
});

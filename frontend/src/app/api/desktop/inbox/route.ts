import { NextResponse } from "next/server";
import { ApiError, requireApiUser } from "@/lib/api/auth";
import { getPool } from "@/lib/db";
import { loadGrantedMenuCodesForUser } from "@/lib/iam/has-menu";
import {
  INBOX_ITEM_LIMIT,
  INBOX_SECTION_LABEL,
  allowedInboxSections,
  leaveInboxItem,
  type InboxSection,
} from "@/lib/desktop/inbox";

/**
 * Isi kartu "Perlu Keputusan" — daftar nyata yang bisa ditindaklanjuti.
 * Seksi dibatasi menu IAM: HR tidak melihat PO, gudang tidak melihat cuti.
 */
export const dynamic = "force-dynamic";

async function loadLeaves(): Promise<InboxSection> {
  const { rows } = await getPool().query(
    `SELECT l.id::text, l.leave_type, l.start_date, l.end_date, l.total_days,
            e.full_name,
            count(*) OVER ()::int AS total
       FROM hris.leaves l
       LEFT JOIN hris.employees e ON e.id = l.employee_id
      WHERE l.status = 'pending'
      ORDER BY l.start_date
      LIMIT ${INBOX_ITEM_LIMIT}`
  );
  return {
    key: "cuti",
    label: INBOX_SECTION_LABEL.cuti,
    total: Number(rows[0]?.total ?? 0),
    items: rows.map(leaveInboxItem),
  };
}

async function loadPurchaseOrders(): Promise<InboxSection> {
  const { rows } = await getPool().query(
    `SELECT po.id::text, po.nomor_po, po.total, s.nama_supplier AS supplier,
            count(*) OVER ()::int AS total_count
       FROM purchasing.purchase_orders po
       LEFT JOIN purchasing.suppliers s ON s.id = po.supplier_id
      WHERE po.status = 'draft'
      ORDER BY po.created_at DESC
      LIMIT ${INBOX_ITEM_LIMIT}`
  );
  return {
    key: "po",
    label: INBOX_SECTION_LABEL.po,
    total: Number(rows[0]?.total_count ?? 0),
    items: rows.map((row) => ({
      id: String(row.id),
      title: String(row.nomor_po ?? "PO"),
      subtitle: `${row.supplier ?? "Supplier"} · Rp ${Number(row.total ?? 0).toLocaleString("id-ID")}`,
      href: "/dashboard/items/raw-material/approval/po",
    })),
  };
}

async function loadLowStock(): Promise<InboxSection> {
  const { rows } = await getPool().query(
    `SELECT rm.id::text, rm.nama, rm.kode,
            i.qty_available, i.qty_minimum,
            count(*) OVER ()::int AS total_count
       FROM inventory.inventory i
       JOIN item.raw_materials rm ON rm.id = i.raw_material_id
      WHERE i.is_active AND rm.deleted_at IS NULL
        AND i.qty_available <= i.qty_minimum
      ORDER BY (i.qty_minimum - i.qty_available) DESC
      LIMIT ${INBOX_ITEM_LIMIT}`
  );
  return {
    key: "stok",
    label: INBOX_SECTION_LABEL.stok,
    total: Number(rows[0]?.total_count ?? 0),
    items: rows.map((row) => ({
      id: String(row.id),
      title: String(row.nama ?? "-"),
      subtitle: `sisa ${Number(row.qty_available ?? 0)} dari minimum ${Number(row.qty_minimum ?? 0)}`,
      href: "/dashboard/items/raw-material/purchasing/pr",
    })),
  };
}

export async function GET() {
  try {
    const user = await requireApiUser();
    const granted = await loadGrantedMenuCodesForUser(user.id, user.role);
    const keys = allowedInboxSections(user.role, granted);

    const loaders: Record<string, () => Promise<InboxSection>> = {
      cuti: loadLeaves,
      po: loadPurchaseOrders,
      stok: loadLowStock,
    };

    const sections = await Promise.all(
      keys.map(async (key) => {
        try {
          return await loaders[key]();
        } catch (error) {
          // Satu modul bermasalah tidak boleh mengosongkan seluruh kartu.
          console.warn(`[desktop/inbox] seksi ${key} gagal:`, error);
          return null;
        }
      })
    );

    return NextResponse.json({
      success: true,
      data: { sections: sections.filter((s): s is InboxSection => s !== null && s.total > 0) },
    });
  } catch (error) {
    if (error instanceof ApiError) return error.toResponse();
    console.error("[desktop/inbox] gagal:", error);
    return NextResponse.json({ success: false, error: "Gagal memuat daftar keputusan" }, { status: 500 });
  }
}

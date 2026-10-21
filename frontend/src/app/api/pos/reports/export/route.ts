import { NextRequest, NextResponse } from "next/server";
import { stallLabel } from "@/lib/auth/stall-label";
import { getPosSession } from "@/lib/api/auth";
import { getApiUserScope } from "@/lib/api/scope";
import { resolveBrandName } from "@/lib/branding-server";
import { REPORT_EXPORTS, buildReportWorkbook, isReportExportKey } from "@/lib/pos/report-excel/builders";
import { workbookBuffer } from "@/lib/pos/report-excel/workbook";
import { validateReportRange } from "@/lib/pos/report-period";

/**
 * GET /api/pos/reports/export?report=<key>&date_from&date_to[&warehouse_id]
 * Unduh laporan POS sebagai Excel rapi (Desktop → Drive → Reports, owner
 * 2026-09-05). Data diambil dengan memanggil handler API laporan terkait
 * secara internal (filter & hak akses persis sama dengan halaman laporan),
 * lalu dibentuk jadi workbook exceljs (judul, header gelap, format Rupiah,
 * baris total, beberapa sheet).
 */
const HANDLERS: Record<string, () => Promise<{ GET: (req: NextRequest) => Promise<Response> }>> = {
  profit: () => import("@/app/api/pos/reports/profit/route"),
  "revenue-composition": () => import("@/app/api/pos/reports/revenue-composition/route"),
  transactions: () => import("@/app/api/pos/reports/transactions/route"),
  "rush-hour": () => import("@/app/api/pos/reports/rush-hour/route"),
  voids: () => import("@/app/api/pos/reports/voids/route"),
  "product-sales": () => import("@/app/api/pos/reports/product-sales/route"),
  "payment-methods": () => import("@/app/api/pos/reports/payment-methods/route"),
};

export async function GET(request: NextRequest) {
  const sessionUserId = await getPosSession();
  if (!sessionUserId) {
    return NextResponse.json({ success: false, error: "Authentication required" }, { status: 401 });
  }
  const sp = request.nextUrl.searchParams;
  const key = String(sp.get("report") || "");
  if (!isReportExportKey(key)) {
    return NextResponse.json({ success: false, error: "Laporan tidak dikenal" }, { status: 400 });
  }
  const dateFrom = String(sp.get("date_from") || "");
  const dateTo = String(sp.get("date_to") || "");
  const invalid = validateReportRange(dateFrom, dateTo);
  if (invalid) return NextResponse.json({ success: false, error: invalid }, { status: 400 });

  try {
    const url = new URL(request.url);
    url.pathname = `/api/pos/reports/${key}`;
    url.search = "";
    url.searchParams.set("date_from", dateFrom);
    url.searchParams.set("date_to", dateTo);
    const warehouseId = sp.get("warehouse_id");
    if (warehouseId) url.searchParams.set("warehouse_id", warehouseId);
    if (key === "payment-methods") url.searchParams.set("granularity", "day");

    const mod = await HANDLERS[key]();
    const res = await mod.GET(new NextRequest(url, { headers: request.headers }));
    const json = (await res.json().catch(() => null)) as { success?: boolean; data?: unknown; error?: string } | null;
    if (!res.ok || !json?.success || !json.data) {
      return NextResponse.json({ success: false, error: json?.error || "Gagal memuat data laporan" }, { status: res.ok ? 500 : res.status });
    }
    const data = json.data as { filters?: { warehouse_id?: string | null }; stall_options?: Array<{ id: string; name: string; label?: string }> };
    const selectedStallLabel = data.filters?.warehouse_id
      ? (() => {
          const stall = data.stall_options?.find((s) => s.id === data.filters?.warehouse_id);
          return stall ? stallLabel(stall) : "Outlet terpilih";
        })()
      : "Semua outlet";

    const scope = await getApiUserScope();
    const companyName = await resolveBrandName(scope?.companyId);
    const wb = await buildReportWorkbook(key, json.data, {
      companyName, stallLabel: selectedStallLabel, generatedAt: new Date(),
    });
    const buffer = await workbookBuffer(wb);
    const filename = `${REPORT_EXPORTS[key].label.toLowerCase().replace(/\s+/g, "-")}_${dateFrom}_${dateTo}.xlsx`;
    return new NextResponse(new Uint8Array(buffer), {
      headers: {
        "Content-Type": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
        "Content-Disposition": `attachment; filename="${filename}"`,
        "Cache-Control": "no-store",
      },
    });
  } catch (error) {
    console.error(`[pos] export ${key}:`, error);
    return NextResponse.json({ success: false, error: "Gagal membuat file Excel" }, { status: 500 });
  }
}

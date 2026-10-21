import { NextRequest, NextResponse } from "next/server";
import { getPosSession } from "@/lib/api/auth";
import { getApiUserScope } from "@/lib/api/scope";
import { resolveBrandName } from "@/lib/branding-server";
import {
  parseReportDateRange,
  resolveReportStallFilter,
} from "@/lib/pos/report-stall-filter";
import { buildRushHourReport } from "@/lib/pos/rush-hour";
import { fetchRushHourPoints } from "@/lib/pos/rush-hour-query";
import { buildRushHourXlsx } from "@/lib/pos/rush-hour-xlsx";

/**
 * GET /api/pos/reports/rush-hour/export — unduh report Rush Hour sebagai
 * Excel rapi (4 sheet: Ringkasan, Per Jam, Per Hari, Heatmap). Filter
 * sama persis dengan halaman report; range_from/range_to (jam 0–23)
 * menentukan rentang jam yang dihitung kontribusinya di sheet Ringkasan.
 */
export async function GET(request: NextRequest) {
  const sessionUserId = await getPosSession();
  if (!sessionUserId) {
    return NextResponse.json({ success: false, error: "Authentication required" }, { status: 401 });
  }

  try {
    const scope = await getApiUserScope();
    const searchParams = request.nextUrl.searchParams;
    const range = parseReportDateRange(
      searchParams.get("date_from"),
      searchParams.get("date_to")
    );
    const stallFilter = await resolveReportStallFilter(
      scope,
      searchParams.get("warehouse_id")
    );

    const clampHour = (value: string | null, fallback: number) => {
      const n = Number(value);
      return Number.isInteger(n) && n >= 0 && n <= 23 ? n : fallback;
    };
    const rangeFrom = clampHour(searchParams.get("range_from"), 7);
    const rangeTo = Math.max(rangeFrom, clampHour(searchParams.get("range_to"), 12));

    const points =
      stallFilter.warehouseIds && stallFilter.warehouseIds.length === 0
        ? []
        : await fetchRushHourPoints({
            startIso: range.startIso,
            endIso: range.endIso,
            warehouseIds: stallFilter.warehouseIds,
          });
    const report = buildRushHourReport(points);

    const stallLabel = stallFilter.selectedWarehouseId
      ? (stallFilter.stallOptions.find((s) => s.id === stallFilter.selectedWarehouseId)?.label ??
        "Outlet terpilih")
      : "Semua outlet";

    const buffer = await buildRushHourXlsx(report, {
      companyName: await resolveBrandName(scope?.companyId),
      periodLabel: `${range.dateFrom} s.d. ${range.dateTo}`,
      stallLabel,
      rangeFrom,
      rangeTo,
      generatedAt: new Date(),
    });

    const stamp = `${range.dateFrom}_${range.dateTo}`;
    return new NextResponse(new Uint8Array(buffer), {
      headers: {
        "Content-Type":
          "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
        "Content-Disposition": `attachment; filename="rush-hour-${stamp}.xlsx"`,
      },
    });
  } catch (error) {
    const message = error instanceof Error ? error.message : "Gagal export rush hour";
    const known = /tanggal|stall|outlet/i.test(message);
    console.error("[pos] rush-hour export:", error);
    return NextResponse.json(
      { success: false, error: known ? message : "Gagal export rush hour" },
      { status: known ? 400 : 500 }
    );
  }
}

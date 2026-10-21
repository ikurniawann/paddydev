import type { ProductSalesReport } from "@/lib/pos/reports/types";
import { stallLabel } from "@/lib/auth/stall-label";
import { formatReportStallLabel } from "@/lib/pos/reports/transaction-labels";

export type PosReportSheet = { name: string; rows: Array<Array<string | number>> };

export function productSalesExportFileName(filters: {
  date_from: string;
  date_to: string;
}) {
  return `penjualan-produk-${filters.date_from}_${filters.date_to}.xlsx`;
}

function stallFilterLabel(report: ProductSalesReport) {
  const id = report.filters.warehouse_id;
  if (!id) return "Semua outlet";
  const stall = report.stall_options.find((row) => row.id === id);
  return stall ? `${stallLabel(stall)} (${stall.code})` : id;
}

export function buildProductSalesExportSheets(
  report: ProductSalesReport
): PosReportSheet[] {
  return [
    {
      name: "Ringkasan",
      rows: [
        ["Laporan Penjualan Produk"],
        ["Periode", `${report.filters.date_from} s/d ${report.filters.date_to}`],
        ["Outlet", stallFilterLabel(report)],
        [],
        ["Metrik", "Nilai"],
        ["Produk terjual", report.summary.products],
        ["Total qty", report.summary.quantity],
        ["Total omzet", report.summary.revenue],
      ],
    },
    {
      name: "Produk",
      rows: [
        ["Produk", "SKU", "Outlet", "Qty", "Orders", "Omzet"],
        ...report.rows.map((row) => [
          row.product_name,
          row.product_sku || "—",
          formatReportStallLabel(row),
          row.quantity,
          row.order_count,
          row.revenue,
        ]),
      ],
    },
  ];
}

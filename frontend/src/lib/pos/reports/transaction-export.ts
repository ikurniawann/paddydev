import type { TransactionReport, TransactionReportRow } from "@/lib/pos/reports/types";
import { stallLabel } from "@/lib/auth/stall-label";
import {
  formatCompTypeLabel,
  formatPaymentMethodLabel,
  formatPaymentStatusLabel,
  formatReportStallLabel,
  formatSoldFromLabel,
} from "@/lib/pos/reports/transaction-labels";

export type PosReportSheet = { name: string; rows: Array<Array<string | number>> };

export function transactionExportFileName(filters: {
  date_from: string;
  date_to: string;
}) {
  return `transaksi-pos-${filters.date_from}_${filters.date_to}.xlsx`;
}

function stallFilterLabel(report: TransactionReport) {
  const id = report.filters.warehouse_id;
  if (!id) return "Semua outlet";
  const stall = report.stall_options.find((row) => row.id === id);
  return stall ? `${stallLabel(stall)} (${stall.code})` : id;
}

function methodLabel(row: TransactionReportRow) {
  const comp = formatCompTypeLabel(row.comp_type);
  if (comp) return comp;
  return formatPaymentMethodLabel(row.payment_method, {
    code: row.payment_method_code,
    name: row.payment_method_name,
  });
}

export function buildTransactionExportSheets(
  report: TransactionReport
): PosReportSheet[] {
  return [
    {
      name: "Ringkasan",
      rows: [
        ["Laporan Transaksi POS"],
        ["Periode", `${report.filters.date_from} s/d ${report.filters.date_to}`],
        ["Outlet", stallFilterLabel(report)],
        [],
        ["Metrik", "Nilai"],
        ["Transaksi", report.summary.transactions],
        ["Revenue (kotor)", report.summary.revenue],
        ["Diskon", report.summary.discount],
        ["Pajak", report.summary.tax],
        ["Service", report.summary.service],
        ["Nett", report.summary.nett],
        ["ARK terpakai", report.summary.total_ark_used],
      ],
    },
    {
      name: "Per Outlet",
      rows: [
        ["Outlet", "Transaksi", "Item Terjual", "Penjualan"],
        ...report.per_stall.map((row) => [
          formatReportStallLabel(row),
          row.transactions,
          row.quantity,
          row.sales,
        ]),
      ],
    },
    {
      name: "Top Produk",
      rows: [
        ["Produk", "Qty", "Omzet"],
        ...report.top_products.map((row) => [
          row.product_name,
          row.quantity,
          row.revenue,
        ]),
      ],
    },
    {
      name: "Harian",
      rows: [
        ["Tanggal", "Transaksi", "Nett"],
        ...report.daily.map((row) => [row.date, row.transactions, row.nett]),
      ],
    },
    {
      name: "Transaksi",
      rows: [
        [
          "Order",
          "Checkout",
          "Waktu",
          "Outlet",
          "Metode",
          "Subtotal",
          "Diskon",
          "Pajak",
          "Total",
          "Service",
          "ARK",
          "Pembayaran",
          "Asal",
          "Comp",
        ],
        ...report.rows.map((row) => [
          row.order_number || row.id,
          row.checkout_number || "",
          row.ordered_at || "",
          formatReportStallLabel(row),
          methodLabel(row),
          row.subtotal,
          row.discount_amount,
          row.tax_amount,
          row.total_amount,
          row.service_charge_amount,
          row.ark_coins_used,
          formatPaymentStatusLabel(row.payment_status, row.status),
          formatSoldFromLabel(row.sold_from),
          formatCompTypeLabel(row.comp_type),
        ]),
      ],
    },
  ];
}

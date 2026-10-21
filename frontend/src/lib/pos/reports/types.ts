export interface ProfitBucket {
  id: string;
  label: string;
  quantity: number;
  revenue: number;
  cogs: number;
  gross_profit: number;
  gross_margin_pct: number;
}

export interface ProfitReport {
  filters: {
    date_from: string;
    date_to: string;
  };
  summary: {
    orders: number;
    items: number;
    quantity: number;
    revenue: number;
    cogs: number;
    gross_profit: number;
    gross_margin_pct: number;
    zero_cost_items: number;
  };
  breakdowns: {
    products: ProfitBucket[];
    categories: ProfitBucket[];
    stations: ProfitBucket[];
    cashiers: ProfitBucket[];
    dates: ProfitBucket[];
  };
}

export interface ProfitReportParams {
  date_from: string;
  date_to: string;
}

export interface ClosingReportParams {
  date: string;
  shift_id?: string;
}

export interface ClosingCategoryRow {
  name: string;
  amount: number;
  percentage: number;
}

export interface ClosingPromoRow {
  name: string;
  qty: number;
}

export interface ClosingTargetRow {
  target: number;
  actual: number;
  variance: number;
}

export interface ClosingReport {
  filters: {
    date: string;
    shift_id: string | null;
  };
  header: {
    title: string;
    outlet_line: string;
    report_date: string;
  };
  shift_sessions: Array<{
    label: string;
    last_order: string;
    closed_at: string;
  }>;
  sales_summary: {
    net_sales: number;
    service: number;
    tax: number;
    discount: number;
    gross: number;
  };
  transaction_totals: {
    transactions: number;
    sales: number;
    discount: number;
    full_discount_transactions: number;
    full_discount_amount: number;
  };
  guests: {
    count: number;
    average_per_pax: number;
  };
  categories_by_segment: Array<{
    segment: string;
    title: string;
    rows: ClosingCategoryRow[];
  }>;
  targets: {
    daily: ClosingTargetRow;
    monthly: ClosingTargetRow;
    month_to_date: ClosingTargetRow;
  };
  promos_by_segment: Array<{
    segment: string;
    title: string;
    rows: ClosingPromoRow[];
  }>;
  footer: {
    printed_by: string;
  };
}

export interface ReportStallOption {
  id: string;
  code: string;
  name: string;
  branch_id?: string;
  branch_name?: string;
  /** "<nama> — <branch>" bila picker melintasi branch (tampilkan lewat stallLabel). */
  label?: string;
}

export interface TransactionReportParams {
  date_from: string;
  date_to: string;
  warehouse_id?: string;
}

export interface TransactionReportRow {
  id: string;
  order_number: string | null;
  ordered_at: string | null;
  status: string | null;
  payment_status: string | null;
  payment_method: string | null;
  payment_method_code?: string | null;
  payment_method_name?: string | null;
  subtotal: number;
  discount_amount: number;
  tax_amount: number;
  service_charge_amount: number;
  total_amount: number;
  ark_coins_used: number;
  cashier_id: string | null;
  warehouse_id: string | null;
  stall_code: string | null;
  stall_name: string | null;
  checkout_id?: string | null;
  checkout_number?: string | null;
  sold_from?: string | null;
  xendit_qr_id?: string | null;
  xendit_external_id?: string | null;
  /** EPIC-043 — 'kol_comp' | 'owner_comp' + nama penyetuju (owner comp). */
  comp_type?: string | null;
  comp_approved_name?: string | null;
}

export interface TransactionReportSummary {
  transactions: number;
  total_sales: number;
  total_ark_used: number;
  /** Σ subtotal — nilai barang SEBELUM diskon/pajak/service. */
  revenue: number;
  discount: number;
  tax: number;
  service: number;
  /** Σ total_amount — yang benar-benar dibayar pelanggan. */
  nett: number;
}

/**
 * Rekap per stall berbasis ITEM (bukan order): order lintas stall (mode
 * Semua Stall) menyumbang ke tiap stall sesuai itemnya. Diskon/pajak/service
 * order-level sengaja tidak dialokasikan per stall — tidak ada dasar jujur
 * untuk membaginya.
 */
export interface TransactionStallSummary {
  stall_code: string | null;
  stall_name: string;
  /** Jumlah order yang menyentuh stall ini */
  transactions: number;
  /** Σ qty item */
  quantity: number;
  /** Σ total item (setelah diskon item, sebelum diskon/pajak transaksi) */
  sales: number;
}

export interface TransactionTopProduct {
  product_name: string;
  quantity: number;
  revenue: number;
}

export interface TransactionDailyPoint {
  date: string;
  nett: number;
  transactions: number;
}

export interface TransactionReport {
  filters: {
    date_from: string;
    date_to: string;
    warehouse_id: string | null;
  };
  stall_options: ReportStallOption[];
  stall_locked: boolean;
  summary: TransactionReportSummary;
  per_stall: TransactionStallSummary[];
  top_products: TransactionTopProduct[];
  daily: TransactionDailyPoint[];
  rows: TransactionReportRow[];
}

export interface RushHourReportParams {
  date_from: string;
  date_to: string;
  warehouse_id?: string;
}

export interface RushHourReport {
  filters: {
    date_from: string;
    date_to: string;
    warehouse_id: string | null;
  };
  stall_options: ReportStallOption[];
  stall_locked: boolean;
  summary: {
    transactions: number;
    revenue: number;
    quantity: number;
    average_ticket: number;
  };
  peak_hour: {
    hour: number | null;
    hour_label: string | null;
    dow: number | null;
    dow_label: string | null;
    transactions: number;
    revenue: number;
  };
  peak_revenue_hour: {
    hour: number | null;
    hour_label: string | null;
    dow: number | null;
    dow_label: string | null;
    transactions: number;
    revenue: number;
  };
  peak_day: {
    hour: number | null;
    hour_label: string | null;
    dow: number | null;
    dow_label: string | null;
    transactions: number;
    revenue: number;
  };
  hourly: Array<{
    hour: number;
    label: string;
    transactions: number;
    revenue: number;
    quantity: number;
    average_ticket: number;
  }>;
  weekdays: Array<{
    dow: number;
    label: string;
    transactions: number;
    revenue: number;
  }>;
  heatmap: Array<{
    hour: number;
    dow: number;
    transactions: number;
    revenue: number;
  }>;
}

export interface ProductSalesReportParams {
  date_from: string;
  date_to: string;
  warehouse_id?: string;
}

export interface ProductSalesReportRow {
  product_id: string | null;
  product_name: string;
  product_sku: string | null;
  warehouse_id: string | null;
  stall_code: string | null;
  stall_name: string | null;
  quantity: number;
  revenue: number;
  order_count: number;
}

export interface ProductSalesReport {
  filters: {
    date_from: string;
    date_to: string;
    warehouse_id: string | null;
  };
  stall_options: ReportStallOption[];
  stall_locked: boolean;
  summary: {
    products: number;
    quantity: number;
    revenue: number;
  };
  rows: ProductSalesReportRow[];
}

export interface VoidReportParams {
  date_from: string;
  date_to: string;
  warehouse_id?: string;
}

export interface VoidReportItem {
  id: string;
  product_name: string;
  product_sku: string | null;
  quantity: number;
  unit_price: number;
  total_amount: number;
}

export interface VoidReportRow {
  id: string;
  order_number: string | null;
  checkout_number: string | null;
  ordered_at: string | null;
  voided_at: string | null;
  void_reason: string | null;
  created_by_name: string;
  voided_by_name: string;
  stall_name: string | null;
  stall_code: string | null;
  total_amount: number;
  payment_method: string | null;
  payment_method_code?: string | null;
  payment_method_name?: string | null;
  sold_from?: string | null;
  checkout_id?: string | null;
  items: VoidReportItem[];
}

export interface VoidReport {
  filters: {
    date_from: string;
    date_to: string;
    warehouse_id: string | null;
  };
  stall_options: ReportStallOption[];
  stall_locked: boolean;
  summary: {
    voids: number;
    amount: number;
  };
  rows: VoidReportRow[];
}

export type PaymentMethodsReportGranularity = "day" | "month" | "year";

export interface PaymentMethodsReportParams {
  date_from: string;
  date_to: string;
  warehouse_id?: string;
  granularity?: PaymentMethodsReportGranularity;
}

export interface PaymentMethodsReportMethodRow {
  method_key: string;
  payment_method: string | null;
  payment_method_code: string | null;
  payment_method_name: string | null;
  label: string;
  amount: number;
  payment_count: number;
  order_count: number;
  pct: number;
}

export interface PaymentMethodsReportSeriesCell {
  method_key: string;
  label: string;
  amount: number;
  payment_count: number;
}

export interface PaymentMethodsReportSeriesRow {
  period: string;
  label: string;
  total_amount: number;
  payment_count: number;
  by_method: PaymentMethodsReportSeriesCell[];
}

export interface PaymentMethodsReport {
  filters: {
    date_from: string;
    date_to: string;
    warehouse_id: string | null;
    granularity: PaymentMethodsReportGranularity;
  };
  stall_options: ReportStallOption[];
  stall_locked: boolean;
  summary: {
    total_amount: number;
    payment_count: number;
    order_count: number;
    method_count: number;
  };
  by_method: PaymentMethodsReportMethodRow[];
  method_columns: Array<{ method_key: string; label: string }>;
  series: PaymentMethodsReportSeriesRow[];
}

export interface RevenueCompositionBucket {
  id: string;
  label: string;
  quantity: number;
  sales: number;
  cost: number;
  margin: number;
  cost_pct: number;
  sales_share_pct: number;
  qty_share_pct: number;
}

export interface RevenueCompositionGroup extends RevenueCompositionBucket {
  categories: RevenueCompositionBucket[];
}

export interface RevenueCompositionDay {
  date: string;
  food: RevenueCompositionBucket;
  beverage: RevenueCompositionBucket;
}

export interface RevenueCompositionReport {
  filters: { date_from: string; date_to: string };
  summary: {
    orders: number;
    items: number;
    quantity: number;
    sales: number;
    cost: number;
    margin: number;
    cost_pct: number;
    zero_cost_items: number;
  };
  groups: RevenueCompositionGroup[];
  daily: RevenueCompositionDay[];
}

export interface RevenueCompositionReportParams {
  date_from: string;
  date_to: string;
}

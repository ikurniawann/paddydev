export interface KDSOrderItem {
  id: string;
  product_id: string;
  product_name: string;
  product_sku: string;
  variant_info?: string;
  modifier_info?: string;
  quantity: number;
  unit_price: number;
  notes?: string;
  station?: string;
  kitchen_status?: string;
  kitchen_started_at?: string;
  kitchen_ready_at?: string;
  served_at?: string;
}

export interface KDSOrder {
  id: string;
  order_number: string;
  queue_number?: string | null;
  checkout_id?: string | null;
  warehouse_id?: string | null;
  station_status?: string;
  status: string;
  payment_status: string;
  order_type: string;
  table_id?: string;
  table_label?: string | null;
  notes?: string;
  special_requests?: string;
  ordered_at: string;
  confirmed_at?: string;
  pos_order_items: KDSOrderItem[];
  wait_seconds: number;
  wait_minutes: number;
  is_overdue: boolean;
  is_urgent: boolean;
}

export interface KdsStallOption {
  id: string;
  name: string;
  code: string;
  /** "<nama> — <branch>" bila daftar melintasi branch (dari stall-options). */
  label?: string;
}

export interface KdsListParams {
  status?: string[];
  station?: string;
  branchId?: string;
  warehouseId?: string;
  dateFrom?: string;
  dateTo?: string;
  limit?: number;
}

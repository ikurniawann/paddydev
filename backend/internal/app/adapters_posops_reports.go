package app

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/modules/posops"
	"nuhabit/backend/internal/modules/posops/domain"
	"nuhabit/backend/internal/platform/database"
)

// Report reads of pos-ops over pos-sales tables (pos_orders,
// pos_order_items, pos_checkouts, pos_split_payments), ported from the
// app/api/pos/reports routes and lib/pos/rush-hour-query.ts. Range bounds
// are the TS ISO strings, cast by PostgreSQL.

// posOpsStallFromItem resolves an order without warehouse_id to the stall of its
// largest item (the reports' LATERAL join).
const posOpsStallFromItem = `
	SELECT COALESCE(w.code, w_sku.code) AS stall_code,
	       COALESCE(w.name, w_sku.name) AS stall_name,
	       COALESCE(p.warehouse_id, p_sku.warehouse_id) AS warehouse_id
	FROM pos.pos_order_items i
	INNER JOIN pos.pos_products pp ON pp.id = i.product_id
	LEFT JOIN item.products p ON p.id = pp.source_product_id AND p.deleted_at IS NULL
	LEFT JOIN configuration.warehouses w ON w.id = p.warehouse_id
	LEFT JOIN item.products p_sku
	  ON pp.source_product_id IS NULL AND pp.sku = ('PUR-' || p_sku.kode)
	 AND p_sku.deleted_at IS NULL AND p_sku.kode IS NOT NULL AND btrim(p_sku.kode) <> ''
	LEFT JOIN configuration.warehouses w_sku ON w_sku.id = p_sku.warehouse_id
	WHERE i.order_id = o.id
	ORDER BY COALESCE(i.total_amount, 0) DESC, i.id
	LIMIT 1`

func (posOpsSales) RushHourPoints(ctx context.Context, q database.Querier, startIso, endIso string, warehouseIDs []string) ([]domain.RushHourPoint, error) {
	rows, err := q.Query(ctx, `SELECT
		EXTRACT(HOUR FROM o.ordered_at AT TIME ZONE 'Asia/Jakarta')::int::float8,
		EXTRACT(ISODOW FROM o.ordered_at AT TIME ZONE 'Asia/Jakarta')::int::float8,
		COUNT(*)::int::float8,
		COALESCE(SUM(o.total_amount), 0)::float8,
		COALESCE(SUM(oi.qty), 0)::float8
		FROM pos.pos_orders o
		LEFT JOIN (SELECT order_id, SUM(quantity) AS qty FROM pos.pos_order_items GROUP BY order_id) oi ON oi.order_id = o.id
		WHERE o.ordered_at >= $1::text::timestamptz AND o.ordered_at <= $2::text::timestamptz
		  AND o.status NOT IN ('cancelled', 'voided', 'merged')
		  AND (o.payment_status = 'paid' OR o.status = 'completed')
		  AND ($3::uuid[] IS NULL OR o.warehouse_id = ANY($3::uuid[]))
		GROUP BY 1, 2`, startIso, endIso, warehouseIDs)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.RushHourPoint, error) {
		var p domain.RushHourPoint
		err := r.Scan(&p.Hour, &p.Dow, &p.Transactions, &p.Revenue, &p.Quantity)
		return p, err
	})
}

func (posOpsSales) VoidedOrders(ctx context.Context, q database.Querier, startIso, endIso string, warehouseIDs []string) ([]posops.VoidOrder, error) {
	rows, err := q.Query(ctx, `SELECT
		o.id::text, o.order_number, chk.checkout_number, o.checkout_id::text, o.ordered_at, o.voided_at, o.void_reason,
		cashier.full_name, voider.full_name,
		COALESCE(w_order.name, stall_from_item.stall_name), COALESCE(w_order.code, stall_from_item.stall_code),
		o.total_amount::text, o.payment_method, o.payment_method_code, o.payment_method_name, o.sold_from
		FROM pos.pos_orders o
		LEFT JOIN pos.pos_checkouts chk ON chk.id = o.checkout_id
		LEFT JOIN configuration.users cashier ON cashier.id = o.cashier_id
		LEFT JOIN configuration.users voider ON voider.id = o.voided_by
		LEFT JOIN configuration.warehouses w_order ON w_order.id = o.warehouse_id
		LEFT JOIN LATERAL (`+posOpsStallFromItem+`) stall_from_item ON o.warehouse_id IS NULL
		WHERE o.status = 'voided'
		  AND COALESCE(o.voided_at, o.ordered_at) >= $1::text::timestamptz
		  AND COALESCE(o.voided_at, o.ordered_at) <= $2::text::timestamptz
		  AND ($3::uuid[] IS NULL OR o.warehouse_id = ANY($3::uuid[])
		       OR (o.warehouse_id IS NULL AND stall_from_item.warehouse_id = ANY($3::uuid[])))
		ORDER BY COALESCE(o.voided_at, o.ordered_at) DESC NULLS LAST`, startIso, endIso, warehouseIDs)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (posops.VoidOrder, error) {
		var o posops.VoidOrder
		err := r.Scan(&o.ID, &o.OrderNumber, &o.CheckoutNumber, &o.CheckoutID, &o.OrderedAt, &o.VoidedAt, &o.VoidReason,
			&o.CreatedByName, &o.VoidedByName, &o.StallName, &o.StallCode, &o.TotalAmount, &o.PaymentMethod,
			&o.PaymentMethodCode, &o.PaymentMethodName, &o.SoldFrom)
		return o, err
	})
}

func (posOpsSales) OrderLines(ctx context.Context, q database.Querier, orderIDs []string) ([]posops.OrderLine, error) {
	rows, err := q.Query(ctx, `SELECT id::text, order_id::text, product_name, product_sku, quantity::text,
		unit_price::text, total_amount::text
		FROM pos.pos_order_items WHERE order_id = ANY($1::uuid[]) ORDER BY created_at, id`, orderIDs)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (posops.OrderLine, error) {
		var l posops.OrderLine
		err := r.Scan(&l.ID, &l.OrderID, &l.ProductName, &l.ProductSku, &l.Quantity, &l.UnitPrice, &l.TotalAmount)
		return l, err
	})
}

func (posOpsSales) PaymentLegs(ctx context.Context, q database.Querier, startIso, endIso string, warehouseIDs []string) ([]posops.PaymentLeg, error) {
	rows, err := q.Query(ctx, `WITH filtered_orders AS (
		  SELECT o.id, o.total_amount, o.payment_method, o.payment_method_code, o.payment_method_name,
		         (o.ordered_at AT TIME ZONE 'Asia/Jakarta')::date::text AS hari_wib
		  FROM pos.pos_orders o
		  LEFT JOIN LATERAL (`+posOpsStallFromItem+`) stall_from_item ON o.warehouse_id IS NULL
		  WHERE o.ordered_at >= $1::text::timestamptz AND o.ordered_at <= $2::text::timestamptz
		    AND o.status NOT IN ('cancelled', 'voided', 'merged')
		    AND (o.payment_status = 'paid' OR o.status = 'completed')
		    AND ($3::uuid[] IS NULL OR o.warehouse_id = ANY($3::uuid[])
		         OR (o.warehouse_id IS NULL AND stall_from_item.warehouse_id = ANY($3::uuid[])))
		), split_legs AS (
		  SELECT fo.id AS order_id, fo.hari_wib, sp.payment_method::text AS payment_method,
		         NULL::text AS payment_method_code, NULL::text AS payment_method_name, sp.amount
		  FROM filtered_orders fo INNER JOIN pos.pos_split_payments sp ON sp.order_id = fo.id
		), single_legs AS (
		  SELECT fo.id AS order_id, fo.hari_wib, fo.payment_method, fo.payment_method_code,
		         fo.payment_method_name, fo.total_amount AS amount
		  FROM filtered_orders fo
		  WHERE NOT EXISTS (SELECT 1 FROM pos.pos_split_payments sp WHERE sp.order_id = fo.id)
		)
		SELECT order_id::text, hari_wib, payment_method, payment_method_code, payment_method_name, amount::text FROM split_legs
		UNION ALL
		SELECT order_id::text, hari_wib, payment_method, payment_method_code, payment_method_name, amount::text FROM single_legs`,
		startIso, endIso, warehouseIDs)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (posops.PaymentLeg, error) {
		var l posops.PaymentLeg
		err := r.Scan(&l.OrderID, &l.HariWib, &l.PaymentMethod, &l.PaymentMethodCode, &l.PaymentMethodName, &l.Amount)
		return l, err
	})
}

func (posOpsSales) PaidOrders(ctx context.Context, q database.Querier, start, end time.Time, newestFirst bool) ([]posops.PaidOrder, error) {
	sql := `SELECT id::text, order_number, ordered_at, cashier_id::text, total_amount::text FROM pos.pos_orders
		WHERE status NOT IN ('cancelled', 'voided', 'merged') AND payment_status = 'paid'
		  AND ordered_at >= $1 AND ordered_at <= $2`
	if newestFirst {
		sql += ` ORDER BY ordered_at DESC`
	}
	rows, err := q.Query(ctx, sql, start, end)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (posops.PaidOrder, error) {
		var o posops.PaidOrder
		err := r.Scan(&o.ID, &o.OrderNumber, &o.OrderedAt, &o.CashierID, &o.TotalAmount)
		return o, err
	})
}

func (posOpsSales) SaleLines(ctx context.Context, q database.Querier, orderIDs []string) ([]posops.SaleLine, error) {
	rows, err := q.Query(ctx, `SELECT COALESCE(order_id::text, ''), product_id::text, product_name, product_sku,
		quantity::text, total_amount::text, cost_price::text, cost_total::text, gross_profit::text,
		gross_margin_pct::text, station, discount_amount::text
		FROM pos.pos_order_items WHERE order_id = ANY($1::uuid[])`, orderIDs)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (posops.SaleLine, error) {
		var l posops.SaleLine
		err := r.Scan(&l.OrderID, &l.ProductID, &l.ProductName, &l.ProductSku, &l.Quantity, &l.TotalAmount, &l.CostPrice,
			&l.CostTotal, &l.GrossProfit, &l.GrossMarginPct, &l.Station, &l.DiscountAmount)
		return l, err
	})
}

func (posOpsSales) ClosingOrders(ctx context.Context, q database.Querier, startIso, endIso string, shiftID *string) ([]posops.ClosingOrder, error) {
	rows, err := q.Query(ctx, `SELECT id::text, shift_id::text, order_type::text, subtotal::text, discount_amount::text,
		tax_amount::text, service_charge_amount::text, total_amount::text, ordered_at, completed_at, discount_reason
		FROM pos.pos_orders
		WHERE status NOT IN ('cancelled', 'voided', 'merged') AND payment_status = 'paid' AND voided_at IS NULL
		  AND ordered_at >= $1::text::timestamptz AND ordered_at <= $2::text::timestamptz
		  AND ($3::text IS NULL OR shift_id = $3::text::uuid)`, startIso, endIso, shiftID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (posops.ClosingOrder, error) {
		var o posops.ClosingOrder
		err := r.Scan(&o.ID, &o.ShiftID, &o.OrderType, &o.Subtotal, &o.DiscountAmount, &o.TaxAmount, &o.ServiceCharge,
			&o.TotalAmount, &o.OrderedAt, &o.CompletedAt, &o.DiscountReason)
		return o, err
	})
}

/* ── Inventory: master item categories ───────────────────────────────── */

func (posOpsInventory) ItemKategori(ctx context.Context, q database.Querier, productIDs []string) (map[string]*string, error) {
	rows, err := q.Query(ctx, `SELECT pp.id::text, COALESCE(p.kategori, p_sku.kategori)
		FROM pos.pos_products pp
		LEFT JOIN item.products p ON p.id = pp.source_product_id AND p.deleted_at IS NULL
		LEFT JOIN item.products p_sku
		  ON pp.source_product_id IS NULL AND pp.sku = ('PUR-' || p_sku.kode)
		 AND p_sku.deleted_at IS NULL AND p_sku.kode IS NOT NULL AND btrim(p_sku.kode) <> ''
		WHERE pp.id = ANY($1::uuid[])`, productIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*string{}
	for rows.Next() {
		var id string
		var kategori *string
		if err := rows.Scan(&id, &kategori); err != nil {
			return nil, err
		}
		out[id] = kategori
	}
	return out, rows.Err()
}

func (posOpsInventory) ItemCategories(ctx context.Context, q database.Querier) ([]domain.ItemCategory, error) {
	rows, err := q.Query(ctx, `SELECT code, nama FROM item.product_categories WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.ItemCategory, error) {
		var c domain.ItemCategory
		err := r.Scan(&c.Code, &c.Nama)
		return c, err
	})
}

/* ── Directory and stalls ────────────────────────────────────────────── */

func (posOpsDirectory) Branches(ctx context.Context, q database.Querier, ids []string) (map[string]posops.Branch, error) {
	rows, err := q.Query(ctx, `SELECT id::text, name, code FROM configuration.branches WHERE id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]posops.Branch{}
	for rows.Next() {
		var id string
		var b posops.Branch
		if err := rows.Scan(&id, &b.Name, &b.Code); err != nil {
			return nil, err
		}
		out[id] = b
	}
	return out, rows.Err()
}

func (posOpsStalls) BranchByName(ctx context.Context, q database.Querier, branchID *string) ([]domain.Stall, error) {
	return scanPosOpsStalls(q.Query(ctx, `SELECT w.id::text, w.name, w.code, w.branch_id::text, w.is_default, COALESCE(b.name, '')
		FROM configuration.warehouses w
		LEFT JOIN configuration.branches b ON b.id = w.branch_id
		WHERE w.is_active = true AND ($1::text IS NULL OR w.branch_id = $1::text::uuid)
		ORDER BY w.name ASC`, branchID))
}

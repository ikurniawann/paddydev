package app

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"nuhabit/backend/internal/modules/posops"
	"nuhabit/backend/internal/modules/posops/domain"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/whatsapp"
)

// Adapters for pos-ops' ports. The SQL is ported from the TS routes and
// reads tables other contexts own: pos-sales (pos_orders, pos_checkouts),
// stored-value (pos_member_bill_payments) and configuration.

/* ── Sales (pos-sales) ───────────────────────────────────────────────── */

type posOpsSales struct{}

var _ posops.Sales = posOpsSales{}

func (posOpsSales) ShiftOrders(ctx context.Context, q database.Querier, shiftIDs []string, detailed bool) (map[string][]posops.ShiftOrderRef, error) {
	rows, err := q.Query(ctx, `SELECT shift_id::text, id::text, total_amount::float8, payment_method,
		amount_paid::float8, ark_coins_used::float8
		FROM pos.pos_orders WHERE shift_id = ANY($1::uuid[])`, shiftIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]posops.ShiftOrderRef{}
	for rows.Next() {
		var shiftID string
		var ref posops.ShiftOrderRef
		var p posops.ShiftOrderPayment
		if err := rows.Scan(&shiftID, &ref.ID, &p.TotalAmount, &p.PaymentMethod, &p.AmountPaid, &p.ArkCoinsUsed); err != nil {
			return nil, err
		}
		if detailed {
			ref.Payment = &p
		}
		out[shiftID] = append(out[shiftID], ref)
	}
	return out, rows.Err()
}

func (posOpsSales) ShiftCloseOrders(ctx context.Context, q database.Querier, shiftID string) ([]domain.ShiftOrder, error) {
	rows, err := q.Query(ctx, `SELECT total_amount::text, ark_coins_used::text, COALESCE(payment_method, ''), payment_method_code
		FROM pos.pos_orders
		WHERE shift_id = $1::text::uuid AND payment_status IN ('paid', 'partial') AND NOT (status = 'cancelled')`, shiftID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.ShiftOrder, error) {
		var o domain.ShiftOrder
		var total, ark *string
		err := r.Scan(&total, &ark, &o.PaymentMethod, &o.PaymentMethodCode)
		o.TotalAmount, o.ArkCoinsUsed = posOpsText(total), posOpsText(ark)
		return o, err
	})
}

func (posOpsSales) TableOpenOrders(ctx context.Context, q database.Querier) ([]domain.BoardOrder, error) {
	rows, err := q.Query(ctx, `SELECT id::text, order_number, table_id, status::text, payment_status::text,
		total_amount::text, pre_settled_at, checkout_id::text, sold_from, guest_count
		FROM pos.pos_orders
		WHERE table_id IS NOT NULL
		  AND status IN ('pending', 'confirmed', 'preparing', 'ready', 'served', 'completed')
		  AND payment_status <> 'paid'`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.BoardOrder, error) {
		var o domain.BoardOrder
		err := r.Scan(&o.ID, &o.OrderNumber, &o.TableID, &o.Status, &o.PaymentStatus, &o.TotalAmount,
			&o.PreSettledAt, &o.CheckoutID, &o.SoldFrom, &o.GuestCount)
		return o, err
	})
}

func (posOpsSales) TableUnpaidCheckouts(ctx context.Context, q database.Querier, companyID, branchID *string) ([]domain.BoardCheckout, error) {
	rows, err := q.Query(ctx, `SELECT id::text, checkout_number, table_id, payment_status::text, total_amount::text, notes
		FROM pos.pos_checkouts
		WHERE table_id IS NOT NULL AND payment_status <> 'paid'
		  AND ($1::uuid IS NULL OR company_id = $1::uuid)
		  AND ($2::uuid IS NULL OR branch_id = $2::uuid)`, companyID, branchID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.BoardCheckout, error) {
		var c domain.BoardCheckout
		err := r.Scan(&c.ID, &c.CheckoutNumber, &c.TableID, &c.PaymentStatus, &c.TotalAmount, &c.Notes)
		return c, err
	})
}

func (posOpsSales) TableHasActiveOrder(ctx context.Context, q database.Querier, tableID, exceptOrderID string) (bool, error) {
	var found bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pos.pos_orders
		WHERE table_id = $1 AND ($2 = '' OR id::text <> $2)
		  AND status IN ('pending', 'confirmed', 'preparing', 'ready', 'served'))`, tableID, exceptOrderID).Scan(&found)
	return found, err
}

func (posOpsSales) NextOrderNumber(ctx context.Context, q database.Querier) (string, error) {
	var n *string
	err := q.QueryRow(ctx, `SELECT * FROM generate_order_number()`).Scan(&n)
	if err != nil || n == nil {
		return "null", err // String(null), as the TS stringifies the RPC result
	}
	return *n, nil
}

func (posOpsSales) OpenTableBill(ctx context.Context, q database.Querier, b posops.OpenTableBill) (*posops.Obj, error) {
	return posops.QueryObj(ctx, q, `INSERT INTO pos.pos_orders
		(order_number, order_type, status, payment_status, customer_id, cashier_id, table_id, guest_count,
		 subtotal, discount_amount, tax_amount, service_charge_amount, total_amount, payment_method,
		 amount_paid, ark_coins_used, notes, special_requests, ordered_at)
		VALUES ($1, 'dine_in', 'pending', 'unpaid', $2::uuid, $3::uuid, $4, $5, 0, 0, 0, 0, 0, NULL, 0, 0, $6, NULL, now())
		RETURNING *`, b.OrderNumber, b.CustomerID, b.CashierID, b.TableID, b.GuestCount, b.Notes)
}

func (posOpsSales) DashboardOrders(ctx context.Context, q database.Querier, start, end time.Time) ([]domain.DashboardOrder, error) {
	rows, err := q.Query(ctx, `SELECT id::text, total_amount::text, cashier_id::text, ordered_at, ark_coins_used::text,
		payment_method, status::text, payment_status::text
		FROM pos.pos_orders WHERE ordered_at >= $1 AND ordered_at <= $2`, start, end)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.DashboardOrder, error) {
		var o domain.DashboardOrder
		var at *time.Time
		err := r.Scan(&o.ID, &o.TotalAmount, &o.CashierID, &at, &o.ArkCoinsUsed, &o.PaymentMethod, &o.Status, &o.PaymentStatus)
		if at != nil {
			o.OrderedAt = *at
		}
		return o, err
	})
}

func (posOpsSales) PaidOrderTotals(ctx context.Context, q database.Querier, start, end time.Time) ([]*string, error) {
	rows, err := q.Query(ctx, `SELECT total_amount::text FROM pos.pos_orders
		WHERE payment_status = 'paid' AND status NOT IN ('cancelled', 'voided', 'merged')
		  AND ordered_at >= $1 AND ordered_at <= $2`, start, end)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[*string])
}

func (posOpsSales) DashboardItems(ctx context.Context, q database.Querier, start, end time.Time) ([]domain.DashboardItem, error) {
	rows, err := q.Query(ctx, `SELECT COALESCE(order_id::text, ''), product_id::text, product_name, quantity::text, total_amount::text
		FROM pos.pos_order_items WHERE created_at >= $1 AND created_at <= $2
		ORDER BY created_at, id`, start, end) // stable: an id-less group is named by its first item
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.DashboardItem, error) {
		var it domain.DashboardItem
		err := r.Scan(&it.OrderID, &it.ProductID, &it.ProductName, &it.Quantity, &it.TotalAmount)
		return it, err
	})
}

func (posOpsSales) RecentOrders(ctx context.Context, q database.Querier, limit int) ([]posops.RecentOrder, error) {
	rows, err := q.Query(ctx, `SELECT id::text, order_number, total_amount::text, status::text, payment_status::text,
		ordered_at, cashier_id::text
		FROM pos.pos_orders ORDER BY ordered_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (posops.RecentOrder, error) {
		var o posops.RecentOrder
		err := r.Scan(&o.ID, &o.OrderNumber, &o.TotalAmount, &o.Status, &o.PaymentStatus, &o.OrderedAt, &o.CashierID)
		return o, err
	})
}

// posOpsText turns a NULL numeric into nil so Number(null) applies.
func posOpsText(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

/* ── Member bills (stored-value) ─────────────────────────────────────── */

type posOpsMemberBills struct{}

var _ posops.MemberBills = posOpsMemberBills{}

func (posOpsMemberBills) ShiftPayments(ctx context.Context, q database.Querier, shiftID string) ([]domain.MemberBillPayment, error) {
	rows, err := q.Query(ctx, `SELECT amount::text, payment_method, payment_method_code
		FROM pos.pos_member_bill_payments WHERE shift_id = $1::text::uuid`, shiftID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.MemberBillPayment, error) {
		var p domain.MemberBillPayment
		var amount string
		err := r.Scan(&amount, &p.PaymentMethod, &p.PaymentMethodCode)
		p.Amount = amount
		return p, err
	})
}

/* ── Directory (configuration) ───────────────────────────────────────── */

type posOpsDirectory struct{}

var _ posops.Directory = posOpsDirectory{}

func (posOpsDirectory) Setting(ctx context.Context, q database.Querier, key string) (*string, error) {
	var v *string
	err := q.QueryRow(ctx, `SELECT value FROM configuration.app_settings WHERE key = $1`, key).Scan(&v)
	if database.IsNoRows(err) {
		return nil, nil
	}
	return v, err
}

func (posOpsDirectory) FirstCompanyName(ctx context.Context, q database.Querier) (*string, error) {
	var v *string
	err := q.QueryRow(ctx, `SELECT name FROM configuration.companies ORDER BY created_at LIMIT 1`).Scan(&v)
	if database.IsNoRows(err) {
		return nil, nil
	}
	return v, err
}

func (posOpsDirectory) UserFullName(ctx context.Context, q database.Querier, userID string) (*string, error) {
	var v *string
	err := q.QueryRow(ctx, `SELECT full_name FROM configuration.users WHERE id = $1::text::uuid`, userID).Scan(&v)
	if database.IsNoRows(err) {
		return nil, nil
	}
	return v, err
}

func (posOpsDirectory) EmployeeNames(ctx context.Context, q database.Querier, ids []string) (map[string]string, error) {
	rows, err := q.Query(ctx, `SELECT id::text, full_name FROM hris.employees WHERE id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id string
		var name *string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		if name != nil && *name != "" {
			out[id] = *name
		}
	}
	return out, rows.Err()
}

func (posOpsDirectory) ActiveBranches(ctx context.Context, q database.Querier) ([]*posops.Obj, error) {
	return posops.QueryObjs(ctx, q, `SELECT id, name, code FROM configuration.branches WHERE is_active = true ORDER BY name ASC`)
}

/* ── CRM ─────────────────────────────────────────────────────────────── */

type posOpsCRM struct{}

var _ posops.CRM = posOpsCRM{}

func (posOpsCRM) TierDiscounts(ctx context.Context, q database.Querier) (map[string]float64, error) {
	rows, err := q.Query(ctx, `SELECT lower(code::text), discount_percent::text FROM crm.crm_membership_tiers WHERE is_active = true`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]float64{}
	for rows.Next() {
		var code string
		var pct *string
		if err := rows.Scan(&code, &pct); err != nil {
			return nil, err
		}
		out[code] = 0
		if pct != nil {
			out[code] = domain.ToNumber(*pct)
		}
	}
	return out, rows.Err()
}

func (posOpsCRM) PosXp(ctx context.Context, q database.Querier, start, end time.Time) ([]domain.XpRow, error) {
	rows, err := q.Query(ctx, `SELECT xp_delta::float8, created_at FROM crm.crm_xp_ledger
		WHERE source_channel = 'pos' AND direction = 'earn' AND created_at >= $1 AND created_at <= $2`, start, end)
	if err != nil {
		return nil, err
	}
	ledger, err := pgx.CollectRows(rows, scanPosOpsXp)
	if err != nil {
		return nil, err
	}
	rows, err = q.Query(ctx, `SELECT COALESCE(xp_earned, 0)::float8, created_at FROM pos.pos_xp_transactions
		WHERE created_at >= $1 AND created_at <= $2`, start, end)
	if err != nil {
		return nil, err
	}
	legacy, err := pgx.CollectRows(rows, scanPosOpsXp)
	return append(ledger, legacy...), err
}

func scanPosOpsXp(r pgx.CollectableRow) (domain.XpRow, error) {
	var x domain.XpRow
	var at *time.Time
	err := r.Scan(&x.XpEarned, &at)
	if at != nil {
		x.CreatedAt = *at
	}
	return x, err
}

// ArkCoinEnabled is getLoyaltyFeatures().arkCoin: a jsonb true/false (or
// "true"/1/"1" …) in crm.crm_settings, true when missing or unreadable.
func (posOpsCRM) ArkCoinEnabled(ctx context.Context, q database.Querier) bool {
	var raw []byte
	err := q.QueryRow(ctx, `SELECT value::text FROM crm.crm_settings WHERE key = 'ark_coin_enabled'`).Scan(&raw)
	if err != nil {
		return true
	}
	switch string(raw) {
	case "false", "0", `"false"`, `"0"`:
		return false
	}
	return true
}

/* ── Stored value (ARK Coin wallet) ──────────────────────────────────── */

type posOpsStoredValue struct{}

var _ posops.StoredValue = posOpsStoredValue{}

func (posOpsStoredValue) WalletCredits(ctx context.Context, q database.Querier, start, end time.Time) ([]*string, error) {
	rows, err := q.Query(ctx, `SELECT amount::text FROM pos.pos_wallet_transactions
		WHERE type IN ('topup', 'topup_bonus', 'bonus') AND created_at >= $1 AND created_at <= $2`, start, end)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[*string])
}

/* ── WhatsApp (lib/whatsapp sendWhatsAppText) ────────────────────────── */

// posOpsWhatsApp sends a "notification" text through platform/whatsapp,
// logged with the sending staff id.
type posOpsWhatsApp struct {
	db *pgxpool.Pool
	wa *whatsapp.Client
}

func (w posOpsWhatsApp) SendText(ctx context.Context, target, message, sentByUserID string) posops.Delivery {
	res := w.wa.SendText(ctx, w.db, target, message, "notification", sentByUserID)
	return posops.Delivery{Delivered: res.Success, Reason: res.Reason}
}

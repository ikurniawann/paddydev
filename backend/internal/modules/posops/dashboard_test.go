package posops_test

import (
	"testing"
)

func TestDashboard(t *testing.T) {
	h := newHarness(t, nil)
	h.anon("GET", "/api/pos/dashboard", nil, 401)
	r := h.call("GET", "/api/pos/dashboard?period=custom&date_from=2031-03-03&date_to=2031-03-01", nil, 400)
	jsonEq(t, r.body, `{"success":false,"error":"Rentang tanggal tidak valid (maks 366 hari, format YYYY-MM-DD)"}`)

	at := func(id, ts string) { h.exec(`UPDATE pos.pos_orders SET ordered_at = $2 WHERE id = $1`, id, ts) }
	a := h.order(nil, "pending", "paid", "cash", nil, 30000, "")
	b := h.order(nil, "completed", "paid", "ark_coin", nil, 20000, "")
	v := h.order(nil, "voided", "paid", "cash", nil, 99999, "")
	u := h.order(nil, "pending", "unpaid", "cash", nil, 5000, "")
	at(a, "2031-03-01T10:15:00+07:00")
	at(b, "2031-03-03T23:30:00+07:00")
	at(v, "2031-03-02T09:00:00+07:00")
	at(u, "2031-03-02T09:00:00+07:00")
	h.exec(`UPDATE pos.pos_orders SET ark_coins_used = 20000 WHERE id = $1`, b)
	item := func(order, product string, qty, total float64) {
		h.exec(`INSERT INTO pos.pos_order_items (order_id, product_name, product_sku, quantity, unit_price, subtotal, total_amount, created_at)
			VALUES ($1, $2, 'GT', $3, $4, $4, $4, '2031-03-02T12:00:00+07:00')`, order, product, qty, total)
	}
	item(a, "Kopi", 2, 20000)
	item(b, "Kopi", 1, 10000)
	item(a, "Teh", 1, 10000)
	// Teh comes after Kopi, so the id-less group keeps Kopi's name.
	h.exec(`UPDATE pos.pos_order_items SET created_at = created_at + interval '1 minute' WHERE order_id = $1 AND product_name = 'Teh'`, a)
	item(v, "Roti", 9, 99999)
	customer := h.customer()
	h.exec(`INSERT INTO pos.pos_wallet_transactions (customer_id, type, amount, ark_coins, balance_before, balance_after, created_at)
		VALUES ($1, 'topup', 50000, 50000, 0, 50000, '2031-03-02T08:00:00+07:00'),
		       ($1, 'payment', -20000, -20000, 50000, 30000, '2031-03-02T08:00:00+07:00')`, customer)
	h.exec(`INSERT INTO pos.pos_xp_transactions (customer_id, xp_earned, balance_before, balance_after, created_at)
		VALUES ($1, 7, 0, 7, '2031-03-03T01:00:00+07:00')`, customer)

	r = h.call("GET", "/api/pos/dashboard?period=custom&date_from=2031-03-01&date_to=2031-03-03", nil, 200)
	d := data(r)
	jsonEq(t, d["stats"], `{"todayRevenue":50000,"todayOrders":2,"averageOrderValue":25000,"activeCashiers":1,"revenueChange":0,"ordersChange":0}`)
	// Items without a product id share one "null" entry, as in the TS.
	jsonEq(t, d["topProducts"], `[{"id":"null","name":"Kopi","sold":4,"revenue":40000}]`)
	jsonEq(t, d["trend"], `[{"label":"1 Mar","revenue":30000,"orders":1,"arkUsed":0,"xpEarned":0},
		{"label":"2 Mar","revenue":0,"orders":0,"arkUsed":0,"xpEarned":0},
		{"label":"3 Mar","revenue":20000,"orders":1,"arkUsed":20000,"xpEarned":7}]`)
	ark := obj(d["arkXp"])
	if ark["totalArkUsed"] != float64(20000) || ark["totalArkEarned"] != float64(50000) || ark["totalXpEarned"] != float64(7) ||
		ark["arkPaymentOrders"] != float64(1) {
		t.Fatalf("arkXp = %v", ark)
	}
	keysInOrder(t, r.raw, "stats", "topProducts", "recentOrders", "arkXp", "trend", "topLoyalMembers")
	keysInOrder(t, r.raw, "totalArkUsed", "totalArkEarned", "totalXpEarned", "arkPaymentOrders", "membersWithXp", "totalArkBalance")
	recent := obj(list(d["recentOrders"])[0])
	if recent["time"] != "23.30" || recent["total"] != float64(20000) || recent["cashier"] != "—" || recent["payment_status"] != "paid" {
		t.Fatalf("recent = %v", recent)
	}
	if loyal := list(d["topLoyalMembers"]); len(loyal) > 5 {
		t.Fatalf("top loyal = %v", loyal)
	}

	r = h.call("GET", "/api/pos/dashboard?period=nope", nil, 200)
	if len(list(data(r)["trend"])) == 0 {
		t.Fatalf("month trend = %s", r.raw)
	}
}

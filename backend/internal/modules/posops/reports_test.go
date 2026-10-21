package posops_test

import (
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

// reportFixture seeds May 2031: a cash sale, a split cash+QRIS sale, a
// voided order and an unpaid one.
func reportFixture(h *harness) (cash, split, void string) {
	h.t.Helper()
	cash = h.order(nil, "pending", "paid", "cash", nil, 30000, "")
	split = h.order(nil, "completed", "paid", "cash", nil, 25000, "")
	void = h.order(nil, "voided", "paid", "qris", nil, 12000, "")
	unpaid := h.order(nil, "pending", "unpaid", "cash", nil, 1000, "")
	h.exec(`UPDATE pos.pos_orders SET ordered_at = '2031-05-04T10:15:00+07:00', order_type = 'dine_in' WHERE id = $1`, cash)
	h.exec(`UPDATE pos.pos_orders SET ordered_at = '2031-05-05T20:00:00+07:00', discount_amount = 5000, discount_reason = 'Promo GT' WHERE id = $1`, split)
	h.exec(`UPDATE pos.pos_orders SET ordered_at = '2031-05-05T09:00:00+07:00', voided_at = '2031-05-05T09:30:00+07:00',
		void_reason = 'salah input', voided_by = $2 WHERE id = $1`, void, h.staff.UserID)
	h.exec(`UPDATE pos.pos_orders SET ordered_at = '2031-05-05T09:00:00+07:00' WHERE id = $1`, unpaid)
	splitID := h.id(`INSERT INTO pos.pos_order_splits (order_id, split_index, total_amount) VALUES ($1, 1, 25000) RETURNING id::text`, split)
	h.exec(`INSERT INTO pos.pos_split_payments (split_id, order_id, amount, payment_method, cashier_id)
		VALUES ($1, $2, 10000, 'cash', 'x'), ($1, $2, 15000, 'qris', 'x')`, splitID, split)
	line := func(order, name, station string, qty, total, cost float64) {
		h.exec(`INSERT INTO pos.pos_order_items (order_id, product_name, product_sku, quantity, unit_price, subtotal,
			total_amount, cost_price, cost_total, gross_profit, station)
			VALUES ($1, $2, 'GT', $3, $4, $4, $4, $5, $5, $4::numeric - $5::numeric, $6)`, order, name, qty, total, cost, station)
	}
	line(cash, "Kopi", "bar", 2, 30000, 10000)
	line(split, "Nasi", "kitchen", 1, 25000, 0)
	line(void, "Teh", "bar", 1, 12000, 0)
	return cash, split, void
}

func TestReports(t *testing.T) {
	h := newHarness(t, nil)
	cash, _, void := reportFixture(h)
	month := "date_from=2031-05-01&date_to=2031-05-31"

	h.anon("GET", "/api/pos/reports/rush-hour", nil, 401)
	r := h.call("GET", "/api/pos/reports/rush-hour?date_from=2031-05-09&date_to=2031-05-01", nil, 400)
	jsonEq(t, r.body, `{"success":false,"error":"Tanggal dari tidak boleh melebihi tanggal sampai"}`)
	r = h.call("GET", "/api/pos/reports/rush-hour?"+month+"&warehouse_id=00000000-0000-4000-8000-000000000001", nil, 400)
	jsonEq(t, r.body, `{"success":false,"error":"Outlet tidak valid atau di luar scope"}`)
	r = h.call("GET", "/api/pos/reports/voids?date_from=2031-05-09&date_to=2031-05-01", nil, 500)
	jsonEq(t, r.body, `{"success":false,"error":"Tanggal dari tidak boleh melebihi tanggal sampai"}`)

	// Rush hour: two paid sales, 10:00 Sunday and 20:00 Monday (WIB).
	r = h.call("GET", "/api/pos/reports/rush-hour?"+month, nil, 200)
	d := data(r)
	jsonEq(t, d["filters"], `{"date_from":"2031-05-01","date_to":"2031-05-31","warehouse_id":null}`)
	jsonEq(t, d["summary"], `{"transactions":2,"revenue":55000,"quantity":3,"average_ticket":27500}`)
	jsonEq(t, d["peak_revenue_hour"], `{"hour":10,"hour_label":"10:00","dow":null,"dow_label":null,"transactions":1,"revenue":30000}`)
	jsonEq(t, d["peak_day"], `{"hour":null,"hour_label":null,"dow":1,"dow_label":"Sen","transactions":1,"revenue":25000}`)
	if len(list(d["hourly"])) != 24 || len(list(d["heatmap"])) != 7*15 || d["stall_locked"] != false {
		t.Fatalf("rush hour = %s", r.raw)
	}
	keysInOrder(t, r.raw, "filters", "stall_options", "stall_locked", "summary", "peak_hour", "peak_revenue_hour",
		"peak_day", "hourly", "weekdays", "heatmap")

	// Voids.
	r = h.call("GET", "/api/pos/reports/voids?"+month, nil, 200)
	d = data(r)
	jsonEq(t, d["summary"], `{"voids":1,"amount":12000}`)
	row := obj(list(d["rows"])[0])
	if row["id"] != void || row["void_reason"] != "salah input" || row["voided_at"] != "2031-05-05T02:30:00.000Z" ||
		row["created_by_name"] != h.staff.FullName || row["voided_by_name"] != h.staff.FullName || row["total_amount"] != float64(12000) {
		t.Fatalf("void row = %v", row)
	}
	jsonEq(t, list(row["items"])[0], `{"id":"`+obj(list(row["items"])[0])["id"].(string)+`","product_name":"Teh","product_sku":"GT","quantity":1,"unit_price":12000,"total_amount":12000}`)

	// Payment methods: the split order counts as two legs.
	r = h.call("GET", "/api/pos/reports/payment-methods?"+month+"&granularity=month", nil, 200)
	d = data(r)
	jsonEq(t, d["summary"], `{"total_amount":55000,"payment_count":3,"order_count":2,"method_count":2}`)
	first := obj(list(d["by_method"])[0])
	if first["method_key"] != "cash" || first["label"] != "Tunai" || first["amount"] != float64(40000) ||
		first["payment_count"] != float64(2) || first["order_count"] != float64(2) || first["pct"] != float64(72.7) {
		t.Fatalf("by method = %s", r.raw)
	}
	series := list(d["series"])
	if len(series) != 1 || obj(series[0])["label"] != "Mei 2031" || obj(series[0])["total_amount"] != float64(55000) {
		t.Fatalf("series = %v", series)
	}
	if obj(d["filters"])["granularity"] != "month" {
		t.Fatalf("filters = %v", d["filters"])
	}
	r = h.call("GET", "/api/pos/reports/payment-methods?"+month, nil, 200)
	if days := list(data(r)["series"]); len(days) != 31 || obj(days[3])["label"] != "4 Mei 2031" {
		t.Fatalf("daily series = %v", days)
	}

	// Profit (UTC days): items of paid orders only.
	r = h.call("GET", "/api/pos/reports/profit?"+month, nil, 200)
	d = data(r)
	jsonEq(t, d["summary"], `{"orders":2,"items":2,"quantity":3,"revenue":55000,"cogs":10000,"gross_profit":45000,"gross_margin_pct":81.82,"zero_cost_items":1}`)
	stations := list(obj(d["breakdowns"])["stations"])
	jsonEq(t, stations[0], `{"id":"kitchen","label":"kitchen","quantity":1,"revenue":25000,"cogs":0,"gross_profit":25000,"gross_margin_pct":100}`)
	dates := list(obj(d["breakdowns"])["dates"])
	if len(dates) != 2 || obj(dates[0])["id"] != "2031-05-04" {
		t.Fatalf("dates = %v", dates)
	}
	r = h.call("GET", "/api/pos/reports/profit?date_from=bad", nil, 500)
	jsonEq(t, r.body, `{"success":false,"error":"Invalid time value"}`)

	// Revenue composition: bar items are Beverage without a category.
	r = h.call("GET", "/api/pos/reports/revenue-composition?"+month, nil, 200)
	d = data(r)
	groups := list(d["groups"])
	bev := obj(groups[1])
	if bev["id"] != "beverage" || bev["sales"] != float64(30000) || bev["cost_pct"] != 33.33 || bev["sales_share_pct"] != 54.55 {
		t.Fatalf("beverage = %v", bev)
	}
	jsonEq(t, list(bev["categories"])[0], `{"id":"tanpa kategori","label":"Tanpa kategori","quantity":2,"sales":30000,"cost":10000,"margin":20000,"cost_pct":33.33,"sales_share_pct":100,"qty_share_pct":100}`)
	days := list(d["daily"])
	if len(days) != 2 || obj(days[0])["date"] != "Mon May 05" || obj(days[1])["date"] != "Sun May 04" {
		t.Fatalf("daily = %v", days)
	}

	// Closing: the WIB day of the cash sale.
	r = h.call("GET", "/api/pos/reports/closing?date=2031-05-04", nil, 200)
	d = data(r)
	jsonEq(t, d["sales_summary"], `{"net_sales":30000,"service":0,"tax":0,"discount":0,"gross":30000}`)
	jsonEq(t, d["transaction_totals"], `{"transactions":1,"sales":30000,"discount":0,"full_discount_transactions":0,"full_discount_amount":0}`)
	jsonEq(t, d["guests"], `{"count":1,"average_per_pax":30000}`)
	jsonEq(t, d["shift_sessions"], `[{"label":"All","last_order":"10:15","closed_at":"23:59"}]`)
	if obj(d["header"])["report_date"] != "Sunday, 04th May 2031" || obj(obj(d["targets"])["daily"])["actual"] != float64(30000) ||
		obj(d["footer"])["printed_by"] != h.staff.FullName {
		t.Fatalf("closing = %s", r.raw)
	}
	bev = obj(list(d["categories_by_segment"])[1])
	jsonEq(t, bev, `{"segment":"BEV","title":"SALES BY CATEGORY BEVERAGE","rows":[{"name":"Others","amount":30000,"percentage":100}]}`)
	keysInOrder(t, r.raw, "filters", "header", "shift_sessions", "sales_summary", "transaction_totals", "guests",
		"categories_by_segment", "targets", "promos_by_segment", "footer")
	r = h.call("GET", "/api/pos/reports/closing?date=2031-05-05", nil, 200)
	jsonEq(t, list(data(r)["promos_by_segment"])[0], `{"segment":"FNB","title":"PROMO F&B","rows":[{"name":"Promo GT","qty":1}]}`)
	r = h.call("GET", "/api/pos/reports/closing?date=nope", nil, 500)
	jsonEq(t, r.body, `{"success":false,"error":"Failed to load cashier closing report"}`)
	_ = cash
}

// An unscoped user picks any outlet of any branch: the stall_options of a
// report must tell the branches' "Main Storage" rows apart.
func TestReportStallOptionsBranchLabels(t *testing.T) {
	h := newHarness(t, nil)
	sfx := testutil.RandomHex(4)
	holding := h.id(`INSERT INTO configuration.holdings (name, code) VALUES ('H '||$1, 'H'||$1) RETURNING id::text`, sfx)
	company := h.id(`INSERT INTO configuration.companies (holding_id, name, code) VALUES ($1, 'C '||$2, 'C'||$2) RETURNING id::text`, holding, sfx)
	zeta := h.id(`INSERT INTO configuration.branches (company_id, name, code) VALUES ($1, 'Zeta '||$2, 'Z'||$2) RETURNING id::text`, company, sfx)
	alpha := h.id(`INSERT INTO configuration.branches (company_id, name, code) VALUES ($1, 'Alpha '||$2, 'A'||$2) RETURNING id::text`, company, sfx)
	mainA := h.id(`SELECT id::text FROM configuration.warehouses WHERE branch_id = $1 AND code = 'MAIN'`, alpha)
	mainZ := h.id(`SELECT id::text FROM configuration.warehouses WHERE branch_id = $1 AND code = 'MAIN'`, zeta)
	stallZ := h.id(`INSERT INTO configuration.warehouses (branch_id, name, code) VALUES ($1, 'Stall 1', 'STALL-01') RETURNING id::text`, zeta)

	r := h.call("GET", "/api/pos/reports/rush-hour?date_from=2031-05-01&date_to=2031-05-31", nil, 200)
	var got []string
	for _, v := range list(data(r)["stall_options"]) {
		o := obj(v)
		if o["branch_id"] == alpha || o["branch_id"] == zeta {
			got = append(got, o["id"].(string)+"|"+o["branch_name"].(string)+"|"+o["label"].(string))
		}
	}
	want := []string{
		mainA + "|Alpha " + sfx + "|Main Storage — Alpha " + sfx,
		mainZ + "|Zeta " + sfx + "|Main Storage — Zeta " + sfx,
		stallZ + "|Zeta " + sfx + "|Stall 1 — Zeta " + sfx,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("stall_options:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// The existing fields stay put.
	first := obj(list(data(r)["stall_options"])[0])
	for _, k := range []string{"id", "code", "name"} {
		if _, ok := first[k]; !ok {
			t.Fatalf("missing %s in %v", k, first)
		}
	}
}

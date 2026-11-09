package shop

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nuhabit/backend/internal/modules/shop/domain"
	"nuhabit/backend/internal/platform/testutil"
)

// wholesaleFixture adds a storefront, a web product with a real
// pos_products row (product_settings references it) and a partner account
// created through the staff route.
type wholesaleFixture struct {
	*fixture
	product  string
	email    string
	password string
	account  map[string]any
}

func newWholesaleFixture(t *testing.T, terms string) *wholesaleFixture {
	t.Helper()
	f := &wholesaleFixture{fixture: newFixture(t)}
	f.exec(`INSERT INTO shop.storefronts (slug, name, is_default) VALUES ($1, 'Toko Go', false)`, "go-"+testutil.RandomHex(4))
	f.product = f.scalar(`INSERT INTO pos.pos_products (sku, name, product_kind, inventory_quantity)
		VALUES ($1, 'Kaos', 'merchandise', 0) RETURNING id::text`, "GO-"+testutil.RandomHex(4))
	stock := "0"
	f.catalog.products[f.product] = WebProduct{ID: f.product, Name: "Kaos", BasePrice: "100000", InventoryQuantity: &stock}
	f.email = "mitra-" + testutil.RandomHex(4) + "@example.test"
	res, body := f.staff("POST", "/api/shop/wholesale/accounts", map[string]any{
		"company_name": "Gym Mitra", "contact_name": "Sari", "email": strings.ToUpper(f.email), "phone": "0811111111",
		"discount_pct": 20, "min_order_idr": 500000, "payment_terms": terms,
	})
	expectStatus(t, res, 201)
	f.account = body["data"].(map[string]any)
	f.password, _ = f.account["password"].(string)
	if len(f.password) != 12 || f.account["email"] != f.email || f.account["password_hash"] != nil {
		t.Fatalf("account = %v", f.account)
	}
	return f
}

// login signs the partner in and returns the session cookie.
func (f *wholesaleFixture) login(password string) (*httptest.ResponseRecorder, string) {
	f.t.Helper()
	res, _ := f.do("POST", "/api/wholesale/login", map[string]string{"email": f.email, "password": password}, "x-forwarded-for", "10.0.0.7")
	for _, c := range res.Result().Cookies() {
		if c.Name == domain.WholesaleSessionCookie && c.Value != "" {
			if !c.HttpOnly {
				f.t.Error("session cookie must be httpOnly")
			}
			return res, c.Value
		}
	}
	return res, ""
}

func (f *wholesaleFixture) partner(token, method, target string, body any) (*httptest.ResponseRecorder, map[string]any) {
	f.t.Helper()
	r := testutil.Request(method, target, body)
	r.AddCookie(&http.Cookie{Name: domain.WholesaleSessionCookie, Value: token})
	return testutil.Do(f.t, f.mux, r)
}

func TestWholesaleStaffRoutesNeedShopMenu(t *testing.T) {
	f := newFixture(t)
	for _, route := range []string{"GET /api/shop/wholesale/accounts", "GET /api/shop/wholesale/orders", "GET /api/shop/wholesale/products"} {
		method, path, _ := strings.Cut(route, " ")
		res, _ := f.do(method, path, nil, "X-Test-Forbid", "1")
		expectStatus(t, res, 403)
		expectJSON(t, res, `{"success":false,"error":"Insufficient permissions"}`)
		res, _ = f.do(method, path, nil)
		expectStatus(t, res, 401)
	}
	res, _ := f.do("POST", "/api/shop/wholesale/accounts", map[string]any{"company_name": "X"}, "X-Test-Forbid", "1")
	expectStatus(t, res, 403)
}

func TestWholesaleLoginAndLockout(t *testing.T) {
	f := newWholesaleFixture(t, "invoice")

	res, _ := f.partner("", "GET", "/api/wholesale/me", nil)
	expectStatus(t, res, 401)

	res, token := f.login("salah")
	expectStatus(t, res, 401)
	expectJSON(t, res, `{"success":false,"error":"Incorrect email or password"}`)
	if token != "" {
		t.Fatal("no cookie on a failed login")
	}

	res, token = f.login(f.password)
	expectStatus(t, res, 200)
	if token == "" {
		t.Fatal("session cookie expected")
	}
	res, body := f.partner(token, "GET", "/api/wholesale/me", nil)
	expectStatus(t, res, 200)
	if me := body["data"].(map[string]any); me["company_name"] != "Gym Mitra" || me["last_login_at"] == nil {
		t.Errorf("me = %v", me)
	}

	res, _ = f.partner(token, "POST", "/api/wholesale/logout", nil)
	expectStatus(t, res, 200)
	res, _ = f.partner(token, "GET", "/api/wholesale/me", nil)
	expectStatus(t, res, 401)

	// Five failures lock the email for 15 minutes, the right password included.
	for i := 1; i < domain.WholesaleLoginEmailLimit; i++ {
		res, _ = f.login("salah")
		expectStatus(t, res, 401)
	}
	res, _ = f.login(f.password)
	expectStatus(t, res, 429)
	if !strings.Contains(res.Body.String(), "Too many sign-in attempts") {
		t.Errorf("body = %s", res.Body.String())
	}

	// A disabled account cannot sign in and loses its sessions.
	f.exec(`DELETE FROM platform.rate_limits WHERE key LIKE 'wholesale-login%'`)
	_, token = f.login(f.password)
	res, _ = f.staff("PATCH", "/api/shop/wholesale/accounts/"+f.account["id"].(string), map[string]any{
		"company_name": "Gym Mitra", "contact_name": "Sari", "email": f.email, "payment_terms": "invoice", "status": "disabled",
	})
	expectStatus(t, res, 200)
	res, _ = f.partner(token, "GET", "/api/wholesale/me", nil)
	expectStatus(t, res, 401)
	res, _ = f.login(f.password)
	expectStatus(t, res, 403)

	// A password reset hands out a new one-time password.
	res, body = f.staff("PATCH", "/api/shop/wholesale/accounts/"+f.account["id"].(string), map[string]any{
		"company_name": "Gym Mitra", "contact_name": "Sari", "email": f.email, "payment_terms": "invoice", "reset_password": true,
	})
	expectStatus(t, res, 200)
	fresh, _ := body["data"].(map[string]any)["password"].(string)
	if len(fresh) != 12 || fresh == f.password {
		t.Fatalf("reset password = %q", fresh)
	}
	res, _ = f.login(fresh)
	expectStatus(t, res, 200)
	res, body = f.staff("GET", "/api/shop/wholesale/accounts", nil)
	expectStatus(t, res, 200)
	if rows := body["data"].([]any); rows[0].(map[string]any)["password"] != nil {
		t.Error("the list never carries a password")
	}
}

func TestWholesaleCatalogPricing(t *testing.T) {
	f := newWholesaleFixture(t, "invoice")
	over := "120000"
	f.svc.ports.Catalog = fakeCatalog{products: f.catalog.products, skus: []CatalogSKU{
		{ID: "s1", ProductID: f.product, SKU: "K-L", Name: "L", StockQuantity: "3"},
		{ID: "s2", ProductID: f.product, SKU: "K-XL", Name: "XL", PriceOverride: &over, StockQuantity: "0"},
	}}
	_, token := f.login(f.password)

	// Discount tier: 20% off each retail price, rounded to rupiah.
	res, body := f.partner(token, "GET", "/api/wholesale/catalog", nil)
	expectStatus(t, res, 200)
	p := body["data"].([]any)[0].(map[string]any)
	skus := p["skus"].([]any)
	if p["price"] != 80000.0 || p["minQty"] != 1.0 || skus[0].(map[string]any)["price"] != 80000.0 || skus[1].(map[string]any)["price"] != 96000.0 {
		t.Errorf("discount pricing = %v", p)
	}

	// A wholesale price override wins over the discount, min qty follows.
	res, _ = f.staff("PATCH", "/api/shop/wholesale/products/"+f.product, map[string]any{"wholesale_price_idr": 70000, "wholesale_min_qty": 12})
	expectStatus(t, res, 200)
	res, body = f.partner(token, "GET", "/api/wholesale/catalog", nil)
	expectStatus(t, res, 200)
	p = body["data"].([]any)[0].(map[string]any)
	skus = p["skus"].([]any)
	if p["price"] != 70000.0 || p["minQty"] != 12.0 || skus[1].(map[string]any)["price"] != 70000.0 || p["retailPrice"] != 100000.0 {
		t.Errorf("override pricing = %v", p)
	}

	res, _ = f.staff("PATCH", "/api/shop/wholesale/products/"+f.product, map[string]any{"wholesale_min_qty": 0})
	expectStatus(t, res, 400)
	res, _ = f.staff("PATCH", "/api/shop/wholesale/products/7f1c2d3e-4b5a-4c6d-8e9f-0a1b2c3d4e5f", map[string]any{"wholesale_min_qty": 2})
	expectStatus(t, res, 404)
}

func TestWholesaleOrderValidationAndInvoice(t *testing.T) {
	f := newWholesaleFixture(t, "invoice")
	f.exec(`INSERT INTO shop.product_settings (product_id, wholesale_min_qty) VALUES ($1::uuid, 6)`, f.product)
	stock := "50"
	f.catalog.products[f.product] = WebProduct{ID: f.product, Name: "Kaos", BasePrice: "100000", InventoryQuantity: &stock}
	_, token := f.login(f.password)
	order := func(qty int) map[string]any {
		return map[string]any{
			"items":            []map[string]any{{"product_id": f.product, "quantity": qty}},
			"shipping_address": "Jl. Mitra Raya no 12, Bandung",
		}
	}

	res, body := f.partner(token, "POST", "/api/wholesale/orders", order(5))
	expectStatus(t, res, 400)
	if body["error"] != "Minimum order for Kaos is 6 pcs" {
		t.Errorf("min qty error = %v", body["error"])
	}
	// 6 x 80000 = 480000, under the 500000 minimum order.
	res, body = f.partner(token, "POST", "/api/wholesale/orders", order(6))
	expectStatus(t, res, 400)
	if body["error"] != "Minimum order value is Rp 500.000" {
		t.Errorf("min order error = %v", body["error"])
	}
	res, _ = f.partner(token, "POST", "/api/wholesale/orders", map[string]any{"items": []any{}, "shipping_address": "x"})
	expectStatus(t, res, 400)

	res, body = f.partner(token, "POST", "/api/wholesale/orders", order(7))
	expectStatus(t, res, 201)
	created := body["data"].(map[string]any)
	if created["invoice_url"] != "https://pay.example/inv-1" || created["status"] != "pending" {
		t.Fatalf("created = %v", created)
	}
	id := created["id"].(string)
	got := f.scalar(`SELECT status || '|' || subtotal || '|' || total || '|' || payment_terms || '|' || (due_at IS NULL) || '|' || customer_name || '|' || (wholesale_account_id = $2::uuid)
		FROM shop.orders WHERE id = $1::uuid`, id, f.account["id"])
	if got != "pending|560000.00|560000.00|invoice|true|Gym Mitra|true" {
		t.Errorf("order = %s", got)
	}
	if got := f.scalar(`SELECT status || '|' || qty FROM shop.stock_reservations WHERE order_id = $1::uuid`, id); got != "held|7.00" {
		t.Errorf("reservation = %s", got)
	}
	if len(f.stock.calls) != 1 || f.stock.calls[0] != f.product+":7" {
		t.Errorf("stock claims = %v", f.stock.calls)
	}
	if inv := f.pay.invoices[0]; inv.Amount != 560000 || inv.PayerName != "Gym Mitra" {
		t.Errorf("invoice = %+v", inv)
	}

	res, body = f.partner(token, "GET", "/api/wholesale/orders", nil)
	expectStatus(t, res, 200)
	rows := body["data"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["invoice_url"] != "https://pay.example/inv-1" || rows[0].(map[string]any)["item_count"] != "1" {
		t.Errorf("history = %v", rows)
	}
	res, body = f.partner(token, "GET", "/api/wholesale/orders/"+id, nil)
	expectStatus(t, res, 200)
	detail := body["data"].(map[string]any)
	if items := detail["items"].([]any); len(items) != 1 || items[0].(map[string]any)["unit_price"] != "80000.00" || detail["shipping_address"] != "Jl. Mitra Raya no 12, Bandung" {
		t.Errorf("detail = %v", detail)
	}

	// Another partner never sees it; staff see it with the company.
	other := newWholesaleFixture(t, "invoice")
	_, otherToken := other.login(other.password)
	res, _ = other.partner(otherToken, "GET", "/api/wholesale/orders/"+id, nil)
	expectStatus(t, res, 404)
	res, body = f.staff("GET", "/api/shop/wholesale/orders?search=Mitra", nil)
	expectStatus(t, res, 200)
	if rows := body["data"].([]any); len(rows) < 1 || rows[0].(map[string]any)["company_name"] != "Gym Mitra" || rows[0].(map[string]any)["payment_terms"] != "invoice" {
		t.Errorf("staff list = %v", rows)
	}
}

func TestWholesalePayLaterOrder(t *testing.T) {
	now := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	f := newWholesaleFixture(t, "pay_later")
	// The session is checked against the database's now(), so it must be issued on the real clock.
	_, token := f.login(f.password)
	f.svc.now = func() time.Time { return now }
	stock := "50"
	f.catalog.products[f.product] = WebProduct{ID: f.product, Name: "Kaos", BasePrice: "100000", InventoryQuantity: &stock}

	res, body := f.partner(token, "POST", "/api/wholesale/orders", map[string]any{
		"items":            []map[string]any{{"product_id": f.product, "quantity": 10}},
		"shipping_address": "Jl. Mitra Raya no 12, Bandung",
		"notes":            "Kirim pagi",
	})
	expectStatus(t, res, 201)
	created := body["data"].(map[string]any)
	if created["invoice_url"] != nil || created["status"] != "pending" {
		t.Fatalf("created = %v", created)
	}
	id := created["id"].(string)
	got := f.scalar(`SELECT status || '|' || payment_terms || '|' || to_char(due_at AT TIME ZONE 'UTC', 'YYYY-MM-DD') || '|' || (xendit_invoice_id IS NULL) || '|' || (invoice_expires_at IS NULL) || '|' || notes
		FROM shop.orders WHERE id = $1::uuid`, id)
	if got != "pending|pay_later|2026-11-06|true|true|Kirim pagi" {
		t.Errorf("order = %s", got)
	}
	if got := f.scalar(`SELECT status FROM shop.stock_reservations WHERE order_id = $1::uuid`, id); got != "committed" {
		t.Errorf("reservation = %s", got)
	}
	if len(f.pay.invoices) != 0 {
		t.Error("pay_later creates no invoice")
	}

	// Staff pack it before payment, then record the payment.
	res, _ = f.staff("PATCH", "/api/shop/orders/"+id, map[string]string{"status": "packing"})
	expectStatus(t, res, 200)
	res, _ = f.staff("PATCH", "/api/shop/orders/"+id, map[string]string{"status": "cancelled"})
	expectStatus(t, res, 200)
	if got := f.scalar(`SELECT status FROM shop.stock_reservations WHERE order_id = $1::uuid`, id); got != "released" {
		t.Errorf("cancelled reservation = %s", got)
	}
	if len(f.stock.calls) != 2 || f.stock.calls[1] != f.product+":-10" {
		t.Errorf("stock calls = %v", f.stock.calls)
	}

	// A retail pending order still cannot be packed or marked paid by hand.
	retail := f.order("pending")
	res, _ = f.staff("PATCH", "/api/shop/orders/"+retail, map[string]string{"status": "paid"})
	expectStatus(t, res, 409)
}

func TestPreorderCheckoutSkipsStock(t *testing.T) {
	f := newFixture(t)
	f.svc.biteshipBase = fakeBiteship(t).URL
	f.env["BITESHIP_API_KEY"] = "bs-key"
	slug := "go-" + testutil.RandomHex(4)
	f.exec(`INSERT INTO shop.storefronts (slug, name) VALUES ($1, 'Toko Go')`, slug)
	f.exec(`UPDATE shop.shipping_settings SET is_active = false`)
	f.exec(`INSERT INTO shop.shipping_settings (provider, origin_area_id, couriers, markup_amount) VALUES ('biteship', 'ORIGIN', 'jne', 0)`)
	product := f.scalar(`INSERT INTO pos.pos_products (sku, name, product_kind, inventory_quantity) VALUES ($1, 'Jaket', 'merchandise', 0) RETURNING id::text`, "GO-"+testutil.RandomHex(4))
	f.exec(`INSERT INTO shop.product_settings (product_id, preorder_until) VALUES ($1::uuid, (now() AT TIME ZONE 'Asia/Jakarta')::date + 14)`, product)
	stock := "0"
	f.catalog.products[product] = WebProduct{ID: product, Name: "Jaket", BasePrice: "250000", InventoryQuantity: &stock}
	f.stock.fail[product] = true // a claim would be refused: zero stock

	res, body := f.do("GET", "/api/public/shop/"+slug+"/catalog", nil)
	expectStatus(t, res, 200)
	view := body["data"].(map[string]any)["products"].([]any)[0].(map[string]any)
	if view["preorder"] != true || view["preorderUntil"] == nil || view["stock"] != 0.0 {
		t.Errorf("catalog = %v", view)
	}

	body = map[string]any{
		"items":       []map[string]any{{"product_id": product, "quantity": 2}},
		"customer":    map[string]any{"name": "Budi", "phone": "081234567890"},
		"destination": map[string]any{"area_id": "IDNP6", "label": "Coblong, Bandung", "address": "Jl. Dago 1 no 10"},
		"courier":     map[string]any{"code": "jne", "service_code": "reg"},
	}
	res, out := f.do("POST", "/api/public/shop/"+slug+"/checkout", body)
	expectStatus(t, res, 201)
	number := out["data"].(map[string]any)["order_number"].(string)
	if got := f.scalar(`SELECT i.is_preorder::text || '|' || i.quantity FROM shop.order_items i JOIN shop.orders o ON o.id = i.order_id WHERE o.order_number = $1`, number); got != "true|2.00" {
		t.Errorf("line = %s", got)
	}
	if got := f.scalar(`SELECT count(*)::text FROM shop.stock_reservations r JOIN shop.orders o ON o.id = r.order_id WHERE o.order_number = $1`, number); got != "0" {
		t.Errorf("reservations = %s", got)
	}
	if len(f.stock.calls) != 0 {
		t.Errorf("stock claims = %v", f.stock.calls)
	}

	// Past the date the product is sold out again.
	f.exec(`UPDATE shop.product_settings SET preorder_until = (now() AT TIME ZONE 'Asia/Jakarta')::date - 1 WHERE product_id = $1::uuid`, product)
	res, out = f.do("POST", "/api/public/shop/"+slug+"/checkout", body)
	expectStatus(t, res, 400)
	if out["error"] != "Not enough stock for Jaket" {
		t.Errorf("error = %v", out["error"])
	}
}

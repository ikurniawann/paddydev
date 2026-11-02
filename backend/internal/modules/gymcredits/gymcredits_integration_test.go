package gymcredits

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/platform/testutil"
)

// Integration tests against TEST_DATABASE_URL. The ledger is append-only (a
// trigger refuses DELETE), so every write runs inside one transaction that is
// rolled back; staff and member fixtures come from testutil and are removed
// after the rollback.

type env struct {
	t      *testing.T
	ctx    context.Context
	tx     pgx.Tx
	mux    http.Handler
	svc    *Service
	staff  testutil.Staff
	member testutil.Member
}

func setup(t *testing.T) *env {
	t.Helper()
	deps := testutil.Deps(t, nil)
	staff := testutil.CreateStaff(t, testutil.StaffOptions{Menus: map[string][]string{
		"gym.packages": nil, "gym.credits": nil, "gym.rules": nil,
	}})
	member := testutil.CreateMember(t)
	ctx := context.Background()
	tx, err := deps.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	h := newHandler(deps, tx)
	return &env{t: t, ctx: ctx, tx: tx, mux: testutil.Mux(creditsModule{routes: h.Routes()}), svc: h.svc, staff: staff, member: member}
}

func (e *env) exec(sql string, args ...any) {
	e.t.Helper()
	if _, err := e.tx.Exec(e.ctx, sql, args...); err != nil {
		e.t.Fatalf("exec %q: %v", sql, err)
	}
}

func (e *env) scalar(dst any, sql string, args ...any) {
	e.t.Helper()
	if err := e.tx.QueryRow(e.ctx, sql, args...).Scan(dst); err != nil {
		e.t.Fatalf("query %q: %v", sql, err)
	}
}

func (e *env) pkg(name string, credits int, price float64, validity int, limit *int) string {
	e.t.Helper()
	var id string
	e.scalar(&id, `INSERT INTO gym.credit_packages (name, credits, price_idr, validity_days, purchase_limit_per_member)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`, name, credits, price, validity, limit)
	return id
}

type who int

const (
	anon who = iota
	asStaff
	asMember
)

// call serves a request and checks the status; it returns the decoded body.
func (e *env) call(as who, method, path string, body any, status int) map[string]any {
	e.t.Helper()
	r := testutil.Request(method, path, body)
	switch as {
	case asStaff:
		r = testutil.AsStaff(r, e.staff)
	case asMember:
		r = testutil.AsMember(r, e.member)
	}
	rec, out := testutil.Do(e.t, e.mux, r)
	if rec.Code != status {
		e.t.Fatalf("%s %s: status %d, want %d: %s", method, path, rec.Code, status, rec.Body.String())
	}
	return out
}

func (e *env) data(as who, method, path string, body any) map[string]any {
	e.t.Helper()
	out := e.call(as, method, path, body, http.StatusOK)
	if out["success"] != true {
		e.t.Fatalf("%s %s: %v", method, path, out)
	}
	data, _ := out["data"].(map[string]any)
	return data
}

func (e *env) fail(as who, method, path string, body any, status int, msg string) map[string]any {
	e.t.Helper()
	out := e.call(as, method, path, body, status)
	if out["success"] != false || out["error"] != msg {
		e.t.Fatalf("%s %s: %v, want error %q", method, path, out, msg)
	}
	return out
}

func (e *env) balance() int {
	e.t.Helper()
	b, err := e.svc.Balance(e.ctx, e.tx, e.member.CustomerID)
	if err != nil {
		e.t.Fatal(err)
	}
	return b
}

func (e *env) arkBalance() float64 {
	e.t.Helper()
	var v float64
	e.scalar(&v, `SELECT ark_coin_balance::float FROM pos.pos_customers WHERE id = $1`, e.member.CustomerID)
	return v
}

// buy sells pkgID at the front desk for cash (paid at once).
func (e *env) buy(pkgID string) *Purchase {
	e.t.Helper()
	p, err := e.svc.SellPackage(e.ctx, FrontDeskSale{CustomerID: e.member.CustomerID, PackageID: pkgID, PaymentMethod: "cash", CashierID: e.staff.UserID})
	if err != nil {
		e.t.Fatal(err)
	}
	return p
}

func wantErr(t *testing.T, err error, status int, contains string) {
	t.Helper()
	var ce *Error
	if !errors.As(err, &ce) || ce.Status != status || !strings.Contains(ce.Message, contains) {
		t.Fatalf("got %v, want %d %q", err, status, contains)
	}
}

/* ── Ledger (port of credits-server.db.test.ts) ─────────────────────── */

func TestLedgerAgainstPostgres(t *testing.T) {
	e := setup(t)
	pkg := e.pkg("Uji 10", 10, 1_500_000, 90, nil)
	one := 1
	trial := e.pkg("Uji Trial", 1, 200_000, 14, &one)
	e.exec(`UPDATE pos.pos_customers SET ark_coin_balance = 2000000 WHERE id = $1`, e.member.CustomerID)
	c := e.member.CustomerID

	// Issues credits once per paid purchase.
	first := e.buy(pkg)
	if first.Status != "paid" {
		t.Fatalf("status %s", first.Status)
	}
	already, _, err := e.svc.markPaid(e.ctx, e.tx, first.ID, nil)
	if err != nil || !already {
		t.Fatalf("second markPaid: %v %v", already, err)
	}
	if b := e.balance(); b != 10 {
		t.Fatalf("balance %d", b)
	}

	// Deducts idempotently and refuses overdraft; class refunds are idempotent.
	move := CreditMove{CustomerID: c, Amount: 3, SourceType: "booking", SourceID: c, IdempotencyKey: "t-" + c}
	for range 2 {
		ok, after, err := e.svc.Deduct(e.ctx, e.tx, move)
		if err != nil || !ok || after != 7 {
			t.Fatalf("deduct: %v %d %v", ok, after, err)
		}
	}
	over := move
	over.Amount, over.IdempotencyKey = 8, "t2-"+c
	if ok, _, err := e.svc.Deduct(e.ctx, e.tx, over); err != nil || ok {
		t.Fatalf("overdraft allowed: %v %v", ok, err)
	}
	refund := move
	refund.IdempotencyKey = "r-" + c
	for range 2 {
		if err := e.svc.RefundClass(e.ctx, e.tx, refund); err != nil {
			t.Fatal(err)
		}
	}
	if b := e.balance(); b != 10 {
		t.Fatalf("balance after refund %d", b)
	}

	// Adjusts with a reason and reverses an entry once.
	_, err = e.svc.Adjust(e.ctx, e.tx, c, 2, "", e.staff.UserID)
	wantErr(t, err, 400, "Alasan")
	adj, err := e.svc.Adjust(e.ctx, e.tx, c, 2, "kompensasi", e.staff.UserID)
	if err != nil || adj.BalanceAfter != 12 {
		t.Fatalf("adjust: %+v %v", adj, err)
	}
	if _, err := e.svc.Reverse(e.ctx, e.tx, adj.EntryID, "salah member", e.staff.UserID); err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.Reverse(e.ctx, e.tx, adj.EntryID, "lagi", e.staff.UserID)
	wantErr(t, err, 409, "sudah pernah")
	wallet, err := e.svc.Wallet(e.ctx, e.tx, c, 100)
	if err != nil || wallet.Balance != 10 {
		t.Fatalf("wallet: %+v %v", wallet, err)
	}
	for _, lot := range wallet.Lots {
		if lot.PackageID == nil && lot.Remaining != 0 {
			t.Fatalf("adjustment lot still has %d", lot.Remaining)
		}
	}

	// Refuses a refund once credits were used (balance 10, used 3 then refunded: use 1 now).
	if _, _, err := e.svc.Deduct(e.ctx, e.tx, CreditMove{CustomerID: c, Amount: 1, SourceType: "booking", SourceID: c, IdempotencyKey: "u-" + c}); err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.RefundPurchaseTx(e.ctx, first.ID, "minta refund", e.staff.UserID)
	wantErr(t, err, 409, "tidak cukup")

	// Purchase limit and ARK Coin refunds.
	arkSale, err := e.svc.SellPackage(e.ctx, FrontDeskSale{CustomerID: c, PackageID: trial, PaymentMethod: "ark_coin", CashierID: e.staff.UserID})
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.SellPackage(e.ctx, FrontDeskSale{CustomerID: c, PackageID: trial, PaymentMethod: "cash", CashierID: e.staff.UserID})
	wantErr(t, err, 409, "Batas pembelian")
	if ark := e.arkBalance(); ark != 1_800_000 {
		t.Fatalf("ark after buy %v", ark)
	}
	refunded, err := e.svc.RefundPurchaseTx(e.ctx, arkSale.ID, "batal ikut", e.staff.UserID)
	if err != nil || refunded.Status != "refunded" || refunded.Note == nil || *refunded.Note != "Refund: batal ikut" {
		t.Fatalf("refund: %+v %v", refunded, err)
	}
	if ark := e.arkBalance(); ark != 2_000_000 {
		t.Fatalf("ark after refund %v", ark)
	}
	var walletRows int
	e.scalar(&walletRows, `SELECT count(*) FROM pos.pos_wallet_transactions WHERE customer_id = $1 AND metadata->>'purchase_id' = $2`, c, arkSale.ID)
	if walletRows != 2 {
		t.Fatalf("wallet rows %d", walletRows)
	}

	// Class coverage follows the package list.
	classType := "00000000-0000-4000-8000-000000000001"
	e.exec(`UPDATE gym.credit_packages SET applicable_class_type_ids = ARRAY[$2::uuid] WHERE id = $1`, pkg, classType)
	covered, err := e.svc.CoveredClassTypeIDs(e.ctx, e.tx, c)
	if err != nil || len(covered) != 1 || covered[0] != classType {
		t.Fatalf("covered %v %v", covered, err)
	}

	// Lapsed lots expire lazily, once.
	e.exec(`UPDATE gym.credit_lots SET expires_at = now() - interval '1 day' WHERE customer_id = $1`, c)
	if b := e.balance(); b != 0 {
		t.Fatalf("balance after expiry %d", b)
	}
	e.balance()
	var expirations int
	e.scalar(&expirations, `SELECT count(*) FROM gym.credit_ledger WHERE customer_id = $1 AND type = 'expiration'`, c)
	if expirations != 1 {
		t.Fatalf("expirations %d", expirations)
	}
}

/* ── Staff routes ────────────────────────────────────────────────────── */

func TestPackageRoutes(t *testing.T) {
	e := setup(t)
	missing := "6f1c2a1e-0d7b-4c55-9a43-1b0b6a0f5e11"

	e.fail(anon, "GET", "/api/gym/packages", nil, 401, "Authentication required")
	e.fail(asStaff, "PATCH", "/api/gym/packages/abc", map[string]any{"status": "archived"}, 400, "ID tidak valid")
	out := e.fail(asStaff, "PATCH", "/api/gym/packages/"+missing, map[string]any{"status": "deleted"}, 400,
		`Invalid option: expected one of "active"|"archived"`)
	if _, ok := out["details"].([]any); !ok {
		t.Fatalf("details missing: %v", out)
	}
	e.fail(asStaff, "PATCH", "/api/gym/packages/"+missing, map[string]any{"status": "active"}, 404, "Paket tidak ditemukan")

	created := e.data(asStaff, "POST", "/api/gym/packages", map[string]any{
		"name": "  Go Paket  ", "credits": 4, "price_idr": 450000.5, "validity_days": 30,
	})
	id, _ := created["id"].(string)
	if len(created) != 1 || !isUUID(id) {
		t.Fatalf("created %v", created)
	}
	list := e.data(asStaff, "GET", "/api/gym/packages", nil)
	var row map[string]any
	for _, p := range list["packages"].([]any) {
		if m := p.(map[string]any); m["id"] == id {
			row = m
		}
	}
	if row == nil || row["name"] != "Go Paket" || row["price_idr"] != 450000.5 || row["applicable_class_type_ids"] != nil ||
		row["sold_count"] != 0.0 || row["referenced"] != false || row["branch_name"] != nil {
		t.Fatalf("row %v", row)
	}
	if _, ok := list["class_types"].([]any); !ok {
		t.Fatalf("class_types %v", list["class_types"])
	}

	updated := e.data(asStaff, "POST", "/api/gym/packages", map[string]any{
		"id": id, "name": "Go Paket 2", "credits": 4, "price_idr": 400000, "validity_days": 30,
		"applicable_class_type_ids": []string{"00000000-0000-4000-8000-000000000001"},
	})
	if updated["id"] != id {
		t.Fatalf("updated %v", updated)
	}
	e.fail(asStaff, "POST", "/api/gym/packages", map[string]any{
		"id": missing, "name": "X Paket", "credits": 1, "price_idr": 1, "validity_days": 1,
	}, 404, "Paket tidak ditemukan")

	archived := e.data(asStaff, "PATCH", "/api/gym/packages/"+id, map[string]any{"status": "archived"})
	if archived["id"] != id || archived["status"] != "archived" {
		t.Fatalf("archived %v", archived)
	}

	sold := e.pkg("Go Sold", 1, 1000, 10, nil)
	e.buy(sold)
	e.fail(asStaff, "DELETE", "/api/gym/packages/"+sold, nil, 409, "Paket sudah pernah dibeli. Arsipkan saja agar riwayat tetap utuh.")
	deleted := e.data(asStaff, "DELETE", "/api/gym/packages/"+id, nil)
	if deleted["id"] != id || deleted["deleted"] != true {
		t.Fatalf("deleted %v", deleted)
	}
	e.fail(asStaff, "DELETE", "/api/gym/packages/"+id, nil, 404, "Paket tidak ditemukan")
}

func TestCreditDeskRoutes(t *testing.T) {
	e := setup(t)
	c := e.member.CustomerID
	pkg := e.pkg("Go Desk", 5, 500_000, 60, nil)

	e.fail(asStaff, "POST", "/api/gym/credits/"+c+"/sell", map[string]any{"package_id": "x"}, 400, "Pilih paket")
	sale := e.data(asStaff, "POST", "/api/gym/credits/"+c+"/sell", map[string]any{
		"package_id": pkg, "payment_method": "transfer", "discount_idr": 100000, "note": "  promo  ",
	})
	if sale["status"] != "paid" || sale["total_idr"] != 400000.0 || sale["discount_idr"] != 100000.0 ||
		sale["note"] != "promo" || sale["channel"] != "front_desk" || sale["created_by"] != e.staff.UserID {
		t.Fatalf("sale %v", sale)
	}
	meta := sale["payment_meta"].(map[string]any)
	if meta["provider"] != "front_desk" || meta["cashier_id"] != e.staff.UserID {
		t.Fatalf("meta %v", meta)
	}
	comp := e.data(asStaff, "POST", "/api/gym/credits/"+c+"/sell", map[string]any{"package_id": pkg, "payment_method": "complimentary"})
	if comp["total_idr"] != 0.0 || comp["discount_idr"] != 500000.0 {
		t.Fatalf("complimentary %v", comp)
	}

	adj := e.data(asStaff, "POST", "/api/gym/credits/"+c+"/adjust", map[string]any{"amount": -3, "reason": "koreksi"})
	if adj["balanceAfter"] != 7.0 || adj["entryId"] == nil {
		t.Fatalf("adjust %v", adj)
	}
	e.fail(asStaff, "POST", "/api/gym/credits/"+c+"/adjust", map[string]any{"amount": -30, "reason": "koreksi"}, 400, "Saldo kredit tidak boleh minus.")

	rev := e.data(asStaff, "POST", "/api/gym/credits/reverse", map[string]any{"entry_id": adj["entryId"], "reason": "batal koreksi"})
	if rev["amount"] != 3.0 || rev["customerId"] != c {
		t.Fatalf("reverse %v", rev)
	}
	e.fail(asStaff, "POST", "/api/gym/credits/reverse", map[string]any{"entry_id": adj["entryId"], "reason": "lagi lagi"}, 409, "Entri ini sudah pernah dibatalkan.")
	e.fail(asStaff, "POST", "/api/gym/credits/reverse", map[string]any{"entry_id": "6f1c2a1e-0d7b-4c55-9a43-1b0b6a0f5e11", "reason": "tidak ada"}, 404, "Entri kredit tidak ditemukan")

	page := e.data(asStaff, "GET", "/api/gym/credits/"+c, nil)
	member := page["member"].(map[string]any)
	if member["id"] != c || member["ark_balance_idr"] != 0.0 || page["balance"] != 10.0 {
		t.Fatalf("page %v", page)
	}
	if len(page["purchases"].([]any)) != 2 || len(page["lots"].([]any)) != 2 {
		t.Fatalf("purchases/lots %v", page)
	}
	var staffNamed bool
	for _, raw := range page["entries"].([]any) {
		entry := raw.(map[string]any)
		if entry["type"] == "adjustment" && entry["created_by_name"] == e.staff.FullName && entry["reversed"] == true {
			staffNamed = true
		}
	}
	if !staffNamed {
		t.Fatalf("entries %v", page["entries"])
	}
	e.fail(asStaff, "GET", "/api/gym/credits/6f1c2a1e-0d7b-4c55-9a43-1b0b6a0f5e11", nil, 404, "Member tidak ditemukan")

	found := e.call(asStaff, "GET", "/api/gym/credits?q="+strings.TrimPrefix(e.member.Phone, "+"), nil, 200)["data"].([]any)
	if len(found) != 1 || found[0].(map[string]any)["balance"] != 10.0 {
		t.Fatalf("search %v", found)
	}
	var listed bool
	for _, raw := range e.call(asStaff, "GET", "/api/gym/credits", nil, 200)["data"].([]any) {
		listed = listed || raw.(map[string]any)["id"] == c
	}
	if !listed {
		t.Fatal("member with recent credit activity not listed")
	}

	purchaseID := sale["id"].(string)
	e.fail(asStaff, "POST", "/api/gym/credits/purchases/"+purchaseID+"/refund", map[string]any{"reason": "x"}, 400, "Alasan refund wajib diisi")
	refunded := e.data(asStaff, "POST", "/api/gym/credits/purchases/"+purchaseID+"/refund", map[string]any{"reason": "salah jual"})
	if refunded["status"] != "refunded" || refunded["note"] != "promo · Refund: salah jual" || refunded["refunded_at"] == nil {
		t.Fatalf("refunded %v", refunded)
	}
	e.fail(asStaff, "POST", "/api/gym/credits/purchases/"+purchaseID+"/refund", map[string]any{"reason": "lagi"}, 409, "Hanya pembelian lunas yang bisa direfund")
}

func TestRulesRoutes(t *testing.T) {
	e := setup(t)
	var branchID string
	// FOR KEY SHARE until the rollback: other packages' tests commit branches
	// and delete them on cleanup (CI runs packages in parallel), and the
	// branch override below references this one.
	e.scalar(&branchID, `SELECT id FROM configuration.branches WHERE COALESCE(is_active, true) ORDER BY name LIMIT 1 FOR KEY SHARE`)

	view := e.data(asStaff, "GET", "/api/gym/rules", nil)
	if view["defaults"].(map[string]any)["qrTtlSec"] != 45.0 {
		t.Fatalf("view %v", view)
	}

	e.fail(asStaff, "PUT", "/api/gym/rules", map[string]any{"rules": map[string]any{"qrTtlSec": 30}}, 400, "creditExpiryDays: Masa berlaku kredit harus berupa angka")
	full := map[string]any{}
	raw, _ := json.Marshal(view["defaults"])
	_ = json.Unmarshal(raw, &full)
	full["qrTtlSec"] = 30
	saved := e.data(asStaff, "PUT", "/api/gym/rules", map[string]any{"branch_id": nil, "rules": full})
	if saved["global"].(map[string]any)["qrTtlSec"] != 30.0 {
		t.Fatalf("global %v", saved["global"])
	}

	e.fail(asStaff, "PUT", "/api/gym/rules", map[string]any{"branch_id": branchID, "rules": map[string]any{"antiPassback": 1}}, 400, "Kunci aturan tidak dikenal: antiPassback")
	saved = e.data(asStaff, "PUT", "/api/gym/rules", map[string]any{"branch_id": branchID, "rules": map[string]any{"antiPassbackMin": 30}})
	override := branchOverride(t, saved, branchID)
	if len(override) != 1 || override["antiPassbackMin"] != 30.0 {
		t.Fatalf("override %v", override)
	}
	rules, err := e.svc.Rules(e.ctx, e.tx, &branchID)
	if err != nil || rules.AntiPassbackMin != 30 || rules.QRTTLSec != 30 {
		t.Fatalf("effective %+v %v", rules, err)
	}
	saved = e.data(asStaff, "PUT", "/api/gym/rules", map[string]any{"branch_id": branchID, "rules": map[string]any{}})
	if len(branchOverride(t, saved, branchID)) != 0 {
		t.Fatalf("override not cleared: %v", saved)
	}
}

func branchOverride(t *testing.T, view map[string]any, branchID string) map[string]any {
	t.Helper()
	for _, b := range view["branches"].([]any) {
		if m := b.(map[string]any); m["id"] == branchID {
			return m["override"].(map[string]any)
		}
	}
	t.Fatalf("branch %s missing", branchID)
	return nil
}

/* ── Member portal ───────────────────────────────────────────────────── */

func TestMemberPortalRoutes(t *testing.T) {
	// A fake Xendit: one QR, whose payments list reports success.
	xendit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/qr_codes":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "qr_test_1", "qr_string": "QRSTRING", "reference_id": body["reference_id"], "amount": body["amount"]})
		case r.URL.Path == "/qr_codes/qr_test_1/payments":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"id": "qrpy_1", "status": "SUCCEEDED"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer xendit.Close()
	t.Setenv("XENDIT_API_BASE_URL", xendit.URL)
	t.Setenv("MEMBER_OTP_DEV_CODE", "")

	e := setup(t)
	c := e.member.CustomerID
	pkg := e.pkg("Go Portal", 3, 300_000, 30, nil)
	e.exec(`UPDATE pos.pos_customers SET ark_coin_balance = 500000 WHERE id = $1`, c)

	e.fail(anon, "GET", "/api/member-portal/gym/credits", nil, 401, "Unauthorized")
	wallet := e.data(asMember, "GET", "/api/member-portal/gym/credits", nil)
	if wallet["balance"] != 0.0 || len(wallet["lots"].([]any)) != 0 {
		t.Fatalf("wallet %v", wallet)
	}

	list := e.data(asMember, "GET", "/api/member-portal/gym/credits/packages", nil)
	if list["ark_balance_idr"] != 500000.0 || list["can_simulate"] != false {
		t.Fatalf("packages %v", list)
	}
	var portalPkg map[string]any
	for _, p := range list["packages"].([]any) {
		if m := p.(map[string]any); m["id"] == pkg {
			portalPkg = m
		}
	}
	if portalPkg == nil || portalPkg["can_buy"] != true || portalPkg["blocked_reason"] != nil || portalPkg["restricted"] != false {
		t.Fatalf("portal package %v", portalPkg)
	}

	e.fail(asMember, "POST", "/api/member-portal/gym/credits/purchases", map[string]any{"package_id": "x"}, 400, "Pilih paket")
	e.fail(asMember, "POST", "/api/member-portal/gym/credits/purchases", "not json", 400, "Invalid input: expected string, received undefined")

	paid := e.data(asMember, "POST", "/api/member-portal/gym/credits/purchases", map[string]any{"package_id": pkg, "method": "ark_coin"})
	if paid["status"] != "paid" || paid["qr_string"] != nil || paid["payment_method"] != "ark_coin" {
		t.Fatalf("ark purchase %v", paid)
	}
	if ark := e.arkBalance(); ark != 200_000 {
		t.Fatalf("ark %v", ark)
	}

	// ARK Coin switched off in CRM.
	e.exec(`INSERT INTO crm.crm_settings (key, value) VALUES ('ark_coin_enabled', 'false'::jsonb)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`)
	off := e.fail(asMember, "POST", "/api/member-portal/gym/credits/purchases", map[string]any{"package_id": pkg, "method": "ark_coin"}, 409,
		"Fitur ARK Coin sedang dinonaktifkan di pengaturan CRM")
	if off["code"] != "ARK_COIN_DISABLED" {
		t.Fatalf("code %v", off)
	}

	// No gateway configured and no dev bypass: 503, purchase closed as failed.
	e.exec(`DELETE FROM configuration.payment_gateways WHERE provider = 'xendit'`)
	e.fail(asMember, "POST", "/api/member-portal/gym/credits/purchases", map[string]any{"package_id": pkg, "method": "qris"}, 503,
		"Pembayaran QRIS belum tersedia: QRIS payment gateway is not configured. Set it in Settings → Payment Gateways.")
	var failed int
	e.scalar(&failed, `SELECT count(*) FROM gym.credit_purchases WHERE customer_id = $1 AND status = 'failed'`, c)
	if failed != 1 {
		t.Fatalf("failed purchases %d", failed)
	}

	// Active gateway: QR created, polling reconciles the payment.
	e.exec(`INSERT INTO configuration.payment_gateways (provider, display_name, is_active, secret_key, callback_url)
		VALUES ('xendit', 'Xendit', true, 'xnd_test', NULL)`)
	pending := e.data(asMember, "POST", "/api/member-portal/gym/credits/purchases", map[string]any{"package_id": pkg, "method": "qris"})
	if pending["status"] != "pending" || pending["qr_string"] != "QRSTRING" || pending["simulated"] != false || pending["expires_at"] == nil {
		t.Fatalf("qris purchase %v", pending)
	}
	var external string
	e.scalar(&external, `SELECT external_id FROM gym.credit_purchases WHERE id = $1`, pending["id"])
	if !IsGymPurchaseReference(external) || len(external) != len(GymPurchaseRefPrefix)+24 {
		t.Fatalf("external_id %q", external)
	}
	e.fail(asMember, "GET", "/api/member-portal/gym/credits/purchases/abc", nil, 400, "ID tidak valid")
	e.fail(asMember, "GET", "/api/member-portal/gym/credits/purchases/6f1c2a1e-0d7b-4c55-9a43-1b0b6a0f5e11", nil, 404, "Pembelian tidak ditemukan")
	status := e.data(asMember, "GET", "/api/member-portal/gym/credits/purchases/"+pending["id"].(string), nil)
	if status["status"] != "paid" || status["qr_string"] != nil {
		t.Fatalf("status %v", status)
	}
	if b := e.balance(); b != 6 {
		t.Fatalf("credits %d", b)
	}

	// Webhook settlement by reference is idempotent.
	res, err := e.svc.SettleByReference(e.ctx, external, nil)
	if err != nil || res.Status != "already_paid" {
		t.Fatalf("settle %+v %v", res, err)
	}
	res, err = e.svc.SettleByReference(e.ctx, "gymcp_missing", nil)
	if err != nil || res.Status != "not_found" {
		t.Fatalf("settle missing %+v %v", res, err)
	}

	// simulate-paid is 404 outside the dev bypass.
	e.fail(asMember, "POST", "/api/member-portal/gym/credits/purchases/"+pending["id"].(string)+"/simulate-paid", nil, 404, "Not found")

	wallet = e.data(asMember, "GET", "/api/member-portal/gym/credits", nil)
	if wallet["balance"] != 6.0 || len(wallet["lots"].([]any)) != 2 {
		t.Fatalf("wallet %v", wallet)
	}
	for _, raw := range wallet["entries"].([]any) {
		if raw.(map[string]any)["created_by_name"] != nil {
			t.Fatalf("staff name leaked: %v", raw)
		}
	}
}

func TestSimulatePaidInDev(t *testing.T) {
	t.Setenv("MEMBER_OTP_DEV_CODE", "123456")
	e := setup(t)
	if !e.svc.CanSimulate() {
		t.Skip("TEST_DATABASE_URL is not local; dev bypass inactive")
	}
	pkg := e.pkg("Go Sim", 2, 100_000, 30, nil)
	e.exec(`DELETE FROM configuration.payment_gateways WHERE provider = 'xendit'`)
	pending := e.data(asMember, "POST", "/api/member-portal/gym/credits/purchases", map[string]any{"package_id": pkg, "method": "qris"})
	if pending["simulated"] != true || !strings.HasPrefix(pending["qr_string"].(string), "DEV-SIMULATED-QRIS-gymcp_") {
		t.Fatalf("pending %v", pending)
	}
	paid := e.data(asMember, "POST", "/api/member-portal/gym/credits/purchases/"+pending["id"].(string)+"/simulate-paid", nil)
	if paid["status"] != "paid" {
		t.Fatalf("paid %v", paid)
	}
	if b := e.balance(); b != 2 {
		t.Fatalf("credits %d", b)
	}
}

/* ── Passes, branch prices, public price list ───────────────────────── */

func TestPassesAndPublicPlans(t *testing.T) {
	e := setup(t)
	org := testutil.CreateOrg(t, e.tx)
	c := e.member.CustomerID

	created := e.data(asStaff, "POST", "/api/gym/packages", map[string]any{
		"name": "Go Pass 4 Minggu", "kind": "pass", "price_idr": 1_200_000, "validity_days": 28, "badge": "Paling laris",
		"branch_prices": []map[string]any{{"branch_id": org.BranchID, "price_idr": 1_000_000}},
	})
	pass := created["id"].(string)
	hidden := e.data(asStaff, "POST", "/api/gym/packages", map[string]any{
		"name": "Go Hidden Pack", "credits": 4, "price_idr": 400_000, "validity_days": 30, "is_public": false,
	})["id"].(string)

	var row map[string]any
	for _, p := range e.data(asStaff, "GET", "/api/gym/packages", nil)["packages"].([]any) {
		if m := p.(map[string]any); m["id"] == pass {
			row = m
		}
	}
	prices, _ := row["branch_prices"].([]any)
	if row["kind"] != "pass" || row["credits"] != 0.0 || row["badge"] != "Paling laris" || row["is_public"] != true ||
		len(prices) != 1 || prices[0].(map[string]any)["price_idr"] != 1000000.0 {
		t.Fatalf("pass row %v", row)
	}

	// Public price list: branch price applied, hidden packages left out.
	e.fail(anon, "GET", "/api/public/site/plans?branch=nope", nil, 404, "Cabang tidak ditemukan")
	plans := e.data(anon, "GET", "/api/public/site/plans?branch="+org.BranchID, nil)
	if plans["branch"].(map[string]any)["id"] != org.BranchID {
		t.Fatalf("branch %v", plans["branch"])
	}
	priced := map[string]map[string]any{}
	for _, p := range plans["plans"].([]any) {
		m := p.(map[string]any)
		priced[m["id"].(string)] = m
	}
	if priced[pass]["price_idr"] != 1000000.0 || priced[pass]["kind"] != "pass" || priced[pass]["validity_days"] != 28.0 || priced[hidden] != nil {
		t.Fatalf("plans %v", plans["plans"])
	}
	base := e.data(anon, "GET", "/api/public/site/plans", nil)
	if base["branch"] != nil {
		t.Fatalf("base branch %v", base["branch"])
	}
	for _, p := range base["plans"].([]any) {
		if m := p.(map[string]any); m["id"] == pass && m["price_idr"] != 1200000.0 {
			t.Fatalf("base price %v", m)
		}
	}

	// Selling a pass at the branch takes the branch price and issues a pass
	// instead of credits.
	sale := e.data(asStaff, "POST", "/api/gym/credits/"+c+"/sell", map[string]any{
		"package_id": pass, "payment_method": "cash", "branch_id": org.BranchID,
	})
	if sale["status"] != "paid" || sale["kind"] != "pass" || sale["credits"] != 0.0 || sale["total_idr"] != 1000000.0 {
		t.Fatalf("pass sale %v", sale)
	}
	if b := e.balance(); b != 0 {
		t.Fatalf("balance %d", b)
	}
	page := e.data(asStaff, "GET", "/api/gym/credits/"+c, nil)
	passes := page["passes"].([]any)
	if len(passes) != 1 || passes[0].(map[string]any)["status"] != "active" || passes[0].(map[string]any)["days_left"] != 28.0 ||
		passes[0].(map[string]any)["package_name"] != "Go Pass 4 Minggu" {
		t.Fatalf("passes %v", passes)
	}
	active, err := e.svc.ActivePassAt(e.ctx, e.tx, c, e.svc.clock().Add(27*24*time.Hour))
	if err != nil || active == nil {
		t.Fatalf("active pass %v %v", active, err)
	}
	if later, err := e.svc.ActivePassAt(e.ctx, e.tx, c, e.svc.clock().Add(29*24*time.Hour)); err != nil || later != nil {
		t.Fatalf("pass after expiry %v %v", later, err)
	}
	// Paying twice never issues a second pass.
	if already, _, err := e.svc.markPaid(e.ctx, e.tx, sale["id"].(string), nil); err != nil || !already {
		t.Fatalf("second markPaid %v %v", already, err)
	}

	refunded := e.data(asStaff, "POST", "/api/gym/credits/purchases/"+sale["id"].(string)+"/refund", map[string]any{"reason": "batal ikut"})
	if refunded["status"] != "refunded" {
		t.Fatalf("refund %v", refunded)
	}
	var passStatus string
	e.scalar(&passStatus, `SELECT status FROM gym.member_passes WHERE purchase_id = $1`, sale["id"])
	if passStatus != "refunded" {
		t.Fatalf("pass status %s", passStatus)
	}
	wallet := e.data(asMember, "GET", "/api/member-portal/gym/credits", nil)
	if p := wallet["passes"].([]any)[0].(map[string]any); p["status"] != "refunded" || p["days_left"] != 0.0 {
		t.Fatalf("member passes %v", wallet["passes"])
	}
}

func TestInvoicePurchase(t *testing.T) {
	invoiceStatus := "PENDING"
	xendit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/v2/invoices":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["external_id"] == nil || body["amount"] != 1000000.0 || body["success_redirect_url"] == nil {
				http.Error(w, "bad invoice", 400)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "inv_test_1", "invoice_url": "https://checkout.test/inv_test_1", "expiry_date": "2030-01-01T00:00:00.000Z"})
		case r.URL.Path == "/v2/invoices/inv_test_1":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "inv_test_1", "status": invoiceStatus})
		default:
			http.NotFound(w, r)
		}
	}))
	defer xendit.Close()
	t.Setenv("XENDIT_API_BASE_URL", xendit.URL)
	t.Setenv("XENDIT_SECRET_KEY", "")
	t.Setenv("XENDIT_MOCK", "")

	e := setup(t)
	org := testutil.CreateOrg(t, e.tx)
	var pass string
	e.scalar(&pass, `INSERT INTO gym.credit_packages (name, kind, credits, price_idr, validity_days)
		VALUES ('Go Pass Invoice', 'pass', 0, 1200000, 14) RETURNING id`)
	e.exec(`INSERT INTO gym.package_branch_prices (package_id, branch_id, price_idr) VALUES ($1, $2, 1000000)`, pass, org.BranchID)

	e.fail(asMember, "POST", "/api/member-portal/gym/credits/purchases", map[string]any{"package_id": pass, "method": "invoice"}, 503,
		"Pembayaran online belum tersedia: XENDIT_SECRET_KEY belum dikonfigurasi")

	t.Setenv("XENDIT_SECRET_KEY", "xnd_test")
	pending := e.data(asMember, "POST", "/api/member-portal/gym/credits/purchases", map[string]any{
		"package_id": pass, "method": "invoice", "branch_id": org.BranchID,
	})
	if pending["status"] != "pending" || pending["invoice_url"] != "https://checkout.test/inv_test_1" || pending["total_idr"] != 1000000.0 ||
		pending["kind"] != "pass" || pending["simulated"] != false || pending["expires_at"] != "2030-01-01T00:00:00.000Z" {
		t.Fatalf("invoice purchase %v", pending)
	}
	id := pending["id"].(string)
	still := e.data(asMember, "GET", "/api/member-portal/gym/credits/purchases/"+id, nil)
	if still["status"] != "pending" {
		t.Fatalf("still pending %v", still)
	}
	invoiceStatus = "PAID"
	paid := e.data(asMember, "GET", "/api/member-portal/gym/credits/purchases/"+id, nil)
	if paid["status"] != "paid" || paid["invoice_url"] != nil {
		t.Fatalf("paid %v", paid)
	}
	wallet := e.data(asMember, "GET", "/api/member-portal/gym/credits", nil)
	if p := wallet["passes"].([]any)[0].(map[string]any); p["status"] != "active" || p["days_left"] != 14.0 {
		t.Fatalf("pass %v", wallet["passes"])
	}

	// Mock mode: the invoice URL is the status page and the first poll settles.
	t.Setenv("XENDIT_MOCK", "1")
	pack := e.pkg("Go Mock Pack", 2, 50_000, 30, nil)
	mock := e.data(asMember, "POST", "/api/member-portal/gym/credits/purchases", map[string]any{"package_id": pack, "method": "invoice"})
	mockID := mock["id"].(string)
	if mock["simulated"] != true || !strings.HasSuffix(mock["invoice_url"].(string), "/member/wallet/pay/"+mockID) {
		t.Fatalf("mock purchase %v", mock)
	}
	if settled := e.data(asMember, "GET", "/api/member-portal/gym/credits/purchases/"+mockID, nil); settled["status"] != "paid" {
		t.Fatalf("mock settle %v", settled)
	}
	if b := e.balance(); b != 2 {
		t.Fatalf("credits %d", b)
	}

	// The public /join checkout returns to its own status page.
	site := e.data(asMember, "POST", "/api/member-portal/gym/credits/purchases", map[string]any{"package_id": pack, "method": "invoice", "source": "site"})
	if !strings.HasSuffix(site["invoice_url"].(string), "/join/status?purchase="+site["id"].(string)) {
		t.Fatalf("site purchase %v", site)
	}
}

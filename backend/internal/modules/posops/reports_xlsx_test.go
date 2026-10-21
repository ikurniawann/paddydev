package posops_test

import (
	"bytes"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"nuhabit/backend/internal/app"
	"nuhabit/backend/internal/platform/testutil"
	"nuhabit/backend/internal/platform/xlsx"
)

// download serves a GET as the harness staff inside a savepoint and
// checks the status, keeping the headers and raw body.
func (h *harness) download(path string, status int) *httptest.ResponseRecorder {
	h.t.Helper()
	h.exec("SAVEPOINT request")
	rec, _ := testutil.Do(h.t, h.mux, testutil.AsStaff(testutil.Request("GET", path, nil), h.staff))
	if _, err := h.tx.Exec(h.ctx, "RELEASE SAVEPOINT request"); err != nil {
		h.exec("ROLLBACK TO SAVEPOINT request")
	}
	if rec.Code != status {
		h.t.Fatalf("GET %s: status %d, want %d: %s", path, rec.Code, status, rec.Body.String())
	}
	return rec
}

// workbook checks the xlsx headers and returns the sheet names and a
// reader of one sheet as ExcelJS strings.
func (h *harness) workbook(rec *httptest.ResponseRecorder, filename string, noStore bool) ([]string, func(sheet string) [][]string) {
	h.t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != xlsx.ContentType {
		h.t.Fatalf("content type %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename="`+filename+`"` {
		h.t.Fatalf("content disposition %q, want %s", cd, filename)
	}
	if cc := rec.Header().Get("Cache-Control"); (cc == "no-store") != noStore {
		h.t.Fatalf("cache control %q", cc)
	}
	body := rec.Body.Bytes()
	f, err := excelize.OpenReader(bytes.NewReader(body))
	if err != nil {
		h.t.Fatalf("open xlsx: %v", err)
	}
	names := f.GetSheetList()
	_ = f.Close()
	return names, func(sheet string) [][]string {
		h.t.Helper()
		m, err := xlsx.ParseMatrix(body, xlsx.ReadOptions{PreferSheet: sheet})
		if err != nil {
			h.t.Fatalf("parse %s: %v", sheet, err)
		}
		return m
	}
}

// cells fails unless row starts with want.
func cells(t *testing.T, m [][]string, row int, want ...string) {
	t.Helper()
	if row >= len(m) || len(m[row]) < len(want) || !slices.Equal(m[row][:len(want)], want) {
		var got []string
		if row < len(m) {
			got = m[row]
		}
		t.Fatalf("row %d = %q, want %q", row, got, want)
	}
}

// stallFixture puts the cash sale's Kopi on a purchasing product of a new
// stall; the other items keep no product.
func stallFixture(h *harness, cashOrder string) (stall, kode string) {
	h.t.Helper()
	sfx := strings.ToUpper(testutil.RandomHex(3))
	holding := h.id(`INSERT INTO configuration.holdings (name, code) VALUES ('H GT'||$1, 'HGT'||$1) RETURNING id::text`, sfx)
	company := h.id(`INSERT INTO configuration.companies (holding_id, name, code) VALUES ($1, 'C GT'||$2, 'CGT'||$2) RETURNING id::text`, holding, sfx)
	branch := h.id(`INSERT INTO configuration.branches (company_id, name, code) VALUES ($1, 'B GT'||$2, 'BGT'||$2) RETURNING id::text`, company, sfx)
	stall = h.id(`INSERT INTO configuration.warehouses (branch_id, name, code) VALUES ($1, 'Stall GT', 'STALL-GT'||$2) RETURNING id::text`, branch, sfx)
	kode = "GT-" + sfx
	product := h.id(`INSERT INTO item.products (kode, nama, kategori, harga_jual, warehouse_id) VALUES ($1, 'Kopi', 'Minuman', 15000, $2) RETURNING id::text`, kode, stall)
	pos := h.id(`INSERT INTO pos.pos_products (sku, name, base_price, source_product_id) VALUES ('PUR-'||$1, 'Kopi', 15000, $2) RETURNING id::text`, kode, product)
	h.exec(`UPDATE pos.pos_order_items SET product_id = $1 WHERE order_id = $2`, pos, cashOrder)
	return stall, kode
}

func TestReportXlsxRoutesGuard(t *testing.T) {
	h := newHarness(t, nil)
	for _, path := range []string{"/api/pos/reports/product-sales", "/api/pos/reports/transactions",
		"/api/pos/reports/export?report=transactions", "/api/pos/reports/rush-hour/export"} {
		r := h.anon("GET", path, nil, 401)
		jsonEq(t, r.body, `{"success":false,"error":"Authentication required"}`)
	}
	// A staff member without a pos.* menu gets the same 401.
	outsider := newHarness(t, map[string][]string{"hris.employees": nil})
	r := outsider.call("GET", "/api/pos/reports/export?report=transactions&date_from=2031-05-01&date_to=2031-05-31", nil, 401)
	jsonEq(t, r.body, `{"success":false,"error":"Authentication required"}`)
	outsider.call("GET", "/api/pos/reports/rush-hour/export", nil, 401)
}

func TestProductSalesAndTransactionReports(t *testing.T) {
	h := newHarness(t, nil)
	cash, split, _ := reportFixture(h)
	stall, kode := stallFixture(h, cash)
	month := "date_from=2031-05-01&date_to=2031-05-31"

	r := h.call("GET", "/api/pos/reports/product-sales?date_from=2031-05-09&date_to=2031-05-01", nil, 500)
	jsonEq(t, r.body, `{"success":false,"error":"Tanggal dari tidak boleh melebihi tanggal sampai"}`)
	r = h.call("GET", "/api/pos/reports/transactions?"+month+"&warehouse_id=00000000-0000-4000-8000-000000000001", nil, 500)
	jsonEq(t, r.body, `{"success":false,"error":"Outlet tidak valid atau di luar scope"}`)

	// Product sales: Kopi on the stall, Nasi without a product.
	r = h.call("GET", "/api/pos/reports/product-sales?"+month, nil, 200)
	d := data(r)
	keysInOrder(t, r.raw, "filters", "stall_options", "stall_locked", "summary", "rows")
	jsonEq(t, d["filters"], `{"date_from":"2031-05-01","date_to":"2031-05-31","warehouse_id":null}`)
	jsonEq(t, d["summary"], `{"products":2,"quantity":3,"revenue":55000}`)
	rows := list(d["rows"])
	if len(rows) != 2 {
		t.Fatalf("rows = %s", r.raw)
	}
	kopi := obj(rows[0])
	if kopi["product_name"] != "Kopi" || kopi["product_sku"] != kode || kopi["warehouse_id"] != stall ||
		kopi["stall_name"] != "Stall GT" || kopi["quantity"] != float64(2) || kopi["revenue"] != float64(30000) || kopi["order_count"] != float64(1) {
		t.Fatalf("kopi = %v", kopi)
	}
	jsonEq(t, rows[1], `{"product_id":null,"product_name":"Nasi","product_sku":"GT","warehouse_id":null,"stall_code":null,"stall_name":null,"quantity":1,"revenue":25000,"order_count":1}`)
	r = h.call("GET", "/api/pos/reports/product-sales?"+month+"&warehouse_id="+stall, nil, 200)
	if d = data(r); len(list(d["rows"])) != 1 || obj(d["filters"])["warehouse_id"] != stall {
		t.Fatalf("stall product sales = %s", r.raw)
	}

	// Transactions: two paid orders, newest first.
	r = h.call("GET", "/api/pos/reports/transactions?"+month, nil, 200)
	d = data(r)
	keysInOrder(t, r.raw, "filters", "stall_options", "stall_locked", "summary", "per_stall", "top_products", "daily", "rows")
	jsonEq(t, d["summary"], `{"transactions":2,"total_sales":55000,"total_ark_used":0,"revenue":55000,"discount":5000,"tax":0,"service":0,"nett":55000}`)
	jsonEq(t, d["daily"], `[{"date":"2031-05-04","nett":30000,"transactions":1},{"date":"2031-05-05","nett":25000,"transactions":1}]`)
	stallCode := obj(list(d["per_stall"])[0])["stall_code"]
	jsonEq(t, d["per_stall"], `[{"stall_code":"`+stallCode.(string)+`","stall_name":"Stall GT","transactions":1,"quantity":2,"sales":30000}]`)
	jsonEq(t, d["top_products"], `[{"product_name":"Kopi","quantity":2,"revenue":30000},{"product_name":"Nasi","quantity":1,"revenue":25000}]`)
	txRows := list(d["rows"])
	first, second := obj(txRows[0]), obj(txRows[1])
	if len(txRows) != 2 || first["id"] != split || first["ordered_at"] != "2031-05-05T13:00:00.000Z" || first["discount_amount"] != float64(5000) ||
		first["stall_name"] != nil || second["id"] != cash || second["warehouse_id"] != stall || second["stall_name"] != "Stall GT" {
		t.Fatalf("transaction rows = %s", r.raw)
	}
	keysInOrder(t, r.raw[strings.Index(r.raw, `"rows":`):], "id", "order_number", "ordered_at", "status", "payment_status",
		"payment_method", "payment_method_code", "payment_method_name", "subtotal", "discount_amount", "tax_amount",
		"service_charge_amount", "total_amount", "ark_coins_used", "cashier_id", "warehouse_id", "stall_code", "stall_name",
		"checkout_id", "checkout_number", "sold_from", "xendit_qr_id", "xendit_external_id", "comp_type", "comp_approved_name")
	r = h.call("GET", "/api/pos/reports/transactions?"+month+"&warehouse_id="+stall, nil, 200)
	if rows := list(data(r)["rows"]); len(rows) != 1 || obj(rows[0])["id"] != cash {
		t.Fatalf("stall transactions = %s", r.raw)
	}

	// ?format=xlsx: the same data as plain sheets.
	names, sheet := h.workbook(h.download("/api/pos/reports/product-sales?"+month+"&format=xlsx", 200),
		"penjualan-produk-2031-05-01_2031-05-31.xlsx", true)
	if !slices.Equal(names, []string{"Ringkasan", "Produk"}) {
		t.Fatalf("sheets = %v", names)
	}
	m := sheet("Ringkasan")
	cells(t, m, 0, "Laporan Penjualan Produk")
	cells(t, m, 1, "Periode", "2031-05-01 s/d 2031-05-31")
	cells(t, m, 2, "Outlet", "Semua outlet")
	cells(t, m, 4, "Metrik", "Nilai")
	cells(t, m, 7, "Total omzet", "55000")
	m = sheet("Produk")
	cells(t, m, 0, "Produk", "SKU", "Outlet", "Qty", "Orders", "Omzet")
	cells(t, m, 1, "Kopi", kode, "Stall GT", "2", "1", "30000")
	cells(t, m, 2, "Nasi", "GT", "—", "1", "1", "25000")

	names, sheet = h.workbook(h.download("/api/pos/reports/transactions?"+month+"&format=xlsx&warehouse_id="+stall, 200),
		"transaksi-pos-2031-05-01_2031-05-31.xlsx", true)
	if !slices.Equal(names, []string{"Ringkasan", "Per Outlet", "Top Produk", "Harian", "Transaksi"}) {
		t.Fatalf("sheets = %v", names)
	}
	// The picker mixes branches, so the outlet is named with its branch.
	cells(t, sheet("Ringkasan"), 2, "Outlet", "Stall GT — B GT"+strings.TrimPrefix(kode, "GT-")+" (STALL-GT"+strings.TrimPrefix(kode, "GT-")+")")
	m = sheet("Transaksi")
	cells(t, m, 0, "Order", "Checkout", "Waktu", "Outlet", "Metode", "Subtotal", "Diskon", "Pajak", "Total",
		"Service", "ARK", "Pembayaran", "Asal", "Comp")
	if len(m) != 2 || !strings.HasPrefix(m[1][2], "2031-05-04T03:15:00") {
		t.Fatalf("transaksi = %q", m)
	}
	cells(t, m, 1, m[1][0], "", m[1][2], "Stall GT", "Tunai", "30000", "0", "0", "30000", "0", "0", "Lunas", "Kasir outlet", "")
}

func TestReportExport(t *testing.T) {
	h := newHarness(t, nil)
	cash, _, _ := reportFixture(h)
	stall, kode := stallFixture(h, cash)
	brand := app.PosOpsPorts(h.deps).Directory.BrandName(h.ctx, h.tx, nil)
	month := "&date_from=2031-05-01&date_to=2031-05-31"

	r := h.call("GET", "/api/pos/reports/export?report=rahasia"+month, nil, 400)
	jsonEq(t, r.body, `{"success":false,"error":"Laporan tidak dikenal"}`)
	r = h.call("GET", "/api/pos/reports/export?report=voids&date_from=2031-5-1&date_to=2031-05-31", nil, 400)
	jsonEq(t, r.body, `{"success":false,"error":"Format tanggal harus YYYY-MM-DD"}`)
	r = h.call("GET", "/api/pos/reports/export?report=voids&date_from=2031-05-09&date_to=2031-05-01", nil, 400)
	jsonEq(t, r.body, `{"success":false,"error":"Tanggal dari tidak boleh melebihi tanggal sampai"}`)
	// A report error keeps the status and message of the report's own route.
	badStall := "&warehouse_id=00000000-0000-4000-8000-000000000001"
	r = h.call("GET", "/api/pos/reports/export?report=transactions"+month+badStall, nil, 500)
	jsonEq(t, r.body, `{"success":false,"error":"Outlet tidak valid atau di luar scope"}`)
	r = h.call("GET", "/api/pos/reports/export?report=rush-hour"+month+badStall, nil, 400)
	jsonEq(t, r.body, `{"success":false,"error":"Outlet tidak valid atau di luar scope"}`)

	for _, c := range []struct{ key, file string }{
		{"profit", "profit"}, {"revenue-composition", "revenue-composition"}, {"transactions", "transaksi"},
		{"rush-hour", "rush-hour"}, {"voids", "void"}, {"product-sales", "product-sales"}, {"payment-methods", "payment-methods"},
	} {
		rec := h.download("/api/pos/reports/export?report="+c.key+month, 200)
		names, sheet := h.workbook(rec, c.file+"_2031-05-01_2031-05-31.xlsx", true)
		switch c.key {
		case "profit":
			if !slices.Equal(names, []string{"Ringkasan", "Per Produk", "Per Kategori", "Per Station", "Per Kasir"}) {
				t.Fatalf("profit sheets = %v", names)
			}
			m := sheet("Ringkasan")
			cells(t, m, 0, brand+" — Laporan Profit")
			cells(t, m, 1, "Periode: 1 Mei 2031 s.d. 31 Mei 2031")
			cells(t, m, 2, "Outlet: Semua outlet")
			if !strings.HasPrefix(m[3][0], "Dicetak: ") || !strings.HasSuffix(m[3][0], " WIB") {
				t.Fatalf("printed line = %q", m[3][0])
			}
			cells(t, m, 8, "Omzet", "55000")
			cells(t, m, 14, "Per Tanggal")
			cells(t, m, 15, "Tanggal", "Qty", "Omzet (Rp)", "HPP (Rp)", "Laba Kotor (Rp)", "Margin")
			cells(t, m, 18, "TOTAL", "3", "55000", "10000", "45000", "81.82")
			cells(t, sheet("Per Station"), 0, "Per Station — 1 Mei 2031 s.d. 31 Mei 2031")
		case "revenue-composition":
			if !slices.Equal(names, []string{"Ringkasan", "Harian"}) {
				t.Fatalf("revenue sheets = %v", names)
			}
			m := sheet("Ringkasan")
			cells(t, m, 13, "Food vs Beverage")
			cells(t, m, 16, "Beverage", "2", "30000", "10000", "20000", "33.33", "54.55", "66.67")
		case "transactions":
			if !slices.Equal(names, []string{"Ringkasan", "Harian", "Transaksi"}) {
				t.Fatalf("transactions sheets = %v", names)
			}
			m := sheet("Ringkasan")
			cells(t, m, 5, "Jumlah transaksi", "2")
			cells(t, m, 7, "Diskon", "5000")
			cells(t, m, 13, "Per Outlet")
			cells(t, m, 15, "Stall GT", "STALL-GT"+strings.TrimPrefix(kode, "GT-"), "1", "2", "30000")
			m = sheet("Transaksi")
			cells(t, m, 1, "2 transaksi")
			cells(t, m, 3, "No", "Waktu (WIB)", "No. Order")
			cells(t, m, 4, "1", "05 Mei 2031, 20.00")
			cells(t, m, 5, "2", "04 Mei 2031, 10.15", m[5][2], "", "Stall GT", "cash", "30000")
			cells(t, m, 6, "TOTAL", "", "", "", "", "", "55000", "5000", "0", "0", "55000", "0")
			cells(t, sheet("Harian"), 5, "TOTAL", "2", "55000")
		case "rush-hour":
			if !slices.Equal(names, []string{"Ringkasan", "Per Jam", "Heatmap"}) {
				t.Fatalf("rush hour sheets = %v", names)
			}
			m := sheet("Ringkasan")
			cells(t, m, 10, "Jam omzet tertinggi", "10:00 · Rp 30.000")
			cells(t, m, 11, "Hari tersibuk", "Sen · 1 transaksi")
			m = sheet("Heatmap")
			cells(t, m, 2, "Hari", "08:00", "09:00", "10:00")
			cells(t, m, 9, "Min", "0", "0", "1")
			cells(t, sheet("Per Jam"), 27, "TOTAL", "2", "55000", "3", "27500", "100", "100")
		case "voids":
			if !slices.Equal(names, []string{"Void", "Item Void"}) {
				t.Fatalf("void sheets = %v", names)
			}
			m := sheet("Void")
			cells(t, m, 6, "Nilai void", "12000")
			if row := m[9]; row[0] != "1" || row[3] != "05 Mei 2031, 09.00" || row[4] != "05 Mei 2031, 09.30" || row[5] != "salah input" ||
				row[6] != h.staff.FullName || row[9] != "qris" || row[10] != "12000" {
				t.Fatalf("void row = %q", row)
			}
			cells(t, sheet("Item Void"), 3, m[9][1], "Teh", "GT", "1", "12000", "12000")
		case "product-sales":
			m := sheet("Product Sales")
			cells(t, m, 9, "Rank", "SKU", "Produk", "Outlet", "Qty", "Omzet (Rp)", "Jumlah order", "Kontribusi omzet")
			cells(t, m, 10, "1", kode, "Kopi", "Stall GT", "2", "30000", "1", "54.55")
			cells(t, m, 12, "TOTAL", "", "", "", "3", "55000", "", "100")
		case "payment-methods":
			if !slices.Equal(names, []string{"Ringkasan", "Per Periode"}) {
				t.Fatalf("payment sheets = %v", names)
			}
			m := sheet("Per Periode")
			cells(t, m, 0, "Tren per hari — 1 Mei 2031 s.d. 31 Mei 2031")
			cells(t, m, 2, "Periode", "Total (Rp)", "Pembayaran")
			cells(t, m, 6, "4 Mei 2031", "30000", "1")
			cells(t, m, 34, "TOTAL", "55000", "3")
		}
	}

	// A stall filter names the stall.
	_, sheet := h.workbook(h.download("/api/pos/reports/export?report=voids"+month+"&warehouse_id="+stall, 200),
		"void_2031-05-01_2031-05-31.xlsx", true)
	cells(t, sheet("Void"), 2, "Outlet: Stall GT — B GT"+strings.TrimPrefix(kode, "GT-"))
}

func TestRushHourExport(t *testing.T) {
	h := newHarness(t, nil)
	reportFixture(h)
	brand := app.PosOpsPorts(h.deps).Directory.BrandName(h.ctx, h.tx, nil)
	month := "date_from=2031-05-01&date_to=2031-05-31"

	r := h.call("GET", "/api/pos/reports/rush-hour/export?date_from=2031-05-09&date_to=2031-05-01", nil, 400)
	jsonEq(t, r.body, `{"success":false,"error":"Tanggal dari tidak boleh melebihi tanggal sampai"}`)
	r = h.call("GET", "/api/pos/reports/rush-hour/export?"+month+"&warehouse_id=00000000-0000-4000-8000-000000000001", nil, 400)
	jsonEq(t, r.body, `{"success":false,"error":"Outlet tidak valid atau di luar scope"}`)

	rec := h.download("/api/pos/reports/rush-hour/export?"+month+"&range_from=10&range_to=12", 200)
	names, sheet := h.workbook(rec, "rush-hour-2031-05-01_2031-05-31.xlsx", false)
	if !slices.Equal(names, []string{"Ringkasan", "Per Jam", "Per Hari", "Heatmap"}) {
		t.Fatalf("sheets = %v", names)
	}
	m := sheet("Ringkasan")
	cells(t, m, 0, brand+" — Report Rush Hour")
	cells(t, m, 1, "Periode: 2031-05-01 s.d. 2031-05-31")
	cells(t, m, 2, "Outlet: Semua outlet")
	cells(t, m, 5, "TOTAL KESELURUHAN")
	cells(t, m, 6, "Jumlah transaksi", "2")
	cells(t, m, 7, "Omzet (Rp)", "55000")
	cells(t, m, 8, "Item terjual", "3")
	cells(t, m, 9, "Rata-rata bill (Rp)", "27500")
	cells(t, m, 13, "Jam omzet tertinggi", "10:00", "Rp 30.000")
	cells(t, m, 14, "Hari tersibuk", "Sen", "1 transaksi")
	cells(t, m, 16, "KONTRIBUSI RENTANG JAM 10:00–12:00")
	// The TS merges row 17 (the column header) like the title rows, so only
	// its first cell keeps a value.
	cells(t, m, 17, "Berdasarkan", "", "", "")
	cells(t, m, 18, "Amount / omzet (Rp)", "30000", "55000", "54.5")
	cells(t, m, 19, "Quantity / item terjual", "2", "3", "66.7")
	cells(t, m, 20, "Jumlah transaksi", "1", "2", "50")
	m = sheet("Per Jam")
	cells(t, m, 1, "Periode: 2031-05-01 s.d. 2031-05-31 · Outlet: Semua outlet")
	cells(t, m, 3, "Jam", "Transaksi", "Omzet (Rp)", "Item Terjual", "Rata-rata Bill (Rp)", "% Omzet", "% Item")
	cells(t, m, 14, "10:00", "1", "30000", "2", "30000", "54.5", "66.7")
	cells(t, m, 28, "TOTAL", "2", "55000", "3", "27500", "100", "100")
	cells(t, sheet("Per Hari"), 4, "Sen", "1", "25000")
	m = sheet("Heatmap")
	cells(t, m, 3, `Hari \ Jam`, "08:00", "09:00", "10:00")
	cells(t, m, 10, "Min", "0", "0", "1")

	// Without range_from/range_to, Number(null) is hour 0 for both ends.
	_, sheet = h.workbook(h.download("/api/pos/reports/rush-hour/export?"+month, 200), "rush-hour-2031-05-01_2031-05-31.xlsx", false)
	cells(t, sheet("Ringkasan"), 16, "KONTRIBUSI RENTANG JAM 00:00–00:00")
}

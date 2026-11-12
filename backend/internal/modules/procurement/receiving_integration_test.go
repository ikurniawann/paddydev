package procurement_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/modules/procurement"
	"nuhabit/backend/internal/platform/outbox"
)

// warehouse creates an active company → branch → warehouse chain.
func (e *env) warehouse() (company, branch, warehouse string) {
	s := suffix()
	holding := e.id(`SELECT id::text FROM configuration.holdings LIMIT 1`)
	company = e.id(`INSERT INTO configuration.companies (holding_id, name, code) VALUES ($1, 'Co '||$2, 'GTC'||$2) RETURNING id::text`, holding, s)
	branch = e.id(`INSERT INTO configuration.branches (company_id, name, code) VALUES ($1, 'Br '||$2, 'GTB'||$2) RETURNING id::text`, company, s)
	warehouse = e.id(`INSERT INTO configuration.warehouses (branch_id, name, code) VALUES ($1, 'Gudang '||$2, 'GTW'||$2) RETURNING id::text`, branch, s)
	return
}

// events collects the payloads published on topics during a test.
type events struct {
	bus *outbox.Bus
	got map[string][]map[string]any
}

func (e *env) listen(topics ...string) *events {
	ev := &events{bus: outbox.NewBus(nil, nil), got: map[string][]map[string]any{}}
	for _, topic := range topics {
		topic := topic
		ev.bus.Subscribe(topic, "procurement-test."+topic, func(_ context.Context, _ pgx.Tx, o outbox.Event) error {
			var p map[string]any
			_ = o.Decode(&p)
			ev.got[topic] = append(ev.got[topic], p)
			return nil
		})
	}
	if err := ev.bus.Register(e.ctx, e.tx); err != nil {
		e.t.Fatal(err)
	}
	return ev
}

func (ev *events) dispatch(e *env) {
	e.t.Helper()
	if _, err := ev.bus.Dispatch(e.ctx, e.tx); err != nil {
		e.t.Fatal(err)
	}
}

func TestRawMaterialReceivingFlow(t *testing.T) {
	e := newEnv(t)
	f := e.fixtures()
	staff := e.staff(itemsAll)
	_, _, wh := e.warehouse()
	ev := e.listen("procurement.grn.stock_received", "procurement.grn.posted")

	poID := e.id(`INSERT INTO purchasing.purchase_orders (nomor_po, supplier_id, status, ppn_persen) VALUES ('GT-PO-'||$1, $2, 'sent', 0) RETURNING id::text`, suffix(), f.Supplier)
	poItem := e.id(`INSERT INTO purchasing.purchase_order_items (purchase_order_id, raw_material_id, qty_ordered, satuan_id, harga_satuan)
		VALUES ($1, $2, 10, $3, 1000) RETURNING id::text`, poID, f.Material, f.BigUnit)

	// Delivery: validation, then create.
	e.expect(e.do(&staff, "POST", "/api/purchasing/delivery", map[string]any{"po_id": poID, "tanggal_kirim": "2026-10-04", "no_surat_jalan": "SJ-1", "tanggal_estimasi_tiba": "2026-10-05"}),
		400, "Validation failed")
	r := e.do(&staff, "POST", "/api/purchasing/delivery", map[string]any{"po_id": poID, "supplier_id": f.Supplier, "tanggal_kirim": "2026-10-04", "no_surat_jalan": "SJ-1", "tanggal_estimasi_tiba": "2026-10-05"})
	e.expect(r, 201, "")
	if r.Body["message"] != "Delivery created successfully" || r.data()["status"] != "pending" || r.data()["supplier_id"] != f.Supplier {
		t.Fatalf("delivery = %s", r.Raw)
	}
	deliveryID := r.data()["id"].(string)
	e.expect(e.do(&staff, "POST", "/api/purchasing/delivery", map[string]any{"po_id": poID, "supplier_id": f.Supplier, "tanggal_kirim": "2026-10-04", "no_surat_jalan": "SJ-2", "tanggal_estimasi_tiba": "2026-10-05"}),
		400, "This purchase order already has an open delivery in progress.")

	r = e.do(&staff, "GET", "/api/purchasing/delivery?po_id="+poID, nil)
	e.expect(r, 200, "")
	if len(r.list()) != 1 || r.list()[0].(map[string]any)["no_surat_jalan"] != "SJ-1" || r.list()[0].(map[string]any)["ekspedisi"] != "-" {
		t.Fatalf("delivery list = %s", r.Raw)
	}
	e.expect(e.do(&staff, "GET", "/api/purchasing/delivery?limit=500", nil), 400, "Parameter tidak valid")
	r = e.do(&staff, "GET", "/api/purchasing/delivery/"+deliveryID, nil)
	e.expect(r, 200, "")
	if r.data()["supplier"].(map[string]any)["id"] != f.Supplier || r.data()["purchase_order"].(map[string]any)["status"] != "sent" || r.data()["vendor"] != nil {
		t.Fatalf("delivery detail = %s", r.Raw)
	}
	r = e.do(&staff, "PUT", "/api/purchasing/delivery/"+deliveryID, map[string]any{"ekspedisi": "JNE", "status": "shipped"})
	e.expect(r, 200, "")
	if r.data()["kurir"] != "JNE" || r.data()["status"] != "shipped" {
		t.Fatalf("delivery update = %s", r.Raw)
	}
	e.expect(e.do(&staff, "PUT", "/api/purchasing/delivery/"+deliveryID, map[string]any{"status": "pending"}), 400,
		"Invalid delivery transition: shipped → pending. Allowed: in_transit, cancelled")
	e.expect(e.do(&staff, "DELETE", "/api/purchasing/delivery/"+deliveryID, nil), 400,
		`Delivery berstatus "shipped" — hanya delivery berstatus PENDING atau CANCELLED yang dapat dihapus`)
	r = e.do(&staff, "GET", "/api/purchasing/delivery/for-grn", nil)
	e.expect(r, 200, "")
	found := false
	for _, d := range r.list() {
		if d.(map[string]any)["id"] == deliveryID {
			found = true
			if d.(map[string]any)["supplier_name"] == "-" || d.(map[string]any)["delivery_number"] != "SJ-1" {
				t.Fatalf("for-grn row = %v", d)
			}
		}
	}
	if !found {
		t.Fatal("delivery missing from for-grn")
	}

	// GRN: over-receipt is rejected with the id-ID formatted limit.
	line := map[string]any{"purchase_order_item_id": poItem, "raw_material_id": f.Material, "satuan_id": f.BigUnit, "qty_diterima": 8, "qty_ditolak": 3}
	e.expect(e.do(&staff, "POST", "/api/purchasing/grn", map[string]any{"delivery_id": deliveryID, "warehouse_id": wh, "items": []any{line}}),
		400, "Qty item ini melebihi sisa PO. Maksimal 10, tetapi diinput 11 (diterima + ditolak).")
	line["qty_ditolak"] = 2
	line["qty_accepted"] = 6
	e.expect(e.do(&staff, "POST", "/api/purchasing/grn", map[string]any{"delivery_id": deliveryID, "warehouse_id": wh, "items": []any{line}}),
		400, "Validation failed")
	line["qty_accepted"], line["qty_rejected"] = 7, 1
	r = e.do(&staff, "POST", "/api/purchasing/grn", map[string]any{"delivery_id": deliveryID, "warehouse_id": wh, "items": []any{line}, "catatan": "ok"})
	e.expect(r, 201, "")
	grn := r.data()
	if !regexp.MustCompile(`^GRN-`+today()+`-\d{4}$`).MatchString(grn["nomor_grn"].(string)) ||
		r.Body["message"] != "GRN "+grn["nomor_grn"].(string)+" berhasil dibuat — QC selesai dan stok sudah diperbarui" ||
		grn["status"] != "partially_received" || grn["total_item_diterima"] != float64(8) || grn["supplier_id"] != f.Supplier {
		t.Fatalf("grn = %s", r.Raw)
	}
	grnID := grn["id"].(string)
	if e.scalar(`SELECT qty_received::float8 FROM purchasing.purchase_order_items WHERE id = $1`, poItem) != float64(7) ||
		e.scalar(`SELECT status FROM purchasing.purchase_orders WHERE id = $1`, poID) != "partially_received" ||
		e.scalar(`SELECT status FROM purchasing.deliveries WHERE id = $1`, deliveryID) != "delivered" {
		t.Fatal("PO/delivery not updated")
	}
	if e.scalar(`SELECT count(*)::int FROM audit.audit_log WHERE entity_id = $1 AND action = 'grn.post'`, grnID) != int32(1) {
		t.Fatal("grn audit missing")
	}
	ev.dispatch(e)
	stock := ev.got["procurement.grn.stock_received"]
	if len(stock) != 1 || len(ev.got["procurement.grn.posted"]) != 1 {
		t.Fatalf("events = %v", ev.got)
	}
	sl := stock[0]["lines"].([]any)[0].(map[string]any)
	// 7 kg at Rp 1.000/kg → 7000 g at Rp 1/g (konversi_factor 1000).
	if sl["raw_material_id"] != f.Material || sl["qty"] != float64(7000) || sl["unit_cost"] != float64(1) || sl["warehouse_id"] != wh {
		t.Fatalf("stock line = %v", sl)
	}
	e.expect(e.do(&staff, "POST", "/api/purchasing/grn", map[string]any{"delivery_id": deliveryID, "warehouse_id": wh, "items": []any{line}}), 400, "")

	// Reads.
	r = e.do(&staff, "GET", "/api/purchasing/grn?po_id="+poID, nil)
	e.expect(r, 200, "")
	if r.Body["message"] != "GRN list retrieved" || len(r.list()) != 1 || r.list()[0].(map[string]any)["po_number"] == poID ||
		r.Body["pagination"].(map[string]any)["totalPages"] != float64(1) {
		t.Fatalf("grn list = %s", r.Raw)
	}
	r = e.do(&staff, "GET", "/api/purchasing/grn/"+grnID, nil)
	e.expect(r, 200, "")
	d := r.data()
	item := d["items"].([]any)[0].(map[string]any)
	if d["delivery_number"] != "SJ-1" || d["po_status"] != "partially_received" || d["supplier"] == nil ||
		item["raw_material"].(map[string]any)["satuan_besar"].(map[string]any)["id"] != f.BigUnit ||
		item["purchase_order_item"].(map[string]any)["qty_ordered"] != float64(10) {
		t.Fatalf("grn detail = %s", r.Raw)
	}
	e.expect(e.do(&staff, "GET", "/api/purchasing/grn/11111111-1111-4111-8111-111111111111", nil), 404, "GRN tidak ditemukan")
	r = e.do(&staff, "GET", "/api/purchasing/grn/"+grnID+"/items", nil)
	e.expect(r, 200, "")
	if it := r.list()[0].(map[string]any); it["raw_material"] == nil || it["purchase_order_item"] == nil {
		t.Fatalf("grn items = %s", r.Raw)
	}
	r = e.do(&staff, "GET", "/api/purchasing/grn/"+grnID+"/returnable-items", nil)
	e.expect(r, 200, "")
	if rows := r.list(); len(rows) != 1 || rows[0].(map[string]any)["qty_available_to_return"] != float64(7) || rows[0].(map[string]any)["unit_price"] != float64(1000) {
		t.Fatalf("returnable = %s", r.Raw)
	}
	r = e.do(&staff, "GET", "/api/purchasing/grn/"+grnID+"/vendor-credits", nil)
	e.expect(r, 200, "")
	e.expect(e.do(&staff, "POST", "/api/purchasing/grn/"+grnID+"/qc", map[string]any{"items": []any{map[string]any{"grn_item_id": item["id"], "raw_material_id": f.Material, "qty_inspected": 1, "qty_accepted": 1, "qty_rejected": 0}}}),
		400, "GRN is not awaiting quality control")
	e.expect(e.do(&staff, "POST", "/api/purchasing/grn/"+grnID+"/items", map[string]any{"qty_diterima": 1}), 400, "Cannot add items to GRN that is not pending")
	e.expect(e.do(&staff, "PATCH", "/api/purchasing/grn", map[string]any{}), 400, "Use /api/purchasing/grn/[id] for updates")

	// Workspace lists the delivery and the GRN.
	r = e.do(&staff, "GET", "/api/purchasing/receiving-workspace", nil)
	e.expect(r, 200, "")
	if ws := r.data(); len(ws["deliveries"].([]any)) == 0 || len(ws["grns"].([]any)) == 0 {
		t.Fatalf("workspace = %s", r.Raw)
	}

	// PATCH (GRN Continue): an over-receipt is checked against the remainder.
	name := e.scalar(`SELECT nama FROM item.raw_materials WHERE id = $1`, f.Material).(string)
	e.expect(e.do(&staff, "PATCH", "/api/purchasing/grn/"+grnID, map[string]any{"items": []any{map[string]any{"purchase_order_item_id": poItem, "raw_material_id": f.Material, "qty_diterima": 20, "qty_ditolak": 2}}}),
		400, "Qty "+name+" melebihi sisa PO. Maksimal 3 untuk penerimaan tambahan, tetapi diinput 12 (diterima + ditolak).")
	r = e.do(&staff, "PATCH", "/api/purchasing/grn/"+grnID, map[string]any{"catatan": "revisi", "items": []any{map[string]any{"purchase_order_item_id": poItem, "raw_material_id": f.Material, "qty_diterima": 9, "qty_ditolak": 1}}})
	e.expect(r, 200, "")
	if r.data()["status"] != "pending" || r.data()["catatan"] != "revisi" || r.Body["message"] != "GRN "+grn["nomor_grn"].(string)+" berhasil diupdate" {
		t.Fatalf("patch = %s", r.Raw)
	}
	r = e.do(&staff, "DELETE", "/api/purchasing/grn/"+grnID, nil)
	e.expect(r, 200, "")
	if r.data()["is_active"] != false || e.scalar(`SELECT qty_received::float8 FROM purchasing.purchase_order_items WHERE id = $1`, poItem) != float64(0) {
		t.Fatalf("delete = %s", r.Raw)
	}
}

func TestGeneralReceiptAndDeliveryArrival(t *testing.T) {
	e := newEnv(t)
	f := e.fixtures()
	staff := e.staff(itemsAll)
	_, _, wh := e.warehouse()
	ev := e.listen("procurement.grn.stock_received", "procurement.grn.posted")
	supply := e.supplyItem()

	poID := e.id(`INSERT INTO purchasing.purchase_orders (nomor_po, vendor_id, status, module_type) VALUES ('GT-PO-'||$1, $2, 'approved', 'general') RETURNING id::text`, suffix(), f.Vendor)
	poItem := e.id(`INSERT INTO purchasing.purchase_order_items (purchase_order_id, supply_item_id, qty_ordered, harga_satuan) VALUES ($1, $2, 4, 2500) RETURNING id::text`, poID, supply)
	e.expect(e.do(&staff, "POST", "/api/purchasing/grn", map[string]any{"warehouse_id": wh, "items": []any{map[string]any{"supply_item_id": supply, "qty_diterima": 4, "qty_ditolak": 0}}}),
		400, "Validation failed")
	r := e.do(&staff, "POST", "/api/purchasing/grn", map[string]any{"po_id": poID, "module_type": "general", "warehouse_id": wh,
		"items": []any{map[string]any{"purchase_order_item_id": poItem, "supply_item_id": supply, "qty_diterima": 4, "qty_ditolak": 0}}})
	e.expect(r, 201, "")
	if r.data()["status"] != "received" || r.data()["vendor_id"] != f.Vendor || r.Body["message"] != "GRN "+r.data()["nomor_grn"].(string)+" berhasil dibuat" {
		t.Fatalf("general grn = %s", r.Raw)
	}
	if e.scalar(`SELECT status FROM purchasing.purchase_orders WHERE id = $1`, poID) != "received" {
		t.Fatal("PO not received")
	}
	ev.dispatch(e)
	if lines := ev.got["procurement.grn.stock_received"]; len(lines) != 1 || lines[0]["lines"].([]any)[0].(map[string]any)["unit_cost"] != float64(2500) {
		t.Fatalf("supply stock = %v", ev.got)
	}
	if len(ev.got["procurement.grn.posted"]) != 1 {
		t.Fatalf("journal events = %v", ev.got)
	}

	// Arrival opens a pending GRN on a raw material delivery.
	po2 := e.id(`INSERT INTO purchasing.purchase_orders (nomor_po, supplier_id, status) VALUES ('GT-PO-'||$1, $2, 'sent') RETURNING id::text`, suffix(), f.Supplier)
	delivery := e.id(`INSERT INTO purchasing.deliveries (purchase_order_id, supplier_id, status, no_surat_jalan) VALUES ($1, $2, 'in_transit', 'SJ-A') RETURNING id::text`, po2, f.Supplier)
	r = e.do(&staff, "POST", "/api/purchasing/delivery/"+delivery+"/arrive", map[string]any{"notes": "tiba"})
	e.expect(r, 200, "")
	g := r.data()["grn"].(map[string]any)
	if r.data()["delivery"].(map[string]any)["status"] != "delivered" || g["status"] != "pending" || g["catatan"] != "tiba" ||
		r.Body["message"] != "Barang arrived — GRN "+g["nomor_grn"].(string)+" berhasil dibuat" {
		t.Fatalf("arrive = %s", r.Raw)
	}
	e.expect(e.do(&staff, "POST", "/api/purchasing/delivery/"+delivery+"/arrive", nil), 400,
		"Invalid delivery transition: delivered → delivered. Allowed: none")

	// PO options: the sent PO has no open delivery any more.
	r = e.do(&staff, "GET", "/api/purchasing/delivery/po-options", nil)
	e.expect(r, 200, "")
	for _, o := range r.list() {
		if o.(map[string]any)["id"] == po2 && (o.(map[string]any)["has_open_delivery"] != false || o.(map[string]any)["active_delivery_status"] != "delivered") {
			t.Fatalf("po option = %v", o)
		}
	}

	// Legacy /deliveries writes.
	e.expect(e.do(&staff, "POST", "/api/purchasing/deliveries", map[string]any{}), 400, "Validation failed")
	e.expect(e.do(&staff, "POST", "/api/purchasing/deliveries", map[string]any{"po_id": "nope"}), 404, "Purchase order not found")
	r = e.do(&staff, "PUT", "/api/purchasing/deliveries/"+delivery, map[string]any{"kurir": "SiCepat"})
	e.expect(r, 200, "")
	if r.data()["kurir"] != "SiCepat" {
		t.Fatalf("legacy put = %s", r.Raw)
	}
	e.expect(e.do(&staff, "PUT", "/api/purchasing/deliveries/"+delivery, []any{}), 400, "Validation failed")
	r = e.do(&staff, "DELETE", "/api/purchasing/deliveries/"+delivery, nil)
	e.expect(r, 200, "")
	if e.scalar(`SELECT status FROM purchasing.deliveries WHERE id = $1`, delivery) != "CANCELLED" {
		t.Fatal("legacy cancel")
	}
}

func TestLegacyQcSubmit(t *testing.T) {
	e := newEnv(t)
	f := e.fixtures()
	staff := e.staff(itemsAll)
	_, _, wh := e.warehouse()
	poID := e.id(`INSERT INTO purchasing.purchase_orders (nomor_po, supplier_id, status) VALUES ('GT-PO-'||$1, $2, 'sent') RETURNING id::text`, suffix(), f.Supplier)
	poItem := e.id(`INSERT INTO purchasing.purchase_order_items (purchase_order_id, raw_material_id, qty_ordered, harga_satuan) VALUES ($1, $2, 5, 100) RETURNING id::text`, poID, f.Material)
	grnID := e.id(`INSERT INTO purchasing.grn (nomor_grn, purchase_order_id, supplier_id, status) VALUES ('GT-GRN-'||$1, $2, $3, 'pending') RETURNING id::text`, suffix(), poID, f.Supplier)
	itemID := e.id(`INSERT INTO purchasing.grn_items (grn_id, purchase_order_item_id, raw_material_id, qty_diterima, warehouse_id) VALUES ($1, $2, $3, 5, $4) RETURNING id::text`, grnID, poItem, f.Material, wh)

	e.expect(e.do(&staff, "POST", "/api/purchasing/qc", map[string]any{"grn_id": grnID, "items": []any{map[string]any{"grn_item_id": itemID}}}),
		400, "raw_material_id atau bahan_baku_id wajib diisi per item")
	r := e.do(&staff, "POST", "/api/purchasing/qc", map[string]any{"grn_id": grnID, "items": []any{map[string]any{"grn_item_id": itemID, "bahan_baku_id": f.Material, "jumlah_diterima": 5}}})
	e.expect(r, 201, "")
	// Accepting the full order on the first QC receives the GRN.
	if r.Body["message"] != "QC submitted successfully" || r.data()["grn_status"] != "received" || r.data()["total_accepted"] != float64(5) {
		t.Fatalf("legacy qc = %s", r.Raw)
	}
	if e.scalar(`SELECT status FROM purchasing.grn_qc_inspections WHERE grn_id = $1`, grnID) != "approved" ||
		e.scalar(`SELECT status FROM purchasing.purchase_orders WHERE id = $1`, poID) != "received" ||
		e.scalar(`SELECT status FROM purchasing.grn WHERE id = $1`, grnID) != "received" {
		t.Fatal("inspection status")
	}
}

func TestApPaymentVoidedVoidsVendorPayment(t *testing.T) {
	e := newEnv(t)
	f := e.fixtures()
	bus := outbox.NewBus(nil, nil)
	procurement.Subscribe(bus, procurement.Ports{})
	if err := bus.Register(e.ctx, e.tx); err != nil {
		t.Fatal(err)
	}
	poID := e.id(`INSERT INTO purchasing.purchase_orders (nomor_po, supplier_id, status) VALUES ('GT-PO-'||$1, $2, 'received') RETURNING id::text`, suffix(), f.Supplier)
	vp := e.id(`INSERT INTO purchasing.vendor_payments (payment_number, purchase_order_id, supplier_id, payment_date, amount, method, status)
		VALUES ('GT-VP-'||$1, $2, $3, '2026-10-04', 1000, 'bank_transfer', 'posted') RETURNING id::text`, suffix(), poID, f.Supplier)
	payload := map[string]any{"payment_id": "x", "payment_no": "APP-1", "vendor_payment_id": vp, "user_id": "", "reason": "salah transfer"}
	for i := 0; i < 2; i++ {
		if err := outbox.Publish(e.ctx, e.tx, "accounting.ap_payment.voided", vp, payload); err != nil {
			t.Fatal(err)
		}
		if _, err := bus.Dispatch(e.ctx, e.tx); err != nil {
			t.Fatal(err)
		}
	}
	if e.scalar(`SELECT status||'|'||void_reason FROM purchasing.vendor_payments WHERE id = $1`, vp) != "void|salah transfer" {
		t.Fatal("vendor payment not voided")
	}
	if n := e.scalar(`SELECT count(*)::int FROM platform.outbox_deliveries d JOIN platform.outbox_events ev ON ev.id = d.event_id
		WHERE ev.key = $1 AND d.subscriber = 'procurement.void-vendor-payment' AND d.dead_at IS NULL AND d.delivered_at IS NOT NULL`, vp); n != int32(2) {
		t.Fatalf("deliveries = %v", n)
	}
}

func TestQcInspectionReads(t *testing.T) {
	e := newEnv(t)
	f := e.fixtures()
	staff := e.staff(itemsAll)
	_, _, wh := e.warehouse()
	poID := e.id(`INSERT INTO purchasing.purchase_orders (nomor_po, supplier_id, status) VALUES ('GT-PO-'||$1, $2, 'sent') RETURNING id::text`, suffix(), f.Supplier)
	poItem := e.id(`INSERT INTO purchasing.purchase_order_items (purchase_order_id, raw_material_id, qty_ordered, harga_satuan) VALUES ($1, $2, 5, 100) RETURNING id::text`, poID, f.Material)
	nomor := "GT-GRN-" + suffix()
	grnID := e.id(`INSERT INTO purchasing.grn (nomor_grn, purchase_order_id, supplier_id, status) VALUES ($1, $2, $3, 'pending') RETURNING id::text`, nomor, poID, f.Supplier)
	itemID := e.id(`INSERT INTO purchasing.grn_items (grn_id, purchase_order_item_id, raw_material_id, qty_diterima, warehouse_id) VALUES ($1, $2, $3, 5, $4) RETURNING id::text`, grnID, poItem, f.Material, wh)

	r := e.do(&staff, "GET", "/api/purchasing/grn/"+grnID+"/qc", nil)
	e.expect(r, 200, "")
	if r.Body["data"] != nil || r.Body["message"] != "No QC inspection yet" {
		t.Fatalf("grn qc before = %s", r.Raw)
	}
	r = e.do(&staff, "POST", "/api/purchasing/qc", map[string]any{"grn_id": grnID, "items": []any{map[string]any{"grn_item_id": itemID, "bahan_baku_id": f.Material, "jumlah_diterima": 4, "jumlah_ditolak": 1}}})
	e.expect(r, 201, "")
	inspection := r.data()["inspection_id"].(string)

	r = e.do(&staff, "GET", "/api/purchasing/qc?search="+nomor, nil)
	e.expect(r, 200, "")
	if len(r.list()) != 1 || r.Body["pagination"].(map[string]any)["total"] != float64(1) {
		t.Fatalf("qc list = %s", r.Raw)
	}
	row := r.list()[0].(map[string]any)
	inspector, _ := row["inspector"].(map[string]any)
	item := row["items"].([]any)[0].(map[string]any)
	if row["id"] != inspection || row["qc_number"] != inspection || row["grn_number"] != nomor || row["goods_receipt_id"] != grnID ||
		row["jumlah_diperiksa"] != float64(5) || row["jumlah_diterima"] != float64(4) || row["jumlah_ditolak"] != float64(1) ||
		row["hasil"] != "partial" || row["status"] != "PARTIAL" || row["rekomendasi"] != "REWORK" || row["bahan_baku_id"] != f.Material ||
		inspector["name"] != staff.FullName || inspector["email"] != staff.Email ||
		item["jumlah_diterima"] != float64(4) || item["raw_material"].(map[string]any)["id"] != f.Material {
		t.Fatalf("qc row = %s", r.Raw)
	}
	r = e.do(&staff, "GET", "/api/purchasing/qc?search=tidak-ada-"+suffix(), nil)
	e.expect(r, 200, "")
	if len(r.list()) != 0 {
		t.Fatalf("qc search = %s", r.Raw)
	}

	r = e.do(&staff, "GET", "/api/purchasing/qc/"+inspection, nil)
	e.expect(r, 200, "")
	d := r.data()
	if d["id"] != inspection || d["grn_number"] != nomor || d["grn"].(map[string]any)["nomor_grn"] != nomor || d["status"] != "PARTIAL" ||
		d["rekomendasi"] != "REWORK" || d["bahan_baku"].(map[string]any)["id"] != f.Material || d["jumlah_diterima"] != float64(4) ||
		d["items"].([]any)[0].(map[string]any)["qty_accepted"] != float64(4) {
		t.Fatalf("qc detail = %s", r.Raw)
	}
	e.expect(e.do(&staff, "GET", "/api/purchasing/qc/11111111-1111-4111-8111-111111111111", nil), 404, "QC inspection tidak ditemukan")

	r = e.do(&staff, "GET", "/api/purchasing/grn/"+grnID+"/qc", nil)
	e.expect(r, 200, "")
	d = r.data()
	item = d["items"].([]any)[0].(map[string]any)
	if r.Body["message"] != "QC inspection retrieved" || d["id"] != inspection || d["status"] != "partial" ||
		d["inspector"].(map[string]any)["id"] != staff.UserID || item["item_status"] != "partially_rejected" ||
		item["raw_material"].(map[string]any)["kode"] == nil {
		t.Fatalf("grn qc = %s", r.Raw)
	}
}

func TestLegacyDeliveries(t *testing.T) {
	e := newEnv(t)
	f := e.fixtures()
	staff := e.staff(itemsAll)
	nomor := "GT-PO-" + suffix()
	poID := e.id(`INSERT INTO purchasing.purchase_orders (nomor_po, supplier_id, status) VALUES ($1, $2, 'sent') RETURNING id::text`, nomor, f.Supplier)

	r := e.do(&staff, "POST", "/api/purchasing/deliveries", map[string]any{"po_id": poID, "no_surat_jalan": "SJ-L"})
	e.expect(r, 200, "")
	d := r.data()
	if d["purchase_order_id"] != poID || d["supplier_id"] != f.Supplier || d["status"] != "IN_TRANSIT" || d["no_surat_jalan"] != "SJ-L" ||
		d["po"].(map[string]any)["nomor_po"] != nomor || d["supplier"].(map[string]any)["id"] != f.Supplier {
		t.Fatalf("legacy create = %s", r.Raw)
	}
	if _, ok := d["po_id"]; ok {
		t.Fatalf("po_id leaked into the row: %s", r.Raw)
	}
	id := d["id"].(string)

	r = e.do(&staff, "GET", "/api/purchasing/deliveries?po_id="+poID, nil)
	e.expect(r, 200, "")
	if len(r.list()) != 1 || r.list()[0].(map[string]any)["id"] != id || r.list()[0].(map[string]any)["po"].(map[string]any)["status"] != "sent" {
		t.Fatalf("legacy list = %s", r.Raw)
	}
	r = e.do(&staff, "GET", "/api/purchasing/deliveries?status=delivered&po_id="+poID, nil)
	e.expect(r, 200, "")
	if len(r.list()) != 0 {
		t.Fatalf("legacy status filter = %s", r.Raw)
	}
	r = e.do(&staff, "GET", "/api/purchasing/deliveries/"+id, nil)
	e.expect(r, 200, "")
	if r.data()["id"] != id || r.data()["po"].(map[string]any)["tanggal_po"] == nil || r.data()["supplier"].(map[string]any)["nama_supplier"] == nil {
		t.Fatalf("legacy detail = %s", r.Raw)
	}
	e.expect(e.do(&staff, "GET", "/api/purchasing/deliveries/11111111-1111-4111-8111-111111111111", nil), 404, "Delivery not found")

	// A vendor PO hands its vendor to the delivery.
	vendorPo := e.id(`INSERT INTO purchasing.purchase_orders (nomor_po, vendor_id, status, module_type) VALUES ('GT-PO-'||$1, $2, 'sent', 'general') RETURNING id::text`, suffix(), f.Vendor)
	r = e.do(&staff, "POST", "/api/purchasing/deliveries", map[string]any{"po_id": vendorPo})
	e.expect(r, 200, "")
	if r.data()["vendor_id"] != f.Vendor || r.data()["supplier"] != nil {
		t.Fatalf("vendor delivery = %s", r.Raw)
	}
}

// EPIC-105 R3 / Review Focus 2: a warehouse with an active branch and
// company keeps its own scope; one without gets a clear 400 instead of the
// old hard-coded MIZU-BANDUNG fallback.
func TestReceivingScopeNoBusiness(t *testing.T) {
	e := newEnv(t)
	f := e.fixtures()
	staff := e.staff(itemsAll)
	supply := e.supplyItem()
	grn := func(wh string) resp {
		po := e.id(`INSERT INTO purchasing.purchase_orders (nomor_po, vendor_id, status, module_type) VALUES ('GT-PO-'||$1, $2, 'approved', 'general') RETURNING id::text`, suffix(), f.Vendor)
		item := e.id(`INSERT INTO purchasing.purchase_order_items (purchase_order_id, supply_item_id, qty_ordered, harga_satuan) VALUES ($1, $2, 2, 1000) RETURNING id::text`, po, supply)
		return e.do(&staff, "POST", "/api/purchasing/grn", map[string]any{"po_id": po, "module_type": "general", "warehouse_id": wh,
			"items": []any{map[string]any{"purchase_order_item_id": item, "supply_item_id": supply, "qty_diterima": 2, "qty_ditolak": 0}}})
	}
	company, branch, active := e.warehouse()
	r := grn(active)
	e.expect(r, 201, "")
	if got := e.scalar(`SELECT company_id::text || '/' || branch_id::text FROM purchasing.grn WHERE id = $1`, r.data()["id"]); got != company+"/"+branch {
		t.Fatalf("active warehouse scope %v, want %s/%s", got, company, branch)
	}
	_, closedBranch, orphan := e.warehouse()
	e.exec(`UPDATE configuration.branches SET is_active = false WHERE id = $1`, closedBranch)
	e.expect(grn(orphan), 400, "Gudang tidak terhubung ke toko dan perusahaan yang aktif")
}

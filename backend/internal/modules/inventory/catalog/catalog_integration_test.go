package catalog_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"nuhabit/backend/internal/modules/inventory/catalog"
	"nuhabit/backend/internal/modules/inventory/kit"
	"nuhabit/backend/internal/modules/inventory/kit/kittest"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/testutil"
)

// Integration tests on TEST_DATABASE_URL in one rolled-back transaction.
// Response shapes were also diffed against the TS routes on seeded data.

type fakePos struct{ synced []string }

func (f *fakePos) SyncProduct(_ context.Context, _ database.Querier, productID, station string, cost *float64) (*kit.Row, error) {
	f.synced = append(f.synced, productID+"|"+station)
	return kit.Obj("mode", "created", "margin_percentage", 50.0), nil
}

func (f *fakePos) VariantCounts(context.Context, database.Querier, []string) (map[string]int, error) {
	return map[string]int{}, nil
}

type noGrns struct{}

func (noGrns) SupplierGrnIDs(context.Context, database.Querier, []string, string) (map[string]bool, error) {
	return map[string]bool{}, nil
}

type env struct {
	*kittest.T
	org kittest.Org
	pos *fakePos
	kg  string
	gr  string
}

func setup(t *testing.T) *env {
	t.Helper()
	k := kittest.Setup(t, kittest.Menus, func() time.Time { return time.Date(2026, 10, 4, 3, 0, 0, 0, time.UTC) })
	e := &env{T: k, org: k.NewOrg(), pos: &fakePos{}}
	e.kg, e.gr = k.Unit("KG", "Kilogram"), k.Unit("GR", "Gram")
	k.Mount(catalog.Routes(k.Env, catalog.Ports{Pos: e.pos, Procurement: noGrns{}}))
	return e
}

func obj(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func TestUnitsLifecycle(t *testing.T) {
	e := setup(t)
	e.ScopeStaff("company", e.org)
	out := e.Call("POST", "/api/purchasing/units", map[string]any{"kode": "LTR-T", "nama": "Liter", "tipe": "BESAR"}, http.StatusCreated)
	id := obj(out["data"])["id"].(string)
	if obj(out["data"])["company_id"] != e.org.CompanyID || out["success"] != nil {
		t.Fatalf("create %v", out)
	}
	e.Fail("POST", "/api/purchasing/units", map[string]any{"kode": "LTR-T", "nama": "x", "tipe": "KECIL"}, http.StatusBadRequest, "Kode satuan sudah digunakan")
	e.Fail("POST", "/api/purchasing/units", map[string]any{"kode": "", "nama": "x"}, http.StatusBadRequest, "Kode satuan wajib diisi")
	out = e.Call("PUT", "/api/purchasing/units/"+id, map[string]any{"nama": "Liter 2"}, http.StatusOK)
	if out["message"] != "Satuan berhasil diupdate" || obj(out["data"])["nama"] != "Liter 2" {
		t.Fatalf("update %v", out)
	}
	out = e.Call("GET", "/api/purchasing/units?search=LTR-T", nil, http.StatusOK)
	if len(out["data"].([]any)) != 1 || obj(out["pagination"])["total_pages"] != 1.0 {
		t.Fatalf("list %v", out)
	}
	rm := e.RawMaterial(e.org, "RMU-1"+e.org.BranchID[:3], "Pakai Unit", id, "", 1)
	_ = rm
	e.Fail("DELETE", "/api/purchasing/units/"+id, nil, http.StatusBadRequest, "Satuan tidak bisa dihapus karena masih digunakan di bahan baku")
	e.Exec(`UPDATE item.raw_materials SET satuan_besar_id = $1 WHERE id = $2`, e.kg, rm)
	out = e.Call("DELETE", "/api/purchasing/units/"+id, nil, http.StatusOK)
	if out["message"] != "Satuan berhasil dihapus" {
		t.Fatal(out)
	}
	var deletedBy string
	e.Scalar(&deletedBy, `SELECT deleted_by::text FROM item.units WHERE id = $1`, id)
	if deletedBy != e.Staff.UserID {
		t.Fatalf("deleted_by %q", deletedBy)
	}
	e.Fail("GET", "/api/purchasing/units/"+id, nil, http.StatusNotFound, "Satuan tidak ditemukan")
}

func TestWarehousesAndLookups(t *testing.T) {
	e := setup(t)
	e.ScopeStaff("branch", e.org)
	out := e.Call("GET", "/api/purchasing/warehouses", nil, http.StatusOK)
	if len(out["data"].([]any)) != 3 {
		t.Fatalf("warehouses %v", out)
	}
	e.Fail("GET", "/api/purchasing/warehouses?branch_id=x", nil, http.StatusBadRequest, "Branch ID harus valid")

	out = e.Call("POST", "/api/purchasing/items/product-categories", map[string]any{"code": " pc1 ", "nama": " Makanan "}, http.StatusCreated)
	data := obj(out["data"])
	if data["code"] != "PC1" || data["nama"] != "Makanan" || out["message"] != "Berhasil ditambahkan" {
		t.Fatalf("lookup %v", out)
	}
	e.Fail("POST", "/api/purchasing/items/product-categories", map[string]any{"code": "PC1", "nama": "x"}, http.StatusBadRequest, "Kode sudah digunakan")
	e.Fail("GET", "/api/purchasing/items/nope", nil, http.StatusNotFound, "Tipe lookup tidak valid")
	out = e.Call("DELETE", "/api/purchasing/items/product-categories/"+data["id"].(string), nil, http.StatusOK)
	if out["message"] != "Berhasil dihapus" {
		t.Fatal(out)
	}
}

func TestRawMaterialCreateUpdateDelete(t *testing.T) {
	e := setup(t)
	e.ScopeStaff("branch", e.org)
	out := e.Call("POST", "/api/purchasing/raw-materials", map[string]any{"kode": "RMC-" + e.org.BranchID[:4], "nama": "Tepung",
		"kategori": "KERING", "satuan_besar_id": e.kg, "satuan_kecil_id": e.gr, "konversi_factor": 1000}, http.StatusCreated)
	m := obj(out["data"])
	id := m["id"].(string)
	if m["coa"] != "PRODUCTION" || m["coa_asset"] != "1301001" || m["coa_production"] != "5101001" || m["branch_id"] != e.org.BranchID {
		t.Fatalf("create %v", m)
	}
	var packs int
	e.Scalar(&packs, `SELECT count(*) FROM item.raw_material_unit_conversions WHERE raw_material_id = $1`, id)
	if packs != 2 {
		t.Fatalf("packs %d", packs)
	}
	e.Fail("POST", "/api/purchasing/raw-materials", map[string]any{"nama": "x", "kategori": "y", "satuan_besar_id": e.kg, "coa_rnd": "12"},
		http.StatusBadRequest, "Kode Chart of Accounts tidak valid (gunakan 7 digit, mis. 1301001)")

	// A default flag on one pack leaves the other packs' flags false.
	e.Call("PATCH", "/api/purchasing/raw-materials/"+id, map[string]any{"unit_conversions": []any{
		map[string]any{"satuan_id": e.kg, "qty_in_base_unit": 1000, "is_purchase_default": true}}}, http.StatusOK)
	flags := func() string {
		var got string
		e.Scalar(&got, `SELECT string_agg(CASE WHEN satuan_id = $2 THEN 'KG' ELSE 'GR' END||'='||is_purchase_default||'/'||is_issue_default, ','
			ORDER BY satuan_id = $2) FROM item.raw_material_unit_conversions WHERE raw_material_id = $1`, id, e.kg)
		return got
	}
	if got := flags(); got != "GR=false/false,KG=true/false" {
		t.Fatalf("pack flags %s", got)
	}
	// A pack that sends only is_issue_default keeps the stored purchase default.
	e.Call("PATCH", "/api/purchasing/raw-materials/"+id, map[string]any{"unit_conversions": []any{
		map[string]any{"satuan_id": e.gr, "qty_in_base_unit": 1, "is_issue_default": false}}}, http.StatusOK)
	if got := flags(); got != "GR=false/false,KG=true/false" {
		t.Fatalf("pack flags kept %s", got)
	}
	out = e.Call("PATCH", "/api/purchasing/raw-materials/"+id, map[string]any{"nama": "Tepung 2", "unit_conversions": []any{
		map[string]any{"satuan_id": e.kg, "qty_in_base_unit": 1000}}}, http.StatusOK)
	if out["message"] != "Bahan baku berhasil diupdate" || obj(out["data"])["nama"] != "Tepung 2" {
		t.Fatalf("update %v", out)
	}
	out = e.Call("GET", "/api/purchasing/raw-materials/"+id, nil, http.StatusOK)
	// The legacy units are always planned, so both packs stay active.
	if convs := obj(out["data"])["unit_conversions"].([]any); len(convs) != 2 {
		t.Fatalf("active packs %v", convs)
	}
	// Stock on hand blocks the delete.
	stock := e.Stock(e.org, id, e.org.MainID, 4, 1000)
	e.Fail("DELETE", "/api/purchasing/raw-materials/"+id, nil, http.StatusBadRequest, "Bahan baku tidak bisa dihapus karena masih ada stok 4 Kilogram")
	e.Exec(`UPDATE inventory.inventory SET qty_available = 0 WHERE id = $1`, stock)
	out = e.Call("DELETE", "/api/purchasing/raw-materials/"+id, nil, http.StatusOK)
	if out["message"] != "Bahan baku berhasil dihapus" {
		t.Fatal(out)
	}
	e.Fail("GET", "/api/purchasing/materials/"+id, nil, http.StatusNotFound, "Bahan baku tidak ditemukan")
}

func TestLegacyMaterialsAndSupplyItems(t *testing.T) {
	e := setup(t)
	e.ScopeStaff("branch", e.org)
	out := e.Call("POST", "/api/purchasing/materials", map[string]any{"kode": "LG-" + e.org.BranchID[:4], "nama": "Legacy", "satuan_id": e.kg}, http.StatusOK)
	id := obj(out["data"])["id"].(string)
	if obj(out["data"])["kategori"] != "LAINNYA" || out["message"] != "Bahan baku berhasil dibuat" {
		t.Fatalf("legacy create %v", out)
	}
	if rec := e.Do("DELETE", "/api/purchasing/materials/"+id, nil); rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("legacy delete %d %q", rec.Code, rec.Body.String())
	}

	out = e.Call("POST", "/api/purchasing/supply-items", map[string]any{"nama": " Sabun ", "stockable": true}, http.StatusCreated)
	item := obj(out["data"])
	if item["kode"] != "SUP-20261004-001" || item["nama"] != "Sabun" || item["created_by"] != e.Staff.UserID {
		t.Fatalf("supply create %v", item)
	}
	sid := item["id"].(string)
	out = e.Call("PATCH", "/api/purchasing/supply-items/"+sid, map[string]any{"kategori": "  ", "harga_beli": 1500}, http.StatusOK)
	if obj(out["data"])["kategori"] != nil || obj(out["data"])["harga_beli"] != "1500" {
		t.Fatalf("patch %v", out)
	}
	e.Call("DELETE", "/api/purchasing/supply-items/"+sid, nil, http.StatusOK)
	e.Fail("GET", "/api/purchasing/supply-items/"+sid, nil, http.StatusNotFound, "Barang tidak ditemukan")
}

func TestProductsAndBom(t *testing.T) {
	e := setup(t)
	e.ScopeStaff("branch", e.org)
	rm := e.RawMaterial(e.org, "RMB-"+e.org.BranchID[:4], "Gula BOM", e.kg, e.gr, 1000)
	e.Stock(e.org, rm, e.org.MainID, 5000, 0.02)

	out := e.Call("POST", "/api/purchasing/products", map[string]any{"nama": "Es Kopi", "kategori": "Minuman", "warehouse_id": e.org.Stall1ID,
		"harga_jual": "15000", "harga_modal": 0, "markup_persen": 30}, http.StatusCreated)
	p := obj(out["data"])
	pid := p["id"].(string)
	if p["kode"] != "PRD-20261004-001" || p["station"] != "kitchen" || out["message"] != "Produk berhasil ditambahkan dan tersinkron ke POS" {
		t.Fatalf("product %v", out)
	}
	out = e.Call("POST", "/api/purchasing/products/"+pid+"/bom", map[string]any{"raw_material_id": rm, "qty_needed": 200, "waste_persen": 10}, http.StatusCreated)
	bom := obj(out["data"])
	if bom["qty_required"] != "200.0000" || bom["waste_factor"] != "0.1000" {
		t.Fatalf("bom %v", bom)
	}
	e.Fail("POST", "/api/purchasing/products/"+pid+"/bom", map[string]any{"raw_material_id": rm}, http.StatusBadRequest, "Jumlah harus lebih dari 0")
	out = e.Call("GET", "/api/purchasing/products/"+pid+"/bom", nil, http.StatusOK)
	line := obj(out["data"].([]any)[0])
	// 0.02 per gram-equivalent avg cost / 1000 per small unit, × 200 × 1.1.
	if line["cost_per_unit"] != 0.00002 || obj(line["raw_material"])["satuan_kecil"] == nil {
		t.Fatalf("bom line %v", line)
	}
	out = e.Call("GET", "/api/purchasing/products/"+pid, nil, http.StatusOK)
	if obj(out["data"])["hpp_perlu_review"] != false || len(obj(out["data"])["bom_items"].([]any)) != 1 {
		t.Fatalf("detail %v", out)
	}
	e.Fail("POST", "/api/purchasing/products/"+pid+"/apply-recipe-hpp", nil, http.StatusBadRequest,
		"HPP seharusnya belum bisa dihitung. Lengkapi BOM dan biaya bahan dulu.")
	out = e.Call("DELETE", "/api/purchasing/bom/"+bom["id"].(string), nil, http.StatusOK)
	if out["message"] != "Bahan berhasil dihapus dari BOM" {
		t.Fatal(out)
	}
	out = e.Call("DELETE", "/api/purchasing/products/"+pid, nil, http.StatusOK)
	if out["message"] != "Produk berhasil dihapus" || len(e.pos.synced) != 1 {
		t.Fatalf("delete %v synced %v", out, e.pos.synced)
	}
}

// Every branch owns a "Main Storage": a list that spans branches must name
// the branch, a single-branch list keeps the plain names.
func TestWarehousesBranchLabels(t *testing.T) {
	e := setup(t)
	type wh struct{ ID, Name, Code, BranchID, BranchName, Label string }
	list := func() []wh {
		t.Helper()
		out := e.Call("GET", "/api/purchasing/warehouses", nil, http.StatusOK)
		var rows []wh
		for _, v := range out["data"].([]any) {
			m := obj(v)
			rows = append(rows, wh{m["id"].(string), m["name"].(string), m["code"].(string),
				m["branch_id"].(string), m["branch_name"].(string), m["label"].(string)})
		}
		return rows
	}

	var branchName string
	e.Scalar(&branchName, `SELECT name FROM configuration.branches WHERE id = $1`, e.org.BranchID)
	e.ScopeStaff("branch", e.org)
	got := list()
	want := []wh{
		{e.org.MainID, "Main Storage", "MAIN", e.org.BranchID, branchName, "Main Storage"},
		{e.org.Stall1ID, "Stall 1", "STALL-01", e.org.BranchID, branchName, "Stall 1"},
		{e.org.Stall2ID, "Stall 2", "STALL-02", e.org.BranchID, branchName, "Stall 2"},
	}
	if len(got) != len(want) {
		t.Fatalf("single branch: %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("single branch %d: got %+v want %+v", i, got[i], want[i])
		}
	}

	// A sibling branch that sorts before the first one; its Main Storage
	// comes from the same trigger.
	otherBranch := e.ID(`INSERT INTO configuration.branches (company_id, name, code) VALUES ($1, 'A '||$2, 'A'||$2) RETURNING id::text`,
		e.org.CompanyID, testutil.RandomHex(4))
	otherMain := e.ID(`SELECT id::text FROM configuration.warehouses WHERE branch_id = $1 AND code = 'MAIN'`, otherBranch)
	var otherName string
	e.Scalar(&otherName, `SELECT name FROM configuration.branches WHERE id = $1`, otherBranch)
	e.ScopeStaff("company", e.org)
	got = list()
	want = []wh{
		{otherMain, "Main Storage", "MAIN", otherBranch, otherName, "Main Storage — " + otherName},
		{e.org.MainID, "Main Storage", "MAIN", e.org.BranchID, branchName, "Main Storage — " + branchName},
		{e.org.Stall1ID, "Stall 1", "STALL-01", e.org.BranchID, branchName, "Stall 1 — " + branchName},
		{e.org.Stall2ID, "Stall 2", "STALL-02", e.org.BranchID, branchName, "Stall 2 — " + branchName},
	}
	if len(got) != len(want) {
		t.Fatalf("two branches: %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("two branches %d: got %+v want %+v", i, got[i], want[i])
		}
	}
}

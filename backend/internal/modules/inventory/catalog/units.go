package catalog

import (
	"context"
	"math"
	"net/http"
	"strings"

	"nuhabit/backend/internal/modules/inventory/kit"
	"nuhabit/backend/internal/platform/httpx"
	ps "nuhabit/backend/internal/platform/scope"
	"nuhabit/backend/internal/platform/stalllabel"
	"nuhabit/backend/internal/platform/validate"
)

// lib/purchasing/unit-api.ts, warehouse-api.ts, item-lookup-api.ts.

// pageList runs count + page of `SELECT * FROM from WHERE where ORDER BY order`.
func (h *handler) pageList(ctx context.Context, from string, where []string, a *kit.Args, order string, page, limit float64) (kit.Rows, int, error) {
	w := strings.Join(where, " AND ")
	if w == "" {
		w = "true"
	}
	total, err := h.countRows(ctx, from, w, a)
	if err != nil {
		return nil, 0, err
	}
	la := &kit.Args{Values: append([]any(nil), a.Values...)}
	rows, err := kit.Query(ctx, h.env.DB, `SELECT * FROM `+from+` WHERE `+w+` ORDER BY `+order+window(la, page, limit), la.Values...)
	return rows, total, err
}

// listBody is `{ data, pagination }` (no success key).
type listBody struct {
	Data       kit.Rows       `json:"data"`
	Pagination listPagination `json:"pagination"`
}

const unitNotFound = "Satuan tidak ditemukan"

/* GET /api/purchasing/units */
func (h *handler) listUnits(w http.ResponseWriter, r *http.Request) error {
	_, scope, err := h.staffScope(r, h.itemsStaff)
	if err != nil {
		return err
	}
	sp := r.URL.Query()
	page, limit := kit.IntParam(sp, "page", "1"), kit.IntParam(sp, "limit", "20")
	a := &kit.Args{}
	where := []string{`"deleted_at" IS NULL`}
	if c := ps.CompanyFilter(scope); c != nil {
		where = append(where, `("company_id" = `+a.Add(*c)+`)`)
	}
	if s := sp.Get("search"); s != "" {
		where = append(where, kit.Search(a, "", s, "kode", "nama"))
	}
	if sp.Has("is_active") {
		where = append(where, `"is_active" = `+a.Add(sp.Get("is_active") == "true"))
	}
	rows, total, err := h.pageList(r.Context(), `units`, where, a, `"nama" ASC`, page, limit)
	if err != nil {
		return err
	}
	return kit.OK(w, listBody{rows, pagination(page, limit, total, math.Ceil(float64(total)/limit))})
}

// unitExists checks a code inside a company (null company = global).
func (h *handler) unitExists(ctx context.Context, kode string, companyID *string, excludeID string) (bool, error) {
	a := &kit.Args{}
	sql := `SELECT id FROM units WHERE kode = ` + a.Add(kode) + ` AND deleted_at IS NULL`
	if excludeID != "" {
		sql += ` AND id <> ` + a.Add(excludeID)
	}
	if companyID != nil {
		sql += ` AND company_id = ` + a.Add(*companyID)
	} else {
		sql += ` AND company_id IS NULL`
	}
	row, err := kit.QueryOne(ctx, h.env.DB, sql+` LIMIT 1`, a.Values...)
	return row != nil, err
}

/* POST /api/purchasing/units */
func (h *handler) createUnit(w http.ResponseWriter, r *http.Request) error {
	_, scope, err := h.staffScope(r, h.itemsStaff)
	if err != nil {
		return err
	}
	f := validate.New(validate.ReadBody(r))
	cols := kit.NewRow()
	if v := f.Str("kode", validate.Rule{}, validate.StrOpts{Check: minLen(1, "Kode satuan wajib diisi"), Max: 10}); v != nil {
		cols.Set("kode", *v)
	}
	if v := f.Str("nama", validate.Rule{}, validate.StrOpts{Check: minLen(1, "Nama satuan wajib diisi"), Max: 50}); v != nil {
		cols.Set("nama", *v)
	}
	if v := kit.Enum(f, "tipe", validate.Rule{}, []string{"BESAR", "KECIL", "KONVERSI"}, "Tipe satuan wajib dipilih"); v != nil {
		cols.Set("tipe", *v)
	}
	optionalStr(f, cols, "deskripsi", validate.Rule{Optional: true}, validate.StrOpts{})
	if err := kit.FirstIssueErr(f); err != nil {
		return err
	}
	ctx := r.Context()
	companyID := ps.EffectiveCompanyID(scope)
	if dup, err := h.unitExists(ctx, cols.Str("kode"), companyID, ""); err != nil {
		return err
	} else if dup {
		return httpx.BadRequest("Kode satuan sudah digunakan")
	}
	cols.Set("company_id", companyID).Set("is_active", true)
	row, err := kit.InsertOne(ctx, h.env.DB, `units`, cols)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, struct {
		Data *kit.Row `json:"data"`
	}{row})
}

func (h *handler) activeUnit(ctx context.Context, id string) (*kit.Row, error) {
	row, err := kit.QueryOne(ctx, h.env.DB, `SELECT * FROM units WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, httpx.NotFound(unitNotFound)
	}
	return row, nil
}

/* GET /api/purchasing/units/{id} */
func (h *handler) getUnit(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.itemsStaff(r); err != nil {
		return err
	}
	row, err := h.activeUnit(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	return kit.Data(w, row)
}

/* PUT /api/purchasing/units/{id} */
func (h *handler) updateUnit(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.itemsStaff(r); err != nil {
		return err
	}
	f := validate.New(validate.ReadBody(r))
	set := kit.NewRow()
	optionalStr(f, set, "kode", validate.Rule{Optional: true}, validate.StrOpts{Min: 1, Max: 10})
	optionalStr(f, set, "nama", validate.Rule{Optional: true}, validate.StrOpts{Min: 1, Max: 50})
	if v := kit.Enum(f, "tipe", validate.Rule{Optional: true}, []string{"BESAR", "KECIL", "KONVERSI"}, "Tipe satuan wajib dipilih"); v != nil {
		set.Set("tipe", *v)
	}
	optionalStr(f, set, "deskripsi", validate.Rule{Optional: true}, validate.StrOpts{})
	optionalBool(f, set, "is_active")
	if err := kit.FirstIssueErr(f); err != nil {
		return err
	}
	ctx := r.Context()
	id := r.PathValue("id")
	if kode := set.Str("kode"); kode != "" {
		var company *string
		if cur, err := kit.QueryOne(ctx, h.env.DB, `SELECT company_id::text FROM units WHERE id = $1`, id); err == nil && cur != nil {
			company = cur.StrPtr("company_id")
		}
		if dup, err := h.unitExists(ctx, kode, company, id); err != nil {
			return err
		} else if dup {
			return httpx.BadRequest("Kode satuan sudah digunakan")
		}
	}
	set.Set("updated_at", h.env.Now())
	rows, err := kit.Update(ctx, h.env.DB, `units`, set, func(a *kit.Args) string {
		return `"id" = ` + a.Add(id) + ` AND "deleted_at" IS NULL`
	}, true)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return httpx.NotFound(unitNotFound)
	}
	return kit.OK(w, kit.Obj("success", true, "data", rows[0], "message", "Satuan berhasil diupdate"))
}

/* DELETE /api/purchasing/units/{id} — refused while raw materials use it. */
func (h *handler) deleteUnit(w http.ResponseWriter, r *http.Request) error {
	u, err := h.itemsStaff(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	id := r.PathValue("id")
	a := &kit.Args{}
	used, err := kit.QueryOne(ctx, h.env.DB, `SELECT id FROM raw_materials WHERE `+
		kit.Or(a, "", "satuan_besar_id.eq."+id+",satuan_kecil_id.eq."+id)+` LIMIT 1`, a.Values...)
	if err == nil && used != nil {
		return httpx.BadRequest("Satuan tidak bisa dihapus karena masih digunakan di bahan baku")
	}
	now := h.env.Now()
	if _, err := kit.Update(ctx, h.env.DB, `units`, kit.Obj("is_active", false, "deleted_at", now, "deleted_by", u.ID, "updated_at", now),
		func(a *kit.Args) string { return `"id" = ` + a.Add(id) + ` AND "deleted_at" IS NULL` }, false); err != nil {
		return err
	}
	return kit.OK(w, kit.Obj("success", true, "message", "Satuan berhasil dihapus"))
}

/* GET /api/purchasing/warehouses?branch_id=&is_active= */
func (h *handler) listWarehouses(w http.ResponseWriter, r *http.Request) error {
	u, err := h.env.Auth.RequireMenuPrefix(r, "items.product", "items.raw-material", "pos.catalog", "settings.users")
	if err != nil {
		return err
	}
	ctx := r.Context()
	scope, err := h.env.Scope(ctx, u)
	if err != nil {
		return err
	}
	f := kit.NewQueryForm(r.URL.Query())
	branch := f.Str("branch_id", validate.Rule{Optional: true}, validate.StrOpts{Check: func(s string) (string, string, bool) {
		return "invalid_format", "Branch ID harus valid", validate.IsUUID(s)
	}})
	active := true
	if v, ok := f.Fields()["is_active"]; ok {
		active = v.(string) != "" // z.coerce.boolean(): Boolean(string)
	}
	if err := kit.FirstIssueErr(f.Form); err != nil {
		return err
	}
	a := &kit.Args{}
	where := `w."is_active" = ` + a.Add(active)
	if b := ps.WarehouseBranchFilter(scope, branch); b != nil {
		where += ` AND w."branch_id" = ` + a.Add(*b)
	} else if c := ps.WarehouseCompanyFilter(scope); c != nil {
		branches, err := kit.Query(ctx, h.env.DB, `SELECT id::text AS id FROM configuration.branches WHERE company_id = $1`, *c)
		if err != nil {
			return err
		}
		if len(branches) == 0 {
			return kit.Data(w, kit.Rows{})
		}
		ids := make([]string, len(branches))
		for i, b := range branches {
			ids[i] = b.Str("id")
		}
		where += ` AND w."branch_id" = ANY(` + a.Add(ids) + `::uuid[])`
	}
	rows, err := kit.Query(ctx, h.env.DB, `SELECT w.*, COALESCE(b.name, '') AS branch_name
		FROM configuration.warehouses w
		LEFT JOIN configuration.branches b ON b.id = w.branch_id
		WHERE `+where+` ORDER BY w."name" ASC`, a.Values...)
	if err != nil {
		return err
	}
	// Each branch owns a "Main Storage": name the branch when the list mixes
	// branches, and group by branch (Main Storage first) so rows read alike.
	rows = stalllabel.SortByBranch(rows, func(r *kit.Row) stalllabel.Key {
		return stalllabel.Key{BranchName: r.Str("branch_name"), BranchID: r.Str("branch_id"), IsDefault: r.Bool("is_default")}
	}, nil)
	withBranch := stalllabel.Spans(rows, func(r *kit.Row) string { return r.Str("branch_id") })
	for _, r := range rows {
		r.Set("label", stalllabel.Label(r.Str("name"), r.Str("branch_name"), withBranch))
	}
	return kit.Data(w, rows)
}

/* ── item lookups (/api/purchasing/items/{lookup}) ───────────────────── */

var lookupTables = map[string]string{
	"raw-material-categories": "raw_material_categories",
	"product-categories":      "product_categories",
	"supply-categories":       "supply_categories",
}

func lookupTable(r *http.Request) (string, error) {
	t, ok := lookupTables[r.PathValue("lookup")]
	if !ok {
		return "", httpx.NotFound("Tipe lookup tidak valid")
	}
	return t, nil
}

/* GET /api/purchasing/items/{lookup} */
func (h *handler) listLookup(w http.ResponseWriter, r *http.Request) error {
	_, scope, err := h.staffScope(r, h.itemsStaff)
	if err != nil {
		return err
	}
	table, err := lookupTable(r)
	if err != nil {
		return err
	}
	sp := r.URL.Query()
	page, limit := kit.IntParam(sp, "page", "1"), kit.IntParam(sp, "limit", "50")
	a := &kit.Args{}
	where := []string{`"deleted_at" IS NULL`}
	if c := ps.CompanyFilter(scope); c != nil {
		where = append(where, `("company_id" = `+a.Add(*c)+`)`)
	}
	if s := sp.Get("search"); s != "" {
		where = append(where, kit.Search(a, "", s, "code", "nama"))
	}
	rows, total, err := h.pageList(r.Context(), table, where, a, `"nama" ASC`, page, limit)
	if err != nil {
		return err
	}
	return kit.OK(w, listBody{rows, pagination(page, limit, total, math.Ceil(float64(total)/limit))})
}

// lookupColumns validates itemsLookupSchema and shapes lookupColumns.
func lookupColumns(r *http.Request) (*kit.Row, error) {
	f := validate.New(validate.ReadBody(r))
	code := f.Str("code", validate.Rule{}, validate.StrOpts{Check: minLen(1, "Kode wajib diisi"), Max: 30})
	nama := f.Str("nama", validate.Rule{}, validate.StrOpts{Check: minLen(1, "Nama wajib diisi"), Max: 100})
	desc := f.Str("deskripsi", validate.Rule{Optional: true, Nullable: true}, validate.StrOpts{})
	active := f.Bool("is_active", validate.Rule{Optional: true})
	if err := kit.FirstIssueErr(f); err != nil {
		return nil, err
	}
	var d any
	if desc != nil && validate.JSTrim(*desc) != "" {
		d = validate.JSTrim(*desc)
	}
	isActive := true
	if active != nil {
		isActive = *active
	}
	return kit.Obj("code", strings.ToUpper(validate.JSTrim(*code)), "nama", validate.JSTrim(*nama), "deskripsi", d, "is_active", isActive), nil
}

func (h *handler) lookupCodeTaken(ctx context.Context, table, code string, companyID *string, excludeID string) (bool, error) {
	a := &kit.Args{}
	sql := `SELECT id FROM ` + table + ` WHERE code = ` + a.Add(code) + ` AND deleted_at IS NULL`
	if excludeID != "" {
		sql += ` AND id <> ` + a.Add(excludeID)
	}
	if companyID != nil {
		sql += ` AND company_id = ` + a.Add(*companyID)
	} else {
		sql += ` AND company_id IS NULL`
	}
	row, err := kit.QueryOne(ctx, h.env.DB, sql+` LIMIT 1`, a.Values...)
	return row != nil, err
}

/* POST /api/purchasing/items/{lookup} */
func (h *handler) createLookup(w http.ResponseWriter, r *http.Request) error {
	_, scope, err := h.staffScope(r, h.itemsStaff)
	if err != nil {
		return err
	}
	table, err := lookupTable(r)
	if err != nil {
		return err
	}
	cols, err := lookupColumns(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	companyID := ps.EffectiveCompanyID(scope)
	if taken, err := h.lookupCodeTaken(ctx, table, cols.Str("code"), companyID, ""); err != nil {
		return err
	} else if taken {
		return httpx.BadRequest("Kode sudah digunakan")
	}
	row, err := kit.InsertOne(ctx, h.env.DB, table, cols.Set("company_id", companyID))
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, kit.Obj("data", row, "message", "Berhasil ditambahkan"))
}

/* PUT /api/purchasing/items/{lookup}/{id} */
func (h *handler) updateLookup(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.itemsStaff(r); err != nil {
		return err
	}
	table, err := lookupTable(r)
	if err != nil {
		return err
	}
	cols, err := lookupColumns(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	id := r.PathValue("id")
	var company *string
	if cur, err := kit.QueryOne(ctx, h.env.DB, `SELECT company_id::text FROM `+table+` WHERE id = $1`, id); err == nil && cur != nil {
		company = cur.StrPtr("company_id")
	}
	if taken, err := h.lookupCodeTaken(ctx, table, cols.Str("code"), company, id); err != nil {
		return err
	} else if taken {
		return httpx.BadRequest("Kode sudah digunakan")
	}
	rows, err := kit.Update(ctx, h.env.DB, table, cols.Set("updated_at", h.env.Now()), func(a *kit.Args) string {
		return `"id" = ` + a.Add(id) + ` AND "deleted_at" IS NULL`
	}, true)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return errNoRows // `.single()` on no row: the TS rethrows → 500
	}
	return kit.OK(w, kit.Obj("data", rows[0], "message", "Berhasil diperbarui"))
}

/* DELETE /api/purchasing/items/{lookup}/{id} */
func (h *handler) deleteLookup(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.itemsStaff(r); err != nil {
		return err
	}
	table, err := lookupTable(r)
	if err != nil {
		return err
	}
	id := r.PathValue("id")
	if _, err := kit.Update(r.Context(), h.env.DB, table, kit.Obj("deleted_at", h.env.Now(), "is_active", false), func(a *kit.Args) string {
		return `"id" = ` + a.Add(id) + ` AND "deleted_at" IS NULL`
	}, false); err != nil {
		return err
	}
	return kit.OK(w, kit.Obj("message", "Berhasil dihapus"))
}

/* ── body helpers ────────────────────────────────────────────────────── */

// minLen is .min(n, msg) on a string.
func minLen(n int, msg string) func(string) (string, string, bool) {
	return func(s string) (string, string, bool) { return "too_small", msg, validate.UTF16Len(s) >= n }
}

// optionalStr validates a string field and copies it when sent (null
// included when the rule allows it).
func optionalStr(f *validate.Form, row *kit.Row, key string, r validate.Rule, o validate.StrOpts) {
	v := f.Str(key, r, o)
	if v != nil {
		row.Set(key, *v)
	} else if raw, sent := f.Fields()[key]; sent && raw == nil && r.Nullable {
		row.Set(key, nil)
	}
}

// optionalBool copies a sent boolean.
func optionalBool(f *validate.Form, row *kit.Row, key string) {
	if v := f.Bool(key, validate.Rule{Optional: true}); v != nil {
		row.Set(key, *v)
	}
}

// optionalNum copies a sent number checked with o (null when nullable).
func optionalNum(f *validate.Form, row *kit.Row, key string, r validate.Rule, o validate.NumOpts) {
	if v := f.Num(key, r, o); v != nil {
		row.Set(key, *v)
	} else if raw, sent := f.Fields()[key]; sent && raw == nil && r.Nullable {
		row.Set(key, nil)
	}
}

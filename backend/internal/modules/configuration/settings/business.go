package settings

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/modules/configuration/kit"
	"nuhabit/backend/internal/modules/configuration/settings/domain"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/validate"
)

/* ── tree (lib/configuration/business-repository.ts fetchBusinessTree) ── */

type businessWarehouse struct {
	ID        string `json:"id"`
	BranchID  string `json:"branch_id"`
	Name      string `json:"name"`
	Code      string `json:"code"`
	IsDefault bool   `json:"is_default"`
	IsActive  bool   `json:"is_active"`
}

type businessBranch struct {
	ID         string              `json:"id"`
	CompanyID  string              `json:"company_id"`
	Name       string              `json:"name"`
	Code       string              `json:"code"`
	IsActive   bool                `json:"is_active"`
	Warehouses []businessWarehouse `json:"warehouses"`
}

type businessCompany struct {
	ID        string           `json:"id"`
	HoldingID string           `json:"holding_id"`
	Name      string           `json:"name"`
	Code      string           `json:"code"`
	IsActive  bool             `json:"is_active"`
	Branches  []businessBranch `json:"branches"`
}

type businessHolding struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Code      string            `json:"code"`
	IsActive  bool              `json:"is_active"`
	Companies []businessCompany `json:"companies"`
}

type businessTree struct {
	Holdings []businessHolding `json:"holdings"`
}

func fetchBusinessTree(ctx context.Context, q database.Querier) (businessTree, error) {
	holdings, err := queryStructs[businessHolding](ctx, q,
		`SELECT id::text, name, code, is_active FROM configuration.holdings ORDER BY name`)
	if err != nil {
		return businessTree{}, err
	}
	companies, err := queryStructs[businessCompany](ctx, q,
		`SELECT id::text, holding_id::text, name, code, is_active FROM configuration.companies ORDER BY name`)
	if err != nil {
		return businessTree{}, err
	}
	branches, err := queryStructs[businessBranch](ctx, q,
		`SELECT id::text, company_id::text, name, code, is_active FROM configuration.branches ORDER BY name`)
	if err != nil {
		return businessTree{}, err
	}
	warehouses, err := queryStructs[businessWarehouse](ctx, q, `SELECT id::text, branch_id::text, name, code, is_default, is_active
     FROM configuration.warehouses
     WHERE is_active = true
     ORDER BY is_default DESC, code`)
	if err != nil {
		return businessTree{}, err
	}

	byBranch := map[string][]businessWarehouse{}
	for _, wh := range warehouses {
		byBranch[wh.BranchID] = append(byBranch[wh.BranchID], wh)
	}
	for _, list := range byBranch {
		domain.SortWarehouses(list, func(w businessWarehouse) domain.Warehouse {
			return domain.Warehouse{IsDefault: w.IsDefault, Code: w.Code, Name: w.Name}
		})
	}
	byCompany := map[string][]businessBranch{}
	for _, br := range branches {
		br.Warehouses = orEmpty(byBranch[br.ID])
		byCompany[br.CompanyID] = append(byCompany[br.CompanyID], br)
	}
	byHolding := map[string][]businessCompany{}
	for _, co := range companies {
		co.Branches = orEmpty(byCompany[co.ID])
		byHolding[co.HoldingID] = append(byHolding[co.HoldingID], co)
	}
	for i := range holdings {
		holdings[i].Companies = orEmpty(byHolding[holdings[i].ID])
	}
	return businessTree{Holdings: holdings}, nil
}

// queryStructs scans every row by position into T's leading fields; the
// nested slice fields stay nil until the tree is assembled.
func queryStructs[T any](ctx context.Context, q database.Querier, sql string) ([]T, error) {
	rows, err := q.Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (T, error) {
		var v T
		return v, row.Scan(scanTargets(&v)...)
	})
}

// scanTargets lists the scalar fields of a tree node in column order.
func scanTargets(v any) []any {
	switch x := v.(type) {
	case *businessHolding:
		return []any{&x.ID, &x.Name, &x.Code, &x.IsActive}
	case *businessCompany:
		return []any{&x.ID, &x.HoldingID, &x.Name, &x.Code, &x.IsActive}
	case *businessBranch:
		return []any{&x.ID, &x.CompanyID, &x.Name, &x.Code, &x.IsActive}
	case *businessWarehouse:
		return []any{&x.ID, &x.BranchID, &x.Name, &x.Code, &x.IsDefault, &x.IsActive}
	}
	return nil
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

/* ── writes ──────────────────────────────────────────────────────────── */

type businessInput struct {
	name     string
	code     *string
	parentID *string
	isActive *bool
}

// createBusinessEntity is createBusinessEntity: its plain errors carry the
// user-facing messages the route turns into 400s.
func createBusinessEntity(ctx context.Context, q database.Querier, typ string, in businessInput) (string, error) {
	name := validate.JSTrim(in.name)
	if name == "" {
		return "", errors.New("Nama wajib diisi")
	}
	isActive := in.isActive == nil || *in.isActive
	code := func(fallback string) string {
		if in.code != nil {
			return strings.ToUpper(*in.code)
		}
		return strings.ToUpper(fallback)
	}
	if typ != "holding" && (in.parentID == nil || *in.parentID == "") {
		return "", errors.New(domain.ParentRequired[typ])
	}
	var id string
	var err error
	switch typ {
	case "holding":
		err = q.QueryRow(ctx, `INSERT INTO configuration.holdings (name, code, is_active)
       VALUES ($1, $2, $3)
       RETURNING id::text`, name, code(domain.SlugCode(name)), isActive).Scan(&id)
	case "company":
		err = q.QueryRow(ctx, `INSERT INTO configuration.companies (holding_id, name, code, is_active)
       VALUES ($1, $2, $3, $4)
       RETURNING id::text`, *in.parentID, name, code(domain.SlugCode(name)), isActive).Scan(&id)
	case "branch":
		err = q.QueryRow(ctx, `INSERT INTO configuration.branches (company_id, name, code, is_active)
       VALUES ($1, $2, $3, $4)
       RETURNING id::text`, *in.parentID, name, code(domain.SlugCode(name)), isActive).Scan(&id)
	case "warehouse":
		var count string
		if err := q.QueryRow(ctx, `SELECT COUNT(*)::text AS count FROM configuration.warehouses WHERE branch_id = $1`,
			*in.parentID).Scan(&count); err != nil {
			return "", err
		}
		n, _ := strconv.Atoi(count)
		autoName, autoCode := domain.NextWarehouse(n)
		if name == "" {
			name = autoName
		}
		err = q.QueryRow(ctx, `INSERT INTO configuration.warehouses (branch_id, name, code, is_default, is_active)
       VALUES ($1, $2, $3, false, $4)
       RETURNING id::text`, *in.parentID, name, code(autoCode), isActive).Scan(&id)
	default:
		return "", errors.New("Tipe entitas tidak valid")
	}
	return id, err
}

// updateBusinessEntity is updateBusinessEntity.
func updateBusinessEntity(ctx context.Context, q database.Querier, typ, id string, in businessInput, setName bool) error {
	var sets []string
	var values []any
	add := func(col string, v any) {
		values = append(values, v)
		sets = append(sets, col+" = $"+strconv.Itoa(len(values)))
	}
	if setName {
		add("name", validate.JSTrim(in.name))
	}
	if in.code != nil {
		add("code", strings.ToUpper(validate.JSTrim(*in.code)))
	}
	if in.isActive != nil {
		add("is_active", *in.isActive)
	}
	if len(sets) == 0 {
		return errors.New("Tidak ada data yang diperbarui")
	}
	sets = append(sets, "updated_at = now()")
	values = append(values, id)
	_, err := q.Exec(ctx, `UPDATE configuration.`+domain.BusinessTables[typ]+` SET `+strings.Join(sets, ", ")+
		` WHERE id = $`+strconv.Itoa(len(values))+` RETURNING id`, values...)
	return err
}

// deleteBusinessEntity is deleteBusinessEntity: a branch keeps its last
// default warehouse.
func deleteBusinessEntity(ctx context.Context, q database.Querier, typ, id string) error {
	if typ == "warehouse" {
		var isDefault bool
		var branchID string
		err := q.QueryRow(ctx, `SELECT is_default, branch_id::text FROM configuration.warehouses WHERE id = $1`, id).
			Scan(&isDefault, &branchID)
		if database.IsNoRows(err) {
			return errors.New("Outlet tidak ditemukan")
		}
		if err != nil {
			return err
		}
		if isDefault {
			var count string
			if err := q.QueryRow(ctx, `SELECT COUNT(*)::text AS count FROM configuration.warehouses WHERE branch_id = $1`,
				branchID).Scan(&count); err != nil {
				return err
			}
			if n, _ := strconv.Atoi(count); n <= 1 {
				return errors.New("Main Storage tidak dapat dihapus. Setiap cabang wajib memiliki minimal 1 outlet.")
			}
		}
	}
	_, err := q.Exec(ctx, `DELETE FROM configuration.`+domain.BusinessTables[typ]+` WHERE id = $1 RETURNING id`, id)
	return err
}

/* ── handlers ────────────────────────────────────────────────────────── */

var (
	createErrors = regexp.MustCompile(`wajib|valid`)
	updateErrors = regexp.MustCompile(`Tidak ada data`)
	deleteErrors = regexp.MustCompile(`tidak dapat|tidak ditemukan`)
)

type businessCreated struct {
	Data struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	} `json:"data"`
	Tree businessTree `json:"tree"`
}

type businessChanged struct {
	Data struct {
		ID string `json:"id"`
	} `json:"data"`
	Tree businessTree `json:"tree"`
}

// businessTreeReaders read the tree: Business settings, and account managers
// (Manajemen User) who pick a login's data scope from it in Buat Akun Login.
// Writes stay Business settings only.
var businessTreeReaders = append(append([]string{}, iam.SettingsBusiness...), iam.SettingsUsers...)

func (h *handler) getBusiness(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.d.Auth.RequireMenuPrefix(r, businessTreeReaders...); err != nil {
		return err
	}
	tree, err := fetchBusinessTree(r.Context(), h.db)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, dataBody{tree})
}

func (h *handler) createBusiness(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.d.Auth.RequireMenuPrefix(r, iam.SettingsBusiness...); err != nil {
		return err
	}
	f, err := kit.Form(r)
	if err != nil {
		return err
	}
	// z.enum(BUSINESS_ENTITY_TYPES, { message }) answers every failure with
	// the custom message.
	typ, isString := f.Fields()["type"].(string)
	if f.Fields() != nil && (!isString || !domain.IsBusinessEntityType(typ)) {
		f.Fail("type", "invalid_value", "Tipe entitas tidak valid")
	}
	in := businessInput{name: f.StrDefault("name", "", validate.StrOpts{})}
	in.code = f.Str("code", validate.Rule{Optional: true}, validate.StrOpts{})
	in.parentID = f.Str("parentId", validate.Rule{Optional: true, Nullable: true}, validate.StrOpts{})
	in.isActive = f.Bool("is_active", validate.Rule{Optional: true})
	if err := f.Err("Validation failed"); err != nil {
		return err
	}
	id, err := createBusinessEntity(r.Context(), h.db, typ, in)
	if err != nil {
		return userFacing(err, createErrors)
	}
	tree, err := fetchBusinessTree(r.Context(), h.db)
	if err != nil {
		return err
	}
	var out businessCreated
	out.Data.ID, out.Data.Type, out.Tree = id, typ, tree
	return httpx.JSON(w, http.StatusCreated, out)
}

func (h *handler) entityType(r *http.Request) (string, error) {
	if _, err := h.d.Auth.RequireMenuPrefix(r, iam.SettingsBusiness...); err != nil {
		return "", err
	}
	typ := r.PathValue("type")
	if !domain.IsBusinessEntityType(typ) {
		return "", httpx.BadRequest("Tipe entitas tidak valid")
	}
	return typ, nil
}

func (h *handler) respondChanged(w http.ResponseWriter, r *http.Request, id string) error {
	tree, err := fetchBusinessTree(r.Context(), h.db)
	if err != nil {
		return err
	}
	var out businessChanged
	out.Data.ID, out.Tree = id, tree
	return httpx.JSON(w, http.StatusOK, out)
}

func (h *handler) updateBusiness(w http.ResponseWriter, r *http.Request) error {
	typ, err := h.entityType(r)
	if err != nil {
		return err
	}
	f, err := kit.Form(r)
	if err != nil {
		return err
	}
	var in businessInput
	name := f.Str("name", validate.Rule{Optional: true}, validate.StrOpts{})
	if name != nil {
		in.name = *name
	}
	in.code = f.Str("code", validate.Rule{Optional: true}, validate.StrOpts{})
	in.isActive = f.Bool("is_active", validate.Rule{Optional: true})
	if err := f.Err("Validation failed"); err != nil {
		return err
	}
	id := r.PathValue("id")
	if err := updateBusinessEntity(r.Context(), h.db, typ, id, in, name != nil); err != nil {
		return userFacing(err, updateErrors)
	}
	return h.respondChanged(w, r, id)
}

func (h *handler) deleteBusiness(w http.ResponseWriter, r *http.Request) error {
	typ, err := h.entityType(r)
	if err != nil {
		return err
	}
	id := r.PathValue("id")
	if err := deleteBusinessEntity(r.Context(), h.db, typ, id); err != nil {
		return userFacing(err, deleteErrors)
	}
	return h.respondChanged(w, r, id)
}

package accounts

import (
	"encoding/json"
	"math"
	"net/url"
	"strings"

	"nuhabit/backend/internal/modules/configuration/accounts/domain"
	"nuhabit/backend/internal/modules/configuration/kit"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/validate"
)

// aborted is zod's util.aborted: an invalid_type or invalid_value issue
// stops the refinements that follow; format and size checks do not.
func aborted(issues []validate.Issue) bool {
	for _, is := range issues {
		if is.Code == "invalid_type" || is.Code == "invalid_value" {
			return true
		}
	}
	return false
}

var (
	optional     = validate.Rule{Optional: true}
	optionalNull = validate.Rule{Optional: true, Nullable: true}
	withDefault  = validate.Rule{HasDefault: true}
)

/* ── approval permissions (approvalPermissionSchema) ─────────────────── */

type approval struct {
	Module, Workflow, Level string
	Limit                   *float64
	IsActive                bool
}

// jsNumber is JavaScript Number(v) for a decoded JSON value: what
// z.coerce.number() reads.
func jsNumber(v any) float64 {
	switch x := v.(type) {
	case nil:
		return 0
	case bool:
		if x {
			return 1
		}
		return 0
	case json.Number:
		return kit.NumberFromString(string(x))
	case string:
		return kit.NumberFromString(x)
	case []any:
		switch len(x) {
		case 0:
			return 0
		case 1:
			if _, isBool := x[0].(bool); !isBool {
				return jsNumber(x[0])
			}
		}
	}
	return math.NaN()
}

// coerceNumber is z.coerce.number() with the given checks.
func coerceNumber(f *validate.Form, key string, r validate.Rule, o validate.NumOpts) *float64 {
	v, _, done := f.Take(key, "number", r)
	if done {
		return nil
	}
	x := jsNumber(v)
	switch {
	case math.IsNaN(x):
		f.Fail(key, "invalid_type", "Invalid input: expected number, received NaN")
		return nil
	case math.IsInf(x, 0):
		f.Fail(key, "invalid_type", "Invalid input: expected number, received number")
		return nil
	}
	if _, ok := f.CheckNumber(key, json.Number(validate.JSNumber(x)), o); !ok {
		return nil
	}
	return &x
}

func readApprovals(f *validate.Form, key string, r validate.Rule) []approval {
	out := []approval{}
	items := f.List(key, r, math.MaxInt, func(list *validate.Form, i int, v any) {
		start := len(list.Issues())
		item := list.Item(i, v)
		item.UUID("id", optional)
		module := item.Enum("module", validate.Rule{}, domain.ApprovalModules)
		workflow := item.Enum("workflow", validate.Rule{}, domain.ApprovalWorkflows)
		level := item.Enum("approval_level", validate.Rule{}, domain.ApprovalLevels)
		limit := coerceNumber(item, "approval_limit", optionalNull, validate.NumOpts{Min: validate.Bound(0)})
		active := item.BoolDefault("is_active", true)
		if aborted(list.Issues()[start:]) {
			return
		}
		if !domain.WorkflowMatchesModule(*workflow, *module) {
			item.Fail("workflow", "custom", "Workflow tidak sesuai dengan module approval")
		}
		out = append(out, approval{Module: *module, Workflow: *workflow, Level: *level, Limit: limit, IsActive: active})
	})
	if items == nil {
		return nil
	}
	return out
}

/* ── employee users (lib/users/schemas.ts) ───────────────────────────── */

// coreFields is employeeCoreSchema in order, minus is_active.
var coreFields = []string{
	"full_name", "email", "phone", "join_date", "employment_status", "ktp", "npwp",
	"birth_date", "gender", "marital_status", "address", "city", "province", "postal_code",
	"department_id", "section_id", "job_title_id", "reporting_to", "bank_name", "bank_account",
	"bpjs_tk", "bpjs_kesehatan", "emergency_contact_name", "emergency_contact_phone",
	"emergency_contact_relationship", "notes", "nip", "is_active", "end_date",
}

// employeeSchemaKeys is the merged schema's key order (Object.keys(input)).
var employeeSchemaKeys = append(append([]string{}, coreFields...),
	"is_access_app", "password", "role", "brand_id", "account_status", "approval_permissions",
	"business_scope", "holding_id", "company_id", "branch_id", "warehouse_ids",
	"default_warehouse_id", "can_switch_stall", "can_central_checkout")

// employeeBody is a parsed createUserEmployeeSchema or
// updateUserEmployeeSchema body. Pointers are nil when absent or null; has
// tells the two apart.
type employeeBody struct {
	raw                                 map[string]any
	text                                map[string]*string
	isActive, isAccessApp               *bool
	password, role, accountStatus       *string
	approvals                           []approval
	businessScope, holdingID, companyID *string
	branchID, defaultWarehouseID        *string
	warehouseIDs                        []string
	canSwitchStall, canCentralCheckout  *bool
	// scheduleType is create-only: changes go through the HRIS
	// schedule-type endpoint so its transition rules always run.
	scheduleType *string
}

func (b *employeeBody) has(key string) bool {
	_, ok := b.raw[key]
	return ok
}

// keys is Object.keys of the parsed body: schema keys the client sent,
// plus is_access_app and approval_permissions when they have a default.
func (b *employeeBody) keys(create bool) []string {
	var out []string
	for _, k := range employeeSchemaKeys {
		if b.has(k) || (create && (k == "is_access_app" || k == "approval_permissions")) {
			out = append(out, k)
		}
	}
	return out
}

func (b *employeeBody) str(key string) *string { return b.text[key] }

// accountKeys are the body keys that configure the login account.
var accountKeys = employeeSchemaKeys[len(coreFields):]

// touchesAccount reports whether the body configures a login account.
// Creating an employee with is_access_app false (and nothing else) does not.
func (b *employeeBody) touchesAccount(create bool) bool {
	for _, k := range accountKeys {
		if !b.has(k) {
			continue
		}
		if create && k == "is_access_app" && b.isAccessApp != nil && !*b.isAccessApp {
			continue
		}
		return true
	}
	return false
}

func roleOf(role *string) string {
	if role == nil {
		return ""
	}
	return *role
}

// parseEmployeeBody validates a create (create true) or update body.
func parseEmployeeBody(f *validate.Form, create bool) *employeeBody {
	b := &employeeBody{raw: f.Fields(), text: map[string]*string{}}
	required := validate.Rule{}
	if !create {
		required = optional
	}
	for _, k := range coreFields {
		switch k {
		case "full_name":
			b.text[k] = f.Str(k, required, validate.StrOpts{Min: 2})
		case "email":
			b.text[k] = f.Str(k, required, validate.StrOpts{Check: kit.EmailCheck})
		case "join_date", "employment_status":
			b.text[k] = f.Str(k, required, validate.StrOpts{Min: 1})
		case "department_id", "section_id", "job_title_id", "reporting_to":
			b.text[k] = f.UUID(k, optionalNull)
		case "is_active":
			b.isActive = f.Bool(k, optional)
		default:
			b.text[k] = f.Str(k, optionalNull, validate.StrOpts{})
		}
	}
	if create {
		b.scheduleType = f.Enum("schedule_type", optional, []string{"shift", "non_shift"})
		access := f.BoolDefault("is_access_app", false)
		b.isAccessApp = &access
	} else {
		b.isAccessApp = f.Bool("is_access_app", optional)
	}
	b.password = f.Str("password", optional, validate.StrOpts{Min: 8})
	b.role = f.Enum("role", optional, domain.AdminUserRoles)
	f.UUID("brand_id", optionalNull)
	b.accountStatus = f.Enum("account_status", optional, []string{"active", "inactive"})
	if create {
		b.approvals = readApprovals(f, "approval_permissions", withDefault)
		if b.approvals == nil {
			b.approvals = []approval{}
		}
	} else {
		b.approvals = readApprovals(f, "approval_permissions", optional)
	}
	b.businessScope = f.Enum("business_scope", optionalNull, []string{"holding", "company", "branch"})
	b.holdingID = f.UUID("holding_id", optionalNull)
	b.companyID = f.UUID("company_id", optionalNull)
	b.branchID = f.UUID("branch_id", optionalNull)
	b.warehouseIDs = f.Strings("warehouse_ids", optional, math.MaxInt, validate.StrOpts{Check: validate.UUIDCheck})
	b.defaultWarehouseID = f.UUID("default_warehouse_id", optionalNull)
	b.canSwitchStall = f.Bool("can_switch_stall", optional)
	b.canCentralCheckout = f.Bool("can_central_checkout", optional)

	if aborted(f.Issues()) {
		return b
	}
	accessOn := b.isAccessApp != nil && *b.isAccessApp
	if create && accessOn {
		if b.password == nil || *b.password == "" {
			f.Fail("password", "custom", "Password is required for app access")
		}
		if b.role == nil {
			f.Fail("role", "custom", "Role is required for app access")
		}
	}
	if !create && accessOn && b.password != nil && validate.UTF16Len(*b.password) < 8 {
		f.Fail("password", "custom", "Password must be at least 8 characters")
	}
	if !accessOn {
		return b
	}
	scope := domain.NormalizeScope(b.businessScope, b.holdingID, b.companyID, b.branchID)
	if msg := domain.ValidateScope(roleOf(b.role), true, scope); msg != "" {
		f.Fail("business_scope", "custom", msg)
	}
	if domain.RequiresStallAssignment(roleOf(b.role), scope.BusinessScope, true) &&
		domain.DefaultWarehouseID(b.defaultWarehouseID, b.warehouseIDs) == nil {
		f.Fail("default_warehouse_id", "custom", "Outlet default wajib untuk scope branch")
	}
	return b
}

/* ── admin users (lib/admin/user-management.ts) ──────────────────────── */

type adminBody struct {
	raw                                    map[string]any
	email, password, fullName, role, brand *string
	status                                 *string
	approvals                              []approval
}

func (b *adminBody) has(key string) bool {
	_, ok := b.raw[key]
	return ok
}

var adminUpdateKeys = []string{"email", "full_name", "role", "brand_id", "status", "approval_permissions"}

// keys is Object.keys(body) of the update schema.
func (b *adminBody) keys() []string {
	out := []string{}
	for _, k := range adminUpdateKeys {
		if b.has(k) {
			out = append(out, k)
		}
	}
	return out
}

// parseAdminCreate is createAdminUserSchema.
func parseAdminCreate(f *validate.Form) *adminBody {
	b := &adminBody{raw: f.Fields()}
	b.email = f.Str("email", validate.Rule{}, validate.StrOpts{Check: kit.EmailCheck})
	b.password = f.Str("password", validate.Rule{}, validate.StrOpts{Min: 8})
	b.fullName = f.Str("full_name", validate.Rule{}, validate.StrOpts{Min: 2})
	b.role = f.Enum("role", validate.Rule{}, domain.AdminUserRoles)
	b.brand = f.UUID("brand_id", optionalNull)
	b.status = f.Enum("status", withDefault, []string{"active", "inactive"})
	if b.status == nil {
		active := "active"
		b.status = &active
	}
	b.approvals = readApprovals(f, "approval_permissions", withDefault)
	if b.approvals == nil {
		b.approvals = []approval{}
	}
	return b
}

// parseAdminUpdate is updateAdminUserSchema.
func parseAdminUpdate(f *validate.Form) *adminBody {
	b := &adminBody{raw: f.Fields()}
	b.email = f.Str("email", optional, validate.StrOpts{Check: kit.EmailCheck})
	b.fullName = f.Str("full_name", optional, validate.StrOpts{Min: 2})
	b.role = f.Enum("role", optional, domain.AdminUserRoles)
	b.brand = f.UUID("brand_id", optionalNull)
	b.status = f.Enum("status", optional, []string{"active", "inactive"})
	b.approvals = readApprovals(f, "approval_permissions", optional)
	return b
}

/* ── GET /api/users query (lib/hris/users-list-query.ts) ─────────────── */

var listSortColumns = []string{"full_name", "nip", "email", "join_date", "employment_status", "created_at"}

type listQuery struct {
	search, departmentID, employmentStatus, role *string
	scheduleType                                 *string
	isActive, isAccessApp                        *bool
	page, limit                                  int
	sortBy                                       string
	asc                                          bool
}

// searchSeparators break the query builder's or() filter expression.
var searchSeparators = strings.NewReplacer(",", " ", "(", " ", ")", " ")

// parseListQuery is parseUserListQuery: empty values are dropped, the last
// value of a repeated key wins, and a failure is 400 "Parameter tidak valid".
func parseListQuery(values url.Values) (listQuery, error) {
	raw := map[string]any{}
	for k, vs := range values {
		for _, v := range vs {
			if v != "" {
				raw[k] = v
			}
		}
	}
	f := validate.New(raw, true)
	text := func(key string, max int) *string {
		s := f.Str(key, optional, validate.StrOpts{Trim: true, Max: max})
		if s == nil || *s == "" {
			return nil
		}
		return s
	}
	flag := func(key string) *bool {
		s := f.Enum(key, optional, []string{"true", "false"})
		if s == nil {
			return nil
		}
		b := *s == "true"
		return &b
	}
	q := listQuery{}
	if s := text("search", 100); s != nil {
		if cleaned := validate.JSTrim(searchSeparators.Replace(*s)); cleaned != "" {
			q.search = &cleaned
		}
	}
	q.departmentID = text("department_id", 64)
	q.employmentStatus = text("employment_status", 32)
	q.isActive = flag("is_active")
	q.isAccessApp = flag("is_access_app")
	q.role = text("role", 32)
	q.scheduleType = f.Enum("schedule_type", optional, []string{"shift", "non_shift"})
	q.page, q.limit = 1, 20
	if n := coerceNumber(f, "page", withDefault, validate.NumOpts{Integer: true, Min: validate.Bound(1)}); n != nil {
		q.page = int(*n)
	}
	if n := coerceNumber(f, "limit", withDefault, validate.NumOpts{Integer: true, Min: validate.Bound(1), Max: validate.Bound(200)}); n != nil {
		q.limit = int(*n)
	}
	q.sortBy = "full_name"
	if s := f.Enum("sort_by", withDefault, listSortColumns); s != nil {
		q.sortBy = *s
	}
	q.asc = true
	if s := f.Enum("sort_order", withDefault, []string{"asc", "desc"}); s != nil {
		q.asc = *s == "asc"
	}
	if !f.Valid() {
		return q, httpx.BadRequest("Parameter tidak valid", f.Issues())
	}
	return q, nil
}

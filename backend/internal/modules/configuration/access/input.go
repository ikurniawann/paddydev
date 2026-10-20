package access

import (
	"math"
	"strings"

	"nuhabit/backend/internal/platform/validate"
)

var (
	optional         = validate.Rule{Optional: true}
	optionalNullable = validate.Rule{Optional: true, Nullable: true}
	menuTypes        = []string{"sidebar", "group"} // MENU_TYPES
)

// codeAndName is roleCreateSchema's z.string().trim().min(1, "Code and name are required").
var codeAndName = validate.StrOpts{Trim: true, Check: func(s string) (string, string, bool) {
	return "too_small", "Code and name are required", s != ""
}}

// roleInput is roleCreateSchema / roleUpdateSchema after parsing. Pointers
// are nil when the key was absent (or null where null is allowed).
type roleInput struct {
	Code, Name, Description *string
	// DescriptionSent is `description !== undefined` (null counts).
	DescriptionSent bool
	IsActive        *bool
}

// parseRoleCreate is roleCreateSchema.
func parseRoleCreate(f *validate.Form) roleInput {
	in := roleInput{
		Code: f.Str("code", validate.Rule{}, codeAndName),
		Name: f.Str("name", validate.Rule{}, codeAndName),
	}
	in.Description = f.Str("description", optionalNullable, validate.StrOpts{})
	in.IsActive = f.Bool("isActive", optional)
	return in
}

// parseRoleUpdate is roleUpdateSchema.
func parseRoleUpdate(f *validate.Form) roleInput {
	in := roleInput{
		Code: f.Str("code", optional, validate.StrOpts{}),
		Name: f.Str("name", optional, validate.StrOpts{}),
	}
	in.Description = f.Str("description", optionalNullable, validate.StrOpts{})
	_, in.DescriptionSent = f.Fields()["description"]
	in.IsActive = f.Bool("isActive", optional)
	return in
}

// menuInput is menuPayloadSchema (or its .partial()) after parsing.
type menuInput struct {
	ParentID, Code, MenuName, Description, RoutePath, Module, MenuType, Icon *string
	OrderNumber                                                              *int
	IsVisible, IsActive, OpenInNewTab                                        *bool
	// PermissionActions is permissionContext.actions; nil when the key was absent.
	PermissionActions []string
	// sent records which nullable keys were present (`!== undefined`).
	sent map[string]bool
}

// parseMenu is menuPayloadSchema, or menuUpdateSchema when partial.
func parseMenu(f *validate.Form, partial bool) menuInput {
	required := validate.Rule{}
	if partial {
		required = optional
	}
	in := menuInput{sent: map[string]bool{}}
	in.ParentID = f.Str("parentId", optionalNullable, validate.StrOpts{})
	in.Code = f.Str("code", required, validate.StrOpts{Trim: true, Min: 1, Max: 100})
	in.MenuName = f.Str("menuName", required, validate.StrOpts{Trim: true, Min: 1, Max: 200})
	in.Description = f.Str("description", optionalNullable, validate.StrOpts{})
	in.RoutePath = f.Str("routePath", optionalNullable, validate.StrOpts{})
	in.Module = f.Str("module", optionalNullable, validate.StrOpts{})
	in.MenuType = f.Enum("menuType", optional, menuTypes)
	in.Icon = f.Str("icon", optionalNullable, validate.StrOpts{})
	in.OrderNumber = f.Int("orderNumber", optional, validate.NumOpts{})
	in.IsVisible = f.Bool("isVisible", optional)
	in.IsActive = f.Bool("isActive", optional)
	in.OpenInNewTab = f.Bool("openInNewTab", optional)
	if _, ok := f.Fields()["permissionContext"]; ok {
		in.PermissionActions = f.Child("permissionContext").Strings("actions", validate.Rule{}, math.MaxInt, validate.StrOpts{})
		if in.PermissionActions == nil {
			in.PermissionActions = []string{}
		}
	}
	for _, key := range []string{"parentId", "description", "routePath", "module", "icon"} {
		_, in.sent[key] = f.Fields()[key]
	}
	return in
}

// grant is one checked row of the permission matrix.
type grant struct {
	MenuID  string
	Actions []string
}

// parsePermissions is rolePermissionsSchema followed by
// normalizeRolePermissions: rows with a menuId that are not unchecked, and
// "read" when no action is listed.
func parsePermissions(f *validate.Form) []grant {
	type row struct {
		menuID    *string
		isGranted *bool
		actions   []string
	}
	var rows []row
	f.List("permissions", validate.Rule{HasDefault: true}, math.MaxInt, func(items *validate.Form, i int, v any) {
		item := items.Item(i, v)
		rows = append(rows, row{
			menuID:    item.Str("menuId", optional, validate.StrOpts{Check: menuIDCheck}),
			isGranted: item.Bool("isGranted", optional),
			actions:   item.Strings("grantedActions", optional, math.MaxInt, validate.StrOpts{}),
		})
	})
	out := []grant{}
	for _, r := range rows {
		if r.menuID == nil || *r.menuID == "" || (r.isGranted != nil && !*r.isGranted) {
			continue
		}
		actions := r.actions
		if len(actions) == 0 {
			// `grantedActions ?? ["read"]`, then replaceIamRolePermissionsInDb
			// turns an empty list into ["read"] too.
			actions = []string{"read"}
		}
		id, _ := canonicalUUID(*r.menuID)
		out = append(out, grant{MenuID: id, Actions: actions})
	}
	return out
}

// canonicalUUID returns s as a lowercase hyphenated uuid. It accepts the
// hyphenated or 32-hex form, optionally in braces, in any case; anything
// else is not a uuid. The HRIS delete guard compares ids, so every menu id
// is stored and checked in this one form (EPIC-061 re-review).
func canonicalUUID(s string) (string, bool) {
	if len(s) >= 2 && s[0] == '{' && s[len(s)-1] == '}' {
		s = s[1 : len(s)-1]
	}
	if len(s) == 36 {
		if s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
			return "", false
		}
		s = s[:8] + s[9:13] + s[14:18] + s[19:23] + s[24:]
	}
	if len(s) != 32 {
		return "", false
	}
	s = strings.ToLower(s)
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", false
		}
	}
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:], true
}

// menuIDCheck: a permission row's menuId is empty (row skipped) or a uuid.
func menuIDCheck(s string) (string, string, bool) {
	_, ok := canonicalUUID(s)
	return "invalid_format", "Invalid UUID", s == "" || ok
}

// lowerTrim is `value?.trim()?.toLowerCase()`.
func lowerTrim(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.ToLower(validate.JSTrim(*s))
	return &v
}

// trimmed is `value?.trim()`.
func trimmed(s *string) *string {
	if s == nil {
		return nil
	}
	v := validate.JSTrim(*s)
	return &v
}

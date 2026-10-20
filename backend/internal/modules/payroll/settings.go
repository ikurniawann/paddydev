package payroll

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strings"

	"nuhabit/backend/internal/modules/payroll/domain"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/validate"
)

// Payroll settings (lib/payroll/settings.ts): the single
// hris.payroll_settings row and hris.payroll_tax_config per tax year.

// settingField is one optional field of the settings/tax-config schemas.
type settingField struct {
	key  string
	kind string // num | int | str | strNull | bool | enum
	opts validate.NumOpts
	min  int // str
}

func pct() validate.NumOpts {
	return validate.NumOpts{Min: validate.Bound(0), Max: validate.Bound(100)}
}
func rupiah() validate.NumOpts { return validate.NumOpts{Min: validate.Bound(0)} }

func nums(kind string, opts validate.NumOpts, keys ...string) []settingField {
	out := make([]settingField, len(keys))
	for i, k := range keys {
		out[i] = settingField{key: k, kind: kind, opts: opts}
	}
	return out
}

func concat(groups ...[]settingField) []settingField {
	var out []settingField
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

var settingsFields = concat(
	[]settingField{{key: "company_name", kind: "str", min: 1}, {key: "npwp", kind: "strNull"}},
	nums("num", pct(), "bpjs_tk_jht_employee", "bpjs_tk_jht_employer", "bpjs_tk_jp_employee", "bpjs_tk_jp_employer",
		"bpjs_tk_jkk", "bpjs_tk_jkm", "bpjs_kes_employee", "bpjs_kes_employer"),
	nums("num", validate.NumOpts{Positive: true}, "bpjs_kes_max_upah"),
	nums("num", pct(), "tapera_employee", "tapera_employer"),
	nums("num", rupiah(), "ptkp_tk_0", "ptkp_tk_1", "ptkp_tk_2", "ptkp_tk_3", "ptkp_k_0", "ptkp_k_1", "ptkp_k_2", "ptkp_k_3",
		"pph21_bracket_1", "pph21_bracket_2", "pph21_bracket_3", "pph21_bracket_4"),
	nums("int", validate.NumOpts{Min: validate.Bound(0), Max: validate.Bound(24)}, "thr_eligible_months"),
	[]settingField{{key: "thr_prorate", kind: "bool"}},
	nums("int", validate.NumOpts{Min: validate.Bound(1), Max: validate.Bound(31)}, "payroll_day"),
	nums("num", validate.NumOpts{Min: validate.Bound(0), Max: validate.Bound(10)}, "overtime_multiplier", "overtime_multiplier_holiday"),
	nums("num", validate.NumOpts{Positive: true, Max: validate.Bound(1000)}, "overtime_hourly_divisor"),
	[]settingField{{key: "late_deduction_mode", kind: "enum"}},
	nums("num", rupiah(), "late_deduction_amount"),
	[]settingField{{key: "late_tier_rules", kind: "tiers"}},
	nums("int", validate.NumOpts{Min: validate.Bound(0), Max: validate.Bound(600)}, "late_min_minutes"),
	[]settingField{{key: "pph21_method", kind: "enum"}, {key: "pph21_bpjs_basis", kind: "enum"}},
	nums("num", rupiah(), "overtime_production_rate"),
	[]settingField{{key: "overtime_requires_clockout", kind: "bool"}, {key: "overtime_rest_from_schedule", kind: "bool"},
		{key: "personal_leave_deduction", kind: "bool"},
		{key: "sick_leave_deduction", kind: "bool"}},
	nums("num", rupiah(), "bpjs_kes_subsidy_amount"),
	nums("num", pct(), "loan_max_installment_percent"),
	nums("int", validate.NumOpts{Min: validate.Bound(1), Max: validate.Bound(10)}, "loan_max_active_per_employee"),
)

var taxFields = concat(
	nums("num", rupiah(), "ptkp_tk_0", "ptkp_tk_1", "ptkp_tk_2", "ptkp_tk_3", "ptkp_k_0", "ptkp_k_1", "ptkp_k_2", "ptkp_k_3",
		"bracket_1_limit"),
	nums("num", pct(), "bracket_1_rate"),
	nums("num", rupiah(), "bracket_2_limit"),
	nums("num", pct(), "bracket_2_rate"),
	nums("num", rupiah(), "bracket_3_limit"),
	nums("num", pct(), "bracket_3_rate"),
	nums("num", rupiah(), "bracket_4_limit"),
	nums("num", pct(), "bracket_4_rate", "bracket_5_rate", "jabatan_expense_percentage"),
	nums("num", rupiah(), "jabatan_expense_max"),
	[]settingField{{key: "is_active", kind: "bool"}},
)

// settingEnums are the accepted values of each enum setting.
var settingEnums = map[string][]string{
	"late_deduction_mode": domain.LateModes,
	"pph21_method":        domain.Pph21Methods,
	"pph21_bpjs_basis":    domain.Pph21BpjsBases,
}

// column is one parsed column to write.
type column struct {
	name  string
	value any
}

// parseSettingFields reads fields in schema order; values maps the coerced
// numbers for the bracket refinement.
func parseSettingFields(f *validate.Form, fields []settingField) (cols []column, values map[string]float64) {
	values = map[string]float64{}
	for _, sf := range fields {
		if _, ok := field(f, sf.key); !ok {
			continue
		}
		switch sf.kind {
		case "num", "int":
			raw, _ := field(f, sf.key)
			if n := domain.JSNumber(raw); !math.IsNaN(n) { // the refinement sees the coerced value
				values[sf.key] = n
			}
			rule := numRule{Rule: optional, NumOpts: sf.opts}
			rule.Integer = sf.kind == "int"
			if n := coerceNum(f, sf.key, rule); n != nil {
				cols = append(cols, column{sf.key, numParam(*n)})
			}
		case "str", "strNull":
			r := optional
			r.Nullable = sf.kind == "strNull"
			v, _ := field(f, sf.key)
			if s := f.Str(sf.key, r, validate.StrOpts{Min: sf.min}); s != nil {
				cols = append(cols, column{sf.key, *s})
			} else if v == nil && r.Nullable {
				cols = append(cols, column{sf.key, nil})
			}
		case "bool":
			if b := f.Bool(sf.key, optional); b != nil {
				cols = append(cols, column{sf.key, *b})
			}
		case "enum":
			if s := f.Enum(sf.key, optional, settingEnums[sf.key]); s != nil {
				cols = append(cols, column{sf.key, *s})
			}
		case "tiers":
			raw, _ := field(f, sf.key)
			if tiers, ok := domain.ParseLateTiers(raw); ok {
				cols = append(cols, column{sf.key, tiers}) // pgx encodes it as JSON
			} else {
				f.Fail(sf.key, "custom", "Aturan tier telat tidak valid: tiap tier wajib min ≥ 1, max ≥ min, days ≥ 0, basis thp/base")
			}
		}
	}
	return cols, values
}

// refineIncreasing is refineIncreasingLimits: cumulative limits sent
// together must strictly increase. zod skips it after a type error.
func refineIncreasing(f *validate.Form, before int, keys []string, values map[string]float64) {
	for _, is := range f.Issues()[before:] {
		if is.Code == "invalid_type" || is.Code == "invalid_value" {
			return
		}
	}
	prevKey := ""
	for _, key := range keys {
		v, ok := values[key]
		if !ok {
			continue
		}
		if prevKey != "" {
			if prev, has := values[prevKey]; has && v <= prev {
				f.Fail(key, "custom", key+" harus lebih besar dari "+prevKey)
			}
		}
		prevKey = key
	}
}

type settingsInput struct {
	settings []column // nil when absent
	hasSet   bool
	tax      []column
	hasTax   bool
	taxYear  int
	compName *string
}

func parseSettingsInput(f *validate.Form) (*settingsInput, error) {
	in := &settingsInput{}
	if _, ok := field(f, "settings"); ok {
		in.hasSet = true
		c := f.Child("settings")
		before := len(f.Issues())
		cols, values := parseSettingFields(c, settingsFields)
		if c.Fields() != nil {
			refineIncreasing(c, before, []string{"pph21_bracket_1", "pph21_bracket_2", "pph21_bracket_3", "pph21_bracket_4"}, values)
		}
		in.settings = cols
		for _, col := range cols {
			if col.name == "company_name" {
				s := col.value.(string)
				in.compName = &s
			}
		}
	}
	if _, ok := field(f, "tax_config"); ok {
		in.hasTax = true
		c := f.Child("tax_config")
		before := len(f.Issues())
		year := coerceInt(c, "tax_year", numRule{NumOpts: validate.NumOpts{Min: validate.Bound(2000), Max: validate.Bound(2100)}})
		cols, values := parseSettingFields(c, taxFields)
		if c.Fields() != nil {
			refineIncreasing(c, before, []string{"bracket_1_limit", "bracket_2_limit", "bracket_3_limit", "bracket_4_limit"}, values)
		}
		if year != nil {
			in.taxYear = *year
		}
		in.tax = cols
	}
	return in, issuesErr(f, "")
}

type settingsView struct {
	Settings  any `json:"settings"`
	TaxConfig any `json:"tax_config"`
}

func (s *service) loadSettings(ctx context.Context, taxYear int) (any, error) {
	settings, _ := queryObj(ctx, s.db, `SELECT * FROM hris.payroll_settings ORDER BY created_at ASC LIMIT 1`)
	tax, _ := queryObj(ctx, s.db, `SELECT * FROM hris.payroll_tax_config WHERE tax_year = $1`, taxYear)
	return struct {
		Settings  any `json:"settings"`
		TaxConfig any `json:"tax_config"`
		TaxYear   int `json:"tax_year"`
	}{settings, tax, taxYear}, nil
}

// upsertSettingsRow updates the existing row or inserts one, returning it.
func (s *service) upsertSettingsRow(ctx context.Context, table, existingID string, cols []column, defaults []column) (*obj, error) {
	cols = append(cols, column{"updated_at", s.clock()})
	var sql string
	var args []any
	if existingID != "" {
		sets := make([]string, len(cols))
		for i, c := range cols {
			sets[i] = c.name + " = $" + itoa(i+1)
			args = append(args, c.value)
		}
		args = append(args, existingID)
		sql = `UPDATE hris.` + table + ` SET ` + strings.Join(sets, ", ") + ` WHERE id = $` + itoa(len(args)) + ` RETURNING *`
	} else {
		all := defaults
		for _, c := range cols {
			replaced := false
			for i := range all {
				if all[i].name == c.name {
					all[i].value, replaced = c.value, true
				}
			}
			if !replaced {
				all = append(all, c)
			}
		}
		names, marks := make([]string, len(all)), make([]string, len(all))
		for i, c := range all {
			names[i], marks[i] = c.name, "$"+itoa(i+1)
			args = append(args, c.value)
		}
		sql = `INSERT INTO hris.` + table + ` (` + strings.Join(names, ", ") + `) VALUES (` + strings.Join(marks, ", ") + `) RETURNING *`
	}
	row, err := queryObj(ctx, s.db, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("save %s: %w", table, opaque(err))
	}
	return row, nil
}

func (s *service) saveSettings(ctx context.Context, in *settingsInput) (any, error) {
	var savedSettings, savedTax any
	if in.hasSet {
		var existing string
		_ = s.db.QueryRow(ctx, `SELECT id::text FROM hris.payroll_settings ORDER BY created_at ASC LIMIT 1`).Scan(&existing)
		name := "Perusahaan"
		if in.compName != nil {
			name = *in.compName
		}
		row, err := s.upsertSettingsRow(ctx, "payroll_settings", existing, in.settings, []column{{"company_name", name}})
		if err != nil {
			return nil, err
		}
		savedSettings = row
	}
	if in.hasTax {
		var existing string
		_ = s.db.QueryRow(ctx, `SELECT id::text FROM hris.payroll_tax_config WHERE tax_year = $1`, in.taxYear).Scan(&existing)
		row, err := s.upsertSettingsRow(ctx, "payroll_tax_config", existing, in.tax, []column{{"tax_year", in.taxYear}})
		if err != nil {
			return nil, err
		}
		savedTax = row
	}
	return settingsView{Settings: savedSettings, TaxConfig: savedTax}, nil
}

// GET /api/hris/payroll-settings?tax_year=2026
func (h *handler) getSettings(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.auth.RequireMenuPrefix(r, iam.HrisCompensation...); err != nil {
		return err
	}
	q := queryForm(r)
	year := coerceInt(q, "tax_year", numRule{Rule: optional})
	if err := issuesErr(q, ""); err != nil {
		return err
	}
	taxYear := h.svc.clock().Year()
	if year != nil && *year != 0 {
		taxYear = *year
	}
	data, err := h.svc.loadSettings(r.Context(), taxYear)
	if err != nil {
		return err
	}
	return writeJSON(w, dataBody{data})
}

// PUT /api/hris/payroll-settings { settings?, tax_config? }
func (h *handler) putSettings(w http.ResponseWriter, r *http.Request) error {
	u, err := h.auth.RequireMenuPrefix(r, iam.HrisCompensation...)
	if err != nil {
		return err
	}
	if !domain.CanWritePayrollSettings(u.Role) {
		return httpx.Forbidden("Hanya super admin dan HRD yang boleh mengubah pengaturan payroll")
	}
	f, err := readJSON(r, "")
	if err != nil {
		return err
	}
	in, err := parseSettingsInput(f)
	if err != nil {
		return err
	}
	if !in.hasSet && !in.hasTax {
		return httpx.BadRequest("Tidak ada perubahan yang dikirim")
	}
	data, err := h.svc.saveSettings(r.Context(), in)
	if err != nil {
		return err
	}
	return writeJSON(w, messageData{Data: data, Message: "Pengaturan payroll tersimpan"})
}

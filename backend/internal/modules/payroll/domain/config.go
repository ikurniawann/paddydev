package domain

import (
	"encoding/json"
	"math"
	"slices"
)

// Allowed setting values (validated by the settings API as well).
var (
	LateModes      = []string{"off", "per_minute", "flat", "tier"}
	Pph21Methods   = []string{"annualized", "ter"}
	Pph21BpjsBases = []string{"employer", "employee", "both"}
)

// ParseLateTiers reads late_tier_rules (jsonb as decoded JSON, raw bytes or
// text). A malformed or empty list is rejected so the default tiers stay.
func ParseLateTiers(v any) ([]LateTier, bool) {
	var raw []byte
	switch x := v.(type) {
	case nil:
		return nil, false
	case []byte:
		raw = x
	case string:
		raw = []byte(x)
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return nil, false
		}
		raw = b
	}
	var tiers []LateTier
	if err := json.Unmarshal(raw, &tiers); err != nil || len(tiers) == 0 {
		return nil, false
	}
	for _, t := range tiers {
		if t.Min < 1 || t.Days < 0 || (t.Max != nil && *t.Max < t.Min) || (t.Basis != "thp" && t.Basis != "base") {
			return nil, false
		}
	}
	return tiers, true
}

// Bracket is one PPh 21 layer: Width rupiah (not a cumulative limit) at Rate.
type Bracket struct {
	Width float64
	Rate  float64
}

// Config is PayrollConfig in lib/payroll/config.ts.
type Config struct {
	Rates struct {
		BpjsTkJht, BpjsTkJp, BpjsKes, Tapera                   float64
		BpjsTkJhtEmployer, BpjsTkJpEmployer, BpjsTkJkkEmployer float64
		BpjsTkJkmEmployer, BpjsKesEmployer, TaperaEmployer     float64
	}
	CapBpjsTk, CapBpjsKes  float64
	Brackets               []Bracket
	PTKP                   map[string]float64 // "TK/0" … "K/3"
	JabatanPercentage      float64
	JabatanMaxPerYear      float64
	OvertimeMultiplier     float64
	OvertimeHolidayMult    float64
	OvertimeHourlyDivisor  float64
	LateMode               string // off | per_minute | flat | tier
	LateAmount             float64
	LateTiers              []LateTier
	LateMinMinutes         float64 // a late day counts toward the tier at >= this many minutes
	LoanMaxInstallmentPct  float64
	LoanMaxActivePerPerson float64
	BpjsKesSubsidyAmount   float64

	Pph21Method              string // annualized | ter
	Pph21BpjsBasis           string // employer | employee | both (BPJS added to the TER DPP)
	OvertimeProductionRate   float64
	OvertimeRequireClockout  bool
	OvertimeRestFromSchedule bool
	PersonalLeaveDeduction   bool // PIP: THP / divisor × personal leave days
	SickLeaveDeduction       bool // PIS: (transport + meal) / divisor × sick days
}

// LateTier is one step of the monthly late-count deduction (Odoo PT/PH/PS):
// Min..Max late days (Max nil = no upper bound) deduct Days daily rates of
// Basis "thp" (take-home pay) or "base" (base salary).
type LateTier struct {
	Min   int     `json:"min"`
	Max   *int    `json:"max,omitempty"`
	Days  float64 `json:"days"`
	Basis string  `json:"basis"`
}

// DefaultLateTiers is the Odoo rule: 0–3 late days a warning only, 4–6 one
// day of THP, 7–9 two days of base salary, 10+ three days of base salary.
func DefaultLateTiers() []LateTier {
	six, nine := 6, 9
	return []LateTier{{4, &six, 1, "thp"}, {7, &nine, 2, "base"}, {10, nil, 3, "base"}}
}

// DefaultConfig is DEFAULT_PAYROLL_CONFIG (2026 rates).
func DefaultConfig() Config {
	var c Config
	c.Rates.BpjsTkJht, c.Rates.BpjsTkJp, c.Rates.BpjsKes, c.Rates.Tapera = 0.02, 0.01, 0.01, 0.025
	c.Rates.BpjsTkJhtEmployer, c.Rates.BpjsTkJpEmployer = 0.037, 0.02
	c.Rates.BpjsTkJkkEmployer, c.Rates.BpjsTkJkmEmployer = 0.0024, 0.003
	c.Rates.BpjsKesEmployer, c.Rates.TaperaEmployer = 0.04, 0.005
	c.CapBpjsTk, c.CapBpjsKes = 10414000, 12000000
	c.Brackets = []Bracket{
		{60_000_000, 0.05}, {190_000_000, 0.15}, {250_000_000, 0.25},
		{4_500_000_000, 0.3}, {math.Inf(1), 0.35},
	}
	c.PTKP = map[string]float64{
		"TK/0": 54_000_000, "TK/1": 58_500_000, "TK/2": 63_000_000, "TK/3": 67_500_000,
		"K/0": 58_500_000, "K/1": 63_000_000, "K/2": 67_500_000, "K/3": 72_000_000,
	}
	c.JabatanPercentage, c.JabatanMaxPerYear = 0.05, 6_000_000
	c.OvertimeMultiplier, c.OvertimeHolidayMult, c.OvertimeHourlyDivisor = 1.5, 2, 173
	c.LateMode, c.LateAmount = "off", 0
	c.LateTiers = DefaultLateTiers()
	c.LoanMaxInstallmentPct, c.LoanMaxActivePerPerson = 30, 1
	c.Pph21Method, c.Pph21BpjsBasis = "annualized", "both"
	c.OvertimeProductionRate, c.OvertimeRequireClockout = 18000, true
	return c
}

// Row is a database row as the TS reads it: a missing key is undefined, a
// nil value is null, numerics are their text.
type Row map[string]any

// num is `toNumber(row?.key, fallback)`: undefined or NaN falls back, null is 0.
func (r Row) num(key string, fallback float64) float64 {
	if r == nil {
		return fallback
	}
	v, ok := r[key]
	if !ok {
		return fallback
	}
	n := JSNumber(v)
	if !Finite(n) {
		return fallback
	}
	return n
}

// flag is a boolean column: a missing key or a non-boolean value falls back.
func (r Row) flag(key string, fallback bool) bool {
	if v, ok := r[key].(bool); ok {
		return v
	}
	return fallback
}

// fraction is `toFraction(row?.key, fallback)`.
func (r Row) fraction(key string, fallback float64) float64 {
	if r == nil {
		return fallback
	}
	if _, ok := r[key]; !ok {
		return fallback
	}
	n := r.num(key, math.NaN())
	if math.IsNaN(n) {
		return fallback
	}
	return n / 100
}

// CumulativeLimitsToBrackets turns cumulative bracket limits (as stored)
// into layer widths; the layer past the last limit is unbounded.
func CumulativeLimitsToBrackets(limits, rates []float64) []Bracket {
	out := make([]Bracket, 0, len(rates))
	previous := 0.0
	for i, rate := range rates {
		if i >= len(limits) {
			out = append(out, Bracket{math.Inf(1), rate})
			continue
		}
		out = append(out, Bracket{math.Max(0, limits[i]-previous), rate})
		previous = limits[i]
	}
	return out
}

// BuildConfig is loadPayrollConfig over the first payroll_settings row and
// the active payroll_tax_config row of the tax year (either may be nil).
func BuildConfig(settings, taxConfig Row) Config {
	d := DefaultConfig()
	c := d
	taxSource := taxConfig
	if taxSource == nil {
		taxSource = settings
	}
	c.PTKP = map[string]float64{
		"TK/0": taxSource.num("ptkp_tk_0", d.PTKP["TK/0"]),
		"TK/1": taxSource.num("ptkp_tk_1", d.PTKP["TK/1"]),
		"TK/2": taxSource.num("ptkp_tk_2", d.PTKP["TK/2"]),
		"TK/3": taxSource.num("ptkp_tk_3", d.PTKP["TK/3"]),
		"K/0":  taxSource.num("ptkp_k_0", d.PTKP["K/0"]),
		"K/1":  taxSource.num("ptkp_k_1", d.PTKP["K/1"]),
		"K/2":  taxSource.num("ptkp_k_2", d.PTKP["K/2"]),
		"K/3":  taxSource.num("ptkp_k_3", d.PTKP["K/3"]),
	}
	switch {
	case taxConfig != nil:
		c.Brackets = CumulativeLimitsToBrackets(
			[]float64{
				taxConfig.num("bracket_1_limit", 60_000_000),
				taxConfig.num("bracket_2_limit", 250_000_000),
				taxConfig.num("bracket_3_limit", 500_000_000),
				taxConfig.num("bracket_4_limit", 5_000_000_000),
			},
			[]float64{
				taxConfig.fraction("bracket_1_rate", 0.05),
				taxConfig.fraction("bracket_2_rate", 0.15),
				taxConfig.fraction("bracket_3_rate", 0.25),
				taxConfig.fraction("bracket_4_rate", 0.3),
				taxConfig.fraction("bracket_5_rate", 0.35),
			})
	case settings != nil:
		rates := make([]float64, len(d.Brackets))
		for i, b := range d.Brackets {
			rates[i] = b.Rate
		}
		c.Brackets = CumulativeLimitsToBrackets(
			[]float64{
				settings.num("pph21_bracket_1", 60_000_000),
				settings.num("pph21_bracket_2", 250_000_000),
				settings.num("pph21_bracket_3", 500_000_000),
				settings.num("pph21_bracket_4", 5_000_000_000),
			}, rates)
	}
	c.Rates.BpjsTkJht = settings.fraction("bpjs_tk_jht_employee", d.Rates.BpjsTkJht)
	c.Rates.BpjsTkJp = settings.fraction("bpjs_tk_jp_employee", d.Rates.BpjsTkJp)
	c.Rates.BpjsKes = settings.fraction("bpjs_kes_employee", d.Rates.BpjsKes)
	c.Rates.Tapera = settings.fraction("tapera_employee", d.Rates.Tapera)
	c.Rates.BpjsTkJhtEmployer = settings.fraction("bpjs_tk_jht_employer", d.Rates.BpjsTkJhtEmployer)
	c.Rates.BpjsTkJpEmployer = settings.fraction("bpjs_tk_jp_employer", d.Rates.BpjsTkJpEmployer)
	c.Rates.BpjsTkJkkEmployer = settings.fraction("bpjs_tk_jkk", d.Rates.BpjsTkJkkEmployer)
	c.Rates.BpjsTkJkmEmployer = settings.fraction("bpjs_tk_jkm", d.Rates.BpjsTkJkmEmployer)
	c.Rates.BpjsKesEmployer = settings.fraction("bpjs_kes_employer", d.Rates.BpjsKesEmployer)
	c.Rates.TaperaEmployer = settings.fraction("tapera_employer", d.Rates.TaperaEmployer)
	c.CapBpjsKes = settings.num("bpjs_kes_max_upah", d.CapBpjsKes)
	c.JabatanPercentage = taxConfig.fraction("jabatan_expense_percentage", d.JabatanPercentage)
	c.JabatanMaxPerYear = taxConfig.num("jabatan_expense_max", d.JabatanMaxPerYear)
	c.OvertimeMultiplier = settings.num("overtime_multiplier", d.OvertimeMultiplier)
	c.OvertimeHolidayMult = settings.num("overtime_multiplier_holiday", d.OvertimeHolidayMult)
	c.OvertimeHourlyDivisor = settings.num("overtime_hourly_divisor", d.OvertimeHourlyDivisor)
	if mode, ok := settings["late_deduction_mode"].(string); ok && slices.Contains(LateModes, mode) {
		c.LateMode = mode
	}
	if tiers, ok := ParseLateTiers(settings["late_tier_rules"]); ok {
		c.LateTiers = tiers
	}
	c.LateMinMinutes = settings.num("late_min_minutes", d.LateMinMinutes)
	if m, ok := settings["pph21_method"].(string); ok && slices.Contains(Pph21Methods, m) {
		c.Pph21Method = m
	}
	if b, ok := settings["pph21_bpjs_basis"].(string); ok && slices.Contains(Pph21BpjsBases, b) {
		c.Pph21BpjsBasis = b
	}
	c.OvertimeProductionRate = settings.num("overtime_production_rate", d.OvertimeProductionRate)
	c.OvertimeRequireClockout = settings.flag("overtime_requires_clockout", d.OvertimeRequireClockout)
	c.OvertimeRestFromSchedule = settings.flag("overtime_rest_from_schedule", d.OvertimeRestFromSchedule)
	c.PersonalLeaveDeduction = settings.flag("personal_leave_deduction", d.PersonalLeaveDeduction)
	c.SickLeaveDeduction = settings.flag("sick_leave_deduction", d.SickLeaveDeduction)
	c.LateAmount = settings.num("late_deduction_amount", d.LateAmount)
	c.LoanMaxInstallmentPct = settings.num("loan_max_installment_percent", d.LoanMaxInstallmentPct)
	c.LoanMaxActivePerPerson = settings.num("loan_max_active_per_employee", d.LoanMaxActivePerPerson)
	c.BpjsKesSubsidyAmount = settings.num("bpjs_kes_subsidy_amount", d.BpjsKesSubsidyAmount)
	return c
}

package payroll

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/validate"
)

// Salary structures (lib/hris/employee-salary-repo.ts): one active version
// per employee and paying entity, a new version deactivates the previous one
// of the same entity. Query errors
// keep their SQLSTATE here (the repo unwraps them with the code), so the
// shared constraint mapping applies.

const salaryRequired = "Employee ID dan base salary wajib diisi"

// salaryField is one column of the salary schemas after base_salary.
type salaryInput struct {
	cols []column
}

func (in *salaryInput) get(name string) (any, bool) {
	for _, c := range in.cols {
		if c.name == name {
			return c.value, true
		}
	}
	return nil, false
}

var salaryAmountKeys = []string{"fixed_allowance", "variable_allowance", "transport_allowance", "meal_allowance",
	"housing_allowance", "loan_deduction", "other_deduction", "position_allowance", "health_allowance"}

// salaryEnums are the EPIC-055 rule columns (hris.employee_salary checks).
var salaryEnums = []struct {
	key  string
	opts []string
}{
	{"pay_basis", []string{"monthly", "monthly_prorated", "daily"}},
	{"transport_mode", []string{"flat", "per_day"}},
	{"meal_mode", []string{"flat", "per_day"}},
	{"position_mode", []string{"flat", "per_day"}},
	{"overtime_scheme", []string{"standard", "production", "freelance", "daily_worker"}},
}

// parseSalaryFields reads salaryFields in schema order.
func parseSalaryFields(f *validate.Form, in *salaryInput) {
	for _, key := range salaryAmountKeys {
		if _, ok := field(f, key); ok {
			if n := coerceNum(f, key, numRule{Rule: optional, NumOpts: validate.NumOpts{Min: validate.Bound(0)}}); n != nil {
				in.cols = append(in.cols, column{key, numParam(*n)})
			}
		}
	}
	for _, e := range salaryEnums {
		if v := f.Enum(e.key, optional, e.opts); v != nil {
			in.cols = append(in.cols, column{e.key, *v})
		}
	}
	if v, ok := field(f, "working_days_divisor"); ok {
		if v == nil {
			in.cols = append(in.cols, column{"working_days_divisor", nil})
		} else if n := coerceNum(f, "working_days_divisor", numRule{Rule: nullish,
			NumOpts: validate.NumOpts{Positive: true, Max: validate.Bound(31)}}); n != nil {
			in.cols = append(in.cols, column{"working_days_divisor", numParam(*n)})
		}
	}
	if _, ok := field(f, "health_min_service_days"); ok {
		if n := coerceInt(f, "health_min_service_days", numRule{Rule: optional,
			NumOpts: validate.NumOpts{Min: validate.Bound(0), Max: validate.Bound(3650)}}); n != nil {
			in.cols = append(in.cols, column{"health_min_service_days", *n})
		}
	}
	if s := f.Str("ptkp_status", optional, validate.StrOpts{Max: 10}); s != nil {
		in.cols = append(in.cols, column{"ptkp_status", *s})
	}
	for _, key := range []string{"is_taxable", "bpjs_tk_enrolled", "bpjs_kes_enrolled", "tapera_enrolled"} {
		if b := f.Bool(key, optional); b != nil {
			in.cols = append(in.cols, column{key, *b})
		}
	}
	nullableStr(f, in, "notes", 2000)
}

// nullableStr adds a z.string().max(n).nullable().optional() field.
func nullableStr(f *validate.Form, in *salaryInput, key string, max int) {
	v, ok := field(f, key)
	if !ok {
		return
	}
	if s := f.Str(key, nullish, validate.StrOpts{Max: max}); s != nil {
		in.cols = append(in.cols, column{key, *s})
	} else if v == nil {
		in.cols = append(in.cols, column{key, nil})
	}
}

// jsDateISO is `new Date(s).toISOString()` for the formats the salary form
// sends; ok is false where JS would throw RangeError (Invalid Date).
func jsDateISO(s string) (string, bool) {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC().Format("2006-01-02T15:04:05.000Z"), true
	}
	if t, ok := validate.ParseJSDate(s); ok {
		return t.UTC().Format("2006-01-02T15:04:05.000Z"), true
	}
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02T15:04:05.000"} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t.UTC().Format("2006-01-02T15:04:05.000Z"), true
		}
	}
	return "", false
}

func (s *service) nowISO() string { return s.clock().UTC().Format("2006-01-02T15:04:05.000Z") }

func (s *service) listSalaries(ctx context.Context, employeeID string) ([]*obj, error) {
	sql, args := `SELECT * FROM hris.employee_salary`, []any{}
	if employeeID != "" {
		sql += ` WHERE employee_id = $1`
		args = append(args, employeeID)
	}
	rows, err := queryObjs(ctx, s.db, sql+` ORDER BY effective_date DESC`, args...)
	if err != nil {
		return nil, err
	}
	if err := s.attachCompanies(ctx, rows); err != nil {
		return nil, err
	}
	return rows, s.stitch(ctx, s.db, rows, embed{"employee", "employee_id", salaryListEmp})
}

func (s *service) createSalary(ctx context.Context, employeeID, companyID string, base float64, effective *string, in *salaryInput) (*obj, error) {
	emp, err := s.brief(ctx, employeeID)
	if err != nil {
		return nil, err
	}
	if emp == nil {
		return nil, httpx.NotFound("Karyawan tidak ditemukan")
	}
	entity, err := s.entityByID(ctx, companyID)
	if err != nil {
		return nil, err
	}
	now := s.nowISO()
	endDate := now
	if effective != nil && *effective != "" {
		iso, ok := jsDateISO(*effective)
		if !ok {
			return nil, errors.New("employee-salary: invalid effective_date (RangeError in TS)")
		}
		endDate = iso
	}
	effectiveDate := now
	if effective != nil && *effective != "" {
		effectiveDate = *effective
	}
	// EPIC-075 R8: one version per employee, entity and effective date. Refuse
	// before touching anything, with a message HR can act on.
	var duplicate bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM hris.employee_salary
		WHERE employee_id = $1 AND company_id = $2::uuid AND effective_date = $3::text::date)`,
		employeeID, entity.ID, effectiveDate).Scan(&duplicate); err != nil {
		return nil, err
	}
	if duplicate {
		return nil, httpx.Conflict("Struktur gaji karyawan ini untuk entitas tersebut dengan tanggal efektif " +
			dateOnly(effectiveDate) + " sudah ada. Ubah lewat Edit atau pilih tanggal efektif lain.")
	}
	val := func(key string, def any) any {
		if v, ok := in.get(key); ok && v != nil && v != "" && v != "0" {
			return v
		}
		return def
	}
	flag := func(key string) any {
		if v, ok := in.get(key); ok {
			return v
		}
		return true
	}
	notes, _ := in.get("notes")
	divisor, _ := in.get("working_days_divisor")
	minService, ok := in.get("health_min_service_days")
	if !ok {
		minService = 0
	}
	// EPIC-075 R7: deactivating the previous version and inserting the new
	// one succeed or fail together, so a failed insert never leaves the
	// employee without an active salary.
	var created *obj
	err = database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE hris.employee_salary SET is_active = false, end_date = $2, updated_at = $3
			WHERE employee_id = $1 AND (company_id = $4::uuid OR company_id IS NULL) AND is_active = true`,
			employeeID, endDate, now, entity.ID); err != nil {
			return err
		}
		var err error
		created, err = s.insertSalary(ctx, tx, employeeID, base, effectiveDate, notes, divisor, minService, entity.ID, val, flag)
		return err
	})
	return created, err
}

// insertSalary inserts the new active version (createSalary's INSERT).
func (s *service) insertSalary(ctx context.Context, q database.Querier, employeeID string, base float64, effectiveDate string,
	notes, divisor, minService any, entityID string, val func(string, any) any, flag func(string) any) (*obj, error) {
	return queryObj(ctx, q, `INSERT INTO hris.employee_salary (employee_id, base_salary, fixed_allowance,
		variable_allowance, transport_allowance, meal_allowance, housing_allowance, loan_deduction, other_deduction,
		ptkp_status, is_taxable, bpjs_tk_enrolled, bpjs_kes_enrolled, tapera_enrolled, effective_date, notes, company_id,
		pay_basis, working_days_divisor, transport_mode, meal_mode, position_allowance, position_mode,
		health_allowance, health_min_service_days, overtime_scheme)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17::uuid,$18,$19,$20,$21,$22,$23,$24,$25,$26) RETURNING *`,
		employeeID, numParam(base), val("fixed_allowance", 0), val("variable_allowance", 0), val("transport_allowance", 0),
		val("meal_allowance", 0), val("housing_allowance", 0), val("loan_deduction", 0), val("other_deduction", 0),
		val("ptkp_status", "TK/0"), flag("is_taxable"), flag("bpjs_tk_enrolled"), flag("bpjs_kes_enrolled"),
		flag("tapera_enrolled"), effectiveDate, notes, entityID,
		val("pay_basis", "monthly"), divisor, val("transport_mode", "flat"), val("meal_mode", "flat"),
		val("position_allowance", 0), val("position_mode", "flat"), val("health_allowance", 0), minService,
		val("overtime_scheme", "standard"))
}

func (s *service) getSalary(ctx context.Context, id string) (*obj, error) {
	row, err := queryObj(ctx, s.db, `SELECT * FROM hris.employee_salary WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, httpx.NotFound("Data salary tidak ditemukan")
	}
	if err := s.attachCompanies(ctx, []*obj{row}); err != nil {
		return nil, err
	}
	return row, s.stitch(ctx, s.db, []*obj{row}, embed{"employee", "employee_id", salaryDetailEm})
}

// errNoRows is the PGRST116 a .single() update raises when nothing matched;
// apiHandler has no mapping for it, so it is a 500.
var errNoRows = errors.New("PGRST116: no rows")

func (s *service) updateSalary(ctx context.Context, id string, in *salaryInput) (*obj, error) {
	cols := append(in.cols, column{"updated_at", s.nowISO()})
	sets, args := make([]string, len(cols)), make([]any, 0, len(cols)+1)
	for i, c := range cols {
		sets[i] = c.name + " = $" + itoa(i+1)
		args = append(args, c.value)
	}
	args = append(args, id)
	row, err := queryObj(ctx, s.db, `UPDATE hris.employee_salary SET `+strings.Join(sets, ", ")+
		` WHERE id = $`+itoa(len(args))+` RETURNING *`, args...)
	if err == nil && row == nil {
		err = errNoRows
	}
	return row, err
}

func (s *service) deactivateSalary(ctx context.Context, id string) error {
	now := s.nowISO()
	_, err := s.db.Exec(ctx, `UPDATE hris.employee_salary SET is_active = false, end_date = $2, updated_at = $3 WHERE id = $1`, id, now, now)
	return err
}

// GET /api/hris/employee-salary[?employee_id=]
func (h *handler) listSalaries(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.auth.RequireMenuPrefix(r, iam.HrisCompensation...); err != nil {
		return err
	}
	rows, err := h.svc.listSalaries(r.Context(), r.URL.Query().Get("employee_id"))
	if err != nil {
		return err
	}
	return writeJSON(w, dataBody{rows})
}

// POST /api/hris/employee-salary
func (h *handler) createSalary(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.auth.RequireMenuPrefix(r, iam.HrisCompensation...); err != nil {
		return err
	}
	f, err := readJSON(r, "")
	if err != nil {
		return err
	}
	before := len(f.Issues())
	employeeID := f.Str("employee_id", validate.Rule{}, validate.StrOpts{Min: 1})
	for i := before; i < len(f.Issues()); i++ {
		f.Issues()[i].Message = salaryRequired
	}
	base := coerceNum(f, "base_salary", numRule{NumOpts: validate.NumOpts{Positive: true}, Message: salaryRequired})
	effective := f.Str("effective_date", nullish, validate.StrOpts{Max: 40})
	company := f.Str("company_id", nullish, validate.StrOpts{})
	in := &salaryInput{}
	parseSalaryFields(f, in)
	if err := issuesErr(f, ""); err != nil {
		return err
	}
	companyID := ""
	if company != nil {
		companyID = *company
	}
	row, err := h.svc.createSalary(r.Context(), *employeeID, companyID, *base, effective, in)
	if err != nil {
		return err
	}
	return writeJSON(w, messageData{Data: row, Message: "Salary structure berhasil dibuat"})
}

// GET /api/hris/employee-salary/{id}
func (h *handler) getSalary(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.auth.RequireMenuPrefix(r, iam.HrisCompensation...); err != nil {
		return err
	}
	row, err := h.svc.getSalary(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	return writeJSON(w, dataBody{row})
}

// PUT /api/hris/employee-salary/{id}
func (h *handler) updateSalary(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.auth.RequireMenuPrefix(r, iam.HrisCompensation...); err != nil {
		return err
	}
	const msg = "Data salary tidak valid"
	f, err := readJSON(r, msg)
	if err != nil {
		return err
	}
	in := &salaryInput{}
	if _, ok := field(f, "base_salary"); ok {
		if n := coerceNum(f, "base_salary", numRule{Rule: optional, NumOpts: validate.NumOpts{Min: validate.Bound(0)}}); n != nil {
			in.cols = append(in.cols, column{"base_salary", numParam(*n)})
		}
	}
	nullableStr(f, in, "end_date", 40)
	parseSalaryFields(f, in)
	if err := issuesErr(f, msg); err != nil {
		return err
	}
	row, err := h.svc.updateSalary(r.Context(), r.PathValue("id"), in)
	if err != nil {
		return err
	}
	return writeJSON(w, messageData{Data: row, Message: "Data salary berhasil diupdate"})
}

// DELETE /api/hris/employee-salary/{id}
func (h *handler) deleteSalary(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.auth.RequireMenuPrefix(r, iam.HrisCompensation...); err != nil {
		return err
	}
	if err := h.svc.deactivateSalary(r.Context(), r.PathValue("id")); err != nil {
		return err
	}
	return writeJSON(w, messageBody{"Data salary berhasil dihapus"})
}

// dateOnly is the YYYY-MM-DD of an effective date as sent (a date or an ISO
// timestamp).
func dateOnly(v string) string {
	if len(v) >= 10 {
		return v[:10]
	}
	return v
}

package payroll

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/modules/payroll/domain"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/validate"
)

// KPI (lib/kpi/{targets-repo,scorecards-repo,rubric,recommendation,
// department-config}.ts). performance.kpi_* belong to this module; the
// monthly snapshot is in kpi_snapshot.go.

// pgUUIDRE accepts what PostgreSQL's uuid input does (hyphens optional
// between groups, optional braces), so ids it would reject are caught first.
var pgUUIDRE = regexp.MustCompile(`^\{?[0-9a-fA-F]{4}-?[0-9a-fA-F]{4}-?[0-9a-fA-F]{4}-?[0-9a-fA-F]{4}-?[0-9a-fA-F]{4}-?[0-9a-fA-F]{4}-?[0-9a-fA-F]{4}-?[0-9a-fA-F]{4}\}?$`)

// errBadUUID is the 22P02 mapping lib/db query errors get.
var errBadUUID = httpx.BadRequest("Format data tidak valid")

/* ── targets ─────────────────────────────────────────────────────────── */

const targetIndicatorEmbed = `(SELECT row_to_json(e) FROM (SELECT code, name, unit, direction
	FROM performance.kpi_indicators WHERE id = t.indicator_id) e) AS indicator`

func (s *service) listTargets(ctx context.Context) ([]*obj, error) {
	rows, err := queryObjs(ctx, s.db, `SELECT t.*, `+targetIndicatorEmbed+`, NULL AS department
		FROM performance.kpi_targets t ORDER BY t.created_at DESC LIMIT 500`)
	if err != nil {
		return nil, err
	}
	if err := s.stitchDepartments(ctx, rows, "department", "department_id", true); err != nil {
		return nil, err
	}
	return rows, s.stitch(ctx, s.db, rows, embed{"employee", "employee_id", personFields})
}

// stitchDepartments sets alias to {id?, name} of the department in column.
func (s *service) stitchDepartments(ctx context.Context, rows []*obj, alias, column string, withID bool) error {
	var ids []string
	for _, r := range rows {
		if id := r.Str(column); id != "" {
			ids = append(ids, id)
		}
	}
	depts := map[string]Department{}
	if len(ids) > 0 {
		var err error
		if depts, err = s.ports.Departments.ByIDs(ctx, s.db, ids); err != nil {
			return err
		}
	}
	for _, r := range rows {
		d, ok := depts[r.Str(column)]
		switch {
		case !ok:
			r.Set(alias, nil)
		case withID:
			r.Set(alias, object("id", d.ID, "name", d.Name))
		default:
			r.Set(alias, object("name", d.Name))
		}
	}
	return nil
}

type targetInput struct {
	IndicatorID                        string
	Target                             float64
	PeriodYear, PeriodMonth            *float64
	RoleCode, DepartmentID, EmployeeID *string
}

func (s *service) createTarget(ctx context.Context, userID string, in targetInput) (*obj, error) {
	row, err := queryObj(ctx, s.db, `INSERT INTO performance.kpi_targets (indicator_id, period_year, period_month,
		role_code, department_id, employee_id, target, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *`,
		in.IndicatorID, numOrNil(in.PeriodYear), numOrNil(in.PeriodMonth), textOrNil(orNil(in.RoleCode)),
		textOrNil(orNil(in.DepartmentID)), textOrNil(orNil(in.EmployeeID)), numParam(in.Target), userID)
	if database.PgCode(err) == "23503" {
		return nil, httpx.BadRequest("Indikator/department/karyawan tidak ditemukan")
	}
	return row, err
}

// GET /api/hris/kpi/targets
func (h *handler) listTargets(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.auth.RequireMenuPrefix(r, iam.HrisPerformance...); err != nil {
		return err
	}
	rows, err := h.svc.listTargets(r.Context())
	if err != nil {
		return err
	}
	return writeJSON(w, dataBody{rows})
}

const targetRequired = "indicator_id dan target (angka ≥ 0) wajib"
const periodInvalid = "Periode tidak valid"

// POST /api/hris/kpi/targets
func (h *handler) createTarget(w http.ResponseWriter, r *http.Request) error {
	u, err := h.auth.RequireMenuPrefix(r, iam.HrisPerformance...)
	if err != nil {
		return err
	}
	f := readOptionalJSON(r)
	var in targetInput
	ind := customStr(f, "indicator_id", targetRequired)
	target := coerceNum(f, "target", numRule{NumOpts: validate.NumOpts{Min: validate.Bound(0)}, Message: targetRequired})
	in.PeriodYear = coerceNum(f, "period_year", numRule{Rule: nullish, NumOpts: validate.NumOpts{Integer: true}, Message: periodInvalid})
	in.PeriodMonth = coerceNum(f, "period_month", numRule{Rule: nullish,
		NumOpts: validate.NumOpts{Integer: true, Min: validate.Bound(1), Max: validate.Bound(12)}, Message: periodInvalid})
	in.RoleCode = f.Str("role_code", nullish, validate.StrOpts{})
	in.DepartmentID = f.Str("department_id", nullish, validate.StrOpts{})
	in.EmployeeID = f.Str("employee_id", nullish, validate.StrOpts{})
	if !hasAbortingIssue(f, 0) {
		scopes := 0
		for _, p := range []*string{in.RoleCode, in.DepartmentID, in.EmployeeID} {
			if p != nil && *p != "" {
				scopes++
			}
		}
		if scopes > 1 {
			f.Fail(nil, "custom", "Pilih satu scope saja (role ATAU department ATAU karyawan)")
		}
	}
	if err := issuesErr(f, ""); err != nil {
		return err
	}
	in.IndicatorID, in.Target = *ind, *target
	row, err := h.svc.createTarget(r.Context(), u.ID, in)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, messageData{Data: row, Message: "Target tersimpan"})
}

// DELETE /api/hris/kpi/targets?id=
func (h *handler) deleteTarget(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.auth.RequireMenuPrefix(r, iam.HrisPerformance...); err != nil {
		return err
	}
	q := queryForm(r)
	id := customStr(q, "id", "id wajib")
	if err := issuesErr(q, ""); err != nil {
		return err
	}
	if _, err := h.svc.db.Exec(r.Context(), `DELETE FROM performance.kpi_targets WHERE id = $1`, *id); err != nil {
		return err
	}
	return writeJSON(w, messageBody{"Target dihapus"})
}

// customStr is z.string({ error: msg }).min(1, msg): required, every issue msg.
func customStr(f *validate.Form, key, msg string) *string {
	before := len(f.Issues())
	s := f.Str(key, validate.Rule{}, validate.StrOpts{Min: 1})
	for i := before; i < len(f.Issues()); i++ {
		f.Issues()[i].Message = msg
	}
	return s
}

// hasAbortingIssue reports a type or enum issue after index before: zod
// skips an object's refinements then.
func hasAbortingIssue(f *validate.Form, before int) bool {
	for _, is := range f.Issues()[before:] {
		if is.Code == "invalid_type" || is.Code == "invalid_value" {
			return true
		}
	}
	return false
}

/* ── scorecards ──────────────────────────────────────────────────────── */

func (s *service) activeIndicators(ctx context.Context) []*obj {
	rows, err := queryObjs(ctx, s.db, `SELECT id, code, name, unit, direction FROM performance.kpi_indicators WHERE is_active = $1`, true)
	if err != nil {
		return nil
	}
	return rows
}

type scorecardQuery struct {
	Year, Month, History *float64
	Team, EmployeeID     *string
}

// scorecardList is the GET body; omitted fields are absent in the TS reply.
type scorecardList struct {
	Data        any      `json:"data"`
	Indicators  any      `json:"indicators,omitempty"`
	PeriodYear  *float64 `json:"period_year,omitempty"`
	PeriodMonth *float64 `json:"period_month,omitempty"`
}

func (s *service) scorecardRows(ctx context.Context, where string, args []any, limit int) ([]*obj, error) {
	rows, err := queryObjs(ctx, s.db, `SELECT * FROM performance.kpi_scorecards WHERE `+where+
		` ORDER BY score DESC NULLS LAST LIMIT `+itoa(limit), args...)
	if err != nil {
		return nil, err
	}
	return rows, s.stitch(ctx, s.db, rows, embed{"employee", "employee_id", runDetailEmp})
}

func (s *service) listScorecards(ctx context.Context, a *actor, q scorecardQuery) (*scorecardList, error) {
	now := s.clock()
	year, month := float64(now.Year()), float64(now.Month())
	if q.Year != nil && *q.Year != 0 {
		year = *q.Year
	}
	if q.Month != nil && *q.Month != 0 {
		month = *q.Month
	}
	indicators := s.activeIndicators(ctx)
	var ind any = []*obj{}
	if indicators != nil {
		ind = indicators
	}
	period := func(out *scorecardList) *scorecardList { out.PeriodYear, out.PeriodMonth = &year, &month; return out }

	if q.Team != nil && *q.Team == "1" {
		if a.EmployeeID == nil {
			return &scorecardList{Data: []any{}, Indicators: ind}, nil
		}
		reports, err := s.ports.Employees.DirectReports(ctx, s.db, *a.EmployeeID)
		if err != nil || len(reports) == 0 {
			return &scorecardList{Data: []any{}, Indicators: ind}, err
		}
		rows, err := s.scorecardRows(ctx, `period_year = $1 AND period_month = $2 AND employee_id::text = ANY($3)`,
			[]any{numParam(year), numParam(month), reports}, 200)
		if err != nil {
			return nil, err
		}
		return period(&scorecardList{Data: rows, Indicators: ind}), nil
	}

	// Non-HR is locked to their own scorecards; HR picks anyone (or "me").
	var filter *string
	switch {
	case !a.PerformanceHR:
		if a.EmployeeID == nil {
			return &scorecardList{Data: []any{}}, nil
		}
		filter = a.EmployeeID
	case q.EmployeeID != nil && *q.EmployeeID == "me":
		filter = a.EmployeeID
	default:
		filter = q.EmployeeID
	}

	if q.History != nil && *q.History > 0 {
		if filter == nil && q.EmployeeID != nil && *q.EmployeeID == "me" {
			return &scorecardList{Data: []any{}, Indicators: ind}, nil
		}
		if filter == nil {
			return nil, httpx.BadRequest("history membutuhkan employee_id")
		}
		rows, err := queryObjs(ctx, s.db, `SELECT * FROM performance.kpi_scorecards WHERE employee_id = $1
			ORDER BY period_year DESC, period_month DESC LIMIT $2`, *filter, numParam(math.Min(24, *q.History)))
		if err != nil {
			return nil, err
		}
		return &scorecardList{Data: rows, Indicators: ind}, nil
	}

	where, args := `period_year = $1 AND period_month = $2`, []any{numParam(year), numParam(month)}
	if filter != nil {
		where += ` AND employee_id = $3`
		args = append(args, *filter)
	}
	rows, err := s.scorecardRows(ctx, where, args, 500)
	if err != nil {
		return nil, err
	}
	return period(&scorecardList{Data: rows, Indicators: ind}), nil
}

type scorecardStatusResult struct {
	Data    any     `json:"data"`
	WaLink  *string `json:"wa_link"`
	Message string  `json:"message"`
}

func (s *service) setScorecardStatus(ctx context.Context, reviewerID, action, id string) (*scorecardStatusResult, error) {
	var status string
	if err := s.db.QueryRow(ctx, `SELECT status FROM performance.kpi_scorecards WHERE id::text = $1`, id).Scan(&status); err != nil {
		return nil, httpx.NotFound("Scorecard tidak ditemukan")
	}
	if action == "finalize" && status != "draft" {
		return nil, httpx.Conflict("Scorecard sudah final")
	}
	if action == "reopen" && status != "final" {
		return nil, httpx.Conflict("Scorecard masih draft")
	}
	now := s.clock()
	var row *obj
	var err error
	if action == "finalize" {
		row, err = queryObj(ctx, s.db, `UPDATE performance.kpi_scorecards SET status = 'final', reviewed_by = $2,
			reviewed_at = $3, updated_at = $3 WHERE id = $1 RETURNING *`, id, reviewerID, now)
	} else {
		row, err = queryObj(ctx, s.db, `UPDATE performance.kpi_scorecards SET status = 'draft', reviewed_by = NULL,
			reviewed_at = NULL, updated_at = $2 WHERE id = $1 RETURNING *`, id, now)
	}
	if err == nil && row == nil {
		err = errNoRows
	}
	if err != nil {
		return nil, err
	}
	out := &scorecardStatusResult{Data: row, Message: "Scorecard dibuka kembali"}
	if action == "finalize" {
		out.Message = "Scorecard difinalkan"
		emp, err := s.brief(ctx, row.Str("employee_id"))
		if err != nil {
			return nil, err
		}
		name, phone := "undefined", (*string)(nil)
		if emp != nil {
			name, phone = emp.FullName, emp.Phone
		}
		scoreText := "belum ada data"
		if row.Get("score") != nil {
			scoreText = domain.FormatNumberID(domain.JSNumber(row.Get("score")), 3)
		}
		month := row.IntPtr("period_month")
		out.WaLink = domain.BuildWaLink(phone, fmt.Sprintf("Halo %s, skor KPI Anda periode %s sudah FINAL: %s. "+
			"Lihat rinciannya di portal karyawan: menu Area Karyawan → KPI Saya.", name, domain.PeriodLabelID(month, row.Int("period_year")), scoreText))
	}
	return out, nil
}

// GET /api/hris/kpi/scorecards?period_year&period_month&employee_id|team=1|history=N
func (h *handler) listScorecards(w http.ResponseWriter, r *http.Request) error {
	a, err := h.requireActor(r)
	if err != nil {
		return err
	}
	f := queryForm(r)
	var q scorecardQuery
	q.Year = coerceNum(f, "period_year", numRule{Rule: optional})
	q.Month = coerceNum(f, "period_month", numRule{Rule: optional})
	q.Team = f.Str("team", optional, validate.StrOpts{})
	q.EmployeeID = f.Str("employee_id", optional, validate.StrOpts{})
	q.History = coerceNum(f, "history", numRule{Rule: optional})
	if err := issuesErr(f, ""); err != nil {
		return err
	}
	out, err := h.svc.listScorecards(r.Context(), a, q)
	if err != nil {
		return err
	}
	return writeJSON(w, out)
}

const actionRequired = "action (finalize|reopen) dan scorecard_id wajib"

// PATCH /api/hris/kpi/scorecards { action, scorecard_id }
func (h *handler) patchScorecard(w http.ResponseWriter, r *http.Request) error {
	u, err := h.auth.RequireMenuPrefix(r, iam.HrisPerformance...)
	if err != nil {
		return err
	}
	f := readOptionalJSON(r)
	before := len(f.Issues())
	action := f.Enum("action", validate.Rule{}, []string{"finalize", "reopen"})
	for i := before; i < len(f.Issues()); i++ {
		f.Issues()[i].Message = actionRequired
	}
	id := customStr(f, "scorecard_id", actionRequired)
	if err := issuesErr(f, ""); err != nil {
		return err
	}
	out, err := h.svc.setScorecardStatus(r.Context(), u.ID, *action, *id)
	if err != nil {
		return err
	}
	return writeJSON(w, out)
}

/* ── rubric ──────────────────────────────────────────────────────────── */

const (
	rubricCode   = "supervisor_rubric"
	rubricMax    = 5.0
	incomplete   = "Parameter tidak lengkap"
	valueRange   = "Nilai rubrik harus 1-5"
	rubricNotesN = 2000
)

type rubricInput struct {
	EmployeeID        string
	PeriodMonth, Year float64
	Value             float64
	Notes             *string
}

type rubricResult struct {
	Score  *float64 `json:"score"`
	Status string   `json:"status"`
}

func (s *service) saveRubric(ctx context.Context, a *actor, in rubricInput) (*rubricResult, error) {
	if !pgUUIDRE.MatchString(in.EmployeeID) {
		return nil, errBadUUID
	}
	// A KPI manager needs the Kinerja menu too; anyone else rates only
	// their direct reports.
	if !(domain.IsKPIManager(a.Role) && a.PerformanceHR) {
		emp, err := s.brief(ctx, in.EmployeeID)
		if err != nil {
			return nil, err
		}
		var reportsTo *string
		if emp != nil {
			reportsTo = emp.ReportingTo
		}
		if a.EmployeeID == nil || reportsTo == nil || *reportsTo != *a.EmployeeID {
			return nil, httpx.Forbidden("Hanya HRD atau atasan langsung yang boleh menilai")
		}
	}
	var indicatorID string
	if err := s.db.QueryRow(ctx, `SELECT id::text FROM performance.kpi_indicators WHERE code = $1`, rubricCode).Scan(&indicatorID); err != nil {
		if database.IsNoRows(err) {
			return nil, fmt.Errorf("Indikator rubrik tidak ditemukan")
		}
		return nil, err
	}
	var status string
	err := s.db.QueryRow(ctx, `SELECT status FROM performance.kpi_scorecards
		WHERE employee_id = $1 AND period_year = $2 AND period_month = $3`, in.EmployeeID, numParam(in.Year), numParam(in.PeriodMonth)).Scan(&status)
	if err != nil && !database.IsNoRows(err) {
		return nil, err
	}
	if status == "final" {
		return nil, httpx.Conflict("Scorecard sudah final — buka kembali dulu untuk mengubah rubrik")
	}
	maxV := rubricMax
	attainment := domain.Attainment("higher_better", &in.Value, &maxV)
	detail, _ := json.Marshal(struct {
		RatedBy string  `json:"rated_by"`
		Notes   *string `json:"notes"`
	}{a.UserID, in.Notes})
	if _, err := s.db.Exec(ctx, `INSERT INTO performance.kpi_snapshots
		   (employee_id, indicator_id, period_year, period_month, actual, target, attainment, sample_size, source_detail)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 1, $8)
		ON CONFLICT (employee_id, indicator_id, period_year, period_month)
		DO UPDATE SET actual = EXCLUDED.actual, target = EXCLUDED.target, attainment = EXCLUDED.attainment,
		  source_detail = EXCLUDED.source_detail, updated_at = now()`,
		in.EmployeeID, indicatorID, numParam(in.Year), numParam(in.PeriodMonth), numParam(in.Value), numParam(rubricMax), numOrNil(attainment), string(detail)); err != nil {
		return nil, err
	}
	score, st, err := s.recomposeScorecard(ctx, in.EmployeeID, in.Year, in.PeriodMonth)
	if err != nil {
		return nil, err
	}
	return &rubricResult{score, st}, nil
}

// recomposeScorecard rebuilds one employee's scorecard from the stored
// snapshots with the role weights; a final scorecard is left alone.
func (s *service) recomposeScorecard(ctx context.Context, employeeID string, year, month float64) (*float64, string, error) {
	role, found, err := s.ports.Employees.KPIRole(ctx, s.db, employeeID)
	if err != nil {
		return nil, "", err
	}
	if !found {
		return nil, "no_components", nil
	}
	rows, err := s.db.Query(ctx, `SELECT ri.indicator_id::text, ri.weight::float, i.code
		FROM performance.kpi_role_indicators ri JOIN performance.kpi_indicators i ON i.id = ri.indicator_id
		WHERE ri.role_code = $1 AND i.is_active = true`, role)
	if err != nil {
		return nil, "", err
	}
	type comp struct {
		indicator, code string
		weight          float64
	}
	comps, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (comp, error) {
		var c comp
		return c, r.Scan(&c.indicator, &c.weight, &c.code)
	})
	if err != nil {
		return nil, "", err
	}
	attain := map[string]*float64{}
	rows, err = s.db.Query(ctx, `SELECT indicator_id::text, attainment::float FROM performance.kpi_snapshots
		WHERE employee_id = $1 AND period_year = $2 AND period_month = $3`, employeeID, numParam(year), numParam(month))
	if err != nil {
		return nil, "", err
	}
	for rows.Next() {
		var id string
		var v *float64
		if err := rows.Scan(&id, &v); err != nil {
			rows.Close()
			return nil, "", err
		}
		attain[id] = v
	}
	rows.Close()
	var existing string
	if err := s.db.QueryRow(ctx, `SELECT status FROM performance.kpi_scorecards
		WHERE employee_id = $1 AND period_year = $2 AND period_month = $3`, employeeID, numParam(year), numParam(month)).Scan(&existing); err != nil && !database.IsNoRows(err) {
		return nil, "", err
	}
	if len(comps) == 0 {
		return nil, "no_components", nil
	}
	if existing == "final" {
		return nil, "skipped_final", nil
	}
	components := make([]domain.ScoreComponent, len(comps))
	for i, c := range comps {
		components[i] = domain.ScoreComponent{Code: c.code, Weight: c.weight, Attainment: attain[c.indicator]}
	}
	score := domain.ComposeScore(components)
	breakdown, _ := json.Marshal(score.Breakdown)
	if _, err := s.db.Exec(ctx, `INSERT INTO performance.kpi_scorecards
		   (employee_id, period_year, period_month, role_code, score, raw_score, used_weight, breakdown, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'draft')
		ON CONFLICT (employee_id, period_year, period_month)
		DO UPDATE SET role_code = EXCLUDED.role_code, score = EXCLUDED.score, raw_score = EXCLUDED.raw_score,
		  used_weight = EXCLUDED.used_weight, breakdown = EXCLUDED.breakdown, updated_at = now()
		WHERE performance.kpi_scorecards.status = 'draft'`,
		employeeID, numParam(year), numParam(month), role, numOrNil(score.Score), numOrNil(score.RawScore), numParam(score.UsedWeight), string(breakdown)); err != nil {
		return nil, "", err
	}
	return score.Score, "updated", nil
}

// POST /api/hris/kpi/rubric { employee_id, period_month, period_year, value, notes? }
func (h *handler) saveRubric(w http.ResponseWriter, r *http.Request) error {
	a, err := h.requireActor(r)
	if err != nil {
		return err
	}
	f := readOptionalJSON(r)
	var in rubricInput
	emp := customStr(f, "employee_id", incomplete)
	month := coerceNum(f, "period_month", numRule{NumOpts: validate.NumOpts{Integer: true, Min: validate.Bound(1), Max: validate.Bound(12)}, Message: incomplete})
	year := coerceNum(f, "period_year", numRule{NumOpts: validate.NumOpts{Integer: true}, Message: incomplete})
	value := coerceNum(f, "value", numRule{NumOpts: validate.NumOpts{Min: validate.Bound(1), Max: validate.Bound(rubricMax)}, Message: valueRange})
	if err := issuesErr(f, ""); err != nil {
		return err
	}
	if v, ok := field(f, "notes"); ok {
		if s, isStr := v.(string); isStr {
			if n := []rune(s); validate.UTF16Len(s) > rubricNotesN {
				s = sliceUTF16(n, rubricNotesN)
			}
			in.Notes = &s
		}
	}
	in.EmployeeID, in.PeriodMonth, in.Year, in.Value = *emp, *month, *year, *value
	res, err := h.svc.saveRubric(r.Context(), a, in)
	if err != nil {
		return err
	}
	return writeJSON(w, messageData{Data: res, Message: "Rubrik tersimpan & scorecard diperbarui"})
}

// sliceUTF16 is String.prototype.slice(0, n) on UTF-16 code units.
func sliceUTF16(runes []rune, n int) string {
	var b strings.Builder
	count := 0
	for _, r := range runes {
		w := 1
		if r > 0xFFFF {
			w = 2
		}
		if count+w > n {
			break
		}
		b.WriteRune(r)
		count += w
	}
	return b.String()
}

/* ── recommendation ──────────────────────────────────────────────────── */

func (s *service) recommendations(ctx context.Context, ids []string) (*obj, error) {
	out := newObj()
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.db.Query(ctx, `WITH ranked AS (
		   SELECT employee_id, score::float AS score, status,
		          ROW_NUMBER() OVER (PARTITION BY employee_id ORDER BY period_year DESC, period_month DESC) AS rn
		     FROM performance.kpi_scorecards
		    WHERE employee_id = ANY($1::uuid[]) AND score IS NOT NULL)
		 SELECT employee_id::text, AVG(score)::float AS avg_score, COUNT(*)::int AS periods,
		        BOOL_OR(status = 'final') AS has_final
		   FROM ranked WHERE rn <= 3 GROUP BY employee_id`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var avg float64
		var periods int
		var final *bool
		if err := rows.Scan(&id, &avg, &periods, &final); err != nil {
			return nil, err
		}
		out.Set(id, object("avg_score", domain.Round(avg*100)/100, "periods", periods, "has_final", final))
	}
	return out, rows.Err()
}

// GET /api/hris/kpi/recommendation?employee_ids=a,b,c
func (h *handler) kpiRecommendation(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.auth.RequireMenuPrefix(r, iam.HrisPerformance...); err != nil {
		return err
	}
	var ids []string
	if raw := r.URL.Query().Get("employee_ids"); raw != "" {
		for _, id := range strings.Split(raw, ",") {
			if id = strings.TrimSpace(id); id != "" {
				ids = append(ids, id)
			}
		}
	}
	if len(ids) > 200 {
		ids = ids[:200]
	}
	valid := ids[:0]
	for _, id := range ids {
		if uuidRE.MatchString(id) {
			valid = append(valid, id)
		}
	}
	out, err := h.svc.recommendations(r.Context(), valid)
	if err != nil {
		return err
	}
	return writeJSON(w, dataBody{out})
}

/* ── department configuration ────────────────────────────────────────── */

// requireKPIManager is requireKpiManager: the Kinerja menu (hris.performance;
// EPIC-057 — HR without it never edits KPI) plus a KPI role.
func (h *handler) requireKPIManager(r *http.Request) (string, error) {
	u, err := h.auth.RequireMenuPrefix(r, iam.HrisPerformance...)
	if err != nil {
		return "", err
	}
	if !domain.IsKPIManager(u.Role) {
		return "", httpx.Forbidden("Hanya pengelola KPI (HRD/admin) yang boleh mengubah konfigurasi")
	}
	return u.FullName, nil
}

// GET /api/hris/kpi-config
func (h *handler) getKPIConfig(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.requireKPIManager(r); err != nil {
		return err
	}
	ctx := r.Context()
	depts, err := h.svc.ports.Departments.All(ctx, h.svc.db)
	if err != nil {
		return err
	}
	deptRows := make([]*obj, len(depts))
	for i, d := range depts {
		deptRows[i] = object("id", d.ID, "name", d.Name)
	}
	indicators, err := queryObjs(ctx, h.svc.db, `SELECT id, code, name, description, unit, direction, default_target
		FROM performance.kpi_indicators WHERE is_active = true ORDER BY name`)
	if err != nil {
		return err
	}
	mappings, err := queryObjs(ctx, h.svc.db, `SELECT department_id, indicator_id, weight, updated_by, updated_at
		FROM performance.kpi_department_indicators`)
	if err != nil {
		return err
	}
	return writeJSON(w, dataBody{object("departments", deptRows, "indicators", indicators, "mappings", mappings)})
}

const weightRange = "Bobot indikator aktif harus 1–100"

type configItem struct {
	IndicatorID string
	Weight      float64
}

// PUT /api/hris/kpi-config { department_id, items: [{ indicator_id, enabled, weight }] }
func (h *handler) putKPIConfig(w http.ResponseWriter, r *http.Request) error {
	editor, err := h.requireKPIManager(r)
	if err != nil {
		return err
	}
	f, err := readJSON(r, "")
	if err != nil {
		return err
	}
	uuidCheck := func(msg string) validate.StrOpts {
		return validate.StrOpts{Check: func(s string) (string, string, bool) { return "invalid_format", msg, uuidRE.MatchString(s) }}
	}
	before := len(f.Issues())
	dept := f.Str("department_id", validate.Rule{}, uuidCheck("Departemen tidak valid"))
	for i := before; i < len(f.Issues()); i++ {
		f.Issues()[i].Message = "Departemen tidak valid"
	}
	var enabled []configItem
	anyEnabled := false
	itemsBefore := len(f.Issues())
	f.List("items", validate.Rule{HasDefault: true}, math.MaxInt, func(sub *validate.Form, i int, v any) {
		item := sub.Item(i, v)
		start := len(item.Issues())
		ind := item.Str("indicator_id", validate.Rule{}, uuidCheck("ID indikator tidak valid"))
		for j := start; j < len(item.Issues()); j++ {
			if item.Issues()[j].Code == "invalid_type" {
				item.Issues()[j].Message = "ID indikator tidak valid"
			}
		}
		on := item.Bool("enabled", optional)
		weight, _ := field(item, "weight")
		if hasAbortingIssue(item, start) {
			return
		}
		isOn := on != nil && *on
		wv := domain.JSNumber(weight)
		if isOn && !(domain.Finite(wv) && wv > 0 && wv <= 100) {
			sub.Fail(i, "custom", weightRange)
		}
		if isOn {
			anyEnabled = true
			enabled = append(enabled, configItem{*ind, wv})
		}
	})
	if !hasAbortingIssue(f, itemsBefore) && !anyEnabled {
		f.Fail("items", "custom", "Minimal satu indikator harus aktif untuk departemen ini")
	}
	if err := issuesErr(f, ""); err != nil {
		return err
	}
	name, err := h.svc.saveKPIConfig(r.Context(), editor, *dept, enabled)
	if err != nil {
		return err
	}
	return writeJSON(w, messageBody{"Konfigurasi KPI departemen " + name + " disimpan — berlaku mulai snapshot bulan berikutnya"})
}

func (s *service) saveKPIConfig(ctx context.Context, editor, deptID string, items []configItem) (string, error) {
	depts, err := s.ports.Departments.ByIDs(ctx, s.db, []string{deptID})
	if err != nil {
		return "", err
	}
	d, ok := depts[deptID]
	if !ok {
		return "", httpx.NotFound("Departemen tidak ditemukan")
	}
	if editor == "" {
		editor = "Pengelola KPI"
	}
	err = database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM performance.kpi_department_indicators WHERE department_id = $1`, deptID); err != nil {
			return err
		}
		for _, it := range items {
			if _, err := tx.Exec(ctx, `INSERT INTO performance.kpi_department_indicators
				(department_id, indicator_id, weight, updated_by, updated_at) VALUES ($1, $2, $3, $4, now())`,
				deptID, it.IndicatorID, numParam(it.Weight), editor); err != nil {
				return err
			}
		}
		return nil
	})
	return d.Name, err
}

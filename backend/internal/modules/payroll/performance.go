package payroll

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/modules/payroll/domain"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/validate"
)

// Quarterly performance reviews (lib/hris/performance-repo.ts).
// performance.review_cycles, performance_reviews, behavioral_* and
// performance_categories belong to this module.

// pgOrder sorts row indexes the way PostgreSQL orders the given column
// arrays (its collation, NULLS rules), so sorting stitched-in names from
// another context keeps the TS ORDER BY. types are the array casts, e.g.
// "text[]"; orderBy refers to the columns as c1, c2, ….
func pgOrder(ctx context.Context, q database.Querier, orderBy string, types []string, cols ...any) ([]int, error) {
	names, args := make([]string, len(types)), make([]string, len(types))
	for i, t := range types {
		names[i] = "c" + itoa(i+1)
		args[i] = "$" + itoa(i+1) + "::" + t
	}
	rows, err := q.Query(ctx, `SELECT (ord - 1)::int FROM unnest(`+strings.Join(args, ", ")+`) WITH ORDINALITY AS t(`+
		strings.Join(names, ", ")+`, ord) ORDER BY `+orderBy+`, ord`, cols...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[int])
}

// actorDepartment is loadActorDepartment: the actor's department and
// whether they head it (active direct reports and a department).
func (s *service) actorDepartment(ctx context.Context, a *actor) (*string, bool, error) {
	if a.EmployeeID == nil {
		return nil, false, nil
	}
	b, err := s.brief(ctx, *a.EmployeeID)
	if err != nil || b == nil {
		return nil, false, err
	}
	reports, err := s.ports.Employees.DirectReports(ctx, s.db, *a.EmployeeID)
	return b.DepartmentID, len(reports) > 0 && b.DepartmentID != nil, err
}

// employeeName is the employee's full name, fallback when unknown.
func (s *service) employeeName(ctx context.Context, id *string, fallback string) (string, error) {
	if id == nil {
		return fallback, nil
	}
	b, err := s.brief(ctx, *id)
	if err != nil || b == nil {
		return fallback, err
	}
	return b.FullName, nil
}

/* ── cycles ──────────────────────────────────────────────────────────── */

func (s *service) listReviewCycles(ctx context.Context) ([]*obj, error) {
	return queryObjs(ctx, s.db, `SELECT c.id, c.name, c.period_year, c.period_quarter,
		       c.start_date::text, c.end_date::text, c.status, c.created_by_name,
		       count(r.id)::int AS total_reviews,
		       count(r.id) FILTER (WHERE r.status = 'final')::int AS final_reviews,
		       count(r.id) FILTER (WHERE r.total_behavioral_score > 0)::int AS rated_reviews
		  FROM performance.review_cycles c
		  LEFT JOIN performance.performance_reviews r ON r.cycle_id = c.id
		 GROUP BY c.id
		 ORDER BY c.period_year DESC, c.period_quarter DESC`)
}

// openCycle creates the cycle, a draft review per active employee with a
// copy of the active behavioural standards, then prefills each review's
// work result from the quarter's KPI (outside the transaction, idempotent).
func (s *service) openCycle(ctx context.Context, a *actor, year, quarter int) (*messageData, error) {
	if !a.PerformanceHR {
		return nil, httpx.Forbidden("Hanya HRD yang boleh membuka siklus review")
	}
	label := domain.QuarterLabel(year, quarter)
	var exists string
	err := s.db.QueryRow(ctx, `SELECT id::text FROM performance.review_cycles WHERE period_year = $1 AND period_quarter = $2`, year, quarter).Scan(&exists)
	if err == nil {
		return nil, httpx.Conflict("Siklus " + label + " sudah ada")
	}
	if !database.IsNoRows(err) {
		return nil, err
	}
	creator, err := s.employeeName(ctx, a.EmployeeID, "HRD")
	if err != nil {
		return nil, err
	}
	start, end := domain.QuarterRange(year, quarter)
	employees, err := s.ports.Employees.Active(ctx, s.db)
	if err != nil {
		return nil, err
	}
	ids, depts := make([]string, len(employees)), make([]*string, len(employees))
	for i, e := range employees {
		ids[i], depts[i] = e.ID, e.DepartmentID
	}
	var reviewIDs []string
	err = database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		var cycleID string
		if err := tx.QueryRow(ctx, `INSERT INTO performance.review_cycles
			   (name, period_year, period_quarter, start_date, end_date, created_by_name)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING id::text`, label, year, quarter, start, end, creator).Scan(&cycleID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `INSERT INTO performance.performance_reviews
			   (cycle_id, employee_id, employee_department_id, period_label, start_date, end_date, status)
			SELECT $1, e.id, e.dept, $2, $3, $4, 'draft'
			  FROM unnest($5::uuid[], $6::uuid[]) WITH ORDINALITY AS e(id, dept, ord) ORDER BY e.ord
			RETURNING id::text`, cycleID, label, start, end, ids, depts)
		if err != nil {
			return err
		}
		if reviewIDs, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO performance.behavioral_review_items
			   (review_id, employee_id, value_name, competency, behavioral_standard,
			    score_1_description, score_2_description, score_3_description,
			    score_4_description, score_5_description, weight, item_order)
			SELECT r.id, r.employee_id, s.value_name, s.competency_name,
			       s.standard_description, s.score_1_description, s.score_2_description,
			       s.score_3_description, s.score_4_description, s.score_5_description,
			       s.weight, row_number() OVER (PARTITION BY r.id ORDER BY s.created_at)
			  FROM performance.performance_reviews r
			  JOIN performance.behavioral_standards s ON s.is_active = true
			 WHERE r.cycle_id = $1`, cycleID)
		return err
	})
	if err != nil {
		return nil, err
	}
	for _, id := range reviewIDs {
		if err := s.recalcReview(ctx, id); err != nil {
			return nil, err
		}
	}
	return &messageData{Data: object("review_count", len(reviewIDs)),
		Message: fmt.Sprintf("Siklus %s dibuka — %d draft review dibuat", label, len(reviewIDs))}, nil
}

// recalcReview recomputes a review's totals from its sources; final reviews
// are left alone.
func (s *service) recalcReview(ctx context.Context, reviewID string) error {
	var employeeID, status string
	var project *string
	var year, quarter int
	err := s.db.QueryRow(ctx, `SELECT r.employee_id::text, COALESCE(r.status, ''), r.total_project_score::text,
		       c.period_year, c.period_quarter
		  FROM performance.performance_reviews r JOIN performance.review_cycles c ON c.id = r.cycle_id
		 WHERE r.id = $1`, reviewID).Scan(&employeeID, &status, &project, &year, &quarter)
	if database.IsNoRows(err) || status == "final" {
		return nil
	}
	if err != nil {
		return err
	}
	var avg *string
	if err := s.db.QueryRow(ctx, `SELECT AVG(score)::numeric(6,2)::text FROM performance.kpi_scorecards
		WHERE employee_id = $1 AND period_year = $2 AND period_month = ANY($3::int[]) AND score IS NOT NULL`,
		employeeID, year, domain.QuarterMonths(quarter)).Scan(&avg); err != nil {
		return err
	}
	rows, err := s.db.Query(ctx, `SELECT score, weight::text FROM performance.behavioral_review_items WHERE review_id = $1`, reviewID)
	if err != nil {
		return err
	}
	items, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.BehaviorItem, error) {
		var it domain.BehaviorItem
		var w *string
		err := r.Scan(&it.Score, &w)
		it.Weight = textOrNil(w)
		return it, err
	})
	if err != nil {
		return err
	}
	var work, proj *float64
	if avg != nil {
		v := domain.JSNumber(*avg)
		work = &v
	}
	if project != nil && domain.JSNumber(*project) != 0 {
		v := domain.JSNumber(*project)
		proj = &v
	}
	behavior := domain.BehaviorScore(items)
	grand := domain.CombineReviewScores(work, behavior, proj)
	orZero := func(p *float64) string {
		if p == nil {
			return "0"
		}
		return numParam(*p)
	}
	_, err = s.db.Exec(ctx, `UPDATE performance.performance_reviews
		SET total_work_result_score = $2, total_behavioral_score = $3, grand_total_score = $4,
		    category = (SELECT category_name FROM performance.performance_categories
		                 WHERE $4::numeric BETWEEN min_score AND max_score ORDER BY min_score DESC LIMIT 1),
		    updated_at = now()
		WHERE id = $1`, reviewID, orZero(work), orZero(behavior), numOrNil(grand))
	return err
}

/* ── reviews & realtime KPI ──────────────────────────────────────────── */

func (s *service) listReviews(ctx context.Context, a *actor, cycleID string) (*obj, error) {
	deptID, isHead, err := s.actorDepartment(ctx, a)
	if err != nil {
		return nil, err
	}
	where, args := "", []any{cycleID}
	if !a.PerformanceHR {
		switch {
		case isHead:
			where = ` AND (r.employee_department_id = $2 OR r.employee_id = $3)`
			args = append(args, *deptID, *a.EmployeeID)
		case a.EmployeeID != nil:
			where = ` AND r.employee_id = $2`
			args = append(args, *a.EmployeeID)
		default:
			return object("reviews", []any{}, "can_review", false), nil
		}
	}
	rows, err := queryObjs(ctx, s.db, `SELECT r.id, r.employee_id, NULL AS full_name, NULL AS nip, NULL AS department_name,
		       r.status, r.total_work_result_score, r.total_behavioral_score,
		       r.total_project_score, r.grand_total_score, r.category,
		       r.employee_sign_date::text, r.reviewer_sign_date::text, r.reviewer_name,
		       (r.self_assessment IS NOT NULL) AS self_done,
		       (SELECT count(*) FROM performance.behavioral_review_items i
		         WHERE i.review_id = r.id AND i.score IS NOT NULL)::int AS rated_items,
		       (SELECT count(*) FROM performance.behavioral_review_items i WHERE i.review_id = r.id)::int AS total_items,
		       r.employee_department_id
		  FROM performance.performance_reviews r
		 WHERE r.cycle_id = $1`+where, args...)
	if err != nil {
		return nil, err
	}
	reviews, err := s.withEmployeeNames(ctx, rows)
	if err != nil {
		return nil, err
	}
	return object("reviews", reviews, "can_review", a.PerformanceHR || isHead, "is_hr", a.PerformanceHR, "my_employee_id", a.EmployeeID), nil
}

// withEmployeeNames fills full_name, nip and department_name, drops rows
// whose employee is gone (the TS inner join) and orders by department name
// NULLS LAST, then full name. The helper column employee_department_id is
// removed.
func (s *service) withEmployeeNames(ctx context.Context, rows []*obj) ([]*obj, error) {
	var empIDs, deptIDs []string
	for _, r := range rows {
		empIDs = append(empIDs, r.Str("employee_id"))
		if d := r.Str("employee_department_id"); d != "" {
			deptIDs = append(deptIDs, d)
		}
	}
	briefs, err := s.ports.Employees.Briefs(ctx, s.db, empIDs)
	if err != nil {
		return nil, err
	}
	depts := map[string]Department{}
	if len(deptIDs) > 0 {
		if depts, err = s.ports.Departments.ByIDs(ctx, s.db, deptIDs); err != nil {
			return nil, err
		}
	}
	var kept []*obj
	var deptNames, fullNames []*string
	for _, r := range rows {
		b, ok := briefs[r.Str("employee_id")]
		if !ok {
			continue
		}
		var deptName *string
		if d, ok := depts[r.Str("employee_department_id")]; ok {
			deptName = &d.Name
		}
		full := b.FullName
		r.Set("full_name", full).Set("nip", b.NIP).Set("department_name", deptName).Del("employee_department_id")
		kept = append(kept, r)
		deptNames, fullNames = append(deptNames, deptName), append(fullNames, &full)
	}
	if len(kept) == 0 {
		return []*obj{}, nil
	}
	order, err := pgOrder(ctx, s.db, "c1 NULLS LAST, c2", []string{"text[]", "text[]"}, deptNames, fullNames)
	if err != nil {
		return nil, err
	}
	out := make([]*obj, len(order))
	for i, idx := range order {
		out[i] = kept[idx]
	}
	return out, nil
}

func (s *service) realtimeKPI(ctx context.Context, a *actor, year, quarter float64) (*obj, error) {
	if year < 2020 || year > 2100 || quarter < 1 || quarter > 4 {
		return nil, httpx.BadRequest("Periode tidak valid")
	}
	months := domain.QuarterMonths(int(quarter))
	deptID, isHead, err := s.actorDepartment(ctx, a)
	if err != nil {
		return nil, err
	}
	all, err := s.ports.Employees.Active(ctx, s.db)
	if err != nil {
		return nil, err
	}
	var employees []EmployeeBrief
	switch {
	case a.PerformanceHR:
		employees = all
	case isHead:
		for _, e := range all {
			if e.DepartmentID != nil && *e.DepartmentID == *deptID {
				employees = append(employees, e)
			}
		}
	case a.EmployeeID != nil:
		for _, e := range all {
			if e.ID == *a.EmployeeID {
				employees = append(employees, e)
			}
		}
	default:
		return object("employees", []any{}, "year", year, "quarter", quarter, "months", months,
			"is_hr", false, "my_employee_id", nil), nil
	}

	ids := make([]string, len(employees))
	var deptIDs []string
	for i, e := range employees {
		ids[i] = e.ID
		if e.DepartmentID != nil {
			deptIDs = append(deptIDs, *e.DepartmentID)
		}
	}
	depts := map[string]Department{}
	if len(deptIDs) > 0 {
		if depts, err = s.ports.Departments.ByIDs(ctx, s.db, deptIDs); err != nil {
			return nil, err
		}
	}
	stats, err := queryObjs(ctx, s.db, `SELECT s.employee_id::text AS employee_id, AVG(s.score)::numeric(6,2) AS avg_score,
		       AVG(s.score)::text AS sort_avg,
		       json_agg(json_build_object('month', s.period_month, 'score', s.score, 'status', s.status)
		                ORDER BY s.period_month) AS months
		  FROM performance.kpi_scorecards s
		 WHERE s.employee_id::text = ANY($1) AND s.period_year = $2 AND s.period_month = ANY($3::int[])
		 GROUP BY s.employee_id`, ids, numParam(year), months)
	if err != nil {
		return nil, err
	}
	byEmp := map[string]*obj{}
	for _, st := range stats {
		byEmp[st.Str("employee_id")] = st
	}
	rows := make([]*obj, len(employees))
	avgs, names := make([]*string, len(employees)), make([]*string, len(employees))
	for i, e := range employees {
		var deptName, avg any
		if e.DepartmentID != nil {
			if d, ok := depts[*e.DepartmentID]; ok {
				deptName = d.Name
			}
		}
		var monthsAgg any
		if st, ok := byEmp[e.ID]; ok {
			avg, monthsAgg = st.Get("avg_score"), st.Get("months")
			avgs[i] = st.StrPtr("sort_avg") // the TS orders by the unrounded average
		}
		name := e.FullName
		names[i] = &name
		rows[i] = object("id", e.ID, "full_name", e.FullName, "department_name", deptName, "avg_score", avg, "months", monthsAgg)
	}
	sorted := []*obj{}
	if len(rows) > 0 {
		order, err := pgOrder(ctx, s.db, "c1 DESC NULLS LAST, c2", []string{"numeric[]", "text[]"}, avgs, names)
		if err != nil {
			return nil, err
		}
		for _, idx := range order {
			sorted = append(sorted, rows[idx])
		}
	}
	return object("employees", sorted, "year", year, "quarter", quarter, "months", months,
		"is_hr", a.PerformanceHR, "my_employee_id", a.EmployeeID), nil
}

/* ── one review ──────────────────────────────────────────────────────── */

type reviewAccess struct {
	EmployeeID, Status string
	DepartmentID       *string
	Year, Quarter      int
	IsOwner, IsHead    bool
}

func (s *service) reviewAccess(ctx context.Context, a *actor, id string) (*reviewAccess, error) {
	var r reviewAccess
	err := s.db.QueryRow(ctx, `SELECT r.employee_id::text, COALESCE(r.status, ''), r.employee_department_id::text,
		       c.period_year, c.period_quarter
		  FROM performance.performance_reviews r JOIN performance.review_cycles c ON c.id = r.cycle_id
		 WHERE r.id = $1`, id).Scan(&r.EmployeeID, &r.Status, &r.DepartmentID, &r.Year, &r.Quarter)
	if database.IsNoRows(err) {
		return nil, httpx.NotFound("Review tidak ditemukan")
	}
	if err != nil {
		return nil, err
	}
	if a.EmployeeID != nil && r.DepartmentID != nil {
		if r.IsHead, err = s.isDepartmentHead(ctx, *a.EmployeeID, *r.DepartmentID, true); err != nil {
			return nil, err
		}
	}
	r.IsOwner = a.EmployeeID != nil && *a.EmployeeID == r.EmployeeID
	return &r, nil
}

func (s *service) reviewDetail(ctx context.Context, a *actor, id string) (*obj, error) {
	access, err := s.reviewAccess(ctx, a, id)
	if err != nil {
		return nil, err
	}
	if !a.PerformanceHR && !access.IsHead && !access.IsOwner {
		return nil, httpx.Forbidden("Tidak berhak melihat review ini")
	}
	review, err := queryObj(ctx, s.db, `SELECT r.*, r.start_date::text AS start_date, r.end_date::text AS end_date,
		       r.reviewee_sign_date::text AS reviewee_sign_date,
		       r.reviewer_sign_date::text AS reviewer_sign_date,
		       r.employee_sign_date::text AS employee_sign_date,
		       NULL AS full_name, NULL AS nip, NULL AS department_name,
		       c.name AS cycle_name, c.status AS cycle_status
		  FROM performance.performance_reviews r
		  JOIN performance.review_cycles c ON c.id = r.cycle_id
		 WHERE r.id = $1`, id)
	if err != nil {
		return nil, err
	}
	if review != nil {
		emp, err := s.brief(ctx, access.EmployeeID)
		if err != nil {
			return nil, err
		}
		if emp == nil {
			review = nil // the TS inner join on hris.employees finds no row
		} else {
			var deptName any
			if access.DepartmentID != nil {
				depts, err := s.ports.Departments.ByIDs(ctx, s.db, []string{*access.DepartmentID})
				if err != nil {
					return nil, err
				}
				if d, ok := depts[*access.DepartmentID]; ok {
					deptName = d.Name
				}
			}
			review.Set("full_name", emp.FullName).Set("nip", emp.NIP).Set("department_name", deptName)
		}
	}
	items, err := queryObjs(ctx, s.db, `SELECT id, value_name, competency, behavioral_standard,
		       score_1_description, score_2_description, score_3_description,
		       score_4_description, score_5_description, weight, score, notes, item_order
		  FROM performance.behavioral_review_items WHERE review_id = $1 ORDER BY item_order, value_name`, id)
	if err != nil {
		return nil, err
	}
	kpi, err := queryObjs(ctx, s.db, `SELECT period_month, score, status FROM performance.kpi_scorecards
		WHERE employee_id = $1 AND period_year = $2 AND period_month = ANY($3::int[]) ORDER BY period_month`,
		access.EmployeeID, access.Year, domain.QuarterMonths(access.Quarter))
	if err != nil {
		return nil, err
	}
	return object("review", review, "items", items, "kpi_months", kpi,
		"can_rate", a.PerformanceHR || access.IsHead, "can_finalize", a.PerformanceHR, "is_owner", access.IsOwner), nil
}

type reviewAction struct {
	Action              string
	Text, Notes, ItemID *string
	Score               float64
	ProjectScore        *float64
}

func trimOrNil(s *string) *string {
	if s == nil {
		return nil
	}
	if t := validate.JSTrim(*s); t != "" {
		return &t
	}
	return nil
}

func (s *service) applyReviewAction(ctx context.Context, a *actor, id string, in reviewAction) (string, error) {
	access, err := s.reviewAccess(ctx, a, id)
	if err != nil {
		return "", err
	}
	if access.Status == "final" && in.Action != "sign" {
		return "", httpx.BadRequest("Review sudah final — tidak bisa diubah")
	}
	canRate := a.PerformanceHR || access.IsHead
	myName := "HRD"
	if a.EmployeeID != nil {
		if myName, err = s.employeeName(ctx, a.EmployeeID, "—"); err != nil {
			return "", err
		}
	}
	exec := func(sql string, args ...any) error { _, err := s.db.Exec(ctx, sql, args...); return err }

	switch in.Action {
	case "self_assessment":
		if !access.IsOwner {
			return "", httpx.Forbidden("Hanya karyawan yang bersangkutan yang boleh mengisi self assessment")
		}
		return "Self assessment tersimpan", exec(`UPDATE performance.performance_reviews
			SET self_assessment = $2, updated_at = now() WHERE id = $1`, id, trimOrNil(in.Text))

	case "rate_item":
		if !canRate {
			return "", httpx.Forbidden("Hanya Head Division departemen ini (atau HRD) yang boleh menilai perilaku")
		}
		itemID := ""
		if in.ItemID != nil {
			itemID = *in.ItemID
		}
		if !uuidRE.MatchString(itemID) {
			return "", httpx.BadRequest("ID item tidak valid")
		}
		score, msg := domain.BehaviorRating(in.Score)
		if msg != "" {
			return "", httpx.BadRequest(msg)
		}
		tag, err := s.db.Exec(ctx, `UPDATE performance.behavioral_review_items
			SET score = $3::int, weighted_score = (weight * $3::numeric / 5.0), notes = $4, updated_at = now()
			WHERE id = $2 AND review_id = $1`, id, itemID, score, trimOrNil(in.Notes))
		if err != nil {
			return "", err
		}
		if tag.RowsAffected() == 0 {
			return "", httpx.BadRequest("Item bukan milik review ini")
		}
		if err := exec(`UPDATE performance.performance_reviews
			SET reviewer_id = COALESCE(reviewer_id, $2), reviewer_name = $3, updated_at = now() WHERE id = $1`,
			id, a.EmployeeID, myName); err != nil {
			return "", err
		}
		return "Penilaian tersimpan", s.recalcReview(ctx, id)

	case "reviewer_notes":
		if !canRate {
			return "", httpx.Forbidden("Hanya Head Division (atau HRD) yang boleh mengisi catatan reviewer")
		}
		project, msg := domain.ProjectScore(in.ProjectScore)
		if msg != "" {
			return "", httpx.BadRequest(msg)
		}
		if err := exec(`UPDATE performance.performance_reviews
			SET reviewer_notes = $2, total_project_score = COALESCE($3::numeric, 0),
			    reviewer_id = COALESCE(reviewer_id, $4), reviewer_name = $5, updated_at = now()
			WHERE id = $1`, id, trimOrNil(in.Text), numOrNil(project), a.EmployeeID, myName); err != nil {
			return "", err
		}
		return "Catatan reviewer tersimpan", s.recalcReview(ctx, id)

	case "sign":
		if access.IsOwner {
			return "Ditandatangani sebagai karyawan", exec(`UPDATE performance.performance_reviews
				SET employee_sign_date = CURRENT_DATE, updated_at = now() WHERE id = $1`, id)
		}
		if !canRate {
			return "", httpx.Forbidden("Tidak berhak menandatangani review ini")
		}
		return "Ditandatangani sebagai reviewer", exec(`UPDATE performance.performance_reviews
			SET reviewer_sign_date = CURRENT_DATE, reviewer_id = COALESCE(reviewer_id, $2),
			    reviewer_name = COALESCE(reviewer_name, $3), updated_at = now()
			WHERE id = $1`, id, a.EmployeeID, myName)

	case "finalize":
		if !a.PerformanceHR {
			return "", httpx.Forbidden("Hanya HRD yang boleh memfinalkan review")
		}
		if err := s.recalcReview(ctx, id); err != nil {
			return "", err
		}
		return "Review difinalkan — nilai terkunci", exec(`UPDATE performance.performance_reviews
			SET status = 'final', manager_id = $2, manager_notes = COALESCE(manager_notes, $3), updated_at = now()
			WHERE id = $1`, id, a.EmployeeID, trimOrNil(in.Text))
	}
	return "", httpx.BadRequest("Aksi tidak dikenal")
}

/* ── handlers ────────────────────────────────────────────────────────── */

// GET /api/hris/performance/cycles
func (h *handler) listReviewCycles(w http.ResponseWriter, r *http.Request) error {
	a, err := h.requireActor(r)
	if err != nil {
		return err
	}
	cycles, err := h.svc.listReviewCycles(r.Context())
	if err != nil {
		return err
	}
	return writeJSON(w, dataBody{object("cycles", cycles, "is_hr", a.PerformanceHR)})
}

// POST /api/hris/performance/cycles { period_year, period_quarter }
func (h *handler) openReviewCycle(w http.ResponseWriter, r *http.Request) error {
	a, err := h.requireActor(r)
	if err != nil {
		return err
	}
	f, err := readJSON(r, "")
	if err != nil {
		return err
	}
	year := coerceInt(f, "period_year", numRule{NumOpts: validate.NumOpts{Min: validate.Bound(2020), Max: validate.Bound(2100)}, Message: "Tahun tidak valid"})
	quarter := coerceInt(f, "period_quarter", numRule{NumOpts: validate.NumOpts{Min: validate.Bound(1), Max: validate.Bound(4)}, Message: "Kuartal harus 1–4"})
	if err := issuesErr(f, ""); err != nil {
		return err
	}
	out, err := h.svc.openCycle(r.Context(), a, *year, *quarter)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, struct {
		Message string `json:"message"`
		Data    any    `json:"data"`
	}{out.Message, out.Data})
}

// GET /api/hris/performance/reviews?cycle_id
func (h *handler) listReviews(w http.ResponseWriter, r *http.Request) error {
	a, err := h.requireActor(r)
	if err != nil {
		return err
	}
	q := queryForm(r)
	const msg = "Parameter cycle_id wajib"
	before := len(q.Issues())
	cycle := q.Str("cycle_id", validate.Rule{}, validate.StrOpts{Check: func(s string) (string, string, bool) {
		return "invalid_format", msg, uuidRE.MatchString(s)
	}})
	for i := before; i < len(q.Issues()); i++ {
		q.Issues()[i].Message = msg
	}
	if err := issuesErr(q, ""); err != nil {
		return err
	}
	data, err := h.svc.listReviews(r.Context(), a, *cycle)
	if err != nil {
		return err
	}
	return writeJSON(w, dataBody{data})
}

// GET /api/hris/performance/realtime?year&quarter
func (h *handler) realtimeKPI(w http.ResponseWriter, r *http.Request) error {
	a, err := h.requireActor(r)
	if err != nil {
		return err
	}
	q := queryForm(r)
	year := coerceNum(q, "year", numRule{Rule: optional})
	quarter := coerceNum(q, "quarter", numRule{Rule: optional})
	if err := issuesErr(q, "Periode tidak valid"); err != nil {
		return err
	}
	now := h.svc.clock()
	y, qt := float64(now.Year()), float64((int(now.Month())-1)/3+1)
	if year != nil && *year != 0 {
		y = *year
	}
	if quarter != nil && *quarter != 0 {
		qt = *quarter
	}
	data, err := h.svc.realtimeKPI(r.Context(), a, y, qt)
	if err != nil {
		return err
	}
	return writeJSON(w, dataBody{data})
}

// GET /api/hris/performance/reviews/{id}
func (h *handler) reviewDetail(w http.ResponseWriter, r *http.Request) error {
	a, err := h.requireActor(r)
	if err != nil {
		return err
	}
	id, err := requireUUID(r.PathValue("id"), "")
	if err != nil {
		return err
	}
	data, err := h.svc.reviewDetail(r.Context(), a, id)
	if err != nil {
		return err
	}
	return writeJSON(w, dataBody{data})
}

// PATCH /api/hris/performance/reviews/{id} { action, text?, item_id?, score?, notes?, project_score? }
func (h *handler) reviewAction(w http.ResponseWriter, r *http.Request) error {
	a, err := h.requireActor(r)
	if err != nil {
		return err
	}
	id, err := requireUUID(r.PathValue("id"), "")
	if err != nil {
		return err
	}
	f, err := readJSON(r, "")
	if err != nil {
		return err
	}
	var in reviewAction
	if s := f.Str("action", optional, validate.StrOpts{}); s != nil {
		in.Action = *s
	}
	in.Text = f.Str("text", nullish, validate.StrOpts{})
	in.ItemID = f.Str("item_id", optional, validate.StrOpts{})
	score, scoreSent := field(f, "score")
	in.Notes = f.Str("notes", nullish, validate.StrOpts{})
	project, _ := field(f, "project_score")
	if err := issuesErr(f, ""); err != nil {
		return err
	}
	in.Score = domain.JSNumber(score)
	if !scoreSent {
		in.Score = math.NaN() // Number(undefined)
	}
	if project != nil {
		n := domain.JSNumber(project)
		in.ProjectScore = &n
	}
	msg, err := h.svc.applyReviewAction(r.Context(), a, id, in)
	if err != nil {
		return err
	}
	return writeJSON(w, messageBody{msg})
}

package payroll

import (
	"fmt"
	"strings"
	"testing"
)

// Integration tests for KPI, department tasks, performance reviews and 360
// feedback. Same rolled-back transaction setup as payroll_integration_test.go.

func (e *env) indicatorID(code string) string {
	e.t.Helper()
	var id string
	e.scalar(&id, `SELECT id::text FROM performance.kpi_indicators WHERE code = $1`, code)
	return id
}

func TestKPITargetsAndScorecards(t *testing.T) {
	e := setup(t)
	dept := e.department("Gudang Go")
	head := e.emp(empOpts{name: "Kepala", user: e.staff.UserID, dept: dept})
	worker := e.emp(empOpts{name: "Pekerja", dept: dept, reportsTo: head})
	ontime := e.indicatorID("att_ontime")

	wantErr(t, e.call(&e.hr, "POST", "/api/hris/kpi/targets", map[string]any{"target": -1}, 400), targetRequired)
	wantErr(t, e.call(&e.hr, "POST", "/api/hris/kpi/targets", map[string]any{"indicator_id": ontime, "target": 1,
		"role_code": "pos", "employee_id": worker}, 400), "Pilih satu scope saja (role ATAU department ATAU karyawan)")
	created := e.call(&e.hr, "POST", "/api/hris/kpi/targets", map[string]any{"indicator_id": ontime, "target": "0.9",
		"period_year": 2026, "employee_id": worker}, 201)
	if created["message"] != "Target tersimpan" || created["data"].(map[string]any)["target"] != "0.9000" ||
		created["data"].(map[string]any)["created_by"] != e.hr.UserID {
		t.Fatalf("created %v", created)
	}
	list := e.call(&e.hr, "GET", "/api/hris/kpi/targets", nil, 200)["data"].([]any)
	first := list[0].(map[string]any)
	if first["indicator"].(map[string]any)["code"] != "att_ontime" || first["employee"].(map[string]any)["full_name"] != "Pekerja" ||
		first["department"] != nil {
		t.Fatalf("list %v", first)
	}
	wantErr(t, e.call(&e.hr, "DELETE", "/api/hris/kpi/targets?reason=Target%20uji%20dihapus", nil, 400), "id wajib")
	if e.call(&e.hr, "DELETE", "/api/hris/kpi/targets?reason=Target%20uji%20dihapus&id="+created["data"].(map[string]any)["id"].(string), nil, 200)["message"] != "Target dihapus" {
		t.Fatal("delete")
	}

	var cardID string
	e.scalar(&cardID, `INSERT INTO performance.kpi_scorecards (employee_id, period_year, period_month, role_code, score)
		VALUES ($1, 2026, 10, 'pos', 83.333) RETURNING id::text`, worker)
	e.exec(`INSERT INTO performance.kpi_scorecards (employee_id, period_year, period_month, role_code, score)
		VALUES ($1, 2026, 9, 'pos', 70), ($1, 2026, 8, 'pos', 90)`, worker)

	// The head sees the team; HR sees the period; the head alone sees their own (none).
	team := e.call(&e.staff, "GET", "/api/hris/kpi/scorecards?team=1", nil, 200)
	if rows := team["data"].([]any); len(rows) != 1 || team["period_month"] != 10.0 ||
		rows[0].(map[string]any)["employee"].(map[string]any)["department"].(map[string]any)["name"] != "Gudang Go" {
		t.Fatalf("team %v", team)
	}
	if own := e.call(&e.staff, "GET", "/api/hris/kpi/scorecards", nil, 200); len(own["data"].([]any)) != 0 || own["indicators"] == nil {
		t.Fatalf("own %v", own)
	}
	hist := e.call(&e.hr, "GET", "/api/hris/kpi/scorecards?employee_id="+worker+"&history=2", nil, 200)
	if rows := hist["data"].([]any); len(rows) != 2 || rows[0].(map[string]any)["period_month"] != 10.0 || hist["period_year"] != nil {
		t.Fatalf("history %v", hist)
	}
	wantErr(t, e.call(&e.hr, "GET", "/api/hris/kpi/scorecards?history=2", nil, 400), "history membutuhkan employee_id")
	if body := e.raw(&e.hr, "GET", "/api/hris/kpi/scorecards?employee_id=me&history=3", nil).Body.String(); !strings.HasPrefix(body, `{"data":[],"indicators":[`) {
		t.Fatalf("me without employee %s", body)
	}

	wantErr(t, e.call(&e.hr, "PATCH", "/api/hris/kpi/scorecards", map[string]any{"action": "x"}, 400), actionRequired)
	fin := e.call(&e.hr, "PATCH", "/api/hris/kpi/scorecards", map[string]any{"action": "finalize", "scorecard_id": cardID}, 200)
	if fin["message"] != "Scorecard difinalkan" || fin["data"].(map[string]any)["status"] != "final" ||
		!strings.Contains(fin["wa_link"].(string), "FINAL%3A%2083%2C33.") {
		t.Fatalf("finalize %v", fin)
	}
	wantErr(t, e.call(&e.hr, "PATCH", "/api/hris/kpi/scorecards", map[string]any{"action": "finalize", "scorecard_id": cardID}, 409), "Scorecard sudah final")
	reopen := e.call(&e.hr, "PATCH", "/api/hris/kpi/scorecards", map[string]any{"action": "reopen", "scorecard_id": cardID}, 200)
	if reopen["wa_link"] != nil || reopen["data"].(map[string]any)["reviewed_by"] != nil {
		t.Fatalf("reopen %v", reopen)
	}

	rec := e.call(&e.hr, "GET", "/api/hris/kpi/recommendation?employee_ids="+worker+",bad", nil, 200)["data"].(map[string]any)
	if r := rec[worker].(map[string]any); r["avg_score"] != 81.11 || r["periods"] != 3.0 || r["has_final"] != false {
		t.Fatalf("recommendation %v", rec)
	}
}

func TestKPIRubricAndConfig(t *testing.T) {
	e := setup(t)
	dept := e.department("Ops Go")
	head := e.emp(empOpts{name: "Atasan", user: e.staff.UserID, dept: dept})
	worker := e.emp(empOpts{name: "Bawahan", dept: dept, reportsTo: head})
	e.emps.roles[worker] = "pos"

	wantErr(t, e.call(&e.hr, "POST", "/api/hris/kpi/rubric", map[string]any{"employee_id": worker, "period_month": 10,
		"period_year": 2026, "value": 6}, 400), valueRange)
	wantErr(t, e.call(&e.staff, "POST", "/api/hris/kpi/rubric", map[string]any{"employee_id": head, "period_month": 10,
		"period_year": 2026, "value": 4}, 403), "Hanya HRD atau atasan langsung yang boleh menilai")
	res := e.call(&e.staff, "POST", "/api/hris/kpi/rubric", map[string]any{"employee_id": worker, "period_month": 10,
		"period_year": 2026, "value": 4, "notes": "ok"}, 200)
	data := res["data"].(map[string]any)
	if res["message"] != "Rubrik tersimpan & scorecard diperbarui" || data["status"] != "updated" {
		t.Fatalf("rubric %v", res)
	}
	var detail, breakdown string
	e.scalar(&detail, `SELECT source_detail::text FROM performance.kpi_snapshots WHERE employee_id = $1`, worker)
	e.scalar(&breakdown, `SELECT breakdown::text FROM performance.kpi_scorecards WHERE employee_id = $1`, worker)
	if !strings.Contains(detail, `"rated_by": "`+e.staff.UserID) || !strings.Contains(breakdown, `"effectiveWeight"`) {
		t.Fatalf("snapshot %s / %s", detail, breakdown)
	}

	// Configuration: HR only, at least one indicator on, weights 1–100.
	if rec := e.raw(&e.staff, "GET", "/api/hris/kpi-config", nil); rec.Code != 403 {
		t.Fatalf("staff config = %d", rec.Code)
	}
	cfg := e.call(&e.hr, "GET", "/api/hris/kpi-config", nil, 200)["data"].(map[string]any)
	if len(cfg["departments"].([]any)) != 1 || len(cfg["indicators"].([]any)) == 0 {
		t.Fatalf("config %v", cfg)
	}
	ind := e.indicatorID("att_ontime")
	wantErr(t, e.call(&e.hr, "PUT", "/api/hris/kpi-config", map[string]any{"department_id": dept}, 400),
		"Minimal satu indikator harus aktif untuk departemen ini")
	wantErr(t, e.call(&e.hr, "PUT", "/api/hris/kpi-config", map[string]any{"department_id": dept,
		"items": []any{map[string]any{"indicator_id": ind, "enabled": true, "weight": 0}}}, 400), weightRange)
	wantErr(t, e.call(&e.hr, "PUT", "/api/hris/kpi-config", map[string]any{"department_id": "x"}, 400), "Departemen tidak valid")
	ok := e.call(&e.hr, "PUT", "/api/hris/kpi-config", map[string]any{"department_id": dept,
		"items": []any{map[string]any{"indicator_id": ind, "enabled": true, "weight": "60"},
			map[string]any{"indicator_id": e.indicatorID("task_completion"), "enabled": false}}}, 200)
	if ok["message"] != "Konfigurasi KPI departemen Ops Go disimpan — berlaku mulai snapshot bulan berikutnya" {
		t.Fatalf("save %v", ok)
	}
	var weight, by string
	e.scalar(&weight, `SELECT weight::text FROM performance.kpi_department_indicators WHERE department_id = $1`, dept)
	e.scalar(&by, `SELECT updated_by FROM performance.kpi_department_indicators WHERE department_id = $1`, dept)
	if weight != "60.00" || by != e.hr.FullName {
		t.Fatalf("mapping %s %s", weight, by)
	}
}

func TestDeptTasks(t *testing.T) {
	e := setup(t)
	dept := e.department("Dapur Go")
	head := e.emp(empOpts{name: "Kepala Dapur", user: e.staff.UserID, dept: dept})
	cook := e.emp(empOpts{name: "Juru Masak", dept: dept, reportsTo: head})

	wantErr(t, e.call(&e.staff, "GET", "/api/hris/dept-tasks?month=2026-13", nil, 400), "Parameter month wajib (YYYY-MM)")
	wantErr(t, e.call(&e.staff, "POST", "/api/hris/dept-tasks", map[string]any{"title": "ab"}, 400), "Judul task minimal 3 karakter")
	other := e.department("Lain Go")
	wantErr(t, e.call(&e.hr, "POST", "/api/hris/dept-tasks", map[string]any{"title": "Cek stok", "recurrence": "daily",
		"assignee_employee_id": cook, "department_id": other}, 400), "Penanggung jawab harus karyawan aktif di departemen yang sama")
	created := e.call(&e.staff, "POST", "/api/hris/dept-tasks", map[string]any{"title": "Bersih dapur", "recurrence": "weekly",
		"weekly_day": "1", "assignee_employee_id": cook, "subtasks": []any{map[string]any{"title": "Sapu"}, map[string]any{"title": "Pel"}}}, 201)
	taskID := created["data"].(map[string]any)["id"].(string)

	board := e.call(&e.staff, "GET", "/api/hris/dept-tasks?month=2026-09", nil, 200)["data"].(map[string]any)
	tasks, occs := board["tasks"].([]any), board["occurrences"].([]any)
	if board["department_id"] != dept || len(occs) != 4 || board["can_review"] != true || board["departments"] == nil ||
		len(board["subtasks"].([]any)) != 2 || len(board["members"].([]any)) != 2 {
		t.Fatalf("board %v", board)
	}
	var found map[string]any
	for _, task := range tasks {
		if task.(map[string]any)["id"] == taskID {
			found = task.(map[string]any)
		}
	}
	if found["assignee_name"] != "Juru Masak" || found["created_by_name"] != "Kepala Dapur" {
		t.Fatalf("task %v", found)
	}

	var occID, subID string
	e.scalar(&occID, `SELECT id::text FROM hris.department_task_occurrences WHERE task_id = $1 ORDER BY occurrence_date LIMIT 1`, taskID)
	e.scalar(&subID, `SELECT id::text FROM hris.department_task_subtasks WHERE task_id = $1 ORDER BY sort_order LIMIT 1`, taskID)
	wantErr(t, e.call(&e.staff, "PATCH", "/api/hris/dept-tasks/occurrences/"+occID, map[string]any{"action": "done"}, 403),
		"Hanya penanggung jawab task ini yang boleh mengerjakannya")
	wantErr(t, e.call(&e.staff, "PATCH", "/api/hris/dept-tasks/occurrences/"+occID, map[string]any{"action": "approve"}, 400),
		"Hanya task berstatus selesai yang bisa direview")
	wantErr(t, e.call(&e.staff, "PATCH", "/api/hris/dept-tasks/occurrences/"+occID, map[string]any{"action": "nope"}, 400), "Aksi tidak dikenal")
	// HR checks a sub-task on behalf of the assignee: 50%.
	half := e.call(&e.hr, "PATCH", "/api/hris/dept-tasks/occurrences/"+occID, map[string]any{"action": "check_subtask",
		"subtask_id": subID, "checked": true}, 200)
	if half["message"] != "Progres 50%" || half["data"].(map[string]any)["complete"] != false {
		t.Fatalf("check %v", half)
	}
	e.exec(`UPDATE hris.department_task_occurrences SET status = 'done' WHERE id = $1`, occID)
	ok := e.call(&e.staff, "PATCH", "/api/hris/dept-tasks/occurrences/"+occID, map[string]any{"action": "approve", "notes": " rapi "}, 200)
	if ok["message"] != "Task disetujui" {
		t.Fatalf("approve %v", ok)
	}
	var reviewer, notes string
	e.scalar(&reviewer, `SELECT reviewed_by::text FROM hris.department_task_occurrences WHERE id = $1`, occID)
	e.scalar(&notes, `SELECT review_notes FROM hris.department_task_occurrences WHERE id = $1`, occID)
	if reviewer != head || notes != "rapi" {
		t.Fatalf("reviewer %s %q (must be the employee id)", reviewer, notes)
	}
	wantErr(t, e.call(&e.staff, "PATCH", "/api/hris/dept-tasks/bad", nil, 400), "ID tidak valid")
	if e.call(&e.staff, "PATCH", "/api/hris/dept-tasks/"+taskID, nil, 200)["message"] != "Task dinonaktifkan" {
		t.Fatal("deactivate")
	}
	wantErr(t, e.call(&e.staff, "PATCH", "/api/hris/dept-tasks/"+taskID, nil, 404), "Task tidak ditemukan")
}

func TestPerformanceReviews(t *testing.T) {
	e := setup(t)
	dept := e.department("Keuangan Go")
	head := e.emp(empOpts{name: "Kepala Keu", user: e.staff.UserID, dept: dept})
	worker := e.emp(empOpts{name: "Staf Keu", dept: dept, reportsTo: head})
	e.exec(`INSERT INTO performance.kpi_scorecards (employee_id, period_year, period_month, role_code, score)
		VALUES ($1, 2026, 7, 'pos', 80), ($1, 2026, 8, 'pos', 90)`, worker)

	wantErr(t, e.call(&e.staff, "POST", "/api/hris/performance/cycles", map[string]any{"period_year": 2026, "period_quarter": 3}, 403),
		"Hanya HRD yang boleh membuka siklus review")
	wantErr(t, e.call(&e.hr, "POST", "/api/hris/performance/cycles", map[string]any{"period_year": 2026, "period_quarter": 5}, 400), "Kuartal harus 1–4")
	opened := e.call(&e.hr, "POST", "/api/hris/performance/cycles", map[string]any{"period_year": "2026", "period_quarter": 3}, 201)
	if opened["message"] != "Siklus Q3 2026 dibuka — 2 draft review dibuat" {
		t.Fatalf("open %v", opened)
	}
	wantErr(t, e.call(&e.hr, "POST", "/api/hris/performance/cycles", map[string]any{"period_year": 2026, "period_quarter": 3}, 409), "Siklus Q3 2026 sudah ada")

	cycles := e.call(&e.staff, "GET", "/api/hris/performance/cycles", nil, 200)["data"].(map[string]any)
	cycle := cycles["cycles"].([]any)[0].(map[string]any)
	if cycles["is_hr"] != false || cycle["total_reviews"] != 2.0 || cycle["start_date"] != "2026-07-01" {
		t.Fatalf("cycles %v", cycles)
	}
	cycleID := cycle["id"].(string)
	wantErr(t, e.call(&e.staff, "GET", "/api/hris/performance/reviews", nil, 400), "Parameter cycle_id wajib")
	list := e.call(&e.staff, "GET", "/api/hris/performance/reviews?cycle_id="+cycleID, nil, 200)["data"].(map[string]any)
	reviews := list["reviews"].([]any)
	if list["can_review"] != true || len(reviews) != 2 || reviews[0].(map[string]any)["full_name"] != "Kepala Keu" ||
		reviews[1].(map[string]any)["department_name"] != "Keuangan Go" || reviews[1].(map[string]any)["total_work_result_score"] != "85.00" {
		t.Fatalf("reviews %v", list)
	}
	if _, leaked := reviews[0].(map[string]any)["employee_department_id"]; leaked {
		t.Fatal("helper column leaked")
	}
	reviewID := reviews[1].(map[string]any)["id"].(string)

	rt := e.call(&e.staff, "GET", "/api/hris/performance/realtime?year=2026&quarter=3", nil, 200)["data"].(map[string]any)
	emps := rt["employees"].([]any)
	if len(emps) != 2 || emps[0].(map[string]any)["avg_score"] != "85.00" || emps[1].(map[string]any)["months"] != nil {
		t.Fatalf("realtime %v", rt)
	}
	wantErr(t, e.call(&e.staff, "GET", "/api/hris/performance/realtime?year=1999", nil, 400), "Periode tidak valid")

	detail := e.call(&e.staff, "GET", "/api/hris/performance/reviews/"+reviewID, nil, 200)["data"].(map[string]any)
	if detail["can_rate"] != true || detail["review"].(map[string]any)["start_date"] != "2026-07-01" ||
		detail["review"].(map[string]any)["cycle_name"] != "Q3 2026" || len(detail["kpi_months"].([]any)) != 2 {
		t.Fatalf("detail %v", detail)
	}
	items := detail["items"].([]any)
	itemID := items[0].(map[string]any)["id"].(string)

	act := func(body map[string]any, status int) map[string]any {
		return e.call(&e.staff, "PATCH", "/api/hris/performance/reviews/"+reviewID, body, status)
	}
	wantErr(t, act(map[string]any{"action": "self_assessment", "text": "x"}, 403), "Hanya karyawan yang bersangkutan yang boleh mengisi self assessment")
	wantErr(t, act(map[string]any{"action": "rate_item", "item_id": itemID, "score": 6}, 400), "Skor harus 1–5")
	if act(map[string]any{"action": "rate_item", "item_id": itemID, "score": 5}, 200)["message"] != "Penilaian tersimpan" {
		t.Fatal("rate")
	}
	wantErr(t, act(map[string]any{"action": "reviewer_notes", "project_score": 101}, 400), "Nilai kontribusi harus 0–100")
	act(map[string]any{"action": "reviewer_notes", "text": " bagus ", "project_score": 90}, 200)
	var grand, category, reviewer string
	e.scalar(&grand, `SELECT grand_total_score::text FROM performance.performance_reviews WHERE id = $1`, reviewID)
	e.scalar(&category, `SELECT category FROM performance.performance_reviews WHERE id = $1`, reviewID)
	e.scalar(&reviewer, `SELECT reviewer_name FROM performance.performance_reviews WHERE id = $1`, reviewID)
	if grand != "90.00" || category != "Istimewa" || reviewer != "Kepala Keu" {
		t.Fatalf("totals %s %s %s", grand, category, reviewer)
	}
	wantErr(t, act(map[string]any{"action": "finalize"}, 403), "Hanya HRD yang boleh memfinalkan review")
	if e.call(&e.hr, "PATCH", "/api/hris/performance/reviews/"+reviewID, map[string]any{"action": "finalize"}, 200)["message"] != "Review difinalkan — nilai terkunci" {
		t.Fatal("finalize")
	}
	wantErr(t, act(map[string]any{"action": "rate_item", "item_id": itemID, "score": 1}, 400), "Review sudah final — tidak bisa diubah")
	if act(map[string]any{"action": "sign"}, 200)["message"] != "Ditandatangani sebagai reviewer" {
		t.Fatal("sign")
	}
	wantErr(t, act(map[string]any{"action": "dance"}, 400), "Review sudah final — tidak bisa diubah")
}

func TestFeedback360(t *testing.T) {
	e := setup(t)
	approverEmail := e.hr.Email
	approver := e.emp(empOpts{name: "Penyetuju", user: e.hr.UserID, email: approverEmail})
	staff := e.emp(empOpts{name: "Dinilai"})
	peer := e.emp(empOpts{name: "Rekan"})

	if rec := e.raw(&e.staff, "GET", "/api/hris/feedback-categories", nil); rec.Code != 403 {
		t.Fatalf("guard = %d", rec.Code)
	}
	wantErr(t, e.call(&e.hr, "POST", "/api/hris/feedback-categories", map[string]any{"name": "  "}, 400), "Data tidak valid")
	cat := e.call(&e.hr, "POST", "/api/hris/feedback-categories", map[string]any{"name": " Kerja Sama ", "weight": 25, "x": 1}, 201)["data"].(map[string]any)
	if cat["name"] != "Kerja Sama" || cat["weight"] != "25.00" {
		t.Fatalf("category %v", cat)
	}
	var criteria string
	e.scalar(&criteria, `INSERT INTO performance.feedback_criteria (category_id, name) VALUES ($1, 'Membantu') RETURNING id::text`, cat["id"])
	cats := e.call(&e.hr, "GET", "/api/hris/feedback-categories", nil, 200)["data"].([]any)
	if len(cats) != 1 || len(cats[0].(map[string]any)["criteria"].([]any)) != 1 {
		t.Fatalf("categories %v", cats)
	}

	cycle := e.call(&e.hr, "POST", "/api/hris/feedback-cycles", map[string]any{"name": "Siklus 1", "period_label": "Q4",
		"start_date": "2026-10-01", "end_date": "2026-12-31", "kpi_weight": 60, "feedback_weight": 40}, 201)["data"].(map[string]any)
	if cycle["created_by"] != approver {
		t.Fatalf("cycle creator must be the employee: %v", cycle)
	}
	cycleID := cycle["id"].(string)

	wantErr(t, e.call(&e.hr, "POST", "/api/hris/feedback-assignments", []any{}, 400), "Data tidak valid")
	assigned := e.call(&e.hr, "POST", "/api/hris/feedback-assignments", []any{
		map[string]any{"cycle_id": cycleID, "employee_id": staff, "reviewer_id": staff, "relationship_type": "self"},
		map[string]any{"cycle_id": cycleID, "employee_id": staff, "reviewer_id": peer, "relationship_type": "peer", "due_date": "2026-11-01"},
	}, 201)["data"].([]any)
	if len(assigned) != 2 || assigned[0].(map[string]any)["due_date"] != nil || assigned[1].(map[string]any)["due_date"] != "2026-11-01T00:00:00.000Z" {
		t.Fatalf("assigned %v", assigned)
	}
	selfID := assigned[0].(map[string]any)["id"].(string)
	page := e.call(&e.hr, "GET", "/api/hris/feedback-assignments?cycle_id="+cycleID+"&limit=1", nil, 200)
	if page["pagination"].(map[string]any)["totalPages"] != 2.0 || page["data"].([]any)[0].(map[string]any)["reviewer"] == nil {
		t.Fatalf("assignments page %v", page)
	}
	got := e.call(&e.hr, "GET", "/api/hris/feedback-assignments/"+selfID, nil, 200)["data"].(map[string]any)
	if got["employee"].(map[string]any)["full_name"] != "Dinilai" || got["cycle"].(map[string]any)["is_anonymous"] != true {
		t.Fatalf("assignment %v", got)
	}
	wantErr(t, e.call(&e.hr, "GET", "/api/hris/feedback-assignments/00000000-0000-0000-0000-000000000000", nil, 404), "Assignment not found")

	// One answer covering every criterion submits the assignment.
	resp := e.call(&e.hr, "POST", "/api/hris/feedback-responses", map[string]any{"assignment_id": selfID, "criteria_id": criteria,
		"rating": 4, "comments": "baik"}, 201)["data"].([]any)[0].(map[string]any)
	var status string
	e.scalar(&status, `SELECT status FROM performance.feedback_assignments WHERE id = $1`, selfID)
	if status != "submitted" || resp["rating"] != 4.0 {
		t.Fatalf("auto submit %s %v", status, resp)
	}
	responses := e.call(&e.hr, "GET", "/api/hris/feedback-responses?assignment_id="+selfID, nil, 200)["data"].([]any)
	if a := responses[0].(map[string]any)["assignment"].(map[string]any); a["employee"].(map[string]any)["full_name"] != "Dinilai" {
		t.Fatalf("responses %v", responses)
	}
	updated := e.call(&e.hr, "PUT", "/api/hris/feedback-responses/"+resp["id"].(string), map[string]any{"rating": 5}, 200)["data"].(map[string]any)
	if updated["rating"] != 5.0 {
		t.Fatalf("update response %v", updated)
	}

	approvals := e.call(&e.hr, "GET", "/api/hris/feedback-approvals", nil, 200)
	if stats := approvals["stats"].(map[string]any); stats["pending"] != 1.0 || stats["total"] != 1.0 {
		t.Fatalf("approvals %v", approvals)
	}
	if row := approvals["data"].([]any)[0].(map[string]any); row["employee"].(map[string]any)["full_name"] != "Dinilai" ||
		row["reviewer"].(map[string]any)["full_name"] != "Dinilai" {
		t.Fatalf("approval embeds the assessed employee and reviewer: %v", row)
	}
	wantErr(t, e.call(&e.hr, "POST", "/api/hris/feedback-approvals/approve", map[string]any{"assignment_id": "x"}, 400), "assignment_id is required")
	ok := e.call(&e.hr, "POST", "/api/hris/feedback-approvals/approve", map[string]any{"assignment_id": selfID, "manager_comments": ""}, 200)
	if ok["success"] != true || ok["data"].(map[string]any)["approved_by"] != approver || ok["data"].(map[string]any)["manager_comments"] != nil {
		t.Fatalf("approve %v", ok)
	}
	// The embeds follow employee_id and reviewer_id, not approved_by.
	detail := e.call(&e.hr, "GET", "/api/hris/feedback-responses/"+resp["id"].(string), nil, 200)["data"].(map[string]any)
	if a := detail["assignment"].(map[string]any); a["employee"].(map[string]any)["full_name"] != "Dinilai" ||
		a["reviewer"].(map[string]any)["full_name"] != "Dinilai" {
		t.Fatalf("response detail %v", detail)
	}
	wantErr(t, e.call(&e.hr, "POST", "/api/hris/feedback-approvals/reject", map[string]any{"assignment_id": selfID, "rejection_reason": " "}, 400),
		"Rejection reason is required")
	bulk := e.call(&e.hr, "POST", "/api/hris/feedback-approvals", map[string]any{"assignment_ids": []any{selfID}, "action": "reject",
		"rejection_reason": "ulang"}, 200)
	if bulk["message"] != "Successfully rejectd 1 submission(s)" {
		t.Fatalf("bulk %v", bulk)
	}

	// A trigger already summarised the reviewed employee; summarise the peer.
	summary := e.call(&e.hr, "POST", "/api/hris/feedback-summaries", map[string]any{"cycle_id": cycleID, "employee_id": peer,
		"kpi_score": 80, "overall_360_score": 100, "strengths": []any{"rapi"}}, 201)["data"].(map[string]any)
	if summary["final_score"] != "88.00" || summary["final_grade"] != "B" || summary["strengths"].([]any)[0] != "rapi" {
		t.Fatalf("summary %v", summary)
	}
	var summaries int
	e.scalar(&summaries, `SELECT count(*)::int FROM performance.feedback_summaries WHERE cycle_id = $1`, cycleID)
	list := e.call(&e.hr, "GET", "/api/hris/feedback-summaries?cycle_id="+cycleID+"&employee_id="+peer, nil, 200)
	if rows := list["data"].([]any); list["pagination"].(map[string]any)["total"] != 1.0 || len(rows) != 1 ||
		rows[0].(map[string]any)["employee"].(map[string]any)["full_name"] != "Rekan" ||
		rows[0].(map[string]any)["cycle"].(map[string]any)["name"] != "Siklus 1" ||
		len(rows[0].(map[string]any)["development_plans"].([]any)) != 0 {
		t.Fatalf("summary list %v", list)
	}
	cycles := e.call(&e.hr, "GET", "/api/hris/feedback-cycles?status=draft", nil, 200)
	var listed map[string]any
	for _, c := range cycles["data"].([]any) {
		if c.(map[string]any)["id"] == cycleID {
			listed = c.(map[string]any)
		}
	}
	if listed == nil || listed["created_by"].(map[string]any)["full_name"] != "Penyetuju" ||
		fmt.Sprint(listed["assignments_count"]) != "[map[count:2]]" ||
		fmt.Sprint(listed["summaries_count"]) != fmt.Sprintf("[map[count:%d]]", summaries) {
		t.Fatalf("cycle list %v", cycles)
	}
	full := e.call(&e.hr, "GET", "/api/hris/feedback-cycles/"+cycleID, nil, 200)["data"].(map[string]any)
	if full["created_by"].(map[string]any)["full_name"] != "Penyetuju" || len(full["assignments"].([]any)) != 2 ||
		full["summaries"].([]any)[0].(map[string]any)["employee"].(map[string]any)["department"] != nil {
		t.Fatalf("cycle detail %v", full)
	}
	upd := e.call(&e.hr, "PUT", "/api/hris/feedback-cycles/"+cycleID, map[string]any{"status": "active"}, 200)["data"].(map[string]any)
	if upd["status"] != "active" || upd["name"] != "Siklus 1" {
		t.Fatalf("cycle update %v", upd)
	}
	wantErr(t, e.call(&e.hr, "PUT", "/api/hris/feedback-assignments/"+selfID, map[string]any{"status": "approved"}, 400), "Data tidak valid")
	sub := e.call(&e.hr, "PUT", "/api/hris/feedback-assignments/"+selfID, map[string]any{"status": "submitted", "submitted_at": nil}, 200)["data"].(map[string]any)
	if sub["submitted_at"] != "2026-10-04T10:00:00.000Z" {
		t.Fatalf("submitted_at %v", sub)
	}
	if e.call(&e.hr, "DELETE", "/api/hris/feedback-responses/"+resp["id"].(string)+"?reason=Feedback%20uji%20dihapus", nil, 200)["success"] != true {
		t.Fatal("delete response")
	}
	if e.call(&e.hr, "DELETE", "/api/hris/feedback-assignments/"+selfID+"?reason=Feedback%20uji%20dihapus", nil, 200)["message"] != "Assignment deleted successfully" {
		t.Fatal("delete assignment")
	}
	if e.call(&e.hr, "DELETE", "/api/hris/feedback-cycles/"+cycleID+"?reason=Feedback%20uji%20dihapus", nil, 200)["message"] != "Cycle deleted successfully" {
		t.Fatal("delete cycle")
	}
}

// Actor gates from hris-actor-gates.test.ts: writes that reference
// hris.employees resolve the caller's employee record, never the account id.
func TestActorGates(t *testing.T) {
	e := setup(t)
	dept := e.department("Gate Go")
	var taskID, occID string
	e.scalar(&taskID, `INSERT INTO hris.department_tasks (department_id, title, recurrence, due_date, created_by_name)
		VALUES ($1, 'Cek', 'once', '2026-10-10', 'x') RETURNING id::text`, dept)
	e.scalar(&occID, `INSERT INTO hris.department_task_occurrences (task_id, occurrence_date, status)
		VALUES ($1, '2026-10-10', 'done') RETURNING id::text`, taskID)
	// HR without an employee record cannot review.
	wantErr(t, e.call(&e.hr, "PATCH", "/api/hris/dept-tasks/occurrences/"+occID, map[string]any{"action": "approve"}, 403),
		"Akun ini tidak terhubung ke data karyawan, tidak bisa mereview task")
	// Nor create a feedback cycle (no employee by user_id or email).
	wantErr(t, e.call(&e.hr, "POST", "/api/hris/feedback-cycles", map[string]any{"name": "Q3"}, 403),
		"Akun ini tidak terhubung ke data karyawan, tidak bisa membuat siklus feedback")
	hrEmp := e.emp(empOpts{name: "HR Rina", user: e.hr.UserID})
	if e.call(&e.hr, "PATCH", "/api/hris/dept-tasks/occurrences/"+occID, map[string]any{"action": "approve"}, 200)["message"] != "Task disetujui" {
		t.Fatal("approve")
	}
	var reviewer string
	e.scalar(&reviewer, `SELECT reviewed_by::text FROM hris.department_task_occurrences WHERE id = $1`, occID)
	if reviewer != hrEmp {
		t.Fatalf("reviewed_by = %s, want the employee id", reviewer)
	}
}

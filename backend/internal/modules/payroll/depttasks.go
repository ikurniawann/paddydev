package payroll

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/modules/payroll/domain"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/validate"
)

// Department tasks (lib/hris/dept-tasks-repo.ts). hris.department_task*
// belong to this module; employees and departments come through ports.

// taskActor is TaskActor: the workforce actor with its department, name
// and whether it heads anyone (has active direct reports).
type taskActor struct {
	*actor
	DepartmentID    *string
	FullName        *string
	HasSubordinates bool
}

func (s *service) loadTaskActor(ctx context.Context, a *actor) (*taskActor, error) {
	me := &taskActor{actor: a}
	if a.EmployeeID == nil {
		return me, nil
	}
	b, err := s.brief(ctx, *a.EmployeeID)
	if err != nil || b == nil {
		return me, err
	}
	me.DepartmentID, me.FullName = b.DepartmentID, &b.FullName
	reports, err := s.ports.Employees.DirectReports(ctx, s.db, *a.EmployeeID)
	me.HasSubordinates = len(reports) > 0
	return me, err
}

// targetDepartment: HR may pick a department; everyone else gets their own.
func targetDepartment(me *taskActor, requested *string) *string {
	if me.PerformanceHR && requested != nil && uuidRE.MatchString(*requested) {
		return requested
	}
	return me.DepartmentID
}

// isDepartmentHead: the employee is in the department (and active when
// required) and has at least one active direct report.
func (s *service) isDepartmentHead(ctx context.Context, employeeID, departmentID string, requireActive bool) (bool, error) {
	b, err := s.brief(ctx, employeeID)
	if err != nil || b == nil || (requireActive && !b.IsActive) || b.DepartmentID == nil || *b.DepartmentID != departmentID {
		return false, err
	}
	reports, err := s.ports.Employees.DirectReports(ctx, s.db, employeeID)
	return len(reports) > 0, err
}

// ensureOccurrences generates the missing occurrences of the department's
// active tasks in [start, end] (idempotent).
func (s *service) ensureOccurrences(ctx context.Context, departmentID, start, end string) error {
	rows, err := s.db.Query(ctx, `SELECT id::text, recurrence, weekly_day, monthly_day, due_date::text
		FROM hris.department_tasks WHERE department_id = $1 AND is_active = true`, departmentID)
	if err != nil {
		return err
	}
	type task struct {
		id string
		domain.TaskSchedule
	}
	tasks, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (task, error) {
		var t task
		return t, r.Scan(&t.id, &t.Recurrence, &t.WeeklyDay, &t.MonthlyDay, &t.DueDate)
	})
	if err != nil {
		return err
	}
	var ids, dates []string
	for _, t := range tasks {
		for _, d := range domain.OccurrenceDates(t.TaskSchedule, start, end) {
			ids, dates = append(ids, t.id), append(dates, d)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	_, err = s.db.Exec(ctx, `INSERT INTO hris.department_task_occurrences (task_id, occurrence_date)
		SELECT * FROM unnest($1::uuid[], $2::date[])
		ON CONFLICT (task_id, occurrence_date) DO NOTHING`, ids, dates)
	return err
}

func departmentRows(depts []Department) []*obj {
	out := make([]*obj, len(depts))
	for i, d := range depts {
		out[i] = object("id", d.ID, "name", d.Name)
	}
	return out
}

func (s *service) deptTaskBoard(ctx context.Context, me *taskActor, month string, requested *string) (*obj, error) {
	departmentID := targetDepartment(me, requested)
	departments := []*obj{}
	if me.PerformanceHR {
		all, err := s.ports.Departments.All(ctx, s.db)
		if err != nil {
			return nil, err
		}
		departments = departmentRows(all)
	}
	if departmentID == nil {
		return object("department_id", nil, "tasks", []any{}, "occurrences", []any{}, "members", []any{},
			"departments", departments, "can_manage", me.PerformanceHR, "is_hr", me.PerformanceHR), nil
	}
	dept := *departmentID
	start, end := domain.MonthRange(month)
	if err := s.ensureOccurrences(ctx, dept, start, end); err != nil {
		return nil, err
	}
	tasks, err := queryObjs(ctx, s.db, `SELECT t.id, t.title, t.description, t.recurrence, t.weekly_day,
		       t.monthly_day, t.due_date::text, t.is_active, t.assignee_employee_id, NULL AS assignee_name,
		       t.created_by_name
		  FROM hris.department_tasks t
		 WHERE t.department_id = $1 AND t.is_active = true
		 ORDER BY t.created_at`, dept)
	if err != nil {
		return nil, err
	}
	if err := s.fillNames(ctx, tasks, "assignee_name", "assignee_employee_id"); err != nil {
		return nil, err
	}
	occurrences, err := queryObjs(ctx, s.db, `SELECT o.id, o.task_id, o.occurrence_date::text, o.status,
		       o.done_at, o.done_by_name, o.review_notes, o.reviewed_by_name
		  FROM hris.department_task_occurrences o
		  JOIN hris.department_tasks t ON t.id = o.task_id
		 WHERE t.department_id = $1 AND o.occurrence_date BETWEEN $2 AND $3
		 ORDER BY o.occurrence_date, t.created_at`, dept, start, end)
	if err != nil {
		return nil, err
	}
	members, err := s.ports.Employees.DepartmentMembers(ctx, s.db, dept)
	if err != nil {
		return nil, err
	}
	memberRows := make([]*obj, len(members))
	for i, m := range members {
		memberRows[i] = object("id", m.ID, "full_name", m.FullName)
	}
	subtasks, err := queryObjs(ctx, s.db, `SELECT st.id, st.task_id, st.title, st.weight, st.sort_order
		  FROM hris.department_task_subtasks st
		  JOIN hris.department_tasks t ON t.id = st.task_id
		 WHERE t.department_id = $1
		 ORDER BY st.task_id, st.sort_order, st.created_at`, dept)
	if err != nil {
		return nil, err
	}
	checked, err := queryObjs(ctx, s.db, `SELECT oi.occurrence_id, oi.subtask_id, oi.is_checked,
		       oi.checked_by_name, oi.checked_at
		  FROM hris.department_task_occurrence_items oi
		  JOIN hris.department_task_occurrences o ON o.id = oi.occurrence_id
		  JOIN hris.department_tasks t ON t.id = o.task_id
		 WHERE t.department_id = $1 AND o.occurrence_date BETWEEN $2 AND $3`, dept, start, end)
	if err != nil {
		return nil, err
	}
	return object("department_id", dept, "tasks", tasks, "occurrences", occurrences, "members", memberRows,
		"subtasks", subtasks, "checked_items", checked, "departments", departments,
		"can_manage", me.PerformanceHR || me.HasSubordinates,
		"can_review", me.PerformanceHR || (me.HasSubordinates && me.DepartmentID != nil && dept == *me.DepartmentID),
		"is_hr", me.PerformanceHR, "my_employee_id", me.EmployeeID), nil
}

// fillNames sets alias to the full name of the employee in column (a LEFT
// JOIN on hris.employees in the TS).
func (s *service) fillNames(ctx context.Context, rows []*obj, alias, column string) error {
	var ids []string
	for _, r := range rows {
		if id := r.Str(column); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		for _, r := range rows {
			r.Set(alias, nil)
		}
		return nil
	}
	briefs, err := s.ports.Employees.Briefs(ctx, s.db, ids)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if b, ok := briefs[r.Str(column)]; ok {
			r.Set(alias, b.FullName)
		} else {
			r.Set(alias, nil)
		}
	}
	return nil
}

func (s *service) createDeptTask(ctx context.Context, me *taskActor, deptParam, assigneeParam *string, task *domain.NormalizedTask) (string, error) {
	departmentID := targetDepartment(me, deptParam)
	if departmentID == nil {
		return "", httpx.BadRequest("Departemen tidak diketahui")
	}
	if !me.PerformanceHR {
		if !me.HasSubordinates {
			return "", httpx.Forbidden("Hanya HRD atau atasan (kepala tim) yang boleh membuat task departemen")
		}
		if me.DepartmentID == nil || *departmentID != *me.DepartmentID {
			return "", httpx.Forbidden("Hanya boleh membuat task untuk departemen sendiri")
		}
	}
	var assignee *string
	if assigneeParam != nil && uuidRE.MatchString(*assigneeParam) {
		b, err := s.brief(ctx, *assigneeParam)
		if err != nil {
			return "", err
		}
		if b == nil || !b.IsActive || b.DepartmentID == nil || *b.DepartmentID != *departmentID {
			return "", httpx.BadRequest("Penanggung jawab harus karyawan aktif di departemen yang sama")
		}
		assignee = assigneeParam
	}
	createdBy := "—"
	if me.FullName != nil {
		createdBy = *me.FullName
	}
	var id string
	err := s.db.QueryRow(ctx, `INSERT INTO hris.department_tasks
		   (department_id, assignee_employee_id, title, description, recurrence,
		    weekly_day, monthly_day, due_date, created_by, created_by_name)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id::text`,
		*departmentID, assignee, task.Title, task.Description, task.Schedule.Recurrence,
		task.Schedule.WeeklyDay, task.Schedule.MonthlyDay, task.Schedule.DueDate, me.EmployeeID, createdBy).Scan(&id)
	if err != nil {
		return "", err
	}
	for i, st := range task.Subtasks {
		if _, err := s.db.Exec(ctx, `INSERT INTO hris.department_task_subtasks (task_id, title, weight, sort_order)
			VALUES ($1, $2, $3, $4)`, id, st.Title, numParam(st.Weight), i); err != nil {
			return "", err
		}
	}
	return id, nil
}

func (s *service) deactivateDeptTask(ctx context.Context, a *actor, id string) error {
	var deptID string
	err := s.db.QueryRow(ctx, `SELECT department_id::text FROM hris.department_tasks WHERE id = $1 AND is_active`, id).Scan(&deptID)
	if database.IsNoRows(err) {
		return httpx.NotFound("Task tidak ditemukan")
	}
	if err != nil {
		return err
	}
	if !a.PerformanceHR {
		if a.EmployeeID == nil {
			return httpx.Forbidden("Akun tidak terhubung ke karyawan")
		}
		head, err := s.isDepartmentHead(ctx, *a.EmployeeID, deptID, false)
		if err != nil {
			return err
		}
		if !head {
			return httpx.Forbidden("Hanya HRD atau atasan departemen ini yang boleh menonaktifkan task")
		}
	}
	_, err = s.db.Exec(ctx, `UPDATE hris.department_tasks SET is_active = false, updated_at = now() WHERE id = $1`, id)
	return err
}

type occurrence struct {
	ID, Status, TaskID, DepartmentID string
	Assignee                         *string
}

// assertWorker: the assignee, or any active department member when the task
// has none; HR as an administrative fallback. Approved is final.
func (s *service) assertWorker(ctx context.Context, a *actor, occ occurrence) error {
	if occ.Status == "approved" {
		return httpx.BadRequest("Sudah disetujui — tidak bisa diubah")
	}
	if a.PerformanceHR {
		return nil
	}
	if a.EmployeeID == nil {
		return httpx.Forbidden("Akun ini tidak terhubung ke data karyawan")
	}
	if occ.Assignee != nil {
		if *occ.Assignee != *a.EmployeeID {
			return httpx.Forbidden("Hanya penanggung jawab task ini yang boleh mengerjakannya")
		}
		return nil
	}
	b, err := s.brief(ctx, *a.EmployeeID)
	if err != nil {
		return err
	}
	if b == nil || !b.IsActive || b.DepartmentID == nil || *b.DepartmentID != occ.DepartmentID {
		return httpx.Forbidden("Hanya anggota departemen ini yang boleh mengerjakannya")
	}
	return nil
}

type occurrenceAction struct {
	Action    string
	Notes     *string
	SubtaskID *string
	Checked   *bool
}

func (s *service) occurrenceAction(ctx context.Context, a *actor, id string, in occurrenceAction) (any, error) {
	var occ occurrence
	err := s.db.QueryRow(ctx, `SELECT o.id::text, o.status, o.task_id::text, t.department_id::text, t.assignee_employee_id::text
		  FROM hris.department_task_occurrences o JOIN hris.department_tasks t ON t.id = o.task_id
		 WHERE o.id = $1`, id).Scan(&occ.ID, &occ.Status, &occ.TaskID, &occ.DepartmentID, &occ.Assignee)
	if database.IsNoRows(err) {
		return nil, httpx.NotFound("Kemunculan task tidak ditemukan")
	}
	if err != nil {
		return nil, err
	}
	myName := "HRD"
	if a.EmployeeID != nil {
		myName = "—"
		if b, err := s.brief(ctx, *a.EmployeeID); err != nil {
			return nil, err
		} else if b != nil {
			myName = b.FullName
		}
	}

	switch in.Action {
	case "check_subtask":
		if err := s.assertWorker(ctx, a, occ); err != nil {
			return nil, err
		}
		return s.checkSubtask(ctx, a, occ, in, myName)
	case "done":
		if err := s.assertWorker(ctx, a, occ); err != nil {
			return nil, err
		}
		if _, err := s.db.Exec(ctx, `UPDATE hris.department_task_occurrences
			SET status = 'done', done_at = now(), done_by = $2, done_by_name = $3,
			    review_notes = NULL, reviewed_by = NULL, reviewed_by_name = NULL,
			    reviewed_at = NULL, updated_at = now()
			WHERE id = $1`, id, a.EmployeeID, myName); err != nil {
			return nil, err
		}
		return messageBody{"Task ditandai selesai — menunggu review Head Division"}, nil
	}

	// reviewed_by holds the reviewer's employee id (like done_by/created_by).
	if a.EmployeeID == nil {
		return nil, httpx.Forbidden("Akun ini tidak terhubung ke data karyawan, tidak bisa mereview task")
	}
	reviewer := *a.EmployeeID
	if !a.PerformanceHR {
		head, err := s.isDepartmentHead(ctx, reviewer, occ.DepartmentID, true)
		if err != nil {
			return nil, err
		}
		if !head {
			return nil, httpx.Forbidden("Hanya Head Division departemen ini (atau HRD) yang boleh mereview task")
		}
	}
	if occ.Status != "done" && !(in.Action == "reject" && occ.Status == "approved") {
		return nil, httpx.BadRequest("Hanya task berstatus selesai yang bisa direview")
	}
	status, msg := "rejected", "Task ditolak — penanggung jawab bisa memperbaiki"
	if in.Action == "approve" {
		status, msg = "approved", "Task disetujui"
	}
	if _, err := s.db.Exec(ctx, `UPDATE hris.department_task_occurrences
		SET status = $2, review_notes = $3, reviewed_by = $4, reviewed_by_name = $5,
		    reviewed_at = now(), updated_at = now()
		WHERE id = $1`, id, status, trimOrNil(in.Notes), reviewer, myName); err != nil {
		return nil, err
	}
	return messageBody{msg}, nil
}

type progressData struct {
	Progress float64 `json:"progress"`
	Complete bool    `json:"complete"`
}

type subtaskResult struct {
	Message string       `json:"message"`
	Data    progressData `json:"data"`
}

func (s *service) checkSubtask(ctx context.Context, a *actor, occ occurrence, in occurrenceAction, myName string) (any, error) {
	subtaskID := ""
	if in.SubtaskID != nil {
		subtaskID = *in.SubtaskID
	}
	if !uuidRE.MatchString(subtaskID) {
		return nil, httpx.BadRequest("ID sub-task tidak valid")
	}
	var found string
	err := s.db.QueryRow(ctx, `SELECT id::text FROM hris.department_task_subtasks WHERE id = $1 AND task_id = $2`, subtaskID, occ.TaskID).Scan(&found)
	if database.IsNoRows(err) {
		return nil, httpx.BadRequest("Sub-task bukan milik task ini")
	}
	if err != nil {
		return nil, err
	}
	checked := in.Checked != nil && *in.Checked
	if _, err := s.db.Exec(ctx, `INSERT INTO hris.department_task_occurrence_items
		   (occurrence_id, subtask_id, is_checked, checked_at, checked_by_name, updated_at)
		VALUES ($1, $2, $3, CASE WHEN $3 THEN now() END, CASE WHEN $3 THEN $4 END, now())
		ON CONFLICT (occurrence_id, subtask_id) DO UPDATE SET
		  is_checked = EXCLUDED.is_checked, checked_at = EXCLUDED.checked_at,
		  checked_by_name = EXCLUDED.checked_by_name, updated_at = now()`, occ.ID, subtaskID, checked, myName); err != nil {
		return nil, err
	}
	var total, done string
	if err := s.db.QueryRow(ctx, `SELECT COALESCE(SUM(st.weight), 0)::text, COALESCE(SUM(st.weight) FILTER (WHERE oi.is_checked), 0)::text
		  FROM hris.department_task_subtasks st
		  LEFT JOIN hris.department_task_occurrence_items oi ON oi.subtask_id = st.id AND oi.occurrence_id = $1
		 WHERE st.task_id = $2`, occ.ID, occ.TaskID).Scan(&total, &done); err != nil {
		return nil, err
	}
	progress, sum := domain.JSNumber(done), domain.JSNumber(total)
	complete := sum > 0 && progress >= sum-0.01
	if _, err := s.db.Exec(ctx, `UPDATE hris.department_task_occurrences
		SET status = CASE WHEN $2::boolean THEN 'done' ELSE 'pending' END,
		    done_at = CASE WHEN $2::boolean THEN now() END,
		    done_by = CASE WHEN $2::boolean THEN $3::uuid END,
		    done_by_name = CASE WHEN $2::boolean THEN $4 END,
		    updated_at = now()
		WHERE id = $1`, occ.ID, complete, a.EmployeeID, myName); err != nil {
		return nil, err
	}
	msg := fmt.Sprintf("Progres %v%%", domain.Round(progress))
	if complete {
		msg = "Semua sub-task selesai (100%) — menunggu review Head Division"
	}
	return subtaskResult{msg, progressData{progress, complete}}, nil
}

var monthRE = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// GET /api/hris/dept-tasks?month=YYYY-MM[&department_id=]
func (h *handler) deptTaskBoard(w http.ResponseWriter, r *http.Request) error {
	a, err := h.requireActor(r)
	if err != nil {
		return err
	}
	me, err := h.svc.loadTaskActor(r.Context(), a)
	if err != nil {
		return err
	}
	month := r.URL.Query().Get("month")
	if !monthRE.MatchString(month) {
		return httpx.BadRequest("Parameter month wajib (YYYY-MM)")
	}
	var dept *string
	if r.URL.Query().Has("department_id") {
		d := r.URL.Query().Get("department_id")
		dept = &d
	}
	data, err := h.svc.deptTaskBoard(r.Context(), me, month, dept)
	if err != nil {
		return err
	}
	return writeJSON(w, dataBody{data})
}

// dayField is z.union([z.number(), z.string()]).nullable().optional().
func dayField(f *validate.Form, key string) any {
	v, ok := field(f, key)
	if !ok || v == nil {
		return nil
	}
	switch x := v.(type) {
	case json.Number:
		n, _ := x.Float64()
		return n
	case string:
		return x
	}
	f.Fail(key, "invalid_union", "Invalid input")
	return nil
}

// POST /api/hris/dept-tasks
func (h *handler) createDeptTask(w http.ResponseWriter, r *http.Request) error {
	a, err := h.requireActor(r)
	if err != nil {
		return err
	}
	me, err := h.svc.loadTaskActor(r.Context(), a)
	if err != nil {
		return err
	}
	f, err := readJSON(r, "")
	if err != nil {
		return err
	}
	var b domain.TaskBody
	dept := f.Str("department_id", nullish, validate.StrOpts{})
	assignee := f.Str("assignee_employee_id", nullish, validate.StrOpts{})
	b.Title = f.Str("title", optional, validate.StrOpts{Max: 255})
	b.Description = f.Str("description", nullish, validate.StrOpts{Max: 5000})
	b.Recurrence = f.Str("recurrence", optional, validate.StrOpts{Max: 20})
	b.WeeklyDay = dayField(f, "weekly_day")
	b.MonthlyDay = dayField(f, "monthly_day")
	b.DueDate = f.Str("due_date", nullish, validate.StrOpts{Max: 10})
	f.List("subtasks", optional, math.MaxInt, func(sub *validate.Form, i int, v any) {
		item := sub.Item(i, v)
		b.Subtasks = append(b.Subtasks, item.Str("title", optional, validate.StrOpts{Max: 255}))
	})
	if err := issuesErr(f, ""); err != nil {
		return err
	}
	task, msg := domain.NormalizeDeptTask(b)
	if msg != "" {
		return httpx.BadRequest(msg)
	}
	id, err := h.svc.createDeptTask(r.Context(), me, dept, assignee, task)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, struct {
		Message string `json:"message"`
		Data    any    `json:"data"`
	}{"Task dibuat", object("id", id)})
}

// PATCH /api/hris/dept-tasks/{id}
func (h *handler) deactivateDeptTask(w http.ResponseWriter, r *http.Request) error {
	a, err := h.requireActor(r)
	if err != nil {
		return err
	}
	id, err := requireUUID(r.PathValue("id"), "")
	if err != nil {
		return err
	}
	if err := h.svc.deactivateDeptTask(r.Context(), a, id); err != nil {
		return err
	}
	return writeJSON(w, messageBody{"Task dinonaktifkan"})
}

// PATCH /api/hris/dept-tasks/occurrences/{id} { action, notes?, subtask_id?, checked? }
func (h *handler) occurrenceAction(w http.ResponseWriter, r *http.Request) error {
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
	var in occurrenceAction
	before := len(f.Issues())
	action := f.Enum("action", validate.Rule{}, []string{"done", "approve", "reject", "check_subtask"})
	for i := before; i < len(f.Issues()); i++ {
		f.Issues()[i].Message = "Aksi tidak dikenal"
	}
	in.Notes = f.Str("notes", nullish, validate.StrOpts{Max: 2000})
	in.SubtaskID = f.Str("subtask_id", optional, validate.StrOpts{})
	in.Checked = f.Bool("checked", optional)
	if err := issuesErr(f, ""); err != nil {
		return err
	}
	in.Action = *action
	out, err := h.svc.occurrenceAction(r.Context(), a, id, in)
	if err != nil {
		return err
	}
	return writeJSON(w, out)
}

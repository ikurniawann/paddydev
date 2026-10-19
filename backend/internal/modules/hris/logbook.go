package hris

import (
	"context"
	"fmt"
	"strings"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/validate"
)

// Department logbook (lib/hris/logbook, logbook-repo): templates with
// checklist items, dated entries filled from a template, submit and review.
// Everyone but super_admin/admin/hrd is locked to their own department.

// LogbookRepo is the logbook storage.
type LogbookRepo interface {
	LogbookEmployee(ctx context.Context, userID string) (*Row, error)
	ActiveDepartments(ctx context.Context) ([]*Row, error)
	LogbookTemplates(ctx context.Context, departmentID *string, includeInactive bool) ([]*Row, error)
	LogbookSummaryRows(ctx context.Context, departmentID *string, from, to string) ([]*Row, error)
	LogbookEntries(ctx context.Context, f logbookEntryFilter, page, limit int) ([]*Row, int64, error)
	InsertLogbookTemplate(ctx context.Context, f fields, items []fields) (*Row, error)
	LogbookTemplate(ctx context.Context, id string) (*Row, error)
	InsertLogbookEntry(ctx context.Context, f fields, templateID string) (*Row, error)
	LogbookItemEntry(ctx context.Context, itemID string) (*string, error)
	LogbookEntryGuard(ctx context.Context, id string) (*Row, error)
	UpdateLogbookItem(ctx context.Context, id string, f fields) (*Row, error)
	UpdateLogbookEntry(ctx context.Context, id string, f fields) (*Row, error)
	DeleteLogbookEntry(ctx context.Context, id string) error
	LogbookTemplateEntries(ctx context.Context, id string) (int64, error)
	ArchiveLogbookTemplate(ctx context.Context, id string, at any) error
	DeleteLogbookTemplate(ctx context.Context, id string) error
}

type logbookEntryFilter struct {
	DepartmentID           *string
	Status, Date, From, To string
}

func logbookForbidden() error {
	return httpx.Forbidden("Anda tidak berhak mengakses department ini")
}

func (a *Actor) logbookScope(requested string) (bool, *string) {
	return domain.ResolveDepartmentScope(domain.HasFullLogbookAccess(a.Role, a.PerformanceMenu), a.DepartmentID, requested)
}

func (a *Actor) assertDepartment(departmentID string) error {
	if ok, _ := a.logbookScope(departmentID); !ok {
		return logbookForbidden()
	}
	return nil
}

// logbookQuery is logbookListQuerySchema.
type logbookQuery struct {
	Resource, DepartmentID, IncludeInactive, From, To, Date, Status string
	Limit, Page                                                     float64
}

// ReadLogbook serves GET resource=me|departments|templates|summary|entries.
func (s *Service) ReadLogbook(ctx context.Context, a *Actor, u *auth.User, q logbookQuery) (*Row, error) {
	switch q.Resource {
	case "me":
		emp, err := s.repo.LogbookEmployee(ctx, a.UserID)
		if err != nil {
			return nil, err
		}
		return obj("data", obj("id", u.ID, "full_name", u.FullName, "role", u.Role, "brand_id", u.BrandID,
			"employee", nullable(emp), "can_review", domain.CanReviewLogbook(a.Role, a.PerformanceMenu),
			"is_full_access", domain.HasFullLogbookAccess(a.Role, a.PerformanceMenu))), nil
	case "departments":
		rows, err := s.repo.ActiveDepartments(ctx)
		return dataOf(rows), err
	}
	allowed, dept := a.logbookScope(q.DepartmentID)
	if !allowed {
		return nil, logbookForbidden()
	}
	switch q.Resource {
	case "templates":
		rows, err := s.repo.LogbookTemplates(ctx, dept, q.IncludeInactive == "true")
		return dataOf(rows), err
	case "summary":
		rows, err := s.repo.LogbookSummaryRows(ctx, dept, q.From, q.To)
		if err != nil {
			return nil, err
		}
		entries := make([]domain.LogbookSummaryEntry, len(rows))
		for i, r := range rows {
			entries[i] = domain.LogbookSummaryEntry{DepartmentID: r.Str("department_id"), Department: r.Get("department"),
				Status: r.Str("status"), Completion: numOrZero(r, "completion_percentage"), KPIScore: numOrZero(r, "kpi_score")}
		}
		return dataOf(domain.SummarizeLogbookEntries(entries)), nil
	}
	limit := q.Limit
	if limit == 0 {
		limit = 50
	}
	limit = min(100, max(1, limit))
	page := max(1, q.Page)
	rows, total, err := s.repo.LogbookEntries(ctx, logbookEntryFilter{DepartmentID: dept, Status: q.Status, Date: q.Date, From: q.From, To: q.To},
		int(page), int(limit))
	if err != nil {
		return nil, err
	}
	return obj("data", rows, "count", total, "page", page, "limit", limit), nil
}

// numOrZero is Number(value || 0).
func numOrZero(r *Row, key string) float64 {
	if r.Get(key) == nil {
		return 0
	}
	return r.Num(key)
}

const templateRequired = "department_id dan nama template wajib diisi"

type templateItemInput struct {
	Title, Description *string
	Weight             any
	IsRequired         *bool
}

type templateInput struct {
	DepartmentID, Name, Description, Frequency *string
	IsActive                                   *bool
	Items                                      []templateItemInput
}

// CreateTemplate: non-full-access users always write their own department.
func (s *Service) CreateTemplate(ctx context.Context, a *Actor, in templateInput) (*Row, error) {
	departmentID := a.DepartmentID
	if domain.HasFullLogbookAccess(a.Role, a.PerformanceMenu) {
		departmentID = in.DepartmentID
	}
	if departmentID == nil || *departmentID == "" {
		return nil, httpx.BadRequest(templateRequired)
	}
	if err := a.assertDepartment(*departmentID); err != nil {
		return nil, err
	}
	var items []fields
	for _, it := range in.Items {
		if it.Title == nil || domain.JSTrim(*it.Title) == "" {
			continue
		}
		required := true
		if it.IsRequired != nil {
			required = *it.IsRequired
		}
		items = append(items, fields{
			{"title", domain.JSTrim(*it.Title)}, {"description", nilIfEmpty(it.Description)},
			{"weight", domain.TemplateItemWeight(it.Weight)}, {"is_required", required}, {"sort_order", len(items)},
		})
	}
	if len(items) == 0 {
		return nil, httpx.BadRequest("Minimal satu checklist item harus diisi")
	}
	frequency := "daily"
	if in.Frequency != nil && *in.Frequency != "" {
		frequency = *in.Frequency
	}
	active := true
	if in.IsActive != nil {
		active = *in.IsActive
	}
	f := fields{{"department_id", *departmentID}, {"name", strOr(in.Name)}, {"description", nilIfEmpty(in.Description)},
		{"frequency", frequency}, {"is_active", active}, {"created_by", a.UserID}}
	var row *Row
	err := s.repo.InTx(ctx, func(r Repository) error {
		var err error
		row, err = r.InsertLogbookTemplate(ctx, f, items)
		return err
	})
	return row, err
}

type entryInput struct {
	TemplateID, EntryDate string
	Title                 *string
	Notes                 any
}

// CreateEntry fills a dated entry from an active template.
func (s *Service) CreateEntry(ctx context.Context, a *Actor, in entryInput) (*Row, error) {
	tpl, err := s.repo.LogbookTemplate(ctx, in.TemplateID)
	if err != nil {
		return nil, err
	}
	if tpl == nil {
		return nil, httpx.NotFound("Template tidak ditemukan")
	}
	if err := a.assertDepartment(tpl.Str("department_id")); err != nil {
		return nil, err
	}
	if !tpl.Bool("is_active") {
		return nil, httpx.Conflict("Template sudah diarsipkan")
	}
	note, valid := domain.NormalizeLogbookNote(in.Notes)
	if !valid {
		return nil, httpx.BadRequest("Catatan tidak valid/terlalu panjang")
	}
	title := tpl.Str("name") + " - " + in.EntryDate
	if in.Title != nil && domain.JSTrim(*in.Title) != "" {
		title = domain.JSTrim(*in.Title)
	}
	f := fields{{"template_id", tpl.Str("id")}, {"department_id", tpl.Str("department_id")},
		{"entry_date", in.EntryDate}, {"title", title}, {"notes", note}}
	var row *Row
	err = s.repo.InTx(ctx, func(r Repository) error {
		var err error
		row, err = r.InsertLogbookEntry(ctx, f, tpl.Str("id"))
		return err
	})
	if database.IsUniqueViolation(err) {
		return nil, httpx.Conflict("Logbook untuk template & tanggal ini sudah ada")
	}
	return row, err
}

// guardEntry loads an entry's status and department and checks the scope.
func (s *Service) guardEntry(ctx context.Context, a *Actor, id string) (*Row, error) {
	entry, err := s.repo.LogbookEntryGuard(ctx, id)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, httpx.NotFound("Logbook tidak ditemukan")
	}
	return entry, a.assertDepartment(entry.Str("department_id"))
}

// UpdateEntryItem ticks a checklist item or edits its note while draft.
func (s *Service) UpdateEntryItem(ctx context.Context, a *Actor, itemID string, checked any, notes any, notesSent bool) (*Row, error) {
	entryID, err := s.repo.LogbookItemEntry(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if entryID == nil {
		return nil, httpx.NotFound("Item tidak ditemukan")
	}
	entry, err := s.guardEntry(ctx, a, *entryID)
	if err != nil {
		return nil, err
	}
	if entry.Str("status") != "draft" {
		return nil, httpx.Conflict("Checklist hanya bisa diubah selama logbook masih draft")
	}
	now := s.clock()
	f := fields{{"updated_at", now}}
	if b, isBool := checked.(bool); isBool {
		f.add("is_checked", b)
		if b {
			f.add("checked_by", a.UserID)
			f.add("checked_at", now)
		} else {
			f.add("checked_by", nil)
			f.add("checked_at", nil)
		}
	}
	if notesSent {
		text, _ := notes.(string)
		if validate.UTF16Len(text) > domain.LogbookNoteMaxLength {
			return nil, httpx.BadRequest("Catatan terlalu panjang")
		}
		// Stored as is; every render goes through the DOMPurify allowlist.
		f.add("notes", nilIfEmpty(&text))
	}
	return s.repo.UpdateLogbookItem(ctx, itemID, f)
}

// SubmitEntry moves a draft to submitted. An absent note keeps the stored
// one (the TS clears it: its update payload carries `notes: undefined`).
func (s *Service) SubmitEntry(ctx context.Context, a *Actor, id string, notes any, notesSent bool) (*Row, error) {
	entry, err := s.guardEntry(ctx, a, id)
	if err != nil {
		return nil, err
	}
	if entry.Str("status") != "draft" {
		return nil, httpx.Conflict("Hanya logbook draft yang bisa disubmit")
	}
	note, valid := domain.NormalizeLogbookNote(notes)
	if !valid {
		return nil, httpx.BadRequest("Catatan tidak valid/terlalu panjang")
	}
	now := s.clock()
	f := fields{{"status", "submitted"}}
	if notesSent {
		f.add("notes", note)
	}
	f = append(f, kv{"submitted_by", a.UserID}, kv{"submitted_at", now}, kv{"updated_at", now})
	return s.repo.UpdateLogbookEntry(ctx, id, f)
}

// ReviewEntry reviews or rejects a submitted entry (super_admin, hrd).
func (s *Service) ReviewEntry(ctx context.Context, a *Actor, id string, status *string, reviewNotes any) (*Row, error) {
	if !domain.CanReviewLogbook(a.Role, a.PerformanceMenu) {
		return nil, httpx.Forbidden("Anda tidak berhak me-review logbook")
	}
	entry, err := s.guardEntry(ctx, a, id)
	if err != nil {
		return nil, err
	}
	if entry.Str("status") != "submitted" {
		return nil, httpx.Conflict("Hanya logbook berstatus submitted yang bisa direview")
	}
	note, valid := domain.NormalizeLogbookNote(reviewNotes)
	if !valid {
		return nil, httpx.BadRequest("Catatan review tidak valid/terlalu panjang")
	}
	next := "reviewed"
	if status != nil && *status == "rejected" {
		next = "rejected"
	}
	now := s.clock()
	return s.repo.UpdateLogbookEntry(ctx, id, fields{{"status", next}, {"review_notes", note},
		{"reviewed_by", a.UserID}, {"reviewed_at", now}, {"updated_at", now}})
}

func (s *Service) DeleteEntry(ctx context.Context, a *Actor, id string) (*Row, error) {
	entry, err := s.guardEntry(ctx, a, id)
	if err != nil {
		return nil, err
	}
	if entry.Str("status") != "draft" {
		return nil, httpx.Conflict("Hanya logbook draft yang bisa dihapus")
	}
	return msgOf("Logbook dihapus"), s.repo.DeleteLogbookEntry(ctx, id)
}

// DeleteTemplate archives a template entries use, else deletes it.
func (s *Service) DeleteTemplate(ctx context.Context, a *Actor, id string) (*Row, error) {
	tpl, err := s.repo.LogbookTemplate(ctx, id)
	if err != nil {
		return nil, err
	}
	if tpl == nil {
		return nil, httpx.NotFound("Template tidak ditemukan")
	}
	if err := a.assertDepartment(tpl.Str("department_id")); err != nil {
		return nil, err
	}
	used, err := s.repo.LogbookTemplateEntries(ctx, id)
	if err != nil {
		return nil, err
	}
	if used > 0 {
		if err := s.repo.ArchiveLogbookTemplate(ctx, id, s.clock()); err != nil {
			return nil, err
		}
		return obj("message", "Template diarsipkan (sudah dipakai logbook)", "archived", true), nil
	}
	return obj("message", "Template dihapus", "archived", false), s.repo.DeleteLogbookTemplate(ctx, id)
}

/* ── SQL ─────────────────────────────────────────────────────────────── */

func (s *store) LogbookEmployee(ctx context.Context, userID string) (*Row, error) {
	return queryRow(ctx, s.db, `SELECT id, department_id, `+
		embedOne("department", "id, name, code", "hris.departments", "id = employees.department_id")+
		` FROM hris.employees WHERE user_id = $1 LIMIT 1`, userID)
}

func (s *store) ActiveDepartments(ctx context.Context) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT id, name, code, is_active FROM hris.departments WHERE is_active = true ORDER BY name ASC`)
}

func (s *store) LogbookTemplates(ctx context.Context, departmentID *string, includeInactive bool) ([]*Row, error) {
	var where []string
	var args []any
	if departmentID != nil {
		args = append(args, *departmentID)
		where = append(where, "department_id = $1")
	}
	if !includeInactive {
		where = append(where, "is_active = true")
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "WHERE " + strings.Join(where, " AND ")
	}
	return queryRows(ctx, s.db, `SELECT *, `+
		embedOne("department", "id, name, code", "hris.departments", "id = hris_logbook_templates.department_id")+", "+
		embedMany("items", "*", "hris.hris_logbook_template_items", "template_id = hris_logbook_templates.id")+
		` FROM hris.hris_logbook_templates `+whereSQL+` ORDER BY created_at DESC`, args...)
}

func entryWhere(departmentID *string, eq map[string]string, from, to string) (string, []any) {
	var where []string
	var args []any
	add := func(cond, v string) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if departmentID != nil && *departmentID != "" {
		add("department_id = $%d", *departmentID)
	}
	for _, col := range []string{"status", "entry_date"} {
		if v := eq[col]; v != "" {
			add(col+" = $%d", v)
		}
	}
	if from != "" {
		add("entry_date >= $%d", from)
	}
	if to != "" {
		add("entry_date <= $%d", to)
	}
	if len(where) == 0 {
		return "", nil
	}
	return "WHERE " + strings.Join(where, " AND "), args
}

func (s *store) LogbookSummaryRows(ctx context.Context, departmentID *string, from, to string) ([]*Row, error) {
	where, args := entryWhere(departmentID, nil, from, to)
	return queryRows(ctx, s.db, `SELECT id, department_id, entry_date, status, completion_percentage, kpi_score, `+
		embedOne("department", "id, name, code", "hris.departments", "id = hris_logbook_entries.department_id")+
		` FROM hris.hris_logbook_entries `+where+` ORDER BY entry_date DESC`, args...)
}

func (s *store) LogbookEntries(ctx context.Context, f logbookEntryFilter, page, limit int) ([]*Row, int64, error) {
	where, args := entryWhere(f.DepartmentID, map[string]string{"status": f.Status, "entry_date": f.Date}, f.From, f.To)
	total, err := countRows(ctx, s.db, `SELECT count(*) FROM hris.hris_logbook_entries `+where, args...)
	if err != nil {
		return nil, 0, err
	}
	n := len(args)
	rows, err := queryRows(ctx, s.db, fmt.Sprintf(`SELECT *, %s, %s, %s FROM hris.hris_logbook_entries %s
		ORDER BY entry_date DESC LIMIT $%d OFFSET $%d`,
		embedOne("department", "id, name, code", "hris.departments", "id = hris_logbook_entries.department_id"),
		embedOne("template", "id, name, frequency", "hris.hris_logbook_templates", "id = hris_logbook_entries.template_id"),
		embedMany("items", "*", "hris.hris_logbook_entry_items", "entry_id = hris_logbook_entries.id"),
		where, n+1, n+2), append(args, limit, (page-1)*limit)...)
	return rows, total, err
}

// InsertLogbookTemplate writes the template and its items (in a transaction).
func (s *store) InsertLogbookTemplate(ctx context.Context, f fields, items []fields) (*Row, error) {
	row, err := insertRow(ctx, s.db, "hris.hris_logbook_templates", f, "*")
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		item = append(fields{{"template_id", row.Str("id")}}, item...)
		if _, err := insertRow(ctx, s.db, "hris.hris_logbook_template_items", item, "id"); err != nil {
			return nil, err
		}
	}
	return row, nil
}

func (s *store) LogbookTemplate(ctx context.Context, id string) (*Row, error) {
	row, err := queryRow(ctx, s.db, `SELECT * FROM hris.hris_logbook_templates WHERE id = $1`, id)
	return row, ignoreBadInput(err)
}

// InsertLogbookEntry writes the entry and copies the template's items in
// sort order (in a transaction).
func (s *store) InsertLogbookEntry(ctx context.Context, f fields, templateID string) (*Row, error) {
	row, err := insertRow(ctx, s.db, "hris.hris_logbook_entries", f, "*")
	if err != nil {
		return nil, err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO hris.hris_logbook_entry_items
		(entry_id, template_item_id, title, description, weight, is_required, sort_order)
		SELECT $1, id, title, description, weight, is_required, sort_order
		FROM hris.hris_logbook_template_items WHERE template_id = $2
		ORDER BY sort_order`, row.Str("id"), templateID)
	return row, err
}

func (s *store) LogbookItemEntry(ctx context.Context, itemID string) (*string, error) {
	var entryID string
	err := s.db.QueryRow(ctx, `SELECT entry_id::text FROM hris.hris_logbook_entry_items WHERE id = $1`, itemID).Scan(&entryID)
	if database.IsNoRows(err) || database.PgCode(err) == "22P02" {
		return nil, nil
	}
	return &entryID, err
}

func (s *store) LogbookEntryGuard(ctx context.Context, id string) (*Row, error) {
	row, err := queryRow(ctx, s.db, `SELECT id, status, department_id FROM hris.hris_logbook_entries WHERE id = $1`, id)
	return row, ignoreBadInput(err)
}

func (s *store) UpdateLogbookItem(ctx context.Context, id string, f fields) (*Row, error) {
	return updateOne(ctx, s.db, "hris.hris_logbook_entry_items", f, fields{{"id", id}}, "*")
}

func (s *store) UpdateLogbookEntry(ctx context.Context, id string, f fields) (*Row, error) {
	return updateOne(ctx, s.db, "hris.hris_logbook_entries", f, fields{{"id", id}}, "*")
}

func (s *store) DeleteLogbookEntry(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM hris.hris_logbook_entries WHERE id = $1`, id)
	return err
}

func (s *store) LogbookTemplateEntries(ctx context.Context, id string) (int64, error) {
	return countRows(ctx, s.db, `SELECT count(*) FROM hris.hris_logbook_entries WHERE template_id = $1`, id)
}

func (s *store) ArchiveLogbookTemplate(ctx context.Context, id string, at any) error {
	_, err := s.db.Exec(ctx, `UPDATE hris.hris_logbook_templates SET is_active = false, updated_at = $2 WHERE id = $1`, id, at)
	return err
}

func (s *store) DeleteLogbookTemplate(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM hris.hris_logbook_templates WHERE id = $1`, id)
	return err
}

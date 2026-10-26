package hris

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/validate"
)

/* ── Self service ────────────────────────────────────────────────────── */

func (h handlers) me(w http.ResponseWriter, r *http.Request, a *Actor) error {
	if a.EmployeeID == nil {
		return ok(w, dataOf(obj("employee", nil, "leave_balance", nil)))
	}
	out, err := h.svc.Me(r.Context(), *a.EmployeeID)
	if err != nil {
		return err
	}
	return ok(w, dataOf(out))
}

func (h handlers) beranda(w http.ResponseWriter, r *http.Request, a *Actor) error {
	if a.EmployeeID == nil {
		return ok(w, dataOf(obj("employee", nil)))
	}
	out, err := h.svc.Beranda(r.Context(), *a.EmployeeID)
	if err != nil {
		return err
	}
	return ok(w, dataOf(out))
}

func (h handlers) team(w http.ResponseWriter, r *http.Request, a *Actor) error {
	members := []*Row{}
	if a.EmployeeID != nil {
		var err error
		if members, err = h.svc.Team(r.Context(), *a.EmployeeID); err != nil {
			return err
		}
	}
	return ok(w, dataOf(obj("members", members)))
}

/* ── Announcements ───────────────────────────────────────────────────── */

func parseAnnouncement(f *validate.Form) announcementInput {
	in := announcementInput{BodyHTML: "", Status: "draft", TargetScope: "global", Tags: []string{}, DepartmentIDs: []string{}}
	if t := f.Str("title", validate.Rule{}, validate.StrOpts{Trim: true, Max: 200, Check: minLen(1, "Judul wajib diisi")}); t != nil {
		in.Title = *t
	}
	in.BodyHTML = f.StrDefault("body_html", "", validate.StrOpts{Max: 100_000})
	in.CoverImageURL = f.Str("cover_image_url", nullish, validate.StrOpts{})
	in.VideoURL = f.Str("video_url", nullish, validate.StrOpts{Trim: true})
	if tags := f.Strings("tags", validate.Rule{HasDefault: true}, 20, validate.StrOpts{Trim: true, Min: 1, Max: 40}); tags != nil {
		in.Tags = tags
	}
	if v := f.Enum("status", validate.Rule{HasDefault: true}, []string{"draft", "published"}); v != nil {
		in.Status = *v
	}
	in.IsPinned = f.BoolDefault("is_pinned", false)
	if v := f.Enum("target_scope", validate.Rule{HasDefault: true}, []string{"global", "department"}); v != nil {
		in.TargetScope = *v
	}
	if ids := f.Strings("department_ids", validate.Rule{HasDefault: true}, 1<<30, uuidOf); ids != nil {
		in.DepartmentIDs = ids
	}
	in.PublishAt = f.Str("publish_at", nullish, validate.StrOpts{Check: validate.DatetimeCheck})
	in.ExpiresAt = f.Str("expires_at", nullish, validate.StrOpts{Check: validate.DatetimeCheck})
	return in
}

func (h handlers) listAnnouncements(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	rows, err := h.svc.Announcements(r.Context(), get(r.URL.Query(), "status"))
	if err != nil {
		return err
	}
	return ok(w, dataOf(rows))
}

func (h handlers) createAnnouncement(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	actor, err := h.actor(r)
	if err != nil {
		return err
	}
	const message = "Validasi gagal"
	f, err := readForm(r, message)
	if err != nil {
		return err
	}
	in := parseAnnouncement(f)
	if err := formErr(f, message); err != nil {
		return err
	}
	var createdBy *string
	if actor != nil {
		createdBy = actor.EmployeeID
	}
	row, err := h.svc.CreateAnnouncement(r.Context(), in, createdBy)
	if err != nil {
		return err
	}
	return ok(w, dataMsg(row, "Pengumuman berhasil dibuat"))
}

func (h handlers) getAnnouncement(w http.ResponseWriter, r *http.Request, a *Actor) error {
	id, err := requireUUID(r.PathValue("id"), "")
	if err != nil {
		return err
	}
	row, err := h.svc.Announcement(r.Context(), id, canManageAnnouncements(a.Role), a.EmployeeID)
	if err != nil {
		return err
	}
	return ok(w, dataOf(row))
}

func (h handlers) updateAnnouncement(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	id, err := requireUUID(r.PathValue("id"), "")
	if err != nil {
		return err
	}
	const message = "Validasi gagal"
	f, err := readForm(r, message)
	if err != nil {
		return err
	}
	in := parseAnnouncement(f)
	if err := formErr(f, message); err != nil {
		return err
	}
	row, err := h.svc.UpdateAnnouncement(r.Context(), id, in)
	if err != nil {
		return err
	}
	return ok(w, dataMsg(row, "Pengumuman diperbarui"))
}

func (h handlers) deleteAnnouncement(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	id, err := requireUUID(r.PathValue("id"), "")
	if err != nil {
		return err
	}
	if err := h.svc.DeleteAnnouncement(r.Context(), id); err != nil {
		return err
	}
	return ok(w, msgOf("Pengumuman dihapus"))
}

func (h handlers) readAnnouncement(w http.ResponseWriter, r *http.Request) error {
	a, err := h.actor(r)
	if err != nil {
		return err
	}
	if a == nil || a.EmployeeID == nil {
		return httpx.Forbidden("Akun tidak tertaut karyawan")
	}
	id, err := requireUUID(r.PathValue("id"), "")
	if err != nil {
		return err
	}
	if err := h.svc.MarkAnnouncementRead(r.Context(), id, *a.EmployeeID); err != nil {
		return err
	}
	return ok(w, obj("ok", true))
}

func (h handlers) announcementFeed(w http.ResponseWriter, r *http.Request, a *Actor) error {
	if a.EmployeeID == nil {
		return ok(w, obj("data", []any{}, "unread", 0))
	}
	rows, unread, err := h.svc.AnnouncementFeed(r.Context(), *a.EmployeeID)
	if err != nil {
		return err
	}
	return ok(w, obj("data", rows, "unread", unread))
}

// unreadAnnouncements never fails: the sidebar badge answers {unread: 0}.
func (h handlers) unreadAnnouncements(w http.ResponseWriter, r *http.Request) {
	var n int64
	if a, err := h.actor(r); err == nil && a != nil && a.EmployeeID != nil {
		if count, err := h.svc.UnreadAnnouncements(r.Context(), *a.EmployeeID); err == nil {
			n = count
		} else {
			h.svc.log.ErrorContext(r.Context(), "[announcements/unread-count] GET failed", "error", err)
		}
	}
	_ = ok(w, obj("unread", n))
}

/* ── Notifications and badges ────────────────────────────────────────── */

func (h handlers) listNotifications(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	f := queryForm(r)
	limit := coerceInt(f, "limit", validate.Bound(1), validate.Bound(200), 50)
	unread := f.Str("unread", optional, validate.StrOpts{})
	if err := formErr(f, ""); err != nil {
		return err
	}
	out, err := h.svc.Notifications(r.Context(), u.ID, unread != nil && *unread == "true", limit)
	if err != nil {
		return err
	}
	return ok(w, out)
}

func (h handlers) markNotifications(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	id := f.Str("notification_id", nullish, validate.StrOpts{})
	all := f.Bool("mark_all", nullish)
	if f.Valid() && !(all != nil && *all) && (id == nil || *id == "") {
		f.Fail(nil, "custom", "notification_id or mark_all is required")
	}
	if err := formErr(f, ""); err != nil {
		return err
	}
	out, err := h.svc.MarkNotificationsRead(r.Context(), u.ID, all != nil && *all, id)
	if err != nil {
		return err
	}
	return ok(w, out)
}

func (h handlers) navBadges(w http.ResponseWriter, r *http.Request) error {
	a, err := h.actor(r)
	if err != nil {
		return err
	}
	if a == nil {
		return ok(w, obj("badges", newRow()))
	}
	badges, err := h.svc.NavBadges(r.Context(), a)
	if err != nil {
		// Badges are decoration: a failure must not break the navigation.
		h.svc.log.ErrorContext(r.Context(), "[nav-badges] gagal membangun badge", "error", err)
		badges = newRow()
	}
	return ok(w, obj("badges", badges))
}

func (h handlers) markModuleSeen(w http.ResponseWriter, r *http.Request) error {
	a, err := h.actor(r)
	if err != nil {
		return err
	}
	if a == nil || a.EmployeeID == nil {
		return httpx.Forbidden("Karyawan tidak ditemukan")
	}
	const message = "Modul tidak valid"
	f, err := readForm(r, message)
	if err != nil {
		return err
	}
	module, _ := rawValue(f, "module")
	name, isStr := module.(string)
	if f.Fields() != nil && (!isStr || !isESSModule(name)) {
		f.Fail("module", "custom", "Invalid input")
	}
	if err := formErr(f, message); err != nil {
		return err
	}
	if err := h.svc.MarkESSModuleSeen(r.Context(), *a.EmployeeID, name); err != nil {
		return err
	}
	return ok(w, obj("success", true))
}

/* ── Logbook ─────────────────────────────────────────────────────────── */

// withLogbook is requireLogbookActor plus the signed-in profile.
func (h handlers) withLogbook(fn func(w http.ResponseWriter, r *http.Request, a *Actor, u *auth.User) error) http.Handler {
	return handle(func(w http.ResponseWriter, r *http.Request) error {
		u, err := h.auth.CurrentUser(r)
		if err != nil {
			return err
		}
		if u == nil {
			return httpx.Unauthorized("Unauthorized")
		}
		a, err := h.actorFor(r.Context(), u)
		if err != nil {
			return err
		}
		granted, err := h.auth.GrantedMenuCodes(r.Context(), u.ID, u.Role)
		if err != nil {
			return err
		}
		a.PerformanceMenu = iam.HasPerformanceHRMenu(granted)
		return fn(w, r, a, u)
	})
}

func (h handlers) readLogbook(w http.ResponseWriter, r *http.Request, a *Actor, u *auth.User) error {
	f := queryForm(r)
	s := func(k string) string { return strOr(f.Str(k, optional, validate.StrOpts{})) }
	q := logbookQuery{Resource: f.StrDefault("resource", "entries", validate.StrOpts{}),
		DepartmentID: s("department_id"), IncludeInactive: s("include_inactive"),
		From: s("from"), To: s("to"), Date: s("date"), Status: s("status")}
	for _, k := range []string{"limit", "page"} {
		if raw, present := f.Fields()[k]; present {
			n := domain.JSNumber(raw.(string))
			if !domain.IsFinite(n) {
				f.Fail(k, "invalid_type", "Invalid input: expected number, received NaN")
			}
			if k == "limit" {
				q.Limit = n
			} else {
				q.Page = n
			}
		}
	}
	if err := formErr(f, ""); err != nil {
		return err
	}
	out, err := h.svc.ReadLogbook(r.Context(), a, u, q)
	if err != nil {
		return err
	}
	return ok(w, out)
}

func unknownAction() error { return httpx.Status(422, "Unknown action") }

// actionForm reads the { action, … } body (readJson(looseObject)).
func actionForm(r *http.Request) (*validate.Form, string, error) {
	f, err := readForm(r, "")
	if err != nil {
		return nil, "", err
	}
	if f.Fields() == nil {
		return nil, "", formErr(f, "")
	}
	action, _ := f.Fields()["action"].(string)
	return f, action, nil
}

func (h handlers) postLogbook(w http.ResponseWriter, r *http.Request, a *Actor, _ *auth.User) error {
	f, action, err := actionForm(r)
	if err != nil {
		return err
	}
	switch action {
	case "create-template":
		in := templateInput{
			DepartmentID: f.Str("department_id", nullish, validate.StrOpts{}),
			Name:         reqStr(f, "name", templateRequired, validate.StrOpts{Trim: true, Check: minLen(1, templateRequired)}),
			Description:  f.Str("description", nullish, validate.StrOpts{}),
			Frequency:    f.Str("frequency", nullish, validate.StrOpts{}),
			IsActive:     f.Bool("is_active", nullish),
		}
		f.List("items", nullish, 1<<30, func(sub *validate.Form, i int, v any) {
			item := sub.Item(i, v)
			if item.Fields() == nil {
				return
			}
			weight, _ := rawValue(item, "weight")
			if n, isNum := weight.(json.Number); isNum {
				weight, _ = n.Float64()
			}
			in.Items = append(in.Items, templateItemInput{
				Title: item.Str("title", nullish, validate.StrOpts{}), Description: item.Str("description", nullish, validate.StrOpts{}),
				Weight: weight, IsRequired: item.Bool("is_required", nullish)})
		})
		if err := formErr(f, ""); err != nil {
			return err
		}
		row, err := h.svc.CreateTemplate(r.Context(), a, in)
		if err != nil {
			return err
		}
		return reply(w, http.StatusCreated, dataOf(row))
	case "create-entry":
		const required = "template_id dan entry_date wajib diisi"
		in := entryInput{}
		in.TemplateID = strOr(reqStr(f, "template_id", required, validate.StrOpts{Check: minLen(1, required)}))
		in.EntryDate = strOr(reqStr(f, "entry_date", required, validate.StrOpts{
			Check: both(minLen(1, required), matches(domain.DateRe, "Format entry_date harus YYYY-MM-DD"))}))
		in.Title = f.Str("title", nullish, validate.StrOpts{})
		in.Notes, _ = rawValue(f, "notes")
		if err := formErr(f, ""); err != nil {
			return err
		}
		row, err := h.svc.CreateEntry(r.Context(), a, in)
		if err != nil {
			return err
		}
		return reply(w, http.StatusCreated, dataOf(row))
	}
	return unknownAction()
}

func (h handlers) patchLogbook(w http.ResponseWriter, r *http.Request, a *Actor, _ *auth.User) error {
	f, action, err := actionForm(r)
	if err != nil {
		return err
	}
	var row *Row
	switch action {
	case "update-item":
		id := reqStr(f, "item_id", "item_id is required", validate.StrOpts{Check: minLen(1, "item_id is required")})
		checked, _ := rawValue(f, "is_checked")
		notes, notesSent := rawValue(f, "notes")
		if err := formErr(f, ""); err != nil {
			return err
		}
		row, err = h.svc.UpdateEntryItem(r.Context(), a, *id, checked, notes, notesSent)
	case "submit-entry":
		id := reqStr(f, "entry_id", "entry_id is required", validate.StrOpts{Check: minLen(1, "entry_id is required")})
		notes, notesSent := rawValue(f, "notes")
		if err := formErr(f, ""); err != nil {
			return err
		}
		row, err = h.svc.SubmitEntry(r.Context(), a, *id, notes, notesSent)
	case "review-entry":
		id := reqStr(f, "entry_id", "entry_id is required", validate.StrOpts{Check: minLen(1, "entry_id is required")})
		status := f.Str("status", nullish, validate.StrOpts{})
		notes, _ := rawValue(f, "review_notes")
		if err := formErr(f, ""); err != nil {
			return err
		}
		row, err = h.svc.ReviewEntry(r.Context(), a, *id, status, notes)
	default:
		return unknownAction()
	}
	if err != nil {
		return err
	}
	return ok(w, dataOf(row))
}

func (h handlers) deleteLogbook(w http.ResponseWriter, r *http.Request, a *Actor, _ *auth.User) error {
	f := queryForm(r)
	resource := f.Str("resource", optional, validate.StrOpts{})
	id := reqStr(f, "id", "id is required", validate.StrOpts{Check: minLen(1, "id is required")})
	if err := formErr(f, ""); err != nil {
		return err
	}
	var out *Row
	var err error
	switch strOr(resource) {
	case "entry":
		out, err = h.svc.DeleteEntry(r.Context(), a, *id)
	case "template":
		out, err = h.svc.DeleteTemplate(r.Context(), a, *id)
	default:
		return httpx.Status(422, "Unknown resource")
	}
	if err != nil {
		return err
	}
	return ok(w, out)
}

/* ── Report ──────────────────────────────────────────────────────────── */

func (h handlers) report(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	f := queryForm(r)
	month := coerceInt(f, "month", validate.Bound(1), validate.Bound(12), 0)
	year := coerceInt(f, "year", validate.Bound(2000), validate.Bound(2100), 0)
	if err := formErr(f, "Periode tidak valid"); err != nil {
		return err
	}
	now := domain.NowWIB(h.svc.now())
	if month == 0 {
		month = int(now.Month())
	}
	if year == 0 {
		year = now.Year()
	}
	out, err := h.svc.Report(r.Context(), month, year)
	if err != nil {
		return err
	}
	return ok(w, out)
}

/* ── Master data ─────────────────────────────────────────────────────── */

var masterReaders = slices.Concat(iam.Hris, iam.SettingsUsers)

// masterBody is validateBody: a body that is not JSON is a 500 (the TS lets
// the SyntaxError through), schema failures are "Validation failed".
func masterBody(r *http.Request) (*validate.Form, error) {
	body, present := validate.ReadBody(r)
	if !present {
		return nil, errBadJSON
	}
	return validate.New(body, true), nil
}

var errBadJSON = errors.New("Unexpected end of JSON input")

// requiredTrim is z.string(msg).trim().min(1, msg).
func requiredTrim(f *validate.Form, key, msg string) string {
	return strOr(reqStr(f, key, msg, validate.StrOpts{Trim: true, Check: minLen(1, msg)}))
}

// optionalText is z.string().nullish().transform(v => v || null).
func optionalText(f *validate.Form, key string) *string {
	return nilIfEmpty(f.Str(key, nullish, validate.StrOpts{}))
}

func parseDepartment(f *validate.Form) masterInput {
	const msg = "Nama dan kode wajib diisi"
	return masterInput{Name: requiredTrim(f, "name", msg), Code: strings.ToUpper(requiredTrim(f, "code", msg)),
		Description: optionalText(f, "description"), IsActive: f.Bool("is_active", optional)}
}

func parseEmploymentStatus(f *validate.Form) masterInput {
	const msg = "Kode dan nama wajib diisi"
	return masterInput{Code: strings.ToLower(requiredTrim(f, "code", msg)), Name: requiredTrim(f, "name", msg),
		Color: optionalText(f, "color"), Description: optionalText(f, "description"), IsActive: f.Bool("is_active", optional)}
}

func parsePosition(f *validate.Form) masterInput {
	in := masterInput{Title: requiredTrim(f, "title", "Nama jabatan wajib diisi"),
		Department: optionalText(f, "department"), Level: optionalText(f, "level"), IsActive: f.Bool("is_active", optional)}
	in.BrandID = nilIfEmpty(f.Str("brand_id", nullish, validate.StrOpts{Check: func(s string) (string, string, bool) {
		return "invalid_union", "Invalid input", s == "" || validate.IsUUID(s)
	}}))
	return in
}

// masterRoutes builds the GET/POST and PUT/DELETE handlers of one master
// table.
type masterKind struct {
	parse            func(*validate.Form) masterInput
	list             func(h handlers, r *http.Request) (any, error)
	save             func(h handlers, r *http.Request, id *string, in masterInput) (*Row, error)
	remove           func(h handlers, r *http.Request, id string) error
	created, updated string
	deleted          string
}

func (h handlers) masterList(k masterKind) http.Handler {
	return h.withMenu(masterReaders, func(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
		rows, err := k.list(h, r)
		if err != nil {
			return err
		}
		return ok(w, dataOf(rows))
	})
}

func (h handlers) masterSave(k masterKind, withID bool) http.Handler {
	return h.withMenu(iam.HrisMaster, func(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
		var id *string
		if withID {
			v, err := requireUUID(r.PathValue("id"), "ID tidak valid")
			if err != nil {
				return err
			}
			id = &v
		}
		f, err := masterBody(r)
		if err != nil {
			return err
		}
		in := k.parse(f)
		if err := f.Err("Validation failed"); err != nil {
			return err
		}
		row, err := k.save(h, r, id, in)
		if err != nil {
			return err
		}
		if withID {
			return ok(w, dataMsg(row, k.updated))
		}
		return reply(w, http.StatusCreated, dataMsg(row, k.created))
	})
}

func (h handlers) masterDelete(k masterKind) http.Handler {
	return h.withMenu(iam.HrisMaster, func(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
		id, err := requireUUID(r.PathValue("id"), "ID tidak valid")
		if err != nil {
			return err
		}
		if err := k.remove(h, r, id); err != nil {
			return err
		}
		return ok(w, msgOf(k.deleted))
	})
}

var (
	departmentsKind = masterKind{
		parse: parseDepartment,
		list:  func(h handlers, r *http.Request) (any, error) { return h.svc.MasterDepartments(r.Context()) },
		save: func(h handlers, r *http.Request, id *string, in masterInput) (*Row, error) {
			return h.svc.SaveDepartment(r.Context(), id, in)
		},
		remove:  func(h handlers, r *http.Request, id string) error { return h.svc.DeleteDepartment(r.Context(), id) },
		created: "Departemen berhasil ditambahkan", updated: "Departemen berhasil diupdate", deleted: "Departemen berhasil dihapus",
	}
	statusesKind = masterKind{
		parse: parseEmploymentStatus,
		list:  func(h handlers, r *http.Request) (any, error) { return h.svc.EmploymentStatuses(r.Context()) },
		save: func(h handlers, r *http.Request, id *string, in masterInput) (*Row, error) {
			return h.svc.SaveEmploymentStatus(r.Context(), id, in)
		},
		remove: func(h handlers, r *http.Request, id string) error {
			return h.svc.DeleteEmploymentStatus(r.Context(), id)
		},
		created: "Status kepegawaian berhasil ditambahkan", updated: "Status kepegawaian berhasil diupdate",
		deleted: "Status kepegawaian berhasil dihapus",
	}
	positionsKind = masterKind{
		parse: parsePosition,
		list:  func(h handlers, r *http.Request) (any, error) { return h.svc.Positions(r.Context(), nil) },
		save: func(h handlers, r *http.Request, id *string, in masterInput) (*Row, error) {
			return h.svc.SavePosition(r.Context(), id, in)
		},
		remove:  func(h handlers, r *http.Request, id string) error { return h.svc.DeletePosition(r.Context(), id) },
		created: "Jabatan berhasil ditambahkan", updated: "Jabatan berhasil diupdate", deleted: "Jabatan berhasil dihapus",
	}
)

/* ── Onboarding / offboarding ────────────────────────────────────────── */

var checklistReaders = slices.Concat(employeeRecordManagers, iam.HrisRecruitment)

var onboardingCategories = []string{"admin", "it", "hr", "manager", "general"}

func parseOnboardingTask(f *validate.Form) onboardingTask {
	return onboardingTask{
		TaskName:    str(f, "task_name", optional, validate.StrOpts{Min: 1}),
		Category:    str(f, "category", optional, enumOf(onboardingCategories, "")),
		Description: str(f, "description", nullish, validate.StrOpts{}),
		Priority:    opt[int]{Val: f.Int("priority", optional, validate.NumOpts{Min: validate.Bound(1), Max: validate.Bound(3)}), Sent: sent(f, "priority")},
		DueDate:     str(f, "due_date", nullish, validate.StrOpts{}),
		AssignedTo:  str(f, "assigned_to", nullish, validate.StrOpts{}),
	}
}

func (h handlers) listOnboarding(w http.ResponseWriter, r *http.Request) error {
	employeeID := r.PathValue("employee_id")
	if err := h.requireEmployeeAccess(r, employeeID, checklistReaders); err != nil {
		return err
	}
	f := queryForm(r)
	category := f.Str("category", optional, validate.StrOpts{})
	completedRaw := f.Str("completed", optional, validate.StrOpts{})
	var completed *bool
	if completedRaw != nil && (*completedRaw == "true" || *completedRaw == "false") {
		v := *completedRaw == "true"
		completed = &v
	}
	out, err := h.svc.OnboardingList(r.Context(), employeeID, category, completed)
	if err != nil {
		return err
	}
	return ok(w, out)
}

func (h handlers) postOnboarding(w http.ResponseWriter, r *http.Request, a *Actor) error {
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	action := f.Str("action", optional, validate.StrOpts{})
	taskID := f.Str("task_id", optional, validate.StrOpts{})
	notes := f.Str("completion_notes", nullish, validate.StrOpts{})
	task := parseOnboardingTask(f)
	if err := formErr(f, ""); err != nil {
		return err
	}
	out, err := h.svc.ActOnOnboarding(r.Context(), a, r.PathValue("employee_id"), strOr(action), strOr(taskID), notes, task)
	if err != nil {
		return err
	}
	return ok(w, out)
}

func (h handlers) putOnboarding(w http.ResponseWriter, r *http.Request, a *Actor) error {
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	taskID := reqStr(f, "task_id", "task_id is required", validate.StrOpts{Check: minLen(1, "task_id is required")})
	task := parseOnboardingTask(f)
	if err := formErr(f, ""); err != nil {
		return err
	}
	out, err := h.svc.UpdateOnboardingTask(r.Context(), a, r.PathValue("employee_id"), *taskID, task)
	if err != nil {
		return err
	}
	return ok(w, out)
}

func (h handlers) listOffboarding(w http.ResponseWriter, r *http.Request) error {
	employeeID := r.PathValue("employee_id")
	if err := h.requireEmployeeAccess(r, employeeID, checklistReaders); err != nil {
		return err
	}
	rows, err := h.svc.Offboarding(r.Context(), employeeID)
	if err != nil {
		return err
	}
	return ok(w, dataOf(rows))
}

func (h handlers) postOffboarding(w http.ResponseWriter, r *http.Request, a *Actor) error {
	const message = "Validation failed"
	f, err := readForm(r, message)
	if err != nil {
		return err
	}
	in := resignation{
		Type:            strOr(f.Enum("resignation_type", validate.Rule{}, []string{"voluntary", "termination", "layoff", "end_of_contract"})),
		ResignationDate: strOr(f.Str("resignation_date", validate.Rule{}, validate.StrOpts{})),
		LastWorkingDay:  strOr(f.Str("last_working_day", validate.Rule{}, validate.StrOpts{})),
		Reason:          f.Str("reason", nullish, validate.StrOpts{}),
	}
	if err := formErr(f, message); err != nil {
		return err
	}
	row, err := h.svc.InitiateOffboarding(r.Context(), a, r.PathValue("employee_id"), in)
	if err != nil {
		return err
	}
	return ok(w, obj("message", "Offboarding process initiated successfully", "data", row))
}

var offboardingPassthrough = []string{"exit_interview_date", "exit_interview_conducted_by", "exit_interview_notes",
	"final_payroll_date", "final_payroll_amount", "final_payroll_notes"}

func (h handlers) putOffboarding(w http.ResponseWriter, r *http.Request, a *Actor) error {
	body, present := validate.ReadBody(r)
	if !present {
		return httpx.BadRequest("Body JSON tidak valid")
	}
	f := validate.New(body, true)
	in := offboardingUpdate{
		ClearanceType: f.Enum("clearance_type", optional, []string{"hrd", "it", "finance", "manager"}),
		Cleared:       boolean(f, "cleared", optional),
		Notes:         str(f, "notes", nullish, validate.StrOpts{}),
	}
	if raw, sent := rawValue(f, "asset_updates"); sent {
		if _, isObj := raw.(map[string]any); !isObj {
			f.Fail("asset_updates", "invalid_type", "Invalid input: expected record, received "+jsonType(raw))
		}
	}
	in.Status = f.Enum("status", optional, []string{"submitted", "notice_period", "exit_interview", "completed"})
	for _, col := range offboardingPassthrough {
		if col == "final_payroll_amount" {
			if v := number(f, col, nullish, validate.NumOpts{}); v.Sent {
				in.Passthrough.add(col, v.Val)
			}
			continue
		}
		if v := str(f, col, nullish, validate.StrOpts{}); v.Sent {
			in.Passthrough.add(col, v.Val)
		}
	}
	if err := formErr(f, ""); err != nil {
		return err
	}
	if raw, sent := rawValue(f, "asset_updates"); sent {
		in.AssetUpdates = toRow(raw)
	}
	row, err := h.svc.UpdateOffboarding(r.Context(), a, r.PathValue("employee_id"), in)
	if err != nil {
		return err
	}
	return ok(w, obj("message", "Offboarding updated successfully", "data", row))
}

func jsonType(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case json.Number:
		return "number"
	case bool:
		return "boolean"
	case []any:
		return "array"
	}
	return "object"
}

// toRow re-encodes a decoded JSON object as a Row.
func toRow(raw any) *Row {
	b, _ := json.Marshal(raw)
	row, err := decodeObject(b)
	if err != nil {
		return newRow()
	}
	return row
}

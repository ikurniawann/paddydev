package hris

import (
	"net/http"
	"regexp"
	"slices"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/validate"
)

// employeeRecordManagers is EMPLOYEE_RECORD_MANAGERS (full records incl.
// bank account, NPWP, KTP).
var employeeRecordManagers = slices.Concat(iam.HrisKepegawaian, iam.SettingsUsers)

// employeeDirectoryReaders is EMPLOYEE_DIRECTORY_READERS (no personal data).
var employeeDirectoryReaders = slices.Concat(employeeRecordManagers, iam.HrisWorkforce,
	iam.HrisCompensation, iam.HrisOrganization, iam.Ticketing)

// zodEmail is zod v4's z.string().email() pattern.
var zodEmail = regexp.MustCompile(`^(?:[A-Za-z0-9_'+\-]+(?:\.[A-Za-z0-9_'+\-]+)*)@(?:[A-Za-z0-9](?:[A-Za-z0-9\-]*[A-Za-z0-9])?\.)+[A-Za-z]{2,}$`)

var emailCheck = func(s string) (string, string, bool) {
	return "invalid_format", "Invalid email address", zodEmail.MatchString(s)
}

// employeeInput is employeeUpdateSchema (and the create schema built on it).
type employeeInput struct {
	nip, fullName, email, phone, joinDate, endDate, employmentStatus opt[string]
	isActive                                                         opt[bool]
	departmentID, sectionID, jobTitleID, reportingTo                 opt[string]
	rest                                                             []namedText
}

type namedText struct {
	col string
	v   opt[string]
}

// personalFields are the plain nullable text columns after reporting_to.
var personalFields = []struct {
	col string
	max int
}{
	{"ktp", 32}, {"npwp", 32}, {"birth_date", 10}, {"gender", 20}, {"marital_status", 20},
	{"address", 1000}, {"city", 120}, {"province", 120}, {"postal_code", 10}, {"bank_name", 120},
	{"bank_account", 64}, {"bpjs_tk", 64}, {"bpjs_kesehatan", 64}, {"emergency_contact_name", 255},
	{"emergency_contact_phone", 50}, {"emergency_contact_relationship", 100}, {"photo_url", 1000}, {"notes", 5000},
}

const employeeRequired = "Field yang wajib diisi: nama lengkap, email, tanggal bergabung, status karyawan"

func parseEmployee(f *validate.Form, create bool) employeeInput {
	text := func(key string, max int) opt[string] { return str(f, key, nullish, validate.StrOpts{Max: max}) }
	uuid := func(key string) opt[string] { return str(f, key, nullish, uuidOf) }
	required := func(key string, o validate.StrOpts) opt[string] {
		return opt[string]{Val: reqStr(f, key, employeeRequired, o), Sent: true}
	}
	var in employeeInput
	in.nip = text("nip", 50)
	if create {
		in.fullName = required("full_name", validate.StrOpts{Trim: true, Max: 255, Check: minLen(1, employeeRequired)})
		in.email = required("email", validate.StrOpts{Max: 255, Check: both(minLen(1, employeeRequired), emailCheck)})
	} else {
		in.fullName = str(f, "full_name", optional, validate.StrOpts{Trim: true, Min: 1, Max: 255})
		in.email = str(f, "email", optional, validate.StrOpts{Max: 255, Check: emailCheck})
	}
	in.phone = text("phone", 50)
	if create {
		in.joinDate = required("join_date", validate.StrOpts{Max: 10, Check: minLen(1, employeeRequired)})
	} else {
		in.joinDate = text("join_date", 10)
		in.endDate = text("end_date", 10)
	}
	if create {
		in.employmentStatus = required("employment_status", validate.StrOpts{Max: 50, Check: minLen(1, employeeRequired)})
	} else {
		in.employmentStatus = str(f, "employment_status", optional, validate.StrOpts{Max: 50})
		in.isActive = boolean(f, "is_active", optional)
	}
	in.departmentID, in.sectionID, in.jobTitleID, in.reportingTo = uuid("department_id"), uuid("section_id"), uuid("job_title_id"), uuid("reporting_to")
	for _, p := range personalFields {
		in.rest = append(in.rest, namedText{p.col, text(p.col, p.max)})
	}
	return in
}

// columns are the sent keys, in schema order, as column values.
func (in employeeInput) columns() fields {
	var f fields
	addStr := func(col string, v opt[string]) {
		if v.Sent {
			f.add(col, v.Val)
		}
	}
	addStr("nip", in.nip)
	addStr("full_name", in.fullName)
	addStr("email", in.email)
	addStr("phone", in.phone)
	addStr("join_date", in.joinDate)
	addStr("end_date", in.endDate)
	addStr("employment_status", in.employmentStatus)
	if in.isActive.Sent {
		f.add("is_active", in.isActive.Val)
	}
	addStr("department_id", in.departmentID)
	addStr("section_id", in.sectionID)
	addStr("job_title_id", in.jobTitleID)
	addStr("reporting_to", in.reportingTo)
	for _, r := range in.rest {
		addStr(r.col, r.v)
	}
	return f
}

func (h handlers) listEmployees(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	p := domain.ParseDirectoryParams(queryOf(r))
	rows, total, err := h.svc.EmployeeDirectory(r.Context(), p)
	if err != nil {
		return err
	}
	return ok(w, obj("data", rows, "total", total, "page", p.Page, "per_page", p.Limit))
}

func (h handlers) createEmployee(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	in := parseEmployee(f, true)
	if err := formErr(f, ""); err != nil {
		return err
	}
	row, err := h.svc.CreateEmployee(r.Context(), in)
	if err != nil {
		return err
	}
	return ok(w, dataMsg(row, "Karyawan berhasil ditambahkan"))
}

func (h handlers) getEmployee(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	if err := h.requireEmployeeAccess(r, id, employeeRecordManagers); err != nil {
		return err
	}
	row, err := h.svc.Employee(r.Context(), id)
	if err != nil {
		return err
	}
	return ok(w, dataOf(row))
}

func (h handlers) updateEmployee(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	const message = "Data karyawan tidak valid"
	f, err := readForm(r, message)
	if err != nil {
		return err
	}
	in := parseEmployee(f, false)
	if err := formErr(f, message); err != nil {
		return err
	}
	granted, err := h.auth.GrantedMenuCodes(r.Context(), u.ID, u.Role)
	if err != nil {
		return err
	}
	accounts := iam.HasAnyMenuPrefix(granted, iam.SettingsUsers)
	row, err := h.svc.UpdateEmployee(r.Context(), r.PathValue("id"), in, accounts)
	if err != nil {
		return err
	}
	return ok(w, dataMsg(row, "Data karyawan berhasil diupdate"))
}

func (h handlers) deleteEmployee(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	row, err := h.svc.DeactivateEmployee(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	return ok(w, dataMsg(row, "Karyawan berhasil dihapus (non-aktif)"))
}

/* ── Documents ───────────────────────────────────────────────────────── */

func (h handlers) listEmployeeDocuments(w http.ResponseWriter, r *http.Request) error {
	employeeID := r.URL.Query().Get("employee_id")
	if employeeID == "" {
		return httpx.BadRequest("employee_id diperlukan")
	}
	if err := h.requireEmployeeAccess(r, employeeID, employeeRecordManagers); err != nil {
		return err
	}
	rows, err := h.svc.EmployeeDocuments(r.Context(), employeeID)
	if err != nil {
		return err
	}
	return ok(w, dataOf(rows))
}

const documentRequired = "Field wajib: employee_id, document_type, document_name, file_url"

// emptyAsAbsent is the preprocess of employeeDocumentCreateSchema's optional
// fields: "" and 0 count as not sent.
func emptyAsAbsent(f *validate.Form, keys ...string) {
	obj := f.Fields()
	for _, k := range keys {
		switch v := obj[k].(type) {
		case string:
			if v == "" {
				delete(obj, k)
			}
		case interface{ String() string }:
			if domain.JSNumber(v.String()) == 0 {
				delete(obj, k)
			}
		}
	}
}

func (h handlers) createEmployeeDocument(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	if f.Fields() != nil {
		emptyAsAbsent(f, "file_size_kb", "mime_type", "issue_date", "expiry_date", "notes")
	}
	req := func(key string, max int) *string {
		return reqStr(f, key, documentRequired, validate.StrOpts{Max: max, Check: minLen(1, documentRequired)})
	}
	var c fields
	c.add("employee_id", req("employee_id", 64))
	c.add("document_type", req("document_type", 120))
	c.add("document_name", req("document_name", 255))
	c.add("file_url", req("file_url", 1000))
	c.add("file_size_kb", f.Num("file_size_kb", nullish, validate.NumOpts{Min: validate.Bound(0)}))
	c.add("mime_type", f.Str("mime_type", nullish, validate.StrOpts{Max: 120}))
	c.add("issue_date", f.Str("issue_date", nullish, validate.StrOpts{Max: 10}))
	c.add("expiry_date", f.Str("expiry_date", nullish, validate.StrOpts{Max: 10}))
	c.add("notes", f.Str("notes", nullish, validate.StrOpts{Max: 2000}))
	if err := formErr(f, ""); err != nil {
		return err
	}
	row, err := h.svc.CreateEmployeeDocument(r.Context(), c)
	if err != nil {
		return err
	}
	return reply(w, http.StatusCreated, dataMsg(row, "Dokumen berhasil disimpan"))
}

func (h handlers) deleteEmployeeDocument(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	if err := h.svc.DeleteEmployeeDocument(r.Context(), r.PathValue("doc_id")); err != nil {
		return err
	}
	return ok(w, msgOf("Dokumen berhasil dihapus"))
}

func (h handlers) patchEmployeeDocument(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	const message = "Data tidak valid"
	f, err := readForm(r, message)
	if err != nil {
		return err
	}
	var c fields
	addSent := func(col string, v opt[string]) {
		if v.Sent {
			c.add(col, v.Val)
		}
	}
	addSent("document_type", str(f, "document_type", optional, validate.StrOpts{Max: 120}))
	addSent("document_name", str(f, "document_name", optional, validate.StrOpts{Max: 255}))
	addSent("issue_date", str(f, "issue_date", nullish, validate.StrOpts{}))
	addSent("expiry_date", str(f, "expiry_date", nullish, validate.StrOpts{}))
	addSent("notes", str(f, "notes", nullish, validate.StrOpts{Max: 2000}))
	if err := formErr(f, message); err != nil {
		return err
	}
	row, err := h.svc.UpdateEmployeeDocument(r.Context(), r.PathValue("doc_id"), c)
	if err != nil {
		return err
	}
	return ok(w, dataMsg(row, "Dokumen berhasil diupdate"))
}

/* ── Employment history ──────────────────────────────────────────────── */

func (h handlers) listEmploymentHistory(w http.ResponseWriter, r *http.Request) error {
	employeeID := r.URL.Query().Get("employee_id")
	if employeeID == "" {
		return httpx.BadRequest("employee_id diperlukan")
	}
	if err := h.requireEmployeeAccess(r, employeeID, employeeRecordManagers); err != nil {
		return err
	}
	rows, err := h.svc.EmploymentHistory(r.Context(), employeeID)
	if err != nil {
		return err
	}
	u, err := h.auth.CurrentUser(r)
	if err != nil {
		return err
	}
	if see, err := h.canSeeSalary(r, u); err != nil {
		return err
	} else if !see {
		hideKeys(rows, historySalaryKeys)
	}
	return ok(w, dataOf(rows))
}

const historyRequired = "Field wajib: employee_id, change_type, effective_date"

// nullDefault is the employment history preprocess: "", 0 and a missing key
// all become null.
func nullDefault(f *validate.Form, keys ...string) {
	obj := f.Fields()
	for _, k := range keys {
		v, present := obj[k]
		switch x := v.(type) {
		case string:
			if x == "" {
				obj[k] = nil
			}
		case interface{ String() string }:
			if n := domain.JSNumber(x.String()); n == 0 {
				obj[k] = nil
			}
		}
		if !present {
			obj[k] = nil
		}
	}
}

// coerceAmount is z.coerce.number().nonnegative() on an already
// null-defaulted field.
func coerceAmount(f *validate.Form, key string) *float64 {
	v := f.Fields()[key]
	if v == nil {
		return nil
	}
	var n float64
	switch x := v.(type) {
	case string:
		n = domain.JSNumber(x)
	case bool:
		if x {
			n = 1
		}
	case interface{ String() string }:
		n = domain.JSNumber(x.String())
	default:
		n = domain.JSNumber("x")
	}
	if !domain.IsFinite(n) {
		f.Fail(key, "invalid_type", "Invalid input: expected number, received NaN")
		return nil
	}
	if n < 0 {
		f.Fail(key, "too_small", "Too small: expected number to be >=0")
		return nil
	}
	return &n
}

func (h handlers) createEmploymentHistory(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	optionalText := map[string]int{
		"prev_department_id": 64, "prev_section_id": 64, "prev_job_title_id": 64, "prev_employment_status": 50,
		"new_department_id": 64, "new_section_id": 64, "new_job_title_id": 64, "new_employment_status": 50,
		"reason": 2000, "notes": 5000,
	}
	if f.Fields() != nil {
		nullDefault(f, "prev_department_id", "prev_section_id", "prev_job_title_id", "prev_employment_status", "prev_salary",
			"new_department_id", "new_section_id", "new_job_title_id", "new_employment_status", "new_salary", "reason", "notes")
	}
	req := func(key string, max int) *string {
		return reqStr(f, key, historyRequired, validate.StrOpts{Max: max, Check: minLen(1, historyRequired)})
	}
	var c fields
	c.add("employee_id", req("employee_id", 64))
	c.add("change_type", req("change_type", 50))
	c.add("effective_date", req("effective_date", 10))
	for _, col := range []string{"prev_department_id", "prev_section_id", "prev_job_title_id", "prev_employment_status"} {
		c.add(col, f.Str(col, nullish, validate.StrOpts{Max: optionalText[col]}))
	}
	c.add("prev_salary", coerceAmount(f, "prev_salary"))
	for _, col := range []string{"new_department_id", "new_section_id", "new_job_title_id", "new_employment_status"} {
		c.add(col, f.Str(col, nullish, validate.StrOpts{Max: optionalText[col]}))
	}
	c.add("new_salary", coerceAmount(f, "new_salary"))
	c.add("reason", f.Str("reason", nullish, validate.StrOpts{Max: 2000}))
	c.add("notes", f.Str("notes", nullish, validate.StrOpts{Max: 5000}))
	if err := formErr(f, ""); err != nil {
		return err
	}
	row, err := h.svc.CreateEmploymentHistory(r.Context(), c)
	if err != nil {
		return err
	}
	return reply(w, http.StatusCreated, dataMsg(row, "Riwayat kerja berhasil disimpan"))
}

/* ── Profile tabs ────────────────────────────────────────────────────── */

func (h handlers) listDepartmentsBrief(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	rows, err := h.svc.Departments(r.Context())
	if err != nil {
		return err
	}
	return ok(w, dataOf(rows))
}

func (h handlers) employeeLifecycle(w http.ResponseWriter, r *http.Request) error {
	id, err := requireUUID(r.PathValue("id"), "ID karyawan tidak valid")
	if err != nil {
		return err
	}
	if err := h.requireEmployeeAccess(r, id, employeeRecordManagers); err != nil {
		return err
	}
	out, err := h.svc.Lifecycle(r.Context(), id)
	if err != nil {
		return err
	}
	return ok(w, dataOf(out))
}

func (h handlers) recruitmentDocuments(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	id, err := requireUUID(r.PathValue("id"), "ID karyawan tidak valid")
	if err != nil {
		return err
	}
	out, err := h.svc.RecruitmentDocuments(r.Context(), id)
	if err != nil {
		return err
	}
	return ok(w, dataOf(out))
}

/* ── Shift pattern ───────────────────────────────────────────────────── */

// shiftManager is authorizeShiftManager: HR menus pass with the user's
// name; otherwise the signed-in employee must be the direct manager.
func (h handlers) shiftManager(r *http.Request, targetID string) (string, error) {
	if u, err := h.auth.RequireMenuPrefix(r, slices.Concat(iam.HrisKepegawaian, iam.HrisWorkforce)...); err == nil {
		return u.FullName, nil
	} else if _, isAPI := err.(*httpx.Error); !isAPI {
		return "", err
	}
	actor, err := h.actor(r)
	if err != nil {
		return "", err
	}
	return h.svc.AuthorizeShiftManager(r.Context(), actor, targetID)
}

func (h handlers) employeeShifts(w http.ResponseWriter, r *http.Request) error {
	id, err := requireUUID(r.PathValue("id"), "ID karyawan tidak valid")
	if err != nil {
		return err
	}
	if _, err := h.shiftManager(r, id); err != nil {
		return err
	}
	rows, err := h.svc.EmployeeShifts(r.Context(), id)
	if err != nil {
		return err
	}
	return ok(w, dataOf(rows))
}

const (
	patternDateMessage = "Tanggal mulai berlaku wajib diisi (YYYY-MM-DD)"
	patternDaysMessage = "Pola jadwal harus lengkap 7 hari (Senin–Minggu)"
)

func (h handlers) saveEmployeeShifts(w http.ResponseWriter, r *http.Request) error {
	id, err := requireUUID(r.PathValue("id"), "ID karyawan tidak valid")
	if err != nil {
		return err
	}
	actorName, err := h.shiftManager(r, id)
	if err != nil {
		return err
	}
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	from := reqStr(f, "effective_from", patternDateMessage, validate.StrOpts{Check: matches(domain.DateRe, patternDateMessage)})
	var days []patternDay
	list := f.Fields()["days"]
	items, isList := list.([]any)
	if f.Fields() != nil && !isList {
		f.Fail("days", "invalid_type", patternDaysMessage)
	}
	if isList {
		f.List("days", optional, 1<<30, func(sub *validate.Form, i int, v any) {
			item := sub.Item(i, v)
			if item.Fields() == nil {
				return
			}
			d := patternDay{}
			raw, isNum := item.Fields()["day_of_week"].(interface{ String() string })
			n := domain.JSNumber("x")
			if isNum {
				n = domain.JSNumber(raw.String())
			}
			if !isNum || n != float64(int(n)) || n < 1 || n > 7 {
				item.Fail("day_of_week", "invalid_type", patternDaysMessage)
			}
			d.DayOfWeek = int(n)
			d.ShiftID = item.Str("shift_id", nullish, validate.StrOpts{Check: matches(domain.UUIDRe, "ID shift tidak valid")})
			days = append(days, d)
		})
		if len(items) != 7 {
			f.Fail("days", "invalid_type", patternDaysMessage)
		} else if f.Valid() {
			seen := map[int]bool{}
			for _, d := range days {
				seen[d.DayOfWeek] = true
			}
			if len(seen) != 7 {
				f.Fail("days", "custom", patternDaysMessage)
			}
		}
	}
	if err := formErr(f, ""); err != nil {
		return err
	}
	if err := h.svc.SaveShiftPattern(r.Context(), id, *from, days, actorName); err != nil {
		return err
	}
	return ok(w, msgOf("Jadwal shift disimpan — berlaku mulai "+*from))
}

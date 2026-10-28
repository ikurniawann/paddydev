package hris

import (
	"fmt"
	"net/http"
	"regexp"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/validate"
)

/* ── Shift master ────────────────────────────────────────────────────── */

var shiftTimeRe = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d(:[0-5]\d)?$`)

const (
	shiftNameRequired = "Nama shift wajib diisi"
	shiftStartInvalid = "Jam mulai tidak valid (HH:MM)"
	shiftEndInvalid   = "Jam selesai tidak valid (HH:MM)"
	shiftTolerance    = "Toleransi terlambat harus angka ≥ 0"
)

func (h handlers) listShifts(w http.ResponseWriter, r *http.Request, _ *Actor) error {
	rows, err := h.svc.Shifts(r.Context(), r.URL.Query().Get("with_usage") == "true")
	if err != nil {
		return err
	}
	return ok(w, dataOf(rows))
}

func (h handlers) createShift(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	var in shiftInput
	rawCode := f.Str("code", optional, validate.StrOpts{})
	in.Name = reqStr(f, "name", shiftNameRequired, validate.StrOpts{Trim: true, Check: minLen(1, shiftNameRequired)})
	in.StartTime = reqStr(f, "start_time", shiftStartInvalid, validate.StrOpts{Check: matches(shiftTimeRe, shiftStartInvalid)})
	in.EndTime = reqStr(f, "end_time", shiftEndInvalid, validate.StrOpts{Check: matches(shiftTimeRe, shiftEndInvalid)})
	in.BreakMinutes = f.Int("break_minutes", optional, validate.NumOpts{Min: validate.Bound(0)})
	if raw, present := rawValue(f, "late_tolerance_minutes"); present {
		n, isNum := raw.(fmt.Stringer)
		v := domain.JSNumber("x")
		if isNum {
			v = domain.JSNumber(n.String())
		}
		if !isNum || v != float64(int64(v)) || v < 0 {
			f.Fail("late_tolerance_minutes", "invalid_type", shiftTolerance)
		} else {
			iv := int(v)
			in.LateTolerance = &iv
		}
	}
	in.IsOvernight = f.Bool("is_overnight", optional)
	in.SortOrder = f.Int("sort_order", optional, validate.NumOpts{})
	if f.Valid() && !(in.IsOvernight != nil && *in.IsOvernight) && !(*in.EndTime > *in.StartTime) {
		f.Fail(nil, "custom", "Jam selesai harus setelah jam mulai — atau tandai sebagai shift malam (lewat tengah malam)")
	}
	if err := formErr(f, ""); err != nil {
		return err
	}
	// Kode dicek setelah field lain agar pesan lama (nama/jam) tetap duluan.
	if in.Code, err = normalizeShiftCode(rawCode); err != nil {
		return err
	}
	if in.Code == nil {
		return httpx.BadRequest(shiftCodeInvalid)
	}
	row, err := h.svc.CreateShift(r.Context(), in)
	if err != nil {
		return err
	}
	return reply(w, http.StatusCreated, dataMsg(row, `Shift "`+row.Str("name")+`" dibuat`))
}

func (h handlers) updateShift(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	id, err := requireUUID(r.PathValue("id"), "ID shift tidak valid")
	if err != nil {
		return err
	}
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	in := shiftInput{
		Code:          f.Str("code", optional, validate.StrOpts{}),
		Name:          f.Str("name", optional, validate.StrOpts{}),
		StartTime:     f.Str("start_time", optional, validate.StrOpts{Check: matches(shiftTimeRe, "Jam mulai tidak valid")}),
		EndTime:       f.Str("end_time", optional, validate.StrOpts{Check: matches(shiftTimeRe, "Jam selesai tidak valid")}),
		BreakMinutes:  f.Int("break_minutes", optional, validate.NumOpts{Min: validate.Bound(0)}),
		LateTolerance: f.Int("late_tolerance_minutes", optional, validate.NumOpts{Min: validate.Bound(0)}),
		IsOvernight:   f.Bool("is_overnight", optional),
		IsActive:      f.Bool("is_active", optional),
		SortOrder:     f.Int("sort_order", optional, validate.NumOpts{}),
	}
	if err := formErr(f, ""); err != nil {
		return err
	}
	if in.Code, err = normalizeShiftCode(in.Code); err != nil {
		return err
	}
	if err := h.svc.UpdateShift(r.Context(), id, in); err != nil {
		return err
	}
	return ok(w, msgOf("Shift diperbarui"))
}

func (h handlers) deleteShift(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	id, err := requireUUID(r.PathValue("id"), "ID shift tidak valid")
	if err != nil {
		return err
	}
	msg, err := h.svc.DeleteShift(r.Context(), id)
	if err != nil {
		return err
	}
	return ok(w, msgOf(msg))
}

/* ── Overtime ────────────────────────────────────────────────────────── */

var overtimeTimeRe = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

func (h handlers) listOvertime(w http.ResponseWriter, r *http.Request, a *Actor) error {
	f := queryForm(r)
	s := func(k string) *string { return f.Str(k, optional, validate.StrOpts{}) }
	q := overtimeQuery{Status: s("status"), Month: s("month"), Year: s("year"), EmployeeID: s("employee_id"), Scope: s("scope")}
	rows, err := h.svc.ListOvertime(r.Context(), a, q)
	if err != nil {
		return err
	}
	return ok(w, dataOf(rows))
}

func regexOf(re *regexp.Regexp) func(string) (string, string, bool) {
	return matches(re, "Invalid string: must match pattern /"+re.String()+"/")
}

func (h handlers) createOvertime(w http.ResponseWriter, r *http.Request, a *Actor) error {
	const message = "Validation failed"
	f, err := readForm(r, message)
	if err != nil {
		return err
	}
	var in overtimeInput
	in.EmployeeID = f.Str("employee_id", optional, validate.StrOpts{Check: func(s string) (string, string, bool) {
		return "invalid_union", "Invalid input", s == "me" || domain.UUIDRe.MatchString(s)
	}})
	in.Date = strOr(f.Str("date", validate.Rule{}, validate.StrOpts{Check: regexOf(domain.DateRe)}))
	in.StartTime = strOr(f.Str("start_time", validate.Rule{}, validate.StrOpts{Check: regexOf(overtimeTimeRe)}))
	in.EndTime = strOr(f.Str("end_time", validate.Rule{}, validate.StrOpts{Check: regexOf(overtimeTimeRe)}))
	in.Reason = strOr(f.Str("reason", optional, validate.StrOpts{Trim: true}))
	if b := f.Int("break_minutes", optional, validate.NumOpts{Min: validate.Bound(0), Max: validate.Bound(720)}); b != nil {
		in.BreakMinutes = *b
	}
	in.Attachments = f.Strings("attachments", optional, maxOvertimeAttachments+1, validate.StrOpts{})
	in.DayType = f.Enum("day_type", optional, overtimeDayTypes)
	if err := formErr(f, message); err != nil {
		return err
	}
	out, err := h.svc.CreateOvertime(r.Context(), a, in)
	if err != nil {
		return err
	}
	return ok(w, out)
}

func (h handlers) decideOvertime(w http.ResponseWriter, r *http.Request, a *Actor) error {
	const message = "Validation failed"
	f, err := readForm(r, message)
	if err != nil {
		return err
	}
	id := f.UUID("overtime_id", validate.Rule{})
	action := f.Enum("action", validate.Rule{}, []string{"approve", "reject", "cancel"})
	rejection := f.Str("rejection_reason", optional, validate.StrOpts{})
	dayType := f.Enum("day_type", optional, overtimeDayTypes)
	var times overtimeTimes
	times.Start = f.Str("start_time", optional, validate.StrOpts{Check: regexOf(overtimeTimeRe)})
	times.End = f.Str("end_time", optional, validate.StrOpts{Check: regexOf(overtimeTimeRe)})
	times.Break = f.Int("break_minutes", optional, validate.NumOpts{Min: validate.Bound(0), Max: validate.Bound(720)})
	if err := formErr(f, message); err != nil {
		return err
	}
	out, err := h.svc.DecideOvertime(r.Context(), a, *id, *action, rejection, dayType, times, auditMeta(r))
	if err != nil {
		return err
	}
	return ok(w, out)
}

/* ── Holidays ────────────────────────────────────────────────────────── */

func parseHoliday(f *validate.Form, withNote bool) domain.HolidayInput {
	s := func(k string, max int) *string { return f.Str(k, optional, validate.StrOpts{Max: max}) }
	in := domain.HolidayInput{
		HolidayDate:  s("holiday_date", 10),
		Name:         s("name", 200),
		Type:         s("type", 30),
		DeductsLeave: f.Bool("deducts_leave", optional),
		Status:       s("status", 10),
	}
	if withNote {
		note := str(f, "note", nullish, validate.StrOpts{Max: 2000})
		in.Note, in.NoteSent = note.Val, note.Sent
	}
	return in
}

func (h handlers) listHolidays(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	q := r.URL.Query()
	start, end, valid := domain.ResolveHolidayRange(get(q, "start_date"), get(q, "end_date"), q.Get("year"), domain.NowWIB(h.svc.now()))
	if !valid {
		return httpx.BadRequest("start_date dan end_date wajib berformat YYYY-MM-DD")
	}
	includeDraft := q.Get("include_draft") == "1" && hrRoles[u.Role]
	rows, err := h.svc.Holidays(r.Context(), start, end, includeDraft)
	if err != nil {
		return err
	}
	return ok(w, obj("data", rows, "meta", obj("start_date", start, "end_date", end)))
}

func (h handlers) createHoliday(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	in := parseHoliday(f, true)
	if err := formErr(f, ""); err != nil {
		return err
	}
	if msg := domain.ValidateHolidayBody(in); msg != "" {
		return httpx.BadRequest(msg)
	}
	row, err := h.svc.CreateHoliday(r.Context(), in, u.ID)
	if err != nil {
		return err
	}
	return reply(w, http.StatusCreated, dataMsg(row, `Libur "`+row.Str("name")+`" ditambahkan`))
}

func (h handlers) updateHoliday(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	id, err := requireUUID(r.PathValue("id"), "ID libur tidak valid")
	if err != nil {
		return err
	}
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	in := parseHoliday(f, true)
	if err := formErr(f, ""); err != nil {
		return err
	}
	if msg := domain.ValidateHolidayPatch(in); msg != "" {
		return httpx.BadRequest(msg)
	}
	row, err := h.svc.UpdateHoliday(r.Context(), id, in, u.ID)
	if err != nil {
		return err
	}
	return ok(w, msgOf(`Libur "`+row.Str("name")+`" diperbarui`))
}

func (h handlers) deleteHoliday(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	id, err := requireUUID(r.PathValue("id"), "ID libur tidak valid")
	if err != nil {
		return err
	}
	row, err := h.svc.DeleteHoliday(r.Context(), id, u.ID)
	if err != nil {
		return err
	}
	return ok(w, msgOf(`Libur "`+row.Str("name")+`" dihapus`))
}

func (h handlers) previewHolidayImport(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	year := domain.ResolveImportYear(r.URL.Query().Get("year"), domain.NowWIB(h.svc.now()))
	rows, err := h.svc.PreviewHolidayImport(r.Context(), year)
	if err != nil {
		return err
	}
	suggested, imported := 0, 0
	for _, row := range rows {
		if row.Bool("suggested") {
			suggested++
		}
		if row.Bool("already_imported") {
			imported++
		}
	}
	return ok(w, obj("data", rows, "meta", obj("year", year, "total", len(rows), "suggested", suggested, "already_imported", imported)))
}

func (h handlers) importHolidays(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	var items []domain.HolidayInput
	f.List("items", optional, 500, func(sub *validate.Form, i int, v any) {
		item := sub.Item(i, v)
		if item.Fields() == nil {
			return
		}
		in := parseHoliday(item, false)
		in.SourceRef = item.Str("source_ref", optional, validate.StrOpts{Max: 500})
		items = append(items, in)
	})
	if err := formErr(f, ""); err != nil {
		return err
	}
	if len(items) == 0 {
		return httpx.BadRequest("Tidak ada hari libur yang dicentang")
	}
	for _, item := range items {
		if msg := domain.ValidateImportItem(item); msg != "" {
			label := "(tanpa nama)"
			if item.Name != nil {
				label = *item.Name
			} else if item.HolidayDate != nil {
				label = *item.HolidayDate
			}
			return httpx.BadRequest(msg + ": " + label)
		}
	}
	created, updated, err := h.svc.ImportHolidays(r.Context(), items, u.ID)
	if err != nil {
		return err
	}
	message := fmt.Sprintf("%d hari libur ditambahkan", created)
	if updated > 0 {
		message += fmt.Sprintf(", %d diperbarui", updated)
	}
	return ok(w, obj("data", obj("created", created, "updated", updated), "message", message))
}

package hris

import (
	"errors"
	"mime"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/module"
	"nuhabit/backend/internal/platform/storage"
	"nuhabit/backend/internal/platform/validate"
)

// The HRIS routes that read or write files: clock-in/out selfies, the
// attendance export, leave attachments, announcement covers and contract
// documents (app/api/hris/**, formerly NEXT_ONLY_ROUTES).
func (h handlers) fileRoutes() []module.Route {
	kepegawaian := iam.HrisKepegawaian
	r := func(pattern string, handler http.Handler) module.Route {
		return module.Route{Pattern: pattern, Handler: handler}
	}
	return []module.Route{
		r("POST /api/hris/attendance", h.withActor(h.clockAttendance)),
		r("GET /api/hris/attendance/export", h.withMenu(iam.HrisWorkforce, h.exportAttendance)),
		r("GET /api/hris/attendance/photo/{path...}", h.withActor(h.attendancePhoto)),
		r("POST /api/hris/leaves/attachment", h.withActor(h.uploadLeaveAttachment)),
		r("GET /api/hris/leaves/attachment/{path...}", h.withActor(h.leaveAttachment)),
		r("GET /api/hris/attendance-requests/attachment/{path...}", h.withActor(h.attendanceRequestAttachment)),
		r("POST /api/hris/announcements/cover", h.withMenu(kepegawaian, h.uploadCover)),
		r("GET /api/hris/announcements/cover/{path...}", h.withActor(h.coverFile)),
		r("GET /api/hris/contracts/{id}/document", h.withMenu(kepegawaian, h.contractDocument)),
		r("GET /api/hris/contracts/{id}/signed-document", h.withMenu(kepegawaian, h.signedDocument)),
		r("POST /api/hris/contracts/{id}/signed-document", h.withMenu(kepegawaian, h.uploadSignedDocument)),
		r("DELETE /api/hris/contracts/{id}/signed-document", h.withDelete(delSignedDoc, h.withMenu(kepegawaian, h.deleteSignedDocument))),
	}
}

/* ── Attendance ──────────────────────────────────────────────────────── */

// POST /api/hris/attendance { action: "clock-in" | "clock-out", … }
func (h handlers) clockAttendance(w http.ResponseWriter, r *http.Request, a *Actor) error {
	const failed = "Validation failed"
	body, err := readUploadJSON(r, failed)
	if err != nil {
		return err
	}
	if err := formErr(body, failed); err != nil { // z.looseObject: an object
		return err
	}
	action, _ := rawValue(body, "action")
	f := validate.New(body.Fields(), true)
	switch action {
	case "clock-in":
		in := parseClockIn(f)
		if err := formErr(f, failed); err != nil {
			return err
		}
		row, existing, err := h.svc.ClockIn(r.Context(), a, in)
		if err != nil {
			return err
		}
		if existing != nil {
			return reply(w, http.StatusBadRequest, obj("success", false, "error", "Already clocked in today",
				"attendance_id", *existing))
		}
		return ok(w, obj("message", "Clock-in successful", "data", row))
	case "clock-out":
		in := parseClockOut(f)
		if err := formErr(f, failed); err != nil {
			return err
		}
		row, err := h.svc.ClockOut(r.Context(), a, in)
		if err != nil {
			return err
		}
		return ok(w, obj("message", "Clock-out successful", "data", row))
	}
	return httpx.BadRequest(`Invalid action. Use "clock-in" or "clock-out"`)
}

// GET /api/hris/attendance/export?format=csv|xlsx|pdf
func (h handlers) exportAttendance(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	q := r.URL.Query()
	file, err := h.svc.ExportAttendance(r.Context(), exportFilter{
		EmployeeID: q.Get("employee_id"), StartDate: q.Get("start_date"), EndDate: q.Get("end_date"), Status: q.Get("status"),
	}, q.Get("format"))
	if err != nil {
		return err
	}
	hd := w.Header()
	hd.Set("Content-Type", file.ContentType)
	hd.Set("Content-Disposition", `attachment; filename="`+file.FileName+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(file.Data)
	return nil
}

// GET /api/hris/attendance/photo/attendance/<employeeId>/<file>: HR sees
// every selfie, an employee only their own.
func (h handlers) attendancePhoto(w http.ResponseWriter, r *http.Request, a *Actor) error {
	return h.ownedFile(w, r, a, "/api/hris/attendance/photo/", "attendance")
}

/* ── Leave attachments ───────────────────────────────────────────────── */

const maxAttachmentBytes = 5 * 1024 * 1024

// POST /api/hris/leaves/attachment { photo, employee_id? }: HR may upload
// for another employee.
func (h handlers) uploadLeaveAttachment(w http.ResponseWriter, r *http.Request, a *Actor) error {
	f, err := readUploadJSON(r, "")
	if err != nil {
		return err
	}
	photo := f.Str("photo", optional, validate.StrOpts{})
	// The id becomes a folder name: a UUID cannot leave it.
	employeeID := f.Str("employee_id", optional, validate.StrOpts{Check: matches(domain.UUIDRe, "ID karyawan tidak valid")})
	if err := formErr(f, ""); err != nil {
		return err
	}
	owner := a.EmployeeID
	if a.IsHR && employeeID != nil && *employeeID != "" {
		owner = employeeID
	}
	if owner == nil {
		return httpx.Forbidden("Akun ini tidak terhubung ke data karyawan")
	}
	path, err := h.svc.saveImage(strOr(photo), maxAttachmentBytes, "leave-attachments/"+*owner,
		"Lampiran harus berupa gambar JPG/PNG/WebP", "Ukuran lampiran maksimal 5 MB")
	if err != nil {
		return err
	}
	return ok(w, dataMsg(obj("path", path), "Lampiran tersimpan"))
}

// GET /api/hris/leaves/attachment/leave-attachments/<employeeId>/<file>
// The employee's direct manager reads it too: Persetujuan Tim links the file
// from the queue (EPIC-063). A reporting_to pointing at oneself grants nothing.
func (h handlers) leaveAttachment(w http.ResponseWriter, r *http.Request, a *Actor) error {
	const prefix, folder = "/api/hris/leaves/attachment/", "leave-attachments"
	if !a.IsHR && a.EmployeeID != nil {
		if segments := privateSegments(r, prefix, folder, 3); segments != nil && segments[1] != *a.EmployeeID {
			reports, err := h.svc.repo.SubordinateIDs(r.Context(), *a.EmployeeID)
			if err != nil {
				return err
			}
			if slices.Contains(reports, segments[1]) {
				return h.svc.servePrivate(w, strings.Join(segments, "/"), "")
			}
		}
	}
	return h.ownedFile(w, r, a, prefix, folder)
}

// ownedFile serves <folder>/<employeeId>/<file>…: HR reads all, others only
// files under their own employee id.
func (h handlers) ownedFile(w http.ResponseWriter, r *http.Request, a *Actor, prefix, folder string) error {
	return h.ownedFileFor(w, r, a, a.IsHR, prefix, folder)
}

// ownedFileFor is ownedFile with the reading staff chosen by the caller
// (IsHR, or the narrower IsHRD).
func (h handlers) ownedFileFor(w http.ResponseWriter, r *http.Request, a *Actor, staff bool, prefix, folder string) error {
	segments := privateSegments(r, prefix, folder, 3)
	if segments == nil {
		return httpx.BadRequest("Path tidak valid")
	}
	if !staff && (a.EmployeeID == nil || *a.EmployeeID != segments[1]) {
		return httpx.Forbidden("Forbidden")
	}
	return h.svc.servePrivate(w, strings.Join(segments, "/"), "")
}

/* ── Announcement covers ─────────────────────────────────────────────── */

// POST /api/hris/announcements/cover { image }: a JPG/PNG/WebP up to 5 MB.
func (h handlers) uploadCover(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	const invalid = "Cover harus berupa gambar JPG/PNG/WebP"
	f, err := readUploadJSON(r, invalid)
	if err != nil {
		return err
	}
	image := f.Str("image", optional, validate.StrOpts{})
	if err := formErr(f, invalid); err != nil {
		return err
	}
	path, err := h.svc.saveImage(strOr(image), maxAttachmentBytes, "announcements", invalid, "Ukuran cover maksimal 5 MB")
	if err != nil {
		return err
	}
	return ok(w, dataOf(obj("path", path)))
}

// GET /api/hris/announcements/cover/announcements/<file>: any signed-in
// account (a cover holds no personal data).
func (h handlers) coverFile(w http.ResponseWriter, r *http.Request, _ *Actor) error {
	segments := privateSegments(r, "/api/hris/announcements/cover/", "announcements", 2)
	if segments == nil {
		return httpx.BadRequest("Path tidak valid")
	}
	return h.svc.servePrivate(w, strings.Join(segments, "/"), "")
}

/* ── Contract documents ──────────────────────────────────────────────── */

// GET /api/hris/contracts/{id}/document: the agreement PDF, generated on
// the fly (drafts print too); 30 per minute per account.
func (h handlers) contractDocument(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	id, err := requireUUID(r.PathValue("id"), "ID kontrak tidak valid")
	if err != nil {
		return err
	}
	if err := h.requireSalaryAccess(r, u); err != nil {
		return err
	}
	win, err := h.svc.limiter.Fixed(r.Context(), "contract_pdf_"+u.ID, 30, time.Minute, h.svc.now())
	if err != nil {
		return err
	}
	if !win.Allowed {
		return httpx.TooManyRequests("Terlalu banyak permintaan, coba lagi sebentar lagi")
	}
	pdf, name, err := h.svc.ContractPDF(r.Context(), id)
	if err != nil {
		return err
	}
	hd := w.Header()
	hd.Set("Content-Type", "application/pdf")
	hd.Set("Content-Length", strconv.Itoa(len(pdf)))
	hd.Set("Content-Disposition", `attachment; filename="`+name+`"`)
	hd.Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdf)
	return nil
}

func (h handlers) signedContract(r *http.Request) (*signedContract, error) {
	id, err := requireUUID(r.PathValue("id"), "ID kontrak tidak valid")
	if err != nil {
		return nil, err
	}
	return h.svc.repo.SignedDocumentContract(r.Context(), id)
}

// GET /api/hris/contracts/{id}/signed-document: the scan, inline.
func (h handlers) signedDocument(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	if err := h.requireSalaryAccess(r, u); err != nil {
		return err
	}
	c, err := h.signedContract(r)
	if err != nil {
		return err
	}
	if c == nil || c.SignedDocumentURL == nil || *c.SignedDocumentURL == "" {
		return httpx.NotFound("Kontrak ini belum punya dokumen bertanda tangan")
	}
	return h.svc.servePrivate(w, *c.SignedDocumentURL, `inline; filename="`+domain.SignedDocumentName(c.ContractNumber)+`"`)
}

// POST /api/hris/contracts/{id}/signed-document (multipart "file", PDF or
// image up to 10 MB): a re-upload replaces the previous scan.
func (h handlers) uploadSignedDocument(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	const tooLarge = "Ukuran dokumen maksimal 10 MB"
	c, err := h.signedContract(r)
	if err != nil {
		return err
	}
	if c == nil {
		return httpx.NotFound("Kontrak tidak ditemukan")
	}
	// The body may exceed the file by its multipart framing.
	form, err := storage.ReadForm(r, MaxSignedDocumentBytes+64<<10)
	var file *storage.File
	switch {
	case errors.Is(err, storage.ErrBodyTooLarge):
		return httpx.BadRequest(tooLarge)
	case err == nil:
		file = form.File("file")
	case !isURLEncoded(r): // formData() also reads urlencoded bodies, which hold no file
		return httpx.BadRequest("Form data tidak valid")
	}
	if file == nil {
		return httpx.BadRequest("File tidak ditemukan")
	}
	if file.Size() > MaxSignedDocumentBytes {
		return httpx.BadRequest(tooLarge)
	}
	if err := h.svc.SaveSignedDocument(r.Context(), c, file.Data); err != nil {
		return err
	}
	return ok(w, msgOf("Dokumen bertanda tangan kontrak "+c.ContractNumber+" tersimpan"))
}

func isURLEncoded(r *http.Request) bool {
	t, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return t == "application/x-www-form-urlencoded"
}

// DELETE /api/hris/contracts/{id}/signed-document
func (h handlers) deleteSignedDocument(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	c, err := h.signedContract(r)
	if err != nil {
		return err
	}
	if c == nil {
		return httpx.NotFound("Kontrak tidak ditemukan")
	}
	if err := h.svc.RemoveSignedDocument(r.Context(), c); err != nil {
		return err
	}
	return ok(w, msgOf("Dokumen bertanda tangan dihapus"))
}

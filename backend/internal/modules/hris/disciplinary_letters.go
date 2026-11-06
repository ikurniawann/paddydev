package hris

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/module"
	"nuhabit/backend/internal/platform/storage"
	"nuhabit/backend/internal/platform/validate"
)

// EPIC-094: Surat Teguran dan Surat Peringatan yang diterbitkan HR. Terbit
// → lonceng ke karyawan, penyetujunya (request_approver + request_manager)
// dan HR; batal → alasan wajib, baris tetap ada, tanpa lonceng.

const (
	letterFolder           = "disciplinary-letters"
	maxLetterBytes         = 5 * 1024 * 1024
	letterAttachmentPrefix = "/api/hris/disciplinary-letters/attachment/"
	letterKind             = "alert"
	letterSelfLink         = "/dashboard/me/absensi#surat"
	letterTeamLink         = "/dashboard/me/absensi-tim"
	letterHRLink           = "/dashboard/hris/surat-peringatan"
	maxLetterRows          = 500
)

// letterAttachmentRe is a stored letter attachment; group 1 is the employee.
var letterAttachmentRe = regexp.MustCompile(`^disciplinary-letters/([0-9a-f-]{36})/[0-9]+-[0-9a-f]{16}\.(pdf|jpg|png|webp)$`)

func (h handlers) disciplinaryRoutes() []module.Route {
	letters := iam.HrisDisciplinaryLetters
	r := func(pattern string, handler http.Handler) module.Route {
		return module.Route{Pattern: pattern, Handler: handler}
	}
	return []module.Route{
		r("GET /api/hris/disciplinary-letters", h.withMenu(letters, h.listLetters)),
		r("POST /api/hris/disciplinary-letters", h.withAction("create", letters, h.issueLetter)),
		r("POST /api/hris/disciplinary-letters/{id}/revoke", h.withAction("update", letters, h.revokeLetter)),
		r("GET /api/hris/disciplinary-letters/lateness", h.withMenu(letters, h.letterLateness)),
		r("POST /api/hris/disciplinary-letters/attachments", h.withAction("create", letters, h.uploadLetterAttachment)),
		r("GET /api/hris/disciplinary-letters/attachment/{path...}", h.withActor(h.letterAttachment)),
		r("GET /api/hris/me/disciplinary-letters", h.withActor(h.myLetters)),
	}
}

type letterFilter struct {
	ID, EmployeeID string
	IncludeRevoked bool
}

// letterRows lists letters by issued date, newest first.
func letterRows(ctx context.Context, q database.Querier, f letterFilter) ([]*Row, error) {
	rows, err := q.Query(ctx, `
		SELECT l.id::text, l.employee_id::text, e.full_name, e.nip, l.letter_type, l.issued_date::text,
		       l.notes, l.attachment_path, cu.full_name, l.created_at, l.revoked_at, ru.full_name, l.revoke_reason
		FROM hris.disciplinary_letters l
		JOIN hris.employees e ON e.id = l.employee_id
		LEFT JOIN configuration.users cu ON cu.id = l.created_by
		LEFT JOIN configuration.users ru ON ru.id = l.revoked_by
		WHERE ($1 = '' OR l.id = $1::uuid) AND ($2 = '' OR l.employee_id = $2::uuid)
		  AND ($3 OR l.revoked_at IS NULL)
		ORDER BY l.issued_date DESC, l.created_at DESC, l.id
		LIMIT $4`, f.ID, f.EmployeeID, f.IncludeRevoked, maxLetterRows)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (*Row, error) {
		var id, emp, name, typ, issued string
		var nip, notes, attachment, createdBy, revokedBy, reason *string
		var createdAt time.Time
		var revokedAt *time.Time
		if err := r.Scan(&id, &emp, &name, &nip, &typ, &issued, &notes, &attachment, &createdBy, &createdAt,
			&revokedAt, &revokedBy, &reason); err != nil {
			return nil, err
		}
		return obj("id", id, "employee_id", emp, "employee_name", name, "nip", nip, "letter_type", typ,
			"label", domain.DisciplinaryLabel(typ), "issued_date", issued, "notes", notes,
			"attachment_path", attachment, "created_by_name", createdBy, "created_at", createdAt,
			"revoked_at", revokedAt, "revoked_by_name", revokedBy, "revoke_reason", reason), nil
	})
}

// GET /api/hris/disciplinary-letters?employee_id=&include_revoked=1
func (h handlers) listLetters(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	q := r.URL.Query()
	emp := strings.ToLower(strings.TrimSpace(q.Get("employee_id")))
	if emp != "" {
		if _, err := requireUUID(emp, "ID karyawan tidak valid"); err != nil {
			return err
		}
	}
	rows, err := letterRows(r.Context(), h.svc.repo.querier(), letterFilter{EmployeeID: emp, IncludeRevoked: q.Get("include_revoked") == "1"})
	if err != nil {
		return err
	}
	return ok(w, dataOf(rows))
}

// GET /api/hris/me/disciplinary-letters: one's own letters, revoked ones left out.
func (h handlers) myLetters(w http.ResponseWriter, r *http.Request, a *Actor) error {
	if a.EmployeeID == nil {
		return ok(w, dataOf([]*Row{}))
	}
	rows, err := letterRows(r.Context(), h.svc.repo.querier(), letterFilter{EmployeeID: *a.EmployeeID})
	if err != nil {
		return err
	}
	return ok(w, dataOf(rows))
}

// letterDate is dd-mm-yyyy for the bell text.
func letterDate(iso string) string {
	if t, ok := domain.ParseDate(iso); ok {
		return t.Format("02-01-2006")
	}
	return iso
}

// POST /api/hris/disciplinary-letters {employee_id, letter_type, issued_date, notes?, attachment_path?}
func (h handlers) issueLetter(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	emp := f.UUID("employee_id", validate.Rule{})
	typ := f.Enum("letter_type", validate.Rule{}, domain.DisciplinaryCodes)
	issued := f.Str("issued_date", validate.Rule{}, dateOpts)
	notes := f.Str("notes", nullish, validate.StrOpts{Trim: true, Max: 2000})
	attachment := f.Str("attachment_path", nullish, validate.StrOpts{Trim: true, Max: 300})
	if err := formErr(f, ""); err != nil {
		return err
	}
	employeeID := strings.ToLower(*emp)
	if *issued > domain.TodayWIB(h.svc.now()) {
		return httpx.BadRequest("Tanggal terbit tidak boleh di masa depan")
	}
	path := nilIfEmpty(attachment)
	if path != nil {
		m := letterAttachmentRe.FindStringSubmatch(*path)
		if m == nil || m[1] != employeeID {
			return httpx.BadRequest("Lampiran surat tidak valid — unggah ulang filenya")
		}
		if _, _, err := h.svc.files.ReadPrivate(*path); err != nil {
			return httpx.BadRequest("Lampiran surat tidak valid — unggah ulang filenya")
		}
	}
	label := domain.DisciplinaryLabel(*typ)
	ctx := r.Context()
	var id string
	err = h.svc.repo.InTx(ctx, func(repo Repository) error {
		q := repo.querier()
		var name string
		var active bool
		err := q.QueryRow(ctx, `SELECT full_name, is_active FROM hris.employees WHERE id = $1`, employeeID).Scan(&name, &active)
		if database.IsNoRows(err) {
			return httpx.NotFound("Karyawan tidak ditemukan")
		}
		if err != nil {
			return err
		}
		if !active {
			return httpx.BadRequest("Karyawan tidak aktif")
		}
		if err := q.QueryRow(ctx, `INSERT INTO hris.disciplinary_letters
			(employee_id, letter_type, issued_date, notes, attachment_path, created_by)
			VALUES ($1, $2, $3::date, $4, $5, $6) RETURNING id::text`,
			employeeID, *typ, *issued, nilIfEmpty(notes), path, u.ID).Scan(&id); err != nil {
			return err
		}
		after := map[string]any{"id": id, "letter_type": *typ, "issued_date": *issued, "notes": nilIfEmpty(notes), "attachment_path": path}
		if err := writeEmployeeAudit(ctx, q, u, "disciplinary_letter.issue", employeeID, name, nil, after); err != nil {
			return err
		}
		date := letterDate(*issued)
		if err := repo.NotifyEmployee(ctx, employeeID, letterKind, label+" dari HR",
			"HR menerbitkan "+label+" untuk Anda tanggal "+date+".", letterSelfLink); err != nil {
			return err
		}
		teamTitle := label + " untuk " + name
		teamMsg := "HR menerbitkan " + label + " untuk " + name + " tanggal " + date + "."
		if _, err := repo.NotifyRequestApprovers(ctx, employeeID, letterKind, teamTitle, teamMsg, letterTeamLink); err != nil {
			return err
		}
		return repo.NotifyHR(ctx, letterKind, teamTitle, teamMsg, letterHRLink)
	})
	if err != nil {
		return err
	}
	rows, err := letterRows(ctx, h.svc.repo.querier(), letterFilter{ID: id, IncludeRevoked: true})
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return httpx.NotFound("Surat tidak ditemukan")
	}
	return reply(w, http.StatusCreated, dataMsg(rows[0], label+" diterbitkan"))
}

// POST /api/hris/disciplinary-letters/{id}/revoke {reason}
func (h handlers) revokeLetter(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	id, err := requireUUID(strings.ToLower(r.PathValue("id")), "ID surat tidak valid")
	if err != nil {
		return err
	}
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	reason := strings.TrimSpace(strOr(f.Str("reason", nullish, validate.StrOpts{Max: 1000})))
	if err := formErr(f, ""); err != nil {
		return err
	}
	if reason == "" {
		return httpx.BadRequest("Alasan pembatalan wajib diisi")
	}
	ctx := r.Context()
	err = h.svc.repo.InTx(ctx, func(repo Repository) error {
		q := repo.querier()
		var emp, name, typ, issued string
		var revoked *time.Time
		err := q.QueryRow(ctx, `SELECT l.employee_id::text, e.full_name, l.letter_type, l.issued_date::text, l.revoked_at
			FROM hris.disciplinary_letters l JOIN hris.employees e ON e.id = l.employee_id
			WHERE l.id = $1 FOR UPDATE OF l`, id).Scan(&emp, &name, &typ, &issued, &revoked)
		if database.IsNoRows(err) {
			return httpx.NotFound("Surat tidak ditemukan")
		}
		if err != nil {
			return err
		}
		if revoked != nil {
			return httpx.BadRequest("Surat sudah dibatalkan")
		}
		if _, err := q.Exec(ctx, `UPDATE hris.disciplinary_letters
			SET revoked_at = now(), revoked_by = $2, revoke_reason = $3 WHERE id = $1`, id, u.ID, reason); err != nil {
			return err
		}
		before := map[string]any{"id": id, "letter_type": typ, "issued_date": issued}
		after := map[string]any{"id": id, "revoke_reason": reason}
		return writeEmployeeAudit(ctx, q, u, "disciplinary_letter.revoke", emp, name, before, after)
	})
	if err != nil {
		return err
	}
	rows, err := letterRows(ctx, h.svc.repo.querier(), letterFilter{ID: id, IncludeRevoked: true})
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return httpx.NotFound("Surat tidak ditemukan")
	}
	return ok(w, dataMsg(rows[0], "Surat dibatalkan"))
}

// GET /api/hris/disciplinary-letters/lateness?employee_id=: late days of the
// running attendance period and the matching rule, a hint for HR (R11).
func (h handlers) letterLateness(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	id, err := requireUUID(strings.ToLower(strings.TrimSpace(r.URL.Query().Get("employee_id"))), "ID karyawan tidak valid")
	if err != nil {
		return err
	}
	ctx := r.Context()
	q := h.svc.repo.querier()
	today := domain.TodayWIB(h.svc.now())
	p, err := periodContaining(ctx, q, today, today)
	if err != nil {
		return err
	}
	days, err := lateDays(ctx, q, []string{id}, p.Start, p.End)
	if err != nil {
		return err
	}
	level := domain.LatenessLevelFor(len(days))
	return ok(w, dataOf(obj("count", len(days), "level", level.Code, "label", level.Label,
		"period_start", p.Start, "period_end", p.End)))
}

// POST /api/hris/disciplinary-letters/attachments (multipart "file", "employee_id"):
// one PDF/JPG/PNG/WebP of at most 5 MB for a letter of that employee.
func (h handlers) uploadLetterAttachment(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	const tooLarge = "Ukuran lampiran maksimal 5 MB"
	form, err := storage.ReadForm(r, maxLetterBytes+64<<10)
	switch {
	case errors.Is(err, storage.ErrBodyTooLarge):
		return httpx.BadRequest(tooLarge)
	case err != nil:
		return httpx.BadRequest("Form data tidak valid")
	}
	emp, err := requireUUID(strings.ToLower(strings.TrimSpace(form.Value("employee_id"))), "ID karyawan tidak valid")
	if err != nil {
		return err
	}
	var exists bool
	if err := h.svc.repo.querier().QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM hris.employees WHERE id = $1)`, emp).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return httpx.NotFound("Karyawan tidak ditemukan")
	}
	file := form.File("file")
	if file == nil {
		return httpx.BadRequest("File tidak ditemukan")
	}
	if file.Size() > maxLetterBytes {
		return httpx.BadRequest(tooLarge)
	}
	path, err := h.svc.files.SavePrivateDocument(file.Data, letterFolder+"/"+emp)
	if err != nil {
		return httpx.BadRequest(err.Error())
	}
	return ok(w, dataMsg(obj("path", path), "Lampiran tersimpan"))
}

// GET /api/hris/disciplinary-letters/attachment/disciplinary-letters/<employeeId>/<file>:
// holders of the Surat Peringatan menu; the employee, their approver and
// manager while the letter is not revoked (R13).
func (h handlers) letterAttachment(w http.ResponseWriter, r *http.Request, a *Actor) error {
	segments := privateSegments(r, letterAttachmentPrefix, letterFolder, 3)
	rel := strings.Join(segments, "/")
	if segments == nil || !letterAttachmentRe.MatchString(rel) {
		return httpx.BadRequest("Path tidak valid")
	}
	codes := make([]string, 0, len(a.grants))
	for code := range a.grants {
		codes = append(codes, code)
	}
	allowed := iam.HasAnyMenuPrefix(codes, iam.HrisDisciplinaryLetters)
	if !allowed && a.EmployeeID != nil {
		if err := h.svc.repo.querier().QueryRow(r.Context(), `SELECT EXISTS (
			SELECT 1 FROM hris.disciplinary_letters l
			WHERE l.attachment_path = $2 AND l.revoked_at IS NULL
			  AND (l.employee_id = $1::uuid OR hris.request_approver(l.employee_id) = $1::uuid
			       OR hris.request_manager(l.employee_id) = $1::uuid))`, *a.EmployeeID, rel).Scan(&allowed); err != nil {
			return err
		}
	}
	if !allowed {
		return httpx.Forbidden("Forbidden")
	}
	return h.svc.servePrivate(w, rel, "")
}

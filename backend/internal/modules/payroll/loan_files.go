package payroll

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"nuhabit/backend/internal/modules/payroll/domain"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/storage"
)

// Loan attachments (EPIC-076 R8–R9): supporting documents (required for a
// kasbon) and collateral (surat berharga, required for a Pinjaman Karyawan),
// stored privately under loan-attachments/<employeeId>/.

const (
	loanAttachmentFolder   = "loan-attachments"
	maxLoanAttachments     = 3
	maxLoanAttachmentBytes = 5 * 1024 * 1024
	loanAttachmentPrefix   = "/api/hris/loans/attachment/"
)

// loanAttachmentRe is a stored loan attachment path; group 1 is the owner.
var loanAttachmentRe = regexp.MustCompile(`^loan-attachments/([0-9a-f-]{36})/[0-9]+-[0-9a-f]{16}\.(pdf|jpg|png|webp)$`)

// checkLoanAttachments: at most 3 distinct paths uploaded for ownerID and
// still on disk; at least one when required (emptyMsg is the error then).
func (s *service) checkLoanAttachments(ownerID string, paths []string, required bool, emptyMsg string) error {
	if len(paths) == 0 {
		if required {
			return httpx.BadRequest(emptyMsg)
		}
		return nil
	}
	if len(paths) > maxLoanAttachments {
		return httpx.BadRequest("Maksimal 3 file per lampiran")
	}
	const invalid = "Lampiran pinjaman tidak valid — unggah ulang filenya"
	seen := make(map[string]bool, len(paths))
	for _, p := range paths {
		if seen[p] {
			return httpx.BadRequest("Lampiran tidak boleh sama")
		}
		seen[p] = true
		m := loanAttachmentRe.FindStringSubmatch(p)
		if m == nil || m[1] != ownerID {
			return httpx.BadRequest(invalid)
		}
		if _, _, err := s.files.ReadPrivate(p); err != nil {
			return httpx.BadRequest(invalid)
		}
	}
	return nil
}

// isLoanApproverOf: employeeID is the stored manager approver, or the
// direct atasan of the applicant (EPIC-091: a Supervisor follows the request
// in Persetujuan Tim → Riwayat), of a loan that holds rel as a supporting
// document or collateral.
func (s *service) isLoanApproverOf(ctx context.Context, employeeID, rel string) (bool, error) {
	var ok bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM hris.loans l JOIN hris.employees e ON e.id = l.employee_id
		WHERE ($2 = ANY(l.attachments) OR $2 = ANY(l.collateral_attachments))
		  AND (l.manager_approver_id::text = $1 OR l.supervisor_approver_id::text = $1
		       OR (e.reporting_to::text = $1 AND e.id::text <> $1)
		       OR hris.request_approver(e.id)::text = $1))`, employeeID, rel).Scan(&ok)
	return ok, err
}

// POST /api/hris/loans/attachments (multipart "file", optional
// "employee_id"): one PDF/JPG/PNG/WebP of at most 5 MB. Loan managers
// (HR, finance) may upload for another employee.
func (h *handler) uploadLoanAttachment(w http.ResponseWriter, r *http.Request) error {
	const tooLarge = "Ukuran lampiran maksimal 5 MB"
	a, err := h.requireActor(r)
	if err != nil {
		return err
	}
	form, err := storage.ReadForm(r, maxLoanAttachmentBytes+64<<10)
	switch {
	case errors.Is(err, storage.ErrBodyTooLarge):
		return httpx.BadRequest(tooLarge)
	case err != nil:
		return httpx.BadRequest("Form data tidak valid")
	}
	owner := ""
	if a.EmployeeID != nil {
		owner = *a.EmployeeID
	}
	if id := strings.ToLower(strings.TrimSpace(form.Value("employee_id"))); id != "" && id != "me" && domain.HasLoanFullAccess(a.Role) {
		if _, err := requireUUID(id, "ID karyawan tidak valid"); err != nil {
			return err
		}
		owner = id
	}
	if owner == "" {
		return httpx.Forbidden("Akun ini tidak terhubung ke data karyawan")
	}
	file := form.File("file")
	if file == nil {
		return httpx.BadRequest("File tidak ditemukan")
	}
	if file.Size() > maxLoanAttachmentBytes {
		return httpx.BadRequest(tooLarge)
	}
	path, err := h.svc.files.SavePrivateDocument(file.Data, loanAttachmentFolder+"/"+owner)
	if err != nil {
		return httpx.BadRequest(err.Error())
	}
	return writeJSON(w, messageData{Data: map[string]string{"path": path}, Message: "Lampiran tersimpan"})
}

// GET /api/hris/loans/attachment/loan-attachments/<employeeId>/<file>: the
// owner, the loan's stored manager approver, loan managers and holders of
// "approve" on Pinjaman.
func (h *handler) loanAttachment(w http.ResponseWriter, r *http.Request) error {
	a, err := h.requireActor(r)
	if err != nil {
		return err
	}
	rest, _ := strings.CutPrefix(r.URL.EscapedPath(), loanAttachmentPrefix)
	raw := strings.Split(rest, "/")
	for i, seg := range raw {
		if raw[i], err = storage.DecodeURIComponent(seg); err != nil {
			return httpx.BadRequest("Path tidak valid")
		}
	}
	segments := storage.SafeSegmentsUnder(raw, loanAttachmentFolder, 3)
	rel := strings.Join(segments, "/")
	m := loanAttachmentRe.FindStringSubmatch(rel)
	if segments == nil || m == nil {
		return httpx.BadRequest("Path tidak valid")
	}
	allowed := domain.HasLoanFullAccess(a.Role) || (a.EmployeeID != nil && *a.EmployeeID == m[1])
	if !allowed && a.EmployeeID != nil {
		if allowed, err = h.svc.isLoanApproverOf(r.Context(), *a.EmployeeID, rel); err != nil {
			return err
		}
	}
	if !allowed {
		u, err := h.auth.CurrentUser(r)
		if err != nil {
			return err
		}
		if u != nil {
			if allowed, err = h.auth.HasMenuAction(r.Context(), u, "approve", iam.HrisLoans...); err != nil {
				return err
			}
		}
	}
	if !allowed {
		return httpx.Forbidden("Forbidden")
	}
	data, mime, err := h.svc.files.ReadPrivate(rel)
	if err != nil {
		return httpx.NotFound("File tidak ditemukan")
	}
	storage.WritePrivateFile(w, data, mime, "private, max-age=3600", "")
	return nil
}

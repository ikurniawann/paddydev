package hris

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/validate"
)

func (h handlers) listEmployeeContracts(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	id, err := requireUUID(r.PathValue("id"), "ID karyawan tidak valid")
	if err != nil {
		return err
	}
	rows, err := h.svc.EmployeeContracts(r.Context(), id)
	if err != nil {
		return err
	}
	see, err := h.canSeeSalary(r, u)
	if err != nil {
		return err
	}
	if !see {
		hideKeys(rows, contractSalaryKeys)
	}
	return ok(w, obj("data", rows, "can_view_salary", see))
}

// zodGUID is z.guid(): any 8-4-4-4-12 hex string.
var zodGUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

const contractTypeMessage = "Tipe kontrak harus 'pkwt' atau 'pkwtt'"

func (h handlers) createEmployeeContract(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	id, err := requireUUID(r.PathValue("id"), "ID karyawan tidak valid")
	if err != nil {
		return err
	}
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	text := func(key string, max int) *string { return f.Str(key, nullish, validate.StrOpts{Max: max}) }
	in := draftContract{EmployeeID: id, CreatedBy: &u.FullName}
	if t := reqStr(f, "contract_type", contractTypeMessage, enumOf([]string{"pkwt", "pkwtt"}, contractTypeMessage)); t != nil {
		in.ContractType = *t
	}
	const startMessage = "Tanggal mulai wajib diisi"
	if s := reqStr(f, "start_date", startMessage, validate.StrOpts{Max: 10, Check: minLen(1, startMessage)}); s != nil {
		in.StartDate = *s
	}
	in.EndDate = text("end_date", 10)
	in.ProbationEndDate = text("probation_end_date", 10)
	in.ParentContractID = f.Str("parent_contract_id", nullish, validate.StrOpts{Check: func(s string) (string, string, bool) {
		return "invalid_format", "Invalid GUID", zodGUID.MatchString(s)
	}})
	in.PositionTitle = text("position_title", 255)
	in.DepartmentName = text("department_name", 255)
	in.WorkLocation = text("work_location", 255)
	if n := f.Num("base_salary", nullish, validate.NumOpts{Min: validate.Bound(0)}); n != nil {
		in.BaseSalary = *n
	}
	in.Notes = text("notes", 5000)
	if err := formErr(f, ""); err != nil {
		return err
	}
	see, err := h.canSeeSalary(r, u)
	if err != nil {
		return err
	}
	if !see {
		in.BaseSalary = nil // the active salary is used (EPIC-057 R4)
	}
	row, number, err := h.svc.CreateEmployeeContract(r.Context(), in)
	if err != nil {
		return err
	}
	if !see {
		hideKeys([]*Row{row}, contractSalaryKeys)
	}
	return reply(w, http.StatusCreated, dataMsg(row, "Draft kontrak "+number+" dibuat"))
}

func (h handlers) listContracts(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	p := domain.ParseContractListParams(queryOf(r))
	rows, total, err := h.svc.ListContracts(r.Context(), p)
	if err != nil {
		return err
	}
	see, err := h.canSeeSalary(r, u)
	if err != nil {
		return err
	}
	if !see {
		hideKeys(rows, contractSalaryKeys)
	}
	return ok(w, obj("data", rows, "total", total, "page", p.Page, "limit", p.Limit))
}

func (h handlers) expiringContracts(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	raw, present := queryOf(r)("days")
	out, err := h.svc.ExpiringContracts(r.Context(), domain.ClampExpiringDays(raw, present))
	if err != nil {
		return err
	}
	return ok(w, dataOf(out))
}

var (
	contractActions  = []string{"activate", "end", "terminate", "convert", "renew", "edit", "update"}
	signedDocumentRe = regexp.MustCompile(`^contract-signed/[\w-]+/[\w.-]+$`)
)

func parseContractAction(f *validate.Form) contractAction {
	date := func(key string) opt[string] { return str(f, key, nullish, validate.StrOpts{Max: 10}) }
	text := func(key string, max int) opt[string] { return str(f, key, nullish, validate.StrOpts{Max: max}) }
	var in contractAction
	if a := reqStr(f, "action", "Aksi tidak dikenal", enumOf(contractActions, "Aksi tidak dikenal")); a != nil {
		in.Action = *a
	}
	in.EndDate = date("end_date")
	in.Reason = f.Str("reason", optional, validate.StrOpts{Max: 2000})
	in.SignedAt = date("signed_at")
	in.SignedDocumentURL = f.Str("signed_document_url", optional, validate.StrOpts{Check: func(s string) (string, string, bool) {
		if !signedDocumentRe.MatchString(s) {
			return "invalid_format", "Path dokumen tidak valid", false
		}
		return "custom", "Path dokumen tidak valid", !strings.Contains(s, "..")
	}})
	in.KemnakerRegisteredAt = date("kemnaker_registered_at")
	in.CompensationPaidAt = date("compensation_paid_at")
	in.Notes = text("notes", 5000)
	in.StartDate = f.Str("start_date", optional, validate.StrOpts{Max: 10})
	in.ProbationEndDate = date("probation_end_date")
	in.PositionTitle = text("position_title", 255)
	in.WorkLocation = text("work_location", 255)
	if raw, present := rawValue(f, "base_salary"); present {
		in.BaseSalarySent = true
		switch v := raw.(type) {
		case nil:
		case json.Number:
			n, _ := v.Float64()
			in.BaseSalary = n
		case string:
			if validate.UTF16Len(v) > 30 {
				f.Fail("base_salary", "invalid_union", "Invalid input")
			}
			in.BaseSalary = v
		default:
			f.Fail("base_salary", "invalid_union", "Invalid input")
		}
	}
	return in
}

func (h handlers) patchContract(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	id, err := requireUUID(r.PathValue("id"), "ID kontrak tidak valid")
	if err != nil {
		return err
	}
	contract, err := h.svc.LoadContract(r.Context(), id)
	if err != nil {
		return err
	}
	if contract == nil {
		return httpx.NotFound("Kontrak tidak ditemukan")
	}
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	in := parseContractAction(f)
	if err := formErr(f, ""); err != nil {
		return err
	}
	see, err := h.canSeeSalary(r, u)
	if err != nil {
		return err
	}
	if !see {
		in.BaseSalary, in.BaseSalarySent = nil, false // edit keeps the salary (EPIC-057 R4)
	}
	result, err := h.svc.RunContractAction(r.Context(), contract, in, u.ID, u.FullName)
	if err != nil {
		return err
	}
	status := http.StatusOK
	if in.Action == "renew" {
		status = http.StatusCreated
	}
	body := result.body()
	if !see {
		body.Del("compensation_amount")
	}
	return reply(w, status, body)
}

func (h handlers) deleteContract(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	id, err := requireUUID(r.PathValue("id"), "ID kontrak tidak valid")
	if err != nil {
		return err
	}
	if err := h.svc.DeleteDraftContract(r.Context(), id); err != nil {
		return err
	}
	return ok(w, msgOf("Draft kontrak dihapus"))
}

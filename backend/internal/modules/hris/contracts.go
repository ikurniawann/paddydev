package hris

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/httpx"
)

// Employment contracts: lib/hris/create-contract, contract-number,
// contracts-lifecycle, contracts-repo, employees-profile.
//
// The TS lifecycle receives date columns as JS Date objects, which breaks
// its string comparisons (a PKWTT with probation always activates as
// "permanent") and makes renewal throw (500). These rules read the dates as
// YYYY-MM-DD here, so activation and renewal behave as the tests intend.

// ContractRepo is the contracts storage.
type ContractRepo interface {
	EmployeeContracts(ctx context.Context, employeeID string) ([]*Row, error)
	ContractByID(ctx context.Context, id string) (*Row, error)
	ContractSnapshot(ctx context.Context, employeeID string) (*contractSnapshot, error)
	PkwtChain(ctx context.Context, employeeID string) ([]domain.ContractPeriod, error)
	// MaxContractSeq is the highest manual number "NNNN/TYPE/<month>/<year>"
	// of the type in year (0 when none); imported ODOO-… numbers are ignored.
	MaxContractSeq(ctx context.Context, contractType string, year int) (int, error)
	InsertContract(ctx context.Context, f fields) (*Row, error)
	LoadContract(ctx context.Context, id string) (*contractRow, error)
	ActiveContractNumber(ctx context.Context, employeeID, exceptID string) (*string, error)
	EmploymentStatusOf(ctx context.Context, employeeID string) (*string, error)
	ActivateContract(ctx context.Context, c *contractRow, signedAt *string, newStatus string, prevStatus, recorderID *string) error
	SetContractStatus(ctx context.Context, id, status string, f fields) error
	UpdateDraftContract(ctx context.Context, id string, f fields) (bool, error)
	UpdateContractMeta(ctx context.Context, id string, signedAt, documentURL, kemnaker, paidAt, notes *string) error
	DeleteDraftContract(ctx context.Context, id string) (bool, error)
	ListContracts(ctx context.Context, p domain.ContractListParams) ([]*Row, int64, error)
	ExpiringContracts(ctx context.Context, days int) ([]*Row, error)
	EndingProbations(ctx context.Context, days int) ([]*Row, error)
	EmployeesWithoutContract(ctx context.Context, superAdmins []string) ([]*Row, error)
}

type contractSnapshot struct {
	PositionTitle  *string
	DepartmentName *string
}

// contractRow is ContractRow with dates as YYYY-MM-DD.
type contractRow struct {
	ID, EmployeeID, ContractNumber, ContractType, Status, StartDate string
	EndDate, ProbationEndDate, BaseSalary                           *string
	PositionTitle, DepartmentName, WorkLocation, Notes              *string
	SignedAt, KemnakerRegisteredAt, CompensationPaidAt              *string
	Sequence                                                        int
}

// draftContract is CreateDraftContractInput.
type draftContract struct {
	EmployeeID, ContractType, StartDate                           string
	EndDate, ProbationEndDate, ParentContractID                   *string
	PositionTitle, DepartmentName, WorkLocation, Notes, CreatedBy *string
	BaseSalary                                                    any // number, numeric text or nil
}

// CreateDraftContract is createDraftContract: compliance checks, defaults
// from the employee record and active salary, numbered insert. It returns
// the new contract id and number.
func (s *Service) CreateDraftContract(ctx context.Context, in draftContract) (string, string, error) {
	if errs := domain.ValidateContractDates(domain.ContractDates{
		ContractType: in.ContractType, StartDate: in.StartDate, EndDate: in.EndDate, ProbationEndDate: in.ProbationEndDate,
	}); len(errs) > 0 {
		return "", "", httpx.BadRequest(strings.Join(errs, " "))
	}
	snap, err := s.repo.ContractSnapshot(ctx, in.EmployeeID)
	if err != nil {
		return "", "", err
	}
	if snap == nil {
		return "", "", httpx.NotFound("Karyawan tidak ditemukan")
	}
	salary, err := s.salary.ActiveBaseSalary(ctx, s.repo.querier(), in.EmployeeID)
	if err != nil {
		return "", "", err
	}

	sequence := 1
	if in.ContractType == "pkwt" {
		chain, err := s.repo.PkwtChain(ctx, in.EmployeeID)
		if err != nil {
			return "", "", err
		}
		end := in.StartDate
		if in.EndDate != nil {
			end = *in.EndDate
		}
		if msg := domain.ValidatePkwtTotal(domain.PkwtChainTotalMonths(chain), domain.MonthsWorked(in.StartDate, end)); msg != "" {
			return "", "", httpx.Status(422, msg)
		}
		sequence = len(chain) + 1
	}

	var f fields
	f.add("employee_id", in.EmployeeID)
	f.add("contract_number", nil)
	f.add("contract_type", in.ContractType)
	f.add("start_date", in.StartDate)
	f.add("end_date", in.EndDate)
	f.add("probation_end_date", in.ProbationEndDate)
	f.add("parent_contract_id", in.ParentContractID)
	f.add("sequence", sequence)
	f.add("position_title", firstOf(in.PositionTitle, snap.PositionTitle))
	f.add("department_name", firstOf(in.DepartmentName, snap.DepartmentName))
	f.add("work_location", in.WorkLocation)
	if in.BaseSalary != nil {
		f.add("base_salary", in.BaseSalary)
	} else {
		f.add("base_salary", salary)
	}
	f.add("notes", in.Notes)
	f.add("created_by_name", in.CreatedBy)
	row, err := s.insertNumbered(ctx, in.ContractType, f)
	if err != nil {
		return "", "", err
	}
	return row.Str("id"), row.Str("contract_number"), nil
}

func firstOf(a, b *string) *string {
	if a != nil {
		return a
	}
	return b
}

// ContractNumberAttempts bounds the retries when a number is already taken
// (also used by recruitment's promotion draft).
const ContractNumberAttempts = 5

// insertNumbered is withContractNumber: insert with the next number of the
// type this year, the highest manual number + 1 (EPIC-075 R6: counting rows
// collided once a contract was deleted or rows were imported), moving to the
// following number when another insert took it.
func (s *Service) insertNumbered(ctx context.Context, contractType string, f fields) (*Row, error) {
	now := domain.NowWIB(s.now())
	last, err := s.repo.MaxContractSeq(ctx, contractType, now.Year())
	if err != nil {
		return nil, err
	}
	for attempt := 1; ; attempt++ {
		f.set("contract_number", domain.BuildContractNumber(contractType, last+attempt, now))
		var row *Row
		err := s.repo.InTx(ctx, func(r Repository) error {
			var err error
			row, err = r.InsertContract(ctx, f)
			return err
		})
		var pgErr *pgconn.PgError
		taken := errors.As(err, &pgErr) && strings.Contains(pgErr.Error(), "employment_contracts_contract_number_key")
		if !taken || attempt == ContractNumberAttempts {
			return row, err
		}
	}
}

func (s *Service) EmployeeContracts(ctx context.Context, employeeID string) ([]*Row, error) {
	return s.repo.EmployeeContracts(ctx, employeeID)
}

// CreateEmployeeContract creates the draft and returns the full row.
func (s *Service) CreateEmployeeContract(ctx context.Context, in draftContract) (*Row, string, error) {
	id, number, err := s.CreateDraftContract(ctx, in)
	if err != nil {
		return nil, "", err
	}
	row, err := s.repo.ContractByID(ctx, id)
	return row, number, err
}

/* ── Lifecycle actions (PATCH /api/hris/contracts/[id]) ──────────────── */

// contractAction is contractActionSchema.
type contractAction struct {
	Action                                      string
	EndDate, SignedAt, KemnakerRegisteredAt     opt[string]
	CompensationPaidAt, Notes, ProbationEndDate opt[string]
	PositionTitle, WorkLocation                 opt[string]
	Reason, SignedDocumentURL, StartDate        *string
	BaseSalary                                  any // float64, string or nil
	BaseSalarySent                              bool
}

// ActionResult is ContractActionResult; CompensationSet adds the key.
type ActionResult struct {
	Message         string
	Compensation    *float64
	CompensationSet bool
}

func (r ActionResult) body() *Row {
	out := obj("message", r.Message)
	if r.CompensationSet {
		out.Set("compensation_amount", r.Compensation)
	}
	return out
}

func (s *Service) LoadContract(ctx context.Context, id string) (*contractRow, error) {
	return s.repo.LoadContract(ctx, id)
}

// todayUTC is new Date().toISOString().slice(0, 10), the lifecycle's today.
func (s *Service) todayUTC() string { return s.now().UTC().Format(domain.DateLayout) }

func keep(v opt[string], current *string) *string {
	if v.Sent {
		return v.Val
	}
	return current
}

// RunContractAction dispatches the lifecycle action.
func (s *Service) RunContractAction(ctx context.Context, c *contractRow, in contractAction, userID, userName string) (ActionResult, error) {
	switch in.Action {
	case "activate":
		return s.activateContract(ctx, c, in, userID)
	case "end":
		return s.endContract(ctx, c, in)
	case "terminate":
		return s.terminateContract(ctx, c, in)
	case "convert":
		if c.Status != "active" || c.ContractType != "pkwt" {
			return ActionResult{}, httpx.Conflict("Hanya kontrak PKWT aktif yang bisa dikonversi ke PKWTT")
		}
		if err := s.repo.SetContractStatus(ctx, c.ID, "converted", nil); err != nil {
			return ActionResult{}, err
		}
		return ActionResult{Message: "Kontrak " + c.ContractNumber + " ditandai konversi — buat kontrak PKWTT baru untuk karyawan ini"}, nil
	case "renew":
		return s.renewContract(ctx, c, in, userName)
	case "edit":
		return s.editDraft(ctx, c, in)
	}
	// update: administrative dates can be corrected (null) or kept (absent).
	err := s.repo.UpdateContractMeta(ctx, c.ID, keep(in.SignedAt, c.SignedAt), in.SignedDocumentURL,
		keep(in.KemnakerRegisteredAt, c.KemnakerRegisteredAt), keep(in.CompensationPaidAt, c.CompensationPaidAt), keep(in.Notes, c.Notes))
	return ActionResult{Message: "Kontrak diperbarui"}, err
}

func (s *Service) activateContract(ctx context.Context, c *contractRow, in contractAction, userID string) (ActionResult, error) {
	if c.Status != "draft" {
		return ActionResult{}, httpx.Conflict("Hanya kontrak berstatus draft yang bisa diaktifkan")
	}
	other, err := s.repo.ActiveContractNumber(ctx, c.EmployeeID, c.ID)
	if err != nil {
		return ActionResult{}, err
	}
	if other != nil {
		return ActionResult{}, httpx.Conflict("Karyawan masih punya kontrak aktif (" + *other +
			") — akhiri dulu sebelum mengaktifkan kontrak baru")
	}
	prevStatus, err := s.repo.EmploymentStatusOf(ctx, c.EmployeeID)
	if err != nil {
		return ActionResult{}, err
	}
	// recorded_by references hris.employees: the activating account's record.
	recorder, err := s.repo.EmployeeIDByUser(ctx, userID)
	if err != nil {
		return ActionResult{}, err
	}
	newStatus := domain.EmploymentStatusOnActivate(c.ContractType, c.ProbationEndDate, s.todayUTC())

	// One transaction: an active contract must not exist without the
	// employee status, the history row and the salary version.
	err = s.repo.InTx(ctx, func(r Repository) error {
		if err := r.ActivateContract(ctx, c, in.SignedAt.Val, newStatus, prevStatus, recorder); err != nil {
			return err
		}
		if c.BaseSalary == nil {
			return nil
		}
		base := domain.JSNumber(*c.BaseSalary)
		if !(base > 0) {
			return nil
		}
		return s.salary.SyncFromContract(ctx, r.querier(), SalarySync{
			EmployeeID: c.EmployeeID, BaseSalary: base, StartDate: c.StartDate, ContractNumber: c.ContractNumber,
		})
	})
	if err != nil {
		return ActionResult{}, err
	}
	return ActionResult{Message: "Kontrak " + c.ContractNumber + " diaktifkan"}, nil
}

func baseSalaryOf(c *contractRow) float64 {
	if c.BaseSalary == nil {
		return 0
	}
	return domain.JSNumber(*c.BaseSalary)
}

func (s *Service) endContract(ctx context.Context, c *contractRow, in contractAction) (ActionResult, error) {
	if c.Status != "active" {
		return ActionResult{}, httpx.Conflict("Hanya kontrak aktif yang bisa diakhiri")
	}
	actualEnd := s.todayUTC()
	switch {
	case in.EndDate.Val != nil:
		actualEnd = *in.EndDate.Val
	case c.EndDate != nil:
		actualEnd = *c.EndDate
	}
	var comp *float64
	if c.ContractType == "pkwt" {
		v := domain.ComputeKompensasi(baseSalaryOf(c), c.StartDate, actualEnd)
		comp = &v
	}
	var f fields
	f.add("end_date", actualEnd)
	f.add("compensation_amount", comp)
	if err := s.repo.SetContractStatus(ctx, c.ID, "ended", f); err != nil {
		return ActionResult{}, err
	}
	return ActionResult{Message: "Kontrak " + c.ContractNumber + " berakhir", Compensation: comp, CompensationSet: true}, nil
}

func (s *Service) terminateContract(ctx context.Context, c *contractRow, in contractAction) (ActionResult, error) {
	if c.Status != "draft" && c.Status != "active" {
		return ActionResult{}, httpx.Conflict("Kontrak sudah tidak berjalan")
	}
	reason := ""
	if in.Reason != nil {
		reason = domain.JSTrim(*in.Reason)
	}
	if reason == "" {
		return ActionResult{}, httpx.BadRequest("Alasan pemutusan kontrak wajib diisi")
	}
	// An early-terminated active PKWT still earns pro-rata compensation.
	var comp *float64
	if c.ContractType == "pkwt" && c.Status == "active" {
		v := domain.ComputeKompensasi(baseSalaryOf(c), c.StartDate, s.todayUTC())
		comp = &v
	}
	var f fields
	f.add("terminated_reason", reason)
	f.add("compensation_amount", comp)
	if err := s.repo.SetContractStatus(ctx, c.ID, "terminated", f); err != nil {
		return ActionResult{}, err
	}
	return ActionResult{Message: "Kontrak " + c.ContractNumber + " diputus", Compensation: comp, CompensationSet: true}, nil
}

func (s *Service) assertPkwtTotal(ctx context.Context, employeeID, start, end string) error {
	chain, err := s.repo.PkwtChain(ctx, employeeID)
	if err != nil {
		return err
	}
	if msg := domain.ValidatePkwtTotal(domain.PkwtChainTotalMonths(chain), domain.MonthsWorked(start, end)); msg != "" {
		return httpx.Status(422, msg)
	}
	return nil
}

func (s *Service) renewContract(ctx context.Context, c *contractRow, in contractAction, userName string) (ActionResult, error) {
	if c.Status != "active" || c.ContractType != "pkwt" {
		return ActionResult{}, httpx.Conflict("Hanya kontrak PKWT aktif yang bisa diperpanjang")
	}
	if in.EndDate.Val == nil || *in.EndDate.Val == "" {
		return ActionResult{}, httpx.BadRequest("Tanggal berakhir kontrak perpanjangan wajib diisi")
	}
	if c.EndDate == nil {
		return ActionResult{}, fmt.Errorf("hris: active PKWT %s has no end date", c.ID)
	}
	newStart := domain.RenewalStartDate(*c.EndDate)
	end := *in.EndDate.Val
	if end <= newStart {
		return ActionResult{}, httpx.BadRequest("Tanggal berakhir perpanjangan harus setelah " + newStart)
	}
	if err := s.assertPkwtTotal(ctx, c.EmployeeID, newStart, end); err != nil {
		return ActionResult{}, err
	}
	var f fields
	f.add("employee_id", c.EmployeeID)
	f.add("contract_number", nil)
	f.add("contract_type", "pkwt")
	f.add("start_date", newStart)
	f.add("end_date", end)
	f.add("parent_contract_id", c.ID)
	f.add("sequence", c.Sequence+1)
	f.add("position_title", c.PositionTitle)
	f.add("department_name", c.DepartmentName)
	f.add("work_location", c.WorkLocation)
	f.add("base_salary", c.BaseSalary)
	f.add("created_by_name", userName)
	row, err := s.insertNumbered(ctx, "pkwt", f)
	if err != nil {
		return ActionResult{}, err
	}
	return ActionResult{Message: "Draft perpanjangan " + row.Str("contract_number") + " dibuat (mulai " + newStart +
		") — aktifkan setelah kontrak berjalan berakhir"}, nil
}

func (s *Service) editDraft(ctx context.Context, c *contractRow, in contractAction) (ActionResult, error) {
	if c.Status != "draft" {
		return ActionResult{}, httpx.Conflict("Hanya kontrak berstatus draft yang bisa diedit")
	}
	start := c.StartDate
	if in.StartDate != nil {
		start = *in.StartDate
	}
	end := keep(in.EndDate, c.EndDate)
	probation := keep(in.ProbationEndDate, c.ProbationEndDate)
	var salary any
	if c.BaseSalary != nil {
		salary = *c.BaseSalary
	}
	if in.BaseSalarySent {
		salary = in.BaseSalary
	}
	if start == "" {
		return ActionResult{}, httpx.BadRequest("Tanggal mulai wajib diisi")
	}
	if salary != nil {
		var n float64
		switch v := salary.(type) {
		case float64:
			n = v
		case string:
			n = domain.JSNumber(v)
		}
		if !domain.IsFinite(n) || n < 0 {
			return ActionResult{}, httpx.BadRequest("Gaji pokok tidak valid")
		}
	}
	if errs := domain.ValidateContractDates(domain.ContractDates{
		ContractType: c.ContractType, StartDate: start, EndDate: end, ProbationEndDate: probation,
	}); len(errs) > 0 {
		return ActionResult{}, httpx.BadRequest(strings.Join(errs, " "))
	}
	if c.ContractType == "pkwt" {
		until := start
		if end != nil {
			until = *end
		}
		if err := s.assertPkwtTotal(ctx, c.EmployeeID, start, until); err != nil {
			return ActionResult{}, err
		}
	}
	var f fields
	f.add("start_date", start)
	f.add("end_date", end)
	f.add("probation_end_date", probation)
	f.add("position_title", keep(in.PositionTitle, c.PositionTitle))
	f.add("work_location", keep(in.WorkLocation, c.WorkLocation))
	f.add("base_salary", salary)
	f.add("notes", keep(in.Notes, c.Notes))
	updated, err := s.repo.UpdateDraftContract(ctx, c.ID, f)
	if err != nil {
		return ActionResult{}, err
	}
	if !updated {
		return ActionResult{}, httpx.Conflict("Kontrak sudah bukan draft — muat ulang halaman")
	}
	return ActionResult{Message: "Draft kontrak " + c.ContractNumber + " diperbarui"}, nil
}

func (s *Service) DeleteDraftContract(ctx context.Context, id string) error {
	deleted, err := s.repo.DeleteDraftContract(ctx, id)
	if err == nil && !deleted {
		return httpx.Conflict("Hanya draft kontrak yang bisa dihapus")
	}
	return err
}

/* ── Lists ───────────────────────────────────────────────────────────── */

func (s *Service) ListContracts(ctx context.Context, p domain.ContractListParams) ([]*Row, int64, error) {
	return s.repo.ListContracts(ctx, p)
}

// ExpiringContracts is loadExpiringContracts: contracts ending within days
// (overdue included), probations ending within days, and active employees
// without an active contract (interns and super admin shells excluded).
func (s *Service) ExpiringContracts(ctx context.Context, days int) (*Row, error) {
	contracts, err := s.repo.ExpiringContracts(ctx, days)
	if err != nil {
		return nil, err
	}
	probations, err := s.repo.EndingProbations(ctx, days)
	if err != nil {
		return nil, err
	}
	admins, err := s.directory.SuperAdminUserIDs(ctx, s.repo.querier())
	if err != nil {
		return nil, err
	}
	none, err := s.repo.EmployeesWithoutContract(ctx, admins)
	if err != nil {
		return nil, err
	}
	return obj("days", days, "contracts", contracts, "probations", probations, "noContract", none), nil
}

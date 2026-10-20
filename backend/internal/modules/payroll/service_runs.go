package payroll

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	payrollevents "nuhabit/backend/internal/contracts/payroll"
	"nuhabit/backend/internal/modules/payroll/domain"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/jsmath"
	"nuhabit/backend/internal/platform/outbox"
)

// Payroll runs (lib/payroll/runs.ts, run-calculation.ts).

var runPeople = []embed{{"processed_by", "processed_by", personFields}, {"approved_by", "approved_by", personFields}}

func (s *service) listRuns(ctx context.Context, year *int, status *string) ([]*obj, error) {
	rows, err := listRunRows(ctx, s.db, year, status)
	if err != nil {
		return nil, opaque(err)
	}
	for _, row := range rows {
		period, err := s.attendancePeriod(ctx, row.Int("period_month"), row.Int("period_year"))
		if err != nil {
			return nil, err
		}
		row.Set("attendance_period", period.info())
	}
	if err := s.attachCompanies(ctx, rows); err != nil {
		return nil, err
	}
	return rows, s.stitch(ctx, s.db, rows, runPeople...)
}

// employeeOf is the employee behind an account (processed_by/approved_by
// reference hris.employees), nil when none.
func (s *service) employeeOf(ctx context.Context, userID string) (*string, error) {
	id, err := s.ports.Employees.IDByUser(ctx, s.db, userID)
	if err != nil || id == "" {
		return nil, err
	}
	return &id, nil
}

func (s *service) createRun(ctx context.Context, userID string, month, year int, name, companyID *string) (*obj, error) {
	id := ""
	if companyID != nil {
		id = *companyID
	}
	entity, err := s.entityByID(ctx, id)
	if err != nil {
		return nil, err
	}
	runName := "Payroll " + domain.PeriodLabelID(&month, year)
	if name != nil && *name != "" {
		runName = *name
	}
	processedBy, err := s.employeeOf(ctx, userID)
	if err != nil {
		return nil, err
	}
	var row *obj
	err = database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		if err := lockPeriod(ctx, tx, month, year); err != nil {
			return err
		}
		other, legacy, found, err := overlappingRun(ctx, tx, month, year, entity.ID)
		switch {
		case err != nil:
			return err
		case found && legacy:
			return httpx.BadRequest("Periode ini masih punya run gabungan sebelum payroll per entitas (" + other +
				") — hapus run itu dulu untuk memproses per entitas")
		case found:
			return httpx.BadRequest("Payroll untuk periode ini sudah ada")
		}
		row, err = insertRun(ctx, tx, runName, month, year, processedBy, entity.ID)
		return err
	})
	// opaque flattens every error (rowjson.go:376): pass the 400s through.
	if he := (*httpx.Error)(nil); errors.As(err, &he) {
		return nil, err
	}
	return row, opaque(err)
}

func (s *service) getRun(ctx context.Context, id string) (*obj, error) {
	row, err := runByID(ctx, s.db, id, true)
	if err != nil {
		return nil, opaque(err)
	}
	if row == nil {
		return nil, httpx.NotFound("Payroll run tidak ditemukan")
	}
	if err := s.stitch(ctx, s.db, []*obj{row}, runPeople...); err != nil {
		return nil, err
	}
	details, _ := row.Get("payroll_details").([]any)
	rows := make([]*obj, 0, len(details))
	for _, d := range details {
		if o, ok := d.(*obj); ok {
			rows = append(rows, o)
		}
	}
	if err := s.stitch(ctx, s.db, rows, embed{"employee", "employee_id", runDetailEmp}); err != nil {
		return nil, err
	}
	summary, err := s.runAllocationSummary(ctx, id)
	if err != nil {
		return nil, err
	}
	row.Set("store_allocation", summary)
	period, err := s.attendancePeriod(ctx, row.Int("period_month"), row.Int("period_year"))
	if err != nil {
		return nil, err
	}
	row.Set("attendance_period", period.info())
	if err := s.attachCompanies(ctx, []*obj{row}); err != nil {
		return nil, err
	}
	return row, nil
}

// runUpdate is updateRunSchema: status (empty means none) and notes
// (Set false when the key was absent).
type runUpdate struct {
	Status   string
	Notes    *string
	NotesSet bool
}

type messageData struct {
	Data    any    `json:"data"`
	Message string `json:"message"`
}

func (s *service) updateRun(ctx context.Context, userID, id string, in runUpdate) (*messageData, error) {
	existing := runHeadByID(ctx, s.db, id)
	if existing == nil {
		return nil, httpx.NotFound("Payroll run tidak ditemukan")
	}
	now := s.clock()
	var patch runPatch
	patch.set("updated_at", now)
	if in.NotesSet {
		patch.set("notes", in.Notes)
	}
	if in.Status != "" && in.Status != existing.Status {
		if !domain.CanTransitionRun(existing.Status, in.Status) {
			return nil, httpx.BadRequest(fmt.Sprintf("Transisi status '%s' → '%s' tidak diizinkan", existing.Status, in.Status))
		}
		if in.Status == "processing" || in.Status == "completed" {
			if err := s.requireClosedPeriod(ctx, existing, in.Status); err != nil {
				return nil, err
			}
		}
		if in.Status == "paid" {
			settled, err := s.markRunPaid(ctx, userID, existing, in.Notes)
			if err != nil {
				return nil, err
			}
			row, _ := runByID(ctx, s.db, id, false)
			if row != nil {
				if err := s.stitch(ctx, s.db, []*obj{row}, runPeople...); err != nil {
					return nil, err
				}
			}
			msg := "Payroll ditandai dibayar"
			if settled > 0 {
				msg += fmt.Sprintf(" — %d cicilan pinjaman dipotong dari saldo", settled)
			}
			return &messageData{Data: row, Message: msg}, nil
		}
		patch.set("status", in.Status)
		actorEmployee, err := s.employeeOf(ctx, userID)
		if err != nil {
			return nil, err
		}
		switch in.Status {
		case "processing":
			patch.set("processed_by", actorEmployee)
			patch.set("processed_at", now)
		case "completed":
			patch.set("approved_by", actorEmployee)
			patch.set("approved_at", now)
		}
	}
	row, err := updateRunRow(ctx, s.db, id, patch)
	if err != nil {
		return nil, opaque(err)
	}
	if row == nil {
		return nil, fmt.Errorf("payroll run %s vanished during update", id)
	}
	return &messageData{Data: row, Message: "Payroll run berhasil diupdate"}, nil
}

type shortfall struct {
	EmployeeID string  `json:"employee_id"`
	Amount     float64 `json:"amount"`
}

// markRunPaid sets the run paid and settles the loan installments on its
// slips in one transaction (the status guard makes a repeat a 409 instead
// of a double deduction), then publishes payroll.run.paid in it.
func (s *service) markRunPaid(ctx context.Context, userID string, run *runHead, notes *string) (int, error) {
	settled := 0
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		paid, err := markPaid(ctx, tx, run.ID, notes)
		if err != nil {
			return err
		}
		if paid == nil {
			return httpx.Conflict("Run sudah diproses/berubah status — muat ulang halaman")
		}
		deductions, err := runLoanDeductions(ctx, tx, run.ID)
		if err != nil {
			return err
		}
		var shortfalls []shortfall
		totalDeducted := 0.0
		deductedBy := map[string]float64{}
		for _, d := range deductions {
			loans, err := lockDueLoans(ctx, tx, d.EmployeeID)
			if err != nil {
				return err
			}
			var due []domain.LoanRow
			for _, l := range loans {
				if domain.IsLoanDue(l, run.PeriodMonth, run.PeriodYear) {
					due = append(due, l)
				}
			}
			deducted := domain.Round(domain.JSNumber(d.Amount))
			allocations := domain.AllocateLoanPayment(due, deducted)
			allocated := 0.0
			for _, a := range allocations {
				allocated += a.Amount
			}
			if allocated < deducted {
				shortfalls = append(shortfalls, shortfall{d.EmployeeID, deducted - allocated})
				continue
			}
			for _, a := range allocations {
				if err := applyLoanAllocation(ctx, tx, a); err != nil {
					return err
				}
				settled++
			}
			totalDeducted += allocated
			deductedBy[d.EmployeeID] += allocated
		}
		if len(shortfalls) > 0 {
			return &httpx.Error{Status: 409, Message: "Cicilan pinjaman di slip tidak lagi cocok dengan saldo pinjaman saat ini " +
				"(kemungkinan run periode lain ditandai dibayar lebih dulu). " +
				"Hapus run ini lalu buat & hitung ulang sebelum menandai dibayar.",
				Details: map[string]any{"shortfalls": shortfalls}}
		}
		event := payrollevents.RunPaid{
			RunID: paid.ID, PeriodMonth: paid.PeriodMonth, PeriodYear: paid.PeriodYear, PaidAt: paid.PaidAt,
			TotalEmployees: paid.TotalEmployees, TotalGross: paid.TotalGross, TotalDeductions: paid.TotalDeductions,
			TotalNet: paid.TotalNet, TotalPph21: paid.TotalPph21, TotalBjtkEmployee: paid.TotalBjtkEmployee,
			TotalBjtkEmployer: paid.TotalBjtkEmployer, TotalLoanDeduction: totalDeducted, SettledLoans: settled, UserID: userID,
		}
		if err := s.splitByCompany(ctx, tx, &event, deductedBy, run.CompanyID); err != nil {
			return err
		}
		return outbox.Publish(ctx, tx, payrollevents.TopicRunPaid, run.ID, event)
	})
	return settled, err
}

// splitByCompany fills the BPJS and Tapera totals and the per-company
// split of a paid run from its slips and the installments settled per
// employee. A run with an entity (runCompany) posts everything to it; a
// legacy run without one splits by each employee's company.
func (s *service) splitByCompany(ctx context.Context, q database.Querier, event *payrollevents.RunPaid, deducted map[string]float64, runCompany *string) error {
	slips, err := runSlipShares(ctx, q, event.RunID)
	if err != nil {
		return err
	}
	ids := make([]string, len(slips))
	for i, slip := range slips {
		ids[i] = slip.EmployeeID
	}
	var companies map[string]string
	if runCompany != nil {
		// A run of one entity posts every slip to that entity.
		companies = make(map[string]string, len(ids))
		for _, id := range ids {
			companies[id] = *runCompany
		}
	} else if companies, err = s.ports.Employees.Companies(ctx, q, ids); err != nil {
		return err
	}
	allocations, err := branchCosts(ctx, q, event.RunID)
	if err != nil {
		return err
	}
	branches := domain.SplitCostsByCompany(allocations, companies)
	for _, g := range domain.SplitRunByCompany(slips, deducted, companies) {
		var company *string
		if g.CompanyID != "" {
			company = &g.CompanyID
		}
		event.TotalBpjsTkEmployer += g.BpjsTkEmployer
		event.TotalBpjsKesEmployer += g.BpjsKesEmployer
		event.TotalBpjsTkEmployee += g.BpjsTkEmployee
		event.TotalBpjsKesEmployee += g.BpjsKesEmployee
		event.TotalTaperaEmployer += g.TaperaEmployer
		event.TotalTaperaEmployee += g.TaperaEmployee
		event.Companies = append(event.Companies, payrollevents.RunCompany{CompanyID: company, TotalGross: g.Gross,
			TotalNet: g.Net, TotalPph21: g.Pph21, TotalLoanDeduction: g.LoanDeduction,
			TotalBpjsTkEmployer: g.BpjsTkEmployer, TotalBpjsKesEmployer: g.BpjsKesEmployer,
			TotalBpjsTkEmployee: g.BpjsTkEmployee, TotalBpjsKesEmployee: g.BpjsKesEmployee,
			TotalTaperaEmployer: g.TaperaEmployer, TotalTaperaEmployee: g.TaperaEmployee,
			Branches: runBranches(branches[g.CompanyID])})
	}
	for _, v := range []*float64{&event.TotalBpjsTkEmployer, &event.TotalBpjsKesEmployer, &event.TotalBpjsTkEmployee,
		&event.TotalBpjsKesEmployee, &event.TotalTaperaEmployer, &event.TotalTaperaEmployee} {
		*v = jsmath.RoundTo(*v, 2)
	}
	return nil
}

func (s *service) deleteRun(ctx context.Context, id string) error {
	existing := runHeadByID(ctx, s.db, id)
	if existing == nil {
		return httpx.NotFound("Payroll run tidak ditemukan")
	}
	if !domain.CanDeleteRun(existing.Status) {
		return httpx.BadRequest("Payroll yang sudah dibayar tidak bisa dihapus")
	}
	// payroll_details cascade with the run (FK ON DELETE CASCADE).
	return opaque(deleteRunRow(ctx, s.db, id))
}

/* ── calculation ────────────────────────────────────────────────────── */

type skipped struct {
	EmployeeID string `json:"employee_id"`
	FullName   string `json:"full_name"`
	Reason     string `json:"reason"`
}

// multiEntityPolicy is the active multi-entity policy (EPIC-054 K5/K6):
// change it only once HR has decided, see domain/policy.go.
const multiEntityPolicy = domain.PolicyPrimaryEntity

type calcSummary struct {
	TotalEmployees int       `json:"total_employees"`
	EmployeeNames  []string  `json:"employee_names"`
	Skipped        []skipped `json:"skipped"`
	// OtherEntity counts the active employees paid only by other entities.
	OtherEntity int `json:"other_entity_count"`
	// AllocationWarnings are the slips whose cost (or part of it) has no
	// store: no placement and no store on attendance ("Tanpa toko").
	AllocationWarnings []allocationWarning `json:"allocation_warnings"`
	domain.RunTotals
}

type calcResult struct {
	Data    []*obj      `json:"data"`
	Summary calcSummary `json:"summary"`
	Message string      `json:"message"`
	// AttendancePeriod is the period the run was calculated over; Warning
	// is set while it is still open.
	AttendancePeriod *periodInfo `json:"attendance_period"`
	Warning          string      `json:"warning,omitempty"`
}

// calculateRun recalculates every active employee of a draft run: old
// details are dropped, each employee's input is loaded and calculated, the
// detail stored, then the run totals written. Like the TS route, each step
// commits on its own; a detail that fails to save is reported as skipped.
func (s *service) calculateRun(ctx context.Context, runID string, includeThr bool) (*calcResult, error) {
	run := runHeadByID(ctx, s.db, runID)
	if run == nil {
		return nil, httpx.NotFound("Payroll run tidak ditemukan")
	}
	if !domain.CanCalculateRun(run.Status) {
		return nil, httpx.BadRequest("Hanya payroll draft yang bisa dihitung")
	}
	if run.CompanyID == nil {
		return nil, httpx.BadRequest("Run gabungan sebelum payroll per entitas tidak bisa dihitung ulang — hapus lalu buat payroll per entitas")
	}
	settings, tax, err := payrollConfigRows(ctx, s.db, run.PeriodYear)
	if err != nil {
		return nil, err
	}
	config := domain.BuildConfig(settings, tax)

	if err := deleteRunDetails(ctx, s.db, runID); err != nil {
		return nil, fmt.Errorf("delete old payroll details: %w", opaque(err))
	}
	period, err := s.attendancePeriod(ctx, run.PeriodMonth, run.PeriodYear)
	if err != nil {
		return nil, err
	}
	start, end := period.Start, period.End
	employees, err := s.ports.Employees.PayrollEmployees(ctx, s.db, start, end)
	if err != nil {
		return nil, fmt.Errorf("load active employees: %w", opaque(err))
	}
	if len(employees) == 0 {
		return nil, httpx.BadRequest("Tidak ada karyawan aktif ditemukan")
	}
	holidays, err := s.ports.Workforce.Holidays(ctx, s.db, start, end)
	if err != nil {
		return nil, err
	}
	company := *run.CompanyID
	branchCompany, err := s.ports.Entities.BranchCompanies(ctx, s.db)
	if err != nil {
		return nil, opaque(err)
	}
	elsewhere, err := onceItemsElsewhere(ctx, s.db, run)
	if err != nil {
		return nil, opaque(err)
	}
	manual, err := runInputs(ctx, s.db, runID)
	if err != nil {
		return nil, opaque(err)
	}
	opts := domain.BuildOptions{OvertimeRequireClockout: config.OvertimeRequireClockout,
		OvertimeRestFromSchedule: config.OvertimeRestFromSchedule, HealthSubsidyAmount: config.BpjsKesSubsidyAmount}

	out := &calcResult{Data: []*obj{}, Summary: calcSummary{EmployeeNames: []string{}, Skipped: []skipped{},
		AllocationWarnings: []allocationWarning{}}, AttendancePeriod: period.info()}
	if !period.Closed {
		out.Warning = openPeriodWarning(period)
	}
	var results []domain.Result
	for _, e := range employees {
		skip := func(reason string) {
			out.Summary.Skipped = append(out.Summary.Skipped, skipped{e.ID, e.FullName, reason})
		}
		salaries, err := activeSalaryEntities(ctx, s.db, e.ID)
		if err != nil {
			return nil, err
		}
		if !slices.ContainsFunc(salaries, func(sal domain.EntitySalary) bool { return sal.CompanyID == company }) {
			if len(salaries) > 0 {
				out.Summary.OtherEntity++
			} else if noEntity, err := hasActiveSalaryWithoutEntity(ctx, s.db, e.ID); err != nil {
				return nil, err
			} else if noEntity {
				skip("Struktur gaji belum punya entitas — buat versi dengan entitas pembayar")
			} else {
				skip("Belum ada struktur gaji aktif")
			}
			continue
		}
		placements, err := s.ports.Workforce.Placements(ctx, s.db, e.ID, start, end)
		if err != nil {
			return nil, err
		}
		in, records, err := s.payrollInput(ctx, e, company, run.PeriodMonth, run.PeriodYear, start, end, includeThr, holidays, opts)
		if err != nil {
			return nil, err
		}
		if in == nil {
			skip("Belum ada struktur gaji aktif")
			continue
		}
		in.ManualInputs = manual[e.ID]
		if config.Pph21Method == "ter" && run.PeriodMonth == 12 {
			if in.Ytd, err = yearToDate(ctx, s.db, run, e.ID); err != nil {
				return nil, opaque(err)
			}
		}
		primary := domain.PrimaryEntity(domain.EntityOf(placements, branchCompany, end), salaries)
		*in = domain.ApplyMultiEntityPolicy(*in, primary == company, multiEntityPolicy)
		// Once-per-period items already on another entity's slip of this period
		// are never repeated.
		if elsewhere[e.ID].Loan {
			in.LoanDeduction, in.LoanBreakdown = 0, nil
		}
		if elsewhere[e.ID].Late {
			in.ClearLate()
		}
		result := domain.Calculate(*in, config)
		var detail *obj
		var allocation domain.SlipAllocation
		err = database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
			var err error
			detail, err = insertDetail(ctx, tx, runID, e.ID, result, *in)
			if err != nil || detail == nil {
				return err
			}
			// The store split reads the stored (numeric(12,2)) amounts so the
			// shares add up to the slip exactly.
			days, scoped := domain.ScopeToCompany(branchDays(records.Attendance), placements, branchCompany, company)
			allocation = domain.AllocateSlip(slipAmounts(detail), days, scoped, start, end)
			return insertAllocations(ctx, tx, runID, detail.Str("id"), e.ID, allocation)
		})
		if err != nil || detail == nil {
			s.log.ErrorContext(ctx, "payroll: gagal menyimpan detail", "employee_id", e.ID, "error", err)
			skip("Gagal menyimpan detail")
			continue
		}
		if w := warningFor(e, allocation); w != nil {
			out.Summary.AllocationWarnings = append(out.Summary.AllocationWarnings, *w)
		}
		detail.Set("employee", object("id", e.ID, "full_name", e.FullName, "nip", e.NIP))
		out.Data = append(out.Data, detail)
		out.Summary.EmployeeNames = append(out.Summary.EmployeeNames, e.FullName)
		results = append(results, result)
	}

	out.Summary.TotalEmployees = len(out.Data)
	out.Summary.RunTotals = domain.SumRunTotals(results)
	if err := updateRunTotals(ctx, s.db, runID, len(out.Data), out.Summary.RunTotals, s.clock()); err != nil {
		// The TS ignores this update's result.
		s.log.ErrorContext(ctx, "payroll: gagal menulis total run", "run_id", runID, "error", err)
	}
	out.Message = fmt.Sprintf("Payroll berhasil dihitung untuk %d karyawan", len(out.Data))
	if n := len(out.Summary.Skipped); n > 0 {
		out.Message += fmt.Sprintf(", %d dilewati", n)
	}
	return out, nil
}

// payrollInput loads one employee's period input and the attendance-side
// records it was built from; nil without an active salary.
func (s *service) payrollInput(ctx context.Context, e EmployeeBrief, companyID string, month, year int, start, end string, includeThr bool,
	holidays map[string]bool, opts domain.BuildOptions) (*domain.Input, domain.WorkforceRecords, error) {
	var records domain.WorkforceRecords
	salary, err := activeSalary(ctx, s.db, e.ID, companyID)
	if err != nil || salary == nil {
		return nil, records, err
	}
	records, err = s.ports.Workforce.PeriodRecords(ctx, s.db, e.ID, start, end)
	if err != nil {
		return nil, records, err
	}
	loans, err := openLoans(ctx, s.db, e.ID)
	if err != nil {
		return nil, records, err
	}
	emp := domain.Employee{ID: e.ID, EmploymentStatus: e.EmploymentStatus, JoinDate: e.JoinDate, EndDate: e.EndDate}
	in := domain.BuildInputRange(emp, *salary, records, loans, holidays, month, year, start, end, includeThr, opts)
	return &in, records, nil
}

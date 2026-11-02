package recruitment

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"nuhabit/backend/internal/modules/recruitment/domain"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/employeekeys"
	"nuhabit/backend/internal/platform/httpx"
)

// ── job openings ────────────────────────────────────────────────────────────

// errNoRows mirrors the shim's PGRST116 on .single() of a missing row: an
// unmapped error, so it renders as a 500.
var errNoRows = errors.New("recruitment: job opening not found (PGRST116)")

// SaveJobOpening creates (id "") or updates an opening from a free-form body.
func (s *Service) SaveJobOpening(ctx context.Context, id string, body map[string]any) (*Row, error) {
	update := id != ""
	p := domain.NormalizeJobOpening(body, update, s.now())
	if msg := domain.ValidateJobOpening(p, update); msg != "" {
		return nil, httpx.BadRequest(msg)
	}
	var row *Row
	var err error
	if update {
		row, err = s.repo.UpdateJobOpening(ctx, s.db, id, p)
	} else {
		row, err = s.repo.InsertJobOpening(ctx, s.db, p)
	}
	if err == nil && row == nil {
		err = errNoRows
	}
	return row, err
}

// ── promotion ───────────────────────────────────────────────────────────────

// promotion is the POST /api/hris/promote body.
type promotion struct {
	Data           any    `json:"data"`
	EmployeeID     string `json:"employee_id"`
	NIP            any    `json:"nip,omitempty"`
	ContractNumber any    `json:"contract_number"`
	Message        string `json:"message"`
	Candidate      struct {
		ID                   string `json:"id"`
		FullName             string `json:"full_name"`
		PromotedToEmployeeID string `json:"promoted_to_employee_id"`
	} `json:"candidate"`
}

// Promote turns a hired / talent pool candidate into an employee through
// public.promote_candidate_to_employee, then drafts a contract from the
// accepted offer. A failed draft only adds a warning to the message.
func (s *Service) Promote(ctx context.Context, in promotionInput) (*promotion, error) {
	c := s.repo.PromotionCandidate(ctx, s.db, in.CandidateID)
	if c == nil {
		return nil, httpx.NotFound(msgCandidateNotFound)
	}
	if c.PromotedTo != nil {
		return nil, httpx.BadRequest("Kandidat sudah dipromosikan menjadi employee")
	}
	if !domain.IsPromotable(c.Status) {
		return nil, httpx.BadRequest(
			fmt.Sprintf(`Status kandidat harus "hired" atau "talent_pool" untuk dipromosikan. Status saat ini: %s`, c.Status),
			map[string]string{"suggestion": `Ubah status kandidat menjadi "hired" terlebih dahulu`})
	}
	joinDate := s.now().UTC().Format("2006-01-02")
	if in.JoinDate != nil && *in.JoinDate != "" {
		joinDate = *in.JoinDate
	}
	status := "probation"
	if in.EmploymentStatus != nil && *in.EmploymentStatus != "" {
		status = *in.EmploymentStatus
	}
	// The NIP is never made up (EPIC-081): required, trimmed, unique.
	nip, err := employeekeys.CleanNIP(in.NIP)
	if err != nil {
		return nil, err
	}
	conflict, err := employeekeys.Find(ctx, s.db, employeekeys.Keys{NIP: &nip}, "")
	if err != nil {
		return nil, err
	}
	if conflict != nil {
		return nil, httpx.BadRequest(conflict.Message())
	}
	employeeID, err := s.repo.PromoteCandidate(ctx, s.db, c.ID, joinDate, status, emptyToNil(in.DepartmentID), emptyToNil(in.ReportingTo), nip)
	if err != nil {
		// A concurrent write won the race past the NIP pre-check.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if friendly, ok := employeekeys.ConstraintMessage(pgErr.ConstraintName); ok {
				return nil, httpx.BadRequest(friendly)
			}
		}
		// a concurrent promotion of the same candidate loses with 409
		if database.IsUniqueViolation(err) {
			return nil, httpx.Conflict("Kandidat sudah dipromosikan menjadi employee")
		}
		return nil, err
	}
	if employeeID == nil {
		return nil, fmt.Errorf("promote_candidate_to_employee tidak mengembalikan id karyawan")
	}

	out := &promotion{EmployeeID: *employeeID}
	employee, err := s.ports.Employees.Employee(ctx, s.db, *employeeID)
	if err != nil {
		s.log.ErrorContext(ctx, "[promote] gagal memuat karyawan baru", "error", err)
	}
	if employee != nil {
		out.Data = employee
		out.NIP = employee.Get("nip")
		if out.NIP == nil {
			out.NIP = jsonNull{}
		}
	}

	number, warning, err := s.draftContract(ctx, *employeeID, c.ID, joinDate, status)
	if err != nil {
		return nil, err
	}
	info := ""
	if number != nil {
		out.ContractNumber = *number
		info = " — draft kontrak " + *number + " dibuat otomatis"
	} else if warning != "" {
		info = " — " + warning
	}
	out.Message = "Berhasil mempromosikan " + c.FullName + " menjadi karyawan" + info
	out.Candidate.ID, out.Candidate.FullName, out.Candidate.PromotedToEmployeeID = c.ID, c.FullName, *employeeID
	return out, nil
}

// jsonNull renders null where omitempty would drop a nil (nip of a loaded
// employee whose nip is NULL).
type jsonNull struct{}

func (jsonNull) MarshalJSON() ([]byte, error) { return []byte("null"), nil }

func emptyToNil(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}
	return s
}

// draftContract is autoDraftContract. The date mapping can fail on a join
// date addMonthsIso cannot read; that error escapes like the TS RangeError.
func (s *Service) draftContract(ctx context.Context, employeeID, candidateID, joinDate, employmentStatus string) (*string, string, error) {
	dates, err := domain.DraftFromEmploymentStatus(employmentStatus, joinDate)
	if err != nil || dates == nil {
		return nil, "", err
	}
	const failed = "Karyawan dibuat, tetapi draft kontrak otomatis gagal — buat manual di tab Kontrak"
	offer, err := s.repo.LatestAcceptedOffer(ctx, s.db, candidateID)
	if err != nil {
		s.log.ErrorContext(ctx, "[promote] auto-draft contract failed", "error", err)
		return nil, failed, nil
	}
	notes := "Draft otomatis saat promote kandidat"
	draft := DraftContract{
		EmployeeID: employeeID, ContractType: dates.ContractType, StartDate: dates.StartDate,
		EndDate: dates.EndDate, ProbationEndDate: dates.ProbationEndDate, CreatedByName: "Sistem (promote kandidat)",
	}
	if offer != nil {
		notes += fmt.Sprintf(" (dari offer v%d yang diterima)", offer.Version)
		draft.PositionTitle, draft.BaseSalary = offer.PositionTitle, offer.BaseSalary
	}
	if dates.ContractType == "pkwt" {
		notes += " — durasi default 12 bulan, sesuaikan sebelum aktivasi"
	}
	draft.Notes = notes + "."
	number, rejection, err := s.ports.Contracts.CreateDraft(ctx, s.db, draft)
	if err != nil {
		s.log.ErrorContext(ctx, "[promote] auto-draft contract failed", "error", err)
		return nil, failed, nil
	}
	if rejection != "" {
		return nil, rejection, nil
	}
	return &number, "", nil
}

// ── live monitoring (HR) ────────────────────────────────────────────────────

// requireLiveSession is requireLiveSession: 404 for an unknown type, a
// non-uuid id or a missing session.
func (s *Service) requireLiveSession(ctx context.Context, sessionType, id string) (*Row, error) {
	if !domain.IsLiveSessionType(sessionType) || !domain.IsUUID(id) {
		return nil, httpx.NotFound("Sesi tidak ditemukan")
	}
	row, err := s.repo.LiveSession(ctx, s.db, sessionType, id)
	if err == nil && row == nil {
		err = httpx.NotFound("Sesi tidak ditemukan")
	}
	return row, err
}

// isLiveSessionRunning reports whether the session is in progress.
func (s *Service) isLiveSessionRunning(ctx context.Context, sessionType, id string) (bool, error) {
	if !domain.IsUUID(id) {
		return false, nil
	}
	return s.repo.SessionExists(ctx, s.db, sessionType, id, true)
}

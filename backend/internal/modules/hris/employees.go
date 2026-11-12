package hris

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/employeekeys"
	"nuhabit/backend/internal/platform/httpx"
)

// Employee directory and records: lib/hris/employees-repo, employees-records,
// employees-profile, employees-shifts.

// EmployeeRepo is the employees storage.
type EmployeeRepo interface {
	DirectoryPage(ctx context.Context, p domain.DirectoryParams) ([]*Row, int64, error)
	EmployeeExists(ctx context.Context, column, value string, excludeID *string) (bool, error)
	// EmployeeKeyConflict is the employee (active or not) already holding one of the keys, nil when free.
	EmployeeKeyConflict(ctx context.Context, k employeekeys.Keys, excludeID string) (*employeekeys.Conflict, error)
	// CheckNewNIP refuses a new employee's NIP out of the EPIC-099 format, as a 400.
	CheckNewNIP(ctx context.Context, nip, joinDate, status string) error
	InsertEmployee(ctx context.Context, f fields) (*Row, error)
	EmployeeDetail(ctx context.Context, id string) (*Row, error)
	EmployeeBrief(ctx context.Context, id string) (*Row, error)
	EmployeeTracked(ctx context.Context, id string) (*domain.TrackedState, error)
	// EmployeeLogin is the employee's email and linked login account (user_id), "" when none.
	EmployeeLogin(ctx context.Context, id string) (email, userID string, err error)
	UpdateEmployee(ctx context.Context, id string, f fields) (*Row, error)
	DeactivateEmployee(ctx context.Context, id string, endDate string, at time.Time) (*Row, error)
	InsertEmploymentHistory(ctx context.Context, f fields) (*Row, error)
	EmploymentHistory(ctx context.Context, employeeID string) ([]*Row, error)
	EmployeeDocuments(ctx context.Context, employeeID string) ([]*Row, error)
	InsertEmployeeDocument(ctx context.Context, f fields) (*Row, error)
	DeleteEmployeeDocument(ctx context.Context, id string) error
	UpdateEmployeeDocument(ctx context.Context, id string, f fields) (*Row, error)
	Departments(ctx context.Context) ([]*Row, error)
	LifecycleEmployee(ctx context.Context, id string) (*Row, error)
	OnboardingProgress(ctx context.Context, employeeID string) (*Row, error)
	LifecycleHistory(ctx context.Context, employeeID string) ([]*Row, error)
	LatestOffboarding(ctx context.Context, employeeID string) (*Row, error)
	EmployeeShiftHistory(ctx context.Context, employeeID string) ([]*Row, error)
	SaveShiftPattern(ctx context.Context, employeeID, effectiveFrom string, days []patternDay, actorName string) error
	IsDirectSubordinate(ctx context.Context, managerID, employeeID string) (bool, error)
	EmployeeName(ctx context.Context, id string) (*string, error)
}

func (s *Service) EmployeeDirectory(ctx context.Context, p domain.DirectoryParams) ([]*Row, int64, error) {
	return s.repo.DirectoryPage(ctx, p)
}

// CreateEmployee is createEmployee: the NIP is required and must be unique,
// the email must be unique.
func (s *Service) CreateEmployee(ctx context.Context, in employeeInput) (*Row, error) {
	nip, err := employeekeys.CleanNIP(in.nip.Val)
	if err != nil {
		return nil, err
	}
	keys := in.keys()
	keys.NIP = &nip
	// EPIC-105 R1: the NIP check and the insert share one transaction, so
	// the sequence lock taken by CheckNewNIP covers the write.
	var row *Row
	err = s.repo.InTx(ctx, func(repo Repository) error {
		if conflict, err := repo.EmployeeKeyConflict(ctx, keys, ""); err != nil {
			return err
		} else if conflict != nil {
			return httpx.BadRequest(conflict.Message())
		}
		// EPIC-099: YYYYMM of the join date + the next number of the series.
		if err := repo.CheckNewNIP(ctx, nip, strOr(in.joinDate.Val), strOr(in.employmentStatus.Val)); err != nil {
			return err
		}
		exists, err := repo.EmployeeExists(ctx, "email", strOr(in.email.Val), nil)
		if err != nil {
			return err
		}
		if exists {
			return httpx.BadRequest("Email sudah digunakan")
		}
		f := in.columns()
		f.set("nip", nip)
		f.set("phone", strOr(nilIfEmpty(in.phone.Val)))
		row, err = repo.InsertEmployee(ctx, f)
		return err
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		// A concurrent write won the race past the pre-check.
		if friendly, ok := employeekeys.ConstraintMessage(pgErr.ConstraintName); ok {
			return nil, httpx.BadRequest(friendly)
		}
		if friendly := domain.EmployeeUniqueMessage(pgErr.Message); friendly != "" {
			return nil, httpx.BadRequest(friendly)
		}
	}
	return row, err
}

// Employee is getEmployee: the record with its references, the manager
// loaded separately.
func (s *Service) Employee(ctx context.Context, id string) (*Row, error) {
	row, err := s.repo.EmployeeDetail(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, httpx.NotFound("Karyawan tidak ditemukan")
	}
	return s.withManager(ctx, row)
}

func (s *Service) withManager(ctx context.Context, row *Row) (*Row, error) {
	var manager any
	if to := row.StrPtr("reporting_to"); to != nil && *to != "" {
		m, err := s.repo.EmployeeBrief(ctx, *to)
		if err != nil {
			return nil, err
		}
		if m != nil {
			manager = m
		}
	}
	row.Set("manager", manager)
	return row, nil
}

// UpdateEmployee is updateEmployee: allowlisted fields, unique email/NIP,
// and an employment history row when a tracked field the PUT sent changed.
// UpdateEmployee updates an employee record. accounts false (no Settings →
// Manajemen User) may not change the email of an employee linked to a login
// account: that email is the login and syncs to it from Manajemen User.
func (s *Service) UpdateEmployee(ctx context.Context, id string, in employeeInput, accounts bool) (*Row, error) {
	current, err := s.repo.EmployeeTracked(ctx, id)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, httpx.NotFound("Karyawan tidak ditemukan")
	}
	if in.reportingTo.Sent && selfManager(id, in.reportingTo.Val) {
		return nil, httpx.BadRequest(errSelfManager)
	}
	if e := in.email.Val; !accounts && e != nil {
		email, userID, err := s.repo.EmployeeLogin(ctx, id)
		if err != nil {
			return nil, err
		}
		if userID != "" && *e != email {
			return nil, httpx.Forbidden("Email karyawan yang punya akun login hanya bisa diubah lewat Settings → Manajemen User")
		}
	}
	if e := in.email.Val; e != nil && *e != "" {
		if taken, err := s.repo.EmployeeExists(ctx, "email", *e, &id); err != nil || taken {
			return nil, errOr(err, httpx.BadRequest("Email sudah digunakan"))
		}
	}
	// NIP can be changed but never blanked (EPIC-081 R2).
	keys := in.keys()
	var nip string
	if in.nip.Sent {
		if nip, err = employeekeys.CleanNIP(in.nip.Val); err != nil {
			return nil, err
		}
		keys.NIP = &nip
	}
	if conflict, err := s.repo.EmployeeKeyConflict(ctx, keys, id); err != nil {
		return nil, err
	} else if conflict != nil {
		return nil, httpx.BadRequest(conflict.Message())
	}
	// A replaced NIP keeps the format (EPIC-099 R4); the old one is left alone.
	if in.nip.Sent && nip != current.NIP {
		join := current.JoinDate
		if j := strOr(in.joinDate.Val); in.joinDate.Sent && j != "" {
			join = j
		}
		if err := employeekeys.CheckChangedNIP(nip, join); err != nil {
			return nil, err
		}
	}

	f := in.columns()
	if in.nip.Sent {
		f.set("nip", nip)
	}
	if in.phone.Sent {
		f.set("phone", strOr(nilIfEmpty(in.phone.Val))) // phone is NOT NULL
	}
	now := s.clock()
	f.set("updated_at", now)
	updated, err := s.repo.UpdateEmployee(ctx, id, f)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		if friendly, ok := employeekeys.ConstraintMessage(pgErr.ConstraintName); ok {
			return nil, httpx.BadRequest(friendly)
		}
	}
	if err != nil {
		return nil, err
	}

	change := domain.EmploymentHistoryChange(*current, domain.TrackedUpdate{
		EmploymentStatus: in.employmentStatus.Val, SentStatus: in.employmentStatus.Sent,
		DepartmentID: in.departmentID.Val, SentDepartment: in.departmentID.Sent,
		SectionID: in.sectionID.Val, SentSection: in.sectionID.Sent,
		JobTitleID: in.jobTitleID.Val, SentJobTitle: in.jobTitleID.Sent,
	})
	if change != nil {
		var h fields
		h.add("employee_id", id)
		h.add("change_type", "status_change")
		h.add("notes", change.Notes)
		h.add("prev_employment_status", change.PrevEmploymentStatus)
		h.add("new_employment_status", change.NewEmploymentStatus)
		h.add("prev_department_id", change.PrevDepartmentID)
		h.add("new_department_id", change.NewDepartmentID)
		h.add("prev_section_id", change.PrevSectionID)
		h.add("new_section_id", change.NewSectionID)
		h.add("prev_job_title_id", change.PrevJobTitleID)
		h.add("new_job_title_id", change.NewJobTitleID)
		h.add("effective_date", now.Format(domain.DateLayout))
		// The history is supplementary: a failure is logged and the update
		// still succeeds (the insert runs in its own savepoint).
		if err := s.repo.InTx(ctx, func(r Repository) error { _, err := r.InsertEmploymentHistory(ctx, h); return err }); err != nil {
			s.log.ErrorContext(ctx, "[employees] employment history insert failed", "error", err)
		}
	}
	return s.withManager(ctx, updated)
}

// DeactivateEmployee is the soft delete: is_active=false, end_date today.
func (s *Service) DeactivateEmployee(ctx context.Context, id string) (*Row, error) {
	existing, err := s.repo.EmployeeTracked(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, httpx.NotFound("Karyawan tidak ditemukan")
	}
	now := s.clock()
	return s.repo.DeactivateEmployee(ctx, id, now.Format(domain.DateLayout), now)
}

func errOr(err, fallback error) error {
	if err != nil {
		return err
	}
	return fallback
}

/* ── Documents and history ───────────────────────────────────────────── */

func (s *Service) EmployeeDocuments(ctx context.Context, employeeID string) ([]*Row, error) {
	return s.repo.EmployeeDocuments(ctx, employeeID)
}

func (s *Service) CreateEmployeeDocument(ctx context.Context, f fields) (*Row, error) {
	return s.repo.InsertEmployeeDocument(ctx, f)
}

func (s *Service) DeleteEmployeeDocument(ctx context.Context, id string) error {
	return s.repo.DeleteEmployeeDocument(ctx, id)
}

func (s *Service) UpdateEmployeeDocument(ctx context.Context, id string, f fields) (*Row, error) {
	f.set("updated_at", s.clock())
	return s.repo.UpdateEmployeeDocument(ctx, id, f)
}

func (s *Service) EmploymentHistory(ctx context.Context, employeeID string) ([]*Row, error) {
	return s.repo.EmploymentHistory(ctx, employeeID)
}

func (s *Service) CreateEmploymentHistory(ctx context.Context, f fields) (*Row, error) {
	return s.repo.InsertEmploymentHistory(ctx, f)
}

func (s *Service) Departments(ctx context.Context) ([]*Row, error) { return s.repo.Departments(ctx) }

/* ── Profile tabs ────────────────────────────────────────────────────── */

var reportStages = map[string]bool{"offer": true, "hired": true}

// RecruitmentDocuments is loadRecruitmentDocuments: CV and pipeline report
// availability of the candidate the employee was promoted from.
func (s *Service) RecruitmentDocuments(ctx context.Context, employeeID string) (any, error) {
	c, err := s.recruitment.PromotedCandidate(ctx, s.repo.querier(), employeeID)
	if err != nil || c == nil {
		return nil, err
	}
	return obj(
		"candidate_id", c.ID,
		"cv_url", c.CVURL,
		"status", c.Status,
		"applied_at", httpx.JSTime(c.CreatedAt),
		"position_title", c.PositionTitle,
		"report_available", reportStages[c.Status],
	), nil
}

// Lifecycle is loadEmployeeLifecycle: recruitment → join → onboarding →
// account → employment history → offboarding.
func (s *Service) Lifecycle(ctx context.Context, employeeID string) (*Row, error) {
	emp, err := s.repo.LifecycleEmployee(ctx, employeeID)
	if err != nil {
		return nil, err
	}
	if emp == nil {
		return nil, httpx.NotFound("Karyawan tidak ditemukan")
	}
	q := s.repo.querier()
	candidate, err := s.recruitment.PromotedCandidate(ctx, q, employeeID)
	if err != nil {
		return nil, err
	}
	var account *Account
	if uid := emp.StrPtr("user_id"); uid != nil {
		if account, err = s.directory.Account(ctx, q, *uid); err != nil {
			return nil, err
		}
	}
	onboarding, err := s.repo.OnboardingProgress(ctx, employeeID)
	if err != nil {
		return nil, err
	}
	history, err := s.repo.LifecycleHistory(ctx, employeeID)
	if err != nil {
		return nil, err
	}
	offboarding, err := s.repo.LatestOffboarding(ctx, employeeID)
	if err != nil {
		return nil, err
	}

	out := obj("employee", obj(
		"join_date", emp.Get("join_date"),
		"end_date", emp.Get("end_date"),
		"employment_status", emp.Get("employment_status"),
		"is_active", emp.Get("is_active"),
		"created_at", emp.Get("created_at"),
		"has_account", emp.StrPtr("user_id") != nil && *emp.StrPtr("user_id") != "",
	))
	var recruitment any
	if candidate != nil {
		recruitment = obj(
			"candidate_id", candidate.ID,
			"applied_at", httpx.JSTime(candidate.CreatedAt),
			"source", candidate.Source,
			"position_title", candidate.PositionTitle,
			"offer_accepted_at", httpx.NewJSTime(candidate.OfferAcceptedAt),
			"promoted_at", httpx.NewJSTime(candidate.PromotionDate),
		)
	}
	out.Set("recruitment", recruitment)
	var acc any
	if account != nil {
		acc = obj("email", account.Email, "created_at", httpx.JSTime(account.CreatedAt),
			"last_sign_in_at", httpx.NewJSTime(account.LastSignInAt))
	}
	out.Set("account", acc)
	var progress any = obj("total", 0, "completed", 0, "last_completed_at", nil)
	if onboarding != nil {
		progress = onboarding
	}
	out.Set("onboarding", progress)
	out.Set("history", history)
	var off any
	if offboarding != nil {
		off = offboarding
	}
	out.Set("offboarding", off)
	return out, nil
}

/* ── Shift pattern (lib/hris/employees-shifts) ───────────────────────── */

type patternDay struct {
	DayOfWeek int
	ShiftID   *string
	Shift2ID  *string // second shift of the day (EPIC-080), nil when none
}

// isoDayNames: ISO day of week 1 = Senin … 7 = Minggu.
var isoDayNames = [...]string{1: "Senin", 2: "Selasa", 3: "Rabu", 4: "Kamis", 5: "Jumat", 6: "Sabtu", 7: "Minggu"}

// patternTwoShiftIssue applies the two-shift rule of the Jadwal Kerja grid
// (EPIC-080 R6) to a weekly pattern: a second shift needs a first one and
// must start after it ends.
func patternTwoShiftIssue(ctx context.Context, q database.Querier, days []patternDay) (string, error) {
	var shifts map[string]domain.UploadShift
	for _, d := range days {
		if d.Shift2ID == nil {
			continue
		}
		day := isoDayNames[d.DayOfWeek]
		if d.ShiftID == nil {
			return day + ": shift kedua hanya bisa diisi bila shift pertama diisi", nil
		}
		if shifts == nil {
			all, err := loadShifts(ctx, q, false)
			if err != nil {
				return "", err
			}
			shifts = map[string]domain.UploadShift{}
			for _, sh := range all {
				shifts[sh.ID] = domain.UploadShift{ID: sh.ID, Code: sh.Code, Start: sh.StartTime, End: sh.EndTime,
					Overnight: sh.Overnight, Active: sh.Active, BreakMinutes: sh.Break}
			}
		}
		first, ok1 := shifts[*d.ShiftID]
		second, ok2 := shifts[*d.Shift2ID]
		if !ok1 || !ok2 {
			return day + ": shift tidak ditemukan", nil
		}
		if !second.Active {
			return day + ": Shift " + second.Code + " sudah nonaktif", nil
		}
		if msg := domain.TwoShiftIssue(first, second); msg != "" {
			return day + ": " + msg, nil
		}
	}
	return "", nil
}

// AuthorizeShiftManager: HR (kepegawaian/workforce) or the direct manager.
// hrName is the HR user's name when the menu guard passed.
func (s *Service) AuthorizeShiftManager(ctx context.Context, actor *Actor, targetID string) (string, error) {
	if actor == nil || actor.EmployeeID == nil {
		return "", httpx.Unauthorized("Unauthorized")
	}
	sub, err := s.repo.IsDirectSubordinate(ctx, *actor.EmployeeID, targetID)
	if err != nil {
		return "", err
	}
	if !sub {
		return "", httpx.Forbidden("Hanya HRD atau atasan langsung yang boleh mengatur jadwal karyawan ini")
	}
	name, err := s.repo.EmployeeName(ctx, *actor.EmployeeID)
	if err != nil {
		return "", err
	}
	if name == nil || *name == "" {
		return "Atasan", nil
	}
	return *name, nil
}

func (s *Service) EmployeeShifts(ctx context.Context, employeeID string) ([]*Row, error) {
	return s.repo.EmployeeShiftHistory(ctx, employeeID)
}

func (s *Service) SaveShiftPattern(ctx context.Context, employeeID, effectiveFrom string, days []patternDay, actorName string) error {
	return s.repo.InTx(ctx, func(r Repository) error {
		// Non-Shift memakai jadwal tetap (EPIC-056); pola bebas hanya untuk Shift.
		var name, typ string
		if err := r.querier().QueryRow(ctx, `SELECT full_name, schedule_type FROM hris.employees WHERE id = $1`, employeeID).
			Scan(&name, &typ); err == nil && typ == scheduleNonShift {
			return httpx.BadRequest(name + " Non-Shift memakai jadwal tetap — ubah tipe jadwal ke Shift dulu")
		}
		msg, err := patternTwoShiftIssue(ctx, r.querier(), days)
		if err != nil {
			return err
		}
		if msg != "" {
			return httpx.BadRequest(msg)
		}
		return r.SaveShiftPattern(ctx, employeeID, effectiveFrom, days, actorName)
	})
}

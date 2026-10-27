// Package hris is the HRIS people context: the employee directory and
// records (documents, employment history, contracts, shift patterns),
// attendance, leave and leave balances, overtime, shifts, public holidays,
// onboarding/offboarding checklists, the employee self service area,
// announcements, in-app notifications, navigation badges, the department
// logbook, the monthly HRIS report and the HR master data (departments,
// positions, employment statuses). Port of frontend/src/app/api/hris/** and
// /api/master/** with the logic of frontend/src/lib/hris/**.
//
// Payroll, loans and KPI tables belong to the payroll module; this module
// reads them through the PayrollReads port. Other contexts read employees
// through the exported Employees read service (employees.go).
package hris

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/module"
	"nuhabit/backend/internal/platform/ratelimit"
	"nuhabit/backend/internal/platform/storage"
)

// Name is the MODULES key.
const Name = "hris"

// Ports are what this module needs from other contexts.
type Ports struct {
	// Payroll reads payslips, loans and KPI summaries (payroll module).
	Payroll PayrollReads
	// Salary reads and versions hris.employee_salary (payroll module).
	Salary SalaryPort
	// Recruitment reads the candidate an employee was promoted from.
	Recruitment Recruitment
	// Directory reads login accounts, super admins and brand names.
	Directory Directory
	// WhatsApp sends the leave request notification to the manager.
	WhatsApp WhatsAppSender
	// HolidayCalendar fetches the public holiday ICS feed; nil uses net/http.
	HolidayCalendar HolidayCalendar
	// Places searches addresses for the Lokasi Toko map; nil uses Nominatim.
	Places PlaceSearch
	// Company reads the company profile printed on contracts and recaps.
	Company CompanyProfile
	// Files is the storage shared with Next; nil is STORAGE_DIR.
	Files *storage.Store
}

// Service holds the HRIS use cases.
type Service struct {
	repo        Repository
	payroll     PayrollReads
	salary      SalaryPort
	recruitment Recruitment
	directory   Directory
	whatsapp    WhatsAppSender
	calendar    HolidayCalendar
	places      PlaceSearch
	company     CompanyProfile
	files       *storage.Store
	limiter     *ratelimit.Limiter
	log         *slog.Logger
	now         func() time.Time
}

type mod struct{ h handlers }

func (m mod) Name() string           { return Name }
func (m mod) Routes() []module.Route { return append(m.h.routes(), m.h.fileRoutes()...) }

// New builds the module. It subscribes the leave notification handler to
// the outbox so the WhatsApp send runs after the leave is committed.
func New(deps module.Deps, ports Ports) module.Module {
	svc := NewService(newStore(deps.DB), ports, deps.Log, deps.Now)
	if deps.Events != nil {
		deps.Events.Subscribe(TopicLeaveRequested, "hris.leave-request-whatsapp", svc.notifyLeaveRequest)
		deps.Events.Subscribe(TopicOvertimeRequested, "hris.overtime-request-whatsapp", svc.notifyOvertimeRequest)
	}
	// The daily missed-attendance digest (EPIC-067 R8). Integration tests
	// build the module with AppEnv "test" and call RunGapDigest directly;
	// app.Routes() (`api -routes`, generate) builds it without a database.
	if deps.DB != nil && deps.Config.AppEnv != "test" {
		go svc.GapDigestLoop(context.Background(), GapDigestEvery)
		// The 1 January leave carry-over (EPIC-068 R9).
		go svc.LeaveCarryOverLoop(context.Background(), LeaveCarryOverEvery)
	}
	return mod{handlers{svc: svc, auth: deps.Auth}}
}

// NewService wires the use cases; log and now may be nil.
func NewService(repo Repository, ports Ports, log *slog.Logger, now func() time.Time) *Service {
	if log == nil {
		log = slog.Default()
	}
	if now == nil {
		now = time.Now
	}
	if ports.HolidayCalendar == nil {
		ports.HolidayCalendar = httpCalendar{client: &http.Client{Timeout: 12 * time.Second}}
	}
	if ports.Places == nil {
		ports.Places = nominatimFromEnv()
	}
	if ports.Files == nil {
		ports.Files = storage.FromEnv()
	}
	return &Service{
		repo: repo, payroll: ports.Payroll, salary: ports.Salary, recruitment: ports.Recruitment,
		directory: ports.Directory, whatsapp: ports.WhatsApp, calendar: ports.HolidayCalendar, places: ports.Places,
		company: ports.Company, files: ports.Files, limiter: ratelimit.New(repo.querier()), log: log, now: now,
	}
}

// clock is now with JavaScript Date precision.
func (s *Service) clock() time.Time { return s.now().UTC().Truncate(time.Millisecond) }

type handlers struct {
	svc  *Service
	auth *auth.Service
}

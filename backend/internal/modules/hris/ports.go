package hris

import (
	"context"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"nuhabit/backend/internal/platform/database"
)

// Ports to other contexts. internal/app wires the adapters; every method
// takes the caller's Querier so a use case can run it in its transaction.

// PayrollReads is what HRIS people reads from the payroll context
// (hris.payroll_runs, payroll_details, loans, employee_kpis).
type PayrollReads interface {
	// LatestPaidPayslip is the employee's newest payslip of a paid run.
	LatestPaidPayslip(ctx context.Context, q database.Querier, employeeID string) (*Payslip, error)
	// ActiveLoans summarizes pending/approved loans with a balance left.
	ActiveLoans(ctx context.Context, q database.Querier, employeeID string) (LoanSummary, error)
	// RecentLoans are the employee's five newest loan requests.
	RecentLoans(ctx context.Context, q database.Querier, employeeID string) ([]RecentLoan, error)
	// LatestKPISummary averages the KPIs of the employee's latest review.
	LatestKPISummary(ctx context.Context, q database.Querier, employeeID string) (KPISummary, error)
	// PendingLoanCount counts loans waiting for a decision.
	PendingLoanCount(ctx context.Context, q database.Querier) (int64, error)
	// ManagerPendingLoanCount counts kasbon waiting for managerID's stage
	// (EPIC-060 manager_approver_id), for the ESS Persetujuan Tim badge.
	ManagerPendingLoanCount(ctx context.Context, q database.Querier, managerID string) (int64, error)
	// LoanUpdatesSince counts the employee's decided loans updated after
	// since (nil: ever).
	LoanUpdatesSince(ctx context.Context, q database.Querier, employeeID string, since *time.Time) (int64, error)
	// PeriodRuns are the payroll runs of a year (any status), for the
	// attendance period closing rules.
	PeriodRuns(ctx context.Context, q database.Querier, year int) ([]PeriodRun, error)
}

// PeriodRun is a payroll run's month and status (draft, processing,
// completed = approved, paid).
type PeriodRun struct {
	ID, Name, Status string
	Year, Month      int
}

// Payslip is a paid payroll detail. NetSalary is the numeric text.
type Payslip struct {
	NetSalary   string
	RunName     *string
	PeriodMonth int64
	PeriodYear  int64
	PaidAt      *string // paid_at::text
}

// LoanSummary amounts are numeric text.
type LoanSummary struct {
	Count              int64
	TotalRemaining     string
	MonthlyInstallment string
}

// RecentLoan is one loan request; CreatedAt is created_at::text.
type RecentLoan struct {
	ID, LoanType, PrincipalAmount, Status, CreatedAt string
}

// KPISummary averages are numeric text (nil when there are no scores).
type KPISummary struct {
	Count          int64
	AvgAchievement *string
	AvgScore       *string
}

// SalaryPort reads and writes hris.employee_salary (payroll context). The
// write runs on the caller's transaction: activating a contract must not
// leave the salary structure out of sync.
type SalaryPort interface {
	// ActiveBaseSalary is the total of the active base salaries across all
	// paying entities (numeric text), nil when there is none.
	ActiveBaseSalary(ctx context.Context, q database.Querier, employeeID string) (*string, error)
	// SyncFromContract versions the salary to the contract's base salary.
	SyncFromContract(ctx context.Context, q database.Querier, in SalarySync) error
}

// SalarySync is syncSalaryFromContract's input.
type SalarySync struct {
	EmployeeID     string
	BaseSalary     float64
	StartDate      string
	ContractNumber string
}

// Recruitment reads the candidate an employee was promoted from.
type Recruitment interface {
	PromotedCandidate(ctx context.Context, q database.Querier, employeeID string) (*PromotedCandidate, error)
}

// PromotedCandidate is the newest candidate promoted to the employee.
type PromotedCandidate struct {
	ID              string
	CVURL           *string
	Status          string
	CreatedAt       time.Time
	PromotionDate   *time.Time
	Source          *string
	PositionTitle   *string
	OfferAcceptedAt *time.Time
}

// Directory reads identity and catalog facts HRIS shows.
type Directory interface {
	// Account is the login account (auth.users) behind userID.
	Account(ctx context.Context, q database.Querier, userID string) (*Account, error)
	// SuperAdminUserIDs are the configuration.users with role super_admin
	// (their employee rows are account shells, not staff).
	SuperAdminUserIDs(ctx context.Context, q database.Querier) ([]string, error)
	// BrandNames maps item.brands ids to names.
	BrandNames(ctx context.Context, q database.Querier, ids []string) (map[string]string, error)
}

// Account is an auth.users row.
type Account struct {
	Email        string
	CreatedAt    time.Time
	LastSignInAt *time.Time
}

// WhatsAppSender sends notifications through the configured WhatsApp
// gateway, deduplicated in configuration.wa_notif_log.
type WhatsAppSender interface {
	// Claim records the notification once per (type, dedupKey); claimed is
	// false when it was already sent.
	Claim(ctx context.Context, q database.Querier, notifType, dedupKey, message string, recipients []string) (id string, claimed bool, err error)
	// Release drops a claim whose send did not happen.
	Release(ctx context.Context, q database.Querier, id string) error
	// Configured reports whether a gateway is set up.
	Configured(ctx context.Context) bool
	// SendText posts the text; sent=false with timedOut=false failed for sure.
	SendText(ctx context.Context, target, message string) (sent, timedOut bool, reason string)
}

// HolidayCalendar fetches the public holiday ICS feed.
type HolidayCalendar interface {
	FetchICS(ctx context.Context) (string, error)
}

// defaultICSURL is the public Google calendar of Indonesian holidays
// (HRIS_HOLIDAY_ICS_URL overrides it).
const defaultICSURL = "https://calendar.google.com/calendar/ical/id.indonesian%23holiday%40group.v.calendar.google.com/public/basic.ics"

const maxICSBytes = 5_000_000

type httpCalendar struct{ client *http.Client }

func (c httpCalendar) FetchICS(ctx context.Context) (string, error) {
	url := os.Getenv("HRIS_HOLIDAY_ICS_URL")
	if url == "" {
		url = defaultICSURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/calendar")
	res, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return "", calendarError("Kalender sumber membalas HTTP " + strconv.Itoa(res.StatusCode))
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxICSBytes+1))
	if err != nil {
		return "", err
	}
	if len(raw) > maxICSBytes {
		return "", calendarError("Respons kalender terlalu besar, impor dibatalkan")
	}
	text := string(raw)
	if !strings.Contains(text, "BEGIN:VCALENDAR") {
		return "", calendarError("Respons kalender bukan berkas ICS yang valid")
	}
	return text, nil
}

type calendarError string

func (e calendarError) Error() string { return string(e) }

// Package payroll is the HRIS pay context (module name "payroll"): payroll
// runs and their calculation, payslips, payroll settings, salary
// structures, employee loans, KPI, performance reviews, 360 feedback and
// department tasks. Port of the /api/hris routes listed in Routes and their
// libs under frontend/src/lib/{payroll,kpi,hris}.
package payroll

import (
	"log/slog"
	"net/http"
	"time"

	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/module"
)

// Name is the MODULES key.
const Name = "payroll"

type payrollModule struct{ routes []module.Route }

func (m payrollModule) Name() string           { return Name }
func (m payrollModule) Routes() []module.Route { return m.routes }

// New mounts the module on the pool. Employees and attendance-side records
// come from the HRIS people context through ports.
func New(deps module.Deps, ports Ports) module.Module { return NewOn(deps, deps.DB, ports) }

// NewOn mounts the module on db (the pool, or a test transaction).
func NewOn(deps module.Deps, db database.DB, ports Ports) module.Module {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	log := deps.Log
	if log == nil {
		log = slog.Default()
	}
	svc := &service{db: db, ports: ports, now: now, log: log}
	h := &handler{svc: svc, auth: deps.Auth}
	return payrollModule{routes: append(h.routes(), h.fileRoutes()...)}
}

// handler parses requests, guards them and renders the service results.
type handler struct {
	svc  *service
	auth *auth.Service
}

// service holds the use cases. db is the pool or a test transaction.
type service struct {
	db    database.DB
	ports Ports
	now   func() time.Time
	log   *slog.Logger
}

// jakarta is the zone the TS server computes "now"-based periods in.
var jakarta = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.FixedZone("WIB", 7*3600)
	}
	return loc
}()

// clock is the request time in Asia/Jakarta, millisecond precision (a JS Date).
func (s *service) clock() time.Time { return s.now().In(jakarta).Truncate(time.Millisecond) }

func route(pattern string, fn httpx.HandlerFunc) module.Route {
	return module.Route{Pattern: pattern, Handler: httpx.Handle(fn)}
}

func (h *handler) routes() []module.Route {
	return []module.Route{
		route("GET /api/hris/payroll", h.listRuns),
		route("POST /api/hris/payroll", h.createRun),
		route("GET /api/hris/payroll/entities", h.listEntities),
		route("GET /api/hris/payroll/{id}", h.getRun),
		route("PUT /api/hris/payroll/{id}", h.updateRun),
		h.del("DELETE /api/hris/payroll/{id}", delRun, h.deleteRun),
		route("POST /api/hris/payroll/{id}/calculate", h.calculateRun),
		route("GET /api/hris/payroll/{id}/inputs", h.listInputs),
		route("PUT /api/hris/payroll/{id}/inputs", h.putInput),
		h.del("DELETE /api/hris/payroll/{id}/inputs", delInput, h.deleteInput),
		route("POST /api/hris/payroll/{id}/inputs/import", h.importInputs),
		route("GET /api/hris/payroll/{id}/store-costs", h.storeCosts),
		route("GET /api/hris/payroll/{id}/store-costs/export", h.exportStoreCosts),

		route("GET /api/hris/payslips", h.listPayslips),
		route("POST /api/hris/payslips/notify", h.notifyPayslip),

		route("GET /api/hris/payroll-settings", h.getSettings),
		route("PUT /api/hris/payroll-settings", h.putSettings),

		route("GET /api/hris/employee-salary", h.listSalaries),
		route("POST /api/hris/employee-salary", h.createSalary),
		route("GET /api/hris/employee-salary/{id}", h.getSalary),
		route("PUT /api/hris/employee-salary/{id}", h.updateSalary),
		h.del("DELETE /api/hris/employee-salary/{id}", delSalary, h.deleteSalary),

		route("GET /api/hris/loans", h.listLoans),
		route("POST /api/hris/loans", h.createLoan),
		route("POST /api/hris/loans/{id}/approve", h.decideLoan),

		route("GET /api/hris/kpi/targets", h.listTargets),
		route("POST /api/hris/kpi/targets", h.createTarget),
		h.del("DELETE /api/hris/kpi/targets", delKPITarget, h.deleteTarget),
		route("GET /api/hris/kpi/scorecards", h.listScorecards),
		route("PATCH /api/hris/kpi/scorecards", h.patchScorecard),
		route("POST /api/hris/kpi/rubric", h.saveRubric),
		route("GET /api/hris/kpi/recommendation", h.kpiRecommendation),
		route("GET /api/hris/kpi-config", h.getKPIConfig),
		route("PUT /api/hris/kpi-config", h.putKPIConfig),

		route("GET /api/hris/dept-tasks", h.deptTaskBoard),
		route("POST /api/hris/dept-tasks", h.createDeptTask),
		route("PATCH /api/hris/dept-tasks/{id}", h.deactivateDeptTask),
		route("PATCH /api/hris/dept-tasks/occurrences/{id}", h.occurrenceAction),

		route("GET /api/hris/performance/cycles", h.listReviewCycles),
		route("POST /api/hris/performance/cycles", h.openReviewCycle),
		route("GET /api/hris/performance/realtime", h.realtimeKPI),
		route("GET /api/hris/performance/reviews", h.listReviews),
		route("GET /api/hris/performance/reviews/{id}", h.reviewDetail),
		route("PATCH /api/hris/performance/reviews/{id}", h.reviewAction),

		route("GET /api/hris/feedback-approvals", h.listApprovals),
		route("POST /api/hris/feedback-approvals", h.bulkDecide),
		route("POST /api/hris/feedback-approvals/approve", h.approveAssignment),
		route("POST /api/hris/feedback-approvals/reject", h.rejectAssignment),
		route("GET /api/hris/feedback-assignments", h.listAssignments),
		route("POST /api/hris/feedback-assignments", h.createAssignments),
		route("GET /api/hris/feedback-assignments/{id}", h.getAssignment),
		route("PUT /api/hris/feedback-assignments/{id}", h.updateAssignment),
		h.del("DELETE /api/hris/feedback-assignments/{id}", delAssignment, h.deleteAssignment),
		route("GET /api/hris/feedback-categories", h.listCategories),
		route("POST /api/hris/feedback-categories", h.createCategory),
		route("GET /api/hris/feedback-cycles", h.listFeedbackCycles),
		route("POST /api/hris/feedback-cycles", h.createFeedbackCycle),
		route("GET /api/hris/feedback-cycles/{id}", h.getFeedbackCycle),
		route("PUT /api/hris/feedback-cycles/{id}", h.updateFeedbackCycle),
		h.del("DELETE /api/hris/feedback-cycles/{id}", delCycle, h.deleteFeedbackCycle),
		route("GET /api/hris/feedback-responses", h.listResponses),
		route("POST /api/hris/feedback-responses", h.createResponses),
		route("GET /api/hris/feedback-responses/{id}", h.getResponse),
		route("PUT /api/hris/feedback-responses/{id}", h.updateResponse),
		h.del("DELETE /api/hris/feedback-responses/{id}", delResponse, h.deleteResponse),
		route("POST /api/hris/feedback-responses/{id}", h.approveViaResponse),
		route("PATCH /api/hris/feedback-responses/{id}", h.rejectViaResponse),
		route("GET /api/hris/feedback-summaries", h.listSummaries),
		route("POST /api/hris/feedback-summaries", h.createSummary),
	}
}

// writeJSON writes a 200 body.
func writeJSON(w http.ResponseWriter, body any) error { return httpx.JSON(w, http.StatusOK, body) }

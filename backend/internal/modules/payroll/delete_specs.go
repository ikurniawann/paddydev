package payroll

import (
	"nuhabit/backend/internal/platform/hrisdel"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/module"
)

// Payroll and Kinerja deletes (EPIC-061, Q-RA-1): "delete" on the leaf,
// a reason, an audit row. Kinerja deletes are KPI admin work, so they
// check the KPI Scorecard leaf (ruling R1).
var (
	delRun        = hrisdel.Spec{Menu: iam.LeafRunPenggajian, Entity: "hris.payroll_runs", Table: "hris.payroll_runs", IDParam: "id"}
	delInput      = hrisdel.Spec{Menu: iam.LeafRunPenggajian, Entity: "hris.payroll_inputs", IDParam: "id"}
	delSalary     = hrisdel.Spec{Menu: iam.LeafSalary, Entity: "hris.employee_salary", Table: "hris.employee_salary", IDParam: "id"}
	delKPITarget  = hrisdel.Spec{Menu: iam.LeafKPIScorecard, Entity: "performance.kpi_targets", Table: "performance.kpi_targets", IDQuery: "id"}
	delAssignment = hrisdel.Spec{Menu: iam.LeafKPIScorecard, Entity: "performance.feedback_assignments", Table: "performance.feedback_assignments", IDParam: "id"}
	delCycle      = hrisdel.Spec{Menu: iam.LeafKPIScorecard, Entity: "performance.feedback_cycles", Table: "performance.feedback_cycles", IDParam: "id"}
	delResponse   = hrisdel.Spec{Menu: iam.LeafKPIScorecard, Entity: "performance.feedback_responses", Table: "performance.feedback_responses", IDParam: "id"}
)

// del is a route behind hrisdel.Guard on this module's database.
func (h *handler) del(pattern string, spec hrisdel.Spec, fn httpx.HandlerFunc) module.Route {
	return module.Route{Pattern: pattern, Handler: hrisdel.Guard(h.auth, h.svc.db, spec, httpx.Handle(fn))}
}

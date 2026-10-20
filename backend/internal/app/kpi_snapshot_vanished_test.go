package app

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"nuhabit/backend/internal/modules/payroll"
	"nuhabit/backend/internal/modules/payroll/domain"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/testutil"
)

// ghostKPI lists one employee whose row is gone by the time the snapshot
// writes, as when another session deletes it mid-run.
type ghostKPI struct {
	payrollKPISQL
	ghost domain.KPIEmployee
}

func (g ghostKPI) Employees(ctx context.Context, q database.Querier) ([]domain.KPIEmployee, error) {
	list, err := g.payrollKPISQL.Employees(ctx, q)
	return append(list, g.ghost), err
}

// An employee deleted while the collectors ran is skipped; the run does not
// fail on its foreign key (flaky TestKPISnapshotRoute, EPIC-057 ledger).
func TestKPISnapshotSkipsVanishedEmployee(t *testing.T) {
	hr := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{"hris.performance": nil}})
	deps := testutil.Deps(t, func() time.Time { return payrollFilesNow })
	tx := testutil.Tx(t)
	ctx := context.Background()
	var dept string
	if err := tx.QueryRow(ctx, `INSERT INTO hris.departments (name, code) VALUES ('KPI Ghost', $1) RETURNING id::text`,
		"KPG-"+testutil.RandomHex(3)).Scan(&dept); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO performance.kpi_department_indicators (department_id, indicator_id, weight)
		SELECT $1, id, 100 FROM performance.kpi_indicators WHERE code = 'att_ontime'`, dept); err != nil {
		t.Fatal(err)
	}
	ports := payrollPorts()
	ports.KPI = ghostKPI{ghost: domain.KPIEmployee{ID: "7f1c2d3e-4b5a-4c6d-8e9f-0a1b2c3d4e5f", DepartmentID: &dept, RoleCode: "pos"}}
	mux := testutil.Mux(payroll.NewOn(deps, tx, ports))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, testutil.AsStaff(testutil.Request("POST", "/api/hris/kpi/snapshot",
		map[string]any{"period_month": 6, "period_year": 2099}), hr))
	if rec.Code != 200 {
		t.Fatalf("snapshot = %d %s", rec.Code, rec.Body)
	}
}

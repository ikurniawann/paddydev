package app

import (
	"context"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

func TestLoanApproverSQL(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	ins := func(name, level string, reportsTo *string) string {
		var pos, id string
		if err := tx.QueryRow(ctx, `INSERT INTO hris.positions (title, level) VALUES ($1||md5(random()::text), $2) RETURNING id::text`, name, level).Scan(&pos); err != nil {
			t.Fatal(err)
		}
		if err := tx.QueryRow(ctx, `INSERT INTO hris.employees (full_name, nip, email, phone, join_date, is_active, job_title_id, reporting_to)
			VALUES ($1, 'LA-'||md5(random()::text), md5(random()::text)||'@la.test', '08', '2024-01-01', true, $2, $3) RETURNING id::text`,
			name, pos, reportsTo).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	gm := ins("GM", "General Manager", nil)
	mgr := ins("Manager", "manager", &gm)
	spv := ins("SPV", "Supervisor", &mgr)
	staff := ins("Staff", "Staff", &spv)
	got, err := payrollEmployees{}.LoanApprover(ctx, tx, staff)
	if err != nil || got == nil || *got != mgr {
		t.Fatalf("approver staff = %v (%v), want manager", got, err)
	}
	// Manager sendiri: approver = GM.
	if got, _ := (payrollEmployees{}).LoanApprover(ctx, tx, mgr); got == nil || *got != gm {
		t.Fatalf("approver manager = %v, want GM", got)
	}
	// Siklus diri sendiri: nil, tidak hang.
	self := ins("Self", "Staff", nil)
	if _, err := tx.Exec(ctx, `UPDATE hris.employees SET reporting_to = id WHERE id = $1`, self); err != nil {
		t.Fatal(err)
	}
	if got, err := (payrollEmployees{}).LoanApprover(ctx, tx, self); err != nil || got != nil {
		t.Fatalf("approver self-loop = %v (%v), want nil", got, err)
	}
}

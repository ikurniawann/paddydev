package desktop_test

import (
	"context"
	"testing"

	"nuhabit/backend/internal/modules/desktop"
	"nuhabit/backend/internal/platform/testutil"
)

// "Menunggu HR" on the board counts kasbon the manager already approved plus
// pending loans with no manager stage; a pending kasbon still waiting for its
// manager is not HR's yet (EPIC-060).
func TestPendingDecisionsLoansFollowHrStage(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	pg := desktop.NewPostgres(tx)

	before, err := pg.PendingDecisions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var emp, mgr string
	if err := tx.QueryRow(ctx, `INSERT INTO hris.employees (full_name, nip, email, phone, join_date)
		VALUES ('Pending Loan', 'T-'||md5(random()::text), 'pl@desktop.test', '', '2026-01-01') RETURNING id::text`).Scan(&emp); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO hris.employees (full_name, nip, email, phone, join_date)
		VALUES ('Pending Mgr', 'T-'||md5(random()::text), 'pm@desktop.test', '', '2026-01-01') RETURNING id::text`).Scan(&mgr); err != nil {
		t.Fatal(err)
	}
	for _, r := range []struct{ status, manager string }{
		{"manager_approved", mgr}, // HR's turn
		{"pending", ""},           // no manager stage: HR's turn
		{"pending", mgr},          // waits for the manager: not counted
		{"pending", mgr},          // second one waiting
		{"approved", mgr},         // decided: not counted
	} {
		if _, err := tx.Exec(ctx, `INSERT INTO hris.loans (employee_id, loan_type, principal_amount, tenor_months,
			monthly_installment, remaining_balance, status, manager_approver_id)
			VALUES ($1, 'kasbon', 100000, 2, 50000, 100000, $2, NULLIF($3, '')::uuid)`, emp, r.status, r.manager); err != nil {
			t.Fatal(err)
		}
	}
	after, err := pg.PendingDecisions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := after.Pinjaman - before.Pinjaman; got != 2 {
		t.Fatalf("pinjaman delta = %d, want 2 (manager_approved + pending without manager)", got)
	}
}

package app

import (
	"context"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

func TestLoanApproverSQL(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	// hasAccount: whether the employee gets a linked login (auth.users) account.
	insAcct := func(name, level string, reportsTo *string, hasAccount bool) string {
		var pos, id string
		if err := tx.QueryRow(ctx, `INSERT INTO hris.positions (title, level) VALUES ($1||md5(random()::text), $2) RETURNING id::text`, name, level).Scan(&pos); err != nil {
			t.Fatal(err)
		}
		var userID *string
		if hasAccount {
			var u string
			if err := tx.QueryRow(ctx, `INSERT INTO auth.users (email, password_hash) VALUES (md5(random()::text)||'@la.test', 'x') RETURNING id::text`).Scan(&u); err != nil {
				t.Fatal(err)
			}
			userID = &u
		}
		if err := tx.QueryRow(ctx, `INSERT INTO hris.employees (full_name, nip, email, phone, join_date, is_active, job_title_id, reporting_to, user_id)
			VALUES ($1, 'LA-'||md5(random()::text), md5(random()::text)||'@la.test', '08', '2024-01-01', true, $2, $3, $4) RETURNING id::text`,
			name, pos, reportsTo, userID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	ins := func(name, level string, reportsTo *string) string { return insAcct(name, level, reportsTo, true) }
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
	// Manager terdekat tanpa akun login dilewati: approver = manager berakun di atasnya.
	gm2 := ins("GM2", "General Manager", nil)
	mgrNoAcct := insAcct("Manager tanpa akun", "Manager", &gm2, false)
	staff2 := ins("Staff2", "Staff", &mgrNoAcct)
	if got, err := (payrollEmployees{}).LoanApprover(ctx, tx, staff2); err != nil || got == nil || *got != gm2 {
		t.Fatalf("approver skipping account-less manager = %v (%v), want %s", got, err, gm2)
	}
	// Tidak ada manager berakun di rantai: nil.
	mgrNoAcct2 := insAcct("Manager tanpa akun 2", "Manager", nil, false)
	staff3 := ins("Staff3", "Staff", &mgrNoAcct2)
	if got, err := (payrollEmployees{}).LoanApprover(ctx, tx, staff3); err != nil || got != nil {
		t.Fatalf("approver with no account-holding manager = %v (%v), want nil", got, err)
	}
}

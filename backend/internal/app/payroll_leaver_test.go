package app

import (
	"context"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

// PY-04: karyawan nonaktif yang end_date-nya di dalam periode tetap ikut run.
func TestPayrollEmployeesIncludesLeaversInPeriod(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	var leaver, gone string
	if err := tx.QueryRow(ctx, `INSERT INTO hris.employees (full_name, nip, email, phone, join_date, is_active, end_date, employment_status)
		VALUES ('Keluar Periode', 'LV-'||md5(random()::text), md5(random()::text)||'@lv.test', '08', '2024-01-01', false, '2031-03-09', 'resigned')
		RETURNING id::text`).Scan(&leaver); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO hris.employees (full_name, nip, email, phone, join_date, is_active, end_date, employment_status)
		VALUES ('Keluar Lama', 'LV-'||md5(random()::text), md5(random()::text)||'@lv.test', '08', '2024-01-01', false, '2031-01-31', 'resigned')
		RETURNING id::text`).Scan(&gone); err != nil {
		t.Fatal(err)
	}
	list, err := payrollEmployees{}.PayrollEmployees(ctx, tx, "2031-02-21", "2031-03-20")
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, e := range list {
		found[e.ID] = true
	}
	if !found[leaver] || found[gone] {
		t.Fatalf("leaver in=%v, gone in=%v", found[leaver], found[gone])
	}
}

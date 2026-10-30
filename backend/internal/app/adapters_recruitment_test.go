package app

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"nuhabit/backend/internal/modules/recruitment"
	"nuhabit/backend/internal/platform/testutil"
)

// The promotion adapters run on a rolled-back transaction: the DB function
// creates the employee, then the adapters read it and draft its contract.
func TestRecruitmentPromotionAdapters(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	var candidateID, employeeID string
	if err := tx.QueryRow(ctx, `INSERT INTO recruitment.candidates (full_name, email, phone, domicile, source, status)
		VALUES ('Go Adapter', 'go' || md5(random()::text) || '@x.id', '0812', 'Jkt', 'walk_in', 'hired') RETURNING id::text`).Scan(&candidateID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT public.promote_candidate_to_employee($1, '2026-08-01', 'contract', NULL, NULL)::text`, candidateID).Scan(&employeeID); err != nil {
		t.Fatal(err)
	}

	row, err := recruitmentEmployees{}.Employee(ctx, tx, employeeID)
	if err != nil || row == nil {
		t.Fatalf("employee: %v", err)
	}
	if nip, _ := row.Get("nip").(string); !strings.HasPrefix(nip, "EMP-") || row.Get("department") != nil {
		t.Fatalf("nip %v department %v", row.Get("nip"), row.Get("department"))
	}

	now := func() time.Time { return time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC) }
	end, salary := "2027-08-01", "5000000"
	contracts := recruitmentContracts{now: now}
	number, rejection, err := contracts.CreateDraft(ctx, tx, recruitment.DraftContract{
		EmployeeID: employeeID, ContractType: "pkwt", StartDate: "2026-08-01", EndDate: &end,
		BaseSalary: &salary, Notes: "Draft otomatis saat promote kandidat.", CreatedByName: "Sistem (promote kandidat)",
	})
	if err != nil || rejection != "" || !strings.HasSuffix(number, "/PKWT/VII/2026") {
		t.Fatalf("draft: %q %q %v", number, rejection, err)
	}
	var status, base string
	if err := tx.QueryRow(ctx, `SELECT status, base_salary::text FROM hris.employment_contracts WHERE contract_number = $1`, number).Scan(&status, &base); err != nil {
		t.Fatal(err)
	}
	if status != "draft" || base != "5000000.00" {
		t.Fatalf("%s %s", status, base)
	}

	// compliance rejections come back as messages
	if _, rejection, _ := contracts.CreateDraft(ctx, tx, recruitment.DraftContract{EmployeeID: employeeID, ContractType: "pkwt", StartDate: "2026-08-01"}); rejection != "PKWT wajib memiliki tanggal berakhir (perjanjian waktu tertentu)." {
		t.Fatal(rejection)
	}
	if _, rejection, _ := contracts.CreateDraft(ctx, tx, recruitment.DraftContract{EmployeeID: "11111111-1111-4111-8111-111111111111", ContractType: "pkwtt", StartDate: "2026-08-01"}); rejection != "Karyawan tidak ditemukan" {
		t.Fatal(rejection)
	}
	if got := monthsWorked("2026-01-15", "2026-03-01"); got != 1.53 {
		t.Fatal(got)
	}
}

// EPIC-075 review #1: promoting a candidate numbers the draft like the HRIS
// contract page (highest manual number + 1), so a number taken by a deleted
// or imported contract no longer fails every promotion.
func TestRecruitmentDraftSkipsTakenNumber(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	var candidateID, employeeID string
	if err := tx.QueryRow(ctx, `INSERT INTO recruitment.candidates (full_name, email, phone, domicile, source, status)
		VALUES ('Go Nomor', 'gn' || md5(random()::text) || '@x.id', '0812', 'Jkt', 'walk_in', 'hired') RETURNING id::text`).Scan(&candidateID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT public.promote_candidate_to_employee($1, '2026-08-01', 'contract', NULL, NULL)::text`, candidateID).Scan(&employeeID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM hris.employment_contracts
		WHERE contract_type = 'pkwt' AND date_part('year', created_at) = date_part('year', now())`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	// One row goes in: the old count+1 is exactly this number.
	taken := fmt.Sprintf("%04d/PKWT/VII/2026", count+2)
	if _, err := tx.Exec(ctx, `INSERT INTO hris.employment_contracts (employee_id, contract_number, contract_type, start_date, end_date)
		VALUES ($1, $2, 'pkwt', '2026-01-01', '2026-12-31')`, employeeID, taken); err != nil {
		t.Fatal(err)
	}
	end := "2027-08-01"
	contracts := recruitmentContracts{now: func() time.Time { return time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC) }}
	number, rejection, err := contracts.CreateDraft(ctx, tx, recruitment.DraftContract{
		EmployeeID: employeeID, ContractType: "pkwt", StartDate: "2026-08-01", EndDate: &end, CreatedByName: "Sistem",
	})
	if err != nil || rejection != "" || number == taken || !strings.HasSuffix(number, "/PKWT/VII/2026") {
		t.Fatalf("draft number %q (taken %q): %q %v", number, taken, rejection, err)
	}
}

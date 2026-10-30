package app

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"nuhabit/backend/internal/modules/hris"
	"nuhabit/backend/internal/modules/recruitment"
	"nuhabit/backend/internal/modules/recruitment/domain"
	"nuhabit/backend/internal/platform/database"
)

// Stopgap adapters for recruitment's promotion ports, with SQL ported from
// lib/hris/promote-candidate.ts, create-contract.ts and contract-number.ts.
// They touch hris.employees, hris.employment_contracts and
// hris.employee_salary (HRIS people and pay). Swap them for the hris
// module's services once those take the caller's Querier.

// recruitmentEmployees reads the promoted employee like the TS shim:
// employees.* with department and job_title embeds.
type recruitmentEmployees struct{}

var _ recruitment.Employees = recruitmentEmployees{}

func (recruitmentEmployees) Employee(ctx context.Context, q database.Querier, id string) (*recruitment.Row, error) {
	return recruitment.CollectRow(q.Query(ctx, `SELECT *, `+
		`(SELECT row_to_json(e) FROM (SELECT "id", "name", "code" FROM "hris"."departments" WHERE "id" = "employees"."department_id") e) AS "department", `+
		`(SELECT row_to_json(e) FROM (SELECT "id", "title" FROM "hris"."positions" WHERE "id" = "employees"."job_title_id") e) AS "job_title" `+
		`FROM hris.employees WHERE "id" = $1`, id))
}

// recruitmentContracts is createDraftContract for the promotion draft.
type recruitmentContracts struct{ now func() time.Time }

var _ recruitment.ContractDrafts = recruitmentContracts{}

var jakarta = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.FixedZone("WIB", 7*3600)
	}
	return loc
}()

func (c recruitmentContracts) CreateDraft(ctx context.Context, q database.Querier, in recruitment.DraftContract) (string, string, error) {
	if errs := validateContractDates(in.ContractType, in.StartDate, in.EndDate, in.ProbationEndDate); len(errs) > 0 {
		return "", strings.Join(errs, " "), nil
	}
	// snapshot defaults from the employee and the active salary
	var position, department, salary *string
	err := q.QueryRow(ctx, `SELECT p.title AS position_title, d.name AS department_name, s.base_salary::text
     FROM hris.employees e
     LEFT JOIN hris.positions p ON p.id = e.job_title_id
     LEFT JOIN hris.departments d ON d.id = e.department_id
     LEFT JOIN LATERAL (
       SELECT sum(base_salary) AS base_salary FROM hris.employee_salary
       WHERE employee_id = e.id AND is_active
     ) s ON true
     WHERE e.id = $1`, in.EmployeeID).Scan(&position, &department, &salary)
	if database.IsNoRows(err) {
		return "", "Karyawan tidak ditemukan", nil
	}
	if err != nil {
		return "", "", err
	}

	// PKWT total of 5 years across the non-draft chain
	sequence := 1
	if in.ContractType == "pkwt" {
		rows, err := q.Query(ctx, `SELECT start_date::text, end_date::text FROM hris.employment_contracts
       WHERE employee_id = $1 AND contract_type = 'pkwt' AND status <> 'draft'`, in.EmployeeID)
		if err != nil {
			return "", "", err
		}
		total, n := 0.0, 0
		for rows.Next() {
			var start string
			var end *string
			if err := rows.Scan(&start, &end); err != nil {
				rows.Close()
				return "", "", err
			}
			n++
			if end != nil {
				total += monthsWorked(start, *end)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return "", "", err
		}
		end := in.StartDate
		if in.EndDate != nil {
			end = *in.EndDate
		}
		existing := math.Round(total*100) / 100
		if sum := existing + monthsWorked(in.StartDate, end); sum > 60 {
			return "", fmt.Sprintf("Total durasi PKWT karyawan ini akan menjadi %d bulan — melebihi batas 5 tahun (60 bulan) sesuai PP 35/2021. "+
				"Pertimbangkan konversi ke PKWTT (karyawan tetap).", int(math.Floor(sum+0.5))), nil
		}
		sequence = n + 1
	}

	if in.PositionTitle != nil {
		position = in.PositionTitle
	}
	if in.BaseSalary != nil {
		salary = in.BaseSalary
	}
	insert := func(number string) (string, error) {
		var out string
		run := func(db database.Querier) error {
			return db.QueryRow(ctx, `INSERT INTO hris.employment_contracts
         (employee_id, contract_number, contract_type, start_date, end_date,
          probation_end_date, parent_contract_id, sequence, position_title,
          department_name, work_location, base_salary, notes, created_by_name)
       VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
       RETURNING contract_number`,
				in.EmployeeID, number, in.ContractType, in.StartDate, in.EndDate, in.ProbationEndDate, nil, sequence,
				position, department, nil, salary, in.Notes, in.CreatedByName).Scan(&out)
		}
		// a savepoint keeps a conflicting insert from aborting the caller's transaction
		if b, ok := q.(database.TxBeginner); ok {
			return out, database.WithTx(ctx, b, func(tx pgx.Tx) error { return run(tx) })
		}
		return out, run(q)
	}
	// EPIC-075 R6: the next number follows the highest manual number of the
	// type this year (hris.MaxContractSeq), moving on when one is taken.
	now := c.now().In(jakarta)
	last, err := hris.MaxContractSeq(ctx, q, in.ContractType, now.Year())
	if err != nil {
		return "", "", err
	}
	for attempt := 1; ; attempt++ {
		out, err := insert(domain.ContractNumber(in.ContractType, last+attempt, now))
		if !isContractNumberConflict(err) || attempt == hris.ContractNumberAttempts {
			return out, "", err
		}
	}
}

func isContractNumberConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.ConstraintName == "employment_contracts_contract_number_key"
}

// parseContractDate is new Date(value) for the date forms a contract uses.
func parseContractDate(s string) (time.Time, bool) {
	for _, layout := range []string{"2006-01-02", time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02T15:04"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// validateContractDates is validateContractDates (lib/hris/contracts.ts).
func validateContractDates(contractType, startDate string, endDate, probationEnd *string) []string {
	var errs []string
	start, ok := parseContractDate(startDate)
	if !ok {
		return []string{"Tanggal mulai kontrak tidak valid."}
	}
	var end, probation *time.Time
	if endDate != nil && *endDate != "" {
		if t, ok := parseContractDate(*endDate); ok {
			end = &t
		} else {
			errs = append(errs, "Tanggal berakhir kontrak tidak valid.")
		}
	}
	if probationEnd != nil && *probationEnd != "" {
		if t, ok := parseContractDate(*probationEnd); ok {
			probation = &t
		}
	}
	if end != nil && !end.After(start) {
		errs = append(errs, "Tanggal berakhir harus setelah tanggal mulai.")
	}
	if contractType == "pkwt" {
		if end == nil {
			errs = append(errs, "PKWT wajib memiliki tanggal berakhir (perjanjian waktu tertentu).")
		}
		if probationEnd != nil && *probationEnd != "" {
			errs = append(errs, "PKWT tidak boleh memiliki masa percobaan — batal demi hukum (PP 35/2021).")
		}
	} else if probation != nil {
		if !probation.After(start) {
			errs = append(errs, "Akhir masa percobaan harus setelah tanggal mulai.")
		} else if probation.After(start.AddDate(0, domain.PkwttMaxProbationMonths, 0)) {
			errs = append(errs, "Masa percobaan PKWTT maksimal 3 bulan (UU 13/2003 Pasal 60).")
		}
	}
	return errs
}

// monthsWorked is full calendar months plus remaining days / 30, 2 decimals.
func monthsWorked(start, end string) float64 {
	s, ok1 := parseContractDate(start)
	e, ok2 := parseContractDate(end)
	if !ok1 || !ok2 || !e.After(s) {
		return 0
	}
	months := (e.Year()-s.Year())*12 + int(e.Month()-s.Month())
	days := e.Day() - s.Day()
	if days < 0 {
		months--
		days += 30
	}
	return math.Round((float64(months)+float64(days)/30)*100) / 100
}

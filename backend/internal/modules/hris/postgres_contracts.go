package hris

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/database"
)

func (s *store) EmployeeContracts(ctx context.Context, employeeID string) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT * FROM hris.employment_contracts WHERE employee_id = $1 ORDER BY created_at DESC`, employeeID)
}

func (s *store) ContractByID(ctx context.Context, id string) (*Row, error) {
	return queryRow(ctx, s.db, `SELECT * FROM hris.employment_contracts WHERE id = $1`, id)
}

func (s *store) ContractSnapshot(ctx context.Context, employeeID string) (*contractSnapshot, error) {
	var c contractSnapshot
	err := s.db.QueryRow(ctx, `SELECT p.title, d.name
		FROM hris.employees e
		LEFT JOIN hris.positions p ON p.id = e.job_title_id
		LEFT JOIN hris.departments d ON d.id = e.department_id
		WHERE e.id = $1`, employeeID).Scan(&c.PositionTitle, &c.DepartmentName)
	if database.IsNoRows(err) {
		return nil, nil
	}
	return &c, err
}

func (s *store) PkwtChain(ctx context.Context, employeeID string) ([]domain.ContractPeriod, error) {
	rows, err := s.db.Query(ctx, `SELECT start_date::text, end_date::text FROM hris.employment_contracts
		WHERE employee_id = $1 AND contract_type = 'pkwt' AND status <> 'draft'`, employeeID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.ContractPeriod, error) {
		var p domain.ContractPeriod
		err := r.Scan(&p.StartDate, &p.EndDate)
		return p, err
	})
}

func (s *store) MaxContractSeq(ctx context.Context, contractType string, year int) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `SELECT coalesce(max(split_part(contract_number, '/', 1)::int), 0)
		FROM hris.employment_contracts
		WHERE contract_number ~ ('^[0-9]+/' || upper($1) || '/[IVXLC]+/' || $2 || '$')`,
		contractType, strconv.Itoa(year)).Scan(&n)
	return n, err
}

func (s *store) InsertContract(ctx context.Context, f fields) (*Row, error) {
	return insertRow(ctx, s.db, "hris.employment_contracts", f, "id, contract_number")
}

func (s *store) LoadContract(ctx context.Context, id string) (*contractRow, error) {
	var c contractRow
	err := s.db.QueryRow(ctx, `SELECT id::text, employee_id::text, contract_number, contract_type, status,
		start_date::text, end_date::text, probation_end_date::text, base_salary::text,
		position_title, department_name, work_location, notes,
		signed_at::text, kemnaker_registered_at::text, compensation_paid_at::text, sequence
		FROM hris.employment_contracts WHERE id = $1`, id).Scan(
		&c.ID, &c.EmployeeID, &c.ContractNumber, &c.ContractType, &c.Status,
		&c.StartDate, &c.EndDate, &c.ProbationEndDate, &c.BaseSalary,
		&c.PositionTitle, &c.DepartmentName, &c.WorkLocation, &c.Notes,
		&c.SignedAt, &c.KemnakerRegisteredAt, &c.CompensationPaidAt, &c.Sequence)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *store) ActiveContractNumber(ctx context.Context, employeeID, exceptID string) (*string, error) {
	var n string
	err := s.db.QueryRow(ctx, `SELECT contract_number FROM hris.employment_contracts
		WHERE employee_id = $1 AND status = 'active' AND id <> $2 LIMIT 1`, employeeID, exceptID).Scan(&n)
	if database.IsNoRows(err) {
		return nil, nil
	}
	return &n, err
}

func (s *store) EmploymentStatusOf(ctx context.Context, employeeID string) (*string, error) {
	var st *string
	err := s.db.QueryRow(ctx, `SELECT employment_status FROM hris.employees WHERE id = $1`, employeeID).Scan(&st)
	if database.IsNoRows(err) {
		return nil, nil
	}
	return st, err
}

// ActivateContract runs the activation writes (call inside a transaction).
func (s *store) ActivateContract(ctx context.Context, c *contractRow, signedAt *string, newStatus string, prevStatus, recorderID *string) error {
	if _, err := s.db.Exec(ctx, `UPDATE hris.employment_contracts
		SET status = 'active', signed_at = COALESCE($2::date, signed_at, now()::date)
		WHERE id = $1`, c.ID, signedAt); err != nil {
		return err
	}
	var end *string
	if c.ContractType == "pkwt" {
		end = c.EndDate
	}
	if _, err := s.db.Exec(ctx, `UPDATE hris.employees SET employment_status = $2, end_date = $3 WHERE id = $1`,
		c.EmployeeID, newStatus, end); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `INSERT INTO hris.employment_history
		(employee_id, change_type, effective_date, prev_employment_status,
		 new_employment_status, reason, recorded_by)
		VALUES ($1, 'contract_activated', $2, $3, $4, $5, $6)`,
		c.EmployeeID, c.StartDate, prevStatus, newStatus,
		fmt.Sprintf("Kontrak %s (%s) aktif", c.ContractNumber, strings.ToUpper(c.ContractType)), recorderID)
	return err
}

func (s *store) SetContractStatus(ctx context.Context, id, status string, f fields) error {
	set := append(fields{{"status", status}}, f...)
	_, err := updateRows(ctx, s.db, "hris.employment_contracts", set, fields{{"id", id}}, "id")
	return err
}

func (s *store) UpdateDraftContract(ctx context.Context, id string, f fields) (bool, error) {
	rows, err := updateRows(ctx, s.db, "hris.employment_contracts", f, fields{{"id", id}, {"status", "draft"}}, "id")
	return len(rows) > 0, err
}

func (s *store) UpdateContractMeta(ctx context.Context, id string, signedAt, documentURL, kemnaker, paidAt, notes *string) error {
	_, err := s.db.Exec(ctx, `UPDATE hris.employment_contracts
		SET signed_at              = $2::date,
		    signed_document_url    = COALESCE($3, signed_document_url),
		    kemnaker_registered_at = $4::date,
		    compensation_paid_at   = $5::date,
		    notes                  = $6
		WHERE id = $1`, id, signedAt, documentURL, kemnaker, paidAt, notes)
	return err
}

func (s *store) DeleteDraftContract(ctx context.Context, id string) (bool, error) {
	tag, err := s.db.Exec(ctx, `DELETE FROM hris.employment_contracts WHERE id = $1 AND status = 'draft'`, id)
	return tag.RowsAffected() > 0, err
}

func (s *store) ListContracts(ctx context.Context, p domain.ContractListParams) ([]*Row, int64, error) {
	var where []string
	var args []any
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.ReplaceAll(cond, "$?", fmt.Sprintf("$%d", len(args))))
	}
	if p.Status != nil {
		add("c.status = $?", *p.Status)
	}
	if p.ContractType != nil {
		add("c.contract_type = $?", *p.ContractType)
	}
	if p.Search != nil {
		add("(e.full_name ILIKE $? OR c.contract_number ILIKE $?)", "%"+*p.Search+"%")
	}
	if p.ExpiringWithin != nil {
		add("c.end_date IS NOT NULL AND c.end_date <= current_date + $?::int", *p.ExpiringWithin)
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "WHERE " + strings.Join(where, " AND ")
	}
	from := `FROM hris.employment_contracts c JOIN hris.employees e ON e.id = c.employee_id ` + whereSQL
	total, err := countRows(ctx, s.db, `SELECT count(*) `+from, args...)
	if err != nil {
		return nil, 0, err
	}
	order := "ASC"
	if p.SortOrder == "desc" {
		order = "DESC"
	}
	n := len(args)
	rows, err := queryRows(ctx, s.db, fmt.Sprintf(`SELECT c.id, c.employee_id, e.full_name AS employee_name,
		c.contract_number, c.contract_type, c.status,
		c.start_date, c.end_date, c.probation_end_date,
		(c.end_date - current_date)::int AS days_left,
		c.position_title, c.department_name, c.base_salary, c.sequence
		%s ORDER BY %s %s NULLS LAST, c.created_at DESC LIMIT $%d OFFSET $%d`,
		from, domain.ContractSortColumns[p.SortBy], order, n+1, n+2), append(args, p.Limit, (p.Page-1)*p.Limit)...)
	return rows, total, err
}

func (s *store) ExpiringContracts(ctx context.Context, days int) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT c.id AS contract_id, c.employee_id, e.full_name AS employee_name,
		c.contract_number, c.contract_type, c.position_title, c.end_date,
		(c.end_date - current_date)::int AS days_left
		FROM hris.employment_contracts c
		JOIN hris.employees e ON e.id = c.employee_id
		WHERE c.status = 'active'
		  AND c.end_date IS NOT NULL
		  AND c.end_date <= current_date + $1::int
		ORDER BY c.end_date ASC`, days)
}

func (s *store) EndingProbations(ctx context.Context, days int) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT c.id AS contract_id, c.employee_id, e.full_name AS employee_name,
		c.contract_number, c.position_title, c.probation_end_date,
		(c.probation_end_date - current_date)::int AS days_left
		FROM hris.employment_contracts c
		JOIN hris.employees e ON e.id = c.employee_id
		WHERE c.status = 'active'
		  AND c.probation_end_date IS NOT NULL
		  AND c.probation_end_date BETWEEN current_date AND current_date + $1::int
		ORDER BY c.probation_end_date ASC`, days)
}

// EmployeesWithoutContract skips the super admin account shells (their
// user ids come from the Directory port).
func (s *store) EmployeesWithoutContract(ctx context.Context, superAdmins []string) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT e.id AS employee_id, e.full_name AS employee_name,
		e.employment_status, e.join_date,
		d.contract_number AS draft_contract_number,
		d.start_date AS draft_start_date
		FROM hris.employees e
		LEFT JOIN LATERAL (
		  SELECT contract_number, start_date FROM hris.employment_contracts c
		  WHERE c.employee_id = e.id AND c.status = 'draft'
		  ORDER BY c.created_at DESC LIMIT 1
		) d ON true
		WHERE e.is_active
		  AND e.employment_status <> 'internship'
		  AND NOT EXISTS (
		    SELECT 1 FROM hris.employment_contracts c
		    WHERE c.employee_id = e.id AND c.status = 'active'
		  )
		  AND (e.user_id IS NULL OR NOT (e.user_id = ANY($1::uuid[])))
		ORDER BY e.join_date ASC NULLS LAST`, nonNil(superAdmins))
}

func nonNil(ids []string) []string {
	if ids == nil {
		return []string{}
	}
	return ids
}

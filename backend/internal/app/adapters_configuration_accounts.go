package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/modules/configuration/accounts"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/module"
	"nuhabit/backend/internal/platform/validate"
)

// Staff accounts reach auth.users (identity) and hris.employees (hris)
// through these adapters. The SQL is what lib/pg/create-client.ts (the
// auth.admin shim) and the query builder ran for lib/users/user-service.ts.

func configurationAccountsPorts(d module.Deps) accounts.Ports {
	return accounts.Ports{Identity: accountsIdentity{}, Employees: accountsEmployees{}}
}

/* ── auth.users ──────────────────────────────────────────────────────── */

type accountsIdentity struct{}

var _ accounts.Identity = accountsIdentity{}

// CreateUser hashes with pgcrypto's bcrypt (cost 10, $2a$), which bcryptjs
// verifies at login.
func (accountsIdentity) CreateUser(ctx context.Context, q database.Querier, u accounts.NewAuthUser) (string, error) {
	userMeta, _ := json.Marshal(map[string]string{"full_name": u.FullName})
	appMeta, _ := json.Marshal(map[string]string{"role": u.Role})
	var id string
	err := q.QueryRow(ctx, `INSERT INTO auth.users (email, password_hash, email_verified_at, raw_user_meta_data, raw_app_meta_data)
  VALUES ($1, public.crypt($2, public.gen_salt('bf', 10)), NOW(), $3::jsonb, $4::jsonb)
  RETURNING id::text`, validate.JSTrim(u.Email), u.Password, string(userMeta), string(appMeta)).Scan(&id)
	return id, err
}

func (accountsIdentity) UpdateUser(ctx context.Context, q database.Querier, id string, u accounts.AuthUpdate) (bool, error) {
	var sets []string
	var args []any
	param := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	if u.Password != nil && *u.Password != "" {
		sets = append(sets, "password_hash = public.crypt("+param(*u.Password)+", public.gen_salt('bf', 10))")
	}
	if u.Email != nil && *u.Email != "" {
		sets = append(sets, "email = "+param(validate.JSTrim(*u.Email)))
	}
	if u.FullName != nil {
		raw, _ := json.Marshal(map[string]string{"full_name": *u.FullName})
		sets = append(sets, "raw_user_meta_data = "+param(string(raw))+"::jsonb")
	}
	if u.Role != nil {
		raw, _ := json.Marshal(map[string]string{"role": *u.Role})
		sets = append(sets, "raw_app_meta_data = "+param(string(raw))+"::jsonb")
	}
	if u.Ban != nil {
		if *u.Ban {
			sets = append(sets, "banned_until = NOW() + INTERVAL '876000 hours'")
		} else {
			sets = append(sets, "banned_until = NULL")
		}
	}
	if len(sets) > 0 {
		if _, err := q.Exec(ctx, `UPDATE auth.users SET `+strings.Join(sets, ", ")+` WHERE id = `+param(id)+`::text::uuid`, args...); err != nil {
			return false, err
		}
	}
	var found bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM auth.users WHERE id = $1::text::uuid)`, id).Scan(&found)
	return found, err
}

func (accountsIdentity) Email(ctx context.Context, q database.Querier, id string) (string, error) {
	var email string
	err := q.QueryRow(ctx, `SELECT email FROM auth.users WHERE id = $1::text::uuid`, id).Scan(&email)
	if database.IsNoRows(err) {
		return "", nil
	}
	return email, err
}

// LastSignIns reads the users listUsers reaches in 19 pages of 100, newest
// first.
func (accountsIdentity) LastSignIns(ctx context.Context, q database.Querier, ids []string) (map[string]*time.Time, error) {
	rows, err := q.Query(ctx, `SELECT id::text, last_sign_in_at
  FROM (SELECT id, last_sign_in_at FROM auth.users ORDER BY created_at DESC LIMIT 1900) u
  WHERE id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*time.Time{}
	for rows.Next() {
		var id string
		var at *time.Time
		if err := rows.Scan(&id, &at); err != nil {
			return nil, err
		}
		out[id] = at
	}
	return out, rows.Err()
}

func (accountsIdentity) Statuses(ctx context.Context, q database.Querier) ([]accounts.AuthStatus, error) {
	rows, err := q.Query(ctx, `SELECT id::text, email, banned_until::text, last_sign_in_at::text, email_verified_at::text FROM auth.users`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (accounts.AuthStatus, error) {
		var a accounts.AuthStatus
		err := r.Scan(&a.ID, &a.Email, &a.BannedUntil, &a.LastSignInAt, &a.EmailVerifiedAt)
		return a, err
	})
}

/* ── hris.employees ──────────────────────────────────────────────────── */

type accountsEmployees struct{}

var _ accounts.Employees = accountsEmployees{}

// employeeSelect is EMPLOYEE_SELECT as the query builder renders it; the
// manager embed aliases the inner employees so reporting_to is the outer
// row's.
const employeeSelect = `SELECT id::text, user_id::text, full_name, nip, email, phone, join_date, end_date,
  employment_status, is_active, is_access_app, schedule_type, department_id::text, section_id::text, job_title_id::text,
  reporting_to::text, ktp, npwp, birth_date, gender::text, marital_status::text, address, city, province,
  postal_code, bank_name, bank_account, bpjs_tk, bpjs_kesehatan, emergency_contact_name,
  emergency_contact_phone, emergency_contact_relationship, photo_url, notes, created_at, updated_at,
  (SELECT row_to_json(e) FROM (SELECT "id", "name", "code" FROM "hris"."departments" WHERE "id" = "employees"."department_id") e) AS "department",
  (SELECT row_to_json(e) FROM (SELECT "id", "name", "code" FROM "hris"."sections" WHERE "id" = "employees"."section_id") e) AS "section",
  (SELECT row_to_json(e) FROM (SELECT "id", "title", "department" FROM "hris"."positions" WHERE "id" = "employees"."job_title_id") e) AS "job_title",
  (SELECT row_to_json(e) FROM (SELECT "id", "full_name", "nip" FROM "hris"."employees" AS "m" WHERE "m"."id" = "employees"."reporting_to") e) AS "manager"
  FROM hris.employees`

func scanEmployee(r pgx.CollectableRow) (accounts.Employee, error) {
	var e accounts.Employee
	var dept, section, job, manager []byte
	err := r.Scan(&e.ID, &e.UserID, &e.FullName, &e.NIP, &e.Email, &e.Phone, &e.JoinDate, &e.EndDate,
		&e.EmploymentStatus, &e.IsActive, &e.IsAccessApp, &e.ScheduleType, &e.DepartmentID, &e.SectionID, &e.JobTitleID,
		&e.ReportingTo, &e.KTP, &e.NPWP, &e.BirthDate, &e.Gender, &e.MaritalStatus, &e.Address, &e.City, &e.Province,
		&e.PostalCode, &e.BankName, &e.BankAccount, &e.BPJSTK, &e.BPJSKesehatan, &e.EmergencyName,
		&e.EmergencyPhone, &e.EmergencyRelation, &e.PhotoURL, &e.Notes, &e.CreatedAt, &e.UpdatedAt,
		&dept, &section, &job, &manager)
	e.Department, e.Section, e.JobTitle, e.Manager = json.RawMessage(dept), json.RawMessage(section), json.RawMessage(job), json.RawMessage(manager)
	return e, err
}

// List mirrors the query builder: the eq filters, then the or() ilike
// filter whose * become %, a separate count(*), ORDER BY, LIMIT/OFFSET.
// Ids are compared as text cast to uuid so PostgreSQL, not pgx, rejects a
// malformed one (22P02, as node-postgres reports it).
func (accountsEmployees) List(ctx context.Context, q database.Querier, f accounts.EmployeeFilter) ([]accounts.Employee, int, error) {
	var where []string
	var args []any
	param := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	if f.DepartmentID != nil {
		where = append(where, `"department_id" = `+param(*f.DepartmentID)+`::text::uuid`)
	}
	if f.EmploymentStatus != nil {
		where = append(where, `"employment_status" = `+param(*f.EmploymentStatus))
	}
	if f.IsActive != nil {
		where = append(where, `"is_active" = `+param(*f.IsActive))
	}
	if f.IsAccessApp != nil {
		where = append(where, `"is_access_app" = `+param(*f.IsAccessApp))
	}
	if f.ScheduleType != nil {
		where = append(where, `"schedule_type" = `+param(*f.ScheduleType))
	}
	if f.Search != nil {
		p := param(strings.ReplaceAll("%"+*f.Search+"%", "*", "%"))
		where = append(where, `("full_name" ILIKE `+p+` OR "email" ILIKE `+p+` OR "nip" ILIKE `+p+`)`)
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*)::int FROM hris.employees`+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	dir := "DESC"
	if f.Asc {
		dir = "ASC"
	}
	sql := fmt.Sprintf(`%s%s ORDER BY "%s" %s LIMIT %s OFFSET %s`, employeeSelect, clause,
		strings.ReplaceAll(f.SortBy, `"`, `""`), dir, param(f.Limit), param(f.Offset))
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, err
	}
	list, err := pgx.CollectRows(rows, scanEmployee)
	return list, total, err
}

func (accountsEmployees) Get(ctx context.Context, q database.Querier, id string) (*accounts.Employee, error) {
	rows, err := q.Query(ctx, employeeSelect+` WHERE "id" = $1::text::uuid`, id)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, scanEmployee)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return &list[0], nil
}

func (accountsEmployees) Access(ctx context.Context, q database.Querier, id string) (*accounts.EmployeeAccess, error) {
	var a accounts.EmployeeAccess
	err := q.QueryRow(ctx, `SELECT id::text, user_id::text, is_access_app, email, full_name
  FROM hris.employees WHERE id = $1::text::uuid`, id).Scan(&a.ID, &a.UserID, &a.IsAccessApp, &a.Email, &a.FullName)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (accountsEmployees) EmailTaken(ctx context.Context, q database.Querier, email, exceptID string) (bool, error) {
	var taken bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM hris.employees WHERE email = $1 AND ($2 = '' OR id <> $2::uuid))`,
		email, exceptID).Scan(&taken)
	return taken, err
}

func (accountsEmployees) EmailOwner(ctx context.Context, q database.Querier, email string) (*accounts.EmailOwnerRow, error) {
	var o accounts.EmailOwnerRow
	err := q.QueryRow(ctx, `SELECT id::text, nip, full_name, is_active, end_date, user_id::text
		FROM hris.employees WHERE lower(email) = lower($1) ORDER BY is_active DESC, updated_at DESC NULLS LAST LIMIT 1`, email).
		Scan(&o.ID, &o.NIP, &o.FullName, &o.IsActive, &o.EndDate, &o.UserID)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (accountsEmployees) ReleaseEmail(ctx context.Context, q database.Querier, id, archive string) (*string, error) {
	var userID *string
	if err := q.QueryRow(ctx, `SELECT user_id::text FROM hris.employees WHERE id = $1::text::uuid FOR UPDATE`, id).Scan(&userID); err != nil {
		return nil, err
	}
	_, err := q.Exec(ctx, `UPDATE hris.employees SET email = $2, user_id = NULL, is_access_app = false, updated_at = now()
		WHERE id = $1::text::uuid`, id, archive)
	return userID, err
}

func (accountsEmployees) NIPTaken(ctx context.Context, q database.Querier, nip string) (bool, error) {
	var taken bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM hris.employees WHERE nip = $1)`, nip).Scan(&taken)
	return taken, err
}

// employeeWritable are the columns the account flows write.
var employeeWritable = map[string]bool{
	"nip": true, "full_name": true, "email": true, "phone": true, "join_date": true, "end_date": true,
	"employment_status": true, "is_active": true, "is_access_app": true, "user_id": true, "schedule_type": true,
	"department_id": true, "section_id": true, "job_title_id": true, "reporting_to": true, "ktp": true,
	"npwp": true, "birth_date": true, "gender": true, "marital_status": true, "address": true, "city": true,
	"province": true, "postal_code": true, "bank_name": true, "bank_account": true, "bpjs_tk": true,
	"bpjs_kesehatan": true, "emergency_contact_name": true, "emergency_contact_phone": true,
	"emergency_contact_relationship": true, "notes": true, "updated_at": true,
}

// columnParams quotes the column names and numbers the values. Values go
// as text and PostgreSQL casts them, as node-postgres sends them.
func columnParams(cols []accounts.Column) (names, params []string, args []any, err error) {
	for _, c := range cols {
		if !employeeWritable[c.Name] {
			return nil, nil, nil, fmt.Errorf("accounts: column %q is not writable", c.Name)
		}
		names = append(names, `"`+c.Name+`"`)
		args = append(args, c.Value)
		params = append(params, "$"+strconv.Itoa(len(args)))
	}
	return names, params, args, nil
}

func (accountsEmployees) Insert(ctx context.Context, q database.Querier, cols []accounts.Column) (string, error) {
	names, params, args, err := columnParams(cols)
	if err != nil {
		return "", err
	}
	var id string
	err = q.QueryRow(ctx, `INSERT INTO hris.employees (`+strings.Join(names, ", ")+`) VALUES (`+
		strings.Join(params, ", ")+`) RETURNING id::text`, args...).Scan(&id)
	return id, err
}

func (accountsEmployees) Update(ctx context.Context, q database.Querier, id string, cols []accounts.Column) error {
	names, params, args, err := columnParams(cols)
	if err != nil {
		return err
	}
	sets := make([]string, len(names))
	for i := range names {
		sets[i] = names[i] + " = " + params[i]
	}
	args = append(args, id)
	_, err = q.Exec(ctx, `UPDATE hris.employees SET `+strings.Join(sets, ", ")+` WHERE id = $`+strconv.Itoa(len(args))+`::text::uuid`, args...)
	return err
}

package accounts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"nuhabit/backend/internal/modules/configuration/accounts/domain"
	"nuhabit/backend/internal/modules/configuration/kit"
	platformaudit "nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/validate"
)

// lib/users/user-service.ts: staff accounts linked to hris employees.

type service struct {
	db     database.DB
	ports  Ports
	now    func() time.Time
	random io.Reader
}

// stamp is new Date().toISOString() as a timestamp value.
func (s *service) stamp() time.Time { return s.now().UTC().Truncate(time.Millisecond) }

const emailInUseMessage = "Email sudah dipakai akun lain"

var emailText = regexp.MustCompile(`(?i)email`)

// emailConflictOr is emailConflictOr: a unique violation on an email
// column is 409 "Email sudah dipakai akun lain"; any other error is kept.
func emailConflictOr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && emailText.MatchString(pgErr.ConstraintName+" "+pgErr.Message) {
		return httpx.Conflict(emailInUseMessage)
	}
	return err
}

// The user-service errors rethrowUserServiceError turns into a 400.
var (
	errEmployeeNotFound = errors.New("Employee not found")
	errAccessNeedsLogin = errors.New("Password and role are required to enable app access")
)

func rethrowUserServiceError(err error) error {
	if errors.Is(err, errEmployeeNotFound) || errors.Is(err, errAccessNeedsLogin) {
		return httpx.BadRequest(err.Error())
	}
	return err
}

/* ── reads ───────────────────────────────────────────────────────────── */

type userListPage struct {
	Data    []employeeView `json:"data"`
	Total   int            `json:"total"`
	Page    int            `json:"page"`
	PerPage int            `json:"perPage"`
}

// enrich is enrichWithAppUser for a set of rows plus the stall lists.
func (s *service) enrich(ctx context.Context, rows []Employee) ([]employeeView, error) {
	apps := make([]*appUser, len(rows))
	var appIDs, userIDs []string
	for i, e := range rows {
		if e.UserID == nil {
			continue
		}
		userIDs = append(userIDs, *e.UserID)
		app, err := loadAppUser(ctx, s.db, *e.UserID)
		if err != nil {
			return nil, err
		}
		if app != nil {
			apps[i] = app
			appIDs = append(appIDs, *e.UserID)
		}
	}
	if len(appIDs) > 0 {
		signIns, err := s.ports.Identity.LastSignIns(ctx, s.db, appIDs)
		if err != nil {
			return nil, err
		}
		for i, app := range apps {
			if app != nil {
				app.LastSignInAt = signIns[*rows[i].UserID]
			}
		}
	}
	warehouses, err := loadWarehouses(ctx, s.db, userIDs)
	if err != nil {
		return nil, err
	}
	out := make([]employeeView, len(rows))
	for i, e := range rows {
		if apps[i] != nil {
			apps[i].Warehouses = warehouses[*e.UserID]
		}
		if out[i], err = mapEmployee(e, apps[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *service) listUsers(ctx context.Context, q listQuery) (*userListPage, error) {
	rows, total, err := s.ports.Employees.List(ctx, s.db, EmployeeFilter{
		Search: q.search, DepartmentID: q.departmentID, EmploymentStatus: q.employmentStatus,
		IsActive: q.isActive, IsAccessApp: q.isAccessApp, ScheduleType: q.scheduleType, SortBy: q.sortBy, Asc: q.asc,
		Limit: q.limit, Offset: (q.page - 1) * q.limit,
	})
	if err != nil {
		return nil, err
	}
	if q.role != nil {
		var ids []string
		for _, r := range rows {
			if r.UserID != nil {
				ids = append(ids, *r.UserID)
			}
		}
		allowed := map[string]bool{}
		if len(ids) > 0 {
			if allowed, err = usersWithRole(ctx, s.db, ids, *q.role); err != nil {
				return nil, err
			}
		}
		kept := rows[:0]
		for _, r := range rows {
			if r.UserID != nil && allowed[*r.UserID] {
				kept = append(kept, r)
			}
		}
		rows = kept
	}
	data, err := s.enrich(ctx, rows)
	if err != nil {
		return nil, err
	}
	return &userListPage{Data: data, Total: total, Page: q.page, PerPage: q.limit}, nil
}

// getUser is getUserEmployeeById: nil when the employee does not exist.
func (s *service) getUser(ctx context.Context, id string) (*employeeView, error) {
	e, err := s.ports.Employees.Get(ctx, s.db, id)
	if err != nil || e == nil {
		return nil, err
	}
	out, err := s.enrich(ctx, []Employee{*e})
	if err != nil {
		return nil, err
	}
	return &out[0], nil
}

func (s *service) reload(ctx context.Context, id string) (*employeeView, error) {
	v, err := s.getUser(ctx, id)
	if err == nil && v == nil {
		err = errors.New("Failed to load employee data")
	}
	return v, err
}

/* ── create ──────────────────────────────────────────────────────────── */

// generateNIP is generateNip: the first free EMP-<year>-<seq>.
func (s *service) generateNIP(ctx context.Context) (string, error) {
	year := s.now().UTC().Year()
	for seq := 1; seq < 100000; seq++ {
		nip := fmt.Sprintf("EMP-%d-%05d", year, seq)
		taken, err := s.ports.Employees.NIPTaken(ctx, s.db, nip)
		if err != nil {
			return "", err
		}
		if !taken {
			return nip, nil
		}
	}
	return "", errors.New("Unable to generate a unique employee ID")
}

// insertColumns are the hris.employees columns createUserEmployee writes
// with `input.x ?? null`, after nip, full_name, email, phone, join_date,
// employment_status and is_access_app.
var insertColumns = []string{
	"department_id", "section_id", "job_title_id", "reporting_to", "ktp", "npwp", "birth_date",
	"gender", "marital_status", "address", "city", "province", "postal_code", "bank_name",
	"bank_account", "bpjs_tk", "bpjs_kesehatan", "emergency_contact_name", "emergency_contact_phone",
	"emergency_contact_relationship", "notes",
}

func orEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

const emailHeldByResigned = "Email ini milik karyawan resign — konfirmasi lepas email"

// archiveEmail is the address a resigned employee's email is replaced
// with when a new employee takes it (".invalid" never delivers).
func archiveEmail(nip string) string {
	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			return r
		case r >= 'A' && r <= 'Z':
			return r + 32
		}
		return -1
	}, nip)
	return "arsip-" + clean + "@resign.invalid"
}

func (s *service) createUser(ctx context.Context, actorID string, b *employeeBody) (*employeeView, error) {
	email := *b.str("email")
	owner, err := s.ports.Employees.EmailOwner(ctx, s.db, email)
	if err != nil {
		return nil, err
	}
	if owner != nil && owner.IsActive {
		return nil, httpx.Conflict(emailInUseMessage)
	}
	if owner != nil && !b.releaseEmail {
		return nil, httpx.Conflict(emailHeldByResigned)
	}
	nip := ""
	if n := b.str("nip"); n != nil {
		nip = validate.JSTrim(*n)
	}
	if nip == "" {
		if nip, err = s.generateNIP(ctx); err != nil {
			return nil, err
		}
	}
	cols := []Column{
		{"nip", nip}, {"full_name", *b.str("full_name")}, {"email", email}, {"phone", orEmpty(b.str("phone"))},
		{"join_date", *b.str("join_date")}, {"employment_status", *b.str("employment_status")},
		{"is_access_app", *b.isAccessApp},
	}
	scheduleType := "shift"
	if b.scheduleType != nil {
		scheduleType = *b.scheduleType
	}
	cols = append(cols, Column{"schedule_type", scheduleType})
	for _, c := range insertColumns {
		cols = append(cols, Column{c, b.str(c)})
	}
	var id string
	movedAccount := false
	err = database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		var prev *string
		if owner != nil {
			archive := archiveEmail(owner.NIP)
			if prev, err = s.ports.Employees.ReleaseEmail(ctx, tx, owner.ID, archive); err != nil {
				return err
			}
			if err := platformaudit.Write(ctx, tx, platformaudit.Entry{ActorID: actorID, Action: "employee.email_released",
				Entity: "hris.employees", EntityID: &owner.ID, EntityLabel: &owner.FullName,
				Before: map[string]any{"email": email}, After: map[string]any{"email": archive}}); err != nil {
				return err
			}
		}
		if id, err = s.ports.Employees.Insert(ctx, tx, cols); err != nil {
			return emailConflictOr(err)
		}
		if prev == nil {
			return nil
		}
		// The same person keeps their login: move the account to the new record.
		if err := s.ports.Employees.Update(ctx, tx, id, []Column{{"user_id", *prev}, {"is_access_app", true}}); err != nil {
			return err
		}
		ban := false
		if _, err := s.ports.Identity.UpdateUser(ctx, tx, *prev, AuthUpdate{Ban: &ban}); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE configuration.users SET status = 'active', updated_at = $1 WHERE id = $2`, s.stamp(), *prev); err != nil {
			return err
		}
		movedAccount = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !movedAccount && *b.isAccessApp && orEmpty(b.password) != "" && b.role != nil {
		if err := s.provision(ctx, actorID, id, b, email, *b.str("full_name")); err != nil {
			return nil, err
		}
	}
	return s.reload(ctx, id)
}

// stall is stallProfileFields.
type stall struct {
	defaultID                          *string
	canCentralCheckout, canSwitchStall bool
	warehouseIDs                       []string
}

func stallFields(b *employeeBody) stall {
	def := domain.DefaultWarehouseID(b.defaultWarehouseID, b.warehouseIDs)
	central := b.canCentralCheckout != nil && *b.canCentralCheckout
	return stall{
		defaultID:          def,
		canCentralCheckout: central,
		canSwitchStall:     central || (b.canSwitchStall != nil && *b.canSwitchStall),
		warehouseIDs:       domain.SavedWarehouseIDs(def),
	}
}

// profileScope is buildUserProfileFields without brand_id (always null).
func profileScope(role string, b *employeeBody) domain.Scope {
	if role == "super_admin" {
		return domain.Scope{}
	}
	return domain.NormalizeScope(b.businessScope, b.holdingID, b.companyID, b.branchID)
}

// provision is provisionAppAccount. The TS deletes the new auth user when
// a later step fails, and the cascade removes the profile, permissions,
// stalls and IAM role; a savepoint undoes the same steps here.
func (s *service) provision(ctx context.Context, actorID, employeeID string, b *employeeBody, email, fullName string) error {
	role := *b.role
	return database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		userID, err := s.ports.Identity.CreateUser(ctx, tx, NewAuthUser{Email: email, Password: *b.password, FullName: fullName, Role: role})
		if err != nil {
			return emailConflictOr(err)
		}
		status := "active"
		if b.accountStatus != nil {
			status = *b.accountStatus
		}
		scope := profileScope(role, b)
		st := stallFields(b)
		_, err = tx.Exec(ctx, `INSERT INTO configuration.users (id, full_name, email, role, status, can_switch_stall,
  can_central_checkout, default_warehouse_id, brand_id, business_scope, holding_id, company_id, branch_id)
  VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULL, $9, $10, $11, $12)`,
			userID, fullName, email, role, status, st.canSwitchStall, st.canCentralCheckout, st.defaultID,
			scope.BusinessScope, scope.HoldingID, scope.CompanyID, scope.BranchID)
		if err != nil {
			return emailConflictOr(err)
		}
		if status == "inactive" {
			ban := true
			quietly(ctx, tx, func(q database.Querier) error {
				_, err := s.ports.Identity.UpdateUser(ctx, q, userID, AuthUpdate{Ban: &ban})
				return err
			})
		}
		if err := insertApprovals(ctx, tx, userID, actorID, b.approvals); err != nil {
			return err
		}
		if err := s.ports.Employees.Update(ctx, tx, employeeID, []Column{
			{"user_id", userID}, {"is_access_app", true}, {"updated_at", s.stamp()},
		}); err != nil {
			return err
		}
		if b.has("warehouse_ids") || b.has("default_warehouse_id") {
			if err := syncWarehouses(ctx, tx, userID, st.warehouseIDs, b.branchID, s.stamp()); err != nil {
				return err
			}
		}
		if err := syncIamRole(ctx, tx, userID, role, actorID); err != nil {
			return err
		}
		audit(ctx, tx, actorID, userID, "create_user", kit.Obj("employee_id", employeeID, "email", email, "role", role))
		return nil
	})
}

/* ── update ──────────────────────────────────────────────────────────── */

// updateUser updates an employee; accounts false (no Manajemen User) may not
// change the email of an employee linked to a login account, since the email
// syncs to that account.
func (s *service) updateUser(ctx context.Context, actorID, id string, b *employeeBody, accounts bool) (*employeeView, error) {
	existing, err := s.ports.Employees.Access(ctx, s.db, id)
	if err != nil || existing == nil {
		return nil, errEmployeeNotFound
	}
	if e := b.str("email"); !accounts && existing.UserID != nil && e != nil && *e != existing.Email {
		return nil, errLoginEmailNeedsSettings
	}
	if e := b.str("email"); e != nil && *e != "" && *e != existing.Email {
		taken, err := s.ports.Employees.EmailTaken(ctx, s.db, *e, id)
		if err != nil {
			return nil, err
		}
		if taken {
			return nil, httpx.Conflict(emailInUseMessage)
		}
	}

	patch := []Column{{"updated_at", s.stamp()}}
	for _, f := range coreFields {
		if !b.has(f) {
			continue
		}
		switch f {
		case "phone":
			patch = append(patch, Column{f, orEmpty(b.str(f))})
		case "is_active":
			patch = append(patch, Column{f, *b.isActive})
		default:
			patch = append(patch, Column{f, b.str(f)})
		}
	}
	if b.isAccessApp != nil {
		patch = append(patch, Column{"is_access_app", *b.isAccessApp})
	}
	if err := s.ports.Employees.Update(ctx, s.db, id, patch); err != nil {
		return nil, emailConflictOr(err)
	}

	wantsAccess := existing.IsAccessApp
	if b.isAccessApp != nil {
		wantsAccess = *b.isAccessApp
	}
	email, fullName := existing.Email, existing.FullName
	if e := b.str("email"); e != nil {
		email = *e
	}
	if n := b.str("full_name"); n != nil {
		fullName = *n
	}
	switch {
	case wantsAccess && existing.UserID == nil:
		if orEmpty(b.password) == "" || b.role == nil {
			return nil, errAccessNeedsLogin
		}
		err = s.provision(ctx, actorID, id, b, email, fullName)
	case existing.UserID != nil && wantsAccess && accounts:
		// Without Manajemen User the linked login account is never touched (email, name, role).
		err = s.syncAccount(ctx, actorID, *existing.UserID, b, email, fullName)
	case existing.UserID != nil && b.isAccessApp != nil && !*b.isAccessApp:
		s.revokeAccount(ctx, actorID, id, *existing.UserID)
	}
	if err != nil {
		return nil, err
	}
	return s.reload(ctx, id)
}

// syncAccount is syncAppAccount; the TS runs it without a transaction.
func (s *service) syncAccount(ctx context.Context, actorID, userID string, b *employeeBody, email, fullName string) error {
	// The IAM role set follows the app role only when the role changes: a save that resends the
	// same role keeps IAM roles assigned outside the app role (e.g. Admin HRIS on app role hrd).
	prevRole := profileColumn(ctx, s.db, userID, "role")
	if !b.has("email") {
		email = "" // the login email changes only when the body sends one
	}
	var upd AuthUpdate
	if email != "" {
		upd.Email = &email
	}
	if fullName != "" {
		upd.FullName = &fullName
	}
	upd.Role = b.role
	if b.accountStatus != nil {
		ban := *b.accountStatus == "inactive"
		upd.Ban = &ban
	}
	if upd != (AuthUpdate{}) {
		found, err := s.ports.Identity.UpdateUser(ctx, s.db, userID, upd)
		if err != nil {
			return emailConflictOr(err)
		}
		if !found {
			return errors.New("User not found")
		}
	}
	if pw := orEmpty(b.password); pw != "" {
		if err := s.setPassword(ctx, s.db, userID, pw); err != nil {
			return err
		}
	}

	sets := []set{{"updated_at", s.stamp()}}
	if email != "" {
		sets = append(sets, set{"email", email})
	}
	if fullName != "" {
		sets = append(sets, set{"full_name", fullName})
	}
	if b.role != nil {
		sets = append(sets, set{"role", *b.role})
	}
	if b.has("brand_id") {
		sets = append(sets, set{"brand_id", nil})
	}
	if b.has("business_scope") || b.has("holding_id") || b.has("company_id") || b.has("branch_id") || b.role != nil {
		role := "admin"
		if b.role != nil {
			role = *b.role
		} else if r := profileColumn(ctx, s.db, userID, "role"); r != nil {
			role = *r
		}
		scope := profileScope(role, b)
		sets = append(sets, set{"brand_id", nil}, set{"business_scope", scope.BusinessScope},
			set{"holding_id", scope.HoldingID}, set{"company_id", scope.CompanyID}, set{"branch_id", scope.BranchID})
	}
	if b.accountStatus != nil {
		sets = append(sets, set{"status", *b.accountStatus})
	}
	canSwitch := b.canSwitchStall
	canCentral := b.canCentralCheckout
	if canCentral != nil && *canCentral {
		on := true
		canSwitch = &on
	}
	var st *stall
	if b.has("warehouse_ids") || b.has("default_warehouse_id") {
		v := stallFields(b)
		st = &v
		sets = append(sets, set{"default_warehouse_id", v.defaultID})
		if canSwitch == nil {
			canSwitch = &v.canSwitchStall
		}
		if canCentral == nil {
			canCentral = &v.canCentralCheckout
		}
		if v.canCentralCheckout {
			on := true
			canSwitch = &on
		}
	}
	if canSwitch != nil {
		sets = append(sets, set{"can_switch_stall", *canSwitch})
	}
	if canCentral != nil {
		sets = append(sets, set{"can_central_checkout", *canCentral})
	}
	if len(sets) > 1 {
		sql, args := updateSQL("configuration.users", dedupe(sets), "")
		if _, err := s.db.Exec(ctx, sql, append(args, userID)...); err != nil {
			return emailConflictOr(err)
		}
	}

	if b.approvals != nil {
		quietly(ctx, s.db, func(q database.Querier) error { return deactivateApprovals(ctx, q, userID, actorID, s.stamp()) })
		if err := insertApprovals(ctx, s.db, userID, actorID, b.approvals); err != nil {
			return err
		}
	}
	if st != nil {
		branchID := b.branchID
		if !b.has("branch_id") {
			branchID = profileColumn(ctx, s.db, userID, "branch_id")
		}
		if err := syncWarehouses(ctx, s.db, userID, st.warehouseIDs, branchID, s.stamp()); err != nil {
			return err
		}
	}
	if b.role != nil && (prevRole == nil || *prevRole != *b.role) {
		if err := syncIamRole(ctx, s.db, userID, *b.role, actorID); err != nil {
			return err
		}
	}
	fields := b.keys(false)
	for _, k := range []string{"email", "full_name"} {
		if !b.has(k) {
			fields = append(fields, k)
		}
	}
	audit(ctx, s.db, actorID, userID, "update_user", kit.Obj("fields", fields))
	return nil
}

// dedupe keeps the last value of a column assigned twice (a JS object key
// written twice keeps its first position and its last value).
func dedupe(sets []set) []set {
	pos := map[string]int{}
	out := make([]set, 0, len(sets))
	for _, s := range sets {
		if i, ok := pos[s.col]; ok {
			out[i].val = s.val
			continue
		}
		pos[s.col] = len(out)
		out = append(out, s)
	}
	return out
}

// revokeAccount is revokeAppAccount: every step's error is ignored.
func (s *service) revokeAccount(ctx context.Context, actorID, employeeID, userID string) {
	ban := true
	quietly(ctx, s.db, func(q database.Querier) error {
		_, err := s.ports.Identity.UpdateUser(ctx, q, userID, AuthUpdate{Ban: &ban})
		return err
	})
	quietly(ctx, s.db, func(q database.Querier) error {
		_, err := q.Exec(ctx, `UPDATE configuration.users SET status = 'inactive', updated_at = $1 WHERE id = $2`, s.stamp(), userID)
		return err
	})
	quietly(ctx, s.db, func(q database.Querier) error {
		return s.ports.Employees.Update(ctx, q, employeeID, []Column{{"is_access_app", false}, {"updated_at", s.stamp()}})
	})
	clearWarehouses(ctx, s.db, userID, s.stamp())
	audit(ctx, s.db, actorID, userID, "revoke_app_access", kit.Obj("employee_id", employeeID))
}

/* ── passwords ───────────────────────────────────────────────────────── */

// setPassword is updateUserById({ password }) where the caller rethrows
// `new Error(error.message)`: any failure is a plain 500.
func (s *service) setPassword(ctx context.Context, q database.Querier, userID, password string) error {
	found, err := s.ports.Identity.UpdateUser(ctx, q, userID, AuthUpdate{Password: &password})
	if err != nil {
		return errors.New(err.Error())
	}
	if !found {
		return errors.New("User not found")
	}
	return nil
}

type resetResult struct {
	Message      string `json:"message"`
	TempPassword string `json:"tempPassword"`
}

// resetPassword is resetUserEmployeePassword.
func (s *service) resetPassword(ctx context.Context, userID string) (*resetResult, error) {
	temp, err := domain.TempPassword(s.random)
	if err != nil {
		return nil, err
	}
	if err := s.setPassword(ctx, s.db, userID, temp); err != nil {
		return nil, err
	}
	return &resetResult{Message: "Password reset successfully", TempPassword: temp}, nil
}

// resetEmployeePassword is resetEmployeeAppPassword.
func (s *service) resetEmployeePassword(ctx context.Context, employeeID string) (*resetResult, error) {
	e, err := s.ports.Employees.Access(ctx, s.db, employeeID)
	if err != nil || e == nil {
		return nil, httpx.NotFound("Employee not found")
	}
	if !e.IsAccessApp || e.UserID == nil {
		return nil, httpx.BadRequest("This employee does not have app access")
	}
	return s.resetPassword(ctx, *e.UserID)
}

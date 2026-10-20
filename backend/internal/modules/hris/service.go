package hris

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
)

// Repository is the module's storage; store implements it on PostgreSQL.
// Each area declares its part next to its use cases.
type Repository interface {
	ActorRepo
	EmployeeRepo
	AttendanceRepo
	LeaveRepo
	ScheduleRepo
	OvertimeRepo
	HolidayRepo
	ContractRepo
	ChecklistRepo
	SelfServiceRepo
	AnnouncementRepo
	LogbookRepo
	MasterRepo
	FileRepo
	PlacementRepo
	// InTx runs fn against one transaction (a savepoint when already in one).
	InTx(ctx context.Context, fn func(Repository) error) error
	// querier is the pool or transaction ports run on.
	querier() database.Querier
}

// store runs the SQL ported from lib/hris/** on a pool or a transaction.
type store struct{ db database.DB }

func newStore(db database.DB) *store { return &store{db: db} }

func (s *store) InTx(ctx context.Context, fn func(Repository) error) error {
	return database.WithTx(ctx, s.db, func(tx pgx.Tx) error { return fn(&store{db: tx}) })
}

/* ── Actor ───────────────────────────────────────────────────────────── */

// hrRoles is HR_ROLES in lib/hris/workforce-auth.
var hrRoles = map[string]bool{"super_admin": true, "admin": true, "hrd": true, "hiring_manager": true}

// Actor is WorkforceActor (plus the logbook's department): the signed-in
// account and the hris.employees record linked to it. approved_by,
// validated_by, completed_by… reference employees, so they take EmployeeID,
// never the user id.
type Actor struct {
	UserID       string
	Role         string
	FullName     string
	EmployeeID   *string
	DepartmentID *string
	IsHR         bool
	// PerformanceMenu: the account holds the HR side of the Kinerja menu, not
	// only the employee-level Review/Tugas Departemen (set by withLogbook).
	PerformanceMenu bool
}

// ActorRepo resolves the employee behind an account.
type ActorRepo interface {
	EmployeeIDByUser(ctx context.Context, userID string) (*string, error)
	EmployeeLink(ctx context.Context, userID string) (id, departmentID *string, err error)
}

// Actor builds the workforce actor of a signed-in user.
func (s *Service) Actor(ctx context.Context, u *auth.User) (*Actor, error) {
	id, dept, err := s.repo.EmployeeLink(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	return &Actor{UserID: u.ID, Role: u.Role, FullName: u.FullName, EmployeeID: id, DepartmentID: dept, IsHR: hrRoles[u.Role]}, nil
}

func (s *store) EmployeeIDByUser(ctx context.Context, userID string) (*string, error) {
	id, _, err := s.EmployeeLink(ctx, userID)
	return id, err
}

func (s *store) EmployeeLink(ctx context.Context, userID string) (*string, *string, error) {
	var id, dept *string
	err := s.db.QueryRow(ctx,
		`SELECT id::text, department_id::text FROM hris.employees WHERE user_id = $1 LIMIT 1`, userID).Scan(&id, &dept)
	if database.IsNoRows(err) {
		return nil, nil, nil
	}
	return id, dept, err
}

/* ── PostgREST-style embeds ──────────────────────────────────────────── */

// embedOne is a many-to-one embed exactly as the TS query builder writes it:
// (SELECT row_to_json(e) FROM (SELECT cols FROM table WHERE cond) e).
// cond references the outer table by its bare name, as the builder does.
func embedOne(alias, cols, table, cond string) string {
	return fmt.Sprintf(`(SELECT row_to_json(e) FROM (SELECT %s FROM %s WHERE %s) e) AS %s`, cols, table, cond, alias)
}

// embedMany is a one-to-many embed: json_agg of the child rows, [] when none.
func embedMany(alias, cols, table, cond string) string {
	return fmt.Sprintf(`COALESCE((SELECT json_agg(e) FROM (SELECT %s FROM %s WHERE %s) e), '[]'::json) AS %s`, cols, table, cond, alias)
}

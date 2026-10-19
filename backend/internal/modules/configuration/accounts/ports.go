package accounts

import (
	"context"
	"encoding/json"
	"time"

	"nuhabit/backend/internal/platform/database"
)

// Ports are the capabilities of other bounded contexts this area uses.
// Every method runs on the caller's querier so it joins its transaction.
type Ports struct {
	Identity  Identity
	Employees Employees
}

// Identity is auth.users, owned by identity: the auth.admin shim of
// lib/pg/create-client.ts.
type Identity interface {
	// CreateUser is auth.admin.createUser with email_confirm: true and the
	// password hashed with bcrypt cost 10. It returns the new id.
	CreateUser(ctx context.Context, q database.Querier, u NewAuthUser) (string, error)
	// UpdateUser is auth.admin.updateUserById. found is false when no user
	// has the id (the shim's "User not found").
	UpdateUser(ctx context.Context, q database.Querier, id string, u AuthUpdate) (found bool, err error)
	// Email is getUserById(id).user.email, "" when there is no such user.
	Email(ctx context.Context, q database.Querier, id string) (string, error)
	// LastSignIns is last_sign_in_at per id among the users listUsers pages
	// through (the 1900 newest). Ids it does not reach are missing.
	LastSignIns(ctx context.Context, q database.Querier, ids []string) (map[string]*time.Time, error)
	// Statuses is every auth user with the ban, sign-in and verification
	// timestamps as PostgreSQL text (the TS casts them ::text).
	Statuses(ctx context.Context, q database.Querier) ([]AuthStatus, error)
}

// NewAuthUser is the createUser input.
type NewAuthUser struct {
	Email, Password, FullName, Role string
}

// AuthUpdate is the updateUserById input; nil fields are left alone.
// Ban true is ban_duration "876000h", false is "none".
type AuthUpdate struct {
	Password, Email, FullName, Role *string
	Ban                             *bool
}

// AuthStatus is one auth.users row of listAdminUsers.
type AuthStatus struct {
	ID, Email                                  string
	BannedUntil, LastSignInAt, EmailVerifiedAt *string
}

// Employees is hris.employees, owned by hris.
type Employees interface {
	// List is the /api/users page: the matching count and one page of rows
	// with the department, section, job title and manager embeds.
	List(ctx context.Context, q database.Querier, f EmployeeFilter) ([]Employee, int, error)
	// Get is one row with the same embeds, nil when missing.
	Get(ctx context.Context, q database.Querier, id string) (*Employee, error)
	// Access is the id, user_id, is_access_app, email and full_name of a
	// row, nil when missing.
	Access(ctx context.Context, q database.Querier, id string) (*EmployeeAccess, error)
	// EmailTaken reports another row (id <> exceptID when set) with email.
	EmailTaken(ctx context.Context, q database.Querier, email, exceptID string) (bool, error)
	// EmailOwner is the employee holding email (case-insensitive), the
	// active one first; nil when none.
	EmailOwner(ctx context.Context, q database.Querier, email string) (*EmailOwnerRow, error)
	// ReleaseEmail sets the row's email to archive and unlinks its login
	// account; it returns the user_id it had.
	ReleaseEmail(ctx context.Context, q database.Querier, id, archive string) (*string, error)
	// NIPTaken reports a row with nip.
	NIPTaken(ctx context.Context, q database.Querier, nip string) (bool, error)
	// Insert adds a row with the given columns and returns its id.
	Insert(ctx context.Context, q database.Querier, cols []Column) (string, error)
	// Update sets the given columns on the row.
	Update(ctx context.Context, q database.Querier, id string, cols []Column) error
}

// EmailOwnerRow is the employee an email belongs to.
type EmailOwnerRow struct {
	ID, NIP, FullName string
	IsActive          bool
	EndDate           *time.Time
	UserID            *string
}

// Column is one hris.employees column and its value (nil is NULL).
type Column struct {
	Name  string
	Value any
}

// EmployeeFilter is the listUserEmployees query. Search is the raw term;
// the query builder's ilike filter wraps it in % and turns * into %.
type EmployeeFilter struct {
	Search, DepartmentID, EmploymentStatus *string
	ScheduleType                           *string // "shift" | "non_shift"
	IsActive, IsAccessApp                  *bool
	SortBy                                 string
	Asc                                    bool
	Limit, Offset                          int
}

// Employee is the hris.employees columns lib/users/user-mapper.ts reads,
// plus the embeds as PostgreSQL row_to_json text (nil when NULL).
type Employee struct {
	ID, FullName, NIP, Email, Phone, EmploymentStatus string
	UserID                                            *string
	JoinDate                                          time.Time
	EndDate, BirthDate                                *time.Time
	IsActive, IsAccessApp                             bool
	ScheduleType                                      string
	DepartmentID, SectionID, JobTitleID, ReportingTo  *string
	KTP, NPWP, Gender, MaritalStatus, Address         *string
	City, Province, PostalCode, BankName, BankAccount *string
	BPJSTK, BPJSKesehatan                             *string
	EmergencyName, EmergencyPhone, EmergencyRelation  *string
	PhotoURL, Notes                                   *string
	CreatedAt                                         time.Time
	UpdatedAt                                         *time.Time
	Department, Section, JobTitle, Manager            json.RawMessage
}

// EmployeeAccess is the slice of a row the update and reset flows read.
type EmployeeAccess struct {
	ID, Email, FullName string
	UserID              *string
	IsAccessApp         bool
}

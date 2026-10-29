package hris

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/validate"
)

// Penempatan Karyawan (hris.employee_placements): the stores
// (configuration.branches) an employee works at. One active primary
// placement per employee and no overlapping placement at the same store are
// enforced by exclusion constraints. Clock-in picks its store among the
// active placements; payroll splits the slip cost per store from them and
// the attendance store.

// placementManagers manage placements: the employee record managers and
// the HRIS → Master Data → Penempatan Karyawan menu.
var placementManagers = slices.Concat(employeeRecordManagers, iam.HrisPlacements)

// PlacementRepo is the placement storage.
type PlacementRepo interface {
	PlacementsOf(ctx context.Context, employeeID string) ([]*Row, error)
	PlacementOverview(ctx context.Context, f placementFilter) ([]*Row, error)
	// PlacementSearch is one page of the overview with the schedule status
	// (EPIC-070); PlacementIDs the matching employee ids (at most limit);
	// PlacementCounts the status chip counts, ignoring f.Status.
	PlacementSearch(ctx context.Context, f placementFilter, limit, offset int) ([]*Row, error)
	PlacementIDs(ctx context.Context, f placementFilter, limit int) ([]string, error)
	PlacementCounts(ctx context.Context, f placementFilter) (*Row, error)
	ActivePlacements(ctx context.Context, employeeID, date string) ([]*Row, error)
	PlacementByID(ctx context.Context, id string) (*Row, error)
	InsertPlacement(ctx context.Context, f fields) (*Row, error)
	UpdatePlacement(ctx context.Context, id string, f fields) (*Row, error)
	DeletePlacement(ctx context.Context, id string) (bool, error)
	ClearOtherPrimary(ctx context.Context, employeeID, exceptID, start string, end *string) error
	// PlacementEmployees are the id, full_name and nip of the employees;
	// PlacementSpans all their placements (bulk actions, EPIC-070).
	PlacementEmployees(ctx context.Context, ids []string) ([]*Row, error)
	PlacementSpans(ctx context.Context, employeeIDs []string) ([]*Row, error)
	EndPlacement(ctx context.Context, id, end string) error
	ActiveBranches(ctx context.Context) ([]*Row, error)
	BranchExists(ctx context.Context, id string) (bool, error)
	// BranchGeofence is the store's name, coordinates and radius override
	// (EPIC-062); nil when the store does not exist.
	BranchGeofence(ctx context.Context, id string) (*Row, error)
	// SaveBranchLocation writes the store coordinates and radius override
	// (EPIC-062 R9), returning the saved location; nil when not found.
	SaveBranchLocation(ctx context.Context, id string, lat, lng *float64, radius *int) (*Row, error)
	PlacementEmployeeExists(ctx context.Context, id string) (bool, error)
}

const placementColumns = `ep.id, ep.employee_id, ep.branch_id, b.code AS branch_code, b.name AS branch_name,
	ep.is_primary, ep.start_date::text AS start_date, ep.end_date::text AS end_date, ep.notes, ep.created_at, ep.updated_at`

const placementFrom = ` FROM hris.employee_placements ep JOIN configuration.branches b ON b.id = ep.branch_id`

func (s *store) PlacementsOf(ctx context.Context, employeeID string) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT `+placementColumns+placementFrom+`
		WHERE ep.employee_id = $1
		ORDER BY (ep.end_date IS NOT NULL AND ep.end_date < (now() + interval '7 hours')::date), ep.is_primary DESC,
		         ep.start_date DESC, b.name`, employeeID)
}

// placementFilter narrows the overview: Branch keeps employees placed at
// the store, Department/Position match the employee record, Search matches
// name/NIP, Status keeps one chip (placementStatuses), All includes ended
// placements in the placements column.
//
// Scope is the store delegate (EPIC-071 R3; nil for HR). It keeps the
// employees in their area: a placement running today (started, not ended,
// as in delegateOutside) at one of Scope.BranchIDs, or a place in
// ScopeSubtree. ScopeSubtree is the delegate's reporting subtree
// (delegateSubtreeIDs) and must be set together with Scope.
type placementFilter struct {
	Branch, Department, Position, Search, Status string
	All                                          bool
	Today                                        string
	Scope                                        *delegateAccess
	ScopeSubtree                                 []string
}

// The overview conditions; $1 is always Today. A placement is running when
// it has not ended by today. Schedule (R5): a per-date row or weekly
// pattern in effect today..today+6; only Shift employees need one.
const (
	placementRunning  = `(ep.end_date IS NULL OR ep.end_date >= $1::date)`
	placementPlaced   = `EXISTS (SELECT 1 FROM hris.employee_placements ep WHERE ep.employee_id = e.id AND ` + placementRunning + `)`
	placementMulti    = `(SELECT count(*) FROM hris.employee_placements ep WHERE ep.employee_id = e.id AND ` + placementRunning + `) > 1`
	placementSchedule = `EXISTS (SELECT 1 FROM hris.v_employee_schedule es WHERE es.employee_id = e.id
		AND es.effective_from <= $1::date + 6 AND (es.effective_to IS NULL OR es.effective_to >= $1::date))`
	placementNoSchedule = `(e.schedule_type = 'shift' AND NOT ` + placementSchedule + `)`
)

// placementStatuses are the status filters of the search page.
var placementStatuses = map[string]string{
	"":            "true",
	"unplaced":    "NOT " + placementPlaced,
	"no_schedule": placementNoSchedule,
	"multi":       placementMulti,
	"placed":      placementPlaced,
}

// placementWhere is the employee condition shared by the overview, search,
// ids and counts, with its arguments ($1 = Today).
func placementWhere(f placementFilter) (string, []any) {
	// $1::date types today when no filter compares with it (PostgreSQL
	// rejects an untyped unused parameter).
	where := []string{"e.is_active", "$1::date IS NOT NULL"}
	args := []any{f.Today}
	if f.Branch != "" {
		args = append(args, f.Branch)
		where = append(where, fmt.Sprintf(`EXISTS (SELECT 1 FROM hris.employee_placements ep
			WHERE ep.employee_id = e.id AND ep.branch_id = $%d AND %s)`, len(args), placementRunning))
	}
	if f.Department != "" {
		args = append(args, f.Department)
		where = append(where, fmt.Sprintf(`e.department_id = $%d`, len(args)))
	}
	if f.Position != "" {
		args = append(args, f.Position)
		where = append(where, fmt.Sprintf(`e.job_title_id = $%d`, len(args)))
	}
	if q := strings.TrimSpace(f.Search); q != "" {
		args = append(args, "%"+q+"%")
		where = append(where, fmt.Sprintf(`(e.full_name ILIKE $%d OR e.nip ILIKE $%d)`, len(args), len(args)))
	}
	if f.Status != "" {
		where = append(where, placementStatuses[f.Status])
	}
	if f.Scope != nil {
		subtree := f.ScopeSubtree
		if subtree == nil {
			subtree = []string{} // an empty array, not NULL
		}
		args = append(args, f.Scope.BranchIDs, subtree)
		// Running as in delegateOutside (R3): started by today, not ended.
		where = append(where, fmt.Sprintf(`(EXISTS (SELECT 1 FROM hris.employee_placements ep
			WHERE ep.employee_id = e.id AND ep.branch_id = ANY($%d::uuid[])
			  AND ep.start_date <= $1::date AND %s) OR e.id = ANY($%d::uuid[]))`,
			len(args)-1, placementRunning, len(args)))
	}
	return strings.Join(where, " AND "), args
}

// placementEmployeeSelect is the overview row: the employee with the
// running (or, with All, every) placements.
func placementEmployeeSelect(f placementFilter) string {
	placementCond := placementRunning
	if f.All {
		placementCond = "true"
	}
	return `SELECT e.id AS employee_id, e.full_name, e.nip, e.photo_url,
		d.name AS department_name, p.title AS position_title,
		COALESCE((SELECT json_agg(x ORDER BY x.is_primary DESC, x.branch_name) FROM (
		   SELECT ` + placementColumns + placementFrom + `
		    WHERE ep.employee_id = e.id AND ` + placementCond + `) x), '[]'::json) AS placements`
}

const placementEmployeeFrom = `
		FROM hris.employees e
		LEFT JOIN hris.departments d ON d.id = e.department_id
		LEFT JOIN hris.positions p ON p.id = e.job_title_id`

func (s *store) PlacementOverview(ctx context.Context, f placementFilter) ([]*Row, error) {
	where, args := placementWhere(f)
	return queryRows(ctx, s.db, placementEmployeeSelect(f)+placementEmployeeFrom+`
		WHERE `+where+`
		ORDER BY e.full_name`, args...)
}

// PlacementSearch adds the record ids and schedule status to the overview
// row: has_schedule (R5) and schedule_until, the last per-date row from
// today on.
func (s *store) PlacementSearch(ctx context.Context, f placementFilter, limit, offset int) ([]*Row, error) {
	where, args := placementWhere(f)
	args = append(args, limit, offset)
	return queryRows(ctx, s.db, placementEmployeeSelect(f)+`,
		e.department_id, e.job_title_id AS position_id, e.schedule_type,
		`+placementSchedule+` AS has_schedule,
		(SELECT max(sd.work_date)::text FROM hris.employee_shift_days sd
		  WHERE sd.employee_id = e.id AND sd.work_date >= $1::date) AS schedule_until`+placementEmployeeFrom+`
		WHERE `+where+fmt.Sprintf(`
		ORDER BY e.full_name, e.id
		LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
}

func (s *store) PlacementIDs(ctx context.Context, f placementFilter, limit int) ([]string, error) {
	where, args := placementWhere(f)
	args = append(args, limit)
	rows, err := queryRows(ctx, s.db, `SELECT e.id FROM hris.employees e WHERE `+where+
		fmt.Sprintf(` ORDER BY e.full_name, e.id LIMIT $%d`, len(args)), args...)
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.Str("id"))
	}
	return ids, err
}

func (s *store) PlacementCounts(ctx context.Context, f placementFilter) (*Row, error) {
	f.Status = ""
	where, args := placementWhere(f)
	return queryRow(ctx, s.db, `SELECT count(*)::int AS "all",
		count(*) FILTER (WHERE `+placementStatuses["unplaced"]+`)::int AS unplaced,
		count(*) FILTER (WHERE `+placementStatuses["no_schedule"]+`)::int AS no_schedule,
		count(*) FILTER (WHERE `+placementStatuses["multi"]+`)::int AS multi
		FROM hris.employees e WHERE `+where, args...)
}

func (s *store) ActivePlacements(ctx context.Context, employeeID, date string) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT ep.id, ep.branch_id, b.code AS branch_code, b.name AS branch_name,
		ep.is_primary, b.lat, b.lng
		`+placementFrom+`
		WHERE ep.employee_id = $1 AND b.is_active
		  AND ep.start_date <= $2::date AND (ep.end_date IS NULL OR ep.end_date >= $2::date)
		ORDER BY ep.is_primary DESC, b.name`, employeeID, date)
}

func (s *store) PlacementByID(ctx context.Context, id string) (*Row, error) {
	return queryRow(ctx, s.db, `SELECT `+placementColumns+placementFrom+` WHERE ep.id = $1`, id)
}

func (s *store) InsertPlacement(ctx context.Context, f fields) (*Row, error) {
	row, err := insertRow(ctx, s.db, "hris.employee_placements", f, "id")
	if err != nil || row == nil {
		return nil, err
	}
	return s.PlacementByID(ctx, row.Str("id"))
}

func (s *store) UpdatePlacement(ctx context.Context, id string, f fields) (*Row, error) {
	if _, err := updateOne(ctx, s.db, "hris.employee_placements", f, fields{{"id", id}}, "id"); err != nil {
		return nil, err
	}
	return s.PlacementByID(ctx, id)
}

func (s *store) DeletePlacement(ctx context.Context, id string) (bool, error) {
	tag, err := s.db.Exec(ctx, `DELETE FROM hris.employee_placements WHERE id = $1`, id)
	return tag.RowsAffected() > 0, err
}

// ClearOtherPrimary drops the primary flag of the employee's other
// placements overlapping [start, end] (a new primary replaces them).
func (s *store) ClearOtherPrimary(ctx context.Context, employeeID, exceptID, start string, end *string) error {
	_, err := s.db.Exec(ctx, `UPDATE hris.employee_placements SET is_primary = false
		WHERE employee_id = $1 AND is_primary AND ($2 = '' OR id::text <> $2)
		  AND daterange(start_date, end_date, '[]') && daterange($3::date, $4::date, '[]')`,
		employeeID, exceptID, start, end)
	return err
}

func (s *store) PlacementEmployees(ctx context.Context, ids []string) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT id::text AS id, full_name, nip FROM hris.employees
		WHERE id = ANY($1::text[]::uuid[])`, ids)
}

func (s *store) PlacementSpans(ctx context.Context, employeeIDs []string) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT ep.id::text AS id, ep.employee_id::text AS employee_id, ep.branch_id::text AS branch_id,
		b.name AS branch_name, ep.start_date::text AS start_date, ep.end_date::text AS end_date, ep.is_primary`+placementFrom+`
		WHERE ep.employee_id = ANY($1::text[]::uuid[])
		ORDER BY ep.start_date, b.name`, employeeIDs)
}

func (s *store) EndPlacement(ctx context.Context, id, end string) error {
	_, err := s.db.Exec(ctx, `UPDATE hris.employee_placements SET end_date = $2::date WHERE id = $1`, id, end)
	return err
}

func (s *store) ActiveBranches(ctx context.Context) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT b.id, b.code, b.name, b.company_id, c.name AS company_name, b.lat, b.lng,
		b.geofence_radius_m, b.min_work_hours
		FROM configuration.branches b JOIN configuration.companies c ON c.id = b.company_id
		WHERE b.is_active
		ORDER BY c.name, b.name`)
}

func (s *store) BranchExists(ctx context.Context, id string) (bool, error) {
	var ok bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM configuration.branches WHERE id::text = $1)`, id).Scan(&ok)
	return ok, err
}

func (s *store) BranchGeofence(ctx context.Context, id string) (*Row, error) {
	row, err := queryRow(ctx, s.db, `SELECT name, lat, lng, geofence_radius_m, min_work_hours FROM configuration.branches WHERE id::text = $1`, id)
	return row, ignoreBadInput(err)
}

func (s *store) SaveBranchLocation(ctx context.Context, id string, lat, lng *float64, radius *int) (*Row, error) {
	return queryRow(ctx, s.db, `UPDATE configuration.branches
		SET lat = $2, lng = $3, geofence_radius_m = $4, updated_at = now()
		WHERE id = $1 RETURNING id, code, name, lat, lng, geofence_radius_m`, id, lat, lng, radius)
}

func (s *store) PlacementEmployeeExists(ctx context.Context, id string) (bool, error) {
	var ok bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM hris.employees WHERE id::text = $1)`, id).Scan(&ok)
	return ok, err
}

/* ── Use cases ───────────────────────────────────────────────────────── */

// placementInput is the create/update body. Sent tells which keys a PATCH
// carried.
type placementInput struct {
	BranchID  opt[string]
	IsPrimary opt[bool]
	StartDate opt[string]
	EndDate   opt[string]
	Notes     opt[string]
}

var dateOpts = validate.StrOpts{Check: func(s string) (string, string, bool) {
	return "invalid_format", "Format tanggal harus YYYY-MM-DD", domain.DateRe.MatchString(s)
}}

func parsePlacement(f *validate.Form) placementInput {
	return placementInput{
		BranchID:  str(f, "branch_id", optional, validate.StrOpts{Check: validate.UUIDCheck}),
		IsPrimary: boolean(f, "is_primary", optional),
		StartDate: str(f, "start_date", optional, dateOpts),
		EndDate:   str(f, "end_date", nullish, dateOpts),
		Notes:     str(f, "notes", nullish, validate.StrOpts{Max: 500}),
	}
}

// placementError maps the constraint violations to the HR-facing messages.
func placementError(err error) error {
	var pg *pgconn.PgError
	if !errors.As(err, &pg) {
		return err
	}
	switch {
	case pg.ConstraintName == "employee_placements_no_overlap":
		return httpx.BadRequest("Karyawan sudah ditempatkan di toko ini pada rentang tanggal yang sama")
	case pg.ConstraintName == "employee_placements_one_primary":
		return httpx.BadRequest("Hanya boleh ada satu penempatan utama pada rentang tanggal yang sama")
	case pg.ConstraintName == "employee_placements_dates_check":
		return httpx.BadRequest("Tanggal selesai tidak boleh sebelum tanggal mulai")
	case pg.Code == "23503":
		return httpx.BadRequest("Toko atau karyawan tidak ditemukan")
	}
	return err
}

// CreatePlacement places an employee at a store. A new primary placement
// takes the primary flag from the overlapping ones.
func (s *Service) CreatePlacement(ctx context.Context, employeeID, userID string, in placementInput) (*Row, error) {
	if in.BranchID.Val == nil {
		return nil, httpx.BadRequest("Toko wajib dipilih")
	}
	start := strOr(in.StartDate.Val)
	if start == "" {
		start = domain.TodayWIB(s.now())
	}
	var row *Row
	err := s.repo.InTx(ctx, func(r Repository) error {
		ok, err := r.PlacementEmployeeExists(ctx, employeeID)
		if err != nil {
			return err
		}
		if !ok {
			return httpx.NotFound("Karyawan tidak ditemukan")
		}
		primary := in.IsPrimary.Val != nil && *in.IsPrimary.Val
		if primary {
			if err := r.ClearOtherPrimary(ctx, employeeID, "", start, in.EndDate.Val); err != nil {
				return err
			}
		}
		var f fields
		f.add("employee_id", employeeID)
		f.add("branch_id", *in.BranchID.Val)
		f.add("is_primary", primary)
		f.add("start_date", start)
		f.add("end_date", nilIfEmpty(in.EndDate.Val))
		f.add("notes", nilIfEmpty(in.Notes.Val))
		f.add("created_by", nilIfEmpty(&userID))
		row, err = r.InsertPlacement(ctx, f)
		return err
	})
	return row, placementError(err)
}

// UpdatePlacement edits the store, dates, notes or primary flag.
func (s *Service) UpdatePlacement(ctx context.Context, id string, in placementInput) (*Row, error) {
	var row *Row
	err := s.repo.InTx(ctx, func(r Repository) error {
		cur, err := r.PlacementByID(ctx, id)
		if err != nil {
			return ignoreBadInput(err)
		}
		if cur == nil {
			return httpx.NotFound("Penempatan tidak ditemukan")
		}
		var f fields
		if in.BranchID.Sent && in.BranchID.Val != nil {
			f.add("branch_id", *in.BranchID.Val)
		}
		start, end := cur.Str("start_date"), cur.StrPtr("end_date")
		if in.StartDate.Sent && in.StartDate.Val != nil {
			start = *in.StartDate.Val
			f.add("start_date", start)
		}
		if in.EndDate.Sent {
			end = nilIfEmpty(in.EndDate.Val)
			f.add("end_date", end)
		}
		if in.Notes.Sent {
			f.add("notes", nilIfEmpty(in.Notes.Val))
		}
		if in.IsPrimary.Sent && in.IsPrimary.Val != nil {
			if *in.IsPrimary.Val {
				if err := r.ClearOtherPrimary(ctx, cur.Str("employee_id"), id, start, end); err != nil {
					return err
				}
			}
			f.add("is_primary", *in.IsPrimary.Val)
		}
		if len(f) == 0 {
			row = cur
			return nil
		}
		row, err = r.UpdatePlacement(ctx, id, f)
		return err
	})
	return row, placementError(err)
}

func (s *Service) DeletePlacement(ctx context.Context, id string) error {
	found, err := s.repo.DeletePlacement(ctx, id)
	if err != nil {
		return ignoreBadInput(err)
	}
	if !found {
		return httpx.NotFound("Penempatan tidak ditemukan")
	}
	return nil
}

// BulkPlacement places several employees at one store; employees already
// placed there in the range are skipped.
func (s *Service) BulkPlacement(ctx context.Context, userID, branchID string, employeeIDs []string, primary bool, start string, end *string) (created, skipped int, err error) {
	if start == "" {
		start = domain.TodayWIB(s.now())
	}
	for _, id := range employeeIDs {
		_, err := s.CreatePlacement(ctx, id, userID, placementInput{
			BranchID: opt[string]{Val: &branchID, Sent: true}, IsPrimary: opt[bool]{Val: &primary, Sent: true},
			StartDate: opt[string]{Val: &start, Sent: true}, EndDate: opt[string]{Val: end, Sent: end != nil}})
		var he *httpx.Error
		switch {
		case err == nil:
			created++
		case errors.As(err, &he) && he.Status < 500:
			skipped++
		default:
			return created, skipped, err
		}
	}
	return created, skipped, nil
}

// Bulk placement actions with preview (EPIC-070): move (R4), add and end
// placements of up to 500 employees. Commit runs every employee in one
// transaction; preview runs the same writes and rolls them back, so it
// answers exactly what commit would do.

const (
	bulkMove = "move"
	bulkAdd  = "add"

	bulkPreview = "preview"
	bulkCommit  = "commit"
)

// errBulkPreview rolls the preview transaction back.
var errBulkPreview = errors.New("bulk placement preview")

// bulkPlacementRequest is a bulk move/add (mode set). Scope is the store
// delegate acting (EPIC-071 R6), nil for HR.
type bulkPlacementRequest struct {
	Mode, Step, BranchID string
	EmployeeIDs          []string
	Primary              bool
	Start                string
	End, Notes           *string
	Scope                *delegateAccess
}

// The per-employee skip reasons of a store delegate's bulk actions (R6).
const (
	reasonForeignPlacement = "Punya penempatan di toko lain di luar wilayah Anda — hubungi HR"
	reasonNoStorePlacement = "Tidak ada penempatan di toko Anda"
)

// delegateSubtreeIDs lists the delegate's reporting subtree (delegateSubtree:
// active employees, at most 10 levels, the delegate excluded); never nil.
func delegateSubtreeIDs(ctx context.Context, q database.Querier, d *delegateAccess) ([]string, error) {
	rows, err := q.Query(ctx, delegateSubtree+` SELECT id::text FROM sub`, d.EmployeeID)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if ids == nil {
		ids = []string{}
	}
	return ids, err
}

// checkDelegateEmployees refuses (403) a delegate's bulk request when any of
// the employees is outside their area (R3), before anything about the
// employees is read, so nothing of an outside employee is answered.
func (s *Service) checkDelegateEmployees(ctx context.Context, q database.Querier, d *delegateAccess, ids []string) error {
	out, err := delegateOutside(ctx, q, d, domain.TodayWIB(s.now()), uniqueIDs(ids))
	if err != nil {
		return err
	}
	if len(out) > 0 {
		return httpx.Forbidden(fmt.Sprintf("%d karyawan di luar wilayah Anda", len(out)))
	}
	return nil
}

// foreignPlacement reports whether a delegate's bulk action would write to
// a placement of the employee at a store outside their area (R6), unless
// the employee is in the delegate's subtree. A move ends every placement
// running on start; a primary add clears the primary flag of the primary
// placements overlapping [start, end] (ClearOtherPrimary). A non-primary
// add writes nothing but the new placement.
func foreignPlacement(sc storeScope, subtree map[string]bool, in bulkPlacementRequest, id string, spans []domain.PlacementSpan, start string) bool {
	if sc.Delegate == nil || subtree[id] {
		return false
	}
	return slices.ContainsFunc(spans, func(p domain.PlacementSpan) bool {
		if sc.AllowsBranch(p.BranchID) || !p.RunningOn(start) {
			return false
		}
		if in.Mode == bulkMove {
			return true
		}
		return in.Primary && p.Primary && (in.End == nil || p.Start <= *in.End)
	})
}

// bulkPlacementTarget is an employee of a bulk action with their placements.
type bulkPlacementTarget struct {
	emp   *Row
	spans []domain.PlacementSpan
}

// uniqueIDs lowercases (as PostgreSQL reads the ids back) and deduplicates
// ids, keeping the order.
func uniqueIDs(ids []string) []string {
	uniq := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.ToLower(id)
		if !seen[id] {
			seen[id] = true
			uniq = append(uniq, id)
		}
	}
	return uniq
}

// loadBulkPlacements reads the employees (nil when not found) and their
// placements; ids are deduplicated, keeping the order.
func loadBulkPlacements(ctx context.Context, r Repository, ids []string) ([]string, map[string]*bulkPlacementTarget, error) {
	uniq := uniqueIDs(ids)
	emps, err := r.PlacementEmployees(ctx, uniq)
	if err != nil {
		return nil, nil, err
	}
	out := map[string]*bulkPlacementTarget{}
	for _, e := range emps {
		out[e.Str("id")] = &bulkPlacementTarget{emp: e}
	}
	spans, err := r.PlacementSpans(ctx, uniq)
	if err != nil {
		return nil, nil, err
	}
	for _, p := range spans {
		if t := out[p.Str("employee_id")]; t != nil {
			t.spans = append(t.spans, domain.PlacementSpan{ID: p.Str("id"), BranchID: p.Str("branch_id"),
				BranchName: p.Str("branch_name"), Start: p.Str("start_date"), End: p.StrPtr("end_date"), Primary: p.Bool("is_primary")})
		}
	}
	return uniq, out, nil
}

// bulkPlacementResult is one employee's line of the response.
func bulkPlacementResult(id string, t *bulkPlacementTarget, outcome, reason string, ended []domain.PlacementSpan, endDate string) *Row {
	var name, nip any
	if t != nil {
		name, nip = t.emp.Get("full_name"), t.emp.Get("nip")
	}
	endedRows := make([]*Row, 0, len(ended))
	for _, p := range ended {
		endedRows = append(endedRows, obj("branch_name", p.BranchName, "end_date", endDate))
	}
	return obj("employee_id", id, "name", name, "nip", nip, "outcome", outcome, "reason", nilIfEmpty(&reason), "ended", endedRows)
}

// runningBranches names the placements running on date (audit "before").
func runningBranches(spans []domain.PlacementSpan, date string) []string {
	names := []string{}
	for _, p := range spans {
		if p.RunningOn(date) {
			names = append(names, p.BranchName)
		}
	}
	return names
}

// bulkCount is one part of a bulk message: "3 dipindahkan".
type bulkCount struct {
	n     int
	label string
}

// bulkPlacementMessage joins the non-zero counts: "N dipindahkan, M
// ditambahkan, K dilewati".
func bulkPlacementMessage(counts ...bulkCount) string {
	parts := []string{}
	for _, c := range counts {
		if c.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", c.n, c.label))
		}
	}
	return strings.Join(parts, ", ")
}

// runBulkPlacement runs fn in one transaction, rolled back for a preview.
func (s *Service) runBulkPlacement(ctx context.Context, step string, fn func(Repository) error) error {
	err := s.repo.InTx(ctx, func(r Repository) error {
		if err := fn(r); err != nil {
			return err
		}
		if step == bulkPreview {
			return errBulkPreview
		}
		return nil
	})
	if errors.Is(err, errBulkPreview) {
		return nil
	}
	return placementError(err)
}

// BulkPlacementPlan moves (R4) or adds the employees to a store; see the
// domain planners for the skip rules. Each processed employee is audited.
func (s *Service) BulkPlacementPlan(ctx context.Context, actor *auth.User, in bulkPlacementRequest) (*Row, error) {
	today := domain.TodayWIB(s.now())
	start := in.Start
	if start == "" {
		start = today
	}
	primary := in.Primary || in.Mode == bulkMove
	if in.Scope != nil {
		if start < today {
			return nil, httpx.BadRequest(errStartBeforeToday)
		}
		if in.End != nil && *in.End < domain.AddDaysISO(today, -1) {
			return nil, httpx.BadRequest(errEndBeforeYesterday)
		}
	}
	if in.End != nil && *in.End < start {
		return nil, httpx.BadRequest("Tanggal selesai tidak boleh sebelum tanggal mulai")
	}
	action := "placement.bulk_add"
	if in.Mode == bulkMove {
		action = "placement.bulk_move"
	}
	sc := storeScope{Delegate: in.Scope}
	if !sc.AllowsBranch(strings.ToLower(in.BranchID)) {
		return nil, httpx.Forbidden(errOutsideScope)
	}
	var out *Row
	err := s.runBulkPlacement(ctx, in.Step, func(r Repository) error {
		subtree := map[string]bool{}
		if in.Scope != nil {
			if err := s.checkDelegateEmployees(ctx, r.querier(), in.Scope, in.EmployeeIDs); err != nil {
				return err
			}
		}
		if in.Scope != nil && primary { // a move is always primary
			ids, err := delegateSubtreeIDs(ctx, r.querier(), in.Scope)
			if err != nil {
				return err
			}
			for _, id := range ids {
				subtree[id] = true
			}
		}
		branch, err := r.BranchGeofence(ctx, in.BranchID)
		if err != nil {
			return err
		}
		if branch == nil {
			return httpx.NotFound("Toko tidak ditemukan")
		}
		ids, targets, err := loadBulkPlacements(ctx, r, in.EmployeeIDs)
		if err != nil {
			return err
		}
		results := make([]*Row, 0, len(ids))
		counts := map[string]int{}
		for _, id := range ids {
			t := targets[id]
			if t == nil {
				counts[domain.PlacementSkipped]++
				results = append(results, bulkPlacementResult(id, nil, domain.PlacementSkipped, "Karyawan tidak ditemukan", nil, ""))
				continue
			}
			var plan domain.MoveDecision
			if foreignPlacement(sc, subtree, in, id, t.spans, start) {
				plan = domain.MoveDecision{Outcome: domain.PlacementSkipped, Reason: reasonForeignPlacement}
			} else if in.Mode == bulkMove {
				plan = domain.PlanPlacementMove(t.spans, in.BranchID, start)
			} else if reason := domain.PlanPlacementAdd(t.spans, in.BranchID, start, in.End); reason != "" {
				plan = domain.MoveDecision{Outcome: domain.PlacementSkipped, Reason: reason}
			} else {
				plan = domain.MoveDecision{Outcome: domain.PlacementCreated}
			}
			counts[plan.Outcome]++
			results = append(results, bulkPlacementResult(id, t, plan.Outcome, plan.Reason, plan.End, plan.EndDate))
			if plan.Outcome == domain.PlacementSkipped {
				continue
			}
			for _, p := range plan.End {
				if err := r.EndPlacement(ctx, p.ID, plan.EndDate); err != nil {
					return err
				}
			}
			if primary {
				if err := r.ClearOtherPrimary(ctx, id, "", start, in.End); err != nil {
					return err
				}
			}
			var f fields
			f.add("employee_id", id)
			f.add("branch_id", in.BranchID)
			f.add("is_primary", primary)
			f.add("start_date", start)
			f.add("end_date", nilIfEmpty(in.End))
			f.add("notes", nilIfEmpty(in.Notes))
			f.add("created_by", nilIfEmpty(&actor.ID))
			if _, err := r.InsertPlacement(ctx, f); err != nil {
				return err
			}
			after := map[string]any{"branch": branch.Str("name"), "start_date": start, "is_primary": primary}
			if in.End != nil {
				after["end_date"] = *in.End
			}
			if err := writeEmployeeAudit(ctx, r.querier(), actor, action, id, t.emp.Str("full_name"),
				map[string]any{"branches": runningBranches(t.spans, start)}, after); err != nil {
				return err
			}
		}
		moved, created, skipped := counts[domain.PlacementMoved], counts[domain.PlacementCreated], counts[domain.PlacementSkipped]
		out = dataMsg(obj("step", in.Step, "created", created, "moved", moved, "skipped", skipped, "results", results),
			bulkPlacementMessage(bulkCount{moved, "dipindahkan"}, bulkCount{created, "ditambahkan"}, bulkCount{skipped, "dilewati"}))
		return nil
	})
	return out, err
}

// BulkEndPlacement ends the running placements (all, or only at branchID)
// of the employees on end; see domain.PlanPlacementEnd for the skips. A
// store delegate (scope, nil for HR; EPIC-071 R6) only names employees and
// a store of their area, only ends placements at their stores (an employee
// with none to end there is skipped) and cannot end before yesterday.
func (s *Service) BulkEndPlacement(ctx context.Context, actor *auth.User, step string, employeeIDs []string, end, branchID string, scope *delegateAccess) (*Row, error) {
	branchID = strings.ToLower(branchID) // as the stored ids compare
	if scope != nil && end < domain.AddDaysISO(domain.TodayWIB(s.now()), -1) {
		return nil, httpx.BadRequest(errEndBeforeYesterday)
	}
	sc := storeScope{Delegate: scope}
	if branchID != "" && !sc.AllowsBranch(branchID) {
		return nil, httpx.Forbidden(errOutsideScope)
	}
	var out *Row
	err := s.runBulkPlacement(ctx, step, func(r Repository) error {
		if scope != nil {
			if err := s.checkDelegateEmployees(ctx, r.querier(), scope, employeeIDs); err != nil {
				return err
			}
		}
		if branchID != "" {
			branch, err := r.BranchGeofence(ctx, branchID)
			if err != nil {
				return err
			}
			if branch == nil {
				return httpx.NotFound("Toko tidak ditemukan")
			}
		}
		ids, targets, err := loadBulkPlacements(ctx, r, employeeIDs)
		if err != nil {
			return err
		}
		results := make([]*Row, 0, len(ids))
		counts := map[string]int{}
		for _, id := range ids {
			t := targets[id]
			if t == nil {
				counts[domain.PlacementSkipped]++
				results = append(results, bulkPlacementResult(id, nil, domain.PlacementSkipped, "Karyawan tidak ditemukan", nil, ""))
				continue
			}
			spans := t.spans
			if scope != nil {
				spans = slices.DeleteFunc(slices.Clone(spans), func(p domain.PlacementSpan) bool { return !sc.AllowsBranch(p.BranchID) })
			}
			plan := domain.PlanPlacementEnd(spans, branchID, end)
			if scope != nil && plan.Outcome == domain.PlacementSkipped && !slices.ContainsFunc(spans, func(p domain.PlacementSpan) bool {
				return p.End == nil || *p.End > end
			}) {
				plan.Reason = reasonNoStorePlacement
			}
			counts[plan.Outcome]++
			results = append(results, bulkPlacementResult(id, t, plan.Outcome, plan.Reason, plan.End, end))
			if plan.Outcome == domain.PlacementSkipped {
				continue
			}
			names := make([]string, 0, len(plan.End))
			for _, p := range plan.End {
				if err := r.EndPlacement(ctx, p.ID, end); err != nil {
					return err
				}
				names = append(names, p.BranchName)
			}
			if err := writeEmployeeAudit(ctx, r.querier(), actor, "placement.bulk_end", id, t.emp.Str("full_name"),
				map[string]any{"branches": names}, map[string]any{"ended": names, "end_date": end}); err != nil {
				return err
			}
		}
		ended, skipped := counts[domain.PlacementEnded], counts[domain.PlacementSkipped]
		out = dataMsg(obj("step", step, "ended", ended, "skipped", skipped, "results", results),
			bulkPlacementMessage(bulkCount{ended, "diakhiri"}, bulkCount{skipped, "dilewati"}))
		return nil
	})
	return out, err
}

// ClockPlacements is the store choice of a clock-in: the active placements
// on date (primary first) and the default store (primary, else the only or
// first placement); nil default without any placement.
func (s *Service) ClockPlacements(ctx context.Context, employeeID, date string) ([]*Row, *string, error) {
	rows, err := s.repo.ActivePlacements(ctx, employeeID, date)
	if err != nil || len(rows) == 0 {
		return rows, nil, err
	}
	def := rows[0].StrPtr("branch_id") // primary sorts first
	return rows, def, nil
}

// clockBranch resolves the store of a clock-in. A requested store must be
// one of the employee's active placements (HR may record any store when the
// employee has none); without a request the default store applies.
func (s *Service) clockBranch(ctx context.Context, a *Actor, employeeID, date string, requested *string) (*string, error) {
	rows, def, err := s.ClockPlacements(ctx, employeeID, date)
	if err != nil {
		return nil, err
	}
	if requested == nil || *requested == "" {
		return def, nil
	}
	for _, r := range rows {
		if r.Str("branch_id") == *requested {
			return requested, nil
		}
	}
	if len(rows) == 0 && a.IsHR {
		ok, err := s.repo.BranchExists(ctx, *requested)
		if err != nil {
			return nil, err
		}
		if ok {
			return requested, nil
		}
	}
	return nil, httpx.BadRequest("Toko yang dipilih bukan penempatan aktif karyawan ini")
}

/* ── HTTP ────────────────────────────────────────────────────────────── */

// GET /api/hris/employee-placements?branch_id&unplaced=true&q&include_ended=true
func (h handlers) listPlacementOverview(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	q := r.URL.Query()
	branch := q.Get("branch_id")
	if branch != "" && !domain.IsUUID(branch) {
		return httpx.BadRequest("ID toko tidak valid")
	}
	f := placementFilter{Branch: branch, Search: q.Get("q"), All: q.Get("include_ended") == "true", Today: domain.TodayWIB(h.svc.now())}
	if q.Get("unplaced") == "true" {
		f.Status = "unplaced"
	}
	rows, err := h.svc.repo.PlacementOverview(r.Context(), f)
	if err != nil {
		return err
	}
	return ok(w, dataOf(rows))
}

// placementSearchFilter reads the search/ids filter:
// q&branch_id&department_id&position_id&status&include_ended. For a store
// delegate (EPIC-071 R3) it is limited to their area, and a branch_id
// outside it is refused before anything is read.
func (h handlers) placementSearchFilter(ctx context.Context, q url.Values, sc storeScope) (placementFilter, error) {
	f := placementFilter{Branch: q.Get("branch_id"), Department: q.Get("department_id"), Position: q.Get("position_id"),
		Search: q.Get("q"), Status: q.Get("status"), All: q.Get("include_ended") == "true", Today: domain.TodayWIB(h.svc.now())}
	for _, id := range []struct{ val, msg string }{
		{f.Branch, "ID toko tidak valid"}, {f.Department, "ID departemen tidak valid"}, {f.Position, "ID jabatan tidak valid"},
	} {
		if id.val != "" && !domain.IsUUID(id.val) {
			return f, httpx.BadRequest(id.msg)
		}
	}
	if _, known := placementStatuses[f.Status]; !known {
		return f, httpx.BadRequest("Status tidak valid")
	}
	if sc.Delegate != nil {
		if f.Branch != "" && !sc.AllowsBranch(strings.ToLower(f.Branch)) {
			return f, httpx.Forbidden(errOutsideScope)
		}
		subtree, err := delegateSubtreeIDs(ctx, h.svc.repo.querier(), sc.Delegate)
		if err != nil {
			return f, err
		}
		f.Scope, f.ScopeSubtree = sc.Delegate, subtree
	}
	return f, nil
}

// placementTotal is the number of employees matching the status, read
// from the chip counts (placed = all - unplaced).
func placementTotal(counts *Row, status string) int {
	switch status {
	case "":
		return int(counts.Num("all"))
	case "placed":
		return int(counts.Num("all") - counts.Num("unplaced"))
	}
	return int(counts.Num(status))
}

// GET /api/hris/employee-placements/search?q&branch_id&department_id&position_id&status&page&page_size&include_ended
// → { rows, total, page, page_size, counts: { all, unplaced, no_schedule, multi } }
func (h handlers) searchPlacements(w http.ResponseWriter, r *http.Request, _ *auth.User, sc storeScope) error {
	q := r.URL.Query()
	f, err := h.placementSearchFilter(r.Context(), q, sc)
	if err != nil {
		return err
	}
	page, size := 1, 25
	if v := q.Get("page"); v != "" {
		if page, err = strconv.Atoi(v); err != nil || page < 1 || page > 100000 {
			return httpx.BadRequest("Halaman tidak valid")
		}
	}
	if v := q.Get("page_size"); v != "" {
		if size, err = strconv.Atoi(v); err != nil || (size != 25 && size != 50 && size != 100) {
			return httpx.BadRequest("Jumlah per halaman harus 25, 50, atau 100")
		}
	}
	counts, err := h.svc.repo.PlacementCounts(r.Context(), f)
	if err != nil {
		return err
	}
	rows, err := h.svc.repo.PlacementSearch(r.Context(), f, size, (page-1)*size)
	if err != nil {
		return err
	}
	return ok(w, dataOf(obj("rows", rows, "total", placementTotal(counts, f.Status), "page", page, "page_size", size,
		"counts", counts)))
}

// placementIDLimit caps "pilih semua hasil filter".
const placementIDLimit = 2000

// GET /api/hris/employee-placements/ids?<search filter> → { ids, total, truncated }
func (h handlers) placementIDs(w http.ResponseWriter, r *http.Request, _ *auth.User, sc storeScope) error {
	f, err := h.placementSearchFilter(r.Context(), r.URL.Query(), sc)
	if err != nil {
		return err
	}
	counts, err := h.svc.repo.PlacementCounts(r.Context(), f)
	if err != nil {
		return err
	}
	ids, err := h.svc.repo.PlacementIDs(r.Context(), f, placementIDLimit)
	if err != nil {
		return err
	}
	total := placementTotal(counts, f.Status)
	return ok(w, dataOf(obj("ids", ids, "total", total, "truncated", total > len(ids))))
}

// GET /api/hris/employees/{id}/placements
func (h handlers) listEmployeePlacements(w http.ResponseWriter, r *http.Request) error {
	id, err := requireUUID(r.PathValue("id"), "ID karyawan tidak valid")
	if err != nil {
		return err
	}
	if err := h.requireEmployeeAccess(r, id, placementManagers); err != nil {
		return err
	}
	rows, err := h.svc.repo.PlacementsOf(r.Context(), id)
	if err != nil {
		return err
	}
	return ok(w, dataOf(rows))
}

// POST /api/hris/employees/{id}/placements { branch_id, is_primary?, start_date?, end_date?, notes? }
func (h handlers) createEmployeePlacement(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	id, err := requireUUID(r.PathValue("id"), "ID karyawan tidak valid")
	if err != nil {
		return err
	}
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	in := parsePlacement(f)
	if err := formErr(f, ""); err != nil {
		return err
	}
	row, err := h.svc.CreatePlacement(r.Context(), id, u.ID, in)
	if err != nil {
		return err
	}
	return ok(w, dataMsg(row, "Penempatan ditambahkan"))
}

// PATCH /api/hris/employee-placements/{id}
func (h handlers) updatePlacement(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	id, err := requireUUID(r.PathValue("id"), "ID penempatan tidak valid")
	if err != nil {
		return err
	}
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	in := parsePlacement(f)
	if err := formErr(f, ""); err != nil {
		return err
	}
	row, err := h.svc.UpdatePlacement(r.Context(), id, in)
	if err != nil {
		return err
	}
	return ok(w, dataMsg(row, "Penempatan diperbarui"))
}

// DELETE /api/hris/employee-placements/{id}
func (h handlers) deletePlacement(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	id, err := requireUUID(r.PathValue("id"), "ID penempatan tidak valid")
	if err != nil {
		return err
	}
	if err := h.svc.DeletePlacement(r.Context(), id); err != nil {
		return err
	}
	return ok(w, msgOf("Penempatan dihapus"))
}

// bulkSteps are the step values of the bulk actions.
var bulkSteps = []string{bulkPreview, bulkCommit}

// POST /api/hris/employee-placements/bulk { branch_id, employee_ids[], is_primary?, start_date?, end_date? }
// With mode ("add"|"move", EPIC-070) it also takes step ("preview", the
// default, | "commit") and notes, and answers
// { step, created, moved, skipped, results[] }; without it the legacy
// { created, skipped } (R6). A store delegate (EPIC-071 R6) must send mode.
func (h handlers) bulkPlacement(w http.ResponseWriter, r *http.Request, u *auth.User, sc storeScope) error {
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	branch := reqStr(f, "branch_id", "Toko wajib dipilih", validate.StrOpts{Check: validate.UUIDCheck})
	ids := f.Strings("employee_ids", validate.Rule{}, 500, validate.StrOpts{Check: validate.UUIDCheck})
	primary := f.Bool("is_primary", optional)
	start := f.Str("start_date", optional, dateOpts)
	end := f.Str("end_date", nullish, dateOpts)
	var mode, step, notes *string
	withMode := sent(f, "mode")
	if withMode {
		mode = f.Enum("mode", validate.Rule{}, []string{bulkAdd, bulkMove})
		step = f.Enum("step", optional, bulkSteps)
		notes = f.Str("notes", nullish, validate.StrOpts{Max: 500})
	}
	if err := formErr(f, ""); err != nil {
		return err
	}
	if len(ids) == 0 {
		return httpx.BadRequest("Pilih minimal satu karyawan")
	}
	if sc.Delegate != nil && !withMode {
		return httpx.BadRequest("Pilih mode pindah atau tambah")
	}
	if withMode {
		in := bulkPlacementRequest{Mode: *mode, Step: bulkPreview, BranchID: *branch, EmployeeIDs: ids,
			Primary: primary != nil && *primary, Start: strOr(start), End: nilIfEmpty(end), Notes: nilIfEmpty(notes), Scope: sc.Delegate}
		if step != nil {
			in.Step = *step
		}
		out, err := h.svc.BulkPlacementPlan(r.Context(), u, in)
		if err != nil {
			return err
		}
		return ok(w, out)
	}
	created, skipped, err := h.svc.BulkPlacement(r.Context(), u.ID, *branch, ids, primary != nil && *primary, strOr(start), nilIfEmpty(end))
	if err != nil {
		return err
	}
	msg := fmt.Sprintf("%d penempatan ditambahkan", created)
	if skipped > 0 {
		msg += fmt.Sprintf(", %d dilewati (sudah ditempatkan / tidak valid)", skipped)
	}
	return ok(w, dataMsg(obj("created", created, "skipped", skipped), msg))
}

// POST /api/hris/employee-placements/bulk-end { employee_ids[], end_date, branch_id?, step }
// → { step, ended, skipped, results[] }
func (h handlers) bulkEndPlacement(w http.ResponseWriter, r *http.Request, u *auth.User, sc storeScope) error {
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	ids := f.Strings("employee_ids", validate.Rule{}, 500, validate.StrOpts{Check: validate.UUIDCheck})
	end := reqStr(f, "end_date", "Tanggal akhir wajib diisi", dateOpts)
	branch := f.Str("branch_id", nullish, validate.StrOpts{Check: validate.UUIDCheck})
	step := f.Enum("step", validate.Rule{}, bulkSteps)
	if err := formErr(f, ""); err != nil {
		return err
	}
	if len(ids) == 0 {
		return httpx.BadRequest("Pilih minimal satu karyawan")
	}
	out, err := h.svc.BulkEndPlacement(r.Context(), u, *step, ids, *end, strOr(branch), sc.Delegate)
	if err != nil {
		return err
	}
	return ok(w, out)
}

// GET /api/hris/me/placements?employee_id&date: the stores a clock-in can
// pick (own record; HR may ask for any employee).
func (h handlers) myPlacements(w http.ResponseWriter, r *http.Request, a *Actor) error {
	q := r.URL.Query()
	employeeID := strOr(a.EmployeeID)
	if requested := q.Get("employee_id"); requested != "" && requested != "me" && a.IsHR {
		employeeID = requested
	}
	if employeeID == "" {
		return ok(w, dataOf(obj("placements", []any{}, "default_branch_id", nil)))
	}
	if !domain.IsUUID(employeeID) {
		return httpx.BadRequest("ID karyawan tidak valid")
	}
	date := q.Get("date")
	if date == "" {
		date = domain.TodayWIB(h.svc.now())
	} else if !domain.DateRe.MatchString(date) {
		return httpx.BadRequest("Format tanggal tidak valid")
	}
	rows, def, err := h.svc.ClockPlacements(r.Context(), employeeID, date)
	if err != nil {
		return err
	}
	if rows == nil {
		rows = []*Row{}
	}
	return ok(w, dataOf(obj("placements", rows, "default_branch_id", def)))
}

// GET /api/hris/branches: active stores for the placement and attendance
// pickers.
func (h handlers) listBranches(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	rows, err := h.svc.repo.ActiveBranches(r.Context())
	if err != nil {
		return err
	}
	return ok(w, dataOf(rows))
}

// SaveBranchLocation is the HR store location edit (EPIC-062 R9): HRIS
// writes the three geofence columns of configuration.branches (lat/lng are
// shared with the public site map) and records the change in the audit log.
func (s *Service) SaveBranchLocation(ctx context.Context, u *auth.User, id string, lat, lng *float64, radius *int, meta audit.Entry) (*Row, error) {
	if msg := domain.ValidateBranchLocation(lat, lng, radius); msg != "" {
		return nil, httpx.BadRequest(msg)
	}
	var row *Row
	err := s.repo.InTx(ctx, func(r Repository) error {
		before, err := r.BranchGeofence(ctx, id)
		if err != nil {
			return err
		}
		if before == nil {
			return httpx.NotFound("Toko tidak ditemukan")
		}
		if row, err = r.SaveBranchLocation(ctx, id, lat, lng, radius); err != nil {
			return err
		}
		meta.ActorID, meta.ActorName = u.ID, ptr(u.FullName)
		meta.Action, meta.Entity, meta.EntityID = "branch_location.update", "configuration.branches", ptr(id)
		meta.EntityLabel = ptr(before.Str("name"))
		meta.Before = map[string]any{"lat": before.Get("lat"), "lng": before.Get("lng"), "geofence_radius_m": before.Get("geofence_radius_m")}
		meta.After = map[string]any{"lat": row.Get("lat"), "lng": row.Get("lng"), "geofence_radius_m": row.Get("geofence_radius_m")}
		return audit.Write(ctx, r.querier(), meta)
	})
	return row, err
}

// PUT /api/hris/branches/{id}/location { lat, lng, geofence_radius_m }
func (h handlers) putBranchLocation(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	id, err := requireUUID(r.PathValue("id"), "ID toko tidak valid")
	if err != nil {
		return err
	}
	const message = "Validation failed"
	f, err := readForm(r, message)
	if err != nil {
		return err
	}
	lat := f.Num("lat", nullish, validate.NumOpts{})
	lng := f.Num("lng", nullish, validate.NumOpts{})
	radius := f.Int("geofence_radius_m", nullish, validate.NumOpts{})
	if err := formErr(f, message); err != nil {
		return err
	}
	row, err := h.svc.SaveBranchLocation(r.Context(), u, id, lat, lng, radius, auditMeta(r))
	if err != nil {
		return err
	}
	return ok(w, dataOf(row))
}

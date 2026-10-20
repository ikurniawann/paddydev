package hris

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/auth"
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
	ActivePlacements(ctx context.Context, employeeID, date string) ([]*Row, error)
	PlacementByID(ctx context.Context, id string) (*Row, error)
	InsertPlacement(ctx context.Context, f fields) (*Row, error)
	UpdatePlacement(ctx context.Context, id string, f fields) (*Row, error)
	DeletePlacement(ctx context.Context, id string) (bool, error)
	ClearOtherPrimary(ctx context.Context, employeeID, exceptID, start string, end *string) error
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
// the store, Unplaced keeps those without any running placement, Search
// matches name/NIP, All includes ended placements.
type placementFilter struct {
	Branch, Search string
	Unplaced, All  bool
	Today          string
}

func (s *store) PlacementOverview(ctx context.Context, f placementFilter) ([]*Row, error) {
	running := `(ep.end_date IS NULL OR ep.end_date >= $1::date)`
	where := []string{"e.is_active"}
	args := []any{f.Today}
	if f.Branch != "" {
		args = append(args, f.Branch)
		where = append(where, fmt.Sprintf(`EXISTS (SELECT 1 FROM hris.employee_placements ep
			WHERE ep.employee_id = e.id AND ep.branch_id = $%d AND %s)`, len(args), running))
	}
	if f.Unplaced {
		where = append(where, `NOT EXISTS (SELECT 1 FROM hris.employee_placements ep WHERE ep.employee_id = e.id AND `+running+`)`)
	}
	if q := strings.TrimSpace(f.Search); q != "" {
		args = append(args, "%"+q+"%")
		where = append(where, fmt.Sprintf(`(e.full_name ILIKE $%d OR e.nip ILIKE $%d)`, len(args), len(args)))
	}
	placementCond := running
	if f.All {
		placementCond = "true"
	}
	return queryRows(ctx, s.db, `SELECT e.id AS employee_id, e.full_name, e.nip, e.photo_url,
		d.name AS department_name, p.title AS position_title,
		COALESCE((SELECT json_agg(x ORDER BY x.is_primary DESC, x.branch_name) FROM (
		   SELECT `+placementColumns+placementFrom+`
		    WHERE ep.employee_id = e.id AND `+placementCond+`) x), '[]'::json) AS placements
		FROM hris.employees e
		LEFT JOIN hris.departments d ON d.id = e.department_id
		LEFT JOIN hris.positions p ON p.id = e.job_title_id
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY e.full_name`, args...)
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

func (s *store) ActiveBranches(ctx context.Context) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT b.id, b.code, b.name, b.company_id, c.name AS company_name, b.lat, b.lng,
		b.geofence_radius_m
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
	row, err := queryRow(ctx, s.db, `SELECT name, lat, lng, geofence_radius_m FROM configuration.branches WHERE id::text = $1`, id)
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
	rows, err := h.svc.repo.PlacementOverview(r.Context(), placementFilter{Branch: branch, Search: q.Get("q"),
		Unplaced: q.Get("unplaced") == "true", All: q.Get("include_ended") == "true", Today: domain.TodayWIB(h.svc.now())})
	if err != nil {
		return err
	}
	return ok(w, dataOf(rows))
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

// POST /api/hris/employee-placements/bulk { branch_id, employee_ids[], is_primary?, start_date?, end_date? }
func (h handlers) bulkPlacement(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	branch := reqStr(f, "branch_id", "Toko wajib dipilih", validate.StrOpts{Check: validate.UUIDCheck})
	ids := f.Strings("employee_ids", validate.Rule{}, 500, validate.StrOpts{Check: validate.UUIDCheck})
	primary := f.Bool("is_primary", optional)
	start := f.Str("start_date", optional, dateOpts)
	end := f.Str("end_date", nullish, dateOpts)
	if err := formErr(f, ""); err != nil {
		return err
	}
	if len(ids) == 0 {
		return httpx.BadRequest("Pilih minimal satu karyawan")
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

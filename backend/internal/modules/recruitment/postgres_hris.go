package recruitment

import (
	"context"

	"nuhabit/backend/internal/modules/recruitment/domain"
	"nuhabit/backend/internal/platform/database"
)

// Job openings live in recruitment.job_openings. The SQL reproduces what the
// TS query-builder shim generated (embeds as row_to_json subqueries,
// RETURNING * without embeds on writes).

const jobOpeningEmbeds = `(SELECT row_to_json(e) FROM (SELECT "id", "name" FROM "item"."brands" WHERE "id" = "job_openings"."brand_id") e) AS "brand", ` +
	`(SELECT row_to_json(e) FROM (SELECT "id", "title", "department", "level" FROM "hris"."positions" WHERE "id" = "job_openings"."position_id") e) AS "position"`

// ListJobOpenings lists every opening, newest first, with brand, position
// and department_ref embeds.
func (Postgres) ListJobOpenings(ctx context.Context, db database.Querier) ([]*Row, error) {
	return collect(db.Query(ctx, `SELECT *, `+jobOpeningEmbeds+`, `+
		`(SELECT row_to_json(e) FROM (SELECT "id", "name", "code" FROM "hris"."departments" WHERE "id" = "job_openings"."department_id") e) AS "department_ref" `+
		`FROM recruitment.job_openings ORDER BY "created_at" DESC`))
}

// PublishedJobOpenings is the public careers list.
func (Postgres) PublishedJobOpenings(ctx context.Context, db database.Querier) ([]*Row, error) {
	return collect(db.Query(ctx, `SELECT "id", "position_id", "brand_id", "department_id", "title", "slug", "department", `+
		`"location", "employment_type", "work_mode", "headcount", "description", "requirements", "benefits", `+
		`"closing_date", "published_at", "created_at", "updated_at", `+jobOpeningEmbeds+
		` FROM recruitment.job_openings WHERE "status" = $1 ORDER BY "published_at" DESC NULLS LAST, "created_at" DESC`, "published"))
}

func jobOpeningArgs(p domain.JobOpening) []any {
	return []any{p.PositionID, p.BrandID, p.DepartmentID, p.Title, p.Slug, p.Department, p.Location,
		p.EmploymentType, p.WorkMode, p.Headcount, p.Description, p.Requirements, p.Benefits, p.Status,
		p.ClosingDate, p.PublishedAt}
}

const jobOpeningColumns = `"position_id", "brand_id", "department_id", "title", "slug", "department", "location", ` +
	`"employment_type", "work_mode", "headcount", "description", "requirements", "benefits", "status", "closing_date", "published_at"`

// InsertJobOpening creates an opening and returns its row.
func (Postgres) InsertJobOpening(ctx context.Context, db database.Querier, p domain.JobOpening) (*Row, error) {
	return collectOne(db.Query(ctx, `INSERT INTO recruitment.job_openings (`+jobOpeningColumns+`) VALUES `+
		`($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16) RETURNING *`, jobOpeningArgs(p)...))
}

// UpdateJobOpening rewrites every payload column (nil when missing).
func (Postgres) UpdateJobOpening(ctx context.Context, db database.Querier, id string, p domain.JobOpening) (*Row, error) {
	return collectOne(db.Query(ctx, `UPDATE recruitment.job_openings SET "position_id" = $1, "brand_id" = $2, `+
		`"department_id" = $3, "title" = $4, "slug" = $5, "department" = $6, "location" = $7, "employment_type" = $8, `+
		`"work_mode" = $9, "headcount" = $10, "description" = $11, "requirements" = $12, "benefits" = $13, `+
		`"status" = $14, "closing_date" = $15, "published_at" = $16 WHERE "id" = $17 RETURNING *`,
		append(jobOpeningArgs(p), id)...))
}

// DeleteJobOpening deletes an opening (no error when it is already gone).
func (Postgres) DeleteJobOpening(ctx context.Context, db database.Querier, id string) error {
	_, err := db.Exec(ctx, `DELETE FROM recruitment.job_openings WHERE "id" = $1`, id)
	return err
}

const positionSelect = `SELECT p.*,
    CASE WHEN b.id IS NULL THEN NULL ELSE json_build_object('name', b.name) END AS brands
  FROM hris.positions p LEFT JOIN item.brands b ON b.id = p.brand_id`

// ListPositions lists positions by title, optionally of one brand.
func (Postgres) ListPositions(ctx context.Context, db database.Querier, brandID *string) ([]*Row, error) {
	return collect(db.Query(ctx, positionSelect+` WHERE $1::uuid IS NULL OR p.brand_id = $1 ORDER BY p.title`, brandID))
}

// InsertPosition creates a position with the master-data defaults and
// returns it with its brand.
func (Postgres) InsertPosition(ctx context.Context, db database.Querier, in positionInput) (*Row, error) {
	var id string
	if err := db.QueryRow(ctx, `INSERT INTO hris.positions (title, department, level, is_active, brand_id)
     VALUES ($1, COALESCE($2, 'Operations'), COALESCE($3, 'Staff'), COALESCE($4, true), $5) RETURNING id::text`,
		in.Title, in.Department, in.Level, in.IsActive, in.BrandID).Scan(&id); err != nil {
		return nil, err
	}
	return collectOne(db.Query(ctx, positionSelect+` WHERE p.id = $1`, id))
}

// promotionCandidate is the candidate fields the promotion checks.
type promotionCandidate struct {
	ID, FullName, Status string
	PromotedTo           *string
}

// PromotionCandidate loads a candidate; like the TS shim's maybeSingle, any
// query failure (an id that is not a uuid) reads as "not found".
func (Postgres) PromotionCandidate(ctx context.Context, db database.Querier, id string) *promotionCandidate {
	c := promotionCandidate{}
	err := db.QueryRow(ctx, `SELECT id::text, full_name, status, promoted_to_employee_id::text
     FROM recruitment.candidates WHERE id = $1`, id).Scan(&c.ID, &c.FullName, &c.Status, &c.PromotedTo)
	if err != nil {
		return nil
	}
	return &c
}

// PromoteCandidate calls public.promote_candidate_to_employee: one
// transaction that inserts the employee with the NIP HR typed (EPIC-081) and
// links the candidate. It raises 23505 when the candidate was already promoted.
func (Postgres) PromoteCandidate(ctx context.Context, db database.Querier, candidateID, joinDate, employmentStatus string, departmentID, reportingTo *string, nip string) (*string, error) {
	var id *string
	err := db.QueryRow(ctx, "SELECT public.promote_candidate_to_employee($1, $2::date, $3, $4, $5, $6)::text AS id",
		candidateID, joinDate, employmentStatus, departmentID, reportingTo, nip).Scan(&id)
	return id, err
}

// AcceptedOffer is the latest accepted offer of a candidate.
type AcceptedOffer struct {
	Version       int
	PositionTitle *string
	BaseSalary    *string
}

// LatestAcceptedOffer returns the newest accepted offer (nil when none).
func (Postgres) LatestAcceptedOffer(ctx context.Context, db database.Querier, candidateID string) (*AcceptedOffer, error) {
	o := AcceptedOffer{}
	err := db.QueryRow(ctx, `SELECT version, position_title, base_salary::text
       FROM recruitment.candidate_offers
       WHERE candidate_id = $1 AND status = 'accepted'
       ORDER BY version DESC LIMIT 1`, candidateID).Scan(&o.Version, &o.PositionTitle, &o.BaseSalary)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

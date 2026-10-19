package insights

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/modules/insights/domain"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
)

// Recruitment dashboard (app/api/dashboard/{attention,funnel,sources,stats,
// weekly}) and analytics (app/api/analytics/*). The TS reads go through the
// query-builder shim; the SQL here is what it generated. Periods are WIB.

const staleAfter = 7 * 24 * time.Hour

// closedStatusArgs binds the closed statuses to $1..$3.
func closedStatusArgs() []any {
	return []any{domain.ClosedStatuses[0], domain.ClosedStatuses[1], domain.ClosedStatuses[2]}
}

// brandFilter returns "AND <col> = $n" and appends the brand id when the
// brand_id query parameter is set.
func brandFilter(r *http.Request, col string, args []any) (string, []any) {
	b := r.URL.Query().Get("brand_id")
	if b == "" {
		return "", args
	}
	args = append(args, b)
	return " AND " + col + " = $" + strconv.Itoa(len(args)), args
}

func (s *Service) attention(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.auth.RequireMenuPrefix(r, recruitmentReaders...); err != nil {
		return err
	}
	now := s.now()
	args := append(closedStatusArgs(), now.Add(-staleAfter))
	brand, args := brandFilter(r, "c.brand_id", args)
	rows, err := s.db.Query(r.Context(), `SELECT c.id::text, c.full_name, c.status, c.updated_at, p.title, b.name
		FROM recruitment.candidates c
		LEFT JOIN hris.positions p ON p.id = c.position_id
		LEFT JOIN item.brands b ON b.id = c.brand_id
		WHERE c.status NOT IN ($1, $2, $3) AND c.updated_at < $4`+brand+`
		ORDER BY c.updated_at ASC LIMIT 10`, args...)
	if err != nil {
		return err
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.AttentionItem, error) {
		var c domain.StaleCandidate
		err := row.Scan(&c.ID, &c.FullName, &c.Status, &c.UpdatedAt, &c.PositionTitle, &c.BrandName)
		return domain.Attention(c, now), err
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"data": items})
}

func (s *Service) funnel(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.auth.RequireMenuPrefix(r, recruitmentReaders...); err != nil {
		return err
	}
	start := domain.PeriodStartByDays(periodParam(r, "month"), s.now(), 30)
	brand, args := brandFilter(r, "brand_id", []any{start})
	statuses, err := s.textColumn(r.Context(), `SELECT status FROM recruitment.candidates WHERE created_at >= $1`+brand+` LIMIT 5000`, args...)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, domain.FunnelStages(statuses))
}

func (s *Service) sources(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.auth.RequireMenuPrefix(r, recruitmentReaders...); err != nil {
		return err
	}
	start := domain.PeriodStartByDays(periodParam(r, "month"), s.now(), 30)
	brand, args := brandFilter(r, "brand_id", []any{start})
	sources, err := s.textColumn(r.Context(), `SELECT source FROM recruitment.candidates WHERE created_at >= $1`+brand, args...)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, domain.SourceDistribution(sources))
}

type recruitmentStats struct {
	CandidatesThisMonth int `json:"candidates_this_month"`
	ActivePipeline      int `json:"active_pipeline"`
	TalentPool          int `json:"talent_pool"`
	OpenPositions       int `json:"open_positions"`
	HiredThisMonth      int `json:"hired_this_month"`
}

func (s *Service) stats(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.auth.RequireMenuPrefix(r, recruitmentReaders...); err != nil {
		return err
	}
	local := s.now().In(domain.WIB)
	monthStart := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, domain.WIB)
	ctx := r.Context()
	// The shim reports a failed count as null, which the route turns into 0.
	count := func(dst *int, table, where string, args ...any) func() {
		return func() {
			brand, args := brandFilter(r, "brand_id", args)
			_ = s.db.QueryRow(ctx, `SELECT count(*)::int FROM `+table+` WHERE `+where+brand, args...).Scan(dst)
		}
	}
	var out recruitmentStats
	s.parallel(
		count(&out.CandidatesThisMonth, "recruitment.candidates", "created_at >= $1", monthStart),
		count(&out.ActivePipeline, "recruitment.candidates", "status NOT IN ($1, $2, $3)", closedStatusArgs()...),
		count(&out.TalentPool, "recruitment.candidates", "status = $1", "talent_pool"),
		count(&out.OpenPositions, "hris.positions", "is_active = $1", true),
		count(&out.HiredThisMonth, "recruitment.candidates", "status = $1 AND updated_at >= $2", "hired", monthStart),
	)
	return httpx.JSON(w, http.StatusOK, out)
}

func (s *Service) weekly(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.auth.RequireMenuPrefix(r, recruitmentReaders...); err != nil {
		return err
	}
	weeks := domain.LastEightWeeks(s.now())
	brand, args := brandFilter(r, "brand_id", []any{weeks[0].Start, weeks[len(weeks)-1].End})
	rows, err := s.db.Query(r.Context(), `SELECT created_at FROM recruitment.candidates
		WHERE created_at >= $1 AND created_at <= $2`+brand, args...)
	if err != nil {
		return err
	}
	created, err := pgx.CollectRows(rows, pgx.RowTo[time.Time])
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, domain.WeeklyApplications(weeks, created))
}

func (s *Service) analyticsBrands(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.auth.RequireMenuPrefix(r, iam.HrisAnalytics...); err != nil {
		return err
	}
	ctx := r.Context()
	start := domain.PeriodStartByCalendar(periodParam(r, "3month"), s.now())
	rows, err := s.db.Query(ctx, `SELECT id::text, name FROM item.brands WHERE is_active = $1 ORDER BY name ASC`, true)
	if err != nil {
		return err
	}
	brands, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Brand, error) {
		var b domain.Brand
		return b, row.Scan(&b.ID, &b.Name)
	})
	if err != nil {
		return err
	}
	brand, args := brandFilter(r, "brand_id", []any{start})
	rows, err = s.db.Query(ctx, `SELECT brand_id::text, status FROM recruitment.candidates WHERE created_at >= $1`+brand, args...)
	if err != nil {
		return err
	}
	cands, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.BrandStatus, error) {
		var c domain.BrandStatus
		return c, row.Scan(&c.BrandID, &c.Status)
	})
	if err != nil {
		return err
	}
	var filter *string
	if b := r.URL.Query().Get("brand_id"); b != "" {
		filter = &b
	}
	return httpx.JSON(w, http.StatusOK, domain.BrandComparison(brands, cands, filter))
}

type analyticsOverview struct {
	domain.OverviewKpis
	HardToFill []domain.HardToFillPosition `json:"hard_to_fill"`
}

func (s *Service) analyticsOverview(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.auth.RequireMenuPrefix(r, iam.HrisAnalytics...); err != nil {
		return err
	}
	ctx := r.Context()
	start := domain.PeriodStartByDays(periodParam(r, "3month"), s.now(), 90)
	brand, args := brandFilter(r, "brand_id", []any{start})
	rows, err := s.db.Query(ctx, `SELECT id::text, status, created_at, updated_at, position_id::text
		FROM recruitment.candidates WHERE created_at >= $1`+brand, args...)
	if err != nil {
		return err
	}
	cands, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.OverviewCandidate, error) {
		var c domain.OverviewCandidate
		return c, row.Scan(&c.ID, &c.Status, &c.CreatedAt, &c.UpdatedAt, &c.PositionID)
	})
	if err != nil {
		return err
	}
	kpis, perPosition := domain.ComputeOverviewKpis(cands)
	out := analyticsOverview{OverviewKpis: kpis, HardToFill: []domain.HardToFillPosition{}}
	if len(perPosition) > 0 {
		ids := make([]string, len(perPosition))
		for i, p := range perPosition {
			ids[i] = p.PositionID
		}
		// A failed positions read leaves hard_to_fill empty, as in the TS.
		if titles, err := s.positionTitles(ctx, ids); err == nil {
			out.HardToFill = domain.HardToFill(perPosition, titles)
		}
	}
	return httpx.JSON(w, http.StatusOK, out)
}

func (s *Service) positionTitles(ctx context.Context, ids []string) (map[string]string, error) {
	rows, err := s.db.Query(ctx, `SELECT id::text, title FROM hris.positions WHERE id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, err
	}
	titles := map[string]string{}
	var id string
	var title *string
	_, err = pgx.ForEachRow(rows, []any{&id, &title}, func() error {
		if title != nil {
			titles[id] = *title
		}
		return nil
	})
	return titles, err
}

func (s *Service) analyticsSources(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.auth.RequireMenuPrefix(r, iam.HrisAnalytics...); err != nil {
		return err
	}
	start := domain.PeriodStartByCalendar(periodParam(r, "3month"), s.now())
	brand, args := brandFilter(r, "brand_id", []any{start})
	rows, err := s.db.Query(r.Context(), `SELECT source, status FROM recruitment.candidates WHERE created_at >= $1`+brand, args...)
	if err != nil {
		return err
	}
	data, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.SourceStatus, error) {
		var c domain.SourceStatus
		return c, row.Scan(&c.Source, &c.Status)
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"data": domain.SourceConversion(data)})
}

// periodParam is sp.get("period") || fallback.
func periodParam(r *http.Request, fallback string) string {
	if p := r.URL.Query().Get("period"); p != "" {
		return p
	}
	return fallback
}

// textColumn reads one nullable text column.
func (s *Service) textColumn(ctx context.Context, sql string, args ...any) ([]*string, error) {
	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[*string])
}

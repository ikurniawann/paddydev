package recruitment

import (
	"context"
	"net/http"
	"os"

	"nuhabit/backend/internal/modules/recruitment/domain"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/module"
	"nuhabit/backend/internal/platform/whatsapp"
)

// The public career form options (GET /api/portal/options) and the HR
// candidate notification (POST /api/notifications/send). POST
// /api/portal/submit is in portal_submit.go.

// portalRoutes lists the routes of this file and notify_send.go.
func (h *handler) portalRoutes() []module.Route {
	n := &notifier{svc: h.svc, users: h.guard, sender: defaultSender{wa: whatsapp.New(h.svc.log), getenv: os.Getenv}}
	return []module.Route{
		{Pattern: "GET /api/portal/options", Handler: httpx.Handle(h.portalOptions)},
		{Pattern: "POST /api/notifications/send", Handler: httpx.Handle(n.send)},
	}
}

type portalOutlet struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type portalPosition struct {
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	BrandID *string `json:"brand_id"`
}

type portalOpening struct {
	BrandID    *string `json:"brand_id"`
	PositionID *string `json:"position_id"`
}

// PortalOptions is getPortalOptions: active outlets and positions, and the
// brand and position of a job opening for auto-fill (only what the career
// page shows publicly).
func (s *Service) PortalOptions(ctx context.Context, openingID string) (outlets []portalOutlet, positions []portalPosition, opening *portalOpening, err error) {
	outlets, positions = []portalOutlet{}, []portalPosition{}
	if err = scanEach(ctx, s.db, `SELECT id::text, name FROM item.brands WHERE is_active = true ORDER BY name`, nil,
		func(scan func(...any) error) error {
			var o portalOutlet
			err := scan(&o.ID, &o.Name)
			outlets = append(outlets, o)
			return err
		}); err != nil {
		return
	}
	if err = scanEach(ctx, s.db, `SELECT id::text, title, brand_id::text FROM hris.positions WHERE is_active = true ORDER BY title`, nil,
		func(scan func(...any) error) error {
			var p portalPosition
			err := scan(&p.ID, &p.Title, &p.BrandID)
			positions = append(positions, p)
			return err
		}); err != nil {
		return
	}
	if openingID != "" && domain.IsUUID(openingID) {
		var o portalOpening
		// The TS reads hris.job_openings, which does not exist (every
		// ?opening=<uuid> fails with a 500); the table is recruitment.job_openings.
		err = s.db.QueryRow(ctx, `SELECT brand_id::text, position_id::text FROM recruitment.job_openings WHERE id = $1`, openingID).Scan(&o.BrandID, &o.PositionID)
		if database.IsNoRows(err) {
			return outlets, positions, nil, nil
		}
		opening = &o
	}
	return
}

func scanEach(ctx context.Context, q database.Querier, sql string, args []any, each func(scan func(...any) error) error) error {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := each(rows.Scan); err != nil {
			return err
		}
	}
	return rows.Err()
}

type portalOptionsData struct {
	Outlets   []portalOutlet   `json:"outlets"`
	Positions []portalPosition `json:"positions"`
	Opening   *portalOpening   `json:"opening"`
}

// portalOptions answers `{ data }` without "success", as the TS route.
func (h *handler) portalOptions(w http.ResponseWriter, r *http.Request) error {
	outlets, positions, opening, err := h.svc.PortalOptions(r.Context(), r.URL.Query().Get("opening"))
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, struct {
		Data portalOptionsData `json:"data"`
	}{portalOptionsData{outlets, positions, opening}})
}

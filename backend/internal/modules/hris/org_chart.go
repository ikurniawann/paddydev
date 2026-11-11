package hris

import (
	"context"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/xlsx"
)

// Bagan organisasi berversi (EPIC-101): baca saja; persetujuan versi ada di
// org_chart_approve.go (EPIC-102). Versi, pohon node,
// validasi BR, dan laporan selisih bagan vs karyawan aktif. Tidak ada data
// karyawan, jabatan, toko, atau approval yang diubah.

type orgVersion struct {
	ID         string    `json:"id"`
	Label      string    `json:"label"`
	Status     string    `json:"status"`
	SourceFile *string   `json:"source_file"`
	ImportedAt time.Time `json:"imported_at"`
	Nodes      int       `json:"nodes"`
	// EPIC-102: tanggal berlaku dan penyetuju (nil selama belum disetujui).
	ValidFrom      *string    `json:"valid_from"`
	ApprovedAt     *time.Time `json:"approved_at"`
	ApprovedByName *string    `json:"approved_by_name"`
}

type orgHolder struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	NIP  string `json:"nip"`
}

type orgNodeView struct {
	Code            string      `json:"code"`
	ParentCode      *string     `json:"parent_code"`
	Title           string      `json:"title"`
	RawTitle        string      `json:"raw_title"`
	IsPosition      bool        `json:"is_position"`
	JobLevel        *string     `json:"job_level"`
	LevelConfidence *string     `json:"level_confidence"`
	EmploymentType  *string     `json:"employment_type"`
	Vendor          *string     `json:"vendor"`
	DivisionCode    string      `json:"division_code"`
	DivisionName    string      `json:"division_name"`
	Region          *string     `json:"region"`
	StoreCodes      []string    `json:"store_codes"`
	Status          string      `json:"status"`
	NeedsConfirm    bool        `json:"needs_confirm"`
	Flags           []string    `json:"flags"`
	Holders         []orgHolder `json:"holders"`
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func orgVersions(ctx context.Context, q database.Querier) ([]orgVersion, error) {
	rows, err := q.Query(ctx, `SELECT v.id::text, v.label, v.status, v.source_file, v.imported_at,
		(SELECT count(*) FROM hris.org_chart_nodes n WHERE n.version_id = v.id),
		to_char(v.valid_from, 'YYYY-MM-DD'), v.approved_at, v.approved_by_name
		FROM hris.org_chart_versions v ORDER BY v.imported_at DESC, v.label`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []orgVersion{}
	for rows.Next() {
		var v orgVersion
		if err := rows.Scan(&v.ID, &v.Label, &v.Status, &v.SourceFile, &v.ImportedAt, &v.Nodes,
			&v.ValidFrom, &v.ApprovedAt, &v.ApprovedByName); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// orgVersionOf resolves ?version= (label); empty = the approved version,
// else the latest import (EPIC-102 R7).
func orgVersionOf(ctx context.Context, q database.Querier, label string) (orgVersion, error) {
	vs, err := orgVersions(ctx, q)
	if err != nil {
		return orgVersion{}, err
	}
	if label == "" {
		for _, v := range vs {
			if v.Status == "approved" {
				return v, nil
			}
		}
		if len(vs) > 0 {
			return vs[0], nil
		}
	}
	for _, v := range vs {
		if label != "" && v.Label == label {
			return v, nil
		}
	}
	return orgVersion{}, httpx.NotFound("Versi bagan tidak ditemukan")
}

func orgNodes(ctx context.Context, q database.Querier, versionID string) ([]orgNodeView, error) {
	rows, err := q.Query(ctx, `SELECT code, parent_code, title, raw_title, is_position, job_level, level_confidence,
		employment_type, vendor, division_code, division_name, region, store_codes, status, needs_confirm, flags
		FROM hris.org_chart_nodes WHERE version_id = $1 ORDER BY sort_order`, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []orgNodeView{}
	for rows.Next() {
		var n orgNodeView
		if err := rows.Scan(&n.Code, &n.ParentCode, &n.Title, &n.RawTitle, &n.IsPosition, &n.JobLevel, &n.LevelConfidence,
			&n.EmploymentType, &n.Vendor, &n.DivisionCode, &n.DivisionName, &n.Region, &n.StoreCodes, &n.Status,
			&n.NeedsConfirm, &n.Flags); err != nil {
			return nil, err
		}
		n.Holders = []orgHolder{}
		out = append(out, n)
	}
	return out, rows.Err()
}

func domainNodes(views []orgNodeView) []domain.OrgNode {
	out := make([]domain.OrgNode, len(views))
	for i, n := range views {
		out[i] = domain.OrgNode{Code: n.Code, ParentCode: deref(n.ParentCode), Title: n.Title, IsPosition: n.IsPosition,
			Level: deref(n.JobLevel), EmploymentType: deref(n.EmploymentType), Vendor: deref(n.Vendor),
			DivisionCode: n.DivisionCode, Region: deref(n.Region), Status: n.Status, StoreCodes: n.StoreCodes}
	}
	return out
}

// storeCodePattern is a store's short code (G: every chart store is 4 letters);
// HQ, warehouse, and event branches are not compared with the chart.
var storeCodePattern = regexp.MustCompile(`^[A-Z]{4}$`)

// orgPeople loads active employees (title, manager, primary store short code
// per ruling R6), the title aliases and the active store codes.
func orgPeople(ctx context.Context, q database.Querier, today string) ([]domain.OrgEmployee, map[string]domain.OrgAlias, []string, error) {
	rows, err := q.Query(ctx, `SELECT e.id::text, e.full_name, coalesce(e.nip, ''), coalesce(p.title, ''),
		  coalesce(e.reporting_to::text, ''),
		  coalesce((SELECT upper(regexp_replace(b.code, '^ODOO-', ''))
		    FROM hris.employee_placements ep JOIN configuration.branches b ON b.id = ep.branch_id
		    WHERE ep.employee_id = e.id AND ep.is_primary AND ep.start_date <= $1::date
		      AND (ep.end_date IS NULL OR ep.end_date >= $1::date)
		    ORDER BY ep.start_date DESC LIMIT 1), '')
		FROM hris.employees e LEFT JOIN hris.positions p ON p.id = e.job_title_id
		WHERE e.is_active ORDER BY e.full_name, e.id`, today)
	if err != nil {
		return nil, nil, nil, err
	}
	emps, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.OrgEmployee, error) {
		var e domain.OrgEmployee
		err := r.Scan(&e.ID, &e.Name, &e.NIP, &e.Title, &e.ManagerID, &e.StoreCode)
		return e, err
	})
	if err != nil {
		return nil, nil, nil, err
	}
	rows, err = q.Query(ctx, `SELECT alias, title, coalesce(region, ''), needs_review FROM hris.org_title_aliases`)
	if err != nil {
		return nil, nil, nil, err
	}
	aliases := map[string]domain.OrgAlias{}
	for rows.Next() {
		var k string
		var a domain.OrgAlias
		if err := rows.Scan(&k, &a.Title, &a.Region, &a.NeedsReview); err != nil {
			rows.Close()
			return nil, nil, nil, err
		}
		aliases[k] = a
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, nil, err
	}
	rows, err = q.Query(ctx, `SELECT DISTINCT upper(regexp_replace(code, '^ODOO-', '')) FROM configuration.branches WHERE is_active`)
	if err != nil {
		return nil, nil, nil, err
	}
	all, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, nil, nil, err
	}
	var stores []string
	for _, s := range all {
		if storeCodePattern.MatchString(s) {
			stores = append(stores, s)
		}
	}
	return emps, aliases, stores, nil
}

type orgGapResult struct {
	version orgVersion
	nodes   []orgNodeView
	gap     domain.OrgGap
}

func (s *Service) orgGap(ctx context.Context, label string) (*orgGapResult, error) {
	q := s.repo.querier()
	v, err := orgVersionOf(ctx, q, label)
	if err != nil {
		return nil, err
	}
	nodes, err := orgNodes(ctx, q, v.ID)
	if err != nil {
		return nil, err
	}
	emps, aliases, stores, err := orgPeople(ctx, q, domain.TodayWIB(s.now()))
	if err != nil {
		return nil, err
	}
	return &orgGapResult{v, nodes, domain.MatchOrgChart(domainNodes(nodes), emps, aliases, stores)}, nil
}

/* ── HTTP ────────────────────────────────────────────────────────────── */

// GET /api/hris/org-chart/versions
func (h handlers) orgChartVersions(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	vs, err := orgVersions(r.Context(), h.svc.repo.querier())
	if err != nil {
		return err
	}
	return ok(w, dataOf(vs))
}

// GET /api/hris/org-chart/tree?version=&division=&region=
func (h handlers) orgChartTree(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	qs := r.URL.Query()
	res, err := h.svc.orgGap(r.Context(), strings.TrimSpace(qs.Get("version")))
	if err != nil {
		return err
	}
	division, region := strings.TrimSpace(qs.Get("division")), strings.TrimSpace(qs.Get("region"))
	out := []orgNodeView{}
	for _, n := range res.nodes {
		if division != "" && n.DivisionCode != division {
			continue
		}
		if region != "" && deref(n.Region) != region {
			continue
		}
		for _, e := range res.gap.Holders[n.Code] {
			n.Holders = append(n.Holders, orgHolder{e.ID, e.Name, e.NIP})
		}
		out = append(out, n)
	}
	return ok(w, dataOf(map[string]any{"version": res.version, "nodes": out}))
}

// GET /api/hris/org-chart/validation?version=
func (h handlers) orgChartValidation(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	q := h.svc.repo.querier()
	v, err := orgVersionOf(r.Context(), q, strings.TrimSpace(r.URL.Query().Get("version")))
	if err != nil {
		return err
	}
	nodes, err := orgNodes(r.Context(), q, v.ID)
	if err != nil {
		return err
	}
	errs, warns := domain.ValidateOrgChart(domainNodes(nodes))
	return ok(w, dataOf(map[string]any{"version": v, "errors": errs, "warnings": warns}))
}

type orgTitleCount struct {
	Title     string `json:"title"`
	Employees int    `json:"employees"`
}

// GET /api/hris/org-chart/gap?version=&format=xlsx
func (h handlers) orgChartGap(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	qs := r.URL.Query()
	res, err := h.svc.orgGap(r.Context(), strings.TrimSpace(qs.Get("version")))
	if err != nil {
		return err
	}
	g := res.gap
	titles := make([]orgTitleCount, 0, len(g.TitlesOutside))
	for t, n := range g.TitlesOutside {
		titles = append(titles, orgTitleCount{t, n})
	}
	sort.Slice(titles, func(i, j int) bool {
		if titles[i].Employees != titles[j].Employees {
			return titles[i].Employees > titles[j].Employees
		}
		return titles[i].Title < titles[j].Title
	})
	sum := map[string]int{"employees": len(g.Matches), "matched": 0, "ambiguous": 0, "outside": 0, "manager_diff": 0,
		"alias_review": 0, "empty_nodes": len(g.EmptyNodes), "stores_outside": len(g.StoresOutside)}
	for _, m := range g.Matches {
		switch {
		case m.NodeCode != "":
			sum["matched"]++
		case strings.HasPrefix(m.Problem, "Cocok ke"):
			sum["ambiguous"]++
		default:
			sum["outside"]++
		}
		if m.ManagerProblem != "" {
			sum["manager_diff"]++
		}
		if m.AliasNeedsReview {
			sum["alias_review"]++
		}
	}
	matches := g.Matches
	if matches == nil {
		matches = []domain.OrgMatch{}
	}
	if qs.Get("format") == "xlsx" {
		file, err := orgGapWorkbook(res, sum, titles)
		if err != nil {
			return err
		}
		return writeFile(w, file)
	}
	return ok(w, dataOf(map[string]any{"version": res.version, "summary": sum, "matches": matches,
		"empty_nodes": g.EmptyNodes, "titles_outside": titles, "stores_outside": g.StoresOutside,
		"chart_stores_missing": g.ChartStoresMissing}))
}

func orgGapWorkbook(res *orgGapResult, sum map[string]int, titles []orgTitleCount) (*exportFile, error) {
	byCode := map[string]orgNodeView{}
	for _, n := range res.nodes {
		byCode[n.Code] = n
	}
	yes := func(b bool) string {
		if b {
			return "Ya"
		}
		return ""
	}
	summary := [][]any{{"Selisih Bagan " + res.version.Label}, {""},
		{"Karyawan aktif", sum["employees"]}, {"Cocok ke satu posisi", sum["matched"]},
		{"Cocok ke beberapa posisi (ambigu)", sum["ambiguous"]}, {"Jabatan tidak ada di bagan", sum["outside"]},
		{"Atasan berbeda dengan bagan", sum["manager_diff"]}, {"Alias perlu ditinjau HR", sum["alias_review"]},
		{"Posisi bagan tanpa pemegang", sum["empty_nodes"]}, {"Toko aktif di luar bagan", sum["stores_outside"]}}
	people := [][]any{{"NIP", "Nama", "Jabatan HRIS", "Jabatan Bagan", "Kode Posisi", "Masalah", "Masalah Atasan", "Alias Perlu Ditinjau"}}
	for _, m := range res.gap.Matches {
		people = append(people, []any{m.NIP, m.Name, m.Title, m.Canonical, m.NodeCode, m.Problem, m.ManagerProblem, yes(m.AliasNeedsReview)})
	}
	empty := [][]any{{"Kode", "Jabatan", "Divisi", "Region", "Toko", "Status"}}
	for _, c := range res.gap.EmptyNodes {
		n := byCode[c]
		empty = append(empty, []any{n.Code, n.Title, n.DivisionName, deref(n.Region), strings.Join(n.StoreCodes, ", "), n.Status})
	}
	outside := [][]any{{"Jabatan", "Jumlah Karyawan"}}
	for _, t := range titles {
		outside = append(outside, []any{t.Title, t.Employees})
	}
	stores := [][]any{{"Kode Toko", "Keterangan"}}
	for _, s := range res.gap.StoresOutside {
		stores = append(stores, []any{s, "Toko aktif di HRIS, tidak ada di bagan"})
	}
	for _, s := range res.gap.ChartStoresMissing {
		stores = append(stores, []any{s, "Ada di bagan, tidak ada sebagai toko aktif di HRIS"})
	}
	data, err := xlsx.Build(
		xlsx.SheetSpec{Name: "Ringkasan", Rows: summary, ColumnWidths: []float64{36, 12}},
		xlsx.SheetSpec{Name: "Karyawan", Rows: people, ColumnWidths: []float64{14, 28, 32, 32, 12, 44, 52, 12}},
		xlsx.SheetSpec{Name: "Posisi Kosong", Rows: empty, ColumnWidths: []float64{10, 34, 22, 18, 14, 10}},
		xlsx.SheetSpec{Name: "Jabatan di Luar Bagan", Rows: outside, ColumnWidths: []float64{40, 16}},
		xlsx.SheetSpec{Name: "Toko", Rows: stores, ColumnWidths: []float64{12, 48}},
	)
	if err != nil {
		return nil, err
	}
	return &exportFile{data, xlsx.ContentType, "Selisih Bagan " + res.version.Label + ".xlsx"}, nil
}

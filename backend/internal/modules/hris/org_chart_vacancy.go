package hris

import (
	"net/http"
	"strings"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/xlsx"
)

// Posisi Kosong (EPIC-102, ORG-08): keadaan setiap slot bagan dihitung dari
// pencocokan karyawan aktif (R2), tidak disimpan. Baca saja.

var vacancyLabels = map[string]string{
	domain.VacancyFilled:  "Terisi",
	domain.VacancyEmpty:   "Kosong",
	domain.VacancyMaybe:   "Mungkin terisi",
	domain.VacancyPlanned: "Rencana",
	domain.VacancyFrozen:  "Dibekukan",
}

type orgVacancySummary struct {
	Positions int `json:"positions"`
	Filled    int `json:"filled"`
	Empty     int `json:"empty"`
	Maybe     int `json:"maybe"`
	Planned   int `json:"planned"`
	Frozen    int `json:"frozen"`
}

type orgVacancyGroupView struct {
	Division     string `json:"division"`
	DivisionName string `json:"division_name"`
	Region       string `json:"region"`
	orgVacancySummary
}

type orgVacancyRow struct {
	Code         string   `json:"code"`
	Title        string   `json:"title"`
	DivisionCode string   `json:"division_code"`
	DivisionName string   `json:"division_name"`
	Region       string   `json:"region"`
	StoreCodes   []string `json:"store_codes"`
	ParentCode   string   `json:"parent_code"`
	ParentTitle  string   `json:"parent_title"`
	State        string   `json:"state"`
	Holders      int      `json:"holders"`
	MaybeHolders []string `json:"maybe_holders"`
}

type orgVacancyResult struct {
	Version orgVersion            `json:"version"`
	Summary orgVacancySummary     `json:"summary"`
	Groups  []orgVacancyGroupView `json:"groups"`
	Rows    []orgVacancyRow       `json:"rows"`
}

// chartParent is the first ancestor that is a position (group nodes are
// skipped), guarded against a cyclic chart.
func chartParent(byCode map[string]orgNodeView, n orgNodeView) (orgNodeView, bool) {
	seen := map[string]bool{n.Code: true}
	p, ok := byCode[deref(n.ParentCode)]
	for ok && !p.IsPosition {
		if seen[p.Code] {
			return orgNodeView{}, false
		}
		seen[p.Code] = true
		p, ok = byCode[deref(p.ParentCode)]
	}
	return p, ok
}

// buildVacancy matches against the whole chart, then narrows to the
// division/region filter for counting; state narrows rows only.
func buildVacancy(res *orgGapResult, division, region, state string) orgVacancyResult {
	states := domain.OrgVacancies(domainNodes(res.nodes), res.gap)
	byCode := map[string]orgNodeView{}
	divisionNames := map[string]string{}
	for _, n := range res.nodes {
		byCode[n.Code] = n
		if divisionNames[n.DivisionCode] == "" {
			divisionNames[n.DivisionCode] = n.DivisionName
		}
	}
	var kept []orgNodeView
	for _, n := range res.nodes {
		if division != "" && n.DivisionCode != division {
			continue
		}
		if region != "" && deref(n.Region) != region {
			continue
		}
		kept = append(kept, n)
	}
	out := orgVacancyResult{Version: res.version, Groups: []orgVacancyGroupView{}, Rows: []orgVacancyRow{}}
	for _, g := range domain.GroupVacancies(domainNodes(kept), states) {
		v := orgVacancyGroupView{Division: g.Division, DivisionName: divisionNames[g.Division], Region: g.Region, orgVacancySummary: orgVacancySummary{
			Positions: g.Positions, Filled: g.Filled, Empty: g.Empty, Maybe: g.Maybe, Planned: g.Planned, Frozen: g.Frozen}}
		out.Groups = append(out.Groups, v)
		s := &out.Summary
		s.Positions += g.Positions
		s.Filled += g.Filled
		s.Empty += g.Empty
		s.Maybe += g.Maybe
		s.Planned += g.Planned
		s.Frozen += g.Frozen
	}
	for _, n := range kept {
		st, ok := states[n.Code]
		if !ok || st.State == domain.VacancyFilled || (state != "" && st.State != state) {
			continue
		}
		row := orgVacancyRow{Code: n.Code, Title: n.Title, DivisionCode: n.DivisionCode, DivisionName: n.DivisionName,
			Region: deref(n.Region), StoreCodes: n.StoreCodes, State: st.State, Holders: st.Holders,
			MaybeHolders: st.MaybeHolders}
		if row.StoreCodes == nil {
			row.StoreCodes = []string{}
		}
		if row.MaybeHolders == nil {
			row.MaybeHolders = []string{}
		}
		if p, ok := chartParent(byCode, n); ok {
			row.ParentCode, row.ParentTitle = p.Code, p.Title
		}
		out.Rows = append(out.Rows, row)
	}
	return out
}

// GET /api/hris/org-chart/vacancy?version=&division=&region=&state=&format=xlsx
func (h handlers) orgChartVacancy(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	qs := r.URL.Query()
	state := strings.TrimSpace(qs.Get("state"))
	if _, known := vacancyLabels[state]; state != "" && (!known || state == domain.VacancyFilled) {
		return httpx.BadRequest("Keadaan tidak dikenal")
	}
	res, err := h.svc.orgGap(r.Context(), strings.TrimSpace(qs.Get("version")))
	if err != nil {
		return err
	}
	v := buildVacancy(res, strings.TrimSpace(qs.Get("division")), strings.TrimSpace(qs.Get("region")), state)
	if qs.Get("format") == "xlsx" {
		file, err := orgVacancyWorkbook(v)
		if err != nil {
			return err
		}
		return writeFile(w, file)
	}
	return ok(w, dataOf(v))
}

func orgVacancyWorkbook(v orgVacancyResult) (*exportFile, error) {
	header := []any{"Divisi", "Region", "Posisi", "Terisi", "Kosong", "Mungkin terisi", "Rencana", "Dibekukan"}
	summary := [][]any{{"Posisi Kosong " + v.Version.Label}, {""}, header}
	for _, g := range v.Groups {
		summary = append(summary, []any{g.DivisionName, g.Region, g.Positions, g.Filled, g.Empty, g.Maybe, g.Planned, g.Frozen})
	}
	s := v.Summary
	summary = append(summary, []any{"Total", "", s.Positions, s.Filled, s.Empty, s.Maybe, s.Planned, s.Frozen})
	rows := [][]any{{"Kode", "Jabatan", "Divisi", "Region", "Toko", "Atasan Bagan", "Keadaan", "Kemungkinan Pemegang"}}
	for _, r := range v.Rows {
		parent := r.ParentTitle
		if r.ParentCode != "" {
			parent = r.ParentTitle + " (" + r.ParentCode + ")"
		}
		rows = append(rows, []any{r.Code, r.Title, r.DivisionName, r.Region, strings.Join(r.StoreCodes, ", "), parent,
			vacancyLabels[r.State], strings.Join(r.MaybeHolders, ", ")})
	}
	data, err := xlsx.Build(
		xlsx.SheetSpec{Name: "Ringkasan", Rows: summary, ColumnWidths: []float64{12, 20, 10, 10, 10, 16, 10, 12}},
		xlsx.SheetSpec{Name: "Posisi", Rows: rows, ColumnWidths: []float64{10, 34, 22, 18, 14, 40, 16, 40}},
	)
	if err != nil {
		return nil, err
	}
	return &exportFile{data, xlsx.ContentType, "Posisi Kosong " + v.Version.Label + ".xlsx"}, nil
}

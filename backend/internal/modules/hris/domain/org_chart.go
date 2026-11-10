package domain

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// Bagan organisasi berversi (EPIC-101): validasi struktur (BR) dan
// pencocokan karyawan nyata ke slot bagan untuk laporan selisih. Approval
// tetap memakai reporting_to; bagan tidak mengubah data apa pun.

// OrgNode is one box of an org chart version.
type OrgNode struct {
	Code, ParentCode, Title                                     string
	IsPosition                                                  bool
	Level, EmploymentType, Vendor, DivisionCode, Region, Status string
	StoreCodes                                                  []string
}

// OrgIssue is one validation finding; Code is the node ("" = whole chart).
type OrgIssue struct {
	Rule    string `json:"rule"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// OrgAlias maps an HRIS job title (key: lower, trimmed) to a chart title.
type OrgAlias struct {
	Title, Region string
	NeedsReview   bool
}

// OrgEmployee is an active employee as the gap report reads them;
// StoreCode is the short code of the primary active placement.
type OrgEmployee struct {
	ID, Name, NIP, Title, ManagerID, StoreCode string
}

// OrgMatch is where an employee sits in the chart, or why not.
type OrgMatch struct {
	EmployeeID       string `json:"employee_id"`
	Name             string `json:"name"`
	NIP              string `json:"nip"`
	Title            string `json:"title"`
	Canonical        string `json:"canonical"`
	NodeCode         string `json:"node_code"`
	Problem          string `json:"problem"`
	ManagerProblem   string `json:"manager_problem"`
	AliasNeedsReview bool   `json:"alias_needs_review"`
}

// OrgGap compares the chart with the people.
type OrgGap struct {
	Matches            []OrgMatch
	Holders            map[string][]OrgEmployee // node code → matched employees
	EmptyNodes         []string
	TitlesOutside      map[string]int
	StoresOutside      []string
	ChartStoresMissing []string
}

var frontlineTitles = map[string]bool{"store leader": true, "shopkeeper": true, "perfumer": true,
	"patcher": true, "penjahit": true, "store support": true}

func levelRank(l string) (int, bool) {
	if len(l) != 2 || l[0] != 'L' {
		return 0, false
	}
	n, err := strconv.Atoi(l[1:])
	return n, err == nil
}

// effectiveParent skips grouping nodes (BR-09: Support is not a position).
func effectiveParent(byCode map[string]OrgNode, n OrgNode) (OrgNode, bool) {
	seen := map[string]bool{n.Code: true}
	p, ok := byCode[n.ParentCode]
	for ok && !p.IsPosition {
		if seen[p.Code] {
			return OrgNode{}, false
		}
		seen[p.Code] = true
		p, ok = byCode[p.ParentCode]
	}
	return p, ok
}

// ValidateOrgChart checks BR-01, 02, 03, 04, 05, 07, 09 and 10 of EPIC-101.
func ValidateOrgChart(nodes []OrgNode) (errs, warns []OrgIssue) {
	errs, warns = []OrgIssue{}, []OrgIssue{}
	byCode := map[string]OrgNode{}
	for _, n := range nodes {
		byCode[n.Code] = n
	}
	var roots []string
	for _, n := range nodes {
		if n.ParentCode == "" {
			roots = append(roots, n.Code)
		}
	}
	if len(roots) != 1 {
		errs = append(errs, OrgIssue{"BR-01", "", fmt.Sprintf("Bagan harus punya tepat satu puncak (ditemukan %d: %s)",
			len(roots), strings.Join(roots, ", "))})
	}
	inCycle := map[string]bool{}
	for _, n := range nodes {
		if n.ParentCode != "" {
			if _, ok := byCode[n.ParentCode]; !ok {
				errs = append(errs, OrgIssue{"BR-03", n.Code, "Atasan " + n.ParentCode + " tidak ada di versi ini"})
			}
		}
		// BR-02: walk up; a node seen twice is a cycle, reported once at its
		// smallest code.
		path := map[string]bool{}
		cur, ok := n, true
		for ok && cur.ParentCode != "" {
			if path[cur.Code] {
				var cycle []string
				start := cur.Code
				for c := byCode[start]; ; c = byCode[c.ParentCode] {
					cycle = append(cycle, c.Code)
					if c.ParentCode == start {
						break
					}
				}
				sort.Strings(cycle)
				if !inCycle[cycle[0]] {
					for _, c := range cycle {
						inCycle[c] = true
					}
					errs = append(errs, OrgIssue{"BR-02", cycle[0], "Siklus garis lapor di " + cycle[0]})
				}
				break
			}
			path[cur.Code] = true
			cur, ok = byCode[cur.ParentCode]
		}
	}
	for _, n := range nodes {
		if !n.IsPosition {
			if n.Level != "" || n.EmploymentType != "" {
				errs = append(errs, OrgIssue{"BR-09", n.Code, "Node grup tidak boleh punya level"})
			}
			continue
		}
		if p, ok := effectiveParent(byCode, n); ok && !inCycle[n.Code] {
			c, okc := levelRank(n.Level)
			r, okp := levelRank(p.Level)
			switch {
			case okc && okp && c == r:
				warns = append(warns, OrgIssue{"BR-04", n.Code, fmt.Sprintf("Level %s sama dengan atasan %s (%s)", n.Level, p.Code, p.Level)})
			case okc && okp && c < r:
				errs = append(errs, OrgIssue{"BR-04", n.Code, fmt.Sprintf("Level %s lebih senior dari atasan %s (%s)", n.Level, p.Code, p.Level)})
			}
		}
		if n.DivisionCode == "RTL" && frontlineTitles[strings.ToLower(n.Title)] && len(n.StoreCodes) == 0 {
			errs = append(errs, OrgIssue{"BR-05", n.Code, n.Title + " di Retail wajib punya kode toko"})
		}
		if t := strings.ToLower(strings.TrimSpace(n.Title)); t == "intern" || t == "freelance" {
			errs = append(errs, OrgIssue{"BR-07", n.Code, fmt.Sprintf("Judul %q bukan jabatan — jadikan tipe kepegawaian", strings.TrimSpace(n.Title))})
		}
		if n.EmploymentType == "OUTSOURCE" && strings.TrimSpace(n.Vendor) == "" {
			errs = append(errs, OrgIssue{"BR-10", n.Code, "Posisi outsource wajib punya vendor"})
		}
	}
	return errs, warns
}

func normTitle(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

func hasStore(n OrgNode, code string) bool {
	for _, s := range n.StoreCodes {
		if s == code {
			return true
		}
	}
	return false
}

// MatchOrgChart places every employee in the chart (ruling R9) and lists
// what does not line up: ambiguous or unknown titles, a manager other than
// the chart's (checked against the first ancestor that has a holder), empty
// positions and stores on one side only.
func MatchOrgChart(nodes []OrgNode, emps []OrgEmployee, aliases map[string]OrgAlias, activeStores []string) OrgGap {
	g := OrgGap{Holders: map[string][]OrgEmployee{}, TitlesOutside: map[string]int{},
		EmptyNodes: []string{}, StoresOutside: []string{}, ChartStoresMissing: []string{}}
	byCode := map[string]OrgNode{}
	for _, n := range nodes {
		byCode[n.Code] = n
	}
	// Alias keys are compared the way titles are (lower, single spaces).
	aliasOf := map[string]OrgAlias{}
	for k, a := range aliases {
		aliasOf[normTitle(k)] = a
	}
	nodeOf := map[string]string{} // employee → node
	for _, e := range emps {
		m := OrgMatch{EmployeeID: e.ID, Name: e.Name, NIP: e.NIP, Title: e.Title, Canonical: e.Title}
		region := ""
		if a, ok := aliasOf[normTitle(e.Title)]; ok {
			m.Canonical, region, m.AliasNeedsReview = a.Title, a.Region, a.NeedsReview
		}
		var cands []OrgNode
		for _, n := range nodes {
			if n.IsPosition && normTitle(n.Title) == normTitle(m.Canonical) && (region == "" || n.Region == region) {
				cands = append(cands, n)
			}
		}
		if len(cands) > 1 && e.StoreCode != "" {
			var at []OrgNode
			for _, n := range cands {
				if hasStore(n, e.StoreCode) {
					at = append(at, n)
				}
			}
			if len(at) > 0 {
				cands = at
			}
		}
		switch len(cands) {
		case 0:
			if strings.TrimSpace(e.Title) == "" {
				m.Problem = "Belum punya jabatan di HRIS"
				g.TitlesOutside["(tanpa jabatan)"]++
				break
			}
			m.Problem = "Jabatan tidak ada di bagan"
			g.TitlesOutside[strings.TrimSpace(e.Title)]++
		case 1:
			m.NodeCode = cands[0].Code
			nodeOf[e.ID] = m.NodeCode
			// The only slot for this title is tied to other stores: still
			// placed (R9), but reported.
			if c := cands[0]; e.StoreCode != "" && len(c.StoreCodes) > 0 && !hasStore(c, e.StoreCode) {
				m.Problem = fmt.Sprintf("Toko %s tidak ada di posisi bagan %s (%s)", e.StoreCode, c.Code, strings.Join(c.StoreCodes, ", "))
			}
			g.Holders[m.NodeCode] = append(g.Holders[m.NodeCode], e)
		default:
			m.Problem = fmt.Sprintf("Cocok ke %d posisi bagan — tidak bisa dipastikan", len(cands))
		}
		g.Matches = append(g.Matches, m)
	}
	names := map[string]string{}
	for _, e := range emps {
		names[e.ID] = e.Name
	}
	for i, m := range g.Matches {
		if m.NodeCode == "" {
			continue
		}
		// First ancestor position with a holder; a chart top has none to check.
		// A seen set stops the climb on a cyclic chart.
		p, ok := effectiveParent(byCode, byCode[m.NodeCode])
		climbed := map[string]bool{m.NodeCode: true}
		for ok && len(g.Holders[p.Code]) == 0 && !climbed[p.Code] {
			climbed[p.Code] = true
			p, ok = effectiveParent(byCode, p)
		}
		if !ok || climbed[p.Code] {
			continue
		}
		var mgrID string
		for _, e := range emps {
			if e.ID == m.EmployeeID {
				mgrID = e.ManagerID
			}
		}
		// An active manager who is not placed in the chart (unknown or
		// ambiguous title) cannot be compared: their node may be the empty
		// one the climb skipped.
		if mgrID != "" && names[mgrID] != "" && nodeOf[mgrID] == "" {
			continue
		}
		if nodeOf[mgrID] == p.Code && mgrID != "" {
			continue
		}
		data := "tidak ada"
		if mgrID != "" {
			data = names[mgrID]
			if data == "" {
				data = "(tidak aktif)"
			}
		}
		g.Matches[i].ManagerProblem = fmt.Sprintf("Atasan di bagan: %s (%s); di data: %s", p.Title, p.Code, data)
	}
	chartStores := map[string]bool{}
	for _, n := range nodes {
		if n.IsPosition && len(g.Holders[n.Code]) == 0 {
			g.EmptyNodes = append(g.EmptyNodes, n.Code)
		}
		for _, s := range n.StoreCodes {
			chartStores[s] = true
		}
	}
	active := map[string]bool{}
	for _, s := range activeStores {
		active[s] = true
		if !chartStores[s] {
			g.StoresOutside = append(g.StoresOutside, s)
		}
	}
	for s := range chartStores {
		if !active[s] {
			g.ChartStoresMissing = append(g.ChartStoresMissing, s)
		}
	}
	sort.Strings(g.StoresOutside)
	sort.Strings(g.ChartStoresMissing)
	return g
}

// LoadOrgSeedCSV reads the client seed (paddy_org_positions_seed.csv);
// status follows ruling R5 (HIGHLIGHTED_NO_LEGEND = planned).
func LoadOrgSeedCSV(r io.Reader) ([]OrgNode, error) {
	recs, err := csv.NewReader(r).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return nil, fmt.Errorf("CSV kosong")
	}
	col := map[string]int{}
	for i, h := range recs[0] {
		col[h] = i
	}
	for _, need := range []string{"position_code", "parent_code", "normalized_title", "is_position", "job_level",
		"employment_type", "vendor", "division_code", "region", "store_codes", "flags"} {
		if _, ok := col[need]; !ok {
			return nil, fmt.Errorf("kolom %s tidak ada", need)
		}
	}
	var out []OrgNode
	for _, r := range recs[1:] {
		get := func(k string) string { return strings.TrimSpace(r[col[k]]) }
		n := OrgNode{Code: get("position_code"), ParentCode: get("parent_code"), Title: get("normalized_title"),
			IsPosition: get("is_position") == "Y", Level: get("job_level"), EmploymentType: get("employment_type"),
			Vendor: get("vendor"), DivisionCode: get("division_code"), Region: get("region"), Status: "active"}
		if strings.Contains(get("flags"), "HIGHLIGHTED_NO_LEGEND") {
			n.Status = "planned"
		}
		for _, s := range strings.Split(get("store_codes"), ";") {
			if s != "" {
				n.StoreCodes = append(n.StoreCodes, s)
			}
		}
		out = append(out, n)
	}
	return out, nil
}

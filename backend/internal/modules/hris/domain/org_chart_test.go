package domain

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestValidateOrgChartDraft6(t *testing.T) {
	f, err := os.Open("testdata/org_draft6.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	nodes, err := LoadOrgSeedCSV(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 150 {
		t.Fatalf("nodes = %d", len(nodes))
	}
	errs, warns := ValidateOrgChart(nodes)
	if len(errs) != 0 {
		t.Errorf("errors %+v", errs)
	}
	var codes []string
	for _, w := range warns {
		if w.Rule != "BR-04" {
			t.Errorf("unexpected warning %+v", w)
		}
		codes = append(codes, w.Code)
	}
	if !reflect.DeepEqual(codes, []string{"HRG-004", "HRG-010", "MKT-004"}) {
		t.Errorf("warnings %+v", warns)
	}
	if len(warns) > 0 && warns[0].Message != "Level L7 sama dengan atasan HRG-003 (L7)" {
		t.Errorf("message %q", warns[0].Message)
	}
}

func hasOrgIssue(list []OrgIssue, rule, code, msg string) bool {
	for _, i := range list {
		if i.Rule == rule && i.Code == code && i.Message == msg {
			return true
		}
	}
	return false
}

func TestValidateOrgChartRules(t *testing.T) {
	pos := func(code, parent, title, level string) OrgNode {
		return OrgNode{Code: code, ParentCode: parent, Title: title, IsPosition: true, Level: level,
			EmploymentType: "PERMANENT_OR_CONTRACT", DivisionCode: "OPS", Status: "active"}
	}
	errs, _ := ValidateOrgChart([]OrgNode{pos("A", "", "CEO", "L0"), pos("B", "", "COO", "L0")})
	if !hasOrgIssue(errs, "BR-01", "", "Bagan harus punya tepat satu puncak (ditemukan 2: A, B)") {
		t.Errorf("BR-01 %+v", errs)
	}
	errs, _ = ValidateOrgChart([]OrgNode{pos("R", "", "CEO", "L0"), pos("A", "B", "X", "L7"), pos("B", "A", "Y", "L7")})
	if !hasOrgIssue(errs, "BR-02", "A", "Siklus garis lapor di A") {
		t.Errorf("BR-02 %+v", errs)
	}
	errs, _ = ValidateOrgChart([]OrgNode{pos("R", "", "CEO", "L0"), pos("C", "ZZZ", "X", "L7")})
	if !hasOrgIssue(errs, "BR-03", "C", "Atasan ZZZ tidak ada di versi ini") {
		t.Errorf("BR-03 %+v", errs)
	}
	errs, _ = ValidateOrgChart([]OrgNode{pos("R", "", "CEO", "L0"), pos("X", "R", "Supervisor", "L4"), pos("Y", "X", "Manager", "L2")})
	if !hasOrgIssue(errs, "BR-04", "Y", "Level L2 lebih senior dari atasan X (L4)") {
		t.Errorf("BR-04 %+v", errs)
	}
	shop := pos("S", "R", "Shopkeeper", "L7")
	shop.DivisionCode = "RTL"
	errs, _ = ValidateOrgChart([]OrgNode{pos("R", "", "CEO", "L0"), shop})
	if !hasOrgIssue(errs, "BR-05", "S", "Shopkeeper di Retail wajib punya kode toko") {
		t.Errorf("BR-05 %+v", errs)
	}
	errs, _ = ValidateOrgChart([]OrgNode{pos("R", "", "CEO", "L0"), pos("I", "R", "Intern", "L8")})
	if !hasOrgIssue(errs, "BR-07", "I", `Judul "Intern" bukan jabatan — jadikan tipe kepegawaian`) {
		t.Errorf("BR-07 %+v", errs)
	}
	group := OrgNode{Code: "G", ParentCode: "R", Title: "Support", Level: "L5", DivisionCode: "SUP"}
	errs, _ = ValidateOrgChart([]OrgNode{pos("R", "", "CEO", "L0"), group})
	if !hasOrgIssue(errs, "BR-09", "G", "Node grup tidak boleh punya level") {
		t.Errorf("BR-09 %+v", errs)
	}
	sec := pos("O", "R", "Security", "L7")
	sec.EmploymentType = "OUTSOURCE"
	errs, _ = ValidateOrgChart([]OrgNode{pos("R", "", "CEO", "L0"), sec})
	if !hasOrgIssue(errs, "BR-10", "O", "Posisi outsource wajib punya vendor") {
		t.Errorf("BR-10 %+v", errs)
	}
}

func matchFixture() ([]OrgNode, map[string]OrgAlias) {
	n := func(code, parent, title, region string, stores ...string) OrgNode {
		return OrgNode{Code: code, ParentCode: parent, Title: title, IsPosition: true, Region: region, StoreCodes: stores, Status: "active"}
	}
	nodes := []OrgNode{
		n("S1", "", "Store Supervisor", "Jawa Barat 1"),
		n("L1", "S1", "Store Leader", "Jawa Barat 1", "PPGA"),
		n("K1", "L1", "Shopkeeper", "Jawa Barat 1", "PPGA"),
		n("K2", "S1", "Shopkeeper", "Jawa Barat 1", "MPVJ"),
		n("K3", "S1", "Shopkeeper", "Jawa Barat 1", "PPGI"),
		n("F1", "", "Finance Staff", ""),
	}
	aliases := map[string]OrgAlias{
		"shopkeeper staff":                {Title: "Shopkeeper"},
		"store supervisor - jawa barat 1": {Title: "Store Supervisor", Region: "Jawa Barat 1"},
		"shopkeeper coordinator":          {Title: "Store Leader", NeedsReview: true},
	}
	return nodes, aliases
}

func matchOf(g OrgGap, id string) OrgMatch {
	for _, m := range g.Matches {
		if m.EmployeeID == id {
			return m
		}
	}
	return OrgMatch{}
}

func TestMatchOrgChart(t *testing.T) {
	nodes, aliases := matchFixture()
	emps := []OrgEmployee{
		{ID: "dewi", Name: "Dewi", Title: "Store Supervisor - Jawa Barat 1"},
		{ID: "lia", Name: "Lia", Title: "Shopkeeper Coordinator", StoreCode: "PPGA", ManagerID: "dewi"},
		{ID: "ani", Name: "Ani", Title: "Shopkeeper Staff", StoreCode: "PPGA", ManagerID: "lia"},
		{ID: "budi", Name: "Budi", Title: "Shopkeeper Staff", StoreCode: "MPVJ", ManagerID: "dewi"},
		{ID: "cici", Name: "Cici", Title: "Shopkeeper Staff"},
		{ID: "eko", Name: "Eko", Title: "Warehouse Staff"},
		{ID: "fani", Name: "Fani", Title: "Shopkeeper Staff", StoreCode: "PPGA", ManagerID: "dewi"},
	}
	g := MatchOrgChart(nodes, emps, aliases, []string{"PPGA", "MPVJ", "PPGI", "BXST"})
	want := map[string][2]string{
		"dewi": {"S1", ""}, "lia": {"L1", ""}, "ani": {"K1", ""}, "budi": {"K2", ""},
		"cici": {"", "Cocok ke 3 posisi bagan — tidak bisa dipastikan"},
		"eko":  {"", "Jabatan tidak ada di bagan"},
		"fani": {"K1", ""},
	}
	for id, w := range want {
		m := matchOf(g, id)
		if m.NodeCode != w[0] || m.Problem != w[1] {
			t.Errorf("%s: node %q problem %q, want %q %q", id, m.NodeCode, m.Problem, w[0], w[1])
		}
	}
	if m := matchOf(g, "lia"); !m.AliasNeedsReview || m.ManagerProblem != "" || m.Canonical != "Store Leader" {
		t.Errorf("lia %+v", m)
	}
	for _, id := range []string{"dewi", "ani", "budi"} {
		if m := matchOf(g, id); m.ManagerProblem != "" {
			t.Errorf("%s manager problem %q", id, m.ManagerProblem)
		}
	}
	if m := matchOf(g, "fani"); m.ManagerProblem != "Atasan di bagan: Store Leader (L1); di data: Dewi" {
		t.Errorf("fani manager problem %q", m.ManagerProblem)
	}
	if !reflect.DeepEqual(g.EmptyNodes, []string{"K3", "F1"}) {
		t.Errorf("empty nodes %v", g.EmptyNodes)
	}
	if g.TitlesOutside["Warehouse Staff"] != 1 || len(g.TitlesOutside) != 1 {
		t.Errorf("titles outside %v", g.TitlesOutside)
	}
	if !reflect.DeepEqual(g.StoresOutside, []string{"BXST"}) || len(g.ChartStoresMissing) != 0 {
		t.Errorf("stores outside %v missing %v", g.StoresOutside, g.ChartStoresMissing)
	}
}

// An empty parent node (no Store Leader) passes the check up to the first
// ancestor that has a holder (Review Focus 2).
func TestMatchOrgChartEmptyParent(t *testing.T) {
	nodes, aliases := matchFixture()
	emps := []OrgEmployee{
		{ID: "dewi", Name: "Dewi", Title: "Store Supervisor - Jawa Barat 1"},
		{ID: "ani", Name: "Ani", Title: "Shopkeeper Staff", StoreCode: "PPGA", ManagerID: "dewi"},
	}
	g := MatchOrgChart(nodes, emps, aliases, nil)
	if m := matchOf(g, "ani"); m.NodeCode != "K1" || m.ManagerProblem != "" {
		t.Errorf("ani %+v", m)
	}
	if !reflect.DeepEqual(g.ChartStoresMissing, []string{"MPVJ", "PPGA", "PPGI"}) {
		t.Errorf("chart stores missing %v", g.ChartStoresMissing)
	}
	if !strings.Contains(strings.Join(g.EmptyNodes, ","), "L1") {
		t.Errorf("empty nodes %v", g.EmptyNodes)
	}
}

// A manager whose own title is not in the chart leaves their node empty; their
// people must not be reported against the grandparent (final review).
func TestMatchOrgChartUnmatchedManager(t *testing.T) {
	nodes, aliases := matchFixture()
	// K1's parent L1 (Store Leader) is empty because Rina's title "Area Lead"
	// is not in the chart; the climb would otherwise reach Dewi at S1.
	emps := []OrgEmployee{
		{ID: "dewi", Name: "Dewi", Title: "Store Supervisor - Jawa Barat 1"},
		{ID: "rina", Name: "Rina", Title: "Area Lead", ManagerID: "dewi"},
		{ID: "ani", Name: "Ani", Title: "Shopkeeper Staff", StoreCode: "PPGA", ManagerID: "rina"},
	}
	g := MatchOrgChart(nodes, emps, aliases, nil)
	if m := matchOf(g, "ani"); m.NodeCode != "K1" || m.ManagerProblem != "" {
		t.Errorf("ani %+v", m)
	}
}

// A cycle of empty positions must not hang the climb (final review).
func TestMatchOrgChartCycleTerminates(t *testing.T) {
	nodes := []OrgNode{
		{Code: "A", ParentCode: "B", Title: "Alpha", IsPosition: true},
		{Code: "B", ParentCode: "A", Title: "Beta", IsPosition: true},
		{Code: "C", ParentCode: "A", Title: "Gamma", IsPosition: true},
	}
	done := make(chan OrgGap, 1)
	go func() { done <- MatchOrgChart(nodes, []OrgEmployee{{ID: "x", Name: "X", Title: "Gamma"}}, nil, nil) }()
	select {
	case g := <-done:
		if m := matchOf(g, "x"); m.NodeCode != "C" {
			t.Errorf("x %+v", m)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("MatchOrgChart did not return on a cyclic chart")
	}
}

// An employee without a job title gets a readable reason and label, not an
// empty row in "Jabatan di Luar Bagan" (staging QA).
func TestMatchOrgChartNoTitle(t *testing.T) {
	nodes, aliases := matchFixture()
	g := MatchOrgChart(nodes, []OrgEmployee{{ID: "x", Name: "X", Title: "  "}}, aliases, nil)
	if m := matchOf(g, "x"); m.Problem != "Belum punya jabatan di HRIS" || m.NodeCode != "" {
		t.Errorf("x %+v", m)
	}
	if g.TitlesOutside["(tanpa jabatan)"] != 1 || len(g.TitlesOutside) != 1 {
		t.Errorf("titles outside %v", g.TitlesOutside)
	}
}

func TestMatchOrgChartSingleCandidateWrongStore(t *testing.T) {
	nodes := []OrgNode{
		{Code: "R", Title: "CEO", IsPosition: true, Status: "active"},
		{Code: "P", ParentCode: "R", Title: "Penjahit", IsPosition: true, StoreCodes: []string{"PBSA"}, Status: "active"},
		{Code: "S", ParentCode: "R", Title: "Store Support", IsPosition: true, StoreCodes: []string{"PPMR"}, Status: "active"},
	}
	emps := []OrgEmployee{
		{ID: "1", Name: "Jahit Lain", Title: "Penjahit", StoreCode: "PLBM"},
		{ID: "2", Name: "Jahit Benar", Title: "Penjahit", StoreCode: "PBSA"},
		{ID: "3", Name: "Support Tanpa Toko", Title: "Store Support"},
	}
	g := MatchOrgChart(nodes, emps, nil, nil)
	if g.Matches[0].NodeCode != "P" || g.Matches[0].Problem != "Toko PLBM tidak ada di posisi bagan P (PBSA)" {
		t.Errorf("wrong store %+v", g.Matches[0])
	}
	if g.Matches[1].NodeCode != "P" || g.Matches[1].Problem != "" {
		t.Errorf("right store %+v", g.Matches[1])
	}
	if g.Matches[2].NodeCode != "S" || g.Matches[2].Problem != "" {
		t.Errorf("no store %+v", g.Matches[2])
	}
}

func TestMatchOrgChartAliasKeyDoubleSpace(t *testing.T) {
	nodes := []OrgNode{{Code: "R", Title: "CEO", IsPosition: true}, {Code: "A", ParentCode: "R", Title: "Store Leader", IsPosition: true}}
	aliases := map[string]OrgAlias{"kepala  toko": {Title: "Store Leader", NeedsReview: true}}
	g := MatchOrgChart(nodes, []OrgEmployee{{ID: "1", Name: "X", Title: "Kepala Toko"}}, aliases, nil)
	if g.Matches[0].NodeCode != "A" || !g.Matches[0].AliasNeedsReview {
		t.Errorf("alias %+v", g.Matches[0])
	}
}

func TestOrgChartPositionUnderGroupingNode(t *testing.T) {
	nodes := []OrgNode{
		{Code: "R", Title: "CEO", IsPosition: true, Level: "L0"},
		{Code: "M", ParentCode: "R", Title: "Manager", IsPosition: true, Level: "L2"},
		{Code: "G", ParentCode: "M", Title: "Support"},
		{Code: "S", ParentCode: "G", Title: "Staff", IsPosition: true, Level: "L2"},
	}
	errs, warns := ValidateOrgChart(nodes)
	if len(errs) != 0 || len(warns) != 1 || warns[0].Code != "S" || warns[0].Message != "Level L2 sama dengan atasan M (L2)" {
		t.Errorf("errs %+v warns %+v", errs, warns)
	}
	emps := []OrgEmployee{{ID: "m", Name: "Bu Manager", Title: "Manager"}, {ID: "s", Name: "Staf", Title: "Staff", ManagerID: "x"},
		{ID: "x", Name: "Orang Lain", Title: "CEO"}}
	g := MatchOrgChart(nodes, emps, nil, nil)
	if g.Matches[1].ManagerProblem != "Atasan di bagan: Manager (M); di data: Orang Lain" {
		t.Errorf("manager %+v", g.Matches[1])
	}
	for _, c := range g.EmptyNodes {
		if c == "G" {
			t.Errorf("grouping node listed as empty position")
		}
	}
}

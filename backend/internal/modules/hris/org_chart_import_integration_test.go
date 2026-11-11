package hris

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/testutil"
	"nuhabit/backend/internal/platform/xlsx"
)

// orgTemplateMatrix downloads the Bagan template of a version and parses it.
func orgTemplateMatrix(t *testing.T, h *harness, s *testutil.Staff, version string) ([][]string, *httptest.ResponseRecorder) {
	t.Helper()
	path := "/api/hris/org-chart/template"
	if version != "" {
		path += "?version=" + strings.ReplaceAll(version, " ", "%20")
	}
	rec := h.send(s, httptest.NewRequest("GET", path, nil))
	if rec.Code != 200 {
		t.Fatalf("template = %d %s", rec.Code, rec.Body.String())
	}
	m, err := xlsx.ParseMatrix(rec.Body.Bytes(), xlsx.ReadOptions{PreferSheet: domain.OrgChartSheet})
	if err != nil {
		t.Fatal(err)
	}
	return m, rec
}

// EPIC-103 R2: the template is the shown version, every cell as text.
func TestOrgChartTemplate(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", iam.HrisOrganization, false)
	none, _ := h.staff("pos", nil, false)
	expect(t, h.do(&none, "GET", "/api/hris/org-chart/template", nil), 403, "")
	expect(t, h.do(&hr, "GET", "/api/hris/org-chart/template?version=Draft%2099", nil), 404, "Versi bagan tidak ditemukan")

	m, rec := orgTemplateMatrix(t, h, &hr, "Draft 6")
	if rec.Header().Get("Content-Type") != xlsx.ContentType || !strings.Contains(rec.Header().Get("Content-Disposition"), "Template Bagan Draft 6.xlsx") {
		t.Fatalf("headers %v", rec.Header())
	}
	if strings.Join(m[0], "|") != strings.Join(domain.OrgChartHeaders, "|") {
		t.Fatalf("header %v", m[0])
	}
	if len(m) != 151 {
		t.Fatalf("rows %d", len(m)-1)
	}
	if got := strings.Join(m[1][:8], "|"); got != "EXE-001||CEO|Ya|EXE|Executive|L0|Tetap/Kontrak" {
		t.Fatalf("first row %q", got)
	}
	var planned, stores bool
	for _, r := range m[1:] {
		planned = planned || r[11] == "Rencana"
		stores = stores || strings.Contains(r[10], ", ")
	}
	if !planned || !stores {
		t.Fatalf("planned %v multi-store %v", planned, stores)
	}
	help, err := xlsx.ParseMatrix(rec.Body.Bytes(), xlsx.ReadOptions{PreferSheet: "Petunjuk"})
	if err != nil {
		t.Fatal(err)
	}
	text, _ := json.Marshal(help)
	if !strings.Contains(string(text), "Dibekukan") || !strings.Contains(string(text), "Tetap/Kontrak") {
		t.Fatalf("Petunjuk %s", text)
	}
	// The template parses back without errors (round-trip, Review Focus 1).
	res, problem := domain.ParseOrgChartUpload(m, nil)
	if problem != "" || len(res.Errors) != 0 || len(res.Nodes) != 150 {
		t.Fatalf("round-trip problem %q errors %+v nodes %d", problem, res.Errors, len(res.Nodes))
	}
}

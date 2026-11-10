package hris

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/xlsx"
)

// EPIC-101: the org chart API (versions, tree, validation, gap report).
func TestOrgChartAPI(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", iam.HrisOrganization, false)
	none, _ := h.staff("pos", nil, false)

	expect(t, h.do(&none, "GET", "/api/hris/org-chart/versions", nil), 403, "")
	v := h.do(&hr, "GET", "/api/hris/org-chart/versions", nil)
	expect(t, v, 200, "")
	if !strings.Contains(v.raw, `"label":"Draft 6"`) || !strings.Contains(v.raw, `"nodes":150`) {
		t.Fatalf("versions %s", v.raw)
	}

	tree := h.do(&hr, "GET", "/api/hris/org-chart/tree?division=RTL&region=Makassar", nil)
	expect(t, tree, 200, "")
	var codes []string
	for _, n := range tree.data()["nodes"].([]any) {
		codes = append(codes, n.(map[string]any)["code"].(string))
	}
	if strings.Join(codes, ",") != "RTL-053,RTL-054,RTL-055,RTL-056" {
		t.Fatalf("Makassar nodes %v", codes)
	}
	empty := h.do(&hr, "GET", "/api/hris/org-chart/tree?division=XXX", nil)
	expect(t, empty, 200, "")
	if n := len(empty.data()["nodes"].([]any)); n != 0 {
		t.Fatalf("unknown division nodes = %d", n)
	}
	expect(t, h.do(&hr, "GET", "/api/hris/org-chart/tree?version=Draft%2099", nil), 404, "Versi bagan tidak ditemukan")

	val := h.do(&hr, "GET", "/api/hris/org-chart/validation", nil)
	expect(t, val, 200, "")
	if len(val.data()["errors"].([]any)) != 0 || len(val.data()["warnings"].([]any)) != 3 {
		t.Fatalf("validation %s", val.raw)
	}

	// A Perfumer at PLBM (code still ODOO-PLBM) lands on RTL-023 through the
	// alias "perfumer staff" → Perfumer and the store (Review Focus 3).
	branch := h.scalar(`INSERT INTO configuration.branches (company_id, name, code)
		SELECT id, 'PLBM - Uji Bagan', 'ODOO-PLBM' FROM configuration.companies ORDER BY created_at LIMIT 1 RETURNING id::text`)
	title := h.scalar(`INSERT INTO hris.positions (title, level, is_active) VALUES ('Perfumer Staff', 'Staff', true) RETURNING id::text`)
	emp := h.employee(emp{name: "Perfumer Uji Bagan"})
	h.exec(`UPDATE hris.employees SET job_title_id = $2 WHERE id = $1`, emp, title)
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, start_date, is_primary) VALUES ($1, $2, '2026-01-01', true)`, emp, branch)

	gap := h.do(&hr, "GET", "/api/hris/org-chart/gap", nil)
	expect(t, gap, 200, "")
	var found bool
	for _, m := range gap.data()["matches"].([]any) {
		mm := m.(map[string]any)
		if mm["employee_id"] == emp {
			found = true
			if mm["node_code"] != "RTL-023" || mm["canonical"] != "Perfumer" {
				t.Fatalf("perfumer match %v", mm)
			}
		}
	}
	if !found {
		t.Fatalf("employee missing from gap: %s", gap.raw)
	}
	if _, ok := gap.data()["summary"].(map[string]any)["employees"]; !ok {
		t.Fatalf("summary %s", gap.raw)
	}
	tree = h.do(&hr, "GET", "/api/hris/org-chart/tree?division=RTL&region=Jabodetabek%201", nil)
	raw, _ := json.Marshal(tree.data())
	if !strings.Contains(string(raw), "Perfumer Uji Bagan") {
		t.Fatalf("holder missing from tree: %s", raw)
	}

	book := h.send(&hr, httptest.NewRequest("GET", "/api/hris/org-chart/gap?format=xlsx", nil))
	if book.Code != 200 || book.Header().Get("Content-Type") != xlsx.ContentType {
		t.Fatalf("xlsx = %d %s", book.Code, book.Body.String())
	}
	m, err := xlsx.ParseMatrix(book.Body.Bytes(), xlsx.ReadOptions{PreferSheet: "Karyawan"})
	if err != nil {
		t.Fatal(err)
	}
	text, _ := json.Marshal(m)
	if !strings.Contains(string(text), "Perfumer Uji Bagan") || !strings.Contains(string(text), "RTL-023") {
		t.Fatalf("Karyawan sheet %s", text)
	}
}

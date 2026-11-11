package hris

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/xlsx"
)

func vacancyNum(t *testing.T, m map[string]any, k string) int {
	t.Helper()
	f, ok := m[k].(float64)
	if !ok {
		t.Fatalf("%s missing in %v", k, m)
	}
	return int(f)
}

// EPIC-102: Posisi Kosong on the seeded Draft 6 (counts are relative: the
// test DB may hold other employees).
func TestOrgChartVacancyAPI(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", iam.HrisOrganization, false)
	none, _ := h.staff("pos", nil, false)

	expect(t, h.do(&none, "GET", "/api/hris/org-chart/vacancy", nil), 403, "")
	all := h.do(&hr, "GET", "/api/hris/org-chart/vacancy", nil)
	expect(t, all, 200, "")
	sum := all.data()["summary"].(map[string]any)
	if vacancyNum(t, sum, "positions") != 149 {
		t.Fatalf("summary %v", sum)
	}
	total := 0
	for _, k := range []string{"filled", "empty", "maybe", "planned", "frozen"} {
		total += vacancyNum(t, sum, k)
	}
	if total != 149 {
		t.Fatalf("states sum %d: %v", total, sum)
	}
	if vacancyNum(t, sum, "planned") != 9 {
		t.Fatalf("planned %v", sum)
	}
	rows := all.data()["rows"].([]any)
	if len(rows) != 149-vacancyNum(t, sum, "filled") {
		t.Fatalf("rows %d vs summary %v", len(rows), sum)
	}
	groupPositions := 0
	var hqKept bool
	for _, g := range all.data()["groups"].([]any) {
		gm := g.(map[string]any)
		groupPositions += vacancyNum(t, gm, "positions")
		if gm["region"] == "" {
			hqKept = true
		}
	}
	if groupPositions != 149 || !hqKept {
		t.Fatalf("groups %v", all.data()["groups"])
	}
	for _, r := range rows {
		rm := r.(map[string]any)
		if rm["state"] == "filled" {
			t.Fatalf("filled row %v", rm)
		}
		if rm["code"] == "EXE-001" && rm["parent_title"] != "" {
			t.Fatalf("top parent title %v", rm)
		}
	}

	mks := h.do(&hr, "GET", "/api/hris/org-chart/vacancy?division=RTL&region=Makassar", nil)
	expect(t, mks, 200, "")
	groups := mks.data()["groups"].([]any)
	if len(groups) != 1 {
		t.Fatalf("Makassar groups %v", groups)
	}
	g := groups[0].(map[string]any)
	if g["division"] != "RTL" || g["division_name"] != "Retail Operational" || g["region"] != "Makassar" || vacancyNum(t, g, "positions") != 4 {
		t.Fatalf("Makassar group %v", g)
	}
	for _, r := range mks.data()["rows"].([]any) {
		rm := r.(map[string]any)
		if c := rm["code"].(string); c < "RTL-053" || c > "RTL-056" {
			t.Fatalf("Makassar row %v", rm)
		}
		if rm["parent_title"] == "" {
			t.Fatalf("Makassar row without chart parent %v", rm)
		}
	}

	planned := h.do(&hr, "GET", "/api/hris/org-chart/vacancy?state=planned", nil)
	expect(t, planned, 200, "")
	prow := planned.data()["rows"].([]any)
	if len(prow) != 9 {
		t.Fatalf("planned rows %d", len(prow))
	}
	for _, r := range prow {
		if r.(map[string]any)["state"] != "planned" {
			t.Fatalf("planned filter %v", r)
		}
	}
	// The state filter narrows rows, not the summary.
	if vacancyNum(t, planned.data()["summary"].(map[string]any), "positions") != 149 {
		t.Fatalf("planned summary %v", planned.data()["summary"])
	}
	expect(t, h.do(&hr, "GET", "/api/hris/org-chart/vacancy?state=bogus", nil), 400, "Keadaan tidak dikenal")
	expect(t, h.do(&hr, "GET", "/api/hris/org-chart/vacancy?version=Draft%2099", nil), 404, "Versi bagan tidak ditemukan")

	book := h.send(&hr, httptest.NewRequest("GET", "/api/hris/org-chart/vacancy?format=xlsx", nil))
	if book.Code != 200 || book.Header().Get("Content-Type") != xlsx.ContentType {
		t.Fatalf("xlsx = %d %s", book.Code, book.Body.String())
	}
	if !strings.Contains(book.Header().Get("Content-Disposition"), "Posisi Kosong Draft 6.xlsx") {
		t.Fatalf("filename %q", book.Header().Get("Content-Disposition"))
	}
	for _, sheet := range []string{"Ringkasan", "Posisi"} {
		m, err := xlsx.ParseMatrix(book.Body.Bytes(), xlsx.ReadOptions{PreferSheet: sheet})
		if err != nil {
			t.Fatal(err)
		}
		text, _ := json.Marshal(m)
		want := map[string]string{"Ringkasan": "Mungkin terisi", "Posisi": "Kemungkinan Pemegang"}[sheet]
		if !strings.Contains(string(text), want) {
			t.Fatalf("%s sheet %s", sheet, text)
		}
	}

	// A Perfumer at PLBM fills RTL-023 (EPIC-101 alias + store).
	inRows := func(c call, code string) bool {
		for _, r := range c.data()["rows"].([]any) {
			if r.(map[string]any)["code"] == code {
				return true
			}
		}
		return false
	}
	if !inRows(all, "RTL-023") {
		t.Fatalf("RTL-023 should be open before the fixture")
	}
	branch := h.scalar(`INSERT INTO configuration.branches (company_id, name, code)
		SELECT id, 'PLBM - Uji Bagan', 'ODOO-PLBM' FROM configuration.companies ORDER BY created_at LIMIT 1 RETURNING id::text`)
	title := h.scalar(`INSERT INTO hris.positions (title, level, is_active) VALUES ('Perfumer Staff', 'Staff', true) RETURNING id::text`)
	perf := h.employee(emp{name: "Perfumer Uji Kosong"})
	h.exec(`UPDATE hris.employees SET job_title_id = $2 WHERE id = $1`, perf, title)
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, start_date, is_primary) VALUES ($1, $2, '2026-01-01', true)`, perf, branch)
	after := h.do(&hr, "GET", "/api/hris/org-chart/vacancy", nil)
	expect(t, after, 200, "")
	if inRows(after, "RTL-023") {
		t.Fatalf("RTL-023 still open after the fixture")
	}
	if a, b := vacancyNum(t, after.data()["summary"].(map[string]any), "filled"), vacancyNum(t, sum, "filled"); a != b+1 {
		t.Fatalf("filled %d → %d", b, a)
	}

	// Review Focus 1: an ambiguous Shopkeeper (no store) makes the
	// Shopkeeper slots "maybe" with their name, never "empty".
	sk := h.scalar(`INSERT INTO hris.positions (title, level, is_active) VALUES ('Shopkeeper', 'Staff', true) RETURNING id::text`)
	amb := h.employee(emp{name: "Shopkeeper Uji Ambigu"})
	h.exec(`UPDATE hris.employees SET job_title_id = $2 WHERE id = $1`, amb, sk)
	maybe := h.do(&hr, "GET", "/api/hris/org-chart/vacancy?state=maybe", nil)
	expect(t, maybe, 200, "")
	mrows := maybe.data()["rows"].([]any)
	if len(mrows) < 2 {
		t.Fatalf("maybe rows %s", maybe.raw)
	}
	for _, r := range mrows {
		rm := r.(map[string]any)
		if rm["title"] != "Shopkeeper" || !strings.Contains(strings.Join(func() []string {
			var out []string
			for _, n := range rm["maybe_holders"].([]any) {
				out = append(out, n.(string))
			}
			return out
		}(), ","), "Shopkeeper Uji Ambigu") {
			t.Fatalf("maybe row %v", rm)
		}
	}
	if vacancyNum(t, maybe.data()["summary"].(map[string]any), "maybe") != len(mrows) {
		t.Fatalf("maybe summary %v", maybe.data()["summary"])
	}
}

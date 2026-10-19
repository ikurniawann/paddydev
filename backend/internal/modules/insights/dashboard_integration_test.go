package insights

import (
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"nuhabit/backend/internal/platform/testutil"
)

// Recruitment dashboard and analytics routes, ported from
// app/api/dashboard/funnel/route.test.ts, analytics/sources/route.test.ts
// and the lib expectations, against real rows filtered by a fresh brand.

type recruitmentData struct {
	brand, otherBrand, position string
}

func seedRecruitment(h *harness) recruitmentData {
	d := recruitmentData{
		brand:      h.scalar(`INSERT INTO item.brands (name) VALUES ('Go Kopi ' || md5(random()::text)) RETURNING id::text`),
		otherBrand: h.scalar(`INSERT INTO item.brands (name) VALUES ('Go Roti ' || md5(random()::text)) RETURNING id::text`),
		position:   h.scalar(`INSERT INTO hris.positions (title) VALUES ('Barista Go') RETURNING id::text`),
	}
	add := func(name, status, source string, created, updated time.Time, position *string) {
		h.exec(`INSERT INTO recruitment.candidates (full_name, email, phone, domicile, source, status, brand_id, position_id, created_at, updated_at)
			VALUES ($1, $1 || '@go.test', '0800', 'Jakarta', $2, $3, $4, $5, $6, $7)`,
			name, source, status, d.brand, position, created, updated)
	}
	day := 24 * time.Hour
	p := &d.position
	add("Ani Hired", "hired", "portal", clock.Add(-12*day), clock.Add(-2*day), p)
	add("Budi Applied", "applied", "portal", clock.Add(-3*day), clock.Add(-20*day), p)
	add("Citra Interview", "interview", "jobstreet", clock.Add(-5*day), clock.Add(-9*day), nil)
	add("Dodi Rejected", "rejected", "tiktok", clock.Add(-5*day), clock.Add(-30*day), p)
	add("Eka Pool", "talent_pool", "referral", clock.Add(-40*day), clock.Add(-40*day), nil)
	return d
}

func TestRecruitmentDashboardGuards(t *testing.T) {
	none := testutil.CreateStaff(t, testutil.StaffOptions{Menus: map[string][]string{}})
	recruiter := testutil.CreateStaff(t, testutil.StaffOptions{Menus: map[string][]string{"hris.recruitment": nil}})
	h := newTxHarness(t)
	for _, path := range []string{"/api/dashboard/attention", "/api/dashboard/funnel", "/api/dashboard/sources", "/api/dashboard/stats", "/api/dashboard/weekly", "/api/analytics/brands"} {
		c := h.get(path, none)
		if c.status != http.StatusForbidden || c.obj(t)["success"] != false || c.obj(t)["error"] != "Insufficient permissions" {
			t.Fatalf("%s: %d %s", path, c.status, c.raw)
		}
	}
	// Analytics is for HR insight readers only.
	if c := h.get("/api/analytics/sources", recruiter); c.status != http.StatusForbidden {
		t.Fatal(c.status, c.raw)
	}
	// A malformed brand id is the shim's 22P02 → 400.
	if c := h.get("/api/dashboard/funnel?brand_id=nope", recruiter); c.status != http.StatusBadRequest || c.obj(t)["error"] != "Format data tidak valid" {
		t.Fatal(c.status, c.raw)
	}
	// stats swallows count errors into 0, as the shim returns count: null.
	if c := h.get("/api/dashboard/stats?brand_id=nope", recruiter); c.status != http.StatusOK ||
		c.raw != `{"candidates_this_month":0,"active_pipeline":0,"talent_pool":0,"open_positions":0,"hired_this_month":0}` {
		t.Fatal(c.status, c.raw)
	}
}

func TestRecruitmentDashboard(t *testing.T) {
	s := testutil.CreateStaff(t, testutil.StaffOptions{Menus: map[string][]string{"hris.insights.analytics": nil}})
	h := newTxHarness(t)
	d := seedRecruitment(h)
	q := "?brand_id=" + d.brand

	c := h.get("/api/dashboard/funnel"+q+"&period=week", s)
	if c.raw != `[{"stage":"Applied","count":1},{"stage":"Screening","count":0},{"stage":"Psikotes","count":0},{"stage":"Interview","count":1},{"stage":"Offer","count":0},{"stage":"Hired","count":0},{"stage":"Talent Pool","count":0}]` {
		t.Fatal(c.raw)
	}
	if c := h.get("/api/dashboard/funnel"+q, s); c.raw != `[{"stage":"Applied","count":1},{"stage":"Screening","count":0},{"stage":"Psikotes","count":0},{"stage":"Interview","count":1},{"stage":"Offer","count":0},{"stage":"Hired","count":1},{"stage":"Talent Pool","count":0}]` {
		t.Fatal(c.raw)
	}

	if c := h.get("/api/dashboard/sources"+q, s); c.raw != `[{"name":"Website Portal","value":2},{"name":"JobStreet","value":1},{"name":"tiktok","value":1}]` {
		t.Fatal(c.raw)
	}

	// Stale for more than 7 days, oldest first; red after 14 days.
	c = h.get("/api/dashboard/attention"+q, s)
	rows := c.obj(t)["data"].([]any)
	if len(rows) != 3 {
		t.Fatal(c.raw)
	}
	first := rows[0].(map[string]any)
	if first["full_name"] != "Eka Pool" || first["days_in_current_status"] != 40.0 || first["urgency"] != "red" || first["position_title"] != nil {
		t.Fatal(c.raw)
	}
	second := rows[1].(map[string]any)
	if second["full_name"] != "Budi Applied" || second["position_title"] != "Barista Go" || second["urgency"] != "red" {
		t.Fatal(c.raw)
	}
	if third := rows[2].(map[string]any); third["full_name"] != "Citra Interview" || third["urgency"] != "amber" || third["days_in_current_status"] != 9.0 {
		t.Fatal(c.raw)
	}

	// October 2026 in WIB: three created this month; Ani was hired (updated) two days ago.
	if c := h.get("/api/dashboard/stats"+q, s); c.raw != `{"candidates_this_month":3,"active_pipeline":3,"talent_pool":1,"open_positions":0,"hired_this_month":1}` {
		t.Fatal(c.raw)
	}

	c = h.get("/api/dashboard/weekly"+q, s)
	weeks := c.json(t).([]any)
	last := weeks[7].(map[string]any)
	// Week 8 is Mon 28 Sep .. Sun 4 Oct WIB; Ani (12 days ago) is week 7.
	if len(weeks) != 8 || last["label"] != "Sep 28" || last["date"] != "2026-09-27" || last["count"] != 3.0 || weeks[6].(map[string]any)["count"] != 1.0 {
		t.Fatal(c.raw)
	}

	c = h.get("/api/analytics/sources"+q, s)
	if c.raw != `{"data":[{"source":"Website Portal","total":2,"hired":1,"rate":50},{"source":"Referral","total":1,"hired":0,"rate":0},{"source":"JobStreet","total":1,"hired":0,"rate":0}]}` {
		t.Fatal(c.raw)
	}

	c = h.get("/api/analytics/brands"+q, s)
	got := c.obj(t)
	bars := got["barData"].([]any)
	if len(bars) != 1 || !reflect.DeepEqual(bars[0].(map[string]any)["applicants"], 5.0) || bars[0].(map[string]any)["active"] != 3.0 ||
		bars[0].(map[string]any)["in_pool"] != 1.0 || len(got["pieData"].([]any)) != 1 {
		t.Fatal(c.raw)
	}
	// Brand filter hides the other brand even without applicants.
	if c := h.get("/api/analytics/brands?brand_id="+d.otherBrand, s); c.raw != `{"barData":[],"pieData":[]}` {
		t.Fatal(c.raw)
	}

	c = h.get("/api/analytics/overview"+q, s)
	ov := c.obj(t)
	if ov["time_to_hire_avg"] != 10.0 || ov["hiring_rate"] != 20.0 || ov["total_applicants"] != 5.0 || ov["total_active"] != 3.0 || ov["total_rejected"] != 1.0 {
		t.Fatal(c.raw)
	}
	htf := ov["hard_to_fill"].([]any)
	if len(htf) != 1 || htf[0].(map[string]any)["position_title"] != "Barista Go" || htf[0].(map[string]any)["count"] != 1.0 {
		t.Fatal(c.raw)
	}
	keys := []string{"time_to_hire_avg", "hiring_rate", "total_applicants", "total_hired", "total_rejected", "total_active", "conversion_rates", "hard_to_fill"}
	at := 0
	for _, k := range keys {
		i := strings.Index(c.raw, `"`+k+`"`)
		if i < at {
			t.Fatalf("key order %s: %s", k, c.raw)
		}
		at = i
	}
}

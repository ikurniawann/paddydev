package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// Ported from frontend/src/lib/desktop/period.test.ts.
func TestResolvePeriod(t *testing.T) {
	for _, c := range []struct {
		kind, now string
		want      Period
	}{
		{"today", "2026-07-15T05:00:00Z", Period{"today", "2026-07-15", "2026-07-15", 1, 1}},
		{"today", "2026-07-15T23:30:00Z", Period{"today", "2026-07-16", "2026-07-16", 1, 1}},
		{"mtd", "2026-07-15T05:00:00Z", Period{"mtd", "2026-07-01", "2026-07-15", 15, 31}},
		{"qtd", "2026-08-10T05:00:00Z", Period{"qtd", "2026-07-01", "2026-08-10", 41, 92}},
		{"qtd", "2026-04-01T05:00:00Z", Period{"qtd", "2026-04-01", "2026-04-01", 1, 91}},
		{"ytd", "2026-03-01T05:00:00Z", Period{"ytd", "2026-01-01", "2026-03-01", 60, 365}},
		{"ytd", "2028-03-01T05:00:00Z", Period{"ytd", "2028-01-01", "2028-03-01", 61, 366}},
	} {
		if got := ResolvePeriod(c.kind, at(c.now)); got != c.want {
			t.Errorf("%s %s = %+v, want %+v", c.kind, c.now, got, c.want)
		}
	}
}

func TestResolveComparison(t *testing.T) {
	for _, c := range []struct {
		kind, now string
		want      Comparison
	}{
		{"today", "2026-07-15T05:00:00Z", Comparison{"2026-07-08", "2026-07-08", 1, true}},
		{"mtd", "2026-07-15T05:00:00Z", Comparison{"2026-06-01", "2026-06-15", 15, true}},
		{"mtd", "2026-03-31T05:00:00Z", Comparison{"2026-02-01", "2026-02-28", 28, false}},
		{"mtd", "2028-02-29T05:00:00Z", Comparison{"2028-01-01", "2028-01-29", 29, true}},
		{"qtd", "2026-07-10T05:00:00Z", Comparison{"2026-04-01", "2026-04-10", 10, true}},
		{"qtd", "2026-01-05T05:00:00Z", Comparison{"2025-10-01", "2025-10-05", 5, true}},
		{"ytd", "2026-07-15T05:00:00Z", Comparison{"2025-01-01", "2025-07-15", 196, true}},
		{"ytd", "2028-02-29T05:00:00Z", Comparison{"2027-01-01", "2027-02-28", 59, false}},
	} {
		if got := ResolveComparison(ResolvePeriod(c.kind, at(c.now))); got != c.want {
			t.Errorf("%s %s = %+v, want %+v", c.kind, c.now, got, c.want)
		}
	}
}

func TestProjectRunRate(t *testing.T) {
	mtd := ResolvePeriod("mtd", at("2026-07-15T05:00:00Z"))
	if got := ProjectRunRate(15_000_000, mtd); got != 31_000_000 {
		t.Fatalf("mtd = %v", got)
	}
	if got := ProjectRunRate(2_500_000, ResolvePeriod("today", at("2026-07-15T05:00:00Z"))); got != 2_500_000 {
		t.Fatalf("today = %v", got)
	}
	mtd.HariBerjalan = 0
	if got := ProjectRunRate(1000, mtd); got != 0 {
		t.Fatalf("zero days = %v", got)
	}
	if ParsePeriod("ytd") != "ytd" || ParsePeriod("x") != "today" || ParsePeriod("") != "today" {
		t.Fatal("ParsePeriod")
	}
}

// Ported from revenue.test.ts.
func TestBreakdownRevenue(t *testing.T) {
	src := func(fnb, b2b float64) []RevenueSource {
		return []RevenueSource{{Kunci: "fnb", Label: "F&B", Nilai: fnb}, {Kunci: "b2b", Label: "B2B", Nilai: b2b}}
	}
	total, s := BreakdownRevenue(src(7_500_000, 2_500_000))
	if total != 10_000_000 || s[0].Porsi != 75 || s[1].Porsi != 25 {
		t.Fatalf("%v %+v", total, s)
	}
	_, s = BreakdownRevenue([]RevenueSource{{Kunci: "fnb", Nilai: 100}, {Kunci: "b2b", Nilai: 100}, {Kunci: "lain", Nilai: 100}})
	if s[0].Porsi+s[1].Porsi+s[2].Porsi != 100 {
		t.Fatalf("shares %+v", s)
	}
	if total, s = BreakdownRevenue(src(0, 0)); total != 0 || len(s) != 0 {
		t.Fatalf("zero: %v %+v", total, s)
	}
	if total, s = BreakdownRevenue(src(1_000_000, 0)); total != 1_000_000 || len(s) != 1 || s[0].Kunci != "fnb" {
		t.Fatalf("one source: %v %+v", total, s)
	}
	if total, s = BreakdownRevenue(src(1_000_000, -1_500_000)); total != -500_000 || s[0].Porsi != 0 {
		t.Fatalf("negative: %v %+v", total, s)
	}
	raw, _ := json.Marshal(s[0])
	if string(raw) != `{"kunci":"fnb","label":"F\u0026B","nilai":1000000,"porsi":0}` { // key order; httpx.JSON does not escape &
		t.Fatalf("json %s", raw)
	}
}

func TestPromoEfficiency(t *testing.T) {
	if v := PromoEfficiency(1_000_000, 8_000_000); v == nil || *v != 8 {
		t.Fatal("8x")
	}
	if PromoEfficiency(0, 5_000_000) != nil {
		t.Fatal("no discount must be nil")
	}
	if v := PromoEfficiency(500_000, 0); v == nil || *v != 0 {
		t.Fatal("discount without sales is 0")
	}
	if v := PromoEfficiency(300_000, 1_000_000); *v != 3.33 {
		t.Fatalf("rounding %v", *v)
	}
}

// Ported from inbox.test.ts.
func TestInbox(t *testing.T) {
	if got := AllowedInboxSections("super_admin", nil); strings.Join(got, ",") != "cuti,po,stok" {
		t.Fatalf("full role %v", got)
	}
	if got := AllowedInboxSections("hrd", []string{"hris.workforce.leaves"}); strings.Join(got, ",") != "cuti" {
		t.Fatalf("hr %v", got)
	}
	if got := AllowedInboxSections("staff", []string{"items.raw-material.approval.po", "items.raw-material.inventory.stock"}); strings.Join(got, ",") != "po,stok" {
		t.Fatalf("warehouse %v", got)
	}
	if got := AllowedInboxSections("staff", nil); len(got) != 0 {
		t.Fatalf("no menus %v", got)
	}
	if got := DescribeLeaveRange("2026-09-18", "2026-09-18", 1); got != "1 hari · 18 Sep" {
		t.Fatal(got)
	}
	if got := DescribeLeaveRange("2026-08-18", "2026-09-20", 3.5); got != "3.5 hari · 18 Agu–20 Sep" {
		t.Fatal(got)
	}
}

// EPIC-058: the inbox subtitle shows the catalog label, not the raw enum code.
func TestLeaveTypeLabel(t *testing.T) {
	for code, want := range map[string]string{
		"leader": "Izin Leader", "personal": "Izin Pribadi", "annual": "Cuti Tahunan",
		"menstrual": "Cuti Haid", "Cuti": "Cuti", "": "",
	} {
		if got := LeaveTypeLabel(code); got != want {
			t.Errorf("LeaveTypeLabel(%q) = %q, want %q", code, got, want)
		}
	}
}

// Ported from search.test.ts.
func TestSearch(t *testing.T) {
	if NormalizeSearchQuery("  kopi   susu  ") != "kopi susu" || len(NormalizeSearchQuery(strings.Repeat("a", 200))) != 80 {
		t.Fatal("normalize")
	}
	if IsSearchable("k") || !IsSearchable("ko") || IsSearchable("  ") {
		t.Fatal("searchable")
	}
	if LikePattern("100%") != `%100\%%` || LikePattern("a_b") != `%a\_b%` || LikePattern(`a\b`) != `%a\\b%` {
		t.Fatal("like pattern")
	}
	keys := func(granted []string) string {
		var out []string
		for _, s := range AllowedSources(granted) {
			out = append(out, s.Key)
		}
		return strings.Join(out, ",")
	}
	if got := keys([]string{"crm.members.list", "pos.reports.dashboard"}); got != "order,member" {
		t.Fatalf("sources %s", got)
	}
	if got := keys([]string{"crmx.members"}); got != "" {
		t.Fatalf("prefix must match whole segments: %s", got)
	}
	if got := keys(nil); got != "order,member,product,material,employee,document" {
		t.Fatalf("no IAM data %s", got)
	}

	hit := func(title string) SearchHit { return SearchHit{Source: "order", ID: title, Title: title, Href: "/x"} }
	ranked := RankSearchHits([]SearchHit{hit("XX POS-5"), hit("POS-5 lanjutan"), hit("POS-5")}, "pos-5")
	if ranked[0].Title != "POS-5" || ranked[1].Title != "POS-5 lanjutan" || ranked[2].Title != "XX POS-5" {
		t.Fatalf("rank %+v", ranked)
	}
	groups := GroupHitsBySource([]SearchHit{{Source: "member", Title: "Budi"}, {Source: "order", Title: "POS-1"}})
	if len(groups) != 2 || groups[0].Source != "order" || groups[1].Label != "Member" {
		t.Fatalf("groups %+v", groups)
	}
}

func TestStatus(t *testing.T) {
	if RollupStatus(nil) != "unknown" || RollupStatus([]StatusItem{{Level: "ok"}, {Level: "unknown"}}) != "unknown" ||
		RollupStatus([]StatusItem{{Level: "warn"}, {Level: "down"}}) != "down" || RollupStatus([]StatusItem{{Level: "ok"}}) != "ok" {
		t.Fatal("rollup")
	}
	stuck, fresh := 10, 2
	for _, c := range []struct {
		pending int
		minutes *int
		want    string
	}{{0, nil, "ok"}, {3, &stuck, "down"}, {12, &fresh, "warn"}, {3, &fresh, "ok"}} {
		if got := PrintQueueLevel(c.pending, c.minutes); got != c.want {
			t.Errorf("%d = %s", c.pending, got)
		}
	}
}

// Ported from preferences.test.ts.
func TestNormalizePreferences(t *testing.T) {
	defaults := `{"wallpaper":null,"widgetVisibility":{},"widgetOrder":[],"soundEnabled":false,"period":null}`
	for _, raw := range []string{"null", `"teks"`, "[1,2]", "not json", ""} {
		got, _ := json.Marshal(NormalizePreferences([]byte(raw)))
		if string(got) != defaults {
			t.Errorf("%q = %s", raw, got)
		}
	}
	got, _ := json.Marshal(NormalizePreferences([]byte(`{"wallpaper":"wp-1",
		"widgetVisibility":{"stok":false,"omzet":true,"widget_hantu":true,"tim":"ya"},
		"widgetOrder":["stok","stok","widget_hantu","omzet",3],"soundEnabled":true,"period":"bulan-berjalan","extra":1}`)))
	want := `{"wallpaper":"wp-1","widgetVisibility":{"stok":false,"omzet":true},"widgetOrder":["stok","omzet"],"soundEnabled":true,"period":"bulan-berjalan"}`
	if string(got) != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
	if p := NormalizePreferences([]byte(`{"wallpaper":""}`)); p.Wallpaper != nil {
		t.Fatal("empty wallpaper is unset")
	}
}

// Ported from wallpapers.test.ts.
func TestParseWallpapers(t *testing.T) {
	for _, raw := range []string{"", "{bukan json", `{"id":"x"}`} {
		if got := ParseWallpapers(&raw); len(got) != 0 {
			t.Errorf("%q = %+v", raw, got)
		}
	}
	if got := ParseWallpapers(nil); got == nil || len(got) != 0 {
		t.Fatal("nil setting")
	}
	raw := `[{"id":"a","name":"WP a","src":"/api/files/desktop-wallpapers/a.jpg","created_at":"2026-09-17T00:00:00.000Z","created_by":null},{"id":"b"},null,"str",{"id":"c","name":"C","src":"/c"}]`
	got, _ := json.Marshal(ParseWallpapers(&raw))
	want := `[{"id":"a","name":"WP a","src":"/api/files/desktop-wallpapers/a.jpg","created_at":"2026-09-17T00:00:00.000Z","created_by":null},` +
		`{"id":"c","name":"C","src":"/c","created_at":"1970-01-01T00:00:00.000Z","created_by":null}]`
	if string(got) != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
}

// Expected values come from Node's toLocaleString("id-ID") and String().
func TestNumberFormats(t *testing.T) {
	for _, c := range []struct {
		in   float64
		want string
	}{{1234567.891, "1.234.567,891"}, {0.0005, "0,001"}, {-1500, "-1.500"}, {12.3456, "12,346"}, {0, "0"}, {999, "999"}} {
		if got := FormatID(c.in); got != c.want {
			t.Errorf("FormatID(%v) = %s, want %s", c.in, got, c.want)
		}
	}
	if JSNumber(2.5) != "2.5" || JSNumber(3) != "3" || JSNumber(-0.25) != "-0.25" {
		t.Fatal("JSNumber")
	}
}

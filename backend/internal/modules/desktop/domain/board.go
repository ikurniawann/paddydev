package domain

import (
	"encoding/json"
	"math"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/jsmath"
)

// ── revenue (revenue.ts) ─────────────────────────────────────────────────

// RevenueSource is one revenue stream with its share in whole percent.
type RevenueSource struct {
	Kunci string  `json:"kunci"`
	Label string  `json:"label"`
	Nilai float64 `json:"nilai"`
	Porsi float64 `json:"porsi"`
}

// BreakdownRevenue is breakdownRevenue: the total, and the positive sources
// with rounded shares whose rounding gap goes to the largest so they add up
// to 100. A total at or below zero gives every share 0.
func BreakdownRevenue(sources []RevenueSource) (float64, []RevenueSource) {
	total := 0.0
	for _, s := range sources {
		total += s.Nilai
	}
	shown := []RevenueSource{}
	for _, s := range sources {
		if s.Nilai > 0 {
			s.Porsi = 0
			shown = append(shown, s)
		}
	}
	if total <= 0 {
		return total, shown
	}
	sum := 0.0
	for i := range shown {
		shown[i].Porsi = jsmath.Round(float64(shown[i].Nilai/total) * 100)
		sum += shown[i].Porsi
	}
	if gap := 100 - sum; gap != 0 && len(shown) > 0 {
		largest := 0
		for i := range shown {
			if shown[i].Nilai > shown[largest].Nilai {
				largest = i
			}
		}
		shown[largest].Porsi += gap
	}
	return total, shown
}

// PromoEfficiency is promoEfficiency: rupiah of sales per rupiah of
// discount, two decimals; nil before any discount was given.
func PromoEfficiency(diskon, omzet float64) *float64 {
	if diskon <= 0 {
		return nil
	}
	v := jsmath.RoundTo(omzet/diskon, 2)
	return &v
}

// ── inbox (inbox.ts) ─────────────────────────────────────────────────────

// InboxKeys are the inbox sections in display order.
var InboxKeys = []string{"cuti", "po", "stok"}

// InboxSectionIAM is INBOX_SECTION_IAM.
var InboxSectionIAM = map[string][]string{
	"cuti": {"hris.workforce", "hris.kepegawaian", "hris"},
	"po":   {"items.product.approval", "items.raw-material.approval", "items.product.purchasing", "items.raw-material.purchasing"},
	"stok": {"items.raw-material", "items", "pos.catalog"},
}

// InboxSectionLabel is INBOX_SECTION_LABEL.
var InboxSectionLabel = map[string]string{
	"cuti": "Pengajuan cuti",
	"po":   "PO menunggu approval",
	"stok": "Stok di bawah minimum",
}

// InboxItemLimit caps the items per section.
const InboxItemLimit = 5

// AllowedInboxSections is allowedInboxSections: full roles see every
// section; others need a menu under the section's prefixes.
func AllowedInboxSections(role string, granted []string) []string {
	if role == "super_admin" || role == "admin" || role == "direksi" {
		return slices.Clone(InboxKeys)
	}
	out := []string{}
	if len(granted) == 0 {
		return out
	}
	for _, key := range InboxKeys {
		if iam.HasAnyMenuPrefix(granted, InboxSectionIAM[key]) {
			out = append(out, key)
		}
	}
	return out
}

var idMonths = [...]string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

// leaveTypeLabels mirrors the label column of the hris leave-type catalog
// (modules/hris/domain/leave_types.go, frontend/src/lib/hris/leave-types.ts).
// Modules never import each other, so desktop keeps its own copy: change them together.
var leaveTypeLabels = map[string]string{
	"annual": "Cuti Tahunan", "sick": "Sakit", "personal": "Izin Pribadi", "leader": "Izin Leader",
	"maternity": "Cuti Melahirkan", "paternity": "Cuti Ayah", "pilgrimage": "Cuti Ibadah Haji/Umrah",
	"marriage": "Cuti Menikah", "bereavement": "Cuti Duka",
	"emergency": "Izin Darurat", "unpaid": "Izin Tanpa Gaji", "menstrual": "Cuti Haid",
}

// LeaveTypeLabel is leaveTypeLabel: the catalog label, or the raw code when unknown.
func LeaveTypeLabel(code string) string {
	if label, ok := leaveTypeLabels[code]; ok {
		return label
	}
	return code
}

// DescribeLeaveRange is describeLeaveRange: "3 hari · 18 Sep–20 Sep"
// (toLocaleDateString("id-ID", {day: "numeric", month: "short"})).
func DescribeLeaveRange(start, end string, days float64) string {
	format := func(d string) string {
		t, err := time.Parse("2006-01-02", d)
		if err != nil {
			return "Invalid Date"
		}
		return strconv.Itoa(t.Day()) + " " + idMonths[t.Month()-1]
	}
	rng := format(start)
	if start != end {
		rng += "–" + format(end)
	}
	return JSNumber(days) + " hari · " + rng
}

// ── search (search.ts) ───────────────────────────────────────────────────

// SearchSource is SearchSourceDef.
type SearchSource struct {
	Key         string
	Label       string
	IAMPrefixes []string
}

// SearchSources is SEARCH_SOURCES, in display order.
var SearchSources = []SearchSource{
	{"order", "Transaksi", []string{"pos.reports", "pos.operations"}},
	{"member", "Member", []string{"crm.members", "crm"}},
	{"product", "Produk", []string{"pos.catalog", "items.product"}},
	{"material", "Bahan Baku", []string{"items.raw-material"}},
	{"employee", "Karyawan", []string{"hris.kepegawaian", "hris"}},
	{"document", "Dataroom", []string{"dataroom"}},
}

// SearchLimitPerSource caps the rows per source.
const SearchLimitPerSource = 5

const minSearchLength = 2

var spaces = regexp.MustCompile(`\s+`)

// NormalizeSearchQuery is normalizeSearchQuery: trimmed, inner whitespace
// collapsed, at most 80 UTF-16 units.
func NormalizeSearchQuery(raw string) string {
	q := spaces.ReplaceAllString(jsTrim(raw), " ")
	if units := utf16.Encode([]rune(q)); len(units) > 80 {
		q = string(utf16.Decode(units[:80]))
	}
	return q
}

// IsSearchable is isSearchable.
func IsSearchable(q string) bool { return utf16Len(NormalizeSearchQuery(q)) >= minSearchLength }

// LikePattern is likePattern: %, _ and \ from the user are escaped.
func LikePattern(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(NormalizeSearchQuery(q)) + "%"
}

// AllowedSources is allowedSources: no IAM data at all (a fresh install)
// shows every source.
func AllowedSources(granted []string) []SearchSource {
	if len(granted) == 0 {
		return slices.Clone(SearchSources)
	}
	out := []SearchSource{}
	for _, s := range SearchSources {
		if iam.HasAnyMenuPrefix(granted, s.IAMPrefixes) {
			out = append(out, s)
		}
	}
	return out
}

// SearchHit is one result.
type SearchHit struct {
	Source   string  `json:"source"`
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Subtitle *string `json:"subtitle,omitempty"`
	Href     string  `json:"href"`
}

// SearchGroup is one source's hits.
type SearchGroup struct {
	Source string      `json:"source"`
	Label  string      `json:"label"`
	Items  []SearchHit `json:"items"`
}

// RankSearchHits is rankSearchHits: exact title, then prefix, then
// contains, then the rest; ties by title.
func RankSearchHits(hits []SearchHit, query string) []SearchHit {
	q := strings.ToLower(NormalizeSearchQuery(query))
	score := func(h SearchHit) int {
		title := strings.ToLower(h.Title)
		switch {
		case title == q:
			return 0
		case strings.HasPrefix(title, q):
			return 1
		case strings.Contains(title, q):
			return 2
		}
		return 3
	}
	out := slices.Clone(hits)
	sort.SliceStable(out, func(i, j int) bool {
		si, sj := score(out[i]), score(out[j])
		if si != sj {
			return si < sj
		}
		return localeCompare(out[i].Title, out[j].Title) < 0
	})
	return out
}

// GroupHitsBySource is groupHitsBySource: source order, empty groups out.
func GroupHitsBySource(hits []SearchHit) []SearchGroup {
	out := []SearchGroup{}
	for _, def := range SearchSources {
		var items []SearchHit
		for _, h := range hits {
			if h.Source == def.Key {
				items = append(items, h)
			}
		}
		if len(items) > 0 {
			out = append(out, SearchGroup{Source: def.Key, Label: def.Label, Items: items})
		}
	}
	return out
}

// localeCompare approximates String.prototype.localeCompare (ICU root
// collation) for titles: case-insensitive first, lower case before upper
// on a tie.
func localeCompare(a, b string) int {
	if c := strings.Compare(strings.ToLower(a), strings.ToLower(b)); c != 0 {
		return c
	}
	return -strings.Compare(a, b)
}

// ── status (status.ts) ───────────────────────────────────────────────────

// StatusItem is one menubar status light.
type StatusItem struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Level  string `json:"level"`
	Detail string `json:"detail,omitempty"`
}

var statusOrder = map[string]int{"down": 0, "warn": 1, "unknown": 2, "ok": 3}

// RollupStatus is rollupStatus: the worst level wins.
func RollupStatus(items []StatusItem) string {
	if len(items) == 0 {
		return "unknown"
	}
	worst := "ok"
	for _, it := range items {
		if statusOrder[it.Level] < statusOrder[worst] {
			worst = it.Level
		}
	}
	return worst
}

// PrintQueueLevel is printQueueLevel: a queue stuck 10 minutes is down,
// ten or more pending jobs is a warning.
func PrintQueueLevel(pending int, stuckMinutes *int) string {
	switch {
	case pending == 0:
		return "ok"
	case stuckMinutes != nil && *stuckMinutes >= 10:
		return "down"
	case pending >= 10:
		return "warn"
	}
	return "ok"
}

// ── preferences (preferences.ts, widgets.ts) ─────────────────────────────

// widgetKeys are MONITOR_WIDGETS keys plus "calendar".
var widgetKeys = []string{"omzet", "promo", "tamu", "pulsa", "tim", "keputusan", "stok", "member", "calendar"}

// Preferences is DesktopPreferences.
type Preferences struct {
	Wallpaper        *string        `json:"wallpaper"`
	WidgetVisibility orderedBoolMap `json:"widgetVisibility"`
	WidgetOrder      []string       `json:"widgetOrder"`
	SoundEnabled     bool           `json:"soundEnabled"`
	Period           *string        `json:"period"`
}

// orderedBoolMap keeps the JSON key order of the input, as a JS object does.
type orderedBoolMap struct {
	keys   []string
	values map[string]bool
}

// MarshalJSON writes the keys in insertion order.
func (m orderedBoolMap) MarshalJSON() ([]byte, error) {
	var b strings.Builder
	b.WriteByte('{')
	for i, k := range m.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		key, _ := json.Marshal(k)
		b.Write(key)
		b.WriteString(":" + strconv.FormatBool(m.values[k]))
	}
	b.WriteByte('}')
	return []byte(b.String()), nil
}

// NormalizePreferences is normalizeDesktopPreferences on raw JSON: unknown
// widget keys and duplicate order entries are dropped; anything that is not
// an object gives the defaults.
func NormalizePreferences(raw []byte) Preferences {
	out := Preferences{WidgetVisibility: orderedBoolMap{values: map[string]bool{}}, WidgetOrder: []string{}}
	fields, ok := objectFields(raw)
	if !ok {
		return out
	}
	if s, ok := jsonString(fields.get("wallpaper")); ok && s != "" {
		out.Wallpaper = &s
	}
	if vis, ok := objectFields(fields.get("widgetVisibility")); ok {
		for _, k := range vis.keys {
			v := strings.TrimSpace(string(vis.get(k)))
			if (v == "true" || v == "false") && slices.Contains(widgetKeys, k) {
				out.WidgetVisibility.keys = append(out.WidgetVisibility.keys, k)
				out.WidgetVisibility.values[k] = v == "true"
			}
		}
	}
	var order []json.RawMessage
	if json.Unmarshal(fields.get("widgetOrder"), &order) == nil {
		for _, item := range order {
			if k, ok := jsonString(item); ok && slices.Contains(widgetKeys, k) && !slices.Contains(out.WidgetOrder, k) {
				out.WidgetOrder = append(out.WidgetOrder, k)
			}
		}
	}
	out.SoundEnabled = strings.TrimSpace(string(fields.get("soundEnabled"))) == "true"
	if s, ok := jsonString(fields.get("period")); ok && s != "" {
		out.Period = &s
	}
	return out
}

// rawObject is a JSON object with its keys in source order (the last
// duplicate wins, as JSON.parse does).
type rawObject struct {
	keys   []string
	values map[string]json.RawMessage
}

func (o rawObject) get(k string) json.RawMessage { return o.values[k] }

func objectFields(raw []byte) (rawObject, bool) {
	o := rawObject{values: map[string]json.RawMessage{}}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return o, false
	}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return o, false
		}
		k, _ := kt.(string)
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return o, false
		}
		if _, seen := o.values[k]; !seen {
			o.keys = append(o.keys, k)
		}
		o.values[k] = v
	}
	return o, true
}

func jsonString(raw json.RawMessage) (string, bool) {
	var s string
	if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &s) != nil {
		return "", false
	}
	return s, true
}

// ── wallpapers (wallpapers.ts) ───────────────────────────────────────────

// Wallpaper is DesktopWallpaper.
type Wallpaper struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Src       string  `json:"src"`
	CreatedAt string  `json:"created_at"`
	CreatedBy *string `json:"created_by"`
}

// ParseWallpapers is parseDesktopWallpapers: entries without a string id,
// src and name are dropped; a missing created_at becomes the epoch.
func ParseWallpapers(raw *string) []Wallpaper {
	out := []Wallpaper{}
	if raw == nil || *raw == "" {
		return out
	}
	var items []json.RawMessage
	if json.Unmarshal([]byte(*raw), &items) != nil {
		return out
	}
	for _, item := range items {
		fields, ok := objectFields(item)
		if !ok {
			continue
		}
		id, okID := jsonString(fields.get("id"))
		src, okSrc := jsonString(fields.get("src"))
		name, okName := jsonString(fields.get("name"))
		if !okID || !okSrc || !okName {
			continue
		}
		w := Wallpaper{ID: id, Name: name, Src: src, CreatedAt: "1970-01-01T00:00:00.000Z"}
		if at, ok := jsonString(fields.get("created_at")); ok {
			w.CreatedAt = at
		}
		// `item.created_by ?? null`: any non-null value passes through; the
		// TS writes a user id string.
		if by, ok := jsonString(fields.get("created_by")); ok {
			w.CreatedBy = &by
		}
		out = append(out, w)
	}
	return out
}

// ── shared ───────────────────────────────────────────────────────────────

// JSNumber is String(x) for the integers and decimals the board prints.
func JSNumber(x float64) string {
	if x == math.Trunc(x) && math.Abs(x) < 1e21 {
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return strconv.FormatFloat(x, 'g', -1, 64)
}

// FormatID is Number.prototype.toLocaleString("id-ID"): "." groups
// thousands, "," marks up to three decimals.
func FormatID(x float64) string {
	neg := x < 0
	s := strconv.FormatFloat(math.Abs(x), 'f', 3, 64)
	intPart, frac, _ := strings.Cut(s, ".")
	frac = strings.TrimRight(frac, "0")
	var b strings.Builder
	if neg && (intPart != "0" || frac != "") {
		b.WriteByte('-')
	}
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	if frac != "" {
		b.WriteString("," + frac)
	}
	return b.String()
}

func utf16Len(s string) int { return len(utf16.Encode([]rune(s))) }

// jsTrim is String.prototype.trim.
func jsTrim(s string) string {
	return strings.TrimFunc(s, func(r rune) bool {
		switch r {
		case ' ', '\t', '\n', '\v', '\f', '\r', 0xA0, 0x1680, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0xFEFF:
			return true
		}
		return r >= 0x2000 && r <= 0x200A
	})
}

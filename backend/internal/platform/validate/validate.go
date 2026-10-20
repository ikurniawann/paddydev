// Package validate checks JSON request bodies the way the TS routes' zod v4
// schemas do: fields in schema order, zod-like issues (code, path, message),
// UTF-16 string lengths, and JS trim. A handler builds a Form from the body,
// reads each field with the rule its schema declares, then returns Err or
// ErrAtPath when any issue was recorded.
package validate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"

	"nuhabit/backend/internal/platform/httpx"
)

// Issue is one zod-like validation issue.
type Issue struct {
	Code    string `json:"code"`
	Path    []any  `json:"path"`
	Message string `json:"message"`
}

// uuidPattern is zod v4's z.string().uuid().
var uuidPattern = regexp.MustCompile(`^([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-8][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}|00000000-0000-0000-0000-000000000000|ffffffff-ffff-ffff-ffff-ffffffffffff)$`)

// IsUUID reports whether s is a zod v4 UUID.
func IsUUID(s string) bool { return uuidPattern.MatchString(s) }

// CanonicalUUID parses an id the way a uuid column accepts it (any case,
// with or without the four hyphens, optionally in braces) and returns its
// canonical lower-case hyphenated form. Guards compare ids in that form, so
// every request-sourced id is canonicalised once at input (EPIC-061).
func CanonicalUUID(s string) (string, bool) {
	if strings.HasPrefix(s, "{") || strings.HasSuffix(s, "}") {
		if len(s) < 2 || s[0] != '{' || s[len(s)-1] != '}' {
			return "", false
		}
		s = s[1 : len(s)-1]
	}
	if len(s) == 36 {
		if s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
			return "", false
		}
		s = s[:8] + s[9:13] + s[14:18] + s[19:23] + s[24:]
	}
	if len(s) != 32 {
		return "", false
	}
	b := []byte(strings.ToLower(s))
	for _, c := range b {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", false
		}
	}
	h := string(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], true
}

// datetimePattern is zod v4's z.string().datetime({ offset: true }).
var datetimePattern = regexp.MustCompile(`^(?:(?:\d\d[2468][048]|\d\d[13579][26]|\d\d0[48]|[02468][048]00|[13579][26]00)-02-29|\d{4}-(?:(?:0[13578]|1[02])-(?:0[1-9]|[12]\d|3[01])|(?:0[469]|11)-(?:0[1-9]|[12]\d|30)|(?:02)-(?:0[1-9]|1\d|2[0-8])))T(?:(?:[01]\d|2[0-3]):[0-5]\d(?::[0-5]\d(?:\.\d+)?)?(?:Z|([+-](?:[01]\d|2[0-3]):[0-5]\d)))$`)

// ParseJSDate parses what datetimePattern accepts, as new Date(...) does.
func ParseJSDate(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04Z07:00"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// ReadBody mirrors `await request.json().catch(() => undefined)`.
func ReadBody(r *http.Request) (any, bool) {
	if r.Body == nil {
		return nil, false
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return nil, false
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	return v, true
}

// Form validates one JSON object; nested objects share the issue list.
type Form struct {
	obj    map[string]any
	path   []any
	issues *[]Issue
}

// New starts a Form over a decoded body; present is false when the body was
// missing or not JSON (ReadBody's second result).
func New(body any, present bool) *Form {
	f := &Form{path: []any{}, issues: &[]Issue{}}
	f.obj = f.object(nil, body, present)
	return f
}

func (f *Form) at(key any) []any {
	if key == nil {
		return slices.Clone(f.path)
	}
	return append(slices.Clone(f.path), key)
}

// Fail records a custom issue (a zod refine) at key.
func (f *Form) Fail(key any, code, msg string) {
	*f.issues = append(*f.issues, Issue{Code: code, Path: f.at(key), Message: msg})
}

func (f *Form) object(key any, v any, present bool) map[string]any {
	obj, ok := v.(map[string]any)
	if !ok {
		f.Fail(key, "invalid_type", "Invalid input: expected object, received "+typeName(v, present))
	}
	return obj
}

// Child validates a nested required object.
func (f *Form) Child(key string) *Form {
	v, sent := f.obj[key]
	c := &Form{path: f.at(key), issues: f.issues}
	if f.obj != nil {
		c.obj = f.object(key, v, sent)
	}
	return c
}

// Valid reports whether no issue was recorded.
func (f *Form) Valid() bool { return len(*f.issues) == 0 }

// Issues returns the recorded issues in schema order.
func (f *Form) Issues() []Issue { return *f.issues }

// Fields returns the decoded object (nil when the body was not an object).
func (f *Form) Fields() map[string]any { return f.obj }

// Err is a 400 with message and the issues as details (validateBody's
// "Validation failed" shape), or nil when every field passed.
func (f *Form) Err(message string) error {
	if f.Valid() {
		return nil
	}
	return httpx.BadRequest(message, *f.issues)
}

// ErrAtPath is Err with the first failing path appended
// ("Data tidak valid: default_spec.distanceM").
func (f *Form) ErrAtPath(message string) error {
	if f.Valid() {
		return nil
	}
	first := (*f.issues)[0]
	if len(first.Path) == 0 {
		return httpx.BadRequest(message, *f.issues)
	}
	parts := make([]string, len(first.Path))
	for i, p := range first.Path {
		parts[i] = fmt.Sprint(p)
	}
	return httpx.BadRequest(message+": "+strings.Join(parts, "."), *f.issues)
}

func typeName(v any, present bool) string {
	if !present {
		return "undefined"
	}
	switch v.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case json.Number:
		return "number"
	case bool:
		return "boolean"
	case []any:
		return "array"
	}
	return "object"
}

// Rule says how undefined and null are treated.
type Rule struct {
	Optional   bool // undefined allowed
	Nullable   bool // null allowed
	HasDefault bool // undefined takes a default
}

// Take returns the raw value, or done=true when the caller should use the
// zero value (absent, null, or already failed).
func (f *Form) Take(key, expected string, r Rule) (v any, sent, done bool) {
	if f.obj == nil {
		return nil, false, true
	}
	v, sent = f.obj[key]
	switch {
	case !sent:
		if !r.Optional && !r.HasDefault {
			f.Fail(key, "invalid_type", "Invalid input: expected "+expected+", received undefined")
		}
		return nil, false, true
	case v == nil:
		if !r.Nullable {
			f.Fail(key, "invalid_type", "Invalid input: expected "+expected+", received null")
		}
		return nil, true, true
	}
	return v, true, false
}

/* ── strings ─────────────────────────────────────────────────────────── */

// StrOpts are z.string() refinements; Min and Max count UTF-16 units.
type StrOpts struct {
	Trim     bool
	Min, Max int
	Check    func(string) (code, msg string, ok bool)
}

// UTF16Len is JS String.length.
func UTF16Len(s string) int { return len(utf16.Encode([]rune(s))) }

// JSTrim is String.prototype.trim: Unicode whitespace plus the BOM.
func JSTrim(s string) string {
	return strings.TrimFunc(s, func(r rune) bool { return unicode.IsSpace(r) || r == '\uFEFF' })
}

// CheckString validates v as a string at key (a map key or array index).
func (f *Form) CheckString(key any, v any, o StrOpts) (string, bool) {
	s, ok := v.(string)
	if !ok {
		f.Fail(key, "invalid_type", "Invalid input: expected string, received "+typeName(v, true))
		return "", false
	}
	if o.Trim {
		s = JSTrim(s)
	}
	n := UTF16Len(s)
	ok = true
	switch {
	case o.Min > 0 && n < o.Min:
		f.Fail(key, "too_small", fmt.Sprintf("Too small: expected string to have >=%d characters", o.Min))
		ok = false
	case o.Max > 0 && n > o.Max:
		f.Fail(key, "too_big", fmt.Sprintf("Too big: expected string to have <=%d characters", o.Max))
		ok = false
	}
	if o.Check != nil {
		if code, msg, good := o.Check(s); !good {
			f.Fail(key, code, msg)
			ok = false
		}
	}
	return s, ok
}

// Str returns nil when the key is absent or null (or invalid).
func (f *Form) Str(key string, r Rule, o StrOpts) *string {
	v, _, done := f.Take(key, "string", r)
	if done {
		return nil
	}
	s, _ := f.CheckString(key, v, o)
	return &s
}

// StrDefault is z.string()….default(def).
func (f *Form) StrDefault(key, def string, o StrOpts) string {
	if s := f.Str(key, Rule{HasDefault: true}, o); s != nil {
		return *s
	}
	return def
}

// UUIDCheck is z.string().uuid().
var UUIDCheck = func(s string) (string, string, bool) { return "invalid_format", "Invalid UUID", IsUUID(s) }

// UUID is Str with UUIDCheck.
func (f *Form) UUID(key string, r Rule) *string { return f.Str(key, r, StrOpts{Check: UUIDCheck}) }

// EnumCheck is z.enum(options).
func EnumCheck(options []string) func(string) (string, string, bool) {
	return func(s string) (string, string, bool) {
		quoted := make([]string, len(options))
		for i, o := range options {
			quoted[i] = `"` + o + `"`
		}
		return "invalid_value", "Invalid option: expected one of " + strings.Join(quoted, "|"), slices.Contains(options, s)
	}
}

// Enum is z.enum(options).
// zod answers invalid_value with the options for a missing, null or
// non-string value too, so this does not go through Take.
func (f *Form) Enum(key string, r Rule, options []string) *string {
	if f.obj == nil {
		return nil
	}
	v, sent := f.obj[key]
	if (!sent && (r.Optional || r.HasDefault)) || (sent && v == nil && r.Nullable) {
		return nil
	}
	s, isString := v.(string)
	if code, msg, ok := EnumCheck(options)(s); !isString || !ok {
		f.Fail(key, code, msg)
	}
	if !isString {
		return nil
	}
	return &s
}

// DatetimeCheck is z.string().datetime({ offset: true }).
var DatetimeCheck = func(s string) (string, string, bool) {
	return "invalid_format", "Invalid ISO datetime", datetimePattern.MatchString(s)
}

// ValidURL mirrors z.string().url(): new URL(s) must succeed, so a scheme is
// required and the special schemes need a host.
func ValidURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "ftp", "ws", "wss":
		return u.Host != ""
	}
	return true
}

// URLCheck is z.string().url().
var URLCheck = func(s string) (string, string, bool) { return "invalid_format", "Invalid URL", ValidURL(s) }

/* ── numbers and booleans ────────────────────────────────────────────── */

// NumOpts are z.number() refinements.
type NumOpts struct {
	Integer  bool
	Positive bool // > 0
	Min, Max *float64
}

// JSNumber formats x like JavaScript's Number#toString: plain decimals for
// 1e-6 <= |x| < 1e21, otherwise exponent form such as 1e+21 or 1.5e-7.
func JSNumber(x float64) string {
	if abs := math.Abs(x); x == 0 || (abs >= 1e-6 && abs < 1e21) {
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	mant, exp, _ := strings.Cut(strconv.FormatFloat(x, 'e', -1, 64), "e")
	sign, digits := exp[:1], strings.TrimLeft(exp[1:], "0")
	return mant + "e" + sign + digits
}

// Bound returns &v for NumOpts.Min and Max.
func Bound(v float64) *float64 { return &v }

// CheckNumber validates v as a number at key (a map key or array index).
func (f *Form) CheckNumber(key any, v any, o NumOpts) (float64, bool) {
	num, ok := v.(json.Number)
	if !ok {
		f.Fail(key, "invalid_type", "Invalid input: expected number, received "+typeName(v, true))
		return 0, false
	}
	x, err := num.Float64()
	if err != nil || math.IsInf(x, 0) {
		f.Fail(key, "invalid_type", "Invalid input: expected number, received number")
		return 0, false
	}
	if o.Integer && (x != math.Trunc(x) || math.Abs(x) > 1<<53-1) {
		f.Fail(key, "invalid_type", "Invalid input: expected int, received number")
		return x, false
	}
	switch {
	case o.Positive && x <= 0:
		f.Fail(key, "too_small", "Too small: expected number to be >0")
		return x, false
	case o.Min != nil && x < *o.Min:
		f.Fail(key, "too_small", "Too small: expected number to be >="+JSNumber(*o.Min))
		return x, false
	case o.Max != nil && x > *o.Max:
		f.Fail(key, "too_big", "Too big: expected number to be <="+JSNumber(*o.Max))
		return x, false
	}
	return x, true
}

// Num returns nil when the key is absent or null (or invalid).
func (f *Form) Num(key string, r Rule, o NumOpts) *float64 {
	v, _, done := f.Take(key, "number", r)
	if done {
		return nil
	}
	x, _ := f.CheckNumber(key, v, o)
	return &x
}

// Int is Num with Integer set.
func (f *Form) Int(key string, r Rule, o NumOpts) *int {
	o.Integer = true
	x := f.Num(key, r, o)
	if x == nil {
		return nil
	}
	n := int(*x)
	return &n
}

// Bool returns nil when the key is absent or null.
func (f *Form) Bool(key string, r Rule) *bool {
	v, _, done := f.Take(key, "boolean", r)
	if done {
		return nil
	}
	b, ok := v.(bool)
	if !ok {
		f.Fail(key, "invalid_type", "Invalid input: expected boolean, received "+typeName(v, true))
	}
	return &b
}

// BoolDefault is z.boolean().default(def).
func (f *Form) BoolDefault(key string, def bool) bool {
	if b := f.Bool(key, Rule{HasDefault: true}); b != nil {
		return *b
	}
	return def
}

/* ── arrays ──────────────────────────────────────────────────────────── */

// List validates z.array(...).max(max); each item is checked by each with a
// form rooted at the array. It returns nil when absent or null.
func (f *Form) List(key string, r Rule, max int, each func(items *Form, i int, v any)) []any {
	v, _, done := f.Take(key, "array", r)
	if done {
		return nil
	}
	items, ok := v.([]any)
	if !ok {
		f.Fail(key, "invalid_type", "Invalid input: expected array, received "+typeName(v, true))
		return nil
	}
	sub := &Form{path: f.at(key), issues: f.issues}
	for i, item := range items {
		each(sub, i, item)
	}
	if len(items) > max {
		f.Fail(key, "too_big", fmt.Sprintf("Too big: expected array to have <=%d items", max))
	}
	if items == nil {
		items = []any{}
	}
	return items
}

// Strings is z.array(z.string()).max(max).
func (f *Form) Strings(key string, r Rule, max int, o StrOpts) []string {
	out := []string{}
	items := f.List(key, r, max, func(sub *Form, i int, v any) {
		s, _ := sub.CheckString(i, v, o)
		out = append(out, s)
	})
	if items == nil {
		return nil
	}
	return out
}

// Ints is z.array(z.number().int()).max(max).
func (f *Form) Ints(key string, r Rule, max int, o NumOpts) []int {
	o.Integer = true
	out := []int{}
	items := f.List(key, r, max, func(sub *Form, i int, v any) {
		x, _ := sub.CheckNumber(i, v, o)
		out = append(out, int(x))
	})
	if items == nil {
		return nil
	}
	return out
}

// Item validates one array item as an object.
func (f *Form) Item(i int, v any) *Form {
	return &Form{path: f.at(i), issues: f.issues, obj: f.object(i, v, true)}
}

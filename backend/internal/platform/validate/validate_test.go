package validate

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/httpx"
)

func form(t *testing.T, body string) *Form {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	return New(v, true)
}

func TestFieldsInSchemaOrder(t *testing.T) {
	f := form(t, `{"name":"  ","qty":1.5,"kind":"x","spec":{"distanceM":-1}}`)
	f.Str("name", Rule{}, StrOpts{Trim: true, Min: 1})
	f.Int("qty", Rule{}, NumOpts{})
	f.Enum("kind", Rule{}, []string{"run", "row"})
	f.Child("spec").Num("distanceM", Rule{}, NumOpts{Min: Bound(0)})
	f.UUID("id", Rule{Optional: true})

	got := []string{}
	for _, is := range f.Issues() {
		got = append(got, is.Code+" "+is.Message)
	}
	want := []string{
		"too_small Too small: expected string to have >=1 characters",
		"invalid_type Invalid input: expected int, received number",
		`invalid_value Invalid option: expected one of "run"|"row"`,
		"too_small Too small: expected number to be >=0",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("issues = %q", got)
	}
	if p := f.Issues()[3].Path; !reflect.DeepEqual(p, []any{"spec", "distanceM"}) {
		t.Fatalf("nested path = %v", p)
	}
}

func TestErrShapes(t *testing.T) {
	f := form(t, `{"items":[{"sku":""}]}`)
	f.List("items", Rule{}, 10, func(items *Form, i int, v any) {
		items.Item(i, v).Str("sku", Rule{}, StrOpts{Min: 1})
	})
	var he *httpx.Error
	if err := f.ErrAtPath("Data tidak valid"); !errors.As(err, &he) || he.Status != 400 || he.Message != "Data tidak valid: items.0.sku" {
		t.Fatalf("ErrAtPath = %v", err)
	}
	if err := f.Err("Validation failed"); !errors.As(err, &he) || he.Message != "Validation failed" {
		t.Fatalf("Err = %v", err)
	}
	if err := form(t, `{}`).Err("Validation failed"); err != nil {
		t.Fatalf("empty schema should pass, got %v", err)
	}
}

func TestMissingBodyAndNulls(t *testing.T) {
	f := New(nil, false)
	if f.Valid() || f.Issues()[0].Message != "Invalid input: expected object, received undefined" {
		t.Fatalf("missing body issues = %v", f.Issues())
	}
	g := form(t, `{"note":null,"tag":null}`)
	if g.Str("note", Rule{Nullable: true}, StrOpts{}) != nil || !g.Valid() {
		t.Fatal("nullable null should pass as nil")
	}
	g.Str("tag", Rule{Optional: true}, StrOpts{})
	if g.Valid() || g.Issues()[0].Message != "Invalid input: expected string, received null" {
		t.Fatalf("optional rejects null: %v", g.Issues())
	}
	if g.StrDefault("missing", "def", StrOpts{}) != "def" || g.BoolDefault("flag", true) != true {
		t.Fatal("defaults")
	}
}

func TestJSStringRules(t *testing.T) {
	if UTF16Len("😀") != 2 {
		t.Fatal("emoji counts as two UTF-16 units, like JS length")
	}
	if JSTrim("\uFEFF a \u00a0") != "a" {
		t.Fatal("JSTrim drops the BOM and NBSP")
	}
	if !ValidURL("mailto:x@y.z") || ValidURL("https://") || ValidURL("no-scheme") {
		t.Fatal("ValidURL follows new URL()")
	}
	if !IsUUID("00000000-0000-0000-0000-000000000000") || IsUUID("123") {
		t.Fatal("IsUUID")
	}
}

func TestZodWording(t *testing.T) {
	for x, want := range map[float64]string{1000000000: "1000000000", 0.5: "0.5", 1e21: "1e+21", 1.5e-7: "1.5e-7", -2e22: "-2e+22", 0: "0"} {
		if got := JSNumber(x); got != want {
			t.Errorf("JSNumber(%v) = %q, want %q", x, got, want)
		}
	}
	f := form(t, `{"amount":2000000000,"b":null,"c":5,"d":"x","e":"a"}`)
	f.Num("amount", Rule{}, NumOpts{Max: Bound(1000000000)})
	for _, k := range []string{"a", "b", "c", "d"} {
		if v := f.Enum(k, Rule{}, []string{"a", "b"}); k == "d" && (v == nil || *v != "x") {
			t.Fatalf("Enum keeps the string it rejected: %v", v)
		}
	}
	if v := f.Enum("e", Rule{}, []string{"a", "b"}); v == nil || *v != "a" {
		t.Fatalf("Enum e = %v", v)
	}
	f.Enum("missing", Rule{Optional: true}, []string{"a"})
	f.Enum("b", Rule{Nullable: true}, []string{"a"})
	got := []string{}
	for _, is := range f.Issues() {
		got = append(got, is.Code+" "+is.Message)
	}
	enum := `invalid_value Invalid option: expected one of "a"|"b"`
	want := []string{"too_big Too big: expected number to be <=1000000000", enum, enum, enum, enum}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("issues = %q", got)
	}
}

// The zod v4 string formats: datetime with offset, UUID and URL.
func TestZodFormatChecks(t *testing.T) {
	for s, want := range map[string]bool{
		"2026-10-05T18:00:00+07:00": true, "2026-10-05T18:00+07:00": true, "2026-10-05T11:00:00.123456789Z": true,
		"2026-10-05T18:00:00+0700": false, "2026-10-05 18:00:00Z": false, "2026-02-29T00:00:00Z": false,
	} {
		if got := datetimePattern.MatchString(s); got != want {
			t.Errorf("datetime %q = %v", s, got)
		}
	}
	for s, want := range map[string]bool{
		"6f1c2a1e-0d7b-4c55-9a43-1b0b6a0f5e11": true, "00000000-0000-0000-0000-000000000000": true,
		"6f1c2a1e-0d7b-9c55-9a43-1b0b6a0f5e11": false, "6f1c2a1e-0d7b-4c55-1a43-1b0b6a0f5e11": false,
	} {
		if IsUUID(s) != want {
			t.Errorf("uuid %q", s)
		}
	}
	for s, want := range map[string]bool{"https://cdn.test/a.png": true, "mailto:a@b": true, "abc": false, "http://": false} {
		if ValidURL(s) != want {
			t.Errorf("url %q", s)
		}
	}
}

func TestCanonicalUUID(t *testing.T) {
	const want = "7b0c1d2e-3f40-4a5b-8c6d-7e8f90a1b2c3"
	for _, in := range []string{want, "7B0C1D2E-3F40-4A5B-8C6D-7E8F90A1B2C3", "7b0c1d2e3f404a5b8c6d7e8f90a1b2c3",
		"{7b0c1d2e-3f40-4a5b-8c6d-7e8f90a1b2c3}", "{7B0C1D2E3F404A5B8C6D7E8F90A1B2C3}"} {
		if got, ok := CanonicalUUID(in); !ok || got != want {
			t.Errorf("CanonicalUUID(%q) = %q, %v", in, got, ok)
		}
	}
	for _, in := range []string{"", "nope", "7b0c1d2e-3f40-4a5b-8c6d-7e8f90a1b2c", "7b0c1d2e-3f40-4a5b-8c6d-7e8f90a1b2cz",
		"7b0c1d2e3f40-4a5b-8c6d-7e8f90a1b2c3", " 7b0c1d2e-3f40-4a5b-8c6d-7e8f90a1b2c3", "{7b0c1d2e-3f40-4a5b-8c6d-7e8f90a1b2c3"} {
		if got, ok := CanonicalUUID(in); ok {
			t.Errorf("CanonicalUUID(%q) = %q, want invalid", in, got)
		}
	}
}

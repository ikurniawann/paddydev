package httpx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func serve(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func TestHandleErrors(t *testing.T) {
	cases := []struct {
		err    error
		status int
		body   string
	}{
		{BadRequest("Validation failed", []string{"x"}), 400, `{"success":false,"error":"Validation failed","details":["x"]}`},
		{Unauthorized(""), 401, `{"success":false,"error":"Authentication required"}`},
		{Forbidden(""), 403, `{"success":false,"error":"Insufficient permissions"}`},
		{NotFound(""), 404, `{"success":false,"error":"Resource not found"}`},
		{Conflict("Sudah ada"), 409, `{"success":false,"error":"Sudah ada"}`},
		{TooManyRequests(""), 429, `{"success":false,"error":"Terlalu banyak permintaan. Silakan coba lagi dalam beberapa saat."}`},
		{fmt.Errorf("wrapped: %w", Status(418, "teh")), 418, `{"success":false,"error":"teh"}`},
		{&pgconn.PgError{Code: "23505"}, 409, `{"success":false,"error":"Data sudah ada di sistem"}`},
		{&pgconn.PgError{Code: "23505", ConstraintName: "employees_npwp_key"}, 409, `{"success":false,"error":"NPWP sudah dipakai karyawan lain"}`},
		{fmt.Errorf("x: %w", &pgconn.PgError{Code: "23505", ConstraintName: "employees_ktp_key"}), 409, `{"success":false,"error":"No. KTP sudah dipakai karyawan lain"}`},
		{&pgconn.PgError{Code: "23505", ConstraintName: "employees_nip_key"}, 409, `{"success":false,"error":"NIP sudah dipakai karyawan lain"}`},
		{&pgconn.PgError{Code: "23505", ConstraintName: "some_other_key"}, 409, `{"success":false,"error":"Data sudah ada di sistem"}`},
		{fmt.Errorf("x: %w", &pgconn.PgError{Code: "23503"}), 400, `{"success":false,"error":"Referensi data tidak valid"}`},
		{&pgconn.PgError{Code: "23514"}, 400, `{"success":false,"error":"Data tidak memenuhi ketentuan"}`},
		{&pgconn.PgError{Code: "23502"}, 400, `{"success":false,"error":"Data wajib diisi"}`},
		{&pgconn.PgError{Code: "22P02"}, 400, `{"success":false,"error":"Format data tidak valid"}`},
		{&pgconn.PgError{Code: "42P01"}, 500, `{"success":false,"error":"Terjadi kesalahan server"}`},
		{errors.New("boom"), 500, `{"success":false,"error":"Terjadi kesalahan server"}`},
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, c := range cases {
		h := Handle(func(w http.ResponseWriter, r *http.Request) error { return c.err })
		rec := serve(h, httptest.NewRequest("GET", "/", nil))
		if rec.Code != c.status || rec.Body.String() != c.body {
			t.Errorf("%v: got %d %s, want %d %s", c.err, rec.Code, rec.Body.String(), c.status, c.body)
		}
	}
}

func TestJSONMatchesJSONStringify(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := Data(rec, 201, map[string]string{"q": "<a & b>"}); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 201 || rec.Body.String() != `{"success":true,"data":{"q":"<a & b>"}}` {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content type %s", ct)
	}
}

func TestDecodeJSON(t *testing.T) {
	var dst struct {
		Name string `json:"name"`
	}
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"a","extra":1}`))
	if err := DecodeJSON(r, &dst); err != nil || dst.Name != "a" {
		t.Fatalf("valid body: %v %v", err, dst)
	}
	for _, body := range []string{"", "{", "nope"} {
		r := httptest.NewRequest("POST", "/", strings.NewReader(body))
		err := DecodeJSON(r, &dst)
		var e *Error
		if !errors.As(err, &e) || e.Status != 400 || e.Message != "Payload tidak valid" {
			t.Errorf("body %q: %v", body, err)
		}
	}
}

func TestMiddleware(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/panic" {
			panic("kaboom")
		}
		_, _ = w.Write([]byte(RequestID(r.Context())))
	}), WithRequestID, AccessLog(log), Recover(log))

	rec := serve(h, httptest.NewRequest("GET", "/ok", nil))
	if rec.Body.Len() == 0 || rec.Header().Get(RequestIDHeader) != rec.Body.String() {
		t.Fatalf("request id not set: %q %q", rec.Body.String(), rec.Header().Get(RequestIDHeader))
	}
	r := httptest.NewRequest("GET", "/ok", nil)
	r.Header.Set(RequestIDHeader, "abc-123")
	if rec := serve(h, r); rec.Body.String() != "abc-123" {
		t.Fatalf("caller id not kept: %q", rec.Body.String())
	}

	rec = serve(h, httptest.NewRequest("GET", "/panic", nil))
	if rec.Code != 500 || rec.Body.String() != `{"success":false,"error":"Terjadi kesalahan server"}` {
		t.Fatalf("panic: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(logs.String(), `"msg":"panic recovered"`) || !strings.Contains(logs.String(), `"status":500`) {
		t.Fatalf("logs: %s", logs.String())
	}
}

func TestJSTime(t *testing.T) {
	jkt := time.FixedZone("WIB", 7*3600)
	ts := time.Date(2026, 10, 4, 15, 56, 52, 984249000, jkt)
	rec := httptest.NewRecorder()
	_ = JSON(rec, 200, map[string]any{"at": JSTime(ts), "none": NewJSTime(nil)})
	if rec.Body.String() != `{"at":"2026-10-04T08:56:52.984Z","none":null}` {
		t.Fatal(rec.Body.String())
	}
}

func TestDraining(t *testing.T) {
	if Draining(context.Background()) != nil {
		t.Fatal("no server: nil channel, which blocks forever in select")
	}
	draining := make(chan struct{})
	ctx := WithShutdown(context.Background(), draining)
	close(draining)
	select {
	case <-Draining(ctx):
	default:
		t.Fatal("closed channel must be visible through the context")
	}
}

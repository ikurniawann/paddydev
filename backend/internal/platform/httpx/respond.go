package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgconn"
)

// maxBodyBytes caps JSON request bodies.
const maxBodyBytes = 10 << 20

// HandlerFunc is a handler that returns its failure instead of writing it.
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

// Handle adapts a HandlerFunc: a returned *Error renders with its status, a
// PostgreSQL constraint error renders as the friendly 4xx from handler.ts,
// anything else is logged and renders as 500 "Terjadi kesalahan server".
func Handle(h HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			WriteError(w, r, err)
		}
	})
}

// WriteError renders err with the shared error envelope.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	var appErr *Error
	if errors.As(err, &appErr) {
		writeEnvelope(w, appErr)
		return
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" {
			if msg, ok := UniqueConstraintMessage(pgErr.ConstraintName); ok {
				writeEnvelope(w, Conflict(msg))
				return
			}
		}
		if mapped, ok := pgErrors[pgErr.Code]; ok {
			writeEnvelope(w, mapped)
			return
		}
	}
	slog.ErrorContext(r.Context(), "request failed",
		"method", r.Method, "path", r.URL.Path, "request_id", RequestID(r.Context()), "error", err.Error())
	writeEnvelope(w, internalError)
}

type errorEnvelope struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Details any    `json:"details,omitempty"`
}

func writeEnvelope(w http.ResponseWriter, e *Error) {
	_ = JSON(w, e.Status, errorEnvelope{Success: false, Error: e.Message, Details: e.Details})
}

// JSON writes body as JSON with the given status. HTML characters are not
// escaped, matching JSON.stringify.
func JSON(w http.ResponseWriter, status int, body any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(body); err != nil {
		return err
	}
	out := bytes.TrimRight(buf.Bytes(), "\n")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(out)))
	w.WriteHeader(status)
	_, err := w.Write(out)
	return err
}

// DecodeJSON reads the request body into dst. Unknown fields are ignored
// (zod strips them in the TS routes). An empty or malformed body is a
// 400 "Payload tidak valid".
func DecodeJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return BadRequest("Payload tidak valid")
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		return BadRequest("Payload tidak valid")
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return BadRequest("Payload tidak valid")
	}
	return nil
}

// dataEnvelope keeps "success" before "data", as NextResponse.json does for
// { success: true, data }. A Go map would sort the keys.
type dataEnvelope struct {
	Success bool   `json:"success"`
	Data    any    `json:"data"`
	Message string `json:"message,omitempty"`
}

// Data writes {"success":true,"data":data} (successResponse in TS).
func Data(w http.ResponseWriter, status int, data any) error {
	return JSON(w, status, dataEnvelope{Success: true, Data: data})
}

// DataMessage writes {"success":true,"data":data,"message":msg}
// (successResponse/createdResponse with a message).
func DataMessage(w http.ResponseWriter, status int, data any, msg string) error {
	return JSON(w, status, dataEnvelope{Success: true, Data: data, Message: msg})
}

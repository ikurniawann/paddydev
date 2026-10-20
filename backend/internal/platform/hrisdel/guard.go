// Package hrisdel guards HRIS deletes (EPIC-061, Q-RA-1/4): the "delete"
// action on the route's own leaf menu (a group prefix or a sibling leaf
// does not count), a reason of at least five characters, and an
// audit.audit_log row with the deleted row as before.
package hrisdel

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/validate"
)

// MinReason is the shortest accepted reason, in characters.
const MinReason = 5

// ErrReason is the 400 for a missing or short reason. The frontend matches
// it (frontend/src/lib/hris/delete-reason.ts) to ask for one.
const ErrReason = "Alasan hapus wajib diisi (minimal 5 karakter)"

// Checker is the part of *auth.Service the guard needs.
type Checker interface {
	RequireMenuAction(r *http.Request, action string, prefixes ...string) (*auth.User, error)
}

// Spec describes one guarded route.
type Spec struct {
	Menu    string // leaf menu that must grant "delete"
	Entity  string // audit entity; the action is Entity + ".delete"
	Table   string // optional table snapshotted by id into before (a constant, never input)
	IDParam string // path value holding the row id ("id", "doc_id")
	IDQuery string // query key holding the id when the path has none
	// SelfAudited: the handler validates the reason and writes its own
	// audit (attendance); the guard checks the permission only.
	SelfAudited bool
}

func (s Spec) id(r *http.Request) string {
	if s.IDParam != "" {
		return r.PathValue(s.IDParam)
	}
	if s.IDQuery != "" {
		return r.URL.Query().Get(s.IDQuery)
	}
	return ""
}

// CheckReason enforces MinReason on the trimmed reason.
func CheckReason(reason string) error {
	if utf8.RuneCountInString(strings.TrimSpace(reason)) < MinReason {
		return httpx.BadRequest(ErrReason)
	}
	return nil
}

// Reason reads ?reason= or a JSON body {"reason": "..."}; the body is put
// back so the handler can still read it.
func Reason(r *http.Request) string {
	if v := strings.TrimSpace(r.URL.Query().Get("reason")); v != "" {
		return v
	}
	if r.Body == nil {
		return ""
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(raw))
	if err != nil || len(raw) == 0 {
		return ""
	}
	var body struct {
		Reason any `json:"reason"`
	}
	if json.Unmarshal(raw, &body) != nil {
		return ""
	}
	s, _ := body.Reason.(string)
	return strings.TrimSpace(s)
}

// Guard runs next only for a caller holding "delete" on spec.Menu and,
// unless SelfAudited, a valid reason; after a 2xx answer it writes the
// audit row on q (the module's database: the pool, or a test transaction).
func Guard(c Checker, q database.Querier, spec Spec, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := c.RequireMenuAction(r, "delete", spec.Menu)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		if spec.SelfAudited {
			next.ServeHTTP(w, r)
			return
		}
		reason := Reason(r)
		if err := CheckReason(reason); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		id := spec.id(r)
		before, err := snapshot(r.Context(), q, spec.Table, id)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		if sw.status < 200 || sw.status > 299 {
			return
		}
		e := audit.Entry{ActorID: u.ID, ActorName: &u.FullName, Action: spec.Entity + ".delete",
			Entity: spec.Entity, Reason: &reason}.WithRequest(r)
		if id != "" {
			e.EntityID = &id
		}
		if before != nil {
			e.Before = before
		} else {
			e.Before = target(r)
		}
		if err := audit.Write(r.Context(), q, e); err != nil {
			slog.ErrorContext(r.Context(), "hrisdel: audit write failed", "entity", spec.Entity, "id", id, "error", err)
		}
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// snapshot is the row about to be deleted, nil when there is no table, no
// id, or no such row (the handler answers the 404).
func snapshot(ctx context.Context, q database.Querier, table, id string) (json.RawMessage, error) {
	if table == "" || id == "" {
		return nil, nil
	}
	// A uuid in any accepted form is compared canonically, so the snapshot
	// finds the row the delete will hit (EPIC-061).
	if c, ok := validate.CanonicalUUID(id); ok {
		id = c
	}
	var raw []byte
	err := q.QueryRow(ctx, `SELECT to_jsonb(t) FROM `+table+` t WHERE t.id::text = $1`, id).Scan(&raw)
	if database.IsNoRows(err) {
		return nil, nil
	}
	return raw, err
}

// target is before for a delete without a row snapshot: the path and the
// query without the reason.
func target(r *http.Request) map[string]any {
	query := r.URL.Query()
	query.Del("reason")
	return map[string]any{"path": r.URL.Path, "query": query.Encode()}
}

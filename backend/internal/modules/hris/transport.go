package hris

import (
	"context"
	"net/http"
	"net/url"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/hrisdel"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/validate"
)

// The TS routes answer with NextResponse.json of ad-hoc objects ({ data },
// { data, message }, { message }, …) rather than the {success, data}
// envelope, so handlers build ordered objects with obj().

func reply(w http.ResponseWriter, status int, body any) error { return httpx.JSON(w, status, body) }

func ok(w http.ResponseWriter, body any) error { return httpx.JSON(w, http.StatusOK, body) }

func dataOf(v any) *Row { return obj("data", v) }

func dataMsg(v any, msg string) *Row { return obj("data", v, "message", msg) }

func msgOf(msg string) *Row { return obj("message", msg) }

// handle is apiHandler: errors render as {success:false,error}.
func handle(fn httpx.HandlerFunc) http.Handler { return httpx.Handle(fn) }

// withUser is requireApiUser.
func (h handlers) withUser(fn func(w http.ResponseWriter, r *http.Request, u *auth.User) error) http.Handler {
	return handle(func(w http.ResponseWriter, r *http.Request) error {
		u, err := h.auth.RequireUser(r)
		if err != nil {
			return err
		}
		return fn(w, r, u)
	})
}

// withMenu is requireIamMenuPrefix(prefixes).
func (h handlers) withMenu(prefixes []string, fn func(w http.ResponseWriter, r *http.Request, u *auth.User) error) http.Handler {
	return handle(func(w http.ResponseWriter, r *http.Request) error {
		u, err := h.auth.RequireMenuPrefix(r, prefixes...)
		if err != nil {
			return err
		}
		return fn(w, r, u)
	})
}

// withDelete is hrisdel.Guard on this module's database (EPIC-061): the
// "delete" action on the route's leaf menu, a reason, an audit row.
func (h handlers) withDelete(spec hrisdel.Spec, next http.Handler) http.Handler {
	return hrisdel.Guard(h.auth, h.svc.repo.querier(), spec, next)
}

// withActor is requireWorkforceActor: a session (401 "Unauthorized").
func (h handlers) withActor(fn func(w http.ResponseWriter, r *http.Request, a *Actor) error) http.Handler {
	return handle(func(w http.ResponseWriter, r *http.Request) error {
		a, err := h.actor(r)
		if err != nil {
			return err
		}
		if a == nil {
			return httpx.Unauthorized("Unauthorized")
		}
		return fn(w, r, a)
	})
}

// actor is getWorkforceActor: nil when nobody is signed in.
func (h handlers) actor(r *http.Request) (*Actor, error) {
	u, err := h.auth.CurrentUser(r)
	if err != nil || u == nil {
		return nil, err
	}
	return h.actorFor(r.Context(), u)
}

// actorFor is svc.Actor plus the leaves of iam.HrisApproveLeaves on which
// the account holds "approve" (EPIC-067 R11): the resolved IAM roles' grants,
// as auth.RequireMenuAction reads them, matched per leaf. They are read on
// the module's querier (the same database as h.auth in production), two or
// three queries for every leaf together.
func (h handlers) actorFor(ctx context.Context, u *auth.User) (*Actor, error) {
	a, err := h.svc.Actor(ctx, u)
	if err != nil {
		return nil, err
	}
	q := h.svc.repo.querier()
	roleIDs, err := auth.ResolveRoleIDsOn(ctx, q, u.ID, u.Role)
	if err != nil {
		return nil, err
	}
	granted, err := auth.GrantedMenuActionsOn(ctx, q, roleIDs)
	if err != nil {
		return nil, err
	}
	for _, leaf := range iam.HrisApproveLeaves {
		if iam.HasGrantedAction(granted, []string{leaf}, "approve") {
			a.Approves = append(a.Approves, leaf)
		}
	}
	return a, nil
}

// GET /api/hris/me/approve-access (EPIC-067 R14): the decisions the account
// may take on the HR side, so the pages show only the buttons it can use.
func (h handlers) myApproveAccess(w http.ResponseWriter, _ *http.Request, a *Actor) error {
	return ok(w, obj("attendance", a.CanApprove(iam.LeafAbsensi), "leaves", a.CanApprove(iam.LeafCuti),
		"overtime", a.CanApprove(iam.LeafLembur)))
}

// requireEmployeeAccess passes managers (any of the prefixes) and the
// employee themself; otherwise 403.
func (h handlers) requireEmployeeAccess(r *http.Request, employeeID string, managerPrefixes []string) error {
	u, err := h.auth.RequireUser(r)
	if err != nil {
		return err
	}
	manager, err := h.auth.HasMenuPrefix(r.Context(), u, managerPrefixes...)
	if err != nil || manager {
		return err
	}
	own, err := h.svc.repo.EmployeeIDByUser(r.Context(), u.ID)
	if err != nil {
		return err
	}
	if own == nil || *own != employeeID {
		return httpx.Forbidden("Insufficient permissions")
	}
	return nil
}

/* ── Query strings ───────────────────────────────────────────────────── */

// queryOf adapts url.Values to the domain parsers (URLSearchParams.get).
func queryOf(r *http.Request) domain.Query {
	v := r.URL.Query()
	return func(k string) (string, bool) {
		if !v.Has(k) {
			return "", false
		}
		return v.Get(k), true
	}
}

// get is searchParams.get(key) (nil when absent, "" kept).
func get(v url.Values, key string) *string {
	if !v.Has(key) {
		return nil
	}
	s := v.Get(key)
	return &s
}

/* ── Bodies ──────────────────────────────────────────────────────────── */

// readForm is readJson's first half: a body that is not JSON is a 400 with
// message (default "Body JSON tidak valid").
func readForm(r *http.Request, message string) (*validate.Form, error) {
	body, present := validate.ReadBody(r)
	if !present {
		if message == "" {
			message = "Body JSON tidak valid"
		}
		return nil, httpx.BadRequest(message)
	}
	return validate.New(body, true), nil
}

// optionalForm is readOptionalJson: a missing or non-JSON body is {}.
func optionalForm(r *http.Request) *validate.Form {
	body, present := validate.ReadBody(r)
	if !present || body == nil {
		body = map[string]any{}
	}
	return validate.New(body, true)
}

// formErr is parseInput's failure: 400 with message, or the first issue's
// message, and the issues as details.
func formErr(f *validate.Form, message string) error {
	if f.Valid() {
		return nil
	}
	if message == "" {
		message = f.Issues()[0].Message
	}
	return httpx.BadRequest(message, f.Issues())
}

// queryForm validates query parameters (searchParamsOf: empty values
// dropped) as a zod object.
func queryForm(r *http.Request) *validate.Form {
	obj := map[string]any{}
	for k, vs := range r.URL.Query() {
		for _, v := range vs {
			if v != "" {
				obj[k] = v // Object.fromEntries keeps the last entry
			}
		}
	}
	return validate.New(obj, true)
}

// requireUUID is requireUuid: a path id that is not a UUID is a 400.
func requireUUID(id, message string) (string, error) {
	if !domain.IsUUID(id) {
		if message == "" {
			message = "ID tidak valid"
		}
		return "", httpx.BadRequest(message)
	}
	return id, nil
}

// withAction is requireIamAction: a menu under the prefixes must grant the
// action (EPIC-056 write routes: resign, import, schedule changes).
func (h handlers) withAction(action string, prefixes []string, fn func(w http.ResponseWriter, r *http.Request, u *auth.User) error) http.Handler {
	return handle(func(w http.ResponseWriter, r *http.Request) error {
		u, err := h.auth.RequireMenuAction(r, action, prefixes...)
		if err != nil {
			return err
		}
		return fn(w, r, u)
	})
}

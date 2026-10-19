package hris

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
)

// Employee audit trail (Spec §1.3): audit.audit_log rows with entity
// hris.employees, written on the caller's querier so they commit with the
// change. before/after hold only the changed fields.
const employeeEntity = "hris.employees"

func writeEmployeeAudit(ctx context.Context, q database.Querier, actor *auth.User, action, employeeID, label string, before, after any) error {
	e := audit.Entry{Action: action, Entity: employeeEntity, EntityID: &employeeID, Before: before, After: after}
	if label != "" {
		e.EntityLabel = &label
	}
	if actor != nil {
		e.ActorID = actor.ID
		name := actor.FullName
		e.ActorName = &name
	}
	return audit.Write(ctx, q, e)
}

type employeeAuditRow struct {
	ID        string          `json:"id"`
	CreatedAt time.Time       `json:"created_at"`
	ActorName *string         `json:"actor_name"`
	Action    string          `json:"action"`
	Before    json.RawMessage `json:"before"`
	After     json.RawMessage `json:"after"`
	Reason    *string         `json:"reason"`
}

// employeeAudit is GET /api/hris/employees/{id}/audit: the employee's audit
// trail, newest first, 20 per page.
func (h handlers) employeeAudit(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	id, err := requireUUID(r.PathValue("id"), "ID karyawan tidak valid")
	if err != nil {
		return err
	}
	page := 1
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 0 {
		page = p
	}
	q := h.svc.repo.querier()
	var total int
	if err := q.QueryRow(r.Context(), `SELECT count(*) FROM audit.audit_log WHERE entity = $1 AND entity_id = $2`,
		employeeEntity, id).Scan(&total); err != nil {
		return err
	}
	rows, err := q.Query(r.Context(), `SELECT id::text, created_at, actor_name, action,
		coalesce(before, 'null'::jsonb)::text, coalesce(after, 'null'::jsonb)::text, reason
		FROM audit.audit_log WHERE entity = $1 AND entity_id = $2
		ORDER BY created_at DESC, id DESC LIMIT 20 OFFSET $3`, employeeEntity, id, (page-1)*20)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []employeeAuditRow{}
	for rows.Next() {
		var a employeeAuditRow
		var before, after string
		if err := rows.Scan(&a.ID, &a.CreatedAt, &a.ActorName, &a.Action, &before, &after, &a.Reason); err != nil {
			return err
		}
		a.Before, a.After = json.RawMessage(before), json.RawMessage(after)
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return ok(w, map[string]any{"success": true, "data": out,
		"meta": map[string]any{"total": total, "totalPages": max(1, (total+19)/20)}})
}

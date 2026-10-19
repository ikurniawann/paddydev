package hris

import (
	"context"

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

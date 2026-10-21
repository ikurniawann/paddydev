package hris

import (
	"context"

	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/database"
)

// Paket Approval atasan (EPIC-063): what leave and overtime decisions share.

// decisionRace is the 409 of a request another approver decided meanwhile.
const decisionRace = "Pengajuan sudah diproses oleh orang lain"

// auditDecision records a decision with its capacity (Q-APV-3: who decided,
// as direct manager or as HR) on q, inside the decision's transaction (LA-01).
func auditDecision(ctx context.Context, q database.Querier, a *Actor, action, entity, id, label string,
	after map[string]any, meta audit.Entry) error {
	meta.ActorID, meta.ActorName = a.UserID, ptr(actorName(a))
	meta.Action, meta.Entity, meta.EntityID, meta.EntityLabel = action, entity, &id, &label
	meta.Before = map[string]any{"status": "pending"}
	meta.After = after
	return audit.Write(ctx, q, meta)
}

package app

import (
	"context"

	"nuhabit/backend/internal/modules/identity"
	"nuhabit/backend/internal/modules/identity/domain"
	"nuhabit/backend/internal/platform/database"
)

// identityStalls adapts configuration.warehouses and user_warehouses to the
// identity stall port with the SQL of lib/auth/stall-access.ts and
// lib/auth/active-stall.ts.
type identityStalls struct{ db database.Querier }

var _ identity.Stalls = identityStalls{}

// Access is getStallAccess: placements and the can_switch_stall flag decide
// free access; with it every active stall of the branch is selectable,
// otherwise only the placements.
func (s identityStalls) Access(ctx context.Context, userID, role string, branchID *string) (domain.StallAccess, error) {
	assigned, err := s.list(ctx, `SELECT w.id::text, w.name, w.code, w.is_default, w.branch_id::text, COALESCE(b.name, '')
		FROM configuration.user_warehouses uw
		JOIN configuration.warehouses w ON w.id = uw.warehouse_id AND w.is_active
		LEFT JOIN configuration.branches b ON b.id = w.branch_id
		WHERE uw.user_id = $1 AND uw.is_active`, userID)
	if err != nil {
		return domain.StallAccess{}, err
	}
	var canSwitch bool
	err = s.db.QueryRow(ctx, `SELECT COALESCE(can_switch_stall, false) AS can_switch_stall
		FROM configuration.users WHERE id = $1`, userID).Scan(&canSwitch)
	if err != nil && !database.IsNoRows(err) {
		return domain.StallAccess{}, err
	}
	main := false
	for _, a := range assigned {
		main = main || a.IsDefault
	}
	access := domain.StallAccess{AllAccess: domain.StallAllAccess(role, canSwitch, main)}
	if !access.AllAccess {
		access.Stalls = domain.SortStallsByBranch(assigned)
		return access, nil
	}
	sql, args := `SELECT w.id::text, w.name, w.code, w.is_default, w.branch_id::text, COALESCE(b.name, '')
		FROM configuration.warehouses w
		LEFT JOIN configuration.branches b ON b.id = w.branch_id
		WHERE w.is_active`, []any{}
	if branchID != nil && *branchID != "" {
		sql, args = sql+` AND w.branch_id = $1`, append(args, *branchID)
	}
	all, err := s.list(ctx, sql, args...)
	if err != nil {
		return domain.StallAccess{}, err
	}
	access.Stalls = domain.SortStallsByBranch(all)
	return access, nil
}

func (s identityStalls) list(ctx context.Context, sql string, args ...any) ([]domain.Stall, error) {
	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Stall
	for rows.Next() {
		var st domain.Stall
		if err := rows.Scan(&st.ID, &st.Name, &st.Code, &st.IsDefault, &st.BranchID, &st.BranchName); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// Active is findActiveStall's query.
func (s identityStalls) Active(ctx context.Context, id string) (*domain.Stall, error) {
	var st domain.Stall
	err := s.db.QueryRow(ctx, `SELECT w.id::text, w.name, w.code, w.branch_id::text, COALESCE(b.name, '')
		FROM configuration.warehouses w
		LEFT JOIN configuration.branches b ON b.id = w.branch_id
		WHERE w.id = $1 AND w.is_active = true`, id).Scan(&st.ID, &st.Name, &st.Code, &st.BranchID, &st.BranchName)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &st, nil
}

// Branch is the active-stall route's branch check read.
func (s identityStalls) Branch(ctx context.Context, id string) (*string, bool, error) {
	var branch *string
	err := s.db.QueryRow(ctx, `SELECT branch_id::text FROM configuration.warehouses WHERE id = $1`, id).Scan(&branch)
	if database.IsNoRows(err) {
		return nil, false, nil
	}
	return branch, err == nil, err
}

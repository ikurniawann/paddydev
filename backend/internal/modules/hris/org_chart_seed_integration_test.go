package hris

import (
	"context"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

// EPIC-101: the Draft 6 org chart is seeded by migration from the client CSV.
func TestOrgChartDraft6Seed(t *testing.T) {
	tx := testutil.Tx(t)
	text := func(sql string, args ...any) string {
		t.Helper()
		var s string
		if err := tx.QueryRow(context.Background(), sql, args...).Scan(&s); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return s
	}
	const nodes = `FROM hris.org_chart_nodes n JOIN hris.org_chart_versions v ON v.id = n.version_id WHERE v.label = 'Draft 6'`
	checks := []struct{ sql, want string }{
		{`SELECT count(*)::text FROM hris.org_chart_versions WHERE label = 'Draft 6' AND status = 'draft'`, "1"},
		{`SELECT count(*)::text ` + nodes, "150"},
		{`SELECT count(*)::text ` + nodes + ` AND n.is_position`, "149"},
		{`SELECT count(DISTINCT c)::text FROM hris.org_chart_nodes n JOIN hris.org_chart_versions v ON v.id = n.version_id,
			unnest(n.store_codes) c WHERE v.label = 'Draft 6'`, "33"},
		{`SELECT count(*)::text ` + nodes + ` AND n.needs_confirm`, "20"},
		{`SELECT count(*)::text ` + nodes + ` AND n.status = 'planned'`, "9"},
		{`SELECT string_agg(n.code || ' ' || n.title, ',') ` + nodes + ` AND n.parent_code IS NULL`, "EXE-001 CEO"},
		{`SELECT n.parent_code ` + nodes + ` AND n.code = 'SUP-002'`, "EXE-002"},
		{`SELECT array_to_string(n.store_codes, ';') ` + nodes + ` AND n.code = 'RTL-030'`, "PPAY;PLAY"},
		{`SELECT n.employment_type || '|' || n.vendor ` + nodes + ` AND n.code = 'HRG-012'`, "OUTSOURCE|Red Guard"},
		{`SELECT count(*)::text FROM hris.org_title_aliases`, "43"},
		{`SELECT title FROM hris.org_title_aliases WHERE alias = 'patcher staff'`, "Patcher"},
	}
	for _, c := range checks {
		if got := text(c.sql); got != c.want {
			t.Errorf("%s\n got %q, want %q", c.sql, got, c.want)
		}
	}
}

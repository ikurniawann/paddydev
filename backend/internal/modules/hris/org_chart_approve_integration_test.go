package hris

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"context"
	"errors"
	"net/http/httptest"

	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/testutil"
)

const orgChartLeaf = "hris.organization.org-chart"

// orgDraft inserts a version with the given nodes (code, parent, level) and
// returns its id. Imported now, so it is the latest import.
func (h *harness) orgDraft(label string, nodes ...[3]string) string {
	h.t.Helper()
	id := h.scalar(`INSERT INTO hris.org_chart_versions (label, status, imported_at) VALUES ($1, 'draft', now() + interval '1 minute') RETURNING id::text`, label)
	for i, n := range nodes {
		var parent *string
		if n[1] != "" {
			parent = &n[1]
		}
		h.exec(`INSERT INTO hris.org_chart_nodes (version_id, code, parent_code, sort_order, title, raw_title, is_position,
			division_code, division_name, job_level, employment_type, status)
			VALUES ($1, $2, $3, $4, 'Jabatan '||$2, 'Jabatan '||$2, true, 'EXE', 'Executive', $5, 'PERMANENT_OR_CONTRACT', 'active')`,
			id, n[0], parent, i+1, n[2])
	}
	return id
}

func (h *harness) orgVersionID(label string) string {
	return h.scalar(`SELECT id::text FROM hris.org_chart_versions WHERE label = $1`, label)
}

func (h *harness) orgStatus(label string) string {
	return h.scalar(`SELECT status FROM hris.org_chart_versions WHERE label = $1`, label)
}

func (h *harness) orgApproveAudits(id string) string {
	return h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'hris.org_chart.version.approve'
		AND entity = 'hris.org_chart_versions' AND entity_id::text = $1`, id)
}

func orgApprovePath(id string) string { return "/api/hris/org-chart/versions/" + id + "/approve" }

func TestOrgChartApprove(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staffGrants("hrd", readApprove(orgChartLeaf), false)
	reader, _ := h.staffGrants("hrd", readOnly(orgChartLeaf), false)
	d6 := h.orgVersionID("Draft 6")

	expect(t, h.do(&reader, "POST", orgApprovePath(d6), map[string]any{"valid_from": "2026-11-12"}), 403, "")
	expect(t, h.do(&hr, "POST", orgApprovePath(d6), map[string]any{"valid_from": "12-11-2026"}), 400, "Tanggal berlaku tidak valid")
	expect(t, h.do(&hr, "POST", orgApprovePath(d6), map[string]any{"valid_from": "2026-02-30"}), 400, "Tanggal berlaku tidak valid")
	expect(t, h.do(&hr, "POST", orgApprovePath("00000000-0000-4000-8000-000000000000"), nil), 404, "Versi bagan tidak ditemukan")
	expect(t, h.do(&hr, "POST", orgApprovePath("bukan-uuid"), nil), 404, "Versi bagan tidak ditemukan")
	if h.orgStatus("Draft 6") != "draft" || h.orgApproveAudits(d6) != "0" {
		t.Fatalf("rejected calls changed Draft 6")
	}

	res := h.do(&hr, "POST", orgApprovePath(d6), map[string]any{"valid_from": "2026-11-12", "reason": "Disahkan rapat HR"})
	expect(t, res, 200, "")
	d := res.data()
	if d["status"] != "approved" || d["valid_from"] != "2026-11-12" || d["approved_by_name"] == nil || d["approved_at"] == nil || d["label"] != "Draft 6" {
		t.Fatalf("approved %s", res.raw)
	}
	if h.orgApproveAudits(d6) != "1" {
		t.Fatalf("audit rows %s", h.orgApproveAudits(d6))
	}
	var before, after, reason string
	if err := h.tx.QueryRow(t.Context(), `SELECT before::text, after::text, coalesce(reason, '') FROM audit.audit_log
		WHERE action = 'hris.org_chart.version.approve' AND entity_id::text = $1`, d6).Scan(&before, &after, &reason); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(before, `"draft"`) || !strings.Contains(after, `"approved"`) || !strings.Contains(after, `2026-11-12`) || reason != "Disahkan rapat HR" {
		t.Fatalf("audit before %s after %s reason %q", before, after, reason)
	}
	vs := h.do(&hr, "GET", "/api/hris/org-chart/versions", nil)
	if !strings.Contains(vs.raw, `"status":"approved"`) || !strings.Contains(vs.raw, `"valid_from":"2026-11-12"`) {
		t.Fatalf("versions %s", vs.raw)
	}
}

// No body: valid_from defaults to today WIB (R6).
func TestOrgChartApproveDefaultDate(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staffGrants("hrd", readApprove(orgChartLeaf), false)
	res := h.do(&hr, "POST", orgApprovePath(h.orgVersionID("Draft 6")), nil)
	expect(t, res, 200, "")
	if res.data()["valid_from"] != "2026-10-05" {
		t.Fatalf("default valid_from %v", res.data()["valid_from"])
	}
}

// Review Focus 3: BR errors block the approval and change nothing.
func TestOrgChartApproveBlockedByErrors(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staffGrants("hrd", readApprove(orgChartLeaf), false)
	x := h.orgDraft("Draft X", [3]string{"X-1", "", "L0"}, [3]string{"X-2", "", "L0"})
	res := h.do(&hr, "POST", orgApprovePath(x), map[string]any{"valid_from": "2026-11-12"})
	expect(t, res, 400, "")
	if msg, _ := res.body["error"].(string); !strings.Contains(msg, "kesalahan struktur") || !strings.HasPrefix(msg, "Bagan masih punya ") {
		t.Fatalf("error %s", res.raw)
	}
	if details, _ := res.body["details"].([]any); len(details) == 0 {
		t.Fatalf("details %s", res.raw)
	}
	if h.orgStatus("Draft X") != "draft" || h.orgApproveAudits(x) != "0" {
		t.Fatalf("blocked approval changed Draft X")
	}
}

// Review Focus 2: the previous approved version is archived in the same
// transaction, and the audit names it.
func TestOrgChartApproveArchivesPrevious(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staffGrants("hrd", readApprove(orgChartLeaf), false)
	expect(t, h.do(&hr, "POST", orgApprovePath(h.orgVersionID("Draft 6")), nil), 200, "")
	d7 := h.orgDraft("Draft 7", [3]string{"D7-1", "", "L0"})
	res := h.do(&hr, "POST", orgApprovePath(d7), map[string]any{"valid_from": "2026-12-01"})
	expect(t, res, 200, "")
	if h.orgStatus("Draft 6") != "archived" || h.orgStatus("Draft 7") != "approved" {
		t.Fatalf("statuses %s / %s", h.orgStatus("Draft 6"), h.orgStatus("Draft 7"))
	}
	after := h.scalar(`SELECT after::text FROM audit.audit_log WHERE action = 'hris.org_chart.version.approve' AND entity_id::text = $1`, d7)
	var a map[string]any
	if err := json.Unmarshal([]byte(after), &a); err != nil || a["archived"] != "Draft 6" {
		t.Fatalf("audit after %s", after)
	}
	if n := h.scalar(`SELECT count(*)::text FROM hris.org_chart_versions WHERE status = 'approved'`); n != "1" {
		t.Fatalf("approved versions %s", n)
	}
	// An archived version cannot be approved again.
	expect(t, h.do(&hr, "POST", orgApprovePath(h.orgVersionID("Draft 6")), nil), 409, "Versi ini sudah tidak berstatus draft")
}

func TestOrgChartApproveTwiceConflict(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staffGrants("hrd", readApprove(orgChartLeaf), false)
	d6 := h.orgVersionID("Draft 6")
	expect(t, h.do(&hr, "POST", orgApprovePath(d6), nil), 200, "")
	expect(t, h.do(&hr, "POST", orgApprovePath(d6), nil), 409, "Versi ini sudah tidak berstatus draft")
	if h.orgApproveAudits(d6) != "1" {
		t.Fatalf("audit rows %s", h.orgApproveAudits(d6))
	}
	// The partial unique index rejects a second approved row outright.
	h.orgDraft("Draft 8", [3]string{"D8-1", "", "L0"})
	if code := pgErrCodeTx(h, `UPDATE hris.org_chart_versions SET status = 'approved' WHERE label = 'Draft 8'`); code != "23505" {
		t.Fatalf("second approved row: code %q, want 23505", code)
	}
}

// Review Focus 4: a newer draft does not replace the approved default.
func TestOrgChartDefaultVersionIsApproved(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staffGrants("hrd", readApprove(orgChartLeaf), false)
	h.orgDraft("Draft 7", [3]string{"D7-1", "", "L0"})
	// Before any approval the latest import is the default (EPIC-101).
	if l := h.do(&hr, "GET", "/api/hris/org-chart/tree", nil).data()["version"].(map[string]any)["label"]; l != "Draft 7" {
		t.Fatalf("default before approval %v", l)
	}
	expect(t, h.do(&hr, "POST", orgApprovePath(h.orgVersionID("Draft 6")), nil), 200, "")
	for _, path := range []string{"/api/hris/org-chart/tree", "/api/hris/org-chart/vacancy", "/api/hris/org-chart/gap", "/api/hris/org-chart/validation"} {
		res := h.do(&hr, "GET", path, nil)
		expect(t, res, 200, "")
		if l := res.data()["version"].(map[string]any)["label"]; l != "Draft 6" {
			t.Fatalf("%s default version %v", path, l)
		}
	}
	if l := h.do(&hr, "GET", "/api/hris/org-chart/tree?version=Draft%207", nil).data()["version"].(map[string]any)["label"]; l != "Draft 7" {
		t.Fatalf("explicit version %v", l)
	}
}

// The menu read grant still reads everything (R8).
func TestOrgChartApproveReadOnlyStillReads(t *testing.T) {
	h := newHarness(t)
	reader, _ := h.staffGrants("hrd", readOnly(orgChartLeaf), false)
	expect(t, h.do(&reader, "GET", "/api/hris/org-chart/vacancy", nil), 200, "")
}

// EPIC-104 R2 / Review Focus 1: two approvals of different drafts at the same
// time end with one approved version; the loser gets the race message and
// its version stays draft. Runs on two real committed transactions.
func TestOrgChartApproveConcurrent(t *testing.T) {
	db := testutil.DB(t)
	ctx := context.Background()
	var approved int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM hris.org_chart_versions WHERE status = 'approved'`).Scan(&approved); err != nil || approved > 0 {
		t.Skipf("test database already has an approved version (%d, %v)", approved, err)
	}
	staff := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd"})
	u := &auth.User{ID: staff.UserID, FullName: "QA Race"}
	ids := map[string]string{}
	for _, label := range []string{"QA Race A", "QA Race B"} {
		var id string
		if err := db.QueryRow(ctx, `INSERT INTO hris.org_chart_versions (label, status) VALUES ($1, 'draft') RETURNING id::text`, label).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, `INSERT INTO hris.org_chart_nodes (version_id, code, sort_order, title, raw_title, is_position,
			division_code, division_name, job_level, employment_type) VALUES ($1, 'R-1', 1, 'CEO', 'CEO', true, 'EXE', 'Executive', 'L0', 'PERMANENT_OR_CONTRACT')`, id); err != nil {
			t.Fatal(err)
		}
		ids[label] = id
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM audit.audit_log WHERE action = 'hris.org_chart.version.approve' AND entity_label LIKE 'QA Race %'`)
		_, _ = db.Exec(context.Background(), `DELETE FROM hris.org_chart_versions WHERE label LIKE 'QA Race %'`)
	})
	req := httptest.NewRequest("POST", "/api/hris/org-chart/versions/x/approve", nil)
	tx1, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx1.Rollback(ctx)
	if _, err := approveOrgVersion(ctx, tx1, req, u, ids["QA Race A"], "2026-11-12", nil); err != nil {
		t.Fatalf("approve A: %v", err)
	}
	done := make(chan error, 1)
	pid := make(chan int, 1)
	go func() {
		tx2, err := db.Begin(ctx)
		if err != nil {
			pid <- 0
			done <- err
			return
		}
		defer tx2.Rollback(ctx)
		var p int
		_ = tx2.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&p)
		pid <- p
		_, err = approveOrgVersion(ctx, tx2, req, u, ids["QA Race B"], "2026-11-12", nil)
		if err == nil {
			err = tx2.Commit(ctx)
		}
		done <- err
	}()
	// Commit only once tx2 waits on tx1's index entry, so the race is real.
	// Watch tx2's own backend, so a lock wait elsewhere (CI runs packages in
	// parallel on one database) cannot fool the probe.
	tx2pid := <-pid
	waiting := false
	for i := 0; i < 100 && !waiting && tx2pid != 0; i++ {
		var n int
		_ = db.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE pid = $1 AND wait_event_type = 'Lock'`, tx2pid).Scan(&n)
		waiting = n > 0
		if !waiting {
			time.Sleep(50 * time.Millisecond)
		}
	}
	if !waiting {
		t.Fatal("second approval never waited on the first")
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	err = <-done
	var he *httpx.Error
	if !errors.As(err, &he) || he.Status != 409 || he.Message != "Versi lain baru saja disetujui — muat ulang lalu coba lagi" {
		t.Fatalf("loser error = %v", err)
	}
	var a, b string
	_ = db.QueryRow(ctx, `SELECT status FROM hris.org_chart_versions WHERE id = $1`, ids["QA Race A"]).Scan(&a)
	_ = db.QueryRow(ctx, `SELECT status FROM hris.org_chart_versions WHERE id = $1`, ids["QA Race B"]).Scan(&b)
	if a != "approved" || b != "draft" {
		t.Fatalf("statuses A=%s B=%s", a, b)
	}
}

// pgErrCodeTx runs sql in a savepoint and returns its SQLSTATE ("" = ok).
func pgErrCodeTx(h *harness, sql string, args ...any) string {
	h.t.Helper()
	h.exec("SAVEPOINT org_probe")
	_, err := h.tx.Exec(h.t.Context(), sql, args...)
	h.exec("ROLLBACK TO SAVEPOINT org_probe")
	h.exec("RELEASE SAVEPOINT org_probe")
	return database.PgCode(err)
}

// EPIC-104 R7: the page asks which buttons the account may use.
func TestOrgChartAccess(t *testing.T) {
	h := newHarness(t)
	none, _ := h.staff("pos", nil, false)
	expect(t, h.do(&none, "GET", "/api/hris/org-chart/access", nil), 403, "")
	for _, c := range []struct {
		actions          []string
		approve, imports bool
	}{
		{[]string{"read"}, false, false},
		{[]string{"read", "approve"}, true, false},
		{[]string{"read", "approve", "import"}, true, true},
	} {
		s, _ := h.staffGrants("hrd", map[string][]string{orgChartLeaf: c.actions}, false)
		res := h.do(&s, "GET", "/api/hris/org-chart/access", nil)
		expect(t, res, 200, "")
		if res.data()["approve"] != c.approve || res.data()["import"] != c.imports {
			t.Fatalf("%v → %s", c.actions, res.raw)
		}
	}
}

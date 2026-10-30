package hris

import (
	"os"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

const teamAttendanceURL = "/api/hris/me/team-attendance"

// teamRows indexes a team-attendance response's employees by id.
func teamRows(t *testing.T, c call) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{}
	list, _ := c.data()["employees"].([]any)
	for _, r := range list {
		m := r.(map[string]any)
		id := m["employee_id"].(string)
		if _, dup := out[id]; dup {
			t.Fatalf("employee %s listed twice: %s", id, c.raw)
		}
		out[id] = m
	}
	return out
}

// EPIC-074 R1–R3: the manager sees every active employee below them (direct
// and indirect, through inactive links cut off), without selfie photos, and
// indirect members carry their direct manager's name.
func TestTeamAttendanceScope(t *testing.T) {
	h := newHarness(t)
	boss, bossID := h.staff("pos", nil, true, emp{name: "Atasan Tim"})
	coord := h.employee(emp{name: "Koordinator K", boss: &bossID})
	staff := h.employee(emp{name: "Staf S", boss: &coord})
	outsider := h.employee(emp{name: "Orang Luar"})
	gone := h.employee(emp{name: "Nonaktif N", boss: &bossID, inactive: true})
	below := h.employee(emp{name: "Bawah N2", boss: &gone})
	h.exec(`INSERT INTO hris.attendance (employee_id, date, clock_in, clock_in_photo_url, status)
		VALUES ($1, '2026-10-05', '2026-10-05 08:05+07', 'attendance-photos/x/masuk.jpg', 'present')`, staff)

	c := h.do(&boss, "GET", teamAttendanceURL+"?date=2026-10-05", nil)
	expect(t, c, 200, "")
	rows := teamRows(t, c)
	if len(rows) != 2 || rows[coord] == nil || rows[staff] == nil {
		t.Fatalf("team = %s", c.raw)
	}
	for _, id := range []string{outsider, gone, below, bossID} {
		if rows[id] != nil {
			t.Fatalf("%s must not be in the team: %s", id, c.raw)
		}
	}
	if rows[coord]["direct"] != true {
		t.Errorf("coordinator direct = %v", rows[coord]["direct"])
	}
	s := rows[staff]
	if s["direct"] != false || s["reporting_to_name"] != "Koordinator K" {
		t.Errorf("staff = %v", s)
	}
	att, _ := s["attendance"].(map[string]any)
	if att == nil || att["clock_in_photo_url"] != nil || att["clock_out_photo_url"] != nil {
		t.Errorf("staff attendance (photos must be null) = %v", s["attendance"])
	}
	if st := s["status"]; st != "hadir" && st != "terlambat" {
		t.Errorf("staff status = %v", st)
	}
	if c.data()["date"] != "2026-10-05" || c.data()["summary"] == nil {
		t.Errorf("envelope = %s", c.raw)
	}
}

// A reporting_to cycle (A→B→A) ends; the manager is never their own member.
func TestTeamAttendanceCycleAndSelf(t *testing.T) {
	h := newHarness(t)
	a, aID := h.staff("pos", nil, true, emp{name: "Siklus A"})
	b := h.employee(emp{name: "Siklus B", boss: &aID})
	h.exec(`UPDATE hris.employees SET reporting_to = $1 WHERE id = $2`, b, aID)

	c := h.do(&a, "GET", teamAttendanceURL+"?date=2026-10-05", nil)
	expect(t, c, 200, "")
	rows := teamRows(t, c)
	if len(rows) != 1 || rows[b] == nil {
		t.Fatalf("team = %s", c.raw)
	}
}

// Accounts with no team (unlinked, or linked without subordinates) get an
// empty envelope, never the whole company (review #1: an empty id list must
// not turn into "every employee"); a bad date is 400.
func TestTeamAttendanceUnlinked(t *testing.T) {
	h := newHarness(t)
	outsider := h.employee(emp{name: "Karyawan Lain"})
	admin, _ := h.staff("pos", nil, false)
	plain, _ := h.staff("pos", nil, true, emp{name: "Tanpa Bawahan"})
	for _, who := range []*struct {
		name string
		s    *testutil.Staff
	}{{"unlinked", &admin}, {"no reports", &plain}} {
		c := h.do(who.s, "GET", teamAttendanceURL+"?date=2026-10-05", nil)
		expect(t, c, 200, "")
		d := c.data()
		list, ok := d["employees"].([]any)
		if !ok || len(list) != 0 {
			t.Fatalf("%s: employees = %s", who.name, c.raw)
		}
		summary, _ := d["summary"].(map[string]any)
		shifts, _ := d["shifts"].([]any)
		if d["date"] != "2026-10-05" || summary == nil || summary["hadir"] != float64(0) || shifts == nil || len(shifts) != 0 {
			t.Fatalf("%s: envelope = %s", who.name, c.raw)
		}
	}
	_ = outsider
	expect(t, h.do(&admin, "GET", teamAttendanceURL+"?date=salah", nil), 400, "Format tanggal tidak valid")
}

// Review #3: a manager whose employee record is inactive has no team, even
// when opened by URL.
func TestTeamAttendanceInactiveManager(t *testing.T) {
	h := newHarness(t)
	boss, bossID := h.staff("pos", nil, true, emp{name: "Atasan Nonaktif"})
	h.employee(emp{name: "Bawahan Aktif", boss: &bossID})
	h.exec(`UPDATE hris.employees SET is_active = false WHERE id = $1`, bossID)
	c := h.do(&boss, "GET", teamAttendanceURL+"?date=2026-10-05", nil)
	expect(t, c, 200, "")
	if list, _ := c.data()["employees"].([]any); len(list) != 0 {
		t.Fatalf("inactive manager team = %s", c.raw)
	}
}

// Review #2: free-text attendance notes reach the manager only where the
// UI shows them, a check-in outside the geofence.
func TestTeamAttendanceNotes(t *testing.T) {
	h := newHarness(t)
	boss, bossID := h.staff("pos", nil, true, emp{name: "Atasan Catatan"})
	inside := h.employee(emp{name: "Di Area", boss: &bossID})
	outside := h.employee(emp{name: "Luar Area", boss: &bossID})
	h.exec(`INSERT INTO hris.attendance (employee_id, date, clock_in, status, notes, clock_in_geofence)
		VALUES ($1, '2026-10-05', '2026-10-05 08:00+07', 'present', 'catatan pribadi', 'valid'),
		       ($2, '2026-10-05', '2026-10-05 08:00+07', 'present', 'di gudang belakang', 'invalid')`, inside, outside)
	rows := teamRows(t, h.do(&boss, "GET", teamAttendanceURL+"?date=2026-10-05", nil))
	in, _ := rows[inside]["attendance"].(map[string]any)
	out, _ := rows[outside]["attendance"].(map[string]any)
	if in == nil || in["notes"] != nil {
		t.Errorf("inside notes = %v", in)
	}
	if out == nil || out["notes"] != "di gudang belakang" {
		t.Errorf("outside notes = %v", out)
	}
}

const teamAttendanceMenuMigration = "../../../database/migrations/deltas/20261030120000_ess_team_attendance_menu.sql"

// EPIC-074 R6: the migration adds ESS → Absensi Tim under "ess", granted
// (read) to every role; running it twice changes nothing.
func TestMigrationEssTeamAttendanceMenu(t *testing.T) {
	h := newHarness(t)
	body, err := os.ReadFile(teamAttendanceMenuMigration)
	if err != nil {
		t.Fatal(err)
	}
	h.exec(`INSERT INTO iam.menus (code, menu_name, menu_type) VALUES ('ess', 'Area Karyawan', 'group') ON CONFLICT (code) DO NOTHING`)
	h.exec(`INSERT INTO iam.roles (code, name) VALUES ('go_test_role_074', 'go_test_role_074') ON CONFLICT (code) DO NOTHING`)
	// The roles existing when the migration runs: other packages' tests may
	// commit new roles concurrently (CI runs packages in parallel).
	roles := h.scalar(`SELECT string_agg(id::text, ',') FROM iam.roles WHERE deleted_at IS NULL`)
	h.exec(string(body))
	h.exec(string(body))
	got := h.scalar(`SELECT m.menu_name || '|' || m.route_path || '|' || m.module || '|' || p.code || '|' || (m.permission_context->'actions')::text
		FROM iam.menus m JOIN iam.menus p ON p.id = m.parent_id WHERE m.code = 'ess.team-attendance'`)
	if got != `Absensi Tim|/dashboard/me/absensi-tim|ess|ess|["read"]` {
		t.Fatalf("menu = %q", got)
	}
	if n := h.scalar(`SELECT count(*)::text FROM iam.roles r WHERE r.id = ANY(string_to_array($1, ',')::uuid[]) AND NOT EXISTS (
		SELECT 1 FROM iam.role_menu_permissions rmp JOIN iam.menus m ON m.id = rmp.menu_id
		WHERE rmp.role_id = r.id AND m.code = 'ess.team-attendance' AND rmp.is_active AND rmp.granted_actions ? 'read')`, roles); n != "0" {
		t.Fatalf("%s roles without the grant", n)
	}
}

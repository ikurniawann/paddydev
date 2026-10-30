package hris

import (
	"os"
	"testing"
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

// An account without an employee gets an empty team; a bad date is 400.
func TestTeamAttendanceUnlinked(t *testing.T) {
	h := newHarness(t)
	admin, _ := h.staff("pos", nil, false)
	c := h.do(&admin, "GET", teamAttendanceURL, nil)
	expect(t, c, 200, "")
	if list, ok := c.data()["employees"].([]any); !ok || len(list) != 0 {
		t.Fatalf("unlinked = %s", c.raw)
	}
	expect(t, h.do(&admin, "GET", teamAttendanceURL+"?date=salah", nil), 400, "Format tanggal tidak valid")
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
	h.exec(string(body))
	h.exec(string(body))
	got := h.scalar(`SELECT m.menu_name || '|' || m.route_path || '|' || m.module || '|' || p.code || '|' || (m.permission_context->'actions')::text
		FROM iam.menus m JOIN iam.menus p ON p.id = m.parent_id WHERE m.code = 'ess.team-attendance'`)
	if got != `Absensi Tim|/dashboard/me/absensi-tim|ess|ess|["read"]` {
		t.Fatalf("menu = %q", got)
	}
	if n := h.scalar(`SELECT count(*)::text FROM iam.roles r WHERE r.deleted_at IS NULL AND NOT EXISTS (
		SELECT 1 FROM iam.role_menu_permissions rmp JOIN iam.menus m ON m.id = rmp.menu_id
		WHERE rmp.role_id = r.id AND m.code = 'ess.team-attendance' AND rmp.is_active AND rmp.granted_actions ? 'read')`); n != "0" {
		t.Fatalf("%s roles without the grant", n)
	}
}

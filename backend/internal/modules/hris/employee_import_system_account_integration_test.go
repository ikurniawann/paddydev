package hris

import (
	"net/http"
	"testing"

	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/testutil"
)

const reactivateSystemMsg = "Baris 2: Akun Super Admin/Admin hanya bisa diaktifkan kembali oleh Super Admin"

// importReleaseFixture: a resigned employee whose banned login is userID,
// and an add-row file reusing that employee's email (so the import releases
// the email and reactivates the login).
func importReleaseFixture(t *testing.T, h *harness, userID, email string) []byte {
	t.Helper()
	h.exec(`INSERT INTO configuration.branches (company_id, name, code)
		SELECT id, 'Cabang Lepas', 'CL'||substr(md5(random()::text),1,4) FROM configuration.companies
		WHERE NOT EXISTS (SELECT 1 FROM configuration.branches WHERE name = 'Cabang Lepas') LIMIT 1`)
	h.exec(`INSERT INTO hris.employment_statuses (code, name, is_active) VALUES ('contract', 'Kontrak (PKWT)', true) ON CONFLICT (code) DO NOTHING`)
	h.exec(`INSERT INTO hris.employees (full_name, nip, email, phone, join_date, user_id, is_access_app, is_active)
		VALUES ('Sudah Resign', $1, $2, '08', '2025-01-01', $3, false, false)`, "RSG-"+testutil.RandomHex(4), email, userID)
	h.exec(`UPDATE auth.users SET banned_until = now() + interval '100 years' WHERE id = $1`, userID)
	h.exec(`UPDATE configuration.users SET status = 'inactive' WHERE id = $1`, userID)
	header := []string{"NIP", "Nama Lengkap *", "Email *", "Tanggal Masuk *", "Status Kepegawaian *", "Tipe Jadwal *", "Cabang Utama *"}
	return employeeFile(t, header, []string{"", "Masuk Lagi", email, "21/10/2026", "Kontrak (PKWT)", "Shift", "Cabang Lepas"})
}

func loginState(h *harness, userID string) string {
	return h.scalar(`SELECT (a.banned_until IS NULL)::text || '|' || u.status || '|' ||
		  (SELECT count(*) FROM hris.employees WHERE user_id = a.id AND is_active)::text
		FROM auth.users a JOIN configuration.users u ON u.id = a.id WHERE a.id = $1`, userID)
}

// TestEmployeeImportReactivationSystemAccount: an import that releases a
// resigned employee's email re-links and unbans their login; for a login
// holding a system role (super_admin/admin, app or IAM role) that needs an
// IAM super_admin caller, else the row fails and nothing is written.
func TestEmployeeImportReactivationSystemAccount(t *testing.T) {
	superLogin := testutil.CreateStaff(t, testutil.StaffOptions{Role: "super_admin"})
	adminLogin := testutil.CreateStaff(t, testutil.StaffOptions{Role: "admin"})
	plainLogin := testutil.CreateStaff(t, testutil.StaffOptions{Role: "pos"})
	h := newHarness(t)
	hr, _ := h.staff("hrd", iam.HrisKepegawaian, false)

	for _, login := range []testutil.Staff{superLogin, adminLogin} {
		email := "lepas." + testutil.RandomHex(4) + "@test.id"
		file := importReleaseFixture(t, h, login.UserID, email)
		before := loginState(h, login.UserID)
		c := uploadEmployees(t, h, &hr, map[string]string{"mode": "add", "step": "commit", "release_emails": "true"}, file)
		expect(t, c, http.StatusForbidden, reactivateSystemMsg)
		if got := loginState(h, login.UserID); got != before {
			t.Fatalf("%s: refused import changed the login: %s → %s", login.Role, before, got)
		}
		if n := h.scalar(`SELECT count(*)::text FROM hris.employees WHERE email = $1`, email); n != "1" {
			t.Fatalf("%s: refused import wrote employees: %s", login.Role, n)
		}
	}

	// A plain account is reactivated as before.
	email := "lepas." + testutil.RandomHex(4) + "@test.id"
	file := importReleaseFixture(t, h, plainLogin.UserID, email)
	c := uploadEmployees(t, h, &hr, map[string]string{"mode": "add", "step": "commit", "release_emails": "true"}, file)
	expect(t, c, http.StatusOK, "")
	if got := loginState(h, plainLogin.UserID); got != "true|active|1" {
		t.Fatalf("plain login after import = %s", got)
	}
}

// TestEmployeeImportReactivationBySuperAdmin: IAM super_admin may reactivate
// a system account through the import.
func TestEmployeeImportReactivationBySuperAdmin(t *testing.T) {
	testutil.RequireSeededIAM(t, testutil.IAMBaseline{GrantedRoles: []string{"super_admin"}, Menus: iam.HrisKepegawaian})
	adminLogin := testutil.CreateStaff(t, testutil.StaffOptions{Role: "admin"})
	caller := testutil.CreateStaff(t, testutil.StaffOptions{Role: "super_admin"})
	h := newHarness(t)
	h.staff("hrd", nil, false) // rolls the fixture transaction back before the accounts go
	email := "lepas." + testutil.RandomHex(4) + "@test.id"
	file := importReleaseFixture(t, h, adminLogin.UserID, email)
	c := uploadEmployees(t, h, &caller, map[string]string{"mode": "add", "step": "commit", "release_emails": "true"}, file)
	expect(t, c, http.StatusOK, "")
	if got := loginState(h, adminLogin.UserID); got != "true|active|1" {
		t.Fatalf("admin login after super_admin import = %s", got)
	}
}

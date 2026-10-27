package accounts_test

import (
	"context"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

// Manajemen User menghormati aksi menu (akses per akun): baca + ubah boleh
// mengubah email login karyawan dan reset password, tetapi tidak membuat akun.
func TestSettingsUsersUpdateWithoutCreate(t *testing.T) {
	// Dibuat sebelum setup: cleanup LIFO menggulung balik transaksi dulu, lalu
	// menghapus akun ini (urutan sebaliknya saling mengunci lewat FK user_id).
	target := testutil.CreateStaff(t, testutil.StaffOptions{Role: "pos"})
	e := setup(t, map[string][]string{"settings.users": {"read", "update"}})
	var linked string
	if err := e.tx.QueryRow(context.Background(), `INSERT INTO hris.employees (full_name, nip, email, phone, join_date, user_id, is_access_app)
		VALUES ('Karyawan Login', $1, $2, '08', '2026-01-01', $3, true) RETURNING id::text`,
		"ACT-"+uniqueEmail()[:8], target.Email, target.UserID).Scan(&linked); err != nil {
		t.Fatal(err)
	}

	code, body, _ := e.do("POST", "/api/users", employeeBody(uniqueEmail()))
	wantError(t, code, body, 403, "Insufficient permissions")
	code, body, _ = e.do("GET", "/api/users/email-owner?email=a@b.c", nil)
	wantError(t, code, body, 403, "Insufficient permissions")
	code, body, _ = e.do("POST", "/api/admin/users", map[string]any{"email": uniqueEmail()})
	wantError(t, code, body, 403, "Insufficient permissions")

	fresh := uniqueEmail()
	if code, _, raw := e.do("PUT", "/api/users/"+linked, map[string]any{"email": fresh}); code != 200 {
		t.Fatalf("PUT login email with read+update = %d %s", code, raw)
	}
	var loginEmail string
	if err := e.tx.QueryRow(context.Background(), `SELECT email FROM auth.users WHERE id = $1`, target.UserID).Scan(&loginEmail); err != nil {
		t.Fatal(err)
	}
	if loginEmail != fresh {
		t.Fatalf("login email = %q, want %q", loginEmail, fresh)
	}
	if code, _, raw := e.do("POST", "/api/users/"+linked+"/reset-password", nil); code != 200 {
		t.Fatalf("reset password with update = %d %s", code, raw)
	}
}

// Baca saja di Manajemen User: daftar boleh, tulis apa pun tidak.
func TestSettingsUsersReadOnlyWritesNothing(t *testing.T) {
	target := testutil.CreateStaff(t, testutil.StaffOptions{Role: "pos"})
	ro := setup(t, map[string][]string{"settings.users": {"read"}})
	var linked string
	if err := ro.tx.QueryRow(context.Background(), `INSERT INTO hris.employees (full_name, nip, email, phone, join_date, user_id, is_access_app)
		VALUES ('Karyawan Login', $1, $2, '08', '2026-01-01', $3, true) RETURNING id::text`,
		"ACR-"+uniqueEmail()[:8], target.Email, target.UserID).Scan(&linked); err != nil {
		t.Fatal(err)
	}
	if code, body, _ := ro.do("GET", "/api/users", nil); code != 200 {
		t.Fatalf("read-only GET /api/users = %d %v", code, body)
	}
	for _, c := range []struct{ method, path string }{
		{"PUT", "/api/users/" + linked}, {"POST", "/api/users/" + linked + "/reset-password"},
		{"PATCH", "/api/admin/users/" + target.UserID}, {"POST", "/api/admin/users/" + target.UserID + "/reset-password"},
	} {
		code, body, _ := ro.do(c.method, c.path, map[string]any{"email": uniqueEmail()})
		wantError(t, code, body, 403, "Insufficient permissions")
	}
}

// Buat Akun Login di profil karyawan memakai PUT (aksi "update"): pemegang
// baca + ubah (hr_user_admin) bisa memberi akun login karyawan yang belum
// punya akun, meski tidak bisa menambah karyawan baru.
func TestSettingsUsersUpdateProvisionsLogin(t *testing.T) {
	e := setup(t, map[string][]string{"settings.users": {"read", "update"}})
	var id string
	if err := e.tx.QueryRow(context.Background(), `INSERT INTO hris.employees (full_name, nip, email, phone, join_date)
		VALUES ('Karyawan Baru', $1, $2, '08', '2026-01-01') RETURNING id::text`,
		"ACP-"+uniqueEmail()[:8], uniqueEmail()).Scan(&id); err != nil {
		t.Fatal(err)
	}
	code, body, raw := e.do("PUT", "/api/users/"+id, map[string]any{
		"is_access_app": true, "password": "rahasia123", "role": "employee",
		"business_scope": "holding", "holding_id": e.holding(),
	})
	if code != 200 || body["data"].(map[string]any)["appAccount"] == nil {
		t.Fatalf("Buat Akun Login with read+update = %d %s", code, raw)
	}
}

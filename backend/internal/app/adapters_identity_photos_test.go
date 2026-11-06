package app

import (
	"context"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

// EPIC-087 R4: foto profil akun disalin ke hris.employees.photo_url tanpa
// menimpa foto yang diisi HR (Review Focus 5).
func TestEmployeePhotosAdapter(t *testing.T) {
	deps := testutil.Deps(t, nil)
	ctx := context.Background()
	a := identityEmployeePhotos{db: deps.DB}
	staff := testutil.CreateStaff(t, testutil.StaffOptions{Role: "employee", FullName: "Foto Uji"})
	var emp string
	if err := deps.DB.QueryRow(ctx, `INSERT INTO hris.employees (full_name, nip, email, phone, join_date, employment_status, user_id)
		VALUES ('Foto Uji', 'FOTO-'||$2, $2||'@foto.test', '0800'||$2, '2025-01-01', 'permanent', $1) RETURNING id::text`,
		staff.UserID, testutil.RandomHex(4)).Scan(&emp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = deps.DB.Exec(ctx, `DELETE FROM hris.employees WHERE id = $1`, emp) })
	photo := func() string {
		var v *string
		if err := deps.DB.QueryRow(ctx, `SELECT photo_url FROM hris.employees WHERE id = $1`, emp).Scan(&v); err != nil {
			t.Fatal(err)
		}
		if v == nil {
			return "<null>"
		}
		return *v
	}
	s := func(v string) *string { return &v }

	if err := a.SetForUser(ctx, staff.UserID, s("/api/files/profile-photos/a.jpg"), nil); err != nil || photo() != "/api/files/profile-photos/a.jpg" {
		t.Fatalf("kosong → terisi: %v %s", err, photo())
	}
	if err := a.SetForUser(ctx, staff.UserID, s("/api/files/profile-photos/b.jpg"), s("/api/files/profile-photos/a.jpg")); err != nil || photo() != "/api/files/profile-photos/b.jpg" {
		t.Fatalf("ganti: %v %s", err, photo())
	}
	if err := a.SetForUser(ctx, staff.UserID, nil, s("/api/files/profile-photos/b.jpg")); err != nil || photo() != "<null>" {
		t.Fatalf("hapus: %v %s", err, photo())
	}
	if _, err := deps.DB.Exec(ctx, `UPDATE hris.employees SET photo_url = 'hr.jpg' WHERE id = $1`, emp); err != nil {
		t.Fatal(err)
	}
	if err := a.SetForUser(ctx, staff.UserID, s("/api/files/profile-photos/c.jpg"), nil); err != nil || photo() != "hr.jpg" {
		t.Fatalf("foto HR tertimpa: %v %s", err, photo())
	}
	if err := a.SetForUser(ctx, staff.UserID, nil, s("/api/files/profile-photos/c.jpg")); err != nil || photo() != "hr.jpg" {
		t.Fatalf("foto HR terhapus: %v %s", err, photo())
	}
}

package hris

import (
	"net/http"
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/testutil"
)

// The harness clock is fixedNow = Monday 2026-10-05 10:00 WIB, so "today" is
// 2026-10-05. Employees resign on 2026-10-02 and are reactivated on
// 2026-10-03 (the day after, in the past); 2026-10-06 is "in the future".

const (
	rehireResignDate = "2026-10-02"
	rehireDate       = "2026-10-03"
	rehireToday      = "2026-10-05"
)

func rehirePath(id string) string { return "/api/hris/employees/" + id + "/rehire" }

func rehireBody(branch string, extra map[string]any) map[string]any {
	b := map[string]any{"effective_date": rehireDate, "employment_status": "contract", "branch_id": branch}
	for k, v := range extra {
		b[k] = v
	}
	return b
}

// resignedEmployee builds an employee with a primary placement and a weekly
// pattern, then resigns them through the API. userID != nil links a login.
func resignedEmployee(t *testing.T, h *harness, hr *testutil.Staff, branch string, userID *string, extra emp) string {
	t.Helper()
	extra.user = userID
	if extra.join == "" {
		extra.join = "2025-01-10"
	}
	e := h.employee(extra)
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date) VALUES ($1, $2, true, '2025-01-10')`, e, branch)
	def := h.scalar(`SELECT id::text FROM hris.shifts WHERE code = 'DEF'`)
	h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from) VALUES ($1, 1, $2, '2025-01-10')`, e, def)
	if userID != nil {
		h.exec(`UPDATE hris.employees SET is_access_app = true WHERE id = $1`, e)
	}
	expect(t, h.do(hr, http.MethodPost, "/api/hris/employees/"+e+"/resign",
		map[string]any{"effective_date": rehireResignDate, "resign_type": "resign"}), http.StatusOK, "")
	return e
}

func TestRehireEmployee(t *testing.T) {
	// Accounts are created before the harness so the fixture transaction is
	// rolled back (cleanup runs last-in first-out) before they are deleted.
	login := testutil.CreateStaff(t, testutil.StaffOptions{Role: "pos"})
	h := newHarness(t)
	hr, _ := h.staff("hrd", iam.HrisKepegawaian, false)
	nobody, _ := h.staff("pos", nil, false)
	branch := h.scalar(`SELECT id::text FROM configuration.branches LIMIT 1`)
	e := resignedEmployee(t, h, &hr, branch, &login.UserID, emp{name: "Rina Kembali", join: "2025-01-10"})
	path := rehirePath(e)

	// 10. Tanpa hak ubah karyawan → 403.
	expect(t, h.do(&nobody, http.MethodPost, path, rehireBody(branch, nil)), http.StatusForbidden, "")

	// 1. Karyawan yang masih aktif tidak bisa "diaktifkan kembali".
	other := h.employee(emp{name: "Masih Aktif"})
	expect(t, h.do(&hr, http.MethodPost, rehirePath(other), rehireBody(branch, nil)), http.StatusBadRequest, "Karyawan masih aktif")
	expect(t, h.do(&hr, http.MethodPost, rehirePath("00000000-0000-4000-8000-000000000000"), rehireBody(branch, nil)),
		http.StatusNotFound, "Karyawan tidak ditemukan")

	// 2. Tanggal efektif harus setelah tanggal keluar.
	expect(t, h.do(&hr, http.MethodPost, path, rehireBody(branch, map[string]any{"effective_date": rehireResignDate})),
		http.StatusBadRequest, "Tanggal efektif harus setelah tanggal keluar (02/10/2026)")
	expect(t, h.do(&hr, http.MethodPost, path, rehireBody(branch, map[string]any{"effective_date": "2026-09-30"})),
		http.StatusBadRequest, "Tanggal efektif harus setelah tanggal keluar (02/10/2026)")
	// 3. Tidak boleh setelah hari ini; wajib diisi.
	expect(t, h.do(&hr, http.MethodPost, path, rehireBody(branch, map[string]any{"effective_date": "2026-10-06"})),
		http.StatusBadRequest, "Tanggal efektif tidak boleh setelah hari ini")
	expect(t, h.do(&hr, http.MethodPost, path, rehireBody(branch, map[string]any{"effective_date": ""})),
		http.StatusBadRequest, "Tanggal efektif wajib diisi")
	// 4. Status resigned/terminated/tidak dikenal ditolak.
	for _, status := range []string{"resigned", "terminated", "bukan-status"} {
		expect(t, h.do(&hr, http.MethodPost, path, rehireBody(branch, map[string]any{"employment_status": status})),
			http.StatusBadRequest, "Status kepegawaian tidak valid untuk karyawan aktif")
	}
	expect(t, h.do(&hr, http.MethodPost, path, map[string]any{"effective_date": rehireDate, "branch_id": branch}),
		http.StatusBadRequest, "Status kepegawaian wajib diisi")
	// 5. Cabang wajib dan harus ada.
	noBranch := rehireBody(branch, nil)
	delete(noBranch, "branch_id")
	expect(t, h.do(&hr, http.MethodPost, path, noBranch), http.StatusBadRequest, "Cabang penempatan wajib diisi")
	expect(t, h.do(&hr, http.MethodPost, path, rehireBody("00000000-0000-4000-8000-000000000000", nil)),
		http.StatusBadRequest, "Cabang tidak ditemukan")
	if h.scalar(`SELECT is_active::text FROM hris.employees WHERE id = $1`, e) != "false" {
		t.Fatal("a rejected rehire must not change the employee")
	}

	// 6. Sukses.
	c := h.do(&hr, http.MethodPost, path, rehireBody(branch, map[string]any{"reason": "Kembali bekerja"}))
	expect(t, c, http.StatusOK, "")
	if c.body["message"] != "Karyawan diaktifkan kembali" {
		t.Fatalf("message = %v", c.body["message"])
	}
	d := c.data()
	if d["id"] != e || d["employment_status"] != "contract" || d["join_date"] != rehireDate {
		t.Fatalf("data = %v", d)
	}
	if note, _ := d["schedule_note"].(string); note != "Atur jadwal shift karyawan ini di Jadwal Kerja" {
		t.Fatalf("schedule_note = %q", note)
	}
	row := h.scalar(`SELECT is_active || '|' || (end_date IS NULL) || '|' || employment_status || '|' || join_date::text || '|' || is_access_app
		FROM hris.employees WHERE id = $1`, e)
	if row != "true|true|contract|2026-10-03|true" {
		t.Fatalf("employee = %s", row)
	}
	if got := h.scalar(`SELECT string_agg(start_date::text || '/' || coalesce(end_date::text, '') || '/' || is_primary, ',' ORDER BY start_date)
		FROM hris.employee_placements WHERE employee_id = $1`, e); got != "2025-01-10/2026-10-02/true,2026-10-03//true" {
		t.Fatalf("placements = %s", got)
	}
	if h.scalar(`SELECT branch_id::text || '|' || notes FROM hris.employee_placements WHERE employee_id = $1 AND end_date IS NULL`, e) != branch+"|Aktif kembali" {
		t.Fatal("new placement must be at the chosen branch with the note")
	}
	if loginState(h, login.UserID) != "true|active|1" {
		t.Fatalf("login = %s", loginState(h, login.UserID))
	}
	if h.scalar(`SELECT count(*)::text FROM hris.employee_shifts WHERE employee_id = $1 AND effective_from > '2026-10-02'`, e) != "0" {
		t.Fatal("a Shift employee must not get a pattern")
	}
	if got := h.scalar(`SELECT change_type || '|' || prev_employment_status || '|' || new_employment_status || '|' || effective_date::text || '|' ||
		coalesce(reason, '') || '|' || coalesce(notes, '') FROM hris.employment_history WHERE employee_id = $1 AND change_type = 'rehire'`, e); got !=
		"rehire|resigned|contract|2026-10-03|Kembali bekerja|Tanggal masuk sebelumnya: 10/01/2025" {
		t.Fatalf("history = %s", got)
	}
	if got := h.scalar(`SELECT concat_ws('|', (before::jsonb)->>'join_date', (before::jsonb)->>'is_active', (after::jsonb)->>'join_date', (after::jsonb)->>'branch_id')
		FROM audit.audit_log WHERE entity_id = $1 AND action = 'employee.rehire'`, e); got != "2025-01-10|false|2026-10-03|"+branch {
		t.Fatalf("audit = %s", got)
	}
	// Sudah aktif sekarang.
	expect(t, h.do(&hr, http.MethodPost, path, rehireBody(branch, nil)), http.StatusBadRequest, "Karyawan masih aktif")
}

// TestRehireOptions: reactivate_login=false, the email rules (R7) and the
// Non-Shift fixed schedule (R5).
func TestRehireOptions(t *testing.T) {
	login := testutil.CreateStaff(t, testutil.StaffOptions{Role: "pos"})
	login2 := testutil.CreateStaff(t, testutil.StaffOptions{Role: "pos"})
	h := newHarness(t)
	hr, _ := h.staff("hrd", iam.HrisKepegawaian, false)
	branch := h.scalar(`SELECT id::text FROM configuration.branches LIMIT 1`)

	// 7. reactivate_login:false → karyawan aktif, login tetap terblokir.
	e := resignedEmployee(t, h, &hr, branch, &login.UserID, emp{name: "Tanpa Login Aktif"})
	h.exec(`UPDATE auth.users SET banned_until = now() + interval '100 years' WHERE id = $1`, login.UserID)
	h.exec(`UPDATE configuration.users SET status = 'inactive' WHERE id = $1`, login.UserID)
	expect(t, h.do(&hr, http.MethodPost, rehirePath(e), rehireBody(branch, map[string]any{"reactivate_login": false})), http.StatusOK, "")
	if got := h.scalar(`SELECT is_active || '|' || is_access_app FROM hris.employees WHERE id = $1`, e); got != "true|false" {
		t.Fatalf("employee = %s", got)
	}
	if got := loginState(h, login.UserID); got != "false|inactive|1" {
		t.Fatalf("login must stay blocked, got %s", got)
	}

	// 8. Email: karyawan dengan login tidak boleh ganti email di sini.
	withLogin := resignedEmployee(t, h, &hr, branch, &login2.UserID, emp{name: "Punya Login"})
	expect(t, h.do(&hr, http.MethodPost, rehirePath(withLogin), rehireBody(branch, map[string]any{"email": "baru." + testutil.RandomHex(4) + "@hris.test"})),
		http.StatusBadRequest, "Email akun login diubah lewat Manajemen User")

	// Karyawan tanpa login: email baru tersimpan; email milik karyawan lain / akun login ditolak.
	noLogin := resignedEmployee(t, h, &hr, branch, nil, emp{name: "Tanpa Akun"})
	taken := h.scalar(`SELECT email FROM hris.employees WHERE id = $1`, withLogin)
	expect(t, h.do(&hr, http.MethodPost, rehirePath(noLogin), rehireBody(branch, map[string]any{"email": taken})),
		http.StatusBadRequest, "Email sudah digunakan")
	// Email dibandingkan tanpa membedakan huruf besar/kecil.
	expect(t, h.do(&hr, http.MethodPost, rehirePath(noLogin), rehireBody(branch, map[string]any{"email": strings.ToUpper(taken)})),
		http.StatusBadRequest, "Email sudah digunakan")
	authEmail := h.scalar(`SELECT email FROM auth.users WHERE id = $1`, login.UserID)
	expect(t, h.do(&hr, http.MethodPost, rehirePath(noLogin), rehireBody(branch, map[string]any{"email": authEmail})),
		http.StatusBadRequest, "Email sudah digunakan")
	expect(t, h.do(&hr, http.MethodPost, rehirePath(noLogin), rehireBody(branch, map[string]any{"email": "bukan-email"})), http.StatusBadRequest, "")
	if h.scalar(`SELECT is_active::text FROM hris.employees WHERE id = $1`, noLogin) != "false" {
		t.Fatal("rejected email must not reactivate")
	}
	fresh := "kembali." + testutil.RandomHex(4) + "@hris.test"
	expect(t, h.do(&hr, http.MethodPost, rehirePath(noLogin), rehireBody(branch, map[string]any{"email": fresh})), http.StatusOK, "")
	if got := h.scalar(`SELECT email || '|' || coalesce(user_id::text, '-') || '|' || is_access_app FROM hris.employees WHERE id = $1`, noLogin); got != fresh+"|-|false" {
		t.Fatalf("employee = %s", got)
	}

	// Nonaktif tanpa tanggal keluar: tanggal efektif harus setelah tanggal masuk lama.
	noEnd := h.employee(emp{name: "Tanpa Tanggal Keluar", join: "2026-09-20", inactive: true})
	expect(t, h.do(&hr, http.MethodPost, rehirePath(noEnd), rehireBody(branch, map[string]any{"effective_date": "2026-09-20"})),
		http.StatusBadRequest, "Tanggal efektif harus setelah tanggal masuk sebelumnya (20/09/2026)")
	expect(t, h.do(&hr, http.MethodPost, rehirePath(noEnd), rehireBody(branch, map[string]any{"effective_date": "2026-09-21"})), http.StatusOK, "")

	// 9. Non-Shift: pola tetap mulai max(tanggal efektif, hari ini).
	ns := resignedEmployee(t, h, &hr, branch, nil, emp{name: "Karyawan NS"})
	h.exec(`UPDATE hris.employees SET schedule_type = 'non_shift' WHERE id = $1`, ns)
	c := h.do(&hr, http.MethodPost, rehirePath(ns), rehireBody(branch, nil))
	expect(t, c, http.StatusOK, "")
	if note, _ := c.data()["schedule_note"].(string); note != "" {
		t.Fatalf("Non-Shift must have no schedule_note, got %q", note)
	}
	if got := h.scalar(`SELECT min(effective_from)::text || '|' || count(*)::text || '|' || min(s.code)
		FROM hris.employee_shifts es JOIN hris.shifts s ON s.id = es.shift_id
		WHERE es.employee_id = $1 AND es.effective_from > $2::date`, ns, rehireResignDate); got != rehireToday+"|5|NS" {
		t.Fatalf("fixed pattern = %s", got)
	}
}

// TestRehireSystemAccountNeedsSuperAdmin: a login holding a system role is
// reactivated by an IAM super_admin only (R6); the refusal changes nothing.
func TestRehireSystemAccountNeedsSuperAdmin(t *testing.T) {
	testutil.RequireSeededIAM(t, testutil.IAMBaseline{GrantedRoles: []string{"super_admin"}, Menus: iam.HrisKepegawaian})
	adminLogin := testutil.CreateStaff(t, testutil.StaffOptions{Role: "admin"})
	caller := testutil.CreateStaff(t, testutil.StaffOptions{Role: "super_admin"})
	h := newHarness(t)
	hr, _ := h.staff("hrd", iam.HrisKepegawaian, false)
	branch := h.scalar(`SELECT id::text FROM configuration.branches LIMIT 1`)
	e := resignedEmployee(t, h, &hr, branch, &adminLogin.UserID, emp{name: "Admin Kembali"})
	h.exec(`UPDATE auth.users SET banned_until = now() + interval '100 years' WHERE id = $1`, adminLogin.UserID)
	h.exec(`UPDATE configuration.users SET status = 'inactive' WHERE id = $1`, adminLogin.UserID)

	expect(t, h.do(&hr, http.MethodPost, rehirePath(e), rehireBody(branch, nil)), http.StatusForbidden,
		"Akun Super Admin/Admin hanya bisa diaktifkan kembali oleh Super Admin")
	if got := loginState(h, adminLogin.UserID); got != "false|inactive|0" {
		t.Fatalf("refused rehire changed the login: %s", got)
	}
	if h.scalar(`SELECT is_active::text FROM hris.employees WHERE id = $1`, e) != "false" {
		t.Fatal("refused rehire must leave the employee inactive")
	}
	expect(t, h.do(&caller, http.MethodPost, rehirePath(e), rehireBody(branch, nil)), http.StatusOK, "")
	if got := loginState(h, adminLogin.UserID); got != "true|active|1" {
		t.Fatalf("login after super_admin rehire = %s", got)
	}
}

// TestRehireSystemAccountWithoutLogin: HR may rehire a system-account holder
// when the login stays blocked (the guard only applies to restoring it).
func TestRehireSystemAccountWithoutLogin(t *testing.T) {
	testutil.RequireSeededIAM(t, testutil.IAMBaseline{GrantedRoles: []string{"super_admin"}, Menus: iam.HrisKepegawaian})
	adminLogin := testutil.CreateStaff(t, testutil.StaffOptions{Role: "admin"})
	h := newHarness(t)
	hr, _ := h.staff("hrd", iam.HrisKepegawaian, false)
	branch := h.scalar(`SELECT id::text FROM configuration.branches LIMIT 1`)
	e := resignedEmployee(t, h, &hr, branch, &adminLogin.UserID, emp{name: "Admin Tanpa Login"})
	h.exec(`UPDATE auth.users SET banned_until = now() + interval '100 years' WHERE id = $1`, adminLogin.UserID)
	h.exec(`UPDATE configuration.users SET status = 'inactive' WHERE id = $1`, adminLogin.UserID)

	expect(t, h.do(&hr, http.MethodPost, rehirePath(e), rehireBody(branch, map[string]any{"reactivate_login": false})), http.StatusOK, "")
	if got := h.scalar(`SELECT is_active || '|' || is_access_app FROM hris.employees WHERE id = $1`, e); got != "true|false" {
		t.Fatalf("employee = %s", got)
	}
	if got := loginState(h, adminLogin.UserID); got != "false|inactive|1" {
		t.Fatalf("login must stay blocked and inactive, got %s", got)
	}
}

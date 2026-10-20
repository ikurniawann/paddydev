package hris

import (
	"slices"
	"strings"
	"testing"
)

const (
	menuKepegawaian = "hris.kepegawaian"
	menuWorkforce   = "hris.workforce"
)

func TestEmployeeGuards(t *testing.T) {
	h := newHarness(t)
	expect(t, h.do(nil, "GET", "/api/hris/employees", nil), 401, "Authentication required")
	nobody, _ := h.staff("pos", nil, false)
	expect(t, h.do(&nobody, "GET", "/api/hris/employees", nil), 403, "Insufficient permissions")

	// ESS: an employee reads their own record, not someone else's.
	self, own := h.staff("pos", nil, true)
	other := h.employee(emp{})
	expect(t, h.do(&self, "GET", "/api/hris/employees/"+own, nil), 200, "")
	expect(t, h.do(&self, "GET", "/api/hris/employees/"+other, nil), 403, "Insufficient permissions")
	expect(t, h.do(&self, "PUT", "/api/hris/employees/"+own, map[string]any{"full_name": "X"}), 403, "")
}

// TestLinkedEmployeeEmailNeedsSettingsUsers: HR without Settings → Manajemen User may edit
// employees, but not the email of one linked to a login account (the login email).
func TestLinkedEmployeeEmailNeedsSettingsUsers(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuKepegawaian}, false)
	_, linked := h.staff("pos", nil, true)
	plain := h.employee(emp{})

	expect(t, h.do(&hr, "PUT", "/api/hris/employees/"+linked, map[string]any{"email": "ganti." + linked[:8] + "@hris.test"}),
		403, "Email karyawan yang punya akun login hanya bisa diubah lewat Settings → Manajemen User")
	expect(t, h.do(&hr, "PUT", "/api/hris/employees/"+linked, map[string]any{"phone": "081200000002"}), 200, "")
	expect(t, h.do(&hr, "PUT", "/api/hris/employees/"+plain, map[string]any{"email": "baru." + plain[:8] + "@hris.test"}), 200, "")
}

func TestEmployeeDirectoryAndCRUD(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuKepegawaian}, false)
	h.grantDelete(hr, "hris.kepegawaian.users")
	dept := h.department("Bar Test")
	boss := h.employee(emp{name: "Bos Test", dept: &dept})

	created := h.do(&hr, "POST", "/api/hris/employees", map[string]any{
		"full_name": "  Budi Directory ", "email": "budi.dir." + boss[:6] + "@hris.test", "join_date": "2026-03-01",
		"employment_status": "probation", "department_id": dept, "reporting_to": boss, "phone": nil,
		"is_active": false, "user_id": "00000000-0000-0000-0000-000000000001",
	})
	expect(t, created, 200, "")
	if created.body["message"] != "Karyawan berhasil ditambahkan" {
		t.Fatalf("message %v", created.body["message"])
	}
	d := created.data()
	if d["full_name"] != "Budi Directory" || d["phone"] != "" || d["is_active"] != true || d["user_id"] != nil {
		t.Fatalf("allowlist/trim/defaults broken: %v", d)
	}
	if nip, _ := d["nip"].(string); !strings.HasPrefix(nip, "EMP-2026-") {
		t.Fatalf("auto NIP = %v", d["nip"])
	}
	if d["join_date"] != "2026-03-01T00:00:00.000Z" {
		t.Fatalf("date column must render as a JS Date: %v", d["join_date"])
	}
	id := d["id"].(string)

	dup := h.do(&hr, "POST", "/api/hris/employees", map[string]any{
		"full_name": "Dup", "email": d["email"], "join_date": "2026-03-01", "employment_status": "probation"})
	expect(t, dup, 400, "Email sudah digunakan")
	missing := h.do(&hr, "POST", "/api/hris/employees", map[string]any{"email": "x@y.test"})
	expect(t, missing, 400, "Field yang wajib diisi: nama lengkap, email, tanggal bergabung, status karyawan")
	if _, ok := missing.body["details"].([]any); !ok {
		t.Fatalf("details missing: %s", missing.raw)
	}

	list := h.do(&hr, "GET", "/api/hris/employees?search=(Directory)&department_id="+dept, nil)
	expect(t, list, 200, "")
	if list.body["total"] != 1.0 || list.body["page"] != 1.0 || list.body["per_page"] != 20.0 {
		t.Fatalf("directory = %s", list.raw)
	}
	row := list.list()[0].(map[string]any)
	if _, leaks := row["bank_account"]; leaks {
		t.Fatal("directory leaks personal columns")
	}
	if dep := row["department"].(map[string]any); dep["name"] != "Bar Test" {
		t.Fatalf("department embed = %v", row["department"])
	}
	if m, _ := row["manager"].(map[string]any); m["full_name"] != "Bos Test" {
		t.Fatalf("manager embed = %v", row["manager"])
	}
	if got := keys(t, list.raw, "data")[:3]; !slices.Equal(got, []string{"id", "user_id", "full_name"}) {
		t.Fatalf("directory key order %v", got)
	}

	detail := h.do(&hr, "GET", "/api/hris/employees/"+id, nil)
	expect(t, detail, 200, "")
	if m := detail.data()["manager"].(map[string]any); m["full_name"] != "Bos Test" {
		t.Fatalf("manager = %v", detail.data()["manager"])
	}
	bossDetail := h.do(&hr, "GET", "/api/hris/employees/"+boss, nil)
	if reports, _ := bossDetail.data()["direct_reports"].([]any); len(reports) != 1 || reports[0].(map[string]any)["id"] != id {
		t.Fatalf("direct_reports = %v", bossDetail.data()["direct_reports"])
	}
	expect(t, h.do(&hr, "GET", "/api/hris/employees/00000000-0000-0000-0000-000000000000", nil), 404, "Karyawan tidak ditemukan")

	upd := h.do(&hr, "PUT", "/api/hris/employees/"+id, map[string]any{"employment_status": "permanent", "phone": nil})
	expect(t, upd, 200, "")
	if upd.body["message"] != "Data karyawan berhasil diupdate" || upd.data()["employment_status"] != "permanent" {
		t.Fatalf("update = %s", upd.raw)
	}
	if notes := h.scalar(`SELECT notes FROM hris.employment_history WHERE employee_id = $1 AND change_type = 'status_change'`, id); notes != "Status: probation → permanent" {
		t.Fatalf("history notes = %q", notes)
	}
	bad := h.do(&hr, "PUT", "/api/hris/employees/"+id, map[string]any{"department_id": "nope"})
	expect(t, bad, 400, "Data karyawan tidak valid")

	del := h.do(&hr, "DELETE", "/api/hris/employees/"+id+"?reason=Karyawan%20uji%20dihapus", nil)
	expect(t, del, 200, "")
	if del.data()["is_active"] != false || !slices.Equal(keys(t, del.raw, "data"), []string{"id", "full_name", "nip", "is_active"}) {
		t.Fatalf("deactivate = %s", del.raw)
	}
}

func TestEmployeeDocumentsAndHistory(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuKepegawaian}, false)
	self, own := h.staff("pos", nil, true)
	h.grantDelete(hr, "hris.kepegawaian.users")

	expect(t, h.do(&hr, "GET", "/api/hris/employees/documents", nil), 400, "employee_id diperlukan")
	doc := h.do(&hr, "POST", "/api/hris/employees/documents", map[string]any{
		"employee_id": own, "document_type": "ktp", "document_name": "KTP", "file_url": "employee-docs/x.pdf",
		"file_size_kb": 0, "notes": "", "issue_date": "2026-01-01",
	})
	expect(t, doc, 201, "")
	if doc.data()["file_size_kb"] != nil || doc.data()["notes"] != nil || doc.body["message"] != "Dokumen berhasil disimpan" {
		t.Fatalf("document = %s", doc.raw)
	}
	expect(t, h.do(&hr, "POST", "/api/hris/employees/documents", map[string]any{"employee_id": own}), 400,
		"Field wajib: employee_id, document_type, document_name, file_url")

	mine := h.do(&self, "GET", "/api/hris/employees/documents?employee_id="+own, nil)
	expect(t, mine, 200, "")
	if len(mine.list()) != 1 {
		t.Fatalf("documents = %s", mine.raw)
	}
	docID := doc.data()["id"].(string)
	patched := h.do(&hr, "PATCH", "/api/hris/employees/documents/"+docID, map[string]any{"document_name": "KTP baru", "is_verified": true})
	expect(t, patched, 200, "")
	if patched.data()["document_name"] != "KTP baru" || patched.data()["is_verified"] != false {
		t.Fatalf("patch allowlist = %s", patched.raw)
	}
	expect(t, h.do(&self, "DELETE", "/api/hris/employees/documents/"+docID, nil), 403, "")
	expect(t, h.do(&hr, "DELETE", "/api/hris/employees/documents/"+docID+"?reason=Dokumen%20salah%20unggah", nil), 200, "")

	hist := h.do(&hr, "POST", "/api/hris/employment-history", map[string]any{
		"employee_id": own, "change_type": "promotion", "effective_date": "2026-09-01", "new_salary": "6500000", "prev_salary": 0,
	})
	expect(t, hist, 201, "")
	if hist.data()["new_salary"] != "6500000.00" || hist.data()["prev_salary"] != nil || hist.data()["reason"] != nil {
		t.Fatalf("history = %s", hist.raw)
	}
	listed := h.do(&self, "GET", "/api/hris/employment-history?employee_id="+own, nil)
	expect(t, listed, 200, "")
	if row := listed.list()[0].(map[string]any); row["prev_department"] != nil {
		t.Fatalf("history embed = %v", row)
	}
	expect(t, h.do(&hr, "POST", "/api/hris/employment-history", map[string]any{"employee_id": own}), 400,
		"Field wajib: employee_id, change_type, effective_date")
}

func TestEmployeeTabs(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuKepegawaian}, false)
	boss, bossID := h.staff("pos", nil, true)
	report := h.employee(emp{boss: &bossID, join: "2026-02-01"})
	shift := h.scalar(`INSERT INTO hris.shifts (name, start_time, end_time) VALUES ('Pagi Test', '08:00', '16:00') RETURNING id::text`)

	life := h.do(&hr, "GET", "/api/hris/employees/"+report+"/lifecycle", nil)
	expect(t, life, 200, "")
	if got := keys(t, life.raw, "data"); !slices.Equal(got, []string{"employee", "recruitment", "account", "onboarding", "history", "offboarding"}) {
		t.Fatalf("lifecycle keys %v", got)
	}
	expect(t, h.do(&hr, "GET", "/api/hris/employees/bad/lifecycle", nil), 400, "ID karyawan tidak valid")
	docs := h.do(&hr, "GET", "/api/hris/employees/"+report+"/recruitment-documents", nil)
	expect(t, docs, 200, "")
	if docs.body["data"] != nil {
		t.Fatalf("recruitment documents = %s", docs.raw)
	}

	days := []map[string]any{}
	for d := 1; d <= 7; d++ {
		var id any = shift
		if d > 5 {
			id = nil
		}
		days = append(days, map[string]any{"day_of_week": d, "shift_id": id})
	}
	// The direct manager may set the pattern; another employee may not.
	saved := h.do(&boss, "PUT", "/api/hris/employees/"+report+"/shifts", map[string]any{"effective_from": "2026-10-05", "days": days})
	expect(t, saved, 200, "")
	if saved.body["message"] != "Jadwal shift disimpan — berlaku mulai 2026-10-05" {
		t.Fatalf("save = %s", saved.raw)
	}
	stranger, _ := h.staff("pos", nil, true)
	expect(t, h.do(&stranger, "GET", "/api/hris/employees/"+report+"/shifts", nil), 403,
		"Hanya HRD atau atasan langsung yang boleh mengatur jadwal karyawan ini")
	bad := h.do(&hr, "PUT", "/api/hris/employees/"+report+"/shifts", map[string]any{"effective_from": "2026-10-05", "days": days[:6]})
	expect(t, bad, 400, "Pola jadwal harus lengkap 7 hari (Senin–Minggu)")
	listed := h.do(&hr, "GET", "/api/hris/employees/"+report+"/shifts", nil)
	expect(t, listed, 200, "")
	if len(listed.list()) != 7 || listed.list()[0].(map[string]any)["shift_name"] != "Pagi Test" {
		t.Fatalf("shifts = %s", listed.raw)
	}
	if n := h.scalar(`SELECT created_by_name FROM hris.employee_shifts WHERE employee_id = $1 LIMIT 1`, report); n == "" {
		t.Fatal("created_by_name missing")
	}

	// Unknown tabs and methods.
	expect(t, h.do(&hr, "GET", "/api/hris/employees/"+report+"/nope", nil), 404, "")
	expect(t, h.do(&hr, "DELETE", "/api/hris/employees/"+report+"/lifecycle", nil), 405, "")
}

func TestEmployeeContracts(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuKepegawaian, "hris.compensation"}, false)
	h.grantDelete(hr, "hris.kepegawaian.contracts")
	e := h.employee(emp{status: "contract"})
	base := "/api/hris/employees/" + e + "/contracts"

	expect(t, h.do(&hr, "POST", base, map[string]any{"contract_type": "x", "start_date": "2026-01-01"}), 400,
		"Tipe kontrak harus 'pkwt' atau 'pkwtt'")
	expect(t, h.do(&hr, "POST", base, map[string]any{"contract_type": "pkwt", "start_date": "2026-01-01"}), 400,
		"PKWT wajib memiliki tanggal berakhir (perjanjian waktu tertentu).")
	created := h.do(&hr, "POST", base, map[string]any{"contract_type": "pkwt", "start_date": "2026-01-01", "end_date": "2027-01-01"})
	expect(t, created, 201, "")
	number := created.data()["contract_number"].(string)
	if !strings.HasSuffix(number, "/PKWT/X/2026") || created.body["message"] != "Draft kontrak "+number+" dibuat" {
		t.Fatalf("contract = %s", created.raw)
	}
	if created.data()["base_salary"] != "4800000.00" || created.data()["sequence"] != 1.0 {
		t.Fatalf("defaults from the active salary = %s", created.raw)
	}
	list := h.do(&hr, "GET", base, nil)
	expect(t, list, 200, "")
	if len(list.list()) != 1 {
		t.Fatalf("contracts = %s", list.raw)
	}

	id := created.data()["id"].(string)
	act := h.do(&hr, "PATCH", "/api/hris/contracts/"+id, map[string]any{"action": "activate"})
	expect(t, act, 200, "")
	if status := h.scalar(`SELECT employment_status FROM hris.employees WHERE id = $1`, e); status != "contract" {
		t.Fatalf("employee status after activate = %q", status)
	}
	end := h.do(&hr, "PATCH", "/api/hris/contracts/"+id, map[string]any{"action": "end", "end_date": "2026-07-01"})
	expect(t, end, 200, "")
	if end.body["compensation_amount"] != 2_400_000.0 {
		t.Fatalf("compensation = %s", end.raw)
	}
	expect(t, h.do(&hr, "PATCH", "/api/hris/contracts/"+id, map[string]any{"action": "hapus"}), 400, "Aksi tidak dikenal")
	expect(t, h.do(&hr, "DELETE", "/api/hris/contracts/"+id+"?reason=Draft%20salah%20input", nil), 409, "Hanya draft kontrak yang bisa dihapus")
}

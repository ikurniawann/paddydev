package hris

import (
	"strings"
	"testing"
)

func TestContractLifecycle(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuKepegawaian, "hris.compensation"}, false)
	e := h.employee(emp{status: "contract", name: "Kontrak Test"})
	draft := func(body map[string]any) string {
		c := h.do(&hr, "POST", "/api/hris/employees/"+e+"/contracts", body)
		expect(t, c, 201, "")
		return c.data()["id"].(string)
	}
	pkwt := draft(map[string]any{"contract_type": "pkwt", "start_date": "2026-01-01", "end_date": "2026-12-31", "base_salary": 5000000})

	edit := h.do(&hr, "PATCH", "/api/hris/contracts/"+pkwt, map[string]any{"action": "edit", "base_salary": "abc"})
	expect(t, edit, 400, "Gaji pokok tidak valid")
	expect(t, h.do(&hr, "PATCH", "/api/hris/contracts/"+pkwt, map[string]any{"action": "edit", "work_location": "Jakarta"}), 200, "")
	expect(t, h.do(&hr, "PATCH", "/api/hris/contracts/"+pkwt, map[string]any{"action": "activate"}), 200, "")

	renew := h.do(&hr, "PATCH", "/api/hris/contracts/"+pkwt, map[string]any{"action": "renew", "end_date": "2027-12-31"})
	expect(t, renew, 201, "")
	if !strings.Contains(renew.body["message"].(string), "(mulai 2027-01-01)") {
		t.Fatalf("renew = %s", renew.raw)
	}
	expect(t, h.do(&hr, "PATCH", "/api/hris/contracts/"+pkwt, map[string]any{"action": "renew", "end_date": "2026-12-31"}), 400,
		"Tanggal berakhir perpanjangan harus setelah 2027-01-01")
	tooLong := h.do(&hr, "PATCH", "/api/hris/contracts/"+pkwt, map[string]any{"action": "renew", "end_date": "2033-01-01"})
	expect(t, tooLong, 422, "")
	expect(t, h.do(&hr, "PATCH", "/api/hris/contracts/"+pkwt, map[string]any{"action": "terminate"}), 400,
		"Alasan pemutusan kontrak wajib diisi")
	expect(t, h.do(&hr, "PATCH", "/api/hris/contracts/"+pkwt, map[string]any{"action": "update",
		"signed_document_url": "contract-signed/../x/a.pdf"}), 400, "Path dokumen tidak valid")
	expect(t, h.do(&hr, "PATCH", "/api/hris/contracts/"+pkwt, map[string]any{"action": "update", "kemnaker_registered_at": "2026-02-01"}), 200, "")
	conv := h.do(&hr, "PATCH", "/api/hris/contracts/"+pkwt, map[string]any{"action": "convert"})
	expect(t, conv, 200, "")

	list := h.do(&hr, "GET", "/api/hris/contracts?status=all&search=Kontrak%20Test&sort_by=start_date", nil)
	expect(t, list, 200, "")
	if list.body["total"] != 2.0 || list.body["limit"] != 15.0 {
		t.Fatalf("contracts = %s", list.raw)
	}
	exp := h.do(&hr, "GET", "/api/hris/contracts/expiring?days=500", nil)
	expect(t, exp, 200, "")
	if exp.data()["days"] != 90.0 || keys(t, exp.raw, "data")[3] != "noContract" {
		t.Fatalf("expiring = %s", exp.raw)
	}
	expect(t, h.do(&hr, "PATCH", "/api/hris/contracts/00000000-0000-0000-0000-000000000000", map[string]any{"action": "end"}), 404,
		"Kontrak tidak ditemukan")
}

func TestOnboardingOffboarding(t *testing.T) {
	h := newHarness(t)
	hr, hrID := h.staff("hrd", []string{menuKepegawaian}, true)
	staff, own := h.staff("pos", nil, true)

	list := h.do(&staff, "GET", "/api/hris/onboarding/"+own, nil)
	expect(t, list, 200, "")
	summary := list.body["summary"].(map[string]any)
	if summary["total"].(float64) != float64(len(list.list())) {
		t.Fatalf("onboarding = %s", list.raw)
	}
	added := h.do(&hr, "POST", "/api/hris/onboarding/"+own, map[string]any{"action": "add", "task_name": "Kartu akses", "category": "it"})
	expect(t, added, 200, "")
	if added.data()["assigned_to"] != hrID || added.data()["priority"] != 3.0 {
		t.Fatalf("add = %s", added.raw)
	}
	task := added.data()["id"].(string)
	expect(t, h.do(&staff, "POST", "/api/hris/onboarding/"+own, map[string]any{"action": "complete", "task_id": task}), 403,
		"Forbidden: Not authorized to complete this task")
	done := h.do(&hr, "POST", "/api/hris/onboarding/"+own, map[string]any{"action": "complete", "task_id": task})
	expect(t, done, 200, "")
	if done.data()["completed_by"] != hrID {
		t.Fatalf("completed_by must be the employee id: %s", done.raw)
	}
	expect(t, h.do(&hr, "POST", "/api/hris/onboarding/"+own, map[string]any{"action": "x"}), 400,
		`Invalid action. Use "complete" with task_id or "add" with task details`)
	expect(t, h.do(&staff, "PUT", "/api/hris/onboarding/"+own, map[string]any{"task_id": task, "priority": 1}), 403,
		"Forbidden: Only HRD or managers can update tasks")
	expect(t, h.do(&hr, "PUT", "/api/hris/onboarding/"+own, map[string]any{}), 400, "task_id is required")
	expect(t, h.do(&hr, "PUT", "/api/hris/onboarding/"+own, map[string]any{"task_id": task, "priority": 1}), 200, "")

	expect(t, h.do(&staff, "POST", "/api/hris/offboarding/"+own, map[string]any{
		"resignation_type": "termination", "resignation_date": "2026-10-05", "last_working_day": "2026-11-05"}), 403,
		"Only HRD/Manager can initiate non-voluntary resignation")
	off := h.do(&staff, "POST", "/api/hris/offboarding/"+own, map[string]any{
		"resignation_type": "voluntary", "resignation_date": "2026-10-05", "last_working_day": "2026-11-05"})
	expect(t, off, 200, "")
	if off.body["message"] != "Offboarding process initiated successfully" {
		t.Fatalf("offboarding = %s", off.raw)
	}
	expect(t, h.do(&staff, "POST", "/api/hris/offboarding/"+own, map[string]any{
		"resignation_type": "voluntary", "resignation_date": "2026-10-05", "last_working_day": "2026-11-05"}), 400,
		"Offboarding already initiated for this employee")
	upd := h.do(&hr, "PUT", "/api/hris/offboarding/"+own, map[string]any{
		"clearance_type": "it", "cleared": true, "asset_updates": map[string]any{"laptop": "returned"}, "status": "completed"})
	expect(t, upd, 200, "")
	if upd.data()["clearance_it"] != true || upd.data()["completed_by"] != hrID {
		t.Fatalf("offboarding update = %s", upd.raw)
	}
	listed := h.do(&staff, "GET", "/api/hris/offboarding/"+own, nil)
	expect(t, listed, 200, "")
	if row := listed.list()[0].(map[string]any); row["completer"].(map[string]any)["id"] != hrID {
		t.Fatalf("offboarding embeds = %s", listed.raw)
	}
}

func TestSelfService(t *testing.T) {
	h := newHarness(t)
	dept := h.department("ESS Dept")
	boss, bossID := h.staff("pos", nil, true, emp{dept: &dept})
	_ = h.employee(emp{boss: &bossID, name: "Anak Buah"})
	unlinked, _ := h.staff("pos", nil, false)

	me := h.do(&boss, "GET", "/api/hris/me", nil)
	expect(t, me, 200, "")
	if keys(t, me.raw, "data")[0] != "employee" || me.data()["has_schedule"] != false {
		t.Fatalf("me = %s", me.raw)
	}
	if h.do(&unlinked, "GET", "/api/hris/me", nil).raw != `{"data":{"employee":null,"leave_balance":null}}` {
		t.Fatal("me without employee")
	}
	home := h.do(&boss, "GET", "/api/hris/me/beranda", nil)
	expect(t, home, 200, "")
	d := home.data()
	if d["latest_payslip"].(map[string]any)["net_salary"] != 5250000.0 || len(d["week_schedule"].([]any)) != 7 || d["kpi"] != nil {
		t.Fatalf("beranda = %s", home.raw)
	}
	if req := d["recent_requests"].([]any)[0].(map[string]any); req["detail"] != "Rp 1.500.000" {
		t.Fatalf("recent requests = %v", d["recent_requests"])
	}
	team := h.do(&boss, "GET", "/api/hris/me/team", nil)
	if members := team.data()["members"].([]any); len(members) != 1 {
		t.Fatalf("team = %s", team.raw)
	}

	// Announcements.
	hr, _ := h.staff("hrd", []string{menuKepegawaian}, true)
	h.grantDelete(hr, "hris.kepegawaian.announcements")
	expect(t, h.do(&hr, "POST", "/api/hris/announcements", map[string]any{"title": " "}), 400, "Validasi gagal")
	expect(t, h.do(&hr, "POST", "/api/hris/announcements", map[string]any{"title": "Video", "video_url": "https://evil.test/v"}), 400,
		"URL video harus YouTube atau Vimeo yang valid")
	ann := h.do(&hr, "POST", "/api/hris/announcements", map[string]any{
		"title": "Rapat", "status": "published", "target_scope": "department", "department_ids": []string{dept},
		"tags": []string{" Umum "}, "video_url": "https://youtu.be/dQw4w9WgXcQ"})
	expect(t, ann, 200, "")
	if ann.data()["video_provider"] != "youtube" || ann.data()["tags"].([]any)[0] != "Umum" {
		t.Fatalf("announcement = %s", ann.raw)
	}
	id := ann.data()["id"].(string)
	feed := h.do(&boss, "GET", "/api/hris/announcements/feed", nil)
	if feed.body["unread"] != 1.0 {
		t.Fatalf("feed = %s", feed.raw)
	}
	if h.do(&boss, "GET", "/api/hris/announcements/unread-count", nil).raw != `{"unread":1}` {
		t.Fatal("unread count")
	}
	if h.do(nil, "GET", "/api/hris/announcements/unread-count", nil).raw != `{"unread":0}` {
		t.Fatal("unread count signed out")
	}
	expect(t, h.do(&unlinked, "GET", "/api/hris/announcements/"+id, nil), 404, "Pengumuman tidak ditemukan")
	expect(t, h.do(&boss, "GET", "/api/hris/announcements/"+id, nil), 200, "")
	expect(t, h.do(&unlinked, "POST", "/api/hris/announcements/"+id+"/read", nil), 403, "Akun tidak tertaut karyawan")
	if h.do(&boss, "POST", "/api/hris/announcements/"+id+"/read", nil).raw != `{"ok":true}` {
		t.Fatal("mark read")
	}
	badges := h.do(&boss, "GET", "/api/hris/nav-badges", nil)
	if badges.raw != `{"badges":{"/dashboard/me/cuti":0,"/dashboard/me/lembur":0,"/dashboard/me/pinjaman":0,"/dashboard/me/pengumuman":0}}` {
		t.Fatalf("badges = %s", badges.raw)
	}
	hrBadges := h.do(&hr, "GET", "/api/hris/nav-badges", nil)
	if !strings.HasPrefix(hrBadges.raw, `{"badges":{"/dashboard/hris/leaves":`) || !strings.Contains(hrBadges.raw, `"/dashboard/hris/loans":2`) {
		t.Fatalf("hr badges = %s", hrBadges.raw)
	}
	expect(t, h.do(&boss, "POST", "/api/hris/nav-badges", map[string]any{"module": "x"}), 400, "Modul tidak valid")
	if h.do(&boss, "POST", "/api/hris/nav-badges", map[string]any{"module": "leaves"}).raw != `{"success":true}` {
		t.Fatal("module seen")
	}
	list := h.do(&hr, "GET", "/api/hris/announcements?status=published", nil)
	if row := list.list()[0].(map[string]any); row["read_count"] != "1" || row["department_ids"].([]any)[0] != dept {
		t.Fatalf("cms list = %s", list.raw)
	}
	expect(t, h.do(&hr, "PATCH", "/api/hris/announcements/"+id, map[string]any{"title": "Rapat 2", "target_scope": "global"}), 200, "")
	expect(t, h.do(&hr, "DELETE", "/api/hris/announcements/"+id+"?reason=Pengumuman%20kedaluwarsa", nil), 200, "")
	expect(t, h.do(&hr, "PATCH", "/api/hris/announcements/"+id, map[string]any{"title": "x"}), 404, "Pengumuman tidak ditemukan")

	// Notifications.
	h.exec(`INSERT INTO public.notifications (user_id, title, message, type, is_read) VALUES ($1, 'Halo', 'Pesan', 'alert', false)`, boss.UserID)
	notes := h.do(&boss, "GET", "/api/hris/notifications?unread=true", nil)
	if len(notes.list()) != 1 || notes.body["pagination"].(map[string]any)["limit"] != 50.0 {
		t.Fatalf("notifications = %s", notes.raw)
	}
	expect(t, h.do(&boss, "POST", "/api/hris/notifications", map[string]any{}), 400, "notification_id or mark_all is required")
	if h.do(&boss, "POST", "/api/hris/notifications", map[string]any{"mark_all": true}).body["message"] != "All notifications marked as read" {
		t.Fatal("mark all")
	}
}

func TestLogbookReportsMaster(t *testing.T) {
	h := newHarness(t)
	dept := h.department("Logbook Dept")
	other := h.department("Logbook Lain")
	hr, _ := h.staff("hrd", []string{"hris.insights", "hris.master", "hris.performance"}, true)
	staff, _ := h.staff("pos", nil, true, emp{dept: &dept})
	h.grantDelete(hr, "hris.master.departments", "hris.master.employment-statuses")

	me := h.do(&staff, "GET", "/api/hris/logbook?resource=me", nil)
	if me.data()["is_full_access"] != false || me.data()["employee"].(map[string]any)["department"].(map[string]any)["name"] != "Logbook Dept" {
		t.Fatalf("logbook me = %s", me.raw)
	}
	expect(t, h.do(&staff, "GET", "/api/hris/logbook?resource=templates&department_id="+other, nil), 403,
		"Anda tidak berhak mengakses department ini")
	expect(t, h.do(&staff, "POST", "/api/hris/logbook", map[string]any{"action": "create-template", "name": "Harian", "items": []any{}}), 400,
		"Minimal satu checklist item harus diisi")
	tpl := h.do(&staff, "POST", "/api/hris/logbook", map[string]any{"action": "create-template", "name": "Harian",
		"department_id": other, "items": []any{map[string]any{"title": "Cek kulkas", "weight": 0}, map[string]any{"title": " "}}})
	expect(t, tpl, 201, "")
	if tpl.data()["department_id"] != dept {
		t.Fatalf("non full access must write own department: %s", tpl.raw)
	}
	entry := h.do(&staff, "POST", "/api/hris/logbook", map[string]any{"action": "create-entry", "template_id": tpl.data()["id"], "entry_date": "2026-10-05"})
	expect(t, entry, 201, "")
	if entry.data()["title"] != "Harian - 2026-10-05" {
		t.Fatalf("entry = %s", entry.raw)
	}
	expect(t, h.do(&staff, "POST", "/api/hris/logbook", map[string]any{"action": "create-entry", "template_id": tpl.data()["id"], "entry_date": "2026-10-05"}), 409,
		"Logbook untuk template & tanggal ini sudah ada")
	entries := h.do(&staff, "GET", "/api/hris/logbook", nil)
	if entries.body["count"] != 1.0 || len(entries.list()[0].(map[string]any)["items"].([]any)) != 1 {
		t.Fatalf("entries = %s", entries.raw)
	}
	item := entries.list()[0].(map[string]any)["items"].([]any)[0].(map[string]any)["id"].(string)
	checked := h.do(&staff, "PATCH", "/api/hris/logbook", map[string]any{"action": "update-item", "item_id": item, "is_checked": true})
	if checked.data()["is_checked"] != true || checked.data()["checked_by"] != staff.UserID {
		t.Fatalf("check = %s", checked.raw)
	}
	entryID := entry.data()["id"].(string)
	expect(t, h.do(&staff, "PATCH", "/api/hris/logbook", map[string]any{"action": "submit-entry", "entry_id": entryID}), 200, "")
	expect(t, h.do(&staff, "PATCH", "/api/hris/logbook", map[string]any{"action": "review-entry", "entry_id": entryID}), 403,
		"Anda tidak berhak me-review logbook")
	expect(t, h.do(&hr, "PATCH", "/api/hris/logbook", map[string]any{"action": "review-entry", "entry_id": entryID}), 200, "")
	expect(t, h.do(&staff, "PATCH", "/api/hris/logbook", map[string]any{"action": "nope"}), 422, "Unknown action")
	expect(t, h.do(&staff, "DELETE", "/api/hris/logbook?resource=entry&id="+entryID, nil), 409, "Hanya logbook draft yang bisa dihapus")
	arch := h.do(&staff, "DELETE", "/api/hris/logbook?resource=template&id="+tpl.data()["id"].(string), nil)
	if arch.raw != `{"message":"Template diarsipkan (sudah dipakai logbook)","archived":true}` {
		t.Fatalf("archive = %s", arch.raw)
	}
	summary := h.do(&hr, "GET", "/api/hris/logbook?resource=summary", nil)
	expect(t, summary, 200, "")
	expect(t, h.do(nil, "GET", "/api/hris/logbook", nil), 401, "Unauthorized")

	// Report.
	expect(t, h.do(&staff, "GET", "/api/hris/reports", nil), 403, "Insufficient permissions")
	expect(t, h.do(&hr, "GET", "/api/hris/reports?month=13", nil), 400, "Periode tidak valid")
	rep := h.do(&hr, "GET", "/api/hris/reports?month=10&year=2026", nil)
	expect(t, rep, 200, "")
	if got := keys(t, rep.raw); strings.Join(got, ",") != "period,headcount,attendance,leaves" {
		t.Fatalf("report keys %v", got)
	}

	// Master data.
	created := h.do(&hr, "POST", "/api/master/departments", map[string]any{"name": "Gudang", "code": "gd" + dept[:4]})
	expect(t, created, 201, "")
	if created.data()["code"] != strings.ToUpper("gd"+dept[:4]) || created.body["message"] != "Departemen berhasil ditambahkan" {
		t.Fatalf("department = %s", created.raw)
	}
	expect(t, h.do(&hr, "POST", "/api/master/departments", map[string]any{"name": "Gudang 2", "code": "gd" + dept[:4]}), 400,
		"Kode departemen sudah digunakan")
	expect(t, h.do(&hr, "POST", "/api/master/departments", map[string]any{"name": ""}), 400, "Validation failed")
	expect(t, h.do(&hr, "POST", "/api/master/departments", "not json"), 500, "Terjadi kesalahan server")
	expect(t, h.do(&hr, "DELETE", "/api/master/departments/"+dept+"?reason=Departemen%20dobel", nil), 400, "Tidak dapat dihapus, masih ada 1 karyawan di departemen ini")
	expect(t, h.do(&hr, "PUT", "/api/master/departments/00000000-0000-0000-0000-000000000000", map[string]any{"name": "A", "code": "B"}), 404,
		"Departemen tidak ditemukan")
	pos := h.do(&hr, "POST", "/api/master/positions", map[string]any{"title": "Barista Test", "brand_id": ""})
	expect(t, pos, 201, "")
	if pos.data()["department"] != "Operations" || pos.data()["brands"] != nil {
		t.Fatalf("position = %s", pos.raw)
	}
	st := h.do(&hr, "POST", "/api/master/employment-statuses", map[string]any{"code": "FREELANCE" + dept[:4], "name": "Freelance"})
	expect(t, st, 201, "")
	if st.data()["color"] != "gray" {
		t.Fatalf("status = %s", st.raw)
	}
	expect(t, h.do(&hr, "DELETE", "/api/master/employment-statuses/"+st.data()["id"].(string)+"?reason=Status%20tidak%20dipakai", nil), 200, "")
	expect(t, h.do(&hr, "GET", "/api/master/positions", nil), 200, "")
}

// An HR account holding only the employee-level Kinerja leaves (Review,
// Tugas Departemen; e.g. through an extra employee role) keeps its own
// department's logbook only (EPIC-057).
func TestLogbookESSKinerjaLeavesAreNotFullAccess(t *testing.T) {
	h := newHarness(t)
	dept := h.department("Logbook ESS")
	hr, _ := h.staff("hrd", []string{"hris.kepegawaian", "hris.performance.review", "hris.performance.dept-tasks"}, true, emp{dept: &dept})
	me := h.do(&hr, "GET", "/api/hris/logbook?resource=me", nil)
	if me.data()["is_full_access"] != false || me.data()["can_review"] != false {
		t.Fatalf("logbook me = %s", me.raw)
	}
}

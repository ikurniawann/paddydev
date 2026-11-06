package hris

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

// EPIC-094: HR menerbitkan Surat Teguran/SP; lonceng ke karyawan, penyetuju, dan HR.

const (
	lettersMenu = "hris.workforce.disciplinary-letters"
	lettersURL  = "/api/hris/disciplinary-letters"
)

var letterPDF = []byte("%PDF-1.4\n% surat peringatan\n")

// letterUpload posts one letter attachment for employeeID as s.
func (h *harness) letterUpload(s *testutil.Staff, employeeID string, data []byte) call {
	h.t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("employee_id", employeeID)
	fw, _ := w.CreateFormFile("file", "surat.pdf")
	_, _ = fw.Write(data)
	_ = w.Close()
	r := httptest.NewRequest("POST", lettersURL+"/attachments", &body)
	r.Header.Set("Content-Type", w.FormDataContentType())
	rec := h.send(s, r)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return call{rec.Code, out, rec.Body.String()}
}

func TestDisciplinaryIssueNotifiesAndAudits(t *testing.T) {
	h, _ := newFileHarness(t)
	h.setNow("2026-10-20 10:00")
	c := h.tieredChain()
	hr, _ := h.staff("hrd", []string{lettersMenu}, true, emp{name: "HR Surat"})
	issue := h.do(&hr, "POST", lettersURL, map[string]any{
		"employee_id": c.crewID, "letter_type": "sp1", "issued_date": "2026-10-20", "notes": "Terlambat 3 kali"})
	expect(t, issue, 201, "")
	d := issue.data()
	if d["label"] != "Surat Peringatan 1" || d["employee_name"] != "Crew Toko" || d["issued_date"] != "2026-10-20" || d["notes"] != "Terlambat 3 kali" {
		t.Fatalf("issue = %s", issue.raw)
	}
	if got := h.notice(c.crew.UserID, "Surat Peringatan 1 dari HR"); got != "alert|/dashboard/me/absensi#surat|HR menerbitkan Surat Peringatan 1 untuk Anda tanggal 20-10-2026." {
		t.Errorf("employee bell = %q", got)
	}
	team := "HR menerbitkan Surat Peringatan 1 untuk Crew Toko tanggal 20-10-2026."
	for name, s := range map[string]string{"SPV": c.spv.UserID, "Manager": c.mgr.UserID} {
		if got := h.notice(s, "Surat Peringatan 1 untuk Crew Toko"); got != "alert|/dashboard/me/absensi-tim|"+team {
			t.Errorf("%s bell = %q", name, got)
		}
	}
	if got := h.notice(hr.UserID, "Surat Peringatan 1 untuk Crew Toko"); got != "alert|/dashboard/hris/surat-peringatan|"+team {
		t.Errorf("HR bell = %q", got)
	}
	if got := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'disciplinary_letter.issue' AND entity_id = $1`, c.crewID); got != "1" {
		t.Errorf("issue audit rows = %s, want 1", got)
	}
	list := h.do(&hr, "GET", lettersURL+"?employee_id="+c.crewID, nil)
	expect(t, list, 200, "")
	if len(list.list()) != 1 {
		t.Fatalf("list = %s", list.raw)
	}
}

func TestDisciplinaryValidation(t *testing.T) {
	h, _ := newFileHarness(t)
	h.setNow("2026-10-20 10:00")
	hr, _ := h.staff("hrd", []string{lettersMenu}, true, emp{name: "HR Surat"})
	reader := h.staffActions("hrd", map[string][]string{lettersMenu: {"read"}})
	staff, staffID := h.staff("pos", nil, true, emp{name: "Crew Biasa"})
	gone := h.employee(emp{name: "Sudah Keluar", inactive: true})
	body := func(id, typ, date string) map[string]any {
		return map[string]any{"employee_id": id, "letter_type": typ, "issued_date": date}
	}
	expect(t, h.do(&hr, "POST", lettersURL, body(staffID, "sp4", "2026-10-20")), 400, "")
	expect(t, h.do(&hr, "POST", lettersURL, body(staffID, "sp1", "2026-10-21")), 400, "Tanggal terbit tidak boleh di masa depan")
	expect(t, h.do(&hr, "POST", lettersURL, body(gone, "sp1", "2026-10-20")), 400, "Karyawan tidak aktif")
	expect(t, h.do(&staff, "POST", lettersURL, body(staffID, "sp1", "2026-10-20")), 403, "")
	expect(t, h.do(&staff, "GET", lettersURL, nil), 403, "")
	expect(t, h.do(&reader, "POST", lettersURL, body(staffID, "sp1", "2026-10-20")), 403, "")
	expect(t, h.do(&reader, "GET", lettersURL, nil), 200, "")
}

func TestDisciplinaryRevoke(t *testing.T) {
	h, _ := newFileHarness(t)
	h.setNow("2026-10-20 10:00")
	hr, _ := h.staff("hrd", []string{lettersMenu}, true, emp{name: "HR Surat"})
	staff, staffID := h.staff("pos", nil, true, emp{name: "Crew Surat"})
	issue := h.do(&hr, "POST", lettersURL, map[string]any{"employee_id": staffID, "letter_type": "teguran", "issued_date": "2026-10-19"})
	expect(t, issue, 201, "")
	id := issue.data()["id"].(string)
	mine := h.do(&staff, "GET", "/api/hris/me/disciplinary-letters", nil)
	expect(t, mine, 200, "")
	if len(mine.list()) != 1 || mine.list()[0].(map[string]any)["label"] != "Surat Teguran" {
		t.Fatalf("me = %s", mine.raw)
	}
	revoke := lettersURL + "/" + id + "/revoke"
	expect(t, h.do(&hr, "POST", revoke, map[string]any{"reason": "  "}), 400, "Alasan pembatalan wajib diisi")
	expect(t, h.do(&hr, "POST", revoke, map[string]any{"reason": "Salah orang"}), 200, "")
	expect(t, h.do(&hr, "POST", revoke, map[string]any{"reason": "Lagi"}), 400, "Surat sudah dibatalkan")
	if got := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'disciplinary_letter.revoke' AND entity_id = $1`, staffID); got != "1" {
		t.Errorf("revoke audit rows = %s, want 1", got)
	}
	if mine := h.do(&staff, "GET", "/api/hris/me/disciplinary-letters", nil); len(mine.list()) != 0 {
		t.Fatalf("me after revoke = %s", mine.raw)
	}
	all := h.do(&hr, "GET", lettersURL+"?employee_id="+staffID+"&include_revoked=1", nil)
	if len(all.list()) != 1 || all.list()[0].(map[string]any)["revoke_reason"] != "Salah orang" {
		t.Fatalf("include_revoked = %s", all.raw)
	}
	if hidden := h.do(&hr, "GET", lettersURL+"?employee_id="+staffID, nil); len(hidden.list()) != 0 {
		t.Fatalf("default list must hide revoked: %s", hidden.raw)
	}
}

func TestDisciplinaryAttachmentAccess(t *testing.T) {
	h, _ := newFileHarness(t)
	h.setNow("2026-10-20 10:00")
	c := h.tieredChain()
	hr, _ := h.staff("hrd", []string{lettersMenu}, true, emp{name: "HR Surat"})
	other, _ := h.staff("pos", nil, true, emp{name: "Crew Lain"})
	up := h.letterUpload(&hr, c.crewID, letterPDF)
	expect(t, up, 200, "")
	path := up.data()["path"].(string)
	issue := h.do(&hr, "POST", lettersURL, map[string]any{
		"employee_id": c.crewID, "letter_type": "sp2", "issued_date": "2026-10-20", "attachment_path": path})
	expect(t, issue, 201, "")
	file := lettersURL + "/attachment/" + path
	for name, s := range map[string]*testutil.Staff{"owner": &c.crew, "SPV": &c.spv, "Manager": &c.mgr, "HR": &hr} {
		if got := h.do(s, "GET", file, nil); got.status != 200 {
			t.Errorf("%s read = %d %s", name, got.status, got.raw)
		}
	}
	expect(t, h.do(&other, "GET", file, nil), 403, "")
	// An attachment of another employee is refused when issuing.
	expect(t, h.do(&hr, "POST", lettersURL, map[string]any{
		"employee_id": c.spvID, "letter_type": "sp1", "issued_date": "2026-10-20", "attachment_path": path}), 400, "")
	id := issue.data()["id"].(string)
	expect(t, h.do(&hr, "POST", lettersURL+"/"+id+"/revoke", map[string]any{"reason": "Dibatalkan"}), 200, "")
	expect(t, h.do(&c.crew, "GET", file, nil), 403, "")
	expect(t, h.do(&hr, "GET", file, nil), 200, "")
}

func TestDisciplinaryLatenessHint(t *testing.T) {
	h, _ := newFileHarness(t)
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 0, closing_offset_days = 3`)
	h.setNow("2026-10-20 10:00")
	hr, _ := h.staff("hrd", []string{lettersMenu}, true, emp{name: "HR Surat"})
	id := h.employee(emp{name: "Sering Telat"})
	for _, d := range []string{"2026-10-02", "2026-10-05", "2026-10-09"} {
		h.late(id, d, 15, true)
	}
	got := h.do(&hr, "GET", lettersURL+"/lateness?employee_id="+id, nil)
	expect(t, got, 200, "")
	if d := got.data(); d["count"] != float64(3) || d["level"] != "sp1" || d["label"] != "Surat Peringatan 1" {
		t.Fatalf("lateness = %s", got.raw)
	}
}

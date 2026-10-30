package hris

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"nuhabit/backend/internal/platform/iam"
)

// EPIC-072 R7: an account holding "approve" on the Cuti or Lembur leaf (an
// IAM grant, without an HR app role) reads the HR lists, the leave detail
// and the attachments it decides on; without the grant it does not.
func TestApproverReadsRequestDetails(t *testing.T) {
	h, dir := newFileHarness(t)
	approver, _ := h.staff("pos", []string{iam.LeafCuti, iam.LeafLembur}, true, emp{name: "Penyetuju IAM"})
	plain, _ := h.staff("pos", nil, true, emp{name: "Bukan Penyetuju"})
	_, crew := h.staff("pos", nil, true, emp{name: "Crew Detail"})

	leaveFile := "leave-attachments/" + crew + "/surat.png"
	otFile := "overtime-attachments/" + crew + "/bukti.pdf"
	for path, data := range map[string][]byte{leaveFile: pngBytes(t), otFile: otPDF} {
		full := filepath.Join(dir, "private", path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	leave := h.scalar(`INSERT INTO hris.leaves (employee_id, leave_type, start_date, end_date, total_days, reason, attachment_url, status)
		VALUES ($1, 'sick', '2026-10-06', '2026-10-07', 2, 'Demam tinggi, surat dokter terlampir', $2, 'pending') RETURNING id::text`,
		crew, leaveFile)
	h.exec(`INSERT INTO hris.overtime_requests (employee_id, date, start_time, end_time, hours, reason, attachments)
		VALUES ($1, '2026-10-05', '18:00', '21:00', 3, 'Stock opname', ARRAY[$2])`, crew, otFile)

	leaves := h.do(&approver, "GET", "/api/hris/leaves?limit=100", nil)
	expect(t, leaves, 200, "")
	if !hasEmployeeRow(leaves.list(), crew) {
		t.Errorf("approver leave list misses the crew's leave: %s", leaves.raw)
	}
	overtime := h.do(&approver, "GET", "/api/hris/overtime", nil)
	expect(t, overtime, 200, "")
	if !hasEmployeeRow(overtime.list(), crew) {
		t.Errorf("approver overtime list misses the crew's overtime: %s", overtime.raw)
	}
	detail := h.do(&approver, "GET", "/api/hris/leaves/"+leave, nil)
	expect(t, detail, 200, "")
	if detail.data()["reason"] != "Demam tinggi, surat dokter terlampir" {
		t.Errorf("leave detail = %s", detail.raw)
	}
	for _, url := range []string{"/api/hris/leaves/attachment/" + leaveFile, "/api/hris/overtime/attachment/" + otFile} {
		if rec := h.send(&approver, httptest.NewRequest("GET", url, nil)); rec.Code != 200 {
			t.Errorf("approver GET %s = %d %s", url, rec.Code, rec.Body.String())
		}
		if rec := h.send(&plain, httptest.NewRequest("GET", url, nil)); rec.Code != 403 {
			t.Errorf("plain GET %s = %d, want 403", url, rec.Code)
		}
	}

	// ESS lists its own requests with employee_id=me: an approver sees only
	// their own rows there, not the HR-wide list (review finding #1).
	if own := h.do(&approver, "GET", "/api/hris/leaves?employee_id=me&limit=100", nil); own.status != 200 || hasEmployeeRow(own.list(), crew) {
		t.Errorf("approver ESS leave list = %d %s", own.status, own.raw)
	}
	if own := h.do(&approver, "GET", "/api/hris/overtime?employee_id=me", nil); own.status != 200 || hasEmployeeRow(own.list(), crew) {
		t.Errorf("approver ESS overtime list = %d %s", own.status, own.raw)
	}

	if l := h.do(&plain, "GET", "/api/hris/leaves?limit=100", nil); hasEmployeeRow(l.list(), crew) {
		t.Errorf("plain account sees the crew's leave")
	}
	expect(t, h.do(&plain, "GET", "/api/hris/leaves/"+leave, nil), 403, "")
}

func hasEmployeeRow(rows []any, employeeID string) bool {
	for _, r := range rows {
		if m, ok := r.(map[string]any); ok && m["employee_id"] == employeeID {
			return true
		}
	}
	return false
}

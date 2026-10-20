package hris

import (
	"net/http"
	"testing"

	"nuhabit/backend/internal/platform/hrisdel"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/testutil"
)

// grantDelete gives s "read" and "delete" on leaf menus its role lacks.
func (h *harness) grantDelete(s testutil.Staff, leaves ...string) {
	h.t.Helper()
	for _, leaf := range leaves {
		testutil.GrantMenu(h.t, s.RoleID, leaf, []string{"read", "delete"})
	}
}

// EPIC-061 Q-RA-1/4: every HRIS delete needs "delete" on its own leaf (a
// group grant with every action or a sibling leaf is refused) and a reason.
func TestHrisDeletesNeedLeafDeleteAndReason(t *testing.T) {
	h := newHarness(t)
	const id = "00000000-0000-4000-8000-0000000000ab"
	const attendanceReason = "Alasan penyesuaian wajib diisi (minimal 5 karakter)"
	routes := []struct{ path, leaf, reasonMsg string }{
		{"/api/hris/employees/" + id, iam.LeafKaryawan, hrisdel.ErrReason},
		{"/api/hris/employees/documents/" + id, iam.LeafKaryawan, hrisdel.ErrReason},
		{"/api/hris/employee-placements/" + id, iam.LeafPenempatan, hrisdel.ErrReason},
		{"/api/hris/attendance/" + id, iam.LeafAbsensi, attendanceReason},
		{"/api/hris/leaves/" + id, iam.LeafCuti, hrisdel.ErrReason},
		{"/api/hris/shifts/" + id, iam.LeafShift, hrisdel.ErrReason},
		{"/api/hris/holidays/" + id, iam.LeafHariLibur, hrisdel.ErrReason},
		{"/api/hris/contracts/" + id, iam.LeafKontrak, hrisdel.ErrReason},
		{"/api/hris/contracts/" + id + "/signed-document", iam.LeafKontrak, hrisdel.ErrReason},
		{"/api/hris/announcements/" + id, iam.LeafPengumuman, hrisdel.ErrReason},
		{"/api/master/departments/" + id, iam.LeafDepartemen, hrisdel.ErrReason},
		{"/api/master/employment-statuses/" + id, iam.LeafStatusKaryawan, hrisdel.ErrReason},
		{"/api/master/positions/" + id, iam.LeafJabatan, hrisdel.ErrReason},
	}
	// Group menus with every action: the old prefix guards let these through.
	group, _ := h.staff("hrd", []string{"hris.kepegawaian", "hris.master", "hris.workforce", "settings.users"}, false)
	for _, rt := range routes {
		var others []string
		for _, l := range iam.HrisDeleteLeaves {
			if l != rt.leaf {
				others = append(others, l)
			}
		}
		sibling, _ := h.staff("hrd", others, false)
		own, _ := h.staff("hrd", []string{rt.leaf}, false)
		expect(t, h.do(&group, http.MethodDelete, rt.path+"?reason=Data%20uji%20hapus", nil), http.StatusForbidden, "Insufficient permissions")
		expect(t, h.do(&sibling, http.MethodDelete, rt.path+"?reason=Data%20uji%20hapus", nil), http.StatusForbidden, "Insufficient permissions")
		expect(t, h.do(&own, http.MethodDelete, rt.path, nil), http.StatusBadRequest, rt.reasonMsg)
		expect(t, h.do(&own, http.MethodDelete, rt.path+"?reason=abcd", nil), http.StatusBadRequest, rt.reasonMsg)
		if c := h.do(&own, http.MethodDelete, rt.path+"?reason=Data%20uji%20hapus", nil); c.status == http.StatusForbidden || c.status == http.StatusInternalServerError || c.body["error"] == rt.reasonMsg {
			t.Errorf("%s with leaf delete + reason = %d %s", rt.path, c.status, c.raw)
		}
	}
}

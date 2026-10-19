package hris

import (
	"context"
	"net/http"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

// Review I3: aksi yang mengubah data butuh aksi "update" pada menunya,
// bukan sekadar menu terlihat (peran baca-saja ditolak).
func TestGoLiveWriteRoutesNeedUpdateAction(t *testing.T) {
	h := newHarness(t)
	ro := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{
		"hris.kepegawaian.users": {"read"}, "hris.kepegawaian.schedules": {"read"}}})
	// Like h.staff: roll the fixture back before testutil deletes the account.
	h.t.Cleanup(func() { _ = h.tx.Rollback(context.Background()) })
	e := h.employee(emp{})
	for _, c := range []struct{ method, path string }{
		{http.MethodPost, "/api/hris/employees/" + e + "/resign"},
		{http.MethodPut, "/api/hris/employees/" + e + "/schedule-type"},
		{http.MethodPost, "/api/hris/employees/schedule-type/bulk"},
		{http.MethodPost, "/api/hris/schedules/fixed"},
		{http.MethodPost, "/api/hris/schedules/import"},
		{http.MethodPost, "/api/hris/employees/import"},
	} {
		got := h.do(&ro, c.method, c.path, map[string]any{})
		if got.status != http.StatusForbidden {
			t.Errorf("%s %s = %d, want 403 (%s)", c.method, c.path, got.status, got.raw)
		}
	}
	// Baca tetap boleh.
	if got := h.do(&ro, http.MethodGet, "/api/hris/employees/"+e+"/schedule-type", nil); got.status != http.StatusOK {
		t.Errorf("GET schedule-type = %d", got.status)
	}
}

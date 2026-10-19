package hris

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xuri/excelize/v2"

	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/testutil"
)

func getFile(t *testing.T, h *harness, s *testutil.Staff, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := testutil.AsStaff(httptest.NewRequest(http.MethodGet, path, nil), *s)
	rec := httptest.NewRecorder()
	h.exec("SAVEPOINT harness_request")
	h.mux.ServeHTTP(rec, r)
	if _, err := h.tx.Exec(context.Background(), "RELEASE SAVEPOINT harness_request"); err != nil {
		h.exec("ROLLBACK TO SAVEPOINT harness_request")
		h.exec("RELEASE SAVEPOINT harness_request")
	}
	return rec
}

func TestEmployeeExport(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", iam.HrisKepegawaian, false)
	e := h.employee(emp{name: "Ekspor Satu", status: "contract", join: "2025-06-01"})
	h.exec(`UPDATE hris.employees SET nip = '0202505272', ktp = '3273012345678901', gender = 'female', marital_status = 'single', schedule_type = 'non_shift' WHERE id = $1`, e)
	rec := getFile(t, h, &hr, "/api/hris/employees/export?search=Ekspor")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	f, err := excelize.OpenReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := f.GetRows("Data Karyawan")
	if rows[0][1] != "Nama Lengkap *" || rows[1][0] != "0202505272" || rows[1][1] != "Ekspor Satu" {
		t.Fatalf("rows = %v", rows[:2])
	}
	get := func(header string) string {
		for i, h := range rows[0] {
			if h == header || h == header+" *" {
				return rows[1][i]
			}
		}
		return "<missing>"
	}
	checks := map[string]string{"Tanggal Masuk": "01/06/2025", "Status Kepegawaian": "Kontrak (PKWT)", "Tipe Jadwal": "Non-Shift",
		"Jenis Kelamin": "Perempuan", "Status Pernikahan": "Belum Menikah", "NIK (KTP)": "3273012345678901", "Status Aktif": "Aktif"}
	for header, want := range checks {
		if got := get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if list := f.GetSheetList(); list[0] != "Petunjuk" {
		t.Fatalf("first sheet = %v", list)
	}
}

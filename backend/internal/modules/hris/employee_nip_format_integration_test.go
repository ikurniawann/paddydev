package hris

import (
	"context"
	"fmt"
	"testing"

	"nuhabit/backend/internal/platform/employeekeys"
	"nuhabit/backend/internal/platform/nipformat"
)

// nextNIP is the next NIP of the series inside the test transaction, plus
// skip numbers (several new employees in one test).
func (h *harness) nextNIP(join, status string, skip int) string {
	h.t.Helper()
	n, err := employeekeys.NextNIP(context.Background(), h.tx, join, status)
	if err != nil {
		h.t.Fatal(err)
	}
	ym, seq, _ := nipformat.Parse(n)
	return fmt.Sprintf("%s%03d", ym, seq+skip)
}

// EPIC-099: a new employee's NIP is YYYYMM of the join date + the next number.
func TestEmployeeNIPFormat(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuKepegawaian}, false)
	tag := 0
	body := func(nip, join, status string) map[string]any {
		tag++
		return map[string]any{"full_name": "Calon Format", "email": fmt.Sprintf("fmt%d.%s@hris.test", tag, h.nextNIP(join, status, 0)),
			"join_date": join, "employment_status": status, "nip": nip}
	}
	next := h.nextNIP("2026-02-10", "probation", 0)
	_, seq, _ := nipformat.Parse(next)
	expectContains(t, h.do(&hr, "POST", "/api/hris/employees", body(next, "2026-01-10", "probation")), 400, "6 digit depan harus 202601")
	expectContains(t, h.do(&hr, "POST", "/api/hris/employees", body("2026-02-001", "2026-02-10", "probation")), 400, "NIP harus angka")
	ok := h.do(&hr, "POST", "/api/hris/employees", body(next, "2026-02-10", "probation"))
	expect(t, ok, 200, "")
	// The number just used is not reused in another month (202601518 / 202603518).
	expectContains(t, h.do(&hr, "POST", "/api/hris/employees", body(fmt.Sprintf("202607%03d", seq), "2026-07-10", "probation")), 400, "nomor urut terakhir karyawan")
	id := ok.data()["id"].(string)
	dw := h.nextNIP("2026-02-10", "daily_worker", 0)
	expect(t, h.do(&hr, "POST", "/api/hris/employees", body(dw, "2026-02-10", "daily_worker")), 200, "")

	// Edit: a replaced NIP must keep the format; other fields leave an old NIP alone.
	expectContains(t, h.do(&hr, "PUT", "/api/hris/employees/"+id, map[string]any{"nip": "EMP-X"}), 400, "NIP harus angka")
	expectContains(t, h.do(&hr, "PUT", "/api/hris/employees/"+id, map[string]any{"join_date": "2026-03-01", "nip": next[:6] + "999"}), 400, "harus 202603")
	legacy := h.employee(emp{})
	h.exec(`UPDATE hris.employees SET nip = 'EMP-2026-' || substr(md5(random()::text), 1, 6) WHERE id = $1`, legacy)
	expect(t, h.do(&hr, "PUT", "/api/hris/employees/"+legacy, map[string]any{"phone": "081200000077"}), 200, "")
}

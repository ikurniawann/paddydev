package hris

import (
	"strings"
	"testing"
)

// EPIC-063 R9: atasan langsung tidak boleh diri sendiri; laporan baca-saja.
func TestEmployeeCannotReportToSelf(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuKepegawaian}, false)
	plain, _ := h.staff("pos", nil, true)
	loop := h.employee(emp{name: "Loop Uji Atasan"})
	boss := h.employee(emp{name: "Atasan Uji Laporan"})
	const msg = "Atasan langsung tidak boleh karyawan itu sendiri"

	expect(t, h.do(&hr, "PUT", "/api/hris/employees/"+loop, map[string]any{"reporting_to": loop}), 400, msg)
	expect(t, h.do(&hr, "PUT", "/api/hris/employees/"+loop, map[string]any{"reporting_to": strings.ToUpper(loop)}), 400, msg)
	expect(t, h.do(&hr, "PUT", "/api/hris/employees/"+loop, map[string]any{"reporting_to": boss}), 200, "")

	h.exec(`UPDATE hris.employees SET reporting_to = id WHERE id = $1`, loop) // data live lama
	h.employee(emp{name: "Tanpa Atasan Uji"})
	gone := h.employee(emp{name: "Atasan Keluar Uji", inactive: true})
	h.employee(emp{name: "Anak Atasan Keluar", boss: &gone})

	rep := h.do(&hr, "GET", "/api/hris/employees/without-manager", nil)
	expect(t, rep, 200, "")
	reasons := map[string]any{}
	for _, item := range rep.list() {
		row := item.(map[string]any)
		reasons[row["full_name"].(string)] = row["reason"]
	}
	if reasons["Loop Uji Atasan"] != "self" || reasons["Tanpa Atasan Uji"] != "empty" ||
		reasons["Anak Atasan Keluar"] != "inactive" {
		t.Fatalf("reasons = %v", reasons)
	}
	if _, listed := reasons["Atasan Keluar Uji"]; listed {
		t.Fatal("karyawan nonaktif tidak masuk laporan")
	}
	if _, listed := reasons["Atasan Uji Laporan"]; !listed {
		t.Fatal("Atasan Uji Laporan (tanpa atasan) harus terdaftar sebagai empty")
	}
	expect(t, h.do(&plain, "GET", "/api/hris/employees/without-manager", nil), 403, "Insufficient permissions")
}

// Baris live lama yang menunjuk dirinya sendiri tetap bisa menyimpan kolom
// lain; hanya mengirim reporting_to = diri sendiri yang ditolak.
func TestSelfManagerRowStillSavesOtherFields(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuKepegawaian}, false)
	loop := h.employee(emp{name: "Loop Lama Simpan"})
	h.exec(`UPDATE hris.employees SET reporting_to = id WHERE id = $1`, loop)

	expect(t, h.do(&hr, "PUT", "/api/hris/employees/"+loop, map[string]any{"notes": "Catatan baru"}), 200, "")
	expect(t, h.do(&hr, "PUT", "/api/hris/employees/"+loop, map[string]any{"reporting_to": nil}), 200, "")
}

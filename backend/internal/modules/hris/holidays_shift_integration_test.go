package hris

import (
	"net/http"
	"testing"
)

// EPIC-079 R2/R10: HR menetapkan cuti bersama sebagai libur; tipe lain ditolak.
func TestCutiBersamaDayOffToggle(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuKepegawaian}, false)
	const msg = "Hanya cuti bersama yang bisa ditetapkan libur"

	cb := h.do(&hr, http.MethodPost, "/api/hris/holidays", map[string]any{
		"holiday_date": "2031-12-24", "name": "Cuti Bersama Natal", "type": "cuti_bersama"})
	expect(t, cb, http.StatusCreated, "")
	if cb.data()["is_day_off"] != false {
		t.Fatalf("default is_day_off = %v", cb.data()["is_day_off"])
	}
	cbID := cb.data()["id"].(string)

	expect(t, h.do(&hr, http.MethodPatch, "/api/hris/holidays/"+cbID, map[string]any{"is_day_off": true}), http.StatusOK, "")
	if h.scalar(`SELECT is_day_off::text FROM hris.public_holidays WHERE id = $1`, cbID) != "true" {
		t.Fatal("is_day_off not stored")
	}
	if h.scalar(`SELECT deducts_leave::text FROM hris.public_holidays WHERE id = $1`, cbID) != "true" {
		t.Fatal("deducts_leave must keep its cuti bersama default")
	}
	expect(t, h.do(&hr, http.MethodPatch, "/api/hris/holidays/"+cbID, map[string]any{"is_day_off": false}), http.StatusOK, "")
	if h.scalar(`SELECT is_day_off::text FROM hris.public_holidays WHERE id = $1`, cbID) != "false" {
		t.Fatal("is_day_off not cleared")
	}

	nas := h.do(&hr, http.MethodPost, "/api/hris/holidays", map[string]any{
		"holiday_date": "2031-12-25", "name": "Natal", "type": "nasional"})
	expect(t, nas, http.StatusCreated, "")
	nasID := nas.data()["id"].(string)
	expect(t, h.do(&hr, http.MethodPatch, "/api/hris/holidays/"+nasID, map[string]any{"is_day_off": true}), http.StatusBadRequest, msg)
	expect(t, h.do(&hr, http.MethodPost, "/api/hris/holidays", map[string]any{
		"holiday_date": "2031-12-26", "name": "Libur X", "type": "perusahaan", "is_day_off": true}), http.StatusBadRequest, msg)

	// Pindah tipe dari cuti bersama membersihkan penetapan libur.
	expect(t, h.do(&hr, http.MethodPatch, "/api/hris/holidays/"+cbID, map[string]any{"is_day_off": true}), http.StatusOK, "")
	expect(t, h.do(&hr, http.MethodPatch, "/api/hris/holidays/"+cbID, map[string]any{"type": "perusahaan"}), http.StatusOK, "")
	if h.scalar(`SELECT is_day_off::text FROM hris.public_holidays WHERE id = $1`, cbID) != "false" {
		t.Fatal("is_day_off must reset when the type leaves cuti_bersama")
	}

	// Impor memakai aturan yang sama dan tidak menghapus penetapan HR saat diimpor ulang.
	imp := func(extra map[string]any) call {
		item := map[string]any{"holiday_date": "2031-12-27", "name": "Cuti Bersama Impor", "type": "cuti_bersama", "source_ref": "uid-1"}
		for k, v := range extra {
			item[k] = v
		}
		return h.do(&hr, http.MethodPost, "/api/hris/holidays/import", map[string]any{"items": []any{item}})
	}
	expect(t, imp(map[string]any{"is_day_off": true}), http.StatusOK, "")
	if h.scalar(`SELECT is_day_off::text FROM hris.public_holidays WHERE source_ref = 'uid-1'`) != "true" {
		t.Fatal("import must store is_day_off")
	}
	expect(t, imp(nil), http.StatusOK, "")
	if h.scalar(`SELECT is_day_off::text FROM hris.public_holidays WHERE source_ref = 'uid-1'`) != "true" {
		t.Fatal("re-import without is_day_off must keep HR's choice")
	}
	expect(t, h.do(&hr, http.MethodPost, "/api/hris/holidays/import", map[string]any{"items": []any{
		map[string]any{"holiday_date": "2031-12-28", "name": "Natal Impor", "type": "nasional", "is_day_off": true}}}),
		http.StatusBadRequest, msg+": Natal Impor")
}

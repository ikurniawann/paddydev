package hris

import (
	"testing"

	"nuhabit/backend/internal/platform/iam"
)

const rehireHint = " — aktifkan kembali data tersebut dari halaman detail karyawan"

func TestEmployeeKeyConflicts(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", iam.HrisKepegawaian, false)
	a := h.employee(emp{name: "Alya Uji", inactive: true})
	h.exec(`UPDATE hris.employees SET ktp = '3273010199000001', npwp = '12.345.678.9-012.000' WHERE id = $1`, a)
	aNIP := h.scalar(`SELECT nip FROM hris.employees WHERE id = $1`, a)
	b := h.employee(emp{name: "Budi Uji"})
	ownerSuffix := " (NIP " + aNIP + ", nonaktif)" + rehireHint
	newBody := func(extra map[string]any) map[string]any {
		m := map[string]any{"full_name": "Calon Baru", "email": "calon." + a[:6] + "@hris.test", "join_date": "2026-03-01", "employment_status": "probation"}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}

	// Create: KTP in a different format still names the (inactive) owner.
	expect(t, h.do(&hr, "POST", "/api/hris/employees", newBody(map[string]any{"ktp": "3273 0101 9900 0001"})), 400,
		"No. KTP sudah dipakai karyawan Alya Uji"+ownerSuffix)
	c := h.do(&hr, "POST", "/api/hris/employees", newBody(map[string]any{"npwp": "123456789012000"}))
	expect(t, c, 400, "NPWP sudah dipakai karyawan Alya Uji"+ownerSuffix)
	expect(t, h.do(&hr, "POST", "/api/hris/employees", newBody(map[string]any{"nip": aNIP})), 400,
		"NIP sudah dipakai karyawan Alya Uji"+ownerSuffix)
	// Blank keys are no key at all.
	expect(t, h.do(&hr, "POST", "/api/hris/employees", newBody(map[string]any{"ktp": "  ", "npwp": ""})), 200, "")

	// Update: own key is not a conflict; another employee's key is.
	expect(t, h.do(&hr, "PUT", "/api/hris/employees/"+a, map[string]any{"ktp": "3273010199000001"}), 200, "")
	expect(t, h.do(&hr, "PUT", "/api/hris/employees/"+b, map[string]any{"ktp": "3273-0101-9900-0001"}), 400,
		"No. KTP sudah dipakai karyawan Alya Uji"+ownerSuffix)
	expect(t, h.do(&hr, "PUT", "/api/hris/employees/"+b, map[string]any{"npwp": "12.345.678.9-012.000"}), 400,
		"NPWP sudah dipakai karyawan Alya Uji"+ownerSuffix)
	expect(t, h.do(&hr, "PUT", "/api/hris/employees/"+b, map[string]any{"nip": aNIP}), 400,
		"NIP sudah dipakai karyawan Alya Uji"+ownerSuffix)
	// An active owner is named as active, without the rehire hint.
	h.exec(`UPDATE hris.employees SET ktp = '3273010199000002' WHERE id = $1`, b)
	bNIP := h.scalar(`SELECT nip FROM hris.employees WHERE id = $1`, b)
	expect(t, h.do(&hr, "PUT", "/api/hris/employees/"+a, map[string]any{"ktp": "3273010199000002"}), 400,
		"No. KTP sudah dipakai karyawan Budi Uji (NIP "+bNIP+", aktif)")
}

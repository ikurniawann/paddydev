package hris

import (
	"net/http"
	"testing"
)

func TestContractSalaryHiddenWithoutPayrollMenu(t *testing.T) {
	h := newHarness(t)
	ops, _ := h.staff("hrd", []string{menuKepegawaian}, false)
	pay, _ := h.staff("hrd", []string{menuKepegawaian, "hris.compensation"}, false)
	e := h.employee(emp{status: "contract"})
	base := "/api/hris/employees/" + e + "/contracts"
	salaryKeys := []string{"base_salary", "allowances", "compensation_amount"}
	noSalary := func(where string, row map[string]any) {
		t.Helper()
		for _, k := range salaryKeys {
			if _, leaked := row[k]; leaked {
				t.Fatalf("%s leaks %s: %v", where, k, row)
			}
		}
	}

	// Non-Payroll may create; the client's salary is ignored (active salary used).
	created := h.do(&ops, http.MethodPost, base, map[string]any{
		"contract_type": "pkwt", "start_date": "2026-01-01", "end_date": "2027-01-01", "base_salary": 1})
	expect(t, created, 201, "")
	noSalary("create", created.data())
	id := created.data()["id"].(string)

	list := h.do(&ops, http.MethodGet, base, nil)
	expect(t, list, 200, "")
	noSalary("employee contracts", list.list()[0].(map[string]any))
	if list.body["can_view_salary"] != false {
		t.Fatalf("can_view_salary = %v", list.body["can_view_salary"])
	}
	all := h.do(&ops, http.MethodGet, "/api/hris/contracts", nil)
	expect(t, all, 200, "")
	for _, r := range all.list() {
		noSalary("contracts list", r.(map[string]any))
	}

	// Editing the draft with a salary leaves it unchanged.
	expect(t, h.do(&ops, http.MethodPatch, "/api/hris/contracts/"+id,
		map[string]any{"action": "edit", "start_date": "2026-01-01", "base_salary": 2}), 200, "")

	full := h.do(&pay, http.MethodGet, base, nil)
	if got := full.list()[0].(map[string]any)["base_salary"]; got != "4800000.00" || full.body["can_view_salary"] != true {
		t.Fatalf("payroll view base_salary = %v, can_view_salary = %v", got, full.body["can_view_salary"])
	}

	const docMsg = "Dokumen kontrak memuat gaji — hanya untuk akun dengan akses Penggajian"
	expect(t, h.do(&ops, http.MethodGet, "/api/hris/contracts/"+id+"/document", nil), 403, docMsg)
	expect(t, h.do(&ops, http.MethodGet, "/api/hris/contracts/"+id+"/signed-document", nil), 403, docMsg)
	// Payroll passes the guard (the harness has no company settings for the PDF,
	// so the scan route proves it: no scan yet is a 404, not a 403).
	expect(t, h.do(&pay, http.MethodGet, "/api/hris/contracts/"+id+"/signed-document", nil), 404,
		"Kontrak ini belum punya dokumen bertanda tangan")
}

func TestEmploymentHistorySalaryHiddenWithoutPayrollMenu(t *testing.T) {
	h := newHarness(t)
	ops, _ := h.staff("hrd", []string{menuKepegawaian}, false)
	pay, _ := h.staff("hrd", []string{menuKepegawaian, "hris.compensation"}, false)
	e := h.employee(emp{})
	expect(t, h.do(&ops, http.MethodPost, "/api/hris/employment-history", map[string]any{
		"employee_id": e, "change_type": "promotion", "effective_date": "2026-09-01", "new_salary": "6500000", "prev_salary": "5000000",
	}), 201, "")
	url := "/api/hris/employment-history?employee_id=" + e
	row := h.do(&ops, http.MethodGet, url, nil).list()[0].(map[string]any)
	for _, k := range []string{"prev_salary", "new_salary"} {
		if _, leaked := row[k]; leaked {
			t.Fatalf("history leaks %s: %v", k, row)
		}
	}
	if got := h.do(&pay, http.MethodGet, url, nil).list()[0].(map[string]any)["new_salary"]; got != "6500000.00" {
		t.Fatalf("payroll history new_salary = %v", got)
	}
}

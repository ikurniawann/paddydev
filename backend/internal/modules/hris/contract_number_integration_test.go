package hris

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// seqOf is the numeric prefix of "NNNN/TYPE/MONTH/YEAR".
func seqOf(t *testing.T, number string) int {
	t.Helper()
	n, err := strconv.Atoi(strings.SplitN(number, "/", 2)[0])
	if err != nil {
		t.Fatalf("contract number %q: %v", number, err)
	}
	return n
}

// EPIC-075 R6: the next number follows the highest manual number of the
// type this year, not the row count. The staging case: imported ODOO-…
// rows plus a deleted contract made "count + 1" an existing number, so
// every new contract failed with "Data sudah ada di sistem".
func TestContractNumberSkipsTakenNumbers(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuKepegawaian, "hris.compensation"}, false)
	e := h.employee(emp{status: "contract"})
	other := h.employee(emp{status: "contract"})
	count := h.scalar(`SELECT count(*)::text FROM hris.employment_contracts
		WHERE contract_type = 'pkwt' AND date_part('year', created_at) = date_part('year', now())`)
	n, _ := strconv.Atoi(count)
	taken := fmt.Sprintf("%04d/PKWT/X/2026", n+3) // two rows go in below: the old rule's count+1
	h.exec(`INSERT INTO hris.employment_contracts (employee_id, contract_number, contract_type, start_date, end_date)
		VALUES ($1, $2, 'pkwt', '2026-01-01', '2026-12-31'),
		       ($1, 'ODOO-99999/PKWT/X/2026', 'pkwt', '2026-01-01', '2026-12-31')`, other, taken)

	base := "/api/hris/employees/" + e + "/contracts"
	body := map[string]any{"contract_type": "pkwt", "start_date": "2026-11-01", "end_date": "2027-04-30"}
	first := h.do(&hr, "POST", base, body)
	expect(t, first, 201, "")
	got := first.data()["contract_number"].(string)
	if !strings.HasSuffix(got, "/PKWT/X/2026") || seqOf(t, got) <= n+3 || seqOf(t, got) >= 99999 {
		t.Fatalf("first number = %q (taken %q)", got, taken)
	}
	second := h.do(&hr, "POST", base, body)
	expect(t, second, 201, "")
	if next := second.data()["contract_number"].(string); seqOf(t, next) != seqOf(t, got)+1 {
		t.Fatalf("second number = %q after %q", next, got)
	}
}

// Review #2: one odd row with a huge prefix must not break numbering for the
// rest of the year (an int overflow made every new contract a 500).
func TestContractNumberIgnoresHugePrefix(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuKepegawaian, "hris.compensation"}, false)
	e := h.employee(emp{status: "contract"})
	h.exec(`INSERT INTO hris.employment_contracts (employee_id, contract_number, contract_type, start_date, end_date)
		VALUES ($1, '99999999999/PKWT/X/2026', 'pkwt', '2026-01-01', '2026-12-31')`, e)
	expect(t, h.do(&hr, "POST", "/api/hris/employees/"+e+"/contracts",
		map[string]any{"contract_type": "pkwt", "start_date": "2026-11-01", "end_date": "2027-04-30"}), 201, "")
}

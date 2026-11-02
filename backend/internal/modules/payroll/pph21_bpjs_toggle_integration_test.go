package payroll

import (
	"encoding/json"
	"math"
	"testing"
)

// EPIC-082: saklar PPh 21, BPJS TK dan BPJS Kesehatan di Pengaturan Payroll
// dibaca saat run dihitung; run yang sudah disetujui tidak ikut berubah.

var toggleDeductionCols = []string{"pph21_deduction", "bpjs_tk_jht_deduction", "bpjs_tk_jp_deduction", "bpjs_kes_deduction"}
var toggleEmployerCols = []string{"bpjs_tk_jht_employer", "bpjs_tk_jp_employer", "bpjs_tk_jkk_employer", "bpjs_tk_jkm_employer",
	"bpjs_kes_employer"}

// detailValues reads the toggled columns of the one detail of a run.
func (e *env) detailValues(run, employee string) map[string]float64 {
	e.t.Helper()
	out := map[string]float64{}
	for _, col := range append(append([]string{}, toggleDeductionCols...), toggleEmployerCols...) {
		var v float64
		e.scalar(&v, `SELECT `+col+`::float8 FROM hris.payroll_details WHERE payroll_run_id = $1 AND employee_id = $2`, run, employee)
		out[col] = v
	}
	return out
}

func (e *env) putToggles(pph, tk, kes bool) {
	e.t.Helper()
	e.call(&e.hr, "PUT", "/api/hris/payroll-settings", map[string]any{"settings": map[string]any{
		"pph21_enabled": pph, "bpjs_tk_enabled": tk, "bpjs_kes_enabled": kes}}, 200)
}

func TestPayrollToggleSettingsAndRun(t *testing.T) {
	e := setup(t)
	e.employee("HR Payroll", e.hr.UserID, true)
	emp := e.employee("Karyawan Saklar", "", true)
	e.exec(`INSERT INTO hris.employee_salary (employee_id, company_id, base_salary, fixed_allowance, ptkp_status, effective_date)
		VALUES ($1, $2, 10000000, 2000000, 'TK/0', '2026-01-01')`, emp, e.co)

	// Baseline: tanpa baris pengaturan semua saklar menyala. Run Juni dihitung
	// lalu disetujui, jadi angkanya tersimpan sebelum saklar diubah.
	approved := e.newRun(6, 2026, e.co)
	e.call(&e.hr, "POST", "/api/hris/payroll/"+approved+"/calculate", nil, 200)
	baseline := e.detailValues(approved, emp)
	for _, col := range append(append([]string{}, toggleDeductionCols...), toggleEmployerCols...) {
		if col == "bpjs_tk_jkm_employer" || col == "bpjs_tk_jkk_employer" {
			continue // tarif kecil, tidak diasersi positif di sini
		}
		if baseline[col] <= 0 {
			t.Fatalf("baseline %s = %v, want > 0 (%v)", col, baseline[col], baseline)
		}
	}
	e.call(&e.hr, "PUT", "/api/hris/payroll/"+approved, map[string]any{"status": "processing"}, 200)
	e.call(&e.hr, "PUT", "/api/hris/payroll/"+approved, map[string]any{"status": "completed"}, 200)

	// PUT mematikan ketiganya; GET mengembalikannya.
	e.putToggles(false, false, false)
	got := e.call(&e.hr, "GET", "/api/hris/payroll-settings?tax_year=2026", nil, 200)["data"].(map[string]any)["settings"].(map[string]any)
	for _, k := range []string{"pph21_enabled", "bpjs_tk_enabled", "bpjs_kes_enabled"} {
		if v, ok := got[k].(bool); !ok || v {
			t.Fatalf("GET %s = %v, want false", k, got[k])
		}
	}

	// Run draf baru: PPh 21 dan seluruh BPJS (karyawan + perusahaan) nol.
	draft := e.newRun(7, 2026, e.co)
	e.call(&e.hr, "POST", "/api/hris/payroll/"+draft+"/calculate", nil, 200)
	for col, v := range e.detailValues(draft, emp) {
		if v != 0 {
			t.Errorf("saklar mati: %s = %v, want 0", col, v)
		}
	}
	var method string
	e.scalar(&method, `SELECT pph21_method FROM hris.payroll_details WHERE payroll_run_id = $1 AND employee_id = $2`, draft, emp)
	if method != "nonaktif" {
		t.Errorf("pph21_method = %q, want nonaktif", method)
	}

	// Run yang sudah disetujui tidak berubah dan tidak bisa dihitung ulang.
	for col, v := range e.detailValues(approved, emp) {
		if math.Abs(v-baseline[col]) > 0.005 {
			t.Errorf("run disetujui berubah: %s = %v, want %v", col, v, baseline[col])
		}
	}
	e.call(&e.hr, "POST", "/api/hris/payroll/"+approved+"/calculate", nil, 400)

	// Saklar dinyalakan lagi: hitung ulang draf sama dengan baseline.
	e.putToggles(true, true, true)
	e.call(&e.hr, "POST", "/api/hris/payroll/"+draft+"/calculate", nil, 200)
	for col, v := range e.detailValues(draft, emp) {
		if math.Abs(v-baseline[col]) > 0.005 {
			t.Errorf("saklar nyala lagi: %s = %v, want %v", col, v, baseline[col])
		}
	}
	for col, v := range e.detailValues(approved, emp) {
		if math.Abs(v-baseline[col]) > 0.005 {
			t.Errorf("run disetujui berubah setelah saklar nyala: %s = %v, want %v", col, v, baseline[col])
		}
	}

	// Audit: satu baris payroll_settings.update per perubahan, hanya kunci yang berubah.
	type auditRow struct{ before, after map[string]any }
	read := func(afterPph bool) auditRow {
		var before, after *string
		var actor *string
		e.scalar(&before, `SELECT before::text FROM audit.audit_log WHERE action = 'payroll_settings.update'
			AND (after->>'pph21_enabled')::boolean = $1`, afterPph)
		e.scalar(&after, `SELECT after::text FROM audit.audit_log WHERE action = 'payroll_settings.update'
			AND (after->>'pph21_enabled')::boolean = $1`, afterPph)
		e.scalar(&actor, `SELECT actor_id::text FROM audit.audit_log WHERE action = 'payroll_settings.update'
			AND (after->>'pph21_enabled')::boolean = $1`, afterPph)
		if actor == nil || *actor != e.hr.UserID {
			t.Errorf("audit actor = %v, want %s", actor, e.hr.UserID)
		}
		var r auditRow
		if before != nil {
			_ = json.Unmarshal([]byte(*before), &r.before)
		}
		_ = json.Unmarshal([]byte(*after), &r.after)
		return r
	}
	off, on := read(false), read(true)
	if len(off.after) != 3 || off.after["bpjs_tk_enabled"] != false {
		t.Errorf("audit mati after = %v", off.after)
	}
	if len(on.before) != 3 || on.before["pph21_enabled"] != false || len(on.after) != 3 || on.after["bpjs_kes_enabled"] != true {
		t.Errorf("audit nyala before=%v after=%v", on.before, on.after)
	}

	// PUT tanpa perubahan nilai tidak menulis audit baru.
	var n int
	e.scalar(&n, `SELECT count(*) FROM audit.audit_log WHERE action = 'payroll_settings.update'`)
	e.putToggles(true, true, true)
	var n2 int
	e.scalar(&n2, `SELECT count(*) FROM audit.audit_log WHERE action = 'payroll_settings.update'`)
	if n2 != n {
		t.Errorf("PUT tanpa perubahan menulis audit: %d -> %d", n, n2)
	}
}

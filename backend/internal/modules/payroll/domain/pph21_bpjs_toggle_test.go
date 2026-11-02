package domain

import (
	"reflect"
	"testing"
)

// EPIC-082: saklar global PPh 21, BPJS TK dan BPJS Kesehatan. Saklar mati
// mengalahkan kepesertaan per struktur gaji; subsidi BPJS Kesehatan (tunjangan)
// tidak ikut berubah.

func toggleInput(mod func(*Input)) Input {
	return parityInput(func(in *Input) {
		in.BaseSalary = 5_000_000
		in.PTKPStatus, in.IsTaxable = "TK/0", true
		in.BpjsTkEnrolled, in.BpjsKesEnrolled = true, true
		in.HealthAllowance, in.HealthEligible = 100_000, true // subsidi, masa kerja > 1 tahun
		if mod != nil {
			mod(in)
		}
	})
}

func allToggles(pph, tk, kes bool) Config {
	c := DefaultConfig()
	c.Pph21Enabled, c.BpjsTkEnabled, c.BpjsKesEnabled = pph, tk, kes
	return c
}

func TestToggleOffZeroesTaxAndBpjs(t *testing.T) {
	for _, method := range []string{"ter", "annualized"} {
		t.Run(method, func(t *testing.T) {
			c := allToggles(false, false, false)
			c.Pph21Method = method
			in := toggleInput(func(in *Input) { in.LoanDeduction = 50_000 })
			r := Calculate(in, c)
			eq(t, "PPh 21", r.Pph21Deduction, 0)
			eq(t, "JHT karyawan", r.BpjsTkJhtDeduction, 0)
			eq(t, "JP karyawan", r.BpjsTkJpDeduction, 0)
			eq(t, "Kes karyawan", r.BpjsKesDeduction, 0)
			eq(t, "JHT perusahaan", r.BpjsTkJhtEmployer, 0)
			eq(t, "JP perusahaan", r.BpjsTkJpEmployer, 0)
			eq(t, "JKK perusahaan", r.BpjsTkJkkEmployer, 0)
			eq(t, "JKM perusahaan", r.BpjsTkJkmEmployer, 0)
			eq(t, "Kes perusahaan", r.BpjsKesEmployer, 0)
			eq(t, "total iuran perusahaan", r.TotalEmployerContribution, 0)
			eq(t, "taxable", r.TaxableIncome, 0)
			eq(t, "dpp", r.Pph21Dpp, 0)
			eq(t, "ter rate", r.TERRate, 0)
			eq(t, "pph annual", r.Pph21Annual, 0)
			eq(t, "pph monthly", r.Pph21Monthly, 0)
			if r.TERCategory != "" {
				t.Errorf("TERCategory = %q, want kosong", r.TERCategory)
			}
			if r.Pph21Method != "nonaktif" {
				t.Errorf("Pph21Method = %q, want nonaktif", r.Pph21Method)
			}
			eq(t, "subsidi BPJS Kes tetap", r.HealthAllowance, 100_000)
			eq(t, "PTKP tetap dicatat", r.PTKPAmount, 54_000_000)
			eq(t, "bruto", r.GrossSalary, 5_100_000)
			eq(t, "total potongan = potongan lain saja", r.TotalDeductions, 50_000)
			eq(t, "THP = bruto - potongan lain", r.NetSalary, r.GrossSalary-50_000)
		})
	}
}

func TestToggleOnlyKesEnabled(t *testing.T) {
	c := allToggles(false, false, true)
	r := Calculate(toggleInput(nil), c)
	eq(t, "Kes = 1% upah", r.BpjsKesDeduction, 50_000)
	eq(t, "Kes perusahaan", r.BpjsKesEmployer, 200_000)
	eq(t, "JHT", r.BpjsTkJhtDeduction, 0)
	eq(t, "JP", r.BpjsTkJpDeduction, 0)
	eq(t, "TK perusahaan", r.BpjsTkJhtEmployer+r.BpjsTkJpEmployer+r.BpjsTkJkkEmployer+r.BpjsTkJkmEmployer, 0)
	eq(t, "total iuran perusahaan = Kes", r.TotalEmployerContribution, 200_000)
	eq(t, "PPh", r.Pph21Deduction, 0)
	eq(t, "THP", r.NetSalary, r.GrossSalary-50_000)
}

func TestToggleOnlyKesEnabledCapsAtMaxUpah(t *testing.T) {
	c := allToggles(false, false, true)
	r := Calculate(toggleInput(func(in *Input) { in.BaseSalary = 20_000_000 }), c)
	eq(t, "Kes = 1% × plafon", r.BpjsKesDeduction, 120_000)
}

func TestToggleOnlyTkEnabled(t *testing.T) {
	c := allToggles(false, true, false)
	r := Calculate(toggleInput(nil), c)
	eq(t, "JHT", r.BpjsTkJhtDeduction, 100_000)
	eq(t, "JP", r.BpjsTkJpDeduction, 50_000)
	eq(t, "Kes", r.BpjsKesDeduction, 0)
	eq(t, "Kes perusahaan", r.BpjsKesEmployer, 0)
	eq(t, "PPh", r.Pph21Deduction, 0)
}

func TestTogglePphOffOnlyKeepsBpjs(t *testing.T) {
	c := allToggles(false, true, true)
	r := Calculate(toggleInput(nil), c)
	eq(t, "PPh", r.Pph21Deduction, 0)
	if r.BpjsTkJhtDeduction == 0 || r.BpjsKesDeduction == 0 {
		t.Errorf("BPJS harus tetap dipotong: JHT=%v Kes=%v", r.BpjsTkJhtDeduction, r.BpjsKesDeduction)
	}
}

func TestToggleBpjsOffPphStillComputed(t *testing.T) {
	c := allToggles(true, false, false)
	c.Pph21Method = "annualized"
	in := toggleInput(func(in *Input) { in.BaseSalary = 15_000_000 })
	r := Calculate(in, c)
	if r.Pph21Deduction <= 0 {
		t.Fatalf("PPh 21 harus tetap dihitung, got %v", r.Pph21Deduction)
	}
	if r.Pph21Method != "annualized" {
		t.Errorf("Pph21Method = %q, want annualized", r.Pph21Method)
	}
}

func TestToggleOffDecemberPphZero(t *testing.T) {
	for _, method := range []string{"ter", "annualized"} {
		c := allToggles(false, true, true)
		c.Pph21Method = method
		in := toggleInput(func(in *Input) {
			in.PeriodMonth, in.BaseSalary = 12, 15_000_000
			in.Ytd = YearToDate{Months: 11, Bruto: 165_000_000, PensionEmployee: 3_000_000, Pph21: 0}
		})
		r := Calculate(in, c)
		eq(t, method+" PPh Desember", r.Pph21Deduction, 0)
		eq(t, method+" PPh tahunan", r.Pph21Annual, 0)
		if r.Pph21Method != "nonaktif" {
			t.Errorf("%s Pph21Method = %q, want nonaktif", method, r.Pph21Method)
		}
		// Kontrol: input yang sama dengan PPh 21 menyala menghasilkan PPh > 0,
		// jadi nol di atas memang berasal dari saklar.
		onCfg := c
		onCfg.Pph21Enabled = true
		on := Calculate(in, onCfg)
		if on.Pph21Deduction <= 0 || on.Pph21Method != method {
			t.Errorf("%s kontrol: PPh Desember saat aktif harus > 0 (method %s), got %v (method %s)", method, method, on.Pph21Deduction, on.Pph21Method)
		}
	}
}

func TestToggleNotEnrolledStaysZero(t *testing.T) {
	c := allToggles(true, true, true)
	r := Calculate(toggleInput(func(in *Input) {
		in.BpjsTkEnrolled, in.BpjsKesEnrolled, in.IsTaxable = false, false, false
	}), c)
	eq(t, "JHT", r.BpjsTkJhtDeduction, 0)
	eq(t, "JP", r.BpjsTkJpDeduction, 0)
	eq(t, "Kes", r.BpjsKesDeduction, 0)
	eq(t, "iuran perusahaan", r.TotalEmployerContribution, 0)
	eq(t, "PPh", r.Pph21Deduction, 0)
}

// R3: Config dengan ketiga saklar menyala sama persis dengan DefaultConfig()
// (yang saklarnya menyala) untuk berbagai bentuk input. Angka mutlak sebelum
// EPIC ini dijaga oleh TestToggleOnGoldenPreEpic.
func TestToggleAllOnEqualsDefaultConfig(t *testing.T) {
	inputs := map[string]Input{
		"gaji kecil": toggleInput(nil),
		"gaji besar": toggleInput(func(in *Input) { in.BaseSalary = 40_000_000 }),
		"desember": toggleInput(func(in *Input) {
			in.PeriodMonth = 12
			in.Ytd = YearToDate{Months: 11, Bruto: 55_000_000, Pph21: 100_000}
		}),
		"tidak ikut":    toggleInput(func(in *Input) { in.BpjsTkEnrolled, in.BpjsKesEnrolled, in.IsTaxable = false, false, false }),
		"lembur+bonus":  toggleInput(func(in *Input) { in.OvertimeHours, in.Bonus = 10, 500_000 }),
		"tapera":        toggleInput(func(in *Input) { in.TaperaEnrolled = true }),
		"menikah anak2": toggleInput(func(in *Input) { in.PTKPStatus, in.BaseSalary = "K/2", 12_000_000 }),
	}
	for name, in := range inputs {
		for _, method := range []string{"ter", "annualized"} {
			def := DefaultConfig()
			def.Pph21Method = method
			on := allToggles(true, true, true)
			on.Pph21Method = method
			if got, want := Calculate(in, on), Calculate(in, def); !reflect.DeepEqual(got, want) {
				t.Errorf("%s/%s: saklar menyala beda dari default\n got  %+v\n want %+v", name, method, got, want)
			}
			if r := Calculate(in, def); r.Pph21Method == "nonaktif" {
				t.Errorf("%s/%s: default tidak boleh nonaktif", name, method)
			}
		}
	}
	d := DefaultConfig()
	if !d.Pph21Enabled || !d.BpjsTkEnabled || !d.BpjsKesEnabled {
		t.Errorf("DefaultConfig: semua saklar harus true: %+v", d)
	}
}

// R3: angka emas dihitung dengan calculator.go commit bba2b166 (sebelum EPIC-082,
// belum punya saklar) pada input yang sama; saklar menyala harus menghasilkan
// angka yang sama persis.
func TestToggleOnGoldenPreEpic(t *testing.T) {
	in := Input{
		EmployeeID: "emp-1", PeriodMonth: 9, PeriodYear: 2026,
		BaseSalary: 15_000_000, FixedAllowance: 1_000_000, WorkingDays: 26, JoinDate: "2024-01-15",
		EmploymentStatus: "contract", PTKPStatus: "K/1", IsTaxable: true,
		BpjsTkEnrolled: true, BpjsKesEnrolled: true,
		PayBasis: "monthly", OvertimeScheme: "standard",
	}
	cases := []struct {
		method                     string
		pph, taxable, terRate, net float64
		category                   string
	}{
		{"ter", 1_229_358, 17_562_254, 0.07, 14_338_222, "B"},
		{"annualized", 972_625, 180_810_960, 0, 14_594_955, ""},
	}
	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			c := allToggles(true, true, true)
			c.Pph21Method = tc.method
			r := Calculate(in, c)
			eq(t, "PPh 21", r.Pph21Deduction, tc.pph)
			eq(t, "taxable", r.TaxableIncome, tc.taxable)
			eq(t, "TER rate", r.TERRate, tc.terRate)
			eq(t, "JHT", r.BpjsTkJhtDeduction, 208_280)
			eq(t, "JP", r.BpjsTkJpDeduction, 104_140)
			eq(t, "Kes", r.BpjsKesDeduction, 120_000)
			eq(t, "iuran perusahaan", r.TotalEmployerContribution, 1_129_834)
			eq(t, "bruto", r.GrossSalary, 16_000_000)
			eq(t, "THP", r.NetSalary, tc.net)
			if r.TERCategory != tc.category || r.Pph21Method != tc.method {
				t.Errorf("category/method = %q/%q, want %q/%q", r.TERCategory, r.Pph21Method, tc.category, tc.method)
			}
		})
	}
}

func TestBuildConfigToggleDefaults(t *testing.T) {
	c := BuildConfig(Row{}, Row{})
	if !c.Pph21Enabled || !c.BpjsTkEnabled || !c.BpjsKesEnabled {
		t.Errorf("baris kosong: semua saklar harus true: %+v", c)
	}
	c = BuildConfig(Row{"pph21_enabled": false}, Row{})
	if c.Pph21Enabled || !c.BpjsTkEnabled || !c.BpjsKesEnabled {
		t.Errorf("pph21_enabled=false: hanya PPh mati: %v %v %v", c.Pph21Enabled, c.BpjsTkEnabled, c.BpjsKesEnabled)
	}
	c = BuildConfig(Row{"bpjs_tk_enabled": false, "bpjs_kes_enabled": false}, nil)
	if !c.Pph21Enabled || c.BpjsTkEnabled || c.BpjsKesEnabled {
		t.Errorf("TK/Kes mati: %v %v %v", c.Pph21Enabled, c.BpjsTkEnabled, c.BpjsKesEnabled)
	}
	c = BuildConfig(nil, nil)
	if !c.Pph21Enabled || !c.BpjsTkEnabled || !c.BpjsKesEnabled {
		t.Errorf("tanpa baris: semua saklar harus true: %+v", c)
	}
}

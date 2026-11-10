package nipformat

import (
	"strings"
	"testing"
)

var data = []string{"202512507", "202601002", "202611095", "202611676", "202610655",
	"2026-05-594", "EMP-2026-00001", "000000", "201601001"}

func TestParse(t *testing.T) {
	cases := []struct {
		nip string
		ym  string
		seq int
		ok  bool
	}{
		{"202611677", "202611", 677, true},
		{"2028011000", "202801", 1000, true},
		{"202611000", "", 0, false},
		{"20261167", "", 0, false},
		{"202613001", "", 0, false},
		{"2026110677", "", 0, false},
		{"2026-11-677", "", 0, false},
		{"EMP-1", "", 0, false},
		{" 202611677", "", 0, false},
	}
	for _, c := range cases {
		ym, seq, ok := Parse(c.nip)
		if ok != c.ok || (ok && (ym != c.ym || seq != c.seq)) {
			t.Errorf("Parse(%q) = (%q,%d,%v), want (%q,%d,%v)", c.nip, ym, seq, ok, c.ym, c.seq, c.ok)
		}
	}
}

func TestSeriesFor(t *testing.T) {
	for _, s := range []string{"daily_worker", "freelance", "intern", "internship"} {
		if SeriesFor(s) != Harian {
			t.Errorf("SeriesFor(%q) = %q, want harian", s, SeriesFor(s))
		}
	}
	for _, s := range []string{"probation", "contract", "permanent", "pkwt", ""} {
		if SeriesFor(s) != Main {
			t.Errorf("SeriesFor(%q) = %q, want main", s, SeriesFor(s))
		}
	}
}

func TestLastFor(t *testing.T) {
	if got, want := LastFor(data, 2026), (Last{Main: 676, Harian: 95}); got != want {
		t.Errorf("LastFor(data, 2026) = %+v, want %+v", got, want)
	}
	if got, want := LastFor(data, 2027), (Last{Main: 676, Harian: 0}); got != want {
		t.Errorf("LastFor(data, 2027) = %+v, want %+v", got, want)
	}
	if got, want := LastFor([]string{"202611005"}, 2026), (Last{Main: 5, Harian: 5}); got != want {
		t.Errorf("LastFor(single, 2026) = %+v, want %+v", got, want)
	}
}

// A back-dated main hire (join Dec 2025, entered in 2026) must not move the
// harian series of 2026 (final review, ruling: harian 001–499, main ≥ 500).
func TestBackdatedMainKeepsHarian(t *testing.T) {
	nips := append(append([]string{}, data...), "202512677")
	l := LastFor(nips, 2026)
	if want := (Last{Main: 677, Harian: 95}); l != want {
		t.Fatalf("LastFor = %+v, want %+v", l, want)
	}
	if msg := CheckNew("202611096", "2026-11-10", "daily_worker", l); msg != "" {
		t.Fatalf("daily worker refused after a back-dated hire: %q", msg)
	}
	if msg := CheckNew("202611678", "2026-11-10", "probation", l); msg != "" {
		t.Fatalf("main refused: %q", msg)
	}
}

func TestMainStartsAt500(t *testing.T) {
	l := LastFor([]string{"202611005"}, 2026)
	if got := Next("2026-11-10", Main, l); got != "202611500" {
		t.Errorf("Next main in a young database = %q, want 202611500", got)
	}
	if got := Next("2026-11-10", Harian, l); got != "202611006" {
		t.Errorf("Next harian = %q", got)
	}
	want := "NIP 202611006: nomor urut karyawan mulai 500 (001–499 untuk Daily Worker/Freelance/Magang) — mis. 202611500"
	if msg := CheckNew("202611006", "2026-11-10", "probation", l); msg != want {
		t.Errorf("got %q\nwant %q", msg, want)
	}
}

func TestNext(t *testing.T) {
	l := LastFor(data, 2026)
	if got := Next("2026-11-10", Main, l); got != "202611677" {
		t.Errorf("Next main = %q", got)
	}
	if got := Next("2026-11-10", Harian, l); got != "202611096" {
		t.Errorf("Next harian = %q", got)
	}
	if got := Next("2028-01-03", Main, Last{Main: 999}); got != "2028011000" {
		t.Errorf("Next after 999 = %q", got)
	}
}

func TestCheckNew(t *testing.T) {
	l := LastFor(data, 2026)
	for _, c := range [][3]string{
		{"202611677", "2026-11-10", "probation"},
		{"202611700", "2026-11-10", "contract"},
		{"202611096", "2026-11-10", "daily_worker"},
	} {
		if msg := CheckNew(c[0], c[1], c[2], l); msg != "" {
			t.Errorf("CheckNew(%v) = %q, want accepted", c, msg)
		}
	}
	if msg := CheckNew("202512677", "2025-12-15", "probation", LastFor(data, 2025)); msg != "" {
		t.Errorf("last-year join date: %q", msg)
	}
	refused := []struct{ nip, join, status, want string }{
		{"2026-11-677", "2026-11-10", "probation", "NIP harus angka: 6 digit tahun-bulan join date + 3 digit nomor urut, mis. 202611677"},
		{"202610677", "2026-11-10", "probation", "NIP 202610677: 6 digit depan harus 202611 (tahun-bulan join date)"},
		{"202611650", "2026-11-10", "probation", "NIP 202611650: nomor urut terakhir karyawan 676 — pakai nomor berikutnya, mis. 202611677"},
		{"202611090", "2026-11-10", "freelance", "NIP 202611090: nomor urut terakhir Daily Worker/Freelance/Magang 2026 adalah 095 — pakai nomor berikutnya, mis. 202611096"},
		{"202611700", "2026-11-10", "intern", "NIP 202611700: nomor urut Daily Worker/Freelance/Magang 2026 maksimal 499 — mis. 202611096"},
	}
	for _, c := range refused {
		if msg := CheckNew(c.nip, c.join, c.status, l); msg != c.want {
			t.Errorf("CheckNew(%q,%q,%q)\n got %q\nwant %q", c.nip, c.join, c.status, msg, c.want)
		}
	}
}

func TestCheckChanged(t *testing.T) {
	if msg := CheckChanged("202603518", "2026-03-09"); msg != "" {
		t.Errorf("matching prefix: %q", msg)
	}
	if msg := CheckChanged("202603518", "2026-07-21"); !strings.Contains(msg, "harus 202607") {
		t.Errorf("prefix mismatch: %q", msg)
	}
	if msg := CheckChanged("EMP-9", "2026-07-21"); !strings.HasPrefix(msg, "NIP harus angka") {
		t.Errorf("format: %q", msg)
	}
	if msg := CheckChanged("202603001", "2026-03-09"); msg != "" {
		t.Errorf("sequence is not checked on change: %q", msg)
	}
}

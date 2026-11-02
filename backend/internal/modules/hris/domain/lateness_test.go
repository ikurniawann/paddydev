package domain

import "testing"

func TestLatenessLevelFor(t *testing.T) {
	cases := map[int]string{-1: "none", 0: "none", 1: "teguran", 2: "teguran", 3: "sp1", 4: "sp1",
		5: "sp2", 6: "sp2", 7: "sp3", 30: "sp3"}
	for count, want := range cases {
		if got := LatenessLevelFor(count).Code; got != want {
			t.Errorf("LatenessLevelFor(%d) = %s, want %s", count, got, want)
		}
	}
	if l := LatenessLevelFor(7); l.Label != "Surat Peringatan 3 dan/atau PHK" || l.Max != nil {
		t.Errorf("sp3 = %+v", l)
	}
}

func TestNextLatenessLevel(t *testing.T) {
	cases := []struct {
		count, remaining int
		code             string
	}{{0, 1, "teguran"}, {2, 1, "sp1"}, {3, 2, "sp2"}, {6, 1, "sp3"}, {-4, 1, "teguran"}}
	for _, c := range cases {
		l, remaining, ok := NextLatenessLevel(c.count)
		if !ok || l.Code != c.code || remaining != c.remaining {
			t.Errorf("NextLatenessLevel(%d) = %s, %d, %v", c.count, l.Code, remaining, ok)
		}
	}
	if _, _, ok := NextLatenessLevel(7); ok {
		t.Error("sp3 has no next level")
	}
}

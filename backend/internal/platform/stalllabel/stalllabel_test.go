package stalllabel

import (
	"strings"
	"testing"
)

type outlet struct {
	id, name, branchID, branch string
	main                       bool
}

func key(o outlet) Key { return Key{BranchName: o.branch, BranchID: o.branchID, IsDefault: o.main} }

func ids(os []outlet) string {
	var s []string
	for _, o := range os {
		s = append(s, o.id)
	}
	return strings.Join(s, " ")
}

func TestLabelAndSpans(t *testing.T) {
	branchID := func(o outlet) string { return o.branchID }
	a1 := outlet{name: "Main Storage", branchID: "a", branch: "Alpha"}
	a2 := outlet{name: "Stall 1", branchID: "a", branch: "Alpha"}
	b1 := outlet{name: "Main Storage", branchID: "b", branch: "Beta"}
	if Spans(nil, branchID) || Spans([]outlet{a1}, branchID) || Spans([]outlet{a1, a2}, branchID) || !Spans([]outlet{a1, b1}, branchID) {
		t.Fatal("Spans")
	}
	for _, c := range []struct {
		name, branch string
		with         bool
		want         string
	}{
		{"Main Storage", "Alpha", false, "Main Storage"},
		{"Main Storage", "Alpha", true, "Main Storage — Alpha"},
		{"Kiosk", "  ", true, "Kiosk"},
		{"Kiosk", "", true, "Kiosk"},
	} {
		if got := Label(c.name, c.branch, c.with); got != c.want {
			t.Errorf("Label(%q,%q,%v)=%q want %q", c.name, c.branch, c.with, got, c.want)
		}
	}
}

func TestSortByBranch(t *testing.T) {
	in := []outlet{
		{id: "b-s", name: "Stall 1", branchID: "2", branch: "Bandung"},
		{id: "a-s", name: "Stall 1", branchID: "1", branch: "alpha"},
		{id: "b-m", name: "Main Storage", branchID: "2", branch: "Bandung", main: true},
		{id: "a-m", name: "Main Storage", branchID: "1", branch: "alpha", main: true},
		{id: "c-s2", name: "Stall 10", branchID: "3", branch: "Cirebon"},
		{id: "c-s1", name: "Stall 2", branchID: "3", branch: "Cirebon"},
	}
	got := SortByBranch(in, key, nil)
	// Branch (case-insensitive), then default first, then input order.
	if want := "a-m a-s b-m b-s c-s2 c-s1"; ids(got) != want {
		t.Fatalf("got %s want %s", ids(got), want)
	}
	if ids(in) != "b-s a-s b-m a-m c-s2 c-s1" {
		t.Fatal("input must not be mutated")
	}
	// Same-named branches stay grouped by id.
	dup := SortByBranch([]outlet{
		{id: "x2", branchID: "2", branch: "Dup", main: true},
		{id: "x1s", branchID: "1", branch: "Dup"},
		{id: "x1", branchID: "1", branch: "Dup", main: true},
	}, key, nil)
	if want := "x1 x1s x2"; ids(dup) != want {
		t.Fatalf("dup: got %s want %s", ids(dup), want)
	}
	// A custom within-branch order wins over the default-first tie-break.
	byName := SortByBranch(in, key, func(a, b outlet) int { return NaturalCompare(a.name, b.name) })
	if want := "a-m a-s b-m b-s c-s1 c-s2"; ids(byName) != want {
		t.Fatalf("within: got %s want %s", ids(byName), want)
	}
}

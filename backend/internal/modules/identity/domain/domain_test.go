package domain

import (
	"strings"
	"testing"
)

// Ported from frontend/src/lib/auth/login-throttle.test.ts.
func TestNormalizeAccountKey(t *testing.T) {
	if got := NormalizeAccountKey("  Admin@Bcdcoffee.ID "); got != "admin@bcdcoffee.id" {
		t.Fatalf("got %q", got)
	}
	if got := NormalizeAccountKey(strings.Repeat("a", 500)); len(got) != 254 {
		t.Fatalf("len %d", len(got))
	}
}

func TestShouldBlockLogin(t *testing.T) {
	if ShouldBlockLogin(LoginMaxPerAccount-1, 0) || !ShouldBlockLogin(LoginMaxPerAccount, 0) {
		t.Fatal("per-account limit")
	}
	if LoginMaxPerIP <= LoginMaxPerAccount*5 {
		t.Fatal("the IP limit must stay much looser (cashiers share a NAT)")
	}
	if ShouldBlockLogin(0, LoginMaxPerIP-1) || !ShouldBlockLogin(0, LoginMaxPerIP) {
		t.Fatal("per-IP limit")
	}
}

func TestChangePasswordProblem(t *testing.T) {
	for _, c := range []struct{ current, next, want string }{
		{"", "x", "Current password and new password are required"},
		{"old-pass", "", "Current password and new password are required"},
		{"old-pass", "1234567", "New password must be at least 8 characters"},
		// Eight UTF-16 units, fewer bytes than code points would suggest.
		{"old-pass", "😀😀😀😀", ""},
		{"old-pass", "old-pass", "New password must be different from the current password"},
		{"old-pass", "new-pass", ""},
	} {
		if got := ChangePasswordProblem(c.current, c.next); got != c.want {
			t.Errorf("(%q, %q) = %q, want %q", c.current, c.next, got, c.want)
		}
	}
}

func TestStallRules(t *testing.T) {
	if !StallAllAccess("super_admin", false, false) || !StallAllAccess("pos", true, false) ||
		!StallAllAccess("pos", false, true) || StallAllAccess("admin", false, false) {
		t.Fatal("computeStallAllAccess")
	}
	one := StallAccess{Stalls: []Stall{{ID: "a"}}}
	two := StallAccess{Stalls: []Stall{{ID: "a"}, {ID: "b"}}}
	if CanSwitchStall("pos", one) || !CanSwitchStall("pos", two) || !CanSwitchStall("admin", one) ||
		!CanSwitchStall("pos", StallAccess{AllAccess: true}) {
		t.Fatal("can_switch")
	}
	if !two.HasStall("b") || two.HasStall("c") {
		t.Fatal("HasStall")
	}
	if !IsStallID("11111111-1111-4111-8111-111111111111") || IsStallID("11111111-1111-7111-8111-111111111111") || IsStallID("all") {
		t.Fatal("UUID_RE")
	}
}

func TestSortStalls(t *testing.T) {
	got := SortStalls([]Stall{
		{ID: "s10", Code: "STALL-10"},
		{ID: "x", Code: "KIOSK-2", Name: "Kiosk"},
		{ID: "s2", Code: "stall-02"},
		{ID: "main", Code: "MAIN"},
		{ID: "def", Code: "ZZZ", IsDefault: true},
		{ID: "k1", Code: "KIOSK-10"},
	})
	var ids []string
	for _, s := range got {
		ids = append(ids, s.ID)
	}
	if want := "def main x k1 s2 s10"; strings.Join(ids, " ") != want {
		t.Fatalf("got %v, want %s", ids, want)
	}
}

func stallIDs(stalls []Stall) string {
	var ids []string
	for _, s := range stalls {
		ids = append(ids, s.ID)
	}
	return strings.Join(ids, " ")
}

func TestSortStallsByBranch(t *testing.T) {
	got := SortStallsByBranch([]Stall{
		{ID: "b-s2", Code: "STALL-02", BranchID: "2", BranchName: "Bandung"},
		{ID: "a-s1", Code: "STALL-01", BranchID: "1", BranchName: "alpha"},
		{ID: "b-main", Code: "MAIN", IsDefault: true, BranchID: "2", BranchName: "Bandung"},
		{ID: "a-main", Code: "MAIN", IsDefault: true, BranchID: "1", BranchName: "alpha"},
		{ID: "c-main", Code: "MAIN", IsDefault: true, BranchID: "3", BranchName: "Cirebon"},
		{ID: "b-s1", Code: "STALL-01", BranchID: "2", BranchName: "Bandung"},
	})
	// Branch name (case-insensitive), then Main Storage, then stall order.
	if want := "a-main a-s1 b-main b-s1 b-s2 c-main"; stallIDs(got) != want {
		t.Fatalf("got %s, want %s", stallIDs(got), want)
	}
	// Same-name branches stay grouped by branch id.
	got = SortStallsByBranch([]Stall{
		{ID: "x2", Code: "MAIN", IsDefault: true, BranchID: "2", BranchName: "Dup"},
		{ID: "x1s", Code: "STALL-01", BranchID: "1", BranchName: "Dup"},
		{ID: "x1", Code: "MAIN", IsDefault: true, BranchID: "1", BranchName: "Dup"},
	})
	if want := "x1 x1s x2"; stallIDs(got) != want {
		t.Fatalf("dup names: got %s, want %s", stallIDs(got), want)
	}
}

func TestStallLabels(t *testing.T) {
	one := []Stall{
		{ID: "1", Name: "Main Storage", BranchID: "a", BranchName: "Alpha"},
		{ID: "2", Name: "Stall 1", BranchID: "a", BranchName: "Alpha"},
	}
	two := append(append([]Stall(nil), one...), Stall{ID: "3", Name: "Main Storage", BranchID: "b", BranchName: "Beta"})
	if SpansBranches(nil) || SpansBranches(one) || !SpansBranches(two) {
		t.Fatal("SpansBranches")
	}
	if got := StallLabel(one[0], SpansBranches(one)); got != "Main Storage" {
		t.Fatalf("single branch: %q", got)
	}
	if got := StallLabel(two[0], SpansBranches(two)); got != "Main Storage — Alpha" {
		t.Fatalf("multi branch: %q", got)
	}
	if got := StallLabel(two[2], true); got != "Main Storage — Beta" {
		t.Fatalf("multi branch: %q", got)
	}
	// No branch name known: never a dangling separator.
	if got := StallLabel(Stall{Name: "Kiosk"}, true); got != "Kiosk" {
		t.Fatalf("no branch name: %q", got)
	}
}

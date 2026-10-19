package domain

import "testing"

func TestLogbookFullAccessNeedsKinerjaMenu(t *testing.T) {
	if HasFullLogbookAccess("hrd", false) || CanReviewLogbook("hrd", false) {
		t.Fatal("hrd without Kinerja must be limited to its own department")
	}
	if !HasFullLogbookAccess("hrd", true) || !CanReviewLogbook("hrd", true) {
		t.Fatal("hrd with Kinerja keeps full access")
	}
	if HasFullLogbookAccess("employee", true) {
		t.Fatal("non-HR roles never get full access")
	}
}

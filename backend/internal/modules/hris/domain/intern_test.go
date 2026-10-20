package domain

import "testing"

func TestIsInternStatus(t *testing.T) {
	for status, want := range map[string]bool{
		"intern": true, "internship": true, " Intern ": true, "contract": false, "": false, "internal": false,
	} {
		if got := IsInternStatus(status); got != want {
			t.Errorf("IsInternStatus(%q) = %v, want %v", status, got, want)
		}
	}
}

func TestPersonalLeaveDeductsThpInCatalog(t *testing.T) {
	p, _ := LookupLeaveType("personal")
	l, _ := LookupLeaveType("leader")
	if !p.DeductsThpDay || l.DeductsThpDay {
		t.Fatalf("DeductsThpDay personal=%v leader=%v", p.DeductsThpDay, l.DeductsThpDay)
	}
}

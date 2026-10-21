package posops

import (
	"slices"
	"strings"
	"testing"

	"nuhabit/backend/internal/modules/posops/domain"
)

func stallIn(id, name, code, branchID, branch string, main bool) domain.Stall {
	return domain.Stall{ID: id, Name: name, Code: code, BranchID: &branchID, BranchName: branch, IsDefault: main}
}

// Report stall pickers name the branch when they mix branches, are grouped by
// branch with Main Storage first, and keep the plain names for one branch.
func TestStallOptionsBranchLabels(t *testing.T) {
	two := []domain.Stall{
		stallIn("z-s", "Stall 1", "STALL-01", "z", "Zeta", false),
		stallIn("z-m", "Main Storage", "MAIN", "z", "Zeta", true),
		stallIn("a-m", "Main Storage", "MAIN", "a", "Alpha", true),
	}
	got := stallOptions(two)
	var order, labels []string
	for _, o := range got {
		order = append(order, o.ID)
		labels = append(labels, o.Label)
	}
	if want := "a-m z-m z-s"; strings.Join(order, " ") != want {
		t.Fatalf("order %v want %s", order, want)
	}
	if want := []string{"Main Storage — Alpha", "Main Storage — Zeta", "Stall 1 — Zeta"}; !slices.Equal(labels, want) {
		t.Fatalf("labels %v want %v", labels, want)
	}
	if got[0].BranchID != "a" || got[0].BranchName != "Alpha" || got[0].Name != "Main Storage" || got[0].Code != "MAIN" {
		t.Fatalf("fields %+v", got[0])
	}

	one := stallOptions(two[:2])
	if one[0].Label != "Main Storage" || one[1].Label != "Stall 1" || one[0].BranchName != "Zeta" {
		t.Fatalf("single branch %+v", one)
	}
	if len(stallOptions(nil)) != 0 {
		t.Fatal("empty")
	}
}

// Exports name the selected outlet the way the picker does.
func TestStallLabelUsesBranchLabel(t *testing.T) {
	opts := []stallOption{{ID: "w1", Name: "Main Storage", Label: "Main Storage — Alpha"}}
	if got := stallLabel(sp("w1"), opts); got != "Main Storage — Alpha" {
		t.Fatal(got)
	}
	head := stallReportHead{Filters: reportFilters{WarehouseID: sp("w1")}, StallOptions: []stallOption{{ID: "w1", Name: "Main Storage", Code: "MAIN", Label: "Main Storage — Alpha"}}}
	if got := head.stallFilterLabel(); got != "Main Storage — Alpha (MAIN)" {
		t.Fatal(got)
	}
}

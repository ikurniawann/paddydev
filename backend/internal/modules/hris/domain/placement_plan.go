package domain

import "fmt"

// Bulk placement planning (EPIC-070): what a bulk move, add or end does to
// one employee's placements, decided without the database so the R4 rules
// are tested on their own.

// PlacementSpan is one placement of the employee: store and date range
// (End nil = open-ended).
type PlacementSpan struct {
	ID, BranchID, BranchName, Start string
	End                             *string
}

// RunningOn: the placement has not ended before date.
func (p PlacementSpan) RunningOn(date string) bool { return p.End == nil || *p.End >= date }

// Bulk outcomes per employee.
const (
	PlacementCreated = "created"
	PlacementMoved   = "moved"
	PlacementEnded   = "ended"
	PlacementSkipped = "skipped"
)

// MoveDecision is the plan of a bulk move: End lists the placements to end
// on EndDate (the day before the move) before the new primary placement is
// created; Reason explains a skip.
type MoveDecision struct {
	Outcome, Reason, EndDate string
	End                      []PlacementSpan
}

// PlanPlacementMove applies R4 for a move to target from start: skipped when
// a running placement at target already covers start ("Sudah di toko ini")
// or a placement starts on/after start (HR edits it through Kelola);
// otherwise every running placement ends the day before start.
func PlanPlacementMove(existing []PlacementSpan, target, start string) MoveDecision {
	d := MoveDecision{EndDate: AddDaysISO(start, -1), End: []PlacementSpan{}}
	for _, p := range existing {
		if p.BranchID == target && p.Start <= start && p.RunningOn(start) {
			d.Outcome, d.Reason = PlacementSkipped, "Sudah di toko ini"
			return d
		}
	}
	future := ""
	for _, p := range existing {
		if p.Start >= start && (future == "" || p.Start < future) {
			future = p.Start
		}
	}
	if future != "" {
		d.Outcome, d.Reason = PlacementSkipped, fmt.Sprintf("Punya penempatan mulai %s — ubah lewat Kelola", FormatDateID(future))
		return d
	}
	for _, p := range existing {
		if p.RunningOn(start) {
			d.End = append(d.End, p)
		}
	}
	d.Outcome = PlacementCreated
	if len(d.End) > 0 {
		d.Outcome = PlacementMoved
	}
	return d
}

// PlanPlacementAdd is the skip reason of a bulk add to target over
// [start, end] (end nil = open): a placement at the same store overlapping
// the range (the no-overlap constraint); "" when the add goes ahead.
func PlanPlacementAdd(existing []PlacementSpan, target, start string, end *string) string {
	for _, p := range existing {
		if p.BranchID == target && p.RunningOn(start) && (end == nil || p.Start <= *end) {
			return "Sudah ditempatkan di toko ini"
		}
	}
	return ""
}

// EndDecision is the plan of a bulk end: End lists the placements whose
// end_date becomes the requested date.
type EndDecision struct {
	Outcome, Reason string
	End             []PlacementSpan
}

// PlanPlacementEnd ends the placements (all, or only at branch) still
// running after end. An employee with such a placement starting after end
// is skipped (the end date would precede its start), as is one without any.
func PlanPlacementEnd(existing []PlacementSpan, branch, end string) EndDecision {
	d := EndDecision{End: []PlacementSpan{}}
	for _, p := range existing {
		if (branch != "" && p.BranchID != branch) || (p.End != nil && *p.End <= end) {
			continue
		}
		if p.Start > end {
			return EndDecision{Outcome: PlacementSkipped, Reason: "Penempatan mulai sesudah tanggal akhir", End: []PlacementSpan{}}
		}
		d.End = append(d.End, p)
	}
	if len(d.End) == 0 {
		d.Outcome, d.Reason = PlacementSkipped, "Tidak ada penempatan berjalan"
		return d
	}
	d.Outcome = PlacementEnded
	return d
}

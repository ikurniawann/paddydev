package domain

import (
	"slices"
	"testing"
)

func span(id, branch, start string, end *string) PlacementSpan {
	return PlacementSpan{ID: id, BranchID: branch, BranchName: "Toko " + branch, Start: start, End: end}
}

func spanIDs(ps []PlacementSpan) []string {
	ids := []string{}
	for _, p := range ps {
		ids = append(ids, p.ID)
	}
	return ids
}

// EPIC-070 R4: pindah massal ke toko tujuan Y mulai start.
func TestPlanPlacementMove(t *testing.T) {
	const start = "2026-10-05"
	cases := []struct {
		name     string
		existing []PlacementSpan
		outcome  string
		reason   string
		ended    []string
	}{
		{"tanpa penempatan dibuat", nil, "created", "", []string{}},
		{"toko lama berjalan diakhiri", []PlacementSpan{span("x", "X", "2026-01-01", nil)}, "moved", "", []string{"x"}},
		{"dua toko berjalan diakhiri semua", []PlacementSpan{
			span("x", "X", "2026-01-01", nil), span("z", "Z", "2026-03-01", sp("2026-12-31"))}, "moved", "", []string{"x", "z"}},
		{"penempatan yang sudah berakhir tidak disentuh", []PlacementSpan{span("x", "X", "2026-01-01", sp("2026-10-04"))},
			"created", "", []string{}},
		{"sudah di toko tujuan", []PlacementSpan{span("y", "Y", "2026-01-01", nil)}, "skipped", "Sudah di toko ini", []string{}},
		{"sudah di toko tujuan mulai hari pindah", []PlacementSpan{span("y", "Y", start, nil)}, "skipped", "Sudah di toko ini", []string{}},
		{"toko tujuan yang sudah berakhir bukan alasan lewati", []PlacementSpan{span("y", "Y", "2026-01-01", sp("2026-09-30"))},
			"created", "", []string{}},
		{"penempatan masa depan", []PlacementSpan{
			span("x", "X", "2026-01-01", nil), span("z", "Z", "2026-11-01", nil)}, "skipped",
			"Punya penempatan mulai 01/11/2026 — ubah lewat Kelola", []string{}},
		{"penempatan masa depan paling awal disebut", []PlacementSpan{
			span("z", "Z", "2026-12-01", nil), span("w", "W", "2026-11-15", sp("2026-11-20"))}, "skipped",
			"Punya penempatan mulai 15/11/2026 — ubah lewat Kelola", []string{}},
		{"penempatan toko tujuan di masa depan", []PlacementSpan{span("y", "Y", "2026-11-01", nil)}, "skipped",
			"Punya penempatan mulai 01/11/2026 — ubah lewat Kelola", []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := PlanPlacementMove(c.existing, "Y", start)
			if d.Outcome != c.outcome || d.Reason != c.reason || !slices.Equal(spanIDs(d.End), c.ended) {
				t.Fatalf("got %+v, want %s %q %v", d, c.outcome, c.reason, c.ended)
			}
			if d.EndDate != "2026-10-04" {
				t.Fatalf("end date = %s", d.EndDate)
			}
		})
	}
}

func TestPlanPlacementAdd(t *testing.T) {
	cases := []struct {
		name       string
		existing   []PlacementSpan
		start      string
		end        *string
		wantReason string
	}{
		{"tanpa penempatan", nil, "2026-10-05", nil, ""},
		{"toko lain", []PlacementSpan{span("x", "X", "2026-01-01", nil)}, "2026-10-05", nil, ""},
		{"toko sama berjalan", []PlacementSpan{span("y", "Y", "2026-01-01", nil)}, "2026-10-05", nil, "Sudah ditempatkan di toko ini"},
		{"toko sama sudah berakhir", []PlacementSpan{span("y", "Y", "2026-01-01", sp("2026-10-04"))}, "2026-10-05", nil, ""},
		{"toko sama masa depan tumpang tindih", []PlacementSpan{span("y", "Y", "2026-12-01", nil)}, "2026-10-05", nil,
			"Sudah ditempatkan di toko ini"},
		{"toko sama masa depan sesudah rentang", []PlacementSpan{span("y", "Y", "2026-12-01", nil)}, "2026-10-05", sp("2026-11-30"), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PlanPlacementAdd(c.existing, "Y", c.start, c.end); got != c.wantReason {
				t.Fatalf("reason = %q, want %q", got, c.wantReason)
			}
		})
	}
}

func TestPlanPlacementEnd(t *testing.T) {
	const end = "2026-10-10"
	cases := []struct {
		name     string
		existing []PlacementSpan
		branch   string
		outcome  string
		reason   string
		ended    []string
	}{
		{"semua berjalan diakhiri", []PlacementSpan{
			span("x", "X", "2026-01-01", nil), span("y", "Y", "2026-02-01", sp("2026-12-31"))}, "", "ended", "", []string{"x", "y"}},
		{"hanya di toko", []PlacementSpan{
			span("x", "X", "2026-01-01", nil), span("y", "Y", "2026-02-01", nil)}, "X", "ended", "", []string{"x"}},
		{"berakhir sebelum tanggal tidak diperpanjang", []PlacementSpan{
			span("x", "X", "2026-01-01", sp("2026-10-01")), span("y", "Y", "2026-02-01", nil)}, "", "ended", "", []string{"y"}},
		{"mulai pada tanggal akhir boleh", []PlacementSpan{span("x", "X", end, nil)}, "", "ended", "", []string{"x"}},
		{"mulai sesudah tanggal akhir", []PlacementSpan{
			span("x", "X", "2026-01-01", nil), span("z", "Z", "2026-10-20", nil)}, "", "skipped",
			"Penempatan mulai sesudah tanggal akhir", []string{}},
		{"penempatan masa depan di toko lain tidak menghalangi", []PlacementSpan{
			span("x", "X", "2026-01-01", nil), span("z", "Z", "2026-10-20", nil)}, "X", "ended", "", []string{"x"}},
		{"tanpa penempatan berjalan", []PlacementSpan{span("x", "X", "2026-01-01", sp("2026-10-10"))}, "", "skipped",
			"Tidak ada penempatan berjalan", []string{}},
		{"tanpa penempatan di toko", []PlacementSpan{span("y", "Y", "2026-01-01", nil)}, "X", "skipped",
			"Tidak ada penempatan berjalan", []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := PlanPlacementEnd(c.existing, c.branch, end)
			if d.Outcome != c.outcome || d.Reason != c.reason || !slices.Equal(spanIDs(d.End), c.ended) {
				t.Fatalf("got %+v, want %s %q %v", d, c.outcome, c.reason, c.ended)
			}
		})
	}
}

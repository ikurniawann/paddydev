package iam

import "testing"

// The employee-level Kinerja leaves (Review, Tugas Departemen) do not make an
// account Kinerja HR; any other Kinerja menu does (EPIC-057).
func TestHasPerformanceHRMenu(t *testing.T) {
	cases := []struct {
		granted []string
		want    bool
	}{
		{nil, false},
		{[]string{"hris.performance.review", "hris.performance.dept-tasks", "hris.employees"}, false},
		{[]string{"hris.performanceX"}, false},
		{[]string{"hris.performance"}, true},
		{[]string{"hris.performance.review", "hris.performance.kpi-scorecard"}, true},
		{[]string{"hris.performance.logbook-list"}, true},
	}
	for _, c := range cases {
		if got := HasPerformanceHRMenu(c.granted); got != c.want {
			t.Errorf("HasPerformanceHRMenu(%v) = %v, want %v", c.granted, got, c.want)
		}
	}
}

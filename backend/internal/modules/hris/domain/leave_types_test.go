package domain

import (
	"slices"
	"testing"
)

func TestLeaveTypeCatalog(t *testing.T) {
	var selectable []string
	for _, c := range LeaveTypeCodes() {
		if lt, _ := LookupLeaveType(c); lt.Selectable {
			selectable = append(selectable, c)
		}
	}
	want := []string{"annual", "sick", "personal", "leader", "maternity", "paternity", "pilgrimage", "marriage", "bereavement"}
	if !slices.Equal(selectable, want) {
		t.Fatalf("selectable = %v", selectable)
	}
	for _, removed := range []string{"emergency", "unpaid", "menstrual"} {
		lt, ok := LookupLeaveType(removed)
		if !ok || lt.Selectable {
			t.Fatalf("%s harus tetap dikenal tetapi tidak bisa diajukan: %+v %v", removed, lt, ok)
		}
	}
	if len(LeaveTypeCodes()) != 12 {
		t.Fatalf("codes = %v", LeaveTypeCodes())
	}
	leader, _ := LookupLeaveType("leader")
	personal, _ := LookupLeaveType("personal")
	if leader.MonthlyQuota != 1 || !leader.SingleDay || !leader.LeaderOnly || leader.Label != "Izin Leader" {
		t.Fatalf("leader = %+v", leader)
	}
	if personal.MonthlyQuota != 3 || !personal.SingleDay || personal.LeaderOnly || personal.Label != "Izin Pribadi" {
		t.Fatalf("personal = %+v", personal)
	}
	q := QuotaLeaveTypes()
	if len(q) != 2 || q[0].Code != "personal" || q[1].Code != "leader" {
		t.Fatalf("quota types = %+v", q)
	}
	for code, want := range map[string][3]string{
		"sick":      {"Sakit", "Sakit", "Cuti Sakit"},
		"emergency": {"Izin Darurat", "Darurat", "Cuti Darurat"},
		"unpaid":    {"Izin Tanpa Gaji", "Tidak Dibayar", "Cuti Tanpa Upah"},
		"menstrual": {"Cuti Haid", "Haid", "Cuti Haid"},
		"marriage":  {"Cuti Menikah", "Pernikahan", "Cuti Menikah"},
		"lainnya":   {"lainnya", "lainnya", "lainnya"},
	} {
		got := [3]string{LeaveTypeLabel(code), LeaveTypeShort(code), LeaveTypeExportLabel(code)}
		if got != want {
			t.Fatalf("%s labels = %v, want %v", code, got, want)
		}
	}
}

func TestIsLeaderLevel(t *testing.T) {
	for _, lv := range []string{"Supervisor", " supervisor ", "manager", "Assistant Manager", "General Manager", "Director", "C-Level", "SENIOR MANAGER"} {
		if !IsLeaderLevel(lv) {
			t.Fatalf("%q harus layak Izin Leader", lv)
		}
	}
	for _, lv := range []string{"", "Staff", "staff", "Senior Staff", "Coordinator", "Lead"} {
		if IsLeaderLevel(lv) {
			t.Fatalf("%q tidak boleh layak Izin Leader", lv)
		}
	}
}

func TestMonthBounds(t *testing.T) {
	cases := map[string][2]string{"2031-03": {"2031-03-01", "2031-03-31"}, "2032-02": {"2032-02-01", "2032-02-29"}, "2031-12": {"2031-12-01", "2031-12-31"}}
	for m, want := range cases {
		from, to, ok := MonthBounds(m)
		if !ok || from != want[0] || to != want[1] {
			t.Fatalf("%s = %s %s %v", m, from, to, ok)
		}
	}
	for _, bad := range []string{"", "2031-3", "2031-13", "maret"} {
		if _, _, ok := MonthBounds(bad); ok {
			t.Fatalf("%q harus ditolak", bad)
		}
	}
	if MonthOf("2031-03-31") != "2031-03" || MonthOf("2031-04-01") != "2031-04" {
		t.Fatal("MonthOf")
	}
}

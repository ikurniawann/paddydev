package hris

import (
	"context"
	"slices"
	"strings"
	"testing"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/xlsx"
)

// EPIC-086: audit, template, pratinjau dan terapkan data kerja massal.

type workDataFixture struct {
	shiftNoSchedule, nonShiftOK, noPlacement, inactive string // NIP
	branchCode, branchID, dept, pos                    string
}

func (h *harness) workDataFixture() workDataFixture {
	h.t.Helper()
	f := workDataFixture{dept: h.department("Retail Uji EPIC086")}
	f.branchID = h.branch("Toko Uji EPIC086")
	f.branchCode = h.scalar(`SELECT code FROM configuration.branches WHERE id = $1`, f.branchID)
	f.pos = h.scalar(`INSERT INTO hris.positions (title, level) VALUES ('Shopkeeper Uji 086', 'Staff') RETURNING id::text`)
	mk := func(name, scheduleType string, placed, pattern, active bool) string {
		id := h.employee(emp{name: name, inactive: !active})
		h.exec(`UPDATE hris.employees SET schedule_type = $2, department_id = $3, job_title_id = $4 WHERE id = $1`,
			id, scheduleType, f.dept, f.pos)
		if placed {
			h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date)
				VALUES ($1, $2, true, '2026-01-01')`, id, f.branchID)
		}
		if pattern {
			h.nsWeek(id, "2026-01-01")
		}
		return h.scalar(`SELECT nip FROM hris.employees WHERE id = $1`, id)
	}
	f.shiftNoSchedule = mk("Shift Tanpa Jadwal", "shift", true, false, true)
	f.nonShiftOK = mk("NonShift Lengkap", "non_shift", true, true, true)
	f.noPlacement = mk("Tanpa Penempatan", "non_shift", false, true, true)
	f.inactive = mk("Sudah Keluar", "shift", true, false, false)
	return f
}

func TestWorkDataAuditAndTemplate(t *testing.T) {
	h := newHarness(t)
	f := h.workDataFixture()
	ctx := context.Background()

	gaps, err := WorkDataAudit(ctx, h.tx, "2026-11-03")
	if err != nil {
		t.Fatal(err)
	}
	byType := map[string]WorkDataGap{}
	for _, g := range gaps {
		byType[g.ScheduleType] = g
	}
	if g := byType["shift"]; g.Total != 1 || g.NoSchedule != 1 || g.NoPrimary != 0 {
		t.Errorf("shift gap = %+v", g)
	}
	if g := byType["non_shift"]; g.Total != 2 || g.NoPrimary != 1 || g.NoSchedule != 0 {
		t.Errorf("non_shift gap = %+v", g)
	}

	data, err := WorkDataTemplate(ctx, h.tx, "2026-11-03")
	if err != nil {
		t.Fatal(err)
	}
	m, err := xlsx.ParseMatrix(data, xlsx.ReadOptions{PreferSheet: "Data"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(m[0][:len(domain.WorkDataHeader)], domain.WorkDataHeader) {
		t.Fatalf("header = %v", m[0])
	}
	var nips []string
	for _, r := range m[1:] {
		if r[0] != "" {
			nips = append(nips, r[0]+"="+r[6])
		}
	}
	slices.Sort(nips)
	want := []string{f.noPlacement + "=penempatan", f.shiftNoSchedule + "=jadwal kerja"}
	slices.Sort(want)
	if !slices.Equal(nips, want) {
		t.Fatalf("baris template = %v, want %v", nips, want)
	}
	codes, err := xlsx.ParseMatrix(data, xlsx.ReadOptions{PreferSheet: "Kode Shift"})
	if err != nil {
		t.Fatal(err)
	}
	var shiftCodes []string
	for _, r := range codes[1:] {
		shiftCodes = append(shiftCodes, r[0])
	}
	if !slices.Contains(shiftCodes, "LIBUR") || !slices.Contains(shiftCodes, "NS") {
		t.Fatalf("kode shift = %v", shiftCodes)
	}
	stores, err := xlsx.ParseMatrix(data, xlsx.ReadOptions{PreferSheet: "Kode Toko"})
	if err != nil {
		t.Fatal(err)
	}
	var storeCodes []string
	for _, r := range stores[1:] {
		storeCodes = append(storeCodes, r[0])
	}
	if !slices.Contains(storeCodes, strings.ToUpper(f.branchCode)) {
		t.Fatalf("kode toko = %v", storeCodes)
	}
}

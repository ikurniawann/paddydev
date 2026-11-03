package hris

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

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

// fillWorkData writes cells into the template's Data sheet: row by NIP (or
// the first empty row for an unknown one), column by header.
func fillWorkData(t *testing.T, data []byte, nip string, cells map[string]any) []byte {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := f.GetRows("Data")
	if err != nil {
		t.Fatal(err)
	}
	target := len(rows) + 1
	for i, r := range rows {
		if len(r) > 0 && r[0] == nip {
			target = i + 1
			break
		}
	}
	_ = f.SetCellValue("Data", "A"+strconv.Itoa(target), nip)
	for header, v := range cells {
		col := slices.Index(domain.WorkDataHeader, header)
		if col < 0 {
			t.Fatalf("kolom %s tidak ada", header)
		}
		name, _ := excelize.CoordinatesToCellName(col+1, target)
		if err := f.SetCellValue("Data", name, v); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sha(data []byte) string { s := sha256.Sum256(data); return hex.EncodeToString(s[:]) }

func TestWorkDataApply(t *testing.T) {
	h := newHarness(t)
	f := h.workDataFixture()
	ctx := context.Background()
	const today = "2026-11-03"
	idOf := func(nip string) string { return h.scalar(`SELECT id::text FROM hris.employees WHERE nip = $1`, nip) }
	shiftEmp, placeEmp := idOf(f.shiftNoSchedule), idOf(f.noPlacement)
	// Penempatan utama mendatang yang harus diganti (Review Focus 1).
	h.exec(`UPDATE hris.employee_placements SET end_date = '2026-11-30' WHERE employee_id = $1`, shiftEmp)
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date) VALUES ($1, $2, true, '2026-12-01')`,
		shiftEmp, f.branchID)
	branch2 := h.branch("Toko Kedua EPIC086")
	code2 := strings.ToUpper(h.scalar(`SELECT code FROM configuration.branches WHERE id = $1`, branch2))
	pos2 := h.scalar(`INSERT INTO hris.positions (title, level) VALUES ('Kasir Uji 086', 'Staff') RETURNING id::text`)
	toNonShift := h.employee(emp{name: "Pindah Non-Shift"})
	h.exec(`UPDATE hris.employees SET schedule_type = 'shift', department_id = $2 WHERE id = $1`, toNonShift, f.dept)
	toNonShiftNIP := h.scalar(`SELECT nip FROM hris.employees WHERE id = $1`, toNonShift)

	tpl, err := WorkDataTemplate(ctx, h.tx, today)
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC) // sel tanggal sungguhan (Review Focus 5)
	file := fillWorkData(t, tpl, f.shiftNoSchedule, map[string]any{
		"Penempatan Utama (Kode Toko)": code2, "Senin": "NS", "Selasa": "NS", "Rabu": "NS", "Kamis": "NS",
		"Jumat": "NS", "Sabtu": "OPSSAT+SIANG14", "Minggu": "LIBUR", "Berlaku Mulai": from, "Jabatan Baru": "Kasir Uji 086"})
	file = fillWorkData(t, file, f.noPlacement, map[string]any{"Penempatan Utama (Kode Toko)": strings.ToUpper(f.branchCode),
		"Berlaku Mulai": "2026-11-10"})
	file = fillWorkData(t, file, toNonShiftNIP, map[string]any{"Tipe Jadwal": "Non-Shift", "Berlaku Mulai": "2026-11-10"})

	snapshot := func() string {
		return h.scalar(`SELECT md5(
			(SELECT coalesce(string_agg(p::text, '' ORDER BY p.id), '') FROM hris.employee_placements p) ||
			(SELECT coalesce(string_agg(s::text, '' ORDER BY s.id), '') FROM hris.employee_shifts s) ||
			(SELECT coalesce(string_agg(e.schedule_type || coalesce(e.job_title_id::text, ''), '' ORDER BY e.id), '') FROM hris.employees e))`)
	}
	before := snapshot()

	// File dengan satu baris salah: pratinjau melaporkan, apply menolak semuanya (Review Focus 4).
	bad := fillWorkData(t, file, f.nonShiftOK, map[string]any{"Penempatan Utama (Kode Toko)": "ZZZ", "Berlaku Mulai": "2026-11-10"})
	rep, err := WorkDataPreview(ctx, h.tx, bad, today)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Errors != 1 || rep.Valid != 3 {
		t.Fatalf("pratinjau file salah = %+v", rep)
	}
	if _, err := WorkDataApply(ctx, h.tx, bad, today, sha(bad)); err == nil {
		t.Fatal("apply file berisi error harus ditolak")
	}
	// sha tidak cocok (file berubah setelah pratinjau).
	if _, err := WorkDataApply(ctx, h.tx, file, today, sha(bad)); err == nil || !strings.Contains(err.Error(), "sha") {
		t.Fatalf("apply sha salah = %v", err)
	}
	if snapshot() != before {
		t.Fatal("penolakan tidak boleh menulis apa pun")
	}

	rep, err = WorkDataPreview(ctx, h.tx, file, today)
	if err != nil || rep.Errors != 0 || rep.Valid != 3 || rep.SHA != sha(file) {
		t.Fatalf("pratinjau = %+v, %v", rep, err)
	}
	if _, err := WorkDataApply(ctx, h.tx, file, today, rep.SHA); err != nil {
		t.Fatal(err)
	}

	pattern := h.scalar(`SELECT string_agg(es.day_of_week || ':' || coalesce(s.code, 'L') || coalesce('+' || s2.code, ''), ',' ORDER BY es.day_of_week)
		FROM hris.employee_shifts es LEFT JOIN hris.shifts s ON s.id = es.shift_id LEFT JOIN hris.shifts s2 ON s2.id = es.shift2_id
		WHERE es.employee_id = $1 AND es.effective_from = '2026-11-10'`, shiftEmp)
	if pattern != "1:NS,2:NS,3:NS,4:NS,5:NS,6:OPSSAT+SIANG14,7:L" {
		t.Fatalf("pola = %s", pattern)
	}
	places := h.scalar(`SELECT string_agg(b.code || ' ' || p.is_primary || ' ' || p.start_date || '..' || coalesce(p.end_date::text, ''), '; ' ORDER BY p.start_date)
		FROM hris.employee_placements p JOIN configuration.branches b ON b.id = p.branch_id WHERE p.employee_id = $1`, shiftEmp)
	code2raw := h.scalar(`SELECT code FROM configuration.branches WHERE id = $1`, branch2)
	if want := f.branchCode + " true 2026-01-01..2026-11-09; " + code2raw + " true 2026-11-10.."; places != want {
		t.Fatalf("penempatan = %s, want %s", places, want)
	}
	if got := h.scalar(`SELECT count(*)::text FROM hris.employee_placements WHERE employee_id = $1 AND is_primary AND start_date = '2026-11-10'`, placeEmp); got != "1" {
		t.Fatalf("penempatan baru = %s", got)
	}
	if got := h.scalar(`SELECT job_title_id::text FROM hris.employees WHERE id = $1`, shiftEmp); got != pos2 {
		t.Fatalf("jabatan = %s", got)
	}
	if got := h.scalar(`SELECT schedule_type || ' ' || (SELECT count(*) FROM hris.employee_shifts WHERE employee_id = $1 AND effective_from = '2026-11-10')
		FROM hris.employees WHERE id = $1`, toNonShift); got != "non_shift 7" {
		t.Fatalf("ke Non-Shift = %s", got)
	}
	if got := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'hris.bulk_work_data' AND after->>'sha' = $1`, rep.SHA); got != "1" {
		t.Fatalf("audit ringkasan = %s", got)
	}
	if got := h.scalar(`SELECT count(DISTINCT entity_id)::text FROM audit.audit_log
		WHERE actor_name = 'Sistem (unggah massal data kerja)' AND entity_id IN ($1, $2, $3)`, shiftEmp, placeEmp, toNonShift); got != "3" {
		t.Fatalf("audit per karyawan = %s", got)
	}
}

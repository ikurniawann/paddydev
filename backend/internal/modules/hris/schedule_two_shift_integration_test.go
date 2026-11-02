package hris

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"nuhabit/backend/internal/platform/testutil"
)

// EPIC-080 R6/R7: two shifts in one day (KODE1+KODE2) on the Jadwal Kerja
// grid, the xlsx upload/template and the weekly pattern.

// twoShiftFixture inserts PAGI4 08:00–12:00 and SORE4 17:00–21:00 (no
// break), SPG1 08:00–16:00 and SPG2 11:00–19:00 when missing.
func twoShiftFixture(h *harness) (pagi4, sore4, spg1, spg2 string) {
	h.exec(`INSERT INTO hris.shifts (code, name, start_time, end_time, break_minutes) VALUES
		('PAGI4', 'Pagi 4 jam', '08:00', '12:00', 0), ('SORE4', 'Sore 4 jam', '17:00', '21:00', 0),
		('SPG1', 'SPG 1', '08:00', '16:00', 60), ('SPG2', 'SPG 2', '11:00', '19:00', 60)
		ON CONFLICT (upper(code)) DO NOTHING`)
	id := func(code string) string { return h.scalar(`SELECT id::text FROM hris.shifts WHERE code = $1`, code) }
	return id("PAGI4"), id("SORE4"), id("SPG1"), id("SPG2")
}

func TestScheduleCellsTwoShiftDay(t *testing.T) {
	h := newHarness(t)
	writer := h.staffActions("hrd", scheduleWriter)
	branch, ana, _ := scheduleFixture(h)
	pagi4, sore4, _, _ := twoShiftFixture(h)

	pre, done := commitCells(t, h, &writer, cellsBody(branch, "", anaCell(ana, "2026-10-06", "pagi4+sore4")))
	wantSummary(t, pre, map[string]float64{"new": 1})
	if ch := pre.data()["changes"].([]any); len(ch) != 1 || ch[0].(map[string]any)["after"] != "PAGI4+SORE4" {
		t.Fatalf("changes = %v", ch)
	}
	expect(t, done, http.StatusOK, "")
	if got := h.scalar(`SELECT shift_id::text || '+' || shift2_id::text FROM hris.employee_shift_days
		WHERE employee_id = $1 AND work_date = '2026-10-06'`, ana); got != pagi4+"+"+sore4 {
		t.Fatalf("stored = %s, want %s+%s", got, pagi4, sore4)
	}
	if code, src := gridCell(t, h, &writer, branch, ana, 1); code != "PAGI4+SORE4" || src != "day" {
		t.Fatalf("06/10 = %s/%s, want PAGI4+SORE4/day", code, src)
	}

	// Saving the same two shifts again changes nothing.
	pre = h.do(&writer, http.MethodPost, cellsPath, cellsBody(branch, "preview", anaCell(ana, "2026-10-06", "PAGI4+SORE4")))
	expect(t, pre, http.StatusOK, "")
	wantSummary(t, pre, map[string]float64{"same": 1, "changed": 0})
	if pre.data()["replaced_days"] != float64(0) {
		t.Fatalf("replaced_days = %v, want 0", pre.data()["replaced_days"])
	}
	// Dropping the second shift replaces the per-date row.
	pre = h.do(&writer, http.MethodPost, cellsPath, cellsBody(branch, "preview", anaCell(ana, "2026-10-06", "PAGI4")))
	expect(t, pre, http.StatusOK, "")
	wantSummary(t, pre, map[string]float64{"changed": 1})
	if pre.data()["replaced_days"] != float64(1) {
		t.Fatalf("replaced_days = %v, want 1", pre.data()["replaced_days"])
	}
	// A second shift changed behind the preview makes the commit stale.
	body := cellsBody(branch, "preview", anaCell(ana, "2026-10-07", "PAGI4+SORE4"))
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id)
		VALUES ($1, '2026-10-07', $2, gen_random_uuid())`, ana, pagi4)
	pre = h.do(&writer, http.MethodPost, cellsPath, body)
	expect(t, pre, http.StatusOK, "")
	h.exec(`UPDATE hris.employee_shift_days SET shift2_id = $2 WHERE employee_id = $1 AND work_date = '2026-10-07'`, ana, sore4)
	body["step"], body["fingerprint"], body["ack_warnings"] = "commit", pre.data()["fingerprint"], true
	expect(t, h.do(&writer, http.MethodPost, cellsPath, body), http.StatusConflict, "")

	// Overlapping shifts are an error.
	bad := h.do(&writer, http.MethodPost, cellsPath, cellsBody(branch, "preview", anaCell(ana, "2026-10-08", "SPG1+SPG2")))
	expect(t, bad, http.StatusOK, "")
	if !strings.Contains(bad.raw, "Shift kedua harus mulai setelah shift pertama selesai (SPG1 selesai 16.00, SPG2 mulai 11.00)") {
		t.Fatalf("preview = %s", bad.raw)
	}

	// The template writes the two-shift day as KODE1+KODE2.
	f := scheduleTemplateFile(t, h, &writer, branch)
	nip := h.scalar(`SELECT nip FROM hris.employees WHERE id = $1`, ana)
	rows, _ := f.GetRows("Jadwal")
	found := false
	for _, r := range rows {
		if len(r) > 4 && r[0] == nip {
			found = true
			if r[4] != "PAGI4+SORE4" {
				t.Fatalf("template 06/10 = %q, want PAGI4+SORE4", r[4])
			}
		}
	}
	if !found {
		t.Fatalf("Ana not in template rows %v", rows)
	}
}

func scheduleTemplateFile(t *testing.T, h *harness, s *testutil.Staff, branch string) *excelize.File {
	t.Helper()
	params := "?branch_id=" + branch + "&from=2026-10-05&to=2026-10-11"
	r := testutil.AsStaff(httptest.NewRequest(http.MethodGet, "/api/hris/schedules/template"+params, nil), *s)
	rec := httptest.NewRecorder()
	h.exec("SAVEPOINT harness_request")
	h.mux.ServeHTTP(rec, r)
	h.exec("RELEASE SAVEPOINT harness_request")
	if rec.Code != http.StatusOK {
		t.Fatalf("template status %d %s", rec.Code, rec.Body.String())
	}
	f, err := excelize.OpenReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestScheduleUploadTwoShifts(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", scheduleTestMenus, false)
	branch, ana, _ := scheduleFixture(h)
	pagi4, sore4, _, _ := twoShiftFixture(h)
	nip := h.scalar(`SELECT nip FROM hris.employees WHERE id = $1`, ana)
	fields := func(step string) map[string]string {
		return map[string]string{"branch_id": branch, "from": "2026-10-05", "to": "2026-10-11", "step": step}
	}

	// Review Focus 5: invalid KODE1+KODE2 cells are per-row errors and the
	// commit saves nothing.
	bad := anaWeek(branch, nip, []string{"SPG1+SPG2", "PAGI4+XX", "L+SPG1", "PAGI4+SORE4", "SPG1", "L", "L"})
	pre := upload(t, h, &hr, fields("preview"), bad)
	expect(t, pre, http.StatusOK, "")
	msgs := []string{}
	for _, e := range pre.data()["errors"].([]any) {
		m := e.(map[string]any)
		if m["line"] != float64(4) {
			t.Fatalf("error line = %v, want 4: %v", m["line"], m)
		}
		msgs = append(msgs, m["column"].(string)+": "+m["message"].(string))
	}
	for _, want := range []string{
		"Sen 05/10: Shift kedua harus mulai setelah shift pertama selesai (SPG1 selesai 16.00, SPG2 mulai 11.00)",
		"Sel 06/10: Kode 'XX' tidak ada di sheet Kode Shift",
		"Rab 07/10: Kode 'L+SPG1': L atau - tidak bisa digabung dengan shift",
	} {
		if !strings.Contains(strings.Join(msgs, "\n"), want) {
			t.Fatalf("missing %q in %v", want, msgs)
		}
	}
	commit := fields("commit")
	commit["fingerprint"], commit["ack_warnings"] = pre.data()["fingerprint"].(string), "true"
	expect(t, upload(t, h, &hr, commit, bad), http.StatusBadRequest, "File masih punya error — perbaiki lalu unggah ulang")
	if n := h.scalar(`SELECT count(*)::text FROM hris.employee_shift_days WHERE employee_id = $1`, ana); n != "0" {
		t.Fatalf("rows saved after a rejected upload: %s", n)
	}

	// A valid two-shift upload stores both shifts.
	good := anaWeek(branch, nip, []string{"PAGI4+SORE4", "pagi4 + sore4", "PAGI4", "SPG1", "SPG1", "L", "L"})
	pre = upload(t, h, &hr, fields("preview"), good)
	expect(t, pre, http.StatusOK, "")
	if errs := pre.data()["errors"].([]any); len(errs) != 0 {
		t.Fatalf("errors %v", errs)
	}
	commit["fingerprint"] = pre.data()["fingerprint"].(string)
	expect(t, upload(t, h, &hr, commit, good), http.StatusOK, "")
	if got := h.scalar(`SELECT string_agg(work_date::text || '=' || coalesce(shift2_id::text, '-'), ',' ORDER BY work_date)
		FROM hris.employee_shift_days WHERE employee_id = $1 AND shift_id = $2`, ana, pagi4); got != "2026-10-05="+sore4+",2026-10-06="+sore4+",2026-10-07=-" {
		t.Fatalf("PAGI4 rows = %s", got)
	}
}

func TestWeeklyPatternTwoShifts(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", scheduleTestMenus, false)
	branch, ana, _ := scheduleFixture(h)
	pagi4, sore4, spg1, spg2 := twoShiftFixture(h)
	put := func(day1 map[string]any) call {
		days := []map[string]any{day1}
		for d := 2; d <= 7; d++ {
			days = append(days, map[string]any{"day_of_week": d, "shift_id": nil})
		}
		return h.do(&hr, "PUT", "/api/hris/employees/"+ana+"/shifts", map[string]any{"effective_from": "2026-10-05", "days": days})
	}

	expect(t, put(map[string]any{"day_of_week": 1, "shift_id": spg1, "shift2_id": spg2}), http.StatusBadRequest,
		"Senin: Shift kedua harus mulai setelah shift pertama selesai (SPG1 selesai 16.00, SPG2 mulai 11.00)")
	expect(t, put(map[string]any{"day_of_week": 1, "shift_id": nil, "shift2_id": sore4}), http.StatusBadRequest,
		"Senin: shift kedua hanya bisa diisi bila shift pertama diisi")
	expect(t, put(map[string]any{"day_of_week": 1, "shift_id": pagi4, "shift2_id": "x"}), http.StatusBadRequest, "ID shift tidak valid")
	expect(t, put(map[string]any{"day_of_week": 1, "shift_id": pagi4, "shift2_id": sore4}), http.StatusOK, "")
	if got := h.scalar(`SELECT shift2_id::text FROM hris.employee_shifts WHERE employee_id = $1 AND day_of_week = 1`, ana); got != sore4 {
		t.Fatalf("shift2_id = %s, want %s", got, sore4)
	}

	listed := h.do(&hr, "GET", "/api/hris/employees/"+ana+"/shifts", nil)
	expect(t, listed, http.StatusOK, "")
	var monday map[string]any
	for _, r := range listed.list() {
		if m := r.(map[string]any); m["day_of_week"] == float64(1) {
			monday = m
		}
	}
	if monday["shift2_id"] != sore4 || monday["shift2_name"] != "Sore 4 jam" {
		t.Fatalf("monday = %v", monday)
	}
	if code, src := gridCell(t, h, &hr, branch, ana, 0); code != "PAGI4+SORE4" || src != "weekly" {
		t.Fatalf("05/10 = %s/%s, want PAGI4+SORE4/weekly", code, src)
	}

	// ShiftPattern (clock-in, leave, overtime) carries the second shift.
	rows, err := h.svc.repo.ShiftPattern(context.Background(), ana)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Num("day_of_week") != 1 {
			continue
		}
		if r.Str("shift2_id") != sore4 || r.Str("shift2_start_time") != "17:00:00" || r.Str("shift2_end_time") != "21:00:00" ||
			r.Num("shift2_break_minutes") != 0 || r.Bool("shift2_is_overnight") {
			t.Fatalf("pattern row = %v", r)
		}
		return
	}
	t.Fatal("Monday pattern row missing")
}

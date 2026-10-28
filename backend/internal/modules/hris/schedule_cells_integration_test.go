package hris

import (
	"net/http"
	"testing"

	"nuhabit/backend/internal/platform/hrisdel"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/testutil"
)

const cellsPath = "/api/hris/schedules/cells"

func cellsBody(branch, step string, cells ...map[string]any) map[string]any {
	return map[string]any{"branch_id": branch, "step": step, "cells": cells}
}

func anaCell(ana, date, code string) map[string]any {
	return map[string]any{"employee_id": ana, "date": date, "code": code}
}

// gridCell is one cell of the Jadwal Kerja grid for 05–11/10.
func gridCell(t *testing.T, h *harness, hr *testutil.Staff, branch, emp string, i int) (code, source string) {
	t.Helper()
	g := h.do(hr, http.MethodGet, "/api/hris/schedules?branch_id="+branch+"&from=2026-10-05&to=2026-10-11", nil)
	expect(t, g, http.StatusOK, "")
	for _, e := range g.data()["employees"].([]any) {
		m := e.(map[string]any)
		if m["id"] == emp {
			c := m["cells"].([]any)[i].(map[string]any)
			return c["code"].(string), c["source"].(string)
		}
	}
	t.Fatalf("employee %s not in grid", emp)
	return "", ""
}

func wantSummary(t *testing.T, c call, want map[string]float64) {
	t.Helper()
	sum := c.data()["summary"].(map[string]any)
	for k, v := range want {
		if sum[k] != v {
			t.Fatalf("summary %s = %v, want %v (%v)", k, sum[k], v, sum)
		}
	}
}

// commitCells previews body then commits it with the preview fingerprint.
func commitCells(t *testing.T, h *harness, s *testutil.Staff, body map[string]any) (preview, commit call) {
	t.Helper()
	body["step"] = "preview"
	preview = h.do(s, http.MethodPost, cellsPath, body)
	expect(t, preview, http.StatusOK, "")
	body["step"], body["fingerprint"], body["ack_warnings"] = "commit", preview.data()["fingerprint"], true
	return preview, h.do(s, http.MethodPost, cellsPath, body)
}

func TestScheduleCellsPreviewCommit(t *testing.T) {
	h := newHarness(t)
	writer := h.staffActions("hrd", scheduleWriter)
	deleter, _ := h.staff("hrd", scheduleTestMenus, false)
	branch, ana, _ := scheduleFixture(h)
	ns := h.scalar(`SELECT id::text FROM hris.shifts WHERE code = 'NS'`)
	// Pola mingguan Ana: Selasa = NS.
	h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from) VALUES ($1, 2, $2, '2026-01-01')`, ana, ns)
	if code, src := gridCell(t, h, &writer, branch, ana, 1); code != "NS" || src != "weekly" {
		t.Fatalf("06/10 before = %s/%s, want NS/weekly", code, src)
	}

	// 06/10 NS (mingguan) → DEF, 07/10 kosong → L: nothing per-date replaced.
	body := cellsBody(branch, "", anaCell(ana, "2026-10-06", "def"), anaCell(ana, "2026-10-07", "L"))
	pre, done := commitCells(t, h, &writer, body)
	wantSummary(t, pre, map[string]float64{"employees": 1, "new": 1, "changed": 1, "same": 0, "cleared": 0})
	if pre.data()["replaced_days"] != float64(0) || len(pre.data()["errors"].([]any)) != 0 || len(pre.data()["changes"].([]any)) != 2 {
		t.Fatalf("preview = %s", pre.raw)
	}
	expect(t, done, http.StatusOK, "")
	if done.body["message"] != "Jadwal 1 karyawan disimpan" {
		t.Fatalf("message = %v", done.body["message"])
	}
	if code, src := gridCell(t, h, &writer, branch, ana, 1); code != "DEF" || src != "day" {
		t.Fatalf("06/10 after = %s/%s, want DEF/day", code, src)
	}
	if code, src := gridCell(t, h, &writer, branch, ana, 2); code != "L" || src != "day" {
		t.Fatalf("07/10 after = %s/%s, want L/day", code, src)
	}

	// Overwriting the per-date L with DEF needs delete on Jadwal Kerja.
	pre, done = commitCells(t, h, &writer, cellsBody(branch, "", anaCell(ana, "2026-10-07", "DEF")))
	if pre.data()["replaced_days"] != float64(1) {
		t.Fatalf("replaced_days = %v, want 1", pre.data()["replaced_days"])
	}
	expect(t, done, http.StatusForbidden, scheduleNeedsDelete)

	// Clearing 06/10 removes its per-date row: back to the weekly NS.
	clear := cellsBody(branch, "", anaCell(ana, "2026-10-06", ""), anaCell(ana, "2026-10-08", ""))
	pre, done = commitCells(t, h, &writer, clear)
	wantSummary(t, pre, map[string]float64{"cleared": 1, "same": 1}) // 08/10 has no per-date row
	if pre.data()["replaced_days"] != float64(1) {
		t.Fatalf("replaced_days = %v, want 1", pre.data()["replaced_days"])
	}
	expect(t, done, http.StatusForbidden, scheduleNeedsDelete)
	expect(t, h.do(&deleter, http.MethodPost, cellsPath, clear), http.StatusBadRequest, hrisdel.ErrReason)
	clear["reason"] = "Kembali ke pola mingguan"
	expect(t, h.do(&deleter, http.MethodPost, cellsPath, clear), http.StatusOK, "")
	if code, src := gridCell(t, h, &writer, branch, ana, 1); code != "NS" || src != "weekly" {
		t.Fatalf("06/10 after clear = %s/%s, want NS/weekly", code, src)
	}
	if got := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'hris.schedule.replace'
		AND reason = 'Kembali ke pola mingguan' AND (before->>'replaced_days')::int = 1`); got != "1" {
		t.Fatalf("replace audit rows = %s", got)
	}
	if got := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'schedule.cells_update' AND entity_id = $1`, ana); got != "2" {
		t.Fatalf("cells_update audit rows = %s, want 2", got)
	}

	// Someone else writes 09/10 after the preview: the commit is stale.
	stale := cellsBody(branch, "preview", anaCell(ana, "2026-10-09", "DEF"))
	pre = h.do(&writer, http.MethodPost, cellsPath, stale)
	expect(t, pre, http.StatusOK, "")
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id) VALUES ($1, '2026-10-09', NULL, gen_random_uuid())`, ana)
	stale["step"], stale["fingerprint"], stale["ack_warnings"] = "commit", pre.data()["fingerprint"], true
	expect(t, h.do(&writer, http.MethodPost, cellsPath, stale), http.StatusConflict, "Jadwal sudah diubah orang lain sejak preview — muat ulang jadwal")
}

func TestScheduleCellsRejects(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", scheduleTestMenus, false)
	branch, ana, budi := scheduleFixture(h)

	// Rule errors come back in the preview and block the commit.
	body := cellsBody(branch, "", anaCell(ana, "2026-10-04", "DEF"), anaCell(budi, "2026-10-06", "DEF"), anaCell(ana, "2026-10-07", "XX"))
	pre, done := commitCells(t, h, &hr, body)
	errs := pre.data()["errors"].([]any)
	if len(errs) != 3 {
		t.Fatalf("errors = %v (past date, Budi not placed, unknown code)", errs)
	}
	expect(t, done, http.StatusBadRequest, "Jadwal masih punya error — perbaiki dulu sebelum menyimpan")

	// Warnings need the acknowledgement: Ana 7 hari berturut-turut.
	var week []map[string]any
	for _, d := range []string{"2026-10-05", "2026-10-06", "2026-10-07", "2026-10-08", "2026-10-09", "2026-10-10", "2026-10-11"} {
		week = append(week, anaCell(ana, d, "DEF"))
	}
	pre = h.do(&hr, http.MethodPost, cellsPath, cellsBody(branch, "preview", week...))
	expect(t, pre, http.StatusOK, "")
	if len(pre.data()["warnings"].([]any)) == 0 {
		t.Fatalf("want streak warning, got %s", pre.raw)
	}
	noAck := cellsBody(branch, "commit", week...)
	noAck["fingerprint"] = pre.data()["fingerprint"]
	expect(t, h.do(&hr, http.MethodPost, cellsPath, noAck), http.StatusBadRequest, `Centang "Saya sudah memeriksa peringatan" sebelum menyimpan`)

	// Request shape: no cells, more than 62 days, invalid date.
	empty := map[string]any{"branch_id": branch, "step": "preview", "cells": []any{}}
	expect(t, h.do(&hr, http.MethodPost, cellsPath, empty), http.StatusBadRequest, "Tidak ada jadwal yang diubah")
	wide := cellsBody(branch, "preview", anaCell(ana, "2026-10-05", "DEF"), anaCell(ana, "2026-12-06", "DEF"))
	expect(t, h.do(&hr, http.MethodPost, cellsPath, wide), http.StatusBadRequest, "Rentang tanggal maksimal 62 hari")
	bad := cellsBody(branch, "preview", anaCell(ana, "2026-02-31", "DEF"))
	expect(t, h.do(&hr, http.MethodPost, cellsPath, bad), http.StatusBadRequest, "Tanggal 2026-02-31 tidak valid")
}

// Reading Jadwal Kerja is not enough to save cells: without "update" on the
// leaf both steps answer 403 and nothing is written.
func TestScheduleCellsNeedUpdate(t *testing.T) {
	h := newHarness(t)
	reader := h.staffActions("hrd", map[string][]string{
		"hris.kepegawaian":  {"read"},
		iam.LeafJadwalKerja: {"read", "export"},
	})
	branch, ana, _ := scheduleFixture(h)
	for _, step := range []string{"preview", "commit"} {
		body := cellsBody(branch, step, anaCell(ana, "2026-10-06", "DEF"))
		expect(t, h.do(&reader, http.MethodPost, cellsPath, body), http.StatusForbidden, "")
	}
	if n := h.scalar(`SELECT count(*)::text FROM hris.employee_shift_days WHERE employee_id = $1`, ana); n != "0" {
		t.Fatalf("read-only user wrote %s schedule rows", n)
	}
	// The grid itself stays readable.
	expect(t, h.do(&reader, http.MethodGet, "/api/hris/schedules?branch_id="+branch+"&from=2026-10-05&to=2026-10-11", nil), http.StatusOK, "")
}

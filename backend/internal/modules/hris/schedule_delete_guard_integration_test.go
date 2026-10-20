package hris

import (
	"bytes"
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/xuri/excelize/v2"

	"nuhabit/backend/internal/platform/hrisdel"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/testutil"
)

const scheduleNeedsDelete = "Menimpa atau menghapus jadwal yang sudah ada butuh hak hapus Jadwal Kerja"

// scheduleWriter edits Jadwal Kerja and employees without "delete".
var scheduleWriter = map[string][]string{
	"hris.kepegawaian":  {"read", "update"},
	iam.LeafJadwalKerja: {"read", "create", "update", "import", "export"},
}

// staffActions is h.staff with explicit actions per menu.
func (h *harness) staffActions(role string, grants map[string][]string) testutil.Staff {
	h.t.Helper()
	s := testutil.CreateStaff(h.t, testutil.StaffOptions{Role: role, Menus: grants})
	h.t.Cleanup(func() { _ = h.tx.Rollback(context.Background()) })
	return s
}

// anaWeek is the Jadwal Kerja file for one employee over 05–11/10.
func anaWeek(branch, nip string, cells []string) []byte {
	f := excelize.NewFile()
	_ = f.SetSheetName("Sheet1", "Jadwal")
	head := []string{"NIP", "Nama Karyawan", "Jabatan", "Sen 05/10", "Sel 06/10", "Rab 07/10", "Kam 08/10", "Jum 09/10", "Sab 10/10", "Min 11/10"}
	row := append([]string{nip, "Ana Shift", ""}, cells...)
	for i := range head {
		c1, _ := excelize.CoordinatesToCellName(i+1, 3)
		c2, _ := excelize.CoordinatesToCellName(i+1, 4)
		_ = f.SetCellStr("Jadwal", c1, head[i])
		_ = f.SetCellStr("Jadwal", c2, row[i])
	}
	_, _ = f.NewSheet("Info")
	for i, kv := range [][2]string{{"Cabang ID", branch}, {"Dari", "2026-10-05"}, {"Sampai", "2026-10-11"}} {
		_ = f.SetCellStr("Info", "A"+strconv.Itoa(i+1), kv[0])
		_ = f.SetCellStr("Info", "B"+strconv.Itoa(i+1), kv[1])
	}
	var buf bytes.Buffer
	_ = f.Write(&buf)
	return buf.Bytes()
}

func TestScheduleUploadReplacingDaysNeedsDelete(t *testing.T) {
	h := newHarness(t)
	writer := h.staffActions("hrd", scheduleWriter)
	deleter, _ := h.staff("hrd", scheduleTestMenus, false)
	branch, ana, _ := scheduleFixture(h)
	nip := h.scalar(`SELECT nip FROM hris.employees WHERE id = $1`, ana)
	def := h.scalar(`SELECT id::text FROM hris.shifts WHERE code = 'DEF'`)
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id)
		SELECT $1, d::date, $2, gen_random_uuid() FROM generate_series('2026-10-05'::date, '2026-10-11', interval '1 day') d`, ana, def)
	file := anaWeek(branch, nip, []string{"DEF", "L", "DEF", "DEF", "DEF", "L", "DEF"})
	fields := map[string]string{"branch_id": branch, "from": "2026-10-05", "to": "2026-10-11", "step": "preview"}
	pre := upload(t, h, &writer, fields, file)
	expect(t, pre, http.StatusOK, "")
	if pre.data()["replaced_days"] != float64(2) {
		t.Fatalf("replaced_days = %v, want 2 (06/10 & 10/10 DEF→L)", pre.data()["replaced_days"])
	}
	fields["step"], fields["fingerprint"], fields["ack_warnings"] = "commit", pre.data()["fingerprint"].(string), "true"
	expect(t, upload(t, h, &writer, fields, file), http.StatusForbidden, scheduleNeedsDelete)
	expect(t, upload(t, h, &deleter, fields, file), http.StatusBadRequest, hrisdel.ErrReason)
	fields["reason"] = "Revisi jadwal minggu ini"
	expect(t, upload(t, h, &deleter, fields, file), http.StatusOK, "")
	if got := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'hris.schedule.replace'
		AND reason = 'Revisi jadwal minggu ini' AND (before->>'replaced_days')::int = 2`); got != "1" {
		t.Fatalf("replace audit rows = %s", got)
	}
}

func TestScheduleTypeClearingDaysNeedsDelete(t *testing.T) {
	h := newHarness(t)
	writer := h.staffActions("hrd", scheduleWriter)
	deleter, _ := h.staff("hrd", scheduleTestMenus, false)
	e := h.employee(emp{})
	free := h.employee(emp{})
	def := h.scalar(`SELECT id::text FROM hris.shifts WHERE code = 'DEF'`)
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id) VALUES
		($1, '2026-10-07', $2, gen_random_uuid()), ($1, '2026-10-08', $2, gen_random_uuid())`, e, def)
	body := map[string]any{"schedule_type": "non_shift", "effective_from": "2026-10-06"}
	// Nothing to clear: no delete needed.
	expect(t, h.do(&writer, http.MethodPut, "/api/hris/employees/"+free+"/schedule-type", body), http.StatusOK, "")
	expect(t, h.do(&writer, http.MethodPut, "/api/hris/employees/"+e+"/schedule-type", body), http.StatusForbidden, scheduleNeedsDelete)
	if h.scalar(`SELECT schedule_type FROM hris.employees WHERE id = $1`, e) != "shift" {
		t.Fatal("a refused change must not apply")
	}
	expect(t, h.do(&deleter, http.MethodPut, "/api/hris/employees/"+e+"/schedule-type", body), http.StatusBadRequest, hrisdel.ErrReason)
	body["reason"] = "Pindah ke kantor pusat"
	expect(t, h.do(&deleter, http.MethodPut, "/api/hris/employees/"+e+"/schedule-type", body), http.StatusOK, "")
	if got := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'hris.schedule.clear' AND entity_id = $1
		AND reason = 'Pindah ke kantor pusat' AND (before->>'cleared_days')::int = 2`, e); got != "1" {
		t.Fatalf("clear audit rows = %s", got)
	}
	// Jadwal tetap on a Non-Shift employee who still has a per-date day.
	ns := h.employee(emp{})
	h.exec(`UPDATE hris.employees SET schedule_type = 'non_shift' WHERE id = $1`, ns)
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id) VALUES ($1, '2026-10-09', $2, gen_random_uuid())`, ns, def)
	fixed := map[string]any{"employee_ids": []string{ns}, "effective_from": "2026-10-06"}
	expect(t, h.do(&writer, http.MethodPost, "/api/hris/schedules/fixed", fixed), http.StatusForbidden, scheduleNeedsDelete)
	fixed["reason"] = "Bersihkan jadwal lama"
	expect(t, h.do(&deleter, http.MethodPost, "/api/hris/schedules/fixed", fixed), http.StatusOK, "")
}

func TestEmployeeImportClearingDaysNeedsDelete(t *testing.T) {
	h := newHarness(t)
	writer := h.staffActions("hrd", scheduleWriter)
	deleter, _ := h.staff("hrd", scheduleTestMenus, false)
	e := h.employee(emp{})
	nip := h.scalar(`SELECT nip FROM hris.employees WHERE id = $1`, e)
	def := h.scalar(`SELECT id::text FROM hris.shifts WHERE code = 'DEF'`)
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id) VALUES ($1, '2026-10-08', $2, gen_random_uuid())`, e, def)
	header := []string{"NIP", "Nama Lengkap *", "Email *", "Tanggal Masuk *", "Status Kepegawaian *", "Tipe Jadwal *", "Cabang Utama *", "Jenis Kelamin"}
	file := employeeFile(t, header, []string{nip, "", "", "", "", "Non-Shift", "", ""})
	fields := map[string]string{"mode": "replace", "step": "commit"}
	expect(t, uploadEmployees(t, h, &writer, fields, file), http.StatusForbidden, scheduleNeedsDelete)
	if h.scalar(`SELECT schedule_type FROM hris.employees WHERE id = $1`, e) != "shift" {
		t.Fatal("a refused import must not apply")
	}
	// The import UI sends no reason: the file name stands in for it.
	expect(t, uploadEmployees(t, h, &deleter, fields, file), http.StatusOK, "")
	if got := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'hris.schedule.clear' AND entity_id = $1
		AND reason = 'Impor data karyawan: data.xlsx'`, e); got != "1" {
		t.Fatalf("clear audit rows = %s", got)
	}
}

package hris

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// EPIC-080 final review: edges of the automatic checkout.

// A manual clock-out on a row someone closed by hand keeps the plain message.
func TestClockOutManualRowKeepsPlainMessage(t *testing.T) {
	h, _ := newFileHarness(t)
	photo := dataURL("image/png", pngBytes(t))
	staff, e := h.staff("pos", nil, true, emp{name: "Crew Tutup Manual"})
	id := h.scalar(`INSERT INTO hris.attendance (employee_id, date, clock_in, clock_out, scheduled_end)
		VALUES ($1, '2026-10-05', '2026-10-05 08:00:00+07', '2026-10-05 15:00:00+07', '2026-10-05 16:00:00+07') RETURNING id::text`, e)
	h.setNow("2026-10-05 16:10")
	expect(t, h.do(&staff, "POST", "/api/hris/attendance", map[string]any{"action": "clock-out", "attendance_id": id, "photo": photo}),
		400, "Sudah clock-out untuk absensi ini")
}

// Item 3: the correction clears the marker when it changes clock_out even
// though it read the row before the job closed it (stale marker).
func TestAdjustClearsMarkerDespiteStaleRead(t *testing.T) {
	h := newHarness(t)
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 0, closing_offset_days = 3`)
	hr, _ := h.staff("hrd", []string{"hris.workforce.attendance"}, true, emp{name: "HR Stale Marker"})
	e := h.employee(emp{name: "Crew Stale Marker"})
	pos := h.scalar(`INSERT INTO hris.positions (title) VALUES ('Jabatan Stale Uji') RETURNING id::text`)
	h.exec(`INSERT INTO hris.auto_checkout_rules (position_id) VALUES ($1)`, pos)
	h.exec(`UPDATE hris.employees SET job_title_id = $2 WHERE id = $1`, e, pos)
	id := h.scalar(`INSERT INTO hris.attendance (employee_id, date, clock_in, scheduled_end, status)
		VALUES ($1, '2026-10-05', '2026-10-05 08:00:00+07', '2026-10-05 16:00:00+07', 'present') RETURNING id::text`, e)
	stale, err := h.svc.repo.AttendanceRecord(context.Background(), id)
	if err != nil || stale == nil {
		t.Fatalf("stale = %v, %v", stale, err)
	}
	h.setNow("2026-10-05 16:10")
	if n, err := h.svc.RunAutoCheckout(context.Background(), time.Date(2026, 10, 5, 16, 10, 0, 0, time.FixedZone("WIB", 7*3600))); err != nil || n != 1 {
		t.Fatalf("RunAutoCheckout = %d, %v; want 1", n, err)
	}
	h.svc.repo = staleRecordRepo{Repository: h.svc.repo, stale: stale}
	expect(t, h.do(&hr, "PUT", "/api/hris/attendance/"+id, map[string]any{"clock_out": "17:15", "reason": "Pulang lebih lama"}), 200, "")
	got := h.scalar(`SELECT to_char(clock_out AT TIME ZONE 'Asia/Jakarta', 'HH24:MI') || '|' || clock_out_auto::text
		FROM hris.attendance WHERE id = $1`, id)
	if got != "17:15|false" {
		t.Fatalf("after stale-read adjust = %s, want 17:15|false", got)
	}
}

// Item 4: a row HR deliberately reopened is not closed again by the job.
func TestAutoCheckoutSkipsRowReopenedByHR(t *testing.T) {
	h := newHarness(t)
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 0, closing_offset_days = 3`)
	hr, _ := h.staff("hrd", []string{"hris.workforce.attendance"}, true, emp{name: "HR Buka Ulang"})
	e := h.employee(emp{name: "Crew Buka Ulang"})
	pos := h.scalar(`INSERT INTO hris.positions (title) VALUES ('Jabatan Buka Ulang Uji') RETURNING id::text`)
	h.exec(`INSERT INTO hris.auto_checkout_rules (position_id) VALUES ($1)`, pos)
	h.exec(`UPDATE hris.employees SET job_title_id = $2 WHERE id = $1`, e, pos)
	id := h.scalar(`INSERT INTO hris.attendance (employee_id, date, clock_in, clock_out, clock_out_auto, scheduled_end, status)
		VALUES ($1, '2026-10-05', '2026-10-05 08:00:00+07', '2026-10-05 16:00:00+07', true, '2026-10-05 16:00:00+07', 'present') RETURNING id::text`, e)
	h.setNow("2026-10-06 09:00")
	expect(t, h.do(&hr, "PUT", "/api/hris/attendance/"+id, map[string]any{"clock_out": nil, "reason": "Masih bekerja, dibuka ulang"}), 200, "")
	if got := h.scalar(`SELECT coalesce(clock_out::text, 'null') FROM hris.attendance WHERE id = $1`, id); got != "null" {
		t.Fatalf("clock_out after reopen = %s", got)
	}
	n, err := h.svc.RunAutoCheckout(context.Background(), time.Date(2026, 10, 6, 9, 5, 0, 0, time.FixedZone("WIB", 7*3600)))
	if err != nil || n != 0 {
		t.Fatalf("RunAutoCheckout = %d, %v; want 0 (reopened by HR)", n, err)
	}
	if got := h.scalar(`SELECT coalesce(clock_out::text, 'null') || '|' || clock_out_auto::text FROM hris.attendance WHERE id = $1`, id); got != "null|false" {
		t.Fatalf("row after run = %s, want null|false", got)
	}
}

// Item 5: HR adds a row on a one-shift day: the break comes from the shift
// master (OPSSAT 0), as it does at clock-in.
func TestManualCreateBreakFromShiftMaster(t *testing.T) {
	h := newHarness(t)
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 0, closing_offset_days = 3`)
	hr, _ := h.staff("hrd", []string{"hris.workforce.attendance"}, true, emp{name: "HR Tambah Manual"})
	e := h.employee(emp{name: "Crew Tambah Manual"})
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id)
		SELECT $1, '2026-10-03'::date, id, gen_random_uuid() FROM hris.shifts WHERE code = 'OPSSAT'`, e)
	h.setNow("2026-10-06 09:00")
	res := h.do(&hr, "POST", "/api/hris/attendance/manual", map[string]any{"employee_id": e, "date": "2026-10-03",
		"clock_in": "08:00", "reason": "Lupa absen masuk, ada CCTV"})
	expect(t, res, http.StatusCreated, "")
	if res.data()["break_minutes"] != float64(0) {
		t.Fatalf("break_minutes = %v, want 0 (OPSSAT master) (%s)", res.data()["break_minutes"], res.raw)
	}
}

// Item 6: the automatic checkout audit carries "Nama · tanggal" like other
// attendance audits.
func TestAutoCheckoutAuditHasEntityLabel(t *testing.T) {
	h := newHarness(t)
	e := h.employee(emp{name: "Crew Label Audit"})
	pos := h.scalar(`INSERT INTO hris.positions (title) VALUES ('Jabatan Label Uji') RETURNING id::text`)
	h.exec(`INSERT INTO hris.auto_checkout_rules (position_id) VALUES ($1)`, pos)
	h.exec(`UPDATE hris.employees SET job_title_id = $2 WHERE id = $1`, e, pos)
	id := h.scalar(`INSERT INTO hris.attendance (employee_id, date, clock_in, scheduled_end)
		VALUES ($1, '2026-10-05', '2026-10-05 08:00:00+07', '2026-10-05 16:00:00+07') RETURNING id::text`, e)
	if n, err := h.svc.RunAutoCheckout(context.Background(), time.Date(2026, 10, 5, 16, 10, 0, 0, time.FixedZone("WIB", 7*3600))); err != nil || n != 1 {
		t.Fatalf("RunAutoCheckout = %d, %v; want 1", n, err)
	}
	if got := h.scalar(`SELECT entity_label FROM audit.audit_log WHERE action = 'attendance.auto_checkout' AND entity_id = $1`, id); got != "Crew Label Audit · 2026-10-05" {
		t.Fatalf("entity_label = %q", got)
	}
}

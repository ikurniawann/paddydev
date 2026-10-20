package hris

import (
	"strings"
	"testing"
)

// EPIC-058: kode cuti baru ada di enum, dan notifikasi in-app memberi label
// yang benar (sebelumnya "personal" tampil sebagai "Lainnya").
func TestLeaveEnumAndNotifyLabels(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuWorkforce}, true)
	_, own := h.staff("pos", nil, true, emp{name: "Pemohon Label"})
	for _, v := range []string{"leader", "marriage", "bereavement", "personal"} {
		if got := h.scalar(`SELECT $1::text::leave_type::text`, v); got != v {
			t.Fatalf("enum %s = %q", v, got)
		}
	}
	for _, c := range []struct{ code, date, label string }{
		{"leader", "2031-03-04", "Izin Leader"},
		{"personal", "2031-03-05", "Izin Pribadi"},
		{"marriage", "2031-03-06", "Menikah"},
	} {
		h.exec(`INSERT INTO hris.leaves (employee_id, leave_type, start_date, end_date, total_days, reason)
			VALUES ($1, $2::text::leave_type, $3::date, $3::date, 1, 'Uji label notifikasi')`, own, c.code, c.date)
		// now() sama sepanjang transaksi fixture, jadi cari berdasarkan isi pesan, bukan created_at.
		n := h.scalar(`SELECT count(*)::text FROM notifications WHERE user_id = $1 AND message LIKE $2`,
			hr.UserID, "Pemohon Label mengajukan cuti "+c.label+" (%")
		if n != "1" {
			t.Fatalf("%s: notifikasi berlabel %q = %s", c.code, c.label, n)
		}
	}
}

// leaderPosition gives an employee a position with the given level.
func (h *harness) leaderPosition(employeeID, level string) {
	h.t.Helper()
	pos := h.scalar(`INSERT INTO hris.positions (title, level) VALUES ('Posisi Uji '||$1, $1) RETURNING id::text`, level)
	h.exec(`UPDATE hris.employees SET job_title_id = $2 WHERE id = $1`, employeeID, pos)
}

func leaveBody(kind, start, end string) map[string]any {
	return map[string]any{"leave_type": kind, "start_date": start, "end_date": end, "reason": "Keperluan uji EPIC-058"}
}

func TestRemovedLeaveTypesRefusedForNewRequests(t *testing.T) {
	h := newHarness(t)
	staff, _ := h.staff("pos", nil, true)
	for _, kind := range []string{"emergency", "unpaid", "menstrual"} {
		expect(t, h.do(&staff, "POST", "/api/hris/leaves", leaveBody(kind, "2031-03-04", "2031-03-04")), 400,
			"Jenis cuti ini sudah tidak berlaku untuk pengajuan baru")
	}
	// Menikah kini tersimpan (dulu error enum).
	expect(t, h.do(&staff, "POST", "/api/hris/leaves", leaveBody("marriage", "2031-03-04", "2031-03-06")), 200, "")
	expect(t, h.do(&staff, "POST", "/api/hris/leaves", leaveBody("lainnya", "2031-03-10", "2031-03-10")), 400, "Validation failed")
}

func TestRemovedLeaveTypesStayReadable(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuWorkforce}, true)
	_, own := h.staff("pos", nil, true, emp{name: "Riwayat Darurat"})
	id := h.scalar(`INSERT INTO hris.leaves (employee_id, leave_type, start_date, end_date, total_days, reason)
		VALUES ($1, 'emergency', '2031-03-04', '2031-03-04', 1, 'Import Odoo: Cuti Menikah') RETURNING id::text`, own)
	list := h.do(&hr, "GET", "/api/hris/leaves?leave_type=emergency&employee_id="+own, nil)
	expect(t, list, 200, "")
	if len(list.list()) != 1 {
		t.Fatalf("filter emergency = %s", list.raw)
	}
	expect(t, h.do(&hr, "POST", "/api/hris/leaves/approve", map[string]any{"leave_id": id, "action": "approve"}), 200, "")
	csv := h.do(&hr, "GET", "/api/hris/leaves/export?leave_type=emergency", nil)
	expect(t, csv, 200, "")
	if !strings.Contains(csv.raw, `"Cuti Darurat"`) {
		t.Fatalf("csv = %s", csv.raw)
	}
}

func TestLeaderLeaveEligibility(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuWorkforce}, true)
	staff, staffID := h.staff("pos", nil, true)
	spv, spvID := h.staff("pos", nil, true)
	lower, lowerID := h.staff("pos", nil, true)
	_, noPosID := h.staff("pos", nil, true)
	h.leaderPosition(staffID, "Staff")
	h.leaderPosition(spvID, "Supervisor")
	h.leaderPosition(lowerID, "manager")

	expect(t, h.do(&staff, "POST", "/api/hris/leaves", leaveBody("leader", "2031-03-04", "2031-03-04")), 400,
		"Izin Leader hanya untuk jabatan Supervisor ke atas")
	expect(t, h.do(&hr, "POST", "/api/hris/leaves", map[string]any{"employee_id": noPosID, "leave_type": "leader",
		"start_date": "2031-03-04", "end_date": "2031-03-04", "reason": "Diinput HR tanpa jabatan"}), 400,
		"Izin Leader hanya untuk jabatan Supervisor ke atas")
	expect(t, h.do(&spv, "POST", "/api/hris/leaves", leaveBody("leader", "2031-03-04", "2031-03-05")), 400,
		"Izin Leader diajukan per satu hari — tanggal mulai dan selesai harus sama")
	expect(t, h.do(&spv, "POST", "/api/hris/leaves", leaveBody("leader", "2031-03-04", "2031-03-04")), 200, "")
	expect(t, h.do(&lower, "POST", "/api/hris/leaves", leaveBody("leader", "2031-03-04", "2031-03-04")), 200, "")
	// Kuota 1/bulan.
	expect(t, h.do(&spv, "POST", "/api/hris/leaves", leaveBody("leader", "2031-03-11", "2031-03-11")), 400,
		"Kuota Izin Leader bulan 2031-03 sudah habis (1 dari 1 hari)")
}

func TestLeaveMonthlyQuota(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuWorkforce}, true)
	staff, _ := h.staff("pos", nil, true)
	var ids []string
	for _, d := range []string{"2031-03-04", "2031-03-05", "2031-03-06"} {
		res := h.do(&staff, "POST", "/api/hris/leaves", leaveBody("personal", d, d))
		expect(t, res, 200, "")
		ids = append(ids, res.data()["id"].(string))
	}
	expect(t, h.do(&staff, "POST", "/api/hris/leaves", leaveBody("personal", "2031-03-31", "2031-03-31")), 400,
		"Kuota Izin Pribadi bulan 2031-03 sudah habis (3 dari 3 hari)")
	// Bulan lain punya kuota sendiri.
	expect(t, h.do(&staff, "POST", "/api/hris/leaves", leaveBody("personal", "2031-04-01", "2031-04-01")), 200, "")
	// Ditolak → kuota kembali.
	expect(t, h.do(&hr, "POST", "/api/hris/leaves/approve", map[string]any{"leave_id": ids[0], "action": "reject",
		"rejection_reason": "Uji kuota"}), 200, "")
	expect(t, h.do(&staff, "POST", "/api/hris/leaves", leaveBody("personal", "2031-03-31", "2031-03-31")), 200, "")
	// Kombinasi: jenis lain di bulan yang sama tetap boleh, kuotanya terpisah.
	expect(t, h.do(&staff, "POST", "/api/hris/leaves", leaveBody("annual", "2031-03-11", "2031-03-12")), 200, "")
}

func TestLeaveOverlapRefused(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuWorkforce}, true)
	staff, _ := h.staff("pos", nil, true)
	annual := h.do(&staff, "POST", "/api/hris/leaves", leaveBody("annual", "2031-03-04", "2031-03-06"))
	expect(t, annual, 200, "")
	expect(t, h.do(&staff, "POST", "/api/hris/leaves", leaveBody("personal", "2031-03-05", "2031-03-05")), 400,
		"Tanggal ini bertabrakan dengan pengajuan Cuti Tahunan (2031-03-04 s.d. 2031-03-06, menunggu persetujuan)")
	// Bersebelahan boleh (dikombinasikan).
	expect(t, h.do(&staff, "POST", "/api/hris/leaves", leaveBody("personal", "2031-03-07", "2031-03-07")), 200, "")
	// Setelah dibatalkan, tanggalnya bebas lagi.
	id := annual.data()["id"].(string)
	expect(t, h.do(&hr, "PUT", "/api/hris/leaves/"+id, map[string]any{"status": "cancelled"}), 200, "")
	expect(t, h.do(&staff, "POST", "/api/hris/leaves", leaveBody("personal", "2031-03-05", "2031-03-05")), 200, "")
}

func TestLeaveQuotaEndpoint(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{menuWorkforce}, true)
	hrNoMenu, _ := h.staff("hrd", nil, true)
	spv, spvID := h.staff("pos", nil, true)
	stranger, _ := h.staff("pos", nil, true)
	h.leaderPosition(spvID, "Supervisor")
	expect(t, h.do(&spv, "POST", "/api/hris/leaves", leaveBody("personal", "2031-03-04", "2031-03-04")), 200, "")

	own := h.do(&spv, "GET", "/api/hris/leave-quota?month=2031-03", nil)
	expect(t, own, 200, "")
	d := own.data()
	if d["leader_eligible"] != true || d["month"] != "2031-03" {
		t.Fatalf("quota = %s", own.raw)
	}
	types := d["types"].([]any)
	personal := types[0].(map[string]any)
	if personal["leave_type"] != "personal" || personal["used"] != 1.0 || personal["remaining"] != 2.0 {
		t.Fatalf("personal = %v", personal)
	}
	if strings.Contains(own.raw, "Rp") || strings.Contains(own.raw, "salary") {
		t.Fatalf("kuota tidak boleh memuat angka gaji: %s", own.raw)
	}

	expect(t, h.do(&hr, "GET", "/api/hris/leave-quota?month=2031-03&employee_id="+spvID, nil), 200, "")
	expect(t, h.do(&hrNoMenu, "GET", "/api/hris/leave-quota?month=2031-03&employee_id="+spvID, nil), 403, "Insufficient permissions")
	expect(t, h.do(&stranger, "GET", "/api/hris/leave-quota?month=2031-03&employee_id="+spvID, nil), 403, "Insufficient permissions")
	expect(t, h.do(&spv, "GET", "/api/hris/leave-quota?month=2031-3", nil), 400, "Format bulan harus YYYY-MM")
	staffQ := h.do(&stranger, "GET", "/api/hris/leave-quota?month=2031-03", nil)
	expect(t, staffQ, 200, "")
	if staffQ.data()["leader_eligible"] != false || staffQ.data()["types"].([]any)[1].(map[string]any)["available"] != false {
		t.Fatalf("staff quota = %s", staffQ.raw)
	}
}

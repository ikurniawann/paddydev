package hris

import (
	"testing"
	"time"

	"nuhabit/backend/internal/platform/testutil"
)

func TestBusinessTripFlow(t *testing.T) {
	h, _ := newFileHarness(t)
	hr, _ := h.staff("hrd", nil, true)
	boss, bossID := h.staff("pos", nil, true, emp{name: "Atasan Dinas"})
	staff, own := h.staff("pos", nil, true, emp{name: "Crew Dinas", boss: &bossID})
	other, otherID := h.staff("pos", nil, true, emp{name: "Rekan Lain"})
	store := h.branch("Toko Dinas")
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date) VALUES
		($1, $3, true, '2026-01-01'), ($2, $3, true, '2026-01-01')`, own, otherID, store)
	photo := dataURL("image/png", pngBytes(t))
	base := "/api/hris/business-trips"
	trip := func(start, end string) map[string]any {
		return map[string]any{"start_date": start, "end_date": end, "destination": "Gudang Cikarang", "purpose": "Stock opname gudang"}
	}
	decide := func(s *testutil.Staff, id, action, reason string) call {
		body := map[string]any{"action": action}
		if reason != "" {
			body["rejection_reason"] = reason
		}
		return h.do(s, "POST", base+"/"+id+"/decide", body)
	}

	// Validasi tanggal (hari ini di harness = 2026-10-05).
	expect(t, h.do(&staff, "POST", base, trip("2026-10-04", "2026-10-05")), 400, "Tanggal mulai tidak boleh sebelum hari ini")
	expect(t, h.do(&staff, "POST", base, trip("2026-10-07", "2026-10-06")), 400, "Tanggal selesai tidak boleh sebelum tanggal mulai")
	expect(t, h.do(&staff, "POST", base, trip("2026-10-05", "2026-11-05")), 400, "Perjalanan Dinas maksimal 31 hari per pengajuan")
	expect(t, h.do(&staff, "POST", base, map[string]any{"start_date": "2026-10-05", "end_date": "2026-10-05",
		"destination": "Gudang", "purpose": "abc"}), 400, "Validation failed")
	expect(t, h.do(&staff, "POST", base, map[string]any{"employee_id": otherID, "start_date": "2026-10-05",
		"end_date": "2026-10-05", "destination": "Gudang", "purpose": "Audit toko"}), 403,
		"Hanya HRD yang bisa mencatat Perjalanan Dinas karyawan lain")

	created := h.do(&staff, "POST", base, trip("2026-10-05", "2026-10-07"))
	expect(t, created, 200, "")
	d := created.data()
	id := d["id"].(string)
	if d["status"] != "pending" || d["start_date"] != "2026-10-05" || d["end_date"] != "2026-10-07" ||
		created.body["message"] != "Pengajuan Perjalanan Dinas terkirim — menunggu persetujuan atasan" {
		t.Fatalf("created = %s", created.raw)
	}
	expect(t, h.do(&staff, "POST", base, trip("2026-10-07", "2026-10-08")), 400,
		"Sudah ada Perjalanan Dinas menunggu/disetujui yang beririsan dengan tanggal ini")

	// Daftar: milik sendiri, antrean atasan, dan rekan lain tidak melihatnya.
	if n := len(h.do(&staff, "GET", base, nil).list()); n != 1 {
		t.Fatalf("own list = %d", n)
	}
	if n := len(h.do(&boss, "GET", base+"?scope=approvals&status=pending", nil).list()); n != 1 {
		t.Fatalf("boss queue = %d", n)
	}
	if n := len(h.do(&other, "GET", base+"?scope=approvals", nil).list()); n != 0 {
		t.Fatalf("other queue = %d", n)
	}
	expect(t, h.do(&hr, "GET", base+"?status=nope", nil), 400, "Status tidak valid")

	// Sebelum disetujui: absen di luar area tetap butuh keterangan, tercatat Invalid.
	in := h.do(&staff, "POST", "/api/hris/attendance", clockInBody(photo, farFromStore, "Berangkat ke gudang"))
	expect(t, in, 200, "")
	attID := in.data()["id"].(string)
	if in.data()["clock_in_geofence"] != "invalid" {
		t.Fatalf("before approval = %s", in.raw)
	}

	expect(t, decide(&staff, id, "approve", ""), 403, "Tidak bisa memutuskan Perjalanan Dinas sendiri")
	expect(t, decide(&other, id, "approve", ""), 403, "Hanya atasan langsung atau HRD yang bisa memproses pengajuan ini")
	expect(t, decide(&boss, id, "reject", ""), 400, "Alasan penolakan wajib diisi")
	approved := decide(&boss, id, "approve", "")
	expect(t, approved, 200, "")
	if approved.data()["status"] != "approved" || approved.body["message"] != "Perjalanan Dinas disetujui" {
		t.Fatalf("approved = %s", approved.raw)
	}
	expect(t, decide(&hr, id, "approve", ""), 400, "Pengajuan sudah approved")
	// Absen pagi tadi ikut menjadi Perjalanan Dinas (R8).
	if g := h.scalar(`SELECT clock_in_geofence FROM hris.attendance WHERE id = $1`, attID); g != "dinas" {
		t.Fatalf("relabel = %q", g)
	}
	// Clock-out jauh pada tanggal dinas: tanpa keterangan.
	co := h.do(&staff, "POST", "/api/hris/attendance", map[string]any{"action": "clock-out", "attendance_id": attID,
		"photo": photo, "clock_out_location": farFromStore})
	expect(t, co, 200, "")
	if co.data()["clock_out_geofence"] != "dinas" || co.data()["clock_out_distance_m"] != 334.0 {
		t.Fatalf("clock-out on trip = %s", co.raw)
	}

	// HRD mencatat untuk karyawan lain: langsung disetujui, absen jauh tanpa keterangan.
	assigned := h.do(&hr, "POST", base, map[string]any{"employee_id": otherID, "start_date": "2026-10-05",
		"end_date": "2026-10-05", "destination": "Kantor pusat", "purpose": "Rapat bulanan"})
	expect(t, assigned, 200, "")
	if assigned.data()["status"] != "approved" || assigned.body["message"] != "Perjalanan Dinas dicatat dan disetujui" {
		t.Fatalf("assigned = %s", assigned.raw)
	}
	onTrip := h.do(&other, "POST", "/api/hris/attendance", clockInBody(photo, farFromStore, ""))
	expect(t, onTrip, 200, "")
	if onTrip.data()["clock_in_geofence"] != "dinas" {
		t.Fatalf("clock-in on trip = %s", onTrip.raw)
	}

	// Pembatalan: pemohon tidak bisa membatalkan yang sudah disetujui, HRD bisa.
	expect(t, decide(&staff, id, "cancel", ""), 403, "Hanya pemohon (saat menunggu) atau HRD yang bisa membatalkan Perjalanan Dinas")
	cancelled := decide(&hr, id, "cancel", "")
	expect(t, cancelled, 200, "")
	if cancelled.data()["status"] != "cancelled" || cancelled.body["message"] != "Perjalanan Dinas dibatalkan" {
		t.Fatalf("cancel after approve = %s", cancelled.raw)
	}
	// P17: membatalkan tidak mengembalikan absen yang sudah menjadi dinas.
	if g := h.scalar(`SELECT clock_in_geofence || '/' || clock_out_geofence FROM hris.attendance WHERE id = $1`, attID); g != "dinas/dinas" {
		t.Fatalf("after cancel = %q", g)
	}
	expect(t, decide(&hr, id, "cancel", ""), 403, "Hanya pemohon (saat menunggu) atau HRD yang bisa membatalkan Perjalanan Dinas")

	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE entity = 'hris.business_trips' AND entity_id = $1`, id); n != "2" {
		t.Fatalf("audit rows (approve + cancel) = %s", n)
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'business_trip.assign'
		AND entity_id = $1`, assigned.data()["id"]); n != "1" {
		t.Fatalf("audit assign = %s", n)
	}
}

// P7: tolak (alasan tersimpan, tanggal bebas lagi), batal saat menunggu, dan
// HRD sebagai cadangan menyetujui: absen di rentang itu yang Invalid/Belum
// dicek menjadi dinas, kecuali di periode yang sudah closing.
func TestBusinessTripRejectCancelAndClosedPeriod(t *testing.T) {
	h, _ := newFileHarness(t)
	hr, _ := h.staff("hrd", nil, true)
	boss, bossID := h.staff("pos", nil, true, emp{name: "Atasan Tolak"})
	staff, own := h.staff("pos", nil, true, emp{name: "Crew Tolak", boss: &bossID})
	base := "/api/hris/business-trips"
	trip := func(start, end string) map[string]any {
		return map[string]any{"start_date": start, "end_date": end, "destination": "Gudang Cikarang", "purpose": "Stock opname gudang"}
	}
	decide := func(s *testutil.Staff, id, action, reason string) call {
		body := map[string]any{"action": action}
		if reason != "" {
			body["rejection_reason"] = reason
		}
		return h.do(s, "POST", base+"/"+id+"/decide", body)
	}

	// Tolak: alasan tersimpan dan diaudit; yang ditolak tidak bisa dibatalkan.
	first := h.do(&staff, "POST", base, trip("2026-10-10", "2026-10-12"))
	expect(t, first, 200, "")
	firstID := first.data()["id"].(string)
	rejected := decide(&boss, firstID, "reject", "  Jadwal toko padat  ")
	expect(t, rejected, 200, "")
	if d := rejected.data(); d["status"] != "rejected" || d["rejection_reason"] != "Jadwal toko padat" ||
		d["decided_by"] != bossID || d["decided_by_name"] == nil || d["decided_at"] == nil ||
		rejected.body["message"] != "Perjalanan Dinas ditolak" {
		t.Fatalf("rejected = %s", rejected.raw)
	}
	expect(t, decide(&staff, firstID, "cancel", ""), 403, "Hanya pemohon (saat menunggu) atau HRD yang bisa membatalkan Perjalanan Dinas")
	expect(t, decide(&hr, firstID, "approve", ""), 400, "Pengajuan sudah rejected")
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'business_trip.reject' AND entity_id = $1`, firstID); n != "1" {
		t.Fatalf("audit reject = %s", n)
	}

	// Tanggal yang ditolak bisa diajukan lagi; pemohon membatalkan saat menunggu.
	again := h.do(&staff, "POST", base, trip("2026-10-10", "2026-10-12"))
	expect(t, again, 200, "")
	againID := again.data()["id"].(string)
	expect(t, decide(&boss, againID, "cancel", ""), 403, "Hanya pemohon (saat menunggu) atau HRD yang bisa membatalkan Perjalanan Dinas")
	own1 := decide(&staff, againID, "cancel", "")
	expect(t, own1, 200, "")
	if own1.data()["status"] != "cancelled" || own1.body["message"] != "Perjalanan Dinas dibatalkan" {
		t.Fatalf("self cancel = %s", own1.raw)
	}

	// Rentang melintasi closing: Oktober ditutup, November terbuka.
	pending := h.do(&staff, "POST", base, trip("2026-10-30", "2026-11-02"))
	expect(t, pending, 200, "")
	pendingID := pending.data()["id"].(string)
	att := func(date, in string, out any) string {
		return h.scalar(`INSERT INTO hris.attendance (employee_id, date, clock_in, status, clock_in_geofence, clock_out_geofence)
			VALUES ($1, $2::date, $2::date + time '08:00', 'present', $3, $4) RETURNING id::text`, own, date, in, out)
	}
	closedRow := att("2026-10-30", "invalid", nil)
	validRow := att("2026-11-01", "valid", "valid")
	openRow := att("2026-11-02", "invalid", "unchecked")
	outside := att("2026-11-03", "invalid", nil)
	h.exec(`DELETE FROM hris.attendance_periods WHERE start_date <= '2026-11-30' AND end_date >= '2026-10-01'`)
	h.exec(`INSERT INTO hris.attendance_periods (period_year, period_month, start_date, end_date, closing_date,
		status, closed_at, close_mode) VALUES (2026, 10, '2026-10-01', '2026-10-31', '2026-11-03', 'closed', now(), 'manual')`)

	approved := decide(&hr, pendingID, "approve", "")
	expect(t, approved, 200, "")
	if approved.data()["status"] != "approved" {
		t.Fatalf("HR approve = %s", approved.raw)
	}
	geo := func(id string) string {
		return h.scalar(`SELECT coalesce(clock_in_geofence, '-') || '/' || coalesce(clock_out_geofence, '-')
			FROM hris.attendance WHERE id = $1`, id)
	}
	for id, want := range map[string]string{closedRow: "invalid/-", validRow: "valid/valid", openRow: "dinas/dinas", outside: "invalid/-"} {
		if got := geo(id); got != want {
			t.Fatalf("attendance %s = %q, want %q", id, got, want)
		}
	}
}

// P5: the Perjalanan Dinas exemption is decided on a server-side date: the
// server's today for a self clock-in (never a body date) and the stored
// attendance date for a clock-out (an overnight shift stays dinas).
func TestBusinessTripDateIsServerSide(t *testing.T) {
	h, _ := newFileHarness(t)
	hr, _ := h.staff("hrd", nil, true)
	early, earlyID := h.staff("pos", nil, true, emp{name: "Crew Dinas Besok"})
	night, nightID := h.staff("pos", nil, true, emp{name: "Crew Shift Malam"})
	store := h.branch("Toko Malam")
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date) VALUES
		($1, $3, true, '2026-01-01'), ($2, $3, true, '2026-01-01')`, earlyID, nightID, store)
	photo := dataURL("image/png", pngBytes(t))
	assign := func(employeeID, date string) {
		expect(t, h.do(&hr, "POST", "/api/hris/business-trips", map[string]any{"employee_id": employeeID,
			"start_date": date, "end_date": date, "destination": "Kantor pusat", "purpose": "Rapat bulanan"}), 200, "")
	}

	// Dinas besok; absen hari ini dengan tanggal dinas di body tetap dinilai hari ini.
	assign(earlyID, "2026-10-06")
	body := clockInBody(photo, farFromStore, "")
	body["date"] = "2026-10-06"
	refused := h.do(&early, "POST", "/api/hris/attendance", body)
	expect(t, refused, 422, "")
	if det, _ := refused.body["details"].(map[string]any); det["code"] != "geofence_note_required" {
		t.Fatalf("body trip date = %s", refused.raw)
	}

	// Shift malam pada hari dinas: clock-out lewat tengah malam tetap dinas.
	assign(nightID, "2026-10-05")
	in := h.do(&night, "POST", "/api/hris/attendance", clockInBody(photo, farFromStore, ""))
	expect(t, in, 200, "")
	if in.data()["clock_in_geofence"] != "dinas" {
		t.Fatalf("clock-in = %s", in.raw)
	}
	h.svc.now = func() time.Time { return fixedNow.Add(16 * time.Hour) } // 2026-10-06 02:00 WIB
	co := h.do(&night, "POST", "/api/hris/attendance", map[string]any{"action": "clock-out",
		"attendance_id": in.data()["id"], "photo": photo, "clock_out_location": farFromStore})
	expect(t, co, 200, "")
	if co.data()["clock_out_geofence"] != "dinas" || co.data()["clock_out_distance_m"] != 334.0 {
		t.Fatalf("overnight clock-out = %s", co.raw)
	}
}

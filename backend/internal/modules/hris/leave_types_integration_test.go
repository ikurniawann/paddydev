package hris

import (
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

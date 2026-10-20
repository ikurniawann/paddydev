package recruitment

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/testutil"
)

// GET /api/portal/options and POST /api/notifications/send (ported from
// app/api/notifications/send/route.test.ts).

type fakeSender struct {
	waOK   bool
	mailOK bool
	sent   []string
}

func (f *fakeSender) WhatsApp(_ context.Context, _ database.Querier, target, message string) (bool, string) {
	f.sent = append(f.sent, "wa|"+target+"|"+message)
	if !f.waOK {
		return false, "gateway-belum-dikonfigurasi"
	}
	return true, ""
}

func (f *fakeSender) Email(_ context.Context, to, subject, _ string) bool {
	f.sent = append(f.sent, "email|"+to+"|"+subject)
	return f.mailOK
}

func TestPortalOptions(t *testing.T) {
	h := newHarness(t)
	brand := h.scalar(`INSERT INTO item.brands (name, is_active) VALUES ('Go Outlet', true) RETURNING id::text`)
	position := h.scalar(`INSERT INTO hris.positions (title, is_active, brand_id) VALUES ('Go Barista', true, $1) RETURNING id::text`, brand)
	opening := h.scalar(`INSERT INTO recruitment.job_openings (title, slug, brand_id, position_id) VALUES ('Barista Go', 'barista-go-' || md5(random()::text), $1, $2) RETURNING id::text`, brand, position)

	c := h.anon("GET", "/api/portal/options?opening="+opening, nil)
	expect(t, c, 200, "")
	if _, has := c.body["success"]; has {
		t.Fatal("the options answer carries no success key")
	}
	data := c.data()
	if !strings.Contains(c.raw, `{"id":"`+brand+`","name":"Go Outlet"}`) || !strings.Contains(c.raw, `{"id":"`+position+`","title":"Go Barista","brand_id":"`+brand+`"}`) {
		t.Fatalf("options = %s", c.raw)
	}
	eqJSON(t, data["opening"].(map[string]any), map[string]any{"brand_id": brand, "position_id": position})
	if c = h.anon("GET", "/api/portal/options?opening=bukan-uuid", nil); c.data()["opening"] != nil {
		t.Fatalf("opening = %v", c.data()["opening"])
	}
}

func eqJSON(t *testing.T, got, want map[string]any) {
	t.Helper()
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s = %v, want %v", k, got[k], v)
		}
	}
}

func TestNotificationsSend(t *testing.T) {
	// created before the harness tx so its cleanup runs after the rollback
	noRecruitment := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{"hris.employees": nil}})
	h := newHarness(t)
	sender := &fakeSender{waOK: true}
	n := &notifier{svc: h.svc, users: testutil.Deps(t, nil).Auth, sender: sender}
	h.svc.now = func() time.Time { return time.Date(2026, 10, 4, 3, 0, 0, 0, time.UTC) }
	mux := http.NewServeMux()
	mux.Handle("POST /api/notifications/send", httpx.Handle(n.send))
	h.mux = mux
	cand := h.candidate("screening")
	h.exec(`UPDATE recruitment.candidates SET full_name = 'Sari', phone = '+62 812-3456' WHERE id = $1`, cand)
	logRow := func(channel string) string {
		return h.scalar(`SELECT channel || '|' || message || '|' || status || '|' || (sent_at IS NOT NULL) FROM notifications_log WHERE candidate_id = $1 AND channel = $2`, cand, channel)
	}

	expect(t, h.anon("POST", "/api/notifications/send", map[string]any{"candidate_id": cand}), 401, "Authentication required")
	expect(t, h.as(h.other, "POST", "/api/notifications/send", map[string]any{"candidate_id": cand}), 403, "Insufficient permissions")
	// EPIC-057: an HR account without the Rekrutmen menu (Non-Rekrutmen & KPI)
	// may not message candidates even when it knows the id.
	expect(t, h.as(noRecruitment, "POST", "/api/notifications/send", map[string]any{"candidate_id": cand}), 403, "Insufficient permissions")
	if len(sender.sent) != 0 {
		t.Fatalf("sent without the Rekrutmen menu: %v", sender.sent)
	}
	expect(t, h.as(h.hr, "POST", "/api/notifications/send", map[string]any{"candidate_id": "7f1c2d3e-4b5a-4c6d-8e9f-0a1b2c3d4e5f"}), 404, "Kandidat tidak ditemukan")
	expect(t, h.as(h.hr, "POST", "/api/notifications/send", map[string]any{"candidate_id": cand, "channel": "sms"}), 400, "Validation failed")

	c := h.as(h.hr, "POST", "/api/notifications/send", map[string]any{"candidate_id": cand})
	expect(t, c, 200, "")
	if c.raw != `{"success":true,"message":"Notifikasi terkirim"}` {
		t.Fatal(c.raw)
	}
	if sender.sent[0] != "wa|628123456|Halo Sari, Status lamaran kamu saat ini: screening" {
		t.Fatalf("sent = %v", sender.sent)
	}
	if got := logRow("whatsapp"); got != "whatsapp|Halo Sari, Status lamaran kamu saat ini: screening|sent|true" {
		t.Fatalf("log = %s", got)
	}

	expect(t, h.as(h.hr, "POST", "/api/notifications/send", map[string]any{"candidate_id": cand, "channel": "email"}), 500, "Gagal mengirim email")
	if got := logRow("email"); got != "email|Notifikasi|failed|false" {
		t.Fatalf("log = %s", got)
	}

	c = h.as(h.hr, "POST", "/api/notifications/send", map[string]any{"candidate_id": cand, "template": "interview_scheduled",
		"interview_date": "2026-10-10T02:30:00Z", "mode": "online", "meeting_link": "https://meet.example/x", "interviewer_name": "Rina"})
	expect(t, c, 200, "")
	want := "Halo Sari, jadwal interview telah ditentukan:\n\n📅 *Interview*\n🗓️ Tanggal: Sabtu, 10 Oktober 2026, 09.30 WIB\n" +
		"📍 Mode: Online (Zoom/Google Meet)\n🔗 Link: https://meet.example/x\n👤 Interviewer: Rina\n\nMohon konfirmasi kehadiran. Terima kasih!"
	if last := sender.sent[len(sender.sent)-1]; last != "wa|628123456|"+want {
		t.Fatalf("interview message = %q", last)
	}
}

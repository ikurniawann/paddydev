package settings_test

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"nuhabit/backend/internal/modules/configuration/settings/domain"
	"nuhabit/backend/internal/platform/testutil"
	"nuhabit/backend/internal/platform/whatsapp"
)

// recorder is a fake provider that remembers each request.
type recorder struct {
	mu    sync.Mutex
	calls []call
}

type call struct {
	path, body string
	header     http.Header
}

func (rc *recorder) server(t *testing.T, reply func(path, body string) (int, string)) *httptest.Server {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		rc.mu.Lock()
		rc.calls = append(rc.calls, call{r.URL.EscapedPath(), string(raw), r.Header.Clone()})
		rc.mu.Unlock()
		status, body := reply(r.URL.Path, string(raw))
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(s.Close)
	return s
}

func (rc *recorder) last() call {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return rc.calls[len(rc.calls)-1]
}

func TestTts(t *testing.T) {
	admin := staff(t, "settings.integrations")
	e := newEnv(t)
	var rec recorder
	srv := rec.server(t, func(path, body string) (int, string) {
		if strings.Contains(body, "fail") || strings.Contains(path, "fail") {
			return 401, `{"error":"bad key"}`
		}
		return 200, "MP3"
	})
	e.ports.Endpoints.ElevenLabs = srv.URL
	e.ports.Endpoints.Azure = func(region string) string { return srv.URL + "/" + region }

	catalog, _ := json.Marshal(domain.TtsProviders)
	expect(t, e.do(&admin, "GET", "/api/settings/tts", nil), 200,
		`{"data":{"provider":"openai","voice":"coral","model":"gpt-4o-mini-tts","catalog":`+string(catalog)+
			`,"credentials":{"openai":{"configured":false},"azure":{"configured":false,"key_masked":null,"region":""},"elevenlabs":{"configured":false,"key_masked":null}}}}`)

	expect(t, e.do(&admin, "PUT", "/api/settings/tts", map[string]any{"voice": 5, "model": nil, "azure_key": 1}), 400,
		`{"success":false,"error":"Validation failed","details":[{"code":"invalid_type","path":["voice"],"message":"Voice tidak valid"},{"code":"invalid_type","path":["model"],"message":"Model tidak valid"},{"code":"invalid_type","path":["azure_key"],"message":"Invalid input: expected string, received number"}]}`)
	expect(t, e.do(&admin, "PUT", "/api/settings/tts", map[string]any{"provider": "aws"}), 400, `{"success":false,"error":"Provider tidak dikenal"}`)
	expect(t, e.do(&admin, "PUT", "/api/settings/tts", map[string]any{"provider": "azure", "voice": " id-ID-ArdiNeural ",
		"model": "tts-1", "azure_key": " abcdefghijkl ", "azure_region": "sea", "elevenlabs_key": ""}), 200, `{"success":true}`)
	expect(t, e.do(&admin, "GET", "/api/settings/tts", nil), 200,
		`{"data":{"provider":"azure","voice":"id-ID-ArdiNeural","model":"neural","catalog":`+string(catalog)+
			`,"credentials":{"openai":{"configured":false},"azure":{"configured":true,"key_masked":"abcd••••••••ijkl","region":"sea"},"elevenlabs":{"configured":false,"key_masked":null}}}}`)
	e.do(&admin, "PUT", "/api/settings/tts", map[string]any{"provider": "elevenlabs", "voice": "   "})
	if got := e.text(`SELECT coalesce(value, 'NULL') FROM configuration.app_settings WHERE key = 'tts_voice'`); got != "NULL" {
		t.Fatalf("empty ElevenLabs voice stores NULL, got %s", got)
	}

	// Preview: stored provider (ElevenLabs) without a key.
	expect(t, e.do(&admin, "POST", "/api/settings/tts/preview", nil), 400,
		`{"success":false,"error":"ElevenLabs API key belum diisi. Atur di Settings → Suara AI."}`)
	expect(t, e.do(&admin, "POST", "/api/settings/tts/preview", map[string]any{"provider": 1}), 400,
		`{"success":false,"error":"Provider tidak dikenal"}`)
	expect(t, e.do(&admin, "POST", "/api/settings/tts/preview", map[string]any{"provider": "openai"}), 400,
		`{"success":false,"error":"API key OpenAI belum diisi. Atur di Settings → Integrasi."}`)

	// Azure with an override voice; the text is trimmed and capped at 300.
	long := strings.Repeat("a", 310) + "<"
	r := e.do(&admin, "POST", "/api/settings/tts/preview", map[string]any{"provider": "azure", "voice": "en-US-X'", "text": "  " + long})
	expect(t, r, 200, `{"data":{"audio_base64":"`+base64.StdEncoding.EncodeToString([]byte("MP3"))+
		`","provider":"azure","voice":"en-US-X'","model":"neural","text":"`+strings.Repeat("a", 300)+`"}}`)
	got := rec.last()
	if got.path != "/sea/cognitiveservices/v1" || got.header.Get("Ocp-Apim-Subscription-Key") != "abcdefghijkl" ||
		got.body != `<speak version="1.0" xml:lang="en-US"><voice name="en-US-X&apos;">`+strings.Repeat("a", 300)+`</voice></speak>` {
		t.Fatalf("azure call %+v", got)
	}

	// OpenAI through openai_base_url, with the accent instructions.
	e.setting("openai_api_key", "sk-test")
	e.setting("openai_base_url", srv.URL+"/v1")
	r = e.do(&admin, "POST", "/api/settings/tts/preview", map[string]any{"provider": "openai", "voice": "nova", "text": "Halo"})
	expect(t, r, 200, `{"data":{"audio_base64":"TVAz","provider":"openai","voice":"nova","model":"gpt-4o-mini-tts","text":"Halo"}}`)
	got = rec.last()
	if got.path != "/v1/audio/speech" || got.header.Get("Authorization") != "Bearer sk-test" ||
		!strings.HasPrefix(got.body, `{"model":"gpt-4o-mini-tts","voice":"nova","input":"Halo","response_format":"mp3","instructions":"Bicaralah`) {
		t.Fatalf("openai call %+v", got)
	}
	expect(t, e.do(&admin, "POST", "/api/settings/tts/preview", map[string]any{"provider": "openai", "model": "tts-1", "text": "fail"}), 502,
		`{"success":false,"error":"OpenAI TTS 401: {\"error\":\"bad key\"}"}`)
	if strings.Contains(rec.last().body, "instructions") {
		t.Fatal("tts-1 gets no instructions")
	}

	// ElevenLabs: the stored voice (none) is the default; a custom voice is
	// path-escaped like encodeURIComponent.
	e.setting("elevenlabs_api_key", "xi")
	r = e.do(&admin, "POST", "/api/settings/tts/preview", map[string]any{"voice": "my voice/1"})
	expect(t, r, 200, "")
	if got := rec.last(); got.path != "/v1/text-to-speech/my%20voice%2F1" || got.header.Get("xi-api-key") != "xi" ||
		!strings.HasPrefix(got.body, `{"text":"Perkenalkan diri Anda`) || !strings.HasSuffix(got.body, `","model_id":"eleven_multilingual_v2"}`) {
		t.Fatalf("elevenlabs call %+v", got)
	}
}

func TestOrderAlerts(t *testing.T) {
	admin := staff(t, "settings.integrations")
	supervisor := testutil.CreateStaff(t, testutil.StaffOptions{Role: "pos_supervisor", FullName: "Go Supervisor"})
	e := newEnv(t)
	e.exec(`DELETE FROM configuration.telegram_subscribers`)
	e.exec(`INSERT INTO hris.employees (full_name, nip, email, phone, join_date, user_id) VALUES ('Go Supervisor', $1, $2, '0812-3456-7890', '2026-01-01', $3)`,
		"GO"+testutil.RandomHex(4), supervisor.Email, supervisor.UserID)
	e.exec(`INSERT INTO configuration.telegram_subscribers (chat_id, chat_type, title, username, status, subscribed_at) VALUES
		(111, 'private', NULL, 'budi', 'pending', '2026-10-01T00:00:00Z'),
		(222, 'group', 'Bar', NULL, 'active', '2026-10-02T00:00:00Z'),
		(403, 'private', NULL, NULL, 'active', '2026-10-03T00:00:00Z'),
		(999, 'private', NULL, NULL, 'stopped', '2026-10-03T00:00:00Z')`)

	var tg, wa recorder
	tgSrv := tg.server(t, func(path, body string) (int, string) {
		switch {
		case strings.HasSuffix(path, "/getMe") && strings.Contains(path, "/bot12345:"):
			return 200, `{"ok":true,"result":{"id":1,"username":"nuhabit_bot"}}`
		case strings.HasSuffix(path, "/getMe"):
			return 401, `{"ok":false,"error_code":401,"description":"Unauthorized"}`
		case strings.Contains(body, `"chat_id":"403"`):
			return 403, `{"ok":false,"description":"Forbidden: bot was blocked by the user"}`
		}
		return 200, `{"ok":true,"result":true}`
	})
	waSrv := wa.server(t, func(string, string) (int, string) { return 200, `{"success":true,"messageId":"m1"}` })
	e.ports.Endpoints.Telegram = tgSrv.URL

	chats := `[{"chat_id":"111","title":null,"username":"budi","chat_type":"private","status":"pending","subscribed_at":"2026-10-01T00:00:00.000Z"},` +
		`{"chat_id":"222","title":"Bar","username":null,"chat_type":"group","status":"active","subscribed_at":"2026-10-02T00:00:00.000Z"},` +
		`{"chat_id":"403","title":null,"username":null,"chat_type":"private","status":"active","subscribed_at":"2026-10-03T00:00:00.000Z"}]`
	roles := `[{"code":"pos","label":"POS (kasir/bar)"},{"code":"pos_supervisor","label":"POS Supervisor"}]`
	recipient := `{"name":"Go Supervisor","role":"pos_supervisor","phone_masked":"6281••••890","has_phone":true}`
	r := e.do(&admin, "GET", "/api/settings/order-alerts", nil)
	expect(t, r, 200, `{"success":true,"data":{"config":{"waEnabled":true,"waRoles":["pos"],"telegramEnabled":true},"role_options":`+roles+
		`,"wa_gateway_configured":false,"wa_recipients":[`+recipient+`],"telegram":{"connected":false,"token_masked":null,"username":null},"telegram_chats":`+chats+`}}`)

	put := func(body any) resp { return e.do(&admin, "PUT", "/api/settings/order-alerts", body) }
	invalid := `{"success":false,"error":"Data tidak valid"}`
	expect(t, put("{bad"), 400, invalid)
	expect(t, put(map[string]any{"wa_enabled": true, "wa_roles": []string{"admin"}, "telegram_enabled": true}), 400, invalid)
	expect(t, put(map[string]any{"wa_enabled": true, "wa_roles": []string{}, "telegram_enabled": true, "telegram_bot_token": strings.Repeat("x", 101)}), 400, invalid)
	expect(t, put(map[string]any{"wa_enabled": true, "wa_roles": []string{}, "telegram_enabled": true, "telegram_bot_token": "bukan-token"}), 400,
		`{"success":false,"error":"Format token bot Telegram tidak valid"}`)
	bad := "99999:" + strings.Repeat("b", 35)
	expect(t, put(map[string]any{"wa_enabled": true, "wa_roles": []string{}, "telegram_enabled": true, "telegram_bot_token": bad}), 502,
		`{"success":false,"error":"Telegram: Unauthorized"}`)

	token := "12345:" + strings.Repeat("a", 35)
	req := map[string]any{"wa_enabled": true, "wa_roles": []string{"pos_supervisor"}, "telegram_enabled": true, "telegram_bot_token": "  " + token + " "}
	r = put(req)
	expect(t, r, 200, "")
	if !strings.Contains(r.Raw, `"config":{"waEnabled":true,"waRoles":["pos_supervisor"],"telegramEnabled":true}`) ||
		!strings.Contains(r.Raw, `"telegram":{"connected":true,"token_masked":"1234••••••••aaaa","username":"nuhabit_bot"}`) {
		t.Fatalf("PUT %s", r.Raw)
	}
	secret := e.text(`SELECT value FROM configuration.app_settings WHERE key = 'telegram_webhook_secret'`)
	hook := tg.last()
	if len(secret) != 48 || hook.path != "/bot"+token+"/setWebhook" || hook.body != `{"url":"http://example.com/api/integrations/telegram/webhook/`+secret+`","secret_token":"`+
		secret+`","allowed_updates":["message","my_chat_member"],"drop_pending_updates":true}` {
		t.Fatalf("setWebhook %+v (secret %s)", hook, secret)
	}

	post := func(body any) resp { return e.do(&admin, "POST", "/api/settings/order-alerts", body) }
	expect(t, post(map[string]any{"action": "approve", "chat_id": "12a"}), 400, `{"success":false,"error":"Aksi tidak valid"}`)
	expect(t, post(map[string]any{"action": "nope"}), 400, `{"success":false,"error":"Aksi tidak valid"}`)
	expect(t, post(map[string]any{"action": "approve", "chat_id": "555"}), 404, `{"success":false,"error":"Chat tidak ditemukan"}`)
	r = post(map[string]any{"action": "approve", "chat_id": "111"})
	expect(t, r, 200, "")
	if got := tg.last(); got.path != "/bot"+token+"/sendMessage" ||
		got.body != `{"chat_id":"111","text":"✅ Disetujui. Chat ini sekarang menerima notifikasi pesanan masuk Paddy. Ketik /stop untuk berhenti.","disable_web_page_preview":true}` {
		t.Fatalf("approve message %+v", got)
	}
	expect(t, post(map[string]any{"action": "remove", "chat_id": "222"}), 200, "")

	// test: WhatsApp through the gateway, Telegram to active chats; the
	// chat that blocked the bot is stopped.
	e.setting("wa_gateway_url", waSrv.URL)
	e.setting("wa_gateway_token", "gw")
	fresh := whatsapp.New(e.deps.Log) // the first client cached "no gateway" for 30 seconds
	fresh.Getenv = func(string) string { return "" }
	e.ports.WhatsApp = fresh
	e.exec(`UPDATE configuration.telegram_subscribers SET status = 'active' WHERE chat_id = 222`)
	// The unofficial gateway only messages numbers that chatted before
	// (whatsapp.KnownContact); a migrations-only DB (CI) has no such row.
	e.exec(`INSERT INTO crm.wa_messages (direction, phone, body) VALUES ('in', '6281234567890', 'halo')`)
	expect(t, post(map[string]any{"action": "test", "chat_id": 1}), 200,
		`{"success":true,"data":{"result":{"wa":{"sent":1,"failed":0,"skippedNoPhone":0},"telegram":{"sent":2,"failed":1}}}}`)
	if got := wa.last(); got.header.Get("x-gateway-token") != "gw" || !strings.Contains(got.body, `"target":"6281234567890"`) ||
		!strings.Contains(got.body, "Tes notifikasi pesanan masuk Paddy.") {
		t.Fatalf("wa call %+v", got)
	}
	if got := e.text(`SELECT status FROM configuration.telegram_subscribers WHERE chat_id = 403`); got != "stopped" {
		t.Fatalf("blocked chat status %s", got)
	}

	e.exec(`DELETE FROM configuration.app_settings WHERE key = 'telegram_bot_token'`)
	expect(t, post(map[string]any{"action": "reconnect"}), 400, `{"success":false,"error":"Token bot Telegram belum diisi"}`)
}

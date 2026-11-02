package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"nuhabit/backend/internal/modules/configuration"
	"nuhabit/backend/internal/platform/testutil"
)

func TestRecruitmentHiredAdapter(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	var candidateID string
	if err := tx.QueryRow(ctx, `INSERT INTO recruitment.candidates (full_name, email, phone, domicile, source, status)
		VALUES ('Go Hired', 'go' || md5(random()::text) || '@x.id', '0812', 'Jkt', 'walk_in', 'hired') RETURNING id::text`).Scan(&candidateID); err != nil {
		t.Fatal(err)
	}
	if row, err := (recruitmentHired{}).Hired(ctx, tx, candidateID); err != nil || row != nil {
		t.Fatalf("not promoted yet: %v %v", row, err)
	}
	if _, err := tx.Exec(ctx, `SELECT public.promote_candidate_to_employee($1, '2026-08-01', 'probation', NULL, NULL, $2)`, candidateID, "202611"+candidateID[:6]); err != nil {
		t.Fatal(err)
	}
	row, err := recruitmentHired{}.Hired(ctx, tx, candidateID)
	if err != nil || row == nil {
		t.Fatalf("hired: %v", err)
	}
	if row.Str("employment_status") != "probation" || row.Str("nip") == "" || row.Get("has_account") != false || row.Int("onboarding_total") < 0 {
		t.Fatalf("%v %v %v", row.Get("employment_status"), row.Get("nip"), row.Get("has_account"))
	}
}

func TestRecruitmentSpeechAdapter(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	var got map[string]any
	var path, auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("Authorization")
		if r.Header.Get("Ocp-Apim-Subscription-Key") != "" {
			auth = r.Header.Get("Ocp-Apim-Subscription-Key")
			body, _ := io.ReadAll(r.Body)
			got = map[string]any{"ssml": string(body)}
		} else {
			_ = json.NewDecoder(r.Body).Decode(&got)
		}
		_, _ = w.Write([]byte("ID3-mp3"))
	}))
	defer srv.Close()
	set := func(k, v string) {
		if _, err := tx.Exec(ctx, `INSERT INTO configuration.app_settings (key, value) VALUES ($1, $2)
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, k, v); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM configuration.app_settings WHERE key LIKE 'tts\_%' OR key LIKE 'openai\_%' OR key LIKE 'azure\_%'`); err != nil {
		t.Fatal(err)
	}
	speech := recruitmentSpeech{log: slog.New(slog.DiscardHandler),
		urls: configuration.TtsEndpoints{ElevenLabs: srv.URL, Azure: func(string) string { return srv.URL }}}

	if audio := speech.Synthesize(ctx, tx, "Halo"); audio != nil {
		t.Fatal("no key: nil, the interview goes on as text")
	}
	set("openai_api_key", "sk-test")
	set("openai_base_url", srv.URL)
	if audio := speech.Synthesize(ctx, tx, "Halo"); string(audio) != "ID3-mp3" {
		t.Fatalf("%q", audio)
	}
	if path != "/audio/speech" || auth != "Bearer sk-test" || got["model"] != "gpt-4o-mini-tts" || got["voice"] != "coral" ||
		got["response_format"] != "mp3" || got["instructions"] == nil {
		t.Fatalf("%s %s %v", path, auth, got)
	}

	set("tts_provider", "azure")
	set("azure_speech_key", "az")
	set("azure_speech_region", "southeastasia")
	speech.Synthesize(ctx, tx, `Gaji <b>"anda"</b>?`)
	if path != "/cognitiveservices/v1" || auth != "az" ||
		got["ssml"] != `<speak version="1.0" xml:lang="id-ID"><voice name="id-ID-GadisNeural">Gaji &lt;b&gt;&quot;anda&quot;&lt;/b&gt;?</voice></speak>` {
		t.Fatalf("%s %v", path, got)
	}

	set("tts_provider", "elevenlabs")
	set("elevenlabs_api_key", "xi")
	set("tts_voice", "voice id")
	if audio := speech.Synthesize(ctx, tx, "Halo"); string(audio) != "ID3-mp3" {
		t.Fatalf("%q", audio)
	}
	if path != "/v1/text-to-speech/voice id" || got["text"] != "Halo" || got["model_id"] == nil {
		t.Fatalf("%s %v", path, got)
	}
}

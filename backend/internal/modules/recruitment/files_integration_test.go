package recruitment

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/extract"
	"nuhabit/backend/internal/platform/pdfgen"
	"nuhabit/backend/internal/platform/testutil"
)

// ── fakes ───────────────────────────────────────────────────────────────────

type fakeSettings map[string]string

func (s fakeSettings) GetMany(_ context.Context, _ database.Querier, keys []string) (map[string]*string, error) {
	out := map[string]*string{}
	for _, k := range keys {
		if v, ok := s[k]; ok {
			out[k] = &v
		}
	}
	return out, nil
}

// mp3Bytes sniff as audio/mpeg.
var mp3Bytes = append([]byte("ID3"), bytes.Repeat([]byte{1}, 29)...)

type fakeSpeech struct{ texts []string }

func (s *fakeSpeech) Synthesize(_ context.Context, _ database.Querier, text string) []byte {
	s.texts = append(s.texts, text)
	return mp3Bytes
}

type fakeHired struct{ row *Row }

func (f fakeHired) Hired(context.Context, database.Querier, string) (*Row, error) { return f.row, nil }

// fakeAI is an OpenAI-compatible server: chat completions answer the
// queued contents in order, transcriptions answer transcript.
type fakeAI struct {
	*httptest.Server
	mu         sync.Mutex
	replies    []string
	chats      []map[string]any
	auth       []string
	transcript string
	whisper    []string // multipart field names seen
}

func newFakeAI(t *testing.T) *fakeAI {
	f := &fakeAI{transcript: " halo dunia "}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		if strings.HasSuffix(r.URL.Path, "/audio/transcriptions") {
			_ = r.ParseMultipartForm(1 << 20)
			for k := range r.MultipartForm.Value {
				f.whisper = append(f.whisper, k+"="+r.MultipartForm.Value[k][0])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"text": f.transcript})
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.chats = append(f.chats, body)
		if len(f.replies) == 0 {
			http.Error(w, "no reply queued", http.StatusInternalServerError)
			return
		}
		content := f.replies[0]
		f.replies = f.replies[1:]
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}}}})
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeAI) queue(contents ...string) {
	f.mu.Lock()
	f.replies = append(f.replies, contents...)
	f.mu.Unlock()
}

// userPrompt is the last chat request's user message as text.
func (f *fakeAI) userPrompt(t *testing.T) string {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.chats) == 0 {
		t.Fatal("no chat request")
	}
	msgs := f.chats[len(f.chats)-1]["messages"].([]any)
	raw, _ := json.Marshal(msgs[len(msgs)-1].(map[string]any)["content"])
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}

type fileHarness struct {
	*harness
	dir      string
	settings fakeSettings
	speech   *fakeSpeech
	hired    *fakeHired
	ai       *fakeAI
	files    *fileHandler
}

func fileTestPorts(h *harness, s fakeSettings, sp *fakeSpeech, hired *fakeHired) Ports {
	return Ports{Employees: h.ports, Contracts: h.ports, Settings: s, Speech: sp, Hired: hired}
}

// newFileHarness runs the module on the test transaction with storage in
// a temporary directory and the AI providers on a fake server.
func newFileHarness(t *testing.T) *fileHarness {
	dir := t.TempDir()
	t.Setenv("STORAGE_DIR", dir)
	h := newHarness(t)
	ai := newFakeAI(t)
	fh := &fileHarness{harness: h, dir: dir, ai: ai, speech: &fakeSpeech{}, hired: &fakeHired{},
		settings: fakeSettings{"deepseek_api_key": "ds-key", "deepseek_base_url": ai.URL, "openai_api_key": "oa-key", "openai_base_url": ai.URL + "/"}}
	m := newModule(testutil.Deps(t, nil), h.tx, fileTestPorts(h, fh.settings, fh.speech, fh.hired))
	h.svc, h.mux, fh.files = m.h.svc, testutil.Mux(m), m.f
	fh.files.ai.retryDelay = 0
	return fh
}

type part struct {
	field, name, ctype string
	data               []byte
}

func multipartRequest(method, target string, fields map[string]string, files ...part) *http.Request {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	for _, p := range files {
		hdr := textproto.MIMEHeader{}
		hdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, p.field, p.name))
		hdr.Set("Content-Type", p.ctype)
		w, _ := mw.CreatePart(hdr)
		_, _ = w.Write(p.data)
	}
	_ = mw.Close()
	r := httptest.NewRequest(method, target, &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	return r
}

func (h *fileHarness) path(rel string) string { return filepath.Join(h.dir, filepath.FromSlash(rel)) }

func (h *fileHarness) writePrivate(rel string, data []byte) {
	h.t.Helper()
	p := h.path("private/" + rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		h.t.Fatal(err)
	}
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func cvPDF(t *testing.T) []byte {
	t.Helper()
	d := pdfgen.New(pdfgen.Options{})
	d.Para("Sari Ayu, barista berpengalaman lima tahun di kedai kopi Bandung. Lulusan D3 Perhotelan.", pdfgen.TextOpts{})
	b, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

var pngBytes = func() []byte {
	b, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	return b
}()

// ── tests ───────────────────────────────────────────────────────────────────

func TestPrivateFiles(t *testing.T) {
	h := newFileHarness(t)
	h.writePrivate("psikotes/s1/t1/a.png", pngBytes)
	h.writePrivate("interview/s1/recording/part-1.webm", []byte("webm-bytes"))
	h.writePrivate("interview/s1/questions/q.mp3", mp3Bytes)
	h.writePrivate("secret.txt", []byte("no"))

	expect(t, h.as(h.none, "GET", "/api/psikotes/files/psikotes/s1/t1/a.png", nil), 403, "")
	c := h.as(h.hr, "GET", "/api/psikotes/files/psikotes/s1/t1/a.png", nil)
	if c.status != 200 || c.raw != string(pngBytes) || c.header.Get("Content-Type") != "image/png" ||
		c.header.Get("Cache-Control") != "private, max-age=300" || c.header.Get("X-Content-Type-Options") != "nosniff" ||
		c.header.Get("Content-Length") != fmt.Sprint(len(pngBytes)) {
		t.Fatalf("%d %v", c.status, c.header)
	}
	c = h.as(h.hr, "GET", "/api/interview/files/interview/s1/recording/part-1.webm", nil)
	if c.status != 200 || c.header.Get("Content-Type") != "video/webm" || c.raw != "webm-bytes" {
		t.Fatalf("recordings are video: %d %v", c.status, c.header)
	}
	if c = h.as(h.hr, "GET", "/api/interview/files/interview/s1/questions/q.mp3", nil); c.header.Get("Content-Type") != "audio/mpeg" {
		t.Fatal(c.header)
	}
	for _, p := range []string{
		"/api/psikotes/files/psikotes/%2e%2e/secret.txt",
		"/api/psikotes/files/psikotes/%252e%252e/secret.txt",
		"/api/psikotes/files/psikotes/s1%2F..%2F..%2Fsecret.txt",
		"/api/psikotes/files/interview/s1/questions/q.mp3", // the other folder
		"/api/psikotes/files/psikotes",
		"/api/psikotes/files/psikotes/s1/t1/missing.png",
		"/api/interview/files/interview/%2e/x",
	} {
		expect(t, h.as(h.hr, "GET", p, nil), 404, "File tidak ditemukan")
	}
}

func TestCVUploadAndRemove(t *testing.T) {
	h := newFileHarness(t)
	id := h.candidate("applied")
	pdf := cvPDF(t)
	up := func(p part) call {
		return h.do(testutil.AsStaff(multipartRequest("POST", "/api/candidates/"+id+"/cv-upload", nil, p), h.hr))
	}

	expect(t, h.do(testutil.AsStaff(multipartRequest("POST", "/api/candidates/"+id+"/cv-upload", nil, part{"file", "cv.pdf", "application/pdf", pdf}), h.none)), 403, "")
	expect(t, h.do(testutil.AsStaff(testutil.Request("POST", "/api/candidates/"+id+"/cv-upload", map[string]any{}), h.hr)), 400, "Invalid form data")
	expect(t, up(part{"other", "cv.pdf", "application/pdf", pdf}), 400, "File tidak ditemukan")
	expect(t, up(part{"file", "cv.exe", "application/pdf", pdf}), 400, "Tipe file tidak valid. Allowed: pdf, doc, docx, jpg, jpeg, png")
	expect(t, up(part{"file", "cv.pdf", "text/html", pdf}), 400, "Tipe file tidak didukung")
	expect(t, h.do(testutil.AsStaff(multipartRequest("POST", "/api/candidates/bad/cv-upload", nil, part{"file", "cv.pdf", "application/pdf", pdf}), h.hr)), 400, "ID kandidat tidak valid")

	c := up(part{"file", "CV Sari.pdf", "application/pdf", pdf})
	expect(t, c, 200, "")
	url, _ := c.data()["cv_url"].(string)
	if c.body["message"] != "CV berhasil diupload" || !strings.HasPrefix(url, "/api/files/candidates/cvs/") || !strings.HasSuffix(url, ".pdf") {
		t.Fatal(c.raw)
	}
	if h.scalar(`SELECT cv_url FROM recruitment.candidates WHERE id = $1`, id) != url {
		t.Fatal("cv_url stored")
	}
	stored, err := os.ReadFile(h.path("uploads" + strings.TrimPrefix(url, "/api/files")))
	if err != nil || !bytes.Equal(stored, pdf) {
		t.Fatalf("uploaded bytes: %v", err)
	}

	testutil.GrantMenu(t, h.hr.RoleID, "hris.recruitment.candidates", []string{"read", "delete"})
	c = h.as(h.hr, "DELETE", "/api/candidates/"+id+"/cv-upload?reason=CV%20salah%20unggah", nil)
	if c.status != 200 || c.raw != `{"message":"CV berhasil dihapus"}` || h.scalar(`SELECT cv_url FROM recruitment.candidates WHERE id = $1`, id) != "<nil>" {
		t.Fatal(c.raw)
	}
}

func TestCVAnalysis(t *testing.T) {
	h := newFileHarness(t)
	id := h.candidate("applied")
	expect(t, h.as(h.hr, "GET", "/api/candidates/x/ai-analysis", nil), 400, "ID kandidat tidak valid")
	if c := h.as(h.hr, "GET", "/api/candidates/"+id+"/ai-analysis", nil); c.raw != `{"data":null}` {
		t.Fatal(c.raw)
	}
	expect(t, h.as(h.hr, "POST", "/api/candidates/"+id+"/ai-analysis", nil), 400, "Kandidat belum memiliki lampiran CV")
	expect(t, h.as(h.hr, "POST", "/api/candidates/00000000-0000-4000-8000-000000000000/ai-analysis", nil), 404, "Kandidat tidak ditemukan")

	url, err := h.files.store.Upload("candidates", "cvs", cvPDF(t), "application/pdf", "cv.pdf")
	if err != nil {
		t.Fatal(err)
	}
	job := h.scalar(`INSERT INTO recruitment.job_openings (title, slug, description, requirements, status)
		VALUES ('Barista', 'barista-' || substr(md5(random()::text), 1, 6), 'Menyeduh kopi', 'Pengalaman 1 tahun', 'published') RETURNING id::text`)
	h.exec(`UPDATE recruitment.candidates SET cv_url = $2, job_opening_id = $3 WHERE id = $1`, id, url, job)

	h.ai.queue(`{"nama":"Sari Ayu","email":null,"pendidikan":"D3 Perhotelan","ringkasan":"Barista berpengalaman.","skor_kecocokan":"86.6","alasan_kecocokan":"Cocok."}`)
	c := h.as(h.hr, "POST", "/api/candidates/"+id+"/ai-analysis", nil)
	expect(t, c, 200, "")
	d := c.data()
	ex := d["extracted"].(map[string]any)
	if d["match_score"] != float64(87) || d["summary"] != "Barista berpengalaman." || d["model"] != "deepseek-chat" ||
		d["job_context"] != "Posisi: Barista\nDeskripsi: Menyeduh kopi\nPersyaratan: Pengalaman 1 tahun" ||
		ex["metode_ekstraksi"] != "pdf" || ex["nama"] != "Sari Ayu" || ex["email"] != nil || len(ex) != 4 || c.body["message"] != "Analisis CV selesai" {
		t.Fatal(c.raw)
	}
	prompt := h.ai.userPrompt(t)
	if !strings.Contains(prompt, "DESKRIPSI PEKERJAAN YANG DILAMAR:\nPosisi: Barista") || !strings.Contains(prompt, "barista berpengalaman lima tahun") {
		t.Fatal(prompt)
	}
	if h.ai.auth[len(h.ai.auth)-1] != "Bearer ds-key" {
		t.Fatal(h.ai.auth)
	}
	if c = h.as(h.hr, "GET", "/api/candidates/"+id+"/ai-analysis", nil); c.data()["match_score"] != float64(87) {
		t.Fatal(c.raw)
	}

	// re-run upserts; a provider failure is a 500 with the TS message
	expect(t, h.as(h.hr, "POST", "/api/candidates/"+id+"/ai-analysis", nil), 500, "Analisis CV gagal")
	delete(h.settings, "deepseek_api_key")
	expect(t, h.as(h.hr, "POST", "/api/candidates/"+id+"/ai-analysis", nil), 400, string(deepseekNotConfigured))
}

func TestCVExtract(t *testing.T) {
	h := newFileHarness(t)
	send := func(p part) call {
		return h.do(testutil.AsStaff(multipartRequest("POST", "/api/candidates/cv-extract", nil, p), h.hr))
	}
	expect(t, send(part{"file", "cv.txt", "text/plain", []byte("x")}), 400, "Tipe file tidak valid. Allowed: pdf, doc, docx, jpg, jpeg, png")

	h.ai.queue(`{"full_name":" Sari Ayu ","email":"sari@contoh.com","phone":"","domicile":5,"notes":"Barista."}`)
	c := send(part{"file", "cv.pdf", "application/pdf", cvPDF(t)})
	if c.raw != `{"data":{"full_name":"Sari Ayu","email":"sari@contoh.com","phone":null,"domicile":null,"last_experience":null,"last_education":null,"notes":"Barista."}}` {
		t.Fatal(c.raw)
	}
	h.ai.mu.Lock()
	body := h.ai.chats[len(h.ai.chats)-1]
	h.ai.mu.Unlock()
	content := body["messages"].([]any)[1].(map[string]any)["content"].([]any)
	file := content[1].(map[string]any)["file"].(map[string]any)
	if body["model"] != "gpt-4o-mini" || body["temperature"] != float64(0) || file["filename"] != "cv.pdf" ||
		!strings.HasPrefix(file["file_data"].(string), "data:application/pdf;base64,JVBERi0") {
		t.Fatalf("%v", body)
	}

	expect(t, send(part{"file", "cv.pdf", "application/pdf", cvPDF(t)}), 500, "OCR CV gagal")
	delete(h.settings, "openai_api_key")
	t.Setenv("OPENAI_API_KEY", "")
	expect(t, send(part{"file", "cv.pdf", "application/pdf", cvPDF(t)}), 400, string(openAIOCRNotConfigured))
}

func TestPipelineReport(t *testing.T) {
	h := newFileHarness(t)
	id := h.candidate("applied")
	expect(t, h.as(h.hr, "GET", "/api/candidates/nope/report", nil), 400, "ID kandidat tidak valid")
	expect(t, h.as(h.hr, "GET", "/api/candidates/00000000-0000-4000-8000-000000000000/report", nil), 404, "Kandidat tidak ditemukan")
	expect(t, h.as(h.hr, "GET", "/api/candidates/"+id+"/report", nil), 409, "Laporan pipeline tersedia mulai tahap Offer")

	h.exec(`UPDATE recruitment.candidates SET status = 'offer', expected_salary = 4500000 WHERE id = $1`, id)
	h.exec(`INSERT INTO recruitment.candidate_ai_analysis (candidate_id, summary, match_score, match_reason, model)
		VALUES ($1, 'Barista berpengalaman.', 87, 'Cocok → lanjut', 'deepseek-chat')`, id)
	h.exec(`INSERT INTO recruitment.candidate_activities (candidate_id, activity_type, description, created_by_name)
		VALUES ($1, 'note_added', 'Kandidat lolos screening', 'Rina HR')`, id)
	inst := h.instrument("drawing", `{}`)
	session := h.scalar(`INSERT INTO recruitment.psikotes_sessions (candidate_id, token, status) VALUES ($1, $2, 'completed') RETURNING id::text`, id, strings.Repeat("cd", 32))
	h.exec(`INSERT INTO recruitment.psikotes_session_tests (session_id, instrument_id, status, ai_insight) VALUES ($1, $2, 'reviewed', $3::jsonb)`,
		session, inst, `{"observation":"Pohon besar","observation_source":"ai","vision_model":"gpt-4o-mini","model":"deepseek-chat","created_at":"2026-07-15T09:30:00.000Z",
		"insight":{"ringkasan":"Stabil.","indikasi":[{"aspek":"Batang","insight":"Kokoh"}],"perhatikan_saat_interview":["Tanya target"],"keterbatasan":"Indikatif."}}`)

	c := h.as(h.hr, "GET", "/api/candidates/"+id+"/report", nil)
	if c.status != 200 || c.header.Get("Content-Type") != "application/pdf" || c.header.Get("Cache-Control") != "no-store" ||
		c.header.Get("Content-Disposition") != `attachment; filename="laporan-pipeline-sari-ayu.pdf"` ||
		c.header.Get("Content-Length") != fmt.Sprint(len(c.raw)) || !strings.HasPrefix(c.raw, "%PDF-") {
		t.Fatalf("%d %v", c.status, c.header)
	}
	text, err := extract.PDFText([]byte(c.raw))
	if err != nil {
		t.Fatal(err)
	}
	// sections in order, values formatted like the TS
	order := []string{"Laporan Pipeline Rekrutmen", "Sari Ayu — Posisi tidak tercatat", "Status saat ini: Offer",
		"1. Profil Kandidat", "Rp 4.500.000", "2. Analisis CV (AI)", "87/100", "Cocok -> lanjut",
		"3. Screening HR", "Tahap screening belum diisi.", "4. Psikotes", "Analisa AI", "Pohon besar (Otomatis (AI vision))",
		"Batang: Kokoh", "deepseek-chat + gpt-4o-mini · 15 Juli 2026 pukul 16.30", "5. Interview AI", "Belum ada sesi interview AI.",
		"6. Offer", "Belum ada offer yang dibuat.", "7. Hired", "Kandidat belum berstatus Hired.",
		"8. Riwayat Aktivitas Pipeline", "Rina HR", "Kandidat lolos screening", "Halaman 1 dari"}
	at := 0
	for _, want := range order {
		i := strings.Index(text[at:], want)
		if i < 0 {
			t.Fatalf("%q missing after offset %d in %q", want, at, text)
		}
		at += i
	}

	// hired section from the HRIS port
	h.exec(`UPDATE recruitment.candidates SET status = 'hired' WHERE id = $1`, id)
	h.hired.row = object("promotion_date", nil, "nip", "EMP-001", "join_date", nil, "employment_status", "probation",
		"is_active", true, "has_account", false, "department_name", "Bar", "job_title", "Barista", "reporting_to_name", nil,
		"onboarding_total", int32(4), "onboarding_completed", int32(1))
	c = h.as(h.hr, "GET", "/api/candidates/"+id+"/report", nil)
	text, _ = extract.PDFText([]byte(c.raw))
	for _, want := range []string{"EMP-001", "Probation", "Belum dibuat", "1/4 item selesai", "Aktif"} {
		if !strings.Contains(text, want) {
			t.Fatalf("%q missing in %q", want, text)
		}
	}
}

func TestDeleteCandidatePurgesPsikotesFiles(t *testing.T) {
	h := newFileHarness(t)
	id := h.candidate("psikotes")
	session := h.scalar(`INSERT INTO recruitment.psikotes_sessions (candidate_id, token, status) VALUES ($1, $2, 'completed') RETURNING id::text`, id, strings.Repeat("ef", 32))
	h.writePrivate("psikotes/"+session+"/t/a.png", pngBytes)
	h.writePrivate("psikotes/other/keep.png", pngBytes)

	testutil.GrantMenu(t, h.hr.RoleID, "hris.recruitment.candidates", []string{"read", "delete"})
	expect(t, h.as(h.none, "DELETE", "/api/candidates/"+id, nil), 403, "")
	expect(t, h.as(h.hr, "DELETE", "/api/candidates/x?reason=Kandidat%20uji%20dihapus", nil), 400, "ID kandidat tidak valid")
	c := h.as(h.hr, "DELETE", "/api/candidates/"+id+"?reason=Kandidat%20uji%20dihapus", nil)
	if c.status != 200 || c.raw != `{"message":"Kandidat dihapus"}` {
		t.Fatal(c.raw)
	}
	if exists(h.path("private/psikotes/"+session)) || !exists(h.path("private/psikotes/other/keep.png")) {
		t.Fatal("only the candidate's session folders are purged")
	}
	if h.scalar(`SELECT count(*)::text FROM recruitment.psikotes_sessions WHERE id = $1`, session) != "0" {
		t.Fatal("cascade")
	}
	expect(t, h.as(h.hr, "DELETE", "/api/candidates/"+id+"?reason=Kandidat%20uji%20dihapus", nil), 404, "Kandidat tidak ditemukan")
}

func (h *fileHarness) interviewSession(status, config string) (id, token string) {
	token = newPortalToken()
	candidate := h.candidate("interview")
	id = h.scalar(`INSERT INTO recruitment.interview_ai_sessions (candidate_id, token, status, invited_at, expires_at, config)
		VALUES ($1, $2, $3, now(), now() + interval '1 day', $4::jsonb) RETURNING id::text`, candidate, token, status, config)
	return id, token
}

func TestInterviewPortalFlow(t *testing.T) {
	h := newFileHarness(t)
	id, token := h.interviewSession("sent", `{"max_questions": 3}`)
	base := "/api/interview/session/" + token

	expect(t, h.anon("GET", "/api/interview/session/"+strings.Repeat("0", 64), nil), 404, "Link interview tidak berlaku")
	expect(t, h.anon("POST", base+"/start", map[string]any{"webcam_consent": false}), 400, "Interview mewajibkan kamera aktif — izinkan kamera untuk memulai")
	expect(t, h.anon("POST", base+"/start", nil), 400, "Invalid input: expected object, received null")
	// the answer bucket allows 6 a minute per session: the flow below uses all of them

	c := h.anon("POST", base+"/start", map[string]any{"webcam_consent": true})
	expect(t, c, 200, "")
	turn := c.data()["turn"].(map[string]any)
	if turn["turn_no"] != float64(1) || turn["question_audio_base64"] != base64.StdEncoding.EncodeToString(mp3Bytes) ||
		!strings.HasPrefix(turn["question"].(string), "Halo Sari Ayu! Terima kasih sudah meluangkan waktu untuk interview. Perkenalkan diri") {
		t.Fatal(c.raw)
	}
	audioPath := h.scalar(`SELECT question_audio_path FROM recruitment.interview_ai_turns WHERE session_id = $1 AND turn_no = 1`, id)
	if !strings.HasPrefix(audioPath, "interview/"+id+"/questions/") || !exists(h.path("private/"+audioPath)) {
		t.Fatal(audioPath)
	}
	// idempotent restart: the active question, no audio
	if c = h.anon("POST", base+"/start", nil); c.data()["turn"].(map[string]any)["id"] != turn["id"] {
		t.Fatal(c.raw)
	}

	c = h.anon("GET", base, nil)
	s := c.data()["session"].(map[string]any)
	cur := c.data()["current_turn"].(map[string]any)
	if s["status"] != "in_progress" || s["max_questions"] != float64(3) || cur["question_audio_base64"] == nil || s["ai_summary"] != nil {
		t.Fatal(c.raw)
	}
	if strings.Contains(c.raw, "question_audio_path") {
		t.Fatal("storage paths stay private")
	}

	answer := func(fields map[string]string, files ...part) call {
		return h.do(multipartRequest("POST", base+"/answer", fields, files...))
	}
	expect(t, answer(map[string]string{"turn_id": turn["id"].(string), "mode": "text", "answer_text": "   "}), 400, "Jawaban kosong")
	expect(t, answer(map[string]string{"turn_id": turn["id"].(string), "mode": "voice"}, part{"audio", "a.webm", "audio/webm", []byte("not audio at all")}), 400, "Format audio tidak dikenali")

	// voice answer: Whisper transcript; DeepSeek asks the next question
	h.ai.queue(`{"action":"ask","topic":"keahlian","question":"  Apa keahlian utama Anda?  "}`)
	webm := append([]byte{0x1a, 0x45, 0xdf, 0xa3}, bytes.Repeat([]byte{7}, 20)...)
	c = answer(map[string]string{"turn_id": turn["id"].(string), "mode": "voice"}, part{"audio", "a.webm", "audio/webm", webm})
	expect(t, c, 200, "")
	next := c.data()["turn"].(map[string]any)
	if c.data()["done"] != false || next["question"] != "Apa keahlian utama Anda?" || next["turn_no"] != float64(2) {
		t.Fatal(c.raw)
	}
	if got := h.scalar(`SELECT answer_transcript || '|' || answer_mode || '|' || transcribe_model FROM recruitment.interview_ai_turns WHERE id = $1`, turn["id"]); got != "halo dunia|voice|whisper-1" {
		t.Fatal(got)
	}
	if !strings.Contains(strings.Join(h.ai.whisper, ","), "language=id") {
		t.Fatal(h.ai.whisper)
	}
	expect(t, answer(map[string]string{"turn_id": turn["id"].(string), "mode": "text", "answer_text": "lagi"}), 409, "Pertanyaan ini sudah dijawab")

	// the last slot is forced to the salary topic without asking the AI
	c = answer(map[string]string{"turn_id": next["id"].(string), "mode": "text", "answer_text": " Latte art "})
	third := c.data()["turn"].(map[string]any)
	if third["question"] != fallbackQuestions["gaji"] {
		t.Fatal(c.raw)
	}

	// max reached: close with the AI summary
	h.ai.queue(`{"ringkasan":"Cocok.","relevansi":{"skor":"91.4","kesimpulan":"aneh"},"keahlian":["latte",5],"ekspektasi_gaji":{"disebutkan":1,"nilai":5000000}}`)
	c = answer(map[string]string{"turn_id": third["id"].(string), "mode": "text", "answer_text": "5 juta"})
	if c.raw != `{"data":{"done":true}}` {
		t.Fatal(c.raw)
	}
	var summary map[string]any
	_ = json.Unmarshal([]byte(h.scalar(`SELECT ai_summary::text FROM recruitment.interview_ai_sessions WHERE id = $1`, id)), &summary)
	rel := summary["relevansi"].(map[string]any)
	gaji := summary["ekspektasi_gaji"].(map[string]any)
	if rel["skor"] != float64(91) || rel["kesimpulan"] != "cukup_relevan" || gaji["disebutkan"] != true || gaji["nilai"] != "5000000" ||
		fmt.Sprint(summary["keahlian"]) != "[latte 5]" || summary["keterbatasan"] == "" {
		t.Fatalf("%v", summary)
	}
	if got := h.scalar(`SELECT status || '|' || summary_model FROM recruitment.interview_ai_sessions WHERE id = $1`, id); got != "completed|deepseek-chat" {
		t.Fatal(got)
	}
	if got := h.scalar(`SELECT description FROM recruitment.candidate_activities WHERE activity_type = 'interview_ai_completed'
		AND candidate_id = (SELECT candidate_id FROM recruitment.interview_ai_sessions WHERE id = $1)`, id); got != "Interview AI selesai (3 pertanyaan) — kesimpulan AI tersedia" {
		t.Fatal(got)
	}
	expect(t, h.anon("POST", base+"/start", nil), 409, "Sesi interview sudah berakhir")
}

func TestInterviewFallbackWhenAIFails(t *testing.T) {
	h := newFileHarness(t)
	delete(h.settings, "deepseek_api_key")
	_, token := h.interviewSession("sent", `{}`)
	base := "/api/interview/session/" + token
	expect(t, h.do(multipartRequest("POST", base+"/answer", map[string]string{"mode": "text"})), 409, "Sesi tidak sedang berjalan")
	turn := h.anon("POST", base+"/start", map[string]any{"webcam_consent": true}).data()["turn"].(map[string]any)
	expect(t, h.do(multipartRequest("POST", base+"/answer", map[string]string{"turn_id": "x", "mode": "text"})), 400, "Payload tidak valid")
	c := h.do(multipartRequest("POST", base+"/answer", map[string]string{"turn_id": turn["id"].(string), "mode": "text", "answer_text": "Saya Sari"}))
	if c.data()["turn"].(map[string]any)["question"] != fallbackQuestions["keahlian"] {
		t.Fatal(c.raw)
	}
}

func TestRecordingChunksAndList(t *testing.T) {
	h := newFileHarness(t)
	id, token := h.interviewSession("in_progress", `{}`)
	chunk := func(p string, data []byte) call {
		return h.do(multipartRequest("POST", "/api/interview/session/"+token+"/recording-chunk", map[string]string{"part": p}, part{"chunk", "blob", "video/webm", data}))
	}
	header := append([]byte{0x1a, 0x45, 0xdf, 0xa3}, []byte("first")...)

	expect(t, chunk("123", header), 400, "Payload tidak valid")
	expect(t, chunk("1700000000000", []byte("not webm")), 400, "Chunk pertama bukan webm valid")
	r := multipartRequest("POST", "/api/interview/session/"+token+"/recording-chunk", map[string]string{"part": "1700000000000"}, part{"chunk", "b", "video/webm", header})
	r.ContentLength = maxChunkBytes + 64*1024 + 1
	expect(t, h.do(r), 413, "Chunk terlalu besar")

	var want []byte
	for _, b := range [][]byte{header, []byte("-second"), []byte("-third")} {
		c := chunk("1700000000000", b)
		want = append(want, b...)
		if c.status != 200 || c.data()["size"] != float64(len(want)) {
			t.Fatal(c.raw)
		}
	}
	got, _ := os.ReadFile(h.path("private/interview/" + id + "/recording/part-1700000000000.webm"))
	if !bytes.Equal(got, want) {
		t.Fatalf("appended in order: %q", got)
	}

	expect(t, h.as(h.hr, "GET", "/api/interview/sessions/x/recordings", nil), 400, "ID sesi tidak valid")
	expect(t, h.as(h.hr, "GET", "/api/interview/sessions/00000000-0000-4000-8000-000000000000/recordings", nil), 404, "Sesi tidak ditemukan")
	c := h.as(h.hr, "GET", "/api/interview/sessions/"+id+"/recordings", nil)
	l := c.list()
	if len(l) != 1 || l[0].(map[string]any)["path"] != "interview/"+id+"/recording/part-1700000000000.webm" || l[0].(map[string]any)["size"] != float64(len(want)) {
		t.Fatal(c.raw)
	}

	h.exec(`UPDATE recruitment.interview_ai_sessions SET status = 'completed' WHERE id = $1`, id)
	expect(t, chunk("1700000000000", []byte("x")), 409, "Sesi tidak sedang berjalan")
}

// Concurrent chunks of one part run on the pool (the shared test
// transaction is not safe for concurrent use).
func TestConcurrentRecordingChunks(t *testing.T) { concurrentRecordingChunks(t, 1) }

// Two module instances stand in for two API replicas: only the database
// lock is shared between them.
func TestConcurrentRecordingChunksAcrossInstances(t *testing.T) { concurrentRecordingChunks(t, 2) }

func concurrentRecordingChunks(t *testing.T, instances int) {
	dir := t.TempDir()
	t.Setenv("STORAGE_DIR", dir)
	deps := testutil.Deps(t, nil)
	pool := testutil.DB(t)
	ctx := context.Background()
	var candidate, session string
	if err := pool.QueryRow(ctx, `INSERT INTO recruitment.candidates (full_name, email, phone, domicile, source, status)
		VALUES ('Rekam Paralel', 'rekam@contoh.com', '0811', 'Bandung', 'walk_in', 'interview') RETURNING id::text`).Scan(&candidate); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM recruitment.candidates WHERE id = $1`, candidate) })
	token := strings.Repeat("9f", 31) + fmt.Sprintf("%02x", instances)
	if err := pool.QueryRow(ctx, `INSERT INTO recruitment.interview_ai_sessions (candidate_id, token, status, invited_at, expires_at)
		VALUES ($1, $2, 'in_progress', now(), now() + interval '1 day') RETURNING id::text`, candidate, token).Scan(&session); err != nil {
		t.Fatal(err)
	}
	muxes := make([]http.Handler, instances)
	for i := range muxes {
		muxes[i] = testutil.Mux(newModule(deps, pool, Ports{Settings: fakeSettings{}, Speech: &fakeSpeech{}, Hired: fakeHired{}}))
	}
	send := func(mux http.Handler, data []byte) (int, float64) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, multipartRequest("POST", "/api/interview/session/"+token+"/recording-chunk",
			map[string]string{"part": "1700000000001"}, part{"chunk", "b", "video/webm", data}))
		var out struct {
			Data struct {
				Size float64 `json:"size"`
			} `json:"data"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out.Data.Size
	}
	header := append([]byte{0x1a, 0x45, 0xdf, 0xa3}, bytes.Repeat([]byte{'H'}, 100)...)
	if code, _ := send(muxes[0], header); code != 200 {
		t.Fatal(code)
	}
	const n, chunkLen = 12, 64 * 1024
	chunks := make([][]byte, n)
	sizes := make([]float64, n)
	var wg sync.WaitGroup
	for i := range n {
		chunks[i] = bytes.Repeat([]byte{byte('a' + i)}, chunkLen)
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, size := send(muxes[i%instances], chunks[i])
			if code != 200 {
				t.Errorf("chunk %d: %d", i, code)
			}
			sizes[i] = size
		}()
	}
	wg.Wait()
	got, err := os.ReadFile(filepath.Join(dir, "private", "interview", session, "recording", "part-1700000000001.webm"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(header)+n*chunkLen || !bytes.HasPrefix(got, header) {
		t.Fatalf("size %d", len(got))
	}
	for i, c := range chunks {
		if !bytes.Contains(got, c) {
			t.Fatalf("chunk %d interleaved", i)
		}
	}
	// Each size check saw the previous append: the reported sizes are the
	// running totals, every one exactly once.
	slices.Sort(sizes)
	for i, size := range sizes {
		if want := float64(len(header) + (i+1)*chunkLen); size != want {
			t.Fatalf("reported sizes %v", sizes)
		}
	}
}

func TestProctorEvents(t *testing.T) {
	h := newFileHarness(t)
	cand := h.candidate("psikotes")
	token := strings.Repeat("be", 32)
	session := h.scalar(`INSERT INTO recruitment.psikotes_sessions (candidate_id, token, status, invited_at, expires_at, webcam_consent)
		VALUES ($1, $2, 'in_progress', now(), now() + interval '1 day', false) RETURNING id::text`, cand, token)
	url := "/api/psikotes/session/" + token + "/proctor-event"
	snapshot := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)

	expect(t, h.anon("POST", url, map[string]any{"event_type": "hack"}), 400,
		`event_type: Invalid option: expected one of "tab_blur"|"fullscreen_exit"|"paste"|"disconnect"|"webcam_snapshot"`)
	expect(t, h.anon("POST", url, map[string]any{"event_type": "tab_blur", "snapshot": snapshot}), 400, "snapshot: Snapshot hanya untuk event webcam_snapshot")
	expect(t, h.anon("POST", url, map[string]any{"event_type": "tab_blur", "meta": map[string]any{"k": []any{}}}), 400, "meta.k: Invalid input")
	expect(t, h.anon("POST", url, map[string]any{"event_type": "webcam_snapshot", "snapshot": "data:text/html;base64,AA=="}), 400, "snapshot: Format snapshot tidak valid")
	expect(t, h.anon("POST", url, map[string]any{"event_type": "webcam_snapshot", "snapshot": snapshot}), 400, "Consent kamera tidak diberikan")

	c := h.anon("POST", url, map[string]any{"event_type": "tab_blur", "meta": map[string]any{"count": 2, "visible": false}})
	if c.status != 201 || c.data()["event_type"] != "tab_blur" || c.data()["id"] == nil {
		t.Fatal(c.raw)
	}
	h.exec(`UPDATE recruitment.psikotes_sessions SET webcam_consent = true WHERE id = $1`, session)
	expect(t, h.anon("POST", url, map[string]any{"event_type": "webcam_snapshot", "snapshot": "data:image/png;base64,AAAAAAAAAAAAAAAAAAAA"}), 400, "Isi snapshot bukan gambar valid")
	expect(t, h.anon("POST", url, map[string]any{"event_type": "webcam_snapshot", "snapshot": snapshot}), 201, "")
	path := h.scalar(`SELECT storage_path FROM recruitment.psikotes_proctor_events WHERE session_id = $1 AND event_type = 'webcam_snapshot'`, session)
	if !strings.HasPrefix(path, "psikotes/"+session+"/proctor/") || !exists(h.path("private/"+path)) {
		t.Fatal(path)
	}
	if got := h.scalar(`SELECT meta::text FROM recruitment.psikotes_proctor_events WHERE session_id = $1 AND event_type = 'tab_blur'`, session); got != `{"count": 2, "visible": false}` {
		t.Fatal(got)
	}

	// interview: always {ok: true}, the wider event list
	_, itoken := h.interviewSession("in_progress", `{}`)
	c = h.anon("POST", "/api/interview/session/"+itoken+"/proctor-event", map[string]any{"event_type": "face_not_detected"})
	if c.status != 200 || c.raw != `{"data":{"ok":true}}` {
		t.Fatal(c.raw)
	}
	expect(t, h.anon("POST", "/api/interview/session/"+itoken+"/proctor-event", map[string]any{"event_type": "webcam_snapshot"}), 400, "Snapshot kosong")
}

func TestDrawingUploadAndInsight(t *testing.T) {
	h := newFileHarness(t)
	cand := h.candidate("psikotes")
	h.exec(`UPDATE recruitment.candidates SET position_id = NULL WHERE id = $1`, cand)
	token := strings.Repeat("dc", 32)
	session := h.scalar(`INSERT INTO recruitment.psikotes_sessions (candidate_id, token, status, invited_at, expires_at)
		VALUES ($1, $2, 'in_progress', now(), now() + interval '1 day') RETURNING id::text`, cand, token)
	drawing := h.instrument("drawing", `{"duration_seconds": 600}`)
	mcq := h.instrument("mcq", `{}`)
	test := h.scalar(`INSERT INTO recruitment.psikotes_session_tests (session_id, instrument_id, status, started_at)
		VALUES ($1, $2, 'in_progress', now()) RETURNING id::text`, session, drawing)
	other := h.scalar(`INSERT INTO recruitment.psikotes_session_tests (session_id, instrument_id, status, started_at)
		VALUES ($1, $2, 'in_progress', now()) RETURNING id::text`, session, mcq)
	upload := func(testID string, p part) call {
		return h.do(multipartRequest("POST", "/api/psikotes/session/"+token+"/tests/"+testID+"/upload", nil, p))
	}

	expect(t, upload(other, part{"file", "a.png", "image/png", pngBytes}), 400, "Tes ini tidak menerima unggahan gambar")
	expect(t, upload(test, part{"other", "a.png", "image/png", pngBytes}), 400, "File tidak ditemukan di form")
	expect(t, upload(test, part{"file", "a.gif", "image/gif", pngBytes}), 400, "Format harus JPG, PNG, atau WebP")
	expect(t, upload(test, part{"file", "a.png", "image/png", bytes.Repeat([]byte{1}, 20)}), 400, "Isi file bukan gambar JPG/PNG/WebP yang valid")

	c := upload(test, part{"file", "a.png", "image/png", pngBytes})
	if c.raw != `{"data":{"uploaded":true},"message":"Gambar terunggah"}` {
		t.Fatal(c.raw)
	}
	first := h.scalar(`SELECT attachment_path FROM recruitment.psikotes_session_tests WHERE id = $1`, test)
	if !strings.HasPrefix(first, "psikotes/"+session+"/"+test+"/") || !exists(h.path("private/"+first)) {
		t.Fatal(first)
	}
	upload(test, part{"file", "b.png", "image/png", pngBytes})
	second := h.scalar(`SELECT attachment_path FROM recruitment.psikotes_session_tests WHERE id = $1`, test)
	if second == first || exists(h.path("private/"+first)) || !exists(h.path("private/"+second)) {
		t.Fatal("a re-upload replaces the old file")
	}

	// HR insight
	insight := "/api/psikotes/session-tests/" + test + "/ai-insight"
	expect(t, h.as(h.hr, "POST", "/api/psikotes/session-tests/x/ai-insight", map[string]any{}), 400, "ID tes tidak valid")
	expect(t, h.as(h.hr, "POST", insight, map[string]any{}), 409, "Tes belum selesai dikerjakan kandidat")
	h.exec(`UPDATE recruitment.psikotes_session_tests SET status = 'perlu_review' WHERE id = $1`, test)
	expect(t, h.as(h.hr, "POST", insight, map[string]any{"observation": "pendek"}), 400,
		"Tulis observasi gambar minimal 20 karakter, atau kosongkan agar AI membaca gambarnya")
	expect(t, h.as(h.hr, "POST", insight, map[string]any{"observation": 5}), 400, "Invalid input")

	// manual observation → DeepSeek only
	h.ai.queue(`{"ringkasan":"Stabil.","indikasi":[{"aspek":"Batang","insight":"Kokoh"},"x"],"perhatikan_saat_interview":["Target"],"keterbatasan":"Indikatif."}`)
	c = h.as(h.hr, "POST", insight, map[string]any{"observation": "  Pohon besar dengan akar kuat dan batang tebal  "})
	expect(t, c, 200, "")
	d := c.data()
	in := d["insight"].(map[string]any)
	if d["observation"] != "Pohon besar dengan akar kuat dan batang tebal" || d["observation_source"] != "manual" || d["vision_model"] != nil ||
		d["created_by_name"] != "Rina HR" || len(in["indikasi"].([]any)) != 1 || c.body["message"] != "Insight AI dibuat" {
		t.Fatal(c.raw)
	}
	if !strings.Contains(h.ai.userPrompt(t), "OBSERVASI HR TERHADAP GAMBAR KANDIDAT:\nPohon besar") {
		t.Fatal(h.ai.userPrompt(t))
	}

	// empty observation → OpenAI vision reads the drawing first
	h.ai.queue("Pohon di tengah kertas.", `{"ringkasan":"R"}`)
	c = h.as(h.hr, "POST", insight, map[string]any{"observation": ""})
	if c.data()["observation"] != "Pohon di tengah kertas." || c.data()["observation_source"] != "ai" || c.data()["vision_model"] != "gpt-4o-mini" {
		t.Fatal(c.raw)
	}
	if got := h.scalar(`SELECT ai_insight->>'observation_source' FROM recruitment.psikotes_session_tests WHERE id = $1`, test); got != "ai" {
		t.Fatal(got)
	}
	delete(h.settings, "openai_api_key")
	expect(t, h.as(h.hr, "POST", insight, map[string]any{}), 400, string(openAIVisionNotConfigured))
	h.exec(`UPDATE recruitment.psikotes_session_tests SET attachment_path = NULL WHERE id = $1`, test)
	expect(t, h.as(h.hr, "POST", insight, map[string]any{}), 400, "Tes ini tidak punya gambar terunggah — tulis observasi manual")
}

func TestPortalSubmit(t *testing.T) {
	h := newFileHarness(t)
	fields := map[string]string{"full_name": "Budi", "email": "budi@contoh.com", "phone": "0812-3", "domicile": "Bandung",
		"source": "instagram", "expected_salary": " 4500000abc", "brand_id": "", "notes": "Siap shift"}
	photo := part{"photo", "foto.png", "image/png", pngBytes}
	cv := part{"cv", "cv.pdf", "application/pdf", cvPDF(t)}
	submit := func(ip string, f map[string]string, files ...part) call {
		r := multipartRequest("POST", "/api/portal/submit", f, files...)
		r.Header.Set("X-Forwarded-For", ip+", 10.0.0.1")
		return h.do(r)
	}
	without := func(key string) map[string]string {
		out := map[string]string{}
		for k, v := range fields {
			if k != key {
				out[k] = v
			}
		}
		return out
	}
	bad := map[string]string{}
	for k, v := range fields {
		bad[k] = v
	}
	bad["email"] = "bukan-email"

	expect(t, submit("203.0.113.1", without("phone"), photo), 400, "Required fields are missing")
	expect(t, submit("203.0.113.1", bad, photo), 400, "Invalid email format")
	expect(t, submit("203.0.113.1", fields), 400, "A photo is required")
	expect(t, submit("203.0.113.1", fields, photo, part{"cv", "cv.txt", "text/plain", []byte("x")}), 400, "CV must be a PDF or DOC file")
	expect(t, submit("203.0.113.1", fields, part{"photo", "f.gif", "image/gif", pngBytes}), 400, "Photo must be a JPG or PNG file")

	c := submit("203.0.113.2", fields, photo, cv)
	expect(t, c, 200, "")
	id, _ := c.body["candidate_id"].(string)
	if c.body["success"] != true || c.body["message"] != "Application submitted" || id == "" {
		t.Fatal(c.raw)
	}
	got := h.scalar(`SELECT concat_ws('|', status, expected_salary::text, brand_id::text, notes, source, cv_url LIKE '/api/files/cv/candidates/%', photo_url LIKE '/api/files/photos/candidates/%')
		FROM recruitment.candidates WHERE id = $1`, id)
	if got != "applied|4500000|Siap shift|instagram|t|t" {
		t.Fatal(got)
	}
	photoURL := h.scalar(`SELECT photo_url FROM recruitment.candidates WHERE id = $1`, id)
	if b, _ := os.ReadFile(h.path("uploads" + strings.TrimPrefix(photoURL, "/api/files"))); !bytes.Equal(b, pngBytes) {
		t.Fatal("photo stored")
	}

	// five applications per IP per 10 minutes, the first IP of x-forwarded-for
	for range 5 {
		expect(t, submit("203.0.113.9", fields, photo), 200, "")
	}
	if c = submit("203.0.113.9", fields, photo); c.status != 429 || c.body["error"] != "Too many applications from this network. Try again in a few minutes." {
		t.Fatal(c.raw)
	}
	expect(t, submit("203.0.113.10", fields, photo), 200, "")
}

func TestApplicationEmailsEscape(t *testing.T) {
	d := applicationEmail{candidateID: "c-1", fullName: `<a href="https://evil.example">Klik</a>`, email: `x"@evil.example`,
		phone: "0812<script>", domicile: "<b>Bandung</b>", source: "instagram", positionTitle: "Barista", brandName: "Kopi & Co",
		origin: "https://app.example"}
	notes := "<img src=x onerror=alert(1)>"
	d.notes = &notes
	html := candidateConfirmationHTML(d)
	if strings.Contains(html, `<a href="https://evil.example">`) || !strings.Contains(html, "&lt;a href=&quot;https://evil.example&quot;&gt;Klik&lt;/a&gt;") ||
		!strings.Contains(html, "Kopi &amp; Co") {
		t.Fatal(html)
	}
	html = hrdNotificationHTML(d)
	for _, bad := range []string{"<script>", "<img", "<b>Bandung"} {
		if strings.Contains(html, bad) {
			t.Fatal(bad)
		}
	}
	for _, want := range []string{`href="mailto:x&quot;@evil.example"`, `href="https://wa.me/0812"`, `href="https://app.example/dashboard/hris/candidates/c-1"`} {
		if !strings.Contains(html, want) {
			t.Fatal(want)
		}
	}
	if emailSubject("Lamaran\r\nBcc: a@b.c") != "Lamaran Bcc: a@b.c" {
		t.Fatal("subject")
	}
}

func TestReportFormatters(t *testing.T) {
	cases := map[string]string{
		reportFileName("Budi Santoso"):          "laporan-pipeline-budi-santoso.pdf",
		reportFileName("  Émile O'Connor Jr. "): "laporan-pipeline-emile-o-connor-jr.pdf",
		reportFileName("!!!"):                   "laporan-pipeline-kandidat.pdf",
		formatIdr(float64(5000000)):             "Rp 5.000.000",
		formatIdr("7500000"):                    "Rp 7.500.000",
		formatIdr("abc") + formatIdr(nil):       "--",
		formatScore(86.6) + formatScore("70"):   "87/10070/100",
		formatScore("n/a") + formatScore(""):    "--",
		formatDate("2026-07-15T09:30:00Z"):      "15 Juli 2026",
		formatDateTime("2026-07-15T09:30:00Z"):  "15 Juli 2026 pukul 16.30",
		formatDateTime("not-a-date"):            "-",
		labelOf(offerStatusLabels, "withdrawn"): "withdrawn",
		labelOf(recommendationLabels, ""):       "-",
		boolLabel(true) + boolLabel(nil):        "Ya-",
		sanitizeText("a → b ✔ “q” 😀 — ok"):      `a -> b v "q"  — ok`,
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}

func TestSessionMaxQuestionsAndParseInt(t *testing.T) {
	for cfg, want := range map[string]int{`{}`: 8, `{"max_questions": 2}`: 8, `{"max_questions": "12"}`: 12, `{"max_questions": 40}`: 15, `{"max_questions": 4.5}`: 5} {
		r := newRow()
		r.Set("config", json.RawMessage(cfg))
		if got := sessionMaxQuestions(r); got != want {
			t.Errorf("%s: %d, want %d", cfg, got, want)
		}
	}
	for in, want := range map[string]int64{" 42abc": 42, "-7": -7, "1e5": 1} {
		if got, ok := jsParseInt(in); !ok || got != want {
			t.Errorf("%q: %d", in, got)
		}
	}
	if _, ok := jsParseInt("abc"); ok {
		t.Error("NaN")
	}
}

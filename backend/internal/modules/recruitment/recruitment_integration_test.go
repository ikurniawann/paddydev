package recruitment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/testutil"
)

// Integration tests run the routes against TEST_DATABASE_URL inside one
// rolled-back transaction per test; staff accounts are committed by
// testutil and removed on cleanup.

type harness struct {
	t     *testing.T
	tx    pgx.Tx
	svc   *Service
	mux   http.Handler
	hr    testutil.Staff // role hrd: sees portal tokens
	other testutil.Staff // recruitment grant, role pos: tokens hidden
	none  testutil.Staff // no grant
	ports *fakePorts
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	deps := testutil.Deps(t, nil)
	grants := map[string][]string{"hris.recruitment": nil, "hris.master": nil}
	h := &harness{
		t:     t,
		hr:    testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", FullName: "Rina HR", Menus: grants}),
		other: testutil.CreateStaff(t, testutil.StaffOptions{Role: "pos", Menus: grants}),
		none:  testutil.CreateStaff(t, testutil.StaffOptions{Menus: map[string][]string{}}),
		ports: &fakePorts{},
	}
	// opened after the staff so its rollback runs before their cleanup
	// (rows in the tx reference the staff users)
	h.tx = testutil.Tx(t)
	m := newModule(deps, h.tx, Ports{Employees: h.ports, Contracts: h.ports})
	h.svc, h.mux = m.h.svc, testutil.Mux(m)
	return h
}

type call struct {
	status int
	body   map[string]any
	raw    string
	header http.Header
}

func (c call) data() map[string]any { m, _ := c.body["data"].(map[string]any); return m }
func (c call) list() []any          { l, _ := c.body["data"].([]any); return l }

// do serves r inside a savepoint, released unless a statement failed: in
// production each TS query autocommits, so a failed request must not
// poison the next one.
func (h *harness) do(r *http.Request) call {
	h.t.Helper()
	ctx := context.Background()
	sp, err := h.tx.Begin(ctx)
	if err != nil {
		h.t.Fatal(err)
	}
	h.svc.db = sp
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, r)
	h.svc.db = h.tx
	if _, err := sp.Exec(ctx, "SELECT 1"); err != nil {
		_ = sp.Rollback(ctx)
	} else if err := sp.Commit(ctx); err != nil {
		h.t.Fatal(err)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return call{rec.Code, body, rec.Body.String(), rec.Header()}
}

func (h *harness) as(s testutil.Staff, method, path string, body any) call {
	h.t.Helper()
	return h.do(testutil.AsStaff(testutil.Request(method, path, body), s))
}

func (h *harness) anon(method, path string, body any) call {
	h.t.Helper()
	return h.do(testutil.Request(method, path, body))
}

func expect(t *testing.T, c call, status int, errMsg string) {
	t.Helper()
	if c.status != status {
		t.Fatalf("status %d, want %d (body %s)", c.status, status, c.raw)
	}
	if errMsg != "" && c.body["error"] != errMsg {
		t.Fatalf("error %q, want %q", c.body["error"], errMsg)
	}
}

func (h *harness) scalar(sql string, args ...any) string {
	h.t.Helper()
	var v *string
	if err := h.tx.QueryRow(context.Background(), sql, args...).Scan(&v); err != nil {
		h.t.Fatalf("query %q: %v", sql, err)
	}
	if v == nil {
		return "<nil>"
	}
	return *v
}

func (h *harness) exec(sql string, args ...any) {
	h.t.Helper()
	if _, err := h.tx.Exec(context.Background(), sql, args...); err != nil {
		h.t.Fatalf("exec %q: %v", sql, err)
	}
}

func (h *harness) candidate(status string) string {
	h.t.Helper()
	return h.scalar(`INSERT INTO recruitment.candidates (full_name, email, phone, domicile, source, status)
		VALUES ('Sari Ayu', 'sari@contoh.com', '081234567890', 'Bandung', 'walk_in', $1) RETURNING id::text`, status)
}

// fakePorts stands in for the HRIS adapters (tested in internal/app).
type fakePorts struct {
	draft     *DraftContract
	rejection string
}

func (f *fakePorts) Employee(ctx context.Context, q database.Querier, id string) (*Row, error) {
	return CollectRow(q.Query(ctx, `SELECT id, nip, full_name FROM hris.employees WHERE id = $1`, id))
}

func (f *fakePorts) CreateDraft(_ context.Context, _ database.Querier, in DraftContract) (string, string, error) {
	f.draft = &in
	if f.rejection != "" {
		return "", f.rejection, nil
	}
	return "0001/" + strings.ToUpper(in.ContractType) + "/X/2026", "", nil
}

func TestCandidatesCRUD(t *testing.T) {
	h := newHarness(t)

	expect(t, h.as(h.none, "GET", "/api/candidates", nil), 403, "Insufficient permissions")

	c := h.as(h.hr, "POST", "/api/candidates", map[string]any{"full_name": "S", "email": "x"})
	expect(t, c, 400, "Nama minimal 2 karakter")
	if _, ok := c.body["details"].([]any); !ok {
		t.Fatal("parseBody keeps the issue list as details")
	}
	expect(t, h.as(h.hr, "POST", "/api/candidates", map[string]any{
		"full_name": "Sari", "email": "s@x.id", "phone": "08-12", "domicile": "Bdg",
	}), 400, "Nomor telepon tidak valid")

	c = h.as(h.hr, "POST", "/api/candidates", map[string]any{
		"full_name": " Sari Ayu ", "email": "sari@contoh.com", "phone": "081234567890", "domicile": "Bandung", "brand_id": "", "notes": "",
	})
	expect(t, c, 201, "")
	if c.body["message"] != "Kandidat berhasil ditambahkan" || c.header.Get("X-RateLimit-Limit") != "100" {
		t.Fatalf("%s %v", c.raw, c.header)
	}
	d := c.data()
	if d["full_name"] != "Sari Ayu" || d["source"] != "walk_in" || d["status"] != "applied" || d["notes"] != nil || d["created_by"] != h.hr.UserID {
		t.Fatalf("%v", d)
	}
	id := d["id"].(string)
	if !strings.HasPrefix(c.raw, `{"data":{"id":`) {
		t.Fatalf("RETURNING * keeps column order: %s", c.raw)
	}

	expect(t, h.as(h.hr, "GET", "/api/candidates/not-a-uuid", nil), 404, "Kandidat tidak ditemukan")
	c = h.as(h.hr, "GET", "/api/candidates/"+id, nil)
	expect(t, c, 200, "")
	if c.data()["brands"] != nil || c.data()["positions"] != nil {
		t.Fatal("refs are null without brand/position")
	}

	// PUT: allowlist (status ignored, it moves through /stage), "" → null
	c = h.as(h.hr, "PUT", "/api/candidates/"+id, map[string]any{"notes": "ok", "status": "hired", "last_education": ""})
	expect(t, c, 200, "")
	if c.data()["notes"] != "ok" || c.data()["status"] != "applied" || c.data()["last_education"] != nil {
		t.Fatalf("%v", c.data())
	}
	expect(t, h.as(h.hr, "PUT", "/api/candidates/"+id, map[string]any{"full_name": nil}), 400, "Invalid input: expected string, received null")
	expect(t, h.as(h.hr, "PUT", "/api/candidates/bad", map[string]any{}), 400, "ID kandidat tidak valid")

	// list: filters as parameters, meta, rate headers
	c = h.as(h.hr, "GET", "/api/candidates?status=applied&search=Sari%20Ayu&limit=1&page=1", nil)
	expect(t, c, 200, "")
	meta := c.body["meta"].(map[string]any)
	if meta["page"] != float64(1) || meta["limit"] != float64(1) || meta["total"].(float64) < 1 || len(c.list()) != 1 {
		t.Fatalf("%v", c.body["meta"])
	}
	expect(t, h.as(h.hr, "GET", "/api/candidates?limit=1000", nil), 400, "Too big: expected number to be <=100")
	expect(t, h.as(h.hr, "GET", "/api/candidates?status=hacked", nil), 400, "")
	c = h.as(h.hr, "GET", "/api/candidates?all=true&status=applied&search=sari%40contoh", nil)
	meta = c.body["meta"].(map[string]any)
	if meta["page"] != float64(1) || meta["totalPages"] != float64(1) || meta["hasNextPage"] != false {
		t.Fatalf("all=true disables paging: %v", meta)
	}
}

func TestStageNotesActivities(t *testing.T) {
	h := newHarness(t)
	id := h.candidate("applied")

	expect(t, h.as(h.hr, "POST", "/api/candidates/"+id+"/stage", map[string]any{"status": "constructor"}), 400, "Status tidak valid")
	c := h.as(h.hr, "POST", "/api/candidates/"+id+"/stage", map[string]any{"status": "applied"})
	if c.body["message"] != "Status tidak berubah" || c.raw != `{"data":{"status":"applied"},"message":"Status tidak berubah"}` {
		t.Fatal(c.raw)
	}
	c = h.as(h.hr, "POST", "/api/candidates/"+id+"/stage", map[string]any{"status": "archived"})
	if c.raw != `{"data":{"status":"archived","previous":"applied"},"message":"Kandidat dipindahkan ke Diarsipkan"}` {
		t.Fatal(c.raw)
	}

	expect(t, h.as(h.hr, "POST", "/api/candidates/"+id+"/notes", map[string]any{"content": "  "}), 400, "Catatan tidak boleh kosong")
	expect(t, h.as(h.hr, "POST", "/api/candidates/"+id+"/notes", map[string]any{"content": 5}), 400, "Catatan tidak boleh kosong")
	c = h.as(h.hr, "POST", "/api/candidates/"+id+"/notes", map[string]any{"content": " Kandidat bagus "})
	expect(t, c, 201, "")
	if c.data()["content"] != "Kandidat bagus" || c.data()["created_by_name"] != "Rina HR" {
		t.Fatalf("%v", c.data())
	}
	if len(h.as(h.hr, "GET", "/api/candidates/"+id+"/notes", nil).list()) != 1 {
		t.Fatal("note listed")
	}

	expect(t, h.as(h.hr, "POST", "/api/candidates/"+id+"/activities", map[string]any{"template": "spam"}), 400, "Template tidak dikenal")
	c = h.as(h.hr, "POST", "/api/candidates/"+id+"/activities", map[string]any{"template": "penolakan"})
	expect(t, c, 201, "")
	if c.data()["description"] != `Template WA "Penolakan Halus" dibuka` {
		t.Fatalf("%v", c.data())
	}
	acts := h.as(h.hr, "GET", "/api/candidates/"+id+"/activities", nil).list()
	// one transaction: created_at ties, so check the set
	byType := map[string]string{}
	for _, a := range acts {
		byType[a.(map[string]any)["activity_type"].(string)] = a.(map[string]any)["description"].(string)
	}
	if len(acts) != 3 || byType["note_added"] == "" || byType["status_change"] != "Tahap diubah: Applied → Diarsipkan" {
		t.Fatalf("%v", byType)
	}
}

func TestScreeningAndSummary(t *testing.T) {
	h := newHarness(t)
	id := h.candidate("screening")

	c := h.as(h.hr, "GET", "/api/candidates/"+id+"/screening", nil)
	if c.raw != `{"data":null}` {
		t.Fatal(c.raw)
	}
	expect(t, h.as(h.hr, "PUT", "/api/candidates/"+id+"/screening", map[string]any{"confirmed_salary": 1.5}), 400, "Gaji harus bilangan bulat")
	expect(t, h.as(h.hr, "PUT", "/api/candidates/"+id+"/screening", map[string]any{"confirmed_salary": -1}), 400, "Gaji tidak boleh negatif")
	c = h.as(h.hr, "PUT", "/api/candidates/"+id+"/screening", map[string]any{"contacted": true, "confirmed_salary": 5000000, "recommendation": "lolos"})
	expect(t, c, 200, "")
	if c.data()["confirmed_salary"] != float64(5000000) || c.data()["interested"] != nil || c.body["message"] != "Hasil screening tersimpan" {
		t.Fatal(c.raw)
	}
	if !strings.Contains(c.raw, `"confirmed_salary":5000000,`) {
		t.Fatal("float8 salary is a JSON number")
	}
	if got := h.scalar(`SELECT description FROM recruitment.candidate_activities WHERE candidate_id = $1`, id); got != "Hasil screening disimpan (rekomendasi: Lolos)" {
		t.Fatal(got)
	}

	expect(t, h.as(h.hr, "PUT", "/api/candidates/"+id+"/psikotes/summary", map[string]any{"recommendation": "maybe"}), 400, `Invalid option: expected one of "lolos"|"hold"|"tidak_lolos"`)
	c = h.as(h.hr, "PUT", "/api/candidates/"+id+"/psikotes/summary", map[string]any{"notes": " oke "})
	expect(t, c, 200, "")
	if c.data()["notes"] != "oke" || c.data()["recommendation"] != nil {
		t.Fatal(c.raw)
	}
}

// instrument creates an active instrument with questions in the test tx.
func (h *harness) instrument(kind string, config string, questions ...[2]string) string {
	h.t.Helper()
	id := h.scalar(`INSERT INTO recruitment.psikotes_instruments (code, name, kind, config, sort_order)
		VALUES ('go_' || substr(md5(random()::text), 1, 8), 'Tes ' || $1, $1, $2::jsonb, 0) RETURNING id::text`, kind, config)
	for i, q := range questions {
		h.exec(`INSERT INTO recruitment.psikotes_questions (instrument_id, body, options, answer_key, sort_order)
			VALUES ($1, 'Soal', $2::jsonb, $3::jsonb, $4)`, id, q[0], q[1], i)
	}
	return id
}

func TestPsikotesFlow(t *testing.T) {
	h := newHarness(t)
	id := h.candidate("psikotes")
	mcq := h.instrument("mcq", `{"duration_seconds": 600}`,
		[2]string{`[{"key":"a","text":"4"},{"key":"b","text":"5"}]`, `{"correct":"a"}`},
		[2]string{`[{"key":"a","text":"x"},{"key":"b","text":"y"}]`, `{"correct":"b"}`})
	papi := h.instrument("forced_choice", `{}`,
		[2]string{`{"a":{"text":"Saya rajin","scale":"N"},"b":{"text":"Saya tenang","scale":"E"}}`, `null`})

	expect(t, h.as(h.hr, "POST", "/api/candidates/"+id+"/psikotes/sessions", map[string]any{"instrument_ids": []any{}}), 400, "Pilih minimal satu instrumen")
	expect(t, h.as(h.hr, "POST", "/api/candidates/"+id+"/psikotes/sessions", map[string]any{"instrument_ids": []any{mcq, mcq}}), 400, "Instrumen duplikat")
	expect(t, h.as(h.hr, "POST", "/api/candidates/"+id+"/psikotes/sessions", map[string]any{"instrument_ids": []any{"x"}}), 400, "ID instrumen tidak valid")
	c := h.as(h.hr, "POST", "/api/candidates/"+id+"/psikotes/sessions", map[string]any{"instrument_ids": []any{mcq, papi}, "expires_days": 2})
	expect(t, c, 201, "")
	token := c.data()["token"].(string)
	if len(token) != 64 || c.body["message"] != "Undangan tes dibuat" {
		t.Fatal(c.raw)
	}

	// HR panel: token only for link-sharing roles
	c = h.as(h.hr, "GET", "/api/candidates/"+id+"/psikotes", nil)
	sessions := c.data()["sessions"].([]any)
	s0 := sessions[0].(map[string]any)
	if s0["token"] != token || len(s0["tests"].([]any)) != 2 || s0["proctor"].(map[string]any)["flags"] != float64(0) || c.data()["summary"] != nil {
		t.Fatal(c.raw)
	}
	if h.as(h.other, "GET", "/api/candidates/"+id+"/psikotes", nil).data()["sessions"].([]any)[0].(map[string]any)["token"] != nil {
		t.Fatal("token hidden for other roles")
	}

	// portal
	expect(t, h.anon("GET", "/api/psikotes/session/bukan-token", nil), 404, "Link tes tidak berlaku")
	c = h.anon("GET", "/api/psikotes/session/"+token, nil)
	expect(t, c, 200, "")
	tests := c.data()["tests"].([]any)
	// Both tests share sort_order, so the list order is arbitrary: pick by kind.
	byKind := map[string]map[string]any{}
	for _, raw := range tests {
		st := raw.(map[string]any)
		byKind[st["instrument"].(map[string]any)["kind"].(string)] = st
	}
	t0 := byKind["mcq"]
	if c.data()["session"].(map[string]any)["status"] != "sent" || t0 == nil || t0["has_attachment"] != false || t0["instrument"].(map[string]any)["duration_seconds"] != float64(600) {
		t.Fatal(c.raw)
	}
	mcqTest := t0["id"].(string)
	papiTest := byKind["forced_choice"]["id"].(string)

	expect(t, h.anon("POST", "/api/psikotes/session/"+token+"/tests/"+mcqTest+"/start", nil), 409, "Sesi belum dimulai atau sudah berakhir")
	expect(t, h.anon("POST", "/api/psikotes/session/"+token+"/start", map[string]any{}), 400, "Payload tidak valid")
	c = h.anon("POST", "/api/psikotes/session/"+token+"/start", map[string]any{"webcam_consent": true})
	if c.status != 200 || c.data()["status"] != "in_progress" || c.body["message"] != "Sesi dimulai" {
		t.Fatal(c.raw)
	}

	c = h.anon("POST", "/api/psikotes/session/"+token+"/tests/"+mcqTest+"/start", nil)
	expect(t, c, 200, "")
	qs := c.data()["questions"].([]any)
	if len(qs) != 2 || strings.Contains(c.raw, "correct") || c.data()["ends_at"] == nil {
		t.Fatal(c.raw)
	}
	q1, q2 := qs[0].(map[string]any)["id"].(string), qs[1].(map[string]any)["id"].(string)
	c = h.anon("PUT", "/api/psikotes/session/"+token+"/tests/"+mcqTest+"/answers", map[string]any{"answers": map[string]any{q1: " a ", "11111111-1111-4111-8111-111111111111": "b"}})
	if c.status != 200 || c.data()["saved"] != float64(1) {
		t.Fatal(c.raw)
	}
	expect(t, h.anon("PUT", "/api/psikotes/session/"+token+"/tests/"+mcqTest+"/answers", map[string]any{"answers": map[string]any{"x": "a"}}), 400, "Payload tidak valid")

	if c = h.anon("POST", "/api/psikotes/session/"+token+"/finish", nil); c.status != 400 ||
		!strings.HasPrefix(c.body["error"].(string), "Masih ada tes yang belum selesai: ") || !strings.Contains(c.raw, "Tes mcq") {
		t.Fatal(c.raw)
	}

	c = h.anon("POST", "/api/psikotes/session/"+token+"/tests/"+mcqTest+"/finish", map[string]any{"answers": map[string]any{q2: "a"}})
	if c.status != 200 || c.data()["score"] != "50" || c.body["message"] != "Tes selesai" {
		t.Fatal(c.raw)
	}
	c = h.anon("POST", "/api/psikotes/session/"+token+"/tests/"+mcqTest+"/finish", nil)
	if c.raw != `{"data":{"status":"selesai"},"message":"Tes sudah selesai"}` {
		t.Fatal(c.raw)
	}

	h.anon("POST", "/api/psikotes/session/"+token+"/tests/"+papiTest+"/start", nil)
	c = h.anon("POST", "/api/psikotes/session/"+token+"/tests/"+papiTest+"/start", nil)
	pq := c.data()["questions"].([]any)[0].(map[string]any)
	if strings.Contains(c.raw, `"scale"`) || pq["options"].(map[string]any)["a"].(map[string]any)["text"] != "Saya rajin" {
		t.Fatal(c.raw)
	}
	c = h.anon("POST", "/api/psikotes/session/"+token+"/tests/"+papiTest+"/finish", map[string]any{"answers": map[string]any{pq["id"].(string): "b"}})
	if c.data()["score"] != nil || c.data()["status"] != "selesai" {
		t.Fatal(c.raw)
	}
	if got := h.scalar(`SELECT score_detail->'dominant'->0->>'code' FROM recruitment.psikotes_session_tests WHERE id = $1`, papiTest); got != "E" {
		t.Fatal(got)
	}

	c = h.anon("POST", "/api/psikotes/session/"+token+"/finish", nil)
	if c.status != 200 || c.data()["status"] != "completed" || c.body["message"] != "Seluruh tes selesai — terima kasih" {
		t.Fatal(c.raw)
	}
	if h.anon("POST", "/api/psikotes/session/"+token+"/finish", nil).raw != `{"data":{"status":"completed"},"message":"Sesi sudah selesai"}` {
		t.Fatal("finish is idempotent")
	}
	if got := h.scalar(`SELECT created_by_name FROM recruitment.candidate_activities WHERE candidate_id = $1 AND activity_type = 'psikotes_completed'`, id); got != "Sistem" {
		t.Fatal(got)
	}

	// HR answer breakdown follows the score snapshot; deleted questions read null
	c = h.as(h.hr, "GET", "/api/psikotes/session-tests/"+mcqTest+"/answers", nil)
	items := c.data()["items"].([]any)
	if c.data()["kind"] != "mcq" || len(items) != 2 || items[0].(map[string]any)["correct_key"] == nil {
		t.Fatal(c.raw)
	}
	c = h.as(h.hr, "GET", "/api/psikotes/session-tests/"+papiTest+"/answers", nil)
	if c.data()["kind"] != "forced_choice" || c.data()["items"].([]any)[0].(map[string]any)["given"] != "b" {
		t.Fatal(c.raw)
	}
	expect(t, h.as(h.hr, "PUT", "/api/psikotes/session-tests/"+mcqTest+"/review", map[string]any{"review_notes": "ok"}), 409, "Tes ini tidak dalam antrian review manual")
	expect(t, h.as(h.hr, "PUT", "/api/psikotes/session-tests/"+mcqTest+"/review", map[string]any{}), 400, "review_notes: Invalid input: expected string, received undefined")
}

func TestPortalExpiryAndLive(t *testing.T) {
	h := newHarness(t)
	id := h.candidate("psikotes")
	token := strings.Repeat("ab", 32)
	sessionID := h.scalar(`INSERT INTO recruitment.psikotes_sessions (candidate_id, token, status, invited_at, expires_at)
		VALUES ($1, $2, 'sent', now() - interval '3 days', now() - interval '1 minute') RETURNING id::text`, id, token)

	c := h.anon("GET", "/api/psikotes/session/"+token, nil)
	if c.data()["session"].(map[string]any)["status"] != "expired" || h.scalar(`SELECT status FROM recruitment.psikotes_sessions WHERE id = $1`, sessionID) != "expired" {
		t.Fatal(c.raw)
	}
	expect(t, h.anon("POST", "/api/psikotes/session/"+token+"/start", map[string]any{"webcam_consent": true}), 410, "Link tes sudah kedaluwarsa")
	expect(t, h.anon("POST", "/api/psikotes/session/"+token+"/chat", map[string]any{"message": "halo"}), 409, "Sesi sudah berakhir")

	// a running session: chat both ways, frames, signaling
	h.exec(`UPDATE recruitment.psikotes_sessions SET status = 'in_progress', expires_at = now() + interval '1 day' WHERE id = $1`, sessionID)
	expect(t, h.anon("POST", "/api/psikotes/session/"+token+"/chat", map[string]any{"message": "   "}), 400, "Pesan kosong")
	expect(t, h.anon("POST", "/api/psikotes/session/"+token+"/chat", map[string]any{"text": "x"}), 400, "Pesan tidak valid")
	c = h.anon("POST", "/api/psikotes/session/"+token+"/chat", map[string]any{"message": " halo HR "})
	if c.status != 201 || c.data()["message"] != "halo HR" || c.data()["sender_name"] != "Sari Ayu" {
		t.Fatal(c.raw)
	}
	// Requests share the test transaction, so now() is equal for both
	// messages; age the first one so "oldest first" has an order.
	h.exec(`UPDATE recruitment.live_chat_messages SET created_at = created_at - interval '1 second' WHERE id = $1`, c.data()["id"])
	c = h.as(h.hr, "POST", "/api/recruitment/live-monitoring/psikotes/"+sessionID+"/chat", map[string]any{"message": "halo"})
	if c.status != 201 || c.data()["sender"] != "hr" || c.data()["sender_name"] != "Rina HR" {
		t.Fatal(c.raw)
	}
	if msgs := h.anon("GET", "/api/psikotes/session/"+token+"/chat", nil).list(); len(msgs) != 2 || msgs[0].(map[string]any)["sender"] != "candidate" {
		t.Fatal("oldest first")
	}
	expect(t, h.as(h.hr, "GET", "/api/recruitment/live-monitoring/ujian/"+sessionID+"/chat", nil), 404, "Sesi tidak ditemukan")

	expect(t, h.as(h.hr, "GET", "/api/recruitment/live-monitoring/psikotes/"+sessionID+"/frame", nil), 404, "Belum ada frame")
	expect(t, h.anon("POST", "/api/psikotes/session/"+token+"/live-frame", map[string]any{"frame": "data:image/png;base64,aGk="}), 400, "Frame tidak valid")
	c = h.anon("POST", "/api/psikotes/session/"+token+"/live-frame", map[string]any{"frame": "data:image/jpeg;base64,aGk="})
	if c.raw != `{"data":{"ok":true}}` {
		t.Fatal(c.raw)
	}
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, testutil.AsStaff(testutil.Request("GET", "/api/recruitment/live-monitoring/psikotes/"+sessionID+"/frame", nil), h.hr))
	if rec.Code != 200 || rec.Body.String() != "hi" || rec.Header().Get("Content-Type") != "image/jpeg" || !strings.HasSuffix(rec.Header().Get("X-Frame-Updated-At"), "GMT+0000 (Coordinated Universal Time)") {
		t.Fatalf("%d %q %v", rec.Code, rec.Body.String(), rec.Header())
	}
	live := h.as(h.hr, "GET", "/api/recruitment/live-monitoring", nil).list()
	found := false
	for _, s := range live {
		found = found || s.(map[string]any)["session_id"] == sessionID
	}
	if !found {
		t.Fatal("a session with a fresh frame is online")
	}

	base := "/api/recruitment/live-monitoring/psikotes/" + sessionID + "/webrtc"
	expect(t, h.as(h.hr, "POST", base, map[string]any{"offer_id": "x", "sdp": "v=0"}), 400, "Payload tidak valid")
	expect(t, h.as(h.hr, "POST", base, map[string]any{"offer_id": "viewer-0001", "sdp": "v=0"}), 201, "")
	c = h.anon("GET", "/api/psikotes/session/"+token+"/webrtc", nil)
	if c.raw != `{"data":{"offers":[{"offer_id":"viewer-0001","sdp":"v=0"}]}}` {
		t.Fatal(c.raw)
	}
	if h.anon("POST", "/api/psikotes/session/"+token+"/webrtc", map[string]any{"offer_id": "viewer-0001", "sdp": "answer"}).raw != `{"data":{"ok":true}}` {
		t.Fatal("answer stored")
	}
	if c = h.as(h.hr, "GET", base+"?offer_id=viewer-0001", nil); c.raw != `{"data":{"sdp":"answer"}}` {
		t.Fatal(c.raw)
	}
	expect(t, h.as(h.hr, "GET", base+"?offer_id=x", nil), 400, "offer_id tidak valid")

	expect(t, h.as(h.hr, "GET", "/api/psikotes/sessions/bad/proctor-events", nil), 400, "ID sesi tidak valid")
	expect(t, h.as(h.hr, "GET", "/api/interview/sessions/"+sessionID+"/proctor-events", nil), 404, "Sesi tidak ditemukan")
	if c = h.as(h.hr, "GET", "/api/psikotes/sessions/"+sessionID+"/proctor-events", nil); c.raw != `{"data":[]}` {
		t.Fatal(c.raw)
	}
}

func TestInterviewInvitation(t *testing.T) {
	h := newHarness(t)
	id := h.candidate("interview")
	expect(t, h.as(h.hr, "POST", "/api/candidates/"+id+"/interview/sessions", map[string]any{"max_questions": 2}), 400, "Too small: expected number to be >=3")
	c := h.as(h.hr, "POST", "/api/candidates/"+id+"/interview/sessions", map[string]any{})
	expect(t, c, 201, "")
	token := c.data()["token"].(string)
	if got := h.scalar(`SELECT config::text FROM recruitment.interview_ai_sessions WHERE token = $1`, token); got != `{"max_questions": 8}` {
		t.Fatal(got)
	}
	if got := h.scalar(`SELECT description FROM recruitment.candidate_activities WHERE candidate_id = $1`, id); got != "Undangan interview AI dibuat (maks 8 pertanyaan; berlaku 7 hari)" {
		t.Fatal(got)
	}
	c = h.as(h.hr, "GET", "/api/candidates/"+id+"/interview", nil)
	s0 := c.data()["sessions"].([]any)[0].(map[string]any)
	if s0["token"] != token || len(s0["turns"].([]any)) != 0 || s0["config"].(map[string]any)["max_questions"] != float64(8) {
		t.Fatal(c.raw)
	}
	// candidate chat works from the moment the link is sent
	if c = h.anon("POST", "/api/interview/session/"+token+"/chat", map[string]any{"message": "halo"}); c.status != 201 {
		t.Fatal(c.raw)
	}
	expect(t, h.anon("POST", "/api/interview/session/"+token+"/live-frame", map[string]any{"frame": "data:image/jpeg;base64,aGk="}), 409, "Sesi tidak sedang berjalan")
	expect(t, h.anon("GET", "/api/interview/session/"+strings.Repeat("0", 64)+"/chat", nil), 404, "Link interview tidak berlaku")
}

func TestOffers(t *testing.T) {
	h := newHarness(t)
	pos := h.scalar(`INSERT INTO hris.positions (title, salary_min, salary_max) VALUES ('Barista', 4500000.00, 6000000) RETURNING id::text`)
	id := h.scalar(`INSERT INTO recruitment.candidates (full_name, email, phone, domicile, source, status, position_id, expected_salary)
		VALUES ('Budi', 'b@x.id', '081234567890', 'Jakarta', 'portal', 'offer', $1, 5000000) RETURNING id::text`, pos)

	expect(t, h.as(h.hr, "POST", "/api/candidates/"+id+"/offers", map[string]any{"base_salary": -1}), 400, "Too small: expected number to be >=0")
	expect(t, h.as(h.hr, "POST", "/api/candidates/"+id+"/offers", map[string]any{"base_salary": 1, "start_date": "1/1/2026"}), 400, "Format tanggal harus YYYY-MM-DD")
	c := h.as(h.hr, "POST", "/api/candidates/"+id+"/offers", map[string]any{"base_salary": 5500000, "benefits": []any{" BPJS "}, "notes": "  "})
	expect(t, c, 201, "")
	firstToken := c.data()["token"].(string)
	c = h.as(h.hr, "POST", "/api/candidates/"+id+"/offers", map[string]any{"base_salary": 5750000.5, "start_date": "2026-11-01", "expires_days": 3})
	if c.data()["version"] != float64(2) {
		t.Fatal(c.raw)
	}
	token := c.data()["token"].(string)

	c = h.as(h.hr, "GET", "/api/candidates/"+id+"/offers", nil)
	ref := c.data()["salary_reference"].(map[string]any)
	offers := c.data()["offers"].([]any)
	if ref["expected_salary"] != float64(5000000) || ref["salary_min"] != float64(4500000) || ref["position_title"] != "Barista" || ref["interview_expectation"] != nil {
		t.Fatal(c.raw)
	}
	o0, o1 := offers[0].(map[string]any), offers[1].(map[string]any)
	if o0["base_salary"] != 5750000.5 || o0["start_date"] != "2026-11-01T00:00:00.000Z" || o1["status"] != "expired" || o1["notes"] != nil {
		t.Fatal(c.raw)
	}
	if h.as(h.other, "GET", "/api/candidates/"+id+"/offers", nil).data()["offers"].([]any)[0].(map[string]any)["token"] != nil {
		t.Fatal("token hidden for other roles")
	}

	// portal
	expect(t, h.anon("GET", "/api/offer/session/"+firstToken, nil), 200, "")
	c = h.anon("GET", "/api/offer/session/"+token, nil)
	offer := c.data()["offer"].(map[string]any)
	if offer["base_salary"] != 5750000.5 || offer["candidate_name"] != "Budi" || offer["notes"] != nil || len(offer) != 12 {
		t.Fatal(c.raw)
	}
	expect(t, h.anon("POST", "/api/offer/session/"+firstToken+"/respond", map[string]any{"action": "accept"}), 409, "Penawaran ini sudah tidak bisa direspons")
	expect(t, h.anon("POST", "/api/offer/session/"+token+"/respond", map[string]any{"action": "maybe"}), 400, `action: Invalid option: expected one of "accept"|"negotiate"|"decline"`)
	expect(t, h.anon("POST", "/api/offer/session/"+token+"/respond", map[string]any{"action": "negotiate", "note": "  "}), 400,
		"Tuliskan catatan negosiasi Anda (mis. angka yang diharapkan)")
	r := testutil.Request("POST", "/api/offer/session/"+token+"/respond", map[string]any{"action": "negotiate", "note": "Minta 6jt"})
	r.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.1")
	c = h.do(r)
	if c.status != 200 || c.data()["status"] != "negotiating" || c.body["message"] != "Respons Anda tercatat" {
		t.Fatal(c.raw)
	}
	if got := h.scalar(`SELECT response_ip FROM recruitment.candidate_offers WHERE token = $1`, token); got != "203.0.113.9" {
		t.Fatal(got)
	}

	// manual response: own {error} bodies
	offerID := h.scalar(`SELECT id::text FROM recruitment.candidate_offers WHERE token = $1`, token)
	c = h.as(h.hr, "PUT", "/api/offers/bad/response", map[string]any{})
	if c.raw != `{"error":"ID offer tidak valid"}` || c.status != 400 {
		t.Fatal(c.raw)
	}
	if c = h.as(h.hr, "PUT", "/api/offers/"+offerID+"/response", map[string]any{"status": "x"}); c.raw != `{"error":"status: Invalid option: expected one of \"negotiating\"|\"accepted\"|\"declined\""}` {
		t.Fatal(c.raw)
	}
	c = h.as(h.hr, "PUT", "/api/offers/"+offerID+"/response", map[string]any{"status": "accepted", "note": "via WA"})
	if c.status != 200 || c.data()["status"] != "accepted" || c.body["message"] != "Respons tercatat" {
		t.Fatal(c.raw)
	}
	if c = h.as(h.hr, "PUT", "/api/offers/"+offerID+"/response", map[string]any{"status": "declined"}); c.raw != `{"error":"Offer ini sudah direspons final oleh kandidat"}` || c.status != 409 {
		t.Fatal(c.raw)
	}
	if c = h.as(h.hr, "PUT", "/api/offers/11111111-1111-4111-8111-111111111111/response", map[string]any{"status": "declined"}); c.status != 404 {
		t.Fatal(c.raw)
	}
	if c = h.as(h.none, "PUT", "/api/offers/"+offerID+"/response", map[string]any{}); c.raw != `{"success":false,"error":"Insufficient permissions"}` {
		t.Fatal(c.raw)
	}
	if got := h.scalar(`SELECT description FROM recruitment.candidate_activities WHERE candidate_id = $1 AND created_by IS NOT NULL AND activity_type = 'offer_response'`, id); got != `Kandidat menerima offer v2 (dicatat manual) — "via WA"` {
		t.Fatal(got)
	}
}

func TestPsikotesBank(t *testing.T) {
	h := newHarness(t)
	inst := h.instrument("mcq", `{"duration_seconds": 600, "instructions": "Kerjakan"}`)
	drawing := h.instrument("drawing", `{}`)

	if len(h.as(h.hr, "GET", "/api/psikotes/instruments", nil).list()) < 2 {
		t.Fatal("instruments listed")
	}
	expect(t, h.as(h.hr, "PUT", "/api/psikotes/instruments/"+inst, map[string]any{}), 400, ": Tidak ada perubahan yang dikirim")
	expect(t, h.as(h.hr, "PUT", "/api/psikotes/instruments/"+inst, map[string]any{"config": map[string]any{"duration_seconds": 10}}), 400, "config.duration_seconds: Durasi minimal 30 detik")
	c := h.as(h.hr, "PUT", "/api/psikotes/instruments/"+inst, map[string]any{"name": " Logika ", "config": map[string]any{"duration_seconds": 900, "question_count": nil}})
	if c.status != 200 || c.data()["name"] != "Logika" || c.data()["config"].(map[string]any)["instructions"] != "Kerjakan" || c.body["message"] != "Instrumen tersimpan" {
		t.Fatal(c.raw)
	}
	expect(t, h.as(h.hr, "PUT", "/api/psikotes/instruments/11111111-1111-4111-8111-111111111111", map[string]any{"is_active": false}), 404, "Instrumen tidak ditemukan")

	expect(t, h.as(h.hr, "POST", "/api/psikotes/instruments/"+drawing+"/questions", map[string]any{}), 400,
		"Instrumen tes gambar tidak memiliki bank soal — atur instruksi di config")
	expect(t, h.as(h.hr, "POST", "/api/psikotes/instruments/"+inst+"/questions", map[string]any{
		"body": "2+2?", "options": []any{map[string]any{"key": "a", "text": "4"}, map[string]any{"key": "b", "text": "5"}}, "answer_key": map[string]any{"correct": "c"},
	}), 400, "answer_key: Kunci jawaban harus salah satu key opsi")
	expect(t, h.as(h.hr, "POST", "/api/psikotes/instruments/"+inst+"/questions", map[string]any{
		"body": "2+2?", "options": []any{map[string]any{"key": "a", "text": "4"}}, "answer_key": map[string]any{"correct": "a"},
	}), 400, "options: Minimal 2 opsi")
	c = h.as(h.hr, "POST", "/api/psikotes/instruments/"+inst+"/questions", map[string]any{
		"body": " 2+2? ", "options": []any{map[string]any{"key": "a", "text": " 4 "}, map[string]any{"key": "b", "text": "5"}}, "answer_key": map[string]any{"correct": "a"},
	})
	expect(t, c, 201, "")
	qid := c.data()["id"].(string)
	if c.data()["body"] != "2+2?" || c.data()["sort_order"] != float64(0) || c.data()["is_active"] != true || !strings.Contains(c.raw, `"options":[{"key":"a","text":"4"}`) {
		t.Fatal(c.raw)
	}
	if len(h.as(h.hr, "GET", "/api/psikotes/instruments/"+inst+"/questions", nil).list()) != 1 {
		t.Fatal("bank listed")
	}
	c = h.as(h.hr, "PUT", "/api/psikotes/questions/"+qid, map[string]any{
		"body": "3+3?", "options": []any{map[string]any{"key": "a", "text": "6"}, map[string]any{"key": "b", "text": "7"}}, "answer_key": map[string]any{"correct": "a"}, "is_active": false,
	})
	if c.status != 200 || c.data()["is_active"] != false {
		t.Fatal(c.raw)
	}
	testutil.GrantMenu(t, h.hr.RoleID, "hris.recruitment.psikotes", []string{"read", "delete"})
	if c = h.as(h.hr, "DELETE", "/api/psikotes/questions/"+qid+"?reason=Soal%20dobel", nil); c.raw != `{"data":{"id":"`+qid+`"},"message":"Soal dihapus"}` {
		t.Fatal(c.raw)
	}
	expect(t, h.as(h.hr, "DELETE", "/api/psikotes/questions/"+qid+"?reason=Soal%20dobel", nil), 404, "Soal tidak ditemukan")
	expect(t, h.as(h.hr, "PUT", "/api/psikotes/questions/zzz", nil), 400, "ID soal tidak valid")
}

func TestJobOpeningsAndPositions(t *testing.T) {
	h := newHarness(t)
	expect(t, h.as(h.hr, "POST", "/api/hris/job-openings", "{oops"), 400, "Body JSON tidak valid")
	expect(t, h.as(h.hr, "POST", "/api/hris/job-openings", map[string]any{"status": "published"}), 400, "Judul lowongan wajib diisi")
	expect(t, h.as(h.hr, "POST", "/api/hris/job-openings", map[string]any{"title": "Kasir", "status": "arsip"}), 400, "Status lowongan tidak valid")
	slug := "barista-senior-" + testutil.RandomHex(3)
	c := h.as(h.hr, "POST", "/api/hris/job-openings", map[string]any{"title": " Barista Senior ", "slug": slug, "status": "published", "headcount": "3"})
	expect(t, c, 201, "")
	id := c.data()["id"].(string)
	if c.data()["headcount"] != float64(3) || c.data()["published_at"] == nil || c.data()["location"] != "Jakarta, ID" || c.body["message"] != "Lowongan berhasil dibuat" {
		t.Fatal(c.raw)
	}
	if _, embedded := c.data()["brand"]; embedded {
		t.Fatal("writes return the plain row")
	}
	expect(t, h.as(h.hr, "POST", "/api/hris/job-openings", map[string]any{"title": "X", "headcount": "abc"}), 400, "Format data tidak valid")

	list := h.as(h.hr, "GET", "/api/hris/job-openings", nil).list()
	var mine map[string]any
	for _, o := range list {
		if o.(map[string]any)["id"] == id {
			mine = o.(map[string]any)
		}
	}
	if mine == nil || mine["brand"] != nil || mine["department_ref"] != nil {
		t.Fatalf("%v", mine)
	}
	public := h.anon("GET", "/api/job-openings/public", nil).list()
	if len(public) == 0 || public[0].(map[string]any)["status"] != nil {
		t.Fatal("public rows have the selected columns only")
	}

	c = h.as(h.hr, "PATCH", "/api/hris/job-openings/"+id, map[string]any{"title": "Barista", "status": "closed"})
	if c.status != 200 || c.data()["status"] != "closed" || c.data()["published_at"] != nil || c.body["message"] != "Lowongan berhasil diperbarui" {
		t.Fatal(c.raw)
	}
	expect(t, h.as(h.hr, "PATCH", "/api/hris/job-openings/11111111-1111-4111-8111-111111111111", map[string]any{"title": "x"}), 500, "Terjadi kesalahan server")
	testutil.GrantMenu(t, h.hr.RoleID, "hris.recruitment.job-portal", []string{"read", "delete"})
	if c = h.as(h.hr, "DELETE", "/api/hris/job-openings/"+id+"?reason=Lowongan%20ditutup", nil); c.raw != `{"message":"Lowongan berhasil dihapus"}` {
		t.Fatal(c.raw)
	}

	expect(t, h.as(h.hr, "GET", "/api/positions?brand_id=x", nil), 400, "Brand tidak valid")
	c = h.as(h.hr, "POST", "/api/positions", map[string]any{"title": " ", "brand_id": ""})
	expect(t, c, 400, "Validation failed")
	if c.body["details"].([]any)[0].(map[string]any)["message"] != "Nama jabatan wajib diisi" {
		t.Fatal(c.raw)
	}
	c = h.as(h.hr, "POST", "/api/positions", map[string]any{"title": "Go Test Supervisor", "department": ""})
	if c.status != 201 || c.data()["department"] != "Operations" || c.data()["level"] != "Staff" || c.data()["brands"] != nil {
		t.Fatal(c.raw)
	}
	c = h.as(h.hr, "GET", "/api/positions", nil)
	if c.body["count"] != float64(len(c.list())) {
		t.Fatal(c.raw)
	}
}

func TestPromote(t *testing.T) {
	h := newHarness(t)
	expect(t, h.as(h.hr, "POST", "/api/hris/promote", "{oops"), 400, "Body JSON tidak valid")
	expect(t, h.as(h.hr, "POST", "/api/hris/promote", map[string]any{}), 400, "candidate_id wajib diisi")
	expect(t, h.as(h.hr, "POST", "/api/hris/promote", map[string]any{"candidate_id": "nope"}), 404, "Kandidat tidak ditemukan")

	applied := h.candidate("applied")
	c := h.as(h.hr, "POST", "/api/hris/promote", map[string]any{"candidate_id": applied})
	expect(t, c, 400, `Status kandidat harus "hired" atau "talent_pool" untuk dipromosikan. Status saat ini: applied`)
	if c.body["details"].(map[string]any)["suggestion"] != `Ubah status kandidat menjadi "hired" terlebih dahulu` {
		t.Fatal(c.raw)
	}

	hired := h.scalar(`INSERT INTO recruitment.candidates (full_name, email, phone, domicile, source, status)
		VALUES ('Go Promote', 'p' || md5(random()::text) || '@x.id', '0812', 'Jkt', 'walk_in', 'hired') RETURNING id::text`)
	h.exec(`INSERT INTO recruitment.candidate_offers (candidate_id, version, token, status, position_title, base_salary)
		VALUES ($1, 1, md5(random()::text), 'accepted', 'Barista', 5000000)`, hired)
	c = h.as(h.hr, "POST", "/api/hris/promote", map[string]any{"candidate_id": hired, "join_date": "2026-08-01", "employment_status": "contract"})
	expect(t, c, 200, "")
	nip, _ := c.body["nip"].(string)
	if !strings.HasPrefix(nip, "EMP-") || c.body["contract_number"] != "0001/PKWT/X/2026" || c.body["employee_id"] == nil ||
		!strings.HasSuffix(c.body["message"].(string), "menjadi karyawan — draft kontrak 0001/PKWT/X/2026 dibuat otomatis") {
		t.Fatal(c.raw)
	}
	d := h.ports.draft
	if d.EndDate == nil || *d.EndDate != "2027-08-01" || *d.BaseSalary != "5000000" || d.Notes != "Draft otomatis saat promote kandidat (dari offer v1 yang diterima) — durasi default 12 bulan, sesuaikan sebelum aktivasi." {
		t.Fatalf("%+v", d)
	}
	expect(t, h.as(h.hr, "POST", "/api/hris/promote", map[string]any{"candidate_id": hired}), 400, "Kandidat sudah dipromosikan menjadi employee")

	pool := h.scalar(`INSERT INTO recruitment.candidates (full_name, email, phone, domicile, source, status)
		VALUES ('Go Pool', 'q' || md5(random()::text) || '@x.id', '0812', 'Jkt', 'walk_in', 'talent_pool') RETURNING id::text`)
	h.ports.rejection = "Karyawan tidak ditemukan"
	c = h.as(h.hr, "POST", "/api/hris/promote", map[string]any{"candidate_id": pool, "employment_status": "internship"})
	if c.body["contract_number"] != nil || !strings.HasSuffix(c.body["message"].(string), "menjadi karyawan") {
		t.Fatal(c.raw)
	}
}

func TestPromoteConcurrentIs409(t *testing.T) {
	// The DB function raises unique_violation when another request already
	// linked the candidate; the route answers 409 with the TS message.
	h := newHarness(t)
	id := h.candidate("hired")
	svc := NewService(h.tx, Ports{Employees: h.ports, Contracts: h.ports}, time.Now, nil)
	h.exec(`SELECT public.promote_candidate_to_employee($1, current_date, 'probation', NULL, NULL)`, id)
	_, err := svc.repo.PromoteCandidate(context.Background(), h.tx, id, "2026-10-04", "probation", nil, nil)
	if !database.IsUniqueViolation(err) {
		t.Fatalf("want 23505, got %v", err)
	}
}

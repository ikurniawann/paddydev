package hris

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/testutil"
	"nuhabit/backend/internal/platform/xlsx"
)

// orgTemplateMatrix downloads the Bagan template of a version and parses it.
func orgTemplateMatrix(t *testing.T, h *harness, s *testutil.Staff, version string) ([][]string, *httptest.ResponseRecorder) {
	t.Helper()
	path := "/api/hris/org-chart/template"
	if version != "" {
		path += "?version=" + strings.ReplaceAll(version, " ", "%20")
	}
	rec := h.send(s, httptest.NewRequest("GET", path, nil))
	if rec.Code != 200 {
		t.Fatalf("template = %d %s", rec.Code, rec.Body.String())
	}
	m, err := xlsx.ParseMatrix(rec.Body.Bytes(), xlsx.ReadOptions{PreferSheet: domain.OrgChartSheet})
	if err != nil {
		t.Fatal(err)
	}
	return m, rec
}

// EPIC-103 R2: the template is the shown version, every cell as text.
func TestOrgChartTemplate(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", iam.HrisOrganization, false)
	none, _ := h.staff("pos", nil, false)
	expect(t, h.do(&none, "GET", "/api/hris/org-chart/template", nil), 403, "")
	expect(t, h.do(&hr, "GET", "/api/hris/org-chart/template?version=Draft%2099", nil), 404, "Versi bagan tidak ditemukan")

	m, rec := orgTemplateMatrix(t, h, &hr, "Draft 6")
	if rec.Header().Get("Content-Type") != xlsx.ContentType || !strings.Contains(rec.Header().Get("Content-Disposition"), "Template Bagan Draft 6.xlsx") {
		t.Fatalf("headers %v", rec.Header())
	}
	if strings.Join(m[0], "|") != strings.Join(domain.OrgChartHeaders, "|") {
		t.Fatalf("header %v", m[0])
	}
	if len(m) != 151 {
		t.Fatalf("rows %d", len(m)-1)
	}
	// EPIC-104 R5: every Bagan column is Text, so Excel keeps codes as typed.
	book, err := excelize.OpenReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	for _, col := range []string{"A", "B", "M"} {
		id, _ := book.GetColStyle(domain.OrgChartSheet, col)
		if st, err := book.GetStyle(id); err != nil || st.NumFmt != 49 {
			t.Fatalf("column %s style %+v %v", col, st, err)
		}
	}
	if got := strings.Join(m[1][:8], "|"); got != "EXE-001||CEO|Ya|EXE|Executive|L0|Tetap/Kontrak" {
		t.Fatalf("first row %q", got)
	}
	var planned, stores bool
	for _, r := range m[1:] {
		planned = planned || r[11] == "Rencana"
		stores = stores || strings.Contains(r[10], ", ")
	}
	if !planned || !stores {
		t.Fatalf("planned %v multi-store %v", planned, stores)
	}
	help, err := xlsx.ParseMatrix(rec.Body.Bytes(), xlsx.ReadOptions{PreferSheet: "Petunjuk"})
	if err != nil {
		t.Fatal(err)
	}
	text, _ := json.Marshal(help)
	if !strings.Contains(string(text), "Dibekukan") || !strings.Contains(string(text), "Tetap/Kontrak") {
		t.Fatalf("Petunjuk %s", text)
	}
	// The template parses back without errors (round-trip, Review Focus 1).
	res, problem := domain.ParseOrgChartUpload(m, nil)
	if problem != "" || len(res.Errors) != 0 || len(res.Nodes) != 150 {
		t.Fatalf("round-trip problem %q errors %+v nodes %d", problem, res.Errors, len(res.Nodes))
	}
}

const orgLeaf = "hris.organization.org-chart"

func orgImporter(h *harness) testutil.Staff {
	s, _ := h.staffGrants("hrd", map[string][]string{orgLeaf: {"read", "approve", "import"}}, false)
	return s
}

// orgUpload posts a Bagan matrix to the import endpoint.
func orgUpload(t *testing.T, h *harness, s *testutil.Staff, fields map[string]string, m [][]string) (call, *httptest.ResponseRecorder) {
	t.Helper()
	rows := make([][]any, len(m))
	for i, r := range m {
		rows[i] = make([]any, len(r))
		for j, c := range r {
			rows[i][j] = c
		}
	}
	data, err := xlsx.Build(xlsx.SheetSpec{Name: domain.OrgChartSheet, Rows: rows})
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	fw, _ := w.CreateFormFile("file", "bagan baru.xlsx")
	_, _ = fw.Write(data)
	_ = w.Close()
	r := httptest.NewRequest(http.MethodPost, "/api/hris/org-chart/import", &body)
	r.Header.Set("Content-Type", w.FormDataContentType())
	r = testutil.AsStaff(r, *s)
	h.exec("SAVEPOINT harness_request")
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, r)
	if _, err := h.tx.Exec(context.Background(), "RELEASE SAVEPOINT harness_request"); err != nil {
		h.exec("ROLLBACK TO SAVEPOINT harness_request")
		h.exec("RELEASE SAVEPOINT harness_request")
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return call{rec.Code, out, rec.Body.String()}, rec
}

func listLen(t *testing.T, v any) int {
	t.Helper()
	l, ok := v.([]any)
	if !ok {
		t.Fatalf("not a list: %#v", v)
	}
	return len(l)
}

func orgPreviewCommit(t *testing.T, h *harness, s *testutil.Staff, label string, m [][]string) (call, call) {
	t.Helper()
	prev, _ := orgUpload(t, h, s, map[string]string{"step": "preview", "label": label}, m)
	expect(t, prev, 200, "")
	fp, _ := prev.data()["fingerprint"].(string)
	com, _ := orgUpload(t, h, s, map[string]string{"step": "commit", "label": label, "fingerprint": fp, "ack_warnings": "true"}, m)
	return prev, com
}

// Review Focus 1: the unchanged template previews clean with an empty diff.
func TestOrgChartImportRoundTrip(t *testing.T) {
	h := newHarness(t)
	hr := orgImporter(h)
	m, _ := orgTemplateMatrix(t, h, &hr, "")
	prev, _ := orgUpload(t, h, &hr, map[string]string{"step": "preview", "label": "Draft 7"}, m)
	expect(t, prev, 200, "")
	d := prev.data()
	if listLen(t, d["errors"]) != 0 {
		t.Fatalf("errors %v", d["errors"])
	}
	diff := d["diff"].(map[string]any)
	if listLen(t, diff["added"])+listLen(t, diff["removed"])+listLen(t, diff["changed"]) != 0 {
		t.Fatalf("diff %v", diff)
	}
	sum := d["summary"].(map[string]any)
	if sum["nodes"] != float64(150) || sum["positions"] != float64(149) || sum["errors"] != float64(0) {
		t.Fatalf("summary %v", sum)
	}
	if v := d["vacancy"].(map[string]any); v["positions"] != float64(149) {
		t.Fatalf("vacancy %v", v)
	}
	if fp, _ := d["fingerprint"].(string); len(fp) != 64 {
		t.Fatalf("fingerprint %q", fp)
	}
	if d["label"] != "Draft 7" || d["base"].(map[string]any)["label"] != "Draft 6" {
		t.Fatalf("label/base %v %v", d["label"], d["base"])
	}
}

func TestOrgChartImportCommit(t *testing.T) {
	h := newHarness(t)
	hr := orgImporter(h)
	m, _ := orgTemplateMatrix(t, h, &hr, "Draft 6")
	var out [][]string
	for _, r := range m {
		switch r[0] {
		case "RTL-056": // a leaf (no child names it as parent)
			continue
		case "RTL-054":
			r = append([]string{}, r...)
			r[2] = "Store Leader Senior"
		}
		out = append(out, r)
	}
	out = append(out, []string{"RTL-900", "RTL-053", "Shopkeeper", "Ya", "RTL", "Retail Operational", "L7", "Tetap/Kontrak", "", "Makassar", "PPMR", "", ""})
	prev, com := orgPreviewCommit(t, h, &hr, "Draft 7", out)
	diff := prev.data()["diff"].(map[string]any)
	raw, _ := json.Marshal(diff)
	if string(raw) != `{"added":["RTL-900"],"changed":[{"code":"RTL-054","fields":["Jabatan"],"title":"Store Leader Senior"}],"removed":["RTL-056"]}` {
		t.Fatalf("diff %s", raw)
	}
	expect(t, com, 200, "")
	if com.body["message"] != "Versi Draft 7 disimpan sebagai draft (150 node)" {
		t.Fatalf("commit %s", com.raw)
	}
	v := com.data()["version"].(map[string]any)
	if v["status"] != "draft" || v["label"] != "Draft 7" || v["nodes"] != float64(150) || v["source_file"] != "bagan baru.xlsx" {
		t.Fatalf("version %v", v)
	}
	id := v["id"].(string)
	if p := h.scalar(`SELECT parent_code FROM hris.org_chart_nodes WHERE version_id = $1 AND code = 'RTL-900'`, id); p != "RTL-053" {
		t.Fatalf("RTL-900 parent %q", p)
	}
	if last := h.scalar(`SELECT code FROM hris.org_chart_nodes WHERE version_id = $1 ORDER BY sort_order DESC LIMIT 1`, id); last != "RTL-900" {
		t.Fatalf("last node %q", last)
	}
	if n := h.scalar(`SELECT count(*)::text FROM hris.org_chart_nodes WHERE version_id = $1 AND level_confidence = 'manual'`, id); n == "0" {
		t.Fatalf("level_confidence not manual")
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'hris.org_chart.version.import' AND entity_id::text = $1`, id); n != "1" {
		t.Fatalf("audit rows %s", n)
	}
	if n := h.scalar(`SELECT count(*)::text FROM hris.org_chart_nodes n JOIN hris.org_chart_versions v ON v.id = n.version_id WHERE v.label = 'Draft 6'`); n != "150" {
		t.Fatalf("Draft 6 nodes %s", n)
	}
	expect(t, h.do(&hr, "POST", "/api/hris/org-chart/versions/"+id+"/approve", nil), 200, "")
}

func TestOrgChartImportErrors(t *testing.T) {
	h := newHarness(t)
	hr := orgImporter(h)
	m, _ := orgTemplateMatrix(t, h, &hr, "")
	m = append(m, []string{"EXE-002", "EXE-001", "Dup", "Ya", "EXE", "Executive", "L2", "", "", "", "", "", ""},
		[]string{"FIN-900", "", "Kepala Kedua", "Ya", "FIN", "Finance", "L1", "", "", "", "", "", ""})
	prev, _ := orgUpload(t, h, &hr, map[string]string{"step": "preview", "label": "Draft 7"}, m)
	expect(t, prev, 200, "")
	var lines []string
	for _, e := range prev.data()["errors"].([]any) {
		em := e.(map[string]any)
		lines = append(lines, em["message"].(string))
		if em["message"] == "Kode EXE-002 dipakai lebih dari sekali (baris 3)" && em["line"] != float64(152) {
			t.Fatalf("dup line %v", em)
		}
	}
	if !strings.Contains(strings.Join(lines, "|"), "dipakai lebih dari sekali") || !strings.Contains(strings.Join(lines, "|"), "tepat satu puncak") {
		t.Fatalf("errors %v", lines)
	}
	fp := prev.data()["fingerprint"].(string)
	com, _ := orgUpload(t, h, &hr, map[string]string{"step": "commit", "label": "Draft 7", "fingerprint": fp, "ack_warnings": "true"}, m)
	expect(t, com, 400, "File masih punya error — perbaiki lalu unggah ulang")
	_, rec := orgUpload(t, h, &hr, map[string]string{"step": "error_file", "label": "Draft 7"}, m)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != xlsx.ContentType || !strings.Contains(rec.Header().Get("Content-Disposition"), "Error - bagan baru.xlsx") {
		t.Fatalf("error file %d %v", rec.Code, rec.Header())
	}
	em, err := xlsx.ParseMatrix(rec.Body.Bytes(), xlsx.ReadOptions{PreferSheet: domain.OrgChartSheet})
	if err != nil {
		t.Fatal(err)
	}
	text, _ := json.Marshal(em)
	if !strings.Contains(string(text), "Keterangan Error") || !strings.Contains(string(text), "dipakai lebih dari sekali") {
		t.Fatalf("error sheet %s", text)
	}
	if n := h.scalar(`SELECT count(*)::text FROM hris.org_chart_versions WHERE label = 'Draft 7'`); n != "0" {
		t.Fatalf("version written: %s", n)
	}
	bad, _ := orgUpload(t, h, &hr, map[string]string{"step": "preview", "label": "Draft 7"}, [][]string{{"Nama"}, {"x"}})
	expect(t, bad, 400, `Judul kolom "Kode" tidak ditemukan — pakai template dari halaman Struktur Org`)
	step, _ := orgUpload(t, h, &hr, map[string]string{"step": "hapus", "label": "Draft 7"}, m)
	expect(t, step, 400, "Langkah import tidak dikenal")
}

// Review Focus 2: labels are unique regardless of case; a repeat commit
// does not create a second version.
func TestOrgChartImportLabel(t *testing.T) {
	h := newHarness(t)
	hr := orgImporter(h)
	m, _ := orgTemplateMatrix(t, h, &hr, "")
	for label, msg := range map[string]string{"": "Label versi wajib diisi", "  ": "Label versi wajib diisi",
		strings.Repeat("D", 41): "Label versi maksimal 40 karakter", "draft 6": "Label versi sudah dipakai"} {
		c, _ := orgUpload(t, h, &hr, map[string]string{"step": "preview", "label": label}, m)
		expect(t, c, 400, msg)
	}
	prev, com := orgPreviewCommit(t, h, &hr, "Draft 7", m)
	expect(t, com, 200, "")
	again, _ := orgUpload(t, h, &hr, map[string]string{"step": "commit", "label": "Draft 7", "fingerprint": prev.data()["fingerprint"].(string), "ack_warnings": "true"}, m)
	expect(t, again, 400, "Label versi sudah dipakai")
	if n := h.scalar(`SELECT count(*)::text FROM hris.org_chart_versions WHERE lower(label) = 'draft 7'`); n != "1" {
		t.Fatalf("Draft 7 rows %s", n)
	}
}

// Review Focus 3: a file or label changed after preview writes nothing.
func TestOrgChartImportFingerprint(t *testing.T) {
	h := newHarness(t)
	hr := orgImporter(h)
	m, _ := orgTemplateMatrix(t, h, &hr, "")
	prev, _ := orgUpload(t, h, &hr, map[string]string{"step": "preview", "label": "Draft 7"}, m)
	fp := prev.data()["fingerprint"].(string)
	if listLen(t, prev.data()["warnings"]) == 0 {
		t.Fatalf("expected warnings (BR-04 + stores)")
	}
	c, _ := orgUpload(t, h, &hr, map[string]string{"step": "commit", "label": "Draft 8", "fingerprint": fp, "ack_warnings": "true"}, m)
	expect(t, c, 409, "File atau label berubah sejak pratinjau — pratinjau ulang")
	m[2][2] = "General Manager Baru"
	c, _ = orgUpload(t, h, &hr, map[string]string{"step": "commit", "label": "Draft 7", "fingerprint": fp, "ack_warnings": "true"}, m)
	expect(t, c, 409, "File atau label berubah sejak pratinjau — pratinjau ulang")
	m[2][2] = "General Manager"
	c, _ = orgUpload(t, h, &hr, map[string]string{"step": "commit", "label": "Draft 7", "fingerprint": fp}, m)
	expect(t, c, 400, `Centang "Saya sudah memeriksa peringatan" sebelum menyimpan`)
	if n := h.scalar(`SELECT count(*)::text FROM hris.org_chart_versions WHERE label IN ('Draft 7', 'Draft 8')`); n != "0" {
		t.Fatalf("versions written: %s", n)
	}
}

func TestOrgChartImportAccess(t *testing.T) {
	h := newHarness(t)
	reader, _ := h.staffGrants("hrd", map[string][]string{orgLeaf: {"read", "approve"}}, false)
	m, _ := orgTemplateMatrix(t, h, &reader, "")
	c, _ := orgUpload(t, h, &reader, map[string]string{"step": "preview", "label": "Draft 7"}, m)
	expect(t, c, 403, "")
	expect(t, h.do(&reader, "DELETE", "/api/hris/org-chart/versions/"+h.orgVersionID("Draft 6"), nil), 403, "")
	importer, _ := h.staffGrants("hrd", map[string][]string{orgLeaf: {"read", "import"}}, false)
	c, _ = orgUpload(t, h, &importer, map[string]string{"step": "preview", "label": "Draft 7"}, m)
	expect(t, c, 200, "")
}

// Review Focus 4: only drafts are deleted.
func TestOrgChartDeleteDraft(t *testing.T) {
	h := newHarness(t)
	hr := orgImporter(h)
	m, _ := orgTemplateMatrix(t, h, &hr, "")
	_, com := orgPreviewCommit(t, h, &hr, "Draft 7", m)
	expect(t, com, 200, "")
	id := com.data()["version"].(map[string]any)["id"].(string)
	del := h.do(&hr, "DELETE", "/api/hris/org-chart/versions/"+id, nil)
	expect(t, del, 200, "")
	if del.body["message"] != "Versi Draft 7 dihapus" || del.data()["label"] != "Draft 7" {
		t.Fatalf("delete %s", del.raw)
	}
	if n := h.scalar(`SELECT count(*)::text FROM hris.org_chart_nodes WHERE version_id = $1`, id); n != "0" {
		t.Fatalf("nodes left %s", n)
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'hris.org_chart.version.delete' AND entity_id::text = $1`, id); n != "1" {
		t.Fatalf("audit %s", n)
	}
	d6 := h.orgVersionID("Draft 6")
	expect(t, h.do(&hr, "POST", "/api/hris/org-chart/versions/"+d6+"/approve", nil), 200, "")
	expect(t, h.do(&hr, "DELETE", "/api/hris/org-chart/versions/"+d6, nil), 409, "Hanya versi draft yang bisa dihapus")
	expect(t, h.do(&hr, "DELETE", "/api/hris/org-chart/versions/00000000-0000-4000-8000-000000000000", nil), 404, "Versi bagan tidak ditemukan")
	expect(t, h.do(&hr, "DELETE", "/api/hris/org-chart/versions/bukan-uuid", nil), 404, "Versi bagan tidak ditemukan")
	if h.orgStatus("Draft 6") != "approved" {
		t.Fatalf("Draft 6 changed")
	}
}

// Review fix: the last remaining version cannot be deleted, so HR always
// has a template to start from.
func TestOrgChartDeleteLastVersion(t *testing.T) {
	h := newHarness(t)
	hr := orgImporter(h)
	h.exec(`DELETE FROM hris.org_chart_versions WHERE label <> 'Draft 6'`)
	expect(t, h.do(&hr, "DELETE", "/api/hris/org-chart/versions/"+h.orgVersionID("Draft 6"), nil), 409,
		"Versi terakhir tidak bisa dihapus — impor versi lain dulu")
	if h.orgStatus("Draft 6") != "draft" {
		t.Fatalf("Draft 6 gone")
	}
}

// EPIC-104 R3 / Review Focus 2: the database itself refuses a label that
// differs only by case, whatever path writes it.
func TestOrgChartLabelLowerUnique(t *testing.T) {
	h := newHarness(t)
	if code := pgErrCodeTx(h, `INSERT INTO hris.org_chart_versions (label) VALUES ('draft 6')`); code != "23505" {
		t.Fatalf("code %q, want 23505", code)
	}
}

// EPIC-104 R4 / Review Focus 3: the 1000-row upper bound is saved whole.
func TestOrgChartImportMaxRows(t *testing.T) {
	h := newHarness(t)
	hr := orgImporter(h)
	m := [][]string{append([]string{}, domain.OrgChartHeaders...),
		{"EXE-001", "", "CEO", "Ya", "EXE", "Executive", "L0", "Tetap/Kontrak", "", "", "", "", ""}}
	for i := 1; i <= 999; i++ {
		m = append(m, []string{fmt.Sprintf("EXE-X%03d", i), "EXE-001", fmt.Sprintf("Staf %d", i), "Ya", "EXE", "Executive", "L7", "", "", "", "", "", ""})
	}
	prev, com := orgPreviewCommit(t, h, &hr, "Draft Besar", m)
	if listLen(t, prev.data()["errors"]) != 0 {
		t.Fatalf("errors %v", prev.data()["errors"])
	}
	expect(t, com, 200, "")
	if com.body["message"] != "Versi Draft Besar disimpan sebagai draft (1000 node)" {
		t.Fatalf("message %v", com.body["message"])
	}
	id := com.data()["version"].(map[string]any)["id"].(string)
	if got := h.scalar(`SELECT count(*)::text || ':' || min(sort_order)::text || ':' || max(sort_order)::text FROM hris.org_chart_nodes WHERE version_id = $1`, id); got != "1000:1:1000" {
		t.Fatalf("nodes %s", got)
	}
	if last := h.scalar(`SELECT code FROM hris.org_chart_nodes WHERE version_id = $1 AND sort_order = 1000`, id); last != "EXE-X999" {
		t.Fatalf("last %s", last)
	}
}

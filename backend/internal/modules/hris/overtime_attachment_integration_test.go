package hris

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

// otUpload posts one overtime documentation file as s (EPIC-066).
func (h *harness) otUpload(s *testutil.Staff, name string, data []byte) call {
	h.t.Helper()
	body, ct := multipartBody(h.t, "file", name, data)
	r := httptest.NewRequest("POST", "/api/hris/overtime/attachments", body)
	r.Header.Set("Content-Type", ct)
	rec := h.send(s, r)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return call{rec.Code, out, rec.Body.String()}
}

var otPDF = []byte("%PDF-1.4\n% dokumentasi lembur\n")

func TestOvertimeAttachmentUploadAndRead(t *testing.T) {
	h, _ := newFileHarness(t)
	hr, _ := h.staff("hrd", []string{menuKepegawaian}, true)
	boss, bossID := h.staff("pos", nil, true, emp{name: "Atasan Dok"})
	staff, own := h.staff("pos", nil, true, emp{name: "Crew Dok", boss: &bossID})
	other, _ := h.staff("pos", nil, true, emp{name: "Crew Lain"})

	expect(t, h.otUpload(&staff, "catatan.txt", []byte("bukan dokumen")), 400, "Isi file bukan PDF/JPG/PNG/WebP yang valid")
	expect(t, h.otUpload(&staff, "besar.pdf", append([]byte("%PDF-1.4\n"), make([]byte, 5*1024*1024)...)), 400,
		"Ukuran dokumentasi maksimal 5 MB")
	saved := h.otUpload(&staff, "bukti.pdf", otPDF)
	expect(t, saved, 200, "")
	path := saved.data()["path"].(string)
	if !strings.HasPrefix(path, "overtime-attachments/"+own+"/") || !strings.HasSuffix(path, ".pdf") {
		t.Fatalf("path = %q", path)
	}
	url := "/api/hris/overtime/attachment/" + path
	for _, who := range []*testutil.Staff{&staff, &boss, &hr} {
		if rec := h.send(who, httptest.NewRequest("GET", url, nil)); rec.Code != 200 ||
			rec.Header().Get("Content-Type") != "application/pdf" {
			t.Fatalf("read = %d %v", rec.Code, rec.Header())
		}
	}
	expect(t, h.do(&other, "GET", url, nil), 403, "Forbidden")
}

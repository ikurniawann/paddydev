package hris

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/extract"
	"nuhabit/backend/internal/platform/storage"
	"nuhabit/backend/internal/platform/testutil"
	"nuhabit/backend/internal/platform/xlsx"
)

// The file routes run against a temporary storage root.

type fakeCompany struct{}

func (fakeCompany) GetMany(_ context.Context, _ database.Querier, keys []string) (map[string]*string, error) {
	values := map[string]string{"company_legal_name": "PT Paddy Nusantara", "company_city": "Bandung",
		"company_signer_name": "Ilham Kurniawan", "company_signer_title": "Direktur"}
	out := map[string]*string{}
	for _, k := range keys {
		if v, ok := values[k]; ok {
			out[k] = &v
		} else {
			out[k] = nil
		}
	}
	return out, nil
}

func (fakeCompany) FirstCompanyName(context.Context, database.Querier) (*string, error) {
	name := "Paddy"
	return &name, nil
}

func newFileHarness(t *testing.T) (*harness, string) {
	h := newHarness(t)
	dir := t.TempDir()
	h.svc.files = storage.New(dir)
	h.svc.company = fakeCompany{}
	return h, dir
}

// send serves a prepared request in a savepoint, like harness.do.
func (h *harness) send(s *testutil.Staff, r *http.Request) *httptest.ResponseRecorder {
	h.t.Helper()
	if s != nil {
		r = testutil.AsStaff(r, *s)
	}
	h.exec("SAVEPOINT harness_request")
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, r)
	if _, err := h.tx.Exec(context.Background(), "RELEASE SAVEPOINT harness_request"); err != nil {
		h.exec("ROLLBACK TO SAVEPOINT harness_request")
		h.exec("RELEASE SAVEPOINT harness_request")
	}
	return rec
}

func pngBytes(t *testing.T) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 8, 6))
	for x := range 8 {
		img.Set(x, 3, color.RGBA{200, 30, 30, 255})
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func dataURL(mime string, b []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b)
}

func readStored(t *testing.T, dir, rel string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "private", rel))
	if err != nil {
		t.Fatalf("stored file %s: %v", rel, err)
	}
	return b
}

func TestClockInOut(t *testing.T) {
	h, dir := newFileHarness(t)
	hr, _ := h.staff("hrd", nil, true)
	staff, own := h.staff("pos", nil, true, emp{name: "Selfie Test"})
	other, _ := h.staff("pos", nil, true)
	shift := h.scalar(`INSERT INTO hris.shifts (name, start_time, end_time, late_tolerance_minutes)
		VALUES ('Pagi Selfie', '08:00', '17:00', 10) RETURNING id::text`)
	h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from) VALUES ($1, 1, $2, '2026-01-01')`, own, shift)
	selfie := pngBytes(t)

	expect(t, h.do(nil, "POST", "/api/hris/attendance", map[string]any{"action": "clock-in"}), 401, "Unauthorized")
	expect(t, h.do(&staff, "POST", "/api/hris/attendance", "not json"), 400, "Validation failed")
	expect(t, h.do(&staff, "POST", "/api/hris/attendance", []any{1}), 400, "Validation failed")
	expect(t, h.do(&staff, "POST", "/api/hris/attendance", map[string]any{"action": "nap"}), 400,
		`Invalid action. Use "clock-in" or "clock-out"`)
	bad := h.do(&staff, "POST", "/api/hris/attendance", map[string]any{"action": "clock-in", "photo": "x"})
	expect(t, bad, 400, "Validation failed")
	if issue := bad.body["details"].([]any)[0].(map[string]any); issue["message"] != "Foto selfie wajib disertakan" {
		t.Fatalf("issue = %v", issue)
	}
	expect(t, h.do(&staff, "POST", "/api/hris/attendance", map[string]any{"action": "clock-in",
		"photo": dataURL("image/png", []byte("hello, not an image"))}), 400, "Isi file bukan gambar JPG/PNG/WebP yang valid")

	// 10:00 WIB against an 08:00 shift with 10 minutes' tolerance: 120 minutes late.
	in := h.do(&staff, "POST", "/api/hris/attendance", map[string]any{"action": "clock-in", "photo": dataURL("image/png", selfie),
		"clock_in_location": map[string]any{"latitude": -6.2, "longitude": 106.8, "address": "Kantor", "extra": "dropped"},
		"employee_id":       "ffffffff-ffff-ffff-ffff-ffffffffffff"}) // ignored for non-HR
	expect(t, in, 200, "")
	d := in.data()
	if in.body["message"] != "Clock-in successful" || d["employee_id"] != own || d["is_late"] != true ||
		d["late_minutes"] != 120.0 || d["shift_id"] != shift || d["status"] != "present" ||
		d["scheduled_start"] != "2026-10-05T01:00:00.000Z" || d["clock_in"] != "2026-10-05T03:00:00.000Z" {
		t.Fatalf("clock-in = %s", in.raw)
	}
	if loc := d["clock_in_location"].(map[string]any); loc["extra"] != nil || loc["address"] != "Kantor" {
		t.Fatalf("location = %v", loc)
	}
	photo := d["clock_in_photo_url"].(string)
	if !strings.HasPrefix(photo, "attendance/"+own+"/") || !strings.HasSuffix(photo, ".png") ||
		!bytes.Equal(readStored(t, dir, photo), selfie) {
		t.Fatalf("selfie path %q", photo)
	}

	again := h.do(&staff, "POST", "/api/hris/attendance", map[string]any{"action": "clock-in", "photo": dataURL("image/png", selfie)})
	if again.status != 400 || again.raw != `{"success":false,"error":"Already clocked in today","attendance_id":"`+d["id"].(string)+`"}` {
		t.Fatalf("duplicate clock-in = %d %s", again.status, again.raw)
	}

	// Photos: the owner and HR read it; another employee and traversal do not.
	rec := h.send(&staff, httptest.NewRequest("GET", "/api/hris/attendance/photo/"+photo, nil))
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" ||
		rec.Header().Get("Cache-Control") != "private, max-age=3600" || !bytes.Equal(rec.Body.Bytes(), selfie) {
		t.Fatalf("photo = %d %v", rec.Code, rec.Header())
	}
	if rec := h.send(&hr, httptest.NewRequest("GET", "/api/hris/attendance/photo/"+photo, nil)); rec.Code != 200 {
		t.Fatalf("hr photo = %d", rec.Code)
	}
	if rec := h.send(&other, httptest.NewRequest("GET", "/api/hris/attendance/photo/"+photo, nil)); rec.Code != 403 {
		t.Fatalf("other photo = %d %s", rec.Code, rec.Body)
	}
	for _, p := range []string{
		"attendance/" + own + "/..%2F..%2Fleave-attachments",
		"attendance/" + own + "/%252e%252e%252Fx.png",
		"leave-attachments/" + own + "/x.png",
		"attendance/" + own,
	} {
		if rec := h.send(&staff, httptest.NewRequest("GET", "/api/hris/attendance/photo/"+p, nil)); rec.Code != 400 {
			t.Fatalf("%s = %d %s", p, rec.Code, rec.Body)
		}
	}
	if rec := h.send(&staff, httptest.NewRequest("GET", "/api/hris/attendance/photo/attendance/"+own+"/missing.png", nil)); rec.Code != 404 {
		t.Fatalf("missing photo = %d", rec.Code)
	}

	out := map[string]any{"action": "clock-out", "attendance_id": d["id"], "photo": dataURL("image/png", selfie), "notes": "pulang"}
	expect(t, h.do(&other, "POST", "/api/hris/attendance", out), 403, "Tidak boleh mengubah absensi karyawan lain")
	expect(t, h.do(&staff, "POST", "/api/hris/attendance", map[string]any{"action": "clock-out",
		"attendance_id": "11111111-1111-4111-8111-111111111111", "photo": dataURL("image/png", selfie)}), 404, "Attendance record not found")
	closed := h.do(&staff, "POST", "/api/hris/attendance", out)
	expect(t, closed, 200, "")
	if c := closed.data(); closed.body["message"] != "Clock-out successful" || c["clock_out"] == nil || c["notes"] != "pulang" ||
		!strings.HasPrefix(c["clock_out_photo_url"].(string), "attendance/"+own+"/") {
		t.Fatalf("clock-out = %s", closed.raw)
	}
	expect(t, h.do(&staff, "POST", "/api/hris/attendance", out), 400, "Sudah clock-out untuk absensi ini")

	// HR clocks another employee in; without a schedule there is no lateness.
	_, free := h.staff("pos", nil, true)
	hrIn := h.do(&hr, "POST", "/api/hris/attendance", map[string]any{"action": "clock-in", "employee_id": free,
		"date": "2026-10-04", "photo": dataURL("image/png", selfie), "notes": ""})
	expect(t, hrIn, 200, "")
	if d := hrIn.data(); d["employee_id"] != free || d["shift_id"] != nil || d["is_late"] != false || d["notes"] != nil {
		t.Fatalf("hr clock-in = %s", hrIn.raw)
	}
	unlinked, _ := h.staff("pos", nil, false)
	expect(t, h.do(&unlinked, "POST", "/api/hris/attendance", map[string]any{"action": "clock-in",
		"photo": dataURL("image/png", selfie)}), 404, "Akun ini tidak terhubung ke data karyawan")
}

func TestAttendanceExport(t *testing.T) {
	h, _ := newFileHarness(t)
	hr, _ := h.staff("hrd", []string{"hris.workforce"}, true)
	staff, _ := h.staff("pos", nil, true)
	dept := h.department("Dapur Ekspor")
	_, worker := h.staff("pos", nil, true, emp{name: "Nanda Ekspor", dept: &dept})
	selfie := pngBytes(t)
	path, err := h.svc.files.SavePrivateImage(selfie, "image/png", "attendance/"+worker)
	if err != nil {
		t.Fatal(err)
	}
	h.exec(`INSERT INTO hris.attendance (employee_id, date, clock_in, clock_out, clock_in_location, status, is_late, late_minutes,
		notes, clock_in_photo_url, clock_out_photo_url)
		VALUES ($1, '2099-06-02', '2099-06-02T01:58:00Z', '2099-06-02T08:58:00Z', '{"latitude": -6.2, "longitude": 106.8, "address": "Kantor"}',
		'present', true, 5, 'kata "kutip"', $2, 'attendance/x/rusak.webp'),
		($1, '2099-06-01', '2099-06-01T01:00:00Z', NULL, NULL, 'present', false, 0, NULL, NULL, NULL)`, worker, path)
	// EPIC-067 R2: the trigger keeps a stored label while clocks and branch stay put.
	h.exec(`UPDATE hris.attendance SET min_work_hours = 9, is_short_hours = true WHERE employee_id = $1 AND date = '2099-06-02'`, worker)
	q := "?employee_id=" + worker + "&start_date=2099-06-01&end_date=2099-06-30"

	csv := h.send(&hr, httptest.NewRequest("GET", "/api/hris/attendance/export"+q, nil))
	if csv.Code != 200 || csv.Header().Get("Content-Type") != "text/csv;charset=utf-8" ||
		csv.Header().Get("Content-Disposition") != `attachment; filename="attendance_export_2026-10-05.csv"` {
		t.Fatalf("csv = %d %v", csv.Code, csv.Header())
	}
	lines := strings.Split(csv.Body.String(), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "NIP,Nama Karyawan,Departemen,Jabatan,Toko,Tanggal") ||
		!strings.Contains(lines[1], `"Nanda Ekspor","Dapur Ekspor","-","-","2099-06-02","2 Jun 2099, 08.58","2 Jun 2099, 15.58","-6.2,106.8 (Kantor)","-"`) ||
		!strings.HasSuffix(lines[0], `Keterlambatan (menit),Kurang Jam,Jam Minimum (jam),Catatan`) ||
		!strings.HasSuffix(lines[1], `"Ya","5","Ya","9.00","kata ""kutip"""`) || !strings.Contains(lines[2], `"2099-06-01"`) ||
		!strings.HasSuffix(lines[2], `"Tidak","0","-","-","-"`) {
		t.Fatalf("csv body:\n%s", csv.Body)
	}

	book := h.send(&hr, httptest.NewRequest("GET", "/api/hris/attendance/export"+q+"&format=xlsx", nil))
	if book.Code != 200 || book.Header().Get("Content-Type") != xlsx.ContentType ||
		book.Header().Get("Content-Disposition") != `attachment; filename="rekap-absensi-2026-10-05.xlsx"` {
		t.Fatalf("xlsx = %d %v", book.Code, book.Header())
	}
	matrix, err := xlsx.ParseMatrix(book.Body.Bytes(), xlsx.ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	text, _ := json.Marshal(matrix)
	for _, want := range []string{"Paddy — Rekap Absensi", "Periode: 1–30 Juni 2099", "Karyawan: Nanda Ekspor", "Toko: Semua Toko",
		"Dicetak: 5 Okt 2026, 10.00 WIB", "Sen, 01 Jun 2099", "Sel, 02 Jun 2099", "08.58", "Hadir"} {
		if !strings.Contains(string(text), want) {
			t.Fatalf("xlsx misses %q: %s", want, text)
		}
	}
	// R2: Kurang Jam and Jam Minimum sit right before Catatan.
	for _, want := range []string{`"Terlambat (menit)","Kurang Jam","Jam Minimum","Catatan"`,
		`"5","Ya","9","kata \"kutip\""`, `"0","-","-",""`} {
		if !strings.Contains(string(text), want) {
			t.Fatalf("xlsx short-hours columns miss %q: %s", want, text)
		}
	}

	doc := h.send(&hr, httptest.NewRequest("GET", "/api/hris/attendance/export"+q+"&format=pdf", nil))
	if doc.Code != 200 || doc.Header().Get("Content-Type") != "application/pdf" ||
		doc.Header().Get("Content-Disposition") != `attachment; filename="rekap-absensi-2026-10-05.pdf"` {
		t.Fatalf("pdf = %d %v", doc.Code, doc.Header())
	}
	pdfText, err := extract.PDFText(doc.Body.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Paddy — Rekap Absensi", "Nanda Ekspor", "Hadir +5m", "kurang jam", "foto tidak dapat", "kata \"kutip\""} {
		if !strings.Contains(pdfText, want) {
			t.Fatalf("pdf misses %q: %s", want, pdfText)
		}
	}

	expect(t, h.do(&hr, "GET", "/api/hris/attendance/export?employee_id="+worker+"&start_date=2098-01-01&end_date=2098-01-31", nil),
		404, "No attendance data found")
	expect(t, h.do(&staff, "GET", "/api/hris/attendance/export", nil), 403, "")
}

// EPIC-075 R1/R2: the export follows the store and late-only filters of
// the Rekap tab and names the store of each attendance.
func TestAttendanceExportBranchFilter(t *testing.T) {
	h, _ := newFileHarness(t)
	hr, _ := h.staff("hrd", []string{"hris.workforce"}, true)
	_, worker := h.staff("pos", nil, true, emp{name: "Rina Toko"})
	tokoA := h.branch("Toko Ekspor A")
	tokoB := h.branch(strings.Repeat("Toko Bernama Sangat Panjang ", 2) + "B")
	codeA := h.scalar(`SELECT code FROM configuration.branches WHERE id = $1`, tokoA)
	h.exec(`INSERT INTO hris.attendance (employee_id, date, clock_in, status, is_late, late_minutes, branch_id) VALUES
		($1, '2099-07-01', '2099-07-01T01:00:00Z', 'present', false, 0, $2),
		($1, '2099-07-02', '2099-07-02T01:30:00Z', 'present', true, 30, $3),
		($1, '2099-07-03', '2099-07-03T01:00:00Z', 'present', false, 0, NULL)`, worker, tokoA, tokoB)
	base := "/api/hris/attendance/export?start_date=2099-07-01&end_date=2099-07-31"
	csvOf := func(q string) []string {
		t.Helper()
		rec := h.send(&hr, httptest.NewRequest("GET", base+q, nil))
		if rec.Code != 200 {
			t.Fatalf("%s = %d %s", q, rec.Code, rec.Body)
		}
		return strings.Split(rec.Body.String(), "\n")[1:]
	}
	if got := csvOf("&branch_id=" + tokoA); len(got) != 1 || !strings.Contains(got[0], `"`+codeA+` - Toko Ekspor A","2099-07-01"`) {
		t.Fatalf("branch A rows = %q", got)
	}
	if got := csvOf("&branch_id=none"); len(got) != 1 || !strings.Contains(got[0], `"-","2099-07-03"`) {
		t.Fatalf("no-store rows = %q", got)
	}
	if got := csvOf("&is_late=true"); len(got) != 1 || !strings.Contains(got[0], `"2099-07-02"`) {
		t.Fatalf("late rows = %q", got)
	}
	if got := csvOf(""); len(got) != 3 {
		t.Fatalf("all rows = %q", got)
	}
	empty := h.branch("Toko Kosong")
	expect(t, h.do(&hr, "GET", base+"&branch_id="+empty, nil), 404, "No attendance data found")

	book := h.send(&hr, httptest.NewRequest("GET", base+"&branch_id="+tokoA+"&format=xlsx", nil))
	matrix, err := xlsx.ParseMatrix(book.Body.Bytes(), xlsx.ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	text, _ := json.Marshal(matrix)
	for _, want := range []string{"Toko: " + codeA + " - Toko Ekspor A", `"Jabatan","Toko","Jam Masuk"`} {
		if !strings.Contains(string(text), want) {
			t.Fatalf("xlsx misses %q: %s", want, text)
		}
	}
	doc := h.send(&hr, httptest.NewRequest("GET", base+"&branch_id="+tokoB+"&format=pdf", nil))
	if doc.Code != 200 || !strings.HasPrefix(doc.Body.String(), "%PDF") {
		t.Fatalf("pdf long store = %d", doc.Code)
	}
}

func TestLeaveAttachmentsAndCovers(t *testing.T) {
	h, dir := newFileHarness(t)
	hr, _ := h.staff("hrd", []string{"hris.kepegawaian"}, true)
	staff, own := h.staff("pos", nil, true)
	other, _ := h.staff("pos", nil, true)
	unlinked, _ := h.staff("pos", nil, false)
	img := pngBytes(t)

	expect(t, h.do(&hr, "POST", "/api/hris/leaves/attachment", map[string]any{"photo": dataURL("image/png", img),
		"employee_id": "../contracts"}), 400, "ID karyawan tidak valid")
	expect(t, h.do(&staff, "POST", "/api/hris/leaves/attachment", "{"), 400, "Body JSON tidak valid")
	expect(t, h.do(&unlinked, "POST", "/api/hris/leaves/attachment", map[string]any{"photo": dataURL("image/png", img)}), 403,
		"Akun ini tidak terhubung ke data karyawan")
	expect(t, h.do(&staff, "POST", "/api/hris/leaves/attachment", map[string]any{"photo": "data:image/gif;base64,R0lG"}), 400,
		"Lampiran harus berupa gambar JPG/PNG/WebP")
	big := append(append([]byte{}, img...), make([]byte, 5*1024*1024)...)
	expect(t, h.do(&staff, "POST", "/api/hris/leaves/attachment", map[string]any{"photo": dataURL("image/png", big)}), 400,
		"Ukuran lampiran maksimal 5 MB")

	saved := h.do(&staff, "POST", "/api/hris/leaves/attachment", map[string]any{"photo": dataURL("image/png", img)})
	expect(t, saved, 200, "")
	path := saved.data()["path"].(string)
	if saved.body["message"] != "Lampiran tersimpan" || !strings.HasPrefix(path, "leave-attachments/"+own+"/") {
		t.Fatalf("attachment = %s", saved.raw)
	}
	for _, c := range []struct {
		who  *testutil.Staff
		want int
	}{{&staff, 200}, {&hr, 200}, {&other, 403}, {nil, 401}} {
		if rec := h.send(c.who, httptest.NewRequest("GET", "/api/hris/leaves/attachment/"+path, nil)); rec.Code != c.want {
			t.Fatalf("attachment read = %d, want %d", rec.Code, c.want)
		}
	}
	if rec := h.send(&staff, httptest.NewRequest("GET", "/api/hris/leaves/attachment/leave-attachments/"+own+"/..%2F..%2Fannouncements%2Fa.png", nil)); rec.Code != 400 {
		t.Fatalf("traversal = %d", rec.Code)
	}
	// HR uploads for an employee.
	forOther := h.do(&hr, "POST", "/api/hris/leaves/attachment", map[string]any{"photo": dataURL("image/png", img), "employee_id": own})
	if !strings.HasPrefix(forOther.data()["path"].(string), "leave-attachments/"+own+"/") {
		t.Fatalf("hr upload = %s", forOther.raw)
	}

	expect(t, h.do(&staff, "POST", "/api/hris/announcements/cover", map[string]any{"image": dataURL("image/png", img)}), 403, "")
	expect(t, h.do(&hr, "POST", "/api/hris/announcements/cover", "nope"), 400, "Cover harus berupa gambar JPG/PNG/WebP")
	expect(t, h.do(&hr, "POST", "/api/hris/announcements/cover", map[string]any{"image": 5}), 400, "Cover harus berupa gambar JPG/PNG/WebP")
	cover := h.do(&hr, "POST", "/api/hris/announcements/cover", map[string]any{"image": dataURL("image/png", img)})
	expect(t, cover, 200, "")
	coverPath := cover.data()["path"].(string)
	if !strings.HasPrefix(coverPath, "announcements/") || !bytes.Equal(readStored(t, dir, coverPath), img) {
		t.Fatalf("cover = %s", cover.raw)
	}
	rec := h.send(&other, httptest.NewRequest("GET", "/api/hris/announcements/cover/"+coverPath, nil))
	if rec.Code != 200 || !bytes.Equal(rec.Body.Bytes(), img) || rec.Header().Get("Cache-Control") != "private, max-age=3600" {
		t.Fatalf("cover read = %d %v", rec.Code, rec.Header())
	}
	for _, p := range []string{"announcements/..%2F" + path, "announcements", "attendance/x.png"} {
		if rec := h.send(&other, httptest.NewRequest("GET", "/api/hris/announcements/cover/"+p, nil)); rec.Code != 400 {
			t.Fatalf("cover %s = %d", p, rec.Code)
		}
	}
}

func multipartBody(t *testing.T, field, name string, data []byte) (*bytes.Buffer, string) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if field != "" {
		fw, err := mw.CreateFormFile(field, name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fw.Write(data)
	} else {
		_ = mw.WriteField("note", "no file")
	}
	_ = mw.Close()
	return &buf, mw.FormDataContentType()
}

func TestContractDocuments(t *testing.T) {
	h, dir := newFileHarness(t)
	hr, _ := h.staff("hrd", []string{"hris.kepegawaian", "hris.compensation"}, true)
	staff, _ := h.staff("pos", nil, true)
	_, worker := h.staff("pos", nil, true, emp{name: "Budi Santoso"})
	h.grantDelete(hr, "hris.kepegawaian.contracts")
	number := "GO-" + testutil.RandomHex(3) + "/PKWT/VII/2026"
	contract := h.scalar(`INSERT INTO hris.employment_contracts (employee_id, contract_number, contract_type, start_date, end_date,
		position_title, department_name, work_location, base_salary, signed_at)
		VALUES ($1, $2, 'pkwt', '2026-08-01', '2027-08-01', 'Kasir', 'Operasional', 'Outlet Paddy Bandung', 4500000, '2026-07-20')
		RETURNING id::text`, worker, number)
	base := "/api/hris/contracts/" + contract

	expect(t, h.do(&staff, "GET", base+"/document", nil), 403, "")
	expect(t, h.do(&hr, "GET", "/api/hris/contracts/abc/document", nil), 400, "ID kontrak tidak valid")
	expect(t, h.do(&hr, "GET", "/api/hris/contracts/11111111-1111-4111-8111-111111111111/document", nil), 404, "Kontrak tidak ditemukan")
	rec := h.send(&hr, httptest.NewRequest("GET", base+"/document", nil))
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/pdf" || rec.Header().Get("Cache-Control") != "no-store" ||
		rec.Header().Get("Content-Length") != strconv.Itoa(rec.Body.Len()) ||
		!strings.HasPrefix(rec.Header().Get("Content-Disposition"), `attachment; filename="kontrak-go-`) {
		t.Fatalf("document = %d %v", rec.Code, rec.Header())
	}
	text, err := extract.PDFText(rec.Body.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"PERJANJIAN KERJA WAKTU TERTENTU (PKWT)", "Nomor: " + number, "20 Juli 2026", "Bandung",
		"Ilham Kurniawan", "Budi Santoso", "Rp 4.500.000,-", "empat juta lima ratus ribu rupiah", "1 Agustus 2027",
		"PASAL 6", "UANG KOMPENSASI", "PASAL 9", "PIHAK KEDUA,"} {
		if !strings.Contains(text, want) {
			t.Fatalf("contract PDF misses %q: %s", want, text)
		}
	}
	// 30 documents per minute per account.
	h.exec(`UPDATE platform.rate_limits SET count = 30 WHERE key = $1`, "contract_pdf_"+hr.UserID)
	expect(t, h.do(&hr, "GET", base+"/document", nil), 429, "Terlalu banyak permintaan, coba lagi sebentar lagi")

	// Signed scan: upload, read, replace, delete.
	expect(t, h.do(&hr, "GET", base+"/signed-document", nil), 404, "Kontrak ini belum punya dokumen bertanda tangan")
	upload := func(field string, data []byte) call {
		body, ct := multipartBody(t, field, "scan.pdf", data)
		r := httptest.NewRequest("POST", base+"/signed-document", body)
		r.Header.Set("Content-Type", ct)
		rec := h.send(&hr, r)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return call{rec.Code, out, rec.Body.String()}
	}
	expect(t, h.do(&hr, "POST", base+"/signed-document", map[string]any{"file": "x"}), 400, "Form data tidak valid")
	expect(t, upload("", nil), 400, "File tidak ditemukan")
	expect(t, upload("file", []byte("plain text, not a scan")), 400, "Isi file bukan PDF/JPG/PNG/WebP yang valid")
	expect(t, upload("file", make([]byte, MaxSignedDocumentBytes+1)), 400, "Ukuran dokumen maksimal 10 MB")
	scan := []byte("%PDF-1.4\n% signed scan\n")
	first := upload("file", scan)
	expect(t, first, 200, "")
	if first.body["message"] != "Dokumen bertanda tangan kontrak "+number+" tersimpan" {
		t.Fatalf("upload = %s", first.raw)
	}
	stored := h.scalar(`SELECT signed_document_url FROM hris.employment_contracts WHERE id = $1`, contract)
	if !strings.HasPrefix(stored, "contract-signed/"+worker+"/") || !bytes.Equal(readStored(t, dir, stored), scan) {
		t.Fatalf("stored scan %q", stored)
	}
	rec = h.send(&hr, httptest.NewRequest("GET", base+"/signed-document", nil))
	wantName := `inline; filename="ttd-` + strings.ReplaceAll(number, "/", "_") + `"`
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/pdf" || rec.Header().Get("Content-Disposition") != wantName ||
		!bytes.Equal(rec.Body.Bytes(), scan) {
		t.Fatalf("signed read = %d %v", rec.Code, rec.Header())
	}
	expect(t, upload("file", append(scan, '2')), 200, "")
	if _, err := os.Stat(filepath.Join(dir, "private", stored)); !os.IsNotExist(err) {
		t.Fatalf("replaced scan still on disk: %v", err)
	}
	replaced := h.scalar(`SELECT signed_document_url FROM hris.employment_contracts WHERE id = $1`, contract)
	if signed := h.scalar(`SELECT signed_at::text FROM hris.employment_contracts WHERE id = $1`, contract); signed != "2026-07-20" {
		t.Fatalf("signed_at overwritten: %s", signed)
	}
	expect(t, h.do(&hr, "DELETE", base+"/signed-document?reason=Scan%20salah%20unggah", nil), 200, "")
	if _, err := os.Stat(filepath.Join(dir, "private", replaced)); !os.IsNotExist(err) {
		t.Fatalf("deleted scan still on disk: %v", err)
	}
	expect(t, h.do(&hr, "DELETE", base+"/signed-document?reason=Scan%20salah%20unggah", nil), 409, "Kontrak ini belum punya dokumen bertanda tangan")
	expect(t, h.do(&hr, "DELETE", "/api/hris/contracts/11111111-1111-4111-8111-111111111111/signed-document?reason=Scan%20salah%20unggah", nil), 404, "Kontrak tidak ditemukan")
	expect(t, h.do(&staff, "DELETE", base+"/signed-document", nil), 403, "")
}

// EPIC-063 final review: Persetujuan Tim links the leave attachment, so the
// direct manager reads their report's file (and nobody else's).
func TestLeaveAttachmentReadableByDirectManager(t *testing.T) {
	h, _ := newFileHarness(t)
	boss, bossID := h.staff("pos", nil, true)
	staff, _ := h.staff("pos", nil, true, emp{boss: &bossID})
	bystander, _ := h.staff("pos", nil, true)
	img := pngBytes(t)

	saved := h.do(&staff, "POST", "/api/hris/leaves/attachment", map[string]any{"photo": dataURL("image/png", img)})
	expect(t, saved, 200, "")
	path := saved.data()["path"].(string)
	for _, c := range []struct {
		who  *testutil.Staff
		want int
	}{{&boss, 200}, {&staff, 200}, {&bystander, 403}} {
		if rec := h.send(c.who, httptest.NewRequest("GET", "/api/hris/leaves/attachment/"+path, nil)); rec.Code != c.want {
			t.Fatalf("attachment read = %d, want %d", rec.Code, c.want)
		}
	}
}

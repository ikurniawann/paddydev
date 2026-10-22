# Input Lembur Setelah Selesai (EPIC-066) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Karyawan menginput lembur **setelah** lembur selesai (dengan istirahat dalam menit dan 1–3 dokumentasi wajib), lalu atasan langsung menyetujui, menolak, atau mengoreksi jamnya sebelum menyetujui.

**Architecture:** Aturan waktu dan jam dibayar menjadi fungsi murni di `backend/internal/modules/hris/domain`. `CreateOvertime` (Go) membalik aturan AP-02 untuk pengajuan milik sendiri: wajib sudah selesai dan periode absensi masih terbuka. Kolom `hours` tetap menyimpan jam yang dibayar (durasi − istirahat), sehingga kalkulator payroll tidak berubah. Lampiran diunggah lebih dulu lewat endpoint multipart dan disimpan sebagai dokumen privat (pola lampiran cuti), lalu path-nya dikirim saat input. `DecideOvertime` menerima koreksi jam dari atasan langsung saat menyetujui.

**Tech Stack:**
- Go: `net/http`, pgx, `platform/storage`, `platform/audit`.
- PostgreSQL 16, migrasi `backend/database/migrations/deltas`.
- Next.js 16 dengan React Query dan Vitest.

**Spec:** `docs/epics/EPIC-066-input-lembur-setelah-selesai.md` (Goal, Keputusan user, Acceptance Criteria). Acuan tarif: `docs/hris/acuan-mom-20261019/aturan-payroll.md`.

## Global Constraints

- **Waktu:** WIB untuk semua aturan waktu. Fungsi yang dipakai: `domain.NowWIB`, `domain.TodayWIB`.
- **Batas input:** pengajuan **milik sendiri** (`source = employee`) baru boleh dibuat setelah tanggal + jam selesai lewat (lembur lewat tengah malam selesai di hari berikutnya). Tanggal harus di periode absensi yang belum di-closing (`Service.ensureAttendanceOpen`).
- **Istirahat:** `break_minutes` bilangan bulat 0–720. Jam dibayar = `(menit selesai − menit mulai, +24 jam bila tidak positif) − break_minutes`, lalu dibagi 60 dan dibulatkan 2 desimal.
- **Durasi:** durasi kotor maksimal 12 jam (`Durasi lembur maksimal 12 jam`). Jam dibayar minimal 1 jam (`Durasi lembur minimal 1 jam setelah dikurangi istirahat`).
- **Lampiran:** 1–3 file, masing-masing ≤ 5 MB. Isi diperiksa dengan sniffing (`storage.SavePrivateDocument`: PDF/JPG/PNG/WebP). Folder privat: `overtime-attachments/<employeeId>/`.
- **Keterangan:** opsional untuk pengajuan milik sendiri. Penugasan HR (`source = company`) tetap wajib alasan ≥ 5 karakter dan tetap ditolak bila jam mulai sudah lewat (perilaku lama, tidak diubah).
- **Koreksi jam:** atasan langsung (`employees.reporting_to`, bukan diri sendiri) mengoreksi pengajuan `source = employee`. HRD hanya boleh mengoreksi bila karyawan tidak punya atasan yang valid. Pesan penolakan: `Koreksi jam lembur hanya oleh atasan langsung`.
- **Tarif:** tidak berubah. Hari kerja biasa 1,5×, libur nasional / istirahat mingguan 2×, Gaji Pokok/173, minimal 1 jam (`payroll/domain.SplitOvertimeWith`).
- **Next.js:** Next 16. Route API baru hanya di Go; tambahkan prefix-nya ke `goNativePrefixes` di `frontend/src/lib/backend-routes.test.ts`, lalu regenerasi `frontend/src/lib/go-routes.generated.json` dengan `go run ./cmd/api -routes`.
- **Teks:** semua pesan untuk user berbahasa Indonesia, persis seperti tertulis di task.

## Review Focus

1. **Lembur lewat tengah malam** (22:00–02:00, input pukul 01:00 hari berikutnya) → ditolak "belum selesai"; input pukul 02:00 → diterima, jam = 4 − istirahat.
2. **Istirahat ≥ durasi**, mis. 17:00–18:00 dengan istirahat 60 → 400 minimal 1 jam, bukan jam negatif atau 0 yang tersimpan.
3. **Path lampiran milik karyawan lain** atau path buatan sendiri (`../`, file tidak ada) → 400, bukan tersimpan.
4. **Atasan mengoreksi jam** lalu menyetujui → `hours` dihitung ulang dengan istirahat baru; karyawan tanpa atasan dikoreksi oleh HR; HR terhadap karyawan yang punya atasan → 403.
5. **Data lama** (`break_minutes` default 0, `attachments` kosong) tetap tampil dan tetap bisa diputus. Lembur lama yang masih pending tidak dipaksa punya lampiran saat diputus.

---

### Task 1: Aturan domain input lembur (Go)

**Files:**
- Create: `backend/internal/modules/hris/domain/overtime_input.go`
- Test: `backend/internal/modules/hris/domain/overtime_input_test.go`

**Interfaces:**
- Produces:
  - `func OvertimeEndsAt(date, start, end string) (time.Time, bool)`: akhir lembur di jam dinding WIB (UTC+7, zona `time.FixedZone("WIB", 7*3600)`), +1 hari bila `end <= start`. `false` bila input tidak bisa diurai.
  - `func OvertimeNotFinished(date, start, end string, now time.Time) bool`
  - `func PaidOvertimeHours(start, end string, breakMinutes int) float64`
  - `func OvertimeGrossMinutes(start, end string) int`
  - `func CanCorrectOvertimeTimes(actor *string, isHRD bool, ownerID string, reportingTo *string) (bool, string)`
  - `const OvertimeCorrectManagerOnly = "Koreksi jam lembur hanya oleh atasan langsung"`

- [ ] **Step 1: Write the failing test**

```go
package domain

import (
	"testing"
	"time"
)

func wib(s string) time.Time {
	t, _ := time.ParseInLocation("2006-01-02 15:04", s, time.FixedZone("WIB", 7*3600))
	return t
}

func TestOvertimeNotFinished(t *testing.T) {
	for _, c := range []struct {
		date, start, end, now string
		want                  bool
	}{
		{"2026-10-21", "17:00", "19:00", "2026-10-21 18:59", true},
		{"2026-10-21", "17:00", "19:00", "2026-10-21 19:00", false},
		{"2026-10-21", "17:00", "19:00", "2026-10-22 08:00", false},
		{"2026-10-21", "22:00", "02:00", "2026-10-22 01:00", true}, // lewat tengah malam
		{"2026-10-21", "22:00", "02:00", "2026-10-22 02:00", false},
	} {
		if got := OvertimeNotFinished(c.date, c.start, c.end, wib(c.now)); got != c.want {
			t.Errorf("%s %s–%s @%s = %v, want %v", c.date, c.start, c.end, c.now, got, c.want)
		}
	}
	if _, ok := OvertimeEndsAt("2026-02-30", "17:00", "19:00"); ok {
		t.Error("tanggal tidak ada harus false")
	}
}

func TestPaidOvertimeHours(t *testing.T) {
	for _, c := range []struct {
		start, end string
		brk        int
		want       float64
	}{
		{"17:00", "19:00", 0, 2},
		{"17:00", "20:00", 30, 2.5},
		{"22:00", "02:00", 60, 3},
		{"17:00", "18:00", 60, 0},
		{"17:00", "17:45", 0, 0.75},
	} {
		if got := PaidOvertimeHours(c.start, c.end, c.brk); got != c.want {
			t.Errorf("%s–%s istirahat %d = %v, want %v", c.start, c.end, c.brk, got, c.want)
		}
	}
	if got := OvertimeGrossMinutes("22:00", "02:00"); got != 240 {
		t.Errorf("gross = %d, want 240", got)
	}
}

func TestCanCorrectOvertimeTimes(t *testing.T) {
	boss, owner, hr := "boss", "owner", "hr"
	self := owner
	for _, c := range []struct {
		name        string
		actor       *string
		isHRD       bool
		reportingTo *string
		want        bool
	}{
		{"atasan langsung", &boss, false, &boss, true},
		{"HR saat ada atasan", &hr, true, &boss, false},
		{"HR tanpa atasan", &hr, true, nil, true},
		{"HR saat atasan menunjuk diri sendiri", &hr, true, &self, true},
		{"karyawan sendiri", &owner, true, nil, false},
		{"orang lain", &hr, false, nil, false},
	} {
		got, msg := CanCorrectOvertimeTimes(c.actor, c.isHRD, owner, c.reportingTo)
		if got != c.want || (!got && msg == "") {
			t.Errorf("%s = %v %q, want %v", c.name, got, msg, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && $HOME/.local/go/bin/go test ./internal/modules/hris/domain/ -run 'TestOvertimeNotFinished|TestPaidOvertimeHours|TestCanCorrectOvertimeTimes'`
Expected: FAIL with `undefined: OvertimeNotFinished`

- [ ] **Step 3: Write minimal implementation**

```go
package domain

import (
	"strconv"
	"strings"
	"time"
)

// Input lembur setelah selesai (EPIC-066): pengajuan milik sendiri dibuat
// setelah jam selesai lewat; jam dibayar = durasi − istirahat.

// OvertimeCorrectManagerOnly is the refusal of a time correction by anyone
// but the direct manager (HR only for an employee without one).
const OvertimeCorrectManagerOnly = "Koreksi jam lembur hanya oleh atasan langsung"

var wibZone = time.FixedZone("WIB", 7*3600)

func clockMinutes(s string) (int, bool) {
	p := strings.Split(s, ":")
	if len(p) < 2 {
		return 0, false
	}
	h, err1 := strconv.Atoi(p[0])
	m, err2 := strconv.Atoi(p[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// OvertimeGrossMinutes is end − start in minutes, past midnight when the end
// is not after the start (0 on unparsable input).
func OvertimeGrossMinutes(start, end string) int {
	s, ok1 := clockMinutes(start)
	e, ok2 := clockMinutes(end)
	if !ok1 || !ok2 {
		return 0
	}
	if e <= s {
		e += 24 * 60
	}
	return e - s
}

// OvertimeEndsAt is the WIB wall-clock end of the overtime; the next day when
// end is not after start. ok is false on unparsable input.
func OvertimeEndsAt(date, start, end string) (time.Time, bool) {
	day, err := time.ParseInLocation(DateLayout, date, wibZone)
	if err != nil {
		return time.Time{}, false
	}
	s, ok1 := clockMinutes(start)
	if !ok1 {
		return time.Time{}, false
	}
	gross := OvertimeGrossMinutes(start, end)
	if gross == 0 {
		return time.Time{}, false
	}
	return day.Add(time.Duration(s+gross) * time.Minute), true
}

// OvertimeNotFinished: the overtime has not ended yet at now (WIB); unparsable
// input is left to the input validation (false).
func OvertimeNotFinished(date, start, end string, now time.Time) bool {
	ends, ok := OvertimeEndsAt(date, start, end)
	return ok && now.Before(ends)
}

// PaidOvertimeHours is (gross minutes − break) / 60, rounded to two decimals,
// never below zero.
func PaidOvertimeHours(start, end string, breakMinutes int) float64 {
	minutes := OvertimeGrossMinutes(start, end) - breakMinutes
	if minutes < 0 {
		minutes = 0
	}
	return Round2(float64(minutes) / 60)
}

// CanCorrectOvertimeTimes: the direct manager corrects an employee's own
// overtime; HRD only when the employee has no valid manager (nil or pointing
// at themselves). Nobody corrects their own overtime.
func CanCorrectOvertimeTimes(actor *string, isHRD bool, ownerID string, reportingTo *string) (bool, string) {
	if samePerson(actor, &ownerID) {
		return false, "Tidak bisa mengoreksi lembur sendiri"
	}
	hasManager := reportingTo != nil && *reportingTo != "" && *reportingTo != ownerID
	if hasManager && samePerson(actor, reportingTo) {
		return true, ""
	}
	if !hasManager && isHRD {
		return true, ""
	}
	return false, OvertimeCorrectManagerOnly
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && $HOME/.local/go/bin/go test ./internal/modules/hris/domain/`
Expected: PASS (`ok nuhabit/backend/internal/modules/hris/domain`)

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/hris/domain/overtime_input.go backend/internal/modules/hris/domain/overtime_input_test.go
git commit -m "feat(hris): aturan domain input lembur setelah selesai (EPIC-066)"
```

### Task 2: Migrasi + unggah/baca dokumentasi lembur

**Files:**
- Create: `backend/database/migrations/deltas/20261026090000_overtime_input_after.sql`
- Modify: `backend/internal/modules/hris/http_files.go`: rute di `fileRoutes()` di bawah dua baris `leaves/attachment`, handler baru di bawah `leaveAttachment`.
- Modify: `frontend/src/lib/go-routes.generated.json` (regenerasi), `frontend/src/lib/backend-routes.test.ts` (`goNativePrefixes`)
- Test: `backend/internal/modules/hris/overtime_attachment_integration_test.go`

**Interfaces:**
- Produces:
  - `POST /api/hris/overtime/attachments` (multipart, field `file`) → `{ data: { path }, message: "Dokumentasi tersimpan" }`. Path berbentuk `overtime-attachments/<employeeId>/<ms>-<16hex>.<pdf|jpg|png|webp>`.
  - `GET /api/hris/overtime/attachment/{path...}`: dibaca pemilik, atasan langsung, dan HR (`a.IsHR`).
  - Konstanta Go `overtimeAttachmentFolder = "overtime-attachments"` dan `maxOvertimeAttachments = 3`, dipakai Task 3.

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test -p 1 ./internal/modules/hris/ -run TestOvertimeAttachmentUploadAndRead`
Expected: FAIL. Upload menjawab 404 karena rute belum ada (`status 404, want 400`).

- [ ] **Step 3: Write minimal implementation**

Migrasi `backend/database/migrations/deltas/20261026090000_overtime_input_after.sql`:

```sql
-- =============================================================================
-- Input lembur setelah selesai (EPIC-066): istirahat (menit) dan dokumentasi
-- wajib untuk input milik sendiri. hours tetap jam dibayar (durasi − istirahat).
-- Baris lama: break_minutes 0, attachments kosong.
-- =============================================================================

ALTER TABLE hris.overtime_requests
  ADD COLUMN IF NOT EXISTS break_minutes integer NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS attachments text[] NOT NULL DEFAULT '{}';

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'overtime_requests_break_minutes_check') THEN
    ALTER TABLE hris.overtime_requests ADD CONSTRAINT overtime_requests_break_minutes_check
      CHECK (break_minutes BETWEEN 0 AND 720);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'overtime_requests_attachments_check') THEN
    ALTER TABLE hris.overtime_requests ADD CONSTRAINT overtime_requests_attachments_check
      CHECK (cardinality(attachments) <= 3);
  END IF;
END $$;
```

Di `http_files.go`, tambahkan dua rute tepat di bawah `r("GET /api/hris/leaves/attachment/{path...}", …)`:

```go
		r("POST /api/hris/overtime/attachments", h.withActor(h.uploadOvertimeAttachment)),
		r("GET /api/hris/overtime/attachment/{path...}", h.withActor(h.overtimeAttachment)),
```

Ubah isi `leaveAttachment` agar memakai helper bersama, lalu tambahkan handler lembur:

```go
// GET /api/hris/leaves/attachment/leave-attachments/<employeeId>/<file>
// The employee's direct manager reads it too: Persetujuan Tim links the file
// from the queue (EPIC-063). A reporting_to pointing at oneself grants nothing.
func (h handlers) leaveAttachment(w http.ResponseWriter, r *http.Request, a *Actor) error {
	return h.managerReadableFile(w, r, a, "/api/hris/leaves/attachment/", "leave-attachments")
}

// managerReadableFile is ownedFile that the owner's direct manager reads too.
func (h handlers) managerReadableFile(w http.ResponseWriter, r *http.Request, a *Actor, prefix, folder string) error {
	if !a.IsHR && a.EmployeeID != nil {
		if segments := privateSegments(r, prefix, folder, 3); segments != nil && segments[1] != *a.EmployeeID {
			reports, err := h.svc.repo.SubordinateIDs(r.Context(), *a.EmployeeID)
			if err != nil {
				return err
			}
			if slices.Contains(reports, segments[1]) {
				return h.svc.servePrivate(w, strings.Join(segments, "/"), "")
			}
		}
	}
	return h.ownedFile(w, r, a, prefix, folder)
}

/* ── Overtime documentation (EPIC-066) ───────────────────────────────── */

const (
	overtimeAttachmentFolder = "overtime-attachments"
	maxOvertimeAttachments   = 3
)

// POST /api/hris/overtime/attachments (multipart "file"): one PDF/JPG/PNG/WebP
// of at most 5 MB for one's own overtime input.
func (h handlers) uploadOvertimeAttachment(w http.ResponseWriter, r *http.Request, a *Actor) error {
	const tooLarge = "Ukuran dokumentasi maksimal 5 MB"
	if a.EmployeeID == nil {
		return httpx.Forbidden("Akun ini tidak terhubung ke data karyawan")
	}
	form, err := storage.ReadForm(r, maxAttachmentBytes+64<<10)
	var file *storage.File
	switch {
	case errors.Is(err, storage.ErrBodyTooLarge):
		return httpx.BadRequest(tooLarge)
	case err == nil:
		file = form.File("file")
	case !isURLEncoded(r):
		return httpx.BadRequest("Form data tidak valid")
	}
	if file == nil {
		return httpx.BadRequest("File tidak ditemukan")
	}
	if file.Size() > maxAttachmentBytes {
		return httpx.BadRequest(tooLarge)
	}
	path, err := h.svc.files.SavePrivateDocument(file.Data, overtimeAttachmentFolder+"/"+*a.EmployeeID)
	if err != nil {
		return httpx.BadRequest(err.Error())
	}
	return ok(w, dataMsg(obj("path", path), "Dokumentasi tersimpan"))
}

// GET /api/hris/overtime/attachment/overtime-attachments/<employeeId>/<file>:
// the owner, their direct manager and HR.
func (h handlers) overtimeAttachment(w http.ResponseWriter, r *http.Request, a *Actor) error {
	return h.managerReadableFile(w, r, a, "/api/hris/overtime/attachment/", overtimeAttachmentFolder)
}
```

Pastikan `errors`, `storage`, `slices`, dan `strings` ada di blok import `http_files.go` (sebagian besar sudah dipakai berkas itu).

Lalu regenerasi manifest rute dan tambahkan prefix Go-only:

```bash
cd backend && $HOME/.local/go/bin/go run ./cmd/api -routes > ../frontend/src/lib/go-routes.generated.json
```

Di `frontend/src/lib/backend-routes.test.ts`, pada daftar `goNativePrefixes`, ubah `"/api/hris/overtime/day-type", "/api/hris/overtime/{id}",` menjadi:

```ts
      "/api/hris/overtime/day-type", "/api/hris/overtime/{id}", "/api/hris/overtime/attachments", "/api/hris/overtime/attachment",
```

- [ ] **Step 4: Run test to verify it passes**

Run:
- `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test -p 1 ./internal/modules/hris/ -run 'TestOvertimeAttachmentUploadAndRead|TestLeave'`
- `cd frontend && npx vitest run src/lib/backend-routes.test.ts`

Expected: PASS untuk keduanya. Migrasi diterapkan otomatis oleh harness DB test. Bila tidak, jalankan dulu `psql "$TEST_DATABASE_URL" -f backend/database/migrations/deltas/20261026090000_overtime_input_after.sql`.

- [ ] **Step 5: Commit**

```bash
git add backend/database/migrations/deltas/20261026090000_overtime_input_after.sql backend/internal/modules/hris/http_files.go \
  backend/internal/modules/hris/overtime_attachment_integration_test.go frontend/src/lib/go-routes.generated.json frontend/src/lib/backend-routes.test.ts
git commit -m "feat(hris): unggah dan baca dokumentasi lembur (EPIC-066)"
```

### Task 3: Input lembur setelah selesai (`CreateOvertime`)

**Files:**
- Modify: `backend/internal/modules/hris/overtime.go`: `overtimeInput`, `overtimeTooLate` → konstanta baru, `CreateOvertime`.
- Modify: `backend/internal/modules/hris/http_schedules.go`: `createOvertime`.
- Modify: `backend/internal/modules/hris/overtime_approval_integration_test.go`, `overtime_correction_integration_test.go`, `overtime_min_integration_test.go`, `team_approvals_integration_test.go`, `workforce_integration_test.go`. Setiap lembur **milik sendiri** di file-file ini dipindah ke tanggal yang sudah lewat dan diberi lampiran.
- Test: `backend/internal/modules/hris/overtime_input_integration_test.go`

**Interfaces:**
- Consumes:
  - `domain.OvertimeNotFinished`, `domain.OvertimeEndsAt`, `domain.PaidOvertimeHours`, `domain.OvertimeGrossMinutes` (Task 1)
  - `overtimeAttachmentFolder`, `maxOvertimeAttachments`, `h.otUpload`, `otPDF` (Task 2)
- Produces:
  - Body `POST /api/hris/overtime` menerima `break_minutes` (int, opsional, default 0) dan `attachments` (array path).
  - Untuk input milik sendiri, `reason` opsional (kosong disimpan `""`).
  - Helper test `func (h *harness) otSelfBody(s *testutil.Staff, date, start, end string) map[string]any`.

- [ ] **Step 1: Write the failing test**

```go
package hris

import (
	"context"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

// otSelfBody is a self-submitted overtime body with one uploaded PDF (EPIC-066).
// fixedNow = Senin 05-10-2026 10:00 WIB: pakai tanggal/jam yang sudah lewat.
func (h *harness) otSelfBody(s *testutil.Staff, date, start, end string) map[string]any {
	h.t.Helper()
	up := h.otUpload(s, "bukti.pdf", otPDF)
	if up.status != 200 {
		h.t.Fatalf("upload = %s", up.raw)
	}
	return map[string]any{"date": date, "start_time": start, "end_time": end,
		"attachments": []string{up.data()["path"].(string)}}
}

func TestOvertimeInputAfterFinished(t *testing.T) {
	h, _ := newFileHarness(t)
	staff, own := h.staff("pos", nil, true, emp{name: "Crew Input"})
	other, _ := h.staff("pos", nil, true, emp{name: "Crew Lain"})
	post := func(body map[string]any) call { return h.do(&staff, "POST", "/api/hris/overtime", body) }

	// Belum selesai: hari ini 09:00–11:00 (sekarang 10:00 WIB).
	expect(t, post(h.otSelfBody(&staff, "2026-10-05", "09:00", "11:00")), 400,
		"Lembur baru bisa diinput setelah selesai (05-10-2026 11:00 WIB)")
	// Lewat tengah malam 04→05 Okt, selesai 02:00: sudah boleh.
	night := h.otSelfBody(&staff, "2026-10-04", "22:00", "02:00")
	night["break_minutes"] = 60
	ok := post(night)
	expect(t, ok, 200, "")
	if d := ok.data(); d["hours"] != "3.00" || d["break_minutes"] != 60.0 || d["status"] != "pending" || d["reason"] != "" {
		t.Fatalf("lembur malam = %s", ok.raw)
	}

	noFile := map[string]any{"date": "2026-10-02", "start_time": "17:00", "end_time": "19:00"}
	expect(t, post(noFile), 400, "Lampirkan minimal 1 dokumentasi lembur")
	tooShort := h.otSelfBody(&staff, "2026-10-02", "17:00", "18:00")
	tooShort["break_minutes"] = 30
	expect(t, post(tooShort), 400, "Durasi lembur minimal 1 jam setelah dikurangi istirahat")
	badBreak := h.otSelfBody(&staff, "2026-10-02", "17:00", "19:00")
	badBreak["break_minutes"] = 721
	expect(t, post(badBreak), 400, "Validation failed")

	foreign := h.otSelfBody(&other, "2026-10-02", "17:00", "19:00") // path milik karyawan lain
	expect(t, post(foreign), 400, "Dokumentasi lembur tidak valid — unggah ulang filenya")
	forged := map[string]any{"date": "2026-10-02", "start_time": "17:00", "end_time": "19:00",
		"attachments": []string{"overtime-attachments/" + own + "/../../x.pdf"}}
	expect(t, post(forged), 400, "Dokumentasi lembur tidak valid — unggah ulang filenya")
	four := h.otSelfBody(&staff, "2026-10-02", "17:00", "19:00")
	paths := four["attachments"].([]string)
	four["attachments"] = []string{paths[0], paths[0], paths[0], paths[0]}
	expect(t, post(four), 400, "Maksimal 3 dokumentasi lembur")

	// Periode absensi yang sudah di-closing → 409. Materialisasi dulu periodenya
	// (periode dibuat saat pertama dibutuhkan), lalu pastikan tertutup.
	_ = h.svc.ensureAttendanceOpen(context.Background(), "2026-09-15")
	h.exec(`UPDATE hris.attendance_periods SET status = 'closed', range_frozen = true
		WHERE '2026-09-15' BETWEEN start_date AND end_date`)
	closed := post(h.otSelfBody(&staff, "2026-09-15", "17:00", "19:00"))
	if closed.status != 409 {
		t.Fatalf("periode tutup = %s", closed.raw)
	}
}
```

Untuk tes yang sudah ada, ganti setiap `h.do(&<karyawan>, "POST", "/api/hris/overtime", otBody(D, S, E))` yang membuat lembur **untuk dirinya sendiri** menjadi `h.do(&<karyawan>, "POST", "/api/hris/overtime", h.otSelfBody(&<karyawan>, D', S, E))`. Ketentuannya:
- `D'` adalah tanggal yang sudah lewat relatif `fixedNow` (Senin 05-10-2026 10:00 WIB) **dan masih di periode Oktober yang terbuka**. Periode September di DB test sudah tertutup otomatis (closing 03-10 < hari ini), jadi tanggal September akan menjawab 409. Urutan pengganti:
  - `2026-10-06 → 2026-10-01` (Kam)
  - `2026-10-07 → 2026-10-02` (Jum)
  - `2026-10-08 → 2026-10-05` dengan jam `06:00`–`08:00` (Sen, sudah selesai sebelum 10:00)
  - `2026-10-09 → 2026-10-03` (Sab). Pakai ini hanya bila tes itu tidak memeriksa jenis Weekday; Sabtu tanpa shift disarankan `rest_day`.

  Assertion yang memeriksa tanggal atau label hari di WA ikut disesuaikan. Contoh: `"Sel, 06 Okt 2026"` menjadi `"Kam, 01 Okt 2026"`.
- Harness tes-tes itu diganti `h, _ := newFileHarness(t)` supaya file tersimpan di direktori sementara.
- Penugasan HR (body dengan `employee_id` karyawan lain) **tetap** memakai `otBody` dan tanggal masa depan.
- `TestOvertimeMustBeRequestedBeforeStart` dihapus, karena perilaku itu diganti `TestOvertimeInputAfterFinished`. Bagian di dalamnya yang menguji penugasan HR dipindahkan ke tes baru `TestOvertimeHRAssignmentStillBeforeStart` dengan isi yang sama.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test -p 1 ./internal/modules/hris/ -run TestOvertimeInputAfterFinished`
Expected: FAIL. Input 05-10 09:00–11:00 menjawab `Lembur wajib diajukan sebelum dimulai — jam mulai sudah lewat` atau `Alasan minimal 5 karakter`, bukan pesan baru.

- [ ] **Step 3: Write minimal implementation**

Di `overtime.go`, ganti konstanta `overtimeTooLate` dan perluas `overtimeInput`:

```go
// overtimeTooLate is the refusal of an HR assignment whose start has passed
// (EPIC-063 R3); one's own overtime is input after it ends (EPIC-066).
const overtimeTooLate = "Lembur wajib diajukan sebelum dimulai — jam mulai sudah lewat"

// overtimeAttachmentRe is a stored overtime documentation path (EPIC-066).
var overtimeAttachmentRe = regexp.MustCompile(`^overtime-attachments/([0-9a-f-]{36})/[0-9]+-[0-9a-f]{16}\.(pdf|jpg|png|webp)$`)

type overtimeInput struct {
	EmployeeID                       *string
	Date, StartTime, EndTime, Reason string
	DayType                          *string // regular (Weekday) | rest_day (Weekend / Libur), K-13; nil = saran jadwal
	BreakMinutes                     int
	Attachments                      []string
}
```

Di `CreateOvertime`, ganti blok `if isSelf && domain.OvertimeStartsInPast(...) { return nil, httpx.BadRequest(overtimeTooLate) }` dengan:

```go
	if isSelf {
		if domain.OvertimeNotFinished(in.Date, in.StartTime, in.EndTime, s.now()) {
			ends, _ := domain.OvertimeEndsAt(in.Date, in.StartTime, in.EndTime)
			return nil, httpx.BadRequest("Lembur baru bisa diinput setelah selesai (" + ends.Format("02-01-2006 15:04") + " WIB)")
		}
		if err := s.ensureAttendanceOpen(ctx, in.Date); err != nil {
			return nil, err
		}
		if err := s.checkOvertimeAttachments(*targetID, in.Attachments); err != nil {
			return nil, err
		}
	} else {
		if domain.OvertimeStartsInPast(in.Date, in.StartTime, s.now()) {
			return nil, httpx.BadRequest(overtimeTooLate)
		}
		if len(domain.JSTrim(in.Reason)) < 5 {
			return nil, httpx.BadRequest("Alasan minimal 5 karakter")
		}
		in.BreakMinutes, in.Attachments = 0, nil
	}
```

Lalu ganti hitungan jam dan validasi durasi (blok `hours := domain.OvertimeHoursFromTimes(...)` sampai `Durasi lembur minimal 1 jam`):

```go
	if domain.OvertimeGrossMinutes(in.StartTime, in.EndTime) > 12*60 {
		return nil, httpx.BadRequest("Durasi lembur maksimal 12 jam")
	}
	hours := domain.PaidOvertimeHours(in.StartTime, in.EndTime, in.BreakMinutes)
	if hours < 1 {
		if in.BreakMinutes > 0 {
			return nil, httpx.BadRequest("Durasi lembur minimal 1 jam setelah dikurangi istirahat")
		}
		return nil, httpx.BadRequest("Durasi lembur minimal 1 jam")
	}
```

Tambahkan kolom saat menyusun `fields` (setelah `f.add("hours", hours)`):

```go
	f.add("break_minutes", in.BreakMinutes)
	f.add("attachments", nonNilStrings(in.Attachments))
```

dan helper di akhir `overtime.go`:

```go
// checkOvertimeAttachments: 1–3 paths uploaded by the owner and still on disk.
func (s *Service) checkOvertimeAttachments(ownerID string, paths []string) error {
	if len(paths) == 0 {
		return httpx.BadRequest("Lampirkan minimal 1 dokumentasi lembur")
	}
	if len(paths) > maxOvertimeAttachments {
		return httpx.BadRequest("Maksimal 3 dokumentasi lembur")
	}
	const invalid = "Dokumentasi lembur tidak valid — unggah ulang filenya"
	for _, p := range paths {
		m := overtimeAttachmentRe.FindStringSubmatch(p)
		if m == nil || m[1] != ownerID {
			return httpx.BadRequest(invalid)
		}
		if _, _, err := s.files.ReadPrivate(p); err != nil {
			return httpx.BadRequest(invalid)
		}
	}
	return nil
}

func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}
```

Tambahkan `"regexp"` ke import `overtime.go` bila belum ada. Payload outbox `contracts.OvertimeRequested` tidak berubah, dan `Hours` sudah jam dibayar.

Di `http_schedules.go` `createOvertime`, ganti baris `in.Reason = …` dan tambahkan dua field:

```go
	in.Reason = strOr(f.Str("reason", optional, validate.StrOpts{Trim: true}))
	if b := f.Int("break_minutes", optional, validate.NumOpts{Min: validate.Bound(0), Max: validate.Bound(720)}); b != nil {
		in.BreakMinutes = *b
	}
	in.Attachments = f.Strings("attachments", optional, maxOvertimeAttachments+1, validate.StrOpts{})
```

Batas `maxOvertimeAttachments+1` pada `Strings` membuat array berisi 4 lolos parser supaya pesannya `Maksimal 3 dokumentasi lembur`, bukan `Validation failed`. Bila `Strings` memotong atau menolak array yang lebih panjang dari batas, naikkan batasnya ke 10 dan biarkan `checkOvertimeAttachments` yang menolak.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test -p 1 ./internal/modules/hris/ -run 'Overtime|TeamApproval|Workforce' 2>&1 | tail -20`
Expected: PASS (`ok nuhabit/backend/internal/modules/hris`)

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/hris/overtime.go backend/internal/modules/hris/http_schedules.go backend/internal/modules/hris/*_integration_test.go
git commit -m "feat(hris): lembur sendiri diinput setelah selesai dengan istirahat dan dokumentasi (EPIC-066)"
```

### Task 4: Atasan langsung mengoreksi jam saat menyetujui; koreksi HR dibatasi

**Files:**
- Modify: `backend/internal/modules/hris/overtime.go`: `DecideOvertime`, signature dan isi.
- Modify: `backend/internal/modules/hris/http_schedules.go`: `decideOvertime`.
- Modify: `backend/internal/modules/hris/overtime_correction.go`: `CorrectOvertime` memakai `domain.CanCorrectOvertimeTimes` untuk `source = employee` dan menghitung jam dengan istirahat.
- Modify: `backend/internal/modules/hris/approvals.go`: `TeamPendingOvertime` ikut memilih `o.break_minutes, o.attachments`.
- Test: `backend/internal/modules/hris/overtime_manager_correction_integration_test.go`

**Interfaces:**
- Consumes: `domain.CanCorrectOvertimeTimes`, `domain.PaidOvertimeHours`, `domain.OvertimeGrossMinutes`, `domain.OvertimeCorrectManagerOnly` (Task 1); `h.otSelfBody` (Task 3).
- Produces:
  - Body `POST /api/hris/overtime/decide` menerima `start_time`, `end_time` (HH:MM), dan `break_minutes` (opsional, hanya untuk `action = approve`).
  - `type overtimeTimes struct { Start, End *string; Break *int }`
  - `func (s *Service) DecideOvertime(ctx context.Context, a *Actor, id, action string, rejection, dayType *string, times overtimeTimes, meta audit.Entry) (*Row, error)`

- [ ] **Step 1: Write the failing test**

```go
package hris

import "testing"

func TestOvertimeManagerCorrectsTimesOnApprove(t *testing.T) {
	h, _ := newFileHarness(t)
	hr, _ := h.staff("hrd", []string{menuKepegawaian, "hris.kepegawaian.overtime"}, true)
	boss, bossID := h.staff("pos", nil, true, emp{name: "Atasan Koreksi"})
	staff, _ := h.staff("pos", nil, true, emp{name: "Crew Dikoreksi", boss: &bossID})
	lone, _ := h.staff("pos", nil, true, emp{name: "Crew Tanpa Atasan"})

	created := h.do(&staff, "POST", "/api/hris/overtime", h.otSelfBody(&staff, "2026-10-01", "17:00", "21:00"))
	expect(t, created, 200, "")
	id := created.data()["id"].(string)
	decide := map[string]any{"overtime_id": id, "action": "approve", "end_time": "20:00", "break_minutes": 30}

	expect(t, h.do(&hr, "POST", "/api/hris/overtime/decide", decide), 403, "Koreksi jam lembur hanya oleh atasan langsung")
	expect(t, h.do(&hr, "PATCH", "/api/hris/overtime/"+id, map[string]any{"end_time": "20:00", "reason": "Sesuai absensi"}),
		403, "Koreksi jam lembur hanya oleh atasan langsung")
	bad := map[string]any{"overtime_id": id, "action": "approve", "end_time": "17:30"}
	expect(t, h.do(&boss, "POST", "/api/hris/overtime/decide", bad), 400, "Durasi lembur minimal 1 jam")

	ok := h.do(&boss, "POST", "/api/hris/overtime/decide", decide)
	expect(t, ok, 200, "")
	if d := ok.data(); d["status"] != "approved" || d["hours"] != "2.50" || d["break_minutes"] != 30.0 ||
		d["end_time"] != "20:00:00" || d["corrected_by_name"] != "Atasan Koreksi" {
		t.Fatalf("approve + koreksi = %s", ok.raw)
	}
	if before := h.scalar(`SELECT before->>'end_time' FROM audit.audit_log
		WHERE entity_id = $1 AND action = 'overtime.approve'`, id); before != "21:00" {
		t.Fatalf("audit before end_time = %q", before)
	}

	// Karyawan tanpa atasan: HR boleh mengoreksi.
	solo := h.do(&lone, "POST", "/api/hris/overtime", h.otSelfBody(&lone, "2026-10-02", "17:00", "20:00"))
	expect(t, solo, 200, "")
	byHR := h.do(&hr, "POST", "/api/hris/overtime/decide",
		map[string]any{"overtime_id": solo.data()["id"], "action": "approve", "break_minutes": 60})
	expect(t, byHR, 200, "")
	if byHR.data()["hours"] != "2.00" {
		t.Fatalf("HR koreksi = %s", byHR.raw)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test -p 1 ./internal/modules/hris/ -run TestOvertimeManagerCorrectsTimesOnApprove`
Expected: FAIL. HR approve dengan koreksi menjawab 200, bukan 403, karena field jam diabaikan.

- [ ] **Step 3: Write minimal implementation**

Di `overtime.go`, tambahkan tipe di atas `DecideOvertime`, ubah signature-nya, dan sisipkan blok koreksi tepat setelah blok `day_type` (sebelum `decidedAs := ""`):

```go
// overtimeTimes is the direct manager's correction while approving (EPIC-066).
type overtimeTimes struct {
	Start, End *string
	Break      *int
}

func (t overtimeTimes) any() bool { return t.Start != nil || t.End != nil || t.Break != nil }
```

```go
	if times.any() {
		if action != "approve" || ot.Str("source") != "employee" {
			return nil, httpx.BadRequest("Koreksi jam hanya saat menyetujui input lembur karyawan")
		}
		if allowed, msg := domain.CanCorrectOvertimeTimes(a.EmployeeID, a.IsHRD, ot.Str("employee_id"), reportingTo); !allowed {
			return nil, httpx.Forbidden(msg)
		}
		was := struct {
			start, end string
			brk        int
		}{overtimeClock(ot.Str("start_time")), overtimeClock(ot.Str("end_time")), int(ot.Num("break_minutes"))}
		start, end, brk := was.start, was.end, was.brk
		if times.Start != nil {
			start = *times.Start
		}
		if times.End != nil {
			end = *times.End
		}
		if times.Break != nil {
			brk = *times.Break
		}
		if start == end {
			return nil, httpx.BadRequest("Jam mulai dan selesai tidak boleh sama")
		}
		if domain.OvertimeGrossMinutes(start, end) > 12*60 {
			return nil, httpx.BadRequest("Durasi lembur maksimal 12 jam")
		}
		hours := domain.PaidOvertimeHours(start, end, brk)
		if hours < 1 {
			return nil, httpx.BadRequest("Durasi lembur minimal 1 jam")
		}
		if start != was.start || end != was.end || brk != was.brk {
			f.add("start_time", start)
			f.add("end_time", end)
			f.add("break_minutes", brk)
			f.add("hours", hours)
			f.add("corrected_at", now)
			f.add("corrected_by_name", actorName(a))
			if auditBefore == nil {
				auditBefore = map[string]any{}
			}
			auditBefore["start_time"], auditBefore["end_time"], auditBefore["break_minutes"] = was.start, was.end, was.brk
			auditBefore["hours"] = ot.Get("hours")
			auditAfter["start_time"], auditAfter["end_time"], auditAfter["break_minutes"], auditAfter["hours"] = start, end, brk, hours
		}
	}
```

Bila `Row` tidak punya method `Num`, baca `break_minutes` lewat helper angka yang dipakai berkas yang sama untuk kolom numerik. Cari dengan `grep -n 'func (r \*Row)' backend/internal/modules/hris/*.go`. Keluarannya harus berupa `int` dari kolom integer.

Di `http_schedules.go` `decideOvertime`, baca field tambahan dan teruskan:

```go
	var times overtimeTimes
	times.Start = f.Str("start_time", optional, validate.StrOpts{Check: regexOf(overtimeTimeRe)})
	times.End = f.Str("end_time", optional, validate.StrOpts{Check: regexOf(overtimeTimeRe)})
	times.Break = f.Int("break_minutes", optional, validate.NumOpts{Min: validate.Bound(0), Max: validate.Bound(720)})
	// … setelah formErr:
	out, err := h.svc.DecideOvertime(r.Context(), a, *id, *action, rejection, dayType, times, auditMeta(r))
```

Pemanggil lain `DecideOvertime` (cari dengan `grep -rn 'DecideOvertime(' backend/internal`) memakai `overtimeTimes{}`.

Di `overtime_correction.go` `CorrectOvertime`, tepat setelah pengecekan `overtimeCorrectSelf`, tambahkan:

```go
		if ot.Str("source") == "employee" {
			var reportingTo *string
			if emp := ot.Child("employee"); emp != nil {
				reportingTo = emp.StrPtr("reporting_to")
			}
			if allowed, msg := domain.CanCorrectOvertimeTimes(a.EmployeeID, a.IsHRD, ot.Str("employee_id"), reportingTo); !allowed {
				return httpx.Forbidden(msg)
			}
		}
```

Bila `OvertimeForUpdate` tidak menyertakan embed `employee`, ambil `reporting_to` lewat `r.OvertimeTarget(ctx, ot.Str("employee_id"))` dengan kolom yang sama. Di fungsi yang sama, ganti `hours := domain.OvertimeHoursFromTimes(start, end)` dengan `hours := domain.PaidOvertimeHours(start, end, int(ot.Num("break_minutes")))`, dan ganti batas 12 jam dengan `domain.OvertimeGrossMinutes(start, end) > 12*60`.

Di `approvals.go` `TeamPendingOvertime`, tambahkan `o.break_minutes, o.attachments,` setelah `o.hours,` pada SELECT.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test -p 1 ./internal/modules/hris/ -run 'Overtime|TeamApproval' 2>&1 | tail -20`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/modules/hris/overtime.go backend/internal/modules/hris/http_schedules.go backend/internal/modules/hris/overtime_correction.go \
  backend/internal/modules/hris/approvals.go backend/internal/modules/hris/overtime_manager_correction_integration_test.go
git commit -m "feat(hris): atasan langsung mengoreksi jam lembur saat menyetujui (EPIC-066)"
```

### Task 5: Aturan input lembur di frontend (TS)

**Files:**
- Modify: `frontend/src/lib/hris/overtime-rules.ts`
- Test: `frontend/src/lib/hris/overtime-input.test.ts`

**Interfaces:**
- Produces (dari `@/lib/hris/overtime-rules`):
  - `overtimeNotFinished(date: string, start: string, end: string, now?: Date): boolean`
  - `overtimeEndLabel(date: string, start: string, end: string): string`, contoh `"05-10-2026 11:00"`
  - `paidOvertimeHours(start: string, end: string, breakMinutes: number): number`
  - `OVERTIME_ATTACHMENT_ACCEPT = "application/pdf,image/jpeg,image/png,image/webp"`
  - `MAX_OVERTIME_ATTACHMENTS = 3`
  - `MAX_OVERTIME_ATTACHMENT_BYTES = 5 * 1024 * 1024`

- [ ] **Step 1: Write the failing test**

```ts
import { describe, expect, it } from "vitest";
import { overtimeEndLabel, overtimeNotFinished, paidOvertimeHours } from "./overtime-rules";

const wib = (s: string) => new Date(`${s}:00+07:00`);

describe("input lembur setelah selesai (EPIC-066)", () => {
  it("baru boleh setelah jam selesai, termasuk lewat tengah malam", () => {
    expect(overtimeNotFinished("2026-10-21", "17:00", "19:00", wib("2026-10-21T18:59"))).toBe(true);
    expect(overtimeNotFinished("2026-10-21", "17:00", "19:00", wib("2026-10-21T19:00"))).toBe(false);
    expect(overtimeNotFinished("2026-10-21", "22:00", "02:00", wib("2026-10-22T01:00"))).toBe(true);
    expect(overtimeNotFinished("2026-10-21", "22:00", "02:00", wib("2026-10-22T02:00"))).toBe(false);
    expect(overtimeEndLabel("2026-10-21", "22:00", "02:00")).toBe("22-10-2026 02:00");
  });
  it("jam dibayar = durasi − istirahat, tidak negatif", () => {
    expect(paidOvertimeHours("17:00", "20:00", 30)).toBe(2.5);
    expect(paidOvertimeHours("22:00", "02:00", 60)).toBe(3);
    expect(paidOvertimeHours("17:00", "18:00", 90)).toBe(0);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && npx vitest run src/lib/hris/overtime-input.test.ts`
Expected: FAIL with `overtimeNotFinished is not a function` (atau `has no exported member`)

- [ ] **Step 3: Write minimal implementation**

Tambahkan di akhir `frontend/src/lib/hris/overtime-rules.ts`:

```ts
/* ── Input lembur setelah selesai (EPIC-066) ─────────────────────────── */

export const OVERTIME_ATTACHMENT_ACCEPT = "application/pdf,image/jpeg,image/png,image/webp";
export const MAX_OVERTIME_ATTACHMENTS = 3;
export const MAX_OVERTIME_ATTACHMENT_BYTES = 5 * 1024 * 1024;

function clockMinutes(s: string): number | null {
  const m = /^(\d{2}):(\d{2})/.exec(s);
  if (!m) return null;
  const h = Number(m[1]);
  const min = Number(m[2]);
  return h <= 23 && min <= 59 ? h * 60 + min : null;
}

/** Durasi kotor (menit), lewat tengah malam bila selesai tidak setelah mulai. */
function grossMinutes(start: string, end: string): number {
  const s = clockMinutes(start);
  const e = clockMinutes(end);
  if (s === null || e === null) return 0;
  return e <= s ? e + 1440 - s : e - s;
}

/** Akhir lembur di jam dinding WIB; null bila input tidak valid. */
function overtimeEnd(date: string, start: string, end: string): Date | null {
  const s = clockMinutes(start);
  const gross = grossMinutes(start, end);
  if (!/^\d{4}-\d{2}-\d{2}$/.test(date) || s === null || gross === 0) return null;
  const base = new Date(`${date}T00:00:00+07:00`);
  return Number.isNaN(base.getTime()) ? null : new Date(base.getTime() + (s + gross) * 60_000);
}

/** Server: Lembur baru bisa diinput setelah selesai (EPIC-066). */
export function overtimeNotFinished(date: string, start: string, end: string, now: Date = new Date()): boolean {
  const ends = overtimeEnd(date, start, end);
  return ends !== null && now.getTime() < ends.getTime();
}

/** "DD-MM-YYYY HH:MM" (WIB) akhir lembur, seperti pesan server. */
export function overtimeEndLabel(date: string, start: string, end: string): string {
  const ends = overtimeEnd(date, start, end);
  if (!ends) return "";
  const wibTime = new Date(ends.getTime() + 7 * 3600_000);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${pad(wibTime.getUTCDate())}-${pad(wibTime.getUTCMonth() + 1)}-${wibTime.getUTCFullYear()} ${pad(wibTime.getUTCHours())}:${pad(wibTime.getUTCMinutes())}`;
}

/** Jam dibayar = (durasi − istirahat) / 60, 2 desimal, tidak negatif. */
export function paidOvertimeHours(start: string, end: string, breakMinutes: number): number {
  const minutes = Math.max(0, grossMinutes(start, end) - (Number.isFinite(breakMinutes) ? breakMinutes : 0));
  return Math.round((minutes / 60) * 100) / 100;
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend && npx vitest run src/lib/hris/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/hris/overtime-rules.ts frontend/src/lib/hris/overtime-input.test.ts
git commit -m "feat(hris): aturan input lembur setelah selesai di frontend (EPIC-066)"
```

### Task 6: Form ESS "Input Lembur"

**Files:**
- Create: `frontend/src/features/hris/ess/components/overtime-attachments-field.tsx`
- Modify: `frontend/src/features/hris/ess/components/lembur-page.tsx`
- Modify: `frontend/src/features/hris/ess/api.ts` (`submitOvertime`, `uploadOvertimeAttachment`) dan `frontend/src/features/hris/ess/types.ts` (`EssOvertimeForm`)
- Test: `frontend/src/features/hris/ess/components/lembur-page.test.tsx`; bila sudah ada, perbarui test lamanya.

**Interfaces:**
- Consumes:
  - `overtimeNotFinished`, `overtimeEndLabel`, `paidOvertimeHours`, `OVERTIME_ATTACHMENT_ACCEPT`, `MAX_OVERTIME_ATTACHMENTS`, `MAX_OVERTIME_ATTACHMENT_BYTES` (Task 5)
  - `POST /api/hris/overtime/attachments` (Task 2)
  - `POST /api/hris/overtime` dengan `break_minutes` dan `attachments` (Task 3)
- Produces:
  - `EssOvertimeForm` = `{ date; start_time; end_time; break_minutes: string; reason; day_type; attachments: string[] }`
  - `uploadOvertimeAttachment(file: File): Promise<string>` (path)
  - Komponen `OvertimeAttachmentsField({ value, onChange }: { value: string[]; onChange: (paths: string[]) => void })`

- [ ] **Step 1: Write the failing test**

```tsx
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const submit = vi.fn();
vi.mock("../queries", () => ({
  useEssMe: () => ({ data: { employee: { id: "e1" } }, isLoading: false }),
  useMyOvertime: () => ({ data: [] }),
  useOvertimeDayType: () => ({ data: null }),
}));
vi.mock("../mutations", () => ({
  useSubmitOvertime: () => ({ mutate: submit, isPending: false }),
  useDecideOvertime: () => ({ mutate: vi.fn(), isPending: false }),
}));
vi.mock("../api", () => ({ uploadOvertimeAttachment: vi.fn().mockResolvedValue("overtime-attachments/e1/1-abcdefabcdefabcd.pdf") }));
const toastError = vi.fn();
vi.mock("sonner", () => ({ toast: { error: (m: string) => toastError(m), success: vi.fn() } }));

import { EssLemburPage } from "./lembur-page";

afterEach(() => {
  cleanup();
  submit.mockReset();
  toastError.mockReset();
});

describe("ESS Input Lembur (EPIC-066)", () => {
  it("menolak lembur yang belum selesai dan tanpa dokumentasi; menampilkan jam dibayar", () => {
    vi.setSystemTime(new Date("2026-10-21T18:00:00+07:00"));
    render(<EssLemburPage />);
    fireEvent.click(screen.getByRole("button", { name: /Input Lembur/ }));
    fireEvent.change(screen.getByLabelText("Tanggal"), { target: { value: "2026-10-21" } });
    fireEvent.change(screen.getByLabelText("Jam Mulai"), { target: { value: "17:00" } });
    fireEvent.change(screen.getByLabelText("Jam Selesai"), { target: { value: "20:00" } });
    fireEvent.change(screen.getByLabelText("Istirahat (menit)"), { target: { value: "30" } });
    expect(screen.getByText(/2,5 jam dibayar/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Kirim" }));
    expect(toastError).toHaveBeenCalledWith("Lembur baru bisa diinput setelah selesai (21-10-2026 20:00 WIB)");
    vi.setSystemTime(new Date("2026-10-22T08:00:00+07:00"));
    fireEvent.click(screen.getByRole("button", { name: "Kirim" }));
    expect(toastError).toHaveBeenLastCalledWith("Lampirkan minimal 1 dokumentasi lembur");
    expect(submit).not.toHaveBeenCalled();
    vi.useRealTimers();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && npx vitest run src/features/hris/ess/components/lembur-page.test.tsx`
Expected: FAIL. Tombol `Input Lembur` dan label `Istirahat (menit)` belum ada.

- [ ] **Step 3: Write minimal implementation**

`frontend/src/features/hris/ess/api.ts`: tambahkan fungsi unggah. `submitOvertime` sudah meneruskan seluruh objek form; pastikan `break_minutes` dikirim sebagai angka:

```ts
/** Unggah 1 dokumentasi lembur (PDF/JPG/PNG/WebP ≤ 5 MB); kembalikan path privat. */
export async function uploadOvertimeAttachment(file: File): Promise<string> {
  const body = new FormData();
  body.append("file", file);
  const res = await fetch("/api/hris/overtime/attachments", { method: "POST", body });
  const json = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(json?.error || "Gagal mengunggah dokumentasi");
  return json.data.path as string;
}
```

Di `submitOvertime(form)`, kirim body `{ ...form, break_minutes: Number(form.break_minutes || 0) }`. Di `types.ts`, `EssOvertimeForm` menjadi:

```ts
export interface EssOvertimeForm {
  date: string;
  start_time: string;
  end_time: string;
  break_minutes: string;
  reason: string;
  day_type: OvertimeDayType;
  attachments: string[];
}
```

`overtime-attachments-field.tsx`:

```tsx
"use client";

import { useRef, useState } from "react";
import { Loader2, Paperclip, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  MAX_OVERTIME_ATTACHMENTS,
  MAX_OVERTIME_ATTACHMENT_BYTES,
  OVERTIME_ATTACHMENT_ACCEPT,
} from "@/lib/hris/overtime-rules";
import { uploadOvertimeAttachment } from "../api";

/** Dokumentasi lembur (EPIC-066): 1–3 file PDF/JPG/PNG/WebP ≤ 5 MB, diunggah satu per satu. */
export function OvertimeAttachmentsField({ value, onChange }: { value: string[]; onChange: (paths: string[]) => void }) {
  const input = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function add(files: FileList | null) {
    if (!files || files.length === 0) return;
    setError("");
    const room = MAX_OVERTIME_ATTACHMENTS - value.length;
    const picked = Array.from(files).slice(0, room);
    if (files.length > room) setError(`Maksimal ${MAX_OVERTIME_ATTACHMENTS} dokumentasi lembur`);
    const tooBig = picked.find((f) => f.size > MAX_OVERTIME_ATTACHMENT_BYTES);
    if (tooBig) {
      setError(`${tooBig.name}: ukuran dokumentasi maksimal 5 MB`);
      return;
    }
    setBusy(true);
    try {
      const paths: string[] = [];
      for (const f of picked) paths.push(await uploadOvertimeAttachment(f));
      onChange([...value, ...paths]);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Gagal mengunggah dokumentasi");
    } finally {
      setBusy(false);
      if (input.current) input.current.value = "";
    }
  }

  return (
    <div className="space-y-1.5">
      <span className="block text-xs font-medium text-gray-600">Dokumentasi (wajib, 1–3 file PDF/foto)</span>
      <ul className="space-y-1">
        {value.map((p, i) => (
          <li key={p} className="flex items-center justify-between rounded border border-gray-200 px-2 py-1 text-xs">
            <span className="truncate">Dokumentasi {i + 1} ({p.split(".").pop()?.toUpperCase()})</span>
            <button type="button" aria-label={`Hapus dokumentasi ${i + 1}`} onClick={() => onChange(value.filter((x) => x !== p))}>
              <X className="h-3.5 w-3.5" />
            </button>
          </li>
        ))}
      </ul>
      {value.length < MAX_OVERTIME_ATTACHMENTS && (
        <Button type="button" variant="outline" size="sm" disabled={busy} onClick={() => input.current?.click()}>
          {busy ? <Loader2 className="mr-1.5 h-4 w-4 animate-spin" /> : <Paperclip className="mr-1.5 h-4 w-4" />}
          Tambah dokumentasi
        </Button>
      )}
      <input
        ref={input}
        type="file"
        accept={OVERTIME_ATTACHMENT_ACCEPT}
        multiple
        className="hidden"
        aria-label="Pilih file dokumentasi lembur"
        onChange={(e) => void add(e.target.files)}
      />
      {error && <p role="alert" className="text-xs text-red-600">{error}</p>}
    </div>
  );
}
```

Perubahan di `lembur-page.tsx`:
1. Ganti import `OVERTIME_TOO_LATE` dan `overtimeStartsInPast` dengan `overtimeEndLabel`, `overtimeNotFinished`, `paidOvertimeHours`. Tambahkan import `OvertimeAttachmentsField`.
2. `EMPTY_FORM = { date: "", start_time: "", end_time: "", break_minutes: "0", reason: "", day_type: "regular", attachments: [] }`.
3. `handleSubmit` diganti seluruhnya:

```tsx
  function handleSubmit() {
    if (!form.date || !form.start_time || !form.end_time) {
      toast.error("Tanggal dan jam lembur wajib diisi");
      return;
    }
    if (overtimeNotFinished(form.date, form.start_time, form.end_time)) {
      toast.error(`Lembur baru bisa diinput setelah selesai (${overtimeEndLabel(form.date, form.start_time, form.end_time)} WIB)`);
      return;
    }
    const breakMinutes = Number(form.break_minutes || 0);
    if (!Number.isInteger(breakMinutes) || breakMinutes < 0 || breakMinutes > 720) {
      toast.error("Istirahat 0–720 menit");
      return;
    }
    if (paidOvertimeHours(form.start_time, form.end_time, breakMinutes) < 1) {
      toast.error("Durasi lembur minimal 1 jam setelah dikurangi istirahat");
      return;
    }
    if (form.attachments.length === 0) {
      toast.error("Lampirkan minimal 1 dokumentasi lembur");
      return;
    }
    submitMutation.mutate({ ...form, day_type: dayType }, {
      onSuccess: () => {
        toast.success("Lembur terkirim — menunggu persetujuan atasan");
        setDialog(false);
        setForm(EMPTY_FORM);
      },
      onError: (error) => toast.error(error.message || "Gagal mengirim lembur"),
    });
  }
```

4. Pada judul halaman dan tombol: teks `Ajukan Lembur` menjadi `Input Lembur`, dan judul dialog `Input Lembur`. Kalimat pengantar menjadi `Input lembur setelah selesai, atau konfirmasi penugasan lembur dari perusahaan`.
5. Di dalam dialog, setelah input `Jam Selesai`:
   - tambahkan field istirahat, ringkasan jam dibayar, dan field dokumentasi;
   - ubah label `Alasan` menjadi `Keterangan (opsional)`;
   - tombol kirim berlabel `Kirim`;
   - input tanggal diberi `max={todayWib()}`.

```tsx
              <div>
                <label htmlFor="ot-break" className="mb-1 block text-xs font-medium text-gray-600">Istirahat (menit)</label>
                <Input id="ot-break" type="number" min={0} max={720} step={5} value={form.break_minutes}
                  onChange={(e) => setForm((f) => ({ ...f, break_minutes: e.target.value }))} />
              </div>
              {form.start_time && form.end_time && (
                <p className="text-xs text-gray-600">
                  {paidOvertimeHours(form.start_time, form.end_time, Number(form.break_minutes || 0)).toLocaleString("id-ID")} jam dibayar
                </p>
              )}
              <OvertimeAttachmentsField value={form.attachments} onChange={(attachments) => setForm((f) => ({ ...f, attachments }))} />
```

6. Pada daftar riwayat (baris yang menampilkan `clockLabel(row.start_time)}–{clockLabel(row.end_time)`), tambahkan ` · istirahat {row.break_minutes} menit` bila `row.break_minutes > 0`. Lalu tambahkan tipe `break_minutes: number; attachments: string[]` pada tipe baris lembur ESS di `types.ts`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend && npx vitest run src/features/hris/ess && npx tsc --noEmit -p .`
Expected: PASS dan tsc tanpa error

- [ ] **Step 5: Commit**

```bash
git add frontend/src/features/hris/ess
git commit -m "feat(hris): form ESS Input Lembur dengan istirahat dan dokumentasi (EPIC-066)"
```

### Task 7: Persetujuan Tim & halaman Lembur HR

**Files:**
- Create: `frontend/src/features/hris/team-approvals/components/overtime-approve-dialog.tsx`
- Modify: `frontend/src/features/hris/team-approvals/components/persetujuan-tim-page.tsx`, `frontend/src/features/hris/team-approvals/api.ts`, `frontend/src/features/hris/team-approvals/types.ts`, `frontend/src/features/hris/team-approvals/mutations.ts`
- Modify: `frontend/src/features/hris/overtime/components/overtime-table.tsx`: tampilkan istirahat dan tautan dokumentasi; sembunyikan tombol koreksi untuk `source = employee` yang punya atasan.
- Test: `frontend/src/features/hris/team-approvals/components/overtime-approve-dialog.test.tsx`

**Interfaces:**
- Consumes: `POST /api/hris/overtime/decide` dengan `start_time`, `end_time`, `break_minutes` (Task 4); `paidOvertimeHours` (Task 5); `GET /api/hris/overtime/attachment/{path}` (Task 2).
- Produces:
  - `overtimeAttachmentHref(path: string): string` di `frontend/src/lib/hris/overtime-rules.ts`
  - `OvertimeApproveDialog({ row, open, onOpenChange, onApprove })`, dengan `onApprove(times: { start_time: string; end_time: string; break_minutes: number })`

- [ ] **Step 1: Write the failing test**

```tsx
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { OvertimeApproveDialog } from "./overtime-approve-dialog";

afterEach(cleanup);

const row = { id: "o1", date: "2026-10-21", start_time: "17:00:00", end_time: "21:00:00", break_minutes: 0,
  attachments: ["overtime-attachments/e1/1-abcdefabcdefabcd.pdf"], employee_name: "Crew A" };

describe("OvertimeApproveDialog (EPIC-066)", () => {
  it("atasan mengubah jam & istirahat lalu menyetujui; jam dibayar dihitung ulang", () => {
    const onApprove = vi.fn();
    render(<OvertimeApproveDialog row={row} open onOpenChange={() => {}} onApprove={onApprove} />);
    expect(screen.getByRole("link", { name: /Dokumentasi 1/ }).getAttribute("href")).toBe(
      "/api/hris/overtime/attachment/overtime-attachments/e1/1-abcdefabcdefabcd.pdf");
    fireEvent.change(screen.getByLabelText("Jam Selesai"), { target: { value: "20:00" } });
    fireEvent.change(screen.getByLabelText("Istirahat (menit)"), { target: { value: "30" } });
    expect(screen.getByText(/2,5 jam dibayar/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Setujui" }));
    expect(onApprove).toHaveBeenCalledWith({ start_time: "17:00", end_time: "20:00", break_minutes: 30 });
  });
  it("menolak jam dibayar < 1 jam tanpa memanggil onApprove", () => {
    const onApprove = vi.fn();
    render(<OvertimeApproveDialog row={row} open onOpenChange={() => {}} onApprove={onApprove} />);
    fireEvent.change(screen.getByLabelText("Jam Selesai"), { target: { value: "17:30" } });
    fireEvent.click(screen.getByRole("button", { name: "Setujui" }));
    expect(onApprove).not.toHaveBeenCalled();
    expect(screen.getByRole("alert").textContent).toBe("Durasi lembur minimal 1 jam");
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && npx vitest run src/features/hris/team-approvals`
Expected: FAIL with `Failed to resolve import "./overtime-approve-dialog"`

- [ ] **Step 3: Write minimal implementation**

Tambahkan di `frontend/src/lib/hris/overtime-rules.ts`:

```ts
/** Tautan dokumentasi lembur privat (pemilik, atasan langsung, HR). */
export function overtimeAttachmentHref(path: string): string {
  return `/api/hris/overtime/attachment/${path.split("/").map(encodeURIComponent).join("/")}`;
}
```

`overtime-approve-dialog.tsx`:

```tsx
"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { overtimeAttachmentHref, paidOvertimeHours } from "@/lib/hris/overtime-rules";

export interface OvertimeApproveRow {
  id: string;
  date: string;
  start_time: string;
  end_time: string;
  break_minutes: number;
  attachments: string[];
  employee_name: string;
}

export interface OvertimeApproveTimes {
  start_time: string;
  end_time: string;
  break_minutes: number;
}

const hhmm = (v: string) => v.slice(0, 5);

/** Atasan langsung menyetujui input lembur, boleh mengubah jam & istirahat dulu (EPIC-066). */
export function OvertimeApproveDialog({
  row,
  open,
  onOpenChange,
  onApprove,
}: {
  row: OvertimeApproveRow;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onApprove: (times: OvertimeApproveTimes) => void;
}) {
  const [start, setStart] = useState(hhmm(row.start_time));
  const [end, setEnd] = useState(hhmm(row.end_time));
  const [brk, setBrk] = useState(String(row.break_minutes ?? 0));
  const [error, setError] = useState("");
  const hours = paidOvertimeHours(start, end, Number(brk || 0));

  function approve() {
    const breakMinutes = Number(brk || 0);
    if (!Number.isInteger(breakMinutes) || breakMinutes < 0 || breakMinutes > 720) {
      setError("Istirahat 0–720 menit");
      return;
    }
    if (hours < 1) {
      setError("Durasi lembur minimal 1 jam");
      return;
    }
    setError("");
    onApprove({ start_time: start, end_time: end, break_minutes: breakMinutes });
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Setujui lembur {row.employee_name}</DialogTitle>
        </DialogHeader>
        <div className="space-y-3 text-sm">
          <ul className="space-y-1">
            {row.attachments.map((p, i) => (
              <li key={p}>
                <a className="text-blue-600 underline" href={overtimeAttachmentHref(p)} target="_blank" rel="noreferrer">
                  Dokumentasi {i + 1}
                </a>
              </li>
            ))}
          </ul>
          <div className="grid grid-cols-3 gap-2">
            <label className="text-xs font-medium text-gray-600">
              Jam Mulai
              <Input type="time" value={start} onChange={(e) => setStart(e.target.value)} />
            </label>
            <label className="text-xs font-medium text-gray-600">
              Jam Selesai
              <Input type="time" value={end} onChange={(e) => setEnd(e.target.value)} />
            </label>
            <label className="text-xs font-medium text-gray-600">
              Istirahat (menit)
              <Input type="number" min={0} max={720} step={5} value={brk} onChange={(e) => setBrk(e.target.value)} />
            </label>
          </div>
          <p className="text-xs text-gray-600">{hours.toLocaleString("id-ID")} jam dibayar</p>
          {error && <p role="alert" className="text-xs text-red-600">{error}</p>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>Batal</Button>
          <Button onClick={approve}>Setujui</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
```

Di team-approvals:
- **`types.ts`:** tambahkan `break_minutes: number; attachments: string[]` pada tipe baris lembur tim.
- **`api.ts`:** fungsi keputusan lembur menerima argumen opsional `times?: OvertimeApproveTimes` dan menyebarkannya ke body `POST /api/hris/overtime/decide`.
- **`mutations.ts`:** variabel mutasi keputusan lembur ditambah `times?`.
- **`persetujuan-tim-page.tsx`:** pada kartu Lembur, tombol `Setujui` tidak lagi langsung memutus. Tombol itu membuka `OvertimeApproveDialog` untuk baris tersebut; `onApprove` memanggil mutasi `approve` dengan `times`. Tampilkan juga ` · istirahat {row.break_minutes} menit` dan jumlah dokumentasi di baris ringkas. `Tolak` tetap seperti sekarang.

Di `overtime-table.tsx` (HR):
- tampilkan istirahat dan tautan `Dokumentasi n` (`overtimeAttachmentHref`) di kolom detail;
- tombol koreksi disembunyikan bila `row.source === "employee"` dan `row.employee?.reporting_to` terisi serta berbeda dari `row.employee_id`, karena koreksi milik atasan langsung.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend && npx vitest run src/features/hris/team-approvals src/features/hris/overtime && npx tsc --noEmit -p . && npx eslint src/features/hris/team-approvals src/features/hris/overtime src/features/hris/ess src/lib/hris`
Expected: PASS; tsc dan eslint tanpa error

- [ ] **Step 5: Commit**

```bash
git add frontend/src/features/hris/team-approvals frontend/src/features/hris/overtime frontend/src/lib/hris/overtime-rules.ts
git commit -m "feat(hris): atasan setujui dan koreksi jam lembur di Persetujuan Tim (EPIC-066)"
```

### Task 8: Dokumen acuan, registry epic, verifikasi menyeluruh

**Files:**
- Modify:
  - `docs/hris/acuan-mom-20261019/status-requirement.md` (baris AP-02 dan AP-04)
  - `docs/hris/acuan-mom-20261019/matriks-approval.md` (baris Overtime)
  - `docs/hris/acuan-mom-20261019/aturan-payroll.md` (baris Lembur)
  - `docs/epics/README.md` (baris EPIC-066)
  - `docs/epics/EPIC-066-input-lembur-setelah-selesai.md` (status dan Automation Log)

- [ ] **Step 1: Write the failing check**

Run: `grep -n "EPIC-066" docs/epics/README.md docs/hris/acuan-mom-20261019/status-requirement.md docs/hris/acuan-mom-20261019/matriks-approval.md`
Expected: tidak ada keluaran (belum dicatat)

- [ ] **Step 2: Update docs**

- **`status-requirement.md` baris AP-02:** kolom bukti diganti menjadi `Diganti EPIC-066 (22-10-2026, permintaan HR): lembur milik sendiri diinput setelah selesai selama periode absensi terbuka, istirahat (menit), 1–3 dokumentasi wajib; penugasan HR tetap sebelum mulai. Bukti: hris/overtime.go CreateOvertime, domain/overtime_input.go`.
- **`status-requirement.md` baris AP-04:** tambahkan `EPIC-066: koreksi jam lembur karyawan oleh atasan langsung (saat menyetujui); AdminHRIS hanya untuk karyawan tanpa atasan dan penugasan HR`.
- **`matriks-approval.md` baris Overtime:** ganti `Wajib diajukan sebelum lembur` menjadi `Diinput setelah lembur selesai dengan dokumentasi (EPIC-066); atasan langsung menyetujui/mengoreksi jam`.
- **`aturan-payroll.md` baris Lembur Hari Kerja Biasa:** tambahkan `Jam Lembur Dibayar = durasi − istirahat (EPIC-066), tetap dibatasi lembur absensi bila ada`.
- **`README.md`:** tambahkan baris `| [EPIC-066](./EPIC-066-input-lembur-setelah-selesai.md) | Input Lembur Setelah Selesai — Istirahat, Dokumentasi Wajib, Koreksi Atasan Langsung (`MODULE-HRIS`) | review | 2026-10-22 |`.
- **`EPIC-066`:** set `status: review` dan catat di Automation Log hasil test (Step 3) beserta langkah rilis:
  1. backup;
  2. migrasi `20261026090000_overtime_input_after.sql` (otomatis lewat `tools/deploy-paddydev.sh`);
  3. build ulang API & web;
  4. QA manusia: input setelah selesai, lampiran, atasan koreksi, slip lembur.

- [ ] **Step 3: Run full verification**

Run:
- `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test -p 1 ./... 2>&1 | grep -E '^--- FAIL' | sed 's/ (.*//' | sort -u`
- `cd frontend && npx vitest run && npx tsc --noEmit -p .`

Expected:
- Go: daftar `--- FAIL` identik dengan baseline main (27 nama, termasuk `TestOrderAlerts` dan `TestResendingSameRoleKeepsIamRoles`), tanpa nama baru.
- Vitest: lulus semua; tsc tanpa error.

- [ ] **Step 4: Commit**

```bash
git add docs/hris/acuan-mom-20261019 docs/epics/README.md docs/epics/EPIC-066-input-lembur-setelah-selesai.md
git commit -m "docs: EPIC-066 input lembur setelah selesai — acuan, registry, log"
```

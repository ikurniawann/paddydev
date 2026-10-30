package hris

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/storage"
	"nuhabit/backend/internal/platform/validate"
)

// Files of the HRIS routes, kept in the private storage Next shares:
// attendance selfies (attendance/<employee>/), leave attachments
// (leave-attachments/<employee>/), announcement covers (announcements/) and
// signed contract scans (contract-signed/<employee>/).

// CompanyProfile reads the company identity printed on HR documents
// (configuration context).
type CompanyProfile interface {
	// GetMany is getSettings: configuration.app_settings values by key, nil
	// when missing or NULL.
	GetMany(ctx context.Context, q database.Querier, keys []string) (map[string]*string, error)
	// FirstCompanyName is loadCompanyName's row: the oldest company's name,
	// nil when there is none.
	FirstCompanyName(ctx context.Context, q database.Querier) (*string, error)
}

// FileRepo is the storage behind the file routes.
type FileRepo interface {
	// AttendanceIDOn is the employee's attendance row of date, nil when none.
	AttendanceIDOn(ctx context.Context, employeeID, date string) (*string, error)
	// ShiftPattern is the employee's shift pattern with each shift's times.
	ShiftPattern(ctx context.Context, employeeID string) ([]*Row, error)
	InsertAttendance(ctx context.Context, f fields) (*Row, error)
	// AttendanceRecord is `SELECT *` of one attendance row, nil when none.
	AttendanceRecord(ctx context.Context, id string) (*Row, error)
	AttendanceExport(ctx context.Context, f exportFilter) ([]*Row, error)
	ContractDocument(ctx context.Context, id string) (*Row, error)
	SignedDocumentContract(ctx context.Context, id string) (*signedContract, error)
	// SetSignedDocument stores path (stamping signed_at when unset), or
	// clears the column when path is nil.
	SetSignedDocument(ctx context.Context, id string, path *string) error
}

func (s *store) AttendanceIDOn(ctx context.Context, employeeID, date string) (*string, error) {
	var id *string
	err := s.db.QueryRow(ctx, `SELECT id::text FROM hris.attendance WHERE employee_id = $1 AND date = $2`,
		employeeID, date).Scan(&id)
	if database.IsNoRows(err) {
		return nil, nil
	}
	// The TS ignores the lookup's error (a malformed date): the insert
	// reports it.
	return id, ignoreBadInput(err)
}

func (s *store) ShiftPattern(ctx context.Context, employeeID string) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT es.day_of_week, es.shift_id,
		es.effective_from::text, es.effective_to::text,
		s.name, s.start_time::text, s.end_time::text,
		s.late_tolerance_minutes, s.is_overnight
		FROM hris.v_employee_schedule es
		LEFT JOIN hris.shifts s ON s.id = es.shift_id
		WHERE es.employee_id = $1
		ORDER BY es.priority`, employeeID)
}

func (s *store) InsertAttendance(ctx context.Context, f fields) (*Row, error) {
	return insertRow(ctx, s.db, "hris.attendance", f, "*")
}

func (s *store) AttendanceRecord(ctx context.Context, id string) (*Row, error) {
	row, err := queryRow(ctx, s.db, `SELECT * FROM hris.attendance WHERE id = $1`, id)
	return row, ignoreBadInput(err)
}

// exportFilter is AttendanceExportFilters; empty strings are absent.
type exportFilter struct {
	EmployeeID, StartDate, EndDate, Status string
	// BranchID is the store of the attendance (hris.attendance.branch_id),
	// "none" for attendance without a store; LateOnly keeps late rows
	// (EPIC-075 R1, the Rekap tab's filters).
	BranchID string
	LateOnly bool
}

// AttendanceExport is listAttendanceForExport, newest first.
func (s *store) AttendanceExport(ctx context.Context, f exportFilter) ([]*Row, error) {
	var where []string
	var args []any
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.EmployeeID != "" {
		add("a.employee_id = $%d", f.EmployeeID)
	}
	if f.StartDate != "" && f.EndDate != "" {
		add("a.date >= $%d", f.StartDate)
		add("a.date <= $%d", f.EndDate)
	}
	if f.Status != "" {
		add("a.status = $%d", f.Status)
	}
	switch f.BranchID {
	case "":
	case "none":
		where = append(where, "a.branch_id IS NULL")
	default:
		add("a.branch_id = $%d", f.BranchID)
	}
	if f.LateOnly {
		add("a.is_late = $%d", true)
	}
	sql := `SELECT a.date::text, a.clock_in, a.clock_out, a.clock_in_location, a.clock_out_location,
		a.work_hours, a.break_minutes, a.status::text, a.is_late, a.late_minutes, a.notes,
		a.is_short_hours, a.min_work_hours,
		a.clock_in_photo_url, a.clock_out_photo_url,
		e.full_name, e.nip, d.name AS department, p.title AS job_title,
		b.code AS branch_code, b.name AS branch_name
		FROM hris.attendance a
		LEFT JOIN configuration.branches b ON b.id = a.branch_id
		LEFT JOIN hris.employees e ON e.id = a.employee_id
		LEFT JOIN hris.departments d ON d.id = e.department_id
		LEFT JOIN hris.positions p ON p.id = e.job_title_id`
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	return queryRows(ctx, s.db, sql+" ORDER BY a.date DESC", args...)
}

func (s *store) ContractDocument(ctx context.Context, id string) (*Row, error) {
	return queryRow(ctx, s.db, `SELECT c.contract_number, c.contract_type::text, c.start_date::text, c.end_date::text,
		c.probation_end_date::text, c.position_title, c.department_name,
		c.work_location, c.base_salary::text, c.signed_at::text,
		e.full_name, e.ktp, e.address, e.city, e.birth_date::text, e.phone
		FROM hris.employment_contracts c
		JOIN hris.employees e ON e.id = c.employee_id
		WHERE c.id = $1`, id)
}

// signedContract is SignedDocumentContract.
type signedContract struct {
	ID, EmployeeID, ContractNumber string
	SignedDocumentURL              *string
}

func (s *store) SignedDocumentContract(ctx context.Context, id string) (*signedContract, error) {
	var c signedContract
	err := s.db.QueryRow(ctx, `SELECT id::text, employee_id::text, contract_number, signed_document_url
		FROM hris.employment_contracts WHERE id = $1`, id).Scan(&c.ID, &c.EmployeeID, &c.ContractNumber, &c.SignedDocumentURL)
	if database.IsNoRows(err) {
		return nil, nil
	}
	return &c, err
}

func (s *store) SetSignedDocument(ctx context.Context, id string, path *string) error {
	sql := `UPDATE hris.employment_contracts SET signed_document_url = NULL WHERE id = $1`
	args := []any{id}
	if path != nil {
		// signed_at defaults to today when it was never set (editable later).
		sql = `UPDATE hris.employment_contracts
			SET signed_document_url = $2, signed_at = COALESCE(signed_at, now()::date) WHERE id = $1`
		args = append(args, *path)
	}
	_, err := s.db.Exec(ctx, sql, args...)
	return err
}

/* ── Data URLs and private files ─────────────────────────────────────── */

// readUploadJSON is readJson for bodies carrying a data URL: like readForm,
// but up to the proxy's 10 MB instead of validate.ReadBody's 1 MB, since
// a 5 MB image is about 7 MB of base64.
func readUploadJSON(r *http.Request, message string) (*validate.Form, error) {
	if message == "" {
		message = "Body JSON tidak valid"
	}
	if r.Body == nil {
		return nil, httpx.BadRequest(message)
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, storage.MaxRequestBytes+1))
	if err != nil || len(raw) > storage.MaxRequestBytes {
		return nil, httpx.BadRequest(message)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var body any
	if err := dec.Decode(&body); err != nil {
		return nil, httpx.BadRequest(message)
	}
	return validate.New(body, true), nil
}

// imageDataURL is /^data:(image\/(?:jpeg|png|webp));base64,(.+)$/: "." in
// JavaScript stops at line terminators, and "$" is the end of the input.
var imageDataURL = regexp.MustCompile(`^data:(image/(?:jpeg|png|webp));base64,([^\n\r\x{2028}\x{2029}]+)$`)

// parseImageDataURL returns the declared MIME and the decoded bytes, ok
// false when s is not a JPG/PNG/WebP data URL.
func parseImageDataURL(s string) (mime string, data []byte, ok bool) {
	m := imageDataURL.FindStringSubmatch(s)
	if m == nil {
		return "", nil, false
	}
	return m[1], nodeBase64(m[2]), true
}

// nodeBase64 is Buffer.from(s, "base64"): characters outside the base64 and
// base64url alphabets are skipped, decoding stops at "=", and a dangling
// sixth of a byte is dropped.
func nodeBase64(s string) []byte {
	clean := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '=':
			i = len(s)
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '+', c == '/':
			clean = append(clean, c)
		case c == '-':
			clean = append(clean, '+')
		case c == '_':
			clean = append(clean, '/')
		}
	}
	if len(clean)%4 == 1 {
		clean = clean[:len(clean)-1]
	}
	out, _ := base64.RawStdEncoding.DecodeString(string(clean))
	return out
}

// saveImage is the routes' data URL upload: a JPG/PNG/WebP data URL of at
// most maxBytes saved under folder. invalid and tooLarge are the route's
// messages; a rejected file answers with the storage's reason.
func (s *Service) saveImage(dataURL string, maxBytes int, folder, invalid, tooLarge string) (string, error) {
	mime, data, ok := parseImageDataURL(dataURL)
	if !ok {
		return "", httpx.BadRequest(invalid)
	}
	if len(data) > maxBytes {
		return "", httpx.BadRequest(tooLarge)
	}
	path, err := s.files.SavePrivateImage(data, mime, folder)
	if err != nil {
		return "", httpx.BadRequest(err.Error())
	}
	return path, nil
}

// privateSegments are the [...path] segments after prefix as Next hands
// them to the route (each decoded once), checked by safeSegmentsUnder.
func privateSegments(r *http.Request, prefix, folder string, minLength int) []string {
	rest, ok := strings.CutPrefix(r.URL.EscapedPath(), prefix)
	if !ok {
		return nil
	}
	raw := strings.Split(rest, "/")
	for i, seg := range raw {
		decoded, err := storage.DecodeURIComponent(seg)
		if err != nil {
			return nil
		}
		raw[i] = decoded
	}
	return storage.SafeSegmentsUnder(raw, folder, minLength)
}

// servePrivate writes a private file inline with the HRIS cache policy, or
// 404 "File tidak ditemukan".
func (s *Service) servePrivate(w http.ResponseWriter, rel, disposition string) error {
	data, mime, err := s.files.ReadPrivate(rel)
	if err != nil {
		return httpx.NotFound("File tidak ditemukan")
	}
	storage.WritePrivateFile(w, data, mime, "private, max-age=3600", disposition)
	return nil
}

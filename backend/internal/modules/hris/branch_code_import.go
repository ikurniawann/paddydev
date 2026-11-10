package hris

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/xlsx"
)

// Kode & Status Toko (EPIC-100): a template of every branch, then preview →
// commit in one transaction, modelled on the Saldo Cuti import. A code is
// changed on the same row (R1), so every history keeps pointing at it.

const maxBranchCodeUpload = 16 << 20

type branchCodeImportInput struct {
	Data        []byte
	FileName    string
	Step        string // preview | commit | error_file
	Fingerprint string
	AckWarnings bool
	Meta        audit.Entry
}

type branchCodeRow struct {
	domain.BranchRef
	Company string
}

func loadBranchRows(ctx context.Context, q database.Querier) ([]branchCodeRow, error) {
	rows, err := q.Query(ctx, `SELECT b.id::text, b.code, b.name, b.is_active, coalesce(c.name, '')
		FROM configuration.branches b LEFT JOIN configuration.companies c ON c.id = b.company_id
		ORDER BY coalesce(c.name, ''), b.code, b.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []branchCodeRow
	for rows.Next() {
		var r branchCodeRow
		if err := rows.Scan(&r.ID, &r.Code, &r.Name, &r.Active, &r.Company); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// branchUsage is, per active branch, what keeps it from being deactivated:
// employees with a running placement and shift days from today (WIB).
func branchUsage(ctx context.Context, q database.Querier, today string) (map[string]domain.BranchUsage, error) {
	out := map[string]domain.BranchUsage{}
	rows, err := q.Query(ctx, `SELECT ep.branch_id::text, e.full_name
		FROM hris.employee_placements ep JOIN hris.employees e ON e.id = ep.employee_id
		WHERE e.is_active AND (ep.end_date IS NULL OR ep.end_date >= $1::date)
		ORDER BY ep.branch_id, e.full_name`, today)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			rows.Close()
			return nil, err
		}
		u := out[id]
		u.Employees = append(u.Employees, name)
		out[id] = u
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows, err = q.Query(ctx, `SELECT branch_id::text, count(*) FROM hris.employee_shift_days
		WHERE branch_id IS NOT NULL AND work_date >= $1::date GROUP BY branch_id`, today)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		u := out[id]
		u.FutureDays = n
		out[id] = u
	}
	return out, rows.Err()
}

func refsOf(rows []branchCodeRow) []domain.BranchRef {
	out := make([]domain.BranchRef, len(rows))
	for i, r := range rows {
		out[i] = r.BranchRef
	}
	return out
}

func activeLabel(active bool) string {
	if active {
		return "Aktif"
	}
	return "Nonaktif"
}

// BranchCodeTemplate is the Data Toko workbook: every branch as stored now.
func (s *Service) BranchCodeTemplate(ctx context.Context) (*exportFile, error) {
	branches, err := loadBranchRows(ctx, s.repo.querier())
	if err != nil {
		return nil, err
	}
	rows := make([][]any, 0, len(branches))
	for _, b := range branches {
		rows = append(rows, []any{b.Code, b.Company, b.Name, activeLabel(b.Active), nil, nil, nil})
	}
	h := domain.BranchCodeHeaders
	sheet := xlsx.TemplateSheet{
		Name: domain.BranchCodeSheet,
		TitleLines: []string{
			"Kode & Status Toko — " + domain.NowWIB(s.now()).Format("2006-01-02 15:04") + " WIB",
			"Isi Kode Baru, Nama Baru, atau Status Baru hanya di toko yang berubah. Kolom abu-abu hanya informasi.",
		},
		Columns: []xlsx.TemplateColumn{
			{Header: h[0], ReadOnly: true, Width: 24},
			{Header: h[1], ReadOnly: true, Width: 20},
			{Header: h[2], ReadOnly: true, Width: 44},
			{Header: h[3], ReadOnly: true, Width: 14},
			{Header: h[4], Width: 18},
			{Header: h[5], Width: 44},
			{Header: h[6], Kind: xlsx.KindList, Options: []string{"Aktif", "Nonaktif"}, Width: 14},
		},
		Rows:       rows,
		FreezeCols: 1,
		Protect:    true,
	}
	notes := []xlsx.SheetSpec{{Name: "Petunjuk", ColumnWidths: []float64{110}, Rows: [][]any{
		{"Cara mengisi"},
		{"1. Setiap baris = satu toko. Jangan ubah Kode Saat Ini atau judul kolom."},
		{"2. Kode Baru: huruf besar, angka, dan tanda -, 2–30 karakter, belum dipakai toko lain. Histori absen, penempatan, jadwal, dan payroll ikut kode baru."},
		{"3. Nama Baru kosong + Kode Baru diisi: awalan \"KODE - \" di nama mengikuti kode baru."},
		{"4. Status Baru Nonaktif untuk toko tutup. Ditolak bila masih ada karyawan aktif atau jadwal mulai hari ini — pindahkan dulu lewat Penempatan."},
		{"5. Sel kosong = tidak diubah. Unggah file di halaman Penempatan → Kode & Status Toko, periksa pratinjau, lalu simpan."},
	}}}
	data, err := xlsx.BuildTemplate([]xlsx.TemplateSheet{sheet}, notes, nil)
	if err != nil {
		return nil, err
	}
	return &exportFile{data, xlsx.ContentType, "Kode & Status Toko.xlsx"}, nil
}

type branchCodeSummary struct {
	Rows       int `json:"rows"`
	Changed    int `json:"changed"`
	Code       int `json:"code"`
	Name       int `json:"name"`
	Deactivate int `json:"deactivate"`
	Activate   int `json:"activate"`
}

type branchCodeChangeView struct {
	Line      int    `json:"line"`
	OldCode   string `json:"old_code"`
	NewCode   string `json:"new_code"`
	OldName   string `json:"old_name"`
	NewName   string `json:"new_name"`
	OldActive bool   `json:"old_active"`
	NewActive bool   `json:"new_active"`
}

// branchCodeFingerprint pins the file and the stored state of the branches it
// changes, so a commit after someone else's edit writes nothing.
func branchCodeFingerprint(data []byte, changes []domain.BranchCodeChange, byID map[string]domain.BranchRef) string {
	h := sha256.New()
	h.Write(data)
	ids := make([]string, 0, len(changes))
	for _, c := range changes {
		ids = append(ids, c.ID)
	}
	sort.Strings(ids)
	for _, id := range ids {
		b := byID[id]
		fmt.Fprintf(h, "|%s|%s|%s|%t", b.ID, b.Code, b.Name, b.Active)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func branchAudit(code, name string, active bool) map[string]any {
	return map[string]any{"code": code, "name": name, "is_active": active}
}

// ImportBranchCodes runs one step of the Kode & Status Toko import.
func (s *Service) ImportBranchCodes(ctx context.Context, a *Actor, in branchCodeImportInput) (map[string]any, *exportFile, error) {
	switch in.Step {
	case "preview", "commit", "error_file":
	default:
		return nil, nil, httpx.BadRequest("Langkah import tidak dikenal")
	}
	if !strings.HasSuffix(strings.ToLower(in.FileName), ".xlsx") {
		return nil, nil, httpx.BadRequest("Unggah file .xlsx dari template Kode & Status Toko")
	}
	matrix, err := xlsx.ParseMatrix(in.Data, xlsx.ReadOptions{PreferSheet: domain.BranchCodeSheet})
	if err != nil {
		return nil, nil, httpx.BadRequest("File tidak bisa dibaca — pastikan formatnya .xlsx")
	}
	today := domain.TodayWIB(s.now())
	var out map[string]any
	var errFile *exportFile
	err = s.repo.InTx(ctx, func(r Repository) error {
		q := r.querier()
		validate := func() (domain.BranchCodeResult, []domain.BranchRef, error) {
			branches, err := loadBranchRows(ctx, q)
			if err != nil {
				return domain.BranchCodeResult{}, nil, err
			}
			usage, err := branchUsage(ctx, q, today)
			if err != nil {
				return domain.BranchCodeResult{}, nil, err
			}
			refs := refsOf(branches)
			res, problem := domain.ValidateBranchCodeUpload(matrix, refs, usage)
			if problem != "" {
				return res, refs, httpx.BadRequest(problem)
			}
			return res, refs, nil
		}
		res, refs, err := validate()
		if err != nil {
			return err
		}
		if in.Step == "commit" && len(res.Changes) > 0 {
			// Lock only the branches the file changes (a placement or clock-in
			// at them waits), then check again with what they hold now.
			ids := make([]string, 0, len(res.Changes))
			for _, c := range res.Changes {
				ids = append(ids, c.ID)
			}
			sort.Strings(ids)
			if _, err := q.Exec(ctx, `SELECT 1 FROM configuration.branches WHERE id::text = ANY($1) ORDER BY id FOR UPDATE`, ids); err != nil {
				return err
			}
			if res, refs, err = validate(); err != nil {
				return err
			}
		}
		if in.Step == "error_file" {
			data, err := errorWorkbook(matrix, res.HeaderLine-1, append(append([]domain.UploadIssue{}, res.Errors...), res.Warnings...), domain.BranchCodeSheet)
			if err != nil {
				return err
			}
			errFile = &exportFile{data, xlsx.ContentType, "Error - " + in.FileName}
			return nil
		}
		byID := map[string]domain.BranchRef{}
		for _, b := range refs {
			byID[b.ID] = b
		}
		sum := branchCodeSummary{Changed: len(res.Changes)}
		for i := res.HeaderLine; i < len(matrix); i++ {
			if strings.TrimSpace(strings.Join(matrix[i], "")) != "" {
				sum.Rows++
			}
		}
		views := make([]branchCodeChangeView, 0, len(res.Changes))
		for _, c := range res.Changes {
			if c.NewCode != c.OldCode {
				sum.Code++
			}
			if c.NewName != c.OldName {
				sum.Name++
			}
			if c.OldActive && !c.NewActive {
				sum.Deactivate++
			}
			if !c.OldActive && c.NewActive {
				sum.Activate++
			}
			views = append(views, branchCodeChangeView{c.Line, c.OldCode, c.NewCode, c.OldName, c.NewName, c.OldActive, c.NewActive})
		}
		fp := branchCodeFingerprint(in.Data, res.Changes, byID)
		if in.Step != "commit" {
			out = map[string]any{"summary": sum, "errors": res.Errors, "warnings": res.Warnings, "fingerprint": fp, "changes": views}
			return nil
		}
		switch {
		case len(res.Errors) > 0:
			return httpx.BadRequest("File masih punya error — perbaiki lalu unggah ulang")
		case len(res.Warnings) > 0 && !in.AckWarnings:
			return httpx.BadRequest(`Centang "Saya sudah memeriksa peringatan" sebelum menyimpan`)
		case fp != in.Fingerprint:
			return httpx.Conflict("Data toko sudah diubah sejak preview — unggah ulang file")
		}
		for _, c := range res.Changes {
			if _, err := q.Exec(ctx, `UPDATE configuration.branches SET code = $2, name = $3, is_active = $4, updated_at = now()
				WHERE id = $1`, c.ID, c.NewCode, c.NewName, c.NewActive); err != nil {
				if database.IsUniqueViolation(err) {
					return httpx.Conflict("Kode " + c.NewCode + " sudah dipakai toko lain — unggah ulang file")
				}
				return err
			}
			entry := in.Meta
			id, label := c.ID, c.NewCode+" · "+c.NewName
			entry.ActorID, entry.ActorName = a.UserID, ptr(actorName(a))
			entry.Action, entry.Entity, entry.EntityID, entry.EntityLabel = "branch.update", "configuration.branches", &id, &label
			entry.Before = branchAudit(c.OldCode, c.OldName, c.OldActive)
			entry.After = branchAudit(c.NewCode, c.NewName, c.NewActive)
			if err := audit.Write(ctx, q, entry); err != nil {
				return err
			}
		}
		msg := "Tidak ada perubahan toko"
		if n := len(res.Changes); n > 0 {
			msg = fmt.Sprintf("%d toko diperbarui", n)
		}
		out = map[string]any{"summary": sum, "message": msg}
		return nil
	})
	return out, errFile, err
}

/* ── HTTP ────────────────────────────────────────────────────────────── */

// GET /api/hris/branches/template
func (h handlers) branchCodeTemplate(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	file, err := h.svc.BranchCodeTemplate(r.Context())
	if err != nil {
		return err
	}
	return writeFile(w, file)
}

// POST /api/hris/branches/import (multipart): file, step = preview | commit
// | error_file, fingerprint, ack_warnings.
func (h handlers) importBranchCodes(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	a, err := h.actorFor(r.Context(), u)
	if err != nil {
		return err
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBranchCodeUpload+1<<20)
	const pick = "Unggah file .xlsx dari template Kode & Status Toko"
	if err = r.ParseMultipartForm(16 << 20); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return httpx.BadRequest("File terlalu besar — maksimal 16 MB")
		}
		return httpx.BadRequest(pick)
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return httpx.BadRequest(pick)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxBranchCodeUpload+1))
	if err != nil {
		return err
	}
	if len(data) > maxBranchCodeUpload {
		return httpx.BadRequest("File terlalu besar — maksimal 16 MB")
	}
	in := branchCodeImportInput{Data: data, FileName: header.Filename, Step: r.FormValue("step"),
		Fingerprint: r.FormValue("fingerprint"), AckWarnings: r.FormValue("ack_warnings") == "true",
		Meta: audit.Entry{}.WithRequest(r)}
	out, errFile, err := h.svc.ImportBranchCodes(r.Context(), a, in)
	if err != nil {
		return err
	}
	if errFile != nil {
		return writeFile(w, errFile)
	}
	if in.Step == "commit" {
		msg, _ := out["message"].(string)
		return ok(w, dataMsg(map[string]any{"summary": out["summary"]}, msg))
	}
	return ok(w, dataOf(out))
}

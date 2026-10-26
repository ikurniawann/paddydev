package hris

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/xlsx"
)

// Saldo Cuti Excel template and import (EPIC-068 R10), modelled on the Jadwal
// Kerja import: a template of the balances in scope, then preview → commit in
// one transaction → an error file. Rows whose figures did not change are
// ignored; every changed employee gets one leave_balance.import audit row.

const (
	leaveSheet     = "Saldo Cuti"
	leaveInfoSheet = "Info"
	// maxLeaveChanges bounds the changes listed in a preview.
	maxLeaveChanges = 500
	// maxLeaveUpload is the largest .xlsx accepted; the request body may add
	// 1 MB of form fields on top.
	maxLeaveUpload = 16 << 20
)

type leaveScope struct {
	Year   int
	Branch *scheduleBranch // nil = all branches
}

func (sc leaveScope) branchID() string {
	if sc.Branch == nil {
		return ""
	}
	return sc.Branch.ID
}

// leaveScopeOf checks the optional branch (empty = all branches).
func leaveScopeOf(ctx context.Context, q database.Querier, year int, branchID string) (leaveScope, error) {
	sc := leaveScope{Year: year}
	if branchID == "" {
		return sc, nil
	}
	if !domain.UUIDRe.MatchString(branchID) {
		return sc, httpx.BadRequest("Cabang tidak valid")
	}
	var b scheduleBranch
	err := q.QueryRow(ctx, `SELECT id::text, name, coalesce(code, '') FROM configuration.branches WHERE id = $1`, branchID).
		Scan(&b.ID, &b.Name, &b.Code)
	if database.IsNoRows(err) {
		return sc, httpx.BadRequest("Cabang tidak ditemukan")
	}
	if err != nil {
		return sc, err
	}
	sc.Branch = &b
	return sc, nil
}

func (sc leaveScope) label() string {
	if sc.Branch == nil {
		return "Semua Cabang"
	}
	return sc.Branch.Name
}

/* ── Template ────────────────────────────────────────────────────────── */

// LeaveBalanceTemplate is the xlsx HR fills in: one row per active employee
// of the branch (primary placement; every branch when empty) with the stored
// balance of year, or the default allocation where none is stored yet.
func (s *Service) LeaveBalanceTemplate(ctx context.Context, year int, branchID string) (*exportFile, error) {
	q := s.repo.querier()
	sc, err := leaveScopeOf(ctx, q, year, branchID)
	if err != nil {
		return nil, err
	}
	today := domain.TodayWIB(s.now())
	views, err := leaveBalanceViews(ctx, q, balanceFilter{Year: year, Today: today, BranchID: sc.branchID()})
	if err != nil {
		return nil, err
	}
	rows := make([][]any, 0, len(views))
	for _, v := range views {
		if v.NIP == nil || *v.NIP == "" {
			continue
		}
		branch := ""
		if v.BranchName != nil {
			branch = *v.BranchName
		}
		var expires any
		if v.CarryOverExpiresOn != nil {
			if t, err := time.Parse(domain.DateLayout, *v.CarryOverExpiresOn); err == nil {
				expires = t
			}
		}
		rows = append(rows, []any{*v.NIP, v.FullName, branch, v.AnnualLeaveUsed,
			v.AnnualLeaveTotal, v.AnnualLeaveAdjust, v.CarryOverDays, expires, nil})
	}
	sheet := xlsx.TemplateSheet{
		Name: leaveSheet,
		TitleLines: []string{
			fmt.Sprintf("Saldo Cuti %d — %s", year, sc.label()),
			"Ubah kolom Jatah, Penyesuaian, Sisa Bawaan, dan Kedaluwarsa, lalu isi Alasan di setiap baris yang berubah. Kolom abu-abu hanya informasi.",
		},
		Columns: []xlsx.TemplateColumn{
			{Header: domain.LeaveColNIP, ReadOnly: true, Width: 16},
			{Header: domain.LeaveColName, ReadOnly: true, Width: 28},
			{Header: domain.LeaveColBranch, ReadOnly: true, Width: 22},
			{Header: domain.LeaveColUsed, ReadOnly: true, Kind: xlsx.KindNumber, Width: 11},
			{Header: domain.LeaveColTotal, Kind: xlsx.KindNumber, Width: 10},
			{Header: domain.LeaveColAdjustment, Kind: xlsx.KindNumber, Width: 13},
			{Header: domain.LeaveColCarry, Kind: xlsx.KindNumber, Width: 13},
			{Header: domain.LeaveColExpires, Kind: xlsx.KindDate, Width: 14},
			{Header: domain.LeaveColReason, Width: 40},
		},
		Rows:       rows,
		FreezeCols: 2,
		Protect:    true,
	}
	notes := []xlsx.SheetSpec{{Name: "Petunjuk", ColumnWidths: []float64{110}, Rows: [][]any{
		{"Cara mengisi saldo cuti"},
		{"1. Buka sheet \"Saldo Cuti\". Setiap baris = satu karyawan aktif; kolom Terpakai hanya informasi."},
		{"2. Ubah Jatah, Penyesuaian (boleh negatif), Sisa Bawaan, dan Kedaluwarsa (tanggal terakhir sisa bawaan boleh dipakai)."},
		{"3. Isi Alasan di setiap baris yang Anda ubah. Baris yang tidak berubah dilewati."},
		{"4. Jangan ubah NIP atau judul kolom. Simpan file, lalu unggah di halaman Saldo Cuti dan periksa hasil pengecekan sebelum menyimpan."},
		{""},
		{"Aturan angka:"},
		{"- Jatah, Penyesuaian, dan Sisa Bawaan kelipatan 0,5 hari."},
		{"- Sisa jatah (Jatah + Penyesuaian − Terpakai) tidak boleh kurang dari 0; Sisa Bawaan tidak boleh kurang dari bawaan yang sudah terpakai."},
		{"- Hanya saldo tahun berjalan dan tahun depan yang bisa diubah."},
		{""},
		{"File akan ditolak bila:"},
		{"- file bukan template tahun/cabang yang dipilih"},
		{"- NIP tidak dikenal, karyawan nonaktif, di luar cabang yang dipilih, atau ganda"},
		{"- angka bukan kelipatan 0,5, sisa jatah kurang dari 0, atau Alasan kosong pada baris yang berubah"},
		{""},
		{"Peringatan (boleh lanjut setelah diperiksa):"},
		{"- Jatah lebih dari 24 hari"},
		{"- Sisa Bawaan tanpa tanggal Kedaluwarsa (dianggap tidak berlaku)"},
		{"- Tanggal Kedaluwarsa yang sudah lewat"},
	}}}
	stamp := domain.NowWIB(s.now()).Format("2006-01-02 15:04")
	info := &xlsx.InfoSheet{Name: leaveInfoSheet, Pairs: [][2]string{
		{"Tahun", strconv.Itoa(year)}, {"Cabang ID", sc.branchID()}, {"Diunduh", stamp},
	}}
	data, err := xlsx.BuildTemplate([]xlsx.TemplateSheet{sheet}, notes, info)
	if err != nil {
		return nil, err
	}
	name := fmt.Sprintf("Saldo Cuti - %s - %d.xlsx", scopeFileLabel(sc), year)
	return &exportFile{data, xlsx.ContentType, name}, nil
}

func scopeFileLabel(sc leaveScope) string {
	switch {
	case sc.Branch == nil:
		return "Semua Cabang"
	case sc.Branch.Code != "":
		return sc.Branch.Code
	}
	return sc.Branch.Name
}

/* ── Import ──────────────────────────────────────────────────────────── */

type leaveImportInput struct {
	Year        int
	BranchID    string
	Data        []byte
	FileName    string
	Step        string // preview | commit | error_file
	Fingerprint string
	AckWarnings bool
	Meta        audit.Entry // IP and user agent of the request
}

type leaveSummary struct {
	Employees int `json:"employees"`
	New       int `json:"new"`
	Changed   int `json:"changed"`
	Same      int `json:"same"`
}

// leaveFigures is a balance's editable part as the preview shows it.
type leaveFigures struct {
	Total        float64 `json:"annual_leave_total"`
	Adjustment   float64 `json:"annual_leave_adjustment"`
	CarryDays    float64 `json:"carry_over_days"`
	CarryExpires *string `json:"carry_over_expires_on"`
}

func figuresOf(v domain.LeaveBalanceValues) leaveFigures {
	f := leaveFigures{Total: v.Total, Adjustment: v.Adjustment, CarryDays: v.CarryDays}
	if v.CarryExpires != "" {
		f.CarryExpires = &v.CarryExpires
	}
	return f
}

type leaveChange struct {
	NIP    string       `json:"nip"`
	Name   string       `json:"name"`
	New    bool         `json:"new"`
	Before leaveFigures `json:"before"`
	After  leaveFigures `json:"after"`
	Reason string       `json:"reason"`
}

// leaveCheck adapts validateLeaveBalance to the pure upload validation: the
// message of its 400, or "".
func leaveCheck(nowYear int) func(domain.LeaveBalanceCheck) string {
	return func(c domain.LeaveBalanceCheck) string {
		in := LeaveBalanceInput{Year: c.Year, Total: c.Values.Total, Adjustment: c.Values.Adjustment,
			CarryDays: c.Values.CarryDays, Reason: c.Reason}
		if c.Values.CarryExpires != "" {
			in.CarryExpires = &c.Values.CarryExpires
		}
		err := validateLeaveBalance(in, c.Used, c.CarryUsed, c.Active, c.CarryChanged, nowYear)
		if err == nil {
			return ""
		}
		var he *httpx.Error
		if errors.As(err, &he) {
			return he.Message
		}
		return err.Error()
	}
}

// leaveUploadContext gathers the balances of the scope and what is known of
// the NIPs in the file.
func leaveUploadContext(ctx context.Context, q database.Querier, sc leaveScope, today string, nowYear int,
	nips []string) (domain.LeaveBalanceUploadContext, map[string]LeaveBalanceView, error) {
	c := domain.LeaveBalanceUploadContext{Year: sc.Year, BranchID: sc.branchID(), Today: today,
		Employees: map[string]domain.LeaveBalanceEmployee{}, Check: leaveCheck(nowYear)}
	if sc.Branch != nil {
		c.BranchName = sc.Branch.Name
	}
	views, err := leaveBalanceViews(ctx, q, balanceFilter{Year: sc.Year, Today: today, BranchID: sc.branchID()})
	if err != nil {
		return c, nil, err
	}
	byID := make(map[string]LeaveBalanceView, len(views))
	for _, v := range views {
		byID[v.EmployeeID] = v
		if v.NIP == nil {
			continue
		}
		cur := domain.LeaveBalanceValues{Total: v.AnnualLeaveTotal, Adjustment: v.AnnualLeaveAdjust, CarryDays: v.CarryOverDays}
		if v.CarryOverExpiresOn != nil {
			cur.CarryExpires = *v.CarryOverExpiresOn
		}
		c.Employees[*v.NIP] = domain.LeaveBalanceEmployee{ID: v.EmployeeID, Name: v.FullName, Active: true, InScope: true,
			Exists: v.Exists, Current: cur, Used: v.AnnualLeaveUsed, CarryUsed: v.CarryOverUsed}
	}
	// NIPs outside the scope or inactive: only needed to say why they are refused.
	var missing []string
	for _, n := range nips {
		if _, ok := c.Employees[n]; !ok {
			missing = append(missing, n)
		}
	}
	if len(missing) == 0 {
		return c, byID, nil
	}
	rows, err := q.Query(ctx, `SELECT id::text, nip, full_name, is_active FROM hris.employees WHERE nip = ANY($1::text[])`, missing)
	if err != nil {
		return c, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var e domain.LeaveBalanceEmployee
		var nip string
		if err := rows.Scan(&e.ID, &nip, &e.Name, &e.Active); err != nil {
			return c, nil, err
		}
		c.Employees[nip] = e
	}
	return c, byID, rows.Err()
}

// leaveFingerprint is the md5 of the uploaded file and of the fields the
// commit writes or validates (jatah, penyesuaian, sisa bawaan, kedaluwarsa and
// bawaan terpakai) of the employees in it: a commit is refused when another
// file is sent or any of those changed since the preview. "Terpakai" is left
// out on purpose, an approval moves it often and the commit re-checks the
// rules against it under the row lock.
func leaveFingerprint(fileData []byte, ids []string, byID map[string]LeaveBalanceView) string {
	sorted := append([]string{}, ids...)
	sort.Strings(sorted)
	fileSum := md5.Sum(fileData)
	var b strings.Builder
	fmt.Fprintf(&b, "%x\n", fileSum)
	for _, id := range sorted {
		v := byID[id]
		expires := ""
		if v.CarryOverExpiresOn != nil {
			expires = *v.CarryOverExpiresOn
		}
		fmt.Fprintf(&b, "%s|%g|%g|%g|%g|%s\n", id, v.AnnualLeaveTotal, v.AnnualLeaveAdjust,
			v.CarryOverDays, v.CarryOverUsed, expires)
	}
	sum := md5.Sum([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// ImportLeaveBalances runs one step of the Saldo Cuti import. preview reports
// errors, warnings, a fingerprint and the changes; commit writes them in one
// transaction (any error or a stale fingerprint writes nothing); error_file
// returns the upload with a "Keterangan Error" column.
func (s *Service) ImportLeaveBalances(ctx context.Context, a *Actor, in leaveImportInput) (map[string]any, *exportFile, error) {
	switch in.Step {
	case "preview", "commit", "error_file":
	default:
		return nil, nil, httpx.BadRequest("Langkah import tidak dikenal")
	}
	if !strings.HasSuffix(strings.ToLower(in.FileName), ".xlsx") {
		return nil, nil, httpx.BadRequest("Unggah file .xlsx dari template Saldo Cuti")
	}
	matrix, err := xlsx.ParseMatrix(in.Data, xlsx.ReadOptions{PreferSheet: leaveSheet})
	if err != nil {
		return nil, nil, httpx.BadRequest("File tidak bisa dibaca — pastikan formatnya .xlsx")
	}
	info, err := xlsx.ParseMatrix(in.Data, xlsx.ReadOptions{PreferSheet: leaveInfoSheet})
	if err != nil || len(info) == 0 || len(info[0]) == 0 || domain.NormalizeHeader(info[0][0]) != "tahun" {
		info = nil
	}
	file, msg := domain.ReadLeaveBalanceFile(matrix, info)
	if msg != "" {
		return nil, nil, httpx.BadRequest(msg)
	}
	nowYear := domain.NowWIB(s.now()).Year()
	today := domain.TodayWIB(s.now())
	nips := make([]string, 0, len(file.Rows))
	for _, row := range file.Rows {
		if nip, m := domain.CleanNIP(row.NIP); m == "" && nip != "" {
			nips = append(nips, nip)
		}
	}
	var out map[string]any
	var errFile *exportFile
	err = s.repo.InTx(ctx, func(r Repository) error {
		q := r.querier()
		sc, err := leaveScopeOf(ctx, q, in.Year, in.BranchID)
		if err != nil {
			return err
		}
		uctx, byID, err := leaveUploadContext(ctx, q, sc, today, nowYear, nips)
		if err != nil {
			return err
		}
		res := domain.ValidateLeaveBalanceUpload(file, uctx)
		if res.Errors == nil {
			res.Errors = []domain.UploadIssue{}
		}
		if res.Warnings == nil {
			res.Warnings = []domain.UploadIssue{}
		}
		if in.Step == "error_file" {
			data, err := errorWorkbook(matrix, file.HeaderLine-1, append(append([]domain.UploadIssue{}, res.Errors...), res.Warnings...), leaveSheet)
			if err != nil {
				return err
			}
			errFile = &exportFile{data, xlsx.ContentType, "Error - " + in.FileName}
			return nil
		}
		sum := leaveSummary{Employees: len(res.EmployeeIDs), Same: res.Same}
		changes := []leaveChange{}
		for _, c := range res.Changes {
			if c.New {
				sum.New++
			} else {
				sum.Changed++
			}
			if len(changes) < maxLeaveChanges {
				changes = append(changes, leaveChange{NIP: c.NIP, Name: c.Name, New: c.New,
					Before: figuresOf(c.Before), After: figuresOf(c.After), Reason: c.Reason})
			}
		}
		fp := leaveFingerprint(in.Data, res.EmployeeIDs, byID)
		if in.Step != "commit" {
			out = map[string]any{"summary": sum, "errors": res.Errors, "warnings": res.Warnings, "fingerprint": fp, "changes": changes}
			return nil
		}
		switch {
		case len(res.Errors) > 0:
			return httpx.BadRequest("File masih punya error — perbaiki lalu unggah ulang")
		case len(res.Warnings) > 0 && !in.AckWarnings:
			return httpx.BadRequest(`Centang "Saya sudah memeriksa peringatan" sebelum menyimpan`)
		case fp != in.Fingerprint:
			return httpx.Conflict("Saldo cuti sudah diubah orang lain sejak preview — unggah ulang file")
		}
		check := leaveCheck(nowYear)
		// Lock in a fixed order so two imports never wait on each other.
		ordered := append([]domain.LeaveBalanceChange{}, res.Changes...)
		sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].EmployeeID < ordered[j].EmployeeID })
		for _, c := range ordered {
			if err := lockPreviousBalance(ctx, q, c.EmployeeID, sc.Year); err != nil {
				return err
			}
			if err := ensureLeaveBalance(ctx, q, c.EmployeeID, sc.Year); err != nil {
				return err
			}
			was, err := r.LockLeaveBalance(ctx, c.EmployeeID, sc.Year)
			if err != nil {
				return err
			}
			if was == nil {
				return httpx.NotFound("Karyawan tidak ditemukan")
			}
			li := LeaveBalanceInput{Year: sc.Year, Total: c.After.Total, Adjustment: c.After.Adjustment,
				CarryDays: c.After.CarryDays, Reason: c.Reason}
			if c.After.CarryExpires != "" {
				li.CarryExpires = &c.After.CarryExpires
			}
			// The rules again under the row lock, with the figures it holds now.
			if m := check(domain.LeaveBalanceCheck{Year: sc.Year, Values: c.After, Reason: c.Reason,
				Used: was.Used, CarryUsed: was.CarryUsed, Active: true,
				CarryChanged: c.After.CarryDays != was.CarryDays || !sameText(li.CarryExpires, was.CarryExpires)}); m != "" {
				return httpx.BadRequest(c.Name + ": " + m)
			}
			if err := r.SetLeaveBalance(ctx, c.EmployeeID, sc.Year, li); err != nil {
				return err
			}
			entry := in.Meta
			label := fmt.Sprintf("%s · Saldo cuti %d", c.Name, sc.Year)
			id, reason := c.EmployeeID, c.Reason
			entry.ActorID, entry.ActorName = a.UserID, ptr(actorName(a))
			entry.Action, entry.Entity, entry.EntityID, entry.EntityLabel = "leave_balance.import", "hris.employees", &id, &label
			entry.Before = balanceAudit(sc.Year, *was)
			after := *was
			after.Total, after.Adjustment, after.CarryDays, after.CarryExpires = li.Total, li.Adjustment, li.CarryDays, li.CarryExpires
			after.Source = nextCarrySource(*was, li.CarryDays, li.CarryExpires)
			entry.After = balanceAudit(sc.Year, after)
			entry.Reason = &reason
			if err := audit.Write(ctx, q, entry); err != nil {
				return err
			}
			if err := s.carryAfterHRInput(ctx, q, c.EmployeeID, li, *was, userOf(a), carryCause{Type: causeImport}); err != nil {
				return err
			}
		}
		msg := "Tidak ada perubahan saldo cuti"
		if n := len(res.Changes); n > 0 {
			msg = fmt.Sprintf("Saldo cuti %d karyawan disimpan", n)
		}
		out = map[string]any{"summary": sum, "message": msg}
		return nil
	})
	return out, errFile, err
}

/* ── HTTP ────────────────────────────────────────────────────────────── */

// GET /api/hris/leave-balances/template?year=&branch_id=
func (h handlers) leaveBalanceTemplate(w http.ResponseWriter, r *http.Request, _ *Actor) error {
	year, err := h.balanceYear(r)
	if err != nil {
		return err
	}
	f := queryForm(r)
	branch := branchParam(f)
	if err := formErr(f, ""); err != nil {
		return err
	}
	file, err := h.svc.LeaveBalanceTemplate(r.Context(), year, branch)
	if err != nil {
		return err
	}
	return writeFile(w, file)
}

// POST /api/hris/leave-balances/import (multipart): file, year, branch_id,
// step = preview | commit | error_file, fingerprint, ack_warnings.
func (h handlers) importLeaveBalances(w http.ResponseWriter, r *http.Request, a *Actor) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxLeaveUpload+1<<20)
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return httpx.BadRequest("File terlalu besar — maksimal 16 MB")
		}
		return httpx.BadRequest("Unggah file .xlsx dari template Saldo Cuti")
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return httpx.BadRequest("Unggah file .xlsx dari template Saldo Cuti")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxLeaveUpload+1))
	if err != nil {
		return err
	}
	if len(data) > maxLeaveUpload {
		return httpx.BadRequest("File terlalu besar — maksimal 16 MB")
	}
	year := domain.NowWIB(h.svc.now()).Year()
	if raw := strings.TrimSpace(r.FormValue("year")); raw != "" {
		y, err := strconv.Atoi(raw)
		if err != nil || y < 2000 || y > 2100 {
			return httpx.BadRequest("Tahun tidak valid")
		}
		year = y
	}
	in := leaveImportInput{Year: year, BranchID: strings.TrimSpace(r.FormValue("branch_id")), Data: data,
		FileName: header.Filename, Step: r.FormValue("step"), Fingerprint: r.FormValue("fingerprint"),
		AckWarnings: r.FormValue("ack_warnings") == "true", Meta: audit.Entry{}.WithRequest(r)}
	out, errFile, err := h.svc.ImportLeaveBalances(r.Context(), a, in)
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

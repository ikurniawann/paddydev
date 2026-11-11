package hris

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/xlsx"
)

const (
	maxOrgChartUpload  = 16 << 20
	errOrgUploadPick   = "Unggah file .xlsx dari template bagan"
	errOrgLabelTaken   = "Label versi sudah dipakai"
	errOrgFingerprint  = "File atau label berubah sejak pratinjau — pratinjau ulang"
	errOrgOnlyDraftDel = "Hanya versi draft yang bisa dihapus"
)

// Impor versi bagan (EPIC-103): template dari versi yang dilihat, lalu
// pratinjau → simpan sebagai draft. Hanya hris.org_chart_versions dan
// hris.org_chart_nodes yang berubah (R1).

func yaTidak(b bool) string {
	if b {
		return "Ya"
	}
	return "Tidak"
}

// orgChartTemplate is the Bagan workbook of one version (R2): every cell is
// text, so codes and levels come back exactly as written.
func orgChartTemplate(v orgVersion, nodes []orgNodeView) (*exportFile, error) {
	header := make([]any, len(domain.OrgChartHeaders))
	for i, h := range domain.OrgChartHeaders {
		header[i] = h
	}
	rows := [][]any{header}
	for _, n := range nodes {
		confirm := ""
		if n.NeedsConfirm {
			confirm = "Ya"
		}
		rows = append(rows, []any{n.Code, deref(n.ParentCode), n.Title, yaTidak(n.IsPosition), n.DivisionCode, n.DivisionName,
			deref(n.JobLevel), domain.EmploymentTypeLabel(deref(n.EmploymentType)), deref(n.Vendor), deref(n.Region),
			strings.Join(n.StoreCodes, ", "), domain.NodeStatusLabel(n.Status), confirm})
	}
	help := [][]any{
		{"Template Bagan " + v.Label},
		{"Ubah sheet Bagan, lalu unggah lewat Struktur Org → Impor versi. File ini tidak mengubah versi " + v.Label + "."},
		{""},
		{"Kolom", "Isi"},
		{"Kode", "Wajib, unik. Huruf besar, angka, dan tanda hubung (contoh RTL-054)."},
		{"Kode Atasan", "Kosong untuk puncak bagan; selain itu kode lain di file ini."},
		{"Jabatan", "Wajib, maksimal 150 karakter."},
		{"Posisi", "Ya untuk slot jabatan; Tidak untuk node grup (misalnya Support)."},
		{"Kode Divisi", "Wajib, 2–6 huruf (contoh RTL)."},
		{"Divisi", "Wajib, nama divisi."},
		{"Level", "Kosong atau L0 sampai L8. Node grup tanpa level."},
		{"Jenis Kerja", "Kosong, Tetap/Kontrak, Magang, Freelance, atau Outsource (Outsource wajib isi Vendor)."},
		{"Vendor", "Nama vendor outsource."},
		{"Region", "Contoh Makassar, Jabodetabek 1."},
		{"Toko", "Kode toko dipisah koma, contoh PPMR, PLMR."},
		{"Status", "Aktif, Rencana, Kosong, atau Dibekukan (kosong = Aktif)."},
		{"Perlu Konfirmasi", "Ya bila masih menunggu jawaban klien."},
	}
	data, err := xlsx.Build(
		xlsx.SheetSpec{Name: domain.OrgChartSheet, Rows: rows, ColumnWidths: []float64{11, 12, 34, 8, 11, 22, 7, 15, 16, 16, 18, 11, 14},
			TextColumns: len(domain.OrgChartHeaders)},
		xlsx.SheetSpec{Name: "Petunjuk", Rows: help, ColumnWidths: []float64{18, 90}},
	)
	if err != nil {
		return nil, err
	}
	return &exportFile{data, xlsx.ContentType, "Template Bagan " + v.Label + ".xlsx"}, nil
}

// GET /api/hris/org-chart/template?version=
func (h handlers) orgChartTemplateFile(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	q := h.svc.repo.querier()
	v, err := orgVersionOf(r.Context(), q, strings.TrimSpace(r.URL.Query().Get("version")))
	if err != nil {
		return err
	}
	nodes, err := orgNodes(r.Context(), q, v.ID)
	if err != nil {
		return err
	}
	file, err := orgChartTemplate(v, nodes)
	if err != nil {
		return err
	}
	return writeFile(w, file)
}

type orgImportInput struct {
	Data        []byte
	FileName    string
	Label, Base string
	Step        string // preview | error_file | commit
	Fingerprint string
	AckWarnings bool
}

// orgImportFingerprint pins the file, the label and the base version.
func orgImportFingerprint(data []byte, label, baseID string) string {
	h := sha256.New()
	h.Write(data)
	fmt.Fprintf(h, "|%s|%s", strings.ToLower(label), baseID)
	return hex.EncodeToString(h.Sum(nil))
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// importOrgChart runs one step of the Bagan import (EPIC-103 R3–R7, R10).
func (s *Service) importOrgChart(r *http.Request, u *auth.User, in orgImportInput) (map[string]any, *exportFile, error) {
	switch in.Step {
	case "preview", "commit", "error_file":
	default:
		return nil, nil, httpx.BadRequest("Langkah import tidak dikenal")
	}
	label := strings.TrimSpace(in.Label)
	switch {
	case label == "":
		return nil, nil, httpx.BadRequest("Label versi wajib diisi")
	case utf8.RuneCountInString(label) > 40:
		return nil, nil, httpx.BadRequest("Label versi maksimal 40 karakter")
	}
	if !strings.HasSuffix(strings.ToLower(in.FileName), ".xlsx") {
		return nil, nil, httpx.BadRequest(errOrgUploadPick)
	}
	matrix, err := xlsx.ParseMatrix(in.Data, xlsx.ReadOptions{PreferSheet: domain.OrgChartSheet})
	if err != nil {
		return nil, nil, httpx.BadRequest(errOrgUploadPick)
	}
	ctx := r.Context()
	var out map[string]any
	var errFile *exportFile
	err = s.repo.InTx(ctx, func(repo Repository) error {
		q := repo.querier()
		var taken bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM hris.org_chart_versions WHERE lower(label) = lower($1))`, label).Scan(&taken); err != nil {
			return err
		}
		if taken {
			return httpx.BadRequest(errOrgLabelTaken)
		}
		base, err := orgVersionOf(ctx, q, strings.TrimSpace(in.Base))
		if err != nil {
			return err
		}
		emps, aliases, stores, err := orgPeople(ctx, q, domain.TodayWIB(s.now()))
		if err != nil {
			return err
		}
		res, problem := domain.ParseOrgChartUpload(matrix, stores)
		if problem != "" {
			return httpx.BadRequest(problem)
		}
		if in.Step == "error_file" {
			data, err := errorWorkbook(matrix, res.HeaderLine-1, append(append([]domain.UploadIssue{}, res.Errors...), res.Warnings...), domain.OrgChartSheet)
			if err != nil {
				return err
			}
			errFile = &exportFile{data, xlsx.ContentType, "Error - " + in.FileName}
			return nil
		}
		baseNodes, err := orgNodes(ctx, q, base.ID)
		if err != nil {
			return err
		}
		next := make([]domain.OrgNode, len(res.Nodes))
		positions := 0
		for i, n := range res.Nodes {
			next[i] = n.OrgNode
			if n.IsPosition {
				positions++
			}
		}
		baseUpload := make([]domain.OrgUploadNode, len(baseNodes))
		for i, n := range domainNodes(baseNodes) {
			baseUpload[i] = domain.OrgUploadNode{OrgNode: n, DivisionName: baseNodes[i].DivisionName, NeedsConfirm: baseNodes[i].NeedsConfirm}
		}
		diff := domain.DiffOrgUpload(baseUpload, res.Nodes)
		var vac orgVacancySummary
		for _, v := range domain.OrgVacancies(next, domain.MatchOrgChart(next, emps, aliases, stores)) {
			vac.Positions++
			switch v.State {
			case domain.VacancyFilled:
				vac.Filled++
			case domain.VacancyEmpty:
				vac.Empty++
			case domain.VacancyMaybe:
				vac.Maybe++
			case domain.VacancyPlanned:
				vac.Planned++
			case domain.VacancyFrozen:
				vac.Frozen++
			}
		}
		fp := orgImportFingerprint(in.Data, label, base.ID)
		if in.Step == "preview" {
			out = map[string]any{"label": label, "base": map[string]any{"label": base.Label},
				"summary": map[string]int{"nodes": len(res.Nodes), "positions": positions, "errors": len(res.Errors), "warnings": len(res.Warnings)},
				"errors":  res.Errors, "warnings": res.Warnings, "diff": diff, "vacancy": vac, "fingerprint": fp}
			return nil
		}
		switch {
		case len(res.Errors) > 0:
			return httpx.BadRequest("File masih punya error — perbaiki lalu unggah ulang")
		case len(res.Warnings) > 0 && !in.AckWarnings:
			return httpx.BadRequest(`Centang "Saya sudah memeriksa peringatan" sebelum menyimpan`)
		case fp != in.Fingerprint:
			return httpx.Conflict(errOrgFingerprint)
		}
		var id string
		err = q.QueryRow(ctx, `INSERT INTO hris.org_chart_versions (label, source_file, status) VALUES ($1, $2, 'draft') RETURNING id::text`,
			label, in.FileName).Scan(&id)
		if database.IsUniqueViolation(err) {
			return httpx.Conflict(errOrgLabelTaken)
		}
		if err != nil {
			return err
		}
		if err := insertOrgNodes(ctx, q, id, res.Nodes); err != nil {
			return err
		}
		name := u.FullName
		if err := audit.Write(ctx, q, audit.Entry{
			ActorID: u.ID, ActorName: &name,
			Action: "hris.org_chart.version.import", Entity: "hris.org_chart_versions", EntityID: &id, EntityLabel: &label,
			After: map[string]any{"label": label, "source_file": in.FileName, "nodes": len(res.Nodes), "positions": positions,
				"base": base.Label, "added": len(diff.Added), "removed": len(diff.Removed), "changed": len(diff.Changed)},
		}.WithRequest(r)); err != nil {
			return err
		}
		vs, err := orgVersions(ctx, q)
		if err != nil {
			return err
		}
		for _, v := range vs {
			if v.ID == id {
				out = map[string]any{"version": v,
					"message": fmt.Sprintf("Versi %s disimpan sebagai draft (%d node)", label, len(res.Nodes))}
			}
		}
		return nil
	})
	return out, errFile, err
}

// POST /api/hris/org-chart/import (multipart): file, label, base, step =
// preview | error_file | commit, fingerprint, ack_warnings.
func (h handlers) orgChartImport(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxOrgChartUpload+1<<20)
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return httpx.BadRequest("File terlalu besar — maksimal 16 MB")
		}
		return httpx.BadRequest(errOrgUploadPick)
	}
	in := orgImportInput{Label: r.FormValue("label"), Base: r.FormValue("base"), Step: r.FormValue("step"),
		Fingerprint: r.FormValue("fingerprint"), AckWarnings: r.FormValue("ack_warnings") == "true"}
	file, header, err := r.FormFile("file")
	if err != nil {
		return httpx.BadRequest(errOrgUploadPick)
	}
	defer file.Close()
	if in.Data, err = io.ReadAll(io.LimitReader(file, maxOrgChartUpload+1)); err != nil {
		return err
	}
	if len(in.Data) > maxOrgChartUpload {
		return httpx.BadRequest("File terlalu besar — maksimal 16 MB")
	}
	in.FileName = header.Filename
	out, errFile, err := h.svc.importOrgChart(r, u, in)
	if err != nil {
		return err
	}
	if errFile != nil {
		return writeFile(w, errFile)
	}
	if in.Step == "commit" {
		msg, _ := out["message"].(string)
		return ok(w, dataMsg(map[string]any{"version": out["version"]}, msg))
	}
	return ok(w, dataOf(out))
}

// DELETE /api/hris/org-chart/versions/{id}: a draft only (R8).
func (h handlers) orgChartDeleteDraft(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	id := r.PathValue("id")
	if !domain.IsUUID(id) {
		return httpx.NotFound(errOrgVersionNotFound)
	}
	ctx := r.Context()
	var label string
	err := h.svc.repo.InTx(ctx, func(repo Repository) error {
		q := repo.querier()
		var status string
		var nodes int
		err := q.QueryRow(ctx, `SELECT label, status FROM hris.org_chart_versions WHERE id = $1 FOR UPDATE`, id).Scan(&label, &status)
		if database.IsNoRows(err) {
			return httpx.NotFound(errOrgVersionNotFound)
		}
		if err != nil {
			return err
		}
		if status != "draft" {
			return httpx.Conflict(errOrgOnlyDraftDel)
		}
		// Lock every version so two deletes cannot both remove "the other".
		var total int
		if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT id FROM hris.org_chart_versions ORDER BY id FOR UPDATE) v`).Scan(&total); err != nil {
			return err
		}
		if total <= 1 {
			return httpx.Conflict("Versi terakhir tidak bisa dihapus — impor versi lain dulu")
		}
		if err := q.QueryRow(ctx, `SELECT count(*) FROM hris.org_chart_nodes WHERE version_id = $1`, id).Scan(&nodes); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `DELETE FROM hris.org_chart_versions WHERE id = $1 AND status = 'draft'`, id); err != nil {
			return err
		}
		name := u.FullName
		return audit.Write(ctx, q, audit.Entry{
			ActorID: u.ID, ActorName: &name,
			Action: "hris.org_chart.version.delete", Entity: "hris.org_chart_versions", EntityID: &id, EntityLabel: &label,
			Before: map[string]any{"label": label, "status": status, "nodes": nodes},
		}.WithRequest(r))
	})
	if err != nil {
		return err
	}
	return ok(w, dataMsg(map[string]any{"id": id, "label": label}, "Versi "+label+" dihapus"))
}

// insertOrgNodes writes every node of a new version in one statement
// (EPIC-104 R4): 1000 nodes × 16 parameters stays under Postgres' 65535.
func insertOrgNodes(ctx context.Context, q database.Querier, versionID string, nodes []domain.OrgUploadNode) error {
	if len(nodes) == 0 {
		return nil
	}
	const cols = 16
	var sb strings.Builder
	sb.WriteString(`INSERT INTO hris.org_chart_nodes (version_id, code, parent_code, sort_order, title, raw_title,
		is_position, division_code, division_name, job_level, level_confidence, employment_type, vendor, region,
		store_codes, status, needs_confirm, flags) VALUES `)
	args := make([]any, 0, len(nodes)*cols)
	for i, n := range nodes {
		if i > 0 {
			sb.WriteString(", ")
		}
		b := i * cols
		fmt.Fprintf(&sb, "($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, '{}')",
			b+1, b+2, b+3, b+4, b+5, b+5, b+6, b+7, b+8, b+9, b+10, b+11, b+12, b+13, b+14, b+15, b+16)
		var confidence *string
		if n.IsPosition {
			m := "manual"
			confidence = &m
		}
		stores := n.StoreCodes
		if stores == nil {
			stores = []string{}
		}
		args = append(args, versionID, n.Code, nullIfEmpty(n.ParentCode), i+1, n.Title, n.IsPosition, n.DivisionCode,
			n.DivisionName, nullIfEmpty(n.Level), confidence, nullIfEmpty(n.EmploymentType), nullIfEmpty(n.Vendor),
			nullIfEmpty(n.Region), stores, n.Status, n.NeedsConfirm)
	}
	_, err := q.Exec(ctx, sb.String(), args...)
	return err
}

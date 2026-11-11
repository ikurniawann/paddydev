package hris

import (
	"net/http"
	"strings"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/xlsx"
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
		xlsx.SheetSpec{Name: domain.OrgChartSheet, Rows: rows, ColumnWidths: []float64{11, 12, 34, 8, 11, 22, 7, 15, 16, 16, 18, 11, 14}},
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

package hris

import (
	"strings"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/xlsx"
)

// errorWorkbook is the uploaded sheet with a "Keterangan Error" column on
// the right (Spec §3.1): every message of a row joined, the header kept.
func errorWorkbook(matrix [][]string, headerRow int, issues []domain.UploadIssue, sheet string) ([]byte, error) {
	byLine := map[int][]string{}
	var general []string
	for _, i := range issues {
		msg := i.Message
		if i.Column != "" {
			msg = i.Column + ": " + msg
		}
		if i.Line == 0 {
			general = append(general, msg)
			continue
		}
		byLine[i.Line] = append(byLine[i.Line], msg)
	}
	width := 0
	for _, r := range matrix {
		width = max(width, len(r))
	}
	rows := make([][]any, 0, len(matrix)+1)
	if len(general) > 0 {
		rows = append(rows, []any{"Catatan: " + strings.Join(general, "; ")})
	}
	for i, r := range matrix {
		out := make([]any, width+1)
		for j, v := range r {
			out[j] = v
		}
		switch {
		case i == headerRow:
			out[width] = "Keterangan Error"
		default:
			out[width] = strings.Join(byLine[i+1], "; ")
		}
		rows = append(rows, out)
	}
	widths := make([]float64, width+1)
	widths[width] = 60
	return xlsx.Build(xlsx.SheetSpec{Name: sheet, Rows: rows, ColumnWidths: widths})
}

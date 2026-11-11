// Package xlsx writes and reads workbooks the way the TS exports and imports
// do with ExcelJS (lib/spreadsheet/exceljs-safe.ts, lib/pos/report-excel,
// lib/purchasing/import-spreadsheet.ts), plus the formula-safe CSV of
// lib/recruitment/candidate-csv.ts.
package xlsx

import (
	"bytes"
	"fmt"

	"github.com/xuri/excelize/v2"
)

// ContentType is the .xlsx MIME type the export routes send.
const ContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// Merge spans cells by 0-based row and column, like xlsx's {s, e}.
type Merge struct{ R1, C1, R2, C2 int }

// SheetSpec is one sheet of Build: rows of string, number, bool or nil
// (nil is written as ""), widths in characters, merges.
type SheetSpec struct {
	Name         string
	Rows         [][]any
	ColumnWidths []float64
	Merges       []Merge
	// TextColumns formats the first N columns as Text ("@"), so values typed
	// later in Excel (codes like 0123) stay text.
	TextColumns int
}

// Build is buildXlsxBuffer: plain sheets with column widths and merges.
func Build(sheets ...SheetSpec) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	for i, spec := range sheets {
		name := spec.Name
		if name == "" {
			name = "Sheet1"
		}
		if err := addSheet(f, i, name); err != nil {
			return nil, err
		}
		if spec.TextColumns > 0 {
			style, err := f.NewStyle(&excelize.Style{NumFmt: 49})
			if err != nil {
				return nil, err
			}
			last, _ := excelize.ColumnNumberToName(spec.TextColumns)
			if err := f.SetColStyle(name, "A:"+last, style); err != nil {
				return nil, err
			}
		}
		for r, row := range spec.Rows {
			for c, v := range row {
				if v == nil {
					v = ""
				}
				if err := f.SetCellValue(name, cellName(c, r), v); err != nil {
					return nil, err
				}
			}
		}
		for c, w := range spec.ColumnWidths {
			if w > 0 {
				col, _ := excelize.ColumnNumberToName(c + 1)
				if err := f.SetColWidth(name, col, col, w); err != nil {
					return nil, err
				}
			}
		}
		for _, m := range spec.Merges {
			if err := f.MergeCell(name, cellName(m.C1, m.R1), cellName(m.C2, m.R2)); err != nil {
				return nil, err
			}
		}
	}
	return writeBytes(f)
}

// addSheet names the default first sheet or appends a new one.
func addSheet(f *excelize.File, index int, name string) error {
	if index == 0 {
		return f.SetSheetName("Sheet1", name)
	}
	_, err := f.NewSheet(name)
	return err
}

// cellName is the A1 name of a 0-based column and row.
func cellName(col, row int) string {
	name, _ := excelize.CoordinatesToCellName(col+1, row+1)
	return name
}

func writeBytes(f *excelize.File) ([]byte, error) {
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("xlsx: write: %w", err)
	}
	return buf.Bytes(), nil
}

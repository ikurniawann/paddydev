package xlsx

import (
	"fmt"
	"time"

	"github.com/xuri/excelize/v2"
)

// Template workbooks for import/export files people fill in by hand
// (Spec 2026-10-19 §3): human headers, required marks, dropdowns from a
// hidden "Pilihan" sheet, text-formatted long numbers, DD/MM/YYYY dates,
// frozen header, optional sheet protection without a password.

type ColumnKind int

const (
	KindText ColumnKind = iota
	KindDate
	KindList
	KindNumber
	KindFormula
)

const RequiredMark = " *"

type TemplateColumn struct {
	Header   string
	Required bool
	Width    float64
	Kind     ColumnKind
	Options  []string
	ReadOnly bool
	Fill     string
	Formula  func(row int) string
}

type TemplateSheet struct {
	Name       string
	TitleLines []string
	Columns    []TemplateColumn
	Rows       [][]any
	FreezeCols int
	DataRows   int
	Protect    bool
	CellFill   map[string]string
}

type InfoSheet struct {
	Name  string
	Pairs [][2]string
}

const optionsSheet = "Pilihan"

// HeaderRow is the 0-based row of the header (after the title lines).
func HeaderRow(s TemplateSheet) int { return len(s.TitleLines) }

// BuildTemplate writes notes sheets first, then the data sheets, then the
// hidden option and info sheets.
func BuildTemplate(sheets []TemplateSheet, notes []SheetSpec, info *InfoSheet) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	index := 0
	for _, n := range notes {
		if err := addSheet(f, index, n.Name); err != nil {
			return nil, err
		}
		index++
		for r, row := range n.Rows {
			for c, v := range row {
				if v == nil {
					v = ""
				}
				if err := f.SetCellValue(n.Name, cellName(c, r), v); err != nil {
					return nil, err
				}
			}
		}
		for c, w := range n.ColumnWidths {
			col, _ := excelize.ColumnNumberToName(c + 1)
			_ = f.SetColWidth(n.Name, col, col, w)
		}
	}
	var dropdowns []dropdown
	styles, err := newTemplateStyles(f)
	if err != nil {
		return nil, err
	}
	for _, s := range sheets {
		if err := addSheet(f, index, s.Name); err != nil {
			return nil, err
		}
		index++
		if err := writeTemplateSheet(f, s, styles, &dropdowns); err != nil {
			return nil, err
		}
	}
	// The options sheet goes after the data sheets so people see their
	// sheets first; validations point at it by name.
	if len(dropdowns) > 0 {
		if _, err := f.NewSheet(optionsSheet); err != nil {
			return nil, err
		}
		for i, d := range dropdowns {
			if err := addDropdown(f, d, i); err != nil {
				return nil, err
			}
		}
		_ = f.SetSheetVisible(optionsSheet, false)
	}
	if info != nil {
		if _, err := f.NewSheet(info.Name); err != nil {
			return nil, err
		}
		for r, p := range info.Pairs {
			_ = f.SetCellStr(info.Name, cellName(0, r), p[0])
			_ = f.SetCellStr(info.Name, cellName(1, r), p[1])
		}
		_ = f.SetSheetVisible(info.Name, false)
	}
	// Open on the first data sheet when there are notes, else the first sheet.
	if idx, err := f.GetSheetIndex(sheets[0].Name); err == nil && len(notes) == 0 {
		f.SetActiveSheet(idx)
	}
	return writeBytes(f)
}

type templateStyles struct {
	title, header, required, readOnlyHeader int
	text, date, number                      map[string]int // by fill ("" = none), editable
	textLocked, dateLocked, numberLocked    map[string]int
}

func newTemplateStyles(f *excelize.File) (*templateStyles, error) {
	st := &templateStyles{text: map[string]int{}, date: map[string]int{}, number: map[string]int{},
		textLocked: map[string]int{}, dateLocked: map[string]int{}, numberLocked: map[string]int{}}
	var err error
	if st.title, err = f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 13}}); err != nil {
		return nil, err
	}
	head := func(fill string) (int, error) {
		return f.NewStyle(&excelize.Style{
			Font:       &excelize.Font{Bold: true, Color: "#111827"},
			Fill:       excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{fill}},
			Alignment:  &excelize.Alignment{WrapText: true, Vertical: "center"},
			Border:     []excelize.Border{{Type: "bottom", Color: "#9CA3AF", Style: 1}},
			Protection: &excelize.Protection{Locked: true},
		})
	}
	if st.header, err = head("#E5E7EB"); err != nil {
		return nil, err
	}
	if st.required, err = head("#FDE68A"); err != nil {
		return nil, err
	}
	if st.readOnlyHeader, err = head("#F3F4F6"); err != nil {
		return nil, err
	}
	return st, nil
}

// cellStyle returns (and caches) the style for a data cell of kind with an
// optional fill; locked cells stay locked when the sheet is protected.
func (st *templateStyles) cellStyle(f *excelize.File, kind ColumnKind, fill string, locked bool) (int, error) {
	cache := map[ColumnKind]map[string]int{KindText: st.text, KindList: st.text, KindDate: st.date, KindNumber: st.number, KindFormula: st.number}
	if locked {
		cache = map[ColumnKind]map[string]int{KindText: st.textLocked, KindList: st.textLocked, KindDate: st.dateLocked, KindNumber: st.numberLocked, KindFormula: st.numberLocked}
	}
	m := cache[kind]
	if id, ok := m[fill]; ok {
		return id, nil
	}
	s := &excelize.Style{Protection: &excelize.Protection{Locked: locked}}
	switch kind {
	case KindDate:
		code := "dd/mm/yyyy"
		s.CustomNumFmt = &code
	case KindText, KindList:
		s.NumFmt = 49 // "@" text: keeps leading zeros and 16-digit NIK
	}
	if fill != "" {
		s.Fill = excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{fill}}
	}
	id, err := f.NewStyle(s)
	if err == nil {
		m[fill] = id
	}
	return id, err
}

// dropdown is a list validation applied once the options sheet exists.
type dropdown struct {
	sheet         string
	col, from, to int
	options       []string
}

func writeTemplateSheet(f *excelize.File, s TemplateSheet, st *templateStyles, dropdowns *[]dropdown) error {
	name := s.Name
	for i, line := range s.TitleLines {
		if err := f.SetCellStr(name, cellName(0, i), line); err != nil {
			return err
		}
		_ = f.SetCellStyle(name, cellName(0, i), cellName(0, i), st.title)
	}
	h := HeaderRow(s)
	last := h + max(len(s.Rows), s.DataRows) // last 0-based data row
	for c, col := range s.Columns {
		header := col.Header
		style := st.header
		switch {
		case col.Required:
			header += RequiredMark
			style = st.required
		case col.ReadOnly:
			style = st.readOnlyHeader
		}
		ref := cellName(c, h)
		if err := f.SetCellStr(name, ref, header); err != nil {
			return err
		}
		_ = f.SetCellStyle(name, ref, ref, style)
		if col.ReadOnly {
			_ = f.AddComment(name, excelize.Comment{Cell: ref, Author: "HRIS",
				Paragraph: []excelize.RichTextRun{{Text: "Hanya informasi, tidak dibaca saat import."}}})
		}
		width := col.Width
		if width == 0 {
			width = float64(max(12, len([]rune(header))+2))
		}
		colName, _ := excelize.ColumnNumberToName(c + 1)
		_ = f.SetColWidth(name, colName, colName, width)

		locked := s.Protect && (col.ReadOnly || col.Kind == KindFormula)
		styleID, err := st.cellStyle(f, col.Kind, col.Fill, locked)
		if err != nil {
			return err
		}
		if last > h {
			_ = f.SetCellStyle(name, cellName(c, h+1), cellName(c, last), styleID)
		}
		if col.Kind == KindList && len(col.Options) > 0 && last > h {
			*dropdowns = append(*dropdowns, dropdown{name, c, h + 1, last, col.Options})
		}
	}
	for r, row := range s.Rows {
		excelRow := h + 1 + r
		for c, v := range row {
			if c >= len(s.Columns) {
				break
			}
			col := s.Columns[c]
			ref := cellName(c, excelRow)
			var err error
			switch {
			case col.Kind == KindFormula && col.Formula != nil:
				err = f.SetCellFormula(name, ref, col.Formula(excelRow+1))
			case v == nil:
				continue
			case col.Kind == KindDate:
				if t, ok := v.(time.Time); ok {
					err = f.SetCellValue(name, ref, t)
				} else {
					err = f.SetCellStr(name, ref, fmt.Sprint(v))
				}
			case col.Kind == KindNumber:
				err = f.SetCellValue(name, ref, v)
			default:
				err = f.SetCellStr(name, ref, fmt.Sprint(v))
			}
			if err != nil {
				return err
			}
		}
	}
	// Formula columns on template rows without data.
	for c, col := range s.Columns {
		if col.Kind != KindFormula || col.Formula == nil {
			continue
		}
		for r := h + 1 + len(s.Rows); r <= last; r++ {
			_ = f.SetCellFormula(name, cellName(c, r), col.Formula(r+1))
		}
	}
	for value, color := range s.CellFill {
		fmtID, err := f.NewConditionalStyle(&excelize.Style{Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{color}}})
		if err != nil {
			return err
		}
		rangeRef := cellName(0, h+1) + ":" + cellName(len(s.Columns)-1, max(last, h+1))
		_ = f.SetConditionalFormat(name, rangeRef, []excelize.ConditionalFormatOptions{{
			Type: "cell", Criteria: "==", Format: &fmtID, Value: `"` + value + `"`}})
	}
	_ = f.SetPanes(name, &excelize.Panes{Freeze: true, XSplit: s.FreezeCols, YSplit: h + 1,
		TopLeftCell: cellName(s.FreezeCols, h+1), ActivePane: "bottomRight"})
	if s.Protect {
		return f.ProtectSheet(name, &excelize.SheetProtectionOptions{
			SelectLockedCells: true, SelectUnlockedCells: true, FormatColumns: true, AutoFilter: true})
	}
	return nil
}

// addDropdown writes the options into a column of the hidden options sheet
// and points a list validation at it (no 255-character limit).
func addDropdown(f *excelize.File, d dropdown, optCol int) error {
	letter, _ := excelize.ColumnNumberToName(optCol + 1)
	for i, o := range d.options {
		if err := f.SetCellStr(optionsSheet, cellName(optCol, i), o); err != nil {
			return err
		}
	}
	dv := excelize.NewDataValidation(true)
	dv.Sqref = cellName(d.col, d.from) + ":" + cellName(d.col, d.to)
	dv.SetSqrefDropList(fmt.Sprintf("'%s'!$%s$1:$%s$%d", optionsSheet, letter, letter, len(d.options)))
	dv.SetError(excelize.DataValidationErrorStyleStop, "Pilihan tidak tersedia", "Pilih salah satu nilai dari daftar.")
	return f.AddDataValidation(d.sheet, dv)
}

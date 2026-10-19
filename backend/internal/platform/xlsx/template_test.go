package xlsx

import (
	"bytes"
	"strconv"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func itoa(n int) string                  { return strconv.Itoa(n) }
func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }

func TestBuildTemplate(t *testing.T) {
	data := TemplateSheet{
		Name: "Data Karyawan",
		Columns: []TemplateColumn{
			{Header: "NIP", Kind: KindText, Width: 14},
			{Header: "Nama Lengkap", Required: true, Kind: KindText, Width: 28},
			{Header: "Tanggal Masuk", Required: true, Kind: KindDate, Width: 14},
			{Header: "Tipe Jadwal", Required: true, Kind: KindList, Options: []string{"Shift", "Non-Shift"}},
			{Header: "Status Aktif", ReadOnly: true, Kind: KindText},
		},
		Rows:       [][]any{{"0202505272", "Feti", time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC), "Shift", "Aktif"}},
		FreezeCols: 2,
		DataRows:   50,
	}
	notes := []SheetSpec{{Name: "Petunjuk", Rows: [][]any{{"Cara mengisi"}}}}
	out, err := BuildTemplate([]TemplateSheet{data}, notes, &InfoSheet{Name: "Info", Pairs: [][2]string{{"Cabang ID", "abc"}}})
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytesReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if got := f.GetSheetList(); len(got) < 3 || got[0] != "Petunjuk" || got[1] != "Data Karyawan" {
		t.Fatalf("sheets = %v", got)
	}
	if v, _ := f.GetCellValue("Data Karyawan", "B1"); v != "Nama Lengkap *" {
		t.Fatalf("B1 = %q", v)
	}
	if v, _ := f.GetCellValue("Data Karyawan", "A2"); v != "0202505272" {
		t.Fatalf("leading zero lost: %q", v)
	}
	if typ, _ := f.GetCellType("Data Karyawan", "A2"); typ != excelize.CellTypeSharedString && typ != excelize.CellTypeInlineString {
		t.Fatalf("NIP cell type = %v, want text", typ)
	}
	if v, _ := f.GetCellValue("Data Karyawan", "C2"); v != "01/06/2025" {
		t.Fatalf("date shown %q, want 01/06/2025", v)
	}
	dvs, _ := f.GetDataValidations("Data Karyawan")
	if len(dvs) == 0 {
		t.Fatal("no dropdown on Tipe Jadwal")
	}
	if visible, _ := f.GetSheetVisible("Info"); visible {
		t.Fatal("Info sheet must be hidden")
	}
	panes, _ := f.GetPanes("Data Karyawan")
	if !panes.Freeze || panes.XSplit != 2 || panes.YSplit != 1 {
		t.Fatalf("panes = %+v", panes)
	}
}

func TestBuildTemplateProtectsOnlyFixedColumns(t *testing.T) {
	s := TemplateSheet{
		Name:       "Jadwal",
		TitleLines: []string{"Jadwal Kerja — PPJA"},
		Columns: []TemplateColumn{
			{Header: "NIP", ReadOnly: true}, {Header: "Sen 03/11", Kind: KindList, Options: []string{"DEF", "L"}},
			{Header: "Hari Masuk", Kind: KindFormula, Formula: func(r int) string { return "COUNTIF(B" + itoa(r) + ",\"<>L\")" }},
		},
		Rows:    [][]any{{"202505272", "DEF", nil}},
		Protect: true,
	}
	out, err := BuildTemplate([]TemplateSheet{s}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	f, _ := excelize.OpenReader(bytesReader(out))
	if HeaderRow(s) != 1 {
		t.Fatalf("header row = %d", HeaderRow(s))
	}
	if v, _ := f.GetCellValue("Jadwal", "A2"); v != "NIP" {
		t.Fatalf("A2 = %q", v)
	}
	if fx, _ := f.GetCellFormula("Jadwal", "C3"); fx == "" {
		t.Fatal("formula missing")
	}
	lockedB := cellLocked(t, f, "Jadwal", "B3")
	lockedA := cellLocked(t, f, "Jadwal", "A3")
	if lockedB || !lockedA {
		t.Fatalf("locks: A3=%v B3=%v, want A locked, B editable", lockedA, lockedB)
	}
}

func cellLocked(t *testing.T, f *excelize.File, sheet, ref string) bool {
	t.Helper()
	id, _ := f.GetCellStyle(sheet, ref)
	st, err := f.GetStyle(id)
	if err != nil {
		t.Fatal(err)
	}
	return st.Protection == nil || st.Protection.Locked
}

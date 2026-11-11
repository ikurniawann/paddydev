package xlsx

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func TestBuildRoundTrip(t *testing.T) {
	data, err := Build(SheetSpec{
		Name:         "Rekap Absensi",
		Rows:         [][]any{{"PT Contoh — Rekap"}, {}, {"Nama", "Jam", "Hadir"}, {" Budi ", 7.5, true}, {"Ani", nil, false}},
		ColumnWidths: []float64{18, 0, 9},
		Merges:       []Merge{{R1: 0, C1: 0, R2: 0, C2: 2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	f, _ := excelize.OpenReader(bytes.NewReader(data))
	defer f.Close()
	if w, _ := f.GetColWidth("Rekap Absensi", "A"); w != 18 {
		t.Errorf("width A = %v", w)
	}
	if m, _ := f.GetMergeCells("Rekap Absensi"); len(m) != 1 || m[0].GetStartAxis() != "A1" || m[0].GetEndAxis() != "C1" {
		t.Errorf("merges %v", m)
	}
	got, err := ParseMatrix(data, ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"PT Contoh — Rekap", "", ""}, {"", "", ""}, {"Nama", "Jam", "Hadir"}, {"Budi", "7.5", "true"}, {"Ani", "", "false"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
	if _, err := ParseMatrix(data, ReadOptions{Limits: Limits{MaxBytes: 10}}); err != ErrTooLarge {
		t.Fatal(err)
	}
	if got, _ := ParseMatrix(data, ReadOptions{Limits: Limits{MaxRows: 1, MaxCols: 2}}); !reflect.DeepEqual(got, [][]string{{"PT Contoh — Rekap"}}) {
		t.Fatalf("limits %q", got)
	}
	parts, err := ParseCSVParts(data, Limits{})
	if err != nil || len(parts) != 1 || parts[0].CSV != "PT Contoh — Rekap\nNama,Jam,Hadir\nBudi,7.5,true\nAni,,false" {
		t.Fatalf("%+v %v", parts, err)
	}
}

func TestReport(t *testing.T) {
	r := NewReport(time.Date(2026, 10, 5, 7, 0, 0, 0, time.UTC))
	s := r.Sheet("Ringkasan")
	row := s.TitleBlock("Laporan Penjualan", "Periode: Oktober 2026")
	row = s.KeyValues(row, []Pair{{"Omzet", 1250000, FmtRp}, {"Catatan", "ok", ""}})
	row = s.SectionTitle(row, "Per produk")
	next := s.Table(row, []Col{{Header: "Produk"}, {Header: "Qty", Fmt: FmtQty}, {Header: "Total", Fmt: FmtRp, Width: 16}},
		[][]any{{"Kopi", 3, 45000}, {"Teh", 2.5, 20000}}, TableOpts{Totals: []any{"Total", nil, 65000}, FreezeHeader: true})
	if row != 8 || next != 13 {
		t.Fatalf("rows %d %d", row, next)
	}
	data, err := r.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	f, _ := excelize.OpenReader(bytes.NewReader(data))
	defer f.Close()
	if props, _ := f.GetDocProps(); props.Creator != "Paddy OS" {
		t.Errorf("creator %q", props.Creator)
	}
	styleOf := func(ref string) *excelize.Style {
		id, _ := f.GetCellStyle("Ringkasan", ref)
		st, _ := f.GetStyle(id)
		return st
	}
	if st := styleOf("B4"); st.CustomNumFmt == nil || *st.CustomNumFmt != `"Rp "#,##0` || !st.Font.Bold {
		t.Errorf("rp value style %+v", st)
	}
	if st := styleOf("A8"); st.Fill.Color[0] != "1F2937" || st.Font.Color != "FFFFFF" || st.Alignment.Horizontal != "left" {
		t.Errorf("header style %+v", st)
	}
	if st := styleOf("B8"); st.Alignment.Horizontal != "right" {
		t.Errorf("numeric header aligns %q", st.Alignment.Horizontal)
	}
	if st := styleOf("A10"); len(st.Fill.Color) == 0 || st.Fill.Color[0] != "F9FAFB" {
		t.Errorf("zebra %+v", st.Fill)
	}
	if st := styleOf("C11"); !st.Font.Bold || st.Fill.Color[0] != "E5E7EB" {
		t.Errorf("totals %+v", st)
	}
	if w, _ := f.GetColWidth("Ringkasan", "A"); w != 30 {
		t.Errorf("widen A %v", w)
	}
	if p, _ := f.GetPanes("Ringkasan"); !p.Freeze || p.YSplit != 8 {
		t.Errorf("panes %+v", p)
	}
}

func TestCSV(t *testing.T) {
	got := ToCSV([][]any{{"Nama", "Catatan"}, {`=HYPERLINK("x")`, "-5"}, {"@a", 1.5}, {nil, "a\tb"}, {"\tx", `say "hi"`}})
	want := "\"Nama\",\"Catatan\"\n\"'=HYPERLINK(\"\"x\"\")\",\"'-5\"\n\"'@a\",\"1.5\"\n\"\",\"a\tb\"\n\"'\tx\",\"say \"\"hi\"\"\""
	if got != want {
		t.Fatalf("got %q", got)
	}
	m := ParseCSVMatrix("Nama Supplier, Kota\r\n\n\"PT A, Tbk\",Bandung\r\n")
	if !reflect.DeepEqual(m, [][]string{{"Nama Supplier", "Kota"}, {"PT A, Tbk", "Bandung"}}) {
		t.Fatalf("%q", m)
	}
	norm := HeaderNormalizer(map[string]string{"kota": "city"})
	rows := MatrixToRows(append(m, []string{"PT B"}), norm)
	if rows[0].RowNumber != 2 || rows[0].Data["nama_supplier"] != "PT A, Tbk" || rows[1].Data["city"] != "" {
		t.Fatalf("%+v", rows)
	}
	if !IsBlankRow(map[string]string{"a": " "}, "a", "b") {
		t.Fatal("blank row")
	}
	tenth, fifth := 0.1, 0.2
	for in, want := range map[float64]string{1e21: "1e+21", 1.5e-7: "1.5e-7", 0.000001: "0.000001", tenth + fifth: "0.30000000000000004", -42: "-42", 123456789012: "123456789012"} {
		if got := JSNumber(in); got != want {
			t.Errorf("JSNumber(%v) = %q want %q", in, got, want)
		}
	}
}

// TestExcelJSParity reads workbooks with the real exceljs-safe.ts under
// Node: a workbook ExcelJS writes (dates, booleans, formulas, rich text)
// must parse to the same matrix and CSV in Go, and a workbook Go writes must
// parse the same in ExcelJS. Skips without node or frontend/node_modules.
func TestExcelJSParity(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	_, here, _, _ := runtime.Caller(0)
	frontend, _ := filepath.Abs(filepath.Join(filepath.Dir(here), "..", "..", "..", "..", "frontend"))
	if _, err := os.Stat(filepath.Join(frontend, "node_modules", "exceljs")); err != nil {
		t.Skip("frontend node_modules not installed")
	}
	dir := t.TempDir()
	goFile := filepath.Join(dir, "go.xlsx")
	goData, _ := Build(SheetSpec{Name: "Data", Rows: [][]any{{"Kode", "Nilai", "Aktif"}, {"007", 1250.5, true}, {nil, -3, false}}})
	_ = os.WriteFile(goFile, goData, 0o644)

	script := `
const [lib, dir, goFile] = process.argv.slice(1);
const ExcelJS = (await import("exceljs")).default;
const safe = await import(lib + "/spreadsheet/exceljs-safe.ts");
const fs = await import("node:fs");
const wb = new ExcelJS.Workbook();
const ws = wb.addWorksheet("COA");
ws.addRow(["Kode", "Tanggal", "Jumlah", "Lunas", "Rumus", "Catatan"]);
const r = ws.addRow(["1-100", new Date(Date.UTC(2026, 9, 4, 13, 30)), 0.1 + 0.2, true, { formula: "C2*2", result: 0.6 }, { richText: [{ text: "Kas " }, { text: "besar", font: { bold: true } }] }]);
ws.addRow([]);
ws.addRow(["  spasi  ", null, 1e-8, false, null, "a,\"b\""]);
const other = wb.addWorksheet("Lain");
other.addRow(["x"]);
const buf = Buffer.from(await wb.xlsx.writeBuffer());
fs.writeFileSync(dir + "/exceljs.xlsx", buf);
const matrix = await safe.parseXlsxToMatrix(buf, { pickSheet: (b) => b.worksheets.find((w) => w.name.toLowerCase() === "coa") ?? b.worksheets[0] });
const csv = await safe.parseXlsxToCsvParts(buf);
const goMatrix = await safe.parseXlsxToMatrix(fs.readFileSync(goFile));
console.log(JSON.stringify({ matrix, csv, goMatrix }));
`
	cmd := exec.Command(node, "--no-warnings", "--input-type=module", "-e", script, filepath.Join(frontend, "src", "lib"), dir, goFile)
	cmd.Dir = frontend
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("node: %v %s", err, stderr.String())
	}
	var ts struct {
		Matrix   [][]string
		CSV      []struct{ Name, CSV string }
		GoMatrix [][]string
	}
	if err := json.Unmarshal(out, &ts); err != nil {
		t.Fatal(err)
	}

	excelJSData, _ := os.ReadFile(filepath.Join(dir, "exceljs.xlsx"))
	matrix, err := ParseMatrix(excelJSData, ReadOptions{PreferSheet: "coa"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(matrix, ts.Matrix) {
		t.Errorf("matrix\n go %q\n ts %q", matrix, ts.Matrix)
	}
	parts, _ := ParseCSVParts(excelJSData, Limits{})
	if len(parts) != len(ts.CSV) {
		t.Fatalf("csv parts %d vs %d", len(parts), len(ts.CSV))
	}
	for i := range parts {
		if parts[i].Name != ts.CSV[i].Name || parts[i].CSV != ts.CSV[i].CSV {
			t.Errorf("csv part %d\n go %q\n ts %q", i, parts[i], ts.CSV[i])
		}
	}
	goMatrix, _ := ParseMatrix(goData, ReadOptions{})
	if !reflect.DeepEqual(goMatrix, ts.GoMatrix) {
		t.Errorf("Go workbook\n go %q\n ts %q", goMatrix, ts.GoMatrix)
	}
}

// EPIC-104 R5: TextColumns formats the first N columns as Text ("@").
func TestBuildTextColumns(t *testing.T) {
	data, err := Build(SheetSpec{Name: "Bagan", Rows: [][]any{{"Kode", "Atasan", "Lain"}, {"001", "", "x"}}, TextColumns: 2})
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for col, want := range map[string]bool{"A": true, "B": true, "C": false} {
		id, err := f.GetColStyle("Bagan", col)
		if err != nil {
			t.Fatal(err)
		}
		st, err := f.GetStyle(id)
		if err != nil {
			t.Fatal(err)
		}
		if (st.NumFmt == 49) != want {
			t.Errorf("column %s NumFmt %d", col, st.NumFmt)
		}
	}
	if v, _ := f.GetCellValue("Bagan", "A2"); v != "001" {
		t.Errorf("A2 = %q", v)
	}
}

// Command hris-data-kerja is the one-off ops tool of EPIC-086: bulk work data
// (primary placement, schedule type, weekly pattern, department, position)
// from one Excel file, without a login session.
//
//	hris-data-kerja -mode audit                       # gaps, read-only
//	hris-data-kerja -mode template -out data-kerja.xlsx
//	hris-data-kerja -mode preview -file isi.xlsx      # validation report, read-only
//	hris-data-kerja -mode apply -file isi.xlsx -sha <sha256 from preview>
//
// The database is DATABASE_URL. tools/ops/data-kerja.sh runs it inside the
// paddydev-api container after a backup.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/modules/hris"
	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/database"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("hris-data-kerja", flag.ContinueOnError)
	fs.SetOutput(stderr)
	mode := fs.String("mode", "", "audit | template | preview | apply")
	file := fs.String("file", "", "file Excel terisi (preview/apply)")
	out := fs.String("out", "", "tujuan template (template)")
	sha := fs.String("sha", "", "sha256 file dari pratinjau (apply)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	fail := func(msg string) int { fmt.Fprintln(stderr, msg); return 2 }
	switch *mode {
	case "audit":
	case "template":
		if *out == "" {
			return fail("-out wajib untuk template")
		}
	case "preview", "apply":
		if *file == "" {
			return fail("-file wajib untuk " + *mode)
		}
		if *mode == "apply" && *sha == "" {
			return fail("-sha wajib untuk apply (salin dari hasil pratinjau)")
		}
	default:
		return fail("mode harus audit, template, preview, atau apply")
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return fail("DATABASE_URL kosong")
	}
	ctx := context.Background()
	pool, err := database.Open(ctx, url)
	if err != nil {
		return fail("koneksi database: " + err.Error())
	}
	defer pool.Close()
	// "Hari ini" selalu jam server (WIB): tidak bisa dimundurkan untuk melewati cek tanggal.
	if err := execute(ctx, pool, *mode, *file, *out, *sha, domain.TodayWIB(time.Now()), stdout); err != nil {
		fmt.Fprintln(stderr, "GAGAL:", err)
		return 1
	}
	return 0
}

func execute(ctx context.Context, db database.DB, mode, file, out, sha, today string, w io.Writer) error {
	switch mode {
	case "audit":
		gaps, err := hris.WorkDataAudit(ctx, db, today)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "# Audit data kerja per %s\n\n| Tipe | Aktif | Belum lengkap | Tanpa penempatan utama | Tanpa jadwal | Tanpa departemen | Tanpa jabatan |\n|---|---|---|---|---|---|---|\n", today)
		for _, g := range gaps {
			fmt.Fprintf(w, "| %s | %d | %d | %d | %d | %d | %d |\n", g.ScheduleType, g.Total, g.Any, g.NoPrimary, g.NoSchedule, g.NoDept, g.NoPosition)
		}
		return nil
	case "template":
		data, err := hris.WorkDataTemplate(ctx, db, today)
		if err != nil {
			return err
		}
		if err := os.WriteFile(out, data, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(w, "Template ditulis: %s (%d byte)\n", out, len(data))
		return nil
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	if mode == "preview" {
		rep, err := hris.WorkDataPreview(ctx, db, data, today)
		if err != nil {
			return err
		}
		report(w, rep, "Pratinjau")
		return nil
	}
	var rep *hris.WorkDataReport
	err = database.WithTx(ctx, db, func(tx pgx.Tx) error {
		var err error
		rep, err = hris.WorkDataApply(ctx, tx, data, today, sha)
		return err
	})
	if err != nil {
		return err
	}
	report(w, rep, "DITERAPKAN")
	return nil
}

func report(w io.Writer, rep *hris.WorkDataReport, title string) {
	fmt.Fprintf(w, "# %s data kerja\n\nsha256: %s\n\nBaris valid: %d · error: %d · peringatan: %d\n\n", title, rep.SHA, rep.Valid, rep.Errors, rep.Warnings)
	if len(rep.Issues) > 0 {
		fmt.Fprintln(w, "| Baris | NIP | Pesan |\n|---|---|---|")
		for i, is := range rep.Issues {
			kind := "ERROR"
			if i >= rep.Errors {
				kind = "peringatan"
			}
			fmt.Fprintf(w, "| %d | %s | %s: %s |\n", is.Line, is.Name, kind, is.Message)
		}
		fmt.Fprintln(w)
	}
	for _, n := range rep.Notes {
		fmt.Fprintln(w, "- "+n)
	}
	fmt.Fprintln(w, "## Perubahan per karyawan\n\n| Baris | NIP | Nama | Penempatan | Tipe | Pola | Berlaku | Dept/Jabatan |\n|---|---|---|---|---|---|---|---|")
	yes := func(b bool) string {
		if b {
			return "ya"
		}
		return ""
	}
	for _, r := range rep.Rows {
		tipe := ""
		if r.ScheduleType != nil {
			tipe = *r.ScheduleType
		}
		fmt.Fprintf(w, "| %d | %s | %s | %s | %s | %s | %s | %s |\n", r.Line, r.NIP, r.Name, yes(r.BranchID != nil), tipe,
			yes(r.Pattern != nil), r.From, yes(r.DepartmentID != nil || r.PositionID != nil))
	}
}

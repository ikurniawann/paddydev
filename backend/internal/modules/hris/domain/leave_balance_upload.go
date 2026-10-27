package domain

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Import Saldo Cuti (EPIC-068 R10): read the template sheet, check each row
// and work out which employees' balances actually change. The per-figure
// rules (multiples of 0,5, remaining >= 0, reason, year) live in the hris
// package's validateLeaveBalance; the caller passes it in as Check so the
// import and the manual PUT can never disagree.

// MaxLeaveBalanceRows bounds one upload.
const MaxLeaveBalanceRows = 5000

// Leave-balance template column headers (NormalizeHeader form is matched on
// upload, so case, spacing and a trailing "*" do not matter).
const (
	LeaveColNIP        = "NIP"
	LeaveColName       = "Nama"
	LeaveColBranch     = "Cabang"
	LeaveColUsed       = "Terpakai"
	LeaveColTotal      = "Jatah"
	LeaveColAdjustment = "Penyesuaian"
	LeaveColCarry      = "Sisa Bawaan"
	LeaveColExpires    = "Kedaluwarsa"
	LeaveColReason     = "Alasan"
)

// leaveWarnTotal is the allocation above which HR is asked to confirm.
const leaveWarnTotal = 24

// LeaveBalanceValues is the editable part of one balance.
type LeaveBalanceValues struct {
	Total, Adjustment, CarryDays float64
	CarryExpires                 string // YYYY-MM-DD, "" = none
}

// LeaveBalanceEmployee is what the upload needs to know about one NIP.
type LeaveBalanceEmployee struct {
	ID, Name        string
	Active, InScope bool
	Exists          bool // a balance row is stored for the year
	Current         LeaveBalanceValues
	Used, CarryUsed float64
}

// LeaveBalanceCheck is one allocation handed to the shared validator.
type LeaveBalanceCheck struct {
	Year            int
	Values          LeaveBalanceValues
	Reason          string
	Used, CarryUsed float64
	Active          bool
	// CarryChanged: the request changes the carry-over days or expiry; the
	// carry-over may not fall below CarryUsed only then.
	CarryChanged bool
}

// LeaveBalanceUploadContext is everything the pure validation needs.
type LeaveBalanceUploadContext struct {
	Year       int
	BranchID   string // "" = all branches
	BranchName string
	Today      string                          // YYYY-MM-DD (WIB)
	Employees  map[string]LeaveBalanceEmployee // by NIP; every employee found for the NIPs in the file
	// Check runs the shared allocation rules and returns their message, or "".
	Check func(LeaveBalanceCheck) string
}

// LeaveBalanceFile is the parsed template sheet.
type LeaveBalanceFile struct {
	InfoYear, InfoBranchID string
	InfoFound              bool
	HeaderLine             int
	Rows                   []LeaveBalanceFileRow
	missing                []string
	cols                   leaveColumns
}

// LeaveBalanceFileRow is one non-blank row below the header.
type LeaveBalanceFileRow struct {
	Line                                               int
	NIP, Total, Adjustment, CarryDays, Expires, Reason string
}

type leaveColumns struct{ nip, total, adjustment, carry, expires, reason int }

// LeaveBalanceChange is one balance the commit writes.
type LeaveBalanceChange struct {
	Line          int
	EmployeeID    string
	NIP, Name     string
	New           bool // no stored row yet
	Before, After LeaveBalanceValues
	Reason        string
}

// LeaveBalanceUploadResult is the outcome of ValidateLeaveBalanceUpload.
type LeaveBalanceUploadResult struct {
	Changes     []LeaveBalanceChange
	Errors      []UploadIssue
	Warnings    []UploadIssue
	Same        int      // rows whose figures equal the stored (or default) ones
	EmployeeIDs []string // employees of the rows that resolved, in file order
	Rows        int      // non-blank rows read
}

// ReadLeaveBalanceFile finds the header row (first cell "NIP"), maps the
// columns by their headers and collects the non-blank rows below it. info is
// the hidden Info sheet as a matrix (nil when the sheet is missing).
func ReadLeaveBalanceFile(matrix, info [][]string) (LeaveBalanceFile, string) {
	var f LeaveBalanceFile
	for _, kv := range info {
		if len(kv) < 2 {
			continue
		}
		switch NormalizeHeader(kv[0]) {
		case "tahun":
			f.InfoFound = true
			f.InfoYear = strings.TrimSpace(kv[1])
		case "cabang id":
			f.InfoBranchID = strings.TrimSpace(kv[1])
		}
	}
	header := -1
	for i, r := range matrix {
		if len(r) > 0 && NormalizeHeader(r[0]) == "nip" {
			header = i
			break
		}
	}
	if header < 0 {
		return f, "Judul kolom NIP tidak ditemukan — pakai template dari halaman Saldo Cuti"
	}
	f.HeaderLine = header + 1
	at := map[string]int{}
	for j, h := range matrix[header] {
		if n := NormalizeHeader(h); n != "" {
			if _, dup := at[n]; !dup {
				at[n] = j
			}
		}
	}
	find := func(name string) int {
		if j, ok := at[NormalizeHeader(name)]; ok {
			return j
		}
		f.missing = append(f.missing, name)
		return -1
	}
	f.cols = leaveColumns{nip: find(LeaveColNIP), total: find(LeaveColTotal), adjustment: find(LeaveColAdjustment),
		carry: find(LeaveColCarry), expires: find(LeaveColExpires), reason: find(LeaveColReason)}
	if len(f.missing) > 0 {
		return f, "Kolom " + strings.Join(f.missing, ", ") + " tidak ditemukan — pakai template dari halaman Saldo Cuti"
	}
	cell := func(r []string, j int) string {
		if j >= 0 && j < len(r) {
			return strings.TrimSpace(r[j])
		}
		return ""
	}
	for i := header + 1; i < len(matrix); i++ {
		r := matrix[i]
		row := LeaveBalanceFileRow{Line: i + 1, NIP: cell(r, f.cols.nip), Total: cell(r, f.cols.total),
			Adjustment: cell(r, f.cols.adjustment), CarryDays: cell(r, f.cols.carry),
			Expires: cell(r, f.cols.expires), Reason: cell(r, f.cols.reason)}
		if row.NIP == "" && row.Total == "" && row.Adjustment == "" && row.CarryDays == "" && row.Expires == "" && row.Reason == "" {
			continue
		}
		f.Rows = append(f.Rows, row)
	}
	return f, ""
}

// ParseLeaveDays reads a figure the way HR types it: 9.5 or 9,5.
func ParseLeaveDays(s string) (float64, bool) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

// ValidateLeaveBalanceUpload checks every row of the file. Rows whose figures
// equal the stored (or default) balance are ignored whatever else they hold;
// a row that differs needs a reason and must pass the shared rules. Errors
// block the commit; warnings need the operator's acknowledgement.
func ValidateLeaveBalanceUpload(f LeaveBalanceFile, c LeaveBalanceUploadContext) LeaveBalanceUploadResult {
	var res LeaveBalanceUploadResult
	res.Rows = len(f.Rows)
	fail := func(line int, col, name, msg string) {
		res.Errors = append(res.Errors, UploadIssue{line, col, name, msg})
	}
	warn := func(line int, col, name, msg string) {
		res.Warnings = append(res.Warnings, UploadIssue{line, col, name, msg})
	}
	scope := "semua cabang"
	if c.BranchID != "" {
		scope = "cabang " + c.BranchName
	}
	switch {
	case !f.InfoFound:
		fail(0, "", "", "Sheet Info tidak ditemukan — pakai template xlsx yang diunduh dari halaman Saldo Cuti")
		return res
	case f.InfoYear != strconv.Itoa(c.Year) || f.InfoBranchID != c.BranchID:
		fail(0, "", "", fmt.Sprintf("File ini bukan template saldo cuti %d untuk %s. Unduh template baru dari halaman Saldo Cuti.", c.Year, scope))
		return res
	}
	if len(f.Rows) > MaxLeaveBalanceRows {
		fail(0, "", "", fmt.Sprintf("File terlalu besar (maksimal %d baris)", MaxLeaveBalanceRows))
		return res
	}
	// The year rule is file-wide: say it once instead of on every row. With
	// an active employee, a reason and zero figures it is the only rule left.
	if c.Check != nil {
		if msg := c.Check(LeaveBalanceCheck{Year: c.Year, Reason: "-", Active: true}); msg != "" {
			fail(0, "", "", msg)
			return res
		}
	}
	seen := map[string]int{}
	for _, row := range f.Rows {
		nip, bad := CleanNIP(row.NIP)
		if bad != "" {
			fail(row.Line, LeaveColNIP, "", bad)
			continue
		}
		if nip == "" {
			fail(row.Line, LeaveColNIP, "", "NIP wajib diisi")
			continue
		}
		if first, dup := seen[nip]; dup {
			fail(row.Line, LeaveColNIP, "", fmt.Sprintf("NIP %s ganda (sudah ada di baris %d)", nip, first))
			continue
		}
		seen[nip] = row.Line
		e, known := c.Employees[nip]
		switch {
		case !known || !e.Active:
			fail(row.Line, LeaveColNIP, "", fmt.Sprintf("NIP %s tidak dikenal atau karyawan sudah nonaktif", nip))
			continue
		case !e.InScope:
			fail(row.Line, LeaveColNIP, e.Name, fmt.Sprintf("Karyawan tidak termasuk %s", scope))
			continue
		}
		res.EmployeeIDs = append(res.EmployeeIDs, e.ID)

		after := LeaveBalanceValues{}
		parsed := true
		figure := func(col, raw string, required bool) float64 {
			if raw == "" {
				if required {
					fail(row.Line, col, e.Name, "wajib diisi")
					parsed = false
				}
				return 0
			}
			v, ok := ParseLeaveDays(raw)
			if !ok {
				fail(row.Line, col, e.Name, "harus berupa angka")
				parsed = false
			}
			return v
		}
		after.Total = figure(LeaveColTotal, row.Total, true)
		after.Adjustment = figure(LeaveColAdjustment, row.Adjustment, false)
		after.CarryDays = figure(LeaveColCarry, row.CarryDays, false)
		if row.Expires != "" {
			if d, ok := ParseDateCell(row.Expires); ok {
				after.CarryExpires = d
			} else {
				fail(row.Line, LeaveColExpires, e.Name, "bukan tanggal yang valid (pakai format dd/mm/yyyy)")
				parsed = false
			}
		}
		if !parsed {
			continue
		}
		if after == e.Current {
			res.Same++
			continue
		}
		if c.Check != nil {
			if msg := c.Check(LeaveBalanceCheck{Year: c.Year, Values: after, Reason: row.Reason,
				Used: e.Used, CarryUsed: e.CarryUsed, Active: e.Active,
				CarryChanged: after.CarryDays != e.Current.CarryDays || after.CarryExpires != e.Current.CarryExpires}); msg != "" {
				fail(row.Line, "", e.Name, msg)
				continue
			}
		}
		if after.Total > leaveWarnTotal {
			warn(row.Line, LeaveColTotal, e.Name, fmt.Sprintf("jatah %s hari lebih dari %d hari — pastikan sudah benar", daysID(after.Total), leaveWarnTotal))
		}
		if after.CarryDays > 0 && after.CarryExpires == "" {
			warn(row.Line, LeaveColExpires, e.Name, "sisa bawaan tanpa tanggal kedaluwarsa dianggap tidak berlaku")
		}
		if after.CarryExpires != "" && after.CarryExpires < c.Today {
			warn(row.Line, LeaveColExpires, e.Name, "tanggal kedaluwarsa sudah lewat — sisa bawaan tidak bisa dipakai")
		}
		res.Changes = append(res.Changes, LeaveBalanceChange{Line: row.Line, EmployeeID: e.ID, NIP: nip, Name: e.Name,
			New: !e.Exists, Before: e.Current, After: after, Reason: strings.TrimSpace(row.Reason)})
	}
	return res
}

// daysID formats a number of days the Indonesian way (9,5).
func daysID(v float64) string {
	return strings.Replace(strconv.FormatFloat(v, 'f', -1, 64), ".", ",", 1)
}

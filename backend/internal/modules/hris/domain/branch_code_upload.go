package domain

import (
	"fmt"
	"regexp"
	"strings"

	"nuhabit/backend/internal/platform/validate"
)

// Kode & Status Toko template (EPIC-100): one row per branch; empty cells in
// the "Baru" columns leave that value as it is.

const BranchCodeSheet = "Data Toko"

var BranchCodeHeaders = []string{"Kode Saat Ini", "Perusahaan", "Nama Saat Ini", "Status Saat Ini", "Kode Baru", "Nama Baru", "Status Baru"}

// BranchRef is a branch as stored now.
type BranchRef struct {
	ID, Code, Name string
	Active         bool
}

// BranchUsage is what keeps a branch from being deactivated (R4): the names
// of employees with an active placement there and the shift days from today.
type BranchUsage struct {
	Employees  []string
	FutureDays int
}

// BranchCodeChange is one branch the file changes.
type BranchCodeChange struct {
	Line                                   int
	ID, OldCode, NewCode, OldName, NewName string
	OldActive, NewActive                   bool
}

// BranchCodeResult is the outcome of a Data Toko file.
type BranchCodeResult struct {
	Changes          []BranchCodeChange
	Errors, Warnings []UploadIssue
	HeaderLine       int
}

var branchCodePattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9-]{1,29}$`)

const maxBranchName = 150

// shortCode drops the ODOO- prefix the Odoo import gave most codes; names
// use the short form ("PPGA - …", EPIC-069).
func shortCode(code string) string { return strings.TrimPrefix(code, "ODOO-") }

// BranchNameForCode is the name after a code change (R3): the "<KODE> - "
// prefix follows the new code; a name without it stays.
func BranchNameForCode(oldCode, newCode, name string) string {
	prefix := shortCode(oldCode) + " - "
	if !strings.HasPrefix(name, prefix) {
		return name
	}
	return shortCode(newCode) + " - " + strings.TrimPrefix(name, prefix)
}

// UsageMessage is why an in-use branch cannot be deactivated.
func (u BranchUsage) Message(code string) string {
	var parts []string
	if n := len(u.Employees); n > 0 {
		names := u.Employees
		more := ""
		if n > 5 {
			names, more = names[:5], fmt.Sprintf(", dan %d lainnya", n-5)
		}
		parts = append(parts, fmt.Sprintf("%d karyawan aktif (%s%s)", n, strings.Join(names, ", "), more))
	}
	if u.FutureDays > 0 {
		parts = append(parts, fmt.Sprintf("%d jadwal mulai hari ini", u.FutureDays))
	}
	return fmt.Sprintf("Toko %s masih dipakai: %s — pindahkan dulu lewat Penempatan", code, strings.Join(parts, " dan "))
}

// InUse reports whether anything keeps the branch from being deactivated.
func (u BranchUsage) InUse() bool { return len(u.Employees) > 0 || u.FutureDays > 0 }

// ValidateBranchCodeUpload checks a Data Toko sheet against the branches as
// stored and their usage (keyed by branch ID). headerProblem is non-empty
// when the sheet is not the template.
func ValidateBranchCodeUpload(matrix [][]string, branches []BranchRef, usage map[string]BranchUsage) (BranchCodeResult, string) {
	res := BranchCodeResult{Errors: []UploadIssue{}, Warnings: []UploadIssue{}}
	header := -1
	for i, r := range matrix {
		if len(r) > 0 && NormalizeHeader(r[0]) == "kode saat ini" {
			header = i
			break
		}
	}
	if header < 0 {
		return res, `Judul kolom "Kode Saat Ini" tidak ditemukan — pakai template dari halaman Penempatan`
	}
	res.HeaderLine = header + 1
	col := map[string]int{}
	for j, h := range matrix[header] {
		col[NormalizeHeader(h)] = j
	}
	cell := func(r []string, name string) string {
		j, ok := col[NormalizeHeader(name)]
		if !ok || j >= len(r) {
			return ""
		}
		return strings.TrimSpace(r[j])
	}
	// holders lists the branches per code; the DB is unique per company only,
	// so a code may name several branches and is then refused, never guessed.
	holders := map[string][]BranchRef{}
	for _, b := range branches {
		k := strings.ToUpper(strings.TrimSpace(b.Code))
		holders[k] = append(holders[k], b)
	}
	seenCode, seenNew := map[string]int{}, map[string]int{}
	for i := header + 1; i < len(matrix); i++ {
		r, line := matrix[i], i+1
		code := strings.ToUpper(cell(r, "Kode Saat Ini"))
		newCode := strings.ToUpper(cell(r, "Kode Baru"))
		newName := validate.JSTrim(cell(r, "Nama Baru"))
		status := strings.ToLower(cell(r, "Status Baru"))
		if code == "" && newCode == "" && newName == "" && status == "" {
			continue
		}
		bad := false
		fail := func(column, msg string) {
			res.Errors = append(res.Errors, UploadIssue{Line: line, Column: column, Message: msg})
			bad = true
		}
		held := holders[code]
		ok := len(held) == 1
		var b BranchRef
		if ok {
			b = held[0]
		}
		switch {
		case code == "":
			fail("Kode Saat Ini", "Kode Saat Ini wajib diisi")
		case len(held) > 1:
			names := make([]string, len(held))
			for i, h := range held {
				names[i] = h.Name
			}
			fail("Kode Saat Ini", fmt.Sprintf("Kode %s dipakai %d toko (%s) — ubah lewat Settings → Business",
				code, len(held), strings.Join(names, ", ")))
		case !ok:
			fail("Kode Saat Ini", "Kode "+code+" tidak ditemukan")
		case seenCode[code] > 0:
			fail("Kode Saat Ini", fmt.Sprintf("Kode %s ganda di file (baris %d & %d)", code, seenCode[code], line))
		}
		if code != "" && seenCode[code] == 0 {
			seenCode[code] = line
		}
		if !ok {
			continue
		}
		c := BranchCodeChange{Line: line, ID: b.ID, OldCode: b.Code, NewCode: b.Code, OldName: b.Name, NewName: b.Name,
			OldActive: b.Active, NewActive: b.Active}
		if newCode != "" && newCode != strings.ToUpper(b.Code) {
			if owners := holders[newCode]; len(owners) > 0 && owners[0].ID != b.ID {
				fail("Kode Baru", "Kode "+newCode+" sudah dipakai toko "+owners[0].Name)
			} else if !branchCodePattern.MatchString(newCode) {
				fail("Kode Baru", "Kode baru hanya huruf besar, angka, dan tanda -, 2–30 karakter")
			} else if first, dup := seenNew[newCode]; dup {
				fail("Kode Baru", fmt.Sprintf("Kode baru %s ganda di file (baris %d & %d)", newCode, first, line))
			} else {
				seenNew[newCode] = line
				c.NewCode = newCode
				c.NewName = BranchNameForCode(b.Code, newCode, b.Name)
			}
		}
		if newName != "" {
			if validate.UTF16Len(newName) > maxBranchName {
				fail("Nama Baru", fmt.Sprintf("Nama baru maksimal %d karakter", maxBranchName))
			} else {
				c.NewName = newName
			}
		}
		switch status {
		case "":
		case "aktif":
			c.NewActive = true
		case "nonaktif":
			c.NewActive = false
			if b.Active && usage[b.ID].InUse() {
				fail("Status Baru", usage[b.ID].Message(b.Code))
			}
		default:
			fail("Status Baru", "Status harus Aktif atau Nonaktif")
		}
		if bad || (c.NewCode == c.OldCode && c.NewName == c.OldName && c.NewActive == c.OldActive) {
			continue
		}
		if c.NewCode != c.OldCode {
			res.Warnings = append(res.Warnings, UploadIssue{Line: line, Message: fmt.Sprintf(
				"Kode %s → %s: file Data Kerja/Jadwal yang diunduh sebelumnya memakai kode lama — unduh ulang", c.OldCode, c.NewCode)})
		}
		res.Changes = append(res.Changes, c)
	}
	return res, ""
}

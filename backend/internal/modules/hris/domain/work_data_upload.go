package domain

import (
	"fmt"
	"strings"
)

// Pengisian massal data kerja (EPIC-086): satu file untuk semua cabang berisi
// penempatan utama, tipe jadwal, pola mingguan, departemen dan jabatan.

// WorkDataHeader is the header row of the "Data" sheet.
var WorkDataHeader = []string{"NIP", "Nama", "Departemen Saat Ini", "Jabatan Saat Ini", "Penempatan Saat Ini",
	"Tipe Saat Ini", "Yang Perlu Dilengkapi", "Penempatan Utama (Kode Toko)", "Tipe Jadwal",
	"Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu", "Minggu", "Berlaku Mulai", "Departemen Baru", "Jabatan Baru", "Catatan"}

// WorkDataDayOff is the day-off code of the weekly pattern.
const WorkDataDayOff = "LIBUR"

const (
	colNIP, colBranch, colType, colMon, colFrom, colDept, colPos = 0, 7, 8, 9, 16, 17, 18
)

var workDataDays = []string{"Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu", "Minggu"}

// WorkDataEmployee is an employee the file may refer to. RosterLast is the
// last per-date roster day on or after today ("" without one).
type WorkDataEmployee struct {
	ID, NIP, Name, ScheduleType string
	Active                      bool
	RosterLast                  string
}

// WorkDataMasters are the live masters a file is checked against. Branches
// map an active store code to its id; Departments and Positions map a
// lower-cased name to its id; ClosedFrom reports a date in a closed period.
type WorkDataMasters struct {
	Employees              map[string]WorkDataEmployee
	Branches               map[string]string
	Shifts                 map[string]UploadShift
	Departments, Positions map[string]string
	Today                  string
	ClosedFrom             func(date string) bool
}

// PatternCell is one day of the weekly pattern; Shift1 nil = LIBUR.
type PatternCell struct{ Shift1, Shift2 *string }

// WorkDataRow is one valid change; nil fields stay as they are.
type WorkDataRow struct {
	Line                     int
	EmployeeID, NIP, Name    string
	BranchID, ScheduleType   *string
	Pattern                  *[7]PatternCell
	From                     string
	DepartmentID, PositionID *string
}

func cellAt(r []string, i int) string {
	if i < len(r) {
		return strings.TrimSpace(r[i])
	}
	return ""
}

// ValidateWorkData checks the "Data" sheet rows (R3): errs block a row,
// warns may be accepted. A row without any change is skipped.
func ValidateWorkData(header []string, rows [][]string, firstLine int, m WorkDataMasters) (valid []WorkDataRow, errs, warns []UploadIssue) {
	if len(header) < len(WorkDataHeader) {
		return nil, []UploadIssue{{Message: "File bukan template data kerja (kolom tidak sesuai)"}}, nil
	}
	for i, h := range WorkDataHeader {
		if strings.TrimSpace(header[i]) != h {
			return nil, []UploadIssue{{Message: "File bukan template data kerja (kolom tidak sesuai)"}}, nil
		}
	}
	seen := map[string]int{}
	for i, r := range rows {
		line := firstLine + i
		nip := cellAt(r, colNIP)
		if nip == "" {
			continue
		}
		fail := func(msg string) { errs = append(errs, UploadIssue{Line: line, Name: nip, Message: msg}) }
		if prev, dup := seen[nip]; dup {
			fail(fmt.Sprintf("NIP %s ganda (juga di baris %d)", nip, prev))
			continue
		}
		seen[nip] = line
		emp, known := m.Employees[nip]
		switch {
		case !known:
			fail("NIP " + nip + " tidak dikenal")
			continue
		case !emp.Active:
			fail("NIP " + nip + " nonaktif")
			continue
		}
		out := WorkDataRow{Line: line, EmployeeID: emp.ID, NIP: nip, Name: emp.Name}
		bad := false
		if code := strings.ToUpper(cellAt(r, colBranch)); code != "" {
			id, ok := m.Branches[code]
			switch {
			case ok && id == "":
				fail("Kode toko " + code + " ada lebih dari satu di master — hubungi tim")
				bad = true
			case ok:
				out.BranchID = &id
			default:
				fail("Kode toko " + code + " tidak dikenal/nonaktif")
				bad = true
			}
		}
		scheduleType := emp.ScheduleType
		switch t := strings.ToLower(cellAt(r, colType)); t {
		case "":
		case "shift", "non-shift":
			v := map[string]string{"shift": "shift", "non-shift": "non_shift"}[t]
			out.ScheduleType, scheduleType = &v, v
		default:
			fail("Tipe Jadwal harus Shift atau Non-Shift")
			bad = true
		}
		var filled int
		for d := 0; d < 7; d++ {
			if cellAt(r, colMon+d) != "" {
				filled++
			}
		}
		switch {
		case filled > 0 && scheduleType != "shift":
			fail("Karyawan Non-Shift mengikuti jadwal tetap departemen — kosongkan Senin–Minggu")
			bad = true
		case filled > 0 && filled < 7:
			fail("Pola mingguan harus diisi 7 hari (Senin–Minggu)")
			bad = true
		case filled == 7:
			var p [7]PatternCell
			off := 0
			for d := 0; d < 7; d++ {
				raw := strings.ToUpper(cellAt(r, colMon+d))
				if raw == WorkDataDayOff {
					off++
					continue
				}
				cell, msg := workDataCell(raw, m.Shifts)
				if msg != "" {
					fail(workDataDays[d] + ": " + msg)
					bad = true
					continue
				}
				p[d] = cell
			}
			out.Pattern = &p
			if off == 0 && !bad {
				warns = append(warns, UploadIssue{Line: line, Name: nip, Message: "Pola tanpa hari LIBUR (masuk 7 hari)"})
			}
		}
		if out.ScheduleType != nil && *out.ScheduleType == "shift" && emp.ScheduleType != "shift" && filled == 0 {
			fail("Ubah ke Shift wajib mengisi pola Senin–Minggu (jadwal tetap berhenti mulai Berlaku Mulai)")
			bad = true
		}
		if name := cellAt(r, colDept); name != "" {
			id, ok := m.Departments[strings.ToLower(name)]
			switch {
			case ok && id == "":
				fail("Departemen " + name + " ada lebih dari satu di master — hubungi tim")
				bad = true
			case ok:
				out.DepartmentID = &id
			default:
				fail("Departemen " + name + " tidak dikenal")
				bad = true
			}
		}
		if name := cellAt(r, colPos); name != "" {
			id, ok := m.Positions[strings.ToLower(name)]
			switch {
			case ok && id == "":
				fail("Jabatan " + name + " ada lebih dari satu di master — tulis maksudnya di Catatan dan hubungi tim")
				bad = true
			case ok:
				out.PositionID = &id
			default:
				fail("Jabatan " + name + " tidak dikenal")
				bad = true
			}
		}
		dated := out.BranchID != nil || out.ScheduleType != nil || out.Pattern != nil
		from := cellAt(r, colFrom)
		if len(from) > 10 && from[10] == 'T' { // sel tanggal Excel dibaca sebagai timestamp ISO
			from = from[:10]
		}
		if from != "" {
			t, ok := ParseDate(from)
			switch {
			case !ok || len(from) != 10:
				fail("Berlaku Mulai harus berformat YYYY-MM-DD")
				bad = true
			case from < m.Today:
				fail("Berlaku Mulai sudah lewat (" + t.Format("02-01-2006") + ")")
				bad = true
			case m.ClosedFrom != nil && m.ClosedFrom(from):
				fail("Berlaku Mulai " + t.Format("02-01-2006") + " ada di periode absensi yang sudah closing")
				bad = true
			default:
				out.From = from
			}
		} else if dated {
			fail("Berlaku Mulai wajib diisi bila ada perubahan")
			bad = true
		}
		if bad {
			continue
		}
		if emp.RosterLast != "" && emp.RosterLast >= out.From {
			last, _ := ParseDate(emp.RosterLast)
			from, _ := ParseDate(out.From)
			switch {
			case out.ScheduleType != nil && *out.ScheduleType == "non_shift" && emp.ScheduleType == "shift":
				warns = append(warns, UploadIssue{Line: line, Name: nip, Message: "Ubah ke Non-Shift menghapus jadwal per tanggal mulai " +
					from.Format("02-01-2006") + " (sampai " + last.Format("02-01-2006") + ")"})
			case out.Pattern != nil:
				warns = append(warns, UploadIssue{Line: line, Name: nip, Message: "Sudah ada jadwal per tanggal sampai " +
					last.Format("02-01-2006") + "; jadwal per tanggal tetap berlaku di atas pola"})
			}
		}
		if dated || out.DepartmentID != nil || out.PositionID != nil {
			valid = append(valid, out)
		}
	}
	return valid, errs, warns
}

// workDataCell resolves "KODE" or "KODE1+KODE2" against the active shifts.
func workDataCell(raw string, shifts map[string]UploadShift) (PatternCell, string) {
	a, b, err := ParseScheduleCode(raw)
	if err != nil {
		return PatternCell{}, err.Error()
	}
	first, ok := shifts[a]
	if !ok || !first.Active {
		return PatternCell{}, "kode shift " + a + " tidak dikenal/nonaktif"
	}
	cell := PatternCell{Shift1: &first.ID}
	if b == "" {
		return cell, ""
	}
	second, ok := shifts[b]
	if !ok || !second.Active {
		return PatternCell{}, "kode shift " + b + " tidak dikenal/nonaktif"
	}
	if msg := TwoShiftIssue(first, second); msg != "" {
		return PatternCell{}, msg
	}
	cell.Shift2 = &second.ID
	return cell, ""
}

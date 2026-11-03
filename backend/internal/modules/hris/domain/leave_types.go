package domain

import (
	"strings"
	"time"
)

// LeaveType is one hris.leaves.leave_type value (EPIC-058). The enum keeps
// every value ever used; Selectable says whether new requests may use it.
type LeaveType struct {
	Code   string
	Label  string // form, daftar, WhatsApp
	Short  string // ringkasan laporan HRIS
	Export string // ekspor CSV (label lama dipertahankan, dibaca di luar aplikasi)

	Selectable   bool // false = riwayat saja
	MonthlyQuota int  // >0: hari per bulan kalender (pending + approved)
	SingleDay    bool // diajukan per satu hari
	LeaderOnly   bool // hanya jabatan Supervisor ke atas
	// DeductsThpDay: memotong 1 hari THP per hari izin (PIP payroll), kecuali intern.
	DeductsThpDay bool

	// Aturan TnC 03-11 (EPIC-084 R1). Nol/kosong = tanpa batas.
	MaxDays            int    // hari kerja per pengajuan
	MaxSpanDays        int    // hari kalender per pengajuan
	LeadDays           int    // diajukan paling lambat H-n (pengajuan milik sendiri)
	SameDayOnly        bool   // diajukan pada hari itu (tanggal mulai = hari ini)
	AttachmentRequired bool   // lampiran wajib
	Gender             string // "female" / "male": hanya untuk gender itu bila data gender terisi
}

// leaveTypes is the catalog, in form order. Mirror: frontend/src/lib/hris/leave-types.ts.
var leaveTypes = []LeaveType{
	{Code: "annual", Label: "Cuti Tahunan", Short: "Tahunan", Export: "Cuti Tahunan", Selectable: true, LeadDays: 3},
	{Code: "sick", Label: "Sakit", Short: "Sakit", Export: "Cuti Sakit", Selectable: true, SameDayOnly: true},
	{Code: "personal", Label: "Izin Pribadi", Short: "Izin Pribadi", Export: "Izin Pribadi", Selectable: true, MonthlyQuota: 3, SingleDay: true, DeductsThpDay: true},
	{Code: "leader", Label: "Izin Leader", Short: "Izin Leader", Export: "Izin Leader", Selectable: true, MonthlyQuota: 1, SingleDay: true, LeaderOnly: true},
	{Code: "birthday", Label: "Birthday Leave (Cuti Ulang Tahun)", Short: "Ulang Tahun", Export: "Birthday Leave (Cuti Ulang Tahun)", Selectable: true, SingleDay: true, LeadDays: 3, AttachmentRequired: true},
	{Code: "maternity", Label: "Cuti Melahirkan", Short: "Melahirkan", Export: "Cuti Melahirkan", Selectable: true, MaxSpanDays: 90, LeadDays: 3, Gender: "female"},
	{Code: "miscarriage", Label: "Cuti Keguguran", Short: "Keguguran", Export: "Cuti Keguguran", Selectable: true, MaxSpanDays: 45, AttachmentRequired: true, Gender: "female"},
	{Code: "paternity", Label: "Cuti Istri Melahirkan", Short: "Istri Melahirkan", Export: "Cuti Ayah", Selectable: true, MaxDays: 2, Gender: "male"},
	{Code: "wife_miscarriage", Label: "Cuti Istri Keguguran", Short: "Istri Keguguran", Export: "Cuti Istri Keguguran", Selectable: true, MaxDays: 2, Gender: "male"},
	{Code: "marriage", Label: "Cuti Menikah", Short: "Pernikahan", Export: "Cuti Menikah", Selectable: true, MaxDays: 3, LeadDays: 3},
	{Code: "child_marriage", Label: "Cuti Menikahkan Anak", Short: "Menikahkan Anak", Export: "Cuti Menikahkan Anak", Selectable: true, MaxDays: 2, LeadDays: 3},
	{Code: "child_ceremony", Label: "Cuti Khitanan/Baptis Anak", Short: "Khitanan/Baptis", Export: "Cuti Khitanan/Baptis Anak", Selectable: true, MaxDays: 2, LeadDays: 3},
	{Code: "pilgrimage", Label: "Cuti Ibadah Haji/Umrah", Short: "Haji/Umrah", Export: "Cuti Haji/Umrah", Selectable: true, MaxSpanDays: 40, LeadDays: 30},
	{Code: "bereavement", Label: "Cuti Berkabung", Short: "Berkabung", Export: "Cuti Duka", Selectable: true, MaxDays: 2},
	// Dihapus untuk pengajuan baru (EPIC-058); riwayat tetap terbaca.
	{Code: "emergency", Label: "Izin Darurat", Short: "Darurat", Export: "Cuti Darurat"},
	{Code: "unpaid", Label: "Izin Tanpa Gaji", Short: "Tidak Dibayar", Export: "Cuti Tanpa Upah"},
	{Code: "menstrual", Label: "Cuti Haid", Short: "Haid", Export: "Cuti Haid"},
}

func LookupLeaveType(code string) (LeaveType, bool) {
	for _, t := range leaveTypes {
		if t.Code == code {
			return t, true
		}
	}
	return LeaveType{}, false
}

// LeaveTypeCodes is every enum value, for validation and filters.
func LeaveTypeCodes() []string {
	out := make([]string, len(leaveTypes))
	for i, t := range leaveTypes {
		out[i] = t.Code
	}
	return out
}

// QuotaLeaveTypes are the types with a monthly quota, in catalog order.
func QuotaLeaveTypes() []LeaveType {
	var out []LeaveType
	for _, t := range leaveTypes {
		if t.MonthlyQuota > 0 {
			out = append(out, t)
		}
	}
	return out
}

func leaveText(code string, pick func(LeaveType) string) string {
	if t, ok := LookupLeaveType(code); ok {
		return pick(t)
	}
	return code
}

// LeaveTypeLabel names a leave type (forms, lists, WhatsApp).
func LeaveTypeLabel(code string) string {
	return leaveText(code, func(t LeaveType) string { return t.Label })
}

// LeaveTypeShort is the report label.
func LeaveTypeShort(code string) string {
	return leaveText(code, func(t LeaveType) string { return t.Short })
}

// LeaveTypeExportLabel is the CSV export label.
func LeaveTypeExportLabel(code string) string {
	return leaveText(code, func(t LeaveType) string { return t.Export })
}

// leaderLevels are the hris.positions.level values from Supervisor up
// (ladder of the master Posisi form).
var leaderLevels = map[string]bool{
	"supervisor": true, "assistant manager": true, "manager": true, "senior manager": true,
	"general manager": true, "director": true, "c-level": true,
}

// IsLeaderLevel reports whether a position level may take Izin Leader.
func IsLeaderLevel(level string) bool { return leaderLevels[strings.ToLower(strings.TrimSpace(level))] }

// MonthBounds is the first and last date of a YYYY-MM month.
func MonthBounds(month string) (from, to string, ok bool) {
	if len(month) != 7 {
		return "", "", false
	}
	t, err := time.Parse("2006-01", month)
	if err != nil {
		return "", "", false
	}
	return t.Format(DateLayout), t.AddDate(0, 1, -1).Format(DateLayout), true
}

// MonthOf is the YYYY-MM of a YYYY-MM-DD date.
func MonthOf(date string) string {
	if len(date) < 7 {
		return ""
	}
	return date[:7]
}

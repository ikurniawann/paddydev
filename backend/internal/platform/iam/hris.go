package iam

// HRIS leaf menus whose deletes and destructive overwrites need the
// "delete" action (EPIC-061, Q-RA-1). Each route checks its own leaf,
// never a group prefix, so a delete grant on a sibling leaf or a group
// does not leak.
const (
	LeafKaryawan       = "hris.kepegawaian.users"
	LeafKontrak        = "hris.kepegawaian.contracts"
	LeafShift          = "hris.kepegawaian.shifts"
	LeafJadwalKerja    = "hris.kepegawaian.schedules"
	LeafHariLibur      = "hris.kepegawaian.holidays"
	LeafPengumuman     = "hris.kepegawaian.announcements"
	LeafPenempatan     = "hris.master.placements"
	LeafDepartemen     = "hris.master.departments"
	LeafJabatan        = "hris.master.positions"
	LeafStatusKaryawan = "hris.master.employment-statuses"
	LeafAbsensi        = "hris.workforce.attendance"
	LeafCuti           = "hris.workforce.leaves"
	LeafRunPenggajian  = "hris.compensation.payroll"
	LeafSalary         = "hris.compensation.salary"
	LeafKPIScorecard   = "hris.performance.kpi-scorecard"
	LeafKandidat       = "hris.recruitment.candidates"
	LeafPsikotes       = "hris.recruitment.psikotes"
	LeafJobPortal      = "hris.recruitment.job-portal"
)

// HrisDeleteLeaves lists every leaf above; the EPIC-061 migration offers
// and grants "delete" on them.
var HrisDeleteLeaves = []string{
	LeafKaryawan, LeafKontrak, LeafShift, LeafJadwalKerja, LeafHariLibur, LeafPengumuman,
	LeafPenempatan, LeafDepartemen, LeafJabatan, LeafStatusKaryawan, LeafAbsensi, LeafCuti,
	LeafRunPenggajian, LeafSalary, LeafKPIScorecard, LeafKandidat, LeafPsikotes, LeafJobPortal,
}

// LeafLembur is the HR Lembur leaf (EPIC-067 R10). It has no delete
// endpoint, so it is not in HrisDeleteLeaves.
const LeafLembur = "hris.kepegawaian.overtime"

// LeafSaldoCuti is the HR leave-balance leaf (EPIC-068 R7): read lists and
// reads balances, update edits one balance, import/export run the Excel
// template. It has no delete endpoint, so it is not in HrisDeleteLeaves.
const LeafSaldoCuti = "hris.workforce.leave-balances"

// HrisApproveLeaves are the leaves whose "approve" action gives the HR side
// of a decision (EPIC-067 R10): Absensi for attendance requests and
// Perjalanan Dinas, Cuti for leaves, Lembur for overtime. Each decision
// checks its own leaf, never a group prefix.
var HrisApproveLeaves = []string{LeafAbsensi, LeafCuti, LeafLembur}

// HrisStaffRoleCodes are the roles "Hak Akses HRIS" may change and assign:
// admin_hris (the source) and the roles iam.sync_hris_staff_roles derives
// from it. Keep in step with iam.hris_staff_role_masks.
var HrisStaffRoleCodes = []string{"admin_hris", "hris_manager", "hris_non_payroll", "hris_non_rekrutmen_kpi"}

// HrisAccessScopes are the subtrees whose menu access AdminHRIS may toggle
// (Q-RA-5: Penggajian, Rekrutmen, Kehadiran & Cuti).
var HrisAccessScopes = []string{"hris.compensation", "hris.recruitment", "hris.workforce"}

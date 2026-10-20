package hris

import (
	"nuhabit/backend/internal/platform/hrisdel"
	"nuhabit/backend/internal/platform/iam"
)

// HRIS deletes (EPIC-061, Q-RA-1): each needs "delete" on its own leaf
// menu and a reason, and leaves an audit.audit_log row with the row.
var (
	delEmployee  = hrisdel.Spec{Menu: iam.LeafKaryawan, Entity: "hris.employees", Table: "hris.employees", IDParam: "id"}
	delDocument  = hrisdel.Spec{Menu: iam.LeafKaryawan, Entity: "hris.employee_documents", Table: "hris.employee_documents", IDParam: "doc_id"}
	delPlacement = hrisdel.Spec{Menu: iam.LeafPenempatan, Entity: "hris.employee_placements", Table: "hris.employee_placements", IDParam: "id"}
	// Attendance validates its reason and writes its adjustment log and
	// audit row itself (DeleteAttendanceWithReason).
	delAttendance   = hrisdel.Spec{Menu: iam.LeafAbsensi, SelfAudited: true}
	delLeave        = hrisdel.Spec{Menu: iam.LeafCuti, Entity: "hris.leaves", Table: "hris.leaves", IDParam: "id"}
	delShift        = hrisdel.Spec{Menu: iam.LeafShift, Entity: "hris.shifts", Table: "hris.shifts", IDParam: "id"}
	delHoliday      = hrisdel.Spec{Menu: iam.LeafHariLibur, Entity: "hris.public_holidays", Table: "hris.public_holidays", IDParam: "id"}
	delContract     = hrisdel.Spec{Menu: iam.LeafKontrak, Entity: "hris.employment_contracts", Table: "hris.employment_contracts", IDParam: "id"}
	delSignedDoc    = hrisdel.Spec{Menu: iam.LeafKontrak, Entity: "hris.contract_signed_document", Table: "hris.employment_contracts", IDParam: "id"}
	delAnnouncement = hrisdel.Spec{Menu: iam.LeafPengumuman, Entity: "hris.announcements", Table: "hris.announcements", IDParam: "id"}
	delDepartment   = hrisdel.Spec{Menu: iam.LeafDepartemen, Entity: "hris.departments", Table: "hris.departments", IDParam: "id"}
	delStatus       = hrisdel.Spec{Menu: iam.LeafStatusKaryawan, Entity: "hris.employment_statuses", Table: "hris.employment_statuses", IDParam: "id"}
	delPosition     = hrisdel.Spec{Menu: iam.LeafJabatan, Entity: "hris.positions", Table: "hris.positions", IDParam: "id"}
)

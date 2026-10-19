package hris

import (
	"net/http"
	"slices"
	"strings"

	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/module"
)

func (h handlers) routes() []module.Route {
	kepegawaian := iam.HrisKepegawaian
	r := func(pattern string, handler http.Handler) module.Route {
		return module.Route{Pattern: pattern, Handler: handler}
	}
	return []module.Route{
		// employees
		r("GET /api/hris/employees", h.withMenu(employeeDirectoryReaders, h.listEmployees)),
		r("POST /api/hris/employees", h.withMenu(employeeRecordManagers, h.createEmployee)),
		r("POST /api/hris/employees/schedule-type/bulk", h.withMenu(kepegawaian, h.bulkScheduleType)),
		r("GET /api/hris/employees/documents", handle(h.listEmployeeDocuments)),
		r("POST /api/hris/employees/documents", h.withMenu(kepegawaian, h.createEmployeeDocument)),
		r("GET /api/hris/employees/{id}", handle(h.getEmployee)),
		r("PUT /api/hris/employees/{id}", h.withMenu(employeeRecordManagers, h.updateEmployee)),
		r("DELETE /api/hris/employees/{id}", h.withMenu(employeeRecordManagers, h.deleteEmployee)),
		// documents/{doc_id} and {id}/<tab> share a shape, which ServeMux
		// would reject as conflicting; Next prefers the static segment.
		r("/api/hris/employees/{id}/{sub}", h.employeeSubroutes()),

		// placements (Penempatan Karyawan: stores an employee works at)
		r("GET /api/hris/employee-placements", h.withMenu(placementManagers, h.listPlacementOverview)),
		r("POST /api/hris/employee-placements/bulk", h.withMenu(placementManagers, h.bulkPlacement)),
		r("PATCH /api/hris/employee-placements/{id}", h.withMenu(placementManagers, h.updatePlacement)),
		r("DELETE /api/hris/employee-placements/{id}", h.withMenu(placementManagers, h.deletePlacement)),
		r("GET /api/hris/me/placements", h.withActor(h.myPlacements)),
		r("GET /api/hris/branches", h.withUser(h.listBranches)),

		// attendance (clock-in/out, export and photos: fileRoutes)
		r("GET /api/hris/attendance", h.withActor(h.listAttendance)),
		r("GET /api/hris/attendance/daily-roster", h.withActor(h.dailyRoster)),
		r("GET /api/hris/attendance/schedule", h.withActor(h.attendanceSchedule)),
		r("GET /api/hris/attendance/stats", h.withActor(h.attendanceStats)),
		r("GET /api/hris/attendance/{id}", h.withActor(h.getAttendance)),
		r("PUT /api/hris/attendance/{id}", h.withActor(h.updateAttendance)),
		r("DELETE /api/hris/attendance/{id}", h.withActor(h.deleteAttendance)),
		// manual adjustment and period closing (closing absensi)
		r("POST /api/hris/attendance/manual", h.withActor(h.createManualAttendance)),
		r("GET /api/hris/attendance-adjustments", h.withActor(h.listAttendanceAdjustments)),
		r("GET /api/hris/attendance-periods", h.withPeriodActor(h.listAttendancePeriods)),
		r("GET /api/hris/attendance-periods/{id}", h.withPeriodActor(h.getAttendancePeriod)),
		r("PATCH /api/hris/attendance-periods/{id}", h.withPeriodActor(h.patchAttendancePeriod)),
		r("POST /api/hris/attendance-periods/{id}/close", h.withPeriodActor(h.closeAttendancePeriod)),
		r("POST /api/hris/attendance-periods/{id}/reopen", h.withPeriodActor(h.reopenAttendancePeriod)),
		r("GET /api/hris/attendance-settings", h.withPeriodActor(h.getAttendanceSettings)),
		r("PUT /api/hris/attendance-settings", h.withPeriodActor(h.putAttendanceSettings)),

		// leaves (attachments: fileRoutes)
		r("GET /api/hris/leaves", h.withActor(h.listLeaves)),
		r("POST /api/hris/leaves", h.withActor(h.createLeave)),
		r("POST /api/hris/leaves/approve", h.withActor(h.decideLeave)),
		r("GET /api/hris/leaves/export", h.withMenu(iam.HrisWorkforce, h.exportLeaves)),
		r("GET /api/hris/leaves/{id}", h.withActor(h.getLeave)),
		r("PUT /api/hris/leaves/{id}", h.withActor(h.updateLeave)),
		r("DELETE /api/hris/leaves/{id}", h.withActor(h.deleteLeave)),
		r("GET /api/hris/leave-balances/{employee_id}", h.withActor(h.getLeaveBalance)),
		r("PUT /api/hris/leave-balances/{employee_id}", h.withActor(h.putLeaveBalance)),

		// shifts and overtime
		r("GET /api/hris/shifts", h.withActor(h.listShifts)),
		r("POST /api/hris/shifts", h.withMenu(kepegawaian, h.createShift)),
		r("PATCH /api/hris/shifts/{id}", h.withMenu(kepegawaian, h.updateShift)),
		r("DELETE /api/hris/shifts/{id}", h.withMenu(kepegawaian, h.deleteShift)),
		r("POST /api/hris/schedules/fixed", h.withMenu(kepegawaian, h.applyFixedSchedule)),
		r("GET /api/hris/overtime", h.withActor(h.listOvertime)),
		r("POST /api/hris/overtime", h.withActor(h.createOvertime)),
		r("POST /api/hris/overtime/decide", h.withActor(h.decideOvertime)),

		// holidays
		r("GET /api/hris/holidays", h.withUser(h.listHolidays)),
		r("POST /api/hris/holidays", h.withMenu(kepegawaian, h.createHoliday)),
		r("GET /api/hris/holidays/import", h.withMenu(kepegawaian, h.previewHolidayImport)),
		r("POST /api/hris/holidays/import", h.withMenu(kepegawaian, h.importHolidays)),
		r("PATCH /api/hris/holidays/{id}", h.withMenu(kepegawaian, h.updateHoliday)),
		r("DELETE /api/hris/holidays/{id}", h.withMenu(kepegawaian, h.deleteHoliday)),

		// contracts (document PDF and signed scans: fileRoutes)
		r("GET /api/hris/contracts", h.withMenu(kepegawaian, h.listContracts)),
		r("GET /api/hris/contracts/expiring", h.withMenu(kepegawaian, h.expiringContracts)),
		r("PATCH /api/hris/contracts/{id}", h.withMenu(kepegawaian, h.patchContract)),
		r("DELETE /api/hris/contracts/{id}", h.withMenu(kepegawaian, h.deleteContract)),

		// records
		r("GET /api/hris/employment-history", handle(h.listEmploymentHistory)),
		r("POST /api/hris/employment-history", h.withMenu(kepegawaian, h.createEmploymentHistory)),
		r("GET /api/hris/departments", h.withUser(h.listDepartmentsBrief)),
		r("GET /api/hris/onboarding/{employee_id}", handle(h.listOnboarding)),
		r("POST /api/hris/onboarding/{employee_id}", h.withActor(h.postOnboarding)),
		r("PUT /api/hris/onboarding/{employee_id}", h.withActor(h.putOnboarding)),
		r("GET /api/hris/offboarding/{employee_id}", handle(h.listOffboarding)),
		r("POST /api/hris/offboarding/{employee_id}", h.withActor(h.postOffboarding)),
		r("PUT /api/hris/offboarding/{employee_id}", h.withActor(h.putOffboarding)),

		// self service
		r("GET /api/hris/me", h.withActor(h.me)),
		r("GET /api/hris/me/beranda", h.withActor(h.beranda)),
		r("GET /api/hris/me/team", h.withActor(h.team)),

		// announcements (cover upload and files: fileRoutes)
		r("GET /api/hris/announcements", h.withMenu(kepegawaian, h.listAnnouncements)),
		r("POST /api/hris/announcements", h.withMenu(kepegawaian, h.createAnnouncement)),
		r("GET /api/hris/announcements/feed", h.withActor(h.announcementFeed)),
		r("GET /api/hris/announcements/unread-count", http.HandlerFunc(h.unreadAnnouncements)),
		r("GET /api/hris/announcements/{id}", h.withActor(h.getAnnouncement)),
		r("PATCH /api/hris/announcements/{id}", h.withMenu(kepegawaian, h.updateAnnouncement)),
		r("DELETE /api/hris/announcements/{id}", h.withMenu(kepegawaian, h.deleteAnnouncement)),
		r("POST /api/hris/announcements/{id}/read", handle(h.readAnnouncement)),

		// notifications, badges, logbook, report
		r("GET /api/hris/notifications", h.withUser(h.listNotifications)),
		r("POST /api/hris/notifications", h.withUser(h.markNotifications)),
		r("GET /api/hris/nav-badges", handle(h.navBadges)),
		r("POST /api/hris/nav-badges", handle(h.markModuleSeen)),
		r("GET /api/hris/logbook", h.withLogbook(h.readLogbook)),
		r("POST /api/hris/logbook", h.withLogbook(h.postLogbook)),
		r("PATCH /api/hris/logbook", h.withLogbook(h.patchLogbook)),
		r("DELETE /api/hris/logbook", h.withLogbook(h.deleteLogbook)),
		r("GET /api/hris/reports", h.withMenu(iam.HrisInsights, h.report)),

		// master data
		r("GET /api/master/departments", h.masterList(departmentsKind)),
		r("POST /api/master/departments", h.masterSave(departmentsKind, false)),
		r("PUT /api/master/departments/{id}", h.masterSave(departmentsKind, true)),
		r("DELETE /api/master/departments/{id}", h.masterDelete(departmentsKind)),
		r("GET /api/master/employment-statuses", h.masterList(statusesKind)),
		r("POST /api/master/employment-statuses", h.masterSave(statusesKind, false)),
		r("PUT /api/master/employment-statuses/{id}", h.masterSave(statusesKind, true)),
		r("DELETE /api/master/employment-statuses/{id}", h.masterDelete(statusesKind)),
		r("GET /api/master/positions", h.masterList(positionsKind)),
		r("POST /api/master/positions", h.masterSave(positionsKind, false)),
		r("PUT /api/master/positions/{id}", h.masterSave(positionsKind, true)),
		r("DELETE /api/master/positions/{id}", h.masterDelete(positionsKind)),
	}
}

// employeeSubroutes dispatches /api/hris/employees/{id}/{sub}:
// documents/{doc_id} first (a static segment wins in Next), then the
// profile tabs.
func (h handlers) employeeSubroutes() http.Handler {
	kepegawaian := iam.HrisKepegawaian
	table := map[string]map[string]http.Handler{
		"contracts": {
			http.MethodGet:  h.withMenu(kepegawaian, h.listEmployeeContracts),
			http.MethodPost: h.withMenu(kepegawaian, h.createEmployeeContract),
		},
		"lifecycle": {http.MethodGet: handle(h.employeeLifecycle)},
		"placements": {
			http.MethodGet:  handle(h.listEmployeePlacements),
			http.MethodPost: h.withMenu(placementManagers, h.createEmployeePlacement),
		},
		"recruitment-documents": {http.MethodGet: h.withMenu(kepegawaian, h.recruitmentDocuments)},
		"shifts": {
			http.MethodGet: handle(h.employeeShifts),
			http.MethodPut: handle(h.saveEmployeeShifts),
		},
		"schedule-type": {
			http.MethodGet: h.withMenu(kepegawaian, h.getScheduleType),
			http.MethodPut: h.withMenu(kepegawaian, h.putScheduleType),
		},
	}
	documents := map[string]http.Handler{
		http.MethodPatch:  h.withMenu(kepegawaian, h.patchEmployeeDocument),
		http.MethodDelete: h.withMenu(kepegawaian, h.deleteEmployeeDocument),
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods := table[r.PathValue("sub")]
		if r.PathValue("id") == "documents" {
			r.SetPathValue("doc_id", r.PathValue("sub"))
			methods = documents
		}
		if methods == nil {
			_ = httpx.JSON(w, http.StatusNotFound, obj("success", false, "error", "Not found"))
			return
		}
		handler, ok := methods[r.Method]
		if !ok {
			w.Header().Set("Allow", allowList(methods))
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		handler.ServeHTTP(w, r)
	})
}

func allowList(methods map[string]http.Handler) string {
	names := make([]string, 0, len(methods))
	for m := range methods {
		names = append(names, m)
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}

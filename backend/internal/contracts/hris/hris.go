// Package hris holds the events the HRIS people module publishes.
package hris

// TopicLeaveRequested fires when an employee (or HR on their behalf)
// submits a leave request. The HRIS module itself sends the WhatsApp note
// to the employee's direct manager (lib/hris/leave-wa notifyLeaveRequestWa).
const TopicLeaveRequested = "hris.leave.requested"

// LeaveRequested is the TopicLeaveRequested payload.
type LeaveRequested struct {
	LeaveID      string  `json:"leave_id"`
	EmployeeID   string  `json:"employee_id"`
	EmployeeName string  `json:"employee_name"`
	LeaveType    string  `json:"leave_type"`
	StartDate    string  `json:"start_date"`
	EndDate      string  `json:"end_date"`
	TotalDays    int     `json:"total_days"`
	Reason       *string `json:"reason"`
}

// TopicOvertimeRequested fires when an employee submits their own overtime
// request (EPIC-063). The HRIS module sends the WhatsApp note to the
// employee's direct manager; HR assignments do not fire it.
const TopicOvertimeRequested = "hris.overtime.requested"

// OvertimeRequested is the TopicOvertimeRequested payload.
type OvertimeRequested struct {
	OvertimeID   string  `json:"overtime_id"`
	EmployeeID   string  `json:"employee_id"`
	EmployeeName string  `json:"employee_name"`
	Date         string  `json:"date"`
	StartTime    string  `json:"start_time"`
	EndTime      string  `json:"end_time"`
	Hours        float64 `json:"hours"`
	DayType      string  `json:"day_type"`
	Reason       *string `json:"reason"`
}

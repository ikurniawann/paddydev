package domain

import "slices"

// HRRoles see and manage every employee's workforce data (HR_ROLES).
var HRRoles = []string{"super_admin", "admin", "hrd", "hiring_manager"}

// LoanManageRoles see and file loans for any employee (LOAN_MANAGE_ROLES).
var LoanManageRoles = []string{"super_admin", "admin", "hrd", "finance_staff"}

// PayslipFullRoles see every employee's payslips (PAYSLIP_FULL_ROLES).
var PayslipFullRoles = []string{"super_admin", "hrd", "finance_staff"}

// PayrollSettingsWriteRoles may change statutory rates (on top of the
// hris.compensation grant).
var PayrollSettingsWriteRoles = []string{"super_admin", "hrd"}

// IsHR reports whether role is one of HRRoles.
func IsHR(role string) bool { return slices.Contains(HRRoles, role) }

// HasLoanFullAccess is hasLoanFullAccess.
func HasLoanFullAccess(role string) bool { return slices.Contains(LoanManageRoles, role) }

// CanWritePayrollSettings is assertPayrollSettingsWriter's check.
func CanWritePayrollSettings(role string) bool {
	return slices.Contains(PayrollSettingsWriteRoles, role)
}

// PayslipScope is resolvePayslipScope's result. Personal views only list
// paid runs. Skip means an account without an employee record on the
// personal view (an empty list); EmployeeID "" on a full view means all.
type PayslipScope struct {
	MeView     bool
	EmployeeID string
	Skip       bool
}

// ResolvePayslipScope: employee_id=me is personal for every role; a full
// view needs a PayslipFullRoles role, and an HR role also needs the
// Penggajian menu (payroll): an HR account without it is forced to its own
// slips (EPIC-057). finance_staff keeps its role-based view.
func ResolvePayslipScope(role string, payroll bool, actorEmployeeID *string, param *string) PayslipScope {
	full := slices.Contains(PayslipFullRoles, role) && (payroll || !IsHR(role))
	if (param != nil && *param == "me") || !full {
		if actorEmployeeID == nil {
			return PayslipScope{MeView: true, Skip: true}
		}
		return PayslipScope{MeView: true, EmployeeID: *actorEmployeeID}
	}
	if param != nil {
		return PayslipScope{EmployeeID: *param}
	}
	return PayslipScope{}
}

var runTransitions = map[string][]string{
	"draft":      {"processing"},
	"processing": {"completed"},
	"completed":  {"paid"},
	"paid":       {},
}

// CanTransitionRun is canTransitionRunStatus: only forward, one step.
func CanTransitionRun(from, to string) bool { return slices.Contains(runTransitions[from], to) }

// CanDeleteRun is canDeleteRun: a paid run is a final financial record.
func CanDeleteRun(status string) bool { return status != "paid" }

// CanCalculateRun is canCalculateRun: only drafts are (re)calculated.
func CanCalculateRun(status string) bool { return status == "draft" }

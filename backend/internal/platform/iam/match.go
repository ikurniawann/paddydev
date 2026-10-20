package iam

import "strings"

// HasMenuCode mirrors hasIamMenuCode: exact code granted.
func HasMenuCode(granted []string, code string) bool {
	for _, g := range granted {
		if g == code {
			return true
		}
	}
	return false
}

// HasAnyMenuPrefix mirrors hasAnyIamMenuPrefix: a granted code equals a
// prefix or sits below it ("items.product" lets "items.product.master" in).
func HasAnyMenuPrefix(granted, prefixes []string) bool {
	for _, code := range granted {
		for _, prefix := range prefixes {
			if code == prefix || strings.HasPrefix(code, prefix+".") {
				return true
			}
		}
	}
	return false
}

// HasGrantedAction mirrors hasGrantedAction: some menu under the prefixes
// carries the action (create/update/delete/approve) in granted_actions.
func HasGrantedAction(granted map[string][]string, prefixes []string, action string) bool {
	for code, actions := range granted {
		if !HasAnyMenuPrefix([]string{code}, prefixes) {
			continue
		}
		for _, a := range actions {
			if a == action {
				return true
			}
		}
	}
	return false
}

// essPerformanceMenus are the Kinerja leaves every employee holds (ESS:
// own review, department tasks).
var essPerformanceMenus = []string{"hris.performance.review", "hris.performance.dept-tasks"}

// HasPerformanceHRMenu: a Kinerja menu beyond the employee-level leaves —
// the HR side of hris.performance (EPIC-057: an HR account that also holds
// an employee role is not Kinerja HR through Review or Tugas Departemen).
func HasPerformanceHRMenu(granted []string) bool {
	for _, code := range granted {
		if HasAnyMenuPrefix([]string{code}, HrisPerformance) && !HasAnyMenuPrefix([]string{code}, essPerformanceMenus) {
			return true
		}
	}
	return false
}

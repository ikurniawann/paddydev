package domain

import "strings"

// IsInternStatus: status kepegawaian Magang. Master live memakai "intern",
// promosi rekrutmen menulis "internship". Mirror: payroll/domain.IsInternStatus.
func IsInternStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "intern", "internship":
		return true
	}
	return false
}

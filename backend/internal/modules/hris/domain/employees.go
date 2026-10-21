package domain

import (
	"fmt"
	"strings"
)

// NextAutoNip is the smallest free "EMP-<year>-<5 digits>"; "" when all
// 99999 numbers are taken.
func NextAutoNip(year int, taken []string) string {
	used := make(map[string]bool, len(taken))
	for _, t := range taken {
		used[t] = true
	}
	for seq := 1; seq <= 99999; seq++ {
		nip := fmt.Sprintf("EMP-%d-%05d", year, seq)
		if !used[nip] {
			return nip
		}
	}
	return ""
}

// EmployeeUniqueMessage maps an email unique-violation message to a friendly
// one. NIP, KTP and NPWP are named by employeekeys (pre-check and constraint).
func EmployeeUniqueMessage(message string) string {
	if strings.Contains(message, "email") {
		return "Email sudah terdaftar"
	}
	return ""
}

// TrackedState is the employee columns PUT compares for employment history.
type TrackedState struct {
	EmploymentStatus *string
	DepartmentID     *string
	SectionID        *string
	JobTitleID       *string
}

// HistoryChange is the employment_history row a PUT records.
type HistoryChange struct {
	Notes                string
	PrevEmploymentStatus *string
	NewEmploymentStatus  *string
	PrevDepartmentID     *string
	NewDepartmentID      *string
	PrevSectionID        *string
	NewSectionID         *string
	PrevJobTitleID       *string
	NewJobTitleID        *string
}

// TrackedUpdate holds the tracked fields a PUT body sent: Sent* says whether
// the key was present (null clears it).
type TrackedUpdate struct {
	EmploymentStatus, DepartmentID, SectionID, JobTitleID *string
	SentStatus, SentDepartment, SentSection, SentJobTitle bool
}

func eqPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func after(sent bool, body, current *string) *string {
	if sent {
		return body
	}
	return current
}

// EmploymentHistoryChange compares only the fields the PUT sent; nil when
// nothing tracked changed.
func EmploymentHistoryChange(current TrackedState, body TrackedUpdate) *HistoryChange {
	c := &HistoryChange{
		PrevEmploymentStatus: current.EmploymentStatus,
		NewEmploymentStatus:  after(body.SentStatus, body.EmploymentStatus, current.EmploymentStatus),
		PrevDepartmentID:     current.DepartmentID,
		NewDepartmentID:      after(body.SentDepartment, body.DepartmentID, current.DepartmentID),
		PrevSectionID:        current.SectionID,
		NewSectionID:         after(body.SentSection, body.SectionID, current.SectionID),
		PrevJobTitleID:       current.JobTitleID,
		NewJobTitleID:        after(body.SentJobTitle, body.JobTitleID, current.JobTitleID),
	}
	var notes []string
	if !eqPtr(c.PrevEmploymentStatus, c.NewEmploymentStatus) {
		notes = append(notes, "Status: "+jsString(c.PrevEmploymentStatus)+" → "+jsString(c.NewEmploymentStatus))
	}
	if !eqPtr(c.PrevDepartmentID, c.NewDepartmentID) {
		notes = append(notes, "Departemen berubah")
	}
	if !eqPtr(c.PrevSectionID, c.NewSectionID) {
		notes = append(notes, "Seksi berubah")
	}
	if !eqPtr(c.PrevJobTitleID, c.NewJobTitleID) {
		notes = append(notes, "Jabatan berubah")
	}
	if len(notes) == 0 {
		return nil
	}
	c.Notes = strings.Join(notes, ", ")
	return c
}

// jsString is `${value}` for a nullable string.
func jsString(s *string) string {
	if s == nil {
		return "null"
	}
	return *s
}

// DirectoryParams is the parsed GET /api/hris/employees query.
type DirectoryParams struct {
	Search           *string
	DepartmentID     *string
	SectionID        *string
	EmploymentStatus *string
	IsActive         *bool
	Page             int
	Limit            int
	SortBy           string
	Ascending        bool
}

var directorySortable = map[string]bool{
	"full_name": true, "nip": true, "email": true, "join_date": true, "employment_status": true, "created_at": true,
}

// ParseDirectoryParams is parseDirectoryParams: commas and parentheses are
// stripped from search so it cannot add filters to the .or() expression.
func ParseDirectoryParams(q Query) DirectoryParams {
	p := DirectoryParams{Page: 1, Limit: 20, SortBy: "full_name", Ascending: true}
	if s, ok := q("search"); ok {
		cleaned := JSTrim(strings.NewReplacer(",", " ", "(", " ", ")", " ").Replace(s))
		if cleaned != "" {
			p.Search = &cleaned
		}
	}
	str := func(key string) *string {
		if v, ok := q(key); ok {
			return &v
		}
		return nil
	}
	p.DepartmentID, p.SectionID, p.EmploymentStatus = str("department_id"), str("section_id"), str("employment_status")
	if v, ok := q("is_active"); ok {
		b := v == "true"
		p.IsActive = &b
	}
	page, _ := q("page")
	p.Page = max(1, IntOr(page, 1))
	limit, _ := q("limit")
	p.Limit = Clamp(IntOr(limit, 20), 1, 500)
	if s, _ := q("sort_by"); directorySortable[s] {
		p.SortBy = s
	}
	if o, ok := q("sort_order"); ok && o != "" && o != "asc" {
		p.Ascending = false
	}
	return p
}

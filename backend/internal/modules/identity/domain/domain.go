// Package domain holds the identity rules that need no database: the login
// throttle (frontend/src/lib/auth/login-throttle.ts), the change-password
// policy, and the stall switcher rules (lib/auth/stall-access.ts,
// lib/users/stall-assignment.ts, lib/configuration/sort-warehouses.ts).
package domain

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
)

// Login throttle limits: strict per account, loose per IP because many
// cashiers can share one office NAT.
const (
	LoginWindowMinutes = 5
	LoginMaxPerAccount = 8
	LoginMaxPerIP      = 60
	// PruneOlderThanMinutes: attempts older than this no longer matter.
	PruneOlderThanMinutes = 60
	// PruneChance is the share of recorded failures that also prune.
	PruneChance = 0.05
)

// NormalizeAccountKey is normalizeAccountKey: trimmed, lower-cased, at most
// 254 UTF-16 units, so A@x.com and a@x.com share one count.
func NormalizeAccountKey(email string) string {
	key := strings.ToLower(jsTrim(email))
	units := utf16.Encode([]rune(key))
	if len(units) > 254 {
		key = string(utf16.Decode(units[:254]))
	}
	return key
}

// ShouldBlockLogin is shouldBlockLogin: either count reached its limit.
func ShouldBlockLogin(accountCount, ipCount int) bool {
	return accountCount >= LoginMaxPerAccount || ipCount >= LoginMaxPerIP
}

// ChangePasswordProblem returns the 400 message of the change-password
// policy, or "" when the pair passes. Lengths are JS string lengths.
func ChangePasswordProblem(current, next string) string {
	switch {
	case current == "" || next == "":
		return "Current password and new password are required"
	case len(utf16.Encode([]rune(next))) < 8:
		return "New password must be at least 8 characters"
	case next == current:
		return "New password must be different from the current password"
	}
	return ""
}

// Stall is a warehouse option of the stall switcher.
type Stall struct {
	ID        string
	Name      string
	Code      string
	IsDefault bool
	// BranchID and BranchName say which branch the warehouse sits in; every
	// branch has its own "Main Storage", so the name alone is ambiguous.
	BranchID   string
	BranchName string
}

// StallAccess is getStallAccess's result.
type StallAccess struct {
	// AllAccess means any stall may be chosen, "Semua Stall" included.
	AllAccess bool
	// Stalls are the selectable stalls, sorted.
	Stalls []Stall
}

// StallAllAccess is computeStallAllAccess.
func StallAllAccess(role string, canSwitchStall, assignedMainStorage bool) bool {
	return role == "super_admin" || canSwitchStall || assignedMainStorage
}

// CanSwitchStall is the can_switch rule of active-stall and stall-options.
func CanSwitchStall(role string, access StallAccess) bool {
	return role == "super_admin" || role == "admin" || access.AllAccess || len(access.Stalls) > 1
}

// HasStall reports whether id is one of the selectable stalls.
func (a StallAccess) HasStall(id string) bool {
	for _, s := range a.Stalls {
		if s.ID == id {
			return true
		}
	}
	return false
}

// stallUUID is UUID_RE of active-stall.ts (versions 1 to 5 only).
var stallUUID = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// IsStallID is the UUID_RE guard of findActiveStall.
func IsStallID(id string) bool { return stallUUID.MatchString(id) }

// SortStalls is sortWarehouses: default (Main Storage) first, STALL-n
// numerically after the rest, MAIN early, then code and name with numeric
// collation.
func SortStalls(stalls []Stall) []Stall {
	out := append([]Stall(nil), stalls...)
	sort.SliceStable(out, func(i, j int) bool { return compareStalls(out[i], out[j]) < 0 })
	return out
}

// SortStallsByBranch orders stalls by branch name (then branch id), and
// within a branch by SortStalls' rules (Main Storage first).
func SortStallsByBranch(stalls []Stall) []Stall {
	out := append([]Stall(nil), stalls...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if c := naturalCompare(a.BranchName, b.BranchName); c != 0 {
			return c < 0
		}
		if a.BranchID != b.BranchID {
			return a.BranchID < b.BranchID
		}
		return compareStalls(a, b) < 0
	})
	return out
}

// SpansBranches reports whether the stalls belong to more than one branch.
func SpansBranches(stalls []Stall) bool {
	for _, s := range stalls[min(1, len(stalls)):] {
		if s.BranchID != stalls[0].BranchID {
			return true
		}
	}
	return false
}

// StallLabel is the text a switcher shows for a stall: the plain name, or
// "<name> — <branch name>" when the list it sits in mixes branches.
func StallLabel(s Stall, withBranch bool) string {
	if !withBranch || strings.TrimSpace(s.BranchName) == "" {
		return s.Name
	}
	return s.Name + " — " + s.BranchName
}

var stallCode = regexp.MustCompile(`^STALL-0*(\d+)$`)

func stallNumber(code string) (int, bool) {
	m := stallCode.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(code)))
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil
}

func compareStalls(a, b Stall) int {
	if a.IsDefault != b.IsDefault {
		if a.IsDefault {
			return -1
		}
		return 1
	}
	an, aok := stallNumber(a.Code)
	bn, bok := stallNumber(b.Code)
	switch {
	case aok && bok:
		return an - bn
	case aok:
		return 1
	case bok:
		return -1
	}
	if strings.EqualFold(a.Code, "MAIN") {
		return -1
	}
	if strings.EqualFold(b.Code, "MAIN") {
		return 1
	}
	if c := naturalCompare(a.Code, b.Code); c != 0 {
		return c
	}
	return naturalCompare(a.Name, b.Name)
}

// naturalCompare approximates localeCompare(a, b, undefined, {numeric: true,
// sensitivity: "base"}) for the ASCII codes and names stalls use.
func naturalCompare(a, b string) int {
	a, b = strings.ToLower(a), strings.ToLower(b)
	for a != "" && b != "" {
		da, db := leadingDigits(a), leadingDigits(b)
		if da != "" && db != "" {
			na, _ := strconv.Atoi(da)
			nb, _ := strconv.Atoi(db)
			if na != nb {
				if na < nb {
					return -1
				}
				return 1
			}
			a, b = a[len(da):], b[len(db):]
			continue
		}
		if a[0] != b[0] {
			if a[0] < b[0] {
				return -1
			}
			return 1
		}
		a, b = a[1:], b[1:]
	}
	switch {
	case a == "" && b == "":
		return 0
	case a == "":
		return -1
	}
	return 1
}

func leadingDigits(s string) string {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i]
}

// jsTrim is String.prototype.trim.
func jsTrim(s string) string {
	return strings.TrimFunc(s, func(r rune) bool { return unicode.IsSpace(r) || r == '\uFEFF' })
}

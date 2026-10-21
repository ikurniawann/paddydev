// Package stalllabel is the one rule for naming and ordering outlets
// (warehouses) in lists that can mix branches. Every branch owns a
// "Main Storage", so a list that spans branches must say which branch each
// row belongs to; a single-branch list keeps the plain names.
//
// It is pure and dependency-free so every module (identity stall switcher,
// purchasing warehouses, receipt settings, POS reports) applies the same
// rule without importing one another. The TS twin is
// frontend/src/lib/auth/stall-label.ts.
package stalllabel

import (
	"sort"
	"strconv"
	"strings"
)

// Spans reports whether the items belong to more than one branch.
func Spans[T any](items []T, branchID func(T) string) bool {
	for i := 1; i < len(items); i++ {
		if branchID(items[i]) != branchID(items[0]) {
			return true
		}
	}
	return false
}

// Label is the text shown for an outlet: the plain name, or
// "<name> — <branch name>" when withBranch and the branch name is known.
func Label(name, branchName string, withBranch bool) string {
	if !withBranch || strings.TrimSpace(branchName) == "" {
		return name
	}
	return name + " — " + branchName
}

// Key is what the ordering needs from an outlet.
type Key struct {
	BranchName string
	BranchID   string
	IsDefault  bool
}

// SortByBranch returns a copy ordered by branch name (natural,
// case-insensitive; branch id breaks ties between same-named branches),
// then within a branch by within, or by "default (Main Storage) first" and
// the input order when within is nil. The sort is stable.
func SortByBranch[T any](items []T, key func(T) Key, within func(a, b T) int) []T {
	out := append([]T(nil), items...)
	sort.SliceStable(out, func(i, j int) bool {
		ka, kb := key(out[i]), key(out[j])
		if c := NaturalCompare(ka.BranchName, kb.BranchName); c != 0 {
			return c < 0
		}
		if ka.BranchID != kb.BranchID {
			return ka.BranchID < kb.BranchID
		}
		if within != nil {
			return within(out[i], out[j]) < 0
		}
		return ka.IsDefault && !kb.IsDefault
	})
	return out
}

// NaturalCompare approximates localeCompare(a, b, undefined, {numeric: true,
// sensitivity: "base"}) for the ASCII codes and names stalls use.
func NaturalCompare(a, b string) int {
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

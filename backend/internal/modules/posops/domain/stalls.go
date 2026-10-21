package domain

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// Stall (warehouse) rules for POS selling and reports, from
// lib/pos/pos-sell-stall.ts, lib/users/stall-assignment.ts,
// lib/pos/central-cashier.ts and lib/configuration/sort-warehouses.ts.

// Active stall modes of the stall switcher cookie.
const (
	StallModeUnset = "unset"
	StallModeAll   = "all"
	StallModeStall = "stall"
)

// Stall is a configuration.warehouses row.
type Stall struct {
	ID        string
	Name      string
	Code      string
	BranchID  *string
	IsDefault bool
	// BranchName is the owning branch's name ("" when unknown); every branch
	// has its own "Main Storage", so pickers that mix branches show it.
	BranchName string
}

// SellStall is resolvePosSellStall's result: WarehouseID when OK, else a
// reason and message.
type SellStall struct {
	OK          bool
	WarehouseID string
	Reason      string
	Message     string
}

var sellStallMessages = map[string]string{
	"all_stalls":          "Pilih satu outlet aktif sebelum membuat transaksi POS",
	"no_stall":            "Tidak ada outlet penempatan. Hubungi admin untuk assign outlet",
	"multiple_unselected": "Pilih satu outlet aktif sebelum membuat transaksi POS",
}

func blockedStall(reason string) SellStall {
	return SellStall{Reason: reason, Message: sellStallMessages[reason]}
}

// SellStallInput is resolvePosSellStall's input.
type SellStallInput struct {
	ActiveMode         string
	ActiveStallID      string
	AssignedIDs        []string
	DefaultWarehouseID string
}

func uniqueNonEmpty(ids []string) []string {
	var out []string
	for _, id := range ids {
		if id != "" && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

// ResolveSellStall picks the single stall a cashier sells from.
func ResolveSellStall(in SellStallInput) SellStall {
	assigned := uniqueNonEmpty(in.AssignedIDs)
	switch {
	case in.ActiveMode == StallModeStall && in.ActiveStallID != "":
		return SellStall{OK: true, WarehouseID: in.ActiveStallID}
	case in.ActiveMode == StallModeAll:
		return blockedStall("all_stalls")
	case in.DefaultWarehouseID != "":
		return SellStall{OK: true, WarehouseID: in.DefaultWarehouseID}
	case len(assigned) == 1:
		return SellStall{OK: true, WarehouseID: assigned[0]}
	case len(assigned) == 0:
		return blockedStall("no_stall")
	}
	return blockedStall("multiple_unselected")
}

// SellScope is resolvePosSellScope's result: Mode "stall" (WarehouseID),
// "all", or "blocked" (Reason, Message).
type SellScope struct {
	Mode        string
	WarehouseID string
	Reason      string
	Message     string
}

// ResolveSellScope lets fully-scoped users sell across stalls ("all").
func ResolveSellScope(in SellStallInput, allStallsAllowed bool) SellScope {
	if allStallsAllowed {
		if in.ActiveMode == StallModeAll {
			return SellScope{Mode: "all"}
		}
		if in.ActiveMode == StallModeUnset && in.DefaultWarehouseID == "" && len(uniqueNonEmpty(in.AssignedIDs)) == 0 {
			return SellScope{Mode: "all"}
		}
	}
	single := ResolveSellStall(in)
	if single.OK {
		return SellScope{Mode: "stall", WarehouseID: single.WarehouseID}
	}
	return SellScope{Mode: "blocked", Reason: single.Reason, Message: single.Message}
}

// SellStallAllowed mirrors isSellStallAllowed.
func SellStallAllowed(warehouseID string, assignedIDs []string, canSwitch, unscoped bool, defaultWarehouseID string) bool {
	return unscoped || canSwitch || slices.Contains(assignedIDs, warehouseID) ||
		(defaultWarehouseID != "" && warehouseID == defaultWarehouseID)
}

// StallAllAccess mirrors computeStallAllAccess.
func StallAllAccess(role string, canSwitch, assignedMainStorage bool) bool {
	return role == "super_admin" || canSwitch || assignedMainStorage
}

// CanSellMixedStall mirrors canSellMixedStall: the central cashier menu,
// the user flag, and "Semua Stall" selected.
func CanSellMixedStall(hasCentralMenu, canCentralCheckout bool, activeMode string) bool {
	return hasCentralMenu && canCentralCheckout && activeMode == StallModeAll
}

// CentralCashierMenu is CENTRAL_CASHIER_MENU.
const CentralCashierMenu = "pos.cashier.central"

var stallCode = regexp.MustCompile(`^STALL-0*(\d+)$`)

func stallNumber(code string) (int, bool) {
	m := stallCode.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(code)))
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil
}

// SortStalls mirrors sortWarehouses: default first, then MAIN, other codes
// in natural order, then STALL-n by number.
func SortStalls(stalls []Stall) []Stall {
	out := slices.Clone(stalls)
	slices.SortStableFunc(out, compareStalls)
	return out
}

func compareStalls(a, b Stall) int {
	if a.IsDefault != b.IsDefault {
		if a.IsDefault {
			return -1
		}
		return 1
	}
	an, aStall := stallNumber(a.Code)
	bn, bStall := stallNumber(b.Code)
	switch {
	case aStall && bStall:
		return an - bn
	case aStall:
		return 1
	case bStall:
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

// naturalCompare approximates localeCompare(…, { numeric: true,
// sensitivity: "base" }): case-insensitive, digit runs by value.
func naturalCompare(a, b string) int {
	ar, br := []rune(strings.ToLower(a)), []rune(strings.ToLower(b))
	i, j := 0, 0
	for i < len(ar) && j < len(br) {
		if unicode.IsDigit(ar[i]) && unicode.IsDigit(br[j]) {
			si := i
			for i < len(ar) && unicode.IsDigit(ar[i]) {
				i++
			}
			sj := j
			for j < len(br) && unicode.IsDigit(br[j]) {
				j++
			}
			na := strings.TrimLeft(string(ar[si:i]), "0")
			nb := strings.TrimLeft(string(br[sj:j]), "0")
			if len(na) != len(nb) {
				return len(na) - len(nb)
			}
			if c := strings.Compare(na, nb); c != 0 {
				return c
			}
			continue
		}
		if ar[i] != br[j] {
			if ar[i] < br[j] {
				return -1
			}
			return 1
		}
		i, j = i+1, j+1
	}
	return (len(ar) - i) - (len(br) - j)
}

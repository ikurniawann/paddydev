package posops

import (
	"context"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"nuhabit/backend/internal/modules/posops/domain"
	"nuhabit/backend/internal/platform/scope"
	"nuhabit/backend/internal/platform/stalllabel"
)

// POS reports (app/api/pos/reports). Sales rows come from the Sales port;
// the shaping, rounding and labels follow the TS routes.

/* ── Stall filter (lib/pos/report-stall-filter.ts) ───────────────────── */

// BranchID, BranchName and Label are additive: Label is Name, or
// "<Name> — <BranchName>" when the picker mixes branches (stalllabel).
type stallOption struct {
	ID         string `json:"id"`
	Code       string `json:"code"`
	Name       string `json:"name"`
	BranchID   string `json:"branch_id"`
	BranchName string `json:"branch_name"`
	Label      string `json:"label"`
}

// display is what an export prints for the outlet: the label, else the name.
func (o stallOption) display() string {
	if o.Label != "" {
		return o.Label
	}
	return o.Name
}

// reportStalls is resolveReportStallFilter's result.
type reportStalls struct {
	WarehouseIDs []string // nil: every stall
	Options      []stallOption
	Locked       bool
	Selected     *string
}

var reportStallID = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// reportStallFilter limits a report to the caller's stalls: unscoped users
// and central cashiers pick any active stall of their branch, stall staff
// only their assignments. A stall outside that is an error.
func (s *Service) reportStallFilter(ctx context.Context, userID, requestedID string) (reportStalls, error) {
	var requested *string
	if reportStallID.MatchString(requestedID) {
		requested = &requestedID
	}
	sc, err := scope.Load(ctx, s.db, userID)
	if err != nil {
		return reportStalls{}, err
	}
	flags, err := s.ports.Stalls.Flags(ctx, s.db, userID)
	if err != nil {
		return reportStalls{}, err
	}
	superAdmin := sc.Role != nil && *sc.Role == "super_admin"
	if sc.Unscoped || superAdmin || flags.CanCentralCheckout {
		stalls, err := s.ports.Stalls.BranchByName(ctx, s.db, sc.BranchID)
		if err != nil {
			return reportStalls{}, err
		}
		out := reportStalls{Options: stallOptions(stalls)}
		if requested != nil {
			if !slices.ContainsFunc(out.Options, func(o stallOption) bool { return o.ID == *requested }) {
				return reportStalls{}, &jsError{msg: "Outlet tidak valid atau di luar scope"}
			}
			out.WarehouseIDs, out.Selected = []string{*requested}, requested
		}
		return out, nil
	}
	assigned, err := s.ports.Stalls.Assigned(ctx, s.db, userID)
	if err != nil {
		return reportStalls{}, err
	}
	options := stallOptions(assigned)
	switch {
	case len(options) == 0:
		return reportStalls{WarehouseIDs: []string{}, Options: options, Locked: true}, nil
	case requested != nil:
		if !slices.ContainsFunc(options, func(o stallOption) bool { return o.ID == *requested }) {
			return reportStalls{}, &jsError{msg: "Outlet tidak sesuai assignment login"}
		}
		return reportStalls{WarehouseIDs: []string{*requested}, Options: options, Locked: len(options) == 1, Selected: requested}, nil
	case len(options) == 1:
		return reportStalls{WarehouseIDs: []string{options[0].ID}, Options: options, Locked: true, Selected: &options[0].ID}, nil
	}
	ids := make([]string, len(options))
	for i, o := range options {
		ids[i] = o.ID
	}
	return reportStalls{WarehouseIDs: ids, Options: options}, nil
}

func stallBranchID(st domain.Stall) string {
	if st.BranchID == nil {
		return ""
	}
	return *st.BranchID
}

// stallOptions orders the picker by branch (Main Storage first within a
// branch) and labels every row by the one rule of stalllabel.
func stallOptions(stalls []domain.Stall) []stallOption {
	sorted := stalllabel.SortByBranch(stalls, func(st domain.Stall) stalllabel.Key {
		return stalllabel.Key{BranchName: st.BranchName, BranchID: stallBranchID(st), IsDefault: st.IsDefault}
	}, nil)
	withBranch := stalllabel.Spans(sorted, stallBranchID)
	out := make([]stallOption, len(sorted))
	for i, st := range sorted {
		out[i] = stallOption{ID: st.ID, Code: st.Code, Name: st.Name, BranchID: stallBranchID(st), BranchName: st.BranchName,
			Label: stalllabel.Label(st.Name, st.BranchName, withBranch)}
	}
	return out
}

// none reports a stall filter that matches nothing.
func (f reportStalls) none() bool { return f.WarehouseIDs != nil && len(f.WarehouseIDs) == 0 }

// reportHead is the filters / stall_options / stall_locked head of a
// stall-filtered report; extra filter pairs follow warehouse_id.
func reportHead(r domain.ReportRange, f reportStalls, extraFilters ...any) *Obj {
	filters := NewObj("date_from", r.DateFrom, "date_to", r.DateTo, "warehouse_id", f.Selected)
	for i := 0; i+1 < len(extraFilters); i += 2 {
		filters.Set(extraFilters[i].(string), extraFilters[i+1])
	}
	return NewObj("filters", filters, "stall_options", f.Options, "stall_locked", f.Locked)
}

// reportRange parses date_from/date_to like parseReportDateRange; a
// reversed range is the Error the routes catch.
func (s *Service) reportRange(from, to string) (domain.ReportRange, error) {
	r, msg := domain.ParseReportRange(from, to, s.now())
	if msg != "" {
		return r, &jsError{msg: msg}
	}
	return r, nil
}

/* ── Rush hour ───────────────────────────────────────────────────────── */

// RushHourReport mirrors GET /api/pos/reports/rush-hour.
func (s *Service) RushHourReport(ctx context.Context, userID, from, to, warehouseID string) (*Obj, error) {
	r, f, rep, err := s.rushHour(ctx, userID, from, to, warehouseID)
	if err != nil {
		return nil, err
	}
	return reportHead(r, f).Set("summary", rep.Summary).Set("peak_hour", rep.PeakHour).
		Set("peak_revenue_hour", rep.PeakRevenueHour).Set("peak_day", rep.PeakDay).Set("hourly", rep.Hourly).
		Set("weekdays", rep.Weekdays).Set("heatmap", rep.Heatmap), nil
}

// rushHour builds the rush hour report of the caller's stalls.
func (s *Service) rushHour(ctx context.Context, userID, from, to, warehouseID string) (domain.ReportRange, reportStalls, domain.RushHourReport, error) {
	r, f, err := s.stallReport(ctx, userID, from, to, warehouseID)
	if err != nil {
		return r, f, domain.RushHourReport{}, err
	}
	var points []domain.RushHourPoint
	if !f.none() {
		if points, err = s.ports.Sales.RushHourPoints(ctx, s.db, r.StartIso, r.EndIso, f.WarehouseIDs); err != nil {
			return r, f, domain.RushHourReport{}, err
		}
	}
	return r, f, domain.BuildRushHourReport(points), nil
}

/* ── Voids ───────────────────────────────────────────────────────────── */

func actorName(name *string, fallback string) string {
	if n := domain.TrimJS(orDefault(name, "")); n != "" {
		return n
	}
	return fallback
}

// VoidReport mirrors GET /api/pos/reports/voids.
func (s *Service) VoidReport(ctx context.Context, userID, from, to, warehouseID string) (*Obj, error) {
	r, err := s.reportRange(from, to)
	if err != nil {
		return nil, err
	}
	f, err := s.reportStallFilter(ctx, userID, warehouseID)
	if err != nil {
		return nil, err
	}
	head := reportHead(r, f)
	if f.none() {
		return head.Set("summary", NewObj("voids", 0, "amount", 0)).Set("rows", []any{}), nil
	}
	orders, err := s.ports.Sales.VoidedOrders(ctx, s.db, r.StartIso, r.EndIso, f.WarehouseIDs)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(orders))
	for i, o := range orders {
		ids[i] = o.ID
	}
	itemsByOrder := map[string][]*Obj{}
	if len(ids) > 0 {
		lines, err := s.ports.Sales.OrderLines(ctx, s.db, ids)
		if err != nil {
			return nil, err
		}
		for _, l := range lines {
			name := "—"
			if l.ProductName != nil && *l.ProductName != "" {
				name = *l.ProductName
			}
			itemsByOrder[l.OrderID] = append(itemsByOrder[l.OrderID], NewObj("id", l.ID, "product_name", name,
				"product_sku", l.ProductSku, "quantity", numOrZero(l.Quantity), "unit_price", numOrZero(l.UnitPrice),
				"total_amount", numOrZero(l.TotalAmount)))
		}
	}
	rows := make([]*Obj, len(orders))
	amount := 0.0
	for i, o := range orders {
		items := itemsByOrder[o.ID]
		if items == nil {
			items = []*Obj{}
		}
		total := numOrZero(o.TotalAmount)
		amount += total
		rows[i] = NewObj("id", o.ID, "order_number", o.OrderNumber, "checkout_number", o.CheckoutNumber,
			"checkout_id", o.CheckoutID, "ordered_at", jsTimePtr(o.OrderedAt), "voided_at", jsTimePtr(o.VoidedAt),
			"void_reason", o.VoidReason, "created_by_name", actorName(o.CreatedByName, "Kasir"),
			"voided_by_name", actorName(o.VoidedByName, "Supervisor"), "stall_name", o.StallName, "stall_code", o.StallCode,
			"total_amount", total, "payment_method", o.PaymentMethod, "payment_method_code", o.PaymentMethodCode,
			"payment_method_name", o.PaymentMethodName, "sold_from", o.SoldFrom, "items", items)
	}
	return head.Set("summary", NewObj("voids", len(rows), "amount", amount)).Set("rows", rows), nil
}

/* ── Payment methods ─────────────────────────────────────────────────── */

type methodTotal struct {
	key                string
	method, code, name *string
	amount             float64
	payments           int
	orders             map[string]bool
}

// PaymentMethodReport mirrors GET /api/pos/reports/payment-methods: one
// leg per payment (split payments apart), per method and per period,
// with every active catalog method as a column.
func (s *Service) PaymentMethodReport(ctx context.Context, userID, from, to, warehouseID, granularityRaw string) (*Obj, error) {
	r, err := s.reportRange(from, to)
	if err != nil {
		return nil, err
	}
	granularity := domain.ParseGranularity(granularityRaw)
	f, err := s.reportStallFilter(ctx, userID, warehouseID)
	if err != nil {
		return nil, err
	}
	head := reportHead(r, f, "granularity", granularity)
	periods := domain.EnumeratePeriods(r.DateFrom, r.DateTo, granularity)
	if f.none() {
		series := make([]*Obj, len(periods))
		for i, p := range periods {
			series[i] = NewObj("period", p, "label", domain.PeriodLabel(p, granularity), "total_amount", 0,
				"payment_count", 0, "by_method", []any{})
		}
		return head.Set("summary", NewObj("total_amount", 0, "payment_count", 0, "order_count", 0, "method_count", 0)).
			Set("by_method", []any{}).Set("method_columns", []any{}).Set("series", series), nil
	}
	legs, err := s.ports.Sales.PaymentLegs(ctx, s.db, r.StartIso, r.EndIso, f.WarehouseIDs)
	if err != nil {
		return nil, err
	}
	catalog, err := activePaymentCatalog(ctx, s.db)
	if err != nil {
		catalog = nil // the TS falls back to no catalog
	}
	catalogByCode := map[string]paymentCatalogRow{}
	for _, c := range catalog {
		catalogByCode[strings.ToLower(domain.TrimJS(c.Code))] = c
	}

	var order []string
	totals := map[string]*methodTotal{}
	periodTotals := map[string]map[string]*methodTotal{}
	allOrders := map[string]bool{}
	bump := func(m map[string]*methodTotal, key string, leg PaymentLeg, amount float64) {
		cur, ok := m[key]
		if !ok {
			cur = &methodTotal{key: key, method: leg.PaymentMethod, code: leg.PaymentMethodCode, name: leg.PaymentMethodName, orders: map[string]bool{}}
			m[key] = cur
		}
		if nonEmptyPtr(cur.code) == nil && nonEmptyPtr(leg.PaymentMethodCode) != nil {
			cur.code = leg.PaymentMethodCode
		}
		if nonEmptyPtr(cur.name) == nil && nonEmptyPtr(leg.PaymentMethodName) != nil {
			cur.name = leg.PaymentMethodName
		}
		if nonEmptyPtr(cur.method) == nil && nonEmptyPtr(leg.PaymentMethod) != nil {
			cur.method = leg.PaymentMethod
		}
		cur.amount += amount
		cur.payments++
		cur.orders[leg.OrderID] = true
	}
	for _, leg := range legs {
		key := domain.PaymentMethodKey(leg.PaymentMethod, leg.PaymentMethodCode)
		amount := numOrZero(leg.Amount)
		period := domain.PeriodKey(leg.HariWib, granularity)
		allOrders[leg.OrderID] = true
		if _, seen := totals[key]; !seen {
			order = append(order, key)
		}
		bump(totals, key, leg, amount)
		if periodTotals[period] == nil {
			periodTotals[period] = map[string]*methodTotal{}
		}
		bump(periodTotals[period], key, leg, amount)
	}
	label := func(t *methodTotal) string {
		cat, inCatalog := catalogByCode[t.key]
		method := orDefault(nonEmptyPtr(t.method), t.key)
		code := orDefault(nonEmptyPtr(t.code), "")
		if code == "" {
			code = t.key
			if inCatalog {
				code = cat.Code
			}
		}
		name := orDefault(nonEmptyPtr(t.name), "")
		if name == "" && inCatalog {
			name = cat.Name
		}
		return domain.PaymentMethodLabel(method, code, name)
	}
	totalAmount := 0.0
	for _, k := range order {
		totalAmount += totals[k].amount
	}
	type methodRow struct {
		obj    *Obj
		label  string
		amount float64
	}
	rows := make([]methodRow, 0, len(order))
	for _, k := range order {
		t := totals[k]
		amount := domain.RoundCurrency(t.amount)
		pct := 0.0
		if totalAmount > 0 {
			pct = domain.Round(amount/totalAmount*1000) / 10
		}
		l := label(t)
		rows = append(rows, methodRow{label: l, amount: amount, obj: NewObj("method_key", k, "payment_method", t.method,
			"payment_method_code", t.code, "payment_method_name", t.name, "label", l, "amount", amount,
			"payment_count", t.payments, "order_count", len(t.orders), "pct", pct)})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].amount != rows[j].amount {
			return rows[i].amount > rows[j].amount
		}
		return domain.LocaleCompare(rows[i].label, rows[j].label) < 0
	})

	type column struct {
		key, label string
		sort       float64
	}
	var columns []column
	seenColumn := map[string]bool{}
	byMethod := make([]*Obj, len(rows))
	for i, row := range rows {
		byMethod[i] = row.obj
		key := row.obj.Str("method_key")
		sortOrder := 10_000.0
		if cat, ok := catalogByCode[key]; ok {
			sortOrder = float64(cat.SortOrder)
		}
		columns, seenColumn[key] = append(columns, column{key, row.label, sortOrder}), true
	}
	for _, c := range catalog {
		key := strings.ToLower(domain.TrimJS(c.Code))
		if seenColumn[key] {
			continue
		}
		seenColumn[key] = true
		columns = append(columns, column{key, domain.PaymentMethodLabel(key, c.Code, c.Name), float64(c.SortOrder)})
	}
	sort.SliceStable(columns, func(i, j int) bool {
		if columns[i].sort != columns[j].sort {
			return columns[i].sort < columns[j].sort
		}
		return domain.LocaleCompare(columns[i].label, columns[j].label) < 0
	})
	methodColumns := make([]*Obj, len(columns))
	for i, c := range columns {
		methodColumns[i] = NewObj("method_key", c.key, "label", c.label)
	}
	series := make([]*Obj, len(periods))
	for i, p := range periods {
		bucket := periodTotals[p]
		cells := make([]*Obj, len(columns))
		sum, count := 0.0, 0
		for j, c := range columns {
			amount, payments := 0.0, 0
			if hit, ok := bucket[c.key]; ok {
				amount, payments = domain.RoundCurrency(hit.amount), hit.payments
			}
			sum += amount
			count += payments
			cells[j] = NewObj("method_key", c.key, "label", c.label, "amount", amount, "payment_count", payments)
		}
		series[i] = NewObj("period", p, "label", domain.PeriodLabel(p, granularity),
			"total_amount", domain.RoundCurrency(sum), "payment_count", count, "by_method", cells)
	}
	return head.Set("summary", NewObj("total_amount", domain.RoundCurrency(totalAmount), "payment_count", len(legs),
		"order_count", len(allOrders), "method_count", len(byMethod))).
		Set("by_method", byMethod).Set("method_columns", methodColumns).Set("series", series), nil
}

/* ── Profit and revenue composition (UTC calendar days) ──────────────── */

var jsISODay = regexp.MustCompile(`^(\d{4})(?:-(\d{2})(?:-(\d{2}))?)?$`)

// utcDay is new Date(`${day}T<clock>Z`): ok is false for an Invalid Date.
func utcDay(day string, end bool) (time.Time, bool) {
	m := jsISODay.FindStringSubmatch(day)
	if m == nil {
		return time.Time{}, false
	}
	y, _ := strconv.Atoi(m[1])
	mo, d := 1, 1
	if m[2] != "" {
		mo, _ = strconv.Atoi(m[2])
	}
	if m[3] != "" {
		d, _ = strconv.Atoi(m[3])
	}
	if mo < 1 || mo > 12 || d < 1 || d > 31 {
		return time.Time{}, false
	}
	if end {
		return time.Date(y, time.Month(mo), d, 23, 59, 59, 999_000_000, time.UTC), true
	}
	return time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC), true
}

// utcRange resolves date_from/date_to (default: the UTC month to date) to
// [from 00:00Z, to 23:59:59.999Z]; a bad date is the RangeError of
// Date.prototype.toISOString.
func (s *Service) utcRange(from, to string) (string, string, time.Time, time.Time, error) {
	now := s.now().UTC()
	if from == "" {
		from = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	}
	if to == "" {
		to = now.Format("2006-01-02")
	}
	start, ok1 := utcDay(from, false)
	end, ok2 := utcDay(to, true)
	if !ok1 || !ok2 {
		return "", "", time.Time{}, time.Time{}, &jsError{msg: "Invalid time value"}
	}
	return from, to, start, end, nil
}

// productCategories maps POS product ids to their POS category name and
// master item category code.
type productCategories struct {
	posName  map[string]*string // product id -> POS category name
	itemCode map[string]*string // product id -> item category code
	labels   map[string]string
}

func (s *Service) categoriesOf(ctx context.Context, productIDs []string, posFallback string) (productCategories, error) {
	pc := productCategories{posName: map[string]*string{}, itemCode: map[string]*string{}, labels: map[string]string{}}
	if len(productIDs) == 0 {
		return pc, nil
	}
	names, err := posCategoryNames(ctx, s.db, productIDs, posFallback)
	if err != nil {
		return pc, err
	}
	pc.posName = names
	if pc.itemCode, err = s.ports.Inventory.ItemKategori(ctx, s.db, productIDs); err != nil {
		return pc, err
	}
	cats, err := s.ports.Inventory.ItemCategories(ctx, s.db)
	if err != nil {
		return pc, err
	}
	pc.labels = domain.ItemCategoryLabels(cats)
	return pc, nil
}

func lineProductIDs(lines []SaleLine) []string {
	var ids []string
	for _, l := range lines {
		if l.ProductID != nil && *l.ProductID != "" && !slices.Contains(ids, *l.ProductID) {
			ids = append(ids, *l.ProductID)
		}
	}
	return ids
}

type profitBucket struct {
	id, label                            string
	quantity, revenue, cogs, grossProfit float64
}

type profitBuckets struct {
	order []string
	byID  map[string]*profitBucket
}

func (b *profitBuckets) add(id, label string, qty, revenue, cogs, profit float64) {
	if b.byID == nil {
		b.byID = map[string]*profitBucket{}
	}
	cur, ok := b.byID[id]
	if !ok {
		cur = &profitBucket{id: id, label: label}
		b.byID[id] = cur
		b.order = append(b.order, id)
	}
	cur.quantity += qty
	cur.revenue += revenue
	cur.cogs += cogs
	cur.grossProfit += profit
}

func marginPct(revenue, profit float64) float64 {
	if revenue > 0 {
		return domain.Round(profit/revenue*10000) / 100
	}
	return 0
}

// sorted is sortBuckets: rounded, by gross profit descending.
func (b *profitBuckets) sorted() []*Obj {
	type row struct {
		obj    *Obj
		id     string
		profit float64
	}
	rows := make([]row, len(b.order))
	for i, id := range b.order {
		c := b.byID[id]
		profit := domain.RoundCurrency(c.grossProfit)
		rows[i] = row{id: id, profit: profit, obj: NewObj("id", c.id, "label", c.label, "quantity", c.quantity,
			"revenue", domain.RoundCurrency(c.revenue), "cogs", domain.RoundCurrency(c.cogs), "gross_profit", profit,
			"gross_margin_pct", marginPct(c.revenue, c.grossProfit))}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].profit > rows[j].profit })
	out := make([]*Obj, len(rows))
	for i, r := range rows {
		out[i] = r.obj
	}
	return out
}

// ProfitReport mirrors GET /api/pos/reports/profit: revenue, COGS and
// gross profit of paid items, broken down by product, category, station,
// cashier and UTC date.
func (s *Service) ProfitReport(ctx context.Context, fromRaw, toRaw string) (*Obj, error) {
	from, to, start, end, err := s.utcRange(fromRaw, toRaw)
	if err != nil {
		return nil, err
	}
	filters := NewObj("date_from", from, "date_to", to)
	orders, err := s.ports.Sales.PaidOrders(ctx, s.db, start, end, true)
	if err != nil {
		return nil, err
	}
	if len(orders) == 0 {
		return NewObj("filters", filters,
			"summary", NewObj("orders", 0, "items", 0, "quantity", 0, "revenue", 0, "cogs", 0, "gross_profit", 0,
				"gross_margin_pct", 0, "zero_cost_items", 0),
			"breakdowns", NewObj("products", []any{}, "categories", []any{}, "stations", []any{}, "cashiers", []any{}, "dates", []any{})), nil
	}
	byID := map[string]PaidOrder{}
	ids := make([]string, len(orders))
	for i, o := range orders {
		ids[i], byID[o.ID] = o.ID, o
	}
	lines, err := s.ports.Sales.SaleLines(ctx, s.db, ids)
	if err != nil {
		return nil, err
	}
	cats, err := s.categoriesOf(ctx, lineProductIDs(lines), "Tanpa kategori")
	if err != nil {
		return nil, err
	}
	var products, categories, stations, cashiers, dates profitBuckets
	var quantity, revenue, cogs, gross float64
	zeroCost := 0
	for _, l := range lines {
		order, hasOrder := byID[l.OrderID]
		qty, rev, cost := numOrZero(l.Quantity), numOrZero(l.TotalAmount), numOrZero(l.CostTotal)
		profit := rev - cost
		if l.GrossProfit != nil {
			profit = numOrZero(l.GrossProfit)
		}
		productID := orDefault(nonEmptyPtr(l.ProductID), "unknown-product")
		productLabel := orDefault(nonEmptyPtr(l.ProductName), orDefault(nonEmptyPtr(l.ProductSku), "Produk tanpa nama"))
		var itemCode, posName *string
		if l.ProductID != nil && *l.ProductID != "" {
			itemCode, posName = nonEmptyPtr(cats.itemCode[*l.ProductID]), cats.posName[*l.ProductID]
		}
		catID, catLabel := domain.ProfitCategory(itemCode, domain.LookupItemCategoryName(itemCode, cats.labels), posName)
		station := orDefault(nonEmptyPtr(l.Station), "kitchen")
		cashierID, cashierLabel := "unknown-cashier", "Tanpa kasir"
		date := "Tanpa tanggal"
		if hasOrder {
			if c := nonEmptyPtr(order.CashierID); c != nil {
				cashierID, cashierLabel = *c, first8(*c)
			}
			if order.OrderedAt != nil {
				date = order.OrderedAt.UTC().Format("2006-01-02")
			}
		}
		quantity += qty
		revenue += rev
		cogs += cost
		gross += profit
		if rev > 0 && numOrZero(l.CostPrice) == 0 {
			zeroCost++
		}
		products.add(productID, productLabel, qty, rev, cost, profit)
		categories.add(catID, catLabel, qty, rev, cost, profit)
		stations.add(station, station, qty, rev, cost, profit)
		cashiers.add(cashierID, cashierLabel, qty, rev, cost, profit)
		dates.add(date, date, qty, rev, cost, profit)
	}
	dateRows := dates.sorted()
	sort.SliceStable(dateRows, func(i, j int) bool { return domain.LocaleCompare(dateRows[i].Str("id"), dateRows[j].Str("id")) < 0 })
	return NewObj("filters", filters,
		"summary", NewObj("orders", len(orders), "items", len(lines), "quantity", domain.RoundCurrency(quantity),
			"revenue", domain.RoundCurrency(revenue), "cogs", domain.RoundCurrency(cogs),
			"gross_profit", domain.RoundCurrency(gross), "gross_margin_pct", marginPct(revenue, gross),
			"zero_cost_items", zeroCost),
		"breakdowns", NewObj("products", products.sorted(), "categories", categories.sorted(),
			"stations", stations.sorted(), "cashiers", cashiers.sorted(), "dates", dateRows)), nil
}

type revenueBuckets struct {
	order []string
	byID  map[string]*domain.RevenueBucket
}

func (b *revenueBuckets) add(id, label string, qty, sales, cost float64) {
	if b.byID == nil {
		b.byID = map[string]*domain.RevenueBucket{}
	}
	cur, ok := b.byID[id]
	if !ok {
		cur = &domain.RevenueBucket{ID: id, Label: label}
		b.byID[id] = cur
		b.order = append(b.order, id)
	}
	cur.Quantity += qty
	cur.Sales += sales
	cur.Cost += cost
}

func (b *revenueBuckets) get(id string) domain.RevenueBucket {
	if c, ok := b.byID[id]; ok {
		return *c
	}
	return domain.RevenueBucket{ID: id, Label: domain.RevenueGroupLabels[id]}
}

// RevenueCompositionReport mirrors GET /api/pos/reports/revenue-composition:
// Food vs Beverage sales, cost and margin with per-category rows and a
// daily split.
func (s *Service) RevenueCompositionReport(ctx context.Context, fromRaw, toRaw string) (*Obj, error) {
	from, to, start, end, err := s.utcRange(fromRaw, toRaw)
	if err != nil {
		return nil, err
	}
	filters := NewObj("date_from", from, "date_to", to)
	orders, err := s.ports.Sales.PaidOrders(ctx, s.db, start, end, false)
	if err != nil {
		return nil, err
	}
	groupsOrder := []string{"food", "beverage"}
	if len(orders) == 0 {
		groups := make([]any, len(groupsOrder))
		for i, id := range groupsOrder {
			groups[i] = groupObj(domain.RevenueBucket{ID: id, Label: domain.RevenueGroupLabels[id]}.Finalize(0, 0), []domain.RevenueBucket{})
		}
		return NewObj("filters", filters,
			"summary", NewObj("orders", 0, "items", 0, "quantity", 0, "sales", 0, "cost", 0, "margin", 0, "cost_pct", 0,
				"zero_cost_items", 0),
			"groups", groups, "daily", []any{}), nil
	}
	orderedAt := map[string]*time.Time{}
	ids := make([]string, len(orders))
	for i, o := range orders {
		ids[i], orderedAt[o.ID] = o.ID, o.OrderedAt
	}
	lines, err := s.ports.Sales.SaleLines(ctx, s.db, ids)
	if err != nil {
		return nil, err
	}
	cats, err := s.categoriesOf(ctx, lineProductIDs(lines), "")
	if err != nil {
		return nil, err
	}
	var groupTotals revenueBuckets
	categoryTotals := map[string]*revenueBuckets{}
	var dayOrder []string
	daily := map[string]map[string]*domain.RevenueBucket{}
	var quantity, sales, cost float64
	zeroCost := 0
	for _, l := range lines {
		qty, rev, c := numOrZero(l.Quantity), numOrZero(l.TotalAmount), numOrZero(l.CostTotal)
		var posName, itemCode *string
		if l.ProductID != nil && *l.ProductID != "" {
			posName, itemCode = nonEmptyPtr(cats.posName[*l.ProductID]), nonEmptyPtr(cats.itemCode[*l.ProductID])
		}
		itemName := domain.LookupItemCategoryName(itemCode, cats.labels)
		group := domain.RevenueGroup(itemCode, posName, l.Station)
		quantity += qty
		sales += rev
		cost += c
		if rev > 0 && numOrZero(l.CostPrice) == 0 {
			zeroCost++
		}
		groupTotals.add(group, domain.RevenueGroupLabels[group], qty, rev, c)
		if categoryTotals[group] == nil {
			categoryTotals[group] = &revenueBuckets{}
		}
		label := orDefault(nonEmptyPtr(itemName), orDefault(posName, "Tanpa kategori"))
		categoryTotals[group].add(strings.ToLower(label), label, qty, rev, c)

		// String(date).slice(0, 10) on a UTC server: "Sun Oct 04".
		dayKey := "—"
		if at := orderedAt[l.OrderID]; at != nil {
			dayKey = at.UTC().Format("Mon Jan 02")
		}
		if daily[dayKey] == nil {
			dayOrder = append(dayOrder, dayKey)
			daily[dayKey] = map[string]*domain.RevenueBucket{
				"food":     {ID: "food", Label: "Food"},
				"beverage": {ID: "beverage", Label: "Beverage"},
			}
		}
		d := daily[dayKey][group]
		d.Quantity += qty
		d.Sales += rev
		d.Cost += c
	}
	groups := make([]any, len(groupsOrder))
	for i, id := range groupsOrder {
		bucket := groupTotals.get(id)
		var rows []domain.RevenueBucket
		if ct := categoryTotals[id]; ct != nil {
			for _, k := range ct.order {
				rows = append(rows, ct.byID[k].Finalize(bucket.Sales, bucket.Quantity))
			}
		}
		sort.SliceStable(rows, func(a, b int) bool { return rows[a].Sales > rows[b].Sales })
		if rows == nil {
			rows = []domain.RevenueBucket{}
		}
		groups[i] = groupObj(bucket.Finalize(sales, quantity), rows)
	}
	sort.SliceStable(dayOrder, func(i, j int) bool { return domain.LocaleCompare(dayOrder[i], dayOrder[j]) < 0 })
	days := make([]*Obj, len(dayOrder))
	for i, k := range dayOrder {
		food, bev := daily[k]["food"], daily[k]["beverage"]
		daySales, dayQty := food.Sales+bev.Sales, food.Quantity+bev.Quantity
		days[i] = NewObj("date", k, "food", food.Finalize(daySales, dayQty), "beverage", bev.Finalize(daySales, dayQty))
	}
	return NewObj("filters", filters,
		"summary", NewObj("orders", len(orders), "items", len(lines), "quantity", quantity,
			"sales", domain.RoundCurrency(sales), "cost", domain.RoundCurrency(cost),
			"margin", domain.RoundCurrency(sales-cost), "cost_pct", domain.SafePct(cost, sales), "zero_cost_items", zeroCost),
		"groups", groups, "daily", days), nil
}

// groupObj is { ...finalizeBucket(group), categories }.
func groupObj(b domain.RevenueBucket, categories []domain.RevenueBucket) *Obj {
	return NewObj("id", b.ID, "label", b.Label, "quantity", b.Quantity, "sales", b.Sales, "cost", b.Cost,
		"margin", b.Margin, "cost_pct", b.CostPct, "sales_share_pct", b.SalesSharePct, "qty_share_pct", b.QtySharePct,
		"categories", categories)
}

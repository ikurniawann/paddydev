package posops

import (
	"context"
	"sort"

	"nuhabit/backend/internal/modules/posops/domain"
	"nuhabit/backend/internal/platform/httpx"
)

// Product sales and transaction reports (app/api/pos/reports/product-sales
// and transactions). Rows come from the ReportRows port; amounts are
// Math.round(x * 100) / 100 where the TS rounds them.

// reportFilters is the filters object of a stall-filtered report.
type reportFilters struct {
	DateFrom    string  `json:"date_from"`
	DateTo      string  `json:"date_to"`
	WarehouseID *string `json:"warehouse_id"`
}

// stallReportHead is filters, stall_options and stall_locked.
type stallReportHead struct {
	Filters      reportFilters `json:"filters"`
	StallOptions []stallOption `json:"stall_options"`
	StallLocked  bool          `json:"stall_locked"`
}

func newStallReportHead(r domain.ReportRange, f reportStalls) stallReportHead {
	return stallReportHead{
		Filters:      reportFilters{DateFrom: r.DateFrom, DateTo: r.DateTo, WarehouseID: f.Selected},
		StallOptions: f.Options,
		StallLocked:  f.Locked,
	}
}

// stallFilterLabel is the export's "Semua stall", "Name (CODE)" or the raw id.
func (h stallReportHead) stallFilterLabel() string {
	id := h.Filters.WarehouseID
	if id == nil || *id == "" {
		return "Semua outlet"
	}
	for _, o := range h.StallOptions {
		if o.ID == *id {
			return o.display() + " (" + o.Code + ")"
		}
	}
	return *id
}

// stallReport resolves the range and the stall filter of a report.
func (s *Service) stallReport(ctx context.Context, userID, from, to, warehouseID string) (domain.ReportRange, reportStalls, error) {
	r, err := s.reportRange(from, to)
	if err != nil {
		return r, reportStalls{}, err
	}
	f, err := s.reportStallFilter(ctx, userID, warehouseID)
	return r, f, err
}

/* ── Product sales ───────────────────────────────────────────────────── */

// ProductSalesReport is the product sales report body.
type ProductSalesReport struct {
	stallReportHead
	Summary ProductSalesSummary `json:"summary"`
	Rows    []ProductSalesLine  `json:"rows"`
}

// ProductSalesSummary totals the product rows.
type ProductSalesSummary struct {
	Products int     `json:"products"`
	Quantity float64 `json:"quantity"`
	Revenue  float64 `json:"revenue"`
}

// ProductSalesLine is one product and stall.
type ProductSalesLine struct {
	ProductID   *string `json:"product_id"`
	ProductName string  `json:"product_name"`
	ProductSku  *string `json:"product_sku"`
	WarehouseID *string `json:"warehouse_id"`
	StallCode   *string `json:"stall_code"`
	StallName   *string `json:"stall_name"`
	Quantity    float64 `json:"quantity"`
	Revenue     float64 `json:"revenue"`
	OrderCount  float64 `json:"order_count"`
}

// ProductSalesReport mirrors GET /api/pos/reports/product-sales.
func (s *Service) ProductSalesReport(ctx context.Context, userID, from, to, warehouseID string) (*ProductSalesReport, error) {
	r, f, err := s.stallReport(ctx, userID, from, to, warehouseID)
	if err != nil {
		return nil, err
	}
	out := &ProductSalesReport{stallReportHead: newStallReportHead(r, f), Rows: []ProductSalesLine{}}
	if f.none() {
		return out, nil
	}
	rows, err := s.ports.ReportRows.ProductSales(ctx, s.db, r.StartIso, r.EndIso, f.WarehouseIDs)
	if err != nil {
		return nil, err
	}
	revenue := 0.0
	for _, row := range rows {
		line := ProductSalesLine{
			ProductID: row.ProductID, ProductName: orDefault(nonEmptyPtr(row.ProductName), "Unknown"),
			ProductSku: row.ProductSku, WarehouseID: row.WarehouseID, StallCode: row.StallCode, StallName: row.StallName,
			Quantity: numOrZero(row.Quantity), Revenue: domain.RoundCurrency(numOrZero(row.Revenue)),
			OrderCount: numOrZero(row.OrderCount),
		}
		out.Rows = append(out.Rows, line)
		out.Summary.Products++
		out.Summary.Quantity += line.Quantity
		revenue += line.Revenue
	}
	out.Summary.Revenue = domain.RoundCurrency(revenue)
	return out, nil
}

/* ── Transactions ────────────────────────────────────────────────────── */

// TransactionReport is the transaction report body.
type TransactionReport struct {
	stallReportHead
	Summary     TransactionSummary `json:"summary"`
	PerStall    []StallSales       `json:"per_stall"`
	TopProducts []TopProduct       `json:"top_products"`
	Daily       []DailySales       `json:"daily"`
	Rows        []TransactionLine  `json:"rows"`
}

// TransactionSummary is summarizeSales under the route's names
// (total_sales stays for older clients).
type TransactionSummary struct {
	Transactions int     `json:"transactions"`
	TotalSales   float64 `json:"total_sales"`
	TotalArkUsed float64 `json:"total_ark_used"`
	Revenue      float64 `json:"revenue"`
	Discount     float64 `json:"discount"`
	Tax          float64 `json:"tax"`
	Service      float64 `json:"service"`
	Nett         float64 `json:"nett"`
}

// StallSales is one stall of per_stall.
type StallSales struct {
	StallCode    *string `json:"stall_code"`
	StallName    string  `json:"stall_name"`
	Transactions float64 `json:"transactions"`
	Quantity     float64 `json:"quantity"`
	Sales        float64 `json:"sales"`
}

// TopProduct is one product of top_products.
type TopProduct struct {
	ProductName string  `json:"product_name"`
	Quantity    float64 `json:"quantity"`
	Revenue     float64 `json:"revenue"`
}

// DailySales is one WIB day of daily.
type DailySales struct {
	Date         string  `json:"date"`
	Nett         float64 `json:"nett"`
	Transactions int     `json:"transactions"`
}

// TransactionLine is one order of rows.
type TransactionLine struct {
	ID                  string        `json:"id"`
	OrderNumber         *string       `json:"order_number"`
	OrderedAt           *httpx.JSTime `json:"ordered_at"`
	Status              *string       `json:"status"`
	PaymentStatus       *string       `json:"payment_status"`
	PaymentMethod       *string       `json:"payment_method"`
	PaymentMethodCode   *string       `json:"payment_method_code"`
	PaymentMethodName   *string       `json:"payment_method_name"`
	Subtotal            float64       `json:"subtotal"`
	DiscountAmount      float64       `json:"discount_amount"`
	TaxAmount           float64       `json:"tax_amount"`
	ServiceChargeAmount float64       `json:"service_charge_amount"`
	TotalAmount         float64       `json:"total_amount"`
	ArkCoinsUsed        float64       `json:"ark_coins_used"`
	CashierID           *string       `json:"cashier_id"`
	WarehouseID         *string       `json:"warehouse_id"`
	StallCode           *string       `json:"stall_code"`
	StallName           *string       `json:"stall_name"`
	CheckoutID          *string       `json:"checkout_id"`
	CheckoutNumber      *string       `json:"checkout_number"`
	SoldFrom            *string       `json:"sold_from"`
	XenditQrID          *string       `json:"xendit_qr_id"`
	XenditExternalID    *string       `json:"xendit_external_id"`
	CompType            *string       `json:"comp_type"`
	CompApprovedName    *string       `json:"comp_approved_name"`
}

// TransactionReport mirrors GET /api/pos/reports/transactions: every paid
// order is stall revenue (checkout totals are never added), with a WIB
// daily trend, an item-based per-stall split and the top products of the
// same orders.
func (s *Service) TransactionReport(ctx context.Context, userID, from, to, warehouseID string) (*TransactionReport, error) {
	r, f, err := s.stallReport(ctx, userID, from, to, warehouseID)
	if err != nil {
		return nil, err
	}
	out := &TransactionReport{stallReportHead: newStallReportHead(r, f), PerStall: []StallSales{},
		TopProducts: []TopProduct{}, Daily: []DailySales{}, Rows: []TransactionLine{}}
	if f.none() {
		return out, nil
	}
	rows, err := s.ports.ReportRows.Transactions(ctx, s.db, r.StartIso, r.EndIso, f.WarehouseIDs)
	if err != nil {
		return nil, err
	}

	var revenue, discount, tax, service, nett, ark float64
	daily := map[string]*DailySales{}
	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
		line := TransactionLine{
			ID: row.ID, OrderNumber: row.OrderNumber, OrderedAt: jsTimePtr(row.OrderedAt), Status: row.Status,
			PaymentStatus: row.PaymentStatus, PaymentMethod: row.PaymentMethod, PaymentMethodCode: row.PaymentMethodCode,
			PaymentMethodName: row.PaymentMethodName, Subtotal: numOrZero(row.Subtotal),
			DiscountAmount: numOrZero(row.DiscountAmount), TaxAmount: numOrZero(row.TaxAmount),
			ServiceChargeAmount: numOrZero(row.ServiceChargeAmount), TotalAmount: numOrZero(row.TotalAmount),
			ArkCoinsUsed: numOrZero(row.ArkCoinsUsed), CashierID: row.CashierID, WarehouseID: row.WarehouseID,
			StallCode: row.StallCode, StallName: row.StallName, CheckoutID: row.CheckoutID,
			CheckoutNumber: row.CheckoutNumber, SoldFrom: row.SoldFrom, XenditQrID: row.XenditQrID,
			XenditExternalID: row.XenditExternalID, CompType: row.CompType, CompApprovedName: row.CompApprovedName,
		}
		out.Rows = append(out.Rows, line)
		revenue += line.Subtotal
		discount += line.DiscountAmount
		tax += line.TaxAmount
		service += line.ServiceChargeAmount
		nett += line.TotalAmount
		ark += line.ArkCoinsUsed
		day := daily[row.HariWib]
		if day == nil {
			day = &DailySales{Date: row.HariWib}
			daily[row.HariWib] = day
		}
		day.Nett += line.TotalAmount
		day.Transactions++
	}
	nett = domain.RoundCurrency(nett)
	out.Summary = TransactionSummary{
		Transactions: len(rows), TotalSales: nett, TotalArkUsed: domain.RoundCurrency(ark),
		Revenue: domain.RoundCurrency(revenue), Discount: domain.RoundCurrency(discount), Tax: domain.RoundCurrency(tax),
		Service: domain.RoundCurrency(service), Nett: nett,
	}
	for _, day := range daily {
		out.Daily = append(out.Daily, DailySales{Date: day.Date, Nett: domain.RoundCurrency(day.Nett), Transactions: day.Transactions})
	}
	sort.Slice(out.Daily, func(i, j int) bool { return out.Daily[i].Date < out.Daily[j].Date })
	if len(ids) == 0 {
		return out, nil
	}

	stalls, err := s.ports.ReportRows.StallSales(ctx, s.db, ids)
	if err != nil {
		return nil, err
	}
	for _, st := range stalls {
		out.PerStall = append(out.PerStall, StallSales{
			StallCode: st.StallCode, StallName: orDefault(st.StallName, "Tanpa Outlet"),
			Transactions: numOrZero(st.Transactions), Quantity: round2Ptr(st.Quantity), Sales: round2Ptr(st.Sales),
		})
	}
	top, err := s.ports.ReportRows.TopProducts(ctx, s.db, ids)
	if err != nil {
		return nil, err
	}
	for _, p := range top {
		out.TopProducts = append(out.TopProducts, TopProduct{ProductName: p.ProductName,
			Quantity: round2Ptr(p.Quantity), Revenue: round2Ptr(p.Revenue)})
	}
	return out, nil
}

// round2Ptr is Math.round(toNumber(v) * 100) / 100 of a float8 column.
func round2Ptr(v *float64) float64 {
	if v == nil {
		return 0
	}
	return domain.Round2Safe(*v)
}

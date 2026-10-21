package posops

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"

	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/scope"
	"nuhabit/backend/internal/platform/xlsx"
)

// Report routes that answer with a workbook: product-sales and
// transactions (JSON, or ?format=xlsx), export (the styled workbook of any
// report) and rush-hour/export.

// writeXlsx sends an attachment; the TS rush hour export sets no
// Cache-Control, the others no-store.
func writeXlsx(w http.ResponseWriter, body []byte, filename string, noStore bool) error {
	h := w.Header()
	h.Set("Content-Type", xlsx.ContentType)
	h.Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	if noStore {
		h.Set("Cache-Control", "no-store")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
	return nil
}

func (h *Handler) productSalesReport(w http.ResponseWriter, r *http.Request) error {
	user, err := h.requirePos(r, "Authentication required")
	if err != nil {
		return err
	}
	q := r.URL.Query()
	data, err := h.svc.ProductSalesReport(r.Context(), user.ID, q.Get("date_from"), q.Get("date_to"), q.Get("warehouse_id"))
	if err != nil {
		return err
	}
	if q.Get("format") != "xlsx" {
		return okData(w, data)
	}
	body, err := xlsx.Build(productSalesSheets(data)...)
	if err != nil {
		return err
	}
	return writeXlsx(w, body, productSalesFileName(data.Filters), true)
}

func (h *Handler) transactionReport(w http.ResponseWriter, r *http.Request) error {
	user, err := h.requirePos(r, "Authentication required")
	if err != nil {
		return err
	}
	q := r.URL.Query()
	data, err := h.svc.TransactionReport(r.Context(), user.ID, q.Get("date_from"), q.Get("date_to"), q.Get("warehouse_id"))
	if err != nil {
		return err
	}
	if q.Get("format") != "xlsx" {
		return okData(w, data)
	}
	body, err := xlsx.Build(transactionSheets(data)...)
	if err != nil {
		return err
	}
	return writeXlsx(w, body, transactionFileName(data.Filters), true)
}

// stallLabel names the selected stall of an export, "Semua stall" for none.
func stallLabel(selected *string, options []stallOption) string {
	if selected == nil || *selected == "" {
		return "Semua outlet"
	}
	for _, o := range options {
		if o.ID == *selected {
			return o.display()
		}
	}
	return "Outlet terpilih"
}

/* ── Rush hour export ────────────────────────────────────────────────── */

// rushHourExport mirrors GET /api/pos/reports/rush-hour/export: four
// sheets, with the share of the range_from..range_to hours.
func (h *Handler) rushHourExport(w http.ResponseWriter, r *http.Request) error {
	user, err := h.requirePos(r, "Authentication required")
	if err != nil {
		return err
	}
	ctx := r.Context()
	sc, err := scope.Load(ctx, h.svc.db, user.ID)
	if err != nil {
		return err
	}
	q := r.URL.Query()
	rg, f, rep, err := h.svc.rushHour(ctx, user.ID, q.Get("date_from"), q.Get("date_to"), q.Get("warehouse_id"))
	if err != nil {
		return err
	}
	rangeFrom := clampHour(q.Get("range_from"), 7)
	body, err := xlsx.Build(rushHourSheets(rep, rushHourMeta{
		company:     h.svc.ports.Directory.BrandName(ctx, h.svc.db, sc.CompanyID),
		period:      rg.DateFrom + " s.d. " + rg.DateTo,
		stall:       stallLabel(f.Selected, f.Options),
		rangeFrom:   rangeFrom,
		rangeTo:     max(rangeFrom, clampHour(q.Get("range_to"), 12)),
		generatedAt: h.svc.now(),
	})...)
	if err != nil {
		return err
	}
	return writeXlsx(w, body, "rush-hour-"+rg.DateFrom+"_"+rg.DateTo+".xlsx", false)
}

/* ── Export (Desktop → Drive → Reports) ──────────────────────────────── */

type exportQuery struct{ from, to, warehouseID string }

// reportExport is one REPORT_EXPORTS entry: how its report route answers
// an error, how to run the report, and its workbook built from the report JSON.
type reportExport struct {
	failed func(error) (int, string)
	load   func(ctx context.Context, s *Service, userID string, q exportQuery) (any, error)
	build  func([]byte, exportMeta) (*xlsx.Report, error)
}

var reportExports = map[string]reportExport{
	"profit": {status500(fixed("Gagal memuat laporan profit POS")),
		func(ctx context.Context, s *Service, _ string, q exportQuery) (any, error) {
			return s.ProfitReport(ctx, q.from, q.to)
		}, decodeAs(profitWorkbook)},
	"revenue-composition": {status500(fixed("Gagal memuat laporan komposisi pendapatan")),
		func(ctx context.Context, s *Service, _ string, q exportQuery) (any, error) {
			return s.RevenueCompositionReport(ctx, q.from, q.to)
		}, decodeAs(revenueWorkbook)},
	"transactions": {status500(pgMessage),
		func(ctx context.Context, s *Service, userID string, q exportQuery) (any, error) {
			return s.TransactionReport(ctx, userID, q.from, q.to, q.warehouseID)
		}, decodeAs(transactionsWorkbook)},
	"rush-hour": {rushHourStatus("Gagal memuat laporan rush hour"),
		func(ctx context.Context, s *Service, userID string, q exportQuery) (any, error) {
			return s.RushHourReport(ctx, userID, q.from, q.to, q.warehouseID)
		}, decodeAs(rushHourWorkbook)},
	"voids": {status500(pgMessage),
		func(ctx context.Context, s *Service, userID string, q exportQuery) (any, error) {
			return s.VoidReport(ctx, userID, q.from, q.to, q.warehouseID)
		}, decodeAs(voidsWorkbook)},
	"product-sales": {status500(pgMessage),
		func(ctx context.Context, s *Service, userID string, q exportQuery) (any, error) {
			return s.ProductSalesReport(ctx, userID, q.from, q.to, q.warehouseID)
		}, decodeAs(productSalesWorkbook)},
	"payment-methods": {status500(pgMessage),
		func(ctx context.Context, s *Service, userID string, q exportQuery) (any, error) {
			return s.PaymentMethodReport(ctx, userID, q.from, q.to, q.warehouseID, "day")
		}, decodeAs(paymentMethodsWorkbook)},
}

var exportDate = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// validateReportRange is lib/pos/report-period.ts': an error message or "".
func validateReportRange(from, to string) string {
	if !exportDate.MatchString(from) || !exportDate.MatchString(to) {
		return "Format tanggal harus YYYY-MM-DD"
	}
	if from > to {
		return "Tanggal dari tidak boleh melebihi tanggal sampai"
	}
	return ""
}

// reportFailure is the export's answer when the report failed: the
// status and error its own route would have sent.
func reportFailure(err error, failed func(error) (int, string)) *Fail {
	var status int
	var msg string
	var f *Fail
	var he *httpx.Error
	switch {
	case errors.As(err, &f):
		status, msg = f.Status, f.Body.Str("error")
	case errors.As(err, &he):
		status, msg = he.Status, he.Message
	default:
		status, msg = failed(err)
	}
	if msg == "" {
		msg = "Gagal memuat data laporan"
	}
	return fail(status, msg)
}

// exportReport mirrors GET /api/pos/reports/export?report=<key>&date_from
// &date_to[&warehouse_id]: the report as its route returns it (same
// filters and access), shaped into a styled workbook.
func (h *Handler) exportReport(w http.ResponseWriter, r *http.Request) error {
	user, err := h.requirePos(r, "Authentication required")
	if err != nil {
		return err
	}
	q := r.URL.Query()
	key := q.Get("report")
	exp, ok := reportExports[key]
	if !ok {
		return fail(http.StatusBadRequest, "Laporan tidak dikenal")
	}
	from, to := q.Get("date_from"), q.Get("date_to")
	if msg := validateReportRange(from, to); msg != "" {
		return fail(http.StatusBadRequest, msg)
	}
	ctx := r.Context()
	data, err := exp.load(ctx, h.svc, user.ID, exportQuery{from, to, q.Get("warehouse_id")})
	if err != nil {
		return reportFailure(err, exp.failed)
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	var head struct {
		Filters struct {
			WarehouseID *string `json:"warehouse_id"`
		} `json:"filters"`
		StallOptions []stallOption `json:"stall_options"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return err
	}
	sc, err := scope.Load(ctx, h.svc.db, user.ID)
	if err != nil {
		return err
	}
	wb, err := exp.build(raw, exportMeta{
		company:     h.svc.ports.Directory.BrandName(ctx, h.svc.db, sc.CompanyID),
		stall:       stallLabel(head.Filters.WarehouseID, head.StallOptions),
		generatedAt: h.svc.now(),
	})
	if err != nil {
		return err
	}
	body, err := wb.Bytes()
	if err != nil {
		return err
	}
	return writeXlsx(w, body, exportFileName(key, from, to), true)
}

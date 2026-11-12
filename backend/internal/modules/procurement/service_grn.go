package procurement

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	contracts "nuhabit/backend/internal/contracts/procurement"
	"nuhabit/backend/internal/modules/procurement/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/outbox"
	pscope "nuhabit/backend/internal/platform/scope"
)

// Port of lib/purchasing/grn*.ts, grn-qc.ts and the GRN half of
// vendor-credit-service.ts.

// receiveErr turns a receive-rule failure into the TS ApiError.badRequest.
func receiveErr(err error) error {
	var re *domain.ReceiveError
	if errors.As(err, &re) {
		return badRequest(re.Message)
	}
	return err
}

// serverMessage is ApiError.server(error.message || fallback): a 500 that
// carries the message.
func serverMessage(err error, fallback string) error {
	msg := pgMessage(err)
	if msg == "" {
		msg = fallback
	}
	return httpx.Status(500, msg)
}

/* ── List and detail ─────────────────────────────────────────────────── */

// GrnListParams is grnListQuerySchema.
type GrnListParams struct {
	Page, Limit                      float64
	Search, Status, DeliveryID, PoID *string
	DateFrom, DateTo                 *string
}

// ListGrns is listGrns.
func (s *Service) ListGrns(ctx context.Context, p GrnListParams, scope *pscope.Scope) ([]*Row, int, error) {
	w := newWhere().add("is_active = %s", true)
	if c := pscope.CompanyFilter(scope); c != nil {
		w.add("company_id = %s::text::uuid", *c)
	}
	if b := pscope.BranchFilter(scope); b != nil {
		w.add("branch_id = %s::text::uuid", *b)
	}
	if set(p.Status) {
		w.add("status = %s", *p.Status)
	}
	if set(p.DeliveryID) {
		w.add("delivery_id = %s::text::uuid", *p.DeliveryID)
	}
	if set(p.PoID) {
		w.add("purchase_order_id = %s::text::uuid", *p.PoID)
	}
	if set(p.DateFrom) {
		w.add("tanggal_penerimaan >= %s::text::date", *p.DateFrom)
	}
	if set(p.DateTo) {
		w.add("tanggal_penerimaan <= %s::text::date", *p.DateTo)
	}
	if set(p.Search) {
		pattern := strings.ReplaceAll("%"+*p.Search+"%", "*", "%")
		w.add("(nomor_grn ILIKE %s OR no_surat_jalan ILIKE %s)", pattern, pattern)
	}
	var total int
	if err := s.db.QueryRow(ctx, `SELECT count(*)::int FROM grn `+w.sql(), w.args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit, offset := w.next(formatJSNumber(p.Limit)), w.next(formatJSNumber((p.Page-1)*p.Limit))
	rows, err := s.rows.Query(ctx, s.db, `SELECT * FROM grn `+w.sql()+` ORDER BY created_at DESC LIMIT `+limit+`::text::bigint OFFSET `+offset+`::text::bigint`, w.args...)
	if err != nil {
		return nil, 0, err
	}
	deliveries, err := s.lookup(ctx, `SELECT id::text, COALESCE(NULLIF(no_resi, ''), nomor_resi) FROM deliveries WHERE id = ANY($1::uuid[])`, uniqueStrings(column(rows, "delivery_id")))
	if err != nil {
		return nil, 0, err
	}
	pos, err := s.lookup(ctx, `SELECT id::text, nomor_po FROM purchase_orders WHERE id = ANY($1::uuid[])`, uniqueStrings(column(rows, "purchase_order_id")))
	if err != nil {
		return nil, 0, err
	}
	suppliers, err := s.lookup(ctx, `SELECT id::text, nama_supplier FROM suppliers WHERE id = ANY($1::uuid[])`, uniqueStrings(column(rows, "supplier_id")))
	if err != nil {
		return nil, 0, err
	}
	out := make([]*Row, len(rows))
	for i, r := range rows {
		out[i] = obj(
			"id", r.Get("id"), "nomor_grn", r.Get("nomor_grn"),
			"delivery_id", r.Get("delivery_id"), "delivery_number", orID(deliveries, r, "delivery_id"),
			"po_id", r.Get("purchase_order_id"), "po_number", orID(pos, r, "purchase_order_id"),
			"supplier_id", r.Get("supplier_id"), "supplier_name", orDash(suppliers, r.Str("supplier_id")),
			"tanggal_penerimaan", r.Get("tanggal_penerimaan"), "no_surat_jalan", r.Get("no_surat_jalan"),
			"status", r.Get("status"), "total_item_diterima", r.Get("total_item_diterima"),
			"total_item_ditolak", r.Get("total_item_ditolak"), "receive_count", intOr1(r.Get("receive_count")),
			"catatan", r.Get("catatan"), "created_at", r.Get("created_at"))
	}
	return out, total, nil
}

// lookup runs an id → text query (selectByIds) into a map; NULL texts are "".
func (s *Service) lookup(ctx context.Context, sql string, ids []string) (map[string]string, error) {
	out := map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.db.Query(ctx, sql, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var v *string
		if err := rows.Scan(&id, &v); err != nil {
			return nil, err
		}
		out[id] = deref(v)
	}
	return out, rows.Err()
}

// orID is `(id && map.get(id)) || id`.
func orID(m map[string]string, r *Row, key string) any {
	if v := m[r.Str(key)]; v != "" {
		return v
	}
	return r.Get(key)
}

func orDash(m map[string]string, id string) string {
	if v := m[id]; v != "" {
		return v
	}
	return "—"
}

func intOr1(v any) any {
	if n, ok := v.(int32); ok && n != 0 {
		return n
	}
	return 1
}

// GrnDetail is getGrnDetail.
func (s *Service) GrnDetail(ctx context.Context, id string) (*Row, error) {
	grn, err := s.rows.One(ctx, s.db, `SELECT * FROM grn WHERE id = $1::text::uuid AND is_active = true`, id)
	if err != nil || grn == nil {
		return nil, notFound("GRN tidak ditemukan")
	}
	items, err := s.rows.Query(ctx, s.db, `SELECT id, grn_id, delivery_id, purchase_order_item_id, raw_material_id, pos_sku_id,
		qty_diterima, qty_ditolak, kondisi, catatan, satuan_id, batch_number, expiry_date, is_active, created_at, updated_at,
		(SELECT row_to_json(e) FROM (SELECT id, qty_ordered, qty_received, harga_satuan, subtotal, satuan_id
		   FROM purchasing.purchase_order_items WHERE id = grn_items.purchase_order_item_id) e) AS purchase_order_item
		FROM grn_items WHERE grn_id = $1 AND is_active = true`, grn.Str("id"))
	if err != nil {
		return nil, serverMessage(err, "Gagal memuat item penerimaan")
	}
	if err := s.embedGrnDetailRefs(ctx, items); err != nil {
		return nil, err
	}

	var delivery, po, supplier *Row
	if d := grn.Str("delivery_id"); d != "" {
		delivery, _ = s.rows.One(ctx, s.db, `SELECT * FROM deliveries WHERE id = $1`, d)
	}
	if p := grn.Str("purchase_order_id"); p != "" {
		po, _ = s.rows.One(ctx, s.db, `SELECT id, nomor_po, status, tanggal_po, total FROM purchase_orders WHERE id = $1`, p)
	}
	if sup := grn.Str("supplier_id"); sup != "" {
		supplier, _ = s.rows.One(ctx, s.db, `SELECT id, nama_supplier, kode, email, telepon FROM suppliers WHERE id = $1`, sup)
	}
	if delivery != nil {
		grn.Set("delivery_number", jsOr(delivery, "no_resi", "nomor_resi", "no_surat_jalan"))
	}
	grn.Set("delivery", rowOrNull(delivery))
	grn.Set("purchase_order", rowOrNull(po))
	if po != nil {
		grn.Set("po_number", po.Get("nomor_po"))
		grn.Set("po_status", po.Get("status"))
	}
	if supplier != nil {
		grn.Set("supplier_name", supplier.Get("nama_supplier"))
	}
	grn.Set("supplier", rowOrNull(supplier))
	grn.Set("items", items)
	return grn, nil
}

// jsOr is `r.a || r.b || r.c`: the first truthy value, else the last one.
func jsOr(r *Row, keys ...string) any {
	for _, k := range keys {
		if v := r.Get(k); v != nil && v != "" {
			return v
		}
	}
	return r.Get(keys[len(keys)-1])
}

func rowOrNull(r *Row) any {
	if r == nil {
		return nil
	}
	return r
}

// embedGrnDetailRefs adds raw_material (with satuan_besar), satuan, pos_sku
// and the nested purchase_order_item.satuan, in GRN_ITEM_DETAIL_SELECT order.
func (s *Service) embedGrnDetailRefs(ctx context.Context, items []*Row) error {
	materials, err := s.ports.Catalog.Refs(ctx, s.db, EntityRawMaterial, "id, nama, kode, satuan_besar_id", uniqueStrings(column(items, "raw_material_id")))
	if err != nil {
		return err
	}
	poItems := make([]*Row, len(items))
	unitIDs := column(items, "satuan_id")
	parsed := map[string]*Row{}
	for _, raw := range materials {
		rm, err := decodeRow(raw)
		if err != nil {
			return err
		}
		parsed[rm.Str("id")] = rm
		unitIDs = append(unitIDs, rm.Str("satuan_besar_id"))
	}
	for i, item := range items {
		if raw, ok := item.Get("purchase_order_item").(json.RawMessage); ok {
			if poItems[i], err = decodeRow(raw); err != nil {
				return err
			}
			unitIDs = append(unitIDs, poItems[i].Str("satuan_id"))
		}
	}
	units, err := s.ports.Catalog.Refs(ctx, s.db, EntityUnit, "id, nama, kode", uniqueStrings(unitIDs))
	if err != nil {
		return err
	}
	skus, err := s.ports.Catalog.Refs(ctx, s.db, EntityPosSku, "id, sku, name", uniqueStrings(column(items, "pos_sku_id")))
	if err != nil {
		return err
	}
	for i, item := range items {
		var rm any
		if m, ok := parsed[item.Str("raw_material_id")]; ok {
			rm = obj("id", m.Get("id"), "nama", m.Get("nama"), "kode", m.Get("kode"), "satuan_besar", refOrNull(units, m.Str("satuan_besar_id")))
		}
		var poItem any
		if p := poItems[i]; p != nil {
			poItem = obj("id", p.Get("id"), "qty_ordered", p.Get("qty_ordered"), "qty_received", p.Get("qty_received"),
				"harga_satuan", p.Get("harga_satuan"), "subtotal", p.Get("subtotal"), "satuan", refOrNull(units, p.Str("satuan_id")))
		}
		item.Set("raw_material", rm)
		item.Set("satuan", refOrNull(units, item.Str("satuan_id")))
		item.Set("pos_sku", refOrNull(skus, item.Str("pos_sku_id")))
		item.Set("purchase_order_item", poItem)
	}
	return nil
}

/* ── Receiving rules on the database ─────────────────────────────────── */

// recalculatePoReceivedQty rebuilds each active PO line's qty_received from
// the active GRN lines (QC-posted, door quantity, or 0 while pending).
func (s *Service) recalculatePoReceivedQty(ctx context.Context, q database.Querier, poID string) error {
	rows, err := s.rows.Query(ctx, q, `SELECT poi.id AS po_item_id,
		  COALESCE((SELECT json_agg(json_build_object('qty_diterima', gi.qty_diterima, 'qty_qc_posted', gi.qty_qc_posted,
		    'status', g.status, 'posted', EXISTS (SELECT 1 FROM grn_qc_inspections qc WHERE qc.grn_id = g.id AND qc.inventory_posted)))
		   FROM grn_items gi JOIN grn g ON g.id = gi.grn_id
		   WHERE gi.purchase_order_item_id = poi.id AND gi.is_active = true AND g.is_active = true AND g.purchase_order_id = $1), '[]'::json) AS lines
		FROM purchase_order_items poi WHERE poi.purchase_order_id = $1 AND poi.is_active = true`, poID)
	if err != nil {
		return err
	}
	for _, r := range rows {
		var lines []struct {
			QtyDiterima float64 `json:"qty_diterima"`
			QtyQcPosted float64 `json:"qty_qc_posted"`
			Status      string  `json:"status"`
			Posted      bool    `json:"posted"`
		}
		raw, _ := r.Get("lines").(json.RawMessage)
		if err := json.Unmarshal(raw, &lines); err != nil {
			return err
		}
		received := 0.0
		for _, l := range lines {
			received += domain.GrnItemReceivedQty(l.QtyDiterima, l.QtyQcPosted, l.Status, l.Posted)
		}
		if _, err := q.Exec(ctx, `UPDATE purchase_order_items SET qty_received = $2 WHERE id = $1`, r.Str("po_item_id"), received); err != nil {
			return err
		}
	}
	return nil
}

// poQtyTotals sums ordered and received over the PO's active lines.
func poQtyTotals(ctx context.Context, q database.Querier, poID string) (ordered, received float64, lines int, err error) {
	err = q.QueryRow(ctx, `SELECT COALESCE(SUM(qty_ordered), 0)::float8, COALESCE(SUM(COALESCE(qty_received, 0)), 0)::float8, count(*)::int
		FROM purchase_order_items WHERE purchase_order_id = $1 AND is_active = true`, poID).Scan(&ordered, &received, &lines)
	return
}

// updatePOStatusAfterGrn recalculates received quantities and moves the PO
// to sent / partially_received / received (closed and cancelled stay).
func (s *Service) updatePOStatusAfterGrn(ctx context.Context, q database.Querier, poID string) error {
	if err := s.recalculatePoReceivedQty(ctx, q, poID); err != nil {
		return err
	}
	var status *string
	if err := q.QueryRow(ctx, `SELECT status FROM purchase_orders WHERE id = $1`, poID).Scan(&status); err != nil && !database.IsNoRows(err) {
		return err
	}
	if current := strings.ToLower(deref(status)); current == domain.PoClosed || current == domain.PoCancelled {
		return nil
	}
	ordered, received, lines, err := poQtyTotals(ctx, q, poID)
	if err != nil || lines == 0 {
		return err
	}
	_, err = q.Exec(ctx, `UPDATE purchase_orders SET status = $2, updated_at = $3 WHERE id = $1`,
		poID, domain.PoStatusFromReceipts(ordered, received), s.now())
	return err
}

// updateDeliveryStatusAfterGrn closes the delivery once its GRN is final.
func (s *Service) updateDeliveryStatusAfterGrn(ctx context.Context, q database.Querier, deliveryID, grnStatus string) error {
	status := domain.DeliveryStatusAfterGrn(grnStatus)
	if status == "" {
		return nil
	}
	_, err := q.Exec(ctx, `UPDATE deliveries SET status = $2, updated_at = $3 WHERE id = $1`, deliveryID, status, s.now())
	return err
}

// nonFatal runs fn in a savepoint; a failure is logged and rolled back,
// as the TS try/catch blocks around side effects did.
func (s *Service) nonFatal(ctx context.Context, q database.TxBeginner, label string, fn func(pgx.Tx) error) {
	if err := database.WithTx(ctx, q, fn); err != nil {
		s.log.ErrorContext(ctx, label+" (non-fatal)", "error", err)
	}
}

/* ── Vendor credits from rejects ─────────────────────────────────────── */

type creditLine struct {
	GrnItemID, RawMaterialID string
	Qty, UnitPrice           float64
	Notes                    *string
}

// syncRejectCredits is syncReceiveRejectCredits / syncQcRejectCredits:
// keep one editable credit per GRN and source in step with the rejects.
func (s *Service) syncRejectCredits(ctx context.Context, q database.Querier, grnID, sourceType string, userID string) error {
	grn, err := s.rows.One(ctx, q, `SELECT id, supplier_id, purchase_order_id FROM grn WHERE id = $1 AND is_active = true`, grnID)
	if err != nil || grn == nil {
		return err
	}
	var lines []creditLine
	if sourceType == "receive_reject" {
		rows, err := s.rows.Query(ctx, q, `SELECT gi.id, gi.raw_material_id, gi.qty_ditolak::float8 AS qty, gi.catatan,
			COALESCE(poi.harga_satuan, 0)::float8 AS price
			FROM grn_items gi LEFT JOIN purchase_order_items poi ON poi.id = gi.purchase_order_item_id
			WHERE gi.grn_id = $1 AND gi.is_active = true AND gi.raw_material_id IS NOT NULL`, grnID)
		if err != nil {
			return err
		}
		for _, r := range rows {
			lines = append(lines, creditLine{GrnItemID: r.Str("id"), RawMaterialID: r.Str("raw_material_id"), Qty: r.Num("qty"), UnitPrice: r.Num("price"), Notes: r.StrPtr("catatan")})
		}
	} else {
		var qcID string
		var posted bool
		err := q.QueryRow(ctx, `SELECT id::text, inventory_posted FROM grn_qc_inspections WHERE grn_id = $1 LIMIT 1`, grnID).Scan(&qcID, &posted)
		if err != nil && !database.IsNoRows(err) {
			return err
		}
		if !posted {
			return s.cancelEditableCredit(ctx, q, grnID, sourceType)
		}
		rows, err := s.rows.Query(ctx, q, `SELECT qi.grn_item_id, qi.raw_material_id, qi.qty_rejected::float8 AS qty, qi.catatan,
			COALESCE(poi.harga_satuan, 0)::float8 AS price
			FROM grn_qc_inspection_items qi
			LEFT JOIN grn_items gi ON gi.id = qi.grn_item_id
			LEFT JOIN purchase_order_items poi ON poi.id = gi.purchase_order_item_id
			WHERE qi.qc_inspection_id = $1 AND qi.raw_material_id IS NOT NULL`, qcID)
		if err != nil {
			return err
		}
		for _, r := range rows {
			lines = append(lines, creditLine{GrnItemID: r.Str("grn_item_id"), RawMaterialID: r.Str("raw_material_id"), Qty: r.Num("qty"), UnitPrice: r.Num("price"), Notes: r.StrPtr("catatan")})
		}
	}
	var active []creditLine
	total := 0.0
	for _, l := range lines {
		if l.Qty > 0.0001 {
			active = append(active, l)
			total += domain.RoundMoney(l.Qty * l.UnitPrice)
		}
	}
	if len(active) == 0 {
		return s.cancelEditableCredit(ctx, q, grnID, sourceType)
	}
	total = domain.RoundMoney(total)
	reason := "Auto-generated from goods receipt reject quantities"
	if sourceType == "qc_reject" {
		reason = "Auto-generated from QC reject quantities"
	}
	creditID, err := s.editableCredit(ctx, q, grnID, sourceType)
	if err != nil {
		return err
	}
	if creditID == "" {
		var creator *string
		if userID != "" {
			creator = &userID
		}
		if err := q.QueryRow(ctx, `INSERT INTO vendor_credits
			(grn_id, purchase_order_id, supplier_id, source_type, status, total_amount, reason_notes, notes, created_by, updated_at)
			VALUES ($1, $2, $3, $4, 'draft', $5, $6, NULL, $7, $8) RETURNING id::text`,
			grnID, grn.StrPtr("purchase_order_id"), grn.StrPtr("supplier_id"), sourceType, total, reason, creator, s.now()).Scan(&creditID); err != nil {
			return err
		}
	} else {
		if _, err := q.Exec(ctx, `UPDATE vendor_credits SET total_amount = $2, reason_notes = $3, updated_at = $4 WHERE id = $1`,
			creditID, total, reason, s.now()); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `DELETE FROM vendor_credit_items WHERE vendor_credit_id = $1`, creditID); err != nil {
			return err
		}
	}
	for _, l := range active {
		if _, err := q.Exec(ctx, `INSERT INTO vendor_credit_items (vendor_credit_id, grn_item_id, raw_material_id, qty, unit_price, line_amount, notes)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`, creditID, l.GrnItemID, l.RawMaterialID, l.Qty, l.UnitPrice, domain.RoundMoney(l.Qty*l.UnitPrice), l.Notes); err != nil {
			return err
		}
	}
	return nil
}

// editableCredit is findEditableCredit: the newest draft/pending credit.
func (s *Service) editableCredit(ctx context.Context, q database.Querier, grnID, sourceType string) (string, error) {
	var id string
	err := q.QueryRow(ctx, `SELECT id::text FROM vendor_credits WHERE grn_id = $1 AND source_type = $2
		AND status IN ('draft', 'pending_approval') ORDER BY created_at DESC LIMIT 1`, grnID, sourceType).Scan(&id)
	if database.IsNoRows(err) {
		return "", nil
	}
	return id, err
}

func (s *Service) cancelEditableCredit(ctx context.Context, q database.Querier, grnID, sourceType string) error {
	id, err := s.editableCredit(ctx, q, grnID, sourceType)
	if err != nil || id == "" {
		return err
	}
	if _, err := q.Exec(ctx, `DELETE FROM vendor_credit_items WHERE vendor_credit_id = $1`, id); err != nil {
		return err
	}
	_, err = q.Exec(ctx, `UPDATE vendor_credits SET status = 'cancelled', total_amount = 0, updated_at = $2 WHERE id = $1`, id, s.now())
	return err
}

/* ── QC ──────────────────────────────────────────────────────────────── */

// QcItem is one QC inspection line (QcInspectionItemInput).
type QcItem struct {
	GrnItemID                              string
	RawMaterialID, ProductID               *string
	QtyInspected, QtyAccepted, QtyRejected float64
	Catatan                                *string
}

// QcInput is SubmitGrnQcInput.
type QcInput struct {
	GrnID             string
	Status            string
	ParameterInspeksi json.RawMessage
	HasilInspeksi     json.RawMessage
	Catatan           *string
	Rekomendasi       *string
	Items             []QcItem
	UserID            string
}

// QcResult is what submitGrnQcInspection returns.
type QcResult struct {
	InspectionID               string
	GrnStatus                  string
	TotalAccepted, TotalReject float64
}

// SubmitQc is submitGrnQcInspection on its own transaction.
func (s *Service) SubmitQc(ctx context.Context, in QcInput) (*QcResult, error) {
	var out *QcResult
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		var err error
		out, err = s.submitQc(ctx, tx, in)
		return err
	})
	return out, err
}

type qcGrnItem struct {
	ID, RawMaterialID, ProductID, SatuanID, WarehouseID, PosSkuID string
	QtyDiterima, QtyQcPosted, Harga                               float64
	BatchNumber, ExpiryDate                                       *string
}

func (s *Service) submitQc(ctx context.Context, tx pgx.Tx, in QcInput) (*QcResult, error) {
	grn, err := s.rows.One(ctx, tx, `SELECT id, nomor_grn, status, purchase_order_id, delivery_id FROM grn WHERE id = $1::text::uuid AND is_active = true`, in.GrnID)
	if err != nil || grn == nil {
		return nil, badRequest("GRN not found")
	}
	if grn.Str("status") != domain.GrnPending {
		return nil, badRequest("GRN is not awaiting quality control")
	}
	var existingQcID *string
	var posted bool
	if err := tx.QueryRow(ctx, `SELECT id::text, inventory_posted FROM grn_qc_inspections WHERE grn_id = $1 LIMIT 1`, grn.Str("id")).Scan(&existingQcID, &posted); err != nil && !database.IsNoRows(err) {
		return nil, err
	}
	if posted {
		return nil, badRequest("Quality control has already been completed for this goods receipt")
	}
	rows, err := s.rows.Query(ctx, tx, `SELECT gi.id, gi.raw_material_id, gi.product_id, gi.qty_diterima::float8 AS qty_diterima, gi.satuan_id,
		gi.warehouse_id, gi.qty_qc_posted::float8 AS qty_qc_posted, gi.pos_sku_id, gi.batch_number, gi.expiry_date::text AS expiry_date,
		poi.harga_satuan::float8 AS harga
		FROM grn_items gi LEFT JOIN purchase_order_items poi ON poi.id = gi.purchase_order_item_id
		WHERE gi.grn_id = $1 AND gi.is_active = true`, grn.Str("id"))
	if err != nil {
		return nil, err
	}
	byID := map[string]qcGrnItem{}
	for _, r := range rows {
		byID[r.Str("id")] = qcGrnItem{
			ID: r.Str("id"), RawMaterialID: r.Str("raw_material_id"), ProductID: r.Str("product_id"), SatuanID: r.Str("satuan_id"),
			WarehouseID: r.Str("warehouse_id"), PosSkuID: r.Str("pos_sku_id"), QtyDiterima: r.Num("qty_diterima"),
			QtyQcPosted: r.Num("qty_qc_posted"), Harga: r.Num("harga"), BatchNumber: r.StrPtr("batch_number"), ExpiryDate: r.StrPtr("expiry_date"),
		}
	}
	itemRef := func(item QcItem) (rm, product string) {
		g := byID[item.GrnItemID]
		rm, product = deref(item.RawMaterialID), deref(item.ProductID)
		if item.RawMaterialID == nil {
			rm = g.RawMaterialID
		}
		if item.ProductID == nil {
			product = g.ProductID
		}
		return
	}
	var productIDs []string
	for _, item := range in.Items {
		if _, product := itemRef(item); product != "" {
			productIDs = append(productIDs, product)
		}
	}
	variants, err := s.variantProducts(ctx, tx, uniqueStrings(productIDs))
	if err != nil {
		return nil, err
	}
	accepted := make([]float64, len(in.Items))
	rejected := make([]float64, len(in.Items))
	for i, item := range in.Items {
		g, ok := byID[item.GrnItemID]
		if !ok {
			return nil, badRequest("GRN item " + item.GrnItemID + " not found")
		}
		rm, product := itemRef(item)
		if rm == "" && product == "" {
			return nil, badRequest("GRN item " + item.GrnItemID + " has no raw material or product")
		}
		switch {
		case item.QtyInspected <= 0:
			return nil, badRequest("Inspected quantity must be greater than zero")
		case abs(item.QtyAccepted+item.QtyRejected-item.QtyInspected) > 0.0001:
			return nil, badRequest("Accepted and rejected quantities must equal inspected quantity")
		case item.QtyInspected > g.QtyDiterima+0.0001:
			return nil, badRequest("Inspected quantity cannot exceed received good quantity")
		case item.QtyAccepted > g.QtyDiterima+0.0001:
			return nil, badRequest("Accepted quantity cannot exceed received good quantity")
		case product != "" && variants[product] && g.PosSkuID == "":
			return nil, badRequest("GRN produk ber-varian wajib menyebut SKU (pos_sku_id)")
		}
		accepted[i], rejected[i] = item.QtyAccepted, item.QtyRejected
	}
	status := in.Status
	if status == "" {
		status = domain.QcOverallStatus(accepted, rejected)
	}
	var totalAccepted, totalRejected float64
	for i := range accepted {
		totalAccepted += accepted[i]
		totalRejected += rejected[i]
	}
	if existingQcID != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM grn_qc_inspection_items WHERE qc_inspection_id = $1`, *existingQcID); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM grn_qc_inspections WHERE id = $1`, *existingQcID); err != nil {
			return nil, err
		}
	}
	var inspectionID string
	if err := tx.QueryRow(ctx, `INSERT INTO grn_qc_inspections
		(grn_id, status, parameter_inspeksi, hasil_inspeksi, catatan, rekomendasi, inspector_id, inspected_at, inventory_posted, created_by, updated_by)
		VALUES ($1, $2, $3::jsonb, $4::jsonb, $5, $6, $7, $8, false, $7, $7) RETURNING id::text`,
		grn.Str("id"), status, jsonOrNil(in.ParameterInspeksi), jsonOrNil(in.HasilInspeksi), nonEmpty(in.Catatan), nonEmpty(in.Rekomendasi),
		in.UserID, s.now()).Scan(&inspectionID); err != nil {
		return nil, plainError(err)
	}
	for _, item := range in.Items {
		rm, product := itemRef(item)
		if _, err := tx.Exec(ctx, `INSERT INTO grn_qc_inspection_items
			(qc_inspection_id, grn_item_id, raw_material_id, product_id, qty_inspected, qty_accepted, qty_rejected, item_status, catatan)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			inspectionID, item.GrnItemID, nilIfEmpty(rm), nilIfEmpty(product), item.QtyInspected, item.QtyAccepted, item.QtyRejected,
			domain.QcItemStatus(item.QtyAccepted, item.QtyRejected), nonEmpty(item.Catatan)); err != nil {
			return nil, plainError(err)
		}
	}

	var materialIDs []string
	for _, item := range in.Items {
		if rm, _ := itemRef(item); rm != "" {
			materialIDs = append(materialIDs, rm)
		}
	}
	units, err := s.ports.Catalog.MaterialUnits(ctx, tx, uniqueStrings(materialIDs))
	if err != nil {
		units = map[string]MaterialUnits{}
	}
	stock := contracts.GrnStockReceived{GrnID: grn.Str("id"), GrnNumber: grn.Str("nomor_grn"), UserID: in.UserID}
	for _, item := range in.Items {
		g := byID[item.GrnItemID]
		toPost := max(0, item.QtyAccepted-g.QtyQcPosted)
		rm, product := itemRef(item)
		if _, err := tx.Exec(ctx, `UPDATE grn_items SET qc_status = $2, qty_qc_posted = $3, updated_at = $4 WHERE id = $1`,
			item.GrnItemID, domain.QcItemStatus(item.QtyAccepted, item.QtyRejected), item.QtyAccepted, s.now()); err != nil {
			return nil, err
		}
		if toPost <= 0 {
			continue
		}
		switch {
		case rm != "":
			factor := 1.0
			if m, ok := units[rm]; ok {
				factor = domain.PackFactor(&m.Units, m.Packs, g.SatuanID)
			}
			stock.Lines = append(stock.Lines, contracts.GrnStockLine{
				RawMaterialID: &rm, Qty: domain.PackToBase(toPost, factor), UnitCost: domain.PackPriceToBase(g.Harga, factor),
				WarehouseID: nilIfEmpty(g.WarehouseID), BatchNumber: g.BatchNumber, ExpiryDate: g.ExpiryDate,
			})
		case product != "" && g.PosSkuID != "":
			n, err := s.ports.Merchandise.ReceiveSkuStock(ctx, tx, g.PosSkuID, toPost)
			if err != nil {
				return nil, plainError(err)
			}
			if n == 0 {
				return nil, badRequest("SKU " + g.PosSkuID + " tidak aktif atau tidak ditemukan — stok tidak dapat diposting")
			}
		case product != "":
			s.nonFatal(ctx, tx, "[GRN QC] Merch stock posting error", func(sp pgx.Tx) error {
				n, err := s.ports.Merchandise.ReceiveProductStock(ctx, sp, product, toPost)
				if err == nil && n == 0 {
					s.log.WarnContext(ctx, "[GRN QC] No linked POS merchandise product — stock not posted", "product_id", product)
				}
				return err
			})
		}
	}
	if len(stock.Lines) > 0 {
		if err := outbox.Publish(ctx, tx, contracts.TopicGrnStockReceived, grn.Str("id"), stock); err != nil {
			return nil, err
		}
	}

	// Mark the inspection posted first so the PO quantities below count
	// this QC's accepted quantity.
	if _, err := tx.Exec(ctx, `UPDATE grn_qc_inspections SET inventory_posted = true, updated_by = $2, updated_at = $3 WHERE id = $1`, inspectionID, in.UserID, s.now()); err != nil {
		return nil, err
	}
	grnStatus := domain.GrnRejected
	poID := grn.Str("purchase_order_id")
	if totalAccepted > 0 {
		if err := s.recalculatePoReceivedQty(ctx, tx, poID); err != nil {
			return nil, err
		}
		ordered, received, lines, err := poQtyTotals(ctx, tx, poID)
		if err != nil {
			return nil, err
		}
		grnStatus = domain.GrnStatusAfterQc(totalAccepted, ordered, received, lines > 0)
	}
	if _, err := tx.Exec(ctx, `UPDATE grn SET status = $2, updated_by = $3, updated_at = $4 WHERE id = $1`, grn.Str("id"), grnStatus, in.UserID, s.now()); err != nil {
		return nil, err
	}
	if d := grn.Str("delivery_id"); d != "" {
		if err := s.updateDeliveryStatusAfterGrn(ctx, tx, d, grnStatus); err != nil {
			return nil, err
		}
	}
	if poID != "" {
		if err := s.updatePOStatusAfterGrn(ctx, tx, poID); err != nil {
			return nil, err
		}
	}
	s.nonFatal(ctx, tx, "[GRN QC] Vendor credit sync error", func(sp pgx.Tx) error {
		return s.syncRejectCredits(ctx, sp, grn.Str("id"), "qc_reject", in.UserID)
	})
	if err := publishGrnPosted(ctx, tx, grn.Str("id"), in.UserID); err != nil {
		return nil, err
	}
	return &QcResult{InspectionID: inspectionID, GrnStatus: grnStatus, TotalAccepted: totalAccepted, TotalReject: totalRejected}, nil
}

// publishGrnPosted hands the GRN journals and AP invoice to accounting
// (postGrnAccountingJournals).
func publishGrnPosted(ctx context.Context, tx pgx.Tx, grnID, userID string) error {
	return outbox.Publish(ctx, tx, contracts.TopicGrnPosted, grnID, contracts.GrnPosted{GrnID: grnID, UserID: userID})
}

// variantProducts is resolveVariantProductIds: products with an active SKU.
func (s *Service) variantProducts(ctx context.Context, q database.Querier, productIDs []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(productIDs) == 0 {
		return out, nil
	}
	skus, err := s.ports.Catalog.MerchandiseSkus(ctx, q, productIDs)
	if err != nil {
		return nil, err
	}
	for _, sku := range skus {
		if sku.Bool("is_active") {
			out[sku.Str("source_product_id")] = true
		}
	}
	return out, nil
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func jsonOrNil(raw json.RawMessage) *string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	s := string(raw)
	return &s
}

/* ── Create ──────────────────────────────────────────────────────────── */

// GrnCreated is createGrn's result.
type GrnCreated struct {
	Grn     *Row
	Status  string
	Message string
	Audit   audit.Entry
}

// warehouseScopeErrors are WAREHOUSE_SCOPE_ERRORS of grn-create.
var warehouseScopeErrors = map[string]string{
	"not_found":       "Gudang tidak ditemukan",
	"inactive":        "Gudang tidak aktif",
	"branch_mismatch": "Gudang tidak sesuai cabang yang diizinkan untuk penerimaan ini",
	"no_business":     "Gudang tidak terhubung ke toko dan perusahaan yang aktif",
}

// receivingScope is resolveReceivingScope: the warehouse's company and
// branch, checked against the user's scope. A warehouse without an active
// branch and company is refused (EPIC-105 R3); the old hard-coded
// MIZU / MIZU-BANDUNG fallback is gone.
func (s *Service) receivingScope(ctx context.Context, q database.Querier, warehouseID string, scope *pscope.Scope) (*BusinessIDs, error) {
	business, err := s.ports.Locations.WarehouseScope(ctx, q, warehouseID)
	if err != nil {
		return nil, err
	}
	contextBranch := (*string)(nil)
	if business != nil {
		contextBranch = &business.BranchID
	}
	// not_found / inactive of the warehouse itself keep their own message.
	if code, err := s.validateWarehouse(ctx, q, warehouseID, scope, contextBranch); err != nil {
		return nil, err
	} else if code != "" {
		return nil, badRequest(warehouseScopeErrors[code])
	}
	if business == nil {
		return nil, badRequest(warehouseScopeErrors["no_business"])
	}
	return business, nil
}

// validateWarehouse is validateWarehouseForReceivingScope; it returns the
// error code ("not_found", "inactive", "branch_mismatch") or "".
func (s *Service) validateWarehouse(ctx context.Context, q database.Querier, warehouseID string, scope *pscope.Scope, contextBranch *string) (string, error) {
	expectedBranch := pscope.WarehouseBranchFilter(scope, contextBranch)
	expectedCompany := pscope.WarehouseCompanyFilter(scope)
	w, err := s.ports.Locations.Warehouse(ctx, q, warehouseID)
	if err != nil {
		return "", err
	}
	switch {
	case w == nil:
		return "not_found", nil
	case !w.IsActive:
		return "inactive", nil
	case expectedBranch != nil && *expectedBranch != "" && w.BranchID != *expectedBranch:
		return "branch_mismatch", nil
	case expectedCompany != nil && (w.CompanyID == nil || *w.CompanyID != *expectedCompany):
		return "branch_mismatch", nil
	}
	return "", nil
}

// CreateGrn is createGrn: record a receipt against a delivery (or a general
// PO without one) and, for raw materials and products, finish QC inline.
func (s *Service) CreateGrn(ctx context.Context, in *createGrnInput, user *auth.User) (*GrnCreated, error) {
	var out *GrnCreated
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		var err error
		out, err = s.createGrn(ctx, tx, in, user)
		return err
	})
	return out, err
}

func (s *Service) createGrn(ctx context.Context, tx pgx.Tx, in *createGrnInput, user *auth.User) (*GrnCreated, error) {
	requested := domain.ModuleType(deref(in.ModuleType))
	deliveryID := deref(in.DeliveryID)
	autoDelivery := deliveryID == ""
	if autoDelivery {
		if requested != "general" || deref(in.PoID) == "" {
			return nil, badRequest("Delivery wajib dipilih untuk penerimaan ini")
		}
		id, err := s.createAutoDelivery(ctx, tx, *in.PoID, user.ID)
		if err != nil {
			return nil, err
		}
		deliveryID = id
	}

	delivery, errs, err := s.validateDeliveryCanReceive(ctx, tx, deliveryID)
	if err != nil {
		return nil, err
	}
	if delivery == nil || delivery.Str("purchase_order_id") == "" {
		msg := strings.Join(errs, "; ")
		if msg == "" {
			msg = "Delivery tidak valid untuk penerimaan barang"
		}
		return nil, badRequest(msg)
	}
	poID := delivery.Str("purchase_order_id")
	moduleType := requested
	if !autoDelivery {
		po, err := s.rows.One(ctx, tx, `SELECT module_type FROM purchase_orders WHERE id = $1`, poID)
		if err != nil || po == nil {
			return nil, badRequest("Purchase order tidak ditemukan untuk delivery ini")
		}
		moduleType = domain.ModuleType(po.Str("module_type"))
		if in.ModuleType != nil && *in.ModuleType != moduleType {
			return nil, badRequest("module_type tidak sesuai purchase order (" + moduleType + ")")
		}
	}
	lines := in.Items
	if moduleType != "general" {
		lines = make([]grnLineInput, len(in.Items))
		for i, l := range in.Items {
			a, r := domain.NormalizeQc(l.QtyDiterima, l.QtyAccepted, l.QtyRejected)
			l.QtyAccepted, l.QtyRejected = &a, &r
			lines[i] = l
		}
	}
	usesVendor := moduleType != "raw_material"

	scope, err := s.Scope(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	business, err := s.receivingScope(ctx, tx, in.WarehouseID, scope)
	if err != nil {
		return nil, err
	}
	if business != nil {
		s.nonFatal(ctx, tx, "Failed to backfill PO business scope", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `UPDATE purchase_orders SET company_id = $2, branch_id = $3 WHERE id = $1`, poID, business.CompanyID, business.BranchID)
			return err
		})
		s.nonFatal(ctx, tx, "Failed to backfill delivery business scope", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `UPDATE deliveries SET company_id = $2, branch_id = $3 WHERE id = $1`, deliveryID, business.CompanyID, business.BranchID)
			return err
		})
	}

	poItems, err := s.poItemsForReceive(ctx, tx, poID, false)
	if err != nil {
		return nil, badRequest(pgMessage(err))
	}
	if len(errs) > 0 {
		statusOnly := false
		for _, e := range errs {
			if strings.Contains(e, "status") {
				statusOnly = true
			}
		}
		if !statusOnly {
			return nil, badRequest(strings.Join(errs, "; "))
		}
	}
	receive := make([]domain.ReceiveLine, len(lines))
	for i, l := range lines {
		receive[i] = l.receive()
	}
	skus, err := domain.ValidateReceiveLines(receive, poItems)
	if err != nil {
		return nil, receiveErr(err)
	}

	number, err := s.nextNumber(ctx, tx, "grn", "nomor_grn", domain.DailyPrefix("GRN", s.now().In(s.loc)))
	if err != nil {
		return nil, err
	}
	diterima, ditolak := domain.GrnTotals(receive)
	var previous int
	if err := tx.QueryRow(ctx, `SELECT count(*)::int FROM grn WHERE delivery_id = $1 AND is_active = true`, deliveryID).Scan(&previous); err != nil {
		return nil, err
	}
	initial := domain.InitialGrnStatus(moduleType, diterima, ditolak)
	receivedOn := s.now().UTC().Format("2006-01-02")
	if deref(in.TanggalPenerimaan) != "" {
		receivedOn = *in.TanggalPenerimaan
	}
	var supplierID, vendorID *string
	if usesVendor {
		vendorID = delivery.StrPtr("vendor_id")
	} else {
		supplierID = delivery.StrPtr("supplier_id")
	}
	var companyID, branchID *string
	if business != nil {
		companyID, branchID = &business.CompanyID, &business.BranchID
	}
	grn, err := s.rows.One(ctx, tx, `INSERT INTO grn
		(nomor_grn, delivery_id, purchase_order_id, supplier_id, vendor_id, company_id, branch_id, tanggal_penerimaan,
		 no_surat_jalan, catatan, status, total_item_diterima, total_item_ditolak, receive_count, penerima_id, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::text::date, $9, $10, $11, $12::numeric, $13::numeric, $14, $15, $15) RETURNING *`,
		number, deliveryID, poID, supplierID, vendorID, companyID, branchID, receivedOn,
		delivery.StrPtr("no_surat_jalan"), nonEmpty(in.Catatan), initial, diterima, ditolak, previous+1, user.ID)
	if err != nil {
		return nil, serverMessage(err, "Gagal menyimpan dokumen penerimaan barang")
	}
	grnID := grn.Str("id")

	var materialIDs []string
	for _, l := range lines {
		if deref(l.RawMaterialID) != "" {
			materialIDs = append(materialIDs, *l.RawMaterialID)
		}
	}
	shelfLife, err := s.ports.Catalog.ShelfLifeDays(ctx, tx, uniqueStrings(materialIDs))
	if err != nil {
		shelfLife = map[string]*float64{}
	}
	type created struct {
		ID, PoItemID, RawMaterialID, ProductID string
		QtyDiterima                            float64
	}
	createdItems := make([]created, 0, len(lines))
	auditItems := make([]*Row, 0, len(lines))
	for i, l := range lines {
		var batch, expiry *string
		hasBatch := deref(l.RawMaterialID) != ""
		if hasBatch {
			if b := strings.TrimSpace(deref(l.BatchNumber)); b != "" {
				batch = &b
			}
			expiry = nonEmpty(l.ExpiryDate)
			if expiry == nil {
				expiry = domain.DefaultExpiryDate(receivedOn, shelfLife[*l.RawMaterialID])
			}
		}
		var c created
		if err := tx.QueryRow(ctx, `INSERT INTO grn_items
			(grn_id, delivery_id, purchase_order_item_id, raw_material_id, product_id, supply_item_id, pos_sku_id, qty_diterima, qty_ditolak,
			 satuan_id, kondisi, catatan, warehouse_id, qc_status, batch_number, expiry_date)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, 'pending', $14, $15::text::date)
			RETURNING id::text, COALESCE(purchase_order_item_id::text, ''), COALESCE(raw_material_id::text, ''), COALESCE(product_id::text, ''), qty_diterima::float8`,
			grnID, deliveryID, l.PurchaseOrderItemID, nonEmpty(l.RawMaterialID), nonEmpty(l.ProductID), nonEmpty(l.SupplyItemID), skus[i],
			l.QtyDiterima, l.QtyDitolak, l.SatuanID, l.Kondisi, nonEmpty(l.Catatan), in.WarehouseID, batch, expiry).
			Scan(&c.ID, &c.PoItemID, &c.RawMaterialID, &c.ProductID, &c.QtyDiterima); err != nil {
			return nil, serverMessage(err, "Gagal menyimpan item penerimaan barang")
		}
		createdItems = append(createdItems, c)
		auditItem := obj("raw_material_id", nonEmpty(l.RawMaterialID), "product_id", nonEmpty(l.ProductID), "supply_item_id", nonEmpty(l.SupplyItemID),
			"qty_diterima", l.QtyDiterima, "qty_ditolak", l.QtyDitolak, "batch_number", nil, "expiry_date", nil)
		if hasBatch {
			auditItem.Set("batch_number", batch).Set("expiry_date", expiry)
		}
		auditItems = append(auditItems, auditItem)
	}

	if moduleType == "general" && initial != domain.GrnRejected {
		if err := s.publishSupplyStock(ctx, tx, grn, lines, poItems, in.WarehouseID, companyID, branchID, user.ID); err != nil {
			return nil, err
		}
	}

	status := initial
	if moduleType != "general" && initial == domain.GrnPending {
		var qcItems []QcItem
		remaining := append([]grnLineInput{}, lines...)
		for _, c := range createdItems {
			if c.QtyDiterima <= 0 {
				continue
			}
			idx := -1
			for j, req := range remaining {
				if (c.PoItemID != "" && deref(req.PurchaseOrderItemID) == c.PoItemID) ||
					(c.RawMaterialID != "" && deref(req.RawMaterialID) == c.RawMaterialID) ||
					(c.ProductID != "" && deref(req.ProductID) == c.ProductID) {
					idx = j
					break
				}
			}
			var req *grnLineInput
			if idx >= 0 {
				r := remaining[idx]
				req = &r
				remaining = append(remaining[:idx], remaining[idx+1:]...)
			}
			var accepted, rejected *float64
			var catatan *string
			if req != nil {
				accepted, rejected, catatan = req.QtyAccepted, req.QtyRejected, req.Catatan
			}
			a, r := domain.NormalizeQc(c.QtyDiterima, accepted, rejected)
			qcItems = append(qcItems, QcItem{GrnItemID: c.ID, RawMaterialID: nilIfEmpty(c.RawMaterialID), ProductID: nilIfEmpty(c.ProductID),
				QtyInspected: c.QtyDiterima, QtyAccepted: a, QtyRejected: r, Catatan: catatan})
		}
		if len(qcItems) > 0 {
			acc, rej := make([]float64, len(qcItems)), make([]float64, len(qcItems))
			for i, q := range qcItems {
				acc[i], rej[i] = q.QtyAccepted, q.QtyRejected
			}
			result, err := s.submitQc(ctx, tx, QcInput{GrnID: grnID, Status: domain.QcOverallStatus(acc, rej), Catatan: nonEmpty(in.Catatan), Items: qcItems, UserID: user.ID})
			if err != nil {
				var apiErr *httpx.Error
				switch {
				case errors.As(err, &apiErr):
					return nil, err
				case isPgError(err):
					// A query-builder error object is not an Error instance.
					return nil, httpx.Status(500, "Gagal menyelesaikan QC dan posting stok pada penerimaan")
				}
				return nil, httpx.Status(500, err.Error())
			}
			status = result.GrnStatus
		}
	}

	switch {
	case status == domain.GrnPending || moduleType == "general":
		if status != domain.GrnRejected {
			if _, err := tx.Exec(ctx, `UPDATE deliveries SET status = 'delivered', updated_at = $2 WHERE id = $1`, deliveryID, s.now()); err != nil {
				return nil, err
			}
		} else if err := s.updateDeliveryStatusAfterGrn(ctx, tx, deliveryID, status); err != nil {
			return nil, err
		}
		if err := s.updatePOStatusAfterGrn(ctx, tx, poID); err != nil {
			return nil, err
		}
	case status == domain.GrnRejected && initial == domain.GrnRejected:
		if err := s.updateDeliveryStatusAfterGrn(ctx, tx, deliveryID, status); err != nil {
			return nil, err
		}
		if err := s.updatePOStatusAfterGrn(ctx, tx, poID); err != nil {
			return nil, err
		}
	}
	s.nonFatal(ctx, tx, "[GRN] Vendor credit sync error", func(sp pgx.Tx) error {
		return s.syncRejectCredits(ctx, sp, grnID, "receive_reject", user.ID)
	})
	if moduleType == "general" && status != domain.GrnRejected {
		if err := publishGrnPosted(ctx, tx, grnID, user.ID); err != nil {
			return nil, err
		}
	}
	grn.Set("status", status)
	return &GrnCreated{
		Grn: grn, Status: status, Message: domain.GrnCreatedMessage(number, status, moduleType, nil),
		Audit: audit.Entry{ActorID: user.ID, ActorName: nonEmpty(&user.FullName), Action: "grn.post", Entity: "grn", EntityID: &grnID, EntityLabel: &number,
			After: obj("status", status, "warehouse_id", in.WarehouseID, "purchase_order_id", poID, "items", auditItems)},
	}, nil
}

// publishSupplyStock is postSupplyStock: received stockable supply items
// go to inventory (non-stockable ones are expensed).
func (s *Service) publishSupplyStock(ctx context.Context, tx pgx.Tx, grn *Row, lines []grnLineInput, poItems []domain.PoItemForReceive, warehouseID string, companyID, branchID *string, userID string) error {
	var ids []string
	for _, l := range lines {
		if deref(l.SupplyItemID) != "" && l.QtyDiterima > 0 {
			ids = append(ids, *l.SupplyItemID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	stockable, err := s.ports.Catalog.StockableSupplies(ctx, tx, uniqueStrings(ids))
	if err != nil {
		s.log.ErrorContext(ctx, "[GRN] Supply stock posting error (non-fatal)", "error", err)
		return nil
	}
	ev := contracts.GrnStockReceived{GrnID: grn.Str("id"), GrnNumber: grn.Str("nomor_grn"), UserID: userID, CompanyID: companyID, BranchID: branchID}
	for _, l := range lines {
		supply := deref(l.SupplyItemID)
		if supply == "" || l.QtyDiterima <= 0 || !stockable[supply] {
			continue
		}
		cost := 0.0
		for _, p := range poItems {
			if (deref(l.PurchaseOrderItemID) != "" && p.ID == *l.PurchaseOrderItemID) || p.SupplyItemID == supply {
				cost = p.HargaSatuan
				break
			}
		}
		ev.Lines = append(ev.Lines, contracts.GrnStockLine{SupplyItemID: &supply, Qty: l.QtyDiterima, UnitCost: cost, WarehouseID: &warehouseID})
	}
	if len(ev.Lines) == 0 {
		return nil
	}
	return outbox.Publish(ctx, tx, contracts.TopicGrnStockReceived, grn.Str("id"), ev)
}

// poItemsForReceive reads the PO lines receiving validates against; with
// labels it also embeds the raw material name (GRN Continue messages).
func (s *Service) poItemsForReceive(ctx context.Context, q database.Querier, poID string, labels bool) ([]domain.PoItemForReceive, error) {
	rows, err := s.rows.Query(ctx, q, `SELECT id, raw_material_id, product_id, supply_item_id, pos_sku_id,
		qty_ordered::float8 AS qty_ordered, COALESCE(qty_received, 0)::float8 AS qty_received, harga_satuan::float8 AS harga_satuan
		FROM purchase_order_items WHERE purchase_order_id = $1 AND is_active = true`, poID)
	if err != nil {
		return nil, err
	}
	var names map[string]json.RawMessage
	if labels {
		if names, err = s.ports.Catalog.Refs(ctx, q, EntityRawMaterial, "id, nama", uniqueStrings(column(rows, "raw_material_id"))); err != nil {
			return nil, err
		}
	}
	out := make([]domain.PoItemForReceive, len(rows))
	for i, r := range rows {
		out[i] = domain.PoItemForReceive{
			ID: r.Str("id"), RawMaterialID: r.Str("raw_material_id"), ProductID: r.Str("product_id"), SupplyItemID: r.Str("supply_item_id"),
			PosSkuID: r.StrPtr("pos_sku_id"), QtyOrdered: r.Num("qty_ordered"), QtyReceived: r.Num("qty_received"), HargaSatuan: r.Num("harga_satuan"),
		}
		if raw, ok := names[r.Str("raw_material_id")]; ok {
			if rm, err := decodeRow(raw); err == nil {
				out[i].Label = rm.Str("nama")
			}
		}
	}
	return out, nil
}

// validateDeliveryCanReceive returns the delivery (nil when missing) and the
// problems that block receiving it.
func (s *Service) validateDeliveryCanReceive(ctx context.Context, q database.Querier, deliveryID string) (*Row, []string, error) {
	delivery, err := s.rows.One(ctx, q, `SELECT * FROM deliveries WHERE id = $1::text::uuid AND is_active = true`, deliveryID)
	if err != nil || delivery == nil {
		return nil, []string{"Delivery tidak ditemukan"}, nil
	}
	var errs []string
	switch delivery.Str("status") {
	case "pending", "shipped", "in_transit", "delivered":
	default:
		errs = append(errs, "Delivery dengan status "+jsString(delivery.Get("status"))+" tidak dapat diterima")
	}
	existing, err := s.rows.One(ctx, q, `SELECT id, nomor_grn, status FROM grn WHERE delivery_id = $1 AND is_active = true LIMIT 1`, delivery.Str("id"))
	if err == nil && existing != nil {
		label := existing.Str("nomor_grn")
		if label == "" {
			label = existing.Str("id")
		}
		errs = append(errs, "Delivery ini sudah memiliki dokumen Barang Masuk ("+label+"). Buat delivery baru jika ada pengiriman susulan.")
	}
	return delivery, errs, nil
}

// jsString is String(v) for a scalar column (null → "null").
func jsString(v any) string {
	if v == nil {
		return "null"
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := marshalJS(v)
	return string(b)
}

// createAutoDelivery opens the implicit delivery of a general PO receipt.
func (s *Service) createAutoDelivery(ctx context.Context, tx pgx.Tx, poID, userID string) (string, error) {
	if errs, err := s.validatePOCanDelivery(ctx, tx, poID); err != nil {
		return "", err
	} else if len(errs) > 0 {
		return "", badRequest(strings.Join(errs, " "))
	}
	po, err := s.rows.One(ctx, tx, `SELECT id, vendor_id, company_id, branch_id, module_type FROM purchase_orders WHERE id = $1::text::uuid`, poID)
	if err != nil || po == nil {
		return "", badRequest("Purchase order tidak ditemukan")
	}
	if po.Str("module_type") != "general" {
		return "", badRequest("Penerimaan otomatis hanya untuk purchase order barang operasional")
	}
	today := s.now().UTC().Format("2006-01-02")
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO deliveries
		(purchase_order_id, supplier_id, vendor_id, tanggal_kirim, no_surat_jalan, tanggal_estimasi_tiba, status, company_id, branch_id, created_by)
		VALUES ($1, NULL, $2, $3::text::date, $4, $3::text::date, 'pending', $5, $6, $7) RETURNING id::text`,
		po.Str("id"), po.StrPtr("vendor_id"), today, "AUTO-"+today, po.StrPtr("company_id"), po.StrPtr("branch_id"), userID).Scan(&id); err != nil {
		s.log.ErrorContext(ctx, "Auto delivery insert error", "error", err)
		return "", httpx.Status(500, "Gagal menyiapkan penerimaan barang operasional")
	}
	return id, nil
}

// validatePOCanDelivery returns the reasons a PO cannot take a delivery.
func (s *Service) validatePOCanDelivery(ctx context.Context, q database.Querier, poID string) ([]string, error) {
	po, err := s.rows.One(ctx, q, `SELECT id, nomor_po, status, supplier_id, is_active FROM purchase_orders WHERE id = $1::text::uuid`, poID)
	if err != nil {
		if isPgError(err) {
			return []string{"Database error: " + pgMessage(err)}, nil
		}
		return nil, err
	}
	if po == nil {
		return []string{"Database error: No rows found"}, nil
	}
	var errs []string
	if !po.Bool("is_active") {
		errs = append(errs, "Purchase order is no longer active")
	}
	if !domain.IsPoStatusEligibleForDelivery(po.Str("status")) {
		errs = append(errs, `Purchase order status is "`+jsString(po.Get("status"))+`". It must be approved, sent, or partially received before creating a delivery.`)
	}
	rows, err := s.rows.Query(ctx, q, `SELECT id, status FROM deliveries WHERE purchase_order_id = $1 AND is_active = true AND status <> 'cancelled'`, po.Str("id"))
	if err != nil {
		return append(errs, "Database error: "+pgMessage(err)), nil
	}
	for _, d := range rows {
		if domain.IsOpenDeliveryStatus(d.Str("status")) {
			errs = append(errs, "This purchase order already has an open delivery in progress.")
			break
		}
	}
	return errs, nil
}

/* ── Update and delete ───────────────────────────────────────────────── */

// UpdateGrn is updateGrn (GRN Continue).
func (s *Service) UpdateGrn(ctx context.Context, id string, in *updateGrnInput, userID string) (*Row, error) {
	var out *Row
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		current, err := s.rows.One(ctx, tx, `SELECT * FROM grn WHERE id = $1::text::uuid AND is_active = true`, id)
		if err != nil || current == nil {
			return notFound("GRN tidak ditemukan")
		}
		currentStatus := current.Str("status")
		if in.Status != nil && *in.Status != currentStatus {
			if msg := domain.GrnTransitionError(currentStatus, *in.Status); msg != "" {
				return badRequest(msg)
			}
		}
		existingRows, err := s.rows.Query(ctx, tx, `SELECT * FROM grn_items WHERE grn_id = $1 AND is_active = true`, current.Str("id"))
		if err != nil {
			existingRows = nil
		}
		existing := map[string]*Row{}
		qty := map[string]domain.ExistingGrnQty{}
		for _, r := range existingRows {
			key := domain.ReceiveLine{PurchaseOrderItemID: r.Str("purchase_order_item_id"), RawMaterialID: r.Str("raw_material_id")}.LineKey()
			existing[key] = r
			qty[key] = domain.ExistingGrnQty{QtyDiterima: r.Num("qty_diterima"), QtyDitolak: r.Num("qty_ditolak")}
		}
		receive := make([]domain.ReceiveLine, len(in.Items))
		for i, l := range in.Items {
			receive[i] = l.receive()
		}
		if len(in.Items) > 0 {
			poItems, err := s.poItemsForReceive(ctx, tx, current.Str("purchase_order_id"), true)
			if err != nil {
				return err
			}
			if err := domain.ValidateAdditionalReceive(receive, poItems, qty); err != nil {
				return receiveErr(err)
			}
			linked := false
			for _, l := range in.Items {
				linked = linked || deref(l.PurchaseOrderItemID) != ""
			}
			if !linked {
				return badRequest("Minimal 1 item harus terhubung dengan item PO")
			}
		}
		qtyChanged := false
		for _, l := range receive {
			if l.QtyDiterima-qty[l.LineKey()].QtyDiterima != 0 {
				qtyChanged = true
			}
		}
		sets := []string{"updated_by = $2", "updated_at = $3"}
		args := []any{current.Str("id"), userID, s.now()}
		add := func(col string, v any) {
			args = append(args, v)
			sets = append(sets, col+" = $"+strconv.Itoa(len(args)))
		}
		status := ""
		if in.Status != nil {
			status = *in.Status
		}
		if in.Catatan != nil {
			add("catatan", *in.Catatan)
		}
		if deref(in.TanggalPenerimaan) != "" {
			args = append(args, *in.TanggalPenerimaan)
			sets = append(sets, "tanggal_penerimaan = $"+strconv.Itoa(len(args))+"::text::date")
		}
		if len(in.Items) > 0 {
			diterima, ditolak := domain.GrnTotals(receive)
			args = append(args, diterima, ditolak)
			sets = append(sets, "total_item_diterima = $"+strconv.Itoa(len(args)-1)+"::numeric", "total_item_ditolak = $"+strconv.Itoa(len(args))+"::numeric")
			if fromLines := domain.StatusFromLines(receive); fromLines != "" {
				status = fromLines
			}
		}
		if qtyChanged && currentStatus != domain.GrnPending {
			status = domain.GrnPending
			if _, err := tx.Exec(ctx, `DELETE FROM grn_qc_inspection_items WHERE qc_inspection_id IN (SELECT id FROM grn_qc_inspections WHERE grn_id = $1)`, current.Str("id")); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `DELETE FROM grn_qc_inspections WHERE grn_id = $1`, current.Str("id")); err != nil {
				return err
			}
		}
		if status != "" {
			add("status", status)
		}
		out, err = s.rows.One(ctx, tx, `UPDATE grn SET `+strings.Join(sets, ", ")+` WHERE id = $1 RETURNING *`, args...)
		if err != nil {
			return err
		}
		if len(in.Items) > 0 {
			if _, err := tx.Exec(ctx, `UPDATE grn_items SET is_active = false WHERE grn_id = $1`, current.Str("id")); err != nil {
				return err
			}
			for _, l := range in.Items {
				prev := existing[l.receive().LineKey()]
				var warehouse, batch, expiry *string
				posted := 0.0
				if prev != nil {
					warehouse, batch, expiry = prev.StrPtr("warehouse_id"), prev.StrPtr("batch_number"), dateOnly(prev.Get("expiry_date"), s.loc)
					posted = prev.Num("qty_qc_posted")
				}
				if b := strings.TrimSpace(deref(l.BatchNumber)); b != "" {
					batch = &b
				}
				if e := nonEmpty(l.ExpiryDate); e != nil {
					expiry = e
				}
				if _, err := tx.Exec(ctx, `INSERT INTO grn_items
					(grn_id, delivery_id, purchase_order_item_id, raw_material_id, qty_diterima, qty_ditolak, kondisi, catatan,
					 warehouse_id, qc_status, qty_qc_posted, batch_number, expiry_date)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'pending', $10, $11, $12::text::date)`,
					current.Str("id"), current.StrPtr("delivery_id"), l.PurchaseOrderItemID, l.RawMaterialID, l.QtyDiterima, l.QtyDitolak,
					l.Kondisi, nonEmpty(l.Catatan), warehouse, posted, batch, expiry); err != nil {
					return err
				}
			}
		}
		if status != "" && status != domain.GrnPending && current.Str("delivery_id") != "" {
			if err := s.updateDeliveryStatusAfterGrn(ctx, tx, current.Str("delivery_id"), status); err != nil {
				return err
			}
		}
		if po := current.Str("purchase_order_id"); po != "" {
			if err := s.updatePOStatusAfterGrn(ctx, tx, po); err != nil {
				return err
			}
		}
		s.nonFatal(ctx, tx, "[PATCH GRN] Vendor credit sync error", func(sp pgx.Tx) error {
			return s.syncRejectCredits(ctx, sp, current.Str("id"), "receive_reject", userID)
		})
		return nil
	})
	return out, err
}

// dateOnly is toDateOnly for a date column read through RowReader (local
// midnight in the process time zone).
func dateOnly(v any, loc *time.Location) *string {
	t, ok := v.(httpx.JSTime)
	if !ok {
		return nil
	}
	s := time.Time(t).In(loc).Format("2006-01-02")
	return &s
}

// DeleteGrn is deleteGrn: soft delete the GRN and its lines.
func (s *Service) DeleteGrn(ctx context.Context, id, userID string) (*Row, string, error) {
	var deleted *Row
	var number string
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		grn, err := s.rows.One(ctx, tx, `SELECT * FROM grn WHERE id = $1::text::uuid AND is_active = true`, id)
		if err != nil || grn == nil {
			return notFound("GRN tidak ditemukan")
		}
		number = grn.Str("nomor_grn")
		if _, err := tx.Exec(ctx, `UPDATE grn_items SET is_active = false WHERE grn_id = $1`, grn.Str("id")); err != nil {
			return err
		}
		if deleted, err = s.rows.One(ctx, tx, `UPDATE grn SET is_active = false, updated_by = $2 WHERE id = $1 RETURNING *`, grn.Str("id"), userID); err != nil {
			return err
		}
		if po := grn.Str("purchase_order_id"); po != "" {
			return s.updatePOStatusAfterGrn(ctx, tx, po)
		}
		return nil
	})
	return deleted, number, err
}

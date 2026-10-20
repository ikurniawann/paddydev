package desktop

import (
	"context"
	"fmt"
	"strings"
	"time"

	"nuhabit/backend/internal/modules/desktop/domain"
	"nuhabit/backend/internal/platform/database"
)

// Postgres implements Repository with the SQL of lib/desktop/overview.ts
// and the desktop routes. Unqualified tables (purchase_orders,
// crm_member_profiles, …) resolve through the shared search_path as in TS.
type Postgres struct{ db database.Querier }

// NewPostgres builds the repository on a pool or transaction.
func NewPostgres(db database.Querier) *Postgres { return &Postgres{db: db} }

// paidOrder is the revenue filter shared with the profit report and the Do
// tool penjualan_periode: paid, and not cancelled, voided or merged.
const paidOrder = `payment_status = 'paid'
        AND status::text NOT IN ('cancelled', 'voided', 'merged')`

// SalesByDay is fetchSalesPulse's eight WIB days of paid sales.
func (p *Postgres) SalesByDay(ctx context.Context, today string) ([]DayRow, error) {
	rows, err := p.db.Query(ctx, `SELECT (COALESCE(ordered_at, created_at) AT TIME ZONE 'Asia/Jakarta')::date::text AS tanggal,
            COALESCE(sum(total_amount), 0)::float8 AS omzet,
            count(*)::int AS pesanan
       FROM pos.pos_orders
      WHERE COALESCE(ordered_at, created_at) >= ($1::date - interval '7 days')
        AND COALESCE(ordered_at, created_at) < ($1::date + interval '1 day')
        AND `+paidOrder+`
      GROUP BY 1
      ORDER BY 1`, today)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DayRow
	for rows.Next() {
		var d DayRow
		if err := rows.Scan(&d.Tanggal, &d.Omzet, &d.Pesanan); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// GrossProfitToday is today's sales minus the per-item cost snapshot.
func (p *Postgres) GrossProfitToday(ctx context.Context, today string) (*GrossProfit, error) {
	var g GrossProfit
	err := p.db.QueryRow(ctx, `SELECT COALESCE(sum(i.total_amount - COALESCE(i.cost_total, 0)), 0)::float8 AS laba,
              count(*) FILTER (WHERE COALESCE(i.cost_total, 0) = 0)::int AS item_tanpa_modal
         FROM pos.pos_order_items i
         JOIN pos.pos_orders o ON o.id = i.order_id
        WHERE (COALESCE(o.ordered_at, o.created_at) AT TIME ZONE 'Asia/Jakarta')::date = $1::date
          AND o.payment_status = 'paid'
          AND o.status::text NOT IN ('cancelled', 'voided', 'merged')`, today).Scan(&g.Laba, &g.ItemTanpaModal)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// RevenueTotals is fetchRevenuePeriod's two queries: F&B orders by WIB
// creation date, and B2B recognised when paid (paid_on), including
// deal-level payments without an invoice.
func (p *Postgres) RevenueTotals(ctx context.Context, sum domain.PeriodSummary) (FnbTotals, B2bTotals, error) {
	args := []any{sum.Periode.Mulai, sum.Periode.Selesai, sum.Banding.Mulai, sum.Banding.Selesai}
	var fnb FnbTotals
	var b2b B2bTotals
	err := p.db.QueryRow(ctx, `WITH terpakai AS (
         SELECT total_amount,
                (created_at AT TIME ZONE 'Asia/Jakarta')::date AS tanggal
           FROM pos.pos_orders
          WHERE status <> 'cancelled'
            AND voided_at IS NULL
       )
       SELECT
         COALESCE(sum(total_amount) FILTER (WHERE tanggal BETWEEN $1::date AND $2::date), 0)::float8 AS omzet,
         count(*) FILTER (WHERE tanggal BETWEEN $1::date AND $2::date)::int AS pesanan,
         COALESCE(sum(total_amount) FILTER (WHERE tanggal BETWEEN $3::date AND $4::date), 0)::float8 AS banding_omzet,
         count(*) FILTER (WHERE tanggal BETWEEN $3::date AND $4::date)::int AS banding_pesanan,
         count(*) FILTER (WHERE tanggal BETWEEN $3::date AND $2::date)::int AS baris
         FROM terpakai`, args...).Scan(&fnb.Omzet, &fnb.Pesanan, &fnb.BandingOmzet, &fnb.BandingPesanan, &fnb.Baris)
	if err != nil {
		return fnb, b2b, err
	}
	err = p.db.QueryRow(ctx, `SELECT
         COALESCE(sum(amount) FILTER (WHERE paid_on BETWEEN $1::date AND $2::date), 0)::float8 AS omzet,
         COALESCE(sum(amount) FILTER (WHERE paid_on BETWEEN $3::date AND $4::date), 0)::float8 AS banding_omzet,
         count(*) FILTER (WHERE paid_on BETWEEN $3::date AND $2::date)::int AS baris
         FROM crm.crm_sales_deal_payments
        WHERE deleted_at IS NULL`, args...).Scan(&b2b.Omzet, &b2b.BandingOmzet, &b2b.Baris)
	return fnb, b2b, err
}

// GuestsSeated is fetchGuestsSeated: one open bill is one party; tables are
// counted once. Zero seated tables is a fact, so adaData is always true.
func (p *Postgres) GuestsSeated(ctx context.Context) (*GuestsSeated, error) {
	g := GuestsSeated{AdaData: true}
	err := p.db.QueryRow(ctx, `SELECT COALESCE(sum(o.guest_count), 0)::float8 AS tamu,
            count(DISTINCT o.table_id)::int AS meja,
            COALESCE(sum(DISTINCT t.capacity), 0)::float8 AS kapasitas
       FROM pos.pos_orders o
       LEFT JOIN pos.pos_tables t ON t.id::text = o.table_id
      WHERE o.table_id IS NOT NULL
        AND o.voided_at IS NULL
        AND o.status IN ('pending', 'confirmed', 'preparing', 'ready', 'served')`).Scan(&g.Tamu, &g.Meja, &g.Kapasitas)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// PromoLines is fetchPromoImpact's top five captured campaigns.
func (p *Postgres) PromoLines(ctx context.Context, period domain.Period) ([]PromoLine, error) {
	rows, err := p.db.Query(ctx, `SELECT COALESCE(r.campaign_name, '(tanpa nama)') AS kampanye,
            COALESCE(sum(r.discount_amount), 0)::float8 AS diskon,
            COALESCE(sum(o.total_amount), 0)::float8 AS omzet,
            count(*)::int AS redemption
       FROM promo.promo_redemptions r
       LEFT JOIN pos.pos_orders o
              ON o.id = r.context_id
             AND o.status <> 'cancelled'
             AND o.voided_at IS NULL
      WHERE r.status = 'captured'
        AND r.context_type = 'pos_order'
        AND (r.created_at AT TIME ZONE 'Asia/Jakarta')::date BETWEEN $1::date AND $2::date
      GROUP BY 1
      ORDER BY 2 DESC
      LIMIT 5`, period.Mulai, period.Selesai)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PromoLine{}
	for rows.Next() {
		var l PromoLine
		if err := rows.Scan(&l.Kampanye, &l.Diskon, &l.Omzet, &l.Redemption); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// TeamToday is fetchTeamToday's counts.
func (p *Postgres) TeamToday(ctx context.Context, today string) (*TeamToday, error) {
	var t TeamToday
	err := p.db.QueryRow(ctx, `SELECT
       (SELECT count(*) FROM hris.employees WHERE is_active)::int AS aktif,
       (SELECT count(*) FROM hris.attendance WHERE date = $1::date)::int AS hadir,
       (SELECT count(*) FROM hris.attendance WHERE date = $1::date AND is_late)::int AS terlambat,
       (SELECT count(DISTINCT l.employee_id)
          FROM hris.leaves l
          JOIN hris.employees e ON e.id = l.employee_id AND e.is_active
         WHERE l.status = 'approved'
           AND $1::date BETWEEN l.start_date AND l.end_date)::int AS cuti`, today).
		Scan(&t.Aktif, &t.Hadir, &t.Terlambat, &t.Cuti)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// PendingDecisions uses the nav-badges definition of pending.
func (p *Postgres) PendingDecisions(ctx context.Context) (*PendingDecisions, error) {
	var d PendingDecisions
	err := p.db.QueryRow(ctx, `SELECT
       (SELECT count(*) FROM hris.leaves WHERE status = 'pending')::int AS cuti,
       (SELECT count(*) FROM hris.overtime_requests WHERE status = 'pending')::int AS lembur,
       (SELECT count(*) FROM hris.loans WHERE status = 'pending')::int AS pinjaman,
       (SELECT count(*) FROM purchase_orders WHERE status = 'draft')::int AS po_draft,
       (SELECT count(*) FROM recruitment.candidates WHERE status = 'applied')::int AS kandidat`).
		Scan(&d.Cuti, &d.Lembur, &d.Pinjaman, &d.PoDraft, &d.KandidatBaru)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// lowStockFrom is the stok_menipis query shared by the board and inbox.
const lowStockFrom = `
       FROM inventory.inventory i
       JOIN item.raw_materials rm ON rm.id = i.raw_material_id
      WHERE i.is_active
        AND rm.deleted_at IS NULL
        AND i.qty_available <= i.qty_minimum
      ORDER BY (i.qty_minimum - i.qty_available) DESC`

// LowStockItems is fetchLowStock's full list.
func (p *Postgres) LowStockItems(ctx context.Context) ([]LowStockItem, error) {
	rows, err := p.db.Query(ctx, `SELECT rm.nama AS bahan,
            i.qty_available::float8 AS tersedia,
            i.qty_minimum::float8 AS minimum`+lowStockFrom)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LowStockItem{}
	for rows.Next() {
		var it LowStockItem
		var bahan *string
		if err := rows.Scan(&bahan, &it.Tersedia, &it.Minimum); err != nil {
			return nil, err
		}
		it.Bahan = deref(bahan)
		out = append(out, it)
	}
	return out, rows.Err()
}

// CrmPulse is fetchCrmPulse's seven-day counts.
func (p *Postgres) CrmPulse(ctx context.Context) (*CrmPulse, error) {
	var c CrmPulse
	err := p.db.QueryRow(ctx, `SELECT
       (SELECT count(*) FROM crm_member_profiles
         WHERE created_at > now() - interval '7 days')::int AS member,
       (SELECT COALESCE(sum(xp_delta), 0) FROM crm_xp_ledger
         WHERE created_at > now() - interval '7 days' AND xp_delta > 0)::int AS xp,
       (SELECT count(*) FROM crm_redemptions
         WHERE created_at > now() - interval '7 days')::int AS redeem`).
		Scan(&c.MemberBaru7Hari, &c.XpTerdistribusi7Hari, &c.RewardDitukar7Hari)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ── inbox ────────────────────────────────────────────────────────────────

// InboxSection loads one inbox list with its full count.
func (p *Postgres) InboxSection(ctx context.Context, key string) (*InboxSection, error) {
	sec := &InboxSection{Key: key, Label: domain.InboxSectionLabel[key], Items: []InboxItem{}}
	var sql string
	switch key {
	case "cuti":
		sql = `SELECT l.id::text, COALESCE(l.leave_type::text, 'Cuti'), l.start_date::text, l.end_date::text,
		            COALESCE(l.total_days::float8, 1), COALESCE(e.full_name, 'Karyawan'),
		            count(*) OVER ()::int AS total
		       FROM hris.leaves l
		       LEFT JOIN hris.employees e ON e.id = l.employee_id
		      WHERE l.status = 'pending'
		      ORDER BY l.start_date
		      LIMIT %d`
	case "po":
		sql = `SELECT po.id::text, COALESCE(po.nomor_po, 'PO'), COALESCE(s.nama_supplier, 'Supplier'),
		            COALESCE(po.total::float8, 0), count(*) OVER ()::int AS total_count
		       FROM purchasing.purchase_orders po
		       LEFT JOIN purchasing.suppliers s ON s.id = po.supplier_id
		      WHERE po.status = 'draft'
		      ORDER BY po.created_at DESC
		      LIMIT %d`
	case "stok":
		sql = `SELECT rm.id::text, COALESCE(rm.nama, '-'), COALESCE(i.qty_available::float8, 0),
		            COALESCE(i.qty_minimum::float8, 0), count(*) OVER ()::int AS total_count` + lowStockFrom + `
		      LIMIT %d`
	default:
		return nil, fmt.Errorf("unknown inbox section %q", key)
	}
	rows, err := p.db.Query(ctx, fmt.Sprintf(sql, domain.InboxItemLimit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var it InboxItem
		switch key {
		case "cuti":
			var leaveType string
			var start, end *string
			var days float64
			if err := rows.Scan(&it.ID, &leaveType, &start, &end, &days, &it.Title, &sec.Total); err != nil {
				return nil, err
			}
			it.Subtitle = domain.LeaveTypeLabel(leaveType) + " · " + domain.DescribeLeaveRange(deref(start), deref(end), days)
			it.Href, it.Actionable = "/dashboard/hris/workforce/leaves", true
		case "po":
			var supplier string
			var total float64
			if err := rows.Scan(&it.ID, &it.Title, &supplier, &total, &sec.Total); err != nil {
				return nil, err
			}
			it.Subtitle = supplier + " · Rp " + domain.FormatID(total)
			it.Href = "/dashboard/items/raw-material/approval/po"
		case "stok":
			var available, minimum float64
			if err := rows.Scan(&it.ID, &it.Title, &available, &minimum, &sec.Total); err != nil {
				return nil, err
			}
			it.Subtitle = "sisa " + domain.JSNumber(available) + " dari minimum " + domain.JSNumber(minimum)
			it.Href = "/dashboard/items/raw-material/purchasing/pr"
		}
		sec.Items = append(sec.Items, it)
	}
	return sec, rows.Err()
}

// ── search ───────────────────────────────────────────────────────────────

// searchSQL is the per-source spotlight query; $1 is the escaped pattern.
var searchSQL = map[string]string{
	"order": `SELECT id::text, order_number, total_amount::float8, status::text
		FROM pos.pos_orders
		WHERE order_number ILIKE $1 ESCAPE '\'
		ORDER BY created_at DESC LIMIT %d`,
	"member": `SELECT id::text, name, phone, membership_tier::text
		FROM pos.pos_customers
		WHERE (name ILIKE $1 ESCAPE '\' OR phone ILIKE $1 ESCAPE '\') AND is_active
		ORDER BY name NULLS LAST LIMIT %d`,
	"product": `SELECT id::text, name, sku, base_price::float8
		FROM pos.pos_products
		WHERE (name ILIKE $1 ESCAPE '\' OR sku ILIKE $1 ESCAPE '\') AND is_active
		ORDER BY name LIMIT %d`,
	"material": `SELECT id::text, kode, nama
		FROM item.raw_materials
		WHERE (nama ILIKE $1 ESCAPE '\' OR kode ILIKE $1 ESCAPE '\') AND deleted_at IS NULL
		ORDER BY nama LIMIT %d`,
	"employee": `SELECT id::text, full_name, nip, email
		FROM hris.employees
		WHERE (full_name ILIKE $1 ESCAPE '\' OR nip ILIKE $1 ESCAPE '\') AND is_active
		ORDER BY full_name LIMIT %d`,
	"document": `SELECT id::text, name, kind::text
		FROM dataroom.nodes
		WHERE name ILIKE $1 ESCAPE '\'
		ORDER BY name LIMIT %d`,
}

// SearchSource runs one source and maps its rows to hits as the route does.
func (p *Postgres) SearchSource(ctx context.Context, key, pattern string) ([]domain.SearchHit, error) {
	sql, ok := searchSQL[key]
	if !ok {
		return nil, nil
	}
	rows, err := p.db.Query(ctx, fmt.Sprintf(sql, domain.SearchLimitPerSource), pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.SearchHit
	for rows.Next() {
		var id string
		var a, b, c *string
		var n *float64
		hit := domain.SearchHit{Source: key}
		switch key {
		case "order":
			err = rows.Scan(&id, &a, &n, &b)
			hit.Title = strOr(a, "-")
			hit.Subtitle = ptr(deref(b) + " · Rp " + domain.FormatID(orZero(n)))
			hit.Href = "/dashboard/pos/orders?q=" + encodeURIComponent(deref(a))
		case "member":
			err = rows.Scan(&id, &a, &b, &c)
			hit.Title = firstNonEmpty(deref(a), deref(b), "-")
			sub := deref(b)
			if deref(c) != "" {
				sub += " · " + *c
			}
			hit.Subtitle = &sub
			q := b
			if q == nil {
				q = a
			}
			hit.Href = "/dashboard/crm/members?q=" + encodeURIComponent(deref(q))
		case "product":
			err = rows.Scan(&id, &a, &b, &n)
			hit.Title = strOr(a, "-")
			hit.Subtitle = ptr(deref(b) + " · Rp " + domain.FormatID(orZero(n)))
			hit.Href = "/dashboard/pos/products?q=" + encodeURIComponent(deref(a))
		case "material":
			err = rows.Scan(&id, &a, &b)
			hit.Title = strOr(b, "-")
			hit.Subtitle = ptr(deref(a))
			hit.Href = "/dashboard/items/raw-material/master/materials?q=" + encodeURIComponent(deref(a))
		case "employee":
			err = rows.Scan(&id, &a, &b, &c)
			hit.Title = strOr(a, "-")
			sub := deref(b)
			if deref(c) != "" {
				sub += " · " + *c
			}
			hit.Subtitle = &sub
			hit.Href = "/dashboard/hris/kepegawaian/users?q=" + encodeURIComponent(deref(a))
		case "document":
			err = rows.Scan(&id, &a, &b)
			hit.Title = strOr(a, "-")
			hit.Subtitle = ptr(deref(b))
			hit.Href = "/dashboard/dataroom"
		}
		if err != nil {
			return nil, err
		}
		hit.ID = id
		out = append(out, hit)
	}
	return out, rows.Err()
}

// ── status / settings / preferences ─────────────────────────────────────

// Ping is the status route's SELECT 1.
func (p *Postgres) Ping(ctx context.Context) error {
	var one int
	return p.db.QueryRow(ctx, `SELECT 1`).Scan(&one)
}

// PrintQueue counts unprinted jobs and the age of the oldest in minutes.
func (p *Postgres) PrintQueue(ctx context.Context) (int, int, error) {
	var pending, oldest int
	err := p.db.QueryRow(ctx, `SELECT count(*)::int AS pending,
              COALESCE(EXTRACT(EPOCH FROM (now() - min(created_at))) / 60, 0)::int AS oldest_minutes
         FROM pos.pos_print_jobs
        WHERE status IN ('pending', 'queued', 'printing')`).Scan(&pending, &oldest)
	return pending, oldest, err
}

// Setting is getSetting.
func (p *Postgres) Setting(ctx context.Context, key string) (*string, error) {
	var v *string
	err := p.db.QueryRow(ctx, `SELECT value FROM configuration.app_settings WHERE key = $1`, key).Scan(&v)
	if database.IsNoRows(err) {
		return nil, nil
	}
	return v, err
}

// Preferences reads the saved jsonb as text.
func (p *Postgres) Preferences(ctx context.Context, userID string) ([]byte, error) {
	var raw []byte
	err := p.db.QueryRow(ctx, `SELECT prefs::text FROM configuration.user_desktop_prefs WHERE user_id = $1`, userID).Scan(&raw)
	if database.IsNoRows(err) {
		return nil, nil
	}
	return raw, err
}

// SavePreferences is the route's upsert on user_id.
func (p *Postgres) SavePreferences(ctx context.Context, userID string, prefs []byte, at time.Time) error {
	_, err := p.db.Exec(ctx, `INSERT INTO configuration.user_desktop_prefs (user_id, prefs, updated_at)
		VALUES ($1, $2::jsonb, $3)
		ON CONFLICT (user_id) DO UPDATE SET prefs = EXCLUDED.prefs, updated_at = EXCLUDED.updated_at`,
		userID, string(prefs), at)
	return err
}

// ── helpers ──────────────────────────────────────────────────────────────

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// strOr is `String(x ?? fallback)`.
func strOr(s *string, fallback string) string {
	if s == nil {
		return fallback
	}
	return *s
}

func orZero(n *float64) float64 {
	if n == nil {
		return 0
	}
	return *n
}

func ptr(s string) *string { return &s }

// firstNonEmpty is `a || b || fallback`.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// encodeURIComponent leaves A-Z a-z 0-9 - _ . ! ~ * ' ( ) as they are and
// percent-encodes every other UTF-8 byte.
func encodeURIComponent(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || strings.IndexByte("-_.!~*'()", c) >= 0 {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}

package app

import (
	"context"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/modules/posops"
	"nuhabit/backend/internal/modules/posops/domain"
	"nuhabit/backend/internal/modules/shop"
	"nuhabit/backend/internal/platform/database"
)

// Catalog adapters of pos-ops: stall assignments (configuration),
// purchasing products (inventory: item.products, v_products_cogs) and the
// online shop channels (shop, not ported in this wave). SQL from
// lib/pos/stall-product-scope.ts, pos-sell-stall-server.ts,
// purchasing-sync.ts and lib/users/user-warehouses.ts.

/* ── Stalls (configuration) ──────────────────────────────────────────── */

type posOpsStalls struct{}

var _ posops.Stalls = posOpsStalls{}

func (posOpsStalls) Flags(ctx context.Context, q database.Querier, userID string) (posops.StallFlags, error) {
	var f posops.StallFlags
	err := q.QueryRow(ctx, `SELECT COALESCE(can_central_checkout, false), COALESCE(can_switch_stall, false),
		default_warehouse_id::text FROM configuration.users WHERE id = $1::text::uuid`, userID).
		Scan(&f.CanCentralCheckout, &f.CanSwitchStall, &f.DefaultWarehouseID)
	if database.IsNoRows(err) {
		return f, nil
	}
	return f, err
}

func scanPosOpsStalls(rows pgx.Rows, err error) ([]domain.Stall, error) {
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Stall, error) {
		var s domain.Stall
		err := r.Scan(&s.ID, &s.Name, &s.Code, &s.BranchID, &s.IsDefault, &s.BranchName)
		return s, err
	})
}

func (posOpsStalls) Assigned(ctx context.Context, q database.Querier, userID string) ([]domain.Stall, error) {
	stalls, err := scanPosOpsStalls(q.Query(ctx, `SELECT uw.warehouse_id::text, w.name, w.code, w.branch_id::text, w.is_default, COALESCE(b.name, '')
		FROM configuration.user_warehouses uw
		INNER JOIN configuration.warehouses w ON w.id = uw.warehouse_id
		LEFT JOIN configuration.branches b ON b.id = w.branch_id
		WHERE uw.user_id = $1::text::uuid AND uw.is_active = true AND w.is_active = true
		ORDER BY w.is_default DESC, w.name ASC`, userID))
	if database.IsUndefinedTable(err) {
		return nil, nil
	}
	return stalls, err
}

func (posOpsStalls) Active(ctx context.Context, q database.Querier, id string) (*domain.Stall, error) {
	stalls, err := scanPosOpsStalls(q.Query(ctx, `SELECT w.id::text, w.name, w.code, w.branch_id::text, w.is_default, COALESCE(b.name, '')
		FROM configuration.warehouses w
		LEFT JOIN configuration.branches b ON b.id = w.branch_id
		WHERE w.id = $1::uuid AND w.is_active = true`, id))
	if err != nil || len(stalls) == 0 {
		return nil, err
	}
	return &stalls[0], nil
}

func (posOpsStalls) Branch(ctx context.Context, q database.Querier, branchID *string) ([]domain.Stall, error) {
	return scanPosOpsStalls(q.Query(ctx, `SELECT w.id::text, w.name, w.code, w.branch_id::text, w.is_default, COALESCE(b.name, '')
		FROM configuration.warehouses w
		LEFT JOIN configuration.branches b ON b.id = w.branch_id
		WHERE w.is_active AND ($1::uuid IS NULL OR w.branch_id = $1::uuid)`, branchID))
}

/* ── Inventory (purchasing products) ─────────────────────────────────── */

type posOpsInventory struct{}

var _ posops.Inventory = posOpsInventory{}

func (posOpsInventory) ProductIDsForStalls(ctx context.Context, q database.Querier, warehouseIDs []string) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT DISTINCT pp.id::text
		FROM pos.pos_products pp
		INNER JOIN item.products p ON p.id = pp.source_product_id
		WHERE p.warehouse_id = ANY($1::uuid[]) AND p.deleted_at IS NULL AND p.is_active = true
		UNION
		SELECT DISTINCT pp.id::text
		FROM pos.pos_products pp
		INNER JOIN item.products p ON pp.sku = ('PUR-' || p.kode)
		WHERE pp.source_product_id IS NULL
		  AND p.warehouse_id = ANY($1::uuid[]) AND p.deleted_at IS NULL AND p.is_active = true
		  AND p.kode IS NOT NULL AND btrim(p.kode) <> ''`, warehouseIDs)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func (posOpsInventory) ProductStalls(ctx context.Context, q database.Querier, productIDs []string) (map[string]posops.ProductStall, error) {
	out := map[string]posops.ProductStall{}
	if len(productIDs) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, `SELECT pp.id::text,
		       COALESCE(p.warehouse_id, p_sku.warehouse_id)::text,
		       COALESCE(w.code, w_sku.code),
		       COALESCE(w.name, w_sku.name)
		FROM pos.pos_products pp
		LEFT JOIN item.products p ON p.id = pp.source_product_id AND p.deleted_at IS NULL
		LEFT JOIN configuration.warehouses w ON w.id = p.warehouse_id
		LEFT JOIN item.products p_sku
		  ON pp.source_product_id IS NULL AND pp.sku = ('PUR-' || p_sku.kode)
		 AND p_sku.deleted_at IS NULL AND p_sku.kode IS NOT NULL AND btrim(p_sku.kode) <> ''
		LEFT JOIN configuration.warehouses w_sku ON w_sku.id = p_sku.warehouse_id
		WHERE pp.id = ANY($1::uuid[])`, productIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var s posops.ProductStall
		if err := rows.Scan(&id, &s.WarehouseID, &s.Code, &s.Name); err != nil {
			return nil, err
		}
		out[id] = s
	}
	return out, rows.Err()
}

func (posOpsInventory) CogsByKode(ctx context.Context, q database.Querier, kodes []string) (map[string]posops.Cogs, error) {
	rows, err := q.Query(ctx, `SELECT COALESCE(kode, ''), hpp_estimasi::text, estimated_cogs::text
		FROM v_products_cogs WHERE kode = ANY($1::text[])`, kodes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]posops.Cogs{}
	for rows.Next() {
		var kode string
		var c posops.Cogs
		if err := rows.Scan(&kode, &c.HppEstimasi, &c.EstimatedCogs); err != nil {
			return nil, err
		}
		out[kode] = c
	}
	return out, rows.Err()
}

func (posOpsInventory) PurchasingProduct(ctx context.Context, q database.Querier, id string) (*posops.PurchasingProduct, error) {
	var p posops.PurchasingProduct
	err := q.QueryRow(ctx, `SELECT id::text, kode, nama, deskripsi, kategori, harga_jual::text,
		hpp_estimasi::text, estimated_cogs::text, is_active
		FROM v_products_cogs WHERE id = $1::text::uuid`, id).
		Scan(&p.ID, &p.Kode, &p.Nama, &p.Deskripsi, &p.Kategori, &p.HargaJual, &p.HppEstimasi, &p.EstimatedCogs, &p.IsActive)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func scanPosOpsRawMaterial(r pgx.CollectableRow) (domain.RawMaterialStock, error) {
	var m domain.RawMaterialStock
	err := r.Scan(&m.ID, &m.Kode, &m.Nama, &m.Kategori, &m.QtyOnhand, &m.MinStock, &m.Satuan, &m.StatusStok)
	return m, err
}

const posOpsRawMaterialColumns = `id::text, kode, nama, kategori, qty_onhand::text, min_stock::text, satuan, status_stok`

func (posOpsInventory) LowRawMaterials(ctx context.Context, q database.Querier) ([]domain.RawMaterialStock, error) {
	rows, err := q.Query(ctx, `SELECT `+posOpsRawMaterialColumns+` FROM v_raw_materials_stock
		WHERE status_stok IN ('MENIPIS', 'HABIS') AND is_active = true
		ORDER BY qty_onhand ASC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanPosOpsRawMaterial)
}

func (posOpsInventory) ActiveBom(ctx context.Context, q database.Querier) ([]domain.BomLine, error) {
	rows, err := q.Query(ctx, `SELECT COALESCE(b.product_id::text, ''), COALESCE(b.raw_material_id::text, ''),
		b.qty_required::text, b.waste_factor::text, p.id IS NOT NULL, p.kode, p.nama, p.is_active, m.nama
		FROM manufacturing.bom_items b
		LEFT JOIN item.products p ON p.id = b.product_id
		LEFT JOIN item.raw_materials m ON m.id = b.raw_material_id
		WHERE b.is_active = true`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.BomLine, error) {
		var b domain.BomLine
		err := r.Scan(&b.ProductID, &b.RawMaterialID, &b.QtyRequired, &b.WasteFactor, &b.ProductFound,
			&b.ProductKode, &b.ProductNama, &b.ProductActive, &b.MaterialNama)
		return b, err
	})
}

func (posOpsInventory) MaterialStock(ctx context.Context, q database.Querier, ids []string) (map[string]domain.RawMaterialStock, error) {
	rows, err := q.Query(ctx, `SELECT `+posOpsRawMaterialColumns+` FROM v_raw_materials_stock WHERE id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, scanPosOpsRawMaterial)
	out := map[string]domain.RawMaterialStock{}
	for _, m := range list {
		out[m.ID] = m
	}
	return out, err
}

/* ── Shop (online catalog distribution) ──────────────────────────────── */

// posOpsShop adapts the shop's channel service.
type posOpsShop struct{ channels shop.Channels }

var _ posops.Shop = posOpsShop{}

func (s posOpsShop) ProductChannels(ctx context.Context, q database.Querier, productIDs []string) (map[string][]posops.ProductChannel, error) {
	byProduct, err := s.channels.ByProduct(ctx, q, productIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]posops.ProductChannel, len(byProduct))
	for id, channels := range byProduct {
		for _, c := range channels {
			out[id] = append(out[id], posops.ProductChannel(c))
		}
	}
	return out, nil
}

func (s posOpsShop) SetWebDistribution(ctx context.Context, q database.Querier, productID string, distributed bool) error {
	return s.channels.SetWeb(ctx, q, productID, distributed)
}

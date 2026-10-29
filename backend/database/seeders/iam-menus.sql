-- =============================================================================
-- IAM Menu Seeder — canonical menu tree (generated from active iam.menus)
-- Idempotent: UPSERT by code, wire module/level/parent_id, prune stale menus.
-- Run: npm run db:seed:iam-menus
-- =============================================================================

-- EPIC-061: menu daun HRIS yang punya endpoint hapus menawarkan "delete" (sama dengan migrasi
-- 20261022090000_iam_hris_delete_masks); hak hapus diberikan lewat Hak Akses HRIS / Super Admin.
-- EPIC-067: Absensi dan Cuti & Izin juga menawarkan "approve" (keputusan HR; Lembur sudah), sama
-- dengan migrasi 20261027090100_iam_hris_approve.
-- EPIC-068: menu daun Saldo Cuti (read/update/import/export), sama dengan migrasi
-- 20261028090000_leave_balance_allocation.
INSERT INTO iam.menus (code, menu_name, route_path, icon, menu_type, order_number, permission_context) VALUES
  ('report', 'Laporan', NULL, 'chart', 'group', 80, '{"actions":["read"]}'::jsonb),
  ('report.sales-vs-target', 'Sales vs Target', '/dashboard/report/sales-vs-target', 'chart', 'sidebar', 10, '{"actions":["read","create","update","delete"]}'::jsonb),
  ('spa.bookings', 'Booking History', '/dashboard/spa/bookings', 'ticket', 'sidebar', 30, '{"actions":["read","create","update"]}'::jsonb),
  ('spa.customer', 'Customer', '/dashboard/spa/customer', 'users', 'sidebar', 25, '{"actions":["read"]}'::jsonb),
  ('spa.rush-hour', 'Rush Hour', '/dashboard/spa/report/rush-hour', 'clock', 'sidebar', 70, '{"actions":["read"]}'::jsonb),
  ('spa.trend', 'Trend', '/dashboard/spa/report/trend', 'chart', 'sidebar', 20, '{"actions":["read"]}'::jsonb),
  ('spa.rating', 'Rating', '/dashboard/spa/report/rating', 'star', 'sidebar', 40, '{"actions":["read"]}'::jsonb),
  ('dashboard', 'Beranda', '/dashboard', 'home', 'sidebar', 0, '{"actions":["read"]}'::jsonb),
  ('ess', 'Area Karyawan', NULL, 'calendar', 'group', 5, '{"actions":["read"]}'::jsonb),
  ('ess.attendance', 'Absensi', '/dashboard/me/absensi', 'calendar', 'sidebar', 10, '{"actions":["read"]}'::jsonb),
  ('ess.leave', 'Izin & Cuti', '/dashboard/me/cuti', 'paper-airplane', 'sidebar', 20, '{"actions":["read"]}'::jsonb),
  ('ess.team-shifts', 'Shift Tim', '/dashboard/me/shift-tim', 'calendar', 'sidebar', 45, '{"actions":["read","update"]}'::jsonb),
  ('ess.store-schedule', 'Jadwal Toko', '/dashboard/me/jadwal-toko', 'calendar-days', 'sidebar', 46, '{"actions":["read","update"]}'::jsonb),
  ('hris', 'HRIS', '/dashboard/hris', 'users', 'group', 10, '{"actions":["read"]}'::jsonb),
  ('items', 'Items', '/dashboard/items', 'cube', 'group', 35, '{"actions":["read"]}'::jsonb),
  ('pos', 'Point of Sales', '/dashboard/pos', 'shopping', 'group', 70, '{"actions":["read"]}'::jsonb),
  ('crm', 'CRM', '/dashboard/crm', 'star', 'group', 80, '{"actions":["read"]}'::jsonb),
  ('settings', 'Settings', NULL, 'settings', 'group', 90, '{"actions":["read"]}'::jsonb),
  ('settings.business', 'Business', '/dashboard/settings/business', 'building', 'sidebar', 10, '{"actions":["read"]}'::jsonb),
  ('settings.integrations', 'Integrasi', '/dashboard/settings/integrations', 'plug', 'sidebar', 11, '{"actions":["read"]}'::jsonb),
  ('settings.appearance', 'Appearance', '/dashboard/settings/appearance', 'palette', 'sidebar', 20, '{"actions":["read"]}'::jsonb),
  ('settings.menus', 'Menu Configuration', '/dashboard/settings/menus', 'sitemap', 'sidebar', 30, '{"actions":["read"]}'::jsonb),
  ('settings.users', 'Manajemen User', '/dashboard/settings/users', 'users', 'sidebar', 35, '{"actions":["read"]}'::jsonb),
  ('settings.roles', 'Role & Permission', '/dashboard/settings/roles', 'identification', 'sidebar', 40, '{"actions":["read"]}'::jsonb),
  ('settings.payment-gateways', 'Payment Gateways', '/dashboard/settings/payment-gateways', 'money', 'sidebar', 50, '{"actions":["read","update"]}'::jsonb),
  ('crm.overview', 'Overview', NULL, 'home', 'group', 10, '{"actions":["read"]}'::jsonb),
  ('hris.master', 'Master Data', NULL, 'database', 'group', 5, '{"actions":["read"]}'::jsonb),
  ('hris.kepegawaian', 'Kepegawaian', NULL, 'users', 'group', 8, '{"actions":["read"]}'::jsonb),
  ('hris.kepegawaian.users', 'Karyawan', '/dashboard/employees', 'users', 'sidebar', 10, '{"actions":["read","delete"]}'::jsonb),
  ('hris.kepegawaian.contracts', 'Kontrak', '/dashboard/hris/contracts', 'file-text', 'sidebar', 20, '{"actions":["read","delete"]}'::jsonb),
  ('hris.kepegawaian.shifts', 'Shift Kerja', '/dashboard/hris/shifts', 'clipboard', 'sidebar', 30, '{"actions":["read","delete"]}'::jsonb),
  ('hris.recruitment', 'Rekrutmen', NULL, 'user-plus', 'group', 10, '{"actions":["read"]}'::jsonb),
  ('items.raw-material', 'Bahan Baku', NULL, 'cube', 'group', 10, '{"actions":["read"]}'::jsonb),
  ('pos.operations', 'Operasional', NULL, 'shopping', 'group', 10, '{"actions":["read"]}'::jsonb),
  ('crm.members', 'Customer Care', NULL, 'users', 'group', 20, '{"actions":["read"]}'::jsonb),
  ('hris.workforce', 'Kehadiran & Cuti', NULL, 'calendar', 'group', 20, '{"actions":["read"]}'::jsonb),
  ('items.product', 'Produk', NULL, 'cube', 'group', 20, '{"actions":["read"]}'::jsonb),
  ('pos.catalog', 'Produk & Stok', NULL, 'cube', 'group', 20, '{"actions":["read"]}'::jsonb),
  ('crm.loyalty', 'Loyalty', NULL, 'star', 'group', 30, '{"actions":["read"]}'::jsonb),
  ('hris.organization', 'Struktur Organisasi', NULL, 'sitemap', 'group', 30, '{"actions":["read"]}'::jsonb),
  ('pos.kitchen', 'Dapur & Cetak', NULL, 'cube', 'group', 30, '{"actions":["read"]}'::jsonb),
  ('hris.compensation', 'Penggajian', NULL, 'dollar-sign', 'group', 40, '{"actions":["read"]}'::jsonb),
  ('pos.loyalty', 'Member', NULL, 'money', 'group', 40, '{"actions":["read"]}'::jsonb),
  ('hris.performance', 'Kinerja', NULL, 'chart', 'group', 50, '{"actions":["read"]}'::jsonb),
  ('pos.reports', 'Laporan', NULL, 'chart', 'group', 50, '{"actions":["read"]}'::jsonb),
  ('hris.insights', 'Laporan', NULL, 'reports', 'group', 60, '{"actions":["read"]}'::jsonb),
  ('crm.loyalty.rewards', 'Rewards', '/dashboard/crm/rewards', 'star', 'sidebar', 10, '{"actions":["read"]}'::jsonb),
  ('crm.members.list', 'Members', '/dashboard/crm/members', 'users', 'sidebar', 10, '{"actions":["read"]}'::jsonb),
  ('crm.overview.dashboard', 'Dashboard', '/dashboard/crm', 'home', 'sidebar', 10, '{"actions":["read"]}'::jsonb),
  ('hris.compensation.payroll', 'Run Penggajian', '/dashboard/hris/payroll', 'dollar-sign', 'sidebar', 10, '{"actions":["read","delete"]}'::jsonb),
  ('hris.insights.analytics', 'Analytics', '/dashboard/hris/analytics', 'chart', 'sidebar', 10, '{"actions":["read"]}'::jsonb),
  ('hris.organization.sections', 'Sections', '/dashboard/hris/sections', 'building', 'sidebar', 10, '{"actions":["read"]}'::jsonb),
  ('hris.performance.kpi-templates', 'KPI Templates', '/dashboard/hris/kpi-templates', 'clipboard', 'sidebar', 10, '{"actions":["read"]}'::jsonb),
  ('hris.performance.dept-tasks', 'Task Departemen', '/dashboard/hris/dept-tasks', 'clipboard-document-check', 'sidebar', 66, '{"actions":["read","update"]}'::jsonb),
  ('hris.performance.kpi-config', 'Konfigurasi KPI Department', '/dashboard/hris/kpi-config', 'clipboard', 'sidebar', 68, '{"actions":["read","update"]}'::jsonb),
  ('hris.recruitment.candidates', 'Kandidat', '/dashboard/hris/candidates', 'user-plus', 'sidebar', 10, '{"actions":["read","delete"]}'::jsonb),
  ('hris.workforce.attendance', 'Absensi', '/dashboard/hris/attendance', 'calendar', 'sidebar', 10, '{"actions":["read","delete","approve"]}'::jsonb),
  ('items.product.master', 'Data Master', NULL, 'database', 'group', 10, '{"actions":["read"]}'::jsonb),
  ('items.raw-material.master', 'Data Master', NULL, 'database', 'group', 10, '{"actions":["read"]}'::jsonb),
  ('pos.catalog.products', 'Produk', '/dashboard/pos/products', 'cube', 'sidebar', 10, '{"actions":["read"]}'::jsonb),
  ('pos.kitchen.kds', 'KDS', '/dashboard/pos/kds', 'cube', 'sidebar', 10, '{"actions":["read"]}'::jsonb),
  ('pos.kitchen.queue-board', 'TV Antrian', '/pos/queue', 'video', 'sidebar', 15, '{"actions":["read"]}'::jsonb),
  ('pos.loyalty.topup', 'Topup', '/dashboard/pos/topup', 'money', 'sidebar', 10, '{"actions":["read"]}'::jsonb),
  ('pos.loyalty.settings', 'ARK & XP', '/dashboard/pos/loyalty-settings', 'settings', 'sidebar', 20, '{"actions":["read","update"]}'::jsonb),
  ('pos.operations.cashier', 'Kasir', '/dashboard/pos/tablet', 'shopping', 'sidebar', 10, '{"actions":["read"]}'::jsonb),
  ('pos.cashier.central', 'Kasir Pusat', NULL, 'shopping', 'sidebar', 12, '{"actions":["read"]}'::jsonb),
  ('pos.operations.restaurant', 'Restaurant', '/dashboard/pos/restaurant-tablet', 'map', 'sidebar', 15, '{"actions":["read"]}'::jsonb),
  ('pos.reports.dashboard', 'Dashboard', '/dashboard/pos', 'home', 'sidebar', 10, '{"actions":["read"]}'::jsonb),
  ('hris.master.departments', 'Departemen', '/dashboard/master/departments', 'building', 'sidebar', 10, '{"actions":["read","delete"]}'::jsonb),
  ('crm.loyalty.avatars', 'Avatars', '/dashboard/crm/avatars', 'star', 'sidebar', 20, '{"actions":["read"]}'::jsonb),
  ('hris.compensation.salary', 'Salary', '/dashboard/hris/salary', 'money', 'sidebar', 20, '{"actions":["read","delete"]}'::jsonb),
  ('hris.insights.reports', 'Laporan HRIS', '/dashboard/hris/reports', 'reports', 'sidebar', 20, '{"actions":["read"]}'::jsonb),
  ('hris.organization.org-chart', 'Struktur Org', '/dashboard/hris/org-chart', 'sitemap', 'sidebar', 20, '{"actions":["read"]}'::jsonb),
  ('hris.performance.logbook', 'Logbook', '/dashboard/hris/logbook', 'clipboard', 'sidebar', 20, '{"actions":["read"]}'::jsonb),
  ('hris.recruitment.pipeline', 'Pipeline', '/dashboard/hris/pipeline', 'clipboard', 'sidebar', 20, '{"actions":["read"]}'::jsonb),
  ('hris.recruitment.psikotes', 'Psikotes', '/dashboard/hris/psikotes', 'clipboard', 'sidebar', 25, '{"actions":["read","delete"]}'::jsonb),
  ('hris.workforce.leaves', 'Cuti & Izin', '/dashboard/hris/leaves', 'file-text', 'sidebar', 20, '{"actions":["read","delete","approve"]}'::jsonb),
  ('items.product.inventory', 'Persediaan', NULL, 'circle-stack', 'group', 20, '{"actions":["read"]}'::jsonb),
  ('items.raw-material.inventory', 'Persediaan', NULL, 'circle-stack', 'group', 20, '{"actions":["read"]}'::jsonb),
  ('pos.catalog.stock-alerts', 'Stok Alert', '/dashboard/pos/stock-alerts', 'chart-bar', 'sidebar', 20, '{"actions":["read"]}'::jsonb),
  ('pos.kitchen.print-queue', 'Print Queue', '/dashboard/pos/print-queue', 'document-text', 'sidebar', 20, '{"actions":["read"]}'::jsonb),
  ('pos.operations.tables', 'Tables', '/dashboard/pos/tables', 'building', 'sidebar', 25, '{"actions":["read","create","update","delete"]}'::jsonb),
  ('pos.reports.profit', 'Profit', '/dashboard/pos/reports/profit', 'chart', 'sidebar', 20, '{"actions":["read"]}'::jsonb),
  ('pos.reports.revenue-composition', 'Komposisi Pendapatan', '/dashboard/pos/reports/revenue-composition', 'chart-pie', 'sidebar', 21, '{"actions":["read"]}'::jsonb),
  ('pos.reports.transactions', 'Transaksi', '/dashboard/pos/reports/transactions', 'clipboard', 'sidebar', 25, '{"actions":["read"]}'::jsonb),
  ('pos.reports.rush-hour', 'Rush Hour', '/dashboard/pos/reports/rush-hour', 'clock', 'sidebar', 26, '{"actions":["read"]}'::jsonb),
  ('pos.reports.product-sales', 'Penjualan Produk', '/dashboard/pos/reports/product-sales', 'cube', 'sidebar', 28, '{"actions":["read"]}'::jsonb),
  ('pos.reports.voids', 'Laporan Void', '/dashboard/pos/reports/voids', 'document-text', 'sidebar', 27, '{"actions":["read"]}'::jsonb),
  ('pos.reports.payment-methods', 'Jenis Pembayaran', '/dashboard/pos/reports/payment-methods', 'credit-card', 'sidebar', 29, '{"actions":["read"]}'::jsonb),
  ('pos.reports.closing', 'Tutup Kasir', '/dashboard/pos/reports/closing', 'document-text', 'sidebar', 30, '{"actions":["read"]}'::jsonb),
  ('hris.master.positions', 'Jabatan', '/dashboard/master/positions', 'briefcase', 'sidebar', 20, '{"actions":["read","delete"]}'::jsonb),
  ('hris.performance.logbook-list', 'Logbook List', '/dashboard/hris/logbook-list', 'clipboard', 'sidebar', 30, '{"actions":["read"]}'::jsonb),
  ('hris.recruitment.talent-pool', 'Talent Pool', '/dashboard/hris/talent-pool', 'star', 'sidebar', 30, '{"actions":["read"]}'::jsonb),
  ('hris.workforce.leave-balances', 'Saldo Cuti', '/dashboard/hris/leave-balances', 'clipboard', 'sidebar', 25, '{"actions":["read","update","import","export"]}'::jsonb),
  ('hris.workforce.schedules', 'Schedules', '/dashboard/hris/schedules', 'calendar', 'sidebar', 30, '{"actions":["read"]}'::jsonb),
  ('items.product.purchasing', 'Pembelian', NULL, 'shopping', 'group', 30, '{"actions":["read"]}'::jsonb),
  ('items.raw-material.purchasing', 'Pembelian', NULL, 'shopping', 'group', 30, '{"actions":["read"]}'::jsonb),
  ('pos.kitchen.printer-settings', 'Printer', '/dashboard/pos/printer-settings', 'settings', 'sidebar', 30, '{"actions":["read"]}'::jsonb),
  ('pos.operations.orders', 'Orders', '/dashboard/pos/orders', 'clipboard', 'sidebar', 30, '{"actions":["read"]}'::jsonb),
  ('pos.operations.gofood', 'GoFood', '/dashboard/pos/gofood', 'truck', 'sidebar', 32, '{"actions":["read","update"]}'::jsonb),
  ('pos.operations.member-bills', 'Tagihan', '/dashboard/pos/member-bills', 'money', 'sidebar', 35, '{"actions":["read","create"]}'::jsonb),
  ('hris.master.employment-statuses', 'Status Karyawan', '/dashboard/master/employment-statuses', 'identification', 'sidebar', 30, '{"actions":["read","delete"]}'::jsonb),
  ('hris.performance.review', 'Performance Review', '/dashboard/hris/performance', 'chart', 'sidebar', 40, '{"actions":["read"]}'::jsonb),
  ('hris.recruitment.job-portal', 'Job Portal', '/dashboard/hris/job-portal', 'briefcase', 'sidebar', 40, '{"actions":["read","delete"]}'::jsonb),
  ('items.product.approval', 'Persetujuan', NULL, 'check-circle', 'group', 40, '{"actions":["read"]}'::jsonb),
  ('items.raw-material.approval', 'Persetujuan', NULL, 'check-circle', 'group', 40, '{"actions":["read"]}'::jsonb),
  ('pos.operations.reservation', 'Reservasi', '/dashboard/pos/reservation', 'calendar', 'sidebar', 40, '{"actions":["read"]}'::jsonb),
  ('items.product.production', 'Produksi', NULL, 'cube', 'group', 50, '{"actions":["read"]}'::jsonb),
  ('items.raw-material.production', 'Produksi', NULL, 'cube', 'group', 50, '{"actions":["read"]}'::jsonb),
  ('items.reports', 'Reports', '/dashboard/purchasing/reports', 'reports', 'group', 30, '{"actions":["read"]}'::jsonb),
  ('items.reports.stock-card', 'Stock Card', '/dashboard/purchasing/reports/stock-card', 'clipboard', 'sidebar', 10, '{"actions":["read"]}'::jsonb),
  ('items.reports.inventory-valuation', 'Inventory Valuation', '/dashboard/purchasing/reports/inventory-valuation', 'database', 'sidebar', 20, '{"actions":["read"]}'::jsonb),
  ('items.reports.po-summary', 'PO Summary', '/dashboard/purchasing/reports/po-summary', 'shopping', 'sidebar', 30, '{"actions":["read"]}'::jsonb),
  ('items.reports.po-detail', 'PO Detail', '/dashboard/purchasing/reports/po-detail', 'document-text', 'sidebar', 40, '{"actions":["read"]}'::jsonb),
  ('items.reports.supplier-performance', 'Supplier Performance', '/dashboard/purchasing/reports/supplier-performance', 'building', 'sidebar', 50, '{"actions":["read"]}'::jsonb),
  ('items.reports.production-in-house', 'Produksi Internal', '/dashboard/purchasing/reports/production-in-house', 'cube', 'sidebar', 60, '{"actions":["read"]}'::jsonb),
  ('items.product.approval.pr', 'Persetujuan PR', '/dashboard/product/approval/pr', 'file-text', 'sidebar', 10, '{"actions":["read","update"]}'::jsonb),
  ('items.product.inventory.stock', 'Stok', '/dashboard/product/inventory/stock', 'circle-stack', 'sidebar', 10, '{"actions":["read"]}'::jsonb),
  ('items.product.master.units', 'Satuan', '/dashboard/product/units', 'database', 'sidebar', 10, '{"actions":["read","create","update","delete"]}'::jsonb),
  ('items.product.production.bom', 'Resep (BOM)', '/dashboard/product/production/recipes', 'cube', 'sidebar', 10, '{"actions":["read","create","update","delete"]}'::jsonb),
  ('items.product.purchasing.vendor', 'Vendor', '/dashboard/product/purchasing/vendor', 'building', 'sidebar', 10, '{"actions":["read","create","update","delete"]}'::jsonb),
  ('items.raw-material.approval.pr', 'Persetujuan PR', '/dashboard/raw-material/approval/pr', 'file-text', 'sidebar', 10, '{"actions":["read","update"]}'::jsonb),
  ('items.raw-material.inventory.stock', 'Stok', '/dashboard/raw-material/inventory/stock', 'circle-stack', 'sidebar', 10, '{"actions":["read"]}'::jsonb),
  ('items.raw-material.master.units', 'Satuan', '/dashboard/raw-material/units', 'database', 'sidebar', 10, '{"actions":["read","create","update","delete"]}'::jsonb),
  ('items.raw-material.production.bom', 'Resep (BOM)', '/dashboard/raw-material/production/recipes', 'cube', 'sidebar', 10, '{"actions":["read","create","update","delete"]}'::jsonb),
  ('items.raw-material.purchasing.suppliers', 'Supplier', '/dashboard/raw-material/purchasing/suppliers', 'building', 'sidebar', 10, '{"actions":["read","create","update","delete"]}'::jsonb),
  ('items.product.approval.po', 'Persetujuan PO', '/dashboard/product/approval/po', 'clipboard-document-check', 'sidebar', 20, '{"actions":["read","update"]}'::jsonb),
  ('items.product.inventory.opname', 'Stok Opname', '/dashboard/product/inventory/opname', 'clipboard', 'sidebar', 20, '{"actions":["read","create","update"]}'::jsonb),
  ('items.product.master.categories', 'Kategori', '/dashboard/product/categories', 'clipboard', 'sidebar', 20, '{"actions":["read","create","update","delete"]}'::jsonb),
  ('items.product.production.hub', 'Produksi Internal', '/dashboard/product/production', 'cube', 'sidebar', 20, '{"actions":["read","create","update"]}'::jsonb),
  ('items.raw-material.approval.po', 'Persetujuan PO', '/dashboard/raw-material/approval/po', 'clipboard-document-check', 'sidebar', 20, '{"actions":["read","update"]}'::jsonb),
  ('items.raw-material.inventory.opname', 'Stok Opname', '/dashboard/raw-material/inventory/opname', 'clipboard', 'sidebar', 20, '{"actions":["read","create","update"]}'::jsonb),
  ('items.raw-material.master.categories', 'Kategori', '/dashboard/raw-material/categories', 'clipboard', 'sidebar', 20, '{"actions":["read","create","update","delete"]}'::jsonb),
  ('items.raw-material.production.hub', 'Produksi Internal', '/dashboard/raw-material/production', 'cube', 'sidebar', 20, '{"actions":["read","create","update"]}'::jsonb),
  ('items.product.inventory.adjustment', 'Penyesuaian Stok', '/dashboard/product/inventory/adjustment', 'clipboard-document-check', 'sidebar', 30, '{"actions":["read","create","update"]}'::jsonb),
  ('items.product.master.products', 'Produk', '/dashboard/product/products', 'cube', 'sidebar', 30, '{"actions":["read","create","update","delete"]}'::jsonb),
  ('items.product.purchasing.pr', 'Purchase Request', '/dashboard/product/purchasing/pr', 'file-text', 'sidebar', 30, '{"actions":["read","create","update","delete"]}'::jsonb),
  ('items.raw-material.inventory.adjustment', 'Penyesuaian Stok', '/dashboard/raw-material/inventory/adjustment', 'clipboard-document-check', 'sidebar', 30, '{"actions":["read","create","update"]}'::jsonb),
  ('items.raw-material.purchasing.pr', 'Purchase Request', '/dashboard/raw-material/purchasing/pr', 'file-text', 'sidebar', 30, '{"actions":["read","create","update","delete"]}'::jsonb),
  ('items.product.inventory.transfer', 'Transfer Stok', '/dashboard/product/inventory/transfer', 'paper-airplane', 'sidebar', 40, '{"actions":["read","create","update"]}'::jsonb),
  ('items.product.purchasing.po', 'Purchase Order', '/dashboard/product/purchasing/po', 'clipboard-document-check', 'sidebar', 40, '{"actions":["read","create","update","delete"]}'::jsonb),
  ('items.raw-material.inventory.transfer', 'Transfer Stok', '/dashboard/raw-material/inventory/transfers', 'paper-airplane', 'sidebar', 40, '{"actions":["read","create","update"]}'::jsonb),
  ('items.raw-material.master.materials', 'Bahan Baku', '/dashboard/raw-material/materials', 'cube', 'sidebar', 40, '{"actions":["read","create","update","delete"]}'::jsonb),
  ('items.raw-material.purchasing.po', 'Purchase Order', '/dashboard/raw-material/purchasing/po', 'clipboard-document-check', 'sidebar', 40, '{"actions":["read","create","update","delete"]}'::jsonb),
  ('items.product.purchasing.delivery', 'Lacak Pengiriman', '/dashboard/product/purchasing/delivery', 'truck', 'sidebar', 50, '{"actions":["read","create","update"]}'::jsonb),
  ('items.raw-material.purchasing.delivery', 'Lacak Pengiriman', '/dashboard/raw-material/purchasing/delivery', 'truck', 'sidebar', 50, '{"actions":["read","create","update"]}'::jsonb),
  ('items.product.purchasing.grn', 'Penerimaan (GRN)', '/dashboard/product/purchasing/receive', 'arrow-down-on-square', 'sidebar', 60, '{"actions":["read","create","update"]}'::jsonb),
  ('items.raw-material.purchasing.grn', 'Penerimaan (GRN)', '/dashboard/raw-material/purchasing/grn', 'arrow-down-on-square', 'sidebar', 60, '{"actions":["read","create","update"]}'::jsonb),
  ('items.product.purchasing.returns', 'Retur', '/dashboard/product/purchasing/returns', 'truck', 'sidebar', 70, '{"actions":["read","create","update"]}'::jsonb),
  ('items.raw-material.purchasing.returns', 'Retur', '/dashboard/raw-material/purchasing/returns', 'truck', 'sidebar', 70, '{"actions":["read","create","update"]}'::jsonb),
  ('items.product.purchasing.invoice', 'Invoice', '/dashboard/product/purchasing/invoice', 'money', 'sidebar', 80, '{"actions":["read","create","update"]}'::jsonb),
  ('items.raw-material.purchasing.settings', 'Pengaturan Pembelian', '/dashboard/raw-material/purchasing/settings', 'settings', 'sidebar', 90, '{"actions":["read","update"]}'::jsonb),
  ('items.product.purchasing.settings', 'Pengaturan Pembelian', '/dashboard/product/purchasing/settings', 'settings', 'sidebar', 90, '{"actions":["read","update"]}'::jsonb),
  ('accounting', 'Accounting', NULL, 'dollar-sign', 'group', 55, '{"actions":["read"]}'::jsonb),
  ('accounting.dashboard', 'Dashboard', NULL, 'home', 'group', 10, '{"actions":["read"]}'::jsonb),
  ('accounting.dashboard.overview', 'Overview', '/dashboard/accounting/reports', 'home', 'sidebar', 10, '{"actions":["read","export"]}'::jsonb),
  ('accounting.master', 'Master Data', NULL, 'database', 'group', 20, '{"actions":["read"]}'::jsonb),
  ('accounting.master.chart-of-accounts', 'Chart of Accounts', '/dashboard/accounting/chart-of-accounts', 'sitemap', 'sidebar', 10, '{"actions":["read","create","update","delete","import","export"]}'::jsonb),
  ('accounting.master.account-types', 'Account Types', '/dashboard/accounting/account-types', 'database', 'sidebar', 20, '{"actions":["read","create","update","delete"]}'::jsonb),
  ('accounting.master.journal-mappings', 'Journal Mapping', '/dashboard/accounting/journal-mappings', 'clipboard', 'sidebar', 30, '{"actions":["read","create","update","delete"]}'::jsonb),
  ('accounting.ledger', 'General Ledger', NULL, 'document-text', 'group', 30, '{"actions":["read"]}'::jsonb),
  ('accounting.ledger.journal-entries', 'Journal Entries', '/dashboard/accounting/journal-entries', 'clipboard', 'sidebar', 10, '{"actions":["read","create","update","delete","approve"]}'::jsonb),
  ('accounting.ledger.journal-history', 'Journal History', '/dashboard/accounting/journal-history', 'document-magnifying-glass', 'sidebar', 20, '{"actions":["read","export"]}'::jsonb),
  ('accounting.ledger.general-ledger', 'General Ledger', '/dashboard/accounting/reports/general-ledger', 'document-text', 'sidebar', 30, '{"actions":["read","export"]}'::jsonb),
  ('accounting.ledger.subsidiary', 'Subsidiary Ledger', '/dashboard/accounting/ledger/subsidiary', 'clipboard', 'sidebar', 40, '{"actions":["read"]}'::jsonb),
  ('accounting.ledger.trial-balance', 'Trial Balance', '/dashboard/accounting/reports/trial-balance', 'clipboard-document-check', 'sidebar', 50, '{"actions":["read","export"]}'::jsonb),
  ('accounting.cash-bank', 'Cash & Bank', NULL, 'banknotes', 'group', 40, '{"actions":["read"]}'::jsonb),
  ('accounting.cash-bank.accounts', 'Cash & Bank Accounts', '/dashboard/accounting/cash-bank', 'banknotes', 'sidebar', 10, '{"actions":["read","export"]}'::jsonb),
  ('accounting.cash-bank.cash-in', 'Cash In', '/dashboard/accounting/cash-bank/cash-in', 'arrow-down-on-square', 'sidebar', 20, '{"actions":["read"]}'::jsonb),
  ('accounting.cash-bank.cash-out', 'Cash Out', '/dashboard/accounting/cash-bank/cash-out', 'banknotes', 'sidebar', 30, '{"actions":["read"]}'::jsonb),
  ('accounting.cash-bank.transfer', 'Bank Transfer', '/dashboard/accounting/cash-bank/transfer', 'truck', 'sidebar', 40, '{"actions":["read"]}'::jsonb),
  ('accounting.cash-bank.reconciliation', 'Bank Reconciliation', '/dashboard/accounting/cash-bank/reconciliation', 'check-circle', 'sidebar', 50, '{"actions":["read"]}'::jsonb),
  ('accounting.receivable', 'Accounts Receivable', NULL, 'money', 'group', 50, '{"actions":["read"]}'::jsonb),
  ('accounting.receivable.list', 'Receivable', '/dashboard/accounting/receivable', 'money', 'sidebar', 10, '{"actions":["read"]}'::jsonb),
  ('accounting.receivable.invoices-b2b', 'Invoice B2B', '/dashboard/accounting/receivable/invoices-b2b', 'file-text', 'sidebar', 20, '{"actions":["read","create","update","delete"]}'::jsonb),
  ('accounting.receivable.invoices', 'Invoice', '/dashboard/accounting/receivable/invoices', 'file-text', 'sidebar', 30, '{"actions":["read"]}'::jsonb),
  ('accounting.receivable.receipts', 'Receipt', '/dashboard/accounting/receivable/receipts', 'banknotes', 'sidebar', 40, '{"actions":["read"]}'::jsonb),
  ('accounting.receivable.aging', 'Aging', '/dashboard/accounting/receivable/aging', 'chart-bar', 'sidebar', 50, '{"actions":["read"]}'::jsonb),
  ('accounting.accounts-payable', 'Accounts Payable', NULL, 'banknotes', 'group', 60, '{"actions":["read"]}'::jsonb),
  ('accounting.accounts-payable.payable', 'Payable', '/dashboard/accounting/accounts-payable', 'banknotes', 'sidebar', 10, '{"actions":["read","create","update"]}'::jsonb),
  ('accounting.accounts-payable.invoices', 'Invoice', '/dashboard/accounting/accounts-payable/invoices', 'file-text', 'sidebar', 20, '{"actions":["read"]}'::jsonb),
  ('accounting.accounts-payable.payments', 'Payment', '/dashboard/accounting/accounts-payable/payments', 'money', 'sidebar', 30, '{"actions":["read"]}'::jsonb),
  ('accounting.accounts-payable.aging', 'Aging', '/dashboard/accounting/accounts-payable/aging', 'chart-bar', 'sidebar', 40, '{"actions":["read"]}'::jsonb),
  ('accounting.period', 'Period & Closing', NULL, 'calendar', 'group', 70, '{"actions":["read"]}'::jsonb),
  ('accounting.period.fiscal-years', 'Fiscal Years', '/dashboard/accounting/fiscal-years', 'calendar', 'sidebar', 10, '{"actions":["read","create","update","delete"]}'::jsonb),
  ('accounting.period.beginning-balance', 'Beginning Balance', '/dashboard/accounting/beginning-balance', 'banknotes', 'sidebar', 20, '{"actions":["read","create","update","delete","approve"]}'::jsonb),
  ('accounting.period.accounting-period', 'Accounting Period', '/dashboard/accounting/period/accounting-period', 'calendar', 'sidebar', 30, '{"actions":["read"]}'::jsonb),
  ('accounting.period.closing', 'Period Closing', '/dashboard/accounting/period/closing', 'check-circle', 'sidebar', 40, '{"actions":["read"]}'::jsonb),
  ('accounting.reports', 'Reports', NULL, 'chart-bar', 'group', 80, '{"actions":["read"]}'::jsonb),
  ('accounting.reports.balance-sheet', 'Balance Sheet', '/dashboard/accounting/reports/balance-sheet', 'chart-bar', 'sidebar', 10, '{"actions":["read","export"]}'::jsonb),
  ('accounting.reports.income-statement', 'Income Statement', '/dashboard/accounting/reports/income-statement', 'chart-pie', 'sidebar', 20, '{"actions":["read","export"]}'::jsonb),
  ('accounting.reports.cash-flow', 'Cash Flow', '/dashboard/accounting/reports/cash-flow', 'banknotes', 'sidebar', 30, '{"actions":["read","export"]}'::jsonb),
  ('accounting.reports.equity', 'Changes in Equity', '/dashboard/accounting/reports/changes-in-equity', 'chart', 'sidebar', 40, '{"actions":["read"]}'::jsonb),
  ('accounting.reports.budget', 'Budget vs Actual', '/dashboard/accounting/reports/budget-vs-actual', 'chart-bar', 'sidebar', 50, '{"actions":["read"]}'::jsonb)
ON CONFLICT (code) DO UPDATE SET
  menu_name            = EXCLUDED.menu_name,
  route_path           = EXCLUDED.route_path,
  icon                 = EXCLUDED.icon,
  menu_type            = EXCLUDED.menu_type,
  order_number         = EXCLUDED.order_number,
  permission_context   = EXCLUDED.permission_context,
  is_active            = true,
  is_visible           = true,
  deleted_at           = NULL,
  updated_at           = now();

-- Capability only: seeder upsert forces is_visible = true for every code.
UPDATE iam.menus SET is_visible = false WHERE code = 'pos.cashier.central';

UPDATE iam.menus
SET module = split_part(code, '.', 1),
    level  = (length(code) - length(replace(code, '.', ''))) + 1
WHERE deleted_at IS NULL;

UPDATE iam.menus child
SET parent_id = parent.id
FROM iam.menus parent
WHERE child.code LIKE '%.%'
  AND parent.code = left(child.code, length(child.code) - position('.' in reverse(child.code)))
  AND child.deleted_at IS NULL
  AND parent.deleted_at IS NULL;

UPDATE iam.menus SET parent_id = NULL WHERE code NOT LIKE '%.%' AND deleted_at IS NULL;

-- Keep report permission codes while nesting their sidebar group under Spa.
UPDATE iam.menus child SET parent_id = parent.id, module = 'spa', level = 2
FROM iam.menus parent WHERE child.code = 'report' AND parent.code = 'spa';
UPDATE iam.menus child SET parent_id = parent.id, module = 'spa', level = 3
FROM iam.menus parent
WHERE child.code IN ('report.sales-vs-target', 'spa.trend', 'spa.bookings', 'spa.rating') AND parent.code = 'report';


-- Soft-delete menus yang tidak ada di canonical tree
UPDATE iam.menus
SET is_active = false, is_visible = false, deleted_at = now(), updated_at = now()
WHERE deleted_at IS NULL
  AND code NOT IN ('dashboard', 'ess', 'ess.attendance', 'ess.leave', 'hris.kepegawaian.shifts', 'hris', 'items', 'pos', 'crm', 'crm.overview', 'hris.recruitment', 'items.raw-material', 'pos.operations', 'crm.members', 'hris.workforce', 'items.product', 'pos.catalog', 'crm.loyalty', 'hris.organization', 'pos.kitchen', 'hris.compensation', 'pos.loyalty', 'hris.performance', 'pos.reports', 'hris.insights', 'items.reports', 'crm.loyalty.rewards', 'crm.members.list', 'crm.overview.dashboard', 'hris.compensation.payroll', 'hris.insights.analytics', 'hris.organization.sections', 'hris.performance.kpi-templates', 'hris.recruitment.candidates', 'hris.workforce.attendance', 'items.product.master', 'items.raw-material.master', 'pos.catalog.products', 'pos.kitchen.kds', 'pos.loyalty.topup', 'pos.loyalty.settings', 'pos.operations.cashier', 'pos.reports.dashboard', 'items.reports.stock-card', 'crm.loyalty.avatars', 'hris.compensation.salary', 'hris.insights.reports', 'hris.organization.org-chart', 'hris.performance.logbook', 'hris.recruitment.pipeline', 'hris.workforce.leaves', 'items.product.inventory', 'items.raw-material.inventory', 'pos.catalog.stock-alerts', 'pos.kitchen.print-queue', 'pos.operations.restaurant', 'pos.reports.profit', 'pos.reports.transactions', 'pos.reports.rush-hour', 'pos.reports.product-sales', 'pos.reports.voids', 'pos.reports.closing', 'items.reports.inventory-valuation', 'hris.performance.logbook-list', 'hris.recruitment.talent-pool', 'hris.workforce.schedules', 'items.product.purchasing', 'items.raw-material.purchasing', 'pos.kitchen.printer-settings', 'pos.operations.orders', 'items.reports.po-summary', 'hris.performance.review', 'hris.recruitment.job-portal', 'items.product.approval', 'items.raw-material.approval', 'pos.operations.reservation', 'items.reports.po-detail', 'items.product.production', 'items.raw-material.production', 'items.reports.supplier-performance', 'items.product.approval.pr', 'items.product.inventory.stock', 'items.product.master.units', 'items.product.production.bom', 'items.product.purchasing.vendor', 'items.raw-material.approval.pr', 'items.raw-material.inventory.stock', 'items.raw-material.master.units', 'items.raw-material.production.bom', 'items.raw-material.purchasing.suppliers', 'items.product.approval.po', 'items.product.inventory.opname', 'items.product.master.categories', 'items.product.production.hub', 'items.raw-material.approval.po', 'items.raw-material.inventory.opname', 'items.raw-material.master.categories', 'items.raw-material.production.hub', 'items.product.inventory.adjustment', 'items.product.master.products', 'items.product.purchasing.pr', 'items.raw-material.inventory.adjustment', 'items.raw-material.purchasing.pr', 'items.product.inventory.transfer', 'items.product.purchasing.po', 'items.raw-material.inventory.transfer', 'items.raw-material.master.materials', 'items.raw-material.purchasing.po', 'items.product.purchasing.delivery', 'items.raw-material.purchasing.delivery', 'items.product.purchasing.grn', 'items.raw-material.purchasing.grn', 'items.product.purchasing.returns', 'items.raw-material.purchasing.returns', 'items.product.purchasing.invoice', 'accounting.accounts-payable', 'hris.master', 'hris.kepegawaian', 'hris.kepegawaian.users', 'hris.kepegawaian.contracts', 'hris.master.departments', 'hris.master.positions', 'hris.master.employment-statuses', 'settings', 'settings.business', 'settings.integrations', 'settings.appearance', 'settings.menus', 'settings.roles', 'pos.operations.tables', 'hris.recruitment.psikotes', 'settings.users',
    -- Epic 005-012 (ditambahkan 19 Jul 2026 — insiden retire menyapu menu baru;
    -- SETIAP migrasi menu baru WAJIB menambahkan code-nya ke daftar ini):
    'ess.home', 'ess.overtime', 'ess.loans', 'ess.payroll', 'ess.kpi',
    'ess.announcements', 'ess.team-shifts', 'ess.store-schedule', 'ess.business-trip', 'ess.team-approvals',
    -- Spa / Paddy Experience (EPIC-052, 8 Okt 2026)
    -- Payroll multi-toko (15 Okt 2026): Penempatan Karyawan + Biaya Gaji per Toko
    'hris.master.placements', 'hris.compensation.store-costs',
    'ess.spa', 'spa', 'spa.bookings', 'spa.book-order', 'spa.customer', 'spa.treatments', 'spa.categories', 'spa.therapists',
    'report', 'report.sales-vs-target',
    'spa.commissions', 'spa.outlets', 'spa.rush-hour', 'spa.trend', 'spa.rating',
    'hris.kepegawaian.overtime', 'hris.kepegawaian.loans',
    -- EPIC-068 (26 Okt 2026): Saldo Cuti
    'hris.workforce.leave-balances',
    'hris.kepegawaian.announcements',
    'hris.performance.kpi-scorecard', 'hris.performance.dept-tasks', 'hris.performance.kpi-config',
    'hris.recruitment.live-monitoring',
    'crm.settings', 'crm.settings.config', 'crm.reports', 'crm.reports.overview',
    'crm.members.inbox',
    'settings.wa_gateway', 'settings.payment-gateways',
    'crm.members.reviews',
    -- Modul Ticketing / Dataroom / Resort / Shop / Promo(CRM) / Items General / dll
    -- (ditambahkan 12 Sep 2026 — seeder ini sempat menyapu 52 menu di deploy BCD
    -- karena kode-kodenya belum ada di daftar canonical):
    'crm.badges', 'crm.campaigns', 'crm.promo', 'crm.wallpapers',
    'dataroom', 'ess.requests', 'hris.kepegawaian.holidays', 'items.general',
    'items.general.inventory', 'items.general.inventory.adjustment', 'items.general.inventory.stock', 'items.general.inventory.usage',
    'items.general.master', 'items.general.master.categories', 'items.general.master.items', 'items.general.purchasing',
    'items.general.purchasing.approval-po', 'items.general.purchasing.approval-pr', 'items.general.purchasing.invoice', 'items.general.purchasing.po',
    'items.general.purchasing.pr', 'items.general.purchasing.receive', 'pos.loyalty.payment-methods', 'pos.loyalty.unlink-card',
    'pos.operations.customer-display', 'pos.settings', 'pos.settings.supervisors', 'promo.campaigns',
    'promo.offers.bundling', 'promo.offers.bxgy', 'promo.offers.volume', 'resort',
    'resort.front-office', 'resort.reservations', 'resort.rooms', 'settings.billing',
    'settings.shipping', 'settings.wa_notifications', 'shop', 'shop.marketplace',
    'shop.orders', 'ticketing', 'ticketing.booking', 'ticketing.channel-manager',
    'ticketing.gate', 'ticketing.gate_pass', 'ticketing.loket', 'ticketing.mapping',
    'ticketing.passes', 'ticketing.reports', 'ticketing.settings', 'ticketing.tickets',
    -- EPIC-050 Fase 1 (13 Sep 2026): CRM Advance — Accounts, Contacts, grup Marketing
    'sales-funnel.accounts', 'sales-funnel.contacts', 'crm.marketing',
    -- EPIC-050 Fase 2 (13 Sep 2026): scoring, workflow, approval
    'crm.settings.scoring', 'crm.settings.workflows', 'crm.settings.approvals', 'sales-funnel.approvals',
    -- EPIC-050 Fase 3 (13 Sep 2026): forecast & custom fields
    'sales-funnel.forecast', 'crm.settings.custom-fields',
    'crm.reports.builder', 'crm.reports.dashboards', 'crm.reports.schedules',
    'crm.marketing.segments', 'crm.marketing.forms', 'crm.marketing.attribution',
    -- EPIC-022 Sales Funneling (21 Jul 2026):
    'sales-funnel', 'sales-funnel.leads',
    'sales-funnel.pipeline', 'sales-funnel.settings',
    'sales-funnel.followups', 'sales-funnel.reports',
    -- POS reports Transaksi + Penjualan Produk (23 Jul 2026 — gap whitelist):
    'pos.reports.transactions', 'pos.reports.product-sales',
    'pos.reports.rush-hour', 'pos.reports.voids',
    'pos.reports.payment-methods',
    -- POS report Komposisi Pendapatan (2 Sep 2026):
    'pos.reports.revenue-composition',
    -- Items Reports (moved from purchasing.reports, 27 Jul 2026):
    'items.reports', 'items.reports.stock-card', 'items.reports.inventory-valuation',
    'items.reports.po-summary', 'items.reports.po-detail', 'items.reports.supplier-performance',
    'items.reports.production-in-house',
    -- Accounting tree (8 Aug 2026 — restructure + coming soon leaves):
    'accounting',
    'accounting.dashboard', 'accounting.dashboard.overview',
    'accounting.master', 'accounting.master.chart-of-accounts', 'accounting.master.account-types',
    'accounting.master.journal-mappings',
    'accounting.ledger', 'accounting.ledger.journal-entries', 'accounting.ledger.journal-history',
    'accounting.ledger.general-ledger', 'accounting.ledger.subsidiary', 'accounting.ledger.trial-balance',
    'accounting.cash-bank', 'accounting.cash-bank.accounts', 'accounting.cash-bank.cash-in',
    'accounting.cash-bank.cash-out', 'accounting.cash-bank.transfer', 'accounting.cash-bank.reconciliation',
    'accounting.receivable', 'accounting.receivable.list',
    'accounting.receivable.invoices-b2b', 'accounting.receivable.invoices',
    'accounting.receivable.receipts', 'accounting.receivable.aging',
    'accounting.accounts-payable', 'accounting.accounts-payable.payable',
    'accounting.accounts-payable.invoices', 'accounting.accounts-payable.payments',
    'accounting.accounts-payable.aging',
    'accounting.period', 'accounting.period.beginning-balance', 'accounting.period.fiscal-years',
    'accounting.period.accounting-period', 'accounting.period.closing',
    'accounting.reports', 'accounting.reports.balance-sheet', 'accounting.reports.income-statement',
    'accounting.reports.cash-flow', 'accounting.reports.equity', 'accounting.reports.budget',
    -- POS TV Antrian (9 Aug 2026):
    'pos.cashier.central',
    'pos.kitchen.queue-board',
    -- GoFood / GoBiz (EPIC-049, 12 Sep 2026):
    'pos.operations.gofood',
    -- Tagihan Member (1 Okt 2026):
    'pos.operations.member-bills',
    -- Pengaturan Pembelian / aturan PO wajib dari PR (14 Okt 2026):
    'items.raw-material.purchasing.settings', 'items.product.purchasing.settings',
    -- HRIS Jadwal Kerja (migrasi 20261019090300) dan Hak Akses HRIS (EPIC-061, migrasi 20261022090200):
    'hris.kepegawaian.schedules', 'hris.access');

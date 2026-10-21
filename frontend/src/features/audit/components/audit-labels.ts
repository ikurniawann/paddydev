export const ACTION_LABELS: Record<string, string> = {
  "stock.adjust": "Penyesuaian stok",
  "stock.opname_complete": "Opname selesai",
  "stock.scrap": "Scrap / write-off",
  "stock.transfer": "Transfer stok",
  "po.approve": "PO disetujui",
  "po.cancel": "PO dibatalkan",
  "grn.post": "GRN diposting",
  "ap_payment.create": "Pembayaran AP",
  "ap_payment.void": "Void pembayaran AP",
  "vendor_credit.approve": "Kredit vendor disetujui",
  "vendor_credit.apply": "Kredit vendor dipakai",
  "purchase_return.revise": "Revisi retur",
  // HRIS karyawan & jadwal (EPIC-056)
  "employee.create": "Karyawan ditambahkan",
  "employee.update": "Data karyawan diubah",
  "employee.resign": "Karyawan resign",
  "employee.rehire": "Karyawan diaktifkan kembali",
  "employee.email_released": "Email karyawan dilepas",
  "employee.schedule_type": "Tipe jadwal diubah",
  "employee.import": "Karyawan diubah lewat import",
  "schedule.fixed_apply": "Jadwal tetap diterapkan",
  "schedule.bulk_upload": "Jadwal diunggah",
};

export const ENTITY_LABELS: Record<string, string> = {
  inventory: "Stok",
  stock_opname: "Stok opname",
  stock_transfer: "Transfer stok",
  purchase_order: "Purchase order",
  grn: "GRN",
  ap_payment: "Pembayaran AP",
  vendor_credit: "Kredit vendor",
  purchase_return: "Retur pembelian",
  "hris.employees": "Karyawan",
};

export const DESTRUCTIVE_ACTIONS = new Set(["ap_payment.void", "po.cancel", "stock.scrap", "employee.resign"]);

import type { SpreadsheetCellValue } from "@/lib/spreadsheet/exceljs-safe";
import type { EmployeeSalary } from "./types";

const collator = new Intl.Collator("id", { sensitivity: "base", numeric: true });

/** Urut nama A–Z (tidak peka huruf besar), lalu NIP (EPIC-075 R3). */
export function sortSalaries(rows: EmployeeSalary[]): EmployeeSalary[] {
  return [...rows].sort(
    (a, b) =>
      collator.compare(a.employee?.full_name ?? "", b.employee?.full_name ?? "") ||
      collator.compare(a.employee?.nip ?? "", b.employee?.nip ?? "")
  );
}

/** Cocok dengan nama atau NIP, tidak peka huruf besar; kata kunci kosong = semua (R4). */
export function matchesSalarySearch(row: EmployeeSalary, query: string): boolean {
  const q = query.trim().toLowerCase();
  if (!q) return true;
  return `${row.employee?.full_name ?? ""} ${row.employee?.nip ?? ""}`.toLowerCase().includes(q);
}

const HEADER = ["NIP", "Nama", "Jabatan", "Departemen", "Entitas", "Gaji Pokok", "Tunjangan Tetap", "Transport",
  "Makan", "Tunjangan Jabatan", "Tunjangan Kesehatan", "Status PTKP", "Efektif"];

const amount = (v: number | string | null | undefined) => Number(v ?? 0) || 0;

/** Baris Excel struktur gaji (R5): header lalu satu baris per versi aktif. */
export function salaryExportRows(rows: EmployeeSalary[], entityName: string): SpreadsheetCellValue[][] {
  return [
    HEADER,
    ...rows.map((s) => [
      s.employee?.nip ?? "-",
      s.employee?.full_name ?? "-",
      s.employee?.position?.title ?? "-",
      s.employee?.department?.name ?? "-",
      s.company?.name ?? entityName,
      amount(s.base_salary),
      amount(s.fixed_allowance),
      amount(s.transport_allowance),
      amount(s.meal_allowance),
      amount(s.position_allowance),
      amount(s.health_allowance),
      s.ptkp_status ?? "-",
      s.effective_date?.slice(0, 10) ?? "-",
    ]),
  ];
}

/**
 * Unduh struktur gaji yang sedang tampil sebagai .xlsx di browser. ExcelJS
 * dipakai langsung (buildXlsxBuffer memakai Buffer Node yang tidak ada di
 * browser) dan dimuat saat tombol ditekan.
 */
export async function downloadSalaryXlsx(rows: EmployeeSalary[], entityName: string, today: string): Promise<void> {
  const { default: ExcelJS } = await import("exceljs");
  const wb = new ExcelJS.Workbook();
  const ws = wb.addWorksheet("Struktur Gaji");
  for (const r of salaryExportRows(rows, entityName)) ws.addRow(r.map((c) => c ?? ""));
  ws.getRow(1).font = { bold: true };
  [14, 28, 22, 18, 22, 14, 16, 12, 12, 18, 20, 12, 12].forEach((w, i) => (ws.getColumn(i + 1).width = w));
  for (let c = 6; c <= 11; c++) ws.getColumn(c).numFmt = "#,##0";
  const blob = new Blob([await wb.xlsx.writeBuffer()], {
    type: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
  });
  const slug = entityName.trim().toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "") || "semua";
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `struktur-gaji-${slug}-${today}.xlsx`;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

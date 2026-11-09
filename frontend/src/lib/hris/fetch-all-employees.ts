import { apiGet } from "@/lib/api-client";

/** Batas `limit` GET /api/hris/employees (domain.ParseDirectoryParams: 1–500). */
export const EMPLOYEE_PAGE_SIZE = 500;
const MAX_PAGES = 50;

type EmployeePage<T> = { data: T[] | null; total?: number };

/**
 * Semua karyawan dari GET /api/hris/employees, halaman demi halaman. Daftar pilihan
 * (atasan, struktur organisasi, combobox) tidak boleh terpotong di satu halaman:
 * dengan `limit=200` karyawan setelah urutan ke-200 menurut abjad tidak bisa dipilih.
 */
export async function fetchAllEmployees<T>(
  query: string,
  get: (url: string) => Promise<EmployeePage<T>> = apiGet
): Promise<T[]> {
  const all: T[] = [];
  for (let page = 1; page <= MAX_PAGES; page++) {
    const res = await get(`/api/hris/employees?${query}&limit=${EMPLOYEE_PAGE_SIZE}&page=${page}`);
    const rows = res.data ?? [];
    all.push(...rows);
    if (rows.length < EMPLOYEE_PAGE_SIZE || (res.total !== undefined && all.length >= res.total)) break;
  }
  return all;
}

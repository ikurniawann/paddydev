import { describe, expect, it, vi } from "vitest";
import { EMPLOYEE_PAGE_SIZE, fetchAllEmployees } from "./fetch-all-employees";

const rows = (n: number, from = 0) => Array.from({ length: n }, (_, i) => ({ id: `e${from + i}` }));

describe("fetchAllEmployees", () => {
  it("mengambil semua halaman sampai total tercapai (lebih dari batas satu halaman)", async () => {
    const get = vi
      .fn()
      .mockResolvedValueOnce({ data: rows(EMPLOYEE_PAGE_SIZE), total: EMPLOYEE_PAGE_SIZE + 120 })
      .mockResolvedValueOnce({ data: rows(120, EMPLOYEE_PAGE_SIZE), total: EMPLOYEE_PAGE_SIZE + 120 });
    const all = await fetchAllEmployees("is_active=true&sort_by=full_name&sort_order=asc", get);
    expect(all).toHaveLength(EMPLOYEE_PAGE_SIZE + 120);
    expect(get).toHaveBeenNthCalledWith(
      1,
      `/api/hris/employees?is_active=true&sort_by=full_name&sort_order=asc&limit=${EMPLOYEE_PAGE_SIZE}&page=1`
    );
    expect(get).toHaveBeenNthCalledWith(
      2,
      `/api/hris/employees?is_active=true&sort_by=full_name&sort_order=asc&limit=${EMPLOYEE_PAGE_SIZE}&page=2`
    );
  });

  it("satu halaman cukup bila datanya sedikit", async () => {
    const get = vi.fn().mockResolvedValue({ data: rows(317), total: 317 });
    expect(await fetchAllEmployees("is_active=true", get)).toHaveLength(317);
    expect(get).toHaveBeenCalledTimes(1);
  });

  it("berhenti pada halaman kosong walau total tidak dikirim", async () => {
    const get = vi
      .fn()
      .mockResolvedValueOnce({ data: rows(EMPLOYEE_PAGE_SIZE) })
      .mockResolvedValueOnce({ data: [] });
    expect(await fetchAllEmployees("is_active=true", get)).toHaveLength(EMPLOYEE_PAGE_SIZE);
    expect(get).toHaveBeenCalledTimes(2);
  });

  it("data null dianggap kosong", async () => {
    const get = vi.fn().mockResolvedValue({ data: null, total: 0 });
    expect(await fetchAllEmployees("is_active=true", get)).toEqual([]);
  });
});

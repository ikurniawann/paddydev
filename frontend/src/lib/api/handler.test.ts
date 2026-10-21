import { describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/auth";
import { apiErrorResponse, apiHandler } from "./handler";

const body = async (res: Response) => ({ status: res.status, json: await res.json() });

describe("apiErrorResponse", () => {
  it("ApiError memakai status dan pesannya", async () => {
    expect(await body(apiErrorResponse(ApiError.notFound("Order tidak ditemukan")))).toEqual({
      status: 404,
      json: { success: false, error: "Order tidak ditemukan" },
    });
  });

  it("unique violation Postgres jadi 409 berpesan ramah", async () => {
    const res = await body(apiErrorResponse({ code: "23505", message: "duplicate key value violates" }));
    expect(res.status).toBe(409);
    expect(res.json).toEqual({ success: false, error: "Data sudah ada di sistem" });
  });

  it.each([
    ["employees_ktp_key", "No. KTP sudah dipakai karyawan lain"],
    ["employees_npwp_key", "NPWP sudah dipakai karyawan lain"],
    ["employees_nip_key", "NIP sudah dipakai karyawan lain"],
  ])("unique violation %s memakai pesan bentrok yang jelas", async (constraint, error) => {
    expect(await body(apiErrorResponse({ code: "23505", constraint, message: "duplicate key" }))).toEqual({
      status: 409,
      json: { success: false, error },
    });
  });

  it("unique violation constraint lain tetap pesan umum", async () => {
    const res = await body(apiErrorResponse({ code: "23505", constraint: "other_key", message: "duplicate key" }));
    expect(res.json).toEqual({ success: false, error: "Data sudah ada di sistem" });
  });

  it("galat lain jadi 500 tanpa membocorkan pesan internal", async () => {
    const spy = vi.spyOn(console, "error").mockImplementation(() => {});
    const res = await body(apiErrorResponse(new Error("connection refused 10.0.0.5")));
    expect(res).toEqual({ status: 500, json: { success: false, error: "Terjadi kesalahan server" } });
    spy.mockRestore();
  });
});

describe("apiHandler", () => {
  it("meneruskan respons sukses dan menangkap ApiError", async () => {
    const ok = apiHandler(async () => Response.json({ success: true }));
    expect((await ok()).status).toBe(200);
    const fail = apiHandler(async () => {
      throw ApiError.forbidden();
    });
    expect((await fail()).status).toBe(403);
  });
});

describe("pemetaan galat Postgres", () => {
  it.each([
    ["23503", 400, "Referensi data tidak valid"],
    ["23514", 400, "Data tidak memenuhi ketentuan"],
    ["23502", 400, "Data wajib diisi"],
    ["22P02", 400, "Format data tidak valid"],
  ])("SQLSTATE %s jadi %i", async (code, status, error) => {
    expect(await body(apiErrorResponse({ code, message: "internal" }))).toEqual({
      status,
      json: { success: false, error },
    });
  });

  it("ApiError.tooManyRequests jadi 429", async () => {
    expect((await body(apiErrorResponse(ApiError.tooManyRequests()))).status).toBe(429);
  });
});
